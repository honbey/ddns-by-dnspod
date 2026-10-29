package model

import "testing"

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		want   string
	}{
		{"typical 32-char key", "abcdefghijklmnopqrstuvwxyz123456", "abc******456"},
		{"exactly nine chars", "123456789", "123******789"},
		{"eight chars fully masked", "12345678", "*********"},
		{"empty", "", "*********"},
		{"three chars", "abc", "*********"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaskSecret(tc.secret); got != tc.want {
				t.Errorf("MaskSecret(%q) = %q, want %q", tc.secret, got, tc.want)
			}
		})
	}
}
