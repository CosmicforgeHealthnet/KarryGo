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

	"github.com/jackc/pgx/v5"

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
	ID              string
	OrganizationID  string
	BranchID        string
	JobType         string
	DedupeKey       string
	IdempotencyKey  string
	Payload         json.RawMessage
	PayloadVersion  int
	Status          string
	Priority        int
	ScheduledAt     time.Time
	CorrelationID   string
	CausationID     string
	ScheduledTaskID string
	ScheduledFor    *time.Time
	Attempts        int
	MaxAttempts     int
	NextAttemptAt   *time.Time
	LeaseOwner      string
	LeaseExpiresAt  *time.Time
	HeartbeatAt     *time.Time
	StartedAt       *time.Time
	LastError       string
	LastErrorClass  string
	LastErrorCode   string
	// ExternalOutcomeStatus is "" (no external call yet), UNKNOWN (an
	// external call was made and its outcome could not be confirmed),
	// CONFIRMED_SUCCEEDED or CONFIRMED_FAILED (a read-back resolved it).
	ExternalOutcomeStatus         string
	AuthoritativeReadbackRequired bool
	Version                       int64
	CreatedAt                     time.Time
	CompletedAt                   *time.Time
	CancelledAt                   *time.Time
	DeadLetteredAt                *time.Time
}

// EnqueueRequest describes a job to enqueue.
type EnqueueRequest struct {
	OrganizationID string
	BranchID       string
	JobType        string
	DedupeKey      string
	// IdempotencyKey is a caller-supplied idempotency identity, distinct
	// from DedupeKey (which a worker or scheduler derives on the caller's
	// behalf) but enforcing the identical "same key returns the existing
	// job" guarantee under its own column.
	IdempotencyKey  string
	Payload         any
	PayloadVersion  int
	Priority        int
	ScheduledAt     time.Time
	CorrelationID   string
	CausationID     string
	ScheduledTaskID string
	ScheduledFor    *time.Time
	MaxAttempts     int
	// AuthoritativeReadbackRequired marks a job whose handler must confirm
	// an ambiguous external outcome via read-back before any further
	// state-changing write, rather than retrying blind.
	AuthoritativeReadbackRequired bool
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
	if request.PayloadVersion <= 0 {
		request.PayloadVersion = 1
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
			(id, organization_id, branch_id, job_type, dedupe_key, idempotency_key,
			 payload, payload_version, status, priority, scheduled_at, correlation_id,
			 causation_id, scheduled_task_id, scheduled_for, attempts, max_attempts,
			 authoritative_readback_required, created_at)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''),
			$7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, 0, $16, $17, now())
		ON CONFLICT (COALESCE(organization_id, ''), job_type, dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING')
		DO NOTHING
		RETURNING id`,
		jobID, request.OrganizationID, request.BranchID, request.JobType, request.DedupeKey,
		request.IdempotencyKey, payload, request.PayloadVersion, StatusPending, request.Priority,
		request.ScheduledAt.UTC(), request.CorrelationID, request.CausationID, request.ScheduledTaskID,
		request.ScheduledFor, request.MaxAttempts, request.AuthoritativeReadbackRequired,
	).Scan(&insertedID)
	if err == nil {
		return insertedID, nil
	}

	// Conflict: return the existing active job for this organization-scoped
	// dedupe key, or (for a scheduled-task occurrence with no dedupe key) the
	// job already created for the same scheduled_task_id + scheduled_for.
	var existingID string
	var lookupErr error
	if request.DedupeKey != "" {
		lookupErr = q.QueryRow(ctx, `
			SELECT id FROM fleet_partner_jobs
			WHERE COALESCE(organization_id, '') = $1 AND job_type = $2 AND dedupe_key = $3
			  AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING')
			ORDER BY created_at DESC LIMIT 1`,
			request.OrganizationID, request.JobType, request.DedupeKey,
		).Scan(&existingID)
	} else if request.ScheduledTaskID != "" && request.ScheduledFor != nil {
		lookupErr = q.QueryRow(ctx, `
			SELECT id FROM fleet_partner_jobs WHERE scheduled_task_id = $1 AND scheduled_for = $2`,
			request.ScheduledTaskID, request.ScheduledFor,
		).Scan(&existingID)
	} else {
		lookupErr = err
	}
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
			attempts = attempts + 1,
			started_at = COALESCE(started_at, now()),
			version = version + 1
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
		RETURNING `+jobColumns,
		StatusClaimed, owner, int64(leaseFor.Seconds()),
		StatusPending, StatusRetryScheduled, StatusClaimed, StatusRunning, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs: claim batch: %w", err)
	}
	defer rows.Close()

	var claimed []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("jobs: scan claimed job: %w", err)
		}
		claimed = append(claimed, job)
	}
	return claimed, rows.Err()
}

const jobColumns = `id, COALESCE(organization_id, ''), COALESCE(branch_id, ''), job_type,
	COALESCE(dedupe_key, ''), COALESCE(idempotency_key, ''), payload, payload_version, status,
	priority, scheduled_at, correlation_id, causation_id, COALESCE(scheduled_task_id, ''),
	scheduled_for, attempts, max_attempts, next_attempt_at, COALESCE(lease_owner, ''),
	lease_expires_at, heartbeat_at, started_at, COALESCE(last_error_redacted, ''),
	last_error_class, last_error_code, external_outcome_status, authoritative_readback_required,
	version, created_at, completed_at, cancelled_at, dead_lettered_at`

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (Job, error) {
	var job Job
	err := row.Scan(
		&job.ID, &job.OrganizationID, &job.BranchID, &job.JobType, &job.DedupeKey,
		&job.IdempotencyKey, &job.Payload, &job.PayloadVersion, &job.Status, &job.Priority,
		&job.ScheduledAt, &job.CorrelationID, &job.CausationID, &job.ScheduledTaskID,
		&job.ScheduledFor, &job.Attempts, &job.MaxAttempts, &job.NextAttemptAt, &job.LeaseOwner,
		&job.LeaseExpiresAt, &job.HeartbeatAt, &job.StartedAt, &job.LastError, &job.LastErrorClass,
		&job.LastErrorCode, &job.ExternalOutcomeStatus, &job.AuthoritativeReadbackRequired,
		&job.Version, &job.CreatedAt, &job.CompletedAt, &job.CancelledAt, &job.DeadLetteredAt,
	)
	return job, err
}

// MarkRunning transitions a claimed job to RUNNING (owner-only).
func (queue *Queue) MarkRunning(ctx context.Context, owner string, jobID string) error {
	return queue.ownerUpdate(ctx, jobID, owner, `
		UPDATE fleet_partner_jobs SET status = '`+StatusRunning+`', heartbeat_at = now(), version = version + 1
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
			lease_owner = NULL, lease_expires_at = NULL, version = version + 1
		WHERE id = $1 AND lease_owner = $2 AND status IN ('`+StatusClaimed+`','`+StatusRunning+`')`)
}

// RecordAttempt appends the immutable execution result before the mutable job
// projection is finalized. If this write fails, callers leave the lease in
// place so expiry recovery can retry without losing attempt evidence.
func (queue *Queue) RecordAttempt(ctx context.Context, job Job, owner, status string, started time.Time, jobErr error) error {
	class, code, message := Classify(jobErr)
	_, err := queue.db.Exec(ctx, `INSERT INTO fleet_partner_job_attempts
		(id,job_id,organization_id,attempt_number,worker_owner,status,error_redacted,
		 error_class,error_code,started_at,heartbeat_at,completed_at)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,now(),now())`, ids.New("jat"), job.ID,
		job.OrganizationID, job.Attempts, owner, status, message, string(class), code, started)
	if err != nil {
		return fmt.Errorf("jobs: record immutable attempt: %w", err)
	}
	return nil
}

// Fail records a classified failure (owner-only). A classified
// (*ClassifiedError) is retried with exponential backoff, and permanent/
// validation/authorization classes skip retry and dead-letter on the first
// failure regardless of max_attempts — repeating an input or code error
// cannot succeed on a later attempt. An unknown-external-outcome error
// leaves authoritative_readback_required set so a caller can require
// read-back before any further state-changing retry.
func (queue *Queue) Fail(ctx context.Context, owner string, jobID string, jobErr error) error {
	class, code, message := Classify(jobErr)
	noRetry := !class.Retryable()
	readback := class.RequiresReadback()
	retryAfterSeconds := int64(RetryDelay(jobErr).Seconds())
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			last_error_redacted = $1,
			last_error_class = $2,
			last_error_code = $3,
			status = CASE WHEN $4 OR attempts >= max_attempts THEN $5 ELSE $6 END,
			completed_at = NULL,
			dead_lettered_at = CASE WHEN $4 OR attempts >= max_attempts THEN now() ELSE dead_lettered_at END,
			next_attempt_at = CASE WHEN $4 OR attempts >= max_attempts THEN NULL
				ELSE now() + make_interval(secs => CASE WHEN $8 > 0 THEN $8 ELSE LEAST(600, power(2, attempts)) END) END,
			scheduled_at = CASE WHEN $4 OR attempts >= max_attempts THEN scheduled_at
				ELSE now() + make_interval(secs => CASE WHEN $8 > 0 THEN $8 ELSE LEAST(600, power(2, attempts)) END) END,
			external_outcome_status = CASE WHEN $7 THEN 'UNKNOWN' ELSE external_outcome_status END,
			authoritative_readback_required = authoritative_readback_required OR $7,
			lease_owner = NULL,
			lease_expires_at = NULL,
			version = version + 1
		WHERE id = $9 AND lease_owner = $10 AND status IN ($11, $12)`,
		message, string(class), code, noRetry, StatusDeadLettered, StatusRetryScheduled, readback,
		retryAfterSeconds, jobID, owner, StatusClaimed, StatusRunning,
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
			status = $1, last_error_redacted = $2, completed_at = NULL, dead_lettered_at = now(),
			lease_owner = NULL, lease_expires_at = NULL, version = version + 1
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

// ReleaseForShutdown releases a lease without recording success or failure,
// for a job whose handler cooperated with shutdown cancellation before any
// external effect that cannot be safely retried. The job returns to PENDING
// immediately (not backed off) since nothing about the job itself failed —
// only the process hosting it stopped.
func (queue *Queue) ReleaseForShutdown(ctx context.Context, owner string, jobID string) error {
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			status = $1, lease_owner = NULL, lease_expires_at = NULL, version = version + 1
		WHERE id = $2 AND lease_owner = $3 AND status IN ($4, $5)`,
		StatusPending, jobID, owner, StatusClaimed, StatusRunning,
	)
	if err != nil {
		return fmt.Errorf("jobs: release for shutdown: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

// Cancel cancels a PENDING or RETRY_SCHEDULED job outright, or marks a
// CLAIMED/RUNNING job's cancellation intent by leaving its lease alone (the
// handler is expected to observe ctx cancellation; the caller is
// responsible for actually cancelling the context that job's handler runs
// under). A SUCCEEDED job can never be retroactively cancelled.
func (queue *Queue) Cancel(ctx context.Context, organizationID string, jobID string) error {
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET
			status = $1, cancelled_at = now(), lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1
		WHERE id = $2 AND COALESCE(organization_id, '') = $3 AND status IN ($4, $5)`,
		StatusCancelled, jobID, organizationID, StatusPending, StatusRetryScheduled,
	)
	if err != nil {
		return fmt.Errorf("jobs: cancel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobs: job not found or not cancellable")
	}
	return nil
}

// Replay returns a tenant-owned dead-lettered job to the pending queue. The
// caller is responsible for enforcing the operator replay permission and
// recording audit; the queue still enforces organization ownership.
func (queue *Queue) Replay(ctx context.Context, organizationID string, jobID string) error {
	tag, err := queue.db.Exec(ctx, `
		UPDATE fleet_partner_jobs SET status=$1, attempts=0, scheduled_at=now(),
			lease_owner=NULL, lease_expires_at=NULL, heartbeat_at=NULL,
			last_error_redacted=NULL, last_error_class='', last_error_code='',
			completed_at=NULL, dead_lettered_at=NULL, next_attempt_at=NULL, version=version+1
		WHERE id=$2 AND organization_id=$3 AND status=$4`,
		StatusPending, jobID, organizationID, StatusDeadLettered)
	if err != nil {
		return fmt.Errorf("jobs: replay: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobs: dead-lettered job not found")
	}
	return nil
}

// Get reads one job, tenant-scoped. A platform-scoped job (organization_id
// NULL) is reachable only when organizationID is also empty, matching how
// every other tenant-scoped read in this service refuses to disclose
// whether a row in another scope exists.
func (queue *Queue) Get(ctx context.Context, organizationID string, jobID string) (Job, error) {
	return scanJob(queue.db.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM fleet_partner_jobs WHERE id = $1 AND COALESCE(organization_id, '') = $2`,
		jobID, organizationID))
}

// ListFilter narrows a tenant-scoped job listing.
type ListFilter struct {
	Status  string
	JobType string
	Limit   int
}

// List returns tenant-scoped jobs, newest first.
func (queue *Queue) List(ctx context.Context, organizationID string, filter ListFilter) ([]Job, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := queue.db.Query(ctx, `
		SELECT `+jobColumns+` FROM fleet_partner_jobs
		WHERE COALESCE(organization_id, '') = $1
		  AND ($2 = '' OR status = $2) AND ($3 = '' OR job_type = $3)
		ORDER BY created_at DESC LIMIT $4`,
		organizationID, filter.Status, filter.JobType, limit)
	if err != nil {
		return nil, fmt.Errorf("jobs: list: %w", err)
	}
	defer rows.Close()
	jobsOut := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("jobs: scan list job: %w", err)
		}
		jobsOut = append(jobsOut, job)
	}
	return jobsOut, rows.Err()
}

// ActiveJobForScheduledTask returns the most recent non-terminal job tied to
// a scheduled task, for concurrency-policy enforcement (ALLOW/FORBID/
// REPLACE) — the scheduler must know whether a prior occurrence is still in
// flight before deciding whether to enqueue, skip, or replace this one.
// found is false (with a zero error) when no such job exists.
func (queue *Queue) ActiveJobForScheduledTask(ctx context.Context, scheduledTaskID string) (job Job, found bool, err error) {
	job, err = scanJob(queue.db.QueryRow(ctx, `
		SELECT `+jobColumns+` FROM fleet_partner_jobs
		WHERE scheduled_task_id = $1 AND status IN ($2,$3,$4,$5)
		ORDER BY created_at DESC LIMIT 1`,
		scheduledTaskID, StatusPending, StatusRetryScheduled, StatusClaimed, StatusRunning))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("jobs: active job for scheduled task: %w", err)
	}
	return job, true, nil
}

// Attempt is one immutable execution record.
type Attempt struct {
	ID             string
	JobID          string
	OrganizationID string
	AttemptNumber  int
	WorkerOwner    string
	Status         string
	ErrorRedacted  string
	ErrorClass     string
	ErrorCode      string
	StartedAt      time.Time
	HeartbeatAt    *time.Time
	CompletedAt    *time.Time
}

// ListAttempts returns a job's immutable attempt history, newest first.
func (queue *Queue) ListAttempts(ctx context.Context, organizationID string, jobID string) ([]Attempt, error) {
	rows, err := queue.db.Query(ctx, `
		SELECT a.id, a.job_id, COALESCE(a.organization_id,''), a.attempt_number, a.worker_owner,
			a.status, a.error_redacted, a.error_class, a.error_code, a.started_at,
			a.heartbeat_at, a.completed_at
		FROM fleet_partner_job_attempts a
		JOIN fleet_partner_jobs j ON j.id = a.job_id
		WHERE a.job_id = $1 AND COALESCE(j.organization_id,'') = $2
		ORDER BY a.attempt_number DESC`, jobID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("jobs: list attempts: %w", err)
	}
	defer rows.Close()
	attempts := []Attempt{}
	for rows.Next() {
		var a Attempt
		if err := rows.Scan(&a.ID, &a.JobID, &a.OrganizationID, &a.AttemptNumber, &a.WorkerOwner,
			&a.Status, &a.ErrorRedacted, &a.ErrorClass, &a.ErrorCode, &a.StartedAt,
			&a.HeartbeatAt, &a.CompletedAt); err != nil {
			return nil, fmt.Errorf("jobs: scan attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// StaleLeaseCount reports CLAIMED/RUNNING jobs whose lease has been expired
// for longer than olderThan — the read-back a health check needs to tell
// "workers are just busy" from "leases are not being recovered."
func (queue *Queue) StaleLeaseCount(ctx context.Context, olderThan time.Duration) (int64, error) {
	var count int64
	err := queue.db.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_jobs
		WHERE status IN ($1, $2) AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now() - make_interval(secs => $3::double precision)`,
		StatusClaimed, StatusRunning, olderThan.Seconds()).Scan(&count)
	return count, err
}

// DeadLetterCount reports the current dead-letter backlog for a health
// threshold check.
func (queue *Queue) DeadLetterCount(ctx context.Context) (int64, error) {
	var count int64
	err := queue.db.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_jobs WHERE status = $1`, StatusDeadLettered).Scan(&count)
	return count, err
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
