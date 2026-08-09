package usecases

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// WorkerConfig tunes one generation pass, following the shared durable-worker
// shape (owner identity, lease, bounded batch, retry ceiling) already used by
// bulk imports and private execution.
type WorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	Backoff                time.Duration
	BatchSize, MaxAttempts int
}

// WorkerResult reports one pass's outcome for observability.
type WorkerResult struct {
	Claimed, Completed, Failed, DeadLettered int
}

// RunGenerationPass claims a bounded batch of runnable jobs and generates
// each one: read rows through the registered Exporter, render CSV, upload
// through Media, mark READY. A job whose type has no registered adapter, or
// whose Media call fails, is recorded as a failed attempt and either
// retried later or dead-lettered once it has exhausted its attempts —
// exactly the same shape bulk imports' worker already uses.
func (s *Service) RunGenerationPass(ctx context.Context, cfg WorkerConfig) (WorkerResult, error) {
	var result WorkerResult
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch {
		return result, nil
	}
	cfg = withDefaults(cfg)

	jobs, err := s.opts.Repository.Claim(ctx, cfg.Owner, cfg.Lease, cfg.Backoff, cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(jobs)

	for _, job := range jobs {
		outcome := s.generateOne(ctx, cfg, job)
		switch outcome {
		case outcomeCompleted:
			result.Completed++
		case outcomeDeadLettered:
			result.DeadLettered++
		default:
			result.Failed++
		}
	}
	return result, nil
}

type generationOutcome int

const (
	outcomeCompleted generationOutcome = iota
	outcomeFailed
	outcomeDeadLettered
)

func (s *Service) generateOne(ctx context.Context, cfg WorkerConfig, job models.Job) generationOutcome {
	t := tenant.Context{
		OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: "export-worker", CorrelationID: job.ID,
	}
	if job.BranchID != "" {
		t.PermittedBranchIDs = []string{job.BranchID}
	}

	exporter, registered := s.opts.Exporters[job.ExportType]
	if !registered {
		return s.failAttempt(ctx, cfg, job, "exporter_not_registered")
	}
	if s.opts.Media == nil {
		return s.failAttempt(ctx, cfg, job, "media_unavailable")
	}

	stopHeartbeat := s.startHeartbeat(ctx, cfg.Owner, job.ID, cfg.Lease)
	rows, err := exporter.Rows(ctx, t, job.FilterSnapshot, s.opts.MaxRows)
	stopHeartbeat()
	if err != nil {
		return s.failAttempt(ctx, cfg, job, "dataset_read_failed")
	}
	if len(rows) > s.opts.MaxRows {
		rows = rows[:s.opts.MaxRows]
	}

	content, err := buildCSV(exporter.Columns(), rows)
	if err != nil {
		return s.failAttempt(ctx, cfg, job, "render_failed")
	}

	stopHeartbeat = s.startHeartbeat(ctx, cfg.Owner, job.ID, cfg.Lease)
	mediaFileID, err := s.opts.Media.WriteExportFile(ctx, t, job.ID, content)
	stopHeartbeat()
	if err != nil {
		return s.failAttempt(ctx, cfg, job, "media_write_failed")
	}

	expiresAt := s.opts.Now().UTC().Add(s.opts.RetentionTTL)
	if err := s.opts.Repository.Complete(ctx, nil, job.ID, mediaFileID, len(rows), &expiresAt); err != nil {
		return s.failAttempt(ctx, cfg, job, "persist_failed")
	}
	_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "COMPLETED", "", job.Attempts)
	s.opts.Metrics.IncCounter("fleet_partner_exports_completed_total",
		map[string]string{"exportType": job.ExportType}, 1)
	return outcomeCompleted
}

func (s *Service) failAttempt(ctx context.Context, cfg WorkerConfig, job models.Job, code string) generationOutcome {
	_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", code, job.Attempts)
	_ = s.opts.Repository.Fail(ctx, cfg.Owner, job.ID, code, cfg.MaxAttempts)
	s.opts.Metrics.IncCounter("fleet_partner_exports_failed_total",
		map[string]string{"exportType": job.ExportType, "reason": code}, 1)
	if job.Attempts >= cfg.MaxAttempts {
		return outcomeDeadLettered
	}
	return outcomeFailed
}

// RunExpiryPass marks READY jobs whose expiry has passed EXPIRED, so a
// download attempt against an old export answers "expired" rather than
// silently serving a file the retention policy said should be gone.
func (s *Service) RunExpiryPass(ctx context.Context, batch int) (int64, error) {
	if s.opts.Flags.KillSwitch {
		return 0, nil
	}
	if batch <= 0 {
		batch = 200
	}
	return s.opts.Repository.ExpireReady(ctx, batch)
}

func withDefaults(cfg WorkerConfig) WorkerConfig {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 30 * time.Second
	}
	if strings.TrimSpace(cfg.Owner) == "" {
		cfg.Owner = "export-worker"
	}
	return cfg
}

// startHeartbeat extends a claimed job's lease periodically during a
// long-running step, mirroring bulk imports' heartbeat helper exactly.
func (s *Service) startHeartbeat(ctx context.Context, owner, id string, lease time.Duration) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	interval := lease / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.opts.Repository.Heartbeat(ctx, owner, id, lease)
			}
		}
	}()
	return func() { close(stop); <-done }
}
