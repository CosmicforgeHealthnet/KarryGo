package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"karrygo/backend/internal/api"
	"karrygo/backend/internal/platform/httpx"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())
	api.RegisterRoutes(router, api.Dependencies{})

	return router
}

func TestHealthWorks(t *testing.T) {
	router := newTestRouter()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if body["success"] != true || body["status"] != "ok" {
		t.Fatalf("unexpected health response: %v", body)
	}
}

func TestCustomerMeRequiresAuth(t *testing.T) {
	router := newTestRouter()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customer/me", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.Code)
	}

	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if body.Success {
		t.Fatalf("expected unsuccessful response")
	}

	if body.Error.Code != "unauthorized" {
		t.Fatalf("expected unauthorized error, got %q", body.Error.Code)
	}
}
