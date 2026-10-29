// Command server is the entry point of the DDNS by DNSPod server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/acme"
	"ddns-by-dnspod/internal/api"
	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/auth"
	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ddns"
	"ddns-by-dnspod/internal/frontend"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// toIPEndpoints converts config endpoint structs into the detector's type.
// The two packages are kept independent on purpose, so the conversion happens here.
func toIPEndpoints(in []config.IPEndpoint) []ipdetect.Endpoint {
	out := make([]ipdetect.Endpoint, 0, len(in))
	for _, e := range in {
		out = append(out, ipdetect.Endpoint{URL: e.URL, Path: e.Path})
	}
	return out
}

// logLevel turns the configured name into a level, falling back rather than
// refusing to start: a typo in this value should not take a working service
// down, it should just be loud about being ignored.
func logLevel(name string) (slog.Level, bool) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(name)))); err != nil {
		return slog.LevelInfo, false
	}
	return level, true
}

// setupLogging installs the process-wide handler. Both formats go to stderr,
// one record per line: a container collects that, a terminal reads it.
func setupLogging(format string, level slog.Level) {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, opts)))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
}

// idpOrigins returns the origins the content policy has to let the page reach:
// the provider the browser signs in with, renews against and revokes against.
// None when there is no provider, which leaves the policy at self-only.
func idpOrigins(cfg *config.Config) []string {
	if cfg.OIDC.Disabled || cfg.OIDC.Issuer == "" {
		return nil
	}
	u, err := url.Parse(cfg.OIDC.Issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	return []string{u.Scheme + "://" + u.Host}
}

// fatal logs a startup failure as a record instead of a bare print, so the
// reason reaches the same collector as everything else. It exits rather than
// returning because every caller has already torn down what it owns.
func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func main() {
	var configPath string
	var backupPath string
	var showVersion bool
	flag.StringVar(&configPath, "c", "", "path to the config file (built-in defaults plus env vars are used when empty)")
	flag.StringVar(&backupPath, "backup", "", "write a consistent snapshot of the database at database.path to this path and exit (needs the same -c config as the server)")
	flag.BoolVar(&showVersion, "version", false, "print the version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println("ddns-server", config.Version)
		return
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config", err)
	}
	level, knownLevel := logLevel(cfg.Server.LogLevel)
	setupLogging(cfg.Server.LogFormat, level)

	// What this process actually runs with, minus anything secret: the first
	// thing to read when something looks wrong.
	slog.Info("configuration loaded",
		"listen", cfg.Server.Listen,
		"mode", cfg.Server.Mode,
		"log_format", cfg.Server.LogFormat,
		"log_level", level.String(),
		"hsts", cfg.Server.HSTS,
		"metrics", cfg.Metrics.Enabled,
		"trusted_proxies", cfg.Server.TrustedProxies,
		"auth_disabled", cfg.OIDC.Disabled,
	)
	if !knownLevel {
		slog.Warn("unknown server.log_level, falling back to info", "value", cfg.Server.LogLevel)
	}
	// Surface likely misconfigurations before they turn into silent 403s
	for _, w := range cfg.Validate() {
		slog.Warn("configuration warning", "detail", w)
	}

	// Backup mode: snapshot and leave. sqlite runs WAL, so copying the file of a
	// live database loses whatever has not been checkpointed - VACUUM INTO is
	// what makes the copy consistent.
	if backupPath != "" {
		db, err := store.OpenForBackup(cfg.Database.Path)
		if err != nil {
			fatal("failed to open the database for backup", err)
		}
		if err := store.Backup(context.Background(), db, backupPath); err != nil {
			_ = db.Close()
			fatal("failed to write the backup", err)
		}
		if err := db.Close(); err != nil {
			slog.Error("failed to close the database", "error", err)
		}
		slog.Info("backup written", "path", backupPath)
		return
	}

	// A loopback listen is where developing without a provider stays safe. On
	// any other address AUTH_DISABLED turns this into a DNS console for the
	// whole network, and for that a warning is not enough.
	if cfg.OIDC.Disabled && !config.ListensOnLoopback(cfg.Server.Listen) {
		slog.Error("oidc.disabled serves every record to whoever can reach this port",
			"listen", cfg.Server.Listen,
		)
		os.Exit(1)
	}

	st, err := store.New(cfg)
	if err != nil {
		fatal("failed to open the database", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			slog.Error("failed to close the database", "error", err)
		}
	}()

	// Import credentials from env vars on first run
	bootstrap(st)

	detector := ipdetect.NewWithJSON(cfg.IPDetect.V4, cfg.IPDetect.V6, toIPEndpoints(cfg.IPDetect.V4JSON), toIPEndpoints(cfg.IPDetect.V6JSON), cfg.IPDetect.Timeout)
	sender := notifier.New(st.Encryptor(), notifier.Config{AllowPrivate: cfg.Notifier.AllowPrivate})
	// The browser signs in on its own and sends the resulting access token with
	// every call; this process only checks it, which is what keeps it stateless.
	verifier := auth.New(cfg.OIDC)
	executor := ddns.NewExecutor(st, detector, sender, cfg.DNSPod, cfg.Scheduler, cfg.Cache)
	scheduler := ddns.NewScheduler(st, executor, cfg.Scheduler.Enabled)
	// The certificate manager answers ACME challenges through the same DNSPod
	// accounts and announces the outcome through the same channels.
	certManager := acme.NewManager(st, cfg.ACME, cfg.DNSPod, sender)
	certScheduler := acme.NewScheduler(certManager)
	// How long shutdown gives a run in flight before it stops waiting for it.
	const issuanceShutdownWait = 10 * time.Second

	gin.SetMode(cfg.Server.Mode)

	// One normalized list goes to both parsers - gin's and the middleware's -
	// so the address in the log and the address gin reports cannot drift apart.
	if err := middleware.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		fatal("invalid server.trusted_proxies", err)
	}

	// The rejection budget is the floor that holds when no proxy rate-limits,
	// so it is applied whatever the trusted-proxy list says.
	middleware.SetRejectBudgetPerMinute(cfg.Server.RejectBudgetPerMinute)

	r := gin.New()
	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		fatal("failed to set trusted proxies", err)
	}
	r.Use(
		middleware.RequestID(),
		middleware.SecureHeaders(idpOrigins(cfg)...),
		middleware.HSTS(cfg.Server.HSTS),
		middleware.Metrics(),
		middleware.RequestLog(),
		// A cap on every request body, so one oversized JSON is a 413 rather than
		// an OOM. The reader is lazy, so a request rejected by Bearer below never
		// has its body pulled in.
		middleware.MaxBody(middleware.DefaultMaxBody),
		// The bundled recovery answers with a bare 500 and an empty body, so a
		// panic would be the one failure without the error envelope and without
		// the request id that leads to its log line.
		gin.CustomRecovery(func(c *gin.Context, recovered any) {
			// The stack is what makes the panic fixable; gin's own recovery
			// prints it, so this one has to as well.
			slog.Error("panic while handling a request",
				"rid", middleware.RequestIDFrom(c),
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			middleware.WriteError(c, http.StatusInternalServerError, "internal server error")
		}),
	)

	srv := api.NewServer(cfg, st, scheduler, detector, sender, verifier, certManager)
	srv.RegisterRoutes(r, *cfg)

	// Static assets and the SPA fallback must be registered last
	frontend.Register(r)

	httpSrv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := scheduler.Start(); err != nil {
		slog.Error("failed to start the scheduler", "error", err)
		// os.Exit skips deferred cleanup, so the store is closed here
		// explicitly. The HTTP server never started, so there is nothing to
		// shut down.
		if err := st.Close(); err != nil {
			slog.Error("failed to close the database", "error", err)
		}
		os.Exit(1)
	}
	if err := certScheduler.Start(); err != nil {
		// Same reasoning as above, and the DDNS schedule is stopped first so
		// the process does not leave one of the two running behind it.
		slog.Error("failed to start the certificate renewal check", "error", err)
		scheduler.Stop()
		if err := st.Close(); err != nil {
			slog.Error("failed to close the database", "error", err)
		}
		os.Exit(1)
	}

	// The serve error is reported back to the main goroutine instead of being
	// logged in here: os.Exit from a goroutine would terminate the process
	// without running any of the main goroutine's cleanup.
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Server.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	// Graceful shutdown: stop the scheduler first, then the HTTP server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		slog.Error("HTTP server stopped with an error", "error", err)
		// A listener that never came up is a failed start, and it must not exit
		// 0: an orchestrator that only restarts on failure would keep a process
		// that is perfectly happy and serves nothing. The store is closed here
		// because os.Exit skips the deferred Close.
		if err := st.Close(); err != nil {
			slog.Error("failed to close the database", "error", err)
		}
		os.Exit(1)
	case <-quit:
		slog.Info("shutdown signal received")
	}

	scheduler.Stop()
	certScheduler.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
		// Drop the requests still in flight, so the deferred Close of the store
		// does not pull the database out from under them.
		_ = httpSrv.Close()
	}
	// An issuance in flight gets a moment to take its challenge record back out
	// of DNS, which is the one thing an abandoned run leaves behind. It is not
	// waited out in full: an issuance can take minutes, and a service that will
	// not stop is worse than a leftover TXT record.
	//
	// After the server has drained, so that no request can start another run
	// while this is waiting for the ones already going.
	waitCtx, cancelWait := context.WithTimeout(context.Background(), issuanceShutdownWait)
	if !certManager.Wait(waitCtx) {
		// A leftover _acme-challenge record is the one thing an abandoned run
		// leaves behind, so it is named rather than described.
		slog.Warn("a certificate issuance is still running, shutting down anyway",
			"waited", issuanceShutdownWait.String(),
			"check_for", "_acme-challenge",
		)
	}
	cancelWait()

	// The store is closed by the deferred call, after the server has drained.
	slog.Info("stopped")
}
