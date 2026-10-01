package customershare

import (
	"context"
	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	"cosmicforge/logistics/services/hauling-service/internal/features/movement"
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
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("isolated hauling database and Redis required")
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
	exec("INSERT INTO truck_providers(id,phone,first_name)VALUES($1,$2,'Ada')", provider, provider)
	exec("INSERT INTO haulage_bookings(id,customer_id,provider_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status,accepted_at)VALUES($1,$2,$3,'pickup',6,3,'dropoff',7,4,'general',10,'picked_up',now())", ref, owner, provider)
	publisher := movement.NewPublisher(db)
	if _, err = publisher.Sweep(ctx); err != nil {
		t.Fatal("publisher sweep", err)
	}
	var version int64
	if err = db.QueryRow(ctx, `SELECT max(source_version) FROM customer_movement_outbox WHERE source_ref=$1`, ref).Scan(&version); err != nil || version < 1 {
		t.Fatal("durable publication", version, err)
	}
	s := &Service{DB: db, Points: journeyshare.BoundPointStore{Client: rc, Namespace: "HAULING"}}
	p, err := s.Read(ctx, ref, owner)
	if err != nil || p.Location.Available {
		t.Fatal(p, err)
	}
	s.Record(ctx, provider, 6.2, 3.2, 10, 5)
	p, err = s.Read(ctx, ref, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	secret := []byte("integration-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret, "wrong": secret}, 5*time.Minute)
	engine := gin.New()
	journeyshare.RegisterRead(engine, "/internal/hauling/v1/customer-share/bookings/:sourceRef", s.Read, fleetauthority.BoundaryWithDB(db, verifier, []string{"customer-service"})...)
	ts := httptest.NewServer(engine)
	defer ts.Close()
	client := journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{"HAULING": {BaseURL: ts.URL, Secret: secret}}}
	p, err = client.Read(ctx, "HAULING", ref, owner)
	if err != nil || p.Location.State != "FRESH" {
		t.Fatal(p, err)
	}
	if _, err = client.Read(ctx, "HAULING", ref, uuid.NewString()); err == nil {
		t.Fatal("IDOR accepted")
	}
	raw, _ := json.Marshal(p.Public([]string{"route", "provider", "location"}))
	for _, secret := range []string{owner, provider, "SECRET RECEIVER", "SECRET PHONE", "pickup_code", "delivery_code"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private field leaked", secret)
		}
	}
	replacement := uuid.NewString()
	exec("INSERT INTO truck_providers(id,phone)VALUES($1,$2)", replacement, replacement)
	exec("UPDATE haulage_bookings SET provider_id=$2 WHERE id=$1", ref, replacement)
	p, err = s.Read(ctx, ref, owner)
	if err != nil || p.Location.Available {
		t.Fatal("old provider leaked", p, err)
	}
	exec("UPDATE haulage_bookings SET status='completed',completed_at=now() WHERE id=$1", ref)
	p, err = s.Read(ctx, ref, owner)
	if err != nil || !p.Terminal || p.Location.Available {
		t.Fatal("terminal leaked", p, err)
	}
}
