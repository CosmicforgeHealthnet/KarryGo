package customershare

import (
	"context"
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

type logisticsFixture struct{}

func (logisticsFixture) Read(_ context.Context, authority, ref, owner string) (journeyshare.Projection, error) {
	return journeyshare.Projection{Authority: authority, SourceRef: ref, CustomerID: owner, ProductCode: "SEND", SourceVersion: 2, Eligible: true, PublicStatus: "IN_PROGRESS", Location: journeyshare.Location{State: "UNAVAILABLE"}}, nil
}
func TestPersistentFoodShopMissionScopeAndSignedBoundary(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("requires isolated Vendor PostgreSQL and Redis")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	secret := []byte("vendor-share-test")
	s := &Service{DB: db, Enabled: true, Logistics: logisticsFixture{}, EnabledAuthorities: map[string]bool{"DISPATCH": true}}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	journeyshare.RegisterRead(engine, "/internal/vendor/v1/customer-share/missions/:sourceRef", s.Read, journeyshare.RedisBoundary(serviceauth.NewVerifier(serviceauth.Secrets{"customer-service": secret}, 5*time.Minute), rc, "customer-service")...)
	server := httptest.NewServer(engine)
	defer server.Close()
	client := &journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{"VENDOR_COMMERCE": {BaseURL: server.URL, Secret: secret}}}
	for _, product := range []string{"FOOD", "SHOP"} {
		for _, executor := range []string{"DISPATCH", "HAULING", "TAXI"} {
			authority := executor
			if authority == "TAXI" {
				authority = "TAXI_GOODS"
			}
			s.EnabledAuthorities[authority] = true
			parent := seedShareParent(t, db, product)
			var owner string
			if err = db.QueryRow(ctx, `SELECT customer_id FROM commerce_parent_orders WHERE id=$1`, parent).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			ref := uuid.NewString()
			_, err = db.Exec(ctx, `INSERT INTO commerce_order_missions(id,mission_ref,parent_order_id,customer_id,plan_mission_ref,service,vehicle_class,agreed_fee_minor,currency,status,external_ref) VALUES($1,$1,$2,$3,$1,$4,'MOTORCYCLE',0,'NGN','ACCEPTED',$1)`, ref, parent, owner, executor)
			if err != nil {
				t.Fatal(err)
			}
			p, err := client.Read(ctx, "VENDOR_COMMERCE", ref, owner)
			if err != nil || p.ProductCode != product {
				t.Fatal(product, p, err)
			}
			raw, _ := json.Marshal(p.Public([]string{"route", "provider", "location", "eta"}))
			for _, private := range []string{owner, parent, ref, "price", "payment", "otp", "qr"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("mission leak", private)
				}
			}
			if _, err = client.Read(ctx, "VENDOR_COMMERCE", ref, "foreign-owner"); err == nil {
				t.Fatal("IDOR")
			}
			s.EnabledAuthorities[authority] = false
			if _, err = s.Read(ctx, ref, owner); err == nil {
				t.Fatal("authority flag ignored")
			}
			s.EnabledAuthorities[authority] = true
		}
	}
}
func seedShareParent(t *testing.T, pool *pgxpool.Pool, product string) string {
	t.Helper()
	ctx := context.Background()
	id := "po_" + uuid.NewString()
	customerID := "customer_" + uuid.NewString()
	suffix := uuid.NewString()
	cartID := "cart_eta_" + suffix
	checkoutID := "checkout_eta_" + suffix
	planID := "plan_eta_" + suffix
	quoteID := "quote_eta_" + suffix
	fingerprint := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at,created_at,updated_at)
		  VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cartID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_checkout_sessions(
			id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,
			currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'eta-fixture-address',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkoutID, customerID, cartID, now, expiresAt}},
		{`INSERT INTO commerce_fulfilment_plans(
			id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,
			delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,
			expires_at,version,planned_at,created_at,updated_at)
		  VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{planID, checkoutID, customerID, expiresAt, now}},
		{`INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,
			discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot,created_at)
		  VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,
			0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quoteID, customerID, checkoutID, cartID, planID, fingerprint, expiresAt, now}},
		{`INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,
			cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,
			selected_fulfilment_mode,address_snapshot,customer_approved_total_minor,max_authorized_total_minor,
			product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,
			subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at)
		  VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,
			0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`, []any{id, customerID, quoteID, fingerprint, checkoutID, cartID, planID, now}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, strings.ReplaceAll(statement.query, "'FOOD'", "'"+product+"'"), statement.args...); err != nil {
			t.Fatalf("seed ETA commerce evidence: %v", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit ETA commerce evidence: %v", err)
	}
	return id
}
