package request

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// EventEmitter publishes canonical marketplace events for broadcast
// transitions. Declared here so this package keeps no dependency on the
// marketplace package; the composition root supplies the implementation.
type EventEmitter interface {
	Emit(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, correlationID string,
		causationID string, payload map[string]any) error
}

// Canonical event types this package emits.
const (
	eventRequestOffered   = "platform.request.offered.v1"
	eventRequestUpdated   = "platform.request.updated.v1"
	eventRequestExpired   = "platform.request.expired.v1"
	eventRequestCancelled = "platform.request.cancelled.v1"
)

const sourceServiceName = "driver-dispatch-delivery-service"

// WithEventEmitter attaches an emitter. Without one the repository behaves
// exactly as it did before the marketplace outbox existed.
func (r *PostgresRepository) WithEventEmitter(emitter EventEmitter) *PostgresRepository {
	r.emitter = emitter
	return r
}

func (r *PostgresRepository) emitting() bool {
	return r.emitter != nil
}

// bumpBroadcastVersion increments and returns the broadcast's source version.
//
// Consumers order and gap-detect on this number, so it advances by exactly one
// per emitted event, inside the transaction that made the change.
func bumpBroadcastVersion(ctx context.Context, tx pgx.Tx, broadcastID string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
		UPDATE request_broadcasts SET source_version = source_version + 1
		WHERE id = $1
		RETURNING source_version`, broadcastID).Scan(&version)
	return version, err
}

// emitBroadcastEvent publishes one request event inside tx.
//
// Nothing from booking_payload reaches the payload. That column holds the
// customer's booking as submitted — addresses, contact details and fare — and
// copying it wholesale onto a consumer-visible stream is exactly how customer
// commercial data escapes. Only the identifiers and the operational status a
// managing organization needs are carried.
func (r *PostgresRepository) emitBroadcastEvent(ctx context.Context, tx pgx.Tx,
	eventType string, broadcastID string, bookingID string, extra map[string]any) error {
	if !r.emitting() {
		return nil
	}
	version, err := bumpBroadcastVersion(ctx, tx, broadcastID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"sourceService": sourceServiceName,
		"requestId":     broadcastID,
		"broadcastId":   broadcastID,
		"bookingId":     bookingID,
		"sourceVersion": version,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return r.emitter.Emit(ctx, tx, eventType, "REQUEST", broadcastID, version, "", "", payload)
}

// withTx runs fn inside a new transaction.
//
// The pool-level statements these transitions used were already atomic on
// their own; they need a transaction now only because the event must commit
// with the change. When no emitter is attached the caller skips this entirely
// and the original single-statement path runs unchanged.
func (r *PostgresRepository) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
