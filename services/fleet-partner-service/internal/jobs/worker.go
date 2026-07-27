package jobs

import (
	"context"
	"errors"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// Handler executes one job type. Implementations must be idempotent: a job
// may run more than once after lease recovery.
type Handler func(ctx context.Context, job Job) error

// Worker claims and executes durable jobs.
type Worker struct {
	Queue             *Queue
	Metrics           *observability.Metrics
	Owner             string
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	BatchSize         int

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

func (w *Worker) execute(ctx context.Context, job Job) {
	// Revalidate tenant scope against the durable row before running.
	if err := ValidateTenant(job); err != nil {
		w.Metrics.IncCounter("fleet_partner_job_tenant_mismatch_total", nil, 1)
		_ = w.Queue.DeadLetter(ctx, w.Owner, job.ID, "security: "+err.Error())
		return
	}

	w.mu.RLock()
	handler, ok := w.handlers[job.JobType]
	w.mu.RUnlock()
	if !ok {
		_ = w.Queue.Fail(ctx, w.Owner, job.ID, errors.New("no handler registered for job type "+job.JobType))
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
	stopHeartbeat()
	heartbeatDone.Wait()

	if err != nil {
		w.Metrics.IncCounter("fleet_partner_job_retries_total", nil, 1)
		_ = w.Queue.Fail(ctx, w.Owner, job.ID, err)
		return
	}
	if err := w.Queue.Complete(ctx, w.Owner, job.ID); err != nil {
		w.Metrics.IncCounter("fleet_partner_job_lease_lost_total", nil, 1)
		return
	}
	w.Metrics.IncCounter("fleet_partner_job_succeeded_total", nil, 1)
}
