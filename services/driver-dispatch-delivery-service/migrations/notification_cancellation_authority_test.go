package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

func TestNotificationCancellationAuthorityMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("000053_notification_cancellation_authority.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"CREATE OR REPLACE FUNCTION dispatch_booking_notification_intent()",
		"TG_TABLE_NAME='cancellations'", "FROM cancellations c WHERE c.trip_id=NEW.id",
		"AFTER INSERT ON cancellations", "ON CONFLICT DO NOTHING",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q", want)
		}
	}
	tripBranch := strings.SplitN(sql, "ELSIF TG_TABLE_NAME='trips' THEN", 2)[1]
	if strings.Contains(tripBranch, "NEW.cancelled_by") || strings.Contains(sql, "t.cancelled_by") {
		t.Fatal("cancellation actor must not be read from a Trip row")
	}
	if strings.Contains(sql, "ALTER TABLE") || strings.Contains(sql, "CREATE TABLE") {
		t.Fatal("notification correction must not duplicate cancellation persistence")
	}
}

func TestNotificationCancellationAuthorityUpgradeRollbackReapplyPostgres(t *testing.T) {
	pool, cleanup := operationalTestPoolThrough(t, "000052_legacy_handover_secret_isolation.up.sql")
	defer cleanup()
	ctx := context.Background()
	for _, suffix := range []string{"up", "down", "up"} {
		path := "000053_notification_cancellation_authority." + suffix + ".sql"
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE tgrelid='cancellations'::regclass AND tgname='dispatch_cancellation_notification_intent' AND NOT tgisinternal`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("cancellation triggers=%d, error=%v", count, err)
	}
	var definition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_functiondef('dispatch_booking_notification_intent()'::regprocedure)`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "FROM cancellations c WHERE c.trip_id=NEW.id") {
		t.Fatal("reapply did not restore the authoritative cancellation lookup")
	}
}

func TestCanonicalCancellationNotificationActorAndDedupPostgres(t *testing.T) {
	for _, actor := range []string{"provider", "customer"} {
		t.Run(actor, func(t *testing.T) {
			_, pool, f, cleanup := seedHandoverFixture(t)
			defer cleanup()
			ctx := context.Background()
			repo := trip.NewPostgresRepository(pool)
			var updated *trip.Trip
			var cancellation *trip.Cancellation
			var err error
			if actor == "provider" {
				updated, cancellation, err = repo.CancelTripTx(ctx, trip.CancelTripInput{TripID: f.trip, ProviderID: f.providerA, FromStatus: trip.TripStatus("arrived_pickup"), ReasonCode: "other", Now: time.Now().UTC()})
			} else {
				updated, cancellation, err = repo.CancelTripByCustomerTx(ctx, trip.CustomerCancelTripInput{TripID: f.trip, FromStatus: trip.TripStatus("arrived_pickup"), Now: time.Now().UTC()})
			}
			if err != nil || updated == nil || cancellation == nil {
				t.Fatalf("canonical cancellation: trip=%v cancellation=%v err=%v", updated, cancellation, err)
			}
			// Booking projection and repeated same-state updates must not notify the
			// wrong party or generate a second intent after canonical cancellation.
			for i := 0; i < 2; i++ {
				if _, err = pool.Exec(ctx, `UPDATE dispatch_bookings SET status='cancelled',provider_id=$2 WHERE id=$1`, f.booking, f.providerA); err != nil {
					t.Fatal(err)
				}
				if _, err = pool.Exec(ctx, `UPDATE trips SET status='cancelled' WHERE id=$1`, f.trip); err != nil {
					t.Fatal(err)
				}
			}
			wantEvent, wantRecipient, wantType := "delivery.cancelled", f.providerA, "provider"
			if actor == "provider" {
				wantEvent, wantRecipient, wantType = "delivery.cancelled_by_provider", f.customer, "customer"
			}
			var count int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE aggregate_type='booking_notification' AND payload->>'booking_id'=$1 AND event_type IN ('delivery.cancelled','delivery.cancelled_by_provider')`, f.booking).Scan(&count); err != nil || count != 1 {
				t.Fatalf("cancellation intents=%d, err=%v", count, err)
			}
			var event, recipient, recipientType string
			if err = pool.QueryRow(ctx, `SELECT event_type,payload->>'recipient_id',payload->>'recipient_type' FROM dispatch_marketplace_outbox WHERE aggregate_type='booking_notification' AND payload->>'booking_id'=$1 AND event_type IN ('delivery.cancelled','delivery.cancelled_by_provider')`, f.booking).Scan(&event, &recipient, &recipientType); err != nil {
				t.Fatal(err)
			}
			if event != wantEvent || recipient != wantRecipient || recipientType != wantType {
				t.Fatalf("intent=(%s,%s,%s), want=(%s,%s,%s)", event, recipient, recipientType, wantEvent, wantRecipient, wantType)
			}
		})
	}
}

func TestCancellationNotificationRollbackPostgres(t *testing.T) {
	_, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE trips SET status='cancelled' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cancellations(trip_id,cancelled_by,reason_code) VALUES($1,'provider','other')`, f.trip); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='delivery.cancelled_by_provider'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("transaction intent count=%d, err=%v", count, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='delivery.cancelled_by_provider'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cancellation notification escaped rollback: count=%d, err=%v", count, err)
	}
}
