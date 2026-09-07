package marketrepositories

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for PostgreSQL market/zone tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedMarketAndZone inserts a throwaway market + one circular zone around
// (centerLat, centerLng) with the given radius, and registers cleanup.
func seedMarketAndZone(t *testing.T, db *pgxpool.Pool, centerLat, centerLng, radiusKm float64) (marketID, zoneID string) {
	t.Helper()
	ctx := context.Background()
	marketID = uuid.NewString()
	zoneID = uuid.NewString()
	code := "TEST-" + uuid.NewString()[:8]

	if _, err := db.Exec(ctx, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`,
		marketID, code); err != nil {
		t.Fatalf("seed market: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, $4, $5, $6, '{economy,comfort,xl}')
	`, zoneID, marketID, code, centerLat, centerLng, radiusKm); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM operating_zones WHERE id = $1`, zoneID)
		_, _ = db.Exec(context.Background(), `DELETE FROM markets WHERE id = $1`, marketID)
	})
	return marketID, zoneID
}

func TestResolveZone_PointInsideRadiusResolves(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresMarketRepository(db)
	marketID, zoneID := seedMarketAndZone(t, db, 70.00, -140.00, 10)

	zone, err := repo.ResolveZone(context.Background(), geo.Point{Lat: 70.00, Lng: -140.00})
	if err != nil {
		t.Fatalf("ResolveZone: %v", err)
	}
	if zone.ID != zoneID || zone.MarketID != marketID {
		t.Fatalf("resolved to the wrong zone: %+v", zone)
	}
}

func TestResolveZone_PointOutsideEveryZoneReturnsNotFound(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresMarketRepository(db)
	seedMarketAndZone(t, db, 70.00, -140.00, 10) // isolated test point, 10km radius

	// Thousands of km from the seeded zone, genuinely outside any 10km zone.
	_, err := repo.ResolveZone(context.Background(), geo.Point{Lat: -40.00, Lng: 100.00})
	if err == nil {
		t.Fatal("a point outside every zone must not resolve")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code != apperrors.CodeNotFound {
		t.Fatalf("expected a NotFound error, got %v", err)
	}
}

func TestResolveZone_PicksNearestWhenZonesOverlap(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresMarketRepository(db)
	ctx := context.Background()

	// Two overlapping zones with different centers; the point sits closer to
	// the second. ResolveZone must pick the nearer one, not the first row
	// Postgres happens to return.
	_, nearZoneID := seedMarketAndZone(t, db, 70.0005, -139.9995, 15) // tens of metres from the query point
	_, farZoneID := seedMarketAndZone(t, db, 70.08, -139.97, 15)      // several km further, but still within 15km

	zone, err := repo.ResolveZone(ctx, geo.Point{Lat: 70.0000, Lng: -140.0000})
	if err != nil {
		t.Fatalf("ResolveZone: %v", err)
	}
	if zone.ID != nearZoneID {
		t.Fatalf("expected the nearer zone %s, got %s (far zone was %s)", nearZoneID, zone.ID, farZoneID)
	}
}

func TestResolveZone_IgnoresInactiveZones(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresMarketRepository(db)
	ctx := context.Background()

	_, zoneID := seedMarketAndZone(t, db, 70.00, -140.00, 10)
	if _, err := db.Exec(ctx, `UPDATE operating_zones SET status = 'inactive' WHERE id = $1`, zoneID); err != nil {
		t.Fatalf("deactivate zone: %v", err)
	}

	_, err := repo.ResolveZone(ctx, geo.Point{Lat: 70.00, Lng: -140.00})
	if err == nil {
		t.Fatal("an inactive zone must not be resolved into")
	}
}
