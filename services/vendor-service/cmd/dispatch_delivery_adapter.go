package main

// Dispatch Commerce delivery adapter.
//
// This is the only Vendor -> Dispatch V1 translation. The orders feature
// supplies committed Vendor evidence; the shared client owns HTTP and HMAC;
// Dispatch remains the route, quote, mission, marketplace and custody
// authority.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

func (a *section17FMissionAuthority) createDispatchMission(
	ctx context.Context, request orders.MissionRequest,
) (orders.MissionResult, error) {
	if a == nil || a.dispatch == nil || !a.dispatch.Configured() {
		return orders.MissionResult{}, dispatchcommerce.ErrNotConfigured
	}
	if request.Execution == nil {
		return orders.MissionResult{}, fmt.Errorf("%w: immutable fulfilment execution is unavailable", dispatchcommerce.ErrInvalidRequest)
	}

	capability, err := a.dispatch.Capability(ctx)
	if err != nil {
		return orders.MissionResult{}, err
	}
	quoteRequest, err := dispatchQuoteRequest(request, capability)
	if err != nil {
		return orders.MissionResult{}, err
	}
	quote, err := a.dispatch.Quote(ctx, quoteRequest)
	if err != nil {
		return orders.MissionResult{}, err
	}
	if !quote.Serviceable {
		return orders.MissionResult{Status: orders.MissionRefused, RefusalCode: quote.ReasonCode}, nil
	}
	if quote.FeeMinor != request.AgreedFeeMinor || !strings.EqualFold(quote.Currency, request.Currency) {
		return orders.MissionResult{}, fmt.Errorf("dispatch route quote changed the customer-approved delivery terms")
	}

	create := dispatchcommerce.MissionCreateRequest{
		OperationRef: request.MissionRef, ParentOrderRef: request.ParentOrderID,
		CustomerRef: request.CustomerID, Plan: *quoteRequest.Plan,
		Quote: dispatchcommerce.QuoteBinding{
			QuoteID: quote.QuoteID, QuoteReference: quote.QuoteReference,
			QuoteRevision: quote.Revision, QuoteFingerprint: quote.RequestFingerprint,
		},
		Mode: quoteRequest.Mode, ServiceClass: quoteRequest.ServiceClass,
		PickupStops: quoteRequest.PickupStops, Receiver: quoteRequest.Receiver,
		Parcels: quoteRequest.Parcels, Vehicle: quoteRequest.Vehicle,
		PromisedBy:       quoteRequest.PromisedBy,
		FundingReference: strings.TrimSpace(request.FundingReference),
		CorrelationID:    request.ParentOrderID,
	}
	mission, err := a.dispatch.CreateMission(ctx, create)
	if err != nil && errors.Is(err, dispatchcommerce.ErrAuthorityUnavailable) {
		// A transport failure after POST is an unknown outcome, never permission
		// to create another vehicle. Resolve the stable operation first.
		if recovered, readErr := a.dispatch.ReadOperation(ctx, request.MissionRef); readErr == nil {
			mission = recovered
			err = nil
		}
	}
	if err != nil {
		return orders.MissionResult{}, err
	}
	if strings.TrimSpace(mission.MissionRef) == "" {
		return orders.MissionResult{}, fmt.Errorf("dispatch returned no canonical mission reference")
	}
	return orders.MissionResult{
		ExternalRef: mission.MissionRef, Status: orders.MissionAccepted,
		AcceptedFeeMinor: quote.FeeMinor,
	}, nil
}

func (a *section17FMissionAuthority) QuoteMission(ctx context.Context, request orders.MissionRequest) (orders.MissionQuote, error) {
	if a == nil || a.dispatch == nil || !a.dispatch.Configured() || request.Execution == nil {
		return orders.MissionQuote{}, dispatchcommerce.ErrNotConfigured
	}
	capability, err := a.dispatch.Capability(ctx)
	if err != nil {
		return orders.MissionQuote{}, err
	}
	quoteRequest, err := dispatchQuoteRequest(request, capability)
	if err != nil {
		return orders.MissionQuote{}, err
	}
	quote, err := a.dispatch.Quote(ctx, quoteRequest)
	if err != nil {
		return orders.MissionQuote{}, err
	}
	return orders.MissionQuote{FeeMinor: quote.FeeMinor, Currency: quote.Currency, Serviceable: quote.Serviceable, ReasonCode: quote.ReasonCode}, nil
}

// CancelMission resolves Vendor's stable create operation to Dispatch's opaque
// canonical mission, then issues one idempotent authority command. An unknown
// command response is resolved from the mission read model before returning.
func (a *section17FMissionAuthority) CancelMission(
	ctx context.Context, operationRef, reasonCode string,
) error {
	if a == nil || a.dispatch == nil || !a.dispatch.Configured() {
		return a.cancelHaulingMission(ctx, operationRef, reasonCode)
	}
	mission, err := a.dispatch.ReadOperation(ctx, operationRef)
	if err != nil {
		// No mission was committed yet; there is no Dispatch execution to
		// cancel and Vendor may safely cancel its pre-execution allocation.
		if errors.Is(err, dispatchcommerce.ErrInvalidRequest) {
			haulingErr := a.cancelHaulingMission(ctx, operationRef, reasonCode)
			if haulingErr == nil || errors.Is(haulingErr, commercemission.ErrInvalidRequest) ||
				errors.Is(haulingErr, commercemission.ErrNotConfigured) {
				return nil
			}
			return haulingErr
		}
		return err
	}
	command := dispatchcommerce.CommandRequest{
		OperationRef: operationRef + "-cancel", ExpectedVersion: mission.Version,
		ReasonCode: strings.TrimSpace(reasonCode), CorrelationID: operationRef,
	}
	_, err = a.dispatch.CancelMission(ctx, mission.MissionRef, command)
	if err == nil {
		return nil
	}
	if !errors.Is(err, dispatchcommerce.ErrAuthorityUnavailable) {
		return err
	}
	recovered, readErr := a.dispatch.ReadMission(ctx, mission.MissionRef)
	if readErr == nil && strings.EqualFold(recovered.State, "CANCELLED") {
		return nil
	}
	return err
}

func dispatchQuoteRequest(
	request orders.MissionRequest, capability dispatchcommerce.MotorcycleCapability,
) (dispatchcommerce.QuoteRequest, error) {
	execution := request.Execution
	mode := dispatchMode(execution.Mode)
	if !mode.Valid() {
		return dispatchcommerce.QuoteRequest{}, fmt.Errorf("%w: unsupported fulfilment mode", dispatchcommerce.ErrInvalidRequest)
	}
	parcelsByUnit := make(map[string][]dispatchcommerce.Parcel)
	allParcels := make([]dispatchcommerce.Parcel, 0, len(execution.Parcels))
	for _, source := range execution.Parcels {
		count := source.PackageCount
		if count < 1 {
			count = 1
		}
		manifestRef := fmt.Sprintf("%s-v%d", source.ParcelRef, source.ManifestVersion)
		parcel := dispatchcommerce.Parcel{
			ParcelRef: source.ParcelRef, VendorRef: source.BusinessID,
			BranchRef: source.BranchID, FulfilmentUnit: source.UnitRef,
			VendorOrderRef: source.VendorOrderID, ManifestRef: manifestRef,
			ManifestVersion: source.ManifestVersion,
			Manifest:        json.RawMessage(source.Manifest), WeightGrams: source.WeightGrams,
			LengthMm: source.LengthMM, WidthMm: source.WidthMM, HeightMm: source.HeightMM,
			PackageCount: count, Handling: parcelHandling(source),
		}
		parcelsByUnit[source.UnitRef] = append(parcelsByUnit[source.UnitRef], parcel)
	}

	pickups := make([]dispatchcommerce.PickupStop, 0, len(execution.Stops))
	for _, source := range execution.Stops {
		parcels := parcelsByUnit[source.UnitRef]
		if len(parcels) == 0 {
			return dispatchcommerce.QuoteRequest{}, fmt.Errorf("%w: pickup has no stable parcel", dispatchcommerce.ErrInvalidRequest)
		}
		refs := make([]string, 0, len(parcels))
		handling := dispatchcommerce.Handling{}
		for index := range parcels {
			parcels[index].PickupStopRef = source.StopRef
			refs = append(refs, parcels[index].ParcelRef)
			handling = mergeHandling(handling, parcels[index].Handling)
			allParcels = append(allParcels, parcels[index])
		}
		pickups = append(pickups, dispatchcommerce.PickupStop{
			StopRef: source.StopRef, PhysicalStopRef: source.PhysicalStopRef,
			VendorRef: source.BusinessID, BranchRef: source.BranchID,
			ManifestRef:     fmt.Sprintf("%s-%s", execution.PlanRef, source.StopRef),
			ManifestVersion: execution.PlanVersion,
			Location:        dispatchcommerce.Point{Ref: source.BranchID, Address: source.Address, Latitude: source.Latitude, Longitude: source.Longitude},
			ReadyWindow:     dispatchcommerce.ReadyWindow{From: source.ReadyFrom, To: source.ReadyTo},
			ParcelRefs:      refs, Handling: handling,
		})
	}

	return dispatchcommerce.QuoteRequest{
		QuoteReference: request.MissionRef + "-route-v1", Mode: mode,
		ServiceClass: strings.ToUpper(strings.TrimSpace(execution.ServiceClass)),
		Plan: &dispatchcommerce.PlanBinding{
			PlanRef: execution.PlanRef, PlanVersion: execution.PlanVersion,
			PlanFingerprint: execution.PlanFingerprint, MissionSlotRef: request.PlanMissionRef,
		},
		PickupStops: pickups,
		Receiver: dispatchcommerce.Point{
			Ref:      request.DeliveryAddress.SourceAddressRef,
			Address:  request.DeliveryAddress.Address,
			Latitude: request.DeliveryAddress.Latitude, Longitude: request.DeliveryAddress.Longitude,
			Contact: request.Receiver.ReceiverName, Phone: request.Receiver.ReceiverPhone,
			Notes: request.DeliveryAddress.Description,
		},
		Parcels: allParcels,
		Vehicle: dispatchcommerce.VehicleRequirement{
			Class: capability.Class, CapabilityVersion: capability.Version,
			MaxWeightGrams: capability.MaxWeightGrams, MaxVolumeMl: capability.MaxVolumeMl,
			LongestDimensionMm: capability.MaxDimensionMm,
		},
		PromisedBy: execution.PromisedBy, Currency: strings.ToUpper(strings.TrimSpace(request.Currency)),
		CorrelationID: request.ParentOrderID,
	}, nil
}

func dispatchMode(value string) dispatchcommerce.Mode {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(dispatchcommerce.ModeFastest):
		return dispatchcommerce.ModeFastest
	case string(dispatchcommerce.ModeSaver):
		return dispatchcommerce.ModeSaver
	default:
		return dispatchcommerce.ModeBalanced
	}
}

func parcelHandling(parcel orders.MissionParcel) dispatchcommerce.Handling {
	handling := dispatchcommerce.Handling{Fragile: parcel.Fragile}
	switch strings.ToUpper(strings.TrimSpace(parcel.TemperatureControl)) {
	case "HOT":
		handling.Hot = true
	case "CHILLED":
		handling.Chilled = true
	case "FROZEN":
		handling.Frozen = true
	}
	return handling
}

func mergeHandling(left, right dispatchcommerce.Handling) dispatchcommerce.Handling {
	left.Fragile = left.Fragile || right.Fragile
	left.Upright = left.Upright || right.Upright
	left.SpillLiquid = left.SpillLiquid || right.SpillLiquid
	left.Hot = left.Hot || right.Hot
	left.Chilled = left.Chilled || right.Chilled
	left.Frozen = left.Frozen || right.Frozen
	left.HighValue = left.HighValue || right.HighValue
	return left
}
