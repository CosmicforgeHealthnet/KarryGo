// Package usecases owns the Section 20 specialized-logistics workflows:
// capability switches, typed extension records, milestones, cold-chain
// temperature readings, order-level data and the maintenance sweep.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Events appends domain events to the outbox inside a transaction.
type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Repository is the specialized-logistics persistence port. Methods taking a
// database.Querier join the caller's transaction.
type Repository interface {
	EnabledCapability(ctx context.Context, org, code string) (models.Capability, error)
	LockCapability(ctx context.Context, q database.Querier, org, code string) (models.Capability, error)
	InsertCapability(ctx context.Context, q database.Querier, org, code, status string, configuration json.RawMessage) (models.Capability, error)
	UpdateCapability(ctx context.Context, q database.Querier, org, code string, expected int64, status string, configuration json.RawMessage) (models.Capability, error)
	InsertCapabilityHistory(ctx context.Context, q database.Querier, org string, v models.Capability, previous, actor, reason string) error
	ListCapabilities(ctx context.Context, org string) ([]models.Capability, error)
	EnabledOrganizations(ctx context.Context) ([]string, error)
	AssetExists(ctx context.Context, org, id string) (bool, error)
	QualificationActive(ctx context.Context, org, id string) (bool, error)
	RecordByIdempotencyKey(ctx context.Context, org, code, key string) (models.Record, error)
	InsertRecord(ctx context.Context, q database.Querier, v models.Record) (models.Record, error)
	GetRecord(ctx context.Context, org, code, id string) (models.Record, error)
	ListRecords(ctx context.Context, org, code string, permittedBranches []string) ([]models.Record, error)
	ListOrderRecords(ctx context.Context, org, orderID string, enabled []string) ([]models.Record, error)
	UpdateRecord(ctx context.Context, q database.Querier, org, id string, expected int64, payload json.RawMessage, status, externalFinancialReference string) (models.Record, error)
	AuditSensitiveRead(ctx context.Context, org, recordID, actor, purpose, correlationID string) error
	OrderBranch(ctx context.Context, org, orderID string) (string, error)
	InsertMilestone(ctx context.Context, q database.Querier, v models.Milestone) (time.Time, error)
	InsertTemperature(ctx context.Context, q database.Querier, v models.TemperatureEvent) (bool, error)
	TemperatureReceivedAt(ctx context.Context, q database.Querier, org, id string) (time.Time, error)
	QuarantineRecord(ctx context.Context, q database.Querier, org, id string) (int64, error)
	GetTemperature(ctx context.Context, org, recordID, sensor string, sourceVersion int64) (models.TemperatureEvent, error)
	Sweep(ctx context.Context, org string) error
}

// Options wires the service. Repository is required.
type Options struct {
	Pool       *pgxpool.Pool
	Repository Repository
	Events     Events
	Audit      *audit.Recorder
	Media      platformclients.MediaContract
	Queue      *jobs.Queue
	Flags      models.Flags
	Now        func() time.Time
}

// Service implements the specialized-logistics workflows.
type Service struct {
	o    Options
	repo Repository
}

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
	return &Service{o: o, repo: o.Repository}
}
func (s *Service) gate(write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	// A disabled specialized capability does not expose its routes (spec
	// 20.1): reads and writes are the same safe not-found.
	if !s.o.Flags.Enabled {
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

// ModuleAvailable reports whether a capability module is switched on for this
// deployment. The HTTP layer answers 404 for a module that is not, before any
// permission check, so a disabled module exposes no route at all.
func (s *Service) ModuleAvailable(code string) bool {
	return s.o.Flags.Enabled && s.o.Flags.Modules[strings.ToUpper(code)]
}

// FeatureAvailable reports whether specialized logistics is switched on.
func (s *Service) FeatureAvailable() bool { return s.o.Flags.Enabled }
func (s *Service) capability(ctx context.Context, org, code string, write bool) (models.Capability, error) {
	if !s.o.Flags.Modules[code] {
		return models.Capability{}, apperrors.NotFound("Resource not found.", nil)
	}
	v, e := s.repo.EnabledCapability(ctx, org, code)
	if errors.Is(e, models.ErrNotFound) {
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

func (s *Service) SetCapability(ctx context.Context, t tenant.Context, code, status, reason string, expected int64, configuration json.RawMessage) (models.Capability, error) {
	if e := s.gate(true); e != nil {
		return models.Capability{}, e
	}
	code = strings.ToUpper(code)
	status = strings.ToUpper(status)
	if !models.ValidCapability(code) || !s.o.Flags.Modules[code] {
		return models.Capability{}, apperrors.NotFound("Resource not found.", nil)
	}
	if status != "ENABLED" && status != "DISABLED" {
		return models.Capability{}, apperrors.BadRequest("models.Capability status is invalid.", nil)
	}
	if reason == "" {
		return models.Capability{}, apperrors.BadRequest("Reason is required.", nil)
	}
	if len(configuration) == 0 {
		configuration = json.RawMessage(`{}`)
	}
	var v models.Capability
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var old string
		var x error
		v, x = s.repo.LockCapability(ctx, tx, t.OrganizationID, code)
		if errors.Is(x, models.ErrNotFound) {
			if status != "DISABLED" && expected > 0 {
				return apperrors.Conflict("models.Capability version conflict.", nil)
			}
			v, x = s.repo.InsertCapability(ctx, tx, t.OrganizationID, code, status, configuration)
			old = ""
		} else if x == nil {
			old = v.Status
			if expected <= 0 || v.Version != expected {
				return apperrors.Conflict("models.Capability version conflict.", nil)
			}
			v, x = s.repo.UpdateCapability(ctx, tx, t.OrganizationID, code, expected, status, configuration)
		}
		if x != nil {
			return x
		}
		if x = s.repo.InsertCapabilityHistory(ctx, tx, t.OrganizationID, v, old, t.ActorID, reason); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "specialized_capability."+strings.ToLower(v.Status)+".v1", "organization_capability", v.ID, v.Version, map[string]any{"capabilityCode": code, "status": status})
	})
	return v, e
}
func (s *Service) ListCapabilities(ctx context.Context, t tenant.Context) ([]models.Capability, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	all, e := s.repo.ListCapabilities(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := []models.Capability{}
	for _, v := range all {
		// A module switched off for this deployment is not shown, whatever
		// the organization row says.
		if s.o.Flags.Modules[v.CapabilityCode] {
			out = append(out, v)
		}
	}
	return out, nil
}

// validateReferences proves that asset and qualification references named
// by the schema belong to the organization (spec 50.2 required
// qualification/asset).
func (s *Service) validateReferences(ctx context.Context, t tenant.Context, v models.Record) error {
	if v.SchemaVersion != models.SchemaVersionCurrent {
		return nil
	}
	assets, qualifications := models.ReferenceFields(v.CapabilityCode, v.RecordType, v.Payload)
	for _, id := range assets {
		ok, e := s.repo.AssetExists(ctx, t.OrganizationID, id)
		if e != nil {
			return e
		}
		if !ok {
			return apperrors.BadRequest("Referenced asset "+id+" does not exist in this organization.", nil)
		}
	}
	for _, id := range qualifications {
		ok, e := s.repo.QualificationActive(ctx, t.OrganizationID, id)
		if e != nil {
			return e
		}
		if !ok {
			return apperrors.BadRequest("Referenced qualification "+id+" is not an active qualification of this organization.", nil)
		}
	}
	return nil
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
func (s *Service) CreateRecord(ctx context.Context, t tenant.Context, v models.Record) (models.Record, error) {
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
	if e := models.ValidateRecord(&v); e != nil {
		return v, e
	}
	if e := s.validateReferences(ctx, t, v); e != nil {
		return v, e
	}
	if e := s.validateMedia(ctx, t, v.Payload); e != nil {
		return v, e
	}
	existing, e := s.repo.RecordByIdempotencyKey(ctx, t.OrganizationID, v.CapabilityCode, v.IdempotencyKey)
	if e == nil {
		return existing, nil
	}
	if !errors.Is(e, models.ErrNotFound) {
		return v, e
	}
	v.ID = ids.New("sprec")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertRecord(ctx, tx, v)
		if x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "specialized_record.created.v1", "specialized_record", v.ID, v.Version, map[string]any{"recordId": v.ID, "capabilityCode": v.CapabilityCode, "recordType": v.RecordType, "status": v.Status, "schemaId": v.SchemaID}); x != nil {
			return x
		}
		if v.SubjectType == models.SubjectOrder && s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "specialized_order.validated.v1", "order", v.SubjectReference, v.Version, t.OrganizationID, t.CorrelationID, map[string]any{"orderId": v.SubjectReference, "recordId": v.ID, "capabilityCode": v.CapabilityCode, "recordType": v.RecordType, "schemaId": v.SchemaID})
		}
		return nil
	})
	return v, e
}
func (s *Service) GetRecord(ctx context.Context, t tenant.Context, code, id string) (models.Record, error) {
	if e := s.gate(false); e != nil {
		return models.Record{}, e
	}
	code = strings.ToUpper(code)
	if _, e := s.capability(ctx, t.OrganizationID, code, false); e != nil {
		return models.Record{}, e
	}
	v, e := s.repo.GetRecord(ctx, t.OrganizationID, code, id)
	if errors.Is(e, models.ErrNotFound) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Resource not found.", nil)
	}
	if e == nil && v.RestrictedVisibility {
		_ = s.repo.AuditSensitiveRead(ctx, t.OrganizationID, v.ID, t.ActorID, "OPERATIONAL_READ", t.CorrelationID)
	}
	return v, e
}
func (s *Service) ListRecords(ctx context.Context, t tenant.Context, code string) ([]models.Record, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	if _, e := s.capability(ctx, t.OrganizationID, strings.ToUpper(code), false); e != nil {
		return nil, e
	}
	all, e := s.repo.ListRecords(ctx, t.OrganizationID, strings.ToUpper(code), t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	out := make([]models.Record, 0, len(all))
	for _, v := range all {
		out = append(out, models.Redact(v))
	}
	return out, nil
}

func (s *Service) UpdateRecord(ctx context.Context, t tenant.Context, code, id string, expected int64, v models.Record, command string) (models.Record, error) {
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
	// An update is validated against the schema version the record was
	// written with, so version 1 records stay editable.
	v.SchemaVersion = old.SchemaVersion
	if e = models.ValidateRecord(&v); e != nil {
		return v, e
	}
	if e = s.validateReferences(ctx, t, v); e != nil {
		return v, e
	}
	status := old.Status
	event := "specialized_record.updated.v1"
	if command != "" {
		if old.Status == "COMPLETED" {
			return v, apperrors.Conflict("The specialized record is already completed.", nil)
		}
		switch command {
		case "COMPLETE":
			// Quarantined cold-chain goods need an authorized release first.
			if old.Status == "QUARANTINED" {
				return v, apperrors.Conflict("A quarantined record must be released before it is completed.", nil)
			}
			status = "COMPLETED"
		case "QUARANTINE":
			if code != models.ColdChain {
				return v, apperrors.BadRequest("Quarantine is only valid for cold chain.", nil)
			}
			status = "QUARANTINED"
		case "RELEASE":
			if code != models.ColdChain || old.Status != "QUARANTINED" {
				return v, apperrors.Conflict("Only quarantined cold-chain records can be released.", nil)
			}
			var release map[string]any
			_ = json.Unmarshal(v.Payload, &release)
			// fmt.Sprint of a missing key is "<nil>", which is not empty, so
			// read the values as strings.
			evidence, _ := release["releaseEvidenceReference"].(string)
			authorizedBy, _ := release["authorizedBy"].(string)
			if strings.TrimSpace(evidence) == "" || strings.TrimSpace(authorizedBy) == "" {
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
		v, x = s.repo.UpdateRecord(ctx, tx, t.OrganizationID, id, expected, v.Payload, status, v.ExternalFinancialReference)
		if errors.Is(x, models.ErrNotFound) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "specialized_record", id, v.Version, map[string]any{"recordId": id, "capabilityCode": v.CapabilityCode, "recordType": v.RecordType, "status": v.Status})
	})
	return v, e
}
func (s *Service) AddMilestone(ctx context.Context, t tenant.Context, code, id string, v models.Milestone) (models.Milestone, error) {
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
		return v, apperrors.BadRequest("models.Milestone type, status and correlationId are required.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		if v.ReceivedAt, x = s.repo.InsertMilestone(ctx, tx, v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "specialized_milestone.recorded.v1", "specialized_record", id, r.Version, map[string]any{"recordId": id, "milestoneId": v.ID, "milestoneType": v.MilestoneType, "status": v.Status})
	})
	return v, e
}

func (s *Service) AddTemperature(ctx context.Context, t tenant.Context, id string, v models.TemperatureEvent) (models.TemperatureEvent, error) {
	r, e := s.GetRecord(ctx, t, models.ColdChain, id)
	if e != nil {
		return v, e
	}
	if r.RecordType != "TEMPERATURE_REQUIREMENT" {
		return v, apperrors.BadRequest("Temperature events require a temperature-requirement record.", nil)
	}
	if v.SensorReference == "" || v.SourceTime.IsZero() || v.SourceVersion < 0 || (v.Unit != "C" && v.Unit != "F") {
		return v, apperrors.BadRequest("Sensor, source time, source version and temperature unit are required.", nil)
	}
	// FLEET-20.7: the excursion decision is made only against thresholds the
	// requirement record actually states. A missing or unreadable bound used
	// to read as 0, which turned every frozen-goods reading into an excursion
	// (and every reading above 0 into one for a record with no maximum).
	minimum, maximum, requiredUnit, e := models.TemperatureThresholds(r.Payload)
	if e != nil {
		return v, e
	}
	if requiredUnit != "" && v.Unit != requiredUnit {
		return v, apperrors.BadRequest("Temperature unit does not match the requirement.", nil)
	}
	// A reading is replayed per record: one sensor may serve several
	// cold-chain records, and the same (sensor, source version) sent against
	// a second record is a different reading, not a replay of the first.
	if existing, err := s.temperatureReplay(ctx, t, id, v); err == nil || !errors.Is(err, models.ErrNotFound) {
		return existing, err
	}
	v.Excursion = v.Temperature < minimum || v.Temperature > maximum
	v.ID = ids.New("tmp")
	v.OrganizationID = t.OrganizationID
	v.RecordID = id
	if len(v.SafeMetadata) == 0 {
		v.SafeMetadata = json.RawMessage(`{}`)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		inserted, x := s.repo.InsertTemperature(ctx, tx, v)
		if x != nil {
			return x
		}
		if !inserted {
			// A concurrent delivery of the same reading won the insert.
			return errTemperatureRaced
		}
		if v.ReceivedAt, x = s.repo.TemperatureReceivedAt(ctx, tx, v.OrganizationID, v.ID); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "cold_chain.temperature_recorded.v1", "temperature_event", v.ID, 1, map[string]any{"recordId": id, "sensorReference": v.SensorReference, "sourceVersion": v.SourceVersion, "excursion": v.Excursion}); x != nil {
			return x
		}
		if !v.Excursion {
			return nil
		}
		version, x := s.repo.QuarantineRecord(ctx, tx, t.OrganizationID, id)
		if errors.Is(x, models.ErrNotFound) {
			// Already quarantined: the excursion is recorded, the status
			// does not move again.
			return nil
		}
		if x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "cold_chain.excursion.v1", "specialized_record", id, version, map[string]any{"recordId": id, "sensorReference": v.SensorReference, "sourceVersion": v.SourceVersion, "quarantined": true}); x != nil {
			return x
		}
		if s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "cold_chain.quarantined.v1", "specialized_record", id, version, t.OrganizationID, t.CorrelationID, map[string]any{"recordId": id, "sourceVersion": v.SourceVersion})
		}
		return nil
	})
	if errors.Is(e, errTemperatureRaced) {
		return s.temperatureReplay(ctx, t, id, v)
	}
	return v, e
}

var errTemperatureRaced = errors.New("specialized: temperature reading inserted concurrently")

// temperatureReplay returns the stored reading for (record, sensor, source
// version). The same identity with a different reading is a conflict: a
// sensor that reports two values for one source version is not a replay.
func (s *Service) temperatureReplay(ctx context.Context, t tenant.Context, recordID string, v models.TemperatureEvent) (models.TemperatureEvent, error) {
	existing, err := s.repo.GetTemperature(ctx, t.OrganizationID, recordID, v.SensorReference, v.SourceVersion)
	if err != nil {
		return v, err
	}
	if existing.Unit != v.Unit || math.Abs(existing.Temperature-v.Temperature) > 0.0005 || !existing.SourceTime.Equal(v.SourceTime) {
		return v, apperrors.Conflict("This sensor already reported a different reading for this source version.", nil)
	}
	return existing, nil
}
