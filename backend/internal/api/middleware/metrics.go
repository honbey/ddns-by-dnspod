package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/metrics"
)

// Metrics records every request that comes through: count, duration and
// concurrency, keyed by the route pattern rather than the concrete path.
//
// The scrape endpoint is skipped: a scrape is not traffic, and counting it
// would hide everything else behind a sawtooth of its own.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.FullPath() == "/metrics" {
			c.Next()
			return
		}

		metrics.IncInFlight()
		start := time.Now()
		defer func() {
			metrics.DecInFlight()
			// FullPath is the template ("/api/todos/:id"); an unmatched path
			// has none, and its raw form would be unbounded.
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			metrics.ObserveRequest(c.Request.Method, route, c.Writer.Status(), time.Since(start))
		}()
		c.Next()
	}
}
