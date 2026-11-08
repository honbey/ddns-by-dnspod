package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/acme"
	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/auth"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ddns"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// newTestServer wires up a server the same way cmd/server does, backed by a
// throwaway sqlite file. Verification stays off, so requests run as the dev
// subject without a provider, which is what AUTH_DISABLED=true does in a real
// deployment.
//
// The options run before the server is built, because the dependencies copy
// what they need at construction: the ACME settings, for instance, are read
// into the certificate manager and cannot be changed afterwards.
func newTestServer(t *testing.T, options ...func(*config.Config)) (*Server, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		// In memory rather than a file, and that is what keeps this suite
		// quick: an empty database costs the first migration of nine tables,
		// and on a file that is ~1.2s of fsync-bound DDL - about a second per
		// test, which is where the whole 131s of this package went. The same
		// migration in memory is ~14ms. It is safe because the store pins
		// itself to one connection, so the schema lives as long as the test,
		// and no test here reopens a database it wrote: one that needs to has
		// to name a path of its own.
		Database: config.DatabaseConfig{Path: ":memory:"},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(dir, "secret.key")},
		API:      config.APIConfig{Enabled: true},
		OIDC:     config.OIDCConfig{Disabled: true, DisabledSub: "dev-user"},
		// A real root, so the output path the API reports is a path and not an
		// empty string. Issuing stays off: nothing here reaches a CA.
		ACME: config.ACMEConfig{OutputDir: filepath.Join(dir, "certs")},
	}
	for _, option := range options {
		option(cfg)
	}

	st, err := store.New(cfg)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var v4json []ipdetect.Endpoint
	for _, e := range cfg.IPDetect.V4JSON {
		v4json = append(v4json, ipdetect.Endpoint{URL: e.URL, Path: e.Path})
	}
	var v6json []ipdetect.Endpoint
	for _, e := range cfg.IPDetect.V6JSON {
		v6json = append(v6json, ipdetect.Endpoint{URL: e.URL, Path: e.Path})
	}
	detector := ipdetect.NewWithJSON(cfg.IPDetect.V4, cfg.IPDetect.V6, v4json, v6json, cfg.IPDetect.Timeout)
	sender := notifier.New(st.Encryptor(), notifier.Config{})
	executor := ddns.NewExecutor(st, detector, sender, cfg.DNSPod, cfg.Scheduler, cfg.Cache)
	scheduler := ddns.NewScheduler(st, executor, false)

	certManager := acme.NewManager(st, cfg.ACME, cfg.DNSPod, sender)

	return NewServer(cfg, st, scheduler, detector, sender, auth.New(cfg.OIDC), certManager), cfg
}

// do performs one request against the real route table
func do(t *testing.T, srv *Server, cfg *config.Config, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// The body cap is part of the real chain in main, so it is installed here
	// too: an oversized payload has to be a 413 rather than a 400, and that
	// mapping is only reachable with the limit in place.
	r.Use(middleware.MaxBody(middleware.DefaultMaxBody))
	srv.RegisterRoutes(r, *cfg)

	var req *http.Request
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// errorMessage reads the error field out of a JSON response
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

// errorCode returns "" when the server sent no code.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return body.Code
}

// newTestAccount inserts an account with a properly encrypted secret.
// A plaintext secret would fail to decrypt, and everything that builds a
// DNSPod client decrypts the secret first.
func newTestAccount(t *testing.T, srv *Server) model.Account {
	t.Helper()
	encrypted, err := srv.store.Encryptor().Encrypt("test-secret-key")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	account := model.Account{
		Name:      "acc",
		SecretID:  "test-secret-id",
		SecretKey: encrypted,
		KeyHint:   "test",
	}
	if err := srv.store.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	return account
}

// newTestTask inserts a task so the delete path has something real to remove
func newTestTask(t *testing.T, srv *Server) model.DDNSTask {
	t.Helper()
	account := newTestAccount(t, srv)
	task := model.DDNSTask{
		AccountID:  account.ID,
		Domain:     "example.com",
		SubDomain:  "a",
		RecordType: "A",
	}
	if err := srv.store.DB().Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

// A delete of a row that is not there used to answer 200 {"deleted": <id>},
// because GORM reports no error when nothing was deleted. These tests pin the
// behaviour of every local resource that can be deleted.
func TestDeleteMissingTaskReturns404(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/ddns/tasks/999999", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", rec.Code, rec.Body.String())
	}
	if msg := errorMessage(t, rec); msg != "task not found" {
		t.Fatalf("want task not found, got %q", msg)
	}
}

func TestDeleteExistingTaskSucceedsThenReturns404(t *testing.T) {
	srv, cfg := newTestServer(t)
	task := newTestTask(t, srv)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/ddns/tasks/"+itoa(task.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("first delete: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// The row is really gone, not just reported as deleted
	var left int64
	if err := srv.store.DB().Model(&model.DDNSTask{}).Count(&left).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if left != 0 {
		t.Fatalf("want 0 tasks left, got %d", left)
	}

	rec = do(t, srv, cfg, http.MethodDelete, "/api/ddns/tasks/"+itoa(task.ID), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: want 404, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteMissingChannelReturns404(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/notify/channels/999999", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", rec.Code, rec.Body.String())
	}
	if msg := errorMessage(t, rec); msg != "channel not found" {
		t.Fatalf("want channel not found, got %q", msg)
	}
}

func TestDeleteMissingAccountReturns404(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/accounts/999999", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", rec.Code, rec.Body.String())
	}
	if msg := errorMessage(t, rec); msg != "account not found" {
		t.Fatalf("want account not found, got %q", msg)
	}
}

// An account still referenced by a task must be refused, and that check must
// keep winning over the plain delete.
func TestDeleteAccountStillReferencedReturns400(t *testing.T) {
	srv, cfg := newTestServer(t)
	task := newTestTask(t, srv)

	rec := do(t, srv, cfg, http.MethodDelete, "/api/accounts/"+itoa(task.AccountID), nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	var left int64
	if err := srv.store.DB().Model(&model.Account{}).Count(&left).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if left != 1 {
		t.Fatalf("the referenced account must survive, got %d accounts", left)
	}
}

func TestDeleteUnreferencedAccountSucceeds(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := model.Account{Name: "spare", SecretID: "id", SecretKey: "key"}
	if err := srv.store.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	rec := do(t, srv, cfg, http.MethodDelete, "/api/accounts/"+itoa(account.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
