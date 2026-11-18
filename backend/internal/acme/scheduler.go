package acme

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// catchUpDelay is how long after start the one-off check runs.
//
// A delay rather than an immediate call so that a container crash-looping on a
// bad configuration does not ask the CA for a certificate on every attempt: the
// process has to survive a minute before it is allowed to.
const catchUpDelay = time.Minute

// Scheduler runs the renewal check.
//
// One shared tick every minute rather than one entry per certificate: the rows
// carry their own expressions, and each check looks at the ones whose schedule
// fires now. Creating, editing or deleting a certificate therefore never has to
// touch the schedule, which is what a per-row cron entry would have to do.
type Scheduler struct {
	manager *Manager
	cron    *cron.Cron
	stop    chan struct{}
}

// NewScheduler builds the renewal check.
func NewScheduler(manager *Manager) *Scheduler {
	return &Scheduler{
		manager: manager,
		// Recover so one panic does not take the schedule down with it, the way
		// the DDNS scheduler is built.
		cron: cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger))),
		stop: make(chan struct{}),
	}
}

// Start adds the tick and arranges the one-off check after boot.
//
// The tick alone is not enough: a schedule measured in days can fall inside a
// restart, and a certificate that came due in that window would wait for the
// next tick - a month, with the default - while a check at start catches it.
func (s *Scheduler) Start() error {
	if !s.manager.Enabled() {
		// Nothing issues while this is off, the button included: the API
		// answers 503 for it, so saying "use the button" here would send
		// whoever reads the log to a refusal.
		slog.Info("certificate manager disabled: no renewal check, and issuing by hand is refused too")
		return nil
	}

	if _, err := s.cron.AddFunc("* * * * *", s.check); err != nil {
		return fmt.Errorf("schedule the renewal check: %w", err)
	}
	s.cron.Start()
	slog.Info("renewal check scheduled", "interval", "minute")

	go func() {
		select {
		case <-time.After(catchUpDelay):
			s.catchUp()
		case <-s.stop:
		}
	}()

	return nil
}

// Stop shuts the schedule down.
func (s *Scheduler) Stop() {
	close(s.stop)
	<-s.cron.Stop().Done()
	slog.Info("renewal check stopped")
}

// check starts the issuance of every certificate whose schedule is due now.
func (s *Scheduler) check() {
	started, err := s.manager.RenewDue()
	if err != nil {
		slog.Error("renewal check failed", "error", err)
		return
	}
	if started > 0 {
		slog.Info("renewal check started issuances", "count", started)
	}
}

// catchUp runs the check once at start, ignoring each certificate's schedule:
// what matters here is whether anything is inside its renewal window after the
// process was away, not whether this happens to be its minute.
func (s *Scheduler) catchUp() {
	started, err := s.manager.RenewDueNow()
	if err != nil {
		slog.Error("catch-up renewal check failed", "error", err)
		return
	}
	if started > 0 {
		slog.Info("catch-up check started issuances", "count", started)
	}
}
