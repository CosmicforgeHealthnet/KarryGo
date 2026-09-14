package fleetproviderwork

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/providercompat"
)

type stubCorporate struct {
	ctx providercompat.CorporateContext
	err error
}

func (s stubCorporate) ReadCorporateContext(context.Context, string, string) (providercompat.CorporateContext, error) {
	return s.ctx, s.err
}

func managedContext(mode string) providercompat.CorporateContext {
	return providercompat.CorporateContext{
		ProviderID: "prov-1", IsManagedProvider: true, MembershipID: "mem-1",
		OrganizationID: "org-1", MembershipStatus: "ACTIVE", OperatingMode: mode,
		ServiceEligibility: map[string]bool{"HAULING": true},
	}
}

// fleetStub answers the Fleet routes the facade calls.
func fleetStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/assignments"):
			_, _ = w.Write([]byte(`{"data":{"items":[{"assignmentId":"asg-1","status":"PENDING"}]},"meta":{"nextCursor":"c2"}}`))
		case strings.HasSuffix(r.URL.Path, "/offers"):
			_, _ = w.Write([]byte(`{"data":{"items":[{"offerId":"off-1","status":"OFFERED"}]},"meta":{}}`))
		case strings.Contains(r.URL.Path, "provider-accept"), strings.Contains(r.URL.Path, "provider-decline"):
			_, _ = w.Write([]byte(`{"data":{"assignmentId":"asg-1","status":"ACCEPTED"}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"assignmentId":"asg-1","status":"PENDING"}}`))
		}
	}))
}

func routerFor(t *testing.T, fleetURL string, corporate providercompat.CorporateContextReader, enabled bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client, ok := NewClient(Config{BaseURL: fleetURL, ServiceName: "driver-hauling-service",
		Secret: []byte("secret"), ProviderService: "HAULING"})
	if !ok && fleetURL != "" {
		t.Fatal("client not configured")
	}
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Set(sharedauth.ClaimsContextKey, sharedauth.Claims{
			Subject: "prov-1", Role: "truck_provider", Service: "hauling", SessionID: "sess-1"})
		c.Next()
	})
	group := router.Group("/provider")
	RegisterRoutes(group, NewHandler(HandlerConfig{
		Client: client, Corporate: corporate, Service: "HAULING", Enabled: enabled}))
	return router
}

func do(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// HLG-005 / HLG-009: the LIST reads exist and return the provider's corporate
// work, which is what makes reconnect recovery possible.
func TestListsRecoverCorporateWork(t *testing.T) {
	server := fleetStub(t)
	defer server.Close()
	router := routerFor(t, server.URL, stubCorporate{ctx: managedContext(ModeHybrid)}, true)

	for path, key := range map[string]string{
		"/provider/company-assignments": "assignments",
		"/provider/organization-offers": "offers",
	} {
		recorder := do(t, router, http.MethodGet, path, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", path, recorder.Code, recorder.Body)
		}
		var out struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s body: %v", path, err)
		}
		if _, ok := out.Data[key]; !ok {
			t.Fatalf("%s response is missing %q: %s", path, key, recorder.Body)
		}
	}
}

// HLG-003/HLG-004: every corporate route fails closed. A Fleet outage is a
// retryable 503; an inactive membership or an ineligible service is a
// definite 403. No route proceeds as independent.
func TestCorporateRoutesFailClosed(t *testing.T) {
	server := fleetStub(t)
	defer server.Close()

	inactive := managedContext(ModeHybrid)
	inactive.MembershipStatus = "SUSPENDED"

	ineligible := managedContext(ModeHybrid)
	ineligible.ServiceEligibility = map[string]bool{"HAULING": false}

	independent := providercompat.CorporateContext{ProviderID: "prov-1"}

	cases := []struct {
		name     string
		reader   providercompat.CorporateContextReader
		wantCode int
	}{
		{"fleet outage is retryable", stubCorporate{err: errors.New("connection refused")}, http.StatusServiceUnavailable},
		{"suspended membership is rejected", stubCorporate{ctx: inactive}, http.StatusForbidden},
		{"HAULING ineligible is rejected", stubCorporate{ctx: ineligible}, http.StatusForbidden},
		{"independent provider is rejected", stubCorporate{ctx: independent}, http.StatusForbidden},
		{"no Fleet integration is retryable", nil, http.StatusServiceUnavailable},
	}
	paths := []string{
		"/provider/company-assignments",
		"/provider/organization-offers",
		"/provider/company-assignments/asg-1",
		"/provider/organization-offers/off-1",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := routerFor(t, server.URL, tc.reader, true)
			for _, path := range paths {
				if got := do(t, router, http.MethodGet, path, "").Code; got != tc.wantCode {
					t.Fatalf("%s status = %d, want %d", path, got, tc.wantCode)
				}
			}
			// Writes must fail closed too, and must never reach Fleet.
			for _, path := range []string{
				"/provider/company-assignments/asg-1/accept",
				"/provider/organization-offers/off-1/accept",
			} {
				got := do(t, router, http.MethodPost, path, `{"clientResponseId":"r1"}`).Code
				if got != tc.wantCode {
					t.Fatalf("%s status = %d, want %d", path, got, tc.wantCode)
				}
			}
		})
	}
}

// HLG-011 / HLG-013: Fleet's PERSISTED operating mode decides whether a
// provider may take an offer directly. Nothing here defaults or hardcodes it.
func TestOperatingModeGovernsDirectOfferAcceptance(t *testing.T) {
	server := fleetStub(t)
	defer server.Close()

	cases := map[string]int{
		ModeDelegated:   http.StatusOK,
		ModeHybrid:      http.StatusOK,
		ModeCentralized: http.StatusForbidden,
		"":              http.StatusForbidden, // unestablished mode is not a licence
	}
	for mode, want := range cases {
		t.Run("mode="+mode, func(t *testing.T) {
			router := routerFor(t, server.URL, stubCorporate{ctx: managedContext(mode)}, true)
			got := do(t, router, http.MethodPost,
				"/provider/organization-offers/off-1/accept", `{"clientResponseId":"r1"}`).Code
			if got != want {
				t.Fatalf("offer accept under %q = %d, want %d", mode, got, want)
			}

			// HLG-008/HLG-012: declining is ALWAYS permitted. A mode that
			// restricts accepting must not trap a provider in an offer they
			// cannot refuse.
			if got := do(t, router, http.MethodPost,
				"/provider/organization-offers/off-1/decline", `{"clientResponseId":"r2"}`).Code; got != http.StatusOK {
				t.Fatalf("offer decline under %q = %d, want 200", mode, got)
			}

			// HLG-007: company assignments are the organization's own
			// dispatch, so CENTRALIZED does not restrict them.
			if got := do(t, router, http.MethodPost,
				"/provider/company-assignments/asg-1/accept", `{"clientResponseId":"r3"}`).Code; got != http.StatusOK {
				t.Fatalf("assignment accept under %q = %d, want 200", mode, got)
			}
		})
	}
}

// A decision without the provider's own idempotency identity is refused
// before anything reaches Fleet.
func TestDecisionRequiresClientResponseID(t *testing.T) {
	server := fleetStub(t)
	defer server.Close()
	router := routerFor(t, server.URL, stubCorporate{ctx: managedContext(ModeHybrid)}, true)

	if got := do(t, router, http.MethodPost,
		"/provider/company-assignments/asg-1/accept", `{}`).Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got)
	}
}

// With the integration switched off every route refuses, so a deployment that
// has not certified the chain cannot serve corporate work (HLG-033).
func TestDisabledIntegrationRefusesEveryRoute(t *testing.T) {
	server := fleetStub(t)
	defer server.Close()
	router := routerFor(t, server.URL, stubCorporate{ctx: managedContext(ModeHybrid)}, false)

	for _, path := range []string{"/provider/company-assignments", "/provider/organization-offers"} {
		if got := do(t, router, http.MethodGet, path, "").Code; got != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503 while disabled", path, got)
		}
	}
}

// The classification mapping a provider actually sees.
func TestAsAPIErrorStatusMapping(t *testing.T) {
	cases := map[providercompat.FailureKind]int{
		providercompat.FailureDependency:           http.StatusServiceUnavailable,
		providercompat.FailureNotManaged:           http.StatusForbidden,
		providercompat.FailureMembershipInvalid:    http.StatusForbidden,
		providercompat.FailureServiceIneligible:    http.StatusForbidden,
		providercompat.FailureProviderMismatch:     http.StatusForbidden,
		providercompat.FailureOrganizationMismatch: http.StatusForbidden,
		providercompat.FailureOperatingMode:        http.StatusForbidden,
	}
	for kind, want := range cases {
		got := apperrors.From(AsAPIError(providercompat.NewContextError(kind, "HAULING", "", nil))).Status
		if got != want {
			t.Fatalf("%s -> %d, want %d", kind, got, want)
		}
	}
	// An unclassified error degrades to a dependency failure, never to a
	// rejection: an unknown cause cannot authorize denying a provider work.
	if got := apperrors.From(AsAPIError(errors.New("boom"))).Status; got != http.StatusServiceUnavailable {
		t.Fatalf("unclassified -> %d, want 503", got)
	}
}

// HLG-003: a Fleet 5xx is retryable, a Fleet 403 is terminal.
func TestClassifyResponse(t *testing.T) {
	cases := []struct {
		status    int
		body      string
		kind      providercompat.FailureKind
		retryable bool
	}{
		{http.StatusBadGateway, ``, providercompat.FailureDependency, true},
		{http.StatusTooManyRequests, ``, providercompat.FailureDependency, true},
		{http.StatusUnauthorized, ``, providercompat.FailureDependency, true},
		{http.StatusBadRequest, `{"error":{"code":"busy","retryable":true}}`, providercompat.FailureDependency, true},
		{http.StatusForbidden, `{"error":{"code":"forbidden"}}`, providercompat.FailureProviderMismatch, false},
		{http.StatusNotFound, ``, providercompat.FailureProviderMismatch, false},
		{http.StatusConflict, ``, providercompat.FailureOperatingMode, false},
		{http.StatusUnprocessableEntity, ``, providercompat.FailureServiceIneligible, false},
	}
	for _, tc := range cases {
		got := ClassifyResponse(tc.status, []byte(tc.body))
		if got.Kind != tc.kind {
			t.Fatalf("status %d kind = %q, want %q", tc.status, got.Kind, tc.kind)
		}
		if got.Retryable() != tc.retryable {
			t.Fatalf("status %d retryable = %v, want %v", tc.status, got.Retryable(), tc.retryable)
		}
	}
}
