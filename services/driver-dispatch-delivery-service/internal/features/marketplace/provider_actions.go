package marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Provider action outcome states returned to the provider application.
const (
	ProviderActionConfirmed = "CONFIRMED"
	// ProviderActionPending means the decision is durably recorded here and
	// will be delivered to fleet by the relay worker. It is NOT a failure:
	// the provider's answer is safe and they must not be asked again.
	ProviderActionPending  = "PENDING"
	ProviderActionRejected = "REJECTED"
)

// ProviderDecision is a provider answering an offer or an assignment.
//
// ProviderID is deliberately absent: it is supplied separately by the handler
// from the verified session and can never be set from a request body.
type ProviderDecision struct {
	OfferID          string
	AssignmentID     string
	ClientResponseID string
	ExpectedVersion  int64
	ReasonCode       string
	Note             string
	EvidenceFileIDs  []string
	RequestID        string
	CorrelationID    string
}

// ProviderActionResult is the stable answer for a provider decision.
type ProviderActionResult struct {
	Status       string `json:"status"`
	RelayID      string `json:"relayId"`
	OfferID      string `json:"offerId,omitempty"`
	AssignmentID string `json:"assignmentId,omitempty"`
	// ErrorCode is set only when fleet permanently refused the decision.
	ErrorCode string `json:"errorCode,omitempty"`
	// ReservationID is set on the delegated path, where dispatch also claimed
	// the canonical request for the provider's organization.
	ReservationID string `json:"reservationId,omitempty"`
	Replayed      bool   `json:"replayed"`
}

// decisionPayload is the body relayed to fleet.
//
// providerId here comes from the verified dispatch session, never from the
// provider's client. Fleet re-verifies that this provider actually owns the
// offer or assignment, so a compromised dispatch would still not let one
// provider answer for another.
type decisionPayload struct {
	ProviderID            string   `json:"providerId"`
	ProviderService       string   `json:"providerService"`
	ProviderSessionID     string   `json:"providerSessionId"`
	ClientResponseID      string   `json:"clientResponseId"`
	ExpectedVersion       int64    `json:"expectedVersion,omitempty"`
	ReasonCode            string   `json:"reasonCode,omitempty"`
	Note                  string   `json:"note,omitempty"`
	EvidenceFileIDs       []string `json:"evidenceFileIds,omitempty"`
	DispatchReservationID string   `json:"dispatchReservationId,omitempty"`
}

// SubmitProviderDecision records a provider's decision durably and attempts
// immediate delivery to fleet.
//
// The provider's HTTP request never waits for canonical trip creation. It waits
// only for the decision to become durable here plus one delivery attempt, so a
// slow or unavailable fleet degrades to a PENDING answer rather than a failure
// the provider would be tempted to retry as a new decision.
func (s *Service) SubmitProviderDecision(ctx context.Context, operationType string,
	providerID string, sessionID string, decision ProviderDecision) (*ProviderActionResult, error) {
	if providerID == "" || sessionID == "" {
		// Only reachable through a programming error: the handler derives both
		// from the verified session.
		return nil, errCoded(401, "unauthorized", "A verified provider session is required.")
	}
	if decision.ClientResponseID == "" {
		return nil, errCoded(400, "validation_error", "clientResponseId is required.")
	}
	if isDeclineOperation(operationType) {
		if decision.ReasonCode == "" {
			return nil, errCoded(400, "decline_reason_required", "A decline reason is required.")
		}
		if !ValidDeclineReason(decision.ReasonCode) {
			return nil, errCoded(400, "decline_reason_invalid",
				"That decline reason is not recognised.")
		}
		if decision.ReasonCode == DeclineOther && decision.Note == "" {
			return nil, errCoded(400, "decline_note_required",
				"Please explain the reason for declining.")
		}
	}

	payload := decisionPayload{
		ProviderID:        providerID,
		ProviderService:   "DISPATCH",
		ProviderSessionID: sessionID,
		ClientResponseID:  decision.ClientResponseID,
		ExpectedVersion:   decision.ExpectedVersion,
		ReasonCode:        decision.ReasonCode,
		Note:              decision.Note,
		EvidenceFileIDs:   decision.EvidenceFileIDs,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errCoded(400, "validation_error", "The decision could not be encoded.")
	}

	// One decision, one identity. The key is the provider's own client
	// response id; its namespace is (provider_id, idempotency_key), enforced
	// by uq_dispatch_relay_single_decision. Prefixing the provider id into the
	// key itself would make the invariant depend on this call site rather than
	// on the schema.
	idempotencyKey := decision.ClientResponseID
	fingerprint := Fingerprint(payload)

	operation := &RelayOperation{
		OperationType:      operationType,
		ProviderID:         providerID,
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		Payload:            encoded,
	}
	if decision.OfferID != "" {
		operation.OfferID = &decision.OfferID
	}
	if decision.AssignmentID != "" {
		operation.AssignmentID = &decision.AssignmentID
	}

	var stored *RelayOperation
	var replayed bool
	if err := s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		// One client response id is one decision. The unique index is scoped
		// per operation type, so without this check the same id could record
		// an accept AND a decline as two valid relays.
		if existing, lookupErr := s.repo.GetRelayByKeyAnyType(ctx, tx, providerID, idempotencyKey); lookupErr == nil {
			if existing.OperationType != operationType {
				return ErrIdempotencyConflict
			}
		} else if !errors.Is(lookupErr, ErrNotFound) {
			return lookupErr
		}

		var createErr error
		stored, replayed, createErr = s.repo.CreateRelay(ctx, tx, operation)
		return createErr
	}); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return nil, errIdempotencyConflict()
		}
		return nil, err
	}

	// A decision already settled is answered from storage: never re-delivered,
	// never duplicated.
	if replayed && stored.Terminal() {
		return terminalResult(stored, decision, true), nil
	}

	return s.attemptImmediateDelivery(ctx, stored, decision, replayed), nil
}

// attemptImmediateDelivery makes one best-effort delivery so a healthy fleet
// gives the provider an immediate confirmation.
//
// Every outcome leaves the relay in a state the worker can carry forward; none
// of them loses the decision.
func (s *Service) attemptImmediateDelivery(ctx context.Context, stored *RelayOperation,
	decision ProviderDecision, replayed bool) *ProviderActionResult {
	result := &ProviderActionResult{
		Status: ProviderActionPending, RelayID: stored.ID,
		OfferID: decision.OfferID, AssignmentID: decision.AssignmentID, Replayed: replayed,
	}
	if s.responder == nil || !s.responder.Configured() {
		// Fail closed: durably queued, never a fabricated success.
		return result
	}

	meta := CallMeta{RequestID: decision.RequestID, CorrelationID: decision.CorrelationID}
	claimed, claimErr := s.repo.ClaimRelayForImmediateDelivery(ctx, stored.ID)
	if claimErr != nil || !claimed {
		return result
	}
	delivery, err := s.responder.Deliver(ctx, meta, *stored)
	if err != nil {
		// Unknown: fleet may already hold the decision. Keep the original key.
		_ = s.repo.MarkRelayUnknown(ctx, nil, stored.ID, "fleet_service_unavailable",
			5*time.Second, s.relayMaxAttempts())
		return result
	}

	switch {
	case delivery.Accepted:
		if err := s.repo.ConfirmRelay(ctx, nil, stored.ID, delivery.Snapshot); err != nil {
			return result
		}
		result.Status = ProviderActionConfirmed
	case delivery.Permanent:
		if err := s.repo.FailRelay(ctx, nil, stored.ID, delivery.ErrorCode, 0, s.relayMaxAttempts(), true); err != nil {
			return result
		}
		result.Status = ProviderActionRejected
		result.ErrorCode = delivery.ErrorCode
	default:
		// Temporary: leave it retryable and tell the provider it is pending.
		_ = s.repo.FailRelay(ctx, nil, stored.ID, delivery.ErrorCode,
			5*time.Second, s.relayMaxAttempts(), false)
	}
	return result
}

func terminalResult(stored *RelayOperation, decision ProviderDecision, replayed bool) *ProviderActionResult {
	result := &ProviderActionResult{
		RelayID: stored.ID, OfferID: decision.OfferID,
		AssignmentID: decision.AssignmentID, Replayed: replayed,
	}
	switch stored.Status {
	case RelayConfirmed:
		result.Status = ProviderActionConfirmed
	case RelayRejected:
		result.Status = ProviderActionRejected
		if stored.LastError != nil {
			result.ErrorCode = *stored.LastError
		}
	default:
		result.Status = ProviderActionPending
	}
	return result
}

func (s *Service) relayMaxAttempts() int {
	if s.relayAttempts > 0 {
		return s.relayAttempts
	}
	return 10
}

func isDeclineOperation(operationType string) bool {
	return operationType == RelayProviderOfferDecline ||
		operationType == RelayProviderAssignmentDecline
}

// Decline reason catalogue, mirrored from the fleet contract so dispatch can
// reject an invalid reason before it ever reaches the relay.
const (
	DeclineIllness              = "ILLNESS"
	DeclineFatigueOrRest        = "FATIGUE_OR_REST_REQUIRED"
	DeclineShiftEnding          = "SHIFT_ENDING"
	DeclineUnsafeVehicle        = "UNSAFE_VEHICLE"
	DeclineVehicleDefect        = "VEHICLE_DEFECT"
	DeclineExcessiveDistance    = "EXCESSIVE_DISTANCE"
	DeclineLocationMismatch     = "LOCATION_MISMATCH"
	DeclineServiceMismatch      = "SERVICE_MISMATCH"
	DeclineCapacityMismatch     = "CAPACITY_MISMATCH"
	DeclineCargoOrSafetyConcern = "CARGO_OR_SAFETY_CONCERN"
	DeclineSecurityConcern      = "SECURITY_CONCERN"
	DeclinePersonalEmergency    = "PERSONAL_EMERGENCY"
	DeclineAlreadyEngaged       = "ALREADY_ENGAGED"
	DeclineDeviceOrNetworkIssue = "DEVICE_OR_NETWORK_ISSUE"
	DeclineOther                = "OTHER"
)

var declineReasons = map[string]bool{
	DeclineIllness: true, DeclineFatigueOrRest: true, DeclineShiftEnding: true,
	DeclineUnsafeVehicle: true, DeclineVehicleDefect: true, DeclineExcessiveDistance: true,
	DeclineLocationMismatch: true, DeclineServiceMismatch: true, DeclineCapacityMismatch: true,
	DeclineCargoOrSafetyConcern: true, DeclineSecurityConcern: true,
	DeclinePersonalEmergency: true, DeclineAlreadyEngaged: true,
	DeclineDeviceOrNetworkIssue: true, DeclineOther: true,
}

// ValidDeclineReason reports whether the reason is in the closed catalogue.
func ValidDeclineReason(reason string) bool { return declineReasons[reason] }

// ProviderReader reads a provider's organization offers and company
// assignments from fleet-partner-service.
//
// Dispatch does NOT own these records — fleet does — so listing them is a
// cross-service read, not a local query. Modelling it as a port keeps that
// boundary explicit and makes the unconfigured case fail closed instead of
// returning an empty list, which a provider app would render as "no work"
// and which would be indistinguishable from genuinely having none.
type ProviderReader interface {
	ListOffers(ctx context.Context, meta CallMeta, providerID string) ([]ProviderOfferSummary, error)
	GetOffer(ctx context.Context, meta CallMeta, providerID string, offerID string) (*ProviderOfferSummary, error)
	ListAssignments(ctx context.Context, meta CallMeta, providerID string) ([]ProviderAssignmentSummary, error)
	GetAssignment(ctx context.Context, meta CallMeta, providerID string, assignmentID string) (*ProviderAssignmentSummary, error)
	Configured() bool
}

// UnconfiguredProviderReader is the fail-closed default.
//
// It reports the fleet read contract as unavailable rather than pretending the
// provider has no work.
type UnconfiguredProviderReader struct{}

// Configured reports false.
func (UnconfiguredProviderReader) Configured() bool { return false }

func (UnconfiguredProviderReader) ListOffers(context.Context, CallMeta, string) ([]ProviderOfferSummary, error) {
	return nil, ErrFleetUnavailable
}

func (UnconfiguredProviderReader) GetOffer(context.Context, CallMeta, string, string) (*ProviderOfferSummary, error) {
	return nil, ErrFleetUnavailable
}

func (UnconfiguredProviderReader) ListAssignments(context.Context, CallMeta, string) ([]ProviderAssignmentSummary, error) {
	return nil, ErrFleetUnavailable
}

func (UnconfiguredProviderReader) GetAssignment(context.Context, CallMeta, string, string) (*ProviderAssignmentSummary, error) {
	return nil, ErrFleetUnavailable
}

// WithProviderReader attaches the fleet read port.
func (s *Service) WithProviderReader(reader ProviderReader) *Service {
	s.providerReader = reader
	return s
}

func (s *Service) reader() ProviderReader {
	if s.providerReader == nil {
		return UnconfiguredProviderReader{}
	}
	return s.providerReader
}

// ListProviderOffers returns the offers addressed to this provider.
func (s *Service) ListProviderOffers(ctx context.Context, providerID string) ([]ProviderOfferSummary, error) {
	offers, err := s.reader().ListOffers(ctx, CallMeta{}, providerID)
	if err != nil {
		return nil, errCoded(503, "fleet_service_unavailable",
			"Organization offers cannot be loaded right now.")
	}
	return offers, nil
}

// GetProviderOffer returns one offer belonging to this provider.
func (s *Service) GetProviderOffer(ctx context.Context, providerID string, offerID string) (*ProviderOfferSummary, error) {
	offer, err := s.reader().GetOffer(ctx, CallMeta{}, providerID, offerID)
	if err != nil {
		return nil, errCoded(503, "fleet_service_unavailable",
			"That offer cannot be loaded right now.")
	}
	return offer, nil
}

// ListProviderAssignments returns this provider's company assignments.
func (s *Service) ListProviderAssignments(ctx context.Context, providerID string) ([]ProviderAssignmentSummary, error) {
	assignments, err := s.reader().ListAssignments(ctx, CallMeta{}, providerID)
	if err != nil {
		return nil, errCoded(503, "fleet_service_unavailable",
			"Company assignments cannot be loaded right now.")
	}
	return assignments, nil
}

// GetProviderAssignment returns one assignment belonging to this provider.
func (s *Service) GetProviderAssignment(ctx context.Context, providerID string, assignmentID string) (*ProviderAssignmentSummary, error) {
	assignment, err := s.reader().GetAssignment(ctx, CallMeta{}, providerID, assignmentID)
	if err != nil {
		return nil, errCoded(503, "fleet_service_unavailable",
			"That assignment cannot be loaded right now.")
	}
	return assignment, nil
}

// DelegatedOfferInput is a provider accepting an organization offer directly.
type DelegatedOfferInput struct {
	OfferID         string
	RequestID       string
	OrganizationID  string
	MembershipID    string
	ExpectedVersion int64
	Decision        ProviderDecision
}

// AcceptOfferDelegated reserves the canonical request for the accepting
// provider AND records their decision in ONE dispatch transaction.
//
// The atomicity is the point. If the reservation committed first and the relay
// second, a crash between them would leave the request claimed by an
// organization with no record of who accepted it or why — the provider's
// decision silently lost while the request looks taken. Committing them
// together means either both exist or neither does.
func (s *Service) AcceptOfferDelegated(ctx context.Context, providerID string,
	sessionID string, input DelegatedOfferInput) (*ProviderActionResult, error) {
	if providerID == "" || sessionID == "" {
		return nil, errCoded(401, "unauthorized", "A verified provider session is required.")
	}
	if input.Decision.ClientResponseID == "" {
		return nil, errCoded(400, "validation_error", "clientResponseId is required.")
	}
	if input.RequestID == "" || input.OrganizationID == "" || input.MembershipID == "" {
		return nil, errCoded(400, "validation_error",
			"requestId, organizationId and organizationMembershipId are required.")
	}
	if !s.flags.ReservationEnabled {
		return nil, errCoded(503, "source_service_unavailable",
			"Organization reservation is not enabled.")
	}

	// Authoritative eligibility, from dispatch's own records. Organization
	// sponsorship never bypasses suspension or verification.
	eligibility, err := s.ValidateProvider(ctx, ProviderEligibilityInput{
		RequestID: input.RequestID, OrganizationID: input.OrganizationID,
		ProviderID: providerID, OrganizationMembershipID: input.MembershipID,
		// The reservation does not exist yet — creating it is the next step,
		// in the same transaction as this provider's decision.
		SkipReservationCheck: true,
	})
	if err != nil {
		return nil, err
	}
	if !eligibility.Eligible {
		code := "provider_not_eligible"
		if len(eligibility.Reasons) > 0 {
			code = eligibility.Reasons[0]
		}
		return nil, errCoded(409, code, "You cannot take this work right now.")
	}

	payload := decisionPayload{
		ProviderID:        providerID,
		ProviderService:   "DISPATCH",
		ProviderSessionID: sessionID,
		ClientResponseID:  input.Decision.ClientResponseID,
		ExpectedVersion:   input.ExpectedVersion,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errCoded(400, "validation_error", "The decision could not be encoded.")
	}

	reserveInput := ReserveInput{
		RequestID:                input.RequestID,
		OrganizationID:           input.OrganizationID,
		AcceptanceSource:         AcceptanceProviderDirect,
		AcceptingProviderID:      providerID,
		OrganizationMembershipID: input.MembershipID,
		// The reservation and the relay share the provider's client response
		// id, so a retried tap replays BOTH rather than reserving twice.
		IdempotencyKey: input.Decision.ClientResponseID,
	}
	reserveFP := Fingerprint(reserveFingerprint{
		RequestID: input.RequestID, OrganizationID: input.OrganizationID,
		AcceptanceSource: AcceptanceProviderDirect, ProviderID: providerID,
		MembershipID: input.MembershipID,
	})

	relay := &RelayOperation{
		OperationType:      RelayProviderOfferAccept,
		ProviderID:         providerID,
		OfferID:            &input.OfferID,
		IdempotencyKey:     input.Decision.ClientResponseID,
		RequestFingerprint: Fingerprint(payload),
		Payload:            encoded,
	}
	if input.RequestID != "" {
		requestID := input.RequestID
		relay.RequestID = &requestID
	}

	var stored *RelayOperation
	var replayed bool
	var reservation *ReservationResult
	if err := s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		// One decision per provider per client response id, across kinds.
		if existing, lookupErr := s.repo.GetRelayByKeyAnyType(ctx, tx, providerID,
			relay.IdempotencyKey); lookupErr == nil {
			if existing.OperationType != relay.OperationType {
				return ErrIdempotencyConflict
			}
		} else if !errors.Is(lookupErr, ErrNotFound) {
			return lookupErr
		}

		// Reservation first: if the request is already claimed there is no
		// decision to record, and the provider must be told immediately.
		var reserveErr error
		reservation, reserveErr = s.reserveWithinTx(ctx, tx, reserveInput, reserveFP)
		if reserveErr != nil {
			return reserveErr
		}
		// The relay must carry the reservation created in this same transaction.
		// Fleet persists that identifier on its immutable acceptance and the
		// execution-start worker later presents it back to dispatch.
		payload.DispatchReservationID = reservation.ReservationID
		var marshalErr error
		encoded, marshalErr = json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		relay.Payload = encoded
		relay.RequestFingerprint = Fingerprint(payload)

		var createErr error
		stored, replayed, createErr = s.repo.CreateRelay(ctx, tx, relay)
		return createErr
	}); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return nil, errIdempotencyConflict()
		}
		return nil, err
	}

	if replayed && stored.Terminal() {
		return terminalResult(stored, input.Decision, true), nil
	}

	// Delivery is attempted only AFTER the commit, so a slow or unavailable
	// fleet degrades to PENDING with the reservation and decision both safe.
	result := s.attemptImmediateDelivery(ctx, stored, input.Decision, replayed)
	result.OfferID = input.OfferID
	if reservation != nil {
		result.ReservationID = reservation.ReservationID
	}
	return result, nil
}
