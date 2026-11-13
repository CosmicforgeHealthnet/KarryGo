package organizations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// TestFleetCompanyDecisionDelivery: Fleet submits with its own text ids, an
// operator decision is queued, and the worker delivers a signed Section 61
// envelope to Fleet; a permanent rejection is dead-lettered.
func TestFleetCompanyDecisionDelivery(t *testing.T) {
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing to reset a database whose name is not test-specific")
	}
	testdb.Lock(t, dsn)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := database.Apply(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	secret := []byte("verification-integration-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router.Group("/api/v1/verification-compliance"), New(pool, true),
		serviceauth.NewVerifier(map[string][]byte{"fleet-partner-service": secret}, 5*time.Minute), []string{"fleet-partner-service"})

	// Fleet's ids are prefixed text, not UUIDs.
	organizationID, submissionID := "org_"+strings.ReplaceAll(uuid.NewString(), "-", ""), "vrq_"+strings.ReplaceAll(uuid.NewString(), "-", "")
	submitted := verificationCall(t, router, secret, http.MethodPost, "/api/v1/verification-compliance/internal/organizations/submissions",
		map[string]any{"organizationId": organizationID, "submissionId": submissionID, "snapshot": map[string]any{"country": "NG"}, "documentFileIds": []string{"file_1"}},
		"verification:"+submissionID, uuid.NewString())
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit with Fleet ids: %d %s", submitted.Code, submitted.Body.String())
	}

	// Changes requested without saying what to change is refused.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDecision(ctx, tx, DecisionInput{OrganizationID: organizationID, SubmissionID: submissionID, Status: StatusChangesRequired, ExpectedVersion: 1}); err != ErrInvalidDecision {
		t.Fatalf("changes without issues: %v", err)
	}
	_ = tx.Rollback(ctx)

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decided, err := ApplyDecision(ctx, tx, DecisionInput{
		OrganizationID: organizationID, SubmissionID: submissionID, Status: StatusChangesRequired, ExpectedVersion: 1,
		Issues: []map[string]any{{"code": "DOCUMENT_EXPIRED", "documentReference": "TAX_CLEARANCE", "remediation": "RESUBMIT_DOCUMENT"}}, DecidedBy: "staff:s1",
	})
	if err != nil || decided.SourceVersion != 2 {
		t.Fatalf("decide: %+v %v", decided, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A fake Fleet that verifies the signature and records envelopes.
	var mu sync.Mutex
	var received []map[string]any
	reject := false
	verifier := serviceauth.NewVerifier(map[string][]byte{"verification-compliance-service": []byte("fleet-secret")}, 5*time.Minute)
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != FleetEventsPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != "verification-compliance-service" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if reject {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var envelope map[string]any
		_ = json.Unmarshal(body, &envelope)
		received = append(received, envelope)
		w.WriteHeader(http.StatusOK)
	}))
	defer fleet.Close()

	worker := NewFleetDeliveryWorker(pool, fleet.URL, "fleet-secret", 2*time.Second)
	if err := worker.Process(ctx, "test", 10); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(received) != 2 {
		mu.Unlock()
		t.Fatalf("delivered %d events, want submitted + status_changed", len(received))
	}
	var changed map[string]any
	for _, envelope := range received {
		if envelope["eventType"] == StatusChangedEventType {
			changed = envelope
		}
	}
	mu.Unlock()
	payload, _ := changed["payload"].(map[string]any)
	if changed["organizationId"] != organizationID || payload["submissionId"] != submissionID || payload["status"] != StatusChangesRequired || payload["sourceVersion"] != float64(2) || changed["producer"] != "verification-compliance-service" {
		t.Fatalf("envelope: %+v", changed)
	}
	if issues, _ := payload["issues"].([]any); len(issues) != 1 {
		t.Fatalf("issues not delivered: %+v", payload)
	}
	var delivered int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_outbox WHERE aggregate_id=$1 AND delivery_status='DELIVERED'`, organizationID).Scan(&delivered); err != nil || delivered != 2 {
		t.Fatalf("delivered rows: %d %v", delivered, err)
	}

	// Fleet refuses the next decision permanently: it is dead-lettered, not
	// retried forever.
	mu.Lock()
	reject = true
	mu.Unlock()
	tx, _ = pool.Begin(ctx)
	if _, err := ApplyDecision(ctx, tx, DecisionInput{OrganizationID: organizationID, SubmissionID: submissionID, Status: StatusVerified, ExpectedVersion: 2}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	if err := worker.Process(ctx, "test", 10); err != nil {
		t.Fatal(err)
	}
	var dead int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_outbox WHERE aggregate_id=$1 AND delivery_status='DEAD_LETTER' AND last_error='FLEET_REJECTED'`, organizationID).Scan(&dead); err != nil || dead != 1 {
		t.Fatalf("dead letters: %d %v", dead, err)
	}
}
