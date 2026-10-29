package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

// certificatePayload is what the form sends for a new certificate.
func certificatePayload(accountID uint) map[string]interface{} {
	return map[string]interface{}{
		"name":       "site",
		"account_id": accountID,
		"domains":    []string{"example.com", "www.example.com"},
		"key_type":   "ec256",
	}
}

func decodeCertificate(t *testing.T, rec *httptest.ResponseRecorder) certificateView {
	t.Helper()
	var body struct {
		Data certificateView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	return body.Data
}

func countCertificates(t *testing.T, srv *Server) int64 {
	t.Helper()
	var count int64
	if err := srv.store.DB().Model(&model.Certificate{}).Count(&count).Error; err != nil {
		t.Fatalf("count certificates: %v", err)
	}
	return count
}

func TestCreateCertificate(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	view := decodeCertificate(t, rec)
	if view.Name != "site" || view.ID == 0 {
		t.Fatalf("created %+v, want a stored certificate named site", view)
	}
	if len(view.Domains) != 2 || view.Domains[0] != "example.com" {
		t.Fatalf("domains = %v, want the two that were sent", view.Domains)
	}
	// Nothing has been issued yet, so there is no expiry to show and no run to
	// report - and the defaults are the ones that keep a certificate renewed
	// without anybody watching it.
	if view.NotAfter != nil || view.LastStatus != "" {
		t.Fatalf("a new certificate should be unissued, got %+v", view)
	}
	if !view.AutoRenew || !view.NotifyOnIssue {
		t.Fatalf("renewal and notification should default on, got %+v", view)
	}
	if want := filepath.Join(cfg.ACME.OutputDir, "site"); view.OutputPath != want {
		t.Fatalf("output path = %q, want %q", view.OutputPath, want)
	}
}

// The names are stored as the form's list, lowercased and deduplicated: the
// order is kept, because the first name is the subject of the certificate.
func TestCreateCertificateNormalisesTheNames(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	payload := certificatePayload(account.ID)
	payload["domains"] = []string{"EXAMPLE.com", "*.Example.com", "example.com", " www.example.com "}

	rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	view := decodeCertificate(t, rec)
	want := []string{"example.com", "*.example.com", "www.example.com"}
	if len(view.Domains) != len(want) {
		t.Fatalf("domains = %v, want %v", view.Domains, want)
	}
	for i := range want {
		if view.Domains[i] != want[i] {
			t.Fatalf("domains = %v, want %v", view.Domains, want)
		}
	}
}

// Everything the form cannot be trusted for. A stored certificate that cannot
// be issued is worse than a rejected request: it fails every day and announces
// it every day.
func TestCreateCertificateRejectsBadInput(t *testing.T) {
	manyNames := make([]string, 0, maxCertificateNames+1)
	for i := 0; i <= maxCertificateNames; i++ {
		manyNames = append(manyNames, fmt.Sprintf("host%d.example.com", i))
	}

	cases := []struct {
		name   string
		change func(payload map[string]interface{})
	}{
		{"no name", func(payload map[string]interface{}) { payload["name"] = "" }},
		{"a name with a slash", func(payload map[string]interface{}) { payload["name"] = "a/b" }},
		{"a name that walks up", func(payload map[string]interface{}) { payload["name"] = ".." }},
		{"no domains", func(payload map[string]interface{}) { payload["domains"] = []string{} }},
		{"a blank domain", func(payload map[string]interface{}) { payload["domains"] = []string{"  "} }},
		{"a domain that is not one", func(payload map[string]interface{}) { payload["domains"] = []string{"not a domain"} }},
		{"a single label", func(payload map[string]interface{}) { payload["domains"] = []string{"localhost"} }},
		{"too many domains", func(payload map[string]interface{}) { payload["domains"] = manyNames }},
		{"an unknown key type", func(payload map[string]interface{}) { payload["key_type"] = "ec521" }},
		{"an absolute directory", func(payload map[string]interface{}) { payload["output_dir"] = "/etc/certs" }},
		{"a directory that walks up", func(payload map[string]interface{}) { payload["output_dir"] = "../escape" }},
		{"a directory that walks up from below", func(payload map[string]interface{}) { payload["output_dir"] = "a/../../escape" }},
		{"the root itself", func(payload map[string]interface{}) { payload["output_dir"] = "." }},
		{"an account that does not exist", func(payload map[string]interface{}) { payload["account_id"] = 4242 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A server per case, so a rejected request cannot leave anything
			// behind for the next one to trip over.
			srv, cfg := newTestServer(t)
			account := newTestAccount(t, srv)

			payload := certificatePayload(account.ID)
			tc.change(payload)

			rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
			if count := countCertificates(t, srv); count != 0 {
				t.Fatalf("%d certificate(s) were stored despite the rejection", count)
			}
		})
	}
}

func TestCreateCertificateRejectsADuplicateName(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)); rec.Code != http.StatusCreated {
		t.Fatalf("want 201 for the first one, got %d (%s)", rec.Code, rec.Body.String())
	}

	payload := certificatePayload(account.ID)
	payload["domains"] = []string{"other.com"}
	rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload)
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestUpdateCertificate(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)
	created := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)))

	rec := do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(created.ID), map[string]interface{}{
		"name":       "renamed",
		"domains":    []string{"example.org"},
		"key_type":   "rsa2048",
		"output_dir": filepath.Join("shared", "tls"),
		"auto_renew": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	view := decodeCertificate(t, rec)
	if view.Name != "renamed" || view.AutoRenew {
		t.Fatalf("update did not land: %+v", view)
	}
	if view.KeyType != "rsa2048" || len(view.Domains) != 1 || view.Domains[0] != "example.org" {
		t.Fatalf("update did not land: %+v", view)
	}
	if want := filepath.Join(cfg.ACME.OutputDir, "shared", "tls"); view.OutputPath != want {
		t.Fatalf("output path = %q, want %q", view.OutputPath, want)
	}

	// An empty directory is a value the caller has to be able to send: it means
	// back to a directory named after the certificate.
	rec = do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(created.ID), map[string]interface{}{
		"output_dir": "",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeCertificate(t, rec).OutputPath; got != filepath.Join(cfg.ACME.OutputDir, "renamed") {
		t.Fatalf("output path = %q, want it back at the certificate name", got)
	}

	// A field that is not sent is left alone, which is what an edit of one
	// setting from a form that carries the others needs.
	rec = do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(created.ID), map[string]interface{}{
		"auto_renew": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	view = decodeCertificate(t, rec)
	if !view.AutoRenew || view.KeyType != "rsa2048" || len(view.Domains) != 1 {
		t.Fatalf("an untouched field changed: %+v", view)
	}
}

// Deleting the record is not the same as knowing nobody wants the files: they
// are what the TLS application is configured with, so they stay.
func TestDeleteCertificateLeavesTheFiles(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)
	created := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)))

	if err := os.MkdirAll(created.OutputPath, 0o755); err != nil {
		t.Fatalf("create the output directory: %v", err)
	}
	certPath := filepath.Join(created.OutputPath, "fullchain.pem")
	if err := os.WriteFile(certPath, []byte("a certificate"), 0o644); err != nil {
		t.Fatalf("write the certificate: %v", err)
	}

	rec := do(t, srv, cfg, http.MethodDelete, "/api/certificates/"+itoa(created.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if count := countCertificates(t, srv); count != 0 {
		t.Fatalf("%d certificate(s) left in the table", count)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("the files should have been left alone: %v", err)
	}

	rec = do(t, srv, cfg, http.MethodDelete, "/api/certificates/"+itoa(created.ID), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 for the second delete, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// An account a certificate answers its challenges with cannot be deleted out
// from under it: the certificate would keep failing on every run and announce
// it every day, so the delete is refused with something to do about it.
func TestDeleteAccountWithACertificate(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)
	created := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)))

	rec := do(t, srv, cfg, http.MethodDelete, "/api/accounts/"+itoa(account.ID), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "certificates") {
		t.Fatalf("the refusal should name what still references it: %s", rec.Body.String())
	}

	// Once the certificate is gone, so is the reason to refuse.
	if rec := do(t, srv, cfg, http.MethodDelete, "/api/certificates/"+itoa(created.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("delete the certificate: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(t, srv, cfg, http.MethodDelete, "/api/accounts/"+itoa(account.ID), nil); rec.Code != http.StatusOK {
		t.Fatalf("want the account to be deletable, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestCertificateListIsEmptyAtFirst(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/certificates", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	body := decodeList[certificateView](t, rec.Body.Bytes())
	// Empty rather than null: the table maps over it before it has a reason to
	// check.
	if body == nil || len(body) != 0 {
		t.Fatalf("list = %v, want an empty array", body)
	}
}

// Issuing is refused when the manager is off, rather than started and failing
// in the background where nobody asked for it.
func TestIssueCertificateWhenTheManagerIsDisabled(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)
	created := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)))

	rec := do(t, srv, cfg, http.MethodPost, "/api/certificates/"+itoa(created.ID)+"/issue", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Nothing was written either: a refused request must not leave a row
	// claiming a run that never started.
	var stored model.Certificate
	if err := srv.store.DB().First(&stored, created.ID).Error; err != nil {
		t.Fatalf("reload the certificate: %v", err)
	}
	if stored.LastStatus != "" {
		t.Fatalf("the refused issuance wrote status %q", stored.LastStatus)
	}
}

// A run answers 202 and keeps going, so the row has to say RUNNING by the time
// the caller reads it back - that status is the only thing there is to watch.
func TestIssueCertificateStartsARun(t *testing.T) {
	// Held until the test ends: a run that fails in microseconds (this stub
	// answers nothing useful) would otherwise race the read-back and answer
	// FAIL some of the time, which is what this test is here to pin. The
	// timeout is set for the same reason: the fixture builds the config
	// directly, where it has no default, and a zero timeout ends a run before
	// it starts.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"newNonce":"","newAccount":""}`))
	}))
	t.Cleanup(endpoint.Close)

	srv, cfg := newTestServer(t, func(cfg *config.Config) {
		cfg.ACME.Enabled = true
		cfg.ACME.Directory = endpoint.URL
		cfg.ACME.Timeout = 60
		cfg.DNSPod.Endpoint = endpoint.URL
		cfg.DNSPod.Timeout = 2
	})
	account := newTestAccount(t, srv)
	created := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)))

	rec := do(t, srv, cfg, http.MethodPost, "/api/certificates/"+itoa(created.ID)+"/issue", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	if status := decodeCertificate(t, rec).LastStatus; status != model.CertStatusRUNNING {
		t.Fatalf("status = %q, want %q", status, model.CertStatusRUNNING)
	}
}

// Two certificates writing to the same directory would silently overwrite each
// other's files: the later issuance replaces the earlier one's certificate and
// key with a pair the earlier row's names say nothing about, and both rows keep
// looking fine in the list. The check is on resolved paths, because an empty
// directory means named after the certificate - which is how two rows end up
// sharing one without either of them naming it.
func TestCertificateDirectoryConflicts(t *testing.T) {
	srv, cfg := newTestServer(t)
	account := newTestAccount(t, srv)

	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", certificatePayload(account.ID)); rec.Code != http.StatusCreated {
		t.Fatalf("create the first: %d (%s)", rec.Code, rec.Body.String())
	}

	// An explicit directory another certificate is already using
	payload := certificatePayload(account.ID)
	payload["name"] = "other"
	payload["output_dir"] = "site"
	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload); rec.Code != http.StatusConflict {
		t.Fatalf("want 409 for a taken directory, got %d (%s)", rec.Code, rec.Body.String())
	}

	// The mirror of it: the default directory, named after the certificate,
	// landing on a directory another certificate claimed explicitly
	payload = certificatePayload(account.ID)
	payload["name"] = "other2"
	payload["output_dir"] = "site2"
	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload); rec.Code != http.StatusCreated {
		t.Fatalf("create other2: %d (%s)", rec.Code, rec.Body.String())
	}
	payload = certificatePayload(account.ID)
	payload["name"] = "site2"
	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload); rec.Code != http.StatusConflict {
		t.Fatalf("want 409 for a name that walks into a taken directory, got %d (%s)",
			rec.Code, rec.Body.String())
	}

	// The update path, both directions
	listed := decodeList[certificateView](t, do(t, srv, cfg, http.MethodGet, "/api/certificates", nil).Body.Bytes())
	var other2ID, siteID uint
	for _, view := range listed {
		switch view.Name {
		case "other2":
			other2ID = view.ID
		case "site":
			siteID = view.ID
		}
	}
	if other2ID == 0 || siteID == 0 {
		t.Fatalf("expected site and other2 in the list, got %+v", listed)
	}

	rec := do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(other2ID),
		map[string]interface{}{"output_dir": "site"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("update into a taken directory: want 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A certificate keeping its own directory is not a conflict
	rec = do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(other2ID),
		map[string]interface{}{"output_dir": "site2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("keeping its own directory: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// And once the first certificate is gone, its directory is up for grabs
	if rec := do(t, srv, cfg, http.MethodDelete, "/api/certificates/"+itoa(siteID), nil); rec.Code != http.StatusOK {
		t.Fatalf("delete the first: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(other2ID),
		map[string]interface{}{"output_dir": "site"})
	if rec.Code != http.StatusOK {
		t.Fatalf("the directory should be free now: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// The rename branch: the new name is free as a name, but its default
	// directory is one another certificate claimed explicitly.
	payload = certificatePayload(account.ID)
	payload["name"] = "beta"
	payload["output_dir"] = "taken-dir"
	if rec := do(t, srv, cfg, http.MethodPost, "/api/certificates", payload); rec.Code != http.StatusCreated {
		t.Fatalf("create beta: %d (%s)", rec.Code, rec.Body.String())
	}
	payload = certificatePayload(account.ID)
	payload["name"] = "alpha"
	alpha := decodeCertificate(t, do(t, srv, cfg, http.MethodPost, "/api/certificates", payload))
	rec = do(t, srv, cfg, http.MethodPut, "/api/certificates/"+itoa(alpha.ID),
		map[string]interface{}{"name": "taken-dir"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("renaming into a taken directory: want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
}
