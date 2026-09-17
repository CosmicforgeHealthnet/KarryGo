package capability

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func snapshotPolicy() Policy {
	return Policy{Version: "p9", Products: []Product{
		{Code: "TAXI_STANDARD", Version: 3, Purpose: PurposePassenger, Active: true, MaxPassengers: 4, LegacyVehicleClass: "economy", GuaranteeProfileVersion: "std-g2"},
		{Code: "TAXI_XL", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 6, LegacyVehicleClass: "xl"},
		{Code: "TAXI_GOODS_CAR", Version: 1, Purpose: PurposeGoods, Active: true},
		{Code: "TAXI_RETIRED", Version: 1, Purpose: PurposePassenger, Active: false},
	}}
}

func yes() *bool           { v := true; return &v }
func no() *bool            { v := false; return &v }
func str(s string) *string { return &s }

func TestPassengerSnapshotRequestWinsOverSavedNeeds(t *testing.T) {
	hints := CustomerMobilityHints{Status: CustomerContextUsed, Version: 7, WheelchairRequired: yes(), ChildSeatPreference: str("BOOSTER"),
		PetTravelPreference: str("NONE"), LuggageDefault: "large", QuietRide: yes(), PreferredChannel: "text"}
	snap, product, err := BuildPassengerSnapshot(snapshotPolicy(), "", "economy", 2,
		HardRequirementInput{ChildSeatRequired: no()}, RidePreferenceInput{PreferredChannel: "call"}, hints, time.Unix(100, 0))
	if err != nil || product.Code != "TAXI_STANDARD" {
		t.Fatalf("build = %v %v", product.Code, err)
	}
	if !snap.WheelchairRequired || snap.Sources["wheelchair_required"] != SourceCustomerProfile {
		t.Fatalf("saved wheelchair need must become a hard requirement: %+v", snap)
	}
	if snap.ChildSeatRequired || snap.Sources["child_seat_required"] != SourceRequest {
		t.Fatalf("explicit request must override the saved child seat: %+v", snap)
	}
	if snap.PetTravel || snap.Sources["pet_travel"] != SourceCustomerProfile {
		t.Fatalf("saved NONE pet preference must be an explicit false: %+v", snap)
	}
	if snap.Preferences.PreferredChannel != "CALL" || !snap.Preferences.QuietRide || snap.Preferences.Luggage != "LARGE" {
		t.Fatalf("preferences = %+v", snap.Preferences)
	}
	if snap.ProductVersion != 3 || snap.PolicyVersion != "p9" || snap.GuaranteeProfileVersion != "std-g2" || snap.CustomerContext.Version != 7 {
		t.Fatalf("versions = %+v", snap)
	}
}

func TestPassengerSnapshotPreferencesNeverBecomeHardGates(t *testing.T) {
	snap, _, err := BuildPassengerSnapshot(snapshotPolicy(), "TAXI_XL", "xl", 5, HardRequirementInput{},
		RidePreferenceInput{QuietRide: yes(), Luggage: "large"}, CustomerMobilityHints{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snap)
	var gate PassengerRequirement
	if err := json.Unmarshal(raw, &gate); err != nil {
		t.Fatal(err)
	}
	if gate != (PassengerRequirement{ProductCode: "TAXI_XL", PassengerCount: 5}) {
		t.Fatalf("gate read from snapshot = %+v", gate)
	}
	if snap.GuaranteeProfileVersion != GuaranteeProfileNone || snap.CustomerContext.Status != CustomerContextNotRequested {
		t.Fatalf("absent guarantee/context must be explicit: %+v", snap)
	}
}

func TestPassengerSnapshotRejectsInvalidProducts(t *testing.T) {
	cases := []struct {
		code, class string
		passengers  int
		want        error
	}{
		{"TAXI_UNKNOWN", "economy", 1, ErrUnknownProduct},
		{"TAXI_RETIRED", "", 1, ErrUnknownProduct},
		{"TAXI_GOODS_CAR", "", 1, ErrProductPurposeMismatch},
		{"TAXI_XL", "economy", 1, ErrProductClassMismatch},
		{"TAXI_STANDARD", "economy", 5, ErrProductPassengerLimit},
		{"", "bike", 1, ErrUnknownProduct},
	}
	for _, c := range cases {
		if _, _, err := BuildPassengerSnapshot(snapshotPolicy(), c.code, c.class, c.passengers, HardRequirementInput{}, RidePreferenceInput{}, CustomerMobilityHints{}, time.Now()); !errors.Is(err, c.want) {
			t.Errorf("%s/%s/%d -> %v, want %v", c.code, c.class, c.passengers, err, c.want)
		}
	}
}
