package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/dnspod"
)

// envelope is the success shape. A single wrapper means the client can tell a
// payload from an error without guessing at the status code alone - one less
// thing for every caller to re-derive.
type envelope struct {
	Data any `json:"data"`
}

func ok(c *gin.Context, data any) {
	middleware.WriteJSON(c, http.StatusOK, envelope{Data: data})
}

func created(c *gin.Context, data any) {
	middleware.WriteJSON(c, http.StatusCreated, envelope{Data: data})
}

// fail is the unified error response. It delegates to the middleware so that a
// handler's refusal and a middleware's are the same bytes.
func fail(c *gin.Context, status int, message string) {
	middleware.WriteError(c, status, message)
}

func badRequest(c *gin.Context, message string) {
	fail(c, http.StatusBadRequest, message)
}

func notFound(c *gin.Context, message string) {
	fail(c, http.StatusNotFound, message)
}

// serverError responds 500 without exposing internal details to the client.
//
// The cause is written to the log together with the request id, and the client
// only receives that id, which is what the user can quote when reporting a
// problem. Echoing err.Error() would leak driver errors, file paths and other
// internals to whoever happens to be logged in.
//
// A cancelled request is a closed tab or a page navigation, not a fault: it
// happens all the time and reporting it as 500 would hide real failures among
// them.
func serverError(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) {
		slog.Debug("request canceled by the caller",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"rid", middleware.RequestIDFrom(c),
		)
		c.Status(middleware.StatusNotResponded)
		return
	}
	slog.Error("request failed",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"rid", middleware.RequestIDFrom(c),
		"error", err,
	)
	fail(c, http.StatusInternalServerError, "internal server error")
}

// upstreamError maps a DNSPod API call failure onto a response: 429 when DNSPod
// asks us to stop, 404 for a record that is not there, and 502 for the rest -
// 500 would claim our own code broke, 400 would blame the caller. The detail
// goes to the log under the request id we hand back.
func upstreamError(c *gin.Context, err error) {
	switch {
	case dnspod.IsRateLimited(err):
		fail(c, http.StatusTooManyRequests, "DNSPod rate limit reached, retry later")
	case dnspod.IsRecordNotFound(err):
		notFound(c, "record not found")
	default:
		slog.Error("DNSPod call failed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"rid", middleware.RequestIDFrom(c),
			"error", err,
		)
		fail(c, http.StatusBadGateway, "DNSPod call failed")
	}
}

// upstreamMessage is the wording one item of a failed batch gets. A batch
// answers per item rather than with one status, so the response is written by
// the caller; the cause still goes to the log under the request id the reply
// carries.
func upstreamMessage(c *gin.Context, err error) string {
	slog.Error("DNSPod call failed",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"rid", middleware.RequestIDFrom(c),
		"error", err,
	)
	if dnspod.IsRateLimited(err) {
		return "DNSPod rate limit reached, retry later"
	}
	return "DNSPod call failed"
}

// accountError names an account problem in words the caller can act on: no
// such id, or no account configured yet. Nothing in it is internal - the id is
// the one the request sent - so the message is returned to the caller rather
// than logged away.
type accountError struct{ msg string }

func (e *accountError) Error() string { return e.msg }

// credentialError answers for a failure to build a DNSPod client.
//
// A missing account is the caller's mistake and is returned as a 400 with its
// own words. Everything else here is our machinery failing to start - a key we
// cannot decrypt, a connection we cannot open - and is logged while the reply
// carries only the request id. No DNSPod call is involved, so upstreamError
// does not apply.
func credentialError(c *gin.Context, err error) {
	var acct *accountError
	if errors.As(err, &acct) {
		badRequest(c, acct.msg)
		return
	}
	serverError(c, err)
}

// bindJSON decodes the request body and writes the failure itself, so every
// caller does not repeat the check.
//
// An oversized body is told apart from a malformed one: a caller who sent too
// much can trim and retry, and a 400 that means "your JSON is broken" sends
// them looking in the wrong place.
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			// The limit comes from the error rather than from the configured
			// default, so the message stays true if a route ever sets its own.
			fail(c, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body is larger than %d bytes", tooLarge.Limit))
			return false
		}
		badRequest(c, "invalid request body")
		return false
	}
	return true
}
