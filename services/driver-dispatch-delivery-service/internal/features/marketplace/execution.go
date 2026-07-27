package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ExecutionInput is fleet's request to create the canonical trip.
type ExecutionInput struct {
	RequestID                string
	ExpectedRequestVersion   int64
	ReservationID            string
	OrganizationID           string
	SettlementOrganizationID string
	ExecutingProviderID      string
	OrganizationMembershipID string
	FleetAssignmentID        string
	ProviderResponseID       string
	AcceptanceSource         string
	VehicleID                string
	ExecutionSource          string
	IdempotencyKey           string
	CorrelationID            string
	RequestIDHeader          string
}

type executionFingerprint struct {
	RequestID           string `json:"requestId"`
	ReservationID       string `json:"reservationId"`
	OrganizationID      string `json:"organizationId"`
	ExecutingProviderID string `json:"executingProviderId"`
	FleetAssignmentID   string `json:"fleetAssignmentId"`
	ProviderResponseID  string `json:"providerResponseId"`
}

// ExecutionResult is the stable response.
//
// It deliberately carries no fare, customer payment, settlement, margin or
// commission field — fleet does not need them and providers must never see them.
type ExecutionResult struct {
	ExecutionID    string    `json:"executionId"`
	TripID         string    `json:"tripId"`
	RequestID      string    `json:"requestId"`
	BroadcastID    string    `json:"broadcastId"`
	BookingID      string    `json:"bookingId"`
	Status         string    `json:"status"`
	RequestVersion int64     `json:"requestVersion"`
	TripVersion    int64     `json:"tripVersion"`
	CreatedAt      time.Time `json:"createdAt"`
}

// CreateExecution creates exactly one canonical trip for an accepted assignment.
//
// The provider-acceptance check is the reason this method exists in this shape.
// Dispatch cannot know locally whether a provider accepted: that record lives in
// fleet. So acceptance is verified over HTTP *before* the transaction opens, and
// the transaction stays short and lock-free of any network call.
func (s *Service) CreateExecution(ctx context.Context, input ExecutionInput) (*ExecutionResult, error) {
	if !s.flags.ExecutionEnabled {
		return nil, errCoded(503, "source_service_unavailable",
			"Organization-managed execution is not enabled.")
	}
	if input.IdempotencyKey == "" {
		return nil, errCoded(400, "validation_error", "Idempotency-Key header is required.")
	}
	if input.FleetAssignmentID == "" || input.ProviderResponseID == "" ||
		input.ExecutingProviderID == "" || input.ReservationID == "" {
		return nil, errCoded(400, "validation_error",
			"reservationId, fleetAssignmentId, providerResponseId and executingProviderId are required.")
	}
	// The organization that settles must be the organization that reserved.
	// Allowing these to diverge here would let one organization's claim be
	// billed to another.
	if input.SettlementOrganizationID != "" &&
		input.SettlementOrganizationID != input.OrganizationID {
		return nil, errCoded(409, "provider_acceptance_mismatch",
			"The settlement organization must match the reserving organization.")
	}

	fingerprint := Fingerprint(executionFingerprint{
		RequestID:           input.RequestID,
		ReservationID:       input.ReservationID,
		OrganizationID:      input.OrganizationID,
		ExecutingProviderID: input.ExecutingProviderID,
		FleetAssignmentID:   input.FleetAssignmentID,
		ProviderResponseID:  input.ProviderResponseID,
	})

	// Step 1: replay check in its own short transaction, so a duplicate never
	// reaches the network call below.
	if replay, err := s.replayExecution(ctx, input, fingerprint); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	// Step 2: verify provider acceptance with fleet, OUTSIDE any transaction.
	// Holding a row lock across an HTTP call would tie a database transaction
	// to another service's availability.
	acceptance, err := s.verifyAcceptance(ctx, input)
	if err != nil {
		return nil, err
	}

	// Step 3: short transaction that creates the trip.
	var result *ExecutionResult
	err = s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		op := &CommandOperation{
			OperationType:      CommandCreateExecution,
			RequestID:          &input.RequestID,
			FleetAssignmentID:  &input.FleetAssignmentID,
			IdempotencyKey:     input.IdempotencyKey,
			RequestFingerprint: fingerprint,
		}
		stored, replayed, err := s.repo.BeginCommand(ctx, tx, op)
		if err != nil {
			if errors.Is(err, ErrIdempotencyConflict) {
				return errIdempotencyConflict()
			}
			return err
		}
		if replayed && stored.Status == CommandConfirmed {
			return decodeExecutionSnapshot(stored, &result)
		}

		broadcast, err := s.repo.LockBroadcast(ctx, tx, input.RequestID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return errCoded(404, "request_not_found", "Request not found.")
			}
			return err
		}
		if broadcast.Status == "cancelled" {
			return errCoded(409, "request_cancelled", "This request has been cancelled.")
		}
		if input.ExpectedRequestVersion != 0 && input.ExpectedRequestVersion != broadcast.SourceVersion {
			return errCoded(409, "source_version_mismatch",
				"This request has changed. Reload and try again.")
		}

		reservation, err := s.repo.LockReservation(ctx, tx, input.ReservationID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return errCoded(404, "reservation_not_found", "Reservation not found.")
			}
			return err
		}
		if reservation.OrganizationID != input.OrganizationID ||
			reservation.RequestID != input.RequestID {
			return errCoded(409, "reservation_conflict",
				"The reservation does not match this organization and request.")
		}
		if reservation.Status != ReservationReserved {
			return errCoded(409, "reservation_conflict", "The reservation is no longer active.")
		}
		// The verified acceptance must agree with the reservation holder.
		if acceptance.OrganizationID != reservation.OrganizationID {
			return errCoded(409, "provider_acceptance_mismatch",
				"The provider acceptance belongs to another organization.")
		}

		// Revalidate the provider against local authoritative state; the
		// eligibility answer fleet saw earlier may now be stale.
		provider, err := s.repo.GetProvider(ctx, tx, input.ExecutingProviderID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return errCoded(409, "provider_not_found", "Provider not found.")
			}
			return err
		}
		if !provider.IsActive {
			return errCoded(409, "provider_not_eligible", "Provider is not eligible.")
		}

		existing, err := s.repo.GetTripByAssignment(ctx, tx, input.FleetAssignmentID)
		if err == nil {
			// A trip already exists for this assignment: return it rather than
			// creating a second one.
			result = tripToResult(existing, broadcast)
			return s.repo.CompleteCommand(ctx, tx, stored.ID, CommandConfirmed, result, "", &existing.ID)
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		trip, err := s.repo.CreateOrganizationTrip(ctx, tx, broadcast, input, acceptance)
		if err != nil {
			if errors.Is(err, ErrExecutionExists) {
				return errCoded(409, "execution_already_exists",
					"A trip already exists for this assignment.")
			}
			return err
		}

		newVersion, err := s.repo.SetBroadcastReservation(ctx, tx, broadcast.ID,
			BroadcastReservationExecuted, &input.OrganizationID, broadcast.SourceVersion)
		if err != nil {
			return err
		}
		broadcast.SourceVersion = newVersion

		if s.flags.OutboxEnabled {
			if err := s.repo.PublishEvent(ctx, tx, "trip.created.v1", "TRIP", trip.ID, 1,
				input.CorrelationID, input.RequestIDHeader, map[string]any{
					"sourceService":            "driver-dispatch-delivery-service",
					"requestId":                broadcast.ID,
					"broadcastId":              broadcast.ID,
					"bookingId":                broadcast.BookingID,
					"tripId":                   trip.ID,
					"sourceVersion":            int64(1),
					"organizationId":           input.OrganizationID,
					"settlementOrganizationId": input.OrganizationID,
					"executingProviderId":      input.ExecutingProviderID,
					"organizationMembershipId": input.OrganizationMembershipID,
					"fleetAssignmentId":        input.FleetAssignmentID,
					"status":                   trip.Status,
					"occurredAt":               s.now(),
				}); err != nil {
				return err
			}
		}

		result = tripToResult(trip, broadcast)
		return s.repo.CompleteCommand(ctx, tx, stored.ID, CommandConfirmed, result, "", &trip.ID)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// replayExecution answers a duplicate submission from the stored snapshot.
func (s *Service) replayExecution(ctx context.Context, input ExecutionInput, fingerprint string) (*ExecutionResult, error) {
	stored, err := s.repo.GetCommand(ctx, nil, CommandCreateExecution, input.IdempotencyKey)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stored.RequestFingerprint != fingerprint {
		return nil, errIdempotencyConflict()
	}
	if stored.Status != CommandConfirmed {
		return nil, nil
	}
	var result *ExecutionResult
	if err := decodeExecutionSnapshot(stored, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// verifyAcceptance asks fleet whether the provider really accepted.
//
// This is the check that makes "no execution before provider acceptance" real.
// The database CHECK constraint only proves a provider_response_id was
// supplied; only fleet can say whether a provider actually said yes.
func (s *Service) verifyAcceptance(ctx context.Context, input ExecutionInput) (*ProviderAcceptance, error) {
	if !s.verifier.Configured() {
		// Fail closed: an unconfigured verifier must never be read as consent.
		return nil, errCoded(503, "fleet_service_unavailable",
			"Provider acceptance cannot be verified right now.")
	}

	acceptance, err := s.verifier.VerifyProviderAcceptance(ctx,
		CallMeta{RequestID: input.RequestIDHeader, CorrelationID: input.CorrelationID},
		input.FleetAssignmentID)
	if err != nil {
		switch {
		case errors.Is(err, ErrFleetAssignmentNotFound):
			return nil, errCoded(409, "fleet_assignment_not_found", "The assignment was not found.")
		case errors.Is(err, ErrAcceptanceNotConfirmed):
			return nil, errCoded(409, "provider_acceptance_not_confirmed",
				"The provider has not accepted this assignment.")
		case errors.Is(err, ErrAcceptanceMismatch):
			return nil, errCoded(409, "provider_acceptance_mismatch",
				"The provider acceptance does not match this request.")
		case errors.Is(err, ErrAcceptanceCancelled):
			// Permanent. Reporting this as a temporary failure would make the
			// fleet execution-start worker retry a cancelled assignment until
			// it exhausted its budget instead of stopping immediately.
			return nil, errCoded(409, "provider_acceptance_cancelled",
				"This assignment was cancelled before execution started.")
		case errors.Is(err, ErrAcceptanceExpired):
			// Also permanent: an expired assignment never becomes executable.
			return nil, errCoded(409, "provider_acceptance_expired",
				"This assignment expired before execution started.")
		default:
			// An unreachable fleet is an unknown outcome, not a rejection.
			return nil, errCoded(503, "fleet_service_unavailable",
				"Provider acceptance cannot be verified right now.")
		}
	}

	// Every binding reference must match. A mismatch means the acceptance
	// belongs to different work, and acting on it would execute a trip nobody
	// agreed to.
	if acceptance.ResponseStatus != "ACCEPTED" {
		return nil, errCoded(409, "provider_acceptance_not_confirmed",
			"The provider has not accepted this assignment.")
	}
	switch acceptance.AssignmentStatus {
	case "ACCEPTED", "EXECUTION_START_PENDING", "READY_FOR_EXECUTION":
	default:
		return nil, errCoded(409, "provider_acceptance_not_confirmed",
			"The assignment is not ready for execution.")
	}
	if acceptance.ProviderID != input.ExecutingProviderID {
		return nil, errCoded(409, "provider_acceptance_mismatch",
			"The accepting provider is not the executing provider.")
	}
	if acceptance.ProviderResponseID != input.ProviderResponseID {
		return nil, errCoded(409, "provider_acceptance_mismatch",
			"The provider response does not match this command.")
	}
	if input.OrganizationMembershipID != "" &&
		acceptance.OrganizationMembershipID != input.OrganizationMembershipID {
		return nil, errCoded(409, "provider_acceptance_mismatch",
			"The organization membership does not match.")
	}
	if acceptance.SourceRequestID != "" && acceptance.SourceRequestID != input.RequestID {
		return nil, errCoded(409, "provider_acceptance_mismatch",
			"The assignment refers to a different request.")
	}
	return acceptance, nil
}

func decodeExecutionSnapshot(stored *CommandOperation, out **ExecutionResult) error {
	if len(stored.ResponseSnapshot) == 0 {
		return errCoded(409, "execution_outcome_unknown",
			"The previous execution outcome is not yet resolved.")
	}
	var decoded ExecutionResult
	if err := json.Unmarshal(stored.ResponseSnapshot, &decoded); err != nil {
		return fmt.Errorf("marketplace: decode execution snapshot: %w", err)
	}
	*out = &decoded
	return nil
}

func tripToResult(trip *TripRecord, broadcast *Broadcast) *ExecutionResult {
	result := &ExecutionResult{
		ExecutionID: trip.ID,
		TripID:      trip.ID,
		Status:      trip.Status,
		TripVersion: trip.SourceVersion,
		CreatedAt:   trip.CreatedAt,
	}
	if broadcast != nil {
		result.RequestID = broadcast.ID
		result.BroadcastID = broadcast.ID
		result.BookingID = broadcast.BookingID
		result.RequestVersion = broadcast.SourceVersion
	}
	return result
}

// ReadExecutionByIdempotencyKey resolves an unknown execution outcome.
func (s *Service) ReadExecutionByIdempotencyKey(ctx context.Context, key string) (*ExecutionResult, error) {
	stored, err := s.repo.GetCommand(ctx, nil, CommandCreateExecution, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "execution_not_found", "No execution for this key.")
		}
		return nil, err
	}
	var result *ExecutionResult
	if err := decodeExecutionSnapshot(stored, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ReadExecutionByAssignment resolves an unknown outcome by fleet assignment.
func (s *Service) ReadExecutionByAssignment(ctx context.Context, assignmentID string) (*ExecutionResult, error) {
	trip, err := s.repo.GetTripByAssignment(ctx, nil, assignmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "execution_not_found", "No execution for this assignment.")
		}
		return nil, err
	}
	broadcast, _ := s.repo.GetBroadcastByBooking(ctx, nil, trip.BookingID)
	return tripToResult(trip, broadcast), nil
}

// ReadTrip returns a canonical trip projection view.
func (s *Service) ReadTrip(ctx context.Context, tripID string) (*TripView, error) {
	trip, err := s.repo.GetTrip(ctx, nil, tripID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "execution_not_found", "Trip not found.")
		}
		return nil, err
	}
	return tripToView(trip), nil
}

// TripView is the commercial-free canonical trip read model.
type TripView struct {
	TripID                   string     `json:"tripId"`
	BookingID                string     `json:"bookingId"`
	Status                   string     `json:"status"`
	ExecutionMode            string     `json:"executionMode"`
	ExecutingProviderID      string     `json:"executingProviderId"`
	OrganizationID           *string    `json:"organizationId"`
	SettlementOrganizationID *string    `json:"settlementOrganizationId"`
	OrganizationMembershipID *string    `json:"organizationMembershipId"`
	FleetAssignmentID        *string    `json:"fleetAssignmentId"`
	ProviderResponseID       *string    `json:"providerResponseId"`
	AcceptanceSource         *string    `json:"acceptanceSource"`
	SourceVersion            int64      `json:"sourceVersion"`
	CompletedAt              *time.Time `json:"completedAt"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

func tripToView(trip *TripRecord) *TripView {
	return &TripView{
		TripID:                   trip.ID,
		BookingID:                trip.BookingID,
		Status:                   trip.Status,
		ExecutionMode:            trip.ExecutionMode,
		ExecutingProviderID:      trip.ProviderID,
		OrganizationID:           trip.OrganizationID,
		SettlementOrganizationID: trip.SettlementOrganizationID,
		OrganizationMembershipID: trip.OrganizationMembershipID,
		FleetAssignmentID:        trip.FleetAssignmentID,
		ProviderResponseID:       trip.ProviderResponseID,
		AcceptanceSource:         trip.AcceptanceSource,
		SourceVersion:            trip.SourceVersion,
		CompletedAt:              trip.CompletedAt,
		CreatedAt:                trip.CreatedAt,
		UpdatedAt:                trip.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// Trip repository access
// ---------------------------------------------------------------------------

// TripRecord is the slice of the canonical trip this feature reads.
//
// fare_amount is deliberately NOT selected. A field that is never loaded
// cannot be leaked by a later refactor.
type TripRecord struct {
	ID                       string
	BookingID                string
	ProviderID               string
	Status                   string
	ExecutionMode            string
	OrganizationID           *string
	SettlementOrganizationID *string
	OrganizationMembershipID *string
	FleetAssignmentID        *string
	ProviderResponseID       *string
	AcceptanceSource         *string
	SourceVersion            int64
	CompletedAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

const tripColumns = `id, booking_id, provider_id, status, execution_mode, organization_id,
	settlement_organization_id, organization_membership_id, fleet_assignment_id,
	provider_response_id, acceptance_source, source_version, completed_at, created_at, updated_at`

func scanTrip(row pgx.Row) (*TripRecord, error) {
	var t TripRecord
	err := row.Scan(&t.ID, &t.BookingID, &t.ProviderID, &t.Status, &t.ExecutionMode,
		&t.OrganizationID, &t.SettlementOrganizationID, &t.OrganizationMembershipID,
		&t.FleetAssignmentID, &t.ProviderResponseID, &t.AcceptanceSource, &t.SourceVersion,
		&t.CompletedAt, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan trip: %w", err)
	}
	return &t, nil
}

// GetTrip reads a canonical trip.
func (r *Repository) GetTrip(ctx context.Context, q Querier, tripID string) (*TripRecord, error) {
	if q == nil {
		q = r.db
	}
	return scanTrip(q.QueryRow(ctx, `SELECT `+tripColumns+` FROM trips WHERE id = $1`, tripID))
}

// GetTripByAssignment finds the trip created for a fleet assignment.
func (r *Repository) GetTripByAssignment(ctx context.Context, q Querier, assignmentID string) (*TripRecord, error) {
	if q == nil {
		q = r.db
	}
	return scanTrip(q.QueryRow(ctx,
		`SELECT `+tripColumns+` FROM trips WHERE fleet_assignment_id = $1`, assignmentID))
}

// GetBroadcastByBooking finds the canonical request behind a booking.
func (r *Repository) GetBroadcastByBooking(ctx context.Context, q Querier, bookingID string) (*Broadcast, error) {
	if q == nil {
		q = r.db
	}
	return scanBroadcast(q.QueryRow(ctx,
		`SELECT `+broadcastColumns+` FROM request_broadcasts WHERE booking_id = $1`, bookingID))
}

// CreateOrganizationTrip inserts the canonical organization-managed trip.
//
// The trip is built from the broadcast's stored booking payload, so the
// customer-facing details stay authoritative and dispatch-owned. The unique
// index on fleet_assignment_id is what prevents a duplicate after a lost
// response.
func (r *Repository) CreateOrganizationTrip(ctx context.Context, q Querier, broadcast *Broadcast,
	input ExecutionInput, acceptance *ProviderAcceptance) (*TripRecord, error) {
	settlement := input.SettlementOrganizationID
	if settlement == "" {
		settlement = input.OrganizationID
	}
	row := q.QueryRow(ctx, `
		INSERT INTO trips (
			booking_id, provider_id, customer_id, status,
			pickup_address, pickup_lat, pickup_lng,
			dropoff_address, dropoff_lat, dropoff_lng,
			distance_km, fare_amount, currency, receiver_name, receiver_phone,
			execution_mode, execution_source, organization_id, settlement_organization_id,
			organization_membership_id, fleet_assignment_id, provider_response_id,
			acceptance_source, fleet_correlation_id, source_version)
		SELECT
			b.booking_id,
			$2::uuid,
			COALESCE((b.booking_payload->>'customerId')::uuid, gen_random_uuid()),
			'assigned',
			COALESCE(b.booking_payload->>'pickupAddress', ''),
			COALESCE((b.booking_payload->>'pickupLat')::numeric, 0),
			COALESCE((b.booking_payload->>'pickupLng')::numeric, 0),
			COALESCE(b.booking_payload->>'dropoffAddress', ''),
			COALESCE((b.booking_payload->>'dropoffLat')::numeric, 0),
			COALESCE((b.booking_payload->>'dropoffLng')::numeric, 0),
			COALESCE((b.booking_payload->>'distanceKm')::numeric, 0),
			COALESCE((b.booking_payload->>'fareAmount')::bigint, 0),
			COALESCE(b.booking_payload->>'currency', 'NGN'),
			COALESCE(b.booking_payload->>'receiverName', ''),
			COALESCE(b.booking_payload->>'receiverPhone', ''),
			'ORGANIZATION_MANAGED', $3, $4, $5, $6, $7, $8, $9, $10, 1
		FROM request_broadcasts b
		WHERE b.id = $1
		RETURNING `+tripColumns,
		broadcast.ID, input.ExecutingProviderID, input.ExecutionSource, input.OrganizationID,
		settlement, input.OrganizationMembershipID, input.FleetAssignmentID,
		input.ProviderResponseID, input.AcceptanceSource, input.CorrelationID)

	trip, err := scanTrip(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrExecutionExists
		}
		return nil, err
	}
	return trip, nil
}
