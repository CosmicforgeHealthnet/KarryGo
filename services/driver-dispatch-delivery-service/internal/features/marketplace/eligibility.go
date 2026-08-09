package marketplace

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Structured ineligibility reason codes. These are part of the cross-service
// contract: fleet maps them onto its own provider-facing messages, so they
// must stay stable.
const (
	ReasonProviderNotFound        = "provider_not_found"
	ReasonProviderNotEligible     = "provider_not_eligible"
	ReasonProviderNotAvailable    = "provider_not_available"
	ReasonProviderSuspended       = "provider_suspended"
	ReasonProviderVerificationBad = "provider_verification_invalid"
	ReasonProviderConflictingWork = "provider_has_conflicting_work"
	ReasonProviderServiceMismatch = "provider_service_mismatch"
	ReasonRequestNotFound         = "request_not_found"
	ReasonRequestNotAssignable    = "request_not_assignable"
	ReasonReservationConflict     = "reservation_conflict"
	ReasonSourceVersionMismatch   = "source_version_mismatch"
	ReasonVehicleNotFound         = "vehicle_not_found"
	ReasonVehicleNotEligible      = "vehicle_not_eligible"
	ReasonVehicleSafetyBlocked    = "vehicle_safety_blocked"
	ReasonVehicleConflict         = "vehicle_conflict"
	ReasonAuthorityRestricted     = "authority_restricted"
)

// EligibilityStatus values.
const (
	EligibilityNotRequired = "NOT_REQUIRED"
	EligibilityChecked     = "CHECKED"
)

// providerRecord is the slice of the authoritative provider row we need.
type providerRecord struct {
	ID                 string
	VerificationStatus string
	IsActive           bool
	OnboardingComplete bool
	OperationType      *string
}

// vehicleRecord is the slice of the authoritative vehicle row we need.
type vehicleRecord struct {
	ID                 string
	ProviderID         string
	VerificationStatus string
	IsActive           bool
	BikeType           string
}

// ProviderEligibilityInput is a fleet eligibility question.
type ProviderEligibilityInput struct {
	RequestID                string
	OrganizationID           string
	ProviderID               string
	OrganizationMembershipID string
	ExpectedRequestVersion   int64
	// RequireAvailability is used by private execution creation. Marketplace
	// eligibility reads historically answered structural authority only; a new
	// private trip must additionally prove the provider is online and verified
	// to work at the commit point.
	RequireAvailability bool
	// SkipReservationCheck is set by the DELEGATED path only.
	//
	// The centralized path requires a live organization reservation to already
	// exist — a dispatcher claimed the request before assigning anyone. In the
	// delegated path the provider's acceptance is what CREATES the
	// reservation, in the same transaction, so demanding one first would make
	// every provider-direct acceptance impossible.
	SkipReservationCheck bool
}

// EligibilityResult is the stable answer.
type EligibilityResult struct {
	Eligible       bool     `json:"eligible"`
	ProviderID     string   `json:"providerId,omitempty"`
	VehicleID      string   `json:"vehicleId,omitempty"`
	Status         string   `json:"status,omitempty"`
	RequestVersion int64    `json:"requestVersion"`
	Reasons        []string `json:"reasons"`
}

// ineligibleResult builds a negative answer carrying structured reasons.
//
// Reasons are codes, never free text and never provider PII: fleet needs to
// know *that* a provider is ineligible and roughly why, not the contents of
// their verification file.
func ineligibleResult(requestVersion int64, reasons ...string) *EligibilityResult {
	return &EligibilityResult{Eligible: false, RequestVersion: requestVersion, Reasons: reasons}
}

// ValidateProvider answers whether a managed provider may take a request.
//
// Every check reads authoritative dispatch state. Organization sponsorship
// never bypasses platform verification, suspension or conflicting-work rules —
// those protect the customer and the provider, not the organization.
func (s *Service) ValidateProvider(ctx context.Context, input ProviderEligibilityInput) (*EligibilityResult, error) {
	if !s.flags.ProviderEligibilityEnabled {
		return nil, errCoded(503, "source_service_unavailable",
			"Provider eligibility checking is not enabled.")
	}
	if input.ProviderID == "" || input.RequestID == "" || input.OrganizationID == "" {
		return nil, errCoded(400, "validation_error",
			"requestId, organizationId and providerId are required.")
	}
	if s.restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", input.OrganizationID}, {"PROVIDER", input.ProviderID}} {
			restricted, err := s.restrictions.Restricted(ctx, input.OrganizationID, subject[0], subject[1])
			if err != nil {
				return nil, fmt.Errorf("marketplace: restriction authority: %w", err)
			}
			if restricted {
				return ineligibleResult(0, ReasonAuthorityRestricted), nil
			}
		}
	}

	broadcast, err := s.repo.GetBroadcast(ctx, nil, input.RequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ineligibleResult(0, ReasonRequestNotFound), nil
		}
		return nil, err
	}
	if input.ExpectedRequestVersion != 0 && input.ExpectedRequestVersion != broadcast.SourceVersion {
		return ineligibleResult(broadcast.SourceVersion, ReasonSourceVersionMismatch), nil
	}
	if broadcast.Status == "cancelled" || broadcast.Status == "expired" {
		return ineligibleResult(broadcast.SourceVersion, ReasonRequestNotAssignable), nil
	}

	// The reservation must still belong to the asking organization; otherwise
	// this organization has no standing to assign anyone to this request.
	if !input.SkipReservationCheck {
		reservation, err := s.repo.GetLiveReservationForRequest(ctx, nil, input.RequestID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ineligibleResult(broadcast.SourceVersion, ReasonReservationConflict), nil
			}
			return nil, err
		}
		if reservation.OrganizationID != input.OrganizationID {
			return ineligibleResult(broadcast.SourceVersion, ReasonReservationConflict), nil
		}
	}

	provider, err := s.repo.GetProvider(ctx, nil, input.ProviderID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ineligibleResult(broadcast.SourceVersion, ReasonProviderNotFound), nil
		}
		return nil, err
	}

	var reasons []string
	if !provider.IsActive {
		reasons = append(reasons, ReasonProviderSuspended)
	}
	if provider.VerificationStatus != "verified" && provider.VerificationStatus != "fully_approved" {
		reasons = append(reasons, ReasonProviderVerificationBad)
	}
	if !provider.OnboardingComplete {
		reasons = append(reasons, ReasonProviderNotEligible)
	}
	if input.RequireAvailability {
		available, availabilityErr := s.repo.ProviderIsAvailable(ctx, nil, input.ProviderID)
		if availabilityErr != nil {
			return nil, availabilityErr
		}
		if !available {
			reasons = append(reasons, ReasonProviderNotAvailable)
		}
	}

	// A provider already on a live canonical trip cannot take another.
	conflicting, err := s.repo.ProviderHasLiveTrip(ctx, nil, input.ProviderID)
	if err != nil {
		return nil, err
	}
	if conflicting {
		reasons = append(reasons, ReasonProviderConflictingWork)
	}

	if len(reasons) > 0 {
		return ineligibleResult(broadcast.SourceVersion, reasons...), nil
	}
	return &EligibilityResult{
		Eligible: true, ProviderID: provider.ID, Status: EligibilityChecked,
		RequestVersion: broadcast.SourceVersion, Reasons: []string{},
	}, nil
}

// VehicleEligibilityInput is a fleet vehicle question.
type VehicleEligibilityInput struct {
	RequestID              string
	OrganizationID         string
	ProviderID             string
	VehicleID              string
	ExpectedRequestVersion int64
}

// ValidateVehicle answers whether a vehicle may be used for a request.
//
// Dispatch delivery does not require a named vehicle for every request. When
// none is supplied the honest answer is NOT_REQUIRED — inventing a vehicle
// reference to satisfy the contract would put a fictional asset on a real trip.
func (s *Service) ValidateVehicle(ctx context.Context, input VehicleEligibilityInput) (*EligibilityResult, error) {
	if !s.flags.VehicleEligibilityEnabled {
		return nil, errCoded(503, "source_service_unavailable",
			"Vehicle eligibility checking is not enabled.")
	}
	if s.restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", input.OrganizationID}, {"PROVIDER", input.ProviderID}, {"ASSET", input.VehicleID}} {
			if subject[1] == "" {
				continue
			}
			restricted, err := s.restrictions.Restricted(ctx, input.OrganizationID, subject[0], subject[1])
			if err != nil {
				return nil, fmt.Errorf("marketplace: restriction authority: %w", err)
			}
			if restricted {
				return ineligibleResult(0, ReasonAuthorityRestricted), nil
			}
		}
	}

	broadcast, err := s.repo.GetBroadcast(ctx, nil, input.RequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ineligibleResult(0, ReasonRequestNotFound), nil
		}
		return nil, err
	}

	if input.VehicleID == "" {
		return &EligibilityResult{
			Eligible: true, Status: EligibilityNotRequired,
			RequestVersion: broadcast.SourceVersion, Reasons: []string{},
		}, nil
	}

	vehicle, err := s.repo.GetVehicle(ctx, nil, input.VehicleID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ineligibleResult(broadcast.SourceVersion, ReasonVehicleNotFound), nil
		}
		return nil, err
	}

	var reasons []string
	// The vehicle must belong to the provider who will drive it.
	if input.ProviderID != "" && vehicle.ProviderID != input.ProviderID {
		reasons = append(reasons, ReasonVehicleNotEligible)
	}
	if !vehicle.IsActive {
		reasons = append(reasons, ReasonVehicleSafetyBlocked)
	}
	if vehicle.VerificationStatus != "verified" && vehicle.VerificationStatus != "approved" {
		reasons = append(reasons, ReasonVehicleNotEligible)
	}

	if len(reasons) > 0 {
		return ineligibleResult(broadcast.SourceVersion, reasons...), nil
	}
	return &EligibilityResult{
		Eligible: true, VehicleID: vehicle.ID, ProviderID: vehicle.ProviderID,
		Status: EligibilityChecked, RequestVersion: broadcast.SourceVersion, Reasons: []string{},
	}, nil
}

// ---------------------------------------------------------------------------
// Repository reads used by eligibility
// ---------------------------------------------------------------------------

// GetProvider reads the authoritative provider row.
func (r *Repository) GetProvider(ctx context.Context, q Querier, providerID string) (*providerRecord, error) {
	if q == nil {
		q = r.db
	}
	var record providerRecord
	err := q.QueryRow(ctx, `
		SELECT id, verification_status, is_active, onboarding_complete, operation_type
		FROM providers WHERE id = $1`, providerID).
		Scan(&record.ID, &record.VerificationStatus, &record.IsActive,
			&record.OnboardingComplete, &record.OperationType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: get provider: %w", err)
	}
	return &record, nil
}

// GetVehicle reads the authoritative vehicle row.
func (r *Repository) GetVehicle(ctx context.Context, q Querier, vehicleID string) (*vehicleRecord, error) {
	if q == nil {
		q = r.db
	}
	var record vehicleRecord
	err := q.QueryRow(ctx, `
		SELECT id, provider_id, verification_status, is_active, bike_type
		FROM bikes WHERE id = $1`, vehicleID).
		Scan(&record.ID, &record.ProviderID, &record.VerificationStatus,
			&record.IsActive, &record.BikeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: get vehicle: %w", err)
	}
	return &record, nil
}

// ProviderIsAvailable reads the durable availability authority. A missing row
// is deliberately unavailable; private execution never infers availability.
func (r *Repository) ProviderIsAvailable(ctx context.Context, q Querier, providerID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var available bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM provider_availability
		WHERE provider_id=$1 AND status='online' AND verified_to_go_online=true)`, providerID).Scan(&available)
	if err != nil {
		return false, fmt.Errorf("marketplace: get provider availability: %w", err)
	}
	return available, nil
}

// ProviderHasLiveTrip reports whether the provider is already on a trip.
func (r *Repository) ProviderHasLiveTrip(ctx context.Context, q Querier, providerID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM trips
			WHERE provider_id = $1 AND status NOT IN ('completed','cancelled','failed')
		)`, providerID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("marketplace: provider live trip: %w", err)
	}
	return exists, nil
}
