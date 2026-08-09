package specialized

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Options struct {
	Pool   *pgxpool.Pool
	Events Events
	Audit  *audit.Recorder
	Media  platformclients.MediaContract
	Queue  *jobs.Queue
	Flags  Flags
	Now    func() time.Time
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
	if o.Flags.Modules == nil {
		o.Flags.Modules = map[string]bool{}
	}
	return &Service{o: o}
}
func (s *Service) gate(write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !s.o.Flags.Enabled {
		if write {
			return apperrors.Forbidden("Specialized logistics is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *Service) capability(ctx context.Context, org, code string, write bool) (Capability, error) {
	if !s.o.Flags.Modules[code] {
		return Capability{}, apperrors.NotFound("Resource not found.", nil)
	}
	v, e := scanCapability(s.o.Pool.QueryRow(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 AND capability=$2 AND status='ENABLED'`, org, code))
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	return v, e
}
func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

const capCols = `id,organization_id,capability,status,configuration,enabled_at,disabled_at,version,created_at,updated_at`

func scanCapability(r pgx.Row) (Capability, error) {
	var v Capability
	e := r.Scan(&v.ID, &v.OrganizationID, &v.CapabilityCode, &v.Status, &v.Configuration, &v.EnabledAt, &v.DisabledAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) SetCapability(ctx context.Context, t tenant.Context, code, status, reason string, expected int64, configuration json.RawMessage) (Capability, error) {
	if e := s.gate(true); e != nil {
		return Capability{}, e
	}
	code = strings.ToUpper(code)
	status = strings.ToUpper(status)
	if !ValidCapability(code) || !s.o.Flags.Modules[code] {
		return Capability{}, apperrors.NotFound("Resource not found.", nil)
	}
	if status != "ENABLED" && status != "DISABLED" {
		return Capability{}, apperrors.BadRequest("Capability status is invalid.", nil)
	}
	if reason == "" {
		return Capability{}, apperrors.BadRequest("Reason is required.", nil)
	}
	if len(configuration) == 0 {
		configuration = json.RawMessage(`{}`)
	}
	var v Capability
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var old string
		var x error
		v, x = scanCapability(tx.QueryRow(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 AND capability=$2 FOR UPDATE`, t.OrganizationID, code))
		if errors.Is(x, pgx.ErrNoRows) {
			if status != "DISABLED" && expected > 0 {
				return apperrors.Conflict("Capability version conflict.", nil)
			}
			v, x = scanCapability(tx.QueryRow(ctx, `INSERT INTO fleet_partner_organization_capabilities(id,organization_id,capability,status,configuration,enabled_at,disabled_at)VALUES($1,$2,$3,$4,$5,CASE WHEN $4='ENABLED' THEN now() END,CASE WHEN $4='DISABLED' THEN now() END) RETURNING `+capCols, ids.New("cap"), t.OrganizationID, code, status, configuration))
			old = ""
		} else if x == nil {
			old = v.Status
			if expected <= 0 || v.Version != expected {
				return apperrors.Conflict("Capability version conflict.", nil)
			}
			v, x = scanCapability(tx.QueryRow(ctx, `UPDATE fleet_partner_organization_capabilities SET status=$4,configuration=$5,enabled_at=CASE WHEN $4='ENABLED' THEN now() ELSE enabled_at END,disabled_at=CASE WHEN $4='DISABLED' THEN now() ELSE NULL END,version=version+1,updated_at=now() WHERE organization_id=$1 AND capability=$2 AND version=$3 RETURNING `+capCols, t.OrganizationID, code, expected, status, configuration))
		}
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_capability_history(id,organization_id,capability_id,previous_status,new_status,actor_reference,reason,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New("caph"), t.OrganizationID, v.ID, old, v.Status, t.ActorID, reason, v.Version, mustJSON(v))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "specialized_capability."+strings.ToLower(v.Status)+".v1", "organization_capability", v.ID, v.Version, map[string]any{"capabilityCode": code, "status": status})
	})
	return v, e
}
func (s *Service) ListCapabilities(ctx context.Context, t tenant.Context) ([]Capability, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+capCols+` FROM fleet_partner_organization_capabilities WHERE organization_id=$1 ORDER BY capability`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Capability{}
	for rows.Next() {
		v, e := scanCapability(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const recordCols = `id,organization_id,COALESCE(branch_id,''),capability_code,record_type,subject_type,subject_reference,status,restricted_visibility,payload,external_financial_reference,source_authority,source_reference,idempotency_key,version,created_by,created_at,updated_at,completed_at`

func scanRecord(r pgx.Row) (Record, error) {
	var v Record
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.CapabilityCode, &v.RecordType, &v.SubjectType, &v.SubjectReference, &v.Status, &v.RestrictedVisibility, &v.Payload, &v.ExternalFinancialReference, &v.SourceAuthority, &v.SourceReference, &v.IdempotencyKey, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, e
}
func (s *Service) validateMedia(ctx context.Context, t tenant.Context, p json.RawMessage) error {
	var v map[string]any
	_ = json.Unmarshal(p, &v)
	for _, key := range []string{"mediaFileId", "customsDocumentMediaId", "evidenceMediaId"} {
		id, _ := v[key].(string)
		if id == "" {
			continue
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
	}
	return nil
}
func (s *Service) CreateRecord(ctx context.Context, t tenant.Context, v Record) (Record, error) {
	if e := s.gate(true); e != nil {
		return v, e
	}
	v.CapabilityCode = strings.ToUpper(v.CapabilityCode)
	if _, e := s.capability(ctx, t.OrganizationID, v.CapabilityCode, true); e != nil {
		return v, e
	}
	if v.BranchID != "" && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	if v.IdempotencyKey == "" {
		return v, apperrors.BadRequest("Idempotency key is required.", nil)
	}
	if v.Status == "" {
		v.Status = "DRAFT"
	}
	if e := ValidateRecord(&v); e != nil {
		return v, e
	}
	if e := s.validateMedia(ctx, t, v.Payload); e != nil {
		return v, e
	}
	existing, e := scanRecord(s.o.Pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND idempotency_key=$3`, t.OrganizationID, v.CapabilityCode, v.IdempotencyKey))
	if e == nil {
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	v.ID = ids.New("sprec")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanRecord(tx.QueryRow(ctx, `INSERT INTO fleet_partner_specialized_records(id,organization_id,branch_id,capability_code,record_type,subject_type,subject_reference,status,restricted_visibility,payload,external_financial_reference,source_authority,source_reference,idempotency_key,created_by)VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING `+recordCols, v.ID, v.OrganizationID, v.BranchID, v.CapabilityCode, v.RecordType, v.SubjectType, v.SubjectReference, v.Status, v.RestrictedVisibility, v.Payload, v.ExternalFinancialReference, v.SourceAuthority, v.SourceReference, v.IdempotencyKey, v.CreatedBy))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "specialized_record.created.v1", "specialized_record", v.ID, v.Version, map[string]any{"recordId": v.ID, "capabilityCode": v.CapabilityCode, "recordType": v.RecordType, "status": v.Status})
	})
	return v, e
}
func (s *Service) GetRecord(ctx context.Context, t tenant.Context, code, id string) (Record, error) {
	if e := s.gate(false); e != nil {
		return Record{}, e
	}
	code = strings.ToUpper(code)
	if _, e := s.capability(ctx, t.OrganizationID, code, false); e != nil {
		return Record{}, e
	}
	v, e := scanRecord(s.o.Pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND id=$3`, t.OrganizationID, code, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Resource not found.", nil)
	}
	if e == nil && v.RestrictedVisibility {
		_, _ = s.o.Pool.Exec(ctx, `INSERT INTO fleet_partner_sensitive_access_audit(id,organization_id,specialized_record_id,actor_reference,permission_code,purpose,correlation_id)VALUES($1,$2,$3,$4,'specialized.read','OPERATIONAL_READ',$5)`, ids.New("saa"), t.OrganizationID, v.ID, t.ActorID, t.CorrelationID)
	}
	return v, e
}
func (s *Service) ListRecords(ctx context.Context, t tenant.Context, code string) ([]Record, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	if _, e := s.capability(ctx, t.OrganizationID, strings.ToUpper(code), false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2 AND(cardinality($3::text[])=0 OR branch_id IS NULL OR branch_id=ANY($3::text[])) ORDER BY created_at DESC`, t.OrganizationID, strings.ToUpper(code), t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		v, e := scanRecord(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) UpdateRecord(ctx context.Context, t tenant.Context, code, id string, expected int64, v Record, command string) (Record, error) {
	old, e := s.GetRecord(ctx, t, code, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	v.CapabilityCode = old.CapabilityCode
	v.RecordType = old.RecordType
	v.SubjectType = old.SubjectType
	v.SubjectReference = old.SubjectReference
	if len(v.Payload) == 0 {
		v.Payload = old.Payload
	}
	v.RestrictedVisibility = old.RestrictedVisibility
	if e = ValidateRecord(&v); e != nil {
		return v, e
	}
	status := old.Status
	event := "specialized_record.updated.v1"
	if command != "" {
		switch command {
		case "COMPLETE":
			status = "COMPLETED"
		case "QUARANTINE":
			if code != ColdChain {
				return v, apperrors.BadRequest("Quarantine is only valid for cold chain.", nil)
			}
			status = "QUARANTINED"
		case "RELEASE":
			if code != ColdChain || old.Status != "QUARANTINED" {
				return v, apperrors.Conflict("Only quarantined cold-chain records can be released.", nil)
			}
			var release map[string]any
			_ = json.Unmarshal(v.Payload, &release)
			if strings.TrimSpace(fmt.Sprint(release["releaseEvidenceReference"])) == "" || strings.TrimSpace(fmt.Sprint(release["authorizedBy"])) == "" {
				return v, apperrors.BadRequest("Cold-chain release requires evidence and an authorized actor.", nil)
			}
			status = "RELEASED"
		default:
			return v, apperrors.BadRequest("Unsupported command.", nil)
		}
		event = "specialized_record." + strings.ToLower(status) + ".v1"
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanRecord(tx.QueryRow(ctx, `UPDATE fleet_partner_specialized_records SET payload=$4,status=$5,external_financial_reference=COALESCE(NULLIF($6,''),external_financial_reference),version=version+1,updated_at=now(),completed_at=CASE WHEN $5='COMPLETED' THEN now() ELSE completed_at END WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+recordCols, t.OrganizationID, id, expected, v.Payload, status, v.ExternalFinancialReference))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "specialized_record", id, v.Version, map[string]any{"recordId": id, "capabilityCode": v.CapabilityCode, "recordType": v.RecordType, "status": v.Status})
	})
	return v, e
}
func (s *Service) AddMilestone(ctx context.Context, t tenant.Context, code, id string, v Milestone) (Milestone, error) {
	r, e := s.GetRecord(ctx, t, code, id)
	_ = r
	if e != nil {
		return v, e
	}
	v.ID = ids.New("spmil")
	v.OrganizationID = t.OrganizationID
	v.RecordID = id
	v.ActorReference = t.ActorID
	if v.SourceTime.IsZero() {
		v.SourceTime = s.o.Now()
	}
	if len(v.SafeMetadata) == 0 {
		v.SafeMetadata = json.RawMessage(`{}`)
	}
	if v.MilestoneType == "" || v.Status == "" || v.CorrelationID == "" {
		return v, apperrors.BadRequest("Milestone type, status and correlationId are required.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_specialized_milestones(id,organization_id,record_id,milestone_type,status,safe_metadata,source_time,actor_reference,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING received_at`, v.ID, v.OrganizationID, v.RecordID, v.MilestoneType, v.Status, v.SafeMetadata, v.SourceTime, v.ActorReference, v.CorrelationID).Scan(&v.ReceivedAt); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "specialized_milestone.recorded.v1", "specialized_record", id, r.Version, map[string]any{"recordId": id, "milestoneId": v.ID, "milestoneType": v.MilestoneType, "status": v.Status})
	})
	return v, e
}

const temperatureCols = `id,organization_id,record_id,sensor_reference,temperature,unit,source_version,source_time,received_at,excursion,safe_metadata`

func scanTemperature(r pgx.Row) (TemperatureEvent, error) {
	var v TemperatureEvent
	e := r.Scan(&v.ID, &v.OrganizationID, &v.RecordID, &v.SensorReference, &v.Temperature, &v.Unit, &v.SourceVersion, &v.SourceTime, &v.ReceivedAt, &v.Excursion, &v.SafeMetadata)
	return v, e
}

func (s *Service) AddTemperature(ctx context.Context, t tenant.Context, id string, v TemperatureEvent) (TemperatureEvent, error) {
	r, e := s.GetRecord(ctx, t, ColdChain, id)
	if e != nil {
		return v, e
	}
	if r.RecordType != "TEMPERATURE_REQUIREMENT" {
		return v, apperrors.BadRequest("Temperature events require a temperature-requirement record.", nil)
	}
	var p map[string]any
	_ = json.Unmarshal(r.Payload, &p)
	min, _ := p["minimumTemperature"].(float64)
	max, _ := p["maximumTemperature"].(float64)
	requiredUnit, _ := p["unit"].(string)
	if v.SensorReference == "" || v.SourceTime.IsZero() || v.SourceVersion < 0 || (v.Unit != "C" && v.Unit != "F") {
		return v, apperrors.BadRequest("Sensor, source time, source version and temperature unit are required.", nil)
	}
	if requiredUnit != "" && v.Unit != requiredUnit {
		return v, apperrors.BadRequest("Temperature unit does not match the requirement.", nil)
	}
	existing, existingErr := scanTemperature(s.o.Pool.QueryRow(ctx, `SELECT `+temperatureCols+` FROM fleet_partner_temperature_events WHERE organization_id=$1 AND sensor_reference=$2 AND source_version=$3`, t.OrganizationID, v.SensorReference, v.SourceVersion))
	if existingErr == nil {
		return existing, nil
	}
	if !errors.Is(existingErr, pgx.ErrNoRows) {
		return v, existingErr
	}
	v.Excursion = v.Temperature < min || v.Temperature > max
	v.ID = ids.New("tmp")
	v.OrganizationID = t.OrganizationID
	v.RecordID = id
	if len(v.SafeMetadata) == 0 {
		v.SafeMetadata = json.RawMessage(`{}`)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_temperature_events(id,organization_id,record_id,sensor_reference,temperature,unit,source_version,source_time,excursion,safe_metadata)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING received_at`, v.ID, v.OrganizationID, v.RecordID, v.SensorReference, v.Temperature, v.Unit, v.SourceVersion, v.SourceTime, v.Excursion, v.SafeMetadata).Scan(&v.ReceivedAt)
		if x != nil {
			return x
		}
		if v.Excursion {
			_, x = tx.Exec(ctx, `UPDATE fleet_partner_specialized_records SET status='QUARANTINED',version=version+1,updated_at=now() WHERE id=$1`, id)
			if x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "cold_chain.excursion.v1", "specialized_record", id, r.Version+1, map[string]any{"recordId": id, "sensorReference": v.SensorReference, "sourceVersion": v.SourceVersion, "quarantined": true}); x != nil {
				return x
			}
			if s.o.Events != nil {
				return s.o.Events.Publish(ctx, tx, "cold_chain.quarantined.v1", "specialized_record", id, r.Version+1, t.OrganizationID, t.CorrelationID, map[string]any{"recordId": id, "sourceVersion": v.SourceVersion})
			}
		}
		return nil
	})
	return v, e
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
