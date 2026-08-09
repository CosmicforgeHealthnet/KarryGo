package models

import (
	"fmt"
	"strings"
	"time"
)

// Journal types introduced by Section 13.
//
// A provider allocation and a marketplace settlement are separate journal
// types on purpose. They describe different economic events — the customer
// paying the organization, and the organization later paying a provider — and
// a single type would make the two indistinguishable in the ledger, which is
// exactly what a provider statement must never be able to reconstruct.
const (
	JournalProviderAllocation         = "PROVIDER_ALLOCATION"
	JournalProviderDeduction          = "PROVIDER_DEDUCTION"
	JournalProviderAllocationReversal = "PROVIDER_ALLOCATION_REVERSAL"
	JournalProviderWithdrawalHold     = "PROVIDER_WITHDRAWAL_HOLD"
	JournalProviderWithdrawalPayout   = "PROVIDER_WITHDRAWAL_PAYOUT"
	JournalProviderWithdrawalRelease  = "PROVIDER_WITHDRAWAL_RELEASE"
)

// Allocation types a company may authorize against a provider sub-wallet.
const (
	AllocationSalary            = "SALARY"
	AllocationWage              = "WAGE"
	AllocationPerJob            = "PER_JOB"
	AllocationAllowance         = "ALLOWANCE"
	AllocationReimbursement     = "REIMBURSEMENT"
	AllocationBonus             = "BONUS"
	AllocationAdjustmentCredit  = "ADJUSTMENT_CREDIT"
	AllocationApprovedDeduction = "APPROVED_DEDUCTION"
	AllocationReversal          = "REVERSAL"
)

// Business reference types currently in service.
//
// PRIVATE_ORDER and SHIPMENT are Section 15 and are deliberately absent: an
// accepted value here is an accepted value in the database CHECK constraint,
// and adding them before Section 15 exists would let a half-built order flow
// move money.
const (
	ReferenceTrip           = "TRIP"
	ReferenceAssignment     = "ASSIGNMENT"
	ReferencePayroll        = "PAYROLL_REFERENCE"
	ReferenceExpense        = "EXPENSE_REFERENCE"
	ReferenceManualApproved = "MANUAL_APPROVED_REFERENCE"
)

// Withdrawal policy modes asserted by the requesting organization.
const (
	WithdrawalPolicyProviderDirect   = "PROVIDER_DIRECT"
	WithdrawalPolicyCompanyApproval  = "COMPANY_APPROVAL_REQUIRED"
	WithdrawalPolicyCompanyInitiated = "COMPANY_INITIATED_ONLY"
	WithdrawalPolicyDisabled         = "WITHDRAWALS_DISABLED"
)

// Withdrawal statuses.
const (
	WithdrawalRequested       = "REQUESTED"
	WithdrawalPendingApproval = "PENDING_APPROVAL"
	WithdrawalApproved        = "APPROVED"
	WithdrawalRejected        = "REJECTED"
	WithdrawalProcessing      = "PROCESSING"
	WithdrawalCompleted       = "COMPLETED"
	WithdrawalFailedRetryable = "FAILED_RETRYABLE"
	WithdrawalFailedPermanent = "FAILED_PERMANENT"
	WithdrawalCancelled       = "CANCELLED"
	WithdrawalDeadLetter      = "DEAD_LETTER"
)

// Allocation approval operation, governed by a versioned policy.
const OperationProviderWithdrawalApproval = "PROVIDER_WITHDRAWAL_APPROVAL"

var allocationTypes = map[string]bool{
	AllocationSalary: true, AllocationWage: true, AllocationPerJob: true,
	AllocationAllowance: true, AllocationReimbursement: true, AllocationBonus: true,
	AllocationAdjustmentCredit: true, AllocationApprovedDeduction: true,
	AllocationReversal: true,
}

var referenceTypes = map[string]bool{
	ReferenceTrip: true, ReferenceAssignment: true, ReferencePayroll: true,
	ReferenceExpense: true, ReferenceManualApproved: true,
}

// ValidAllocationType reports whether value is a supported allocation type.
func ValidAllocationType(value string) bool { return allocationTypes[strings.ToUpper(value)] }

// ValidBusinessReferenceType reports whether value is a supported reference
// type. Section 15 reference names return false.
func ValidBusinessReferenceType(value string) bool { return referenceTypes[strings.ToUpper(value)] }

// DebitsProvider reports whether an allocation type takes value away from the
// provider rather than giving it.
func DebitsProvider(allocationType string) bool {
	switch strings.ToUpper(allocationType) {
	case AllocationApprovedDeduction, AllocationReversal:
		return true
	default:
		return false
	}
}

// ProviderAllocation is one company-authorized movement between an
// organization wallet and one provider sub-wallet.
type ProviderAllocation struct {
	ID                       string    `json:"id"`
	OrganizationID           string    `json:"organizationId"`
	OrganizationWalletID     string    `json:"organizationWalletId"`
	EarningAccountID         string    `json:"earningAccountId"`
	ProviderID               string    `json:"providerId,omitempty"`
	OrganizationMembershipID string    `json:"organizationMembershipId,omitempty"`
	AllocationType           string    `json:"allocationType"`
	BusinessReferenceType    string    `json:"businessReferenceType"`
	BusinessReferenceID      string    `json:"businessReferenceId"`
	AmountMinor              int64     `json:"amountMinor"`
	Currency                 string    `json:"currency"`
	ReasonCode               string    `json:"reasonCode"`
	ProviderExplanation      string    `json:"providerExplanation,omitempty"`
	JournalID                string    `json:"journalId"`
	ReversesAllocationID     string    `json:"reversesAllocationId,omitempty"`
	Status                   string    `json:"status"`
	SourceService            string    `json:"sourceService,omitempty"`
	SourceReference          string    `json:"sourceReference,omitempty"`
	ApprovalReference        string    `json:"approvalReference,omitempty"`
	RequestedByActorType     string    `json:"requestedByActorType,omitempty"`
	RequestedByActorID       string    `json:"requestedByActorId,omitempty"`
	IdempotencyKey           string    `json:"-"`
	RequestFingerprint       string    `json:"-"`
	CorrelationID            string    `json:"correlationId,omitempty"`
	CausationID              string    `json:"-"`
	Version                  int64     `json:"version"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}

// ProviderWithdrawal is one payout request against a provider sub-wallet.
//
// DestinationReference is opaque. The account number behind it is held by the
// payout provider and is never copied into this service.
type ProviderWithdrawal struct {
	ID                       string     `json:"id"`
	OrganizationID           string     `json:"organizationId"`
	EarningAccountID         string     `json:"earningAccountId"`
	OrganizationMembershipID string     `json:"organizationMembershipId,omitempty"`
	ProviderID               string     `json:"providerId,omitempty"`
	AmountMinor              int64      `json:"amountMinor"`
	Currency                 string     `json:"currency"`
	DestinationReference     string     `json:"destinationReference"`
	PolicyMode               string     `json:"policyMode"`
	ApprovalReference        string     `json:"approvalReference,omitempty"`
	Status                   string     `json:"status"`
	HoldJournalID            string     `json:"holdJournalId,omitempty"`
	PayoutJournalID          string     `json:"payoutJournalId,omitempty"`
	ExternalReference        string     `json:"externalReference,omitempty"`
	SafeErrorCode            string     `json:"safeErrorCode,omitempty"`
	Attempts                 int        `json:"attempts"`
	NextAttemptAt            time.Time  `json:"nextAttemptAt"`
	LeaseOwner               string     `json:"-"`
	LeaseExpiresAt           *time.Time `json:"-"`
	RequestedByActorType     string     `json:"requestedByActorType,omitempty"`
	RequestedByActorID       string     `json:"requestedByActorId,omitempty"`
	DecidedByActorType       string     `json:"decidedByActorType,omitempty"`
	DecidedByActorID         string     `json:"decidedByActorId,omitempty"`
	DecidedAt                *time.Time `json:"decidedAt,omitempty"`
	IdempotencyKey           string     `json:"-"`
	RequestFingerprint       string     `json:"-"`
	CorrelationID            string     `json:"correlationId,omitempty"`
	CausationID              string     `json:"-"`
	Version                  int64      `json:"version"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
	CompletedAt              *time.Time `json:"completedAt,omitempty"`
}

// RequesterIdentity returns the identity that asked for the withdrawal, for
// separation-of-duties checks at approval time.
func (w ProviderWithdrawal) RequesterIdentity() string {
	return w.RequestedByActorType + ":" + w.RequestedByActorID
}

// Resolved reports whether the withdrawal has reached a state that no longer
// accepts an approve or reject decision.
func (w ProviderWithdrawal) Resolved() bool {
	switch w.Status {
	case WithdrawalRequested, WithdrawalPendingApproval:
		return false
	default:
		return true
	}
}

// ProviderStatementEntry is one provider-visible line.
//
// This type is the whole data-minimization boundary for Section 13 reads. It
// is built field by field from allocation and withdrawal records; no internal
// record is ever serialized and then pruned, because a field added upstream
// later would silently start leaking through a prune-based approach.
type ProviderStatementEntry struct {
	ID                    string    `json:"id"`
	EntryType             string    `json:"entryType"`
	AllocationType        string    `json:"allocationType,omitempty"`
	Direction             string    `json:"direction"`
	AmountMinor           int64     `json:"amountMinor"`
	Currency              string    `json:"currency"`
	BusinessReferenceType string    `json:"businessReferenceType,omitempty"`
	BusinessReferenceID   string    `json:"businessReferenceId,omitempty"`
	ReasonCode            string    `json:"reasonCode,omitempty"`
	Explanation           string    `json:"explanation,omitempty"`
	Status                string    `json:"status"`
	OccurredAt            time.Time `json:"occurredAt"`
	SourceVersion         int64     `json:"sourceVersion"`
}

// Statement entry types and directions.
const (
	StatementEntryAllocation = "ALLOCATION"
	StatementEntryWithdrawal = "WITHDRAWAL"
	DirectionCredit          = "CREDIT"
	DirectionDebit           = "DEBIT"
)

// ProviderStatement is the provider-facing view of one sub-wallet.
type ProviderStatement struct {
	EarningAccountID string                   `json:"earningAccountId"`
	Currency         string                   `json:"currency"`
	Status           string                   `json:"status"`
	AvailableMinor   int64                    `json:"availableMinor"`
	HeldMinor        int64                    `json:"heldMinor"`
	SettledMinor     int64                    `json:"settledMinor"`
	SourceVersion    int64                    `json:"sourceVersion"`
	GeneratedAt      time.Time                `json:"generatedAt"`
	Entries          []ProviderStatementEntry `json:"entries"`
}

// ValidateAllocation applies the invariants that must hold before any journal
// is written.
func ValidateAllocation(allocationType, referenceType, referenceID, currency string, amountMinor int64) error {
	if !ValidAllocationType(allocationType) {
		return fmt.Errorf("provider allocation: unsupported allocation type %q", allocationType)
	}
	if !ValidBusinessReferenceType(referenceType) {
		return fmt.Errorf("provider allocation: unsupported business reference type %q", referenceType)
	}
	if strings.TrimSpace(referenceID) == "" {
		return fmt.Errorf("provider allocation: business reference is required")
	}
	if amountMinor <= 0 {
		return fmt.Errorf("provider allocation: amount must be positive")
	}
	if _, err := parseCurrency(currency); err != nil {
		return err
	}
	if strings.ToUpper(allocationType) == AllocationPerJob &&
		strings.ToUpper(referenceType) != ReferenceTrip &&
		strings.ToUpper(referenceType) != ReferenceAssignment {
		return fmt.Errorf("provider allocation: per-job allocation requires a trip or assignment reference")
	}
	return nil
}

func parseCurrency(currency string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(currency))
	if len(value) != 3 {
		return "", fmt.Errorf("provider allocation: currency must be a three letter ISO code")
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return "", fmt.Errorf("provider allocation: currency must be a three letter ISO code")
		}
	}
	return value, nil
}
