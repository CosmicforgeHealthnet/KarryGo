package missionplanning

import (
	"testing"
	"time"
)

func fixture() PlanRequest {
	now := time.Now().UTC().Add(-time.Minute)
	return PlanRequest{SourceRef: "commerce-1", SourceVersion: 1, CustomerID: "11111111-1111-1111-1111-111111111111", Receiver: Point{Address: "R", Latitude: 6.5, Longitude: 3.4}, ReceiverName: "Receiver", ReceiverPhone: "+2348012345678", FareAmount: 5000, Currency: "NGN", PaymentRef: "pay-1", ServiceClass: "STANDARD", OperationRef: "op-1", VehicleClass: "MOTORCYCLE", MaxWeightGrams: 10000, MaxVolumeML: 30000, PromisedBy: now.Add(2 * time.Hour), RouteEvidence: &RouteEvidence{Reference: "route:v1", Serviceable: true, DistanceKM: 10, DurationSeconds: 1200}, Units: []Unit{{UnitRef: "u1", VendorRef: "v1", BranchRef: "b1", OrderRef: "o1", Pickup: Point{Address: "A", Latitude: 6.4, Longitude: 3.3}, Ready: ReadyWindow{From: now, To: now.Add(time.Hour)}, ParcelRefs: []string{"p1"}}, {UnitRef: "u2", VendorRef: "v2", BranchRef: "b2", OrderRef: "o2", Pickup: Point{Address: "B", Latitude: 6.45, Longitude: 3.35}, Ready: ReadyWindow{From: now.Add(time.Minute), To: now.Add(time.Hour)}, ParcelRefs: []string{"p2"}}}, Parcels: []Parcel{{ParcelRef: "p1", UnitRef: "u1", WeightGrams: 1000, VolumeML: 1000, PackagePolicyVersion: "v1"}, {ParcelRef: "p2", UnitRef: "u2", WeightGrams: 1000, VolumeML: 1000, PackagePolicyVersion: "v1"}}}
}
func TestPlannerConsolidatesCompatibleMultiVendorUnits(t *testing.T) {
	p, err := NewPlanner().Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome != Consolidate || len(p.Missions) != 1 || len(p.Missions[0].Stops) != 3 {
		t.Fatalf("plan=%+v", p)
	}
	if len(p.Missions[0].ParcelRefs) != 2 {
		t.Fatal("manifest membership missing")
	}
}
func TestPlannerSplitsForCapacityAndExpress(t *testing.T) {
	in := fixture()
	in.MaxWeightGrams = 1500
	p, err := NewPlanner().Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome != Split || len(p.Missions) != 2 {
		t.Fatalf("plan=%+v", p)
	}
	in = fixture()
	in.ServiceClass = "EXPRESS"
	p, _ = NewPlanner().Build(in)
	if p.Outcome != Split || len(p.Missions) != 2 {
		t.Fatalf("express outcome=%s", p.Outcome)
	}
}

func TestPlannerReadinessSplitProducesMultipleMissions(t *testing.T) {
	in := fixture()
	in.Units[1].Ready.From = in.Units[0].Ready.From.Add(2 * time.Hour)
	in.Units[1].Ready.To = in.Units[1].Ready.From.Add(time.Hour)
	p, err := NewPlanner().Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome != Split || len(p.Missions) != 2 {
		t.Fatalf("plan=%+v", p)
	}
}

func TestPlannerCapsCallerCapacity(t *testing.T) {
	in := fixture()
	in.MaxWeightGrams = 999999999
	in.MaxVolumeML = 999999999
	p, err := NewPlanner().Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.EffectiveMaxWeightGrams != 25000 || p.EffectiveMaxVolumeML != 120000 {
		t.Fatalf("capacity=%+v", p)
	}
}

func TestPlannerUnknownVehicleCapacityRequiresReview(t *testing.T) {
	in := fixture()
	in.VehicleClass = "UNKNOWN"
	p, err := NewPlanner().Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Outcome != OperationalReview || p.Reasons[0] != "VEHICLE_CAPACITY_POLICY_UNAVAILABLE" {
		t.Fatalf("plan=%+v", p)
	}
}
func TestParcelScannableIdentityIsServerGeneratedAndStable(t *testing.T) {
	in := fixture()
	p, err := NewPlanner().Build(in)
	if err != nil {
		t.Fatal(err)
	}
	BindStableIdentities(&p, &in)
	firstRef, firstCode := p.Parcels[0].KarryGoParcelRef, p.Parcels[0].ScannableCode
	if firstRef == "" || firstCode == "" || firstRef == p.Parcels[0].ParcelRef {
		t.Fatalf("parcel=%+v", p.Parcels[0])
	}
	BindStableIdentities(&p, &in)
	if p.Parcels[0].KarryGoParcelRef != firstRef || p.Parcels[0].ScannableCode != firstCode {
		t.Fatal("identity changed")
	}
}
func TestPlannerRejectsMedicalAndIncompatibleTemperature(t *testing.T) {
	in := fixture()
	in.Parcels[0].Handling = []string{"MEDICAL"}
	p, _ := NewPlanner().Build(in)
	if p.Outcome != RejectIncompatible {
		t.Fatalf("medical=%s", p.Outcome)
	}
	in = fixture()
	in.Parcels[0].Temperature = "FROZEN"
	in.Parcels[1].Temperature = "AMBIENT_ONLY"
	p, _ = NewPlanner().Build(in)
	if p.Outcome != RejectIncompatible {
		t.Fatalf("temperature=%s", p.Outcome)
	}
}
func TestPlannerWaitsForFutureReadiness(t *testing.T) {
	in := fixture()
	for i := range in.Units {
		in.Units[i].Ready.From = time.Now().UTC().Add(time.Hour)
		in.Units[i].Ready.To = time.Now().UTC().Add(2 * time.Hour)
	}
	p, _ := NewPlanner().Build(in)
	if p.Outcome != WaitReadiness {
		t.Fatalf("outcome=%s", p.Outcome)
	}
}
