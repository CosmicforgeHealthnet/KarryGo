package capabilities

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/observability"
)

const policyJSON = `{"policy_version":"p1","effective_at":"2026-01-01T00:00:00Z","products":[
 {"code":"TAXI_STANDARD","version":1,"purpose":"PASSENGER","label":"Standard","active":true,"max_passengers":4,"legacy_vehicle_class":"economy"},
 {"code":"TAXI_COMFORT","version":1,"purpose":"PASSENGER","label":"Comfort","active":true,"max_passengers":4,"legacy_vehicle_class":"comfort"}],
 "certification_required_fields":["passenger_capacity","wheelchair_accessible"]}`

type fakeStore struct {
	policyErr  error
	candidates map[string]capability.Candidate
	evidence   []EvidenceRow
	mode       string
}

func (f *fakeStore) LatestPolicy(context.Context, string, time.Time) ([]byte, string, error) {
	if f.policyErr != nil {
		return nil, "", f.policyErr
	}
	return []byte(policyJSON), "sum", nil
}
func (f *fakeStore) InsertPolicy(context.Context, string, string, string, time.Time, []byte) (bool, error) {
	return true, nil
}
func (f *fakeStore) GetCapability(context.Context, string, string) (capability.VehicleCapability, error) {
	return capability.VehicleCapability{}, ErrNotFound
}
func (f *fakeStore) PutProviderCapability(context.Context, string, capability.VehicleCapability, int64, []string) (capability.VehicleCapability, []string, error) {
	return capability.VehicleCapability{}, nil, nil
}
func (f *fakeStore) ListEligibility(context.Context, string) ([]capability.Eligibility, error) {
	return nil, nil
}
func (f *fakeStore) ApplyCertification(context.Context, CertificationEvent) (string, error) {
	return "APPLIED", nil
}
func (f *fakeStore) PassengerCandidates(context.Context, []string) (map[string]capability.Candidate, error) {
	return f.candidates, nil
}
func (f *fakeStore) RecordMatchEvidence(_ context.Context, _, _, mode string, rows []EvidenceRow) error {
	f.mode, f.evidence = mode, append(f.evidence, rows...)
	return nil
}
func (f *fakeStore) BookingRequirement(context.Context, string) (string, []byte, error) {
	return "", nil, nil
}

func capacity(n int) *int { return &n }

func candidates() map[string]capability.Candidate {
	certified := capability.Candidate{ProviderID: "certified", VehicleID: "v1", ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true,
		Capability:  &capability.VehicleCapability{PassengerCapacity: capacity(4)},
		Eligibility: map[string]capability.Eligibility{"TAXI_COMFORT": {ProductCode: "TAXI_COMFORT", Status: capability.EligibilityEligible}}}
	legacy := capability.Candidate{ProviderID: "legacy-comfort", VehicleID: "v2", ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true,
		Capability:  &capability.VehicleCapability{},
		Eligibility: map[string]capability.Eligibility{"TAXI_COMFORT": {ProductCode: "TAXI_COMFORT", Status: capability.EligibilityPending}}}
	return map[string]capability.Candidate{"certified": certified, "legacy-comfort": legacy}
}

var comfortBooking = bookingmodels.Booking{ID: "b1", VehicleClass: "comfort", PassengerCount: 2}

func TestShadowModeRecordsButDoesNotFilter(t *testing.T) {
	store := &fakeStore{candidates: candidates()}
	svc := NewService(store, Flags{ServiceProductsEnabled: true}, observability.NewMetrics())
	kept, err := svc.FilterPassengerCandidates(context.Background(), comfortBooking, "", nil, []string{"certified", "legacy-comfort"}, "i1")
	if err != nil || len(kept) != 2 {
		t.Fatalf("shadow kept = %v %v", kept, err)
	}
	if store.mode != "SHADOW" || len(store.evidence) != 2 {
		t.Fatalf("evidence = %s %+v", store.mode, store.evidence)
	}
	for _, row := range store.evidence {
		if row.ProviderID == "legacy-comfort" && (row.Decision.Eligible || row.Decision.Reason != capability.ReasonVehicleNotCertified) {
			t.Fatalf("legacy comfort class must not be certified comfort: %+v", row)
		}
	}
}

func TestEnforcedModeRejectsBeforeScoring(t *testing.T) {
	store := &fakeStore{candidates: candidates()}
	svc := NewService(store, Flags{ServiceProductsEnabled: true, CapabilityMatchingEnabled: true}, nil)
	kept, err := svc.FilterPassengerCandidates(context.Background(), comfortBooking, "", nil, []string{"legacy-comfort", "certified"}, "i1")
	if err != nil || len(kept) != 1 || kept[0] != "certified" {
		t.Fatalf("enforced kept = %v %v", kept, err)
	}
	five := comfortBooking
	five.PassengerCount = 5
	if kept, _ := svc.FilterPassengerCandidates(context.Background(), five, "", nil, []string{"certified"}, "i1"); len(kept) != 0 {
		t.Fatalf("five passengers on a four-passenger vehicle kept = %v", kept)
	}
}

func TestPolicyFallsBackToLastValidSnapshot(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, Flags{ServiceProductsEnabled: true}, nil)
	first, err := svc.CurrentPolicy(context.Background())
	if err != nil || first.Version != "p1" {
		t.Fatalf("first policy = %+v %v", first, err)
	}
	store.policyErr = errors.New("control plane unreachable")
	again, err := svc.CurrentPolicy(context.Background())
	if err != nil || again.Version != "p1" {
		t.Fatalf("fallback policy = %+v %v", again, err)
	}
	fresh := NewService(&fakeStore{policyErr: errors.New("down")}, Flags{}, nil)
	if _, err := fresh.CurrentPolicy(context.Background()); err == nil {
		t.Fatal("no applied policy at all must fail closed")
	}
}

func TestCertificationSensitiveChangesAreDetected(t *testing.T) {
	yes, no := true, false
	before := capability.VehicleCapability{PassengerCapacity: capacity(4), WheelchairAccessible: &no, BodyStyle: "SEDAN"}
	after := before
	after.WheelchairAccessible, after.BodyStyle = &yes, "SUV"
	changed := ChangedSensitiveFields(before, after, []string{"passenger_capacity", "wheelchair_accessible"})
	if len(changed) != 1 || changed[0] != "wheelchair_accessible" {
		t.Fatalf("changed = %v (body_style is not certification-sensitive)", changed)
	}
}

func TestCertificationEventValidation(t *testing.T) {
	svc := NewService(&fakeStore{}, Flags{}, nil)
	past := time.Now().Add(-time.Hour)
	for name, event := range map[string]CertificationEvent{
		"missing event id":          {VehicleID: "11111111-1111-4111-8111-111111111111", ProductCode: "TAXI_COMFORT", Decision: "CERTIFIED", SourceVersion: 1, CertificationRef: "c", PolicyVersion: "p1"},
		"bad vehicle id":            {EventID: "e", VehicleID: "nope", ProductCode: "TAXI_COMFORT", Decision: "CERTIFIED", SourceVersion: 1, CertificationRef: "c", PolicyVersion: "p1"},
		"certified already expired": {EventID: "e", VehicleID: "11111111-1111-4111-8111-111111111111", ProductCode: "TAXI_COMFORT", Decision: "CERTIFIED", SourceVersion: 1, CertificationRef: "c", PolicyVersion: "p1", ExpiresAt: &past},
		"no certification ref":      {EventID: "e", VehicleID: "11111111-1111-4111-8111-111111111111", ProductCode: "TAXI_COMFORT", Decision: "REVOKED", SourceVersion: 1, PolicyVersion: "p1"},
	} {
		if _, err := svc.ApplyCertification(context.Background(), event); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestProductListingHonoursPassengerCountAndHasNoPrice(t *testing.T) {
	svc := NewService(&fakeStore{}, Flags{ServiceProductsEnabled: true}, nil)
	products, version, err := svc.ListProducts(context.Background(), ProductQuery{Purpose: "passenger", PassengerCount: 5})
	if err != nil || version != "p1" || len(products) != 0 {
		t.Fatalf("five passengers against four-seat products = %+v %s %v", products, version, err)
	}
	if _, _, err := NewService(&fakeStore{}, Flags{}, nil).ListProducts(context.Background(), ProductQuery{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled listing: %v", err)
	}
}

// The frozen booking snapshot is what matching evaluates: a hard wheelchair
// requirement rejects a vehicle that is not certified accessible, while a
// soft preference in the same snapshot restricts nothing.
func TestMatchingUsesFrozenBookingSnapshot(t *testing.T) {
	store := &fakeStore{candidates: candidates()}
	svc := NewService(store, Flags{ServiceProductsEnabled: true, CapabilityMatchingEnabled: true}, nil)
	hard := []byte(`{"product_code":"TAXI_COMFORT","passenger_count":2,"wheelchair_required":true,"preferences":{"quiet_ride":true}}`)
	if kept, err := svc.FilterPassengerCandidates(context.Background(), comfortBooking, "TAXI_COMFORT", hard, []string{"certified"}, "i1"); err != nil || len(kept) != 0 {
		t.Fatalf("wheelchair snapshot kept = %v %v", kept, err)
	}
	soft := []byte(`{"product_code":"TAXI_COMFORT","passenger_count":2,"preferences":{"quiet_ride":true,"luggage":"LARGE"}}`)
	if kept, err := svc.FilterPassengerCandidates(context.Background(), comfortBooking, "TAXI_COMFORT", soft, []string{"certified"}, "i1"); err != nil || len(kept) != 1 {
		t.Fatalf("preference-only snapshot kept = %v %v", kept, err)
	}
	if store.evidence[0].Decision.Eligible || store.evidence[0].Decision.Reason == "" {
		t.Fatalf("rejection evidence missing: %+v", store.evidence[0])
	}
}
