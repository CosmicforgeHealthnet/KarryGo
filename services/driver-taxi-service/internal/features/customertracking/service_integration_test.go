package customertracking

import (
	"context"
	br "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bt "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	bu "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/journeyshare/journeysharetest"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestProviderIngestionToN07AndN08(t *testing.T) {
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("isolated Taxi PostgreSQL and Redis required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	id, owner, provider, market, zone := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO markets(id,country,name,code)VALUES($1,'NG','test',$2)`, market, market)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes)VALUES($1,$2,'test',$3,6,3,10,'{economy}')`, zone, market, zone)
	exec(`INSERT INTO taxi_providers(id,phone,first_name)VALUES($1,$2,'Ada')`, provider, provider)
	exec(`INSERT INTO taxi_bookings(id,customer_id,provider_id,market_id,operating_zone_id,status,accepted_at,pricing_quote_reference)VALUES($1,$2,$3,$4,$5,'in_progress',now()-interval '5 minutes','test')`, id, owner, provider, market, zone)
	bookingService := bu.NewBookingService(bu.Options{Bookings: br.NewPostgresBookingRepository(db), Telemetry: bt.NewPostgresRepository(db), TelemetryConfig: bt.Config{MaxSpeedKph: 300, StaleThreshold: time.Minute}})
	now := time.Now().UTC()
	out, err := bookingService.IngestLocationPoints(ctx, provider, []bu.LocationPointInput{{BookingID: id, Sequence: 1, Lat: 6.1, Lng: 3.1, RecordedAt: now}})
	if err != nil || len(out) != 1 || !out[0].Stored {
		t.Fatal(out, err)
	}
	reader := &Postgres{DB: db, FreshFor: 30 * time.Second}
	p, err := reader.Read(ctx, id, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	if p.Pickup != nil || p.Dropoff != nil {
		t.Fatal("unknown route coordinates fabricated")
	}
	exec(`INSERT INTO taxi_booking_stops(id,booking_id,sequence,type,address,lat,lng)VALUES($1,$2,1,'PICKUP','pickup',6.1,3.1),($3,$2,2,'DROPOFF','dropoff',6.2,3.2)`, uuid.NewString(), id, uuid.NewString())
	p, err = reader.Read(ctx, id, owner)
	if err != nil || p.Pickup == nil || p.Pickup.Label != "pickup" || p.Dropoff == nil || p.Dropoff.Label != "dropoff" {
		t.Fatal("canonical route unavailable", err)
	}
	signer := auth.NewTokenSigner([]byte("test"))
	engine := gin.New()
	RegisterCustomer(engine.Group("/api/v1/taxi"), reader, signer, true)
	secret := []byte("test-hmac")
	RegisterInternal(engine, reader, serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret}, 5*time.Minute), rc, true)
	server := httptest.NewServer(engine)
	defer server.Close()
	client := &journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{"TAXI": {BaseURL: server.URL, Secret: secret}}}
	p, err = client.Read(ctx, "TAXI", id, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	if _, err = client.Read(ctx, "TAXI", id, uuid.NewString()); err == nil {
		t.Fatal("N08 IDOR")
	}
	gateEngine := gin.New()
	RegisterInternal(gateEngine, reader, serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret}, 5*time.Minute), rc, false)
	gateServer := httptest.NewServer(gateEngine)
	gateClient := &journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{"TAXI": {BaseURL: gateServer.URL, Secret: secret}}}
	if _, err = gateClient.Read(ctx, "TAXI", id, owner); err == nil {
		t.Fatal("N08 flag ignored")
	}
	gateServer.Close()
	bearer, err := signer.Sign(auth.Claims{Subject: owner, Role: "customer", Service: "customer", SessionID: uuid.NewString(), Type: "access", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/taxi/customer/bookings/"+id+"/location", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var data map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for seq := int64(2); seq < 52; seq++ {
			results, e := bookingService.IngestLocationPoints(ctx, provider, []bu.LocationPointInput{{BookingID: id, Sequence: seq, Lat: 6.1, Lng: 3.1, RecordedAt: time.Now().UTC()}})
			if e != nil || len(results) != 1 || !results[0].Stored {
				t.Errorf("provider write during polling: %v %+v", e, results)
				return
			}
		}
	}()
	journeysharetest.Exercise(t, "N07 bearer polling", 200, func() error {
		req := httptest.NewRequest("GET", "/api/v1/taxi/customer/bookings/"+id+"/location", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		out := httptest.NewRecorder()
		engine.ServeHTTP(out, req)
		if out.Code != 200 {
			return fmt.Errorf("status %d", out.Code)
		}
		return nil
	})
	journeysharetest.Exercise(t, "N08 signed read-through", 200, func() error { _, err := client.Read(ctx, "TAXI", id, owner); return err })
	<-writerDone
	reader.Now = func() time.Time { return now.Add(time.Minute) }
	p, err = reader.Read(ctx, id, owner)
	if err != nil || p.Location.State != "STALE" {
		t.Fatal(p, err)
	}
	replacement := uuid.NewString()
	exec(`INSERT INTO taxi_providers(id,phone)VALUES($1,$2)`, replacement, replacement)
	exec(`UPDATE taxi_bookings SET provider_id=$2,accepted_at=now() WHERE id=$1`, id, replacement)
	p, err = reader.Read(ctx, id, owner)
	if err != nil || p.Location.Available {
		t.Fatal("previous provider leaked", p, err)
	}
	exec(`UPDATE taxi_bookings SET status='completed',completed_at=now() WHERE id=$1`, id)
	p, err = reader.Read(ctx, id, owner)
	if err != nil || !p.Terminal || p.Location.Available {
		t.Fatal("terminal point leaked", p, err)
	}
}
func TestSafeLocationStates(t *testing.T) {
	now := time.Now().UTC()
	lat, lng := 6.0, 3.0
	for _, status := range []string{"completed", "cancelled", "pending_match", "unknown"} {
		p := Safe(journeyshare.Projection{Status: status}, &lat, &lng, nil, nil, &now, now, time.Minute)
		if p.Location.Available {
			t.Fatal(status)
		}
	}
	future := now.Add(time.Minute)
	p := Safe(journeyshare.Projection{Status: "in_progress"}, &lat, &lng, nil, nil, &future, now, time.Minute)
	if p.Location.Available {
		t.Fatal("future location accepted")
	}
}
