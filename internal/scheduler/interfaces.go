// Package scheduler provides cron job management for proactive tasks.
package scheduler

import (
	"context"
	"time"
)

// Scheduler manages scheduled jobs and proactive tasks.
type Scheduler interface {
	// Schedule adds a new job with cron expression
	Schedule(job Job) error

	// Remove removes a scheduled job
	Remove(jobID string) error

	// Start begins running scheduled jobs
	Start(ctx context.Context) error

	// Stop gracefully stops all jobs
	Stop() error

	// List returns all scheduled jobs
	List() []Job
}

// Job represents a scheduled task.
type Job struct {
	LastRun  time.Time
	NextRun  time.Time
	Handler  JobHandler
	ID       string
	Name     string
	CronExpr string
	Enabled  bool
}

// JobHandler executes a scheduled job.
type JobHandler interface {
	// Execute runs the job
	Execute(ctx context.Context) error

	// Name returns the job name for logging
	Name() string
}
