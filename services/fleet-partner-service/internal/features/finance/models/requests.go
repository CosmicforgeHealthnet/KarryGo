package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound            = errors.New("finance projection: not found")
	ErrFingerprintConflict = errors.New("finance operation: idempotency fingerprint conflict")
	// ErrFinancialStateBlocked means authoritative state does not permit the
	// action.
	ErrFinancialStateBlocked = errors.New("finance: authoritative state is not current and eligible")
	// ErrSeparationOfDuties means the acting identity may not act on a
	// request it made itself.
	ErrSeparationOfDuties = errors.New("finance: separation of duties forbids acting on your own request")
	// ErrEventConflict means one source version arrived with two payloads.
	ErrEventConflict = errors.New("finance event: source version payload conflict")
)

// Actor identifies the operator or service behind an orchestrated command.
type Actor struct {
	Type          string
	ID            string
	CorrelationID string
	CausationID   string
}

// Request is one durable operation Fleet asks Wallet to perform.
type Request struct {
	OrganizationID, BranchID, SubjectType, SubjectID, OperationType, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	Payload                                                                                                                         json.RawMessage
	PayloadVersion                                                                                                                  int
	ExpectedVersion                                                                                                                 int64
}

// SettlementRequest asks Wallet to settle earnings to a provider or to the
// organization itself.
type SettlementRequest struct {
	Actor                Actor
	OrganizationID       string
	BranchID             string
	BeneficiaryType      string
	BeneficiaryReference string
	EarningAccountID     string
	WalletID             string
	Currency             string
	IdempotencyKey       string
	ExpectedVersion      int64
}

// RefundRequest asks Wallet to refund against an original posted entry.
type RefundRequest struct {
	Actor             Actor
	OrganizationID    string
	BranchID          string
	OriginalJournalID string
	PaymentReference  string
	AmountMinor       int64
	Currency          string
	ReasonCode        string
	Reason            string
	IdempotencyKey    string
	ExpectedVersion   int64
}

// ReversalRequest asks Wallet to open a durable reversal.
type ReversalRequest struct {
	Actor             Actor
	OrganizationID    string
	BranchID          string
	OriginalJournalID string
	SourceOperationID string
	AmountMinor       int64
	Currency          string
	ReasonCode        string
	Reason            string
	IdempotencyKey    string
	ExpectedVersion   int64
}

// SubscriptionInvoiceRequest asks Wallet to invoice a subscription plan.
type SubscriptionInvoiceRequest struct {
	OrganizationID, PlanID, Currency, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	ExpectedVersion                                                                                  int64
}

// JobEarningRequest asks Wallet to allocate a completed, paid job's earning.
type JobEarningRequest struct {
	OrganizationID, TripID, PaymentReference, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	RequireRecipientConfirmation                                                                             bool
	ExpectedVersion                                                                                          int64
	Payload                                                                                                  json.RawMessage
}

// WorkerConfig tunes the finance operation and reconciliation worker.
type WorkerConfig struct {
	Owner                        string
	BatchSize                    int
	Lease, Interval, BaseBackoff time.Duration
	ProjectionStaleAfter         time.Duration
	MaxAttempts                  int
}

// Norm fills defaults.
func (c WorkerConfig) Norm() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "fleet-finance"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 2 * time.Second
	}
	if c.ProjectionStaleAfter <= 0 {
		c.ProjectionStaleAfter = 15 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	return c
}

// StatusIn reports whether value (case-insensitive) is one of allowed.
func StatusIn(value string, allowed ...string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// ProjectionTypeForEvent maps a Wallet event type onto a projection type, or
// "" when unsupported.
func ProjectionTypeForEvent(t string) string {
	// Ordered, entity-specific matching is intentional. All of the newer event
	// names begin with "wallet.", so a generic wallet prefix would also match
	// wallet.invoice and wallet.refund and make routing depend on map iteration.
	for _, route := range []struct{ needle, value string }{
		{"earning_account.", "EARNING_ACCOUNT"},
		{"subscription.", "SUBSCRIPTION"}, {"invoice.", "INVOICE"},
		{"hold.", "HOLD"}, {"commission.", "COMMISSION"},
		{"earning.", "EARNING"}, {"settlement.", "SETTLEMENT"},
		{"refund.", "REFUND"}, {"reversal.", "REVERSAL"},
		{"wallet.organization.", "WALLET"}, {"finance.wallet.", "WALLET"},
	} {
		if strings.Contains(t, route.needle) {
			return route.value
		}
	}
	return ""
}

// StatusForEvent derives a status from an event type's verb.
func StatusForEvent(t string) string {
	parts := strings.Split(t, ".")
	if len(parts) > 2 {
		return strings.ToUpper(parts[len(parts)-2])
	}
	return "UNKNOWN"
}

// MinorAmount reads the first present money field as an exact, non-negative
// whole number of minor units. A fractional, negative or non-numeric amount is
// refused rather than truncated: guessing at money is worse than a rejected
// event, which the sender retries and reconciliation surfaces.
func MinorAmount(payload map[string]any, fields ...string) (*int64, error) {
	for _, field := range fields {
		raw, present := payload[field]
		if !present || raw == nil {
			continue
		}
		number, ok := raw.(json.Number)
		if !ok {
			return nil, fmt.Errorf("finance event: %s must be a number", field)
		}
		value, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("finance event: %s must be a whole, non-negative number of minor units", field)
		}
		return &value, nil
	}
	return nil, nil
}
