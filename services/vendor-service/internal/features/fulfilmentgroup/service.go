package fulfilmentgroup

// The fulfilment workflow, and the authorization that guards it.
//
// Every vendor-facing entry point here answers two questions before it does
// anything: does this staff member hold the permission, and does their scope
// reach the branch that actually owns the row. The second question is the one
// that matters — a branch manager holding a permission for branch A must not be
// able to repack branch B's parcel — and it is answered against the row's own
// business and branch, never against anything the request supplied.

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

// Service is the fulfilment aggregate's workflow.
type Service struct {
	store      *Store
	production *production.Service
	enabled    bool
	now        func() time.Time
	handover   VendorHandoverAuthority
	fallback   HybridFallbackAuthority
	orders     VendorMissionOrderAuthority
}

// Options configures the service.
type Options struct {
	Store      *Store
	Production *production.Service
	Enabled    bool
	Now        func() time.Time
	Handover   VendorHandoverAuthority
	Fallback   HybridFallbackAuthority
	Orders     VendorMissionOrderAuthority
}

// NewService builds the fulfilment service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		store:      options.Store,
		production: options.Production,
		enabled:    options.Enabled,
		now:        now,
		handover:   options.Handover,
		fallback:   options.Fallback,
		orders:     options.Orders,
	}
}

// Enabled reports the platform capability switch.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

// SetHybridFallbackAuthority closes the composition cycle after the
// fulfilment projection has been attached to Orders. The injected adapter is
// still only Wallet + the accepted platform mission authority.
func (s *Service) SetHybridFallbackAuthority(authority HybridFallbackAuthority) {
	if s != nil {
		s.fallback = authority
	}
}

func (s *Service) SetVendorMissionOrderAuthority(authority VendorMissionOrderAuthority) {
	if s != nil {
		s.orders = authority
	}
}

// Ready reports whether the service can serve a request at all.
func (s *Service) Ready() bool {
	return s != nil && s.enabled && s.store.Ready() && s.production != nil
}

func (s *Service) guard() error {
	if s == nil || !s.enabled {
		return ErrDisabled
	}
	if !s.store.Ready() || s.production == nil {
		return ErrDependencyMissing
	}
	return nil
}

// authorize resolves the actor's reach for one permission.
func (s *Service) authorize(
	ctx context.Context, actor production.Actor, permission string,
) (production.Scope, error) {
	if s == nil || s.production == nil {
		return production.Scope{}, ErrForbidden
	}
	scope, err := s.production.Authorize(ctx, actor, permission)
	if err != nil {
		return production.Scope{}, ErrForbidden
	}
	if scope.Empty() {
		return production.Scope{}, ErrForbidden
	}
	return scope, nil
}

// authorizeBranch authorizes a permission and confirms the scope reaches the
// branch being acted on.
//
// This is the branch-escape guard. Without it, holding a fulfilment permission
// anywhere in a vendor would mean holding it everywhere in that vendor.
func (s *Service) authorizeBranch(
	ctx context.Context, actor production.Actor, permission, branchID string,
) error {
	scope, err := s.authorize(ctx, actor, permission)
	if err != nil {
		return err
	}
	if strings.TrimSpace(branchID) == "" || !scope.Covers(branchID) {
		return ErrForbidden
	}
	return nil
}

// -----------------------------------------------------------------------------
// Group and unit lifecycle
// -----------------------------------------------------------------------------

// OpenGroup opens (or returns) the fulfilment group for one commerce order.
//
// Internal: called by order orchestration once an order exists, not by a
// vendor staff request. A vendor does not decide that a customer's order has a
// fulfilment context; the order does.
func (s *Service) OpenGroup(
	ctx context.Context, parentOrderID, customerID string,
) (Group, error) {
	if err := s.guard(); err != nil {
		return Group{}, err
	}
	return s.store.EnsureGroup(ctx, parentOrderID, customerID, s.now().UTC())
}

// OpenUnit opens the pickup obligation for one vendor order.
//
// Also internal. The unit's vendor and branch come from the vendor order, so
// this cannot be used to attach an obligation to the wrong tenant.
func (s *Service) OpenUnit(
	ctx context.Context, group Group, in UnitInput,
) (Unit, error) {
	if err := s.guard(); err != nil {
		return Unit{}, err
	}
	return s.store.EnsureUnit(ctx, group, in, s.now().UTC())
}

// AdvanceUnitReadiness moves a pickup obligation forward.
func (s *Service) AdvanceUnitReadiness(
	ctx context.Context, actor production.Actor, unitID string,
	expectedVersion int64, to Readiness,
) (Unit, error) {
	if err := s.guard(); err != nil {
		return Unit{}, err
	}

	unit, err := s.loadUnitForActor(ctx, actor, unitID, PermParcelManage)
	if err != nil {
		return Unit{}, err
	}
	return s.store.SetUnitReadiness(
		ctx, unit.BusinessID, unitID, expectedVersion, to, s.now().UTC())
}

// loadUnitForActor reads a unit and confirms the actor may act on it.
//
// The read happens first and the branch check second, against the branch the
// row actually names. Doing it the other way round — trusting a branch id from
// the request and then reading — is how a branch-escape gets written.
func (s *Service) loadUnitForActor(
	ctx context.Context, actor production.Actor, unitID, permission string,
) (Unit, error) {
	unit, err := s.store.GetUnit(ctx, unitID)
	if err != nil {
		return Unit{}, err
	}
	if unit.BusinessID != actor.VendorID {
		// Deliberately NotFound: a foreign tenant's unit must be
		// indistinguishable from one that does not exist.
		return Unit{}, ErrNotFound
	}
	if err = s.authorizeBranch(ctx, actor, permission, unit.BranchID); err != nil {
		return Unit{}, err
	}
	return unit, nil
}

// -----------------------------------------------------------------------------
// Parcels
// -----------------------------------------------------------------------------

// PackParcel creates one parcel inside a pickup obligation.
func (s *Service) PackParcel(
	ctx context.Context, actor production.Actor, unitID string, in ParcelInput,
) (Parcel, error) {
	if err := s.guard(); err != nil {
		return Parcel{}, err
	}

	unit, err := s.loadUnitForActor(ctx, actor, unitID, PermParcelManage)
	if err != nil {
		return Parcel{}, err
	}
	return s.store.CreateParcel(ctx, unit.BusinessID, unitID, in, s.now().UTC())
}

// EnsureOrderParcel creates the stable physical parcel opened by paid-order
// activation. It is intentionally not an HTTP operation: tenant identity is
// resolved from the already-persisted fulfilment unit and the idempotency key
// makes activation/recovery return the same parcel.
func (s *Service) EnsureOrderParcel(
	ctx context.Context, unitID string, in ParcelInput,
) (Parcel, error) {
	if err := s.guard(); err != nil {
		return Parcel{}, err
	}
	unit, err := s.store.GetUnit(ctx, strings.TrimSpace(unitID))
	if err != nil {
		return Parcel{}, err
	}
	return s.store.CreateParcel(ctx, unit.BusinessID, unit.ID, in, s.now().UTC())
}

// AmendManifest updates a parcel's manifest before handover.
func (s *Service) AmendManifest(
	ctx context.Context, actor production.Actor, parcelID string,
	expectedVersion int64, manifest Manifest,
) (Parcel, error) {
	if err := s.guard(); err != nil {
		return Parcel{}, err
	}

	parcel, err := s.loadParcelForActor(ctx, actor, parcelID, PermParcelManage)
	if err != nil {
		return Parcel{}, err
	}
	return s.store.UpdateParcelManifest(
		ctx, parcel.BusinessID, parcelID, expectedVersion, manifest, s.now().UTC())
}

// GetParcel reads one parcel the actor is allowed to see.
func (s *Service) GetParcel(
	ctx context.Context, actor production.Actor, parcelID string,
) (Parcel, error) {
	if err := s.guard(); err != nil {
		return Parcel{}, err
	}
	return s.loadParcelForActor(ctx, actor, parcelID, PermRead)
}

// ListParcelCustody reads one parcel's custody chain.
func (s *Service) ListParcelCustody(
	ctx context.Context, actor production.Actor, parcelID string,
) ([]CustodyEvent, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	if _, err := s.loadParcelForActor(ctx, actor, parcelID, PermRead); err != nil {
		return nil, err
	}
	return s.store.ListCustodyEvents(ctx, parcelID)
}

// ListUnitParcels reads a pickup obligation's parcels.
func (s *Service) ListUnitParcels(
	ctx context.Context, actor production.Actor, unitID string,
) ([]Parcel, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	if _, err := s.loadUnitForActor(ctx, actor, unitID, PermRead); err != nil {
		return nil, err
	}
	return s.store.ListParcelsByUnit(ctx, unitID)
}

func (s *Service) loadParcelForActor(
	ctx context.Context, actor production.Actor, parcelID, permission string,
) (Parcel, error) {
	parcel, err := s.store.GetParcel(ctx, actor.VendorID, parcelID)
	if err != nil {
		return Parcel{}, err
	}
	if err = s.authorizeBranch(ctx, actor, permission, parcel.BranchID); err != nil {
		return Parcel{}, err
	}
	return parcel, nil
}

// -----------------------------------------------------------------------------
// Planning
// -----------------------------------------------------------------------------

// ActivatePlan appends and activates the next plan version for a group.
//
// Internal: planning is a platform decision driven by delivery policy and
// logistics evidence, not something a vendor triggers by hand. Exposing it to a
// vendor request would let one tenant re-route another's delivery.
func (s *Service) ActivatePlan(
	ctx context.Context, groupID string, in PlanInput,
) (Plan, error) {
	if err := s.guard(); err != nil {
		return Plan{}, err
	}
	return s.store.ActivatePlan(ctx, groupID, in, s.now().UTC())
}

// PlanHistory reads every plan version for a group, newest first.
func (s *Service) PlanHistory(
	ctx context.Context, actor production.Actor, groupID string,
) ([]Plan, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	if _, err := s.readableGroup(ctx, actor, groupID); err != nil {
		return nil, err
	}
	return s.store.ListPlans(ctx, groupID)
}

// GroupView is the read model a vendor sees for one order's fulfilment.
//
// Platform missions appear here as PROJECTIONS: the vendor can see which
// authority is carrying the goods and how far along it is, and can change none
// of it.
type GroupView struct {
	Group Group `json:"group"`

	CurrentPlan *Plan `json:"current_plan,omitempty"`

	PlatformMissions []PlatformMissionRef `json:"platform_missions"`
	VendorMissions   []VendorMission      `json:"vendor_missions"`
	Returns          []ParcelReturn       `json:"returns"`
}

// ViewGroup reads one order's fulfilment as the acting vendor may see it.
//
// A commerce order can contain several vendors. Units and parcels belonging to
// other vendors in the same order are filtered out here, so one vendor in a
// shared order never learns what another vendor packed.
func (s *Service) ViewGroup(
	ctx context.Context, actor production.Actor, groupID string,
) (GroupView, error) {
	if err := s.guard(); err != nil {
		return GroupView{}, err
	}

	group, err := s.readableGroup(ctx, actor, groupID)
	if err != nil {
		return GroupView{}, err
	}

	scope, err := s.authorize(ctx, actor, PermRead)
	if err != nil {
		return GroupView{}, err
	}

	view := GroupView{Group: group}

	if group.Planned() {
		plan, planErr := s.store.GetPlan(ctx, group.ID, group.CurrentPlanID)
		if planErr != nil && planErr != ErrNotFound {
			return GroupView{}, planErr
		}
		if planErr == nil {
			view.CurrentPlan = &plan
		}
	}

	if view.PlatformMissions, err = s.store.ListMissionRefs(ctx, group.ID); err != nil {
		return GroupView{}, err
	}

	missions, err := s.store.ListVendorMissions(ctx, actor.VendorID, scopeBranches(scope), 200)
	if err != nil {
		return GroupView{}, err
	}
	for _, mission := range missions {
		if mission.GroupID == group.ID {
			view.VendorMissions = append(view.VendorMissions, mission)
		}
	}

	if view.Returns, err = s.store.ListReturns(ctx, group.ID); err != nil {
		return GroupView{}, err
	}

	return view, nil
}

// readableGroup loads a group the actor has at least one unit in.
//
// Membership is the access rule: a vendor may read the fulfilment of an order
// it is actually part of, and the read is then narrowed to its own rows. A
// vendor with nothing in the order gets NotFound.
func (s *Service) readableGroup(
	ctx context.Context, actor production.Actor, groupID string,
) (Group, error) {
	scope, err := s.authorize(ctx, actor, PermRead)
	if err != nil {
		return Group{}, err
	}

	group, err := s.store.GetGroup(ctx, groupID)
	if err != nil {
		return Group{}, err
	}

	var units []Unit
	for _, unit := range group.Units {
		if unit.BusinessID != actor.VendorID || !scope.Covers(unit.BranchID) {
			continue
		}
		units = append(units, unit)
	}
	if len(units) == 0 {
		return Group{}, ErrNotFound
	}

	visible := map[string]bool{}
	for _, unit := range units {
		visible[unit.ID] = true
	}
	var parcels []Parcel
	for _, parcel := range group.Parcels {
		if visible[parcel.UnitID] {
			parcels = append(parcels, parcel)
		}
	}

	group.Units = units
	group.Parcels = parcels
	return group, nil
}

func scopeBranches(scope production.Scope) []string {
	if scope.Global {
		return nil
	}
	return scope.BranchIDs
}

// -----------------------------------------------------------------------------
// Platform missions (internal)
// -----------------------------------------------------------------------------

// ReserveMission durably reserves the mission reference for a plan slot before
// any authority is called.
//
// Internal only, and deliberately so: a vendor request must never be able to
// pick which canonical mission is bound to a slot.
func (s *Service) ReserveMission(
	ctx context.Context, groupID, planID, planMissionID string, authority Authority,
) (PlatformMissionRef, bool, error) {
	if err := s.guard(); err != nil {
		return PlatformMissionRef{}, false, err
	}
	return s.store.EnsurePlatformMissionRef(
		ctx, groupID, planID, planMissionID, authority, s.now().UTC())
}

// LinkMission binds an authority's answer to a reserved mission reference.
func (s *Service) LinkMission(
	ctx context.Context, missionRef string, link MissionLink,
) (PlatformMissionRef, error) {
	if err := s.guard(); err != nil {
		return PlatformMissionRef{}, err
	}
	return s.store.LinkPlatformMission(ctx, missionRef, link, s.now().UTC())
}

// ReplaceMission records that a canonical mission was replaced by another.
func (s *Service) ReplaceMission(
	ctx context.Context, missionRef, reason string,
) (PlatformMissionRef, error) {
	if err := s.guard(); err != nil {
		return PlatformMissionRef{}, err
	}
	return s.store.ReplacePlatformMission(ctx, missionRef, reason, s.now().UTC())
}

// ProjectMissionStatus applies an authority's own status report.
func (s *Service) ProjectMissionStatus(
	ctx context.Context, missionRef, status, sourceRef string, sourceVersion int64,
) (PlatformMissionRef, bool, error) {
	if err := s.guard(); err != nil {
		return PlatformMissionRef{}, false, err
	}
	return s.store.ProjectMissionStatus(
		ctx, missionRef, status, sourceRef, sourceVersion, s.now().UTC())
}

// LiveMissionRefForSlot resolves the projection currently executing one plan
// slot of one order.
func (s *Service) LiveMissionRefForSlot(
	ctx context.Context, parentOrderID, slotRef string,
) (PlatformMissionRef, bool, error) {
	if err := s.guard(); err != nil {
		return PlatformMissionRef{}, false, err
	}
	return s.store.LiveMissionRefForSlot(ctx, parentOrderID, slotRef)
}

// GetPlan reads one immutable plan version for an internal execution adapter.
func (s *Service) GetPlan(ctx context.Context, groupID, planID string) (Plan, error) {
	if err := s.guard(); err != nil {
		return Plan{}, err
	}
	return s.store.GetPlan(ctx, strings.TrimSpace(groupID), strings.TrimSpace(planID))
}

// ParcelsForMissionSlot reads the parcels riding one plan mission slot.
func (s *Service) ParcelsForMissionSlot(
	ctx context.Context, planMissionID string,
) ([]Parcel, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return s.store.ParcelsForMissionSlot(ctx, planMissionID)
}

// RecordLogisticsCustody records a custody fact reported by a logistics
// authority.
//
// The source is the authority itself and the event carries its version, so a
// replayed or out-of-order report is absorbed without moving the parcel
// backwards. Vendor Service never invents one of these.
func (s *Service) RecordLogisticsCustody(
	ctx context.Context, event CustodyEvent,
) (bool, error) {
	if err := s.guard(); err != nil {
		return false, err
	}
	if event.Source == SourceVendor {
		// A logistics fact attributed to the vendor would misreport who
		// witnessed it.
		return false, ErrInvalid
	}
	return s.store.RecordCustody(ctx, event, s.now().UTC())
}

// -----------------------------------------------------------------------------
// Vendor missions
// -----------------------------------------------------------------------------

// OpenVendorMission creates the canonical mission for a vendor-owned slot.
func (s *Service) OpenVendorMission(
	ctx context.Context, groupID, planID, planMissionID string,
) (VendorMission, error) {
	if err := s.guard(); err != nil {
		return VendorMission{}, err
	}
	return s.store.EnsureVendorMission(ctx, groupID, planID, planMissionID, s.now().UTC())
}

// ListVendorMissions reads the acting vendor's own delivery missions.
func (s *Service) ListVendorMissions(
	ctx context.Context, actor production.Actor, limit int,
) ([]VendorMission, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	scope, err := s.authorize(ctx, actor, PermRead)
	if err != nil {
		return nil, err
	}
	return s.store.ListVendorMissions(ctx, actor.VendorID, scopeBranches(scope), limit)
}

// GetVendorMission reads one of the acting vendor's delivery missions.
func (s *Service) GetVendorMission(
	ctx context.Context, actor production.Actor, missionID string,
) (VendorMission, error) {
	if err := s.guard(); err != nil {
		return VendorMission{}, err
	}
	mission, err := s.store.GetVendorMission(ctx, actor.VendorID, missionID)
	if err != nil {
		return VendorMission{}, err
	}
	if err = s.authorizeBranch(ctx, actor, PermRead, mission.BranchID); err != nil {
		return VendorMission{}, err
	}
	return mission, nil
}

// AssignRider puts one of the vendor's own staff on a vendor-owned mission.
func (s *Service) AssignRider(
	ctx context.Context, actor production.Actor, missionID, staffID string,
	expectedVersion int64,
) (VendorMission, error) {
	if err := s.guard(); err != nil {
		return VendorMission{}, err
	}
	mission, err := s.store.GetVendorMission(ctx, actor.VendorID, missionID)
	if err != nil {
		return VendorMission{}, err
	}
	if err = s.authorizeBranch(ctx, actor, PermMissionManage, mission.BranchID); err != nil {
		return VendorMission{}, err
	}
	if strings.TrimSpace(actor.CommandRef) == "" {
		return VendorMission{}, ErrInvalid
	}
	return s.store.AssignEligibleVendorRider(
		ctx, actor.VendorID, missionID, staffID, actor.StaffID,
		actor.CommandRef, expectedVersion, s.now().UTC())
}

// AdvanceVendorMission executes one step of a vendor-owned delivery.
func (s *Service) AdvanceVendorMission(
	ctx context.Context, actor production.Actor, missionID string,
	transition VendorMissionTransition,
) (VendorMission, error) {
	if err := s.guard(); err != nil {
		return VendorMission{}, err
	}
	if transition.To == VendorMissionCustodyTransferred || transition.To == VendorMissionDelivered || transition.To == VendorMissionReturned {
		// Secure custody transitions belong to the assigned Rider action path,
		// which calls the approved handover authority. An operator-supplied
		// opaque string is not verification.
		return VendorMission{}, ErrHandoverUnavailable
	}
	mission, err := s.store.GetVendorMission(ctx, actor.VendorID, missionID)
	if err != nil {
		return VendorMission{}, err
	}
	if err = s.authorizeBranch(ctx, actor, PermMissionManage, mission.BranchID); err != nil {
		return VendorMission{}, err
	}
	return s.store.TransitionVendorMissionStatus(
		ctx, actor.VendorID, missionID, transition, s.now().UTC())
}

// -----------------------------------------------------------------------------
// Returns
// -----------------------------------------------------------------------------

// InitiateReturn opens a return journey for a parcel.
//
// The parcel keeps its identity. That is the whole contract of this call: it
// records that the SAME box is coming back, and creates nothing that could be
// mistaken for a replacement parcel.
func (s *Service) InitiateReturn(
	ctx context.Context, actor production.Actor, parcelID, reasonCode, idempotencyKey string,
) (ParcelReturn, error) {
	if err := s.guard(); err != nil {
		return ParcelReturn{}, err
	}
	if _, err := s.loadParcelForActor(ctx, actor, parcelID, PermMissionManage); err != nil {
		return ParcelReturn{}, err
	}
	return s.store.InitiateReturn(ctx, parcelID, reasonCode, idempotencyKey, s.now().UTC())
}

// LinkReturnMission attaches the mission carrying a parcel back.
//
// Internal: which mission carries a return is a logistics decision, and a new
// mission reference here is expected and correct. The parcel is not new.
func (s *Service) LinkReturnMission(
	ctx context.Context, returnID, platformMissionRefID, vendorMissionID string,
) (ParcelReturn, error) {
	if err := s.guard(); err != nil {
		return ParcelReturn{}, err
	}
	return s.store.LinkReturnMission(
		ctx, returnID, platformMissionRefID, vendorMissionID, s.now().UTC())
}

// -----------------------------------------------------------------------------
// Replanning
// -----------------------------------------------------------------------------

// ReplanRequest asks the aggregate to regroup a group's units and append the
// next plan version.
//
// This is the entry point every operational replan uses: a provider that never
// arrived, a capacity or vehicle overflow, a vendor rider that could not go
// out. It never edits the version in force — it computes the next one from the
// units as they stand and the authorities as they actually are.
type ReplanRequest struct {
	GroupID string

	// ExpectedVersion is the version the caller planned against. A caller that
	// planned against a superseded version is told its plan is stale rather
	// than silently overwriting the newer one.
	ExpectedVersion int64

	Reason       PlanReason
	ReasonDetail string

	// Candidates are the units to be planned, each with the executor its
	// delivery policy already selected. Regrouping never changes an executor;
	// it only decides which units can share a vehicle.
	Candidates []Candidate

	// Capabilities states what each authority can actually execute. Supplied
	// by the composition root from the authorities' own contracts, so this
	// package never assumes a capability a logistics service does not have.
	Capabilities map[Authority]Capability

	Destination Stop

	DeliveryMode  DeliveryMode
	PolicyVersion int64

	Currency         string
	PlannedCostMinor int64

	IdempotencyKey string
}

// Replan appends and activates the next plan version for a group.
//
// Group, unit and parcel identities are untouched: a replan changes which
// vehicle collects the goods, not what was purchased or which box it is in.
// The previous version stays readable exactly as it was written.
//
// Concurrency is the caller's ExpectedVersion plus the store's row lock and
// unique(group_id, version): two replans against v3 produce one v4 and one
// ErrStalePlan, never two v4 rows.
func (s *Service) Replan(ctx context.Context, request ReplanRequest) (Plan, error) {
	if err := s.guard(); err != nil {
		return Plan{}, err
	}
	if !request.Reason.Replan() {
		// A replan that claims to be the initial plan would make the history
		// unable to explain why it changed.
		return Plan{}, ErrInvalid
	}
	if len(request.Candidates) == 0 {
		return Plan{}, ErrInvalid
	}

	missions := BuildPlanMissions(
		request.Candidates, request.Capabilities, request.Destination, request.Currency)
	if len(missions) == 0 {
		return Plan{}, ErrConflict
	}

	return s.store.ActivatePlan(ctx, request.GroupID, PlanInput{
		ExpectedVersion:  request.ExpectedVersion,
		Reason:           request.Reason,
		ReasonDetail:     request.ReasonDetail,
		PolicyVersion:    request.PolicyVersion,
		DeliveryMode:     request.DeliveryMode,
		PlannedCostMinor: request.PlannedCostMinor,
		Currency:         request.Currency,
		IdempotencyKey:   request.IdempotencyKey,
		Missions:         missions,
	}, s.now().UTC())
}
