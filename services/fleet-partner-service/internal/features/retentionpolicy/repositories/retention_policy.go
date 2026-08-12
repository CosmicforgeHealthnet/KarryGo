// Package repositories persists per-organization, per-class retention policy
// overrides.
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
)

// ErrVersionMismatch means ExpectedVersion did not match the stored row.
var ErrVersionMismatch = errors.New("retentionpolicy: expected version does not match")

// Repository reads and writes fleet_partner_retention_policies.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository over a pool or transaction.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

// Get returns the organization's policy for class, or the platform minimum
// (Version 0, meaning "no override recorded") if none has been set.
func (r *Repository) Get(ctx context.Context, organizationID, class string) (models.Policy, error) {
	row := r.db.QueryRow(ctx, `
		SELECT organization_id, retention_class, retention_days, version
		FROM fleet_partner_retention_policies WHERE organization_id = $1 AND retention_class = $2`,
		organizationID, class)
	var p models.Policy
	err := row.Scan(&p.OrganizationID, &p.RetentionClass, &p.RetentionDays, &p.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Policy{OrganizationID: organizationID, RetentionClass: class,
				RetentionDays: models.PlatformMinimumDays[class], Version: 0}, nil
		}
		return models.Policy{}, fmt.Errorf("retentionpolicy: get: %w", err)
	}
	return p, nil
}

// List returns every class the organization has explicitly overridden.
// Classes with no row still exist implicitly at the platform minimum; the
// HTTP layer fills those in from models.PlatformMinimumDays.
func (r *Repository) List(ctx context.Context, organizationID string) ([]models.Policy, error) {
	rows, err := r.db.Query(ctx, `
		SELECT organization_id, retention_class, retention_days, version
		FROM fleet_partner_retention_policies WHERE organization_id = $1
		ORDER BY retention_class`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("retentionpolicy: list: %w", err)
	}
	defer rows.Close()
	var policies []models.Policy
	for rows.Next() {
		var p models.Policy
		if err := rows.Scan(&p.OrganizationID, &p.RetentionClass, &p.RetentionDays, &p.Version); err != nil {
			return nil, fmt.Errorf("retentionpolicy: scan: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// Upsert creates or updates the organization's override for a class under
// optimistic concurrency: expectedVersion 0 means "create."
func (r *Repository) Upsert(ctx context.Context, policy models.Policy, expectedVersion int64) (models.Policy, error) {
	if expectedVersion == 0 {
		row := r.db.QueryRow(ctx, `
			INSERT INTO fleet_partner_retention_policies (organization_id, retention_class, retention_days, version)
			VALUES ($1, $2, $3, 1)
			ON CONFLICT (organization_id, retention_class) DO NOTHING
			RETURNING organization_id, retention_class, retention_days, version`,
			policy.OrganizationID, policy.RetentionClass, policy.RetentionDays)
		var created models.Policy
		err := row.Scan(&created.OrganizationID, &created.RetentionClass, &created.RetentionDays, &created.Version)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Policy{}, ErrVersionMismatch
			}
			return models.Policy{}, fmt.Errorf("retentionpolicy: create: %w", err)
		}
		return created, nil
	}

	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_retention_policies
		SET retention_days = $1, version = version + 1, updated_at = now()
		WHERE organization_id = $2 AND retention_class = $3 AND version = $4
		RETURNING organization_id, retention_class, retention_days, version`,
		policy.RetentionDays, policy.OrganizationID, policy.RetentionClass, expectedVersion)
	var updated models.Policy
	err := row.Scan(&updated.OrganizationID, &updated.RetentionClass, &updated.RetentionDays, &updated.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Policy{}, ErrVersionMismatch
		}
		return models.Policy{}, fmt.Errorf("retentionpolicy: update: %w", err)
	}
	return updated, nil
}
