package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

// newAPIKeyRow stores a key for the given plaintext and returns the row
func newAPIKeyRow(t *testing.T, srv *Server, plaintext, domain string, accountID uint, enabled bool) *model.APIKey {
	t.Helper()
	key := model.APIKey{
		Name:      "test",
		KeyHash:   middleware.HashAPIKey(plaintext),
		KeyPrefix: middleware.DisplayPrefix(plaintext),
		Domain:    domain,
		AccountID: accountID,
		Enabled:   enabled,
	}
	if err := srv.store.DB().Create(&key).Error; err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return &key
}

// v1Request calls the public API the way an external script would
func v1Request(t *testing.T, srv *Server, cfg *config.Config, method, path, key string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv.RegisterRoutes(r, *cfg)

	req := httptest.NewRequest(method, path, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// The public API must be reachable with a key and closed without one.
//
// The test database has no account, so any call that gets past auth stops at
// clientFor instead of reaching DNSPod. That keeps these tests offline while
// still proving which requests made it through authentication.
func TestPublicAPIRequiresAKey(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/example.com/records", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a key, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing API key") {
		t.Fatalf("body = %s, want the missing key message", rec.Body.String())
	}
}

func TestPublicAPIRejectsAnUnknownKey(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/example.com/records", "ddp_"+strings.Repeat("0", 64))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for an unknown key, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPublicAPIRejectsADisabledKey(t *testing.T) {
	srv, cfg := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_disabled", "example.com", 1, false)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/example.com/records", "ddp_disabled")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for a disabled key, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("body = %s, want the disabled message", rec.Body.String())
	}
}

// This is the important one: a key bound to one domain must not touch another
// domain of the same account.
func TestAPIKeyCannotReachAnotherDomain(t *testing.T) {
	srv, cfg := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_key", "example.com", 1, true)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/other.com/records", "ddp_key")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 when the domain does not match, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not scoped to this domain") {
		t.Fatalf("body = %s, want the forbidden message", rec.Body.String())
	}
}

// A key issued with the wildcard scope covers every domain of its account: that
// is what the "every domain" option in the UI creates, and the whole point of
// the scope is that the domains need not be known when the key is minted.
func TestAPIKeyWildcardReachesEveryDomain(t *testing.T) {
	srv, cfg := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_key", model.APIKeyWildcard, 1, true)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/other.com/records", "ddp_key")

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("a wildcard key must pass the domain check, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// A matching domain must get through auth, no matter the case
func TestAPIKeyDomainMatchIsCaseInsensitive(t *testing.T) {
	srv, cfg := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_key", "example.com", 1, true)

	rec := v1Request(t, srv, cfg, http.MethodGet,
		"/api/v1/domains/EXAMPLE.com/records", "ddp_key")

	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("a matching domain must pass auth, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAPIKeyAuthAcceptsXAPIKeyHeader(t *testing.T) {
	srv, cfg := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_key", "example.com", 1, true)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	srv.RegisterRoutes(r, *cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/example.com/records", nil)
	req.Header.Set("X-Api-Key", "ddp_key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("the X-Api-Key header must be accepted: %s", rec.Body.String())
	}
}

// The account of a key is fixed, a request must not be able to pick another one
func TestAPIKeyPinsTheAccount(t *testing.T) {
	srv, _ := newTestServer(t)
	key := newAPIKeyRow(t, srv, "ddp_key", "example.com", 7, true)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?account_id=42", nil)
	c.Set(middleware.ContextAPIKeyKey, key)

	got, valid := parseAccountID(c)
	if !valid {
		t.Fatal("parseAccountID must be usable for an API key call")
	}
	if got != 7 {
		t.Fatalf("parseAccountID = %d, want the account bound to the key (7), not the 42 from the query", got)
	}
}

func TestParseAccountIDStillHonoursTheQueryWithoutAKey(t *testing.T) {
	newTestServer(t)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?account_id=42", nil)

	got, valid := parseAccountID(c)
	if !valid {
		t.Fatal("parseAccountID must be usable for a numeric query")
	}
	if got != 42 {
		t.Fatalf("parseAccountID = %d, want 42 from the query", got)
	}
}

// Existing handlers log middleware.CurrentUser, so a key must show up there
func TestAPIKeyCallsAreVisibleInTheAuditIdentity(t *testing.T) {
	srv, _ := newTestServer(t)
	newAPIKeyRow(t, srv, "ddp_key", "example.com", 1, true)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		middleware.APIKeyAuth(srv.store.DB())(c)
		// Stand in for a handler: report what the logs would record
		c.String(http.StatusOK, middleware.CurrentUser(c))
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer ddp_key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "apikey:") {
		t.Fatalf("identity = %q, want it to name the api key", rec.Body.String())
	}
}

func TestAPIKeyUsageIsRecorded(t *testing.T) {
	srv, _ := newTestServer(t)
	key := newAPIKeyRow(t, srv, "ddp_key", "example.com", 1, true)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("Authorization", "Bearer ddp_key")
	middleware.APIKeyAuth(srv.store.DB())(c)

	var stored model.APIKey
	if err := srv.store.DB().First(&stored, key.ID).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if stored.LastUsedAt == nil {
		t.Fatal("a successful authentication must record last_used_at")
	}
}

// The management endpoints live behind the normal auth, and must never expose
// the secret. Only the creation call returns the plaintext.
func TestCreateAPIKeyReturnsThePlaintextOnce(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "home-assistant", "domain": "example.com", "account_id": account.ID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Key    string `json:"key"`
			APIKey struct {
				KeyHash   string `json:"key_hash"`
				KeyPrefix string `json:"key_prefix"`
				Name      string `json:"name"`
			} `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(body.Data.Key, "ddp_") {
		t.Fatalf("the plaintext key must be returned on creation, got %q", body.Data.Key)
	}
	if body.Data.APIKey.KeyHash != "" {
		t.Fatal("the hash must never be serialized")
	}
	if body.Data.APIKey.KeyPrefix == "" {
		t.Fatal("the display prefix must be returned")
	}

	// Listing the keys later must not bring the plaintext back
	rec = do(t, srv, cfg, http.MethodGet, "/api/keys", nil)
	if strings.Contains(rec.Body.String(), body.Data.Key) {
		t.Fatal("the plaintext leaked into the key list")
	}
}

func TestCreateAPIKeyRejectsAnUnknownAccount(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "x", "domain": "example.com", "account_id": 12345,
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// The scope is either a domain name or the wildcard. Both have to be storable:
// the row is what every later call is checked against.
func TestCreateAPIKeyAcceptsTheWildcardScope(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "everywhere", "domain": model.APIKeyWildcard, "account_id": account.ID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var stored model.APIKey
	if err := srv.store.DB().First(&stored).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if stored.Domain != model.APIKeyWildcard {
		t.Fatalf("stored domain = %q, want %q", stored.Domain, model.APIKeyWildcard)
	}
}

func TestDeleteMissingAPIKeyReturns404(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/keys/999999", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// Turning a key off must take effect on the public API immediately
func TestDisabledKeyLosesAccessImmediately(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	created := do(t, srv, cfg, http.MethodPost, "/api/keys", map[string]interface{}{
		"name": "k", "domain": "example.com", "account_id": account.ID,
	})
	var createdBody struct {
		Data struct {
			Key    string `json:"key"`
			APIKey struct {
				ID uint `json:"id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	plaintext := createdBody.Data.Key

	disable := do(t, srv, cfg, http.MethodPut, "/api/keys/"+itoa(createdBody.Data.APIKey.ID),
		map[string]interface{}{"enabled": false})
	if disable.Code != http.StatusOK {
		t.Fatalf("disable: want 200, got %d (%s)", disable.Code, disable.Body.String())
	}

	rec := v1Request(t, srv, cfg, http.MethodGet, "/api/v1/domains/example.com/records", plaintext)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled key must be refused, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// A key is bound to one domain - or to the wildcard, the one accepted value
// that is not a name - and every call is checked against it, so a domain that is
// not a domain produces a key that can never work, and it is compared and logged
// downstream.
func TestCreateAPIKeyRejectsAMalformedDomain(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	for _, domain := range []string{"example.com/", "exa mple.com", "example\n.com", "com"} {
		rec := do(t, srv, cfg, http.MethodPost, "/api/keys", map[string]interface{}{
			"name": "k", "domain": domain, "account_id": account.ID,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("domain %q: want 400, got %d (%s)", domain, rec.Code, rec.Body.String())
		}
	}

	var keys int64
	if err := srv.store.DB().Model(&model.APIKey{}).Count(&keys).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if keys != 0 {
		t.Fatalf("a refused key must not be stored, got %d", keys)
	}
}
