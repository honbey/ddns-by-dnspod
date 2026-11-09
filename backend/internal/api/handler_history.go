package api

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/store"
)

// historyPageSize is the default page size of both history endpoints. It is
// larger than the other lists' because a timeline is read in context, and it
// matches the contract's PageSize parameter.
const historyPageSize = 20

// listHistory returns the change history, filterable by task, domain and status, with pagination
func (s *Server) listHistory(c *gin.Context) {
	params := parsePageParamsWith(c, historyPageSize)

	q := s.store.DB().Model(&model.DDNSHistory{})

	if v := c.Query("task_id"); v != "" {
		// Validate before it reaches the query builder: the value is numeric by contract
		taskID, err := strconv.ParseUint(v, 10, 64)
		if err != nil || taskID == 0 {
			badRequest(c, "task_id must be a positive integer")
			return
		}
		q = q.Where("task_id = ?", taskID)
	}
	if v := c.Query("domain"); v != "" {
		q = q.Where("domain = ?", v)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}

	var items []model.DDNSHistory
	err := q.Order("id DESC").
		Offset(params.offset()).
		Limit(params.PageSize).
		Find(&items).Error
	if err != nil {
		serverError(c, err)
		return
	}

	paged(c, items, total, params)
}

// ipChangeStats returns recent public IP changes of this host, for charting.
//
// Only real changes are returned: the first row of a family has no previous
// address to compare against, so it carries no duration and would plot a
// misleading zero-height bar. It still has to be stored, because it is the
// baseline the next change measures its lifetime against.
func (s *Server) ipChangeStats(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 50)
	if limit > 200 {
		limit = 200
	}

	family, valid := familyQuery(c)
	if !valid {
		badRequest(c, "family must be v4 or v6")
		return
	}

	q := s.store.DB().Where("old_ip <> ''")
	if family != "" {
		q = q.Where("family = ?", family)
	}

	var items []model.PublicIPChange
	// By instant: an import rebuilds a family in timeline order, so id order
	// agrees with it today, but the chart should not depend on that.
	err := q.Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		serverError(c, err)
		return
	}

	// The chart reads better in chronological order
	points := make([]gin.H, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		points = append(points, gin.H{
			"time":         it.CreatedAt,
			"family":       it.Family,
			"old_ip":       it.OldIP,
			"new_ip":       it.NewIP,
			"duration_sec": it.DurationSec,
		})
	}

	ok(c, points)
}

// listIPHistory is the paged, newest-first version of the address timeline.
// The chart endpoint returns a short window in chronological order; this one
// carries every row, including the baseline, because a table is read top down.
func (s *Server) listIPHistory(c *gin.Context) {
	params := parsePageParamsWith(c, historyPageSize)
	family, valid := familyQuery(c)
	if !valid {
		badRequest(c, "family must be v4 or v6")
		return
	}

	q := s.store.DB().Model(&model.PublicIPChange{})
	if family != "" {
		q = q.Where("family = ?", family)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}

	var items []model.PublicIPChange
	err := q.Order("created_at DESC, id DESC").
		Offset(params.offset()).
		Limit(params.PageSize).
		Find(&items).Error
	if err != nil {
		serverError(c, err)
		return
	}

	paged(c, items, total, params)
}

// familyQuery reads the address family filter, empty meaning both timelines.
// A value the contract does not allow is refused here rather than answered
// with an empty page, which would read as "this family has no history".
func familyQuery(c *gin.Context) (string, bool) {
	switch v := strings.ToLower(strings.TrimSpace(c.Query("family"))); v {
	case "":
		return "", true
	case model.IPFamilyV4, model.IPFamilyV6:
		return v, true
	default:
		return "", false
	}
}

const (
	// One file is a history, not a database dump. The request body cap already
	// limits the bytes; this limits the work a single import can ask for.
	ipHistoryImportMaxRows = 10000
	// Bad rows are reported, not fatal, so the rest of a file still lands. The
	// list is capped so a malformed export cannot turn the answer into a file.
	ipHistoryImportMaxErrors = 20
)

// Codes the SPA translates; adding one is a contract change.
const (
	ipImportTimeInvalid    = "time_invalid"
	ipImportTimeFuture     = "time_future"
	ipImportTimeTooOld     = "time_too_old"
	ipImportIPInvalid      = "ip_invalid"
	ipImportFamilyInvalid  = "family_invalid"
	ipImportFamilyMismatch = "family_mismatch"
	ipImportRowInvalid     = "row_invalid"
	ipImportDuplicateRow   = "duplicate_row"
)

type ipHistoryImportError struct {
	Line int    `json:"line"`
	Code string `json:"code"`
	// Message is English and for logs: the SPA renders Code, so a localized
	// interface does not turn into a bilingual one at the toast.
	Message string `json:"message"`
}

// parsedIPHistoryRow keeps the file line next to the sample. The store hands
// back indexes into the parsed slice, and the line is the only part a person
// can act on.
type parsedIPHistoryRow struct {
	sample store.IPHistorySample
	line   int
}

// importIPHistory reads a CSV of `time,ip[,family]` and merges it into the
// address timeline. Bad rows are skipped and reported; only a file that cannot
// be read at all is refused.
func (s *Server) importIPHistory(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		// A body over the request cap reaches the multipart reader as this
		// error, not as a missing field: telling the caller to check their form
		// would send them looking in the wrong place. The answer is a 400
		// rather than the JSON endpoints' 413 because the contract declares no
		// 413, and a smaller file is something the caller can fix.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			badRequest(c, fmt.Sprintf("the uploaded file is larger than %d bytes", tooLarge.Limit))
			return
		}
		badRequest(c, "a CSV file is required in the file field")
		return
	}
	opened, err := file.Open()
	if err != nil {
		badRequest(c, "the CSV file could not be read")
		return
	}
	defer opened.Close()

	rows, rowErrors, invalid, reason := parseIPHistoryCSV(opened)
	if reason != "" {
		// The reasons are written here, not taken from the decoder: a CSV parse
		// error names the caller's own line and is theirs to see.
		badRequest(c, reason)
		return
	}
	if len(rows) == 0 {
		badRequest(c, "the CSV file contains no usable rows")
		return
	}

	samples := make([]store.IPHistorySample, len(rows))
	for i, row := range rows {
		samples[i] = row.sample
	}

	imported, skippedSamples, err := s.store.ImportPublicIPHistory(samples)
	if err != nil {
		serverError(c, err)
		return
	}

	// A sample the timeline already held is reported like a refused row: the
	// caller gave us a line and it did not land, so silence would leave them
	// hunting for it.
	duplicates := make([]ipHistoryImportError, 0, len(skippedSamples))
	for _, index := range skippedSamples {
		duplicates = append(duplicates, ipHistoryImportError{
			Line:    rows[index].line,
			Code:    ipImportDuplicateRow,
			Message: "already in the timeline",
		})
	}

	// Each source keeps its own budget: a block of refused rows early in the
	// file would otherwise fill the list and hide every duplicate behind it.
	sort.SliceStable(duplicates, func(i, j int) bool { return duplicates[i].Line < duplicates[j].Line })
	if len(rowErrors) > ipHistoryImportMaxErrors {
		rowErrors = rowErrors[:ipHistoryImportMaxErrors]
	}
	if len(duplicates) > ipHistoryImportMaxErrors {
		duplicates = duplicates[:ipHistoryImportMaxErrors]
	}

	// One list in file order: it is read against the caller's own file, so the
	// order the two sources were discovered in is noise.
	rowErrors = append(rowErrors, duplicates...)
	sort.SliceStable(rowErrors, func(i, j int) bool { return rowErrors[i].Line < rowErrors[j].Line })

	ok(c, gin.H{
		"imported": imported,
		// A row refused here is still a row the caller gave us and we did not
		// store, so it belongs in the number the success message reports.
		"skipped": len(skippedSamples) + invalid,
		"errors":  rowErrors,
	})
}

// parseIPHistoryCSV reads `time,ip[,family]` with or without a header.
//
// A header is recognised by naming a time and an address column, and may use
// the aliases below; without one the order is fixed. family may be left out,
// in which case the address decides it - the common export from another tool
// is a timestamp and an address, and asking for a column it does not have
// would only make the file be edited by hand.
//
// invalid counts every refused row; rowErrors only carries the first few, so a
// malformed export cannot turn the answer into another file.
func parseIPHistoryCSV(r io.Reader) (
	rows []parsedIPHistoryRow,
	rowErrors []ipHistoryImportError,
	invalid int,
	reason string,
) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	first, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, 0, "the CSV file is empty"
		}
		return nil, nil, 0, "the CSV file is not valid CSV"
	}

	// Excel and Windows tools leave a byte order mark in front of the first
	// field. It is not whitespace, so without this the header would be read as
	// a data row and the time column would fail to parse.
	if len(first) > 0 {
		first[0] = strings.TrimPrefix(first[0], "\ufeff")
	}

	columns := headerColumns(first)
	startLine := 2
	if columns == nil {
		columns = map[string]int{"time": 0, "ip": 1, "family": 2}
		startLine = 1
	}

	rows = make([]parsedIPHistoryRow, 0, 64)
	rowErrors = make([]ipHistoryImportError, 0)
	seen := 0
	// One clock for the whole file: a boundary that moves between rows would
	// make the same upload succeed or fail depending on how large it is.
	now := time.Now()

	// Every refused row is kept here and the cap is applied once, after the
	// store has also reported its skips. Truncating per source would let a
	// block of bad rows early in the file hide every duplicate further down.
	reject := func(line int, code, message string) {
		invalid++
		rowErrors = append(rowErrors, ipHistoryImportError{
			Line:    line,
			Code:    code,
			Message: message,
		})
	}
	collect := func(record []string, line int) {
		sample, code, why := sampleFromRow(record, columns, now)
		if code != "" {
			reject(line, code, why)
			return
		}
		rows = append(rows, parsedIPHistoryRow{sample: sample, line: line})
	}
	overLimit := func() bool {
		if seen > ipHistoryImportMaxRows {
			reason = fmt.Sprintf("the CSV file has more than %d rows", ipHistoryImportMaxRows)
			return true
		}
		return false
	}

	if startLine == 1 {
		seen++
		collect(first, 1)
	}
	line := startLine - 1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) {
				// A parse error whose failure line is past the record's start
				// means the reader is still inside an unterminated quote at
				// EOF: everything after StartLine was swallowed, so reporting
				// one bad row and a partial success would hide the loss.
				if parseErr.Line > parseErr.StartLine {
					return nil, nil, 0, fmt.Sprintf(
						"a quoted field starting on line %d is not terminated; the rest of the file was not read",
						parseErr.StartLine)
				}
				if parseErr.StartLine > 0 {
					line = parseErr.StartLine
				}
			}
			seen++
			reject(line, ipImportRowInvalid, "the row is not valid CSV")
			if overLimit() {
				return nil, nil, 0, reason
			}
			continue
		}
		// FieldPos reports the physical line the record starts on, so a field
		// with an embedded newline still points at the row a person sees.
		if at, _ := reader.FieldPos(0); at > 0 {
			line = at
		}
		seen++
		if overLimit() {
			return nil, nil, 0, reason
		}
		collect(record, line)
	}

	return rows, rowErrors, invalid, ""
}

// headerColumns maps the aliases a header may use onto the three columns, or
// returns nil when the row is data rather than a header.
func headerColumns(record []string) map[string]int {
	columns := make(map[string]int, 3)
	for i, field := range record {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "time", "timestamp", "at":
			columns["time"] = i
		case "ip", "address", "new_ip":
			columns["ip"] = i
		case "family":
			columns["family"] = i
		}
	}
	if _, ok := columns["time"]; !ok {
		return nil
	}
	if _, ok := columns["ip"]; !ok {
		return nil
	}
	return columns
}

// ipHistoryEarliest is the oldest instant an import may carry. It is a fixed
// date rather than a window relative to now: the same file has to import the
// same way today and years from now, and a DNS record cannot predate the API
// this history describes.
var ipHistoryEarliest = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// sampleFromRow validates one row and returns either a sample or the code and
// English wording of the reason it was refused. The code is what the SPA
// translates; the wording is for the log.
func sampleFromRow(record []string, columns map[string]int, now time.Time) (store.IPHistorySample, string, string) {
	cell := func(name string) string {
		index, ok := columns[name]
		if !ok || index < 0 || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}

	at, err := parseHistoryTime(cell("time"))
	if err != nil {
		return store.IPHistorySample{}, ipImportTimeInvalid,
			"time must be RFC3339 or 2006-01-02 15:04:05"
	}
	// A future instant would become the row the next DDNS run compares against:
	// the real change after it would measure a negative lifetime, and an
	// address that matches would be swallowed as unchanged.
	if at.After(now.Add(time.Minute)) {
		return store.IPHistorySample{}, ipImportTimeFuture, "time is in the future"
	}
	if at.Before(ipHistoryEarliest) {
		return store.IPHistorySample{}, ipImportTimeTooOld, "time is before 2000-01-01"
	}
	parsed := net.ParseIP(cell("ip"))
	if parsed == nil {
		return store.IPHistorySample{}, ipImportIPInvalid, "ip must be an IPv4 or IPv6 address"
	}

	family := model.IPFamilyV4
	if parsed.To4() == nil {
		family = model.IPFamilyV6
	}
	switch declared := strings.ToLower(cell("family")); declared {
	case "":
	case model.IPFamilyV4, model.IPFamilyV6:
		if declared != family {
			return store.IPHistorySample{}, ipImportFamilyMismatch, "family does not match the address"
		}
	default:
		return store.IPHistorySample{}, ipImportFamilyInvalid, "family must be v4 or v6"
	}

	return store.IPHistorySample{Family: family, IP: parsed.String(), At: at}, "", ""
}

// parseHistoryTime accepts what an export usually carries: RFC 3339 with an
// offset, or a plain date and time.
//
// A plain one has no zone to read, so it is taken as UTC rather than the
// server's local zone: the container is usually UTC, and the same file must
// land on the same instants wherever it is imported. The contract says so too.
func parseHistoryTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}
	if at, err := time.Parse(time.RFC3339, value); err == nil {
		return at, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if at, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return at, nil
		}
	}
	return time.Time{}, errors.New("unrecognized time")
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
