package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Relay operation types.
const (
	RelayProviderOfferAccept       = "PROVIDER_OFFER_ACCEPT"
	RelayProviderOfferDecline      = "PROVIDER_OFFER_DECLINE"
	RelayProviderAssignmentAccept  = "PROVIDER_ASSIGNMENT_ACCEPT"
	RelayProviderAssignmentDecline = "PROVIDER_ASSIGNMENT_DECLINE"
)

// Relay statuses.
const (
	RelayPending        = "PENDING"
	RelayDelivering     = "DELIVERING"
	RelayConfirmed      = "CONFIRMED"
	RelayRejected       = "REJECTED"
	RelayUnknownOutcome = "UNKNOWN_OUTCOME"
	RelayRetryScheduled = "RETRY_SCHEDULED"
	RelayDeadLetter     = "DEAD_LETTER"
)

// RelayOperation is the durable record of a provider's decision and of our
// attempts to deliver it to fleet.
//
// It exists because a provider's Accept or Decline is a decision a human made.
// A single synchronous HTTP call is not an acceptable record of it: if fleet
// is briefly unreachable the decision must survive, and when we retry it must
// be delivered as the SAME decision rather than a second one.
type RelayOperation struct {
	ID                 string
	OperationType      string
	ProviderID         string
	RequestID          *string
	OfferID            *string
	AssignmentID       *string
	IdempotencyKey     string
	RequestFingerprint string
	Payload            []byte
	PayloadHash        string
	Status             string
	AttemptCount       int
	NextAttemptAt      time.Time
	LeaseOwner         *string
	LeaseExpiresAt     *time.Time
	ResponseSnapshot   []byte
	LastError          *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

// Terminal reports whether no further delivery attempt will be made.
func (o RelayOperation) Terminal() bool {
	return o.Status == RelayConfirmed || o.Status == RelayRejected || o.Status == RelayDeadLetter
}

// NeedsReadBack reports whether the outcome must be resolved by asking fleet
// what happened rather than by re-sending the decision.
func (o RelayOperation) NeedsReadBack() bool {
	return o.Status == RelayUnknownOutcome || o.Status == RelayDelivering
}

const relayColumns = `id, operation_type, provider_id, request_id, offer_id, assignment_id,
	idempotency_key, request_fingerprint, payload, payload_hash, status, attempt_count,
	next_attempt_at, lease_owner, lease_expires_at, response_snapshot, last_error,
	created_at, updated_at, completed_at`

func scanRelay(row pgx.Row) (*RelayOperation, error) {
	var op RelayOperation
	err := row.Scan(&op.ID, &op.OperationType, &op.ProviderID, &op.RequestID, &op.OfferID,
		&op.AssignmentID, &op.IdempotencyKey, &op.RequestFingerprint, &op.Payload,
		&op.PayloadHash, &op.Status, &op.AttemptCount, &op.NextAttemptAt, &op.LeaseOwner,
		&op.LeaseExpiresAt, &op.ResponseSnapshot, &op.LastError,
		&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan relay: %w", err)
	}
	return &op, nil
}

// CreateRelay records a provider decision for durable delivery.
//
// Returns replayed=true when this idempotency key already exists with the same
// fingerprint — a retried provider tap re-delivers the ORIGINAL decision rather
// than creating a second one. A different fingerprint under the same key is a
// conflict, because it would mean two different decisions claiming one identity.
func (r *Repository) CreateRelay(ctx context.Context, q Querier, op *RelayOperation) (*RelayOperation, bool, error) {
	if len(op.Payload) == 0 {
		op.Payload = []byte(`{}`)
	}
	if op.PayloadHash == "" {
		op.PayloadHash = PayloadHash(op.Payload)
	}

	tag, err := q.Exec(ctx, `
		INSERT INTO dispatch_fleet_relay_operations
			(operation_type, provider_id, request_id, offer_id, assignment_id,
			 idempotency_key, request_fingerprint, payload, payload_hash, status,
			 attempt_count, next_attempt_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING', 0, now(), now(), now())
		-- Untargeted: this must absorb a collision on EITHER unique index,
		-- the per-type key or uq_dispatch_relay_single_decision. A targeted
		-- clause would let a cross-type collision raise and abort the caller's
		-- transaction instead of being reported as a conflict.
		ON CONFLICT DO NOTHING`,
		op.OperationType, op.ProviderID, op.RequestID, op.OfferID, op.AssignmentID,
		op.IdempotencyKey, op.RequestFingerprint, op.Payload, op.PayloadHash)
	if err != nil {
		return nil, false, fmt.Errorf("marketplace: create relay: %w", err)
	}

	if tag.RowsAffected() == 1 {
		// Read back by (provider, key): the key alone is no longer unique
		// across providers, so an unscoped read could return another
		// provider's relay.
		stored, err := r.GetRelayByKeyAnyType(ctx, q, op.ProviderID, op.IdempotencyKey)
		if err != nil {
			return nil, false, err
		}
		return stored, false, nil
	}

	// Nothing was inserted. Either this exact decision already exists (a
	// replay) or the same client response id already carries a DIFFERENT
	// decision. Look the key up across operation types, because
	// uq_dispatch_relay_single_decision spans them.
	stored, err := r.GetRelayByKeyAnyType(ctx, q, op.ProviderID, op.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if stored.OperationType != op.OperationType ||
		stored.RequestFingerprint != op.RequestFingerprint {
		return stored, true, ErrIdempotencyConflict
	}
	return stored, true, nil
}

// GetRelayByIdempotencyKey is the relay read-back path.
func (r *Repository) GetRelayByIdempotencyKey(ctx context.Context, q Querier,
	operationType string, key string) (*RelayOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanRelay(q.QueryRow(ctx,
		`SELECT `+relayColumns+` FROM dispatch_fleet_relay_operations
		 WHERE operation_type = $1 AND idempotency_key = $2`, operationType, key))
}

// GetRelayByKeyAnyType finds a relay with this idempotency key under ANY
// operation type.
//
// The unique index is per (operation_type, idempotency_key), so accept and
// decline under one key would otherwise be two valid rows — one client
// response id representing two contradictory decisions. This read lets the
// service refuse that before it reaches the database.
func (r *Repository) GetRelayByKeyAnyType(ctx context.Context, q Querier,
	providerID string, key string) (*RelayOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanRelay(q.QueryRow(ctx,
		`SELECT `+relayColumns+` FROM dispatch_fleet_relay_operations
		 WHERE provider_id = $1 AND idempotency_key = $2
		 ORDER BY created_at LIMIT 1`, providerID, key))
}

// GetRelayByID loads a relay operation.
func (r *Repository) GetRelayByID(ctx context.Context, q Querier, id string) (*RelayOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanRelay(q.QueryRow(ctx,
		`SELECT `+relayColumns+` FROM dispatch_fleet_relay_operations WHERE id = $1`, id))
}

// GetLiveRelayForAssignment finds an in-flight relay for an assignment.
func (r *Repository) GetLiveRelayForAssignment(ctx context.Context, q Querier, assignmentID string) (*RelayOperation, error) {
	if q == nil {
		q = r.db
	}
	return scanRelay(q.QueryRow(ctx,
		`SELECT `+relayColumns+` FROM dispatch_fleet_relay_operations
		 WHERE assignment_id = $1
		   AND status IN ('PENDING','DELIVERING','RETRY_SCHEDULED','UNKNOWN_OUTCOME')`,
		assignmentID))
}

// ClaimRelays leases due relay operations for one worker.
func (r *Repository) ClaimRelays(ctx context.Context, q Querier, owner string,
	lease time.Duration, limit int) ([]RelayOperation, error) {
	rows, err := q.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM dispatch_fleet_relay_operations
			WHERE status IN ('PENDING','RETRY_SCHEDULED','UNKNOWN_OUTCOME')
			  AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE dispatch_fleet_relay_operations o SET
			status = 'DELIVERING',
			lease_owner = $1,
			lease_expires_at = now() + make_interval(secs => $2),
			updated_at = now()
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING o.id, o.operation_type, o.provider_id, o.request_id, o.offer_id,
			o.assignment_id, o.idempotency_key, o.request_fingerprint, o.payload,
			o.payload_hash, o.status, o.attempt_count, o.next_attempt_at, o.lease_owner,
			o.lease_expires_at, o.response_snapshot, o.last_error,
			o.created_at, o.updated_at, o.completed_at`,
		owner, lease.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim relays: %w", err)
	}
	defer rows.Close()

	var claimed []RelayOperation
	for rows.Next() {
		var op RelayOperation
		if err := rows.Scan(&op.ID, &op.OperationType, &op.ProviderID, &op.RequestID,
			&op.OfferID, &op.AssignmentID, &op.IdempotencyKey, &op.RequestFingerprint,
			&op.Payload, &op.PayloadHash, &op.Status, &op.AttemptCount, &op.NextAttemptAt,
			&op.LeaseOwner, &op.LeaseExpiresAt, &op.ResponseSnapshot, &op.LastError,
			&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan claimed relay: %w", err)
		}
		claimed = append(claimed, op)
	}
	return claimed, rows.Err()
}

// RecoverExpiredRelayLeases returns abandoned relays to the queue.
//
// A worker that dies mid-delivery must not strand a provider's decision
// forever; the lease timeout is what makes the work recoverable.
func (r *Repository) RecoverExpiredRelayLeases(ctx context.Context, q Querier) (int64, error) {
	if q == nil {
		q = r.db
	}
	// Recovered rows become UNKNOWN_OUTCOME rather than PENDING: the previous
	// attempt may already have reached fleet, so the next pass must read back
	// rather than re-send.
	tag, err := q.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations SET
			status = 'UNKNOWN_OUTCOME', lease_owner = NULL, lease_expires_at = NULL,
			updated_at = now()
		WHERE status = 'DELIVERING'
		  AND lease_expires_at IS NOT NULL AND lease_expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("marketplace: recover relay leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ConfirmRelay records successful delivery.
func (r *Repository) ConfirmRelay(ctx context.Context, q Querier, id string, snapshot any) error {
	if q == nil {
		q = r.db
	}
	var encoded []byte
	if snapshot != nil {
		var err error
		encoded, err = json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("marketplace: encode relay snapshot: %w", err)
		}
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations SET
			status = 'CONFIRMED', response_snapshot = $2, lease_owner = NULL,
			lease_expires_at = NULL, last_error = NULL, completed_at = now(), updated_at = now()
		WHERE id = $1`, id, encoded)
	if err != nil {
		return fmt.Errorf("marketplace: confirm relay: %w", err)
	}
	return nil
}

// FailRelay schedules a retry, records a permanent rejection, or dead-letters.
func (r *Repository) FailRelay(ctx context.Context, q Querier, id string, errorCode string,
	backoff time.Duration, maxAttempts int, permanent bool) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations SET
			status = CASE
				WHEN $5 THEN 'REJECTED'
				WHEN attempt_count + 1 >= $4 THEN 'DEAD_LETTER'
				ELSE 'RETRY_SCHEDULED' END,
			attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => $3),
			lease_owner = NULL, lease_expires_at = NULL,
			last_error = $2,
			completed_at = CASE WHEN $5 THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $1`, id, errorCode, backoff.Seconds(), maxAttempts, permanent)
	if err != nil {
		return fmt.Errorf("marketplace: fail relay: %w", err)
	}
	return nil
}

// MarkRelayUnknown records an ambiguous delivery outcome.
func (r *Repository) MarkRelayUnknown(ctx context.Context, q Querier, id string,
	errorCode string, backoff time.Duration, maxAttempts int) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations SET
			status = CASE WHEN attempt_count + 1 >= $4 THEN 'DEAD_LETTER' ELSE 'UNKNOWN_OUTCOME' END,
			attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => $3),
			lease_owner = NULL, lease_expires_at = NULL,
			last_error = $2, updated_at = now()
		WHERE id = $1`, id, errorCode, backoff.Seconds(), maxAttempts)
	if err != nil {
		return fmt.Errorf("marketplace: mark relay unknown: %w", err)
	}
	return nil
}

// ReplayRelay returns a dead-lettered relay to the queue for an operator.
func (r *Repository) ReplayRelay(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations SET
			status = 'PENDING', attempt_count = 0, next_attempt_at = now(),
			last_error = NULL, updated_at = now()
		WHERE id = $1 AND status = 'DEAD_LETTER'`, id)
	if err != nil {
		return fmt.Errorf("marketplace: replay relay: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountRelaysByStatus supports metrics and smoke tests.
func (r *Repository) CountRelaysByStatus(ctx context.Context, status string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_fleet_relay_operations WHERE status = $1`,
		status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("marketplace: count relays: %w", err)
	}
	return count, nil
}
