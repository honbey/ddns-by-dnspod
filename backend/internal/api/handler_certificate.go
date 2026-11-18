package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/idna"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"ddns-by-dnspod/internal/acme"
	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
)

// certificateNameLookup bounds the provider call the name check makes. Timing
// out is not an error the caller sees: the check fails open, and the issuance
// reports the same problem with its own message.
const certificateNameLookup = 8 * time.Second

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
	Name            string   `json:"name"`
	AccountID       uint     `json:"account_id"`
	Domains         []string `json:"domains"`
	KeyType         string   `json:"key_type"`
	OutputDir       *string  `json:"output_dir"`
	RenewCron       *string  `json:"renew_cron"`
	RenewBeforeDays *int     `json:"renew_before_days"`
	NotifyOnIssue   *bool    `json:"notify_on_issue"`
	// AutoRenew is the switch renew_cron replaced. A caller still sending it has
	// not been updated, and ignoring it would leave them with a schedule they
	// never asked for; the field is read only to refuse it.
	AutoRenew *bool `json:"auto_renew"`
}

// certificateView is a certificate as the API hands it out: the row, plus the
// names as a list and where the files are. The path depends on the configured
// root, which only the server knows, and it is the one thing a person needs to
// mount into whatever reads the certificate.
type certificateView struct {
	ID              uint       `json:"id"`
	Name            string     `json:"name"`
	AccountID       uint       `json:"account_id"`
	Domains         []string   `json:"domains"`
	KeyType         string     `json:"key_type"`
	OutputDir       string     `json:"output_dir"`
	OutputPath      string     `json:"output_path"`
	RenewCron       string     `json:"renew_cron"`
	RenewBeforeDays int        `json:"renew_before_days"`
	NotifyOnIssue   bool       `json:"notify_on_issue"`
	LastIssuedAt    *time.Time `json:"last_issued_at"`
	NotAfter        *time.Time `json:"not_after"`
	LastStatus      string     `json:"last_status"`
	LastMessage     string     `json:"last_message"`
	FailCount       int        `json:"fail_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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
		ID:              cert.ID,
		Name:            cert.Name,
		AccountID:       cert.AccountID,
		Domains:         names,
		KeyType:         cert.KeyType,
		OutputDir:       cert.OutputDir,
		OutputPath:      s.acme.OutputPath(cert),
		RenewCron:       cert.RenewCron,
		RenewBeforeDays: cert.RenewBeforeDays,
		NotifyOnIssue:   cert.NotifyOnIssue,
		LastIssuedAt:    cert.LastIssuedAt,
		NotAfter:        cert.NotAfter,
		LastStatus:      cert.LastStatus,
		LastMessage:     cert.LastMessage,
		FailCount:       cert.FailCount,
		CreatedAt:       cert.CreatedAt,
		UpdatedAt:       cert.UpdatedAt,
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
// issuance is the button, or the check when this certificate's own schedule
// comes round.
func (s *Server) createCertificate(c *gin.Context) {
	var input certificateInput
	if !bindJSON(c, &input) {
		return
	}
	if input.AutoRenew != nil {
		badRequest(c, "auto_renew is gone: send renew_cron instead (an empty expression renews by hand)")
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
	if s.rejectUncoveredNames(c, account.ID, domains) {
		return
	}

	encoded, err := model.EncodeCertificateNames(domains)
	if err != nil {
		serverError(c, err)
		return
	}
	renewCron, err := normaliseRenewCron(input.RenewCron, s.acme.Config().DefaultRenewCron())
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	renewBeforeDays, err := normaliseRenewBeforeDays(input.RenewBeforeDays)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	notify := true
	if input.NotifyOnIssue != nil {
		notify = *input.NotifyOnIssue
	}

	cert := model.Certificate{
		Name:            name,
		AccountID:       account.ID,
		Domains:         encoded,
		KeyType:         keyType,
		OutputDir:       output,
		RenewCron:       renewCron,
		RenewBeforeDays: renewBeforeDays,
		NotifyOnIssue:   notify,
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
	if input.AutoRenew != nil {
		badRequest(c, "auto_renew is gone: send renew_cron instead (an empty expression renews by hand)")
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
		target := cert.AccountID
		if input.AccountID > 0 {
			target = input.AccountID
		}
		if s.rejectUncoveredNames(c, target, domains) {
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
		// The names do not move with the account. When both arrive, the domains
		// branch above already checked them against this account; an account-only
		// update is the one that would otherwise slip through unchecked.
		if input.Domains == nil {
			names, err := model.CertificateNames(cert.Domains)
			if err != nil {
				serverError(c, err)
				return
			}
			if s.rejectUncoveredNames(c, input.AccountID, names) {
				return
			}
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
	if input.RenewCron != nil {
		renewCron, err := normaliseRenewCron(input.RenewCron, "")
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		updates["renew_cron"] = renewCron
	}
	if input.RenewBeforeDays != nil {
		days, err := normaliseRenewBeforeDays(input.RenewBeforeDays)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		updates["renew_before_days"] = days
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
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		// A wildcard certificate is ordered for the base name; the star is the
		// only thing the validator below would refuse, and the only thing the
		// IDNA conversion cannot carry.
		starred := strings.HasPrefix(trimmed, "*.")
		name := asciiName(strings.TrimPrefix(trimmed, "*."))
		if err := model.ValidateDomain(name); err != nil {
			return nil, err
		}
		if starred {
			name = "*." + name
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

// asciiName folds a name into the spelling DNSPod's domain list uses: lower
// case, no trailing dot, and punycode instead of unicode labels. A name that
// cannot be converted is returned unchanged, so it simply fails to match
// rather than becoming something else.
//
// The certificate has to be stored the way the CA will order it and the
// challenge will be published, which is the ASCII form for an internationalised
// name; comparing the unicode spelling against the domain list would report
// every such certificate as foreign to its own account.
func asciiName(name string) string {
	trimmed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if ascii, err := idna.ToASCII(trimmed); err == nil {
		return ascii
	}
	return trimmed
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

// rejectUncoveredNames answers the request and reports true when one of the
// names cannot be validated by this account.
//
// A certificate is only as good as the DNS-01 challenge behind it, and that
// challenge can only be answered for a domain the account owns. Checking here
// keeps a typo from becoming a row that fails every issuance the scheduler
// tries.
func (s *Server) rejectUncoveredNames(c *gin.Context, accountID uint, names []string) bool {
	// Bounded: the listing is a handful of provider calls on a cold cache, and a
	// request should not sit on them for as long as the provider feels like.
	ctx, cancel := context.WithTimeout(c.Request.Context(), certificateNameLookup)
	defer cancel()
	owned, err := s.ownedDomains(ctx, accountID)
	if err != nil {
		// Storing a certificate is a local operation otherwise, so a provider that
		// cannot be reached must not block it: the issuance reports the same
		// problem with a message of its own. The check still runs whenever DNSPod
		// answers, which is every time an issuance could actually happen.
		slog.Warn("could not check the certificate names against the account's domains",
			"account", accountID, "error", err, "rid", middleware.RequestIDFrom(c))
		return false
	}
	for _, name := range names {
		if !coveredByAccount(name, owned) {
			badRequest(c, fmt.Sprintf(
				"%s is not one of this account's domains, nor a subdomain of one", name))
			return true
		}
	}
	return false
}

// coveredByAccount reports whether a certificate name can be challenged for by
// an account that owns these domains.
//
// The name may be a domain itself, any number of levels below one, or a
// wildcard over either - the wildcard is asked for at its own level, so
// *.example.com needs example.com and nothing more.
func coveredByAccount(name string, domains []string) bool {
	base := asciiName(strings.TrimPrefix(strings.TrimSpace(name), "*."))
	for _, domain := range domains {
		domain = asciiName(domain)
		if base == domain || strings.HasSuffix(base, "."+domain) {
			return true
		}
	}
	return false
}

// normaliseRenewCron accepts an empty expression, which means the check never
// touches this row, or one the scheduler can match against a minute. A nil
// pointer keeps the fallback: on create that is the deployment's default
// schedule, so saying nothing still means "renew it for me".
func normaliseRenewCron(raw *string, fallback string) (string, error) {
	if raw == nil {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return "", nil
	}
	// @every is an interval measured from the moment it is parsed, not a
	// schedule: the check ticks once a minute and asks whether the expression
	// names this minute, which an interval whose phase is arbitrary can never
	// answer - @every 2h would never fire, @every 30s would fire every minute.
	// The calendar descriptors (@daily and friends) parse into ordinary fields.
	if strings.HasPrefix(trimmed, "@every") {
		return "", fmt.Errorf("renew_cron cannot use @every: name a five-field schedule or @daily")
	}
	if _, err := cron.ParseStandard(trimmed); err != nil {
		return "", fmt.Errorf("renew_cron must be a five-field cron expression or a calendar descriptor")
	}
	return trimmed, nil
}

// normaliseRenewBeforeDays keeps the window inside what a 90-day certificate
// can use. 0 means the deployment default.
func normaliseRenewBeforeDays(raw *int) (int, error) {
	if raw == nil {
		return 0, nil
	}
	if *raw < 0 || *raw > 60 {
		return 0, fmt.Errorf("renew_before_days must be between 0 and 60")
	}
	return *raw, nil
}
