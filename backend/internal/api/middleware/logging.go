package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLog records one structured line per request, including the identity
// the Bearer middleware derived.
//
// That caller is what turns the access log into something worth searching: the
// request says what happened, the caller says who asked for it. Business events
// are logged again at their own handlers with their own names, but this is the
// one record no request escapes.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http_request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", SourceIP(c),
			"peer", PeerIP(c),
			"rid", RequestIDFrom(c),
			"caller", CurrentUser(c),
		)
	}
}
