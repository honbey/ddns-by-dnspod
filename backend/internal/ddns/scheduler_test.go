package ddns

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/robfig/cron/v3"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/store"
)

// Tasks only share a probe and a record listing when they agree on the account,
// the domain and the address family, so those three decide the buckets.
func TestGroupTasksBucketsByAccountDomainAndFamily(t *testing.T) {
	tasks := []*model.DDNSTask{
		{ID: 1, AccountID: 1, Domain: "example.com", RecordType: "A", SubDomain: "a"},
		{ID: 2, AccountID: 1, Domain: "example.com", RecordType: "A", SubDomain: "b"},
		{ID: 3, AccountID: 1, Domain: "example.com", RecordType: "AAAA", SubDomain: "a"},
		{ID: 4, AccountID: 2, Domain: "example.com", RecordType: "A", SubDomain: "a"},
		{ID: 5, AccountID: 1, Domain: "other.com", RecordType: "A", SubDomain: "a"},
	}

	groups := groupTasks(tasks)

	got := make([][]uint, 0, len(groups))
	for _, g := range groups {
		ids := make([]uint, 0, len(g))
		for _, task := range g {
			ids = append(ids, task.ID)
		}
		got = append(got, ids)
	}

	// The two hosts of one domain stay together; AAAA, the other account and
	// the other domain each need their own round trip.
	want := [][]uint{{1, 2}, {3}, {4}, {5}}
	if len(got) != len(want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("groups = %v, want %v", got, want)
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("groups = %v, want %v", got, want)
			}
		}
	}
}

type countingDetector struct {
	calls int
	ip    string
	err   error
}

func (d *countingDetector) Detect(context.Context, ipdetect.Family) (string, error) {
	d.calls++
	return d.ip, d.err
}

// The public address is probed once for the whole group, not once per task:
// a host keeping a dozen subdomains up to date must not issue a dozen probes.
func TestRunGroupProbesOnceForTheWholeGroup(t *testing.T) {
	db := newTestStore(t)

	secret, err := db.Encryptor().Encrypt("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	account := model.Account{Name: "acc", SecretID: "AKID", SecretKey: secret}
	if err := db.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	var tasks []*model.DDNSTask
	for _, sub := range []string{"a", "b", "c"} {
		task := model.DDNSTask{
			AccountID:  account.ID,
			Domain:     "example.com",
			SubDomain:  sub,
			RecordType: "A",
			RecordLine: "默认",
		}
		if err := db.DB().Create(&task).Error; err != nil {
			t.Fatalf("create task: %v", err)
		}
		tasks = append(tasks, &task)
	}

	e := NewExecutor(db, nil, nil, config.DNSPodConfig{}, config.SchedulerConfig{}, config.CacheConfig{})
	// Failing the probe keeps the run off the network: the group stops before
	// it ever reaches DNSPod.
	det := &countingDetector{err: errors.New("unreachable")}
	e.detector = det

	results := e.RunGroup(context.Background(), tasks)

	if det.calls != 1 {
		t.Fatalf("probe calls = %d, want 1 for the whole group", det.calls)
	}
	if len(results) != len(tasks) {
		t.Fatalf("results = %d, want one per task (%d)", len(results), len(tasks))
	}
	for i, r := range results {
		if r.Status != model.StatusFAIL {
			t.Errorf("task %d: status = %q, want FAIL: the group shares one probe, so all fail together", i, r.Status)
		}
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: filepath.Join(dir, "test.db")},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(dir, "secret.key")},
	}
	st, err := store.New(cfg)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// A reload that cannot read the task list leaves the schedule as it was.
//
// Callers report a reload failure only to the log, so dropping every entry
// first would leave the process serving, and looking healthy, with nothing
// scheduled until the next start.
func TestReloadKeepsTheScheduleWhenTheReadFails(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: filepath.Join(dir, "ddns.db")},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(dir, "secret.key")},
		Server:   config.ServerConfig{Mode: "release"},
	}
	st, err := store.New(cfg)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	// The task table carries a foreign key on account_id, so the account the
	// task points at has to exist first.
	account := model.Account{Name: "a", SecretID: "id", SecretKey: "key", KeyHint: "hint"}
	if err := st.DB().Create(&account).Error; err != nil {
		t.Fatalf("insert account: %v", err)
	}

	task := model.DDNSTask{
		AccountID:  account.ID,
		Domain:     "example.com",
		SubDomain:  "a",
		RecordType: "A",
		CronExpr:   "*/5 * * * *",
		Enabled:    true,
	}
	if err := st.DB().Create(&task).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	// The executor is not under test here: Reload never touches it.
	s := &Scheduler{store: st, enabled: true, cron: cron.New()}
	if err := s.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s.taskCount != 1 || len(s.entryIDs) != 1 {
		t.Fatalf("after a good reload: %d tasks in %d entries, want 1 and 1", s.taskCount, len(s.entryIDs))
	}

	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.Reload(); err == nil {
		t.Fatal("a reload against a closed database must report a failure")
	}
	if s.taskCount != 1 || len(s.entryIDs) != 1 {
		t.Fatalf("after a failed reload: %d tasks in %d entries, want the old schedule kept", s.taskCount, len(s.entryIDs))
	}
}
