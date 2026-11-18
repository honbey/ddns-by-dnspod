package acme

import (
	"context"
	"crypto"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// newTestManager builds a manager over a throwaway database, which is what the
// tests about stored state need. In memory rather than a file: the store's first
// migration on a fresh file is fsync-bound and costs about a second per test.
// The store pins itself to one connection, so the schema lasts as long as the
// test; one that needs to reopen a database has to name a path of its own.
func newTestManager(t *testing.T, cfg config.ACMEConfig) (*Manager, *store.Store) {
	t.Helper()
	st, err := store.New(&config.Config{
		Database: config.DatabaseConfig{Path: ":memory:"},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(t.TempDir(), "secret.key")},
	})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return NewManager(st, cfg, config.DNSPodConfig{}, notifier.New(st.Encryptor(), notifier.Config{})), st
}

// The path is what the UI shows and what somebody mounts into the application
// that serves TLS, so it has to be the configured root plus the directory the
// certificate asked for.
func TestOutputPath(t *testing.T) {
	root := t.TempDir()
	// OutputPath never touches the store, so this manager is built without one
	// rather than over a database the test has no use for.
	manager := NewManager(nil, config.ACMEConfig{OutputDir: root}, config.DNSPodConfig{}, nil)

	cases := []struct {
		name string
		cert model.Certificate
		want string
	}{
		{
			name: "named after the certificate",
			cert: model.Certificate{Name: "site"},
			want: filepath.Join(root, "site"),
		},
		{
			name: "a directory of its own",
			cert: model.Certificate{Name: "site", OutputDir: filepath.Join("shared", "tls")},
			want: filepath.Join(root, "shared", "tls"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := manager.OutputPath(&tc.cert)
			if got != tc.want {
				t.Fatalf("OutputPath = %q, want %q", got, tc.want)
			}
			// Whatever the certificate carries stays under the root: this is
			// the promise the API's validation relies on.
			if rel, err := filepath.Rel(root, got); err != nil || rel == ".." {
				t.Fatalf("OutputPath %q is outside %q", got, root)
			}
		})
	}
}

func TestDueForRenewal(t *testing.T) {
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	at := func(days int) *time.Time {
		when := now.AddDate(0, 0, days)
		return &when
	}

	const daily = "0 3 * * *"
	certs := []model.Certificate{
		{ID: 1, RenewCron: daily, NotAfter: at(10)},                     // inside the default window
		{ID: 2, RenewCron: daily, NotAfter: at(31)},                     // not yet
		{ID: 3, RenewCron: daily, NotAfter: nil},                        // never issued
		{ID: 4, RenewCron: "", NotAfter: at(1)},                         // manual only
		{ID: 5, RenewCron: daily, NotAfter: at(-2)},                     // already expired
		{ID: 6, RenewCron: daily, NotAfter: at(30)},                     // exactly on the edge
		{ID: 7, RenewCron: "", NotAfter: at(-30)},                       // manual, and long expired
		{ID: 8, RenewCron: daily, RenewBeforeDays: 5, NotAfter: at(10)}, // its own, tighter window
		{ID: 9, RenewCron: "0 4 * * *", NotAfter: at(1)},                // another hour: not now
		{ID: 10, RenewCron: "not a schedule", NotAfter: at(1)},          // unreadable: skipped
	}

	got := dueForRenewal(certs, 30, now, true)
	want := []uint{1, 3, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("dueForRenewal = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dueForRenewal = %v, want %v", got, want)
		}
	}
}

// The check at start ignores the schedules: a monthly expression can fall
// inside a restart, and what matters then is the window, not the minute.
func TestDueForRenewalAtStart(t *testing.T) {
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	at := func(days int) *time.Time {
		when := now.AddDate(0, 0, days)
		return &when
	}

	certs := []model.Certificate{
		{ID: 1, RenewCron: "0 5 1 * *", NotAfter: at(10)},     // another day: still caught
		{ID: 2, RenewCron: "0 3 1 * *", NotAfter: at(45)},     // outside the window
		{ID: 3, RenewCron: "not a schedule", NotAfter: at(1)}, // unreadable, but due
		{ID: 4, RenewCron: "", NotAfter: at(1)},               // manual only
	}

	got := dueForRenewal(certs, 30, now, false)
	want := []uint{1, 3}
	if len(got) != len(want) {
		t.Fatalf("dueForRenewal = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dueForRenewal = %v, want %v", got, want)
		}
	}
}

// A certificate with no expiry has never been issued, and creating one with a
// schedule is how it is asked for without pressing anything.
func TestDueForRenewalWithNoCertificates(t *testing.T) {
	if got := dueForRenewal(nil, 30, time.Now(), true); len(got) != 0 {
		t.Fatalf("dueForRenewal = %v, want nothing", got)
	}
}

// A failure can be announced before the names could be read - a row whose
// domain list is unusable is announced exactly that way - and the message used
// to be built by indexing that list blind. A panic here ends the process, which
// is every schedule in it.
func TestRecordFailureWithoutNames(t *testing.T) {
	manager, st := newTestManager(t, config.ACMEConfig{Enabled: true, OutputDir: t.TempDir()})

	// An enabled channel, because there was nowhere to send anything without
	// one: the panic needed a channel to happen at all.
	channel := model.NotifyChannel{
		Name:    "catch",
		Type:    model.ChannelWebhook,
		Enabled: true,
		Config:  `{"url":"http://127.0.0.1:9/hook"}`,
	}
	if err := st.DB().Create(&channel).Error; err != nil {
		t.Fatalf("create the channel: %v", err)
	}

	cert := model.Certificate{Name: "site", Domains: "", AccountID: 1, KeyType: KeyEC256}
	if err := st.DB().Create(&cert).Error; err != nil {
		t.Fatalf("create the certificate: %v", err)
	}

	// What the run does when the domain list cannot be read.
	manager.recordFailure(&cert, nil, "this certificate has no usable domain list")

	var stored model.Certificate
	if err := st.DB().First(&stored, cert.ID).Error; err != nil {
		t.Fatalf("reload the certificate: %v", err)
	}
	if stored.LastStatus != model.CertStatusFAIL {
		t.Fatalf("status = %q, want %q", stored.LastStatus, model.CertStatusFAIL)
	}
	if stored.FailCount != 1 {
		t.Fatalf("fail_count = %d, want 1", stored.FailCount)
	}
}

// The first issuance after a reset creates the account key. A run that gets
// there at the same moment loses the race at the unique index, and it has to
// use the key that won rather than report a failure for a certificate that has
// nothing wrong with it.
func TestCreateAccountKeyReusesTheStoredKeyOnConflict(t *testing.T) {
	manager, st := newTestManager(t, config.ACMEConfig{
		Enabled:   true,
		Directory: "https://ca.example/dir",
	})

	first, err := manager.createAccountKey()
	if err != nil {
		t.Fatalf("the first creation: %v", err)
	}
	second, err := manager.createAccountKey()
	if err != nil {
		t.Fatalf("the second creation must fall back to the stored key: %v", err)
	}
	if !second.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(first.Public()) {
		t.Fatal("the second run did not pick up the stored account key")
	}

	var rows int64
	if err := st.DB().Model(&model.ACMEAccount{}).Count(&rows).Error; err != nil {
		t.Fatalf("count the accounts: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d account rows, want 1", rows)
	}
}

// Shutdown waits for runs in flight, but not forever: an issuance can take
// minutes, and a service that will not stop is worse than a leftover record.
func TestWaitForRunningIssuances(t *testing.T) {
	manager, _ := newTestManager(t, config.ACMEConfig{Enabled: true})

	if !manager.Wait(context.Background()) {
		t.Fatal("an idle manager must not hold shutdown up")
	}

	// A run in flight, as the counter sees it.
	manager.inflight.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if manager.Wait(ctx) {
		t.Fatal("the wait reported an idle manager while a run was going")
	}

	manager.inflight.Done()
	if !manager.Wait(context.Background()) {
		t.Fatal("the run finished, so the wait should return")
	}
}

// Every run has to unwind the counter it was counted on, and the count has to
// be added before the goroutine starts: a Done without its Add is a panic in
// that goroutine, which ends the process as surely as anything else.
func TestStartUnwindsTheInFlightCount(t *testing.T) {
	manager, st := newTestManager(t, config.ACMEConfig{
		Enabled:   true,
		Timeout:   5,
		OutputDir: t.TempDir(),
	})
	// The account it names does not exist, so the run fails at its first step
	// and reaches the end of the goroutine without touching the network.
	cert := model.Certificate{Name: "site", Domains: `["example.com"]`, KeyType: KeyEC256, AccountID: 4242}
	if err := st.DB().Create(&cert).Error; err != nil {
		t.Fatalf("create the certificate: %v", err)
	}

	if err := manager.Start(cert.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		var stored model.Certificate
		if err := st.DB().First(&stored, cert.ID).Error; err != nil {
			t.Fatalf("reload the certificate: %v", err)
		}
		if stored.LastStatus == model.CertStatusFAIL {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run never reported a failure, status is %q", stored.LastStatus)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !manager.Wait(context.Background()) {
		t.Fatal("the run finished but the manager still counts it as in flight")
	}
}

// Two at a time, and a run that cannot take a slot inside its own deadline is
// reported rather than left queued for as long as it takes.
func TestIssuanceSlotsBoundTheConcurrency(t *testing.T) {
	manager, st := newTestManager(t, config.ACMEConfig{
		Enabled:   true,
		Timeout:   1,
		OutputDir: t.TempDir(),
	})
	cert := model.Certificate{Name: "site", Domains: `["example.com"]`, KeyType: KeyEC256, AccountID: 1}
	if err := st.DB().Create(&cert).Error; err != nil {
		t.Fatalf("create the certificate: %v", err)
	}

	// Every slot taken, as they would be by other certificates being issued.
	for i := 0; i < maxConcurrentIssuances; i++ {
		manager.slots <- struct{}{}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.run(&cert)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the run never gave up waiting for a slot")
	}

	var stored model.Certificate
	if err := st.DB().First(&stored, cert.ID).Error; err != nil {
		t.Fatalf("reload the certificate: %v", err)
	}
	if stored.LastStatus != model.CertStatusFAIL {
		t.Fatalf("status = %q, want %q", stored.LastStatus, model.CertStatusFAIL)
	}
	if !strings.Contains(stored.LastMessage, "before it could start") {
		t.Fatalf("message = %q, want it to say the run never started", stored.LastMessage)
	}
}

// Upgrading must not turn automatic renewal into a manual one: the rows that
// had the old switch on get the schedule it stood for.
func TestAdoptLegacyAutoRenew(t *testing.T) {
	manager, st := newTestManager(t, config.ACMEConfig{Enabled: true, OutputDir: t.TempDir(), CheckHour: 4})
	db := st.DB()

	// What a database that predates the change carries: the switch, no
	// schedule, and NULL rather than an empty string in the new column.
	if err := db.Exec(`ALTER TABLE certificates ADD COLUMN auto_renew numeric`).Error; err != nil {
		t.Fatalf("add the legacy column: %v", err)
	}
	for _, row := range []struct {
		name      string
		autoRenew int
	}{{"on", 1}, {"off", 0}} {
		if err := db.Exec(
			`INSERT INTO certificates
			   (name, account_id, domains, key_type, auto_renew, notify_on_issue, created_at, updated_at)
			 VALUES (?, 1, '["example.com"]', 'ec256', ?, 1, '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
			row.name, row.autoRenew,
		).Error; err != nil {
			t.Fatalf("seed %s: %v", row.name, err)
		}
	}

	if err := manager.AdoptLegacyAutoRenew(); err != nil {
		t.Fatalf("AdoptLegacyAutoRenew: %v", err)
	}

	var certs []model.Certificate
	if err := db.Order("id").Find(&certs).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(certs) != 2 {
		t.Fatalf("got %d certificates, want 2", len(certs))
	}
	if certs[0].RenewCron != "0 4 1 * *" {
		t.Fatalf("the row that renewed daily got %q, want the configured hour", certs[0].RenewCron)
	}
	if certs[1].RenewCron != "" {
		t.Fatalf("the row that did not renew got %q, want no schedule", certs[1].RenewCron)
	}

	// The switch is consumed, not re-read on every start: otherwise a
	// certificate the operator later put back on manual would be given a
	// schedule again on the next boot.
	var flags []struct {
		ID        uint
		AutoRenew int
	}
	if err := db.Table("certificates").Select("id, auto_renew").Order("id").Scan(&flags).Error; err != nil {
		t.Fatalf("read the switch: %v", err)
	}
	for _, flag := range flags {
		if flag.AutoRenew != 0 {
			t.Fatalf("certificate %d still carries auto_renew = %d", flag.ID, flag.AutoRenew)
		}
	}

	// What the operator does next: back to manual.
	if err := db.Exec(`UPDATE certificates SET renew_cron = '' WHERE id = ?`, certs[0].ID).Error; err != nil {
		t.Fatalf("clear the schedule: %v", err)
	}
	if err := manager.AdoptLegacyAutoRenew(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var after []model.Certificate
	if err := db.Order("id").Find(&after).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after[0].RenewCron != "" {
		t.Fatalf("a manual certificate was given %q on the next start", after[0].RenewCron)
	}
}

// The scheduler asks each expression whether it names the current minute. A
// five-field schedule and a calendar descriptor both can; @every cannot, which
// is why the API refuses it rather than storing one that never fires.
func TestCronFires(t *testing.T) {
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 10, 15, hour, minute, 30, 0, time.UTC)
	}

	cases := []struct {
		expr string
		now  time.Time
		want bool
	}{
		{"0 3 * * *", at(3, 0), true},
		{"0 3 * * *", at(3, 1), false},
		{"0 4 1 * *", at(4, 0), false}, // another day of the month
		{"@daily", at(0, 0), true},
		{"@daily", at(0, 1), false},
		{"@every 2h", at(3, 0), false},
		{"not a schedule", at(3, 0), false},
	}

	for _, c := range cases {
		if got := cronFires(c.expr, c.now); got != c.want {
			t.Errorf("cronFires(%q, %s) = %v, want %v", c.expr, c.now.Format("15:04"), got, c.want)
		}
	}
}
