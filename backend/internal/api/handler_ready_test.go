package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/metrics"
)

// Live and ready answer different questions, and a proxy acts on the second:
// a process whose database has gone must leave rotation without being killed.
func TestReadyReportsTheDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv, cfg := newTestServer(t)

	r := gin.New()
	srv.RegisterRoutes(r, *cfg)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("a served database must be ready, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ready") {
		t.Fatalf("body = %s, want a ready status", rec.Body.String())
	}

	// The handle goes away underneath the process; the probe has to notice.
	if err := srv.store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("a closed database must be unready, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// The scrape endpoint exists only when it is switched on: a deployment that
// never scrapes it should not publish operational detail at all.
func TestMetricsEndpointFollowsTheSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, enabled := range []bool{true, false} {
		srv, cfg := newTestServer(t, func(c *config.Config) {
			c.Metrics = config.MetricsConfig{Enabled: enabled}
		})
		r := gin.New()
		srv.RegisterRoutes(r, *cfg)

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

		if !enabled {
			if rec.Code != http.StatusNotFound {
				t.Fatalf("metrics.enabled=false serves %d, want 404", rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("metrics.enabled=true serves %d, want 200", rec.Code)
		}
		for _, want := range []string{"go_goroutines", "http_requests_in_flight"} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("the scrape output is missing %q", want)
			}
		}
	}
}

// A request that was refused has to be countable: a rising rejection rate is
// either a misconfiguration or somebody guessing.
func TestRejectedTokensAreCounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv, cfg := newTestServer(t, func(c *config.Config) {
		c.OIDC.Disabled = false
		c.OIDC.Issuer = "https://idp.invalid"
		c.OIDC.ClientID = "ddns-by-dnspod"
		c.Metrics = config.MetricsConfig{Enabled: true}
	})

	r := gin.New()
	srv.RegisterRoutes(r, *cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	r.ServeHTTP(httptest.NewRecorder(), req)

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "auth_rejections_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric.GetCounter().GetValue() > 0 {
				return
			}
		}
	}
	t.Fatal("a refused token was not counted")
}
