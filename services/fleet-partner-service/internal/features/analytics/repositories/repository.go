// Package repositories persists the Section 24/54 analytics records and
// reads the tenant-scoped operational tables reports and recommendation
// models are computed from. Every statement filters by organization; report
// queries additionally honour the requester's permitted branches.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the PostgreSQL analytics store.
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
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("Resource not found.", nil)
		}
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(raw, &out)
}

func jsonList(rows pgx.Rows, err error) ([]map[string]any, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Report generation
// ---------------------------------------------------------------------------

// section is one report table: its title, the table it reads and the query.
// Every query takes ($1 organization, $2 from date, $3 to date, $4 permitted
// branch ids) and returns one JSON object per row. A section over a table
// with no time or branch dimension still binds the unused parameters with a
// tautology ("$2::date IS NOT NULL", "$4::text[] IS NOT DISTINCT FROM
// $4::text[]") so every query has the same signature; such sections are
// organization-wide aggregate counts, never individual records.
type section struct{ title, source, sql string }

const window = ` BETWEEN $2::date AND $3::date`

var reportSections = map[string][]section{
	"OPERATIONS": {
		{"Provider assignments by status", "fleet_partner_provider_assignments", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_provider_assignments WHERE organization_id=$1 AND created_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY status ORDER BY status) x`},
		{"Assignment declines by reason (safety reasons separated)", "fleet_partner_assignment_responses", `SELECT row_to_json(x) FROM (SELECT COALESCE(reason_code,'UNSPECIFIED') AS "reasonCode",safety_related AS "safetyRelated",count(*) AS count FROM fleet_partner_assignment_responses WHERE organization_id=$1 AND response='DECLINED' AND occurred_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1,2 ORDER BY 2 DESC,3 DESC) x`},
		{"Private orders by status", "fleet_partner_private_orders", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_private_orders WHERE organization_id=$1 AND created_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY status ORDER BY status) x`},
		{"Operational exceptions by severity and status", "fleet_partner_operational_exceptions", `SELECT row_to_json(x) FROM (SELECT severity,status,count(*) AS count FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND created_at::date` + window + ` AND (branch_id IS NULL OR ` + "COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1,2 ORDER BY 1,2) x`},
	},
	"FLEET": {
		{"Assets by operational status", "fleet_partner_assets", `SELECT row_to_json(x) FROM (SELECT operational_status AS "operationalStatus",count(*) AS count FROM fleet_partner_assets WHERE organization_id=$1 AND lifecycle_status='ACTIVE' AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Work orders by status", "fleet_partner_work_orders", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_work_orders WHERE organization_id=$1 AND created_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1 ORDER BY 1) x`},
		{"Open asset defects by severity", "fleet_partner_asset_defects", `SELECT row_to_json(x) FROM (SELECT severity,count(*) AS count,count(*) FILTER(WHERE work_order_id IS NULL) AS "withoutWorkOrder" FROM fleet_partner_asset_defects WHERE organization_id=$1 AND status='OPEN' AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Fuel and energy records by type and approval", "fleet_partner_fuel_records", `SELECT row_to_json(x) FROM (SELECT fuel_or_energy_type AS "fuelOrEnergyType",approval_status AS "approvalStatus",count(*) AS count,sum(quantity)::float8 AS quantity FROM fleet_partner_fuel_records WHERE organization_id=$1 AND recorded_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1,2 ORDER BY 1,2) x`},
	},
	"WAREHOUSE": {
		{"Stock by status", "fleet_partner_inventory_items", `SELECT row_to_json(x) FROM (SELECT i.status,count(*) AS lines,sum(i.quantity_projection)::float8 AS quantity,sum(i.reserved_quantity)::float8 AS reserved FROM fleet_partner_inventory_items i JOIN fleet_partner_warehouses w ON w.id=i.warehouse_id WHERE i.organization_id=$1 AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR w.branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Warehouse receipts by status", "fleet_partner_warehouse_receipts", `SELECT row_to_json(x) FROM (SELECT r.status,count(*) AS count FROM fleet_partner_warehouse_receipts r JOIN fleet_partner_warehouses w ON w.id=r.warehouse_id WHERE r.organization_id=$1 AND r.created_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR w.branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Open warehouse tasks", "fleet_partner_putaway_tasks,fleet_partner_pick_tasks", `SELECT row_to_json(x) FROM (SELECT 'PUT_AWAY' AS "taskType",t.status,count(*) AS count FROM fleet_partner_putaway_tasks t JOIN fleet_partner_warehouses w ON w.id=t.warehouse_id WHERE t.organization_id=$1 AND t.status IN('OPEN','CLAIMED') AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR w.branch_id=ANY($4::text[]))" + ` GROUP BY 1,2
 UNION ALL SELECT 'PICK',p.status,count(*) FROM fleet_partner_pick_tasks p JOIN fleet_partner_pick_waves pw ON pw.id=p.pick_wave_id JOIN fleet_partner_warehouses w ON w.id=pw.warehouse_id WHERE p.organization_id=$1 AND p.status IN('OPEN','CLAIMED') AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR w.branch_id=ANY($4::text[]))" + ` GROUP BY 1,2) x`},
	},
	"WORKFORCE": {
		{"Managed provider memberships by status", "fleet_partner_organization_provider_memberships", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1 ORDER BY 1) x`},
		{"Shifts by status", "fleet_partner_shifts", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_shifts WHERE organization_id=$1 AND start_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Training enrolments by status", "fleet_partner_training_enrollments", `SELECT row_to_json(x) FROM (SELECT status,count(*) AS count FROM fleet_partner_training_enrollments WHERE organization_id=$1 AND created_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1 ORDER BY 1) x`},
		{"Decline responses by safety classification", "fleet_partner_assignment_responses", `SELECT row_to_json(x) FROM (SELECT CASE WHEN safety_related THEN 'SAFETY' ELSE 'OTHER' END AS classification,count(*) AS count FROM fleet_partner_assignment_responses WHERE organization_id=$1 AND response='DECLINED' AND occurred_at::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1 ORDER BY 1) x`},
	},
	"FINANCE": {
		{"Invoices by status and currency", "fleet_partner_invoices", `SELECT row_to_json(x) FROM (SELECT status,currency,count(*) AS count,sum(total_minor)::bigint AS "totalMinor" FROM fleet_partner_invoices WHERE organization_id=$1 AND created_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1,2 ORDER BY 2,1) x`},
		{"Open receivables by currency", "fleet_partner_receivable_entries", `SELECT row_to_json(x) FROM (SELECT re.currency,re.status,count(*) AS count,sum(re.outstanding_amount_minor)::bigint AS "outstandingMinor" FROM fleet_partner_receivable_entries re JOIN fleet_partner_invoices i ON i.id=re.invoice_id WHERE re.organization_id=$1 AND re.status IN('OPEN','PARTIALLY_PAID','OVERDUE') AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR i.branch_id=ANY($4::text[]))" + ` GROUP BY 1,2 ORDER BY 1,2) x`},
		{"Expenses by status and currency", "fleet_partner_expenses", `SELECT row_to_json(x) FROM (SELECT status,currency,count(*) AS count,sum(amount_minor)::bigint AS "amountMinor" FROM fleet_partner_expenses WHERE organization_id=$1 AND created_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1,2 ORDER BY 2,1) x`},
	},
	"SLA": {
		{"SLA measurements by metric and breach status", "fleet_partner_sla_measurements", `SELECT row_to_json(x) FROM (SELECT metric_code AS "metricCode",breach_status AS "breachStatus",count(*) AS count FROM fleet_partner_sla_measurements WHERE organization_id=$1 AND period_end::date` + window + ` AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1,2 ORDER BY 1,2) x`},
	},
	"CUSTOMER": {
		{"Private orders per customer", "fleet_partner_private_orders", `SELECT row_to_json(x) FROM (SELECT customer_id AS "customerId",count(*) AS orders,count(*) FILTER(WHERE status='COMPLETED') AS completed,count(*) FILTER(WHERE status='CANCELLED') AS cancelled FROM fleet_partner_private_orders WHERE organization_id=$1 AND created_at::date` + window + ` AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 2 DESC LIMIT 200) x`},
	},
	"EXECUTIVE": {
		{"Live trips by status", "fleet_partner_trip_projections", `SELECT row_to_json(x) FROM (SELECT trip_status AS "tripStatus",freshness_state AS "freshnessState",count(*) AS count FROM fleet_partner_trip_projections WHERE organization_id=$1 AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND $4::text[] IS NOT DISTINCT FROM $4::text[] GROUP BY 1,2 ORDER BY 1,2) x`},
		{"Active managed providers", "fleet_partner_organization_provider_memberships", `SELECT row_to_json(x) FROM (SELECT count(*) FILTER(WHERE status='ACTIVE') AS active,count(*) AS total FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND $4::text[] IS NOT DISTINCT FROM $4::text[]) x`},
		{"Open operational exceptions", "fleet_partner_operational_exceptions", `SELECT row_to_json(x) FROM (SELECT severity,count(*) AS count FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND status IN('OPEN','ACKNOWLEDGED','ESCALATED') AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND (branch_id IS NULL OR ` + "COALESCE(cardinality($4::text[]),0)=0 OR branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
		{"Overdue receivables by currency", "fleet_partner_receivable_entries", `SELECT row_to_json(x) FROM (SELECT re.currency,count(*) AS count,sum(re.outstanding_amount_minor)::bigint AS "outstandingMinor" FROM fleet_partner_receivable_entries re JOIN fleet_partner_invoices i ON i.id=re.invoice_id WHERE re.organization_id=$1 AND re.status IN('OPEN','PARTIALLY_PAID','OVERDUE') AND re.due_at<now() AND $2::date IS NOT NULL AND $3::date IS NOT NULL AND ` + "(COALESCE(cardinality($4::text[]),0)=0 OR i.branch_id=ANY($4::text[]))" + ` GROUP BY 1 ORDER BY 1) x`},
	},
}

// GenerateReport runs every section of a report type for one organization
// and branch scope. It reads only authoritative operational tables.
func (r *Repository) GenerateReport(ctx context.Context, organizationID, reportType string, from, to time.Time, branches []string) ([]models.ReportSection, []string, error) {
	defs, ok := reportSections[reportType]
	if !ok {
		return nil, nil, apperrors.Validation("Unsupported report type.", nil)
	}
	if branches == nil {
		branches = []string{}
	}
	var sections []models.ReportSection
	var sources []string
	for _, d := range defs {
		rows, err := jsonList(r.pool.Query(ctx, d.sql, organizationID, from, to, branches))
		if err != nil {
			return nil, nil, err
		}
		sections = append(sections, models.ReportSection{Title: d.title, Rows: rows})
		sources = append(sources, d.source)
	}
	return sections, sources, nil
}

// ---------------------------------------------------------------------------
// Report definitions and runs
// ---------------------------------------------------------------------------

func (r *Repository) InsertDefinition(ctx context.Context, q database.Querier, id, organizationID string, in models.ReportDefinitionInput, filters []byte, next *time.Time, actor string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_report_definitions(id,organization_id,name,report_type,filters,schedule_cron,schedule_timezone,next_run_at,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, organizationID, in.Name, in.ReportType, filters, in.ScheduleCron, in.ScheduleTimezone, next, actor)
	return err
}

func (r *Repository) DefinitionByName(ctx context.Context, organizationID, name string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_report_definitions WHERE organization_id=$1 AND name=$2`, organizationID, name).Scan(&id)
	return id, err
}

func (r *Repository) GetDefinition(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(d) FROM fleet_partner_report_definitions d WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

func (r *Repository) ListDefinitions(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(d) FROM fleet_partner_report_definitions d WHERE organization_id=$1 ORDER BY created_at DESC,id LIMIT 500`, organizationID))
}

// DueDefinitions locks scheduled definitions whose next run is due.
func (r *Repository) DueDefinitions(ctx context.Context, q database.Querier, now time.Time) ([]map[string]any, error) {
	return jsonList(r.q(q).Query(ctx, `SELECT row_to_json(d) FROM fleet_partner_report_definitions d WHERE status='ACTIVE' AND schedule_cron<>'' AND next_run_at<=$1 ORDER BY next_run_at LIMIT 100 FOR UPDATE SKIP LOCKED`, now))
}

func (r *Repository) AdvanceSchedule(ctx context.Context, q database.Querier, id string, next time.Time) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_report_definitions SET next_run_at=$2,updated_at=now() WHERE id=$1`, id, next)
	return err
}

// InsertRun records a queued run; the trigger reference makes a scheduled
// slot or a client retry create the run once.
func (r *Repository) InsertRun(ctx context.Context, q database.Querier, id, organizationID, definitionID, reportType string, filters, permissions []byte, trigger, actor string, expires time.Time) (string, bool, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_report_runs(id,organization_id,definition_id,report_type,filter_snapshot,permission_snapshot,trigger_reference,requested_by,expires_at)VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9) ON CONFLICT(organization_id,trigger_reference) DO NOTHING`,
		id, organizationID, definitionID, reportType, filters, permissions, trigger, actor, expires)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 1 {
		return id, true, nil
	}
	err = r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_report_runs WHERE organization_id=$1 AND trigger_reference=$2`, organizationID, trigger).Scan(&id)
	return id, false, err
}

func (r *Repository) GetRun(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM fleet_partner_report_runs x WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

func (r *Repository) ListRuns(ctx context.Context, organizationID, requester string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT id,organization_id,definition_id,report_type,status,row_count,error_code,requested_by,created_at,completed_at,expires_at FROM fleet_partner_report_runs WHERE organization_id=$1 ORDER BY created_at DESC,id LIMIT 500) x`, organizationID))
}

// ClaimRun moves a queued run to RUNNING and returns its snapshot.
func (r *Repository) ClaimRun(ctx context.Context, organizationID, id string) (reportType string, filters, permissions []byte, ok bool, err error) {
	err = r.pool.QueryRow(ctx, `UPDATE fleet_partner_report_runs SET status='RUNNING',started_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('QUEUED','RUNNING') RETURNING report_type,filter_snapshot,permission_snapshot`, organizationID, id).Scan(&reportType, &filters, &permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil, false, nil
	}
	return reportType, filters, permissions, err == nil, err
}

func (r *Repository) CompleteRun(ctx context.Context, organizationID, id string, result []byte, rows int) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_report_runs SET status='COMPLETED',result=$3,row_count=$4,completed_at=now() WHERE organization_id=$1 AND id=$2 AND status='RUNNING'`, organizationID, id, result, rows)
	return err
}

func (r *Repository) FailRun(ctx context.Context, organizationID, id, code string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_report_runs SET status='FAILED',error_code=$3,completed_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('QUEUED','RUNNING')`, organizationID, id, code)
	return err
}

// ExpireRuns drops the result of every run past its expiry.
func (r *Repository) ExpireRuns(ctx context.Context, organizationID string, now time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE fleet_partner_report_runs SET status='EXPIRED',result=NULL WHERE organization_id=$1 AND status='COMPLETED' AND expires_at<=$2`, organizationID, now)
	return tag.RowsAffected(), err
}

// ---------------------------------------------------------------------------
// Recommendation model inputs (rule-based, explainable)
// ---------------------------------------------------------------------------

// MaintenanceCandidates are open HIGH/CRITICAL defects with no work order.
func (r *Repository) MaintenanceCandidates(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT d.id AS "defectId",d.asset_id AS "assetId",d.severity,d.branch_id AS "branchId",d.created_at AS "reportedAt" FROM fleet_partner_asset_defects d WHERE d.organization_id=$1 AND d.status='OPEN' AND d.severity IN('HIGH','CRITICAL') AND d.work_order_id IS NULL ORDER BY d.created_at LIMIT 200) x`, organizationID))
}

// FuelOdometerAnomalies are fuel records whose odometer is below the
// previous record of the same asset.
func (r *Repository) FuelOdometerAnomalies(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT id AS "fuelRecordId",asset_id AS "assetId",odometer::float8 AS odometer,previous::float8 AS "previousOdometer",previous_id AS "previousFuelRecordId",recorded_at AS "recordedAt" FROM (
 SELECT id,asset_id,odometer,recorded_at,lag(odometer) OVER w AS previous,lag(id) OVER w AS previous_id FROM fleet_partner_fuel_records WHERE organization_id=$1 AND odometer IS NOT NULL WINDOW w AS (PARTITION BY asset_id ORDER BY recorded_at,id)) f
 WHERE previous IS NOT NULL AND odometer<previous ORDER BY recorded_at LIMIT 200) x`, organizationID))
}

// StaleLiveTrips are live trip projections whose source data is stale.
func (r *Repository) StaleLiveTrips(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT id AS "tripProjectionId",trip_status AS "tripStatus",freshness_state AS "freshnessState",source_service AS "sourceService" FROM fleet_partner_trip_projections WHERE organization_id=$1 AND freshness_state<>'CURRENT' AND trip_status NOT IN('COMPLETED','CANCELLED','FAILED') ORDER BY id LIMIT 200) x`, organizationID))
}

// ---------------------------------------------------------------------------
// Recommendations
// ---------------------------------------------------------------------------

// InsertRecommendation records one recommendation; a live duplicate for the
// same subject and model version is left as it is.
func (r *Repository) InsertRecommendation(ctx context.Context, q database.Querier, id, organizationID string, v models.Recommendation, hash string, expires time.Time) (bool, error) {
	explanation, _ := json.Marshal(v.Explanation)
	inputs, _ := json.Marshal(v.InputReference)
	var draft []byte
	if v.ActionDraft != nil {
		draft, _ = json.Marshal(v.ActionDraft)
	}
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_recommendations(id,organization_id,recommendation_type,subject_type,subject_id,model_code,model_version,confidence,explanation,input_reference,inputs_hash,action_draft,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`, id, organizationID, v.Type, v.SubjectType, v.SubjectID, v.ModelCode, v.ModelVersion, v.Confidence, explanation, inputs, hash, draft, expires)
	return tag.RowsAffected() == 1, err
}

func (r *Repository) GetRecommendation(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM fleet_partner_recommendations x WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

func (r *Repository) ListRecommendations(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM fleet_partner_recommendations x WHERE organization_id=$1 ORDER BY (status='OPEN') DESC,created_at DESC,id LIMIT 500`, organizationID))
}

func (r *Repository) LockRecommendation(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, expires time.Time, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT status,version,expires_at FROM fleet_partner_recommendations WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version, &expires)
	return
}

func (r *Repository) DecideRecommendation(ctx context.Context, q database.Querier, organizationID, id, status, actor, reason string) (int64, error) {
	var v int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_recommendations SET status=$3,decided_by=$4,decided_at=now(),decision_reason=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, actor, reason).Scan(&v)
	return v, err
}

func (r *Repository) ExpireRecommendations(ctx context.Context, organizationID string, now time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE fleet_partner_recommendations SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='OPEN' AND expires_at<=$2`, organizationID, now)
	return tag.RowsAffected(), err
}

func (r *Repository) ListModelVersions(ctx context.Context) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM fleet_partner_model_versions x ORDER BY code,version`))
}

// ---------------------------------------------------------------------------
// Automation
// ---------------------------------------------------------------------------

func (r *Repository) InsertRule(ctx context.Context, q database.Querier, id, organizationID string, in models.AutomationRuleInput, actor string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_automation_rules(id,organization_id,name,trigger_type,trigger_config,condition_tree,action_plan,created_by)VALUES($1,$2,$3,'EVENT',$4,$5,$6,$7)`,
		id, organizationID, in.Name, in.TriggerConfig, models.Raw(in.ConditionTree, `{"all":[]}`), in.ActionPlan, actor)
	return err
}

func (r *Repository) RuleByName(ctx context.Context, organizationID, name string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_automation_rules WHERE organization_id=$1 AND name=$2`, organizationID, name).Scan(&id)
	return id, err
}

func (r *Repository) GetRule(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM fleet_partner_automation_rules x WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

func (r *Repository) ListRules(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM fleet_partner_automation_rules x WHERE organization_id=$1 ORDER BY created_at DESC,id LIMIT 500`, organizationID))
}

// LockRule returns status, version, creator and submitter under lock.
func (r *Repository) LockRule(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, creator, submitter string, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT status,version,created_by,submitted_by FROM fleet_partner_automation_rules WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version, &creator, &submitter)
	return
}

func (r *Repository) UpdateRuleDefinition(ctx context.Context, q database.Querier, organizationID, id string, in models.AutomationRuleInput) (int64, error) {
	var v int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_automation_rules SET trigger_config=$3,condition_tree=$4,action_plan=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`,
		organizationID, id, in.TriggerConfig, models.Raw(in.ConditionTree, `{"all":[]}`), in.ActionPlan).Scan(&v)
	return v, err
}

func (r *Repository) SetRuleStatus(ctx context.Context, q database.Querier, organizationID, id, status, actor string) (int64, error) {
	var v int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_automation_rules SET status=$3,
 submitted_by=CASE WHEN $3='PENDING_APPROVAL' THEN $4 ELSE submitted_by END,
 approved_by=CASE WHEN $3='ACTIVE' THEN $4 WHEN $3='DRAFT' THEN '' ELSE approved_by END,
 approved_at=CASE WHEN $3='ACTIVE' THEN now() WHEN $3='DRAFT' THEN NULL ELSE approved_at END,
 version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, actor).Scan(&v)
	return v, err
}

// InsertRuleVersion appends the rule's snapshot at a version.
func (r *Repository) InsertRuleVersion(ctx context.Context, q database.Querier, organizationID, id string, version int64, status, actor string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_automation_rule_versions(rule_id,organization_id,version,snapshot,status,actor_reference)
 SELECT id,organization_id,$3,jsonb_build_object('triggerConfig',trigger_config,'conditionTree',condition_tree,'actionPlan',action_plan,'name',name),$4,$5 FROM fleet_partner_automation_rules WHERE organization_id=$1 AND id=$2`, organizationID, id, version, status, actor)
	return err
}

// ActiveEventRules lists active rules with their definitions.
func (r *Repository) ActiveEventRules(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT id,version,trigger_config AS "triggerConfig",condition_tree AS "conditionTree",action_plan AS "actionPlan" FROM fleet_partner_automation_rules WHERE organization_id=$1 AND status='ACTIVE' AND trigger_type='EVENT') x`, organizationID))
}

// OutboxEvent is a committed domain event read by the evaluator.
type OutboxEvent struct {
	ID, Type  string
	CreatedAt time.Time
	Payload   map[string]any
}

// OutboxAfter reads an organization's committed events after a cursor, in
// commit order.
func (r *Repository) OutboxAfter(ctx context.Context, organizationID string, after time.Time, afterID string, types []string, limit int) ([]OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,event_type,created_at,payload->'payload' FROM fleet_partner_outbox WHERE organization_id=$1 AND (created_at,id)>($2,$3) AND event_type=ANY($4::text[]) ORDER BY created_at,id LIMIT $5`, organizationID, after, afterID, types, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var raw []byte
		if err = rows.Scan(&e.ID, &e.Type, &e.CreatedAt, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Cursor reads (and on first use, initializes to now) the evaluator cursor.
// A new organization's rules apply to events from activation onward, never
// retroactively.
func (r *Repository) Cursor(ctx context.Context, q database.Querier, organizationID string, now time.Time) (time.Time, string, error) {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_automation_cursors(organization_id,last_created_at,last_event_id)VALUES($1,$2,'') ON CONFLICT DO NOTHING`, organizationID, now)
	if err != nil {
		return time.Time{}, "", err
	}
	var at time.Time
	var id string
	err = r.q(q).QueryRow(ctx, `SELECT last_created_at,last_event_id FROM fleet_partner_automation_cursors WHERE organization_id=$1 FOR UPDATE`, organizationID).Scan(&at, &id)
	return at, id, err
}

func (r *Repository) AdvanceCursor(ctx context.Context, q database.Querier, organizationID string, at time.Time, id string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_automation_cursors SET last_created_at=$2,last_event_id=$3,updated_at=now() WHERE organization_id=$1`, organizationID, at, id)
	return err
}

// InsertExecution records an execution; the (rule, trigger) pair is unique
// so a replayed event never runs a rule twice.
func (r *Repository) InsertExecution(ctx context.Context, q database.Querier, id, organizationID, ruleID string, ruleVersion int64, trigger string, trace []byte, status string) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_automation_executions(id,organization_id,rule_id,rule_version,trigger_reference,status,evaluation_trace,completed_at)VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $6='SKIPPED' THEN now() END) ON CONFLICT(rule_id,trigger_reference) DO NOTHING`,
		id, organizationID, ruleID, ruleVersion, trigger, status, trace)
	return tag.RowsAffected() == 1, err
}

// ExecutionForRun returns a STARTED execution with its rule's action plan.
func (r *Repository) ExecutionForRun(ctx context.Context, q database.Querier, organizationID, id string) (status, ruleID, trigger string, plan []byte, trace []byte, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT e.status,e.rule_id,e.trigger_reference,v.snapshot->'actionPlan',e.evaluation_trace FROM fleet_partner_automation_executions e JOIN fleet_partner_automation_rule_versions v ON v.rule_id=e.rule_id AND v.version=e.rule_version WHERE e.organization_id=$1 AND e.id=$2 FOR UPDATE OF e`, organizationID, id).Scan(&status, &ruleID, &trigger, &plan, &trace)
	return
}

func (r *Repository) FinishExecution(ctx context.Context, q database.Querier, organizationID, id, status string, results []byte, code string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_automation_executions SET status=$3,action_results=$4,error_code=$5,completed_at=now() WHERE organization_id=$1 AND id=$2 AND status='STARTED'`, organizationID, id, status, results, code)
	return err
}

func (r *Repository) ListExecutions(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return jsonList(r.pool.Query(ctx, `SELECT row_to_json(x) FROM fleet_partner_automation_executions x WHERE organization_id=$1 ORDER BY started_at DESC,id LIMIT 500`, organizationID))
}

// OrganizationsWithAnalytics lists organizations that have report
// definitions, recommendations or active automation rules.
func (r *Repository) OrganizationsWithAnalytics(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT organization_id FROM fleet_partner_automation_rules WHERE status='ACTIVE' UNION SELECT organization_id FROM fleet_partner_report_runs WHERE status='COMPLETED' UNION SELECT organization_id FROM fleet_partner_recommendations WHERE status='OPEN'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
