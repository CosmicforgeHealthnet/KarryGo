package promotions

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

func TestPromotionWriteRoutesRequireIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: "staff-a", VendorID: "vendor-a", SessionID: "session-a"})
		c.Next()
	})
	store := idempotency.NewStore(nil, time.Hour)
	RegisterRoutes(router.Group("/api/v1/vendor"), NewHandler(NewService(nil, nil, nil, true), store), true)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/vendor/promotions", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing Idempotency-Key status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Idempotency-Key") {
		t.Fatalf("missing-key response does not name the contract requirement: %s", response.Body.String())
	}
}
