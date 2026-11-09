package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
)

// A request authenticated with an API key may not pick its own account through
// the query string. The key is bound to one account, and letting the request
// override that binding would let a key reach another account's credentials.
// This test pins that behaviour so a future refactor cannot quietly drop it.
func TestParseAccountIDForcesAPIKeyAccount(t *testing.T) {
	key := &model.APIKey{AccountID: 7, Domain: "example.com"}

	cases := []struct {
		name    string
		query   string
		withKey bool
		want    uint
	}{
		// With a key the account_id query is ignored entirely.
		{"key overrides matching query", "?account_id=7", true, 7},
		{"key overrides different query", "?account_id=99", true, 7},
		{"key overrides absent query", "", true, 7},
		// Without a key the query decides, and an empty query means default.
		{"no key reads query", "?account_id=42", false, 42},
		{"no key empty query is default", "", false, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := httptest.NewRequest("GET", "/domains/example.com/records"+tc.query, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			if tc.withKey {
				c.Set(middleware.ContextAPIKeyKey, key)
			}

			got, valid := parseAccountID(c)
			if !valid {
				t.Fatalf("parseAccountID() valid = false, want true")
			}
			if got != tc.want {
				t.Errorf("parseAccountID() = %d, want %d", got, tc.want)
			}
		})
	}
}

// The handler must stop at the 400 instead of running on with the default
// account and writing a second response. Two JSON documents glued together would
// also fail to parse here, which is what the bug looked like from the client.
func TestInvalidAccountIDWritesOneResponse(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/domains?account_id=abc", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response must be a single JSON document, got %q: %v", rec.Body.String(), err)
	}
	if body.Error != "invalid account_id" {
		t.Fatalf("error = %q, want invalid account_id", body.Error)
	}
}

// An unusable account_id answers 400 itself and reports valid = false, so the
// handler returns before writing a second response on top of the first one.
func TestParseAccountIDRejectsInvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("GET", "/domains/example.com/records?account_id=abc", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	id, valid := parseAccountID(c)
	if valid {
		t.Error("parseAccountID() valid = true, want false")
	}
	if id != 0 {
		t.Errorf("parseAccountID() = %d, want 0", id)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("want the 400 to be written, got %d", w.Code)
	}
}
