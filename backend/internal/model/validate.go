package model

import (
	"fmt"
	"regexp"
	"strings"
)

// Host names reach two places where they are not inert:
//
//   - the change history, which the frontend renders as HTML (an ECharts
//     tooltip uses innerHTML), so a stored value can become script;
//   - the server log, where an embedded newline can forge extra log lines.
//
// Restricting them to the characters a DNS name can legitimately contain
// removes both problems at the source, rather than relying on every consumer
// to remember to escape.
var (
	// One DNS label: starts and ends alphanumeric, may contain hyphens inside.
	labelRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

	// A host record: a single label, or labels joined by dots. Wildcard "*"
	// and root "@" are DNSPod conventions and are accepted as whole values.
	subDomainRe = regexp.MustCompile(`^[A-Za-z0-9_*]([A-Za-z0-9-_*]{0,61}[A-Za-z0-9_*])?(\.[A-Za-z0-9_*]([A-Za-z0-9-_*]{0,61}[A-Za-z0-9_*])?)*$`)
)

// maxNameLen caps stored host names. DNS itself allows longer strings in
// presentation format, but nothing legitimate in this system needs it.
const maxNameLen = 253

// ValidateDomain checks a zone name, e.g. example.com.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	if len(domain) > maxNameLen {
		return fmt.Errorf("domain is too long (at most %d characters)", maxNameLen)
	}

	labels := strings.Split(domain, ".")
	// A zone name needs at least two labels: a name and a TLD.
	if len(labels) < 2 {
		return fmt.Errorf("invalid domain, expected a name like example.com")
	}
	for _, l := range labels {
		if l == "" {
			return fmt.Errorf("invalid domain: it contains an empty label")
		}
		if !labelRe.MatchString(l) {
			return fmt.Errorf("invalid domain: label %q has illegal characters", l)
		}
	}
	return nil
}

// ValidateSubDomain checks a host record: a label, "@" for the zone apex,
// or "*" for a wildcard.
func ValidateSubDomain(sub string) error {
	if sub == "" {
		return fmt.Errorf("host record is required")
	}
	if len(sub) > maxNameLen {
		return fmt.Errorf("host record is too long (at most %d characters)", maxNameLen)
	}
	if sub == "@" || sub == "*" {
		return nil
	}
	if !subDomainRe.MatchString(sub) {
		return fmt.Errorf("invalid host record: only letters, digits, hyphen, dot and underscore are allowed")
	}
	return nil
}
