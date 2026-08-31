package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/walletclient"
)

type vendorDeliveryRuntimeAuthority struct {
	db         *pgxpool.Pool
	fulfilment *fulfilmentgroup.Service
	projection *fulfilmentOperationsProjection
	orders     *orders.Service
	wallet     walletclient.Client
}

func newVendorDeliveryRuntimeAuthority(db *pgxpool.Pool, fulfilment *fulfilmentgroup.Service, projection *fulfilmentOperationsProjection, orderService *orders.Service, walletURL, walletSecret string, timeout time.Duration) *vendorDeliveryRuntimeAuthority {
	if db == nil || fulfilment == nil || projection == nil || orderService == nil || strings.TrimSpace(walletURL) == "" || strings.TrimSpace(walletSecret) == "" {
		return nil
	}
	return &vendorDeliveryRuntimeAuthority{db: db, fulfilment: fulfilment, projection: projection, orders: orderService, wallet: walletclient.Client{BaseURL: walletURL, ServiceName: "vendor-service", Secret: []byte(walletSecret), HTTPClient: &http.Client{Timeout: timeout}}}
}

func (a *vendorDeliveryRuntimeAuthority) TransitionFunding(ctx context.Context, request fulfilmentgroup.FallbackFundingRequest) (string, error) {
	result, err := a.wallet.TransitionDeliveryResponsibility(ctx, request.VendorOrderID, walletclient.DeliveryResponsibilityTransitionRequest{OperationRef: request.OperationRef, IdempotencyKey: request.OperationRef, ReasonCode: request.ReasonCode, PlatformExecutionCostMinor: request.PlatformExecutionCostMinor})
	if err != nil {
		recovered, readErr := a.wallet.GetDeliveryResponsibilityTransition(ctx, request.OperationRef)
		if readErr == nil {
			result = recovered
			err = nil
		}
	}
	if err != nil {
		return "", err
	}
	if result.Status != "APPLIED" || result.ToResponsibility != "PLATFORM" || result.VendorOrderID != request.VendorOrderID {
		return "", fmt.Errorf("wallet returned an invalid delivery responsibility transition")
	}
	return result.OperationRef, nil
}

func (a *vendorDeliveryRuntimeAuthority) QuotePlatformMission(ctx context.Context, request fulfilmentgroup.FallbackMissionRequest) (int64, error) {
	parentOrderID, order, execution, plan, err := a.fallbackMissionEvidence(ctx, request)
	_ = parentOrderID
	if err != nil {
		return 0, err
	}
	quote, err := a.orders.QuotePlatformFallback(ctx, orders.MissionRequest{MissionRef: "quote-" + request.OperationRef, ParentOrderID: order.ID, VendorOrderIDs: plan.VendorOrderIDs, CustomerID: order.CustomerID, Service: plan.Service, VehicleClass: plan.VehicleClass, Currency: order.Currency, PlanMissionRef: plan.PlanMissionRef, PickupBusiness: plan.PickupBusiness, PickupBranch: plan.PickupBranch, DeliveryAddress: order.Address, Receiver: order.Receiver, PackageCount: plan.PackageCount, Execution: &execution})
	if err != nil {
		return 0, err
	}
	if !quote.Serviceable {
		return 0, fmt.Errorf("Dispatch refused fallback route: %s", quote.ReasonCode)
	}
	if !strings.EqualFold(quote.Currency, order.Currency) || quote.FeeMinor < 0 {
		return 0, errors.New("Dispatch fallback quote is invalid")
	}
	return quote.FeeMinor, nil
}

func (a *vendorDeliveryRuntimeAuthority) CreatePlatformMission(ctx context.Context, request fulfilmentgroup.FallbackMissionRequest) (fulfilmentgroup.FallbackMissionResult, error) {
	ref, _, err := a.fulfilment.ReserveMission(ctx, request.GroupID, request.Plan.ID, request.Mission.ID, fulfilmentgroup.AuthorityDispatch)
	if err != nil {
		return fulfilmentgroup.FallbackMissionResult{}, err
	}
	var parentOrderID string
	err = a.db.QueryRow(ctx, `SELECT parent_order_id FROM commerce_fulfilment_groups WHERE id=$1`, request.GroupID).Scan(&parentOrderID)
	if err != nil {
		return fulfilmentgroup.FallbackMissionResult{}, err
	}
	pickups := request.Mission.PickupStops()
	if len(pickups) == 0 {
		return fulfilmentgroup.FallbackMissionResult{}, fulfilmentgroup.ErrInvalid
	}
	plan := orders.MissionPlan{MissionRef: ref.MissionRef, PlanMissionRef: request.Mission.SlotRef, Service: orders.LogisticsDispatch, VehicleClass: request.Mission.VehicleClass, AgreedFeeMinor: request.Mission.PlannedCostMinor, VendorOrderIDs: request.VendorOrderIDs, PickupBusiness: pickups[0].BusinessID, PickupBranch: pickups[0].BranchID, PackageCount: int64(len(request.Mission.ParcelIDs))}
	result, err := a.orders.ExecutePlatformFallback(ctx, parentOrderID, plan, strings.Join(request.FundingReferences, ","))
	if err != nil {
		return fulfilmentgroup.FallbackMissionResult{}, err
	}
	if result.Status != orders.MissionAccepted || strings.TrimSpace(result.ExternalRef) == "" {
		return fulfilmentgroup.FallbackMissionResult{}, errors.New("Dispatch refused the Hybrid fallback mission")
	}
	linked, found, err := a.fulfilment.LiveMissionRefForSlot(ctx, parentOrderID, request.Mission.SlotRef)
	if err != nil {
		return fulfilmentgroup.FallbackMissionResult{}, err
	}
	if !found || linked.LinkState != fulfilmentgroup.LinkLinked {
		return fulfilmentgroup.FallbackMissionResult{}, errors.New("platform mission reference was not linked")
	}
	return fulfilmentgroup.FallbackMissionResult{PlatformMissionRefID: linked.ID, ExternalRef: result.ExternalRef}, nil
}

func (a *vendorDeliveryRuntimeAuthority) fallbackMissionEvidence(ctx context.Context, request fulfilmentgroup.FallbackMissionRequest) (string, orders.ParentOrder, orders.MissionExecution, orders.MissionPlan, error) {
	var parentOrderID string
	if err := a.db.QueryRow(ctx, `SELECT parent_order_id FROM commerce_fulfilment_groups WHERE id=$1`, request.GroupID).Scan(&parentOrderID); err != nil {
		return "", orders.ParentOrder{}, orders.MissionExecution{}, orders.MissionPlan{}, err
	}
	order, err := a.orders.ParentOrderForFulfilment(ctx, parentOrderID)
	if err != nil {
		return "", order, orders.MissionExecution{}, orders.MissionPlan{}, err
	}
	pickups := request.Mission.PickupStops()
	if len(pickups) == 0 {
		return "", order, orders.MissionExecution{}, orders.MissionPlan{}, fulfilmentgroup.ErrInvalid
	}
	parcels, err := a.fulfilment.ParcelsForMissionSlot(ctx, request.Mission.ID)
	if err != nil {
		return "", order, orders.MissionExecution{}, orders.MissionPlan{}, err
	}
	byUnit := map[string][]fulfilmentgroup.Parcel{}
	for _, parcel := range parcels {
		byUnit[parcel.UnitID] = append(byUnit[parcel.UnitID], parcel)
	}
	now := time.Now().UTC()
	execution := orders.MissionExecution{PlanRef: "prospective-" + request.OperationRef, PlanVersion: request.Plan.Version + 1, PlanFingerprint: fulfilmentgroup.Fingerprint(fulfilmentgroup.ModeHybrid, []fulfilmentgroup.PlanMission{request.Mission}), Mode: order.SelectedFulfilmentMode, ServiceClass: "COMMERCE"}
	for _, stop := range pickups {
		readyFrom, readyTo := a.projection.readyWindow(ctx, order.ID, byUnit[stop.UnitID], now)
		execution.Stops = append(execution.Stops, orders.MissionStop{StopRef: stop.ID, PhysicalStopRef: physicalStopRef(stop), UnitRef: stop.UnitID, BusinessID: stop.BusinessID, BranchID: stop.BranchID, Address: stop.Address, Latitude: stop.Latitude, Longitude: stop.Longitude, ReadyFrom: readyFrom, ReadyTo: readyTo})
		for _, parcel := range byUnit[stop.UnitID] {
			manifest, marshalErr := json.Marshal(parcel.Manifest)
			if marshalErr != nil {
				return "", order, orders.MissionExecution{}, orders.MissionPlan{}, marshalErr
			}
			execution.Parcels = append(execution.Parcels, orders.MissionParcel{ParcelRef: parcel.ParcelRef, UnitRef: parcel.UnitID, VendorOrderID: parcel.VendorOrderID, BusinessID: parcel.BusinessID, BranchID: parcel.BranchID, Manifest: manifest, ManifestVersion: parcel.ManifestVersion, WeightGrams: parcel.WeightGrams, LengthMM: parcel.LengthMm, WidthMM: parcel.WidthMm, HeightMM: parcel.HeightMm, PackageCount: parcel.Manifest.PackageCount, Fragile: parcel.Fragile, TemperatureControl: string(parcel.TemperatureControl)})
		}
	}
	execution.PromisedBy = now.Add(2 * time.Hour)
	plan := orders.MissionPlan{PlanMissionRef: request.Mission.SlotRef, Service: orders.LogisticsDispatch, VehicleClass: request.Mission.VehicleClass, VendorOrderIDs: request.VendorOrderIDs, PickupBusiness: pickups[0].BusinessID, PickupBranch: pickups[0].BranchID, PackageCount: int64(len(request.Mission.ParcelIDs))}
	return parentOrderID, order, execution, plan, nil
}

var _ fulfilmentgroup.HybridFallbackAuthority = (*vendorDeliveryRuntimeAuthority)(nil)
