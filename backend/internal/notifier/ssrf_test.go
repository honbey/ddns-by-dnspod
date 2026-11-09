package notifier

import "testing"

func TestGuardURLAcceptsPublicHTTP(t *testing.T) {
	// example.com and friends are documentation domains, they resolve publicly
	for _, raw := range []string{
		"https://example.com/hook",
		"http://example.com/hook",
		"https://example.com:8443/path",
	} {
		if err := GuardURL(raw); err != nil {
			t.Errorf("GuardURL(%q) = %v, want nil", raw, err)
		}
	}
}

func TestGuardURLRejectsNonHTTP(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://internal-host/_x",
		"ftp://example.com/file",
	} {
		if err := GuardURL(raw); err == nil {
			t.Errorf("GuardURL(%q) must be rejected", raw)
		}
	}
}

func TestGuardURLRejectsInternalIPs(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/hook",         // loopback
		"http://[::1]/hook",             // IPv6 loopback
		"http://169.254.169.254/latest", // cloud metadata endpoint
		"http://10.0.0.5/hook",          // private
		"http://192.168.1.1/hook",       // private
		"http://172.16.0.9/hook",        // private
		"http://0.0.0.0/hook",           // unspecified
		"http://[fe80::1]/hook",         // IPv6 link local
	} {
		if err := GuardURL(raw); err == nil {
			t.Errorf("GuardURL(%q) must be rejected", raw)
		}
	}
}

func TestGuardURLRejectsMalformed(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"https://", // no host
		"http:///", // empty host
	} {
		if err := GuardURL(raw); err == nil {
			t.Errorf("GuardURL(%q) must be rejected", raw)
		}
	}
}
