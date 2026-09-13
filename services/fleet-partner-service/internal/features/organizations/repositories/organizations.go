// Package repositories implements tenant-aware PostgreSQL repositories for
// the foundational organization schema.
//
// Every method on organization-owned tables requires the organization ID
// explicitly; every SQL statement filters on organization_id. Resources are
// never loaded globally and ownership-checked afterwards.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
	_ "time/tzdata" // embed IANA tzdata so timezone validation works on Windows and slim containers

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ErrNotFound is returned when a tenant-scoped lookup matches nothing. It is
// indistinguishable from "exists in another organization" by design (IDOR-safe).
var ErrNotFound = errors.New("organizations: not found")

// ErrTenantMismatch is returned when a referenced entity (branch, parent
// department) does not belong to the same organization.
var ErrTenantMismatch = errors.New("organizations: referenced entity belongs to a different organization")

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateTimezone checks that value is a resolvable IANA timezone.
func ValidateTimezone(value string) error {
	if value == "" || value == "Local" {
		return fmt.Errorf("organizations: timezone must be an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return fmt.Errorf("organizations: timezone %q is not a valid IANA timezone", value)
	}
	return nil
}

// ValidateCurrency checks the explicit three-letter currency format.
func ValidateCurrency(value string) error {
	if !currencyPattern.MatchString(value) {
		return fmt.Errorf("organizations: currency %q must be a three-letter uppercase code", value)
	}
	return nil
}

// OrganizationRepository stores organizations. Organization rows are the
// tenant root, so creation is platform-scoped (future onboarding flow);
// all reads after creation are by explicit organization ID.
type OrganizationRepository struct {
	db database.Querier
}

// NewOrganizationRepository wires the repository.
func NewOrganizationRepository(db database.Querier) *OrganizationRepository {
	return &OrganizationRepository{db: db}
}

// Update performs a full update of an organization's editable fields.
// Only updates if the organization is in DRAFT status.
func (r *OrganizationRepository) Update(ctx context.Context, q database.Querier, org *models.Organization) error {
	// Validate timezone and currency
	if err := ValidateTimezone(org.Timezone); err != nil {
		return err
	}
	if err := ValidateCurrency(org.DefaultCurrency); err != nil {
		return err
	}

	tag, err := q.Exec(ctx, `
        UPDATE fleet_partner_organizations SET
            legal_name = $1,
            trading_name = $2,
            registration_number = $3,
            country_code = $4,
            timezone = $5,
            default_currency = $6,
            operating_capabilities = $7,
            version = version + 1,
            updated_at = now()
        WHERE id = $8 AND version = $9 AND status = $10`,
		org.LegalName,
		org.TradingName,
		org.RegistrationNumber,
		org.CountryCode,
		org.Timezone,
		org.DefaultCurrency,
		// We need to marshal capabilities
		org.OperatingCapabilities,
		org.ID,
		org.Version,
		models.OrganizationStatusDraft, // Only allow updates in DRAFT status
	)
	if err != nil {
		return fmt.Errorf("organizations: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Check if the row exists and what status it has
		existing, err := r.GetByID(ctx, org.ID)
		if err != nil {
			return err
		}
		if existing.Status != models.OrganizationStatusDraft {
			return apperrors.Conflict("Company can only be edited while in DRAFT status.", nil)
		}
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// Create inserts a new organization foundation record after validating the
// timezone (IANA) and currency (explicit three-letter code).
func (r *OrganizationRepository) Create(ctx context.Context, q database.Querier, org *models.Organization) error {
	if err := ValidateTimezone(org.Timezone); err != nil {
		return err
	}
	if err := ValidateCurrency(org.DefaultCurrency); err != nil {
		return err
	}
	if org.ID == "" {
		org.ID = ids.New(ids.PrefixOrganization)
	}
	if org.Status == "" {
		org.Status = models.OrganizationStatusPendingReview
	}
	org.Version = 1
	capabilities, err := json.Marshal(org.OperatingCapabilities)
	if err != nil {
		return fmt.Errorf("organizations: marshal capabilities: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organizations
			(id, status, legal_name, trading_name, registration_number, country_code,
			 timezone, default_currency, operating_capabilities, version, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, now(), now())`,
		org.ID, org.Status, org.LegalName, org.TradingName, org.RegistrationNumber,
		org.CountryCode, org.Timezone, org.DefaultCurrency, capabilities, org.Version,
	)
	if err != nil {
		return fmt.Errorf("organizations: create: %w", err)
	}
	return nil
}

// GetByID loads one organization by its own ID (the tenant scope itself).
func (r *OrganizationRepository) GetByID(ctx context.Context, organizationID string) (*models.Organization, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, status, legal_name, COALESCE(trading_name, ''), registration_number,
			country_code, timezone, default_currency, operating_capabilities,
			version, created_at, updated_at, archived_at
		FROM fleet_partner_organizations
		WHERE id = $1`,
		organizationID,
	)
	return scanOrganization(row)
}

// UpdateStatus performs a compare-and-swap status transition and appends the
// status-history record in the same transaction.
func (r *OrganizationRepository) UpdateStatus(ctx context.Context, q database.Querier, organizationID string, expectedVersion int64, change models.StatusChange) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organizations SET
			status = $1, version = version + 1, updated_at = now(),
			archived_at = CASE WHEN $1 = 'ARCHIVED' THEN now() ELSE archived_at END
		WHERE id = $2 AND version = $3`,
		change.NewStatus, organizationID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("organizations: update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}

	evidence, err := json.Marshal(change.EvidenceFileIDs)
	if err != nil {
		return fmt.Errorf("organizations: marshal evidence: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_status_history
			(id, organization_id, previous_status, new_status, actor_type, actor_id,
			 reason_code, reason_text, evidence_file_ids, request_id, correlation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $11, now())`,
		ids.New(ids.PrefixStatusChange), organizationID, change.PreviousStatus, change.NewStatus,
		change.ActorType, change.ActorID, change.ReasonCode, change.ReasonText,
		evidence, change.RequestID, change.CorrelationID,
	)
	if err != nil {
		return fmt.Errorf("organizations: append status history: %w", err)
	}
	return nil
}

// StatusHistory lists the append-only transitions of one organization.
func (r *OrganizationRepository) StatusHistory(ctx context.Context, organizationID string) ([]models.StatusChange, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, previous_status, new_status, actor_type, actor_id,
			reason_code, COALESCE(reason_text, ''), evidence_file_ids,
			request_id, correlation_id, occurred_at
		FROM fleet_partner_organization_status_history
		WHERE organization_id = $1
		ORDER BY occurred_at DESC, id DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("organizations: status history: %w", err)
	}
	defer rows.Close()

	var history []models.StatusChange
	for rows.Next() {
		var change models.StatusChange
		var evidence []byte
		if err := rows.Scan(
			&change.ID, &change.OrganizationID, &change.PreviousStatus, &change.NewStatus,
			&change.ActorType, &change.ActorID, &change.ReasonCode, &change.ReasonText,
			&evidence, &change.RequestID, &change.CorrelationID, &change.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("organizations: scan status history: %w", err)
		}
		if len(evidence) > 0 {
			_ = json.Unmarshal(evidence, &change.EvidenceFileIDs)
		}
		history = append(history, change)
	}
	return history, rows.Err()
}

func scanOrganization(row pgx.Row) (*models.Organization, error) {
	var org models.Organization
	var capabilities []byte
	err := row.Scan(
		&org.ID, &org.Status, &org.LegalName, &org.TradingName, &org.RegistrationNumber,
		&org.CountryCode, &org.Timezone, &org.DefaultCurrency, &capabilities,
		&org.Version, &org.CreatedAt, &org.UpdatedAt, &org.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: scan: %w", err)
	}
	if len(capabilities) > 0 {
		_ = json.Unmarshal(capabilities, &org.OperatingCapabilities)
	}
	return &org, nil
}
