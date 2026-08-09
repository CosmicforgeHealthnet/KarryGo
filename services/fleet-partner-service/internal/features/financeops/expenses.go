package financeops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) CreateExpense(ctx context.Context, t tenant.Context, in ExpenseInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 || strings.TrimSpace(in.Category) == "" || strings.TrimSpace(in.CostCentre) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, apperrors.Validation("Category, cost centre, description and a positive minor-unit amount are required.", nil)
	}
	if err = s.validateMedia(ctx, t, in.ReceiptMediaID); err != nil {
		return nil, err
	}
	id := ids.New("exp")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_expenses(id,organization_id,branch_id,category,cost_centre,asset_id,provider_membership_id,order_id,shipment_id,work_order_id,incident_id,amount_minor,currency,receipt_media_id,description,submitted_by) VALUES($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12,$13,$14,$15,$16)`, id, t.OrganizationID, in.BranchID, strings.ToUpper(in.Category), in.CostCentre, in.AssetID, in.ProviderMembershipID, in.OrderID, in.ShipmentID, in.WorkOrderID, in.IncidentID, in.AmountMinor, currency, in.ReceiptMediaID, in.Description, t.ActorID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.created.v1", "expense", id, 1, map[string]any{"expenseId": id, "currency": currency, "amountMinor": in.AmountMinor, "category": strings.ToUpper(in.Category)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}
func (s *Service) GetExpense(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, false); err != nil {
		return nil, err
	}
	v, err := jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(e) FROM fleet_partner_expenses e WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}
func (s *Service) ListExpenses(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(e) FROM fleet_partner_expenses e WHERE organization_id=$1 AND (cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateExpense(ctx context.Context, t tenant.Context, id string, in ExpenseInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 {
		return nil, apperrors.Validation("Expense amount must be positive.", nil)
	}
	if err = s.validateMedia(ctx, t, in.ReceiptMediaID); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status, branch string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,COALESCE(branch_id,''),version FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &branch, &version); err != nil {
			return notFoundOnNoRows(err)
		}
		if !branchAllowed(t, branch) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Expense version changed.", nil)
		}
		if status != "DRAFT" {
			return apperrors.Conflict("Approved or submitted expenses are immutable.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_expenses SET category=$3,cost_centre=$4,amount_minor=$5,currency=$6,receipt_media_id=$7,description=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, strings.ToUpper(in.Category), in.CostCentre, in.AmountMinor, currency, in.ReceiptMediaID, in.Description).Scan(&nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.updated.v1", "expense", id, nv, map[string]any{"expenseId": id, "currency": currency, "amountMinor": in.AmountMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

func (s *Service) ExpenseCommand(ctx context.Context, t tenant.Context, id, command, reason string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if command == "reverse" {
		return s.reverseExpense(ctx, t, id, reason, expected)
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, submitter, currency string
		var version, amount int64
		if err := tx.QueryRow(ctx, `SELECT status,submitted_by,currency,version,amount_minor FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &submitter, &currency, &version, &amount); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != expected {
			return apperrors.Conflict("Expense version changed.", nil)
		}
		next := status
		switch command {
		case "submit":
			if status != "DRAFT" {
				return apperrors.Conflict("Only a draft expense may be submitted.", nil)
			}
			next = "SUBMITTED"
		case "approve":
			if status != "SUBMITTED" && status != "PENDING_APPROVAL" {
				return apperrors.Conflict("Expense cannot be approved from its current state.", nil)
			}
			if submitter == t.ActorID {
				return apperrors.Forbidden("Expense submitter cannot approve their own expense.", nil)
			}
			next = "APPROVED"
		case "reject":
			if status != "SUBMITTED" && status != "PENDING_APPROVAL" {
				return apperrors.Conflict("Expense cannot be rejected from its current state.", nil)
			}
			if strings.TrimSpace(reason) == "" {
				return apperrors.Validation("Rejection reason is required.", nil)
			}
			next = "REJECTED"
		default:
			return apperrors.Validation("Unsupported expense command.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_expenses SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,approved_at=CASE WHEN $3='APPROVED' THEN now() ELSE approved_at END,rejected_at=CASE WHEN $3='REJECTED' THEN now() ELSE rejected_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, next, t.ActorID).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"expenseId": id, "status": next, "currency": currency, "amountMinor": amount, "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_expense_history(id,organization_id,expense_id,previous_status,new_status,snapshot,actor_reference,reason,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("exh"), t.OrganizationID, id, status, next, snapshot, t.ActorID, reason, nv); err != nil {
			return err
		}
		if command == "approve" && s.o.Flags.Posting && s.o.Flags.Workers {
			if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: jobExpensePosting, DedupeKey: id, Payload: map[string]string{"organizationId": t.OrganizationID, "expenseId": id}, MaxAttempts: 8}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "expense."+command+"d.v1", "expense", id, nv, map[string]any{"expenseId": id, "status": next, "currency": currency, "amountMinor": amount})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

func (s *Service) reverseExpense(ctx context.Context, t tenant.Context, id, reason string, expected int64) (map[string]any, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperrors.Validation("Reversal reason is required.", nil)
	}
	var status, currency string
	var amount, version int64
	if err := s.o.Pool.QueryRow(ctx, `SELECT status,currency,amount_minor,version FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&status, &currency, &amount, &version); err != nil {
		return nil, notFoundOnNoRows(err)
	}
	if version != expected || (status != "APPROVED" && status != "POSTED") {
		return nil, apperrors.Conflict("Expense changed or is not approved or posted.", nil)
	}
	postingOperationID := ""
	if status == "POSTED" {
		payload, _ := json.Marshal(map[string]any{"reason": reason, "originalPostingReference": id})
		req := PostingRequest{OrganizationID: t.OrganizationID, RecordType: "EXPENSE", RecordID: id, OperationType: "EXPENSE_REVERSAL", IdempotencyKey: fmt.Sprintf("expense-reversal:%s:%d", id, expected), AmountMinor: amount, Currency: currency, ExpectedVersion: expected, Payload: payload}
		opID, confirmed, err := s.createPosting(ctx, t, req)
		if err != nil {
			return nil, err
		}
		postingOperationID = opID
		if !confirmed {
			if _, err = s.executePosting(ctx, t, opID, req); err != nil {
				return nil, apperrors.Unavailable("Expense reversal requires authoritative reconciliation.", err)
			}
		}
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var currentStatus string
		var currentVersion int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&currentStatus, &currentVersion); err != nil {
			return notFoundOnNoRows(err)
		}
		if currentVersion != expected || currentStatus != status {
			return apperrors.Conflict("Expense changed while its reversal was being confirmed.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_expenses SET status='REVERSED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id).Scan(&nv); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_expense_reversals(id,organization_id,expense_id,amount_minor,currency,reason,posting_operation_id,created_by) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`, ids.New("exr"), t.OrganizationID, id, amount, currency, reason, postingOperationID, t.ActorID); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"expenseId": id, "status": "REVERSED", "currency": currency, "amountMinor": amount, "version": nv, "postingOperationId": postingOperationID})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_expense_history(id,organization_id,expense_id,previous_status,new_status,snapshot,actor_reference,reason,version) VALUES($1,$2,$3,$4,'REVERSED',$5,$6,$7,$8)`, ids.New("exh"), t.OrganizationID, id, status, snapshot, t.ActorID, reason, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.reversed.v1", "expense", id, nv, map[string]any{"expenseId": id, "currency": currency, "amountMinor": amount, "postingOperationId": postingOperationID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

func (s *Service) CreateBudget(ctx context.Context, t tenant.Context, in BudgetInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.PeriodStart.IsZero() || in.PeriodEnd.Before(in.PeriodStart) || in.ApprovedAmountMinor < 0 || in.CostCentre == "" || in.Category == "" {
		return nil, apperrors.Validation("Budget period, scope and amount are invalid.", nil)
	}
	id := ids.New("bdg")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_budgets(id,organization_id,cost_centre,category,period_start,period_end,currency,approved_amount_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, t.OrganizationID, in.CostCentre, strings.ToUpper(in.Category), in.PeriodStart, in.PeriodEnd, currency, in.ApprovedAmountMinor)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "budget.created.v1", "budget", id, 1, map[string]any{"budgetId": id, "currency": currency, "approvedAmountMinor": in.ApprovedAmountMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetBudget(ctx, t, id)
}
func (s *Service) GetBudget(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(b) FROM fleet_partner_budgets b WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListBudgets(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(b) FROM fleet_partner_budgets b WHERE organization_id=$1 ORDER BY period_start DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateBudget(ctx context.Context, t tenant.Context, id string, in BudgetInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	tag, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_budgets SET cost_centre=$3,category=$4,period_start=$5,period_end=$6,currency=$7,approved_amount_minor=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$9 AND status IN('DRAFT','PENDING_APPROVAL')`, t.OrganizationID, id, in.CostCentre, strings.ToUpper(in.Category), in.PeriodStart, in.PeriodEnd, currency, in.ApprovedAmountMinor, in.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apperrors.Conflict("Budget changed or is immutable.", nil)
	}
	return s.GetBudget(ctx, t, id)
}
func (s *Service) BudgetCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_budgets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != expected {
			return apperrors.Conflict("Budget version changed.", nil)
		}
		next := ""
		if command == "approve" && (status == "DRAFT" || status == "PENDING_APPROVAL") {
			next = "APPROVED"
		} else if command == "close" && (status == "APPROVED" || status == "ACTIVE") {
			next = "CLOSED"
		} else {
			return apperrors.Conflict("Budget command is invalid for its current state.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_budgets SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,approved_at=CASE WHEN $3='APPROVED' THEN now() ELSE approved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, next, t.ActorID).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"budgetId": id, "status": next, "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_budget_history(id,organization_id,budget_id,previous_status,new_status,snapshot,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("bdh"), t.OrganizationID, id, status, next, snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "budget."+command+"d.v1", "budget", id, nv, map[string]any{"budgetId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetBudget(ctx, t, id)
}

func (s *Service) CreateClaim(ctx context.Context, t tenant.Context, in ClaimInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, true); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	in.ClaimType = strings.ToUpper(in.ClaimType)
	valid := map[string]bool{"INSURANCE": true, "CARGO": true, "VEHICLE": true, "PROPERTY": true, "PERSONAL_INJURY": true, "SECURITY": true, "OTHER": true}
	if !valid[in.ClaimType] {
		return nil, apperrors.Validation("Claim type is invalid.", nil)
	}
	for _, fileID := range in.EvidenceMediaIDs {
		if err = s.validateMedia(ctx, t, fileID); err != nil {
			return nil, err
		}
	}
	if in.Status == "" {
		in.Status = "DRAFT"
	}
	evidence, _ := json.Marshal(in.EvidenceMediaIDs)
	id := ids.New("clm")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_claims(id,organization_id,claim_type,incident_id,asset_id,shipment_id,package_id,policy_reference,amount_reference_minor,currency,status,external_claim_reference,evidence_media_ids,submitted_at,approved_amount_minor,settlement_reference,owner_user_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,CASE WHEN $11='SUBMITTED' THEN now() END,$14,$15,$16)`, id, t.OrganizationID, in.ClaimType, in.IncidentID, in.AssetID, in.ShipmentID, in.PackageID, in.PolicyReference, in.AmountReferenceMinor, currency, strings.ToUpper(in.Status), in.ExternalClaimReference, evidence, in.ApprovedAmountMinor, in.SettlementReference, in.OwnerUserID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "claim.created.v1", "claim", id, 1, map[string]any{"claimId": id, "claimType": in.ClaimType, "currency": currency, "amountReferenceMinor": in.AmountReferenceMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetClaim(ctx, t, id)
}
func (s *Service) GetClaim(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(c) FROM fleet_partner_claims c WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListClaims(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(c) FROM fleet_partner_claims c WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateClaim(ctx context.Context, t tenant.Context, id string, in ClaimInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	for _, fileID := range in.EvidenceMediaIDs {
		if err := s.validateMedia(ctx, t, fileID); err != nil {
			return nil, err
		}
	}
	evidence, _ := json.Marshal(in.EvidenceMediaIDs)
	status := strings.ToUpper(in.Status)
	if status == "" {
		status = "DRAFT"
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var old string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_claims WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&old, &version); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Claim version changed.", nil)
		}
		if old == "SETTLED" || old == "CLOSED" || old == "CANCELLED" {
			return apperrors.Conflict("Final claim records are immutable.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_claims SET status=$3,external_claim_reference=$4,evidence_media_ids=$5,approved_amount_minor=$6,settlement_reference=$7,owner_user_id=$8,submitted_at=CASE WHEN $3='SUBMITTED' AND submitted_at IS NULL THEN now() ELSE submitted_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, status, in.ExternalClaimReference, evidence, in.ApprovedAmountMinor, in.SettlementReference, in.OwnerUserID).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"claimId": id, "status": status, "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_claim_history(id,organization_id,claim_id,previous_status,new_status,snapshot,actor_reference) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("clh"), t.OrganizationID, id, old, status, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "claim.updated.v1", "claim", id, nv, map[string]any{"claimId": id, "status": status, "externalClaimReference": in.ExternalClaimReference})
	})
	if err != nil {
		return nil, err
	}
	return s.GetClaim(ctx, t, id)
}
