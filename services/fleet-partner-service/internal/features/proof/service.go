package proof

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Options struct {
	Pool       *pgxpool.Pool
	Events     Events
	Audit      *audit.Recorder
	Media      platformclients.MediaContract
	Queue      *jobs.Queue
	HashSecret []byte
	Flags      Flags
	Now        func() time.Time
}
type Service struct{ o Options }

var supportedProofTypes = map[string]bool{
	"OTP": true, "SIGNATURE": true, "PHOTO": true, "PACKAGE_SCAN": true,
	"DOCUMENT_SCAN": true, "LOCATION": true, "TIMESTAMP": true,
	"RECIPIENT_NAME": true, "IDENTITY_REFERENCE": true, "SEAL": true,
	"TEMPERATURE_READING": true, "CUSTOM_FIELD": true,
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
	return &Service{o: o}
}
func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("Proof capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
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

const policyCols = `id,organization_id,policy_scope,scope_reference,service_type,stop_type,required_proof_types,minimum_count,location_required,time_window_rules,failure_policy,status,version,created_by,created_at,updated_at`

func scanPolicy(r pgx.Row) (Policy, error) {
	var v Policy
	e := r.Scan(&v.ID, &v.OrganizationID, &v.PolicyScope, &v.ScopeReference, &v.ServiceType, &v.StopType, &v.RequiredProofTypes, &v.MinimumCount, &v.LocationRequired, &v.TimeWindowRules, &v.FailurePolicy, &v.Status, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) SavePolicy(ctx context.Context, t tenant.Context, id string, expected int64, v Policy) (Policy, PolicySnapshot, error) {
	if e := s.gate(s.o.Flags.Policies, true); e != nil {
		return v, PolicySnapshot{}, e
	}
	v.PolicyScope = strings.ToUpper(v.PolicyScope)
	v.FailurePolicy = strings.ToUpper(v.FailurePolicy)
	if v.Status == "" {
		v.Status = "PUBLISHED"
	}
	v.Status = strings.ToUpper(v.Status)
	v.RequiredProofTypes = raw(v.RequiredProofTypes, "[]")
	v.TimeWindowRules = raw(v.TimeWindowRules, "{}")
	var types []string
	if json.Unmarshal(v.RequiredProofTypes, &types) != nil || len(types) == 0 {
		return v, PolicySnapshot{}, apperrors.BadRequest("At least one valid required proof type is required.", nil)
	}
	for i := range types {
		types[i] = strings.ToUpper(strings.TrimSpace(types[i]))
		if !supportedProofTypes[types[i]] {
			return v, PolicySnapshot{}, apperrors.BadRequest("Policy contains an unsupported proof type.", nil)
		}
	}
	v.RequiredProofTypes, _ = json.Marshal(types)
	if v.MinimumCount <= 0 {
		v.MinimumCount = 1
	}
	if v.PolicyScope == "" || v.FailurePolicy == "" || (v.FailurePolicy != "BLOCK" && v.FailurePolicy != "WARN") {
		return v, PolicySnapshot{}, apperrors.BadRequest("Policy scope and failure policy are required.", nil)
	}
	if v.Status != "DRAFT" && v.Status != "PUBLISHED" && v.Status != "INACTIVE" && v.Status != "ARCHIVED" {
		return v, PolicySnapshot{}, apperrors.BadRequest("Policy status is invalid.", nil)
	}
	var snap PolicySnapshot
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		if id == "" {
			v.ID = ids.New("prfpol")
			v.OrganizationID = t.OrganizationID
			v.CreatedBy = t.ActorID
			v, x = scanPolicy(tx.QueryRow(ctx, `INSERT INTO fleet_partner_proof_policies(id,organization_id,policy_scope,scope_reference,service_type,stop_type,required_proof_types,minimum_count,location_required,time_window_rules,failure_policy,status,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+policyCols, v.ID, v.OrganizationID, v.PolicyScope, v.ScopeReference, v.ServiceType, v.StopType, v.RequiredProofTypes, v.MinimumCount, v.LocationRequired, v.TimeWindowRules, v.FailurePolicy, v.Status, v.CreatedBy))
		} else {
			if expected <= 0 {
				return apperrors.BadRequest("expectedVersion is required.", nil)
			}
			v, x = scanPolicy(tx.QueryRow(ctx, `UPDATE fleet_partner_proof_policies SET required_proof_types=$4,minimum_count=$5,location_required=$6,time_window_rules=$7,failure_policy=$8,status=$9,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status<>'ARCHIVED' RETURNING `+policyCols, t.OrganizationID, id, expected, v.RequiredProofTypes, v.MinimumCount, v.LocationRequired, v.TimeWindowRules, v.FailurePolicy, v.Status))
			if errors.Is(x, pgx.ErrNoRows) {
				return apperrors.Conflict("Version conflict.", nil)
			}
		}
		if x != nil {
			return x
		}
		if v.Status == "PUBLISHED" {
			snapshot, _ := json.Marshal(v)
			snap = PolicySnapshot{ID: ids.New("prfsnp"), OrganizationID: t.OrganizationID, PolicyID: v.ID, PolicyVersion: v.Version, Snapshot: snapshot, PublishedBy: t.ActorID}
			x = tx.QueryRow(ctx, `INSERT INTO fleet_partner_proof_policy_versions(id,organization_id,policy_id,policy_version,snapshot,published_by)VALUES($1,$2,$3,$4,$5,$6) RETURNING published_at`, snap.ID, snap.OrganizationID, snap.PolicyID, snap.PolicyVersion, snap.Snapshot, snap.PublishedBy).Scan(&snap.PublishedAt)
			if x != nil {
				return x
			}
			return s.record(ctx, tx, t, "proof_policy.published.v1", "proof_policy", v.ID, v.Version, map[string]any{"policyId": v.ID, "snapshotReference": snap.ID, "version": v.Version})
		}
		return s.record(ctx, tx, t, "proof_policy.saved.v1", "proof_policy", v.ID, v.Version, map[string]any{"policyId": v.ID, "status": v.Status, "version": v.Version})
	})
	return v, snap, e
}
func (s *Service) GetPolicy(ctx context.Context, t tenant.Context, id string) (Policy, error) {
	if e := s.gate(s.o.Flags.Policies, false); e != nil {
		return Policy{}, e
	}
	v, e := scanPolicy(s.o.Pool.QueryRow(ctx, `SELECT `+policyCols+` FROM fleet_partner_proof_policies WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Proof policy not found.", nil)
	}
	return v, e
}
func (s *Service) ListPolicies(ctx context.Context, t tenant.Context) ([]Policy, error) {
	if e := s.gate(s.o.Flags.Policies, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+policyCols+` FROM fleet_partner_proof_policies WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Policy{}
	for rows.Next() {
		v, e := scanPolicy(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) snapshot(ctx context.Context, org, id string) (PolicySnapshot, Policy, error) {
	var snap PolicySnapshot
	e := s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,policy_id,policy_version,snapshot,published_by,published_at FROM fleet_partner_proof_policy_versions WHERE organization_id=$1 AND id=$2`, org, id).Scan(&snap.ID, &snap.OrganizationID, &snap.PolicyID, &snap.PolicyVersion, &snap.Snapshot, &snap.PublishedBy, &snap.PublishedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return snap, Policy{}, apperrors.NotFound("Proof policy snapshot not found.", nil)
	}
	if e != nil {
		return snap, Policy{}, e
	}
	var p Policy
	if e = json.Unmarshal(snap.Snapshot, &p); e != nil {
		return snap, p, e
	}
	return snap, p, nil
}
func (s *Service) media(ctx context.Context, t tenant.Context, id string) error {
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
func (s *Service) hash(org, policy, typ, value string) (string, error) {
	if len(s.o.HashSecret) < 32 {
		return "", apperrors.Unavailable("Proof hashing is not configured.", nil)
	}
	h := hmac.New(sha256.New, s.o.HashSecret)
	for _, p := range []string{org, policy, typ, value} {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

const recordCols = `id,organization_id,policy_snapshot_reference,proof_type,order_id,shipment_id,COALESCE(package_id,''),route_stop_id,assignment_id,canonical_trip_reference,actor_type,actor_reference,media_file_id,value_hash,safe_value_metadata,latitude,longitude,accuracy,source_timestamp,received_at,source,verification_status,source_version,idempotency_key,version,created_at`

func scanRecord(r pgx.Row) (Record, error) {
	var v Record
	e := r.Scan(&v.ID, &v.OrganizationID, &v.PolicySnapshotReference, &v.ProofType, &v.OrderID, &v.ShipmentID, &v.PackageID, &v.RouteStopID, &v.AssignmentID, &v.CanonicalTripReference, &v.ActorType, &v.ActorReference, &v.MediaFileID, &v.ValueHash, &v.SafeValueMetadata, &v.Latitude, &v.Longitude, &v.Accuracy, &v.SourceTimestamp, &v.ReceivedAt, &v.Source, &v.VerificationStatus, &v.SourceVersion, &v.IdempotencyKey, &v.Version, &v.CreatedAt)
	return v, e
}
func (s *Service) CreateRecord(ctx context.Context, t tenant.Context, in RecordInput) (Record, error) {
	if e := s.gate(s.o.Flags.Records, true); e != nil {
		return Record{}, e
	}
	if in.IdempotencyKey == "" {
		return Record{}, apperrors.BadRequest("Idempotency key is required.", nil)
	}
	existing, e := scanRecord(s.o.Pool.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 AND idempotency_key=$2`, t.OrganizationID, in.IdempotencyKey))
	if e == nil {
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Record{}, e
	}
	_, p, e := s.snapshot(ctx, t.OrganizationID, in.PolicySnapshotReference)
	if e != nil {
		return Record{}, e
	}
	in.ProofType = strings.ToUpper(in.ProofType)
	if !supportedProofTypes[in.ProofType] {
		return Record{}, apperrors.BadRequest("Proof type is invalid.", nil)
	}
	var required []string
	_ = json.Unmarshal(p.RequiredProofTypes, &required)
	found := false
	for _, x := range required {
		if strings.ToUpper(x) == in.ProofType {
			found = true
		}
	}
	if !found {
		return Record{}, apperrors.Conflict("Proof type is not required by the policy snapshot.", nil)
	}
	if p.LocationRequired && (in.Latitude == nil || in.Longitude == nil) {
		return Record{}, apperrors.BadRequest("Location proof is required by policy.", nil)
	}
	verified := "PENDING"
	valueHash := ""
	if in.ProofType == "OTP" {
		if strings.TrimSpace(in.Value) == "" {
			return Record{}, apperrors.BadRequest("OTP proof value is required.", nil)
		}
		valueHash, e = s.hash(t.OrganizationID, in.PolicySnapshotReference, in.ProofType, in.Value)
		if e != nil {
			return Record{}, e
		}
		in.Value = ""
	}
	if (in.ProofType == "PHOTO" || in.ProofType == "SIGNATURE" || in.ProofType == "DOCUMENT_SCAN") && in.MediaFileID == "" {
		return Record{}, apperrors.BadRequest("Media evidence is required for this proof type.", nil)
	}
	if in.ProofType == "LOCATION" && (in.Latitude == nil || in.Longitude == nil) {
		return Record{}, apperrors.BadRequest("Coordinates are required for location proof.", nil)
	}
	if in.MediaFileID != "" {
		if e = s.media(ctx, t, in.MediaFileID); e != nil {
			return Record{}, e
		}
		verified = "VERIFIED"
	}
	if in.SourceTimestamp.IsZero() {
		return Record{}, apperrors.BadRequest("sourceTimestamp is required.", nil)
	}
	if in.ActorType == "" || in.Source == "" {
		return Record{}, apperrors.BadRequest("Actor type and source are required.", nil)
	}
	if in.OrderID == "" && in.ShipmentID == "" && in.PackageID == "" && in.RouteStopID == "" && in.AssignmentID == "" && in.CanonicalTripReference == "" {
		return Record{}, apperrors.BadRequest("Proof must reference an operation subject.", nil)
	}
	if in.ActorReference == "" {
		in.ActorReference = t.ActorID
	}
	v := Record{ID: ids.New("prfrec"), OrganizationID: t.OrganizationID, PolicySnapshotReference: in.PolicySnapshotReference, ProofType: in.ProofType, OrderID: in.OrderID, ShipmentID: in.ShipmentID, PackageID: in.PackageID, RouteStopID: in.RouteStopID, AssignmentID: in.AssignmentID, CanonicalTripReference: in.CanonicalTripReference, ActorType: in.ActorType, ActorReference: in.ActorReference, MediaFileID: in.MediaFileID, ValueHash: valueHash, SafeValueMetadata: raw(in.SafeValueMetadata, "{}"), Latitude: in.Latitude, Longitude: in.Longitude, Accuracy: in.Accuracy, SourceTimestamp: in.SourceTimestamp, Source: in.Source, VerificationStatus: verified, SourceVersion: in.SourceVersion, IdempotencyKey: in.IdempotencyKey}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanRecord(tx.QueryRow(ctx, `INSERT INTO fleet_partner_proof_records(id,organization_id,policy_snapshot_reference,proof_type,order_id,shipment_id,package_id,route_stop_id,assignment_id,canonical_trip_reference,actor_type,actor_reference,media_file_id,value_hash,safe_value_metadata,latitude,longitude,accuracy,source_timestamp,source,verification_status,source_version,idempotency_key)VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23) RETURNING `+recordCols, v.ID, v.OrganizationID, v.PolicySnapshotReference, v.ProofType, v.OrderID, v.ShipmentID, v.PackageID, v.RouteStopID, v.AssignmentID, v.CanonicalTripReference, v.ActorType, v.ActorReference, v.MediaFileID, v.ValueHash, v.SafeValueMetadata, v.Latitude, v.Longitude, v.Accuracy, v.SourceTimestamp, v.Source, v.VerificationStatus, v.SourceVersion, v.IdempotencyKey))
		if x != nil {
			return x
		}
		if verified == "VERIFIED" {
			_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,'VERIFIED','MEDIA',$4,$5,'["MEDIA_SAFE_AND_OWNED"]')`, ids.New("prfver"), t.OrganizationID, v.ID, v.MediaFileID, v.SourceVersion)
			if x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "proof_record.created.v1", "proof_record", v.ID, v.Version, map[string]any{"proofRecordId": v.ID, "proofType": v.ProofType, "policySnapshotReference": v.PolicySnapshotReference, "verificationStatus": v.VerificationStatus})
	})
	return v, e
}
func (s *Service) ListRecords(ctx context.Context, t tenant.Context) ([]Record, error) {
	if e := s.gate(s.o.Flags.Records, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 200`, t.OrganizationID)
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
func (s *Service) MandatorySatisfied(ctx context.Context, org, snapshot, order, shipment, pkg, stop string) (bool, error) {
	_, p, e := s.snapshot(ctx, org, snapshot)
	if e != nil {
		return false, e
	}
	var required []string
	_ = json.Unmarshal(p.RequiredProofTypes, &required)
	for _, typ := range required {
		var n int
		e = s.o.Pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_proof_records r WHERE r.organization_id=$1 AND r.policy_snapshot_reference=$2 AND r.proof_type=$3 AND($4=''OR r.order_id=$4)AND($5=''OR r.shipment_id=$5)AND($6=''OR r.package_id=$6)AND($7=''OR r.route_stop_id=$7) AND(r.verification_status='VERIFIED' OR EXISTS(SELECT 1 FROM fleet_partner_proof_verifications v WHERE v.proof_record_id=r.id AND v.status='VERIFIED'))`, org, snapshot, strings.ToUpper(typ), order, shipment, pkg, stop).Scan(&n)
		if e != nil {
			return false, e
		}
		if n < p.MinimumCount {
			return false, nil
		}
	}
	return true, nil
}
func raw(v json.RawMessage, d string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(d)
	}
	return v
}
