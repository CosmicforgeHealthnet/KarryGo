package production

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

func TestProductionHTTPUsesSharedIdempotencyStore(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffA, VendorID: businessA, SessionID: "session-production-idempotency"})
		c.Next()
	})
	RegisterRoutes(group, NewHandler(env.service).UseIdempotency(idempotency.NewStore(env.pool, time.Hour)), allFlagsOn())

	do := func(body, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/vendor/production/pools", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	body := `{"branch_id":"brn_prod_a1","capability":"CLOUD_KITCHEN","name":"Idempotent pool","code":"IDEMPOTENT_POOL","scope_type":"CAPABILITY","unit_type":"WORKLOAD_UNIT","default_capacity":25}`
	first := do(body, "production-pool-command-1")
	if first.Code != http.StatusOK {
		t.Fatalf("first command status=%d body=%s", first.Code, first.Body.String())
	}
	replay := do(body, "production-pool-command-1")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("identical retry was not replayed: first=%d %s retry=%d %s", first.Code, first.Body.String(), replay.Code, replay.Body.String())
	}
	var count int
	if err := env.pool.QueryRow(t.Context(), `SELECT count(*) FROM production_capacity_pools WHERE business_id=$1 AND code='IDEMPOTENT_POOL'`, businessA).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("identical retry created %d pools; want 1", count)
	}

	changed := do(`{"branch_id":"brn_prod_a1","capability":"CLOUD_KITCHEN","name":"Different","code":"DIFFERENT_POOL","scope_type":"CAPABILITY","unit_type":"WORKLOAD_UNIT","default_capacity":25}`, "production-pool-command-1")
	if changed.Code != http.StatusConflict {
		t.Fatalf("same key with another fingerprint status=%d body=%s", changed.Code, changed.Body.String())
	}
	missing := do(body, "")
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status=%d body=%s", missing.Code, missing.Body.String())
	}
}
