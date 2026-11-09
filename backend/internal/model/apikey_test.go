package model

import "testing"

// The scope check is the only thing keeping one key out of another domain of the
// same account, so it is pinned here rather than only through the handler.
func TestAPIKeyCovers(t *testing.T) {
	cases := []struct {
		scope  string
		domain string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"Example.COM", "example.com", true},
		{"example.com", "EXAMPLE.com", true},
		{"example.com", "other.com", false},
		{"example.com", "sub.example.com", false},
		{APIKeyWildcard, "example.com", true},
		{APIKeyWildcard, "other.com", true},
		{APIKeyWildcard, "deep.sub.example.com", true},
		{"", "example.com", false},
		{"com", "example.com", false},
	}

	for _, tc := range cases {
		if got := APIKeyCovers(tc.scope, tc.domain); got != tc.want {
			t.Errorf("APIKeyCovers(%q, %q) = %v, want %v", tc.scope, tc.domain, got, tc.want)
		}
	}
}
