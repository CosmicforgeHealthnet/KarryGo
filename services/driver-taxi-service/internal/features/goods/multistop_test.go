package goods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
)

var allOn = ExecutionFlags{CapabilityEnabled: true, MultiPickupEnabled: true, SecureHandoverEnabled: true, CumulativeFitEnabled: true, RecoveryEnabled: true}

func policyWith(maxPickups int) capability.Policy {
	return capability.Policy{Version: "test-1", EffectiveAt: time.Now().Add(-time.Hour), Products: []capability.Product{
		{Code: "TAXI_GOODS_STANDARD", Version: 3, Purpose: capability.PurposeGoods, Active: true, MaxPickups: maxPickups, MaxDropoffs: 1, SecurePickupHandover: true, SecureDeliveryHandover: true},
	}}
}

func commitRequest(stops ...string) commercemission.Request {
	r := commercemission.Request{MissionRef: "mission-1", OperationRef: "op-1", ParentOrderRef: "order-1", VendorOrderRefs: []string{"vo-1"}, CustomerRef: "11111111-1111-4111-8111-111111111111",
		Service: commercemission.ServiceTaxi, Plan: &commercemission.PlanBinding{PlanRef: "plan-1", PlanVersion: 31, PlanFingerprint: "fp", MissionSlotRef: "slot-1"},
		Pickup: commercemission.Point{Address: "cluster"}, Dropoff: commercemission.Point{Ref: "customer", Address: "home", Latitude: 6.5, Longitude: 3.4}, Currency: "NGN", AgreedFeeMinor: 500_00}
	for i, stop := range stops {
		r.PickupStops = append(r.PickupStops, commercemission.PickupStop{StopRef: stop, VendorRef: "vendor-" + stop, Location: commercemission.Point{Latitude: 6.4 + float64(i)/100, Longitude: 3.3}})
		r.Parcels = append(r.Parcels, commercemission.Parcel{ParcelRef: "parcel-" + stop, PickupStopRef: stop, ManifestRef: "manifest-" + stop, ManifestVersion: 1, WeightGrams: 1_000, LengthMm: 300, WidthMm: 200, HeightMm: 100, PackageCount: 1})
	}
	return r
}

func TestBuildMultiStopCommitOneMissionForCompatibleVendors(t *testing.T) {
	for _, stops := range [][]string{{"A", "B"}, {"A", "B", "C"}, {"A", "B", "C", "D"}} {
		commit, reason, err := BuildMultiStopCommit(commitRequest(stops...), policyWith(4), allOn)
		if err != nil {
			t.Fatalf("%v: %s %v", stops, reason, err)
		}
		if len(commit.Stops) != len(stops)+1 || commit.Stops[len(commit.Stops)-1].StopType != custodyops.StopDropoff {
			t.Fatalf("%v: stops = %+v", stops, commit.Stops)
		}
		if len(commit.Custody.Manifests) != len(stops) {
			t.Fatalf("%v: every vendor pickup must keep its own manifest, got %d", stops, len(commit.Custody.Manifests))
		}
		for i, manifest := range commit.Custody.Manifests {
			if manifest.PickupStopRef != stops[i] {
				t.Fatalf("manifest %s bound to %s, want %s", manifest.ManifestRef, manifest.PickupStopRef, stops[i])
			}
		}
		if commit.Product.Code != "TAXI_GOODS_STANDARD" || commit.PolicyVersion != "test-1" {
			t.Fatalf("product snapshot = %+v %s", commit.Product, commit.PolicyVersion)
		}
	}
}

func TestBuildMultiStopCommitPolicyAndFlags(t *testing.T) {
	if _, reason, err := BuildMultiStopCommit(commitRequest("A", "B", "C"), policyWith(2), allOn); !errors.Is(err, ErrConflict) || reason != capability.ReasonPickupCountExceeded {
		t.Fatalf("pickups above policy: %s %v", reason, err)
	}
	off := allOn
	off.CumulativeFitEnabled = false
	if _, _, err := BuildMultiStopCommit(commitRequest("A", "B"), policyWith(4), off); !errors.Is(err, ErrDisabled) {
		t.Fatalf("multi-pickup without cumulative fit must stay disabled: %v", err)
	}
	noPlan := commitRequest("A", "B")
	noPlan.Plan = nil
	if _, _, err := BuildMultiStopCommit(noPlan, policyWith(4), allOn); !errors.Is(err, ErrInvalid) {
		t.Fatalf("commit without fulfilment plan binding: %v", err)
	}
	orphan := commitRequest("A", "B")
	orphan.Parcels = orphan.Parcels[:1]
	if _, _, err := BuildMultiStopCommit(orphan, policyWith(4), allOn); !errors.Is(err, ErrInvalid) {
		t.Fatalf("pickup stop without any parcel: %v", err)
	}
}

func TestBuildMultiStopCommitMultipleManifestsAtOneStop(t *testing.T) {
	r := commitRequest("A", "B")
	r.Parcels = append(r.Parcels, commercemission.Parcel{ParcelRef: "parcel-A2", PickupStopRef: "A", ManifestRef: "manifest-A-sealed-2", ManifestVersion: 1, WeightGrams: 500, PackageCount: 2})
	commit, _, err := BuildMultiStopCommit(r, policyWith(4), allOn)
	if err != nil {
		t.Fatal(err)
	}
	atA := 0
	for _, m := range commit.Custody.Manifests {
		if m.PickupStopRef == "A" {
			atA++
		}
	}
	if atA != 2 {
		t.Fatalf("stop A manifests = %d, want 2", atA)
	}
	var cargoA capability.StopCargo
	for _, c := range commit.Cargo {
		if c.StopRef == "A" {
			cargoA = c
		}
	}
	if cargoA.WeightGrams != 1_500 || cargoA.PackageCount != 3 {
		t.Fatalf("stop A cargo = %+v", cargoA)
	}
}

func TestVendorCannotSendProviderOrVehicleIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, field := range []string{"provider_id", "driver_id", "rider_id", "vehicle_id", "fleet_member_id", "assignee_id"} {
		for name, target := range map[string]any{"mission": &commercemission.Request{}, "preview": &ServiceabilityRequest{}} {
			body := `{"mission_ref":"m","` + field + `":"x"}`
			if name == "preview" {
				body = `{"preview_ref":"p","` + field + `":"x"}`
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
			if bind(c, target) {
				t.Fatalf("%s accepted %s", name, field)
			}
		}
	}
}

type fakeMarkets struct{}

func (fakeMarkets) ResolveZone(context.Context, geo.Point) (marketmodels.Zone, error) {
	return marketmodels.Zone{ID: "zone-1", MarketID: "market-1"}, nil
}

type recordingCandidates struct{ discoverableCalls int }

func (r *recordingCandidates) NearbyProviders(_ context.Context, _, class string, _ geo.Point, _ float64, _ int) ([]availabilityrepositories.ProviderStatus, error) {
	if class != "economy" {
		return nil, nil
	}
	return []availabilityrepositories.ProviderStatus{{ProviderID: "p-small"}, {ProviderID: "p-large"}}, nil
}

func (r *recordingCandidates) SetDiscoverable(context.Context, string, string, string, bool) error {
	r.discoverableCalls++
	return nil
}

type staticPolicy struct{ policy capability.Policy }

func (s staticPolicy) CurrentPolicy(context.Context) (capability.Policy, error) { return s.policy, nil }

func ptr[T any](v T) *T { return &v }

func vehicle(provider string, maxGrams int64) capability.Candidate {
	return capability.Candidate{ProviderID: provider, VehicleID: "v-" + provider, ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true,
		Eligibility: map[string]capability.Eligibility{"TAXI_GOODS_STANDARD": {ProductCode: "TAXI_GOODS_STANDARD", Status: capability.EligibilityEligible}},
		Capability: &capability.VehicleCapability{GoodsMaxWeightGrams: ptr(maxGrams), GoodsMaxVolumeML: ptr(int64(500_000)), GoodsMaxLengthMM: ptr(int64(900)),
			GoodsMaxWidthMM: ptr(int64(700)), GoodsMaxHeightMM: ptr(int64(500)), GoodsMaxPackageCount: ptr(int64(30))}}
}

func previewService(t *testing.T, candidates *recordingCandidates, caps ExecutionFlags, maxPickups int) *Service {
	t.Helper()
	return NewService(Options{Repository: &Repository{}, Markets: fakeMarkets{}, Candidates: candidates, Enabled: true, Capabilities: caps,
		Policies: staticPolicy{policyWith(maxPickups)},
		GoodsCandidates: func(context.Context, []string) ([]capability.Candidate, error) {
			return []capability.Candidate{vehicle("p-small", 30_000), vehicle("p-large", 60_000)}, nil
		}})
}

func previewRequest(grams ...int64) ServiceabilityRequest {
	r := ServiceabilityRequest{PreviewRef: "cart-31", Purpose: PurposeCommercePreview, CartVersion: 31, Dropoff: commercequote.Point{Latitude: 6.5, Longitude: 3.4}}
	for i, g := range grams {
		r.Pickups = append(r.Pickups, PreviewPickup{StopRef: string(rune('A' + i)), Location: commercequote.Point{Latitude: 6.4, Longitude: 3.3},
			Cargo: capability.StopCargo{WeightGrams: g, VolumeML: 10_000, MaxLengthMM: 300, MaxWidthMM: 200, MaxHeightMM: 100, PackageCount: 1}})
	}
	return r
}

func TestPreviewHasNoSideEffectsAndExposesNoProvider(t *testing.T) {
	candidates := &recordingCandidates{}
	svc := previewService(t, candidates, allOn, 4)
	for i := 0; i < 2; i++ {
		out, err := svc.PreviewServiceability(context.Background(), previewRequest(12_000, 17_000, 17_000))
		if err != nil {
			t.Fatal(err)
		}
		if !out.Serviceable || out.EligibleVehicleCount != 1 || out.CartVersion != 31 {
			t.Fatalf("A+B+C (46 kg) preview = %+v, want only the 60 kg vehicle", out)
		}
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "p-large") || strings.Contains(string(raw), "v-p-large") {
			t.Fatalf("preview exposed provider identity: %s", raw)
		}
	}
	if candidates.discoverableCalls != 0 {
		t.Fatalf("preview locked or reserved providers: %d calls", candidates.discoverableCalls)
	}
	// Repository{} has no database handle: any booking, offer, custody or
	// outbox write during preview would have panicked above.
}

func TestPreviewCumulativeOverloadAndPickupPolicy(t *testing.T) {
	svc := previewService(t, &recordingCandidates{}, allOn, 4)
	out, err := svc.PreviewServiceability(context.Background(), previewRequest(25_000, 25_000, 25_000))
	if err != nil || out.Serviceable || out.DetailReasonCode != capability.ReasonCargoWeightExceeded {
		t.Fatalf("75 kg cluster = %+v %v", out, err)
	}
	out, err = svc.PreviewServiceability(context.Background(), previewRequest(1, 1, 1, 1, 1))
	if err != nil || out.Serviceable || out.DetailReasonCode != capability.ReasonPickupCountExceeded {
		t.Fatalf("five pickups on four-pickup policy = %+v %v", out, err)
	}
}

func TestCapabilityAdvertisesMultiPickupOnlyWhenOperational(t *testing.T) {
	partial := allOn
	partial.SecureHandoverEnabled = false
	for name, tc := range map[string]struct {
		caps       ExecutionFlags
		maxPickups int
		want       bool
	}{
		"all execution pieces on":    {allOn, 4, true},
		"secure handover off":        {partial, 4, false},
		"policy still single pickup": {allOn, 1, false},
	} {
		out, err := previewService(t, &recordingCandidates{}, tc.caps, tc.maxPickups).Capability(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.MultiPickupSupported != tc.want || out.Authority != "TAXI" || out.MaximumDropoffs != 1 {
			t.Errorf("%s: %+v", name, out)
		}
		if !tc.want && out.MaximumPickups > 1 {
			t.Errorf("%s: advertised %d pickups while multi-pickup is not operational", name, out.MaximumPickups)
		}
	}
}

func TestReplanKeepsCollectedStopsAndDropoffLast(t *testing.T) {
	current := []MissionStop{{ExternalStopRef: "A", StopType: "PICKUP", Status: "COLLECTED"}, {ExternalStopRef: "B", StopType: "PICKUP", Status: "PLANNED"},
		{ExternalStopRef: "C", StopType: "PICKUP", Status: "PLANNED"}, {ExternalStopRef: "D", StopType: "DROPOFF", Status: "PLANNED"}}
	if err := validateReplan(current, []string{"A", "C", "B", "D"}); err != nil {
		t.Fatalf("reordering pending pickups: %v", err)
	}
	if err := validateReplan(current, []string{"B", "A", "C", "D"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("moving a collected stop: %v", err)
	}
	if err := validateReplan(current, []string{"A", "B", "D", "C"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dropoff not last: %v", err)
	}
	if err := validateReplan(current, []string{"A", "B", "C"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dropping a stop: %v", err)
	}
}

func TestReplacementMustFitHeldAndFutureCargo(t *testing.T) {
	svc := previewService(t, &recordingCandidates{}, allOn, 4)
	// Provider 1 already collected A (12 kg) and B (17 kg); C (17 kg) is still pending.
	pending := capability.GoodsMission{ProductCode: "TAXI_GOODS_STANDARD", Dropoffs: 1, Stops: []capability.StopCargo{
		{StopRef: "A", WeightGrams: 12_000, PackageCount: 1}, {StopRef: "B", WeightGrams: 17_000, PackageCount: 1}, {StopRef: "C", WeightGrams: 17_000, PackageCount: 1}}}
	product, _ := policyWith(4).Product("TAXI_GOODS_STANDARD")
	eligible, reason := svc.evaluateCandidates([]capability.Candidate{vehicle("nearby-small", 40_000), vehicle("farther-large", 60_000)}, pending, product)
	if len(eligible) != 1 || eligible[0].ProviderID != "farther-large" {
		t.Fatalf("replacement candidates = %+v (%s), the nearby 40 kg vehicle must be rejected", eligible, reason)
	}
}
