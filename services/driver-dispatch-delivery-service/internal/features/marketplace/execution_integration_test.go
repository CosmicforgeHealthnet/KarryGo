package marketplace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// stubVerifier stands in for fleet-partner-service in dispatch-only tests.
//
// It is a TEST double only: production wiring uses the real HTTP verifier, and
// the default when unconfigured is UnconfiguredVerifier, which fails closed.
type stubVerifier struct {
	acceptance *marketplace.ProviderAcceptance
	err        error
	calls      int
}

func (s *stubVerifier) Configured() bool { return true }

func (s *stubVerifier) VerifyProviderAcceptance(_ context.Context, _ marketplace.CallMeta,
	assignmentID string) (*marketplace.ProviderAcceptance, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.acceptance, nil
}

func executionFlags() marketplace.Flags {
	return marketplace.Flags{
		InternalAPIEnabled:         true,
		ReservationEnabled:         true,
		ExecutionEnabled:           true,
		OutboxEnabled:              true,
		ProviderEligibilityEnabled: true,
		VehicleEligibilityEnabled:  true,
	}
}

// newExecutionServer builds a server with an injectable acceptance verifier.
func newExecutionServer(t *testing.T, pool *pgxpool.Pool, flags marketplace.Flags,
	verifier marketplace.AcceptanceVerifier) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())

	repo := marketplace.NewRepository(pool)
	service := marketplace.NewService(repo, flags, verifier)
	handler := marketplace.NewHandler(service, flags)
	v := serviceauth.NewVerifier(serviceauth.Secrets{fleetService: []byte(fleetSecret)}, time.Minute)
	marketplace.RegisterInternalRoutes(
		engine.Group("/api/v1/internal/marketplace"), handler, v, []string{fleetService})

	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

// seedProvider inserts an eligible provider.
func seedProvider(t *testing.T, pool *pgxpool.Pool, phone string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO providers (id, phone, verification_status, is_active, onboarding_complete, operation_type)
		VALUES (gen_random_uuid(), $1, 'verified', true, true, 'fleet') RETURNING id`, phone).Scan(&id)
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	return id
}

// reserveFor claims a request and returns the reservation id.
func reserveFor(t *testing.T, server *httptest.Server, requestID string, org string, key string) string {
	t.Helper()
	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": org,
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, key, fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("reserve for %s = %d (%v)", org, status, body)
	}
	id, _ := dataOf(t, body)["reservationId"].(string)
	return id
}

func acceptanceFor(assignmentID, org, providerID, membership, responseID, requestID string) *marketplace.ProviderAcceptance {
	now := time.Now().UTC()
	return &marketplace.ProviderAcceptance{
		AssignmentID: assignmentID, OrganizationID: org, ProviderID: providerID,
		OrganizationMembershipID: membership, ProviderResponseID: responseID,
		ResponseStatus: "ACCEPTED", AcceptedAt: &now,
		AssignmentStatus: "ACCEPTED", AssignmentVersion: 3, SourceRequestID: requestID,
	}
}

func executionBodyFor(requestID, reservationID, org, providerID, membership, assignmentID, responseID string) map[string]any {
	return map[string]any{
		"requestId": requestID, "reservationId": reservationID,
		"organizationId": org, "settlementOrganizationId": org,
		"executingProviderId": providerID, "organizationMembershipId": membership,
		"fleetAssignmentId": assignmentID, "providerResponseId": responseID,
		"acceptanceSource": "ORGANIZATION_DISPATCHER", "executionSource": "MARKETPLACE_REQUEST",
	}
}

func TestExecutionRequiresVerifiedProviderAcceptance(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000001")

	// The verifier reports the provider has NOT accepted, even though the
	// caller supplies a providerResponseId. A supplied id is not consent.
	verifier := &stubVerifier{err: marketplace.ErrAcceptanceNotConfirmed}
	server := newExecutionServer(t, pool, executionFlags(), verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-exec-res-1")

	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_1", "resp_1"),
		"idem-exec-1", fleetService, fleetSecret)
	if status != http.StatusConflict || errorCodeOf(body) != "provider_acceptance_not_confirmed" {
		t.Fatalf("unaccepted execution = %d %q, want 409 provider_acceptance_not_confirmed",
			status, errorCodeOf(body))
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}

	// No trip may exist.
	var trips int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 0 {
		t.Fatalf("trips created without acceptance = %d, want 0", trips)
	}
}

func TestExecutionFailsClosedWhenVerifierUnconfigured(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000002")

	// nil verifier => UnconfiguredVerifier => must refuse, never assume.
	server := newExecutionServer(t, pool, executionFlags(), nil)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-exec-res-2")

	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_2", "resp_2"),
		"idem-exec-2", fleetService, fleetSecret)
	if status != http.StatusServiceUnavailable || errorCodeOf(body) != "fleet_service_unavailable" {
		t.Fatalf("unconfigured verifier = %d %q, want 503 fleet_service_unavailable",
			status, errorCodeOf(body))
	}
	var trips int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 0 {
		t.Fatalf("trips created with unconfigured verifier = %d, want 0", trips)
	}
}

func TestExecutionRejectsAcceptanceMismatch(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000003")
	otherProvider := seedProvider(t, pool, "+2348010000004")

	cases := []struct {
		name       string
		acceptance *marketplace.ProviderAcceptance
	}{
		{"different provider", acceptanceFor("asg_3", "org_A", otherProvider, "mem_1", "resp_3", "")},
		{"different response id", acceptanceFor("asg_3", "org_A", providerID, "mem_1", "OTHER", "")},
		{"different membership", acceptanceFor("asg_3", "org_A", providerID, "OTHER", "resp_3", "")},
		{"different organization", acceptanceFor("asg_3", "org_ZZZ", providerID, "mem_1", "resp_3", "")},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := newExecutionServer(t, pool, executionFlags(),
				&stubVerifier{acceptance: testCase.acceptance})
			reservationID := reserveFor(t, server, requestID, "org_A", "idem-res-"+testCase.name)

			status, body := signedCall(t, server, http.MethodPost,
				"/api/v1/internal/marketplace/executions",
				executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_3", "resp_3"),
				"idem-exec-"+testCase.name, fleetService, fleetSecret)
			if status != http.StatusConflict {
				t.Fatalf("%s = %d %q, want 409", testCase.name, status, errorCodeOf(body))
			}
			var trips int
			_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
			if trips != 0 {
				t.Fatalf("%s created a trip", testCase.name)
			}
			// Release so the next sub-case can reserve the same request.
			_, _ = signedCall(t, server, http.MethodPost,
				"/api/v1/internal/marketplace/reservations/"+reservationID+"/release",
				map[string]any{"reasonCode": "PLATFORM_RECOVERY"},
				"idem-rel-"+testCase.name, fleetService, fleetSecret)
		})
	}
}

func TestExecutionCreatesExactlyOneTripAndReplays(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000005")

	verifier := &stubVerifier{
		acceptance: acceptanceFor("asg_ok", "org_A", providerID, "mem_1", "resp_ok", requestID),
	}
	server := newExecutionServer(t, pool, executionFlags(), verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-ok-res")

	body := executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_ok", "resp_ok")
	status, created := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions", body, "idem-ok", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("execution status = %d (%v)", status, created)
	}
	tripID, _ := dataOf(t, created)["tripId"].(string)
	if tripID == "" {
		t.Fatal("execution returned no trip id")
	}

	// The response must carry no commercial data.
	encoded, _ := json.Marshal(created)
	for _, forbidden := range []string{"fare_amount", "fareAmount", "customerFare",
		"customerPrice", "organizationSettlement", "platformCommission", "margin", "profit"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("execution response leaked %q: %s", forbidden, encoded)
		}
	}

	// Replay with the same key returns the same trip: this is the lost-response
	// case, and it must not create a second canonical trip.
	status, replay := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions", body, "idem-ok", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("replay status = %d (%v)", status, replay)
	}
	if dataOf(t, replay)["tripId"] != tripID {
		t.Fatal("replay produced a different trip")
	}

	var trips int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 1 {
		t.Fatalf("trip rows = %d, want exactly 1", trips)
	}

	// A NEW idempotency key for the same assignment must still not duplicate:
	// the unique index on fleet_assignment_id is the backstop.
	status, second := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions", body, "idem-ok-different-key",
		fleetService, fleetSecret)
	if status != http.StatusCreated && status != http.StatusConflict {
		t.Fatalf("new key for same assignment = %d (%v)", status, second)
	}
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 1 {
		t.Fatalf("trip rows after new key = %d, want exactly 1", trips)
	}

	// Organization metadata and settlement separation are persisted.
	var mode, org, settlement, assignment, response string
	err := pool.QueryRow(context.Background(), `
		SELECT execution_mode, organization_id, settlement_organization_id,
		       fleet_assignment_id, provider_response_id
		FROM trips WHERE id = $1`, tripID).
		Scan(&mode, &org, &settlement, &assignment, &response)
	if err != nil {
		t.Fatalf("load trip: %v", err)
	}
	if mode != "ORGANIZATION_MANAGED" || org != "org_A" || settlement != "org_A" ||
		assignment != "asg_ok" || response != "resp_ok" {
		t.Fatalf("trip metadata wrong: mode=%s org=%s settlement=%s asg=%s resp=%s",
			mode, org, settlement, assignment, response)
	}

	// Read-back by assignment and by idempotency key both resolve.
	status, byAssignment := signedCall(t, server, http.MethodGet,
		"/api/v1/internal/marketplace/executions/by-assignment/asg_ok", nil, "",
		fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, byAssignment)["tripId"] != tripID {
		t.Fatalf("read-back by assignment = %d (%v)", status, byAssignment)
	}
	status, byKey := signedCall(t, server, http.MethodGet,
		"/api/v1/internal/marketplace/executions/by-idempotency/idem-ok", nil, "",
		fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, byKey)["tripId"] != tripID {
		t.Fatalf("read-back by key = %d (%v)", status, byKey)
	}

	// trip.created event was written in the same transaction, once. Analytics
	// records the assignment exactly once from the trip row itself
	// (migration 000076), so trip.created.v1 is not copied to analytics.
	var eventCount, analyticsCopies, assigned int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE event_type = 'trip.created.v1' AND aggregate_type NOT LIKE 'analytics:%'),
			count(*) FILTER (WHERE event_type = 'trip.created.v1' AND aggregate_type LIKE 'analytics:%'),
			count(*) FILTER (WHERE event_type = 'dispatch.booking.assigned.v1' AND target_service = 'analytics-service')
		FROM dispatch_marketplace_outbox`).
		Scan(&eventCount, &analyticsCopies, &assigned)
	if eventCount != 1 {
		t.Fatalf("trip.created events = %d, want exactly 1", eventCount)
	}
	if analyticsCopies != 0 || assigned != 1 {
		t.Fatalf("analytics assignment facts: trip.created copies = %d, assigned = %d, want 0 and 1", analyticsCopies, assigned)
	}
}

func TestExecutionRejectsSettlementOrganizationMismatch(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000006")
	verifier := &stubVerifier{
		acceptance: acceptanceFor("asg_s", "org_A", providerID, "mem_1", "resp_s", requestID),
	}
	server := newExecutionServer(t, pool, executionFlags(), verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-settle-res")

	body := executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_s", "resp_s")
	// Bill a different organization than the one that reserved.
	body["settlementOrganizationId"] = "org_SOMEONE_ELSE"

	status, resp := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions", body, "idem-settle", fleetService, fleetSecret)
	if status != http.StatusConflict {
		t.Fatalf("settlement mismatch = %d %q, want 409", status, errorCodeOf(resp))
	}
	var trips int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 0 {
		t.Fatal("a settlement mismatch created a trip")
	}
}

func TestProviderEligibilityUsesAuthoritativeDispatchState(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000007")
	server := newExecutionServer(t, pool, executionFlags(), &stubVerifier{})
	reserveFor(t, server, requestID, "org_A", "idem-elig-res")

	path := "/api/v1/internal/marketplace/eligibility/providers/validate"
	body := map[string]any{"requestId": requestID, "organizationId": "org_A", "providerId": providerID}

	status, ok := signedCall(t, server, http.MethodPost, path, body, "", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, ok)["eligible"] != true {
		t.Fatalf("eligible provider = %d (%v)", status, ok)
	}

	// Suspension in dispatch overrides organization sponsorship.
	if _, err := pool.Exec(context.Background(),
		`UPDATE providers SET is_active = false WHERE id = $1`, providerID); err != nil {
		t.Fatalf("suspend provider: %v", err)
	}
	status, suspended := signedCall(t, server, http.MethodPost, path, body, "", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, suspended)["eligible"] != false {
		t.Fatalf("suspended provider = %d (%v)", status, suspended)
	}

	// Another organization has no standing on this reservation.
	status, wrongOrg := signedCall(t, server, http.MethodPost, path,
		map[string]any{"requestId": requestID, "organizationId": "org_B", "providerId": providerID},
		"", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, wrongOrg)["eligible"] != false {
		t.Fatalf("cross-organization eligibility = %d (%v)", status, wrongOrg)
	}
}

func TestVehicleEligibilityNotRequiredIsExplicit(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000008")
	server := newExecutionServer(t, pool, executionFlags(), &stubVerifier{})

	path := "/api/v1/internal/marketplace/eligibility/vehicles/validate"
	// No vehicleId: the honest answer is NOT_REQUIRED, not a fabricated vehicle.
	status, body := signedCall(t, server, http.MethodPost, path,
		map[string]any{"requestId": requestID, "organizationId": "org_A", "providerId": providerID},
		"", fleetService, fleetSecret)
	if status != http.StatusOK {
		t.Fatalf("vehicle not required = %d (%v)", status, body)
	}
	data := dataOf(t, body)
	if data["eligible"] != true || data["status"] != "NOT_REQUIRED" {
		t.Fatalf("expected NOT_REQUIRED, got %v", data)
	}
	if data["vehicleId"] != nil {
		t.Fatalf("a vehicle reference was fabricated: %v", data["vehicleId"])
	}

	// An unknown vehicle is ineligible, not invented.
	status, missing := signedCall(t, server, http.MethodPost, path,
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"providerId": providerID, "vehicleId": "00000000-0000-0000-0000-000000000000"},
		"", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, missing)["eligible"] != false {
		t.Fatalf("unknown vehicle = %d (%v)", status, missing)
	}
}

func TestExecutionDisabledFailsClosed(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000009")

	flags := executionFlags()
	flags.ExecutionEnabled = false
	verifier := &stubVerifier{
		acceptance: acceptanceFor("asg_off", "org_A", providerID, "mem_1", "resp_off", requestID),
	}
	server := newExecutionServer(t, pool, flags, verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-off-res")

	status, _ := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_off", "resp_off"),
		"idem-off-exec", fleetService, fleetSecret)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("disabled execution = %d, want 503", status)
	}
	// The verifier must not even be consulted when the feature is off.
	if verifier.calls != 0 {
		t.Fatalf("verifier called %d times while disabled", verifier.calls)
	}
}

// TestUnconfiguredVerifierAlwaysRefuses guards the production default.
func TestUnconfiguredVerifierAlwaysRefuses(t *testing.T) {
	var verifier marketplace.AcceptanceVerifier = marketplace.UnconfiguredVerifier{}
	if verifier.Configured() {
		t.Fatal("the unconfigured verifier must never report configured")
	}
	acceptance, err := verifier.VerifyProviderAcceptance(context.Background(),
		marketplace.CallMeta{}, "any-assignment")
	if acceptance != nil {
		t.Fatal("the unconfigured verifier must never return an acceptance")
	}
	if !errors.Is(err, marketplace.ErrFleetUnavailable) {
		t.Fatalf("error = %v, want ErrFleetUnavailable", err)
	}
}
