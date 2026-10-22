package bookinghttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/idempotency"
	"cosmicforge/logistics/services/taxi-service/internal/pricing"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
	"cosmicforge/logistics/shared/go/geo"
)

type zoneLookup struct{ zone marketmodels.Zone }

func (z zoneLookup) ResolveZone(context.Context, geo.Point) (marketmodels.Zone, error) {
	return z.zone, nil
}

type flatPricing struct{}

func (flatPricing) Quote(_ context.Context, req pricing.QuoteRequest) ([]pricing.Quote, error) {
	out := []pricing.Quote{}
	for _, class := range req.VehicleClasses {
		out = append(out, pricing.Quote{VehicleClass: class, FareKobo: 150000, DistanceKm: 5, DurationMin: 12,
			QuoteReference: "quote-" + class, RateVersion: "test", ExpiresAt: time.Now().Add(time.Minute)})
	}
	return out, nil
}

func (flatPricing) Reconcile(context.Context, pricing.ReconcileRequest) (pricing.FinalFare, error) {
	return pricing.FinalFare{}, nil
}

// TestPostgresCreateBookingRouteEnforcesStanding drives the real customer
// route (bearer auth + idempotency + handler + usecase + repository): the
// customer id comes only from the verified token, a hold is a stable 403
// customer_booking_hold, an outage a 503, and neither writes a booking.
func TestPostgresCreateBookingRouteEnforcesStanding(t *testing.T) {
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	marketID, zoneID, code := uuid.NewString(), uuid.NewString(), "STH-"+uuid.NewString()[:8]
	if _, err := db.Exec(ctx, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Standing HTTP', $2)`, marketID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Standing HTTP', $3, 1.0, -10.0, 15, '{economy}')`, zoneID, marketID, code); err != nil {
		t.Fatal(err)
	}
	held, other := uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`,
			`DELETE FROM booking_events WHERE booking_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`,
			`DELETE FROM taxi_booking_stops WHERE booking_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`,
			`DELETE FROM taxi_bookings WHERE market_id = $1`,
			`DELETE FROM operating_zones WHERE market_id = $1`,
			`DELETE FROM markets WHERE id = $1`,
		} {
			_, _ = db.Exec(ctx, sql, marketID)
		}
		_, _ = db.Exec(ctx, `DELETE FROM taxi_idempotency_keys WHERE scope_id IN ($1, $2)`, held, other)
	})

	srv := standingtest.New(t, "driver-taxi-service")
	srv.Hold(held, "BOOKING_HOLD", 4)
	svc := bookingusecases.NewBookingService(bookingusecases.Options{
		Bookings: bookingrepositories.NewPostgresBookingRepository(db),
		Markets: zoneLookup{zone: marketmodels.Zone{ID: zoneID, MarketID: marketID, Name: "z", Code: code, CenterLat: 1, CenterLng: -10,
			RadiusKm: 15, SupportedClasses: []string{"economy"}, Status: "active"}},
		Pricing: flatPricing{}, OperationalState: availabilityrepositories.NewPostgresOperationalStateRepository(db),
		CustomerStanding: srv.Gate(500 * time.Millisecond),
	})
	customerSigner := sharedauth.NewTokenSigner([]byte("customer-test-secret-customer-test-secret"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterBookingRoutes(router.Group("/api/v1/taxi"), svc, sharedauth.NewTokenSigner([]byte("provider-secret")), customerSigner,
		idempotency.NewStore(db, time.Hour), nil)

	post := func(customer string) (int, map[string]any) {
		t.Helper()
		token, err := customerSigner.Sign(sharedauth.Claims{Subject: customer, Role: "customer", Service: "customer",
			SessionID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		// A body-supplied customer id or eligibility is ignored: identity is
		// the verified token subject only.
		body := `{"vehicle_class":"economy","customer_id":"` + other + `","booking_eligible":true,
			"stops":[{"address":"A","lat":1.0005,"lng":-9.9995},{"address":"B","lat":1.02,"lng":-9.98}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/taxi/customer/bookings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	rows := func(customer string) int {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM taxi_bookings WHERE customer_id = $1`, customer).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	errorCode := func(body map[string]any) string {
		e, _ := body["error"].(map[string]any)
		s, _ := e["code"].(string)
		return s
	}

	if status, body := post(held); status != http.StatusForbidden || errorCode(body) != "customer_booking_hold" {
		t.Fatalf("held customer: %d %v", status, body)
	}
	if rows(held) != 0 || rows(other) != 0 {
		t.Fatal("a refused booking wrote rows")
	}
	srv.FailWith(http.StatusBadGateway)
	if status, body := post(held); status != http.StatusServiceUnavailable || errorCode(body) != "customer_standing_unavailable" {
		t.Fatalf("outage: %d %v", status, body)
	}
	srv.FailWith(0)
	srv.Lift(held, 5)
	if status, body := post(held); status != http.StatusCreated {
		t.Fatalf("lifted: %d %v", status, body)
	}
	if rows(held) != 1 || rows(other) != 0 {
		t.Fatalf("booking attributed wrongly: held=%d other=%d", rows(held), rows(other))
	}
}
