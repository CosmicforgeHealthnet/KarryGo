// Package repositories persists per-organization security settings.
package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/models"
)

// ErrVersionMismatch means ExpectedVersion did not match the stored row (or
// the caller expected a row to already exist when it does not, or vice
// versa).
var ErrVersionMismatch = errors.New("securitysettings: expected version does not match")

const settingsColumns = `
	organization_id, mfa_required_for_sensitive_actions, step_up_freshness_seconds,
	session_revocation_policy, allowed_provider_access_policy, sensitive_export_threshold_kobo,
	restricted_document_step_up_required, audit_capture_policy, retention_preference_days, version`

// Repository reads and writes fleet_partner_security_settings.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository over a pool or transaction.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

// Get returns the organization's settings, or models.Defaults with Version 0
// if no row has ever been written.
func (r *Repository) Get(ctx context.Context, organizationID string) (models.Settings, error) {
	row := r.db.QueryRow(ctx, `SELECT `+settingsColumns+`
		FROM fleet_partner_security_settings WHERE organization_id = $1`, organizationID)
	settings, err := scanSettings(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Defaults(organizationID), nil
		}
		return models.Settings{}, fmt.Errorf("securitysettings: get: %w", err)
	}
	return settings, nil
}

// Upsert creates or updates the organization's settings row under optimistic
// concurrency: expectedVersion 0 means "create" (fails if a row already
// exists); a positive expectedVersion means "update this exact version."
func (r *Repository) Upsert(ctx context.Context, settings models.Settings, expectedVersion int64) (models.Settings, error) {
	if expectedVersion == 0 {
		row := r.db.QueryRow(ctx, `
			INSERT INTO fleet_partner_security_settings (`+settingsColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)
			ON CONFLICT (organization_id) DO NOTHING
			RETURNING `+settingsColumns,
			settings.OrganizationID, settings.MFARequiredForSensitiveActions, settings.StepUpFreshnessSeconds,
			settings.SessionRevocationPolicy, settings.AllowedProviderAccessPolicy, settings.SensitiveExportThresholdKobo,
			settings.RestrictedDocumentStepUpRequired, settings.AuditCapturePolicy, settings.RetentionPreferenceDays)
		created, err := scanSettings(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Settings{}, ErrVersionMismatch
			}
			return models.Settings{}, fmt.Errorf("securitysettings: create: %w", err)
		}
		return created, nil
	}

	row := r.db.QueryRow(ctx, `
		UPDATE fleet_partner_security_settings
		SET mfa_required_for_sensitive_actions = $1, step_up_freshness_seconds = $2,
		    session_revocation_policy = $3, allowed_provider_access_policy = $4,
		    sensitive_export_threshold_kobo = $5, restricted_document_step_up_required = $6,
		    audit_capture_policy = $7, retention_preference_days = $8,
		    version = version + 1, updated_at = now()
		WHERE organization_id = $9 AND version = $10
		RETURNING `+settingsColumns,
		settings.MFARequiredForSensitiveActions, settings.StepUpFreshnessSeconds,
		settings.SessionRevocationPolicy, settings.AllowedProviderAccessPolicy, settings.SensitiveExportThresholdKobo,
		settings.RestrictedDocumentStepUpRequired, settings.AuditCapturePolicy, settings.RetentionPreferenceDays,
		settings.OrganizationID, expectedVersion)
	updated, err := scanSettings(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Settings{}, ErrVersionMismatch
		}
		return models.Settings{}, fmt.Errorf("securitysettings: update: %w", err)
	}
	return updated, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSettings(row scannable) (models.Settings, error) {
	var s models.Settings
	err := row.Scan(
		&s.OrganizationID, &s.MFARequiredForSensitiveActions, &s.StepUpFreshnessSeconds,
		&s.SessionRevocationPolicy, &s.AllowedProviderAccessPolicy, &s.SensitiveExportThresholdKobo,
		&s.RestrictedDocumentStepUpRequired, &s.AuditCapturePolicy, &s.RetentionPreferenceDays, &s.Version,
	)
	if err != nil {
		return models.Settings{}, err
	}
	return s, nil
}
