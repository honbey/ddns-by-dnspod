package middleware

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// WriteJSON encodes without HTML escaping: gin's c.JSON would turn "old -> new"
// into "\u003e" in history entries.
func WriteJSON(c *gin.Context, status int, obj any) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(status)
	encoder := json.NewEncoder(c.Writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(obj)
}

// WriteError writes {"error","request_id"}; the id is omitted when absent.
func WriteError(c *gin.Context, status int, message string) {
	writeError(c, status, "", message)
}

// WriteCodedError adds the contract's optional code.
func WriteCodedError(c *gin.Context, status int, code, message string) {
	writeError(c, status, code, message)
}

func writeError(c *gin.Context, status int, code, message string) {
	body := gin.H{"error": message}
	if code != "" {
		body["code"] = code
	}
	if rid := RequestIDFrom(c); rid != "" {
		body["request_id"] = rid
	}
	c.Abort()
	WriteJSON(c, status, body)
}

// StatusNotResponded (nginx 499) keeps abandoned requests out of the 5xx count.
const StatusNotResponded = 499
