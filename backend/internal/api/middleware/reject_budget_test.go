package middleware

import (
	"net/http"
	"strings"
	"testing"
)

// Past the budget the answer changes from 401 to 429. A valid token is not
// affected: only failures spend the budget, so a neighbour on the same NAT is
// not locked out by somebody else's noise.
func TestBearerBudgetTurnsIntoTooManyAttempts(t *testing.T) {
	t.Cleanup(func() {
		SetRejectBudgetPerMinute(30)
		rejectMu.Lock()
		rejects = map[string]*rejectCounter{}
		rejectMu.Unlock()
	})

	SetRejectBudgetPerMinute(2)
	rejectMu.Lock()
	rejects = map[string]*rejectCounter{}
	rejectMu.Unlock()

	r := newBearerEngine(stubVerifier{enabled: true, token: "good"})

	for attempt := 1; attempt <= 2; attempt++ {
		if w := doWithAuth(r, "Bearer bad"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401 inside the budget, got %d %s", attempt, w.Code, w.Body.String())
		}
	}

	w := doWithAuth(r, "Bearer bad")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 past the budget, got %d %s", w.Code, w.Body.String())
	}
	// Deliberately not WriteError: a flood must not be handed a correlation
	// key for its own answers.
	if strings.Contains(w.Body.String(), "request_id") {
		t.Fatalf("the 429 must not carry a request id: %s", w.Body.String())
	}

	if w := doWithAuth(r, "Bearer good"); w.Code != http.StatusOK {
		t.Fatalf("a valid token must still pass, got %d %s", w.Code, w.Body.String())
	}
}
