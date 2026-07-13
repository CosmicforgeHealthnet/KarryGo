package infra

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
)

// InvoiceRepo is the Postgres implementation of billing.InvoiceRepository.
type InvoiceRepo struct {
	pool *pgxpool.Pool
}

func NewInvoiceRepo(pool *pgxpool.Pool) *InvoiceRepo { return &InvoiceRepo{pool: pool} }

const invoiceColumns = `id, partner_id, invoice_number, period_start, period_end, delivery_count,
	rate_per_delivery_kobo, amount_kobo, currency, status, paid_at, created_at`

func (r *InvoiceRepo) Create(ctx context.Context, inv *billing.Invoice) error {
	const q = `INSERT INTO partner_invoices
		(partner_id, invoice_number, period_start, period_end, delivery_count,
		 rate_per_delivery_kobo, amount_kobo, currency, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`
	return r.pool.QueryRow(ctx, q, inv.PartnerID, inv.InvoiceNumber, inv.PeriodStart, inv.PeriodEnd,
		inv.DeliveryCount, inv.RatePerDeliveryKobo, inv.AmountKobo, inv.Currency, inv.Status, inv.CreatedAt,
	).Scan(&inv.ID)
}

func (r *InvoiceRepo) ListByPartner(ctx context.Context, partnerID string, limit, offset int) ([]billing.Invoice, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM partner_invoices WHERE partner_id = $1`, partnerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + invoiceColumns + ` FROM partner_invoices WHERE partner_id = $1
		ORDER BY period_start DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, partnerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []billing.Invoice
	for rows.Next() {
		var inv billing.Invoice
		if err := rows.Scan(&inv.ID, &inv.PartnerID, &inv.InvoiceNumber, &inv.PeriodStart, &inv.PeriodEnd,
			&inv.DeliveryCount, &inv.RatePerDeliveryKobo, &inv.AmountKobo, &inv.Currency, &inv.Status,
			&inv.PaidAt, &inv.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, inv)
	}
	return out, total, rows.Err()
}

func (r *InvoiceRepo) SumPaidByPartner(ctx context.Context, partnerID string) (int64, error) {
	var total *int64
	const q = `SELECT sum(amount_kobo) FROM partner_invoices WHERE partner_id = $1 AND status = 'paid'`
	if err := r.pool.QueryRow(ctx, q, partnerID).Scan(&total); err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// --- payment methods ---------------------------------------------------------

// PaymentMethodRepo is the Postgres implementation of
// billing.PaymentMethodRepository.
type PaymentMethodRepo struct {
	pool *pgxpool.Pool
}

func NewPaymentMethodRepo(pool *pgxpool.Pool) *PaymentMethodRepo {
	return &PaymentMethodRepo{pool: pool}
}

const paymentMethodColumns = `id, partner_id, provider, authorization_code, card_brand, last4,
	exp_month, exp_year, is_default, created_at`

func (r *PaymentMethodRepo) Create(ctx context.Context, pm *billing.PaymentMethod) (*billing.PaymentMethod, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if pm.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE partner_payment_methods SET is_default = false WHERE partner_id = $1`, pm.PartnerID); err != nil {
			return nil, err
		}
	}

	const q = `INSERT INTO partner_payment_methods
		(partner_id, provider, authorization_code, card_brand, last4, exp_month, exp_year, is_default, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now()) RETURNING id, created_at`
	err = tx.QueryRow(ctx, q, pm.PartnerID, pm.Provider, pm.AuthorizationCode, pm.CardBrand,
		pm.Last4, pm.ExpMonth, pm.ExpYear, pm.IsDefault).Scan(&pm.ID, &pm.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return pm, nil
}

func (r *PaymentMethodRepo) ListByPartner(ctx context.Context, partnerID string) ([]billing.PaymentMethod, error) {
	q := `SELECT ` + paymentMethodColumns + ` FROM partner_payment_methods
		WHERE partner_id = $1 ORDER BY is_default DESC, created_at DESC`
	rows, err := r.pool.Query(ctx, q, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []billing.PaymentMethod
	for rows.Next() {
		var pm billing.PaymentMethod
		if err := rows.Scan(&pm.ID, &pm.PartnerID, &pm.Provider, &pm.AuthorizationCode, &pm.CardBrand,
			&pm.Last4, &pm.ExpMonth, &pm.ExpYear, &pm.IsDefault, &pm.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, pm)
	}
	return out, rows.Err()
}

func (r *PaymentMethodRepo) SetDefault(ctx context.Context, partnerID, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE partner_payment_methods SET is_default = false WHERE partner_id = $1`, partnerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE partner_payment_methods SET is_default = true WHERE id = $1 AND partner_id = $2`, id, partnerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- usage reader (billing.DeliveryUsageReader) -------------------------------

// BillingUsageReader counts completed deliveries directly against
// partner_bookings for the Billing & Usage screen. It intentionally does not
// depend on delivery.BookingRepository - it only ever needs a count, not the
// full booking repository interface.
type BillingUsageReader struct {
	pool *pgxpool.Pool
}

func NewBillingUsageReader(pool *pgxpool.Pool) *BillingUsageReader {
	return &BillingUsageReader{pool: pool}
}

func (r *BillingUsageReader) CountCompletedInRange(ctx context.Context, partnerID string, from, to time.Time) (int, error) {
	const q = `SELECT count(*) FROM partner_bookings
		WHERE partner_id = $1 AND status = 'completed' AND created_at >= $2 AND created_at < $3`
	var n int
	err := r.pool.QueryRow(ctx, q, partnerID, from, to).Scan(&n)
	return n, err
}

// --- partner lister (billing.PartnerLister) -----------------------------------

// BillingPartnerLister lists active partner ids for the monthly invoice
// generation pass.
type BillingPartnerLister struct {
	pool *pgxpool.Pool
}

func NewBillingPartnerLister(pool *pgxpool.Pool) *BillingPartnerLister {
	return &BillingPartnerLister{pool: pool}
}

func (r *BillingPartnerLister) ListActivePartnerIDs(ctx context.Context) ([]string, error) {
	const q = `SELECT id FROM partner_accounts WHERE status = 'active'`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}