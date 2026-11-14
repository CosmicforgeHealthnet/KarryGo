// Package repositories persists the Section 22 finance-operations records.
// Every statement is tenant-scoped by organization ID. Methods taking a
// database.Querier run inside the caller's transaction; nil means the pool.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the PostgreSQL finance-operations store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

// WithinTx runs f in one transaction.
func (r *Repository) WithinTx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, r.pool, f)
}

func notFoundOnNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound("Resource not found.", nil)
	}
	return err
}

func jsonRow(row pgx.Row) (map[string]any, error) {
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return nil, notFoundOnNoRows(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonList(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) exists(ctx context.Context, q database.Querier, sql string, args ...any) (bool, error) {
	var ok bool
	err := r.q(q).QueryRow(ctx, sql, args...).Scan(&ok)
	return ok, err
}

// ActiveOrganizations lists the organizations the maintenance pass covers.
func (r *Repository) ActiveOrganizations(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// ---------------------------------------------------------------------------
// Invoices
// ---------------------------------------------------------------------------

// CustomerExists reports whether the CRM customer belongs to the organization.
func (r *Repository) CustomerExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_company_customers WHERE organization_id=$1 AND id=$2)`, organizationID, id)
}

// ContractExists reports whether the contract belongs to the organization.
func (r *Repository) ContractExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_contracts WHERE organization_id=$1 AND id=$2)`, organizationID, id)
}

// QuotationExists reports whether the quotation belongs to the organization.
func (r *Repository) QuotationExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_quotations WHERE organization_id=$1 AND id=$2)`, organizationID, id)
}

// InsertInvoice creates a draft invoice.
func (r *Repository) InsertInvoice(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.InvoiceInput, totals models.InvoiceTotals) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_invoices(id,organization_id,branch_id,customer_id,contract_id,quotation_id,currency,subtotal_minor,tax_total_minor,total_minor,due_date,created_by) VALUES($1,$2,NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12)`, id, organizationID, in.BranchID, in.CustomerID, in.ContractID, in.QuotationID, totals.Currency, totals.Subtotal, totals.Tax, totals.Total, in.DueDate, actorID)
	return err
}

// InsertInvoiceLines stores an invoice's lines.
func (r *Repository) InsertInvoiceLines(ctx context.Context, q database.Querier, organizationID, invoiceID string, lines []models.CalculatedLine) error {
	for _, line := range lines {
		if _, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_invoice_lines(id,organization_id,invoice_id,source_type,source_reference,description,quantity,unit_amount_minor,line_total_minor,tax_reference,tax_amount_minor,adjustment_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, line.ID, organizationID, invoiceID, line.SourceType, line.SourceReference, line.Description, line.Quantity, line.UnitAmountMinor, line.LineTotal, line.TaxReference, line.TaxAmountMinor, line.AdjustmentReference); err != nil {
			return err
		}
	}
	return nil
}

// DeleteInvoiceLines removes an invoice's lines.
func (r *Repository) DeleteInvoiceLines(ctx context.Context, q database.Querier, organizationID, invoiceID string) error {
	_, err := r.q(q).Exec(ctx, `DELETE FROM fleet_partner_invoice_lines WHERE organization_id=$1 AND invoice_id=$2`, organizationID, invoiceID)
	return err
}

const invoiceJSON = `SELECT row_to_json(x) FROM (SELECT i.*,COALESCE((SELECT json_agg(row_to_json(l) ORDER BY l.created_at) FROM fleet_partner_invoice_lines l WHERE l.organization_id=i.organization_id AND l.invoice_id=i.id),'[]'::json) AS lines FROM fleet_partner_invoices i WHERE i.organization_id=$1 AND i.id=$2) x`

// GetInvoice reads an invoice with its lines.
func (r *Repository) GetInvoice(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, invoiceJSON, organizationID, id))
}

// InvoiceSnapshot reads the invoice-with-lines JSON in the caller's
// transaction.
func (r *Repository) InvoiceSnapshot(ctx context.Context, q database.Querier, organizationID, id string) ([]byte, error) {
	var snapshot []byte
	err := r.q(q).QueryRow(ctx, invoiceJSON, organizationID, id).Scan(&snapshot)
	return snapshot, err
}

// ListInvoices lists invoices in the permitted branches, newest first.
func (r *Repository) ListInvoices(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(i) FROM fleet_partner_invoices i WHERE organization_id=$1 AND (COALESCE(cardinality($2::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, organizationID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockInvoice reads an invoice's state FOR UPDATE; a missing invoice is not
// found.
func (r *Repository) LockInvoice(ctx context.Context, q database.Querier, organizationID, id string) (models.InvoiceState, error) {
	var s models.InvoiceState
	err := r.q(q).QueryRow(ctx, `SELECT COALESCE(branch_id,''),status,version,currency,total_minor,due_date::timestamptz FROM fleet_partner_invoices WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&s.Branch, &s.Status, &s.Version, &s.Currency, &s.Total, &s.Due)
	return s, notFoundOnNoRows(err)
}

// UpdateInvoiceTotals rewrites a draft invoice's totals and returns the
// version.
func (r *Repository) UpdateInvoiceTotals(ctx context.Context, q database.Querier, organizationID, id string, totals models.InvoiceTotals, due time.Time) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_invoices SET currency=$3,subtotal_minor=$4,tax_total_minor=$5,adjustment_total_minor=0,total_minor=$6,due_date=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, totals.Currency, totals.Subtotal, totals.Tax, totals.Total, due).Scan(&nv)
	return nv, err
}

// SetInvoiceStatus moves an invoice to status and returns the version.
func (r *Repository) SetInvoiceStatus(ctx context.Context, q database.Querier, organizationID, id, status string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_invoices SET status=$3,issued_at=CASE WHEN $3='ISSUED' THEN now() ELSE issued_at END,cancelled_at=CASE WHEN $3='CANCELLED' THEN now() ELSE cancelled_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status).Scan(&nv)
	return nv, err
}

// InsertInvoiceSnapshot stores the issued invoice snapshot.
func (r *Repository) InsertInvoiceSnapshot(ctx context.Context, q database.Querier, organizationID, invoiceID string, version int64, snapshot []byte, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_invoice_snapshots(id,organization_id,invoice_id,invoice_version,snapshot,issued_by) VALUES($1,$2,$3,$4,$5,$6)`, ids.New("isn"), organizationID, invoiceID, version, snapshot, actorID)
	return err
}

// CreditInvoiceTotals reduces an invoice by a credit note and returns the
// version.
func (r *Repository) CreditInvoiceTotals(ctx context.Context, q database.Querier, organizationID, id string, amount int64, status string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_invoices SET adjustment_total_minor=adjustment_total_minor-$3,total_minor=total_minor-$3,status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, amount, status).Scan(&nv)
	return nv, err
}

// SetInvoicePaymentStatus records the payment status a receivable reached.
func (r *Repository) SetInvoicePaymentStatus(ctx context.Context, q database.Querier, organizationID, invoiceID, status string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_invoices SET status=$3,paid_at=CASE WHEN $3='PAID' THEN now() ELSE paid_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, invoiceID, status)
	return err
}

// InsertCreditNote stores a credit note.
func (r *Repository) InsertCreditNote(ctx context.Context, q database.Querier, id, organizationID, invoiceID, currency string, amount int64, affectedLines json.RawMessage, reason, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_credit_notes(id,organization_id,invoice_id,currency,amount_minor,affected_lines,reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, organizationID, invoiceID, currency, amount, affectedLines, reason, actorID)
	return err
}

// GetCreditNote reads one credit note.
func (r *Repository) GetCreditNote(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(c) FROM fleet_partner_credit_notes c WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ---------------------------------------------------------------------------
// Receivables
// ---------------------------------------------------------------------------

// InsertReceivable opens the receivable of an issued invoice.
func (r *Repository) InsertReceivable(ctx context.Context, q database.Querier, organizationID, invoiceID string, amount int64, currency string, due time.Time) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_receivable_entries(id,organization_id,invoice_id,original_amount_minor,outstanding_amount_minor,currency,due_at) VALUES($1,$2,$3,$4,$4,$5,$6)`, ids.New("rcv"), organizationID, invoiceID, amount, currency, due)
	return err
}

// LockReceivable reads an invoice's receivable FOR UPDATE.
func (r *Repository) LockReceivable(ctx context.Context, q database.Querier, organizationID, invoiceID string) (models.ReceivableState, error) {
	var s models.ReceivableState
	err := r.q(q).QueryRow(ctx, `SELECT id,currency,original_amount_minor,paid_amount_minor,outstanding_amount_minor,source_version FROM fleet_partner_receivable_entries WHERE organization_id=$1 AND invoice_id=$2 FOR UPDATE`, organizationID, invoiceID).Scan(&s.ID, &s.Currency, &s.Original, &s.Paid, &s.Outstanding, &s.SourceVersion)
	return s, err
}

// AdjustReceivable records a credit against a receivable.
func (r *Repository) AdjustReceivable(ctx context.Context, q database.Querier, organizationID, id string, original, outstanding int64, status string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_receivable_entries SET original_amount_minor=$3,outstanding_amount_minor=$4,status=$5,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, id, original, outstanding, status)
	return err
}

// InsertReceivablePayment appends a payment once per source event.
func (r *Repository) InsertReceivablePayment(ctx context.Context, q database.Querier, receivableID string, in models.PaymentEvent, amount int64, currency string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_receivable_payment_history(id,organization_id,receivable_id,source_event_id,source_version,amount_minor,currency,payment_reference,wallet_operation_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(organization_id,source_event_id) DO NOTHING`, ids.New("rph"), in.OrganizationID, receivableID, in.EventID, in.SourceVersion, amount, currency, in.PaymentReference, in.WalletOperationReference)
	return err
}

// ApplyReceivablePayment writes a payment's effect onto a receivable.
func (r *Repository) ApplyReceivablePayment(ctx context.Context, q database.Querier, receivableID string, in models.PaymentEvent, paid, outstanding int64, status string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_receivable_entries SET paid_amount_minor=$3,outstanding_amount_minor=$4,status=$5,payment_reference=$6,wallet_operation_reference=$7,source_version=$8,updated_at=now() WHERE organization_id=$1 AND id=$2`, in.OrganizationID, receivableID, paid, outstanding, status, in.PaymentReference, in.WalletOperationReference, in.SourceVersion)
	return err
}

// ListReceivables lists receivables, newest first.
func (r *Repository) ListReceivables(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_receivable_entries r WHERE organization_id=$1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// GetReceivable reads one receivable.
func (r *Repository) GetReceivable(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_receivable_entries r WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// SweepOverdue marks open receivables past due as OVERDUE.
func (r *Repository) SweepOverdue(ctx context.Context, limit int) (int64, error) {
	tag, err := r.pool.Exec(ctx, `WITH due AS (SELECT id,invoice_id FROM fleet_partner_receivable_entries WHERE status IN('OPEN','PARTIALLY_PAID') AND due_at<now() ORDER BY due_at LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE fleet_partner_receivable_entries r SET status='OVERDUE',updated_at=now() FROM due WHERE r.id=due.id`, limit)
	return tag.RowsAffected(), err
}
