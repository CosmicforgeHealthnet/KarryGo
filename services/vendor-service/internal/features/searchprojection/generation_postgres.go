package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresSearchGenerationStore struct {
	pool *pgxpool.Pool
}

func NewPostgresSearchGenerationStore(
	pool *pgxpool.Pool,
) *PostgresSearchGenerationStore {
	return &PostgresSearchGenerationStore{
		pool: pool,
	}
}

func generationAdvisoryKey(
	namespace SearchCacheNamespace,
) string {
	return "search-generation:" +
		string(namespace)
}

func lockGenerationNamespace(
	ctx context.Context,
	tx pgx.Tx,
	namespace SearchCacheNamespace,
) error {
	_, err :=
		tx.Exec(
			ctx,
			`
            SELECT pg_advisory_xact_lock(
                hashtextextended($1, 0)
            )
            `,
			generationAdvisoryKey(
				namespace,
			),
		)

	return err
}

func (
	store *PostgresSearchGenerationStore,
) CurrentActiveGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) {
		return 0,
			ErrInvalidArgument
	}

	var generation int64

	err :=
		store.pool.QueryRow(
			ctx,
			`
            SELECT generation
              FROM search_projection_generations
             WHERE namespace_code=$1
               AND status='ACTIVE'
            `,
			string(namespace),
		).Scan(
			&generation,
		)

	if err == pgx.ErrNoRows {
		return 0,
			ErrGenerationNotFound
	}

	if err != nil {
		return 0, err
	}

	return generation, nil
}

func scanGeneration(
	row pgx.Row,
) (
	SearchProjectionGeneration,
	error,
) {
	var generation SearchProjectionGeneration

	var sourceRevisions []byte

	err :=
		row.Scan(
			&generation.Namespace,
			&generation.Generation,
			&generation.Status,
			&generation.BaseGeneration,
			&generation.Scope,
			&generation.ScopeRef,
			&sourceRevisions,
			&generation.DocumentCount,
			&generation.ProcessedCount,
			&generation.FailureCode,
			&generation.FailureReason,
			&generation.RequestedBy,
			&generation.StartedAt,
			&generation.CompletedAt,
			&generation.ValidatedAt,
			&generation.ActivatedAt,
			&generation.RetiredAt,
			&generation.WorkerID,
			&generation.LeaseToken,
			&generation.LeaseExpiresAt,
			&generation.HeartbeatAt,
			&generation.CreatedAt,
			&generation.UpdatedAt,
		)
	if err != nil {
		return SearchProjectionGeneration{},
			err
	}

	generation.SourceRevisions =
		append(
			json.RawMessage(nil),
			sourceRevisions...,
		)

	return generation, nil
}

const generationSelectColumns = `
    namespace_code,
    generation,
    status,
    base_generation,
    rebuild_scope,
    COALESCE(scope_ref, ''),
    source_revisions,
    document_count,
    processed_count,
    COALESCE(failure_code, ''),
    COALESCE(failure_reason, ''),
    requested_by,
    started_at,
    completed_at,
    validated_at,
    activated_at,
    retired_at,
    COALESCE(worker_id, ''),
    COALESCE(lease_token, ''),
    lease_expires_at,
    heartbeat_at,
    created_at,
    updated_at
`

func (
	store *PostgresSearchGenerationStore,
) GetGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) (
	SearchProjectionGeneration,
	error,
) {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		generation <= 0 {
		return SearchProjectionGeneration{},
			ErrInvalidArgument
	}

	value, err :=
		scanGeneration(
			store.pool.QueryRow(
				ctx,
				`
                SELECT `+
					generationSelectColumns+
					`
                  FROM search_projection_generations
                 WHERE namespace_code=$1
                   AND generation=$2
                `,
				string(namespace),
				generation,
			),
		)

	if err == pgx.ErrNoRows {
		return SearchProjectionGeneration{},
			ErrGenerationNotFound
	}

	return value, err
}

func (
	store *PostgresSearchGenerationStore,
) RequestFullRebuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	requestedBy string,
) (
	SearchProjectionGeneration,
	error,
) {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		strings.TrimSpace(requestedBy) == "" {
		return SearchProjectionGeneration{},
			ErrInvalidArgument
	}

	requestedBy =
		strings.TrimSpace(
			requestedBy,
		)

	tx, err :=
		store.pool.BeginTx(
			ctx,
			pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		)
	if err != nil {
		return SearchProjectionGeneration{},
			err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		lockGenerationNamespace(
			ctx,
			tx,
			namespace,
		); err != nil {
		return SearchProjectionGeneration{},
			err
	}

	var baseGeneration int64

	err =
		tx.QueryRow(
			ctx,
			`
            SELECT generation
              FROM search_projection_generations
             WHERE namespace_code=$1
               AND status='ACTIVE'
            `,
			string(namespace),
		).Scan(
			&baseGeneration,
		)

	if err == pgx.ErrNoRows {
		baseGeneration = 0
	} else if err != nil {
		return SearchProjectionGeneration{},
			err
	}

	var nextGeneration int64

	if err =
		tx.QueryRow(
			ctx,
			`
            SELECT
                COALESCE(
                    max(generation),
                    0
                ) + 1
              FROM search_projection_generations
             WHERE namespace_code=$1
            `,
			string(namespace),
		).Scan(
			&nextGeneration,
		); err != nil {
		return SearchProjectionGeneration{},
			err
	}

	_, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_generations (
                namespace_code,
                generation,
                status,
                base_generation,
                rebuild_scope,
                requested_by,
                started_at,
                created_at,
                updated_at
            )
            VALUES (
                $1,
                $2,
                'BUILDING',
                $3,
                'FULL',
                $4,
                now(),
                now(),
                now()
            )
            `,
			string(namespace),
			nextGeneration,
			baseGeneration,
			requestedBy,
		)
	if err != nil {
		return SearchProjectionGeneration{},
			err
	}

	jobID :=
		fmt.Sprintf(
			"search-rebuild-%s-%d",
			strings.ToLower(
				string(namespace),
			),
			nextGeneration,
		)

	_, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_jobs (
                job_id,
                job_type,
                namespace_code,
                generation,
                scope_type,
                status,
                priority,
                max_attempts,
                available_at,
                dedupe_key,
                created_at,
                updated_at
            )
            VALUES (
                $1,
                'FULL_REBUILD',
                $2,
                $3,
                'FULL',
                'PENDING',
                100,
                5,
                now(),
                $4,
                now(),
                now()
            )
            `,
			jobID,
			string(namespace),
			nextGeneration,
			"FULL_REBUILD:"+
				string(namespace)+
				":"+
				fmt.Sprintf(
					"%d",
					nextGeneration,
				),
		)
	if err != nil {
		return SearchProjectionGeneration{},
			err
	}

	_, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                to_status,
                document_count,
                processed_count
            )
            VALUES (
                $1,
                $2,
                'REQUESTED',
                'BUILDING',
                0,
                0
            )
            `,
			string(namespace),
			nextGeneration,
		)
	if err != nil {
		return SearchProjectionGeneration{},
			err
	}

	if err =
		tx.Commit(ctx); err != nil {
		return SearchProjectionGeneration{},
			err
	}

	return store.GetGeneration(
		ctx,
		namespace,
		nextGeneration,
	)
}

func (
	store *PostgresSearchGenerationStore,
) RequestScopedRebuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	scope SearchRebuildScope,
	scopeRef string,
	requestedBy string,
) (
	string,
	error,
) {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		(scope != SearchRebuildScopeBusiness &&
			scope != SearchRebuildScopeBranch) ||
		!validRebuildScope(
			scope,
			scopeRef,
		) ||
		strings.TrimSpace(requestedBy) == "" {
		return "",
			ErrInvalidArgument
	}

	activeGeneration, err :=
		store.CurrentActiveGeneration(
			ctx,
			namespace,
		)
	if err != nil {
		return "", err
	}

	scopeRef =
		strings.TrimSpace(
			scopeRef,
		)

	jobID :=
		fmt.Sprintf(
			"search-scoped-%s-%s-%d-%d",
			strings.ToLower(
				string(namespace),
			),
			strings.ToLower(
				string(scope),
			),
			activeGeneration,
			time.Now().
				UTC().
				UnixNano(),
		)

	_, err =
		store.pool.Exec(
			ctx,
			`
            INSERT INTO search_projection_jobs (
                job_id,
                job_type,
                namespace_code,
                generation,
                scope_type,
                scope_ref,
                reference_id,
                status,
                priority,
                max_attempts,
                available_at,
                dedupe_key,
                created_at,
                updated_at
            )
            VALUES (
                $1,
                'SCOPED_REBUILD',
                $2,
                $3,
                $4,
                $5,
                $6,
                'PENDING',
                100,
                5,
                now(),
                $7,
                now(),
                now()
            )
            `,
			jobID,
			string(namespace),
			activeGeneration,
			string(scope),
			scopeRef,
			strings.TrimSpace(
				requestedBy,
			),
			jobID,
		)
	if err != nil {
		return "", err
	}

	return jobID, nil
}

func (
	store *PostgresSearchGenerationStore,
) RecordGenerationProgress(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
	processedDelta int64,
	workerID string,
	sourceRevisions json.RawMessage,
) error {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		generation <= 0 ||
		processedDelta < 0 {
		return ErrInvalidArgument
	}

	if len(sourceRevisions) == 0 {
		sourceRevisions =
			json.RawMessage(`{}`)
	}

	if !json.Valid(sourceRevisions) {
		return ErrInvalidArgument
	}

	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   processed_count =
                       processed_count + $3,

                   source_revisions =
                       source_revisions || $4::jsonb,

                   worker_id =
                       NULLIF($5, ''),

                   heartbeat_at = now(),
                   updated_at = now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status='BUILDING'
            `,
			string(namespace),
			generation,
			processedDelta,
			sourceRevisions,
			strings.TrimSpace(
				workerID,
			),
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationStateConflict
	}

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) CompleteGenerationBuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		generation <= 0 {
		return ErrInvalidArgument
	}

	tableName := ""

	switch namespace {
	case SearchCacheNamespaceFood:
		tableName =
			"food_search_documents"

	case SearchCacheNamespaceShop:
		tableName =
			"shop_search_documents"

	default:
		return ErrInvalidArgument
	}

	// Completion, its evidence and the follow-on validate/activate job all
	// commit together.
	//
	// Before this was transactional, a crash between "build completed" and
	// "enqueue validation" left the generation BUILDING with nothing left to
	// drive it: the rebuild had finished, no job existed to validate it, and
	// the generation sat there permanently while the operator watched a
	// progress figure that had already reached its total. Committing the job
	// with the completion is what makes the pipeline self-driving rather than
	// dependent on someone noticing.
	tx, err :=
		store.pool.Begin(
			ctx,
		)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var documentCount int64

	if err =
		tx.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM `+
				tableName+
				`
             WHERE generation=$1
            `,
			generation,
		).Scan(
			&documentCount,
		); err != nil {
		return err
	}

	tag, err :=
		tx.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   document_count=$3,
                   completed_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status='BUILDING'
            `,
			string(namespace),
			generation,
			documentCount,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationStateConflict
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                from_status,
                to_status,
                document_count
            )
            VALUES (
                $1,
                $2,
                'BUILD_COMPLETED',
                'BUILDING',
                'BUILDING',
                $3
            )
            `,
			string(namespace),
			generation,
			documentCount,
		); err != nil {
		return err
	}

	// The generation does NOT become READY here. This only queues the
	// validation that may make it READY: MarkGenerationReady refuses unless
	// the documents actually present in the table match the count the worker
	// reported processing, which is the check that stops a half-built
	// generation being promoted.
	if _, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_jobs (
                job_id,
                job_type,
                namespace_code,
                generation,
                scope_type,
                status,
                priority,
                max_attempts,
                available_at,
                dedupe_key,
                created_at,
                updated_at
            )
            VALUES (
                $1,
                'GENERATION_VALIDATE_ACTIVATE',
                $2,
                $3,
                'FULL',
                'PENDING',
                50,
                5,
                now(),
                $4,
                now(),
                now()
            )
            ON CONFLICT (dedupe_key) DO NOTHING
            `,
			validateActivateJobID(
				namespace,
				generation,
			),
			string(namespace),
			generation,
			validateActivateDedupeKey(
				namespace,
				generation,
			),
		); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// validateActivateJobID and validateActivateDedupeKey are derived from the
// namespace and generation rather than random, so completing the same build
// twice - a duplicated worker pass, a replayed job - converges on one
// validation job instead of queueing a second.
func validateActivateJobID(
	namespace SearchCacheNamespace,
	generation int64,
) string {
	return fmt.Sprintf(
		"search-validate-%s-%d",
		strings.ToLower(
			string(namespace),
		),
		generation,
	)
}

func validateActivateDedupeKey(
	namespace SearchCacheNamespace,
	generation int64,
) string {
	return fmt.Sprintf(
		"GENERATION_VALIDATE_ACTIVATE:%s:%d",
		string(namespace),
		generation,
	)
}

func (
	store *PostgresSearchGenerationStore,
) MarkGenerationReady(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   status='READY',
                   validated_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status='BUILDING'
               AND completed_at IS NOT NULL
               AND document_count=processed_count
            `,
			string(namespace),
			generation,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationValidationFailed
	}

	_, _ =
		store.pool.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                from_status,
                to_status
            )
            VALUES (
                $1,
                $2,
                'VALIDATED',
                'BUILDING',
                'READY'
            )
            `,
			string(namespace),
			generation,
		)

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) MarkGenerationFailed(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
	failureCode string,
	failureReason string,
	workerID string,
) error {
	failureCode =
		strings.TrimSpace(
			failureCode,
		)

	failureReason =
		boundedFailureText(
			failureReason,
		)

	if len(failureCode) > 64 {
		failureCode =
			failureCode[:64]
	}

	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   status='FAILED',
                   failure_code=NULLIF($3, ''),
                   failure_reason=NULLIF($4, ''),
                   worker_id=NULLIF($5, ''),
                   lease_token=NULL,
                   lease_expires_at=NULL,
                   heartbeat_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status='BUILDING'
            `,
			string(namespace),
			generation,
			failureCode,
			failureReason,
			strings.TrimSpace(
				workerID,
			),
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationStateConflict
	}

	_, _ =
		store.pool.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                from_status,
                to_status,
                worker_id,
                failure_code,
                failure_reason
            )
            VALUES (
                $1,
                $2,
                'FAILED',
                'BUILDING',
                'FAILED',
                NULLIF($3, ''),
                NULLIF($4, ''),
                NULLIF($5, '')
            )
            `,
			string(namespace),
			generation,
			strings.TrimSpace(
				workerID,
			),
			failureCode,
			failureReason,
		)

	return nil
}

func (
	store *PostgresSearchGenerationStore,
) PrepareFailedGenerationRetry(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if store == nil ||
		store.pool == nil ||
		generation <= 0 {
		return ErrInvalidArgument
	}

	tx, err :=
		store.pool.BeginTx(
			ctx,
			pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		lockGenerationNamespace(
			ctx,
			tx,
			namespace,
		); err != nil {
		return err
	}

	var status string

	if err =
		tx.QueryRow(
			ctx,
			`
            SELECT status
              FROM search_projection_generations
             WHERE namespace_code=$1
               AND generation=$2
             FOR UPDATE
            `,
			string(namespace),
			generation,
		).Scan(
			&status,
		); err != nil {
		if err == pgx.ErrNoRows {
			return ErrGenerationNotFound
		}

		return err
	}

	if status !=
		string(
			SearchGenerationFailed,
		) {
		return ErrGenerationStateConflict
	}

	tableName := ""

	switch namespace {
	case SearchCacheNamespaceFood:
		tableName =
			"food_search_documents"

	case SearchCacheNamespaceShop:
		tableName =
			"shop_search_documents"

	default:
		return ErrInvalidArgument
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            DELETE FROM `+
				tableName+
				`
             WHERE generation=$1
            `,
			generation,
		); err != nil {
		return err
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   status='BUILDING',
                   source_revisions='{}'::jsonb,
                   document_count=0,
                   processed_count=0,
                   failure_code=NULL,
                   failure_reason=NULL,
                   completed_at=NULL,
                   validated_at=NULL,
                   worker_id=NULL,
                   lease_token=NULL,
                   lease_expires_at=NULL,
                   heartbeat_at=NULL,
                   started_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
            `,
			string(namespace),
			generation,
		); err != nil {
		return err
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                from_status,
                to_status
            )
            VALUES (
                $1,
                $2,
                'RETRY_REQUESTED',
                'FAILED',
                'BUILDING'
            )
            `,
			string(namespace),
			generation,
		); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ActivateGeneration promotes a READY generation to ACTIVE and retires the
// generation it was built from.
//
// Two sibling generations built from the same base racing to activate is a
// normal operational event, not an internal fault, and it has two possible
// endings. Usually the advisory namespace lock serialises them and the loser
// observes that the ACTIVE generation is no longer the base it was built from,
// which is the compare-and-swap below. But the transaction is SERIALIZABLE and
// takes its snapshot at the advisory-lock statement - before the winner
// commits - so the loser can instead be aborted by PostgreSQL with a
// serialization failure when it reaches the row the winner has since updated.
//
// Both endings mean the same thing: this activation lost a race and applied
// nothing. A serialization failure always aborts its transaction, so treating
// it as an activation conflict cannot mask a partial write. Reporting it as an
// opaque internal error instead would turn a routine race into a 500 on the
// projection administration API - and would hide that the invariant actually
// held.
//
// The isolation level is deliberately left at SERIALIZABLE rather than lowered
// to let the post-lock read see fresh rows: not every generation mutation in
// this store takes the advisory lock, so SERIALIZABLE is doing real work as
// the backstop for the paths that do not.
func (
	store *PostgresSearchGenerationStore,
) ActivateGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	err := store.activateGeneration(
		ctx,
		namespace,
		generation,
	)

	if isSerializationFailure(err) {
		return ErrGenerationActivationConflict
	}

	return err
}

// isSerializationFailure reports the two SQLSTATEs PostgreSQL uses for a
// transaction it aborted to preserve serializability: 40001 (serialization
// failure) and 40P01 (deadlock detected). Neither leaves any effect behind.
func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

func (
	store *PostgresSearchGenerationStore,
) activateGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if store == nil ||
		store.pool == nil ||
		!validSearchNamespace(namespace) ||
		generation <= 0 {
		return ErrInvalidArgument
	}

	tx, err :=
		store.pool.BeginTx(
			ctx,
			pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		lockGenerationNamespace(
			ctx,
			tx,
			namespace,
		); err != nil {
		return err
	}

	var targetStatus string
	var baseGeneration int64

	if err =
		tx.QueryRow(
			ctx,
			`
            SELECT
                status,
                base_generation
              FROM search_projection_generations
             WHERE namespace_code=$1
               AND generation=$2
             FOR UPDATE
            `,
			string(namespace),
			generation,
		).Scan(
			&targetStatus,
			&baseGeneration,
		); err != nil {
		if err == pgx.ErrNoRows {
			return ErrGenerationNotFound
		}

		return err
	}

	if targetStatus !=
		string(
			SearchGenerationReady,
		) {
		return ErrGenerationStateConflict
	}

	var activeGeneration int64

	err =
		tx.QueryRow(
			ctx,
			`
            SELECT generation
              FROM search_projection_generations
             WHERE namespace_code=$1
               AND status='ACTIVE'
             FOR UPDATE
            `,
			string(namespace),
		).Scan(
			&activeGeneration,
		)

	if err == pgx.ErrNoRows {
		activeGeneration = 0
	} else if err != nil {
		return err
	}

	// Compare-and-swap prevents two sibling rebuilds from both
	// activating after observing the same old generation.
	if activeGeneration !=
		baseGeneration {
		return ErrGenerationActivationConflict
	}

	if activeGeneration > 0 {
		if _, err =
			tx.Exec(
				ctx,
				`
                UPDATE search_projection_generations
                   SET
                       status='RETIRED',
                       retired_at=now(),
                       updated_at=now()
                 WHERE namespace_code=$1
                   AND generation=$2
                   AND status='ACTIVE'
                `,
				string(namespace),
				activeGeneration,
			); err != nil {
			return err
		}

		if _, err =
			tx.Exec(
				ctx,
				`
                INSERT INTO search_projection_generation_history (
                    namespace_code,
                    generation,
                    action,
                    from_status,
                    to_status
                )
                VALUES (
                    $1,
                    $2,
                    'RETIRED',
                    'ACTIVE',
                    'RETIRED'
                )
                `,
				string(namespace),
				activeGeneration,
			); err != nil {
			return err
		}
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   status='ACTIVE',
                   activated_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status='READY'
            `,
			string(namespace),
			generation,
		); err != nil {
		return err
	}

	if _, err =
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_generation_history (
                namespace_code,
                generation,
                action,
                from_status,
                to_status
            )
            VALUES (
                $1,
                $2,
                'ACTIVATED',
                'READY',
                'ACTIVE'
            )
            `,
			string(namespace),
			generation,
		); err != nil {
		return err
	}

	// Generation switch changes all cache identities.
	if _, err =
		tx.Exec(
			ctx,
			`
            UPDATE search_cache_revisions
               SET
                   revision=revision+1,
                   updated_at=now()
             WHERE namespace_code=$1
            `,
			string(namespace),
		); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (
	store *PostgresSearchGenerationStore,
) RetireGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	tag, err :=
		store.pool.Exec(
			ctx,
			`
            UPDATE search_projection_generations
               SET
                   status='RETIRED',
                   retired_at=now(),
                   updated_at=now()
             WHERE namespace_code=$1
               AND generation=$2
               AND status IN (
                   'READY',
                   'FAILED'
               )
            `,
			string(namespace),
			generation,
		)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrGenerationStateConflict
	}

	return nil
}

func boundedFailureText(
	value string,
) string {
	value =
		strings.TrimSpace(
			value,
		)

	runes :=
		[]rune(value)

	if len(runes) > 500 {
		runes =
			runes[:500]
	}

	return string(runes)
}

var _ SearchProjectionGenerationProvider = (*PostgresSearchGenerationStore)(nil)
