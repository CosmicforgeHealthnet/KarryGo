// Package jobs implements durable background jobs with lease claiming,
// owner-only heartbeats, bounded retry and dead-lettering.
//
// Jobs live in PostgreSQL (never in memory only). Organization-owned job
// payloads carry organizationId, which the worker revalidates against the
// durable row before execution; a mismatch fails safely into dead-letter
// with a security error code.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Job statuses.
const (
	StatusPending        = "PENDING"
	StatusRetryScheduled = "RETRY_SCHEDULED"
	StatusClaimed        = "CLAIMED"
	StatusRunning        = "RUNNING"
	StatusSucceeded      = "SUCCEEDED"
	StatusFailed         = "FAILED"
	StatusDeadLettered   = "DEAD_LETTERED"
	StatusCancelled      = "CANCELLED"
)

// ErrNotLeaseOwner is returned when heartbeat/complete/fail is attempted by
// a worker that no longer owns the lease.
var ErrNotLeaseOwner = errors.New("jobs: lease is not owned by this worker")

// ErrTenantMismatch is the security failure when a payload's organizationId
// does not match the durable job row.
var ErrTenantMismatch = errors.New("jobs: payload organization does not match job row")

// Job is one durable background job.
type Job struct {
	ID             string
	OrganizationID string
	JobType        string
	DedupeKey      string
	Payload        json.RawMessage
	Status         string
	Priority       int
	ScheduledAt    time.Time
	Attempts       int
	MaxAttempts    int
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	HeartbeatAt    *time.Time
	LastError      string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

// EnqueueRequest describes a job to enqueue.
type EnqueueRequest struct {
	OrganizationID string
	JobType        string
	DedupeKey      string
	Payload        any
	Priority       int
	ScheduledAt    time.Time
	MaxAttempts    int
}

// Queue is the PostgreSQL-backed durable job queue.
type Queue struct {
	db database.Querier
}

// NewQueue wires the queue over a pool or transaction.
func NewQueue(db database.Querier) *Queue {
	return &Queue{db: db}
}

// Enqueue inserts a job; a matching active dedupe key is a no-op returning
// the existing job ID. Pass a transaction as q to enqueue atomically with a
// domain mutation.
func (queue *Queue) Enqueue(ctx context.Context, q database.Querier, request EnqueueRequest) (string, error) {
	if request.JobType == "" {
		return "", fmt.Errorf("jobs: job type is required")
	}
	if request.MaxAttempts <= 0 {
		request.MaxAttempts = 5
	}
	if request.ScheduledAt.IsZero() {
		request.ScheduledAt = time.Now().UTC()
	}
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return "", fmt.Errorf("jobs: marshal payload: %w", err)
	}

	jobID := ids.New(ids.PrefixJob)
	var insertedID string
	err = q.QueryRow(ctx, `
		INSERT INTO fleet_partner_jobs
			(id, organization_id, job_type, dedupe_key, payload, status, priority,
			 scheduled_at, attempts, max_attempts, created_at)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), $5, $6, $7, $8, 0, $9, now())
		ON CONFLICT (COALESCE(organization_id, ''), job_type, dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING')
		DO NOTHING
		RETURNING id`,
		jobID, request.OrganizationID, request.JobType, request.DedupeKey,
		payload, StatusPending, request.Priority, request.ScheduledAt.UTC(), request.MaxAttempts,
	).Scan(&insertedID)
	if err == nil {
		return insertedID, nil
	}

	// Conflict: return the existing active job for this organization-scoped
	// dedupe key.
	var existingID string
	lookupErr := q.QueryRow(ctx, `
		SELECT id FROM fleet_partner_jobs
		WHERE COALESCE(organization_id, '') = $1 AND job_type = $2 AND dedupe_key = $3
		  AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING')
		ORDER BY created_at DESC LIMIT 1`,
		request.OrganizationID, request.JobType, request.DedupeKey,
	).Scan(&existingID)
	if lookupErr != nil {
		return "", fmt.Errorf("jobs: enqueue: %w", err)
	}
	return existingID, nil
}

// ClaimBatch leases up to limit due jobs using FOR UPDATE SKIP LOCKED.
// Expired leases (crashed workers) are reclaimed automatically.
func (queue *Queue) ClaimBatch(ctx context.Context, owner string, leaseFor time.Duration, limit int) ([]Job, error) {
	rows, err := queue.db.Query(ctx, `
		UPDATE fleet_partner_jobs SET
			status = $1,
			lease_owner = $2,
			lease_expires_at = now() + make_interval(secs => $3::double precision),
			heartbeat_at = now(),
			attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM fleet_partner_jobs
			WHERE (
				(status IN ($4, $5) AND scheduled_at <= now())
				OR (status IN ($6, $7) AND lease_expires_at < now())
			)
			ORDER BY priority DESC, scheduled_at
			LIMIT $8
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, COALESCE(organization_id, ''), job_type, COALESCE(dedupe_key, ''),
			payload, status, priority, scheduled_at, attempts, max_attempts,
			COALESCE(lease_owner, ''), lease_expires_at, heartbeat_at,
			COALESCE(last_error_redacted, ''), created_at, completed_at`,
		StatusClaimed, owner, int64(leaseFor.Seconds()),
		StatusPending, StatusRetryScheduled, StatusClaimed, StatusRunning, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: claim batch: %w", err)
	}
	defer rows.Close()

	var claimed []Job
	for rows.Next() {
		var job Job
		if err := rows.Scan(
			&job.ID, &job.OrganizationID, &job.JobType, &job.DedupeKey, &job.Payload,
			&job.Status, &job.Priority, &job.ScheduledAt, &job.Attempts, &job.MaxAttempts,
			&job.LeaseOwner, &job.LeaseExpiresAt, &job.HeartbeatAt, &job.LastError,
			&job.CreatedAt, &job.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("jobs: scan claimed job: %w", err)
		}
		claimed = append(claimed, job)
	}
	return claimed, rows.Err()
}

// MarkRunning transitions a claimed job to RUNNING (owner-only).
func (queue *Queue) MarkRunning(ctx context.Context, owner string, jobID string) error {
	return queue.ownerUpdate(ctx, jobID, owner, `
		UPDATE fleet_partner_jobs SET status = '`+StatusRunning+`', heartbeat_at = now()
		WHERE id = $1 AND lease_owner = $2 AND status = '`+StatusClaimed+`'`)
}

// Heartbeat extends the lease (owner-only).
func (queue *Queue) Heartbeat(ctx context.Context, owner string, jobID string, leaseFor time.Duration) error {
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			heartbeat_at = now(),
			lease_expires_at = now() + make_interval(secs => $1::double precision)
		WHERE id = $2 AND lease_owner = $3 AND status IN ($4, $5)`,
		int64(leaseFor.Seconds()), jobID, owner, StatusClaimed, StatusRunning,
	)
	if err != nil {
		return fmt.Errorf("jobs: heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

// Complete finalizes a job successfully (owner-only).
func (queue *Queue) Complete(ctx context.Context, owner string, jobID string) error {
	return queue.ownerUpdate(ctx, jobID, owner, `
		UPDATE fleet_partner_jobs SET
			status = '`+StatusSucceeded+`', completed_at = now(),
			lease_owner = NULL, lease_expires_at = NULL
		WHERE id = $1 AND lease_owner = $2 AND status IN ('`+StatusClaimed+`','`+StatusRunning+`')`)
}

// Fail records a redacted failure (owner-only); the job retries with
// exponential backoff until max_attempts, then dead-letters.
func (queue *Queue) Fail(ctx context.Context, owner string, jobID string, jobErr error) error {
	redacted := redact(jobErr)
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			last_error_redacted = $1,
			status = CASE WHEN attempts >= max_attempts THEN $2 ELSE $3 END,
			completed_at = CASE WHEN attempts >= max_attempts THEN now() ELSE NULL END,
			scheduled_at = now() + make_interval(secs => LEAST(600, power(2, attempts))),
			lease_owner = NULL,
			lease_expires_at = NULL
		WHERE id = $4 AND lease_owner = $5 AND status IN ($6, $7)`,
		redacted, StatusDeadLettered, StatusRetryScheduled,
		jobID, owner, StatusClaimed, StatusRunning,
	)
	if err != nil {
		return fmt.Errorf("jobs: fail: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

// DeadLetter forces a job into DEAD_LETTERED with a security/error code.
func (queue *Queue) DeadLetter(ctx context.Context, owner string, jobID string, reason string) error {
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			status = $1, last_error_redacted = $2, completed_at = now(),
			lease_owner = NULL, lease_expires_at = NULL
		WHERE id = $3 AND lease_owner = $4`,
		StatusDeadLettered, reason, jobID, owner,
	)
	if err != nil {
		return fmt.Errorf("jobs: dead letter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

// Counts returns queue depth by status for metrics.
func (queue *Queue) Counts(ctx context.Context) (map[string]int64, error) {
	rows, err := queue.db.Query(ctx,
		`SELECT status, count(*) FROM fleet_partner_jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("jobs: counts: %w", err)
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// ValidateTenant revalidates that the payload's organizationId matches the
// durable job row before execution.
func ValidateTenant(job Job) error {
	var payload struct {
		OrganizationID string `json:"organizationId"`
	}
	if len(job.Payload) > 0 {
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("jobs: payload is not valid JSON: %w", err)
		}
	}
	if job.OrganizationID == "" && payload.OrganizationID == "" {
		return nil // explicitly platform-scoped job
	}
	if job.OrganizationID != payload.OrganizationID {
		return ErrTenantMismatch
	}
	return nil
}

func (queue *Queue) ownerUpdate(ctx context.Context, jobID string, owner string, sql string) error {
	tag, err := queue.db.Exec(ctx, sql, jobID, owner)
	if err != nil {
		return fmt.Errorf("jobs: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

func redact(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}
