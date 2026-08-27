package main

// Composition root for the operational fulfilment aggregate.
//
// This is where the orders feature and the fulfilment aggregate meet, and
// nowhere else. Orders never imports the aggregate and the aggregate never
// imports orders: each speaks its own vocabulary, and the translation lives
// here so neither one inherits the other's shape.
//
// The ordering this adapter enforces is the point of it:
//
//	group + units + plan version + mission slot reservations  (durable)
//	                     ↓
//	          logistics authority is called
//	                     ↓
//	        answer is linked to the slot it was requested for
//
// A crash between the second and third step leaves a REQUESTED reservation
// that recovery can read back against the authority. A crash before the first
// leaves nothing to reconcile, which is why the reservation must not be
// written after the call.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// fulfilmentOperationsProjection adapts the fulfilment aggregate to the order
// feature's narrow port.
type fulfilmentOperationsProjection struct {
	fulfilment *fulfilmentgroup.Service
	db         *pgxpool.Pool
}

func newFulfilmentOperationsProjection(
	service *fulfilmentgroup.Service, databases ...*pgxpool.Pool,
) *fulfilmentOperationsProjection {
	if service == nil || !service.Enabled() {
		return nil
	}
	projection := &fulfilmentOperationsProjection{fulfilment: service}
	if len(databases) > 0 {
		projection.db = databases[0]
	}
	return projection
}

// OpenFulfilment makes the fulfilment aggregate durable for one activating
// order.
//
// Everything it does is idempotent, because activation is re-run after any
// failure and by the recovery path: the group is opened or found, each unit is
// opened or found, and the plan version is keyed on the order so a second pass
// reads back the version it already wrote instead of appending another one.
func (p *fulfilmentOperationsProjection) OpenFulfilment(
	ctx context.Context, order orders.ParentOrder, plans []orders.MissionPlan,
) error {
	if p == nil || p.fulfilment == nil {
		return nil
	}

	group, err := p.fulfilment.OpenGroup(ctx, order.ID, order.CustomerID)
	if err != nil {
		return err
	}

	// One unit per fulfillable allocation. Two branches of the same business
	// are two allocations and therefore two units: they are two places a rider
	// physically has to go, and merging them because the owning company
	// matches is how a pickup gets silently skipped.
	units := map[string]fulfilmentgroup.Unit{}
	parcels := map[string]fulfilmentgroup.Parcel{}
	for _, vendorOrder := range order.VendorOrders {
		if vendorOrder.Status.Terminal() {
			continue
		}
		unit, unitErr := p.fulfilment.OpenUnit(ctx, group, fulfilmentgroup.UnitInput{
			VendorOrderID:        vendorOrder.ID,
			PickupAddress:        "Vendor branch " + vendorOrder.BranchID,
			ServiceClass:         vendorOrder.Domain,
			VehicleClass:         vehicleClassFor(vendorOrder, plans),
			HandlingRequirements: nil,
		})
		if unitErr != nil {
			return unitErr
		}
		units[vendorOrder.ID] = unit

		manifest := fulfilmentgroup.Manifest{PackageCount: 1}
		for _, line := range vendorOrder.Lines {
			manifest.Lines = append(manifest.Lines, fulfilmentgroup.ManifestLine{
				OrderLineRef: line.ID, ItemRef: line.ItemRef,
				Description: line.ItemName, Quantity: line.Quantity,
				UnitRef: line.SellUnitRef,
			})
		}
		parcel, parcelErr := p.fulfilment.EnsureOrderParcel(ctx, unit.ID, fulfilmentgroup.ParcelInput{
			IdempotencyKey: "order-activation-" + order.ID + "-" + vendorOrder.ID,
			Manifest:       manifest,
		})
		if parcelErr != nil {
			return parcelErr
		}
		parcels[vendorOrder.ID] = parcel
	}

	missions := p.planMissionsWithParcels(ctx, order, plans, units, parcels)
	if len(missions) == 0 {
		// Nothing to execute — a pickup-only order, or every allocation
		// already terminal. There is no plan to write, and writing an empty
		// one would claim a fulfilment decision nobody made.
		return nil
	}

	// The plan is keyed on the order's activation, so re-running activation
	// reads back the same version rather than appending a second identical
	// one.
	plan, err := p.fulfilment.ActivatePlan(ctx, group.ID, fulfilmentgroup.PlanInput{
		ExpectedVersion:  group.CurrentPlanVersion,
		Reason:           planReasonFor(group.CurrentPlanVersion),
		PolicyVersion:    0,
		DeliveryMode:     deliveryModeFor(order),
		PlannedCostMinor: plannedCostFor(plans),
		Currency:         order.Currency,
		IdempotencyKey:   "order-activation-" + order.ID,
		Missions:         missions,

		// The vehicle-selection evidence commits with the plan it justifies.
		// A plan version that exists without it is a delivery nobody can
		// explain, and the explanation is wanted precisely when something has
		// gone wrong.
		RequirementDecisions: requirementEvidenceFor(
			plans, fulfilment.CapabilityPolicy{}, time.Now().UTC(),
		),
	})
	if err != nil {
		// A stale plan here means something else already replanned this group.
		// That newer plan is the one in force, and overwriting it with this
		// older decision is exactly what must not happen.
		if err == fulfilmentgroup.ErrStalePlan {
			return nil
		}
		return err
	}

	return p.reserveMissions(ctx, group.ID, plan)
}

// reserveMissions makes a durable reservation for every slot the plan says a
// platform authority must execute, and opens the canonical mission for every
// slot the vendor executes itself.
func (p *fulfilmentOperationsProjection) reserveMissions(
	ctx context.Context, groupID string, plan fulfilmentgroup.Plan,
) error {
	for _, mission := range plan.Missions {
		if mission.Owner == fulfilmentgroup.OwnerVendor {
			if _, err := p.fulfilment.OpenVendorMission(
				ctx, groupID, plan.ID, mission.ID); err != nil {
				return err
			}
			continue
		}
		if _, _, err := p.fulfilment.ReserveMission(
			ctx, groupID, plan.ID, mission.ID, mission.Authority); err != nil {
			return err
		}
	}
	return nil
}

// planMissions translates the order feature's mission plans into plan mission
// slots.
//
// SlotRef is deliberately the Section 17C plan mission reference. That is the
// same reference a logistics authority echoes back on its own events, so the
// correlation from an inbound event to the slot it belongs to is an equality
// check rather than a guess.
func (p *fulfilmentOperationsProjection) planMissions(
	order orders.ParentOrder,
	plans []orders.MissionPlan,
	units map[string]fulfilmentgroup.Unit,
) []fulfilmentgroup.PlanMission {
	return p.planMissionsWithParcels(context.Background(), order, plans, units, nil)
}

func (p *fulfilmentOperationsProjection) planMissionsWithParcels(
	ctx context.Context,
	order orders.ParentOrder,
	plans []orders.MissionPlan,
	units map[string]fulfilmentgroup.Unit,
	parcels map[string]fulfilmentgroup.Parcel,
) []fulfilmentgroup.PlanMission {
	destination := fulfilmentgroup.Stop{
		Address:   order.Address.Address,
		Latitude:  order.Address.Latitude,
		Longitude: order.Address.Longitude,
	}

	missions := make([]fulfilmentgroup.PlanMission, 0, len(plans))
	sequence := 0

	for _, plan := range plans {
		var stops []fulfilmentgroup.Stop
		for _, vendorOrderID := range plan.VendorOrderIDs {
			unit, found := units[vendorOrderID]
			if !found {
				continue
			}
			address, latitude, longitude := p.branchPoint(ctx, unit.BusinessID, unit.BranchID, unit.PickupAddress)
			stops = append(stops, fulfilmentgroup.Stop{
				Sequence:   len(stops) + 1,
				Type:       fulfilmentgroup.StopPickup,
				UnitID:     unit.ID,
				BusinessID: unit.BusinessID,
				BranchID:   unit.BranchID,
				Address:    address,
				Latitude:   latitude,
				Longitude:  longitude,
			})
		}
		if len(stops) == 0 {
			continue
		}

		// Exactly one receiver destination, appended last.
		dropoff := destination
		dropoff.Sequence = len(stops) + 1
		dropoff.Type = fulfilmentgroup.StopDropoff
		stops = append(stops, dropoff)

		// A slot covering several allocations is a consolidation the
		// checkout-time planner already made, and it only clusters groups
		// whose pickup points are within a tight shared radius — precisely
		// because no logistics authority models a multi-pickup mission. So
		// the cluster really is one physical collection, the single-pickup
		// mission contract really can execute it, and this translation
		// preserves that decision rather than second-guessing it.
		//
		// What is new is that the plan now RECORDS each allocation as its own
		// pickup stop. The execution call is unchanged; the traceability is
		// not. See authorityCapabilities for what the executing services can
		// actually represent today.
		sequence++
		parcelIDs := make([]string, 0, len(plan.VendorOrderIDs))
		for _, vendorOrderID := range plan.VendorOrderIDs {
			if parcel, found := parcels[vendorOrderID]; found {
				parcelIDs = append(parcelIDs, parcel.ID)
			}
		}
		missions = append(missions, fulfilmentgroup.PlanMission{
			SlotRef:          plan.PlanMissionRef,
			Sequence:         sequence,
			Owner:            fulfilmentgroup.OwnerPlatform,
			Authority:        authorityFor(plan.Service),
			ServiceClass:     "",
			VehicleClass:     plan.VehicleClass,
			PlannedCostMinor: plan.AgreedFeeMinor,
			Currency:         order.Currency,
			Stops:            stops,
			ParcelIDs:        parcelIDs,
		})
	}

	// Vendor-owned allocations execute themselves. Each gets its own slot:
	// vendor-owned delivery is the vendor's own vehicle, and consolidating two
	// vendors' own deliveries would put one vendor's goods in another's van.
	for _, vendorOrder := range order.VendorOrders {
		if vendorOrder.DeliveryResponsibility != orders.DeliveryVendorOwn ||
			vendorOrder.Status.Terminal() {
			continue
		}
		unit, found := units[vendorOrder.ID]
		if !found {
			continue
		}

		dropoff := destination
		dropoff.Sequence = 2
		dropoff.Type = fulfilmentgroup.StopDropoff

		sequence++
		missions = append(missions, fulfilmentgroup.PlanMission{
			SlotRef:   "vendor-" + vendorOrder.ID,
			Sequence:  sequence,
			Owner:     fulfilmentgroup.OwnerVendor,
			Authority: fulfilmentgroup.AuthorityVendor,
			Currency:  order.Currency,
			Stops: []fulfilmentgroup.Stop{
				{
					Sequence:   1,
					Type:       fulfilmentgroup.StopPickup,
					UnitID:     unit.ID,
					BusinessID: unit.BusinessID,
					BranchID:   unit.BranchID,
					Address:    unit.PickupAddress,
				},
				dropoff,
			},
			ParcelIDs: func() []string {
				if parcel, ok := parcels[vendorOrder.ID]; ok {
					return []string{parcel.ID}
				}
				return nil
			}(),
			PlannedCostMinor: vendorOrder.DeliveryGrossMinor,
		})
	}

	return missions
}

func (p *fulfilmentOperationsProjection) branchPoint(
	ctx context.Context, businessID, branchID, fallback string,
) (string, float64, float64) {
	if p == nil || p.db == nil {
		return fallback, 0, 0
	}
	var line1, line2, city, region string
	var latitude, longitude float64
	err := p.db.QueryRow(ctx, `
		SELECT address_line1,COALESCE(address_line2,''),city,region,latitude,longitude
		  FROM vendor_branches WHERE id=$1 AND business_id=$2`,
		branchID, businessID).Scan(&line1, &line2, &city, &region, &latitude, &longitude)
	if err != nil {
		return fallback, 0, 0
	}
	address := strings.Join(nonEmpty(line1, line2, city, region), ", ")
	return address, latitude, longitude
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

// LinkMission binds the authority's answer to the slot it was requested for.
//
// Nothing here creates a mission. It records what an authority already said,
// which is the only thing a projection is entitled to do.
func (p *fulfilmentOperationsProjection) LinkMission(
	ctx context.Context, order orders.ParentOrder,
	plan orders.MissionPlan, result orders.MissionResult,
) error {
	if p == nil || p.fulfilment == nil {
		return nil
	}

	ref, err := p.missionRefForSlot(ctx, order, plan)
	if err != nil || ref == "" {
		// No reservation for this slot means the plan was superseded between
		// activation and this answer. The newer plan owns the slot now, and
		// forcing this answer onto it would bind a mission to work that has
		// already moved.
		return err
	}

	_, err = p.fulfilment.LinkMission(ctx, ref, fulfilmentgroup.MissionLink{
		ExternalRef: result.ExternalRef,
		Accepted:    result.Status == orders.MissionAccepted,
		RefusalCode: refusalCodeFor(result),
	})
	if err == fulfilmentgroup.ErrConflict || err == fulfilmentgroup.ErrNotFound {
		// Already answered, or the slot moved. Both are states a retry must
		// tolerate rather than fail on.
		return nil
	}
	return err
}

// MissionExecution resolves the exact active plan slot and stable parcel
// identities that Dispatch must execute. No request DTO participates in this
// translation; every value comes from Vendor's committed order/fulfilment
// records.
func (p *fulfilmentOperationsProjection) MissionExecution(
	ctx context.Context, order orders.ParentOrder, requested orders.MissionPlan,
) (orders.MissionExecution, error) {
	if p == nil || p.fulfilment == nil {
		return orders.MissionExecution{}, fulfilmentgroup.ErrDependencyMissing
	}
	ref, found, err := p.fulfilment.LiveMissionRefForSlot(ctx, order.ID, requested.PlanMissionRef)
	if err != nil {
		return orders.MissionExecution{}, err
	}
	if !found {
		return orders.MissionExecution{}, fulfilmentgroup.ErrNotFound
	}
	plan, err := p.fulfilment.GetPlan(ctx, ref.GroupID, ref.PlanID)
	if err != nil {
		return orders.MissionExecution{}, err
	}
	var slot *fulfilmentgroup.PlanMission
	for index := range plan.Missions {
		if plan.Missions[index].ID == ref.PlanMissionID {
			slot = &plan.Missions[index]
			break
		}
	}
	if slot == nil {
		return orders.MissionExecution{}, fulfilmentgroup.ErrNotFound
	}

	parcels, err := p.fulfilment.ParcelsForMissionSlot(ctx, slot.ID)
	if err != nil {
		return orders.MissionExecution{}, err
	}
	byUnit := make(map[string][]fulfilmentgroup.Parcel)
	for _, parcel := range parcels {
		byUnit[parcel.UnitID] = append(byUnit[parcel.UnitID], parcel)
	}

	now := time.Now().UTC()
	execution := orders.MissionExecution{
		PlanRef: plan.ID, PlanVersion: plan.Version, PlanFingerprint: plan.Fingerprint,
		Mode: order.SelectedFulfilmentMode, ServiceClass: missionServiceClass(order, requested),
	}
	for _, stop := range slot.PickupStops() {
		readyFrom, readyTo := p.readyWindow(ctx, order.ID, byUnit[stop.UnitID], now)
		execution.Stops = append(execution.Stops, orders.MissionStop{
			StopRef: stop.ID, PhysicalStopRef: physicalStopRef(stop), UnitRef: stop.UnitID,
			BusinessID: stop.BusinessID, BranchID: stop.BranchID,
			Address: stop.Address, Latitude: stop.Latitude, Longitude: stop.Longitude,
			ReadyFrom: readyFrom, ReadyTo: readyTo,
		})
		for _, parcel := range byUnit[stop.UnitID] {
			manifest, marshalErr := json.Marshal(parcel.Manifest)
			if marshalErr != nil {
				return orders.MissionExecution{}, marshalErr
			}
			execution.Parcels = append(execution.Parcels, orders.MissionParcel{
				ParcelRef: parcel.ParcelRef, UnitRef: parcel.UnitID,
				VendorOrderID: parcel.VendorOrderID, BusinessID: parcel.BusinessID,
				BranchID: parcel.BranchID, Manifest: manifest,
				ManifestVersion: parcel.ManifestVersion, WeightGrams: parcel.WeightGrams,
				LengthMM: parcel.LengthMm, WidthMM: parcel.WidthMm, HeightMM: parcel.HeightMm,
				PackageCount: parcel.Manifest.PackageCount, Fragile: parcel.Fragile,
				TemperatureControl: string(parcel.TemperatureControl),
			})
		}
	}
	latestReady := now
	for _, stop := range execution.Stops {
		if stop.ReadyTo.After(latestReady) {
			latestReady = stop.ReadyTo
		}
	}
	execution.PromisedBy = latestReady.Add(2 * time.Hour)
	return execution, nil
}

func missionServiceClass(order orders.ParentOrder, mission orders.MissionPlan) string {
	classes := map[string]bool{}
	selected := map[string]bool{}
	for _, id := range mission.VendorOrderIDs {
		selected[id] = true
	}
	for _, vendorOrder := range order.VendorOrders {
		if selected[vendorOrder.ID] {
			classes[strings.ToUpper(strings.TrimSpace(vendorOrder.Domain))] = true
		}
	}
	if len(classes) == 1 {
		for class := range classes {
			return class
		}
	}
	return "COMMERCE"
}

func physicalStopRef(stop fulfilmentgroup.Stop) string {
	if stop.Latitude != 0 || stop.Longitude != 0 {
		return fmt.Sprintf("geo:%.6f:%.6f", stop.Latitude, stop.Longitude)
	}
	return "branch:" + stop.BranchID
}

func (p *fulfilmentOperationsProjection) readyWindow(
	ctx context.Context, parentOrderID string, parcels []fulfilmentgroup.Parcel, now time.Time,
) (time.Time, time.Time) {
	from, to := now, now.Add(60*time.Minute)
	if p == nil || p.db == nil || len(parcels) == 0 {
		return from, to
	}
	var storedFrom, storedTo *time.Time
	err := p.db.QueryRow(ctx, `
		SELECT logistics_window_start,logistics_window_end
		  FROM commerce_vendor_order_readiness
		 WHERE parent_order_id=$1 AND vendor_order_id=$2`,
		parentOrderID, parcels[0].VendorOrderID).Scan(&storedFrom, &storedTo)
	if err == nil && storedFrom != nil && storedTo != nil && storedTo.After(*storedFrom) && storedTo.After(now) {
		from, to = storedFrom.UTC(), storedTo.UTC()
		if from.Before(now) {
			from = now
		}
	}
	return from, to
}

// ProjectLogisticsEvent applies an authority's own status and custody
// evidence.
//
// Correlation is by slot reference: the event names the Section 17C plan
// mission, which is exactly the slot reference the plan stored, so no
// identifier has to be parsed for meaning.
func (p *fulfilmentOperationsProjection) ProjectLogisticsEvent(
	ctx context.Context, event orders.LogisticsOrderEvent,
) error {
	if p == nil || p.fulfilment == nil {
		return nil
	}

	ref, err := p.missionRefForMissionSlot(ctx, event.ParentOrderID, event.MissionRef)
	if err != nil || ref == nil {
		return err
	}

	if _, _, err = p.fulfilment.ProjectMissionStatus(
		ctx, ref.MissionRef, event.EventType, event.EventID, event.SourceVersion,
	); err != nil {
		return err
	}

	custodyType, moves := custodyEventFor(event.EventType)
	if !moves {
		return nil
	}

	parcels, err := p.fulfilment.ParcelsForMissionSlot(ctx, ref.PlanMissionID)
	if err != nil {
		return err
	}

	selected := map[string]bool{}
	for _, parcelRef := range event.ParcelRefs {
		selected[strings.TrimSpace(parcelRef)] = true
	}
	for _, parcel := range parcels {
		if len(selected) > 0 && !selected[parcel.ParcelRef] {
			continue
		}
		var operational struct {
			EvidenceID     string `json:"evidenceId"`
			TransferRef    string `json:"transferRef"`
			OldProviderRef string `json:"oldProviderRef"`
			NewProviderRef string `json:"newProviderRef"`
			ReasonCode     string `json:"reasonCode"`
		}
		_ = json.Unmarshal(event.Payload, &operational)
		fromType, fromRef := fulfilmentgroup.CustodianType(""), ""
		toRef := ""
		if custodyType == fulfilmentgroup.CustodyTransferred {
			fromType, fromRef = fulfilmentgroup.CustodianPlatform, operational.OldProviderRef
			toRef = operational.NewProviderRef
		}
		if custodyType == fulfilmentgroup.CustodyReturned {
			fromType, toRef = fulfilmentgroup.CustodianPlatform, parcel.BranchID
		}
		if _, err = p.fulfilment.RecordLogisticsCustody(ctx, fulfilmentgroup.CustodyEvent{
			ParcelID:             parcel.ID,
			GroupID:              parcel.GroupID,
			EventType:            custodyType,
			FromCustodianType:    fromType,
			FromCustodianRef:     fromRef,
			ToCustodianType:      custodianFor(custodyType),
			ToCustodianRef:       toRef,
			Source:               custodySourceFor(event.Source),
			SourceEventID:        event.EventID,
			SourceVersion:        event.SourceVersion,
			PlatformMissionRefID: ref.ID,
			HandoverRef:          operational.TransferRef,
			ProofRef:             operational.EvidenceID,
			ReasonCode:           operational.ReasonCode,
			OccurredAt:           event.OccurredAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *fulfilmentOperationsProjection) missionRefForSlot(
	ctx context.Context, order orders.ParentOrder, plan orders.MissionPlan,
) (string, error) {
	ref, err := p.missionRefForMissionSlot(ctx, order.ID, plan.PlanMissionRef)
	if err != nil || ref == nil {
		return "", err
	}
	return ref.MissionRef, nil
}

func (p *fulfilmentOperationsProjection) missionRefForMissionSlot(
	ctx context.Context, parentOrderID, slotRef string,
) (*fulfilmentgroup.PlatformMissionRef, error) {
	ref, found, err := p.fulfilment.LiveMissionRefForSlot(ctx, parentOrderID, slotRef)
	if err != nil || !found {
		return nil, err
	}
	return &ref, nil
}

// -----------------------------------------------------------------------------
// Authority capability
// -----------------------------------------------------------------------------

// authorityCapabilities states what each logistics authority can actually
// execute today.
//
// Every value is read off the authority's own contract rather than assumed
// here. Dispatch V1 now accepts route-bound multi-pickup missions; Hauling's
// legacy commerce contract still accepts one pickup only.
//
// Taxi is absent. It has no commerce mission ingress, so there is no capability
// to state and nothing that could accidentally select it.
//
// This table is the input to the aggregate's post-order grouping decision
// (fulfilmentgroup.GroupCandidates), which is what a REPLAN uses when it has to
// regroup units itself. It is deliberately not applied to the initial
// translation: the initial grouping was already decided at checkout time under
// a stricter rule (shared pickup point within a tight radius), and re-deciding
// it here would split a cluster that was quoted once and can genuinely be
// collected in one stop.
func authorityCapabilities() map[fulfilmentgroup.Authority]fulfilmentgroup.Capability {
	return map[fulfilmentgroup.Authority]fulfilmentgroup.Capability{
		fulfilmentgroup.AuthorityDispatch: {
			Authority:       fulfilmentgroup.AuthorityDispatch,
			MultiStopPickup: true,
			MaxPickupStops:  32,
			// The physical envelope dispatch's own commerce mission surface
			// already refuses above.
			MaxWeightGrams: 25_000,
		},
		fulfilmentgroup.AuthorityHauling: {
			Authority:       fulfilmentgroup.AuthorityHauling,
			MultiStopPickup: false,
			MaxPickupStops:  1,
		},
	}
}

// Replan regroups a fulfilment group's units and appends the next plan version.
//
// This is the operational replan entry point: a provider that never arrived, a
// capacity or vehicle overflow, a vendor rider that could not go out. It
// supplies the authority capability table so the aggregate never has to assume
// what a logistics service can execute, and it changes no identity — the group,
// its units and its parcels are the same ones afterwards.
//
// The previous version is superseded, not edited. Whatever it said about how
// the delivery was going to happen is what it still says.
func (p *fulfilmentOperationsProjection) Replan(
	ctx context.Context, request fulfilmentgroup.ReplanRequest,
) (fulfilmentgroup.Plan, error) {
	if p == nil || p.fulfilment == nil {
		return fulfilmentgroup.Plan{}, fulfilmentgroup.ErrDependencyMissing
	}
	if request.Capabilities == nil {
		request.Capabilities = authorityCapabilities()
	}
	return p.fulfilment.Replan(ctx, request)
}

// -----------------------------------------------------------------------------
// Vocabulary translation
// -----------------------------------------------------------------------------

func authorityFor(service orders.LogisticsService) fulfilmentgroup.Authority {
	switch service {
	case orders.LogisticsHauling:
		return fulfilmentgroup.AuthorityHauling
	default:
		return fulfilmentgroup.AuthorityDispatch
	}
}

func custodySourceFor(source orders.LogisticsService) fulfilmentgroup.CustodySource {
	switch source {
	case orders.LogisticsHauling:
		return fulfilmentgroup.SourceHauling
	default:
		return fulfilmentgroup.SourceDispatch
	}
}

// custodyEventFor maps a logistics event onto the custody chain.
//
// Only events that actually move the goods produce a custody link. A timing or
// ETA update is real information but it is not a change of hands, and
// recording it as one would make the chain unreadable.
func custodyEventFor(eventType string) (fulfilmentgroup.CustodyEventType, bool) {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "PICKED_UP":
		return fulfilmentgroup.CustodyHandedOver, true
	case "DELIVERED":
		return fulfilmentgroup.CustodyDelivered, true
	case "DELIVERY_FAILED":
		return fulfilmentgroup.CustodyFailed, true
	case "PROVIDER_TRANSFERRED":
		return fulfilmentgroup.CustodyTransferred, true
	case "RETURNING":
		return fulfilmentgroup.CustodyReturnInitiated, true
	case "RETURNED":
		return fulfilmentgroup.CustodyReturned, true
	default:
		return "", false
	}
}

func custodianFor(event fulfilmentgroup.CustodyEventType) fulfilmentgroup.CustodianType {
	switch event {
	case fulfilmentgroup.CustodyDelivered:
		return fulfilmentgroup.CustodianCustomer
	case fulfilmentgroup.CustodyReturned:
		return fulfilmentgroup.CustodianVendor
	default:
		return fulfilmentgroup.CustodianPlatform
	}
}

func planReasonFor(currentVersion int64) fulfilmentgroup.PlanReason {
	if currentVersion == 0 {
		return fulfilmentgroup.ReasonInitialPlan
	}
	return fulfilmentgroup.ReasonProviderUnavailable
}

func deliveryModeFor(order orders.ParentOrder) fulfilmentgroup.DeliveryMode {
	platform, vendor := false, false
	for _, vendorOrder := range order.VendorOrders {
		switch vendorOrder.DeliveryResponsibility {
		case orders.DeliveryPlatform:
			platform = true
		case orders.DeliveryVendorOwn:
			vendor = true
		}
	}
	switch {
	case platform && vendor:
		// Both executors are genuinely in use for this order. HYBRID here is
		// a statement about the plan, not about any one mission: every mission
		// inside it still has exactly one owner.
		return fulfilmentgroup.ModeHybrid
	case vendor:
		return fulfilmentgroup.ModeVendor
	default:
		return fulfilmentgroup.ModePlatform
	}
}

// plannedCostFor sums what the plan believed fulfilment would cost.
//
// Evidence, not a ledger entry: payment-wallet-service remains the authority
// for what was actually charged, and nothing here creates an obligation.
func plannedCostFor(plans []orders.MissionPlan) int64 {
	var total int64
	for _, plan := range plans {
		total += plan.AgreedFeeMinor
	}
	return total
}

func vehicleClassFor(vendorOrder orders.VendorOrder, plans []orders.MissionPlan) string {
	for _, plan := range plans {
		for _, id := range plan.VendorOrderIDs {
			if id == vendorOrder.ID {
				return plan.VehicleClass
			}
		}
	}
	return ""
}

func refusalCodeFor(result orders.MissionResult) string {
	if result.Status == orders.MissionAccepted {
		return ""
	}
	if strings.TrimSpace(result.RefusalCode) != "" {
		return result.RefusalCode
	}
	// A refusal must carry a reason a caller can act on. An authority that
	// declined without one is recorded as unspecified rather than as success.
	return "AUTHORITY_REFUSED_WITHOUT_CODE"
}

var _ orders.FulfilmentProjection = (*fulfilmentOperationsProjection)(nil)
