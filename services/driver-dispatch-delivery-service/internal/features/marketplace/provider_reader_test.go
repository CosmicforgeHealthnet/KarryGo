package marketplace_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// fleetReadStub is a fleet-shaped read API enforcing real service auth and a
// caller allowlist, and scoping every answer to the provider in the path.
type fleetReadStub struct {
	server *httptest.Server

	// offersByProvider and assignmentsByProvider model fleet's ownership
	// scoping: a provider only ever sees their own rows.
	offersByProvider      map[string][]map[string]any
	assignmentsByProvider map[string][]map[string]any

	allowlisted map[string]bool
	forceStatus int
	lastAuth    string
	lastRequest string
}

func newFleetReadStub(t *testing.T) *fleetReadStub {
	t.Helper()
	stub := &fleetReadStub{
		offersByProvider:      map[string][]map[string]any{},
		assignmentsByProvider: map[string][]map[string]any{},
		allowlisted:           map[string]bool{dispatchService: true},
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{dispatchService: []byte(responderSecret)}, time.Minute)

	guard := func(c *gin.Context) (string, bool) {
		stub.lastRequest = c.GetHeader("X-Request-ID")
		name, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
			return "", false
		}
		stub.lastAuth = name
		if !stub.allowlisted[name] {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "forbidden"}})
			return "", false
		}
		if stub.forceStatus != 0 {
			c.JSON(stub.forceStatus, gin.H{"error": gin.H{"code": "fleet_unavailable"}})
			return "", false
		}
		return name, true
	}

	group := engine.Group("/internal/fleet-partner/v1")
	group.GET("/providers/:providerId/offers", func(c *gin.Context) {
		if _, ok := guard(c); !ok {
			return
		}
		items := stub.offersByProvider[c.Param("providerId")]
		if items == nil {
			items = []map[string]any{}
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": items, "hasMore": false}})
	})
	group.GET("/providers/:providerId/offers/:offerId", func(c *gin.Context) {
		if _, ok := guard(c); !ok {
			return
		}
		for _, item := range stub.offersByProvider[c.Param("providerId")] {
			if item["offerId"] == c.Param("offerId") {
				c.JSON(http.StatusOK, gin.H{"data": item})
				return
			}
		}
		// Not owned by this provider => not found, never "exists but not yours".
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "marketplace_offer_not_found"}})
	})
	group.GET("/providers/:providerId/assignments", func(c *gin.Context) {
		if _, ok := guard(c); !ok {
			return
		}
		items := stub.assignmentsByProvider[c.Param("providerId")]
		if items == nil {
			items = []map[string]any{}
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": items, "hasMore": false}})
	})
	group.GET("/providers/:providerId/assignments/:assignmentId", func(c *gin.Context) {
		if _, ok := guard(c); !ok {
			return
		}
		for _, item := range stub.assignmentsByProvider[c.Param("providerId")] {
			if item["assignmentId"] == c.Param("assignmentId") {
				c.JSON(http.StatusOK, gin.H{"data": item})
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "assignment_not_found"}})
	})

	stub.server = httptest.NewServer(engine)
	t.Cleanup(stub.server.Close)
	return stub
}

func (f *fleetReadStub) reader(t *testing.T, secret string) marketplace.ProviderReader {
	t.Helper()
	reader, ok := marketplace.NewHTTPProviderReader(marketplace.ResponderConfig{
		BaseURL: f.server.URL, ServiceName: dispatchService,
		Secret: []byte(secret), Timeout: 3 * time.Second,
	})
	if !ok {
		t.Fatal("expected a configured reader")
	}
	return reader
}

func offerRow(offerID string) map[string]any {
	return map[string]any{
		"offerId": offerID, "requestId": "req-1", "serviceType": "dispatch",
		"status": "OFFERED", "expiresAt": "2026-01-01T00:00:00Z",
		"allowedActions": []string{"accept", "decline"},
	}
}

func assignmentRow(assignmentID string) map[string]any {
	return map[string]any{
		"assignmentId": assignmentID, "requestId": "req-1",
		"status": "PENDING_PROVIDER_RESPONSE", "responseDeadline": "2026-01-01T00:00:00Z",
		"allowedActions": []string{"accept", "decline"},
	}
}

func TestProviderReaderReturnsOnlyTheOwnProviderRows(t *testing.T) {
	stub := newFleetReadStub(t)
	stub.offersByProvider["prov-a"] = []map[string]any{offerRow("off-a")}
	stub.offersByProvider["prov-b"] = []map[string]any{offerRow("off-b")}
	stub.assignmentsByProvider["prov-a"] = []map[string]any{assignmentRow("asg-a")}
	stub.assignmentsByProvider["prov-b"] = []map[string]any{assignmentRow("asg-b")}
	reader := stub.reader(t, responderSecret)
	ctx := context.Background()

	offers, err := reader.ListOffers(ctx, marketplace.CallMeta{}, "prov-a")
	if err != nil {
		t.Fatalf("list offers: %v", err)
	}
	if len(offers) != 1 || offers[0].OfferID != "off-a" {
		t.Fatalf("provider A saw %+v; must see only their own offer", offers)
	}

	assignments, err := reader.ListAssignments(ctx, marketplace.CallMeta{}, "prov-a")
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	if len(assignments) != 1 || assignments[0].AssignmentID != "asg-a" {
		t.Fatalf("provider A saw %+v; must see only their own assignment", assignments)
	}

	// Actionable rows always carry both actions.
	if len(offers[0].AllowedActions) != 2 ||
		offers[0].AllowedActions[0] != "accept" || offers[0].AllowedActions[1] != "decline" {
		t.Fatalf("allowedActions = %v, want accept and decline", offers[0].AllowedActions)
	}
}

func TestProviderReaderRejectsAnotherProvidersRow(t *testing.T) {
	stub := newFleetReadStub(t)
	stub.offersByProvider["prov-a"] = []map[string]any{offerRow("off-a")}
	stub.assignmentsByProvider["prov-a"] = []map[string]any{assignmentRow("asg-a")}
	reader := stub.reader(t, responderSecret)
	ctx := context.Background()

	// Provider B asking for provider A's offer must not receive it.
	if _, err := reader.GetOffer(ctx, marketplace.CallMeta{}, "prov-b", "off-a"); err == nil {
		t.Fatal("provider B read provider A's offer")
	}
	if _, err := reader.GetAssignment(ctx, marketplace.CallMeta{}, "prov-b", "asg-a"); err == nil {
		t.Fatal("provider B read provider A's assignment")
	}

	// And provider A still can.
	offer, err := reader.GetOffer(ctx, marketplace.CallMeta{}, "prov-a", "off-a")
	if err != nil || offer.OfferID != "off-a" {
		t.Fatalf("provider A could not read their own offer: %v", err)
	}
}

func TestProviderReaderUsesRealServiceAuthentication(t *testing.T) {
	stub := newFleetReadStub(t)
	stub.offersByProvider["prov-a"] = []map[string]any{offerRow("off-a")}
	ctx := context.Background()

	// Correct secret authenticates.
	if _, err := stub.reader(t, responderSecret).ListOffers(ctx,
		marketplace.CallMeta{RequestID: "req-trace"}, "prov-a"); err != nil {
		t.Fatalf("signed read failed: %v", err)
	}
	if stub.lastAuth != dispatchService {
		t.Fatalf("caller identity = %q", stub.lastAuth)
	}
	if stub.lastRequest != "req-trace" {
		t.Fatalf("request id not propagated: %q", stub.lastRequest)
	}

	// Wrong secret fails closed.
	if _, err := stub.reader(t, "the-wrong-secret").ListOffers(ctx,
		marketplace.CallMeta{}, "prov-a"); err == nil {
		t.Fatal("a wrong secret produced data")
	}

	// Non-allowlisted caller fails closed.
	stub.allowlisted = map[string]bool{"someone-else": true}
	if _, err := stub.reader(t, responderSecret).ListOffers(ctx,
		marketplace.CallMeta{}, "prov-a"); err == nil {
		t.Fatal("a non-allowlisted caller produced data")
	}
}

func TestProviderReaderFailsClosedOnFleetOutage(t *testing.T) {
	stub := newFleetReadStub(t)
	stub.offersByProvider["prov-a"] = []map[string]any{offerRow("off-a")}
	reader := stub.reader(t, responderSecret)
	ctx := context.Background()

	for _, status := range []int{
		http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusInternalServerError,
	} {
		stub.forceStatus = status
		offers, err := reader.ListOffers(ctx, marketplace.CallMeta{}, "prov-a")
		if err == nil {
			t.Fatalf("status %d returned data instead of failing closed", status)
		}
		// The critical property: NOT an empty success. A provider app would
		// render that as "you have no work".
		if offers != nil && len(offers) == 0 && err == nil {
			t.Fatalf("status %d produced an empty list", status)
		}
	}

	// A dead server likewise.
	stub.forceStatus = 0
	stub.server.Close()
	if _, err := reader.ListOffers(ctx, marketplace.CallMeta{}, "prov-a"); err == nil {
		t.Fatal("an unreachable fleet returned data")
	}
}

func TestProviderReaderPayloadsCarryNoCommercialFields(t *testing.T) {
	stub := newFleetReadStub(t)
	// Fleet answers correctly, but include hostile extra fields to prove the
	// reader rebuilds from an allowlist rather than passing the body through.
	hostileOffer := offerRow("off-a")
	hostileOffer["fareAmount"] = 4500
	hostileOffer["customerFare"] = 4500
	hostileOffer["organizationSettlement"] = 3800
	hostileOffer["margin"] = 700
	stub.offersByProvider["prov-a"] = []map[string]any{hostileOffer}

	hostileAssignment := assignmentRow("asg-a")
	hostileAssignment["platformCommission"] = 200
	hostileAssignment["customerPayment"] = "card"
	stub.assignmentsByProvider["prov-a"] = []map[string]any{hostileAssignment}

	reader := stub.reader(t, responderSecret)
	ctx := context.Background()

	offers, err := reader.ListOffers(ctx, marketplace.CallMeta{}, "prov-a")
	if err != nil {
		t.Fatalf("list offers: %v", err)
	}
	assignments, err := reader.ListAssignments(ctx, marketplace.CallMeta{}, "prov-a")
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}

	for _, payload := range []any{offers, assignments} {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		for _, forbidden := range []string{
			"fare_amount", "fareAmount", "customerFare", "customerPrice", "quotedPrice",
			"organizationSettlement", "companySettlement", "platformCommission",
			"commissionPercentage", "profit", "margin", "contractTerms",
			"customerWallet", "organizationWallet", "customerPayment",
		} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("reader passed through %q from fleet: %s", forbidden, encoded)
			}
		}
	}
}

func TestProviderReaderUnconfiguredFailsClosed(t *testing.T) {
	for _, config := range []marketplace.ResponderConfig{
		{},
		{BaseURL: "http://fleet"},
		{BaseURL: "http://fleet", ServiceName: dispatchService},
	} {
		reader, ok := marketplace.NewHTTPProviderReader(config)
		if ok {
			t.Fatalf("incomplete config %+v reported configured", config)
		}
		if reader.Configured() {
			t.Fatal("fallback must report unconfigured")
		}
		if _, err := reader.ListOffers(context.Background(),
			marketplace.CallMeta{}, "prov-a"); err == nil {
			t.Fatal("the fail-closed reader returned data")
		}
	}
}
