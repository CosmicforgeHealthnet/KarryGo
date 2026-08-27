package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type etaSourceState struct {
	PlanVersion          int64      `json:"plan_version"`
	Mode                 string     `json:"mode"`
	MissionState         string     `json:"mission_state"`
	Domain               string     `json:"domain"`
	PromisedLatest       *time.Time `json:"promised_latest"`
	ReadyFrom            *time.Time `json:"ready_from"`
	ReadyTo              *time.Time `json:"ready_to"`
	ActualReady          bool       `json:"actual_ready"`
	ProviderAssigned     bool       `json:"provider_assigned"`
	ProviderArrived      bool       `json:"provider_arrived"`
	PickedUp             bool       `json:"picked_up"`
	ProviderRef          string     `json:"provider_ref"`
	RouteSeconds         int64      `json:"route_seconds"`
	RouteProvider        string     `json:"route_provider"`
	RouteEvidenceVersion string     `json:"route_evidence_version"`
	RouteObservedAt      time.Time  `json:"route_observed_at"`
	PickupStopCount      int        `json:"pickup_stop_count"`
	CompletedPickupStops int        `json:"completed_pickup_stops"`
	OperationalException bool       `json:"operational_exception"`
}

func isLogisticsSignalEvent(eventType string) bool {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "MISSION_CREATED", "PROVIDER_ASSIGNED", "PROVIDER_REASSIGNED", "PROVIDER_ARRIVED",
		"STOP_COMPLETED", "OPERATIONAL_EXCEPTION", "RETURNING":
		return true
	default:
		return false
	}
}

func signalNotificationAction(eventType string) bool {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "PROVIDER_ASSIGNED", "PROVIDER_REASSIGNED", "PROVIDER_ARRIVED":
		return true
	default:
		return false
	}
}

func etaTriggerFor(eventType, reason string) ETATrigger {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "MISSION_CREATED":
		return TriggerMissionCreated
	case "PROVIDER_ASSIGNED":
		return TriggerProviderAssigned
	case "PROVIDER_REASSIGNED":
		return TriggerProviderReassigned
	case "PROVIDER_ARRIVED":
		return TriggerProviderArrived
	case "PICKED_UP":
		return TriggerPickupCompleted
	case "STOP_COMPLETED":
		return TriggerStopCompleted
	case "DELIVERED":
		return TriggerDelivered
	case "DELIVERY_FAILED":
		return TriggerCancelled
	case "RETURNING":
		return TriggerReturning
	case "OPERATIONAL_EXCEPTION":
		reason = strings.ToUpper(strings.TrimSpace(reason))
		if strings.Contains(reason, "BREAKDOWN") {
			return TriggerBreakdown
		}
		if strings.Contains(reason, "ROUTE") || strings.Contains(reason, "DEVIATION") {
			return TriggerRouteDeviation
		}
		return TriggerOperationalException
	default:
		return TriggerTimingUpdated
	}
}

// applyDispatchConfidenceETATx merges a versioned Dispatch fact into the last
// ETA source snapshot. Missing route evidence fails closed to "no ETA" and
// never fabricates a straight-line estimate or blocks mission progression.
func (s *Store) applyDispatchConfidenceETATx(ctx context.Context, tx pgx.Tx, event LogisticsOrderEvent, order VendorOrder) (DeliveryETA, bool, error) {
	var payload dispatchCommercePayload
	if len(event.Payload) == 0 || json.Unmarshal(event.Payload, &payload) != nil {
		return DeliveryETA{}, false, nil
	}
	var state etaSourceState
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT e.source_snapshot FROM commerce_delivery_eta_current c
		JOIN commerce_delivery_eta_versions e ON e.id=c.eta_id
		WHERE c.parent_order_id=$1 AND c.mission_slot_ref=$2`, event.ParentOrderID, event.MissionRef).Scan(&raw)
	if err == nil {
		if json.Unmarshal(raw, &state) != nil {
			return DeliveryETA{}, false, ErrOrderConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return DeliveryETA{}, false, err
	}

	if payload.PlanVersion > 0 {
		state.PlanVersion = payload.PlanVersion
	}
	if payload.Mode != "" {
		state.Mode = payload.Mode
	}
	if payload.PromisedBy != nil {
		state.PromisedLatest = payload.PromisedBy
	}
	if payload.ReadyFrom != nil && payload.ReadyTo != nil {
		state.ReadyFrom, state.ReadyTo = payload.ReadyFrom, payload.ReadyTo
	}
	if payload.RouteProvider != "" && payload.RouteEvidenceVersion != "" && payload.EstimatedSeconds > 0 && payload.RouteObservedAt != nil {
		state.RouteProvider, state.RouteEvidenceVersion = payload.RouteProvider, payload.RouteEvidenceVersion
		state.RouteSeconds, state.RouteObservedAt = payload.EstimatedSeconds, payload.RouteObservedAt.UTC()
	}
	if payload.PickupStopCount > 0 {
		state.PickupStopCount = payload.PickupStopCount
	}
	if payload.CompletedPickupStops > state.CompletedPickupStops {
		state.CompletedPickupStops = payload.CompletedPickupStops
	}
	if state.Domain == "" {
		state.Domain = order.Domain
	}
	if state.ReadyFrom == nil || state.ReadyTo == nil {
		var from, to *time.Time
		if readErr := tx.QueryRow(ctx, `SELECT expected_window_start,expected_window_end
			FROM commerce_vendor_order_readiness WHERE vendor_order_id=$1`, order.ID).Scan(&from, &to); readErr == nil && from != nil && to != nil {
			state.ReadyFrom, state.ReadyTo = from, to
		}
	}
	providerRef := strings.TrimSpace(payload.NewProviderRef)
	if providerRef == "" {
		providerRef = strings.TrimSpace(payload.ProviderRef)
	}
	if providerRef != "" {
		state.ProviderRef, state.ProviderAssigned = providerRef, true
	}
	eventType := strings.ToUpper(strings.TrimSpace(event.EventType))
	if payload.State != "" {
		state.MissionState = strings.ToUpper(payload.State)
	}
	switch eventType {
	case "MISSION_CREATED":
		if state.MissionState == "" {
			state.MissionState = "SEARCHING"
		}
	case "PROVIDER_ASSIGNED":
		state.ProviderAssigned, state.OperationalException = true, false
		state.MissionState = "ASSIGNED"
	case "PROVIDER_REASSIGNED":
		state.ProviderAssigned, state.ProviderArrived, state.OperationalException = providerRef != "", false, true
		state.MissionState = "TRANSFER_PENDING"
	case "PROVIDER_ARRIVED":
		state.ProviderAssigned, state.ProviderArrived = true, true
	case "PICKED_UP":
		if state.CompletedPickupStops < state.PickupStopCount {
			state.CompletedPickupStops++
		}
		state.PickedUp = state.PickupStopCount > 0 && state.CompletedPickupStops >= state.PickupStopCount
		state.MissionState = "IN_TRANSIT"
	case "STOP_COMPLETED":
		if state.CompletedPickupStops < state.PickupStopCount {
			state.CompletedPickupStops++
		}
	case "DELIVERING":
		state.PickedUp, state.MissionState = true, "IN_TRANSIT"
	case "DELIVERED":
		state.PickedUp, state.MissionState = true, "COMPLETED"
	case "DELIVERY_FAILED":
		state.MissionState = "FAILED"
	case "RETURNING":
		state.MissionState = "RETURNING"
	case "OPERATIONAL_EXCEPTION":
		state.OperationalException = true
	}
	if state.RouteSeconds <= 0 || state.RouteProvider == "" || state.RouteEvidenceVersion == "" || state.RouteObservedAt.IsZero() {
		return DeliveryETA{}, false, nil
	}
	groupID := ""
	_ = tx.QueryRow(ctx, `SELECT id FROM commerce_fulfilment_groups WHERE parent_order_id=$1`, event.ParentOrderID).Scan(&groupID)
	sourceEventID := event.EventID
	if before, _, found := strings.Cut(sourceEventID, ":"); found {
		sourceEventID = before
	}
	input := ETAInput{
		ParentOrderID: event.ParentOrderID, FulfilmentGroupID: groupID, MissionSlotRef: event.MissionRef,
		PlanVersion: state.PlanVersion, SourceVersion: event.SourceVersion, SourceEventID: sourceEventID,
		SourceAuthority:  strings.ToUpper(string(event.Source)),
		SourceOccurredAt: event.OccurredAt, Trigger: etaTriggerFor(eventType, payload.ReasonCode),
		Mode: state.Mode, Domain: state.Domain, MissionState: state.MissionState,
		PromisedLatest: state.PromisedLatest, ReadyFrom: state.ReadyFrom, ReadyTo: state.ReadyTo,
		ActualReady: state.ActualReady, ProviderAssigned: state.ProviderAssigned,
		ProviderArrived: state.ProviderArrived, PickedUp: state.PickedUp, ProviderRef: state.ProviderRef,
		RouteSeconds: state.RouteSeconds, RouteProvider: state.RouteProvider,
		RouteEvidenceVersion: state.RouteEvidenceVersion, RouteObservedAt: state.RouteObservedAt,
		PickupStopCount: state.PickupStopCount, CompletedPickupStops: state.CompletedPickupStops,
		OperationalException: state.OperationalException,
	}
	return s.applyConfidenceETATx(ctx, tx, input)
}

// ProjectReadinessETA implements production.ETAReadinessProjector. The
// Production domain supplies its authoritative range while this Orders-owned
// projector combines it with the already stored route/mission evidence.
func (s *Service) ProjectReadinessETA(ctx context.Context, tx pgx.Tx, vendorOrderID, parentOrderID string,
	windowStart, windowEnd *time.Time, delayed, actualReady bool, sourceEventID string,
	sourceVersion int64, occurredAt time.Time) error {
	if s == nil || s.store == nil || tx == nil || sourceVersion < 1 || strings.TrimSpace(sourceEventID) == "" {
		return ErrDependencyMissing
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT e.mission_slot_ref,e.source_snapshot
		FROM commerce_delivery_eta_current c JOIN commerce_delivery_eta_versions e ON e.id=c.eta_id
		JOIN commerce_fulfilment_groups g ON g.parent_order_id=e.parent_order_id
		JOIN commerce_fulfilment_plan_versions pv ON pv.id=g.current_plan_id
		JOIN commerce_fulfilment_plan_missions pm ON pm.plan_id=pv.id AND pm.slot_ref=e.mission_slot_ref
		JOIN commerce_fulfilment_plan_mission_parcels mp ON mp.plan_mission_id=pm.id
		JOIN commerce_fulfilment_parcels fp ON fp.id=mp.parcel_id
		WHERE e.parent_order_id=$1 AND fp.vendor_order_id=$2`, parentOrderID, vendorOrderID)
	if err != nil {
		return err
	}
	type target struct {
		mission string
		state   etaSourceState
	}
	var targets []target
	for rows.Next() {
		var mission string
		var raw []byte
		if err = rows.Scan(&mission, &raw); err != nil {
			rows.Close()
			return err
		}
		var state etaSourceState
		if json.Unmarshal(raw, &state) != nil {
			rows.Close()
			return ErrOrderConflict
		}
		targets = append(targets, target{mission: mission, state: state})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	groupID := ""
	_ = tx.QueryRow(ctx, `SELECT id FROM commerce_fulfilment_groups WHERE parent_order_id=$1`, parentOrderID).Scan(&groupID)
	for _, item := range targets {
		item.state.ReadyFrom, item.state.ReadyTo, item.state.ActualReady = windowStart, windowEnd, actualReady
		trigger := TriggerVendorEarlyReady
		if delayed {
			trigger = TriggerVendorDelay
		}
		_, _, err = s.store.applyConfidenceETATx(ctx, tx, ETAInput{
			ParentOrderID: parentOrderID, FulfilmentGroupID: groupID, MissionSlotRef: item.mission,
			PlanVersion: item.state.PlanVersion, SourceVersion: sourceVersion, SourceEventID: sourceEventID,
			SourceAuthority: "PRODUCTION", SourceOccurredAt: occurredAt.UTC(), Trigger: trigger,
			Mode: item.state.Mode, Domain: item.state.Domain, MissionState: item.state.MissionState,
			PromisedLatest: item.state.PromisedLatest, ReadyFrom: item.state.ReadyFrom, ReadyTo: item.state.ReadyTo,
			ActualReady: item.state.ActualReady, ProviderAssigned: item.state.ProviderAssigned,
			ProviderArrived: item.state.ProviderArrived, PickedUp: item.state.PickedUp,
			ProviderRef: item.state.ProviderRef, RouteSeconds: item.state.RouteSeconds,
			RouteProvider: item.state.RouteProvider, RouteEvidenceVersion: item.state.RouteEvidenceVersion,
			RouteObservedAt: item.state.RouteObservedAt, PickupStopCount: item.state.PickupStopCount,
			CompletedPickupStops: item.state.CompletedPickupStops,
			OperationalException: item.state.OperationalException,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
