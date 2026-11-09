package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ddns-by-dnspod/internal/model"
)

// ContextAPIKeyKey is the gin context key holding the key that authenticated the request
const ContextAPIKeyKey = "api_key"

// Key length and alphabet
const (
	keyPrefix    = "ddp_"
	keyRandomLen = 32
	// keyDisplayPrefixLen is how much of the key is kept for display in the UI
	keyDisplayPrefixLen = 12
)

// ErrNoAPIKey is returned when the request carries no usable credential
var ErrNoAPIKey = errors.New("no api key present")

// NewAPIKey builds a random key and its stored form.
//
// The plaintext is handed back to the caller exactly once, at creation time;
// only the SHA-256 hash and a short display prefix are persisted.
func NewAPIKey() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, keyRandomLen)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", err
	}
	plaintext = keyPrefix + hex.EncodeToString(buf)
	return plaintext, HashAPIKey(plaintext), DisplayPrefix(plaintext), nil
}

// HashAPIKey is the canonical stored form of a key
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// DisplayPrefix returns the part of a key that is safe to show in a list,
// enough to tell two keys apart without revealing the secret.
func DisplayPrefix(plaintext string) string {
	if len(plaintext) <= keyDisplayPrefixLen {
		return plaintext
	}
	return plaintext[:keyDisplayPrefixLen]
}

// APIKeyAuth authenticates a request against the api_keys table.
//
// It is meant for the public API under /api/v1, which sits outside the reverse
// proxy single sign-on: the caller is a script, not a browser session, so there
// is no identity header to trust. Source IPs are deliberately not checked, the
// whole point is that these calls come from the internet.
func APIKeyAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		plaintext, err := KeyFromRequest(c)
		if err != nil {
			unauthorized(c, "missing API key")
			return
		}

		var key model.APIKey
		err = db.Where("key_hash = ?", HashAPIKey(plaintext)).First(&key).Error
		if err != nil {
			unauthorized(c, "invalid API key")
			return
		}
		if !key.Enabled {
			unauthorized(c, "API key is disabled")
			return
		}

		c.Set(ContextAPIKeyKey, &key)
		// Existing handlers log the acting user, so the key shows up there too
		c.Set(ContextUserKey, "apikey:"+key.Name)

		// Auditing must never break authentication. The write is throttled to
		// once a minute: this path is hit by every single API call, and an
		// UPDATE per request turns the read-heavy path into a write-heavy one
		// for a column whose granularity nobody cares about.
		//
		// The threshold is part of the UPDATE rather than an earlier read: every
		// concurrent request observed the same stale LastUsedAt, so each of them
		// passed the comparison and wrote, which made the throttle a no-op on a
		// busy key.
		now := time.Now()
		if err := db.Model(&model.APIKey{}).
			Where("id = ? AND (last_used_at IS NULL OR last_used_at < ?)", key.ID, now.Add(-time.Minute)).
			Update("last_used_at", now).Error; err != nil {
			fmt.Fprintf(gin.DefaultErrorWriter,
				"[api] update API key last_used_at (rid=%s): %v\n", RequestIDFrom(c), err)
		}

		c.Next()
	}
}

// APIKeyDomain rejects calls whose domain differs from the one the key is bound to.
//
// This is the only thing keeping a key from reaching the other domains of the
// same account, so it lives in a middleware rather than in each handler. A key
// issued with the wildcard scope is let through everywhere by design: that
// scope was chosen when the key was created, and it is the only way to cover an
// account whose domains are not known in advance.
func APIKeyDomain() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := APIKeyFrom(c)
		if !ok {
			// Not an API key call, the normal auth path already handled it
			c.Next()
			return
		}
		if !model.APIKeyCovers(key.Domain, c.Param("domain")) {
			abortJSON(c, http.StatusForbidden, "API key is not scoped to this domain")
			return
		}
		c.Next()
	}
}

// APIKeyFrom returns the key that authenticated the request
func APIKeyFrom(c *gin.Context) (*model.APIKey, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(ContextAPIKeyKey)
	if !ok {
		return nil, false
	}
	key, ok := value.(*model.APIKey)
	return key, ok
}

// KeyFromRequest pulls the credential out of the request, accepting both the
// standard Authorization header and the simpler X-Api-Key one.
func KeyFromRequest(c *gin.Context) (string, error) {
	if raw := strings.TrimSpace(c.GetHeader("Authorization")); raw != "" {
		scheme, token, found := strings.Cut(raw, " ")
		if found && strings.EqualFold(scheme, "Bearer") {
			if token = strings.TrimSpace(token); token != "" {
				return token, nil
			}
		}
	}
	if raw := strings.TrimSpace(c.GetHeader("X-Api-Key")); raw != "" {
		return raw, nil
	}
	return "", ErrNoAPIKey
}

func unauthorized(c *gin.Context, msg string) {
	c.Header("WWW-Authenticate", "Bearer")
	abortJSON(c, http.StatusUnauthorized, msg)
}

// abortJSON is the shared refusal writer: these rejections are the ones most
// often reported back, and without the request id there is no way to find their
// log line.
func abortJSON(c *gin.Context, status int, msg string) {
	WriteError(c, status, msg)
}
