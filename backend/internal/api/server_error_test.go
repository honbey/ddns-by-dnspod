package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
)

// A caller walking away is logged below `info` on purpose - it is a closed tab,
// and nobody asked for one line per navigation - but "below info" must not mean
// "never": it is the one record that separates an honest abort from a fault, so
// an operator has to be able to turn it on. These cases are that evidence.
func serverErrorAt(t *testing.T, level slog.Level, err error) (*httptest.ResponseRecorder, []string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil))) })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/domains", nil)

	serverError(c, err)
	// gin flushes a deferred status at the end of ServeHTTP; there is no
	// ServeHTTP here, so the 499 has to be asked for explicitly.
	c.Writer.WriteHeaderNow()

	var lines []string
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return rec, lines
}

// A real failure is an error whatever the level: raising the floor is never
// allowed to hide one.
func TestServerErrorLogsAtEveryLevel(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		t.Run(level.String(), func(t *testing.T) {
			rec, lines := serverErrorAt(t, level, errors.New("the database is gone"))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if len(lines) != 1 {
				t.Fatalf("want one log record, got %d: %v", len(lines), lines)
			}
			if want := `"request failed"`; !strings.Contains(lines[0], want) {
				t.Fatalf("the record is missing %s: %s", want, lines[0])
			}
		})
	}
}

// The cancel branch answers 499 and writes no body, and its record reaches an
// operator exactly when one lowers the level for it.
func TestServerErrorCanceledIsVisibleWhenAskedFor(t *testing.T) {
	t.Run("at debug it is written", func(t *testing.T) {
		rec, lines := serverErrorAt(t, slog.LevelDebug, context.Canceled)

		if rec.Code != middleware.StatusNotResponded {
			t.Fatalf("status = %d, want 499", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("a canceled request answers with no body, got %q", rec.Body.String())
		}
		if len(lines) != 1 {
			t.Fatalf("want the cancel record, got %d lines: %v", len(lines), lines)
		}
		if want := `"request canceled by the caller"`; !strings.Contains(lines[0], want) {
			t.Fatalf("the record is missing %s: %s", want, lines[0])
		}
		if want := `"level":"DEBUG"`; !strings.Contains(lines[0], want) {
			t.Fatalf("the record is not a debug one: %s", lines[0])
		}
	})

	t.Run("at info it stays quiet", func(t *testing.T) {
		rec, lines := serverErrorAt(t, slog.LevelInfo, context.Canceled)

		if rec.Code != middleware.StatusNotResponded {
			t.Fatalf("status = %d, want 499", rec.Code)
		}
		if len(lines) != 0 {
			t.Fatalf("a closed tab must not write a line at info, got %v", lines)
		}
	})
}
