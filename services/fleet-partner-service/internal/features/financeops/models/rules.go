package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

// NormalizeCurrency returns a three-letter upper-case ISO code.
func NormalizeCurrency(v string) (string, error) {
	v = strings.ToUpper(strings.TrimSpace(v))
	if len(v) != 3 {
		return "", apperrors.Validation("Currency must be a three-letter ISO code.", nil)
	}
	for _, r := range v {
		if r < 'A' || r > 'Z' {
			return "", apperrors.Validation("Currency must be a three-letter ISO code.", nil)
		}
	}
	return v, nil
}

// RawOr returns v when it is valid JSON, otherwise fallback.
func RawOr(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 || !json.Valid(v) {
		return json.RawMessage(fallback)
	}
	return v
}

// Fingerprint is the SHA-256 of v's JSON encoding.
func Fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Invoices and receivables
// ---------------------------------------------------------------------------

// CalculatedLine is an invoice line with its ID and line total.
type CalculatedLine struct {
	InvoiceLineInput
	ID        string
	LineTotal int64
}

// InvoiceTotals is a validated invoice computed in exact minor units.
type InvoiceTotals struct {
	Currency             string
	Subtotal, Tax, Total int64
	Lines                []CalculatedLine
}

// CalculateInvoice validates an invoice and computes its totals, refusing
// any amount that would overflow.
func CalculateInvoice(in InvoiceInput) (InvoiceTotals, error) {
	currency, err := NormalizeCurrency(in.Currency)
	if err != nil {
		return InvoiceTotals{}, err
	}
	if in.CustomerID == "" || in.DueDate.IsZero() || len(in.Lines) == 0 {
		return InvoiceTotals{}, apperrors.Validation("Customer, due date and at least one line are required.", nil)
	}
	var subtotal, tax int64
	lines := make([]CalculatedLine, 0, len(in.Lines))
	for _, line := range in.Lines {
		line.SourceType = strings.ToUpper(strings.TrimSpace(line.SourceType))
		if line.SourceType == "" || strings.TrimSpace(line.SourceReference) == "" || strings.TrimSpace(line.Description) == "" || line.Quantity <= 0 || line.UnitAmountMinor < 0 || line.TaxAmountMinor < 0 {
			return InvoiceTotals{}, apperrors.Validation("Every invoice line requires a source, description, positive quantity and non-negative minor-unit amounts.", nil)
		}
		if line.UnitAmountMinor > 0 && line.Quantity > math.MaxInt64/line.UnitAmountMinor {
			return InvoiceTotals{}, apperrors.Validation("Invoice line amount is too large.", nil)
		}
		lineTotal := line.Quantity * line.UnitAmountMinor
		if subtotal > math.MaxInt64-lineTotal || tax > math.MaxInt64-line.TaxAmountMinor {
			return InvoiceTotals{}, apperrors.Validation("Invoice total is too large.", nil)
		}
		subtotal += lineTotal
		tax += line.TaxAmountMinor
		lines = append(lines, CalculatedLine{InvoiceLineInput: line, ID: ids.New("inl"), LineTotal: lineTotal})
	}
	if subtotal > math.MaxInt64-tax {
		return InvoiceTotals{}, apperrors.Validation("Invoice total is too large.", nil)
	}
	return InvoiceTotals{Currency: currency, Subtotal: subtotal, Tax: tax, Total: subtotal + tax, Lines: lines}, nil
}

// InvoiceState is the locked state of an invoice.
type InvoiceState struct {
	Branch, Status, Currency string
	Version, Total           int64
	Due                      time.Time
}

// NextInvoiceStatus validates an approve, issue or cancel command.
func NextInvoiceStatus(status, command string) (string, error) {
	switch command {
	case "approve":
		if status != "DRAFT" && status != "PENDING_APPROVAL" {
			return "", apperrors.Conflict("Invoice cannot be approved from its current state.", nil)
		}
		return "APPROVED", nil
	case "issue":
		if status != "APPROVED" {
			return "", apperrors.Conflict("Only an approved invoice may be issued.", nil)
		}
		return "ISSUED", nil
	case "cancel":
		if status == "PAID" || status == "CREDITED" || status == "CANCELLED" {
			return "", apperrors.Conflict("Invoice cannot be cancelled from its current state.", nil)
		}
		return "CANCELLED", nil
	}
	return "", apperrors.Validation("Unsupported invoice command.", nil)
}

// ReceivableState is the locked state of an invoice's receivable.
type ReceivableState struct {
	ID, Currency                               string
	Original, Paid, Outstanding, SourceVersion int64
}

// ReceivableStatusAfterCredit is the receivable status once a credit leaves
// outstanding unpaid.
func ReceivableStatusAfterCredit(outstanding, paid int64) string {
	if outstanding == 0 {
		return "PAID"
	} else if paid > 0 {
		return "PARTIALLY_PAID"
	}
	return "OPEN"
}

// ---------------------------------------------------------------------------
// Expenses, budgets and claims
// ---------------------------------------------------------------------------

// ExpenseState is the stored state of an expense.
type ExpenseState struct {
	Status, Branch, Submitter, Currency string
	Version, Amount                     int64
}

// NextExpenseStatus validates submit, approve and reject.
func NextExpenseStatus(status, command, submitter, actorID, reason string) (string, error) {
	switch command {
	case "submit":
		if status != "DRAFT" {
			return "", apperrors.Conflict("Only a draft expense may be submitted.", nil)
		}
		return "SUBMITTED", nil
	case "approve":
		if status != "SUBMITTED" && status != "PENDING_APPROVAL" {
			return "", apperrors.Conflict("Expense cannot be approved from its current state.", nil)
		}
		if submitter == actorID {
			return "", apperrors.Forbidden("Expense submitter cannot approve their own expense.", nil)
		}
		return "APPROVED", nil
	case "reject":
		if status != "SUBMITTED" && status != "PENDING_APPROVAL" {
			return "", apperrors.Conflict("Expense cannot be rejected from its current state.", nil)
		}
		if strings.TrimSpace(reason) == "" {
			return "", apperrors.Validation("Rejection reason is required.", nil)
		}
		return "REJECTED", nil
	}
	return "", apperrors.Validation("Unsupported expense command.", nil)
}

// NextBudgetStatus validates approve and close.
func NextBudgetStatus(status, command string) (string, error) {
	if command == "approve" && (status == "DRAFT" || status == "PENDING_APPROVAL") {
		return "APPROVED", nil
	} else if command == "close" && (status == "APPROVED" || status == "ACTIVE") {
		return "CLOSED", nil
	}
	return "", apperrors.Conflict("Budget command is invalid for its current state.", nil)
}

var claimTypes = map[string]bool{"INSURANCE": true, "CARGO": true, "VEHICLE": true, "PROPERTY": true, "PERSONAL_INJURY": true, "SECURITY": true, "OTHER": true}

// ValidClaimType reports whether t (upper-case) is a supported claim type.
func ValidClaimType(t string) bool { return claimTypes[t] }

// ClaimFinal reports whether a claim status is immutable.
func ClaimFinal(status string) bool {
	return status == "SETTLED" || status == "CLOSED" || status == "CANCELLED"
}

// ---------------------------------------------------------------------------
// Compensation and payroll
// ---------------------------------------------------------------------------

// RuleCalculation is the deterministic configuration of a compensation rule.
type RuleCalculation struct {
	AmountMinor   int64  `json:"amountMinor"`
	ApprovedBasis string `json:"approvedBasis"`
}

var ruleTypes = map[string]bool{"SALARY": true, "PER_JOB": true, "PER_SHIFT": true, "COMMISSION": true, "ALLOWANCE": true, "OVERTIME": true, "BONUS": true, "MANUAL_ADJUSTMENT": true, "DEDUCTION": true}

// ValidRuleType reports whether t (upper-case) is a supported rule type.
func ValidRuleType(t string) bool { return ruleTypes[t] }

// ParseRuleCalculation decodes a rule configuration and requires a non-zero
// amount.
func ParseRuleCalculation(cfg json.RawMessage) (RuleCalculation, error) {
	var calc RuleCalculation
	if err := json.Unmarshal(cfg, &calc); err != nil || calc.AmountMinor == 0 {
		return calc, apperrors.Validation("A deterministic non-zero amountMinor is required.", nil)
	}
	return calc, nil
}

// ActiveRule is an approved compensation rule in force for a period.
type ActiveRule struct {
	ID, Type string
	Version  int64
	Config   RuleCalculation
}

// JobLevel reports whether the rule pays per completed job.
func (r ActiveRule) JobLevel() bool {
	return r.Type == "PER_JOB" || r.Type == "COMMISSION" || r.Type == "BONUS"
}

// Skipped reports whether payroll calculation ignores the rule.
func (r ActiveRule) Skipped() bool {
	return r.Type == "DEDUCTION" || r.Type == "MANUAL_ADJUSTMENT"
}

// SourceAssignment is a completed assignment payroll may pay for.
type SourceAssignment struct {
	Member, Source string
	Version        int64
}

// PayrollState is the stored state of a payroll period. End is the day after
// the period's last date.
type PayrollState struct {
	Status, Currency, Branch, Creator string
	Version, CalculationVersion       int64
	Start, End                        time.Time
}

// PayrollFrozen reports whether calculation is no longer allowed.
func PayrollFrozen(status string) bool {
	return status == "APPROVED" || status == "POSTING" || status == "POSTED" || status == "CLOSED"
}

// PayrollItem is a compensation item with the provider's sub-wallet.
type PayrollItem struct {
	ID, MembershipID, ProviderID, SubwalletID, Reason string
	Amount                                            int64
}

// PayrollAllocationType maps a compensation reason onto Wallet's allocation
// type.
func PayrollAllocationType(reason string) string {
	switch strings.ToUpper(reason) {
	case "SALARY":
		return "SALARY"
	case "PER_JOB", "COMMISSION":
		return "PER_JOB"
	case "ALLOWANCE":
		return "ALLOWANCE"
	case "BONUS":
		return "BONUS"
	default:
		return "WAGE"
	}
}

// PostingConfirmed reports whether an authoritative posting status confirms
// the posting.
func PostingConfirmed(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CONFIRMED", "POSTED", "SUCCEEDED", "COMPLETED":
		return true
	default:
		return false
	}
}

// DuePosting is a posting operation awaiting reconciliation.
type DuePosting struct {
	ID, RecordType, RecordID, OperationType, Key string
}

// CompensationItem is one calculated item for a payroll period.
type CompensationItem struct {
	OrganizationID, PeriodID, MembershipID, SourceType, SourceReference, RuleID string
	RuleVersion, CalculationVersion, AmountMinor                                int64
	Currency, ReasonCode, Explanation                                           string
	SourceInputs                                                                []byte
}

// PostingAttemptRecord is one attempt against a posting operation.
type PostingAttemptRecord struct {
	OrganizationID, OperationID, Outcome, Status, Reference, ErrorCode string
	SourceVersion                                                      int64
	Started                                                            time.Time
}
