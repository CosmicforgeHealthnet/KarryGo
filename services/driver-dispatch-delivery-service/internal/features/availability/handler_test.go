package availability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

func TestProviderAvailabilityRoutesAreJWTProtected(t *testing.T) {
	router, _ := newAvailabilityTestRouter()

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/availability", strings.NewReader(`{"status":"online"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/provider/availability", strings.NewReader(`{"status":"online"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer invalid-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid JWT status = %d, want 401", response.Code)
	}
}

func TestProviderLocationRoutesAreJWTProtected(t *testing.T) {
	router, _ := newAvailabilityTestRouter()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/provider/location", strings.NewReader(`{"latitude":6,"longitude":3}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestGetProviderAvailabilityRequiresJWT(t *testing.T) {
	router, _ := newAvailabilityTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/provider/availability", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestInternalNearbyRequiresServiceKeyAndRejectsBearer(t *testing.T) {
	router, _ := newAvailabilityTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/internal/nearby?lat=6&lng=3", nil)
	request.Header.Set("Authorization", "Bearer normal-provider-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("bearer status = %d, want 401", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/nearby?lat=6&lng=3", nil)
	request.Header.Set("X-Internal-Service-Key", "test-service-key")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("service key status = %d, want 200 body=%s", response.Code, response.Body.String())
	}
}

func TestWebSocketQueryTokenIsValidatedBeforeUpgrade(t *testing.T) {
	router, tokens := newAvailabilityTestRouter()
	providerID := uuid.NewString()

	for _, tc := range []struct {
		name string
		url  string
		want int
	}{
		{name: "missing token", url: "/ws/provider/" + providerID + "/location", want: http.StatusUnauthorized},
		{name: "invalid token", url: "/ws/provider/" + providerID + "/location?token=bad", want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.url, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}

	token, _, err := tokens.GenerateAccessToken(uuid.NewString(), "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/ws/provider/"+providerID+"/location?token="+token, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("mismatch status = %d, want 403", response.Code)
	}
}

func TestWebSocketConnectsAndPublishesLocationMessages(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("test-access-secret"), time.Hour, time.Hour)
	providerID := uuid.NewString()
	token, _, err := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}

	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	// fakeAvailabilityService.GetLocation returns (LocationResponse{}, nil) — nil
	// Location — so the WebSocket handler will send location_unavailable first.
	handler := NewHandlerWithService(fakeAvailabilityService{}, tokens, redisClient)
	RegisterRoutes(router, tokens, "test-service-key", handler)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/provider/" + providerID + "/location?token=" + token
	conn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if wsResp != nil {
			status = wsResp.StatusCode
		}
		t.Fatalf("dial websocket status=%d error=%v", status, err)
	}
	defer conn.Close()

	// First message must be location_unavailable (no cached location in fake service).
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, firstMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read first message error = %v", err)
	}
	if !strings.Contains(string(firstMsg), "location_unavailable") {
		t.Fatalf("first message = %s, want location_unavailable", firstMsg)
	}

	// Publish a WS-format location update directly to Redis and expect it on the client.
	messageCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}
		messageCh <- string(payload)
	}()

	time.Sleep(50 * time.Millisecond)
	pubPayload := `{"type":"location_update","provider_id":"` + providerID + `","lat":6,"lng":3}`
	if err := redisClient.Publish(context.Background(), ProviderLocationChannel(providerID), pubPayload).Err(); err != nil {
		t.Fatalf("publish location error = %v", err)
	}

	select {
	case got := <-messageCh:
		if got != pubPayload {
			t.Fatalf("message = %s, want %s", got, pubPayload)
		}
	case err := <-errCh:
		t.Fatalf("read websocket error = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket location message")
	}
}

func newAvailabilityTestRouter() (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("test-access-secret"), time.Hour, time.Hour)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	handler := NewHandlerWithService(fakeAvailabilityService{}, tokens, nil)
	RegisterRoutes(router, tokens, "test-service-key", handler)
	return router, tokens
}

type fakeAvailabilityService struct{}

func (fakeAvailabilityService) SetStatus(context.Context, string, SetAvailabilityRequest) (AvailabilityResponse, error) {
	return AvailabilityResponse{}, nil
}

func (fakeAvailabilityService) GetStatus(context.Context, string) (AvailabilityStatusResponse, error) {
	return AvailabilityStatusResponse{}, nil
}

func (fakeAvailabilityService) GetCurrentSession(context.Context, string) (CurrentSessionResponse, error) {
	return CurrentSessionResponse{}, nil
}

func (fakeAvailabilityService) UpdateLocation(context.Context, string, UpdateLocationRequest) (LocationUpdateResponse, error) {
	return LocationUpdateResponse{Updated: true}, nil
}

func (fakeAvailabilityService) GetLocation(context.Context, string) (LocationResponse, error) {
	return LocationResponse{}, nil
}

func (fakeAvailabilityService) GetNearbyProviders(context.Context, NearbyProvidersRequest) (NearbyResponse, error) {
	return NearbyResponse{Providers: []NearbyProvider{}}, nil
}
