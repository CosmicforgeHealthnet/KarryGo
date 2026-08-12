package compliance

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

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

type Options struct {
	Pool     *pgxpool.Pool
	Events   EventPublisher
	Audit    *audit.Recorder
	Queue    *jobs.Queue
	Dispatch platformclients.RestrictionAuthority
	Hauling  platformclients.RestrictionAuthority
	Flags    Flags
	Now      func() time.Time
}

type Service struct{ o Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}
}

func (s *Service) gate(write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !s.o.Flags.Compliance {
		if write {
			return apperrors.Forbidden("Compliance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if err := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}

const requirementColumns = `id,organization_id,code,name,subject_type,service_type,asset_type,jurisdiction_reference,location_reference,requirement_type,required_document_type,COALESCE(required_qualification_id,''),COALESCE(required_training_course_id,''),validity_rules,severity,blocks_operation,effective_from,effective_until,status,version,created_by,created_at,updated_at`

func scanRequirement(row pgx.Row) (Requirement, error) {
	var v Requirement
	err := row.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Name, &v.SubjectType, &v.ServiceType, &v.AssetType, &v.JurisdictionReference, &v.LocationReference, &v.RequirementType, &v.RequiredDocumentType, &v.RequiredQualificationID, &v.RequiredTrainingCourseID, &v.ValidityRules, &v.Severity, &v.BlocksOperation, &v.EffectiveFrom, &v.EffectiveUntil, &v.Status, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func normalizeRequirement(v *Requirement) {
	v.Code = strings.ToUpper(strings.TrimSpace(v.Code))
	v.SubjectType = strings.ToUpper(strings.TrimSpace(v.SubjectType))
	v.RequirementType = strings.ToUpper(strings.TrimSpace(v.RequirementType))
	v.Severity = strings.ToUpper(strings.TrimSpace(v.Severity))
	if v.Status == "" {
		v.Status = "DRAFT"
	}
	if v.EffectiveFrom.IsZero() {
		v.EffectiveFrom = time.Now().UTC()
	}
	if len(v.ValidityRules) == 0 {
		v.ValidityRules = json.RawMessage(`{}`)
	}
}
func validSubject(v string) bool {
	switch v {
	case "ORGANIZATION", "PROVIDER", "ASSET", "ORDER", "SHIPMENT", "CARGO", "WAREHOUSE", "LOCATION":
		return true
	}
	return false
}

func (s *Service) CreateRequirement(ctx context.Context, t tenant.Context, v Requirement) (Requirement, error) {
	if err := s.gate(true); err != nil {
		return v, err
	}
	normalizeRequirement(&v)
	if v.Code == "" || v.Name == "" || !validSubject(v.SubjectType) || v.RequirementType == "" || v.Severity == "" {
		return v, apperrors.BadRequest("Code, name, subject type, requirement type and severity are required.", nil)
	}
	v.ID = ids.New("cmpreq")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = scanRequirement(tx.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_requirements(id,organization_id,code,name,subject_type,service_type,asset_type,jurisdiction_reference,location_reference,requirement_type,required_document_type,required_qualification_id,required_training_course_id,validity_rules,severity,blocks_operation,effective_from,effective_until,status,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,''),$14,$15,$16,$17,$18,'DRAFT',$19) RETURNING `+requirementColumns, v.ID, t.OrganizationID, v.Code, v.Name, v.SubjectType, v.ServiceType, v.AssetType, v.JurisdictionReference, v.LocationReference, v.RequirementType, v.RequiredDocumentType, v.RequiredQualificationID, v.RequiredTrainingCourseID, v.ValidityRules, v.Severity, v.BlocksOperation, v.EffectiveFrom, v.EffectiveUntil, v.CreatedBy))
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "compliance_requirement.created.v1", "compliance_requirement", v.ID, v.Version, v)
	})
	return v, err
}
func (s *Service) GetRequirement(ctx context.Context, t tenant.Context, id string) (Requirement, error) {
	if err := s.gate(false); err != nil {
		return Requirement{}, err
	}
	v, e := scanRequirement(s.o.Pool.QueryRow(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Requirement not found.", nil)
	}
	return v, e
}
func (s *Service) ListRequirements(ctx context.Context, t tenant.Context) ([]Requirement, error) {
	if err := s.gate(false); err != nil {
		return nil, err
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Requirement{}
	for rows.Next() {
		v, e := scanRequirement(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) UpdateRequirement(ctx context.Context, t tenant.Context, id string, expected int64, in Requirement, action string) (Requirement, error) {
	if err := s.gate(true); err != nil {
		return in, err
	}
	if expected <= 0 {
		return in, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetRequirement(ctx, t, id)
	if e != nil {
		return in, e
	}
	status := old.Status
	blocksOperation := in.BlocksOperation
	event := "compliance_requirement.updated.v1"
	switch action {
	case "ACTIVATE":
		status = "ACTIVE"
		blocksOperation = old.BlocksOperation
		event = "compliance_requirement.activated.v1"
	case "DEACTIVATE":
		status = "INACTIVE"
		blocksOperation = old.BlocksOperation
		event = "compliance_requirement.deactivated.v1"
	}
	if old.Status == "ARCHIVED" {
		return in, apperrors.Conflict("Archived requirements cannot be changed.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		in, x = scanRequirement(tx.QueryRow(ctx, `UPDATE fleet_partner_compliance_requirements SET name=COALESCE(NULLIF($4,''),name),service_type=COALESCE(NULLIF($5,''),service_type),asset_type=COALESCE(NULLIF($6,''),asset_type),jurisdiction_reference=COALESCE(NULLIF($7,''),jurisdiction_reference),location_reference=COALESCE(NULLIF($8,''),location_reference),validity_rules=CASE WHEN $9::jsonb='{}'::jsonb THEN validity_rules ELSE $9 END,severity=COALESCE(NULLIF($10,''),severity),blocks_operation=$11,effective_until=COALESCE($12,effective_until),status=$13,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+requirementColumns, t.OrganizationID, id, expected, in.Name, in.ServiceType, in.AssetType, in.JurisdictionReference, in.LocationReference, orJSON(in.ValidityRules), strings.ToUpper(in.Severity), blocksOperation, in.EffectiveUntil, status))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "compliance_requirement", id, in.Version, in)
	})
	return in, e
}

const statusColumns = `id,organization_id,subject_type,subject_reference,requirement_id,state,due_at,expires_at,source_type,source_reference,source_version,evaluated_at,reason_codes,restriction_reference,version,created_at,updated_at`

func scanStatus(row pgx.Row) (Status, error) {
	var v Status
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.RequirementID, &v.State, &v.DueAt, &v.ExpiresAt, &v.SourceType, &v.SourceReference, &v.SourceVersion, &v.EvaluatedAt, &v.ReasonCodes, &v.RestrictionReference, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (s *Service) evidence(ctx context.Context, org string, r Requirement, subject string) (string, []string, *time.Time) {
	now := s.o.Now().UTC()
	var ok bool
	var expires *time.Time
	switch {
	case r.RequiredQualificationID != "" && r.SubjectType == "PROVIDER":
		err := s.o.Pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2 AND qualification_id=$3 AND status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY expires_at DESC NULLS FIRST LIMIT 1`, org, subject, r.RequiredQualificationID, now).Scan(&expires)
		ok = err == nil
	case r.RequiredTrainingCourseID != "" && r.SubjectType == "PROVIDER":
		err := s.o.Pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_training_completions WHERE organization_id=$1 AND membership_id=$2 AND course_id=$3 AND status='PASSED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY completed_at DESC LIMIT 1`, org, subject, r.RequiredTrainingCourseID, now).Scan(&expires)
		ok = err == nil
	case r.RequiredDocumentType != "" && r.SubjectType == "ASSET":
		err := s.o.Pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2 AND document_type=$3 AND verification_status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY expires_at DESC NULLS FIRST LIMIT 1`, org, subject, r.RequiredDocumentType, now).Scan(&expires)
		ok = err == nil
	case r.SubjectType == "ORGANIZATION":
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organizations WHERE id=$1 AND status='ACTIVE')`, org).Scan(&ok)
	default:
		return StateUnknown, []string{"AUTHORITATIVE_EVIDENCE_UNAVAILABLE"}, nil
	}
	if !ok {
		return StateNonCompliant, []string{"REQUIRED_EVIDENCE_MISSING"}, expires
	}
	if expires != nil && expires.Before(now.Add(30*24*time.Hour)) {
		return StateDueSoon, []string{"EVIDENCE_DUE_SOON"}, expires
	}
	return StateCompliant, []string{"REQUIREMENT_SATISFIED"}, expires
}

func (s *Service) Evaluate(ctx context.Context, t tenant.Context, in EvaluateInput) (Status, error) {
	if err := s.gate(true); err != nil {
		return Status{}, err
	}
	if in.SubjectReference == "" || in.RequirementID == "" || in.SourceType == "" || in.SourceVersion < 0 {
		return Status{}, apperrors.BadRequest("Subject, requirement, source and sourceVersion are required.", nil)
	}
	r, e := s.GetRequirement(ctx, t, in.RequirementID)
	if e != nil {
		return Status{}, e
	}
	if !r.ActiveAt(s.o.Now()) {
		return Status{}, apperrors.Conflict("Requirement is not active.", nil)
	}
	if strings.ToUpper(in.SubjectType) != r.SubjectType {
		return Status{}, apperrors.BadRequest("Subject type does not match requirement.", nil)
	}
	state, reasons, expires := s.evidence(ctx, t.OrganizationID, r, in.SubjectReference)
	if state == StateNonCompliant && r.BlocksOperation {
		state = StateRestricted
	}
	reasonJSON, _ := json.Marshal(reasons)
	out := Status{ID: ids.New("cmpst"), OrganizationID: t.OrganizationID, SubjectType: r.SubjectType, SubjectReference: in.SubjectReference, RequirementID: r.ID, State: state, ExpiresAt: expires, SourceType: strings.ToUpper(in.SourceType), SourceReference: in.SourceReference, SourceVersion: in.SourceVersion, EvaluatedAt: s.o.Now().UTC(), ReasonCodes: reasonJSON}
	var restrictionID string
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var oldState string
		var oldVersion, oldSource int64
		err := tx.QueryRow(ctx, `SELECT state,version,source_version FROM fleet_partner_compliance_statuses WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND requirement_id=$4 FOR UPDATE`, t.OrganizationID, out.SubjectType, out.SubjectReference, out.RequirementID).Scan(&oldState, &oldVersion, &oldSource)
		_ = oldVersion
		if err == nil && in.SourceVersion < oldSource {
			return apperrors.Conflict("A newer evidence version has already been evaluated.", nil)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_statuses(id,organization_id,subject_type,subject_reference,requirement_id,state,expires_at,source_type,source_reference,source_version,evaluated_at,reason_codes)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+statusColumns, out.ID, out.OrganizationID, out.SubjectType, out.SubjectReference, out.RequirementID, out.State, out.ExpiresAt, out.SourceType, out.SourceReference, out.SourceVersion, out.EvaluatedAt, out.ReasonCodes).Scan(&out.ID, &out.OrganizationID, &out.SubjectType, &out.SubjectReference, &out.RequirementID, &out.State, &out.DueAt, &out.ExpiresAt, &out.SourceType, &out.SourceReference, &out.SourceVersion, &out.EvaluatedAt, &out.ReasonCodes, &out.RestrictionReference, &out.Version, &out.CreatedAt, &out.UpdatedAt)
		} else {
			out.ID = ""
			out, err = scanStatus(tx.QueryRow(ctx, `UPDATE fleet_partner_compliance_statuses SET state=$5,expires_at=$6,source_type=$7,source_reference=$8,source_version=$9,evaluated_at=$10,reason_codes=$11,version=version+1,updated_at=now() WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND requirement_id=$4 RETURNING `+statusColumns, t.OrganizationID, out.SubjectType, out.SubjectReference, out.RequirementID, out.State, out.ExpiresAt, out.SourceType, out.SourceReference, out.SourceVersion, out.EvaluatedAt, out.ReasonCodes))
		}
		if err != nil {
			return err
		}
		if out.State == StateRestricted {
			rr, er := s.createRestrictionTx(ctx, tx, t, RestrictInput{SubjectType: out.SubjectType, SubjectReference: out.SubjectReference, ReasonCode: "COMPLIANCE_" + r.Code, SourceType: "COMPLIANCE_STATUS", SourceReference: out.ID, Severity: r.Severity})
			if er != nil {
				return er
			}
			restrictionID = rr.ID
			out.RestrictionReference = rr.ID
			_, er = tx.Exec(ctx, `UPDATE fleet_partner_compliance_statuses SET restriction_reference=$1 WHERE id=$2`, rr.ID, out.ID)
			if er != nil {
				return er
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_compliance_status_history(id,organization_id,compliance_status_id,previous_state,new_state,source_type,source_reference,source_version,reason_codes,restriction_reference,snapshot,actor_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, ids.New("cmph"), t.OrganizationID, out.ID, oldState, out.State, out.SourceType, out.SourceReference, out.SourceVersion, out.ReasonCodes, out.RestrictionReference, mustJSON(out), t.ActorID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "compliance_status.evaluated.v1", "compliance_status", out.ID, out.Version, out)
	})
	if e == nil && restrictionID != "" {
		_ = s.ProcessRestriction(ctx, t.OrganizationID, restrictionID)
	}
	return out, e
}

const restrictionColumns = `id,organization_id,subject_type,subject_reference,reason_code,source_type,source_reference,severity,state,required_authorities,effective_at,cleared_at,version,created_by,cleared_by,clearance_reason,created_at,updated_at`

func scanRestriction(row pgx.Row) (Restriction, error) {
	var v Restriction
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.ReasonCode, &v.SourceType, &v.SourceReference, &v.Severity, &v.State, &v.RequiredAuthorities, &v.EffectiveAt, &v.ClearedAt, &v.Version, &v.CreatedBy, &v.ClearedBy, &v.ClearanceReason, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) createRestrictionTx(ctx context.Context, tx pgx.Tx, t tenant.Context, in RestrictInput) (Restriction, error) {
	auth := RequiredAuthorities(strings.ToUpper(in.SubjectType))
	raw, _ := json.Marshal(auth)
	v := Restriction{ID: ids.New("rst"), OrganizationID: t.OrganizationID, SubjectType: strings.ToUpper(in.SubjectType), SubjectReference: in.SubjectReference, ReasonCode: strings.ToUpper(in.ReasonCode), SourceType: in.SourceType, SourceReference: in.SourceReference, Severity: strings.ToUpper(in.Severity), State: "APPLIED", RequiredAuthorities: raw, CreatedBy: t.ActorID}
	var e error
	v, e = scanRestriction(tx.QueryRow(ctx, `INSERT INTO fleet_partner_restrictions(id,organization_id,subject_type,subject_reference,reason_code,source_type,source_reference,severity,required_authorities,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id,subject_type,subject_reference,source_type,source_reference) DO UPDATE SET reason_code=excluded.reason_code,severity=excluded.severity,state='APPLIED',cleared_at=NULL,version=fleet_partner_restrictions.version+1,updated_at=now() RETURNING `+restrictionColumns, v.ID, v.OrganizationID, v.SubjectType, v.SubjectReference, v.ReasonCode, v.SourceType, v.SourceReference, v.Severity, v.RequiredAuthorities, v.CreatedBy))
	if e != nil {
		return v, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_history(id,organization_id,restriction_id,previous_state,new_state,reason_code,actor_reference,version,snapshot)VALUES($1,$2,$3,'', 'APPLIED',$4,$5,$6,$7)`, ids.New("rsh"), v.OrganizationID, v.ID, v.ReasonCode, t.ActorID, v.Version, mustJSON(v))
	if e != nil {
		return v, e
	}
	for _, a := range auth {
		_, e = tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_operations(id,organization_id,restriction_id,authority_service,action,idempotency_key,expected_version)VALUES($1,$2,$3,$4,'APPLY',$5,$6) ON CONFLICT DO NOTHING`, ids.New("rso"), v.OrganizationID, v.ID, a, v.ID+":apply", v.Version)
		if e != nil {
			return v, e
		}
	}
	if e = s.record(ctx, tx, t, "restriction.applied.v1", "restriction", v.ID, v.Version, v); e != nil {
		return v, e
	}
	if len(auth) > 0 && s.o.Events != nil {
		if e = s.o.Events.Publish(ctx, tx, "restriction.propagation_required.v1", "restriction", v.ID, v.Version, v.OrganizationID, t.CorrelationID, map[string]any{"restrictionId": v.ID, "action": "APPLY", "authorities": auth}); e != nil {
			return v, e
		}
	}
	return v, nil
}
func (s *Service) ApplyRestriction(ctx context.Context, t tenant.Context, in RestrictInput) (Restriction, error) {
	if !s.o.Flags.Restrictions {
		return Restriction{}, apperrors.Forbidden("Restriction propagation is disabled.", nil)
	}
	if in.SubjectReference == "" || in.ReasonCode == "" || in.SourceReference == "" {
		return Restriction{}, apperrors.BadRequest("Subject, reason and source are required.", nil)
	}
	var v Restriction
	e := s.tx(ctx, func(tx pgx.Tx) error { var x error; v, x = s.createRestrictionTx(ctx, tx, t, in); return x })
	if e == nil {
		e = s.ProcessRestriction(ctx, t.OrganizationID, v.ID)
	}
	return v, e
}
func (s *Service) GetRestriction(ctx context.Context, org, id string) (Restriction, error) {
	v, e := scanRestriction(s.o.Pool.QueryRow(ctx, `SELECT `+restrictionColumns+` FROM fleet_partner_restrictions WHERE organization_id=$1 AND id=$2`, org, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Restriction not found.", nil)
	}
	return v, e
}
func (s *Service) ClearRestriction(ctx context.Context, t tenant.Context, id, reason string, expected int64) (Restriction, error) {
	if !s.o.Flags.Restrictions {
		return Restriction{}, apperrors.Forbidden("Restriction propagation is disabled.", nil)
	}
	if expected <= 0 || strings.TrimSpace(reason) == "" {
		return Restriction{}, apperrors.BadRequest("expectedVersion and clearance reason are required.", nil)
	}
	var v Restriction
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanRestriction(tx.QueryRow(ctx, `UPDATE fleet_partner_restrictions SET state='CLEAR_PENDING',cleared_by=$4,clearance_reason=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND state='APPLIED' RETURNING `+restrictionColumns, t.OrganizationID, id, expected, t.ActorID, reason))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict or restriction is not active.", nil)
		}
		if x != nil {
			return x
		}
		var auth []string
		_ = json.Unmarshal(v.RequiredAuthorities, &auth)
		for _, a := range auth {
			_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_operations(id,organization_id,restriction_id,authority_service,action,idempotency_key,expected_version)VALUES($1,$2,$3,$4,'CLEAR',$5,$6) ON CONFLICT DO NOTHING`, ids.New("rso"), t.OrganizationID, id, a, id+":clear", v.Version)
			if x != nil {
				return x
			}
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_history(id,organization_id,restriction_id,previous_state,new_state,reason_code,actor_reference,version,snapshot)VALUES($1,$2,$3,'APPLIED','CLEAR_PENDING',$4,$5,$6,$7)`, ids.New("rsh"), t.OrganizationID, id, reason, t.ActorID, v.Version, mustJSON(v))
		if x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "restriction.clear_requested.v1", "restriction", id, v.Version, v); x != nil {
			return x
		}
		if len(auth) > 0 && s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "restriction.propagation_required.v1", "restriction", id, v.Version, t.OrganizationID, t.CorrelationID, map[string]any{"restrictionId": id, "action": "CLEAR", "authorities": auth})
		}
		return nil
	})
	if e == nil {
		e = s.ProcessRestriction(ctx, t.OrganizationID, id)
		v, _ = s.GetRestriction(ctx, t.OrganizationID, id)
	}
	return v, e
}
func (s *Service) CheckSubject(ctx context.Context, org, typ, ref string) (Eligibility, error) {
	var count int
	err := s.o.Pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_restrictions WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND state<>'CLEARED'`, org, strings.ToUpper(typ), ref).Scan(&count)
	if err != nil {
		return Eligibility{}, err
	}
	v := Eligibility{Eligible: count == 0}
	if count > 0 {
		v.HardBlocks = []string{"COMPLIANCE_RESTRICTION_ACTIVE"}
	}
	return v, nil
}
func (s *Service) ListStatuses(ctx context.Context, t tenant.Context, typ, ref string) ([]Status, error) {
	rows, e := s.o.Pool.Query(ctx, `SELECT `+statusColumns+` FROM fleet_partner_compliance_statuses WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3) ORDER BY evaluated_at DESC`, t.OrganizationID, strings.ToUpper(typ), ref)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Status{}
	for rows.Next() {
		v, e := scanStatus(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func orJSON(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(`{}`)
	}
	return v
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func clientOutcome(err error) platformclients.Outcome {
	var pe *platformclients.Error
	if errors.As(err, &pe) {
		return pe.Outcome
	}
	return platformclients.OutcomePermanent
}
func (s *Service) authority(name string) platformclients.RestrictionAuthority {
	if name == "DISPATCH" {
		return s.o.Dispatch
	}
	if name == "HAULING" {
		return s.o.Hauling
	}
	return nil
}
func (s *Service) ProcessRestriction(ctx context.Context, org, id string) error {
	r, e := s.GetRestriction(ctx, org, id)
	if e != nil {
		return e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,authority_service,action,idempotency_key,expected_version,attempts,status FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND restriction_id=$2 AND status IN('PENDING','OUTCOME_UNKNOWN','RETRY_SCHEDULED') ORDER BY created_at`, org, id)
	if e != nil {
		return e
	}
	defer rows.Close()
	type op struct {
		id, a, action, key, status string
		version                    int64
		attempt                    int
	}
	var ops []op
	for rows.Next() {
		var v op
		if e = rows.Scan(&v.id, &v.a, &v.action, &v.key, &v.version, &v.attempt, &v.status); e != nil {
			return e
		}
		ops = append(ops, v)
	}
	for _, o := range ops {
		c := s.authority(o.a)
		if c == nil || !c.Configured() {
			return apperrors.Unavailable("Required restriction authority is unavailable.", nil)
		}
		meta := platformclients.CallMeta{OrganizationID: org, IdempotencyKey: o.key}
		cmd := platformclients.RestrictionCommand{RestrictionID: r.ID, OrganizationID: org, SubjectType: r.SubjectType, SubjectReference: r.SubjectReference, ReasonCode: r.ReasonCode, SourceReference: r.SourceReference, ExpectedVersion: o.version}
		started := s.o.Now()
		var out platformclients.RestrictionOutcome
		var callErr error
		if o.status == "OUTCOME_UNKNOWN" {
			if o.action == "APPLY" {
				out, callErr = c.Read(ctx, meta, o.key)
			} else {
				out, callErr = c.ReadClear(ctx, meta, o.key)
			}
			if callErr == nil && ((o.action == "APPLY" && out.State != "APPLIED") || (o.action == "CLEAR" && out.State != "CLEARED")) {
				callErr = errors.New("restriction read-back state mismatch")
			}
		}
		// An ambiguous mutation is never resubmitted merely because its read-back
		// failed. Only an authoritative NOT_FOUND proves the original operation
		// did not land and makes an idempotent resubmission safe.
		shouldSubmit := o.status != "OUTCOME_UNKNOWN"
		if o.status == "OUTCOME_UNKNOWN" && clientOutcome(callErr) == platformclients.OutcomeNotFound {
			shouldSubmit = true
		}
		if shouldSubmit {
			if o.action == "APPLY" {
				out, callErr = c.Apply(ctx, meta, cmd)
			} else {
				out, callErr = c.Clear(ctx, meta, cmd)
			}
		}
		outcome := "CONFIRMED"
		status := "CONFIRMED"
		code := ""
		if callErr != nil {
			oc := clientOutcome(callErr)
			outcome = string(oc)
			status = "RETRY_SCHEDULED"
			if oc == platformclients.OutcomeUnknown {
				status = "OUTCOME_UNKNOWN"
			}
			code = string(oc)
		}
		attempt := o.attempt + 1
		e = s.tx(ctx, func(tx pgx.Tx) error {
			if _, x := tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_attempts(id,organization_id,operation_id,attempt_number,request_action,outcome,error_code,authority_version,started_at,completed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("rsa"), org, o.id, attempt, o.action, outcome, code, out.SourceVersion, started, s.o.Now()); x != nil {
				return x
			}
			if _, x := tx.Exec(ctx, `UPDATE fleet_partner_restriction_operations SET status=$2,attempts=$3::integer,last_error_code=$4,authority_version=$5,confirmed_at=CASE WHEN $2='CONFIRMED' THEN now() ELSE NULL END,next_attempt_at=now()+make_interval(secs=>LEAST(600,power(2::numeric,$3::numeric)::integer)),version=version+1,updated_at=now() WHERE id=$1`, o.id, status, attempt, code, out.SourceVersion); x != nil {
				return x
			}
			if callErr == nil && s.o.Events != nil {
				return s.o.Events.Publish(ctx, tx, "restriction.propagation_confirmed.v1", "restriction", r.ID, r.Version, org, "", map[string]any{"restrictionId": r.ID, "authority": o.a, "action": o.action, "authorityVersion": out.SourceVersion})
			}
			return nil
		})
		if e != nil {
			return e
		}
		if callErr != nil {
			_, _ = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobRestrictionReconcile, DedupeKey: o.id, Payload: map[string]string{"organizationId": org, "restrictionId": id}, MaxAttempts: 8})
			continue
		}
	}
	var pending int
	_ = s.o.Pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_restriction_operations WHERE restriction_id=$1 AND status<>'CONFIRMED'`, id).Scan(&pending)
	if pending == 0 && r.State == "CLEAR_PENDING" {
		e = s.tx(ctx, func(tx pgx.Tx) error {
			cleared, x := scanRestriction(tx.QueryRow(ctx, `UPDATE fleet_partner_restrictions SET state='CLEARED',cleared_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND state='CLEAR_PENDING' RETURNING `+restrictionColumns, org, id))
			if errors.Is(x, pgx.ErrNoRows) {
				return nil
			}
			if x != nil {
				return x
			}
			actor := cleared.ClearedBy
			if actor == "" {
				actor = "restriction-reconciliation-worker"
			}
			if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_restriction_history(id,organization_id,restriction_id,previous_state,new_state,reason_code,actor_reference,version,snapshot)VALUES($1,$2,$3,'CLEAR_PENDING','CLEARED',$4,$5,$6,$7)`, ids.New("rsh"), org, id, cleared.ClearanceReason, actor, cleared.Version, mustJSON(cleared)); x != nil {
				return x
			}
			return s.record(ctx, tx, tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: actor}, "restriction.cleared.v1", "restriction", id, cleared.Version, cleared)
		})
	}
	return e
}
