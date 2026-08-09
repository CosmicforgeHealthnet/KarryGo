// Package repositories persists generic (non-document) legal holds.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// PrefixLegalHold is this feature's opaque-ID prefix.
const PrefixLegalHold = "lgh"

// ErrNotFound means the hold does not exist within the caller's organization.
var ErrNotFound = errors.New("legalhold: not found")

// ErrVersionMismatch means ExpectedVersion did not match the stored row.
var ErrVersionMismatch = errors.New("legalhold: expected version does not match")

// ErrAlreadyReleased means Release was called on a hold that is not ACTIVE.
var ErrAlreadyReleased = errors.New("legalhold: hold is not active")

const holdColumns = `
	id, organization_id, scope_type, scope_reference, reason, status,
	created_by_actor_type, created_by_actor_id, COALESCE(approved_by_actor_id, ''),
	COALESCE(step_up_reference, ''), COALESCE(released_by_actor_id, ''),
	COALESCE(release_reason, ''), created_at, released_at, version`

// Repository reads and writes fleet_partner_legal_holds.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository over a pool.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

// Create inserts a new ACTIVE hold.
func (r *Repository) Create(ctx context.Context, hold models.Hold) (models.Hold, error) {
	hold.ID = ids.New(PrefixLegalHold)
	hold.Status = models.StatusActive
	hold.Version = 1
	row := r.db.QueryRow(ctx, `
		INSERT INTO fleet_partner_legal_holds
			(id, organization_id, scope_type, scope_reference, reason, status,
			 created_by_actor_type, created_by_actor_id, approved_by_actor_id,
			 step_up_reference, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11)
		RETURNING `+holdColumns,
		hold.ID, hold.OrganizationID, hold.ScopeType, hold.ScopeReference, hold.Reason, hold.Status,
		hold.CreatedByActorType, hold.CreatedByActorID, hold.ApprovedByActorID, hold.StepUpReference, hold.Version)
	created, err := scanHold(row)
	if err != nil {
		return models.Hold{}, fmt.Errorf("legalhold: create: %w", err)
	}
	return created, nil
}

// Get returns one hold scoped to the caller's organization.
func (r *Repository) Get(ctx context.Context, organizationID, id string) (models.Hold, error) {
	row := r.db.QueryRow(ctx, `SELECT `+holdColumns+`
		FROM fleet_partner_legal_holds WHERE organization_id = $1 AND id = $2`, organizationID, id)
	hold, err := scanHold(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Hold{}, ErrNotFound
		}
		return models.Hold{}, fmt.Errorf("legalhold: get: %w", err)
	}
	return hold, nil
}

// ListFilter narrows a tenant-scoped listing.
type ListFilter struct {
	OrganizationID string
	ScopeType      string
	ScopeReference string
	Status         string
	Limit          int
}

// List returns holds matching filter, newest first.
func (r *Repository) List(ctx context.Context, filter ListFilter) ([]models.Hold, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	conditions := []string{"organization_id = $1"}
	args := []any{filter.OrganizationID}
	if filter.ScopeType != "" {
		args = append(args, filter.ScopeType)
		conditions = append(conditions, fmt.Sprintf("scope_type = $%d", len(args)))
	}
	if filter.ScopeReference != "" {
		args = append(args, filter.ScopeReference)
		conditions = append(conditions, fmt.Sprintf("scope_reference = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`SELECT %s FROM fleet_partner_legal_holds WHERE %s
		ORDER BY created_at DESC, id DESC LIMIT $%d`, holdColumns, joinAnd(conditions), len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("legalhold: list: %w", err)
	}
	defer rows.Close()
	var holds []models.Hold
	for rows.Next() {
		hold, err := scanHold(rows)
		if err != nil {
			return nil, fmt.Errorf("legalhold: scan: %w", err)
		}
		holds = append(holds, hold)
	}
	return holds, rows.Err()
}

// IsUnderHold reports whether any ACTIVE hold exists for the given scope.
// Business-domain deletion/anonymization paths call this before proceeding;
// it is the one query that must never be skipped when wiring a new domain
// into legal-hold enforcement.
func (r *Repository) IsUnderHold(ctx context.Context, organizationID, scopeType, scopeReference string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM fleet_partner_legal_holds
			WHERE organization_id = $1 AND scope_type = $2 AND scope_reference = $3 AND status = 'ACTIVE'
		)`, organizationID, scopeType, scopeReference).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("legalhold: is under hold: %w", err)
	}
	return exists, nil
}

// Release transitions an ACTIVE hold to RELEASED under optimistic
// concurrency. Concurrent hold creation is not in tension with release (a
// release only ever affects the one row it targets), but release itself
// must not silently no-op against a stale read.
func (r *Repository) Release(ctx context.Context, organizationID, id string, expectedVersion int64,
	releasedByActorID, releaseReason string) (models.Hold, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_legal_holds
		SET status = 'RELEASED', released_by_actor_id = $1, release_reason = NULLIF($2, ''),
		    released_at = now(), version = version + 1
		WHERE organization_id = $3 AND id = $4 AND version = $5 AND status = 'ACTIVE'
		RETURNING `+holdColumns,
		releasedByActorID, releaseReason, organizationID, id, expectedVersion)
	hold, err := scanHold(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.Get(ctx, organizationID, id)
			if getErr != nil {
				return models.Hold{}, getErr
			}
			if !current.Active() {
				return models.Hold{}, ErrAlreadyReleased
			}
			return models.Hold{}, ErrVersionMismatch
		}
		return models.Hold{}, fmt.Errorf("legalhold: release: %w", err)
	}
	return hold, nil
}

func joinAnd(conditions []string) string {
	out := conditions[0]
	for _, c := range conditions[1:] {
		out += " AND " + c
	}
	return out
}

type scannable interface {
	Scan(dest ...any) error
}

func scanHold(row scannable) (models.Hold, error) {
	var (
		hold       models.Hold
		releasedAt *time.Time
	)
	err := row.Scan(
		&hold.ID, &hold.OrganizationID, &hold.ScopeType, &hold.ScopeReference, &hold.Reason, &hold.Status,
		&hold.CreatedByActorType, &hold.CreatedByActorID, &hold.ApprovedByActorID,
		&hold.StepUpReference, &hold.ReleasedByActorID, &hold.ReleaseReason,
		&hold.CreatedAt, &releasedAt, &hold.Version,
	)
	if err != nil {
		return models.Hold{}, err
	}
	hold.ReleasedAt = releasedAt
	return hold, nil
}
