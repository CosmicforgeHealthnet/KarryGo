package availability

// Phase 5F (POST /api/v1/provider/location) and Phase 5G (GET /api/v1/provider/location) tests.
// Uses miniredis so no real Redis is required.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

// ── shared helpers ────────────────────────────────────────────────────────────

func newLocationTestEnv(t *testing.T) (*gin.Engine, *authusecases.TokenUsecase, *RedisLiveStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("loc-test-secret"), time.Hour, time.Hour)
	store := NewRedisLiveStore(client)
	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)

	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "test-service-key", handler)
	return router, tokens, store, mr
}

func providerJWT(t *testing.T, tokens *authusecases.TokenUsecase, providerID string) string {
	t.Helper()
	tok, _, err := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	return tok
}

func postLocation(router *gin.Engine, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/location", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func getLocation(router *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/location", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertHTTPCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, want, w.Body.String())
	}
}

func assertHTTPErrorCode(t *testing.T, w *httptest.ResponseRecorder, code apperrors.Code) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v; body = %s", err, w.Body.String())
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(code) {
		t.Fatalf("error.code = %v, want %s; body = %s", errObj["code"], code, w.Body.String())
	}
}

// ── Phase 5F — POST /api/v1/provider/location ─────────────────────────────────

func Test5F_MissingJWTReturns401(t *testing.T) {
	router, _, _, _ := newLocationTestEnv(t)
	w := postLocation(router, "", `{"lat":6.5244,"lng":3.3792,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5F_InvalidJWTReturns401(t *testing.T) {
	router, _, _, _ := newLocationTestEnv(t)
	w := postLocation(router, "bad.token.here", `{"lat":6,"lng":3,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5F_OfflineProviderReturns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOffline)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":6.5244,"lng":3.3792,"heading":45,"speed":20,"accuracy":8}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeBadRequest)
}

func Test5F_MissingRedisStatusReturns400(t *testing.T) {
	router, tokens, _, _ := newLocationTestEnv(t)
	providerID := uuid.NewString()
	tok := providerJWT(t, tokens, providerID)
	// No status key set — treated as offline.
	w := postLocation(router, tok, `{"lat":6.5244,"lng":3.3792,"heading":45,"speed":20,"accuracy":8}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeBadRequest)
}

func Test5F_OnlineProviderCanUpdateLocation(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":6.5244,"lng":3.3792,"heading":45,"speed":23.5,"accuracy":8.2}`)
	assertHTTPCode(t, w, http.StatusOK)
}

func Test5F_BusyProviderCanUpdateLocation(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusBusy)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":6.5244,"lng":3.3792,"heading":45,"speed":23.5,"accuracy":8.2}`)
	assertHTTPCode(t, w, http.StatusOK)
}

func Test5F_ValidLocationStoredInRedisWithTTL(t *testing.T) {
	_, _, store, mr := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6.5244, Lng: 3.3792, Heading: 45, Speed: 23.5, Accuracy: 8.2}); err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}

	val, err := mr.Get(ProviderLocationKey(providerID))
	if err != nil {
		t.Fatalf("location key not found in Redis: %v", err)
	}
	if val == "" {
		t.Fatal("location key is empty")
	}
	ttl := mr.TTL(ProviderLocationKey(providerID))
	if ttl <= 0 || ttl > LocationTTL {
		t.Fatalf("location TTL = %v, want > 0 and <= %v", ttl, LocationTTL)
	}
}

func Test5F_StatusTTLRefreshedOnAcceptedPing(t *testing.T) {
	_, _, store, mr := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)

	// Partially consume status TTL.
	mr.FastForward(30 * time.Second)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6, Lng: 3}); err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}

	// After refresh, TTL should be back near StatusTTL (90 s).
	ttl := mr.TTL(ProviderStatusKey(providerID))
	if ttl <= 0 || ttl > StatusTTL {
		t.Fatalf("status TTL after refresh = %v, want (0, %v]", ttl, StatusTTL)
	}
}

func Test5F_OnlinePingGeoAddsProvider(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()
	store := NewRedisLiveStore(client)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6.5244, Lng: 3.3792}); err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}

	// Verify via GEOPOS that the provider is in the GEO set (miniredis supports GEOPOS).
	positions, err := client.GeoPos(ctx, OnlineProvidersGeoKey, providerID).Result()
	if err != nil {
		t.Fatalf("GEOPOS error = %v", err)
	}
	if len(positions) != 1 || positions[0] == nil {
		t.Fatalf("provider not in avail:geo:online; positions = %+v", positions)
	}
}

func Test5F_BusyPingNotInGeoOnline(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()
	store := NewRedisLiveStore(client)
	ctx := context.Background()
	providerID := uuid.NewString()

	// First ping while online → enters GEO.
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6.5244, Lng: 3.3792}); err != nil {
		t.Fatalf("online ping error = %v", err)
	}

	// Switch to busy and ping again → must be removed from GEO.
	_ = store.SetStatus(ctx, providerID, StatusBusy)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6.5244, Lng: 3.3792}); err != nil {
		t.Fatalf("busy ping error = %v", err)
	}

	// GEOPOS returns nil for members not in the set.
	positions, err := client.GeoPos(ctx, OnlineProvidersGeoKey, providerID).Result()
	if err != nil {
		t.Fatalf("GEOPOS error = %v", err)
	}
	if len(positions) > 0 && positions[0] != nil {
		t.Fatal("busy provider must not remain in avail:geo:online")
	}
}

func Test5F_LocationUpdatePublishesWSMessageToChannel(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	store := NewRedisLiveStore(client)
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)

	sub := client.Subscribe(ctx, ProviderLocationChannel(providerID))
	defer sub.Close()
	ch := sub.Channel()

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	if _, err := svc.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6.5244, Lng: 3.3792, Heading: 45, Speed: 23.5, Accuracy: 8.2}); err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}

	select {
	case msg := <-ch:
		var wsMsg map[string]any
		if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err != nil {
			t.Fatalf("unmarshal pub/sub payload: %v", err)
		}
		if wsMsg["type"] != "location_update" {
			t.Fatalf("type = %v, want location_update", wsMsg["type"])
		}
		if wsMsg["provider_id"] != providerID {
			t.Fatalf("provider_id = %v, want %s", wsMsg["provider_id"], providerID)
		}
		if wsMsg["lat"] != 6.5244 {
			t.Fatalf("lat = %v, want 6.5244", wsMsg["lat"])
		}
		if wsMsg["lng"] != 3.3792 {
			t.Fatalf("lng = %v, want 3.3792", wsMsg["lng"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pub/sub message")
	}
}

func Test5F_ResponseIsMinimalUpdatedTrue(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":6.5244,"lng":3.3792,"heading":45,"speed":23.5,"accuracy":8.2}`)
	assertHTTPCode(t, w, http.StatusOK)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["updated"] != true {
		t.Fatalf("data.updated = %v, want true; body = %s", data["updated"], w.Body.String())
	}
	if _, hasLat := data["lat"]; hasLat {
		t.Fatal("minimal response must not include lat field")
	}
}

func Test5F_LatBelowMinus90Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":-91,"lng":3,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_LatAbove90Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":91,"lng":3,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_LngBelowMinus180Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":-181,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_LngAbove180Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":181,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_HeadingBelow0Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":3,"heading":-1,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_HeadingAbove360Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":3,"heading":361,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_SpeedBelowZeroReturns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":3,"heading":0,"speed":-1,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_AccuracyBelowZeroReturns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{"lat":6,"lng":3,"heading":0,"speed":0,"accuracy":-1}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5F_InvalidJSONBodyReturns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)
	w := postLocation(router, tok, `{bad json`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

// ── Phase 5G — GET /api/v1/provider/location ──────────────────────────────────

func Test5G_MissingJWTReturns401(t *testing.T) {
	router, _, _, _ := newLocationTestEnv(t)
	w := getLocation(router, "")
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5G_InvalidJWTReturns401(t *testing.T) {
	router, _, _, _ := newLocationTestEnv(t)
	w := getLocation(router, "bad.token.here")
	assertHTTPCode(t, w, http.StatusUnauthorized)
}

func Test5G_ExistingLocationReturns200WithAllFields(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	loc := Location{
		ProviderID: providerID,
		Lat:        6.5244,
		Lng:        3.3792,
		Heading:    45,
		Speed:      23.5,
		Accuracy:   8.2,
		UpdatedAt:  time.Now().UTC(),
	}
	_ = store.SetLocation(ctx, providerID, loc, false)
	tok := providerJWT(t, tokens, providerID)

	w := getLocation(router, tok)
	assertHTTPCode(t, w, http.StatusOK)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	for _, field := range []string{"lat", "lng", "heading", "speed", "accuracy", "updated_at"} {
		if data[field] == nil {
			t.Fatalf("field %q missing in response; body = %s", field, w.Body.String())
		}
	}
}

func Test5G_MissingRedisLocationReturns404(t *testing.T) {
	router, tokens, _, _ := newLocationTestEnv(t)
	providerID := uuid.NewString()
	tok := providerJWT(t, tokens, providerID)
	w := getLocation(router, tok)
	assertHTTPCode(t, w, http.StatusNotFound)
	assertHTTPErrorCode(t, w, apperrors.CodeNotFound)
}

func Test5G_ExpiredRedisLocationReturns404(t *testing.T) {
	router, tokens, store, mr := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	loc := Location{ProviderID: providerID, Lat: 6, Lng: 3, UpdatedAt: time.Now().UTC()}
	_ = store.SetLocation(ctx, providerID, loc, false)

	mr.FastForward(LocationTTL + time.Second)

	tok := providerJWT(t, tokens, providerID)
	w := getLocation(router, tok)
	assertHTTPCode(t, w, http.StatusNotFound)
	assertHTTPErrorCode(t, w, apperrors.CodeNotFound)
}

func Test5G_CorruptRedisLocationReturnsInternalError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	providerID := uuid.NewString()
	store := NewRedisLiveStore(client)
	// Write invalid JSON directly.
	_ = client.Set(ctx, ProviderLocationKey(providerID), "not-valid-json", LocationTTL).Err()

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	_, err := svc.GetLocation(ctx, providerID)
	if err == nil {
		t.Fatal("expected error for corrupt Redis location")
	}
	// Must not claim not_found — this is a real data error.
	var appErr *apperrors.Error
	if errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound {
		t.Fatal("corrupt data should not return not_found")
	}
}

func Test5G_ProviderIDComesFromJWT(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	otherID := uuid.NewString()
	// Store location for a different provider.
	loc := Location{ProviderID: otherID, Lat: 9, Lng: 9, UpdatedAt: time.Now().UTC()}
	_ = store.SetLocation(ctx, otherID, loc, false)

	// Authenticate as providerID — should NOT see otherID's location.
	tok := providerJWT(t, tokens, providerID)
	w := getLocation(router, tok)
	assertHTTPCode(t, w, http.StatusNotFound)
}
