// Package movement publishes dispatch deliveries to the customer context
// owner as shared movement events.
//
// It does NOT hook the delivery state machine. It reads the state dispatch
// has already committed, compares it with what was last published, and
// queues the difference durably. That keeps this package additive: no
// transition here can change, delay or fail a delivery, and a crash between a
// transition and its event only delays the event.
package movement

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
)

// SourceService is dispatch's identity on the shared envelope.
const SourceService = "driver-dispatch-delivery-service"

// stateFor maps dispatch's own derived wire status onto the shared movement
// vocabulary. An unknown status publishes nothing rather than guessing.
func stateFor(tripStatus, bookingStatus, broadcastStatus string, cancelled bool) string {
	switch tripStatus {
	case "assigned", "en_route_pickup":
		return events.MovementStateAssigned
	case "arrived_pickup":
		return events.MovementStateArrived
	case "in_progress":
		return events.MovementStateInProgress
	case "proof_submitted":
		return events.MovementStateDelivered
	case "completed":
		return events.MovementStateCompleted
	case "cancelled", "failed":
		return events.MovementStateCancelled
	}
	if bookingStatus == "unmatched" {
		return events.MovementStateUnmatched
	}
	if cancelled || bookingStatus == "cancelled" {
		return events.MovementStateCancelled
	}
	switch broadcastStatus {
	case "cancelled":
		return events.MovementStateCancelled
	case "no_provider_found", "expired":
		return events.MovementStateUnmatched
	case "accepted":
		return events.MovementStateAssigned
	}
	return events.MovementStateSearching
}

// Publisher sweeps changed deliveries into the movement outbox.
type Publisher struct {
	pool    *pgxpool.Pool
	emitter *events.Emitter
	// cursor is the high-water mark of source updates already inspected. It
	// starts at zero so every boot reconciles the whole table, which is also
	// the backfill path.
	cursor time.Time
	batch  int
}

func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool, batch: 200,
		emitter: &events.Emitter{Pool: pool, SourceService: SourceService, EntityType: "dispatch_booking"}}
}

// Overlap re-inspects a short window behind the cursor, so a row committed
// with a slightly earlier timestamp than one already read is not skipped.
const Overlap = 2 * time.Minute

// Sweep publishes every delivery whose committed state differs from what was
// last published.
func (p *Publisher) Sweep(ctx context.Context) (events.EmissionResult, error) {
	if p == nil || p.pool == nil {
		return events.EmissionResult{}, events.ErrEmitterNotConfigured
	}
	since := p.cursor.Add(-Overlap)
	rows, err := p.pool.Query(ctx, `
		SELECT b.id::text, b.customer_id::text, b.status, COALESCE(b.service_tier,''),
		       b.pickup_address, b.pickup_lat::float8, b.pickup_lng::float8,
		       b.dropoff_address, b.dropoff_lat::float8, b.dropoff_lng::float8,
		       b.cancelled_at, b.completed_at,
		       COALESCE(rb.status,''), COALESCE(t.status,''), t.started_at, t.completed_at,
		       COALESCE(cm.parent_order_ref,''), COALESCE(cm.mission_ref,''),
		       GREATEST(b.updated_at, COALESCE(t.updated_at, b.updated_at)) AS changed_at
		FROM dispatch_bookings b
		LEFT JOIN request_broadcasts rb ON rb.booking_id = b.id
		LEFT JOIN trips t ON t.booking_id = b.id
		LEFT JOIN commerce_missions cm ON cm.booking_id = b.id
		WHERE b.updated_at > $1 OR t.updated_at > $1
		ORDER BY changed_at
		LIMIT $2`, since, p.batch)
	if err != nil {
		return events.EmissionResult{}, err
	}
	var (
		summaries []events.MovementSummary
		newest    = p.cursor
	)
	for rows.Next() {
		var (
			id, customerID, bookingStatus, tier                     string
			pickupAddress, dropoffAddress                           string
			pickupLat, pickupLng, dropoffLat, dropoffLng            float64
			cancelledAt, completedAt, startedAt, tripCompletedAt    *time.Time
			broadcastStatus, tripStatus, parentOrderRef, missionRef string
			changedAt                                               time.Time
		)
		if err = rows.Scan(&id, &customerID, &bookingStatus, &tier,
			&pickupAddress, &pickupLat, &pickupLng, &dropoffAddress, &dropoffLat, &dropoffLng,
			&cancelledAt, &completedAt, &broadcastStatus, &tripStatus, &startedAt, &tripCompletedAt,
			&parentOrderRef, &missionRef, &changedAt); err != nil {
			rows.Close()
			return events.EmissionResult{}, err
		}
		if changedAt.After(newest) {
			newest = changedAt
		}
		if completedAt == nil {
			completedAt = tripCompletedAt
		}
		summary := events.MovementSummary{
			CustomerID: customerID, Authority: events.MovementAuthorityDispatch, SourceRef: id,
			ProductCode: tier, MovementType: events.MovementTypeDispatch,
			State:       stateFor(tripStatus, bookingStatus, broadcastStatus, cancelledAt != nil),
			Origin:      &events.PlaceSummary{Label: pickupAddress, Latitude: pickupLat, Longitude: pickupLng},
			Destination: &events.PlaceSummary{Label: dropoffAddress, Latitude: dropoffLat, Longitude: dropoffLng},
			StartedAt:   startedAt, CompletedAt: completedAt,
		}
		if parentOrderRef != "" {
			summary.ParentContextType, summary.ParentContextRef = "COMMERCE_ORDER", parentOrderRef
			summary.FulfilmentGroupRef = missionRef
		}
		summaries = append(summaries, summary)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return events.EmissionResult{}, err
	}
	result, err := p.emitter.Emit(ctx, summaries)
	if err != nil {
		return result, err
	}
	p.cursor = newest
	return result, nil
}

// Run sweeps until the context ends.
func (p *Publisher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := p.Sweep(ctx); err != nil {
				logging.Error("dispatch-movement", "sweep: %v", err)
			}
		}
	}
}
