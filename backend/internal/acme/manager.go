// Package acme issues and renews TLS certificates through an ACME server,
// answering the DNS-01 challenge through the DNSPod account this deployment
// already writes records with.
//
// It is here because the certificate a TLS application needs is the one thing
// this server is already positioned to fetch: it holds the DNS credentials and
// it runs on a schedule, which is the whole of what an ACME client needs. The
// files land under the configured directory with the names certbot uses, so an
// application configured for certbot needs no change at all.
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// errWrite is the one failure that happens on this side of the wire.
var errWrite = errors.New("the certificate could not be written to disk")

// Manager owns the certificates: it runs issuance in the background, writes the
// files where the TLS applications can reach them, and announces the outcome.
type Manager struct {
	store  *store.Store
	cfg    config.ACMEConfig
	dnspod config.DNSPodConfig
	sender *notifier.Sender

	// One issuance per certificate at a time, the way the DDNS executor holds
	// one run per task: the schedule and the button can both ask at once.
	mu      sync.Mutex
	running map[uint]struct{}

	// slots bounds how many issuances are in flight at once, and inflight counts
	// the ones that are still going, so shutdown can give them a moment.
	slots    chan struct{}
	inflight sync.WaitGroup
}

// maxConcurrentIssuances is how many certificates are issued at the same time.
//
// An issuance is slow and rare, and both ends of it prefer a small number: a
// morning when every certificate is due should not become that many orders at
// the CA and that many record writes at DNSPod in the same second.
const maxConcurrentIssuances = 2

// NewManager builds the certificate manager.
func NewManager(st *store.Store, cfg config.ACMEConfig, dnspodCfg config.DNSPodConfig, sender *notifier.Sender) *Manager {
	return &Manager{
		store:   st,
		cfg:     cfg,
		dnspod:  dnspodCfg,
		sender:  sender,
		running: make(map[uint]struct{}),
		slots:   make(chan struct{}, maxConcurrentIssuances),
	}
}

// Enabled reports whether the manager is configured to run at all.
func (m *Manager) Enabled() bool { return m.cfg.Enabled }

// Config is what the API needs to answer with: where the files land, and how
// long before expiry a renewal is due.
func (m *Manager) Config() config.ACMEConfig { return m.cfg }

// OutputPath is the directory one certificate's files are written to: a
// subdirectory of the configured root, named after the certificate unless it
// named a subdirectory of its own.
//
// The subdirectory is checked when it is stored, so this only joins the two:
// nothing a certificate carries can walk out of the root.
func (m *Manager) OutputPath(cert *model.Certificate) string {
	sub := cert.OutputDir
	if sub == "" {
		sub = cert.Name
	}
	return filepath.Join(m.cfg.OutputDir, sub)
}

// Start begins one issuance in the background and returns at once.
//
// Issuance is asynchronous because it outlives a request: the CA has to be
// asked to validate a challenge and answers when it is done, which takes tens
// of seconds, while the HTTP server gives up after one. The row is what the UI
// watches - RUNNING first, then OK or FAIL.
func (m *Manager) Start(id uint) error {
	var cert model.Certificate
	if err := m.store.DB().First(&cert, id).Error; err != nil {
		return err
	}
	if !m.tryStart(id) {
		return fmt.Errorf("an issuance for this certificate is already running")
	}
	// Written before the goroutine starts, so the caller that has just asked
	// for a run reads RUNNING back rather than the status of the run before it.
	m.setStatus(id, model.CertStatusRUNNING, "issuing")

	// Counted here rather than inside the goroutine: shutdown waits on this
	// counter, and a run that has not started yet is still a run it has to know
	// about.
	m.inflight.Add(1)
	go func() {
		defer m.inflight.Done()
		defer m.finish(id)
		defer m.recoverIssuance(&cert)
		m.run(&cert)
	}()
	return nil
}

// recoverIssuance keeps a panic inside one issuance from taking the process
// down with it.
//
// The run is its own goroutine, and the process it would end holds the HTTP
// server, the DDNS schedule and every other certificate. The ACME client in
// there is third-party code, so a panic is not something this side can rule
// out. This is the only place a panic is caught: anywhere else it should reach
// the surface, and a panic swallowed where it happened is a bug nobody sees.
func (m *Manager) recoverIssuance(cert *model.Certificate) {
	recovered := recover()
	if recovered == nil {
		return
	}
	slog.Error("recovered from a panic while issuing a certificate",
		"certificate", cert.ID,
		"name", cert.Name,
		"panic", recovered,
		"stack", string(debug.Stack()),
	)
	m.recordFailure(cert, nil, "the issuance failed unexpectedly")
}

// Wait blocks until no issuance is running, or until the context ends, and
// reports whether it finished waiting.
//
// Shutdown calls this so a run in flight gets the chance to take its challenge
// record back out of DNS - that record is the one thing an abandoned issuance
// leaves behind. It does not wait indefinitely: an issuance can take minutes,
// and a service that will not stop is worse than a leftover TXT record.
func (m *Manager) Wait(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		m.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// RenewDue starts an issuance for every certificate that is due, and reports
// how many it started.
//
// A certificate with no not_after has never been issued, and one that has is
// due once it is inside the renewal window. A failure is left in the row and
// announced; the next check tries again the following day, which is why the
// window is a month wide.
func (m *Manager) RenewDue() (int, error) {
	if !m.cfg.Enabled || m.cfg.RenewBeforeDays <= 0 {
		return 0, nil
	}

	var certs []model.Certificate
	if err := m.store.DB().Where("auto_renew = ?", true).Order("id").Find(&certs).Error; err != nil {
		return 0, fmt.Errorf("read the certificates: %w", err)
	}

	started := 0
	for _, cert := range dueForRenewal(certs, m.cfg.RenewBeforeDays, time.Now()) {
		if err := m.Start(cert); err != nil {
			// Already running, or gone: neither is worth failing the check for.
			slog.Error("renewal check could not read the certificate", "certificate", cert, "error", err)
			continue
		}
		started++
	}
	return started, nil
}

// dueForRenewal picks the certificates a check at this moment should issue.
//
// A certificate that has never been issued counts as due: creating one with
// automatic renewal on is how a certificate is asked for without pressing
// anything, and the daily check is what presses it. One that is still inside
// the window keeps its place, and the next check an hour later will try again.
func dueForRenewal(certs []model.Certificate, renewBeforeDays int, now time.Time) []uint {
	cutoff := now.AddDate(0, 0, renewBeforeDays)
	var due []uint
	for i := range certs {
		cert := certs[i]
		if !cert.AutoRenew {
			continue
		}
		if cert.NotAfter != nil && cert.NotAfter.After(cutoff) {
			continue
		}
		due = append(due, cert.ID)
	}
	return due
}

// run performs one issuance and records what came of it.
func (m *Manager) run(cert *model.Certificate) {
	names, err := model.CertificateNames(cert.Domains)
	if err != nil || len(names) == 0 {
		slog.Error("certificate has an unusable domain list", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, nil, "this certificate has no usable domain list")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.cfg.Timeout)*time.Second)
	defer cancel()

	// Waiting for a slot counts against the run's own deadline: a run that
	// cannot start inside its time budget is reported like any other failure,
	// which is what the deadline would have done to it anyway.
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		m.recordFailure(cert, names, "the issuance ran out of time before it could start")
		return
	}

	cli, err := m.clientFor(cert.AccountID)
	if err != nil {
		slog.Error("the DNSPod credentials of this account could not be read", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, names, "the DNSPod credentials of this account could not be read")
		return
	}
	accountKey, err := m.accountKey()
	if err != nil {
		slog.Error("the ACME account key could not be prepared", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, names, "the ACME account key could not be prepared")
		return
	}
	presenter, err := newDNSPresenter(ctx, cli)
	if err != nil {
		slog.Error("the domains of this account could not be listed", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, names, "the domains of this account could not be listed")
		return
	}

	job := newRun(m.cfg, cert.KeyType, names, accountKey, presenter)
	bundle, err := job.issue(ctx)
	if err != nil {
		slog.Error("certificate issuance failed", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, names, failureReason(err))
		return
	}
	m.rememberAccountURL(job.accountURL)

	dir := m.OutputPath(cert)
	if err := WriteFiles(dir, bundle, m.cfg.KeyFileMode); err != nil {
		slog.Error("the certificate files could not be written", "certificate", cert.ID, "name", cert.Name, "error", err)
		m.recordFailure(cert, names, errWrite.Error())
		return
	}

	now := time.Now()
	message := fmt.Sprintf("issued, valid until %s", bundle.NotAfter.Format("2006-01-02 15:04:05"))
	if err := m.store.DB().Model(&model.Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
		"last_status":    model.CertStatusOK,
		"last_message":   message,
		"not_after":      bundle.NotAfter,
		"last_issued_at": now,
		"fail_count":     0,
	}).Error; err != nil {
		slog.Error("could not record the issuance result", "certificate", cert.ID, "error", err)
	}

	// The serial is worth a log line: it is what the file on disk can be
	// matched against when two certificates for the same name are in play.
	slog.Info("certificate issued",
		"certificate", cert.ID,
		"name", cert.Name,
		"serial", bundle.Serial,
		"valid_until", bundle.NotAfter.Format(time.RFC3339),
		"directory", dir,
	)

	if cert.NotifyOnIssue {
		m.notify(cert, names, model.CertStatusOK, fmt.Sprintf("%s, files in %s", message, dir))
	}
}

// setStatus writes what the run is doing, so the UI can show it while it
// happens.
func (m *Manager) setStatus(id uint, status, message string) {
	if err := m.store.DB().Model(&model.Certificate{}).Where("id = ?", id).
		Updates(map[string]any{"last_status": status, "last_message": message}).Error; err != nil {
		slog.Error("could not record the run status", "certificate", id, "error", err)
	}
}

// recordFailure writes a failed run and announces it.
//
// A failure is always announced: a certificate that quietly stopped renewing is
// the one outcome this feature exists to prevent, and it is invisible until the
// day a browser refuses the connection.
func (m *Manager) recordFailure(cert *model.Certificate, names []string, reason string) {
	failCount := cert.FailCount + 1
	if err := m.store.DB().Model(&model.Certificate{}).Where("id = ?", cert.ID).Updates(map[string]any{
		"last_status":  model.CertStatusFAIL,
		"last_message": reason,
		"fail_count":   failCount,
	}).Error; err != nil {
		slog.Error("could not record the failure", "certificate", cert.ID, "error", err)
	}
	slog.Error("certificate issuance failed", "certificate", cert.ID, "name", cert.Name, "reason", reason)
	m.notify(cert, names, model.CertStatusFAIL, reason)
}

// notify pushes a message to every enabled channel.
func (m *Manager) notify(cert *model.Certificate, names []string, status, message string) {
	var channels []model.NotifyChannel
	if err := m.store.DB().Where("enabled = ?", true).Find(&channels).Error; err != nil {
		slog.Error("could not read the notification channels", "error", err)
		return
	}
	if len(channels) == 0 {
		return
	}

	title := "Certificate issued"
	if status == model.CertStatusFAIL {
		title = "Certificate renewal failed"
	}
	msg := notifier.Message{
		Title:   title,
		Content: fmt.Sprintf("%s (%s) %s", cert.Name, strings.Join(names, ", "), message),
		Status:  status,
		Time:    time.Now().Format("2006-01-02 15:04:05"),
	}
	// A failure can be announced before the names could be read - a row whose
	// domain list is unusable is announced exactly that way - so the field is
	// filled only when there is something to fill it with. Indexing it blind
	// was a panic, and a panic here ends the process.
	if len(names) > 0 {
		msg.Domain = names[0]
	}

	for _, channel := range channels {
		if err := m.sender.Dispatch(channel.Type, channel.Config, msg); err != nil {
			slog.Error("could not send the notification", "channel", channel.Name, "type", channel.Type, "error", err)
		}
	}
}

// tryStart marks a certificate as being issued, and reports false when one is
// already running for it.
func (m *Manager) tryStart(id uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.running[id]; ok {
		return false
	}
	m.running[id] = struct{}{}
	return true
}

// finish releases the in-flight mark of a certificate.
func (m *Manager) finish(id uint) {
	m.mu.Lock()
	delete(m.running, id)
	m.mu.Unlock()
}

// clientFor builds a DNSPod client for the account a certificate answers its
// challenges with.
//
// It is deliberately not cached: an issuance is rare enough that a fresh client
// costs nothing, and this keeps the certificate manager out of the client cache
// the request path owns.
func (m *Manager) clientFor(accountID uint) (*dnspod.Client, error) {
	var account model.Account
	if err := m.store.DB().First(&account, accountID).Error; err != nil {
		return nil, fmt.Errorf("read the account %d: %w", accountID, err)
	}
	secret, err := m.store.Encryptor().Decrypt(account.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("read the credentials of account %d: %w", accountID, err)
	}
	// An empty TTL set turns the read cache off: a challenge listing is read
	// once per issuance, and a stale zone list is the one thing that would put
	// the TXT record in the wrong place.
	return dnspod.NewClientWithCache(account.SecretID, secret, m.dnspod.Endpoint, m.dnspod.Timeout, dnspod.CacheTTL{})
}

// accountKey returns the ACME account key for the configured directory,
// creating one the first time a certificate is issued there.
//
// The key is the account at the CA, so it is stored encrypted like every other
// credential here. Losing it means registering again as a new account: the
// certificates issued so far stay valid, they simply cannot be managed through
// the old account any more.
func (m *Manager) accountKey() (crypto.Signer, error) {
	key, err := m.readAccountKey()
	switch {
	case err == nil:
		return key, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return m.createAccountKey()
	default:
		return nil, err
	}
}

// readAccountKey reads the stored key, reporting gorm.ErrRecordNotFound when
// this directory has none yet.
func (m *Manager) readAccountKey() (crypto.Signer, error) {
	var row model.ACMEAccount
	if err := m.store.DB().Where("directory = ?", m.cfg.Directory).First(&row).Error; err != nil {
		return nil, err
	}
	plain, err := m.store.Encryptor().Decrypt(row.Key)
	if err != nil {
		return nil, fmt.Errorf("read the ACME account key: %w", err)
	}
	return parseKey([]byte(plain))
}

// createAccountKey generates and stores an account key.
//
// P-256 rather than RSA: every ACME server accepts it, and it is a fraction of
// the work to sign with on every request of an order.
func (m *Manager) createAccountKey() (crypto.Signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate the ACME account key: %w", err)
	}
	encoded, err := keyPEM(key)
	if err != nil {
		return nil, err
	}
	encrypted, err := m.store.Encryptor().Encrypt(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("encrypt the ACME account key: %w", err)
	}

	row := model.ACMEAccount{
		Directory: m.cfg.Directory,
		Email:     m.cfg.Email,
		Key:       encrypted,
		KeyHint:   keyHint(key.Public()),
	}
	if err := m.store.DB().Create(&row).Error; err != nil {
		// Another issuance may have created one in the meantime - the first
		// certificate after a cold start, or two tabs at once. The directory
		// has a unique index and the account is the same either way, so the
		// stored key is used rather than failing a run that has nothing wrong
		// with it.
		if stored, readErr := m.readAccountKey(); readErr == nil {
			slog.Info("another run created the ACME account key first, using it", "directory", m.cfg.Directory)
			return stored, nil
		}
		return nil, fmt.Errorf("store the ACME account key: %w", err)
	}
	slog.Info("created an ACME account key", "directory", m.cfg.Directory, "key_hint", row.KeyHint)
	return key, nil
}

// rememberAccountURL keeps the URL the CA answered with, which is the account's
// identity in its own logs and in any support conversation about it.
func (m *Manager) rememberAccountURL(url string) {
	if url == "" {
		return
	}
	err := m.store.DB().Model(&model.ACMEAccount{}).Where("directory = ?", m.cfg.Directory).
		Where("account_url <> ?", url).
		Update("account_url", url).Error
	if err != nil {
		slog.Error("could not record the account URL", "error", err)
	}
}

// keyHint is a short fingerprint of an account's public key, so a log line can
// name the account without carrying the key.
func keyHint(pub crypto.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:8])
}
