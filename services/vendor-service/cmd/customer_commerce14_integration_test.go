package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestCustomerCommerce14DiscoveryPreviewDBAndLoad(t *testing.T) {
	env := newSection17BEnv(t, 15*time.Minute)
	ctx := context.Background()
	foodItem := env.seedFoodItem("Public meal", 1000)
	item := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Public goods", PriceMinor: 2000, Quantity: 100})
	// The reused checkout fixture seeds generic Inventory only. Production's
	// Shop inventory bridge also maintains the Shop discovery projection.
	var basis string
	if err := env.pool.QueryRow(ctx, `SELECT id FROM shop_inventory_stock_bases WHERE listing_id=$1`, item.ListingID).Scan(&basis); err != nil {
		t.Fatal(err)
	}
	actor := env.shopActor(env.businessA, env.staffA)
	receipt, err := env.shopService.CreateReceipt(ctx, actor, shop.Scope{Global: true}, shop.CreateReceiptInput{BranchID: env.branchA1, SourceRef: "public-discovery-" + env.suffix, ReceivedAt: time.Now().UTC(), Lines: []shop.ReceiptLineInput{{StockBasisID: basis, Quantity: 100, LotNumber: "public-stock"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.shopService.PostReceipt(ctx, actor, shop.Scope{Global: true}, receipt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE shop_variants SET net_weight_grams=1000,stackable=true WHERE id=$1`, item.VariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO vendor_branch_service_areas(branch_id,business_id,mode,center_latitude,center_longitude,radius_meters) VALUES($1,$2,'RADIUS',6.5244,3.3792,15000)`, env.branchA1, env.businessA); err != nil {
		t.Fatal(err)
	}
	customer := env.customer("preview")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})
	planner, err := fulfilment.NewPlanner(fulfilment.Options{Logistics: newSection17CLogisticsAuthority(env.pool), Branches: newSection17CBranchAuthority(env.pool), Dispatch: fixedQuoteAuthority{feeMinor: 333}, Taxi: fixedQuoteAuthority{feeMinor: 444}, Hauling: fixedQuoteAuthority{feeMinor: 555}})
	if err != nil {
		t.Fatal(err)
	}
	planning, err := fulfilment.NewService(fulfilment.ServiceOptions{Store: fulfilment.NewStore(env.pool), Planner: planner, Checkout: newSection17CCheckoutAuthority(env.pool)})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{CustomerAccessTokenSecret: []byte("customer14-integration-secret")}
	cfg.Flags.CustomerFoodEnabled = true
	cfg.Flags.CommerceFoodEnabled = true
	cfg.Flags.CustomerShopEnabled = true
	cfg.Flags.CommerceShopEnabled = true
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	redisAddress := os.Getenv("CUSTOMER14_TEST_REDIS_ADDR")
	if redisAddress == "" {
		redisAddress = "127.0.0.1:56379"
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddress})
	defer client.Close()
	if err = client.Ping(ctx).Err(); err != nil {
		t.Fatalf("local Redis required: %v", err)
	}
	limiter := ratelimit.New(client, "customer14-test:"+env.suffix)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	registerCustomerDiscoveryRoutes(router, cfg, env.foodService, env.shopService, section13{}, limiter)
	registerCartDeliveryPreview(router, cfg, env.carts, newSection17BFoodAuthority(env.pool, env.foodService), newSection17BShopAuthority(env.pool, env.shopService, env.stock), planning, limiter)
	server := httptest.NewServer(router)
	defer server.Close()
	token := discoveryToken(t, cfg.CustomerAccessTokenSecret, customer, "customer", "customer", "access")
	get := func(path string) (int, string) {
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	for _, path := range []string{"/api/v1/commerce/food/branches/" + env.foodBranch + "/catalogue?capability=RESTAURANT", "/api/v1/commerce/food/branches/" + env.foodBranch + "/items/" + foodItem.ItemID + "/sellability", "/api/v1/commerce/shop/branches/" + env.branchA1 + "/catalogue", "/api/v1/commerce/shop/branches/" + env.branchA1 + "/products/" + item.ProductID} {
		status, body := get(path)
		if status != 200 {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		for _, forbidden := range []string{"physical_on_hand", "physical_available", "reserved_quantity", "quarantined_quantity", "matched_schedule_ref", "operational_status", "branch_override_ref", "archived_at", "source_evidence"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("unsafe DTO: %s", forbidden)
			}
		}
	}
	previewPath := "/api/v1/commerce/carts/" + current.ID + "/delivery-preview"
	status, body := get(previewPath)
	if status != 200 {
		t.Fatalf("preview %d: %s", status, body)
	}
	var response struct {
		Data struct {
			Serviceable bool   `json:"serviceable"`
			Fee         int64  `json:"delivery_customer_minor"`
			Fingerprint string `json:"fingerprint"`
			Groups      []struct {
				Class string `json:"vehicle_class"`
			} `json:"groups"`
		}
	}
	if err = json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Data.Serviceable || response.Data.Fee != 333 {
		t.Fatalf("real planner must consume authority fee: %s", body)
	}
	first := response.Data.Fingerprint
	current, err = env.carts.UpdateQuantity(ctx, customer, current.ID, current.Items[0].ID, 30, current.Version)
	if err != nil {
		t.Fatal(err)
	}
	status, body = get(previewPath)
	if status != 200 {
		t.Fatalf("quantity preview %d %s", status, body)
	}
	if err = json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Fingerprint == first || len(response.Data.Groups) != 1 || response.Data.Groups[0].Class == "MOTORCYCLE" {
		t.Fatalf("must escalate quantity: %s", body)
	}
	var sessions, plans int
	if err = env.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM commerce_checkout_sessions WHERE customer_id=$1),(SELECT count(*) FROM commerce_fulfilment_plans WHERE customer_id=$1)`, customer).Scan(&sessions, &plans); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || plans != 0 {
		t.Fatal("preview persisted checkout/plan")
	}
	if stock := env.availableStock(env.businessA, env.branchA1, item.InventoryItemID); stock != 100 {
		t.Fatalf("preview changed stock %d", stock)
	}
	// Exercise the new catalogue hot path through live HTTP, real PostgreSQL and
	// real Redis. Excess reads must be 429, not SQL/transport errors.
	start := time.Now()
	var wg sync.WaitGroup
	results := make(chan int, 240)
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 30; n++ {
				status, _ := get("/api/v1/commerce/shop/branches/" + env.branchA1 + "/catalogue")
				results <- status
			}
		}()
	}
	wg.Wait()
	close(results)
	ok, limited := 0, 0
	for status := range results {
		switch status {
		case 200:
			ok++
		case 429:
			limited++
		default:
			t.Fatalf("load unexpected status: %d", status)
		}
	}
	if ok == 0 || limited == 0 {
		t.Fatal("load did not exercise success and budget")
	}
	t.Log(fmt.Sprintf("live HTTP load: 240 reads, %d OK, %d expected 429, elapsed=%s", ok, limited, time.Since(start)))
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_businesses SET status='SUSPENDED' WHERE id=$1`, env.businessA); err != nil {
		t.Fatal(err)
	}
	other := env.customer("suspended")
	token = discoveryToken(t, cfg.CustomerAccessTokenSecret, other, "customer", "customer", "access")
	status, _ = get("/api/v1/commerce/shop/branches/" + env.branchA1 + "/catalogue")
	if status != 404 {
		t.Fatalf("suspended vendor exposed: %d", status)
	}
}
