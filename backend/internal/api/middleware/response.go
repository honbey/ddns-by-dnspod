package middleware

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// WriteJSON encodes obj without HTML escaping.
//
// gin's c.JSON escapes <, > and &, which turns every "old -> new" pair in the
// history into "\u003e" on the wire. The API serves application/json only -
// there is no HTML to inject - so the escape buys nothing and costs
// readability.
func WriteJSON(c *gin.Context, status int, obj any) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(status)
	encoder := json.NewEncoder(c.Writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(obj)
}

// WriteError writes the shared failure shape: {"error": "...", "request_id":
// "..."}. Every refusal goes through here, so the shape cannot drift between
// layers and the request id is always the one the log carries. The id is
// omitted only when there is none to carry - a bare context a unit test
// builds, never a request.
//
// WriteCodedError adds the optional code on top of this shape. A client that
// only shows the message can ignore it.
func WriteError(c *gin.Context, status int, message string) {
	writeError(c, status, "", message)
}

// WriteCodedError is WriteError with a stable identifier, for a failure the
// client reacts to rather than shows. The message is for a person and may be
// reworded or translated; the code is the contract.
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

// StatusNotResponded is what a request the caller walked away from answers
// with. net/http has no such code - nginx defined 499 for it - and it keeps
// abandoned requests out of the 5xx count, where they would look like faults.
const StatusNotResponded = 499
