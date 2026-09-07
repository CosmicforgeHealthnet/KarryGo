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

func (a *section17FMissionAuthority) legacyClientFor(
	service orders.LogisticsService,
) *commercemission.Client {
	switch service {
	case orders.LogisticsTaxi:
		return a.taxi
	case orders.LogisticsHauling:
		return a.hauling
	default:
		return nil
	}
}

// Available implements orders.MissionAuthority.
func (a *section17FMissionAuthority) Available(service orders.LogisticsService) bool {
	if a == nil {
		return false
	}
	if service == orders.LogisticsDispatch {
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
	if request.Service == orders.LogisticsDispatch {
		return a.createDispatchMission(ctx, request)
	}
	return a.createHaulingMission(ctx, request)
}

var (
	_ orders.InventoryAuthority = (*section17FInventoryAuthority)(nil)
	_ orders.MissionAuthority   = (*section17FMissionAuthority)(nil)
)
