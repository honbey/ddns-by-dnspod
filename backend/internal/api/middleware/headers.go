package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SecureHeaders sets the response headers that keep a browser from helping an
// attacker.
//
// The obvious gap a server that only answers JSON might think it does not have
// is the one that matters most here: the Bearer header is attached by the
// browser automatically, so the whole UI is drivable from inside an iframe, and
// a click there is a click on "delete this record" or "issue this certificate"
// with a valid token. X-Frame-Options and frame-ancestors close that. The rest
// keep the browser from second-guessing the content it is given.
//
// idpOrigins are the identity provider's own origins, which the policy has to
// let through: signing in, the silent renew, and the token revocation all talk
// to the provider from the page, so a policy of self-only would break the very
// sign-in it is not meant to touch. Pass none when there is no provider.
func SecureHeaders(idpOrigins ...string) gin.HandlerFunc {
	external := ""
	if len(idpOrigins) > 0 {
		external = " " + strings.Join(idpOrigins, " ")
	}
	csp := "default-src 'self'; script-src 'self'; " +
		// 'unsafe-inline' is for the style attributes Tailwind emits; scripts
		// stay pinned to 'self', which is the half that matters for XSS.
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		// Fonts are bundled assets; data: stays out so an inlined font is a
		// build bug that shows up here instead of a silent fallback.
		"font-src 'self'; " +
		"connect-src 'self'" + external + "; " +
		"frame-src 'self'" + external + "; " +
		"frame-ancestors 'none'; base-uri 'self'; " +
		// form-action does not fall back to default-src: a form added later
		// without it could post anywhere. object-src closes plugin content.
		"form-action 'self'; object-src 'none'"

	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		// Nothing this app serves needs the browser's sensors or its location.
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// Keep this document out of other origins' windows and other origins'
		// responses out of this one: the two halves of cross-origin isolation.
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		c.Next()
	}
}

// HSTS asks a browser to only ever reach this origin over TLS.
//
// It is opt-in and only correct behind HTTPS: on a plain HTTP deployment the
// header would lock users out of a host that cannot answer TLS at all. Enable
// it when TLS terminates in front of this process - which is the production
// shape.
func HSTS(enabled bool) gin.HandlerFunc {
	const value = "max-age=31536000; includeSubDomains"
	return func(c *gin.Context) {
		if enabled {
			c.Writer.Header().Set("Strict-Transport-Security", value)
		}
		c.Next()
	}
}
