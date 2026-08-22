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
	dispatch *commercemission.Client
	hauling  *commercemission.Client
}

func newSection17FMissionAuthority(
	dispatchEnabled bool, dispatchURL, dispatchSecret string,
	haulingEnabled bool, haulingURL, haulingSecret string,
	timeout time.Duration,
) *section17FMissionAuthority {
	authority := &section17FMissionAuthority{}
	if dispatchEnabled {
		authority.dispatch = commercemission.New(commercemission.Options{
			// BOTH URLs are BARE ORIGINS: the client appends each service's
			// own API base and path itself.
			BaseURL: dispatchURL,
			APIBase: commercemission.DispatchAPIBase,
			Service: string(orders.LogisticsDispatch),
			Secret:  dispatchSecret,
			Timeout: timeout,
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

func (a *section17FMissionAuthority) clientFor(
	service orders.LogisticsService,
) *commercemission.Client {
	switch service {
	case orders.LogisticsDispatch:
		return a.dispatch
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
	client := a.clientFor(service)
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
	client := a.clientFor(request.Service)
	if client == nil || !client.Configured() {
		return orders.MissionResult{}, commercemission.ErrNotConfigured
	}

	mission, err := client.CreateMission(ctx, commercemission.Request{
		MissionRef:      request.MissionRef,
		ParentOrderRef:  request.ParentOrderID,
		VendorOrderRefs: request.VendorOrderIDs,
		CustomerRef:     request.CustomerID,
		Service:         commercemission.Service(request.Service),
		VehicleClass:    commercemission.VehicleClass(request.VehicleClass),
		Pickup: commercemission.Point{
			Ref: request.PickupBranch,
			// The pickup address is the branch's own; the authority resolves
			// it from the branch reference it already knows.
			Address:   "Vendor branch " + request.PickupBranch,
			Latitude:  request.DeliveryAddress.Latitude,
			Longitude: request.DeliveryAddress.Longitude,
		},
		Dropoff: commercemission.Point{
			Ref:       request.DeliveryAddress.SourceAddressRef,
			Address:   request.DeliveryAddress.Address,
			Latitude:  request.DeliveryAddress.Latitude,
			Longitude: request.DeliveryAddress.Longitude,
			Contact:   request.Receiver.ReceiverName,
			Phone:     request.Receiver.ReceiverPhone,
			Notes:     request.DeliveryAddress.Description,
		},
		Load: commercemission.Load{
			WeightGrams:  request.WeightGrams,
			PackageCount: request.PackageCount,
		},
		Currency:       request.Currency,
		AgreedFeeMinor: request.AgreedFeeMinor,
		RequestedAt:    time.Now().UTC(),
	})
	if err != nil {
		return orders.MissionResult{}, err
	}

	result := orders.MissionResult{
		ExternalRef:      mission.ExternalRef,
		RefusalCode:      mission.RefusalCode,
		AcceptedFeeMinor: mission.AcceptedFeeMinor,
	}
	if mission.Status == commercemission.StatusRefused {
		result.Status = orders.MissionRefused
		return result, nil
	}
	result.Status = orders.MissionAccepted
	return result, nil
}

var (
	_ orders.InventoryAuthority = (*section17FInventoryAuthority)(nil)
	_ orders.MissionAuthority   = (*section17FMissionAuthority)(nil)
)
