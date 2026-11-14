package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// CreateInvoice creates a draft invoice for a customer of the organization.
func (s *Service) CreateInvoice(ctx context.Context, t tenant.Context, in models.InvoiceInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	totals, err := models.CalculateInvoice(in)
	if err != nil {
		return nil, err
	}
	id := ids.New("inv")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		exists, err := s.o.Store.CustomerExists(ctx, tx, t.OrganizationID, in.CustomerID)
		if err != nil {
			return err
		}
		if !exists {
			return apperrors.Validation("Customer does not belong to this organization.", nil)
		}
		if in.ContractID != "" {
			if exists, err = s.o.Store.ContractExists(ctx, tx, t.OrganizationID, in.ContractID); err != nil {
				return err
			}
			if !exists {
				return apperrors.Validation("Contract does not belong to this organization.", nil)
			}
		}
		if in.QuotationID != "" {
			if exists, err = s.o.Store.QuotationExists(ctx, tx, t.OrganizationID, in.QuotationID); err != nil {
				return err
			}
			if !exists {
				return apperrors.Validation("Quotation does not belong to this organization.", nil)
			}
		}
		if err := s.o.Store.InsertInvoice(ctx, tx, id, t.OrganizationID, t.ActorID, in, totals); err != nil {
			return err
		}
		if err := s.o.Store.InsertInvoiceLines(ctx, tx, t.OrganizationID, id, totals.Lines); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "invoice.created.v1", "invoice", id, 1, map[string]any{"invoiceId": id, "currency": totals.Currency, "subtotalMinor": totals.Subtotal, "taxTotalMinor": totals.Tax, "totalMinor": totals.Total})
	})
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, t, id)
}

// GetInvoice reads an invoice in the caller's branch scope.
func (s *Service) GetInvoice(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, false); err != nil {
		return nil, err
	}
	v, err := s.o.Store.GetInvoice(ctx, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// ListInvoices lists invoices in the caller's branch scope.
func (s *Service) ListInvoices(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListInvoices(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// UpdateInvoice rewrites a draft invoice at the expected version.
func (s *Service) UpdateInvoice(ctx context.Context, t tenant.Context, id string, in models.InvoiceInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	totals, err := models.CalculateInvoice(in)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		state, err := s.o.Store.LockInvoice(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if !branchAllowed(t, state.Branch) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if state.Version != in.ExpectedVersion {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		if state.Status != "DRAFT" && state.Status != "PENDING_APPROVAL" {
			return apperrors.Conflict("Only draft invoices may be edited.", nil)
		}
		if err := s.o.Store.DeleteInvoiceLines(ctx, tx, t.OrganizationID, id); err != nil {
			return err
		}
		if err := s.o.Store.InsertInvoiceLines(ctx, tx, t.OrganizationID, id, totals.Lines); err != nil {
			return err
		}
		newVersion, err := s.o.Store.UpdateInvoiceTotals(ctx, tx, t.OrganizationID, id, totals, in.DueDate)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "invoice.updated.v1", "invoice", id, newVersion, map[string]any{"invoiceId": id, "currency": totals.Currency, "totalMinor": totals.Total})
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
		state, err := s.o.Store.LockInvoice(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if state.Version != expected {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		newStatus, err := models.NextInvoiceStatus(state.Status, command)
		if err != nil {
			return err
		}
		newVersion, err := s.o.Store.SetInvoiceStatus(ctx, tx, t.OrganizationID, id, newStatus)
		if err != nil {
			return err
		}
		if command == "issue" {
			snapshot, err := s.o.Store.InvoiceSnapshot(ctx, tx, t.OrganizationID, id)
			if err != nil {
				return err
			}
			if err := s.o.Store.InsertInvoiceSnapshot(ctx, tx, t.OrganizationID, id, newVersion, snapshot, t.ActorID); err != nil {
				return err
			}
			if err := s.o.Store.InsertReceivable(ctx, tx, t.OrganizationID, id, state.Total, state.Currency, state.Due); err != nil {
				return err
			}
			if err := s.record(ctx, tx, t, "receivable.created.v1", "invoice", id, newVersion, map[string]any{"invoiceId": id, "currency": state.Currency, "amountMinor": state.Total}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "invoice."+command+"d.v1", "invoice", id, newVersion, map[string]any{"invoiceId": id, "status": newStatus, "currency": state.Currency, "totalMinor": state.Total})
	})
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, t, id)
}

// ApproveInvoice approves a draft invoice.
func (s *Service) ApproveInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "approve", v)
}

// IssueInvoice issues an approved invoice and opens its receivable.
func (s *Service) IssueInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "issue", v)
}

// CancelInvoice cancels an unpaid invoice.
func (s *Service) CancelInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error) {
	return s.invoiceCommand(ctx, t, id, "cancel", v)
}

// CreditInvoice issues a credit note against the unpaid part of an issued
// invoice.
func (s *Service) CreditInvoice(ctx context.Context, t tenant.Context, id string, in models.CreditNoteInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 || strings.TrimSpace(in.Reason) == "" {
		return nil, apperrors.Validation("A positive credit amount and reason are required.", nil)
	}
	in.AffectedLines = models.RawOr(in.AffectedLines, "[]")
	noteID := ids.New("crn")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		state, err := s.o.Store.LockInvoice(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if state.Version != in.ExpectedVersion {
			return apperrors.Conflict("Invoice version changed.", nil)
		}
		if state.Status != "ISSUED" && state.Status != "PARTIALLY_PAID" && state.Status != "PAID" {
			return apperrors.Conflict("Only an issued invoice may be credited.", nil)
		}
		if in.AmountMinor > state.Total {
			return apperrors.Conflict("Credit note exceeds the eligible invoice value.", nil)
		}
		receivable, err := s.o.Store.LockReceivable(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if in.AmountMinor > receivable.Outstanding {
			return apperrors.Conflict("Credit note exceeds the unpaid receivable value; paid value requires an authoritative refund workflow.", nil)
		}
		if err := s.o.Store.InsertCreditNote(ctx, tx, noteID, t.OrganizationID, id, state.Currency, in.AmountMinor, in.AffectedLines, in.Reason, t.ActorID); err != nil {
			return err
		}
		newOriginal := receivable.Original - in.AmountMinor
		newOutstanding := receivable.Outstanding - in.AmountMinor
		if err := s.o.Store.AdjustReceivable(ctx, tx, t.OrganizationID, receivable.ID, newOriginal, newOutstanding, models.ReceivableStatusAfterCredit(newOutstanding, receivable.Paid)); err != nil {
			return err
		}
		newStatus := state.Status
		if newOutstanding == 0 {
			newStatus = "CREDITED"
		}
		nv, err := s.o.Store.CreditInvoiceTotals(ctx, tx, t.OrganizationID, id, in.AmountMinor, newStatus)
		if err != nil {
			return err
		}
		if err := s.record(ctx, tx, t, "receivable.adjusted.v1", "receivable", receivable.ID, nv, map[string]any{"receivableId": receivable.ID, "invoiceId": id, "currency": state.Currency, "creditAmountMinor": in.AmountMinor, "outstandingAmountMinor": newOutstanding}); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "credit_note.issued.v1", "credit_note", noteID, 1, map[string]any{"creditNoteId": noteID, "invoiceId": id, "currency": state.Currency, "amountMinor": in.AmountMinor, "invoiceVersion": nv})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetCreditNote(ctx, t.OrganizationID, noteID)
}

// ListReceivables lists the organization's receivables.
func (s *Service) ListReceivables(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Receivables, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListReceivables(ctx, t.OrganizationID)
}

// GetReceivable reads one receivable.
func (s *Service) GetReceivable(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Receivables, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetReceivable(ctx, t.OrganizationID, id)
}

// ApplyPayment applies Wallet's payment event to an invoice's receivable.
// Events at or below the receivable's source version are ignored.
func (s *Service) ApplyPayment(ctx context.Context, in models.PaymentEvent) error {
	if err := s.gate(s.o.Flags.Receivables, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.InvoiceID == "" || in.AmountMinor <= 0 || in.SourceVersion <= 0 {
		return apperrors.Validation("Payment event is incomplete.", nil)
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "payment-wallet-service"}
	return s.tx(ctx, func(tx pgx.Tx) error {
		receivable, err := s.o.Store.LockReceivable(ctx, tx, in.OrganizationID, in.InvoiceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.NotFound("Resource not found.", nil)
			}
			return err
		}
		if receivable.SourceVersion >= in.SourceVersion {
			return nil
		}
		if currency != receivable.Currency {
			return apperrors.Conflict("Payment currency does not match the invoice.", nil)
		}
		amount := in.AmountMinor
		if amount > receivable.Outstanding {
			amount = receivable.Outstanding
		}
		newPaid := receivable.Paid + amount
		newOutstanding := receivable.Original - newPaid
		status := "PARTIALLY_PAID"
		if newOutstanding == 0 {
			status = "PAID"
		}
		if err := s.o.Store.InsertReceivablePayment(ctx, tx, receivable.ID, in, amount, currency); err != nil {
			return err
		}
		if err := s.o.Store.ApplyReceivablePayment(ctx, tx, receivable.ID, in, newPaid, newOutstanding, status); err != nil {
			return err
		}
		if err := s.o.Store.SetInvoicePaymentStatus(ctx, tx, in.OrganizationID, in.InvoiceID, status); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "receivable.payment_applied.v1", "receivable", receivable.ID, in.SourceVersion, map[string]any{"receivableId": receivable.ID, "invoiceId": in.InvoiceID, "currency": currency, "amountMinor": amount, "status": status, "sourceVersion": in.SourceVersion})
	})
}

// SweepOverdue marks receivables past due as overdue.
func (s *Service) SweepOverdue(ctx context.Context, limit int) (int64, error) {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	return s.o.Store.SweepOverdue(ctx, limit)
}
