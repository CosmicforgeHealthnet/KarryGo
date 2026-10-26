// Package financereport is the Wallet -> Bridge -> HQ group finance reporting
// contract (Section 26). It is financial REPORTING, never financial control:
// figures are aggregate integer minor units per currency, each with an exact
// source field and calculation, and none is labelled recognized revenue unless
// an approved recognition policy says so (none is supplied in this build).
package financereport

import (
	"errors"
	"regexp"
	"time"
)

const (
	Contract = "karryam.finance-report.v1"
	Owner    = "payment-wallet-service"
	Path     = "/internal/payment-wallet/v1/admin/finance-reports"
)

// Recognition states. GMV, escrow and pending balances are never revenue.
const (
	NotRevenue           = "NOT_REVENUE"
	SourceFigureOnly     = "SOURCE_FIGURE_RECOGNITION_POLICY_PENDING"
	RecognizedByPolicy   = "RECOGNIZED_BY_APPROVED_POLICY"
	CompletenessComplete = "COMPLETE"
	CompletenessPartial  = "PARTIAL_COVERAGE"
)

// Definition documents one measure. Every report carries all definitions so a
// consumer can never confuse GMV with recognized revenue.
type Definition struct {
	Measure     string `json:"measure"`
	SourceField string `json:"source_field"`
	Calculation string `json:"calculation"`
	Recognition string `json:"recognition"`
	Correction  string `json:"correction"`
}

// Measures is the closed list (Section 26). Calculations reference Wallet's
// own tables; the owner implementation must match these exactly.
func Measures() []Definition {
	corr := "Recomputed from source rows on every request for the requested window; late settlement or reversal changes appear as a different value with a newer data_as_of; never double counted because each row is counted once by its own state timestamp."
	return []Definition{
		{"gross_transaction_value",
			"payment_intents.amount_kobo (commerce_payment_intents.final_authorized_minor for commerce orders), excluding source_service payment-wallet-service top-ups",
			"SUM and COUNT per payment_intents.currency over intents whose FIRST funding ledger_transactions row (transaction_type wallet_payment_hold, paystack_charge_success or commerce_payment_hold, matched on source_service+source_reference) has created_at in [period_start, min(period_end, data_as_of))",
			NotRevenue, corr},
		{"net_platform_fee",
			"ledger_entries on wallet_accounts.account_type platform_revenue (job_settlement fee, managed_work_settlement platform share, commerce platform revenue, and their reversals)",
			"SUM(credit amount_kobo) - SUM(debit amount_kobo) per ledger_entries.currency over entries whose ledger_transactions.created_at is in the window; COUNT of distinct ledger transactions. A reversal or refund recovery reduces it once, in the window it was posted; legacy refunds only release held escrow, which never carried a posted fee",
			SourceFigureOnly, corr},
		{"provider_earned",
			"credit ledger_entries on wallet_accounts.account_type provider_payable, excluding transaction_type withdrawal_*",
			"SUM(amount_kobo) and COUNT of those credit entries per currency whose ledger_transactions.created_at is in the window; gross of later reversal debits, which are not netted",
			NotRevenue, corr},
		{"successful_settlements",
			"payment_intents released from escrow by a ledger_transactions row of transaction_type job_settlement or managed_work_settlement",
			"COUNT and SUM(payment_intents.amount_kobo) per currency of intents whose settlement posting created_at is in the window; commerce allocation releases (commerce_financial_operations VENDOR_RELEASE/DELIVERY_RELEASE) are not intent settlements and mark the figure PARTIAL_COVERAGE when present",
			NotRevenue, corr},
		{"pending_payouts",
			"withdrawals (status pending, processing, requires_otp) and finance_provider_withdrawals (status REQUESTED, PENDING_APPROVAL, APPROVED, PROCESSING, FAILED_RETRYABLE, DEAD_LETTER)",
			"SUM(amount) and COUNT per currency of those rows created at or before data_as_of, as visible at data_as_of",
			NotRevenue, corr},
		{"refunds",
			"refunds (status processed), finance_refunds (status COMPLETED), commerce_financial_operations (PARTIAL_REFUND, APPLIED)",
			"SUM(amount) and COUNT per currency of refunds completed in the window: legacy refunds at the earliest refund_processed (source_reference = refund reference) or wallet_refund_processed (intent source pair) ledger_transactions.created_at, finance_refunds at completed_at, commerce partial refunds at created_at",
			NotRevenue, corr},
		{"pending_liabilities",
			"ledger balance of wallet_accounts.account_type job_escrow and commerce_escrow",
			"SUM(credit - debit amount_kobo) per currency over escrow accounts, using ledger entries posted at or before data_as_of; COUNT of escrow accounts with a non-zero balance. A negative total is reported as unknown (ledger inconsistency), never clamped",
			NotRevenue, corr},
		{"reconciled_transactions",
			"paystack_webhook_events matched to exactly one Wallet row (payment_intents.paystack_reference, else withdrawals.reference/paystack_transfer_code, else refunds.paystack_refund_reference)",
			"COUNT and SUM(matched row amount) per matched row currency of events with processing_error NULL and processed_at in the window",
			NotRevenue, corr},
		{"unreconciled_transactions",
			"matched paystack_webhook_events not processed or processed with an error, OPEN managed_settlement_reconciliations; currency-less items: unmatched paystack_webhook_events and unresolved finance_reconciliation_findings",
			"COUNT and SUM(amount) per currency at data_as_of; null with a reason (PARTIAL_COVERAGE) when no COMPLETED finance_reconciliation_runs row exists at data_as_of or any currency-less open item exists, because those cannot be attributed to a currency",
			NotRevenue, corr},
	}
}

type Figure struct {
	Measure       string `json:"measure"`
	Currency      string `json:"currency"`
	AmountMinor   *int64 `json:"amount_minor"` // nil = unknown, never zero-by-default
	Count         *int64 `json:"count"`
	Completeness  string `json:"completeness"`
	UnknownReason string `json:"unknown_reason,omitempty"`
}

type Report struct {
	Contract           string       `json:"contract_version"`
	Owner              string       `json:"owner"`
	PeriodStart        time.Time    `json:"period_start"`
	PeriodEnd          time.Time    `json:"period_end"`
	Timezone           string       `json:"timezone"`
	DataAsOf           time.Time    `json:"data_as_of"`
	SourceWatermark    time.Time    `json:"source_watermark"`
	AggregationVersion string       `json:"aggregation_version"`
	Unit               string       `json:"unit"`
	Definitions        []Definition `json:"definitions"`
	Figures            []Figure     `json:"figures"`
	Completeness       string       `json:"completeness"`
	ErrorReason        string       `json:"error_reason,omitempty"`
}

var currency = regexp.MustCompile(`^[A-Z]{3}$`)
var ErrInvalid = errors.New("invalid finance report")

// Validate enforces honesty rules: known measures only, integer minor units,
// unknown is null with a reason, partial reports are labelled partial.
func (r Report) Validate() error {
	if r.Contract != Contract || r.Owner != Owner || !r.PeriodEnd.After(r.PeriodStart) || r.DataAsOf.IsZero() || r.SourceWatermark.IsZero() || r.Timezone == "" || r.Unit != "MINOR_UNITS" || r.AggregationVersion == "" || len(r.Definitions) != len(Measures()) {
		return ErrInvalid
	}
	known := map[string]bool{}
	for _, d := range Measures() {
		known[d.Measure] = true
	}
	partial := false
	for _, f := range r.Figures {
		if !known[f.Measure] || !currency.MatchString(f.Currency) || (f.AmountMinor == nil && f.Count == nil && f.UnknownReason == "") || (f.Completeness != CompletenessComplete && f.Completeness != CompletenessPartial) {
			return ErrInvalid
		}
		if f.AmountMinor != nil && *f.AmountMinor < 0 && f.Measure != "net_platform_fee" {
			return ErrInvalid
		}
		partial = partial || f.Completeness == CompletenessPartial
	}
	if partial && r.Completeness != CompletenessPartial {
		return ErrInvalid
	}
	return nil
}
