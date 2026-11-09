package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The Bearer header rides along automatically, so the whole UI is one iframe
// away from being driven by a page the user never meant to trust. These headers
// are the only thing that refuses to be framed, and they have to be on every
// answer, including the error ones.
func TestSecureHeadersRefusesFramingAndSniffing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecureHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(204) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	h := rec.Header()
	if h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", h.Get("X-Frame-Options"))
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", h.Get("X-Content-Type-Options"))
	}
	csp := h.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP does not forbid framing: %q", csp)
	}
	if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP does not pin scripts to self: %q", csp)
	}
}

// A self-only policy breaks the sign-in it is not meant to touch: the silent
// renew runs in a provider iframe, and the token exchange and revocation are
// calls from the page to the provider. The provider's origin has to be let
// through connect-src and frame-src, or the first expired token is the last one
// the page ever holds.
func TestSecureHeadersLetsTheProviderThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecureHeaders("https://auth.example.com"))
	r.GET("/", func(c *gin.Context) { c.Status(204) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https://auth.example.com") {
		t.Errorf("the provider is not in connect-src: %q", csp)
	}
	if !strings.Contains(csp, "frame-src 'self' https://auth.example.com") {
		t.Errorf("the provider is not in frame-src: %q", csp)
	}
}

// HSTS is a browser instruction that outlives the response, so it is opt-in:
// sent on a deployment that cannot answer TLS, it locks users out of the host.
func TestHSTSIsOptIn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, enabled := range []bool{true, false} {
		r := gin.New()
		r.Use(HSTS(enabled))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		got := rec.Header().Get("Strict-Transport-Security")
		if enabled && got == "" {
			t.Fatal("hsts=true sends no Strict-Transport-Security")
		}
		if !enabled && got != "" {
			t.Fatalf("hsts=false sent %q", got)
		}
	}
}

// The frame and referrer policies are not optional: the bearer header is
// attached by the browser, so a click inside an iframe is a click with a token.
func TestSecureHeadersDenyFraming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SecureHeaders("https://idp.example.com"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	h := rec.Header()

	for header, want := range map[string]string{
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := h.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if policy := h.Get("Content-Security-Policy"); !strings.Contains(policy, "frame-ancestors 'none'") {
		t.Errorf("CSP has no frame-ancestors: %q", policy)
	}
	if policy := h.Get("Content-Security-Policy"); !strings.Contains(policy, "https://idp.example.com") {
		t.Errorf("CSP does not let the provider through: %q", policy)
	}
}
