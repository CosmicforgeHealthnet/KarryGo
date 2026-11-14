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
	SourceType          string `json:"sourceType"`
	SourceReference     string `json:"sourceReference"`
	Description         string `json:"description"`
	Quantity            int64  `json:"quantity"`
	UnitAmountMinor     int64  `json:"unitAmountMinor"`
	TaxAmountMinor      int64  `json:"taxAmountMinor"`
	TaxReference        string `json:"taxReference"`
	AdjustmentReference string `json:"adjustmentReference"`
}

type InvoiceInput struct {
	BranchID        string             `json:"branchId"`
	CustomerID      string             `json:"customerId"`
	ContractID      string             `json:"contractId"`
	QuotationID     string             `json:"quotationId"`
	Currency        string             `json:"currency"`
	DueDate         time.Time          `json:"dueDate"`
	Lines           []InvoiceLineInput `json:"lines"`
	ExpectedVersion int64              `json:"expectedVersion"`
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
	BranchID             string `json:"branchId"`
	Category             string `json:"category"`
	CostCentre           string `json:"costCentre"`
	AssetID              string `json:"assetId"`
	ProviderMembershipID string `json:"providerMembershipId"`
	OrderID              string `json:"orderId"`
	ShipmentID           string `json:"shipmentId"`
	WorkOrderID          string `json:"workOrderId"`
	IncidentID           string `json:"incidentId"`
	AmountMinor          int64  `json:"amountMinor"`
	Currency             string `json:"currency"`
	ReceiptMediaID       string `json:"receiptMediaId"`
	Description          string `json:"description"`
	ExpectedVersion      int64  `json:"expectedVersion"`
}

type BudgetInput struct {
	CostCentre          string    `json:"costCentre"`
	Category            string    `json:"category"`
	Currency            string    `json:"currency"`
	PeriodStart         time.Time `json:"periodStart"`
	PeriodEnd           time.Time `json:"periodEnd"`
	ApprovedAmountMinor int64     `json:"approvedAmountMinor"`
	ExpectedVersion     int64     `json:"expectedVersion"`
}

type CompensationRuleInput struct {
	Name                     string          `json:"name"`
	RuleType                 string          `json:"ruleType"`
	Currency                 string          `json:"currency"`
	ApplicableRoles          json.RawMessage `json:"applicableRoles"`
	ApplicableServices       json.RawMessage `json:"applicableServices"`
	CalculationConfiguration json.RawMessage `json:"calculationConfiguration"`
	EffectiveFrom            time.Time       `json:"effectiveFrom"`
	EffectiveUntil           *time.Time      `json:"effectiveUntil"`
	ExpectedVersion          int64           `json:"expectedVersion"`
}

type PayrollPeriodInput struct {
	BranchID        string    `json:"branchId"`
	Currency        string    `json:"currency"`
	PeriodStart     time.Time `json:"periodStart"`
	PeriodEnd       time.Time `json:"periodEnd"`
	ExpectedVersion int64     `json:"expectedVersion"`
}

type SettlementInput struct {
	SupplierReference          string    `json:"supplierReference"`
	ProviderOrCarrierReference string    `json:"providerOrCarrierReference"`
	Currency                   string    `json:"currency"`
	PeriodStart                time.Time `json:"periodStart"`
	PeriodEnd                  time.Time `json:"periodEnd"`
	JobReferences              []string  `json:"jobReferences"`
	AmountMinor                int64     `json:"amountMinor"`
	ExpectedVersion            int64     `json:"expectedVersion"`
}

type ClaimInput struct {
	ClaimType              string   `json:"claimType"`
	IncidentID             string   `json:"incidentId"`
	AssetID                string   `json:"assetId"`
	ShipmentID             string   `json:"shipmentId"`
	PackageID              string   `json:"packageId"`
	PolicyReference        string   `json:"policyReference"`
	AmountReferenceMinor   *int64   `json:"amountReferenceMinor"`
	Currency               string   `json:"currency"`
	EvidenceMediaIDs       []string `json:"evidenceMediaIds"`
	ExternalClaimReference string   `json:"externalClaimReference"`
	SettlementReference    string   `json:"settlementReference"`
	ApprovedAmountMinor    *int64   `json:"approvedAmountMinor"`
	OwnerUserID            string   `json:"ownerUserId"`
	Status                 string   `json:"status"`
	ExpectedVersion        int64    `json:"expectedVersion"`
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
