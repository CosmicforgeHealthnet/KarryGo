// Package repositories holds the PostgreSQL persistence for fleet-side
// document references.
//
// Every write takes the caller's querier so the document row, its history, its
// audit record and its outbox event commit in one transaction. Every read that
// a caller can reach is tenant-scoped in SQL rather than filtered afterwards:
// a filter applied after the rows are already loaded is one refactor away from
// being forgotten.
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

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Sentinel errors the use cases branch on.
var (
	ErrNotFound        = errors.New("documents: record not found")
	ErrConflict        = errors.New("documents: conflicting record")
	ErrVersionMismatch = errors.New("documents: expected version does not match")
)

const documentColumns = `
	id, organization_id, owner_type, owner_id, branch_id, media_object_id, category,
	document_type, issue_date, expiry_date, lifecycle_state, verification_state,
	safety_projection, media_upload_status, media_deletion_state, media_source_version,
	projection_freshness, last_media_event_id, synced_at, replacement_document_id,
	replaces_document_id, revoked_at, revoked_reason, rejected_reason, deleted_at,
	retention_class, retention_until, legal_hold, legal_hold_reason,
	visibility, archived_at, archived_by_actor_id,
	created_by_actor_type, created_by_actor_id, correlation_id, causation_id,
	idempotency_key, request_fingerprint, version, created_at, updated_at,
	media_checksum_sha256, captured_at, source_timestamp, capture_method,
	device_reference, actor_reference, original_mime_type, original_size_bytes`

// Repository reads and writes fleet document references.
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

func scanDocument(row scannable) (*models.Document, error) {
	var document models.Document
	err := row.Scan(
		&document.ID, &document.OrganizationID, &document.OwnerType, &document.OwnerID,
		&document.BranchID, &document.MediaObjectID, &document.Category, &document.DocumentType,
		&document.IssueDate, &document.ExpiryDate, &document.LifecycleState,
		&document.VerificationState, &document.SafetyProjection, &document.MediaUploadStatus,
		&document.MediaDeletionState, &document.MediaSourceVersion, &document.ProjectionFreshness,
		&document.LastMediaEventID, &document.SyncedAt, &document.ReplacementDocumentID,
		&document.ReplacesDocumentID, &document.RevokedAt, &document.RevokedReason,
		&document.RejectedReason, &document.DeletedAt, &document.RetentionClass,
		&document.RetentionUntil, &document.LegalHold, &document.LegalHoldReason,
		&document.Visibility, &document.ArchivedAt, &document.ArchivedByActorID,
		&document.CreatedByActorType, &document.CreatedByActorID, &document.CorrelationID,
		&document.CausationID, &document.IdempotencyKey, &document.RequestFingerprint,
		&document.Version, &document.CreatedAt, &document.UpdatedAt,
		&document.MediaChecksumSHA256, &document.CapturedAt, &document.SourceTimestamp,
		&document.CaptureMethod, &document.DeviceReference, &document.ActorReference,
		&document.OriginalMimeType, &document.OriginalSizeBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("documents: scan document: %w", err)
	}
	return &document, nil
}

// Create inserts a document reference.
func (r *Repository) Create(ctx context.Context, q database.Querier, document *models.Document) error {
	if document.ID == "" {
		document.ID = ids.New("doc")
	}
	if document.Visibility == "" {
		document.Visibility = models.VisibilityInternal
	}
	row := r.querier(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_documents
			(id, organization_id, owner_type, owner_id, branch_id, media_object_id, category,
			 document_type, issue_date, expiry_date, lifecycle_state, verification_state,
			 safety_projection, media_upload_status, media_deletion_state, media_source_version,
			 projection_freshness, replaces_document_id, retention_class, retention_until,
			 visibility, created_by_actor_type, created_by_actor_id, correlation_id, causation_id,
			 idempotency_key, request_fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		RETURNING `+documentColumns,
		document.ID, document.OrganizationID, document.OwnerType, document.OwnerID,
		document.BranchID, document.MediaObjectID, document.Category, document.DocumentType,
		document.IssueDate, document.ExpiryDate, document.LifecycleState, document.VerificationState,
		document.SafetyProjection, document.MediaUploadStatus, document.MediaDeletionState,
		document.MediaSourceVersion, document.ProjectionFreshness, document.ReplacesDocumentID,
		document.RetentionClass, document.RetentionUntil, document.Visibility, document.CreatedByActorType,
		document.CreatedByActorID, document.CorrelationID, document.CausationID,
		document.IdempotencyKey, document.RequestFingerprint)

	created, err := scanDocument(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	*document = *created
	return nil
}

// GetByID reads one document inside the tenant scope.
//
// A document belonging to another organization answers ErrNotFound rather than
// a permission error, so an identifier cannot be used to discover that another
// tenant holds it.
func (r *Repository) GetByID(ctx context.Context, q database.Querier,
	organizationID string, documentID string) (*models.Document, error) {
	return scanDocument(r.querier(q).QueryRow(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE id = $1 AND organization_id = $2`, documentID, organizationID))
}

// LockByID reads one document FOR UPDATE inside the tenant scope.
func (r *Repository) LockByID(ctx context.Context, q database.Querier,
	organizationID string, documentID string) (*models.Document, error) {
	return scanDocument(r.querier(q).QueryRow(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE id = $1 AND organization_id = $2 FOR UPDATE`, documentID, organizationID))
}

// GetByMediaObject resolves the document a media event is about.
//
// Deliberately not tenant-scoped: an inbound event names an object, not a
// tenant, and the ingestion path checks the event's organization against the
// row it found rather than trusting either alone.
func (r *Repository) GetByMediaObject(ctx context.Context, q database.Querier,
	mediaObjectID string) (*models.Document, error) {
	return scanDocument(r.querier(q).QueryRow(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE media_object_id = $1 FOR UPDATE`, mediaObjectID))
}

// FindByIdempotencyKey resolves a replayed write.
func (r *Repository) FindByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Document, error) {
	return scanDocument(r.querier(q).QueryRow(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// ListFilter narrows a document listing.
type ListFilter struct {
	OwnerType       string
	OwnerID         string
	BranchID        string
	Category        string
	DocumentType    string
	LifecycleState  string
	Visibility      string
	IncludeRetired  bool
	IncludeArchived bool
}

// List returns the organization's documents inside the caller's branch scope.
//
// PermittedBranchIDs is applied in SQL. A caller restricted to two branches
// never loads a third branch's rows, so there is nothing for a later mistake
// to leak.
func (r *Repository) List(ctx context.Context, query tenant.Query,
	filter ListFilter) ([]models.Document, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	conditions := []string{"organization_id = $1"}
	args := []any{query.OrganizationID}
	add := func(clause string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(clause, len(args)))
	}

	if filter.OwnerType != "" {
		add("owner_type = $%d", filter.OwnerType)
	}
	if filter.OwnerID != "" {
		add("owner_id = $%d", filter.OwnerID)
	}
	if filter.BranchID != "" {
		add("branch_id = $%d", filter.BranchID)
	}
	if filter.Category != "" {
		add("category = $%d", filter.Category)
	}
	if filter.DocumentType != "" {
		add("document_type = $%d", filter.DocumentType)
	}
	if filter.LifecycleState != "" {
		add("lifecycle_state = $%d", filter.LifecycleState)
	}
	if filter.Visibility != "" {
		add("visibility = $%d", filter.Visibility)
	}
	if !filter.IncludeRetired {
		conditions = append(conditions, "lifecycle_state <> 'DELETED'")
	}
	if !filter.IncludeArchived {
		conditions = append(conditions, "archived_at IS NULL")
	}
	if len(query.PermittedBranchIDs) > 0 {
		args = append(args, query.PermittedBranchIDs)
		conditions = append(conditions,
			fmt.Sprintf("(branch_id IS NULL OR branch_id = ANY($%d::text[]))", len(args)))
	}

	args = append(args, limit)
	sql := `SELECT ` + documentColumns + ` FROM fleet_partner_documents WHERE ` +
		strings.Join(conditions, " AND ") +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))

	rows, err := r.querier(nil).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("documents: list: %w", err)
	}
	defer rows.Close()

	documents := []models.Document{}
	for rows.Next() {
		document, scanErr := scanDocument(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		documents = append(documents, *document)
	}
	return documents, rows.Err()
}

// Update writes a new document state with an optimistic version check.
func (r *Repository) Update(ctx context.Context, q database.Querier,
	document *models.Document, expectedVersion int64) error {
	row := r.querier(q).QueryRow(ctx, `
		UPDATE fleet_partner_documents SET
			issue_date = $1,
			expiry_date = $2,
			lifecycle_state = $3,
			verification_state = $4,
			safety_projection = $5,
			media_upload_status = $6,
			media_deletion_state = $7,
			media_source_version = $8,
			projection_freshness = $9,
			last_media_event_id = $10,
			synced_at = $11,
			replacement_document_id = $12,
			revoked_at = $13,
			revoked_reason = $14,
			rejected_reason = $15,
			deleted_at = $16,
			retention_class = $17,
			retention_until = $18,
			legal_hold = $19,
			legal_hold_reason = $20,
			visibility = $21,
			archived_at = $22,
			archived_by_actor_id = $23,
			media_checksum_sha256 = $24,
			captured_at = $25,
			source_timestamp = $26,
			capture_method = $27,
			device_reference = $28,
			actor_reference = $29,
			original_mime_type = $30,
			original_size_bytes = $31,
			version = version + 1,
			updated_at = now()
		WHERE id = $32 AND organization_id = $33 AND version = $34
		RETURNING `+documentColumns,
		document.IssueDate, document.ExpiryDate, document.LifecycleState,
		document.VerificationState, document.SafetyProjection, document.MediaUploadStatus,
		document.MediaDeletionState, document.MediaSourceVersion, document.ProjectionFreshness,
		document.LastMediaEventID, document.SyncedAt, document.ReplacementDocumentID,
		document.RevokedAt, document.RevokedReason, document.RejectedReason, document.DeletedAt,
		document.RetentionClass, document.RetentionUntil, document.LegalHold,
		document.LegalHoldReason, document.Visibility, document.ArchivedAt, document.ArchivedByActorID,
		document.MediaChecksumSHA256, document.CapturedAt, document.SourceTimestamp,
		document.CaptureMethod, document.DeviceReference, document.ActorReference,
		document.OriginalMimeType, document.OriginalSizeBytes,
		document.ID, document.OrganizationID, expectedVersion)

	updated, err := scanDocument(row)
	if errors.Is(err, ErrNotFound) {
		return ErrVersionMismatch
	}
	if err != nil {
		return err
	}
	*document = *updated
	return nil
}

// AppendHistory writes one append-only history entry.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier,
	entry *models.HistoryEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New("dhs")
	}
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("documents: marshal history metadata: %w", err)
	}
	if len(metadata) == 0 || string(metadata) == "null" {
		metadata = []byte("{}")
	}
	_, err = r.querier(q).Exec(ctx, `
		INSERT INTO fleet_partner_document_history
			(id, organization_id, document_id, action, from_state, to_state,
			 actor_type, actor_id, metadata, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		entry.ID, entry.OrganizationID, entry.DocumentID, entry.Action, entry.FromState,
		entry.ToState, entry.ActorType, entry.ActorID, metadata, entry.CorrelationID,
		entry.CausationID)
	if err != nil {
		return fmt.Errorf("documents: append history: %w", err)
	}
	return nil
}

// ListHistory returns a document's append-only history, newest first.
func (r *Repository) ListHistory(ctx context.Context, q database.Querier,
	organizationID string, documentID string) ([]models.HistoryEntry, error) {
	rows, err := r.querier(q).Query(ctx, `
		SELECT id, organization_id, document_id, action, from_state, to_state,
		       actor_type, actor_id, metadata, correlation_id, causation_id, occurred_at
		FROM fleet_partner_document_history
		WHERE organization_id = $1 AND document_id = $2
		ORDER BY occurred_at DESC, id DESC`, organizationID, documentID)
	if err != nil {
		return nil, fmt.Errorf("documents: list history: %w", err)
	}
	defer rows.Close()

	entries := []models.HistoryEntry{}
	for rows.Next() {
		var entry models.HistoryEntry
		var metadata []byte
		if scanErr := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.DocumentID,
			&entry.Action, &entry.FromState, &entry.ToState, &entry.ActorType, &entry.ActorID,
			&metadata, &entry.CorrelationID, &entry.CausationID, &entry.OccurredAt); scanErr != nil {
			return nil, fmt.Errorf("documents: scan history: %w", scanErr)
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &entry.Metadata)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// AppendVerification writes one immutable verification decision.
func (r *Repository) AppendVerification(ctx context.Context, q database.Querier,
	decision *models.VerificationDecision) error {
	if decision.ID == "" {
		decision.ID = ids.New("dvd")
	}
	_, err := r.querier(q).Exec(ctx, `
		INSERT INTO fleet_partner_document_verifications
			(id, organization_id, document_id, decision, reason_code, reason_note,
			 media_source_version, media_checksum_sha256, decided_by_actor_type, decided_by_actor_id,
			 document_version, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		decision.ID, decision.OrganizationID, decision.DocumentID, decision.Decision,
		decision.ReasonCode, decision.ReasonNote, decision.MediaSourceVersion,
		decision.MediaChecksumSHA256, decision.DecidedByActorType, decision.DecidedByActorID,
		decision.DocumentVersion, decision.CorrelationID, decision.CausationID)
	if err != nil {
		return fmt.Errorf("documents: append verification: %w", err)
	}
	return nil
}

// ListVerifications returns the immutable decision history.
func (r *Repository) ListVerifications(ctx context.Context, q database.Querier,
	organizationID string, documentID string) ([]models.VerificationDecision, error) {
	rows, err := r.querier(q).Query(ctx, `
		SELECT id, organization_id, document_id, decision, reason_code, reason_note,
		       media_source_version, media_checksum_sha256, decided_by_actor_type, decided_by_actor_id,
		       document_version, correlation_id, causation_id, decided_at
		FROM fleet_partner_document_verifications
		WHERE organization_id = $1 AND document_id = $2
		ORDER BY decided_at DESC, id DESC`, organizationID, documentID)
	if err != nil {
		return nil, fmt.Errorf("documents: list verifications: %w", err)
	}
	defer rows.Close()

	decisions := []models.VerificationDecision{}
	for rows.Next() {
		var decision models.VerificationDecision
		if scanErr := rows.Scan(&decision.ID, &decision.OrganizationID, &decision.DocumentID,
			&decision.Decision, &decision.ReasonCode, &decision.ReasonNote,
			&decision.MediaSourceVersion, &decision.MediaChecksumSHA256,
			&decision.DecidedByActorType, &decision.DecidedByActorID,
			&decision.DocumentVersion, &decision.CorrelationID, &decision.CausationID,
			&decision.DecidedAt); scanErr != nil {
			return nil, fmt.Errorf("documents: scan verification: %w", scanErr)
		}
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
}

// SourceVersion is the forward-only version record for one media object.
type SourceVersion struct {
	MediaObjectID string
	Version       int64
	LastEventID   string
	PayloadHash   string
}

// LockSourceVersion reads the recorded source version FOR UPDATE.
func (r *Repository) LockSourceVersion(ctx context.Context, q database.Querier,
	mediaObjectID string) (SourceVersion, error) {
	var record SourceVersion
	record.MediaObjectID = mediaObjectID
	err := r.querier(q).QueryRow(ctx, `
		SELECT source_version, last_event_id, payload_hash
		FROM fleet_partner_document_media_versions
		WHERE media_object_id = $1 FOR UPDATE`, mediaObjectID).
		Scan(&record.Version, &record.LastEventID, &record.PayloadHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceVersion{MediaObjectID: mediaObjectID}, ErrNotFound
	}
	if err != nil {
		return SourceVersion{}, fmt.Errorf("documents: lock source version: %w", err)
	}
	return record, nil
}

// RecordSourceVersion advances the forward-only version marker.
func (r *Repository) RecordSourceVersion(ctx context.Context, q database.Querier,
	record SourceVersion) error {
	_, err := r.querier(q).Exec(ctx, `
		INSERT INTO fleet_partner_document_media_versions
			(media_object_id, source_version, last_event_id, payload_hash, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (media_object_id) DO UPDATE SET
			source_version = EXCLUDED.source_version,
			last_event_id = EXCLUDED.last_event_id,
			payload_hash = EXCLUDED.payload_hash,
			updated_at = now()`,
		record.MediaObjectID, record.Version, record.LastEventID, record.PayloadHash)
	if err != nil {
		return fmt.Errorf("documents: record source version: %w", err)
	}
	return nil
}

const caseColumns = `
	id, organization_id, document_id, media_object_id, reason, status, attempts,
	last_error_code, next_attempt_at, lease_owner, lease_expires_at, resolved_at,
	correlation_id, created_at, updated_at`

func scanCase(row scannable) (*models.ReconciliationCase, error) {
	var record models.ReconciliationCase
	err := row.Scan(&record.ID, &record.OrganizationID, &record.DocumentID, &record.MediaObjectID,
		&record.Reason, &record.Status, &record.Attempts, &record.LastErrorCode,
		&record.NextAttemptAt, &record.LeaseOwner, &record.LeaseExpiresAt, &record.ResolvedAt,
		&record.CorrelationID, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("documents: scan reconciliation case: %w", err)
	}
	return &record, nil
}

// OpenReconciliationCase records a disagreement awaiting a read-back.
//
// A case already open for the document is left alone rather than duplicated:
// the partial unique index enforces it, and doubling the retry rate against an
// authority that is already struggling is the opposite of helpful.
func (r *Repository) OpenReconciliationCase(ctx context.Context, q database.Querier,
	record *models.ReconciliationCase) error {
	if record.ID == "" {
		record.ID = ids.New("drc")
	}
	tag, err := r.querier(q).Exec(ctx, `
		INSERT INTO fleet_partner_document_reconciliation_cases
			(id, organization_id, document_id, media_object_id, reason, status,
			 next_attempt_at, correlation_id)
		SELECT $1, $2, $3, $4, $5, $6, now(), $7
		WHERE NOT EXISTS (
			SELECT 1 FROM fleet_partner_document_reconciliation_cases
			WHERE document_id = $3 AND status IN ('OPEN', 'IN_PROGRESS')
		)`,
		record.ID, record.OrganizationID, record.DocumentID, record.MediaObjectID,
		record.Reason, models.CaseOpen, record.CorrelationID)
	if err != nil {
		return fmt.Errorf("documents: open reconciliation case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already open. Not an error: the condition is already being tracked.
		record.ID = ""
	}
	return nil
}

// OpenCaseFor reports whether the document has an unresolved case.
func (r *Repository) OpenCaseFor(ctx context.Context, q database.Querier,
	documentID string) (bool, error) {
	var exists bool
	err := r.querier(q).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_document_reconciliation_cases
			WHERE document_id = $1 AND status IN ('OPEN', 'IN_PROGRESS')
		)`, documentID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("documents: check open case: %w", err)
	}
	return exists, nil
}

// ClaimReconciliationCases leases due cases for one worker.
//
// The lease is what lets a crashed worker's cases be picked up by another
// rather than being stuck forever, and SKIP LOCKED is what stops two workers
// claiming the same one.
func (r *Repository) ClaimReconciliationCases(ctx context.Context, q database.Querier,
	owner string, lease time.Duration, limit int) ([]models.ReconciliationCase, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.querier(q).Query(ctx, `
		UPDATE fleet_partner_document_reconciliation_cases SET
			status = $1,
			lease_owner = $2,
			lease_expires_at = now() + make_interval(secs => $3),
			attempts = attempts + 1,
			updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_document_reconciliation_cases
			WHERE status IN ($4, $1)
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY next_attempt_at
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+caseColumns,
		models.CaseInProgress, owner, lease.Seconds(), models.CaseOpen, limit)
	if err != nil {
		return nil, fmt.Errorf("documents: claim reconciliation cases: %w", err)
	}
	defer rows.Close()

	cases := []models.ReconciliationCase{}
	for rows.Next() {
		record, scanErr := scanCase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		cases = append(cases, *record)
	}
	return cases, rows.Err()
}

// ResolveReconciliationCase closes a case that was successfully repaired.
func (r *Repository) ResolveReconciliationCase(ctx context.Context, q database.Querier,
	id string) error {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_reconciliation_cases SET
			status = $1, resolved_at = now(), lease_owner = NULL,
			lease_expires_at = NULL, last_error_code = '', updated_at = now()
		WHERE id = $2`, models.CaseResolved, id)
	if err != nil {
		return fmt.Errorf("documents: resolve reconciliation case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FailReconciliationCase schedules a retry with backoff, dead-lettering after
// maxAttempts so a permanently broken case stops consuming the batch.
func (r *Repository) FailReconciliationCase(ctx context.Context, q database.Querier,
	id string, errorCode string, backoff time.Duration, maxAttempts int) error {
	if len(errorCode) > 100 {
		errorCode = errorCode[:100]
	}
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_reconciliation_cases SET
			status = CASE WHEN attempts >= $1 THEN $2 ELSE $3 END,
			last_error_code = $4,
			next_attempt_at = now() + make_interval(secs => $5),
			lease_owner = NULL,
			lease_expires_at = NULL,
			updated_at = now()
		WHERE id = $6`,
		maxAttempts, models.CaseDeadLettered, models.CaseOpen, errorCode, backoff.Seconds(), id)
	if err != nil {
		return fmt.Errorf("documents: fail reconciliation case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplayReconciliationCase returns a dead-lettered case to the queue.
func (r *Repository) ReplayReconciliationCase(ctx context.Context, q database.Querier,
	organizationID string, id string) error {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_reconciliation_cases SET
			status = $1, attempts = 0, next_attempt_at = now(), last_error_code = '',
			lease_owner = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE id = $2 AND organization_id = $3 AND status = $4`,
		models.CaseOpen, id, organizationID, models.CaseDeadLettered)
	if err != nil {
		return fmt.Errorf("documents: replay reconciliation case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecoverExpiredCaseLeases returns abandoned cases to the queue.
func (r *Repository) RecoverExpiredCaseLeases(ctx context.Context, q database.Querier) (int64, error) {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_reconciliation_cases SET
			status = $1, lease_owner = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now() AND status = $2`,
		models.CaseOpen, models.CaseInProgress)
	if err != nil {
		return 0, fmt.Errorf("documents: recover case leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListCases returns reconciliation cases for one organization.
func (r *Repository) ListCases(ctx context.Context, q database.Querier,
	organizationID string, documentID string) ([]models.ReconciliationCase, error) {
	rows, err := r.querier(q).Query(ctx,
		`SELECT `+caseColumns+` FROM fleet_partner_document_reconciliation_cases
		 WHERE organization_id = $1 AND ($2 = '' OR document_id = $2)
		 ORDER BY created_at DESC LIMIT 100`, organizationID, documentID)
	if err != nil {
		return nil, fmt.Errorf("documents: list reconciliation cases: %w", err)
	}
	defer rows.Close()

	cases := []models.ReconciliationCase{}
	for rows.Next() {
		record, scanErr := scanCase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		cases = append(cases, *record)
	}
	return cases, rows.Err()
}

const deletionColumns = `
	id, organization_id, document_id, media_object_id, reason, status, block_reason,
	attempts, last_error_code, scheduled_for, next_attempt_at, lease_owner,
	lease_expires_at, requested_by, idempotency_key, request_fingerprint,
	correlation_id, causation_id, created_at, updated_at, completed_at`

func scanDeletion(row scannable) (*models.DeletionRequest, error) {
	var record models.DeletionRequest
	err := row.Scan(&record.ID, &record.OrganizationID, &record.DocumentID, &record.MediaObjectID,
		&record.Reason, &record.Status, &record.BlockReason, &record.Attempts,
		&record.LastErrorCode, &record.ScheduledFor, &record.NextAttemptAt, &record.LeaseOwner,
		&record.LeaseExpiresAt, &record.RequestedBy, &record.IdempotencyKey,
		&record.RequestFingerprint, &record.CorrelationID, &record.CausationID,
		&record.CreatedAt, &record.UpdatedAt, &record.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("documents: scan deletion request: %w", err)
	}
	return &record, nil
}

// CreateDeletionRequest schedules a deletion.
func (r *Repository) CreateDeletionRequest(ctx context.Context, q database.Querier,
	record *models.DeletionRequest) error {
	if record.ID == "" {
		record.ID = ids.New("ddr")
	}
	row := r.querier(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_document_deletion_requests
			(id, organization_id, document_id, media_object_id, reason, status, block_reason,
			 scheduled_for, next_attempt_at, requested_by, idempotency_key,
			 request_fingerprint, correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8,$9,$10,$11,$12,$13)
		RETURNING `+deletionColumns,
		record.ID, record.OrganizationID, record.DocumentID, record.MediaObjectID,
		record.Reason, record.Status, record.BlockReason, record.ScheduledFor,
		record.RequestedBy, record.IdempotencyKey, record.RequestFingerprint,
		record.CorrelationID, record.CausationID)

	created, err := scanDeletion(row)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	*record = *created
	return nil
}

// DeletionRequestByIdempotencyKey resolves a replayed deletion request.
func (r *Repository) DeletionRequestByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.DeletionRequest, error) {
	return scanDeletion(r.querier(q).QueryRow(ctx,
		`SELECT `+deletionColumns+` FROM fleet_partner_document_deletion_requests
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// LatestDeletionRequest returns the newest request for one document.
func (r *Repository) LatestDeletionRequest(ctx context.Context, q database.Querier,
	organizationID string, documentID string) (*models.DeletionRequest, error) {
	return scanDeletion(r.querier(q).QueryRow(ctx,
		`SELECT `+deletionColumns+` FROM fleet_partner_document_deletion_requests
		 WHERE organization_id = $1 AND document_id = $2
		 ORDER BY created_at DESC LIMIT 1`, organizationID, documentID))
}

// ClaimDeletionRequests leases due deletion work for one worker.
func (r *Repository) ClaimDeletionRequests(ctx context.Context, q database.Querier,
	owner string, lease time.Duration, limit int) ([]models.DeletionRequest, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.querier(q).Query(ctx, `
		UPDATE fleet_partner_document_deletion_requests SET
			status = $1,
			lease_owner = $2,
			lease_expires_at = now() + make_interval(secs => $3),
			attempts = attempts + 1,
			updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_document_deletion_requests
			WHERE status IN ($4, $5, $1)
			  AND scheduled_for <= now()
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY next_attempt_at
			LIMIT $6
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+deletionColumns,
		models.DeletionInProgress, owner, lease.Seconds(),
		models.DeletionScheduled, models.DeletionUnknown, limit)
	if err != nil {
		return nil, fmt.Errorf("documents: claim deletion requests: %w", err)
	}
	defer rows.Close()

	requests := []models.DeletionRequest{}
	for rows.Next() {
		record, scanErr := scanDeletion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		requests = append(requests, *record)
	}
	return requests, rows.Err()
}

// SetDeletionStatus transitions a deletion request.
func (r *Repository) SetDeletionStatus(ctx context.Context, q database.Querier,
	id string, status string, blockReason string, errorCode string, backoff time.Duration) error {
	if len(errorCode) > 100 {
		errorCode = errorCode[:100]
	}
	completed := status == models.DeletionCompleted
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_deletion_requests SET
			status = $1,
			block_reason = $2,
			last_error_code = $3,
			next_attempt_at = now() + make_interval(secs => $4),
			lease_owner = NULL,
			lease_expires_at = NULL,
			completed_at = CASE WHEN $5 THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $6`, status, blockReason, errorCode, backoff.Seconds(), completed, id)
	if err != nil {
		return fmt.Errorf("documents: set deletion status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplayDeletionRequest returns a dead-lettered deletion to the queue.
func (r *Repository) ReplayDeletionRequest(ctx context.Context, q database.Querier,
	organizationID string, id string) error {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_deletion_requests SET
			status = $1, attempts = 0, next_attempt_at = now(), last_error_code = '',
			lease_owner = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE id = $2 AND organization_id = $3 AND status IN ($4, $5)`,
		models.DeletionScheduled, id, organizationID,
		models.DeletionDeadLettered, models.DeletionFailed)
	if err != nil {
		return fmt.Errorf("documents: replay deletion request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecoverExpiredDeletionLeases returns abandoned deletions to the queue.
func (r *Repository) RecoverExpiredDeletionLeases(ctx context.Context, q database.Querier) (int64, error) {
	tag, err := r.querier(q).Exec(ctx, `
		UPDATE fleet_partner_document_deletion_requests SET
			status = $1, lease_owner = NULL, lease_expires_at = NULL, updated_at = now()
		WHERE lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now() AND status = $2`,
		models.DeletionScheduled, models.DeletionInProgress)
	if err != nil {
		return 0, fmt.Errorf("documents: recover deletion leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ClaimExpiredDocuments returns documents whose stated expiry has passed and
// that have not yet been marked EXPIRED.
func (r *Repository) ClaimExpiredDocuments(ctx context.Context, q database.Querier,
	limit int) ([]models.Document, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.querier(q).Query(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE expiry_date IS NOT NULL
		   AND expiry_date <= (now() AT TIME ZONE 'UTC')::date
		   AND lifecycle_state NOT IN ('EXPIRED', 'REVOKED', 'REPLACED', 'DELETED')
		 ORDER BY expiry_date
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("documents: claim expired documents: %w", err)
	}
	defer rows.Close()

	documents := []models.Document{}
	for rows.Next() {
		document, scanErr := scanDocument(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		documents = append(documents, *document)
	}
	return documents, rows.Err()
}

// ClaimStaleProjections returns documents whose media projection has not been
// confirmed within the threshold.
func (r *Repository) ClaimStaleProjections(ctx context.Context, q database.Querier,
	staleAfter time.Duration, limit int) ([]models.Document, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.querier(q).Query(ctx,
		`SELECT `+documentColumns+` FROM fleet_partner_documents
		 WHERE lifecycle_state NOT IN ('DELETED', 'REVOKED', 'REPLACED')
		   AND projection_freshness = 'CURRENT'
		   AND (synced_at IS NULL OR synced_at < now() - make_interval(secs => $1))
		 ORDER BY synced_at NULLS FIRST
		 LIMIT $2`, staleAfter.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("documents: claim stale projections: %w", err)
	}
	defer rows.Close()

	documents := []models.Document{}
	for rows.Next() {
		document, scanErr := scanDocument(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		documents = append(documents, *document)
	}
	return documents, rows.Err()
}

// OwnerBelongsToOrganization proves the organization owns what a document is
// being attached to.
//
// This is the tenant boundary for writes. Without it, an operator could point
// their organization's document at another tenant's provider link, and from
// then on every listing that joins the two would be wrong.
func (r *Repository) OwnerBelongsToOrganization(ctx context.Context, q database.Querier,
	organizationID string, ownerType string, ownerID string) (bool, error) {
	var query string
	switch ownerType {
	case models.OwnerCompany:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_organizations WHERE id = $2 AND id = $1)`
	case models.OwnerBranch:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_branches WHERE id = $2 AND organization_id = $1)`
	case models.OwnerProviderLink:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE id = $2 AND organization_id = $1)`
	case models.OwnerVehicleLink:
		// Fleet holds no vehicle table; a vehicle-linked document is anchored
		// to the membership that controls the vehicle, which fleet does own.
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE id = $2 AND organization_id = $1)`
	case models.OwnerIncident, models.OwnerDispute:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_safety_references WHERE id = $2 AND organization_id = $1)`
	case models.OwnerApproval:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_approval_requests WHERE id = $2 AND organization_id = $1)`
	case models.OwnerOrder:
		query = `SELECT EXISTS (SELECT 1 FROM fleet_partner_private_orders WHERE id = $2 AND organization_id = $1)`
	default:
		return false, nil
	}

	var exists bool
	if err := r.querier(q).QueryRow(ctx, query, organizationID, ownerID).Scan(&exists); err != nil {
		return false, fmt.Errorf("documents: owner ownership check: %w", err)
	}
	return exists, nil
}

// CustomerForOrder resolves the customer that owns a private order, for
// customer-facing document authorization: a customer may only reach a
// document whose owner_type is ORDER and whose order's customer_id is theirs.
func (r *Repository) CustomerForOrder(ctx context.Context, q database.Querier,
	organizationID string, orderID string) (string, error) {
	var customerID string
	err := r.querier(q).QueryRow(ctx,
		`SELECT customer_id FROM fleet_partner_private_orders WHERE id = $1 AND organization_id = $2`,
		orderID, organizationID).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("documents: customer for order: %w", err)
	}
	return customerID, nil
}

// BranchBelongsToOrganization proves a named branch is this tenant's.
func (r *Repository) BranchBelongsToOrganization(ctx context.Context, q database.Querier,
	organizationID string, branchID string) (bool, error) {
	var exists bool
	err := r.querier(q).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM fleet_partner_branches WHERE id = $1 AND organization_id = $2)`,
		branchID, organizationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("documents: branch ownership check: %w", err)
	}
	return exists, nil
}

// RequiredDocumentState is the standing of one required document type for the
// eligibility gate.
type RequiredDocumentState struct {
	OwnerType    string
	OwnerID      string
	DocumentType string
	Present      bool
	Satisfied    bool
	// Reason is the controlled eligibility reason code when Satisfied is false.
	Reason string
}

// RequiredDocumentStates reports whether each required document type is
// satisfied for one owner.
//
// The newest non-replaced document per type decides. Every rule that could
// block — unverified, unsafe, expired, revoked, replaced, deleted, stale
// projection, open reconciliation case — is evaluated here rather than in the
// caller, so a second gate cannot be written that forgets one of them.
func (r *Repository) RequiredDocumentStates(ctx context.Context, q database.Querier,
	organizationID string, ownerType string, ownerID string,
	requiredTypes []string, now time.Time) ([]RequiredDocumentState, error) {
	if len(requiredTypes) == 0 {
		return nil, nil
	}

	rows, err := r.querier(q).Query(ctx, `
		SELECT DISTINCT ON (d.document_type)
			d.document_type, d.lifecycle_state, d.verification_state, d.safety_projection,
			d.media_deletion_state, d.projection_freshness, d.expiry_date,
			d.revoked_at, d.deleted_at,
			EXISTS (
				SELECT 1 FROM fleet_partner_document_reconciliation_cases c
				WHERE c.document_id = d.id AND c.status IN ('OPEN', 'IN_PROGRESS')
			) AS reconciling
		FROM fleet_partner_documents d
		WHERE d.organization_id = $1
		  AND d.owner_type = $2
		  AND d.owner_id = $3
		  AND d.document_type = ANY($4::text[])
		ORDER BY d.document_type,
			-- Prefer a live document over a superseded one, then newest first.
			(d.lifecycle_state IN ('REPLACED', 'DELETED', 'REVOKED')) ASC,
			d.created_at DESC`,
		organizationID, ownerType, ownerID, requiredTypes)
	if err != nil {
		return nil, fmt.Errorf("documents: required document states: %w", err)
	}
	defer rows.Close()

	held := map[string]RequiredDocumentState{}
	for rows.Next() {
		var documentType string
		var document models.Document
		var reconciling bool
		if scanErr := rows.Scan(&documentType, &document.LifecycleState,
			&document.VerificationState, &document.SafetyProjection,
			&document.MediaDeletionState, &document.ProjectionFreshness, &document.ExpiryDate,
			&document.RevokedAt, &document.DeletedAt, &reconciling); scanErr != nil {
			return nil, fmt.Errorf("documents: scan required document state: %w", scanErr)
		}

		state := RequiredDocumentState{
			OwnerType: ownerType, OwnerID: ownerID, DocumentType: documentType, Present: true,
		}
		switch {
		case reconciling:
			// An unresolved reconciliation case outranks everything else: the
			// projection is known to disagree with the authority.
			state.Reason = models.ReasonDocumentReconciling
		default:
			satisfied, reason := document.SatisfiesRequirement(now, true)
			state.Satisfied = satisfied
			state.Reason = reason
		}
		held[documentType] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	states := make([]RequiredDocumentState, 0, len(requiredTypes))
	for _, documentType := range requiredTypes {
		state, ok := held[documentType]
		if !ok {
			state = RequiredDocumentState{
				OwnerType: ownerType, OwnerID: ownerID, DocumentType: documentType,
				Reason: models.ReasonDocumentMissing,
			}
		}
		states = append(states, state)
	}
	return states, nil
}

// RecordAccess writes one append-only access-audit entry.
//
// Called on every provider-facing and customer-facing list/read/download
// attempt, allowed or denied — the audit trail this service owes an auditor
// is "who reached for what, and were they let through," not just the allowed
// half.
func (r *Repository) RecordAccess(ctx context.Context, q database.Querier,
	entry *models.AccessAuditEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New("daa")
	}
	_, err := r.querier(q).Exec(ctx, `
		INSERT INTO fleet_partner_document_access_audit
			(id, organization_id, document_id, actor_type, actor_reference,
			 subject_type, subject_reference, visibility, action, result,
			 denial_reason_code, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		entry.ID, entry.OrganizationID, entry.DocumentID, entry.ActorType, entry.ActorReference,
		entry.SubjectType, entry.SubjectReference, entry.Visibility, entry.Action, entry.Result,
		entry.DenialReasonCode, entry.CorrelationID)
	if err != nil {
		return fmt.Errorf("documents: record access: %w", err)
	}
	return nil
}

// ListAccessAudit returns access-audit entries for one document, newest
// first, for operator/compliance review.
func (r *Repository) ListAccessAudit(ctx context.Context, q database.Querier,
	organizationID string, documentID string, limit int) ([]models.AccessAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.querier(q).Query(ctx, `
		SELECT id, organization_id, document_id, actor_type, actor_reference,
		       subject_type, subject_reference, visibility, action, result,
		       denial_reason_code, correlation_id, occurred_at
		FROM fleet_partner_document_access_audit
		WHERE organization_id = $1 AND document_id = $2
		ORDER BY occurred_at DESC, id DESC LIMIT $3`, organizationID, documentID, limit)
	if err != nil {
		return nil, fmt.Errorf("documents: list access audit: %w", err)
	}
	defer rows.Close()

	entries := []models.AccessAuditEntry{}
	for rows.Next() {
		var entry models.AccessAuditEntry
		if scanErr := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.DocumentID,
			&entry.ActorType, &entry.ActorReference, &entry.SubjectType, &entry.SubjectReference,
			&entry.Visibility, &entry.Action, &entry.Result, &entry.DenialReasonCode,
			&entry.CorrelationID, &entry.OccurredAt); scanErr != nil {
			return nil, fmt.Errorf("documents: scan access audit: %w", scanErr)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
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
