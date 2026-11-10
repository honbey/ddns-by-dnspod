// Package dnspod wraps the Tencent Cloud DNSPod API v20210323.
//
// It exposes value-type domain models and hides the SDK pointer fields, so callers need no nil checks.
package dnspod

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tcCommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sdk "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

const (
	// DefaultEndpoint is the DNSPod API endpoint
	DefaultEndpoint = "dnspod.tencentcloudapi.com"

	// maxDomainPages caps the pages one domain listing may read: 20 x 100, well
	// past any account this tool is meant for, and a stop for an API that keeps
	// answering with a full window.
	maxDomainPages = 20

	// StatusEnable marks a record as enabled
	StatusEnable = "ENABLE"
	// StatusDisable marks a record as paused
	StatusDisable = "DISABLE"

	// maxLimit is the maximum number of records per query (API limit is 3000)
	maxLimit = 3000
)

// Record is a DNS record
type Record struct {
	RecordID   uint64 `json:"record_id"`
	SubDomain  string `json:"sub_domain"`
	RecordType string `json:"record_type"`
	Value      string `json:"value"`
	Line       string `json:"line"`
	LineID     string `json:"line_id"`
	TTL        uint64 `json:"ttl"`
	MX         uint64 `json:"mx"`
	Weight     uint64 `json:"weight"`
	Status     string `json:"status"`
	Remark     string `json:"remark"`
	UpdatedOn  string `json:"updated_on"`
}

// Domain is an entry of the domain list
type Domain struct {
	DomainID uint64 `json:"domain_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	TTL      uint64 `json:"ttl"`
	Grade    string `json:"grade"`
	Records  uint64 `json:"records"`
	GroupID  uint64 `json:"group_id"`
}

// DomainGroup spans domains, unlike Group, which is a tag inside one domain.
// Size is how many domains DNSPod reports in the group.
type DomainGroup struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Size uint64 `json:"size"`
}

// Line describes a record line
type Line struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Group is a record group, i.e. a tag used to organise the records of one
// domain (for example a "DDNS" group holding the hosts kept up to date by this
// service). It is not the same as a domain group, which spans domains.
type Group struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// RecordFilter holds record query filters; zero values mean no filter
type RecordFilter struct {
	SubDomain  string
	RecordType string
	Line       string
	Keyword    string
	// GroupID restricts the result to one record group. The record list API
	// does not report which group a record belongs to, so filtering is only
	// possible by asking for a group explicitly.
	GroupID uint64
	Offset  uint64
	Limit   uint64
}

// RecordPage is one page of records together with the total number available
type RecordPage struct {
	Records []Record
	Total   uint64
}

// CreateRecordInput is the input for creating a record
type CreateRecordInput struct {
	SubDomain  string
	RecordType string
	Value      string
	Line       string
	TTL        uint64
	MX         uint64
	Weight     uint64
	Status     string
	Remark     string
}

// ModifyRecordInput is the input for updating a record.
//
// SubDomain, RecordType and Line are required and go out verbatim: DNSPod needs
// them on every modify and this package never fills in a value for them, so a
// caller that wants one left alone has to send the record's current one. The
// api layer refuses an update that omits any of them rather than letting an
// empty string, or the wrong line, reach the record.
//
// Value, TTL, Status, Remark, MX and Weight are the optional ones, and each is
// left out of the request while it is the zero value - which is what keeps a
// priority off a record that carries none.
type ModifyRecordInput struct {
	SubDomain  string
	RecordType string
	Value      string
	Line       string
	TTL        uint64
	MX         uint64
	Weight     uint64
	Status     string
	Remark     string
}

// Client is a DNSPod API client.
//
// Read calls are cached for a short while (see CacheTTL) to stay well below the
// DNSPod rate limit; every write call invalidates the affected entries.
type Client struct {
	cli *sdk.Client

	domains      *cache[[]Domain]
	domainGroups *cache[[]DomainGroup]
	records      *cache[*RecordPage]
	lines        *cache[[]Line]
}

// CacheTTL holds the lifetime of cached read results
type CacheTTL struct {
	Domains time.Duration
	Records time.Duration
	Lines   time.Duration
}

// DefaultCacheTTL is used when no TTL is supplied
var DefaultCacheTTL = CacheTTL{
	Domains: 5 * time.Minute,
	Records: 30 * time.Second,
	Lines:   10 * time.Minute,
}

// NewClient builds a client. region is meaningless for DNSPod (a global service), pass an empty string.
func NewClient(secretID, secretKey, endpoint string, timeoutSec int) (*Client, error) {
	return NewClientWithCache(secretID, secretKey, endpoint, timeoutSec, DefaultCacheTTL)
}

// NewClientWithCache builds a client with custom cache lifetimes.
// A zero or negative duration disables caching for that call.
func NewClientWithCache(secretID, secretKey, endpoint string, timeoutSec int, ttl CacheTTL) (*Client, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	// Note: the new SDK uses common.NewCredential; credential.NewSecretIdSecretKey has been removed
	cred := tcCommon.NewCredential(secretID, secretKey)

	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = endpoint
	cpf.HttpProfile.ReqTimeout = timeoutSec

	cli, err := sdk.NewClient(cred, "", cpf)
	if err != nil {
		return nil, fmt.Errorf("create DNSPod client: %w", err)
	}
	return &Client{
		cli:          cli,
		domains:      newCache[[]Domain](ttl.Domains),
		domainGroups: newCache[[]DomainGroup](ttl.Domains),
		records:      newCache[*RecordPage](ttl.Records),
		lines:        newCache[[]Line](ttl.Lines),
	}, nil
}

// Verify checks whether the credentials work
func (c *Client) Verify() error {
	req := sdk.NewDescribeDomainListRequest()
	if _, err := c.cli.DescribeDomainList(req); err != nil {
		return fmt.Errorf("credential check failed: %w", err)
	}
	return nil
}

// ListDomains lists every domain of the account.
//
// Cached like the record listing, and for the same reason: one entry serves
// every caller for the length of the TTL. The context is honoured between
// pages, which is where a listing of a large account can be abandoned.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	if cached, ok := c.domains.get("all"); ok {
		// A copy, as the record listing does: the entry is shared with every
		// other request, which silently corrupts the cache if a caller appends
		// to or writes into its slice.
		return append([]Domain(nil), cached...), nil
	}

	var (
		domains []Domain
		offset  int64
	)
	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list domains: %w", err)
		}
		if page >= maxDomainPages {
			slog.Warn("stopped reading domains at the page limit", "pages", maxDomainPages)
			break
		}
		req := sdk.NewDescribeDomainListRequest()
		req.Offset = tcCommon.Int64Ptr(offset)
		req.Limit = tcCommon.Int64Ptr(100)

		resp, err := c.cli.DescribeDomainList(req)
		if err != nil {
			return nil, fmt.Errorf("list domains: %w", err)
		}
		if resp.Response == nil || len(resp.Response.DomainList) == 0 {
			break
		}
		for _, d := range resp.Response.DomainList {
			domains = append(domains, Domain{
				DomainID: derefU64(d.DomainId),
				Name:     derefStr(d.Name),
				Status:   derefStr(d.Status),
				TTL:      derefU64(d.TTL),
				Grade:    derefStr(d.Grade),
				Records:  derefU64(d.RecordCount),
				GroupID:  derefU64(d.GroupId),
			})
		}
		offset += int64(len(resp.Response.DomainList))
		if len(resp.Response.DomainList) < 100 {
			break
		}
	}
	c.domains.set("all", domains)
	return domains, nil
}

// ListDomainGroups lists the account's domain groups.
//
// The SDK takes no paging parameters for this one, so a single call answers
// every group. Cached like the domain list: the frontend asks on every visit
// and DNSPod rate limits reads.
func (c *Client) ListDomainGroups(ctx context.Context) ([]DomainGroup, error) {
	if cached, ok := c.domainGroups.get("all"); ok {
		return append([]DomainGroup(nil), cached...), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list domain groups: %w", err)
	}

	resp, err := c.cli.DescribeDomainGroupList(sdk.NewDescribeDomainGroupListRequest())
	if err != nil {
		return nil, fmt.Errorf("list domain groups: %w", err)
	}

	groups := make([]DomainGroup, 0)
	if resp.Response != nil {
		for _, g := range resp.Response.GroupList {
			groups = append(groups, DomainGroup{
				ID:   derefGroupID(g.GroupId),
				Name: derefStr(g.GroupName),
				Size: derefGroupID(g.Size),
			})
		}
	}
	c.domainGroups.set("all", groups)
	return groups, nil
}

// ListRecords queries DNS records.
//
// Paging is applied locally on top of a cached full result: the API is always
// called with Offset=0 so that one entry serves every page of the same filter.
// The returned slice is a copy, callers may not mutate the cached data.
//
// The context is honoured between pages. The SDK call it guards cannot be
// interrupted, but a listing of a large domain is up to maxRecordPages of them,
// and a caller that gave up - a run past its timeout, a browser that went away -
// must not keep paying for the rest.
func (c *Client) ListRecords(ctx context.Context, domain string, f RecordFilter) (*RecordPage, error) {
	// Offset and Limit are excluded from the key on purpose. Including them made
	// each page of a listing a separate entry, and an entry holds up to maxLimit
	// records, so flipping through a few pages pinned a lot of memory.
	// Keyword stays in the key: it changes which records match, so results of
	// different searches must not be shared.
	// The group is part of the key: it decides which records match, so two
	// different groups must never share a cache entry.
	key := fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		domain, f.SubDomain, f.RecordType, f.Line, f.Keyword, f.GroupID)
	if cached, ok := c.records.get(key); ok {
		return pageSlice(cached, f.Offset, f.Limit), nil
	}

	// Read in full rather than as one call: the API caps a response at maxLimit
	// records, and keeping only that first window while reporting the API's own
	// Total made every later page empty - and left a host sorted past the window
	// invisible to a DDNS run, which then failed it with "no matching record
	// found" on every tick.
	// A ceiling on the calls one listing may make, so an API that keeps
	// answering with a full window cannot keep this loop going forever.
	const maxRecordPages = 20

	var (
		records []Record
		total   uint64
	)
	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list records (%s): %w", domain, err)
		}
		window, pageTotal, haveTotal, err := c.fetchRecords(domain, f, uint64(page)*maxLimit)
		if err != nil {
			return nil, err
		}
		records = append(records, window...)
		if pageTotal > total {
			total = pageTotal
		}
		// A short window is the end of the listing. Having read every record the
		// API says match is too, but only when the API did say: with no count of
		// its own the reported total is just the length of this window, and
		// trusting it stops after the first one - the truncation this loop
		// exists to remove.
		if uint64(len(window)) < maxLimit || (haveTotal && uint64(len(records)) >= total) {
			break
		}
		if page+1 >= maxRecordPages {
			slog.Warn("stopped reading records at the page limit", "domain", domain, "pages", maxRecordPages)
			break
		}
	}
	if total < uint64(len(records)) {
		total = uint64(len(records))
	}

	page := &RecordPage{Records: records, Total: total}
	c.records.set(key, page)
	return pageSlice(page, f.Offset, f.Limit), nil
}

// fetchRecords reads one window of the record list, starting at offset.
//
// The boolean reports whether the API gave a count of its own; without one the
// returned total is only the length of this window.
func (c *Client) fetchRecords(domain string, f RecordFilter, offset uint64) ([]Record, uint64, bool, error) {
	req := sdk.NewDescribeRecordListRequest()
	req.Domain = tcCommon.StringPtr(domain)

	// This API exposes both Subdomain and SubDomain fields,
	// and the docs say the backend prefers SubDomain when both are sent.
	if f.SubDomain != "" {
		req.SubDomain = tcCommon.StringPtr(f.SubDomain)
	}
	if f.RecordType != "" {
		req.RecordType = tcCommon.StringPtr(f.RecordType)
	}
	if f.Line != "" {
		req.RecordLine = tcCommon.StringPtr(f.Line)
	}
	if f.Keyword != "" {
		req.Keyword = tcCommon.StringPtr(f.Keyword)
	}
	if f.GroupID > 0 {
		req.GroupId = tcCommon.Uint64Ptr(f.GroupID)
	}

	req.Limit = tcCommon.Uint64Ptr(maxLimit)
	req.Offset = tcCommon.Uint64Ptr(offset)

	// ErrorOnEmpty defaults to yes and would error out when nothing matches;
	// setting it to no yields an empty list, which is easier to handle upstream.
	req.ErrorOnEmpty = tcCommon.StringPtr("no")

	resp, err := c.cli.DescribeRecordList(req)
	if err != nil {
		return nil, 0, false, fmt.Errorf("list records (domain=%s): %w", domain, err)
	}
	if resp.Response == nil {
		return nil, 0, false, nil
	}

	records := make([]Record, 0, len(resp.Response.RecordList))
	for _, r := range resp.Response.RecordList {
		records = append(records, Record{
			RecordID:   derefU64(r.RecordId),
			SubDomain:  derefStr(r.Name),
			RecordType: derefStr(r.Type),
			Value:      derefStr(r.Value),
			Line:       derefStr(r.Line),
			LineID:     derefStr(r.LineId),
			TTL:        derefU64(r.TTL),
			MX:         derefU64(r.MX),
			Weight:     derefU64(r.Weight),
			Status:     derefStr(r.Status),
			Remark:     derefStr(r.Remark),
			UpdatedOn:  derefStr(r.UpdatedOn),
		})
	}
	var (
		total     uint64
		haveTotal bool
	)
	if info := resp.Response.RecordCountInfo; info != nil && info.TotalCount != nil {
		total, haveTotal = *info.TotalCount, true
	}
	if total < uint64(len(records)) {
		total = uint64(len(records))
	}
	return records, total, haveTotal, nil
}

// pageSlice returns one window of a cached page.
//
// The records are copied so a caller cannot mutate what the cache holds.
// limit 0 means no limit. Total is carried over untouched, it reports how many
// records the filter matches in total, not how many this window contains.
func pageSlice(page *RecordPage, offset, limit uint64) *RecordPage {
	if page == nil {
		return &RecordPage{}
	}
	total := uint64(len(page.Records))
	if offset >= total {
		return &RecordPage{Records: []Record{}, Total: page.Total}
	}

	end := total
	if limit > 0 && limit < total-offset {
		end = offset + limit
	}

	window := page.Records[offset:end]
	records := make([]Record, len(window))
	copy(records, window)
	return &RecordPage{Records: records, Total: page.Total}
}

// FindDomain looks up a domain by exact name, to obtain its DomainId and grade.
//
// It walks the account's own domain list instead of running one keyword search:
// that search only ever returned its first hundred entries, so a domain past
// that point was never found - and ListRecordLines, which asks for it to fill in
// the grade, then went on without one. The list is cached and paged in full.
func (c *Client) FindDomain(ctx context.Context, domain string) (*Domain, error) {
	domains, err := c.ListDomains(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range domains {
		if d.Name != domain {
			continue
		}
		found := d
		return &found, nil
	}
	return nil, nil
}

// ListRecordLines lists the record lines a domain supports
func (c *Client) ListRecordLines(ctx context.Context, domain string) ([]Line, error) {
	if cached, ok := c.lines.get(domain); ok {
		return append([]Line(nil), cached...), nil
	}

	req := sdk.NewDescribeRecordLineListRequest()
	req.Domain = tcCommon.StringPtr(domain)

	// DomainGrade is required here; sending DomainId as well avoids same-name ambiguity
	if d, err := c.FindDomain(ctx, domain); err == nil && d != nil {
		if d.DomainID > 0 {
			req.DomainId = tcCommon.Uint64Ptr(d.DomainID)
		}
		if d.Grade != "" {
			req.DomainGrade = tcCommon.StringPtr(d.Grade)
		}
	}

	resp, err := c.cli.DescribeRecordLineList(req)
	if err != nil {
		return nil, fmt.Errorf("list record lines (domain=%s): %w", domain, err)
	}
	if resp.Response == nil {
		return nil, nil
	}

	lines := make([]Line, 0, len(resp.Response.LineList))
	for _, l := range resp.Response.LineList {
		lines = append(lines, Line{
			Name: derefStr(l.Name),
			// Note: the SDK field is named LineId, not Id
			ID: derefStr(l.LineId),
		})
	}
	c.lines.set(domain, lines)
	return lines, nil
}

// invalidateRecords drops the cached record lists of a domain after a write.
//
// The domain list goes as well: it carries the record count a domain shows, and
// keeping it meant that number stayed behind for the whole domains TTL - five
// minutes by default - after records were added or removed.
func (c *Client) invalidateRecords(domain string) {
	c.records.invalidate(domain + "|")
	c.domains.invalidate("")
}

// ListRecordGroups lists the record groups of a domain.
//
// These are tags inside a single domain, used by the DNSPod console to organise
// records by purpose. They are distinct from domain groups, which span domains.
func (c *Client) ListRecordGroups(domain string) ([]Group, error) {
	req := sdk.NewDescribeRecordGroupListRequest()
	req.Domain = tcCommon.StringPtr(domain)

	resp, err := c.cli.DescribeRecordGroupList(req)
	if err != nil {
		return nil, fmt.Errorf("list record groups (domain=%s): %w", domain, err)
	}
	if resp.Response == nil {
		return nil, nil
	}

	groups := make([]Group, 0, len(resp.Response.GroupList))
	for _, g := range resp.Response.GroupList {
		groups = append(groups, Group{
			ID:   derefU64(g.GroupId),
			Name: derefStr(g.GroupName),
			Type: derefStr(g.GroupType),
		})
	}
	return groups, nil
}

// CreateRecord creates a record and returns the new record ID
func (c *Client) CreateRecord(domain string, in CreateRecordInput) (uint64, error) {
	req := sdk.NewCreateRecordRequest()
	req.Domain = tcCommon.StringPtr(domain)
	req.SubDomain = tcCommon.StringPtr(in.SubDomain)
	req.RecordType = tcCommon.StringPtr(in.RecordType)
	req.Value = tcCommon.StringPtr(in.Value)
	req.RecordLine = tcCommon.StringPtr(in.Line)

	if in.TTL > 0 {
		req.TTL = tcCommon.Uint64Ptr(in.TTL)
	}
	if in.Status != "" {
		req.Status = tcCommon.StringPtr(in.Status)
	}
	if in.Remark != "" {
		req.Remark = tcCommon.StringPtr(in.Remark)
	}
	// Priority is mandatory for MX/HTTPS/SVCB, so it is sent whenever one was
	// given. It is not sent as a zero: that overwrote a record's existing
	// priority with one the API does not accept, which is worse than leaving the
	// field out and hearing about it.
	if in.MX > 0 {
		req.MX = tcCommon.Uint64Ptr(in.MX)
	}
	// Weight splits the answer between several records of the same host and
	// type; zero means every one of them carries the same share.
	if in.Weight > 0 {
		req.Weight = tcCommon.Uint64Ptr(in.Weight)
	}

	resp, err := c.cli.CreateRecord(req)
	if err != nil {
		return 0, fmt.Errorf("create record (%s.%s): %w", in.SubDomain, domain, err)
	}
	if resp.Response == nil || resp.Response.RecordId == nil {
		return 0, fmt.Errorf("create record: response carries no RecordId")
	}
	c.invalidateRecords(domain)
	return derefU64(resp.Response.RecordId), nil
}

// ModifyRecord updates a record
func (c *Client) ModifyRecord(ctx context.Context, domain string, recordID uint64, in ModifyRecordInput) error {
	req := sdk.NewModifyRecordRequest()
	req.Domain = tcCommon.StringPtr(domain)
	req.RecordId = tcCommon.Uint64Ptr(recordID)
	req.SubDomain = tcCommon.StringPtr(in.SubDomain)
	req.RecordType = tcCommon.StringPtr(in.RecordType)
	req.RecordLine = tcCommon.StringPtr(in.Line)

	if in.Value != "" {
		req.Value = tcCommon.StringPtr(in.Value)
	}
	if in.TTL > 0 {
		req.TTL = tcCommon.Uint64Ptr(in.TTL)
	}
	if in.Status != "" {
		req.Status = tcCommon.StringPtr(in.Status)
	}
	if in.Remark != "" {
		req.Remark = tcCommon.StringPtr(in.Remark)
	}
	// Same reasoning as on create: a priority of zero is not a priority, and
	// sending it resets the one the record already has.
	if in.MX > 0 {
		req.MX = tcCommon.Uint64Ptr(in.MX)
	}
	if in.Weight > 0 {
		req.Weight = tcCommon.Uint64Ptr(in.Weight)
	}

	// Checked before the call rather than during it: the SDK request cannot be
	// interrupted, so the only cancellation this can honour is "do not start".
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("modify record (recordID=%d): %w", recordID, err)
	}
	if _, err := c.cli.ModifyRecord(req); err != nil {
		return fmt.Errorf("modify record (recordID=%d): %w", recordID, err)
	}
	c.invalidateRecords(domain)
	return nil
}

// DeleteRecord deletes a record
func (c *Client) DeleteRecord(domain string, recordID uint64) error {
	req := sdk.NewDeleteRecordRequest()
	req.Domain = tcCommon.StringPtr(domain)
	req.RecordId = tcCommon.Uint64Ptr(recordID)

	if _, err := c.cli.DeleteRecord(req); err != nil {
		return fmt.Errorf("delete record (recordID=%d): %w", recordID, err)
	}
	c.invalidateRecords(domain)
	return nil
}

// SetRecordStatus enables or pauses a record
func (c *Client) SetRecordStatus(domain string, recordID uint64, enable bool) error {
	req := sdk.NewModifyRecordStatusRequest()
	req.Domain = tcCommon.StringPtr(domain)
	req.RecordId = tcCommon.Uint64Ptr(recordID)

	status := StatusDisable
	if enable {
		status = StatusEnable
	}
	req.Status = tcCommon.StringPtr(status)

	if _, err := c.cli.ModifyRecordStatus(req); err != nil {
		return fmt.Errorf("set record status (recordID=%d): %w", recordID, err)
	}
	c.invalidateRecords(domain)
	return nil
}

// IsRateLimited reports whether an error comes from the DNSPod rate limiter.
//
// The Chinese markers are not leftovers: the upstream answers in Chinese
// (X-TC-Language defaults to zh-CN in the SDK), so they have to stay in that
// language for the match to keep working.
// IsRateLimited reports whether the provider refused the call for sending too
// many requests.
//
// The typed error carries the provider's own verdict: RequestLimitExceeded is
// the code the API returns for a rate limit, and it is read from the error's
// code field rather than from the digits 429, which a domain or a record value
// can carry without any limit having been hit.
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	var sdkErr *sdkerrors.TencentCloudSDKError
	if errors.As(err, &sdkErr) {
		return strings.Contains(sdkErr.GetCode(), "RequestLimitExceeded")
	}
	for _, marker := range []string{"RequestLimitExceeded", "请求频率", "限频"} {
		if strings.Contains(err.Error(), marker) {
			return true
		}
	}
	return false
}

// IsRecordNotFound reports whether an error means the record is not there.
//
// Verified against the live API: deleting a record id that does not exist comes
// back as Code=InvalidParameter.RecordIdInvalid / "记录编号错误". The other
// codes are kept in the list because different actions phrase it differently.
// As in IsRateLimited the Chinese entries match the upstream wording and must
// not be translated.
func IsRecordNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{
		"RecordIdInvalid",
		"NoDataOfRecord",
		"RecordIdNotExists",
		"RecordNotExists",
		"记录不存在",
		"记录编号错误",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefU64(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// derefGroupID narrows the SDK's signed group fields to the unsigned ids the
// API surface uses. A missing or negative value means "no group".
func derefGroupID(p *int64) uint64 {
	if p == nil || *p < 0 {
		return 0
	}
	return uint64(*p)
}
