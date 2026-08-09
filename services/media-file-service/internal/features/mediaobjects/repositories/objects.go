// Package repositories holds the PostgreSQL persistence for authoritative
// media objects.
//
// Every write takes the caller's querier so the object row, its history, its
// audit record and its outbox event commit in one transaction. A repository
// method that opened its own transaction would make that guarantee impossible
// to keep.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/media-file-service/internal/database"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/platform/ids"
)

// Sentinel errors the use cases branch on.
var (
	ErrNotFound        = errors.New("media: record not found")
	ErrConflict        = errors.New("media: conflicting record")
	ErrVersionMismatch = errors.New("media: expected version does not match")
)

const objectColumns = `
	id, organization_id, owner_service, owner_type, owner_reference, upload_purpose,
	declared_mime, detected_mime, size_bytes, checksum_sha256, signature_validation,
	captured_at, source_timestamp, capture_method, device_reference, actor_reference,
	original_mime_type, original_size_bytes, integrity_hash,
	upload_status, safety_status, safety_reason, scanned_at, validation_failure_code,
	storage_bucket, storage_object_key, deletion_state, deletion_requested_at, deleted_at,
	deletion_failure_code, retention_class, retention_until, legal_hold, version,
	correlation_id, causation_id, created_at, updated_at`

// Repository reads and writes authoritative media state.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository over a pool or transaction.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) querier(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

type scannable interface{ Scan(dest ...any) error }

func scanObject(row scannable) (*models.Object, error) {
	var object models.Object
	err := row.Scan(
		&object.ID, &object.OrganizationID, &object.OwnerService, &object.OwnerType,
		&object.OwnerReference, &object.UploadPurpose, &object.DeclaredMIME, &object.DetectedMIME,
		&object.SizeBytes, &object.ChecksumSHA256, &object.SignatureValidation,
		&object.CapturedAt, &object.SourceTimestamp, &object.CaptureMethod,
		&object.DeviceReference, &object.ActorReference, &object.OriginalMIMEType,
		&object.OriginalSizeBytes, &object.IntegrityHash,
		&object.UploadStatus, &object.SafetyStatus, &object.SafetyReason, &object.ScannedAt,
		&object.ValidationFailureCode, &object.StorageBucket, &object.StorageObjectKey,
		&object.DeletionState, &object.DeletionRequestedAt, &object.DeletedAt,
		&object.DeletionFailureCode, &object.RetentionClass, &object.RetentionUntil,
		&object.LegalHold, &object.Version, &object.CorrelationID, &object.CausationID,
		&object.CreatedAt, &object.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: scan object: %w", err)
	}
	return &object, nil
}

// CreateObject inserts a media object in its pre-upload state.
func (r *Repository) CreateObject(ctx context.Context, q database.Querier, object *models.Object) error {
	row := r.querier(q).QueryRow(ctx, `
		INSERT INTO media_objects
			(id, organization_id, owner_service, owner_type, owner_reference, upload_purpose,
			 declared_mime, upload_status, safety_status, storage_bucket, storage_object_key,
			 retention_class, retention_until, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING `+objectColumns,
		object.ID, object.OrganizationID, object.OwnerService, object.OwnerType,
		object.OwnerReference, object.UploadPurpose, object.DeclaredMIME, object.UploadStatus,
		object.SafetyStatus, object.StorageBucket, object.StorageObjectKey,
		object.RetentionClass, object.RetentionUntil, object.CorrelationID, object.CausationID)

	created, err := scanObject(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	*object = *created
	return nil
}

// GetObject reads one object without tenant scoping. Only reconciliation and
// worker paths use it; every caller-facing read goes through GetObjectForOwner.
func (r *Repository) GetObject(ctx context.Context, q database.Querier, id string) (*models.Object, error) {
	return scanObject(r.querier(q).QueryRow(ctx,
		`SELECT `+objectColumns+` FROM media_objects WHERE id = $1`, id))
}

// LockObject reads one object FOR UPDATE inside the caller's transaction.
func (r *Repository) LockObject(ctx context.Context, q database.Querier, id string) (*models.Object, error) {
	return scanObject(r.querier(q).QueryRow(ctx,
		`SELECT `+objectColumns+` FROM media_objects WHERE id = $1 FOR UPDATE`, id))
}

// GetObjectForOwner reads one object inside the owning service and tenant.
//
// A mismatch answers ErrNotFound rather than a permission error, so the
// contract cannot be used to discover that another tenant holds an object with
// a guessed identifier.
func (r *Repository) GetObjectForOwner(ctx context.Context, q database.Querier,
	callerService string, organizationID string, id string) (*models.Object, error) {
	return scanObject(r.querier(q).QueryRow(ctx,
		`SELECT `+objectColumns+` FROM media_objects
		 WHERE id = $1 AND owner_service = $2 AND organization_id = $3`,
		id, callerService, organizationID))
}

// UpdateObject writes a new object state with an optimistic version check.
//
// expectedVersion must be the version the caller read. A mismatch means
// something else changed the object in between, and the write is refused
// rather than overwriting an answer the caller never saw.
func (r *Repository) UpdateObject(ctx context.Context, q database.Querier,
	object *models.Object, expectedVersion int64) error {
	row := r.querier(q).QueryRow(ctx, `
		UPDATE media_objects SET
			detected_mime = $1,
			size_bytes = $2,
			checksum_sha256 = $3,
			signature_validation = $4,
			captured_at = $5,
			source_timestamp = $6,
			capture_method = $7,
			device_reference = $8,
			actor_reference = $9,
			original_mime_type = $10,
			original_size_bytes = $11,
			integrity_hash = $12,
			upload_status = $13,
			safety_status = $14,
			safety_reason = $15,
			scanned_at = $16,
			validation_failure_code = $17,
			deletion_state = $18,
			deletion_requested_at = $19,
			deleted_at = $20,
			deletion_failure_code = $21,
			retention_class = $22,
			retention_until = $23,
			legal_hold = $24,
			version = version + 1,
			updated_at = now()
		WHERE id = $25 AND version = $26
		RETURNING `+objectColumns,
		object.DetectedMIME, object.SizeBytes, object.ChecksumSHA256, object.SignatureValidation,
		object.CapturedAt, object.SourceTimestamp, object.CaptureMethod, object.DeviceReference,
		object.ActorReference, object.OriginalMIMEType, object.OriginalSizeBytes, object.IntegrityHash,
		object.UploadStatus, object.SafetyStatus, object.SafetyReason, object.ScannedAt,
		object.ValidationFailureCode, object.DeletionState, object.DeletionRequestedAt,
		object.DeletedAt, object.DeletionFailureCode, object.RetentionClass, object.RetentionUntil,
		object.LegalHold, object.ID, expectedVersion)

	updated, err := scanObject(row)
	if errors.Is(err, ErrNotFound) {
		return ErrVersionMismatch
	}
	if err != nil {
		return err
	}
	*object = *updated
	return nil
}

// ExpireStaleUploads marks authorized-but-never-completed objects EXPIRED.
func (r *Repository) ExpireStaleUploads(ctx context.Context, q database.Querier, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE media_objects SET upload_status = $1, version = version + 1, updated_at = now()
		WHERE id IN (
			SELECT o.id FROM media_objects o
			JOIN media_upload_authorizations a ON a.media_object_id = o.id
			WHERE o.upload_status IN ($2, $3)
			  AND a.expires_at < now()
			LIMIT $4
		)`, models.UploadExpired, models.UploadAuthorized, models.UploadUploading, limit)
	if err != nil {
		return 0, fmt.Errorf("media: expire stale uploads: %w", err)
	}
	return tag.RowsAffected(), nil
}

const authorizationColumns = `
	id, media_object_id, organization_id, caller_service, owner_service, owner_type,
	owner_reference, document_category, upload_purpose, allowed_mime_types, max_size_bytes,
	upload_token_hash, state, expires_at, consumed_at, idempotency_key, request_fingerprint,
	correlation_id, causation_id, payload_version, created_at, updated_at`

func scanAuthorization(row scannable) (*models.UploadAuthorization, error) {
	var authorization models.UploadAuthorization
	err := row.Scan(
		&authorization.ID, &authorization.MediaObjectID, &authorization.OrganizationID,
		&authorization.CallerService, &authorization.OwnerService, &authorization.OwnerType,
		&authorization.OwnerReference, &authorization.DocumentCategory, &authorization.UploadPurpose,
		&authorization.AllowedMIMETypes, &authorization.MaxSizeBytes, &authorization.UploadTokenHash,
		&authorization.State, &authorization.ExpiresAt, &authorization.ConsumedAt,
		&authorization.IdempotencyKey, &authorization.RequestFingerprint,
		&authorization.CorrelationID, &authorization.CausationID, &authorization.PayloadVersion,
		&authorization.CreatedAt, &authorization.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: scan upload authorization: %w", err)
	}
	return &authorization, nil
}

// CreateAuthorization inserts a single-use upload grant.
func (r *Repository) CreateAuthorization(ctx context.Context, q database.Querier,
	authorization *models.UploadAuthorization) error {
	row := r.querier(q).QueryRow(ctx, `
		INSERT INTO media_upload_authorizations
			(id, media_object_id, organization_id, caller_service, owner_service, owner_type,
			 owner_reference, document_category, upload_purpose, allowed_mime_types,
			 max_size_bytes, upload_token_hash, state, expires_at, idempotency_key,
			 request_fingerprint, correlation_id, causation_id, payload_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING `+authorizationColumns,
		authorization.ID, authorization.MediaObjectID, authorization.OrganizationID,
		authorization.CallerService, authorization.OwnerService, authorization.OwnerType,
		authorization.OwnerReference, authorization.DocumentCategory, authorization.UploadPurpose,
		authorization.AllowedMIMETypes, authorization.MaxSizeBytes, authorization.UploadTokenHash,
		authorization.State, authorization.ExpiresAt, authorization.IdempotencyKey,
		authorization.RequestFingerprint, authorization.CorrelationID, authorization.CausationID,
		authorization.PayloadVersion)

	created, err := scanAuthorization(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	*authorization = *created
	return nil
}

// LockAuthorization reads one grant FOR UPDATE, which is what serializes two
// concurrent attempts to redeem the same single-use authorization.
func (r *Repository) LockAuthorization(ctx context.Context, q database.Querier,
	id string) (*models.UploadAuthorization, error) {
	return scanAuthorization(r.querier(q).QueryRow(ctx,
		`SELECT `+authorizationColumns+` FROM media_upload_authorizations WHERE id = $1 FOR UPDATE`, id))
}

// AuthorizationByIdempotencyKey resolves a replayed authorization request.
func (r *Repository) AuthorizationByIdempotencyKey(ctx context.Context, q database.Querier,
	callerService string, organizationID string, key string) (*models.UploadAuthorization, error) {
	return scanAuthorization(r.querier(q).QueryRow(ctx,
		`SELECT `+authorizationColumns+` FROM media_upload_authorizations
		 WHERE caller_service = $1 AND organization_id = $2 AND idempotency_key = $3`,
		callerService, organizationID, key))
}

// SetAuthorizationState transitions a grant.
func (r *Repository) SetAuthorizationState(ctx context.Context, q database.Querier,
	id string, state string, consumedAt *time.Time) error {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE media_upload_authorizations
		SET state = $1, consumed_at = COALESCE($2, consumed_at), updated_at = now()
		WHERE id = $3`, state, consumedAt, id)
	if err != nil {
		return fmt.Errorf("media: set authorization state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpireAuthorizations marks lapsed grants EXPIRED so a late upload is refused
// by state rather than by a timestamp comparison somebody might forget.
func (r *Repository) ExpireAuthorizations(ctx context.Context, q database.Querier, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE media_upload_authorizations SET state = $1, updated_at = now()
		WHERE id IN (
			SELECT id FROM media_upload_authorizations
			WHERE state = $2 AND expires_at < now()
			LIMIT $3
		)`, models.AuthorizationExpired, models.AuthorizationIssued, limit)
	if err != nil {
		return 0, fmt.Errorf("media: expire authorizations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AppendHistory writes one append-only history entry.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier,
	entry *models.HistoryEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New(ids.PrefixHistory)
	}
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("media: marshal history metadata: %w", err)
	}
	if len(metadata) == 0 || string(metadata) == "null" {
		metadata = []byte("{}")
	}
	_, err = r.querier(q).Exec(ctx, `
		INSERT INTO media_object_history
			(id, organization_id, media_object_id, action, from_state, to_state,
			 actor_type, actor_id, metadata, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		entry.ID, entry.OrganizationID, entry.MediaObjectID, entry.Action, entry.FromState,
		entry.ToState, entry.ActorType, entry.ActorID, metadata, entry.CorrelationID,
		entry.CausationID)
	if err != nil {
		return fmt.Errorf("media: append history: %w", err)
	}
	return nil
}

// ListHistory returns an object's append-only history, newest first.
func (r *Repository) ListHistory(ctx context.Context, q database.Querier,
	mediaObjectID string) ([]models.HistoryEntry, error) {
	rows, err := r.querier(q).Query(ctx, `
		SELECT id, organization_id, media_object_id, action, from_state, to_state,
		       actor_type, actor_id, metadata, correlation_id, causation_id, occurred_at
		FROM media_object_history WHERE media_object_id = $1 ORDER BY occurred_at DESC, id DESC`,
		mediaObjectID)
	if err != nil {
		return nil, fmt.Errorf("media: list history: %w", err)
	}
	defer rows.Close()

	entries := []models.HistoryEntry{}
	for rows.Next() {
		var entry models.HistoryEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.MediaObjectID, &entry.Action,
			&entry.FromState, &entry.ToState, &entry.ActorType, &entry.ActorID, &metadata,
			&entry.CorrelationID, &entry.CausationID, &entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("media: scan history: %w", err)
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &entry.Metadata)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

const deletionColumns = `
	id, media_object_id, organization_id, requested_by, reason, status, block_reason,
	attempts, last_error_redacted, idempotency_key, request_fingerprint, correlation_id,
	causation_id, created_at, updated_at, completed_at`

func scanDeletion(row scannable) (*models.DeletionRequest, error) {
	var request models.DeletionRequest
	err := row.Scan(&request.ID, &request.MediaObjectID, &request.OrganizationID,
		&request.RequestedBy, &request.Reason, &request.Status, &request.BlockReason,
		&request.Attempts, &request.LastError, &request.IdempotencyKey,
		&request.RequestFingerprint, &request.CorrelationID, &request.CausationID,
		&request.CreatedAt, &request.UpdatedAt, &request.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: scan deletion request: %w", err)
	}
	return &request, nil
}

// CreateDeletionRequest records a durable deletion request.
func (r *Repository) CreateDeletionRequest(ctx context.Context, q database.Querier,
	request *models.DeletionRequest) error {
	row := r.querier(q).QueryRow(ctx, `
		INSERT INTO media_deletion_requests
			(id, media_object_id, organization_id, requested_by, reason, status, block_reason,
			 idempotency_key, request_fingerprint, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+deletionColumns,
		request.ID, request.MediaObjectID, request.OrganizationID, request.RequestedBy,
		request.Reason, request.Status, request.BlockReason, request.IdempotencyKey,
		request.RequestFingerprint, request.CorrelationID, request.CausationID)

	created, err := scanDeletion(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	*request = *created
	return nil
}

// DeletionRequestByIdempotencyKey resolves a replayed deletion request.
func (r *Repository) DeletionRequestByIdempotencyKey(ctx context.Context, q database.Querier,
	requestedBy string, organizationID string, key string) (*models.DeletionRequest, error) {
	return scanDeletion(r.querier(q).QueryRow(ctx,
		`SELECT `+deletionColumns+` FROM media_deletion_requests
		 WHERE requested_by = $1 AND organization_id = $2 AND idempotency_key = $3`,
		requestedBy, organizationID, key))
}

// LatestDeletionRequest returns the newest deletion request for an object,
// which is what a read-back after an UNKNOWN outcome consults.
func (r *Repository) LatestDeletionRequest(ctx context.Context, q database.Querier,
	mediaObjectID string) (*models.DeletionRequest, error) {
	return scanDeletion(r.querier(q).QueryRow(ctx,
		`SELECT `+deletionColumns+` FROM media_deletion_requests
		 WHERE media_object_id = $1 ORDER BY created_at DESC LIMIT 1`, mediaObjectID))
}

// SetDeletionRequestStatus transitions a deletion request.
func (r *Repository) SetDeletionRequestStatus(ctx context.Context, q database.Querier,
	id string, status string, blockReason string, lastError string) error {
	completed := status == models.DeletionRequestCompleted
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE media_deletion_requests SET
			status = $1,
			block_reason = $2,
			last_error_redacted = $3,
			attempts = attempts + 1,
			completed_at = CASE WHEN $4 THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $5`, status, blockReason, lastError, completed, id)
	if err != nil {
		return fmt.Errorf("media: set deletion request status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimPendingDeletions leases deletion requests still awaiting an outcome.
func (r *Repository) ClaimPendingDeletions(ctx context.Context, q database.Querier,
	limit int) ([]models.DeletionRequest, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.querier(q).Query(ctx,
		`SELECT `+deletionColumns+` FROM media_deletion_requests
		 WHERE status IN ($1, $2) ORDER BY created_at LIMIT $3
		 FOR UPDATE SKIP LOCKED`,
		models.DeletionRequestPending, models.DeletionRequestUnknown, limit)
	if err != nil {
		return nil, fmt.Errorf("media: claim pending deletions: %w", err)
	}
	defer rows.Close()

	requests := []models.DeletionRequest{}
	for rows.Next() {
		request, scanErr := scanDeletion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		requests = append(requests, *request)
	}
	return requests, rows.Err()
}

// ClaimServiceNonce records a one-time private-call nonce. Duplicate service
// and nonce pairs are rejected even while the HMAC timestamp is still fresh.
func ClaimServiceNonce(ctx context.Context, q database.Querier, serviceName, nonce,
	fingerprint string, expiresAt time.Time) error {
	if serviceName == "" || nonce == "" || fingerprint == "" {
		return ErrConflict
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO media_service_request_nonces
			(service_name, nonce, request_fingerprint, expires_at, created_at)
		VALUES ($1, $2, $3, $4, now()) ON CONFLICT DO NOTHING`,
		serviceName, nonce, fingerprint, expiresAt)
	if err != nil {
		return fmt.Errorf("media: claim service nonce: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// DeleteExpiredServiceNonces prunes the replay table.
func DeleteExpiredServiceNonces(ctx context.Context, q database.Querier, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	tag, err := q.Exec(ctx, `
		DELETE FROM media_service_request_nonces WHERE ctid IN
			(SELECT ctid FROM media_service_request_nonces WHERE expires_at < now() LIMIT $1)`, limit)
	if err != nil {
		return 0, fmt.Errorf("media: delete expired nonces: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AcquireWorkerLease claims single-runner ownership of a sweep worker.
func AcquireWorkerLease(ctx context.Context, q database.Querier, name, owner string,
	lease time.Duration) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO media_worker_leases (worker_name, lease_owner, lease_expires_at, updated_at)
		VALUES ($1, $2, now() + make_interval(secs => $3), now())
		ON CONFLICT (worker_name) DO UPDATE SET
			lease_owner = EXCLUDED.lease_owner,
			lease_expires_at = EXCLUDED.lease_expires_at,
			updated_at = now()
		WHERE media_worker_leases.lease_expires_at < now() OR media_worker_leases.lease_owner = $2`,
		name, owner, lease.Seconds())
	if err != nil {
		return false, fmt.Errorf("media: acquire worker lease: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ReleaseWorkerLease gives up ownership so a restart does not have to wait out
// the lease.
func ReleaseWorkerLease(ctx context.Context, q database.Querier, name, owner string) error {
	_, err := q.Exec(ctx,
		`DELETE FROM media_worker_leases WHERE worker_name = $1 AND lease_owner = $2`, name, owner)
	return err
}

// isUniqueViolation recognises a PostgreSQL unique-constraint failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "SQLSTATE 23505")
}
