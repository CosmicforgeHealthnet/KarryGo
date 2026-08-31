package shop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

func TestShopHTTPUsesSharedIdempotencyAndStrictSnakeCaseCommands(t *testing.T) {
	f := section12Fixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: f.actorA.StaffID, VendorID: f.actorA.VendorID, SessionID: "shop-http-test"})
		c.Next()
	})
	flags := Flags{ShopEnabled: true, WritesEnabled: true}
	RegisterRoutes(group, NewHandler(f.service, idempotency.NewStore(f.pool, time.Hour)), flags)

	do := func(body, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/vendor/shop/categories", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	body := `{"name":"Idempotent Foodstuff","description":"Strict snake_case management command","position":3}`
	first := do(body, "shop-category-command-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("first command status=%d body=%s", first.Code, first.Body.String())
	}
	replay := do(body, "shop-category-command-1")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("retry did not replay: first=%d %s replay=%d %s", first.Code, first.Body.String(), replay.Code, replay.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM shop_categories WHERE business_id=$1 AND name='Idempotent Foodstuff'`, f.actorA.VendorID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry created %d categories; want 1", count)
	}

	changed := do(`{"name":"Different Foodstuff"}`, "shop-category-command-1")
	if changed.Code != http.StatusConflict {
		t.Fatalf("fingerprint conflict status=%d body=%s", changed.Code, changed.Body.String())
	}
	missing := do(body, "")
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing key status=%d body=%s", missing.Code, missing.Body.String())
	}
	unknown := do(`{"name":"Unknown Field","inventory_quantity":10}`, "shop-category-command-2")
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown/inventory field status=%d body=%s", unknown.Code, unknown.Body.String())
	}
}

func TestInventoryHTTPStrictSnakeCaseIdempotencyAndPostedReceiptDefense(t *testing.T) {
	f := section12Fixture(t)
	basis := createInventoryBasis(t, f, "HTTP Receipt", 1)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: f.actorA.StaffID, VendorID: f.actorA.VendorID, SessionID: "shop-inventory-http-test"})
		c.Next()
	})
	RegisterRoutes(group, NewHandler(f.service, idempotency.NewStore(f.pool, time.Hour)), Flags{ShopEnabled: true, WritesEnabled: true})

	do := func(method, path, body, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	body := `{"branch_id":"` + f.branchShop + `","source_ref":"http-receipt","notes":"strict snake case","lines":[{"stock_basis_id":"` + basis.ID + `","quantity":3,"lot_number":"http-lot"}]}`
	first := do(http.MethodPost, "/api/v1/vendor/shop/inventory/receipts", body, "shop-inventory-receipt-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("create receipt status=%d body=%s", first.Code, first.Body.String())
	}
	replay := do(http.MethodPost, "/api/v1/vendor/shop/inventory/receipts", body, "shop-inventory-receipt-1")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("receipt replay differs: first=%s replay=%s", first.Body.String(), replay.Body.String())
	}
	var envelope struct {
		Data InventoryReceipt `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("receipt response=%s err=%v", first.Body.String(), err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM shop_inventory_receipts WHERE business_id=$1 AND source_ref='http-receipt'`, f.actorA.VendorID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotent receipt count=%d err=%v", count, err)
	}
	changed := do(http.MethodPost, "/api/v1/vendor/shop/inventory/receipts", strings.Replace(body, `"quantity":3`, `"quantity":4`, 1), "shop-inventory-receipt-1")
	if changed.Code != http.StatusConflict {
		t.Fatalf("receipt fingerprint conflict status=%d body=%s", changed.Code, changed.Body.String())
	}
	unknown := do(http.MethodPost, "/api/v1/vendor/shop/inventory/receipts", strings.Replace(body, `"notes":"strict snake case"`, `"notes":"strict snake case","stock_quantity":99`, 1), "shop-inventory-receipt-2")
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown inventory command field status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	postPath := "/api/v1/vendor/shop/inventory/receipts/" + envelope.Data.ID + "/post"
	posted := do(http.MethodPost, postPath, `{}`, "shop-inventory-post-1")
	if posted.Code != http.StatusOK {
		t.Fatalf("post receipt status=%d body=%s", posted.Code, posted.Body.String())
	}
	postedAgain := do(http.MethodPost, postPath, `{}`, "shop-inventory-post-2")
	if postedAgain.Code != http.StatusOK {
		t.Fatalf("idempotent service post status=%d body=%s", postedAgain.Code, postedAgain.Body.String())
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM shop_inventory_movements WHERE business_id=$1 AND source_type='RECEIPT' AND source_ref=$2`, f.actorA.VendorID, envelope.Data.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("posted receipt movement count=%d err=%v", count, err)
	}
}
