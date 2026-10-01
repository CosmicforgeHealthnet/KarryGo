package messageusecases_test

// Section 14: PostgreSQL-backed certification of network notifications.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/notification-service/internal/database"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/notifications"
)

func networkService(t *testing.T) (*messageusecases.NotificationService, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("NOTIFICATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTIFICATION_TEST_DATABASE_URL not set")
	}
	pool, err := database.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	previous, _ := os.Getwd()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return messageusecases.NewNotificationService(messageusecases.Options{
		Repository:          messagerepositories.NewPostgresNotificationRepository(pool),
		RealtimeTokenSecret: []byte("realtime-secret"),
	}), pool
}

func replaced(recipientType, recipientID, fact string) notifications.NetworkIntent {
	return notifications.NetworkIntent{
		EventType: notifications.EventNetworkCustomerProviderReplaced, RecipientType: recipientType,
		RecipientID: recipientID, SourceEventID: fact, SourceAggregateType: "recovery_case",
		SourceAggregateID: fact, SourceVersion: 2, Data: map[string]interface{}{"booking_id": "b-1"},
	}
}

func messageCount(t *testing.T, pool *pgxpool.Pool, fact string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_messages WHERE source_event_id=$1`, fact).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestANetworkFactIsAnnouncedOncePerRecipient(t *testing.T) {
	service, pool := networkService(t)
	ctx := context.Background()
	fact := "case-" + uuid.NewString()
	customer := uuid.NewString()

	first, err := service.Send(ctx, notifications.NetworkRequest("driver-hauling-service", replaced("customer", customer, fact)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Send(ctx, notifications.NetworkRequest("driver-hauling-service", replaced("customer", customer, fact)))
	if err != nil || again.MessageID != first.MessageID {
		t.Fatalf("a retried send must return the original message: %+v err=%v", again, err)
	}
	// The same fact re-published under a fresh idempotency key: only the
	// semantic index can recognise it.
	republished := notifications.NetworkRequest("driver-hauling-service", replaced("customer", customer, fact))
	republished.IDempotencyKey = "republished-" + uuid.NewString()
	republishResult, err := service.Send(ctx, republished)
	if err != nil || republishResult.MessageID != first.MessageID {
		// An error here would make every producer retry a fact that was
		// already delivered, forever.
		t.Fatalf("a re-published fact must resolve to the original message: %+v err=%v", republishResult, err)
	}
	if got := messageCount(t, pool, fact); got != 1 {
		t.Fatalf("messages for one fact to one recipient = %d, want 1", got)
	}

	if _, err = service.Send(ctx, notifications.NetworkRequest("driver-hauling-service",
		replaced(notifications.RecipientFleetOrganization, uuid.NewString(), fact))); err != nil {
		t.Fatal(err)
	}
	if got := messageCount(t, pool, fact); got != 2 {
		t.Fatalf("a second audience must get its own message, got %d", got)
	}
}

func TestNetworkTemplatesResolve(t *testing.T) {
	service, pool := networkService(t)
	fact := "case-" + uuid.NewString()
	if _, err := service.Send(context.Background(),
		notifications.NetworkRequest("driver-hauling-service", replaced("customer", uuid.NewString(), fact))); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := pool.QueryRow(context.Background(),
		`SELECT title FROM notification_messages WHERE source_event_id=$1`, fact).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Your new driver is on the way" {
		t.Fatalf("title=%q, the network template did not resolve", title)
	}
	var templates int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM notification_templates
		WHERE key = ANY($1) AND active`, notifications.NetworkEventTypes()).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if templates != len(notifications.NetworkEventTypes()) {
		t.Fatalf("%d of %d network templates are installed", templates, len(notifications.NetworkEventTypes()))
	}
}

func TestShareAndWalletHintsResolveAndRequireAuthorityReadBack(t *testing.T) {
	service, pool := networkService(t)
	for _, tc := range []struct{ source, event, refKey, target string }{
		{"customer-service", "share.created", "share_ref", "SHARE_MANAGEMENT"},
		{"payment-wallet-service", notifications.EventRefundCompleted, "reference", "PAYMENT_DETAIL"},
	} {
		id, ref := uuid.NewString(), uuid.NewString()
		_, err := service.Send(context.Background(), notifications.Request{
			IDempotencyKey: id, SourceService: tc.source, EventType: tc.event,
			SourceAggregateType: "operation", SourceAggregateID: ref,
			Recipient:   notifications.Recipient{Type: notifications.RecipientCustomer, ID: uuid.NewString()},
			TemplateKey: tc.event, Data: map[string]any{tc.refKey: ref},
		})
		if err != nil {
			t.Fatal(tc.event, err)
		}
		var data, body string
		if err = pool.QueryRow(context.Background(), `SELECT data::text,body FROM notification_messages WHERE idempotency_key=$1`, id).Scan(&data, &body); err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{tc.target, `"read_back": true`, ref} {
			if !strings.Contains(data, required) {
				t.Fatalf("%s missing %q: %s", tc.event, required, data)
			}
		}
		if strings.Contains(data, "token") || strings.Contains(data, "amount") || strings.Contains(body, "{{") {
			t.Fatal("hint exposed sensitive or unresolved data")
		}
	}
}

func TestALeakyNetworkNotificationStoresNothing(t *testing.T) {
	service, pool := networkService(t)
	fact := "case-" + uuid.NewString()
	leaky := replaced("customer", uuid.NewString(), fact)
	leaky.Data["dropoff_address"] = "12 Marina"
	if _, err := service.Send(context.Background(), notifications.NetworkRequest("driver-hauling-service", leaky)); err == nil {
		t.Fatal("a network notification carrying an address was accepted")
	}
	if messageCount(t, pool, fact) != 0 {
		t.Fatal("a refused notification left a message behind")
	}
}

func TestRideLifecycleTemplateAndReadBackIntentPersist(t *testing.T) {
	service, pool := networkService(t)
	fact := "ride-event-" + uuid.NewString()
	booking := "ride-" + uuid.NewString()
	request := notifications.NetworkRequest("driver-taxi-service", notifications.NetworkIntent{
		EventType: "taxi.booking.accepted", RecipientType: notifications.RecipientCustomer,
		RecipientID: uuid.NewString(), SourceEventID: fact,
		SourceAggregateType: "booking", SourceAggregateID: booking,
		Data: map[string]interface{}{"booking_id": booking},
	})
	if _, err := service.Send(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var title string
	var data map[string]any
	if err := pool.QueryRow(context.Background(), `SELECT title,data FROM notification_messages WHERE source_event_id=$1`, fact).Scan(&title, &data); err != nil {
		t.Fatal(err)
	}
	if title != "Driver assigned" {
		t.Fatalf("title=%q", title)
	}
	intent, ok := data["route_intent"].(map[string]any)
	if !ok || intent["target"] != "RIDE_DETAIL" || intent["ref"] != booking || intent["read_back"] != true {
		t.Fatalf("unsafe or missing read-back intent: %v", data)
	}
}
