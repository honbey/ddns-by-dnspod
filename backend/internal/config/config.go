// Package config loads application configuration: a YAML file as the base, env vars on top.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"ddns-by-dnspod/internal/netutil"
)

// defaultOIDCTimeout bounds one call to the identity provider
const defaultOIDCTimeout = 5 * time.Second

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Listen string `mapstructure:"listen"`
	Mode   string `mapstructure:"mode"`
	// LogFormat selects the log handler: "text" for a terminal, "json" for a
	// collector. Both go to stderr, one record per line.
	LogFormat string `mapstructure:"log_format"`
	// HSTS sends Strict-Transport-Security. Only enable it when TLS
	// terminates in front of this process; a plain HTTP deployment would be
	// locked out of its own host otherwise.
	HSTS bool `mapstructure:"hsts"`
	// LogLevel is one of debug, info, warn, error. Debug adds the records an
	// operator only wants when hunting something: a caller walking away from
	// a request, which otherwise looks the same as nothing having happened.
	LogLevel string `mapstructure:"log_level"`
	// TrustedProxies are the CIDRs whose X-Forwarded-For is believed: the
	// reverse proxy in front of this process, nothing wider. An entry that
	// is not a CIDR or an address is refused at startup rather than
	// silently corrected.
	//
	// The default is loopback only, so behind a proxy every request is
	// attributed to the proxy itself until this lists the proxy's network.
	// That is the fail-safe direction: an address nobody can forge, even at
	// the cost of one shared IP in the log.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// RejectBudgetPerMinute caps failed token verifications per attributed
	// source per minute. Past it the answer is 429 instead of 401, which is
	// what bounds the load a flood of invented tokens puts on the issuer.
	// A valid token is never turned away: only failures spend the budget.
	RejectBudgetPerMinute int `mapstructure:"reject_budget_per_minute"`
}

// DatabaseConfig holds database settings
// MetricsConfig holds the Prometheus scrape switch.
type MetricsConfig struct {
	// Enabled serves /metrics. Off by default: the endpoint is unauthenticated
	// and nothing in this repository constrains how the reverse proxy treats
	// it, so a deployment that scrapes nothing would publish its route table
	// and traffic shape for no benefit. Turn it on where a scraper is.
	Enabled bool `mapstructure:"enabled"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

// HistoryConfig holds how long the change history is kept.
type HistoryConfig struct {
	// RetentionDays is the age at which a history row is deleted. 0 keeps
	// everything forever, which is what the table did before it had a keeper.
	RetentionDays int `mapstructure:"retention_days"`
}

// OIDCConfig holds the resource server half of the OIDC setup.
//
// The browser signs in on its own and sends the resulting access token as a
// bearer credential; this process only checks it. Nothing about a caller is remembered between
// requests and no reverse proxy has to be trusted, which is what makes the
// server stateless.
type OIDCConfig struct {
	// Disabled turns off token verification and impersonates DisabledSub.
	// Local development only - never enable it in production.
	Disabled    bool   `mapstructure:"disabled"`
	DisabledSub string `mapstructure:"disabled_sub"`
	// Issuer is the provider's discovery URL, e.g. https://auth.example.com
	Issuer string `mapstructure:"issuer"`
	// ClientID is the client the browser signs in with. Required, and not only
	// for this process: the SPA is handed the same value and cannot start a
	// sign in without it.
	ClientID string `mapstructure:"client_id"`
	// Audience is the value expected in the token's aud claim. Empty means the
	// client id, which is what a provider issuing aud for the client produces.
	Audience string `mapstructure:"audience"`
	// SkipAudienceCheck accepts a token whatever its audience, for providers
	// that issue a different aud than the client id used to sign in.
	SkipAudienceCheck bool `mapstructure:"skip_audience_check"`
	// OpaqueTokens enables the userinfo fallback for non-JWT access tokens.
	// Off by default: a userinfo response proves the token belongs to some user
	// of this issuer, not that it was issued to this client, so the audience
	// check cannot be applied to it at all. Turn it on only against an issuer
	// that scopes its userinfo per client.
	OpaqueTokens bool `mapstructure:"opaque_tokens"`
	// JWKSURL overrides the jwks_uri from the discovery document, for providers
	// that publish an internal-only address behind a reverse proxy
	JWKSURL string `mapstructure:"jwks_url"`
	// TimeoutMS bounds one request to the provider
	TimeoutMS int `mapstructure:"timeout_ms"`
	// UsernameClaim names the claim holding the display name. When it is
	// missing, preferred_username, name, email and the subject are tried in turn.
	UsernameClaim string `mapstructure:"username_claim"`
	GroupsClaim   string `mapstructure:"groups_claim"`
}

// Timeout is how long one call to the provider may take.
func (c OIDCConfig) Timeout() time.Duration {
	if c.TimeoutMS <= 0 {
		return defaultOIDCTimeout
	}
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

// FrontendConfig holds what the browser bundle is told at run time.
//
// The server answers the SPA's config.json itself, so the values it already
// knows are not repeated here: oidc.disabled, oidc.issuer and oidc.client_id are
// handed over directly. Only the fields with no server side counterpart live in
// this section, which is why it is this short.
//
// Keeping the settings on the server is what makes the image generic: a
// container gets them from its environment and nothing has to be rebuilt.
type FrontendConfig struct {
	// AppName is the browser title and the name shown in the UI
	AppName string `mapstructure:"app_name"`
	// APIBaseURL is where the SPA sends its calls. The server mounts the API
	// there, so the default is the route table's own prefix.
	APIBaseURL string `mapstructure:"api_base_url"`
	// ProviderName is how the identity provider is named in the UI. Discovery
	// documents carry no provider name, so it is configuration; an empty value
	// is shown as "OIDC".
	ProviderName string `mapstructure:"provider_name"`
	// LogoutURLTemplate is for providers that implement no OIDC logout, e.g.
	// https://auth.example.com/logout?rd={redirectUrl}. Empty hides the
	// "sign out everywhere" entry.
	LogoutURLTemplate string `mapstructure:"logout_url_template"`
	// Scope is the OAuth scope requested at sign in
	Scope string `mapstructure:"scope"`
	// RedirectPath is where the provider sends the browser back; it has to match
	// the callback registered on the client
	RedirectPath string `mapstructure:"redirect_path"`
}

// CryptoConfig holds credential encryption settings
type CryptoConfig struct {
	KeyFile string `mapstructure:"key_file"`
}

// DNSPodConfig holds Tencent Cloud DNSPod API settings
type DNSPodConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Timeout  int    `mapstructure:"timeout"`
}

// IPEndpoint is an IP detection source that returns a JSON body instead of a
// bare address. Path names the JSON field holding the address, walked from the
// root (e.g. Path:["data","ip"] reads {"data":{"ip":"..."}}). An empty path
// falls back to the "ip" key, matching the common case.
type IPEndpoint struct {
	URL  string   `mapstructure:"url"`
	Path []string `mapstructure:"path"`
}

// IPDetectConfig holds public IP detection settings, with separate v4 and v6
// endpoint pools. Plain pools return the bare address; JSON pools return a JSON
// document with the address under the configured path.
type IPDetectConfig struct {
	V4      []string     `mapstructure:"v4"`
	V6      []string     `mapstructure:"v6"`
	V4JSON  []IPEndpoint `mapstructure:"v4_json"`
	V6JSON  []IPEndpoint `mapstructure:"v6_json"`
	Timeout int          `mapstructure:"timeout"`
}

// SchedulerConfig holds global DDNS scheduler settings
type SchedulerConfig struct {
	Enabled       bool `mapstructure:"enabled"`
	FailThreshold int  `mapstructure:"fail_threshold"`
}

// CacheConfig holds the lifetime of cached DNSPod read results, in seconds.
// Zero or negative disables caching for that call.
type CacheConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	DomainsSec int  `mapstructure:"domains_sec"`
	RecordsSec int  `mapstructure:"records_sec"`
	LinesSec   int  `mapstructure:"lines_sec"`
}

// APIConfig holds the public REST API settings
type APIConfig struct {
	// Enabled is a master switch for everything under /api/v1. Turning it off
	// closes the public API without having to delete the keys.
	Enabled bool `mapstructure:"enabled"`
}

// ACME directory endpoints. The staging one is the endpoint to develop
// against: it issues certificates no browser trusts, and it does not count
// against the production rate limits.
const (
	ACMEDirectoryProduction = "https://acme-v02.api.letsencrypt.org/directory"
	ACMEDirectoryStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// ACMEConfig holds the certificate manager settings.
//
// Only the DNS-01 challenge is implemented: this container publishes no port of
// its own, so proving control of a name has to go through DNS, which is also
// what makes a wildcard certificate possible. The TXT record is published with
// the DNSPod credentials already stored for the DDNS tasks.
type ACMEConfig struct {
	// Enabled turns the manager off: no renewal check, and an issuance asked
	// for by hand is refused as well, rather than starting a run the
	// configuration says should not happen. Certificates already on disk are
	// left alone either way.
	Enabled bool `mapstructure:"enabled"`
	// Directory is the ACME directory endpoint; see ACMEDirectoryProduction and
	// ACMEDirectoryStaging for the two Let's Encrypt ones.
	Directory string `mapstructure:"directory"`
	// Email is the account contact. It is optional - an account registered
	// without one works - but it is the only address the CA can use to warn
	// about a problem with the account.
	//
	// It is read when the account is first registered and does not update it
	// afterwards: the CA keeps the contact it was given, and a client cannot
	// change it without signing in as the account.
	Email string `mapstructure:"email"`
	// OutputDir is where certificates are written. Each certificate gets a
	// subdirectory named after it, holding the four PEM files; a certificate may
	// name a subdirectory of its own, but never a path outside this root.
	OutputDir string `mapstructure:"output_dir"`
	// RenewBeforeDays is how long before notAfter a certificate is renewed.
	// 0 turns automatic renewal off, leaving only the manual button.
	RenewBeforeDays int `mapstructure:"renew_before_days"`
	// CheckHour is the local hour of the daily renewal check, 0-23.
	//
	// A check also runs about a minute after start, so a process that was down
	// at that hour does not skip a day of renewals.
	CheckHour int `mapstructure:"check_hour"`
	// KeyMode is the permission of privkey.pem, in octal. 0600 keeps the key to
	// this process, and that is normally enough: the image runs as root, which a
	// rootless runtime maps to whoever started it. An application running as
	// another user needs 0640 and a group shared with it, arranged on the host.
	KeyMode string `mapstructure:"key_mode"`
	// Timeout bounds one issuance, seconds. A DNS-01 order spends most of it
	// waiting for the CA to validate the challenge.
	Timeout int `mapstructure:"timeout"`
	// PropagationTimeout is how long an issuance waits for the published TXT
	// record to appear in the resolver before telling the CA to validate,
	// seconds. Giving up on it is not a failure: the CA reads the zone's
	// authoritative servers, so the wait is a head start, not a gate.
	PropagationTimeout int `mapstructure:"propagation_timeout"`

	// KeyFileMode is KeyMode parsed into the type the file writer needs. Load
	// sets it; the configuration file only carries the text.
	KeyFileMode os.FileMode `mapstructure:"-"`
}

// NotifierConfig holds the delivery settings of notification channels
type NotifierConfig struct {
	// AllowPrivate lists the CIDRs of internal addresses a notification channel
	// may reach. Loopback, link local, private and multicast addresses are
	// refused unless they match one of these entries, so a channel cannot be
	// pointed at the cloud metadata endpoint or at services inside the network.
	//
	// It is deliberately server configuration and not part of a channel: every
	// logged in user can edit channels, and letting them widen the allow list
	// would hand them the key to the lock.
	AllowPrivate []string `mapstructure:"allow_private"`
}

// Config is the root configuration structure
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Database  DatabaseConfig  `mapstructure:"database"`
	History   HistoryConfig   `mapstructure:"history"`
	OIDC      OIDCConfig      `mapstructure:"oidc"`
	Frontend  FrontendConfig  `mapstructure:"frontend"`
	Crypto    CryptoConfig    `mapstructure:"crypto"`
	DNSPod    DNSPodConfig    `mapstructure:"dnspod"`
	IPDetect  IPDetectConfig  `mapstructure:"ipdetect"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Cache     CacheConfig     `mapstructure:"cache"`
	API       APIConfig       `mapstructure:"api"`
	ACME      ACMEConfig      `mapstructure:"acme"`
	Notifier  NotifierConfig  `mapstructure:"notifier"`
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.listen", ":8080")
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.hsts", false)
	v.SetDefault("server.log_format", "text")
	v.SetDefault("server.log_level", "info")
	// Only loopback by default. Widening this to a private range would let any
	// host inside it choose the address this process attributes the request
	// to, and the default listen is every interface. A deployment behind a
	// proxy on another network has to name that network here - see
	// config.example.yml - because guessing it would be guessing about trust.
	v.SetDefault("server.trusted_proxies", []string{"127.0.0.0/8", "::1/128"})
	// The floor that holds without a rate-limiting proxy in front: a refused
	// token costs a log line at minimum and, on the opaque path, a round trip
	// to the issuer.
	v.SetDefault("server.reject_budget_per_minute", 30)

	v.SetDefault("database.path", "data/ddns.db")

	v.SetDefault("metrics.enabled", false)

	// Ninety days of history rather than none: without a keeper the table only
	// ever grows, and a record that flaps every five minutes fills it with rows
	// nobody will ever page through.
	v.SetDefault("history.retention_days", 90)

	// Every key needs a default: viper only lets AutomaticEnv override a key it
	// already knows about, so a missing one turns its environment variable into
	// a silent no-op. The OIDC and frontend blocks are bound to bare names in
	// Load, which is why their keys look nothing like the DDNS_ prefixed rest.
	v.SetDefault("oidc.disabled", false)
	v.SetDefault("oidc.disabled_sub", "dev-user")
	v.SetDefault("oidc.issuer", "")
	v.SetDefault("oidc.client_id", "")
	v.SetDefault("oidc.audience", "")
	v.SetDefault("oidc.skip_audience_check", false)
	v.SetDefault("oidc.opaque_tokens", false)
	v.SetDefault("oidc.jwks_url", "")
	v.SetDefault("oidc.timeout_ms", 5000)
	v.SetDefault("oidc.username_claim", "preferred_username")
	v.SetDefault("oidc.groups_claim", "groups")

	v.SetDefault("frontend.app_name", "DDNS by DNSPod")
	v.SetDefault("frontend.api_base_url", "/api")
	v.SetDefault("frontend.provider_name", "")
	v.SetDefault("frontend.logout_url_template", "")
	v.SetDefault("frontend.scope", "openid profile email groups")
	v.SetDefault("frontend.redirect_path", "/auth/callback")

	v.SetDefault("crypto.key_file", "data/secret.key")

	v.SetDefault("dnspod.endpoint", "dnspod.tencentcloudapi.com")
	v.SetDefault("dnspod.timeout", 30)

	// No built-in address pool: which services are asked for the public address
	// is a deployment decision, so it lives in the configuration only. See
	// config.example.yml at the repository root for a working set.
	v.SetDefault("ipdetect.v4", []string{})
	v.SetDefault("ipdetect.v6", []string{})
	v.SetDefault("ipdetect.v4_json", []IPEndpoint{})
	v.SetDefault("ipdetect.v6_json", []IPEndpoint{})
	// Per source: how long one service gets before the pool moves on
	v.SetDefault("ipdetect.timeout", 30)

	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.fail_threshold", 3)

	v.SetDefault("cache.enabled", true)
	v.SetDefault("cache.domains_sec", 300)
	v.SetDefault("cache.records_sec", 30)
	v.SetDefault("cache.lines_sec", 600)

	v.SetDefault("api.enabled", true)

	v.SetDefault("acme.enabled", true)
	v.SetDefault("acme.directory", ACMEDirectoryProduction)
	v.SetDefault("acme.email", "")
	// The image overrides this with /certs; a local run writes into the
	// checkout, beside data/.
	v.SetDefault("acme.output_dir", "certs")
	// Let's Encrypt certificates live 90 days, so a month of headroom still
	// leaves two renewals' worth of retries before one expires.
	v.SetDefault("acme.renew_before_days", 30)
	// The middle of the night, when a failed challenge is least likely to be
	// noticed by anyone but the notification channel.
	v.SetDefault("acme.check_hour", 3)
	v.SetDefault("acme.key_mode", "0600")
	v.SetDefault("acme.timeout", 300)
	v.SetDefault("acme.propagation_timeout", 30)

	// Empty means no internal address may be reached by a notification channel
	v.SetDefault("notifier.allow_private", []string{})
}

// Validate reports configuration combinations that are legal on their own but
// almost certainly a mistake. It only warns, never fails: a wrong guess here
// must not stop the service from starting.
//
// The checks that matter most are the sign in settings, because a failure there
// surfaces in the browser rather than in the record updates. The settings that
// make sign in impossible are refused by Load outright; only the ones that
// still leave a working setup are left for a warning.
func (c *Config) Validate() []string {
	var warnings []string

	// Nothing in the code falls back to a built-in pool any more, so an empty
	// one is not "use the defaults" but "detection can never succeed".
	if len(c.IPDetect.V4) == 0 && len(c.IPDetect.V4JSON) == 0 {
		warnings = append(warnings,
			"ipdetect.v4 and ipdetect.v4_json are both empty: no source is left to detect the public IPv4 address, so every A record task fails")
	}
	if len(c.IPDetect.V6) == 0 && len(c.IPDetect.V6JSON) == 0 {
		warnings = append(warnings,
			"ipdetect.v6 and ipdetect.v6_json are both empty: no source is left to detect the public IPv6 address, so every AAAA record task fails")
	}

	// Reported even when verification is off: the setting most likely to be
	// wrong in exactly that setup is the address pool, which is why the rest of
	// the checks still run.
	if c.OIDC.Disabled {
		warnings = append(warnings,
			"AUTH_DISABLED=true: no identity check at all, anyone who can reach this port can read and write DNS records")
	}

	if c.ACME.Enabled {
		if c.ACME.Email == "" {
			warnings = append(warnings,
				"acme.email is empty: the account is registered without a contact address, so the CA has no way to warn about a problem with it")
		}
		switch {
		case c.ACME.RenewBeforeDays <= 0:
			warnings = append(warnings,
				"acme.renew_before_days is 0: certificates are only renewed when someone presses the button")
		case c.ACME.RenewBeforeDays > 60:
			warnings = append(warnings, fmt.Sprintf(
				"acme.renew_before_days is %d: every certificate is renewed on every check, which the CA rate limit does not take kindly; 30 is the usual value",
				c.ACME.RenewBeforeDays))
		}
		// 0640 is the documented setting for a TLS application running as
		// another user, so the group read bit is not a mistake to warn about.
		// Anything beyond the owner and the group is: a key readable by every
		// user on the host, or writable by someone other than its owner.
		if perm := c.ACME.KeyFileMode.Perm(); perm&0o007 != 0 || perm&0o020 != 0 {
			warnings = append(warnings, fmt.Sprintf(
				"acme.key_mode is %s: privkey.pem is readable or writable beyond its owner and group (0600 is the default, 0640 suits a group shared with the TLS application)",
				c.ACME.KeyMode))
		}
		// The other way to get the mode wrong is to lock everybody out of it,
		// including whoever serves the certificate.
		if c.ACME.KeyFileMode.Perm()&0o400 == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"acme.key_mode is %s: privkey.pem is written without read permission for its owner, so the application loading it cannot",
				c.ACME.KeyMode))
		}
	}

	if c.OIDC.SkipAudienceCheck {
		warnings = append(warnings,
			"OIDC_SKIP_AUDIENCE_CHECK=true: a token from any client of this provider is accepted, only do this when the provider cannot be made to issue the right audience")
	}

	// A wildcard entry disables the notification SSRF protection altogether,
	// which is almost never what someone typing a CIDR meant to do.
	for _, item := range c.Notifier.AllowPrivate {
		host := strings.SplitN(strings.TrimSpace(item), "/", 2)[0]
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			warnings = append(warnings,
				"notifier.allow_private contains "+item+": it lets notification channels reach any address, including the cloud metadata service")
			break
		}
	}

	// A malformed entry is dropped when the list is parsed, and the only trace
	// it leaves is a channel that cannot reach something it was meant to.
	if rejected := netutil.Rejected(c.Notifier.AllowPrivate); len(rejected) > 0 {
		warnings = append(warnings,
			"notifier.allow_private ignored "+strings.Join(rejected, ", ")+
				": every entry must be a CIDR or an IP address")
	}

	return warnings
}

// ListensOnLoopback reports whether an address serves only this host. It is the
// line between "developing without a provider" and "every record on the
// network", because AUTH_DISABLED is only ever safe on the first of those.
func ListensOnLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		// A bare ":8080" has no host part to judge, and it serves everywhere.
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

// bareEnvVars are the two blocks the browser also knows about. They keep their
// bare names instead of the DDNS_ prefix used everywhere else, so the SPA's
// build-time variables and this server's run-time ones are the same words.
// Every other key keeps the DDNS_ mapping.
var bareEnvVars = []struct {
	key string
	env string
}{
	{"oidc.disabled", "AUTH_DISABLED"},
	{"oidc.disabled_sub", "AUTH_DISABLED_SUB"},
	{"oidc.issuer", "OIDC_ISSUER"},
	{"oidc.client_id", "OIDC_CLIENT_ID"},
	{"oidc.audience", "OIDC_AUDIENCE"},
	{"oidc.skip_audience_check", "OIDC_SKIP_AUDIENCE_CHECK"},
	{"oidc.jwks_url", "OIDC_JWKS_URL"},
	{"oidc.timeout_ms", "OIDC_TIMEOUT_MS"},
	{"oidc.username_claim", "OIDC_USERNAME_CLAIM"},
	{"oidc.groups_claim", "OIDC_GROUPS_CLAIM"},
	{"frontend.app_name", "APP_NAME"},
	{"frontend.api_base_url", "API_BASE_URL"},
	{"frontend.scope", "OIDC_SCOPE"},
	{"frontend.redirect_path", "OIDC_REDIRECT_PATH"},
	{"frontend.provider_name", "OIDC_PROVIDER_NAME"},
	{"frontend.logout_url_template", "OIDC_LOGOUT_URL"},
}

// Load loads the configuration. An empty path means defaults plus env vars only.
func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	// Env var override: DDNS_SERVER_LISTEN maps to server.listen
	v.SetEnvPrefix("DDNS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	for _, binding := range bareEnvVars {
		if err := v.BindEnv(binding.key, binding.env); err != nil {
			return nil, fmt.Errorf("bind %s: %w", binding.env, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Viper cannot reliably turn a single environment variable into a []string,
	// so the list is read and split here instead of relying on AutomaticEnv.
	// A variable set to nothing is an instruction to clear the list, not an
	// absent one: an operator has to be able to end up trusting nobody.
	if raw, ok := os.LookupEnv("DDNS_NOTIFIER_ALLOW_PRIVATE"); ok {
		cfg.Notifier.AllowPrivate = splitList(raw)
	}
	// The two other list-only blocks: a value such as "https://a.example.com,
	// https://b.example.com" leaves a leading space on the second entry, which
	// then fails every probe for no visible reason. Allowing whitespace around
	// the comma costs nothing and matches what file configuration does.
	cfg.IPDetect.V4 = trimList(cfg.IPDetect.V4)
	cfg.IPDetect.V6 = trimList(cfg.IPDetect.V6)
	// The JSON pools have the same problem on every field: an untrimmed URL
	// fails every probe, and an untrimmed path never matches a key.
	for i := range cfg.IPDetect.V4JSON {
		cfg.IPDetect.V4JSON[i].URL = strings.TrimSpace(cfg.IPDetect.V4JSON[i].URL)
		cfg.IPDetect.V4JSON[i].Path = trimList(cfg.IPDetect.V4JSON[i].Path)
	}
	for i := range cfg.IPDetect.V6JSON {
		cfg.IPDetect.V6JSON[i].URL = strings.TrimSpace(cfg.IPDetect.V6JSON[i].URL)
		cfg.IPDetect.V6JSON[i].Path = trimList(cfg.IPDetect.V6JSON[i].Path)
	}

	// gin panics outside every middleware on a mode it does not recognise, so a
	// typo in this value is corrected here rather than crashing the process.
	cfg.Server.Mode = normalizeMode(cfg.Server.Mode)
	// Durations that only make sense positive. They would otherwise reach the
	// SDK and the prober unchecked, where a negative value is accepted.
	if cfg.DNSPod.Timeout <= 0 {
		cfg.DNSPod.Timeout = 30
	}
	if cfg.IPDetect.Timeout <= 0 {
		cfg.IPDetect.Timeout = 30
	}
	if cfg.OIDC.TimeoutMS <= 0 {
		cfg.OIDC.TimeoutMS = 5000
	}
	// A non-positive budget reads as "no limit" the moment it reaches the
	// counter, which is the one thing the setting exists to prevent.
	if cfg.Server.RejectBudgetPerMinute <= 0 {
		cfg.Server.RejectBudgetPerMinute = 30
	}
	// A check hour off the clock would make the daily entry invalid, and the
	// scheduler refuses the whole spec rather than one field of it.
	if cfg.ACME.CheckHour < 0 || cfg.ACME.CheckHour > 23 {
		slog.Warn("acme.check_hour is not an hour, using 3", "value", cfg.ACME.CheckHour)
		cfg.ACME.CheckHour = 3
	}
	if cfg.ACME.Timeout <= 0 {
		cfg.ACME.Timeout = 300
	}
	if cfg.ACME.PropagationTimeout < 0 {
		cfg.ACME.PropagationTimeout = 0
	}
	// The key mode is written the way chmod and the configuration file write it,
	// as octal text. An unreadable value falls back rather than refusing to
	// start: a certificate manager that will not run over one digit is worse
	// than one that writes 0600.
	if mode, err := strconv.ParseUint(strings.TrimSpace(cfg.ACME.KeyMode), 8, 32); err != nil {
		slog.Warn("acme.key_mode is not an octal file mode, using 0600", "value", cfg.ACME.KeyMode)
		cfg.ACME.KeyFileMode = 0o600
	} else {
		cfg.ACME.KeyFileMode = os.FileMode(mode).Perm()
	}

	// The audience defaults to the client id, so a deployment that does not
	// distinguish the two only sets one value.
	if cfg.OIDC.Audience == "" {
		cfg.OIDC.Audience = cfg.OIDC.ClientID
	}
	// A trailing slash would land in the middle of the discovery URL, and the
	// SPA is handed this value as well.
	cfg.OIDC.Issuer = strings.TrimSuffix(cfg.OIDC.Issuer, "/")

	// Missing sign in settings are refused rather than warned about: no token
	// can be accepted without them, and starting anyway would only move the
	// failure into the browser. An issuer that is configured but unreachable is
	// a different matter - discovery is retried per request on purpose, so the
	// scheduler keeps updating records while the provider is down.
	if !cfg.OIDC.Disabled {
		if cfg.OIDC.Issuer == "" {
			return nil, fmt.Errorf("OIDC_ISSUER is required (or set AUTH_DISABLED=true for local development)")
		}
		if cfg.OIDC.ClientID == "" {
			return nil, fmt.Errorf("OIDC_CLIENT_ID is required")
		}
	}

	return &cfg, nil
}

// splitList never returns nil: an empty result has to mean "this list is now
// empty" and not "no value here", or an operator cannot clear a list through
// the environment.
//
// It splits an environment value on commas or whitespace, dropping blanks
func splitList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
}

// trimList cleans up a list that arrived through Viper, where a value can carry
// the whitespace written around its separating comma.
func trimList(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// normalizeMode maps every value gin does not recognise onto the default. gin's
// SetMode panics rather than returning an error, so an unknown mode has to be
// replaced before it gets there.
func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "debug":
		return "debug"
	case "test":
		return "test"
	case "release", "":
		return "release"
	default:
		slog.Warn("unknown server.mode, falling back to release", "value", mode)
		return "release"
	}
}
