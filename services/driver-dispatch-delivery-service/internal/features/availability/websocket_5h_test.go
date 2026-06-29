package availability

// Phase 5H WebSocket location stream tests.
// Uses a real HTTP test server + miniredis for reliable WebSocket handshake testing.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

// ── test environment ──────────────────────────────────────────────────────────

type wsTestEnv struct {
	server     *httptest.Server
	client     *redis.Client
	store      *RedisLiveStore
	tokens     *authusecases.TokenUsecase
	providerID string
	token      string
}

func newWSTestEnv(t *testing.T) *wsTestEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("ws-test-secret"), time.Hour, time.Hour)
	providerID := uuid.NewString()
	tok, _, err := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	store := NewRedisLiveStore(client)
	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "test-service-key", handler)
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
		_ = client.Close()
	})

	return &wsTestEnv{
		server:     server,
		client:     client,
		store:      store,
		tokens:     tokens,
		providerID: providerID,
		token:      tok,
	}
}

func (e *wsTestEnv) buildURL(providerID, token string) string {
	base := "ws" + strings.TrimPrefix(e.server.URL, "http")
	u := base + "/ws/provider/" + providerID + "/location"
	if token != "" {
		u += "?token=" + token
	}
	return u
}

func tryDialWS(t *testing.T, rawURL string) (*websocket.Conn, *http.Response) {
	t.Helper()
	conn, resp, _ := websocket.DefaultDialer.Dial(rawURL, nil)
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, resp
}

func mustDialWS(t *testing.T, rawURL string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(rawURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial WS status=%d error=%v", status, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readWSMessage(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	return string(payload)
}

func tryReadWSMessage(conn *websocket.Conn) (string, bool) {
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		return "", false
	}
	return string(payload), true
}

// ── Phase 5H tests ────────────────────────────────────────────────────────────

func Test5H_MissingTokenReturns401BeforeUpgrade(t *testing.T) {
	env := newWSTestEnv(t)
	conn, resp := tryDialWS(t, env.buildURL(env.providerID, ""))
	if conn != nil {
		t.Fatal("expected connection failure, got a connected WebSocket")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("status = %d, want 401", code)
	}
}

func Test5H_InvalidTokenReturns401BeforeUpgrade(t *testing.T) {
	env := newWSTestEnv(t)
	conn, resp := tryDialWS(t, env.buildURL(env.providerID, "bad.token"))
	if conn != nil {
		t.Fatal("expected connection failure")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("status = %d, want 401", code)
	}
}

func Test5H_InvalidProviderIDReturns400BeforeUpgrade(t *testing.T) {
	env := newWSTestEnv(t)
	conn, resp := tryDialWS(t, env.buildURL("not-a-uuid", env.token))
	if conn != nil {
		t.Fatal("expected connection failure")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("status = %d, want 400", code)
	}
}

func Test5H_ProviderTokenMismatchReturns403(t *testing.T) {
	env := newWSTestEnv(t)
	// env.token is issued for env.providerID; connect to a different provider's stream.
	otherID := uuid.NewString()
	conn, resp := tryDialWS(t, env.buildURL(otherID, env.token))
	if conn != nil {
		t.Fatal("expected connection failure")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("status = %d, want 403", code)
	}
}

func Test5H_ValidTokenUpgradesToWebSocket(t *testing.T) {
	env := newWSTestEnv(t)
	conn := mustDialWS(t, env.buildURL(env.providerID, env.token))
	// Must receive the initial message without error.
	_ = readWSMessage(t, conn)
}

func Test5H_InitialLocationSentImmediatelyOnConnect(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewRedisLiveStore(client)
	ctx := context.Background()
	providerID := uuid.NewString()

	// Pre-seed a cached location.
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	loc := Location{
		ProviderID: providerID,
		Lat:        6.5244,
		Lng:        3.3792,
		Heading:    45,
		Speed:      23.5,
		Accuracy:   8.2,
		UpdatedAt:  time.Now().UTC(),
	}
	_ = store.SetLocation(ctx, providerID, loc, true)

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("ws-test-secret-2"), time.Hour, time.Hour)
	tok, _, err := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "key", handler)
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
		_ = client.Close()
	})

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") +
		"/ws/provider/" + providerID + "/location?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	firstMsg := readWSMessage(t, conn)
	var msg map[string]any
	if err := json.Unmarshal([]byte(firstMsg), &msg); err != nil {
		t.Fatalf("unmarshal: %v; raw = %s", err, firstMsg)
	}
	if msg["type"] != "location_update" {
		t.Fatalf("first message type = %v, want location_update; raw = %s", msg["type"], firstMsg)
	}
	if msg["lat"] != 6.5244 {
		t.Fatalf("lat = %v, want 6.5244", msg["lat"])
	}
}

func Test5H_LocationUnavailableSentWhenNoCachedLocation(t *testing.T) {
	env := newWSTestEnv(t)
	conn := mustDialWS(t, env.buildURL(env.providerID, env.token))

	firstMsg := readWSMessage(t, conn)
	var msg map[string]any
	_ = json.Unmarshal([]byte(firstMsg), &msg)
	if msg["type"] != "location_unavailable" {
		t.Fatalf("first message type = %v, want location_unavailable; raw = %s", msg["type"], firstMsg)
	}
	if msg["provider_id"] != env.providerID {
		t.Fatalf("provider_id = %v, want %s", msg["provider_id"], env.providerID)
	}
}

func Test5H_NewGPSPingForwardedToWebSocketClient(t *testing.T) {
	env := newWSTestEnv(t)
	conn := mustDialWS(t, env.buildURL(env.providerID, env.token))
	// Drain initial message.
	_ = readWSMessage(t, conn)

	pubPayload := `{"type":"location_update","provider_id":"` + env.providerID + `","lat":6.5244,"lng":3.3792}`
	if err := env.client.Publish(context.Background(), ProviderLocationChannel(env.providerID), pubPayload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	got, ok := tryReadWSMessage(conn)
	if !ok {
		t.Fatal("timed out waiting for GPS ping on WebSocket")
	}
	if got != pubPayload {
		t.Fatalf("message = %s, want %s", got, pubPayload)
	}
}

func Test5H_MultipleClientsReceiveSameUpdate(t *testing.T) {
	env := newWSTestEnv(t)

	conn1 := mustDialWS(t, env.buildURL(env.providerID, env.token))
	conn2 := mustDialWS(t, env.buildURL(env.providerID, env.token))
	// Drain initial messages.
	_ = readWSMessage(t, conn1)
	_ = readWSMessage(t, conn2)

	pubPayload := `{"type":"location_update","provider_id":"` + env.providerID + `","lat":6,"lng":3}`
	if err := env.client.Publish(context.Background(), ProviderLocationChannel(env.providerID), pubPayload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	got1, ok1 := tryReadWSMessage(conn1)
	got2, ok2 := tryReadWSMessage(conn2)
	if !ok1 {
		t.Fatal("client 1 did not receive the GPS update")
	}
	if !ok2 {
		t.Fatal("client 2 did not receive the GPS update")
	}
	if got1 != pubPayload {
		t.Fatalf("client 1 message = %s, want %s", got1, pubPayload)
	}
	if got2 != pubPayload {
		t.Fatalf("client 2 message = %s, want %s", got2, pubPayload)
	}
}

func Test5H_ProviderOfflineMessageForwardedToWebSocketClient(t *testing.T) {
	env := newWSTestEnv(t)
	ctx := context.Background()
	_ = env.store.SetStatus(ctx, env.providerID, StatusOnline)

	conn := mustDialWS(t, env.buildURL(env.providerID, env.token))
	// Drain initial location_unavailable (no cached location yet).
	_ = readWSMessage(t, conn)

	// Give the handler goroutine a moment to get back into its select loop
	// before we publish — same pattern as TestWebSocketConnectsAndPublishesLocationMessages.
	time.Sleep(50 * time.Millisecond)

	// ClearProvider publishes provider_offline to the location channel.
	if err := env.store.ClearProvider(ctx, env.providerID); err != nil {
		t.Fatalf("ClearProvider: %v", err)
	}

	got, ok := tryReadWSMessage(conn)
	if !ok {
		t.Fatal("timed out waiting for provider_offline on WebSocket")
	}
	var msg map[string]any
	_ = json.Unmarshal([]byte(got), &msg)
	if msg["type"] != "provider_offline" {
		t.Fatalf("message type = %v, want provider_offline; raw = %s", msg["type"], got)
	}
	if msg["provider_id"] != env.providerID {
		t.Fatalf("provider_id = %v, want %s", msg["provider_id"], env.providerID)
	}
}

func Test5H_ClientDisconnectAllowsNewConnection(t *testing.T) {
	env := newWSTestEnv(t)
	conn1 := mustDialWS(t, env.buildURL(env.providerID, env.token))
	_ = readWSMessage(t, conn1)
	_ = conn1.Close()

	time.Sleep(50 * time.Millisecond)

	// A new connection should succeed cleanly.
	tok2, _, _ := env.tokens.GenerateAccessToken(env.providerID, "+2348000000001", uuid.NewString())
	conn2 := mustDialWS(t, env.buildURL(env.providerID, tok2))
	_ = readWSMessage(t, conn2)
}

func Test5H_AdminRoleCanStreamAnyProvider(t *testing.T) {
	env := newWSTestEnv(t)
	otherID := uuid.NewString()
	// Build a platform_admin token issued to otherID.
	adminTok, _, err := env.tokens.GenerateAccessToken(otherID, "+2348000000002", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	adminTok = wsInjectRole(t, adminTok, "platform_admin", []byte("ws-test-secret"))

	// Admin should be able to connect to env.providerID's stream.
	conn := mustDialWS(t, env.buildURL(env.providerID, adminTok))
	_ = readWSMessage(t, conn)
}

// ── JWT helper ────────────────────────────────────────────────────────────────

func wsInjectRole(t *testing.T, token, role string, secret []byte) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	claims["role"] = role
	updated, _ := json.Marshal(claims)
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(updated)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig
}
