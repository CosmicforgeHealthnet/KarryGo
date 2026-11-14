package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---------------------------------------------------------------------------
// Compensation rules
// ---------------------------------------------------------------------------

// InsertCompensationRule creates a draft rule.
func (r *Repository) InsertCompensationRule(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.CompensationRuleInput, roles, services, cfg json.RawMessage, currency string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_compensation_rules(id,organization_id,name,rule_type,applicable_roles,applicable_services,calculation_configuration,currency,effective_from,effective_until,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, organizationID, in.Name, in.RuleType, roles, services, cfg, currency, in.EffectiveFrom, in.EffectiveUntil, actorID)
	return err
}

// InsertRuleVersion appends a rule version snapshot.
func (r *Repository) InsertRuleVersion(ctx context.Context, q database.Querier, organizationID, ruleID string, version int64, snapshot []byte, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_compensation_rule_versions(id,organization_id,rule_id,rule_version,snapshot,actor_reference) VALUES($1,$2,$3,$4,$5,$6)`, ids.New("crv"), organizationID, ruleID, version, snapshot, actorID)
	return err
}

// GetCompensationRule reads one rule.
func (r *Repository) GetCompensationRule(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_compensation_rules r WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListCompensationRules lists rules, newest first.
func (r *Repository) ListCompensationRules(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_compensation_rules r WHERE organization_id=$1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockCompensationRule reads a rule's status and version FOR UPDATE.
func (r *Repository) LockCompensationRule(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error) {
	var status string
	var version int64
	err := r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_compensation_rules WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, notFoundOnNoRows(err)
}

// UpdateCompensationRule rewrites a draft rule and returns the version.
func (r *Repository) UpdateCompensationRule(ctx context.Context, q database.Querier, organizationID, id string, in models.CompensationRuleInput, cfg json.RawMessage) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_compensation_rules SET name=$3,calculation_configuration=$4,effective_from=$5,effective_until=$6,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, in.Name, cfg, in.EffectiveFrom, in.EffectiveUntil).Scan(&nv)
	return nv, err
}

// TransitionCompensationRule approves or deactivates a rule at the expected
// version and reports whether a row changed.
func (r *Repository) TransitionCompensationRule(ctx context.Context, organizationID, id, next, actorID string, expected int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE fleet_partner_compensation_rules SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$5 AND (($3='APPROVED' AND status IN('DRAFT','PENDING_APPROVAL')) OR ($3='INACTIVE' AND status IN('APPROVED','ACTIVE')))`, organizationID, id, next, actorID, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ---------------------------------------------------------------------------
// Payroll periods
// ---------------------------------------------------------------------------

// InsertPayrollPeriod creates a draft payroll period.
func (r *Repository) InsertPayrollPeriod(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.PayrollPeriodInput, currency string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_payroll_periods(id,organization_id,branch_id,period_start,period_end,currency,created_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7)`, id, organizationID, in.BranchID, in.PeriodStart, in.PeriodEnd, currency, actorID)
	return err
}

// GetPayrollPeriod reads a payroll period with its compensation items.
func (r *Repository) GetPayrollPeriod(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT p.*,COALESCE((SELECT json_agg(row_to_json(i) ORDER BY i.created_at) FROM fleet_partner_provider_compensation_items i WHERE i.organization_id=p.organization_id AND i.payroll_period_id=p.id),'[]'::json) items FROM fleet_partner_payroll_periods p WHERE p.organization_id=$1 AND p.id=$2) x`, organizationID, id))
}

// ListPayrollPeriods lists periods in the permitted branches, latest first.
func (r *Repository) ListPayrollPeriods(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(p) FROM fleet_partner_payroll_periods p WHERE organization_id=$1 AND (COALESCE(cardinality($2::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY period_start DESC`, organizationID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// PayrollState reads a payroll period's state, FOR UPDATE when lock is set;
// a missing period is not found.
func (r *Repository) PayrollState(ctx context.Context, q database.Querier, organizationID, id string, lock bool) (models.PayrollState, error) {
	sql := `SELECT status,currency,COALESCE(branch_id,''),created_by,version,calculation_version,period_start::timestamptz,period_end::timestamptz + interval '1 day' FROM fleet_partner_payroll_periods WHERE organization_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	var s models.PayrollState
	err := r.q(q).QueryRow(ctx, sql, organizationID, id).Scan(&s.Status, &s.Currency, &s.Branch, &s.Creator, &s.Version, &s.CalculationVersion, &s.Start, &s.End)
	return s, notFoundOnNoRows(err)
}

// ActiveCompensationRules lists approved rules in force for a period.
func (r *Repository) ActiveCompensationRules(ctx context.Context, q database.Querier, organizationID, currency string, start, end time.Time) ([]models.ActiveRule, error) {
	rows, err := r.q(q).Query(ctx, `SELECT id,rule_type,version,calculation_configuration FROM fleet_partner_compensation_rules WHERE organization_id=$1 AND currency=$2 AND status IN('APPROVED','ACTIVE') AND effective_from <= $3::date AND (effective_until IS NULL OR effective_until >= $4::date) ORDER BY id`, organizationID, currency, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ActiveRule
	for rows.Next() {
		var rule models.ActiveRule
		var raw []byte
		if err := rows.Scan(&rule.ID, &rule.Type, &rule.Version, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &rule.Config); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// CompletedAssignments lists assignments completed in a period.
func (r *Repository) CompletedAssignments(ctx context.Context, q database.Querier, organizationID, branch string, start, end time.Time) ([]models.SourceAssignment, error) {
	rows, err := r.q(q).Query(ctx, `SELECT provider_membership_id,id,version FROM fleet_partner_private_assignments WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND status='COMPLETED' AND updated_at >= $3 AND updated_at < $4 ORDER BY id`, organizationID, branch, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SourceAssignment
	for rows.Next() {
		var item models.SourceAssignment
		if err := rows.Scan(&item.Member, &item.Source, &item.Version); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// WorkSettledComponents returns the "<assignment>:<rule>" components already
// paid with a completed job's own money (CMP-027).
func (r *Repository) WorkSettledComponents(ctx context.Context, q database.Querier, organizationID string, assignmentIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(assignmentIDs) == 0 {
		return out, nil
	}
	rows, err := r.q(q).Query(ctx, `SELECT source_assignment_id,rule_id FROM fleet_partner_work_settled_components WHERE organization_id=$1 AND settlement_level='WORK' AND source_assignment_id = ANY($2)`, organizationID, assignmentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var assignmentID, ruleID string
		if err := rows.Scan(&assignmentID, &ruleID); err != nil {
			return nil, err
		}
		out[assignmentID+":"+ruleID] = true
	}
	return out, rows.Err()
}

// ActiveMemberships lists the organization's active provider memberships.
func (r *Repository) ActiveMemberships(ctx context.Context, q database.Querier, organizationID string) ([]string, error) {
	rows, err := r.q(q).Query(ctx, `SELECT id FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND status='ACTIVE' ORDER BY id`, organizationID)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// InsertCompensationItem stores an item once per source and rule, returning
// the rows inserted.
func (r *Repository) InsertCompensationItem(ctx context.Context, q database.Querier, item models.CompensationItem) (int64, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_provider_compensation_items(id,organization_id,payroll_period_id,membership_id,source_type,source_reference,rule_id,rule_version,calculation_version,amount_minor,currency,reason_code,explanation,source_inputs) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`, ids.New("pci"), item.OrganizationID, item.PeriodID, item.MembershipID, item.SourceType, item.SourceReference, item.RuleID, item.RuleVersion, item.CalculationVersion, item.AmountMinor, item.Currency, item.ReasonCode, item.Explanation, item.SourceInputs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// MarkPayrollCalculated records a calculation and returns the version.
func (r *Repository) MarkPayrollCalculated(ctx context.Context, q database.Querier, organizationID, id string, calculationVersion int64) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='CALCULATED',calculation_version=$3,calculated_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, calculationVersion).Scan(&nv)
	return nv, err
}

// InsertPayrollHistory appends a payroll transition.
func (r *Repository) InsertPayrollHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, calculationVersion int64, snapshot []byte, actorID, reason string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_payroll_history(id,organization_id,payroll_period_id,previous_status,new_status,calculation_version,snapshot,actor_reference,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("pyh"), organizationID, id, previous, next, calculationVersion, snapshot, actorID, reason)
	return err
}

// ApprovePayrollPeriod approves a period and returns the version.
func (r *Repository) ApprovePayrollPeriod(ctx context.Context, q database.Querier, organizationID, id, actorID string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='APPROVED',approved_by=$3,approved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, actorID).Scan(&nv)
	return nv, err
}

// ApproveCompensationItems approves the items of one calculation.
func (r *Repository) ApproveCompensationItems(ctx context.Context, q database.Querier, organizationID, periodID string, calculationVersion int64) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='APPROVED' WHERE organization_id=$1 AND payroll_period_id=$2 AND calculation_version=$3`, organizationID, periodID, calculationVersion)
	return err
}

// PayrollItems lists a period's items in the given statuses with each
// provider's active sub-wallet.
func (r *Repository) PayrollItems(ctx context.Context, organizationID, periodID string, statuses []string) ([]models.PayrollItem, error) {
	rows, err := r.pool.Query(ctx, `SELECT i.id,i.membership_id,COALESCE(m.provider_id,''),COALESCE(w.wallet_account_id,''),i.reason_code,i.amount_minor
		FROM fleet_partner_provider_compensation_items i
		JOIN fleet_partner_organization_provider_memberships m ON m.id=i.membership_id AND m.organization_id=i.organization_id
		LEFT JOIN fleet_partner_provider_subwallet_links w ON w.provider_membership_id=i.membership_id AND w.organization_id=i.organization_id AND w.status='ACTIVE'
		WHERE i.organization_id=$1 AND i.payroll_period_id=$2 AND i.status = ANY($3) ORDER BY i.id`, organizationID, periodID, statuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.PayrollItem
	for rows.Next() {
		var x models.PayrollItem
		if err := rows.Scan(&x.ID, &x.MembershipID, &x.ProviderID, &x.SubwalletID, &x.Reason, &x.Amount); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// CompensationItemStatus reads one item's status.
func (r *Repository) CompensationItemStatus(ctx context.Context, organizationID, id string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_compensation_items WHERE organization_id=$1 AND id=$2`, organizationID, id).Scan(&status)
	return status, err
}

// MarkCompensationItemPosted marks an approved item posted.
func (r *Repository) MarkCompensationItemPosted(ctx context.Context, organizationID, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='POSTED' WHERE organization_id=$1 AND id=$2 AND status='APPROVED'`, organizationID, id)
	return err
}

// MarkPayrollPartiallyPosted records that some items posted.
func (r *Repository) MarkPayrollPartiallyPosted(ctx context.Context, organizationID, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_payroll_periods SET status='PARTIALLY_POSTED',updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, id)
	return err
}

// MarkPayrollPosted posts an approved or partially posted period and returns
// the version; posted is false when the period was already past that state.
func (r *Repository) MarkPayrollPosted(ctx context.Context, q database.Querier, organizationID, id, lastOperationID string) (version int64, posted bool, err error) {
	err = r.q(q).QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='POSTED',posted_at=now(),wallet_operation_reference=COALESCE((SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),wallet_operation_reference),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('APPROVED','PARTIALLY_POSTED') RETURNING version`, organizationID, id, lastOperationID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return version, err == nil, err
}

// MarkPayrollReversed reverses a posted period at the expected version; a
// changed period returns pgx.ErrNoRows.
func (r *Repository) MarkPayrollReversed(ctx context.Context, q database.Querier, organizationID, id string, expected int64) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='REVERSED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status='POSTED' RETURNING version`, organizationID, id, expected).Scan(&nv)
	return nv, err
}

// ReverseCompensationItems reverses a period's posted items.
func (r *Repository) ReverseCompensationItems(ctx context.Context, q database.Querier, organizationID, periodID string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='REVERSED' WHERE organization_id=$1 AND payroll_period_id=$2 AND status='POSTED'`, organizationID, periodID)
	return err
}

// MembershipExists reports whether a provider membership belongs to the
// organization.
func (r *Repository) MembershipExists(ctx context.Context, organizationID, id string) (bool, error) {
	return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2)`, organizationID, id)
}

// ProviderStatementPeriods returns each payroll period with the member's
// items, latest first.
func (r *Repository) ProviderStatementPeriods(ctx context.Context, organizationID, membershipID string) ([]json.RawMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT json_build_object('period',row_to_json(p),'items',COALESCE(json_agg(row_to_json(i) ORDER BY i.created_at),'[]'::json)) FROM fleet_partner_payroll_periods p JOIN fleet_partner_provider_compensation_items i ON i.payroll_period_id=p.id AND i.organization_id=p.organization_id WHERE p.organization_id=$1 AND i.membership_id=$2 GROUP BY p.id ORDER BY p.period_start DESC`, organizationID, membershipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Subcontractor settlements
// ---------------------------------------------------------------------------

// InsertSettlement creates a draft settlement.
func (r *Repository) InsertSettlement(ctx context.Context, q database.Querier, id, organizationID string, in models.SettlementInput, currency, actorID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_subcontractor_settlements(id,organization_id,supplier_reference,provider_or_carrier_reference,period_start,period_end,currency,amount_minor,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, organizationID, in.SupplierReference, in.ProviderOrCarrierReference, in.PeriodStart, in.PeriodEnd, currency, in.AmountMinor, actorID)
	return err
}

// CompletedAssignmentExists reports whether a job is a completed assignment
// of the organization.
func (r *Repository) CompletedAssignmentExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2 AND status='COMPLETED')`, organizationID, id)
}

// InsertSettlementJob links a job to a settlement; a job settled twice fails.
func (r *Repository) InsertSettlementJob(ctx context.Context, q database.Querier, settlementID, organizationID, job string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_subcontractor_settlement_jobs(settlement_id,organization_id,job_reference) VALUES($1,$2,$3)`, settlementID, organizationID, job)
	return err
}

// GetSettlement reads a settlement with its job references.
func (r *Repository) GetSettlement(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT s.*,COALESCE((SELECT json_agg(j.job_reference ORDER BY j.job_reference) FROM fleet_partner_subcontractor_settlement_jobs j WHERE j.organization_id=s.organization_id AND j.settlement_id=s.id),'[]'::json) job_references FROM fleet_partner_subcontractor_settlements s WHERE s.organization_id=$1 AND s.id=$2) x`, organizationID, id))
}

// ListSettlements lists settlements, newest first.
func (r *Repository) ListSettlements(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(s) FROM fleet_partner_subcontractor_settlements s WHERE organization_id=$1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// ApproveSettlement approves a settlement at the expected version and
// reports whether a row changed.
func (r *Repository) ApproveSettlement(ctx context.Context, organizationID, id, actorID string, expected int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE fleet_partner_subcontractor_settlements SET status='APPROVED',approved_by=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$4 AND status IN('DRAFT','CALCULATED','PENDING_APPROVAL')`, organizationID, id, actorID, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// MarkSettlementPosted records the confirmed posting of an approved
// settlement.
func (r *Repository) MarkSettlementPosted(ctx context.Context, organizationID, id, operationID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_subcontractor_settlements SET status='POSTED',posting_reference=(SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='APPROVED'`, organizationID, id, operationID)
	return err
}

// ---------------------------------------------------------------------------
// Posting operations
// ---------------------------------------------------------------------------

// InsertPostingOperation records a posting operation once per key and
// reports whether it was inserted.
func (r *Repository) InsertPostingOperation(ctx context.Context, id string, req models.PostingRequest, fingerprint string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO fleet_partner_financial_posting_operations(id,organization_id,record_type,record_id,operation_type,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id,operation_type,idempotency_key) DO NOTHING`, id, req.OrganizationID, req.RecordType, req.RecordID, req.OperationType, req.IdempotencyKey, fingerprint)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// PostingOperationByKey reads an operation's ID, status and fingerprint.
func (r *Repository) PostingOperationByKey(ctx context.Context, organizationID, operationType, key string) (id, status, fingerprint string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id,status,request_fingerprint FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND operation_type=$2 AND idempotency_key=$3`, organizationID, operationType, key).Scan(&id, &status, &fingerprint)
	return id, status, fingerprint, err
}

// RecordPostingAttempt appends one attempt row and moves the operation to the
// status that attempt established. An operation left OUTCOME_UNKNOWN or
// RETRY_SCHEDULED is rescheduled thirty seconds out.
func (r *Repository) RecordPostingAttempt(ctx context.Context, q database.Querier, a models.PostingAttemptRecord) error {
	var attempt int
	if e := r.q(q).QueryRow(ctx, `SELECT attempts+1 FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, a.OrganizationID, a.OperationID).Scan(&attempt); e != nil {
		return e
	}
	if _, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_financial_posting_attempts(id,organization_id,operation_id,attempt_number,outcome,external_reference,error_code,started_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())`, ids.New("fpa"), a.OrganizationID, a.OperationID, attempt, a.Outcome, a.Reference, a.ErrorCode, a.Started); e != nil {
		return e
	}
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status=$3,external_operation_reference=$4,source_version=$5,attempts=$6,last_error_code=$7,next_attempt_at=CASE WHEN $3 IN('OUTCOME_UNKNOWN','RETRY_SCHEDULED') THEN now()+interval '30 seconds' ELSE next_attempt_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, a.OrganizationID, a.OperationID, a.Status, a.Reference, a.SourceVersion, attempt, a.ErrorCode)
	return e
}

// PostingState reads an operation's state.
func (r *Repository) PostingState(ctx context.Context, organizationID, operationID string) (models.PostingOperationState, error) {
	var st models.PostingOperationState
	err := r.pool.QueryRow(ctx, `SELECT status,external_operation_reference,last_error_code,source_version FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND id=$2`,
		organizationID, operationID).Scan(&st.Status, &st.Reference, &st.LastErrorCode, &st.SourceVersion)
	return st, err
}

// MarkPostingFailed records that Wallet refused the operation as stated.
func (r *Repository) MarkPostingFailed(ctx context.Context, organizationID, operationID, code string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status='FAILED',last_error_code=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status<>'CONFIRMED'`,
		organizationID, operationID, code)
	return err
}

// DuePostingOperations lists operations awaiting reconciliation, excluding
// one operation type.
func (r *Repository) DuePostingOperations(ctx context.Context, organizationID, excludeType string) ([]models.DuePosting, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,record_type,record_id,operation_type,idempotency_key FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND status IN('OUTCOME_UNKNOWN','RETRY_SCHEDULED') AND operation_type<>$2 AND next_attempt_at<=now() ORDER BY next_attempt_at LIMIT 50`, organizationID, excludeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DuePosting
	for rows.Next() {
		var x models.DuePosting
		if err := rows.Scan(&x.ID, &x.RecordType, &x.RecordID, &x.OperationType, &x.Key); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// ConfirmPostingOperation records an authoritative confirmation read back.
func (r *Repository) ConfirmPostingOperation(ctx context.Context, organizationID, id, reference string, sourceVersion int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status='CONFIRMED',external_operation_reference=$3,source_version=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, id, reference, sourceVersion)
	return err
}
