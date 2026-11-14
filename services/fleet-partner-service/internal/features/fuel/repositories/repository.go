// Package repositories is the PostgreSQL persistence of Section 18 fuel and
// energy records and their approval history. A missing row is pgx.ErrNoRows.
package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

const fuelCols = `id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,operating_hours,receipt_media_id,supplier_reference,recorded_at,approval_status,approved_by,version,created_at,updated_at`

func scanFuel(row pgx.Row) (models.FuelRecord, error) {
	var v models.FuelRecord
	e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.ProviderMembershipID, &v.FuelOrEnergyType, &v.Quantity, &v.Unit, &v.Odometer, &v.OperatingHours, &v.ReceiptMediaID, &v.SupplierReference, &v.RecordedAt, &v.ApprovalStatus, &v.ApprovedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertFuel(ctx context.Context, q database.Querier, v *models.FuelRecord) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_fuel_records(id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,operating_hours,receipt_media_id,supplier_reference,recorded_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING approval_status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.ProviderMembershipID, v.FuelOrEnergyType, v.Quantity, v.Unit, v.Odometer, v.OperatingHours, v.ReceiptMediaID, v.SupplierReference, v.RecordedAt).Scan(&v.ApprovalStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) GetFuel(ctx context.Context, org, id string) (models.FuelRecord, error) {
	return scanFuel(r.db.QueryRow(ctx, `SELECT `+fuelCols+` FROM fleet_partner_fuel_records WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListFuel returns up to limit records newest recordedAt first, keyset on
// (recorded_at, id) after the given position (nil starts at the newest).
func (r *Repository) ListFuel(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.FuelRecord, error) {
	rows, e := r.db.Query(ctx, `SELECT `+fuelCols+` FROM fleet_partner_fuel_records WHERE organization_id=$1 AND($3::timestamptz IS NULL OR(recorded_at,id)<($3::timestamptz,$4)) ORDER BY recorded_at DESC,id DESC LIMIT $2`, org, limit, after, afterID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.FuelRecord{}
	for rows.Next() {
		v, e := scanFuel(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DecideFuel moves a PENDING record to the decided status.
func (r *Repository) DecideFuel(ctx context.Context, q database.Querier, org, id string, expected int64, to, actor string) (models.FuelRecord, error) {
	return scanFuel(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_fuel_records SET approval_status=$4,approved_by=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND approval_status='PENDING' RETURNING `+fuelCols, org, id, expected, to, actor))
}

func (r *Repository) InsertApprovalHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_fuel_approval_history(id,organization_id,fuel_record_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("fah"), org, id, previous, next, actor, reason, version)
	return e
}
