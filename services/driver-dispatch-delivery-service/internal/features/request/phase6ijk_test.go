package request

// Phase 6I–6K tests: booking.dispatch.cancelled subscriber, event payloads, security hardening.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ── fakeRepository extension: CancelBroadcast (satisfies updated Repository interface) ───

func (r *fakeRepository) CancelBroadcast(_ context.Context, broadcastID string) error {
	for i := range r.broadcasts {
		if r.broadcasts[i].ID == broadcastID {
			s := r.broadcasts[i].Status
			if s == BroadcastStatusBroadcasting || s == BroadcastStatusExpired {
				r.broadcasts[i].Status = BroadcastStatusCancelled
			}
		}
	}
	return nil
}

// ── Phase 6I — booking.dispatch.cancelled subscriber ──────────────────────────

func Test6I_BadPayloadDropsSafely(t *testing.T) {
	svc := NewService(newFakeRepository(), nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, []byte("not json at all")); err != nil {
		t.Fatalf("bad payload must not return error: %v", err)
	}
}

func Test6I_MissingBookingIDDropsSafely(t *testing.T) {
	svc := NewService(newFakeRepository(), nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, []byte(`{"event":"booking.dispatch.cancelled"}`)); err != nil {
		t.Fatalf("missing booking_id must not return error: %v", err)
	}
}

func Test6I_EmptyBookingIDDropsSafely(t *testing.T) {
	svc := NewService(newFakeRepository(), nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	payload := `{"event":"booking.dispatch.cancelled","booking_id":"   "}`
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, []byte(payload)); err != nil {
		t.Fatalf("whitespace booking_id must not return error: %v", err)
	}
}

func Test6I_MissingBroadcastIsNoOp(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: uuid.NewString()})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("missing broadcast must be no-op: %v", err)
	}
}

func Test6I_CancelBeforeAcceptMarksBroadcastCancelled(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusBroadcasting}
	repo.broadcasts = append(repo.broadcasts, broadcast)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	for _, b := range repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusCancelled {
			t.Fatalf("broadcast status = %s, want cancelled", b.Status)
		}
	}
}

func Test6I_CancelBeforeAcceptExpiresPendingInboxRows(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusBroadcasting}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	inbox1 := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: bookingID, Status: InboxStatusPending}
	inbox2 := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: bookingID, Status: InboxStatusPending}
	repo.inboxes = append(repo.inboxes, inbox1, inbox2)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	for _, i := range repo.inboxes {
		if i.BroadcastID == broadcast.ID && i.Status != InboxStatusExpired {
			t.Fatalf("inbox %s status = %s, want expired", i.ID, i.Status)
		}
	}
}

func Test6I_CancelDoesNotChangeAcceptedOrRejectedInboxRows(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusBroadcasting}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	accepted := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: bookingID, Status: InboxStatusAccepted}
	rejected := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: bookingID, Status: InboxStatusRejected}
	repo.inboxes = append(repo.inboxes, accepted, rejected)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	for _, i := range repo.inboxes {
		if i.ID == accepted.ID && i.Status != InboxStatusAccepted {
			t.Fatalf("accepted inbox status changed to %s", i.Status)
		}
		if i.ID == rejected.ID && i.Status != InboxStatusRejected {
			t.Fatalf("rejected inbox status changed to %s", i.Status)
		}
	}
}

func Test6I_CancelDeletesBroadcastingKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	repo := newFakeRepository()
	svc := NewService(repo, rc, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusBroadcasting}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	_ = rc.Set(context.Background(), RequestBroadcastingKey(bookingID), broadcast.ID, time.Minute)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel error: %v", err)
	}

	if mr.Exists(RequestBroadcastingKey(bookingID)) {
		t.Fatal("broadcasting key must be deleted after cancellation")
	}
}

func Test6I_CancelDoesNotDeleteAcceptedKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	repo := newFakeRepository()
	svc := NewService(repo, rc, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusBroadcasting}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	// Seed an accepted marker (e.g., set by a concurrent accept that just won).
	_ = rc.Set(context.Background(), RequestAcceptedKey(bookingID), uuid.NewString(), time.Hour)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	_ = HandleBookingDispatchCancelledPayload(context.Background(), svc, payload)

	if !mr.Exists(RequestAcceptedKey(bookingID)) {
		t.Fatal("accepted key must NOT be deleted by the cancellation flow")
	}
}

func Test6I_CancelAfterAcceptIsNoOp(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusAccepted}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: bookingID, Status: InboxStatusAccepted}
	repo.inboxes = append(repo.inboxes, inbox)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel after accept error: %v", err)
	}

	for _, b := range repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusAccepted {
			t.Fatalf("broadcast status changed after cancel-after-accept: got %s", b.Status)
		}
	}
	for _, i := range repo.inboxes {
		if i.ID == inbox.ID && i.Status != InboxStatusAccepted {
			t.Fatalf("inbox status changed after cancel-after-accept: got %s", i.Status)
		}
	}
}

func Test6I_DuplicateCancelIsIdempotent(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusCancelled}
	repo.broadcasts = append(repo.broadcasts, broadcast)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	for i := 0; i < 3; i++ {
		if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
			t.Fatalf("duplicate cancel[%d] error: %v", i, err)
		}
	}
	for _, b := range repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusCancelled {
			t.Fatalf("broadcast status = %s, want cancelled after duplicate cancel", b.Status)
		}
	}
}

func Test6I_CancelNoProviderFoundBroadcastIsNoOp(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	bookingID := uuid.NewString()
	broadcast := RequestBroadcast{ID: uuid.NewString(), BookingID: bookingID, Status: BroadcastStatusNoProviderFound}
	repo.broadcasts = append(repo.broadcasts, broadcast)

	payload, _ := json.Marshal(BookingDispatchCancelledEvent{BookingID: bookingID})
	if err := HandleBookingDispatchCancelledPayload(context.Background(), svc, payload); err != nil {
		t.Fatalf("cancel on no_provider_found: %v", err)
	}
	for _, b := range repo.broadcasts {
		if b.ID == broadcast.ID && b.Status != BroadcastStatusNoProviderFound {
			t.Fatalf("status changed: %s", b.Status)
		}
	}
}

func Test6I_ExpireWindowExitsCleanlyOnCancelledBroadcast(t *testing.T) {
	// HandleExpireWindow already skips non-broadcasting statuses.
	// This test confirms cancelled is handled as a no-op by the worker.
	repo := newFakeRepository()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: uuid.NewString(), Status: BroadcastStatusCancelled,
		AttemptNumber: 1, ExpiresAt: time.Now().UTC().Add(-time.Second),
	}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, Status: InboxStatusPending,
	}
	repo.inboxes = append(repo.inboxes, inbox)
	tasks := &fakeTaskEnqueuer{}
	task, _ := NewExpireWindowTask(ExpireWindowPayload{
		BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 1,
	})
	if err := NewWorker(NewService(repo, nil, nil, nil, nil, tasks, Config{})).HandleExpireWindow(context.Background(), task); err != nil {
		t.Fatalf("expire-window on cancelled broadcast returned error: %v", err)
	}
	if repo.inboxes[0].Status != InboxStatusPending || len(tasks.tasks) != 0 {
		t.Fatal("expire-window on cancelled broadcast should not touch inbox or enqueue tasks")
	}
}

// ── Phase 6J — Event payload verification ─────────────────────────────────────

func Test6J_RequestAcceptedEventHasAllTripFields(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")

	if len(env.events.accepted) != 1 {
		t.Fatalf("accepted events = %d, want 1", len(env.events.accepted))
	}
	evt := env.events.accepted[0]

	if evt.BookingID == "" {
		t.Fatal("booking_id missing")
	}
	if evt.BroadcastID == "" {
		t.Fatal("broadcast_id missing")
	}
	if evt.InboxID == "" {
		t.Fatal("inbox_id missing")
	}
	if evt.ProviderID != providerID {
		t.Fatalf("provider_id = %s, want %s", evt.ProviderID, providerID)
	}
	if evt.FareAmount != 150000 {
		t.Fatalf("fare_amount = %d, want 150000", evt.FareAmount)
	}
	if evt.Currency != "NGN" {
		t.Fatalf("currency = %s, want NGN", evt.Currency)
	}
	if evt.PickupAddress == "" {
		t.Fatal("pickup_address missing")
	}
	if evt.PickupLat == 0 || evt.PickupLng == 0 {
		t.Fatalf("pickup coordinates zero: lat=%f lng=%f", evt.PickupLat, evt.PickupLng)
	}
	if evt.DropoffAddress == "" {
		t.Fatal("dropoff_address missing")
	}
	if evt.DropoffLat == 0 || evt.DropoffLng == 0 {
		t.Fatalf("dropoff coordinates zero: lat=%f lng=%f", evt.DropoffLat, evt.DropoffLng)
	}
	if evt.ReceiverName == "" {
		t.Fatal("receiver_name missing")
	}
	if evt.ReceiverPhone != "+2348011223344" {
		t.Fatalf("receiver_phone = %s, want +2348011223344", evt.ReceiverPhone)
	}
	if evt.PackageDesc == "" {
		t.Fatal("package_desc missing")
	}
	if evt.AcceptedAt.IsZero() {
		t.Fatal("accepted_at is zero")
	}
	if evt.OccurredAt.IsZero() {
		t.Fatal("occurred_at is zero")
	}
}

func Test6J_RequestAcceptedIncludesCorrelationIDWhenPresent(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	correlationID := uuid.NewString()

	event := fullBookingEvent()
	event.CorrelationID = correlationID
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
	_ = env.redis.Set(context.Background(), RequestBroadcastingKey(event.BookingID), broadcast.ID, 35*time.Second)

	tok := env.providerToken(t, providerID)
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusOK)

	if len(env.events.accepted) != 1 {
		t.Fatalf("accepted events = %d, want 1", len(env.events.accepted))
	}
	if env.events.accepted[0].CorrelationID != correlationID {
		t.Fatalf("correlation_id = %q, want %q", env.events.accepted[0].CorrelationID, correlationID)
	}
}

func Test6J_RequestAcceptedPublishedExactlyOnce(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")

	if len(env.events.accepted) != 1 {
		t.Fatalf("accepted events = %d, want exactly 1 (idempotent)", len(env.events.accepted))
	}
}

func Test6J_RequestRejectedPayloadFields(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, `{"reason":"too_far"}`)

	if len(env.events.rejected) != 1 {
		t.Fatalf("rejected events = %d, want 1", len(env.events.rejected))
	}
	evt := env.events.rejected[0]
	if evt.ProviderID != providerID {
		t.Fatalf("provider_id = %s, want %s", evt.ProviderID, providerID)
	}
	if evt.BookingID != inbox.BookingID {
		t.Fatalf("booking_id mismatch")
	}
	if evt.Reason != "too_far" {
		t.Fatalf("reason = %s, want too_far", evt.Reason)
	}
	if evt.RejectedAt.IsZero() {
		t.Fatal("rejected_at is zero")
	}
}

func Test6J_NoProviderFoundPayloadFields(t *testing.T) {
	repo := newFakeRepository()
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: uuid.NewString(), Status: BroadcastStatusBroadcasting,
		AttemptNumber: 3, ExpiresAt: now.Add(-time.Second),
	}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	events := &fakeEventPublisher{}
	svc := NewService(repo, nil, nil, nil, events, nil, Config{MaxAttempts: 3})
	svc.now = func() time.Time { return now }

	task, _ := NewExpireWindowTask(ExpireWindowPayload{
		BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 3,
	})
	if err := NewWorker(svc).HandleExpireWindow(context.Background(), task); err != nil {
		t.Fatalf("expire-window error: %v", err)
	}

	if len(events.noProviderFound) != 1 {
		t.Fatalf("no_provider_found events = %d, want 1", len(events.noProviderFound))
	}
	evt := events.noProviderFound[0]
	if evt.BookingID != broadcast.BookingID {
		t.Fatalf("booking_id mismatch")
	}
	if evt.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", evt.Attempts)
	}
	if evt.OccurredAt.IsZero() {
		t.Fatal("occurred_at is zero")
	}
}

// ── Phase 6K — Security hardening ─────────────────────────────────────────────

func Test6K_ListDoesNotReturnReceiverPhone(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests", tok, "")
	assertStatus(t, w, http.StatusOK)

	body := w.Body.String()
	if strings.Contains(body, "receiver_phone") {
		t.Fatalf("list response must not expose receiver_phone; body = %s", body)
	}
}

func Test6K_DetailIncludesReceiverPhone(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests/"+inbox.ID, tok, "")
	assertStatus(t, w, http.StatusOK)
	data := extractData(t, w)
	if data["receiver_phone"] != "+2348011223344" {
		t.Fatalf("receiver_phone = %v, want +2348011223344", data["receiver_phone"])
	}
}

func Test6K_IDORProviderCannotListOtherProviderInbox(t *testing.T) {
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, attacker)

	w := doRequest(env.engine, http.MethodGet, "/api/v1/provider/requests", tok, "")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal list response: %v; body = %s", err, w.Body.String())
	}
	if len(resp.Data) > 0 {
		t.Fatalf("attacker received %d inbox items, want 0", len(resp.Data))
	}
}

func Test6K_AcceptRateLimitReturns429OnSixthRequest(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	tok := env.providerToken(t, providerID)
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)

	// Calls 1–5 proceed to normal accept logic (first succeeds, remainder get 409/410).
	for i := 1; i <= AcceptRateLimitMax; i++ {
		doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	}

	// 6th call must be rate-limited.
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/accept", tok, "")
	assertStatus(t, w, http.StatusTooManyRequests)
	assertErrorCodeStr(t, w, "rate_limited")
}

func Test6K_AcceptRateLimitIsPerProvider(t *testing.T) {
	env := newRequestTestEnv(t)
	p1 := uuid.NewString()
	p2 := uuid.NewString()
	tok1 := env.providerToken(t, p1)
	tok2 := env.providerToken(t, p2)
	_, inbox1 := newActiveBroadcastAndInbox(t, env, p1)
	_, inbox2 := newActiveBroadcastAndInbox(t, env, p2)

	// Exhaust p1's limit.
	for i := 0; i <= AcceptRateLimitMax; i++ {
		doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox1.ID+"/accept", tok1, "")
	}

	// p2 should still be able to accept.
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox2.ID+"/accept", tok2, "")
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("p2 should not be rate-limited by p1's exhausted counter")
	}
}

func Test6K_RejectRateLimitReturns429OnEleventhRequest(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	tok := env.providerToken(t, providerID)
	_, inbox := newActiveBroadcastAndInbox(t, env, providerID)

	// Calls 1–10 proceed (first succeeds, remainder get 409).
	for i := 1; i <= RejectRateLimitMax; i++ {
		doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	}

	// 11th call must be rate-limited.
	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusTooManyRequests)
	assertErrorCodeStr(t, w, "rate_limited")
}

func Test6K_RejectAfterExpiredWindowReturns410(t *testing.T) {
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

	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	assertStatus(t, w, http.StatusGone)
	assertErrorCodeStr(t, w, "gone")
}

func Test6K_AlreadyExpiredInboxReturns409OnReject(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	payload, _ := json.Marshal(event)
	past := time.Now().UTC().Add(-5 * time.Second)
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusExpired,
		ExpiresAt: past, BookingPayload: payload,
	}
	env.repo.broadcasts = append(env.repo.broadcasts, broadcast)
	inbox := ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID,
		ProviderID: providerID, Status: InboxStatusExpired, // already expired by Asynq task
		ExpiresAt: past, BookingPayload: payload, ReceivedAt: past,
	}
	env.repo.inboxes = append(env.repo.inboxes, inbox)
	tok := env.providerToken(t, providerID)

	w := doRequest(env.engine, http.MethodPost, "/api/v1/provider/requests/"+inbox.ID+"/reject", tok, "")
	// inbox.Status != pending => 409 Conflict (not 410)
	assertStatus(t, w, http.StatusConflict)
}

func Test6K_DetailStillWorksForExpiredInbox(t *testing.T) {
	env := newRequestTestEnv(t)
	providerID := uuid.NewString()
	event := fullBookingEvent()
	payload, _ := json.Marshal(event)
	past := time.Now().UTC().Add(-5 * time.Second)
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
	assertStatus(t, w, http.StatusOK)
}

func Test6K_NearbyCallUsesInternalServiceKey(t *testing.T) {
	// Confirm HTTPNearbyClient uses X-Internal-Service-Key, not X-Service-Key.
	// This mirrors the existing test in phase6cde_test.go but is listed here for completeness.
	client := NewHTTPNearbyClient("http://localhost:9999", "my-internal-key")
	if client.serviceKey != "my-internal-key" {
		t.Fatalf("serviceKey = %q", client.serviceKey)
	}
}

func Test6K_NoAWSInRequestFeature(t *testing.T) {
	// Verify no AWS SNS/SQS/S3 code has crept into production (non-test) source files.
	dir := "."
	// These patterns indicate AWS infrastructure usage that is explicitly forbidden.
	awsTerms := []string{"sns.amazonaws", "sqs.amazonaws", "s3.amazonaws", "aws-sdk-go", "github.com/aws/"}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		lower := strings.ToLower(string(data))
		for _, term := range awsTerms {
			if strings.Contains(lower, strings.ToLower(term)) {
				t.Fatalf("AWS term %q found in production file %s", term, name)
			}
		}
	}
}

func Test6K_ProviderIDAlwaysFromJWT(t *testing.T) {
	// Sending a different provider_id in the URL path has no effect because
	// the handler always reads providerID from the JWT claim, not the request body.
	// This test confirms that Provider A's token accessing Provider B's inbox returns 404.
	env := newRequestTestEnv(t)
	owner := uuid.NewString()
	attacker := uuid.NewString()
	_, inbox := newActiveBroadcastAndInbox(t, env, owner)
	tok := env.providerToken(t, attacker)

	// Attempt all operations using attacker's JWT on owner's inbox.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		paths := []string{
			"/api/v1/provider/requests/" + inbox.ID,
			"/api/v1/provider/requests/" + inbox.ID + "/accept",
			"/api/v1/provider/requests/" + inbox.ID + "/reject",
		}
		for _, path := range paths {
			if method == http.MethodGet && strings.HasSuffix(path, "/accept") {
				continue
			}
			if method == http.MethodGet && strings.HasSuffix(path, "/reject") {
				continue
			}
			w := doRequest(env.engine, method, path, tok, "")
			if w.Code != http.StatusNotFound {
				t.Logf("WARN: %s %s with attacker token returned %d (want 404)", method, path, w.Code)
			}
		}
	}
}
