package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ddns-by-dnspod/internal/config"
)

// newIssuer starts a provider that knows exactly one RSA key, so a token can be
// signed the way Authelia would and then verified against a real key set.
func newIssuer(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate a signing key: %v", err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		discoveryHits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":            srv.URL,
			"jwks_uri":          srv.URL + "/jwks",
			"userinfo_endpoint": srv.URL + "/userinfo",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": signingKeyID,
				"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(bigEndian(key.PublicKey.E)),
			}},
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "sub-2",
			"preferred_username": "li",
			"groups":             []string{"ops"},
		})
	})

	return key, srv
}

// discoveryHits counts the discovery fetches the test issuers answered. It is
// package level because the handler that reports them is registered inside
// newIssuer, and no test in this package runs in parallel.
var discoveryHits atomic.Int64

const signingKeyID = "test-key"

func bigEndian(e int) []byte {
	return []byte{byte(e >> 16), byte(e >> 8), byte(e)}
}

func signToken(t *testing.T, key *rsa.PrivateKey, issuer, audience string, ttl time.Duration, kid string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                issuer,
		"sub":                "sub-1",
		"preferred_username": "zhang",
		"groups":             []string{"admins"},
		"exp":                time.Now().Add(ttl).Unix(),
	}
	if audience != "" {
		claims["aud"] = audience
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign the token: %v", err)
	}
	return signed
}

func newTestVerifier(issuer, audience string) *Verifier {
	return New(config.OIDCConfig{
		Issuer:        issuer,
		ClientID:      audience,
		Audience:      audience,
		UsernameClaim: "preferred_username",
		GroupsClaim:   "groups",
		TimeoutMS:     5000,
		OpaqueTokens:  true,
	})
}

func TestVerifyAcceptsATokenFromTheIssuer(t *testing.T) {
	key, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	id, err := v.Verify(context.Background(), "Bearer "+signToken(t, key, srv.URL, "ddns", time.Hour, signingKeyID))
	if err != nil {
		t.Fatalf("a token from the issuer must be accepted: %v", err)
	}
	if id.Subject != "sub-1" || id.Username != "zhang" || len(id.Groups) != 1 || id.Groups[0] != "admins" {
		t.Fatalf("identity = %+v, want sub-1 / zhang / [admins]", id)
	}
}

// A token issued for another client must not open this one: that is the whole
// point of checking the audience.
func TestVerifyRejectsATokenForAnotherAudience(t *testing.T) {
	key, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	if _, err := v.Verify(context.Background(), signToken(t, key, srv.URL, "something-else", time.Hour, signingKeyID)); err == nil {
		t.Fatal("a token for another audience must be rejected")
	}
}

func TestVerifyRejectsAnExpiredToken(t *testing.T) {
	key, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	if _, err := v.Verify(context.Background(), signToken(t, key, srv.URL, "ddns", -time.Hour, signingKeyID)); err == nil {
		t.Fatal("an expired token must be rejected")
	}
}

// An unknown key id must not be answered with a fetch on every request; the
// refresh interval in the verifier is what keeps that from becoming an outage
// at the provider.
func TestVerifyRejectsAnUnknownSigningKey(t *testing.T) {
	key, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	if _, err := v.Verify(context.Background(), signToken(t, key, srv.URL, "ddns", time.Hour, "no-such-key")); err == nil {
		t.Fatal("a token signed by an unknown key must be rejected")
	}
}

// Providers that do not issue readable access tokens are still usable: the
// claims are fetched from the userinfo endpoint instead.
func TestVerifyFallsBackToUserinfoForAnOpaqueToken(t *testing.T) {
	_, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	id, err := v.Verify(context.Background(), "opaque-access-token-0123456789abcdef")
	if err != nil {
		t.Fatalf("an opaque token must be resolved through userinfo: %v", err)
	}
	if id.Subject != "sub-2" || id.Username != "li" || len(id.Groups) != 1 || id.Groups[0] != "ops" {
		t.Fatalf("identity = %+v, want sub-2 / li / [ops]", id)
	}
	if !v.KnownOpaque("Bearer opaque-access-token-0123456789abcdef") {
		t.Fatal("a token just verified through userinfo must be known to the cache")
	}
	if v.KnownOpaque("Bearer another-opaque-token-0123456789") {
		t.Fatal("an unrelated token must not be known")
	}

	// Off by default: a userinfo response proves the token belongs to some user
	// of this issuer, not that it was issued to this client.
	v.cfg.OpaqueTokens = false
	if _, err := v.Verify(context.Background(), "opaque-access-token-0123456789abcdef"); err == nil {
		t.Error("an opaque token must be refused when the fallback is disabled")
	}
}

// An issuer that is neither https nor loopback hands the choice of verification
// keys to whoever is on the network path. The refusal has to hold on the request
// path too: discovery runs again on the first token, and that is where such an
// issuer would otherwise come back - carrying whatever jwks_uri the path chose.
func TestUntrustedIssuerIsRefused(t *testing.T) {
	v := New(config.OIDCConfig{
		Issuer:   "http://idp.example.com",
		ClientID: "ddns",
		Audience: "ddns",
	})
	if ready, _ := v.state(); ready {
		t.Fatal("an http issuer off this host must never become ready")
	}

	_, err := v.Verify(context.Background(), "Bearer a.b.c")
	if err == nil || !strings.Contains(err.Error(), "misconfigured") {
		t.Fatalf("Verify = %v, want the permanent configuration refusal", err)
	}
	if ready, _ := v.state(); ready {
		t.Fatal("a refused issuer must never become ready, not even after a request")
	}

	if err := requireTrustedURL("jwks_uri", "http://idp.example.com/jwks"); err == nil {
		t.Error("an http jwks_uri off this host must be refused")
	}
	if err := requireTrustedURL("jwks_uri", "https://idp.example.com/jwks"); err != nil {
		t.Errorf("https must be accepted: %v", err)
	}
	if err := requireTrustedURL("jwks_uri", "http://127.0.0.1:9443/jwks"); err != nil {
		t.Errorf("loopback http is a development issuer and must be accepted: %v", err)
	}
}

// The userinfo fallback is an outbound request this server makes for whoever
// sends the header, so a string too short to be a token is refused without it.
// The issuer here answers any token, so being rejected is the proof that no
// request left the process.
func TestVerifyRefusesATooShortTokenWithoutCallingTheProvider(t *testing.T) {
	_, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")

	if _, err := v.Verify(context.Background(), "x"); err == nil {
		t.Fatal("a one-character token must be rejected")
	}
}

func TestVerifyRejectsAnEmptyToken(t *testing.T) {
	v := newTestVerifier("https://issuer.example.com", "ddns")

	if _, err := v.Verify(context.Background(), "   "); err == nil {
		t.Fatal("no token at all must be rejected")
	}
}

// Providers disagree on which claim carries the name, so the chain of fallbacks
// is what makes a deployment work without touching the configuration.
func TestIdentityFromFallsBackThroughTheClaims(t *testing.T) {
	cases := []struct {
		name       string
		claims     map[string]any
		wantUser   string
		wantGroups []string
	}{
		{
			name:       "the configured claim",
			claims:     map[string]any{"sub": "s", "preferred_username": "zhang", "groups": []any{"a"}},
			wantUser:   "zhang",
			wantGroups: []string{"a"},
		},
		{
			name:     "name when preferred_username is missing",
			claims:   map[string]any{"sub": "s", "name": "Zhang San"},
			wantUser: "Zhang San",
		},
		{
			name:     "email as the last resort before the subject",
			claims:   map[string]any{"sub": "s", "email": "zhang@example.com"},
			wantUser: "zhang@example.com",
		},
		{
			name:     "the subject when nothing else is there",
			claims:   map[string]any{"sub": "s"},
			wantUser: "s",
		},
		{
			// Authelia sends an array, but a single string is a legal way to
			// carry one group
			name:       "a single group as a plain string",
			claims:     map[string]any{"sub": "s", "groups": "admins"},
			wantUser:   "s",
			wantGroups: []string{"admins"},
		},
	}

	v := &Verifier{cfg: config.OIDCConfig{UsernameClaim: "preferred_username", GroupsClaim: "groups"}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, err := v.identityFrom(c.claims)
			if err != nil {
				t.Fatalf("identityFrom: %v", err)
			}
			if id.Username != c.wantUser {
				t.Errorf("username = %q, want %q", id.Username, c.wantUser)
			}
			if len(id.Groups) != len(c.wantGroups) {
				t.Fatalf("groups = %v, want %v", id.Groups, c.wantGroups)
			}
			for i, g := range c.wantGroups {
				if id.Groups[i] != g {
					t.Errorf("groups[%d] = %q, want %q", i, id.Groups[i], g)
				}
			}
		})
	}
}

// Without a subject there is no identity to speak of, whatever else the token says.
func TestIdentityFromRequiresASubject(t *testing.T) {
	v := &Verifier{cfg: config.OIDCConfig{}}
	if _, err := v.identityFrom(map[string]any{"preferred_username": "zhang"}); err == nil {
		t.Fatal("a token without a subject must be rejected")
	}
}

// Eight requests that find an unready verifier must produce one discovery, not
// eight: repeating the round trips is both a queue in front of this process and
// an amplifier pointed at the provider.
func TestVerifyDiscoversOnceUnderConcurrency(t *testing.T) {
	key, srv := newIssuer(t)
	v := newTestVerifier(srv.URL, "ddns")
	token := signToken(t, key, srv.URL, "ddns", time.Hour, signingKeyID)

	// Back to unready, which is the state the single flight exists for: the
	// eight calls below are the ones that used to each run their own discovery.
	v.mu.Lock()
	v.ready = false
	v.mu.Unlock()
	discoveryHits.Store(0)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := v.Verify(context.Background(), token); err != nil {
				t.Errorf("verify: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := discoveryHits.Load(); got != 1 {
		t.Fatalf("want one discovery for 8 concurrent requests, got %d", got)
	}
}

// A provider that is down must not be probed once per request: the failure is
// remembered for the retry window, and the attempt after the window is real.
func TestVerifyRemembersAFailedDiscovery(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	previous := setupRetryInterval
	setupRetryInterval = time.Hour
	t.Cleanup(func() { setupRetryInterval = previous })

	v := newTestVerifier(srv.URL, "ddns")
	// New attempts discovery once while it is being built; that attempt is not
	// the one this test is about, and its failure is not the remembered one.
	hits.Store(0)
	token := "opaque-access-token-0123456789abcdef"

	for i := 0; i < 2; i++ {
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("a provider answering 500 must fail the request")
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("want the failure remembered, got %d discovery calls", got)
	}

	setupRetryInterval = 0
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("still failing")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("want a retry once the window has passed, got %d discovery calls", got)
	}
}

// The trust check on the initial endpoint is not enough: a redirect arrives
// later and can point somewhere else. The client used for discovery, JWKS and
// userinfo must apply the same rule to every hop.
func TestOIDCClientRejectsAnUntrustedRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://untrusted.example/jwks", http.StatusFound)
	}))
	defer srv.Close()

	v := &Verifier{client: &http.Client{Timeout: time.Second, CheckRedirect: checkRedirect}}
	var out map[string]any
	err := v.fetchJSON(context.Background(), srv.URL, &out)
	if err == nil {
		t.Fatal("a redirect to an http non-loopback host must be refused")
	}
	if !strings.Contains(err.Error(), "must be https") {
		t.Fatalf("redirect error = %v, want the trusted-URL refusal", err)
	}
}
