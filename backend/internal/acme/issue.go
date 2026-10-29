package acme

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	xacme "golang.org/x/crypto/acme"

	"ddns-by-dnspod/internal/config"
)

// userAgent identifies this deployment to the CA, which is the first thing
// somebody on the other side looks at when the requests turn strange.
const userAgent = "ddns-by-dnspod/1.0"

// callTimeout bounds one call to the CA. The issuance as a whole has the
// deadline from the configuration, which is much longer: most of it is spent
// waiting for the CA to validate a challenge.
const callTimeout = 30 * time.Second

// The steps an issuance can fail at. They are what the certificate row says
// went wrong, because each one means something different to whoever reads it.
var (
	errAccount          = errors.New("the ACME account could not be registered")
	errChallengePublish = errors.New("the challenge record could not be published to DNSPod")
	errOrder            = errors.New("the certificate authority refused the order")
)

// invalidChallengeError is a challenge the CA could not validate. It names the
// domain, which is the one thing its reader has to look at.
type invalidChallengeError struct {
	name string
	err  error
}

func (e *invalidChallengeError) Error() string {
	return fmt.Sprintf("the certificate authority could not validate %s: check that the challenge record is published and resolves", e.name)
}

func (e *invalidChallengeError) Unwrap() error { return e.err }

// issuedRecord is one challenge record that was published and still has to go.
type issuedRecord struct {
	zone string
	id   uint64
}

// challengePresenter publishes the TXT records one issuance needs, and takes
// them back out.
//
// The ACME path talks to this rather than to DNSPod directly, which is also
// what lets the whole order flow run against a local CA in a test.
type challengePresenter interface {
	// Zones are the zones the account holds; a challenge goes into the longest
	// one that serves its name.
	Zones() []string
	// Present publishes one TXT record and returns the id it was created with.
	Present(zone, host, value string) (uint64, error)
	// CleanUp removes one published record.
	CleanUp(zone string, recordID uint64) error
}

// run is one issuance: the order, the challenges it waits on, and the
// certificate that comes out of it.
type run struct {
	cfg        config.ACMEConfig
	keyType    string
	names      []string
	accountKey crypto.Signer
	presenter  challengePresenter
	client     *xacme.Client
	accountURL string
}

func newRun(cfg config.ACMEConfig, keyType string, names []string, accountKey crypto.Signer, presenter challengePresenter) *run {
	return &run{
		cfg:        cfg,
		keyType:    keyType,
		names:      names,
		accountKey: accountKey,
		presenter:  presenter,
		client: &xacme.Client{
			Key:          accountKey,
			DirectoryURL: cfg.Directory,
			HTTPClient:   &http.Client{Timeout: callTimeout},
			UserAgent:    userAgent,
		},
	}
}

// register makes sure the account key is known to the CA, and learns the
// account URL the rest of the protocol is addressed with.
//
// Two answers are fine here, and the second one is the common case: a key the
// CA has not seen before is registered (201), and a key it has seen is returned
// as the existing account (200). The client reports the second as
// ErrAccountAlreadyExists - after caching the account URL, which is what makes
// it usable - so it is handled as the success it is. Without that, every
// issuance after the first one would fail.
func (r *run) register(ctx context.Context) error {
	contact := []string{}
	if r.cfg.Email != "" {
		contact = append(contact, "mailto:"+r.cfg.Email)
	}

	// The terms prompt has to be answered, and there is exactly one answer a
	// non-interactive process can give.
	account, err := r.client.Register(ctx, &xacme.Account{Contact: contact}, xacme.AcceptTOS)
	switch {
	case err == nil:
		r.accountURL = account.URI
	case errors.Is(err, xacme.ErrAccountAlreadyExists):
		r.accountURL = string(r.client.KID)
	default:
		// Both errors are wrapped: the step for the log, and whatever the CA
		// said for the reason the row reports, which can be more specific than
		// the step - a rate limit is a rate limit wherever it is hit.
		return fmt.Errorf("%w: %w", errAccount, err)
	}

	// An order is addressed to the account, so there is nothing to do without
	// its URL, and saying so here is clearer than the 401 that would follow.
	if r.accountURL == "" {
		return fmt.Errorf("%w: the certificate authority did not name the account", errAccount)
	}
	r.client.KID = xacme.KeyID(r.accountURL)
	return nil
}

// issue runs the whole protocol and returns the certificate to write out.
func (r *run) issue(ctx context.Context) (*Bundle, error) {
	if err := r.register(ctx); err != nil {
		return nil, err
	}

	order, err := r.client.AuthorizeOrder(ctx, xacme.DomainIDs(r.names...))
	if err != nil {
		return nil, fmt.Errorf("create the order: %w", err)
	}

	// Every authorization the order is waiting on gets its own TXT record, and
	// every one of them is taken back out even when a later step fails: a
	// leftover _acme-challenge record is what the next issuance trips over.
	var published []issuedRecord
	defer func() { r.removeAll(published) }()

	for _, authzURL := range order.AuthzURLs {
		authz, err := r.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, fmt.Errorf("read the authorization %s: %w", authzURL, err)
		}
		// An authorization the CA still considers valid needs nothing: it is
		// reused, and asking for the challenge again would only add a record.
		if authz.Status == xacme.StatusValid {
			continue
		}
		record, err := r.answer(ctx, authz)
		if record != nil {
			published = append(published, *record)
		}
		if err != nil {
			return nil, err
		}
	}

	key, err := generateKey(r.keyType)
	if err != nil {
		return nil, err
	}
	csr, err := csrDER(key, r.names)
	if err != nil {
		return nil, err
	}

	der, err := r.finalize(ctx, order.URI, order.FinalizeURL, csr)
	if err != nil {
		return nil, err
	}
	return bundleFromDER(der, key)
}

// finalize submits the request and collects the issued certificate.
//
// The library's own call is tried first, and it is enough for a CA that names
// the order in the Location header of its answer, which Let's Encrypt does.
// The fallback exists because a CA is allowed not to: Pebble does not, and the
// library then polls an empty URL and fails with the certificate already
// issued. The wait and the download are done from the order URL this side
// holds, which is the one the order was created with.
func (r *run) finalize(ctx context.Context, orderURL, finalizeURL string, csr []byte) ([][]byte, error) {
	der, _, err := r.client.CreateOrderCert(ctx, finalizeURL, csr, true)
	if err == nil {
		return der, nil
	}

	order, waitErr := r.client.WaitOrder(ctx, orderURL)
	if waitErr != nil || order.CertURL == "" {
		// The wait failed as well, so the order really is not complete: the
		// first error is the one that says what the CA made of the request.
		return nil, fmt.Errorf("%w: %w", errOrder, err)
	}
	der, err = r.client.FetchCert(ctx, order.CertURL, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errOrder, err)
	}
	return der, nil
}

// answer publishes the TXT record of one authorization, has the CA validate it,
// and hands the record back so the caller can remove it.
func (r *run) answer(ctx context.Context, authz *xacme.Authorization) (*issuedRecord, error) {
	challenge := dnsChallenge(authz)
	if challenge == nil {
		return nil, fmt.Errorf("the certificate authority offers no DNS-01 challenge for %s", authz.Identifier.Value)
	}
	value, err := r.client.DNS01ChallengeRecord(challenge.Token)
	if err != nil {
		return nil, fmt.Errorf("build the challenge value: %w", err)
	}
	zone, host, err := RecordPlan(authz.Identifier.Value, r.presenter.Zones())
	if err != nil {
		return nil, err
	}

	recordID, err := r.presenter.Present(zone, host, value)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errChallengePublish, err)
	}
	record := &issuedRecord{zone: zone, id: recordID}

	// The wait is a head start, not a gate: whatever the local resolver makes of
	// the record, the CA reads the zone's authoritative servers.
	name := ChallengeRecordName(authz.Identifier.Value)
	if err := waitForTXT(ctx, name, value, time.Duration(r.cfg.PropagationTimeout)*time.Second); err != nil {
		if ctx.Err() != nil {
			return record, ctx.Err()
		}
		slog.Warn("letting the certificate authority decide", "name", name, "error", err)
	}

	if _, err := r.client.Accept(ctx, challenge); err != nil {
		return record, fmt.Errorf("tell the certificate authority the challenge is ready: %w", err)
	}
	if _, err := r.client.WaitAuthorization(ctx, authz.URI); err != nil {
		return record, &invalidChallengeError{name: authz.Identifier.Value, err: err}
	}
	return record, nil
}

// removeAll takes the published records back out.
//
// It runs whatever the issuance's own context did: a run that ran out of time
// is exactly when a record would otherwise be left behind. Each deletion still
// has the SDK's request timeout of its own, so a stalled call cannot hold the
// goroutine forever.
func (r *run) removeAll(records []issuedRecord) {
	for _, record := range records {
		if err := r.presenter.CleanUp(record.zone, record.id); err != nil {
			slog.Error("could not remove the challenge record", "error", err)
		}
	}
}

// dnsChallenge picks the DNS-01 challenge out of an authorization.
func dnsChallenge(authz *xacme.Authorization) *xacme.Challenge {
	for _, challenge := range authz.Challenges {
		if challenge.Type == "dns-01" {
			return challenge
		}
	}
	return nil
}

// failureReason is the sentence the certificate row and the notification carry:
// which step failed, in words whoever owns the domain can act on. The error
// itself - the CA's wording, the record id, the URLs - goes to the log instead,
// which is where an operator looks for it.
func failureReason(err error) string {
	var noZone *noZoneError
	if errors.As(err, &noZone) {
		return noZone.Error()
	}
	var invalid *invalidChallengeError
	if errors.As(err, &invalid) {
		return invalid.Error()
	}
	var problem *xacme.Error
	if errors.As(err, &problem) {
		// The two the ACME problem document carries in practice, said in a
		// sentence; anything else is rare enough to be reported by its type.
		// The status is checked as well because a rate limit is a 429 whatever
		// a CA calls the problem.
		switch {
		case problem.StatusCode == http.StatusTooManyRequests,
			problem.ProblemType == "urn:ietf:params:acme:error:rateLimited":
			return "the certificate authority rate limit was reached, try again later"
		case problem.ProblemType == "urn:ietf:params:acme:error:rejectedIdentifier":
			return "the certificate authority refused one of these names: a public suffix or an internal name cannot be issued"
		}
		return fmt.Sprintf("the certificate authority refused the request (%s)", problem.ProblemType)
	}

	switch {
	case errors.Is(err, errAccount):
		return errAccount.Error()
	case errors.Is(err, errChallengePublish):
		return errChallengePublish.Error()
	case errors.Is(err, errOrder):
		return errOrder.Error()
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "the issuance ran out of time before the certificate authority was done"
	default:
		return "the issuance failed, see the log for what the certificate authority said"
	}
}
