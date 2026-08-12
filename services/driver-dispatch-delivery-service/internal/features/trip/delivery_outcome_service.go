package trip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

// ConfirmRecipient records the authoritative recipient confirmation.
//
// This is a versioned, idempotent command. The order below is deliberate:
//
//  1. shape validation, so a malformed command never reaches the database;
//  2. replay lookup, so a retry returns the original outcome rather than
//     colliding with itself;
//  3. a single transaction that writes the confirmation, denormalises it onto
//     the trip, appends the state log and publishes the canonical event.
//
// The trip's status is NOT changed. Recipient confirmation is a fact about the
// delivery, not a new place in the lifecycle, and giving it its own status
// would mean two columns could disagree about where a delivery is.
func (s *Service) ConfirmRecipient(ctx context.Context,
	input ConfirmRecipientInput) (*ConfirmRecipientResponse, error) {
	if err := validateID(input.TripID, "id"); err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(input.VerificationMethod))
	if !IsValidVerificationMethod(method) {
		return nil, validationError("verification_method",
			"Verification method must be one of SIGNATURE, PHOTO, OTP, ID_CHECK or VERBAL.")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, validationError("idempotency_key", "An idempotency key is required.")
	}

	fingerprint := confirmationFingerprint(input.TripID, input.ProviderID, method,
		input.ExpectedSourceVersion)

	// A retry replays; the same key carrying different inputs is a different
	// command wearing a used key and must not be answered with the old result.
	if existing, err := s.repository.FindRecipientConfirmationByKey(ctx, key); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.TripID != input.TripID {
			return nil, apperrors.Conflict("This idempotency key was already used for another trip.", nil)
		}
		if existing.RequestFingerprint != fingerprint {
			return nil, apperrors.Conflict("This idempotency key was already used for a different request.", nil)
		}
		return confirmationResponse(existing, StatusInProgress, true), nil
	}

	trip, err := s.repository.GetProviderTripByID(ctx, input.TripID, input.ProviderID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	if !CanConfirmRecipient(trip.Status) {
		return nil, apperrors.Conflict("This trip is not in a state where the recipient can confirm delivery.", nil)
	}

	// The confirming party is compared against the booking's expected receiver
	// here, and the comparison inputs go no further. What survives is a
	// boolean verdict and an opaque surrogate.
	matched := recipientMatchesExpected(input.RecipientContact, trip.ReceiverPhone)

	// The proof, when one exists, is referenced rather than duplicated.
	var proofID *string
	if proof, proofErr := s.repository.GetDeliveryProofByTripID(ctx, input.TripID); proofErr == nil && proof != nil {
		id := proof.ID
		proofID = &id
	}

	confirmation, err := s.repository.ConfirmRecipientTx(ctx, ConfirmRecipientDBInput{
		TripID:                   input.TripID,
		ProviderID:               input.ProviderID,
		FromStatus:               trip.Status,
		VerificationMethod:       method,
		RecipientReference:       newRecipientReference(),
		RecipientMatchedExpected: matched,
		ProofID:                  proofID,
		ExpectedSourceVersion:    input.ExpectedSourceVersion,
		IdempotencyKey:           key,
		RequestFingerprint:       fingerprint,
		CorrelationID:            input.CorrelationID,
		CausationID:              input.CausationID,
		ActorType:                "provider",
		ActorID:                  input.ProviderID,
		Now:                      s.now(),
	})
	if err != nil {
		return nil, err
	}
	if confirmation == nil {
		// The transaction found the trip in a state it could not confirm — a
		// concurrent transition won the race.
		return nil, apperrors.Conflict("This trip is not in a state where the recipient can confirm delivery.", nil)
	}
	return confirmationResponse(confirmation, trip.Status, false), nil
}

// FailDelivery records the authoritative delivery failure.
//
// The classification — category, retry eligibility, recipient-unavailable — is
// derived from the reason code on the server. A provider may say WHY a
// delivery failed; they may not say whether it can be re-attempted, because a
// caller who could would be deciding their own consequences.
func (s *Service) FailDelivery(ctx context.Context,
	input FailDeliveryInput) (*FailDeliveryResponse, error) {
	if err := validateID(input.TripID, "id"); err != nil {
		return nil, err
	}
	reason := strings.ToUpper(strings.TrimSpace(input.ReasonCode))
	classification, ok := ClassifyFailure(reason)
	if !ok {
		return nil, validationError("reason_code", "That failure reason is not recognised.")
	}
	notes := strings.TrimSpace(input.OperatorNotes)
	if classification.RequiresNote && notes == "" {
		return nil, validationError("operator_notes", "Please explain what happened.")
	}
	if len(notes) > MaxFailureNoteLength {
		return nil, validationError("operator_notes", "The note is too long.")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, validationError("idempotency_key", "An idempotency key is required.")
	}

	fingerprint := failureFingerprint(input.TripID, input.ProviderID, reason,
		input.ExpectedSourceVersion)

	if existing, err := s.repository.FindDeliveryFailureByKey(ctx, key); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.TripID != input.TripID {
			return nil, apperrors.Conflict("This idempotency key was already used for another trip.", nil)
		}
		if existing.RequestFingerprint != fingerprint {
			return nil, apperrors.Conflict("This idempotency key was already used for a different request.", nil)
		}
		return failureResponse(existing, true), nil
	}

	trip, err := s.repository.GetProviderTripByID(ctx, input.TripID, input.ProviderID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	if !CanFailDelivery(trip.Status) {
		return nil, ValidateTransition(trip.Status, StatusFailed)
	}

	failure, err := s.repository.FailDeliveryTx(ctx, FailDeliveryDBInput{
		TripID:                input.TripID,
		ProviderID:            input.ProviderID,
		FromStatus:            trip.Status,
		ReasonCode:            reason,
		ReasonCategory:        classification.Category,
		OperatorNotes:         notes,
		RetryEligible:         classification.RetryEligible,
		RecipientUnavailable:  classification.RecipientUnavailable,
		AttemptReference:      strings.TrimSpace(input.AttemptReference),
		IncidentReference:     strings.TrimSpace(input.IncidentReference),
		ExpectedSourceVersion: input.ExpectedSourceVersion,
		IdempotencyKey:        key,
		RequestFingerprint:    fingerprint,
		CorrelationID:         input.CorrelationID,
		CausationID:           input.CausationID,
		ActorType:             "provider",
		ActorID:               input.ProviderID,
		Now:                   s.now(),
	})
	if err != nil {
		return nil, err
	}
	if failure == nil {
		return nil, ValidateTransition(trip.Status, StatusFailed)
	}
	return failureResponse(failure, false), nil
}

// GetRecipientConfirmation returns a provider's own trip confirmation.
func (s *Service) GetRecipientConfirmation(ctx context.Context, tripID string,
	providerID string) (*RecipientConfirmation, error) {
	if err := validateID(tripID, "id"); err != nil {
		return nil, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	confirmation, err := s.repository.GetRecipientConfirmation(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if confirmation == nil {
		return nil, apperrors.NotFound("This delivery has not been confirmed.", nil)
	}
	return confirmation, nil
}

// newRecipientReference mints the opaque per-confirmation surrogate.
//
// Random rather than derived from the recipient's contact details on purpose:
// a hash of a phone number is guessable from a phone number, and the whole
// point of this field is that it can travel on an event stream safely.
func newRecipientReference() string {
	return "rcp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// recipientMatchesExpected reports whether the confirming party is the
// receiver the booking named.
//
// An empty contact means the provider did not capture one, which is not a
// match — silence must not read as confirmation of identity.
func recipientMatchesExpected(provided string, expected string) bool {
	normalizedProvided, err := phonenumber.NormalizeNigerianPhoneNumber(strings.TrimSpace(provided))
	if err != nil {
		return false
	}
	normalizedExpected, err := phonenumber.NormalizeNigerianPhoneNumber(strings.TrimSpace(expected))
	if err != nil {
		return false
	}
	return normalizedProvided == normalizedExpected
}

func confirmationFingerprint(tripID, providerID, method string, expectedVersion int64) string {
	return commandFingerprint("recipient-confirmation", tripID, providerID, method, expectedVersion)
}

func failureFingerprint(tripID, providerID, reason string, expectedVersion int64) string {
	return commandFingerprint("delivery-failure", tripID, providerID, reason, expectedVersion)
}

// commandFingerprint hashes the inputs that define a command.
//
// expectedVersion participates: a client that changes the version it claims to
// be acting on has issued a different command, and answering it with the
// earlier one's result would hide a real conflict behind a replay.
func commandFingerprint(kind, tripID, providerID, discriminator string, expectedVersion int64) string {
	hasher := sha256.New()
	for _, part := range []string{kind, tripID, providerID, discriminator} {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	_, _ = hasher.Write([]byte{byte(expectedVersion), byte(expectedVersion >> 8),
		byte(expectedVersion >> 16), byte(expectedVersion >> 24),
		byte(expectedVersion >> 32), byte(expectedVersion >> 40),
		byte(expectedVersion >> 48), byte(expectedVersion >> 56)})
	return hex.EncodeToString(hasher.Sum(nil))
}

func confirmationResponse(confirmation *RecipientConfirmation, status TripStatus,
	replayed bool) *ConfirmRecipientResponse {
	if confirmation.FromStatus != "" {
		status = TripStatus(confirmation.FromStatus)
	}
	return &ConfirmRecipientResponse{
		TripID:                   confirmation.TripID,
		Status:                   status,
		VerificationMethod:       confirmation.VerificationMethod,
		RecipientReference:       confirmation.RecipientReference,
		RecipientMatchedExpected: confirmation.RecipientMatchedExpected,
		ConfirmedAt:              confirmation.ConfirmedAt,
		SourceVersion:            confirmation.SourceVersion,
		Replayed:                 replayed,
		Message:                  "Recipient confirmation recorded.",
	}
}

func failureResponse(failure *DeliveryFailure, replayed bool) *FailDeliveryResponse {
	return &FailDeliveryResponse{
		TripID:               failure.TripID,
		Status:               StatusFailed,
		ReasonCode:           failure.ReasonCode,
		ReasonCategory:       failure.ReasonCategory,
		RetryEligible:        failure.RetryEligible,
		RecipientUnavailable: failure.RecipientUnavailable,
		FailedAt:             failure.FailedAt,
		SourceVersion:        failure.SourceVersion,
		Replayed:             replayed,
		Message:              "Delivery failure recorded.",
	}
}
