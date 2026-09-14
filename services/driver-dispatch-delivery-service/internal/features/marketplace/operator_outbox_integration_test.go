package marketplace_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// seedOutboxRow inserts one event in a chosen delivery state.
func seedOutboxRow(t *testing.T, pool *pgxpool.Pool, status, lastError string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO dispatch_marketplace_outbox (
			event_type, aggregate_type, aggregate_id, aggregate_version,
			payload, payload_hash, correlation_id, status, attempt_count,
			last_error, target_service)
		VALUES ('assignment.completed','assignment',gen_random_uuid()::text,7,
			'{"secretish":"payload"}'::jsonb,'hash-1','cor-1',$1,9,
			NULLIF($2,''),'fleet-partner-service')
		RETURNING id::text`, status, lastError).Scan(&id)
	if err != nil {
		t.Fatalf("seed outbox row: %v", err)
	}
	return id
}

func TestOperatorCanSeeWhatIsStuckWithoutSQL(t *testing.T) {
	pool := testPool(t)
	store := &marketplace.OperatorOutboxStore{DB: pool}
	dead := seedOutboxRow(t, pool, marketplace.OutboxDeadLetter, "fleet rejected the record")
	seedOutboxRow(t, pool, marketplace.OutboxPublished, "")

	rows, err := store.List(context.Background(), marketplace.OutboxDeadLetter, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != dead {
		t.Fatalf("dead-letter filter returned %d rows", len(rows))
	}

	// EVT-010 names exactly what an operator has to be able to see.
	row := rows[0]
	switch {
	case row.EventType == "":
		t.Fatal("the event type is missing")
	case row.AggregateType == "" || row.AggregateID == "" || row.AggregateVersion != 7:
		t.Fatalf("the aggregate is not identified: %+v", row)
	case row.TargetService != "fleet-partner-service":
		t.Fatalf("the destination is missing: %q", row.TargetService)
	case row.AttemptCount != 9:
		t.Fatalf("attemptCount = %d", row.AttemptCount)
	case row.LastError == "":
		t.Fatal("the last error is missing")
	case row.Status != marketplace.OutboxDeadLetter:
		t.Fatalf("status = %q", row.Status)
	}

	// An unfiltered read sees both, and the payload never travels: a
	// dispatch payload names a customer's address and a rider's identity,
	// and delivery health does not require reading either.
	all, err := store.List(context.Background(), "", "", 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered list returned %d rows", len(all))
	}
	encoded, _ := json.Marshal(all)
	if string(encoded) == "" || contains(string(encoded), "secretish") {
		t.Fatal("the event payload leaked into the operator view")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}

// EVT-011: a requeue is a REDELIVERY. The event's identity, payload and
// aggregate version survive it untouched, and the repair itself is recorded
// so a row that keeps coming back is visible as such.
func TestRequeuePreservesEventIdentityAndRecordsTheRepair(t *testing.T) {
	pool := testPool(t)
	store := &marketplace.OperatorOutboxStore{DB: pool}
	id := seedOutboxRow(t, pool, marketplace.OutboxDeadLetter, "fleet rejected the record")

	var beforeHash, beforePayload string
	var beforeVersion int64
	if err := pool.QueryRow(context.Background(), `
		SELECT payload_hash, payload::text, aggregate_version
		  FROM dispatch_marketplace_outbox WHERE id=$1::uuid`, id,
	).Scan(&beforeHash, &beforePayload, &beforeVersion); err != nil {
		t.Fatalf("read before: %v", err)
	}

	if err := store.Requeue(context.Background(), id, "admin-backoffice-service"); err != nil {
		t.Fatalf("requeue: %v", err)
	}

	var status, afterHash, afterPayload, requeuedBy string
	var afterVersion int64
	var attempts, requeues int
	var requeuedAt *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT status, payload_hash, payload::text, aggregate_version, attempt_count,
		       requeue_count, last_requeued_at, last_requeued_by
		  FROM dispatch_marketplace_outbox WHERE id=$1::uuid`, id,
	).Scan(&status, &afterHash, &afterPayload, &afterVersion, &attempts,
		&requeues, &requeuedAt, &requeuedBy); err != nil {
		t.Fatalf("read after: %v", err)
	}

	switch {
	case status != marketplace.OutboxPending:
		t.Fatalf("status = %q; want the row back on the queue", status)
	case afterHash != beforeHash || afterPayload != beforePayload:
		t.Fatal("the requeue changed the event: this is a new event, not a redelivery")
	case afterVersion != beforeVersion:
		t.Fatalf("aggregate version changed %d -> %d", beforeVersion, afterVersion)
	case attempts != 0:
		t.Fatalf("attemptCount = %d; a requeued row starts its schedule again", attempts)
	case requeues != 1 || requeuedAt == nil || requeuedBy != "admin-backoffice-service":
		t.Fatalf("the repair was not recorded: count=%d by=%q", requeues, requeuedBy)
	}

	// A second repair is counted rather than overwriting the first.
	if err := store.Requeue(context.Background(), id, "fleet-partner-service"); err != nil {
		t.Fatalf("second requeue: %v", err)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT requeue_count FROM dispatch_marketplace_outbox WHERE id=$1::uuid`, id,
	).Scan(&requeues); err != nil {
		t.Fatalf("read repair count: %v", err)
	}
	if requeues != 2 {
		t.Fatalf("requeue_count = %d after two repairs", requeues)
	}
}

// Republishing a settled fact is not a repair. A delivered row is refused.
func TestDeliveredEventCannotBeRequeued(t *testing.T) {
	pool := testPool(t)
	store := &marketplace.OperatorOutboxStore{DB: pool}
	id := seedOutboxRow(t, pool, marketplace.OutboxPublished, "")
	if _, err := pool.Exec(context.Background(),
		`UPDATE dispatch_marketplace_outbox SET published_at=now() WHERE id=$1::uuid`, id); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	if err := store.Requeue(context.Background(), id, "admin-backoffice-service"); !errors.Is(err, marketplace.ErrNotRequeueable) {
		t.Fatalf("err = %v; want marketplace.ErrNotRequeueable", err)
	}
	if err := store.Requeue(context.Background(),
		"00000000-0000-0000-0000-000000000000", "admin-backoffice-service"); !errors.Is(err, marketplace.ErrNotRequeueable) {
		t.Fatalf("unknown row err = %v; want marketplace.ErrNotRequeueable", err)
	}
}

// The surface is a security boundary: an unsigned caller and a signed but
// un-allowlisted caller are both refused, and the refusal is rendered by the
// handler itself rather than depending on a downstream error middleware.
func TestOperatorOutboxRefusesUnauthorizedCallers(t *testing.T) {
	pool := testPool(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		"admin-backoffice-service": []byte("admin-secret"),
		"vendor-service":           []byte("vendor-secret"),
	}, 5*time.Minute)
	marketplace.RegisterOperatorOutboxRoutes(engine.Group("/api/v1"),
		marketplace.NewOperatorOutboxHandler(&marketplace.OperatorOutboxStore{DB: pool},
			[]string{"admin-backoffice-service"}), verifier)

	const path = "/api/v1/internal/dispatch/v1/operations/outbox"

	unsigned := httptest.NewRecorder()
	engine.ServeHTTP(unsigned, httptest.NewRequest(http.MethodGet, path, nil))
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned read = %d; want 401", unsigned.Code)
	}

	signedByVendor := httptest.NewRequest(http.MethodGet, path, nil)
	if err := serviceauth.SignRequest(signedByVendor, "vendor-service",
		[]byte("vendor-secret"), nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	refused := httptest.NewRecorder()
	engine.ServeHTTP(refused, signedByVendor)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("un-allowlisted read = %d; want 403", refused.Code)
	}

	signedByAdmin := httptest.NewRequest(http.MethodGet, path, nil)
	if err := serviceauth.SignRequest(signedByAdmin, "admin-backoffice-service",
		[]byte("admin-secret"), nil, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	allowed := httptest.NewRecorder()
	engine.ServeHTTP(allowed, signedByAdmin)
	if allowed.Code != http.StatusOK {
		t.Fatalf("allowlisted read = %d body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestUnknownDeliveryStatusIsRefused(t *testing.T) {
	if marketplace.ValidOutboxStatus("ALMOST_SENT") {
		t.Fatal("an invented delivery status was accepted as a filter")
	}
	for _, status := range []string{"", marketplace.OutboxPending, marketplace.OutboxInFlight,
		marketplace.OutboxPublished, marketplace.OutboxFailed, marketplace.OutboxDeadLetter} {
		if !marketplace.ValidOutboxStatus(status) {
			t.Fatalf("%q is a real delivery status but was refused", status)
		}
	}
}
