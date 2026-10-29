package acme

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// catchUpDelay is how long after start the first renewal check runs.
//
// It is a delay rather than an immediate call so that a container crash-looping
// on a bad configuration does not ask the CA for a certificate on every attempt:
// the process has to survive a minute before it is allowed to.
const catchUpDelay = time.Minute

// Scheduler runs the daily renewal check.
type Scheduler struct {
	manager *Manager
	hour    int
	cron    *cron.Cron
	stop    chan struct{}
}

// NewScheduler builds the renewal check for the configured hour.
func NewScheduler(manager *Manager) *Scheduler {
	return &Scheduler{
		manager: manager,
		hour:    manager.Config().CheckHour,
		// Recover so one panic does not take the schedule down with it, the way
		// the DDNS scheduler is built.
		cron: cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger))),
		stop: make(chan struct{}),
	}
}

// Start adds the daily entry and runs one check shortly after boot.
//
// The catch-up check is what makes a restart at the wrong moment harmless: the
// container may well have been down at the configured hour, and a certificate
// that expired during an outage is exactly what this is here to prevent.
func (s *Scheduler) Start() error {
	if !s.manager.Enabled() {
		// Nothing issues while this is off, the button included: the API
		// answers 503 for it, so saying "use the button" here would send
		// whoever reads the log to a refusal.
		slog.Info("certificate manager disabled: no renewal check, and issuing by hand is refused too")
		return nil
	}

	if _, err := s.cron.AddFunc(fmt.Sprintf("0 %d * * *", s.hour), s.check); err != nil {
		return fmt.Errorf("schedule the renewal check: %w", err)
	}
	s.cron.Start()
	slog.Info("renewal check scheduled", "hour", s.hour)

	go func() {
		select {
		case <-time.After(catchUpDelay):
			s.check()
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

// check starts the issuance of every certificate that is due.
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
