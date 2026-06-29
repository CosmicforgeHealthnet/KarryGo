package request

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestBroadcastFlowEnqueuesPushAndExpiryAndSetsRedisWindow(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	repo := newFakeRepository()
	providerID := uuid.NewString()
	nearby := &recordingNearbyFinder{providers: []NearbyProvider{{ProviderID: providerID, DistanceKM: 1.25}}}
	tasks := &fakeTaskEnqueuer{}
	now := time.Now().UTC().Truncate(time.Second)
	service := NewService(repo, redisClient, nearby, &fakeNotificationSender{}, nil, tasks, Config{BroadcastWindow: 30 * time.Second})
	service.now = func() time.Time { return now }

	event := validBookingEvent()
	broadcast, err := service.StartBroadcast(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if len(nearby.calls) != 1 || nearby.calls[0].radius != 5 || nearby.calls[0].limit != 20 {
		t.Fatalf("nearby calls=%+v", nearby.calls)
	}
	if broadcast.ProvidersNotified != 1 || repo.broadcasts[0].ProvidersNotified != 1 {
		t.Fatalf("providers_notified broadcast=%d repo=%d", broadcast.ProvidersNotified, repo.broadcasts[0].ProvidersNotified)
	}
	if len(tasks.tasksOfType(TaskSendPush)) != 1 || len(tasks.tasksOfType(TaskExpireWindow)) != 1 {
		t.Fatalf("tasks=%v", tasks.types())
	}
	var push SendPushPayload
	if err := json.Unmarshal(tasks.tasksOfType(TaskSendPush)[0].Payload(), &push); err != nil {
		t.Fatal(err)
	}
	if push.ProviderID != providerID || push.FareAmount != event.FareAmount || push.DistanceKM != 1.25 {
		t.Fatalf("push=%+v", push)
	}
	var stored BookingDispatchCreatedEvent
	if err := json.Unmarshal(repo.broadcasts[0].BookingPayload, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.PickupAddress != event.PickupAddress || stored.PackageDesc != event.PackageDesc || stored.FareAmount != event.FareAmount {
		t.Fatalf("stored booking payload lost summary: %+v", stored)
	}
	if got, err := redisServer.Get(RequestBroadcastingKey(event.BookingID)); err != nil || got != broadcast.ID {
		t.Fatalf("broadcasting key=%q want %q", got, broadcast.ID)
	}
	if ttl := redisServer.TTL(RequestBroadcastingKey(event.BookingID)); ttl != 35*time.Second {
		t.Fatalf("broadcasting TTL=%s want 35s", ttl)
	}

	if _, err := service.StartBroadcast(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(tasks.tasks) != 2 || len(repo.inboxes) != 1 {
		t.Fatal("duplicate event created tasks or inbox rows")
	}
}

func TestNoNearbyProvidersStillSchedulesWindow(t *testing.T) {
	repo := newFakeRepository()
	tasks := &fakeTaskEnqueuer{}
	service := NewService(repo, nil, &recordingNearbyFinder{}, nil, nil, tasks, Config{})
	if _, err := service.StartBroadcast(context.Background(), validBookingEvent()); err != nil {
		t.Fatal(err)
	}
	if len(repo.broadcasts) != 1 || len(repo.inboxes) != 0 || len(tasks.tasksOfType(TaskExpireWindow)) != 1 {
		t.Fatalf("broadcasts=%d inboxes=%d tasks=%v", len(repo.broadcasts), len(repo.inboxes), tasks.types())
	}
}

func TestNearbyFailureStillSchedulesRetryWindow(t *testing.T) {
	repo := newFakeRepository()
	tasks := &fakeTaskEnqueuer{}
	service := NewService(repo, nil, &recordingNearbyFinder{err: errors.New("nearby unavailable")}, nil, nil, tasks, Config{})
	if _, err := service.StartBroadcast(context.Background(), validBookingEvent()); err != nil {
		t.Fatal(err)
	}
	if len(repo.broadcasts) != 1 || len(repo.inboxes) != 0 || len(tasks.tasksOfType(TaskExpireWindow)) != 1 {
		t.Fatalf("broadcasts=%d inboxes=%d tasks=%v", len(repo.broadcasts), len(repo.inboxes), tasks.types())
	}
}

func TestHTTPNearbyClientUsesInternalServiceKeyAndExpectedQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Service-Key") != "internal-secret" || r.Header.Get("X-Service-Key") != "" {
			t.Fatalf("unexpected service key headers: %+v", r.Header)
		}
		if r.URL.Query().Get("lat") != "6.5244" || r.URL.Query().Get("lng") != "3.3792" ||
			r.URL.Query().Get("radius") != "8" || r.URL.Query().Get("limit") != "20" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[],"count":0,"radius_km":8}}`))
	}))
	defer server.Close()

	if _, err := NewHTTPNearbyClient(server.URL, "internal-secret").FindNearby(context.Background(), 6.5244, 3.3792, 8, 20); err != nil {
		t.Fatal(err)
	}
}

func TestSendPushWorkerMarksSuccessAndHandlesMissingToken(t *testing.T) {
	repo := newFakeRepository()
	inboxID := uuid.NewString()
	repo.inboxes = append(repo.inboxes, ProviderRequestInbox{ID: inboxID})
	sender := &fakeNotificationSender{}
	worker := NewWorker(NewService(repo, nil, nil, sender, nil, nil, Config{}))
	task, _ := NewSendPushTask(SendPushPayload{ProviderID: uuid.NewString(), InboxID: inboxID, FareAmount: 150000})
	if err := worker.HandleSendPush(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !repo.inboxes[0].FCMSent || sender.last.Type != "new_request" || sender.last.FareAmount != 150000 {
		t.Fatalf("inbox=%+v push=%+v", repo.inboxes[0], sender.last)
	}

	repo.inboxes[0].FCMSent = false
	sender.err = ErrNoFCMToken
	if err := worker.HandleSendPush(context.Background(), task); err != nil {
		t.Fatalf("missing token should be controlled: %v", err)
	}
	if repo.inboxes[0].FCMSent {
		t.Fatal("missing token marked FCM sent")
	}
}

func TestExpireWindowIgnoresStaleAndEnqueuesExpandedRebroadcast(t *testing.T) {
	now := time.Now().UTC()
	event := validBookingEvent()
	payload, _ := json.Marshal(event)
	repo := newFakeRepository()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		AttemptNumber: 1, BroadcastRadiusKM: 5, ExpiresAt: now.Add(-time.Second), BookingPayload: payload,
	}
	repo.broadcasts = append(repo.broadcasts, broadcast)
	repo.inboxes = append(repo.inboxes, ProviderRequestInbox{
		ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, Status: InboxStatusPending,
	})
	tasks := &fakeTaskEnqueuer{}
	service := NewService(repo, nil, nil, nil, nil, tasks, Config{MaxAttempts: 3})
	service.now = func() time.Time { return now }
	worker := NewWorker(service)

	stale, _ := NewExpireWindowTask(ExpireWindowPayload{BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 2})
	if err := worker.HandleExpireWindow(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if repo.inboxes[0].Status != InboxStatusPending || len(tasks.tasks) != 0 {
		t.Fatal("stale expiry task changed current attempt")
	}

	current, _ := NewExpireWindowTask(ExpireWindowPayload{BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 1})
	if err := worker.HandleExpireWindow(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if repo.inboxes[0].Status != InboxStatusExpired || len(tasks.tasksOfType(TaskReBroadcast)) != 1 {
		t.Fatalf("inbox=%+v tasks=%v", repo.inboxes[0], tasks.types())
	}
	var next ReBroadcastPayload
	_ = json.Unmarshal(tasks.tasksOfType(TaskReBroadcast)[0].Payload(), &next)
	if next.AttemptNumber != 2 || next.NewRadiusKM != 8 {
		t.Fatalf("next=%+v", next)
	}
}

func TestExpireWindowIgnoresTerminalBroadcasts(t *testing.T) {
	for _, status := range []BroadcastStatus{BroadcastStatusAccepted, BroadcastStatusCancelled, BroadcastStatusNoProviderFound} {
		t.Run(string(status), func(t *testing.T) {
			repo := newFakeRepository()
			broadcast := RequestBroadcast{
				ID: uuid.NewString(), BookingID: uuid.NewString(), Status: status,
				AttemptNumber: 1, ExpiresAt: time.Now().UTC().Add(-time.Second),
			}
			repo.broadcasts = append(repo.broadcasts, broadcast)
			repo.inboxes = append(repo.inboxes, ProviderRequestInbox{
				ID: uuid.NewString(), BroadcastID: broadcast.ID, Status: InboxStatusPending,
			})
			tasks := &fakeTaskEnqueuer{}
			task, _ := NewExpireWindowTask(ExpireWindowPayload{
				BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 1,
			})
			if err := NewWorker(NewService(repo, nil, nil, nil, nil, tasks, Config{})).HandleExpireWindow(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if repo.inboxes[0].Status != InboxStatusPending || len(tasks.tasks) != 0 {
				t.Fatalf("terminal broadcast changed inbox or tasks: inbox=%+v tasks=%v", repo.inboxes[0], tasks.types())
			}
		})
	}
}

func TestMaxAttemptPublishesNoProviderFoundAndDeletesWindow(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	now := time.Now().UTC()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: uuid.NewString(), Status: BroadcastStatusBroadcasting,
		AttemptNumber: 3, BroadcastRadiusKM: 11, ExpiresAt: now.Add(-time.Second),
	}
	repo := newFakeRepository()
	repo.broadcasts = append(repo.broadcasts, broadcast)
	redisServer.Set(RequestBroadcastingKey(broadcast.BookingID), broadcast.ID)
	events := &fakeEventPublisher{}
	service := NewService(repo, redisClient, nil, nil, events, nil, Config{MaxAttempts: 3})
	service.now = func() time.Time { return now }
	task, _ := NewExpireWindowTask(ExpireWindowPayload{BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 3})
	if err := NewWorker(service).HandleExpireWindow(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if repo.broadcasts[0].Status != BroadcastStatusNoProviderFound || len(events.noProviderFound) != 1 {
		t.Fatalf("broadcast=%+v events=%+v", repo.broadcasts[0], events.noProviderFound)
	}
	if redisServer.Exists(RequestBroadcastingKey(broadcast.BookingID)) {
		t.Fatal("broadcasting key remains after no_provider_found")
	}
}

func TestRebroadcastExcludesAlreadyNotifiedAndIsRetrySafe(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	now := time.Now().UTC()
	event := validBookingEvent()
	payload, _ := json.Marshal(event)
	oldProvider, newProvider := uuid.NewString(), uuid.NewString()
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		AttemptNumber: 1, BroadcastRadiusKM: 5, ExpiresAt: now.Add(-time.Second), BookingPayload: payload,
	}
	repo := newFakeRepository()
	repo.broadcasts = append(repo.broadcasts, broadcast)
	repo.inboxes = append(repo.inboxes, ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID, ProviderID: oldProvider, Status: InboxStatusExpired})
	tasks := &fakeTaskEnqueuer{}
	nearby := &recordingNearbyFinder{providers: []NearbyProvider{{ProviderID: oldProvider}, {ProviderID: newProvider}}}
	service := NewService(repo, redisClient, nearby, nil, nil, tasks, Config{BroadcastWindow: 30 * time.Second})
	service.now = func() time.Time { return now }
	task, _ := NewReBroadcastTask(ReBroadcastPayload{BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: 2, NewRadiusKM: 8})
	worker := NewWorker(service)
	if err := worker.HandleReBroadcast(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(repo.inboxes) != 2 || repo.inboxes[1].ProviderID != newProvider || repo.broadcasts[0].AttemptNumber != 2 || repo.broadcasts[0].BroadcastRadiusKM != 8 {
		t.Fatalf("broadcast=%+v inboxes=%+v", repo.broadcasts[0], repo.inboxes)
	}
	if len(tasks.tasksOfType(TaskSendPush)) != 1 || len(tasks.tasksOfType(TaskExpireWindow)) != 1 {
		t.Fatalf("tasks=%v", tasks.types())
	}
	if err := worker.HandleReBroadcast(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(repo.inboxes) != 2 || len(tasks.tasks) != 2 {
		t.Fatal("rebroadcast retry duplicated inboxes or tasks")
	}
}

func TestProviderInboxListReturnsOwnActiveBookingSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC().Truncate(time.Second)
	providerID := uuid.NewString()
	event := validBookingEvent()
	payload, _ := json.Marshal(event)
	broadcast := RequestBroadcast{
		ID: uuid.NewString(), BookingID: event.BookingID, Status: BroadcastStatusBroadcasting,
		ExpiresAt: now.Add(20 * time.Second), BookingPayload: payload,
	}
	repo := newFakeRepository()
	repo.broadcasts = append(repo.broadcasts, broadcast)
	repo.inboxes = append(repo.inboxes,
		ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID, ProviderID: providerID, Status: InboxStatusPending, ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now},
		ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: event.BookingID, ProviderID: uuid.NewString(), Status: InboxStatusPending, ExpiresAt: broadcast.ExpiresAt, BookingPayload: payload, ReceivedAt: now},
	)
	service := NewService(repo, nil, nil, nil, nil, nil, Config{})
	service.now = func() time.Time { return now }
	tokens := authusecases.NewTokenUsecase([]byte("request-list-secret"), time.Hour, time.Hour)
	token, _, err := tokens.GenerateAccessToken(providerID, "+2348012345678", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterRoutes(engine, tokens, NewHandler(service))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/requests", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []ProviderRequestInboxItem `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].FareAmount != event.FareAmount ||
		response.Data[0].PickupAddress != event.PickupAddress || response.Data[0].RemainingSeconds != 20 {
		t.Fatalf("data=%+v", response.Data)
	}
}

func TestProviderInboxListExcludesExpiredAndTerminalBroadcasts(t *testing.T) {
	now := time.Now().UTC()
	providerID := uuid.NewString()
	event := validBookingEvent()
	payload, _ := json.Marshal(event)
	repo := newFakeRepository()
	for _, spec := range []struct {
		status  BroadcastStatus
		expires time.Time
	}{
		{BroadcastStatusBroadcasting, now.Add(-time.Second)},
		{BroadcastStatusAccepted, now.Add(time.Minute)},
		{BroadcastStatusCancelled, now.Add(time.Minute)},
		{BroadcastStatusNoProviderFound, now.Add(time.Minute)},
	} {
		broadcast := RequestBroadcast{
			ID: uuid.NewString(), BookingID: uuid.NewString(), Status: spec.status,
			ExpiresAt: spec.expires, BookingPayload: payload,
		}
		repo.broadcasts = append(repo.broadcasts, broadcast)
		repo.inboxes = append(repo.inboxes, ProviderRequestInbox{
			ID: uuid.NewString(), BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, ProviderID: providerID,
			Status: InboxStatusPending, ExpiresAt: spec.expires, BookingPayload: payload,
		})
	}
	result, err := NewService(repo, nil, nil, nil, nil, nil, Config{}).ListInbox(context.Background(), providerID, ListInboxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("terminal or expired requests returned: %+v", result)
	}
}

func TestAcceptNonPendingInboxReturnsRequestTakenWhenAcceptedMarkerExists(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	providerID := uuid.NewString()
	bookingID := uuid.NewString()
	repo := newFakeRepository()
	repo.inboxes = append(repo.inboxes, ProviderRequestInbox{
		ID: uuid.NewString(), BookingID: bookingID, ProviderID: providerID, Status: InboxStatusExpired,
	})
	if err := redisClient.Set(context.Background(), RequestAcceptedKey(bookingID), uuid.NewString(), time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	_, err := NewService(repo, redisClient, nil, nil, nil, nil, Config{}).Accept(
		context.Background(), repo.inboxes[0].ID, providerID,
	)
	if err == nil || apperrors.From(err).Code != "request_taken" {
		t.Fatalf("err=%v code=%s want request_taken", err, apperrors.From(err).Code)
	}
}

type nearbyCall struct {
	lat, lng, radius float64
	limit            int
}

type recordingNearbyFinder struct {
	providers []NearbyProvider
	err       error
	calls     []nearbyCall
}

func (f *recordingNearbyFinder) FindNearby(_ context.Context, lat, lng, radius float64, limit int) ([]NearbyProvider, error) {
	f.calls = append(f.calls, nearbyCall{lat: lat, lng: lng, radius: radius, limit: limit})
	return f.providers, f.err
}

type fakeEventPublisher struct {
	accepted        []RequestAcceptedEvent
	rejected        []RequestRejectedEvent
	noProviderFound []NoProviderFoundEvent
	err             error
}

func (f *fakeEventPublisher) PublishRequestAccepted(_ context.Context, event RequestAcceptedEvent) error {
	f.accepted = append(f.accepted, event)
	return f.err
}

func (f *fakeEventPublisher) PublishRequestRejected(_ context.Context, event RequestRejectedEvent) error {
	f.rejected = append(f.rejected, event)
	return f.err
}

func (f *fakeEventPublisher) PublishNoProviderFound(_ context.Context, event NoProviderFoundEvent) error {
	f.noProviderFound = append(f.noProviderFound, event)
	return f.err
}
