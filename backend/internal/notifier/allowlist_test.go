package notifier

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ddns-by-dnspod/internal/netutil"
)

// Internal addresses stay refused by default, and only the CIDRs listed in
// notifier.allow_private are let through.
func TestGuardURLHonoursTheAllowList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		allowed []string
		wantErr bool
	}{
		{"loopback blocked by default", "http://127.0.0.1/hook", nil, true},
		{"loopback allowed when listed", "http://127.0.0.1/hook", []string{"127.0.0.1/32"}, false},
		{"private blocked by default", "http://192.168.1.10/hook", nil, true},
		{"private allowed when listed", "http://192.168.1.10/hook", []string{"192.168.1.0/24"}, false},
		{"outside the allowed range", "http://192.168.2.10/hook", []string{"192.168.1.0/24"}, true},
		{"metadata blocked by default", "http://169.254.169.254/latest", nil, true},
		{"ipv6 loopback allowed when listed", "http://[::1]/hook", []string{"::1/128"}, false},
		{"bare ip is expanded to one host", "http://10.0.0.5/hook", []string{"10.0.0.5"}, false},
		{"public stays allowed", "https://example.com/hook", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := GuardURL(tc.raw, netutil.ParseCIDRs(tc.allowed)...)
			if tc.wantErr && err == nil {
				t.Errorf("GuardURL(%q) must be rejected", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("GuardURL(%q) = %v, want nil", tc.raw, err)
			}
		})
	}
}

// A name that resolves to an internal address must be refused too, otherwise a
// hostname with a private A record would be a way around the check.
func TestGuardURLResolvesHostNames(t *testing.T) {
	if _, err := net.LookupIP("localhost"); err != nil {
		t.Skip("localhost does not resolve here")
	}

	if err := GuardURL("http://localhost/hook"); err == nil {
		t.Error("localhost must be rejected without an allow list")
	}
	if err := GuardURL("http://localhost/hook", netutil.ParseCIDRs([]string{"127.0.0.0/8", "::1/128"})...); err != nil {
		t.Errorf("localhost must pass when loopback is allowed: %v", err)
	}
}

// The check also runs when the connection is opened, which is what catches a
// redirect to an internal address. The first hop is on the allow list, the
// redirect target is not, so only a dial time check can stop it.
func TestDialGuardCoversRedirects(t *testing.T) {
	// Redirects to the IPv6 loopback, while the allow list only covers IPv4
	target := httptest.NewServer(nil)
	defer target.Close()
	ipv6Target := strings.Replace(target.URL, "127.0.0.1", "[::1]", 1)

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ipv6Target, http.StatusFound)
	}))
	defer first.Close()

	s := New(nil, Config{AllowPrivate: []string{"127.0.0.1/32"}})
	err := s.SendWebhook(WebhookConfig{URL: first.URL}, Message{})
	if err == nil {
		t.Fatal("the redirect to the IPv6 loopback must be refused")
	}
	if !strings.Contains(err.Error(), "may not target") {
		t.Fatalf("want the guard error, got: %v", err)
	}
}

// With the address on the allow list the very same local target is reachable,
// which is what makes an internal ntfy server usable.
func TestDialGuardAllowsListedInternalAddress(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	allowed := New(nil, Config{AllowPrivate: []string{"127.0.0.1/32"}})
	if err := allowed.SendWebhook(WebhookConfig{URL: target.URL}, Message{}); err != nil {
		t.Fatalf("a listed internal address must be reachable: %v", err)
	}
}

func TestIsRestricted(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "169.254.169.254", "10.0.0.1", "192.168.1.1", "172.16.0.1", "0.0.0.0"} {
		if !isRestricted(net.ParseIP(raw)) {
			t.Errorf("%s must be restricted", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111"} {
		if isRestricted(net.ParseIP(raw)) {
			t.Errorf("%s must not be restricted", raw)
		}
	}
}

// ntfy shares the same guard, so an internal server needs the allow list too
func TestSendNtfyChecksTheServer(t *testing.T) {
	blocked := New(nil, Config{})
	err := blocked.SendNtfy(NtfyConfig{Server: "http://169.254.169.254", Topic: "ddns"}, Message{})
	if err == nil {
		t.Fatal("an internal ntfy server must be refused")
	}
	if !strings.Contains(err.Error(), "may not target") {
		t.Fatalf("want the guard error, got: %v", err)
	}
}

// A channel is configured with a name, not an address, so the dial-time check
// has to resolve it: refusing every name would make every domain target
// unreachable while looking like a stricter guard.
func TestDialGuardAllowsANameOnTheAllowList(t *testing.T) {
	if _, err := net.LookupIP("localhost"); err != nil {
		t.Skip("localhost does not resolve here")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	named := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	s := New(nil, Config{AllowPrivate: []string{"127.0.0.0/8", "::1/128"}})
	if err := s.SendWebhook(WebhookConfig{URL: named}, Message{}); err != nil {
		t.Fatalf("a name that resolves to a listed address must be reachable: %v", err)
	}
}

// The resolution is the check: a name pointing somewhere else must still be
// refused, whichever address the dialer is handed.
func TestDialGuardRefusesANameThatResolvesElsewhere(t *testing.T) {
	if _, err := net.LookupIP("localhost"); err != nil {
		t.Skip("localhost does not resolve here")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	named := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	s := New(nil, Config{})
	err := s.SendWebhook(WebhookConfig{URL: named}, Message{})
	if err == nil {
		t.Fatal("a name resolving to the loopback must be refused without an allow list")
	}
	if !strings.Contains(err.Error(), "may not target") {
		t.Fatalf("want the guard error, got: %v", err)
	}
}

// A name with several answers is refused when any of them is internal: the
// public one is not a reason to let the name through, because the internal one
// would be reachable by a later dial.
func TestGuardURLRefusesANameWithAnyInternalAddress(t *testing.T) {
	withResolver(t, []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.1")})

	err := GuardURL("http://mixed.example.com/hook")
	if err == nil {
		t.Fatal("a name with one internal answer must be refused")
	}
	if !strings.Contains(err.Error(), "may not target") {
		t.Fatalf("want the guard error, got: %v", err)
	}
}

// A name that answers with nothing is refused as well: there is no address to
// check, and dialing would resolve it a second time, outside this guard.
func TestGuardURLRefusesANameThatResolvesToNothing(t *testing.T) {
	withResolver(t, nil)

	err := GuardURL("http://empty.example.com/hook")
	if err == nil {
		t.Fatal("a name that resolves to no address must be refused")
	}
	if !strings.Contains(err.Error(), "no address") {
		t.Fatalf("want the empty-resolution error, got: %v", err)
	}
}

// withResolver replaces the resolver for one test and restores it afterwards.
func withResolver(t *testing.T, ips []net.IP) {
	t.Helper()
	original := lookupIP
	t.Cleanup(func() { lookupIP = original })
	lookupIP = func(string) ([]net.IP, error) { return ips, nil }
}
