package repositories

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const locationColumns = `id, organization_id, customer_id, location_type, label,
	address_snapshot, latitude, longitude, COALESCE(geofence_reference, ''), operating_hours,
	access_instructions, safety_instructions, COALESCE(contact_reference, ''), timezone,
	status, version, created_by, updated_by, created_at, updated_at, archived_at`

func scanLocation(row pgx.Row) (models.Location, error) {
	var l models.Location
	var address, hours []byte
	err := row.Scan(&l.ID, &l.OrganizationID, &l.CustomerID, &l.LocationType, &l.Label,
		&address, &l.Latitude, &l.Longitude, &l.GeofenceReference, &hours,
		&l.AccessInstructions, &l.SafetyInstructions, &l.ContactReference, &l.Timezone,
		&l.Status, &l.Version, &l.CreatedBy, &l.UpdatedBy, &l.CreatedAt, &l.UpdatedAt,
		&l.ArchivedAt)
	if err != nil {
		return l, scanErr(err)
	}
	if len(address) > 0 {
		_ = json.Unmarshal(address, &l.Address)
	}
	l.OperatingHours = decodeJSONMap(hours)
	return l, nil
}

// CreateLocation inserts a customer location.
func (r *Repository) CreateLocation(ctx context.Context, q database.Querier,
	l models.Location) (models.Location, error) {
	address, err := encodeJSON(l.Address)
	if err != nil {
		return models.Location{}, err
	}
	hours, err := encodeJSON(l.OperatingHours)
	if err != nil {
		return models.Location{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_customer_locations
			(id, organization_id, customer_id, location_type, label, address_snapshot,
			 latitude, longitude, geofence_reference, operating_hours, access_instructions,
			 safety_instructions, contact_reference, timezone, status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
		RETURNING `+locationColumns,
		l.ID, l.OrganizationID, l.CustomerID, l.LocationType, l.Label, address,
		l.Latitude, l.Longitude, nullable(l.GeofenceReference), hours, l.AccessInstructions,
		l.SafetyInstructions, nullable(l.ContactReference), l.Timezone, l.Status, l.CreatedBy)
	created, err := scanLocation(row)
	if err != nil {
		return created, classify(err, "create location")
	}
	return created, nil
}

// GetLocation reads one location scoped to its customer.
func (r *Repository) GetLocation(ctx context.Context, q database.Querier,
	organizationID, customerID, locationID string) (models.Location, error) {
	return scanLocation(r.q(q).QueryRow(ctx, `SELECT `+locationColumns+`
		FROM fleet_partner_customer_locations
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3`,
		organizationID, customerID, locationID))
}

// LockLocation reads one location FOR UPDATE.
func (r *Repository) LockLocation(ctx context.Context, q database.Querier,
	organizationID, customerID, locationID string) (models.Location, error) {
	return scanLocation(r.q(q).QueryRow(ctx, `SELECT `+locationColumns+`
		FROM fleet_partner_customer_locations
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 FOR UPDATE`,
		organizationID, customerID, locationID))
}

// LocationFilter is the allowlisted location list query.
type LocationFilter struct {
	LocationType string
	Status       string
	Cursor       cursor.Cursor
	HasCursor    bool
	Limit        int
}

// ListLocations returns a page of a customer's locations, newest first.
func (r *Repository) ListLocations(ctx context.Context, organizationID, customerID string,
	filter LocationFilter) ([]models.Location, error) {
	args := []any{organizationID, customerID, filter.LocationType, filter.Status}
	where := `organization_id = $1 AND customer_id = $2
		AND ($3 = '' OR location_type = $3) AND ($4 = '' OR status = $4)`
	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+locationColumns+`
		FROM fleet_partner_customer_locations WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list locations")
	}
	defer rows.Close()
	out := make([]models.Location, 0)
	for rows.Next() {
		value, scanError := scanLocation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// LocationUpdate carries the mutable location fields.
type LocationUpdate struct {
	LocationType       *string
	Label              *string
	Address            *models.Address
	Latitude           *float64
	Longitude          *float64
	ClearCoordinates   bool
	GeofenceReference  *string
	OperatingHours     models.JSONMap
	AccessInstructions *string
	SafetyInstructions *string
	ContactReference   *string
	Timezone           *string
	Status             *string
	UpdatedBy          string
}

// UpdateLocation applies a compare-and-swap update.
func (r *Repository) UpdateLocation(ctx context.Context, q database.Querier,
	organizationID, customerID, locationID string, expectedVersion int64,
	update LocationUpdate) (models.Location, error) {
	var address, hours any
	if update.Address != nil {
		encoded, err := encodeJSON(*update.Address)
		if err != nil {
			return models.Location{}, err
		}
		address = encoded
	}
	if update.OperatingHours != nil {
		encoded, err := encodeJSON(update.OperatingHours)
		if err != nil {
			return models.Location{}, err
		}
		hours = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_locations SET
			location_type = COALESCE($5, location_type),
			label = COALESCE($6, label),
			address_snapshot = COALESCE($7::jsonb, address_snapshot),
			latitude = CASE WHEN $8 THEN NULL ELSE COALESCE($9, latitude) END,
			longitude = CASE WHEN $8 THEN NULL ELSE COALESCE($10, longitude) END,
			geofence_reference = COALESCE($11, geofence_reference),
			operating_hours = COALESCE($12::jsonb, operating_hours),
			access_instructions = COALESCE($13, access_instructions),
			safety_instructions = COALESCE($14, safety_instructions),
			contact_reference = COALESCE($15, contact_reference),
			timezone = COALESCE($16, timezone),
			status = COALESCE($17, status),
			updated_by = $18, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 AND version = $4
		  AND archived_at IS NULL
		RETURNING `+locationColumns,
		organizationID, customerID, locationID, expectedVersion,
		update.LocationType, update.Label, address, update.ClearCoordinates,
		update.Latitude, update.Longitude, update.GeofenceReference, hours,
		update.AccessInstructions, update.SafetyInstructions, update.ContactReference,
		update.Timezone, update.Status, update.UpdatedBy)
	updated, err := scanLocation(row)
	if err != nil {
		return updated, r.classifyLocationWrite(ctx, q, organizationID, customerID, locationID,
			err, "update location")
	}
	return updated, nil
}

// ArchiveLocation closes a location.
//
// The row is not deleted. Issued quotation versions and activated contract
// versions snapshot what they needed at the time, and a historical record that
// pointed at a deleted row would be a record with a hole in it.
func (r *Repository) ArchiveLocation(ctx context.Context, q database.Querier,
	organizationID, customerID, locationID string, expectedVersion int64,
	actor string) (models.Location, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_locations SET
			status = 'ARCHIVED', archived_at = now(), updated_by = $5,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 AND version = $4
		  AND archived_at IS NULL
		RETURNING `+locationColumns,
		organizationID, customerID, locationID, expectedVersion, actor)
	updated, err := scanLocation(row)
	if err != nil {
		return updated, r.classifyLocationWrite(ctx, q, organizationID, customerID, locationID,
			err, "archive location")
	}
	return updated, nil
}

func (r *Repository) classifyLocationWrite(ctx context.Context, q database.Querier,
	organizationID, customerID, locationID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetLocation(ctx, q, organizationID, customerID, locationID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendLocationHistory writes one immutable location change record.
func (r *Repository) AppendLocationHistory(ctx context.Context, q database.Querier,
	entry models.LocationHistory) error {
	fields, err := encodeJSONArray(entry.ChangedFields)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err = r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_crm_location_history
			(id, organization_id, customer_id, location_id, change_type, previous_status,
			 new_status, changed_fields, reason_code, actor_type, actor_id, correlation_id,
			 causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entry.ID, entry.OrganizationID, entry.CustomerID, entry.LocationID, entry.ChangeType,
		entry.PreviousStatus, entry.NewStatus, fields, entry.ReasonCode, entry.ActorType,
		entry.ActorID, entry.CorrelationID, entry.CausationID)
	return classify(err, "append location history")
}

// CountLocations reports the total location count for metrics.
func (r *Repository) CountLocations(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_customer_locations
		WHERE archived_at IS NULL`).Scan(&total)
	return total, classify(err, "count locations")
}
