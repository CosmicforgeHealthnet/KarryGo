package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/router"
)

const (
	compositionSecret     = "composition-fleet-secret"
	compositionCallerName = "driver-dispatch-delivery-service"
	accessTokenSecret     = "composition-access-token-secret-value"
)

// fleetReadServer is a fleet-shaped API enforcing real service authentication
// and scoping every answer to the provider in the path.
type fleetReadServer struct {
	server *httptest.Server

	offers      map[string][]map[string]any
	assignments map[string][]map[string]any
	forceStatus int
}

func newFleetReadServer(t *testing.T) *fleetReadServer {
	t.Helper()
	fleet := &fleetReadServer{
		offers:      map[string][]map[string]any{},
		assignments: map[string][]map[string]any{},
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{compositionCallerName: []byte(compositionSecret)}, time.Minute)

	guard := func(c *gin.Context) bool {
		if _, err := verifier.VerifyRequest(c.Request); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized"}})
			return false
		}
		if fleet.forceStatus != 0 {
			c.JSON(fleet.forceStatus, gin.H{"error": gin.H{"code": "fleet_unavailable"}})
			return false
		}
		return true
	}

	group := engine.Group("/internal/fleet-partner/v1")
	group.GET("/providers/:providerId/offers", func(c *gin.Context) {
		if !guard(c) {
			return
		}
		items := fleet.offers[c.Param("providerId")]
		if items == nil {
			items = []map[string]any{}
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": items, "hasMore": false}})
	})
	group.GET("/providers/:providerId/offers/:offerId", func(c *gin.Context) {
		if !guard(c) {
			return
		}
		for _, item := range fleet.offers[c.Param("providerId")] {
			if item["offerId"] == c.Param("offerId") {
				c.JSON(http.StatusOK, gin.H{"data": item})
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "marketplace_offer_not_found"}})
	})
	group.GET("/providers/:providerId/assignments", func(c *gin.Context) {
		if !guard(c) {
			return
		}
		items := fleet.assignments[c.Param("providerId")]
		if items == nil {
			items = []map[string]any{}
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"items": items, "hasMore": false}})
	})
	group.GET("/providers/:providerId/assignments/:assignmentId", func(c *gin.Context) {
		if !guard(c) {
			return
		}
		for _, item := range fleet.assignments[c.Param("providerId")] {
			if item["assignmentId"] == c.Param("assignmentId") {
				c.JSON(http.StatusOK, gin.H{"data": item})
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "assignment_not_found"}})
	})

	fleet.server = httptest.NewServer(engine)
	t.Cleanup(fleet.server.Close)
	return fleet
}

// newDispatchThroughCompositionRoot builds the dispatch engine via router.New.
//
// This is the point of the test: the routes, the auth middleware and the
// ProviderReader all come from the real composition root, not from a
// hand-assembled handler.
func newDispatchThroughCompositionRoot(t *testing.T, fleetURL string) (*httptest.Server, *authusecases.AuthUsecase) {
	t.Helper()

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	cfg := config.Config{
		AppEnv:                            "test",
		ServiceName:                       "dispatch-delivery-service",
		FleetServiceURL:                   fleetURL,
		FleetServiceCaller:                compositionCallerName,
		FleetServiceSecret:                []byte(compositionSecret),
		FleetServiceTimeout:               3 * time.Second,
		ProviderOrganizationOffersEnabled: true,
		ProviderCompanyAssignmentsEnabled: true,
		AccessTokenSecret:                 []byte(accessTokenSecret),
		AccessTokenTTL:                    time.Hour,
		RefreshTokenTTL:                   time.Hour,
	}

	authService := authusecases.NewAuthUsecase(authusecases.Options{
		AccessTokenSecret:  cfg.AccessTokenSecret,
		RefreshTokenSecret: cfg.AccessTokenSecret,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		Redis:              redisClient,
	})

	engine := router.New(cfg, nil, redisClient, authService, nil, nil, nil, nil)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server, authService
}

// providerToken mints a real access token for a provider session.
func providerToken(t *testing.T, authService *authusecases.AuthUsecase, providerID string) string {
	t.Helper()
	// The session id must itself be a UUID: the token validator parses both
	// the provider id and the session id.
	token, _, err := authService.TokenUsecase().GenerateAccessToken(
		providerID, "+2348090000000", uuid.NewString())
	if err != nil {
		t.Fatalf("mint provider token: %v", err)
	}
	return token
}

func compositionCall(t *testing.T, server *httptest.Server, path string, token string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL+path, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

func compositionOffer(offerID string) map[string]any {
	return map[string]any{
		"offerId": offerID, "requestId": "req-comp-1", "serviceType": "dispatch",
		"status": "OFFERED", "expiresAt": "2026-01-01T00:00:00Z",
		"allowedActions": []string{"accept", "decline"},
		// Hostile extras: fleet must never send these, and if it did the
		// reader must not pass them through.
		"fareAmount": 4500, "organizationSettlement": 3800, "margin": 700,
	}
}

func compositionAssignment(assignmentID string) map[string]any {
	return map[string]any{
		"assignmentId": assignmentID, "requestId": "req-comp-1",
		"status": "PENDING_PROVIDER_RESPONSE", "responseDeadline": "2026-01-01T00:00:00Z",
		"allowedActions":     []string{"accept", "decline"},
		"platformCommission": 200, "customerPayment": "card",
	}
}

func assertNoCommercialFields(t *testing.T, label string, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode %s: %v", label, err)
	}
	for _, forbidden := range []string{
		"fare_amount", "fareAmount", "customerFare", "customerPrice", "quotedPrice",
		"organizationSettlement", "companySettlement", "platformCommission",
		"commissionPercentage", "profit", "margin", "contractTerms",
		"customerWallet", "organizationWallet", "customerPayment",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("%s leaked %q: %s", label, forbidden, encoded)
		}
	}
}

// TestProviderReadRoutesThroughCompositionRoot proves the four routes work
// end to end through router.New: real auth middleware, real injected reader,
// real signed HTTP to fleet.
func TestProviderReadRoutesThroughCompositionRoot(t *testing.T) {
	fleet := newFleetReadServer(t)
	providerA := "11111111-1111-4111-8111-111111111111"
	providerB := "22222222-2222-4222-8222-222222222222"
	fleet.offers[providerA] = []map[string]any{compositionOffer("off-a")}
	fleet.assignments[providerA] = []map[string]any{compositionAssignment("asg-a")}

	dispatch, authService := newDispatchThroughCompositionRoot(t, fleet.server.URL)
	tokenA := providerToken(t, authService, providerA)

	// 1. Offer list.
	status, body := compositionCall(t, dispatch, "/api/v1/provider/organization-offers", tokenA)
	if status != http.StatusOK {
		t.Fatalf("offer list = %d (%v)", status, body)
	}
	assertNoCommercialFields(t, "offer list", body)
	if !strings.Contains(mustEncode(t, body), "off-a") {
		t.Fatalf("offer list did not return fleet data: %v", body)
	}

	// 2. Offer detail.
	status, body = compositionCall(t, dispatch,
		"/api/v1/provider/organization-offers/off-a", tokenA)
	if status != http.StatusOK {
		t.Fatalf("offer detail = %d (%v)", status, body)
	}
	assertNoCommercialFields(t, "offer detail", body)
	if !strings.Contains(mustEncode(t, body), "off-a") {
		t.Fatalf("offer detail did not return fleet data: %v", body)
	}

	// 3. Assignment list.
	status, body = compositionCall(t, dispatch, "/api/v1/provider/company-assignments", tokenA)
	if status != http.StatusOK {
		t.Fatalf("assignment list = %d (%v)", status, body)
	}
	assertNoCommercialFields(t, "assignment list", body)
	if !strings.Contains(mustEncode(t, body), "asg-a") {
		t.Fatalf("assignment list did not return fleet data: %v", body)
	}

	// 4. Assignment detail.
	status, body = compositionCall(t, dispatch,
		"/api/v1/provider/company-assignments/asg-a", tokenA)
	if status != http.StatusOK {
		t.Fatalf("assignment detail = %d (%v)", status, body)
	}
	assertNoCommercialFields(t, "assignment detail", body)
	if !strings.Contains(mustEncode(t, body), "asg-a") {
		t.Fatalf("assignment detail did not return fleet data: %v", body)
	}

	// Provider B holds a valid session but owns nothing: they must not see
	// provider A's records.
	tokenB := providerToken(t, authService, providerB)
	status, body = compositionCall(t, dispatch, "/api/v1/provider/organization-offers", tokenB)
	if status == http.StatusOK && strings.Contains(mustEncode(t, body), "off-a") {
		t.Fatal("provider B read provider A's offers")
	}
	status, _ = compositionCall(t, dispatch,
		"/api/v1/provider/organization-offers/off-a", tokenB)
	if status == http.StatusOK {
		t.Fatal("provider B read provider A's offer detail")
	}
	status, _ = compositionCall(t, dispatch,
		"/api/v1/provider/company-assignments/asg-a", tokenB)
	if status == http.StatusOK {
		t.Fatal("provider B read provider A's assignment detail")
	}
}

// TestProviderReadRoutesRejectNonProviderSessions covers the auth boundary
// through the real middleware.
func TestProviderReadRoutesRejectNonProviderSessions(t *testing.T) {
	fleet := newFleetReadServer(t)
	dispatch, _ := newDispatchThroughCompositionRoot(t, fleet.server.URL)

	// No token at all.
	if status, _ := compositionCall(t, dispatch,
		"/api/v1/provider/organization-offers", ""); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", status)
	}

	// A token signed with a different secret stands in for a customer or
	// corporate session: it is not a valid dispatch-provider session.
	foreign := authusecases.NewTokenUsecase([]byte("a-different-service-secret"),
		time.Hour, time.Hour)
	foreignToken, _, err := foreign.GenerateAccessToken(
		"44444444-4444-4444-8444-444444444444", "+2348091111111", uuid.NewString())
	if err != nil {
		t.Fatalf("mint foreign token: %v", err)
	}
	if status, _ := compositionCall(t, dispatch,
		"/api/v1/provider/organization-offers", foreignToken); status != http.StatusUnauthorized {
		t.Fatalf("foreign-issuer token = %d, want 401", status)
	}
	if status, _ := compositionCall(t, dispatch,
		"/api/v1/provider/company-assignments", foreignToken); status != http.StatusUnauthorized {
		t.Fatalf("foreign-issuer token on assignments = %d, want 401", status)
	}
}

// TestProviderReadRoutesFailClosedOnFleetOutage proves an outage is reported,
// never rendered as "you have no work".
func TestProviderReadRoutesFailClosedOnFleetOutage(t *testing.T) {
	fleet := newFleetReadServer(t)
	provider := "33333333-3333-4333-8333-333333333333"
	fleet.offers[provider] = []map[string]any{compositionOffer("off-a")}
	fleet.assignments[provider] = []map[string]any{compositionAssignment("asg-a")}

	dispatch, authService := newDispatchThroughCompositionRoot(t, fleet.server.URL)
	token := providerToken(t, authService, provider)

	fleet.forceStatus = http.StatusServiceUnavailable
	for _, path := range []string{
		"/api/v1/provider/organization-offers",
		"/api/v1/provider/company-assignments",
	} {
		status, body := compositionCall(t, dispatch, path, token)
		if status == http.StatusOK {
			t.Fatalf("%s returned success during a fleet outage: %v", path, body)
		}
		// The critical distinction: an outage must not look like an empty
		// result set, which a provider app would render as "no work".
		encoded := mustEncode(t, body)
		if strings.Contains(encoded, `"items":[]`) || strings.Contains(encoded, `"offers":[]`) {
			t.Fatalf("%s returned an empty successful list during an outage: %s", path, encoded)
		}
		assertNoCommercialFields(t, "outage error", body)
	}
}

func mustEncode(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(encoded)
}

var _ = context.Background
var _ = pgxpool.Pool{}
