package financeops

import (
	"context"
	"encoding/json"
	"errors"
	"time"

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
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE' ORDER BY id`)
	if err != nil {
		return err
	}
	var orgs []string
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			rows.Close()
			return err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	bucket := s.o.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)
	for _, org := range orgs {
		for _, typ := range []string{jobInvoiceOverdue, jobReceivableReconcile, jobPostingReconcile, jobBudgetProjection, jobClaimSync} {
			if _, err := s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: typ, DedupeKey: typ + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 8}); err != nil {
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
	var status, currency string
	var amount, version int64
	if err := s.o.Pool.QueryRow(ctx, `SELECT status,currency,amount_minor,version FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2`, org, id).Scan(&status, &currency, &amount, &version); err != nil {
		return err
	}
	if status == "POSTED" || status == "REVERSED" {
		return nil
	}
	if status != "APPROVED" && status != "POSTING" {
		return errors.New("expense is not approved for posting")
	}
	key := "expense:" + id
	req := PostingRequest{OrganizationID: org, RecordType: "EXPENSE", RecordID: id, OperationType: "EXPENSE_POST", IdempotencyKey: key, AmountMinor: amount, Currency: currency, ExpectedVersion: version}
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
	_, err = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_expenses SET status='POSTED',posting_reference=(SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('APPROVED','POSTING')`, org, id, opID)
	return err
}
func (s *Service) handlePostingReconcile(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	if s.o.Wallet == nil {
		return errors.New("posting adapter is not configured")
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT id,record_type,record_id,operation_type,idempotency_key FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND status IN('OUTCOME_UNKNOWN','RETRY_SCHEDULED') AND next_attempt_at<=now() ORDER BY next_attempt_at LIMIT 50`, v["organizationId"])
	if err != nil {
		return err
	}
	type op struct{ id, recordType, recordID, operationType, key string }
	var all []op
	for rows.Next() {
		var x op
		if err := rows.Scan(&x.id, &x.recordType, &x.recordID, &x.operationType, &x.key); err != nil {
			rows.Close()
			return err
		}
		all = append(all, x)
	}
	rows.Close()
	t := tenant.Context{OrganizationID: v["organizationId"], ActorType: tenant.ActorTypeSystem, ActorID: "section22-reconciliation-worker"}
	for _, x := range all {
		if !s.o.Wallet.Configured(x.operationType) {
			return errors.New("authoritative posting adapter is not configured for " + x.operationType)
		}
		req := PostingRequest{OrganizationID: t.OrganizationID, RecordType: x.recordType, RecordID: x.recordID, OperationType: x.operationType, IdempotencyKey: x.key}
		out, err := s.o.Wallet.ReadByKey(ctx, operationMeta(t, x.key), req)
		if err != nil {
			return err
		}
		if !postingConfirmed(out.Status) {
			return errors.New("authoritative posting is not yet confirmed")
		}
		if _, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status='CONFIRMED',external_operation_reference=$3,source_version=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, x.id, out.Reference, out.SourceVersion); err != nil {
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
	_, err = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_budgets b SET committed_amount_projection=x.committed,actual_amount_projection=x.actual,updated_at=now() FROM(SELECT b2.id,COALESCE(sum(e.amount_minor)FILTER(WHERE e.status IN('APPROVED','POSTING')),0)::bigint committed,COALESCE(sum(e.amount_minor)FILTER(WHERE e.status='POSTED'),0)::bigint actual FROM fleet_partner_budgets b2 LEFT JOIN fleet_partner_expenses e ON e.organization_id=b2.organization_id AND e.cost_centre=b2.cost_centre AND e.category=b2.category AND e.currency=b2.currency AND e.created_at::date BETWEEN b2.period_start AND b2.period_end WHERE b2.organization_id=$1 AND b2.status IN('APPROVED','ACTIVE') GROUP BY b2.id)x WHERE b.organization_id=$1 AND b.id=x.id`, v["organizationId"])
	return err
}
func (s *Service) handleClaimSync(ctx context.Context, job jobs.Job) error {
	v, err := financeJobPayload(job)
	if err != nil {
		return err
	}
	_, err = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_claims SET updated_at=updated_at WHERE organization_id=$1 AND status IN('SUBMITTED','UNDER_REVIEW','MORE_INFORMATION_REQUIRED')`, v["organizationId"])
	return err
}
