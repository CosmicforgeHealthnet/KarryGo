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
	"encoding/json"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
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
type LogisticsService string

const (
	LogisticsDispatch LogisticsService = "DISPATCH"
	LogisticsTaxi     LogisticsService = "TAXI"
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

// ItemInventoryAuthority releases one unavailable line's hold without
// finalising or disturbing sibling items in the same reservation.
type ItemInventoryAuthority interface {
	ReleaseReservationItem(ctx context.Context, businessID, branchID, reservationRef, inventoryItemRef, idempotencyKey string) error
}

// ReplacementInventoryAuthority is the optional atomic substitution
// capability. Inventory, not Orders, owns all lot and reservation arithmetic.
type ReplacementInventoryAuthority interface {
	ResolveShopVariantItem(ctx context.Context, businessID, branchID, variantRef string) (string, error)
	ReplaceReservationAllocation(ctx context.Context, businessID, branchID string, request InventoryReplacementRequest) (InventoryReplacementResult, error)
}

type InventoryReplacementRequest struct {
	ReservationRef     string
	SourceType         string
	SourceRef          string
	OriginalItemRef    string
	OriginalOrderedQty int64
	OriginalFoundQty   int64
	MissingQty         int64
	ReplacementItemRef string
	ReplacementQty     int64
	ExpectedVersion    int64
	IdempotencyKey     string
	CorrelationID      string
}

type InventoryReplacementResult struct {
	OriginalFulfilled   int64
	OriginalReleased    int64
	ReplacementReserved int64
	Version             int64
}

// MissionRequest asks a logistics authority to execute one mission.
type MissionRequest struct {
	MissionRef       string
	ParentOrderID    string
	VendorOrderIDs   []string
	CustomerID       string
	Service          LogisticsService
	VehicleClass     string
	Currency         string
	AgreedFeeMinor   int64
	FundingReference string
	PlanMissionRef   string
	PickupBusiness   string
	PickupBranch     string
	DeliveryAddress  AddressSnapshot
	Receiver         ReceiverSnapshot
	WeightGrams      int64
	PackageCount     int64
	Execution        *MissionExecution

	// SecurePickupRequired and SecureDeliveryRequired state the protected
	// handovers commerce needs (VND-015, VND-016). Commerce states the
	// requirement; the executing authority owns the credential and its
	// verification. A requirement an authority cannot meet is refused before
	// the mission is created, never quietly downgraded to an unverified
	// completion.
	SecurePickupRequired   bool
	SecureDeliveryRequired bool

	// CorrelationID threads this mission back to its parent order across
	// Vendor, the execution service and Fleet (VND-025).
	CorrelationID string

	// AuthoritySelectedBy records whether the authority came from the frozen
	// checkout plan or an explicit replan, so a later change of authority is
	// always a deliberate operation rather than a runtime heuristic (VND-004).
	AuthoritySelectedBy string

	// SourceVersion is the plan version this mission was cut from. With the
	// mission ref and the authority it forms the deterministic execution
	// identity a retry reuses and a replan does not (VND-012).
	SourceVersion int64
}

// Intent projects the request onto the canonical conceptual mission model, so
// the capability gate and the execution identity are derived from one place
// rather than recomputed per adapter.
func (r MissionRequest) Intent() logistics.MissionIntent {
	authority, err := logistics.Parse(string(r.Service))
	if err != nil {
		authority = logistics.Authority(r.Service)
	}
	selectedBy := logistics.SelectionSource(r.AuthoritySelectedBy)
	if !selectedBy.Valid() {
		// An unrecorded selection is treated as the checkout decision, which
		// is what every mission created before this field existed was.
		selectedBy = logistics.SelectedAtCheckout
	}
	intent := logistics.MissionIntent{
		CommerceOrderRef: r.ParentOrderID, MissionRef: r.MissionRef,
		Authority: authority, SelectedBy: selectedBy, SourceVersion: r.SourceVersion,
		Delivery: logistics.Location{
			Ref: r.DeliveryAddress.SourceAddressRef, Address: r.DeliveryAddress.Address,
			Latitude: r.DeliveryAddress.Latitude, Longitude: r.DeliveryAddress.Longitude,
		},
		Cargo: logistics.CargoMetadata{
			WeightGrams: r.WeightGrams, PackageCount: r.PackageCount,
			VehicleClass: r.VehicleClass,
		},
		SecurePickupRequired:   r.SecurePickupRequired,
		SecureDeliveryRequired: r.SecureDeliveryRequired,
		CorrelationID:          r.correlation(),
	}
	if r.Execution != nil && len(r.Execution.Stops) > 0 {
		for _, stop := range r.Execution.Stops {
			intent.Pickups = append(intent.Pickups, logistics.Location{
				Ref: stop.StopRef, Address: stop.Address,
				Latitude: stop.Latitude, Longitude: stop.Longitude,
			})
		}
		return intent
	}
	// Without a committed multi-stop plan the mission collects from the one
	// branch that owns the allocation.
	intent.Pickups = []logistics.Location{{Ref: r.PickupBranch}}
	return intent
}

// correlation falls back to the parent order, which is the identity every
// mission for this order already shares.
func (r MissionRequest) correlation() string {
	if trimmed := strings.TrimSpace(r.CorrelationID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(r.ParentOrderID)
}

// IdempotencyKey is the deterministic execution identity for this request
// (VND-012).
func (r MissionRequest) IdempotencyKey() string { return r.Intent().IdempotencyKey() }

// MissionExecution is the immutable operational plan handed to a logistics
// authority. It is populated by the fulfilment aggregate after the active plan
// and its stable parcels have been committed; callers cannot supply it.
type MissionExecution struct {
	PlanRef                 string
	PlanVersion             int64
	PlanFingerprint         string
	AuthorityQuoteID        string
	AuthorityQuoteReference string
	Mode                    string
	ServiceClass            string
	Stops                   []MissionStop
	Parcels                 []MissionParcel
	PromisedBy              time.Time
	LoadingDurationSeconds  int64
}

type MissionStop struct {
	StopRef, PhysicalStopRef, UnitRef, BusinessID, BranchID, Address string
	Latitude, Longitude                                              float64
	ReadyFrom, ReadyTo                                               time.Time
}

type MissionParcel struct {
	ParcelRef, UnitRef, VendorOrderID, BusinessID, BranchID string
	Manifest                                                json.RawMessage
	ManifestVersion                                         int64
	WeightGrams, LengthMM, WidthMM, HeightMM, PackageCount  int64
	Fragile, Upright, StackRestricted                       bool
	TemperatureControl                                      string
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

type MissionQuote struct {
	FeeMinor    int64
	Currency    string
	Serviceable bool
	ReasonCode  string
}

type MissionQuoteAuthority interface {
	QuoteMission(context.Context, MissionRequest) (MissionQuote, error)
}

type MissionCancellationAuthority interface {
	CancelMission(ctx context.Context, operationRef, reasonCode string) error
}

// FulfilmentProjection is the narrow port onto the operational fulfilment
// aggregate: the fulfilment group rooted on this order, the pickup obligation
// per allocation, the versioned plan, and the projections of the logistics
// missions that execute it.
//
// It is a port rather than a direct dependency for the usual reason — the
// orders feature must not learn another feature's persistence — and for one
// specific one: the aggregate is a PROJECTION owner. Orders decides what is
// being fulfilled; it does not decide what a logistics authority believes, and
// routing that belief through an interface keeps the two from blurring.
//
// Every method must be idempotent. Activation and recovery both run more than
// once, and a projection that could not absorb a repeat would turn a normal
// retry into a duplicate.
type FulfilmentProjection interface {
	// OpenFulfilment makes the group, its units and the plan version durable
	// BEFORE any logistics authority is called, so a crash mid-activation
	// leaves a record a recovery pass can resolve rather than a request
	// nothing wrote down.
	OpenFulfilment(ctx context.Context, order ParentOrder, plans []MissionPlan) error

	// LinkMission binds an authority's answer to the plan mission slot it was
	// requested for.
	LinkMission(ctx context.Context, order ParentOrder, plan MissionPlan, result MissionResult) error

	// ProjectLogisticsEvent applies an authority's own status and custody
	// evidence. Stale and duplicate events are absorbed, never applied.
	ProjectLogisticsEvent(ctx context.Context, event LogisticsOrderEvent) error
}

// MissionExecutionProjection is an optional extension implemented by the
// operational fulfilment aggregate. Older test doubles and deployments remain
// source compatible; Dispatch V1 fails closed when the immutable evidence is
// unavailable.
type MissionExecutionProjection interface {
	MissionExecution(ctx context.Context, order ParentOrder, plan MissionPlan) (MissionExecution, error)
}

// MissionPlanResolver restores the logistics authority and vehicle class from
// the customer's frozen Section 17C mission. This prevents later order code
// from guessing an authority from domain or fee size.
type MissionPlanResolver interface {
	ResolveMissionPlan(ctx context.Context, order ParentOrder, plan MissionPlan) (MissionPlan, error)
}

// SetFulfilmentProjection wires the operational fulfilment aggregate.
//
// Separate from SetFulfilmentAuthorities, and optional: a deployment without
// it activates orders exactly as before rather than failing. The aggregate adds
// traceability, and traceability that could take an order down would be a poor
// trade.
func (s *Service) SetFulfilmentProjection(projection FulfilmentProjection) {
	s.fulfilmentProjection = projection
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
	if s.restrictedGoods == nil {
		return outcome, ErrDependencyMissing
	}
	if err = s.restrictedGoods.ValidateOrderForFulfilment(ctx, order); err != nil {
		return outcome, err
	}
	for _, vendorOrder := range order.VendorOrders {
		if !vendorOrderFulfillable(vendorOrder.Status) {
			continue
		}
		providerType, providerID := "vendor", vendorOrder.BusinessID
		if vendorOrder.DeliveryResponsibility == DeliveryPlatform {
			providerType = string(serviceFor(vendorOrder))
			providerID = vendorOrder.LogisticsMissionRef
		}
		if err = s.ensureDeliveryFunding(ctx, order, vendorOrder, providerType, providerID); err != nil {
			return outcome, err
		}
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
	if resolver, ok := s.fulfilmentProjection.(MissionPlanResolver); ok {
		for index := range plans {
			plans[index], err = resolver.ResolveMissionPlan(ctx, order, plans[index])
			if err != nil {
				return outcome, err
			}
		}
	}
	outcome.VendorOwnDelivery = countVendorOwn(order)

	// The fulfilment aggregate is made durable BEFORE the first authority
	// call. That ordering is the whole reason a lost mission response is
	// recoverable: the slot already exists to be read back against, instead of
	// a request that nothing recorded and a second vehicle nobody ordered.
	if s.fulfilmentProjection != nil {
		if err = s.fulfilmentProjection.OpenFulfilment(ctx, order, plans); err != nil {
			return outcome, err
		}
	}

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
		var execution *MissionExecution
		if projection, ok := s.fulfilmentProjection.(MissionExecutionProjection); ok {
			snapshot, snapshotErr := projection.MissionExecution(ctx, order, plan)
			if snapshotErr != nil {
				return outcome, snapshotErr
			}
			execution = &snapshot
		}

		result, missionErr := s.missions.CreateMission(ctx, MissionRequest{
			MissionRef:       mission.MissionRef,
			ParentOrderID:    order.ID,
			VendorOrderIDs:   plan.VendorOrderIDs,
			CustomerID:       order.CustomerID,
			Service:          plan.Service,
			VehicleClass:     plan.VehicleClass,
			Currency:         order.Currency,
			AgreedFeeMinor:   plan.AgreedFeeMinor,
			FundingReference: order.QuoteID,
			PlanMissionRef:   plan.PlanMissionRef,
			PickupBusiness:   plan.PickupBusiness,
			PickupBranch:     plan.PickupBranch,
			DeliveryAddress:  order.Address,
			Receiver:         order.Receiver,
			WeightGrams:      plan.WeightGrams,
			PackageCount:     plan.PackageCount,
			Execution:        execution,
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
		if s.fulfilmentProjection != nil {
			if err = s.fulfilmentProjection.LinkMission(ctx, order, plan, result); err != nil {
				return outcome, err
			}
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
	// MissionRef optionally supplies an already-reserved cross-service
	// operation reference. Normal activation leaves it empty and derives the
	// established value; Hybrid fallback passes the PlatformMissionRef value
	// so Vendor, Dispatch and recovery all name the same operation.
	MissionRef             string
	PlanMissionRef         string
	Service                LogisticsService
	VehicleClass           string
	AgreedFeeMinor         int64
	VendorOrderIDs         []string
	PickupBusiness         string
	PickupBranch           string
	WeightGrams            int64
	PackageCount           int64
	LoadingDurationSeconds int64
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
// This is a legacy fallback for projections that cannot restore the frozen
// Section 17C authority. Production resolves the actual service and vehicle
// through MissionPlanResolver before any mission is persisted or called.
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
