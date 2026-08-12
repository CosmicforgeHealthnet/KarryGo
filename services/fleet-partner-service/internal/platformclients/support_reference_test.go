package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
)

const supportTestSecret = "support-dispute-reference-client-test-secret"

// supportStub is a stand-in for support-dispute-service that verifies the
// request the way the real service does.
type supportStub struct {
	server     *httptest.Server
	lastPath   string
	lastCaller string
	verified   bool
	status     int
}

func newSupportStub(t *testing.T, status int) *supportStub {
	t.Helper()
	stub := &supportStub{status: status}
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		config.ServiceName: []byte(supportTestSecret),
	}, time.Minute)

	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The RAW request target, not the decoded path. A percent-encoded
		// identifier is a single path segment on the wire; reading the decoded
		// form would report a traversal that never left this process.
		stub.lastPath = r.RequestURI
		caller, err := verifier.VerifyRequest(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		stub.lastCaller = caller
		stub.verified = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(stub.status)
		_, _ = w.Write([]byte(`{"data":{"id":"cmp_1"}}`))
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newReferenceClient(stub *supportStub) *SupportDisputeReferenceClient {
	return NewSupportDisputeReferenceClient(Options{
		ServiceName: "support-dispute-service",
		BaseURL:     stub.server.URL,
		Secret:      []byte(supportTestSecret),
		Timeout:     5 * time.Second,
	})
}

// TestIncidentValidationIsHMACSignedAndAttributed proves the private call
// carries fleet's authenticated identity rather than an unauthenticated read.
func TestIncidentValidationIsHMACSignedAndAttributed(t *testing.T) {
	stub := newSupportStub(t, http.StatusOK)
	client := newReferenceClient(stub)

	check, err := client.ValidateIncident(context.Background(),
		CallMeta{CorrelationID: "corr-1", IdempotencyKey: "key-1"}, "cmp_1")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !check.Exists {
		t.Fatal("a 200 answer means the case exists")
	}
	if !stub.verified {
		t.Fatal("the request must be HMAC-verified by the authoritative service")
	}
	if stub.lastCaller != config.ServiceName {
		t.Fatalf("caller = %q, want %q", stub.lastCaller, config.ServiceName)
	}
	if !strings.HasSuffix(stub.lastPath, "/admin/complaints/cmp_1") {
		t.Fatalf("path = %q; the authoritative admin read is the only lookup", stub.lastPath)
	}
}

// TestIncidentValidationRejectsWrongSecret proves the allowlist on the
// authoritative side is what actually gates the call: a client holding the
// wrong secret is refused, and the refusal is an error rather than a verdict.
func TestIncidentValidationRejectsWrongSecret(t *testing.T) {
	stub := newSupportStub(t, http.StatusOK)
	client := NewSupportDisputeReferenceClient(Options{
		ServiceName: "support-dispute-service",
		BaseURL:     stub.server.URL,
		Secret:      []byte("the-wrong-secret-entirely"),
		Timeout:     5 * time.Second,
	})

	if _, err := client.ValidateIncident(context.Background(), CallMeta{}, "cmp_1"); err == nil {
		t.Fatal("a caller with the wrong secret must be refused")
	}
}

// TestIncidentValidationTreatsNotFoundAsDefinitiveNo proves a missing case is
// a clean verdict rather than an error the caller must interpret.
func TestIncidentValidationTreatsNotFoundAsDefinitiveNo(t *testing.T) {
	stub := newSupportStub(t, http.StatusNotFound)
	client := newReferenceClient(stub)

	check, err := client.ValidateIncident(context.Background(), CallMeta{}, "cmp_missing")
	if err != nil {
		t.Fatalf("a 404 must not be an error: %v", err)
	}
	if check.Exists {
		t.Fatal("a 404 means the case does not exist")
	}
}

// TestIncidentValidationSurfacesServerFailureAsError proves an outage is never
// mistaken for "the case does not exist".
//
// The distinction is the whole safety property: one answer refuses the write
// because the reference is wrong, the other refuses it because we do not know.
func TestIncidentValidationSurfacesServerFailureAsError(t *testing.T) {
	stub := newSupportStub(t, http.StatusServiceUnavailable)
	client := newReferenceClient(stub)

	if _, err := client.ValidateIncident(context.Background(), CallMeta{}, "cmp_1"); err == nil {
		t.Fatal("an unavailable authority must surface as an error, not as a verdict")
	}
}

// TestUnconfiguredClientRefusesRatherThanSkipping proves a deployment missing
// credentials fails closed.
func TestUnconfiguredClientRefusesRatherThanSkipping(t *testing.T) {
	client := NewSupportDisputeReferenceClient(Options{ServiceName: "support-dispute-service"})
	if client.Configured() {
		t.Fatal("a client with no base URL or secret is not configured")
	}
	_, err := client.ValidateIncident(context.Background(), CallMeta{}, "cmp_1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// TestDisputeValidationReportsAbsentEndpoint proves the structural gap is
// reported as its own condition, distinct from an outage.
//
// support-dispute-service exposes disputes only as a list and as a child of a
// complaint. Guessing a dispute's parent complaint would be fleet inventing a
// relationship it does not own, so the client says so plainly and the caller
// stores an explicitly unverified reference.
func TestDisputeValidationReportsAbsentEndpoint(t *testing.T) {
	stub := newSupportStub(t, http.StatusOK)
	client := newReferenceClient(stub)

	_, err := client.ValidateDispute(context.Background(), CallMeta{}, "dsp_1")
	if !errors.Is(err, ErrValidationEndpointAbsent) {
		t.Fatalf("err = %v, want ErrValidationEndpointAbsent", err)
	}
	if !errors.Is(err, ErrValidationEndpointAbsent) || errors.Is(err, ErrNotConfigured) {
		t.Fatal("an absent endpoint must not be confused with a missing credential")
	}
}

// TestReferenceIdentifiersArePathEscaped proves a hostile identifier cannot
// reach outside the intended path.
func TestReferenceIdentifiersArePathEscaped(t *testing.T) {
	stub := newSupportStub(t, http.StatusOK)
	client := newReferenceClient(stub)

	if _, err := client.ValidateIncident(context.Background(), CallMeta{},
		"../../admin/disputes"); err != nil {
		// A rejection is acceptable; a traversal is not.
		t.Logf("validate returned %v", err)
	}
	// On the wire the identifier is one percent-encoded segment, so the
	// request target still addresses the complaints collection.
	if !strings.HasPrefix(stub.lastPath, "/api/v1/support-disputes/admin/complaints/") {
		t.Fatalf("request target left the complaints path: %q", stub.lastPath)
	}
	if strings.Contains(stub.lastPath, "/admin/complaints/../") {
		t.Fatalf("path traversal reached %q", stub.lastPath)
	}
}
