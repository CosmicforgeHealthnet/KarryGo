// Package referralcontract is the wire contract between referral-service and
// the services that feed it: payment-wallet-service (paid jobs) and
// customer-service, driver-hauling-service, driver-dispatch-delivery-service
// and driver-taxi-service (accounts).
//
// Producers write these events to their own outbox table and relay them to
// referral-service with shared/go/outbox. This package is schema only; it
// holds no referral business rules.
package referralcontract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Event types.
const (
	// EventJobSettled is appended by payment-wallet-service inside the same
	// transaction that pays a job out of escrow.
	EventJobSettled = "finance.job.settled.v1"

	// EventJobSettlementReversed is the contract for a paid-out job being
	// refunded, charged back or reversed. The wallet cannot refund a job after
	// payout today (only held payments are refundable), so nothing produces it
	// yet; referral-service handles it when a producer appears.
	EventJobSettlementReversed = "finance.job.settlement_reversed.v1"

	// EventCustomerRegistered and EventProviderRegistered are appended when an
	// account is created (IsNewAccount=true) or re-announced by a one-time
	// backfill of accounts that predate the referral programme
	// (IsNewAccount=false).
	EventCustomerRegistered = "identity.customer.registered.v1"
	EventProviderRegistered = "identity.provider.registered.v1"

	// EventProviderOnboardingCompleted is appended when a driver becomes fully
	// approved to work.
	EventProviderOnboardingCompleted = "identity.provider.onboarding_completed.v1"
)

// IngestPath is where referral-service receives relayed events.
const IngestPath = "/internal/referral/v1/events"

// OutboxTable is the conventional producer-side outbox table name.
const OutboxTable = "referral_outbox"

// Actor types: one per kind of account the programme covers.
const (
	ActorCustomer = "customer"
	ActorHauling  = "hauling"
	ActorDispatch = "dispatch"
	ActorTaxi     = "taxi"
)

// ValidActorType reports whether t is a programme actor type.
func ValidActorType(t string) bool {
	switch t {
	case ActorCustomer, ActorHauling, ActorDispatch, ActorTaxi:
		return true
	}
	return false
}

// IsProviderActor reports whether t is one of the driver actor types.
func IsProviderActor(t string) bool {
	return t == ActorHauling || t == ActorDispatch || t == ActorTaxi
}

// ActorTypeForWalletProvider maps a payment intent's provider_type to a
// referral actor type. Anything else (vendor businesses, fleet payroll, ...)
// returns "" and is outside the programme.
func ActorTypeForWalletProvider(providerType string) string {
	switch strings.TrimSpace(providerType) {
	case "truck_provider", "hauling":
		return ActorHauling
	case "dispatch", "dispatch_provider":
		return ActorDispatch
	case "taxi", "taxi_provider":
		return ActorTaxi
	}
	return ""
}

// WalletProviderTypeForActor is the inverse of ActorTypeForWalletProvider:
// the provider_type payment-wallet keys a driver's ledger accounts by
// ("<type>:<id>"). Referral credits for a driver must use it, or they land in
// an account the driver's wallet screens never read. "" for non-drivers.
func WalletProviderTypeForActor(actorType string) string {
	switch actorType {
	case ActorHauling:
		return "truck_provider"
	case ActorDispatch:
		return "dispatch"
	case ActorTaxi:
		return "taxi"
	}
	return ""
}

// Where a job's completion time came from.
const (
	CompletedAtFromProvider   = "provider"
	CompletedAtFromSettlement = "settlement"
)

// JobSettled is the payload of EventJobSettled. (SourceService,
// SourceReference) is the job's natural key and the idempotency key for every
// referral effect derived from it.
type JobSettled struct {
	PaymentIntentID  string `json:"paymentIntentId"`
	PaymentReference string `json:"paymentReference"`
	SourceService    string `json:"sourceService"`
	SourceReference  string `json:"sourceReference"`
	CustomerID       string `json:"customerId"`
	ProviderID       string `json:"providerId"`
	ProviderType     string `json:"providerType"`
	AmountKobo       int64  `json:"amountKobo"`
	// PlatformFeeKobo is the commission the platform actually kept on this job.
	PlatformFeeKobo int64  `json:"platformFeeKobo"`
	ProviderNetKobo int64  `json:"providerNetKobo"`
	Currency        string `json:"currency"`
	// JobCompletedAt drives every time rule (activity windows, the 90-day
	// earning window). SettledAt is audit only.
	JobCompletedAt       time.Time `json:"jobCompletedAt"`
	JobCompletedAtSource string    `json:"jobCompletedAtSource"`
	SettledAt            time.Time `json:"settledAt"`
}

// Validate checks the fields every consumer relies on.
func (p JobSettled) Validate() error {
	if strings.TrimSpace(p.SourceService) == "" || strings.TrimSpace(p.SourceReference) == "" {
		return errors.New("referralcontract: job settled needs a source service and reference")
	}
	if p.AmountKobo <= 0 || p.PlatformFeeKobo < 0 || p.PlatformFeeKobo > p.AmountKobo {
		return errors.New("referralcontract: job settled has inconsistent amounts")
	}
	if p.JobCompletedAt.IsZero() {
		return errors.New("referralcontract: job settled needs a completion time")
	}
	return nil
}

// Reversal kinds.
const (
	ReversalRefund          = "refund"
	ReversalChargeback      = "chargeback"
	ReversalFraud           = "fraud"
	ReversalAdminAdjustment = "admin_adjustment"
)

// JobSettlementReversed is the payload of EventJobSettlementReversed.
// ReversalReference is the idempotency key for the reversal's effect.
type JobSettlementReversed struct {
	PaymentIntentID         string    `json:"paymentIntentId"`
	PaymentReference        string    `json:"paymentReference"`
	SourceService           string    `json:"sourceService"`
	SourceReference         string    `json:"sourceReference"`
	CustomerID              string    `json:"customerId"`
	ProviderID              string    `json:"providerId"`
	ProviderType            string    `json:"providerType"`
	ReversalReference       string    `json:"reversalReference"`
	ReversedAmountKobo      int64     `json:"reversedAmountKobo"`
	ReversedPlatformFeeKobo int64     `json:"reversedPlatformFeeKobo"`
	Currency                string    `json:"currency"`
	ReversalKind            string    `json:"reversalKind"`
	FullyReversed           bool      `json:"fullyReversed"`
	OccurredAt              time.Time `json:"occurredAt"`
}

// Validate checks the fields every consumer relies on.
func (p JobSettlementReversed) Validate() error {
	if strings.TrimSpace(p.SourceService) == "" || strings.TrimSpace(p.SourceReference) == "" || strings.TrimSpace(p.ReversalReference) == "" {
		return errors.New("referralcontract: reversal needs a source and a reversal reference")
	}
	if p.ReversedAmountKobo < 0 || p.ReversedPlatformFeeKobo < 0 {
		return errors.New("referralcontract: reversal amounts cannot be negative")
	}
	return nil
}

// AccountRegistered is the payload of EventCustomerRegistered and
// EventProviderRegistered.
//
// Identifiers arrive only as keyed hashes (see HashPhone), never raw. Referral
// codes arrive exactly as the user supplied them; referral-service
// normalizes them. When both are present the link code wins.
type AccountRegistered struct {
	ActorType     string    `json:"actorType"`
	ActorID       string    `json:"actorId"`
	PhoneHash     string    `json:"phoneHash,omitempty"`
	EmailHash     string    `json:"emailHash,omitempty"`
	DeviceHash    string    `json:"deviceHash,omitempty"`
	LinkCode      string    `json:"linkCode,omitempty"`
	TypedCode     string    `json:"typedCode,omitempty"`
	LocationState string    `json:"locationState,omitempty"`
	SignupAt      time.Time `json:"signupAt"`
	// IsNewAccount is false for accounts re-announced by a backfill. Those
	// get a code but can never be attributed to a referrer.
	IsNewAccount bool `json:"isNewAccount"`
}

// Validate checks the fields every consumer relies on.
func (p AccountRegistered) Validate() error {
	if !ValidActorType(p.ActorType) || strings.TrimSpace(p.ActorID) == "" {
		return errors.New("referralcontract: account registration needs a valid actor")
	}
	if p.PhoneHash == "" && p.EmailHash == "" {
		return errors.New("referralcontract: account registration needs a phone or email hash")
	}
	if p.SignupAt.IsZero() {
		return errors.New("referralcontract: account registration needs a signup time")
	}
	return nil
}

// ReferralCode returns the code to attribute: the link code when present,
// otherwise the typed code.
func (p AccountRegistered) ReferralCode() string {
	if strings.TrimSpace(p.LinkCode) != "" {
		return p.LinkCode
	}
	return p.TypedCode
}

// ProviderOnboardingCompleted is the payload of
// EventProviderOnboardingCompleted.
type ProviderOnboardingCompleted struct {
	ActorType   string    `json:"actorType"`
	ActorID     string    `json:"actorId"`
	CompletedAt time.Time `json:"completedAt"`
}

// Validate checks the fields every consumer relies on.
func (p ProviderOnboardingCompleted) Validate() error {
	if !IsProviderActor(p.ActorType) || strings.TrimSpace(p.ActorID) == "" || p.CompletedAt.IsZero() {
		return errors.New("referralcontract: onboarding completion needs a driver actor and a time")
	}
	return nil
}

// HashPhone returns the keyed hash of an already-normalized (+234...) phone
// number. All producers share one pepper so the same person hashes the same
// way in every service; the pepper is what stops a leaked hash being
// reversed by trying every phone number. Empty input or pepper gives "".
func HashPhone(pepper []byte, e164 string) string {
	return keyedHash(pepper, "phone", strings.TrimSpace(e164))
}

// HashEmail returns the keyed hash of an email, trimmed and lowercased.
func HashEmail(pepper []byte, email string) string {
	return keyedHash(pepper, "email", strings.ToLower(strings.TrimSpace(email)))
}

// HashDevice returns the keyed hash of a device identifier.
func HashDevice(pepper []byte, deviceID string) string {
	return keyedHash(pepper, "device", strings.TrimSpace(deviceID))
}

func keyedHash(pepper []byte, kind string, value string) string {
	if len(pepper) == 0 || value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
