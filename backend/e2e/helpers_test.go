//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type api struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func (a *api) do(method, path string, headers map[string]string) (int, []byte) {
	a.t.Helper()

	req, err := http.NewRequest(method, a.base+path, nil)
	if err != nil {
		a.t.Fatalf("build %s %s: %v", method, path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, raw
}

func envWithoutDDNS() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "DDNS_") && !strings.HasPrefix(entry, "AUTH_DISABLED") &&
			!strings.HasPrefix(entry, "OIDC_") && !strings.HasPrefix(entry, "APP_NAME") &&
			!strings.HasPrefix(entry, "API_BASE_URL") {
			env = append(env, entry)
		}
	}
	return env
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// waitHealthy gives up early when the log shows a bind failure.
func waitHealthy(base, logPath string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var last error

	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(logPath); err == nil &&
			strings.Contains(string(data), "address already in use") {
			return fmt.Errorf("port already in use")
		}

		resp, err := client.Get(base + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("health check: %w", last)
}

// serverOptions is the part of the configuration a test varies.
type serverOptions struct {
	// With auth off the server impersonates disabled_sub.
	auth bool
}

type server struct {
	base string
	bin  string
	cfg  string
}

// startServer builds and starts the real binary, retrying a taken port.
func startServer(t *testing.T, opts serverOptions) *server {
	t.Helper()

	if _, err := os.Stat("../internal/frontend/dist/index.html"); err != nil {
		t.Skip("frontend not built: run npm run build in frontend first")
	}

	work := t.TempDir()
	bin := filepath.Join(work, "ddns-server")

	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(work, "config.yml")
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		addr := freeAddr(t)
		cfg := fmt.Sprintf(`server:
  listen: %s
database:
  path: %s
crypto:
  key_file: %s
scheduler:
  enabled: false
acme:
  enabled: false
api:
  enabled: false
oidc:
  disabled: %t
  issuer: %q
  client_id: %q
  timeout_ms: 1000
`, addr,
			filepath.Join(work, "ddns.db"),
			filepath.Join(work, "secret.key"),
			!opts.auth,
			map[bool]string{true: "https://idp.invalid", false: ""}[opts.auth],
			map[bool]string{true: "e2e", false: ""}[opts.auth],
		)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		logPath := filepath.Join(work, fmt.Sprintf("server-%d.log", attempt))
		logFile, err := os.Create(logPath)
		if err != nil {
			t.Fatalf("create log: %v", err)
		}

		cmd := exec.Command(bin, "-c", cfgPath)
		cmd.Env = envWithoutDDNS()
		cmd.Stdout, cmd.Stderr = logFile, logFile
		if err := cmd.Start(); err != nil {
			logFile.Close()
			lastErr = err
			continue
		}

		base := "http://" + addr
		if err := waitHealthy(base, logPath, 30*time.Second); err != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			logFile.Close()
			log, _ := os.ReadFile(logPath)
			lastErr = fmt.Errorf("%v\n%s", err, log)
			if strings.Contains(string(log), "address already in use") {
				continue
			}
			t.Fatalf("server did not become healthy: %v", lastErr)
		}

		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			logFile.Close()
			if t.Failed() {
				if data, err := os.ReadFile(logPath); err == nil {
					t.Logf("server log:\n%s", data)
				}
			}
		})

		return &server{base: base, bin: bin, cfg: cfgPath}
	}

	t.Fatalf("server failed to start after 3 attempts: %v", lastErr)
	return nil
}

// decodeEnvelope reads the {"data": ...} wrapper every success response uses.
func decodeEnvelope(t *testing.T, raw []byte, into any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, raw)
	}
	if err := json.Unmarshal(envelope.Data, into); err != nil {
		t.Fatalf("decode data: %v (%s)", err, raw)
	}
}

func contains(raw []byte, needle string) bool { return bytes.Contains(raw, []byte(needle)) }
