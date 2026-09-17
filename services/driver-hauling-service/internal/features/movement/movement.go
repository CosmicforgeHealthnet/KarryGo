// Package movement publishes haulage bookings to the customer context owner
// as shared movement events.
//
// Like dispatch's own emitter, it reads committed state rather than hooking
// the booking state machine: publishing can never change, delay or fail a
// haulage, and a crash only delays an event.
package movement

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
)

// SourceService is hauling's identity on the shared envelope.
const SourceService = "driver-hauling-service"

// stateFor maps the haulage status ladder onto the shared vocabulary.
func stateFor(status string) string {
	switch status {
	case "pending_match":
		return events.MovementStateSearching
	case "awaiting_acceptance":
		return events.MovementStateSearching
	case "accepted":
		return events.MovementStateAssigned
	case "en_route_pickup":
		return events.MovementStateAssigned
	case "arrived_at_pickup":
		return events.MovementStateArrived
	case "picked_up", "en_route_delivery":
		return events.MovementStateInProgress
	case "delivered":
		return events.MovementStateDelivered
	case "completed":
		return events.MovementStateCompleted
	case "cancelled":
		return events.MovementStateCancelled
	case "unmatched":
		return events.MovementStateUnmatched
	}
	return ""
}

// Publisher sweeps changed haulages into the movement outbox.
type Publisher struct {
	pool    *pgxpool.Pool
	emitter *events.Emitter
	cursor  time.Time
	batch   int
}

func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool, batch: 200,
		emitter: &events.Emitter{Pool: pool, SourceService: SourceService, EntityType: "haulage_booking"}}
}

// Overlap re-inspects a short window behind the cursor so a row committed
// slightly out of timestamp order is never skipped.
const Overlap = 2 * time.Minute

// Sweep publishes every haulage whose committed state differs from the last
// published view.
func (p *Publisher) Sweep(ctx context.Context) (events.EmissionResult, error) {
	if p == nil || p.pool == nil {
		return events.EmissionResult{}, events.ErrEmitterNotConfigured
	}
	since := p.cursor.Add(-Overlap)
	rows, err := p.pool.Query(ctx, `
		SELECT b.id::text, b.customer_id::text, b.status, b.cargo_type,
		       b.pickup_address, b.pickup_lat, b.pickup_lng,
		       b.dropoff_address, b.dropoff_lat, b.dropoff_lng,
		       b.picked_up_at, b.completed_at,
		       COALESCE(cm.parent_order_ref,''), COALESCE(cm.mission_ref,''),
		       b.updated_at
		FROM haulage_bookings b
		LEFT JOIN commerce_missions cm ON cm.booking_id = b.id
		WHERE b.updated_at > $1
		ORDER BY b.updated_at
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
			id, customerID, status, cargoType            string
			pickupAddress, dropoffAddress                string
			pickupLat, pickupLng, dropoffLat, dropoffLng float64
			startedAt, completedAt                       *time.Time
			parentOrderRef, missionRef                   string
			changedAt                                    time.Time
		)
		if err = rows.Scan(&id, &customerID, &status, &cargoType,
			&pickupAddress, &pickupLat, &pickupLng, &dropoffAddress, &dropoffLat, &dropoffLng,
			&startedAt, &completedAt, &parentOrderRef, &missionRef, &changedAt); err != nil {
			rows.Close()
			return events.EmissionResult{}, err
		}
		if changedAt.After(newest) {
			newest = changedAt
		}
		state := stateFor(status)
		if state == "" {
			// An unmapped status publishes nothing rather than guessing a
			// movement state the customer would see.
			continue
		}
		summary := events.MovementSummary{
			CustomerID: customerID, Authority: events.MovementAuthorityHauling, SourceRef: id,
			ProductCode: cargoType, MovementType: events.MovementTypeHaulage, State: state,
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
				logging.Error("hauling-movement", "sweep: %v", err)
			}
		}
	}
}
