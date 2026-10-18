// Package billing implements the partner Billing & Usage dashboard, monthly
// invoice generation, and invoice charging through payment-wallet-service.
// api-gateway never holds Paystack secret keys: charge execution goes through
// payment-wallet's internal charge-authorization endpoint using HMAC service
// auth, and results arrive through POST /api/v1/internal/callbacks/charge-result.
package billing

import "time"

type InvoiceStatus string

const (
	InvoiceStatusPending      InvoiceStatus = "pending"
	InvoiceStatusCharging     InvoiceStatus = "charging"
	InvoiceStatusPaid         InvoiceStatus = "paid"
	InvoiceStatusFailed       InvoiceStatus = "failed"
	InvoiceStatusChargeFailed InvoiceStatus = "charge_failed"
)

// Charge attempt statuses.
const (
	AttemptStatusPending = "pending"
	AttemptStatusSuccess = "success"
	AttemptStatusFailed  = "failed"
)

// Charge result statuses accepted from payment-wallet-service.
const (
	ChargeResultSuccess = "success"
	ChargeResultFailed  = "failed"
)

// Invoice mirrors a partner_invoices row. The monthly invoice fields come from
// the Billing & Usage feature; the charge fields are additive columns used by
// the payment-wallet charge loop.
type Invoice struct {
	ID                  string
	PartnerID           string
	PartnerEmail        *string
	InvoiceNumber       string
	PeriodStart         time.Time
	PeriodEnd           time.Time
	DeliveryCount       int
	RatePerDeliveryKobo int64
	AmountKobo          int64
	Currency            string
	Status              InvoiceStatus
	DueAt               *time.Time
	PaidAt              *time.Time
	WalletReference     *string
	FailureReason       *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// PaymentMethod mirrors a partner_payment_methods row. AuthorizationCode is a
// Paystack-issued token, never a raw card number.
type PaymentMethod struct {
	ID                string
	PartnerID         string
	Provider          string
	AuthorizationCode string
	CardBrand         string
	Last4             string
	ExpMonth          string
	ExpYear           string
	IsDefault         bool
	CreatedAt         time.Time
}

// Authorization mirrors a partner_billing_authorizations row. AuthorizationCode
// must never appear in any API response.
type Authorization struct {
	ID                string
	PartnerID         string
	Email             string
	AuthorizationCode string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ChargeAttempt mirrors a partner_invoice_charge_attempts row.
type ChargeAttempt struct {
	ID              string
	InvoiceID       string
	PartnerID       string
	IdempotencyKey  string
	WalletReference *string
	Status          string
	AmountKobo      int64
	FailureReason   *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// --- response DTOs -----------------------------------------------------------

// InvoiceView is the partner-facing invoice shape. It never includes card
// authorization data.
type InvoiceView struct {
	ID              string  `json:"id"`
	InvoiceNumber   string  `json:"invoice_number,omitempty"`
	Period          string  `json:"period,omitempty"`
	AmountKobo      int64   `json:"amount_kobo"`
	AmountDisplay   string  `json:"amount_display,omitempty"`
	Currency        string  `json:"currency"`
	Status          string  `json:"status"`
	DueAt           *string `json:"due_at,omitempty"`
	PaidAt          *string `json:"paid_at"`
	WalletReference *string `json:"wallet_reference,omitempty"`
	FailureReason   *string `json:"failure_reason,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type PaymentMethodView struct {
	ID        string `json:"id"`
	CardBrand string `json:"card_brand"`
	Last4     string `json:"last4"`
	ExpMonth  string `json:"exp_month"`
	ExpYear   string `json:"exp_year"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
}

// UsageSummary is computed live from partner_bookings on every request, not
// stored, so "Deliveries this month" cannot drift from reality.
type UsageSummary struct {
	DeliveriesThisMonth    int    `json:"deliveries_this_month"`
	BillableDeliveries     int    `json:"billable_deliveries"`
	RatePerDeliveryKobo    int64  `json:"rate_per_delivery_kobo"`
	RatePerDeliveryDisplay string `json:"rate_per_delivery_display"`
	TotalCostKobo          int64  `json:"total_cost_kobo"`
	TotalCostDisplay       string `json:"total_cost_display"`
}

// Overview is the GET /billing/overview response data.
type Overview struct {
	CurrentBalanceKobo   int64               `json:"current_balance_kobo"`
	ThisMonthChargesKobo int64               `json:"this_month_charges_kobo"`
	TotalBilledKobo      int64               `json:"total_billed_kobo"`
	CurrentPlan          string              `json:"current_plan"`
	BillingModel         string              `json:"billing_model"`
	Status               string              `json:"status"`
	Usage                UsageSummary        `json:"usage"`
	PaymentMethods       []PaymentMethodView `json:"payment_methods"`
}

// ChargeResultInput is the POST /api/v1/internal/callbacks/charge-result body
// sent by payment-wallet-service.
type ChargeResultInput struct {
	SourceReference string `json:"source_reference"` // api-gateway invoice id
	Status          string `json:"status"`           // success | failed
	Reference       string `json:"reference"`        // Paystack/payment reference
	AmountKobo      int64  `json:"amount_kobo"`
}

// ChargeResultOutcome reports what HandleChargeResult did.
type ChargeResultOutcome struct {
	Skipped   bool   `json:"skipped,omitempty"`
	InvoiceID string `json:"invoice_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

type DashboardSummary struct {
	TotalDeliveries    int     `json:"total_deliveries"`
	DeliveriesToday    int     `json:"deliveries_today"`
	SuccessRate        float64 `json:"success_rate"`
	SuccessRateDisplay string  `json:"success_rate_display"`
}
