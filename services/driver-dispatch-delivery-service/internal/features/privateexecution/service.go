package privateexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

type canonicalCanceller interface {
	CancelOrganizationTrip(context.Context, string, string, string) (*tripfeature.Trip, error)
}

// executionAuthority is the existing Dispatch marketplace use-case surface
// used by the private Fleet contract. Keeping this as a port makes the boundary
// testable without teaching the private handler how to manipulate Dispatch
// tables or duplicating the canonical trip lifecycle.
type executionAuthority interface {
	Reserve(context.Context, marketplace.ReserveInput) (*marketplace.ReservationResult, error)
	ValidateProvider(context.Context, marketplace.ProviderEligibilityInput) (*marketplace.EligibilityResult, error)
	ValidateVehicle(context.Context, marketplace.VehicleEligibilityInput) (*marketplace.EligibilityResult, error)
	CreateExecution(context.Context, marketplace.ExecutionInput) (*marketplace.ExecutionResult, error)
	ReadExecutionByIdempotencyKey(context.Context, string) (*marketplace.ExecutionResult, error)
	ReadTrip(context.Context, string) (*marketplace.TripView, error)
}

type Service struct {
	repo       *repository
	market     executionAuthority
	canceller  canonicalCanceller
	enabled    bool
	killSwitch bool
}

func NewService(repo *repository, market executionAuthority, canceller canonicalCanceller,
	enabled, killSwitch bool) *Service {
	return &Service{repo: repo, market: market, canceller: canceller,
		enabled: enabled, killSwitch: killSwitch}
}

func requestFingerprint(in CreateRequest) string {
	copy := in
	copy.IdempotencyKey = ""
	copy.CorrelationID = ""
	raw, _ := json.Marshal(copy)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateCreate(in CreateRequest) error {
	required := []string{in.OrganizationID, in.SettlementOrganizationID,
		in.ExecutingProviderID, in.OrganizationMembershipID, in.PrivateOrderID,
		in.ShipmentID, in.PrivateAssignmentID, in.ProviderResponseID,
		in.FleetOperationID, in.IdempotencyKey}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return errors.New("required private execution reference is missing")
		}
	}
	if in.ServiceCategory != "DISPATCH" {
		return errors.New("serviceCategory must be DISPATCH")
	}
	if strings.TrimSpace(in.Route.PickupAddress) == "" || strings.TrimSpace(in.Route.DropoffAddress) == "" {
		return errors.New("pickup and dropoff snapshots are required")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, in CreateRequest) (View, error) {
	if !s.enabled || s.killSwitch {
		return View{}, ErrDisabled
	}
	if err := validateCreate(in); err != nil {
		return View{}, err
	}
	fp := requestFingerprint(in)
	if prior, err := s.repo.getByKey(ctx, in.IdempotencyKey); err == nil {
		if prior.RequestFingerprint != fp {
			return View{}, ErrConflict
		}
		return s.reconcile(ctx, prior)
	} else if !errors.Is(err, ErrNotFound) {
		return View{}, err
	}

	record, err := s.repo.prepare(ctx, in, fp)
	if errors.Is(err, ErrConflict) {
		prior, getErr := s.repo.getByKey(ctx, in.IdempotencyKey)
		if getErr != nil {
			return View{}, getErr
		}
		if prior.RequestFingerprint != fp {
			return View{}, ErrConflict
		}
		return s.reconcile(ctx, prior)
	}
	if err != nil {
		return View{}, err
	}

	reservation, err := s.market.Reserve(ctx, marketplace.ReserveInput{
		RequestID: record.CanonicalRequestID, ExpectedRequestVersion: 1,
		OrganizationID:           in.OrganizationID,
		AcceptanceSource:         marketplace.AcceptanceProviderDirect,
		AcceptingProviderID:      in.ExecutingProviderID,
		OrganizationMembershipID: in.OrganizationMembershipID,
		IdempotencyKey:           "private-reservation:" + in.FleetOperationID,
		CorrelationID:            in.CorrelationID,
	})
	if err != nil {
		_ = s.repo.fail(ctx, in.FleetOperationID, StatusFailed, "reservation_rejected")
		return View{}, err
	}
	if err := s.repo.setReservation(ctx, in.FleetOperationID, reservation.ReservationID); err != nil {
		return View{}, err
	}

	provider, err := s.market.ValidateProvider(ctx, marketplace.ProviderEligibilityInput{
		RequestID: record.CanonicalRequestID, OrganizationID: in.OrganizationID,
		ProviderID:               in.ExecutingProviderID,
		OrganizationMembershipID: in.OrganizationMembershipID,
		ExpectedRequestVersion:   reservation.RequestVersion,
		RequireAvailability:      true,
	})
	if err != nil || !provider.Eligible {
		_ = s.repo.fail(ctx, in.FleetOperationID, StatusFailed, "provider_ineligible")
		if err != nil {
			return View{}, err
		}
		return View{}, ErrProviderIneligible
	}
	vehicle, err := s.market.ValidateVehicle(ctx, marketplace.VehicleEligibilityInput{
		RequestID: record.CanonicalRequestID, OrganizationID: in.OrganizationID,
		ProviderID: in.ExecutingProviderID, VehicleID: in.VehicleID,
		ExpectedRequestVersion: reservation.RequestVersion,
	})
	if err != nil || !vehicle.Eligible {
		_ = s.repo.fail(ctx, in.FleetOperationID, StatusFailed, "vehicle_ineligible")
		if err != nil {
			return View{}, err
		}
		return View{}, ErrVehicleIneligible
	}

	out, err := s.market.CreateExecution(ctx, marketplace.ExecutionInput{
		RequestID:              record.CanonicalRequestID,
		ExpectedRequestVersion: reservation.RequestVersion,
		ReservationID:          reservation.ReservationID, OrganizationID: in.OrganizationID,
		SettlementOrganizationID: in.SettlementOrganizationID,
		ExecutingProviderID:      in.ExecutingProviderID,
		OrganizationMembershipID: in.OrganizationMembershipID,
		FleetAssignmentID:        in.PrivateAssignmentID,
		ProviderResponseID:       in.ProviderResponseID,
		AcceptanceSource:         marketplace.AcceptanceProviderDirect,
		VehicleID:                in.VehicleID, ExecutionSource: "PRIVATE_ORDER",
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID,
		RequestIDHeader: in.FleetOperationID, FleetOperationID: in.FleetOperationID,
		PrivateOrderID: in.PrivateOrderID, ShipmentID: in.ShipmentID,
	})
	if err != nil {
		_ = s.repo.fail(ctx, in.FleetOperationID, StatusUnknown, "execution_outcome_unknown")
		return View{}, err
	}
	if err := s.repo.confirm(ctx, in.FleetOperationID, out.TripID, out.TripVersion); err != nil {
		return View{}, err
	}
	confirmed, err := s.repo.getByOperation(ctx, in.FleetOperationID)
	return confirmed.View, err
}

func (s *Service) reconcile(ctx context.Context, prior record) (View, error) {
	if prior.Status == StatusConfirmed || prior.Status == StatusCompleted || prior.Status == StatusCancelled {
		return prior.View, nil
	}
	out, err := s.market.ReadExecutionByIdempotencyKey(ctx, prior.IdempotencyKey)
	if err != nil {
		return prior.View, nil
	}
	if err := s.repo.confirm(ctx, prior.FleetOperationID, out.TripID, out.TripVersion); err != nil {
		return View{}, err
	}
	current, err := s.repo.getByOperation(ctx, prior.FleetOperationID)
	return current.View, err
}

func (s *Service) Read(ctx context.Context, operationID string) (View, error) {
	if !s.enabled || s.killSwitch {
		return View{}, ErrDisabled
	}
	record, err := s.repo.getByOperation(ctx, operationID)
	if err != nil {
		return View{}, err
	}
	return s.reconcile(ctx, record)
}

func cancelFingerprint(operationID, key, reason string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{operationID, key, reason}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Cancel(ctx context.Context, operationID string, in CancelRequest) (CancelView, error) {
	if !s.enabled || s.killSwitch {
		return CancelView{}, ErrDisabled
	}
	if in.IdempotencyKey == "" {
		return CancelView{}, errors.New("idempotency key is required")
	}
	record, err := s.repo.getByOperation(ctx, operationID)
	if err != nil {
		return CancelView{}, err
	}
	fp := cancelFingerprint(operationID, in.IdempotencyKey, in.Reason)
	if record.CancellationKey != "" {
		if record.CancellationKey != in.IdempotencyKey || record.CancellationFingerprint != fp {
			return CancelView{}, ErrConflict
		}
		if record.CancellationStatus == "CONFIRMED" {
			return CancelView{Status: "CANCELLED", SourceVersion: record.SourceVersion}, nil
		}
	}
	if record.CancellationStatus == "CONFIRMED" {
		return CancelView{Status: "CANCELLED", SourceVersion: record.SourceVersion}, nil
	}
	if record.CanonicalTripID == "" {
		return CancelView{}, ErrOutcomeUnknown
	}
	if s.canceller == nil {
		return CancelView{}, ErrDisabled
	}
	if _, err = s.canceller.CancelOrganizationTrip(ctx, record.CanonicalTripID, in.Reason, record.CorrelationID); err != nil {
		_ = s.repo.setCancellation(ctx, operationID, in.IdempotencyKey, fp, "REJECTED", record.SourceVersion)
		return CancelView{}, err
	}
	trip, err := s.market.ReadTrip(ctx, record.CanonicalTripID)
	if err != nil {
		_ = s.repo.setCancellation(ctx, operationID, in.IdempotencyKey, fp, "OUTCOME_UNKNOWN", record.SourceVersion)
		return CancelView{}, err
	}
	if err = s.repo.setCancellation(ctx, operationID, in.IdempotencyKey, fp, "CONFIRMED", trip.SourceVersion); err != nil {
		return CancelView{}, err
	}
	return CancelView{Status: "CANCELLED", SourceVersion: trip.SourceVersion}, nil
}

func (s *Service) ReadCancellation(ctx context.Context, operationID string) (CancelView, error) {
	record, err := s.repo.getByOperation(ctx, operationID)
	if err != nil {
		return CancelView{}, err
	}
	if record.CancellationStatus == "CONFIRMED" {
		return CancelView{Status: "CANCELLED", SourceVersion: record.SourceVersion}, nil
	}
	if record.CanonicalTripID == "" {
		return CancelView{Status: record.CancellationStatus, SourceVersion: record.SourceVersion}, nil
	}
	trip, err := s.market.ReadTrip(ctx, record.CanonicalTripID)
	if err != nil {
		return CancelView{}, err
	}
	if strings.EqualFold(trip.Status, "cancelled") {
		_ = s.repo.setCancellation(ctx, operationID, record.IdempotencyKey+":cancel", "readback", "CONFIRMED", trip.SourceVersion)
		return CancelView{Status: "CANCELLED", SourceVersion: trip.SourceVersion}, nil
	}
	return CancelView{Status: record.CancellationStatus, SourceVersion: trip.SourceVersion}, nil
}

var (
	ErrDisabled           = errors.New("dispatch private execution is disabled")
	ErrProviderIneligible = errors.New("dispatch provider is not eligible")
	ErrVehicleIneligible  = errors.New("dispatch vehicle is not eligible")
	ErrOutcomeUnknown     = errors.New("dispatch private execution outcome is unknown")
)
