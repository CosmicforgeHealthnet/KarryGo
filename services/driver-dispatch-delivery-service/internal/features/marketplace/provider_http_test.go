package marketplace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// scriptedResponder lets a test choose what fleet answers.
type scriptedResponder struct {
	result     marketplace.RelayDeliveryResult
	err        error
	configured bool
	seen       []marketplace.RelayOperation
}

func (s *scriptedResponder) Configured() bool { return s.configured }

func (s *scriptedResponder) Deliver(_ context.Context, _ marketplace.CallMeta,
	op marketplace.RelayOperation) (marketplace.RelayDeliveryResult, error) {
	s.seen = append(s.seen, op)
	return s.result, s.err
}

// providerSession installs a fake verified session so tests exercise the real
// handler and identity extraction without minting JWTs.
//
// It stands in for the auth middleware only; the handler still reads the
// provider id exclusively through authhttp.DispatchRiderID.
func providerSession(providerID string, sessionID string, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if providerID != "" {
			c.Set(authhttp.ContextDispatchRiderID, providerID)
			c.Set(authhttp.ContextSessionID, sessionID)
			c.Set(authhttp.ContextRole, role)
		}
		c.Next()
	}
}

// newProviderServer builds a real Gin server with the real provider handler.
func newProviderServer(t *testing.T, pool *pgxpool.Pool, responder marketplace.FleetResponder,
	session gin.HandlerFunc, flags marketplace.Flags) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler(), session)

	repo := marketplace.NewRepository(pool)
	service := marketplace.NewService(repo, flags, nil).WithResponder(responder, 5)
	handler := marketplace.NewProviderHandler(service, flags)

	// Mount the same paths RegisterProviderRoutes uses, minus the JWT
	// middleware which the session helper above replaces.
	offers := engine.Group("/api/v1/provider/organization-offers")
	offers.Use(requireProviderRoleForTest())
	offers.POST("/:offerId/accept", handler.AcceptOffer)
	offers.POST("/:offerId/decline", handler.DeclineOffer)
	offers.GET("", handler.ListOffers)

	assignments := engine.Group("/api/v1/provider/company-assignments")
	assignments.Use(requireProviderRoleForTest())
	assignments.POST("/:assignmentId/accept", handler.AcceptAssignment)
	assignments.POST("/:assignmentId/decline", handler.DeclineAssignment)
	assignments.GET("", handler.ListAssignments)

	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

func requireProviderRoleForTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(authhttp.ContextDispatchRiderID) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{
				"code": "unauthorized", "message": "A provider session is required."}})
			return
		}
		if c.GetString(authhttp.ContextRole) != authmodels.RoleDispatchProvider {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code": "forbidden", "message": "Dispatch providers only."}})
			return
		}
		c.Next()
	}
}

func providerCall(t *testing.T, server *httptest.Server, method string, path string,
	body any) (int, map[string]any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

func providerFlags() marketplace.Flags {
	return marketplace.Flags{ProviderOffersEnabled: true, CompanyAssignEnabled: true}
}

func acceptingResponder() *scriptedResponder {
	return &scriptedResponder{configured: true,
		result: marketplace.RelayDeliveryResult{Accepted: true}}
}

func TestProviderRoutesRequireAProviderSession(t *testing.T) {
	pool := testPool(t)

	// No session at all.
	anonymous := newProviderServer(t, pool, acceptingResponder(),
		providerSession("", "", ""), providerFlags())
	status, _ := providerCall(t, anonymous, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept",
		map[string]any{"clientResponseId": "c1"})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", status)
	}

	// A customer session authenticates but must not authorize a provider action.
	customer := newProviderServer(t, pool, acceptingResponder(),
		providerSession("cust-1", "sess-1", "customer"), providerFlags())
	status, _ = providerCall(t, customer, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept",
		map[string]any{"clientResponseId": "c1"})
	if status != http.StatusForbidden {
		t.Fatalf("customer role = %d, want 403", status)
	}

	// A corporate session likewise.
	corporate := newProviderServer(t, pool, acceptingResponder(),
		providerSession("staff-1", "sess-2", "corporate_user"), providerFlags())
	status, _ = providerCall(t, corporate, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept",
		map[string]any{"clientResponseId": "c1"})
	if status != http.StatusForbidden {
		t.Fatalf("corporate role = %d, want 403", status)
	}

	// None of the rejected calls recorded a decision.
	var relays int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 0 {
		t.Fatalf("rejected calls created %d relays", relays)
	}
}

func TestClientSuppliedProviderIDCannotImpersonate(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	real := seedProvider(t, pool, "+2348040000001")
	victim := seedProvider(t, pool, "+2348040000002")

	responder := acceptingResponder()
	server := newProviderServer(t, pool, responder,
		providerSession(real, "sess-real", authmodels.RoleDispatchProvider), providerFlags())

	// The client tries every field it might hope is honoured.
	status, _ := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-imp/accept?providerId="+victim,
		map[string]any{
			"clientResponseId":    "c-imp",
			"providerId":          victim,
			"provider_id":         victim,
			"executingProviderId": victim,
		})
	if status != http.StatusOK {
		t.Fatalf("accept = %d, want 200", status)
	}

	// The relay is attributed to the SESSION provider, not the body.
	var storedProvider string
	if err := pool.QueryRow(ctx,
		`SELECT provider_id FROM dispatch_fleet_relay_operations`).Scan(&storedProvider); err != nil {
		t.Fatalf("load relay: %v", err)
	}
	if storedProvider != real {
		t.Fatalf("relay provider = %s, want the session provider %s", storedProvider, real)
	}
	if storedProvider == victim {
		t.Fatal("a client-supplied providerId impersonated another provider")
	}

	// And the payload delivered to fleet names the session provider.
	if len(responder.seen) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(responder.seen))
	}
	var payload map[string]any
	_ = json.Unmarshal(responder.seen[0].Payload, &payload)
	if payload["providerId"] != real {
		t.Fatalf("delivered providerId = %v, want %s", payload["providerId"], real)
	}
}

func TestProviderActionsCreateExactlyOneRelayEach(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		body     map[string]any
		wantType string
	}{
		// Offer accept is the DELEGATED path: it also reserves the canonical
		// request, so it needs the request and organization references.
		{"offer accept", "/api/v1/provider/organization-offers/off-1/accept",
			map[string]any{"clientResponseId": "c-oa", "needsBroadcast": true,
				"organizationId": "org_A", "organizationMembershipId": "mem_1"},
			marketplace.RelayProviderOfferAccept},
		{"offer decline", "/api/v1/provider/organization-offers/off-2/decline",
			map[string]any{"clientResponseId": "c-od", "reasonCode": "EXCESSIVE_DISTANCE"},
			marketplace.RelayProviderOfferDecline},
		{"assignment accept", "/api/v1/provider/company-assignments/asg-1/accept",
			map[string]any{"clientResponseId": "c-aa"}, marketplace.RelayProviderAssignmentAccept},
		{"assignment decline", "/api/v1/provider/company-assignments/asg-2/decline",
			map[string]any{"clientResponseId": "c-ad", "reasonCode": "ILLNESS"},
			marketplace.RelayProviderAssignmentDecline},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			provider := seedEligibleProvider(t, pool, "+2348041000001")
			server := newProviderServer(t, pool, acceptingResponder(),
				providerSession(provider, "sess-1", authmodels.RoleDispatchProvider),
				delegatedFlags())

			body := map[string]any{}
			for key, value := range testCase.body {
				body[key] = value
			}
			if _, needs := body["needsBroadcast"]; needs {
				delete(body, "needsBroadcast")
				body["requestId"] = seedBroadcast(t, pool)
			}

			status, body2 := providerCall(t, server, http.MethodPost, testCase.path, body)
			body = body2
			if status != http.StatusOK {
				t.Fatalf("%s = %d (%v)", testCase.name, status, body)
			}

			var count int
			var operationType string
			_ = pool.QueryRow(ctx,
				`SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&count)
			if count != 1 {
				t.Fatalf("%s created %d relays, want exactly 1", testCase.name, count)
			}
			_ = pool.QueryRow(ctx,
				`SELECT operation_type FROM dispatch_fleet_relay_operations`).Scan(&operationType)
			if operationType != testCase.wantType {
				t.Fatalf("operation type = %s, want %s", operationType, testCase.wantType)
			}

			// A healthy fleet gives an immediate confirmation.
			data, _ := body["data"].(map[string]any)
			if data["status"] != marketplace.ProviderActionConfirmed {
				t.Fatalf("status = %v, want CONFIRMED", data["status"])
			}
		})
	}
}

func TestProviderDeclineValidatesReason(t *testing.T) {
	pool := testPool(t)
	provider := seedProvider(t, pool, "+2348042000001")
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	// Missing reason.
	status, _ := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c1"})
	if status != http.StatusBadRequest {
		t.Fatalf("missing reason = %d, want 400", status)
	}
	// Unknown reason.
	status, _ = providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c2", "reasonCode": "MADE_UP"})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown reason = %d, want 400", status)
	}
	// OTHER without a note.
	status, _ = providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c3", "reasonCode": "OTHER"})
	if status != http.StatusBadRequest {
		t.Fatalf("OTHER without note = %d, want 400", status)
	}

	var relays int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 0 {
		t.Fatalf("invalid declines created %d relays", relays)
	}

	// A safety reason is accepted, and nothing punitive is recorded here.
	status, _ = providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c4", "reasonCode": "UNSAFE_VEHICLE"})
	if status != http.StatusOK {
		t.Fatalf("safety decline = %d, want 200", status)
	}
}

func TestProviderDeclineIsNotGatedByFeatureFlag(t *testing.T) {
	pool := testPool(t)
	provider := seedProvider(t, pool, "+2348043000001")
	// Every write flag OFF.
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider),
		marketplace.Flags{})

	// Accepting is refused...
	status, _ := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept",
		map[string]any{"clientResponseId": "c-acc"})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("accept with flags off = %d, want 503", status)
	}

	// ...but declining must still work. A provider left holding work they
	// cannot refuse is exactly what the universal rule forbids.
	status, _ = providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c-dec", "reasonCode": "ILLNESS"})
	if status != http.StatusOK {
		t.Fatalf("decline with flags off = %d, want 200 — decline can never be disabled", status)
	}
}

func TestProviderActionReplaysAndDetectsConflict(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedProvider(t, pool, "+2348044000001")
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	body := map[string]any{"clientResponseId": "c-replay"}
	status, first := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept", body)
	if status != http.StatusOK {
		t.Fatalf("first accept = %d", status)
	}

	// A retried tap replays the SAME relay.
	status, second := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/accept", body)
	if status != http.StatusOK {
		t.Fatalf("replay = %d (%v)", status, second)
	}
	firstData, _ := first["data"].(map[string]any)
	secondData, _ := second["data"].(map[string]any)
	if firstData["relayId"] != secondData["relayId"] {
		t.Fatal("replay produced a different relay")
	}

	var relays int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 1 {
		t.Fatalf("relay rows = %d, want exactly 1", relays)
	}

	// The same key with a DIFFERENT decision is a conflict, not a merge.
	status, conflict := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-1/decline",
		map[string]any{"clientResponseId": "c-replay", "reasonCode": "ILLNESS"})
	if status != http.StatusConflict || errorCodeOf(conflict) != "idempotency_conflict" {
		t.Fatalf("changed decision under same key = %d %q, want 409 idempotency_conflict",
			status, errorCodeOf(conflict))
	}
}

func TestFleetOutageLeavesDecisionPendingAndDurable(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedProvider(t, pool, "+2348045000001")

	down := &scriptedResponder{configured: true, err: marketplace.ErrFleetUnavailable}
	server := newProviderServer(t, pool, down,
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	status, body := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-out/accept",
		map[string]any{"clientResponseId": "c-out"})
	// The provider's decision is safe, so this is a success with a pending
	// status — not an error the provider might retry as a new decision.
	if status != http.StatusOK {
		t.Fatalf("fleet outage = %d, want 200 with pending status", status)
	}
	data, _ := body["data"].(map[string]any)
	if data["status"] != marketplace.ProviderActionPending {
		t.Fatalf("status = %v, want PENDING", data["status"])
	}

	var relayStatus, key string
	if err := pool.QueryRow(ctx,
		`SELECT status, idempotency_key FROM dispatch_fleet_relay_operations`).
		Scan(&relayStatus, &key); err != nil {
		t.Fatalf("load relay: %v", err)
	}
	if relayStatus != marketplace.RelayUnknownOutcome {
		t.Fatalf("relay status = %s, want UNKNOWN_OUTCOME", relayStatus)
	}
	if key != "c-out" {
		t.Fatalf("idempotency key = %s; the original must be preserved", key)
	}
}

func TestPermanentFleetRejectionIsNotRetried(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedProvider(t, pool, "+2348046000001")

	refusing := &scriptedResponder{configured: true, result: marketplace.RelayDeliveryResult{
		Permanent: true, ErrorCode: "assignment_already_resolved",
	}}
	server := newProviderServer(t, pool, refusing,
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	status, body := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-rej/accept",
		map[string]any{"clientResponseId": "c-rej"})
	if status != http.StatusOK {
		t.Fatalf("permanent rejection = %d", status)
	}
	data, _ := body["data"].(map[string]any)
	if data["status"] != marketplace.ProviderActionRejected {
		t.Fatalf("status = %v, want REJECTED", data["status"])
	}
	if data["errorCode"] != "assignment_already_resolved" {
		t.Fatalf("errorCode = %v", data["errorCode"])
	}

	var relayStatus string
	_ = pool.QueryRow(ctx,
		`SELECT status FROM dispatch_fleet_relay_operations`).Scan(&relayStatus)
	if relayStatus != marketplace.RelayRejected {
		t.Fatalf("relay status = %s, want REJECTED", relayStatus)
	}
}

func TestProviderResponsesCarryNoCommercialFields(t *testing.T) {
	pool := testPool(t)
	provider := seedProvider(t, pool, "+2348047000001")
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	var payloads [][]byte
	for _, call := range []struct {
		path string
		body map[string]any
	}{
		{"/api/v1/provider/company-assignments/asg-1/accept",
			map[string]any{"clientResponseId": "c-1"}},
		{"/api/v1/provider/company-assignments/asg-2/decline",
			map[string]any{"clientResponseId": "c-2", "reasonCode": "ILLNESS"}},
		{"/api/v1/provider/organization-offers/off-1/accept",
			map[string]any{"clientResponseId": "c-3"}},
		// An error payload too.
		{"/api/v1/provider/company-assignments/asg-3/decline",
			map[string]any{"clientResponseId": "c-4", "reasonCode": "MADE_UP"}},
	} {
		_, body := providerCall(t, server, http.MethodPost, call.path, call.body)
		encoded, _ := json.Marshal(body)
		payloads = append(payloads, encoded)
	}
	// The list route while the fleet read contract is unconfigured.
	_, listBody := providerCall(t, server, http.MethodGet,
		"/api/v1/provider/company-assignments", nil)
	encoded, _ := json.Marshal(listBody)
	payloads = append(payloads, encoded)

	for _, payload := range payloads {
		for _, forbidden := range []string{
			"fare_amount", "fareAmount", "customerFare", "customerPrice", "quotedPrice",
			"organizationSettlement", "companySettlement", "platformCommission",
			"commissionPercentage", "profit", "margin", "contractTerms",
			"customerWallet", "organizationWallet", "customerPayment",
		} {
			if bytes.Contains(payload, []byte(forbidden)) {
				t.Fatalf("provider payload leaked %q: %s", forbidden, payload)
			}
		}
	}
}

func TestProviderReadsFailClosedWhenFleetContractUnconfigured(t *testing.T) {
	pool := testPool(t)
	provider := seedProvider(t, pool, "+2348048000001")
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	// An unconfigured read contract must report unavailable, NOT an empty
	// list — a provider app would render "no work" and the provider would
	// never learn that work exists.
	status, body := providerCall(t, server, http.MethodGet,
		"/api/v1/provider/organization-offers", nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured offer list = %d, want 503 (%v)", status, body)
	}
	status, _ = providerCall(t, server, http.MethodGet,
		"/api/v1/provider/company-assignments", nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured assignment list = %d, want 503", status)
	}
}

// TestRelayWorkerPicksUpPendingProviderDecisions proves the provider-facing
// layer and the durable worker are actually connected.
func TestRelayWorkerPicksUpPendingProviderDecisions(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedProvider(t, pool, "+2348049000001")

	// No network attempt has occurred: this decision is safe to deliver later.
	down := &scriptedResponder{configured: false}
	server := newProviderServer(t, pool, down,
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), providerFlags())

	if status, _ := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/company-assignments/asg-w/accept",
		map[string]any{"clientResponseId": "c-w"}); status != http.StatusOK {
		t.Fatalf("accept during outage = %d", status)
	}

	// The failed attempt scheduled a backoff, which is correct in production.
	// Bring it due so this test exercises the worker rather than the clock.
	if _, err := pool.Exec(ctx,
		`UPDATE dispatch_fleet_relay_operations SET next_attempt_at = now()`); err != nil {
		t.Fatalf("make relay due: %v", err)
	}

	// Fleet recovers; the worker delivers the SAME decision.
	up := &scriptedResponder{configured: true,
		result: marketplace.RelayDeliveryResult{Accepted: true}}
	worker := marketplace.NewRelayWorker(marketplace.NewRepository(pool), up,
		marketplace.RelayWorkerConfig{BaseBackoff: time.Millisecond})
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("worker: %v", err)
	}

	if len(up.seen) != 1 || up.seen[0].IdempotencyKey != "c-w" {
		t.Fatalf("worker deliveries = %+v; must reuse the original key", up.seen)
	}
	var status string
	_ = pool.QueryRow(ctx,
		`SELECT status FROM dispatch_fleet_relay_operations`).Scan(&status)
	if status != marketplace.RelayConfirmed {
		t.Fatalf("relay status = %s, want CONFIRMED", status)
	}
	var relays int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 1 {
		t.Fatalf("relay rows = %d; the decision must never be duplicated", relays)
	}
}
