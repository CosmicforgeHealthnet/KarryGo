package bikecapability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// stubNearby is a recording availability index.
type stubNearby struct {
	providers []NearbyProvider
	err       error
	calls     int
}

func (s *stubNearby) NearbyForPreview(_ context.Context, _, _, _ float64, _ int) ([]NearbyProvider, error) {
	s.calls++
	return s.providers, s.err
}

// stubFits stands in for the capability store, recording every call so a test
// can prove the preview only ever READ.
type stubFits struct {
	bikes map[string][]parcelfit.Capability
	err   error
	loads int
}

func (s *stubFits) load(_ context.Context, _ []string, _ parcelfit.Requirement) (map[string][]parcelfit.Capability, error) {
	s.loads++
	return s.bikes, s.err
}

func fittingBike(id string) parcelfit.Capability {
	return parcelfit.Capability{
		BikeID: id, BikeType: "dispatch_bike", Active: true, Verified: true,
		MaxPayloadGrams: 20_000, MaxPayloadGramsKnown: true,
		CapabilityVersion: 2,
	}
}

func previewerWith(enabled bool, nearby NearbyProviders, fits *stubFits) *Previewer {
	frozen := func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	previewer := NewPreviewer(PreviewOptions{
		Service: &Service{now: frozen},
		Nearby:  nearby,
		Enabled: enabled,
		Now:     frozen,
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
		PickupLatitude: 6.5244, PickupLongitude: 3.3792, WeightGrams: 4_000,
	}
}

// The central guarantee: a preview reads and nothing else. The stubs record
// every call the previewer is capable of making, and only reads appear.
func TestPreviewCreatesNothing(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{
		{ProviderID: "p1", DistanceKm: 1}, {ProviderID: "p2", DistanceKm: 2},
		{ProviderID: "p3", DistanceKm: 3}, {ProviderID: "p4", DistanceKm: 4},
	}}
	fits := &stubFits{bikes: map[string][]parcelfit.Capability{
		"p1": {fittingBike("b1")}, "p2": {fittingBike("b2")},
		"p3": {fittingBike("b3")}, "p4": {fittingBike("b4")},
	}}

	response, err := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !response.Serviceable {
		t.Fatalf("expected serviceable, got %s / %s", response.SupplyStatus, response.ReasonCode)
	}
	// One index read and one capability read. A mission, a booking, an offer
	// or a provider lock would need a write path the Previewer does not hold.
	if nearby.calls != 1 || fits.loads != 1 {
		t.Fatalf("unexpected call pattern: nearby=%d fits=%d", nearby.calls, fits.loads)
	}
}

// The Previewer must hold no write capability at all, so "creates nothing" is
// enforced by its dependencies rather than by the care of its author.
func TestPreviewerHoldsNoWriteCapability(t *testing.T) {
	previewerType := reflect.TypeOf(Previewer{})
	for i := 0; i < previewerType.NumField(); i++ {
		name := strings.ToLower(previewerType.Field(i).Name)
		for _, forbidden := range []string{"repo", "writer", "booking", "mission", "offer", "queue", "lock"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Previewer.%s looks like a write path", previewerType.Field(i).Name)
			}
		}
	}
	// NearbyProviders exposes exactly one method, and it is a read.
	nearbyType := reflect.TypeOf((*NearbyProviders)(nil)).Elem()
	if nearbyType.NumMethod() != 1 || nearbyType.Method(0).Name != "NearbyForPreview" {
		t.Fatalf("NearbyProviders exposes more than the one read: %d methods", nearbyType.NumMethod())
	}
}

// The response must never let a caller learn which rider or bike would have
// taken the job -- that would make the preview an assignment.
func TestPreviewLeaksNoProviderOrBikeIdentity(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "provider-secret", DistanceKm: 2}}}
	fits := &stubFits{bikes: map[string][]parcelfit.Capability{
		"provider-secret": {fittingBike("bike-secret")},
	}}

	response, _ := previewerWith(true, nearby, fits).PreviewSupply(previewContext(), lagosRequest())
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)
	for _, secret := range []string{"provider-secret", "bike-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("preview leaked %q: %s", secret, body)
		}
	}
}

// An availability index that did not answer is UNKNOWN, never "no riders". A
// planner that reads the second stops routing to Dispatch altogether.
func TestPreviewDependencyFailureIsUnknownNotRefusal(t *testing.T) {
	nearby := &stubNearby{err: errors.New("redis unavailable")}
	response, _ := previewerWith(true, nearby, &stubFits{}).PreviewSupply(previewContext(), lagosRequest())

	if response.SupplyStatus != supplypreview.StatusUnknown {
		t.Fatalf("status = %s, want UNKNOWN", response.SupplyStatus)
	}
	if response.ReasonCode != supplypreview.ReasonDependencyUnavailable {
		t.Fatalf("reason = %s", response.ReasonCode)
	}
}

func TestPreviewCapabilityStoreFailureIsUnknown(t *testing.T) {
	nearby := &stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 1}}}
	response, _ := previewerWith(true, nearby, &stubFits{err: errors.New("postgres down")}).
		PreviewSupply(previewContext(), lagosRequest())

	if response.ReasonCode != supplypreview.ReasonDependencyUnavailable {
		t.Fatalf("reason = %s", response.ReasonCode)
	}
}

// "No candidates at all" and "candidates that all failed fit" are different
// problems, and a planner acts on them differently.
func TestPreviewDistinguishesNoSupplyFromNoFit(t *testing.T) {
	noSupply, _ := previewerWith(true, &stubNearby{}, &stubFits{}).
		PreviewSupply(previewContext(), lagosRequest())
	if noSupply.ReasonCode != supplypreview.ReasonSupplyUnavailable {
		t.Fatalf("no-supply reason = %s", noSupply.ReasonCode)
	}

	tiny := fittingBike("b1")
	tiny.MaxPayloadGrams = 500
	noFit, _ := previewerWith(true,
		&stubNearby{providers: []NearbyProvider{{ProviderID: "p1", DistanceKm: 1}}},
		&stubFits{bikes: map[string][]parcelfit.Capability{"p1": {tiny}}}).
		PreviewSupply(previewContext(), lagosRequest())

	if noFit.Serviceable {
		t.Fatal("a parcel nothing can carry was reported serviceable")
	}
	if noFit.ReasonCode != capabilityfit.ReasonCapacityExceeded {
		t.Fatalf("reason = %s, want the specific fit failure", noFit.ReasonCode)
	}
}

// Thin supply is worth saying out loud: a planner that treats one eligible
// rider as a guarantee is surprised when they are taken before the commit.
func TestPreviewReportsLimitedSupply(t *testing.T) {
	response, _ := previewerWith(true,
		&stubNearby{providers: []NearbyProvider{
			{ProviderID: "p1", DistanceKm: 1}, {ProviderID: "p2", DistanceKm: 3},
		}},
		&stubFits{bikes: map[string][]parcelfit.Capability{
			"p1": {fittingBike("b1")}, "p2": {fittingBike("b2")},
		}}).PreviewSupply(previewContext(), lagosRequest())

	if response.SupplyStatus != supplypreview.StatusLimited {
		t.Fatalf("status = %s, want LIMITED with 2 eligible", response.SupplyStatus)
	}
	if response.EstimatedPickupETASeconds <= 0 {
		t.Fatal("no pickup ETA for available supply")
	}
}

// A disabled deployment says UNKNOWN rather than pretending it has no riders.
func TestDisabledPreviewAnswersUnknownWithoutReading(t *testing.T) {
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
	response, _ := previewerWith(true, nearby, &stubFits{}).
		PreviewSupply(previewContext(), supplypreview.Request{})

	if response.ReasonCode != supplypreview.ReasonNotServiceable {
		t.Fatalf("reason = %s", response.ReasonCode)
	}
	if nearby.calls != 0 {
		t.Fatal("an invalid coordinate still hit the index")
	}
}

// Preview and assignment must evaluate identically, or a planner is told "yes"
// by one path and "no" by another for the same parcel.
func TestPreviewRequirementMatchesTheAssignmentVocabulary(t *testing.T) {
	requirement := requirementFromPreview(supplypreview.Request{
		WeightGrams: 4_000, LengthMm: 300, WidthMm: 200, HeightMm: 200,
		Handling:     []string{"fragile", "CHILLED", "high_value"},
		SecurePickup: true,
	})

	if !requirement.Fragile || !requirement.UprightRequired {
		t.Fatalf("FRAGILE did not imply upright: %+v", requirement)
	}
	if requirement.TemperatureClass != "CHILLED" || !requirement.NeedsThermal() {
		t.Fatalf("CHILLED was dropped: %+v", requirement)
	}
	// High-value goods need a box that closes, not merely one that keeps rain
	// off.
	if !requirement.EnclosureRequired {
		t.Fatal("HIGH_VALUE did not require an enclosed box")
	}
	if !requirement.SecureHandover {
		t.Fatal("secure pickup was dropped")
	}
}

// ── Vendor boundary ───────────────────────────────────────────────────────────

// Vendor may state what the goods need. It may never state who carries them.
// The preview request is a cross-service contract, so this is checked
// structurally rather than by reading the handler.
func TestPreviewRequestCannotCarryAnExecutionIdentity(t *testing.T) {
	forbidden := []string{"provider", "rider", "driver", "vehicle", "bike", "truck", "assignee", "member", "fleet"}

	// VehicleClass is a requirement, not an identity: "this needs a van" is a
	// statement about the goods that the quote and mission contracts already
	// carry, and it names no particular van. The distinction the rule is really
	// drawing is between describing what is needed and choosing who provides
	// it, so the one legitimate class descriptor is listed explicitly rather
	// than the rule being weakened for everything.
	allowed := map[string]bool{"VehicleClass": true}

	requestType := reflect.TypeOf(supplypreview.Request{})
	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i).Name
		if allowed[field] {
			continue
		}
		name := strings.ToLower(field)
		for _, needle := range forbidden {
			if strings.Contains(name, needle) {
				t.Fatalf("supplypreview.Request.%s lets a caller name an executor", field)
			}
		}
	}

	// The allowlist must stay an allowlist: a field added to it later that
	// actually carries an id would defeat the check above.
	for field := range allowed {
		if strings.HasSuffix(field, "ID") || strings.HasSuffix(field, "Id") || strings.HasSuffix(field, "Ref") {
			t.Fatalf("%s is an identity and cannot be allowlisted", field)
		}
	}
}

// An unknown JSON field must not become a hidden channel for an identity: the
// request decodes into a fixed struct, so a provider_id in the body is simply
// discarded rather than reaching any decision.
func TestUnknownRequestFieldsAreDiscarded(t *testing.T) {
	var request supplypreview.Request
	body := `{"pickupLatitude":6.5,"pickupLongitude":3.3,"providerId":"p1","bikeId":"b1"}`
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode: %v", err)
	}
	encoded, _ := json.Marshal(request)
	lowered := strings.ToLower(string(encoded))
	if strings.Contains(lowered, "p1") || strings.Contains(lowered, "b1") {
		t.Fatalf("an identity survived decoding: %s", encoded)
	}
}
