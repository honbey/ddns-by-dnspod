package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/config"
)

// Where the document sits, relative to this package. openapi.yml is the only
// source of the payload shapes the SPA is generated from, and this file is what
// keeps it true: a route either side invents on its own fails here.
const contractPath = "../../openapi.yml"

func loadContract(t *testing.T) (*openapi3.T, routers.Router) {
	t.Helper()

	doc, err := openapi3.NewLoader().LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load %s: %v", contractPath, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("%s is not a valid document: %v", contractPath, err)
	}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	return doc, router
}

// openAPIPath turns gin's `:id` placeholder into the specification's `{id}`.
// It stops at `*` rather than translating it: a catch-all is not a path the
// document can describe.
func openAPIPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return strings.Join(segments, "/")
}

func inContract(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/config.json"
}

// TestContractMatchesRoutes checks both directions: an undocumented route has
// no type, and a documented path nobody serves is a type for nothing.
func TestContractMatchesRoutes(t *testing.T) {
	doc, _ := loadContract(t)
	srv, cfg := newTestServer(t)

	r := gin.New()
	r.Use(middlewareRequestIDForContract())
	srv.RegisterRoutes(r, *cfg)

	t.Run("every served route is documented", func(t *testing.T) {
		for _, route := range r.Routes() {
			if !inContract(route.Path) || strings.Contains(route.Path, "*") {
				continue
			}
			item := doc.Paths.Find(openAPIPath(route.Path))
			if item == nil {
				t.Errorf("%s %s has no path in %s", route.Method, route.Path, contractPath)
				continue
			}
			if item.GetOperation(route.Method) == nil {
				t.Errorf("%s %s has no operation in %s", route.Method, route.Path, contractPath)
			}
		}
	})

	t.Run("every documented operation is served", func(t *testing.T) {
		served := make(map[string]bool, len(r.Routes()))
		for _, route := range r.Routes() {
			served[route.Method+" "+openAPIPath(route.Path)] = true
		}
		for _, path := range doc.Paths.Keys() {
			for method := range doc.Paths.Value(path).Operations() {
				if !served[strings.ToUpper(method)+" "+path] {
					t.Errorf("%s %s is documented in %s but no route serves it", strings.ToUpper(method), path, contractPath)
				}
			}
		}
	})
}

func middlewareRequestIDForContract() gin.HandlerFunc {
	return requestIDForContract
}

func requestIDForContract(c *gin.Context) {
	c.Header("X-Request-Id", "contract")
	c.Next()
}

// TestContractResponses validates the answers of everything that can be served
// without a DNSPod credential: the public documents and the refusals. A payload
// that only ever succeeds is one whose error shape nobody knows.
func TestContractResponses(t *testing.T) {
	_, router := loadContract(t)
	srv, cfg := newTestServer(t)

	r := gin.New()
	r.Use(requestIDForContract)
	srv.RegisterRoutes(r, *cfg)

	t.Run("the public documents", func(t *testing.T) {
		for _, path := range []string{"/api/health", "/api/ready", "/config.json"} {
			expectDocumented(t, r, router, http.MethodGet, path, "", http.StatusOK)
		}
	})

	// The success envelope is what this refactor bought: the bundle's types are
	// generated from this document, so a field that drifts here is a wrong
	// type in the UI. These three listings need nothing but an empty database,
	// which makes them the cheapest place to check every field of every item.
	t.Run("a listing answers inside the envelope", func(t *testing.T) {
		for _, path := range []string{"/api/accounts", "/api/keys", "/api/certificates"} {
			expectDocumented(t, r, router, http.MethodGet, path, "", http.StatusOK)
		}
	})

	t.Run("a paged listing carries its page fields", func(t *testing.T) {
		for _, path := range []string{"/api/ddns/tasks", "/api/history", "/api/history/ip-changes", "/api/stats/ip-changes"} {
			expectDocumented(t, r, router, http.MethodGet, path+"?page=1&page_size=20", "", http.StatusOK)
		}
	})

	t.Run("the public address reflects that this host reached nothing", func(t *testing.T) {
		expectDocumented(t, r, router, http.MethodGet, "/api/ip?family=v6", "", http.StatusOK)
	})

	t.Run("a CSV import answers inside the envelope", func(t *testing.T) {
		stamp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		expectDocumentedMultipart(t, r, router, "/api/history/ip-changes/import",
			"time,ip\n"+stamp+",203.0.113.1\n", http.StatusOK)
	})

	// An empty errors array validates nothing about the error shape, so the
	// enum on IPHistoryImportError.code would otherwise never meet the gate.
	t.Run("a CSV import reports its bad rows inside the contract", func(t *testing.T) {
		stamp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		expectDocumentedMultipart(t, r, router, "/api/history/ip-changes/import",
			"time,ip\n"+stamp+",203.0.113.2\nnot-a-time,1.1.1.1\n", http.StatusOK)
	})

	// Both endpoints refuse a family outside the enum, and both answers are in
	// the document: a 400 nobody declared is a shape the SPA cannot know.
	t.Run("a family the contract does not allow is refused", func(t *testing.T) {
		for _, path := range []string{
			"/api/history/ip-changes?family=v9",
			"/api/stats/ip-changes?family=v9",
		} {
			expectDocumented(t, r, router, http.MethodGet, path, "", http.StatusBadRequest)
		}
	})

	t.Run("a request without a token is refused", func(t *testing.T) {
		// Verification is what the default fixture turns off; a broken audit
		// trail is worth less than one more test server.
		srv, cfg := newTestServer(t, func(c *config.Config) {
			c.OIDC.Disabled = false
			c.OIDC.Issuer = "https://idp.invalid"
			c.OIDC.ClientID = "ddns-by-dnspod"
		})
		r := gin.New()
		srv.RegisterRoutes(r, *cfg)

		for _, path := range []string{"/api/accounts", "/api/domains", "/api/keys"} {
			expectDocumented(t, r, router, http.MethodGet, path, "", http.StatusUnauthorized)
		}
		// The import rewrites a timeline, so the refusal has to reach it too
		// rather than only the listings.
		expectDocumented(t, r, router, http.MethodPost, "/api/history/ip-changes/import", "", http.StatusUnauthorized)
	})

	t.Run("an id that cannot name a resource is refused", func(t *testing.T) {
		for _, path := range []string{"/api/accounts/0", "/api/keys/0", "/api/certificates/0"} {
			expectDocumented(t, r, router, http.MethodDelete, path, "", http.StatusBadRequest)
		}
	})

	t.Run("the API key path refuses a missing key", func(t *testing.T) {
		expectDocumented(t, r, router, http.MethodGet, "/api/v1/domains/example.com/records", "", http.StatusUnauthorized)
	})

	// The batch result and the 409 it shares with single-task creation are new
	// shapes in the document. Neither needs a DNSPod credential, so they can be
	// checked here with nothing but a local account.
	t.Run("task creation answers inside the contract", func(t *testing.T) {
		account := newTestAccount(t, srv)
		id := itoa(account.ID)
		batch := `{"account_id":` + id + `,"domain":"example.com","sub_domains":["www","api"],"cron_expr":"*/5 * * * *"}`
		expectDocumented(t, r, router, http.MethodPost, "/api/ddns/tasks/batch", batch, http.StatusOK)

		single := `{"account_id":` + id + `,"domain":"example.com","sub_domain":"www","cron_expr":"*/10 * * * *"}`
		expectDocumented(t, r, router, http.MethodPost, "/api/ddns/tasks", single, http.StatusConflict)
	})
}

// expectDocumented drives one request and validates the answer against the
// document: the status code (an undocumented one fails), the content type and
// every field of the body.
func expectDocumented(
	t *testing.T,
	r *gin.Engine,
	router routers.Router,
	method, path, body string,
	want int,
) {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	validateDocumented(t, req, res, router, want)
}

// expectDocumentedMultipart drives one upload through the same validation.
// The import is the only route that answers from a multipart request, and its
// result shape is part of the contract like every other envelope.
func expectDocumentedMultipart(
	t *testing.T,
	r *gin.Engine,
	router routers.Router,
	path, content string,
	want int,
) {
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

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)

	validateDocumented(t, req, res, router, want)
}

// validateDocumented checks one answer against the document: the status code,
// the content type and every field of the body.
func validateDocumented(
	t *testing.T,
	req *http.Request,
	res *httptest.ResponseRecorder,
	router routers.Router,
	want int,
) {
	t.Helper()

	if res.Code != want {
		t.Fatalf("%s %s -> %d, want %d (body: %s)", req.Method, req.URL.Path, res.Code, want, res.Body.String())
	}

	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is missing from %s: %v", req.Method, req.URL.Path, contractPath, err)
	}

	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status:  res.Code,
		Header:  res.Header(),
		Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), input.SetBodyBytes(res.Body.Bytes())); err != nil {
		t.Fatalf("%s %s -> %d does not match %s: %v\nbody: %s",
			req.Method, req.URL.Path, res.Code, contractPath, err, res.Body.String())
	}
}
