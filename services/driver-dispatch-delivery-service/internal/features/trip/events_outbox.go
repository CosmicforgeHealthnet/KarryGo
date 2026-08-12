package trip

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// EventEmitter publishes canonical marketplace events for trip transitions.
//
// Declared here rather than imported so this package keeps no dependency on
// the marketplace package; the composition root supplies the implementation.
type EventEmitter interface {
	Emit(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, correlationID string,
		causationID string, payload map[string]any) error
}

// Canonical event types this package emits.
const (
	eventTripStatusChanged = "trip.status_changed.v1"
	eventTripCompleted     = "trip.completed.v1"
	eventTripCancelled     = "trip.cancelled.v1"
	eventProofCompleted    = "proof.completed.v1"
)

const sourceServiceName = "driver-dispatch-delivery-service"

// WithEventEmitter attaches an emitter. Without one the repository behaves
// exactly as before, which is what keeps the existing suite — none of which
// wires an emitter — describing unchanged behaviour.
func (r *PostgresRepository) WithEventEmitter(emitter EventEmitter) *PostgresRepository {
	r.emitter = emitter
	return r
}

// emitting reports whether an emitter is attached and active.
func (r *PostgresRepository) emitting() bool {
	return r.emitter != nil
}

// bumpTripVersion increments and returns the trip's source version.
//
// Consumers order and gap-detect on this number, so it must advance by exactly
// one per emitted event and it must advance in the same transaction as the
// state change. A separate statement rather than an extra RETURNING column
// keeps every existing scan path untouched.
func bumpTripVersion(ctx context.Context, tx pgx.Tx, tripID string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
		UPDATE trips SET source_version = source_version + 1
		WHERE id = $1
		RETURNING source_version`, tripID).Scan(&version)
	return version, err
}

// tripEventPayload builds the consumer-visible projection of a trip.
//
// Deliberately assembled field by field instead of marshalling the row: the
// trip carries fare, currency and receiver contact details, none of which a
// managing organization is entitled to see on an event stream. The publish
// path rejects the commercial keys by name as a second line of defence, but
// the receiver's name and phone would pass that check, so they are excluded
// here — at the only place that knows they are not needed.
func tripEventPayload(t *Trip, extra map[string]any) map[string]any {
	payload := map[string]any{
		"sourceService":  sourceServiceName,
		"tripId":         t.ID,
		"bookingId":      t.BookingID,
		"providerId":     t.ProviderID,
		"status":         string(t.Status),
		"serviceTier":    t.ServiceTier,
		"distanceKm":     t.DistanceKM,
		"pickupAddress":  t.PickupAddress,
		"dropoffAddress": t.DropoffAddress,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

// emitTripEvent publishes one trip event inside tx, bumping the source version.
func (r *PostgresRepository) emitTripEvent(ctx context.Context, tx pgx.Tx,
	eventType string, t *Trip, extra map[string]any) error {
	if !r.emitting() || t == nil {
		return nil
	}
	version, err := bumpTripVersion(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	payload := tripEventPayload(t, extra)
	payload["sourceVersion"] = version
	return r.emitter.Emit(ctx, tx, eventType, "TRIP", t.ID, version, "", "", payload)
}

// emitStatusChange publishes trip.status_changed.v1 for a transition whose
// caller holds only identifiers rather than the trip row.
func (r *PostgresRepository) emitStatusChange(ctx context.Context, tx pgx.Tx,
	tripID string, from TripStatus, to TripStatus, changedBy string, changedAt any) error {
	if !r.emitting() {
		return nil
	}
	version, err := bumpTripVersion(ctx, tx, tripID)
	if err != nil {
		return err
	}
	return r.emitter.Emit(ctx, tx, eventTripStatusChanged, "TRIP", tripID, version, "", "",
		map[string]any{
			"sourceService": sourceServiceName,
			"tripId":        tripID,
			"sourceVersion": version,
			"status":        string(to),
			"fromStatus":    string(from),
			"toStatus":      string(to),
			"changedBy":     changedBy,
			"changedAt":     changedAt,
		})
}

// emitProofEvent publishes proof.completed.v1 for a trip whose row the caller
// did not re-read.
//
// The payload reports that proof exists and whether it has been verified. It
// carries no photo reference, no signature reference and neither the
// receiver's name nor phone number: a managing organization needs to know the
// delivery was evidenced, not to hold the evidence or the recipient's contact
// details. Fetching the artefacts is a separate, separately authorized read
// against the media service.
func (r *PostgresRepository) emitProofEvent(ctx context.Context, tx pgx.Tx,
	tripID string, proof *DeliveryProof, submittedAt any) error {
	if !r.emitting() || proof == nil {
		return nil
	}
	version, err := bumpTripVersion(ctx, tx, tripID)
	if err != nil {
		return err
	}
	return r.emitter.Emit(ctx, tx, eventProofCompleted, "TRIP", tripID, version, "", "",
		map[string]any{
			"sourceService": sourceServiceName,
			"tripId":        tripID,
			"proofId":       proof.ID,
			"sourceVersion": version,
			"hasPhoto":      proof.PhotoRef != "",
			"hasSignature":  proof.SignatureRef != "",
			"verified":      proof.Verified,
			"submittedAt":   submittedAt,
		})
}
