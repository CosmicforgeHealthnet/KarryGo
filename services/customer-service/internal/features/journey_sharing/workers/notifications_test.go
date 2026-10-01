package shareworkers

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	m "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/models"
	r "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	"cosmicforge/logistics/shared/go/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeOutbox struct {
	intent     r.NotificationIntent
	ack, retry int
}

func (f *fakeOutbox) ClaimNotification(context.Context) (r.NotificationIntent, bool, error) {
	return f.intent, true, nil
}
func (f *fakeOutbox) AcknowledgeNotification(context.Context, string) error { f.ack++; return nil }
func (f *fakeOutbox) RetryNotification(context.Context, r.NotificationIntent) error {
	f.retry++
	return nil
}

type fakeSender struct {
	request notifications.Request
	err     error
}

func (f *fakeSender) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	f.request = request
	return notifications.SendResponse{}, f.err
}

func TestShareNotificationHintRetriesWithoutLeakingCapability(t *testing.T) {
	outbox := &fakeOutbox{intent: r.NotificationIntent{ID: "event-1", SessionID: "share-1", CustomerID: "customer-1", EventType: "share.created"}}
	sender := &fakeSender{err: errors.New("notification unavailable")}
	if err := DeliverOne(context.Background(), outbox, sender); err == nil || outbox.retry != 1 || outbox.ack != 0 {
		t.Fatal("dependency failure must retain the intent for retry")
	}
	if sender.request.IDempotencyKey != "event-1" || sender.request.Data["share_ref"] != "share-1" || sender.request.Data["token"] != nil {
		t.Fatal("notification must contain only stable identity and opaque share ref")
	}
	sender.err = nil
	if err := DeliverOne(context.Background(), outbox, sender); err != nil || outbox.ack != 1 {
		t.Fatal("successful retry must acknowledge the committed intent", err)
	}
}

func TestCommittedShareSurvivesNotificationOutage(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated CUSTOMER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, sessionID, eventID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err = db.Exec(ctx, `INSERT INTO customers(id,email) VALUES($1,$2)`, owner, owner+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO journey_share_sessions(id,customer_id,authority,source_ref,product_code,token_hash,permissions,status,expires_at,version,created_at,updated_at)
	 VALUES($1,$2,'TAXI',$3,'RIDE',$4,'[]','ACTIVE',now()+interval '1 hour',1,now(),now())`, sessionID, owner, uuid.NewString(), m.Digest(uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO journey_share_notification_outbox(id,session_id,customer_id,event_type,available_at) VALUES($1,$2,$3,'share.created',TIMESTAMPTZ '2000-01-01')`, eventID, sessionID, owner); err != nil {
		t.Fatal(err)
	}
	outbox := &r.Postgres{DB: db}
	sender := &fakeSender{err: errors.New("notification offline")}
	if err = DeliverOne(ctx, outbox, sender); err == nil {
		t.Fatal("outage not observed")
	}
	var status, delivered string
	if err = db.QueryRow(ctx, `SELECT status FROM journey_share_sessions WHERE id=$1`, sessionID).Scan(&status); err != nil || status != "ACTIVE" {
		t.Fatal("business state rolled back", status, err)
	}
	if err = db.QueryRow(ctx, `SELECT COALESCE(delivered_at::text,'') FROM journey_share_notification_outbox WHERE id=$1`, eventID).Scan(&delivered); err != nil || delivered != "" {
		t.Fatal("failed notification was lost", delivered, err)
	}
	if _, err = db.Exec(ctx, `UPDATE journey_share_notification_outbox SET available_at=TIMESTAMPTZ '2000-01-01' WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	sender.err = nil
	if err = DeliverOne(ctx, outbox, sender); err != nil {
		t.Fatal(err)
	}
	if sender.request.IDempotencyKey != eventID || sender.request.Data["share_ref"] != sessionID {
		t.Fatal("unstable retry identity")
	}
	var sentAt *time.Time
	if err = db.QueryRow(ctx, `SELECT delivered_at FROM journey_share_notification_outbox WHERE id=$1`, eventID).Scan(&sentAt); err != nil || sentAt == nil {
		t.Fatal("recovery did not acknowledge", err)
	}
}
