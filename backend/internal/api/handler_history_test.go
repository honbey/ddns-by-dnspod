package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

// doCSV posts one file through the real route table, the way the import button
// does: a multipart form with the file under the field the contract names.
func doCSV(t *testing.T, srv *Server, cfg *config.Config, path, content string) *httptest.ResponseRecorder {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "history.csv")
	if err != nil {
		t.Fatalf("create the form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write the form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close the form: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.MaxBody(middleware.DefaultMaxBody))
	srv.RegisterRoutes(r, *cfg)

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

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

type ipHistoryPage struct {
	Items    []model.PublicIPChange `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

func decodeIPHistory(t *testing.T, rec *httptest.ResponseRecorder) ipHistoryPage {
	t.Helper()
	var got struct {
		Data ipHistoryPage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return got.Data
}

// The table shows the baseline the chart leaves out: a first row is a fact
// about when the address was first seen, and only the chart has a reason to
// hide it.
func TestListIPHistoryIncludesTheBaseline(t *testing.T) {
	srv, cfg := newTestServer(t)

	duration := int64(3600)
	rows := []model.PublicIPChange{
		{Family: model.IPFamilyV4, NewIP: "1.1.1.1", CreatedAt: time.Unix(1700000000, 0)},
		{Family: model.IPFamilyV4, OldIP: "1.1.1.1", NewIP: "2.2.2.2", DurationSec: &duration, CreatedAt: time.Unix(1700003600, 0)},
	}
	for i := range rows {
		if err := srv.store.DB().Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed ip history: %v", err)
		}
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/history/ip-changes?page=1&page_size=20", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	page := decodeIPHistory(t, rec)
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("total = %d, items = %d, want 2 and 2", page.Total, len(page.Items))
	}
	if page.Items[0].NewIP != "2.2.2.2" {
		t.Errorf("first item = %+v, want the newest change", page.Items[0])
	}
	if page.Items[1].OldIP != "" || page.Items[1].NewIP != "1.1.1.1" {
		t.Errorf("second item = %+v, want the baseline", page.Items[1])
	}
}

// The import accepts the shape another tool exports: a timestamp and an
// address, no family column. The address decides the family.
func TestImportIPHistoryCSV(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "time,ip\n" +
		"2026-01-01T00:00:00Z,1.1.1.1\n" +
		"2026-01-02T00:00:00Z,2.2.2.2\n" +
		"2026-01-03T00:00:00Z,2001:db8::1\n"
	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Line    int    `json:"line"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 3 || got.Data.Skipped != 0 || len(got.Data.Errors) != 0 {
		t.Fatalf("result = %+v, want 3 imported and no errors", got.Data)
	}

	var v6 []model.PublicIPChange
	if err := srv.store.DB().Where("family = ?", model.IPFamilyV6).Find(&v6).Error; err != nil {
		t.Fatalf("load the v6 history: %v", err)
	}
	if len(v6) != 1 || v6[0].NewIP != "2001:db8::1" {
		t.Fatalf("v6 rows = %+v, want 2001:db8::1", v6)
	}
}

// A bad row is the caller's to fix, but it must not discard the good ones: the
// answer says which lines were refused and the rest still lands.
func TestImportIPHistoryReportsBadRows(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "time,ip\n" +
		"2026-01-01T00:00:00Z,1.1.1.1\n" +
		"not-a-time,2.2.2.2\n" +
		"2026-01-02T00:00:00Z,not-an-ip\n"
	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int              `json:"imported"`
			Skipped  int              `json:"skipped"`
			Errors   []map[string]any `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 1 || got.Data.Skipped != 2 || len(got.Data.Errors) != 2 {
		t.Fatalf("result = %+v, want 1 imported, 2 skipped and 2 errors", got.Data)
	}
	if got.Data.Errors[0]["line"] != float64(3) || got.Data.Errors[1]["line"] != float64(4) {
		t.Errorf("error lines = %v and %v, want 3 and 4", got.Data.Errors[0]["line"], got.Data.Errors[1]["line"])
	}
	if got.Data.Errors[0]["code"] != "time_invalid" || got.Data.Errors[1]["code"] != "ip_invalid" {
		t.Errorf("codes = %v and %v, want time_invalid and ip_invalid",
			got.Data.Errors[0]["code"], got.Data.Errors[1]["code"])
	}
}

// A future timestamp would become the row the next DDNS run compares against,
// so it is refused while the rest of the file still lands.
func TestImportIPHistoryRejectsFutureTime(t *testing.T) {
	srv, cfg := newTestServer(t)

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	content := "time,ip\n" + past + ",1.1.1.1\n" + future + ",2.2.2.2\n"

	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 1 || got.Data.Skipped != 1 {
		t.Fatalf("result = %+v, want 1 imported and 1 skipped", got.Data)
	}
	if len(got.Data.Errors) != 1 || got.Data.Errors[0].Code != "time_future" {
		t.Fatalf("errors = %+v, want the future row to be refused", got.Data.Errors)
	}
}

// Without a header the columns are time, ip, family; the family is optional
// and must agree with the address when it is there.
func TestImportIPHistoryWithoutHeader(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "2026-01-01T00:00:00Z,1.1.1.1\n" +
		"2026-01-02T00:00:00Z,2001:db8::1,v6\n" +
		"2026-01-03T00:00:00Z,2.2.2.2,v6\n"
	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 2 || got.Data.Skipped != 1 {
		t.Fatalf("result = %+v, want 2 imported and 1 skipped", got.Data)
	}
	if len(got.Data.Errors) != 1 || got.Data.Errors[0].Code != "family_mismatch" {
		t.Fatalf("errors = %+v, want the mismatched family to be refused", got.Data.Errors)
	}
}

// A record cannot predate the API this history describes. The lower bound is a
// fixed date, so the same file imports the same way in any year.
func TestImportIPHistoryRejectsAncientTime(t *testing.T) {
	srv, cfg := newTestServer(t)

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	content := "time,ip\n" + past + ",1.1.1.1\n1999-12-31T23:59:59Z,2.2.2.2\n"

	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 1 || got.Data.Skipped != 1 {
		t.Fatalf("result = %+v, want 1 imported and 1 skipped", got.Data)
	}
	if len(got.Data.Errors) != 1 || got.Data.Errors[0].Code != "time_too_old" {
		t.Fatalf("errors = %+v, want the ancient row to be refused", got.Data.Errors)
	}
}

// A sample the timeline already holds is reported with its line, not only
// counted: a bare skipped number leaves the user hunting for the row.
func TestImportIPHistoryReportsDuplicateRows(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "time,ip\n2026-01-01T00:00:00Z,1.1.1.1\n"
	if rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content); rec.Code != http.StatusOK {
		t.Fatalf("first import: %d (%s)", rec.Code, rec.Body.String())
	}

	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import: %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Line int    `json:"line"`
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 0 || got.Data.Skipped != 1 {
		t.Fatalf("result = %+v, want 0 imported and 1 skipped", got.Data)
	}
	if len(got.Data.Errors) != 1 || got.Data.Errors[0].Line != 2 {
		t.Fatalf("errors = %+v, want the duplicate on line 2", got.Data.Errors)
	}
	if got.Data.Errors[0].Code != "duplicate_row" {
		t.Fatalf("code = %q, want duplicate_row", got.Data.Errors[0].Code)
	}
}

// A block of refused rows must not hide the duplicates behind it: each source
// has its own budget before the two are merged into file order.
func TestImportIPHistoryKeepsDuplicatesBehindManyBadRows(t *testing.T) {
	srv, cfg := newTestServer(t)

	good := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + ",203.0.113.88"
	first := "time,ip\n" + good + "\n"
	if rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", first); rec.Code != http.StatusOK {
		t.Fatalf("seed import: %d (%s)", rec.Code, rec.Body.String())
	}

	// 25 rows the parser refuses, then the same address again: the duplicate's
	// line number is larger than every refused one.
	var content strings.Builder
	content.WriteString("time,ip\n")
	for i := 0; i < 25; i++ {
		content.WriteString("bad-time,1.1.1.1\n")
	}
	content.WriteString(good + "\n")

	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Line int    `json:"line"`
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 0 || got.Data.Skipped != 26 {
		t.Fatalf("result = %+v, want 0 imported and 26 skipped", got.Data)
	}
	if len(got.Data.Errors) != 21 {
		t.Fatalf("errors = %d, want 20 refused rows plus the duplicate", len(got.Data.Errors))
	}
	last := got.Data.Errors[len(got.Data.Errors)-1]
	if last.Line != 27 || last.Code != "duplicate_row" {
		t.Fatalf("last error = %+v, want the duplicate on line 27", last)
	}
	for i := 1; i < len(got.Data.Errors); i++ {
		if got.Data.Errors[i].Line < got.Data.Errors[i-1].Line {
			t.Fatalf("errors are not in file order at %d", i)
		}
	}
}

// The request cap reaches the multipart reader as an oversized-body error, not
// as a missing field: the answer has to name the real problem.
func TestImportIPHistoryRejectsOversizedBody(t *testing.T) {
	srv, cfg := newTestServer(t)

	big := "time,ip\n" + strings.Repeat("a", int(middleware.DefaultMaxBody)+1024)
	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", big)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if message := errorMessage(t, rec); !strings.Contains(message, "larger than") {
		t.Fatalf("message = %q, want it to name the size limit", message)
	}
}

// Excel puts a byte order mark in front of the first cell. It is not
// whitespace, so the header would otherwise be read as a data row.
func TestImportIPHistoryAcceptsByteOrderMark(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "\ufefftime,ip\n" +
		"2026-01-01T00:00:00Z,1.1.1.1\n" +
		"2026-01-02T00:00:00Z,2.2.2.2\n"
	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Data struct {
			Imported int `json:"imported"`
			Skipped  int `json:"skipped"`
			Errors   []struct {
				Code string `json:"code"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if got.Data.Imported != 2 || got.Data.Skipped != 0 || len(got.Data.Errors) != 0 {
		t.Fatalf("result = %+v, want both rows imported and no errors", got.Data)
	}
}

// An unterminated quote makes the CSV reader consume the rest of the file, so
// a partial success would silently drop every row after it. The upload is
// refused instead.
func TestImportIPHistoryRejectsUnterminatedQuote(t *testing.T) {
	srv, cfg := newTestServer(t)

	content := "time,ip\n" +
		"2026-01-01T00:00:00Z,1.1.1.1\n" +
		"\"2026-01-02T00:00:00Z,2.2.2.2\n" +
		"2026-01-03T00:00:00Z,3.3.3.3\n"

	rec := doCSV(t, srv, cfg, "/api/history/ip-changes/import", content)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	message := errorMessage(t, rec)
	if !strings.Contains(message, "not terminated") {
		t.Fatalf("message = %q, want it to name the unterminated field", message)
	}
}

// A request with no file, and a file with no usable row, are both the caller's
// mistake and both answer 400 rather than an empty success.
func TestImportIPHistoryRefusesUnusableInput(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/history/ip-changes/import", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no file -> %d, want 400 (%s)", rec.Code, rec.Body.String())
	}

	rec = doCSV(t, srv, cfg, "/api/history/ip-changes/import", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty file -> %d, want 400 (%s)", rec.Code, rec.Body.String())
	}

	rec = doCSV(t, srv, cfg, "/api/history/ip-changes/import", "time,ip\nnope,nope\n")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no usable row -> %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}
