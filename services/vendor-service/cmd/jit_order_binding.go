package main

// The two adapters that join the paid order to the just-in-time orchestration.
//
// They point in opposite directions and both live here rather than in either
// feature, which is the point: orders must not learn the orchestration's
// persistence and the orchestration must not learn how a commerce mission is
// assembled. The composition root is the only place allowed to know both.
//
//	orders  --PlanMission-->  orchestration     (defer this handover)
//	orders  <--CreateMission--  orchestration   (it is due; dispatch it now)
//
// Neither adapter assembles a mission and neither decides a price. The
// orchestration decides WHEN, WHICH AUTHORITY and WHICH PRODUCT; the orders
// feature decides what the mission contains, exactly as it does on the legacy
// path, from the same code.

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// orchestrationPlanner defers a durable mission slot instead of dispatching it.
type orchestrationPlanner struct {
	service *orchestration.Service
}

// PlanMission opens or resolves the one orchestration that owns this slot.
//
// The fulfilment group reference is the slot's own mission reference, not the
// parent order's. That is what gives a split group one orchestration PER
// physical mission: two riders for one order are two independent decisions
// about when to commit, and collapsing them onto the order would make the
// earlier vendor wait for the later one.
func (p orchestrationPlanner) PlanMission(ctx context.Context, planned orders.PlannedMission) error {
	if p.service == nil {
		return errors.New("jit planning is not configured")
	}
	requirement, pickups, err := requirementForPlannedMission(planned)
	if err != nil {
		return err
	}
	authority, _ := logistics.Parse(string(planned.Service))
	opened, err := p.service.Open(ctx, orchestration.OpenRequest{
		FulfilmentGroupRef: planned.MissionRef,
		ParentOrderID:      planned.ParentOrderID,
		Requirement:        requirement,
		Pickups:            pickups,
		PlannedAuthority:   authority,
		PlannedProductCode: planned.VehicleClass,
	})
	if err != nil {
		return err
	}
	// Record the binding itself, once. It is what makes the plan attributable
	// to a real customer order rather than to a worker that woke up.
	return p.service.RecordOrderBinding(ctx, opened.ID, planned.ParentOrderID, planned.MissionRef)
}

// CancelMissionPlan withdraws an uncommitted plan.
//
// A plan the authority already owns is NOT withdrawn here; false is returned so
// the order flow falls through to the authority's own cancellation contract.
// Reporting success would leave Vendor believing nothing is happening while a
// rider is on their way.
func (p orchestrationPlanner) CancelMissionPlan(
	ctx context.Context, missionRef, reasonCode string,
) (bool, error) {
	if p.service == nil {
		return false, nil
	}
	_, cancelled, err := p.service.CancelByGroup(ctx, missionRef, reasonCode)
	if errors.Is(err, orchestration.ErrAlreadyCommitted) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return cancelled, nil
}

// requirementForPlannedMission projects the frozen Section 17F execution
// evidence onto the orchestration's requirement and pickups.
//
// Every value here is READ from evidence the checkout already froze. Nothing is
// derived, defaulted or estimated: a requirement invented at planning time
// would be a claim about goods nobody weighed.
func requirementForPlannedMission(planned orders.PlannedMission) (
	orchestration.Requirement, []orchestration.Pickup, error) {
	execution := planned.Execution
	if len(execution.Stops) == 0 {
		return orchestration.Requirement{}, nil, errors.New(
			"planned mission carries no pickup stops")
	}

	requirement := orchestration.Requirement{
		DropoffCount:     1,
		DropoffLatitude:  planned.Delivery.Latitude,
		DropoffLongitude: planned.Delivery.Longitude,
		DeliveryDeadline: execution.PromisedBy,
	}

	pickups := make([]orchestration.Pickup, 0, len(execution.Stops))
	handling := map[string]bool{}
	var earliest, latest time.Time

	for index, stop := range execution.Stops {
		pickup := orchestration.Pickup{
			VendorOrderID:   stopVendorOrderID(execution, stop),
			Sequence:        index + 1,
			BusinessID:      stop.BusinessID,
			BranchID:        stop.BranchID,
			PickupLatitude:  stop.Latitude,
			PickupLongitude: stop.Longitude,
		}
		for _, parcel := range execution.Parcels {
			if parcel.UnitRef != stop.UnitRef {
				continue
			}
			pickup.WeightGrams += parcel.WeightGrams
			pickup.PackageCount += parcel.PackageCount
			pickup.VolumeMl += volumeMlOf(parcel)
			if band := strings.TrimSpace(parcel.TemperatureControl); band != "" {
				pickup.TemperatureBand = band
			}
			for _, code := range orchestrationParcelHandling(parcel) {
				handling[code] = true
				pickup.Handling = append(pickup.Handling, code)
			}
			requirement.LongestDimensionMm = maxInt64(
				requirement.LongestDimensionMm, longestEdgeOf(parcel))
		}
		requirement.WeightGrams += pickup.WeightGrams
		requirement.PackageCount += pickup.PackageCount
		requirement.VolumeMl += pickup.VolumeMl
		if pickup.TemperatureBand != "" {
			requirement.TemperatureBand = pickup.TemperatureBand
		}

		if !stop.ReadyFrom.IsZero() && (earliest.IsZero() || stop.ReadyFrom.Before(earliest)) {
			earliest = stop.ReadyFrom.UTC()
		}
		// The window ends when the LAST stop is ready. A vehicle cannot collect
		// the group before its slowest member is done, so taking the earliest
		// would arm the commit while a kitchen is still cooking.
		if !stop.ReadyTo.IsZero() && stop.ReadyTo.After(latest) {
			latest = stop.ReadyTo.UTC()
		}
		pickups = append(pickups, pickup)
	}

	requirement.PickupCount = int64(len(pickups))
	requirement.DesiredPickupStart = earliest
	requirement.DesiredPickupEnd = latest
	for code := range handling {
		requirement.Handling = append(requirement.Handling, code)
	}
	sortStrings(requirement.Handling)

	// The first stop anchors the group's coordinate for serviceability and
	// supply questions, matching how the requirement's single pickup point is
	// used everywhere else.
	requirement.PickupLatitude = pickups[0].PickupLatitude
	requirement.PickupLongitude = pickups[0].PickupLongitude

	return requirement, pickups, nil
}

// stopVendorOrderID finds the allocation a stop collects from.
func stopVendorOrderID(execution orders.MissionExecution, stop orders.MissionStop) string {
	for _, parcel := range execution.Parcels {
		if parcel.UnitRef == stop.UnitRef && parcel.VendorOrderID != "" {
			return parcel.VendorOrderID
		}
	}
	// A stop with no parcel still has to be identifiable, and its unit
	// reference is unique within the mission.
	return stop.UnitRef
}

// orchestrationParcelHandling names the handling constraints the grouping
// policy reasons about. It is distinct from the dispatch adapter's own
// Handling projection: that one speaks Dispatch's wire vocabulary, this one
// speaks the planner's.
func orchestrationParcelHandling(parcel orders.MissionParcel) []string {
	codes := []string{}
	if parcel.Fragile {
		codes = append(codes, "FRAGILE")
	}
	if parcel.Upright {
		codes = append(codes, "UPRIGHT")
	}
	if parcel.StackRestricted {
		codes = append(codes, "NO_STACK")
	}
	return codes
}

// volumeMlOf converts a parcel's millimetre dimensions to millilitres.
//
// An undeclared dimension yields zero rather than a guess: the grouping policy
// treats an unknown bound as UNKNOWN and refuses to fill it, which is the safe
// direction.
func volumeMlOf(parcel orders.MissionParcel) int64 {
	if parcel.LengthMM <= 0 || parcel.WidthMM <= 0 || parcel.HeightMM <= 0 {
		return 0
	}
	// 1 ml is 1 cm^3, which is 1000 mm^3.
	return parcel.LengthMM * parcel.WidthMM * parcel.HeightMM / 1000
}

func longestEdgeOf(parcel orders.MissionParcel) int64 {
	return maxInt64(parcel.LengthMM, maxInt64(parcel.WidthMM, parcel.HeightMM))
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// productionMissionCommitter is the real committer.
//
// It creates nothing itself. Both halves delegate to the orders feature, which
// owns mission assembly, so a just-in-time mission is produced by exactly the
// code that produces a legacy one.
type productionMissionCommitter struct {
	orders *orders.Service
}

// CreateMission dispatches the slot the orchestration decided is due.
func (c productionMissionCommitter) CreateMission(
	ctx context.Context, current orchestration.Orchestration, _ []orchestration.Pickup,
) (orchestration.MissionOutcome, error) {
	if c.orders == nil {
		return orchestration.MissionOutcome{}, errors.New("orders service is not configured")
	}
	outcome, err := c.orders.CommitPlannedMission(ctx,
		current.FulfilmentGroupRef,
		orders.LogisticsService(current.SelectedAuthority),
		current.SelectedProductCode)
	if err != nil {
		// An error here is NOT a definite refusal. The orchestration reads an
		// indefinite outcome as "ask the authority what happened", which is
		// the safe reading of a failure we cannot classify.
		return orchestration.MissionOutcome{}, err
	}
	return orchestration.MissionOutcome{
		Definite:    outcome.Definite,
		Accepted:    outcome.Accepted,
		MissionRef:  outcome.MissionRef,
		ExternalRef: outcome.ExternalRef,
		RefusalCode: outcome.RefusalCode,
	}, nil
}

// ReadMission answers whether the authority actually holds this mission.
//
// found=false means definitely absent, which is what permits a safe retry under
// the same identity. An authority that could not be reached returns an error
// instead, and the orchestration keeps reconciling rather than creating again.
func (c productionMissionCommitter) ReadMission(
	ctx context.Context, _ logistics.Authority, missionRef string,
) (orchestration.MissionOutcome, bool, error) {
	if c.orders == nil {
		return orchestration.MissionOutcome{}, false, errors.New("orders service is not configured")
	}
	outcome, found, err := c.orders.ReadPlannedMission(ctx, missionRef)
	if err != nil || !found {
		return orchestration.MissionOutcome{}, false, err
	}
	return orchestration.MissionOutcome{
		Definite:    outcome.Definite,
		Accepted:    outcome.Accepted,
		MissionRef:  outcome.MissionRef,
		ExternalRef: outcome.ExternalRef,
		RefusalCode: outcome.RefusalCode,
	}, true, nil
}
