// Package usecases implements the Section 19 proof workflows: policy
// publication, proof capture, organization verification decisions and the
// media reconciliation sweep. Media owns bytes; execution services own trip
// completion.
package usecases

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Repository is the persistence this feature needs. A nil Querier runs on the
// pool; a transaction composes the calls atomically.
type Repository interface {
	InsertPolicy(ctx context.Context, q database.Querier, v models.Policy) (models.Policy, error)
	UpdatePolicy(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Policy) (models.Policy, error)
	InsertSnapshot(ctx context.Context, q database.Querier, snap *models.PolicySnapshot) error
	GetPolicy(ctx context.Context, org, id string) (models.Policy, error)
	ListPolicies(ctx context.Context, org string) ([]models.Policy, error)
	GetSnapshot(ctx context.Context, org, id string) (models.PolicySnapshot, error)
	GetRecordByIdempotencyKey(ctx context.Context, org, key string) (models.Record, error)
	GetRecord(ctx context.Context, q database.Querier, org, id string, share bool) (models.Record, error)
	InsertRecord(ctx context.Context, q database.Querier, v models.Record) (models.Record, error)
	InsertMediaVerified(ctx context.Context, q database.Querier, org, recordID, mediaID string, sourceVersion int64) error
	ListRecords(ctx context.Context, org string) ([]models.Record, error)
	CountSatisfying(ctx context.Context, org, snapshot, proofType, order, shipment, pkg, stop string) (int, error)
	ListVerifications(ctx context.Context, q database.Querier, org, id string) ([]models.Verification, error)
	LockDecision(ctx context.Context, q database.Querier, recordID string) error
	InsertDecision(ctx context.Context, q database.Querier, org, recordID, decision, authority, actor string, reasonCodes []byte) error
	ActiveOrganizationIDs(ctx context.Context) ([]string, error)
	ListPendingMediaRecords(ctx context.Context, org string) ([]models.PendingMediaRecord, error)
	InsertMediaReconciliation(ctx context.Context, org, recordID, status, mediaID string, sourceVersion int64, reason string) error
}

type Options struct {
	Pool       *pgxpool.Pool
	Repository Repository
	Events     Events
	Audit      *audit.Recorder
	Media      platformclients.MediaContract
	Queue      *jobs.Queue
	HashSecret []byte
	Flags      models.Flags
	Now        func() time.Time
}
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
	return &Service{o: o, repo: o.Repository}
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

func (s *Service) SavePolicy(ctx context.Context, t tenant.Context, id string, expected int64, v models.Policy) (models.Policy, models.PolicySnapshot, error) {
	if e := s.gate(s.o.Flags.Policies, true); e != nil {
		return v, models.PolicySnapshot{}, e
	}
	v.PolicyScope = strings.ToUpper(v.PolicyScope)
	v.FailurePolicy = strings.ToUpper(v.FailurePolicy)
	if v.Status == "" {
		v.Status = "PUBLISHED"
	}
	v.Status = strings.ToUpper(v.Status)
	v.RequiredProofTypes = models.Raw(v.RequiredProofTypes, "[]")
	v.TimeWindowRules = models.Raw(v.TimeWindowRules, "{}")
	var types []string
	if json.Unmarshal(v.RequiredProofTypes, &types) != nil || len(types) == 0 {
		return v, models.PolicySnapshot{}, apperrors.BadRequest("At least one valid required proof type is required.", nil)
	}
	for i := range types {
		types[i] = strings.ToUpper(strings.TrimSpace(types[i]))
		if !models.SupportedProofTypes[types[i]] {
			return v, models.PolicySnapshot{}, apperrors.BadRequest("Policy contains an unsupported proof type.", nil)
		}
	}
	v.RequiredProofTypes, _ = json.Marshal(types)
	if v.MinimumCount <= 0 {
		v.MinimumCount = 1
	}
	if v.PolicyScope == "" || v.FailurePolicy == "" || (v.FailurePolicy != "BLOCK" && v.FailurePolicy != "WARN") {
		return v, models.PolicySnapshot{}, apperrors.BadRequest("Policy scope and failure policy are required.", nil)
	}
	if v.Status != "DRAFT" && v.Status != "PUBLISHED" && v.Status != "INACTIVE" && v.Status != "ARCHIVED" {
		return v, models.PolicySnapshot{}, apperrors.BadRequest("Policy status is invalid.", nil)
	}
	var snap models.PolicySnapshot
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		if id == "" {
			v.ID = ids.New("prfpol")
			v.OrganizationID = t.OrganizationID
			v.CreatedBy = t.ActorID
			v, x = s.repo.InsertPolicy(ctx, tx, v)
		} else {
			if expected <= 0 {
				return apperrors.BadRequest("expectedVersion is required.", nil)
			}
			v, x = s.repo.UpdatePolicy(ctx, tx, t.OrganizationID, id, expected, v)
			if errors.Is(x, pgx.ErrNoRows) {
				return apperrors.Conflict("Version conflict.", nil)
			}
		}
		if x != nil {
			return x
		}
		if v.Status == "PUBLISHED" {
			snapshot, _ := json.Marshal(v)
			snap = models.PolicySnapshot{ID: ids.New("prfsnp"), OrganizationID: t.OrganizationID, PolicyID: v.ID, PolicyVersion: v.Version, Snapshot: snapshot, PublishedBy: t.ActorID}
			if x = s.repo.InsertSnapshot(ctx, tx, &snap); x != nil {
				return x
			}
			return s.record(ctx, tx, t, "proof_policy.published.v1", "proof_policy", v.ID, v.Version, map[string]any{"policyId": v.ID, "snapshotReference": snap.ID, "version": v.Version})
		}
		return s.record(ctx, tx, t, "proof_policy.saved.v1", "proof_policy", v.ID, v.Version, map[string]any{"policyId": v.ID, "status": v.Status, "version": v.Version})
	})
	return v, snap, e
}
func (s *Service) GetPolicy(ctx context.Context, t tenant.Context, id string) (models.Policy, error) {
	if e := s.gate(s.o.Flags.Policies, false); e != nil {
		return models.Policy{}, e
	}
	v, e := s.repo.GetPolicy(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Proof policy not found.", nil)
	}
	return v, e
}
func (s *Service) ListPolicies(ctx context.Context, t tenant.Context) ([]models.Policy, error) {
	if e := s.gate(s.o.Flags.Policies, false); e != nil {
		return nil, e
	}
	return s.repo.ListPolicies(ctx, t.OrganizationID)
}
func (s *Service) snapshot(ctx context.Context, org, id string) (models.PolicySnapshot, models.Policy, error) {
	snap, e := s.repo.GetSnapshot(ctx, org, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return snap, models.Policy{}, apperrors.NotFound("Proof policy snapshot not found.", nil)
	}
	if e != nil {
		return snap, models.Policy{}, e
	}
	var p models.Policy
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

func (s *Service) CreateRecord(ctx context.Context, t tenant.Context, in models.RecordInput) (models.Record, error) {
	if e := s.gate(s.o.Flags.Records, true); e != nil {
		return models.Record{}, e
	}
	if in.IdempotencyKey == "" {
		return models.Record{}, apperrors.BadRequest("Idempotency key is required.", nil)
	}
	existing, e := s.repo.GetRecordByIdempotencyKey(ctx, t.OrganizationID, in.IdempotencyKey)
	if e == nil {
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return models.Record{}, e
	}
	_, p, e := s.snapshot(ctx, t.OrganizationID, in.PolicySnapshotReference)
	if e != nil {
		return models.Record{}, e
	}
	in.ProofType = strings.ToUpper(in.ProofType)
	if !models.SupportedProofTypes[in.ProofType] {
		return models.Record{}, apperrors.BadRequest("Proof type is invalid.", nil)
	}
	if !models.RequiresType(p, in.ProofType) {
		return models.Record{}, apperrors.Conflict("Proof type is not required by the policy snapshot.", nil)
	}
	if p.LocationRequired && (in.Latitude == nil || in.Longitude == nil) {
		return models.Record{}, apperrors.BadRequest("Location proof is required by policy.", nil)
	}
	verified := "PENDING"
	valueHash := ""
	if in.ProofType == "OTP" {
		if strings.TrimSpace(in.Value) == "" {
			return models.Record{}, apperrors.BadRequest("OTP proof value is required.", nil)
		}
		valueHash, e = s.hash(t.OrganizationID, in.PolicySnapshotReference, in.ProofType, in.Value)
		if e != nil {
			return models.Record{}, e
		}
		in.Value = ""
	}
	if (in.ProofType == "PHOTO" || in.ProofType == "SIGNATURE" || in.ProofType == "DOCUMENT_SCAN") && in.MediaFileID == "" {
		return models.Record{}, apperrors.BadRequest("Media evidence is required for this proof type.", nil)
	}
	if in.ProofType == "LOCATION" && (in.Latitude == nil || in.Longitude == nil) {
		return models.Record{}, apperrors.BadRequest("Coordinates are required for location proof.", nil)
	}
	if in.MediaFileID != "" {
		if e = s.media(ctx, t, in.MediaFileID); e != nil {
			return models.Record{}, e
		}
		verified = "VERIFIED"
	}
	if in.SourceTimestamp.IsZero() {
		return models.Record{}, apperrors.BadRequest("sourceTimestamp is required.", nil)
	}
	if in.ActorType == "" || in.Source == "" {
		return models.Record{}, apperrors.BadRequest("Actor type and source are required.", nil)
	}
	if in.OrderID == "" && in.ShipmentID == "" && in.PackageID == "" && in.RouteStopID == "" && in.AssignmentID == "" && in.CanonicalTripReference == "" {
		return models.Record{}, apperrors.BadRequest("Proof must reference an operation subject.", nil)
	}
	if in.ActorReference == "" {
		in.ActorReference = t.ActorID
	}
	v := models.Record{ID: ids.New("prfrec"), OrganizationID: t.OrganizationID, PolicySnapshotReference: in.PolicySnapshotReference, ProofType: in.ProofType, OrderID: in.OrderID, ShipmentID: in.ShipmentID, PackageID: in.PackageID, RouteStopID: in.RouteStopID, AssignmentID: in.AssignmentID, CanonicalTripReference: in.CanonicalTripReference, ActorType: in.ActorType, ActorReference: in.ActorReference, MediaFileID: in.MediaFileID, ValueHash: valueHash, SafeValueMetadata: models.Raw(in.SafeValueMetadata, "{}"), Latitude: in.Latitude, Longitude: in.Longitude, Accuracy: in.Accuracy, SourceTimestamp: in.SourceTimestamp, Source: in.Source, VerificationStatus: verified, SourceVersion: in.SourceVersion, IdempotencyKey: in.IdempotencyKey}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertRecord(ctx, tx, v)
		if x != nil {
			return x
		}
		if verified == "VERIFIED" {
			if x = s.repo.InsertMediaVerified(ctx, tx, t.OrganizationID, v.ID, v.MediaFileID, v.SourceVersion); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "proof_record.created.v1", "proof_record", v.ID, v.Version, map[string]any{"proofRecordId": v.ID, "proofType": v.ProofType, "policySnapshotReference": v.PolicySnapshotReference, "verificationStatus": v.VerificationStatus})
	})
	return v, e
}
func (s *Service) ListRecords(ctx context.Context, t tenant.Context) ([]models.Record, error) {
	if e := s.gate(s.o.Flags.Records, false); e != nil {
		return nil, e
	}
	return s.repo.ListRecords(ctx, t.OrganizationID)
}
func (s *Service) MandatorySatisfied(ctx context.Context, org, snapshot, order, shipment, pkg, stop string) (bool, error) {
	_, p, e := s.snapshot(ctx, org, snapshot)
	if e != nil {
		return false, e
	}
	var required []string
	_ = json.Unmarshal(p.RequiredProofTypes, &required)
	for _, typ := range required {
		n, e := s.repo.CountSatisfying(ctx, org, snapshot, typ, order, shipment, pkg, stop)
		if e != nil {
			return false, e
		}
		if n < p.MinimumCount {
			return false, nil
		}
	}
	return true, nil
}
