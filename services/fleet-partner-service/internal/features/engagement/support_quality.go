package engagement

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) CreateSupportCase(ctx context.Context, t tenant.Context, key string, in SupportCreateInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, apperrors.Validation("Support idempotency key, subject and description are required.", nil)
	}
	if s.o.Support == nil || !s.o.Support.Configured() {
		return nil, apperrors.Unavailable("Support service is unavailable.", nil)
	}
	if existing, err := jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 AND create_idempotency_key=$2`, t.OrganizationID, key)); err == nil {
		return existing, nil
	}
	projection, err := s.o.Support.Create(ctx, t.OrganizationID, key, in)
	if err != nil {
		return nil, apperrors.Unavailable("Support case outcome is unknown and requires reconciliation.", err)
	}
	if projection.CaseID == "" || projection.SourceVersion <= 0 {
		return nil, apperrors.Unavailable("Support service returned an invalid projection.", nil)
	}
	localID := ids.New("scp")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,customer_reference,provider_reference,trip_reference,order_reference,assignment_reference,owner_reference,source_version,last_synced_at,create_idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),$14) ON CONFLICT(organization_id,create_idempotency_key) WHERE create_idempotency_key<>'' DO NOTHING`, localID, t.OrganizationID, projection.CaseID, in.CaseType, projection.Status, projection.Priority, in.CustomerReference, in.ProviderReference, in.TripReference, in.OrderReference, in.AssignmentReference, projection.OwnerReference, projection.SourceVersion, key)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "support_case_projection.created.v1", "support_case_projection", localID, 1, map[string]any{"projectionId": localID, "supportCaseId": projection.CaseID, "status": projection.Status, "sourceVersion": projection.SourceVersion})
	})
	if err != nil {
		return nil, err
	}
	return s.GetSupportCase(ctx, t, projection.CaseID)
}
func (s *Service) GetSupportCase(ctx context.Context, t tenant.Context, caseID string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 AND (id=$2 OR support_service_case_id=$2)`, t.OrganizationID, caseID))
}
func (s *Service) ListSupportCases(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 ORDER BY updated_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

func (s *Service) ApplySupportEvent(ctx context.Context, in SupportEvent) error {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.CaseID == "" || in.SourceVersion <= 0 {
		return apperrors.Validation("Support event is incomplete.", nil)
	}
	in.SafePayload = rawOr(in.SafePayload, "{}")
	return s.tx(ctx, func(tx pgx.Tx) error { return s.ApplySupportEventTx(ctx, tx, in) })
}

// ApplySupportEventTx projects a support event in a caller-owned transaction.
// It is used by the common Section 26 stream consumer so the shared inbox and
// the authoritative support projection commit atomically.
func (s *Service) ApplySupportEventTx(ctx context.Context, tx pgx.Tx, in SupportEvent) error {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.CaseID == "" || in.SourceVersion <= 0 {
		return apperrors.Validation("Support event is incomplete.", nil)
	}
	in.SafePayload = rawOr(in.SafePayload, "{}")
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "support-dispute-service"}
	var id string
	var current int64
	err := tx.QueryRow(ctx, `SELECT id,source_version FROM fleet_partner_support_case_projections WHERE organization_id=$1 AND support_service_case_id=$2 FOR UPDATE`, in.OrganizationID, in.CaseID).Scan(&id, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		id = ids.New("scp")
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,source_version,last_synced_at,gap_status) VALUES($1,$2,$3,'PLATFORM',$4,$5,$6,now(),CASE WHEN $6=1 THEN 'CURRENT' ELSE 'GAP_DETECTED' END)`, id, in.OrganizationID, in.CaseID, in.Status, in.Priority, in.SourceVersion); err != nil {
			return err
		}
		current = 0
	} else if err != nil {
		return err
	}
	result := "APPLIED"
	gap := "CURRENT"
	if in.SourceVersion <= current {
		result = "DUPLICATE"
	} else if in.SourceVersion > current+1 {
		result = "GAP_DETECTED"
		gap = "GAP_DETECTED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_support_case_events_inbox(id,organization_id,event_id,support_service_case_id,source_version,event_type,safe_payload,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_id) DO NOTHING`, ids.New("sei"), in.OrganizationID, in.EventID, in.CaseID, in.SourceVersion, in.EventType, in.SafePayload, result); err != nil {
		return err
	}
	if result == "DUPLICATE" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE fleet_partner_support_case_projections SET status=$3,priority=$4,owner_reference=$5,source_version=$6,last_synced_at=now(),gap_status=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND support_service_case_id=$2`, in.OrganizationID, in.CaseID, in.Status, in.Priority, in.OwnerReference, in.SourceVersion, gap); err != nil {
		return err
	}
	return s.record(ctx, tx, t, "support_case_projection.updated.v1", "support_case_projection", id, in.SourceVersion, map[string]any{"projectionId": id, "supportCaseId": in.CaseID, "status": in.Status, "sourceVersion": in.SourceVersion, "gapStatus": gap})
}

func (s *Service) CreateQualityCase(ctx context.Context, t tenant.Context, in QualityInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, true); err != nil {
		return nil, err
	}
	in.Severity = strings.ToUpper(in.Severity)
	if in.Category == "" || in.SourceType == "" || in.SourceReference == "" || (in.Severity != "LOW" && in.Severity != "MEDIUM" && in.Severity != "HIGH" && in.Severity != "CRITICAL") {
		return nil, apperrors.Validation("Quality category, severity and source are required.", nil)
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	id := ids.New("qlt")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_quality_cases(id,organization_id,branch_id,category,severity,source_type,source_reference,customer_reference,provider_reference,owner_user_id,created_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)`, id, t.OrganizationID, in.BranchID, in.Category, in.Severity, in.SourceType, in.SourceReference, in.CustomerReference, in.ProviderReference, in.OwnerUserID, t.ActorID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "quality_case.created.v1", "quality_case", id, 1, map[string]any{"qualityCaseId": id, "category": in.Category, "severity": in.Severity, "sourceType": in.SourceType, "sourceReference": in.SourceReference})
	})
	if err != nil {
		return nil, err
	}
	return s.GetQualityCase(ctx, t, id)
}
func (s *Service) GetQualityCase(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, false); err != nil {
		return nil, err
	}
	v, err := jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(q) FROM fleet_partner_quality_cases q WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}
func (s *Service) ListQualityCases(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(q) FROM fleet_partner_quality_cases q WHERE organization_id=$1 AND (cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) UpdateQualityCase(ctx context.Context, t tenant.Context, id string, in QualityInput, resolve bool) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	if resolve && (strings.TrimSpace(in.RootCause) == "" || strings.TrimSpace(in.CorrectiveActionSummary) == "") {
		return nil, apperrors.Validation("Quality resolution requires root cause and corrective action.", nil)
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_quality_cases WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Quality case version changed.", nil)
		}
		if status == "CLOSED" || status == "CANCELLED" {
			return apperrors.Conflict("Final quality cases are immutable.", nil)
		}
		next := strings.ToUpper(in.Status)
		if resolve {
			next = "RESOLVED"
		}
		if next == "" {
			next = status
		}
		valid := map[string]bool{"OPEN": true, "UNDER_REVIEW": true, "ACTION_REQUIRED": true, "MONITORING": true, "RESOLVED": true, "CLOSED": true, "CANCELLED": true}
		if !valid[next] {
			return apperrors.Validation("Quality case status is invalid.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_quality_cases SET category=CASE WHEN $3='' THEN category ELSE $3 END,severity=CASE WHEN $4='' THEN severity ELSE $4 END,owner_user_id=CASE WHEN $5='' THEN owner_user_id ELSE $5 END,status=$6,root_cause=CASE WHEN $7='' THEN root_cause ELSE $7 END,corrective_action_summary=CASE WHEN $8='' THEN corrective_action_summary ELSE $8 END,resolved_by=CASE WHEN $6='RESOLVED' THEN $9 ELSE resolved_by END,resolved_at=CASE WHEN $6='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, in.Category, strings.ToUpper(in.Severity), in.OwnerUserID, next, in.RootCause, in.CorrectiveActionSummary, t.ActorID).Scan(&nv); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_quality_case_history(id,organization_id,quality_case_id,previous_status,new_status,root_cause,corrective_action_summary,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("qhh"), t.OrganizationID, id, status, next, in.RootCause, in.CorrectiveActionSummary, t.ActorID, nv); err != nil {
			return err
		}
		event := "quality_case.updated.v1"
		if resolve {
			event = "quality_case.resolved.v1"
		}
		return s.record(ctx, tx, t, event, "quality_case", id, nv, map[string]any{"qualityCaseId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetQualityCase(ctx, t, id)
}

func parseRat(v string) (*big.Rat, error) {
	r := new(big.Rat)
	if _, ok := r.SetString(strings.TrimSpace(v)); !ok {
		return nil, apperrors.Validation("SLA values must be fixed decimal numbers.", nil)
	}
	return r, nil
}
func (s *Service) RecordMeasurement(ctx context.Context, in MeasurementInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, true); err != nil {
		return nil, err
	}
	if in.OrganizationID == "" || in.AgreementReference == "" || in.MetricCode == "" || in.FormulaVersion <= 0 || !in.PeriodEnd.After(in.PeriodStart) {
		return nil, apperrors.Validation("SLA measurement scope, formula and period are required.", nil)
	}
	target, err := parseRat(in.TargetValue)
	if err != nil {
		return nil, err
	}
	status := "UNKNOWN"
	var measured any = nil
	if in.Complete {
		m, err := parseRat(in.MeasuredValue)
		if err != nil {
			return nil, err
		}
		measured = in.MeasuredValue
		cmp := m.Cmp(target)
		if in.HigherIsBetter {
			if cmp >= 0 {
				status = "MET"
			} else {
				status = "BREACHED"
			}
		} else {
			if cmp <= 0 {
				status = "MET"
			} else {
				status = "BREACHED"
			}
		}
	}
	id := ids.New("slm")
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section23-sla-worker"}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO fleet_partner_sla_measurements(id,organization_id,agreement_reference,contract_id,customer_id,metric_code,formula_version,period_start,period_end,target_value,measured_value,unit,breach_status,source_inputs,source_versions,generated_at) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now()) ON CONFLICT(organization_id,agreement_reference,metric_code,period_start,period_end,formula_version) DO NOTHING`, id, in.OrganizationID, in.AgreementReference, in.ContractID, in.CustomerID, in.MetricCode, in.FormulaVersion, in.PeriodStart, in.PeriodEnd, in.TargetValue, measured, in.Unit, status, rawOr(in.SourceInputs, "{}"), rawOr(in.SourceVersions, "{}"))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return tx.QueryRow(ctx, `SELECT id FROM fleet_partner_sla_measurements WHERE organization_id=$1 AND agreement_reference=$2 AND metric_code=$3 AND period_start=$4 AND period_end=$5 AND formula_version=$6`, in.OrganizationID, in.AgreementReference, in.MetricCode, in.PeriodStart, in.PeriodEnd, in.FormulaVersion).Scan(&id)
		}
		if err := s.record(ctx, tx, t, "sla_measurement.created.v1", "sla_measurement", id, 1, map[string]any{"measurementId": id, "metricCode": in.MetricCode, "breachStatus": status, "formulaVersion": in.FormulaVersion}); err != nil {
			return err
		}
		if status == "BREACHED" {
			eid := ids.New("esc")
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_escalations(id,organization_id,source_type,source_reference,escalation_level,recipient_type,recipient_reference,due_at) VALUES($1,$2,'SLA_MEASUREMENT',$3,1,'ORGANIZATION_ROLE','OPERATIONS',now()) ON CONFLICT DO NOTHING`, eid, in.OrganizationID, id); err != nil {
				return err
			}
			return s.record(ctx, tx, t, "escalation.created.v1", "escalation", eid, 1, map[string]any{"escalationId": eid, "sourceType": "SLA_MEASUREMENT", "sourceReference": id, "level": 1})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 AND id=$2`, in.OrganizationID, id))
}
func (s *Service) GetMeasurement(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListMeasurements(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 ORDER BY generated_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

func (s *Service) CreateEscalation(ctx context.Context, t tenant.Context, in EscalationInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, true); err != nil {
		return nil, err
	}
	if in.SourceType == "" || in.SourceReference == "" || in.Level <= 0 || in.RecipientType == "" || in.RecipientReference == "" || in.DueAt.IsZero() {
		return nil, apperrors.Validation("Escalation source, level, recipient and due time are required.", nil)
	}
	id := ids.New("esc")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO fleet_partner_escalations(id,organization_id,source_type,source_reference,escalation_level,recipient_type,recipient_reference,due_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(organization_id,source_type,source_reference,escalation_level) DO NOTHING`, id, t.OrganizationID, in.SourceType, in.SourceReference, in.Level, in.RecipientType, in.RecipientReference, in.DueAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return tx.QueryRow(ctx, `SELECT id FROM fleet_partner_escalations WHERE organization_id=$1 AND source_type=$2 AND source_reference=$3 AND escalation_level=$4`, t.OrganizationID, in.SourceType, in.SourceReference, in.Level).Scan(&id)
		}
		return s.record(ctx, tx, t, "escalation.created.v1", "escalation", id, 1, map[string]any{"escalationId": id, "sourceType": in.SourceType, "sourceReference": in.SourceReference, "level": in.Level})
	})
	if err != nil {
		return nil, err
	}
	return s.GetEscalation(ctx, t, id)
}
func (s *Service) GetEscalation(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, false); err != nil {
		return nil, err
	}
	return jsonRow(s.o.Pool.QueryRow(ctx, `SELECT row_to_json(e) FROM fleet_partner_escalations e WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
}
func (s *Service) ListEscalations(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, false); err != nil {
		return nil, err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT row_to_json(e) FROM fleet_partner_escalations e WHERE organization_id=$1 ORDER BY due_at`, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
func (s *Service) EscalationCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT status,version FROM fleet_partner_escalations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version); err != nil {
			return err
		}
		if version != expected {
			return apperrors.Conflict("Escalation version changed.", nil)
		}
		next := ""
		if command == "acknowledge" && (status == "PENDING" || status == "NOTIFIED") {
			next = "ACKNOWLEDGED"
		} else if command == "resolve" && (status == "NOTIFIED" || status == "ACKNOWLEDGED") {
			next = "RESOLVED"
		} else {
			return apperrors.Conflict("Escalation command is invalid for its current state.", nil)
		}
		var nv int64
		if err := tx.QueryRow(ctx, `UPDATE fleet_partner_escalations SET status=$3,acknowledged_at=CASE WHEN $3='ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,resolved_at=CASE WHEN $3='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, id, next).Scan(&nv); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_escalation_history(id,organization_id,escalation_id,previous_status,new_status,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("esh"), t.OrganizationID, id, status, next, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "escalation."+command+"d.v1", "escalation", id, nv, map[string]any{"escalationId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetEscalation(ctx, t, id)
}

var _ = time.Now
