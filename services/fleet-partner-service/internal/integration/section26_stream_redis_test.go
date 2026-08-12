package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

func TestSection26RealRedisTransactionalDedupDeadLetterAndReplay(t *testing.T) {
	addr := os.Getenv("FLEET_PARTNER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("FLEET_PARTNER_TEST_REDIS_ADDR not set; real Redis required")
	}
	f := newSearchFixture(t)
	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = redisClient.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	stream := "fleet-partner:section26:e2e:" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() { _ = redisClient.Del(context.Background(), stream).Err() })
	syncService := synchronization.New(f.pool, synchronization.Flags{Enabled: true, WorkerEnabled: true})
	components := &app.Components{Pool: f.pool, Synchronization: syncService}
	cfg := config.Config{SynchronizationEnabled: true, OutboxStreamKey: stream}
	var workers sync.WaitGroup
	app.StartSection26Workers(ctx, cfg, components, redisClient, observability.NewMetrics(), &workers)
	t.Cleanup(func() { cancel(); workers.Wait() })

	payload, _ := json.Marshal(events.ContractPayload{SubjectReference: "provider_redis_1", SourceVersion: 3, AvailabilityState: "AVAILABLE"})
	envelope := events.Envelope{
		EventID: "evt_section26_redis", EventType: "provider.availability.changed.v1", SchemaVersion: 1,
		AggregateType: "provider", AggregateID: "provider_redis_1", AggregateVersion: 3,
		OrganizationID: f.orgID, CorrelationID: "cor_section26_redis", OccurredAt: time.Now().UTC(),
		Producer: "driver-dispatch-delivery-service", Payload: payload,
	}
	raw, _ := json.Marshal(envelope)
	record := events.OutboxRecord{ID: envelope.EventID, EventType: envelope.EventType, SchemaVersion: 1, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID, OrganizationID: f.orgID, Payload: raw}
	publisher := events.NewRedisStreamPublisher(redisClient, stream)
	if err := publisher.Publish(ctx, record); err != nil {
		t.Fatal(err)
	}
	waitForSection26(t, 8*time.Second, func() bool {
		var count int
		_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_synchronization_projections WHERE organization_id=$1 AND subject_reference='provider_redis_1' AND source_version=3`, f.orgID).Scan(&count)
		return count == 1
	})
	if err := publisher.Publish(ctx, record); err != nil {
		t.Fatal(err)
	}
	waitForSection26(t, 8*time.Second, func() bool {
		var count int
		_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_inbox WHERE event_id=$1 AND status='PROCESSED'`, envelope.EventID).Scan(&count)
		return count == 1
	})

	processed, err := syncService.RunReconciliationPass(ctx, synchronization.WorkerConfig{Owner: "redis-e2e", BatchSize: 1, MaxAttempts: 1, Lease: time.Second})
	if err != nil || processed != 1 {
		t.Fatalf("dead-letter reconciliation processed=%d err=%v", processed, err)
	}
	var deadLetterID string
	if err = f.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_synchronization_dead_letters WHERE organization_id=$1 AND subject_reference='provider_redis_1' AND status='OPEN'`, f.orgID).Scan(&deadLetterID); err != nil {
		t.Fatal(err)
	}
	if err = syncService.Replay(ctx, f.orgID, deadLetterID); err != nil {
		t.Fatal(err)
	}
	var deadStatus, caseStatus string
	if err = f.pool.QueryRow(ctx, `SELECT d.status,c.status FROM fleet_partner_synchronization_dead_letters d JOIN fleet_partner_synchronization_projections p ON p.organization_id=d.organization_id AND p.projection_type=d.event_type AND p.subject_reference=d.subject_reference JOIN fleet_partner_synchronization_cases c ON c.projection_id=p.id WHERE d.id=$1`, deadLetterID).Scan(&deadStatus, &caseStatus); err != nil {
		t.Fatal(err)
	}
	if deadStatus != "REPLAYED" || caseStatus != "OPEN" {
		t.Fatalf("replay dead=%s case=%s", deadStatus, caseStatus)
	}
}

func waitForSection26(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Section 26 stream state")
}
