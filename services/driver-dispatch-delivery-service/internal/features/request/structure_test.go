package request

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

func TestRequestFeatureFilesExist(t *testing.T) {
	for _, name := range []string{"handler.go", "service.go", "repository.go", "model.go", "tasks.go", "workers.go", "subscribers.go"} {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("request feature file %s is missing: %v", name, err)
			}
		})
	}
}

func TestRequestRedisKeys(t *testing.T) {
	id := "11111111-1111-1111-1111-111111111111"
	if RequestLockKey(id) != "request:lock:"+id {
		t.Fatal("request lock key mismatch")
	}
	if RequestAcceptedKey(id) != "request:accepted:"+id {
		t.Fatal("request accepted key mismatch")
	}
	if RequestBroadcastingKey(id) != "request:broadcasting:"+id {
		t.Fatal("request broadcasting key mismatch")
	}
}

func TestProviderRequestRoutesRequireJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	tokens := authusecases.NewTokenUsecase([]byte("request-test-secret"), time.Hour, time.Hour)
	RegisterRoutes(engine, tokens, NewHandler(NewService(newFakeRepository(), nil, nil, nil, nil, nil, Config{})))

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/provider/requests"},
		{http.MethodGet, "/api/v1/provider/requests/11111111-1111-1111-1111-111111111111"},
		{http.MethodPost, "/api/v1/provider/requests/11111111-1111-1111-1111-111111111111/accept"},
		{http.MethodPost, "/api/v1/provider/requests/11111111-1111-1111-1111-111111111111/reject"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d want 401; body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
