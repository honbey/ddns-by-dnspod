// Package netutil holds the small IP allow list helpers shared by the API
// middlewares and the notifier.
package netutil

import (
	"net"
	"strings"
)

// ParseCIDRs turns a list of CIDR strings into networks, skipping blank and
// malformed entries.
//
// Bare IPs are accepted and expanded to a single host network, so "10.0.0.5"
// behaves like "10.0.0.5/32" and "fe80::1" like "fe80::1/128". Malformed input
// is dropped rather than reported: these lists come from configuration, and one
// typo must not keep the service from starting.
func ParseCIDRs(list []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(list))
	for _, item := range list {
		if n, ok := parseCIDR(item); ok {
			nets = append(nets, n)
		}
	}
	return nets
}

// Rejected names the entries ParseCIDRs dropped.
//
// A list of this kind decides what may be reached, and one typo in it is
// otherwise invisible: the entry simply never matches and the thing it was
// supposed to allow stays blocked. Reporting the entries lets a deployment see
// which value it got wrong.
func Rejected(list []string) []string {
	var rejected []string
	for _, item := range list {
		if _, ok := parseCIDR(item); !ok {
			rejected = append(rejected, strings.TrimSpace(item))
		}
	}
	return rejected
}

// parseCIDR accepts a CIDR, or a bare IP expanded to a single host network
func parseCIDR(item string) (*net.IPNet, bool) {
	item = strings.TrimSpace(item)
	if item == "" {
		return nil, false
	}
	if !strings.Contains(item, "/") {
		if strings.Contains(item, ":") {
			item += "/128"
		} else {
			item += "/32"
		}
	}
	_, n, err := net.ParseCIDR(item)
	if err != nil {
		return nil, false
	}
	return n, true
}

// Contains reports whether the address falls inside one of the networks.
// An address that does not parse never matches.
func Contains(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
