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

// Values are quoted, so a path containing %0A cannot end the line early.
func TestSetupLoggingWritesOneLinePerRecord(t *testing.T) {
	cases := []struct {
		format string
		marker string
	}{
		{"text", "addr="},
		{"json", `"addr":`},
		// An unknown format falls back to text.
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

// Only a 200 is healthy.
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
			// The handler runs on its own goroutine; assert after it returns.
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

func TestHealthcheckFailsWhenNothingAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/api/health"
	srv.Close()

	if got := healthcheck("", url); got != 1 {
		t.Fatalf("healthcheck() = %d; want 1", got)
	}
}
