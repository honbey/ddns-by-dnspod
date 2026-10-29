package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/model"
)

// isUniqueViolation reports whether an error is a unique index rejecting a
// duplicate.
//
// Matched on the driver's wording rather than gorm.ErrDuplicatedKey: the store
// does not turn on TranslateError, so the code never reaches GORM as a sentinel.
// Turning that setting on is a store-wide decision, and until it is made this is
// the only signal available - the coupling it leaves is why the check lives here
// rather than being spread over the call sites.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// accountInput is the payload for creating or updating an account.
// IsDefault is a pointer so "not sent" can be told apart from an explicit false,
// which is what lets a user take the default flag back off an account.
type accountInput struct {
	Name      string `json:"name"`
	SecretID  string `json:"secret_id"`
	SecretKey string `json:"secret_key"`
	IsDefault *bool  `json:"is_default"`
}

// listAccounts lists accounts. SecretKey is tagged json:"-" so it is never exposed.
func (s *Server) listAccounts(c *gin.Context) {
	var accounts []model.Account
	params, total, fine := pageRows(c, s.store.DB(), &accounts)
	if !fine {
		return
	}
	paged(c, accounts, total, params)
}

// createAccount creates an account, storing SecretKey encrypted
func (s *Server) createAccount(c *gin.Context) {
	var input accountInput
	if !bindJSON(c, &input) {
		return
	}
	if input.Name == "" || input.SecretID == "" || input.SecretKey == "" {
		badRequest(c, "name, secret_id and secret_key are required")
		return
	}

	encrypted, err := s.store.Encryptor().Encrypt(input.SecretKey)
	if err != nil {
		serverError(c, err)
		return
	}

	isDefault := input.IsDefault != nil && *input.IsDefault

	account := model.Account{
		Name:      input.Name,
		SecretID:  input.SecretID,
		SecretKey: encrypted,
		KeyHint:   model.MaskSecret(input.SecretKey),
		IsDefault: isDefault,
	}

	// Clearing the flag and inserting the new default belong in one transaction,
	// otherwise a crash in between leaves either two defaults or none at all
	if err := s.store.DB().Transaction(func(tx *gorm.DB) error {
		if isDefault {
			if err := tx.Model(&model.Account{}).Where("is_default = ?", true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&account).Error
	}); err != nil {
		// The name is the caller's to change, so this is a conflict rather than a
		// broken server, and it must not read as "retry, it may pass".
		if isUniqueViolation(err) {
			fail(c, http.StatusConflict, "an account with this name already exists")
			return
		}
		serverError(c, err)
		return
	}

	slog.Info("account created", "name", account.Name,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	created(c, account)
}

// updateAccount updates an account. An empty SecretKey means the value stays unchanged.
func (s *Server) updateAccount(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var account model.Account
	if err := s.store.DB().First(&account, id).Error; err != nil {
		notFound(c, "account not found")
		return
	}

	var input accountInput
	if !bindJSON(c, &input) {
		return
	}

	if input.Name != "" {
		account.Name = input.Name
	}
	if input.SecretID != "" {
		account.SecretID = input.SecretID
	}
	if input.SecretKey != "" {
		encrypted, err := s.store.Encryptor().Encrypt(input.SecretKey)
		if err != nil {
			serverError(c, err)
			return
		}
		account.SecretKey = encrypted
		account.KeyHint = model.MaskSecret(input.SecretKey)
	}
	if input.IsDefault != nil {
		account.IsDefault = *input.IsDefault
	}

	// Same reasoning as on create: the flag swap must not be visible half done
	if err := s.store.DB().Transaction(func(tx *gorm.DB) error {
		if account.IsDefault {
			if err := tx.Model(&model.Account{}).Where("is_default = ?", true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&account).Error
	}); err != nil {
		// The same collision as on create, and the same answer: renaming onto a
		// name that is taken is a conflict the caller can resolve.
		if isUniqueViolation(err) {
			fail(c, http.StatusConflict, "an account with this name already exists")
			return
		}
		serverError(c, err)
		return
	}
	// Drop the cached client so the next request uses the new credentials
	s.clients.remove(account.ID)
	ok(c, account)
}

// deleteAccount deletes an account, refusing while DDNS tasks or certificates
// still reference it.
func (s *Server) deleteAccount(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var account model.Account
	if err := s.store.DB().First(&account, id).Error; err != nil {
		notFound(c, "account not found")
		return
	}

	// The count is read before the transaction rather than inside it. A deferred
	// SQLite transaction that reads before it writes cannot always upgrade its
	// lock: if another connection commits in between, the upgrade returns
	// SQLITE_BUSY without ever consulting busy_timeout, which the DSN does set.
	// That leaves a window - a task created right after this check - and it is
	// accepted: nothing constrains such a row anyway, because neither DDNSTask
	// nor Certificate declares an association to the account it names.
	//
	// The delete and the switching off of the account's keys share one
	// transaction, so an account never lingers without its keys being closed: a
	// key whose account no longer exists still authenticates and then fails
	// deeper in, at every call.
	var tasks, certificates int64
	// The query errors must be checked: a failed count leaves 0, which would look like "no references"
	if err := s.store.DB().Model(&model.DDNSTask{}).Where("account_id = ?", id).
		Count(&tasks).Error; err != nil {
		serverError(c, err)
		return
	}
	if err := s.store.DB().Model(&model.Certificate{}).Where("account_id = ?", id).
		Count(&certificates).Error; err != nil {
		serverError(c, err)
		return
	}
	switch {
	case tasks > 0:
		badRequest(c, "this account is still referenced by DDNS tasks, update or delete them first")
		return
	case certificates > 0:
		// A certificate left pointing at an account that is gone fails on every
		// run and announces it every day, which is worse than saying so here.
		badRequest(c, "this account is still referenced by certificates, update or delete them first")
		return
	}

	err := s.store.DB().Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.Account{}, id)
		if res.Error != nil {
			return res.Error
		}
		// GORM reports no error when the row is already gone, so the affected
		// row count is what tells us whether the account survived the reference
		// check
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Model(&model.APIKey{}).Where("account_id = ?", id).
			Update("enabled", false).Error
	})

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		notFound(c, "account not found")
		return
	case err != nil:
		serverError(c, err)
		return
	}

	s.clients.remove(id)
	ok(c, gin.H{"deleted": id})
}

// verifyAccount checks whether the account credentials work
func (s *Server) verifyAccount(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	cli, err := s.clientFor(id)
	if err != nil {
		slog.Error("could not reach DNSPod with this account", "account", id, "rid", middleware.RequestIDFrom(c), "error", err)
		badRequest(c, "cannot reach DNSPod with this account")
		return
	}
	if err := cli.Verify(); err != nil {
		slog.Error("DNSPod rejected this account", "account", id, "rid", middleware.RequestIDFrom(c), "error", err)
		if dnspod.IsRateLimited(err) {
			// Not a verdict on the key: DNSPod never got far enough to form one,
			// so saying "these credentials are wrong" would send the user off to
			// replace a key that is fine.
			fail(c, http.StatusTooManyRequests, "DNSPod rate limit reached, retry later")
			return
		}
		badRequest(c, "DNSPod rejected these credentials")
		return
	}
	ok(c, gin.H{"valid": true})
}
