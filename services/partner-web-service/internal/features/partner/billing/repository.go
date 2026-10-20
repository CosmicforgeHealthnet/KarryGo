package billing

import (
	"context"
	"time"
)

// InvoiceRepository persists partner_invoices rows for dashboard reads and
// monthly invoice generation.
type InvoiceRepository interface {
	Create(ctx context.Context, inv *Invoice) error
	ListByPartner(ctx context.Context, partnerID string, limit, offset int) ([]Invoice, int, error)
	// SumPaidByPartner returns the lifetime total of paid invoices - "Total Billed".
	SumPaidByPartner(ctx context.Context, partnerID string) (int64, error)
}

// PaymentMethodRepository persists partner_payment_methods rows.
type PaymentMethodRepository interface {
	// Create inserts a payment method. When pm.IsDefault is true, any existing
	// default for the same partner is unset first.
	Create(ctx context.Context, pm *PaymentMethod) (*PaymentMethod, error)
	ListByPartner(ctx context.Context, partnerID string) ([]PaymentMethod, error)
	SetDefault(ctx context.Context, partnerID, id string) error
}

// DeliveryUsageReader counts completed deliveries in a time range, and
// assembles the Dashboard home screen stat cards.
type DeliveryUsageReader interface {
	CountCompletedInRange(ctx context.Context, partnerID string, from, to time.Time) (int, error)
	// GetDashboardSummary returns all-time delivery count, today's count, and
	// the completed/total success rate for a partner, in one round trip.
	GetDashboardSummary(ctx context.Context, partnerID string) (DashboardSummary, error)
}

// PartnerLister resolves which partners to bill during monthly invoice runs.
type PartnerLister interface {
	ListActivePartnerIDs(ctx context.Context) ([]string, error)
}

// ChargeRepository is the persistence surface for charging invoices. All
// partner-facing reads must filter by partner_id.
type ChargeRepository interface {
	FindDueUnpaidInvoices(ctx context.Context, now time.Time, limit int) ([]Invoice, error)
	FindInvoiceByID(ctx context.Context, invoiceID string) (*Invoice, error)
	FindInvoiceByPartnerAndID(ctx context.Context, partnerID, invoiceID string) (*Invoice, error)
	ListInvoicesByPartner(ctx context.Context, partnerID string) ([]Invoice, error)
	MarkInvoiceCharging(ctx context.Context, invoiceID, walletReference string) error
	MarkInvoicePaid(ctx context.Context, invoiceID, walletReference string, paidAt time.Time) error
	MarkInvoiceChargeFailed(ctx context.Context, invoiceID, walletReference, failureReason string) error

	FindActiveAuthorization(ctx context.Context, partnerID string) (*Authorization, error)

	HasPendingChargeAttempt(ctx context.Context, invoiceID string) (bool, error)
	CreateChargeAttempt(ctx context.Context, attempt *ChargeAttempt) error
	SetChargeAttemptReference(ctx context.Context, invoiceID, walletReference string) error
	ResolveLatestChargeAttempt(ctx context.Context, invoiceID, status, walletReference, failureReason string) error
}

// ChargeAuthorizationInput is the payment-wallet internal charge request.
type ChargeAuthorizationInput struct {
	SourceService     string `json:"source_service"`
	SourceReference   string `json:"source_reference"` // invoice id
	Email             string `json:"email"`
	AmountKobo        int64  `json:"amount_kobo"`
	AuthorizationCode string `json:"authorization_code"`
	IdempotencyKey    string `json:"idempotency_key"`
}

// ChargeAuthorizationResult is the immediate payment-wallet response; the final
// outcome arrives asynchronously on the charge-result callback.
type ChargeAuthorizationResult struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
}

// ChargeClient calls payment-wallet-service's internal charge-authorization
// endpoint using HMAC service auth. api-gateway never talks to Paystack.
type ChargeClient interface {
	ChargeAuthorization(ctx context.Context, in ChargeAuthorizationInput) (ChargeAuthorizationResult, error)
}
