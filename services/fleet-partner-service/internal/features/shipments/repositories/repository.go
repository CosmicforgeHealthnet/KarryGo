// Package repositories implements PostgreSQL persistence for Section 15
// shipments and cargo items.
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
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Errors returned by the shipments repository.
var (
	ErrNotFound        = errors.New("shipments: record not found")
	ErrVersionConflict = errors.New("shipments: expected version does not match")
)

// ID prefixes for Section 15 shipment records.
const (
	PrefixShipment = "shp"
	PrefixCargo    = "cgo"
	PrefixHistory  = "shh"
)

// Repository persists Section 15 shipments and cargo items.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func classify(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	default:
		return fmt.Errorf("shipments: %s: %w", operation, err)
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
		return nil, fmt.Errorf("shipments: encode json: %w", err)
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

// ---------------------------------------------------------------------------
// Shipments
// ---------------------------------------------------------------------------

const shipmentColumns = `id, organization_id, COALESCE(branch_id, ''), order_id, origin_stop_id,
	destination_stop_id, cargo_profile, passenger_profile, required_vehicle_class,
	required_service_capability, temperature_requirements, declared_value_amount_minor,
	declared_value_currency, package_count, status, version, created_by, updated_by,
	created_at, updated_at, completed_at, cancelled_at`

func scanShipment(row pgx.Row) (models.Shipment, error) {
	var s models.Shipment
	var cargoRaw, passengerRaw, temperatureRaw []byte
	err := row.Scan(&s.ID, &s.OrganizationID, &s.BranchID, &s.OrderID, &s.OriginStopID,
		&s.DestinationStopID, &cargoRaw, &passengerRaw, &s.RequiredVehicleClass,
		&s.RequiredServiceCapability, &temperatureRaw, &s.DeclaredValueAmountMinor,
		&s.DeclaredValueCurrency, &s.PackageCount, &s.Status, &s.Version, &s.CreatedBy,
		&s.UpdatedBy, &s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.CancelledAt)
	if err != nil {
		return s, scanErr(err)
	}
	s.CargoProfile = decodeJSONObject(cargoRaw)
	s.PassengerProfile = decodeJSONObject(passengerRaw)
	s.TemperatureRequirements = decodeJSONObject(temperatureRaw)
	return s, nil
}

// Create inserts a new DRAFT shipment.
func (r *Repository) Create(ctx context.Context, q database.Querier, s models.Shipment) (models.Shipment, error) {
	cargo, err := encodeJSONObject(s.CargoProfile)
	if err != nil {
		return models.Shipment{}, err
	}
	passenger, err := encodeJSONObject(s.PassengerProfile)
	if err != nil {
		return models.Shipment{}, err
	}
	temperature, err := encodeJSONObject(s.TemperatureRequirements)
	if err != nil {
		return models.Shipment{}, err
	}

	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_shipments
			(id, organization_id, branch_id, order_id, origin_stop_id, destination_stop_id,
			 cargo_profile, passenger_profile, required_vehicle_class, required_service_capability,
			 temperature_requirements, declared_value_amount_minor, declared_value_currency,
			 status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
		RETURNING `+shipmentColumns,
		s.ID, s.OrganizationID, nullable(s.BranchID), s.OrderID, s.OriginStopID, s.DestinationStopID,
		cargo, passenger, s.RequiredVehicleClass, s.RequiredServiceCapability, temperature,
		s.DeclaredValueAmountMinor, s.DeclaredValueCurrency, s.Status, s.CreatedBy)
	created, err := scanShipment(row)
	if err != nil {
		return created, classify(err, "create shipment")
	}
	return created, nil
}

// Get reads one shipment inside the organization.
func (r *Repository) Get(ctx context.Context, q database.Querier, organizationID, shipmentID string) (models.Shipment, error) {
	return scanShipment(r.q(q).QueryRow(ctx, `SELECT `+shipmentColumns+`
		FROM fleet_partner_shipments WHERE organization_id = $1 AND id = $2`,
		organizationID, shipmentID))
}

// Lock reads one shipment FOR UPDATE, serializing concurrent transitions.
func (r *Repository) Lock(ctx context.Context, q database.Querier, organizationID, shipmentID string) (models.Shipment, error) {
	return scanShipment(r.q(q).QueryRow(ctx, `SELECT `+shipmentColumns+`
		FROM fleet_partner_shipments WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, shipmentID))
}

// SetStatus applies one version-checked lifecycle transition. Callers must
// validate the transition with the shipment state machine before invoking it.
func (r *Repository) SetStatus(ctx context.Context, q database.Querier, organizationID,
	shipmentID string, expectedVersion int64, status, actorID string) (models.Shipment, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_shipments SET status=$1, updated_by=$2,
			version=version+1, updated_at=now(),
			completed_at=CASE WHEN $1='COMPLETED' THEN now() ELSE completed_at END,
			cancelled_at=CASE WHEN $1='CANCELLED' THEN now() ELSE cancelled_at END
		WHERE organization_id=$3 AND id=$4 AND version=$5
		RETURNING `+shipmentColumns,
		status, actorID, organizationID, shipmentID, expectedVersion)
	updated, err := scanShipment(row)
	if errors.Is(err, ErrNotFound) {
		return updated, ErrVersionConflict
	}
	if err != nil {
		return updated, classify(err, "set shipment status")
	}
	return updated, nil
}

// Filter is the allowlisted shipment list query.
type Filter struct {
	Status    string
	OrderID   string
	BranchID  string
	Cursor    cursor.Cursor
	HasCursor bool
	Limit     int
}

// List returns a tenant-scoped page, newest first.
func (r *Repository) List(ctx context.Context, organizationID string, filter Filter) ([]models.Shipment, error) {
	args := []any{organizationID, filter.Status, filter.OrderID, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR order_id = $3)
		AND ($4 = '' OR branch_id = $4)`

	if filter.HasCursor {
		args = append(args, filter.Cursor.SortTime(), filter.Cursor.LastID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	limit := cursor.ClampLimit(filter.Limit)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, `SELECT `+shipmentColumns+`
		FROM fleet_partner_shipments
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("shipments: list: %w", err)
	}
	defer rows.Close()

	var shipments []models.Shipment
	for rows.Next() {
		s, err := scanShipment(rows)
		if err != nil {
			return nil, fmt.Errorf("shipments: list scan: %w", err)
		}
		shipments = append(shipments, s)
	}
	return shipments, rows.Err()
}

// UpdateFields is a version-checked update of the mutable shipment fields.
// Only reachable while the shipment is DRAFT — the usecase enforces the lock,
// this is the persistence half.
func (r *Repository) UpdateFields(ctx context.Context, q database.Querier, organizationID, shipmentID string,
	expectedVersion int64, s models.Shipment) (models.Shipment, error) {
	cargo, err := encodeJSONObject(s.CargoProfile)
	if err != nil {
		return models.Shipment{}, err
	}
	passenger, err := encodeJSONObject(s.PassengerProfile)
	if err != nil {
		return models.Shipment{}, err
	}
	temperature, err := encodeJSONObject(s.TemperatureRequirements)
	if err != nil {
		return models.Shipment{}, err
	}

	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_shipments SET
			cargo_profile = $1, passenger_profile = $2, required_vehicle_class = $3,
			required_service_capability = $4, temperature_requirements = $5,
			declared_value_amount_minor = $6, declared_value_currency = $7, updated_by = $8,
			version = version + 1, updated_at = now()
		WHERE organization_id = $9 AND id = $10 AND version = $11
		RETURNING `+shipmentColumns,
		cargo, passenger, s.RequiredVehicleClass, s.RequiredServiceCapability, temperature,
		s.DeclaredValueAmountMinor, s.DeclaredValueCurrency, s.UpdatedBy,
		organizationID, shipmentID, expectedVersion)
	updated, err := scanShipment(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "update shipment")
	}
	return updated, nil
}

// AppendHistory writes one immutable shipment status change.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier, h models.ShipmentHistory) error {
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_shipment_history
			(id, organization_id, shipment_id, previous_status, new_status, reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ids.New(PrefixHistory), h.OrganizationID, h.ShipmentID, h.PreviousStatus, h.NewStatus,
		h.ReasonCode, h.ActorType, h.ActorID, h.CorrelationID)
	if err != nil {
		return fmt.Errorf("shipments: append history: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Cargo items
// ---------------------------------------------------------------------------

const cargoColumns = `id, organization_id, shipment_id, description, quantity, unit,
	handling_category, hazardous_classification, temperature_min, temperature_max,
	temperature_unit, declared_value_amount_minor, declared_value_currency,
	restricted_item_status, compliance_reference, compliance_decision_version, version,
	created_by, updated_by, created_at, updated_at`

func scanCargoItem(row pgx.Row) (models.CargoItem, error) {
	var c models.CargoItem
	err := row.Scan(&c.ID, &c.OrganizationID, &c.ShipmentID, &c.Description, &c.Quantity, &c.Unit,
		&c.HandlingCategory, &c.HazardousClassification, &c.TemperatureMin, &c.TemperatureMax,
		&c.TemperatureUnit, &c.DeclaredValueAmountMinor, &c.DeclaredValueCurrency,
		&c.RestrictedItemStatus, &c.ComplianceReference, &c.ComplianceDecisionVersion, &c.Version,
		&c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, scanErr(err)
	}
	return c, nil
}

// CreateCargoItem inserts one cargo item.
func (r *Repository) CreateCargoItem(ctx context.Context, q database.Querier, c models.CargoItem) (models.CargoItem, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_cargo_items
			(id, organization_id, shipment_id, description, quantity, unit, handling_category,
			 hazardous_classification, temperature_min, temperature_max, temperature_unit,
			 declared_value_amount_minor, declared_value_currency, restricted_item_status,
			 compliance_reference, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
		RETURNING `+cargoColumns,
		c.ID, c.OrganizationID, c.ShipmentID, c.Description, c.Quantity, c.Unit, c.HandlingCategory,
		c.HazardousClassification, c.TemperatureMin, c.TemperatureMax, c.TemperatureUnit,
		c.DeclaredValueAmountMinor, c.DeclaredValueCurrency, c.RestrictedItemStatus,
		c.ComplianceReference, c.CreatedBy)
	created, err := scanCargoItem(row)
	if err != nil {
		return created, classify(err, "create cargo item")
	}
	return created, nil
}

// ListCargoItems returns a shipment's cargo items, oldest first.
func (r *Repository) ListCargoItems(ctx context.Context, q database.Querier, organizationID, shipmentID string) ([]models.CargoItem, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+cargoColumns+`
		FROM fleet_partner_cargo_items WHERE organization_id = $1 AND shipment_id = $2
		ORDER BY created_at ASC`, organizationID, shipmentID)
	if err != nil {
		return nil, fmt.Errorf("shipments: list cargo items: %w", err)
	}
	defer rows.Close()

	var items []models.CargoItem
	for rows.Next() {
		item, err := scanCargoItem(rows)
		if err != nil {
			return nil, fmt.Errorf("shipments: list cargo items scan: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateCargoItem is a version-checked update of a cargo item's mutable
// fields, callable only while its shipment remains editable.
func (r *Repository) UpdateCargoItem(ctx context.Context, q database.Querier, organizationID, shipmentID, cargoID string,
	expectedVersion int64, c models.CargoItem) (models.CargoItem, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_cargo_items SET
			description = $1, quantity = $2::numeric, unit = $3, handling_category = $4,
			hazardous_classification = $5, temperature_min = $6, temperature_max = $7,
			temperature_unit = $8, declared_value_amount_minor = $9, declared_value_currency = $10,
			restricted_item_status = $11, compliance_reference = $12, updated_by = $13,
			version = version + 1, updated_at = now()
		WHERE organization_id = $14 AND shipment_id = $15 AND id = $16 AND version = $17
		RETURNING `+cargoColumns,
		c.Description, c.Quantity, c.Unit, c.HandlingCategory, c.HazardousClassification,
		c.TemperatureMin, c.TemperatureMax, c.TemperatureUnit, c.DeclaredValueAmountMinor,
		c.DeclaredValueCurrency, c.RestrictedItemStatus, c.ComplianceReference, c.UpdatedBy,
		organizationID, shipmentID, cargoID, expectedVersion)
	updated, err := scanCargoItem(row)
	if err != nil {
		if errors.Is(scanErr(err), ErrNotFound) {
			return updated, ErrVersionConflict
		}
		return updated, classify(err, "update cargo item")
	}
	return updated, nil
}
