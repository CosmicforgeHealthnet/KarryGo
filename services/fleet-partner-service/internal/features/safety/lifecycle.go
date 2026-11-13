package safety

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Incident lifecycle (spec 58.11):
//
//	OPEN -> TRIAGED -> CONTAINED -> INVESTIGATING -> CORRECTIVE_ACTION_REQUIRED -> RESOLVED -> CLOSED
//	any of the first five -> ESCALATED
//
// An escalated incident keeps the stage it escalated from
// (pre_escalation_status) and continues from that stage: escalation adds
// attention, it never skips a precondition.

// stageIs reports whether the incident is at stage, directly or escalated
// from it.
func stageIs(v Incident, stage string) bool {
	return v.Status == stage || v.Status == IncidentEscalated && v.PreEscalationStatus == stage
}

func validSeverity(v string) bool {
	switch v {
	case "LOW", "MEDIUM", "HIGH", "CRITICAL":
		return true
	}
	return false
}

func (s *OperationsService) lockIncident(ctx context.Context, tx pgx.Tx, t tenant.Context, id string) (Incident, error) {
	v, e := scanIncident(tx.QueryRow(ctx, `SELECT `+incidentCols+` FROM fleet_partner_incidents WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Incident not found.", nil)
	}
	return v, e
}

// transition runs one lifecycle command: it locks the incident, checks the
// expected version, lets apply validate preconditions and write its own
// side effects, then applies the incident update, appends history, audit and
// the outbox event in the same transaction.
func (s *OperationsService) transition(ctx context.Context, t tenant.Context, id string, expected int64, to, action, event, reason string, apply func(pgx.Tx, Incident) (string, []any, error)) (Incident, error) {
	if e := s.gate(s.o.Flags.Incidents, true); e != nil {
		return Incident{}, e
	}
	if expected <= 0 {
		return Incident{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var out Incident
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.lockIncident(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if old.Version != expected {
			return apperrors.Conflict("Version conflict.", nil)
		}
		set, args, x := apply(tx, old)
		if x != nil {
			return x
		}
		query := `UPDATE fleet_partner_incidents SET status=$3,version=version+1,updated_at=now()`
		if set != "" {
			query += "," + set
		}
		query += ` WHERE organization_id=$1 AND id=$2 RETURNING ` + incidentCols
		out, x = scanIncident(tx.QueryRow(ctx, query, append([]any{t.OrganizationID, id, to}, args...)...))
		if x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_incident_history(id,organization_id,incident_id,previous_status,new_status,action,actor_reference,reason,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("inh"), t.OrganizationID, id, old.Status, out.Status, action, t.ActorID, reason, out.Version, mustSafetyJSON(out)); x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "incident", id, out.Version, map[string]any{"incidentId": id, "previousStatus": old.Status, "status": out.Status, "severity": out.Severity})
	})
	return out, e
}

// TriageInput carries the facts spec 58.11 requires before triage.
type TriageInput struct {
	Severity, LocationReference, ImmediateActions string
	PeopleReferences, AssetReferences             json.RawMessage
}

func jsonArray(v json.RawMessage) bool {
	var a []any
	return len(v) > 0 && json.Unmarshal(v, &a) == nil
}

// TriageIncident records severity, location, affected people/assets and the
// immediate actions taken (reported -> triaged).
func (s *OperationsService) TriageIncident(ctx context.Context, t tenant.Context, id string, expected int64, in TriageInput) (Incident, error) {
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	return s.transition(ctx, t, id, expected, IncidentTriaged, "TRIAGED", "incident.triaged.v1", in.ImmediateActions, func(_ pgx.Tx, old Incident) (string, []any, error) {
		if !stageIs(old, IncidentOpen) {
			return "", nil, apperrors.Conflict("Only a reported incident can be triaged.", nil)
		}
		if in.Severity == "" {
			in.Severity = old.Severity
		}
		if in.LocationReference == "" {
			in.LocationReference = old.LocationReference
		}
		if len(in.PeopleReferences) == 0 {
			in.PeopleReferences = old.PeopleReferences
		}
		if len(in.AssetReferences) == 0 {
			in.AssetReferences = old.AssetReferences
		}
		if !validSeverity(in.Severity) || strings.TrimSpace(in.LocationReference) == "" || strings.TrimSpace(in.ImmediateActions) == "" {
			return "", nil, apperrors.BadRequest("Triage requires severity, location and the immediate actions taken.", nil)
		}
		if !jsonArray(in.PeopleReferences) || !jsonArray(in.AssetReferences) {
			return "", nil, apperrors.BadRequest("Affected people and asset references must be arrays.", nil)
		}
		return `severity=$4,location_reference=$5,people_references=$6,asset_references=$7,immediate_actions=$8,triaged_by=$9,triaged_at=now()`,
			[]any{in.Severity, strings.TrimSpace(in.LocationReference), in.PeopleReferences, in.AssetReferences, strings.TrimSpace(in.ImmediateActions), t.ActorID}, nil
	})
}

// ContainIncident records that the immediate hazard is controlled
// (triaged -> contained).
func (s *OperationsService) ContainIncident(ctx context.Context, t tenant.Context, id string, expected int64, summary string) (Incident, error) {
	return s.transition(ctx, t, id, expected, IncidentContained, "CONTAINED", "incident.contained.v1", summary, func(_ pgx.Tx, old Incident) (string, []any, error) {
		if !stageIs(old, IncidentTriaged) {
			return "", nil, apperrors.Conflict("Only a triaged incident can be contained.", nil)
		}
		if strings.TrimSpace(summary) == "" {
			return "", nil, apperrors.BadRequest("A containment summary is required.", nil)
		}
		return `containment_summary=$4,contained_by=$5,contained_at=now()`, []any{strings.TrimSpace(summary), t.ActorID}, nil
	})
}

var escalationTriggers = map[string]bool{"SEVERITY": true, "REGULATORY": true, "CUSTOMER": true, "SAFETY_CRITICAL": true, "OTHER": true}

// EscalateIncident records that a severity, regulatory or customer threshold
// was triggered. The stage the incident escalated from is kept.
func (s *OperationsService) EscalateIncident(ctx context.Context, t tenant.Context, id string, expected int64, trigger, reason string) (Incident, error) {
	trigger = strings.ToUpper(strings.TrimSpace(trigger))
	return s.transition(ctx, t, id, expected, IncidentEscalated, "ESCALATED", "incident.escalated.v1", reason, func(_ pgx.Tx, old Incident) (string, []any, error) {
		switch old.Status {
		case IncidentOpen, IncidentTriaged, IncidentContained, IncidentInvestigating, IncidentCorrectiveAction:
		default:
			return "", nil, apperrors.Conflict("Incident cannot be escalated from its current status.", nil)
		}
		if !escalationTriggers[trigger] || strings.TrimSpace(reason) == "" {
			return "", nil, apperrors.BadRequest("Escalation requires a trigger (SEVERITY, REGULATORY, CUSTOMER, SAFETY_CRITICAL, OTHER) and a reason.", nil)
		}
		return `escalation_trigger=$4,escalation_reason=$5,pre_escalation_status=$6,escalated_by=$7,escalated_at=now()`, []any{trigger, strings.TrimSpace(reason), old.Status, t.ActorID}, nil
	})
}

const investigationCols = `id,organization_id,incident_id,investigator_user_id,scope,evidence_preservation,status,findings,root_cause,contributing_factors,findings_submitted_at,approved_by,approved_at,closed_at,opened_by,version,created_at,updated_at`

func scanInvestigation(r pgx.Row) (Investigation, error) {
	var v Investigation
	e := r.Scan(&v.ID, &v.OrganizationID, &v.IncidentID, &v.InvestigatorUserID, &v.Scope, &v.EvidencePreservation, &v.Status, &v.Findings, &v.RootCause, &v.ContributingFactors, &v.FindingsSubmittedAt, &v.ApprovedBy, &v.ApprovedAt, &v.ClosedAt, &v.OpenedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InvestigationInput opens an investigation.
type InvestigationInput struct {
	InvestigatorUserID, Scope, EvidencePreservation string
}

// OpenInvestigation assigns an investigator with a scope and an evidence
// preservation plan (contained -> under_investigation).
func (s *OperationsService) OpenInvestigation(ctx context.Context, t tenant.Context, incidentID string, expected int64, in InvestigationInput) (Investigation, Incident, error) {
	var inv Investigation
	inc, e := s.transition(ctx, t, incidentID, expected, IncidentInvestigating, "INVESTIGATION_STARTED", "incident.investigation_started.v1", in.Scope, func(tx pgx.Tx, old Incident) (string, []any, error) {
		if !stageIs(old, IncidentContained) {
			return "", nil, apperrors.Conflict("An investigation is opened once the incident is contained.", nil)
		}
		in.InvestigatorUserID = strings.TrimSpace(in.InvestigatorUserID)
		if in.InvestigatorUserID == "" || strings.TrimSpace(in.Scope) == "" || strings.TrimSpace(in.EvidencePreservation) == "" {
			return "", nil, apperrors.BadRequest("Investigator, scope and evidence preservation are required.", nil)
		}
		var active bool
		if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_users WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, t.OrganizationID, in.InvestigatorUserID).Scan(&active); x != nil {
			return "", nil, x
		}
		if !active {
			return "", nil, apperrors.BadRequest("Investigator must be an active member of the organization.", nil)
		}
		var x error
		inv, x = scanInvestigation(tx.QueryRow(ctx, `INSERT INTO fleet_partner_incident_investigations(id,organization_id,incident_id,investigator_user_id,scope,evidence_preservation,opened_by)VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+investigationCols, ids.New("iiv"), t.OrganizationID, incidentID, in.InvestigatorUserID, strings.TrimSpace(in.Scope), strings.TrimSpace(in.EvidencePreservation), t.ActorID))
		if isUniqueViolation(x) {
			return "", nil, apperrors.Conflict("The incident already has an open investigation.", nil)
		}
		if x != nil {
			return "", nil, x
		}
		return "", nil, nil
	})
	return inv, inc, e
}

// ListInvestigations returns the investigations of one visible incident.
func (s *OperationsService) ListInvestigations(ctx context.Context, t tenant.Context, incidentID string) ([]Investigation, error) {
	if _, e := s.GetIncident(ctx, t, incidentID); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+investigationCols+` FROM fleet_partner_incident_investigations WHERE organization_id=$1 AND incident_id=$2 ORDER BY created_at`, t.OrganizationID, incidentID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Investigation{}
	for rows.Next() {
		v, e := scanInvestigation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// lockInvestigation reads an investigation for update; its incident must be
// visible in the caller's organization and branch scope.
func (s *OperationsService) lockInvestigation(ctx context.Context, tx pgx.Tx, t tenant.Context, id string) (Investigation, Incident, error) {
	v, e := scanInvestigation(tx.QueryRow(ctx, `SELECT `+investigationCols+` FROM fleet_partner_incident_investigations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return v, Incident{}, apperrors.NotFound("Investigation not found.", nil)
	}
	if e != nil {
		return v, Incident{}, e
	}
	inc, e := s.lockIncident(ctx, tx, t, v.IncidentID)
	if e != nil {
		return v, inc, apperrors.NotFound("Investigation not found.", nil)
	}
	return v, inc, nil
}

// FindingsInput carries the investigator's conclusions.
type FindingsInput struct {
	Findings, RootCause string
	ContributingFactors json.RawMessage
}

// SubmitFindings records findings and root cause. Only the assigned
// investigator may submit them.
func (s *OperationsService) SubmitFindings(ctx context.Context, t tenant.Context, investigationID string, expected int64, in FindingsInput) (Investigation, error) {
	if e := s.gate(s.o.Flags.Incidents, true); e != nil {
		return Investigation{}, e
	}
	if expected <= 0 {
		return Investigation{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if strings.TrimSpace(in.Findings) == "" || strings.TrimSpace(in.RootCause) == "" {
		return Investigation{}, apperrors.BadRequest("Findings and root cause are required.", nil)
	}
	in.ContributingFactors = safeJSON(in.ContributingFactors, "[]")
	if !jsonArray(in.ContributingFactors) {
		return Investigation{}, apperrors.BadRequest("Contributing factors must be an array.", nil)
	}
	var out Investigation
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, inc, x := s.lockInvestigation(ctx, tx, t, investigationID)
		if x != nil {
			return x
		}
		if v.InvestigatorUserID != t.OrganizationUserID {
			return apperrors.Forbidden("Only the assigned investigator can submit findings.", nil)
		}
		if v.Version != expected {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if v.Status != "OPEN" || !stageIs(inc, IncidentInvestigating) {
			return apperrors.Conflict("Findings can only be submitted for an open investigation.", nil)
		}
		out, x = scanInvestigation(tx.QueryRow(ctx, `UPDATE fleet_partner_incident_investigations SET status='FINDINGS_SUBMITTED',findings=$3,root_cause=$4,contributing_factors=$5,findings_submitted_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+investigationCols, t.OrganizationID, investigationID, strings.TrimSpace(in.Findings), strings.TrimSpace(in.RootCause), in.ContributingFactors))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "incident_investigation.findings_submitted.v1", "incident_investigation", investigationID, out.Version, map[string]any{"investigationId": investigationID, "incidentId": out.IncidentID})
	})
	return out, e
}

// ApproveFindings approves submitted findings and creates the corrective
// actions they require in one transaction (under_investigation ->
// corrective_action). The approver must not be the investigator.
func (s *OperationsService) ApproveFindings(ctx context.Context, t tenant.Context, investigationID string, expected int64, actions []CorrectiveAction) (Investigation, Incident, []CorrectiveAction, error) {
	if e := s.gate(s.o.Flags.Incidents, true); e != nil {
		return Investigation{}, Incident{}, nil, e
	}
	if expected <= 0 {
		return Investigation{}, Incident{}, nil, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if len(actions) == 0 {
		return Investigation{}, Incident{}, nil, apperrors.BadRequest("Approving findings requires at least one corrective action.", nil)
	}
	now := s.o.Now()
	for _, a := range actions {
		if e := validateAction(a, now); e != nil {
			return Investigation{}, Incident{}, nil, e
		}
	}
	var inv Investigation
	var created []CorrectiveAction
	var incidentID string
	if e := s.o.Pool.QueryRow(ctx, `SELECT incident_id FROM fleet_partner_incident_investigations WHERE organization_id=$1 AND id=$2`, t.OrganizationID, investigationID).Scan(&incidentID); e != nil {
		return inv, Incident{}, nil, apperrors.NotFound("Investigation not found.", nil)
	}
	var incidentVersion int64
	if e := s.o.Pool.QueryRow(ctx, `SELECT version FROM fleet_partner_incidents WHERE organization_id=$1 AND id=$2`, t.OrganizationID, incidentID).Scan(&incidentVersion); e != nil {
		return inv, Incident{}, nil, apperrors.NotFound("Investigation not found.", nil)
	}
	inc, e := s.transition(ctx, t, incidentID, incidentVersion, IncidentCorrectiveAction, "CORRECTIVE_ACTION_STARTED", "incident.corrective_action_started.v1", "findings approved", func(tx pgx.Tx, old Incident) (string, []any, error) {
		v, _, x := s.lockInvestigation(ctx, tx, t, investigationID)
		if x != nil {
			return "", nil, x
		}
		if v.Version != expected {
			return "", nil, apperrors.Conflict("Version conflict.", nil)
		}
		if v.InvestigatorUserID == t.OrganizationUserID {
			return "", nil, apperrors.Forbidden("The investigator cannot approve their own findings.", nil)
		}
		if v.Status != "FINDINGS_SUBMITTED" || !stageIs(old, IncidentInvestigating) {
			return "", nil, apperrors.Conflict("Only submitted findings of an incident under investigation can be approved.", nil)
		}
		inv, x = scanInvestigation(tx.QueryRow(ctx, `UPDATE fleet_partner_incident_investigations SET status='APPROVED',approved_by=$3,approved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+investigationCols, t.OrganizationID, investigationID, t.OrganizationUserID))
		if x != nil {
			return "", nil, x
		}
		if x = s.record(ctx, tx, t, "incident_investigation.findings_approved.v1", "incident_investigation", investigationID, inv.Version, map[string]any{"investigationId": investigationID, "incidentId": incidentID, "correctiveActionCount": len(actions)}); x != nil {
			return "", nil, x
		}
		for _, a := range actions {
			c, x := s.insertAction(ctx, tx, t, incidentID, a)
			if x != nil {
				return "", nil, x
			}
			created = append(created, c)
		}
		return "", nil, nil
	})
	return inv, inc, created, e
}

// ResolveIncident resolves an incident whose corrective actions are all
// verified or formally cancelled (corrective_action -> resolved).
func (s *OperationsService) ResolveIncident(ctx context.Context, t tenant.Context, id string, expected int64, action, reason string) (Incident, error) {
	return s.transition(ctx, t, id, expected, IncidentResolved, "RESOLVED", "incident.resolved.v1", reason, func(tx pgx.Tx, old Incident) (string, []any, error) {
		if !stageIs(old, IncidentCorrectiveAction) {
			return "", nil, apperrors.Conflict("Only an incident in its corrective-action stage can be resolved.", nil)
		}
		if strings.TrimSpace(action) == "" || strings.TrimSpace(reason) == "" {
			return "", nil, apperrors.BadRequest("Resolution action and reason are required.", nil)
		}
		var open, verified int
		if x := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status NOT IN ('VERIFIED','CANCELLED')), count(*) FILTER (WHERE status='VERIFIED') FROM fleet_partner_corrective_actions WHERE organization_id=$1 AND incident_id=$2`, t.OrganizationID, id).Scan(&open, &verified); x != nil {
			return "", nil, x
		}
		if open > 0 || verified == 0 {
			return "", nil, apperrors.Conflict("Every corrective action must be verified or cancelled before the incident is resolved.", nil)
		}
		return `resolution_action=$4,resolution_reason=$5`, []any{strings.TrimSpace(action), strings.TrimSpace(reason)}, nil
	})
}

// CloseIncident closes a resolved incident once its claims are settled and
// its investigations can be closed (resolved -> closed).
func (s *OperationsService) CloseIncident(ctx context.Context, t tenant.Context, id string, expected int64, reason string) (Incident, error) {
	return s.transition(ctx, t, id, expected, IncidentClosed, "CLOSED", "incident.closed.v1", reason, func(tx pgx.Tx, old Incident) (string, []any, error) {
		if old.Status != IncidentResolved {
			return "", nil, apperrors.Conflict("Only a resolved incident can be closed.", nil)
		}
		if strings.TrimSpace(reason) == "" {
			return "", nil, apperrors.BadRequest("A closure reason is required.", nil)
		}
		var openClaims int
		if x := tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_claims WHERE organization_id=$1 AND incident_id=$2 AND status NOT IN ('REJECTED','SETTLED','CLOSED','CANCELLED')`, t.OrganizationID, id).Scan(&openClaims); x != nil {
			return "", nil, x
		}
		if openClaims > 0 {
			return "", nil, apperrors.Conflict("Claims linked to this incident must be settled, rejected, closed or cancelled first.", nil)
		}
		if _, x := tx.Exec(ctx, `UPDATE fleet_partner_incident_investigations SET status='CLOSED',closed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND incident_id=$2 AND status='APPROVED'`, t.OrganizationID, id); x != nil {
			return "", nil, x
		}
		return `closure_reason=$4,closed_by=$5,closed_at=now()`, []any{strings.TrimSpace(reason), t.ActorID}, nil
	})
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
