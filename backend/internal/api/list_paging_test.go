package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ddns-by-dnspod/internal/model"
)

// Every stored list is paged the same way, so page 2 of the accounts means what
// page 2 of the tasks means: the same envelope, the same caps, the same order.
func TestStoredListsArePagedAlike(t *testing.T) {
	srv, cfg := newTestServer(t)

	for i := 1; i <= 3; i++ {
		name := "a" + string(rune('0'+i))
		rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", map[string]any{
			"name": name, "secret_id": "id", "secret_key": "key",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s: want 201, got %d (%s)", name, rec.Code, rec.Body.String())
		}
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/accounts?page=2&page_size=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	items := decodeList[model.Account](t, rec.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("page 2 of 3 rows at size 2 holds %d rows, want 1", len(items))
	}

	var page struct {
		Data struct {
			Total    int64 `json:"total"`
			Page     int   `json:"page"`
			PageSize int   `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Data.Total != 3 || page.Data.Page != 2 || page.Data.PageSize != 2 {
		t.Fatalf("page = %+v, want total 3, page 2, size 2", page.Data)
	}

	// A page past the end is empty rather than an error: a list that shrank
	// under a caller's fingers is a normal thing to read.
	rec = do(t, srv, cfg, http.MethodGet, "/api/accounts?page=9&page_size=2", nil)
	if got := decodeList[model.Account](t, rec.Body.Bytes()); len(got) != 0 {
		t.Fatalf("page 9 holds %d rows, want 0", len(got))
	}
}

// A page size is capped rather than obeyed: the same ceiling every other list
// has, so one call cannot ask a small table to walk the whole file.
func TestStoredListsCapThePageSize(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/accounts?page_size=99999", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var page struct {
		Data struct {
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Data.PageSize != maxPageSize {
		t.Fatalf("page_size = %d, want the cap %d", page.Data.PageSize, maxPageSize)
	}
}

// A TTL that is not a positive number means "not specified" on every path into
// a task: the import path and the update already said so, and the create used

// A status DNSPod would reject is refused here, before the call. An empty one

// An empty page is an empty array, never null.
//
// The console walks the pages with `items.push(...chunk.items)`, which throws
// on null and takes the whole table down instead of showing an empty state.
func TestEmptyPageIsAnEmptyArray(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/accounts", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("an empty page is not an empty array: %s", rec.Body.String())
	}
}
