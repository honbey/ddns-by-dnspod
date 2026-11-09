package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"ddns-by-dnspod/internal/model"
)

// accountByID reads one account back from the database
func accountByID(t *testing.T, srv *Server, id uint) model.Account {
	t.Helper()
	var account model.Account
	if err := srv.store.DB().First(&account, id).Error; err != nil {
		t.Fatalf("load account %d: %v", id, err)
	}
	return account
}

// accountPayload builds the JSON body for an account create/update
func accountPayload(name string, isDefault any) map[string]any {
	body := map[string]any{
		"name":       name,
		"secret_id":  "AKID-test",
		"secret_key": "a-secret-key-that-is-long-enough",
	}
	if isDefault != nil {
		body["is_default"] = isDefault
	}
	return body
}

func TestCreateAccountSetsTheDefault(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var created struct {
		Data model.Account `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !accountByID(t, srv, created.Data.ID).IsDefault {
		t.Error("an account created with is_default=true must be the default")
	}
}

func TestCreateAccountWithoutFlagIsNotDefault(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Data model.Account `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if accountByID(t, srv, created.Data.ID).IsDefault {
		t.Error("an account created without the flag must not become the default")
	}
}

// Making an account the default must leave exactly one default, no matter how
// many already existed
func TestUpdateAccountKeepsExactlyOneDefault(t *testing.T) {
	srv, cfg := newTestServer(t)
	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create a: %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Data model.Account `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	firstID := created.Data.ID

	rec = do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("b", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create b: %d (%s)", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	secondID := created.Data.ID

	// Promote the second one; the first must lose the flag
	rec = do(t, srv, cfg, http.MethodPut, "/api/accounts/"+itoa(secondID), accountPayload("b", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("update b: %d (%s)", rec.Code, rec.Body.String())
	}
	if accountByID(t, srv, secondID).IsDefault == false {
		t.Error("b must be the default after promotion")
	}
	if accountByID(t, srv, firstID).IsDefault {
		t.Error("a must lose the default flag when b is promoted")
	}
}

// Sending is_default=false explicitly must take the flag back off, which a
// missing field would never do
func TestUpdateAccountCanClearTheDefault(t *testing.T) {
	srv, cfg := newTestServer(t)
	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Data model.Account `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created.Data.ID

	rec = do(t, srv, cfg, http.MethodPut, "/api/accounts/"+itoa(id), accountPayload("a", false))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d (%s)", rec.Code, rec.Body.String())
	}
	if accountByID(t, srv, id).IsDefault {
		t.Error("an explicit is_default=false must clear the flag")
	}
}

// A name is unique, and a duplicate is the caller's own doing: answering 500
// made the client retry a request that can never succeed.
func TestCreateAccountRefusesADuplicateName(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", false))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := errorMessage(t, rec); got != "an account with this name already exists" {
		t.Fatalf("want the conflict explained, got %q", got)
	}
}

// Renaming onto a name that is taken is the same collision as creating one, and
// it deserves the same answer: the caller can fix it, so it is not a broken
// server and must not read as "retry, it may pass".
func TestUpdateAccountRefusesADuplicateName(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("a", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create a: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var second struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	rec = do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("b", false))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create b: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode b: %v", err)
	}

	rec = do(t, srv, cfg, http.MethodPut, "/api/accounts/"+itoa(second.Data.ID),
		accountPayload("a", false))
	if rec.Code != http.StatusConflict {
		t.Fatalf("rename onto a taken name: want 409, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := accountByID(t, srv, second.Data.ID).Name; got != "b" {
		t.Fatalf("the rename must not have landed, name = %q", got)
	}
}
