package acme

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ddns-by-dnspod/internal/config"
)

// accountLocation is the account URL the stub CA hands out. Nothing dereferences
// it here: it is the identity every later call is addressed to, and the point is
// that registration ends up holding it.
const accountLocation = "https://ca.example/account/7b1"

// stubCA answers the three calls registration is made of: the directory, a
// nonce, and the new-account endpoint. createAccount decides what the last one
// says - 201 the first time and 200 afterwards, which is what a CA with an
// account for this key answers.
func stubCA(t *testing.T, createAccount http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var server *httptest.Server

	mux.HandleFunc("/dir", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"newNonce": %q,
			"newAccount": %q,
			"newOrder": %q,
			"revokeCert": %q,
			"keyChange": %q,
			"meta": {"termsOfService": %q}
		}`, server.URL+"/new-nonce", server.URL+"/new-account", server.URL+"/new-order",
			server.URL+"/revoke", server.URL+"/rollover", server.URL+"/terms")
	})
	mux.HandleFunc("/new-nonce", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Replay-Nonce", "c3R1Yi1ub25jZQ")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/new-account", createAccount)

	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// A key the CA already knows is answered with the existing account, which
// x/crypto/acme reports as ErrAccountAlreadyExists. Treating that as a failure
// would mean the first issuance works and every renewal after it does not.
func TestRegisterAcceptsAnExistingAccount(t *testing.T) {
	var calls int
	server := stubCA(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", accountLocation)
		status := http.StatusCreated
		if calls > 1 {
			// The CA already has an account for this key, which is what a
			// renewal sees.
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"status":"valid","contact":["mailto:e2e@example.com"]}`)
	})

	cfg := config.ACMEConfig{Directory: server.URL + "/dir", Email: "e2e@example.com"}
	key, err := generateKey(KeyEC256)
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}

	first := newRun(cfg, KeyEC256, []string{"example.com"}, key, nil)
	if err := first.register(context.Background()); err != nil {
		t.Fatalf("the first registration failed: %v", err)
	}
	if first.accountURL != accountLocation {
		t.Fatalf("account URL = %q, want %q", first.accountURL, accountLocation)
	}
	if string(first.client.KID) != accountLocation {
		t.Fatalf("KID = %q, want %q - orders are addressed to it", first.client.KID, accountLocation)
	}

	// The same key again, which is what a renewal is.
	second := newRun(cfg, KeyEC256, []string{"example.com"}, key, nil)
	if err := second.register(context.Background()); err != nil {
		t.Fatalf("the second registration failed: %v", err)
	}
	if second.accountURL != accountLocation {
		t.Fatalf("account URL = %q, want %q", second.accountURL, accountLocation)
	}
}

// A CA that refuses the account is a failure, and it is reported as the step
// that failed rather than as whatever the CA's problem document said.
func TestRegisterReportsARefusedAccount(t *testing.T) {
	server := stubCA(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"urn:ietf:params:acme:error:invalidEmail","detail":"bad address"}`)
	})

	cfg := config.ACMEConfig{Directory: server.URL + "/dir", Email: "nope@example.com"}
	key, err := generateKey(KeyEC256)
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}

	err = newRun(cfg, KeyEC256, []string{"example.com"}, key, nil).register(context.Background())
	if err == nil {
		t.Fatal("want an error when the CA refuses the account")
	}
	if !errors.Is(err, errAccount) {
		t.Fatalf("error = %v, want it to be the registration step", err)
	}
	if got := failureReason(err); strings.Contains(got, "bad address") {
		t.Fatalf("the reason %q carries the CA's own wording, which belongs in the log", got)
	}
}

// A CA that answers without naming the account leaves no URL to address an
// order to, and that has to be said here rather than at the next call.
func TestRegisterNeedsTheAccountURL(t *testing.T) {
	server := stubCA(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"status":"valid"}`)
	})

	cfg := config.ACMEConfig{Directory: server.URL + "/dir"}
	key, err := generateKey(KeyEC256)
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}

	err = newRun(cfg, KeyEC256, []string{"example.com"}, key, nil).register(context.Background())
	if !errors.Is(err, errAccount) {
		t.Fatalf("error = %v, want it to be the registration step", err)
	}
}
