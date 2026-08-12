package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Schedule types.
const (
	ScheduleCron     = "CRON"
	ScheduleInterval = "INTERVAL"
	ScheduleOnce     = "ONCE"
)

// Concurrency policies.
const (
	ConcurrencyAllow   = "ALLOW"
	ConcurrencyForbid  = "FORBID"
	ConcurrencyReplace = "REPLACE"
)

// Misfire policies.
const (
	MisfireSkip           = "SKIP"
	MisfireRunOnce        = "RUN_ONCE"
	MisfireCatchUpLimited = "CATCH_UP_LIMITED"
)

// misfireGrace is how far behind next_run_at the wall clock must be before
// an occurrence counts as missed rather than merely "due this poll cycle."
const misfireGrace = 30 * time.Second

var (
	ErrNotFound        = errors.New("scheduler: scheduled task not found")
	ErrVersionConflict = errors.New("scheduler: expected version does not match")
)

// Task is one durable scheduled task.
type Task struct {
	ID                 string
	OrganizationID     string
	BranchID           string
	TaskType           string
	JobType            string
	ScheduleType       string
	ScheduleExpression string
	Timezone           string
	Payload            json.RawMessage
	PayloadVersion     int
	Enabled            bool
	NextRunAt          *time.Time
	LastScheduledAt    *time.Time
	LastStartedAt      *time.Time
	LastCompletedAt    *time.Time
	LastResult         string
	FailureCount       int
	ConcurrencyPolicy  string
	MisfirePolicy      string
	MaxCatchUp         int
	MaxAttempts        int
	Priority           int
	LeaseOwner         string
	LeaseExpiresAt     *time.Time
	CreatedBy          string
	UpdatedBy          string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
}

// ComputeNextRun determines the next occurrence strictly after `after`, in
// the task's own timezone. A ONCE task whose one occurrence has already
// passed returns (nil, nil) — no error, simply no further occurrences,
// exactly like a cron/interval task is never expected to return nil.
func ComputeNextRun(task Task, after time.Time) (*time.Time, error) {
	loc, err := time.LoadLocation(task.Timezone)
	if err != nil {
		return nil, fmt.Errorf("scheduler: invalid timezone %q: %w", task.Timezone, err)
	}
	switch task.ScheduleType {
	case ScheduleCron:
		cron, err := ParseCron(task.ScheduleExpression)
		if err != nil {
			return nil, err
		}
		next, err := cron.Next(after, loc)
		if err != nil {
			return nil, err
		}
		return &next, nil
	case ScheduleInterval:
		seconds, err := strconv.Atoi(task.ScheduleExpression)
		if err != nil || seconds <= 0 {
			return nil, fmt.Errorf("scheduler: invalid interval expression %q", task.ScheduleExpression)
		}
		next := after.Add(time.Duration(seconds) * time.Second).UTC()
		return &next, nil
	case ScheduleOnce:
		at, err := time.ParseInLocation(time.RFC3339, task.ScheduleExpression, loc)
		if err != nil {
			return nil, fmt.Errorf("scheduler: invalid once expression %q: %w", task.ScheduleExpression, err)
		}
		atUTC := at.UTC()
		if !atUTC.After(after) {
			return nil, nil
		}
		return &atUTC, nil
	default:
		return nil, fmt.Errorf("scheduler: unknown schedule type %q", task.ScheduleType)
	}
}

// Repository persists scheduled tasks and their run history.
type Repository struct {
	db  database.Querier
	now func() time.Time
}

// NewRepository wires the repository. now defaults to time.Now.
func NewRepository(db database.Querier) *Repository {
	return &Repository{db: db, now: time.Now}
}

const taskColumns = `id, COALESCE(organization_id,''), COALESCE(branch_id,''), task_type, job_type,
	schedule_type, schedule_expression, timezone, payload, payload_version, enabled, next_run_at,
	last_scheduled_at, last_started_at, last_completed_at, last_result, failure_count,
	concurrency_policy, misfire_policy, max_catch_up, max_attempts, priority,
	COALESCE(lease_owner,''), lease_expires_at, COALESCE(created_by,''), COALESCE(updated_by,''),
	version, created_at, updated_at, archived_at`

type scannable interface{ Scan(dest ...any) error }

func scanTask(row scannable) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.OrganizationID, &t.BranchID, &t.TaskType, &t.JobType, &t.ScheduleType,
		&t.ScheduleExpression, &t.Timezone, &t.Payload, &t.PayloadVersion, &t.Enabled, &t.NextRunAt,
		&t.LastScheduledAt, &t.LastStartedAt, &t.LastCompletedAt, &t.LastResult, &t.FailureCount,
		&t.ConcurrencyPolicy, &t.MisfirePolicy, &t.MaxCatchUp, &t.MaxAttempts, &t.Priority,
		&t.LeaseOwner, &t.LeaseExpiresAt, &t.CreatedBy, &t.UpdatedBy, &t.Version, &t.CreatedAt,
		&t.UpdatedAt, &t.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

// Create inserts a new scheduled task with its first computed occurrence.
func (r *Repository) Create(ctx context.Context, q database.Querier, task *Task) error {
	if task.ID == "" {
		task.ID = ids.New("sct")
	}
	if task.Timezone == "" {
		task.Timezone = "UTC"
	}
	if task.ConcurrencyPolicy == "" {
		task.ConcurrencyPolicy = ConcurrencyForbid
	}
	if task.MisfirePolicy == "" {
		task.MisfirePolicy = MisfireSkip
	}
	if task.MaxCatchUp <= 0 {
		task.MaxCatchUp = 1
	}
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = 5
	}
	if task.PayloadVersion <= 0 {
		task.PayloadVersion = 1
	}
	if _, err := time.LoadLocation(task.Timezone); err != nil {
		return fmt.Errorf("scheduler: invalid timezone %q: %w", task.Timezone, err)
	}
	next, err := ComputeNextRun(*task, r.now().UTC())
	if err != nil {
		return err
	}
	if task.Payload == nil {
		task.Payload = json.RawMessage(`{}`)
	}
	querier := q
	if querier == nil {
		querier = r.db
	}
	row := querier.QueryRow(ctx, `
		INSERT INTO fleet_partner_scheduled_tasks
			(id, organization_id, branch_id, task_type, job_type, schedule_type,
			 schedule_expression, timezone, payload, payload_version, enabled, next_run_at,
			 concurrency_policy, misfire_policy, max_catch_up, max_attempts, priority,
			 created_by, updated_by)
		VALUES ($1, NULLIF($2,''), NULLIF($3,''), $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $18)
		RETURNING `+taskColumns,
		task.ID, task.OrganizationID, task.BranchID, task.TaskType, task.JobType, task.ScheduleType,
		task.ScheduleExpression, task.Timezone, task.Payload, task.PayloadVersion, task.Enabled, next,
		task.ConcurrencyPolicy, task.MisfirePolicy, task.MaxCatchUp, task.MaxAttempts, task.Priority,
		task.CreatedBy,
	)
	created, err := scanTask(row)
	if err != nil {
		return fmt.Errorf("scheduler: create: %w", err)
	}
	*task = created
	return nil
}

// Get reads one tenant-scoped task.
func (r *Repository) Get(ctx context.Context, organizationID string, taskID string) (Task, error) {
	task, err := scanTask(r.db.QueryRow(ctx,
		`SELECT `+taskColumns+` FROM fleet_partner_scheduled_tasks
		 WHERE id = $1 AND COALESCE(organization_id,'') = $2 AND archived_at IS NULL`,
		taskID, organizationID))
	if err != nil {
		return Task{}, fmt.Errorf("scheduler: get: %w", err)
	}
	return task, nil
}

// List returns tenant-scoped tasks.
func (r *Repository) List(ctx context.Context, organizationID string, limit int) ([]Task, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+taskColumns+` FROM fleet_partner_scheduled_tasks
		 WHERE COALESCE(organization_id,'') = $1 AND archived_at IS NULL
		 ORDER BY created_at DESC LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list: %w", err)
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduler: scan list: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// SetEnabled toggles a task, version-guarded.
func (r *Repository) SetEnabled(ctx context.Context, organizationID string, taskID string, enabled bool, expectedVersion int64, actor string) (Task, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_scheduled_tasks SET enabled=$1, updated_by=$2, updated_at=now(), version=version+1
		WHERE id=$3 AND COALESCE(organization_id,'')=$4 AND version=$5 AND archived_at IS NULL
		RETURNING `+taskColumns,
		enabled, actor, taskID, organizationID, expectedVersion)
	task, err := scanTask(row)
	if errors.Is(err, ErrNotFound) {
		return Task{}, ErrVersionConflict
	}
	return task, err
}

// UpdateSchedule changes the schedule definition, version-guarded, and
// recomputes next_run_at from the new definition.
func (r *Repository) UpdateSchedule(ctx context.Context, organizationID string, taskID string,
	scheduleType, scheduleExpression, timezone string, payload json.RawMessage, expectedVersion int64,
	actor string) (Task, error) {
	existing, err := r.Get(ctx, organizationID, taskID)
	if err != nil {
		return Task{}, err
	}
	if existing.Version != expectedVersion {
		return Task{}, ErrVersionConflict
	}
	updated := existing
	updated.ScheduleType = scheduleType
	updated.ScheduleExpression = scheduleExpression
	updated.Timezone = timezone
	if payload != nil {
		updated.Payload = payload
	}
	next, err := ComputeNextRun(updated, r.now().UTC())
	if err != nil {
		return Task{}, err
	}
	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_scheduled_tasks SET
			schedule_type=$1, schedule_expression=$2, timezone=$3, payload=$4, next_run_at=$5,
			updated_by=$6, updated_at=now(), version=version+1
		WHERE id=$7 AND COALESCE(organization_id,'')=$8 AND version=$9 AND archived_at IS NULL
		RETURNING `+taskColumns,
		updated.ScheduleType, updated.ScheduleExpression, updated.Timezone, updated.Payload, next,
		actor, taskID, organizationID, expectedVersion)
	task, err := scanTask(row)
	if errors.Is(err, ErrNotFound) {
		return Task{}, ErrVersionConflict
	}
	return task, err
}

// RunNow forces next_run_at to now, so the next scheduler pass picks the
// task up as due regardless of its ordinary schedule — a manual run does
// not otherwise alter the schedule.
func (r *Repository) RunNow(ctx context.Context, organizationID string, taskID string, expectedVersion int64, actor string) (Task, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_scheduled_tasks SET next_run_at=now(), updated_by=$1, updated_at=now(), version=version+1
		WHERE id=$2 AND COALESCE(organization_id,'')=$3 AND version=$4 AND archived_at IS NULL
		RETURNING `+taskColumns,
		actor, taskID, organizationID, expectedVersion)
	task, err := scanTask(row)
	if errors.Is(err, ErrNotFound) {
		return Task{}, ErrVersionConflict
	}
	return task, err
}

// ClaimDue leases up to limit due tasks using FOR UPDATE SKIP LOCKED, so
// multiple scheduler instances never process the same due task
// concurrently.
func (r *Repository) ClaimDue(ctx context.Context, owner string, leaseFor time.Duration, limit int) ([]Task, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		UPDATE fleet_partner_scheduled_tasks SET
			lease_owner=$1, lease_expires_at=now()+make_interval(secs=>$2::double precision)
		WHERE id IN (
			SELECT id FROM fleet_partner_scheduled_tasks
			WHERE enabled=true AND archived_at IS NULL AND next_run_at IS NOT NULL AND next_run_at<=now()
			  AND (lease_expires_at IS NULL OR lease_expires_at<now())
			ORDER BY next_run_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+taskColumns,
		owner, leaseFor.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("scheduler: claim due: %w", err)
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scheduler: scan claimed: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// Advance records the outcome of one due pass and moves the task to its
// next occurrence (owner-only — only the instance holding the lease may
// advance it), clearing the lease.
func (r *Repository) Advance(ctx context.Context, owner string, taskID string, nextRunAt *time.Time, result string, failed bool) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_scheduled_tasks SET
			next_run_at=$1, last_scheduled_at=now(), last_result=$2,
			failure_count = CASE WHEN $3 THEN failure_count+1 ELSE 0 END,
			lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE id=$4 AND lease_owner=$5`,
		nextRunAt, result, failed, taskID, owner)
	if err != nil {
		return fmt.Errorf("scheduler: advance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("scheduler: task %s is not leased by %s", taskID, owner)
	}
	return nil
}

// RecordRun appends one immutable scheduler-decision record.
func (r *Repository) RecordRun(ctx context.Context, taskID string, organizationID string,
	scheduledFor time.Time, outcome string, jobID string, safeErrorCode string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO fleet_partner_scheduled_task_runs
			(id, scheduled_task_id, organization_id, scheduled_for, outcome, job_id, safe_error_code)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),$7)`,
		ids.New("sctr"), taskID, organizationID, scheduledFor, outcome, jobID, safeErrorCode)
	if err != nil {
		return fmt.Errorf("scheduler: record run: %w", err)
	}
	return nil
}

// ListRuns returns a task's run history, newest first.
func (r *Repository) ListRuns(ctx context.Context, organizationID string, taskID string, limit int) ([]RunRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, scheduled_task_id, COALESCE(organization_id,''), scheduled_for, outcome,
			COALESCE(job_id,''), safe_error_code, occurred_at
		FROM fleet_partner_scheduled_task_runs
		WHERE scheduled_task_id=$1 AND COALESCE(organization_id,'')=$2
		ORDER BY occurred_at DESC LIMIT $3`, taskID, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduler: list runs: %w", err)
	}
	defer rows.Close()
	runs := []RunRecord{}
	for rows.Next() {
		var r RunRecord
		if err := rows.Scan(&r.ID, &r.ScheduledTaskID, &r.OrganizationID, &r.ScheduledFor, &r.Outcome,
			&r.JobID, &r.SafeErrorCode, &r.OccurredAt); err != nil {
			return nil, fmt.Errorf("scheduler: scan run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// RunRecord is one immutable scheduler decision.
type RunRecord struct {
	ID              string
	ScheduledTaskID string
	OrganizationID  string
	ScheduledFor    time.Time
	Outcome         string
	JobID           string
	SafeErrorCode   string
	OccurredAt      time.Time
}

// Run-outcome constants, matching the fleet_partner_scheduled_task_runs
// CHECK constraint.
const (
	OutcomeEnqueued       = "ENQUEUED"
	OutcomeSkippedForbid  = "SKIPPED_FORBID"
	OutcomeSkippedMisfire = "SKIPPED_MISFIRE"
	OutcomeReplacedPrior  = "REPLACED_PRIOR"
	OutcomeEnqueueFailed  = "ENQUEUE_FAILED"
)
