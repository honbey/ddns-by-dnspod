// Package auth implements the resource-server half of the OIDC integration: it
// decides whether an access token is acceptable and nothing else.
//
// Two token shapes are handled:
//   - JWT access tokens (what Authelia issues) are verified locally against the
//     issuer's JWKS, so a request costs no round trip to the provider.
//   - Opaque tokens fall back to the issuer's userinfo endpoint.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ddns-by-dnspod/internal/config"
)

// Identity is the authenticated caller extracted from a token.
type Identity struct {
	Subject  string
	Username string
	Groups   []string
}

// discoveryTimeout bounds one attempt to read the provider metadata.
const discoveryTimeout = 15 * time.Second

// minJWKSRefresh keeps an unknown signing key from turning every request into a
// fetch of the key set.
const minJWKSRefresh = time.Minute

// setupRetryInterval is how long a failed discovery is remembered. Without it
// every request that arrives while the provider is down repeats two round trips
// to it, which is a queue in front of this process and an amplifier pointed at
// the issuer - reachable by anyone who can send one header.
//
// A variable rather than a constant so a test can shrink the window instead of
// sleeping through it.
var setupRetryInterval = 30 * time.Second

// maxProviderBytes caps how much of a provider answer is decoded. Nothing this
// process asks an issuer for is larger than a few kilobytes, and reading without
// a limit lets a hostile or broken endpoint decide how much memory it uses.
const maxProviderBytes = 1 << 20
const (
	// opaqueInFlight is how many userinfo lookups may run at once; beyond it
	// requests fail closed so the issuer is never flooded.
	opaqueInFlight = 8
	// opaquePerSourceInFlight bounds one address's share of those slots.
	opaquePerSourceInFlight = 2
	// maxOpaqueSources bounds the per-source gate map.
	maxOpaqueSources = 4096
	// maxOpaqueEntries bounds both opaque caches.
	maxOpaqueEntries = 4096
	// opaqueNegativeTTL is how long a refused opaque token is remembered.
	opaqueNegativeTTL = 30 * time.Second
	// opaquePositiveTTL is how long a proven identity is reused.
	opaquePositiveTTL = 30 * time.Second
)

// errTokenRejected marks a refusal that says something about the token: the
// issuer answered 401/403. An outage says nothing about it and must not be
// cached as a bad token.
var errTokenRejected = errors.New("the provider refused the token")

type cachedIdentity struct {
	until time.Time
	id    *Identity
}

// Verifier validates bearer tokens against the configured issuer.
type Verifier struct {
	cfg config.OIDCConfig

	// configErr is a startup finding that makes every token unacceptable. It is
	// kept rather than only logged: discovery runs again on the first request,
	// so an issuer refused here would otherwise be consulted again by the very
	// code path that should have stopped it.
	configErr error

	client   *http.Client
	issuer   string
	userinfo string
	jwksURL  string

	mu        sync.RWMutex
	ready     bool
	keys      map[string]any // kid -> *rsa.PublicKey | *ecdsa.PublicKey
	fetchedAt time.Time

	// Discovery is serialised and its failure remembered, both under setupMu
	// rather than mu: mu guards the state, and holding it across two round trips
	// to the provider would block every reader, including requests whose token
	// could already have been verified.
	setupMu      sync.Mutex
	lastSetup    time.Time
	lastSetupErr error

	// opaqueMu guards the caches and the per-source gate map. The maps and the
	// global channel are written by unauthenticated callers, so all are bounded.
	opaqueMu    sync.Mutex
	opaqueBad   map[[sha256.Size]byte]time.Time
	opaqueGood  map[[sha256.Size]byte]cachedIdentity
	opaqueGates map[string]chan struct{}
	opaqueInUse chan struct{}
}

// state reads the discovery results under the lock.
//
// setup writes them, so an unlocked read races with whichever request happens to
// trigger it, and can observe an empty userinfo endpoint long after discovery
// succeeded - which then reads as "the issuer publishes no userinfo endpoint".
func (v *Verifier) state() (ready bool, userinfo string) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.ready, v.userinfo
}

// discovery is the subset of the OIDC discovery document we rely on.
type discovery struct {
	JWKSURI          string `json:"jwks_uri"`
	UserinfoEndpoint string `json:"userinfo_endpoint"`
}

// jwksDocument is the subset of a JWKS response we rely on.
type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KID string `json:"kid"`
	KTY string `json:"kty"`
	// RSA
	N string `json:"n"`
	E string `json:"e"`
	// EC
	CRV string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// New builds the verifier.
//
// Discovery is attempted right away so a misconfigured issuer shows up in the
// log at startup, but a failure is not fatal and is not remembered: the DDNS
// scheduler has to keep running whether or not anybody can sign in, and the
// next request retries the discovery.
func New(cfg config.OIDCConfig) *Verifier {
	v := &Verifier{
		cfg:         cfg,
		client:      &http.Client{Timeout: cfg.Timeout(), CheckRedirect: checkRedirect},
		issuer:      strings.TrimSuffix(cfg.Issuer, "/"),
		keys:        map[string]any{},
		opaqueBad:   map[[sha256.Size]byte]time.Time{},
		opaqueGood:  map[[sha256.Size]byte]cachedIdentity{},
		opaqueInUse: make(chan struct{}, opaqueInFlight),
	}
	if cfg.Disabled {
		return v
	}

	// An issuer that is neither https nor loopback means discovery and JWKS
	// travel in the clear, and whoever sits in the middle decides what counts
	// as a valid identity here. The refusal is remembered: discovery runs again
	// on the first request, so logging it here would not stop it.
	if err := requireTrustedURL("oidc.issuer", v.issuer); err != nil {
		slog.Error("the identity provider will not be trusted", "error", err)
		v.configErr = err
		return v
	}

	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	if err := v.setup(ctx); err != nil {
		slog.Warn("the identity provider is not ready yet, signing in will fail until it is", "error", err)
	}
	return v
}

// Enabled reports whether bearer tokens are being checked at all.
// Health reports whether this process can check tokens at all: verification
// off is healthy by definition, and so is an issuer that has been reached
// once. A provider nobody can reach produces a 401 for every user, which is
// what readiness is for.
func (v *Verifier) Health() (bool, string) {
	if !v.Enabled() {
		return true, "disabled"
	}
	if ready, _ := v.state(); ready {
		return true, "ready"
	}
	return false, "the identity provider has not answered yet"
}

func (v *Verifier) Enabled() bool { return !v.cfg.Disabled }

// DevSub is the subject impersonated while verification is off.
func (v *Verifier) DevSub() string { return v.cfg.DisabledSub }

// Verify validates a raw bearer token and returns its identity.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Identity, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if raw == "" {
		return nil, fmt.Errorf("missing bearer token")
	}

	if v.configErr != nil {
		return nil, fmt.Errorf("the identity provider is misconfigured: %w", v.configErr)
	}

	if ready, _ := v.state(); !ready {
		if err := v.ensureSetup(ctx); err != nil {
			return nil, err
		}
	}

	if !IsOpaqueToken(raw) {
		return v.verifyJWT(ctx, raw)
	}
	if !v.cfg.OpaqueTokens {
		return nil, fmt.Errorf("opaque tokens are disabled by configuration")
	}
	return v.verifyOpaque(ctx, raw)
}

// IsOpaqueToken reports whether a raw Authorization header value takes the
// userinfo path. A JWT has exactly two dots; anything else is treated as
// opaque. The length floor keeps a probe like "Bearer x" off the expensive
// path. The check is exported so the middleware can apply the rejection budget
// before the round trip it is meant to bound.
func IsOpaqueToken(raw string) bool {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if raw == "" {
		return false
	}
	return !(strings.Count(raw, ".") == 2 || len(raw) < 20)
}

func (v *Verifier) verifyJWT(ctx context.Context, raw string) (*Identity, error) {
	opts := []jwt.ParserOption{
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
	}
	if v.cfg.Audience != "" && !v.cfg.SkipAudienceCheck {
		opts = append(opts, jwt.WithAudience(v.cfg.Audience))
	}

	var claims jwt.MapClaims
	if _, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	}, opts...); err != nil {
		return nil, fmt.Errorf("reject the access token: %w", err)
	}
	return v.identityFrom(claims)
}

// KnownOpaque reports whether this exact opaque token was verified recently.
// The middleware uses it to let a proven token through a source whose failure
// budget is exhausted; the token has already paid for its userinfo lookup.
func (v *Verifier) KnownOpaque(raw string) bool {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if !v.cfg.OpaqueTokens || raw == "" {
		return false
	}
	sum := sha256.Sum256([]byte(raw))
	v.opaqueMu.Lock()
	defer v.opaqueMu.Unlock()
	hit, ok := v.opaqueGood[sum]
	return ok && time.Now().Before(hit.until)
}

// verifyOpaque trades an opaque token for its claims at the userinfo endpoint.
// The caller is unauthenticated: a token already refused is answered from
// cache, a proven identity is reused briefly, and at most opaqueInFlight
// lookups run at once.
func (v *Verifier) verifyOpaque(ctx context.Context, raw string) (*Identity, error) {
	sum := sha256.Sum256([]byte(raw))

	v.opaqueMu.Lock()
	if until, bad := v.opaqueBad[sum]; bad {
		if time.Now().Before(until) {
			v.opaqueMu.Unlock()
			return nil, fmt.Errorf("this token was recently refused")
		}
		delete(v.opaqueBad, sum)
	}
	if hit, good := v.opaqueGood[sum]; good && time.Now().Before(hit.until) {
		v.opaqueMu.Unlock()
		return hit.id, nil
	}
	v.opaqueMu.Unlock()

	sourceGate := v.opaqueGate(sourceFrom(ctx))
	select {
	case sourceGate <- struct{}{}:
		defer func() { <-sourceGate }()
	default:
		return nil, fmt.Errorf("too many token lookups in flight for this source")
	}
	select {
	case v.opaqueInUse <- struct{}{}:
		defer func() { <-v.opaqueInUse }()
	default:
		return nil, fmt.Errorf("too many token lookups in flight")
	}

	id, err := v.resolveUserinfo(ctx, raw)
	if err != nil {
		if errors.Is(err, errTokenRejected) {
			v.opaqueMu.Lock()
			v.pruneOpaqueLocked()
			v.opaqueBad[sum] = time.Now().Add(opaqueNegativeTTL)
			v.opaqueMu.Unlock()
		}
		return nil, err
	}

	v.opaqueMu.Lock()
	v.pruneOpaqueLocked()
	v.opaqueGood[sum] = cachedIdentity{until: time.Now().Add(opaquePositiveTTL), id: id}
	v.opaqueMu.Unlock()
	return id, nil
}

func (v *Verifier) resolveUserinfo(ctx context.Context, raw string) (*Identity, error) {
	_, userinfo := v.state()
	if userinfo == "" {
		return nil, fmt.Errorf("opaque token, but the issuer publishes no userinfo endpoint")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfo, nil)
	if err != nil {
		return nil, fmt.Errorf("build the userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+raw)

	res, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call the userinfo endpoint: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("the userinfo endpoint refused the token: %w", errTokenRejected)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the userinfo endpoint responded %d", res.StatusCode)
	}

	var info map[string]any
	if err := json.NewDecoder(io.LimitReader(res.Body, maxProviderBytes)).Decode(&info); err != nil {
		return nil, fmt.Errorf("read the userinfo response: %w", err)
	}
	return v.identityFrom(info)
}

func (v *Verifier) pruneOpaqueLocked() {
	if len(v.opaqueBad) <= maxOpaqueEntries && len(v.opaqueGood) <= maxOpaqueEntries {
		return
	}
	now := time.Now()
	for sum, until := range v.opaqueBad {
		if now.After(until) {
			delete(v.opaqueBad, sum)
		}
	}
	for sum, hit := range v.opaqueGood {
		if now.After(hit.until) {
			delete(v.opaqueGood, sum)
		}
	}
	if len(v.opaqueBad) > maxOpaqueEntries {
		v.opaqueBad = map[[sha256.Size]byte]time.Time{}
	}
	if len(v.opaqueGood) > maxOpaqueEntries {
		v.opaqueGood = map[[sha256.Size]byte]cachedIdentity{}
	}
}

func (v *Verifier) opaqueGate(source string) chan struct{} {
	v.opaqueMu.Lock()
	defer v.opaqueMu.Unlock()
	if v.opaqueGates == nil {
		v.opaqueGates = map[string]chan struct{}{}
	}
	if gate, ok := v.opaqueGates[source]; ok {
		return gate
	}
	if len(v.opaqueGates) >= maxOpaqueSources {
		v.opaqueGates = map[string]chan struct{}{}
	}
	gate := make(chan struct{}, opaquePerSourceInFlight)
	v.opaqueGates[source] = gate
	return gate
}

type sourceKey struct{}

// WithSource tags a verification with the address the request came from, so
// the opaque path can bound its work per source instead of only globally.
func WithSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, sourceKey{}, source)
}

func sourceFrom(ctx context.Context) string {
	source, _ := ctx.Value(sourceKey{}).(string)
	return source
}

// identityFrom turns claims into an identity.
//
// The name is picked through a chain of fallbacks because providers disagree on
// which claim they fill: Authelia sets preferred_username, others only set name
// or email. The subject is always there, so it is the last resort.
func (v *Verifier) identityFrom(claims map[string]any) (*Identity, error) {
	subject := stringClaim(claims, "sub")
	if subject == "" {
		return nil, fmt.Errorf("the token carries no subject claim")
	}

	username := stringClaim(claims, v.cfg.UsernameClaim)
	for _, fallback := range []string{"preferred_username", "name", "email"} {
		if username != "" {
			break
		}
		username = stringClaim(claims, fallback)
	}
	if username == "" {
		username = subject
	}

	return &Identity{
		Subject:  subject,
		Username: username,
		Groups:   stringListClaim(claims, v.cfg.GroupsClaim),
	}, nil
}

// ensureSetup runs discovery when the verifier is not ready yet, at most one
// attempt at a time and at most once per retry window.
//
// The naive version - every request that finds an unready verifier runs setup
// itself - turns a provider that is down into two serialised round trips per
// request, all of them held behind the same lock, which is a queue in front of
// this process and an outbound amplifier pointed at the issuer. Both are
// reachable by anyone who can send one Authorization header.
func (v *Verifier) ensureSetup(ctx context.Context) error {
	v.setupMu.Lock()
	defer v.setupMu.Unlock()

	if ready, _ := v.state(); ready {
		return nil
	}
	if v.lastSetupErr != nil && time.Since(v.lastSetup) < setupRetryInterval {
		return v.lastSetupErr
	}
	v.lastSetup = time.Now()
	v.lastSetupErr = v.setup(ctx)
	return v.lastSetupErr
}

// setup resolves the discovery document and primes the key set. It is safe to
// call more than once: a provider that comes back later is picked up by the next
// request rather than staying broken until the process restarts.
func (v *Verifier) setup(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var doc discovery
	if err := v.fetchJSON(ctx, v.issuer+"/.well-known/openid-configuration", &doc); err != nil {
		return fmt.Errorf("discover the identity provider at %s: %w", v.issuer, err)
	}
	if doc.JWKSURI == "" {
		return fmt.Errorf("the discovery document at %s has no jwks_uri", v.issuer)
	}
	// An explicit override keeps working with providers that publish an
	// internal-only jwks_uri behind a reverse proxy.
	v.jwksURL = v.cfg.JWKSURL
	if v.jwksURL == "" {
		v.jwksURL = doc.JWKSURI
	}
	v.userinfo = doc.UserinfoEndpoint

	// What discovery advertises is as trusted as the issuer itself: an http
	// endpoint here would let whoever is on the path pick the verification key.
	if err := requireTrustedURL("jwks_uri", v.jwksURL); err != nil {
		return err
	}
	if v.userinfo != "" {
		if err := requireTrustedURL("userinfo_endpoint", v.userinfo); err != nil {
			return err
		}
	}

	if _, err := v.refreshKeysLocked(ctx); err != nil {
		return err
	}
	v.ready = true
	slog.Info("the identity provider is ready", "issuer", v.issuer)
	return nil
}

// key returns the key for kid, refreshing the key set on a miss.
func (v *Verifier) key(ctx context.Context, kid string) (any, error) {
	v.mu.RLock()
	k, ok := v.lookupLocked(kid)
	age := time.Since(v.fetchedAt)
	v.mu.RUnlock()

	if ok {
		return k, nil
	}
	if age < minJWKSRefresh {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}

	// Locked across the refresh, with a second look before doing it: the age
	// above is read before the lock is taken, so concurrent requests carrying an
	// unknown key used to each make their own call to the provider - one burst
	// per unlucky token, and an easy way to multiply traffic at the issuer.
	v.mu.Lock()
	if k, ok := v.keys[kid]; ok {
		v.mu.Unlock()
		return k, nil
	}
	if time.Since(v.fetchedAt) >= minJWKSRefresh {
		if _, err := v.refreshKeysLocked(ctx); err != nil {
			v.mu.Unlock()
			return nil, err
		}
	}
	// Read before releasing the lock: taking it again afterwards would let
	// another refresh slip in between and drop a key this call just fetched.
	k, ok = v.lookupLocked(kid)
	v.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	return k, nil
}

// lookupLocked resolves a token's key id to something the parser can verify
// with. Requires the lock held.
//
// A token that carries no kid may have been signed by any key of the set, so it
// gets all of them rather than one: the parser tries each in turn. That is also
// why keys without a kid are stored under a synthetic id - several of them would
// otherwise land on the same entry and only the last one would survive.
func (v *Verifier) lookupLocked(kid string) (any, bool) {
	if kid != "" {
		k, ok := v.keys[kid]
		return k, ok
	}
	if len(v.keys) == 0 {
		return nil, false
	}
	set := jwt.VerificationKeySet{Keys: make([]jwt.VerificationKey, 0, len(v.keys))}
	for _, k := range v.keys {
		set.Keys = append(set.Keys, k)
	}
	return set, true
}

func (v *Verifier) refreshKeysLocked(ctx context.Context) (map[string]any, error) {
	var doc jwksDocument
	if err := v.fetchJSON(ctx, v.jwksURL, &doc); err != nil {
		return nil, err
	}

	keys := make(map[string]any, len(doc.Keys))
	for i, k := range doc.Keys {
		pub, err := publicKeyFromJWK(k)
		if err != nil || pub == nil {
			continue
		}
		// A key without a kid still has to end up in the map, and a synthetic id
		// keeps several of them from overwriting each other. Nothing looks them
		// up by it: a token carrying no kid is checked against the whole set.
		id := k.KID
		if id == "" {
			id = fmt.Sprintf("no-kid-%d", i)
		}
		keys[id] = pub
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("the key set at %s held no usable key", v.jwksURL)
	}

	v.keys = keys
	v.fetchedAt = time.Now()
	return keys, nil
}

func (v *Verifier) fetchJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s responded %d", url, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxProviderBytes)).Decode(out)
}

// stringClaim reads a claim holding a single string.
func stringClaim(claims map[string]any, name string) string {
	if name == "" {
		return ""
	}
	if v, ok := claims[name].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// stringListClaim reads a claim holding a group list. Providers are not
// consistent about this one: Authelia sends an array, but a plain string is a
// legal way to carry a single group, so both are accepted.
func stringListClaim(claims map[string]any, name string) []string {
	if name == "" {
		return nil
	}
	raw, ok := claims[name].([]any)
	if !ok {
		if single := stringClaim(claims, name); single != "" {
			return []string{single}
		}
		return nil
	}

	groups := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			groups = append(groups, strings.TrimSpace(s))
		}
	}
	return groups
}

func publicKeyFromJWK(k jwk) (any, error) {
	switch k.KTY {
	case "RSA":
		if k.N == "" || k.E == "" {
			return nil, fmt.Errorf("incomplete RSA key")
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{
			N: new(big.Int).SetBytes(n),
			E: int(new(big.Int).SetBytes(e).Int64()),
		}, nil

	case "EC":
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		var curve elliptic.Curve
		switch k.CRV {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported EC curve %q", k.CRV)
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(x),
			Y:     new(big.Int).SetBytes(y),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported key type %q", k.KTY)
	}
}

// maxAuthRedirects bounds a redirect chain. Five is well past what a discovery
// document or a userinfo endpoint should need, and keeps a malicious chain from
// walking the network indefinitely.
const maxAuthRedirects = 5

// checkRedirect applies the same trust rule to every hop. The initial URL is
// checked before the request; without this, an https endpoint could redirect
// to http (or an internal address) and the client would follow it - for
// userinfo, carrying the bearer token.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxAuthRedirects {
		return fmt.Errorf("stopped after %d redirects", maxAuthRedirects)
	}
	return requireTrustedURL("redirect", req.URL.String())
}

// requireTrustedURL refuses a URL whose traffic anyone on the path could read
// or rewrite. Discovery and the key fetch decide what counts as a valid
// identity here, so an http endpoint off this host hands that decision to
// whoever sits in the middle.
func requireTrustedURL(what, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s is not an absolute URL: %q", what, raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("%s must be https, got %q", what, raw)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
