package models

import (
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
)

// OperationSummary is the operator-facing view of one durable Wallet
// operation.
//
// It carries the operation's own lifecycle and the opaque Wallet reference —
// never a balance, a ledger account or a journal line. An operator who needs
// to know whether a payout ran does not need to see the books to find out.
type OperationSummary struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	BranchID        string     `json:"branchId,omitempty"`
	SubjectType     string     `json:"subjectType"`
	SubjectID       string     `json:"subjectId"`
	OperationType   string     `json:"operationType"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	NextAttemptAt   time.Time  `json:"nextAttemptAt"`
	WalletReference string     `json:"walletReference,omitempty"`
	SafeErrorCode   string     `json:"safeErrorCode,omitempty"`
	RequestedByType string     `json:"requestedByActorType,omitempty"`
	RequestedByID   string     `json:"requestedByActorId,omitempty"`
	CorrelationID   string     `json:"correlationId,omitempty"`
	ExpectedVersion int64      `json:"expectedVersion"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
}

// ProjectionSummary is the operator-facing view of one finance projection.
type ProjectionSummary struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organizationId"`
	BranchID        string `json:"branchId,omitempty"`
	ProjectionType  string `json:"projectionType"`
	SubjectType     string `json:"subjectType"`
	SubjectID       string `json:"subjectId"`
	WalletReference string `json:"walletReference"`
	AmountMinor     *int64 `json:"amountMinor,omitempty"`
	Currency        string `json:"currency,omitempty"`
	// Amount is the spec 6.1 Money object for AmountMinor and Currency,
	// present exactly when AmountMinor is.
	Amount                 *money.Money `json:"amount,omitempty"`
	SafeStatus             string       `json:"safeStatus"`
	SourceVersion          int64        `json:"sourceVersion"`
	FreshnessState         string       `json:"freshnessState"`
	ReconciliationStatus   string       `json:"reconciliationStatus"`
	ReconciliationAttempts int          `json:"reconciliationAttempts"`
	SafeErrorCode          string       `json:"safeErrorCode,omitempty"`
	LastSyncedAt           time.Time    `json:"lastSyncedAt"`
	Version                int64        `json:"version"`
}

// Authoritative reports whether the projection may authorize money movement.
// Only a fresh, fully reconciled projection may.
func (p ProjectionSummary) Authoritative() bool {
	return p.FreshnessState == "CURRENT" && p.ReconciliationStatus == "IDLE"
}

// HistoryEntry is one recorded operation transition.
type HistoryEntry struct {
	ID             string    `json:"id"`
	PreviousStatus string    `json:"previousStatus"`
	NewStatus      string    `json:"newStatus"`
	SafeReasonCode string    `json:"safeReasonCode,omitempty"`
	ActorType      string    `json:"actorType"`
	ActorID        string    `json:"actorId"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// AuditEntry is one recorded operator action.
type AuditEntry struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	ActorType    string    `json:"actorType"`
	ActorID      string    `json:"actorId"`
	OccurredAt   time.Time `json:"occurredAt"`
}

// Scope is the tenant and branch restriction applied to every operator read.
//
// BranchIDs empty means organization-wide authority. A caller restricted to
// branches sees rows in those branches plus rows with no branch at all, which
// are organization-level operations.
type Scope struct {
	OrganizationID string
	BranchIDs      []string
	Limit          int
}

// PageLimit is the capped list size (default 50, at most 200).
func (s Scope) PageLimit() int {
	if s.Limit <= 0 || s.Limit > 200 {
		return 50
	}
	return s.Limit
}

// OperationFilter narrows an operation listing.
type OperationFilter struct {
	Status        string
	OperationType string
	SubjectID     string
}

// ProjectionFilter narrows a projection listing.
type ProjectionFilter struct {
	ProjectionType string
	Freshness      string
	SubjectID      string
	// ReconciliationOnly restricts the listing to open reconciliation cases.
	ReconciliationOnly bool
}

// WithAmount sets Amount from the flat AmountMinor and Currency.
func (p ProjectionSummary) WithAmount() ProjectionSummary {
	p.Amount = nil
	if p.AmountMinor != nil {
		p.Amount = &money.Money{AmountMinor: *p.AmountMinor, Currency: p.Currency}
	}
	return p
}

// ErrReadBackNotAvailable means a read-back is already in progress.
var ErrReadBackNotAvailable = errors.New("finance: authoritative read-back is already in progress")
