package billing

import (
	"context"
	"time"
)

// InvoiceRepository persists partner_invoices rows.
type InvoiceRepository interface {
	Create(ctx context.Context, inv *Invoice) error
	ListByPartner(ctx context.Context, partnerID string, limit, offset int) ([]Invoice, int, error)
	// SumPaidByPartner returns the lifetime total of paid invoices - "Total Billed".
	SumPaidByPartner(ctx context.Context, partnerID string) (int64, error)
}

// PaymentMethodRepository persists partner_payment_methods rows.
type PaymentMethodRepository interface {
	// Create inserts a payment method. When pm.IsDefault is true, any existing
	// default for the same partner is unset first (single default per partner).
	Create(ctx context.Context, pm *PaymentMethod) (*PaymentMethod, error)
	ListByPartner(ctx context.Context, partnerID string) ([]PaymentMethod, error)
	SetDefault(ctx context.Context, partnerID, id string) error
}

// DeliveryUsageReader counts completed deliveries in a time range. It is
// intentionally a narrow interface (not delivery.BookingRepository) since
// billing only ever needs a count, never full booking rows.
type DeliveryUsageReader interface {
	// CountCompletedInRange counts a partner's completed deliveries whose
	// created_at falls within [from, to).
	CountCompletedInRange(ctx context.Context, partnerID string, from, to time.Time) (int, error)
}

// PartnerLister resolves which partners to bill during the monthly invoice
// generation pass. Implemented over partner_accounts (active partners only).
type PartnerLister interface {
	ListActivePartnerIDs(ctx context.Context) ([]string, error)
}