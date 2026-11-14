// Package repositories owns every compliance SQL statement. Methods that take
// a database.Querier join the caller's transaction; the rest read from the
// pool. Missing scoped rows are reported as models.ErrNotFound.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Repository is the PostgreSQL compliance store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ErrNotFound
	}
	return err
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

const requirementColumns = `id,organization_id,code,name,subject_type,service_type,asset_type,jurisdiction_reference,location_reference,requirement_type,required_document_type,COALESCE(required_qualification_id,''),COALESCE(required_training_course_id,''),validity_rules,severity,blocks_operation,effective_from,effective_until,status,version,created_by,created_at,updated_at`

func scanRequirement(row pgx.Row) (models.Requirement, error) {
	var v models.Requirement
	err := row.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Name, &v.SubjectType, &v.ServiceType, &v.AssetType, &v.JurisdictionReference, &v.LocationReference, &v.RequirementType, &v.RequiredDocumentType, &v.RequiredQualificationID, &v.RequiredTrainingCourseID, &v.ValidityRules, &v.Severity, &v.BlocksOperation, &v.EffectiveFrom, &v.EffectiveUntil, &v.Status, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// CreateRequirement inserts a DRAFT requirement.
func (r *Repository) CreateRequirement(ctx context.Context, q database.Querier, v models.Requirement) (models.Requirement, error) {
	return scanRequirement(q.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_requirements(id,organization_id,code,name,subject_type,service_type,asset_type,jurisdiction_reference,location_reference,requirement_type,required_document_type,required_qualification_id,required_training_course_id,validity_rules,severity,blocks_operation,effective_from,effective_until,status,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,''),$14,$15,$16,$17,$18,'DRAFT',$19) RETURNING `+requirementColumns, v.ID, v.OrganizationID, v.Code, v.Name, v.SubjectType, v.ServiceType, v.AssetType, v.JurisdictionReference, v.LocationReference, v.RequirementType, v.RequiredDocumentType, v.RequiredQualificationID, v.RequiredTrainingCourseID, v.ValidityRules, v.Severity, v.BlocksOperation, v.EffectiveFrom, v.EffectiveUntil, v.CreatedBy))
}

// GetRequirement reads one requirement of the organization.
func (r *Repository) GetRequirement(ctx context.Context, org, id string) (models.Requirement, error) {
	v, e := scanRequirement(r.pool.QueryRow(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 AND id=$2`, org, id))
	return v, notFound(e)
}

func collectRequirements(rows pgx.Rows, err error) ([]models.Requirement, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Requirement{}
	for rows.Next() {
		v, e := scanRequirement(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListRequirements lists the organization's requirements, newest first.
func (r *Repository) ListRequirements(ctx context.Context, org string) ([]models.Requirement, error) {
	return collectRequirements(r.pool.Query(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 ORDER BY created_at DESC`, org))
}

// ListActiveRequirements lists the ACTIVE requirements for one subject type.
func (r *Repository) ListActiveRequirements(ctx context.Context, q database.Querier, org, subjectType string) ([]models.Requirement, error) {
	return collectRequirements(q.Query(ctx, `SELECT `+requirementColumns+` FROM fleet_partner_compliance_requirements WHERE organization_id=$1 AND subject_type=$2 AND status='ACTIVE' ORDER BY code,id`, org, subjectType))
}

// UpdateRequirement applies a versioned edit or status change. A stale
// version is models.ErrNotFound.
func (r *Repository) UpdateRequirement(ctx context.Context, q database.Querier, org, id string, expected int64, in models.Requirement, validityRules json.RawMessage, severity string, blocksOperation bool, status string) (models.Requirement, error) {
	v, e := scanRequirement(q.QueryRow(ctx, `UPDATE fleet_partner_compliance_requirements SET name=COALESCE(NULLIF($4,''),name),service_type=COALESCE(NULLIF($5,''),service_type),asset_type=COALESCE(NULLIF($6,''),asset_type),jurisdiction_reference=COALESCE(NULLIF($7,''),jurisdiction_reference),location_reference=COALESCE(NULLIF($8,''),location_reference),validity_rules=CASE WHEN $9::jsonb='{}'::jsonb THEN validity_rules ELSE $9 END,severity=COALESCE(NULLIF($10,''),severity),blocks_operation=$11,effective_until=COALESCE($12,effective_until),status=$13,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+requirementColumns, org, id, expected, in.Name, in.ServiceType, in.AssetType, in.JurisdictionReference, in.LocationReference, validityRules, severity, blocksOperation, in.EffectiveUntil, status))
	return v, notFound(e)
}

// QualificationEvidence returns the expiry of the provider's verified
// qualification, and whether one exists.
func (r *Repository) QualificationEvidence(ctx context.Context, org, membership, qualification string, now time.Time) (*time.Time, bool) {
	var expires *time.Time
	err := r.pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2 AND qualification_id=$3 AND status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY expires_at DESC NULLS FIRST LIMIT 1`, org, membership, qualification, now).Scan(&expires)
	return expires, err == nil
}

// TrainingEvidence returns the expiry of the provider's passed training
// completion, and whether one exists.
func (r *Repository) TrainingEvidence(ctx context.Context, org, membership, course string, now time.Time) (*time.Time, bool) {
	var expires *time.Time
	err := r.pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_training_completions WHERE organization_id=$1 AND membership_id=$2 AND course_id=$3 AND status='PASSED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY completed_at DESC LIMIT 1`, org, membership, course, now).Scan(&expires)
	return expires, err == nil
}

// AssetDocumentEvidence returns the expiry of the asset's verified document
// of the type, and whether one exists.
func (r *Repository) AssetDocumentEvidence(ctx context.Context, org, asset, documentType string, now time.Time) (*time.Time, bool) {
	var expires *time.Time
	err := r.pool.QueryRow(ctx, `SELECT expires_at FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2 AND document_type=$3 AND verification_status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4) ORDER BY expires_at DESC NULLS FIRST LIMIT 1`, org, asset, documentType, now).Scan(&expires)
	return expires, err == nil
}

// OrganizationActive reports whether the organization is ACTIVE.
func (r *Repository) OrganizationActive(ctx context.Context, org string) bool {
	var ok bool
	_ = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organizations WHERE id=$1 AND status='ACTIVE')`, org).Scan(&ok)
	return ok
}

const statusColumns = `id,organization_id,subject_type,subject_reference,requirement_id,state,due_at,expires_at,source_type,source_reference,source_version,evaluated_at,reason_codes,restriction_reference,version,created_at,updated_at`

func scanStatus(row pgx.Row) (models.Status, error) {
	var v models.Status
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.RequirementID, &v.State, &v.DueAt, &v.ExpiresAt, &v.SourceType, &v.SourceReference, &v.SourceVersion, &v.EvaluatedAt, &v.ReasonCodes, &v.RestrictionReference, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// LockStatus locks the current status of one subject/requirement pair and
// returns its state and evidence source version.
func (r *Repository) LockStatus(ctx context.Context, q database.Querier, org, subjectType, subjectReference, requirementID string) (string, int64, error) {
	var state string
	var version, source int64
	err := q.QueryRow(ctx, `SELECT state,version,source_version FROM fleet_partner_compliance_statuses WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND requirement_id=$4 FOR UPDATE`, org, subjectType, subjectReference, requirementID).Scan(&state, &version, &source)
	return state, source, notFound(err)
}

// InsertStatus records the first evaluation of a subject/requirement pair.
func (r *Repository) InsertStatus(ctx context.Context, q database.Querier, v models.Status) (models.Status, error) {
	return scanStatus(q.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_statuses(id,organization_id,subject_type,subject_reference,requirement_id,state,expires_at,source_type,source_reference,source_version,evaluated_at,reason_codes)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+statusColumns, v.ID, v.OrganizationID, v.SubjectType, v.SubjectReference, v.RequirementID, v.State, v.ExpiresAt, v.SourceType, v.SourceReference, v.SourceVersion, v.EvaluatedAt, v.ReasonCodes))
}

// UpdateStatus records a re-evaluation of a subject/requirement pair.
func (r *Repository) UpdateStatus(ctx context.Context, q database.Querier, v models.Status) (models.Status, error) {
	return scanStatus(q.QueryRow(ctx, `UPDATE fleet_partner_compliance_statuses SET state=$5,expires_at=$6,source_type=$7,source_reference=$8,source_version=$9,evaluated_at=$10,reason_codes=$11,version=version+1,updated_at=now() WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND requirement_id=$4 RETURNING `+statusColumns, v.OrganizationID, v.SubjectType, v.SubjectReference, v.RequirementID, v.State, v.ExpiresAt, v.SourceType, v.SourceReference, v.SourceVersion, v.EvaluatedAt, v.ReasonCodes))
}

// SetStatusRestriction links a status to the restriction it applied.
func (r *Repository) SetStatusRestriction(ctx context.Context, q database.Querier, statusID, restrictionID string) error {
	_, e := q.Exec(ctx, `UPDATE fleet_partner_compliance_statuses SET restriction_reference=$1 WHERE id=$2`, restrictionID, statusID)
	return e
}

// InsertStatusHistory appends one status transition.
func (r *Repository) InsertStatusHistory(ctx context.Context, q database.Querier, previousState string, v models.Status, actor string) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_compliance_status_history(id,organization_id,compliance_status_id,previous_state,new_state,source_type,source_reference,source_version,reason_codes,restriction_reference,snapshot,actor_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, ids.New("cmph"), v.OrganizationID, v.ID, previousState, v.State, v.SourceType, v.SourceReference, v.SourceVersion, v.ReasonCodes, v.RestrictionReference, mustJSON(v), actor)
	return e
}

// ListStatuses lists statuses, optionally for one subject, newest first.
func (r *Repository) ListStatuses(ctx context.Context, org, subjectType, subjectReference string) ([]models.Status, error) {
	rows, e := r.pool.Query(ctx, `SELECT `+statusColumns+` FROM fleet_partner_compliance_statuses WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3) ORDER BY evaluated_at DESC`, org, subjectType, subjectReference)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Status{}
	for rows.Next() {
		v, e := scanStatus(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SweepStatuses moves expired and soon-expiring statuses forward.
func (r *Repository) SweepStatuses(ctx context.Context, org string) error {
	_, e := r.pool.Exec(ctx, `UPDATE fleet_partner_compliance_statuses SET state=CASE WHEN expires_at<=now() THEN 'EXPIRED' WHEN expires_at<=now()+interval '30 days' AND state='COMPLIANT' THEN 'DUE_SOON' ELSE state END,version=version+1,updated_at=now() WHERE organization_id=$1 AND expires_at IS NOT NULL AND((expires_at<=now() AND state NOT IN('EXPIRED','RESTRICTED'))OR(expires_at<=now()+interval '30 days' AND state='COMPLIANT'))`, org)
	return e
}

const restrictionColumns = `id,organization_id,subject_type,subject_reference,reason_code,source_type,source_reference,severity,state,required_authorities,effective_at,cleared_at,version,created_by,cleared_by,clearance_reason,created_at,updated_at`

func scanRestriction(row pgx.Row) (models.Restriction, error) {
	var v models.Restriction
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.ReasonCode, &v.SourceType, &v.SourceReference, &v.Severity, &v.State, &v.RequiredAuthorities, &v.EffectiveAt, &v.ClearedAt, &v.Version, &v.CreatedBy, &v.ClearedBy, &v.ClearanceReason, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// UpsertRestriction applies (or re-applies) a restriction for its source.
func (r *Repository) UpsertRestriction(ctx context.Context, q database.Querier, v models.Restriction) (models.Restriction, error) {
	return scanRestriction(q.QueryRow(ctx, `INSERT INTO fleet_partner_restrictions(id,organization_id,subject_type,subject_reference,reason_code,source_type,source_reference,severity,required_authorities,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id,subject_type,subject_reference,source_type,source_reference) DO UPDATE SET reason_code=excluded.reason_code,severity=excluded.severity,state='APPLIED',cleared_at=NULL,version=fleet_partner_restrictions.version+1,updated_at=now() RETURNING `+restrictionColumns, v.ID, v.OrganizationID, v.SubjectType, v.SubjectReference, v.ReasonCode, v.SourceType, v.SourceReference, v.Severity, v.RequiredAuthorities, v.CreatedBy))
}

// InsertRestrictionHistory appends one restriction transition with a snapshot.
func (r *Repository) InsertRestrictionHistory(ctx context.Context, q database.Querier, org, restrictionID, from, to, reasonCode, actor string, version int64, snapshot any) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_restriction_history(id,organization_id,restriction_id,previous_state,new_state,reason_code,actor_reference,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("rsh"), org, restrictionID, from, to, reasonCode, actor, version, mustJSON(snapshot))
	return e
}

// InsertRestrictionOperation queues one propagation to one authority.
func (r *Repository) InsertRestrictionOperation(ctx context.Context, q database.Querier, org, restrictionID, authority, action, key string, version int64) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_restriction_operations(id,organization_id,restriction_id,authority_service,action,idempotency_key,expected_version)VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, ids.New("rso"), org, restrictionID, authority, action, key, version)
	return e
}

// GetRestriction reads one restriction of the organization.
func (r *Repository) GetRestriction(ctx context.Context, org, id string) (models.Restriction, error) {
	v, e := scanRestriction(r.pool.QueryRow(ctx, `SELECT `+restrictionColumns+` FROM fleet_partner_restrictions WHERE organization_id=$1 AND id=$2`, org, id))
	return v, notFound(e)
}

// RequestClear moves an APPLIED restriction to CLEAR_PENDING. A stale version
// or inactive restriction is models.ErrNotFound.
func (r *Repository) RequestClear(ctx context.Context, q database.Querier, org, id string, expected int64, actor, reason string) (models.Restriction, error) {
	v, e := scanRestriction(q.QueryRow(ctx, `UPDATE fleet_partner_restrictions SET state='CLEAR_PENDING',cleared_by=$4,clearance_reason=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND state='APPLIED' RETURNING `+restrictionColumns, org, id, expected, actor, reason))
	return v, notFound(e)
}

// MarkCleared completes a CLEAR_PENDING restriction. Already cleared is
// models.ErrNotFound.
func (r *Repository) MarkCleared(ctx context.Context, q database.Querier, org, id string) (models.Restriction, error) {
	v, e := scanRestriction(q.QueryRow(ctx, `UPDATE fleet_partner_restrictions SET state='CLEARED',cleared_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND state='CLEAR_PENDING' RETURNING `+restrictionColumns, org, id))
	return v, notFound(e)
}

// CountActiveRestrictions counts uncleared restrictions on one subject.
func (r *Repository) CountActiveRestrictions(ctx context.Context, org, subjectType, subjectReference string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_restrictions WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND state<>'CLEARED'`, org, subjectType, subjectReference).Scan(&count)
	return count, err
}

// ListRestrictions lists restrictions, optionally for one subject or state.
func (r *Repository) ListRestrictions(ctx context.Context, org, subjectType, subjectReference, state string) ([]models.Restriction, error) {
	rows, e := r.pool.Query(ctx, `SELECT `+restrictionColumns+` FROM fleet_partner_restrictions WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3)AND($4=''OR state=$4) ORDER BY effective_at DESC,id DESC LIMIT 200`, org, subjectType, subjectReference, state)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Restriction{}
	for rows.Next() {
		v, e := scanRestriction(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// PendingOperations lists a restriction's unconfirmed propagations in order.
func (r *Repository) PendingOperations(ctx context.Context, org, restrictionID string) ([]models.RestrictionOperation, error) {
	rows, e := r.pool.Query(ctx, `SELECT id,authority_service,action,idempotency_key,expected_version,attempts,status FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND restriction_id=$2 AND status IN('PENDING','OUTCOME_UNKNOWN','RETRY_SCHEDULED') ORDER BY created_at`, org, restrictionID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var ops []models.RestrictionOperation
	for rows.Next() {
		var v models.RestrictionOperation
		if e = rows.Scan(&v.ID, &v.Authority, &v.Action, &v.IdempotencyKey, &v.ExpectedVersion, &v.Attempts, &v.Status); e != nil {
			return nil, e
		}
		ops = append(ops, v)
	}
	return ops, nil
}

// RecordOperationAttempt appends an attempt and moves the operation to its
// new status with capped exponential backoff.
func (r *Repository) RecordOperationAttempt(ctx context.Context, q database.Querier, org string, op models.RestrictionOperation, attempt int, outcome, status, code string, authorityVersion int64, started, completed time.Time) error {
	if _, x := q.Exec(ctx, `INSERT INTO fleet_partner_restriction_attempts(id,organization_id,operation_id,attempt_number,request_action,outcome,error_code,authority_version,started_at,completed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("rsa"), org, op.ID, attempt, op.Action, outcome, code, authorityVersion, started, completed); x != nil {
		return x
	}
	_, x := q.Exec(ctx, `UPDATE fleet_partner_restriction_operations SET status=$2,attempts=$3::integer,last_error_code=$4,authority_version=$5,confirmed_at=CASE WHEN $2='CONFIRMED' THEN now() ELSE NULL END,next_attempt_at=now()+make_interval(secs=>LEAST(600,power(2::numeric,$3::numeric)::integer)),version=version+1,updated_at=now() WHERE id=$1`, op.ID, status, attempt, code, authorityVersion)
	return x
}

// CountUnconfirmedOperations counts a restriction's unconfirmed operations.
func (r *Repository) CountUnconfirmedOperations(ctx context.Context, restrictionID string) int {
	var pending int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_restriction_operations WHERE restriction_id=$1 AND status<>'CONFIRMED'`, restrictionID).Scan(&pending)
	return pending
}

// ListPropagation reads every propagation of a restriction.
func (r *Repository) ListPropagation(ctx context.Context, org, restrictionID string) ([]models.Propagation, error) {
	rows, e := r.pool.Query(ctx, `SELECT authority_service,action,status,attempts,last_error_code,next_attempt_at,confirmed_at,authority_version FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND restriction_id=$2 ORDER BY created_at,authority_service`, org, restrictionID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Propagation{}
	for rows.Next() {
		var p models.Propagation
		if e = rows.Scan(&p.Authority, &p.Action, &p.Status, &p.Attempts, &p.LastErrorCode, &p.NextAttemptAt, &p.ConfirmedAt, &p.AuthorityVersion); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DueRestrictionIDs lists restrictions with propagation work due now.
func (r *Repository) DueRestrictionIDs(ctx context.Context, org string) ([]string, error) {
	rows, e := r.pool.Query(ctx, `SELECT DISTINCT restriction_id FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND status IN('PENDING','OUTCOME_UNKNOWN','RETRY_SCHEDULED') AND next_attempt_at<=now()`, org)
	if e != nil {
		return nil, e
	}
	return collectStrings(rows)
}

// ActiveOrganizations lists every ACTIVE organization for the sweep.
func (r *Repository) ActiveOrganizations(ctx context.Context) ([]string, error) {
	rows, e := r.pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if e != nil {
		return nil, e
	}
	return collectStrings(rows)
}

func collectStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if e := rows.Scan(&v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SubjectBranch resolves the branch of an organization-owned subject. A
// subject outside the organization is models.ErrNotFound; a subject type
// without an owning record here reports ok=false.
func (r *Repository) SubjectBranch(ctx context.Context, q database.Querier, org, subjectType, ref string) (string, bool, error) {
	var branch string
	var err error
	switch subjectType {
	case "PROVIDER":
		err = q.QueryRow(ctx, `SELECT COALESCE(home_branch_id,'') FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2`, org, ref).Scan(&branch)
	case "ASSET":
		err = q.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, org, ref).Scan(&branch)
	case "ORDER":
		err = q.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_private_orders WHERE organization_id=$1 AND id=$2`, org, ref).Scan(&branch)
	case "SHIPMENT":
		err = q.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_shipments WHERE organization_id=$1 AND id=$2`, org, ref).Scan(&branch)
	default:
		return "", false, nil
	}
	return branch, true, notFound(err)
}

const checkColumns = `id,organization_id,subject_type,subject_reference,source_type,source_reference,source_version,overall_state,governing_requirement_id,requirement_count,results,restriction_references,requested_by,correlation_id,created_at`

func scanCheck(row pgx.Row) (models.Check, error) {
	var v models.Check
	var results, restrictions []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.SubjectType, &v.SubjectReference, &v.SourceType, &v.SourceReference, &v.SourceVersion, &v.OverallState, &v.GoverningRequirementID, &v.RequirementCount, &results, &restrictions, &v.RequestedBy, &v.CorrelationID, &v.CreatedAt)
	if e != nil {
		return v, e
	}
	v.Results, v.RestrictionReferences = []models.CheckResult{}, []string{}
	if e = json.Unmarshal(results, &v.Results); e != nil {
		return v, e
	}
	return v, json.Unmarshal(restrictions, &v.RestrictionReferences)
}

// InsertCheck records a completed check and returns its creation time.
func (r *Repository) InsertCheck(ctx context.Context, q database.Querier, v models.Check) (time.Time, error) {
	results, _ := json.Marshal(v.Results)
	restrictions, _ := json.Marshal(v.RestrictionReferences)
	var created time.Time
	err := q.QueryRow(ctx, `INSERT INTO fleet_partner_compliance_checks(id,organization_id,subject_type,subject_reference,source_type,source_reference,source_version,overall_state,governing_requirement_id,requirement_count,results,restriction_references,requested_by,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING created_at`, v.ID, v.OrganizationID, v.SubjectType, v.SubjectReference, v.SourceType, v.SourceReference, v.SourceVersion, v.OverallState, v.GoverningRequirementID, v.RequirementCount, results, restrictions, v.RequestedBy, v.CorrelationID).Scan(&created)
	return created, err
}

// GetCheck reads one compliance check of the organization.
func (r *Repository) GetCheck(ctx context.Context, org, id string) (models.Check, error) {
	v, e := scanCheck(r.pool.QueryRow(ctx, `SELECT `+checkColumns+` FROM fleet_partner_compliance_checks WHERE organization_id=$1 AND id=$2`, org, id))
	return v, notFound(e)
}

// ListChecks lists checks, newest first, optionally for one subject.
func (r *Repository) ListChecks(ctx context.Context, org, subjectType, subjectReference string) ([]models.Check, error) {
	rows, e := r.pool.Query(ctx, `SELECT `+checkColumns+` FROM fleet_partner_compliance_checks WHERE organization_id=$1 AND($2=''OR subject_type=$2)AND($3=''OR subject_reference=$3) ORDER BY created_at DESC,id DESC LIMIT 200`, org, subjectType, subjectReference)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Check{}
	for rows.Next() {
		v, e := scanCheck(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
