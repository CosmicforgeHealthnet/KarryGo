package main

import (
	"context"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// ReassignSlotAuthority re-points one plan slot to another platform authority
// after a post-pick physical escalation (Karryam V4 §14.6).
//
// It appends a VEHICLE_OVERFLOW plan version: the group, its units and parcels
// keep their identities and every other slot is copied unchanged. The previous
// version stays readable exactly as written. Idempotent on the key, so a
// replayed escalation reads back the version it already wrote.
func (p *fulfilmentOperationsProjection) ReassignSlotAuthority(
	ctx context.Context, order orders.ParentOrder, planMissionRef string,
	service orders.LogisticsService, idempotencyKey string,
) error {
	if p == nil || p.fulfilment == nil {
		return nil
	}
	group, err := p.fulfilment.OpenGroup(ctx, order.ID, order.CustomerID)
	if err != nil {
		return err
	}
	if !group.Planned() {
		return nil
	}
	current, err := p.fulfilment.GetPlan(ctx, group.ID, group.CurrentPlanID)
	if err != nil {
		return err
	}
	target := authorityFor(service)
	missions := make([]fulfilmentgroup.PlanMission, 0, len(current.Missions))
	changed := false
	for _, mission := range current.Missions {
		copied := fulfilmentgroup.PlanMission{
			SlotRef: mission.SlotRef, Sequence: mission.Sequence, Owner: mission.Owner,
			Authority: mission.Authority, ServiceClass: mission.ServiceClass,
			VehicleClass: mission.VehicleClass, PlannedCostMinor: mission.PlannedCostMinor,
			Currency: mission.Currency, LoadingDurationSeconds: mission.LoadingDurationSeconds,
			Stops: mission.Stops, ParcelIDs: mission.ParcelIDs,
		}
		if strings.TrimSpace(mission.SlotRef) == strings.TrimSpace(planMissionRef) &&
			mission.Owner != fulfilmentgroup.OwnerVendor && mission.Authority != target {
			copied.Authority = target
			changed = true
		}
		missions = append(missions, copied)
	}
	if !changed {
		return nil
	}
	plan, err := p.fulfilment.ActivatePlan(ctx, group.ID, fulfilmentgroup.PlanInput{
		ExpectedVersion:  group.CurrentPlanVersion,
		Reason:           fulfilmentgroup.ReasonVehicleOverflow,
		ReasonDetail:     "PHYSICAL_LOAD_ESCALATION",
		PolicyVersion:    current.PolicyVersion,
		DeliveryMode:     current.DeliveryMode,
		PlannedCostMinor: current.PlannedCostMinor,
		Currency:         current.Currency,
		IdempotencyKey:   idempotencyKey,
		Missions:         missions,
	})
	if err != nil {
		return err
	}
	return p.reserveMissions(ctx, group.ID, plan)
}

// escalationFor reads the physical escalation now executing a plan slot.
func (p *fulfilmentOperationsProjection) escalationFor(
	ctx context.Context, order orders.ParentOrder, planMissionRef string,
) (orders.PhysicalEscalationEvidence, bool, error) {
	if p == nil || p.db == nil {
		return orders.PhysicalEscalationEvidence{}, false, nil
	}
	return orders.NewStore(p.db).LatestPhysicalEscalation(ctx, order.ID, planMissionRef)
}
