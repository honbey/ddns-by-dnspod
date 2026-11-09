package config

import (
	"os"
	"strings"
	"testing"
)

// Detection sources come from the configuration only, so a fixture without any
// looks broken to Validate() and would drown the auth assertions in noise.
func withSources(cfg *Config) *Config {
	cfg.IPDetect = IPDetectConfig{
		V4: []string{"https://example.com/ip"},
		V6: []string{"https://example.com/ip"},
	}
	return cfg
}

// A complete sign in setup, which is what a deployment should look like
func completeOIDC() OIDCConfig {
	return OIDCConfig{
		Issuer:   "https://auth.example.com",
		ClientID: "ddns",
		Audience: "ddns",
	}
}

// Turning verification off leaves the port open to anyone who can reach it, so
// it is the one setting worth shouting about.
func TestValidateWarnsWhenVerificationIsDisabled(t *testing.T) {
	cfg := withSources(&Config{OIDC: OIDCConfig{Disabled: true}})
	warnings := cfg.Validate()
	if len(warnings) != 1 {
		t.Fatalf("want exactly 1 warning, got %v", warnings)
	}
}

func TestValidateQuietForCompleteSignInSetup(t *testing.T) {
	cfg := withSources(&Config{
		Server: ServerConfig{Listen: ":8080"},
		OIDC:   completeOIDC(),
	})
	if warnings := cfg.Validate(); len(warnings) != 0 {
		t.Fatalf("sane setup must not warn, got %v", warnings)
	}
}

// A missing issuer or client id is no longer a warning: Load refuses to start
// over it, because a token can never be accepted without them. Only the
// audience check can be switched off and still leave a working setup.
func TestValidateWarnsWhenTheAudienceCheckIsOff(t *testing.T) {
	oidc := completeOIDC()
	oidc.SkipAudienceCheck = true
	cfg := withSources(&Config{OIDC: oidc})
	if warnings := cfg.Validate(); len(warnings) == 0 {
		t.Fatal("a switched off audience check must warn")
	}
}

// The private key's mode can be wrong in both directions: readable by more than
// the process that wrote it, or readable by nobody at all - the second is the
// one that looks safe and stops the certificate from being served.
func TestValidateWarnsAboutTheCertificateKeyMode(t *testing.T) {
	cases := []struct {
		mode string
		perm os.FileMode
		want string
	}{
		// 0640 is what the configuration recommends for a TLS application in a
		// shared group, so it must not be warned about as if it were a slip.
		{"0640", 0o640, ""},
		{"0600", 0o600, ""},
		{"0400", 0o400, ""},
		{"0644", 0o644, "readable or writable beyond its owner and group"},
		{"0660", 0o660, "readable or writable beyond its owner and group"},
		{"0000", 0o000, "without read permission for its owner"},
		{"0044", 0o044, "without read permission for its owner"},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := withSources(&Config{
				Server: ServerConfig{Listen: ":8080"},
				OIDC:   completeOIDC(),
				ACME: ACMEConfig{
					Enabled:         true,
					Email:           "ops@example.com",
					KeyMode:         tc.mode,
					KeyFileMode:     tc.perm,
					RenewBeforeDays: 30,
				},
			})

			warnings := cfg.Validate()
			if tc.want == "" {
				if len(warnings) != 0 {
					t.Fatalf("a usable key mode must not warn, got %v", warnings)
				}
				return
			}
			for _, warning := range warnings {
				if strings.Contains(warning, tc.want) {
					return
				}
			}
			t.Fatalf("warnings = %v, want one about %q", warnings, tc.want)
		})
	}
}

// No address pool is built into the code any more, so an empty one is a broken
// deployment rather than a request for the defaults.
func TestValidateWarnsWhenADetectionPoolIsEmpty(t *testing.T) {
	cases := []struct {
		name     string
		ipDetect IPDetectConfig
		want     int
	}{
		{name: "both families empty", ipDetect: IPDetectConfig{}, want: 2},
		{name: "v4 only", ipDetect: IPDetectConfig{V4: []string{"https://example.com/ip"}}, want: 1},
		{
			name:     "v4 from a JSON source only",
			ipDetect: IPDetectConfig{V4JSON: []IPEndpoint{{URL: "https://example.com/ip"}}},
			want:     1,
		},
		{
			name:     "both families configured",
			ipDetect: IPDetectConfig{V4: []string{"a"}, V6: []string{"b"}},
			want:     0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{
				OIDC:     completeOIDC(),
				IPDetect: c.ipDetect,
			}
			if got := len(cfg.Validate()); got != c.want {
				t.Fatalf("warnings = %d, want %d: %v", got, c.want, cfg.Validate())
			}
		})
	}
}

// The line AUTH_DISABLED is allowed to cross is the loopback one: a test for
// every shape of listen address, because this is the only thing standing
// between a developer convenience and an open DNS console.
func TestListensOnLoopback(t *testing.T) {
	cases := []struct {
		listen string
		want   bool
	}{
		{":8080", false},             // no host part: serves everywhere
		{"0.0.0.0:8080", false},      // every interface
		{"[::]:8080", false},         // every interface, v6
		{"192.168.1.10:8080", false}, // a LAN address
		{"10.0.0.5:8080", false},     // a private address that is not this host
		{"203.0.113.7:8080", false},  // a public address
		{"127.0.0.1:8080", true},
		{"127.0.0.2:8080", true}, // the whole 127/8 is loopback
		{"[::1]:8080", true},
		{"localhost:8080", true},
		{"localhost", false}, // no port at all: not a listen address
	}
	for _, tc := range cases {
		if got := ListensOnLoopback(tc.listen); got != tc.want {
			t.Errorf("ListensOnLoopback(%q) = %v, want %v", tc.listen, got, tc.want)
		}
	}
}
