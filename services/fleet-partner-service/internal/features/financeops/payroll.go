package financeops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type ruleCalculation struct {
	AmountMinor   int64  `json:"amountMinor"`
	ApprovedBasis string `json:"approvedBasis"`
}

func (s *Service) CreateCompensationRule(ctx context.Context, t tenant.Context, in CompensationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	in.RuleType = strings.ToUpper(in.RuleType)
	valid := map[string]bool{"SALARY": true, "PER_JOB": true, "PER_SHIFT": true, "COMMISSION": true, "ALLOWANCE": true, "OVERTIME": true, "BONUS": true, "MANUAL_ADJUSTMENT": true, "DEDUCTION": true}
	if !valid[in.RuleType] || strings.TrimSpace(in.Name) == "" || in.EffectiveFrom.IsZero() {
		return nil, apperrors.Validation("Compensation rule is invalid.", nil)
	}
	cfg := rawOr(in.CalculationConfiguration, "{}")
	var calc ruleCalculation
	if err = json.Unmarshal(cfg, &calc); err != nil || calc.AmountMinor == 0 {
		return nil, apperrors.Validation("A deterministic non-zero amountMinor is required.", nil)
	}
	if in.RuleType == "DEDUCTION" && strings.TrimSpace(calc.ApprovedBasis) == "" {
		return nil, apperrors.Validation("Deduction rules require an approved basis.", nil)
	}
	roles := rawOr(in.ApplicableRoles, "[]")
	services := rawOr(in.ApplicableServices, "[]")
	id := ids.New("cpr")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_compensation_rules(id,organization_id,name,rule_type,applicable_roles,applicable_services,calculation_configuration,currency,effective_from,effective_until,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, t.OrganizationID, in.Name, in.RuleType, roles, services, cfg, currency, in.EffectiveFrom, in.EffectiveUntil, t.ActorID)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"id": id, "name": in.Name, "ruleType": in.RuleType, "configuration": json.RawMessage(cfg), "currency": currency, "version": 1})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_compensation_rule_versions(id,organization_id,rule_id,rule_version,snapshot,actor_reference) VALUES($1,$2,$3,1,$4,$5)`, ids.New("crv"), t.OrganizationID, id, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "compensation_rule.created.v1", "compensation_rule", id, 1, map[string]any{"ruleId": id, "ruleType": in.RuleType, "currency": currency})
	})
	if err != nil {
		return nil, err
	}
	return s.GetCompensationRule(ctx, t, id)
}
func (s *Service) GetCompensationRule(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_compensation_rules r WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListCompensationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_compensation_rules r WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateCompensationRule(ctx context.Context, t tenant.Context, id string, in CompensationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	cfg := rawOr(in.CalculationConfiguration, "{}")
	var calc ruleCalculation
	if err := json.Unmarshal(cfg, &calc); err != nil || calc.AmountMinor == 0 {
		return nil, apperrors.Validation("A deterministic non-zero amountMinor is required.", nil)
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_compensation_rules WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Compensation rule version changed.", nil)
		}
		if status == "APPROVED" || status == "ACTIVE" || status == "ARCHIVED" {
			return apperrors.Conflict("Approved compensation rules are immutable; create a replacement version.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_compensation_rules SET name=$3,calculation_configuration=$4,effective_from=$5,effective_until=$6,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, in.Name, cfg, in.EffectiveFrom, in.EffectiveUntil).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"id": id, "name": in.Name, "configuration": json.RawMessage(cfg), "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_compensation_rule_versions(id,organization_id,rule_id,rule_version,snapshot,actor_reference) VALUES($1,$2,$3,$4,$5,$6)`, ids.New("crv"), t.OrganizationID, id, nv, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "compensation_rule.updated.v1", "compensation_rule", id, nv, map[string]any{"ruleId": id, "version": nv})
	})
	if err != nil {
		return nil, err
	}
	return s.GetCompensationRule(ctx, t, id)
}
func (s *Service) CompensationRuleCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	next := "APPROVED"
	if command == "deactivate" {
		next = "INACTIVE"
	}
	tag, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_compensation_rules SET status=$3,approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$5 AND (($3='APPROVED' AND status IN('DRAFT','PENDING_APPROVAL')) OR ($3='INACTIVE' AND status IN('APPROVED','ACTIVE')))`, t.OrganizationID, id, next, t.ActorID, expected)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apperrors.Conflict("Compensation rule changed or command is invalid.", nil)
	}
	return s.GetCompensationRule(ctx, t, id)
}

func (s *Service) CreatePayrollPeriod(ctx context.Context, t tenant.Context, in PayrollPeriodInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.PeriodStart.IsZero() || in.PeriodEnd.Before(in.PeriodStart) {
		return nil, apperrors.Validation("Payroll period is invalid.", nil)
	}
	id := ids.New("pay")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_payroll_periods(id,organization_id,branch_id,period_start,period_end,currency,created_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7)`, id, t.OrganizationID, in.BranchID, in.PeriodStart, in.PeriodEnd, currency, t.ActorID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.created.v1", "payroll_period", id, 1, map[string]any{"payrollPeriodId": id, "currency": currency, "periodStart": in.PeriodStart, "periodEnd": in.PeriodEnd})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}
func (s *Service) GetPayrollPeriod(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT p.*,COALESCE((SELECT json_agg(row_to_json(i) ORDER BY i.created_at) FROM fleet_partner_provider_compensation_items i WHERE i.organization_id=p.organization_id AND i.payroll_period_id=p.id),'[]'::json) items FROM fleet_partner_payroll_periods p WHERE p.organization_id=$1 AND p.id=$2) x`, t.OrganizationID, id))
}
func (s *Service) ListPayrollPeriods(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(p) FROM fleet_partner_payroll_periods p WHERE organization_id=$1 AND (cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY period_start DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

func (s *Service) CalculatePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, currency, branch string
		var version, calcVersion int64
		var start, end time.Time
		if err := tx.QueryRow(ctx, `SELECT status,currency,COALESCE(branch_id,''),version,calculation_version,period_start::timestamptz,period_end::timestamptz + interval '1 day' FROM fleet_partner_payroll_periods WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &currency, &branch, &version, &calcVersion, &start, &end); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != expected {
			return apperrors.Conflict("Payroll period version changed.", nil)
		}
		if status == "APPROVED" || status == "POSTING" || status == "POSTED" || status == "CLOSED" {
			return apperrors.Conflict("Approved payroll periods are immutable.", nil)
		}
		calcVersion++
		type activeRule struct {
			id, typ string
			version int64
			cfg     ruleCalculation
		}
		rules, err := tx.Query(ctx, `SELECT id,rule_type,version,calculation_configuration FROM fleet_partner_compensation_rules WHERE organization_id=$1 AND currency=$2 AND status IN('APPROVED','ACTIVE') AND effective_from <= $3::date AND (effective_until IS NULL OR effective_until >= $4::date) ORDER BY id`, t.OrganizationID, currency, end, start)
		if err != nil {
			return err
		}
		var active []activeRule
		for rules.Next() {
			var r activeRule
			var raw []byte
			if err := rules.Scan(&r.id, &r.typ, &r.version, &raw); err != nil {
				rules.Close()
				return err
			}
			if err := json.Unmarshal(raw, &r.cfg); err != nil {
				rules.Close()
				return err
			}
			active = append(active, r)
		}
		if err := rules.Err(); err != nil {
			rules.Close()
			return err
		}
		rules.Close()
		// CMP-027. Components already paid with a completed job's own money
		// must not be paid a second time here. The two sides of that check are
		// computed by different code from different tables, so payroll reads
		// the durable record of what work settlement already paid and skips
		// exactly those components.
		//
		// The completed assignments and the exclusions are read once rather
		// than per rule: the query does not depend on the rule, and re-reading
		// it per rule only multiplies the work.
		type sourceItem struct {
			member, source string
			version        int64
		}
		var jobSources []sourceItem
		var workSettled map[string]bool
		jobSourcesLoaded := false
		loadJobSources := func() error {
			if jobSourcesLoaded {
				return nil
			}
			jobsRows, err := tx.Query(ctx, `SELECT provider_membership_id,id,version FROM fleet_partner_private_assignments WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND status='COMPLETED' AND updated_at >= $3 AND updated_at < $4 ORDER BY id`, t.OrganizationID, branch, start, end)
			if err != nil {
				return err
			}
			for jobsRows.Next() {
				var item sourceItem
				if err := jobsRows.Scan(&item.member, &item.source, &item.version); err != nil {
					jobsRows.Close()
					return err
				}
				jobSources = append(jobSources, item)
			}
			if err := jobsRows.Err(); err != nil {
				jobsRows.Close()
				return err
			}
			jobsRows.Close()
			assignmentIDs := make([]string, 0, len(jobSources))
			for _, item := range jobSources {
				assignmentIDs = append(assignmentIDs, item.source)
			}
			workSettled = map[string]bool{}
			if len(assignmentIDs) > 0 {
				// Matched on the assignment rather than a time window: a job
				// completed inside the period can be settled moments after the
				// period ends, and a windowed read would miss it and pay it
				// again.
				settledRows, err := tx.Query(ctx, `SELECT source_assignment_id,rule_id FROM fleet_partner_work_settled_components WHERE organization_id=$1 AND settlement_level='WORK' AND source_assignment_id = ANY($2)`, t.OrganizationID, assignmentIDs)
				if err != nil {
					return err
				}
				for settledRows.Next() {
					var assignmentID, settledRuleID string
					if err := settledRows.Scan(&assignmentID, &settledRuleID); err != nil {
						settledRows.Close()
						return err
					}
					workSettled[assignmentID+":"+settledRuleID] = true
				}
				if err := settledRows.Err(); err != nil {
					settledRows.Close()
					return err
				}
				settledRows.Close()
			}
			jobSourcesLoaded = true
			return nil
		}
		count := 0
		for _, rule := range active {
			ruleID, ruleType, ruleVersion, cfg := rule.id, rule.typ, rule.version, rule.cfg
			if ruleType == "DEDUCTION" || ruleType == "MANUAL_ADJUSTMENT" {
				continue
			}
			if ruleType == "PER_JOB" || ruleType == "COMMISSION" || ruleType == "BONUS" {
				if err := loadJobSources(); err != nil {
					return err
				}
				for _, item := range jobSources {
					var member, source string
					var sourceVersion int64
					member, source, sourceVersion = item.member, item.source, item.version
					// Already paid with this job's own money. Paying it here
					// as well is the double payment CMP-027 exists to stop.
					if workSettled[source+":"+ruleID] {
						continue
					}
					inputs, _ := json.Marshal(map[string]any{"assignmentId": source, "assignmentVersion": sourceVersion, "ruleVersion": ruleVersion})
					tag, err := tx.Exec(ctx, `INSERT INTO fleet_partner_provider_compensation_items(id,organization_id,payroll_period_id,membership_id,source_type,source_reference,rule_id,rule_version,calculation_version,amount_minor,currency,reason_code,explanation,source_inputs) VALUES($1,$2,$3,$4,'COMPLETED_ASSIGNMENT',$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`, ids.New("pci"), t.OrganizationID, id, member, source, ruleID, ruleVersion, calcVersion, cfg.AmountMinor, currency, ruleType, "Deterministic "+ruleType+" rule version "+fmt.Sprint(ruleVersion), inputs)
					if err != nil {
						return err
					}
					count += int(tag.RowsAffected())
				}
			} else {
				members, err := tx.Query(ctx, `SELECT id FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND status='ACTIVE' ORDER BY id`, t.OrganizationID)
				if err != nil {
					return err
				}
				var memberIDs []string
				for members.Next() {
					var member string
					if err := members.Scan(&member); err != nil {
						members.Close()
						return err
					}
					memberIDs = append(memberIDs, member)
				}
				if err := members.Err(); err != nil {
					members.Close()
					return err
				}
				members.Close()
				for _, member := range memberIDs {
					inputs, _ := json.Marshal(map[string]any{"periodStart": start, "periodEnd": end, "ruleVersion": ruleVersion})
					tag, err := tx.Exec(ctx, `INSERT INTO fleet_partner_provider_compensation_items(id,organization_id,payroll_period_id,membership_id,source_type,source_reference,rule_id,rule_version,calculation_version,amount_minor,currency,reason_code,explanation,source_inputs) VALUES($1,$2,$3,$4,'PAYROLL_PERIOD',$3,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, ids.New("pci"), t.OrganizationID, id, member, ruleID, ruleVersion, calcVersion, cfg.AmountMinor, currency, ruleType, "Deterministic "+ruleType+" rule version "+fmt.Sprint(ruleVersion), inputs)
					if err != nil {
						return err
					}
					count += int(tag.RowsAffected())
				}
			}
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='CALCULATED',calculation_version=$3,calculated_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, calcVersion).Scan(&nv); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "CALCULATED", "calculationVersion": calcVersion, "itemCount": count})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_payroll_history(id,organization_id,payroll_period_id,previous_status,new_status,calculation_version,snapshot,actor_reference) VALUES($1,$2,$3,$4,'CALCULATED',$5,$6,$7)`, ids.New("pyh"), t.OrganizationID, id, status, calcVersion, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.calculated.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "calculationVersion": calcVersion, "itemCount": count, "currency": currency})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

func (s *Service) ApprovePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, creator string
		var version, calc int64
		if err := tx.QueryRow(ctx, `SELECT status,created_by,version,calculation_version FROM fleet_partner_payroll_periods WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &creator, &version, &calc); err != nil {
			return notFoundOnNoRows(err)
		}
		if version != expected || status != "CALCULATED" {
			return apperrors.Conflict("Payroll period changed or is not calculated.", nil)
		}
		if creator == t.ActorID {
			return apperrors.Forbidden("Payroll creator cannot approve the same period.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='APPROVED',approved_by=$3,approved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, t.ActorID).Scan(&nv); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='APPROVED' WHERE organization_id=$1 AND payroll_period_id=$2 AND calculation_version=$3`, t.OrganizationID, id, calc); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "APPROVED", "calculationVersion": calc})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_payroll_history(id,organization_id,payroll_period_id,previous_status,new_status,calculation_version,snapshot,actor_reference) VALUES($1,$2,$3,$4,'APPROVED',$5,$6,$7)`, ids.New("pyh"), t.OrganizationID, id, status, calc, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.approved.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "calculationVersion": calc})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

func (s *Service) createPosting(ctx context.Context, t tenant.Context, req PostingRequest) (string, bool, error) {
	operationID := ids.New("fpo")
	fp := fingerprint(req)
	tag, err := s.o.Pool.Exec(ctx, `INSERT INTO fleet_partner_financial_posting_operations(id,organization_id,record_type,record_id,operation_type,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(organization_id,operation_type,idempotency_key) DO NOTHING`, operationID, t.OrganizationID, req.RecordType, req.RecordID, req.OperationType, req.IdempotencyKey, fp)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 0 {
		var status, storedFingerprint string
		if err := s.o.Pool.QueryRow(ctx, `SELECT id,status,request_fingerprint FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND operation_type=$2 AND idempotency_key=$3`, t.OrganizationID, req.OperationType, req.IdempotencyKey).Scan(&operationID, &status, &storedFingerprint); err != nil {
			return "", false, err
		}
		if storedFingerprint != fp {
			return "", false, apperrors.Conflict("Posting idempotency key was already used for a different request.", nil)
		}
		return operationID, status == "CONFIRMED", nil
	}
	return operationID, false, nil
}

func (s *Service) executePosting(ctx context.Context, t tenant.Context, operationID string, req PostingRequest) (PostingOutcome, error) {
	if !s.o.Flags.Posting || s.o.Wallet == nil || !s.o.Wallet.Configured(req.OperationType) {
		return PostingOutcome{}, apperrors.Unavailable("The authoritative financial posting adapter is unavailable.", nil)
	}
	start := s.o.Now().UTC()
	out, err := s.o.Wallet.Submit(ctx, operationMeta(t, req.IdempotencyKey), req)
	outcome := "SUCCESS"
	status := "CONFIRMED"
	code := ""
	if err != nil {
		outcome = "FAILED"
		status = "RETRY_SCHEDULED"
		code = "wallet_unavailable"
		var ce *platformclients.Error
		if errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeUnknown {
			outcome = "OUTCOME_UNKNOWN"
			status = "OUTCOME_UNKNOWN"
			read, readErr := s.o.Wallet.ReadByKey(ctx, operationMeta(t, req.IdempotencyKey), req)
			if readErr == nil {
				out = read
				err = nil
				outcome = "READ_BACK_CONFIRMED"
				status = "CONFIRMED"
				code = ""
			}
		}
	}
	if err == nil && !postingConfirmed(out.Status) {
		err = fmt.Errorf("authoritative posting status %q is not confirmed", out.Status)
		outcome = "OUTCOME_UNKNOWN"
		status = "OUTCOME_UNKNOWN"
		code = "authoritative_confirmation_pending"
	}
	txErr := s.tx(ctx, func(tx pgx.Tx) error {
		var attempt int
		if e := tx.QueryRow(ctx, `SELECT attempts+1 FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, operationID).Scan(&attempt); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO fleet_partner_financial_posting_attempts(id,organization_id,operation_id,attempt_number,outcome,external_reference,error_code,started_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())`, ids.New("fpa"), t.OrganizationID, operationID, attempt, outcome, out.Reference, code, start); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status=$3,external_operation_reference=$4,source_version=$5,attempts=$6,last_error_code=$7,next_attempt_at=CASE WHEN $3 IN('OUTCOME_UNKNOWN','RETRY_SCHEDULED') THEN now()+interval '30 seconds' ELSE next_attempt_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, operationID, status, out.Reference, out.SourceVersion, attempt, code)
		return e
	})
	if txErr != nil {
		return out, txErr
	}
	return out, err
}

func postingConfirmed(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CONFIRMED", "POSTED", "SUCCEEDED", "COMPLETED":
		return true
	default:
		return false
	}
}

func (s *Service) PostPayroll(ctx context.Context, t tenant.Context, id, key string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, apperrors.Validation("Posting idempotency key is required.", nil)
	}
	var status, currency string
	var version int64
	if err := s.o.Pool.QueryRow(ctx, `SELECT status,currency,version FROM fleet_partner_payroll_periods WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&status, &currency, &version); err != nil {
		return nil, notFoundOnNoRows(err)
	}
	if status != "APPROVED" || version != expected {
		return nil, apperrors.Conflict("Payroll period changed or is not approved.", nil)
	}
	type item struct {
		id, membershipID, providerID, subwalletID, reason string
		amount                                            int64
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT i.id,i.membership_id,COALESCE(m.provider_id,''),COALESCE(w.wallet_account_id,''),i.reason_code,i.amount_minor
		FROM fleet_partner_provider_compensation_items i
		JOIN fleet_partner_organization_provider_memberships m ON m.id=i.membership_id AND m.organization_id=i.organization_id
		LEFT JOIN fleet_partner_provider_subwallet_links w ON w.provider_membership_id=i.membership_id AND w.organization_id=i.organization_id AND w.status='ACTIVE'
		WHERE i.organization_id=$1 AND i.payroll_period_id=$2 AND i.status IN('APPROVED','POSTED') ORDER BY i.id`, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	var items []item
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.id, &x.membershipID, &x.providerID, &x.subwalletID, &x.reason, &x.amount); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(items) == 0 {
		return nil, apperrors.Conflict("Payroll has no approved compensation items.", nil)
	}
	total := int64(0)
	lastOperationID := ""
	for _, x := range items {
		total += x.amount
		if x.subwalletID == "" || x.providerID == "" {
			return nil, apperrors.Unavailable("Provider sub-wallet provisioning is incomplete.", nil)
		}
		var itemStatus string
		if err := s.o.Pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_compensation_items WHERE organization_id=$1 AND id=$2`, t.OrganizationID, x.id).Scan(&itemStatus); err != nil {
			return nil, err
		}
		if itemStatus == "POSTED" {
			continue
		}
		allocationType := payrollAllocationType(x.reason)
		payload, _ := json.Marshal(map[string]any{"earningAccountId": x.subwalletID, "organizationMembershipId": x.membershipID, "providerId": x.providerID, "allocationType": allocationType, "businessReferenceType": "PAYROLL_REFERENCE", "businessReferenceId": id + ":" + x.id, "amountMinor": x.amount, "currency": currency, "reasonCode": x.reason, "providerExplanation": "Approved payroll compensation item " + x.id, "approvalReference": id, "sourceService": "fleet-partner-service", "sourceReference": x.id})
		req := PostingRequest{OrganizationID: t.OrganizationID, RecordType: "PAYROLL_ITEM", RecordID: x.id, OperationType: "PROVIDER_ALLOCATION", IdempotencyKey: key + ":" + x.id, AmountMinor: x.amount, Currency: currency, Payload: payload}
		opID, confirmed, createErr := s.createPosting(ctx, t, req)
		if createErr != nil {
			return nil, createErr
		}
		lastOperationID = opID
		if !confirmed {
			out, execErr := s.executePosting(ctx, t, opID, req)
			if execErr != nil {
				_, _ = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_payroll_periods SET status='PARTIALLY_POSTED',updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id)
				return nil, apperrors.Unavailable("Payroll posting requires reconciliation.", execErr)
			}
			if out.Status != "" && out.Status != "CONFIRMED" && out.Status != "POSTED" && out.Status != "SUCCEEDED" {
				return nil, apperrors.Unavailable("Wallet has not confirmed payroll posting.", nil)
			}
		}
		if _, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='POSTED' WHERE organization_id=$1 AND id=$2 AND status='APPROVED'`, t.OrganizationID, x.id); err != nil {
			return nil, err
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='POSTED',posted_at=now(),wallet_operation_reference=COALESCE((SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),wallet_operation_reference),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('APPROVED','PARTIALLY_POSTED') RETURNING version`, t.OrganizationID, id, lastOperationID).Scan(&nv); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.posted.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "currency": currency, "amountMinor": total, "postingOperationId": lastOperationID, "itemCount": len(items)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

func payrollAllocationType(reason string) string {
	switch strings.ToUpper(reason) {
	case "SALARY":
		return "SALARY"
	case "PER_JOB", "COMMISSION":
		return "PER_JOB"
	case "ALLOWANCE":
		return "ALLOWANCE"
	case "BONUS":
		return "BONUS"
	default:
		return "WAGE"
	}
}

func (s *Service) ReversePayroll(ctx context.Context, t tenant.Context, id, reason string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperrors.Validation("Reversal reason is required.", nil)
	}
	var periodStatus, currency string
	var periodVersion, calculationVersion int64
	if err := s.o.Pool.QueryRow(ctx, `SELECT status,currency,version,calculation_version FROM fleet_partner_payroll_periods WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&periodStatus, &currency, &periodVersion, &calculationVersion); err != nil {
		return nil, notFoundOnNoRows(err)
	}
	if periodStatus != "POSTED" || periodVersion != expected {
		return nil, apperrors.Conflict("Payroll period changed or is not posted.", nil)
	}
	type reversalItem struct {
		id, membershipID, providerID, subwalletID string
		amount                                    int64
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT i.id,i.membership_id,COALESCE(m.provider_id,''),COALESCE(w.wallet_account_id,''),i.amount_minor
		FROM fleet_partner_provider_compensation_items i
		JOIN fleet_partner_organization_provider_memberships m ON m.id=i.membership_id AND m.organization_id=i.organization_id
		LEFT JOIN fleet_partner_provider_subwallet_links w ON w.provider_membership_id=i.membership_id AND w.organization_id=i.organization_id AND w.status='ACTIVE'
		WHERE i.organization_id=$1 AND i.payroll_period_id=$2 AND i.status='POSTED' ORDER BY i.id`, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	var items []reversalItem
	for rows.Next() {
		var x reversalItem
		if err := rows.Scan(&x.id, &x.membershipID, &x.providerID, &x.subwalletID, &x.amount); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(items) == 0 {
		return nil, apperrors.Conflict("Posted payroll has no compensation items to reverse.", nil)
	}
	for _, x := range items {
		if x.providerID == "" || x.subwalletID == "" || x.amount <= 0 {
			return nil, apperrors.Unavailable("Provider sub-wallet reversal data is incomplete.", nil)
		}
		payload, _ := json.Marshal(map[string]any{"earningAccountId": x.subwalletID, "organizationMembershipId": x.membershipID, "providerId": x.providerID, "allocationType": "REVERSAL", "businessReferenceType": "PAYROLL_REFERENCE", "businessReferenceId": id + ":" + x.id + ":reversal", "amountMinor": x.amount, "currency": currency, "reasonCode": "PAYROLL_REVERSAL", "providerExplanation": reason, "approvalReference": id, "sourceService": "fleet-partner-service", "sourceReference": x.id})
		req := PostingRequest{OrganizationID: t.OrganizationID, RecordType: "PAYROLL_ITEM", RecordID: x.id, OperationType: "PAYROLL_REVERSAL", IdempotencyKey: fmt.Sprintf("payroll-reversal:%s:%d:%s", id, expected, x.id), AmountMinor: x.amount, Currency: currency, Payload: payload}
		opID, confirmed, err := s.createPosting(ctx, t, req)
		if err != nil {
			return nil, err
		}
		if !confirmed {
			if _, err = s.executePosting(ctx, t, opID, req); err != nil {
				return nil, apperrors.Unavailable("Payroll reversal requires authoritative reconciliation.", err)
			}
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_payroll_periods SET status='REVERSED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status='POSTED' RETURNING version`, t.OrganizationID, id, expected).Scan(&nv); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.Conflict("Payroll period changed while its reversals were being confirmed.", nil)
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE fleet_partner_provider_compensation_items SET status='REVERSED' WHERE organization_id=$1 AND payroll_period_id=$2 AND status='POSTED'`, t.OrganizationID, id); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "REVERSED", "calculationVersion": calculationVersion, "itemCount": len(items), "version": nv})
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_payroll_history(id,organization_id,payroll_period_id,previous_status,new_status,calculation_version,snapshot,actor_reference,reason) VALUES($1,$2,$3,'POSTED','REVERSED',$4,$5,$6,$7)`, ids.New("pyh"), t.OrganizationID, id, calculationVersion, snapshot, t.ActorID, reason); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.reversed.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "currency": currency, "itemCount": len(items)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

func (s *Service) ProviderStatements(ctx context.Context, t tenant.Context, membershipID string) (Statement, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return Statement{}, err
	}
	var belongs bool
	if err := s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, membershipID).Scan(&belongs); err != nil {
		return Statement{}, err
	}
	if !belongs {
		return Statement{}, apperrors.NotFound("Resource not found.", nil)
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT json_build_object('period',row_to_json(p),'items',COALESCE(json_agg(row_to_json(i) ORDER BY i.created_at),'[]'::json)) FROM fleet_partner_payroll_periods p JOIN fleet_partner_provider_compensation_items i ON i.payroll_period_id=p.id AND i.organization_id=p.organization_id WHERE p.organization_id=$1 AND i.membership_id=$2 GROUP BY p.id ORDER BY p.period_start DESC`, t.OrganizationID, membershipID)
	if err != nil {
		return Statement{}, err
	}
	defer rows.Close()
	out := Statement{MembershipID: membershipID, Periods: []json.RawMessage{}}
	for rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return Statement{}, err
		}
		out.Periods = append(out.Periods, raw)
	}
	return out, rows.Err()
}

func (s *Service) CreateSettlement(ctx context.Context, t tenant.Context, in SettlementInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, true); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.SupplierReference == "" || in.ProviderOrCarrierReference == "" || len(in.JobReferences) == 0 || in.AmountMinor < 0 || in.PeriodEnd.Before(in.PeriodStart) {
		return nil, apperrors.Validation("Settlement scope, jobs, period and amount are required.", nil)
	}
	id := ids.New("sts")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_subcontractor_settlements(id,organization_id,supplier_reference,provider_or_carrier_reference,period_start,period_end,currency,amount_minor,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, t.OrganizationID, in.SupplierReference, in.ProviderOrCarrierReference, in.PeriodStart, in.PeriodEnd, currency, in.AmountMinor, t.ActorID); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, job := range in.JobReferences {
			job = strings.TrimSpace(job)
			if job == "" || seen[job] {
				return apperrors.Validation("Settlement job references must be unique and non-empty.", nil)
			}
			seen[job] = true
			var belongs bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2 AND status='COMPLETED')`, t.OrganizationID, job).Scan(&belongs); err != nil {
				return err
			}
			if !belongs {
				return apperrors.Validation("Settlement job does not belong to this organization or is incomplete.", nil)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_subcontractor_settlement_jobs(settlement_id,organization_id,job_reference) VALUES($1,$2,$3)`, id, t.OrganizationID, job); err != nil {
				return apperrors.Conflict("A job cannot be settled twice.", err)
			}
		}
		return s.record(ctx, tx, t, "subcontractor_settlement.created.v1", "subcontractor_settlement", id, 1, map[string]any{"settlementId": id, "currency": currency, "amountMinor": in.AmountMinor, "jobCount": len(in.JobReferences)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetSettlement(ctx, t, id)
}
func (s *Service) GetSettlement(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT s.*,COALESCE((SELECT json_agg(j.job_reference ORDER BY j.job_reference) FROM fleet_partner_subcontractor_settlement_jobs j WHERE j.organization_id=s.organization_id AND j.settlement_id=s.id),'[]'::json) job_references FROM fleet_partner_subcontractor_settlements s WHERE s.organization_id=$1 AND s.id=$2) x`, t.OrganizationID, id))
}
func (s *Service) ListSettlements(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(s) FROM fleet_partner_subcontractor_settlements s WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) SettlementCommand(ctx context.Context, t tenant.Context, id, command, key string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if command == "approve" {
		tag, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_subcontractor_settlements SET status='APPROVED',approved_by=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$4 AND status IN('DRAFT','CALCULATED','PENDING_APPROVAL')`, t.OrganizationID, id, t.ActorID, expected)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, apperrors.Conflict("Settlement changed or cannot be approved.", nil)
		}
		return s.GetSettlement(ctx, t, id)
	}
	if command != "post" {
		return nil, apperrors.Validation("Unsupported settlement command.", nil)
	}
	v, err := s.GetSettlement(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if statusString(v) != "APPROVED" || int64Value(v, "version") != expected {
		return nil, apperrors.Conflict("Settlement changed or is not approved.", nil)
	}
	req := PostingRequest{OrganizationID: t.OrganizationID, RecordType: "SUBCONTRACTOR_SETTLEMENT", RecordID: id, OperationType: "SETTLEMENT_REQUEST", IdempotencyKey: key, AmountMinor: int64Value(v, "amount_minor"), Currency: fmt.Sprint(v["currency"]), ExpectedVersion: expected}
	opID, confirmed, err := s.createPosting(ctx, t, req)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		if _, err = s.executePosting(ctx, t, opID, req); err != nil {
			return nil, apperrors.Unavailable("Settlement posting requires reconciliation.", err)
		}
	}
	tag, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_subcontractor_settlements SET status='POSTED',posting_reference=(SELECT external_operation_reference FROM fleet_partner_financial_posting_operations WHERE id=$3),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='APPROVED'`, t.OrganizationID, id, opID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return s.GetSettlement(ctx, t, id)
	}
	return s.GetSettlement(ctx, t, id)
}
