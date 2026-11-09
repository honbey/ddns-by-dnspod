package ddns

import (
	"context"
	"sync"
	"testing"
	"time"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
)

func TestComputeDurationWithoutIPSince(t *testing.T) {
	task := &model.DDNSTask{}
	if got := computeDuration(task); got != nil {
		t.Fatalf("want nil when the IP start time is unknown, got %v", *got)
	}
}

func TestComputeDurationCountsSeconds(t *testing.T) {
	since := time.Now().Add(-90 * time.Second)
	task := &model.DDNSTask{IPSince: &since}

	got := computeDuration(task)
	if got == nil {
		t.Fatal("want a duration when the IP start time is known")
	}
	if *got < 88 || *got > 95 {
		t.Fatalf("want roughly 90 seconds, got %d", *got)
	}
}

// The executor is shared between the scheduler and the HTTP layer, so the
// in-flight guard has to reject a second concurrent run of the same task.
func TestTryStartBlocksConcurrentRunOfSameTask(t *testing.T) {
	e := &Executor{running: make(map[uint]struct{})}

	if !e.tryStart(7) {
		t.Fatal("first run must be allowed")
	}
	if e.tryStart(7) {
		t.Fatal("second concurrent run must be rejected")
	}

	e.finish(7)
	if !e.tryStart(7) {
		t.Fatal("run must be allowed again once the previous one finished")
	}
}

func TestTryStartAllowsDifferentTasks(t *testing.T) {
	e := &Executor{running: make(map[uint]struct{})}

	if !e.tryStart(1) || !e.tryStart(2) {
		t.Fatal("different tasks must not block each other")
	}
}

func TestTryStartIsSafeUnderConcurrency(t *testing.T) {
	e := &Executor{running: make(map[uint]struct{})}

	var wg sync.WaitGroup
	var started int
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.tryStart(42) {
				mu.Lock()
				started++
				mu.Unlock()
				e.finish(42)
			}
		}()
	}
	wg.Wait()

	if started == 0 {
		t.Fatal("at least one run must start")
	}
	if _, ok := e.running[42]; ok {
		t.Fatal("in-flight map must be empty once every run finished")
	}
}

// A run whose deadline has passed has learned nothing about its tasks. Recording
// a failure would write a history row and send an alert about a DNS change that
// never happened, which is worse than recording nothing at all.
func TestARunThatWasCancelledRecordsNoFailure(t *testing.T) {
	st := newTestStore(t)

	encrypted, err := st.Encryptor().Encrypt("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	account := model.Account{Name: "acc", SecretID: "id", SecretKey: encrypted, KeyHint: "t"}
	if err := st.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	task := model.DDNSTask{
		AccountID:  account.ID,
		Domain:     "example.com",
		SubDomain:  "home",
		RecordType: "A",
	}
	if err := st.DB().Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	e := NewExecutor(st,
		ipdetect.NewWithJSON(nil, nil, nil, nil, 1),
		notifier.New(nil, notifier.Config{}),
		config.DNSPodConfig{}, config.SchedulerConfig{}, config.CacheConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := e.Run(ctx, &task)
	if result.Status != model.StatusSKIP {
		t.Fatalf("status = %q, want SKIP: nothing was attempted", result.Status)
	}
	if result.Message != runEndedMessage {
		t.Fatalf("message = %q, want %q", result.Message, runEndedMessage)
	}

	var history int64
	if err := st.DB().Model(&model.DDNSHistory{}).Count(&history).Error; err != nil {
		t.Fatalf("count history: %v", err)
	}
	if history != 0 {
		t.Fatalf("a cancelled run must write no history, got %d rows", history)
	}

	var stored model.DDNSTask
	if err := st.DB().First(&stored, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.FailCount != 0 || stored.LastStatus == model.StatusFAIL {
		t.Fatalf("fail_count = %d, last_status = %q: a cancelled run is not a failure",
			stored.FailCount, stored.LastStatus)
	}
}
