package providercompat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type stubReader struct {
	ctx CorporateContext
	err error
}

func (s stubReader) ReadCorporateContext(context.Context, string, string) (CorporateContext, error) {
	return s.ctx, s.err
}

func managedContext() CorporateContext {
	return CorporateContext{
		ProviderID: "prov-1", IsManagedProvider: true, MembershipID: "mem-1",
		OrganizationID: "org-1", MembershipStatus: "ACTIVE",
		ServiceEligibility: map[string]bool{"TAXI": true},
		Version:            3,
	}
}

// A Fleet outage must never be reported as "independent": it is a retryable
// dependency failure, which is the whole fail-closed requirement.
func TestAuthorizeManagedOperation_DependencyFailureIsRetryable(t *testing.T) {
	reader := stubReader{err: errors.New("dial tcp: connection refused")}
	_, err := AuthorizeManagedOperation(context.Background(), reader, ManagedRequest{
		Service: "TAXI", ProviderID: "prov-1", RequireManaged: true,
	})
	kind, ok := KindOf(err)
	if !ok || kind != FailureDependency {
		t.Fatalf("want FailureDependency, got kind=%q ok=%v err=%v", kind, ok, err)
	}
	if !kind.Retryable() {
		t.Fatal("dependency failure must be retryable")
	}
}

func TestAuthorizeManagedOperation_ClassifiesRejections(t *testing.T) {
	base := managedContext()

	inactive := base
	inactive.MembershipStatus = "SUSPENDED"

	ineligible := base
	ineligible.ServiceEligibility = map[string]bool{"TAXI": false}

	independent := CorporateContext{ProviderID: "prov-1", ServiceEligibility: map[string]bool{}}

	incomplete := base
	incomplete.MembershipID = ""

	cases := []struct {
		name string
		ctx  CorporateContext
		req  ManagedRequest
		want FailureKind
	}{
		{"inactive membership", inactive, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", RequireManaged: true}, FailureMembershipInvalid},
		{"service ineligible", ineligible, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", RequireManaged: true}, FailureServiceIneligible},
		{"not managed", independent, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", RequireManaged: true}, FailureNotManaged},
		{"wrong provider", base, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", ExpectedProviderID: "prov-2", RequireManaged: true}, FailureProviderMismatch},
		{"wrong organization", base, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", ExpectedOrganizationID: "org-9", RequireManaged: true}, FailureOrganizationMismatch},
		{"incomplete context", incomplete, ManagedRequest{Service: "TAXI", ProviderID: "prov-1", RequireManaged: true}, FailureDependency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AuthorizeManagedOperation(context.Background(), stubReader{ctx: tc.ctx}, tc.req)
			kind, ok := KindOf(err)
			if !ok || kind != tc.want {
				t.Fatalf("want %q, got kind=%q ok=%v err=%v", tc.want, kind, ok, err)
			}
			if tc.want != FailureDependency && kind.Retryable() {
				t.Fatalf("%q must not be retryable", tc.want)
			}
		})
	}
}

func TestAuthorizeManagedOperation_HappyPath(t *testing.T) {
	got, err := AuthorizeManagedOperation(context.Background(), stubReader{ctx: managedContext()}, ManagedRequest{
		Service: "TAXI", ProviderID: "prov-1",
		ExpectedProviderID: "prov-1", ExpectedOrganizationID: "org-1", RequireManaged: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MembershipID != "mem-1" || got.OrganizationID != "org-1" {
		t.Fatalf("unexpected context: %+v", got)
	}
}

// An operation that permits both modes must return the independent context
// without error, so independent providers keep working unchanged.
func TestAuthorizeManagedOperation_IndependentAllowedWhenNotRequired(t *testing.T) {
	got, err := AuthorizeManagedOperation(context.Background(),
		stubReader{ctx: CorporateContext{ProviderID: "prov-1"}},
		ManagedRequest{Service: "TAXI", ProviderID: "prov-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsManagedProvider {
		t.Fatal("expected independent context")
	}
}

// No reader configured plus a managed-only operation must fail closed.
func TestAuthorizeManagedOperation_NoReaderFailsClosedForManagedOnly(t *testing.T) {
	_, err := AuthorizeManagedOperation(context.Background(), nil, ManagedRequest{
		Service: "TAXI", ProviderID: "prov-1", RequireManaged: true,
	})
	if kind, ok := KindOf(err); !ok || kind != FailureDependency {
		t.Fatalf("want FailureDependency, got %q (%v)", kind, err)
	}
}

func TestCompletedWorkRequestValidate(t *testing.T) {
	valid := CompletedWorkRequest{
		SourceService: SourceServiceTaxi, SourceJobID: "job-1", SourceAssignmentID: "asg-1",
		ProviderID: "prov-1", MembershipID: "mem-1", OrganizationID: "org-1",
		CompletedAt: time.Now().UTC(), Currency: "NGN", EligibleBasisMinor: 150000,
		SourceVersion: 2, IdempotencyKey: "key-1",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	notUTC := valid
	notUTC.CompletedAt = time.Now().In(time.FixedZone("WAT", 3600))
	if err := notUTC.Validate(); err == nil {
		t.Fatal("non-UTC completedAt must be rejected")
	}
	badSource := valid
	badSource.SourceService = "FLEET"
	if err := badSource.Validate(); err == nil {
		t.Fatal("FLEET is not a valid execution source")
	}
	negative := valid
	negative.EligibleBasisMinor = -1
	if err := negative.Validate(); err == nil {
		t.Fatal("negative basis must be rejected")
	}
}

func TestSubmitCompletedWork_Classification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   SubmitOutcome
	}{
		{"created", http.StatusCreated, `{"data":{"completedWork":{"ID":"cw-1","IngestionStatus":"RECORDED"}}}`, SubmitAccepted},
		{"conflict is duplicate", http.StatusConflict, `{"error":{"code":"conflict"}}`, SubmitDuplicate},
		{"server error retryable", http.StatusBadGateway, ``, SubmitRetryable},
		{"unavailable retryable", http.StatusServiceUnavailable, `{"error":{"code":"unavailable","retryable":true}}`, SubmitRetryable},
		{"forbidden terminal", http.StatusForbidden, `{"error":{"code":"forbidden"}}`, SubmitTerminal},
		{"validation terminal", http.StatusBadRequest, `{"error":{"code":"validation_failed"}}`, SubmitTerminal},
		{"retryable 4xx honoured", http.StatusBadRequest, `{"error":{"code":"projection_disabled","retryable":true}}`, SubmitRetryable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != CompletedWorkPath {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				if r.Header.Get("Idempotency-Key") != "key-1" {
					t.Errorf("idempotency key not forwarded")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := NewClient(server.URL, "driver-taxi-service", []byte("secret"), time.Second)
			result, _ := client.SubmitCompletedWork(context.Background(), CompletedWorkRequest{
				SourceService: SourceServiceTaxi, SourceJobID: "job-1", SourceAssignmentID: "asg-1",
				ProviderID: "prov-1", MembershipID: "mem-1", OrganizationID: "org-1",
				CompletedAt: time.Now().UTC(), Currency: "NGN", EligibleBasisMinor: 1000,
				SourceVersion: 1, IdempotencyKey: "key-1",
			})
			if result.Outcome != tc.want {
				t.Fatalf("want %q, got %q (code=%s)", tc.want, result.Outcome, result.Code)
			}
		})
	}
}

// A transport failure must be UNKNOWN, never TERMINAL: Fleet may have
// committed before the connection broke, so the retry has to reuse the same
// idempotency key rather than mint a new completion event.
func TestSubmitCompletedWork_TransportFailureIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()
	client := NewClient(url, "driver-taxi-service", []byte("secret"), 200*time.Millisecond)
	result, err := client.SubmitCompletedWork(context.Background(), CompletedWorkRequest{
		SourceService: SourceServiceDispatch, SourceJobID: "job-1", SourceAssignmentID: "asg-1",
		ProviderID: "prov-1", MembershipID: "mem-1", OrganizationID: "org-1",
		CompletedAt: time.Now().UTC(), Currency: "NGN", EligibleBasisMinor: 1,
		SourceVersion: 1, IdempotencyKey: "key-1",
	})
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if result.Outcome != SubmitUnknown {
		t.Fatalf("want UNKNOWN, got %q", result.Outcome)
	}
	if !result.Outcome.Retryable() {
		t.Fatal("UNKNOWN must be retryable so the same key is redelivered")
	}
}

func TestFieldVisible_TypedClassification(t *testing.T) {
	commissionOnly := FinancialVisibilityPolicy{ShowOwnEarning: true, ShowCommissionRate: true, ShowProviderShare: true, ShowPayroll: true}

	// Organization and platform classes are concealed even when every policy
	// flag is set: no policy may authorize them in Sections 8/9.
	everything := IndependentVisibilityPolicy()
	for _, field := range []string{"organization_share_minor", "platformMarginMinor", "settlement_split", "organization_gross"} {
		if visible, classified := FieldVisible(field, true, &everything); visible || !classified {
			t.Fatalf("%s must be classified and concealed (visible=%v classified=%v)", field, visible, classified)
		}
	}

	// A managed provider on a commission policy sees their own earning but
	// not the customer fare.
	if visible, _ := FieldVisible("own_earning_minor", true, &commissionOnly); !visible {
		t.Fatal("own earning must be visible under a commission policy")
	}
	if visible, _ := FieldVisible("customerFareMinor", true, &commissionOnly); visible {
		t.Fatal("customer fare must be concealed under a commission policy")
	}

	// An independent provider loses nothing.
	for _, field := range []string{"fare_amount", "customerFareMinor", "earnings_kobo", "tips_kobo", "commission_rate_bps"} {
		if visible, _ := FieldVisible(field, false, nil); !visible {
			t.Fatalf("independent provider must still see %s", field)
		}
	}

	// A managed provider with no policy at all falls back to conservative.
	if visible, _ := FieldVisible("customerFareMinor", true, nil); visible {
		t.Fatal("missing policy must fail closed")
	}

	// Unclassified fields report classified=false so callers keep their own
	// default-deny heuristic in place.
	if _, classified := FieldVisible("pickup_address", true, &everything); classified {
		t.Fatal("non-financial field must not be classified")
	}
}
