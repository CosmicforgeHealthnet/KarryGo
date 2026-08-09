// Package repositories implements PostgreSQL persistence for the Section 15
// service catalogue.
//
// Every read and every write carries organization_id in SQL; a handler check
// is a convenience, the WHERE clause is the control.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
)

// Errors returned by the service catalogue repository.
var (
	// ErrNotFound covers both "no such row" and "not yours".
	ErrNotFound = errors.New("servicecatalog: record not found")
	// ErrVersionConflict is an optimistic-concurrency failure.
	ErrVersionConflict = errors.New("servicecatalog: expected version does not match")
	// ErrDuplicate is a uniqueness violation inside the organization (a reused code).
	ErrDuplicate = errors.New("servicecatalog: conflicting record")
)

// PrefixCatalogItem is the opaque ID prefix for catalogue items.
const PrefixCatalogItem = "svc"

// Repository persists the Section 15 service catalogue.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

func classify(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case isUniqueViolation(err):
		return ErrDuplicate
	default:
		return fmt.Errorf("servicecatalog: %s: %w", operation, err)
	}
}

func scanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func encodeJSONObject(value models.JSONMap) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(map[string]any(value))
	if err != nil {
		return nil, fmt.Errorf("servicecatalog: encode json: %w", err)
	}
	return encoded, nil
}

func decodeJSONObject(raw []byte) models.JSONMap {
	if len(raw) == 0 {
		return models.JSONMap{}
	}
	var out models.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return models.JSONMap{}
	}
	return out
}

func encodeStringSlice(values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("servicecatalog: encode string slice: %w", err)
	}
	return encoded, nil
}

func decodeStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

const catalogColumns = `id, organization_id, COALESCE(branch_id, ''), name, code, category, description,
	capability_reference, required_fields_schema, supported_vehicle_classes, supported_package_types,
	pricing_policy_reference, approval_policy_reference, proof_policy_reference, status, version,
	created_by, updated_by, created_at, updated_at, archived_at`

func scanItem(row pgx.Row) (models.CatalogItem, error) {
	var item models.CatalogItem
	var schemaRaw, vehiclesRaw, packagesRaw []byte
	err := row.Scan(&item.ID, &item.OrganizationID, &item.BranchID, &item.Name, &item.Code,
		&item.Category, &item.Description, &item.CapabilityReference, &schemaRaw, &vehiclesRaw,
		&packagesRaw, &item.PricingPolicyReference, &item.ApprovalPolicyReference,
		&item.ProofPolicyReference, &item.Status, &item.Version, &item.CreatedBy, &item.UpdatedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.ArchivedAt)
	if err != nil {
		return item, scanErr(err)
	}
	item.RequiredFieldsSchema = decodeJSONObject(schemaRaw)
	item.SupportedVehicleClasses = decodeStringSlice(vehiclesRaw)
	item.SupportedPackageTypes = decodeStringSlice(packagesRaw)
	return item, nil
}

// Create inserts a new catalogue item.
func (r *Repository) Create(ctx context.Context, q database.Querier,
	item models.CatalogItem) (models.CatalogItem, error) {
	schema, err := encodeJSONObject(item.RequiredFieldsSchema)
	if err != nil {
		return models.CatalogItem{}, err
	}
	vehicles, err := encodeStringSlice(item.SupportedVehicleClasses)
	if err != nil {
		return models.CatalogItem{}, err
	}
	packages, err := encodeStringSlice(item.SupportedPackageTypes)
	if err != nil {
		return models.CatalogItem{}, err
	}

	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_service_catalog_items
			(id, organization_id, branch_id, name, code, category, description,
			 capability_reference, required_fields_schema, supported_vehicle_classes,
			 supported_package_types, pricing_policy_reference, approval_policy_reference,
			 proof_policy_reference, status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
		RETURNING `+catalogColumns,
		item.ID, item.OrganizationID, nullable(item.BranchID), item.Name, item.Code, item.Category,
		item.Description, item.CapabilityReference, schema, vehicles, packages,
		item.PricingPolicyReference, item.ApprovalPolicyReference, item.ProofPolicyReference,
		item.Status, item.CreatedBy)
	created, err := scanItem(row)
	if err != nil {
		return created, classify(err, "create catalog item")
	}
	return created, nil
}

// Get reads one catalogue item inside the organization.
func (r *Repository) Get(ctx context.Context, q database.Querier,
	organizationID, itemID string) (models.CatalogItem, error) {
	return scanItem(r.q(q).QueryRow(ctx, `SELECT `+catalogColumns+`
		FROM fleet_partner_service_catalog_items WHERE organization_id = $1 AND id = $2`,
		organizationID, itemID))
}

// Lock reads one catalogue item FOR UPDATE, serializing concurrent transitions.
func (r *Repository) Lock(ctx context.Context, q database.Querier,
	organizationID, itemID string) (models.CatalogItem, error) {
	return scanItem(r.q(q).QueryRow(ctx, `SELECT `+catalogColumns+`
		FROM fleet_partner_service_catalog_items WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, itemID))
}

// Filter is the allowlisted catalogue list query.
type Filter struct {
	Status    string
	Category  string
	BranchID  string
	Cursor    cursor.Cursor
	HasCursor bool
	Limit     int
}

// List returns a tenant-scoped page, newest first.
func (r *Repository) List(ctx context.Context, organizationID string, filter Filter) ([]models.CatalogItem, error) {
	args := []any{organizationID, filter.Status, filter.Category, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR category = $3)
		AND ($4 = '' OR branch_id = $4)`

	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}

	limit := cursor.ClampLimit(filter.Limit)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `SELECT `+catalogColumns+`
		FROM fleet_partner_service_catalog_items
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("servicecatalog: list: %w", err)
	}
	defer rows.Close()

	var items []models.CatalogItem
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("servicecatalog: list scan: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateFields is a version-checked update of the mutable catalogue fields.
func (r *Repository) UpdateFields(ctx context.Context, q database.Querier, organizationID, itemID string,
	expectedVersion int64, item models.CatalogItem) (models.CatalogItem, error) {
	schema, err := encodeJSONObject(item.RequiredFieldsSchema)
	if err != nil {
		return models.CatalogItem{}, err
	}
	vehicles, err := encodeStringSlice(item.SupportedVehicleClasses)
	if err != nil {
		return models.CatalogItem{}, err
	}
	packages, err := encodeStringSlice(item.SupportedPackageTypes)
	if err != nil {
		return models.CatalogItem{}, err
	}

	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_service_catalog_items SET
			name = $1, description = $2, capability_reference = $3, required_fields_schema = $4,
			supported_vehicle_classes = $5, supported_package_types = $6, pricing_policy_reference = $7,
			approval_policy_reference = $8, proof_policy_reference = $9, updated_by = $10,
			version = version + 1, updated_at = now()
		WHERE organization_id = $11 AND id = $12 AND version = $13
		RETURNING `+catalogColumns,
		item.Name, item.Description, item.CapabilityReference, schema, vehicles, packages,
		item.PricingPolicyReference, item.ApprovalPolicyReference, item.ProofPolicyReference,
		item.UpdatedBy, organizationID, itemID, expectedVersion)
	updated, err := scanItem(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "update catalog item")
	}
	return updated, nil
}

// SetStatus is a version-checked status transition.
func (r *Repository) SetStatus(ctx context.Context, q database.Querier, organizationID, itemID string,
	expectedVersion int64, newStatus string, actorID string, archived bool) (models.CatalogItem, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_service_catalog_items SET
			status = $1, updated_by = $2, version = version + 1, updated_at = now(),
			archived_at = CASE WHEN $3 THEN now() ELSE archived_at END
		WHERE organization_id = $4 AND id = $5 AND version = $6
		RETURNING `+catalogColumns,
		newStatus, actorID, archived, organizationID, itemID, expectedVersion)
	updated, err := scanItem(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "set catalog item status")
	}
	return updated, nil
}
