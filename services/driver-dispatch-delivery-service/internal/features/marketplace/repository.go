package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors. Handlers map these to stable API error codes.
var (
	ErrNotFound            = errors.New("marketplace: not found")
	ErrRequestAlreadyClaim = errors.New("marketplace: request already claimed")
	ErrIdempotencyConflict = errors.New("marketplace: idempotency key reused with a different request")
	ErrVersionMismatch     = errors.New("marketplace: expected version mismatch")
	ErrExecutionExists     = errors.New("marketplace: execution already exists")
)

// Querier lets repository methods run on a pool or inside a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository is the PostgreSQL adapter for organization marketplace state.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Pool exposes the pool for transaction management.
func (r *Repository) Pool() *pgxpool.Pool { return r.db }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func constraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// WithinTx runs fn inside a transaction, rolling back on error.
func (r *Repository) WithinTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("marketplace: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("marketplace: commit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Canonical request (request_broadcasts)
// ---------------------------------------------------------------------------

const broadcastColumns = `id, booking_id, status, reservation_status, reserved_organization_id,
	accepted_by_provider_id, source_version, expires_at, created_at, updated_at`

func scanBroadcast(row pgx.Row) (*Broadcast, error) {
	var b Broadcast
	err := row.Scan(&b.ID, &b.BookingID, &b.Status, &b.ReservationStatus, &b.ReservedOrganizationID,
		&b.AcceptedByProviderID, &b.SourceVersion, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan broadcast: %w", err)
	}
	return &b, nil
}

// LockBroadcast selects the canonical request FOR NO KEY UPDATE.
//
// Every reservation and execution decision is made under this lock, so two
// concurrent organizations serialize here rather than both reading a stale
// "available" state.
//
// The lock strength is deliberate. Child tables (command operations,
// reservations) carry foreign keys to request_broadcasts, so inserting into
// them takes an implicit FOR KEY SHARE lock on this row. FOR UPDATE conflicts
// with FOR KEY SHARE, which deadlocks the moment two organizations race: each
// holds KEY SHARE from its own child insert and then asks for FOR UPDATE.
// FOR NO KEY UPDATE excludes other reservers exactly the same way but is
// compatible with KEY SHARE, and it is honest about the fact that we never
// modify the row's key.
func (r *Repository) LockBroadcast(ctx context.Context, q Querier, requestID string) (*Broadcast, error) {
	return scanBroadcast(q.QueryRow(ctx,
		`SELECT `+broadcastColumns+` FROM request_broadcasts WHERE id = $1 FOR NO KEY UPDATE`, requestID))
}

// GetBroadcast reads the canonical request without locking.
func (r *Repository) GetBroadcast(ctx context.Context, q Querier, requestID string) (*Broadcast, error) {
	if q == nil {
		q = r.db
	}
	return scanBroadcast(q.QueryRow(ctx,
		`SELECT `+broadcastColumns+` FROM request_broadcasts WHERE id = $1`, requestID))
}

// SetBroadcastReservation updates reservation state and bumps the source
// version, guarded by a compare-and-swap on the version fleet expects.
func (r *Repository) SetBroadcastReservation(ctx context.Context, q Querier, requestID string,
	reservationStatus string, organizationID *string, expectedVersion int64) (int64, error) {
	var newVersion int64
	err := q.QueryRow(ctx, `
		UPDATE request_broadcasts SET
			reservation_status = $2,
			reserved_organization_id = $3,
			source_version = source_version + 1,
			updated_at = now()
		WHERE id = $1 AND source_version = $4
		RETURNING source_version`,
		requestID, reservationStatus, organizationID, expectedVersion).Scan(&newVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	if err != nil {
		return 0, fmt.Errorf("marketplace: set broadcast reservation: %w", err)
	}
	return newVersion, nil
}

// ---------------------------------------------------------------------------
// Reservations
// ---------------------------------------------------------------------------

const reservationColumns = `id, request_id, organization_id, status, acceptance_source,
	accepting_provider_id, organization_membership_id, source_request_version,
	idempotency_key, request_fingerprint, reserved_at, released_at, release_reason,
	version, created_at, updated_at`

func scanReservation(row pgx.Row) (*Reservation, error) {
	var res Reservation
	err := row.Scan(&res.ID, &res.RequestID, &res.OrganizationID, &res.Status, &res.AcceptanceSource,
		&res.AcceptingProviderID, &res.OrganizationMembershipID, &res.SourceRequestVersion,
		&res.IdempotencyKey, &res.RequestFingerprint, &res.ReservedAt, &res.ReleasedAt,
		&res.ReleaseReason, &res.Version, &res.CreatedAt, &res.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan reservation: %w", err)
	}
	return &res, nil
}

// CreateReservation inserts an organization's claim.
//
// The partial unique index uq_dispatch_reservation_live is what actually
// decides a race between two organizations: the loser gets a definite
// ErrRequestAlreadyClaim rather than an ambiguous outcome it would have to
// reconcile.
func (r *Repository) CreateReservation(ctx context.Context, q Querier, res *Reservation) error {
	now := time.Now().UTC()
	err := q.QueryRow(ctx, `
		INSERT INTO dispatch_organization_reservations
			(request_id, organization_id, status, acceptance_source, accepting_provider_id,
			 organization_membership_id, source_request_version, idempotency_key,
			 request_fingerprint, reserved_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, now(), now())
		RETURNING id, version, created_at, updated_at`,
		res.RequestID, res.OrganizationID, res.Status, res.AcceptanceSource, res.AcceptingProviderID,
		res.OrganizationMembershipID, res.SourceRequestVersion, res.IdempotencyKey,
		res.RequestFingerprint, now).
		Scan(&res.ID, &res.Version, &res.CreatedAt, &res.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			switch constraintName(err) {
			case "uq_dispatch_reservation_live":
				return ErrRequestAlreadyClaim
			case "uq_dispatch_reservation_idempotency":
				return ErrIdempotencyConflict
			}
			return ErrRequestAlreadyClaim
		}
		return fmt.Errorf("marketplace: create reservation: %w", err)
	}
	res.ReservedAt = &now
	return nil
}

// GetReservation loads a reservation by id.
func (r *Repository) GetReservation(ctx context.Context, q Querier, id string) (*Reservation, error) {
	if q == nil {
		q = r.db
	}
	return scanReservation(q.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM dispatch_organization_reservations WHERE id = $1`, id))
}

// LockReservation selects a reservation FOR UPDATE.
func (r *Repository) LockReservation(ctx context.Context, q Querier, id string) (*Reservation, error) {
	return scanReservation(q.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM dispatch_organization_reservations
		 WHERE id = $1 FOR UPDATE`, id))
}

// GetReservationByIdempotencyKey powers read-back after a lost response.
func (r *Repository) GetReservationByIdempotencyKey(ctx context.Context, q Querier, key string) (*Reservation, error) {
	if q == nil {
		q = r.db
	}
	return scanReservation(q.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM dispatch_organization_reservations
		 WHERE idempotency_key = $1`, key))
}

// GetLiveReservationForRequest returns the current holder of a request.
func (r *Repository) GetLiveReservationForRequest(ctx context.Context, q Querier, requestID string) (*Reservation, error) {
	if q == nil {
		q = r.db
	}
	return scanReservation(q.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM dispatch_organization_reservations
		 WHERE request_id = $1 AND status IN ('PENDING','RESERVED')`, requestID))
}

// ReleaseReservation moves a reservation out of its live state.
func (r *Repository) ReleaseReservation(ctx context.Context, q Querier, id string,
	reason string, expectedVersion int64) (*Reservation, error) {
	row := q.QueryRow(ctx, `
		UPDATE dispatch_organization_reservations SET
			status = 'RELEASED',
			released_at = now(),
			release_reason = $2,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $3 AND status IN ('PENDING','RESERVED')
		RETURNING `+reservationColumns, id, reason, expectedVersion)
	res, err := scanReservation(row)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrVersionMismatch
	}
	return res, err
}

// ---------------------------------------------------------------------------
// Command operations
// ---------------------------------------------------------------------------

const commandColumns = `id, operation_type, request_id, trip_id, fleet_assignment_id,
	idempotency_key, request_fingerprint, status, response_snapshot, error_code,
	attempt_count, created_at, updated_at`

func scanCommand(row pgx.Row) (*CommandOperation, error) {
	var op CommandOperation
	err := row.Scan(&op.ID, &op.OperationType, &op.RequestID, &op.TripID, &op.FleetAssignmentID,
		&op.IdempotencyKey, &op.RequestFingerprint, &op.Status, &op.ResponseSnapshot,
		&op.ErrorCode, &op.AttemptCount, &op.CreatedAt, &op.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan command: %w", err)
	}
	return &op, nil
}

// BeginCommand starts or replays a command operation.
//
// It returns replayed=true when this idempotency key has been seen before with
// the same fingerprint, so the caller can answer from the stored snapshot
// instead of executing again. ON CONFLICT DO NOTHING is used rather than
// catching a unique violation, because a raised constraint error would abort
// the surrounding transaction and the read-back below runs inside it.
func (r *Repository) BeginCommand(ctx context.Context, q Querier, op *CommandOperation) (*CommandOperation, bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO dispatch_marketplace_command_operations
			(operation_type, request_id, trip_id, fleet_assignment_id, idempotency_key,
			 request_fingerprint, status, attempt_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', 0, now(), now())
		ON CONFLICT (operation_type, idempotency_key) DO NOTHING`,
		op.OperationType, op.RequestID, op.TripID, op.FleetAssignmentID,
		op.IdempotencyKey, op.RequestFingerprint)
	if err != nil {
		return nil, false, fmt.Errorf("marketplace: begin command: %w", err)
	}
	if tag.RowsAffected() == 1 {
		created, err := r.GetCommand(ctx, q, op.OperationType, op.IdempotencyKey)
		return created, false, err
	}

	existing, err := r.GetCommand(ctx, q, op.OperationType, op.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if existing.RequestFingerprint != op.RequestFingerprint {
		return existing, true, ErrIdempotencyConflict
	}
	return existing, true, nil
}

// GetCommand loads a command operation by its natural key.
func (r *Repository) GetCommand(ctx context.Context, q Querier, operationType string, key string) (*CommandOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanCommand(q.QueryRow(ctx,
		`SELECT `+commandColumns+` FROM dispatch_marketplace_command_operations
		 WHERE operation_type = $1 AND idempotency_key = $2`, operationType, key))
}

// GetCommandByAssignment powers execution read-back by fleet assignment.
func (r *Repository) GetCommandByAssignment(ctx context.Context, q Querier, operationType string, assignmentID string) (*CommandOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanCommand(q.QueryRow(ctx,
		`SELECT `+commandColumns+` FROM dispatch_marketplace_command_operations
		 WHERE operation_type = $1 AND fleet_assignment_id = $2
		 ORDER BY created_at DESC LIMIT 1`, operationType, assignmentID))
}

// CompleteCommand stores the stable outcome so replays are answered from disk.
func (r *Repository) CompleteCommand(ctx context.Context, q Querier, id string,
	status string, snapshot any, errorCode string, tripID *string) error {
	var encoded []byte
	if snapshot != nil {
		var err error
		encoded, err = json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("marketplace: encode command snapshot: %w", err)
		}
	}
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_marketplace_command_operations SET
			status = $2,
			response_snapshot = COALESCE($3, response_snapshot),
			error_code = $4,
			trip_id = COALESCE($5, trip_id),
			attempt_count = attempt_count + 1,
			updated_at = now()
		WHERE id = $1`, id, status, encoded, code, tripID)
	if err != nil {
		return fmt.Errorf("marketplace: complete command: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outbox
// ---------------------------------------------------------------------------

// PublishEvent inserts an outbox row in the caller's transaction.
//
// Being in the same transaction as the state change is the whole point: an
// event can never describe a change that was rolled back, and a committed
// change can never silently fail to produce its event.
func (r *Repository) PublishEvent(ctx context.Context, q Querier, eventType string,
	aggregateType string, aggregateID string, aggregateVersion int64,
	correlationID string, causationID string, payload any) error {
	return r.PublishEventTo(ctx, q, "fleet-partner-service", eventType, aggregateType,
		aggregateID, aggregateVersion, correlationID, causationID, payload)
}

func (r *Repository) PublishEventTo(ctx context.Context, q Querier, targetService,
	eventType string, aggregateType string, aggregateID string, aggregateVersion int64,
	correlationID string, causationID string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marketplace: encode event payload: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO dispatch_marketplace_outbox
			(event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
			 payload, payload_hash, correlation_id, causation_id, status, attempt_count,
			 next_attempt_at, created_at, target_service)
		VALUES ($1, 1, $2, $3, $4, $5, $6, $7, $8, 'PENDING', 0, now(), now(), $9)`,
		eventType, aggregateType, aggregateID, aggregateVersion, encoded,
		PayloadHash(encoded), correlationID, causationID, targetService)
	if err != nil {
		return fmt.Errorf("marketplace: publish event: %w", err)
	}
	return nil
}

// OutboxRow is a claimed event awaiting delivery.
type OutboxRow struct {
	LeaseOwner       string
	ID               string
	EventType        string
	SchemaVersion    int
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	Payload          []byte
	PayloadHash      string
	CorrelationID    string
	CausationID      string
	AttemptCount     int
	CreatedAt        time.Time
}

// ClaimOutbox leases pending events for delivery.
//
// FOR UPDATE SKIP LOCKED lets several worker instances run concurrently
// without any of them blocking on rows another has already taken.
func (r *Repository) ClaimOutbox(ctx context.Context, q Querier, owner string,
	lease time.Duration, limit int) ([]OutboxRow, error) {
	return r.ClaimOutboxForTarget(ctx, q, owner, "fleet-partner-service", lease, limit)
}

func (r *Repository) ClaimOutboxForTarget(ctx context.Context, q Querier, owner,
	targetService string, lease time.Duration, limit int) ([]OutboxRow, error) {
	owner += ":" + uuid.NewString()
	rows, err := q.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM dispatch_marketplace_outbox
			WHERE status IN ('PENDING','FAILED') AND next_attempt_at <= now()
			  AND target_service = $4
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE dispatch_marketplace_outbox o SET
			status = 'IN_FLIGHT',
			lease_owner = $1,
			lease_expires_at = now() + make_interval(secs => $2)
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING o.id, o.event_type, o.schema_version, o.aggregate_type, o.aggregate_id,
			o.aggregate_version, o.payload, o.payload_hash, o.correlation_id, o.causation_id,
			o.attempt_count, o.created_at`,
		owner, lease.Seconds(), limit, targetService)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim outbox: %w", err)
	}
	defer rows.Close()

	var claimed []OutboxRow
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.ID, &row.EventType, &row.SchemaVersion, &row.AggregateType,
			&row.AggregateID, &row.AggregateVersion, &row.Payload, &row.PayloadHash,
			&row.CorrelationID, &row.CausationID, &row.AttemptCount, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan outbox: %w", err)
		}
		row.LeaseOwner = owner
		claimed = append(claimed, row)
	}
	return claimed, rows.Err()
}

// RecoverExpiredLeases returns abandoned in-flight events to the queue.
func (r *Repository) RecoverExpiredLeases(ctx context.Context, q Querier) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE dispatch_marketplace_outbox SET
			status = 'PENDING', lease_owner = NULL, lease_expires_at = NULL
		WHERE status = 'IN_FLIGHT' AND lease_expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("marketplace: recover leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// MarkPublished records a successful delivery.
func (r *Repository) MarkPublished(ctx context.Context, q Querier, id string, leaseOwner ...string) error {
	owner := ""
	if len(leaseOwner) > 0 {
		owner = leaseOwner[0]
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_marketplace_outbox SET
			status = 'PUBLISHED', published_at = now(), lease_owner = NULL,
			lease_expires_at = NULL, last_error = NULL
		WHERE id = $1 AND ($2='' OR (status='IN_FLIGHT' AND lease_owner=$2))`, id, owner)
	if err != nil {
		return fmt.Errorf("marketplace: mark published: %w", err)
	}
	return nil
}

// MarkDeliveryFailed schedules a retry or moves the event to dead letter.
func (r *Repository) MarkDeliveryFailed(ctx context.Context, q Querier, id string,
	lastError string, backoff time.Duration, maxAttempts int, leaseOwner ...string) error {
	owner := ""
	if len(leaseOwner) > 0 {
		owner = leaseOwner[0]
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_marketplace_outbox SET
			status = CASE WHEN attempt_count + 1 >= $4 THEN 'DEAD_LETTER' ELSE 'FAILED' END,
			attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => $3),
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $2
		WHERE id = $1 AND ($5='' OR (status='IN_FLIGHT' AND lease_owner=$5))`, id, lastError, backoff.Seconds(), maxAttempts, owner)
	if err != nil {
		return fmt.Errorf("marketplace: mark delivery failed: %w", err)
	}
	return nil
}

// RecordDeliveryAttempt appends to the delivery history.
func (r *Repository) RecordDeliveryAttempt(ctx context.Context, q Querier, outboxID string,
	target string, attempt int, httpStatus int, errorCode string, duration time.Duration) error {
	var status *int
	if httpStatus > 0 {
		status = &httpStatus
	}
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		INSERT INTO dispatch_marketplace_outbox_deliveries
			(outbox_id, target_service, attempt_number, http_status, error_code, duration_ms, attempted_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (outbox_id, target_service, attempt_number) DO NOTHING`,
		outboxID, target, attempt, status, code, duration.Milliseconds())
	if err != nil {
		return fmt.Errorf("marketplace: record delivery attempt: %w", err)
	}
	return nil
}

// CountOutboxByStatus supports metrics and smoke tests.
func (r *Repository) CountOutboxByStatus(ctx context.Context, status string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_marketplace_outbox WHERE status = $1`, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("marketplace: count outbox: %w", err)
	}
	return count, nil
}

// GetOutboxByID loads a single outbox event.
func (r *Repository) GetOutboxByID(ctx context.Context, q Querier, id string) (*OutboxRow, error) {
	if q == nil {
		q = r.db
	}
	var row OutboxRow
	err := q.QueryRow(ctx, `
		SELECT id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
			payload, payload_hash, correlation_id, causation_id, attempt_count, created_at
		FROM dispatch_marketplace_outbox WHERE id = $1`, id).
		Scan(&row.ID, &row.EventType, &row.SchemaVersion, &row.AggregateType, &row.AggregateID,
			&row.AggregateVersion, &row.Payload, &row.PayloadHash, &row.CorrelationID,
			&row.CausationID, &row.AttemptCount, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: get outbox: %w", err)
	}
	return &row, nil
}

// ReplayDeadLetterEvent returns a dead-lettered event to the queue.
//
// The event id and payload are untouched, so an operator replay is a
// redelivery of the original event rather than a new one.
func (r *Repository) ReplayDeadLetterEvent(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_marketplace_outbox SET
			status = 'PENDING', attempt_count = 0, next_attempt_at = now(), last_error = NULL
		WHERE id = $1 AND status = 'DEAD_LETTER'`, id)
	if err != nil {
		return fmt.Errorf("marketplace: replay dead letter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOutboxByAggregate returns events for one aggregate, oldest first.
func (r *Repository) ListOutboxByAggregate(ctx context.Context, aggregateType string,
	aggregateID string) ([]OutboxRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
			payload, payload_hash, correlation_id, causation_id, attempt_count, created_at
		FROM dispatch_marketplace_outbox
		WHERE aggregate_type = $1 AND aggregate_id = $2
		ORDER BY aggregate_version, created_at, id`, aggregateType, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list outbox by aggregate: %w", err)
	}
	defer rows.Close()

	var events []OutboxRow
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.ID, &row.EventType, &row.SchemaVersion, &row.AggregateType,
			&row.AggregateID, &row.AggregateVersion, &row.Payload, &row.PayloadHash,
			&row.CorrelationID, &row.CausationID, &row.AttemptCount, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan outbox: %w", err)
		}
		events = append(events, row)
	}
	return events, rows.Err()
}
