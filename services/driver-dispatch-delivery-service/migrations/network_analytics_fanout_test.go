package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

// Section 13: the outbox copies Dispatch's network facts to analytics once,
// and never copies a copy.
func TestNetworkAnalyticsFanout(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	insert := func(eventType, aggregateType, aggregateID, target string, version int64) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox
			(event_type, aggregate_type, aggregate_id, aggregate_version, payload, payload_hash, target_service)
			VALUES ($1,$2,$3,$4,'{"providerId":"r1"}'::jsonb,'h',$5) ON CONFLICT DO NOTHING`,
			eventType, aggregateType, aggregateID, version, target); err != nil {
			t.Fatalf("insert %s: %v", eventType, err)
		}
	}
	copies := func(eventType string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox
			WHERE target_service='analytics-service' AND event_type=$1`, eventType).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	trip := uuid.NewString()
	insert("trip.completed.v1", "trip", trip, "fleet-partner-service", 3)
	if copies("trip.completed.v1") != 1 {
		t.Fatal("a completed trip must reach analytics")
	}
	var aggregateType string
	if err := pool.QueryRow(ctx, `SELECT aggregate_type FROM dispatch_marketplace_outbox
		WHERE target_service='analytics-service' AND event_type='trip.completed.v1'`).Scan(&aggregateType); err != nil ||
		aggregateType != "analytics:trip" {
		t.Fatalf("copy aggregate_type=%q err=%v", aggregateType, err)
	}
	// The same fact sent to a second consumer is still one analytics fact.
	insert("trip.completed.v1", "trip", trip, "vendor-service", 3)
	if copies("trip.completed.v1") != 1 {
		t.Fatal("the same trip outcome was copied twice")
	}
	// Already addressed to analytics: no copy of a copy.
	insert("dispatch.capability_mismatch.v1", "delivery_mission", uuid.NewString(), "analytics-service", 1)
	if copies("dispatch.capability_mismatch.v1") != 1 {
		t.Fatal("an event already addressed to analytics was copied")
	}
	// The Vendor projection re-emits recovery facts; counting it would double
	// every recovery.
	insert("dispatch.provider_replacement.started.v1", "vendor_delivery_projection", uuid.NewString(), "vendor-service", 1)
	if copies("dispatch.provider_replacement.started.v1") != 0 {
		t.Fatal("the vendor projection was copied to analytics")
	}
	// A rider's decline, with its reason, is a network fact too.
	insert("dispatch.offer.declined.v1", "delivery_mission", uuid.NewString(), "vendor-service", 2)
	if copies("dispatch.offer.declined.v1") != 1 {
		t.Fatal("a decline must reach analytics")
	}
	insert("trip.status_changed.v1", "trip", uuid.NewString(), "fleet-partner-service", 1)
	if copies("trip.status_changed.v1") != 0 {
		t.Fatal("a non-network event was copied to analytics")
	}

	down, err := os.ReadFile("000063_network_analytics_fanout.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	insert("trip.cancelled.v1", "trip", uuid.NewString(), "fleet-partner-service", 1)
	if copies("trip.cancelled.v1") != 0 {
		t.Fatal("rolling back 000063 must stop the copying")
	}
	up, err := os.ReadFile("000063_network_analytics_fanout.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}
