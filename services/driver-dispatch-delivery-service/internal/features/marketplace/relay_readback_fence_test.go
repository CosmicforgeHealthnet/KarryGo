package marketplace_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/providerreceipt"
)

func TestRelayReadbackExpiredReaderCannotFinishNewLease(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	provider := seedProvider(t, pool, "+2348030070013")
	op := seedRelay(t, pool, provider, "asg-read-lease", "read-lease-key", "read-lease-fp")
	payload, _ := json.Marshal(map[string]string{"providerId": provider, "providerService": "DISPATCH", "clientResponseId": op.IdempotencyKey})
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET payload=$2::jsonb WHERE id=$1`, op.ID, payload); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRelayUnknown(ctx, nil, op.ID, "uncertain", 0, 1); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	intent := providerreceipt.Intent{AssignmentID: *op.AssignmentID, ProviderID: provider, ClientResponseID: op.IdempotencyKey, Decision: "ACCEPTED"}
	receipt := providerreceipt.Receipt{ContractVersion: providerreceipt.Version, ReceiptID: "ars-read-lease", AssignmentID: intent.AssignmentID, ProviderID: provider, ClientResponseID: intent.ClientResponseID, Decision: intent.Decision, IntentDigest: intent.Digest(), RecordedAt: time.Now().UTC()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("read-back issued mutation")
			w.WriteHeader(500)
			return
		}
		if reads.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": receipt})
	}))
	defer server.Close()
	responder, _ := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{BaseURL: server.URL, ServiceName: dispatchService, Secret: []byte("lease-only-test-secret")})
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{Owner: "identical-owner-label"})
	finished := make(chan error, 1)
	go func() { _, err := worker.RunOnce(ctx); finished <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not start")
	}
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	result, err := worker.RunOnce(ctx)
	close(release)
	if err != nil || result.Reconciled != 1 {
		t.Fatalf("new lease failed: %+v %v", result, err)
	}
	if err := <-finished; err == nil {
		t.Fatal("expired reader modified newer lease")
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_readback_audit WHERE operation_id=$1`, op.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count=%d err=%v", audits, err)
	}
}

func TestRelayReadbackAuditFailureCannotConfirm(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	provider := seedProvider(t, pool, "+2348030070014")
	op := seedRelay(t, pool, provider, "asg-audit-fault", "audit-key", "audit-fp")
	payload, _ := json.Marshal(map[string]string{"providerId": provider, "providerService": "DISPATCH", "clientResponseId": op.IdempotencyKey})
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET payload=$2::jsonb WHERE id=$1`, op.ID, payload); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRelayUnknown(ctx, nil, op.ID, "uncertain", 0, 1); err != nil {
		t.Fatal(err)
	}
	intent := providerreceipt.Intent{AssignmentID: *op.AssignmentID, ProviderID: provider, ClientResponseID: op.IdempotencyKey, Decision: "ACCEPTED"}
	receipt := providerreceipt.Receipt{ContractVersion: providerreceipt.Version, ReceiptID: "ars-audit", AssignmentID: intent.AssignmentID, ProviderID: provider, ClientResponseID: intent.ClientResponseID, Decision: intent.Decision, IntentDigest: intent.Digest(), RecordedAt: time.Now().UTC()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unexpected mutation")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": receipt})
	}))
	defer server.Close()
	responder, _ := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{BaseURL: server.URL, ServiceName: dispatchService, Secret: []byte("audit-only-test-secret")})
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{})
	if _, err := pool.Exec(ctx, `CREATE FUNCTION test_readback_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test-only audit failure'; END $$; CREATE TRIGGER test_readback_audit_failure BEFORE INSERT ON dispatch_fleet_relay_readback_audit FOR EACH ROW EXECUTE FUNCTION test_readback_audit_failure()`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_readback_audit_failure ON dispatch_fleet_relay_readback_audit; DROP FUNCTION IF EXISTS test_readback_audit_failure()`)
	if _, err := worker.RunOnce(ctx); err == nil {
		t.Fatal("confirmation succeeded without audit")
	}
	stored, err := repo.GetRelayByID(ctx, nil, op.ID)
	if err != nil || stored.Status != marketplace.RelayUnknownOutcome || !stored.ReadbackRequired || len(stored.ResponseSnapshot) > 0 {
		t.Fatalf("audit failure released fence: %+v %v", stored, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER test_readback_audit_failure ON dispatch_fleet_relay_readback_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := worker.RunOnce(ctx); err != nil || result.Reconciled != 1 {
		t.Fatalf("safe GET retry failed: %+v %v", result, err)
	}
	for _, query := range []string{`UPDATE dispatch_fleet_relay_readback_audit SET outcome='UNRESOLVED' WHERE operation_id=$1`, `DELETE FROM dispatch_fleet_relay_readback_audit WHERE operation_id=$1`} {
		if _, err := pool.Exec(ctx, query, op.ID); err == nil {
			t.Fatal("immutable audit changed")
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE dispatch_fleet_relay_readback_audit`); err == nil {
		t.Fatal("immutable audit truncated")
	}
}
