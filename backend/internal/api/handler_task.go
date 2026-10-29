package api

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/dnspod"
	"ddns-by-dnspod/internal/model"
)

// taskInput is the payload for creating or updating a task.
// Enabled and NotifyOnChange are pointers to tell "not sent" apart from an explicit false.
type taskInput struct {
	AccountID      uint   `json:"account_id"`
	Domain         string `json:"domain"`
	SubDomain      string `json:"sub_domain"`
	RecordType     string `json:"record_type"`
	RecordLine     string `json:"record_line"`
	TTL            int    `json:"ttl"`
	Enabled        *bool  `json:"enabled"`
	CronExpr       string `json:"cron_expr"`
	NotifyOnChange *bool  `json:"notify_on_change"`
}

// importInput is the payload for importing tasks from a record group
type importInput struct {
	GroupID    uint64 `json:"group_id"`
	AccountID  uint   `json:"account_id"`
	RecordType string `json:"record_type"`
	CronExpr   string `json:"cron_expr"`
	Enabled    *bool  `json:"enabled"`
}

// importResult reports what one import did
type importResult struct {
	Created int    `json:"created"`
	Skipped int    `json:"skipped"`
	TaskIDs []uint `json:"task_ids"`
}

// importTasksFromGroup turns every address record of one group into a DDNS task.
//
// Only A and AAAA records can be kept up to date, so other types are ignored.
// A host that already has a task is skipped rather than duplicated, which makes
// a repeated import a no-op instead of a pile of duplicate tasks.
//
// Imported tasks are disabled by default: the point is to let the operator look
// at what would be managed, run one by hand and confirm it reports SKIP, before
// handing the records over.
func (s *Server) importTasksFromGroup(c *gin.Context) {
	domain := c.Param("domain")
	if err := model.ValidateDomain(domain); err != nil {
		badRequest(c, err.Error())
		return
	}

	var input importInput
	if !bindJSON(c, &input) {
		return
	}

	recordType := input.RecordType
	if recordType == "" {
		recordType = "A"
	}
	if recordType != "A" && recordType != "AAAA" {
		badRequest(c, "record_type must be A or AAAA")
		return
	}

	accountID, valid := parseAccountID(c)
	if !valid {
		return
	}
	account, err := s.resolveAccount(accountID)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	// parseAccountID already pins the account to the key that authenticated the
	// call, so resolveAccount cannot hand back a different one
	cli, err := s.clientFor(account.ID)
	if err != nil {
		credentialError(c, err)
		return
	}

	page, err := cli.ListRecords(c.Request.Context(), domain, dnspod.RecordFilter{
		RecordType: recordType,
		GroupID:    input.GroupID,
	})
	if err != nil {
		upstreamError(c, err)
		return
	}

	// Existing tasks, keyed by host + type, so the import stays idempotent
	var existing []model.DDNSTask
	if err := s.store.DB().Where("domain = ? AND record_type = ?", domain, recordType).
		Find(&existing).Error; err != nil {
		serverError(c, err)
		return
	}
	seen := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		seen[t.SubDomain+"|"+t.RecordType] = struct{}{}
	}

	cronExpr := input.CronExpr
	if cronExpr == "" {
		cronExpr = defaultCronExpr("")
	}
	if _, err := cron.ParseStandard(cronExpr); err != nil {
		badRequest(c, "invalid cron expression")
		return
	}

	// The whole group is capped like every other batch call: each task is a row,
	// and an unbounded listing would turn one request into minutes of writes.
	// The caller cannot know this size in advance - the group listing carries no
	// record count - so the refusal is the only place it can be said, and it says
	// what to do about it.
	if !batchSizeOK(c, len(page.Records), "narrow the group") {
		return
	}

	enabled := false
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	// One transaction for the group: a failure halfway through must not leave a
	// half-imported group behind, where repeating the import skips whatever made
	// it in and offers no way to finish.
	result := importResult{TaskIDs: []uint{}}
	if err := s.store.DB().Transaction(func(tx *gorm.DB) error {
		for _, r := range page.Records {
			if r.RecordType != recordType {
				continue
			}
			if _, dup := seen[r.SubDomain+"|"+r.RecordType]; dup {
				result.Skipped++
				continue
			}

			subDomain := normalizeSubDomain(r.SubDomain)
			// Values read back from DNSPod are held to the same rule as typed
			// ones: they reach the change history and the log, which is exactly
			// what the restriction exists to protect.
			if err := model.ValidateSubDomain(subDomain); err != nil {
				result.Skipped++
				continue
			}

			task := model.DDNSTask{
				AccountID:      account.ID,
				Domain:         domain,
				SubDomain:      subDomain,
				RecordType:     r.RecordType,
				RecordLine:     defaultLine(r.Line),
				TTL:            int(r.TTL),
				Enabled:        enabled,
				CronExpr:       cronExpr,
				CurrentIP:      r.Value,
				NotifyOnChange: true,
			}
			if task.TTL <= 0 {
				task.TTL = 600
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
			seen[r.SubDomain+"|"+r.RecordType] = struct{}{}
			result.Created++
			result.TaskIDs = append(result.TaskIDs, task.ID)
		}
		return nil
	}); err != nil {
		serverError(c, err)
		return
	}

	if err := s.scheduler.Reload(); err != nil {
		slog.Error("could not reload the scheduler after the import", "rid", middleware.RequestIDFrom(c), "error", err)
	}

	slog.Info("tasks imported from a group", "domain", domain,
		"created", result.Created, "skipped", result.Skipped,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, result)
}

// listTasks lists tasks together with their account info.
//
// status narrows the list to one outcome, which is how the dashboard counts the
// failing ones without reading every task that exists.
func (s *Server) listTasks(c *gin.Context) {
	params := parsePageParams(c)

	q := s.store.DB().Model(&model.DDNSTask{})
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("last_status = ?", strings.ToUpper(status))
	}

	var total int64
	// Counted on a session of its own: the chain is reused for the page below,
	// and a query built on the same statement would carry the count's state over.
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}

	var tasks []model.DDNSTask
	err := q.Preload("Account").Order("id").
		Offset(params.offset()).
		Limit(params.PageSize).
		Find(&tasks).Error
	if err != nil {
		serverError(c, err)
		return
	}
	paged(c, tasks, total, params)
}

// getTask returns a single task
func (s *Server) getTask(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}
	var task model.DDNSTask
	if err := s.store.DB().Preload("Account").First(&task, id).Error; err != nil {
		notFound(c, "task not found")
		return
	}
	ok(c, task)
}

// createTask creates a task
func (s *Server) createTask(c *gin.Context) {
	var input taskInput
	if !bindJSON(c, &input) {
		return
	}
	if input.Domain == "" {
		badRequest(c, "domain is required")
		return
	}
	if err := model.ValidateDomain(input.Domain); err != nil {
		badRequest(c, err.Error())
		return
	}
	// Normalised first: an empty host means the apex, which is exactly what the
	// form offers, and "@" passes the validator - whose real job is keeping a
	// name that could forge a log line out of the log and the history.
	if err := model.ValidateSubDomain(normalizeSubDomain(input.SubDomain)); err != nil {
		badRequest(c, err.Error())
		return
	}
	// Refused at creation rather than at run time: the address family is derived
	// from the record type, so any other type would end up with an IP written
	// into it by the executor
	recordType, err := normalizeRecordType(input.RecordType)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	// Fall back to the default account when none is given
	account, err := s.resolveAccount(input.AccountID)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	notifyOnChange := true
	if input.NotifyOnChange != nil {
		notifyOnChange = *input.NotifyOnChange
	}

	task := model.DDNSTask{
		AccountID:      account.ID,
		Domain:         input.Domain,
		SubDomain:      normalizeSubDomain(input.SubDomain),
		RecordType:     recordType,
		RecordLine:     defaultLine(input.RecordLine),
		TTL:            input.TTL,
		Enabled:        enabled,
		CronExpr:       defaultCronExpr(input.CronExpr),
		NotifyOnChange: notifyOnChange,
	}
	// Same rule as the import path and the update below: a TTL that is not a
	// positive number means "not specified", never a value to hand DNSPod.
	if task.TTL <= 0 {
		task.TTL = 600
	}
	// Refused here rather than at reload time: an expression the scheduler cannot
	// parse would otherwise be accepted and then silently never run.
	if _, err := cron.ParseStandard(task.CronExpr); err != nil {
		badRequest(c, "invalid cron expression")
		return
	}

	if err := s.store.DB().Create(&task).Error; err != nil {
		serverError(c, err)
		return
	}

	if err := s.scheduler.Reload(); err != nil {
		slog.Error("could not reload the scheduler after creating the task", "rid", middleware.RequestIDFrom(c), "error", err)
	}

	slog.Info("task created", "task", task.ID, "name", task.SubDomain+"."+task.Domain,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	created(c, task)
}

// updateTask updates a task
func (s *Server) updateTask(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var task model.DDNSTask
	if err := s.store.DB().First(&task, id).Error; err != nil {
		notFound(c, "task not found")
		return
	}

	var input taskInput
	if !bindJSON(c, &input) {
		return
	}

	// Moving a task to another account is checked here like it is on create:
	// writing the id straight through would leave the task pointing at a row
	// that does not exist, which the scheduler then fails forever.
	if input.AccountID > 0 && input.AccountID != task.AccountID {
		if err := s.store.DB().First(&model.Account{}, input.AccountID).Error; err != nil {
			badRequest(c, "account not found")
			return
		}
		task.AccountID = input.AccountID
	}
	if input.Domain != "" {
		if err := model.ValidateDomain(input.Domain); err != nil {
			badRequest(c, err.Error())
			return
		}
		task.Domain = input.Domain
	}
	if input.SubDomain != "" {
		if err := model.ValidateSubDomain(input.SubDomain); err != nil {
			badRequest(c, err.Error())
			return
		}
		task.SubDomain = normalizeSubDomain(input.SubDomain)
	}
	if input.RecordType != "" {
		recordType, err := normalizeRecordType(input.RecordType)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		task.RecordType = recordType
	}
	if input.RecordLine != "" {
		task.RecordLine = input.RecordLine
	}
	if input.TTL > 0 {
		task.TTL = input.TTL
	}
	if input.Enabled != nil {
		task.Enabled = *input.Enabled
	}
	if input.CronExpr != "" {
		task.CronExpr = input.CronExpr
	}
	if input.NotifyOnChange != nil {
		task.NotifyOnChange = *input.NotifyOnChange
	}
	// Refused here rather than at reload time, same reasoning as on create
	if _, err := cron.ParseStandard(task.CronExpr); err != nil {
		badRequest(c, "invalid cron expression")
		return
	}

	// Only user-editable columns are written. The scheduler and executor concurrently update
	// runtime fields such as last_run_at / fail_count / current_ip, which a full Save would overwrite.
	if err := s.store.DB().Model(&model.DDNSTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"account_id":       task.AccountID,
		"domain":           task.Domain,
		"sub_domain":       task.SubDomain,
		"record_type":      task.RecordType,
		"record_line":      task.RecordLine,
		"ttl":              task.TTL,
		"enabled":          task.Enabled,
		"cron_expr":        task.CronExpr,
		"notify_on_change": task.NotifyOnChange,
	}).Error; err != nil {
		serverError(c, err)
		return
	}

	if err := s.scheduler.Reload(); err != nil {
		slog.Error("could not reload the scheduler after updating the task", "rid", middleware.RequestIDFrom(c), "error", err)
	}

	// Read the row back: the runtime columns are written by the scheduler and the
	// executor in parallel, so echoing the struct we started from reports stale values.
	var updated model.DDNSTask
	if err := s.store.DB().Preload("Account").First(&updated, task.ID).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, updated)
}

// deleteTask deletes a task
func (s *Server) deleteTask(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	tx := s.store.DB().Delete(&model.DDNSTask{}, id)
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	// GORM reports no error when the row is already gone, so the affected row
	// count is what tells us whether the task ever existed
	if tx.RowsAffected == 0 {
		notFound(c, "task not found")
		return
	}

	if err := s.scheduler.Reload(); err != nil {
		slog.Error("could not reload the scheduler after deleting the task", "rid", middleware.RequestIDFrom(c), "error", err)
	}

	slog.Info("task deleted", "task", id,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	ok(c, gin.H{"deleted": id})
}

// runTask triggers a task manually and returns the result synchronously
func (s *Server) runTask(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	slog.Info("task triggered by hand", "task", id,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))

	result, err := s.scheduler.RunNow(id)
	if err != nil {
		// A deleted task between the list and the click is a missing resource,
		// not a malformed request
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notFound(c, "task not found")
			return
		}
		slog.Error("manual run failed", "task", id, "rid", middleware.RequestIDFrom(c), "error", err)
		serverError(c, err)
		return
	}
	ok(c, result)
}

// reloadScheduler reloads the schedule
func (s *Server) reloadScheduler(c *gin.Context) {
	if err := s.scheduler.Reload(); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"entries": s.scheduler.EntryCount()})
}

func normalizeSubDomain(sub string) string {
	if sub == "" {
		return "@"
	}
	return sub
}

// normalizeRecordType accepts only the record types a DDNS task can keep up to
// date, upper cased so the value matches what DNSPod expects. An empty value
// falls back to A.
func normalizeRecordType(recordType string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(recordType))
	switch normalized {
	case "", "A":
		return "A", nil
	case "AAAA":
		return "AAAA", nil
	default:
		return "", fmt.Errorf("record_type must be A or AAAA, got %s", recordType)
	}
}

func defaultCronExpr(expr string) string {
	if expr == "" {
		return "*/5 * * * *"
	}
	return expr
}
