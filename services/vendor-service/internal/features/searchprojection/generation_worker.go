package searchprojection

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProjectionJobType string

const (
	ProjectionJobIncremental ProjectionJobType = "INCREMENTAL_PROJECTION"

	ProjectionJobFullRebuild ProjectionJobType = "FULL_REBUILD"

	ProjectionJobScopedRebuild ProjectionJobType = "SCOPED_REBUILD"

	ProjectionJobValidateActivate ProjectionJobType = "GENERATION_VALIDATE_ACTIVATE"

	ProjectionJobCacheReconcile ProjectionJobType = "CACHE_RECONCILE"
)

type ProjectionJobStatus string

const (
	ProjectionJobPending ProjectionJobStatus = "PENDING"

	ProjectionJobLeased ProjectionJobStatus = "LEASED"

	ProjectionJobRetry ProjectionJobStatus = "RETRY"

	ProjectionJobCompleted ProjectionJobStatus = "COMPLETED"

	ProjectionJobDeadLetter ProjectionJobStatus = "DEAD_LETTER"
)

type ProjectionJob struct {
	JobID string

	JobType ProjectionJobType

	Namespace SearchCacheNamespace

	Generation int64

	ScopeType SearchRebuildScope
	ScopeRef  string

	ReferenceID string

	Status ProjectionJobStatus

	Priority int

	AttemptCount int
	MaxAttempts  int

	AvailableAt time.Time

	LeaseOwner     string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	HeartbeatAt    *time.Time

	ProgressCheckpoint string

	FailureCode   string
	FailureReason string

	DedupeKey string
}

type ProjectionJobStore interface {
	ClaimProjectionJobs(
		ctx context.Context,
		workerID string,
		jobTypes []ProjectionJobType,
		limit int,
		leaseDuration time.Duration,
	) (
		[]ProjectionJob,
		error,
	)

	HeartbeatProjectionJob(
		ctx context.Context,
		jobID string,
		leaseToken string,
		leaseDuration time.Duration,
	) error

	UpdateProjectionJobCheckpoint(
		ctx context.Context,
		jobID string,
		leaseToken string,
		checkpoint string,
	) error

	CompleteProjectionJob(
		ctx context.Context,
		jobID string,
		leaseToken string,
	) error

	FailProjectionJob(
		ctx context.Context,
		job ProjectionJob,
		failureCode string,
		failureReason string,
		retryDelay time.Duration,
	) error

	ReplayDeadLetterProjectionJob(
		ctx context.Context,
		jobID string,
	) error
}

func randomLeaseToken() (
	string,
	error,
) {
	var value [16]byte

	if _, err :=
		rand.Read(
			value[:],
		); err != nil {
		return "", err
	}

	return hex.EncodeToString(
		value[:],
	), nil
}

func validProjectionJobType(
	jobType ProjectionJobType,
) bool {
	switch jobType {
	case ProjectionJobIncremental,
		ProjectionJobFullRebuild,
		ProjectionJobScopedRebuild,
		ProjectionJobValidateActivate,
		ProjectionJobCacheReconcile:
		return true

	default:
		return false
	}
}

func (
	store *PostgresSearchGenerationStore,
) ClaimProjectionJobs(
	ctx context.Context,
	workerID string,
	jobTypes []ProjectionJobType,
	limit int,
	leaseDuration time.Duration,
) (
	[]ProjectionJob,
	error,
) {
	if store == nil ||
		store.pool == nil ||
		strings.TrimSpace(workerID) == "" ||
		len(jobTypes) == 0 ||
		len(jobTypes) > 10 ||
		limit < 1 ||
		limit > 100 ||
		leaseDuration < 5*time.Second ||
		leaseDuration > 10*time.Minute {
		return nil,
			ErrInvalidArgument
	}

	typeNames :=
		make(
			[]string,
			0,
			len(jobTypes),
		)

	for _, jobType := range jobTypes {
		if !validProjectionJobType(
			jobType,
		) {
			return nil,
				ErrInvalidArgument
		}

		typeNames = append(
			typeNames,
			string(jobType),
		)
	}

	leaseToken, err :=
		randomLeaseToken()
	if err != nil {
		return nil, err
	}

	leaseSeconds :=
		int64(
			leaseDuration /
				time.Second,
		)

	rows, err :=
		store.pool.Query(
			ctx,
			`
            WITH candidates AS (
                SELECT job_id
                  FROM search_projection_jobs
                 WHERE job_type = ANY($1::text[])
                   AND (
                       (
                           status IN (
                               'PENDING',
                               'RETRY'
                           )
                           AND available_at <= now()
                       )
                       OR
                       (
                           status='LEASED'
                           AND lease_expires_at < now()
                       )
                   )
                 ORDER BY
                     priority ASC,
                     available_at ASC,
                     created_at ASC,
                     job_id ASC
                 FOR UPDATE SKIP LOCKED
                 LIMIT $2
            )
            UPDATE search_projection_jobs job
               SET
                   status='LEASED',
                   attempt_count=attempt_count+1,
                   lease_owner=$3,
                   lease_token=$4,
                   lease_expires_at=
                       now() +
                       make_interval(
                           secs => $5
                       ),
                   heartbeat_at=now(),
                   updated_at=now()
              FROM candidates
             WHERE job.job_id=candidates.job_id
            RETURNING
                job.job_id,
                job.job_type,
                job.namespace_code,
                COALESCE(job.generation, 0),
                COALESCE(job.scope_type, ''),
                COALESCE(job.scope_ref, ''),
                COALESCE(job.reference_id, ''),
                job.status,
                job.priority,
                job.attempt_count,
                job.max_attempts,
                job.available_at,
                COALESCE(job.lease_owner, ''),
                COALESCE(job.lease_token, ''),
                job.lease_expires_at,
                job.heartbeat_at,
                COALESCE(job.progress_checkpoint, ''),
                COALESCE(job.failure_code, ''),
                COALESCE(job.failure_reason, ''),
                job.dedupe_key
            `,
			typeNames,
			limit,
			strings.TrimSpace(
				workerID,
			),
			leaseToken,
			leaseSeconds,
		)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result :=
		make(
			[]ProjectionJob,
			0,
			limit,
		)

	for rows.Next() {
		var job ProjectionJob

		if err =
			rows.Scan(
				&job.JobID,
				&job.JobType,
				&job.Namespace,
				&job.Generation,
				&job.ScopeType,
				&job.ScopeRef,
				&job.ReferenceID,
				&job.Status,
				&job.Priority,
				&job.AttemptCount,
				&job.MaxAttempts,
				&job.AvailableAt,
				&job.LeaseOwner,
				&job.LeaseToken,
				&job.LeaseExpiresAt,
				&job.HeartbeatAt,
				&job.ProgressCheckpoint,
				&job.FailureCode,
				&job.FailureReason,
				&job.DedupeKey,
			); err != nil {
			return nil, err
		}

		result = append(
			result,
			job,
		)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (
	store *PostgresSearchGenerationStore,
) HeartbeatProjectionJob(
	ctx context.Context,
	jobID string,
	leaseToken string,
	leaseDuration time.Duration,
) error {
	if leaseDuration < 5*time.Second ||
		leaseDuration > 10*time.Minute {
		return ErrInvalidArgument
	}

	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_jobs
               SET
                   heartbeat_at=now(),
                   lease_expires_at=
                       now() +
                       make_interval(
                           secs => $3
                       ),
                   updated_at=now()
             WHERE job_id=$1
               AND lease_token=$2
               AND status='LEASED'
               AND lease_expires_at > now()
            `,
			jobID,
			leaseToken,
			int64(
				leaseDuration/
					time.Second,
			),
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrProjectionJobLeaseLost
	}

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) UpdateProjectionJobCheckpoint(
	ctx context.Context,
	jobID string,
	leaseToken string,
	checkpoint string,
) error {
	if len(
		[]rune(checkpoint),
	) > 2048 {
		return ErrInvalidArgument
	}

	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_jobs
               SET
                   progress_checkpoint=
                       NULLIF($3, ''),
                   updated_at=now()
             WHERE job_id=$1
               AND lease_token=$2
               AND status='LEASED'
               AND lease_expires_at > now()
            `,
			jobID,
			leaseToken,
			checkpoint,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrProjectionJobLeaseLost
	}

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) CompleteProjectionJob(
	ctx context.Context,
	jobID string,
	leaseToken string,
) error {
	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_jobs
               SET
                   status='COMPLETED',
                   lease_owner=NULL,
                   lease_token=NULL,
                   lease_expires_at=NULL,
                   heartbeat_at=NULL,
                   failure_code=NULL,
                   failure_reason=NULL,
                   completed_at=now(),
                   updated_at=now()
             WHERE job_id=$1
               AND lease_token=$2
               AND status='LEASED'
            `,
			jobID,
			leaseToken,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrProjectionJobLeaseLost
	}

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) FailProjectionJob(
	ctx context.Context,
	job ProjectionJob,
	failureCode string,
	failureReason string,
	retryDelay time.Duration,
) error {
	if retryDelay < 0 ||
		retryDelay > 15*time.Minute {
		return ErrInvalidArgument
	}

	failureCode =
		strings.TrimSpace(
			failureCode,
		)

	if len(failureCode) > 64 {
		failureCode =
			failureCode[:64]
	}

	failureReason =
		boundedFailureText(
			failureReason,
		)

	nextStatus :=
		ProjectionJobRetry

	if job.AttemptCount >=
		job.MaxAttempts {
		nextStatus =
			ProjectionJobDeadLetter
	}

	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_jobs
               SET
                   status=$3,
                   failure_code=NULLIF($4, ''),
                   failure_reason=NULLIF($5, ''),
                   available_at=
                       CASE
                           WHEN $3='RETRY'
                               THEN now() +
                                   make_interval(
                                       secs => $6
                                   )
                           ELSE available_at
                       END,
                   lease_owner=NULL,
                   lease_token=NULL,
                   lease_expires_at=NULL,
                   heartbeat_at=NULL,
                   updated_at=now()
             WHERE job_id=$1
               AND lease_token=$2
               AND status='LEASED'
            `,
			job.JobID,
			job.LeaseToken,
			string(nextStatus),
			failureCode,
			failureReason,
			int64(
				retryDelay/
					time.Second,
			),
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrProjectionJobLeaseLost
	}

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) ReplayDeadLetterProjectionJob(
	ctx context.Context,
	jobID string,
) error {
	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_jobs
               SET
                   status='PENDING',
                   attempt_count=0,
                   available_at=now(),
                   lease_owner=NULL,
                   lease_token=NULL,
                   lease_expires_at=NULL,
                   heartbeat_at=NULL,
                   failure_code=NULL,
                   failure_reason=NULL,
                   completed_at=NULL,
                   updated_at=now()
             WHERE job_id=$1
               AND status='DEAD_LETTER'
            `,
			jobID,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationStateConflict
	}

	return nil
}

type ProjectionJobRuntime struct {
	store ProjectionJobStore

	job ProjectionJob

	leaseDuration time.Duration
}

func (
	runtime *ProjectionJobRuntime,
) Heartbeat(
	ctx context.Context,
) error {
	return runtime.store.
		HeartbeatProjectionJob(
			ctx,
			runtime.job.JobID,
			runtime.job.LeaseToken,
			runtime.leaseDuration,
		)
}

func (
	runtime *ProjectionJobRuntime,
) Checkpoint(
	ctx context.Context,
	checkpoint string,
) error {
	return runtime.store.
		UpdateProjectionJobCheckpoint(
			ctx,
			runtime.job.JobID,
			runtime.job.LeaseToken,
			checkpoint,
		)
}

type ProjectionJobHandler interface {
	HandleProjectionJob(
		ctx context.Context,
		job ProjectionJob,
		runtime *ProjectionJobRuntime,
	) error
}

type ProjectionWorker struct {
	store ProjectionJobStore

	workerID string

	jobTypes []ProjectionJobType

	handler ProjectionJobHandler

	claimBatch int

	leaseDuration time.Duration
}

func NewProjectionWorker(
	store ProjectionJobStore,
	workerID string,
	jobTypes []ProjectionJobType,
	handler ProjectionJobHandler,
	claimBatch int,
	leaseDuration time.Duration,
) (
	*ProjectionWorker,
	error,
) {
	if store == nil ||
		strings.TrimSpace(workerID) == "" ||
		len(jobTypes) == 0 ||
		handler == nil ||
		claimBatch < 1 ||
		claimBatch > 50 ||
		leaseDuration < 5*time.Second ||
		leaseDuration > 10*time.Minute {
		return nil,
			ErrInvalidArgument
	}

	return &ProjectionWorker{
		store: store,

		workerID: strings.TrimSpace(
			workerID,
		),

		jobTypes: append(
			[]ProjectionJobType(nil),
			jobTypes...,
		),

		handler: handler,

		claimBatch: claimBatch,

		leaseDuration: leaseDuration,
	}, nil
}

func boundedRetryDelay(
	attempt int,
) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	delay :=
		5 * time.Second

	for value := 1; value < attempt &&
		delay < 5*time.Minute; value++ {
		delay *= 2
	}

	if delay > 5*time.Minute {
		delay =
			5 * time.Minute
	}

	return delay
}

func (
	worker *ProjectionWorker,
) Pass(
	ctx context.Context,
) error {
	if worker == nil {
		return ErrInvalidArgument
	}

	jobs, err :=
		worker.store.
			ClaimProjectionJobs(
				ctx,
				worker.workerID,
				worker.jobTypes,
				worker.claimBatch,
				worker.leaseDuration,
			)
	if err != nil {
		return err
	}

	var firstError error

	for _, job := range jobs {
		runtime :=
			&ProjectionJobRuntime{
				store: worker.store,

				job: job,

				leaseDuration: worker.leaseDuration,
			}

		handleErr :=
			worker.handler.
				HandleProjectionJob(
					ctx,
					job,
					runtime,
				)

		if handleErr == nil {
			if completeErr :=
				worker.store.
					CompleteProjectionJob(
						ctx,
						job.JobID,
						job.LeaseToken,
					); completeErr != nil &&
				firstError == nil {
				firstError =
					completeErr
			}

			continue
		}

		failErr :=
			worker.store.
				FailProjectionJob(
					ctx,
					job,
					"JOB_FAILED",
					handleErr.Error(),
					boundedRetryDelay(
						job.AttemptCount,
					),
				)

		if firstError == nil {
			firstError =
				handleErr
		}

		if failErr != nil &&
			firstError == nil {
			firstError =
				failErr
		}
	}

	return firstError
}

var (
	_ ProjectionJobStore = (*PostgresSearchGenerationStore)(nil)
)

var _ = errors.Is
var _ = pgx.ErrNoRows
