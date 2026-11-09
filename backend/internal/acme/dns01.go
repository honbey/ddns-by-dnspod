package acme

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"ddns-by-dnspod/internal/dnspod"
)

// challengePrefix is the label a DNS-01 challenge is published under.
const challengePrefix = "_acme-challenge"

// challengeLine is the DNSPod line the TXT records are filed under: the API
// wants the line's name, and the default line's name is Chinese. It is the one
// non-ASCII string in this package, for the same reason record management
// already carries it.
const challengeLine = "默认"

// noZoneError says a challenge name has nowhere to go, and names it: that is
// the whole answer the reader needs, because the fix is a zone in this account
// or a different account on the certificate.
type noZoneError struct{ name string }

func (e *noZoneError) Error() string {
	return fmt.Sprintf("no domain of this account serves %s, so the challenge record has nowhere to go", e.name)
}

// ChallengeRecordName is the name a DNS-01 challenge is published under.
//
// A wildcard order is authorized for the base name - the identifier is
// example.com with the wildcard flag set - so "*.example.com" and "example.com"
// both publish under _acme-challenge.example.com.
func ChallengeRecordName(identifier string) string {
	return challengePrefix + "." + strings.TrimPrefix(strings.TrimSuffix(identifier, "."), "*.")
}

// RecordPlan says where a challenge record goes, given the zones an account
// holds, and returns the zone together with the host inside it.
//
// The longest zone that is a suffix of the name wins: for
// _acme-challenge.a.b.example.com with both b.example.com and example.com in
// the account, the record belongs in b.example.com as _acme-challenge.a.
// Filing it in the other one would put the TXT record in a zone that is not
// answering for the name, and the CA would never see it.
func RecordPlan(identifier string, zones []string) (zone, host string, err error) {
	name := ChallengeRecordName(identifier)

	var longest string
	for _, candidate := range zones {
		candidate = strings.TrimSuffix(strings.TrimSpace(candidate), ".")
		if candidate == "" {
			continue
		}
		if name == candidate || strings.HasSuffix(name, "."+candidate) {
			if len(candidate) > len(longest) {
				longest = candidate
			}
		}
	}
	if longest == "" {
		return "", "", &noZoneError{name: name}
	}
	// The zone is a suffix of the name, so what is left of the name after it -
	// and after the dot that joins them - is the host the record is created
	// under. Trimming the wrong end would hand DNSPod the whole name as the
	// host and file the record one zone too deep.
	host = strings.TrimSuffix(name, "."+longest)
	if host == name {
		// The zone is the challenge name itself, which happens when
		// _acme-challenge.example.com is a zone of its own - a delegation some
		// setups use. The record then belongs at that zone's apex, which DNSPod
		// writes as "@".
		host = "@"
	}
	return longest, host, nil
}

// dnsPresenter publishes the TXT records one issuance needs, through the same
// DNSPod API the DDNS tasks write through.
type dnsPresenter struct {
	cli   *dnspod.Client
	zones []string
}

// newDNSPresenter reads the zones of the account once: every challenge of an
// issuance has to be placed in one of them, and the client caches the listing.
func newDNSPresenter(ctx context.Context, cli *dnspod.Client) (*dnsPresenter, error) {
	domains, err := cli.ListDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the domains of this account: %w", err)
	}
	zones := make([]string, 0, len(domains))
	for _, domain := range domains {
		zones = append(zones, domain.Name)
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("this account holds no domain, so a DNS-01 challenge cannot be answered")
	}
	return &dnsPresenter{cli: cli, zones: zones}, nil
}

// Zones is what the account holds, in the form a record plan needs it.
func (p *dnsPresenter) Zones() []string { return p.zones }

// Present publishes one challenge record and returns the id it was created
// with.
//
// The id rather than the name is what removal needs: one name carries one
// record per challenge, and a wildcard order and a base-name order are two
// records that differ only in their value.
func (p *dnsPresenter) Present(zone, host, value string) (uint64, error) {
	recordID, err := p.cli.CreateRecord(zone, dnspod.CreateRecordInput{
		SubDomain:  host,
		RecordType: "TXT",
		Value:      value,
		Line:       challengeLine,
		// The CA asks the zone's authoritative servers directly, so the TTL
		// only decides how long a stale answer could sit in a cache after the
		// record is removed again.
		TTL: 600,
	})
	if err != nil {
		return 0, fmt.Errorf("publish %s.%s: %w", host, zone, err)
	}
	return recordID, nil
}

// CleanUp removes one published challenge record.
func (p *dnsPresenter) CleanUp(zone string, recordID uint64) error {
	if err := p.cli.DeleteRecord(zone, recordID); err != nil {
		return fmt.Errorf("remove the challenge record %d in %s: %w", recordID, zone, err)
	}
	return nil
}

// waitForTXT waits until the resolver answers with the published value.
//
// The CA reads the zone's authoritative servers itself, so a resolver that
// never sees the record is not a failure: the wait only gives the record a head
// start, and a timeout leaves the CA as the judge of whether it is there.
func waitForTXT(ctx context.Context, name, value string, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	var last error

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		records, err := net.DefaultResolver.LookupTXT(lookupCtx, name)
		cancel()

		if err == nil {
			for _, record := range records {
				if record == value {
					return nil
				}
			}
			last = fmt.Errorf("%s answers, but with no matching TXT record yet", name)
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waited %s for %s: %w", timeout, name, last)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
