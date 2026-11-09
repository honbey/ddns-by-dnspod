// Package middleware provides HTTP middlewares.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/auth"
	"ddns-by-dnspod/internal/metrics"
)

// TokenVerifier is what Bearer needs from the identity provider. *auth.Verifier
// satisfies it; tests hand in a stub instead, so the middleware is checked
// without a provider or a signing key.
type TokenVerifier interface {
	Enabled() bool
	// DevSub is the subject impersonated while verification is off, which is
	// local development. See config.OIDCConfig.Disabled.
	DevSub() string
	Verify(ctx context.Context, raw string) (*auth.Identity, error)
}

// ContextUserKey and ContextGroupsKey are the gin context keys holding the identity
const (
	ContextUserKey   = "user"
	ContextGroupsKey = "groups"
)

// fallbackDevSub covers a nil verifier, which only a test hands in; a real one
// carries AUTH_DISABLED_SUB.
const fallbackDevSub = "dev-user"

// Bearer authenticates a request by the access token the browser sends. Nothing
// is kept afterwards, which is what makes the server stateless.
//
// The reason for a rejection stays in the log: a token error message is a hint
// about the provider's configuration, so the client only gets the request id.
func Bearer(v TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if v == nil {
			c.Set(ContextUserKey, fallbackDevSub)
			c.Set(ContextGroupsKey, []string{})
			c.Next()
			return
		}
		if !v.Enabled() {
			c.Set(ContextUserKey, v.DevSub())
			c.Set(ContextGroupsKey, []string{})
			c.Next()
			return
		}

		source := SourceIP(c)
		raw := c.GetHeader("Authorization")
		// A source past its budget gets no more verification work on the
		// expensive path, except for a token already proven valid and one
		// bounded probe per interval. The opaque check is a userinfo round
		// trip; answering 429 only after paying for it would leave the issuer's
		// load unbounded.
		if auth.IsOpaqueToken(raw) {
			known := false
			if cache, ok := v.(interface{ KnownOpaque(string) bool }); ok {
				known = cache.KnownOpaque(raw)
			}
			if !known && !allowOpaqueAttempt(source) {
				writeTooManyAttempts(c)
				return
			}
		}

		id, err := v.Verify(auth.WithSource(c.Request.Context(), source), raw)
		if err != nil {
			recordReject(source)

			// Past its budget this source is answered without the ceremony:
			// no rejection warning, no request id, nothing for a flood to
			// amplify.
			if overRejectBudget(source) {
				writeTooManyAttempts(c)
				return
			}

			slog.Warn("rejected a request", "rid", RequestIDFrom(c), "error", err)
			// A refused token costs a log line at least and, on the opaque
			// path, a round trip to the issuer - so it is worth a counter of
			// its own: a rising rate is either a misconfiguration or somebody
			// guessing.
			metrics.IncAuthRejection("invalid")
			WriteError(c, http.StatusUnauthorized, "the access token was rejected")
			return
		}

		c.Set(ContextUserKey, id.Username)
		c.Set(ContextGroupsKey, id.Groups)
		c.Next()
	}
}

// CurrentUser returns the current username from the gin context, flattened into
// something safe to print.
//
// A dozen audit lines format this with %s, and the value is not necessarily a
// bare name: an API key's name is chosen by whoever created it, and an identity
// claim can be a display name the user edits. A newline in either would forge
// a log line, and the C0 controls would rewrite the terminal showing it, so the
// value is reduced here rather than at each of those call sites.
func CurrentUser(c *gin.Context) string {
	if v, ok := c.Get(ContextUserKey); ok {
		if s, ok := v.(string); ok {
			return sanitizeLogValue(s)
		}
	}
	return ""
}

// sanitizeLogValue replaces what could break a log line: newlines and tabs
// become spaces, and the remaining control characters are dropped rather than
// truncated, since half an escape sequence is worse than none.
func sanitizeLogValue(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r == '\u2028' || r == '\u2029':
			// A line and a paragraph separator: not control characters, but
			// several log viewers break the line on them anyway.
			return ' '
		case unicode.IsControl(r):
			// Cc covers C0, DEL and the C1 block (U+0080-U+009F): the last two are
			// legal UTF-8, so they arrive intact and a log viewer's terminal acts
			// on them - U+0085 reads as a line break, U+009B introduces a
			// control sequence.
			return -1
		}
		return r
	}, s)
}
