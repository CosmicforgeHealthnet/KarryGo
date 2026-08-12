package trip

import (
	"strings"
	"time"
)

// ── WP6.3: recipient confirmation ────────────────────────────────────────────

// Recipient verification methods.
//
// A closed set: an unrecognised method would let a client describe how it
// verified the recipient in words nobody downstream can act on.
const (
	VerificationSignature = "SIGNATURE"
	VerificationPhoto     = "PHOTO"
	VerificationOTP       = "OTP"
	VerificationIDCheck   = "ID_CHECK"
	VerificationVerbal    = "VERBAL"
)

var validVerificationMethods = map[string]struct{}{
	VerificationSignature: {},
	VerificationPhoto:     {},
	VerificationOTP:       {},
	VerificationIDCheck:   {},
	VerificationVerbal:    {},
}

// IsValidVerificationMethod reports whether a method is recognised.
func IsValidVerificationMethod(method string) bool {
	_, ok := validVerificationMethods[strings.ToUpper(strings.TrimSpace(method))]
	return ok
}

// ConfirmableStatuses are the trip states in which a recipient confirmation is
// meaningful.
//
// in_progress and proof_submitted only. Before in_progress the package has not
// reached anyone; after completion the delivery is closed and a late
// "confirmation" would be a rewrite of a settled fact.
func CanConfirmRecipient(status TripStatus) bool {
	return status == StatusInProgress || status == StatusProofSubmitted
}

// RecipientConfirmation is the authoritative record that a delivery reached
// its recipient.
//
// It carries no recipient name, phone or address. RecipientReference is an
// opaque per-confirmation surrogate, and RecipientMatchedExpected is the
// server's verdict on whether the confirming party was the receiver the
// booking named — computed from data that stays behind.
type RecipientConfirmation struct {
	ID                       string     `json:"id"`
	TripID                   string     `json:"trip_id"`
	VerificationMethod       string     `json:"verification_method"`
	RecipientReference       string     `json:"recipient_reference"`
	RecipientMatchedExpected bool       `json:"recipient_matched_expected"`
	ProofID                  *string    `json:"proof_id,omitempty"`
	FromStatus               string     `json:"from_status"`
	ConfirmedAt              time.Time  `json:"confirmed_at"`
	ConfirmedByActorType     string     `json:"confirmed_by_actor_type"`
	ConfirmedByActorID       string     `json:"confirmed_by_actor_id"`
	ExpectedSourceVersion    int64      `json:"expected_source_version"`
	IdempotencyKey           string     `json:"idempotency_key"`
	RequestFingerprint       string     `json:"request_fingerprint"`
	CorrelationID            string     `json:"correlation_id"`
	CausationID              string     `json:"causation_id"`
	CreatedAt                time.Time  `json:"created_at"`
	Replayed                 bool       `json:"-"`
	SourceVersion            int64      `json:"-"`
	TripUpdatedAt            *time.Time `json:"-"`
}

// ConfirmRecipientInput is the versioned command.
type ConfirmRecipientInput struct {
	TripID             string
	ProviderID         string
	VerificationMethod string
	// RecipientContact is compared against the booking's expected receiver and
	// then discarded. It is never stored and never published.
	RecipientContact      string
	ExpectedSourceVersion int64
	IdempotencyKey        string
	CorrelationID         string
	CausationID           string
}

// ConfirmRecipientResponse is returned by the provider route.
type ConfirmRecipientResponse struct {
	TripID                   string     `json:"trip_id"`
	Status                   TripStatus `json:"status"`
	VerificationMethod       string     `json:"verification_method"`
	RecipientReference       string     `json:"recipient_reference"`
	RecipientMatchedExpected bool       `json:"recipient_matched_expected"`
	ConfirmedAt              time.Time  `json:"confirmed_at"`
	SourceVersion            int64      `json:"source_version"`
	Replayed                 bool       `json:"replayed"`
	Message                  string     `json:"message"`
}

// ── WP6.4: failed delivery ───────────────────────────────────────────────────

// Failure reason codes.
const (
	FailureRecipientUnavailable = "RECIPIENT_UNAVAILABLE"
	FailureRecipientRefused     = "RECIPIENT_REFUSED"
	FailureWrongAddress         = "WRONG_ADDRESS"
	FailureAddressInaccessible  = "ADDRESS_INACCESSIBLE"
	FailurePackageDamaged       = "PACKAGE_DAMAGED"
	FailurePackageLost          = "PACKAGE_LOST"
	FailureProviderIncident     = "PROVIDER_INCIDENT"
	FailureVehicleBreakdown     = "VEHICLE_BREAKDOWN"
	FailureWeatherOrRoad        = "WEATHER_OR_ROAD"
	FailureSecurityRisk         = "SECURITY_RISK"
	FailureOther                = "OTHER"
)

// Failure categories, which is what consumers branch on.
const (
	FailureCategoryRecipient = "RECIPIENT"
	FailureCategoryAddress   = "ADDRESS"
	FailureCategoryPackage   = "PACKAGE"
	FailureCategoryProvider  = "PROVIDER"
	FailureCategoryExternal  = "EXTERNAL"
	FailureCategoryOther     = "OTHER"
)

// failureReasons maps each reason onto its category and whether another
// attempt is permitted.
//
// Retry eligibility is derived here rather than accepted from the caller. A
// provider who could declare their own failure re-attemptable could keep a
// delivery alive indefinitely; a provider who could declare a lost package
// re-attemptable would hide a loss behind a retry.
var failureReasons = map[string]struct {
	Category             string
	RetryEligible        bool
	RecipientUnavailable bool
	RequiresNote         bool
}{
	FailureRecipientUnavailable: {FailureCategoryRecipient, true, true, false},
	FailureRecipientRefused:     {FailureCategoryRecipient, false, false, false},
	FailureWrongAddress:         {FailureCategoryAddress, true, false, false},
	FailureAddressInaccessible:  {FailureCategoryAddress, true, false, false},
	FailurePackageDamaged:       {FailureCategoryPackage, false, false, true},
	FailurePackageLost:          {FailureCategoryPackage, false, false, true},
	FailureProviderIncident:     {FailureCategoryProvider, false, false, true},
	FailureVehicleBreakdown:     {FailureCategoryProvider, true, false, false},
	FailureWeatherOrRoad:        {FailureCategoryExternal, true, false, false},
	FailureSecurityRisk:         {FailureCategoryExternal, false, false, true},
	FailureOther:                {FailureCategoryOther, false, false, true},
}

// MaxFailureNoteLength bounds operator notes.
const MaxFailureNoteLength = 500

// IsValidFailureReason reports whether a reason code is recognised.
func IsValidFailureReason(code string) bool {
	_, ok := failureReasons[strings.ToUpper(strings.TrimSpace(code))]
	return ok
}

// FailureClassification is the server's derived verdict on a failure reason.
type FailureClassification struct {
	Category             string
	RetryEligible        bool
	RecipientUnavailable bool
	RequiresNote         bool
}

// ClassifyFailure returns the server-owned classification for a reason code.
func ClassifyFailure(code string) (FailureClassification, bool) {
	entry, ok := failureReasons[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return FailureClassification{}, false
	}
	return FailureClassification{
		Category:             entry.Category,
		RetryEligible:        entry.RetryEligible,
		RecipientUnavailable: entry.RecipientUnavailable,
		RequiresNote:         entry.RequiresNote,
	}, true
}

// CanFailDelivery reports whether a trip may still be failed.
//
// Mirrors validTransitions rather than restating it: `failed` is reachable
// from every live state, and from none of the terminal ones.
func CanFailDelivery(status TripStatus) bool {
	return CanTransition(status, StatusFailed)
}

// DeliveryFailure is the authoritative record of a delivery that could not be
// completed.
type DeliveryFailure struct {
	ID                    string    `json:"id"`
	TripID                string    `json:"trip_id"`
	ReasonCode            string    `json:"reason_code"`
	ReasonCategory        string    `json:"reason_category"`
	OperatorNotes         *string   `json:"operator_notes,omitempty"`
	FromStatus            string    `json:"from_status"`
	FailedAt              time.Time `json:"failed_at"`
	ResponsibleActorType  string    `json:"responsible_actor_type"`
	ResponsibleActorID    string    `json:"responsible_actor_id"`
	RetryEligible         bool      `json:"retry_eligible"`
	RecipientUnavailable  bool      `json:"recipient_unavailable"`
	AttemptReference      *string   `json:"attempt_reference,omitempty"`
	IncidentReference     *string   `json:"incident_reference,omitempty"`
	ExpectedSourceVersion int64     `json:"expected_source_version"`
	IdempotencyKey        string    `json:"idempotency_key"`
	RequestFingerprint    string    `json:"request_fingerprint"`
	CorrelationID         string    `json:"correlation_id"`
	CausationID           string    `json:"causation_id"`
	CreatedAt             time.Time `json:"created_at"`
	Replayed              bool      `json:"-"`
	SourceVersion         int64     `json:"-"`
}

// FailDeliveryInput is the versioned command.
type FailDeliveryInput struct {
	TripID                string
	ProviderID            string
	ReasonCode            string
	OperatorNotes         string
	AttemptReference      string
	IncidentReference     string
	ExpectedSourceVersion int64
	IdempotencyKey        string
	CorrelationID         string
	CausationID           string
}

// FailDeliveryResponse is returned by the provider route.
type FailDeliveryResponse struct {
	TripID               string     `json:"trip_id"`
	Status               TripStatus `json:"status"`
	ReasonCode           string     `json:"reason_code"`
	ReasonCategory       string     `json:"reason_category"`
	RetryEligible        bool       `json:"retry_eligible"`
	RecipientUnavailable bool       `json:"recipient_unavailable"`
	FailedAt             time.Time  `json:"failed_at"`
	SourceVersion        int64      `json:"source_version"`
	Replayed             bool       `json:"replayed"`
	Message              string     `json:"message"`
}

// ── Shared command plumbing ──────────────────────────────────────────────────

// ConfirmRecipientDBInput carries the resolved command into the transaction.
type ConfirmRecipientDBInput struct {
	TripID                   string
	ProviderID               string
	FromStatus               TripStatus
	VerificationMethod       string
	RecipientReference       string
	RecipientMatchedExpected bool
	ProofID                  *string
	ExpectedSourceVersion    int64
	IdempotencyKey           string
	RequestFingerprint       string
	CorrelationID            string
	CausationID              string
	ActorType                string
	ActorID                  string
	Now                      time.Time
}

// FailDeliveryDBInput carries the resolved command into the transaction.
type FailDeliveryDBInput struct {
	TripID                string
	ProviderID            string
	FromStatus            TripStatus
	ReasonCode            string
	ReasonCategory        string
	OperatorNotes         string
	RetryEligible         bool
	RecipientUnavailable  bool
	AttemptReference      string
	IncidentReference     string
	ExpectedSourceVersion int64
	IdempotencyKey        string
	RequestFingerprint    string
	CorrelationID         string
	CausationID           string
	ActorType             string
	ActorID               string
	Now                   time.Time
}
