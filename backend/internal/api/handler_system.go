package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ipdetect"
)

// health is the health check endpoint used by container probes
// health is liveness: the process answers at all. The version travels with it
// so a deployment can be identified without reading the startup log.
func (s *Server) health(c *gin.Context) {
	ok(c, gin.H{"status": "ok", "version": config.Version})
}

// healthReporter is what a verifier adds to readiness: whether the issuer has
// ever been reached. Optional, so a test can hand in a stub.
type healthReporter interface {
	Health() (bool, string)
}

// ready answers whether this instance can serve traffic: the database answers,
// and - when verification is on - the identity provider has been reached at
// least once. A load balancer uses this to take a broken instance out of
// rotation while leaving the process alive for inspection.
func (s *Server) ready(c *gin.Context) {
	if err := s.store.Ping(c.Request.Context()); err != nil {
		slog.Warn("readiness check failed", "rid", middleware.RequestIDFrom(c), "error", err)
		fail(c, http.StatusServiceUnavailable, "database is not ready")
		return
	}
	if reporter, ok := any(s.verifier).(healthReporter); ok {
		if healthy, detail := reporter.Health(); !healthy {
			slog.Warn("readiness check failed", "rid", middleware.RequestIDFrom(c), "detail", detail)
			fail(c, http.StatusServiceUnavailable, "identity provider is not ready")
			return
		}
	}
	ok(c, gin.H{"status": "ready"})
}

// frontendConfig answers the SPA's runtime configuration.
//
// The bundle fetches this instead of carrying the values inside itself, which is
// what lets one image be deployed against any identity provider: everything comes
// from this process's own configuration, so an environment variable on the
// container is enough and nothing has to be rebuilt.
//
// The answer is the config object itself rather than the usual {"data": ...}
// envelope: the bundle reads it as a plain document, the same shape the static
// config.json has when something other than this server hosts the files.
func (s *Server) frontendConfig(c *gin.Context) {
	// The same URL answers differently after a redeploy
	c.Header("Cache-Control", "no-store")
	middleware.WriteJSON(c, http.StatusOK, gin.H{
		"appName": s.cfg.Frontend.AppName,
		// The browser only needs a token while this server checks one
		"apiBaseUrl":   s.cfg.Frontend.APIBaseURL,
		"authRequired": !s.cfg.OIDC.Disabled,
		// What this deployment can do at all, as opposed to what the browser
		// may do: the certificate pages offer no issuance while this is false,
		// because the API would answer 503 for one. The flag is not secret -
		// the answer is a public document either way - and the server keeps
		// refusing whatever a stale bundle asks for.
		"acmeEnabled": s.cfg.ACME.Enabled,
		"oidc": gin.H{
			"issuer":            s.cfg.OIDC.Issuer,
			"clientId":          s.cfg.OIDC.ClientID,
			"scope":             s.cfg.Frontend.Scope,
			"redirectPath":      s.cfg.Frontend.RedirectPath,
			"providerName":      s.cfg.Frontend.ProviderName,
			"logoutUrlTemplate": s.cfg.Frontend.LogoutURLTemplate,
		},
	})
}

// currentIP detects the current public IP; family is v4 or v6
func (s *Server) currentIP(c *gin.Context) {
	family := ipdetect.V4
	if c.DefaultQuery("family", "v4") == "v6" {
		family = ipdetect.V6
	}

	ip, err := s.detector.Detect(c.Request.Context(), family)
	if err != nil {
		// Detection failure (no IPv6 egress, for example) is an environment state, not a server error.
		// Responding 200 lets the frontend degrade gracefully and keeps the console free of red errors,
		// while also stopping the reverse proxy from retrying a 500 and turning it into a 502.
		// The reason names the detection endpoints, which are deployment
		// configuration, so it goes to the log instead of the response
		slog.Error("could not detect the public address", "family", family, "rid", middleware.RequestIDFrom(c), "error", err)
		ok(c, gin.H{"ip": "", "family": family, "available": false, "message": "the public address could not be detected"})
		return
	}
	ok(c, gin.H{"ip": ip, "family": family, "available": true})
}
