package events

import (
	"context"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPostgresReconciliationRequeuesMissingVersions(t *testing.T) {
	db := movementTestPool(t)
	ctx := context.Background()
	ref := "zz-repair-" + uuid.NewString()
	emitter := &Emitter{Pool: db, SourceService: "driver-dispatch-delivery-service", EntityType: "dispatch_booking"}
	if _, err := emitter.Emit(ctx, []MovementSummary{summaryFor(ref, MovementStateInProgress)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE customer_movement_outbox SET status='DELIVERED' WHERE source_ref=$1`, ref); err != nil {
		t.Fatal(err)
	}
	secret := []byte("repair-test")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-dispatch-delivery-service": secret}, 5*time.Minute)
	projected := int64(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != CustomerMovementStatePath || !strings.Contains(r.URL.Query().Get("source_refs"), ref) {
			t.Error("wrong reconciliation query")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"versions": map[string]int64{ref: projected}}})
	}))
	defer server.Close()
	reconciler := &MovementReconciler{Pool: db, BaseURL: server.URL, ServiceName: "driver-dispatch-delivery-service", Secret: secret, After: "zz-repair-"}
	if repaired, err := reconciler.Sweep(ctx); err != nil || repaired != 1 {
		t.Fatal("repair", repaired, err)
	}
	reconciler.After = "zz-repair-"
	if repaired, err := reconciler.Sweep(ctx); err != nil || repaired != 0 {
		t.Fatal("pending repair duplicated", repaired, err)
	}
	projected = 1
	reconciler.After = "zz-repair-"
	if repaired, err := reconciler.Sweep(ctx); err != nil || repaired != 0 {
		t.Fatal("caught-up projection replayed", repaired, err)
	}
	var version int64
	var eventCount int
	if err := db.QueryRow(ctx, `SELECT max(source_version),count(DISTINCT event_id) FROM customer_movement_outbox WHERE source_ref=$1`, ref).Scan(&version, &eventCount); err != nil || version != 1 || eventCount != 2 {
		t.Fatal("business version changed", version, eventCount, err)
	}
	_, _ = db.Exec(ctx, `DELETE FROM customer_movement_outbox WHERE source_ref=$1`, ref)
	_, _ = db.Exec(ctx, `DELETE FROM customer_movement_emissions WHERE source_ref=$1`, ref)
}
