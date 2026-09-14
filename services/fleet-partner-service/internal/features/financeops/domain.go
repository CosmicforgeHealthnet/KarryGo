// Package financeops implements Fleet-owned billing and compensation records.
// Wallet balances, ledger entries and payment execution remain external.
package financeops

import (
	"encoding/json"
	"time"
)

type Flags struct {
	Invoices, Receivables, Expenses, Budgets, Compensation, Payroll bool
	Settlements, Claims, Posting, Workers, KillSwitch               bool
}

type InvoiceLineInput struct {
	SourceType, SourceReference, Description string
	Quantity                                 int64
	UnitAmountMinor, TaxAmountMinor          int64
	TaxReference, AdjustmentReference        string
}

type InvoiceInput struct {
	BranchID, CustomerID, ContractID, QuotationID, Currency string
	DueDate                                                 time.Time
	Lines                                                   []InvoiceLineInput
	ExpectedVersion                                         int64
}

type CreditNoteInput struct {
	AmountMinor     int64           `json:"amountMinor"`
	AffectedLines   json.RawMessage `json:"affectedLines"`
	Reason          string          `json:"reason"`
	ExpectedVersion int64           `json:"expectedVersion"`
}

type PaymentEvent struct {
	EventID, OrganizationID, InvoiceID, PaymentReference, WalletOperationReference, Currency string
	AmountMinor, SourceVersion                                                               int64
}

type ExpenseInput struct {
	BranchID, Category, CostCentre, AssetID, ProviderMembershipID string
	OrderID, ShipmentID, WorkOrderID, IncidentID                  string
	AmountMinor                                                   int64
	Currency, ReceiptMediaID, Description                         string
	ExpectedVersion                                               int64
}

type BudgetInput struct {
	CostCentre, Category, Currency string
	PeriodStart, PeriodEnd         time.Time
	ApprovedAmountMinor            int64
	ExpectedVersion                int64
}

type CompensationRuleInput struct {
	Name, RuleType, Currency            string
	ApplicableRoles, ApplicableServices json.RawMessage
	CalculationConfiguration            json.RawMessage
	EffectiveFrom                       time.Time
	EffectiveUntil                      *time.Time
	ExpectedVersion                     int64
}

type PayrollPeriodInput struct {
	BranchID, Currency     string
	PeriodStart, PeriodEnd time.Time
	ExpectedVersion        int64
}

type SettlementInput struct {
	SupplierReference, ProviderOrCarrierReference, Currency string
	PeriodStart, PeriodEnd                                  time.Time
	JobReferences                                           []string
	AmountMinor                                             int64
	ExpectedVersion                                         int64
}

type ClaimInput struct {
	ClaimType, IncidentID, AssetID, ShipmentID, PackageID string
	PolicyReference                                       string
	AmountReferenceMinor                                  *int64
	Currency                                              string
	EvidenceMediaIDs                                      []string
	ExternalClaimReference, SettlementReference           string
	ApprovedAmountMinor                                   *int64
	OwnerUserID, Status                                   string
	ExpectedVersion                                       int64
}

type PostingOutcome struct {
	Reference, Status string
	SourceVersion     int64
}

// OperationManagedWorkSettlement is the Section 12 posting operation: one
// Fleet ManagedWorkSettlementInstruction handed to the money authority.
//
// It is named here, beside the posting request it travels in, because the
// retry, read-back and reconciliation machinery that carries every other
// posting carries this one unchanged -- including the rule that an ambiguous
// outcome is read back rather than assumed failed.
const OperationManagedWorkSettlement = "MANAGED_WORK_SETTLEMENT"

type PostingRequest struct {
	OrganizationID, RecordType, RecordID, OperationType, IdempotencyKey string
	AmountMinor, ExpectedVersion                                        int64
	Currency                                                            string
	Payload                                                             json.RawMessage
}

type Statement struct {
	MembershipID string            `json:"membershipId"`
	Periods      []json.RawMessage `json:"periods"`
}
