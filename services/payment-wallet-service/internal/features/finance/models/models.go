// Package models holds the authoritative financial domain types.
//
// Every monetary field is an integer count of minor units paired with an
// explicit currency. Balances exposed here are derived from postings or from
// the atomic balance projection — a caller-supplied balance is never stored.
package models

import (
	"encoding/json"
	"fmt"
	"time"

	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/money"
)

// Ledger account owner types.
const (
	OwnerOrganizationWallet = "ORGANIZATION_WALLET"
	OwnerProviderEarning    = "PROVIDER_EARNING"
	OwnerPlatform           = "PLATFORM"
)

// Ledger account codes. Together with owner type and currency these identify
// exactly one account.
const (
	AccountWalletAvailable      = "WALLET_AVAILABLE"
	AccountWalletHeld           = "WALLET_HELD"
	AccountEarningPayable       = "EARNING_PAYABLE"
	AccountEarningHeld          = "EARNING_HELD"
	AccountEarningSettled       = "EARNING_SETTLED"
	AccountPlatformCommission   = "PLATFORM_COMMISSION"
	AccountPlatformFleetFee     = "PLATFORM_FLEET_FEE"
	AccountPlatformReceivable   = "PLATFORM_RECEIVABLE"
	AccountPlatformSubscription = "PLATFORM_SUBSCRIPTION_REVENUE"
	AccountPlatformClearing     = "PLATFORM_SETTLEMENT_CLEARING"
	AccountPlatformRefund       = "PLATFORM_REFUND_CLEARING"
)

// Ledger sides and normal balances.
const (
	SideDebit  = "DEBIT"
	SideCredit = "CREDIT"
)

// Account statuses.
const (
	StatusActive     = "ACTIVE"
	StatusRestricted = "RESTRICTED"
	StatusSuspended  = "SUSPENDED"
	StatusClosed     = "CLOSED"
)

// Journal types.
const (
	JournalInvoicePayment      = "INVOICE_PAYMENT"
	JournalEarningAllocation   = "EARNING_ALLOCATION"
	JournalHoldPlaced          = "HOLD_PLACED"
	JournalHoldReleased        = "HOLD_RELEASED"
	JournalSettlementPayout    = "SETTLEMENT_PAYOUT"
	JournalSettlementReversal  = "SETTLEMENT_REVERSAL"
	JournalRefund              = "REFUND"
	JournalReversal            = "REVERSAL"
	JournalWalletTopUp         = "WALLET_TOPUP"
	JournalSubscriptionInvoice = "SUBSCRIPTION_INVOICE"
)

// LedgerAccount is one account in the chart of accounts.
type LedgerAccount struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId,omitempty"`
	OwnerType      string    `json:"ownerType"`
	OwnerRef       string    `json:"ownerRef"`
	AccountCode    string    `json:"accountCode"`
	Currency       string    `json:"currency"`
	NormalBalance  string    `json:"normalBalance"`
	Status         string    `json:"status"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Balance is the atomic balance projection for one ledger account.
type Balance struct {
	LedgerAccountID string    `json:"ledgerAccountId"`
	Currency        string    `json:"currency"`
	DebitMinor      int64     `json:"debitMinor"`
	CreditMinor     int64     `json:"creditMinor"`
	NormalBalance   string    `json:"normalBalance"`
	Version         int64     `json:"version"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// Amount returns the signed balance in the account's normal direction.
func (b Balance) Amount() money.Money {
	amount := b.DebitMinor - b.CreditMinor
	if b.NormalBalance == SideCredit {
		amount = b.CreditMinor - b.DebitMinor
	}
	return money.Money{AmountMinor: amount, Currency: b.Currency}
}

// JournalLine is one immutable posting line.
type JournalLine struct {
	ID              string    `json:"id"`
	JournalID       string    `json:"journalId"`
	LineNo          int       `json:"lineNo"`
	LedgerAccountID string    `json:"ledgerAccountId"`
	Side            string    `json:"side"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	Memo            string    `json:"memo,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

// Journal is one immutable posted double-entry transaction.
type Journal struct {
	ID                 string        `json:"id"`
	OrganizationID     string        `json:"organizationId,omitempty"`
	JournalType        string        `json:"journalType"`
	Currency           string        `json:"currency"`
	Status             string        `json:"status"`
	Reference          string        `json:"reference,omitempty"`
	SourceService      string        `json:"sourceService,omitempty"`
	SourceReference    string        `json:"sourceReference,omitempty"`
	IdempotencyKey     string        `json:"idempotencyKey,omitempty"`
	RequestFingerprint string        `json:"-"`
	CorrelationID      string        `json:"correlationId,omitempty"`
	CausationID        string        `json:"causationId,omitempty"`
	ReversesJournalID  string        `json:"reversesJournalId,omitempty"`
	PostedByActorType  string        `json:"postedByActorType"`
	PostedByActorID    string        `json:"postedByActorId"`
	Memo               string        `json:"memo,omitempty"`
	PostedAt           time.Time     `json:"postedAt"`
	Lines              []JournalLine `json:"lines,omitempty"`
}

// ValidateBalanced checks a proposed set of lines before it reaches the
// database: at least two lines, a single currency matching the journal, every
// amount strictly positive, and debits equal to credits.
//
// The database enforces the same invariant with a deferred constraint trigger.
// This check exists so the caller gets a precise, actionable error instead of
// a raised exception at COMMIT.
func ValidateBalanced(currency string, lines []JournalLine) error {
	if len(lines) < 2 {
		return fmt.Errorf("finance: a journal must contain at least two lines")
	}
	var debit, credit int64
	for _, line := range lines {
		if line.AmountMinor <= 0 {
			return fmt.Errorf("finance: journal line %d amount must be positive", line.LineNo)
		}
		if line.Currency != currency {
			return fmt.Errorf("finance: journal line %d currency %q does not match journal currency %q",
				line.LineNo, line.Currency, currency)
		}
		switch line.Side {
		case SideDebit:
			debit += line.AmountMinor
		case SideCredit:
			credit += line.AmountMinor
		default:
			return fmt.Errorf("finance: journal line %d side %q is invalid", line.LineNo, line.Side)
		}
	}
	if debit != credit {
		return fmt.Errorf("finance: journal is unbalanced (debit %d, credit %d)", debit, credit)
	}
	return nil
}

// Wallet is an organization's authoritative wallet.
type Wallet struct {
	ID                     string          `json:"id"`
	OrganizationID         string          `json:"organizationId"`
	Currency               string          `json:"currency"`
	Status                 string          `json:"status"`
	AvailableMinor         int64           `json:"availableMinor"`
	HeldMinor              int64           `json:"heldMinor"`
	PendingSettlementMinor int64           `json:"pendingSettlementMinor"`
	Restrictions           json.RawMessage `json:"restrictions"`
	LedgerAccountID        string          `json:"ledgerAccountId"`
	Version                int64           `json:"version"`
	CreatedAt              time.Time       `json:"createdAt"`
	UpdatedAt              time.Time       `json:"updatedAt"`
}

// Spendable reports whether the wallet may fund an outgoing operation.
func (w Wallet) Spendable() bool { return w.Status == StatusActive }

// EarningAccount is a provider's authoritative earning account.
//
// It holds only an opaque reference to the provider's identity: the provider
// record itself belongs to the provider service and is never duplicated here.
type EarningAccount struct {
	ID                   string          `json:"id"`
	ProviderReference    string          `json:"providerReference"`
	ProviderService      string          `json:"providerService"`
	OrganizationID       string          `json:"organizationId,omitempty"`
	Currency             string          `json:"currency"`
	Status               string          `json:"status"`
	PayableMinor         int64           `json:"payableMinor"`
	HeldMinor            int64           `json:"heldMinor"`
	SettledMinor         int64           `json:"settledMinor"`
	Restrictions         json.RawMessage `json:"restrictions"`
	PayoutDestinationRef string          `json:"payoutDestinationRef,omitempty"`
	LedgerAccountID      string          `json:"ledgerAccountId"`
	Version              int64           `json:"version"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

// Payable reports whether the account may be settled.
func (e EarningAccount) Payable() bool { return e.Status == StatusActive }

// Plan billing intervals.
const (
	IntervalWeek  = "WEEK"
	IntervalMonth = "MONTH"
	IntervalYear  = "YEAR"
)

// Plan statuses.
const (
	PlanDraft   = "DRAFT"
	PlanActive  = "ACTIVE"
	PlanRetired = "RETIRED"
)

// SubscriptionPlan is a configured billing plan.
type SubscriptionPlan struct {
	ID              string     `json:"id"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	Currency        string     `json:"currency"`
	PriceMinor      *int64     `json:"priceMinor"`
	BillingInterval string     `json:"billingInterval"`
	IntervalCount   int        `json:"intervalCount"`
	GraceDays       int        `json:"graceDays"`
	Status          string     `json:"status"`
	EffectiveFrom   time.Time  `json:"effectiveFrom"`
	EffectiveTo     *time.Time `json:"effectiveTo,omitempty"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// Billable reports whether the plan may produce an invoice right now.
//
// A plan with no configured price is never billable: inventing a price is the
// one thing an invoicing system must not do.
func (p SubscriptionPlan) Billable(now time.Time) bool {
	if p.Status != PlanActive || p.PriceMinor == nil {
		return false
	}
	if now.Before(p.EffectiveFrom) {
		return false
	}
	return p.EffectiveTo == nil || now.Before(*p.EffectiveTo)
}

// PeriodEnd returns the end of a billing period starting at start.
func (p SubscriptionPlan) PeriodEnd(start time.Time) time.Time {
	count := p.IntervalCount
	if count <= 0 {
		count = 1
	}
	switch p.BillingInterval {
	case IntervalWeek:
		return start.AddDate(0, 0, 7*count)
	case IntervalYear:
		return start.AddDate(count, 0, 0)
	default:
		return start.AddDate(0, count, 0)
	}
}

// Subscription statuses.
const (
	SubscriptionPending     = "PENDING"
	SubscriptionActive      = "ACTIVE"
	SubscriptionPastDue     = "PAST_DUE"
	SubscriptionGracePeriod = "GRACE_PERIOD"
	SubscriptionSuspended   = "SUSPENDED"
	SubscriptionCancelled   = "CANCELLED"
	SubscriptionExpired     = "EXPIRED"
)

// Subscription is an organization's authoritative subscription state.
type Subscription struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	PlanID             string     `json:"planId"`
	Currency           string     `json:"currency"`
	Status             string     `json:"status"`
	CurrentPeriodStart *time.Time `json:"currentPeriodStart,omitempty"`
	CurrentPeriodEnd   *time.Time `json:"currentPeriodEnd,omitempty"`
	GraceExpiresAt     *time.Time `json:"graceExpiresAt,omitempty"`
	ActivatedAt        *time.Time `json:"activatedAt,omitempty"`
	CancelledAt        *time.Time `json:"cancelledAt,omitempty"`
	LatestInvoiceID    string     `json:"latestInvoiceId,omitempty"`
	IdempotencyKey     string     `json:"-"`
	RequestFingerprint string     `json:"-"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// EntitlesPaidFeatures reports whether the subscription currently authorizes
// paid features. PAST_DUE and SUSPENDED do not; a grace period does, because
// that is what a grace period is for.
func (s Subscription) EntitlesPaidFeatures(now time.Time) bool {
	switch s.Status {
	case SubscriptionActive:
		return s.CurrentPeriodEnd == nil || now.Before(*s.CurrentPeriodEnd)
	case SubscriptionGracePeriod:
		return s.GraceExpiresAt != nil && now.Before(*s.GraceExpiresAt)
	default:
		return false
	}
}

// Invoice statuses.
const (
	InvoiceDraft         = "DRAFT"
	InvoiceOpen          = "OPEN"
	InvoicePartiallyPaid = "PARTIALLY_PAID"
	InvoicePaid          = "PAID"
	InvoiceVoid          = "VOID"
	InvoiceUncollectible = "UNCOLLECTIBLE"
	InvoiceExpired       = "EXPIRED"
)

// InvoiceLine is one billed item.
type InvoiceLine struct {
	ID              string    `json:"id"`
	InvoiceID       string    `json:"invoiceId"`
	LineNo          int       `json:"lineNo"`
	Description     string    `json:"description"`
	Quantity        int       `json:"quantity"`
	UnitAmountMinor int64     `json:"unitAmountMinor"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	CreatedAt       time.Time `json:"createdAt"`
}

// Invoice is the authoritative bill.
type Invoice struct {
	ID                 string        `json:"id"`
	OrganizationID     string        `json:"organizationId"`
	SubscriptionID     string        `json:"subscriptionId,omitempty"`
	Number             string        `json:"number"`
	Currency           string        `json:"currency"`
	SubtotalMinor      int64         `json:"subtotalMinor"`
	TaxMinor           int64         `json:"taxMinor"`
	TotalMinor         int64         `json:"totalMinor"`
	AmountPaidMinor    int64         `json:"amountPaidMinor"`
	Status             string        `json:"status"`
	PeriodStart        *time.Time    `json:"periodStart,omitempty"`
	PeriodEnd          *time.Time    `json:"periodEnd,omitempty"`
	IssuedAt           time.Time     `json:"issuedAt"`
	DueAt              time.Time     `json:"dueAt"`
	PaidAt             *time.Time    `json:"paidAt,omitempty"`
	IdempotencyKey     string        `json:"-"`
	RequestFingerprint string        `json:"-"`
	CorrelationID      string        `json:"correlationId,omitempty"`
	Version            int64         `json:"version"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	Lines              []InvoiceLine `json:"lines,omitempty"`
}

// OutstandingMinor is the amount still owed.
func (i Invoice) OutstandingMinor() int64 { return i.TotalMinor - i.AmountPaidMinor }

// Payment statuses.
const (
	PaymentPending   = "PENDING"
	PaymentSucceeded = "SUCCEEDED"
	PaymentFailed    = "FAILED"
	PaymentUnknown   = "UNKNOWN"
	PaymentReversed  = "REVERSED"
)

// Payment records money received against an invoice.
type Payment struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organizationId"`
	InvoiceID          string    `json:"invoiceId"`
	AmountMinor        int64     `json:"amountMinor"`
	Currency           string    `json:"currency"`
	Method             string    `json:"method"`
	Status             string    `json:"status"`
	ExternalReference  string    `json:"externalReference,omitempty"`
	JournalID          string    `json:"journalId,omitempty"`
	IdempotencyKey     string    `json:"-"`
	RequestFingerprint string    `json:"-"`
	CorrelationID      string    `json:"correlationId,omitempty"`
	LastError          string    `json:"lastError,omitempty"`
	Version            int64     `json:"version"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// CommissionRule is a versioned marketplace calculation rule.
type CommissionRule struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId,omitempty"`
	ServiceType          string     `json:"serviceType"`
	Currency             string     `json:"currency"`
	RuleVersion          int        `json:"ruleVersion"`
	CommissionBPS        int        `json:"commissionBps"`
	FleetFeeBPS          int        `json:"fleetFeeBps"`
	ProviderShareBPS     int        `json:"providerShareBps"`
	OrganizationShareBPS int        `json:"organizationShareBps"`
	Status               string     `json:"status"`
	EffectiveFrom        time.Time  `json:"effectiveFrom"`
	EffectiveTo          *time.Time `json:"effectiveTo,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
}

// Split is the reproducible result of applying a commission rule to a gross
// amount.
type Split struct {
	Gross               money.Money `json:"gross"`
	Commission          money.Money `json:"commission"`
	FleetFee            money.Money `json:"fleetFee"`
	ProviderEarning     money.Money `json:"providerEarning"`
	OrganizationEarning money.Money `json:"organizationEarning"`
}

// Apply computes the split for gross under this rule.
//
// The residual after commission and fleet fee is divided by basis points, and
// the organization takes the remainder rather than a second rounded share.
// That is what guarantees the four parts add back up to the gross exactly, and
// therefore that the derived journal balances.
func (r CommissionRule) Apply(gross money.Money) (Split, error) {
	if err := gross.Validate(); err != nil {
		return Split{}, err
	}
	if gross.AmountMinor <= 0 {
		return Split{}, fmt.Errorf("finance: gross amount must be positive")
	}
	if gross.Currency != r.Currency {
		return Split{}, fmt.Errorf("%w: rule %s vs amount %s",
			money.ErrCurrencyMismatch, r.Currency, gross.Currency)
	}

	commission, err := gross.ApplyBasisPoints(int64(r.CommissionBPS))
	if err != nil {
		return Split{}, err
	}
	fleetFee, err := gross.ApplyBasisPoints(int64(r.FleetFeeBPS))
	if err != nil {
		return Split{}, err
	}
	residual := money.Money{
		AmountMinor: gross.AmountMinor - commission.AmountMinor - fleetFee.AmountMinor,
		Currency:    gross.Currency,
	}
	if residual.AmountMinor < 0 {
		return Split{}, fmt.Errorf("finance: commission and fleet fee exceed the gross amount")
	}
	providerEarning, err := residual.ApplyBasisPoints(int64(r.ProviderShareBPS))
	if err != nil {
		return Split{}, err
	}
	organizationEarning := money.Money{
		AmountMinor: residual.AmountMinor - providerEarning.AmountMinor,
		Currency:    gross.Currency,
	}

	return Split{
		Gross:               gross,
		Commission:          commission,
		FleetFee:            fleetFee,
		ProviderEarning:     providerEarning,
		OrganizationEarning: organizationEarning,
	}, nil
}

// Snapshot renders the rule exactly as applied, for storage next to the
// allocation it produced.
func (r CommissionRule) Snapshot() map[string]any {
	return map[string]any{
		"ruleId":               r.ID,
		"ruleVersion":          r.RuleVersion,
		"serviceType":          r.ServiceType,
		"currency":             r.Currency,
		"commissionBps":        r.CommissionBPS,
		"fleetFeeBps":          r.FleetFeeBPS,
		"providerShareBps":     r.ProviderShareBPS,
		"organizationShareBps": r.OrganizationShareBPS,
		"effectiveFrom":        r.EffectiveFrom.UTC().Format(time.RFC3339),
	}
}

// Allocation statuses.
const (
	AllocationPosted   = "POSTED"
	AllocationReversed = "REVERSED"
)

// EarningAllocation is a posted marketplace earning split.
type EarningAllocation struct {
	ID                       string          `json:"id"`
	OrganizationID           string          `json:"organizationId"`
	SourceService            string          `json:"sourceService"`
	JobReference             string          `json:"jobReference"`
	Currency                 string          `json:"currency"`
	RuleID                   string          `json:"ruleId"`
	RuleVersion              int             `json:"ruleVersion"`
	RuleSnapshot             json.RawMessage `json:"ruleSnapshot"`
	GrossMinor               int64           `json:"grossMinor"`
	CommissionMinor          int64           `json:"commissionMinor"`
	FleetFeeMinor            int64           `json:"fleetFeeMinor"`
	ProviderEarningMinor     int64           `json:"providerEarningMinor"`
	OrganizationEarningMinor int64           `json:"organizationEarningMinor"`
	ProviderReference        string          `json:"providerReference,omitempty"`
	EarningAccountID         string          `json:"earningAccountId,omitempty"`
	WalletID                 string          `json:"walletId,omitempty"`
	JournalID                string          `json:"journalId"`
	Status                   string          `json:"status"`
	IdempotencyKey           string          `json:"-"`
	RequestFingerprint       string          `json:"-"`
	CorrelationID            string          `json:"correlationId,omitempty"`
	CausationID              string          `json:"causationId,omitempty"`
	Version                  int64           `json:"version"`
	CreatedAt                time.Time       `json:"createdAt"`
	UpdatedAt                time.Time       `json:"updatedAt"`
}

// Hold subject types and states.
const (
	HoldSubjectWallet         = "WALLET"
	HoldSubjectEarningAccount = "EARNING_ACCOUNT"

	HoldProvisional = "PROVISIONAL"
	HoldActive      = "ACTIVE"
	HoldReleased    = "RELEASED"
	HoldExpired     = "EXPIRED"
	HoldCancelled   = "CANCELLED"
)

// Hold reserves funds against an authoritative reference.
type Hold struct {
	ID                     string     `json:"id"`
	OrganizationID         string     `json:"organizationId"`
	SubjectType            string     `json:"subjectType"`
	WalletID               string     `json:"walletId,omitempty"`
	EarningAccountID       string     `json:"earningAccountId,omitempty"`
	ReasonCode             string     `json:"reasonCode"`
	Reason                 string     `json:"reason,omitempty"`
	AmountMinor            int64      `json:"amountMinor"`
	Currency               string     `json:"currency"`
	AuthorityService       string     `json:"authorityService,omitempty"`
	AuthoritativeReference string     `json:"authoritativeReference,omitempty"`
	ReferenceVerified      bool       `json:"referenceVerified"`
	State                  string     `json:"state"`
	ExpiresAt              *time.Time `json:"expiresAt,omitempty"`
	ReleasedAt             *time.Time `json:"releasedAt,omitempty"`
	JournalID              string     `json:"journalId,omitempty"`
	ReleaseJournalID       string     `json:"releaseJournalId,omitempty"`
	IdempotencyKey         string     `json:"-"`
	RequestFingerprint     string     `json:"-"`
	CorrelationID          string     `json:"correlationId,omitempty"`
	CausationID            string     `json:"causationId,omitempty"`
	Version                int64      `json:"version"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

// Holding reports whether the hold currently reserves funds.
func (h Hold) Holding() bool { return h.State == HoldProvisional || h.State == HoldActive }

// Settlement batch and item statuses.
const (
	BatchPendingApproval = "PENDING_APPROVAL"
	BatchApproved        = "APPROVED"
	BatchProcessing      = "PROCESSING"
	BatchCompleted       = "COMPLETED"
	BatchPartiallyFailed = "PARTIALLY_FAILED"
	BatchCancelled       = "CANCELLED"

	SettlementPending          = "PENDING"
	SettlementProcessing       = "PROCESSING"
	SettlementUnknown          = "UNKNOWN"
	SettlementSucceeded        = "SUCCEEDED"
	SettlementRetryableFailure = "RETRYABLE_FAILURE"
	SettlementPermanentFailure = "PERMANENT_FAILURE"
	SettlementReversed         = "REVERSED"
	SettlementReconciling      = "RECONCILING"
	SettlementDeadLetter       = "DEAD_LETTER"

	BeneficiaryProvider = "PROVIDER"
	BeneficiaryCompany  = "COMPANY"
)

// SettlementBatch groups payouts to one beneficiary.
type SettlementBatch struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId"`
	BeneficiaryType      string     `json:"beneficiaryType"`
	BeneficiaryReference string     `json:"beneficiaryReference"`
	EarningAccountID     string     `json:"earningAccountId,omitempty"`
	WalletID             string     `json:"walletId,omitempty"`
	Currency             string     `json:"currency"`
	TotalMinor           int64      `json:"totalMinor"`
	ItemCount            int        `json:"itemCount"`
	Status               string     `json:"status"`
	RequestedByType      string     `json:"requestedByType,omitempty"`
	RequestedByID        string     `json:"requestedById,omitempty"`
	ApprovedByType       string     `json:"approvedByType,omitempty"`
	ApprovedByID         string     `json:"approvedById,omitempty"`
	ApprovedAt           *time.Time `json:"approvedAt,omitempty"`
	IdempotencyKey       string     `json:"-"`
	RequestFingerprint   string     `json:"-"`
	CorrelationID        string     `json:"correlationId,omitempty"`
	Version              int64      `json:"version"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// RequesterIdentity is the identity compared for separation of duties.
func (b SettlementBatch) RequesterIdentity() string {
	if b.RequestedByType == "" || b.RequestedByID == "" {
		return ""
	}
	return b.RequestedByType + ":" + b.RequestedByID
}

// SettlementItem is one durable payout attempt.
type SettlementItem struct {
	ID                  string     `json:"id"`
	BatchID             string     `json:"batchId"`
	OrganizationID      string     `json:"organizationId"`
	EarningAllocationID string     `json:"earningAllocationId,omitempty"`
	AmountMinor         int64      `json:"amountMinor"`
	Currency            string     `json:"currency"`
	Status              string     `json:"status"`
	Attempts            int        `json:"attempts"`
	NextAttemptAt       time.Time  `json:"nextAttemptAt"`
	LeaseOwner          string     `json:"-"`
	LeaseExpiresAt      *time.Time `json:"-"`
	ExternalReference   string     `json:"externalReference,omitempty"`
	JournalID           string     `json:"journalId,omitempty"`
	ReversalJournalID   string     `json:"reversalJournalId,omitempty"`
	IdempotencyKey      string     `json:"-"`
	LastError           string     `json:"lastError,omitempty"`
	Version             int64      `json:"version"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	CompletedAt         *time.Time `json:"completedAt,omitempty"`
}

// NeedsReadBack reports whether the next attempt must resolve an ambiguous
// outcome by reading the payout back rather than issuing a fresh payout.
func (i SettlementItem) NeedsReadBack() bool {
	return i.Status == SettlementUnknown || i.Status == SettlementReconciling
}

// Refund statuses.
const (
	RefundRequested   = "REQUESTED"
	RefundApproved    = "APPROVED"
	RefundRejected    = "REJECTED"
	RefundProcessing  = "PROCESSING"
	RefundUnknown     = "UNKNOWN"
	RefundCompleted   = "COMPLETED"
	RefundFailed      = "FAILED"
	RefundReconciling = "RECONCILING"
	RefundDeadLetter  = "DEAD_LETTER"
	RefundReversed    = "REVERSED"
)

// Refund reverses value against an original posted transaction.
type Refund struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	OriginalJournalID  string     `json:"originalJournalId"`
	OriginalReference  string     `json:"originalReference,omitempty"`
	AmountMinor        int64      `json:"amountMinor"`
	Currency           string     `json:"currency"`
	ReasonCode         string     `json:"reasonCode,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	Status             string     `json:"status"`
	RequestedByType    string     `json:"requestedByType,omitempty"`
	RequestedByID      string     `json:"requestedById,omitempty"`
	ApprovedByType     string     `json:"approvedByType,omitempty"`
	ApprovedByID       string     `json:"approvedById,omitempty"`
	ApprovedAt         *time.Time `json:"approvedAt,omitempty"`
	Attempts           int        `json:"attempts"`
	NextAttemptAt      time.Time  `json:"nextAttemptAt"`
	LeaseOwner         string     `json:"-"`
	LeaseExpiresAt     *time.Time `json:"-"`
	ExternalReference  string     `json:"externalReference,omitempty"`
	JournalID          string     `json:"journalId,omitempty"`
	ReversalJournalID  string     `json:"reversalJournalId,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
	IdempotencyKey     string     `json:"-"`
	RequestFingerprint string     `json:"-"`
	CorrelationID      string     `json:"correlationId,omitempty"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
}

// RequesterIdentity is the identity compared for separation of duties.
func (r Refund) RequesterIdentity() string {
	if r.RequestedByType == "" || r.RequestedByID == "" {
		return ""
	}
	return r.RequestedByType + ":" + r.RequestedByID
}

// NeedsReadBack reports whether the outcome must be established by reading it
// back rather than by issuing a fresh refund.
func (r Refund) NeedsReadBack() bool {
	return r.Status == RefundUnknown || r.Status == RefundReconciling
}

// Reversal statuses. Every outcome an executor can observe has a name: an
// ambiguous external result is UNKNOWN and is resolved by reading the
// authoritative operation back, never by issuing a second reversal.
const (
	ReversalRequested       = "REQUESTED"
	ReversalPendingApproval = "PENDING_APPROVAL"
	ReversalApproved        = "APPROVED"
	ReversalProcessing      = "PROCESSING"
	ReversalUnknown         = "UNKNOWN"
	ReversalCompleted       = "COMPLETED"
	ReversalFailedRetryable = "FAILED_RETRYABLE"
	ReversalFailedPermanent = "FAILED_PERMANENT"
	ReversalRejected        = "REJECTED"
	ReversalReconciling     = "RECONCILING"
	ReversalDeadLetter      = "DEAD_LETTER"
)

// Reversal is the durable contract for compensating an already posted entry.
//
// The original journal is immutable. Completion records the compensating
// journal in ResultingJournalID; the pair, read together, is the audit trail.
type Reversal struct {
	ID                         string     `json:"id"`
	OrganizationID             string     `json:"organizationId"`
	OriginalJournalID          string     `json:"originalJournalId"`
	OriginalReference          string     `json:"originalReference,omitempty"`
	ReasonCode                 string     `json:"reasonCode,omitempty"`
	Reason                     string     `json:"reason,omitempty"`
	AmountMinor                int64      `json:"amountMinor"`
	Currency                   string     `json:"currency"`
	RequestFingerprint         string     `json:"-"`
	IdempotencyKey             string     `json:"-"`
	ExpectedVersion            int64      `json:"expectedVersion"`
	Status                     string     `json:"status"`
	ExternalOperationReference string     `json:"externalOperationReference,omitempty"`
	Attempts                   int        `json:"attempts"`
	NextAttemptAt              time.Time  `json:"nextAttemptAt"`
	LeaseOwner                 string     `json:"-"`
	LeaseExpiresAt             *time.Time `json:"-"`
	CorrelationID              string     `json:"correlationId,omitempty"`
	CausationID                string     `json:"causationId,omitempty"`
	CreatedByActorType         string     `json:"createdByActorType,omitempty"`
	CreatedByActorID           string     `json:"createdByActorId,omitempty"`
	ApprovedByActorType        string     `json:"approvedByActorType,omitempty"`
	ApprovedByActorID          string     `json:"approvedByActorId,omitempty"`
	ApprovalReference          string     `json:"approvalReference,omitempty"`
	ApprovalPolicyVersion      int        `json:"approvalPolicyVersion,omitempty"`
	ApprovedAt                 *time.Time `json:"approvedAt,omitempty"`
	ResultingJournalID         string     `json:"resultingJournalId,omitempty"`
	SafeErrorCode              string     `json:"safeErrorCode,omitempty"`
	Version                    int64      `json:"version"`
	CreatedAt                  time.Time  `json:"createdAt"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
	CompletedAt                *time.Time `json:"completedAt,omitempty"`
}

// RequesterIdentity is the identity compared for separation of duties.
func (r Reversal) RequesterIdentity() string {
	if r.CreatedByActorType == "" || r.CreatedByActorID == "" {
		return ""
	}
	return r.CreatedByActorType + ":" + r.CreatedByActorID
}

// NeedsReadBack reports whether the outcome must be established by reading the
// authoritative operation back rather than by executing again.
func (r Reversal) NeedsReadBack() bool {
	return r.Status == ReversalUnknown || r.Status == ReversalReconciling
}

// Terminal reports whether no further execution will occur.
func (r Reversal) Terminal() bool {
	switch r.Status {
	case ReversalCompleted, ReversalRejected, ReversalFailedPermanent, ReversalDeadLetter:
		return true
	default:
		return false
	}
}

// Job payment statuses. These describe an authoritative payment for one unit
// of dispatched work, not a wallet balance.
const (
	JobPaymentPending           = "PENDING"
	JobPaymentAuthorized        = "AUTHORIZED"
	JobPaymentPaid              = "PAID"
	JobPaymentFailed            = "FAILED"
	JobPaymentRefunded          = "REFUNDED"
	JobPaymentPartiallyRefunded = "PARTIALLY_REFUNDED"
	JobPaymentReversed          = "REVERSED"
	JobPaymentUnknown           = "UNKNOWN"
)

// Refund and reversal sub-states carried on a job payment.
const (
	JobPaymentStateNone       = "NONE"
	JobPaymentStateRequested  = "REQUESTED"
	JobPaymentStateProcessing = "PROCESSING"
	JobPaymentStateCompleted  = "COMPLETED"
	JobPaymentStateFailed     = "FAILED"
)

// Supported authoritative lookup keys. No new identifier is minted: each key is
// a reference another service already owns.
const (
	JobPaymentByDispatchRequest = "DISPATCH_REQUEST"
	JobPaymentByTrip            = "TRIP"
	JobPaymentByBooking         = "BOOKING"
	JobPaymentByPaymentRef      = "PAYMENT_REFERENCE"
)

// JobPayment is Wallet's authoritative record that a unit of dispatched work
// was paid for.
type JobPayment struct {
	ID                       string     `json:"id"`
	OrganizationID           string     `json:"organizationId"`
	SourceService            string     `json:"sourceService"`
	DispatchRequestReference string     `json:"dispatchRequestReference,omitempty"`
	TripReference            string     `json:"tripReference,omitempty"`
	BookingReference         string     `json:"bookingReference,omitempty"`
	PaymentReference         string     `json:"paymentReference,omitempty"`
	AmountMinor              int64      `json:"amountMinor"`
	Currency                 string     `json:"currency"`
	Status                   string     `json:"status"`
	PaidAt                   *time.Time `json:"paidAt,omitempty"`
	JournalID                string     `json:"-"`
	RefundState              string     `json:"refundState"`
	ReversalState            string     `json:"reversalState"`
	OperationReference       string     `json:"operationReference,omitempty"`
	IdempotencyKey           string     `json:"-"`
	RequestFingerprint       string     `json:"-"`
	CorrelationID            string     `json:"-"`
	Version                  int64      `json:"version"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

// Settled reports whether the payment currently authorizes downstream money
// movement. Anything unknown, failed, refunded or reversed does not.
func (p JobPayment) Settled() bool {
	if p.Status != JobPaymentPaid {
		return false
	}
	return p.RefundState == JobPaymentStateNone && p.ReversalState == JobPaymentStateNone
}

// Statement types.
const (
	StatementTransaction = "TRANSACTION"
	StatementSettlement  = "SETTLEMENT"
)

// StatementLine is one row of an exported statement.
type StatementLine struct {
	JournalID   string    `json:"journalId"`
	JournalType string    `json:"journalType"`
	Reference   string    `json:"reference,omitempty"`
	Side        string    `json:"side"`
	AmountMinor int64     `json:"amountMinor"`
	Currency    string    `json:"currency"`
	Memo        string    `json:"memo,omitempty"`
	PostedAt    time.Time `json:"postedAt"`
}

// Statement is a generated, hash-stamped export.
type Statement struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	StatementType    string          `json:"statementType"`
	Currency         string          `json:"currency"`
	PeriodStart      time.Time       `json:"periodStart"`
	PeriodEnd        time.Time       `json:"periodEnd"`
	LineCount        int             `json:"lineCount"`
	TotalDebitMinor  int64           `json:"totalDebitMinor"`
	TotalCreditMinor int64           `json:"totalCreditMinor"`
	ContentHash      string          `json:"contentHash"`
	GeneratedByType  string          `json:"generatedByType,omitempty"`
	GeneratedByID    string          `json:"generatedById,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	Lines            []StatementLine `json:"lines,omitempty"`
}

// Reconciliation finding codes.
const (
	FindingBalanceDrift        = "BALANCE_PROJECTION_DRIFT"
	FindingUnbalancedJournal   = "UNBALANCED_JOURNAL"
	FindingStuckSettlement     = "SETTLEMENT_UNKNOWN_OUTCOME"
	FindingStuckRefund         = "REFUND_UNKNOWN_OUTCOME"
	FindingExpiredHoldActive   = "HOLD_EXPIRED_BUT_ACTIVE"
	FindingInvoiceOverpaid     = "INVOICE_OVERPAID"
	FindingWalletProjectionOff = "WALLET_PROJECTION_DRIFT"
	FindingStuckReversal       = "REVERSAL_UNKNOWN_OUTCOME"
	FindingReversalNoJournal   = "REVERSAL_COMPLETED_WITHOUT_JOURNAL"
)

// ReconciliationRun is one reconciliation pass.
type ReconciliationRun struct {
	ID            string     `json:"id"`
	Scope         string     `json:"scope"`
	Status        string     `json:"status"`
	FindingsCount int        `json:"findingsCount"`
	CheckedCount  int        `json:"checkedCount"`
	StartedAt     time.Time  `json:"startedAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

// ReconciliationFinding is one detected discrepancy.
type ReconciliationFinding struct {
	ID             string          `json:"id"`
	RunID          string          `json:"runId"`
	OrganizationID string          `json:"organizationId,omitempty"`
	EntityType     string          `json:"entityType"`
	EntityID       string          `json:"entityId"`
	FindingCode    string          `json:"findingCode"`
	Details        json.RawMessage `json:"details"`
	ResolvedAt     *time.Time      `json:"resolvedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}
