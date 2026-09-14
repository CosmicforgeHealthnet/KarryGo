package walletmodels

import (
	"fmt"
	"strings"
	"time"
)

// Section 12: Fleet-managed work settlement.
//
// The division of authority this file exists to hold:
//
//   - Fleet decides WHAT each party is entitled to. It resolves the provider's
//     effective compensation plan and produces an immutable instruction.
//   - Wallet decides HOW that entitlement is posted, and nothing else. It does
//     not resolve plans, does not apply commission rates and does not decide
//     that a salaried provider should be paid per job.
//
// The one number Wallet keeps authority over is the platform's own share: that
// was fixed when the customer's payment intent was created, and Fleet has no
// business re-deciding it. So the instruction may state it (and is checked
// against the intent) but may not change it.

// Settlement modes recorded on a payment intent.
const (
	// SettlementModeIndependent is the legacy direct-to-provider path.
	SettlementModeIndependent = "INDEPENDENT"

	// SettlementModeManaged means this job's money is allocated by Fleet.
	// The legacy CompleteJob path refuses it.
	SettlementModeManaged = "ORGANIZATION_MANAGED"
)

// Managed settlement lifecycle.
const (
	ManagedSettlementPending = "PENDING"
	ManagedSettlementPosted  = "POSTED"

	// ManagedSettlementFailedRetryable is a transient failure: the caller may
	// resend the same instruction unchanged.
	ManagedSettlementFailedRetryable = "FAILED_RETRYABLE"

	// ManagedSettlementFailedTerminal means the instruction will never post as
	// stated. Resending it unchanged cannot help.
	ManagedSettlementFailedTerminal = "FAILED_TERMINAL"

	// ManagedSettlementReconciliationRequired means Wallet cannot tell whether
	// it posted. It is not a failure, and it is never resolved by guessing:
	// the row is read back and the truth taken from the ledger.
	ManagedSettlementReconciliationRequired = "RECONCILIATION_REQUIRED"

	ManagedSettlementReversed = "REVERSED"
)

// ValidManagedSettlementStatus reports whether a status is one this service
// stores.
func ValidManagedSettlementStatus(status string) bool {
	switch status {
	case ManagedSettlementPending, ManagedSettlementPosted,
		ManagedSettlementFailedRetryable, ManagedSettlementFailedTerminal,
		ManagedSettlementReconciliationRequired, ManagedSettlementReversed:
		return true
	default:
		return false
	}
}

// Organization ledger identity. Organization entitlement is credited to a
// payable account owned by the organization, in the same ledger that holds the
// job escrow -- so the release of one job's money is a single balanced journal
// rather than a posting here and a transfer somewhere else.
const (
	OwnerTypeOrganization      = "organization"
	AccountOrganizationPayable = "organization_payable"
)

// JournalManagedSettlement and JournalManagedSettlementReversal name the two
// journal types this path can write.
const (
	JournalManagedSettlement         = "managed_work_settlement"
	JournalManagedSettlementReversal = "managed_work_settlement_reversal"
)

// ManagedSettlementInstruction is Fleet's immutable instruction as Wallet
// receives it.
//
// Note what is absent: no rate, no plan rules, no basis mode. Wallet is given
// resolved amounts precisely so it cannot be tempted to recompute them.
type ManagedSettlementInstruction struct {
	InstructionID      string `json:"instruction_id"`
	InstructionVersion int64  `json:"instruction_version"`

	OrganizationID string `json:"organization_id"`
	MembershipID   string `json:"membership_id"`
	ProviderID     string `json:"provider_id"`

	// ProviderType is the wallet-side provider namespace (taxi, dispatch,
	// hauling). It selects the provider's payable account, and it is required:
	// two services can legitimately use the same provider id.
	ProviderType string `json:"provider_type"`

	SourceService      string `json:"source_service"`
	SourceJobID        string `json:"source_job_id"`
	SourceAssignmentID string `json:"source_assignment_id,omitempty"`

	Currency string `json:"currency"`

	// GrossBasisMinor is the money Fleet allocated against. It must equal what
	// is actually distributable on the held intent once the platform's own
	// share is taken out -- otherwise Fleet resolved against a figure this
	// service cannot honour, and the settlement is refused rather than
	// silently posted short.
	GrossBasisMinor        int64 `json:"gross_basis_minor"`
	ProviderShareMinor     int64 `json:"provider_share_minor"`
	OrganizationShareMinor int64 `json:"organization_share_minor"`

	// PlatformShareMinor is optional. When present it is verified against the
	// intent's own platform fee; it never overrides it.
	PlatformShareMinor *int64 `json:"platform_share_minor,omitempty"`

	CompensationPlanID            string `json:"compensation_plan_id,omitempty"`
	CompensationPlanVersion       int64  `json:"compensation_plan_version,omitempty"`
	CompensationResolutionVersion int64  `json:"compensation_resolution_version,omitempty"`

	CorrelationID  string `json:"correlation_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Normalized trims and upper-cases the fields that are compared or stored.
func (i ManagedSettlementInstruction) Normalized() ManagedSettlementInstruction {
	i.InstructionID = strings.TrimSpace(i.InstructionID)
	i.OrganizationID = strings.TrimSpace(i.OrganizationID)
	i.MembershipID = strings.TrimSpace(i.MembershipID)
	i.ProviderID = strings.TrimSpace(i.ProviderID)
	i.ProviderType = strings.ToLower(strings.TrimSpace(i.ProviderType))
	i.SourceService = strings.TrimSpace(i.SourceService)
	i.SourceJobID = strings.TrimSpace(i.SourceJobID)
	i.SourceAssignmentID = strings.TrimSpace(i.SourceAssignmentID)
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	i.CompensationPlanID = strings.TrimSpace(i.CompensationPlanID)
	i.CorrelationID = strings.TrimSpace(i.CorrelationID)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	return i
}

// Validate checks the instruction is internally coherent.
//
// It deliberately does NOT check the instruction against the payment intent:
// that comparison needs the intent and happens inside the posting transaction,
// where the intent is locked. This is only the part that can be judged from the
// instruction alone.
func (i ManagedSettlementInstruction) Validate() error {
	switch {
	case i.InstructionID == "":
		return fmt.Errorf("instruction_id is required")
	case i.InstructionVersion <= 0:
		return fmt.Errorf("instruction_version must be positive")
	case i.OrganizationID == "":
		return fmt.Errorf("organization_id is required")
	case i.MembershipID == "":
		return fmt.Errorf("membership_id is required")
	case i.ProviderID == "":
		return fmt.Errorf("provider_id is required")
	case i.ProviderType == "":
		return fmt.Errorf("provider_type is required")
	case i.SourceService == "":
		return fmt.Errorf("source_service is required")
	case i.SourceJobID == "":
		return fmt.Errorf("source_job_id is required")
	case len(i.Currency) != 3:
		return fmt.Errorf("currency must be a three letter code")
	case i.IdempotencyKey == "":
		return fmt.Errorf("idempotency_key is required")
	case i.GrossBasisMinor < 0 || i.ProviderShareMinor < 0 || i.OrganizationShareMinor < 0:
		return fmt.Errorf("monetary shares cannot be negative")
	case i.PlatformShareMinor != nil && *i.PlatformShareMinor < 0:
		return fmt.Errorf("platform share cannot be negative")
	}
	if i.ProviderShareMinor+i.OrganizationShareMinor != i.GrossBasisMinor {
		// Silently dropping the remainder is how money goes missing without
		// anybody noticing for a month.
		return fmt.Errorf("provider and organization shares must sum to the gross basis")
	}
	return nil
}

// ManagedSettlement is a posted (or attempted) managed settlement.
type ManagedSettlement struct {
	ID                 string `json:"id"`
	InstructionID      string `json:"instruction_id"`
	InstructionVersion int64  `json:"instruction_version"`

	OrganizationID string `json:"organization_id"`
	MembershipID   string `json:"membership_id"`
	ProviderID     string `json:"provider_id"`
	ProviderType   string `json:"provider_type"`

	SourceService      string `json:"source_service"`
	SourceJobID        string `json:"source_job_id"`
	SourceAssignmentID string `json:"source_assignment_id,omitempty"`

	Currency               string `json:"currency"`
	GrossBasisMinor        int64  `json:"gross_basis_minor"`
	ProviderShareMinor     int64  `json:"provider_share_minor"`
	OrganizationShareMinor int64  `json:"organization_share_minor"`
	PlatformShareMinor     int64  `json:"platform_share_minor"`

	CompensationPlanID            string `json:"compensation_plan_id,omitempty"`
	CompensationPlanVersion       int64  `json:"compensation_plan_version,omitempty"`
	CompensationResolutionVersion int64  `json:"compensation_resolution_version,omitempty"`

	Status      string `json:"status"`
	FailureCode string `json:"failure_code,omitempty"`

	LedgerTransactionID   string `json:"ledger_transaction_id,omitempty"`
	ReversalTransactionID string `json:"reversal_transaction_id,omitempty"`

	IdempotencyKey     string `json:"idempotency_key"`
	RequestFingerprint string `json:"request_fingerprint"`
	CorrelationID      string `json:"correlation_id,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	PostedAt   *time.Time `json:"posted_at,omitempty"`
	ReversedAt *time.Time `json:"reversed_at,omitempty"`
}

// Settled reports whether money has actually moved for this instruction.
func (m ManagedSettlement) Settled() bool { return m.Status == ManagedSettlementPosted }

// TotalAllocatedMinor is what the journal distributed.
func (m ManagedSettlement) TotalAllocatedMinor() int64 {
	return m.PlatformShareMinor + m.OrganizationShareMinor + m.ProviderShareMinor
}
