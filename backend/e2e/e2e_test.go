//go:build e2e

// Package e2e drives the built server over HTTP: the embedded SPA, the probes,
// the runtime configuration and the rejection budget. It runs the real binary
// against a temporary database, so what it checks is the process a deployment
// starts, not a handler wired by hand.
//
// Run from backend with:
//
//	GOPATH=~/.go go test -tags=e2e ./e2e/ -v
//
// The frontend must be built first (frontend npm run build) so the binary has
// assets to embed.
package e2e

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// TestProbesAndEmbeddedSPA covers what a deployment answers before anyone signs
// in: the bundle, the two probes, the runtime configuration and the fact that
// an unknown /api path is JSON rather than the page.
func TestProbesAndEmbeddedSPA(t *testing.T) {
	srv := startServer(t, serverOptions{})
	a := &api{t: t, base: srv.base, hc: &http.Client{Timeout: 15 * time.Second}}

	t.Run("serves the bundle and falls back to it", func(t *testing.T) {
		code, index := a.do(http.MethodGet, "/", nil)
		if code != http.StatusOK || !contains(index, `id="root"`) {
			t.Fatalf("GET / = %d, body %s", code, index)
		}
		// A client-side route is not a file; it must still answer with the app.
		if code, spa := a.do(http.MethodGet, "/accounts", nil); code != http.StatusOK || !contains(spa, `id="root"`) {
			t.Fatalf("GET /accounts = %d", code)
		}
		asset := regexp.MustCompile(`/assets/[^"]+\.js`).FindString(string(index))
		if asset == "" {
			t.Fatal("index has no JS asset reference")
		}
		if code, _ := a.do(http.MethodGet, asset, nil); code != http.StatusOK {
			t.Fatalf("GET %s = %d", asset, code)
		}
	})

	t.Run("answers both probes", func(t *testing.T) {
		code, raw := a.do(http.MethodGet, "/api/health", nil)
		if code != http.StatusOK {
			t.Fatalf("health = %d, body %s", code, raw)
		}
		var health struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		decodeEnvelope(t, raw, &health)
		if health.Status != "ok" {
			t.Errorf("health status = %q", health.Status)
		}
		if health.Version == "" {
			t.Error("health carries no version")
		}

		if code, raw := a.do(http.MethodGet, "/api/ready", nil); code != http.StatusOK {
			t.Fatalf("ready = %d, body %s", code, raw)
		}
	})

	t.Run("describes itself to the client", func(t *testing.T) {
		code, raw := a.do(http.MethodGet, "/config.json", nil)
		if code != http.StatusOK {
			t.Fatalf("config.json = %d", code)
		}
		var cfg struct {
			AppName      string `json:"appName"`
			AuthRequired bool   `json:"authRequired"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("decode config.json: %v (%s)", err, raw)
		}
		if cfg.AppName == "" {
			t.Error("config.json names no app")
		}
		// Verification is off for this instance, which is what the SPA uses to
		// decide whether to send the visitor to the IdP.
		if cfg.AuthRequired {
			t.Error("authRequired is true while verification is off")
		}
	})

	t.Run("unknown API paths are JSON, not the page", func(t *testing.T) {
		code, raw := a.do(http.MethodGet, "/api/does-not-exist", nil)
		if code != http.StatusNotFound {
			t.Fatalf("unknown API path = %d, body %s", code, raw)
		}
		var payload struct {
			Error     string `json:"error"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unknown API path answered HTML: %s", raw)
		}
		if payload.Error == "" {
			t.Error("the JSON 404 names no error")
		}
	})

	t.Run("metrics are off by default", func(t *testing.T) {
		code, raw := a.do(http.MethodGet, "/metrics", nil)
		if code != http.StatusNotFound {
			t.Fatalf("/metrics = %d, body %s", code, raw)
		}
		if err := json.Unmarshal(raw, &struct{}{}); err != nil {
			t.Fatalf("/metrics answered something other than JSON: %s", raw)
		}
	})
}

// TestRejectionBudget holds the flood guard end to end: with verification on
// and an issuer that cannot be reached, every request fails, and the answer
// stops being 401 once the budget is spent. Without it an invented token buys
// unbounded work.
func TestRejectionBudget(t *testing.T) {
	srv := startServer(t, serverOptions{auth: true})
	a := &api{t: t, base: srv.base, hc: &http.Client{Timeout: 15 * time.Second}}

	badToken := map[string]string{"Authorization": "Bearer not-a-real-token"}

	if code, _ := a.do(http.MethodGet, "/api/accounts", badToken); code != http.StatusUnauthorized {
		t.Fatalf("first bad token = %d, want 401", code)
	}

	// The budget is per source per minute; 127.0.0.1 is the only source here.
	var code int
	for i := 0; i < 60; i++ {
		code, _ = a.do(http.MethodGet, "/api/accounts", badToken)
		if code == http.StatusTooManyRequests {
			break
		}
	}
	if code != http.StatusTooManyRequests {
		t.Fatalf("after 60 rejected tokens the answer is still %d, want 429", code)
	}

	// A 429 is deliberately not given a request id: the point is to stop
	// spending work on the caller, and an id is work.
	_, raw := a.do(http.MethodGet, "/api/accounts", badToken)
	var payload struct {
		Error     string `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("429 is not JSON: %s", raw)
	}
	if payload.Error == "" {
		t.Error("429 names no error")
	}

	// The health probe stays public and unthrottled: an orchestrator must not
	// lose the ability to restart a process that is being flooded.
	if code, _ := a.do(http.MethodGet, "/api/health", nil); code != http.StatusOK {
		t.Errorf("health during a flood = %d, want 200", code)
	}
}

// The container healthcheck runs this command, not curl: it has to find the
// server through the config the process was started with, and it has to fail
// when nothing answers. Both halves are what make the exit code trustworthy.
func TestHealthcheckCommand(t *testing.T) {
	srv := startServer(t, serverOptions{})

	// The same environment the server was started with: viper lets DDNS_* win
	// over the file, so a stray listener variable would point the probe at a
	// port this test never opened.
	up := exec.Command(srv.bin, "-healthcheck", "-c", srv.cfg)
	up.Env = envWithoutDDNS()
	if out, err := up.CombinedOutput(); err != nil {
		t.Fatalf("healthcheck against a running server = %v\n%s", err, out)
	}

	dead := freeAddr(t)
	down := exec.Command(srv.bin, "-healthcheck", "-healthcheck-url", "http://"+dead+"/api/health")
	down.Env = envWithoutDDNS()
	out, err := down.CombinedOutput()
	if err == nil {
		t.Fatalf("healthcheck against %s must fail, output:\n%s", dead, out)
	}
}
