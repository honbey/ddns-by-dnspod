package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/auth"
)

type budgetStub struct {
	token string
}

func (s budgetStub) Enabled() bool { return true }

func (s budgetStub) DevSub() string { return "" }

func (s budgetStub) Verify(_ context.Context, raw string) (*auth.Identity, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if raw != s.token {
		return nil, errors.New("not this token")
	}
	return &auth.Identity{Subject: "sub-1", Username: "alice"}, nil
}

// An opaque token cannot be compared locally before verification. Once a
// source has spent its failure budget, one bounded probe is still allowed, so a
// legitimate opaque token behind a NAT is not locked out for the whole window.
func TestBearerAllowsAnOpaqueProbePastTheBudget(t *testing.T) {
	old := rejectBudgetPerMinute
	rejectBudgetPerMinute = 2
	rejectMu.Lock()
	rejects = map[string]*rejectCounter{}
	rejectMu.Unlock()
	t.Cleanup(func() {
		rejectBudgetPerMinute = old
		rejectMu.Lock()
		rejects = map[string]*rejectCounter{}
		rejectMu.Unlock()
	})

	valid := strings.Repeat("v", 32)
	invalid := strings.Repeat("x", 32)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Bearer(budgetStub{token: valid}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	do := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 2; i++ {
		if got := do(invalid); got != http.StatusUnauthorized {
			t.Fatalf("rejection %d = %d, want 401", i, got)
		}
	}
	if got := do(invalid); got != http.StatusTooManyRequests {
		t.Fatalf("over budget = %d, want 429", got)
	}
	if got := do(valid); got != http.StatusNoContent {
		t.Fatalf("valid opaque probe = %d, want 204", got)
	}
}
