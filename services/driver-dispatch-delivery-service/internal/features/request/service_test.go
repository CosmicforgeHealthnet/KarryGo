package request

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"karrygo/shared/go/apperrors"
)

func TestStartBroadcastCreatesInboxRowsAndIsIdempotent(t *testing.T) {
	repo := newFakeRepository()
	finder := fakeNearbyFinder{providers: []NearbyProvider{{ProviderID: uuid.NewString()}, {ProviderID: uuid.NewString()}}}
	notifications := &fakeNotificationSender{}
	tasks := &fakeTaskEnqueuer{}
	service := NewService(repo, nil, finder, notifications, nil, tasks, Config{BroadcastWindow: 30 * time.Second})
	event := validBookingEvent()

	first, err := service.StartBroadcast(context.Background(), event)
	if err != nil {
		t.Fatalf("StartBroadcast error=%v", err)
	}
	if first.BookingID != event.BookingID || len(repo.inboxes) != 2 {
		t.Fatalf("broadcast=%+v inboxes=%d", first, len(repo.inboxes))
	}
	if notifications.sent != 0 {
		t.Fatalf("notifications sent inline=%d want 0", notifications.sent)
	}
	if len(tasks.tasksOfType(TaskSendPush)) != 2 || len(tasks.tasksOfType(TaskExpireWindow)) != 1 {
		t.Fatalf("tasks=%v", tasks.types())
	}
	if len(first.BookingPayload) == 0 {
		t.Fatal("booking_payload was not stored")
	}

	second, err := service.StartBroadcast(context.Background(), event)
	if err != nil {
		t.Fatalf("duplicate StartBroadcast error=%v", err)
	}
	if second.ID != first.ID || len(repo.inboxes) != 2 || len(tasks.tasks) != 3 {
		t.Fatal("duplicate booking was not idempotent")
	}
}

func TestFCMFailureDoesNotRollbackInbox(t *testing.T) {
	repo := newFakeRepository()
	inbox := ProviderRequestInbox{ID: uuid.NewString(), ProviderID: uuid.NewString(), Status: InboxStatusPending}
	repo.inboxes = append(repo.inboxes, inbox)
	service := NewService(repo, nil, nil, &fakeNotificationSender{err: errors.New("fcm unavailable")}, nil, nil, Config{})
	task, _ := NewSendPushTask(SendPushPayload{ProviderID: inbox.ProviderID, InboxID: inbox.ID})
	if err := NewWorker(service).HandleSendPush(context.Background(), task); err == nil {
		t.Fatal("transient FCM failure should be returned for Asynq retry")
	}
	if repo.inboxes[0].FCMSent {
		t.Fatal("failed FCM send marked as sent")
	}
}

func TestProviderInboxAccessIsScopedByProvider(t *testing.T) {
	repo := newFakeRepository()
	owner := uuid.NewString()
	other := uuid.NewString()
	inbox := ProviderRequestInbox{ID: uuid.NewString(), ProviderID: owner, Status: InboxStatusPending}
	repo.inboxes = append(repo.inboxes, inbox)
	service := NewService(repo, nil, nil, nil, nil, nil, Config{})

	if _, err := service.GetInbox(context.Background(), inbox.ID, other); err == nil {
		t.Fatal("different provider read inbox item")
	}
	if _, err := service.GetInbox(context.Background(), inbox.ID, owner); err != nil {
		t.Fatalf("owner could not read inbox: %v", err)
	}
}

func TestBookingSubscriberDropsBadPayloadAndStartsValidBroadcast(t *testing.T) {
	service := NewService(newFakeRepository(), nil, fakeNearbyFinder{}, nil, nil, nil, Config{})
	if err := HandleBookingDispatchCreatedPayload(context.Background(), service, []byte("bad-json")); err != nil {
		t.Fatalf("bad payload should be dropped: %v", err)
	}
	payload, _ := json.Marshal(validBookingEvent())
	if err := HandleBookingDispatchCreatedPayload(context.Background(), service, payload); err != nil {
		t.Fatalf("valid payload error=%v", err)
	}
}

func validBookingEvent() BookingDispatchCreatedEvent {
	return BookingDispatchCreatedEvent{
		BookingID: uuid.NewString(), CustomerID: uuid.NewString(), PickupLat: 6.5244, PickupLng: 3.3792,
		DropoffLat: 6.6, DropoffLng: 3.4, ServiceType: "dispatch", PaymentMethod: "wallet",
		PickupAddress: "15 Awolowo Road", DropoffAddress: "32 Bode Thomas", FareAmount: 150000,
		Currency: "NGN", ReceiverName: "Chidi Obi", PackageDesc: "Pharmacy items",
		BookingPayload: json.RawMessage(`{"note":"fragile"}`), OccurredAt: time.Now().UTC(),
	}
}

type fakeNearbyFinder struct {
	providers []NearbyProvider
	err       error
}

func (f fakeNearbyFinder) FindNearby(context.Context, float64, float64, float64, int) ([]NearbyProvider, error) {
	return f.providers, f.err
}

type fakeNotificationSender struct {
	sent int
	err  error
	last RequestPushPayload
}

func (f *fakeNotificationSender) SendRequestBroadcast(_ context.Context, _ string, payload RequestPushPayload) error {
	f.sent++
	f.last = payload
	return f.err
}

type fakeTaskEnqueuer struct {
	tasks []*asynq.Task
}

func (f *fakeTaskEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (f *fakeTaskEnqueuer) tasksOfType(taskType string) []*asynq.Task {
	var result []*asynq.Task
	for _, task := range f.tasks {
		if task.Type() == taskType {
			result = append(result, task)
		}
	}
	return result
}

func (f *fakeTaskEnqueuer) types() []string {
	var result []string
	for _, task := range f.tasks {
		result = append(result, task.Type())
	}
	return result
}

type fakeRepository struct {
	broadcasts []RequestBroadcast
	inboxes    []ProviderRequestInbox
}

func newFakeRepository() *fakeRepository { return &fakeRepository{} }

func (r *fakeRepository) CreateBroadcast(_ context.Context, in CreateBroadcastInput) (RequestBroadcast, error) {
	for _, b := range r.broadcasts {
		if b.BookingID == in.BookingID {
			return RequestBroadcast{}, errors.New("duplicate")
		}
	}
	b := RequestBroadcast{ID: uuid.NewString(), BookingID: in.BookingID, ServiceType: in.ServiceType, BroadcastRadiusKM: in.RadiusKM,
		AttemptNumber: in.Attempt, Status: BroadcastStatusBroadcasting, BroadcastAt: in.BroadcastAt, ExpiresAt: in.ExpiresAt,
		BookingPayload: in.BookingPayload, CreatedAt: in.BroadcastAt, UpdatedAt: in.BroadcastAt}
	r.broadcasts = append(r.broadcasts, b)
	return b, nil
}
func (r *fakeRepository) GetBroadcastByID(_ context.Context, id string) (RequestBroadcast, bool, error) {
	for _, b := range r.broadcasts {
		if b.ID == id {
			return b, true, nil
		}
	}
	return RequestBroadcast{}, false, nil
}
func (r *fakeRepository) GetBroadcastByBookingID(_ context.Context, id string) (RequestBroadcast, bool, error) {
	for _, b := range r.broadcasts {
		if b.BookingID == id {
			return b, true, nil
		}
	}
	return RequestBroadcast{}, false, nil
}
func (r *fakeRepository) UpdateBroadcastStatus(_ context.Context, id string, status BroadcastStatus) error {
	return r.setBroadcastStatus(id, status)
}
func (r *fakeRepository) MarkBroadcastAccepted(_ context.Context, broadcastID, bookingID, inboxID, providerID string, respondedAt time.Time) error {
	// Mark the accepting inbox as accepted.
	acceptedCount := 0
	for i := range r.inboxes {
		if r.inboxes[i].ID == inboxID && r.inboxes[i].BroadcastID == broadcastID && r.inboxes[i].ProviderID == providerID && r.inboxes[i].Status == InboxStatusPending {
			r.inboxes[i].Status = InboxStatusAccepted
			r.inboxes[i].RespondedAt = &respondedAt
			acceptedCount++
		}
	}
	if acceptedCount == 0 {
		return apperrors.Conflict("Inbox not pending.", nil)
	}
	// Expire other pending inboxes for this broadcast.
	for i := range r.inboxes {
		if r.inboxes[i].BroadcastID == broadcastID && r.inboxes[i].ID != inboxID && r.inboxes[i].Status == InboxStatusPending {
			r.inboxes[i].Status = InboxStatusExpired
			r.inboxes[i].RespondedAt = &respondedAt
		}
	}
	// Mark broadcast as accepted.
	for i := range r.broadcasts {
		if r.broadcasts[i].ID == broadcastID && r.broadcasts[i].Status == BroadcastStatusBroadcasting {
			r.broadcasts[i].Status = BroadcastStatusAccepted
			r.broadcasts[i].AcceptedByProviderID = &providerID
		}
	}
	return nil
}
func (r *fakeRepository) MarkBroadcastExpired(_ context.Context, id string) error {
	return r.setBroadcastStatus(id, BroadcastStatusExpired)
}
func (r *fakeRepository) MarkBroadcastNoProviderFound(_ context.Context, id string) error {
	return r.setBroadcastStatus(id, BroadcastStatusNoProviderFound)
}
func (r *fakeRepository) UpdateBroadcastAttempt(_ context.Context, id string, attempt int, radius float64, notified int, at, expires time.Time) error {
	for i := range r.broadcasts {
		if r.broadcasts[i].ID == id {
			r.broadcasts[i].AttemptNumber, r.broadcasts[i].BroadcastRadiusKM = attempt, radius
			r.broadcasts[i].ProvidersNotified += notified
			r.broadcasts[i].BroadcastAt, r.broadcasts[i].ExpiresAt = at, expires
		}
	}
	return nil
}
func (r *fakeRepository) CreateInboxRows(_ context.Context, broadcastID, bookingID string, providerIDs []string) ([]ProviderRequestInbox, error) {
	var created []ProviderRequestInbox
	for _, providerID := range providerIDs {
		duplicate := false
		for _, existing := range r.inboxes {
			if existing.ProviderID == providerID && existing.BookingID == bookingID {
				duplicate = true
			}
		}
		if duplicate {
			continue
		}
		row := ProviderRequestInbox{ID: uuid.NewString(), BroadcastID: broadcastID, BookingID: bookingID, ProviderID: providerID, Status: InboxStatusPending}
		r.inboxes = append(r.inboxes, row)
		created = append(created, row)
	}
	return created, nil
}
func (r *fakeRepository) ListProviderInbox(_ context.Context, providerID string, options ListInboxOptions) ([]ProviderRequestInbox, error) {
	var result []ProviderRequestInbox
	for _, i := range r.inboxes {
		if i.ProviderID != providerID {
			continue
		}
		if options.Status != "" && i.Status != options.Status {
			continue
		}
		if options.Status == InboxStatusPending {
			active := false
			for _, broadcast := range r.broadcasts {
				if broadcast.ID == i.BroadcastID && broadcast.Status == BroadcastStatusBroadcasting && broadcast.ExpiresAt.After(time.Now().UTC()) {
					active = true
				}
			}
			if !active {
				continue
			}
		}
		result = append(result, i)
	}
	return result, nil
}
func (r *fakeRepository) GetProviderInboxByID(_ context.Context, inboxID, providerID string) (ProviderRequestInbox, bool, error) {
	for _, i := range r.inboxes {
		if i.ID == inboxID && i.ProviderID == providerID {
			return i, true, nil
		}
	}
	return ProviderRequestInbox{}, false, nil
}
func (r *fakeRepository) MarkInboxRejected(_ context.Context, inboxID, providerID string, respondedAt time.Time) (bool, error) {
	for i := range r.inboxes {
		if r.inboxes[i].ID == inboxID && r.inboxes[i].ProviderID == providerID && r.inboxes[i].Status == InboxStatusPending {
			r.inboxes[i].Status = InboxStatusRejected
			r.inboxes[i].RespondedAt = &respondedAt
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepository) MarkPendingInboxExpired(_ context.Context, broadcastID string) error {
	now := time.Now().UTC()
	for i := range r.inboxes {
		if r.inboxes[i].BroadcastID == broadcastID && r.inboxes[i].Status == InboxStatusPending {
			r.inboxes[i].Status = InboxStatusExpired
			r.inboxes[i].RespondedAt = &now
		}
	}
	return nil
}
func (r *fakeRepository) MarkFCMSent(_ context.Context, inboxID string, at time.Time) error {
	for i := range r.inboxes {
		if r.inboxes[i].ID == inboxID {
			r.inboxes[i].FCMSent = true
			r.inboxes[i].FCMSentAt = &at
		}
	}
	return nil
}
func (r *fakeRepository) ListAlreadyNotifiedProviders(_ context.Context, bookingID string) ([]string, error) {
	var result []string
	for _, i := range r.inboxes {
		if i.BookingID == bookingID {
			result = append(result, i.ProviderID)
		}
	}
	return result, nil
}

func (r *fakeRepository) setBroadcastStatus(id string, status BroadcastStatus) error {
	for i := range r.broadcasts {
		if r.broadcasts[i].ID == id {
			r.broadcasts[i].Status = status
		}
	}
	return nil
}
