package fleetprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"
)

// TAX-003: a Fleet outage must produce a RETRYABLE dependency error, never a
// rejection and never an independent-provider downgrade.
func TestClassifyResponse_DependencyVsRejection(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		want      providercompat.FailureKind
		retryable bool
	}{
		{"gateway timeout", http.StatusGatewayTimeout, ``, providercompat.FailureDependency, true},
		{"internal error", http.StatusInternalServerError, ``, providercompat.FailureDependency, true},
		{"rate limited", http.StatusTooManyRequests, ``, providercompat.FailureDependency, true},
		{"our credentials wrong", http.StatusUnauthorized, ``, providercompat.FailureDependency, true},
		{"explicitly retryable", http.StatusBadRequest, `{"error":{"code":"busy","retryable":true}}`, providercompat.FailureDependency, true},
		{"forbidden", http.StatusForbidden, `{"error":{"code":"forbidden"}}`, providercompat.FailureProviderMismatch, false},
		{"not found", http.StatusNotFound, ``, providercompat.FailureProviderMismatch, false},
		{"conflict", http.StatusConflict, ``, providercompat.FailureOperatingMode, false},
		{"unprocessable", http.StatusUnprocessableEntity, ``, providercompat.FailureServiceIneligible, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyResponse(tc.status, []byte(tc.body))
			if got.Kind != tc.want {
				t.Fatalf("kind = %q, want %q", got.Kind, tc.want)
			}
			if got.Retryable() != tc.retryable {
				t.Fatalf("retryable = %v, want %v", got.Retryable(), tc.retryable)
			}
		})
	}
}

// The API mapping is what a provider actually sees: a dependency failure is a
// 503 they should retry, every authoritative rejection is a definite 403.
func TestAsAPIError_StatusMapping(t *testing.T) {
	cases := map[providercompat.FailureKind]int{
		providercompat.FailureDependency:           http.StatusServiceUnavailable,
		providercompat.FailureNotManaged:           http.StatusForbidden,
		providercompat.FailureMembershipInvalid:    http.StatusForbidden,
		providercompat.FailureServiceIneligible:    http.StatusForbidden,
		providercompat.FailureProviderMismatch:     http.StatusForbidden,
		providercompat.FailureOrganizationMismatch: http.StatusForbidden,
		providercompat.FailureOperatingMode:        http.StatusForbidden,
	}
	for kind, wantStatus := range cases {
		err := AsAPIError(providercompat.NewContextError(kind, "TAXI", "", nil))
		appErr := apperrors.From(err)
		if appErr.Status != wantStatus {
			t.Fatalf("%s -> status %d, want %d", kind, appErr.Status, wantStatus)
		}
	}
	// An unclassified error must degrade to a dependency failure, never to a
	// rejection: an unknown cause cannot authorize denying a provider work.
	appErr := apperrors.From(AsAPIError(errors.New("something unexpected")))
	if appErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("unclassified error -> status %d, want 503", appErr.Status)
	}
}

// A Fleet 5xx on a list read must surface as a retryable dependency error
// through the real client, not as an empty success.
func TestClientGet_ServerErrorIsRetryableDependency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	client, ok := NewClient(Config{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: []byte("s")})
	if !ok {
		t.Fatal("client not configured")
	}
	_, err := client.ListOffers(context.Background(), Meta{}, "prov-1")
	if err == nil {
		t.Fatal("expected an error")
	}
	var fleetErr *FleetError
	if !errors.As(err, &fleetErr) {
		t.Fatalf("error type = %T, want *FleetError", err)
	}
	if !fleetErr.Retryable() {
		t.Fatal("a Fleet 5xx must be retryable")
	}
}

// A Fleet 403 on an accept must be a definite rejection: retrying it would
// hammer Fleet with a decision it has already refused.
func TestClientDecision_ForbiddenIsTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden"}}`))
	}))
	defer server.Close()
	client, _ := NewClient(Config{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: []byte("s")})
	_, err := client.AcceptAssignment(context.Background(), Meta{}, "prov-1", "sess-1", "asg-1",
		Decision{ClientResponseID: "resp-1"})
	var fleetErr *FleetError
	if !errors.As(err, &fleetErr) {
		t.Fatalf("error type = %T, want *FleetError", err)
	}
	if fleetErr.Retryable() {
		t.Fatal("a Fleet 403 must not be retryable")
	}
	if apperrors.From(AsAPIError(err)).Status != http.StatusForbidden {
		t.Fatal("a Fleet 403 must surface to the provider as 403")
	}
}

// TAX-004: the accept path asserts the provider's membership covers TAXI.
// Fleet membership alone is not sufficient authority.
func TestAuthorizeManaged_RequiresTaxiEligibility(t *testing.T) {
	reader := stubCorporate{ctx: providercompat.CorporateContext{
		ProviderID: "prov-1", IsManagedProvider: true, MembershipID: "mem-1",
		OrganizationID: "org-1", MembershipStatus: "ACTIVE",
		// Membership is live but covers DISPATCH, not TAXI.
		ServiceEligibility: map[string]bool{"DISPATCH": true, "TAXI": false},
	}}
	_, err := providercompat.AuthorizeManagedOperation(context.Background(), reader,
		providercompat.ManagedRequest{Service: ServiceTaxi, ProviderID: "prov-1", RequireManaged: true})
	kind, ok := providercompat.KindOf(err)
	if !ok || kind != providercompat.FailureServiceIneligible {
		t.Fatalf("kind = %q ok=%v, want SERVICE_NOT_ELIGIBLE", kind, ok)
	}
	if apperrors.From(AsAPIError(err)).Status != http.StatusForbidden {
		t.Fatal("an ineligible service must be a definite rejection")
	}
}

type stubCorporate struct {
	ctx providercompat.CorporateContext
	err error
}

func (s stubCorporate) ReadCorporateContext(context.Context, string, string) (providercompat.CorporateContext, error) {
	return s.ctx, s.err
}
