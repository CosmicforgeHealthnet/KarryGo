package marketrepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

const zoneColumns = `id::text, market_id::text, name, code, center_lat, center_lng,
	          supported_classes, match_radius_km, is_airport_zone, compliance_profile,
	          version, status, created_at, updated_at`

// MarketRepository resolves markets and operating zones. Zone resolution is a
// simple "nearest active zone whose coverage circle contains the point"
// query — see Zone's doc comment for why a circle rather than a polygon.
type MarketRepository interface {
	GetByID(ctx context.Context, id string) (marketmodels.Market, error)
	GetZoneByID(ctx context.Context, id string) (marketmodels.Zone, error)
	// ResolveZone finds the active zone containing point, nearest-center-first
	// when more than one zone's coverage circle overlaps it. Returns
	// apperrors.NotFound when no zone covers the point at all — callers must
	// treat that as "we do not operate here yet", not retry or default.
	ResolveZone(ctx context.Context, point geo.Point) (marketmodels.Zone, error)
}

type PostgresMarketRepository struct {
	db *pgxpool.Pool
}

func NewPostgresMarketRepository(db *pgxpool.Pool) *PostgresMarketRepository {
	return &PostgresMarketRepository{db: db}
}

func (r *PostgresMarketRepository) GetByID(ctx context.Context, id string) (marketmodels.Market, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, country, name, code, status, created_at, updated_at
		FROM markets WHERE id = $1
	`, id)

	var m marketmodels.Market
	err := row.Scan(&m.ID, &m.Country, &m.Name, &m.Code, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return marketmodels.Market{}, apperrors.NotFound("Market could not be found.", err)
	}
	return m, err
}

func (r *PostgresMarketRepository) GetZoneByID(ctx context.Context, id string) (marketmodels.Zone, error) {
	row := r.db.QueryRow(ctx, `SELECT `+zoneColumns+` FROM operating_zones WHERE id = $1`, id)
	z, err := scanZone(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return marketmodels.Zone{}, apperrors.NotFound("Operating zone could not be found.", err)
	}
	return z, err
}

// ResolveZone computes distance in SQL (the haversine formula inlined, not a
// PostGIS extension) so "nearest first, coverage-circle-contains-point" is one
// round trip rather than fetching every active zone and filtering in Go —
// this table stays small (a handful of zones per market for the platform's
// current scale), but there is no reason to pay an N+1 pattern for it anyway.
func (r *PostgresMarketRepository) ResolveZone(ctx context.Context, point geo.Point) (marketmodels.Zone, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+zoneColumns+`
		FROM operating_zones
		WHERE status = 'active'
		  AND (
		    6371 * acos(
		      LEAST(1.0, GREATEST(-1.0,
		        cos(radians($1)) * cos(radians(center_lat)) * cos(radians(center_lng) - radians($2))
		        + sin(radians($1)) * sin(radians(center_lat))
		      ))
		    )
		  ) <= match_radius_km
		ORDER BY (
		  6371 * acos(
		    LEAST(1.0, GREATEST(-1.0,
		      cos(radians($1)) * cos(radians(center_lat)) * cos(radians(center_lng) - radians($2))
		      + sin(radians($1)) * sin(radians(center_lat))
		    ))
		  )
		) ASC
		LIMIT 1
	`, point.Lat, point.Lng)

	z, err := scanZone(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return marketmodels.Zone{}, apperrors.NotFound("We do not operate in this area yet.", err)
	}
	return z, err
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanZone(row scannable) (marketmodels.Zone, error) {
	var z marketmodels.Zone
	err := row.Scan(
		&z.ID, &z.MarketID, &z.Name, &z.Code, &z.CenterLat, &z.CenterLng,
		&z.SupportedClasses, &z.RadiusKm, &z.IsAirportZone, &z.ComplianceProfile,
		&z.Version, &z.Status, &z.CreatedAt, &z.UpdatedAt,
	)
	return z, err
}
