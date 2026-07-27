package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// DispatchExecution is the REAL adapter for driver-dispatch-delivery-service.
//
// It targets the internal, service-authenticated routes added to dispatch by
// this work package (documented in docs/marketplace-service-contracts.md):
//
//	POST /api/v1/internal/marketplace/reservations
//	GET  /api/v1/internal/marketplace/reservations/{sourceRequestId}
//	POST /api/v1/internal/marketplace/reservations/{sourceRequestId}/release
//	GET  /api/v1/internal/marketplace/eligibility
//	POST /api/v1/internal/marketplace/executions
//	GET  /api/v1/internal/marketplace/executions/{sourceRequestId}
//	GET  /api/v1/internal/marketplace/requests/{sourceRequestId}
//	GET  /api/v1/internal/marketplace/trips/{sourceTripId}
//	POST /api/v1/internal/marketplace/trips/{sourceTripId}/cancel
type DispatchExecution struct {
	client *Client
}

// NewDispatchExecution builds the dispatch adapter over the shared signed
// HTTP client (timeouts, tracing headers, error classification, metrics).
func NewDispatchExecution(client *Client) *DispatchExecution {
	return &DispatchExecution{client: client}
}

// Configured reports whether the dispatch base URL and secret are present.
func (d *DispatchExecution) Configured() bool { return d.client.Configured() }

// ServiceCategory identifies this adapter.
func (d *DispatchExecution) ServiceCategory() string { return CategoryDispatch }

const dispatchInternalBase = "/api/v1/internal/marketplace"

type dispatchEnvelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

type dispatchReservationBody struct {
	ReservationID        string `json:"reservation_id"`
	Status               string `json:"status"`
	WinnerOrganizationID string `json:"winner_organization_id"`
	SourceVersion        int64  `json:"source_version"`
}

// ReserveForOrganization asks dispatch to reserve a broadcast for the
// organization. Dispatch decides the single winner atomically; this adapter
// only reports what it decided.
func (d *DispatchExecution) ReserveForOrganization(ctx context.Context, meta CallMeta, request ReserveRequest) (ReservationOutcome, error) {
	body := map[string]any{
		"source_request_id":          request.SourceRequestID,
		"expected_source_version":    request.ExpectedSourceVersion,
		"organization_id":            request.OrganizationID,
		"acceptance_source":          request.AcceptanceSource,
		"accepting_provider_id":      request.AcceptingProviderID,
		"organization_membership_id": request.OrganizationMembershipID,
		"idempotency_key":            request.IdempotencyKey,
	}
	var response dispatchEnvelope[dispatchReservationBody]
	if err := d.client.Do(ctx, meta, http.MethodPost, dispatchInternalBase+"/reservations", body, &response); err != nil {
		return ReservationOutcome{}, classifyReservationError(err)
	}
	return ReservationOutcome{
		ReservationID:        response.Data.ReservationID,
		Status:               response.Data.Status,
		WinnerOrganizationID: response.Data.WinnerOrganizationID,
		SourceVersion:        response.Data.SourceVersion,
	}, nil
}

// classifyReservationError turns a losing race into a definite outcome rather
// than an error, so the caller does not treat "someone else won" as "unknown".
func classifyReservationError(err error) error {
	var clientErr *Error
	if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeConflict {
		return &Error{Service: clientErr.Service, Outcome: OutcomeConflict, Status: clientErr.Status,
			Message: "request already claimed"}
	}
	return err
}

// ReadReservation resolves an uncertain reservation by asking dispatch what
// happened, keyed by the same idempotency key.
func (d *DispatchExecution) ReadReservation(ctx context.Context, meta CallMeta, sourceRequestID string, idempotencyKey string) (ReservationOutcome, error) {
	var response dispatchEnvelope[dispatchReservationBody]
	path := dispatchInternalBase + "/reservations/" + sourceRequestID + "?idempotency_key=" + idempotencyKey
	if err := d.client.Do(ctx, meta, http.MethodGet, path, nil, &response); err != nil {
		return ReservationOutcome{}, err
	}
	return ReservationOutcome{
		ReservationID:        response.Data.ReservationID,
		Status:               response.Data.Status,
		WinnerOrganizationID: response.Data.WinnerOrganizationID,
		SourceVersion:        response.Data.SourceVersion,
	}, nil
}

// ReleaseReservation returns a reserved request to the marketplace.
func (d *DispatchExecution) ReleaseReservation(ctx context.Context, meta CallMeta, sourceRequestID string, organizationID string, reason string, idempotencyKey string) error {
	body := map[string]any{
		"organization_id": organizationID,
		"reason":          reason,
		"idempotency_key": idempotencyKey,
	}
	return d.client.Do(ctx, meta, http.MethodPost,
		dispatchInternalBase+"/reservations/"+sourceRequestID+"/release", body, nil)
}

type dispatchEligibilityBody struct {
	Eligible       bool   `json:"eligible"`
	PlatformStatus string `json:"platform_status"`
	ReasonCode     string `json:"reason_code"`
	VehicleOK      bool   `json:"vehicle_ok"`
	SourceVersion  int64  `json:"source_version"`
}

// ValidateProviderEligibility asks dispatch whether the provider and vehicle
// may take this work. Vehicle verification stays owned by dispatch: this
// service never builds a competing vehicle source of truth.
func (d *DispatchExecution) ValidateProviderEligibility(ctx context.Context, meta CallMeta, sourceRequestID string, providerID string, vehicleReference string) (ProviderEligibility, error) {
	path := dispatchInternalBase + "/eligibility?source_request_id=" + sourceRequestID +
		"&provider_id=" + providerID + "&vehicle_reference=" + vehicleReference
	var response dispatchEnvelope[dispatchEligibilityBody]
	if err := d.client.Do(ctx, meta, http.MethodGet, path, nil, &response); err != nil {
		return ProviderEligibility{}, err
	}
	return ProviderEligibility{
		Eligible:       response.Data.Eligible,
		PlatformStatus: response.Data.PlatformStatus,
		ReasonCode:     response.Data.ReasonCode,
		VehicleOK:      response.Data.VehicleOK,
		SourceVersion:  response.Data.SourceVersion,
	}, nil
}

type dispatchExecutionBody struct {
	Status        string `json:"status"`
	TripID        string `json:"tripId"`
	RequestID     string `json:"requestId"`
	SourceVersion int64  `json:"requestVersion"`
	RejectionCode string `json:"rejectionCode"`
}

// ConfirmProviderAndCreateExecution binds the managed provider and creates the
// canonical dispatch trip. It is idempotent on the supplied key: a repeat
// returns the existing trip rather than creating a second one.
func (d *DispatchExecution) ConfirmProviderAndCreateExecution(ctx context.Context, meta CallMeta, request ConfirmProviderRequest) (ExecutionOutcome, error) {
	body := map[string]any{
		"requestId":                request.SourceRequestID,
		"expectedRequestVersion":   request.ExpectedSourceVersion,
		"reservationId":            request.ReservationID,
		"organizationId":           request.OrganizationID,
		"settlementOrganizationId": request.SettlementOrganizationID,
		"executingProviderId":      request.ExecutingProviderID,
		"organizationMembershipId": request.OrganizationMembershipID,
		"vehicleId":                request.AssetOrVehicleReference,
		"fleetAssignmentId":        request.FleetAssignmentID,
		"providerResponseId":       request.ProviderResponseID,
		"acceptanceSource":         request.AcceptanceSource,
		"executionSource":          "fleet-partner-service",
	}
	meta.IdempotencyKey = request.IdempotencyKey
	var response dispatchEnvelope[dispatchExecutionBody]
	if err := d.client.Do(ctx, meta, http.MethodPost, dispatchInternalBase+"/executions", body, &response); err != nil {
		return ExecutionOutcome{}, err
	}
	return ExecutionOutcome{
		Status:        ExecutionCreatedStatus,
		TripID:        response.Data.TripID,
		RequestID:     response.Data.RequestID,
		SourceVersion: response.Data.SourceVersion,
		RejectionCode: response.Data.RejectionCode,
	}, nil
}

// ReadExecutionOutcome resolves an uncertain execution creation by read-back.
// This is what makes a timed-out trip creation safe: the answer comes from
// dispatch, never from an assumption or a blind retry.
func (d *DispatchExecution) ReadExecutionOutcome(ctx context.Context, meta CallMeta, sourceRequestID string, idempotencyKey string) (ExecutionOutcome, error) {
	path := dispatchInternalBase + "/executions/by-idempotency/" + url.PathEscape(idempotencyKey)
	var response dispatchEnvelope[dispatchExecutionBody]
	if err := d.client.Do(ctx, meta, http.MethodGet, path, nil, &response); err != nil {
		return ExecutionOutcome{}, err
	}
	_ = sourceRequestID
	return ExecutionOutcome{
		Status:        ExecutionAlreadyExists,
		TripID:        response.Data.TripID,
		RequestID:     response.Data.RequestID,
		SourceVersion: response.Data.SourceVersion,
		RejectionCode: response.Data.RejectionCode,
	}, nil
}

type dispatchRequestBody struct {
	RequestID            string `json:"request_id"`
	BookingID            string `json:"booking_id"`
	Status               string `json:"status"`
	SourceVersion        int64  `json:"source_version"`
	WinnerOrganizationID string `json:"winner_organization_id"`
	AcceptedProviderID   string `json:"accepted_provider_id"`
	ExpiresAt            string `json:"expires_at"`
}

// ReadRequest returns the authoritative request state for reconciliation.
func (d *DispatchExecution) ReadRequest(ctx context.Context, meta CallMeta, sourceRequestID string) (SourceRequestView, error) {
	var response dispatchEnvelope[dispatchRequestBody]
	if err := d.client.Do(ctx, meta, http.MethodGet,
		dispatchInternalBase+"/requests/"+sourceRequestID, nil, &response); err != nil {
		return SourceRequestView{}, err
	}
	view := SourceRequestView{
		RequestID:            response.Data.RequestID,
		BookingID:            response.Data.BookingID,
		Status:               response.Data.Status,
		SourceVersion:        response.Data.SourceVersion,
		WinnerOrganizationID: response.Data.WinnerOrganizationID,
		AcceptedProviderID:   response.Data.AcceptedProviderID,
	}
	if response.Data.ExpiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, response.Data.ExpiresAt); err == nil {
			view.ExpiresAt = parsed
		}
	}
	return view, nil
}

type dispatchTripBody struct {
	TripID              string `json:"trip_id"`
	RequestID           string `json:"request_id"`
	BookingID           string `json:"booking_id"`
	Status              string `json:"status"`
	SourceVersion       int64  `json:"source_version"`
	ExecutingProviderID string `json:"executing_provider_id"`
	CompletedAt         string `json:"completed_at"`
	CancelledAt         string `json:"cancelled_at"`
}

// ReadTrip returns the authoritative trip state for reconciliation.
func (d *DispatchExecution) ReadTrip(ctx context.Context, meta CallMeta, sourceTripID string) (SourceTripView, error) {
	var response dispatchEnvelope[dispatchTripBody]
	if err := d.client.Do(ctx, meta, http.MethodGet,
		dispatchInternalBase+"/trips/"+sourceTripID, nil, &response); err != nil {
		return SourceTripView{}, err
	}
	view := SourceTripView{
		TripID:              response.Data.TripID,
		RequestID:           response.Data.RequestID,
		BookingID:           response.Data.BookingID,
		Status:              response.Data.Status,
		SourceVersion:       response.Data.SourceVersion,
		ExecutingProviderID: response.Data.ExecutingProviderID,
	}
	if response.Data.CompletedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, response.Data.CompletedAt); err == nil {
			view.CompletedAt = &parsed
		}
	}
	if response.Data.CancelledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, response.Data.CancelledAt); err == nil {
			view.CancelledAt = &parsed
		}
	}
	return view, nil
}

// CancelExecution cancels a canonical trip through dispatch's own contract.
// A live trip is never cancelled locally without source confirmation.
func (d *DispatchExecution) CancelExecution(ctx context.Context, meta CallMeta, sourceTripID string, reason string, idempotencyKey string) error {
	body := map[string]any{"reason": reason, "idempotency_key": idempotencyKey}
	return d.client.Do(ctx, meta, http.MethodPost,
		dispatchInternalBase+"/trips/"+sourceTripID+"/cancel", body, nil)
}
