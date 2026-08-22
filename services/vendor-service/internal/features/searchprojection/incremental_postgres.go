package searchprojection

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresSearchCacheRevisionProvider struct {
	pool *pgxpool.Pool
}

func NewPostgresSearchCacheRevisionProvider(
	pool *pgxpool.Pool,
) *PostgresSearchCacheRevisionProvider {
	return &PostgresSearchCacheRevisionProvider{
		pool: pool,
	}
}

func (
	provider *PostgresSearchCacheRevisionProvider,
) CurrentSearchCacheRevision(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	if provider == nil ||
		provider.pool == nil {
		return 0,
			ErrInvalidArgument
	}

	if namespace != SearchCacheNamespaceFood &&
		namespace != SearchCacheNamespaceShop {
		return 0,
			ErrInvalidArgument
	}

	var revision int64

	err :=
		provider.pool.QueryRow(
			ctx,
			`
            SELECT revision
              FROM search_cache_revisions
             WHERE namespace_code=$1
            `,
			string(namespace),
		).Scan(
			&revision,
		)
	if err != nil {
		return 0, err
	}

	return revision, nil
}

type IncrementalProjectionProcessResult struct {
	Applied   bool
	Duplicate bool
	Stale     bool

	Outcome string

	CacheRevision int64
}

type PostgresIncrementalProjectionProcessor struct {
	pool *pgxpool.Pool

	source AuthoritativeIncrementalProjectionSource

	applier IncrementalProjectionApplier
}

func NewPostgresIncrementalProjectionProcessor(
	pool *pgxpool.Pool,
	source AuthoritativeIncrementalProjectionSource,
	applier IncrementalProjectionApplier,
) *PostgresIncrementalProjectionProcessor {
	return &PostgresIncrementalProjectionProcessor{
		pool: pool,

		source: source,

		applier: applier,
	}
}

type existingProjectionEvent struct {
	PayloadHash string

	Outcome string

	CacheRevision int64
}

func lockProjectionEvent(
	ctx context.Context,
	tx pgx.Tx,
	event SearchProjectionEvent,
) error {
	// Event identity lock prevents same event ID from racing
	// through different source aggregates.
	if _, err :=
		tx.Exec(
			ctx,
			`
            SELECT pg_advisory_xact_lock(
                hashtextextended($1, 0)
            )
            `,
			"search-event:"+
				event.EventID,
		); err != nil {
		return err
	}

	// Source lock serializes revisions for one authoritative
	// aggregate without blocking unrelated products/items.
	_, err :=
		tx.Exec(
			ctx,
			`
            SELECT pg_advisory_xact_lock(
                hashtextextended($1, 0)
            )
            `,
			"search-source:"+
				string(event.Namespace)+
				":"+
				event.SourceKind+
				":"+
				event.SourceRef,
		)

	return err
}

func existingProjectionEventTx(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
) (
	existingProjectionEvent,
	bool,
	error,
) {
	var existing existingProjectionEvent

	err :=
		tx.QueryRow(
			ctx,
			`
            SELECT
                payload_hash,
                outcome,
                cache_revision
              FROM search_projection_event_ledger
             WHERE event_id=$1
            `,
			eventID,
		).Scan(
			&existing.PayloadHash,
			&existing.Outcome,
			&existing.CacheRevision,
		)

	if err == pgx.ErrNoRows {
		return existing,
			false,
			nil
	}

	if err != nil {
		return existing,
			false,
			err
	}

	return existing,
		true,
		nil
}

func latestProjectionCheckpointTx(
	ctx context.Context,
	tx pgx.Tx,
	event SearchProjectionEvent,
) (
	int64,
	bool,
	error,
) {
	var latestRevision int64

	err :=
		tx.QueryRow(
			ctx,
			`
            SELECT latest_revision
              FROM search_projection_source_checkpoints
             WHERE namespace_code=$1
               AND source_kind=$2
               AND source_ref=$3
            `,
			string(event.Namespace),
			event.SourceKind,
			event.SourceRef,
		).Scan(
			&latestRevision,
		)

	if err == pgx.ErrNoRows {
		return 0,
			false,
			nil
	}

	if err != nil {
		return 0,
			false,
			err
	}

	return latestRevision,
		true,
		nil
}

func currentCacheRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	var revision int64

	err :=
		tx.QueryRow(
			ctx,
			`
            SELECT revision
              FROM search_cache_revisions
             WHERE namespace_code=$1
             FOR UPDATE
            `,
			string(namespace),
		).Scan(
			&revision,
		)

	return revision,
		err
}

func bumpCacheRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	var revision int64

	err :=
		tx.QueryRow(
			ctx,
			`
            UPDATE search_cache_revisions
               SET
                   revision=revision+1,
                   updated_at=now()
             WHERE namespace_code=$1
            RETURNING revision
            `,
			string(namespace),
		).Scan(
			&revision,
		)

	return revision,
		err
}

func insertProjectionLedgerTx(
	ctx context.Context,
	tx pgx.Tx,
	event SearchProjectionEvent,
	payloadHash string,
	outcome string,
	cacheRevision int64,
) error {
	_, err :=
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_event_ledger (
                event_id,
                namespace_code,
                event_type,
                source_kind,
                source_ref,
                source_revision,
                generation,
                payload_hash,
                outcome,
                cache_revision,
                created_at,
                processed_at
            )
            VALUES (
                $1,
                $2,
                $3,
                $4,
                $5,
                $6,
                $7,
                $8,
                $9,
                $10,
                now(),
                now()
            )
            `,
			event.EventID,
			string(event.Namespace),
			string(event.EventType),
			event.SourceKind,
			event.SourceRef,
			event.SourceRevision,
			event.Generation,
			payloadHash,
			outcome,
			cacheRevision,
		)

	return err
}

func upsertProjectionCheckpointTx(
	ctx context.Context,
	tx pgx.Tx,
	event SearchProjectionEvent,
) error {
	_, err :=
		tx.Exec(
			ctx,
			`
            INSERT INTO search_projection_source_checkpoints (
                namespace_code,
                source_kind,
                source_ref,
                latest_revision,
                last_event_id,
                updated_at
            )
            VALUES (
                $1,
                $2,
                $3,
                $4,
                $5,
                now()
            )
            ON CONFLICT (
                namespace_code,
                source_kind,
                source_ref
            )
            DO UPDATE SET
                latest_revision=EXCLUDED.latest_revision,
                last_event_id=EXCLUDED.last_event_id,
                updated_at=now()
            WHERE
                search_projection_source_checkpoints.latest_revision
                <
                EXCLUDED.latest_revision
            `,
			string(event.Namespace),
			event.SourceKind,
			event.SourceRef,
			event.SourceRevision,
			event.EventID,
		)

	return err
}

func resultForExistingEvent(
	existing existingProjectionEvent,
	payloadHash string,
) (
	IncrementalProjectionProcessResult,
	error,
) {
	if existing.PayloadHash !=
		payloadHash {
		return IncrementalProjectionProcessResult{},
			ErrProjectionEventCollision
	}

	return IncrementalProjectionProcessResult{
		Applied: false,

		Duplicate: true,

		Stale: existing.Outcome ==
			"STALE",

		Outcome: existing.Outcome,

		CacheRevision: existing.CacheRevision,
	}, nil
}

func (
	processor *PostgresIncrementalProjectionProcessor,
) preflight(
	ctx context.Context,
	event SearchProjectionEvent,
	payloadHash string,
) (
	IncrementalProjectionProcessResult,
	bool,
	error,
) {
	tx, err :=
		processor.pool.BeginTx(
			ctx,
			pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			false,
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		lockProjectionEvent(
			ctx,
			tx,
			event,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			false,
			err
	}

	existing,
		exists,
		err :=
		existingProjectionEventTx(
			ctx,
			tx,
			event.EventID,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			false,
			err
	}

	if exists {
		result, resultErr :=
			resultForExistingEvent(
				existing,
				payloadHash,
			)

		if resultErr != nil {
			return IncrementalProjectionProcessResult{},
				false,
				resultErr
		}

		if err =
			tx.Commit(ctx); err != nil {
			return IncrementalProjectionProcessResult{},
				false,
				err
		}

		return result,
			false,
			nil
	}

	latest,
		hasCheckpoint,
		err :=
		latestProjectionCheckpointTx(
			ctx,
			tx,
			event,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			false,
			err
	}

	if hasCheckpoint &&
		event.SourceRevision <= latest {
		cacheRevision, cacheErr :=
			currentCacheRevisionTx(
				ctx,
				tx,
				event.Namespace,
			)
		if cacheErr != nil {
			return IncrementalProjectionProcessResult{},
				false,
				cacheErr
		}

		if err =
			insertProjectionLedgerTx(
				ctx,
				tx,
				event,
				payloadHash,
				"STALE",
				cacheRevision,
			); err != nil {
			return IncrementalProjectionProcessResult{},
				false,
				err
		}

		if err =
			tx.Commit(ctx); err != nil {
			return IncrementalProjectionProcessResult{},
				false,
				err
		}

		return IncrementalProjectionProcessResult{
				Applied: false,

				Stale: true,

				Outcome: "STALE",

				CacheRevision: cacheRevision,
			},
			false,
			nil
	}

	if err =
		tx.Commit(ctx); err != nil {
		return IncrementalProjectionProcessResult{},
			false,
			err
	}

	return IncrementalProjectionProcessResult{},
		true,
		nil
}

func (
	processor *PostgresIncrementalProjectionProcessor,
) Process(
	ctx context.Context,
	event SearchProjectionEvent,
) (
	IncrementalProjectionProcessResult,
	error,
) {
	if processor == nil ||
		processor.pool == nil ||
		processor.source == nil ||
		processor.applier == nil {
		return IncrementalProjectionProcessResult{},
			ErrInvalidArgument
	}

	event.EventID =
		stringsTrimSpace(event.EventID)

	event.SourceKind =
		stringsTrimSpace(event.SourceKind)

	event.SourceRef =
		stringsTrimSpace(event.SourceRef)

	if err :=
		validateProjectionEvent(
			event,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	payloadHash, err :=
		canonicalProjectionEventHash(
			event,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	preflightResult,
		proceed,
		err :=
		processor.preflight(
			ctx,
			event,
			payloadHash,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if !proceed {
		return preflightResult,
			nil
	}

	// Resolve from authoritative domain AFTER preflight.
	// The event payload itself is not trusted as product/stock
	// authority.
	updates, err :=
		processor.source.
			ResolveIncrementalProjection(
				ctx,
				event,
			)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if err =
		validateIncrementalProjectionUpdates(
			event,
			updates,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	tx, err :=
		processor.pool.BeginTx(
			ctx,
			pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		lockProjectionEvent(
			ctx,
			tx,
			event,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	// Recheck after authoritative resolve because another
	// worker may have applied the event/newer revision.
	existing,
		exists,
		err :=
		existingProjectionEventTx(
			ctx,
			tx,
			event.EventID,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if exists {
		result, resultErr :=
			resultForExistingEvent(
				existing,
				payloadHash,
			)
		if resultErr != nil {
			return IncrementalProjectionProcessResult{},
				resultErr
		}

		if err =
			tx.Commit(ctx); err != nil {
			return IncrementalProjectionProcessResult{},
				err
		}

		return result, nil
	}

	latest,
		hasCheckpoint,
		err :=
		latestProjectionCheckpointTx(
			ctx,
			tx,
			event,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if hasCheckpoint &&
		event.SourceRevision <= latest {
		cacheRevision, cacheErr :=
			currentCacheRevisionTx(
				ctx,
				tx,
				event.Namespace,
			)
		if cacheErr != nil {
			return IncrementalProjectionProcessResult{},
				cacheErr
		}

		if err =
			insertProjectionLedgerTx(
				ctx,
				tx,
				event,
				payloadHash,
				"STALE",
				cacheRevision,
			); err != nil {
			return IncrementalProjectionProcessResult{},
				err
		}

		if err =
			tx.Commit(ctx); err != nil {
			return IncrementalProjectionProcessResult{},
				err
		}

		return IncrementalProjectionProcessResult{
			Stale: true,

			Outcome: "STALE",

			CacheRevision: cacheRevision,
		}, nil
	}

	// The projection application may use its own PostgreSQL
	// operations. 13D/13F writes are revision-aware and
	// idempotent, so a retry after an interrupted processor
	// remains safe.
	if err =
		processor.applier.
			ApplyIncrementalProjection(
				ctx,
				event,
				updates,
			); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if err =
		upsertProjectionCheckpointTx(
			ctx,
			tx,
			event,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	cacheRevision, err :=
		bumpCacheRevisionTx(
			ctx,
			tx,
			event.Namespace,
		)
	if err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if err =
		insertProjectionLedgerTx(
			ctx,
			tx,
			event,
			payloadHash,
			"APPLIED",
			cacheRevision,
		); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	if err =
		tx.Commit(ctx); err != nil {
		return IncrementalProjectionProcessResult{},
			err
	}

	return IncrementalProjectionProcessResult{
		Applied: true,

		Outcome: "APPLIED",

		CacheRevision: cacheRevision,
	}, nil
}

func stringsTrimSpace(
	value string,
) string {
	start := 0
	end := len(value)

	for start < end {
		switch value[start] {
		case ' ', '\t', '\r', '\n':
			start++

		default:
			goto trimEnd
		}
	}

trimEnd:

	for end > start {
		switch value[end-1] {
		case ' ', '\t', '\r', '\n':
			end--

		default:
			return value[start:end]
		}
	}

	return value[start:end]
}

var _ SearchCacheRevisionProvider = (*PostgresSearchCacheRevisionProvider)(nil)
