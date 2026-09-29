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

// stageFor is the fine-grained lifecycle step beneath state (Customer V4
// §22.2). Haulage records loading as picked_up; arrival at the drop-off and
// delivery are one authority step (delivered), so no separate drop-off stage
// is reported.
func stageFor(status string) string {
	switch status {
	case "pending_match", "awaiting_acceptance":
		return events.MovementStageSearching
	case "accepted":
		return events.MovementStageProviderAssigned
	case "en_route_pickup":
		return events.MovementStageEnRoutePickup
	case "arrived_at_pickup":
		return events.MovementStageArrivedPickup
	case "picked_up":
		return events.MovementStageCustodyStarted
	case "en_route_delivery":
		return events.MovementStageInTransit
	case "delivered":
		return events.MovementStageDelivered
	case "completed":
		return events.MovementStageCompleted
	case "cancelled":
		return events.MovementStageCancelled
	case "unmatched":
		return events.MovementStageUnmatched
	}
	return ""
}

// Publisher sweeps changed haulages into the movement outbox.
type Publisher struct {
	pool    *pgxpool.Pool
	emitter *events.Emitter
	batch   int
	after   string
}

func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool, batch: 200,
		emitter: &events.Emitter{Pool: pool, SourceService: SourceService, EntityType: "haulage_booking"}}
}

// Each bounded sweep advances through committed booking IDs and cycles back.

// Sweep publishes every haulage whose committed state differs from the last
// published view.
func (p *Publisher) Sweep(ctx context.Context) (events.EmissionResult, error) {
	if p == nil || p.pool == nil {
		return events.EmissionResult{}, events.ErrEmitterNotConfigured
	}
	since := time.Time{}
	after := p.after
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	rows, err := p.pool.Query(ctx, `
		SELECT b.id::text, b.customer_id::text, b.status, b.cargo_type,
		       b.pickup_address, b.pickup_lat, b.pickup_lng,
		       b.dropoff_address, b.dropoff_lat, b.dropoff_lng,
		       b.picked_up_at, b.completed_at,
		       COALESCE(cm.parent_order_ref,''), COALESCE(cm.mission_ref,''),
		       b.updated_at
		FROM haulage_bookings b
		LEFT JOIN commerce_missions cm ON cm.booking_id = b.id
		WHERE b.updated_at > $1 AND b.id > $3::uuid
		ORDER BY b.id
		LIMIT $2`, since, p.batch, after)
	if err != nil {
		return events.EmissionResult{}, err
	}
	var (
		summaries []events.MovementSummary
		lastID    string
	)
	scanned := 0
	for rows.Next() {
		scanned++
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
		lastID = id
		state := stateFor(status)
		if state == "" {
			// An unmapped status publishes nothing rather than guessing a
			// movement state the customer would see.
			continue
		}
		summary := events.MovementSummary{
			CustomerID: customerID, Authority: events.MovementAuthorityHauling, SourceRef: id,
			ProductCode: cargoType, MovementType: events.MovementTypeHaulage, State: state, Stage: stageFor(status),
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
	p.after = lastID
	if scanned < p.batch {
		p.after = ""
	}
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
