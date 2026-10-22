package managedexec

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"
)

// ServiceTaxi is the Fleet service category this package speaks for.
const ServiceTaxi = "TAXI"

// ExecutionEngine is the slice of the normal taxi booking service this package
// drives.
//
// Section 8 TAX-014: corporate work enters the NORMAL taxi trip engine. This
// interface exists precisely so that is structurally true -- the only way this
// package can start a corporate ride is by calling the same accept path an
// independent ride uses, with all its guards, its payment hold and its
// operational-state transition intact.
type ExecutionEngine interface {
	AcceptBooking(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error)
	// AdminCancelBooking is the existing operator cancel path. Fleet's
	// CancelExecution maps onto it rather than onto a new taxi-side
	// cancellation rule, so a Fleet-driven cancellation follows exactly the
	// same guards, refund handling and audit trail as an operator one.
	AdminCancelBooking(ctx context.Context, operatorID, bookingID, reason string) (bookingmodels.Booking, error)
}

// BookingBinder offers a booking to a provider using the normal matcher
// transition (pending_match -> awaiting_acceptance).
type BookingBinder interface {
	MarkMatched(ctx context.Context, exec Executor, bookingID, providerID string, paymentIntentID *string) (bookingmodels.Booking, error)
}

// OperationalStateReader answers Taxi's own readiness question.
type OperationalStateReader interface {
	Get(ctx context.Context, providerID string) (availabilitymodels.State, error)
}

// AdminInterventionGate reports why a provider may not receive new work
// (restriction, out-of-service vehicle, reverification, invalid document).
type AdminInterventionGate interface {
	AssignmentBlock(ctx context.Context, providerID, vehicleID string) (string, error)
}

// VehicleReader answers Taxi's own vehicle-capability question. Satisfied
// directly by the existing provider_profile vehicle repository -- no adapter.
type VehicleReader interface {
	CountActiveByProvider(ctx context.Context, providerID string) (int, error)
}

// Options configure the service. Every collaborator is optional except the
// repository: a deployment without the taxi engine wired simply cannot serve
// the execution routes, and fails closed rather than half-working.
type Options struct {
	Repo             *Repository
	Engine           ExecutionEngine
	Binder           BookingBinder
	Bookings         Executor
	OperationalState OperationalStateReader
	Vehicles         VehicleReader
	Corporate        providercompat.CorporateContextReader
	// AdminGate is the Taxi-owned administrative overlay (Admin Section 8).
	// nil keeps eligibility unchanged.
	AdminGate AdminInterventionGate
	// ReservationTTL bounds how long a reservation holds a booking before it
	// can be reclaimed. Zero uses the default.
	ReservationTTL time.Duration
	Now            func() time.Time
}

// Service implements Taxi's half of the Fleet execution contract.
type Service struct {
	opts Options
}

const defaultReservationTTL = 5 * time.Minute

// NewService builds the managed-execution service.
func NewService(opts Options) *Service {
	if opts.ReservationTTL <= 0 {
		opts.ReservationTTL = defaultReservationTTL
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{opts: opts}
}

// Reserve claims a taxi booking for one organization (TAX-013).
//
// Idempotency comes first and is total: the same key always resolves to the
// same reservation. That is what makes Fleet's own retry loop safe -- a
// reservation that timed out in transit is READ BACK, never re-created.
func (s *Service) Reserve(ctx context.Context, in ReserveInput) (ReservationOutcome, error) {
	if err := in.Validate(); err != nil {
		return ReservationOutcome{}, apperrors.BadRequest(err.Error(), err)
	}
	fingerprint := Fingerprint(in)

	// Replay: identical input returns the original answer, different input
	// under the same key is a conflict rather than a second reservation.
	if existing, err := s.opts.Repo.GetByIdempotencyKey(ctx, in.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fingerprint {
			return ReservationOutcome{}, apperrors.Conflict("This idempotency key was already used with different input.", ErrIdempotencyConflict)
		}
		return outcomeFor(existing), nil
	} else if !errors.Is(err, ErrNotFound) {
		return ReservationOutcome{}, apperrors.Unavailable("The reservation could not be read.", err)
	}

	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, in.SourceRequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReservationOutcome{}, apperrors.NotFound("This taxi request does not exist.", err)
		}
		return ReservationOutcome{}, apperrors.Unavailable("The taxi request could not be read.", err)
	}

	// Optimistic-concurrency check against Fleet's view of the request.
	if in.ExpectedSourceVersion > 0 && snapshot.SourceVersion != in.ExpectedSourceVersion {
		return ReservationOutcome{Status: OutcomeVersionMismatch, SourceVersion: snapshot.SourceVersion}, nil
	}
	switch snapshot.Status {
	case string(bookingmodels.StatusPendingMatch):
		// Reservable.
	case string(bookingmodels.StatusCancelled):
		return ReservationOutcome{Status: OutcomeCancelled, SourceVersion: snapshot.SourceVersion}, nil
	case string(bookingmodels.StatusUnmatched):
		return ReservationOutcome{Status: OutcomeExpired, SourceVersion: snapshot.SourceVersion}, nil
	default:
		// Already matched, accepted or in flight: someone else owns it.
		return ReservationOutcome{
			Status: OutcomeAlreadyClaimed, SourceVersion: snapshot.SourceVersion,
			WinnerOrganizationID: snapshot.OrganizationID,
		}, nil
	}

	// A provider-direct (delegated) reservation must clear Taxi's own
	// readiness rules before the claim is taken. Fleet eligibility alone is
	// never sufficient authority for taxi execution (TAX-004).
	if provider := strings.TrimSpace(in.AcceptingProviderID); provider != "" {
		answer, err := s.Eligibility(ctx, in.SourceRequestID, provider, "")
		if err != nil {
			return ReservationOutcome{}, err
		}
		if !answer.Eligible {
			return ReservationOutcome{Status: OutcomeNotEligible, SourceVersion: snapshot.SourceVersion}, nil
		}
	}

	expires := s.opts.Now().Add(s.opts.ReservationTTL)
	created, err := s.opts.Repo.Insert(ctx, Reservation{
		BookingID: snapshot.BookingID, SourceRequestID: in.SourceRequestID,
		ProviderID: in.AcceptingProviderID, OrganizationID: in.OrganizationID,
		MembershipID: in.OrganizationMembershipID, AcceptanceSource: in.AcceptanceSource,
		ExpectedSourceVersion: in.ExpectedSourceVersion, SourceVersion: snapshot.SourceVersion,
		IdempotencyKey: in.IdempotencyKey, RequestFingerprint: fingerprint,
		CorrelationID: in.CorrelationID, ExpiresAt: &expires,
	})
	if err != nil {
		// The partial unique index decided the race: another organization
		// already holds this booking. Report the definite outcome rather than
		// an error, so Fleet records a lost race instead of retrying.
		if isUniqueViolation(err) {
			if live, readErr := s.opts.Repo.GetLiveByBooking(ctx, snapshot.BookingID); readErr == nil {
				return ReservationOutcome{
					Status: OutcomeAlreadyClaimed, ReservationID: live.ID,
					WinnerOrganizationID: live.OrganizationID, SourceVersion: live.SourceVersion,
				}, nil
			}
			return ReservationOutcome{Status: OutcomeAlreadyClaimed, SourceVersion: snapshot.SourceVersion}, nil
		}
		return ReservationOutcome{}, apperrors.Unavailable("The reservation could not be created.", err)
	}
	return outcomeFor(created), nil
}

// ReadReservation resolves an ambiguous reservation by read-back.
//
// This is what makes a timed-out reservation safe: the answer comes from
// Taxi's own durable row keyed by the SAME idempotency key, never from a blind
// retry that could create a second claim.
func (s *Service) ReadReservation(ctx context.Context, sourceRequestID, idempotencyKey string) (ReservationOutcome, error) {
	if strings.TrimSpace(idempotencyKey) != "" {
		res, err := s.opts.Repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err == nil {
			return outcomeFor(res), nil
		}
		if !errors.Is(err, ErrNotFound) {
			return ReservationOutcome{}, apperrors.Unavailable("The reservation could not be read.", err)
		}
	}
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceRequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReservationOutcome{}, apperrors.NotFound("This taxi request does not exist.", err)
		}
		return ReservationOutcome{}, apperrors.Unavailable("The taxi request could not be read.", err)
	}
	live, err := s.opts.Repo.GetLiveByBooking(ctx, snapshot.BookingID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ReservationOutcome{Status: OutcomeExpired, SourceVersion: snapshot.SourceVersion}, nil
		}
		return ReservationOutcome{}, apperrors.Unavailable("The reservation could not be read.", err)
	}
	return outcomeFor(live), nil
}

// ReleaseReservation returns a reserved booking to the open pool.
func (s *Service) ReleaseReservation(ctx context.Context, sourceRequestID, organizationID string) error {
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceRequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperrors.NotFound("This taxi request does not exist.", err)
		}
		return apperrors.Unavailable("The taxi request could not be read.", err)
	}
	live, err := s.opts.Repo.GetLiveByBooking(ctx, snapshot.BookingID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Already released. Releasing twice is a no-op, not an error.
			return nil
		}
		return apperrors.Unavailable("The reservation could not be read.", err)
	}
	// An organization may only release its own claim.
	if organizationID != "" && live.OrganizationID != organizationID {
		return apperrors.Forbidden("This reservation belongs to a different organization.", nil)
	}
	if _, err := s.opts.Repo.Release(ctx, live.ID, StatusReleased); err != nil {
		return apperrors.Unavailable("The reservation could not be released.", err)
	}
	return nil
}

// Eligibility answers Taxi's own readiness question (TAX-004).
//
// Fleet membership validity is necessary but NOT sufficient: a driver whose
// membership is perfect but who is offline, already on a trip, or has no
// active vehicle is not eligible for taxi execution. Taxi keeps that
// authority; it is never moved into Fleet.
func (s *Service) Eligibility(ctx context.Context, sourceRequestID, providerID, vehicleReference string) (EligibilityAnswer, error) {
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceRequestID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return EligibilityAnswer{}, apperrors.Unavailable("The taxi request could not be read.", err)
	}
	answer := EligibilityAnswer{SourceVersion: snapshot.SourceVersion}

	// 1. Taxi operational state: available and not already on conflicting work.
	if s.opts.OperationalState != nil {
		state, err := s.opts.OperationalState.Get(ctx, providerID)
		if err != nil {
			return EligibilityAnswer{}, apperrors.Unavailable("Driver readiness could not be verified.", err)
		}
		answer.PlatformStatus = string(state.Current)
		if !state.Current.Eligible() {
			answer.ReasonCode = "PROVIDER_NOT_AVAILABLE"
			return answer, nil
		}
	}

	// 1b. Taxi administrative interventions. Fail closed on a read error.
	if s.opts.AdminGate != nil {
		reason, err := s.opts.AdminGate.AssignmentBlock(ctx, providerID, "")
		if err != nil {
			return EligibilityAnswer{}, apperrors.Unavailable("Driver readiness could not be verified.", err)
		}
		if reason != "" {
			answer.ReasonCode = "ADMIN_RESTRICTED"
			return answer, nil
		}
	}

	// 2. Taxi vehicle capability.
	if s.opts.Vehicles != nil {
		count, err := s.opts.Vehicles.CountActiveByProvider(ctx, providerID)
		if err != nil {
			return EligibilityAnswer{}, apperrors.Unavailable("Driver vehicle records could not be verified.", err)
		}
		answer.VehicleOK = count > 0
		if !answer.VehicleOK {
			answer.ReasonCode = "NO_ACTIVE_VEHICLE"
			return answer, nil
		}
	} else {
		answer.VehicleOK = true
	}

	// 3. Fleet membership must be live and cover TAXI. Checked LAST so a
	// Fleet outage cannot mask a Taxi-side reason the provider could fix, and
	// so an outage is reported as a retryable dependency failure rather than
	// as "not eligible" -- an invented negative about a provider nobody asked.
	if s.opts.Corporate != nil {
		if _, err := providercompat.AuthorizeManagedOperation(ctx, s.opts.Corporate, providercompat.ManagedRequest{
			Service: ServiceTaxi, ProviderID: providerID, RequireManaged: true,
		}); err != nil {
			kind, _ := providercompat.KindOf(err)
			if kind.Retryable() {
				return EligibilityAnswer{}, apperrors.Unavailable("Corporate membership could not be verified.", err)
			}
			answer.Eligible = false
			answer.ReasonCode = string(kind)
			return answer, nil
		}
	}

	answer.Eligible = true
	return answer, nil
}

// ConfirmProviderAndCreateExecution binds an organization's managed provider
// to a reserved booking and starts the canonical taxi execution (TAX-012,
// TAX-014, TAX-014A).
//
// The sequencing matters:
//  1. resolve the reservation (no reservation => NOT_RESERVED, never a trip),
//  2. re-verify the provider against Fleet fail-closed,
//  3. bind corporate metadata and offer the booking to the provider inside ONE
//     transaction, so a crash can never leave a bound booking with no offer,
//  4. accept through the NORMAL taxi engine, which performs the payment hold
//     and the operational-state transition exactly as an independent ride.
func (s *Service) ConfirmProviderAndCreateExecution(ctx context.Context, in ConfirmInput) (ExecutionOutcome, error) {
	if err := in.Validate(); err != nil {
		return ExecutionOutcome{}, apperrors.BadRequest(err.Error(), err)
	}
	if s.opts.Engine == nil || s.opts.Binder == nil {
		return ExecutionOutcome{}, apperrors.Unavailable("Taxi execution is not available.", nil)
	}

	reservation, err := s.opts.Repo.GetByID(ctx, in.ReservationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ExecutionOutcome{Status: ExecutionNotReserved, RejectionCode: "RESERVATION_NOT_FOUND"}, nil
		}
		return ExecutionOutcome{}, apperrors.Unavailable("The reservation could not be read.", err)
	}

	// Idempotent replay: the reservation already produced an execution.
	if reservation.Status == StatusConfirmed && reservation.ExecutionBookingID != nil {
		return ExecutionOutcome{
			Status: ExecutionAlreadyExists, TripID: *reservation.ExecutionBookingID,
			RequestID: reservation.SourceRequestID, RequestVersion: reservation.SourceVersion,
		}, nil
	}
	if !reservation.Live() {
		return ExecutionOutcome{Status: ExecutionNotReserved, RejectionCode: "RESERVATION_" + reservation.Status}, nil
	}
	if reservation.OrganizationID != in.OrganizationID {
		return ExecutionOutcome{Status: ExecutionRejected, RejectionCode: "ORGANIZATION_MISMATCH"}, nil
	}
	// A reservation taken for one provider can never be confirmed for a
	// different one: that is the "two providers, one job" failure TAX-013
	// exists to prevent.
	if reservation.ProviderID != "" && reservation.ProviderID != in.ExecutingProviderID {
		return ExecutionOutcome{Status: ExecutionRejected, RejectionCode: "PROVIDER_MISMATCH"}, nil
	}

	// Fail-closed re-verification against Fleet at the moment of execution.
	if s.opts.Corporate != nil {
		if _, err := providercompat.AuthorizeManagedOperation(ctx, s.opts.Corporate, providercompat.ManagedRequest{
			Service: ServiceTaxi, ProviderID: in.ExecutingProviderID,
			ExpectedOrganizationID: in.OrganizationID, RequireManaged: true,
		}); err != nil {
			kind, _ := providercompat.KindOf(err)
			if kind.Retryable() {
				return ExecutionOutcome{}, apperrors.Unavailable("Corporate membership could not be verified.", err)
			}
			return ExecutionOutcome{Status: ExecutionRejected, RejectionCode: string(kind)}, nil
		}
	}

	tx, err := s.opts.Repo.BeginTx(ctx)
	if err != nil {
		return ExecutionOutcome{}, apperrors.Unavailable("The execution could not be created.", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sourceVersion, err := s.opts.Repo.BindCorporateExecution(ctx, tx, in, reservation.BookingID)
	if err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return ExecutionOutcome{Status: ExecutionRejected, RejectionCode: "ASSIGNMENT_CONFLICT"}, nil
		}
		if isUniqueViolation(err) {
			// The TAX-014A unique index refused a second execution for this
			// Fleet assignment.
			return ExecutionOutcome{Status: ExecutionRejected, RejectionCode: "ASSIGNMENT_ALREADY_EXECUTING"}, nil
		}
		return ExecutionOutcome{}, apperrors.Unavailable("The execution could not be created.", err)
	}
	if _, err := s.opts.Binder.MarkMatched(ctx, tx, reservation.BookingID, in.ExecutingProviderID, nil); err != nil {
		return ExecutionOutcome{}, err
	}
	if _, err := s.opts.Repo.Confirm(ctx, tx, reservation.ID, reservation.BookingID, in.ExecutingProviderID, reservation.Version); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ExecutionOutcome{Status: ExecutionNotReserved, RejectionCode: "RESERVATION_RACE"}, nil
		}
		return ExecutionOutcome{}, apperrors.Unavailable("The reservation could not be confirmed.", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionOutcome{}, apperrors.Unavailable("The execution could not be created.", err)
	}

	// The booking is now offered to the managed provider. Accept through the
	// NORMAL taxi engine so the payment hold, the guarded status transition
	// and the operational-state change are exactly the independent ones.
	if _, err := s.opts.Engine.AcceptBooking(ctx, reservation.BookingID, in.ExecutingProviderID); err != nil {
		// The booking stays offered; Fleet resolves the ambiguity by reading
		// the execution outcome back rather than by retrying blindly.
		return ExecutionOutcome{}, err
	}
	return ExecutionOutcome{
		Status: ExecutionCreated, TripID: reservation.BookingID,
		RequestID: reservation.SourceRequestID, RequestVersion: sourceVersion,
	}, nil
}

// ReadExecutionOutcome resolves an ambiguous execution creation by read-back
// on the same idempotency identity.
func (s *Service) ReadExecutionOutcome(ctx context.Context, idempotencyKey string) (ExecutionOutcome, error) {
	res, err := s.opts.Repo.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ExecutionOutcome{}, apperrors.NotFound("No execution exists for this idempotency key.", err)
		}
		return ExecutionOutcome{}, apperrors.Unavailable("The execution could not be read.", err)
	}
	if res.ExecutionBookingID == nil {
		return ExecutionOutcome{Status: ExecutionNotReserved, RequestID: res.SourceRequestID}, nil
	}
	return ExecutionOutcome{
		Status: ExecutionAlreadyExists, TripID: *res.ExecutionBookingID,
		RequestID: res.SourceRequestID, RequestVersion: res.SourceVersion,
	}, nil
}

// ReadRequest returns the authoritative booking state for Fleet reconciliation.
func (s *Service) ReadRequest(ctx context.Context, sourceRequestID string) (RequestView, error) {
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceRequestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RequestView{}, apperrors.NotFound("This taxi request does not exist.", err)
		}
		return RequestView{}, apperrors.Unavailable("The taxi request could not be read.", err)
	}
	view := RequestView{
		RequestID: snapshot.BookingID, BookingID: snapshot.BookingID,
		Status: snapshot.Status, SourceVersion: snapshot.SourceVersion,
		WinnerOrganizationID: snapshot.OrganizationID, AcceptedProviderID: snapshot.ProviderID,
	}
	if live, err := s.opts.Repo.GetLiveByBooking(ctx, snapshot.BookingID); err == nil && live.ExpiresAt != nil {
		view.ExpiresAt = live.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return view, nil
}

// ReadTrip returns the authoritative trip state for Fleet reconciliation. In
// Taxi the booking IS the trip, so this is the same row under its trip view.
func (s *Service) ReadTrip(ctx context.Context, sourceTripID string) (TripView, error) {
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceTripID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return TripView{}, apperrors.NotFound("This taxi trip does not exist.", err)
		}
		return TripView{}, apperrors.Unavailable("The taxi trip could not be read.", err)
	}
	view := TripView{
		TripID: snapshot.BookingID, RequestID: snapshot.BookingID, BookingID: snapshot.BookingID,
		Status: snapshot.Status, SourceVersion: snapshot.SourceVersion,
		ExecutingProviderID: snapshot.ProviderID,
	}
	if snapshot.CompletedAt != nil {
		view.CompletedAt = snapshot.CompletedAt.UTC().Format(time.RFC3339)
	}
	if snapshot.CancelledAt != nil {
		view.CancelledAt = snapshot.CancelledAt.UTC().Format(time.RFC3339)
	}
	return view, nil
}

// CancelExecution cancels a canonical taxi execution on Fleet's instruction.
//
// The reservation is released FIRST so that a cancel which then fails its
// lifecycle guard (a ride already completed, say) still frees the claim rather
// than stranding the booking behind a reservation nobody owns.
func (s *Service) CancelExecution(ctx context.Context, sourceTripID, reason string) error {
	if s.opts.Engine == nil {
		return apperrors.Unavailable("Taxi execution is not available.", nil)
	}
	snapshot, err := s.opts.Repo.ResolveBookingByRequest(ctx, sourceTripID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperrors.NotFound("This taxi trip does not exist.", err)
		}
		return apperrors.Unavailable("The taxi trip could not be read.", err)
	}
	if err := s.opts.Repo.ReleaseLiveByBooking(ctx, snapshot.BookingID, StatusCancelled); err != nil {
		return apperrors.Unavailable("The reservation could not be released.", err)
	}
	if reason == "" {
		reason = "cancelled_by_fleet"
	}
	_, err = s.opts.Engine.AdminCancelBooking(ctx, "fleet-partner-service", snapshot.BookingID, reason)
	return err
}

func outcomeFor(res Reservation) ReservationOutcome {
	status := OutcomeReserved
	switch res.Status {
	case StatusReleased:
		status = OutcomeCancelled
	case StatusExpired:
		status = OutcomeExpired
	case StatusCancelled:
		status = OutcomeCancelled
	}
	return ReservationOutcome{
		ReservationID: res.ID, Status: status,
		WinnerOrganizationID: res.OrganizationID, SourceVersion: res.SourceVersion,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ = pgx.ErrNoRows
