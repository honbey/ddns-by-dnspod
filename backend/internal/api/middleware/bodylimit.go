package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DefaultMaxBody is how much of a request body this service is willing to read.
// It is a whole-request budget rather than a per-field one: every payload here
// is a small JSON document, so anything past this is a mistake or an attempt to
// exhaust memory, and both deserve the same short answer.
const DefaultMaxBody = 4 << 20

// MaxBody caps the request body.
//
// The reader is lazy, so nothing is buffered for a request whose handler never
// reads the body - an unauthenticated call is rejected by Bearer before a byte
// is pulled in. What the cap buys is that an oversized body surfaces as a read
// error the decoding layer answers with 413, instead of an allocation the
// process has to survive.
func MaxBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}
