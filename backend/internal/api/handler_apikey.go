package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
)

// apiKeyInput is the payload for creating or updating an API key
type apiKeyInput struct {
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	AccountID uint   `json:"account_id"`
	Enabled   *bool  `json:"enabled"`
}

// listAPIKeys lists every key. Only the display prefix is exposed, never the secret.
func (s *Server) listAPIKeys(c *gin.Context) {
	var keys []model.APIKey
	params, total, fine := pageRows(c, s.store.DB(), &keys)
	if !fine {
		return
	}
	paged(c, keys, total, params)
}

// createAPIKey mints a new key. The plaintext is returned by this call and by
// no other, it is not recoverable from the database afterwards.
func (s *Server) createAPIKey(c *gin.Context) {
	var input apiKeyInput
	if !bindJSON(c, &input) {
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		badRequest(c, "name is required")
		return
	}
	input.Domain = strings.TrimSpace(input.Domain)
	if input.Domain == "" {
		badRequest(c, "domain is required")
		return
	}
	// A key is bound to one domain, or to every domain of its account through
	// the wildcard the form offers. Anything else has to be a name that can
	// match a request path: the value is logged and compared downstream, and a
	// key that can never match is a key that can never work.
	if input.Domain != model.APIKeyWildcard {
		if err := model.ValidateDomain(input.Domain); err != nil {
			badRequest(c, err.Error())
			return
		}
	}

	// The account has to exist and its credentials must be readable, otherwise
	// the key would be handed out knowing it can never work
	if err := s.store.DB().First(&model.Account{}, input.AccountID).Error; err != nil {
		badRequest(c, "account not found")
		return
	}
	if _, err := s.clientFor(input.AccountID); err != nil {
		// Building the client means reading and decrypting the stored
		// credential, so the failure text belongs in the log only
		slog.Error("credentials of this account are unusable", "key", input.Name,
			"rid", middleware.RequestIDFrom(c), "error", err)
		badRequest(c, "credentials of this account are unusable")
		return
	}

	plaintext, hash, prefix, err := middleware.NewAPIKey()
	if err != nil {
		serverError(c, err)
		return
	}

	key := model.APIKey{
		Name:      input.Name,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Domain:    input.Domain,
		AccountID: input.AccountID,
		Enabled:   true,
	}
	if err := s.store.DB().Create(&key).Error; err != nil {
		serverError(c, err)
		return
	}

	slog.Info("API key created", "key", key.Name, "domain", key.Domain,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))

	created(c, gin.H{"key": plaintext, "api_key": key})
}

// updateAPIKey renames a key or turns it on and off.
//
// The domain and the account of an existing key are deliberately frozen: moving
// an already handed out key to another domain would silently change what a
// leaked key can reach. Delete it and mint a new one instead.
func (s *Server) updateAPIKey(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var key model.APIKey
	if err := s.store.DB().First(&key, id).Error; err != nil {
		notFound(c, "API key not found")
		return
	}

	var input apiKeyInput
	if !bindJSON(c, &input) {
		return
	}

	if name := strings.TrimSpace(input.Name); name != "" {
		key.Name = name
	}
	if input.Enabled != nil {
		key.Enabled = *input.Enabled
	}

	if err := s.store.DB().Model(&model.APIKey{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":    key.Name,
			"enabled": key.Enabled,
		}).Error; err != nil {
		serverError(c, err)
		return
	}

	ok(c, key)
}

// deleteAPIKey revokes a key
func (s *Server) deleteAPIKey(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	tx := s.store.DB().Delete(&model.APIKey{}, id)
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	// GORM reports no error when the row is already gone, so the affected row
	// count is what tells us whether the key ever existed
	if tx.RowsAffected == 0 {
		notFound(c, "API key not found")
		return
	}

	slog.Info("API key deleted", "key", id,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, gin.H{"deleted": id})
}

// apiKeyDisabled is the response used when the public API is switched off
func apiKeyDisabled(c *gin.Context) {
	fail(c, http.StatusServiceUnavailable, "public API is disabled")
}
