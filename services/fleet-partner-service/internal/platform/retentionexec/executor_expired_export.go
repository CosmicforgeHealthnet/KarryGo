package retentionexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ExportMediaDeleter is the narrow port this executor needs from Media
// File Service -- deliberately not the full platformclients.MediaAuthority
// interface (this executor never uploads, downloads or reads content, only
// requests and confirms deletion of a file it never touches the bytes of).
// A real adapter wraps platformclients.MediaFileClient, matching how
// internal/features/documents' own deletion workflow (usecases/
// deletion.go) already talks to the same service.
type ExportMediaDeleter interface {
	RequestDeletion(ctx context.Context, meta platformclients.CallMeta, mediaObjectID string, reason string) (platformclients.MediaDeletionOutcome, error)
	ReadDeletionState(ctx context.Context, meta platformclients.CallMeta, mediaObjectID string) (platformclients.MediaDeletionState, error)
	Configured() bool
}

// ExpiredExportExecutor implements retention for the ClassTemporary class,
// scoped to exactly one target: fleet_partner_export_jobs rows that have
// already reached EXPIRED (exports/usecases' own RunExpiryPass sets this,
// unchanged by this package -- see exports_test.go's
// TestExports_ExpiryBlocksDownloadOfStaleReadyJobs) and still reference an
// undeleted media object.
//
// Deletion is requested THROUGH Media Service, never performed directly --
// media-file-service is the sole authority over its own bytes; this
// executor only ever asks and records the answer, mirroring
// internal/features/documents/usecases/deletion.go's executeDeletion
// exactly: COMPLETED settles the row, BLOCKED/FAILED/an outright error
// leaves it untouched for a later pass, and an UNKNOWN outcome (a timeout,
// not a real answer) triggers one synchronous read-back before giving up
// for this pass -- never assumed deleted on the strength of a request
// nobody confirmed.
//
// The export row itself, and fleet_partner_export_attempts (immutable,
// append-only-triggered), are never touched beyond the two tracking
// columns migration 000073 added -- "preserve export audit history" is
// therefore true by construction, not by a check this executor performs.
type ExpiredExportExecutor struct {
	media ExportMediaDeleter
}

// NewExpiredExportExecutor builds the executor. A nil media port makes
// FindCandidates return nothing (fail closed), matching the established
// convention for every other unconfigured external port in this codebase
// (documents' Media, financeops' Wallet) -- never silently deleting
// nothing while reporting success, and never guessing at an answer no
// configured service can give.
func NewExpiredExportExecutor(media ExportMediaDeleter) *ExpiredExportExecutor {
	return &ExpiredExportExecutor{media: media}
}

// Class implements ClassExecutor.
func (e *ExpiredExportExecutor) Class() string { return retentionmodels.ClassTemporary }

const expiredExportRecordType = "export_job"

// FindCandidates returns EXPIRED exports of organizationID whose
// underlying media object has not yet been confirmed deleted.
func (e *ExpiredExportExecutor) FindCandidates(ctx context.Context, q database.Querier, organizationID string, limit int) ([]Candidate, error) {
	if e.media == nil || !e.media.Configured() {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT id
		FROM fleet_partner_export_jobs
		WHERE organization_id = $1
		  AND status = 'EXPIRED'
		  AND media_file_id <> ''
		  AND media_deletion_status <> 'CONFIRMED'
		ORDER BY id
		LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("retentionexec: expired export find candidates: %w", err)
	}
	defer rows.Close()
	var candidates []Candidate
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("retentionexec: expired export scan candidate: %w", err)
		}
		candidates = append(candidates, Candidate{RecordType: expiredExportRecordType, RecordID: id})
	}
	return candidates, rows.Err()
}

// ScopeForLegalHold checks an ORGANIZATION-scoped hold -- an export is a
// generated artifact of that organization's own data, so the organization
// is the correct, conservative scope (there is no per-export legal hold
// concept anywhere in this schema, and inventing one for a generated
// report file would be pure speculation).
func (e *ExpiredExportExecutor) ScopeForLegalHold(candidate Candidate) (scopeType, scopeReference string) {
	return "ORGANIZATION", ""
}

// EligibleSince returns the export's own expiry time -- the retention
// clock for "how long may an expired export's file linger before we ask
// Media to remove it" runs from expiry, not from when the export was
// originally created (a long-lived READY export that only just expired
// must not be treated as though it had already been eligible for months).
func (e *ExpiredExportExecutor) EligibleSince(ctx context.Context, q database.Querier, candidate Candidate) (time.Time, error) {
	var expiresAt *time.Time
	var status string
	if err := q.QueryRow(ctx, `SELECT expires_at, status FROM fleet_partner_export_jobs WHERE id = $1`,
		candidate.RecordID).Scan(&expiresAt, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, fmt.Errorf("retentionexec: export %s no longer exists", candidate.RecordID)
		}
		return time.Time{}, fmt.Errorf("retentionexec: expired export resolve expiry: %w", err)
	}
	if status != "EXPIRED" || expiresAt == nil {
		// FindCandidates already required status='EXPIRED' with the same
		// read-committed snapshot risk every other executor in this package
		// accepts (OrgContactExecutor's EligibleSince has the identical
		// shape) -- refuses to guess rather than treating a nil/changed
		// value as "eligible now".
		return time.Time{}, fmt.Errorf("retentionexec: export %s is not in a stable EXPIRED state", candidate.RecordID)
	}
	return *expiresAt, nil
}

// Preview reports what EXECUTE would do, without writing anything and
// without calling Media (a preview must never itself request an
// irreversible action against another service).
func (e *ExpiredExportExecutor) Preview(ctx context.Context, q database.Querier, candidate Candidate) (string, string, error) {
	var mediaFileID string
	if err := q.QueryRow(ctx, `SELECT media_file_id FROM fleet_partner_export_jobs WHERE id = $1`,
		candidate.RecordID).Scan(&mediaFileID); err != nil {
		return "", "", fmt.Errorf("retentionexec: expired export preview: %w", err)
	}
	if mediaFileID == "" {
		return "", "", fmt.Errorf("retentionexec: export %s has no media file to delete", candidate.RecordID)
	}
	return OutcomeArchived, "would request deletion of the underlying export file through Media Service", nil
}

// Execute requests deletion through Media Service and records only what
// Media actually confirmed -- see the type doc comment for the full
// outcome mapping.
func (e *ExpiredExportExecutor) Execute(ctx context.Context, tx pgx.Tx, candidate Candidate) (string, string, error) {
	if e.media == nil {
		return "", "", errors.New("retentionexec: expired export: media deletion port is not configured")
	}
	var organizationID, mediaFileID string
	if err := tx.QueryRow(ctx, `SELECT organization_id, media_file_id FROM fleet_partner_export_jobs WHERE id = $1 FOR UPDATE`,
		candidate.RecordID).Scan(&organizationID, &mediaFileID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("retentionexec: export %s no longer exists", candidate.RecordID)
		}
		return "", "", fmt.Errorf("retentionexec: expired export lock: %w", err)
	}
	if mediaFileID == "" {
		return "", "", fmt.Errorf("retentionexec: export %s has no media file to delete", candidate.RecordID)
	}

	meta := platformclients.CallMeta{OrganizationID: organizationID, IdempotencyKey: "retention-export-deletion:" + candidate.RecordID}
	outcome, err := e.media.RequestDeletion(ctx, meta, mediaFileID, "retention: expired export past effective retention window")
	if err != nil {
		var clientErr *platformclients.Error
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeNotFound {
			// Media does not know the object -- it cannot be reachable by
			// anybody, so it is deleted as far as anybody can act on it.
			// Same reasoning documents' executeDeletion already applies.
			return e.confirmDeleted(ctx, tx, candidate.RecordID, "")
		}
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeUnknown {
			return e.readBackOnce(ctx, tx, candidate.RecordID, organizationID, mediaFileID)
		}
		return "", "", fmt.Errorf("retentionexec: expired export request deletion: %w", err)
	}

	switch outcome.Status {
	case "COMPLETED":
		return e.confirmDeleted(ctx, tx, candidate.RecordID, outcome.DeletionRequestID)
	case "BLOCKED":
		return "", "", fmt.Errorf("retentionexec: media blocked deletion of export %s: %s", candidate.RecordID, outcome.BlockReason)
	default:
		// PENDING, FAILED, or anything unrecognised: read back once rather
		// than guessing what a non-terminal answer meant.
		return e.readBackOnce(ctx, tx, candidate.RecordID, organizationID, mediaFileID)
	}
}

// readBackOnce asks Media directly for the object's current deletion
// state -- the one authoritative read-back Section 28 requires before an
// UNKNOWN or non-terminal RequestDeletion answer may ever be treated as a
// completed deletion. If the read-back itself does not confirm deletion,
// this candidate is left un-recorded as ARCHIVED (an error is returned,
// mapped to OutcomeFailed by Runner.decide) so a LATER run's
// FindCandidates reconsiders it -- never assumed, only confirmed.
func (e *ExpiredExportExecutor) readBackOnce(ctx context.Context, tx pgx.Tx, exportID, organizationID, mediaFileID string) (string, string, error) {
	meta := platformclients.CallMeta{OrganizationID: organizationID}
	state, err := e.media.ReadDeletionState(ctx, meta, mediaFileID)
	if err != nil {
		var clientErr *platformclients.Error
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeNotFound {
			return e.confirmDeleted(ctx, tx, exportID, "")
		}
		return "", "", fmt.Errorf("retentionexec: expired export read-back: %w", err)
	}
	if state.DeletionState != "DELETED" {
		return "", "", fmt.Errorf("retentionexec: export %s deletion outcome still unresolved after read-back (state=%s)", exportID, state.DeletionState)
	}
	requestID := ""
	if state.Request != nil {
		requestID = state.Request.DeletionRequestID
	}
	return e.confirmDeleted(ctx, tx, exportID, requestID)
}

func (e *ExpiredExportExecutor) confirmDeleted(ctx context.Context, tx pgx.Tx, exportID, requestID string) (string, string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_export_jobs
		SET media_deletion_status = 'CONFIRMED', media_deletion_request_id = $2
		WHERE id = $1 AND media_deletion_status <> 'CONFIRMED'`, exportID, requestID)
	if err != nil {
		return "", "", fmt.Errorf("retentionexec: expired export confirm: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", "", fmt.Errorf("retentionexec: export %s already confirmed deleted or no longer exists", exportID)
	}
	return OutcomeArchived, "Media Service confirmed the underlying file is deleted", nil
}
