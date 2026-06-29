package request

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryRequestFoundation(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_REQUEST_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_REQUEST_TEST_DATABASE_URL or TEST_DATABASE_URL to run request repository integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := "request_repository_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{"000004_create_providers.up.sql", "000019_create_request_broadcasts.up.sql", "000020_create_provider_request_inbox.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}

	providerA, providerB := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2),($3,$4)`,
		providerA, "+2348000061001", providerB, "+2348000061002"); err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresRepository(pool)
	now := time.Now().UTC()
	bookingID := uuid.NewString()
	broadcast, err := repo.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: bookingID, ServiceType: "dispatch", RadiusKM: 5, Attempt: 1,
		BroadcastAt: now, ExpiresAt: now.Add(30 * time.Second), BookingPayload: []byte(`{"pickup_lat":6.5}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(broadcast.BookingPayload) != `{"pickup_lat": 6.5}` && string(broadcast.BookingPayload) != `{"pickup_lat":6.5}` {
		t.Fatalf("booking_payload=%s", broadcast.BookingPayload)
	}
	if _, err := repo.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: bookingID, ServiceType: "dispatch", RadiusKM: 5, Attempt: 1,
		BroadcastAt: now, ExpiresAt: now.Add(time.Minute), BookingPayload: []byte(`{}`),
	}); err == nil {
		t.Fatal("duplicate booking_id accepted")
	}
	inboxes, err := repo.CreateInboxRows(ctx, broadcast.ID, bookingID, []string{providerA, providerB, providerA})
	if err != nil {
		t.Fatal(err)
	}
	if len(inboxes) != 2 {
		t.Fatalf("inboxes=%d want 2", len(inboxes))
	}
	listA, err := repo.ListProviderInbox(ctx, providerA, ListInboxOptions{})
	if err != nil || len(listA) != 1 || listA[0].ProviderID != providerA {
		t.Fatalf("provider A list=%+v err=%v", listA, err)
	}
	if _, ok, err := repo.GetProviderInboxByID(ctx, listA[0].ID, providerB); err != nil || ok {
		t.Fatalf("provider B accessed provider A inbox: ok=%v err=%v", ok, err)
	}
	if updated, err := repo.MarkInboxRejected(ctx, listA[0].ID, providerB, now); err != nil || updated {
		t.Fatalf("provider B rejected provider A inbox: updated=%v err=%v", updated, err)
	}
	if updated, err := repo.MarkInboxRejected(ctx, listA[0].ID, providerA, now); err != nil || !updated {
		t.Fatalf("provider A reject updated=%v err=%v", updated, err)
	}
	if err := repo.MarkFCMSent(ctx, inboxes[1].ID, now); err != nil {
		t.Fatal(err)
	}
	inboxB, ok, err := repo.GetProviderInboxByID(ctx, inboxes[1].ID, providerB)
	if err != nil || !ok || !inboxB.FCMSent || inboxB.FCMSentAt == nil {
		t.Fatalf("FCM state=%+v ok=%v err=%v", inboxB, ok, err)
	}

	acceptBookingID := uuid.NewString()
	acceptBroadcast, err := repo.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: acceptBookingID, ServiceType: "dispatch", RadiusKM: 5, Attempt: 1,
		BroadcastAt: now, ExpiresAt: now.Add(time.Minute), BookingPayload: []byte(`{"pickup_lat":6.5}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	acceptInboxes, err := repo.CreateInboxRows(ctx, acceptBroadcast.ID, acceptBookingID, []string{providerA, providerB})
	if err != nil || len(acceptInboxes) != 2 {
		t.Fatalf("accept inboxes=%+v err=%v", acceptInboxes, err)
	}
	if err := repo.MarkBroadcastAccepted(ctx, acceptBroadcast.ID, acceptBookingID, acceptInboxes[0].ID, providerA, now); err != nil {
		t.Fatalf("MarkBroadcastAccepted: %v", err)
	}
	accepted, ok, err := repo.GetBroadcastByID(ctx, acceptBroadcast.ID)
	if err != nil || !ok || accepted.Status != BroadcastStatusAccepted ||
		accepted.AcceptedByProviderID == nil || *accepted.AcceptedByProviderID != providerA {
		t.Fatalf("accepted broadcast=%+v ok=%v err=%v", accepted, ok, err)
	}
	otherInbox, ok, err := repo.GetProviderInboxByID(ctx, acceptInboxes[1].ID, providerB)
	if err != nil || !ok || otherInbox.Status != InboxStatusExpired {
		t.Fatalf("other inbox=%+v ok=%v err=%v", otherInbox, ok, err)
	}
}
