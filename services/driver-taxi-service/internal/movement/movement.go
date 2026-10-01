// Package movement publishes Taxi's customer movement summaries to
// customer-service. Events are written to taxi_outbox in the SAME
// transaction as the booking or mission change, with destination
// CUSTOMER_MOVEMENT. They are delivered asynchronously, so a customer-service
// outage never blocks Taxi execution.
package movement

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/dbtx"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/events"
)

const SourceService = "driver-taxi-service"

// BookingState maps a passenger booking status to the shared movement state.
func BookingState(status bookingmodels.Status) (string, bool) {
	switch status {
	case bookingmodels.StatusScheduled:
		return events.MovementStateScheduled, true
	case bookingmodels.StatusPendingMatch, bookingmodels.StatusAwaitingAccept:
		return events.MovementStateSearching, true
	case bookingmodels.StatusAccepted, bookingmodels.StatusEnRoutePickup:
		return events.MovementStateAssigned, true
	case bookingmodels.StatusArrivedAtPickup, bookingmodels.StatusPickupVerificationPending:
		return events.MovementStateArrived, true
	case bookingmodels.StatusInProgress:
		return events.MovementStateInProgress, true
	case bookingmodels.StatusCompleted:
		return events.MovementStateCompleted, true
	case bookingmodels.StatusCancelled:
		return events.MovementStateCancelled, true
	case bookingmodels.StatusUnmatched:
		return events.MovementStateUnmatched, true
	}
	return "", false
}

// BookingStage is the Ride's fine-grained lifecycle step (Customer V4
// §22.2): en route and arrived are distinct from assignment.
func BookingStage(status bookingmodels.Status) string {
	switch status {
	case bookingmodels.StatusScheduled:
		return events.MovementStageScheduled
	case bookingmodels.StatusPendingMatch, bookingmodels.StatusAwaitingAccept:
		return events.MovementStageSearching
	case bookingmodels.StatusAccepted:
		return events.MovementStageProviderAssigned
	case bookingmodels.StatusEnRoutePickup:
		return events.MovementStageEnRoutePickup
	case bookingmodels.StatusArrivedAtPickup, bookingmodels.StatusPickupVerificationPending:
		return events.MovementStageArrivedPickup
	case bookingmodels.StatusInProgress:
		return events.MovementStageInTransit
	case bookingmodels.StatusCompleted:
		return events.MovementStageCompleted
	case bookingmodels.StatusCancelled:
		return events.MovementStageCancelled
	case bookingmodels.StatusUnmatched:
		return events.MovementStageUnmatched
	}
	return ""
}

// GoodsStage is the Taxi Goods mission's fine-grained lifecycle step.
func GoodsStage(status string) string {
	switch status {
	case "MATCHING", "OFFERED":
		return events.MovementStageSearching
	case "ASSIGNED":
		return events.MovementStageProviderAssigned
	case "EN_ROUTE_PICKUP":
		return events.MovementStageEnRoutePickup
	case "AT_PICKUP", "PICKUP_HANDOVER_REQUIRED":
		return events.MovementStageArrivedPickup
	case "PICKED_UP":
		return events.MovementStageCustodyStarted
	case "IN_TRANSIT":
		return events.MovementStageInTransit
	case "AT_DROPOFF", "DELIVERY_HANDOVER_REQUIRED":
		return events.MovementStageArrivedDropoff
	case "DELIVERED":
		return events.MovementStageDelivered
	case "COMPLETED":
		return events.MovementStageCompleted
	case "CANCELLED":
		return events.MovementStageCancelled
	case "UNMATCHED":
		return events.MovementStageUnmatched
	case "FAILED":
		return events.MovementStageFailed
	}
	return ""
}

// BookingEventName maps a movement state to the canonical Taxi event name.
func BookingEventName(state string) string {
	switch state {
	case events.MovementStateAssigned:
		return events.TaxiBookingAssigned
	case events.MovementStateArrived:
		return events.TaxiBookingProviderArrived
	case events.MovementStateInProgress:
		return events.TaxiBookingStarted
	case events.MovementStateCompleted:
		return events.TaxiBookingCompleted
	case events.MovementStateCancelled:
		return events.TaxiBookingCancelled
	case events.MovementStateUnmatched:
		return events.TaxiBookingUnmatched
	}
	return events.TaxiBookingCreated
}

// GoodsState maps a Taxi Goods mission status to the shared movement state.
func GoodsState(status string) (string, bool) {
	switch status {
	case "MATCHING", "OFFERED":
		return events.MovementStateSearching, true
	case "ASSIGNED", "EN_ROUTE_PICKUP":
		return events.MovementStateAssigned, true
	case "AT_PICKUP", "PICKUP_HANDOVER_REQUIRED":
		return events.MovementStateArrived, true
	case "PICKED_UP", "IN_TRANSIT", "AT_DROPOFF", "DELIVERY_HANDOVER_REQUIRED":
		return events.MovementStateInProgress, true
	case "DELIVERED":
		return events.MovementStateDelivered, true
	case "COMPLETED":
		return events.MovementStateCompleted, true
	case "CANCELLED":
		return events.MovementStateCancelled, true
	case "UNMATCHED":
		return events.MovementStateUnmatched, true
	case "FAILED":
		return events.MovementStateFailed, true
	}
	return "", false
}

// Writer appends movement envelopes. A disabled Writer is a no-op, so the
// feature can be deployed dark.
type Writer struct {
	Enabled bool
	Now     func() time.Time
}

func (w *Writer) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

// WriteBooking bumps the booking's movement_version and writes one envelope.
// The version is taken from the row lock inside the caller's transaction, so
// concurrent transitions can never publish the same version twice.
func (w *Writer) WriteBooking(ctx context.Context, exec dbtx.Executor, booking bookingmodels.Booking, correlationID string) error {
	if w == nil || !w.Enabled {
		return nil
	}
	state, ok := BookingState(booking.Status)
	if !ok {
		return nil
	}
	var version int64
	var replacements int
	var product *string
	if err := exec.QueryRow(ctx, `UPDATE taxi_bookings SET movement_version=movement_version+1 WHERE id=$1 RETURNING movement_version, provider_replacement_count, service_product_code`, booking.ID).Scan(&version, &replacements, &product); err != nil {
		return err
	}
	summary := events.MovementSummary{
		CustomerID: booking.CustomerID, Authority: events.MovementAuthorityTaxi, SourceRef: booking.ID,
		MovementType: events.MovementTypePassenger, State: state, Stage: BookingStage(booking.Status), ScheduledAt: booking.ScheduledAt,
		ProviderReplacementCount: replacements,
	}
	if booking.RidePurpose == "parcel" {
		summary.MovementType = events.MovementTypeGoods
	}
	if product != nil {
		summary.ProductCode = *product
	}
	for _, stop := range booking.Stops {
		place := &events.PlaceSummary{Label: stop.Address, Latitude: stop.Lat, Longitude: stop.Lng}
		switch stop.Type {
		case bookingmodels.StopTypePickup:
			if summary.Origin == nil {
				summary.Origin = place
			}
		case bookingmodels.StopTypeDropoff:
			summary.Destination = place
		}
	}
	now := w.now()
	switch state {
	case events.MovementStateInProgress:
		summary.StartedAt = &now
	case events.MovementStateCompleted, events.MovementStateCancelled, events.MovementStateUnmatched:
		summary.CompletedAt = &now
	}
	return write(ctx, exec, booking.ID, "booking", BookingEventName(state), version, now, correlationID, summary)
}

// GoodsMissionRefs carries the commerce context a goods movement must show.
type GoodsMissionRefs struct {
	MissionID          string
	MissionRef         string
	CustomerRef        string
	Status             string
	ProductCode        string
	ParentOrderRef     string
	FulfilmentPlanRef  string
	FulfilmentGroupRef string
	Pickup             *events.PlaceSummary
	Dropoff            *events.PlaceSummary
}

// WriteGoodsMission publishes a commerce goods movement. Mission count and
// sequence are never computed here: they belong to Vendor fulfilment and are
// only forwarded when Vendor supplied them.
func (w *Writer) WriteGoodsMission(ctx context.Context, exec dbtx.Executor, refs GoodsMissionRefs, eventName, correlationID string) error {
	if w == nil || !w.Enabled {
		return nil
	}
	state, ok := GoodsState(refs.Status)
	if !ok {
		return nil
	}
	if _, err := uuid.Parse(strings.TrimSpace(refs.CustomerRef)); err != nil {
		// Vendor commerce customer refs that are not customer-service ids
		// cannot be projected; skipping never blocks execution.
		return nil
	}
	var version int64
	var replacements int
	var recovery string
	if err := exec.QueryRow(ctx, `UPDATE taxi_goods_missions SET movement_version=movement_version+1 WHERE id=$1 RETURNING movement_version, provider_replacement_count, recovery_state`, refs.MissionID).Scan(&version, &replacements, &recovery); err != nil {
		return err
	}
	summary := events.MovementSummary{
		CustomerID: refs.CustomerRef, Authority: events.MovementAuthorityTaxi, SourceRef: refs.MissionRef,
		MovementType: events.MovementTypeGoods, State: state, Stage: GoodsStage(refs.Status), ProductCode: refs.ProductCode,
		ProviderReplacementCount: replacements, RecoveryState: recovery,
		ParentContextType: "VENDOR_ORDER", ParentContextRef: refs.ParentOrderRef,
		FulfilmentPlanRef: refs.FulfilmentPlanRef, FulfilmentGroupRef: refs.FulfilmentGroupRef,
		Origin: refs.Pickup, Destination: refs.Dropoff,
	}
	now := w.now()
	if events.IsTerminalMovementState(state) {
		summary.CompletedAt = &now
	}
	return write(ctx, exec, refs.MissionID, "taxi_goods_mission", eventName, version, now, correlationID, summary)
}

func write(ctx context.Context, exec dbtx.Executor, aggregateID, entityType, eventName string, version int64, now time.Time, correlationID string, summary events.MovementSummary) error {
	eventID := uuid.NewString()
	envelope, err := events.NewMovementEnvelope(eventID, eventName, SourceService, entityType, version, now, correlationID, "", summary)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = exec.Exec(ctx, `INSERT INTO taxi_outbox (id, aggregate_type, aggregate_id, event_type, payload, target) VALUES ($1,$2,$3,$4,$5,$6)`,
		eventID, entityType, aggregateID, eventName, body, outbox.TargetCustomer)
	return err
}

// Destination delivers customer movement outbox rows to customer-service.
type Destination struct {
	Deliverer events.HTTPDeliverer
}

func (d Destination) Deliver(ctx context.Context, record outbox.Record) error {
	var envelope events.Envelope
	if err := json.Unmarshal(record.Payload, &envelope); err != nil {
		return errors.New("customer movement outbox row is not an envelope")
	}
	return d.Deliverer.Deliver(ctx, envelope)
}
