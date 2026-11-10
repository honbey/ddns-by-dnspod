package api

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/model"
)

// recordInput is the payload for creating or updating a record
type recordInput struct {
	SubDomain  string `json:"sub_domain"`
	RecordType string `json:"record_type"`
	Value      string `json:"value"`
	Line       string `json:"line"`
	TTL        uint64 `json:"ttl"`
	MX         uint64 `json:"mx"`
	Weight     uint64 `json:"weight"`
	Status     string `json:"status"`
	Remark     string `json:"remark"`
}

// validateStatus refuses a status DNSPod would not accept.
//
// The empty value is "not specified": the client layer leaves it out of the
// request, so a caller that means "keep what is there" sends nothing. Anything
// else has to be one of the two the provider knows - a typo would otherwise
// travel all the way there and come back as an upstream failure.
func validateStatus(status string) error {
	if status == "" {
		return nil
	}
	if status != dnspod.StatusEnable && status != dnspod.StatusDisable {
		return fmt.Errorf("status must be %s or %s", dnspod.StatusEnable, dnspod.StatusDisable)
	}
	return nil
}

// priorityRequired names the record types that cannot be written without a
// priority. DNSPod rejects them without one, and a zero would reset the
// priority an existing record already has, so both are refused here.
var priorityRequired = map[string]bool{
	"MX": true, "HTTPS": true, "SVCB": true,
}

// errRecordFieldsRequired and errRecordFieldsAndLineRequired are the two
// wordings the API has answered a record missing a field with. They are values
// rather than string literals because the batch path answers with the first one
// plus its own suffix.
var (
	errRecordFieldsRequired        = errors.New("sub_domain, record_type and value are required")
	errRecordFieldsAndLineRequired = errors.New("sub_domain, record_type, value and line are required")
)

// prepareInput normalises a record payload in place and reports what is wrong
// with it, if anything.
//
// One function for the three endpoints that write a record, rather than the
// same four lines in each: the host rule used to live in two of them with a
// comment asking the reader to keep them in step.
//
// requireLine is the one real difference. An update has to name the line the
// record lives on, because ModifyRecord writes it unconditionally; a create may
// leave it blank, which still means "the default".
//
// Value is never trimmed - the spaces inside a TXT value are part of the value -
// so it is only tested for emptiness and sent as typed.
func prepareInput(in *recordInput, requireLine bool) error {
	in.SubDomain = strings.TrimSpace(in.SubDomain)
	in.RecordType = strings.TrimSpace(in.RecordType)
	in.Line = strings.TrimSpace(in.Line)

	// The empty host is the one that matters most: DNSPod has no idea which host
	// to file the record under when it is blank, and an update would send it as
	// an empty string rather than leaving the record alone.
	if in.SubDomain == "" || in.RecordType == "" || strings.TrimSpace(in.Value) == "" ||
		(requireLine && in.Line == "") {
		if requireLine {
			return errRecordFieldsAndLineRequired
		}
		return errRecordFieldsRequired
	}
	// The rule the task endpoints already apply, for the reason model/validate.go
	// gives: a host reaches the server log and the change history, so a newline
	// or an illegal character in it forges a log line.
	if err := model.ValidateSubDomain(in.SubDomain); err != nil {
		return err
	}
	return validateStatus(in.Status)
}

// pathDomain reads the domain out of the path and checks it.
//
// Held to the same rule as a typed domain: the value goes into the log, and an
// unescaped line break in it is another log line. The import endpoint already
// did this, the record endpoints did not.
// An empty return means the response has already been written. It is not a
// boolean pair because that would shadow the ok() answer helper.
func pathDomain(c *gin.Context) string {
	domain := c.Param("domain")
	if err := model.ValidateDomain(domain); err != nil {
		badRequest(c, "invalid domain")
		return ""
	}
	return domain
}

// maxBatchItems caps how many records one batch call may touch.
//
// The calls are issued one at a time against DNSPod, so an unbounded list would
// keep a single request busy for minutes and eat through the API rate limit.
const maxBatchItems = 100

// batchSizeOK reports whether a list fits the cap, answering the caller when it
// does not.
//
// Every path that takes a list of records shares the cap and the sentence, so a
// caller who has met the limit on one screen recognises it on the next. hint is
// what this path in particular can do about it - an import narrows its group -
// and is empty where the caller is looking at the list they just sent.
func batchSizeOK(c *gin.Context, n int, hint string) bool {
	if n <= maxBatchItems {
		return true
	}
	msg := fmt.Sprintf("at most %d items per batch", maxBatchItems)
	if hint != "" {
		msg += ", " + hint
	}
	badRequest(c, msg)
	return false
}

// batchCreateInput is the payload for creating several subdomains at once.
// min=1 because binding:"required" only asserts that a slice is not nil, so an
// empty list would otherwise pass and leave the request doing nothing.
type batchCreateInput struct {
	Items []recordInput `json:"items" binding:"required,min=1"`
}

// batchDeleteInput is the payload for batch deletion
type batchDeleteInput struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

// listDomains lists domains
func (s *Server) listDomains(c *gin.Context) {
	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	domains, err := cli.ListDomains(c.Request.Context())
	if err != nil {
		upstreamError(c, err)
		return
	}
	ok(c, domains)
}

// listDomainGroups lists the account's domain groups.
//
// Unlike record groups, these span domains: the domain list reports one group
// id per domain, and this list is what turns that id into a name the UI can
// show and filter on.
func (s *Server) listDomainGroups(c *gin.Context) {
	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	groups, err := cli.ListDomainGroups(c.Request.Context())
	if err != nil {
		upstreamError(c, err)
		return
	}
	ok(c, groups)
}

// listRecords lists records, filterable by host, type and keyword
func (s *Server) listRecords(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	params := parsePageParams(c)
	page, err := cli.ListRecords(c.Request.Context(), domain, dnspod.RecordFilter{
		SubDomain:  c.Query("sub_domain"),
		RecordType: c.Query("record_type"),
		Keyword:    c.Query("keyword"),
		GroupID:    parseGroupID(c),
		Offset:     uint64(params.offset()),
		Limit:      uint64(params.PageSize),
	})
	if err != nil {
		upstreamError(c, err)
		return
	}
	paged(c, page.Records, int64(page.Total), params)
}

// listRecordGroups lists the record groups of a domain.
//
// The record list API does not report which group a record belongs to, so a
// group can only be shown by asking for it explicitly. This endpoint exists so
// the frontend can offer a group filter and import a whole group at once.
func (s *Server) listRecordGroups(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	groups, err := cli.ListRecordGroups(domain)
	if err != nil {
		upstreamError(c, err)
		return
	}
	ok(c, groups)
}

// listRecordLines lists the record lines a domain supports, for the frontend dropdown
func (s *Server) listRecordLines(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	lines, err := cli.ListRecordLines(c.Request.Context(), domain)
	if err != nil {
		upstreamError(c, err)
		return
	}
	ok(c, lines)
}

// createRecord creates a record
func (s *Server) createRecord(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	var input recordInput
	if !bindJSON(c, &input) {
		return
	}
	if err := prepareInput(&input, false); err != nil {
		badRequest(c, err.Error())
		return
	}
	if priorityRequired[strings.ToUpper(input.RecordType)] && input.MX == 0 {
		badRequest(c, "mx is required for MX, HTTPS and SVCB records")
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	recordID, err := cli.CreateRecord(domain, dnspod.CreateRecordInput{
		SubDomain:  input.SubDomain,
		RecordType: input.RecordType,
		Value:      input.Value,
		Line:       defaultLine(input.Line),
		TTL:        input.TTL,
		MX:         input.MX,
		Weight:     input.Weight,
		Status:     input.Status,
		Remark:     input.Remark,
	})
	if err != nil {
		upstreamError(c, err)
		return
	}

	slog.Info("record created", "name", input.SubDomain+"."+domain,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	created(c, gin.H{"record_id": recordID})
}

// batchCreateRecords creates records one by one and aggregates the results
func (s *Server) batchCreateRecords(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	var input batchCreateInput
	if !bindJSON(c, &input) {
		return
	}
	if !batchSizeOK(c, len(input.Items), "") {
		return
	}
	// Checked before the loop rather than reported per item: a batch that is
	// going to fail anyway should not spend one DNSPod call per item finding
	// that out, and a half-written batch is not what anyone asked for.
	for i := range input.Items {
		if err := prepareInput(&input.Items[i], false); err != nil {
			// The wording says a rule applied to one of the entries, not which
			// entry: the caller has the list in front of them.
			if errors.Is(err, errRecordFieldsRequired) {
				badRequest(c, err.Error()+" for every item")
				return
			}
			badRequest(c, err.Error())
			return
		}
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	var succeeded, failed []gin.H
	for _, item := range input.Items {
		recordID, err := cli.CreateRecord(domain, dnspod.CreateRecordInput{
			SubDomain:  item.SubDomain,
			RecordType: item.RecordType,
			Value:      item.Value,
			Line:       defaultLine(item.Line),
			TTL:        item.TTL,
			MX:         item.MX,
			Weight:     item.Weight,
			Status:     item.Status,
			Remark:     item.Remark,
		})
		if err != nil {
			failed = append(failed, gin.H{"sub_domain": item.SubDomain, "error": upstreamMessage(c, err)})
			continue
		}
		succeeded = append(succeeded, gin.H{"sub_domain": item.SubDomain, "record_id": recordID})
	}

	slog.Info("records created in a batch", "domain", domain,
		"succeeded", len(succeeded), "failed", len(failed),
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))

	ok(c, gin.H{"succeeded": succeeded, "failed": failed})
}

// updateRecord updates a record
func (s *Server) updateRecord(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	recordID := parseRecordID(c)
	if recordID == 0 {
		return
	}

	var input recordInput
	if !bindJSON(c, &input) {
		return
	}

	// Line required: four fields reach DNSPod unconditionally (see
	// dnspod.ModifyRecord), so an update that leaves the line out does not leave
	// the record alone - a blank one is filled in with the default and moves the
	// record off the line it lives on. Blank stays a value only a create may
	// read as "the default", where there is no record yet to move.
	if err := prepareInput(&input, true); err != nil {
		badRequest(c, err.Error())
		return
	}
	// Checked on the way in as well as on create: an update that leaves the
	// priority out would otherwise clear the one the record already carries.
	if priorityRequired[strings.ToUpper(input.RecordType)] && input.MX == 0 {
		badRequest(c, "mx is required for MX, HTTPS and SVCB records")
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	err = cli.ModifyRecord(c.Request.Context(), domain, recordID, dnspod.ModifyRecordInput{
		SubDomain:  input.SubDomain,
		RecordType: input.RecordType,
		Value:      input.Value,
		Line:       input.Line,
		TTL:        input.TTL,
		MX:         input.MX,
		Weight:     input.Weight,
		Status:     input.Status,
		Remark:     input.Remark,
	})
	if err != nil {
		upstreamError(c, err)
		return
	}

	slog.Info("record updated", "record", recordID,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, gin.H{"record_id": recordID})
}

// deleteRecord deletes a record
func (s *Server) deleteRecord(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	recordID := parseRecordID(c)
	if recordID == 0 {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}
	if err := cli.DeleteRecord(domain, recordID); err != nil {
		upstreamError(c, err)
		return
	}

	slog.Info("record deleted", "record", recordID,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, gin.H{"deleted": recordID})
}

// batchDeleteRecords deletes records in batch
func (s *Server) batchDeleteRecords(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	var input batchDeleteInput
	if !bindJSON(c, &input) {
		return
	}
	if !batchSizeOK(c, len(input.IDs), "") {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}

	var failed []gin.H
	deleted := 0
	for _, id := range input.IDs {
		if err := cli.DeleteRecord(domain, id); err != nil {
			failed = append(failed, gin.H{"record_id": id, "error": upstreamMessage(c, err)})
			continue
		}
		deleted++
	}

	slog.Info("records deleted in a batch", "domain", domain,
		"deleted", deleted, "failed", len(failed),
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))

	ok(c, gin.H{"deleted": deleted, "failed": failed})
}

// setRecordStatus enables or disables a record
func (s *Server) setRecordStatus(c *gin.Context) {
	domain := pathDomain(c)
	if domain == "" {
		return
	}
	recordID := parseRecordID(c)
	if recordID == 0 {
		return
	}

	var payload struct {
		Enable bool `json:"enable"`
	}
	if !bindJSON(c, &payload) {
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	cli, err := s.clientFor(accountID)
	if err != nil {
		credentialError(c, err)
		return
	}
	if err := cli.SetRecordStatus(domain, recordID, payload.Enable); err != nil {
		upstreamError(c, err)
		return
	}
	ok(c, gin.H{"record_id": recordID, "enabled": payload.Enable})
}

// defaultLine falls back to the default line when the value is empty
func defaultLine(line string) string {
	if line == "" {
		return "默认"
	}
	return line
}
