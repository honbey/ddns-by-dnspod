package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
)

// batchCreatePayload builds a batch create body with n items
func batchCreatePayload(n int) map[string]any {
	items := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, map[string]any{
			"sub_domain":  "h" + itoa(uint(i)),
			"record_type": "A",
			"value":       "1.2.3.4",
		})
	}
	return map[string]any{"items": items}
}

// The batch calls issue one DNSPod request per item, so an unbounded list would
// tie up the connection and burn through the rate limit
func TestBatchCreateCapsTheItemCount(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost,
		"/api/domains/example.com/records/batch", batchCreatePayload(maxBatchItems+1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestBatchCreateAcceptsUpToTheLimit(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost,
		"/api/domains/example.com/records/batch", batchCreatePayload(maxBatchItems))
	// There is no account, so the call fails on the account guard before touching
	// DNSPod. Getting past the size guard (no "items per batch" message) is what we pin.
	if errorMessage(t, rec) == "at most 100 items per batch" {
		t.Fatal("an at-the-limit batch must not be rejected by the size guard")
	}
}

func TestBatchDeleteCapsTheItemCount(t *testing.T) {
	srv, cfg := newTestServer(t)

	ids := make([]uint64, 0, maxBatchItems+1)
	for i := 0; i <= maxBatchItems; i++ {
		ids = append(ids, uint64(i))
	}
	rec := do(t, srv, cfg, http.MethodPost,
		"/api/domains/example.com/records/batch-delete", map[string]any{"ids": ids})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// The cap and its sentence are one thing, shared by every batch path, so a
// caller who has met the limit on the records list recognises it on an import.
// The import's own form is pinned here rather than through the API: it is the
// one path whose size cannot be known on the client, and reaching it needs a
// live group listing.
func TestBatchSizeCapSharesOneSentence(t *testing.T) {
	cases := []struct {
		name string
		n    int
		hint string
		ok   bool
		want string
	}{
		{"at the cap", maxBatchItems, "", true, ""},
		{"over the cap", maxBatchItems + 1, "", false, "at most 100 items per batch"},
		{"over the cap with a remedy", maxBatchItems + 1, "narrow the group", false,
			"at most 100 items per batch, narrow the group"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			if got := batchSizeOK(c, tc.n, tc.hint); got != tc.ok {
				t.Fatalf("batchSizeOK(%d, %q) = %v, want %v", tc.n, tc.hint, got, tc.ok)
			}
			if tc.want != "" && !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("body %q does not carry %q", rec.Body.String(), tc.want)
			}
		})
	}
}

// An update forwards host, type, value and line to DNSPod as they are, so one
// left out used to go out as an empty string - and a blank line was filled in
// with the default one, moving a record off the line it lives on.
//
// Padding counts as left out too: host, type and line are trimmed before this
// check reads them, so what is refused here is what would have gone out as a
// host with spaces in it. Value is only ever tested for emptiness and is sent
// exactly as typed.
//
// The check runs before the account is even looked up, which is what lets these
// cases stand in for a real modify without any credentials.
func TestUpdateRecordRequiresEveryFieldItForwards(t *testing.T) {
	srv, cfg := newTestServer(t)

	const want = "sub_domain, record_type, value and line are required"
	cases := []struct {
		name string
		body map[string]any
	}{
		{"no sub_domain", map[string]any{"record_type": "A", "value": "1.2.3.4", "line": "default"}},
		{"no record_type", map[string]any{"sub_domain": "home", "value": "1.2.3.4", "line": "default"}},
		{"no value", map[string]any{"sub_domain": "home", "record_type": "A", "line": "default"}},
		{"no line", map[string]any{"sub_domain": "home", "record_type": "A", "value": "1.2.3.4"}},
		{"blank line", map[string]any{"sub_domain": "home", "record_type": "A", "value": "1.2.3.4", "line": "  "}},
		{"blank sub_domain", map[string]any{"sub_domain": "  ", "record_type": "A", "value": "1.2.3.4", "line": "default"}},
		{"blank record_type", map[string]any{"sub_domain": "home", "record_type": " ", "value": "1.2.3.4", "line": "default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, srv, cfg, http.MethodPut,
				"/api/domains/example.com/records/1", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
			if got := errorMessage(t, rec); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
		})
	}
}

// A create still reads a blank line as "the default", so it must not inherit
// the update rule. What it cannot do is file a record with no host - and a
// host that is all padding has no content either, so it is refused here
// instead of being trimmed down to an empty host after the check passed.
func TestCreateRecordRequiresAHost(t *testing.T) {
	srv, cfg := newTestServer(t)

	const want = "sub_domain, record_type and value are required"
	cases := []struct {
		name string
		body map[string]any
	}{
		{"no host", map[string]any{"record_type": "A", "value": "1.2.3.4", "line": "default"}},
		{"blank host", map[string]any{"sub_domain": "  ", "record_type": "A", "value": "1.2.3.4", "line": "default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
			if got := errorMessage(t, rec); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
		})
	}
}

// One item without a host refuses the whole batch before any DNSPod call: the
// alternative is a half-written batch reported item by item after the fact.
// All padding is no host at all, for the reason createRecord trims before it
// checks, so the second case pins that the batch path does the same.
func TestBatchCreateRefusesAnItemWithoutAHost(t *testing.T) {
	srv, cfg := newTestServer(t)

	const want = "sub_domain, record_type and value are required for every item"
	cases := []struct {
		name string
		item map[string]any
	}{
		{"missing host", map[string]any{"record_type": "A", "value": "1.2.3.4"}},
		{"blank host", map[string]any{"sub_domain": "  ", "record_type": "A", "value": "1.2.3.4"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records/batch",
				map[string]any{"items": []map[string]any{
					{"sub_domain": "home", "record_type": "A", "value": "1.2.3.4"},
					tc.item,
				}})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
			if got := errorMessage(t, rec); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
		})
	}
}

// Every check above also passed while a padded value was still on its way to
// DNSPod: trimming happens before them and changes nothing they look at. This
// is the one assertion that sees it, because the batch echoes back the host it
// dispatched - the very field the call is built from.
//
// The client is pointed at a port nothing listens on so the DNSPod call fails
// here instead of over the network. That failure is only a vehicle: what is
// under test is the string in "failed".
func TestBatchCreateDispatchesTheTrimmedHost(t *testing.T) {
	srv, cfg := newTestServer(t)
	cfg.DNSPod.Endpoint = "127.0.0.1:1"
	cfg.DNSPod.Timeout = 1
	newTestAccount(t, srv)

	rec := do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records/batch",
		map[string]any{"items": []map[string]any{
			{"sub_domain": " home ", "record_type": "A", "value": "1.2.3.4"},
		}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Failed []struct {
				SubDomain string `json:"sub_domain"`
			} `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if len(body.Data.Failed) != 1 {
		t.Fatalf("want exactly one failed item, got %d (%s)", len(body.Data.Failed), rec.Body.String())
	}
	if got := body.Data.Failed[0].SubDomain; got != "home" {
		t.Fatalf("want the dispatched host %q, got %q", "home", got)
	}
}

// A DNSPod call failure is the upstream's fault, not the caller's, so it cannot
// be a 400 - but a rate limit stays a 429 so the caller knows to back off, and a
// missing record is a genuine 404. The detail never reaches the response.
func TestUpstreamErrorSeparatesRateLimitsFromUpstreamFaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"rate limited", errors.New("[TencentCloudSDKError] Code=RequestLimitExceeded"), http.StatusTooManyRequests},
		{"record missing", errors.New("[TencentCloudSDKError] Code=InvalidParameter.RecordIdInvalid"), http.StatusNotFound},
		{"upstream fault", errors.New("[TencentCloudSDKError] Code=InternalError"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/domains/x/records", nil)
			upstreamError(c, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// A batch answers with a message per item instead of a status code, so the
// wording is all the caller gets. The SDK's own text describes the request it
// made and belongs in the log, which is what the request id points at.
func TestUpstreamMessageKeepsTheSDKTextOutOfTheResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		err  error
		want string
	}{
		{errors.New("[TencentCloudSDKError] Code=RequestLimitExceeded"), "DNSPod rate limit reached, retry later"},
		{errors.New("[TencentCloudSDKError] Code=InternalError, Message=backend exploded at /srv/main.go:42"), "DNSPod call failed"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/domains/x/records/batch", nil)
		if got := upstreamMessage(c, tc.err); got != tc.want {
			t.Fatalf("want %q, got %q", tc.want, got)
		}
	}
}

// With no account configured, the reason is a sentence written for the
// operator - "add one first" - and it was being swallowed by a message blaming
// DNSPod, a service that had not been called at all. The wording is the one
// resolveAccount's other call sites have always sent back.
func TestNoAccountIsExplainedRatherThanBlamedOnDNSPod(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodGet, "/api/domains/example.com/records", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	const want = "no Tencent Cloud account configured yet, add one first"
	if got := errorMessage(t, rec); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// A credential the server cannot read is our problem, not DNSPod's and not the
// caller's. The reason names the key material, so it stays in the log and the
// reply carries only the request id, exactly as a 500 does.
func TestAnUnreadableCredentialStaysOutOfTheResponse(t *testing.T) {
	srv, cfg := newTestServer(t)

	account := model.Account{
		Name:      "acc",
		SecretID:  "test-secret-id",
		SecretKey: "not a ciphertext",
		KeyHint:   "test",
	}
	if err := srv.store.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/domains/example.com/records", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := errorMessage(t, rec); got != "internal server error" {
		t.Fatalf("want %q, got %q", "internal server error", got)
	}
	if body := rec.Body.String(); strings.Contains(body, "decrypt") || strings.Contains(body, "secret") {
		t.Fatalf("the credential detail travelled with the response: %s", body)
	}
}

// The record write path is the one place a host used to reach the log and the
// history unvalidated: createRecord logs it and the scheduler stores it, while
// the task endpoints have refused these names all along.
func TestCreateRecordRejectsMalformedHostNames(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"newline", "a\n[api] record \"forged\""},
		{"script tag", "<script>alert(1)</script>"},
		{"quote", `a"onmouseover="alert(1)`},
		{"space", "a b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cfg := newTestServer(t)
			newTestAccount(t, srv)
			// Pointed away from the real API on purpose: if the check ever stops
			// rejecting these, the test must fail on the assertion below rather
			// than first spend a slow round trip on a live DNSPod call.
			cfg.DNSPod.Endpoint = "127.0.0.1:1"
			cfg.DNSPod.Timeout = 1

			body := map[string]any{
				"sub_domain":  tc.value,
				"record_type": "A",
				"value":       "1.2.3.4",
			}
			rec := do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("create: want 400, got %d (%s)", rec.Code, rec.Body.String())
			}

			// The batch path shares the rule, so a long paste cannot slip it in.
			rec = do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records/batch",
				map[string]any{"items": []map[string]any{
					{"sub_domain": "home", "record_type": "A", "value": "1.2.3.4"},
					body,
				}})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("batch: want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// An update forwards the host to DNSPod as it is, so it has to hold the same
// name create does - otherwise the two write paths disagree about what a host
// may contain and the create check is bypassable through this one.
func TestUpdateRecordRejectsMalformedHostNames(t *testing.T) {
	srv, cfg := newTestServer(t)
	newTestAccount(t, srv)
	cfg.DNSPod.Endpoint = "127.0.0.1:1"
	cfg.DNSPod.Timeout = 1

	for _, host := range []string{"a b", "a\nb", "<script>alert(1)</script>"} {
		rec := do(t, srv, cfg, http.MethodPut, "/api/domains/example.com/records/1",
			map[string]any{
				"sub_domain":  host,
				"record_type": "A",
				"value":       "1.2.3.4",
				"line":        "默认",
			})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("host %q: want 400, got %d (%s)", host, rec.Code, rec.Body.String())
		}
	}
}

// A body past the cap is a different mistake from one that will not parse, and
// only the first is fixed by sending less. One number for both sent the caller
// looking in the wrong place.
func TestAnOversizedBodyIsRefusedAsTooLarge(t *testing.T) {
	srv, cfg := newTestServer(t)

	huge := accountPayload("big", nil)
	huge["secret_key"] = strings.Repeat("x", middleware.DefaultMaxBody+1024)
	rec := do(t, srv, cfg, http.MethodPost, "/api/accounts", huge)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := errorMessage(t, rec); !strings.Contains(got, "larger than") {
		t.Fatalf("want the limit named in the message, got %q", got)
	}

	// The same endpoint inside the cap still works, so the limit is not simply
	// refusing everything.
	rec = do(t, srv, cfg, http.MethodPost, "/api/accounts", accountPayload("small", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// A body that will not parse is still a 400. The 413 mapping must not swallow
// the ordinary failure, which is what an errors.As written too broadly does.
func TestAMalformedBodyIsStillABadRequest(t *testing.T) {
	srv, cfg := newTestServer(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.MaxBody(middleware.DefaultMaxBody))
	srv.RegisterRoutes(r, *cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := errorMessage(t, rec); got != "invalid request body" {
		t.Fatalf("want %q, got %q", "invalid request body", got)
	}
}

// is "leave it alone" and is passed through as that.
func TestRecordStatusIsCheckedLocally(t *testing.T) {
	srv, cfg := newTestServer(t)

	rec := do(t, srv, cfg, http.MethodPost, "/api/domains/example.com/records", map[string]any{
		"sub_domain":  "a",
		"record_type": "A",
		"value":       "1.2.3.4",
		"status":      "enable",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("lower-case status: want 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ENABLE") {
		t.Fatalf("the refusal does not name the two values it accepts: %s", rec.Body.String())
	}
}
