package safety

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type OperationsEvents interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type OperationsOptions struct {
	Pool   *pgxpool.Pool
	Events OperationsEvents
	Audit  *audit.Recorder
	Media  platformclients.MediaContract
	Queue  *jobs.Queue
	Flags  OperationsFlags
	Now    func() time.Time
}
type OperationsService struct{ o OperationsOptions }

func NewOperationsService(o OperationsOptions) *OperationsService {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &OperationsService{o: o}
}
func (s *OperationsService) gate(enabled, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !enabled {
		if write {
			return apperrors.Forbidden("Capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *OperationsService) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}
func (s *OperationsService) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}
func safeJSON(v json.RawMessage, empty string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(empty)
	}
	return v
}

const reportCols = `id,organization_id,COALESCE(branch_id,''),reporter_type,reporter_reference_protected,anonymous,report_type,severity,location_reference,route_id,assignment_id,trip_reference,provider_membership_id,COALESCE(asset_id,''),description,immediate_action,status,owner_user_id,resolution_action,resolution_reason,version,created_at,updated_at,closed_at`

func scanReport(r pgx.Row) (Report, error) {
	var v Report
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.ReporterType, &v.ReporterReference, &v.Anonymous, &v.ReportType, &v.Severity, &v.LocationReference, &v.RouteID, &v.AssignmentID, &v.TripReference, &v.ProviderMembershipID, &v.AssetID, &v.Description, &v.ImmediateAction, &v.Status, &v.OwnerUserID, &v.ResolutionAction, &v.ResolutionReason, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ClosedAt)
	return v, e
}
func (s *OperationsService) CreateReport(ctx context.Context, t tenant.Context, v Report) (Report, error) {
	if e := s.gate(s.o.Flags.Reports, true); e != nil {
		return v, e
	}
	if v.BranchID != "" && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Branch not found.", nil)
	}
	v.ReportType = strings.ToUpper(strings.TrimSpace(v.ReportType))
	v.Severity = strings.ToUpper(strings.TrimSpace(v.Severity))
	if v.ReportType == "" || v.Severity == "" || strings.TrimSpace(v.Description) == "" {
		return v, apperrors.BadRequest("Report type, severity and description are required.", nil)
	}
	v.ID = ids.New("srp")
	v.OrganizationID = t.OrganizationID
	if v.ReporterType == "" {
		v.ReporterType = "ORGANIZATION_USER"
	}
	if v.ReporterReference == "" {
		v.ReporterReference = t.ActorID
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanReport(tx.QueryRow(ctx, `INSERT INTO fleet_partner_safety_reports(id,organization_id,branch_id,reporter_type,reporter_reference_protected,anonymous,report_type,severity,location_reference,route_id,assignment_id,trip_reference,provider_membership_id,asset_id,description,immediate_action)VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16) RETURNING `+reportCols, v.ID, v.OrganizationID, v.BranchID, v.ReporterType, v.ReporterReference, v.Anonymous, v.ReportType, v.Severity, v.LocationReference, v.RouteID, v.AssignmentID, v.TripReference, v.ProviderMembershipID, v.AssetID, v.Description, v.ImmediateAction))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "safety_report.created.v1", "safety_report", v.ID, v.Version, map[string]any{"reportId": v.ID, "severity": v.Severity, "anonymous": v.Anonymous})
	})
	return v, e
}
func (s *OperationsService) GetReport(ctx context.Context, t tenant.Context, id string) (Report, error) {
	if e := s.gate(s.o.Flags.Reports, false); e != nil {
		return Report{}, e
	}
	v, e := scanReport(s.o.Pool.QueryRow(ctx, `SELECT `+reportCols+` FROM fleet_partner_safety_reports WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Report not found.", nil)
	}
	return v, e
}
func (s *OperationsService) ListReports(ctx context.Context, t tenant.Context) ([]Report, error) {
	if e := s.gate(s.o.Flags.Reports, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+reportCols+` FROM fleet_partner_safety_reports WHERE organization_id=$1 AND(cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2::text[])) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		v, e := scanReport(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *OperationsService) UpdateReport(ctx context.Context, t tenant.Context, id string, expected int64, in Report, resolve bool) (Report, error) {
	if e := s.gate(s.o.Flags.Reports, true); e != nil {
		return in, e
	}
	if expected <= 0 {
		return in, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetReport(ctx, t, id)
	if e != nil {
		return in, e
	}
	status := strings.ToUpper(in.Status)
	if status == "" {
		status = old.Status
	}
	if resolve {
		status = "RESOLVED"
		if strings.TrimSpace(in.ResolutionAction) == "" || strings.TrimSpace(in.ResolutionReason) == "" {
			return in, apperrors.BadRequest("Resolution action and reason are required.", nil)
		}
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		in, x = scanReport(tx.QueryRow(ctx, `UPDATE fleet_partner_safety_reports SET status=$4,owner_user_id=COALESCE(NULLIF($5,''),owner_user_id),immediate_action=COALESCE(NULLIF($6,''),immediate_action),resolution_action=COALESCE(NULLIF($7,''),resolution_action),resolution_reason=COALESCE(NULLIF($8,''),resolution_reason),closed_at=CASE WHEN $4 IN('RESOLVED','DISMISSED') THEN now() ELSE closed_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+reportCols, t.OrganizationID, id, expected, status, in.OwnerUserID, in.ImmediateAction, in.ResolutionAction, in.ResolutionReason))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		event := "safety_report.updated.v1"
		if resolve {
			event = "safety_report.resolved.v1"
		}
		return s.record(ctx, tx, t, event, "safety_report", id, in.Version, map[string]any{"reportId": id, "status": in.Status})
	})
	return in, e
}

const incidentCols = `id,organization_id,COALESCE(branch_id,''),incident_type,severity,status,occurred_at,location_reference,route_id,assignment_id,trip_reference,people_references,asset_references,shipment_references,external_reference,description,reported_by,owner_user_id,resolution_action,resolution_reason,version,created_at,updated_at,closed_at`

func scanIncident(r pgx.Row) (Incident, error) {
	var v Incident
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.IncidentType, &v.Severity, &v.Status, &v.OccurredAt, &v.LocationReference, &v.RouteID, &v.AssignmentID, &v.TripReference, &v.PeopleReferences, &v.AssetReferences, &v.ShipmentReferences, &v.ExternalReference, &v.Description, &v.ReportedBy, &v.OwnerUserID, &v.ResolutionAction, &v.ResolutionReason, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ClosedAt)
	return v, e
}
func (s *OperationsService) CreateIncident(ctx context.Context, t tenant.Context, v Incident) (Incident, error) {
	if e := s.gate(s.o.Flags.Incidents, true); e != nil {
		return v, e
	}
	if v.BranchID != "" && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Branch not found.", nil)
	}
	v.IncidentType = strings.ToUpper(v.IncidentType)
	v.Severity = strings.ToUpper(v.Severity)
	if v.IncidentType == "" || v.Severity == "" || v.Description == "" {
		return v, apperrors.BadRequest("Incident type, severity and description are required.", nil)
	}
	if v.OccurredAt.IsZero() {
		v.OccurredAt = s.o.Now()
	}
	v.ID = ids.New("inc")
	v.OrganizationID = t.OrganizationID
	v.ReportedBy = t.ActorID
	v.PeopleReferences = safeJSON(v.PeopleReferences, "[]")
	v.AssetReferences = safeJSON(v.AssetReferences, "[]")
	v.ShipmentReferences = safeJSON(v.ShipmentReferences, "[]")
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanIncident(tx.QueryRow(ctx, `INSERT INTO fleet_partner_incidents(id,organization_id,branch_id,incident_type,severity,occurred_at,location_reference,route_id,assignment_id,trip_reference,people_references,asset_references,shipment_references,external_reference,description,reported_by,owner_user_id)VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING `+incidentCols, v.ID, v.OrganizationID, v.BranchID, v.IncidentType, v.Severity, v.OccurredAt, v.LocationReference, v.RouteID, v.AssignmentID, v.TripReference, v.PeopleReferences, v.AssetReferences, v.ShipmentReferences, v.ExternalReference, v.Description, v.ReportedBy, v.OwnerUserID))
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_incident_history(id,organization_id,incident_id,previous_status,new_status,action,actor_reference,version,snapshot)VALUES($1,$2,$3,'','OPEN','CREATED',$4,$5,$6)`, ids.New("inh"), t.OrganizationID, v.ID, t.ActorID, v.Version, mustSafetyJSON(v))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "incident.created.v1", "incident", v.ID, v.Version, map[string]any{"incidentId": v.ID, "severity": v.Severity})
	})
	return v, e
}
func (s *OperationsService) GetIncident(ctx context.Context, t tenant.Context, id string) (Incident, error) {
	if e := s.gate(s.o.Flags.Incidents, false); e != nil {
		return Incident{}, e
	}
	v, e := scanIncident(s.o.Pool.QueryRow(ctx, `SELECT `+incidentCols+` FROM fleet_partner_incidents WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Incident not found.", nil)
	}
	return v, e
}
func (s *OperationsService) ListIncidents(ctx context.Context, t tenant.Context) ([]Incident, error) {
	if e := s.gate(s.o.Flags.Incidents, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+incidentCols+` FROM fleet_partner_incidents WHERE organization_id=$1 AND(cardinality($2::text[])=0 OR branch_id IS NULL OR branch_id=ANY($2::text[])) ORDER BY occurred_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		v, e := scanIncident(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *OperationsService) UpdateIncident(ctx context.Context, t tenant.Context, id string, expected int64, in Incident) (Incident, error) {
	if expected <= 0 {
		return in, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetIncident(ctx, t, id)
	if e != nil {
		return in, e
	}
	status := strings.ToUpper(in.Status)
	if status == "" {
		status = old.Status
	}
	if status == "RESOLVED" || status == "CLOSED" {
		if in.ResolutionAction == "" || in.ResolutionReason == "" {
			return in, apperrors.BadRequest("Resolution action and reason are required.", nil)
		}
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		in, x = scanIncident(tx.QueryRow(ctx, `UPDATE fleet_partner_incidents SET status=$4,owner_user_id=COALESCE(NULLIF($5,''),owner_user_id),resolution_action=COALESCE(NULLIF($6,''),resolution_action),resolution_reason=COALESCE(NULLIF($7,''),resolution_reason),closed_at=CASE WHEN $4 IN('RESOLVED','CLOSED') THEN now() ELSE closed_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+incidentCols, t.OrganizationID, id, expected, status, in.OwnerUserID, in.ResolutionAction, in.ResolutionReason))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_incident_history(id,organization_id,incident_id,previous_status,new_status,action,actor_reference,reason,version,snapshot)VALUES($1,$2,$3,$4,$5,'UPDATED',$6,$7,$8,$9)`, ids.New("inh"), t.OrganizationID, id, old.Status, in.Status, t.ActorID, in.ResolutionReason, in.Version, mustSafetyJSON(in))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "incident.updated.v1", "incident", id, in.Version, map[string]any{"incidentId": id, "status": in.Status})
	})
	return in, e
}
func (s *OperationsService) validateMedia(ctx context.Context, t tenant.Context, id string) error {
	if id == "" {
		return apperrors.BadRequest("Media file ID is required.", nil)
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media authority is unavailable.", nil)
	}
	m, e := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
	if e != nil {
		return e
	}
	if !m.Ready() || m.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media ownership or safety validation failed.", nil)
	}
	return nil
}
func (s *OperationsService) AddEvidence(ctx context.Context, t tenant.Context, incidentID string, v Evidence) (Evidence, error) {
	if _, e := s.GetIncident(ctx, t, incidentID); e != nil {
		return v, e
	}
	if e := s.validateMedia(ctx, t, v.MediaFileID); e != nil {
		return v, e
	}
	if v.CapturedAt.IsZero() {
		v.CapturedAt = s.o.Now()
	}
	v.ID = ids.New("ine")
	v.OrganizationID = t.OrganizationID
	v.IncidentID = incidentID
	v.CapturedBy = t.ActorID
	v.HashMetadata = safeJSON(v.HashMetadata, "{}")
	v.SafeMetadata = safeJSON(v.SafeMetadata, "{}")
	e := s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_incident_evidence(id,organization_id,incident_id,media_file_id,hash_metadata,captured_by,captured_at,source_time,safe_metadata)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`, v.ID, v.OrganizationID, v.IncidentID, v.MediaFileID, v.HashMetadata, v.CapturedBy, v.CapturedAt, v.SourceTime, v.SafeMetadata).Scan(&v.CreatedAt)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "incident.evidence_attached.v1", "incident", incidentID, 1, map[string]any{"incidentId": incidentID, "evidenceId": v.ID, "mediaFileId": v.MediaFileID})
	})
	return v, e
}

const actionCols = `id,organization_id,incident_id,action_type,owner_user_id,due_at,status,verification_requirements,completion_notes,completed_at,verified_by,verified_at,version,created_at,updated_at`

func scanAction(r pgx.Row) (CorrectiveAction, error) {
	var v CorrectiveAction
	e := r.Scan(&v.ID, &v.OrganizationID, &v.IncidentID, &v.ActionType, &v.OwnerUserID, &v.DueAt, &v.Status, &v.VerificationRequirements, &v.CompletionNotes, &v.CompletedAt, &v.VerifiedBy, &v.VerifiedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *OperationsService) CreateCorrectiveAction(ctx context.Context, t tenant.Context, incidentID string, v CorrectiveAction) (CorrectiveAction, error) {
	if _, e := s.GetIncident(ctx, t, incidentID); e != nil {
		return v, e
	}
	if v.ActionType == "" || v.OwnerUserID == "" || v.DueAt.IsZero() {
		return v, apperrors.BadRequest("Action type, owner and dueAt are required.", nil)
	}
	v.ID = ids.New("coa")
	v.OrganizationID = t.OrganizationID
	v.IncidentID = incidentID
	v.VerificationRequirements = safeJSON(v.VerificationRequirements, "{}")
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanAction(tx.QueryRow(ctx, `INSERT INTO fleet_partner_corrective_actions(id,organization_id,incident_id,action_type,owner_user_id,due_at,verification_requirements)VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+actionCols, v.ID, v.OrganizationID, v.IncidentID, v.ActionType, v.OwnerUserID, v.DueAt, v.VerificationRequirements))
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `UPDATE fleet_partner_incidents SET status='CORRECTIVE_ACTION_REQUIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, incidentID)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "corrective_action.created.v1", "corrective_action", v.ID, v.Version, v)
	})
	return v, e
}
func (s *OperationsService) Action(ctx context.Context, t tenant.Context, id, command, notes string, expected int64) (CorrectiveAction, error) {
	if expected <= 0 {
		return CorrectiveAction{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var v CorrectiveAction
	status := "COMPLETED"
	if command == "VERIFY" {
		status = "VERIFIED"
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var old string
		if x := tx.QueryRow(ctx, `SELECT status FROM fleet_partner_corrective_actions WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&old); x != nil {
			return apperrors.NotFound("Corrective action not found.", x)
		}
		if command == "VERIFY" && old != "COMPLETED" {
			return apperrors.Conflict("Only completed corrective actions can be verified.", nil)
		}
		var x error
		v, x = scanAction(tx.QueryRow(ctx, `UPDATE fleet_partner_corrective_actions SET status=$4,completion_notes=CASE WHEN $4='COMPLETED' THEN $5 ELSE completion_notes END,completed_at=CASE WHEN $4='COMPLETED' THEN now() ELSE completed_at END,verified_by=CASE WHEN $4='VERIFIED' THEN $6 ELSE verified_by END,verified_at=CASE WHEN $4='VERIFIED' THEN now() ELSE verified_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+actionCols, t.OrganizationID, id, expected, status, notes, t.ActorID))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "corrective_action."+strings.ToLower(status)+".v1", "corrective_action", id, v.Version, v)
	})
	return v, e
}
func mustSafetyJSON(v any) []byte { b, _ := json.Marshal(v); return b }
