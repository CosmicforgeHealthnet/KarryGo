package infra

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/billing"
)

// InvoiceRepo is the Postgres implementation of billing.InvoiceRepository.
type InvoiceRepo struct {
	pool *pgxpool.Pool
}

func NewInvoiceRepo(pool *pgxpool.Pool) *InvoiceRepo { return &InvoiceRepo{pool: pool} }

const monthlyInvoiceColumns = `id, partner_id, partner_email, invoice_number, period_start, period_end, delivery_count,
	rate_per_delivery_kobo, amount_kobo, currency, status, due_at, paid_at, wallet_reference, failure_reason, created_at, updated_at`

func (r *InvoiceRepo) Create(ctx context.Context, inv *billing.Invoice) error {
	const q = `INSERT INTO partner_invoices
		(partner_id, partner_email, invoice_number, period_start, period_end, delivery_count,
		 rate_per_delivery_kobo, amount_kobo, currency, status, due_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`
	return r.pool.QueryRow(ctx, q, inv.PartnerID, inv.PartnerEmail, inv.InvoiceNumber, inv.PeriodStart, inv.PeriodEnd,
		inv.DeliveryCount, inv.RatePerDeliveryKobo, inv.AmountKobo, inv.Currency, inv.Status, inv.DueAt, inv.CreatedAt, inv.UpdatedAt,
	).Scan(&inv.ID)
}

func (r *InvoiceRepo) ListByPartner(ctx context.Context, partnerID string, limit, offset int) ([]billing.Invoice, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM partner_invoices WHERE partner_id = $1`, partnerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + monthlyInvoiceColumns + ` FROM partner_invoices WHERE partner_id = $1
		ORDER BY period_start DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, partnerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []billing.Invoice
	for rows.Next() {
		inv, err := scanBillingInvoice(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *inv)
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

// PaymentMethodRepo is the Postgres implementation of billing.PaymentMethodRepository.
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

// --- usage reader ------------------------------------------------------------

// BillingUsageReader counts completed deliveries directly against partner_bookings.
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

// GetDashboardSummary returns all-time delivery count, today's count (using
// the database's own clock/day boundary, not app-server local time), and the
// completed/total success rate for the Dashboard home screen, in one query.
func (r *BillingUsageReader) GetDashboardSummary(ctx context.Context, partnerID string) (billing.DashboardSummary, error) {
	const q = `SELECT
		count(*) AS total,
		count(*) FILTER (
			WHERE created_at >= date_trunc('day', now())
			  AND created_at < date_trunc('day', now()) + interval '1 day'
		) AS today,
		count(*) FILTER (WHERE status = 'completed') AS completed
		FROM partner_bookings
		WHERE partner_id = $1`

	var total, today, completed int
	if err := r.pool.QueryRow(ctx, q, partnerID).Scan(&total, &today, &completed); err != nil {
		return billing.DashboardSummary{}, err
	}

	var successRate float64
	if total > 0 {
		successRate = float64(completed) / float64(total) * 100
	}

	return billing.DashboardSummary{
		TotalDeliveries:    total,
		DeliveriesToday:    today,
		SuccessRate:        successRate,
		SuccessRateDisplay: fmt.Sprintf("%.1f%%", successRate),
	}, nil
}

// --- partner lister ----------------------------------------------------------

// BillingPartnerLister lists active partner ids for monthly invoice generation.
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

// --- charge repository -------------------------------------------------------

// BillingRepo is the Postgres implementation of billing.ChargeRepository.
type BillingRepo struct {
	pool *pgxpool.Pool
}

func NewBillingRepo(pool *pgxpool.Pool) *BillingRepo { return &BillingRepo{pool: pool} }

const chargeInvoiceColumns = monthlyInvoiceColumns

func (r *BillingRepo) FindDueUnpaidInvoices(ctx context.Context, now time.Time, limit int) ([]billing.Invoice, error) {
	q := `SELECT ` + chargeInvoiceColumns + ` FROM partner_invoices
		WHERE status IN ('pending', 'failed', 'charge_failed') AND due_at IS NOT NULL AND due_at <= $1
		ORDER BY due_at ASC LIMIT $2`
	rows, err := r.pool.Query(ctx, q, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []billing.Invoice
	for rows.Next() {
		inv, err := scanBillingInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (r *BillingRepo) FindInvoiceByID(ctx context.Context, invoiceID string) (*billing.Invoice, error) {
	if !isUUID(invoiceID) {
		return nil, nil
	}
	q := `SELECT ` + chargeInvoiceColumns + ` FROM partner_invoices WHERE id = $1`
	inv, err := scanBillingInvoice(r.pool.QueryRow(ctx, q, invoiceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *BillingRepo) FindInvoiceByPartnerAndID(ctx context.Context, partnerID, invoiceID string) (*billing.Invoice, error) {
	if !isUUID(invoiceID) {
		return nil, nil
	}
	q := `SELECT ` + chargeInvoiceColumns + ` FROM partner_invoices WHERE partner_id = $1 AND id = $2`
	inv, err := scanBillingInvoice(r.pool.QueryRow(ctx, q, partnerID, invoiceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *BillingRepo) ListInvoicesByPartner(ctx context.Context, partnerID string) ([]billing.Invoice, error) {
	q := `SELECT ` + chargeInvoiceColumns + ` FROM partner_invoices WHERE partner_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []billing.Invoice
	for rows.Next() {
		inv, err := scanBillingInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (r *BillingRepo) MarkInvoiceCharging(ctx context.Context, invoiceID, walletReference string) error {
	const q = `UPDATE partner_invoices
		SET status = 'charging', wallet_reference = NULLIF($2, ''), failure_reason = NULL, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, invoiceID, walletReference)
	return err
}

func (r *BillingRepo) MarkInvoicePaid(ctx context.Context, invoiceID, walletReference string, paidAt time.Time) error {
	const q = `UPDATE partner_invoices
		SET status = 'paid', paid_at = COALESCE(paid_at, $3),
		    wallet_reference = COALESCE(NULLIF($2, ''), wallet_reference), failure_reason = NULL, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, invoiceID, walletReference, paidAt)
	return err
}

func (r *BillingRepo) MarkInvoiceChargeFailed(ctx context.Context, invoiceID, walletReference, failureReason string) error {
	const q = `UPDATE partner_invoices
		SET status = 'charge_failed',
		    wallet_reference = COALESCE(NULLIF($2, ''), wallet_reference),
		    failure_reason = COALESCE(NULLIF($3, ''), failure_reason), updated_at = now()
		WHERE id = $1 AND status <> 'paid'`
	_, err := r.pool.Exec(ctx, q, invoiceID, walletReference, failureReason)
	return err
}

func (r *BillingRepo) FindActiveAuthorization(ctx context.Context, partnerID string) (*billing.Authorization, error) {
	const q = `SELECT id, partner_id, email, authorization_code, status, created_at, updated_at
		FROM partner_billing_authorizations
		WHERE partner_id = $1 AND status = 'active'
		ORDER BY created_at DESC LIMIT 1`
	var a billing.Authorization
	err := r.pool.QueryRow(ctx, q, partnerID).Scan(
		&a.ID, &a.PartnerID, &a.Email, &a.AuthorizationCode, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *BillingRepo) HasPendingChargeAttempt(ctx context.Context, invoiceID string) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM partner_invoice_charge_attempts
		WHERE invoice_id = $1 AND status = 'pending')`
	var exists bool
	err := r.pool.QueryRow(ctx, q, invoiceID).Scan(&exists)
	return exists, err
}

func (r *BillingRepo) CreateChargeAttempt(ctx context.Context, a *billing.ChargeAttempt) error {
	const q = `INSERT INTO partner_invoice_charge_attempts
		(id, invoice_id, partner_id, idempotency_key, status, amount_kobo, failure_reason, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (idempotency_key) DO UPDATE SET
		    status = EXCLUDED.status,
		    amount_kobo = EXCLUDED.amount_kobo,
		    wallet_reference = NULL,
		    failure_reason = NULL,
		    updated_at = EXCLUDED.updated_at`
	_, err := r.pool.Exec(ctx, q, a.ID, a.InvoiceID, a.PartnerID, a.IdempotencyKey,
		a.Status, a.AmountKobo, a.FailureReason, a.CreatedAt, a.UpdatedAt)
	return err
}

func (r *BillingRepo) SetChargeAttemptReference(ctx context.Context, invoiceID, walletReference string) error {
	const q = `UPDATE partner_invoice_charge_attempts
		SET wallet_reference = NULLIF($2, ''), updated_at = now()
		WHERE invoice_id = $1 AND status = 'pending'`
	_, err := r.pool.Exec(ctx, q, invoiceID, walletReference)
	return err
}

func (r *BillingRepo) ResolveLatestChargeAttempt(ctx context.Context, invoiceID, status, walletReference, failureReason string) error {
	const q = `UPDATE partner_invoice_charge_attempts
		SET status = $2,
		    wallet_reference = COALESCE(NULLIF($3, ''), wallet_reference),
		    failure_reason = COALESCE(NULLIF($4, ''), failure_reason), updated_at = now()
		WHERE id = (SELECT id FROM partner_invoice_charge_attempts
		            WHERE invoice_id = $1 AND status = 'pending'
		            ORDER BY created_at DESC LIMIT 1)`
	_, err := r.pool.Exec(ctx, q, invoiceID, status, walletReference, failureReason)
	return err
}

func scanBillingInvoice(row rowScanner) (*billing.Invoice, error) {
	var inv billing.Invoice
	err := row.Scan(&inv.ID, &inv.PartnerID, &inv.PartnerEmail, &inv.InvoiceNumber, &inv.PeriodStart, &inv.PeriodEnd,
		&inv.DeliveryCount, &inv.RatePerDeliveryKobo, &inv.AmountKobo, &inv.Currency, &inv.Status, &inv.DueAt,
		&inv.PaidAt, &inv.WalletReference, &inv.FailureReason, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}