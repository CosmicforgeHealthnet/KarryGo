package vendors

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestVendorCaseAuthorityHMACReplayIdempotencyAndHistory(t *testing.T) {
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	parsed, e := url.Parse(dsn)
	if e != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	testdb.Lock(t, dsn)
	pool, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(context.Background(), `DROP SCHEMA public CASCADE;CREATE SCHEMA public`); e != nil {
		t.Fatal(e)
	}
	if e = database.Apply(context.Background(), pool, "../../../migrations"); e != nil {
		t.Fatal(e)
	}
	vendorSecret := []byte("vendor-secret")
	adminSecret := []byte("admin-secret")
	router := gin.New()
	root := router.Group("/api/v1/verification-compliance")
	Register(root, New(pool, true, []string{"vendor-service", "admin-backoffice-service"}), serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": vendorSecret, "admin-backoffice-service": adminSecret}, 5*time.Minute))
	clientRef := "vcase_" + uuid.NewString()
	body := map[string]any{"client_reference": clientRef, "business_id": "vendor-1", "dimension": "BUSINESS", "subject_type": "BUSINESS", "subject_ref": "vendor-1", "document_refs": []string{"vdoc_1"}}
	path := "/api/v1/verification-compliance/internal/vendor-cases/cases"
	first := call(t, router, "vendor-service", vendorSecret, http.MethodPost, path, body, clientRef, "nonce-create-1")
	if first.Code != 201 {
		t.Fatalf("create=%d %s", first.Code, first.Body.String())
	}
	replay := call(t, router, "vendor-service", vendorSecret, http.MethodPost, path, body, clientRef, "nonce-create-2")
	if replay.Code != 201 {
		t.Fatalf("idempotent replay=%d %s", replay.Code, replay.Body.String())
	}
	nonceReplay := call(t, router, "vendor-service", vendorSecret, http.MethodGet, "/api/v1/verification-compliance/internal/vendor-cases/cases/by-client-reference/"+clientRef, nil, "", "nonce-create-2")
	if nonceReplay.Code != 401 {
		t.Fatalf("nonce replay=%d", nonceReplay.Code)
	}
	read := call(t, router, "vendor-service", vendorSecret, http.MethodGet, "/api/v1/verification-compliance/internal/vendor-cases/cases/by-client-reference/"+clientRef, nil, "", "nonce-read")
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"status":"PENDING_REVIEW"`) {
		t.Fatalf("read=%d %s", read.Code, read.Body.String())
	}
	var envelope struct {
		Data view `json:"data"`
	}
	if e = json.Unmarshal(first.Body.Bytes(), &envelope); e != nil {
		t.Fatal(e)
	}
	caseID := envelope.Data.SourceRef
	// Section 18: PENDING_REVIEW -> PENDING_REVIEW is not a transition of the
	// case state machine (the pre-Section-18 route accepted it); a reviewer
	// picking the case up is PENDING_REVIEW -> IN_PROGRESS.
	decision := map[string]any{"status": "IN_PROGRESS", "reason_code": "", "remediation": "", "expected_source_version": 1}
	decisionPath := "/api/v1/verification-compliance/internal/vendor-cases/cases/" + caseID + "/decisions"
	forbidden := call(t, router, "vendor-service", vendorSecret, http.MethodPost, decisionPath, decision, "decision-1", "nonce-vendor-decision")
	if forbidden.Code != 403 {
		t.Fatalf("vendor spoofed decision=%d", forbidden.Code)
	}
	plainPending := call(t, router, "admin-backoffice-service", adminSecret, http.MethodPost, decisionPath, map[string]any{"status": "PENDING", "expected_source_version": 1}, "decision-plain-pending", "nonce-admin-plain-pending")
	if plainPending.Code != http.StatusBadRequest {
		t.Fatalf("plain PENDING verification status=%d body=%s", plainPending.Code, plainPending.Body.String())
	}
	decided := call(t, router, "admin-backoffice-service", adminSecret, http.MethodPost, decisionPath, decision, "decision-1", "nonce-admin-decision")
	if decided.Code != 200 || !strings.Contains(decided.Body.String(), `"source_version":2`) {
		t.Fatalf("decision=%d %s", decided.Code, decided.Body.String())
	}
	stale := call(t, router, "admin-backoffice-service", adminSecret, http.MethodPost, decisionPath, map[string]any{"status": "SUSPENDED", "expected_source_version": 1}, "decision-stale", "nonce-admin-stale")
	if stale.Code != 409 {
		t.Fatalf("stale decision=%d %s", stale.Code, stale.Body.String())
	}
	var history, outbox int
	if e = pool.QueryRow(context.Background(), `SELECT count(*) FROM vendor_verification_case_history WHERE case_id=$1`, caseID).Scan(&history); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_outbox WHERE aggregate_id=$1`, caseID).Scan(&outbox); e != nil {
		t.Fatal(e)
	}
	if history != 2 || outbox != 2 {
		t.Fatalf("history=%d outbox=%d", history, outbox)
	}
	var deliveries atomic.Int32
	callbackSecret := []byte("callback-secret")
	callbackVerifier := serviceauth.NewVerifier(serviceauth.Secrets{"verification-compliance-service": callbackSecret}, 5*time.Minute)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/vendor/v1/verification-events" {
			http.Error(w, "unexpected callback path", http.StatusNotFound)
			return
		}
		if _, err := callbackVerifier.VerifyRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var event map[string]any
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil || event["event_id"] == "" || event["status"] != "IN_PROGRESS" {
			http.Error(w, "invalid callback", http.StatusBadRequest)
			return
		}
		deliveries.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer callback.Close()
	worker := NewCallbackWorker(pool, callback.URL, string(callbackSecret), time.Second)
	if e = worker.Process(context.Background(), "test-worker", 10); e != nil {
		t.Fatal(e)
	}
	if deliveries.Load() != 1 {
		t.Fatalf("callback deliveries=%d want 1", deliveries.Load())
	}
	var callbackID, deliveryStatus string
	if e = pool.QueryRow(context.Background(), `SELECT id::text,delivery_status FROM verification_outbox WHERE event_type='vendor.verification.status_changed'`).Scan(&callbackID, &deliveryStatus); e != nil || deliveryStatus != "DELIVERED" {
		t.Fatalf("callback status=%s err=%v", deliveryStatus, e)
	}
	if e = worker.Process(context.Background(), "test-worker", 10); e != nil || deliveries.Load() != 1 {
		t.Fatalf("delivered callback replayed deliveries=%d err=%v", deliveries.Load(), e)
	}
	if _, e = pool.Exec(context.Background(), `UPDATE verification_outbox SET delivery_status='DEAD_LETTER' WHERE id=$1`, callbackID); e != nil {
		t.Fatal(e)
	}
	if e = worker.Replay(context.Background(), callbackID); e != nil {
		t.Fatal(e)
	}
	if e = worker.Process(context.Background(), "test-worker", 10); e != nil || deliveries.Load() != 2 {
		t.Fatalf("operator replay deliveries=%d err=%v", deliveries.Load(), e)
	}
}

func call(t *testing.T, router http.Handler, caller string, secret []byte, method, path string, body any, key, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var e error
		raw, e = json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(serviceauth.HeaderNonce, nonce)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if e := serviceauth.SignRequest(request, caller, secret, raw, time.Now()); e != nil {
		t.Fatal(e)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestVendorCaseMigrationRollbackAndReapply(t *testing.T) {
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	testdb.Lock(t, dsn)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(context.Background(), `DROP SCHEMA public CASCADE;CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = database.Apply(context.Background(), pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	// Later migrations depend on the case tables, so roll back in reverse
	// order (Section 18 added 000004 on top of 000002).
	for _, name := range []string{"000004_secure_admin_boundary.down.sql", "000002_vendor_verification_cases.down.sql"} {
		down, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(context.Background(), string(down)); err != nil {
			t.Fatalf("rollback %s: %v", name, err)
		}
	}
	var exists bool
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.vendor_verification_cases') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("Vendor case table survived rollback exists=%t err=%v", exists, err)
	}
	for _, name := range []string{"000002_vendor_verification_cases.up.sql", "000004_secure_admin_boundary.up.sql"} {
		up, err := os.ReadFile("../../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(context.Background(), string(up)); err != nil {
			t.Fatalf("reapply %s: %v", name, err)
		}
	}
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.vendor_verification_cases') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("Vendor case table missing after reapply exists=%t err=%v", exists, err)
	}
	var deliveryColumn bool
	if err = pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='verification_outbox' AND column_name='delivery_status')`).Scan(&deliveryColumn); err != nil || !deliveryColumn {
		t.Fatalf("delivery state missing after reapply exists=%t err=%v", deliveryColumn, err)
	}
	for _, constraint := range []string{"vendor_verification_cases_status_check", "vendor_verification_case_history_status_check"} {
		var definition string
		if err = pool.QueryRow(context.Background(), `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, status := range []string{"NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"} {
			if !strings.Contains(definition, "'"+status+"'") {
				t.Fatalf("%s does not contain %s: %s", constraint, status, definition)
			}
		}
		if strings.Contains(definition, "'PENDING'") {
			t.Fatalf("%s incorrectly permits plain PENDING: %s", constraint, definition)
		}
	}
}
