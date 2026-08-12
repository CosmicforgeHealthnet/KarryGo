package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
)

// Known feature-flag namespace prefixes. Unknown, un-namespaced flags are
// rejected so a typo cannot silently become configuration.
var allowedFlagPrefixes = []string{"fleet.", "corporate.", "experimental."}

// SettingsRepository stores the single current settings record per
// organization.
type SettingsRepository struct {
	db database.Querier
}

// NewSettingsRepository wires the repository.
func NewSettingsRepository(db database.Querier) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func validateSettings(settings *models.Settings) error {
	if settings.AssignmentTimeoutSeconds < 30 || settings.AssignmentTimeoutSeconds > 3600 {
		return fmt.Errorf("organizations: assignment timeout must be between 30 and 3600 seconds")
	}
	for flag := range settings.FeatureFlags {
		valid := false
		for _, prefix := range allowedFlagPrefixes {
			if len(flag) > len(prefix) && flag[:len(prefix)] == prefix {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("organizations: feature flag %q is not namespaced (%v)", flag, allowedFlagPrefixes)
		}
	}
	return nil
}

// Create inserts the initial settings record for an organization.
func (r *SettingsRepository) Create(ctx context.Context, q database.Querier, organizationID string, settings *models.Settings) error {
	if organizationID == "" {
		return fmt.Errorf("organizations: organization id is required")
	}
	if settings.AssignmentTimeoutSeconds == 0 {
		settings.AssignmentTimeoutSeconds = 300
	}
	if settings.ProviderVisibility == "" {
		settings.ProviderVisibility = "ORGANIZATION"
	}
	if err := validateSettings(settings); err != nil {
		return err
	}
	settings.OrganizationID = organizationID
	settings.Version = 1

	thresholds, retention, flags, err := marshalSettings(settings)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_settings
			(organization_id, assignment_timeout_seconds, approval_thresholds,
			 provider_visibility, retention_settings, feature_flags, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())`,
		organizationID, settings.AssignmentTimeoutSeconds, thresholds,
		settings.ProviderVisibility, retention, flags, settings.Version,
	)
	if err != nil {
		return fmt.Errorf("organizations: create settings: %w", err)
	}
	return nil
}

// Get loads the current settings of an organization.
func (r *SettingsRepository) Get(ctx context.Context, organizationID string) (*models.Settings, error) {
	var settings models.Settings
	var thresholds, retention, flags []byte
	err := r.db.QueryRow(ctx, `
		SELECT organization_id, assignment_timeout_seconds, approval_thresholds,
			provider_visibility, retention_settings, feature_flags, version, created_at, updated_at
		FROM fleet_partner_organization_settings
		WHERE organization_id = $1`,
		organizationID,
	).Scan(
		&settings.OrganizationID, &settings.AssignmentTimeoutSeconds, &thresholds,
		&settings.ProviderVisibility, &retention, &flags, &settings.Version,
		&settings.CreatedAt, &settings.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: get settings: %w", err)
	}
	_ = json.Unmarshal(thresholds, &settings.ApprovalThresholds)
	_ = json.Unmarshal(retention, &settings.RetentionSettings)
	_ = json.Unmarshal(flags, &settings.FeatureFlags)
	return &settings, nil
}

// Update performs a compare-and-swap settings update.
func (r *SettingsRepository) Update(ctx context.Context, q database.Querier, organizationID string, expectedVersion int64, settings *models.Settings) error {
	if err := validateSettings(settings); err != nil {
		return err
	}
	thresholds, retention, flags, err := marshalSettings(settings)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_settings SET
			assignment_timeout_seconds = $1, approval_thresholds = $2,
			provider_visibility = $3, retention_settings = $4, feature_flags = $5,
			version = version + 1, updated_at = now()
		WHERE organization_id = $6 AND version = $7`,
		settings.AssignmentTimeoutSeconds, thresholds, settings.ProviderVisibility,
		retention, flags, organizationID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("organizations: update settings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

func marshalSettings(settings *models.Settings) (thresholds []byte, retention []byte, flags []byte, err error) {
	if thresholds, err = json.Marshal(orEmptyMap(settings.ApprovalThresholds)); err != nil {
		return nil, nil, nil, fmt.Errorf("organizations: marshal thresholds: %w", err)
	}
	if retention, err = json.Marshal(orEmptyMap(settings.RetentionSettings)); err != nil {
		return nil, nil, nil, fmt.Errorf("organizations: marshal retention: %w", err)
	}
	flagMap := settings.FeatureFlags
	if flagMap == nil {
		flagMap = map[string]bool{}
	}
	if flags, err = json.Marshal(flagMap); err != nil {
		return nil, nil, nil, fmt.Errorf("organizations: marshal flags: %w", err)
	}
	return thresholds, retention, flags, nil
}
