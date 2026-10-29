package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/metrics"
)

// A refused token costs a log line at minimum and, on the opaque path, a round
// trip to the issuer. A public deployment rate-limits at the proxy; this is the
// floor that holds without one.
//
// Only failures spend the budget. A token already proven valid is let through
// whatever its source address has been up to; an unknown opaque token gets one
// bounded probe per interval - behind a NAT or CGNAT, one neighbour's noise is
// not another's permanent lockout.
//
// The key is the attributed client (SourceIP): the TCP peer, or the
// X-Forwarded-For client when the peer is a configured proxy.
var (
	rejectBudgetPerMinute = 30
	rejectWindow          = time.Minute

	rejectMu sync.Mutex
	rejects  = map[string]*rejectCounter{}
)

// maxRejectSources bounds the per-source map: one entry per address that has
// failed at least once in the window.
const maxRejectSources = 4096

type rejectCounter struct {
	start     time.Time
	count     int
	lastProbe time.Time
}

// opaqueProbeInterval is how often a source over its rejection budget may still
// spend one token verification. It keeps a valid opaque token from being locked
// out while bounding the work an invalid flood can cause.
const opaqueProbeInterval = 5 * time.Second

// allowOpaqueAttempt reports whether an opaque token may be verified even when
// its source is over budget. It grants one probe per interval; a known-good
// token is handled before this and does not consume a probe.
func allowOpaqueAttempt(ip string) bool {
	rejectMu.Lock()
	defer rejectMu.Unlock()

	now := time.Now()
	w := rejects[ip]
	if w == nil || now.Sub(w.start) > rejectWindow || w.count <= rejectBudgetPerMinute {
		return true
	}
	if !w.lastProbe.IsZero() && now.Sub(w.lastProbe) < opaqueProbeInterval {
		return false
	}
	w.lastProbe = now
	return true
}

// SetRejectBudgetPerMinute makes the rejection budget an operational setting
// rather than a constant. Guarded by the same mutex as the reads, so a future
// runtime reconfiguration cannot race a request.
func SetRejectBudgetPerMinute(n int) {
	if n <= 0 {
		return
	}
	rejectMu.Lock()
	rejectBudgetPerMinute = n
	rejectMu.Unlock()
}

// overRejectBudget reports whether this source has spent its budget for the
// window. Checked after a failure: the work of one verification is what the
// budget is measured in, and a caller that verifies successfully is never
// turned away.
func overRejectBudget(ip string) bool {
	rejectMu.Lock()
	defer rejectMu.Unlock()

	now := time.Now()
	w := rejects[ip]
	if w == nil || now.Sub(w.start) > rejectWindow {
		return false
	}
	return w.count > rejectBudgetPerMinute
}

// recordReject spends one unit of that budget.
func recordReject(ip string) {
	rejectMu.Lock()
	defer rejectMu.Unlock()

	now := time.Now()
	w := rejects[ip]
	if w == nil || now.Sub(w.start) > rejectWindow {
		// Bound the map: a rotating cast of sources must not grow it forever.
		// Expired windows go first; clearing the whole map would hand every
		// flooding source a fresh budget.
		if len(rejects) > maxRejectSources {
			for source, counter := range rejects {
				if now.Sub(counter.start) > rejectWindow {
					delete(rejects, source)
				}
			}
			for source := range rejects {
				if len(rejects) <= maxRejectSources {
					break
				}
				delete(rejects, source)
			}
		}
		rejects[ip] = &rejectCounter{start: now, count: 1}
		return
	}
	w.count++
}

// writeTooManyAttempts answers a source past its budget. Deliberately not
// WriteError: the body carries no request id, so a flood cannot correlate its
// own answers, and the per-failure detail is not amplified.
func writeTooManyAttempts(c *gin.Context) {
	metrics.IncAuthRejection("rate_limited")
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": "too many failed authentication attempts",
	})
}
