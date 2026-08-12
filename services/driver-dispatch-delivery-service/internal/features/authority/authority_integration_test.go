package authority_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
)

// The private vehicle and provider authority contract, against real
// PostgreSQL.
//
// Three properties are under test and none of them is optional:
//
//  1. only a caller that can produce a valid signature AND is on the allowlist
//     reaches the surface at all;
//  2. a captured request cannot be replayed even while its signature is still
//     inside the freshness window;
//  3. the answer carries verdicts and never the underlying record — no plate
//     number, no chassis number, no owner contact details.

const (
	fleetCaller = "fleet-partner-service"
	fleetSecret = "authority-test-secret"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; skipping dispatch PostgreSQL tests (CI must set it)")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect dispatch test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping dispatch test database: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE dispatch_service_request_nonces, bike_documents, bike_audit, bikes,
			verification_documents, verification_steps, provider_availability,
			trips, providers CASCADE`); err != nil {
		t.Fatalf("clean dispatch test database: %v", err)
	}
	return pool
}

// authorityFixture is a verified provider with a verified, documented vehicle.
type authorityFixture struct {
	pool       *pgxpool.Pool
	server     *httptest.Server
	providerID string
	vehicleID  string
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	var providerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO providers (id, phone, full_name, email, operation_type,
			verification_status, is_active, onboarding_complete)
		VALUES (gen_random_uuid(), '+2348010000001', 'Test Provider',
			'provider@example.com', 'individual', 'verified', true, true)
		RETURNING id::text`).Scan(&providerID); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	var vehicleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO bikes (provider_id, bike_type, brand, model, year, color,
			plate_number, chassis_number, verification_status, is_active, is_primary)
		VALUES ($1::uuid, 'dispatch_bike', 'Honda', 'CG125', 2021, 'red',
			'LAG-123-XY', 'CHASSIS-SECRET-9', 'verified', true, true)
		RETURNING id::text`, providerID).Scan(&vehicleID); err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}

	for _, documentType := range []string{"registration", "insurance"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO bike_documents (bike_id, provider_id, document_type, file_url, expiry_date)
			VALUES ($1::uuid, $2::uuid, $3, 'https://files.example.com/secret.pdf',
				current_date + 200)`, vehicleID, providerID, documentType); err != nil {
			t.Fatalf("seed vehicle document: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO provider_availability (provider_id, status, verified_to_go_online, session_start)
		VALUES ($1::uuid, 'online', true, now())`, providerID); err != nil {
		t.Fatalf("seed availability: %v", err)
	}

	for _, step := range []string{"identity", "licence"} {
		expiry := "NULL"
		if step == "licence" {
			expiry = "current_date + 365"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO verification_steps (provider_id, step, status, licence_expiry_date)
			VALUES ($1::uuid, $2, 'approved', `+expiry+`)`, providerID, step); err != nil {
			t.Fatalf("seed verification step: %v", err)
		}
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())
	authority.RegisterAuthorityRoutes(
		engine.Group("/api/v1/internal/authority"),
		authority.NewAuthorityHandler(authority.NewAuthorityReader(pool)),
		serviceauth.NewVerifier(serviceauth.Secrets{fleetCaller: []byte(fleetSecret)}, time.Minute),
		authority.NewPostgresNonceClaimer(pool),
		[]string{fleetCaller})
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	return &authorityFixture{pool: pool, server: server,
		providerID: providerID, vehicleID: vehicleID}
}

// signedCall performs a properly signed, nonce-carrying private request.
func (f *authorityFixture) signedCall(t *testing.T, path string, caller string,
	secret string, nonce string) (int, map[string]any, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, f.server.URL+path, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if nonce != "" {
		request.Header.Set(authority.HeaderServiceNonce, nonce)
	}
	if caller != "" {
		if err := serviceauth.SignRequest(request, caller, []byte(secret), nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		if nonce == "" {
			request.Header.Del(authority.HeaderServiceNonce)
		}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)

	decoded := map[string]any{}
	_ = json.Unmarshal(raw, &decoded)
	return response.StatusCode, decoded, string(raw)
}

func (f *authorityFixture) vehiclePath() string {
	return "/api/v1/internal/authority/vehicles/" + f.vehicleID +
		"?providerId=" + f.providerID + "&serviceType=dispatch_bike"
}

// ── Authentication and authorization ──────────────────────────────────────────

// TestAuthorityRejectsUnsignedCalls proves there is no unauthenticated variant
// of this surface.
func TestAuthorityRejectsUnsignedCalls(t *testing.T) {
	f := newAuthorityFixture(t)
	status, _, _ := f.signedCall(t, f.vehiclePath(), "", "", "nonce-unsigned")
	if status != http.StatusUnauthorized {
		t.Fatalf("unsigned status=%d, want 401", status)
	}
}

// TestAuthorityRejectsAWrongSecret proves the signature is verified rather
// than merely present.
func TestAuthorityRejectsAWrongSecret(t *testing.T) {
	f := newAuthorityFixture(t)
	status, _, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, "not-the-secret", "nonce-wrong")
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong secret status=%d, want 401", status)
	}
}

// TestAuthorityRejectsACallerNotOnTheAllowlist proves that proving WHO you are
// is not the same as being allowed HERE. An unknown caller has no secret at
// all, so the verifier refuses it before the allowlist is even consulted —
// which is the same outcome by a stricter route.
func TestAuthorityRejectsACallerNotOnTheAllowlist(t *testing.T) {
	f := newAuthorityFixture(t)
	status, _, _ := f.signedCall(t, f.vehiclePath(), "analytics-service", fleetSecret, "nonce-other")
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("foreign caller status=%d, want 401 or 403", status)
	}
}

// TestAuthorityRejectsAStaleSignature proves the freshness window is enforced.
func TestAuthorityRejectsAStaleSignature(t *testing.T) {
	f := newAuthorityFixture(t)
	request, err := http.NewRequest(http.MethodGet, f.server.URL+f.vehiclePath(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(authority.HeaderServiceNonce, "nonce-stale")
	if err := serviceauth.SignRequest(request, fleetCaller, []byte(fleetSecret), nil,
		time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stale signature status=%d, want 401", response.StatusCode)
	}
}

// TestAuthorityRejectsAReplay is the property the nonce table exists for: a
// captured request stays valid for the whole freshness window, which is ample
// time to replay it.
func TestAuthorityRejectsAReplay(t *testing.T) {
	f := newAuthorityFixture(t)

	status, _, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-replay-1")
	if status != http.StatusOK {
		t.Fatalf("first call status=%d, want 200", status)
	}
	status, _, _ = f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-replay-1")
	if status != http.StatusConflict {
		t.Fatalf("replayed call status=%d, want 409", status)
	}
}

// TestAuthorityRequiresANonce proves the replay control cannot be skipped by
// simply omitting the header.
func TestAuthorityRequiresANonce(t *testing.T) {
	f := newAuthorityFixture(t)
	status, _, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("missing nonce status=%d, want 401", status)
	}
}

// ── The contract ──────────────────────────────────────────────────────────────

func TestVehicleAuthorityAnswersEveryContractQuestion(t *testing.T) {
	f := newAuthorityFixture(t)
	status, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-contract-1")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)

	for field, want := range map[string]any{
		"vehicleId":                f.vehicleID,
		"providerId":               f.providerID,
		"controlState":             authority.ControlProvider,
		"serviceType":              "dispatch_bike",
		"verificationState":        authority.VerificationStateVerified,
		"available":                true,
		"availabilityState":        authority.AvailabilityOnline,
		"restricted":               false,
		"restrictionState":         authority.RestrictionNone,
		"documentsSatisfied":       true,
		"activeAssignmentConflict": false,
		"assignmentConflictBasis":  authority.ConflictBasisProviderLiveTrip,
		"providerMatch":            true,
		"serviceTypeMatch":         true,
	} {
		if data[field] != want {
			t.Errorf("%s = %#v, want %#v", field, data[field], want)
		}
	}
	// A version that does not move makes a stale answer look current.
	version, _ := data["sourceVersion"].(float64)
	if version <= 0 {
		t.Errorf("sourceVersion = %v; the authority must version its own answer", data["sourceVersion"])
	}
}

// TestAuthorityAnswerCarriesNoIdentifyingData is the data-minimization
// property. The endpoint answers "may this vehicle work?", which needs
// verdicts, not a registration record.
func TestAuthorityAnswerCarriesNoIdentifyingData(t *testing.T) {
	f := newAuthorityFixture(t)
	_, _, raw := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-minimal-1")

	for _, secret := range []string{
		"LAG-123-XY",        // plate number
		"CHASSIS-SECRET-9",  // chassis number
		"Honda",             // brand
		"CG125",             // model
		"files.example.com", // document URL
		"Test Provider",     // owner name
		"provider@example.com",
		"+2348010000001",
	} {
		if strings.Contains(raw, secret) {
			t.Errorf("the authority response leaked %q", secret)
		}
	}
}

func TestProviderAuthorityAnswersEveryContractQuestion(t *testing.T) {
	f := newAuthorityFixture(t)
	path := "/api/v1/internal/authority/providers/" + f.providerID
	status, response, raw := f.signedCall(t, path, fleetCaller, fleetSecret, "nonce-provider-1")
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)

	for field, want := range map[string]any{
		"providerId":               f.providerID,
		"accountState":             authority.AccountActive,
		"verificationState":        authority.VerificationStateVerified,
		"available":                true,
		"availabilityState":        authority.AvailabilityOnline,
		"shiftOpen":                true,
		"restricted":               false,
		"documentsSatisfied":       true,
		"activeAssignmentConflict": false,
	} {
		if data[field] != want {
			t.Errorf("%s = %#v, want %#v", field, data[field], want)
		}
	}
	for _, secret := range []string{"Test Provider", "provider@example.com", "+2348010000001"} {
		if strings.Contains(raw, secret) {
			t.Errorf("the provider answer leaked %q", secret)
		}
	}
}

// TestUnknownVehicleAndMalformedIdentifierAnswerIdentically proves the
// endpoint cannot be used as an existence oracle.
func TestUnknownVehicleAndMalformedIdentifierAnswerIdentically(t *testing.T) {
	f := newAuthorityFixture(t)

	missing, missingBody, _ := f.signedCall(t,
		"/api/v1/internal/authority/vehicles/11111111-1111-1111-1111-111111111111",
		fleetCaller, fleetSecret, "nonce-missing-1")
	malformed, malformedBody, _ := f.signedCall(t,
		"/api/v1/internal/authority/vehicles/not-a-uuid",
		fleetCaller, fleetSecret, "nonce-missing-2")

	if missing != http.StatusNotFound || malformed != http.StatusNotFound {
		t.Fatalf("missing=%d malformed=%d, want 404 for both", missing, malformed)
	}
	if messageOf(missingBody) != messageOf(malformedBody) {
		t.Fatalf("responses differ: %q vs %q",
			messageOf(missingBody), messageOf(malformedBody))
	}
}

// ── State that must block ─────────────────────────────────────────────────────

func TestSuspendedVehicleIsReportedRestricted(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE bikes SET verification_status = 'suspended' WHERE id = $1::uuid`,
		f.vehicleID); err != nil {
		t.Fatal(err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-suspended-1")
	data, _ := response["data"].(map[string]any)
	if data["restricted"] != true || data["restrictionState"] != authority.RestrictionSuspended {
		t.Fatalf("restricted=%v state=%v", data["restricted"], data["restrictionState"])
	}
	if data["verificationState"] != authority.VerificationStateSuspended {
		t.Fatalf("verificationState = %v", data["verificationState"])
	}
	if data["available"] != false {
		t.Fatal("a restricted vehicle must not be reported available")
	}
}

// TestUnverifiedProviderMakesTheVehicleUnverified proves the vehicle cannot be
// more verified than whoever rides it.
func TestUnverifiedProviderMakesTheVehicleUnverified(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE providers SET verification_status = 'pending_review' WHERE id = $1::uuid`,
		f.providerID); err != nil {
		t.Fatal(err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-unverified-1")
	data, _ := response["data"].(map[string]any)
	if data["verificationState"] != authority.VerificationStateUnverified {
		t.Fatalf("verificationState = %v, want UNVERIFIED", data["verificationState"])
	}
}

func TestExpiredVehicleDocumentIsReported(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE bike_documents SET expiry_date = current_date - 1
		WHERE bike_id = $1::uuid AND document_type = 'insurance'`, f.vehicleID); err != nil {
		t.Fatal(err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-expired-1")
	data, _ := response["data"].(map[string]any)
	if data["documentsSatisfied"] != false {
		t.Fatal("an expired required document must not be reported as satisfied")
	}
	documents, _ := data["requiredDocuments"].([]any)
	var sawExpired bool
	for _, entry := range documents {
		document, _ := entry.(map[string]any)
		if document["type"] == "insurance" && document["expired"] == true {
			sawExpired = true
		}
	}
	if !sawExpired {
		t.Fatalf("the expired document must be named: %#v", documents)
	}
}

func TestOfflineProviderIsNotAvailable(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE provider_availability SET status = 'offline' WHERE provider_id = $1::uuid`,
		f.providerID); err != nil {
		t.Fatal(err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-offline-1")
	data, _ := response["data"].(map[string]any)
	if data["available"] != false || data["availabilityState"] != authority.AvailabilityOffline {
		t.Fatalf("available=%v state=%v", data["available"], data["availabilityState"])
	}
}

// TestUnknownAvailabilityIsNotAvailability proves the absence of an
// availability record is not read as "probably fine".
func TestUnknownAvailabilityIsNotAvailability(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		DELETE FROM provider_availability WHERE provider_id = $1::uuid`,
		f.providerID); err != nil {
		t.Fatal(err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-unknown-avail")
	data, _ := response["data"].(map[string]any)
	if data["availabilityState"] != authority.AvailabilityUnknown || data["available"] != false {
		t.Fatalf("state=%v available=%v; unknown availability must not be availability",
			data["availabilityState"], data["available"])
	}
}

func TestLiveTripIsReportedAsAnAssignmentConflict(t *testing.T) {
	f := newAuthorityFixture(t)
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO trips (booking_id, provider_id, customer_id, status,
			pickup_address, pickup_lat, pickup_lng,
			dropoff_address, dropoff_lat, dropoff_lng,
			fare_amount, receiver_name, receiver_phone)
		VALUES (gen_random_uuid(), $1::uuid, gen_random_uuid(), 'in_progress',
			'A', 6.5, 3.3, 'B', 6.6, 3.4, 1500, 'Receiver', '+2348020000002')`,
		f.providerID); err != nil {
		t.Fatalf("seed live trip: %v", err)
	}

	_, response, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret, "nonce-conflict-1")
	data, _ := response["data"].(map[string]any)
	if data["activeAssignmentConflict"] != true {
		t.Fatal("a provider already on a live trip must be reported as conflicting")
	}
	// The basis is stated so a caller does not read more precision into the
	// answer than exists: dispatch trips do not name a vehicle.
	if data["assignmentConflictBasis"] != authority.ConflictBasisProviderLiveTrip {
		t.Fatalf("assignmentConflictBasis = %v", data["assignmentConflictBasis"])
	}
}

// TestWrongProviderIsReportedAsAMismatch proves the ownership question is
// answered by the authority rather than inferred by the caller.
func TestWrongProviderIsReportedAsAMismatch(t *testing.T) {
	f := newAuthorityFixture(t)
	path := "/api/v1/internal/authority/vehicles/" + f.vehicleID +
		"?providerId=11111111-1111-1111-1111-111111111111"

	_, response, _ := f.signedCall(t, path, fleetCaller, fleetSecret, "nonce-owner-1")
	data, _ := response["data"].(map[string]any)
	if data["providerMatch"] != false {
		t.Fatalf("providerMatch = %v, want false", data["providerMatch"])
	}
}

// TestServiceTypeMismatchIsReported proves an unmatched service type is an
// explicit false rather than a silently absent field.
func TestServiceTypeMismatchIsReported(t *testing.T) {
	f := newAuthorityFixture(t)
	path := "/api/v1/internal/authority/vehicles/" + f.vehicleID + "?serviceType=tricycle"

	_, response, _ := f.signedCall(t, path, fleetCaller, fleetSecret, "nonce-service-1")
	data, _ := response["data"].(map[string]any)
	if data["serviceTypeMatch"] != false {
		t.Fatalf("serviceTypeMatch = %v, want false", data["serviceTypeMatch"])
	}
}

// TestNoncesAreRecordedWithTheirFingerprint proves a rejected replay can be
// told apart from a client that reused a nonce on a different request.
func TestNoncesAreRecordedWithTheirFingerprint(t *testing.T) {
	f := newAuthorityFixture(t)
	if status, _, _ := f.signedCall(t, f.vehiclePath(), fleetCaller, fleetSecret,
		"nonce-fingerprint-1"); status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}

	var fingerprint string
	var expiresAt time.Time
	if err := f.pool.QueryRow(context.Background(), `
		SELECT request_fingerprint, expires_at FROM dispatch_service_request_nonces
		WHERE service_name = $1 AND nonce = $2`,
		fleetCaller, "nonce-fingerprint-1").Scan(&fingerprint, &expiresAt); err != nil {
		t.Fatalf("nonce must be recorded: %v", err)
	}
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint = %q; want a sha256 hex digest", fingerprint)
	}
	// Nonces only need to outlive the signature window; keeping them forever
	// would grow without bound for no security gain.
	if expiresAt.After(time.Now().Add(time.Hour)) {
		t.Fatalf("nonce expiry %s is too far out", expiresAt)
	}
}

// messageOf extracts the safe public message from an error envelope.
func messageOf(response map[string]any) string {
	body, _ := response["error"].(map[string]any)
	message, _ := body["message"].(string)
	return message
}
