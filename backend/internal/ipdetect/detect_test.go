package ipdetect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestExtractJSONPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		path []string
		want string
	}{
		{
			name: "top-level ip with default path",
			raw:  `{"ip":"1.2.3.4","city":"X"}`,
			path: nil,
			want: "1.2.3.4",
		},
		{
			name: "explicit top-level path",
			raw:  `{"ip":"1.2.3.4"}`,
			path: []string{"ip"},
			want: "1.2.3.4",
		},
		{
			name: "nested path",
			raw:  `{"data":{"ip":"1.2.3.4"}}`,
			path: []string{"data", "ip"},
			want: "1.2.3.4",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := extractJSONPath(c.raw, c.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestExtractJSONPathErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		path []string
	}{
		{name: "invalid json", raw: "not-json", path: []string{"ip"}},
		{name: "missing key", raw: `{"other":"x"}`, path: []string{"ip"}},
		{name: "wrong type", raw: `{"ip":123}`, path: []string{"ip"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := extractJSONPath(c.raw, c.path); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

// Sources are tried one at a time, so a healthy first source costs exactly one
// request and the rest of the pool is never asked.
func TestDetectStopsAtFirstWorkingSource(t *testing.T) {
	var secondHits int32

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "1.2.3.4")
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&secondHits, 1)
		fmt.Fprint(w, "5.5.5.5")
	}))
	defer second.Close()

	d := New([]string{first.URL, second.URL}, nil, 5)
	got, err := d.Detect(context.Background(), V4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.2.3.4" {
		t.Fatalf("got %q, want the answer of the first source", got)
	}
	if n := atomic.LoadInt32(&secondHits); n != 0 {
		t.Fatalf("second source was called %d time(s), want 0", n)
	}
}

// A source that cannot be reached must not end the run: the next one is tried.
func TestDetectFallsThroughToNextSource(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "1.2.3.4")
	}))
	defer good.Close()

	// Port 1 is refused straight away, so this exercises the fallback path
	// without waiting for a timeout.
	d := New([]string{"http://127.0.0.1:1", good.URL}, nil, 5)
	got, err := d.Detect(context.Background(), V4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.2.3.4" {
		t.Fatalf("got %q, want the answer of the surviving source", got)
	}
}

func TestDetectReportsWhenEverySourceFails(t *testing.T) {
	d := New([]string{"http://127.0.0.1:1"}, nil, 5)
	if _, err := d.Detect(context.Background(), V4); err == nil {
		t.Fatal("want an error when no source answers")
	}
}

func TestValidateFamily(t *testing.T) {
	d := &Detector{}
	cases := []struct {
		name   string
		raw    string
		family Family
		want   string
	}{
		{name: "v4 ip for v4", raw: "1.2.3.4", family: V4, want: "1.2.3.4"},
		{name: "v6 ip for v6", raw: "2001:db8::1", family: V6, want: "2001:db8::1"},
		{name: "whitespace trimmed", raw: "  1.2.3.4\n", family: V4, want: "1.2.3.4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := d.validate(c.raw, c.family, "test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	for _, c := range []struct {
		name   string
		raw    string
		family Family
	}{
		{name: "v6 ip in v4", raw: "2001:db8::1", family: V4},
		{name: "v4 ip in v6", raw: "1.2.3.4", family: V6},
		{name: "not an ip", raw: "hello", family: V4},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := d.validate(c.raw, c.family, "test"); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}
