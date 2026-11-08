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

// envelope is the success shape.
type envelope struct {
	Data any `json:"data"`
}

func ok(c *gin.Context, data any) {
	middleware.WriteJSON(c, http.StatusOK, envelope{Data: data})
}

func created(c *gin.Context, data any) {
	middleware.WriteJSON(c, http.StatusCreated, envelope{Data: data})
}

// fail delegates to the middleware so handlers and middleware write the same bytes.
func fail(c *gin.Context, status int, message string) {
	middleware.WriteError(c, status, message)
}

// failWithCode adds the contract's optional code.
func failWithCode(c *gin.Context, status int, code, message string) {
	middleware.WriteCodedError(c, status, code, message)
}

func badRequest(c *gin.Context, message string) {
	fail(c, http.StatusBadRequest, message)
}

func notFound(c *gin.Context, message string) {
	fail(c, http.StatusNotFound, message)
}

// serverError logs the cause and returns only the request id. A cancelled
// request is a closed tab, not a fault.
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

// upstreamError maps 429 / 404 / 502; the detail goes to the log.
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

// upstreamMessage is the wording one item of a failed batch gets.
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

// CodeNoAccount is the one refusal a client keys on. The message may change;
// this string is the contract.
const CodeNoAccount = "no_account"

type accountError struct {
	msg  string
	code string
}

func (e *accountError) Error() string { return e.msg }

// credentialError returns account errors as 400; everything else is a 500.
func credentialError(c *gin.Context, err error) {
	var acct *accountError
	if errors.As(err, &acct) {
		failWithCode(c, http.StatusBadRequest, acct.code, acct.msg)
		return
	}
	serverError(c, err)
}

// bindJSON decodes the body and writes the failure itself.
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(c, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body is larger than %d bytes", tooLarge.Limit))
			return false
		}
		badRequest(c, "invalid request body")
		return false
	}
	return true
}
