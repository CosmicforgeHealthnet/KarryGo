package app

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type privateExecutionAdapter struct {
	dispatch, hauling *platformclients.PrivateExecutionClient
}

func BuildPrivateExecutionAdapter(cfg config.Config, metrics *observability.Metrics) privateusecases.ExecutionPort {
	dispatch := platformclients.NewPrivateExecutionClient(platformclients.New(platformclients.Options{ServiceName: "driver-dispatch-delivery-service", BaseURL: cfg.DispatchServiceURL, Secret: cfg.DispatchServiceSecret, Timeout: cfg.DispatchServiceTimeout, Metrics: metrics}), "/api/v1/internal/private-executions")
	haulingCfg := cfg.Clients["driver-hauling-service"]
	hauling := platformclients.NewPrivateExecutionClient(platformclients.New(platformclients.Options{ServiceName: "driver-hauling-service", BaseURL: haulingCfg.BaseURL, Secret: haulingCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics}), "/api/v1/hauling/internal/private-executions")
	return &privateExecutionAdapter{dispatch, hauling}
}

func (a *privateExecutionAdapter) client(service string) (*platformclients.PrivateExecutionClient, error) {
	switch service {
	case "DISPATCH":
		if a.dispatch.Configured() {
			return a.dispatch, nil
		}
	case "HAULING":
		if a.hauling.Configured() {
			return a.hauling, nil
		}
	case "TAXI":
		return nil, errors.New("taxi private execution is disabled")
	}
	return nil, platformclients.ErrNotConfigured
}
func meta(_ tenant.Context, o privatemodels.ExecutionOperation, key string) platformclients.CallMeta {
	return platformclients.CallMeta{OrganizationID: o.OrganizationID, RequestID: o.ID, CorrelationID: o.CorrelationID, CausationID: o.CausationID, IdempotencyKey: key}
}
func request(o privatemodels.ExecutionOperation) platformclients.PrivateExecutionRequest {
	return platformclients.PrivateExecutionRequest{OrganizationID: o.OrganizationID, SettlementOrganizationID: o.SettlementOrganizationID, ExecutingProviderID: o.ExecutingProviderID, OrganizationMembershipID: o.OrganizationMembershipID, PrivateOrderID: o.PrivateOrderID, ShipmentID: o.ShipmentID, PrivateAssignmentID: o.AssignmentID, ProviderResponseID: o.ProviderResponseID, FleetOperationID: o.ID, ServiceCategory: o.ExecutionService, VehicleID: o.VehicleOrTruckReference, TruckID: o.VehicleOrTruckReference, ContractReference: o.ContractReference, QuotationReference: o.QuotationReference, CorrelationID: o.CorrelationID, Route: platformclients.PrivateRouteSnapshot{PickupAddress: o.Payload.PickupAddress, PickupLatitude: o.Payload.PickupLatitude, PickupLongitude: o.Payload.PickupLongitude, DropoffAddress: o.Payload.DropoffAddress, DropoffLatitude: o.Payload.DropoffLatitude, DropoffLongitude: o.Payload.DropoffLongitude}, Cargo: platformclients.PrivateCargoSnapshot{Description: o.Payload.CargoDescription, WeightKG: o.Payload.CargoWeightKG, CargoType: o.Payload.CargoType}}
}
func outcome(v platformclients.PrivateExecutionView) privateusecases.ExecutionOutcome {
	requestID := v.CanonicalRequestID
	if requestID == "" {
		requestID = v.CanonicalBookingID
	}
	confirmed := v.Status == "CONFIRMED" || v.Status == "IN_PROGRESS" || v.Status == "COMPLETED"
	cancelled := v.Status == "CANCELLED"
	failed := v.Status == "FAILED" || v.Status == "REJECTED" || v.Status == "NOT_CANCELLABLE"
	return privateusecases.ExecutionOutcome{Definite: confirmed || cancelled || failed, Confirmed: confirmed, Cancelled: cancelled, CanonicalRequestReference: requestID, CanonicalTripReference: v.CanonicalTripID, NormalizedStatus: v.Status, SourceVersion: v.SourceVersion}
}
func (a *privateExecutionAdapter) Submit(ctx context.Context, t tenant.Context, o privatemodels.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	c, e := a.client(o.ExecutionService)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	v, e := c.Create(ctx, meta(t, o, o.IdempotencyKey), request(o))
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	return outcome(v), nil
}
func (a *privateExecutionAdapter) ReadBack(ctx context.Context, t tenant.Context, o privatemodels.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	c, e := a.client(o.ExecutionService)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	v, e := c.Read(ctx, meta(t, o, o.IdempotencyKey+":read"), o.ID)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	return outcome(v), nil
}
func (a *privateExecutionAdapter) Cancel(ctx context.Context, t tenant.Context, o privatemodels.ExecutionOperation, reason string) (privateusecases.ExecutionOutcome, error) {
	c, e := a.client(o.ExecutionService)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	v, e := c.Cancel(ctx, meta(t, o, o.IdempotencyKey+":cancel"), o.ID, reason)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	return privateusecases.ExecutionOutcome{Definite: v.Status != "OUTCOME_UNKNOWN" && v.Status != "PENDING", Cancelled: v.Status == "CANCELLED", CanonicalRequestReference: o.CanonicalRequestReference, CanonicalTripReference: o.CanonicalTripReference, NormalizedStatus: v.Status, SourceVersion: v.SourceVersion}, nil
}
func (a *privateExecutionAdapter) ReadCancellation(ctx context.Context, t tenant.Context, o privatemodels.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	c, e := a.client(o.ExecutionService)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	v, e := c.ReadCancellation(ctx, meta(t, o, o.IdempotencyKey+":cancel-read"), o.ID)
	if e != nil {
		return privateusecases.ExecutionOutcome{}, e
	}
	return privateusecases.ExecutionOutcome{Definite: v.Status != "OUTCOME_UNKNOWN" && v.Status != "PENDING", Cancelled: v.Status == "CANCELLED", CanonicalRequestReference: o.CanonicalRequestReference, CanonicalTripReference: o.CanonicalTripReference, NormalizedStatus: v.Status, SourceVersion: v.SourceVersion}, nil
}
