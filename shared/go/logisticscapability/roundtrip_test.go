package logisticscapability

// Client/server round trip.
//
// The matcher tests prove the rules; this proves the two halves of the
// contract agree about the wire. A path constant that drifts, an envelope the
// client cannot decode, or a signature the server rejects would all leave
// Vendor unable to ask an authority anything -- and, because the gate treats
// an unreachable authority as "no refusal", that failure would be invisible
// until a mission failed in the field.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	roundTripCaller = "vendor-service"
	roundTripSecret = "round-trip-secret"
	roundTripBase   = "/api/v1/hauling"
)

func roundTripCapability() Capability {
	return Capability{
		Authority: AuthorityHauling, Version: 7, Available: true,
		MultiPickupSupported: true, MaxPickups: 10,
		MultiStopSupported: false, MaxDropoffs: 1,
		SecurePickupSupported: true, SecureDeliverySupported: true,
		CustodyEvidenceSupported: true, CorporateProviderSupported: true,
		VehicleClasses: []string{"VAN", "TRUCK"},
		MaxWeightGrams: 20_000_000, MaxPackageCount: 500,
		HandlingSupported: []string{"FRAGILE", "UPRIGHT"},
		ObservedAt:        time.Unix(1_700_000_000, 0).UTC(),
	}
}

// newAuthorityServer mounts the routes behind an HMAC boundary shaped like the
// real services', so the test exercises signing rather than assuming it.
func newAuthorityServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{roundTripCaller: []byte(roundTripSecret)}, 5*time.Minute,
	)
	group := engine.Group(roundTripBase)
	group.Use(serviceauth.Middleware(verifier))
	RegisterRoutes(group, ProviderFunc(roundTripCapability))
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

func newRoundTripClient(baseURL string) *Client {
	return New(Options{
		BaseURL: baseURL, APIBase: roundTripBase,
		Caller: roundTripCaller, Secret: []byte(roundTripSecret),
		Authority: AuthorityHauling, Timeout: 5 * time.Second,
	})
}

func TestCapabilityRoundTripsOverTheRealBoundary(t *testing.T) {
	client := newRoundTripClient(newAuthorityServer(t).URL)

	capability, err := client.Capability(context.Background())
	if err != nil {
		t.Fatalf("capability read failed: %v", err)
	}
	want := roundTripCapability()
	if capability.Authority != want.Authority || capability.Version != want.Version {
		t.Fatalf("identity did not survive the wire: %+v", capability)
	}
	if !capability.Available || capability.MaxPickups != want.MaxPickups || !capability.MultiPickupSupported {
		t.Fatalf("capability lost its declared limits: %+v", capability)
	}
	if len(capability.VehicleClasses) != len(want.VehicleClasses) {
		t.Fatalf("vehicle classes lost: %+v", capability.VehicleClasses)
	}
	if !capability.ObservedAt.Equal(want.ObservedAt) {
		t.Fatalf("observation time did not survive: %v", capability.ObservedAt)
	}
}

func TestServiceabilityRoundTripsAndRefusesWithTheAuthorityReason(t *testing.T) {
	client := newRoundTripClient(newAuthorityServer(t).URL)

	within, err := client.Serviceability(context.Background(), Requirement{
		PickupCount: 4, DropoffCount: 1, WeightGrams: 500_000, PackageCount: 20,
		VehicleClass: "TRUCK", SecurePickup: true, SecureDelivery: true,
	})
	if err != nil {
		t.Fatalf("serviceability read failed: %v", err)
	}
	if !within.Serviceable || within.ReasonCode != ReasonServiceable {
		t.Fatalf("a request inside every declared limit must be serviceable: %+v", within)
	}
	if within.Authority != AuthorityHauling {
		t.Fatalf("the answer must name its authority, got %q", within.Authority)
	}

	beyond, err := client.Serviceability(context.Background(), Requirement{
		PickupCount: 40, DropoffCount: 1,
	})
	if err != nil {
		t.Fatalf("serviceability read failed: %v", err)
	}
	if beyond.Serviceable {
		t.Fatal("forty pickups against a declared maximum of ten must not be serviceable")
	}
	if beyond.ReasonCode != ReasonTooManyPickups {
		t.Fatalf("reason=%q, want the authority's own %q", beyond.ReasonCode, ReasonTooManyPickups)
	}
}

func TestAnUnsignedCallerCannotReadCapability(t *testing.T) {
	server := newAuthorityServer(t)

	response, err := http.Get(server.URL + roundTripBase + Path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		t.Fatal("capability is internal contract detail and must not be readable unsigned")
	}
}

func TestAWrongSecretCannotReadCapability(t *testing.T) {
	server := newAuthorityServer(t)
	client := New(Options{
		BaseURL: server.URL, APIBase: roundTripBase,
		Caller: roundTripCaller, Secret: []byte("not-the-secret"),
		Authority: AuthorityHauling, Timeout: 5 * time.Second,
	})
	if _, err := client.Capability(context.Background()); err == nil {
		t.Fatal("a wrongly signed capability read must fail")
	}
}

func TestAnUnconfiguredClientRefusesRatherThanGuessing(t *testing.T) {
	client := New(Options{Authority: AuthorityHauling})
	if client.Configured() {
		t.Fatal("a client with no base URL or secret must report itself unconfigured")
	}
	if _, err := client.Capability(context.Background()); err == nil {
		t.Fatal("an unconfigured client must refuse the call rather than answer from nothing")
	}
}
