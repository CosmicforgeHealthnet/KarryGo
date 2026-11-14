package repositories

import (
	"context"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---------------------------------------------------------------------------
// Expenses
// ---------------------------------------------------------------------------

// InsertExpense creates a draft expense.
func (r *Repository) InsertExpense(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.ExpenseInput, currency string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_expenses(id,organization_id,branch_id,category,cost_centre,asset_id,provider_membership_id,order_id,shipment_id,work_order_id,incident_id,amount_minor,currency,receipt_media_id,description,submitted_by) VALUES($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12,$13,$14,$15,$16)`, id, organizationID, in.BranchID, strings.ToUpper(in.Category), in.CostCentre, in.AssetID, in.ProviderMembershipID, in.OrderID, in.ShipmentID, in.WorkOrderID, in.IncidentID, in.AmountMinor, currency, in.ReceiptMediaID, in.Description, actorID)
	return err
}

// GetExpense reads one expense.
func (r *Repository) GetExpense(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(e) FROM fleet_partner_expenses e WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListExpenses lists expenses in the permitted branches, newest first.
func (r *Repository) ListExpenses(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(e) FROM fleet_partner_expenses e WHERE organization_id=$1 AND (COALESCE(cardinality($2::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, organizationID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// ExpenseState reads an expense's state, FOR UPDATE when lock is set; a
// missing expense is not found.
func (r *Repository) ExpenseState(ctx context.Context, q database.Querier, organizationID, id string, lock bool) (models.ExpenseState, error) {
	sql := `SELECT status,COALESCE(branch_id,''),submitted_by,currency,version,amount_minor FROM fleet_partner_expenses WHERE organization_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	var s models.ExpenseState
	err := r.q(q).QueryRow(ctx, sql, organizationID, id).Scan(&s.Status, &s.Branch, &s.Submitter, &s.Currency, &s.Version, &s.Amount)
	return s, notFoundOnNoRows(err)
}

// UpdateExpense rewrites a draft expense and returns the version.
func (r *Repository) UpdateExpense(ctx context.Context, q database.Querier, organizationID, id string, in models.ExpenseInput, currency string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_expenses SET category=$3,cost_centre=$4,amount_minor=$5,currency=$6,receipt_media_id=$7,description=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, strings.ToUpper(in.Category), in.CostCentre, in.AmountMinor, currency, in.ReceiptMediaID, in.Description).Scan(&nv)
	return nv, err
}

// SetExpenseStatus moves an expense to status and returns the version.
func (r *Repository) SetExpenseStatus(ctx context.Context, q database.Querier, organizationID, id, status, actorID string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_expenses SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,approved_at=CASE WHEN $3='APPROVED' THEN now() ELSE approved_at END,rejected_at=CASE WHEN $3='REJECTED' THEN now() ELSE rejected_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, actorID).Scan(&nv)
	return nv, err
}

// MarkExpenseReversed reverses an expense and returns the version.
func (r *Repository) MarkExpenseReversed(ctx context.Context, q database.Querier, organizationID, id string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_expenses SET status='REVERSED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id).Scan(&nv)
	return nv, err
}

// MarkExpensePosted records the confirmed posting of an approved expense.
func (r *Repository) MarkExpensePosted(ctx context.Context, organizationID, id, operationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_expenses SET status='POSTED',posting_reference=(SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('APPROVED','POSTING')`, organizationID, id, operationID)
	return err
}

// InsertExpenseHistory appends an expense transition.
func (r *Repository) InsertExpenseHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID, reason string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_expense_history(id,organization_id,expense_id,previous_status,new_status,snapshot,actor_reference,reason,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("exh"), organizationID, id, previous, next, snapshot, actorID, reason, version)
	return err
}

// InsertExpenseReversal records an expense reversal.
func (r *Repository) InsertExpenseReversal(ctx context.Context, q database.Querier, organizationID, id string, amount int64, currency, reason, postingOperationID, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_expense_reversals(id,organization_id,expense_id,amount_minor,currency,reason,posting_operation_id,created_by) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`, ids.New("exr"), organizationID, id, amount, currency, reason, postingOperationID, actorID)
	return err
}

// ---------------------------------------------------------------------------
// Budgets
// ---------------------------------------------------------------------------

// InsertBudget creates a draft budget.
func (r *Repository) InsertBudget(ctx context.Context, q database.Querier, id, organizationID string, in models.BudgetInput, currency string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_budgets(id,organization_id,cost_centre,category,period_start,period_end,currency,approved_amount_minor) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, organizationID, in.CostCentre, strings.ToUpper(in.Category), in.PeriodStart, in.PeriodEnd, currency, in.ApprovedAmountMinor)
	return err
}

// GetBudget reads one budget.
func (r *Repository) GetBudget(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(b) FROM fleet_partner_budgets b WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListBudgets lists budgets by period, latest first.
func (r *Repository) ListBudgets(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(b) FROM fleet_partner_budgets b WHERE organization_id=$1 ORDER BY period_start DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// UpdateBudget rewrites a draft budget at the expected version and reports
// whether a row changed.
func (r *Repository) UpdateBudget(ctx context.Context, organizationID, id string, in models.BudgetInput, currency string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE fleet_partner_budgets SET cost_centre=$3,category=$4,period_start=$5,period_end=$6,currency=$7,approved_amount_minor=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$9 AND status IN('DRAFT','PENDING_APPROVAL')`, organizationID, id, in.CostCentre, strings.ToUpper(in.Category), in.PeriodStart, in.PeriodEnd, currency, in.ApprovedAmountMinor, in.ExpectedVersion)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// LockBudget reads a budget's status and version FOR UPDATE.
func (r *Repository) LockBudget(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error) {
	var status string
	var version int64
	err := r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_budgets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, notFoundOnNoRows(err)
}

// SetBudgetStatus moves a budget to status and returns the version.
func (r *Repository) SetBudgetStatus(ctx context.Context, q database.Querier, organizationID, id, status, actorID string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_budgets SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,approved_at=CASE WHEN $3='APPROVED' THEN now() ELSE approved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, actorID).Scan(&nv)
	return nv, err
}

// InsertBudgetHistory appends a budget transition.
func (r *Repository) InsertBudgetHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_budget_history(id,organization_id,budget_id,previous_status,new_status,snapshot,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("bdh"), organizationID, id, previous, next, snapshot, actorID, version)
	return err
}

// ProjectBudgets recomputes committed and actual spend of active budgets.
func (r *Repository) ProjectBudgets(ctx context.Context, organizationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_budgets b SET committed_amount_projection=x.committed,actual_amount_projection=x.actual,updated_at=now() FROM(SELECT b2.id,COALESCE(sum(e.amount_minor)FILTER(WHERE e.status IN('APPROVED','POSTING')),0)::bigint committed,COALESCE(sum(e.amount_minor)FILTER(WHERE e.status='POSTED'),0)::bigint actual FROM fleet_partner_budgets b2 LEFT JOIN fleet_partner_expenses e ON e.organization_id=b2.organization_id AND e.cost_centre=b2.cost_centre AND e.category=b2.category AND e.currency=b2.currency AND e.created_at::date BETWEEN b2.period_start AND b2.period_end WHERE b2.organization_id=$1 AND b2.status IN('APPROVED','ACTIVE') GROUP BY b2.id)x WHERE b.organization_id=$1 AND b.id=x.id`, organizationID)
	return err
}

// ---------------------------------------------------------------------------
// Claims
// ---------------------------------------------------------------------------

// InsertClaim creates a claim record.
func (r *Repository) InsertClaim(ctx context.Context, q database.Querier, id, organizationID string, in models.ClaimInput, currency, status string, evidence []byte) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_claims(id,organization_id,claim_type,incident_id,asset_id,shipment_id,package_id,policy_reference,amount_reference_minor,currency,status,external_claim_reference,evidence_media_ids,submitted_at,approved_amount_minor,settlement_reference,owner_user_id) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,CASE WHEN $11='SUBMITTED' THEN now() END,$14,$15,$16)`, id, organizationID, in.ClaimType, in.IncidentID, in.AssetID, in.ShipmentID, in.PackageID, in.PolicyReference, in.AmountReferenceMinor, currency, status, in.ExternalClaimReference, evidence, in.ApprovedAmountMinor, in.SettlementReference, in.OwnerUserID)
	return err
}

// GetClaim reads one claim.
func (r *Repository) GetClaim(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(c) FROM fleet_partner_claims c WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListClaims lists claims, newest first.
func (r *Repository) ListClaims(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(c) FROM fleet_partner_claims c WHERE organization_id=$1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockClaim reads a claim's status and version FOR UPDATE.
func (r *Repository) LockClaim(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error) {
	var status string
	var version int64
	err := r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_claims WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, notFoundOnNoRows(err)
}

// UpdateClaim rewrites a claim and returns the version.
func (r *Repository) UpdateClaim(ctx context.Context, q database.Querier, organizationID, id, status string, in models.ClaimInput, evidence []byte) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_claims SET status=$3,external_claim_reference=$4,evidence_media_ids=$5,approved_amount_minor=$6,settlement_reference=$7,owner_user_id=$8,submitted_at=CASE WHEN $3='SUBMITTED' AND submitted_at IS NULL THEN now() ELSE submitted_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, in.ExternalClaimReference, evidence, in.ApprovedAmountMinor, in.SettlementReference, in.OwnerUserID).Scan(&nv)
	return nv, err
}

// InsertClaimHistory appends a claim transition.
func (r *Repository) InsertClaimHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_claim_history(id,organization_id,claim_id,previous_status,new_status,snapshot,actor_reference) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("clh"), organizationID, id, previous, next, snapshot, actorID)
	return err
}

// TouchOpenClaims refreshes open claims for the claim sync pass.
func (r *Repository) TouchOpenClaims(ctx context.Context, organizationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_claims SET updated_at=updated_at WHERE organization_id=$1 AND status IN('SUBMITTED','UNDER_REVIEW','MORE_INFORMATION_REQUIRED')`, organizationID)
	return err
}
