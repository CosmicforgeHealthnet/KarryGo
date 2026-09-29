package customershare

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/movement"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAuthorityProviderPointShareIntegration(t *testing.T) {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("isolated dispatch database and Redis required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	ref, owner, provider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO providers(id,phone,full_name)VALUES($1,$2,'Ada Private')", provider, provider)
	bookingID := uuid.NewString()
	exec(`INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)VALUES($1,$2,'accepted','pickup',6,3,'dropoff',7,4,0,'SECRET RECEIVER','SECRET PHONE')`, bookingID, owner)
	exec("INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)VALUES($1,$2,$3,$4,'in_progress','pickup',6,3,'dropoff',7,4,0,'SECRET RECEIVER','SECRET PHONE')", ref, bookingID, provider, owner)
	publisher := movement.NewPublisher(db)
	if _, err = publisher.Sweep(ctx); err != nil {
		t.Fatal("publisher sweep", err)
	}
	var version int64
	if err = db.QueryRow(ctx, `SELECT max(source_version) FROM customer_movement_outbox WHERE source_ref=$1`, bookingID).Scan(&version); err != nil || version < 1 {
		t.Fatal("durable publication", version, err)
	}
	s := &Service{DB: db, Points: journeyshare.BoundPointStore{Client: rc, Namespace: "DISPATCH"}}
	p, err := s.Read(ctx, ref, owner)
	if err != nil || p.Location.Available {
		t.Fatal(p, err)
	}
	s.Record(ctx, provider, availability.Location{ProviderID: provider, Lat: 6.2, Lng: 3.2, UpdatedAt: time.Now().UTC()})
	p, err = s.Read(ctx, ref, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	secret := []byte("integration-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret, "wrong": secret}, 5*time.Minute)
	engine := gin.New()
	journeyshare.RegisterRead(engine, "/internal/dispatch/v1/customer-share/trips/:sourceRef", s.Read, authority.ServiceBoundary(verifier, authority.NewPostgresNonceClaimer(db), []string{"customer-service"})...)
	ts := httptest.NewServer(engine)
	defer ts.Close()
	client := journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{"DISPATCH": {BaseURL: ts.URL, Secret: secret}}}
	p, err = client.Read(ctx, "DISPATCH", ref, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	if _, err = client.Read(ctx, "DISPATCH", ref, uuid.NewString()); err == nil {
		t.Fatal("IDOR accepted")
	}
	raw, _ := json.Marshal(p.Public([]string{"route", "provider", "location"}))
	for _, secret := range []string{owner, provider, "SECRET RECEIVER", "SECRET PHONE", "pickup_code", "delivery_code"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private field leaked", secret)
		}
	}
	replacement := uuid.NewString()
	exec("INSERT INTO providers(id,phone)VALUES($1,$2)", replacement, replacement)
	exec("UPDATE trips SET provider_id=$2 WHERE id=$1", ref, replacement)
	p, err = s.Read(ctx, ref, owner)
	if err != nil || p.Location.Available {
		t.Fatal("old provider leaked", p, err)
	}
	exec("UPDATE trips SET status='completed',completed_at=now() WHERE id=$1", ref)
	p, err = s.Read(ctx, ref, owner)
	if err != nil || !p.Terminal || p.Location.Available {
		t.Fatal("terminal leaked", p, err)
	}
}
