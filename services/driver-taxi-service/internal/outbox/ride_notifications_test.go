package outbox

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/notifications"
)

func TestRideLifecycleIntentsOnlyCarryOwnerReadReference(t *testing.T) {
	for _, event := range []string{EventBookingAccepted, EventBookingEnRoutePickup,
		EventBookingArrived, EventBookingStarted, EventBookingCompleted,
		EventBookingCancelled, "taxi.booking.auto_completed", "taxi.booking.admin_cancelled"} {
		intents := RideLifecycleIntents(Record{ID: "event-1", AggregateType: "booking",
			AggregateID: "booking-1", EventType: event}, RecoveryAudience{CustomerID: "customer-1"})
		if len(intents) != 1 || intents[0].EventType != event ||
			intents[0].RecipientID != "customer-1" || len(intents[0].Data) != 1 ||
			intents[0].Data["booking_id"] != "booking-1" {
			t.Fatalf("%s: %+v", event, intents)
		}
		request := notifications.NetworkRequest("driver-taxi-service", intents[0])
		if err := request.Validate(); err != nil {
			t.Fatalf("%s: %v", event, err)
		}
		if hint, ok := notifications.RouteIntentFor(request); !ok ||
			hint.Target != "RIDE_DETAIL" || hint.Ref != "booking-1" || !hint.ReadBack {
			t.Fatalf("%s: route hint %+v, %v", event, hint, ok)
		}
	}
	if got := RideLifecycleIntents(Record{EventType: EventBookingAccepted}, RecoveryAudience{Goods: true, CustomerID: "c"}); len(got) != 0 {
		t.Fatal("Taxi Goods must not send passenger notifications")
	}
}

func TestCommittedRideFactCopiesOnceToNotificationOutbox(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	bookingID := uuid.NewString()
	if err := Write(ctx, db, "booking", bookingID, EventBookingAccepted, []byte(`{"otp":"must-not-forward"}`)); err != nil {
		t.Fatal(err)
	}
	var copies int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1::uuid AND event_type=$2 AND target=$3`,
		bookingID, EventBookingAccepted, TargetNotification).Scan(&copies); err != nil {
		t.Fatal(err)
	}
	if copies != 1 {
		t.Fatalf("notification copies=%d, want one", copies)
	}
}

func TestRideNotificationsUseTheCommittedOutboxAndTemplates(t *testing.T) {
	up, err := os.ReadFile("../../migrations/021_customer_ride_notifications.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	templates, err := os.ReadFile("../../../notification-service/migrations/005_customer_ride_templates.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{EventBookingAccepted, EventBookingEnRoutePickup,
		EventBookingArrived, EventBookingStarted, EventBookingCompleted, EventBookingCancelled,
		"taxi.booking.auto_completed", "taxi.booking.admin_cancelled"} {
		if !strings.Contains(string(up), "'"+event+"'") || !strings.Contains(string(templates), "'"+event+"'") {
			t.Errorf("missing durable copy or template for %s", event)
		}
	}
}

func TestRideNotificationDestinationRetriesQueueFailure(t *testing.T) {
	q := &queue{fail: true}
	d := RecoveryNotificationDestination{Enabled: true, Publisher: q,
		Audience: staticRideAudience{customer: "customer-1"}}
	err := d.Deliver(context.Background(), Record{ID: "event-1", EventType: EventBookingAccepted,
		AggregateType: "booking", AggregateID: "booking-1"})
	if err == nil {
		t.Fatal("queue failure must keep the outbox row retryable")
	}
	q.fail = false
	if err := d.Deliver(context.Background(), Record{ID: "event-1", EventType: EventBookingAccepted,
		AggregateType: "booking", AggregateID: "booking-1"}); err != nil || len(q.requests) != 1 {
		t.Fatalf("retry: %v, sent=%d", err, len(q.requests))
	}
}

type staticRideAudience struct{ customer string }

func (a staticRideAudience) RecoveryAudience(context.Context, string, string) (RecoveryAudience, error) {
	return RecoveryAudience{CustomerID: a.customer}, nil
}
