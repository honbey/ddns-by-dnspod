package config

import (
	"strings"
	"testing"
)

// Verification on is the production shape, so an issuer and a client id are
// what a deployment has to provide. Starting without them would only move the
// failure into the browser.
func TestLoadRequiresTheIssuerAndTheClientID(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "false")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_CLIENT_ID", "")

	if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "OIDC_ISSUER") {
		t.Fatalf("want a missing issuer to be refused, got %v", err)
	}

	t.Setenv("OIDC_ISSUER", "https://auth.example.com")
	if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "OIDC_CLIENT_ID") {
		t.Fatalf("want a missing client id to be refused, got %v", err)
	}
}

// Local development runs without a provider at all, which is the one case where
// the two required values are not.
func TestLoadAcceptsADisabledSetup(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.OIDC.Disabled {
		t.Fatal("AUTH_DISABLED=true must reach the config")
	}
	if cfg.OIDC.DisabledSub == "" {
		t.Fatal("a disabled setup still needs a subject to impersonate")
	}
}

// A provider that issues aud for the client does not need the value repeated,
// and the SPA is handed the same value either way.
func TestLoadDefaultsTheAudienceToTheClientID(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "false")
	t.Setenv("OIDC_ISSUER", "https://auth.example.com/")
	t.Setenv("OIDC_CLIENT_ID", "ddns-by-dnspod")
	t.Setenv("OIDC_AUDIENCE", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OIDC.Audience != "ddns-by-dnspod" {
		t.Fatalf("audience = %q, want the client id", cfg.OIDC.Audience)
	}
	// The discovery URL is built by appending to the issuer, so a trailing
	// slash would land in the middle of it
	if cfg.OIDC.Issuer != "https://auth.example.com" {
		t.Fatalf("issuer = %q, want the trailing slash trimmed", cfg.OIDC.Issuer)
	}
}
