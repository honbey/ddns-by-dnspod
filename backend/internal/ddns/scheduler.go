package ddns

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/store"
)

// defaultCron is the default schedule
const defaultCron = "*/5 * * * *"

// taskTimeout is the timeout of a single task run
const taskTimeout = 2 * time.Minute

// Scheduler manages the cron scheduling of DDNS tasks
type Scheduler struct {
	exec    *Executor
	store   *store.Store
	cron    *cron.Cron
	enabled bool
	mu      sync.Mutex
	// One entry per distinct schedule, not per task: a tick runs every task
	// that shares it, so they can share one probe and one record listing.
	entryIDs  []cron.EntryID
	taskCount int
}

// NewScheduler builds a scheduler
func NewScheduler(st *store.Store, exec *Executor, enabled bool) *Scheduler {
	return &Scheduler{
		exec:    exec,
		store:   st,
		enabled: enabled,
		// SkipIfStillRunning keeps slow tasks from overlapping; Recover stops one panic from killing the scheduler
		cron: cron.New(cron.WithChain(
			cron.SkipIfStillRunning(cron.DefaultLogger),
			cron.Recover(cron.DefaultLogger),
		)),
	}
}

// Start loads tasks from the database and starts scheduling
func (s *Scheduler) Start() error {
	if err := s.Reload(); err != nil {
		return err
	}
	if !s.enabled {
		slog.Info("scheduler disabled, only manual runs are available")
		return nil
	}
	s.cron.Start()
	slog.Info("scheduler started", "tasks", s.taskCount)
	return nil
}

// Stop shuts the scheduler down gracefully, waiting for running tasks to finish
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	slog.Info("scheduler stopped")
}

// Reload reloads the task list; call it after any task change.
//
// The new schedule is built before the old one is dropped: a task list that
// cannot be read leaves the schedule running as it was, rather than taking
// every entry out and leaving nothing behind. Callers report a failure only to
// the log, so an empty schedule would go on serving until the next start.
func (s *Scheduler) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var tasks []model.DDNSTask
	if err := s.store.DB().Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return fmt.Errorf("load DDNS tasks: %w", err)
	}

	// Tasks that share a schedule are dispatched by a single entry. They would
	// otherwise be independent goroutines on the same tick, each probing the
	// public address and each listing the same domain's records, which is pure
	// waste when one host keeps a dozen subdomains up to date.
	bySpec := make(map[string][]uint)
	var specs []string
	for i := range tasks {
		spec := tasks[i].CronExpr
		if spec == "" {
			spec = defaultCron
		}
		if _, ok := bySpec[spec]; !ok {
			specs = append(specs, spec)
		}
		bySpec[spec] = append(bySpec[spec], tasks[i].ID)
	}

	// Collected before the swap, so one bad spec among many cannot leave the
	// scheduler holding half the old entries and half the new ones.
	var (
		entries   []cron.EntryID
		taskCount int
	)
	for _, spec := range specs {
		ids := bySpec[spec]
		entryID, err := s.cron.AddFunc(spec, func() {
			s.runTasks(ids)
		})
		if err != nil {
			slog.Error("invalid cron expression", "spec", spec, "error", err)
			continue
		}
		entries = append(entries, entryID)
		taskCount += len(ids)
	}

	for _, entryID := range s.entryIDs {
		s.cron.Remove(entryID)
	}
	s.entryIDs = entries
	s.taskCount = taskCount

	return nil
}

// RunNow triggers a task manually and returns the result synchronously
func (s *Scheduler) RunNow(id uint) (*Result, error) {
	var task model.DDNSTask
	if err := s.store.DB().First(&task, id).Error; err != nil {
		return nil, fmt.Errorf("task not found (id=%d): %w", id, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	return s.exec.Run(ctx, &task), nil
}

// runTasks runs every task of one tick, grouped so that the tasks of a single
// domain share one address probe and one record listing.
func (s *Scheduler) runTasks(ids []uint) {
	var tasks []*model.DDNSTask
	for _, id := range ids {
		var task model.DDNSTask
		// Filtered on enabled as well: the entries keep the ids they were built
		// with, so this re-reads a task that has been switched off since - or
		// one that survived because the reload after the change failed, which
		// only reaches the log. Without this it goes on writing DNS.
		if err := s.store.DB().Where("enabled = ?", true).First(&task, id).Error; err != nil {
			slog.Warn("task is gone or disabled, skipped", "task", id, "error", err)
			continue
		}
		tasks = append(tasks, &task)
	}
	if len(tasks) == 0 {
		return
	}

	// One timeout per group, not one for the whole tick: a slow group must not
	// leave the next one with an already expired context.
	for _, group := range groupTasks(tasks) {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		s.exec.RunGroup(ctx, group)
		cancel()
	}
}

// groupTasks buckets tasks that can share one round trip: the same account,
// the same domain and the same address family.
//
// The buckets follow the order of the input, so the grouping a run produces is
// reproducible rather than dependent on map iteration.
func groupTasks(tasks []*model.DDNSTask) [][]*model.DDNSTask {
	type groupKey struct {
		accountID uint
		domain    string
		family    ipdetect.Family
	}

	buckets := make(map[groupKey][]*model.DDNSTask)
	var order []groupKey
	for _, task := range tasks {
		k := groupKey{task.AccountID, task.Domain, ipdetect.FamilyOf(task.RecordType)}
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], task)
	}

	groups := make([][]*model.DDNSTask, 0, len(order))
	for _, k := range order {
		groups = append(groups, buckets[k])
	}
	return groups
}

// EntryCount returns the number of currently registered tasks
func (s *Scheduler) EntryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taskCount
}
