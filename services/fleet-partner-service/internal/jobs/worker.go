package jobs

import (
	"context"
	"errors"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tracing"
)

// Handler executes one job type. Implementations must be idempotent: a job
// may run more than once after lease recovery.
type Handler func(ctx context.Context, job Job) error

// ErrShutdownSafeToRelease is the sentinel a handler returns when it
// observed context cancellation (graceful shutdown) before performing any
// externally-visible, not-safely-repeatable effect. The worker treats this
// distinctly from an ordinary failure: the job is released back to PENDING
// immediately, with no backoff and no consumed attempt toward max_attempts,
// because nothing about the job itself went wrong -- only the process
// hosting it stopped before starting the work.
//
// A handler that has already made an external call whose outcome is not yet
// confirmed must NOT return this sentinel -- it must return a
// *ClassifiedError with ErrorClassUnknownExternalOutcome instead, so the job
// is preserved for authoritative read-back rather than blindly released for
// another worker to potentially repeat the same external effect.
var ErrShutdownSafeToRelease = errors.New("jobs: handler observed shutdown before any external effect")

// Worker claims and executes durable jobs.
type Worker struct {
	Queue             *Queue
	Metrics           *observability.Metrics
	Owner             string
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	BatchSize         int
	// Environment labels every structured log line this worker emits
	// (cfg.AppEnv). Empty is valid (logs simply carry an empty environment
	// field) so existing test call sites that build a Worker{} literal
	// without it keep compiling and running unchanged.
	Environment string

	mu       sync.RWMutex
	handlers map[string]Handler
	ready    bool
}

// Register binds a handler to a job type. Must be called before Run.
func (w *Worker) Register(jobType string, handler Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.handlers == nil {
		w.handlers = map[string]Handler{}
	}
	w.handlers[jobType] = handler
}

// Ready reports whether the worker loop is polling (for readiness checks).
func (w *Worker) Ready() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

// Run polls until ctx is cancelled. During shutdown it stops claiming new
// jobs; in-flight jobs finish under their lease or are recovered by another
// worker after lease expiry.
func (w *Worker) Run(ctx context.Context) {
	if w.BatchSize <= 0 {
		w.BatchSize = 10
	}
	w.mu.Lock()
	w.ready = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.ready = false
		w.mu.Unlock()
	}()

	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if counts, err := w.Queue.Counts(ctx); err == nil {
		for status, count := range counts {
			w.Metrics.SetGauge("fleet_partner_jobs", map[string]string{"status": status}, float64(count))
		}
	}

	claimed, err := w.Queue.ClaimBatch(ctx, w.Owner, w.LeaseDuration, w.BatchSize)
	if err != nil {
		w.Metrics.IncCounter("fleet_partner_job_claim_errors_total", nil, 1)
		return
	}
	w.Metrics.IncCounter("fleet_partner_job_claims_total", nil, float64(len(claimed)))

	for _, job := range claimed {
		w.execute(ctx, job)
	}
}

// logJob emits one structured job-lifecycle log line. Payloads are never
// included — only identity, attempt and lease fields, matching the same
// no-raw-payload rule this service already applies to event logging.
func (w *Worker) logJob(job Job, event string, level string, extra map[string]any) {
	fields := map[string]any{
		"jobId":         job.ID,
		"jobType":       job.JobType,
		"attempt":       job.Attempts,
		"leaseOwner":    w.Owner,
		"correlationId": job.CorrelationID,
	}
	if job.OrganizationID != "" {
		fields["organizationId"] = job.OrganizationID
	}
	for k, v := range extra {
		fields[k] = v
	}
	observability.Log(config.ServiceName, w.Environment, level, event, fields)
}

func (w *Worker) execute(ctx context.Context, job Job) {
	started := time.Now().UTC()
	// Async spans link back to the request/workflow that enqueued this job
	// via job.CorrelationID (see internal/platform/tracing's own doc comment
	// on why an async boundary links by correlationId rather than a
	// persisted trace-id column) rather than a literal W3C parent trace.
	spanCtx, endSpan := tracing.StartSpan(ctx, w.Metrics, config.ServiceName, w.Environment,
		"job."+job.JobType, map[string]any{"jobId": job.ID, "correlationId": job.CorrelationID, "attempt": job.Attempts})
	ctx = spanCtx
	var handlerErr error
	defer func() { endSpan(handlerErr) }()

	w.logJob(job, "job_started", "info", nil)

	// Revalidate tenant scope against the durable row before running.
	if err := ValidateTenant(job); err != nil {
		w.Metrics.IncCounter("fleet_partner_job_tenant_mismatch_total", nil, 1)
		w.logJob(job, "job_tenant_mismatch", "error", map[string]any{"safeErrorClass": "security"})
		if recordErr := w.Queue.RecordAttempt(ctx, job, w.Owner, StatusDeadLettered, started, err); recordErr != nil {
			return
		}
		_ = w.Queue.DeadLetter(ctx, w.Owner, job.ID, "security: "+err.Error())
		return
	}

	w.mu.RLock()
	handler, ok := w.handlers[job.JobType]
	w.mu.RUnlock()
	if !ok {
		err := errors.New("no handler registered for job type " + job.JobType)
		status := StatusFailed
		if job.Attempts >= job.MaxAttempts {
			status = StatusDeadLettered
		}
		w.logJob(job, "job_no_handler", "error", map[string]any{"safeErrorClass": "configuration", "status": status})
		if recordErr := w.Queue.RecordAttempt(ctx, job, w.Owner, status, started, err); recordErr != nil {
			return
		}
		_ = w.Queue.Fail(ctx, w.Owner, job.ID, err)
		w.Metrics.IncCounter("fleet_partner_job_retries_total", nil, 1)
		return
	}

	if err := w.Queue.MarkRunning(ctx, w.Owner, job.ID); err != nil {
		return // lease lost; another worker owns it now
	}

	// Owner-only heartbeat while the handler runs.
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	var heartbeatDone sync.WaitGroup
	heartbeatDone.Add(1)
	go func() {
		defer heartbeatDone.Done()
		ticker := time.NewTicker(w.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := w.Queue.Heartbeat(heartbeatCtx, w.Owner, job.ID, w.LeaseDuration); err != nil {
					return
				}
			}
		}
	}()

	err := handler(ctx, job)
	handlerErr = err
	stopHeartbeat()
	heartbeatDone.Wait()

	durationMs := time.Since(started).Milliseconds()

	if errors.Is(err, ErrShutdownSafeToRelease) {
		// Use a background context: ctx is already cancelled at this point,
		// and releasing the lease so another worker can pick the job back up
		// is exactly the cleanup shutdown must still be allowed to do.
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.logJob(job, "job_shutdown_released", "info", map[string]any{"durationMs": durationMs})
		if recordErr := w.Queue.RecordAttempt(releaseCtx, job, w.Owner, "SHUTDOWN_RELEASED", started, err); recordErr != nil {
			return
		}
		if releaseErr := w.Queue.ReleaseForShutdown(releaseCtx, w.Owner, job.ID); releaseErr != nil {
			w.Metrics.IncCounter("fleet_partner_job_lease_lost_total", nil, 1)
		}
		w.Metrics.IncCounter("fleet_partner_job_shutdown_released_total", nil, 1)
		return
	}

	if err != nil {
		w.Metrics.IncCounter("fleet_partner_job_retries_total", nil, 1)
		status := StatusFailed
		if job.Attempts >= job.MaxAttempts {
			status = StatusDeadLettered
		}
		w.logJob(job, "job_failed", "error", map[string]any{
			"durationMs": durationMs, "status": status, "safeErrorClass": "job_handler_error",
		})
		if recordErr := w.Queue.RecordAttempt(ctx, job, w.Owner, status, started, err); recordErr != nil {
			return
		}
		_ = w.Queue.Fail(ctx, w.Owner, job.ID, err)
		return
	}
	w.logJob(job, "job_succeeded", "info", map[string]any{"durationMs": durationMs})
	if err := w.Queue.RecordAttempt(ctx, job, w.Owner, StatusSucceeded, started, nil); err != nil {
		return
	}
	if err := w.Queue.Complete(ctx, w.Owner, job.ID); err != nil {
		w.Metrics.IncCounter("fleet_partner_job_lease_lost_total", nil, 1)
		return
	}
	w.Metrics.IncCounter("fleet_partner_job_succeeded_total", nil, 1)
}
