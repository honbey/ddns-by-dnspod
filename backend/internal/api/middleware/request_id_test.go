package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// runRequestID serves one request through the middleware and reports the id that
// was echoed back in the header and the one seen by the handler.
//
// The id is read inside the handler on purpose: ServeHTTP builds its own gin
// context from the pool, so a context created outside the engine never receives
// anything the middleware sets.
func runRequestID(upstream string) (headerID, handlerID string, status int) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if upstream != "" {
		req.Header.Set(RequestIDHeader, upstream)
	}

	r.Use(RequestID())
	r.GET("/api/me", func(ctx *gin.Context) {
		handlerID = RequestIDFrom(ctx)
		ctx.String(http.StatusOK, "ok")
	})
	r.ServeHTTP(rec, req)

	return rec.Header().Get(RequestIDHeader), handlerID, rec.Code
}

func TestRequestIDReusesValidUpstreamID(t *testing.T) {
	for _, upstream := range []string{"abc-123.XYZ_9", "550e8400-e29b-41d4-a716-446655440000"} {
		headerID, handlerID, status := runRequestID(upstream)
		if status != http.StatusOK {
			t.Fatalf("want 200, got %d", status)
		}
		for name, got := range map[string]string{"header": headerID, "handler": handlerID} {
			if got != upstream {
				t.Errorf("%s id = %q, want the upstream value %q", name, got, upstream)
			}
		}
	}
}

func TestRequestIDRejectsUnsafeUpstreamID(t *testing.T) {
	// Every one of these would corrupt the log file if echoed back verbatim
	unsafe := []string{
		"evil\n[api] forged line",
		"evil\r[api] forged line",
		"\x1b[31mred",
		"has space",
		"../../etc/passwd",
		"quote\"inside",
		strings.Repeat("a", maxRequestIDLen+1),
	}
	for _, raw := range unsafe {
		headerID, handlerID, _ := runRequestID(raw)
		if headerID == raw || handlerID == raw {
			t.Errorf("unsafe id %q was reused verbatim", raw)
		}
		if headerID != handlerID {
			t.Errorf("header id %q and handler id %q disagree", headerID, handlerID)
		}
		if !isUUID(headerID) {
			t.Errorf("want a freshly generated uuid for %q, got %q", raw, headerID)
		}
	}
}

func TestRequestIDGeneratesWithoutUpstream(t *testing.T) {
	headerID, handlerID, _ := runRequestID("")
	if !isUUID(headerID) {
		t.Fatalf("want a uuid, got %q", headerID)
	}
	if headerID != handlerID {
		t.Fatalf("ids disagree: header=%q handler=%q", headerID, handlerID)
	}
}

func TestNewRequestIDIsUnique(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := newRequestID()
		if !isUUID(id) {
			t.Fatalf("want a uuid, got %q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q after %d generations", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestSanitizeRequestID(t *testing.T) {
	cases := map[string]string{
		"simple":       "simple",
		"a.b_c-1":      "a.b_c-1",
		"":             "",
		"bad\ttab":     "",
		"new\nline":    "",
		"two words":    "",
		"uni\u00e7ode": "",
	}
	for raw, want := range cases {
		if got := sanitizeRequestID(raw); got != want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", raw, got, want)
		}
	}
	boundary := strings.Repeat("a", maxRequestIDLen)
	if got := sanitizeRequestID(boundary); got != boundary {
		t.Errorf("an id of exactly %d characters must be accepted", maxRequestIDLen)
	}
}

func TestRequestIDFromWithoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := RequestIDFrom(c); got != "" {
		t.Fatalf("want an empty id, got %q", got)
	}
	if got := RequestIDFrom(nil); got != "" {
		t.Fatalf("a nil context must be tolerated, got %q", got)
	}
}

// isUUID reports whether the id has the canonical uuid form
func isUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}
