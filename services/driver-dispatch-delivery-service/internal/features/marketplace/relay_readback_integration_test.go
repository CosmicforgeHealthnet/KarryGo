package marketplace_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/providerreceipt"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5"
)

func TestRelayAuthoritativeReadbackNeverRepeatsUnknownMutation(t *testing.T) {
	for _, assignment := range []string{"asg-receipt", "pas_private-receipt"} {
		t.Run(assignment, func(t *testing.T) { testRelayAuthoritativeReadback(t, assignment) })
	}
}

func testRelayAuthoritativeReadback(t *testing.T, assignment string) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	provider := seedProvider(t, pool, "+2348030070011")
	key := "original-receipt-key"
	intent := providerreceipt.Intent{AssignmentID: assignment, ProviderID: provider, ClientResponseID: key, Decision: "ACCEPTED"}
	receipt := providerreceipt.Receipt{ContractVersion: providerreceipt.Version, ReceiptID: "ars-original", AssignmentID: assignment, ProviderID: provider, ClientResponseID: key, Decision: "ACCEPTED", IntentDigest: intent.Digest(), RecordedAt: time.Now().UTC()}
	secret := []byte("receipt-execution-test-only-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{dispatchService: secret}, time.Minute)
	var posts, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := verifier.VerifyRequest(r)
		if err != nil || caller != dispatchService {
			t.Error("unsigned read-back or mutation")
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(503)
			return
		} // owner committed; transport response is uncertain
		reads.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/internal/fleet-partner/v1/assignments/"+assignment+"/provider-decision-receipt" || r.URL.Query().Get("clientResponseId") != key || r.URL.Query().Get("providerId") != provider {
			t.Error("read-back changed original identity")
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": receipt})
	}))
	defer server.Close()
	responder, _ := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{BaseURL: server.URL, ServiceName: dispatchService, Secret: secret})
	payload, _ := json.Marshal(map[string]any{"providerId": provider, "providerService": "DISPATCH", "clientResponseId": key, "providerSessionId": "session-original"})
	var op *marketplace.RelayOperation
	if err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
		var err error
		op, _, err = repo.CreateRelay(ctx, tx, &marketplace.RelayOperation{OperationType: marketplace.RelayProviderAssignmentAccept, ProviderID: provider, AssignmentID: &assignment, IdempotencyKey: key, RequestFingerprint: "fp-original", Payload: payload})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{Owner: "same-replica-label"})
	if result, err := worker.RunOnce(ctx); err != nil || result.Unknown != 1 {
		t.Fatalf("ambiguous commit: %+v %v", result, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET next_attempt_at=now() WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var confirmed atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{Owner: "same-replica-label"}).RunOnce(ctx)
			if err != nil {
				t.Error(err)
			}
			confirmed.Add(int32(result.Reconciled))
		}()
	}
	wg.Wait()
	stored, err := repo.GetRelayByID(ctx, nil, op.ID)
	if err != nil || stored.Status != marketplace.RelayConfirmed || stored.NeedsReadBack() || confirmed.Load() != 1 || posts.Load() != 1 || reads.Load() != 1 {
		t.Fatalf("read-back failed or mutation replayed: %+v confirmed=%d posts=%d reads=%d err=%v", stored, confirmed.Load(), posts.Load(), reads.Load(), err)
	}
	if !strings.Contains(string(stored.ResponseSnapshot), receipt.ReceiptID) {
		t.Fatal("durable canonical receipt missing")
	}
}

func TestRelayReadbackDeniesMissingOrMismatchedDecision(t *testing.T) {
	for _, failure := range []string{"missing", "provider", "key", "assignment", "decision", "digest", "note", "evidence", "malformed", "extra-field", "redirect", "oversize"} {
		t.Run(failure, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			repo := marketplace.NewRepository(pool)
			provider := seedProvider(t, pool, "+2348030070012")
			assignment, key := "asg-negative-receipt", "original-key"
			intent := providerreceipt.Intent{AssignmentID: assignment, ProviderID: provider, ClientResponseID: key, Decision: "DECLINED", ReasonCode: "UNSAFE_VEHICLE", Note: "private-note", Evidence: []string{"evidence-original"}}
			receipt := providerreceipt.Receipt{ContractVersion: providerreceipt.Version, ReceiptID: "ars-negative", AssignmentID: assignment, ProviderID: provider, ClientResponseID: key, Decision: intent.Decision, IntentDigest: intent.Digest(), RecordedAt: time.Now().UTC()}
			switch failure {
			case "provider":
				receipt.ProviderID = "other"
			case "key":
				receipt.ClientResponseID = "other"
			case "assignment":
				receipt.AssignmentID = "other"
			case "decision":
				receipt.Decision = "ACCEPTED"
			case "digest":
				receipt.IntentDigest = "bad"
			case "note":
				intent.Note = "changed"
				receipt.IntentDigest = intent.Digest()
			case "evidence":
				intent.Evidence = []string{"different"}
				receipt.IntentDigest = intent.Digest()
			}
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					posts.Add(1)
					w.WriteHeader(500)
					return
				}
				switch failure {
				case "missing":
					w.WriteHeader(404)
					return
				case "redirect":
					http.Redirect(w, r, "/mutation", 302)
					return
				case "malformed":
					w.Write([]byte(`{"data":`))
					return
				case "extra-field":
					w.Write([]byte(`{"data":{},"rawEvidence":"forbidden"}`))
					return
				case "oversize":
					w.Write([]byte(strings.Repeat("x", (1<<20)+1)))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": receipt})
			}))
			defer server.Close()
			responder, _ := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{BaseURL: server.URL, ServiceName: dispatchService, Secret: []byte("receipt-negative-test-only")})
			payload, _ := json.Marshal(map[string]any{"providerId": provider, "providerService": "DISPATCH", "clientResponseId": key, "reasonCode": "UNSAFE_VEHICLE", "note": "private-note", "evidenceFileIds": []string{"evidence-original"}})
			var op *marketplace.RelayOperation
			if err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
				var err error
				op, _, err = repo.CreateRelay(ctx, tx, &marketplace.RelayOperation{OperationType: marketplace.RelayProviderAssignmentDecline, ProviderID: provider, AssignmentID: &assignment, IdempotencyKey: key, RequestFingerprint: "negative", Payload: payload})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := repo.MarkRelayUnknown(ctx, nil, op.ID, "ambiguous", 0, 1); err != nil {
				t.Fatal(err)
			}
			worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{})
			if result, err := worker.RunOnce(ctx); err != nil || result.Reconciled != 0 || result.Claimed != 0 {
				t.Fatalf("false confirmation: %+v %v", result, err)
			}
			stored, err := repo.GetRelayByID(ctx, nil, op.ID)
			if err != nil || !stored.NeedsReadBack() || stored.Status != marketplace.RelayUnknownOutcome || posts.Load() != 0 {
				t.Fatalf("uncertainty fence lost: %+v %v", stored, err)
			}
			if err := repo.ReplayRelay(ctx, op.ID); err == nil {
				t.Fatal("missing receipt enabled mutation replay")
			}
		})
	}
}
