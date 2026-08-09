package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// BillingRepository owns subscription plans, subscriptions, invoices and
// invoice payments.
type BillingRepository struct {
	db database.Querier
}

// NewBillingRepository wires the repository.
func NewBillingRepository(db database.Querier) *BillingRepository {
	return &BillingRepository{db: db}
}

const planColumns = `id, code, name, description, currency, price_minor, billing_interval,
	interval_count, grace_days, status, effective_from, effective_to, version, created_at, updated_at`

func scanPlan(row pgx.Row) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := row.Scan(&plan.ID, &plan.Code, &plan.Name, &plan.Description, &plan.Currency,
		&plan.PriceMinor, &plan.BillingInterval, &plan.IntervalCount, &plan.GraceDays,
		&plan.Status, &plan.EffectiveFrom, &plan.EffectiveTo, &plan.Version,
		&plan.CreatedAt, &plan.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan plan: %w", err)
	}
	return &plan, nil
}

// CreatePlan inserts a plan. priceMinor may be nil, which means pricing has
// not been configured and the plan cannot yet be invoiced.
func (r *BillingRepository) CreatePlan(ctx context.Context, q database.Querier,
	plan *models.SubscriptionPlan) error {
	if q == nil {
		q = r.db
	}
	if plan.ID == "" {
		plan.ID = ids.New(ids.PrefixPlan)
	}
	if plan.IntervalCount <= 0 {
		plan.IntervalCount = 1
	}
	if plan.Status == "" {
		plan.Status = models.PlanDraft
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_subscription_plans
			(id, code, name, description, currency, price_minor, billing_interval,
			 interval_count, grace_days, status, effective_from, effective_to,
			 version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, COALESCE($11, now()), $12, 1, now(), now())
		RETURNING effective_from, created_at, updated_at, version`,
		plan.ID, plan.Code, plan.Name, plan.Description, plan.Currency, plan.PriceMinor,
		plan.BillingInterval, plan.IntervalCount, plan.GraceDays, plan.Status,
		nullableTime(plan.EffectiveFrom), plan.EffectiveTo).
		Scan(&plan.EffectiveFrom, &plan.CreatedAt, &plan.UpdatedAt, &plan.Version)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create plan: %w", err)
	}
	return nil
}

// GetPlan loads a plan by ID.
func (r *BillingRepository) GetPlan(ctx context.Context, q database.Querier,
	id string) (*models.SubscriptionPlan, error) {
	if q == nil {
		q = r.db
	}
	return scanPlan(q.QueryRow(ctx,
		`SELECT `+planColumns+` FROM finance_subscription_plans WHERE id = $1`, id))
}

// GetPlanByCode loads a plan by its stable code.
func (r *BillingRepository) GetPlanByCode(ctx context.Context, q database.Querier,
	code string) (*models.SubscriptionPlan, error) {
	if q == nil {
		q = r.db
	}
	return scanPlan(q.QueryRow(ctx,
		`SELECT `+planColumns+` FROM finance_subscription_plans WHERE code = $1`, code))
}

// SetPlanPricing configures or retires a plan's price under an expected
// version.
func (r *BillingRepository) SetPlanPricing(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, priceMinor *int64, status string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_subscription_plans SET
			price_minor = $3, status = $4, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2`, id, expectedVersion, priceMinor, status)
	if err != nil {
		return fmt.Errorf("finance: set plan pricing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

const subscriptionColumns = `id, organization_id, plan_id, currency, status,
	current_period_start, current_period_end, grace_expires_at, activated_at, cancelled_at,
	COALESCE(latest_invoice_id, ''), idempotency_key, request_fingerprint,
	version, created_at, updated_at`

func scanSubscription(row pgx.Row) (*models.Subscription, error) {
	var subscription models.Subscription
	err := row.Scan(&subscription.ID, &subscription.OrganizationID, &subscription.PlanID,
		&subscription.Currency, &subscription.Status, &subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd, &subscription.GraceExpiresAt, &subscription.ActivatedAt,
		&subscription.CancelledAt, &subscription.LatestInvoiceID, &subscription.IdempotencyKey,
		&subscription.RequestFingerprint, &subscription.Version, &subscription.CreatedAt,
		&subscription.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan subscription: %w", err)
	}
	return &subscription, nil
}

// CreateSubscription inserts a PENDING subscription. The partial unique index
// on live subscriptions is what refuses a second one for the same
// organization.
func (r *BillingRepository) CreateSubscription(ctx context.Context, q database.Querier,
	subscription *models.Subscription) error {
	if q == nil {
		q = r.db
	}
	if subscription.ID == "" {
		subscription.ID = ids.New(ids.PrefixSubscription)
	}
	if subscription.Status == "" {
		subscription.Status = models.SubscriptionPending
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_subscriptions
			(id, organization_id, plan_id, currency, status, idempotency_key,
			 request_fingerprint, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 1, now(), now())
		RETURNING version, created_at, updated_at`,
		subscription.ID, subscription.OrganizationID, subscription.PlanID,
		subscription.Currency, subscription.Status, subscription.IdempotencyKey,
		subscription.RequestFingerprint).
		Scan(&subscription.Version, &subscription.CreatedAt, &subscription.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create subscription: %w", err)
	}
	return nil
}

// GetSubscription loads a subscription inside a tenant scope.
func (r *BillingRepository) GetSubscription(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.Subscription, error) {
	if q == nil {
		q = r.db
	}
	return scanSubscription(q.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM finance_subscriptions
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
}

// GetLiveSubscription loads the organization's current subscription.
func (r *BillingRepository) GetLiveSubscription(ctx context.Context, q database.Querier,
	organizationID string) (*models.Subscription, error) {
	if q == nil {
		q = r.db
	}
	return scanSubscription(q.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM finance_subscriptions
		 WHERE organization_id = $1
		   AND status IN ('PENDING', 'ACTIVE', 'PAST_DUE', 'GRACE_PERIOD', 'SUSPENDED')`,
		organizationID))
}

// FindSubscriptionByIdempotencyKey resolves a replayed create.
func (r *BillingRepository) FindSubscriptionByIdempotencyKey(ctx context.Context,
	q database.Querier, organizationID string, key string) (*models.Subscription, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanSubscription(q.QueryRow(ctx,
		`SELECT `+subscriptionColumns+` FROM finance_subscriptions
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// TransitionSubscription moves a subscription under an expected version.
func (r *BillingRepository) TransitionSubscription(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status string, periodStart *time.Time,
	periodEnd *time.Time, graceExpiresAt *time.Time, latestInvoiceID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_subscriptions SET
			status = $3,
			current_period_start = COALESCE($4, current_period_start),
			current_period_end = COALESCE($5, current_period_end),
			grace_expires_at = $6,
			activated_at = CASE WHEN $3 = 'ACTIVE' AND activated_at IS NULL THEN now() ELSE activated_at END,
			cancelled_at = CASE WHEN $3 = 'CANCELLED' THEN now() ELSE cancelled_at END,
			latest_invoice_id = COALESCE(NULLIF($7, ''), latest_invoice_id),
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2`,
		id, expectedVersion, status, periodStart, periodEnd, graceExpiresAt, latestInvoiceID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: transition subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// DueSubscriptions returns subscriptions whose period or grace has elapsed.
func (r *BillingRepository) DueSubscriptions(ctx context.Context, q database.Querier,
	now time.Time, limit int) ([]models.Subscription, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
		SELECT `+subscriptionColumns+` FROM finance_subscriptions
		WHERE (status = 'ACTIVE' AND current_period_end IS NOT NULL AND current_period_end <= $1)
		   OR (status = 'GRACE_PERIOD' AND grace_expires_at IS NOT NULL AND grace_expires_at <= $1)
		ORDER BY COALESCE(current_period_end, grace_expires_at)
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list due subscriptions: %w", err)
	}
	defer rows.Close()
	return collectSubscriptions(rows)
}

func collectSubscriptions(rows pgx.Rows) ([]models.Subscription, error) {
	var subscriptions []models.Subscription
	for rows.Next() {
		var subscription models.Subscription
		if err := rows.Scan(&subscription.ID, &subscription.OrganizationID, &subscription.PlanID,
			&subscription.Currency, &subscription.Status, &subscription.CurrentPeriodStart,
			&subscription.CurrentPeriodEnd, &subscription.GraceExpiresAt, &subscription.ActivatedAt,
			&subscription.CancelledAt, &subscription.LatestInvoiceID, &subscription.IdempotencyKey,
			&subscription.RequestFingerprint, &subscription.Version, &subscription.CreatedAt,
			&subscription.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan subscription: %w", err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, rows.Err()
}

const invoiceColumns = `id, organization_id, COALESCE(subscription_id, ''), number, currency,
	subtotal_minor, tax_minor, total_minor, amount_paid_minor, status, period_start, period_end,
	issued_at, due_at, paid_at, idempotency_key, request_fingerprint, correlation_id,
	version, created_at, updated_at`

func scanInvoice(row pgx.Row) (*models.Invoice, error) {
	var invoice models.Invoice
	err := row.Scan(&invoice.ID, &invoice.OrganizationID, &invoice.SubscriptionID, &invoice.Number,
		&invoice.Currency, &invoice.SubtotalMinor, &invoice.TaxMinor, &invoice.TotalMinor,
		&invoice.AmountPaidMinor, &invoice.Status, &invoice.PeriodStart, &invoice.PeriodEnd,
		&invoice.IssuedAt, &invoice.DueAt, &invoice.PaidAt, &invoice.IdempotencyKey,
		&invoice.RequestFingerprint, &invoice.CorrelationID, &invoice.Version,
		&invoice.CreatedAt, &invoice.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan invoice: %w", err)
	}
	return &invoice, nil
}

// CreateInvoice inserts an invoice and its lines.
func (r *BillingRepository) CreateInvoice(ctx context.Context, q database.Querier,
	invoice *models.Invoice, lines []models.InvoiceLine) error {
	if q == nil {
		q = r.db
	}
	if invoice.ID == "" {
		invoice.ID = ids.New(ids.PrefixInvoice)
	}
	if invoice.Status == "" {
		invoice.Status = models.InvoiceOpen
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_invoices
			(id, organization_id, subscription_id, number, currency, subtotal_minor, tax_minor,
			 total_minor, amount_paid_minor, status, period_start, period_end, issued_at, due_at,
			 idempotency_key, request_fingerprint, correlation_id, version, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, 0, $9, $10, $11, now(), $12,
		        $13, $14, $15, 1, now(), now())
		RETURNING issued_at, version, created_at, updated_at`,
		invoice.ID, invoice.OrganizationID, invoice.SubscriptionID, invoice.Number,
		invoice.Currency, invoice.SubtotalMinor, invoice.TaxMinor, invoice.TotalMinor,
		invoice.Status, invoice.PeriodStart, invoice.PeriodEnd, invoice.DueAt,
		invoice.IdempotencyKey, invoice.RequestFingerprint, invoice.CorrelationID).
		Scan(&invoice.IssuedAt, &invoice.Version, &invoice.CreatedAt, &invoice.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create invoice: %w", err)
	}

	for index := range lines {
		line := &lines[index]
		line.ID = ids.New(ids.PrefixInvoiceLine)
		line.InvoiceID = invoice.ID
		line.LineNo = index + 1
		if line.Currency == "" {
			line.Currency = invoice.Currency
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO finance_invoice_lines
				(id, invoice_id, line_no, description, quantity, unit_amount_minor,
				 amount_minor, currency, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
			line.ID, line.InvoiceID, line.LineNo, line.Description, line.Quantity,
			line.UnitAmountMinor, line.AmountMinor, line.Currency); err != nil {
			return fmt.Errorf("finance: create invoice line: %w", err)
		}
	}
	invoice.Lines = lines
	return nil
}

// GetInvoice loads an invoice with its lines inside a tenant scope.
func (r *BillingRepository) GetInvoice(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.Invoice, error) {
	if q == nil {
		q = r.db
	}
	invoice, err := scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM finance_invoices
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
	if err != nil {
		return nil, err
	}
	lines, err := r.InvoiceLines(ctx, q, invoice.ID)
	if err != nil {
		return nil, err
	}
	invoice.Lines = lines
	return invoice, nil
}

// LockInvoice loads an invoice FOR UPDATE.
func (r *BillingRepository) LockInvoice(ctx context.Context, q database.Querier,
	id string) (*models.Invoice, error) {
	return scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM finance_invoices WHERE id = $1 FOR UPDATE`, id))
}

// FindInvoiceByIdempotencyKey resolves a replayed invoice creation.
func (r *BillingRepository) FindInvoiceByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Invoice, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanInvoice(q.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM finance_invoices
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// InvoiceLines loads the ordered lines of an invoice.
func (r *BillingRepository) InvoiceLines(ctx context.Context, q database.Querier,
	invoiceID string) ([]models.InvoiceLine, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `
		SELECT id, invoice_id, line_no, description, quantity, unit_amount_minor,
		       amount_minor, currency, created_at
		FROM finance_invoice_lines WHERE invoice_id = $1 ORDER BY line_no`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("finance: list invoice lines: %w", err)
	}
	defer rows.Close()

	var lines []models.InvoiceLine
	for rows.Next() {
		var line models.InvoiceLine
		if err := rows.Scan(&line.ID, &line.InvoiceID, &line.LineNo, &line.Description,
			&line.Quantity, &line.UnitAmountMinor, &line.AmountMinor, &line.Currency,
			&line.CreatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan invoice line: %w", err)
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// ApplyPaymentToInvoice records money received and moves the invoice status
// under an expected version.
//
// The status is derived from the resulting paid amount rather than supplied by
// the caller: a caller that could name the status could mark an unpaid invoice
// PAID.
func (r *BillingRepository) ApplyPaymentToInvoice(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, amountMinor int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_invoices SET
			amount_paid_minor = amount_paid_minor + $3,
			status = CASE
				WHEN amount_paid_minor + $3 >= total_minor THEN 'PAID'
				WHEN amount_paid_minor + $3 > 0 THEN 'PARTIALLY_PAID'
				ELSE status END,
			paid_at = CASE
				WHEN amount_paid_minor + $3 >= total_minor THEN now()
				ELSE paid_at END,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2 AND amount_paid_minor + $3 >= 0
		  AND status IN ('OPEN', 'PARTIALLY_PAID')`,
		id, expectedVersion, amountMinor)
	if err != nil {
		return fmt.Errorf("finance: apply payment to invoice: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SetInvoiceStatus voids or expires an invoice under an expected version.
func (r *BillingRepository) SetInvoiceStatus(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status string) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_invoices SET status = $3, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2`, id, expectedVersion, status)
	if err != nil {
		return fmt.Errorf("finance: set invoice status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// OverdueInvoices returns open invoices past their due date.
func (r *BillingRepository) OverdueInvoices(ctx context.Context, q database.Querier,
	now time.Time, limit int) ([]models.Invoice, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
		SELECT `+invoiceColumns+` FROM finance_invoices
		WHERE status IN ('OPEN', 'PARTIALLY_PAID') AND due_at <= $1
		ORDER BY due_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list overdue invoices: %w", err)
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		var invoice models.Invoice
		if err := rows.Scan(&invoice.ID, &invoice.OrganizationID, &invoice.SubscriptionID,
			&invoice.Number, &invoice.Currency, &invoice.SubtotalMinor, &invoice.TaxMinor,
			&invoice.TotalMinor, &invoice.AmountPaidMinor, &invoice.Status, &invoice.PeriodStart,
			&invoice.PeriodEnd, &invoice.IssuedAt, &invoice.DueAt, &invoice.PaidAt,
			&invoice.IdempotencyKey, &invoice.RequestFingerprint, &invoice.CorrelationID,
			&invoice.Version, &invoice.CreatedAt, &invoice.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan invoice: %w", err)
		}
		invoices = append(invoices, invoice)
	}
	return invoices, rows.Err()
}

// OverpaidInvoices finds invoices paid beyond their total — a reconciliation
// signal, never an expected state.
func (r *BillingRepository) OverpaidInvoices(ctx context.Context, q database.Querier,
	limit int) ([]string, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.Query(ctx,
		`SELECT id FROM finance_invoices WHERE amount_paid_minor > total_minor LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: find overpaid invoices: %w", err)
	}
	defer rows.Close()

	var invoiceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("finance: scan overpaid invoice: %w", err)
		}
		invoiceIDs = append(invoiceIDs, id)
	}
	return invoiceIDs, rows.Err()
}

const paymentColumns = `id, organization_id, invoice_id, amount_minor, currency, method, status,
	external_reference, COALESCE(journal_id, ''), idempotency_key, request_fingerprint,
	correlation_id, last_error, version, created_at, updated_at`

func scanPayment(row pgx.Row) (*models.Payment, error) {
	var payment models.Payment
	err := row.Scan(&payment.ID, &payment.OrganizationID, &payment.InvoiceID, &payment.AmountMinor,
		&payment.Currency, &payment.Method, &payment.Status, &payment.ExternalReference,
		&payment.JournalID, &payment.IdempotencyKey, &payment.RequestFingerprint,
		&payment.CorrelationID, &payment.LastError, &payment.Version,
		&payment.CreatedAt, &payment.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan payment: %w", err)
	}
	return &payment, nil
}

// CreatePayment records a payment attempt against an invoice.
func (r *BillingRepository) CreatePayment(ctx context.Context, q database.Querier,
	payment *models.Payment) error {
	if q == nil {
		q = r.db
	}
	if payment.ID == "" {
		payment.ID = ids.New(ids.PrefixPayment)
	}
	if payment.Status == "" {
		payment.Status = models.PaymentPending
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_payments
			(id, organization_id, invoice_id, amount_minor, currency, method, status,
			 external_reference, journal_id, idempotency_key, request_fingerprint,
			 correlation_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12, 1, now(), now())
		RETURNING version, created_at, updated_at`,
		payment.ID, payment.OrganizationID, payment.InvoiceID, payment.AmountMinor,
		payment.Currency, payment.Method, payment.Status, payment.ExternalReference,
		payment.JournalID, payment.IdempotencyKey, payment.RequestFingerprint,
		payment.CorrelationID).
		Scan(&payment.Version, &payment.CreatedAt, &payment.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create payment: %w", err)
	}
	return nil
}

// GetPayment loads a payment inside a tenant scope.
func (r *BillingRepository) GetPayment(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.Payment, error) {
	if q == nil {
		q = r.db
	}
	return scanPayment(q.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM finance_payments
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
}

// FindPaymentByIdempotencyKey resolves a replayed payment.
func (r *BillingRepository) FindPaymentByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Payment, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanPayment(q.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM finance_payments
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// SetPaymentOutcome records the resolved state of a payment attempt.
func (r *BillingRepository) SetPaymentOutcome(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status string, journalID string,
	externalReference string, lastError string) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_payments SET
			status = $3,
			journal_id = COALESCE(NULLIF($4, ''), journal_id),
			external_reference = COALESCE(NULLIF($5, ''), external_reference),
			last_error = $6,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2`,
		id, expectedVersion, status, journalID, externalReference, lastError)
	if err != nil {
		return fmt.Errorf("finance: set payment outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// NextInvoiceNumber allocates a human-readable sequential invoice number.
//
// The number is derived from the count of existing invoices under a row lock
// so two concurrent creations cannot mint the same one; the unique constraint
// on the column is the backstop.
func (r *BillingRepository) NextInvoiceNumber(ctx context.Context, q database.Querier,
	prefix string) (string, error) {
	var next int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(max(substring(number from '[0-9]+$')::bigint), 0) + 1
		FROM finance_invoices WHERE number LIKE $1 || '-%'`, prefix).Scan(&next)
	if err != nil {
		return "", fmt.Errorf("finance: allocate invoice number: %w", err)
	}
	return fmt.Sprintf("%s-%08d", prefix, next), nil
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
