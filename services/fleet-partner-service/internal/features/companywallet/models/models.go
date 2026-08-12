// Package models holds the Section 13 company wallet domain types.
//
// Fleet owns references, instructions, policy and workflow. It owns no ledger
// and no authoritative balance. Every balance-shaped value that appears here
// arrives from payment-wallet-service and carries a freshness state saying how
// far it may be trusted; the answer is never "far enough to move money".
package models

import (
	"strings"
	"time"
)

// Wallet link states. UNKNOWN, STALE and RECONCILING are deliberately separate
// from the FAILED states: they mean "we do not currently know", and every
// sensitive gate treats not knowing as a refusal rather than as a soft yes.
const (
	LinkNotRequested      = "NOT_REQUESTED"
	LinkRequested         = "REQUESTED"
	LinkProvisioning      = "PROVISIONING"
	LinkActive            = "ACTIVE"
	LinkSuspended         = "SUSPENDED"
	LinkRestricted        = "RESTRICTED"
	LinkUnknown           = "UNKNOWN"
	LinkStale             = "STALE"
	LinkReconciling       = "RECONCILING"
	LinkFailedRetryable   = "FAILED_RETRYABLE"
	LinkFailedPermanent   = "FAILED_PERMANENT"
	LinkReconcileRequired = "RECONCILIATION_REQUIRED"
	LinkClosed            = "CLOSED"
	LinkDisabled          = "DISABLED"
)

// Freshness states of a local projection.
const (
	FreshnessCurrent     = "CURRENT"
	FreshnessStale       = "STALE"
	FreshnessGapDetected = "GAP_DETECTED"
	FreshnessUnknown     = "UNKNOWN"
	FreshnessReconciling = "RECONCILING"
)

// OperationalLink reports whether a link state permits a sensitive financial
// operation.
//
// Only a link that is ACTIVE and whose local copy is CURRENT qualifies. A
// stale copy of an ACTIVE link does not: the link may have been restricted
// since, and the whole point of the freshness state is that Fleet cannot tell.
func OperationalLink(status, freshness string) bool {
	return status == LinkActive && freshness == FreshnessCurrent
}

// OrganizationWalletLink is Fleet's reference to the organization's
// Wallet-owned account.
type OrganizationWalletLink struct {
	OrganizationID      string
	Status              string
	WalletAccountID     string
	Currency            string
	IdempotencyKey      string
	Attempts            int
	LastErrorCode       string
	NextAttemptAt       *time.Time
	SourceVersion       int64
	LastEventID         string
	FreshnessState      string
	ReconciliationState string
	CorrelationID       string
	SyncedAt            *time.Time
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Operational reports whether the organization wallet may fund an operation.
func (l OrganizationWalletLink) Operational() bool {
	return OperationalLink(l.Status, l.FreshnessState) && l.WalletAccountID != ""
}

// ProviderSubWalletLink is Fleet's reference to one provider sub-wallet.
type ProviderSubWalletLink struct {
	ProviderMembershipID string
	OrganizationID       string
	ProviderID           string
	ProviderService      string
	WalletAccountID      string
	Currency             string
	Status               string
	IdempotencyKey       string
	SourceVersion        int64
	LastEventID          string
	FreshnessState       string
	LastErrorCode        string
	SyncedAt             *time.Time
	Version              int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Operational reports whether the sub-wallet may receive an allocation.
func (l ProviderSubWalletLink) Operational() bool {
	return OperationalLink(l.Status, l.FreshnessState) && l.WalletAccountID != ""
}

// Allocation types.
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
// PRIVATE_ORDER and SHIPMENT belong to Section 15 and are intentionally not
// accepted. They are named nowhere in this file so that adding them is a
// deliberate act rather than an oversight.
const (
	ReferenceTrip           = "TRIP"
	ReferenceAssignment     = "ASSIGNMENT"
	ReferencePayroll        = "PAYROLL_REFERENCE"
	ReferenceExpense        = "EXPENSE_REFERENCE"
	ReferenceManualApproved = "MANUAL_APPROVED_REFERENCE"
)

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

// RequiresCompletedWork reports whether an allocation type may only be created
// against work that actually finished.
func RequiresCompletedWork(allocationType, referenceType string) bool {
	return strings.ToUpper(allocationType) == AllocationPerJob &&
		strings.ToUpper(referenceType) == ReferenceTrip
}

// DebitsProvider reports whether an allocation takes value from the provider.
func DebitsProvider(allocationType string) bool {
	switch strings.ToUpper(allocationType) {
	case AllocationApprovedDeduction, AllocationReversal:
		return true
	default:
		return false
	}
}

// Allocation instruction statuses.
const (
	AllocationDraft             = "DRAFT"
	AllocationPendingApproval   = "PENDING_APPROVAL"
	AllocationApproved          = "APPROVED"
	AllocationSubmissionPending = "SUBMISSION_PENDING"
	AllocationProcessing        = "PROCESSING"
	AllocationUnknown           = "UNKNOWN"
	AllocationCompleted         = "COMPLETED"
	AllocationFailedRetryable   = "FAILED_RETRYABLE"
	AllocationFailedPermanent   = "FAILED_PERMANENT"
	AllocationRejected          = "REJECTED"
	AllocationReconciling       = "RECONCILING"
	AllocationReversed          = "REVERSED"
	AllocationDeadLetter        = "DEAD_LETTER"
)

// TerminalAllocationStatus reports whether an instruction has finished moving.
func TerminalAllocationStatus(status string) bool {
	switch status {
	case AllocationCompleted, AllocationFailedPermanent, AllocationRejected,
		AllocationReversed, AllocationDeadLetter:
		return true
	default:
		return false
	}
}

// AllocationInstruction is Fleet's record of a company decision to move value
// to a provider sub-wallet. The movement itself belongs to Wallet.
type AllocationInstruction struct {
	ID                         string
	OrganizationID             string
	BranchID                   string
	ProviderMembershipID       string
	ProviderID                 string
	ProviderSubWalletID        string
	OrganizationWalletID       string
	AllocationType             string
	AmountMinor                int64
	Currency                   string
	BusinessReferenceType      string
	BusinessReferenceID        string
	ReasonCode                 string
	ProviderExplanation        string
	PrivateNote                string
	LegalBasisReference        string
	RequestedBy                string
	ApprovalRequestID          string
	FinanceOperationID         string
	WalletOperationID          string
	WalletTransactionReference string
	IdempotencyKey             string
	RequestFingerprint         string
	ExpectedVersion            int64
	Status                     string
	SourceVersion              int64
	LastEventID                string
	FreshnessState             string
	ReconciliationState        string
	SafeErrorCode              string
	CorrelationID              string
	CausationID                string
	Version                    int64
	CreatedBy                  string
	UpdatedBy                  string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	CompletedAt                *time.Time
}

// Withdrawal policy modes.
const (
	PolicyProviderDirect   = "PROVIDER_DIRECT"
	PolicyCompanyApproval  = "COMPANY_APPROVAL_REQUIRED"
	PolicyCompanyInitiated = "COMPANY_INITIATED_ONLY"
	PolicyDisabled         = "WITHDRAWALS_DISABLED"
)

var policyModes = map[string]bool{
	PolicyProviderDirect: true, PolicyCompanyApproval: true,
	PolicyCompanyInitiated: true, PolicyDisabled: true,
}

// ValidPolicyMode reports whether value names a supported policy mode.
func ValidPolicyMode(value string) bool { return policyModes[strings.ToUpper(value)] }

// WithdrawalPolicy is the organization's own rule set.
//
// It can only narrow what a provider may do. Wallet-side compliance, bank
// verification, platform limits, fraud controls and account status are applied
// independently and cannot be switched off from here.
type WithdrawalPolicy struct {
	OrganizationID                   string
	PolicyMode                       string
	MinimumAmountMinor               int64
	ApprovalThresholdMinor           int64
	AllocationApprovalThresholdMinor int64
	Currency                         string
	UpdatedBy                        string
	Version                          int64
	CreatedAt                        time.Time
	UpdatedAt                        time.Time
}

// DefaultWithdrawalPolicy is what an organization that has never configured
// one gets: approval required. The safe default for money leaving the platform
// is that a second person looks at it.
func DefaultWithdrawalPolicy(organizationID string) WithdrawalPolicy {
	return WithdrawalPolicy{OrganizationID: organizationID, PolicyMode: PolicyCompanyApproval, Version: 1}
}

// AllowsProviderRequest reports whether a provider may open the request
// themselves under this policy.
func (p WithdrawalPolicy) AllowsProviderRequest() bool {
	mode := strings.ToUpper(p.PolicyMode)
	return mode == PolicyProviderDirect || mode == PolicyCompanyApproval
}

// RequiresApproval reports whether a withdrawal of this amount needs a company
// decision before it may execute.
func (p WithdrawalPolicy) RequiresApproval(amountMinor int64) bool {
	mode := strings.ToUpper(p.PolicyMode)
	if mode == PolicyCompanyApproval || mode == PolicyCompanyInitiated {
		return true
	}
	return p.ApprovalThresholdMinor > 0 && amountMinor >= p.ApprovalThresholdMinor
}

// RequiresAllocationApproval reports whether an allocation of this amount
// needs approval under the organization's threshold.
func (p WithdrawalPolicy) RequiresAllocationApproval(amountMinor int64, allocationType string) bool {
	// A deduction always needs approval regardless of size: it takes money
	// from someone who is not present to object.
	if DebitsProvider(allocationType) {
		return true
	}
	return p.AllocationApprovalThresholdMinor > 0 && amountMinor >= p.AllocationApprovalThresholdMinor
}

// Withdrawal request statuses.
const (
	WithdrawalRequested         = "REQUESTED"
	WithdrawalPendingApproval   = "PENDING_APPROVAL"
	WithdrawalApproved          = "APPROVED"
	WithdrawalRejected          = "REJECTED"
	WithdrawalSubmissionPending = "SUBMISSION_PENDING"
	WithdrawalProcessing        = "PROCESSING"
	WithdrawalUnknown           = "UNKNOWN"
	WithdrawalCompleted         = "COMPLETED"
	WithdrawalFailedRetryable   = "FAILED_RETRYABLE"
	WithdrawalFailedPermanent   = "FAILED_PERMANENT"
	WithdrawalReconciling       = "RECONCILING"
	WithdrawalCancelled         = "CANCELLED"
	WithdrawalDeadLetter        = "DEAD_LETTER"
)

// DecidableWithdrawal reports whether a withdrawal still accepts an approve or
// reject decision.
func DecidableWithdrawal(status string) bool {
	return status == WithdrawalRequested || status == WithdrawalPendingApproval
}

// WithdrawalRequest is Fleet's record of a payout request.
//
// DestinationReference is opaque. Fleet stores no account number, no sort
// code and no bank name: the payout destination lives with the authoritative
// payment service, and a copy here would be a second place to breach.
type WithdrawalRequest struct {
	ID                        string
	OrganizationID            string
	BranchID                  string
	ProviderMembershipID      string
	ProviderID                string
	ProviderSubWalletID       string
	AmountMinor               int64
	Currency                  string
	DestinationReference      string
	PolicyMode                string
	RequestedByActorType      string
	RequestedBy               string
	ApprovalRequestID         string
	DecidedBy                 string
	DecidedAt                 *time.Time
	DecisionReasonCode        string
	FinanceOperationID        string
	WalletOperationID         string
	WalletWithdrawalReference string
	IdempotencyKey            string
	RequestFingerprint        string
	ExpectedVersion           int64
	Status                    string
	SourceVersion             int64
	LastEventID               string
	FreshnessState            string
	ReconciliationState       string
	SafeErrorCode             string
	CorrelationID             string
	CausationID               string
	Version                   int64
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	CompletedAt               *time.Time
}

// TransactionProjection is a display-only copy of one Wallet transaction.
//
// Nothing sums this type. A balance is asked of payment-wallet-service, never
// reconstructed from these rows, because a projection with one missing event
// would produce a number that looks exactly as authoritative as a correct one.
type TransactionProjection struct {
	ID                   string
	OrganizationID       string
	BranchID             string
	WalletReference      string
	TransactionReference string
	SubjectType          string
	SubjectID            string
	Direction            string
	Category             string
	AmountMinor          int64
	Currency             string
	SafeStatus           string
	OccurredAt           time.Time
	SourceVersion        int64
	LastEventID          string
	FreshnessState       string
	LastSyncedAt         time.Time
	Version              int64
}

// ProviderStatementEntry is one provider-visible statement line.
//
// This is the data-minimization boundary. It is constructed field by field;
// no internal record is ever serialized and then pruned, because a field added
// upstream later would start leaking the moment it was added.
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
	EntryAllocation = "ALLOCATION"
	EntryWithdrawal = "WITHDRAWAL"
	DirectionCredit = "CREDIT"
	DirectionDebit  = "DEBIT"
)
