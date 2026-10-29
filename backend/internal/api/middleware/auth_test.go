package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/auth"
)

// stubVerifier answers like a provider would, without one: the middleware is
// about what happens to the result, not about signatures.
type stubVerifier struct {
	enabled bool
	devSub  string // the subject impersonated while verification is off
	token   string // the only token accepted
}

func (s stubVerifier) Enabled() bool { return s.enabled }

func (s stubVerifier) DevSub() string { return s.devSub }

func (s stubVerifier) Verify(_ context.Context, raw string) (*auth.Identity, error) {
	// The real verifier takes the header as it arrives, prefix included
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if raw != s.token {
		return nil, errors.New("the token was not issued by the provider")
	}
	return &auth.Identity{Subject: "sub-1", Username: "zhang", Groups: []string{"admins"}}, nil
}

func newBearerEngine(v TokenVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID(), Bearer(v))
	r.GET("/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user":   CurrentUser(c),
			"groups": c.GetStringSlice(ContextGroupsKey),
		})
	})
	return r
}

func doWithAuth(r *gin.Engine, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// With verification off there is no provider to ask, and local development
// still gets a usable identity - the configured one rather than a fixed name.
func TestBearerDisabledUsesTheConfiguredIdentity(t *testing.T) {
	r := newBearerEngine(stubVerifier{enabled: false, devSub: "dev-user"})

	w := doWithAuth(r, "")
	if w.Code != http.StatusOK || w.Body.String() != `{"groups":[],"user":"dev-user"}` {
		t.Fatalf("want the dev subject, got %d %s", w.Code, w.Body.String())
	}
}

func TestBearerRejectsRequestWithoutToken(t *testing.T) {
	r := newBearerEngine(stubVerifier{enabled: true, token: "good"})

	w := doWithAuth(r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a token, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "request_id") {
		t.Fatalf("the response must carry the request id, got %s", w.Body.String())
	}
}

func TestBearerAcceptsValidToken(t *testing.T) {
	r := newBearerEngine(stubVerifier{enabled: true, token: "good"})

	w := doWithAuth(r, "Bearer good")
	if w.Code != http.StatusOK || w.Body.String() != `{"groups":["admins"],"user":"zhang"}` {
		t.Fatalf("want the token identity, got %d %s", w.Code, w.Body.String())
	}
}

// A rejected token must not reach the handler, and the reason must not be sent
// back: it would describe the provider's configuration.
func TestBearerRejectsUnknownToken(t *testing.T) {
	r := newBearerEngine(stubVerifier{enabled: true, token: "good"})

	w := doWithAuth(r, "Bearer stolen")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for an unknown token, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "was not issued") {
		t.Fatalf("the provider's reason must stay in the log: %s", w.Body.String())
	}
}

// The identity is printed with %s by a dozen audit lines, so a newline in it -
// from an API key name or from a display name the user edits - would forge one
// of those lines.
func TestCurrentUserFlattensTheIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ContextUserKey, "apikey:prod\n[api] account \"x\" created by admin\x1b[2J")

	got := CurrentUser(c)
	if strings.ContainsAny(got, "\n\r\x1b") {
		t.Fatalf("the identity can still break a log line: %q", got)
	}
	if !strings.Contains(got, "apikey:prod") {
		t.Fatalf("the identity was lost: %q", got)
	}
}

// The characters that could break a log line, one by one. The C1 block is legal
// UTF-8, so it arrives intact, and only this stands between it and the terminal
// a log viewer prints on.
func TestSanitizeLogValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "alice", "alice"},
		{"newline", "a\nb", "a b"},
		{"carriage return", "a\rb", "a b"},
		{"tab", "a\tb", "a b"},
		{"escape sequence", "a\x1bb[2J", "ab[2J"},
		{"nel", "a\u0085b", "ab"},
		{"csi", "a\u009bb", "ab"},
		{"line separator", "a\u2028b", "a b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogValue(tc.in); got != tc.want {
				t.Fatalf("sanitizeLogValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
