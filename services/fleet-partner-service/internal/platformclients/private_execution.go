package platformclients

import (
	"context"
	"net/http"
	"net/url"
)

type PrivateRouteSnapshot struct {
	PickupAddress    string  `json:"pickupAddress"`
	PickupLatitude   float64 `json:"pickupLatitude"`
	PickupLongitude  float64 `json:"pickupLongitude"`
	DropoffAddress   string  `json:"dropoffAddress"`
	DropoffLatitude  float64 `json:"dropoffLatitude"`
	DropoffLongitude float64 `json:"dropoffLongitude"`
}
type PrivateCargoSnapshot struct {
	Description string `json:"description,omitempty"`
	WeightKG    int    `json:"weightKg,omitempty"`
	CargoType   string `json:"cargoType,omitempty"`
}
type PrivateExecutionRequest struct {
	OrganizationID           string               `json:"organizationId"`
	SettlementOrganizationID string               `json:"settlementOrganizationId"`
	ExecutingProviderID      string               `json:"executingProviderId"`
	OrganizationMembershipID string               `json:"organizationMembershipId"`
	PrivateOrderID           string               `json:"privateOrderId"`
	ShipmentID               string               `json:"shipmentId"`
	PrivateAssignmentID      string               `json:"privateAssignmentId"`
	ProviderResponseID       string               `json:"providerResponseId"`
	FleetOperationID         string               `json:"fleetOperationId"`
	ServiceCategory          string               `json:"serviceCategory"`
	VehicleID                string               `json:"vehicleId,omitempty"`
	TruckID                  string               `json:"truckId,omitempty"`
	ContractReference        string               `json:"contractReference,omitempty"`
	QuotationReference       string               `json:"quotationReference,omitempty"`
	Route                    PrivateRouteSnapshot `json:"route"`
	Cargo                    PrivateCargoSnapshot `json:"cargo,omitempty"`
	CorrelationID            string               `json:"correlationId,omitempty"`
}
type PrivateExecutionView struct {
	FleetOperationID       string `json:"fleetOperationId"`
	Status                 string `json:"status"`
	CanonicalRequestID     string `json:"canonicalRequestId"`
	CanonicalReservationID string `json:"canonicalReservationId"`
	CanonicalBookingID     string `json:"canonicalBookingId"`
	CanonicalTripID        string `json:"canonicalTripId"`
	SourceVersion          int64  `json:"sourceVersion"`
	CancellationStatus     string `json:"cancellationStatus"`
}
type PrivateCancellationView struct {
	Status        string `json:"status"`
	SourceVersion int64  `json:"sourceVersion"`
}
type privateEnvelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

type PrivateExecutionClient struct {
	client   *Client
	basePath string
}

func NewPrivateExecutionClient(client *Client, basePath string) *PrivateExecutionClient {
	return &PrivateExecutionClient{client, basePath}
}
func (p *PrivateExecutionClient) Configured() bool { return p != nil && p.client.Configured() }
func (p *PrivateExecutionClient) Create(ctx context.Context, meta CallMeta, in PrivateExecutionRequest) (PrivateExecutionView, error) {
	var env privateEnvelope[PrivateExecutionView]
	err := p.client.Do(ctx, meta, http.MethodPost, p.basePath, in, &env)
	return env.Data, err
}
func (p *PrivateExecutionClient) Read(ctx context.Context, meta CallMeta, operationID string) (PrivateExecutionView, error) {
	var env privateEnvelope[PrivateExecutionView]
	err := p.client.Do(ctx, meta, http.MethodGet, p.basePath+"/by-operation/"+url.PathEscape(operationID), nil, &env)
	return env.Data, err
}
func (p *PrivateExecutionClient) Cancel(ctx context.Context, meta CallMeta, operationID, reason string) (PrivateCancellationView, error) {
	var env privateEnvelope[PrivateCancellationView]
	err := p.client.Do(ctx, meta, http.MethodPost, p.basePath+"/"+url.PathEscape(operationID)+"/cancellations", map[string]string{"reason": reason}, &env)
	return env.Data, err
}
func (p *PrivateExecutionClient) ReadCancellation(ctx context.Context, meta CallMeta, operationID string) (PrivateCancellationView, error) {
	var env privateEnvelope[PrivateCancellationView]
	err := p.client.Do(ctx, meta, http.MethodGet, p.basePath+"/"+url.PathEscape(operationID)+"/cancellation", nil, &env)
	return env.Data, err
}
