package financeops

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

type calculatedLine struct {
	InvoiceLineInput
	ID        string
	LineTotal int64
}

func calculateInvoice(in InvoiceInput) (string, int64, int64, int64, []calculatedLine, error) {
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return "", 0, 0, 0, nil, err
	}
	if in.CustomerID == "" || in.DueDate.IsZero() || len(in.Lines) == 0 {
		return "", 0, 0, 0, nil, apperrors.Validation("Customer, due date and at least one line are required.", nil)
	}
	var subtotal, tax int64
	lines := make([]calculatedLine, 0, len(in.Lines))
	for _, line := range in.Lines {
		line.SourceType = strings.ToUpper(strings.TrimSpace(line.SourceType))
		if line.SourceType == "" || strings.TrimSpace(line.SourceReference) == "" || strings.TrimSpace(line.Description) == "" || line.Quantity <= 0 || line.UnitAmountMinor < 0 || line.TaxAmountMinor < 0 {
			return "", 0, 0, 0, nil, apperrors.Validation("Every invoice line requires a source, description, positive quantity and non-negative minor-unit amounts.", nil)
		}
		if line.UnitAmountMinor > 0 && line.Quantity > math.MaxInt64/line.UnitAmountMinor {
			return "", 0, 0, 0, nil, apperrors.Validation("Invoice line amount is too large.", nil)
		}
		lineTotal := line.Quantity * line.UnitAmountMinor
		if subtotal > math.MaxInt64-lineTotal || tax > math.MaxInt64-line.TaxAmountMinor {
			return "", 0, 0, 0, nil, apperrors.Validation("Invoice total is too large.", nil)
		}
		subtotal += lineTotal
		tax += line.TaxAmountMinor
		lines = append(lines, calculatedLine{InvoiceLineInput: line, ID: ids.New("inl"), LineTotal: lineTotal})
	}
	if subtotal > math.MaxInt64-tax {
		return "", 0, 0, 0, nil, apperrors.Validation("Invoice total is too large.", nil)
	}
	return currency, subtotal, tax, subtotal + tax, lines, nil
}

func (s *Service) CreateInvoice(ctx context.Context, t tenant.Context, in InvoiceInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	currency, subtotal, tax, total, lines, err := calculateInvoice(in)
	if err != nil {
		return nil, err
	}
	id := ids.New("inv")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_company_customers WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, in.CustomerID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperrors.Validation("Customer does not belong to this organization.", nil)
		}
		if in.ContractID != "" {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_contracts WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, in.ContractID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return apperrors.Validation("Contract does not belong to this organization.", nil)
			}
		}
		if in.QuotationID != "" {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_quotations WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, in.QuotationID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return apperrors.Validation("Quotation does not belong to this organization.", nil)
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_invoices(id,organization_id,branch_id,customer_id,contract_id,quotation_id,currency,subtotal_minor,tax_total_minor,total_minor,due_date,created_by) VALUES($1,$2,NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12)`, id, t.OrganizationID, in.BranchID, in.CustomerID, in.ContractID, in.QuotationID, currency, subtotal, tax, total, in.DueDate, t.ActorID)
		if err != nil {
			return err
		}
		for _, line := range lines {
			_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_invoice_lines(id,organization_id,invoice_id,source_type,source_reference,description,quantity,unit_amount_minor,line_total_minor,tax_reference,tax_amount_minor,adjustment_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, line.ID, t.OrganizationID, id, line.SourceType, line.SourceReference, line.Description, line.Quantity, line.UnitAmountMinor, line.LineTotal, line.TaxReference, line.TaxAmountMinor, line.AdjustmentReference)
			if err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "invoice.created.v1", "invoice", id, 1, map[string]any{"invoiceId": id, "currency": currency, "subtotalMinor": subtotal, "taxTotalMinor": tax, "totalMinor": total})
	})
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, t, id)
}

const invoiceJSON = `SELECT row_to_json(x) FROM (SELECT i.*,COALESCE((SELECT json_agg(row_to_json(l) ORDER BY l.created_at) FROM fleet_partner_invoice_lines l WHERE l.organization_id=i.organization_id AND l.invoice_id=i.id),'[]'::json) AS lines FROM fleet_partner_invoices i WHERE i.organization_id=$1 AND i.id=$2) x`

func (s *Service) GetInvoice(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, false); err != nil {
		return nil, err
	}
	v, err := jsonRow(s.o.Pool.QueryRow(ctx, invoiceJSON, t.OrganizationID, id))
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}
func (s *Service) ListInvoices(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(i) FROM fleet_partner_invoices i WHERE organization_id=$1 AND (cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

func (s *Service) UpdateInvoice(ctx context.Context, t tenant.Context, id string, in InvoiceInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	currency, subtotal, tax, total, lines, err := calculateInvoice(in)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var branch, status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(branch_id,''),status,version FROM fleet_partner_invoices WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&branch, &status, &version); err != nil {
			return notFoundOnNoRows(err)
		}
		if !branchAllowed(t, branch) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		if status != "DRAFT" && status != "PENDING_APPROVAL" {
			return apperrors.Conflict("Only draft invoices may be edited.", nil)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM fleet_partner_invoice_lines WHERE organization_id=$1 AND invoice_id=$2`, t.OrganizationID, id); err != nil {
			return err
		}
		for _, line := range lines {
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_invoice_lines(id,organization_id,invoice_id,source_type,source_reference,description,quantity,unit_amount_minor,line_total_minor,tax_reference,tax_amount_minor,adjustment_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, line.ID, t.OrganizationID, id, line.SourceType, line.SourceReference, line.Description, line.Quantity, line.UnitAmountMinor, line.LineTotal, line.TaxReference, line.TaxAmountMinor, line.AdjustmentReference); err != nil {
				return err
			}
		}
		var newVersion int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_invoices SET currency=$3,subtotal_minor=$4,tax_total_minor=$5,adjustment_total_minor=0,total_minor=$6,due_date=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, currency, subtotal, tax, total, in.DueDate).Scan(&newVersion); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "invoice.updated.v1", "invoice", id, newVersion, map[string]any{"invoiceId": id, "currency": currency, "totalMinor": total})
	})
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, t, id)
}

func (s *Service) invoiceCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, currency string
		var version, total int64
		var due time.Time
		if err := tx.QueryRow(ctx, `SELECT status,version,currency,total_minor,due_date::timestamptz FROM fleet_partner_invoices WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version, &currency, &total, &due); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != expected {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		newStatus := status
		event := "invoice." + command + "d.v1"
		switch command {
		case "approve":
			if status != "DRAFT" && status != "PENDING_APPROVAL" {
				return apperrors.Conflict("Invoice cannot be approved from its current state.", nil)
			}
			newStatus = "APPROVED"
		case "issue":
			if status != "APPROVED" {
				return apperrors.Conflict("Only an approved invoice may be issued.", nil)
			}
			newStatus = "ISSUED"
		case "cancel":
			if status == "PAID" || status == "CREDITED" || status == "CANCELLED" {
				return apperrors.Conflict("Invoice cannot be cancelled from its current state.", nil)
			}
			newStatus = "CANCELLED"
		default:
			return apperrors.Validation("Unsupported invoice command.", nil)
		}
		var newVersion int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_invoices SET status=$3,issued_at=CASE WHEN $3='ISSUED' THEN now() ELSE issued_at END,cancelled_at=CASE WHEN $3='CANCELLED' THEN now() ELSE cancelled_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, newStatus).Scan(&newVersion); err != nil {
			return err
		}
		if command == "issue" {
			var snapshot []byte
			if err := tx.QueryRow(ctx, invoiceJSON, t.OrganizationID, id).Scan(&snapshot); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_invoice_snapshots(id,organization_id,invoice_id,invoice_version,snapshot,issued_by) VALUES($1,$2,$3,$4,$5,$6)`, ids.New("isn"), t.OrganizationID, id, newVersion, snapshot, t.ActorID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_receivable_entries(id,organization_id,invoice_id,original_amount_minor,outstanding_amount_minor,currency,due_at) VALUES($1,$2,$3,$4,$4,$5,$6)`, ids.New("rcv"), t.OrganizationID, id, total, currency, due); err != nil {
				return err
			}
			if err := s.record(ctx, tx, t, "receivable.created.v1", "invoice", id, newVersion, map[string]any{"invoiceId": id, "currency": currency, "amountMinor": total}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, event, "invoice", id, newVersion, map[string]any{"invoiceId": id, "status": newStatus, "currency": currency, "totalMinor": total})
	})
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, t, id)
}

func (s *Service) ApproveInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "approve", v)
}
func (s *Service) IssueInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "issue", v)
}
func (s *Service) CancelInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "cancel", v)
}

func (s *Service) CreditInvoice(ctx context.Context, t tenant.Context, id string, in CreditNoteInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 || strings.TrimSpace(in.Reason) == "" {
		return nil, apperrors.Validation("A positive credit amount and reason are required.", nil)
	}
	in.AffectedLines = rawOr(in.AffectedLines, "[]")
	noteID := ids.New("crn")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, currency string
		var version, total int64
		if err := tx.QueryRow(ctx, `SELECT status,version,currency,total_minor FROM fleet_partner_invoices WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version, &currency, &total); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		if status != "ISSUED" && status != "PARTIALLY_PAID" && status != "PAID" {
			return apperrors.Conflict("Only an issued invoice may be credited.", nil)
		}
		if in.AmountMinor > total {
			return apperrors.Conflict("Credit note exceeds the eligible invoice value.", nil)
		}
		var receivableID string
		var receivableOriginal, receivablePaid, receivableOutstanding int64
		if err := tx.QueryRow(ctx, `SELECT id,original_amount_minor,paid_amount_minor,outstanding_amount_minor FROM fleet_partner_receivable_entries WHERE organization_id=$1 AND invoice_id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&receivableID, &receivableOriginal, &receivablePaid, &receivableOutstanding); err != nil {
			return err
		}
		if in.AmountMinor > receivableOutstanding {
			return apperrors.Conflict("Credit note exceeds the unpaid receivable value; paid value requires an authoritative refund workflow.", nil)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_credit_notes(id,organization_id,invoice_id,currency,amount_minor,affected_lines,reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, noteID, t.OrganizationID, id, currency, in.AmountMinor, in.AffectedLines, in.Reason, t.ActorID); err != nil {
			return err
		}
		newReceivableOriginal := receivableOriginal - in.AmountMinor
		newOutstanding := receivableOutstanding - in.AmountMinor
		newReceivableStatus := "OPEN"
		if newOutstanding == 0 {
			newReceivableStatus = "PAID"
		} else if receivablePaid > 0 {
			newReceivableStatus = "PARTIALLY_PAID"
		}
		if _, err := tx.Exec(ctx, `UPDATE fleet_partner_receivable_entries SET original_amount_minor=$3,outstanding_amount_minor=$4,status=$5,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, receivableID, newReceivableOriginal, newOutstanding, newReceivableStatus); err != nil {
			return err
		}
		newStatus := status
		if newOutstanding == 0 {
			newStatus = "CREDITED"
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_invoices SET adjustment_total_minor=adjustment_total_minor-$3,total_minor=total_minor-$3,status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, in.AmountMinor, newStatus).Scan(&nv); err != nil {
			return err
		}
		if err := s.record(ctx, tx, t, "receivable.adjusted.v1", "receivable", receivableID, nv, map[string]any{"receivableId": receivableID, "invoiceId": id, "currency": currency, "creditAmountMinor": in.AmountMinor, "outstandingAmountMinor": newOutstanding}); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "credit_note.issued.v1", "credit_note", noteID, 1, map[string]any{"creditNoteId": noteID, "invoiceId": id, "currency": currency, "amountMinor": in.AmountMinor, "invoiceVersion": nv})
	})
	if err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(c) FROM fleet_partner_credit_notes c WHERE organization_id=$1 AND id=$2`, t.OrganizationID, noteID))
}

func (s *Service) ListReceivables(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Receivables, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_receivable_entries r WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) GetReceivable(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Receivables, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_receivable_entries r WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}

func (s *Service) ApplyPayment(ctx context.Context, in PaymentEvent) error {
	if err := s.gate(s.o.Flags.Receivables, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.InvoiceID == "" || in.AmountMinor <= 0 || in.SourceVersion <= 0 {
		return apperrors.Validation("Payment event is incomplete.", nil)
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "payment-wallet-service"}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var rid, currentCurrency string
		var original, paid, outstanding, sourceVersion int64
		err := tx.QueryRow(ctx, `SELECT id,currency,original_amount_minor,paid_amount_minor,outstanding_amount_minor,source_version FROM fleet_partner_receivable_entries WHERE organization_id=$1 AND invoice_id=$2 FOR UPDATE`, in.OrganizationID, in.InvoiceID).Scan(&rid, &currentCurrency, &original, &paid, &outstanding, &sourceVersion)
		if err != nil {
			return notFoundOnNoRows(err)
		}
		if sourceVersion >= in.SourceVersion {
			return nil
		}
		if currency != currentCurrency {
			return apperrors.Conflict("Payment currency does not match the invoice.", nil)
		}
		amount := in.AmountMinor
		if amount > outstanding {
			amount = outstanding
		}
		newPaid := paid + amount
		newOutstanding := original - newPaid
		status := "PARTIALLY_PAID"
		if newOutstanding == 0 {
			status = "PAID"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO fleet_partner_receivable_payment_history(id,organization_id,receivable_id,source_event_id,source_version,amount_minor,currency,payment_reference,wallet_operation_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(organization_id,source_event_id) DO NOTHING`, ids.New("rph"), in.OrganizationID, rid, in.EventID, in.SourceVersion, amount, currency, in.PaymentReference, in.WalletOperationReference); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE fleet_partner_receivable_entries SET paid_amount_minor=$3,outstanding_amount_minor=$4,status=$5,payment_reference=$6,wallet_operation_reference=$7,source_version=$8,updated_at=now() WHERE organization_id=$1 AND id=$2`, in.OrganizationID, rid, newPaid, newOutstanding, status, in.PaymentReference, in.WalletOperationReference, in.SourceVersion); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE fleet_partner_invoices SET status=$3,paid_at=CASE WHEN $3='PAID' THEN now() ELSE paid_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, in.OrganizationID, in.InvoiceID, status); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "receivable.payment_applied.v1", "receivable", rid, in.SourceVersion, map[string]any{"receivableId": rid, "invoiceId": in.InvoiceID, "currency": currency, "amountMinor": amount, "status": status, "sourceVersion": in.SourceVersion})
	})
}

func (s *Service) SweepOverdue(ctx context.Context, limit int) (int64, error) {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	tag, err := s.o.Pool.Exec(ctx, `WITH due AS (SELECT id,invoice_id FROM fleet_partner_receivable_entries WHERE status IN('OPEN','PARTIALLY_PAID') AND due_at<now() ORDER BY due_at LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE fleet_partner_receivable_entries r SET status='OVERDUE',updated_at=now() FROM due WHERE r.id=due.id`, limit)
	return tag.RowsAffected(), err
}

var _ = json.Valid
var _ = errors.Is
