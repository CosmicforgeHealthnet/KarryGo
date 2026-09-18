package truckcapability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// stubNearby is a recording availability index.
type stubNearby struct {
	providers []NearbyProvider
	err       error
	calls     int
}

func (s *stubNearby) NearbyProviders(_ context.Context, _ geo.Point, _ float64, _ int) ([]NearbyProvider, error) {
	s.calls++
	return s.providers, s.err
}

// stubFits stands in for the capability store, recording every call so a test
// can prove the preview only ever READ.
type stubFits struct {
	trucks map[string][]cargofit.TruckCapability
	err    error
	loads  int
}

func (s *stubFits) load(_ context.Context, _ []string, _ cargofit.Requirement) (map[string][]cargofit.TruckCapability, error) {
	s.loads++
	return s.trucks, s.err
}

func fittingTruck(id string) cargofit.TruckCapability {
	return cargofit.TruckCapability{
		TruckID: id, TruckType: "box", Active: true, Verified: true,
		MaxPayloadGrams: 3_000_000, MaxPayloadGramsKnown: true,
		CapabilityVersion: 4,
	}
}

// previewerWith builds a Previewer whose capability load is stubbed, so the
// test never needs a database.
func previewerWith(enabled bool, nearby NearbyProviders, fits *stubFits) *Previewer {
	previewer := NewPreviewer(PreviewOptions{
		Service: &Service{now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }},
		Nearby:  nearby,
		Enabled: enabled,
		Now:     func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	previewer.loadFits = fits.load
	return previewer
}

func previewContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", supplypreview.Path, nil)
	return c
}

func lagosRequest() supplypreview.Request {
	return supplypreview.Request{
		PickupLatitude: 6.5244, PickupLongitude: 3.3792, WeightGrams: 1_000_000,
	}
}

// The central guarantee: a preview reads and nothing else. The stubs record
// every call the previewer is capable of making, and only reads appear.
func TestPreviewCreatesNothing(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{
		{ProviderID: "p1", DistanceKm: 2}, {ProviderID: "p2", DistanceKm: 4},
		{ProviderID: "p3", DistanceKm: 6}, {ProviderID: "p4", DistanceKm: 9},
	}}
	fits := &stubFits{trucks: map[string][]cargofit.TruckCapability{
		"p1": {fittingTruck("t1")}, "p2": {fittingTruck("t2")},
		"p3": {fittingTruck("t3")}, "p4": {fittingTruck("t4")},
	}}

	response, err := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !response.Serviceable {
		t.Fatalf("expected serviceable, got %s / %s", response.SupplyStatus, response.ReasonCode)
	}
	// Exactly one index read and one capability read. A booking, an offer or a
	// lock would need a write path the Previewer simply does not hold: it has
	// no repository writer, no availability mutator and no booking creator.
	if nearby.calls != 1 || fits.loads != 1 {
		t.Fatalf("unexpected call pattern: nearby=%d fits=%d", nearby.calls, fits.loads)
	}
}

// The response must never let a caller learn which provider or truck would
// have taken the job -- that would make the preview an assignment.
func TestPreviewLeaksNoProviderOrTruckIdentity(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "provider-secret", DistanceKm: 3}}}
	fits := &stubFits{trucks: map[string][]cargofit.TruckCapability{
		"provider-secret": {fittingTruck("truck-secret")},
	}}

	response, _ := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)
	for _, secret := range []string{"provider-secret", "truck-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("preview response leaked %q: %s", secret, body)
		}
	}
}

// An availability index that did not answer is UNKNOWN, never "no trucks". A
// planner that reads the second stops routing to Hauling altogether.
func TestPreviewDependencyFailureIsUnknownNotRefusal(t *testing.T) {
	nearby := &stubNearby{err: errors.New("redis unavailable")}
	response, _ := previewerWith(true, nearby, &stubFits{}).PreviewSupply(previewContext(), lagosRequest())

	if response.SupplyStatus != supplypreview.StatusUnknown {
		t.Fatalf("status = %s, want UNKNOWN", response.SupplyStatus)
	}
	if response.ReasonCode != supplypreview.ReasonDependencyUnavailable {
		t.Fatalf("reason = %s, want %s", response.ReasonCode, supplypreview.ReasonDependencyUnavailable)
	}
}

func TestPreviewCapabilityStoreFailureIsUnknown(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 2}}}
	fits := &stubFits{err: errors.New("postgres unavailable")}
	response, _ := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())

	if response.ReasonCode != supplypreview.ReasonDependencyUnavailable {
		t.Fatalf("reason = %s, want %s", response.ReasonCode, supplypreview.ReasonDependencyUnavailable)
	}
}

// "No candidates at all" and "candidates that all failed fit" are different
// problems: the first is about the city, the second about the load.
func TestPreviewDistinguishesNoSupplyFromNoFit(t *testing.T) {
	empty := &stubNearby{}
	noSupply, _ := previewerWith(true, empty, &stubFits{}).PreviewSupply(previewContext(), lagosRequest())
	if noSupply.ReasonCode != supplypreview.ReasonSupplyUnavailable {
		t.Fatalf("no-supply reason = %s, want %s", noSupply.ReasonCode, supplypreview.ReasonSupplyUnavailable)
	}

	tooSmall := fittingTruck("t1")
	tooSmall.MaxPayloadGrams = 100
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 2}}}
	fits := &stubFits{trucks: map[string][]cargofit.TruckCapability{"p1": {tooSmall}}}
	noFit, _ := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())

	if noFit.Serviceable {
		t.Fatal("a load nothing can carry was reported serviceable")
	}
	if noFit.ReasonCode == supplypreview.ReasonSupplyUnavailable {
		t.Fatal("a fit failure was reported as absent supply")
	}
	if noFit.ReasonCode != capabilityfit.ReasonCapacityExceeded {
		t.Fatalf("reason = %s, want the specific fit failure", noFit.ReasonCode)
	}
}

// Thin supply is worth saying out loud: a planner that treats one eligible
// truck as a guarantee is surprised when it is taken before the commit.
func TestPreviewReportsLimitedSupply(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{
		{ProviderID: "p1", DistanceKm: 2}, {ProviderID: "p2", DistanceKm: 5},
	}}
	fits := &stubFits{trucks: map[string][]cargofit.TruckCapability{
		"p1": {fittingTruck("t1")}, "p2": {fittingTruck("t2")},
	}}
	response, _ := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())

	if !response.Serviceable {
		t.Fatalf("expected serviceable: %s", response.ReasonCode)
	}
	if response.SupplyStatus != supplypreview.StatusLimited {
		t.Fatalf("status = %s, want LIMITED with 2 eligible", response.SupplyStatus)
	}
	if response.EstimatedPickupETASeconds <= 0 {
		t.Fatal("no pickup ETA for available supply")
	}
}

// A disabled deployment says UNKNOWN rather than pretending it has no trucks,
// so a planner retries instead of writing Hauling off.
func TestDisabledPreviewAnswersUnknown(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 1}}}
	response, _ := previewerWith(false, nearby, &stubFits{}).PreviewSupply(previewContext(), lagosRequest())

	if response.SupplyStatus != supplypreview.StatusUnknown {
		t.Fatalf("status = %s, want UNKNOWN", response.SupplyStatus)
	}
	if nearby.calls != 0 {
		t.Fatal("a disabled preview still read the availability index")
	}
}

func TestPreviewRejectsNullIsland(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 1}}}
	response, _ := previewerWith(true, nearby, &stubFits{}).PreviewSupply(previewContext(), supplypreview.Request{})

	if response.ReasonCode != supplypreview.ReasonNotServiceable {
		t.Fatalf("reason = %s, want %s", response.ReasonCode, supplypreview.ReasonNotServiceable)
	}
	if nearby.calls != 0 {
		t.Fatal("an invalid coordinate still hit the index")
	}
}

// Preview and matching must evaluate identically. A second, preview-only fit
// rule is how a planner gets told "yes" by one path and "no" by another.
func TestPreviewRequirementMatchesTheMatcherVocabulary(t *testing.T) {
	requirement := requirementFromPreview(supplypreview.Request{
		WeightGrams: 500_000, LengthMm: 1_200, WidthMm: 800, HeightMm: 600,
		Handling: []string{"fragile", "CHILLED", "tail_lift"},
	})

	if !requirement.Fragile || !requirement.UprightRequired {
		t.Fatalf("FRAGILE did not imply upright: %+v", requirement)
	}
	if requirement.TemperatureClass != "CHILLED" {
		t.Fatalf("temperature class = %q", requirement.TemperatureClass)
	}
	if !requirement.TailLiftRequired {
		t.Fatal("TAIL_LIFT handling was dropped")
	}
	if !requirement.NeedsRefrigeration() {
		t.Fatal("CHILLED did not require refrigeration")
	}
}
