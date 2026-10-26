package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/platform/audit"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminmedia"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// LegacyContent reads legacy media_assets bytes from Media's own legacy
// storage backend. It is the existing upload storage client, never a URL a
// caller supplies: the adopted bytes never travel through the attesting owner.
type LegacyContent interface {
	Download(ctx context.Context, path string) (io.ReadCloser, error)
}

// legacyOwnerIdentities maps a modern, HMAC-authenticated source owner to the
// legacy upload identity it used for media_assets.owner_service. It is fixed
// code: only an owner listed here can ever adopt, and only its own uploads.
var legacyOwnerIdentities = map[string]string{
	"driver-dispatch-delivery-service": "dispatch-delivery-service",
}

// legacyPurposeMap is the fixed vocabulary bridge from a legacy upload purpose
// to the modern evidence purposes it may be adopted as. Personal or identity
// material (profile photos, signatures, identity documents) is never mapped.
var legacyPurposeMap = map[string][]string{
	"proof_image": {models.PurposeCargoCondition, models.PurposeDamageEvidence, models.PurposeRecoveryTransfer, models.PurposeIncidentAttachment},
}

// LegacyIdentity reports the legacy owner identity a modern owner may adopt.
func LegacyIdentity(owner string) (string, bool) {
	v, ok := legacyOwnerIdentities[owner]
	return v, ok
}

type LegacyAdoptionConfig struct {
	Content LegacyContent
	// Bucket is the configured legacy bucket; an asset recorded in any other
	// bucket cannot be read authoritatively and is refused.
	Bucket   string
	MaxBytes int64
}

type LegacyAdoptionService struct {
	media  *Service
	config LegacyAdoptionConfig
}

func (s *Service) NewLegacyAdoption(config LegacyAdoptionConfig) *LegacyAdoptionService {
	return &LegacyAdoptionService{media: s, config: config}
}

var ErrLegacyDenied = errors.New("media: legacy asset adoption denied")

type legacyAsset struct {
	ownerService, ownerID, purpose, contentType, checksum, bucket, path, status, fileName string
	size                                                                                  int64
	deleted                                                                               bool
}

// Adopt records an owner's attestation that legacy asset X is evidence for
// its case Y / subject Z with purpose P. Idempotent by legacy asset ID; a
// different attestation for an adopted asset is a conflict.
func (l *LegacyAdoptionService) Adopt(ctx context.Context, caller string, a adminmedia.LegacyAdoption) (adminmedia.LegacyAdoptionReceipt, error) {
	var empty adminmedia.LegacyAdoptionReceipt
	if l == nil || l.media == nil || l.media.opts.Pool == nil || l.config.Content == nil || l.config.Bucket == "" {
		return empty, ErrUnavailable
	}
	legacyOwner, ok := legacyOwnerIdentities[caller]
	if !ok || !a.Valid(caller) {
		return empty, ErrLegacyDenied
	}
	spec, _ := admincase.Find(caller, a.CaseKind)
	digest := a.Digest(caller)
	// Serialize every adoption of one legacy asset across replicas.
	conn, err := l.media.opts.Pool.Acquire(ctx)
	if err != nil {
		return empty, err
	}
	defer conn.Release()
	lockKey := "media-legacy-adoption:" + a.LegacyAssetID
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return empty, err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1,0))`, lockKey)

	if receipt, found, err := l.existing(ctx, a.LegacyAssetID, digest); found || err != nil {
		return receipt, err
	}
	var asset legacyAsset
	err = l.media.opts.Pool.QueryRow(ctx, `SELECT owner_service,owner_id,purpose,content_type,checksum_sha256,storage_bucket,storage_path,status,original_filename,size_bytes,deleted_at IS NOT NULL FROM media_assets WHERE id=$1::uuid`, a.LegacyAssetID).Scan(&asset.ownerService, &asset.ownerID, &asset.purpose, &asset.contentType, &asset.checksum, &asset.bucket, &asset.path, &asset.status, &asset.fileName, &asset.size, &asset.deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	// The legacy row must belong to exactly this owner and reference, still be
	// active (not revoked, quarantined or deleted) and map to the attested purpose.
	allowed := false
	for _, purpose := range legacyPurposeMap[asset.purpose] {
		allowed = allowed || purpose == a.Purpose
	}
	if asset.ownerService != legacyOwner || asset.ownerID != a.LegacyOwnerReference {
		return empty, ErrLegacyDenied
	}
	if asset.status != "active" || asset.deleted || !allowed || !admincase.Contains(spec.Purposes, a.Purpose) || asset.bucket != l.config.Bucket {
		return empty, ErrLegacyDenied
	}
	max := l.config.MaxBytes
	if max <= 0 {
		max = 32 << 20
	}
	if asset.size <= 0 || asset.size > max || len(asset.checksum) != 64 {
		return empty, ErrLegacyDenied
	}
	reader, err := l.config.Content.Download(ctx, asset.path)
	if err != nil {
		return empty, ErrUnavailable
	}
	content, err := io.ReadAll(io.LimitReader(reader, max+1))
	reader.Close()
	if err != nil {
		return empty, ErrUnavailable
	}
	sum := sha256.Sum256(content)
	if int64(len(content)) != asset.size || !strings.EqualFold(hex.EncodeToString(sum[:]), asset.checksum) {
		// Stored bytes no longer match the recorded checksum: never adopt.
		return empty, ErrLegacyDenied
	}
	object, err := l.materialize(ctx, caller, a, asset, digest, content)
	if err != nil {
		return empty, err
	}
	if object.ChecksumSHA256 != hex.EncodeToString(sum[:]) || !object.BoundTo(spec.BindingType, a.CaseReference) || object.OwnerService != caller || object.OrganizationID != a.OrganizationID {
		return empty, ErrConflict
	}
	now := l.media.opts.Now().UTC()
	tx, err := l.media.opts.Pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO media_legacy_adoptions(legacy_asset_id,media_object_id,owner_service,legacy_owner_service,legacy_owner_reference,legacy_purpose,organization_id,case_kind,case_reference,subject_type,subject_reference,purpose,checksum_sha256,attestation_digest,created_at)VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, a.LegacyAssetID, object.ID, caller, legacyOwner, a.LegacyOwnerReference, asset.purpose, a.OrganizationID, a.CaseKind, a.CaseReference, a.SubjectType, a.SubjectReference, a.Purpose, object.ChecksumSHA256, digest, now); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return empty, ErrConflict
		}
		return empty, err
	}
	if err := l.media.opts.Audit.Record(ctx, tx, audit.Entry{OrganizationID: a.OrganizationID, ActorType: "SERVICE", ActorID: caller, Action: "media.legacy_asset.adopted", ResourceType: "media_object", ResourceID: object.ID, NewValues: map[string]any{"legacyAssetId": a.LegacyAssetID, "caseOwner": caller, "caseKind": a.CaseKind, "caseReference": a.CaseReference, "purpose": a.Purpose}}); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return adminmedia.LegacyAdoptionReceipt{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: a.LegacyAssetID, MediaObjectID: object.ID, ChecksumSHA256: object.ChecksumSHA256, AttestationDigest: digest, RecordedAt: now}, nil
}

func (l *LegacyAdoptionService) existing(ctx context.Context, legacyID, digest string) (adminmedia.LegacyAdoptionReceipt, bool, error) {
	var out adminmedia.LegacyAdoptionReceipt
	var stored string
	err := l.media.opts.Pool.QueryRow(ctx, `SELECT media_object_id,checksum_sha256,attestation_digest,created_at FROM media_legacy_adoptions WHERE legacy_asset_id=$1::uuid`, legacyID).Scan(&out.MediaObjectID, &out.ChecksumSHA256, &stored, &out.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, true, err
	}
	if stored != digest {
		return adminmedia.LegacyAdoptionReceipt{}, true, ErrConflict
	}
	out.ContractVersion, out.LegacyAssetID, out.AttestationDigest, out.Replayed = adminmedia.LegacyAdoptionContract, legacyID, digest, true
	out.RecordedAt = out.RecordedAt.UTC()
	return out, true, nil
}

// materialize creates (or resumes) the modern object through the ordinary
// authorize/complete pipeline under the attesting owner's identity, so the
// existing signature, MIME, scanning, privacy and binding rules all apply.
// The idempotency key is the legacy asset ID; the fingerprint is the digest.
func (l *LegacyAdoptionService) materialize(ctx context.Context, caller string, a adminmedia.LegacyAdoption, asset legacyAsset, digest string, content []byte) (*models.Object, error) {
	spec, _ := admincase.Find(caller, a.CaseKind)
	key := "legacy-adoption:" + a.LegacyAssetID
	identity := Caller{Service: caller, OrganizationID: a.OrganizationID}
	if found, err := l.media.opts.Objects.AuthorizationByIdempotencyKey(ctx, nil, caller, a.OrganizationID, key); err == nil {
		if found.RequestFingerprint != digest {
			return nil, ErrConflict
		}
		if found.State == models.AuthorizationConsumed {
			object, err := l.media.opts.Objects.GetObject(ctx, nil, found.MediaObjectID)
			if err != nil {
				return nil, err
			}
			if object.UploadStatus != models.UploadCompleted {
				return nil, ErrUnavailable
			}
			return object, nil
		}
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	name := asset.fileName
	if name == "" || len(name) > 200 {
		name = "legacy-" + a.LegacyAssetID
	}
	grant, err := l.media.AuthorizeUpload(ctx, AuthorizeUploadInput{Caller: identity, OwnerType: a.SubjectType, OwnerReference: a.SubjectReference, UploadPurpose: a.Purpose, Visibility: models.VisibilityPrivate, FileName: name, AllowedMIMETypes: []string{asset.contentType}, MaxSizeBytes: asset.size, RetentionClass: models.DefaultRetentionFor(a.Purpose), IdempotencyKey: key, RequestFingerprint: digest, BindingType: spec.BindingType, BindingID: a.CaseReference})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("%w: %v", ErrLegacyDenied, err)
	}
	object, err := l.media.CompleteUpload(ctx, CompleteUploadInput{Caller: identity, MediaObjectID: grant.MediaObjectID, AuthorizationID: grant.AuthorizationID, UploadToken: grant.UploadToken, DeclaredMIME: asset.contentType, DeclaredSize: asset.size, DeclaredChecksum: asset.checksum, Content: content})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLegacyDenied, err)
	}
	if object.UploadStatus != models.UploadCompleted {
		return nil, ErrLegacyDenied
	}
	object.SecondaryBindings, err = l.media.opts.Objects.SecondaryBindings(ctx, nil, object.ID)
	if err != nil {
		return nil, err
	}
	return object, nil
}

// LegacyMapping resolves an adopted legacy asset to its modern object for the
// attesting owner. Unmapped legacy IDs have no modern identity.
func (l *LegacyAdoptionService) LegacyMapping(ctx context.Context, legacyID string) (string, error) {
	var id string
	err := l.media.opts.Pool.QueryRow(ctx, `SELECT media_object_id FROM media_legacy_adoptions WHERE legacy_asset_id=$1::uuid`, legacyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// ErrLegacyMappingInconsistent means a stored mapping row does not agree with
// the legacy row, the modern object or its own attestation digest — for
// example a row written outside the adoption pipeline. It is never served.
var ErrLegacyMappingInconsistent = errors.New("media: legacy mapping inconsistent")

// LookupMapping resolves an adopted legacy asset for the private Bridge
// lookup (Section 19). It never infers a relationship: the only source is the
// immutable adoption row, and that row is re-verified against the legacy
// asset, the modern object (owner, tenant, subject, purpose, checksum, case
// binding, privacy) and the owner's recomputed attestation digest before an
// answer of opaque IDs plus the digest is returned. The lookup is audited.
func (l *LegacyAdoptionService) LookupMapping(ctx context.Context, actor, legacyID string) (adminmedia.LegacyMapping, error) {
	var empty adminmedia.LegacyMapping
	if l == nil || l.media == nil || l.media.opts.Pool == nil {
		return empty, ErrUnavailable
	}
	var a adminmedia.LegacyAdoption
	var objectID, owner, legacyOwner, legacyPurpose, checksum, digest string
	var assetOwner, assetOwnerID, assetPurpose, assetChecksum string
	err := l.media.opts.Pool.QueryRow(ctx, `SELECT a.media_object_id,a.owner_service,a.legacy_owner_service,a.legacy_owner_reference,a.legacy_purpose,a.organization_id,a.case_kind,a.case_reference,a.subject_type,a.subject_reference,a.purpose,a.checksum_sha256,a.attestation_digest,m.owner_service,m.owner_id,m.purpose,m.checksum_sha256 FROM media_legacy_adoptions a JOIN media_assets m ON m.id=a.legacy_asset_id WHERE a.legacy_asset_id=$1::uuid`, legacyID).
		Scan(&objectID, &owner, &legacyOwner, &a.LegacyOwnerReference, &legacyPurpose, &a.OrganizationID, &a.CaseKind, &a.CaseReference, &a.SubjectType, &a.SubjectReference, &a.Purpose, &checksum, &digest, &assetOwner, &assetOwnerID, &assetPurpose, &assetChecksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, ErrUnavailable
	}
	a.ContractVersion, a.LegacyAssetID = adminmedia.LegacyAdoptionContract, legacyID
	expectedLegacyOwner, known := legacyOwnerIdentities[owner]
	allowed := false
	for _, purpose := range legacyPurposeMap[assetPurpose] {
		allowed = allowed || purpose == a.Purpose
	}
	if !known || legacyOwner != expectedLegacyOwner || assetOwner != legacyOwner || assetOwnerID != a.LegacyOwnerReference ||
		assetPurpose != legacyPurpose || !allowed || !strings.EqualFold(assetChecksum, checksum) ||
		!a.Valid(owner) || a.Digest(owner) != digest {
		return empty, ErrLegacyMappingInconsistent
	}
	object, err := l.media.opts.Objects.GetObject(ctx, nil, objectID)
	if err != nil {
		return empty, ErrLegacyMappingInconsistent
	}
	object.SecondaryBindings, err = l.media.opts.Objects.SecondaryBindings(ctx, nil, object.ID)
	if err != nil {
		return empty, ErrUnavailable
	}
	spec, _ := admincase.Find(owner, a.CaseKind)
	if object.ChecksumSHA256 != checksum || object.OwnerService != owner || object.OrganizationID != a.OrganizationID ||
		object.OwnerType != a.SubjectType || object.OwnerReference != a.SubjectReference || object.UploadPurpose != a.Purpose ||
		object.Visibility != models.VisibilityPrivate || !object.BoundTo(spec.BindingType, a.CaseReference) {
		return empty, ErrLegacyMappingInconsistent
	}
	if err := l.media.opts.Audit.Record(ctx, l.media.opts.Pool, audit.Entry{OrganizationID: a.OrganizationID, ActorType: "SERVICE", ActorID: actor, Action: "media.legacy_mapping.resolved", ResourceType: "media_object", ResourceID: object.ID, NewValues: map[string]any{"legacyAssetId": legacyID}}); err != nil {
		return empty, ErrUnavailable
	}
	return adminmedia.LegacyMapping{ContractVersion: adminmedia.LegacyMappingContract, LegacyAssetID: legacyID, MediaObjectID: object.ID, AttestationDigest: digest}, nil
}
