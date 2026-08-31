package supporthttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestCommerceCaseInternalCallerAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secrets := serviceauth.Secrets{"vendor-service": []byte("vendor-secret"), "fleet-partner-service": []byte("fleet-secret")}
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	group := router.Group("/api/v1/support-disputes")
	RegisterCommerceCaseRoutes(group, supportusecases.NewSupportService(nil, supportusecases.Options{}), secrets)
	for _, tc := range []struct {
		name, caller string
		secret       []byte
		want         int
	}{{"wrong authenticated service", "fleet-partner-service", []byte("fleet-secret"), http.StatusForbidden}, {"vendor reaches typed handler", "vendor-service", []byte("vendor-secret"), http.StatusUnprocessableEntity}} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/support-disputes/internal/commerce-cases", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if err := serviceauth.SignRequest(req, tc.caller, tc.secret, body, time.Now()); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGenericDisputeResolutionRejectsDirectMoneyInstruction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	handler := NewHandler(nil)
	router.POST("/admin/disputes/:id/resolve", handler.AdminResolveDispute)
	body := []byte(`{"outcome":"favour_complainant","note":"upheld","refund_amount_kobo":5000,"refund_source_reference":"payment-a"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/disputes/dispute-a/resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
