package filemetadatarepositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Legacy asset event types (media_legacy_asset_events, migration 000009).
const (
	LegacyEventTombstoned      = "TOMBSTONED"
	LegacyEventDeletionBlocked = "DELETION_BLOCKED"
)

// ErrLegacyDeletionBlocked is returned when a recorded legal hold or an
// unexpired recorded retention forbids deleting a legacy asset.
var ErrLegacyDeletionBlocked = errors.New("legacy asset deletion blocked by legal hold or retention")

// LegacyGovernance holds the Section 19 controls over legacy media_assets
// rows: the adoption lookup that stops legacy serving once an asset has a
// modern identity, quarantine on a positive malware verdict, and privacy
// deletion as a redacting tombstone with an immutable event row.
type LegacyGovernance interface {
	// AdoptedObject reports whether the legacy asset was adopted into a
	// modern private object. The modern ID is never returned to a public
	// caller; it is exposed only on the HMAC private mapping route.
	AdoptedObject(ctx context.Context, assetID string) (string, bool, error)
	// Quarantine marks a legacy asset whose bytes a scanner flagged.
	Quarantine(ctx context.Context, assetID string) error
	// Tombstone deletes the bytes (via destroy) and redacts the row, unless a
	// recorded legal hold or retention forbids it. Both outcomes are written
	// to the immutable event trail.
	Tombstone(ctx context.Context, assetID, actorService, reasonCode string, destroy func(ctx context.Context, storagePath string) error) error
}

type PostgresLegacyGovernance struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewPostgresLegacyGovernance(db *pgxpool.Pool) *PostgresLegacyGovernance {
	return &PostgresLegacyGovernance{db: db, now: time.Now}
}

func (g *PostgresLegacyGovernance) AdoptedObject(ctx context.Context, assetID string) (string, bool, error) {
	var id string
	err := g.db.QueryRow(ctx, `SELECT media_object_id FROM media_legacy_adoptions WHERE legacy_asset_id=$1::uuid`, assetID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (g *PostgresLegacyGovernance) Quarantine(ctx context.Context, assetID string) error {
	_, err := g.db.Exec(ctx, `UPDATE media_assets SET status='quarantined', updated_at=now() WHERE id=$1::uuid AND deleted_at IS NULL`, assetID)
	return err
}

func (g *PostgresLegacyGovernance) Tombstone(ctx context.Context, assetID, actorService, reasonCode string, destroy func(ctx context.Context, storagePath string) error) error {
	tx, err := g.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var storagePath string
	var legalHold bool
	var retentionUntil, deletedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT storage_path, legal_hold, retention_until, deleted_at FROM media_assets WHERE id=$1::uuid FOR UPDATE`, assetID).
		Scan(&storagePath, &legalHold, &retentionUntil, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && deletedAt != nil {
		return apperrors.NotFound("Media file could not be found.", nil)
	}
	if err != nil {
		return err
	}

	blocked := ""
	switch {
	case legalHold:
		blocked = "LEGAL_HOLD"
	case retentionUntil != nil && retentionUntil.After(g.now().UTC()):
		blocked = "RETENTION_UNEXPIRED"
	}
	if blocked != "" {
		// A refused deletion is itself evidence: record it, then refuse.
		if _, err := tx.Exec(ctx, `INSERT INTO media_legacy_asset_events(asset_id,event_type,actor_service,reason_code) VALUES($1::uuid,$2,$3,$4)`,
			assetID, LegacyEventDeletionBlocked, actorService, blocked); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return ErrLegacyDeletionBlocked
	}

	if destroy != nil {
		if err := destroy(ctx, storagePath); err != nil {
			return apperrors.Unavailable("File storage is temporarily unavailable.", err)
		}
	}
	// Redact everything caller-supplied or derived from caller input: the
	// filename, free-form metadata, the URL and the storage path (which
	// embedded the owner id and filename). The checksum, size, purpose and
	// owner identity stay as non-content evidence of what existed.
	if _, err := tx.Exec(ctx, `UPDATE media_assets SET status='deleted', deleted_at=COALESCE(deleted_at, now()), tombstoned_at=now(),
		original_filename='', metadata='{}'::jsonb, public_url='', storage_path='tombstoned/' || id::text, updated_at=now()
		WHERE id=$1::uuid`, assetID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO media_legacy_asset_events(asset_id,event_type,actor_service,reason_code) VALUES($1::uuid,$2,$3,$4)`,
		assetID, LegacyEventTombstoned, actorService, reasonCode); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
