package walletclients

import (
	"context"
	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type outboxSender struct {
	requests []notifications.Request
	err      error
}

func (s *outboxSender) Send(_ context.Context, r notifications.Request) (notifications.SendResponse, error) {
	s.requests = append(s.requests, r)
	return notifications.SendResponse{}, s.err
}

func TestWalletNotificationOutboxCommitsRetriesAndDeduplicates(t *testing.T) {
	dsn := os.Getenv("WALLET_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated WALLET_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, file, _, _ := runtime.Caller(0)
	if err = database.ApplyMigrations(ctx, db, filepath.Join(filepath.Dir(file), "../../../../migrations")); err != nil {
		t.Fatal(err)
	}
	intentID, ref, customer := uuid.NewString(), "pay_"+uuid.NewString(), uuid.NewString()
	if _, err = db.Exec(ctx, `INSERT INTO payment_intents(id,reference,source_service,source_reference,customer_id,amount_kobo,payment_method,status)
	 VALUES($1,$2,'test',$3,$4,1000,'card','pending')`, intentID, ref, uuid.NewString(), customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE payment_intents SET status='held' WHERE id=$1`, intentID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE payment_intents SET status='held' WHERE id=$1`, intentID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM wallet_notification_outbox WHERE entity_ref=$1 AND event_type='payment.success'`, ref).Scan(&count); err != nil || count != 1 {
		t.Fatal("exactly one committed hint required", count, err)
	}
	if _, err = db.Exec(ctx, `UPDATE wallet_notification_outbox SET available_at=TIMESTAMPTZ '2000-01-01' WHERE entity_ref=$1`, ref); err != nil {
		t.Fatal(err)
	}
	sender := &outboxSender{err: errors.New("notification offline")}
	worker := WalletNotificationOutbox{DB: db, Sender: sender}
	if ok, e := worker.DeliverOne(ctx); !ok || e == nil {
		t.Fatal("outage must retain the committed intent", ok, e)
	}
	var status string
	if err = db.QueryRow(ctx, `SELECT status FROM payment_intents WHERE id=$1`, intentID).Scan(&status); err != nil || status != "held" {
		t.Fatal("notification outage rolled back money state", status, err)
	}
	if _, err = db.Exec(ctx, `UPDATE wallet_notification_outbox SET available_at=TIMESTAMPTZ '2000-01-01' WHERE entity_ref=$1`, ref); err != nil {
		t.Fatal(err)
	}
	sender.err = nil
	if ok, e := worker.DeliverOne(ctx); !ok || e != nil {
		t.Fatal("retry failed", ok, e)
	}
	if len(sender.requests) != 2 || sender.requests[0].IDempotencyKey != sender.requests[1].IDempotencyKey || sender.requests[1].Data["reference"] != ref {
		t.Fatal("retry changed identity")
	}
	if len(sender.requests[1].Data) != 1 {
		t.Fatal("financial data leaked into hint")
	}
	var delivered bool
	if err = db.QueryRow(ctx, `SELECT delivered_at IS NOT NULL FROM wallet_notification_outbox WHERE entity_ref=$1`, ref).Scan(&delivered); err != nil || !delivered {
		t.Fatal("successful retry was not acknowledged", err)
	}
	refundRef := "refund_" + uuid.NewString()
	if _, err = db.Exec(ctx, `INSERT INTO refunds(id,reference,payment_intent_id,amount_kobo,status) VALUES($1,$2,$3,250,'pending')`, uuid.NewString(), refundRef, intentID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE refunds SET status='processed' WHERE reference=$1`, refundRef); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM wallet_notification_outbox WHERE entity_ref=$1 AND event_type='refund.completed' AND recipient_id=$2`, refundRef, customer).Scan(&count); err != nil || count != 1 {
		t.Fatal("refund notification missing", count, err)
	}
}
