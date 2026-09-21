package networkevents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	sharedevents "cosmicforge/logistics/shared/go/events"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	previous, _ := os.Getwd()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

type recorder struct {
	mu        sync.Mutex
	envelopes []sharedevents.Envelope
	fail      bool
}

func (r *recorder) Deliver(_ context.Context, envelope sharedevents.Envelope) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("analytics unreachable")
	}
	r.envelopes = append(r.envelopes, envelope)
	return nil
}

func enqueue(t *testing.T, pool *pgxpool.Pool, target, eventType string, payload map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(payload)
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO hauling_domain_outbox
		(aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id, target)
		VALUES ('recovery_case', $1, $2, 2, $3, 'corr-1', $4) RETURNING id::text`,
		uuid.NewString(), eventType, raw, target).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func delivered(t *testing.T, pool *pgxpool.Pool, id string) (bool, bool) {
	t.Helper()
	var done, dead bool
	if err := pool.QueryRow(context.Background(), `SELECT delivered_at IS NOT NULL, dead_lettered_at IS NOT NULL
		FROM hauling_domain_outbox WHERE id=$1::uuid`, id).Scan(&done, &dead); err != nil {
		t.Fatal(err)
	}
	return done, dead
}

func TestAnalyticsRowsAreDeliveredAndNothingElse(t *testing.T) {
	pool := testPool(t)
	stamped := uuid.NewString()
	analytics := enqueue(t, pool, Target, "hauling.recovery.started",
		map[string]any{"event_id": stamped, "schema_version": 1, "original_provider_id": "p1"})
	fleet := enqueue(t, pool, "fleet-events", "trip.created.v1", map[string]any{})
	completed := enqueue(t, pool, "fleet-completed-work", "hauling.fleet.completed_work", map[string]any{})

	sink := &recorder{}
	worker := &Worker{DB: pool, Deliverer: sink}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var mine *sharedevents.Envelope
	for i := range sink.envelopes {
		if sink.envelopes[i].EventID == stamped {
			mine = &sink.envelopes[i]
		}
		if name := sink.envelopes[i].EventName; name == "trip.created.v1" || name == "hauling.fleet.completed_work" {
			t.Fatalf("the analytics worker took a Fleet row (%s)", name)
		}
	}
	if mine == nil {
		t.Fatal("the analytics row was not delivered under its stamped event id")
	}
	if mine.SourceService != "driver-hauling-service" || mine.SourceVersion != 2 || mine.Validate() != nil {
		t.Fatalf("envelope %+v", *mine)
	}
	if done, _ := delivered(t, pool, analytics); !done {
		t.Fatal("a delivered row must be marked delivered")
	}
	for _, other := range []string{fleet, completed} {
		if done, _ := delivered(t, pool, other); done {
			t.Fatal("the analytics worker marked another consumer's row delivered")
		}
	}
}

func TestAnUnreachableAnalyticsIsRetriedThenDeadLettered(t *testing.T) {
	pool := testPool(t)
	id := enqueue(t, pool, Target, "hauling.recovery.failed", map[string]any{})
	worker := &Worker{DB: pool, Deliverer: &recorder{fail: true}, MaxAttempts: 2, BaseBackoff: 1}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := pool.Exec(context.Background(),
			`UPDATE hauling_domain_outbox SET available_at=now() WHERE id=$1::uuid`, id); err != nil {
			t.Fatal(err)
		}
		_ = worker.RunOnce(context.Background())
		done, dead := delivered(t, pool, id)
		if done {
			t.Fatal("a failed delivery was marked delivered")
		}
		if dead != (attempt == 2) {
			t.Fatalf("attempt %d: dead-lettered=%v", attempt, dead)
		}
	}
}

// The Fleet domain-event worker used to claim every row regardless of target,
// so analytics facts were posted to Fleet. It now takes only its own.
func TestTheFleetWorkerLeavesAnalyticsRowsAlone(t *testing.T) {
	pool := testPool(t)
	analytics := enqueue(t, pool, Target, "hauling.recovery.resolved", map[string]any{})
	var fleetBodies int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fleetBodies++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	fleet := &fleetauthority.OutboxWorker{DB: pool, URL: server.URL, Secret: []byte("s"), Client: server.Client()}
	fleet.RunBatchForTest(context.Background())
	if done, _ := delivered(t, pool, analytics); done {
		t.Fatal("the Fleet worker delivered an analytics row")
	}
	var leased bool
	if err := pool.QueryRow(context.Background(), `SELECT lease_owner IS NOT NULL OR attempts > 0
		FROM hauling_domain_outbox WHERE id=$1::uuid`, analytics).Scan(&leased); err != nil || leased {
		t.Fatalf("the Fleet worker claimed an analytics row (leased=%v err=%v)", leased, err)
	}
}
