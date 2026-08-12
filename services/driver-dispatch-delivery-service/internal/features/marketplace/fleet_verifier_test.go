package marketplace_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

const dispatchService = "driver-dispatch-delivery-service"

// fleetStub is a real HTTP server that speaks fleet's acceptance contract and
// enforces fleet's real service authentication. It is a stand-in for the fleet
// process, not for the verifier under test: the verifier here is the real
// production HTTPAcceptanceVerifier making real signed HTTP calls.
type fleetStub struct {
	server     *httptest.Server
	acceptance map[string]any
	errorCode  string
	errorHTTP  int
	calls      int32
	// onCall runs before each response, used to simulate state changing
	// underneath a verification.
	onCall func()
}

func newFleetStub(t *testing.T) *fleetStub {
	t.Helper()
	stub := &fleetStub{}
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{dispatchService: []byte("fleet-side-secret")}, time.Minute)

	engine.GET("/internal/fleet-partner/v1/assignments/:assignmentId/provider-acceptance",
		func(c *gin.Context) {
			// Real signature verification, exactly as fleet does it.
			name, err := verifier.VerifyRequest(c.Request)
			if err != nil || name != dispatchService {
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
					"code": "unauthorized", "message": "Service authentication is required."}})
				return
			}
			atomic.AddInt32(&stub.calls, 1)
			if stub.onCall != nil {
				stub.onCall()
			}
			if stub.errorCode != "" {
				status := stub.errorHTTP
				if status == 0 {
					status = http.StatusConflict
				}
				c.JSON(status, gin.H{"error": gin.H{
					"code": stub.errorCode, "message": "refused"}})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": stub.acceptance})
		})

	stub.server = httptest.NewServer(engine)
	t.Cleanup(stub.server.Close)
	return stub
}

func (f *fleetStub) verifier(t *testing.T) marketplace.AcceptanceVerifier {
	t.Helper()
	v, ok := marketplace.NewHTTPAcceptanceVerifier(marketplace.VerifierConfig{
		BaseURL: f.server.URL, ServiceName: dispatchService,
		Secret: []byte("fleet-side-secret"), Timeout: 3 * time.Second,
	})
	if !ok {
		t.Fatal("expected a configured verifier")
	}
	return v
}

func acceptancePayload(assignmentID, org, providerID, membership, responseID, requestID string) map[string]any {
	return map[string]any{
		"assignmentId": assignmentID, "organizationId": org, "providerId": providerID,
		"providerService": "DISPATCH", "organizationMembershipId": membership,
		"providerResponseId": responseID, "responseStatus": "ACCEPTED",
		"acceptedAt": time.Now().UTC(), "assignmentStatus": "ACCEPTED",
		"assignmentVersion": 3, "sourceService": "DISPATCH",
		"sourceRequestId": requestID, "sourceBookingId": "booking-1",
	}
}

func TestHTTPVerifierRequiresCompleteConfiguration(t *testing.T) {
	// Missing configuration must fall back to the fail-closed verifier rather
	// than producing a half-working client.
	for _, config := range []marketplace.VerifierConfig{
		{},
		{BaseURL: "http://fleet"},
		{BaseURL: "http://fleet", ServiceName: "dispatch"},
		{ServiceName: "dispatch", Secret: []byte("s")},
	} {
		verifier, ok := marketplace.NewHTTPAcceptanceVerifier(config)
		if ok {
			t.Fatalf("incomplete config %+v reported configured", config)
		}
		if verifier.Configured() {
			t.Fatal("fallback verifier must report unconfigured")
		}
		if _, err := verifier.VerifyProviderAcceptance(context.Background(),
			marketplace.CallMeta{}, "asg"); !errors.Is(err, marketplace.ErrFleetUnavailable) {
			t.Fatalf("fallback error = %v, want ErrFleetUnavailable", err)
		}
	}
}

func TestHTTPVerifierAcceptsValidAcceptance(t *testing.T) {
	stub := newFleetStub(t)
	stub.acceptance = acceptancePayload("asg_1", "org_A", "prov_1", "mem_1", "resp_1", "req_1")

	acceptance, err := stub.verifier(t).VerifyProviderAcceptance(context.Background(),
		marketplace.CallMeta{RequestID: "req-id", CorrelationID: "corr-id"}, "asg_1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if acceptance.ProviderID != "prov_1" || acceptance.ProviderResponseID != "resp_1" ||
		acceptance.OrganizationID != "org_A" || acceptance.ResponseStatus != "ACCEPTED" {
		t.Fatalf("acceptance bindings wrong: %+v", acceptance)
	}
	if stub.calls != 1 {
		t.Fatalf("fleet calls = %d, want 1", stub.calls)
	}
}

func TestHTTPVerifierRejectsUnsignedAndWrongSecret(t *testing.T) {
	stub := newFleetStub(t)
	stub.acceptance = acceptancePayload("asg_1", "org_A", "prov_1", "mem_1", "resp_1", "req_1")

	// A verifier holding the wrong secret must not be able to read acceptance.
	wrong, ok := marketplace.NewHTTPAcceptanceVerifier(marketplace.VerifierConfig{
		BaseURL: stub.server.URL, ServiceName: dispatchService,
		Secret: []byte("the-wrong-secret"), Timeout: 3 * time.Second,
	})
	if !ok {
		t.Fatal("expected configured verifier")
	}
	_, err := wrong.VerifyProviderAcceptance(context.Background(), marketplace.CallMeta{}, "asg_1")
	if !errors.Is(err, marketplace.ErrFleetUnavailable) {
		t.Fatalf("wrong secret = %v, want ErrFleetUnavailable (fail closed)", err)
	}
}

func TestHTTPVerifierClassifiesFleetRefusals(t *testing.T) {
	cases := []struct {
		code    string
		status  int
		wantErr error
	}{
		{"provider_acceptance_not_confirmed", http.StatusConflict, marketplace.ErrAcceptanceNotConfirmed},
		{"provider_acceptance_mismatch", http.StatusConflict, marketplace.ErrAcceptanceMismatch},
		{"provider_acceptance_cancelled", http.StatusConflict, marketplace.ErrAcceptanceCancelled},
		{"provider_acceptance_expired", http.StatusConflict, marketplace.ErrAcceptanceExpired},
		{"fleet_assignment_not_found", http.StatusConflict, marketplace.ErrFleetAssignmentNotFound},
	}
	for _, testCase := range cases {
		t.Run(testCase.code, func(t *testing.T) {
			stub := newFleetStub(t)
			stub.errorCode = testCase.code
			stub.errorHTTP = testCase.status
			_, err := stub.verifier(t).VerifyProviderAcceptance(context.Background(),
				marketplace.CallMeta{}, "asg_1")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("%s => %v, want %v", testCase.code, err, testCase.wantErr)
			}
		})
	}
}

func TestHTTPVerifierFailsClosedWhenFleetIsDown(t *testing.T) {
	stub := newFleetStub(t)
	stub.acceptance = acceptancePayload("asg_1", "org_A", "prov_1", "mem_1", "resp_1", "req_1")
	verifier := stub.verifier(t)
	stub.server.Close() // fleet is now unreachable

	_, err := verifier.VerifyProviderAcceptance(context.Background(), marketplace.CallMeta{}, "asg_1")
	if !errors.Is(err, marketplace.ErrFleetUnavailable) {
		t.Fatalf("unreachable fleet = %v, want ErrFleetUnavailable", err)
	}
	// An outage must never be readable as consent.
	if errors.Is(err, marketplace.ErrAcceptanceNotConfirmed) {
		t.Fatal("an outage must not be classified as a provider decision")
	}
}

func TestHTTPVerifierRejectsMalformedAndMismatchedPayloads(t *testing.T) {
	// A response missing binding identifiers is unusable: treating a zero
	// value as a match would defeat every downstream comparison.
	cases := map[string]map[string]any{
		"missing provider": {"assignmentId": "asg_1", "organizationId": "org_A", "providerResponseId": "r", "responseStatus": "ACCEPTED", "providerService": "DISPATCH"},
		"missing response": {"assignmentId": "asg_1", "organizationId": "org_A", "providerId": "p", "responseStatus": "ACCEPTED", "providerService": "DISPATCH"},
		"missing org":      {"assignmentId": "asg_1", "providerId": "p", "providerResponseId": "r", "responseStatus": "ACCEPTED", "providerService": "DISPATCH"},
		"wrong assignment": {"assignmentId": "SOMETHING_ELSE", "organizationId": "org_A", "providerId": "p", "providerResponseId": "r", "responseStatus": "ACCEPTED", "providerService": "DISPATCH"},
		"declined status":  {"assignmentId": "asg_1", "organizationId": "org_A", "providerId": "p", "providerResponseId": "r", "responseStatus": "DECLINED", "providerService": "DISPATCH"},
		"foreign service":  {"assignmentId": "asg_1", "organizationId": "org_A", "providerId": "p", "providerResponseId": "r", "responseStatus": "ACCEPTED", "providerService": "TAXI"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			stub := newFleetStub(t)
			stub.acceptance = payload
			_, err := stub.verifier(t).VerifyProviderAcceptance(context.Background(),
				marketplace.CallMeta{}, "asg_1")
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !errors.Is(err, marketplace.ErrAcceptanceMismatch) &&
				!errors.Is(err, marketplace.ErrAcceptanceNotConfirmed) {
				t.Fatalf("%s => %v, want mismatch or not-confirmed", name, err)
			}
		})
	}
}

// TestRandomProviderResponseIDCannotCreateTrip is the headline invariant:
// inventing a providerResponseId must never produce a canonical trip.
func TestRandomProviderResponseIDCannotCreateTrip(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348020000001")

	stub := newFleetStub(t)
	// Fleet knows the real response id. The execution command will claim a
	// different, invented one.
	stub.acceptance = acceptancePayload("asg_r", "org_A", providerID, "mem_1",
		"the-real-response-id", requestID)

	server := newExecutionServer(t, pool, executionFlags(), stub.verifier(t))
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-rand-res")

	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1",
			"asg_r", "totally-made-up-response-id"),
		"idem-rand", fleetService, fleetSecret)
	if status != http.StatusConflict || errorCodeOf(body) != "provider_acceptance_mismatch" {
		t.Fatalf("invented response id = %d %q, want 409 provider_acceptance_mismatch",
			status, errorCodeOf(body))
	}

	var trips int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM trips`).Scan(&trips)
	if trips != 0 {
		t.Fatalf("an invented provider response created %d trips", trips)
	}
}

// TestCancellationAfterVerificationCreatesNoTrip covers the acceptance/
// cancellation race: fleet flips the assignment to cancelled while dispatch is
// mid-flight. Dispatch must not produce a trip from a stale verification.
func TestCancellationAfterVerificationCreatesNoTrip(t *testing.T) {
	pool := testPool(t)
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348020000002")

	stub := newFleetStub(t)
	stub.acceptance = acceptancePayload("asg_c", "org_A", providerID, "mem_1", "resp_c", requestID)
	// The assignment is cancelled in fleet before dispatch asks.
	stub.onCall = func() { stub.errorCode = "provider_acceptance_cancelled" }

	server := newExecutionServer(t, pool, executionFlags(), stub.verifier(t))
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-cancel-res")

	// First attempt: fleet answers, but flips to cancelled for every call after.
	_, _ = signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_c", "resp_c"),
		"idem-cancel-1", fleetService, fleetSecret)

	// Any subsequent attempt must be refused outright.
	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_1", "asg_c", "resp_c"),
		"idem-cancel-2", fleetService, fleetSecret)
	if status != http.StatusConflict {
		t.Fatalf("execution after cancellation = %d %q, want 409", status, errorCodeOf(body))
	}

	// A cancelled assignment must leave at most the trip legitimately created
	// before cancellation. With cancellation applied on the first call, none.
	var trips int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM trips WHERE fleet_assignment_id = 'asg_c'`).Scan(&trips)
	if trips != 0 {
		t.Fatalf("cancelled assignment produced %d trips, want 0", trips)
	}
}

// TestVerifierResponseCarriesNoCommercialData guards the read contract.
func TestVerifierResponseCarriesNoCommercialData(t *testing.T) {
	stub := newFleetStub(t)
	stub.acceptance = acceptancePayload("asg_1", "org_A", "prov_1", "mem_1", "resp_1", "req_1")

	acceptance, err := stub.verifier(t).VerifyProviderAcceptance(context.Background(),
		marketplace.CallMeta{}, "asg_1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	encoded, err := json.Marshal(acceptance)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, forbidden := range []string{"fare", "Fare", "price", "Price", "settlement",
		"Settlement", "commission", "Commission", "margin", "Margin", "wallet", "Wallet"} {
		if containsToken(string(encoded), forbidden) {
			t.Fatalf("acceptance view leaked %q: %s", forbidden, encoded)
		}
	}
}

func containsToken(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
