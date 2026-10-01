package outbox

// Section 14 closure: Taxi recovery notifications.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/notifications"
)

type queue struct {
	mu       sync.Mutex
	requests []notifications.Request
	fail     bool
}

func (q *queue) Publish(_ context.Context, request notifications.Request) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fail {
		return errors.New("notification queue unavailable")
	}
	q.requests = append(q.requests, request)
	return nil
}

func audienceFor(goods bool) RecoveryAudience {
	return RecoveryAudience{CustomerID: "cus-1", OrganizationID: "org-1", OriginalProviderID: "drv-old",
		ReplacementProviderID: "drv-new", Goods: goods}
}

func recipients(intents []notifications.NetworkIntent) map[string]string {
	out := map[string]string{}
	for _, intent := range intents {
		out[intent.RecipientType+"/"+intent.RecipientID] = intent.EventType
	}
	return out
}

func TestUserVisibleRecoveryFactsReachTheirAudience(t *testing.T) {
	cases := map[string]map[string]string{
		"taxi.recovery.started": {
			"customer/cus-1":           notifications.EventNetworkCustomerRecoveryStarted,
			"fleet_organization/org-1": notifications.EventNetworkFleetRecoveryStarted,
		},
		"taxi.recovery.provider_replaced.v1": {
			"customer/cus-1":           notifications.EventNetworkCustomerProviderReplaced,
			"provider/drv-old":         notifications.EventNetworkProviderJobReassigned,
			"provider/drv-new":         notifications.EventNetworkProviderReplacementAssigned,
			"fleet_organization/org-1": notifications.EventNetworkFleetProviderReplaced,
		},
		"taxi.recovery.failed": {
			"customer/cus-1": notifications.EventNetworkCustomerRecoveryFailed,
			"admin_operations/" + notifications.AdminOperationsQueue: notifications.EventNetworkAdminRecoveryFailed,
		},
		"taxi.recovery.custody_transfer_required.v1": {
			"provider/drv-old": notifications.EventNetworkProviderRecoveryTransfer,
			"provider/drv-new": notifications.EventNetworkProviderRecoveryTransfer,
		},
	}
	for event, want := range cases {
		got := recipients(RecoveryIntents(Record{ID: "r1", EventType: event, AggregateType: "booking",
			AggregateID: "bk-1", IdempotencyKey: "notify:r1"}, audienceFor(false)))
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", event, got, want)
		}
		for recipient, template := range want {
			if got[recipient] != template {
				t.Errorf("%s: %s got %q, want %q", event, recipient, got[recipient], template)
			}
		}
	}
}

// Internal worker transitions, and a resolution nobody needs telling about,
// produce nothing.
func TestInternalRecoveryTransitionsNotifyNobody(t *testing.T) {
	for _, event := range []string{"taxi.recovery.resolved", "taxi.recovery.evaluating", "taxi.recovery.search_tick",
		"taxi.booking.created"} {
		if intents := RecoveryIntents(Record{ID: "r1", EventType: event}, audienceFor(false)); len(intents) != 0 {
			t.Errorf("%s produced %d notifications", event, len(intents))
		}
	}
}

func TestAGoodsCustomerIsToldByTheirVendorNotTaxi(t *testing.T) {
	got := recipients(RecoveryIntents(Record{ID: "r1", EventType: "taxi.recovery.provider_replaced.v1",
		AggregateType: "taxi_goods_mission", AggregateID: "gm-1"}, audienceFor(true)))
	if _, told := got["customer/cus-1"]; told {
		t.Fatal("Taxi must not notify a goods customer directly")
	}
	if got["provider/drv-new"] != notifications.EventNetworkProviderReplacementAssigned {
		t.Fatalf("goods providers must still be told: %v", got)
	}
}

func TestRecoveryNotificationsCarryReferencesOnly(t *testing.T) {
	for _, intent := range RecoveryIntents(Record{ID: "r1", EventType: "taxi.recovery.provider_replaced",
		AggregateType: "booking", AggregateID: "bk-1", IdempotencyKey: "notify:r1"}, audienceFor(false)) {
		request := notifications.NetworkRequest("driver-taxi-service", intent)
		if err := request.Validate(); err != nil {
			t.Fatalf("%s: %v", intent.EventType, err)
		}
		for key := range request.Data {
			if key != "booking_id" {
				t.Fatalf("%s carries %q", intent.EventType, key)
			}
		}
		// The customer's id is the recipient, never data another recipient sees.
		for _, value := range request.Data {
			if value == "cus-1" || value == "drv-old" || value == "drv-new" {
				t.Fatalf("%s exposes another party's id in its data", intent.EventType)
			}
		}
	}
}

func TestAReplayedRecoveryFactDedupes(t *testing.T) {
	record := Record{ID: "r1", EventType: "taxi.recovery.started", AggregateType: "booking", AggregateID: "bk-1",
		IdempotencyKey: "notify:r1"}
	a := RecoveryIntents(record, audienceFor(false))
	b := RecoveryIntents(record, audienceFor(false))
	if notifications.NetworkRequest("driver-taxi-service", a[0]).IDempotencyKey !=
		notifications.NetworkRequest("driver-taxi-service", b[0]).IDempotencyKey {
		t.Fatal("a redelivered row must produce the same notification identity")
	}
}

type fixedAudience RecoveryAudience

func (a fixedAudience) RecoveryAudience(context.Context, string, string) (RecoveryAudience, error) {
	return RecoveryAudience(a), nil
}

func TestAQueueOutageIsRetriedNotLost(t *testing.T) {
	destination := RecoveryNotificationDestination{Enabled: true, Publisher: &queue{fail: true},
		Audience: fixedAudience(audienceFor(false))}
	err := destination.Deliver(context.Background(), Record{ID: "r1", EventType: "taxi.recovery.started",
		AggregateType: "booking", AggregateID: "bk-1"})
	var terminal *TerminalDeliveryError
	if err == nil || errors.As(err, &terminal) {
		t.Fatalf("a queue outage must be retryable, got %v", err)
	}
}

// PostgreSQL: the committed recovery fact is copied once for notification,
// delivered from that copy, and a failed delivery changes no recovery state.
func TestRecoveryFactsAreNotifiedFromTheirCommittedCopy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	bookingID, customerID, oldDriver, newDriver := seedRecoveredBooking(t, db)

	if err := Write(ctx, db, "booking", bookingID, "taxi.recovery.provider_replaced",
		[]byte(`{"booking_id":"`+bookingID+`"}`)); err != nil {
		t.Fatal(err)
	}
	record := notificationCopy(t, db, bookingID, "taxi.recovery.provider_replaced")
	var copies int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1::uuid AND target=$2`,
		bookingID, TargetNotification).Scan(&copies)
	if copies != 1 {
		t.Fatalf("the fact was copied %d times for notification", copies)
	}

	failing := RecoveryNotificationDestination{Enabled: true, Publisher: &queue{fail: true}, Audience: PostgresAudience{DB: db}}
	if err := failing.Deliver(ctx, record); err == nil {
		t.Fatal("a queue outage must surface as a retry")
	}
	var state string
	if err := db.QueryRow(ctx, `SELECT state FROM taxi_recovery_cases WHERE booking_id=$1::uuid`, bookingID).Scan(&state); err != nil ||
		state != "RESOLVED" {
		t.Fatalf("a notification failure changed the recovery (%q, %v)", state, err)
	}

	sent := &queue{}
	working := RecoveryNotificationDestination{Enabled: true, Publisher: sent, Audience: PostgresAudience{DB: db}}
	if err := working.Deliver(ctx, record); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, request := range sent.requests {
		got[request.Recipient.Type+"/"+request.Recipient.ID] = request.EventType
		if request.SourceEventID != record.IdempotencyKey {
			t.Fatalf("source event %q, want the copy's key", request.SourceEventID)
		}
	}
	if got["customer/"+customerID] != notifications.EventNetworkCustomerProviderReplaced ||
		got["provider/"+oldDriver] != notifications.EventNetworkProviderJobReassigned ||
		got["provider/"+newDriver] != notifications.EventNetworkProviderReplacementAssigned {
		t.Fatalf("notifications %v", got)
	}
}

func seedRecoveredBooking(t *testing.T, db *pgxpool.Pool) (bookingID, customerID, oldDriver, newDriver string) {
	t.Helper()
	ctx := context.Background()
	market, zone := uuid.NewString(), uuid.NewString()
	bookingID, customerID, oldDriver, newDriver = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	code := "TEST-" + uuid.NewString()[:8]
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", strings.Fields(query)[2], err)
		}
	}
	exec(`INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, market, code)
	exec(`INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy}')`, zone, market, code)
	for _, driver := range []string{oldDriver, newDriver} {
		exec(`INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, driver, "+234"+driver[:10])
	}
	exec(`INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, matched_at)
		VALUES ($1, $2, $3, $4, $5, 'economy', 'accepted', now())`, bookingID, customerID, newDriver, market, zone)
	exec(`INSERT INTO taxi_recovery_cases (subject_type, booking_id, trigger_code, state, original_provider_id,
		replacement_provider_id, operation_ref, audit_json, resolved_at)
		VALUES ('BOOKING', $1, 'SCHEDULED_RISK', 'RESOLVED', $2, $3, $4, '[]', now())`,
		bookingID, oldDriver, newDriver, "test:"+bookingID)
	t.Cleanup(func() {
		c := context.Background()
		_, _ = db.Exec(c, `DELETE FROM taxi_outbox WHERE aggregate_id=$1::uuid`, bookingID)
		_, _ = db.Exec(c, `DELETE FROM taxi_recovery_cases WHERE booking_id=$1::uuid`, bookingID)
		_, _ = db.Exec(c, `DELETE FROM taxi_bookings WHERE id=$1::uuid`, bookingID)
		_, _ = db.Exec(c, `DELETE FROM taxi_providers WHERE id = ANY($1::uuid[])`, []string{oldDriver, newDriver})
		_, _ = db.Exec(c, `DELETE FROM markets WHERE id=$1::uuid`, market)
	})
	return bookingID, customerID, oldDriver, newDriver
}

func notificationCopy(t *testing.T, db *pgxpool.Pool, aggregateID, event string) Record {
	t.Helper()
	var record Record
	if err := db.QueryRow(context.Background(), `SELECT id::text, aggregate_type, aggregate_id::text, event_type,
		payload, occurred_at, target, idempotency_key FROM taxi_outbox
		WHERE aggregate_id=$1::uuid AND event_type=$2 AND target=$3`, aggregateID, event, TargetNotification).
		Scan(&record.ID, &record.AggregateType, &record.AggregateID, &record.EventType, &record.Payload,
			&record.OccurredAt, &record.Target, &record.IdempotencyKey); err != nil {
		t.Fatalf("notification copy of %s: %v", event, err)
	}
	return record
}

// Migration 020 rolls back to 019's copy rules and reapplies cleanly.
func TestMigration020DownRestoresTheEarlierCopyRules(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	copies := func(event, target string) int {
		booking := uuid.NewString()
		if err := Write(ctx, db, "booking", booking, event, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1::uuid AND target=$2`,
			booking, target).Scan(&n); err != nil {
			t.Fatal(err)
		}
		_, _ = db.Exec(ctx, `DELETE FROM taxi_outbox WHERE aggregate_id=$1::uuid`, booking)
		return n
	}
	up := read("020_network_lifecycle_and_recovery_notifications.up.sql")
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), up) })

	if _, err := db.Exec(ctx, read("020_network_lifecycle_and_recovery_notifications.down.sql")); err != nil {
		t.Fatalf("down: %v", err)
	}
	if copies("taxi.booking.created", "analytics-service") != 0 || copies("taxi.recovery.started", TargetNotification) != 0 {
		t.Fatal("020 down left its copy rules in place")
	}
	if copies("taxi.recovery.started", "analytics-service") != 1 {
		t.Fatal("020 down removed 019's recovery analytics copy")
	}
	if _, err := db.Exec(ctx, up); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if copies("taxi.booking.created", "analytics-service") != 1 || copies("taxi.recovery.started", TargetNotification) != 1 {
		t.Fatal("020 reapplied without its copy rules")
	}
}
