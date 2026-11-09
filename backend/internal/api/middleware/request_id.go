package middleware

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ContextRequestIDKey is the gin context key holding the request id
const ContextRequestIDKey = "request_id"

// RequestIDHeader carries the id on both the incoming request and the response
const RequestIDHeader = "X-Request-Id"

// maxRequestIDLen is the longest upstream id we are willing to reuse
const maxRequestIDLen = 64

// requestIDCounter keeps ids unique when the random source is unavailable
var requestIDCounter uint64

// RequestID attaches an id to every request, so that a failed call can be traced
// from the browser error message back to the access log line.
//
// An id supplied by an upstream proxy is reused, which keeps the value stable
// across the whole chain. That value is client controlled, so it is validated
// first: anything outside [A-Za-z0-9._-] is rejected, otherwise a crafted
// header could inject newlines or ANSI escapes straight into the log file.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := sanitizeRequestID(c.GetHeader(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		c.Set(ContextRequestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the id of the request, or an empty string when the
// RequestID middleware is not installed.
func RequestIDFrom(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(ContextRequestIDKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

// sanitizeRequestID returns the upstream id when it is safe to log, "" otherwise.
func sanitizeRequestID(raw string) string {
	if raw == "" || len(raw) > maxRequestIDLen {
		return ""
	}
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
		case ch == '.', ch == '_', ch == '-':
		default:
			return ""
		}
	}
	return raw
}

// newRequestID builds a random UUID (v4).
// The fallback is only reached if the system random source fails.
func newRequestID() string {
	if id, err := uuid.NewRandom(); err == nil {
		return id.String()
	}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 16)
	return stamp + "-" + strconv.FormatUint(atomic.AddUint64(&requestIDCounter, 1), 16)
}
