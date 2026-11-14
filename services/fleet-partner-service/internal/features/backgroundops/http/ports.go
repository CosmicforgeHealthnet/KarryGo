package http

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// The worker-operations handlers depend on these interfaces rather than the
// concrete queue, scheduler and outbox stores (FLEET-5.1a). jobs.Queue,
// scheduler.Repository and events.OutboxRepository satisfy them.

// JobQueue reads and steers background jobs.
type JobQueue interface {
	Cancel(ctx context.Context, organizationID string, jobID string) error
	Counts(ctx context.Context) (map[string]int64, error)
	DeadLetterCount(ctx context.Context) (int64, error)
	Get(ctx context.Context, organizationID string, jobID string) (jobs.Job, error)
	List(ctx context.Context, organizationID string, filter jobs.ListFilter) ([]jobs.Job, error)
	ListAttempts(ctx context.Context, organizationID string, jobID string) ([]jobs.Attempt, error)
	Replay(ctx context.Context, organizationID string, jobID string) error
}

// ScheduledTasks reads and steers scheduled tasks.
type ScheduledTasks interface {
	Get(ctx context.Context, organizationID string, taskID string) (scheduler.Task, error)
	List(ctx context.Context, organizationID string, limit int) ([]scheduler.Task, error)
	ListRuns(ctx context.Context, organizationID string, taskID string, limit int) ([]scheduler.RunRecord, error)
	RunNow(ctx context.Context, organizationID string, taskID string, expectedVersion int64, actor string) (scheduler.Task, error)
	SetEnabled(ctx context.Context, organizationID string, taskID string, enabled bool, expectedVersion int64, actor string) (scheduler.Task, error)
}

// Outbox reports and replays the domain event outbox.
type Outbox interface {
	OldestPendingAge(ctx context.Context) (time.Duration, error)
	PendingCount(ctx context.Context) (int64, error)
	ReplayDeadLettered(ctx context.Context, id string) error
}
