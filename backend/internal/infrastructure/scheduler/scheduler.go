package scheduler

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// Scheduler wraps robfig/cron and owns all registered periodic tasks.
type Scheduler struct {
	cron   *cron.Cron
	report *DailyReportTask
	log    *logger.Logger
}

// NewScheduler creates and configures the scheduler.
// Call Start() to begin scheduling.
func NewScheduler(report *DailyReportTask, log *logger.Logger) *Scheduler {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback: UTC+8 fixed offset
		loc = time.FixedZone("CST", 8*60*60)
	}

	c := cron.New(
		cron.WithSeconds(),
		cron.WithLocation(loc),
	)

	return &Scheduler{cron: c, report: report, log: log}
}

// Start registers all tasks and starts the cron runner.
func (s *Scheduler) Start() {
	// Fire every day at 14:40:00 CST.
	// Format: sec min hour day-of-month month day-of-week
	if _, err := s.cron.AddFunc("0 40 14 * * *", func() {
		s.log.Info("Scheduler: triggering daily market report...")
		s.report.Run()
	}); err != nil {
		s.log.Errorf("Scheduler: failed to register daily report task: %v", err)
		return
	}

	s.cron.Start()
	s.log.Info("Scheduler started — daily report fires at 14:40 CST")
}

// Stop gracefully stops the scheduler, waiting for running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.log.Info("Scheduler stopped")
}
