package request

// Phase 6F–6H tests: request detail, accept (with Redis atomic lock), reject.
// Uses miniredis for all Redis-dependent tests.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/httpx"
)

// ── shared test helpers ───────────────────────────────────────────────────────

type requestTestEnv struct {
	mr     *miniredis.Miniredis
	redis  *redis.Client
	repo   *fakeRepository
	events *fakeEventPublisher
	svc    *Service
	tokens *authusecases.TokenUsecase
	engine *gin.Engine
}

func newRequestTestEnv(t *testing.T) *requestTestEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("request-fgh-secret"), time.Hour, time.Hour)
	repo := newFakeRepository()
	events := &fakeEventPublisher{}
	svc := NewService(repo, rc, fakeNearbyFinder{}, &fakeNotificationSender{}, events, nil, Config{BroadcastWindow: 30 * time.Second})
	svc.now = func() time.Time { return time.Now().UTC() }

	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(engine, tokens, NewHandler(svc))
	return &requestTestEnv{mr: mr, redis: rc, repo: repo, events: events, svc: svc, tokens: tokens, engine: engine}
}

func (e *requestTestEnv) providerToken(t *testing.T, providerID string) string {
	t.Helper()
	tok, _, err := e.tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return injectRequestRole(t, tok, "dispatch_provider", []byte("request-fgh-secret"))
}

func injectRequestRole(t *testing.T, token, role string, secret []byte) string {
	t.Helper()
	import_crypto_hmac_sha256_base64 := func(unsigned string) string {
		return computeRequestHMAC(secret, unsigned)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	payload, err := decodeBase64RawURL(parts[1])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	claims["role"] = role
	updated, _ := json.Marshal(claims)
	unsigned := parts[0] + "." + encodeBase64RawURL(updated)
	return unsigned + "." + import_crypto_hmac_sha256_base64(unsigned)
}

func newActiveBroadcastAndInbox(t *testing.T, env *requestTestEnv, providerID string) (RequestBroadcast, ProviderRequestInbox) {
	t.Helper()
	event := fullBookingEvent()
	payload, _ := json.Marshal(event)
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(30 * time.Second), BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	// Seed broadcasting key in Redis.
	_ = env.redis.Set(context.Background(), RequestBroadcastingKey(event.BookingID), broadcast.ID, 35*time.Second)
	return broadcast, inbox
}

func fullBookingEvent() BookingDispatchCreatedEvent {
	return BookingDispatchCreatedEvent{
		BookingID: uuid.NewString(), CustomerID: uuid.NewString(),
		PickupLat: 6.4474, PickupLng: 3.4343, DropoffLat: 6.4969, DropoffLng: 3.3481,
		PickupAddress: "15 Awolowo Road, Ikoyi", DropoffAddress: "32 Bode Thomas, Surulere",
		FareAmount: 150000, Currency: "NGN", ServiceType: "dispatch",
		ReceiverName: "Chidi Obi", ReceiverPhone: "+2348011223344",
		PackageDesc: "Pharmacy items", PackageWeight: 1.5,
		BookingPayload: json.RawMessage(`{"note":"fragile"}`), OccurredAt: time.Now().UTC(),
	}
}

func doRequest(engine *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, want, w.Body.String())
	}
}

func assertErrorCodeStr(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body = %s", err, w.Body.String())
	}
	errObj, _ := resp["error"].(map[string]any)
	if fmt.Sprint(errObj["code"]) != code {
		t.Fatalf("error.code = %v, want %s; body = %s", errObj["code"], code, w.Body.String())
	}
}

func extractData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body = %s", err, w.Body.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatalf("data field missing; body = %s", w.Body.String())
	}
	return data
}

// ── Phase 6F — GET /api/v1/provider/requests/:id ─────────────────────────────

func Test6F_MissingJWTReturns401(t *testing.T) {
	env := newRequestTestEnv(t)
	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+uuid.NewString(), "", "")
	assertStatus(t, w, http.StatusUnauthorized)
}

func Test6F_InvalidInboxIDReturns400(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/not-a-uuid", tok, "")
	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCodeStr(t, w, "validation_failed")
}

func Test6F_ReturnsFullDetailWithReceiverPhone(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)

	for _, field := range []string{"inbox_id", "broadcast_id", "booking_id", "status", "fare_amount", "currency",
		"pickup_address", "pickup_lat", "pickup_lng", "dropoff_address", "dropoff_lat", "dropoff_lng",
		"receiver_name", "receiver_phone", "remaining_seconds", "expires_at", "received_at"} {
		if data[field] == nil {
			t.Fatalf("field %q missing in detail response; body = %s", field, w.Body.String())
		}
	}
	if data["receiver_phone"] != "+2348011223344" {
		t.Fatalf("receiver_phone = %v, want +2348011223344", data["receiver_phone"])
	}
	if data["fare_amount"] != float64(150000) {
		t.Fatalf("fare_amount = %v, want 150000", data["fare_amount"])
	}
}

func Test6F_CrossProviderReturns404(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	other := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, other)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

func Test6F_ExpiredInboxStillReturnsDetailWithZeroRemainingSeconds(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	payload, _ := json.Marshal(event)
	past := time.Now().UTC().Add(-5 * time.Second) // already expired
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusExpired,
		ExpiresAt: past, BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusExpired,
		ExpiresAt: past, BookingPayload: payload, ReceivedAt: past.Add(-20 * time.Second),
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusOK) // must still return 200
	data := extractData(t, w)
	if data["status"] != "expired" {
		t.Fatalf("status = %v, want expired", data["status"])
	}
	if data["remaining_seconds"] != float64(0) {
		t.Fatalf("remaining_seconds = %v, want 0", data["remaining_seconds"])
	}
}

func Test6F_CorruptPayloadReturns500(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: uuid.NewString(), Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(20 * time.Second), BookingPayload: json.RawMessage(`{corrupt`),
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: broadcast.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: json.RawMessage(`{corrupt`), ReceivedAt: now,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusInternalServerError)
}

// ── Phase 6G — POST /api/v1/provider/requests/:id/accept ─────────────────────

func Test6G_MissingJWTReturns401(t *testing.T) {
	env := newRequestTestEnv(t)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+uuid.NewString()+"/accept", "", "")
	assertStatus(t, w, http.StatusUnauthorized)
}

func Test6G_InvalidInboxIDReturns400(t *testing.T) {
	env := newRequestTestEnv(t)
	tok := env.providerToken(t, uuid.NewString())
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/not-uuid/accept", tok, "")
	assertStatus(t, w, http.StatusBadRequest)
}

func Test6G_CrossProviderReturns404(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	other := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, other)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

func Test6G_AcceptNonPendingReturns409(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	// Mark inbox as already rejected.
	for i := range env.repo.inboxes {
		if env.repo.inboxes[i].ID == inbox.ID {
			env.repo.inboxes[i].Status = InboxStatusRejected
		}
	}
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusConflict)
}

func Test6G_AcceptExpiredBroadcastReturns410(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	payload, _ := json.Marshal(event)
	past := time.Now().UTC().Add(-5 * time.Second)
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: past, BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusPending,
		ExpiresAt: past, BookingPayload: payload, ReceivedAt: past,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusGone)
	assertErrorCodeStr(t, w, "gone")
}

func Test6G_AcceptWhenBroadcastingKeyMissingReturns410(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	// Remove the broadcasting key.
	_ = env.redis.Del(context.Background(), RequestBroadcastingKey(inbox.BookingID))
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusGone)
}

func Test6G_FirstAcceptReturns200(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)
	if data["message"] == nil {
		t.Fatalf("message missing; body = %s", w.Body.String())
	}
	if data["pickup_address"] == nil {
		t.Fatalf("pickup_address missing; body = %s", w.Body.String())
	}
	if data["receiver_phone"] != "+2348011223344" {
		t.Fatalf("receiver_phone = %v, want +2348011223344", data["receiver_phone"])
	}
}

func Test6G_AcceptedInboxStatusBecomesAccepted(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)
	for _, i := range env.repo.inboxes {
		if i.ID == inbox.ID && i.Status != InboxStatusAccepted {
			t.Fatalf("inbox status = %s, want accepted", i.Status)
		}
	}
}

func Test6G_OtherPendingInboxesExpireAfterAccept(t *testing.T) {
	env := newRequestTestEnv(t)
	p1 := uuid.NewString()
	p2 := uuid.NewString()
	broadcast, inbox1 := newActiveBroadcastAndInbox(t, env, p1)
	// Add a second provider to the same broadcast.
	event := fullBookingEvent()
	event.BookingID = inbox1.BookingID
	payload, _ := json.Marshal(event)
	inbox2 := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: inbox1.BookingID,
		ProviderID: p2, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: time.Now().UTC(),
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox2)

	tok1 := env.providerToken(t, p1)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox1.ID+"/accept", tok1, "")
	assertStatus(t, w, http.StatusOK)

	// p2's inbox must now be expired.
	for _, i := range env.repo.inboxes {
		if i.ID == inbox2.ID && i.Status != InboxStatusExpired {
			t.Fatalf("p2 inbox status = %s, want expired", i.Status)
		}
	}
}

func Test6G_BroadcastStatusBecomesAccepted(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	broadcast, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)

	for _, b := range env.repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusAccepted {
			t.Fatalf("broadcast status = %s, want accepted", b.Status)
		}
		if b.ID == broadcast.ID && (b.AcceptedByProviderID == nil || *b.AcceptedByProviderID != providerID) {
			t.Fatalf("accepted_by_provider_id = %v, want %s", b.AcceptedByProviderID, providerID)
		}
	}
}

func Test6G_AcceptedKeySetAfterSuccess(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)

	val, err := env.redis.Get(context.Background(), RequestAcceptedKey(inbox.BookingID)).Result()
	if err != nil || val != providerID {
		t.Fatalf("accepted key = %q err = %v, want %s", val, err, providerID)
	}
}

func Test6G_BroadcastingKeyDeletedAfterSuccess(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)

	exists, _ := env.redis.Exists(context.Background(), RequestBroadcastingKey(inbox.BookingID)).Result()
	if exists != 0 {
		t.Fatal("broadcasting key still exists after accept")
	}
}

func Test6G_RequestAcceptedEventPublished(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	if len(env.events.accepted) != 1 {
		t.Fatalf("accepted events = %d, want 1", len(env.events.accepted))
	}
	evt := env.events.accepted[0]
	if evt.ProviderID != providerID || evt.BookingID != inbox.BookingID {
		t.Fatalf("event = %+v", evt)
	}
	if evt.AcceptedAt.IsZero() {
		t.Fatal("accepted_at not set in event")
	}
}

func Test6G_SecondAcceptReturns409RequestTaken(t *testing.T) {
	env := newRequestTestEnv(t)
	p1 := uuid.NewString()
	p2 := uuid.NewString()
	broadcast, inbox1 := newActiveBroadcastAndInbox(t, env, p1)
	// Add inbox2 for p2 in same broadcast.
	event := fullBookingEvent()
	event.BookingID = inbox1.BookingID
	payload, _ := json.Marshal(event)
	inbox2 := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: inbox1.BookingID,
		ProviderID: p2, Status: InboxStatusPending,
		ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: time.Now().UTC(),
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox2)

	tok1 := env.providerToken(t, p1)
	tok2 := env.providerToken(t, p2)

	// p1 accepts first.
	w1 := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox1.ID+"/accept", tok1, "")
	assertStatus(t, w1, http.StatusOK)

	// p2 tries to accept after p1 already accepted — broadcasting key is gone.
	w2 := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox2.ID+"/accept", tok2, "")
	// Inbox2 was marked expired by the DB transaction — expect 409 (non-pending).
	if w2.Code != http.StatusGone && w2.Code != http.StatusConflict {
		t.Fatalf("second accept status = %d, want 409 or 410; body = %s", w2.Code, w2.Body.String())
	}
}

func Test6G_ConcurrentAcceptExactlyOneWins(t *testing.T) {
	// Two goroutines race to accept the same inbox.
	// Exactly one must get 200, the other must get a conflict/gone.
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	results := make([]int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
			results[i] = w.Code
		}()
	}
	wg.Wait()

	success := 0
	for _, code := range results {
		if code == http.StatusOK {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent accept: %d success(es), want exactly 1; results = %v", success, results)
	}
}

// ── Phase 6H — POST /api/v1/provider/requests/:id/reject ─────────────────────

func Test6H_MissingJWTReturns401(t *testing.T) {
	env := newRequestTestEnv(t)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+uuid.NewString()+"/reject", "", "")
	assertStatus(t, w, http.StatusUnauthorized)
}

func Test6H_InvalidInboxIDReturns400(t *testing.T) {
	env := newRequestTestEnv(t)
	tok := env.providerToken(t, uuid.NewString())
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/not-uuid/reject", tok, "")
	assertStatus(t, w, http.StatusBadRequest)
}

func Test6H_CrossProviderReturns404(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	other := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, other)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

func Test6H_RejectPendingReturns200(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, `{"reason":"too_far"}`)
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)
	if data["message"] == nil {
		t.Fatalf("message missing; body = %s", w.Body.String())
	}
}

func Test6H_InboxStatusBecomesRejected(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")

	for _, i := range env.repo.inboxes {
		if i.ID == inbox.ID {
			if i.Status != InboxStatusRejected {
				t.Fatalf("inbox status = %s, want rejected", i.Status)
			}
			if i.RespondedAt == nil {
				t.Fatal("responded_at not set after reject")
			}
		}
	}
}

func Test6H_BroadcastStatusRemainsUnchangedAfterReject(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	broadcast, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")

	for _, b := range env.repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusBroadcasting {
			t.Fatalf("broadcast status = %s, want broadcasting (unchanged)", b.Status)
		}
	}
}

func Test6H_RequestRejectedEventPublished(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, `{"reason":"busy"}`)

	if len(env.events.rejected) != 1 {
		t.Fatalf("rejected events = %d, want 1", len(env.events.rejected))
	}
	evt := env.events.rejected[0]
	if evt.ProviderID != providerID || evt.BookingID != inbox.BookingID {
		t.Fatalf("event provider_id/booking_id mismatch: %+v", evt)
	}
	if evt.Reason != "busy" {
		t.Fatalf("event reason = %q, want busy", evt.Reason)
	}
}

func Test6H_RejectAcceptedInboxReturns409(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	// Accept first.
	tok := env.providerToken(t, providerID)
	w1 := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w1, http.StatusOK)
	// Now reject the same inbox.
	w2 := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w2, http.StatusConflict)
}

func Test6H_RejectAlreadyRejectedReturns409(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusConflict)
}

func Test6H_InvalidReasonReturns400(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, `{"reason":"not_a_valid_reason"}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCodeStr(t, w, "validation_failed")
}

func Test6H_EmptyReasonIsAccepted(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusOK)
	// Event reason should default to "other".
	if len(env.events.rejected) > 0 && env.events.rejected[0].Reason != "other" {
		t.Fatalf("default reason = %q, want other", env.events.rejected[0].Reason)
	}
}

func Test6H_AllValidReasonsAccepted(t *testing.T) {
	for _, reason := range []string{"too_far", "busy", "other"} {
		t.Run(reason, func(t *testing.T) {
			env := newRequestTestEnv(t)
			providerID := uuid.NewString()
			_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
			tok := env.providerToken(t, providerID)
			w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject",
				tok, fmt.Sprintf(`{"reason":%q}`, reason))
			assertStatus(t, w, http.StatusOK)
		})
	}
}

// ── IDOR / security tests ─────────────────────────────────────────────────────

func Test_IDORProviderCannotViewOtherDetail(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, attacker)
	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

func Test_IDORProviderCannotAcceptOtherInbox(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, attacker)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

func Test_IDORProviderCannotRejectOtherInbox(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, attacker)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusNotFound)
}

// ── JWT helpers (local to this file) ─────────────────────────────────────────

func computeRequestHMAC(secret []byte, msg string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func decodeBase64RawURL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func encodeBase64RawURL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
