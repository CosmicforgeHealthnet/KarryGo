package marketplace

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Flags gate every organization-managed write. They default to false so an
// incompletely configured deployment fails closed rather than half-working.
type Flags struct {
	InternalAPIEnabled    bool
	ReservationEnabled    bool
	ExecutionEnabled      bool
	OutboxEnabled         bool
	EventDeliveryEnabled  bool
	ProviderOffersEnabled bool
	CompanyAssignEnabled  bool
	// Eligibility checks are separately gated so they can be exercised in a
	// read-only rollout before any organization write is enabled.
	ProviderEligibilityEnabled bool
	VehicleEligibilityEnabled  bool
	FleetRelayEnabled          bool
}

// ProviderAcceptance is fleet's answer to "did this provider actually accept?".
type ProviderAcceptance struct {
	AssignmentID             string
	OrganizationID           string
	ProviderID               string
	OrganizationMembershipID string
	ProviderResponseID       string
	ResponseStatus           string
	AcceptedAt               *time.Time
	AssignmentStatus         string
	AssignmentVersion        int64
	SourceRequestID          string
}

// AcceptanceVerifier reads provider acceptance from fleet-partner-service.
//
// Dispatch cannot answer this question locally. The provider's decision is
// owned by fleet, and a non-null provider_response_id column proves only that
// a value was supplied — not that a provider ever accepted. This port is the
// authoritative check, and it must fail closed.
type AcceptanceVerifier interface {
	VerifyProviderAcceptance(ctx context.Context, meta CallMeta, assignmentID string) (*ProviderAcceptance, error)
	Configured() bool
}

// CallMeta carries correlation across the service boundary.
type CallMeta struct {
	RequestID     string
	CorrelationID string
}

// UnconfiguredVerifier is the default: it refuses rather than assuming
// acceptance. Wiring a real verifier is required to enable execution.
type UnconfiguredVerifier struct{}

// Configured reports false.
func (UnconfiguredVerifier) Configured() bool { return false }

// VerifyProviderAcceptance always fails closed.
func (UnconfiguredVerifier) VerifyProviderAcceptance(context.Context, CallMeta, string) (*ProviderAcceptance, error) {
	return nil, ErrFleetUnavailable
}

// Cross-service verification errors.
var (
	ErrFleetUnavailable        = errors.New("marketplace: fleet service unavailable")
	ErrAcceptanceNotConfirmed  = errors.New("marketplace: provider acceptance not confirmed")
	ErrAcceptanceMismatch      = errors.New("marketplace: provider acceptance does not match the request")
	ErrFleetAssignmentNotFound = errors.New("marketplace: fleet assignment not found")
)

// Service implements the organization marketplace use cases.
type Service struct {
	repo     *Repository
	flags    Flags
	verifier AcceptanceVerifier
	now      func() time.Time
	// responder delivers provider decisions to fleet. Nil or unconfigured
	// means decisions are still recorded durably and delivered later by the
	// relay worker — never dropped, never faked as success.
	responder      FleetResponder
	relayAttempts  int
	providerReader ProviderReader
}

// WithResponder attaches the fleet responder used for immediate delivery of
// provider decisions.
func (s *Service) WithResponder(responder FleetResponder, maxAttempts int) *Service {
	s.responder = responder
	s.relayAttempts = maxAttempts
	return s
}

// NewService builds the service.
func NewService(repo *Repository, flags Flags, verifier AcceptanceVerifier) *Service {
	if verifier == nil {
		verifier = UnconfiguredVerifier{}
	}
	return &Service{repo: repo, flags: flags, verifier: verifier, now: func() time.Time { return time.Now().UTC() }}
}

// ReserveInput is a fleet request to claim a canonical request.
type ReserveInput struct {
	RequestID                string
	ExpectedRequestVersion   int64
	OrganizationID           string
	AcceptanceSource         string
	AcceptingProviderID      string
	OrganizationMembershipID string
	IdempotencyKey           string
	CorrelationID            string
}

// reserveFingerprint is the deterministic shape hashed for idempotency.
// It is a struct, not a map, so field order cannot vary between runs.
type reserveFingerprint struct {
	RequestID        string `json:"requestId"`
	OrganizationID   string `json:"organizationId"`
	AcceptanceSource string `json:"acceptanceSource"`
	ProviderID       string `json:"acceptingProviderId"`
	MembershipID     string `json:"organizationMembershipId"`
	ExpectedVersion  int64  `json:"expectedRequestVersion"`
}

// ReservationResult is the stable response body.
type ReservationResult struct {
	ReservationID            string     `json:"reservationId"`
	RequestID                string     `json:"requestId"`
	BroadcastID              string     `json:"broadcastId"`
	BookingID                string     `json:"bookingId"`
	OrganizationID           string     `json:"organizationId"`
	Status                   string     `json:"status"`
	AcceptanceSource         string     `json:"acceptanceSource"`
	AcceptingProviderID      *string    `json:"acceptingProviderId"`
	OrganizationMembershipID *string    `json:"organizationMembershipId"`
	RequestVersion           int64      `json:"requestVersion"`
	ReservationVersion       int64      `json:"reservationVersion"`
	ReservedAt               *time.Time `json:"reservedAt"`
	ReleasedAt               *time.Time `json:"releasedAt"`
	ReleaseReason            *string    `json:"releaseReason"`
}

// Reserve claims a canonical dispatch request for one organization.
//
// Reserving is NOT provider acceptance. It only means the organization has
// taken commercial responsibility for the request; the provider who will
// actually perform it still has to say yes.
func (s *Service) Reserve(ctx context.Context, input ReserveInput) (*ReservationResult, error) {
	if !s.flags.ReservationEnabled {
		return nil, apperrors.Unavailable("Organization reservation is not enabled.", nil)
	}
	if input.IdempotencyKey == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}
	if input.OrganizationID == "" || input.RequestID == "" {
		return nil, apperrors.BadRequest("requestId and organizationId are required.", nil)
	}
	switch input.AcceptanceSource {
	case AcceptanceOrganizationDispatcher:
		if input.AcceptingProviderID != "" {
			return nil, apperrors.BadRequest(
				"A dispatcher reservation must not name an accepting provider.", nil)
		}
	case AcceptanceProviderDirect:
		// Delegated mode: the provider who accepted the organization's offer
		// is recorded here, and that same provider must be the one who later
		// executes. A missing provider would make the claim unattributable.
		if input.AcceptingProviderID == "" || input.OrganizationMembershipID == "" {
			return nil, apperrors.BadRequest(
				"A provider-direct reservation requires the accepting provider and membership.", nil)
		}
	default:
		return nil, apperrors.BadRequest("acceptanceSource is invalid.", nil)
	}

	fingerprint := Fingerprint(reserveFingerprint{
		RequestID:        input.RequestID,
		OrganizationID:   input.OrganizationID,
		AcceptanceSource: input.AcceptanceSource,
		ProviderID:       input.AcceptingProviderID,
		MembershipID:     input.OrganizationMembershipID,
		ExpectedVersion:  input.ExpectedRequestVersion,
	})

	// The reservation body lives in a separate tx-scoped method so the
	// delegated provider-direct path can commit the reservation and its relay
	// in ONE transaction, instead of duplicating this logic.
	var result *ReservationResult
	err := s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		var reserveErr error
		result, reserveErr = s.reserveWithinTx(ctx, tx, input, fingerprint)
		return reserveErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// reserveWithinTx performs the reservation inside a caller-supplied
// transaction.
//
// Exposed separately so the delegated provider-direct flow can atomically
// commit the canonical reservation together with the durable relay that
// carries the provider's decision. Committing them apart would leave a
// crash window where the request is reserved but the decision is lost.
func (s *Service) reserveWithinTx(ctx context.Context, tx pgx.Tx, input ReserveInput,
	fingerprint string) (*ReservationResult, error) {
	var result *ReservationResult
	run := func(tx pgx.Tx) error {
		requestID := input.RequestID
		op := &CommandOperation{
			OperationType:      CommandReserveRequest,
			RequestID:          &requestID,
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
			// A confirmed replay is answered from disk. This is what makes a
			// lost response safe: fleet retries with the same key and gets the
			// original reservation instead of a second one.
			existing, err := s.repo.GetReservationByIdempotencyKey(ctx, tx, input.IdempotencyKey)
			if err != nil {
				return err
			}
			broadcast, err := s.repo.GetBroadcast(ctx, tx, existing.RequestID)
			if err != nil {
				return err
			}
			result = buildReservationResult(existing, broadcast)
			return nil
		}
		if replayed && stored.Status == CommandRejected {
			return errRequestAlreadyClaimed()
		}

		broadcast, err := s.repo.LockBroadcast(ctx, tx, input.RequestID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return apperrors.NotFound("Request not found.", nil)
			}
			return err
		}

		now := s.now()
		if broadcast.Status == "cancelled" {
			_ = s.repo.CompleteCommand(ctx, tx, stored.ID, CommandRejected, nil, "request_cancelled", nil)
			return errCoded(409, "request_cancelled", "This request has been cancelled.")
		}
		if broadcast.Status == "expired" || !now.Before(broadcast.ExpiresAt) {
			_ = s.repo.CompleteCommand(ctx, tx, stored.ID, CommandRejected, nil, "request_expired", nil)
			return errCoded(409, "request_expired", "This request has expired.")
		}
		if !BroadcastAssignable(broadcast.Status) {
			_ = s.repo.CompleteCommand(ctx, tx, stored.ID, CommandRejected, nil,
				"request_not_marketplace_eligible", nil)
			return errCoded(409, "request_not_marketplace_eligible",
				"This request is not available to organizations.")
		}
		if input.ExpectedRequestVersion != 0 && input.ExpectedRequestVersion != broadcast.SourceVersion {
			return errCoded(409, "source_version_mismatch",
				"This request has changed. Reload and try again.")
		}
		if broadcast.ReservationStatus != BroadcastReservationNone {
			_ = s.repo.CompleteCommand(ctx, tx, stored.ID, CommandRejected, nil, "request_already_claimed", nil)
			return errRequestAlreadyClaimed()
		}

		reservation := &Reservation{
			RequestID:            input.RequestID,
			OrganizationID:       input.OrganizationID,
			Status:               ReservationReserved,
			AcceptanceSource:     input.AcceptanceSource,
			SourceRequestVersion: broadcast.SourceVersion,
			IdempotencyKey:       input.IdempotencyKey,
			RequestFingerprint:   fingerprint,
		}
		if input.AcceptingProviderID != "" {
			reservation.AcceptingProviderID = &input.AcceptingProviderID
		}
		if input.OrganizationMembershipID != "" {
			reservation.OrganizationMembershipID = &input.OrganizationMembershipID
		}

		if err := s.repo.CreateReservation(ctx, tx, reservation); err != nil {
			if errors.Is(err, ErrRequestAlreadyClaim) {
				_ = s.repo.CompleteCommand(ctx, tx, stored.ID, CommandRejected, nil,
					"request_already_claimed", nil)
				return errRequestAlreadyClaimed()
			}
			if errors.Is(err, ErrIdempotencyConflict) {
				return errIdempotencyConflict()
			}
			return err
		}

		newVersion, err := s.repo.SetBroadcastReservation(ctx, tx, broadcast.ID,
			BroadcastReservationReserved, &input.OrganizationID, broadcast.SourceVersion)
		if err != nil {
			if errors.Is(err, ErrVersionMismatch) {
				return errCoded(409, "source_version_mismatch",
					"This request has changed. Reload and try again.")
			}
			return err
		}
		broadcast.SourceVersion = newVersion

		if s.flags.OutboxEnabled {
			if err := s.repo.PublishEvent(ctx, tx, "platform.request.reserved.v1", "REQUEST",
				broadcast.ID, newVersion, input.CorrelationID, "", map[string]any{
					"sourceService":            "driver-dispatch-delivery-service",
					"requestId":                broadcast.ID,
					"broadcastId":              broadcast.ID,
					"bookingId":                broadcast.BookingID,
					"sourceVersion":            newVersion,
					"organizationId":           input.OrganizationID,
					"organizationMembershipId": input.OrganizationMembershipID,
					"acceptanceSource":         input.AcceptanceSource,
					"status":                   "reserved",
					"occurredAt":               now,
				}); err != nil {
				return err
			}
		}

		result = buildReservationResult(reservation, broadcast)
		return s.repo.CompleteCommand(ctx, tx, stored.ID, CommandConfirmed, result, "", nil)
	}
	if err := run(tx); err != nil {
		return nil, err
	}
	return result, nil
}

// ReadReservation returns a reservation by id.
func (s *Service) ReadReservation(ctx context.Context, reservationID string) (*ReservationResult, error) {
	reservation, err := s.repo.GetReservation(ctx, nil, reservationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "reservation_not_found", "Reservation not found.")
		}
		return nil, err
	}
	broadcast, err := s.repo.GetBroadcast(ctx, nil, reservation.RequestID)
	if err != nil {
		return nil, err
	}
	return buildReservationResult(reservation, broadcast), nil
}

// ReadReservationByIdempotencyKey is the read-back after an unknown outcome.
func (s *Service) ReadReservationByIdempotencyKey(ctx context.Context, key string) (*ReservationResult, error) {
	reservation, err := s.repo.GetReservationByIdempotencyKey(ctx, nil, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "reservation_not_found", "Reservation not found.")
		}
		return nil, err
	}
	broadcast, err := s.repo.GetBroadcast(ctx, nil, reservation.RequestID)
	if err != nil {
		return nil, err
	}
	return buildReservationResult(reservation, broadcast), nil
}

// ReadReservationForRequest returns the live reservation on a request.
func (s *Service) ReadReservationForRequest(ctx context.Context, requestID string) (*ReservationResult, error) {
	reservation, err := s.repo.GetLiveReservationForRequest(ctx, nil, requestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "reservation_not_found", "No live reservation for this request.")
		}
		return nil, err
	}
	broadcast, err := s.repo.GetBroadcast(ctx, nil, reservation.RequestID)
	if err != nil {
		return nil, err
	}
	return buildReservationResult(reservation, broadcast), nil
}

// ReleaseInput releases an organization's claim.
type ReleaseInput struct {
	ReservationID              string
	ReasonCode                 string
	ExpectedReservationVersion int64
	IdempotencyKey             string
	CorrelationID              string
}

type releaseFingerprint struct {
	ReservationID string `json:"reservationId"`
	ReasonCode    string `json:"reasonCode"`
}

// Release gives the request back to the marketplace.
func (s *Service) Release(ctx context.Context, input ReleaseInput) (*ReservationResult, error) {
	if !s.flags.ReservationEnabled {
		return nil, apperrors.Unavailable("Organization reservation is not enabled.", nil)
	}
	if input.IdempotencyKey == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}
	if !ValidReleaseReason(input.ReasonCode) {
		return nil, apperrors.BadRequest("reasonCode is not a supported release reason.", nil)
	}

	fingerprint := Fingerprint(releaseFingerprint{
		ReservationID: input.ReservationID, ReasonCode: input.ReasonCode,
	})

	var result *ReservationResult
	err := s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		op := &CommandOperation{
			OperationType:      CommandReleaseReservation,
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
			return s.loadReleaseResult(ctx, tx, input.ReservationID, &result)
		}

		reservation, err := s.repo.LockReservation(ctx, tx, input.ReservationID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return errCoded(404, "reservation_not_found", "Reservation not found.")
			}
			return err
		}
		if !reservation.Releasable() {
			return errCoded(409, "reservation_already_released",
				"This reservation is no longer active.")
		}

		broadcast, err := s.repo.LockBroadcast(ctx, tx, reservation.RequestID)
		if err != nil {
			return err
		}
		// Releasing after execution has started would orphan a live canonical
		// trip; that must go through trip cancellation instead.
		if broadcast.ReservationStatus == BroadcastReservationExecuted {
			return errCoded(409, "reservation_conflict",
				"This request already has a canonical trip. Cancel the trip instead.")
		}

		expected := input.ExpectedReservationVersion
		if expected == 0 {
			expected = reservation.Version
		}
		released, err := s.repo.ReleaseReservation(ctx, tx, reservation.ID, input.ReasonCode, expected)
		if err != nil {
			if errors.Is(err, ErrVersionMismatch) {
				return errCoded(409, "expected_version_mismatch",
					"This reservation changed. Reload and try again.")
			}
			return err
		}

		newVersion, err := s.repo.SetBroadcastReservation(ctx, tx, broadcast.ID,
			BroadcastReservationNone, nil, broadcast.SourceVersion)
		if err != nil {
			return err
		}
		broadcast.SourceVersion = newVersion

		if s.flags.OutboxEnabled {
			if err := s.repo.PublishEvent(ctx, tx, "platform.request.reservation_released.v1",
				"REQUEST", broadcast.ID, newVersion, input.CorrelationID, "", map[string]any{
					"sourceService":  "driver-dispatch-delivery-service",
					"requestId":      broadcast.ID,
					"broadcastId":    broadcast.ID,
					"bookingId":      broadcast.BookingID,
					"sourceVersion":  newVersion,
					"organizationId": reservation.OrganizationID,
					"reasonCode":     input.ReasonCode,
					"status":         "released",
					"occurredAt":     s.now(),
				}); err != nil {
				return err
			}
		}

		result = buildReservationResult(released, broadcast)
		return s.repo.CompleteCommand(ctx, tx, stored.ID, CommandConfirmed, result, "", nil)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) loadReleaseResult(ctx context.Context, tx pgx.Tx, reservationID string, out **ReservationResult) error {
	reservation, err := s.repo.GetReservation(ctx, tx, reservationID)
	if err != nil {
		return err
	}
	broadcast, err := s.repo.GetBroadcast(ctx, tx, reservation.RequestID)
	if err != nil {
		return err
	}
	*out = buildReservationResult(reservation, broadcast)
	return nil
}

// CanonicalRequestView is the reconciliation read model.
//
// It deliberately excludes fare_amount and every other commercial field: this
// endpoint exists so fleet can repair projections, not so it can learn what
// the customer paid.
type CanonicalRequestView struct {
	RequestID              string     `json:"requestId"`
	BroadcastID            string     `json:"broadcastId"`
	BookingID              string     `json:"bookingId"`
	BroadcastStatus        string     `json:"broadcastStatus"`
	ReservationStatus      string     `json:"reservationStatus"`
	ReservedOrganizationID *string    `json:"reservedOrganizationId"`
	AcceptedProviderID     *string    `json:"acceptedProviderId"`
	CanonicalTripID        *string    `json:"canonicalTripId"`
	SourceVersion          int64      `json:"sourceVersion"`
	ExpiresAt              *time.Time `json:"expiresAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

// ReadCanonicalRequest returns reconciliation state for a request.
func (s *Service) ReadCanonicalRequest(ctx context.Context, requestID string) (*CanonicalRequestView, error) {
	broadcast, err := s.repo.GetBroadcast(ctx, nil, requestID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, errCoded(404, "request_not_found", "Request not found.")
		}
		return nil, err
	}
	expires := broadcast.ExpiresAt
	return &CanonicalRequestView{
		RequestID:              broadcast.ID,
		BroadcastID:            broadcast.ID,
		BookingID:              broadcast.BookingID,
		BroadcastStatus:        broadcast.Status,
		ReservationStatus:      broadcast.ReservationStatus,
		ReservedOrganizationID: broadcast.ReservedOrganizationID,
		AcceptedProviderID:     broadcast.AcceptedByProviderID,
		SourceVersion:          broadcast.SourceVersion,
		ExpiresAt:              &expires,
		UpdatedAt:              broadcast.UpdatedAt,
	}, nil
}

func buildReservationResult(res *Reservation, broadcast *Broadcast) *ReservationResult {
	result := &ReservationResult{
		ReservationID:            res.ID,
		RequestID:                res.RequestID,
		OrganizationID:           res.OrganizationID,
		Status:                   res.Status,
		AcceptanceSource:         res.AcceptanceSource,
		AcceptingProviderID:      res.AcceptingProviderID,
		OrganizationMembershipID: res.OrganizationMembershipID,
		ReservationVersion:       res.Version,
		ReservedAt:               res.ReservedAt,
		ReleasedAt:               res.ReleasedAt,
		ReleaseReason:            res.ReleaseReason,
	}
	if broadcast != nil {
		result.BroadcastID = broadcast.ID
		result.BookingID = broadcast.BookingID
		result.RequestVersion = broadcast.SourceVersion
	}
	return result
}
