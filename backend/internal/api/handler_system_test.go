package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"ddns-by-dnspod/internal/config"
)

// The bundle reads this document for everything it cannot know at build time:
// where to call, whether a token is needed, and what this deployment does at
// all. A capability missing from here is a page offering something the API will
// refuse.
func TestFrontendConfigReportsTheCapabilities(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/config.json", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the answer: %v (%s)", err, rec.Body.String())
	}

	// The document itself, not the usual {"data": ...} envelope: the bundle
	// reads it as a plain document, and a static config.json has that shape.
	if _, wrapped := body["data"]; wrapped {
		t.Fatalf("the answer is wrapped, which the bundle does not expect: %s", rec.Body.String())
	}
	for _, key := range []string{"appName", "apiBaseUrl", "authRequired", "acmeEnabled", "oidc"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("%q is missing from the answer: %s", key, rec.Body.String())
		}
	}

	// Verification is off in this fixture, which is what AUTH_DISABLED looks
	// like, and issuing is off until a deployment asks for it.
	if authRequired, _ := body["authRequired"].(bool); authRequired {
		t.Fatalf("authRequired is true with verification off: %s", rec.Body.String())
	}
	if enabled, _ := body["acmeEnabled"].(bool); enabled {
		t.Fatalf("acmeEnabled is true with the manager off: %s", rec.Body.String())
	}
}

func TestFrontendConfigReportsTheCertificateManager(t *testing.T) {
	srv, cfg := newTestServer(t, func(cfg *config.Config) { cfg.ACME.Enabled = true })

	rec := do(t, srv, cfg, http.MethodGet, "/config.json", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode the answer: %v (%s)", err, rec.Body.String())
	}
	if enabled, ok := body["acmeEnabled"].(bool); !ok || !enabled {
		t.Fatalf("acmeEnabled = %v, want true", body["acmeEnabled"])
	}
}
