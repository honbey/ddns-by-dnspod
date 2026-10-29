package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newPageContext(query string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/list?"+query, nil)
	return c
}

func TestParsePageParamsDefaults(t *testing.T) {
	p := parsePageParams(newPageContext(""))
	if p.Page != 1 || p.PageSize != defaultPageSize {
		t.Fatalf("want 1/%d, got %d/%d", defaultPageSize, p.Page, p.PageSize)
	}
}

func TestParsePageParamsHonoursValidValues(t *testing.T) {
	p := parsePageParams(newPageContext("page=3&page_size=25"))
	if p.Page != 3 || p.PageSize != 25 {
		t.Fatalf("want 3/25, got %d/%d", p.Page, p.PageSize)
	}
	if p.offset() != 50 {
		t.Fatalf("want offset 50, got %d", p.offset())
	}
}

func TestParsePageParamsFallsBackOnGarbage(t *testing.T) {
	cases := map[string]pageParams{
		"page=abc&page_size=xyz": {1, defaultPageSize},
		"page=0&page_size=0":     {1, defaultPageSize},
		"page=-2&page_size=-5":   {1, defaultPageSize},
	}
	for query, want := range cases {
		got := parsePageParams(newPageContext(query))
		if got != want {
			t.Errorf("query %q: want %+v, got %+v", query, want, got)
		}
	}
}

func TestParsePageParamsCapsPageSize(t *testing.T) {
	p := parsePageParams(newPageContext("page_size=100000"))
	if p.PageSize != maxPageSize {
		t.Fatalf("want page size capped at %d, got %d", maxPageSize, p.PageSize)
	}
}

func TestPageOffsetIsNeverNegative(t *testing.T) {
	if got := (pageParams{Page: 0, PageSize: 10}).offset(); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
	if got := (pageParams{Page: 1, PageSize: 10}).offset(); got != 0 {
		t.Fatalf("first page must start at offset 0, got %d", got)
	}
}

func TestPagesFromTotal(t *testing.T) {
	cases := []struct {
		total    int64
		pageSize int
		want     int
	}{
		{0, 10, 1},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{24, 10, 3},
		{20, 10, 2},
		{5, 0, 1},
	}
	for _, tc := range cases {
		if got := pagesFromTotal(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("pagesFromTotal(%d, %d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}

// decodeList reads one page out of a listing response.
//
// Every stored list answers with the same envelope, so the tests unwrap it in
// one place rather than each naming the page fields again.
func decodeList[T any](t *testing.T, body []byte) []T {
	t.Helper()
	var payload struct {
		Data struct {
			Items []T `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response: %v (%s)", err, body)
	}
	return payload.Data.Items
}
