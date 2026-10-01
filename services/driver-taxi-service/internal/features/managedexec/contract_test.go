package managedexec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func nowUTC() time.Time { return time.Now().UTC() }

// fleetAdapterPaths are the exact paths fleet-partner-service's execution
// adapter calls, relative to the service's internal marketplace base.
//
// They are pinned here because Fleet's TaxiExecution adapter is the Dispatch
// adapter pointed at Taxi's base path: if Taxi's surface drifts from this
// list, every Fleet call silently 404s at runtime and the failure only shows
// up as "source unavailable" in production.
var fleetAdapterPaths = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/reservations"},
	{http.MethodGet, "/reservations/:sourceRequestId"},
	{http.MethodPost, "/reservations/:sourceRequestId/release"},
	{http.MethodGet, "/eligibility"},
	{http.MethodPost, "/executions"},
	{http.MethodGet, "/executions/by-idempotency/:idempotencyKey"},
	{http.MethodGet, "/requests/:sourceRequestId"},
	{http.MethodGet, "/trips/:sourceTripId"},
	{http.MethodPost, "/trips/:sourceTripId/cancel"},
}

func TestRegisteredRoutesMatchFleetExecutionContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/taxi")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"fleet-partner-service": []byte("secret")}, 0)
	RegisterRoutes(group, NewHandler(&Service{}, true, []string{"fleet-partner-service"}), verifier)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range fleetAdapterPaths {
		key := want.method + " /api/v1/taxi/internal/marketplace" + want.path
		if !registered[key] {
			t.Fatalf("Fleet's execution adapter calls %q but Taxi does not serve it", key)
		}
	}
}

// A nil verifier means no internal caller is trusted, so nothing is mounted.
// A half-open execution surface would be worse than none.
func TestRoutesUnmountedWithoutVerifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/taxi"), NewHandler(&Service{}, true, nil), nil)
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "internal/marketplace") {
			t.Fatalf("route %s %s mounted without a verifier", route.Method, route.Path)
		}
	}
}

// The integration must stay dark until it is switched on (TAX-028).
func TestDisabledIntegrationRefusesEveryRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/taxi")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"fleet-partner-service": []byte("secret")}, 0)
	// enabled=false
	RegisterRoutes(group, NewHandler(&Service{}, false, []string{"fleet-partner-service"}), verifier)

	body := `{"source_request_id":"r","organization_id":"o","idempotency_key":"k"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/taxi/internal/marketplace/reservations",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, "fleet-partner-service", []byte("secret"), []byte(body), nowUTC()); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 while the integration is disabled", recorder.Code)
	}
}

// Only allowlisted services may drive taxi execution. HMAC proves who is
// calling; the allowlist decides whether they may.
func TestNonAllowlistedCallerIsRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/taxi")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		"fleet-partner-service": []byte("secret"),
		"vendor-service":        []byte("other"),
	}, 0)
	RegisterRoutes(group, NewHandler(&Service{}, true, []string{"fleet-partner-service"}), verifier)

	body := `{"source_request_id":"r","organization_id":"o","idempotency_key":"k"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/taxi/internal/marketplace/reservations",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, "vendor-service", []byte("other"), []byte(body), nowUTC()); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a caller that is not allowlisted", recorder.Code)
	}
}

// The reservation and execution status vocabularies must be exactly Fleet's:
// a status Fleet cannot interpret is as bad as no answer at all.
func TestOutcomeVocabularyMatchesFleet(t *testing.T) {
	for _, status := range []string{
		OutcomeReserved, OutcomeAlreadyClaimed, OutcomeNotEligible,
		OutcomeExpired, OutcomeCancelled, OutcomeVersionMismatch,
	} {
		if status != strings.ToUpper(status) {
			t.Fatalf("reservation status %q is not Fleet's vocabulary", status)
		}
	}
	for _, status := range []string{
		ExecutionCreated, ExecutionAlreadyExists, ExecutionRejected, ExecutionNotReserved,
	} {
		if status != strings.ToUpper(status) {
			t.Fatalf("execution status %q is not Fleet's vocabulary", status)
		}
	}
}

// The wire field names must match Fleet's adapter exactly. Fleet reads
// reservations with snake_case and executions with camelCase (it inherited
// both from the dispatch contract), so this pins both.
func TestWireFieldNames(t *testing.T) {
	reservation, _ := json.Marshal(ReservationOutcome{ReservationID: "r", Status: "RESERVED"})
	for _, field := range []string{"reservation_id", "status", "winner_organization_id", "source_version"} {
		if !strings.Contains(string(reservation), `"`+field+`"`) {
			t.Fatalf("reservation outcome is missing %q: %s", field, reservation)
		}
	}
	execution, _ := json.Marshal(ExecutionOutcome{Status: "CREATED"})
	for _, field := range []string{"status", "tripId", "requestId", "requestVersion", "rejectionCode"} {
		if !strings.Contains(string(execution), `"`+field+`"`) {
			t.Fatalf("execution outcome is missing %q: %s", field, execution)
		}
	}
}

// ConfirmInput refuses to create an organization-authorized execution
// without every identifier TAX-012 requires.
func TestConfirmInputValidation(t *testing.T) {
	complete := ConfirmInput{
		SourceRequestID: "req-1", ReservationID: "res-1", OrganizationID: "org-1",
		ExecutingProviderID: "prov-1", OrganizationMembershipID: "mem-1",
		FleetAssignmentID: "asg-1",
	}
	if err := complete.Validate(); err != nil {
		t.Fatalf("complete input rejected: %v", err)
	}
	for _, drop := range []func(*ConfirmInput){
		func(in *ConfirmInput) { in.SourceRequestID = "" },
		func(in *ConfirmInput) { in.ReservationID = "" },
		func(in *ConfirmInput) { in.OrganizationID = "" },
		func(in *ConfirmInput) { in.ExecutingProviderID = "" },
		func(in *ConfirmInput) { in.OrganizationMembershipID = "" },
		func(in *ConfirmInput) { in.FleetAssignmentID = "" },
	} {
		partial := complete
		drop(&partial)
		if err := partial.Validate(); err == nil {
			t.Fatalf("incomplete input accepted: %+v", partial)
		}
	}
}
