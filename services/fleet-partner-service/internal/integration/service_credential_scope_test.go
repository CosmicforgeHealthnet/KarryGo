package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	sharedserviceauth "cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	platformserviceauth "cosmicforge/logistics/services/fleet-partner-service/internal/platform/serviceauth"
)

const (
	scopeTestCaller = "scope-test-service"
	scopeTestSecret = "scope-test-service-secret-value"
)

// credentialScopeRouter mounts the real internal route class (through the
// real app.BuildRouter and middleware.InternalServiceScopeAuth -- what a
// deployment actually runs with ServiceCredentialScopingEnabled=true), with
// two dedicated test-only routes. Not part of the real
// internal/platform/riskregistry: ScopeVerifier.Authorize is a plain
// in-memory scope check against whatever the credential row declares, and
// never itself consults the registry (only
// platformserviceauth.Administrator.Create/Rotate's VALIDATION does, tested
// separately in service_credential_admin_test.go against real registry
// paths) -- so these routes exercise the exact same enforcement path a real
// registered route would, without needing this file to also stand up a
// full feature's dependencies.
func credentialScopeRouter(t *testing.T, pool *pgxpool.Pool, secrets platformserviceauth.VersionedSecrets) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	verifier := platformserviceauth.NewScopeVerifier(platformserviceauth.NewStore(pool), secrets, 5*time.Minute)
	return app.BuildRouter(app.Options{
		Config:        config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:       observability.NewMetrics(),
		TokenSigner:   auth.NewTokenSigner([]byte("unused-service-credential-scope-test-token-secret")),
		ScopeVerifier: verifier,
		NonceStore:    noncestore.New(pool),
		Limiter:       ratelimit.NewMemoryLimiter(),
		InternalRoutes: func(group *gin.RouterGroup) {
			group.GET("/test/resource", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
			group.POST("/test/events", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		},
	})
}

// createTestCredential inserts a credential row directly through the Store
// (bypassing Administrator's registry-path validation deliberately -- this
// file tests RUNTIME enforcement against arbitrary test-chosen route
// patterns; registry validation is a separate, Administrator-level concern
// tested in service_credential_admin_test.go).
func createTestCredential(t *testing.T, pool *pgxpool.Pool, cred platformserviceauth.Credential) platformserviceauth.Credential {
	t.Helper()
	store := platformserviceauth.NewStore(pool)
	if cred.ServiceName == "" {
		cred.ServiceName = scopeTestCaller
	}
	if cred.ActiveFrom.IsZero() {
		cred.ActiveFrom = time.Now().UTC().Add(-time.Minute)
	}
	if cred.CreatedBy == "" {
		cred.CreatedBy = "test"
	}
	if err := store.Create(t.Context(), pool, cred); err != nil {
		t.Fatalf("create test credential: %v", err)
	}
	stored, err := store.GetByID(t.Context(), cred.ID)
	if err != nil {
		t.Fatalf("reload test credential: %v", err)
	}
	return stored
}

func signedScopeTestRequest(t *testing.T, method, path string, body map[string]any, service string, secret []byte) *http.Request {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	if service != "" && len(secret) > 0 {
		if err := sharedserviceauth.SignRequest(req, service, secret, encoded, time.Now()); err != nil {
			t.Fatalf("sign request: %v", err)
		}
	}
	return req
}

func nextCredentialID(t *testing.T) string {
	t.Helper()
	return "svccred_test_" + t.Name() + "_" + time.Now().Format("150405.000000000")
}

// --- Mandatory scenario 1: valid caller/route/method ---

func TestServiceCredentialScope_ValidCallerRouteAndMethod(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/resource"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 2: wrong route ---

func TestServiceCredentialScope_WrongRouteRefused(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"POST"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/events"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	// Signature is valid; the credential simply does not cover this route.
	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 3: wrong method ---

func TestServiceCredentialScope_WrongMethodRefused(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{platformserviceauth.Wildcard},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodPost, "/internal/fleet-partner/v1/test/events",
		map[string]any{"eventType": "anything"}, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 4: wrong event type ---

func TestServiceCredentialScope_WrongEventTypeRefused(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"POST"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/events"},
		AllowedEventTypes: []string{"trip.completed"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodPost, "/internal/fleet-partner/v1/test/events",
		map[string]any{"eventType": "trip.cancelled"}, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}

	// The declared event type must still pass.
	req2 := signedScopeTestRequest(t, http.MethodPost, "/internal/fleet-partner/v1/test/events",
		map[string]any{"eventType": "trip.completed"}, scopeTestCaller, []byte(scopeTestSecret))
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusOK {
		t.Fatalf("declared event type status = %d, want 200, body = %s", resp2.Code, resp2.Body.String())
	}
}

// --- Mandatory scenario 5: expired credential ---

func TestServiceCredentialScope_ExpiredCredentialRefused(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	past := time.Now().UTC().Add(-time.Hour)
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1, ExpiresAt: &past,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/resource"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 6: revoked credential ---

func TestServiceCredentialScope_RevokedCredentialRefused(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	created := createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/resource"},
	})
	store := platformserviceauth.NewStore(pool)
	if err := store.Revoke(t.Context(), pool, created.ID, "test revocation"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil, scopeTestCaller, []byte(scopeTestSecret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 7: nonce replay ---

func TestServiceCredentialScope_NonceReplayRejected(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/resource"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil, scopeTestCaller, []byte(scopeTestSecret))
	first := httptest.NewRecorder()
	router.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200, body = %s", first.Code, first.Body.String())
	}

	// Replay the exact same signed request (same nonce, same signature).
	replay := httptest.NewRequest(http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil)
	replay.Header = req.Header.Clone()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, replay)
	if resp.Code != http.StatusConflict {
		t.Fatalf("replay status = %d, want 409, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 8: body tampering ---

func TestServiceCredentialScope_BodyTamperingRejected(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"POST"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/events"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	req := signedScopeTestRequest(t, http.MethodPost, "/internal/fleet-partner/v1/test/events",
		map[string]any{"eventType": "trip.completed"}, scopeTestCaller, []byte(scopeTestSecret))
	// Tamper with the body AFTER signing -- the signature no longer covers
	// this content.
	tampered := httptest.NewRequest(http.MethodPost, "/internal/fleet-partner/v1/test/events",
		bytes.NewReader([]byte(`{"eventType":"trip.cancelled"}`)))
	tampered.Header = req.Header.Clone()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, tampered)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario 9: organization user denied internal route ---

func TestServiceCredentialScope_OrganizationUserDeniedInternalRoute(t *testing.T) {
	pool := testPool(t)
	secrets := platformserviceauth.VersionedSecrets{scopeTestCaller: {1: []byte(scopeTestSecret)}}
	createTestCredential(t, pool, platformserviceauth.Credential{
		ID: nextCredentialID(t), Version: 1,
		AllowedMethods: []string{"GET"}, AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/test/resource"},
	})
	router := credentialScopeRouter(t, pool, secrets)

	signer := auth.NewTokenSigner([]byte("unused-service-credential-scope-test-token-secret"))
	token, err := signer.Sign(auth.Claims{
		Subject: "corp_user_1", Role: config.CorporateRole, Service: config.ServiceName,
		SessionID: "sess_test_1", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/fleet-partner/v1/test/resource", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code == http.StatusOK {
		t.Fatalf("organization user bearer token must never authorize an internal route, got 200: %s", resp.Body.String())
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no X-Service-Name header present at all), body = %s", resp.Code, resp.Body.String())
	}
}
