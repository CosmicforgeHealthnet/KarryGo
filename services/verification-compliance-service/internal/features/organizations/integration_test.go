package organizations

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestOrganizationVerificationLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing to reset a database whose name is not test-specific")
	}
	testdb.Lock(t, dsn)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := database.Apply(context.Background(), pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	secret := []byte("verification-integration-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	root := router.Group("/api/v1/verification-compliance")
	Register(root, New(pool, true), serviceauth.NewVerifier(map[string][]byte{"fleet-partner-service": secret}, 5*time.Minute), []string{"fleet-partner-service"})

	organizationID, submissionID := uuid.NewString(), uuid.NewString()
	submitPath := "/api/v1/verification-compliance/internal/organizations/submissions"
	submitBody := map[string]any{"organizationId": organizationID, "submissionId": submissionID, "snapshot": map[string]any{"country": "NG"}, "documentFileIds": []string{uuid.NewString()}}
	first := verificationCall(t, router, secret, http.MethodPost, submitPath, submitBody, "submit-"+submissionID, uuid.NewString())
	if first.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", first.Code, first.Body.String())
	}
	replay := verificationCall(t, router, secret, http.MethodPost, submitPath, submitBody, "submit-"+submissionID, uuid.NewString())
	if replay.Code != http.StatusAccepted {
		t.Fatalf("submit replay status=%d body=%s", replay.Code, replay.Body.String())
	}

	decisionPath := "/api/v1/verification-compliance/internal/organizations/" + organizationID + "/submissions/" + submissionID + "/decisions"
	decision := map[string]any{"status": "VERIFIED", "restrictionStatus": "NONE", "expectedVersion": 1, "issues": []any{}}
	decided := verificationCall(t, router, secret, http.MethodPost, decisionPath, decision, "decision-"+submissionID, uuid.NewString())
	if decided.Code != http.StatusOK {
		t.Fatalf("decision status=%d body=%s", decided.Code, decided.Body.String())
	}
	decisionReplay := verificationCall(t, router, secret, http.MethodPost, decisionPath, decision, "decision-"+submissionID, uuid.NewString())
	if decisionReplay.Code != http.StatusOK {
		t.Fatalf("decision replay status=%d body=%s", decisionReplay.Code, decisionReplay.Body.String())
	}
	conflict := map[string]any{"status": "SUSPENDED", "restrictionStatus": "SUSPENDED", "expectedVersion": 1, "issues": []any{}}
	conflicting := verificationCall(t, router, secret, http.MethodPost, decisionPath, conflict, "decision-"+submissionID, uuid.NewString())
	if conflicting.Code != http.StatusConflict {
		t.Fatalf("decision fingerprint conflict status=%d body=%s", conflicting.Code, conflicting.Body.String())
	}

	readPath := "/api/v1/verification-compliance/internal/organizations/" + organizationID + "/submissions/" + submissionID
	read := verificationCall(t, router, secret, http.MethodGet, readPath, nil, "", uuid.NewString())
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"status":"VERIFIED"`) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
	var outboxRows int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM verification_outbox WHERE aggregate_id=$1`, organizationID).Scan(&outboxRows); err != nil || outboxRows != 2 {
		t.Fatalf("outbox rows=%d err=%v", outboxRows, err)
	}
}

func verificationCall(t *testing.T, router http.Handler, secret []byte, method, path string, body any, key, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Correlation-ID", uuid.NewString())
	request.Header.Set("X-Causation-ID", uuid.NewString())
	request.Header.Set(serviceauth.HeaderNonce, nonce)
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if err := serviceauth.SignRequest(request, "fleet-partner-service", secret, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
