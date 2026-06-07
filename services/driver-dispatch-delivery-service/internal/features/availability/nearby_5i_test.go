package availability

// Phase 5I — GET /api/v1/internal/nearby tests.
// Covers auth, validation, response shape, and count/radius fields.
// GEOSEARCH-based provider filtering is tested via unit tests (miniredis
// does not support GEOSEARCH so full provider-query integration is deferred
// to a live DB/Redis environment).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const testServiceKey = "test-service-key-for-5i"

func newNearbyTestRouter() (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("nearby-test-secret"), time.Hour, time.Hour)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	handler := NewHandlerWithService(fakeAvailabilityService{}, tokens, nil)
	RegisterRoutes(router, tokens, testServiceKey, handler)
	return router, tokens
}

func doNearby(router *gin.Engine, serviceKey, authHeader, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/nearby"+query, nil)
	if serviceKey != "" {
		req.Header.Set("X-Internal-Service-Key", serviceKey)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── auth tests ────────────────────────────────────────────────────────────────

func Test5I_MissingServiceKeyReturns401(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, "", "", "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5I_InvalidServiceKeyReturns401(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, "wrong-key", "", "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5I_JWTWithoutServiceKeyReturns401(t *testing.T) {
	router, tokens := newNearbyTestRouter()
	tok, _, _ := tokens.GenerateAccessToken(uuid.NewString(), "+2348000000000", uuid.NewString())
	// Send Bearer JWT but no service key.
	w := doNearby(router, "", "Bearer "+tok, "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5I_JWTAndServiceKeyTogetherReturns401(t *testing.T) {
	// Service-key middleware rejects Bearer header even when the key is also present.
	router, tokens := newNearbyTestRouter()
	tok, _, _ := tokens.GenerateAccessToken(uuid.NewString(), "+2348000000000", uuid.NewString())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/nearby?lat=6&lng=3", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Internal-Service-Key", testServiceKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5I_ValidServiceKeyReturns200(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusOK)
}

// ── validation tests ──────────────────────────────────────────────────────────

func Test5I_MissingLatReturns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lng=3")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_MissingLngReturns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_InvalidLatReturns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=999&lng=3")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_InvalidLngReturns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=999")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_NegativeRadiusReturns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3&radius=-1")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_RadiusOver50Returns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3&radius=51")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5I_LimitOver50Returns400(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3&limit=51")
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

// ── response shape tests ──────────────────────────────────────────────────────

func Test5I_ResponseHasProvidersCountAndRadiusKM(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusOK)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if _, ok := data["providers"]; !ok {
		t.Fatalf("providers field missing; body = %s", w.Body.String())
	}
	if _, ok := data["count"]; !ok {
		t.Fatalf("count field missing; body = %s", w.Body.String())
	}
	if _, ok := data["radius_km"]; !ok {
		t.Fatalf("radius_km field missing; body = %s", w.Body.String())
	}
}

func Test5I_EmptyNearbyReturns200NotFound(t *testing.T) {
	router, _ := newNearbyTestRouter()
	w := doNearby(router, testServiceKey, "", "?lat=6&lng=3")
	assertHTTPCode(t, w, http.StatusOK) // must be 200, not 404

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	providers, _ := data["providers"].([]any)
	if len(providers) != 0 {
		t.Fatalf("providers = %v, want empty array", providers)
	}
}

// ── default/cap tests via service unit ───────────────────────────────────────

func Test5I_RadiusDefaultsTo5km(t *testing.T) {
	// Test via service.normalizeNearbyRequest directly.
	req := NearbyProvidersRequest{Latitude: 6, Longitude: 3, RadiusKM: 0, Limit: 0}
	normalizeNearbyRequest(&req)
	if req.RadiusKM != 5.0 {
		t.Fatalf("default radius = %.1f, want 5.0", req.RadiusKM)
	}
}

func Test5I_LimitDefaultsTo20(t *testing.T) {
	req := NearbyProvidersRequest{Latitude: 6, Longitude: 3, RadiusKM: 0, Limit: 0}
	normalizeNearbyRequest(&req)
	if req.Limit != 20 {
		t.Fatalf("default limit = %d, want 20", req.Limit)
	}
}

func Test5I_RadiusExactly50IsAccepted(t *testing.T) {
	req := NearbyProvidersRequest{Latitude: 6, Longitude: 3, RadiusKM: 50, Limit: 20}
	if err := validateNearbyInput(req); err != nil {
		t.Fatalf("radius=50 should be accepted: %v", err)
	}
}

func Test5I_LimitExactly50IsAccepted(t *testing.T) {
	req := NearbyProvidersRequest{Latitude: 6, Longitude: 3, RadiusKM: 5, Limit: 50}
	if err := validateNearbyInput(req); err != nil {
		t.Fatalf("limit=50 should be accepted: %v", err)
	}
}

// ── provider filtering unit tests (no live Redis/GEOSEARCH needed) ────────────

func Test5I_ServiceReturnsNearbyWithCorrectCount(t *testing.T) {
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	live.nearby = []NearbyProvider{
		{ProviderID: uuid.NewString(), Lat: 6.52, Lng: 3.38, DistanceKM: 0.5},
		{ProviderID: uuid.NewString(), Lat: 6.53, Lng: 3.39, DistanceKM: 1.2},
	}
	svc := NewService(repo, live)

	resp, err := svc.GetNearbyProviders(context.Background(), NearbyProvidersRequest{
		Latitude: 6.5244, Longitude: 3.3792,
	})
	if err != nil {
		t.Fatalf("GetNearbyProviders error = %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("count = %d, want 2", resp.Count)
	}
	if resp.RadiusKM != 5.0 {
		t.Fatalf("radius_km = %.1f, want 5.0 (default)", resp.RadiusKM)
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(resp.Providers))
	}
}

func Test5I_ServiceReturnsEmptyProvidersNotNil(t *testing.T) {
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	live.nearby = nil // GetNearby may return nil on empty
	svc := NewService(repo, live)

	resp, err := svc.GetNearbyProviders(context.Background(), NearbyProvidersRequest{
		Latitude: 6.5244, Longitude: 3.3792,
	})
	if err != nil {
		t.Fatalf("GetNearbyProviders error = %v", err)
	}
	if resp.Providers == nil {
		t.Fatal("providers must be an empty slice, not nil")
	}
	if resp.Count != 0 {
		t.Fatalf("count = %d, want 0", resp.Count)
	}
}
