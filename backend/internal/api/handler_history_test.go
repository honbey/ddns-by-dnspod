package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ddns-by-dnspod/internal/model"
)

type ipChangePoint struct {
	Family      string `json:"family"`
	OldIP       string `json:"old_ip"`
	NewIP       string `json:"new_ip"`
	DurationSec *int64 `json:"duration_sec"`
}

func decodeIPChanges(t *testing.T, rec *httptest.ResponseRecorder) []ipChangePoint {
	t.Helper()
	var got struct {
		Data []ipChangePoint `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return got.Data
}

// The first row of a family only establishes the baseline: it has no previous
// address, so it carries no duration and plotting it would draw a zero-height
// bar that looks like an instant change.
func TestIPChangeStatsSkipsTheBaseline(t *testing.T) {
	srv, cfg := newTestServer(t)

	duration := int64(3600)
	rows := []model.PublicIPChange{
		{Family: model.IPFamilyV4, NewIP: "1.1.1.1"},
		{Family: model.IPFamilyV4, OldIP: "1.1.1.1", NewIP: "2.2.2.2", DurationSec: &duration},
	}
	for i := range rows {
		if err := srv.store.DB().Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed ip changes: %v", err)
		}
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/stats/ip-changes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	points := decodeIPChanges(t, rec)
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	if points[0].OldIP != "1.1.1.1" || points[0].NewIP != "2.2.2.2" {
		t.Fatalf("point = %+v, want 1.1.1.1 -> 2.2.2.2", points[0])
	}
	if points[0].DurationSec == nil || *points[0].DurationSec != duration {
		t.Fatalf("duration = %v, want %d", points[0].DurationSec, duration)
	}
}

// Filtering the chart by family must reach the query rather than being applied
// client side, otherwise a busy history would send every row to the browser.
func TestIPChangeStatsFiltersByFamily(t *testing.T) {
	srv, cfg := newTestServer(t)

	rows := []model.PublicIPChange{
		{Family: model.IPFamilyV4, OldIP: "1.1.1.1", NewIP: "2.2.2.2"},
		{Family: model.IPFamilyV6, OldIP: "2001:db8::1", NewIP: "2001:db8::2"},
	}
	for i := range rows {
		if err := srv.store.DB().Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed ip changes: %v", err)
		}
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/stats/ip-changes?family=v6", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	points := decodeIPChanges(t, rec)
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	if points[0].Family != model.IPFamilyV6 {
		t.Fatalf("family = %q, want %q", points[0].Family, model.IPFamilyV6)
	}
}

// With no history the endpoint answers an empty list rather than null, so the
// frontend can iterate it without a null check.
func TestIPChangeStatsWithoutHistory(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/stats/ip-changes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeIPChanges(t, rec); len(got) != 0 {
		t.Fatalf("points = %v, want an empty list", got)
	}
}
