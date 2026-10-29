// Package notifier sends change and alert notifications, supporting ntfy and generic webhooks.
package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/netutil"
)

// httpTimeout bounds a single notification request
const httpTimeout = 15 * time.Second

// dialTimeout bounds how long opening one connection may take
const dialTimeout = 5 * time.Second

// drainLimit is how much of a response is read on the way to closing it, only
// so the connection can be reused.
const drainLimit = 1 << 16

// headerReplacer squashes the line breaks a header may not carry: a stored
// channel is user input and a header is written to the wire verbatim.
var headerReplacer = strings.NewReplacer("\r", "", "\n", " ")

// reservedHeaders are the names a channel may not set, because the sender sets
// them itself: overriding one of them changes what the request is.
var reservedHeaders = map[string]bool{
	"authorization": true,
	"content-type":  true,
	"title":         true,
	"priority":      true,
	"tags":          true,
}

// Record is one row of a message that covers more than one change.
type Record struct {
	SubDomain  string `json:"sub_domain"`
	RecordType string `json:"type"`
	OldIP      string `json:"old_ip"`
	NewIP      string `json:"new_ip"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

// Message is the notification payload
type Message struct {
	Title      string
	Content    string
	Status     string
	Domain     string
	SubDomain  string
	RecordType string
	OldIP      string
	NewIP      string
	TaskName   string
	Time       string
	// Records carries every row this message covers. The fields above mirror
	// the first one, so a template written for a single record keeps
	// rendering; a template that wants all of them ranges over this.
	Records []Record
}

// NtfyConfig holds ntfy channel settings
type NtfyConfig struct {
	Server        string `json:"server"`
	Topic         string `json:"topic"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Priority      int    `json:"priority"`
	Tags          string `json:"tags"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	// TitleTemplate and BodyTemplate are Go templates over Message. Empty means
	// the built-in wording, which is what most channels should stay on: the
	// template exists so an ntfy topic can match a house style, not to become a
	// second webhook.
	TitleTemplate string `json:"title_template"`
	BodyTemplate  string `json:"body_template"`
	// Headers carries extra headers for the publish request. The headers ntfy
	// itself gives meaning to - Title, Priority, Tags - and the transport ones
	// stay under the channel's own fields, so a custom entry cannot quietly
	// change the priority or replace the credentials.
	Headers map[string]string `json:"headers"`
}

// WebhookConfig holds generic webhook channel settings.
// Body is rendered as a Go template; the variables are the Message fields.
type WebhookConfig struct {
	URL           string            `json:"url"`
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	SkipTLSVerify bool              `json:"skip_tls_verify"`
}

// topicRe is what an ntfy topic may consist of.
var topicRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Decryptor decrypts channel passwords stored encrypted.
// Implemented by store.Encryptor; nil means passwords are stored as plain text.
type Decryptor interface {
	Decrypt(string) (string, error)
}

// Config holds the delivery settings of a Sender
type Config struct {
	// AllowPrivate lists the CIDRs of internal addresses a channel may reach.
	// See config.NotifierConfig for why this is not part of a channel.
	AllowPrivate []string
}

// Sender dispatches notifications
type Sender struct {
	// One client per TLS policy, reused across sends so connections are pooled.
	// Building a client per send leaked a transport every time, and the pooled
	// connections it left behind were only reclaimed by the idle timeout.
	normal   *http.Client
	insecure *http.Client
	dec      Decryptor
	// allowed holds the internal addresses a channel may reach, empty by default
	allowed []*net.IPNet
}

// New builds a sender. dec may be nil only in tests; a real deployment always
// has one, because the store creates the key on first start.
func New(dec Decryptor, cfg Config) *Sender {
	nets := netutil.ParseCIDRs(cfg.AllowPrivate)
	return &Sender{
		normal:   &http.Client{Timeout: httpTimeout, Transport: newTransport(false, nets)},
		insecure: &http.Client{Timeout: httpTimeout, Transport: newTransport(true, nets)},
		dec:      dec,
		allowed:  nets,
	}
}

// guardURL validates a destination against the allow list of this sender
func (s *Sender) guardURL(rawURL string) error {
	return GuardURL(rawURL, s.allowed...)
}

// pick returns the client matching the TLS policy of a channel
func (s *Sender) pick(skipTLSVerify bool) *http.Client {
	if skipTLSVerify {
		return s.insecure
	}
	return s.normal
}

// resolveSecret restores an encrypted password.
//
// A decryption failure is an error rather than a fallback to the stored value:
// when there is a decryptor, that value is ciphertext, and handing it to the
// remote server as a Basic Auth password sends the stored secret - encrypted or
// not - somewhere it was never meant to go, and fails the authentication anyway.
// A deployment with no decryptor keeps its plain text passwords.
func (s *Sender) resolveSecret(value string) (string, error) {
	if s.dec == nil || value == "" {
		return value, nil
	}
	plain, err := s.dec.Decrypt(value)
	if err != nil {
		return "", fmt.Errorf("decrypt the channel password: %w", err)
	}
	return plain, nil
}

// SendNtfy sends an ntfy notification
func (s *Sender) SendNtfy(cfg NtfyConfig, msg Message) error {
	if cfg.Server == "" || cfg.Topic == "" {
		return fmt.Errorf("incomplete ntfy config: server and topic are required")
	}
	server := strings.TrimRight(cfg.Server, "/")
	// Checked here as well as at dial time, so a bad server is reported as such
	// instead of surfacing as a connection failure
	if err := s.guardURL(server); err != nil {
		return err
	}
	// The topic is one path segment, not a path: the URL is built by joining, so
	// an unchecked value rewrites the request line - "../../other" walks out of
	// the topic, and "topic?a=b" appends a query. Both are within the guard's
	// host, which is exactly what it does not look at.
	if !topicRe.MatchString(cfg.Topic) {
		return fmt.Errorf("invalid ntfy topic %q: only letters, digits, dot, dash and underscore are allowed", cfg.Topic)
	}
	url := fmt.Sprintf("%s/%s", server, cfg.Topic)

	client := s.pick(cfg.SkipTLSVerify)

	// Templates render over the same Message the webhook gets. A title cannot
	// carry a line break - it goes out as a header - so CR/LF is squashed rather
	// than trusted, which also closes a header-injection hole.
	title := msg.Title
	if cfg.TitleTemplate != "" {
		rendered, err := renderTemplate("ntfy title", cfg.TitleTemplate, msg)
		if err != nil {
			return err
		}
		title = strings.NewReplacer("\r", "", "\n", " ").Replace(rendered)
	}
	body := msg.Content
	if cfg.BodyTemplate != "" {
		rendered, err := renderTemplate("ntfy body", cfg.BodyTemplate, msg)
		if err != nil {
			return err
		}
		body = rendered
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if title != "" {
		req.Header.Set("Title", title)
	}
	if cfg.Priority > 0 {
		req.Header.Set("Priority", fmt.Sprintf("%d", cfg.Priority))
	}
	// Tags goes out as a header too, so it is cleaned like the custom ones
	if tags := strings.TrimSpace(cfg.Tags); tags != "" {
		req.Header.Set("Tags", headerReplacer.Replace(tags))
	}
	// Self-hosted setups that only set a password (an ntfy token, say) still need Basic Auth
	if cfg.Username != "" || cfg.Password != "" {
		password, err := s.resolveSecret(cfg.Password)
		if err != nil {
			return err
		}
		req.SetBasicAuth(cfg.Username, password)
	}

	// Custom headers go out last, with CR/LF squashed in name and value alike -
	// a stored channel is user input, and a header value is written verbatim to
	// the wire. Reserved names are skipped: they are the fields set above.
	names := make([]string, 0, len(cfg.Headers))
	for name := range cfg.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := strings.TrimSpace(headerReplacer.Replace(name))
		if header == "" || reservedHeaders[strings.ToLower(header)] {
			continue
		}
		req.Header.Set(header, headerReplacer.Replace(cfg.Headers[name]))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	// Drained rather than only closed, so the connection goes back to the pool
	// instead of being torn down after one request.
	defer drainAndClose(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ntfy returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// SendWebhook sends a generic webhook notification
//
// GuardURL runs first and the dialer checks again when the connection opens. The
// URL comes from a stored channel config, and without those checks a user could
// point a channel at a link local address such as the cloud metadata endpoint,
// or at any host inside the network, and read the response status back through
// the error message. Reaching internal services by way of a notification channel
// is not a use case worth keeping, which is why the allow list is server
// configuration and not part of a channel.
func (s *Sender) SendWebhook(cfg WebhookConfig, msg Message) error {
	if cfg.URL == "" {
		return fmt.Errorf("incomplete webhook config: url is required")
	}
	if err := s.guardURL(cfg.URL); err != nil {
		return err
	}

	body, err := renderBody(cfg.Body, msg)
	if err != nil {
		return err
	}

	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	client := s.pick(cfg.SkipTLSVerify)

	req, err := http.NewRequest(method, cfg.URL, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	// A header name or value is written to the wire verbatim, and the config is
	// user input, so CR/LF is squashed here as well as on the ntfy path.
	//
	// The reserved names are skipped here as they are on the ntfy path: a custom
	// entry that replaces Authorization or Content-Type changes what this request
	// is, and neither is something a channel should be able to set.
	for k, v := range cfg.Headers {
		name := strings.TrimSpace(headerReplacer.Replace(k))
		if name == "" || reservedHeaders[strings.ToLower(name)] {
			continue
		}
		req.Header.Set(name, headerReplacer.Replace(v))
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// drainAndClose reads what is left of a response so the connection underneath
// can be reused, then closes it.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, drainLimit))
	body.Close()
}

// Dispatch routes a notification by channel type, shared by the executor and the test endpoint
func (s *Sender) Dispatch(chType, configJSON string, msg Message) error {
	switch chType {
	case model.ChannelNtfy:
		var cfg NtfyConfig
		if err := unmarshalConfig(configJSON, &cfg); err != nil {
			return err
		}
		return s.SendNtfy(cfg, msg)
	case model.ChannelWebhook:
		var cfg WebhookConfig
		if err := unmarshalConfig(configJSON, &cfg); err != nil {
			return err
		}
		return s.SendWebhook(cfg, msg)
	default:
		return fmt.Errorf("unknown channel type: %s", chType)
	}
}

// unmarshalConfig parses channel config, treating an empty string as an empty object
func unmarshalConfig(raw string, out interface{}) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("parse channel config: %w", err)
	}
	return nil
}

// templateFuncs are the helpers a channel template can call.
//
// json is the one that matters: a template is written as a JSON body more often
// than not, and a value carrying a quote, a backslash or a newline would
// otherwise break the payload it is dropped into - or change what it says.
// {{json .Content}} puts it in as a properly encoded string instead.
var templateFuncs = template.FuncMap{
	"json": func(value any) (string, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	},
}

// renderTemplate renders a Go template over Message under a named template, so
// parse and execute errors say which field they came from.
func renderTemplate(name, tpl string, msg Message) (string, error) {
	t, err := template.New(name).Funcs(templateFuncs).Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("parse %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, msg); err != nil {
		return "", fmt.Errorf("render %s template: %w", name, err)
	}
	return buf.String(), nil
}

// renderBody renders the body template; an empty template falls back to a built-in JSON layout.
func renderBody(tpl string, msg Message) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		payload := map[string]any{
			"title":      msg.Title,
			"content":    msg.Content,
			"status":     msg.Status,
			"domain":     msg.Domain,
			"sub_domain": msg.SubDomain,
			"type":       msg.RecordType,
			"old_ip":     msg.OldIP,
			"new_ip":     msg.NewIP,
			"records":    msg.Records,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}

	return renderTemplate("webhook", tpl, msg)
}

// newTransport builds the transport shared by both senders.
//
// The address check runs in DialContext, which is where a connection is actually
// opened. Checking the URL before sending is not enough on its own: a redirect
// to an internal address, or a name that resolves publicly during the check and
// privately by the time the connection is opened, would both slip through.
//
// The transport hands DialContext an unresolved host:port, so the name is
// resolved here and every answer is checked. Refusing every name instead would
// be simpler and wrong twice over: it makes a domain target unreachable, and it
// is the resolution, not the refusal, that closes the rebinding gap.
func newTransport(insecure bool, allowed []*net.IPNet) *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("parse dial address: %w", err)
			}
			ips, err := lookupIP(host)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("%s resolved to no address", host)
			}
			for _, ip := range ips {
				if err := checkAddress(ip, host, allowed...); err != nil {
					return nil, err
				}
			}
			// Already an address: nothing to resolve, and nothing to re-resolve
			// either, which is what keeps the check and the connection on the
			// same host. A literal carrying a zone ("fe80::1%eth0") does not
			// reach this branch: ParseIP rejects it and a name with a percent
			// sign does not resolve, so such a target fails closed.
			if net.ParseIP(host) != nil {
				return (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, addr)
			}
			// A name with several addresses: each one was checked, so any of them
			// may be the one that answers.
			var lastErr error
			for _, ip := range ips {
				conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(
					ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

// lookupIP is the resolver the guard uses, at both the URL check and the dial
// check. It is a variable so a test can decide what a name answers with: the
// rule that every answer has to pass is only reachable if a name can be made to
// answer with an internal address.
//
// The default carries its own deadline: net.LookupIP takes no context, and a
// slow or silently dropped answer would otherwise hold a notification send open
// for as long as the operating system cares to wait.
var lookupIP = func(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// GuardURL rejects destinations a notification channel should never reach.
//
// Only http and https are accepted, so file://, gopher:// and friends are out.
// The host is resolved first, because a name such as a public hostname with a
// private A record would otherwise pass a purely textual check. Every resolved
// address is then tested: loopback, link local (the 169.254.0.0/16 metadata
// endpoint lives here), private ranges and the IPv6 equivalents are refused
// unless they appear in allowed.
//
// allowed holds the internal addresses explicitly permitted through
// notifier.allow_private. It is variadic so the plain call stays usable, and an
// empty list means no internal address may be reached at all.
func GuardURL(rawURL string, allowed ...*net.IPNet) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("webhook URL must use http or https, got %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL has no host")
	}
	// A bare name is fine to resolve; an IP literal is checked directly.
	if ip := net.ParseIP(host); ip != nil {
		return checkAddress(ip, host, allowed...)
	}

	ips, err := lookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve webhook host: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("webhook host resolved to no address")
	}
	for _, ip := range ips {
		if err := checkAddress(ip, host, allowed...); err != nil {
			return err
		}
	}
	return nil
}

// isRestricted reports an address a channel may not reach unless it is allowed:
// loopback, the link local range carrying the cloud metadata endpoint, private
// ranges, multicast and the unspecified address.
// cgnatRange is the RFC 6598 shared address space, which providers hand out in
// place of a public address. It is internal in every sense that matters here,
// even though net.IP does not classify it as private.
var cgnatRange = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func isRestricted(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		cgnatRange.Contains(ip)
}

// checkAddress reports whether a single address may be reached.
//
// Public addresses always pass. Restricted ones need a matching entry in
// allowed, which comes from notifier.allow_private; without one, nothing
// internal is reachable no matter which channel asks.
func checkAddress(ip net.IP, host string, allowed ...*net.IPNet) error {
	if !isRestricted(ip) || netutil.Contains(allowed, ip) {
		return nil
	}
	switch {
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return fmt.Errorf("notification channels may not target link local addresses such as the cloud metadata service (%s); add it to notifier.allow_private to permit it", host)
	case ip.IsLoopback():
		return fmt.Errorf("notification channels may not target loopback addresses (%s); add it to notifier.allow_private to permit it", host)
	default:
		return fmt.Errorf("notification channels may not target internal addresses (%s); add it to notifier.allow_private to permit it", host)
	}
}
