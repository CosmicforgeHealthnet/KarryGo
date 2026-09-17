package events

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func movementTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("MOVEMENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOVEMENT_TEST_DATABASE_URL is required for movement emission certification")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile(filepath.Join("migrations", "customer_movement_outbox.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(schema)); err != nil {
		t.Fatalf("apply canonical movement schema: %v", err)
	}
	return pool
}

func summaryFor(ref, state string) MovementSummary {
	return MovementSummary{CustomerID: "customer-" + ref, Authority: MovementAuthorityDispatch, SourceRef: ref,
		MovementType: MovementTypeDispatch, State: state}
}

// Emission is a function of committed state: the same view emits once, a
// changed view emits again with the next source_version, and nothing is ever
// published for a movement the authority does not hold.
func TestPostgresEmissionIsIdempotentAndMonotonic(t *testing.T) {
	pool := movementTestPool(t)
	ctx := context.Background()
	ref := uuid.NewString()
	emitter := &Emitter{Pool: pool, SourceService: "driver-dispatch-delivery-service", EntityType: "dispatch_booking"}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM customer_movement_outbox WHERE source_ref=$1`, ref)
		_, _ = pool.Exec(ctx, `DELETE FROM customer_movement_emissions WHERE source_ref=$1`, ref)
	})

	searching := summaryFor(ref, MovementStateSearching)
	if result, err := emitter.Emit(ctx, []MovementSummary{searching}); err != nil || result.Emitted != 1 {
		t.Fatalf("first emission = %+v (%v)", result, err)
	}
	if result, err := emitter.Emit(ctx, []MovementSummary{searching}); err != nil || result.Emitted != 0 || result.Skipped != 1 {
		t.Fatalf("unchanged state must not republish: %+v (%v)", result, err)
	}
	if result, err := emitter.Emit(ctx, []MovementSummary{summaryFor(ref, MovementStateAssigned)}); err != nil || result.Emitted != 1 {
		t.Fatalf("changed state = %+v (%v)", result, err)
	}

	rows, err := pool.Query(ctx, `SELECT source_version, payload FROM customer_movement_outbox WHERE source_ref=$1 ORDER BY source_version`, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []int64
	var names []string
	for rows.Next() {
		var version int64
		var payload []byte
		if err = rows.Scan(&version, &payload); err != nil {
			t.Fatal(err)
		}
		var envelope Envelope
		if err = json.Unmarshal(payload, &envelope); err != nil {
			t.Fatal(err)
		}
		if err = envelope.Validate(); err != nil {
			t.Fatalf("queued an envelope a consumer would reject: %v", err)
		}
		versions = append(versions, version)
		names = append(names, envelope.EventName)
	}
	if len(versions) != 2 || versions[0] != 1 || versions[1] != 2 {
		t.Fatalf("source versions = %v, want a monotonic 1,2", versions)
	}
	if names[0] != "dispatch.delivery.created" || names[1] != "dispatch.delivery.assigned" {
		t.Fatalf("event names = %v", names)
	}
	if _, err = emitter.Emit(ctx, []MovementSummary{{CustomerID: "c", Authority: MovementAuthorityDispatch, SourceRef: ref, State: "NOT_A_STATE", MovementType: MovementTypeDispatch}}); err == nil {
		t.Fatal("an unmappable state must not be published")
	}
}

// Delivery is at-least-once with bounded retries, and a refused delivery is
// retried rather than acknowledged.
func TestPostgresPublisherDeliversSignedAndRetries(t *testing.T) {
	pool := movementTestPool(t)
	ctx := context.Background()
	ref := uuid.NewString()
	secret := []byte("movement-publisher-secret-0123456789")
	emitter := &Emitter{Pool: pool, SourceService: "driver-hauling-service", EntityType: "haulage_booking"}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM customer_movement_outbox WHERE source_ref=$1`, ref)
		_, _ = pool.Exec(ctx, `DELETE FROM customer_movement_emissions WHERE source_ref=$1`, ref)
	})
	summary := summaryFor(ref, MovementStateCompleted)
	summary.Authority = MovementAuthorityHauling
	summary.MovementType = MovementTypeHaulage
	if _, err := emitter.Emit(ctx, []MovementSummary{summary}); err != nil {
		t.Fatal(err)
	}

	verifier := serviceauth.NewVerifier(map[string][]byte{"driver-hauling-service": secret}, 5*time.Minute)
	refuse := true
	var signedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if refuse {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		signedPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	publisher := &MovementPublisher{Pool: pool, Owner: "test",
		Deliverer: &HTTPDeliverer{BaseURL: server.URL, Path: CustomerMovementIngressPath, ServiceName: "driver-hauling-service", Secret: secret}}

	if delivered, err := publisher.PublishBatch(ctx); err != nil || delivered != 0 {
		t.Fatalf("refused delivery = %d (%v)", delivered, err)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM customer_movement_outbox WHERE source_ref=$1`, ref).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" || attempts != 1 {
		t.Fatalf("after a refusal status=%s attempts=%d, want a retry", status, attempts)
	}

	refuse = false
	if _, err := pool.Exec(ctx, `UPDATE customer_movement_outbox SET next_attempt_at=now() - interval '1 second' WHERE source_ref=$1`, ref); err != nil {
		t.Fatal(err)
	}
	if delivered, err := publisher.PublishBatch(ctx); err != nil || delivered != 1 {
		t.Fatalf("delivery = %d (%v)", delivered, err)
	}
	if signedPath != CustomerMovementIngressPath {
		t.Fatalf("delivered to %q", signedPath)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM customer_movement_outbox WHERE source_ref=$1`, ref).Scan(&status); err != nil || status != "DELIVERED" {
		t.Fatalf("status = %s (%v)", status, err)
	}
	if delivered, err := publisher.PublishBatch(ctx); err != nil || delivered != 0 {
		t.Fatalf("a delivered row must not be re-claimed: %d (%v)", delivered, err)
	}
}
