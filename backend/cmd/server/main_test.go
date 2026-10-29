package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
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
