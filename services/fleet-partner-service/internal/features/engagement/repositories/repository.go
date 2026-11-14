// Package repositories persists the Section 23 engagement records: support
// case projections, quality cases, SLA measurements and escalations. Every
// statement is tenant-scoped by organization ID. Methods taking a
// database.Querier run inside the caller's transaction; nil means the pool.
package repositories

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the PostgreSQL engagement store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

// WithinTx runs f in one transaction.
func (r *Repository) WithinTx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, r.pool, f)
}

func jsonRow(row pgx.Row) (map[string]any, error) {
	var raw []byte
	if e := row.Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("Resource not found.", nil)
		}
		return nil, e
	}
	var out map[string]any
	if e := json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out, nil
}

func jsonList(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if e := rows.Scan(&raw); e != nil {
			return nil, e
		}
		var v map[string]any
		if e := json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ActiveOrganizations lists the organizations the maintenance pass covers.
func (r *Repository) ActiveOrganizations(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// ---------------------------------------------------------------------------
// Support case projections
// ---------------------------------------------------------------------------

// SupportCaseByCreateKey reads the projection created under an idempotency key.
func (r *Repository) SupportCaseByCreateKey(ctx context.Context, organizationID, key string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 AND create_idempotency_key=$2`, organizationID, key))
}

// InsertCreatedSupportCase projects a case Fleet just created upstream.
func (r *Repository) InsertCreatedSupportCase(ctx context.Context, q database.Querier, id, organizationID string, in models.SupportCreateInput, p models.SupportProjection, key string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,customer_reference,provider_reference,trip_reference,order_reference,assignment_reference,owner_reference,source_version,last_synced_at,create_idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),$14) ON CONFLICT(organization_id,create_idempotency_key) WHERE create_idempotency_key<>'' DO NOTHING`, id, organizationID, p.CaseID, in.CaseType, p.Status, p.Priority, in.CustomerReference, in.ProviderReference, in.TripReference, in.OrderReference, in.AssignmentReference, p.OwnerReference, p.SourceVersion, key)
	return err
}

// GetSupportCase reads a projection by local or upstream ID.
func (r *Repository) GetSupportCase(ctx context.Context, organizationID, caseID string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 AND (id=$2 OR support_service_case_id=$2)`, organizationID, caseID))
}

// ListSupportCases lists the organization's projections, newest first.
func (r *Repository) ListSupportCases(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(p) FROM fleet_partner_support_case_projections p WHERE organization_id=$1 ORDER BY updated_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockSupportCase reads a projection's ID and source version FOR UPDATE.
func (r *Repository) LockSupportCase(ctx context.Context, q database.Querier, organizationID, caseID string) (id string, sourceVersion int64, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT id,source_version FROM fleet_partner_support_case_projections WHERE organization_id=$1 AND support_service_case_id=$2 FOR UPDATE`, organizationID, caseID).Scan(&id, &sourceVersion)
	return id, sourceVersion, err
}

// InsertEventSupportCase projects a platform case first seen through an event.
func (r *Repository) InsertEventSupportCase(ctx context.Context, q database.Querier, id string, in models.SupportEvent) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,source_version,last_synced_at,gap_status) VALUES($1,$2,$3,'PLATFORM',$4,$5,$6::bigint,now(),CASE WHEN $6::bigint=1 THEN 'CURRENT' ELSE 'GAP_DETECTED' END)`, id, in.OrganizationID, in.CaseID, in.Status, in.Priority, in.SourceVersion)
	return err
}

// InsertSupportInbox records a support event in the inbox.
func (r *Repository) InsertSupportInbox(ctx context.Context, q database.Querier, in models.SupportEvent, result string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_support_case_events_inbox(id,organization_id,event_id,support_service_case_id,source_version,event_type,safe_payload,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_id) DO NOTHING`, ids.New("sei"), in.OrganizationID, in.EventID, in.CaseID, in.SourceVersion, in.EventType, in.SafePayload, result)
	return err
}

// ApplySupportEvent writes an event's state onto the projection.
func (r *Repository) ApplySupportEvent(ctx context.Context, q database.Querier, in models.SupportEvent, gap string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_support_case_projections SET status=$3,priority=$4,owner_reference=$5,source_version=$6,last_synced_at=now(),gap_status=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND support_service_case_id=$2`, in.OrganizationID, in.CaseID, in.Status, in.Priority, in.OwnerReference, in.SourceVersion, gap)
	return err
}

// SupportCasesNeedingReconcile lists projections with a gap or unknown state.
func (r *Repository) SupportCasesNeedingReconcile(ctx context.Context, organizationID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT support_service_case_id FROM fleet_partner_support_case_projections WHERE organization_id=$1 AND gap_status IN('GAP_DETECTED','RECONCILING','UNKNOWN') ORDER BY updated_at LIMIT 50`, organizationID)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// ReconcileSupportCase writes the authoritative read-back onto a projection
// unless the projection is already newer.
func (r *Repository) ReconcileSupportCase(ctx context.Context, organizationID, caseID string, p models.SupportProjection) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_support_case_projections SET status=$3,priority=$4,owner_reference=$5,source_version=$6,last_synced_at=now(),gap_status='CURRENT',version=version+1,updated_at=now() WHERE organization_id=$1 AND support_service_case_id=$2 AND source_version<=$6`, organizationID, caseID, p.Status, p.Priority, p.OwnerReference, p.SourceVersion)
	return err
}

// ---------------------------------------------------------------------------
// Quality cases
// ---------------------------------------------------------------------------

// InsertQualityCase creates a quality case.
func (r *Repository) InsertQualityCase(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.QualityInput) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_quality_cases(id,organization_id,branch_id,category,severity,source_type,source_reference,customer_reference,provider_reference,owner_user_id,created_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)`, id, organizationID, in.BranchID, in.Category, in.Severity, in.SourceType, in.SourceReference, in.CustomerReference, in.ProviderReference, in.OwnerUserID, actorID)
	return err
}

// GetQualityCase reads one quality case.
func (r *Repository) GetQualityCase(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(q) FROM fleet_partner_quality_cases q WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListQualityCases lists quality cases in the permitted branches.
func (r *Repository) ListQualityCases(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(q) FROM fleet_partner_quality_cases q WHERE organization_id=$1 AND (COALESCE(cardinality($2::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($2)) ORDER BY created_at DESC`, organizationID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockQualityCase reads a quality case's status and version FOR UPDATE.
func (r *Repository) LockQualityCase(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_quality_cases WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, err
}

// UpdateQualityCase applies an edit and returns the new version.
func (r *Repository) UpdateQualityCase(ctx context.Context, q database.Querier, organizationID, id string, in models.QualityInput, severity, next, actorID string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_quality_cases SET category=CASE WHEN $3='' THEN category ELSE $3 END,severity=CASE WHEN $4='' THEN severity ELSE $4 END,owner_user_id=CASE WHEN $5='' THEN owner_user_id ELSE $5 END,status=$6,root_cause=CASE WHEN $7='' THEN root_cause ELSE $7 END,corrective_action_summary=CASE WHEN $8='' THEN corrective_action_summary ELSE $8 END,resolved_by=CASE WHEN $6='RESOLVED' THEN $9 ELSE resolved_by END,resolved_at=CASE WHEN $6='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, in.Category, severity, in.OwnerUserID, next, in.RootCause, in.CorrectiveActionSummary, actorID).Scan(&nv)
	return nv, err
}

// InsertQualityHistory appends a quality-case transition.
func (r *Repository) InsertQualityHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, in models.QualityInput, actorID string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_quality_case_history(id,organization_id,quality_case_id,previous_status,new_status,root_cause,corrective_action_summary,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("qhh"), organizationID, id, previous, next, in.RootCause, in.CorrectiveActionSummary, actorID, version)
	return err
}

// OverdueSevereQualityCases lists open high-severity cases older than a day.
func (r *Repository) OverdueSevereQualityCases(ctx context.Context, organizationID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM fleet_partner_quality_cases WHERE organization_id=$1 AND status IN('OPEN','UNDER_REVIEW','ACTION_REQUIRED') AND severity IN('HIGH','CRITICAL') AND created_at<now()-interval '24 hours' ORDER BY created_at LIMIT 100`, organizationID)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// ---------------------------------------------------------------------------
// SLA measurements
// ---------------------------------------------------------------------------

// InsertMeasurement stores a measurement; an existing one for the same
// agreement, metric, period and formula is kept and its ID returned.
func (r *Repository) InsertMeasurement(ctx context.Context, q database.Querier, id string, in models.MeasurementInput, measured any, status string) (string, bool, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_sla_measurements(id,organization_id,agreement_reference,contract_id,customer_id,metric_code,formula_version,period_start,period_end,target_value,measured_value,unit,breach_status,source_inputs,source_versions,generated_at) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now()) ON CONFLICT(organization_id,agreement_reference,metric_code,period_start,period_end,formula_version) DO NOTHING`, id, in.OrganizationID, in.AgreementReference, in.ContractID, in.CustomerID, in.MetricCode, in.FormulaVersion, in.PeriodStart, in.PeriodEnd, in.TargetValue, measured, in.Unit, status, models.RawOr(in.SourceInputs, "{}"), models.RawOr(in.SourceVersions, "{}"))
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() > 0 {
		return id, true, nil
	}
	err = r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_sla_measurements WHERE organization_id=$1 AND agreement_reference=$2 AND metric_code=$3 AND period_start=$4 AND period_end=$5 AND formula_version=$6`, in.OrganizationID, in.AgreementReference, in.MetricCode, in.PeriodStart, in.PeriodEnd, in.FormulaVersion).Scan(&id)
	return id, false, err
}

// GetMeasurement reads one measurement.
func (r *Repository) GetMeasurement(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListMeasurements lists measurements, newest first.
func (r *Repository) ListMeasurements(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 ORDER BY generated_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LatestSLAObservations reads the latest observation of every active SLA.
func (r *Repository) LatestSLAObservations(ctx context.Context, organizationID string) ([]models.SLAObservation, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id,a.contract_id,a.customer_id,a.metric_code,a.target_value::text,a.measurement_unit,a.version,o.window_start,o.window_end,COALESCE(o.measured_value::text,''),o.outcome,o.source_type,o.source_reference,o.source_version FROM fleet_partner_service_level_agreements a JOIN LATERAL(SELECT * FROM fleet_partner_sla_observations x WHERE x.organization_id=a.organization_id AND x.sla_id=a.id ORDER BY x.measured_at DESC LIMIT 1)o ON true WHERE a.organization_id=$1 AND a.status='ACTIVE'`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SLAObservation
	for rows.Next() {
		var x models.SLAObservation
		if err := rows.Scan(&x.Agreement, &x.Contract, &x.Customer, &x.Metric, &x.Target, &x.Unit, &x.FormulaVersion, &x.Start, &x.End, &x.Measured, &x.Outcome, &x.SourceType, &x.SourceReference, &x.SourceVersion); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Escalations
// ---------------------------------------------------------------------------

// InsertEscalation creates an escalation; an existing one for the same source
// and level is kept and its ID returned.
func (r *Repository) InsertEscalation(ctx context.Context, q database.Querier, id, organizationID string, in models.EscalationInput) (string, bool, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_escalations(id,organization_id,source_type,source_reference,escalation_level,recipient_type,recipient_reference,due_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(organization_id,source_type,source_reference,escalation_level) DO NOTHING`, id, organizationID, in.SourceType, in.SourceReference, in.Level, in.RecipientType, in.RecipientReference, in.DueAt)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() > 0 {
		return id, true, nil
	}
	err = r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_escalations WHERE organization_id=$1 AND source_type=$2 AND source_reference=$3 AND escalation_level=$4`, organizationID, in.SourceType, in.SourceReference, in.Level).Scan(&id)
	return id, false, err
}

// InsertBreachEscalation raises the first-level escalation for a breached
// SLA measurement.
func (r *Repository) InsertBreachEscalation(ctx context.Context, q database.Querier, id, organizationID, measurementID string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_escalations(id,organization_id,source_type,source_reference,escalation_level,recipient_type,recipient_reference,due_at) VALUES($1,$2,'SLA_MEASUREMENT',$3,1,'ORGANIZATION_ROLE','OPERATIONS',now()) ON CONFLICT DO NOTHING`, id, organizationID, measurementID)
	return err
}

// GetEscalation reads one escalation.
func (r *Repository) GetEscalation(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(e) FROM fleet_partner_escalations e WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListEscalations lists escalations by due time.
func (r *Repository) ListEscalations(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(e) FROM fleet_partner_escalations e WHERE organization_id=$1 ORDER BY due_at`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockEscalation reads an escalation's status and version FOR UPDATE.
func (r *Repository) LockEscalation(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_escalations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, err
}

// UpdateEscalationStatus moves an escalation to next and returns the version.
func (r *Repository) UpdateEscalationStatus(ctx context.Context, q database.Querier, organizationID, id, next string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_escalations SET status=$3,acknowledged_at=CASE WHEN $3='ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,resolved_at=CASE WHEN $3='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, next).Scan(&nv)
	return nv, err
}

// InsertEscalationHistory appends an escalation transition.
func (r *Repository) InsertEscalationHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actorID string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_escalation_history(id,organization_id,escalation_id,previous_status,new_status,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("esh"), organizationID, id, previous, next, actorID, version)
	return err
}

// DueEscalations lists pending escalations whose due time has passed.
func (r *Repository) DueEscalations(ctx context.Context, organizationID string) ([]models.DueEscalation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,recipient_type,recipient_reference,version FROM fleet_partner_escalations WHERE organization_id=$1 AND status='PENDING' AND due_at<=now() ORDER BY due_at LIMIT 50`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DueEscalation
	for rows.Next() {
		var x models.DueEscalation
		if err := rows.Scan(&x.ID, &x.RecipientType, &x.RecipientReference, &x.Version); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

// MarkEscalationNotified records the notification intent of a due escalation.
func (r *Repository) MarkEscalationNotified(ctx context.Context, q database.Querier, organizationID, id, intentID string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_escalations SET status='NOTIFIED',notification_operation_reference=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='PENDING'`, organizationID, id, intentID)
	return err
}
