package acme

import (
	"errors"
	"strings"
	"testing"
)

func TestChallengeRecordName(t *testing.T) {
	cases := []struct {
		name       string
		identifier string
		want       string
	}{
		{"a plain name", "example.com", "_acme-challenge.example.com"},
		{"a wildcard order", "*.example.com", "_acme-challenge.example.com"},
		{"a deeper name", "a.b.example.com", "_acme-challenge.a.b.example.com"},
		{"a trailing dot", "example.com.", "_acme-challenge.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChallengeRecordName(tc.identifier); got != tc.want {
				t.Fatalf("ChallengeRecordName(%q) = %q, want %q", tc.identifier, got, tc.want)
			}
		})
	}
}

// The plan decides which zone the record is filed in, which is the one thing
// that decides whether the CA can ever see it.
func TestRecordPlan(t *testing.T) {
	cases := []struct {
		name       string
		identifier string
		zones      []string
		wantZone   string
		wantHost   string
	}{
		{
			name:       "the only zone",
			identifier: "a.example.com",
			zones:      []string{"example.com"},
			wantZone:   "example.com",
			wantHost:   "_acme-challenge.a",
		},
		{
			name:       "the longest zone wins",
			identifier: "a.b.example.com",
			zones:      []string{"example.com", "b.example.com"},
			wantZone:   "b.example.com",
			wantHost:   "_acme-challenge.a",
		},
		{
			name:       "a zone that is not a suffix is skipped",
			identifier: "a.example.com",
			zones:      []string{"notexample.com", "example.com"},
			wantZone:   "example.com",
			wantHost:   "_acme-challenge.a",
		},
		{
			name:       "a wildcard lands at the base name",
			identifier: "*.example.com",
			zones:      []string{"example.com"},
			wantZone:   "example.com",
			wantHost:   "_acme-challenge",
		},
		{
			name:       "whitespace and a trailing dot are tolerated",
			identifier: "a.example.com",
			zones:      []string{" example.com. "},
			wantZone:   "example.com",
			wantHost:   "_acme-challenge.a",
		},
		{
			name:       "a zone holding the challenge name itself",
			identifier: "example.com",
			zones:      []string{"example.com", "_acme-challenge.example.com"},
			// The record goes to the apex of that zone, which DNSPod writes as
			// "@", rather than to a host that would be a name of its own.
			wantZone: "_acme-challenge.example.com",
			wantHost: "@",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zone, host, err := RecordPlan(tc.identifier, tc.zones)
			if err != nil {
				t.Fatalf("RecordPlan: %v", err)
			}
			if zone != tc.wantZone || host != tc.wantHost {
				t.Fatalf("RecordPlan = (%q, %q), want (%q, %q)", zone, host, tc.wantZone, tc.wantHost)
			}
		})
	}
}

func TestRecordPlanWithoutAMatchingZone(t *testing.T) {
	_, _, err := RecordPlan("a.example.com", []string{"other.com", ""})
	if err == nil {
		t.Fatal("want an error when no zone of the account serves the name")
	}
	var noZone *noZoneError
	if !errors.As(err, &noZone) {
		t.Fatalf("error = %T, want *noZoneError", err)
	}
	if !strings.Contains(err.Error(), "_acme-challenge.a.example.com") {
		t.Fatalf("error %q should name the record it could not place", err)
	}
}
