// Package outbox writes taxi_outbox rows. It is deliberately a thin,
// generic writer, not a domain-specific one: booking creation (phase B1),
// lifecycle transitions (B3), and provider operational-state changes (R)
// all need to append an event inside the SAME transaction as their own
// mutation, so this package depends on nothing feature-specific and every
// feature depends on it, never the other way around.
//
// The publisher worker (leasing unread rows and delivering them to
// notification-service, per addendum §26) is a separate, later piece of
// work -- this package is the write side only, which is sufficient for a
// crash between a booking's commit and delivery to never lose the event: the
// row is already durable in the same transaction as the booking itself.
//
// Event type constants are added incrementally, one per phase, as that
// phase actually starts emitting it -- not pre-declared as a speculative
// full list, so every constant here is backed by a real caller.
package outbox

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is satisfied by both *pgxpool.Pool and pgx.Tx (structurally, no
// explicit wiring needed) -- the same shape as every other Executor
// abstraction already in this service, so a caller can write an outbox row
// either standalone or, far more commonly, as one more statement inside a
// transaction it already holds open for its own mutation.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Event type constants, one per transition a caller actually emits.
const (
	// EventBookingCreated fires when a passenger's booking is first persisted
	// (phase B1).
	EventBookingCreated = "taxi.booking.created"
	// EventBookingMatched fires when the matcher offers a booking to a driver
	// (phase B2): pending_match -> awaiting_acceptance.
	EventBookingMatched = "taxi.booking.matched"
	// EventBookingAccepted fires when a driver accepts an offer (phase B2):
	// awaiting_acceptance -> accepted.
	EventBookingAccepted = "taxi.booking.accepted"
	// EventBookingUnmatched fires when the search window elapses with no
	// driver found (phase B2): pending_match -> unmatched.
	EventBookingUnmatched = "taxi.booking.unmatched"
	// EventBookingOfferExpired fires when an offer is requeued (phase B2):
	// either the driver let it time out, or explicitly rejected it --
	// awaiting_acceptance -> pending_match either way.
	EventBookingOfferExpired = "taxi.booking.offer_expired"
	// The remaining four mirror the phase B3 lifecycle ladder.
	EventBookingEnRoutePickup = "taxi.booking.en_route_pickup"
	EventBookingArrived       = "taxi.booking.arrived_at_pickup"
	EventBookingStarted       = "taxi.booking.started"
	EventBookingCompleted     = "taxi.booking.completed"
	EventBookingCancelled     = "taxi.booking.cancelled"
)

// Write appends one event row. payload must already be valid JSON (typically
// the output of json.Marshal) -- this package does not know or care about any
// event's shape, only that it durably lands in the same transaction as
// whatever caller holds exec.
func Write(ctx context.Context, exec Executor, aggregateType, aggregateID, eventType string, payload []byte) error {
	_, err := exec.Exec(ctx, `
		INSERT INTO taxi_outbox (aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4)
	`, aggregateType, aggregateID, eventType, payload)
	return err
}
