// Package ipdetect detects the public IP of this host, with separate IPv4 and IPv6 pools.
package ipdetect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Family is an IP address family, matching a DNS record type
type Family string

const (
	// V4 maps to A records
	V4 Family = "A"
	// V6 maps to AAAA records
	V6 Family = "AAAA"
)

// defaultTimeoutSec is how long one source gets before the next one is tried.
//
// Sources are probed one at a time, so this is also how long a failover to the
// next source takes when one is broken. It is generous on purpose: a run
// happens every few minutes, far slower than the DNS TTL it feeds, so there is
// no reason to give up on a source quickly.
const defaultTimeoutSec = 30

// Endpoint is one IP detection source. Path is optional: when set, the
// response is parsed as JSON and the address is read from that path; otherwise
// the bare response body is used directly.
type Endpoint struct {
	URL  string
	Path []string
}

// Detector probes the public IP, trying the pool in order and returning the first success
type Detector struct {
	V4Endpoints []string
	V6Endpoints []string
	V4JSON      []Endpoint
	V6JSON      []Endpoint
	Timeout     time.Duration
	client      *http.Client
}

// New builds a detector
func New(v4, v6 []string, timeoutSec int) *Detector {
	return NewWithJSON(v4, v6, nil, nil, timeoutSec)
}

// NewWithJSON builds a detector with additional JSON-returning sources for each family
func NewWithJSON(v4, v6 []string, v4JSON, v6JSON []Endpoint, timeoutSec int) *Detector {
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}
	return &Detector{
		V4Endpoints: v4,
		V6Endpoints: v6,
		V4JSON:      v4JSON,
		V6JSON:      v6JSON,
		Timeout:     time.Duration(timeoutSec) * time.Second,
		client:      &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// Detect probes the public IP for an address family.
// The result is validated against the family so an IPv4 never lands in an AAAA record.
//
// The pool is walked one source at a time, in configuration order, and the
// first usable answer wins. Probing every source at once used to be the way,
// but a run happens every few minutes while the DNS TTL is ten times that, so
// there is nothing to gain from asking six services the same question at the
// same moment: the preferred source answers on its own in the common case, and
// the rest are only asked when it cannot.
//
// Each source gets its own deadline, so a hanging one cannot hold up the next
// for longer than the configured timeout, and the caller's context still caps
// the whole call.
func (d *Detector) Detect(ctx context.Context, family Family) (string, error) {
	endpoints, jsonEndpoints := d.V4Endpoints, d.V4JSON
	if family == V6 {
		endpoints, jsonEndpoints = d.V6Endpoints, d.V6JSON
	}

	var lastErr error

	// sourcesLeft drives the per-source deadline below
	sourcesLeft := len(endpoints) + len(jsonEndpoints)

	// sourceDeadline caps one source by its share of what is left of the
	// caller's budget, rather than giving each of them the full timeout: with
	// several sources that hang, the pool used up the budget before reaching
	// the last ones and the failure read like they had all failed.
	//
	// The share includes the source being tried, which is why the count below
	// has to still be whole when this runs.
	sourceDeadline := func() time.Duration {
		left := sourcesLeft
		if left < 1 {
			left = 1
		}
		deadline := d.Timeout
		if due, ok := ctx.Deadline(); ok {
			if share := time.Until(due) / time.Duration(left); share < deadline {
				deadline = share
			}
		}
		return deadline
	}

	// try returns the address when this source answered with a usable one
	try := func(url string, path []string, isJSON bool) (string, bool) {
		if ctx.Err() != nil {
			return "", false
		}

		deadline := sourceDeadline()
		sourcesLeft--
		// Nothing left to hand out: reported as an abort below rather than as a
		// source that answered nothing, which a zero or negative timeout would
		// make every remaining source look like.
		if deadline <= 0 {
			return "", false
		}

		probeCtx, cancel := context.WithTimeout(ctx, deadline)
		defer cancel()

		raw, err := d.fetch(probeCtx, url)
		if err == nil && isJSON {
			raw, err = extractJSONPath(raw, path)
			if err != nil {
				err = fmt.Errorf("%s JSON parse failed: %w", url, err)
			}
		}
		if err == nil {
			var ip string
			if ip, err = d.validate(raw, family, url); err == nil {
				return ip, true
			}
		}
		lastErr = err
		return "", false
	}

	for _, endpoint := range endpoints {
		if ip, ok := try(endpoint, nil, false); ok {
			return ip, nil
		}
	}
	// The JSON sources come last: they are a fallback for hosts whose only
	// usable service returns a document, and must not outrank a plain one.
	for _, ep := range jsonEndpoints {
		if ip, ok := try(ep.URL, ep.Path, true); ok {
			return ip, nil
		}
	}

	// Checked before the stored error: when the caller's deadline ran out while
	// the pool was being walked, that cancellation is why detection failed, not
	// whichever source happened to be tried last.
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("detection aborted: %w", err)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("endpoint pool is empty")
	}
	// Sequential probing makes this the failure of the last source tried, which
	// is enough to see why none of them worked
	return "", fmt.Errorf("every detection endpoint failed: %w", lastErr)
}

// validate checks that raw is a legal IP of the requested family.
func (d *Detector) validate(raw string, family Family, endpoint string) (string, error) {
	raw = strings.TrimSpace(raw)
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("%s did not return a valid IP: %q", endpoint, raw)
	}
	isV4 := ip.To4() != nil
	switch {
	case family == V4 && !isV4:
		return "", fmt.Errorf("%s returned an IPv6 address but IPv4 is required: %s", endpoint, raw)
	case family == V6 && isV4:
		return "", fmt.Errorf("%s returned an IPv4 address but IPv6 is required: %s", endpoint, raw)
	}
	return ip.String(), nil
}

// extractJSONPath walks path from the JSON root and returns the string at that node.
// An empty path defaults to ["ip"].
func extractJSONPath(raw string, path []string) (string, error) {
	if len(path) == 0 {
		path = []string{"ip"}
	}
	var doc interface{}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	cur := doc
	for _, key := range path {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("path %q not found", key)
		}
		cur, ok = obj[key]
		if !ok {
			return "", fmt.Errorf("path %q not found", key)
		}
	}
	s, ok := cur.(string)
	if !ok {
		return "", fmt.Errorf("path %v is not a string", path)
	}
	return s, nil
}

func (d *Detector) fetch(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ddns-by-dnspod/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	// Cap the read size so a malformed response cannot exhaust memory
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Key returns the short family name ("v4" / "v6").
//
// The Family value itself is a DNS record type, which is the right thing when
// talking to DNSPod but wrong anywhere the address is not tied to a record,
// such as the public IP change history.
func (f Family) Key() string {
	if f == V6 {
		return "v6"
	}
	return "v4"
}

// FamilyOf derives the address family from a record type
func FamilyOf(recordType string) Family {
	if strings.EqualFold(recordType, "AAAA") {
		return V6
	}
	return V4
}
