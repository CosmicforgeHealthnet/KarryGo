package orders

// Section 17F fulfilment activation.
//
// Activation is what happens once, and only once, a payment is actually HELD:
// the stock each allocation reserved is confirmed, and a logistics mission is
// created for every platform-executed allocation.
//
// Two rules this file exists to keep:
//
//   - Nothing is activated for an unpaid order. An order whose payment is not
//     held has not been paid, and working it would be fulfilling goods nobody
//     has been charged for.
//   - The customer's charge is fixed. A mission is created for the fee the
//     customer already paid, and an authority that answers with a different
//     fee is refused. An internal re-plan is a platform cost, never a
//     customer charge.

import (
	"context"
	"sort"
	"strings"
	"time"
)

// InventoryState is where one allocation's stock stands.
type InventoryState string

const (
	// InventoryNotRequired is a Food allocation, or any allocation that never
	// held Shop stock.
	InventoryNotRequired InventoryState = "NOT_REQUIRED"

	InventoryHeld      InventoryState = "HELD"
	InventoryConfirmed InventoryState = "CONFIRMED"
	InventoryCommitted InventoryState = "COMMITTED"
	InventoryReleased  InventoryState = "RELEASED"
)

// MissionStatus is the state of one logistics mission.
type MissionStatus string

const (
	MissionPending  MissionStatus = "PENDING"
	MissionAccepted MissionStatus = "ACCEPTED"
	MissionRefused  MissionStatus = "REFUSED"
	MissionFailed   MissionStatus = "FAILED"
)

// LogisticsService names the executing authority.
//
// Taxi is deliberately absent. It is not required for commerce, and a mission
// must never be created against an integration that is not ready.
type LogisticsService string

const (
	LogisticsDispatch LogisticsService = "DISPATCH"
	LogisticsHauling  LogisticsService = "HAULING"
)

// OrderMission is one logistics mission for a platform-executed allocation.
type OrderMission struct {
	ID             string           `json:"mission_id"`
	MissionRef     string           `json:"mission_ref"`
	ParentOrderID  string           `json:"parent_order_id"`
	PlanMissionRef string           `json:"plan_mission_ref"`
	Service        LogisticsService `json:"service"`
	VehicleClass   string           `json:"vehicle_class,omitempty"`
	AgreedFeeMinor int64            `json:"agreed_fee_minor"`
	Currency       string           `json:"currency"`
	Status         MissionStatus    `json:"status"`
	ExternalRef    string           `json:"external_ref,omitempty"`
	RefusalCode    string           `json:"refusal_code,omitempty"`
	FailureReason  string           `json:"failure_reason,omitempty"`
	VendorOrderIDs []string         `json:"vendor_order_ids"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InventoryAuthority is the narrow port onto Section 14 stock.
//
// Every method is idempotent from this feature's point of view: a reservation
// already in the target state is not an error, because activation and recovery
// both run more than once.
type InventoryAuthority interface {
	ConfirmReservation(ctx context.Context, businessID, branchID, reservationRef string) error
	CommitReservation(ctx context.Context, businessID, branchID, reservationRef string) error
	ReleaseReservation(ctx context.Context, businessID, branchID, reservationRef string) error
}

// PartialInventoryAuthority is an optional extension implemented by the
// Section 14 authority. Older adapters remain valid and continue to use whole
// reservation transitions when no picking outcome exists.
type PartialInventoryAuthority interface {
	FinalizeReservationPartial(ctx context.Context, businessID, branchID, reservationRef string, fulfilledByItem map[string]int64) error
}

// MissionRequest asks a logistics authority to execute one mission.
type MissionRequest struct {
	MissionRef      string
	ParentOrderID   string
	VendorOrderIDs  []string
	CustomerID      string
	Service         LogisticsService
	VehicleClass    string
	Currency        string
	AgreedFeeMinor  int64
	PlanMissionRef  string
	PickupBusiness  string
	PickupBranch    string
	DeliveryAddress AddressSnapshot
	Receiver        ReceiverSnapshot
	WeightGrams     int64
	PackageCount    int64
}

// MissionResult is the authority's answer.
type MissionResult struct {
	ExternalRef      string
	Status           MissionStatus
	RefusalCode      string
	AcceptedFeeMinor int64
}

// MissionAuthority executes commerce missions.
type MissionAuthority interface {
	// Available reports whether this authority can be called at all.
	Available(service LogisticsService) bool

	// CreateMission must be idempotent on MissionRef, so a retried activation
	// returns the same mission rather than dispatching a second vehicle.
	CreateMission(ctx context.Context, request MissionRequest) (MissionResult, error)
}

// SetFulfilmentAuthorities wires stock and logistics onto the order service.
//
// Separate from NewService so a deployment without them builds a working order
// service that simply cannot activate fulfilment, rather than one that fails
// halfway through activating it.
func (s *Service) SetFulfilmentAuthorities(
	inventory InventoryAuthority, missions MissionAuthority,
) {
	s.inventory = inventory
	s.missions = missions
}

// FulfilmentEnabled reports whether activation can run.
func (s *Service) FulfilmentEnabled() bool {
	return s != nil && s.inventory != nil && s.missions != nil
}

// ActivationOutcome reports what one activation did.
type ActivationOutcome struct {
	InventoryConfirmed int
	MissionsCreated    int
	MissionsRefused    int
	VendorOwnDelivery  int
}

// ActivateFulfilment finalizes stock and creates logistics missions for a paid
// order.
//
// It is safe to call repeatedly and safe to resume after a crash: every step
// is idempotent and every result is durable before the next step runs.
func (s *Service) ActivateFulfilment(
	ctx context.Context, parentOrderID string,
) (ActivationOutcome, error) {
	var outcome ActivationOutcome
	if !s.enabled {
		return outcome, ErrDisabled
	}
	if !s.FulfilmentEnabled() {
		return outcome, ErrDependencyMissing
	}

	order, err := s.store.GetParentByID(ctx, strings.TrimSpace(parentOrderID))
	if err != nil {
		return outcome, err
	}

	// The gate that matters: an order whose payment is not held has not been
	// paid, and nothing about it may be worked as if it were.
	if order.PaymentState != string(PaymentHeld) {
		return outcome, ErrPaymentConflict
	}

	// Stock first. Confirming before a mission exists means a refused mission
	// leaves confirmed stock that cancellation releases, rather than a
	// dispatched vehicle for goods nothing reserved.
	for _, vendorOrder := range order.VendorOrders {
		if !vendorOrderFulfillable(vendorOrder.Status) {
			continue
		}
		if InventoryState(vendorOrder.InventoryState) != InventoryHeld {
			continue
		}
		if err = s.inventory.ConfirmReservation(
			ctx, vendorOrder.BusinessID, vendorOrder.BranchID, vendorOrder.ReservationRef,
		); err != nil {
			return outcome, err
		}
		if err = s.store.SetInventoryState(
			ctx, vendorOrder.ID, InventoryConfirmed, s.now().UTC(),
		); err != nil {
			return outcome, err
		}
		outcome.InventoryConfirmed++
	}

	plans, err := PlanMissions(order)
	if err != nil {
		return outcome, err
	}
	outcome.VendorOwnDelivery = countVendorOwn(order)

	for _, plan := range plans {
		mission, created, ensureErr := s.store.EnsureMission(ctx, order, plan, s.now().UTC())
		if ensureErr != nil {
			return outcome, ensureErr
		}
		_ = created
		if mission.Status != MissionPending {
			// Already answered by the authority. Asking again would risk a
			// second vehicle for the same goods.
			continue
		}
		if !s.missions.Available(plan.Service) {
			// An unavailable authority is left PENDING for recovery rather
			// than recorded as a refusal it never made.
			continue
		}

		result, missionErr := s.missions.CreateMission(ctx, MissionRequest{
			MissionRef:      mission.MissionRef,
			ParentOrderID:   order.ID,
			VendorOrderIDs:  plan.VendorOrderIDs,
			CustomerID:      order.CustomerID,
			Service:         plan.Service,
			VehicleClass:    plan.VehicleClass,
			Currency:        order.Currency,
			AgreedFeeMinor:  plan.AgreedFeeMinor,
			PlanMissionRef:  plan.PlanMissionRef,
			PickupBusiness:  plan.PickupBusiness,
			PickupBranch:    plan.PickupBranch,
			DeliveryAddress: order.Address,
			Receiver:        order.Receiver,
			WeightGrams:     plan.WeightGrams,
			PackageCount:    plan.PackageCount,
		})
		if missionErr != nil {
			if err = s.store.RecordMissionFailure(
				ctx, mission.ID, redactAuthorityError(missionErr), s.now().UTC(),
			); err != nil {
				return outcome, err
			}
			continue
		}

		if err = s.store.RecordMissionResult(ctx, mission.ID, result, s.now().UTC()); err != nil {
			return outcome, err
		}
		switch result.Status {
		case MissionAccepted:
			outcome.MissionsCreated++
		case MissionRefused:
			outcome.MissionsRefused++
		}
	}

	return outcome, nil
}

// MissionPlan is one mission to be created.
type MissionPlan struct {
	PlanMissionRef string
	Service        LogisticsService
	VehicleClass   string
	AgreedFeeMinor int64
	VendorOrderIDs []string
	PickupBusiness string
	PickupBranch   string
	WeightGrams    int64
	PackageCount   int64
}

// PlanMissions groups an order's platform-executed allocations into missions.
//
// Grouping is by the Section 17C plan mission the quote already froze, so a
// consolidated delivery stays one mission and its fee is counted once. A
// vendor-owned allocation produces no mission at all: the vendor is the
// executor.
func PlanMissions(order ParentOrder) ([]MissionPlan, error) {
	byPlanMission := map[string]*MissionPlan{}

	for _, vendorOrder := range order.VendorOrders {
		if vendorOrder.DeliveryResponsibility != DeliveryPlatform {
			// Vendor-owned delivery and pickup-only allocations are executed
			// by the vendor, not by a platform authority.
			continue
		}
		if !vendorOrderFulfillable(vendorOrder.Status) {
			continue
		}
		reference := vendorOrder.LogisticsMissionRef
		if reference == "" {
			// A platform delivery with no plan mission would be a fee nobody
			// can execute against.
			return nil, ErrQuoteNotOrderable
		}

		plan, found := byPlanMission[reference]
		if !found {
			plan = &MissionPlan{
				PlanMissionRef: reference,
				Service:        serviceFor(vendorOrder),
				VehicleClass:   "",
				PickupBusiness: vendorOrder.BusinessID,
				PickupBranch:   vendorOrder.BranchID,
			}
			byPlanMission[reference] = plan
		}
		plan.VendorOrderIDs = append(plan.VendorOrderIDs, vendorOrder.ID)
		// The fee is counted once per plan mission. A consolidated mission
		// covering two allocations is one delivery at one agreed fee, and
		// summing per allocation would double-charge the platform for it.
		plan.AgreedFeeMinor = vendorOrder.DeliveryGrossMinor
		plan.PackageCount++
	}

	plans := make([]MissionPlan, 0, len(byPlanMission))
	for _, plan := range byPlanMission {
		sort.Strings(plan.VendorOrderIDs)
		plans = append(plans, *plan)
	}
	// A stable order keeps activation reproducible across runs.
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].PlanMissionRef < plans[j].PlanMissionRef
	})
	return plans, nil
}

// vendorOrderFulfillable is deliberately stricter than Terminal. A rejected
// or refunded child is terminal evidence, not a candidate for confirmation or
// mission creation. Keeping this predicate in the fulfilment layer prevents a
// rejected sibling from leaking into a consolidated platform mission.
func vendorOrderFulfillable(status VendorStatus) bool {
	switch status {
	case VendorStatusCancelled, VendorStatusRejected, VendorStatusRefundPending,
		VendorStatusRefunded, VendorStatusPartiallyRefunded, VendorStatusCompleted:
		return false
	default:
		return true
	}
}

// serviceFor picks the executing authority for one allocation.
//
// Food and small Shop consignments go to dispatch; a Shop allocation heavy
// enough to need a truck goes to hauling. Taxi is never chosen.
func serviceFor(vendorOrder VendorOrder) LogisticsService {
	if vendorOrder.Domain == "SHOP" && vendorOrder.DeliveryGrossMinor >= haulingFeeThreshold {
		return LogisticsHauling
	}
	return LogisticsDispatch
}

// haulingFeeThreshold is a deliberately coarse split. The authoritative choice
// was already made by Section 17C when it quoted the plan mission; this only
// routes the execution call to the authority that priced it.
const haulingFeeThreshold = 500_000

func countVendorOwn(order ParentOrder) int {
	count := 0
	for _, vendorOrder := range order.VendorOrders {
		if vendorOrder.DeliveryResponsibility == DeliveryVendorOwn {
			count++
		}
	}
	return count
}

// FinalizeInventoryFor moves an allocation's stock to its terminal state.
//
// The timing is the whole point, and it is deliberately explicit:
//
//   - HELD while the checkout reservation stands and nothing is paid;
//   - CONFIRMED once payment is held, so the stock belongs to this purchase;
//   - COMMITTED when the allocation completes, because the goods actually left;
//   - RELEASED when it cancels, returning ONLY this allocation's unconsumed
//     reservation and never a sibling vendor's.
//
// Stock is never double-committed: a reservation already past CONFIRMED is
// left alone, so a replayed transition or a recovery pass is a no-op.
func (s *Service) FinalizeInventoryFor(ctx context.Context, vendorOrder VendorOrder) error {
	if !s.FulfilmentEnabled() || !vendorOrder.ReservationRequired {
		return nil
	}
	state := InventoryState(vendorOrder.InventoryState)
	if state == InventoryCommitted || state == InventoryReleased ||
		state == InventoryNotRequired {
		return nil
	}

	switch vendorOrder.Status {
	case VendorStatusCompleted:
		if partial, ok := s.inventory.(PartialInventoryAuthority); ok {
			if fulfilled, found, err := s.store.FulfilledQuantities(ctx, vendorOrder.ID); err != nil {
				return err
			} else if found {
				if err = partial.FinalizeReservationPartial(ctx, vendorOrder.BusinessID, vendorOrder.BranchID, vendorOrder.ReservationRef, fulfilled); err != nil {
					return err
				}
				return s.store.SetInventoryState(ctx, vendorOrder.ID, InventoryCommitted, s.now().UTC())
			}
		}
		if err := s.inventory.CommitReservation(
			ctx, vendorOrder.BusinessID, vendorOrder.BranchID, vendorOrder.ReservationRef,
		); err != nil {
			return err
		}
		return s.store.SetInventoryState(
			ctx, vendorOrder.ID, InventoryCommitted, s.now().UTC(),
		)

	case VendorStatusCancelled, VendorStatusRejected:
		if partial, ok := s.inventory.(PartialInventoryAuthority); ok {
			if fulfilled, found, err := s.store.FulfilledQuantities(ctx, vendorOrder.ID); err != nil {
				return err
			} else if found {
				if err = partial.FinalizeReservationPartial(ctx, vendorOrder.BusinessID, vendorOrder.BranchID, vendorOrder.ReservationRef, fulfilled); err != nil {
					return err
				}
				return s.store.SetInventoryState(ctx, vendorOrder.ID, InventoryReleased, s.now().UTC())
			}
		}
		if err := s.inventory.ReleaseReservation(
			ctx, vendorOrder.BusinessID, vendorOrder.BranchID, vendorOrder.ReservationRef,
		); err != nil {
			return err
		}
		return s.store.SetInventoryState(
			ctx, vendorOrder.ID, InventoryReleased, s.now().UTC(),
		)
	}
	return nil
}
