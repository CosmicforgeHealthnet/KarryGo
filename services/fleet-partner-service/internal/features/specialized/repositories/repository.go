// Package repositories owns every Section 20 specialized-logistics SQL
// statement. Methods that take a database.Querier join the caller's
// transaction. Missing scoped rows are reported as models.ErrNotFound.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Repository is the PostgreSQL specialized-logistics store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ErrNotFound
	}
	return err
}

const capCols = `id,organization_id,capability,status,configuration,enabled_at,disabled_at,version,created_at,updated_at`

func scanCapability(r pgx.Row) (models.Capability, error) {
	var v models.Capability
	e := r.Scan(&v.ID, &v.OrganizationID, &v.CapabilityCode, &v.Status, &v.Configuration, &v.EnabledAt, &v.DisabledAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, notFound(e)
}

// EnabledCapability reads the organization's ENABLED capability row.
func (r *Repository) EnabledCapability(ctx context.Context, org, code string) (models.Capability, error) {
	return scanCapability(r.pool.QueryRow(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 AND capability=$2 AND status='ENABLED'`, org, code))
}

// LockCapability locks the organization's capability row in any status.
func (r *Repository) LockCapability(ctx context.Context, q database.Querier, org, code string) (models.Capability, error) {
	return scanCapability(q.QueryRow(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 AND capability=$2 FOR UPDATE`, org, code))
}

// InsertCapability creates the organization's capability row.
func (r *Repository) InsertCapability(ctx context.Context, q database.Querier, org, code, status string, configuration json.RawMessage) (models.Capability, error) {
	return scanCapability(q.QueryRow(ctx, `INSERT INTO fleet_partner_organization_capabilities(id,organization_id,capability,status,configuration,enabled_at,disabled_at)VALUES($1,$2,$3,$4,$5,CASE WHEN $4='ENABLED' THEN now() END,CASE WHEN $4='DISABLED' THEN now() END) RETURNING `+capCols, ids.New("cap"), org, code, status, configuration))
}

// UpdateCapability switches a capability at the expected version.
func (r *Repository) UpdateCapability(ctx context.Context, q database.Querier, org, code string, expected int64, status string, configuration json.RawMessage) (models.Capability, error) {
	return scanCapability(q.QueryRow(ctx, `UPDATE fleet_partner_organization_capabilities SET status=$4,configuration=$5,enabled_at=CASE WHEN $4='ENABLED' THEN now() ELSE enabled_at END,disabled_at=CASE WHEN $4='DISABLED' THEN now() ELSE NULL END,version=version+1,updated_at=now() WHERE organization_id=$1 AND capability=$2 AND version=$3 RETURNING `+capCols, org, code, expected, status, configuration))
}

// InsertCapabilityHistory appends one capability transition.
func (r *Repository) InsertCapabilityHistory(ctx context.Context, q database.Querier, org string, v models.Capability, previous, actor, reason string) error {
	snapshot, _ := json.Marshal(v)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_capability_history(id,organization_id,capability_id,previous_status,new_status,actor_reference,reason,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("caph"), org, v.ID, previous, v.Status, actor, reason, v.Version, snapshot)
	return e
}

// ListCapabilities lists every capability row of the organization.
func (r *Repository) ListCapabilities(ctx context.Context, org string) ([]models.Capability, error) {
	rows, e := r.pool.Query(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 ORDER BY capability`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Capability{}
	for rows.Next() {
		v, e := scanCapability(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// EnabledOrganizations lists organizations with at least one enabled module.
func (r *Repository) EnabledOrganizations(ctx context.Context) ([]string, error) {
	rows, e := r.pool.Query(ctx, `SELECT organization_id FROM fleet_partner_organization_capabilities WHERE status='ENABLED' GROUP BY organization_id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var org string
		if e = rows.Scan(&org); e != nil {
			return nil, e
		}
		out = append(out, org)
	}
	return out, rows.Err()
}

// AssetExists reports whether the asset belongs to the organization.
func (r *Repository) AssetExists(ctx context.Context, org, id string) (bool, error) {
	var ok bool
	e := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2)`, org, id).Scan(&ok)
	return ok, e
}

// QualificationActive reports whether the qualification is an active
// qualification of the organization.
func (r *Repository) QualificationActive(ctx context.Context, org, id string) (bool, error) {
	var ok bool
	e := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_qualifications WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, id).Scan(&ok)
	return ok, e
}

const recordCols = `id,organization_id,COALESCE(branch_id,''),capability_code,record_type,subject_type,subject_reference,status,restricted_visibility,payload,external_financial_reference,source_authority,source_reference,idempotency_key,version,created_by,created_at,updated_at,completed_at,schema_version`

func scanRecord(r pgx.Row) (models.Record, error) {
	var v models.Record
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.CapabilityCode, &v.RecordType, &v.SubjectType, &v.SubjectReference, &v.Status, &v.RestrictedVisibility, &v.Payload, &v.ExternalFinancialReference, &v.SourceAuthority, &v.SourceReference, &v.IdempotencyKey, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt, &v.SchemaVersion)
	if e == nil {
		v.SchemaID = models.SchemaID(v.CapabilityCode, v.SchemaVersion)
	}
	return v, notFound(e)
}

func collectRecords(rows pgx.Rows, err error) ([]models.Record, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Record{}
	for rows.Next() {
		v, e := scanRecord(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RecordByIdempotencyKey reads the record created under an idempotency key.
func (r *Repository) RecordByIdempotencyKey(ctx context.Context, org, code, key string) (models.Record, error) {
	return scanRecord(r.pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND idempotency_key=$3`, org, code, key))
}

// InsertRecord stores a validated record.
func (r *Repository) InsertRecord(ctx context.Context, q database.Querier, v models.Record) (models.Record, error) {
	return scanRecord(q.QueryRow(ctx, `INSERT INTO fleet_partner_specialized_records(id,organization_id,branch_id,capability_code,record_type,subject_type,subject_reference,status,restricted_visibility,payload,external_financial_reference,source_authority,source_reference,idempotency_key,created_by,schema_version)VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING `+recordCols, v.ID, v.OrganizationID, v.BranchID, v.CapabilityCode, v.RecordType, v.SubjectType, v.SubjectReference, v.Status, v.RestrictedVisibility, v.Payload, v.ExternalFinancialReference, v.SourceAuthority, v.SourceReference, v.IdempotencyKey, v.CreatedBy, v.SchemaVersion))
}

// GetRecord reads one record of a capability.
func (r *Repository) GetRecord(ctx context.Context, org, code, id string) (models.Record, error) {
	return scanRecord(r.pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND id=$3`, org, code, id))
}

// ListRecords lists a capability's records visible in the branch scope
// (an empty scope means every branch), newest first.
func (r *Repository) ListRecords(ctx context.Context, org, code string, permittedBranches []string) ([]models.Record, error) {
	return collectRecords(r.pool.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND(COALESCE(cardinality($3::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($3::text[])) ORDER BY created_at DESC`, org, code, permittedBranches))
}

// ListOrderRecords lists an order's records across the enabled capabilities.
func (r *Repository) ListOrderRecords(ctx context.Context, org, orderID string, enabled []string) ([]models.Record, error) {
	return collectRecords(r.pool.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records r WHERE r.organization_id=$1 AND r.subject_type=$2 AND r.subject_reference=$3 AND r.capability_code=ANY($4::text[]) AND EXISTS(SELECT 1 FROM fleet_partner_organization_capabilities c WHERE c.organization_id=r.organization_id AND c.capability=r.capability_code AND c.status='ENABLED') ORDER BY r.created_at,r.id`, org, models.SubjectOrder, orderID, enabled))
}

// UpdateRecord applies a versioned record change. A stale version is
// models.ErrNotFound.
func (r *Repository) UpdateRecord(ctx context.Context, q database.Querier, org, id string, expected int64, payload json.RawMessage, status, externalFinancialReference string) (models.Record, error) {
	return scanRecord(q.QueryRow(ctx, `UPDATE fleet_partner_specialized_records SET payload=$4,status=$5,external_financial_reference=COALESCE(NULLIF($6,''),external_financial_reference),version=version+1,updated_at=now(),completed_at=CASE WHEN $5='COMPLETED' THEN now() ELSE completed_at END WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+recordCols, org, id, expected, payload, status, externalFinancialReference))
}

// AuditSensitiveRead records a read of restricted specialized data.
func (r *Repository) AuditSensitiveRead(ctx context.Context, org, recordID, actor, purpose, correlationID string) error {
	_, e := r.pool.Exec(ctx, `INSERT INTO fleet_partner_sensitive_access_audit(id,organization_id,specialized_record_id,actor_reference,permission_code,purpose,correlation_id)VALUES($1,$2,$3,$4,'specialized.read',$5,$6)`, ids.New("saa"), org, recordID, actor, purpose, correlationID)
	return e
}

// OrderBranch resolves the branch of an order of the organization.
func (r *Repository) OrderBranch(ctx context.Context, org, orderID string) (string, error) {
	var branch string
	e := r.pool.QueryRow(ctx, `SELECT COALESCE(branch_id,'') FROM fleet_partner_private_orders WHERE organization_id=$1 AND id=$2`, org, orderID).Scan(&branch)
	return branch, notFound(e)
}

// InsertMilestone stores a milestone and returns when it was received.
func (r *Repository) InsertMilestone(ctx context.Context, q database.Querier, v models.Milestone) (time.Time, error) {
	var received time.Time
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_specialized_milestones(id,organization_id,record_id,milestone_type,status,safe_metadata,source_time,actor_reference,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING received_at`, v.ID, v.OrganizationID, v.RecordID, v.MilestoneType, v.Status, v.SafeMetadata, v.SourceTime, v.ActorReference, v.CorrelationID).Scan(&received)
	return received, e
}

const temperatureCols = `id,organization_id,record_id,sensor_reference,temperature,unit,source_version,source_time,received_at,excursion,safe_metadata`

// InsertTemperature stores a reading unless the same (record, sensor, source
// version) already exists; inserted reports which happened.
func (r *Repository) InsertTemperature(ctx context.Context, q database.Querier, v models.TemperatureEvent) (bool, error) {
	tag, e := q.Exec(ctx, `INSERT INTO fleet_partner_temperature_events(id,organization_id,record_id,sensor_reference,temperature,unit,source_version,source_time,excursion,safe_metadata)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (organization_id,record_id,sensor_reference,source_version) DO NOTHING`, v.ID, v.OrganizationID, v.RecordID, v.SensorReference, v.Temperature, v.Unit, v.SourceVersion, v.SourceTime, v.Excursion, v.SafeMetadata)
	if e != nil {
		return false, e
	}
	return tag.RowsAffected() > 0, nil
}

// TemperatureReceivedAt reads when a stored reading was received.
func (r *Repository) TemperatureReceivedAt(ctx context.Context, q database.Querier, org, id string) (time.Time, error) {
	var received time.Time
	e := q.QueryRow(ctx, `SELECT received_at FROM fleet_partner_temperature_events WHERE organization_id=$1 AND id=$2`, org, id).Scan(&received)
	return received, e
}

// QuarantineRecord quarantines a record. Already quarantined is
// models.ErrNotFound.
func (r *Repository) QuarantineRecord(ctx context.Context, q database.Querier, org, id string) (int64, error) {
	var version int64
	e := q.QueryRow(ctx, `UPDATE fleet_partner_specialized_records SET status='QUARANTINED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status<>'QUARANTINED' RETURNING version`, org, id).Scan(&version)
	return version, notFound(e)
}

// GetTemperature reads the stored reading for (record, sensor, version).
func (r *Repository) GetTemperature(ctx context.Context, org, recordID, sensor string, sourceVersion int64) (models.TemperatureEvent, error) {
	var v models.TemperatureEvent
	e := r.pool.QueryRow(ctx, `SELECT `+temperatureCols+` FROM fleet_partner_temperature_events WHERE organization_id=$1 AND record_id=$2 AND sensor_reference=$3 AND source_version=$4`, org, recordID, sensor, sourceVersion).Scan(&v.ID, &v.OrganizationID, &v.RecordID, &v.SensorReference, &v.Temperature, &v.Unit, &v.SourceVersion, &v.SourceTime, &v.ReceivedAt, &v.Excursion, &v.SafeMetadata)
	return v, notFound(e)
}

// Sweep expires lapsed permits and quarantines cold-chain records with an
// excursion.
func (r *Repository) Sweep(ctx context.Context, org string) error {
	_, e := r.pool.Exec(ctx, `UPDATE fleet_partner_specialized_records SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND record_type='PERMIT' AND status NOT IN('EXPIRED','COMPLETED') AND NULLIF(payload->>'validUntil','')::timestamptz<=now()`, org)
	if e != nil {
		return e
	}
	_, e = r.pool.Exec(ctx, `UPDATE fleet_partner_specialized_records r SET status='QUARANTINED',version=version+1,updated_at=now() WHERE organization_id=$1 AND capability_code='COLD_CHAIN' AND EXISTS(SELECT 1 FROM fleet_partner_temperature_events t WHERE t.record_id=r.id AND t.excursion) AND status NOT IN('QUARANTINED','RELEASED')`, org)
	return e
}
