package notifier

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Templates reach ntfy as a Title header and the plain-text body, so the two
// paths worth pinning are: the variables actually render, and a title cannot
// smuggle a second header in through a line break.
func TestSendNtfyTemplates(t *testing.T) {
	var got http.Header
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// The loopback allow list is what lets the test server be reached at all.
	s := New(nil, Config{AllowPrivate: []string{"127.0.0.1/32", "::1/128"}})
	msg := Message{
		Title:     "built-in",
		Content:   "built-in body",
		Status:    "OK",
		Domain:    "example.com",
		SubDomain: "home",
		NewIP:     "1.2.3.4",
	}

	cfg := NtfyConfig{
		Server:        srv.URL,
		Topic:         "ddns",
		TitleTemplate: "{{.SubDomain}}.{{.Domain}} {{.Status}}",
		BodyTemplate:  "{{.SubDomain}} is now {{.NewIP}}",
	}
	if err := s.SendNtfy(cfg, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Get("Title") != "home.example.com OK" {
		t.Fatalf("title = %q", got.Get("Title"))
	}
	if body != "home is now 1.2.3.4" {
		t.Fatalf("body = %q", body)
	}

	// An empty template keeps the built-in wording instead of rendering nothing.
	builtin := NtfyConfig{Server: srv.URL, Topic: "ddns"}
	if err := s.SendNtfy(builtin, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Get("Title") != "built-in" || body != "built-in body" {
		t.Fatalf("empty template should keep defaults, title = %q body = %q", got.Get("Title"), body)
	}

	cfg.TitleTemplate = "{{.Domain}}\r\nX-Injected: yes"
	if err := s.SendNtfy(cfg, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Get("X-Injected") != "" {
		t.Fatalf("title template smuggled a header: %v", got)
	}
	if strings.ContainsAny(got.Get("Title"), "\r\n") {
		t.Fatalf("title kept a line break: %q", got.Get("Title"))
	}

	// Custom headers go out, but the reserved ones stay under the channel's own
	// fields, and a line break in a value cannot split the request.
	cfg.Priority = 3
	cfg.Headers = map[string]string{
		"X-Trace":  "abc",
		"Priority": "5\r\nX-Smuggled: 1",
		"priority": "5",
	}
	if err := s.SendNtfy(cfg, msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Get("X-Trace") != "abc" {
		t.Fatalf("custom header missing: %v", got)
	}
	if got.Get("X-Smuggled") != "" {
		t.Fatalf("value kept a line break: %v", got)
	}
	if got.Get("Priority") != "3" {
		t.Fatalf("reserved header was overridden: %q", got.Get("Priority"))
	}
}
