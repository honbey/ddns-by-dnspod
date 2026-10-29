package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ddns-by-dnspod/internal/acme"
	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
)

// certificateNameRe is what a certificate may be called. The name becomes a
// directory name under the configured root, so it is kept to characters a file
// name carries without quoting - and no leading dot, which would hide it.
var certificateNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// maxCertificateNames caps how many names one certificate may carry. The CA's
// own cap is higher, but a form offering twenty is already past what anyone
// types by hand.
const maxCertificateNames = 20

// certificateInput is the payload for creating or updating a certificate.
// OutputDir and the booleans are pointers so "not sent" can be told apart from
// an empty value, which is how a field is cleared or turned off.
type certificateInput struct {
	Name          string   `json:"name"`
	AccountID     uint     `json:"account_id"`
	Domains       []string `json:"domains"`
	KeyType       string   `json:"key_type"`
	OutputDir     *string  `json:"output_dir"`
	AutoRenew     *bool    `json:"auto_renew"`
	NotifyOnIssue *bool    `json:"notify_on_issue"`
}

// certificateView is a certificate as the API hands it out: the row, plus the
// names as a list and where the files are. The path depends on the configured
// root, which only the server knows, and it is the one thing a person needs to
// mount into whatever reads the certificate.
type certificateView struct {
	ID            uint       `json:"id"`
	Name          string     `json:"name"`
	AccountID     uint       `json:"account_id"`
	Domains       []string   `json:"domains"`
	KeyType       string     `json:"key_type"`
	OutputDir     string     `json:"output_dir"`
	OutputPath    string     `json:"output_path"`
	AutoRenew     bool       `json:"auto_renew"`
	NotifyOnIssue bool       `json:"notify_on_issue"`
	LastIssuedAt  *time.Time `json:"last_issued_at"`
	NotAfter      *time.Time `json:"not_after"`
	LastStatus    string     `json:"last_status"`
	LastMessage   string     `json:"last_message"`
	FailCount     int        `json:"fail_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// certificateViewOf renders one row for the API, including the directory its
// files are written to.
func (s *Server) certificateViewOf(cert *model.Certificate) certificateView {
	names, err := model.CertificateNames(cert.Domains)
	if err != nil {
		// A row whose domain list cannot be read is still worth showing: it is
		// the row someone has to fix.
		slog.Error("could not read the domains of this account", "certificate", cert.ID, "error", err)
		names = nil
	}
	if names == nil {
		names = []string{}
	}
	return certificateView{
		ID:            cert.ID,
		Name:          cert.Name,
		AccountID:     cert.AccountID,
		Domains:       names,
		KeyType:       cert.KeyType,
		OutputDir:     cert.OutputDir,
		OutputPath:    s.acme.OutputPath(cert),
		AutoRenew:     cert.AutoRenew,
		NotifyOnIssue: cert.NotifyOnIssue,
		LastIssuedAt:  cert.LastIssuedAt,
		NotAfter:      cert.NotAfter,
		LastStatus:    cert.LastStatus,
		LastMessage:   cert.LastMessage,
		FailCount:     cert.FailCount,
		CreatedAt:     cert.CreatedAt,
		UpdatedAt:     cert.UpdatedAt,
	}
}

// listCertificates lists the certificates. The files are not touched: what is
// on disk is only ever written by an issuance.
func (s *Server) listCertificates(c *gin.Context) {
	var certs []model.Certificate
	params, total, fine := pageRows(c, s.store.DB(), &certs)
	if !fine {
		return
	}
	views := make([]certificateView, 0, len(certs))
	for i := range certs {
		views = append(views, s.certificateViewOf(&certs[i]))
	}
	paged(c, views, total, params)
}

// createCertificate stores a certificate. Nothing is issued yet: the first
// issuance is the button, or the daily check when automatic renewal is on.
func (s *Server) createCertificate(c *gin.Context) {
	var input certificateInput
	if !bindJSON(c, &input) {
		return
	}

	name := strings.TrimSpace(input.Name)
	if !certificateNameRe.MatchString(name) {
		badRequest(c, "the name may hold letters, digits, dot, dash and underscore only")
		return
	}
	domains, err := normaliseCertificateNames(input.Domains)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	keyType := strings.TrimSpace(input.KeyType)
	if keyType == "" {
		keyType = acme.KeyEC256
	}
	if !acme.ValidKeyType(keyType) {
		badRequest(c, fmt.Sprintf("key_type must be one of %s", strings.Join(acme.KeyTypes, ", ")))
		return
	}
	output, err := normaliseOutputDir(input.OutputDir)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	// The directory is compared as the manager resolves it, not as it was
	// typed: an empty output_dir means "named after the certificate", which is
	// exactly how two rows end up sharing a directory without either of them
	// naming it.
	candidate := &model.Certificate{Name: name, OutputDir: output}
	if s.rejectDirectoryConflict(c, candidate, 0) {
		return
	}
	account, err := s.resolveAccount(input.AccountID)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	encoded, err := model.EncodeCertificateNames(domains)
	if err != nil {
		serverError(c, err)
		return
	}
	autoRenew := true
	if input.AutoRenew != nil {
		autoRenew = *input.AutoRenew
	}
	notify := true
	if input.NotifyOnIssue != nil {
		notify = *input.NotifyOnIssue
	}

	cert := model.Certificate{
		Name:          name,
		AccountID:     account.ID,
		Domains:       encoded,
		KeyType:       keyType,
		OutputDir:     output,
		AutoRenew:     autoRenew,
		NotifyOnIssue: notify,
	}
	if err := s.store.DB().Create(&cert).Error; err != nil {
		if isUniqueViolation(err) {
			fail(c, http.StatusConflict, "a certificate with this name already exists")
			return
		}
		serverError(c, err)
		return
	}

	slog.Info("certificate created", "certificate", cert.ID, "name", cert.Name,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	created(c, s.certificateViewOf(&cert))
}

// updateCertificate changes a certificate. The files already on disk are left
// where they are; the next issuance writes to wherever this now points.
func (s *Server) updateCertificate(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}
	var cert model.Certificate
	if err := s.store.DB().First(&cert, id).Error; err != nil {
		notFound(c, "certificate not found")
		return
	}

	var input certificateInput
	if !bindJSON(c, &input) {
		return
	}

	updates := make(map[string]any)
	if name := strings.TrimSpace(input.Name); name != "" {
		if !certificateNameRe.MatchString(name) {
			badRequest(c, "the name may hold letters, digits, dot, dash and underscore only")
			return
		}
		updates["name"] = name
	}
	if input.Domains != nil {
		domains, err := normaliseCertificateNames(input.Domains)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		encoded, err := model.EncodeCertificateNames(domains)
		if err != nil {
			serverError(c, err)
			return
		}
		updates["domains"] = encoded
	}
	if input.KeyType != "" {
		keyType := strings.TrimSpace(input.KeyType)
		if !acme.ValidKeyType(keyType) {
			badRequest(c, fmt.Sprintf("key_type must be one of %s", strings.Join(acme.KeyTypes, ", ")))
			return
		}
		updates["key_type"] = keyType
	}
	if input.AccountID > 0 && input.AccountID != cert.AccountID {
		// Checked like the task endpoints do it: writing the id straight
		// through would leave the row pointing at an account that is not there,
		// and the challenge would fail on every run.
		if err := s.store.DB().First(&model.Account{}, input.AccountID).Error; err != nil {
			badRequest(c, "account not found")
			return
		}
		updates["account_id"] = input.AccountID
	}
	if input.OutputDir != nil {
		output, err := normaliseOutputDir(input.OutputDir)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		updates["output_dir"] = output
	}
	if input.AutoRenew != nil {
		updates["auto_renew"] = *input.AutoRenew
	}
	if input.NotifyOnIssue != nil {
		updates["notify_on_issue"] = *input.NotifyOnIssue
	}

	// The directory is compared as the manager would resolve it, from whatever
	// this update leaves the row with: renaming moves the default directory
	// along with the name, which is another way two rows end up sharing one.
	candidate := &model.Certificate{
		Name:      cert.Name,
		OutputDir: cert.OutputDir,
	}
	if v, ok := updates["name"]; ok {
		candidate.Name = v.(string)
	}
	if v, ok := updates["output_dir"]; ok {
		candidate.OutputDir = v.(string)
	}
	if s.rejectDirectoryConflict(c, candidate, cert.ID) {
		return
	}

	if len(updates) > 0 {
		// Only the editable columns are written: the manager runs in parallel
		// and owns last_status, not_after and the rest of the run state.
		if err := s.store.DB().Model(&model.Certificate{}).Where("id = ?", cert.ID).
			Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				fail(c, http.StatusConflict, "a certificate with this name already exists")
				return
			}
			serverError(c, err)
			return
		}
	}

	// Read the row back: an issuance may have written its result in the
	// meantime, and echoing what was loaded would report stale values.
	var updated model.Certificate
	if err := s.store.DB().First(&updated, cert.ID).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, s.certificateViewOf(&updated))
}

// deleteCertificate removes the row. The files stay where they are: something
// out there is configured to read them, and deleting the record of a
// certificate is not the same as knowing nobody wants it any more.
func (s *Server) deleteCertificate(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}
	tx := s.store.DB().Delete(&model.Certificate{}, id)
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	if tx.RowsAffected == 0 {
		notFound(c, "certificate not found")
		return
	}
	slog.Info("certificate deleted, its files were left on disk", "certificate", id,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, gin.H{"deleted": id})
}

// issueCertificate starts an issuance and answers at once.
//
// The run is asynchronous because it outlives a request: the CA has to validate
// a challenge and answers when it is done, which takes tens of seconds, while
// the HTTP server gives up after one. The UI watches last_status - RUNNING
// first, then OK or FAIL.
func (s *Server) issueCertificate(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}
	if !s.acme.Enabled() {
		fail(c, http.StatusServiceUnavailable, "the certificate manager is disabled")
		return
	}

	var cert model.Certificate
	if err := s.store.DB().First(&cert, id).Error; err != nil {
		notFound(c, "certificate not found")
		return
	}
	if err := s.acme.Start(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notFound(c, "certificate not found")
			return
		}
		// Already running is a conflict rather than a failure: the run the
		// caller asked for is the one in flight.
		fail(c, http.StatusConflict, "an issuance for this certificate is already running")
		return
	}

	slog.Info("certificate issuance started", "certificate", id, "name", cert.Name,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))

	// Read the row back so the answer carries the RUNNING state, which is what
	// tells the caller there is something to watch.
	if err := s.store.DB().First(&cert, id).Error; err != nil {
		serverError(c, err)
		return
	}
	middleware.WriteJSON(c, http.StatusAccepted, gin.H{"data": s.certificateViewOf(&cert)})
}

// rejectDirectoryConflict answers the request with a conflict when another
// certificate already resolves to the directory the candidate would write to,
// and with an internal error when the check itself cannot run; it reports
// whether the request has been answered.
//
// Two rows sharing a directory would silently overwrite each other's files: the
// later issuance replaces the earlier one's certificate and key with a pair the
// earlier row's names say nothing about. The comparison is on resolved paths -
// an empty output_dir means named after the certificate - and the table is
// small and listed in full anyway, so reading it whole costs nothing. The check
// is read-then-write with nothing behind it, so two concurrent creates can both
// pass; that window is the same one deleteAccount accepts.
func (s *Server) rejectDirectoryConflict(c *gin.Context, candidate *model.Certificate, editID uint) bool {
	var others []model.Certificate
	if err := s.store.DB().Where("id <> ?", editID).Find(&others).Error; err != nil {
		serverError(c, err)
		return true
	}

	candidatePath := s.acme.OutputPath(candidate)
	for i := range others {
		if s.acme.OutputPath(&others[i]) == candidatePath {
			fail(c, http.StatusConflict, fmt.Sprintf(
				"another certificate already writes to %s; give this one a different directory or name",
				candidatePath))
			return true
		}
	}
	return false
}

// normaliseCertificateNames lowercases the names, drops duplicates and checks
// each one, wildcard included: "*." covers the base name and everything below
// it, and what follows has to be a name this deployment could be asked for.
func normaliseCertificateNames(names []string) ([]string, error) {
	if len(names) > maxCertificateNames {
		return nil, fmt.Errorf("at most %d domains per certificate", maxCertificateNames)
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		// A wildcard certificate is ordered for the base name; the star is the
		// only thing the validator below would refuse.
		if err := model.ValidateDomain(strings.TrimPrefix(name, "*.")); err != nil {
			return nil, err
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}
	return out, nil
}

// normaliseOutputDir checks the subdirectory a certificate writes to.
//
// The path is joined onto the configured root, so it has to stay inside it: an
// absolute path or a ".." would let a signed-in user write a certificate - and
// a private key - anywhere this process may write.
func normaliseOutputDir(sub *string) (string, error) {
	if sub == nil {
		return "", nil
	}
	trimmed := strings.TrimSpace(*sub)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("output_dir must be a relative path without backslashes")
	}
	cleaned := filepath.Clean(trimmed)
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("output_dir must be a relative path inside the certificate directory")
	}
	return cleaned, nil
}
