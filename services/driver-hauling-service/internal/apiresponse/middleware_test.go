package apiresponse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/providercompat"
)

type stubCorporate struct {
	ctx   providercompat.CorporateContext
	err   error
	calls int
}

func (s *stubCorporate) ReadCorporateContext(context.Context, string, string) (providercompat.CorporateContext, error) {
	s.calls++
	return s.ctx, s.err
}

const (
	providerRole    = "truck_provider"
	providerService = "hauling"
)

// routerWith builds a router whose single route emits a finance-bearing body,
// behind the projection middleware and a fake bearer layer.
func routerWith(t *testing.T, reader providercompat.CorporateContextReader, enabled bool, claims *sharedauth.Claims) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if claims != nil {
			c.Set(sharedauth.ClaimsContextKey, *claims)
		}
		c.Next()
	})
	router.Use(ProviderFinancialPrivacy(reader, enabled, providerRole, providerService))
	router.GET("/job", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"id": "book-1", "fare_final_kobo": 750000,
			"own_earning_minor": 600000, "distance_km": 42.5,
		}})
	})
	return router
}

func bodyOf(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
	}
	data, _ := out["data"].(map[string]any)
	return data
}

func managedContext() providercompat.CorporateContext {
	return providercompat.CorporateContext{
		ProviderID: "prov-1", IsManagedProvider: true, MembershipID: "mem-1",
		OrganizationID: "org-1", MembershipStatus: "ACTIVE",
		ServiceEligibility:        map[string]bool{"HAULING": true},
		FinancialVisibilityPolicy: providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true, ShowPayroll: true},
	}
}

// HLG-021: the projection is applied server-side to a real response body.
func TestMiddlewareProjectsManagedProviderResponse(t *testing.T) {
	router := routerWith(t, &stubCorporate{ctx: managedContext()}, true,
		&sharedauth.Claims{Subject: "prov-1", Role: providerRole, Service: providerService})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	data := bodyOf(t, recorder)
	if data["fare_final_kobo"] != nil {
		t.Fatal("the customer fare must be concealed for a managed driver")
	}
	if data["own_earning_minor"] == nil {
		t.Fatal("the driver's own earning must stay visible under this policy")
	}
	if data["distance_km"] == nil {
		t.Fatal("operational data must survive")
	}
	// Content-Length must match the projected body, or the response truncates.
	if got := recorder.Header().Get("Content-Length"); got != "" {
		if got != itoa(recorder.Body.Len()) {
			t.Fatalf("Content-Length %s does not match body length %d", got, recorder.Body.Len())
		}
	}
}

// HLG-024: an independent driver's body is returned untouched.
func TestMiddlewareLeavesIndependentProviderUntouched(t *testing.T) {
	router := routerWith(t, &stubCorporate{ctx: providercompat.CorporateContext{ProviderID: "prov-1"}}, true,
		&sharedauth.Claims{Subject: "prov-1", Role: providerRole, Service: providerService})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	data := bodyOf(t, recorder)
	for _, field := range []string{"fare_final_kobo", "own_earning_minor", "distance_km"} {
		if data[field] == nil {
			t.Fatalf("independent driver lost %s", field)
		}
	}
}

// HLG-021 fail-closed: a Fleet outage refuses the request rather than serving
// an unmasked body.
func TestMiddlewareFailsClosedOnFleetOutage(t *testing.T) {
	router := routerWith(t, &stubCorporate{err: errors.New("connection refused")}, true,
		&sharedauth.Claims{Subject: "prov-1", Role: providerRole, Service: providerService})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	if recorder.Body.Len() > 0 && bodyOf(t, recorder)["fare_final_kobo"] != nil {
		t.Fatal("a Fleet outage must never serve an unmasked fare")
	}
}

// A customer request carries claims too. Reading a customer id as a provider
// id would ask Fleet about the wrong subject entirely, so the middleware must
// ignore it completely.
func TestMiddlewareIgnoresNonProviderClaims(t *testing.T) {
	reader := &stubCorporate{ctx: managedContext()}
	router := routerWith(t, reader, true,
		&sharedauth.Claims{Subject: "cust-1", Role: "customer", Service: "customer"})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	if reader.calls != 0 {
		t.Fatalf("Fleet was asked about a customer subject %d time(s)", reader.calls)
	}
	if bodyOf(t, recorder)["fare_final_kobo"] == nil {
		t.Fatal("a customer response must not be projected")
	}
}

// An unauthenticated request passes straight through.
func TestMiddlewareIgnoresUnauthenticatedRequests(t *testing.T) {
	reader := &stubCorporate{ctx: managedContext()}
	router := routerWith(t, reader, true, nil)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	if reader.calls != 0 {
		t.Fatal("Fleet must not be consulted for an unauthenticated request")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
}

// With typed visibility disabled the deployment behaves exactly as before.
func TestMiddlewareDisabledPreservesPreviousBehaviour(t *testing.T) {
	reader := &stubCorporate{ctx: managedContext()}
	router := routerWith(t, reader, false,
		&sharedauth.Claims{Subject: "prov-1", Role: providerRole, Service: providerService})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/job", nil))

	if reader.calls != 0 {
		t.Fatal("a disabled projection must not call Fleet")
	}
	if bodyOf(t, recorder)["fare_final_kobo"] == nil {
		t.Fatal("a disabled projection must not alter the body")
	}
}
