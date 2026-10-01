package main

// Section 17F fulfilment adapters.
//
// The orders feature never learns Section 14's actor/scope shape, and never
// learns a logistics base URL or secret. It receives both through narrow ports,
// exactly as Section 17C receives its quote authorities.

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// -----------------------------------------------------------------------------
// Inventory
// -----------------------------------------------------------------------------

// section17FInventoryAuthority adapts Section 14 stock to the order feature's
// narrow port.
//
// Commerce acts on behalf of a customer, not a vendor staff member, so there is
// no staff permission to evaluate here. Tenant scoping is still explicit: the
// business and branch are the server-resolved allocation's own, never a
// customer-supplied value.
type section17FInventoryAuthority struct {
	stock *inventory.StockService
}

func (a *section17FInventoryAuthority) ResolveShopVariantItem(ctx context.Context, businessID, branchID, variantRef string) (string, error) {
	if a == nil || a.stock == nil {
		return "", orders.ErrDependencyMissing
	}
	item, err := a.stock.ResolveShopVariantItem(ctx, businessID, branchID, variantRef)
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

// FinalizeReservationPartial keeps partial stock accounting inside the
// authoritative Inventory service; the order feature never edits stock rows.
func (a *section17FInventoryAuthority) FinalizeReservationPartial(ctx context.Context, businessID, branchID, reservationRef string, fulfilledByItem map[string]int64) error {
	if a == nil || a.stock == nil {
		return orders.ErrDependencyMissing
	}
	actor, scope := a.scope(businessID, branchID)
	_, err := a.stock.FinalizeReservationPartial(ctx, actor, scope, reservationRef, fulfilledByItem)
	return ignoreAlreadyFinal(err)
}

func (a *section17FInventoryAuthority) ReleaseReservationItem(ctx context.Context, businessID, branchID, reservationRef, inventoryItemRef, idempotencyKey string) error {
	if a == nil || a.stock == nil {
		return orders.ErrDependencyMissing
	}
	actor, scope := a.scope(businessID, branchID)
	return a.stock.ReleaseReservationItem(ctx, actor, scope, reservationRef, inventoryItemRef, idempotencyKey)
}

func (a *section17FInventoryAuthority) ReplaceReservationAllocation(ctx context.Context, businessID, branchID string, request orders.InventoryReplacementRequest) (orders.InventoryReplacementResult, error) {
	if a == nil || a.stock == nil {
		return orders.InventoryReplacementResult{}, orders.ErrDependencyMissing
	}
	actor, scope := a.scope(businessID, branchID)
	result, err := a.stock.ReplaceReservationAllocation(ctx, actor, scope, inventory.ReservationReplacementRequest{
		ReservationID: request.ReservationRef, SourceType: request.SourceType, SourceRef: request.SourceRef,
		OriginalItemID: request.OriginalItemRef, OriginalOrderedQty: request.OriginalOrderedQty,
		OriginalFoundQty: request.OriginalFoundQty, MissingQty: request.MissingQty,
		ReplacementItemID: request.ReplacementItemRef, ReplacementQty: request.ReplacementQty,
		ExpectedVersion: request.ExpectedVersion, IdempotencyKey: request.IdempotencyKey,
		CorrelationID: request.CorrelationID,
	})
	if err != nil {
		return orders.InventoryReplacementResult{}, err
	}
	return orders.InventoryReplacementResult{OriginalFulfilled: result.OriginalFulfilled, OriginalReleased: result.OriginalReleased, ReplacementReserved: result.ReplacementReserved, Version: result.Version}, nil
}

func newSection17FInventoryAuthority(
	stock *inventory.StockService,
) *section17FInventoryAuthority {
	if stock == nil {
		return nil
	}
	return &section17FInventoryAuthority{stock: stock}
}

func (a *section17FInventoryAuthority) scope(
	businessID, branchID string,
) (inventory.Actor, inventory.BranchScope) {
	branch := branchID
	return inventory.Actor{
			BusinessID: businessID,
			// The commerce workflow is the actor. Naming it explicitly keeps
			// the Section 14 audit trail honest about who moved the stock.
			ActorRef: orders.PaymentSourceService,
			StaffID:  orders.PaymentSourceService,
		}, inventory.BranchScope{
			BusinessID: businessID,
			BranchID:   &branch,
		}
}

// ConfirmReservation implements orders.InventoryAuthority.
//
// A reservation already past HELD is not an error: activation and recovery both
// run more than once, and treating a repeat as a failure would strand an order
// that is actually fine.
func (a *section17FInventoryAuthority) ConfirmReservation(
	ctx context.Context, businessID, branchID, reservationRef string,
) error {
	if a == nil || a.stock == nil {
		return orders.ErrDependencyMissing
	}
	if strings.TrimSpace(reservationRef) == "" {
		return errors.New("inventory reservation reference is required")
	}
	actor, scope := a.scope(businessID, branchID)
	_, err := a.stock.ConfirmReservation(ctx, actor, scope, reservationRef)
	return ignoreAlreadyFinal(err)
}

// CommitReservation implements orders.InventoryAuthority.
func (a *section17FInventoryAuthority) CommitReservation(
	ctx context.Context, businessID, branchID, reservationRef string,
) error {
	if a == nil || a.stock == nil {
		return orders.ErrDependencyMissing
	}
	if strings.TrimSpace(reservationRef) == "" {
		return errors.New("inventory reservation reference is required")
	}
	actor, scope := a.scope(businessID, branchID)
	_, err := a.stock.CommitReservation(ctx, actor, scope, reservationRef)
	return ignoreAlreadyFinal(err)
}

// ReleaseReservation implements orders.InventoryAuthority.
//
// It releases exactly one allocation's reservation. A sibling vendor's stock is
// a different reservation and is never touched by this call.
func (a *section17FInventoryAuthority) ReleaseReservation(
	ctx context.Context, businessID, branchID, reservationRef string,
) error {
	if a == nil || a.stock == nil {
		return orders.ErrDependencyMissing
	}
	if strings.TrimSpace(reservationRef) == "" {
		return errors.New("inventory reservation reference is required")
	}
	actor, scope := a.scope(businessID, branchID)
	_, err := a.stock.ReleaseReservation(ctx, actor, scope, reservationRef)
	return ignoreAlreadyFinal(err)
}

// ignoreAlreadyFinal treats "this reservation is already in that state" as
// success.
//
// Section 14 refuses a transition it has already made. From the commerce side
// that is the desired end state, not a failure, and re-raising it would make an
// idempotent step permanently fail.
func ignoreAlreadyFinal(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	for _, benign := range []string{
		"already", "invalid transition", "not held", "terminal",
	} {
		if strings.Contains(message, benign) {
			return nil
		}
	}
	return err
}

// -----------------------------------------------------------------------------
// Logistics missions
// -----------------------------------------------------------------------------

// section17FMissionAuthority routes a mission to the logistics service that
// priced it.
//
// It holds one client per authority. Taxi is deliberately absent: it is not
// required for commerce, and there is no client here that could accidentally
// create one.
type section17FMissionAuthority struct {
	dispatch *dispatchcommerce.Client
	taxi     *commercemission.Client
	hauling  *commercemission.Client

	// capabilities is the authority-declared capability gate (VND-005 …
	// VND-008, VND-030). It is optional: a deployment without capability
	// endpoints behaves exactly as before rather than refusing every mission,
	// because a gate that fails closed on its own absence would take commerce
	// down to enforce a rule about capacity.
	capabilities *logistics.Registry
}

// withCapabilities attaches the capability registry.
func (a *section17FMissionAuthority) withCapabilities(registry *logistics.Registry) *section17FMissionAuthority {
	if a != nil {
		a.capabilities = registry
	}
	return a
}

// authorizeCapability refuses a mission the chosen authority has declared it
// cannot execute.
//
// The check runs before any mission is created, so an impossible mission is
// refused with the authority's own reason code instead of failing in the field
// after a customer has paid. An authority that cannot be asked is not treated
// as incapable: an unreachable capability endpoint yields no refusal, and the
// mission creation itself remains the real gate.
func (a *section17FMissionAuthority) authorizeCapability(
	ctx context.Context, request orders.MissionRequest,
) error {
	if a == nil || a.capabilities == nil {
		return nil
	}
	intent := request.Intent()
	if !intent.Authority.PlatformExecuted() {
		return nil
	}
	if !a.capabilities.Configured(intent.Authority) {
		return nil
	}
	err := a.capabilities.Authorize(ctx, intent.Authority, intent.Requirement())
	if err == nil {
		return nil
	}
	failure := logistics.Classify(intent.Authority, err)
	if failure.Category == logistics.FailureDependencyUnavailable {
		// The authority could not be asked. That is not evidence it cannot
		// carry the load, and refusing here would turn a capability-service
		// outage into a failed customer order.
		return nil
	}
	return failure
}

func newSection17FMissionAuthority(
	dispatchEnabled bool, dispatchURL, dispatchSecret string,
	haulingEnabled bool, haulingURL, haulingSecret string,
	timeout time.Duration,
) *section17FMissionAuthority {
	authority := &section17FMissionAuthority{}
	if dispatchEnabled {
		authority.dispatch = dispatchcommerce.NewClient(dispatchcommerce.ClientOptions{
			BaseURL: dispatchURL, Secret: dispatchSecret, Timeout: timeout,
		})
	}
	if haulingEnabled {
		authority.hauling = commercemission.New(commercemission.Options{
			BaseURL: haulingURL,
			APIBase: commercemission.HaulingAPIBase,
			Service: string(orders.LogisticsHauling),
			Secret:  haulingSecret,
			Timeout: timeout,
		})
	}
	return authority
}

func newSection17FMissionAuthorityWithTaxi(
	dispatchEnabled bool, dispatchURL, dispatchSecret string,
	taxiEnabled bool, taxiURL, taxiSecret string,
	haulingEnabled bool, haulingURL, haulingSecret string,
	timeout time.Duration,
) *section17FMissionAuthority {
	authority := newSection17FMissionAuthority(dispatchEnabled, dispatchURL, dispatchSecret, haulingEnabled, haulingURL, haulingSecret, timeout)
	if taxiEnabled {
		authority.taxi = commercemission.New(commercemission.Options{
			BaseURL: taxiURL, APIBase: commercemission.TaxiAPIBase,
			Service: string(orders.LogisticsTaxi), Secret: taxiSecret, Timeout: timeout,
		})
	}
	return authority
}

// legacyClientFor resolves the shared-contract client for an authority.
//
// The value is parsed through the canonical vocabulary rather than compared
// raw: a plan carrying the product spelling TAXI_GOODS names the same
// authority as one carrying TAXI, and a raw comparison would quietly return no
// client for it — which reads at the call site as "Taxi is not configured"
// rather than as the routing miss it actually is.
func (a *section17FMissionAuthority) legacyClientFor(
	service orders.LogisticsService,
) *commercemission.Client {
	authority, err := logistics.Parse(string(service))
	if err != nil {
		return nil
	}
	switch authority {
	case logistics.Taxi:
		return a.taxi
	case logistics.Hauling:
		return a.hauling
	default:
		return nil
	}
}

// Available implements orders.MissionAuthority.
//
// Availability is the integration's, not the capability service's: it answers
// whether this deployment can reach the authority at all. Whether the
// authority can carry a particular load is the capability gate's question and
// is asked per mission.
func (a *section17FMissionAuthority) Available(service orders.LogisticsService) bool {
	if a == nil {
		return false
	}
	authority, err := logistics.Parse(string(service))
	if err != nil {
		return false
	}
	if authority == logistics.Dispatch {
		return a.dispatch != nil && a.dispatch.Configured()
	}
	client := a.legacyClientFor(service)
	return client != nil && client.Configured()
}

// CreateMission implements orders.MissionAuthority.
//
// The agreed fee travels to the authority and its answer is verified against
// it by the shared contract. An authority that re-prices is refused, so an
// internal re-plan can never raise what the customer pays.
func (a *section17FMissionAuthority) CreateMission(
	ctx context.Context, request orders.MissionRequest,
) (orders.MissionResult, error) {
	// The authority is whatever the frozen plan chose. It is resolved through
	// the canonical vocabulary rather than compared as a raw string, so a
	// TAXI_GOODS spelling and a TAXI spelling reach the same client, and it is
	// matched explicitly rather than by falling through a default: an
	// unrecognised authority must refuse, never silently become the last
	// branch in the chain (VND-004).
	authority, err := logistics.Parse(string(request.Service))
	if err != nil {
		return orders.MissionResult{}, logistics.NewFailure(
			logistics.Authority(request.Service),
			logistics.FailureCapabilityMismatch,
			logistics.ReasonUnknownAuthority,
		)
	}
	if capabilityErr := a.authorizeCapability(ctx, request); capabilityErr != nil {
		failure := logistics.Classify(authority, capabilityErr)
		// A declared incapability is the authority's answer, not a fault:
		// commerce records a refusal it can show the customer rather than
		// retrying a mission that can never succeed.
		return orders.MissionResult{
			Status:      orders.MissionRefused,
			RefusalCode: capabilityRefusalCode(failure),
		}, nil
	}
	switch authority {
	case logistics.Dispatch:
		return a.createDispatchMission(ctx, request)
	case logistics.Taxi, logistics.Hauling:
		return a.createHaulingMission(ctx, request)
	default:
		return orders.MissionResult{}, logistics.NewFailure(
			authority, logistics.FailureCapabilityMismatch, logistics.ReasonNotPlatform,
		)
	}
}

// capabilityRefusalCode prefers the authority's own reason code and falls back
// to the normalized category, so a refusal is always explainable without
// exposing an internal error message.
func capabilityRefusalCode(failure logistics.Failure) string {
	if failure.ReasonCode != "" {
		return failure.ReasonCode
	}
	return string(failure.Category)
}

var (
	_ orders.InventoryAuthority = (*section17FInventoryAuthority)(nil)
	_ orders.MissionAuthority   = (*section17FMissionAuthority)(nil)
)
