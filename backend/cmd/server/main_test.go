package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// The level decides what a deployment hears, and nothing about it may be a
// guess: a caller walking away is logged at debug, so an operator has to be
// able to ask for debug to see it. An unrecognised name must not silently
// become something stricter.
func TestLogLevelFallsBackToInfo(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  slog.Level
		known bool
	}{
		{"debug lowers the floor", "debug", slog.LevelDebug, true},
		{"warn raises it", "warn", slog.LevelWarn, true},
		{"case and space do not matter", " DEBUG ", slog.LevelDebug, true},
		{"an unknown name falls back", "verbose", slog.LevelInfo, false},
		{"empty falls back", "", slog.LevelInfo, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := logLevel(tc.value)
			if got != tc.want || known != tc.known {
				t.Fatalf("logLevel(%q) = %v, %v; want %v, %v", tc.value, got, known, tc.want, tc.known)
			}
		})
	}
}

// One record is one line, whatever goes into it.
//
// The values are quoted by the handler, so nothing a caller controls can end
// the line early and start another one that reads like ours: a path carrying
// %0A arrives percent-decoded, and so does anything else that reaches a record
// without going through CurrentUser first. Hand-formatting every value at every
// call site is what failed quietly before, so the handler does it once here.
func TestSetupLoggingWritesOneLinePerRecord(t *testing.T) {
	cases := []struct {
		format string
		marker string
	}{
		{"text", "addr="},
		{"json", `"addr":`},
		// An unrecognised format is not a third shape to maintain: it falls
		// back to the one a terminal reads.
		{"", "addr="},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("failed to make a pipe: %v", err)
			}

			stderr := os.Stderr
			os.Stderr = w
			t.Cleanup(func() {
				os.Stderr = stderr
				slog.SetDefault(slog.New(slog.NewTextHandler(stderr, nil)))
			})

			setupLogging(tc.format, slog.LevelInfo)
			slog.Info("listening", "addr", "/api/domains/a\n[ddns] forged audit line")
			slog.Info("stopped")

			os.Stderr = stderr
			if err := w.Close(); err != nil {
				t.Fatalf("failed to close the pipe: %v", err)
			}

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, r); err != nil {
				t.Fatalf("failed to read the log: %v", err)
			}

			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("want one line per record, got %d lines: %q", len(lines), buf.String())
			}
			if !strings.Contains(lines[0], tc.marker) {
				t.Fatalf("%q is missing the fields: %q", tc.format, lines[0])
			}
		})
	}
}

// The probe has to find the server at whatever address it was told to listen
// on, including the wildcard form that cannot itself be dialed.
func TestProbeURLFollowsTheListenAddress(t *testing.T) {
	cases := []struct {
		name   string
		listen string
		want   string
	}{
		{"port only", ":8080", "http://127.0.0.1:8080/api/health"},
		{"wildcard v4", "0.0.0.0:9090", "http://127.0.0.1:9090/api/health"},
		{"wildcard v6", "[::]:8081", "http://127.0.0.1:8081/api/health"},
		{"an explicit loopback is kept", "127.0.0.1:7070", "http://127.0.0.1:7070/api/health"},
		{"empty falls back to the default port", "", "http://127.0.0.1:8080/api/health"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := probeURL(tc.listen); got != tc.want {
				t.Fatalf("probeURL(%q) = %q; want %q", tc.listen, got, tc.want)
			}
		})
	}
}

// Only a 200 is healthy: the exit code is the whole answer Docker reads, and a
// 503 that exited 0 would report a broken instance as fine.
func TestHealthcheckReportsTheHTTPStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   int
	}{
		{"a healthy server", http.StatusOK, 0},
		{"a server that cannot serve", http.StatusServiceUnavailable, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Recorded rather than asserted inside the handler: it runs on its
			// own goroutine, and t.Errorf after the test returned panics.
			var mu sync.Mutex
			var asked string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				asked = r.URL.Path
				mu.Unlock()
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			if got := healthcheck("", srv.URL+"/api/health"); got != tc.want {
				t.Fatalf("healthcheck() = %d; want %d", got, tc.want)
			}

			mu.Lock()
			path := asked
			mu.Unlock()
			if path != "/api/health" {
				t.Fatalf("probe asked for %q, want /api/health", path)
			}
		})
	}
}

// A server that is gone is the case this exists for: the probe must not hang
// on it and must not report success.
func TestHealthcheckFailsWhenNothingAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/api/health"
	srv.Close()

	if got := healthcheck("", url); got != 1 {
		t.Fatalf("healthcheck() = %d; want 1", got)
	}
}
