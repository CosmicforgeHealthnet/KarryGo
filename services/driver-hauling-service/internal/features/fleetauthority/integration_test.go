package fleetauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestProviderAndTruckLinkLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for authority integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	providerID, truckID := uuid.NewString(), uuid.NewString()
	_, err = pool.Exec(context.Background(), `INSERT INTO truck_providers(id,phone,email,status,onboarding_status,service_type) VALUES($1,$2,$3,'active','complete','hauling')`, providerID, "+234"+providerID[:10], providerID+"@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status) VALUES($1,$2,'flatbed',1000,$3,'active')`, truckID, providerID, "TEST-"+truckID[:8])
	if err != nil {
		t.Fatal(err)
	}

	secret := []byte("integration-only-hmac-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	root := router.Group("/api/v1/hauling")
	Register(root, New(pool, Flags{Enabled: true}), serviceauth.NewVerifier(map[string][]byte{"fleet-partner-service": secret}, 5*time.Minute), []string{"fleet-partner-service"})

	organizationID, membershipID, branchID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	keyPrefix := uuid.NewString()
	providerBody := map[string]any{"organizationId": organizationID, "membershipId": membershipID, "providerId": providerID, "branchId": branchID}
	created := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links", providerBody, keyPrefix+"-provider-create", uuid.NewString(), http.StatusCreated)
	linkID := nestedString(t, created, "linkId")
	version := nestedVersion(t, created)
	replayed := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links", providerBody, keyPrefix+"-provider-create", uuid.NewString(), http.StatusOK)
	if nestedString(t, replayed, "linkId") != linkID {
		t.Fatal("idempotent request returned a different provider link")
	}
	conflict := map[string]any{"organizationId": uuid.NewString(), "membershipId": uuid.NewString(), "providerId": providerID}
	authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links", conflict, keyPrefix+"-provider-create", uuid.NewString(), http.StatusConflict)

	activeBody := map[string]any{"state": "ACTIVE", "expectedVersion": version, "organizationId": organizationID, "reasonCode": "TEST"}
	activeKey := keyPrefix + "-provider-active"
	result := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID+"/transitions", activeBody, activeKey, uuid.NewString(), http.StatusOK)
	version = nestedVersion(t, result)
	replayedTransition := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID+"/transitions", activeBody, activeKey, uuid.NewString(), http.StatusOK)
	if nestedVersion(t, replayedTransition) != version {
		t.Fatal("idempotent transition replay changed the provider link version")
	}
	conflictingTransition := map[string]any{"state": "REVOKED", "expectedVersion": version - 1, "organizationId": organizationID, "reasonCode": "TEST"}
	authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID+"/transitions", conflictingTransition, activeKey, uuid.NewString(), http.StatusConflict)

	for _, state := range []string{"SUSPENDED", "ACTIVE", "REVOKED"} {
		result := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID+"/transitions", map[string]any{"state": state, "expectedVersion": version, "organizationId": organizationID, "reasonCode": "TEST"}, keyPrefix+"-provider-"+state+uuid.NewString(), uuid.NewString(), http.StatusOK)
		version = nestedVersion(t, result)
	}

	truckBody := map[string]any{"organizationId": organizationID, "membershipId": membershipID, "truckId": truckID, "branchId": branchID}
	truck := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/truck-links", truckBody, keyPrefix+"-truck-create", uuid.NewString(), http.StatusCreated)
	truckLinkID := nestedString(t, truck, "linkId")
	truckVersion := nestedVersion(t, truck)
	newBranch := uuid.NewString()
	truck = authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/truck-links/"+truckLinkID+"/transitions", map[string]any{"state": "ACTIVE", "expectedVersion": truckVersion, "organizationId": organizationID, "branchId": newBranch}, keyPrefix+"-truck-active", uuid.NewString(), http.StatusOK)
	if nestedString(t, truck, "branchId") != newBranch {
		t.Fatal("truck branch reassignment was not persisted")
	}

	// The exact same signed nonce is rejected, and changing the nonce without
	// recomputing the signature is rejected by the shared v2 contract.
	request := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", keyPrefix+"-replay-nonce")
	request.Header.Set("X-Organization-ID", organizationID)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, request)
	if first.Code != http.StatusOK {
		t.Fatalf("first nonce request status=%d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, request.Clone(context.Background()))
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("nonce replay status=%d", second.Code)
	}
	tampered := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", "signed-nonce")
	tampered.Header.Set("X-Organization-ID", organizationID)
	tampered.Header.Set(serviceauth.HeaderNonce, "changed-without-resigning")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, tampered)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("tampered nonce status=%d", response.Code)
	}
	missingNonce := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", uuid.NewString())
	missingNonce.Header.Del(serviceauth.HeaderNonce)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, missingNonce)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing nonce status=%d", response.Code)
	}
	expired := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", uuid.NewString())
	if err := serviceauth.SignRequest(expired, "fleet-partner-service", secret, nil, time.Now().Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, expired)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired signature status=%d", response.Code)
	}
	foreign := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", uuid.NewString())
	if err := serviceauth.SignRequest(foreign, "unapproved-service", secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, foreign)
	if response.Code != http.StatusForbidden {
		t.Fatalf("caller allowlist status=%d", response.Code)
	}
	crossTenant := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", uuid.NewString())
	crossTenant.Header.Set("X-Organization-ID", uuid.NewString())
	response = httptest.NewRecorder()
	router.ServeHTTP(response, crossTenant)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read status=%d", response.Code)
	}
}

func authorityCall(t *testing.T, router http.Handler, secret []byte, method, path string, body any, key, nonce string, want int) map[string]any {
	t.Helper()
	request := signedAuthorityRequest(t, secret, method, path, body, key, nonce)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, want, response.Body.String())
	}
	var decoded map[string]any
	if response.Body.Len() > 0 {
		if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
	}
	return decoded
}

func signedAuthorityRequest(t *testing.T, secret []byte, method, path string, body any, key, nonce string) *http.Request {
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
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if object, ok := body.(map[string]any); ok {
		if organizationID, ok := object["organizationId"].(string); ok {
			request.Header.Set("X-Organization-ID", organizationID)
		}
	}
	if nonce != "" {
		request.Header.Set(serviceauth.HeaderNonce, nonce)
	}
	if err := serviceauth.SignRequest(request, "fleet-partner-service", secret, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	return request
}

func nestedString(t *testing.T, response map[string]any, key string) string {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", response)
	}
	value, _ := data[key].(string)
	if value == "" {
		t.Fatalf("missing %s: %#v", key, data)
	}
	return value
}

func nestedVersion(t *testing.T, response map[string]any) int64 {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", response)
	}
	value, ok := data["sourceVersion"].(float64)
	if !ok {
		t.Fatalf("missing version: %#v", data)
	}
	return int64(value)
}
