package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
)

func TestHealthAndReadySuccessUseStandardResponseShape(t *testing.T) {
	router := buildResponseAuditRouter(func(context.Context) error { return nil })

	cases := []struct {
		path       string
		wantStatus string
	}{
		{path: "/health", wantStatus: "ok"},
		{path: "/ready", wantStatus: "ready"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp["success"] != true {
				t.Fatalf("success = %v, want true", resp["success"])
			}
			if _, ok := resp["error"]; ok {
				t.Fatalf("success response must not include error: %s", w.Body.String())
			}
			data, _ := resp["data"].(map[string]any)
			if data == nil {
				t.Fatal("data is missing")
			}
			if data["service"] != "driver-dispatch-delivery-service" {
				t.Fatalf("service = %v, want driver-dispatch-delivery-service", data["service"])
			}
			if data["status"] != tc.wantStatus {
				t.Fatalf("status = %v, want %s", data["status"], tc.wantStatus)
			}
		})
	}
}

func TestReadyErrorUsesStandardResponseShapeWithRequestID(t *testing.T) {
	router := buildResponseAuditRouter(func(context.Context) error {
		return apperrors.Unavailable("PostgreSQL is not ready.", errors.New("db down"))
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req.Header.Set("X-Request-ID", "ready-req-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["success"] != false {
		t.Fatalf("success = %v, want false", resp["success"])
	}
	errorObj, _ := resp["error"].(map[string]any)
	if errorObj == nil {
		t.Fatal("error object is missing")
	}
	if errorObj["code"] != string(apperrors.CodeUnavailable) {
		t.Fatalf("code = %v, want service_unavailable", errorObj["code"])
	}
	if errorObj["request_id"] != "ready-req-123" {
		t.Fatalf("request_id = %v, want ready-req-123", errorObj["request_id"])
	}
	if _, ok := errorObj["fields"]; ok {
		t.Fatalf("non-validation error must not include fields: %v", errorObj["fields"])
	}
}

func TestInternalErrorUsesStandardResponseShapeWithRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	r.GET("/boom", func(c *gin.Context) {
		httpx.Abort(c, apperrors.Internal("Something went wrong. Please try again later.", errors.New("boom")))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set("X-Request-ID", "internal-req-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errorObj, _ := resp["error"].(map[string]any)
	if errorObj == nil {
		t.Fatal("error object is missing")
	}
	if errorObj["code"] != string(apperrors.CodeInternal) {
		t.Fatalf("code = %v, want internal_error", errorObj["code"])
	}
	if errorObj["request_id"] != "internal-req-123" {
		t.Fatalf("request_id = %v, want internal-req-123", errorObj["request_id"])
	}
}

func buildResponseAuditRouter(ready func(context.Context) error) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	registerHealthRoutes(r, config.Config{ServiceName: "driver-dispatch-delivery-service"}, ready)
	return r
}
