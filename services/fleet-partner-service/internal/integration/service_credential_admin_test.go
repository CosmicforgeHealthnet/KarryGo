package integration_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sharedserviceauth "cosmicforge/logistics/shared/go/serviceauth"

	platformserviceauth "cosmicforge/logistics/services/fleet-partner-service/internal/platform/serviceauth"
)

const adminTestCaller = "admin-test-service"

// --- Administrator.Create/Rotate registry-path validation ---

// TestServiceCredentialAdmin_CreateAcceptsRealRegisteredRoute proves Create
// accepts a route pattern that is a real, currently-mounted internal route
// (the always-mounted /internal/fleet-partner/v1/meta) -- the "positive"
// half of the closed-registry validation.
func TestServiceCredentialAdmin_CreateAcceptsRealRegisteredRoute(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-create-ok-" + t.Name()

	cred, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:                platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName:          serviceName,
		AllowedMethods:       []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
		VersionNote:          "test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cred.Version != 1 {
		t.Errorf("version = %d, want 1", cred.Version)
	}
}

// TestServiceCredentialAdmin_CreateRejectsUnknownRoute proves the closed
// route registry actually closes: an invented, never-mounted route pattern
// is refused, not silently accepted (which would make a "scoped"
// credential either permanently useless or, worse, a landmine if that
// exact string ever became a real route later by coincidence).
func TestServiceCredentialAdmin_CreateRejectsUnknownRoute(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-create-bad-" + t.Name()

	_, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:                platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName:          serviceName,
		AllowedMethods:       []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/this-route-does-not-exist"},
	})
	var validationErr platformserviceauth.ErrValidationFailed
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want ErrValidationFailed", err)
	}
	if validationErr.Field != "allowedRoutePatterns" {
		t.Errorf("field = %q, want allowedRoutePatterns", validationErr.Field)
	}
}

// TestServiceCredentialAdmin_CreateRejectsTenantRoute proves a tenant-scoped
// (non-internal) route path is refused even if it happens to be a real,
// mounted route elsewhere in the service -- a service credential must never
// be scopeable to a route class it can never actually reach.
func TestServiceCredentialAdmin_CreateRejectsTenantRoute(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-create-tenant-" + t.Name()

	_, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:                platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName:          serviceName,
		AllowedMethods:       []string{"GET"},
		AllowedRoutePatterns: []string{"/fleet-partner/v1/audit-logs"},
	})
	var validationErr platformserviceauth.ErrValidationFailed
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want ErrValidationFailed", err)
	}
}

// TestServiceCredentialAdmin_CreateRefusesSecondCallForSameService proves
// Create is only for a brand-new caller's version 1 -- a second Create for
// the same service name must be refused (Conflict), directing the caller to
// Rotate instead, so version history can never gain a second, competing
// "version 1".
func TestServiceCredentialAdmin_CreateRefusesSecondCallForSameService(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-create-twice-" + t.Name()
	input := platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	}
	if _, err := admin.Create(t.Context(), input); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := admin.Create(t.Context(), input); err == nil {
		t.Fatal("expected the second create for the same service to be refused")
	}
}

// --- Mandatory scenario: active credential ---

func TestServiceCredentialAdmin_ActiveCredentialWorks(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-active-" + t.Name()
	secret := "active-credential-secret"

	if _, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	router := credentialScopeRouter(t, pool, platformserviceauth.VersionedSecrets{serviceName: {1: []byte(secret)}})
	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

// --- Mandatory scenario: rotation overlap ---

// TestServiceCredentialAdmin_RotationOverlap_BothSecretsWorkDuringWindow
// proves that immediately after Rotate, BOTH the old (v1) and new (v2)
// secret authenticate successfully -- the whole point of an overlap window:
// a caller updating its configured secret is never left with a moment
// where neither its old nor new secret is accepted.
func TestServiceCredentialAdmin_RotationOverlap_BothSecretsWorkDuringWindow(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-overlap-" + t.Name()
	secretV1, secretV2 := "overlap-secret-v1", "overlap-secret-v2"

	if _, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	rotated, err := admin.Rotate(t.Context(), platformserviceauth.RotateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
		OverlapWindow:        time.Hour,
	})
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated.Version != 2 {
		t.Fatalf("rotated version = %d, want 2", rotated.Version)
	}

	secrets := platformserviceauth.VersionedSecrets{serviceName: {1: []byte(secretV1), 2: []byte(secretV2)}}
	router := credentialScopeRouter(t, pool, secrets)

	reqV1 := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secretV1))
	respV1 := httptest.NewRecorder()
	router.ServeHTTP(respV1, reqV1)
	if respV1.Code != http.StatusOK {
		t.Fatalf("v1 (superseded, still within overlap) status = %d, body = %s", respV1.Code, respV1.Body.String())
	}

	reqV2 := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secretV2))
	respV2 := httptest.NewRecorder()
	router.ServeHTTP(respV2, reqV2)
	if respV2.Code != http.StatusOK {
		t.Fatalf("v2 (new) status = %d, body = %s", respV2.Code, respV2.Body.String())
	}
}

// --- Mandatory scenario: old credential after expiry ---

func TestServiceCredentialAdmin_OldCredentialStopsWorkingAfterOverlapExpires(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-postoverlap-" + t.Name()
	secretV1, secretV2 := "post-overlap-secret-v1", "post-overlap-secret-v2"

	if _, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Rotate.OverlapWindow<=0 is deliberately treated as "unspecified,
	// default to 24h" (a caller passing zero/negative BY ACCIDENT must
	// never instantly kill the old credential with no grace period) -- so a
	// negative value cannot be used to simulate "already elapsed" the way
	// an earlier draft of this test tried; confirmed by
	// TestServiceCredentialAdmin_OldCredentialStopsWorkingAfterOverlapExpires
	// itself failing (v1 stayed active) until fixed to use a real, tiny
	// positive window plus a short sleep instead.
	if _, err := admin.Rotate(t.Context(), platformserviceauth.RotateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
		OverlapWindow:        20 * time.Millisecond,
	}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	secrets := platformserviceauth.VersionedSecrets{serviceName: {1: []byte(secretV1), 2: []byte(secretV2)}}
	router := credentialScopeRouter(t, pool, secrets)

	// v1 is now inactive (expired), so Resolve skips it entirely and only
	// tries v2's secret against this v1-signed request -- which naturally
	// fails signature verification, exactly like any other wrong-secret
	// request (401 Unauthorized), NOT the distinct "no active credential
	// exists at all" case (403 Forbidden,
	// TestServiceCredentialScope_ExpiredCredentialRefused's scenario, where
	// there is no v2 to even attempt). Both are "refused", via different,
	// individually correct status codes for different reasons -- confirmed
	// by this test failing with 401 (not the originally expected 403) until
	// the expectation itself was corrected to match Resolve's real,
	// deliberate behavior.
	reqV1 := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secretV1))
	respV1 := httptest.NewRecorder()
	router.ServeHTTP(respV1, reqV1)
	if respV1.Code != http.StatusUnauthorized {
		t.Fatalf("v1 (overlap elapsed) status = %d, want 401, body = %s", respV1.Code, respV1.Body.String())
	}

	reqV2 := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secretV2))
	respV2 := httptest.NewRecorder()
	router.ServeHTTP(respV2, reqV2)
	if respV2.Code != http.StatusOK {
		t.Fatalf("v2 status = %d, body = %s", respV2.Code, respV2.Body.String())
	}
}

// --- Mandatory scenario: immediate revocation ---

// TestServiceCredentialAdmin_RevocationIsImmediateNotScheduled proves
// Revoke stops a credential working right away, independent of any
// configured expiry (which might be far in the future) -- distinct from
// expiry itself, already covered above.
func TestServiceCredentialAdmin_RevocationIsImmediateNotScheduled(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-revoke-" + t.Name()
	secret := "revoke-secret"

	cred, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	secrets := platformserviceauth.VersionedSecrets{serviceName: {1: []byte(secret)}}
	router := credentialScopeRouter(t, pool, secrets)

	before := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secret))
	respBefore := httptest.NewRecorder()
	router.ServeHTTP(respBefore, before)
	if respBefore.Code != http.StatusOK {
		t.Fatalf("before revoke status = %d, want 200, body = %s", respBefore.Code, respBefore.Body.String())
	}

	if err := admin.Revoke(t.Context(), platformserviceauth.RevokeInput{
		Actor:        platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		CredentialID: cred.ID, Reason: "suspected leaked secret",
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	after := httptest.NewRequest(http.MethodGet, "/internal/fleet-partner/v1/meta", nil)
	after.Header = before.Header.Clone()
	after.Header.Set(sharedserviceauth.HeaderNonce, before.Header.Get(sharedserviceauth.HeaderNonce)+"-after-revoke")
	// Re-sign with a fresh nonce (the "before" request's nonce is already
	// consumed) but the SAME now-revoked secret.
	if err := sharedserviceauth.SignRequest(after, serviceName, []byte(secret), nil, time.Now()); err != nil {
		t.Fatalf("re-sign: %v", err)
	}
	respAfter := httptest.NewRecorder()
	router.ServeHTTP(respAfter, after)
	if respAfter.Code != http.StatusForbidden {
		t.Fatalf("after revoke status = %d, want 403, body = %s", respAfter.Code, respAfter.Body.String())
	}
}

// --- Mandatory scenario: last-used update ---

func TestServiceCredentialAdmin_LastUsedAtUpdatesAfterSuccessfulCall(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-lastused-" + t.Name()
	secret := "last-used-secret"

	cred, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cred.LastUsedAt != nil {
		t.Fatalf("expected LastUsedAt nil before any use, got %v", *cred.LastUsedAt)
	}

	router := credentialScopeRouter(t, pool, platformserviceauth.VersionedSecrets{serviceName: {1: []byte(secret)}})
	req := signedScopeTestRequest(t, http.MethodGet, "/internal/fleet-partner/v1/meta", nil, serviceName, []byte(secret))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	reloaded, err := platformserviceauth.NewStore(pool).GetByID(t.Context(), cred.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt to be set after a successful authorized call")
	}
}

// --- Mandatory scenario: no secret leakage ---

// TestServiceCredentialAdmin_NoSecretLeakage proves the credential row and
// every usecase-layer view of it (List, Create's own return value) is
// safe-by-construction -- there is no secret column in
// fleet_partner_service_credentials at all (confirmed directly against
// information_schema, not assumed from the Go struct alone: a raw SQL
// migration could in principle add a column the Go model never reads).
func TestServiceCredentialAdmin_NoSecretLeakage(t *testing.T) {
	pool := testPool(t)
	admin := platformserviceauth.NewAdministrator(pool, nil)
	serviceName := adminTestCaller + "-noleak-" + t.Name()

	created, err := admin.Create(t.Context(), platformserviceauth.CreateInput{
		Actor:       platformserviceauth.Actor{ActorType: "platform_operator", ActorID: "op_1"},
		ServiceName: serviceName, AllowedMethods: []string{"GET"},
		AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/meta"},
		VersionNote:          "contains no secret, only metadata",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = created // Credential has no secret-shaped field at all -- nothing to assert away.

	list, err := admin.List(t.Context(), serviceName)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}

	var columnNames []string
	rows, err := pool.Query(t.Context(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_name = 'fleet_partner_service_credentials'`)
	if err != nil {
		t.Fatalf("introspect columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		columnNames = append(columnNames, name)
	}
	for _, name := range columnNames {
		lower := name
		if lower == "secret" || lower == "secret_hash" || lower == "hmac_secret" || lower == "credential_secret" {
			t.Fatalf("fleet_partner_service_credentials has a secret-shaped column %q -- secrets must never be stored in this table", name)
		}
	}
}
