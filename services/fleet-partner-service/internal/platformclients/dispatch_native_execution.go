package platformclients

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DispatchNativeExecution is Fleet's adapter for driver-dispatch-delivery-
// service's REAL internal marketplace API
// (driver-dispatch-delivery-service/internal/features/marketplace/http.go):
//
//	POST /api/v1/internal/marketplace/reservations
//	GET  /api/v1/internal/marketplace/reservations/by-idempotency/{key}
//	POST /api/v1/internal/marketplace/reservations/{reservationId}/release
//	GET  /api/v1/internal/marketplace/requests/{requestId}
//	GET  /api/v1/internal/marketplace/requests/{requestId}/reservation
//	POST /api/v1/internal/marketplace/eligibility/providers/validate
//	POST /api/v1/internal/marketplace/eligibility/vehicles/validate
//	POST /api/v1/internal/marketplace/executions
//	GET  /api/v1/internal/marketplace/executions/by-idempotency/{key}
//	GET  /api/v1/internal/marketplace/trips/{tripId}
//
// Dispatch speaks camelCase and keys reservations by reservation id, unlike
// the snake_case dialect Hauling and Taxi implement (DispatchExecution, which
// they embed). Before this adapter existed Fleet sent snake_case bodies,
// called an eligibility route Dispatch does not have, and decoded snake_case
// responses Dispatch never sends, so every reservation, eligibility check and
// reconciliation read against Dispatch failed or decoded to zero values. Only
// execution creation and its read-back happened to match.
type DispatchNativeExecution struct {
	client  *Client
	apiBase string
}

// NewDispatchExecution builds the Dispatch adapter over the shared signed HTTP
// client (timeouts, tracing headers, error classification, metrics).
func NewDispatchExecution(client *Client) *DispatchNativeExecution {
	return &DispatchNativeExecution{client: client, apiBase: dispatchInternalBase}
}

// Configured reports whether the dispatch base URL and secret are present.
func (d *DispatchNativeExecution) Configured() bool { return d.client.Configured() }

// ServiceCategory identifies this adapter.
func (d *DispatchNativeExecution) ServiceCategory() string { return CategoryDispatch }

type dispatchNativeReservation struct {
	ReservationID      string `json:"reservationId"`
	RequestID          string `json:"requestId"`
	OrganizationID     string `json:"organizationId"`
	Status             string `json:"status"`
	RequestVersion     int64  `json:"requestVersion"`
	ReservationVersion int64  `json:"reservationVersion"`
}

func (r dispatchNativeReservation) outcome() ReservationOutcome {
	return ReservationOutcome{
		ReservationID:        r.ReservationID,
		Status:               r.Status,
		WinnerOrganizationID: r.OrganizationID,
		SourceVersion:        r.RequestVersion,
	}
}

// ReserveForOrganization asks Dispatch to reserve the request. Idempotency and
// correlation travel as headers (Dispatch reads them from headers only).
func (d *DispatchNativeExecution) ReserveForOrganization(ctx context.Context, meta CallMeta,
	request ReserveRequest) (ReservationOutcome, error) {
	meta.IdempotencyKey = request.IdempotencyKey
	if meta.CorrelationID == "" {
		meta.CorrelationID = request.CorrelationID
	}
	body := map[string]any{
		"requestId":                request.SourceRequestID,
		"expectedRequestVersion":   request.ExpectedSourceVersion,
		"organizationId":           request.OrganizationID,
		"acceptanceSource":         request.AcceptanceSource,
		"acceptingProviderId":      request.AcceptingProviderID,
		"organizationMembershipId": request.OrganizationMembershipID,
	}
	var response dispatchEnvelope[dispatchNativeReservation]
	if err := d.client.Do(ctx, meta, http.MethodPost, d.apiBase+"/reservations", body, &response); err != nil {
		return ReservationOutcome{}, classifyReservationError(err)
	}
	return response.Data.outcome(), nil
}

// ReadReservation resolves an uncertain reservation by the original
// idempotency key — the only identity Fleet holds before Dispatch answered.
func (d *DispatchNativeExecution) ReadReservation(ctx context.Context, meta CallMeta,
	sourceRequestID string, idempotencyKey string) (ReservationOutcome, error) {
	var response dispatchEnvelope[dispatchNativeReservation]
	path := d.apiBase + "/reservations/by-idempotency/" + url.PathEscape(idempotencyKey)
	if err := d.client.Do(ctx, meta, http.MethodGet, path, nil, &response); err != nil {
		return ReservationOutcome{}, err
	}
	if response.Data.RequestID != "" && sourceRequestID != "" && response.Data.RequestID != sourceRequestID {
		return ReservationOutcome{}, &Error{Service: "driver-dispatch-delivery-service",
			Outcome: OutcomeConflict, Message: "reservation belongs to a different request"}
	}
	return response.Data.outcome(), nil
}

// dispatchReleaseReasons is Dispatch's closed release-reason catalogue
// (driver-dispatch-delivery-service marketplace/models.go releaseReasons).
var dispatchReleaseReasons = map[string]bool{
	"ORGANIZATION_CANCELLED": true, "ASSIGNMENT_CANCELLED": true, "ASSIGNMENT_EXPIRED": true,
	"NO_ELIGIBLE_PROVIDER": true, "REQUEST_CANCELLED": true, "PLATFORM_RECOVERY": true,
}

// ReleaseReservation returns a reserved request to the marketplace. Dispatch
// releases by reservation id and requires the reservation's current version,
// so the live reservation is read first and must belong to the organization.
func (d *DispatchNativeExecution) ReleaseReservation(ctx context.Context, meta CallMeta,
	sourceRequestID string, organizationID string, reason string, idempotencyKey string) error {
	var live dispatchEnvelope[dispatchNativeReservation]
	if err := d.client.Do(ctx, meta, http.MethodGet,
		d.apiBase+"/requests/"+url.PathEscape(sourceRequestID)+"/reservation", nil, &live); err != nil {
		return err
	}
	if live.Data.OrganizationID != "" && live.Data.OrganizationID != organizationID {
		return &Error{Service: "driver-dispatch-delivery-service", Outcome: OutcomeConflict,
			Message: "reservation is held by another organization"}
	}
	code := strings.ToUpper(strings.TrimSpace(reason))
	if !dispatchReleaseReasons[code] {
		code = "ORGANIZATION_CANCELLED"
	}
	meta.IdempotencyKey = idempotencyKey
	body := map[string]any{"reasonCode": code, "expectedReservationVersion": live.Data.ReservationVersion}
	return d.client.Do(ctx, meta, http.MethodPost,
		d.apiBase+"/reservations/"+url.PathEscape(live.Data.ReservationID)+"/release", body, nil)
}

type dispatchNativeEligibility struct {
	Eligible       bool     `json:"eligible"`
	Status         string   `json:"status"`
	RequestVersion int64    `json:"requestVersion"`
	Reasons        []string `json:"reasons"`
}

func (e dispatchNativeEligibility) reason() string {
	if len(e.Reasons) > 0 {
		return e.Reasons[0]
	}
	return ""
}

// ValidateProviderEligibility asks Dispatch whether the provider (and, when
// named, the vehicle) may take this request. The asking organization comes
// from meta.OrganizationID: Dispatch refuses to answer for a request another
// organization holds.
func (d *DispatchNativeExecution) ValidateProviderEligibility(ctx context.Context, meta CallMeta,
	sourceRequestID string, providerID string, vehicleReference string) (ProviderEligibility, error) {
	var provider dispatchEnvelope[dispatchNativeEligibility]
	if err := d.client.Do(ctx, meta, http.MethodPost, d.apiBase+"/eligibility/providers/validate",
		map[string]any{
			"requestId":      sourceRequestID,
			"organizationId": meta.OrganizationID,
			"providerId":     providerID,
		}, &provider); err != nil {
		return ProviderEligibility{}, err
	}
	result := ProviderEligibility{
		Eligible:       provider.Data.Eligible,
		PlatformStatus: provider.Data.Status,
		ReasonCode:     provider.Data.reason(),
		VehicleOK:      vehicleReference == "",
		SourceVersion:  provider.Data.RequestVersion,
	}
	if !result.Eligible || vehicleReference == "" {
		return result, nil
	}
	var vehicle dispatchEnvelope[dispatchNativeEligibility]
	if err := d.client.Do(ctx, meta, http.MethodPost, d.apiBase+"/eligibility/vehicles/validate",
		map[string]any{
			"requestId":      sourceRequestID,
			"organizationId": meta.OrganizationID,
			"providerId":     providerID,
			"vehicleId":      vehicleReference,
		}, &vehicle); err != nil {
		return ProviderEligibility{}, err
	}
	result.VehicleOK = vehicle.Data.Eligible
	if !vehicle.Data.Eligible && result.ReasonCode == "" {
		result.ReasonCode = vehicle.Data.reason()
	}
	return result, nil
}

// ConfirmProviderAndCreateExecution creates the canonical organization-managed
// trip. Shared with the snake_case adapter because Dispatch's execution route
// already matched Fleet's camelCase body.
func (d *DispatchNativeExecution) ConfirmProviderAndCreateExecution(ctx context.Context, meta CallMeta,
	request ConfirmProviderRequest) (ExecutionOutcome, error) {
	return (&DispatchExecution{client: d.client, apiBase: d.apiBase, category: CategoryDispatch}).
		ConfirmProviderAndCreateExecution(ctx, meta, request)
}

// ReadExecutionOutcome resolves an uncertain execution creation by read-back.
func (d *DispatchNativeExecution) ReadExecutionOutcome(ctx context.Context, meta CallMeta,
	sourceRequestID string, idempotencyKey string) (ExecutionOutcome, error) {
	return (&DispatchExecution{client: d.client, apiBase: d.apiBase, category: CategoryDispatch}).
		ReadExecutionOutcome(ctx, meta, sourceRequestID, idempotencyKey)
}

type dispatchNativeRequest struct {
	RequestID              string     `json:"requestId"`
	BookingID              string     `json:"bookingId"`
	BroadcastStatus        string     `json:"broadcastStatus"`
	ReservationStatus      string     `json:"reservationStatus"`
	ReservedOrganizationID *string    `json:"reservedOrganizationId"`
	AcceptedProviderID     *string    `json:"acceptedProviderId"`
	SourceVersion          int64      `json:"sourceVersion"`
	ExpiresAt              *time.Time `json:"expiresAt"`
}

// ReadRequest returns Dispatch's authoritative request state.
func (d *DispatchNativeExecution) ReadRequest(ctx context.Context, meta CallMeta,
	sourceRequestID string) (SourceRequestView, error) {
	var response dispatchEnvelope[dispatchNativeRequest]
	if err := d.client.Do(ctx, meta, http.MethodGet,
		d.apiBase+"/requests/"+url.PathEscape(sourceRequestID), nil, &response); err != nil {
		return SourceRequestView{}, err
	}
	view := SourceRequestView{
		RequestID:     response.Data.RequestID,
		BookingID:     response.Data.BookingID,
		Status:        response.Data.BroadcastStatus,
		SourceVersion: response.Data.SourceVersion,
	}
	if response.Data.ReservedOrganizationID != nil {
		view.WinnerOrganizationID = *response.Data.ReservedOrganizationID
	}
	if response.Data.AcceptedProviderID != nil {
		view.AcceptedProviderID = *response.Data.AcceptedProviderID
	}
	if response.Data.ExpiresAt != nil {
		view.ExpiresAt = *response.Data.ExpiresAt
	}
	return view, nil
}

type dispatchNativeTrip struct {
	TripID              string     `json:"tripId"`
	BookingID           string     `json:"bookingId"`
	Status              string     `json:"status"`
	ExecutingProviderID string     `json:"executingProviderId"`
	SourceVersion       int64      `json:"sourceVersion"`
	CompletedAt         *time.Time `json:"completedAt"`
}

// ReadTrip returns Dispatch's authoritative trip state. Dispatch's trip read
// model carries no request id and no cancellation timestamp; a cancelled trip
// is reported by status, which reconciliation maps on its own.
func (d *DispatchNativeExecution) ReadTrip(ctx context.Context, meta CallMeta,
	sourceTripID string) (SourceTripView, error) {
	var response dispatchEnvelope[dispatchNativeTrip]
	if err := d.client.Do(ctx, meta, http.MethodGet,
		d.apiBase+"/trips/"+url.PathEscape(sourceTripID), nil, &response); err != nil {
		return SourceTripView{}, err
	}
	return SourceTripView{
		TripID:              response.Data.TripID,
		BookingID:           response.Data.BookingID,
		Status:              response.Data.Status,
		SourceVersion:       response.Data.SourceVersion,
		ExecutingProviderID: response.Data.ExecutingProviderID,
		CompletedAt:         response.Data.CompletedAt,
	}, nil
}

// CancelExecution cancels an organization-managed canonical trip through
// Dispatch's internal marketplace API:
//
//	POST {base}/trips/{sourceTripId}/cancel
//
// Dispatch checks that the trip belongs to meta.OrganizationID (another
// organization's trip, or an independent one, is not found) and cancels it
// through its own trip state machine. A repeat returns the cancelled trip, so
// a retry after an unknown outcome is safe. A trip past pickup is refused
// with a conflict; Fleet never cancels a live trip locally.
func (d *DispatchNativeExecution) CancelExecution(ctx context.Context, meta CallMeta,
	sourceTripID string, reason string, idempotencyKey string) error {
	meta.IdempotencyKey = idempotencyKey
	body := map[string]any{
		"organizationId": meta.OrganizationID, "reason": reason,
		"idempotencyKey": idempotencyKey, "correlationId": meta.CorrelationID,
	}
	return d.client.Do(ctx, meta, http.MethodPost,
		d.apiBase+"/trips/"+url.PathEscape(sourceTripID)+"/cancel", body, nil)
}

var _ ExecutionContract = (*DispatchNativeExecution)(nil)
