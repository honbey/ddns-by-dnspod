package middleware

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// PeerIP is the address that actually opened the connection.
func PeerIP(c *gin.Context) string {
	if c.Request == nil {
		return ""
	}
	addr := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// trustedNetworks are the peers whose X-Forwarded-For is believed: the reverse
// proxy in front of this process. Everything else is identified by its TCP
// peer, so a client cannot pick its own address by sending a header.
var (
	trustedMu       sync.RWMutex
	trustedNetworks []*net.IPNet
)

// SetTrustedProxies configures which peers may speak for a client. An empty
// list means nobody can. A malformed entry is an error rather than a warning:
// silently falling back would widen or narrow trust without saying so.
func SetTrustedProxies(cidrs []string) error {
	networks := make([]*net.IPNet, 0, len(cidrs))
	for _, raw := range cidrs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(value); err == nil {
			networks = append(networks, network)
			continue
		}
		// A bare address is a /32 (or /128) and a fair thing to write.
		ip := net.ParseIP(value)
		if ip == nil {
			return fmt.Errorf("trusted proxy %q is not a CIDR or an address", raw)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}

	trustedMu.Lock()
	trustedNetworks = networks
	trustedMu.Unlock()
	return nil
}

func isTrustedProxy(ip net.IP) bool {
	trustedMu.RLock()
	defer trustedMu.RUnlock()
	for _, network := range trustedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// SourceIP is the client this request is attributed to: X-Forwarded-For when
// the peer is a configured proxy, the peer itself otherwise.
//
// The access log is its only reader today, and that is reason enough for it to
// be one function: a log keyed on anything a caller can forge is a log the
// caller writes. Widening the proxy list beyond the reverse proxy itself gives
// every client inside that range the same power.
func SourceIP(c *gin.Context) string {
	peer := PeerIP(c)
	if ip := net.ParseIP(peer); ip != nil && isTrustedProxy(ip) {
		return c.ClientIP()
	}
	return peer
}
