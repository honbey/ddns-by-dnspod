package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"ddns-by-dnspod/internal/model"
)

func taskPayload(overrides map[string]any) map[string]any {
	body := map[string]any{
		"account_id":       0, // default account
		"domain":           "example.com",
		"sub_domain":       "ddns",
		"record_type":      "A",
		"record_line":      "默认",
		"cron_expr":        "*/5 * * * *",
		"enabled":          true,
		"notify_on_change": true,
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

// A cron expression the scheduler cannot parse used to be stored silently, and
// the task then never ran because Reload dropped it. It must be rejected up front.
func TestCreateTaskRejectsInvalidCron(t *testing.T) {
	srv, cfg := newTestServer(t)
	newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/ddns/tasks", taskPayload(map[string]any{
		"cron_expr": "not a cron",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	var left int64
	if err := srv.store.DB().Model(&model.DDNSTask{}).Count(&left).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if left != 0 {
		t.Fatalf("a rejected task must not be stored, got %d", left)
	}
}

func TestUpdateTaskRejectsInvalidCron(t *testing.T) {
	srv, cfg := newTestServer(t)
	task := newTestTask(t, srv)

	rec := do(t, srv, cfg, http.MethodPut, "/api/ddns/tasks/"+itoa(task.ID), taskPayload(map[string]any{
		"cron_expr": "not a cron",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// Host names are stored in the change history, which the frontend renders as
// HTML, and they also land in the server log. Rejecting anything that is not a
// legitimate DNS name at the API boundary keeps both sinks inert, instead of
// trusting every consumer to escape.
func TestCreateTaskRejectsMalformedHostNames(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
	}{
		{"script tag in domain", "domain", "<script>alert(1)</script>.com"},
		{"event handler in domain", "domain", "<img src=x onerror=alert(1)>.com"},
		{"newline in domain", "domain", "example\n.com"},
		{"space in domain", "domain", "exa mple.com"},
		{"script tag in subdomain", "sub_domain", "<script>alert(1)</script>"},
		{"quote in subdomain", "sub_domain", `a"onmouseover="alert(1)`},
		{"newline in subdomain", "sub_domain", "a\nb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cfg := newTestServer(t)
			newTestAccount(t, srv)

			rec := do(t, srv, cfg, http.MethodPost, "/api/ddns/tasks", taskPayload(map[string]any{
				tc.field: tc.value,
			}))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}

			// Nothing may be persisted, otherwise the payload still reaches the
			// history table and the log through the scheduler.
			var left int64
			if err := srv.store.DB().Model(&model.DDNSTask{}).Count(&left).Error; err != nil {
				t.Fatalf("count: %v", err)
			}
			if left != 0 {
				t.Fatalf("a rejected task must not be stored, got %d", left)
			}
		})
	}
}

// Updating must apply the same rules: a valid task is not a way to smuggle a
// payload past the create-time check.
func TestUpdateTaskRejectsMalformedHostNames(t *testing.T) {
	srv, cfg := newTestServer(t)
	task := newTestTask(t, srv)

	rec := do(t, srv, cfg, http.MethodPut, "/api/ddns/tasks/"+itoa(task.ID), taskPayload(map[string]any{
		"domain": "<script>alert(1)</script>.com",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	var stored model.DDNSTask
	if err := srv.store.DB().First(&stored, task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Domain != task.Domain {
		t.Fatalf("domain = %q, the original %q must survive a rejected update", stored.Domain, task.Domain)
	}
}

// The stored row is where the truth lives; echoing the request input would hide
// whatever the scheduler wrote concurrently, so the response is read back.
func TestUpdateTaskReturnsTheFreshRow(t *testing.T) {
	srv, cfg := newTestServer(t)
	task := newTestTask(t, srv)

	rec := do(t, srv, cfg, http.MethodPut, "/api/ddns/tasks/"+itoa(task.ID), taskPayload(map[string]any{
		"domain": "changed.example.com",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var updated struct {
		Data model.DDNSTask `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Data.Domain != "changed.example.com" {
		t.Fatalf("domain = %q, want changed.example.com", updated.Data.Domain)
	}
}

// The form says an empty host is the apex, and the editor has always stored it
// that way. Create used to validate before normalising, so the documented way
// to ask for the apex was the one way that could not be sent.
func TestCreateTaskTreatsAnEmptyHostAsTheApex(t *testing.T) {
	srv, cfg := newTestServer(t)
	newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/ddns/tasks", taskPayload(map[string]any{
		"sub_domain": "",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var stored model.DDNSTask
	if err := srv.store.DB().First(&stored).Error; err != nil {
		t.Fatalf("load the created task: %v", err)
	}
	if stored.SubDomain != "@" {
		t.Fatalf("sub_domain = %q, want @", stored.SubDomain)
	}
}

// to hand a negative one to DNSPod.
func TestCreateTaskRefusesANonPositiveTTL(t *testing.T) {
	srv, cfg := newTestServer(t)

	created := do(t, srv, cfg, http.MethodPost, "/api/accounts", map[string]any{
		"name": "a", "secret_id": "id", "secret_key": "key",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create account: want 201, got %d (%s)", created.Code, created.Body.String())
	}
	var account struct {
		Data model.Account `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, ttl := range []int{-5, 0} {
		rec := do(t, srv, cfg, http.MethodPost, "/api/ddns/tasks", map[string]any{
			"account_id": account.Data.ID,
			"domain":     "example.com",
			"sub_domain": "a",
			"ttl":        ttl,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create with ttl %d: want 201, got %d (%s)", ttl, rec.Code, rec.Body.String())
		}
		var body struct {
			Data model.DDNSTask `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Data.TTL != 600 {
			t.Fatalf("ttl %d stored as %d, want the default 600", ttl, body.Data.TTL)
		}
	}
}
