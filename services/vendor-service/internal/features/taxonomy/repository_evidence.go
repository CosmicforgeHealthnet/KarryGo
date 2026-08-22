package taxonomy

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CommandRecord is the durable domain-level idempotency evidence for a
// platform taxonomy command. Records are immutable once written.
type CommandRecord struct {
	ID                 string
	NamespaceID        string
	CommandRef         string
	CommandType        string
	RequestFingerprint string
	ResultType         string
	ResultRef          string
	ResultRevision     int64
	CreatedAt          time.Time
}

type ChangeHistoryEntry struct {
	ID            string
	NamespaceID   string
	VersionID     string
	ActorRef      string
	CommandRef    string
	ChangeType    string
	AggregateType string
	AggregateRef  string
	BeforeState   any
	AfterState    any
	CorrelationID string
	CausationID   string
}

type PublicationHistoryEntry struct {
	ID                string
	NamespaceID       string
	VersionID         string
	PreviousVersionID *string
	EventType         string
	ActorRef          string
	CommandRef        string
	CorrelationID     string
	CausationID       string
	Metadata          any
}

// -----------------------------------------------------------------------------
// Domain command idempotency
// -----------------------------------------------------------------------------

func (r *Repository) GetCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
	commandRef string,
) (CommandRecord, error) {
	if tx == nil || namespaceID == "" || commandRef == "" {
		return CommandRecord{}, ErrInvalidArgument
	}

	var record CommandRecord

	err := tx.QueryRow(ctx, `
        SELECT
            id,
            namespace_id,
            command_ref,
            command_type,
            request_fingerprint,
            result_type,
            result_ref,
            result_revision,
            created_at
        FROM canonical_taxonomy_commands
        WHERE namespace_id = $1
          AND command_ref = $2
    `, namespaceID, commandRef).Scan(
		&record.ID,
		&record.NamespaceID,
		&record.CommandRef,
		&record.CommandType,
		&record.RequestFingerprint,
		&record.ResultType,
		&record.ResultRef,
		&record.ResultRevision,
		&record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommandRecord{}, ErrNotFound
		}
		return CommandRecord{}, err
	}

	return record, nil
}

func (r *Repository) InsertCommand(
	ctx context.Context,
	tx pgx.Tx,
	record CommandRecord,
) error {
	if tx == nil ||
		record.ID == "" ||
		record.NamespaceID == "" ||
		record.CommandRef == "" ||
		record.CommandType == "" ||
		record.RequestFingerprint == "" ||
		record.ResultType == "" ||
		record.ResultRef == "" ||
		record.ResultRevision <= 0 {
		return ErrInvalidArgument
	}

	_, err := tx.Exec(ctx, `
        INSERT INTO canonical_taxonomy_commands (
            id,
            namespace_id,
            command_ref,
            command_type,
            request_fingerprint,
            result_type,
            result_ref,
            result_revision
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
    `,
		record.ID,
		record.NamespaceID,
		record.CommandRef,
		record.CommandType,
		record.RequestFingerprint,
		record.ResultType,
		record.ResultRef,
		record.ResultRevision,
	)
	return err
}

// -----------------------------------------------------------------------------
// Change history
// -----------------------------------------------------------------------------

func (r *Repository) InsertChangeHistory(
	ctx context.Context,
	tx pgx.Tx,
	entry ChangeHistoryEntry,
) error {
	if tx == nil ||
		entry.ID == "" ||
		entry.NamespaceID == "" ||
		entry.VersionID == "" ||
		entry.ActorRef == "" ||
		entry.CommandRef == "" ||
		entry.ChangeType == "" ||
		entry.AggregateType == "" ||
		entry.AggregateRef == "" {
		return ErrInvalidArgument
	}

	beforeState, err := marshalNullableHistoryState(entry.BeforeState)
	if err != nil {
		return err
	}

	afterState, err := marshalNullableHistoryState(entry.AfterState)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO canonical_taxonomy_change_history (
            id,
            namespace_id,
            version_id,
            actor_ref,
            command_ref,
            change_type,
            aggregate_type,
            aggregate_ref,
            before_state,
            after_state,
            correlation_id,
            causation_id
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,
            $9::jsonb,$10::jsonb,$11,$12
        )
    `,
		entry.ID,
		entry.NamespaceID,
		entry.VersionID,
		entry.ActorRef,
		entry.CommandRef,
		entry.ChangeType,
		entry.AggregateType,
		entry.AggregateRef,
		beforeState,
		afterState,
		entry.CorrelationID,
		entry.CausationID,
	)
	return err
}

// -----------------------------------------------------------------------------
// Publication history
// -----------------------------------------------------------------------------

func (r *Repository) InsertPublicationHistory(
	ctx context.Context,
	tx pgx.Tx,
	entry PublicationHistoryEntry,
) error {
	if tx == nil ||
		entry.ID == "" ||
		entry.NamespaceID == "" ||
		entry.VersionID == "" ||
		entry.EventType == "" ||
		entry.ActorRef == "" ||
		entry.CommandRef == "" {
		return ErrInvalidArgument
	}

	metadata, err := marshalHistoryObject(entry.Metadata)
	if err != nil {
		return err
	}

	if metadata == "null" {
		metadata = "{}"
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO canonical_taxonomy_publication_history (
            id,
            namespace_id,
            version_id,
            previous_version_id,
            event_type,
            actor_ref,
            command_ref,
            correlation_id,
            causation_id,
            metadata
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb
        )
    `,
		entry.ID,
		entry.NamespaceID,
		entry.VersionID,
		entry.PreviousVersionID,
		entry.EventType,
		entry.ActorRef,
		entry.CommandRef,
		entry.CorrelationID,
		entry.CausationID,
		metadata,
	)
	return err
}

// -----------------------------------------------------------------------------
// Version lifecycle transitions
// -----------------------------------------------------------------------------

func (r *Repository) PublishDraftVersion(
	ctx context.Context,
	tx pgx.Tx,
	versionID string,
	actorRef string,
	expectedRevision int64,
) (Version, error) {
	if tx == nil ||
		versionID == "" ||
		actorRef == "" ||
		expectedRevision <= 0 {
		return Version{}, ErrInvalidArgument
	}

	version, err := scanVersion(tx.QueryRow(ctx, `
        UPDATE canonical_taxonomy_versions
        SET
            status = 'PUBLISHED',
            revision = revision + 1,
            updated_at = NOW(),
            published_by_actor_ref = $2,
            published_at = NOW()
        WHERE id = $1
          AND status = 'DRAFT'
          AND revision = $3
        RETURNING
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
    `, versionID, actorRef, expectedRevision))
	if !errors.Is(err, ErrNotFound) {
		return version, err
	}

	return Version{}, r.classifyVersionTransitionFailure(
		ctx,
		tx,
		versionID,
		VersionStatusDraft,
		expectedRevision,
	)
}

func (r *Repository) RetirePublishedVersion(
	ctx context.Context,
	tx pgx.Tx,
	versionID string,
	actorRef string,
	expectedRevision int64,
) (Version, error) {
	if tx == nil ||
		versionID == "" ||
		actorRef == "" ||
		expectedRevision <= 0 {
		return Version{}, ErrInvalidArgument
	}

	version, err := scanVersion(tx.QueryRow(ctx, `
        UPDATE canonical_taxonomy_versions
        SET
            status = 'RETIRED',
            revision = revision + 1,
            updated_at = NOW(),
            retired_by_actor_ref = $2,
            retired_at = NOW()
        WHERE id = $1
          AND status = 'PUBLISHED'
          AND revision = $3
        RETURNING
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
    `, versionID, actorRef, expectedRevision))
	if !errors.Is(err, ErrNotFound) {
		return version, err
	}

	return Version{}, r.classifyVersionTransitionFailure(
		ctx,
		tx,
		versionID,
		VersionStatusPublished,
		expectedRevision,
	)
}

func (r *Repository) classifyVersionTransitionFailure(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	requiredStatus VersionStatus,
	expectedRevision int64,
) error {
	var (
		status   string
		revision int64
	)

	err := tx.QueryRow(ctx, `
        SELECT status, revision
        FROM canonical_taxonomy_versions
        WHERE id = $1
    `, id).Scan(&status, &revision)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if VersionStatus(status) != requiredStatus {
		if requiredStatus == VersionStatusDraft {
			return ErrNotDraft
		}
		return ErrConflict
	}

	if revision != expectedRevision {
		return ErrVersionMismatch
	}

	return ErrConflict
}

func marshalNullableHistoryState(value any) (any, error) {
	if value == nil {
		// SQL NULL is intentionally different from JSON literal null.
		// Change-history constraints permit NULL or a JSON object.
		return nil, nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalidArgument
	}

	if !json.Valid(raw) {
		return nil, ErrInvalidArgument
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, ErrInvalidArgument
	}

	return string(raw), nil
}

func marshalHistoryObject(value any) (string, error) {
	if value == nil {
		return "{}", nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalidArgument
	}

	if !json.Valid(raw) {
		return "", ErrInvalidArgument
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return "", ErrInvalidArgument
	}

	return string(raw), nil
}
