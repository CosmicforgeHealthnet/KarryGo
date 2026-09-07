package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/commercemission"
)

func (a *section17FMissionAuthority) createHaulingMission(
	ctx context.Context,
	request orders.MissionRequest,
) (orders.MissionResult, error) {
	client := a.legacyClientFor(request.Service)
	if client == nil || !client.Configured() {
		return orders.MissionResult{}, commercemission.ErrNotConfigured
	}
	payload, err := haulingMissionRequest(request)
	if err != nil {
		return orders.MissionResult{}, err
	}
	mission, err := client.CreateMission(ctx, payload)
	if err != nil && errors.Is(err, commercemission.ErrAuthorityUnavailable) {
		if recovered, readErr := client.ReadMission(ctx, request.MissionRef); readErr == nil {
			mission = recovered
			err = nil
		}
	}
	if err != nil {
		return orders.MissionResult{}, err
	}
	return haulingMissionResult(mission), nil
}

func (a *section17FMissionAuthority) cancelHaulingMission(
	ctx context.Context,
	operationRef, reasonCode string,
) error {
	if a == nil {
		return commercemission.ErrNotConfigured
	}
	return cancelCommerceMission(ctx, a.hauling, operationRef, reasonCode)
}

func (a *section17FMissionAuthority) cancelLegacyMission(ctx context.Context, operationRef, reasonCode string) error {
	if a == nil {
		return commercemission.ErrNotConfigured
	}
	configured := false
	for _, client := range []*commercemission.Client{a.taxi, a.hauling} {
		if client == nil || !client.Configured() {
			continue
		}
		configured = true
		err := cancelCommerceMission(ctx, client, operationRef, reasonCode)
		if err == nil {
			return nil
		}
		if !errors.Is(err, commercemission.ErrInvalidRequest) {
			return err
		}
	}
	if !configured {
		return commercemission.ErrNotConfigured
	}
	return nil
}

func cancelCommerceMission(ctx context.Context, client *commercemission.Client, operationRef, reasonCode string) error {
	if client == nil || !client.Configured() {
		return commercemission.ErrNotConfigured
	}
	mission, err := client.ReadMission(ctx, operationRef)
	if err != nil {
		if errors.Is(err, commercemission.ErrInvalidRequest) {
			return nil
		}
		return err
	}
	_, err = client.CancelMission(ctx, commercemission.CancelRequest{
		OperationRef:    operationRef,
		MissionRef:      mission.MissionRef,
		ReasonCode:      strings.TrimSpace(reasonCode),
		ExpectedVersion: mission.Version,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, commercemission.ErrAuthorityUnavailable) {
		return err
	}
	recovered, readErr := client.ReadMission(ctx, operationRef)
	if readErr == nil && recovered.Status == commercemission.StatusCancelled {
		return nil
	}
	return err
}

func haulingMissionRequest(request orders.MissionRequest) (commercemission.Request, error) {
	payload := commercemission.Request{
		MissionRef:      request.MissionRef,
		OperationRef:    request.MissionRef,
		ParentOrderRef:  request.ParentOrderID,
		VendorOrderRefs: request.VendorOrderIDs,
		CustomerRef:     request.CustomerID,
		Service:         commercemission.Service(request.Service),
		VehicleClass:    commercemission.VehicleClass(request.VehicleClass),
		QuoteReference:  request.MissionRef + "-route-v1",
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
	}
	if request.Execution == nil {
		payload.Pickup = commercemission.Point{
			Ref:       request.PickupBranch,
			Address:   "Vendor branch " + request.PickupBranch,
			Latitude:  request.DeliveryAddress.Latitude,
			Longitude: request.DeliveryAddress.Longitude,
		}
		return payload, nil
	}

	execution := request.Execution
	payload.QuoteID = strings.TrimSpace(execution.AuthorityQuoteID)
	if reference := strings.TrimSpace(execution.AuthorityQuoteReference); reference != "" {
		payload.QuoteReference = reference
	}
	if request.Service == orders.LogisticsTaxi && (payload.QuoteID == "" || payload.QuoteReference == "") {
		return commercemission.Request{}, fmt.Errorf("%w: taxi mission requires the frozen authority quote", commercemission.ErrInvalidRequest)
	}
	payload.Load.LoadingDurationSeconds = execution.LoadingDurationSeconds
	payload.Plan = &commercemission.PlanBinding{
		PlanRef: execution.PlanRef, PlanVersion: execution.PlanVersion,
		PlanFingerprint: execution.PlanFingerprint, MissionSlotRef: request.PlanMissionRef,
	}
	parcelsByUnit := map[string][]commercemission.Parcel{}
	for _, source := range execution.Parcels {
		count := source.PackageCount
		if count < 1 {
			count = 1
		}
		manifestRef := fmt.Sprintf("%s-v%d", source.ParcelRef, source.ManifestVersion)
		parcel := commercemission.Parcel{
			ParcelRef: source.ParcelRef, VendorRef: source.BusinessID, BranchRef: source.BranchID,
			FulfilmentUnit: source.UnitRef, VendorOrderRef: source.VendorOrderID,
			ManifestRef: manifestRef, ManifestVersion: source.ManifestVersion,
			Manifest: json.RawMessage(source.Manifest), WeightGrams: source.WeightGrams,
			LengthMm: source.LengthMM, WidthMm: source.WidthMM, HeightMm: source.HeightMM,
			PackageCount: count, Handling: haulingParcelHandling(source),
		}
		parcelsByUnit[source.UnitRef] = append(parcelsByUnit[source.UnitRef], parcel)
	}

	for _, source := range execution.Stops {
		parcels := parcelsByUnit[source.UnitRef]
		if len(parcels) == 0 {
			return commercemission.Request{}, fmt.Errorf("%w: pickup has no stable parcel", commercemission.ErrInvalidRequest)
		}
		refs := make([]string, 0, len(parcels))
		handling := commercemission.Handling{}
		for index := range parcels {
			parcels[index].PickupStopRef = source.StopRef
			refs = append(refs, parcels[index].ParcelRef)
			handling = mergeHaulingHandling(handling, parcels[index].Handling)
			payload.Parcels = append(payload.Parcels, parcels[index])
		}
		stop := commercemission.PickupStop{
			StopRef: source.StopRef, PhysicalStopRef: source.PhysicalStopRef,
			VendorRef: source.BusinessID, BranchRef: source.BranchID,
			ManifestRef:     fmt.Sprintf("%s-%s", execution.PlanRef, source.StopRef),
			ManifestVersion: execution.PlanVersion,
			Location: commercemission.Point{
				Ref: source.BranchID, Address: source.Address, Latitude: source.Latitude, Longitude: source.Longitude,
			},
			ReadyWindow: commercemission.ReadyWindow{
				From: source.ReadyFrom, To: source.ReadyTo, Timezone: "Africa/Lagos",
			},
			ParcelRefs: refs, Handling: handling,
		}
		payload.PickupStops = append(payload.PickupStops, stop)
		payload.Handling = mergeHaulingHandling(payload.Handling, handling)
	}
	if len(payload.PickupStops) == 0 {
		return commercemission.Request{}, fmt.Errorf("%w: hauling mission has no pickup stop", commercemission.ErrInvalidRequest)
	}
	payload.Pickup = payload.PickupStops[0].Location
	payload.Load.Description = "Commerce fulfilment manifest " + execution.PlanRef
	payload.Vehicle = commercemission.VehicleRequirement{Class: commercemission.VehicleClass(request.VehicleClass)}
	return payload, nil
}

func haulingMissionResult(mission commercemission.Mission) orders.MissionResult {
	result := orders.MissionResult{
		ExternalRef:      mission.ExternalRef,
		RefusalCode:      mission.RefusalCode,
		AcceptedFeeMinor: mission.AcceptedFeeMinor,
	}
	if mission.Status == commercemission.StatusRefused {
		result.Status = orders.MissionRefused
	} else {
		result.Status = orders.MissionAccepted
	}
	return result
}

func haulingParcelHandling(parcel orders.MissionParcel) commercemission.Handling {
	handling := commercemission.Handling{
		Fragile: parcel.Fragile, Upright: parcel.Upright,
		StackRestricted: parcel.StackRestricted,
	}
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

func mergeHaulingHandling(left, right commercemission.Handling) commercemission.Handling {
	left.Fragile = left.Fragile || right.Fragile
	left.Upright = left.Upright || right.Upright
	left.StackRestricted = left.StackRestricted || right.StackRestricted
	left.SpillLiquid = left.SpillLiquid || right.SpillLiquid
	left.Hot = left.Hot || right.Hot
	left.Chilled = left.Chilled || right.Chilled
	left.Frozen = left.Frozen || right.Frozen
	left.HighValue = left.HighValue || right.HighValue
	left.SpecialHandling = append(left.SpecialHandling, right.SpecialHandling...)
	return left
}
