package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		for _, path := range []string{"/api/ddns/tasks", "/api/history", "/api/stats/ip-changes"} {
			expectDocumented(t, r, router, http.MethodGet, path+"?page=1&page_size=20", "", http.StatusOK)
		}
	})

	t.Run("the public address reflects that this host reached nothing", func(t *testing.T) {
		expectDocumented(t, r, router, http.MethodGet, "/api/ip?family=v6", "", http.StatusOK)
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
	})

	t.Run("an id that cannot name a resource is refused", func(t *testing.T) {
		for _, path := range []string{"/api/accounts/0", "/api/keys/0", "/api/certificates/0"} {
			expectDocumented(t, r, router, http.MethodDelete, path, "", http.StatusBadRequest)
		}
	})

	t.Run("the API key path refuses a missing key", func(t *testing.T) {
		expectDocumented(t, r, router, http.MethodGet, "/api/v1/domains/example.com/records", "", http.StatusUnauthorized)
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

	if res.Code != want {
		t.Fatalf("%s %s -> %d, want %d (body: %s)", method, path, res.Code, want, res.Body.String())
	}

	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is missing from %s: %v", method, path, contractPath, err)
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
			method, path, res.Code, contractPath, err, res.Body.String())
	}
}
