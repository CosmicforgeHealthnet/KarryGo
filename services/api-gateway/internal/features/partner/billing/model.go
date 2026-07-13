// Package billing implements the partner Billing & Usage dashboard screen:
// running usage totals (computed live from partner_bookings, not a stored
// counter), monthly invoice generation, and saved payment methods.
//
// This package intentionally does NOT perform Paystack card capture or
// charging. Payment method attachment expects the frontend to have already
// tokenized the card via Paystack Inline/Popup and sent only the resulting
// authorization_code - this backend never receives or stores a raw card
// number. Actual recurring charge collection against a saved
// authorization_code is not implemented in this pass; invoices are generated
// as 'pending' and left for a future pass that reuses payment-wallet-service's
// existing Paystack integration rather than duplicating it here.
package billing

import "time"

type InvoiceStatus string

const (
	InvoiceStatusPending InvoiceStatus = "pending"
	InvoiceStatusPaid    InvoiceStatus = "paid"
	InvoiceStatusFailed  InvoiceStatus = "failed"
)

// Invoice mirrors a partner_invoices row - one row per partner per billing
// period (currently: one calendar month).
type Invoice struct {
	ID                  string
	PartnerID           string
	InvoiceNumber       string
	PeriodStart         time.Time
	PeriodEnd           time.Time
	DeliveryCount       int
	RatePerDeliveryKobo int64
	AmountKobo          int64
	Currency            string
	Status              InvoiceStatus
	PaidAt              *time.Time
	CreatedAt           time.Time
}

// PaymentMethod mirrors a partner_payment_methods row. AuthorizationCode is a
// Paystack-issued token, never a raw card number - see package doc comment.
type PaymentMethod struct {
	ID                string
	PartnerID         string
	Provider          string // "paystack"
	AuthorizationCode string
	CardBrand         string
	Last4             string
	ExpMonth          string
	ExpYear           string
	IsDefault         bool
	CreatedAt         time.Time
}

// --- response DTOs -----------------------------------------------------------

type InvoiceView struct {
	ID            string `json:"id"`
	InvoiceNumber string `json:"invoice_number"`
	Period        string `json:"period"`
	AmountKobo    int64  `json:"amount_kobo"`
	AmountDisplay string `json:"amount_display"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
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
// stored - "Deliveries this month" can never drift out of sync with reality.
type UsageSummary struct {
	DeliveriesThisMonth    int    `json:"deliveries_this_month"`
	BillableDeliveries     int    `json:"billable_deliveries"`
	RatePerDeliveryKobo    int64  `json:"rate_per_delivery_kobo"`
	RatePerDeliveryDisplay string `json:"rate_per_delivery_display"`
	TotalCostKobo          int64  `json:"total_cost_kobo"`
	TotalCostDisplay       string `json:"total_cost_display"`
}

// Overview is the GET /billing/overview response data - the whole
// Billing & Usage screen in one call.
type Overview struct {
	CurrentBalanceKobo   int64               `json:"current_balance_kobo"`
	ThisMonthChargesKobo int64               `json:"this_month_charges_kobo"`
	TotalBilledKobo      int64               `json:"total_billed_kobo"`
	CurrentPlan          string              `json:"current_plan"`
	BillingModel         string              `json:"billing_model"`
	Status                string              `json:"status"`
	Usage                UsageSummary        `json:"usage"`
	PaymentMethods       []PaymentMethodView `json:"payment_methods"`
}