package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	jobInvoiceOverdue      = "SECTION22_INVOICE_OVERDUE"
	jobReceivableReconcile = "SECTION22_RECEIVABLE_RECONCILE"
	jobExpensePosting      = "SECTION22_EXPENSE_POSTING"
	jobPostingReconcile    = "SECTION22_POSTING_RECONCILE"
	jobBudgetProjection    = "SECTION22_BUDGET_PROJECTION"
	jobClaimSync           = "SECTION22_CLAIM_SYNC"
)

func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(jobInvoiceOverdue, s.handleInvoiceOverdue)
	w.Register(jobReceivableReconcile, s.handlePostingReconcile)
	w.Register(jobExpensePosting, s.handleExpensePosting)
	w.Register(jobPostingReconcile, s.handlePostingReconcile)
	w.Register(jobBudgetProjection, s.handleBudgetProjection)
	w.Register(jobClaimSync, s.handleClaimSync)
}
func financeJobPayload(job jobs.Job) (map[string]string, error) {
	var v map[string]string
	if err := json.Unmarshal(job.Payload, &v); err != nil {
		return nil, err
	}
	if v["organizationId"] == "" || v["organizationId"] != job.OrganizationID {
		return nil, jobs.ErrTenantMismatch
	}
	return v, nil
}

// ScheduleMaintenance enqueues the per-organization finance jobs.
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, err := s.o.Store.ActiveOrganizations(ctx)
	if err != nil {
		return err
	}
	bucket := s.o.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)
	for _, org := range orgs {
		for _, typ := range []string{jobInvoiceOverdue, jobReceivableReconcile, jobPostingReconcile, jobBudgetProjection, jobClaimSync} {
			org, typ := org, typ
			if err := s.tx(ctx, func(tx pgx.Tx) error {
				_, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: typ, DedupeKey: typ + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
				return err
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Service) handleInvoiceOverdue(ctx context.Context, job jobs.Job) error {
	if _, err := financeJobPayload(job); err != nil {
		return err
	}
	_, err := s.SweepOverdue(ctx, 200)
	return err
}
func (s *Service) handleExpensePosting(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	org, id := v["organizationId"], v["expenseId"]
	expense, err := s.o.Store.ExpenseState(ctx, nil, org, id, false)
	if err != nil {
		return err
	}
	status, currency, amount, version := expense.Status, expense.Currency, expense.Amount, expense.Version
	if status == "POSTED" || status == "REVERSED" {
		return nil
	}
	if status != "APPROVED" && status != "POSTING" {
		return errors.New("expense is not approved for posting")
	}
	key := "expense:" + id
	req := models.PostingRequest{OrganizationID: org, RecordType: "EXPENSE", RecordID: id, OperationType: "EXPENSE_POST", IdempotencyKey: key, AmountMinor: amount, Currency: currency, ExpectedVersion: version}
	t := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "section22-expense-worker"}
	opID, confirmed, err := s.createPosting(ctx, t, req)
	if err != nil {
		return err
	}
	if !confirmed {
		if _, err = s.executePosting(ctx, t, opID, req); err != nil {
			return err
		}
	}
	return s.o.Store.MarkExpensePosted(ctx, org, id, opID)
}
func (s *Service) handlePostingReconcile(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	if s.o.Wallet == nil {
		return errors.New("posting adapter is not configured")
	}
	// Managed work settlements are excluded: their delivery job owns their
	// reconciliation, because only it can move the instruction itself to
	// POSTED, and only it may resend after a read-back finds nothing.
	all, err := s.o.Store.DuePostingOperations(ctx, v["organizationId"], models.OperationManagedWorkSettlement)
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: v["organizationId"], ActorType: tenant.ActorTypeSystem, ActorID: "section22-reconciliation-worker"}
	for _, x := range all {
		if !s.o.Wallet.Configured(x.OperationType) {
			return errors.New("authoritative posting adapter is not configured for " + x.OperationType)
		}
		req := models.PostingRequest{OrganizationID: t.OrganizationID, RecordType: x.RecordType, RecordID: x.RecordID, OperationType: x.OperationType, IdempotencyKey: x.Key}
		out, err := s.o.Wallet.ReadByKey(ctx, operationMeta(t, x.Key), req)
		if err != nil {
			return err
		}
		if !models.PostingConfirmed(out.Status) {
			return errors.New("authoritative posting is not yet confirmed")
		}
		if err := s.o.Store.ConfirmPostingOperation(ctx, t.OrganizationID, x.ID, out.Reference, out.SourceVersion); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) handleBudgetProjection(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	return s.o.Store.ProjectBudgets(ctx, v["organizationId"])
}
func (s *Service) handleClaimSync(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	return s.o.Store.TouchOpenClaims(ctx, v["organizationId"])
}
