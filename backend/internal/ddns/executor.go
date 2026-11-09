// Package ddns implements the scheduling and execution of DDNS tasks.
package ddns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/metrics"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// Result is the outcome of a single task run
type Result struct {
	TaskID      uint    `json:"task_id"`
	Domain      string  `json:"domain"`
	SubDomain   string  `json:"sub_domain"`
	RecordType  string  `json:"record_type"`
	RecordID    *uint64 `json:"record_id"`
	OldIP       string  `json:"old_ip"`
	NewIP       string  `json:"new_ip"`
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Changed     bool    `json:"changed"`
	DurationSec *int64  `json:"duration_sec"`

	// notify marks a row the run should announce. The decision is per row - a
	// task opts in, a failure counts against its threshold - but the sending
	// happens once per run, so ten changed records are one message.
	notify bool
}

// addressDetector is the part of ipdetect the executor needs.
//
// It is kept narrow so a test can stand in for the real thing and count how
// often a group probes, which is the whole point of running tasks in groups.
type addressDetector interface {
	Detect(ctx context.Context, family ipdetect.Family) (string, error)
}

// Executor runs DDNS tasks
//
// A single Executor instance is shared by the scheduler and the HTTP layer,
// so the in-flight guard below covers both cron triggered runs and manual ones.
type Executor struct {
	store    *store.Store
	detector addressDetector
	sender   *notifier.Sender
	dnspod   config.DNSPodConfig
	sched    config.SchedulerConfig
	cache    config.CacheConfig

	mu      sync.Mutex
	running map[uint]struct{}
}

// NewExecutor builds an executor
func NewExecutor(st *store.Store, det *ipdetect.Detector, snd *notifier.Sender, dnspodCfg config.DNSPodConfig, schedCfg config.SchedulerConfig, cacheCfg config.CacheConfig) *Executor {
	return &Executor{
		store:    st,
		detector: det,
		sender:   snd,
		dnspod:   dnspodCfg,
		sched:    schedCfg,
		cache:    cacheCfg,
		running:  make(map[uint]struct{}),
	}
}

// cacheTTL mirrors the lifetimes the HTTP layer gives its clients, so switching
// the read cache off in the configuration switches it off here too.
func (e *Executor) cacheTTL() dnspod.CacheTTL {
	if !e.cache.Enabled {
		return dnspod.CacheTTL{}
	}
	return dnspod.CacheTTL{
		Domains: time.Duration(e.cache.DomainsSec) * time.Second,
		Records: time.Duration(e.cache.RecordsSec) * time.Second,
		Lines:   time.Duration(e.cache.LinesSec) * time.Second,
	}
}

// tryStart marks a task as running. It reports false when a previous run of the
// same task is still in flight, which can happen when a manual trigger races a cron tick.
func (e *Executor) tryStart(id uint) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.running[id]; ok {
		return false
	}
	e.running[id] = struct{}{}
	return true
}

// finish releases the in-flight mark of a task
func (e *Executor) finish(id uint) {
	e.mu.Lock()
	delete(e.running, id)
	e.mu.Unlock()
}

// Run executes a single task. It is RunGroup with exactly one member.
func (e *Executor) Run(ctx context.Context, task *model.DDNSTask) *Result {
	return e.RunGroup(ctx, []*model.DDNSTask{task})[0]
}

// RunGroup executes every task that shares an account, a domain and an address
// family in one pass.
//
// This is what keeps a host with a dozen subdomains affordable: the public
// address is probed once instead of once per task, and the domain's records are
// listed once instead of once per host. What belongs to a single task stays per
// task - its status, history row, failure counter - while the announcement is
// made once for the pass (see notifyRun).
//
// Members are updated sequentially on purpose. They share one DNSPod client,
// and a parallel fan out would race that client's read cache, turning the
// single listing back into one call per record.
//
// What one pass changed is announced once, at the end (see notifyRun): a host
// whose address moved takes every record with it, and one alert per record is
// how a person learns to ignore alerts.
func (e *Executor) RunGroup(ctx context.Context, tasks []*model.DDNSTask) []*Result {
	results := e.runGroup(ctx, tasks)
	e.notifyRun(results)
	return results
}

// runGroup is RunGroup without the announcement, so that every exit path -
// including the ones that give up before touching a record - reports the same.
func (e *Executor) runGroup(ctx context.Context, tasks []*model.DDNSTask) []*Result {
	results := make([]*Result, len(tasks))
	var active []int

	for i, task := range tasks {
		results[i] = &Result{
			TaskID:     task.ID,
			Domain:     task.Domain,
			SubDomain:  task.SubDomain,
			RecordType: task.RecordType,
		}
		if !e.tryStart(task.ID) {
			results[i].Status = model.StatusSKIP
			results[i].Message = "previous run still in flight, skipped"
			results[i].OldIP = task.CurrentIP
			continue
		}
		active = append(active, i)
	}
	defer func() {
		for _, i := range active {
			e.finish(tasks[i].ID)
		}
	}()

	if len(active) == 0 {
		return results
	}

	// The group shares one account, so it is read and decrypted once.
	// Every member uses the same domain and address family by construction.
	leader := tasks[active[0]]

	var account model.Account
	if err := e.store.DB().First(&account, leader.AccountID).Error; err != nil {
		return e.failEach(results, active, tasks, "", fmt.Sprintf("account not found (id=%d)", leader.AccountID))
	}

	secretKey, err := e.store.Encryptor().Decrypt(account.SecretKey)
	if err != nil {
		slog.Error("could not decrypt the account secret key", "domain", leader.Domain, "account", leader.AccountID, "error", err)
		return e.failEach(results, active, tasks, "", "the credentials of this account could not be read")
	}

	cli, err := dnspod.NewClientWithCache(account.SecretID, secretKey, e.dnspod.Endpoint, e.dnspod.Timeout, e.cacheTTL())
	if err != nil {
		slog.Error("could not create the DNSPod client", "domain", leader.Domain, "error", err)
		return e.failEach(results, active, tasks, "", "DNSPod could not be reached with this account")
	}

	// Pick the address family from the record type so an IPv4 never lands in an AAAA record
	family := ipdetect.FamilyOf(leader.RecordType)
	ip, err := e.detector.Detect(ctx, family)
	if err != nil {
		if ctx.Err() != nil {
			// The run gave up; the probe did not fail. Recorded as a task that
			// was never reached, which writes no history and sends no alert.
			return e.markUnreached(results, tasks, "")
		}
		slog.Error("could not detect the public address", "domain", leader.Domain, "family", family, "error", err)
		return e.failEach(results, active, tasks, "", fmt.Sprintf("the public %s address could not be detected", family))
	}

	// The detected address belongs to this host, not to any one domain, so it is
	// tracked separately from the per-task history. It is recorded before the
	// DNS update: a failed update still leaves the egress address changed.
	if err := e.store.RecordPublicIPChange(family.Key(), ip, time.Now()); err != nil {
		slog.Error("could not read the recorded public address", "family", family.Key(), "error", err)
		metrics.IncPublicIPChangeError()
	}

	// One listing per distinct record type and line. Leaving the host out of the
	// filter is what collapses one call per task into one call per domain; the
	// host is matched locally afterwards.
	for _, members := range groupByRecordTypeAndLine(active, tasks) {
		head := tasks[members[0]]
		page, err := cli.ListRecords(ctx, head.Domain, dnspod.RecordFilter{
			RecordType: head.RecordType,
			Line:       head.RecordLine,
		})
		if err != nil {
			if ctx.Err() != nil {
				// Not the API failing: this run gave up. The rest of the group is
				// in the same position, so it stops here and records nothing.
				return e.markUnreached(results, tasks, ip)
			}
			slog.Error("could not list the records", "domain", head.Domain, "error", err)
			for _, i := range members {
				results[i] = e.failed(tasks[i], ip, "the records of this domain could not be listed")
			}
			continue
		}

		// Only the first record of a host is kept: DNSPod allows several records
		// with the same host, type and line, and a task has no way to tell which
		// one it owns. The extra ones keep their old value until they are
		// consolidated into a single record.
		records := make(map[string]dnspod.Record, len(page.Records))
		for _, r := range page.Records {
			if _, exists := records[r.SubDomain]; !exists {
				records[r.SubDomain] = r
			}
		}

		for _, i := range members {
			// The run's deadline has to mean something between tasks: the SDK
			// call inside apply cannot be interrupted, but the next task's must
			// not start once the caller has given up on this one.
			if err := ctx.Err(); err != nil {
				return e.markUnreached(results, tasks, ip)
			}
			rec, ok := records[tasks[i].SubDomain]
			if !ok {
				results[i] = e.failed(tasks[i], ip, "no matching record found, check the domain, host record and line")
				continue
			}
			results[i] = e.apply(ctx, cli, tasks[i], ip, &rec)
		}
	}

	return results
}

// failEach records the same failure on every member of a group. It is used for
// the steps a group shares, where one failure means no member can go on.
func (e *Executor) failEach(results []*Result, members []int, tasks []*model.DDNSTask, ip, reason string) []*Result {
	for _, i := range members {
		results[i] = e.failed(tasks[i], ip, reason)
	}
	return results
}

// groupByRecordTypeAndLine buckets the members that can share one record listing.
// The order of the buckets follows the order of the members, so a run stays
// reproducible.
func groupByRecordTypeAndLine(members []int, tasks []*model.DDNSTask) [][]int {
	type key struct {
		recordType string
		line       string
	}

	buckets := make(map[key][]int)
	var order []key
	for _, i := range members {
		k := key{tasks[i].RecordType, tasks[i].RecordLine}
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], i)
	}

	out := make([][]int, 0, len(order))
	for _, k := range order {
		out = append(out, buckets[k])
	}
	return out
}

// apply compares one record against the detected address and updates it when
// they differ. It runs once per task, on a record the group already fetched.
//
// Key design: every run trusts the real record value on DNSPod instead of a local cache,
// so behaviour stays correct even if the local database is wiped or records are edited in the console.
// The old Python version relied on a local last_ip and misbehaved once that state was lost.
func (e *Executor) apply(ctx context.Context, cli *dnspod.Client, task *model.DDNSTask, ip string, rec *dnspod.Record) *Result {
	result := &Result{
		TaskID:     task.ID,
		Domain:     task.Domain,
		SubDomain:  task.SubDomain,
		RecordType: task.RecordType,
		OldIP:      rec.Value,
		NewIP:      ip,
		RecordID:   &rec.RecordID,
	}

	// IP unchanged: only refresh the run timestamp and skip history, so the table is not flooded
	if rec.Value == ip {
		now := time.Now()
		// A failed update leaves the task row stale: last_run_at stays behind and
		// ip_since never moves, which quietly corrupts the duration stats. The
		// DNS record itself is unaffected, so the run still counts as a SKIP.
		if err := e.store.DB().Model(&model.DDNSTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"last_run_at":  now,
			"last_status":  model.StatusSKIP,
			"last_message": fmt.Sprintf("IP unchanged (%s)", ip),
			"fail_count":   0,
			"current_ip":   ip,
		}).Error; err != nil {
			slog.Error("could not update the task state", "task", task.ID, "error", err)
		}
		// Record when the current IP became effective, on the first run
		if task.IPSince == nil {
			if err := e.store.DB().Model(&model.DDNSTask{}).Where("id = ?", task.ID).
				Update("ip_since", now).Error; err != nil {
				slog.Error("could not update ip_since", "task", task.ID, "error", err)
			}
		}
		result.Status = model.StatusSKIP
		result.Message = fmt.Sprintf("IP unchanged (%s)", ip)
		slog.Info("address unchanged", "task", task.ID, "name", task.SubDomain+"."+task.Domain, "address", ip)
		return result
	}

	// Compute how long the previous IP actually lived
	result.DurationSec = computeDuration(task)

	input := dnspod.ModifyRecordInput{
		SubDomain:  task.SubDomain,
		RecordType: task.RecordType,
		Line:       task.RecordLine,
		Value:      ip,
		// Keep the record status so a record paused in the console is not force-enabled
		Status: rec.Status,
	}
	// The task carries its own TTL, and every update is a chance to apply it,
	// so changing it takes effect on the next run rather than never. Zero or a
	// negative value leaves DNSPod's own value alone.
	if task.TTL > 0 {
		input.TTL = uint64(task.TTL)
	}

	if err := cli.ModifyRecord(ctx, task.Domain, rec.RecordID, input); err != nil {
		slog.Error("could not update the record", "task", task.ID, "error", err)
		result := e.failed(task, ip, "the record could not be updated")
		// The old value and the record id belong to this run even when the write
		// failed: without them the first-failure history row cannot say what it
		// was trying to move away from.
		result.OldIP = rec.Value
		result.RecordID = &rec.RecordID
		return result
	}

	now := time.Now()

	// Same reasoning as on the SKIP path: the record was updated on DNSPod
	// already, so a failure here is not worth failing the run over, but it does
	// leave the task state behind
	if err := e.store.DB().Model(&model.DDNSTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"current_ip":   ip,
		"ip_since":     now,
		"last_run_at":  now,
		"last_status":  model.StatusOK,
		"last_message": fmt.Sprintf("%s -> %s", rec.Value, ip),
		"fail_count":   0,
	}).Error; err != nil {
		slog.Error("could not update the task state", "task", task.ID, "error", err)
	}

	result.Status = model.StatusOK
	result.Changed = true
	result.Message = fmt.Sprintf("%s -> %s", rec.Value, ip)
	slog.Info("record updated", "task", task.ID, "name", task.SubDomain+"."+task.Domain, "from", rec.Value, "to", ip)

	e.writeHistory(task, result)

	// On a successful change, mark it for the run's announcement; only when the
	// task enables it
	if task.NotifyOnChange {
		result.notify = true
	}

	return result
}

// runEndedMessage marks a task the run never reached. It is deliberately not a
// failure sentence: nothing was attempted, so nothing is known about it.
const runEndedMessage = "the run ended before this record was written"

// untouched records a task the run never reached, and records it nowhere else.
//
// failed is the wrong tool for this: it bumps fail_count, writes a history row
// on the first failure and sends an alert. A run that stopped on its own
// deadline has learned nothing about this record, so writing "the write failed"
// would page someone about a DNS change that never happened.
func (e *Executor) untouched(task *model.DDNSTask, ip string) *Result {
	return &Result{
		TaskID:     task.ID,
		Domain:     task.Domain,
		SubDomain:  task.SubDomain,
		RecordType: task.RecordType,
		OldIP:      task.CurrentIP,
		NewIP:      ip,
		Status:     model.StatusSKIP,
		Message:    runEndedMessage,
	}
}

// markUnreached answers for every task the run never got to: a run that ended
// early still reports on each of its tasks, and every one of those answers says
// plainly that nothing was attempted.
//
// A result that already carries a status is left alone - a task that ran, failed
// or was skipped because another run of it is in flight has its answer already.
func (e *Executor) markUnreached(results []*Result, tasks []*model.DDNSTask, ip string) []*Result {
	for i := range results {
		switch results[i].Status {
		case model.StatusOK, model.StatusFAIL, model.StatusSKIP:
		default:
			results[i] = e.untouched(tasks[i], ip)
		}
	}
	return results
}

// failed handles the failure path: bump the counter, maybe write history, maybe alert.
//
// The reason it is given is what ends up in the task row, in the change history
// and in every notification, so it carries no internals: the callers log the
// real error and pass a sentence that is safe to show and safe to send out.
func (e *Executor) failed(task *model.DDNSTask, ip, reason string) *Result {
	result := &Result{
		TaskID:     task.ID,
		Domain:     task.Domain,
		SubDomain:  task.SubDomain,
		RecordType: task.RecordType,
		NewIP:      ip,
		Status:     model.StatusFAIL,
		Message:    reason,
	}

	failCount := task.FailCount + 1
	now := time.Now()

	// fail_count is what the alert policy counts on, so a failed write means the
	// next run repeats the "first failure" alert instead of spacing them out
	if err := e.store.DB().Model(&model.DDNSTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"last_run_at":  now,
		"last_status":  model.StatusFAIL,
		"last_message": reason,
		"fail_count":   failCount,
	}).Error; err != nil {
		slog.Error("could not update the task state", "task", task.ID, "error", err)
	}

	slog.Error("task failed", "task", task.ID, "name", task.SubDomain+"."+task.Domain, "reason", reason)

	// History is written on the first failure only. Ongoing failure state is carried by the task
	// last_status / fail_count, otherwise every cron tick would add another row.
	if failCount == 1 {
		e.writeHistory(task, result)
	}

	// Alert policy: notify on the first failure, then once every threshold failures,
	// so a permanently failing task does not push on every tick.
	threshold := e.sched.FailThreshold
	if threshold <= 0 {
		threshold = 3
	}
	if failCount == 1 || failCount%threshold == 0 {
		result.notify = true
	}

	return result
}

// writeHistory appends a history row. SKIP is not recorded, to avoid flooding the table.
func (e *Executor) writeHistory(task *model.DDNSTask, r *Result) {
	entry := model.DDNSHistory{
		TaskID:      &task.ID,
		Domain:      r.Domain,
		SubDomain:   r.SubDomain,
		RecordType:  r.RecordType,
		RecordID:    r.RecordID,
		OldIP:       r.OldIP,
		NewIP:       r.NewIP,
		Status:      r.Status,
		Message:     r.Message,
		DurationSec: r.DurationSec,
	}
	if err := e.store.DB().Create(&entry).Error; err != nil {
		slog.Error("could not write the history", "task", task.ID, "error", err)
	}
}

// notifyRun announces what one pass changed, in one message.
//
// The decision to announce is per row (a task opts in, a failure counts against
// its threshold); the sending is per run, because an address change takes every
// record on the host with it and one alert per record trains people to ignore
// them. A run that changed one record says exactly what it always said, so the
// common case reads the same.
func (e *Executor) notifyRun(results []*Result) {
	var channels []model.NotifyChannel
	if err := e.store.DB().Where("enabled = ?", true).Find(&channels).Error; err != nil {
		slog.Error("could not read the notification channels", "error", err)
		return
	}
	// A deployment without channels is the common case; nothing is built for it.
	if len(channels) == 0 {
		return
	}

	msg, ok := buildRunMessage(results)
	if !ok {
		return
	}
	for _, ch := range channels {
		if err := e.dispatch(ch, msg); err != nil {
			slog.Error("could not send the notification", "channel", ch.Name, "type", ch.Type, "error", err)
		}
	}
}

// buildRunMessage turns the rows a run wants to announce into one message.
//
// It answers false when no row asked to be announced, which is the ordinary
// case for a run where every record already pointed at the right address.
func buildRunMessage(results []*Result) (notifier.Message, bool) {
	var rows []*Result
	for _, r := range results {
		if r != nil && r.notify {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return notifier.Message{}, false
	}

	var changed, failed int
	lines := make([]string, 0, len(rows))
	records := make([]notifier.Record, 0, len(rows))
	for _, r := range rows {
		if r.Status == model.StatusFAIL {
			failed++
		} else {
			changed++
		}
		lines = append(lines, fmt.Sprintf("%s.%s (%s) %s", r.SubDomain, r.Domain, r.RecordType, r.Message))
		records = append(records, notifier.Record{
			SubDomain:  r.SubDomain,
			RecordType: r.RecordType,
			OldIP:      r.OldIP,
			NewIP:      r.NewIP,
			Status:     r.Status,
			Message:    r.Message,
		})
	}

	// A run that failed anywhere reads as a failure: it is the half someone has
	// to act on, and the per-row statuses say which rows those were.
	status := model.StatusOK
	if failed > 0 {
		status = model.StatusFAIL
	}

	title := "DDNS update succeeded"
	switch {
	case len(rows) == 1 && failed == 1:
		title = "DDNS update failed"
	case len(rows) > 1 && failed == 0:
		title = fmt.Sprintf("DDNS update: %d records updated", changed)
	case len(rows) > 1 && changed == 0:
		title = fmt.Sprintf("DDNS update: %d records failed", failed)
	case len(rows) > 1:
		title = fmt.Sprintf("DDNS update: %d updated, %d failed", changed, failed)
	}

	// The scalar fields mirror the first row so a template written for a single
	// record still renders something true; Records carries all of them.
	first := rows[0]
	return notifier.Message{
		Title:      title,
		Content:    strings.Join(lines, "\n"),
		Status:     status,
		Domain:     first.Domain,
		SubDomain:  first.SubDomain,
		RecordType: first.RecordType,
		OldIP:      first.OldIP,
		NewIP:      first.NewIP,
		Time:       time.Now().Format("2006-01-02 15:04:05"),
		Records:    records,
	}, true
}

// dispatch routes by channel type, reusing the shared notifier logic
func (e *Executor) dispatch(ch model.NotifyChannel, msg notifier.Message) error {
	return e.sender.Dispatch(ch.Type, ch.Config, msg)
}

// computeDuration returns how many seconds the old IP lived, based on the task IP-since timestamp.
func computeDuration(task *model.DDNSTask) *int64 {
	if task.IPSince == nil {
		return nil
	}
	sec := int64(time.Since(*task.IPSince).Seconds())
	return &sec
}
