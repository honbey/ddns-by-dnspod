package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ddns-by-dnspod/internal/config"
)

// stubToken is the challenge token the stub CA hands out. It is fixed, so the
// value the run publishes can be checked against what the token implies.
const stubToken = "stub-token-1"

// presented is one challenge record an issuance published.
type presented struct {
	zone  string
	host  string
	value string
}

// fakePresenter stands in for DNSPod: it records what was published and removed,
// so the whole order flow can be asserted without a DNS provider in the middle.
type fakePresenter struct {
	zones     []string
	published []presented
	removed   []uint64

	// refusePresent makes publishing fail, which is what unusable credentials
	// look like from here.
	refusePresent bool
}

func (p *fakePresenter) Zones() []string { return p.zones }

func (p *fakePresenter) Present(zone, host, value string) (uint64, error) {
	if p.refusePresent {
		return 0, errors.New("dnspod refused the record")
	}
	p.published = append(p.published, presented{zone: zone, host: host, value: value})
	return uint64(len(p.published)), nil
}

func (p *fakePresenter) CleanUp(_ string, recordID uint64) error {
	p.removed = append(p.removed, recordID)
	return nil
}

// orderCA is a stub ACME server that runs a whole order: one authorization with
// a DNS-01 challenge, a finalize, and a certificate to download.
type orderCA struct {
	server   *httptest.Server
	leaf     []byte
	intermed []byte

	// brokenFinalize answers the finalize request the way Pebble does - no
	// Location header, order still processing - which is the answer the
	// fallback in finalize exists for.
	brokenFinalize bool
	// rejectChallenge fails the challenge instead of validating it.
	rejectChallenge bool

	mu    sync.Mutex
	authz string
}

func newOrderCA(t *testing.T, options ...func(*orderCA)) *orderCA {
	t.Helper()
	ca := &orderCA{authz: "pending"}
	ca.leaf, ca.intermed = testChain(t)
	for _, option := range options {
		option(ca)
	}

	mux := http.NewServeMux()
	var server *httptest.Server
	base := func() string { return server.URL }

	mux.HandleFunc("/dir", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"newNonce":   base() + "/new-nonce",
			"newAccount": base() + "/new-account",
			"newOrder":   base() + "/new-order",
			"revokeCert": base() + "/revoke",
			"keyChange":  base() + "/rollover",
			"meta":       map[string]any{"termsOfService": base() + "/terms"},
		})
	})
	mux.HandleFunc("/new-nonce", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Replay-Nonce", "c3R1Yi1ub25jZQ")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/new-account", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", base()+"/account/1")
		w.WriteHeader(http.StatusCreated)
		writeJSONBody(t, w, map[string]any{"status": "valid"})
	})
	mux.HandleFunc("/new-order", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", base()+"/order/1")
		w.WriteHeader(http.StatusCreated)
		writeJSONBody(t, w, map[string]any{
			"status":         "pending",
			"identifiers":    []map[string]string{{"type": "dns", "value": "example.com"}},
			"authorizations": []string{base() + "/authz/1"},
			"finalize":       base() + "/finalize/1",
		})
	})

	mux.HandleFunc("/authz/1", func(w http.ResponseWriter, _ *http.Request) {
		ca.mu.Lock()
		status := ca.authz
		ca.mu.Unlock()

		challenge := map[string]any{
			"type": "dns-01", "url": base() + "/chal/1", "token": stubToken, "status": "pending",
		}
		if status == "invalid" {
			challenge["status"] = "invalid"
			challenge["error"] = map[string]any{
				"type": "urn:ietf:params:acme:error:dns", "detail": "no TXT record found",
			}
		}
		writeJSON(t, w, map[string]any{
			"identifier": map[string]string{"type": "dns", "value": "example.com"},
			"status":     status,
			"challenges": []map[string]any{challenge},
		})
	})
	mux.HandleFunc("/chal/1", func(w http.ResponseWriter, _ *http.Request) {
		ca.mu.Lock()
		if ca.rejectChallenge {
			ca.authz = "invalid"
		} else {
			ca.authz = "valid"
		}
		ca.mu.Unlock()
		writeJSON(t, w, map[string]any{
			"type": "dns-01", "url": base() + "/chal/1", "token": stubToken, "status": "processing",
		})
	})
	mux.HandleFunc("/finalize/1", func(w http.ResponseWriter, _ *http.Request) {
		if ca.brokenFinalize {
			// The order is still being processed, and the CA does not say where
			// the order lives.
			writeJSON(t, w, map[string]any{"status": "processing", "finalize": base() + "/finalize/1"})
			return
		}
		w.Header().Set("Location", base()+"/order/1")
		writeJSON(t, w, map[string]any{
			"status": "valid", "certificate": base() + "/cert/1", "finalize": base() + "/finalize/1",
		})
	})
	mux.HandleFunc("/order/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"status": "valid", "certificate": base() + "/cert/1", "finalize": base() + "/finalize/1",
		})
	})
	mux.HandleFunc("/cert/1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		for _, der := range [][]byte{ca.leaf, ca.intermed} {
			if _, err := w.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
				t.Errorf("write the chain: %v", err)
			}
		}
	})

	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	ca.server = server
	return ca
}

// testChain builds the two DER certificates the stub hands out: a leaf, and the
// certificate that stands in for the intermediate.
func testChain(t *testing.T) (leaf, intermed []byte) {
	t.Helper()
	makeCert := func(commonName string) []byte {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(int64(len(commonName)) * 7),
			Subject:      pkix.Name{CommonName: commonName},
			DNSNames:     []string{"example.com"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			t.Fatalf("create certificate: %v", err)
		}
		return der
	}
	return makeCert("example.com"), makeCert("Stub Intermediate CA")
}

func writeJSON(t *testing.T, w http.ResponseWriter, body map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(t, w, body)
}

func writeJSONBody(t *testing.T, w http.ResponseWriter, body map[string]any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("write the stub response: %v", err)
	}
}

// orderRun builds the run one test drives, with a presenter that talks to
// nothing.
func orderRun(t *testing.T, ca *orderCA, presenter challengePresenter) *run {
	t.Helper()
	key, err := generateKey(KeyEC256)
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}
	cfg := config.ACMEConfig{
		Directory: ca.server.URL + "/dir",
		Email:     "e2e@example.com",
		// Zero: waiting for a resolver that cannot see a stub's records would
		// only make the test slower, and it is not what is being asserted.
		PropagationTimeout: 0,
	}
	return newRun(cfg, KeyEC256, []string{"example.com"}, key, presenter)
}

// The whole protocol in one test: order, publish, validate, request, download.
func TestIssueRunsTheWholeOrder(t *testing.T) {
	ca := newOrderCA(t)
	presenter := &fakePresenter{zones: []string{"example.com"}}
	job := orderRun(t, ca, presenter)

	bundle, err := job.issue(context.Background())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// The record went where the zone says it goes, carrying the value this
	// token implies - which is what the CA will be looking for - and it was
	// taken back out afterwards.
	if len(presenter.published) != 1 {
		t.Fatalf("published %d records, want 1: %+v", len(presenter.published), presenter.published)
	}
	record := presenter.published[0]
	if record.zone != "example.com" || record.host != "_acme-challenge" {
		t.Fatalf("published %s in %s, want _acme-challenge in example.com", record.host, record.zone)
	}
	want, err := job.client.DNS01ChallengeRecord(stubToken)
	if err != nil {
		t.Fatalf("challenge value: %v", err)
	}
	if record.value != want {
		t.Fatalf("published value %q, want %q for token %q", record.value, want, stubToken)
	}
	if len(presenter.removed) != 1 || presenter.removed[0] != 1 {
		t.Fatalf("removed %v, want the record that was published", presenter.removed)
	}

	// What came back is the leaf and the intermediate, in that order.
	if len(bundle.Chain) == 0 {
		t.Fatal("the bundle has no chain")
	}
	if string(bundle.FullChain) != string(bundle.Cert)+string(bundle.Chain) {
		t.Fatal("fullchain.pem is not the leaf followed by the chain")
	}
	leaf, err := x509.ParseCertificate(mustDecodePEM(t, bundle.Cert))
	if err != nil {
		t.Fatalf("parse the leaf: %v", err)
	}
	if !leaf.NotAfter.Equal(bundle.NotAfter) {
		t.Fatalf("bundle expiry = %s, want the leaf's %s", bundle.NotAfter, leaf.NotAfter)
	}
}

// A CA that answers the finalize request without naming the order is the case
// the fallback in finalize exists for: the certificate is issued, and the
// library's own wait would poll an empty URL and report a failure.
func TestIssueWithAFinalizeThatNamesNoOrder(t *testing.T) {
	ca := newOrderCA(t, func(ca *orderCA) { ca.brokenFinalize = true })
	presenter := &fakePresenter{zones: []string{"example.com"}}
	job := orderRun(t, ca, presenter)

	bundle, err := job.issue(context.Background())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if len(bundle.FullChain) == 0 || bundle.NotAfter.IsZero() {
		t.Fatalf("bundle is incomplete: %+v", bundle)
	}
	if len(presenter.removed) != 1 {
		t.Fatalf("the challenge record was left behind: removed %v", presenter.removed)
	}
}

// A challenge the CA refuses ends the run, and the record still comes back out:
// a leftover _acme-challenge record is what the next attempt trips over.
func TestIssueCleansUpAfterARejectedChallenge(t *testing.T) {
	ca := newOrderCA(t, func(ca *orderCA) { ca.rejectChallenge = true })
	presenter := &fakePresenter{zones: []string{"example.com"}}
	job := orderRun(t, ca, presenter)

	_, err := job.issue(context.Background())
	if err == nil {
		t.Fatal("want a failure when the CA rejects the challenge")
	}
	var invalid *invalidChallengeError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v (%T), want an invalidChallengeError", err, err)
	}
	if !strings.Contains(invalid.Error(), "example.com") {
		t.Fatalf("the reason %q should name the domain", invalid.Error())
	}
	if len(presenter.removed) != 1 {
		t.Fatalf("removed %v, want the challenge record taken back out", presenter.removed)
	}
}

// Nothing to clean up when the record never went out, and the reason says which
// step failed without carrying the provider's wording.
func TestIssueReportsARecordThatCouldNotBePublished(t *testing.T) {
	ca := newOrderCA(t)
	presenter := &fakePresenter{zones: []string{"example.com"}, refusePresent: true}
	job := orderRun(t, ca, presenter)

	_, err := job.issue(context.Background())
	if !errors.Is(err, errChallengePublish) {
		t.Fatalf("error = %v, want the publishing step", err)
	}
	if len(presenter.removed) != 0 {
		t.Fatalf("removed %v, want nothing removed", presenter.removed)
	}
	if strings.Contains(failureReason(err), "dnspod refused") {
		t.Fatalf("the reason leaked the underlying error: %q", failureReason(err))
	}
}

// The zones decide where the record goes, and a name none of them serves is
// refused before anything is published.
func TestIssueRefusesANameNoZoneServes(t *testing.T) {
	ca := newOrderCA(t)
	presenter := &fakePresenter{zones: []string{"other.com"}}
	job := orderRun(t, ca, presenter)

	_, err := job.issue(context.Background())
	var noZone *noZoneError
	if !errors.As(err, &noZone) {
		t.Fatalf("error = %v (%T), want a noZoneError", err, err)
	}
	if len(presenter.published) != 0 {
		t.Fatalf("published %+v, want nothing published", presenter.published)
	}
}

// The whole run happens on the caller's context: one that is already over stops
// before any request goes out.
func TestIssueStopsWithTheContext(t *testing.T) {
	ca := newOrderCA(t)
	presenter := &fakePresenter{zones: []string{"example.com"}}
	job := orderRun(t, ca, presenter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := job.issue(ctx); err == nil {
		t.Fatal("want an error from an issuance whose context is already done")
	}
	if len(presenter.published) != 0 {
		t.Fatalf("published %+v with a cancelled context", presenter.published)
	}
}

func mustDecodePEM(t *testing.T, data []byte) []byte {
	t.Helper()
	block, rest := pem.Decode(data)
	if block == nil {
		t.Fatalf("not PEM: %q", data)
	}
	if len(rest) != 0 {
		t.Fatalf("more than one block in %d bytes", len(data))
	}
	return block.Bytes
}
