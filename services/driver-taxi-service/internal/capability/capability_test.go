package capability

import (
	"testing"
	"time"
)

var now = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

func ptrInt(v int) *int           { return &v }
func ptrInt64(v int64) *int64     { return &v }
func ptrBool(v bool) *bool        { return &v }
func ptrFloat(v float64) *float64 { return &v }

func eligible(products ...string) map[string]Eligibility {
	out := map[string]Eligibility{}
	for _, product := range products {
		out[product] = Eligibility{ProductCode: product, Status: EligibilityEligible}
	}
	return out
}

func passengerCandidate(capacity *int, products ...string) Candidate {
	return Candidate{ProviderID: "p", VehicleID: "v", ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true,
		Capability: &VehicleCapability{VehicleID: "v", TotalSeats: ptrInt(5), PassengerCapacity: capacity}, Eligibility: eligible(products...)}
}

var standard = Product{Code: "TAXI_STANDARD", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 4}
var max8 = Product{Code: "TAXI_MAX", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 8, MinPassengerCapacity: 7}

func TestPassengerCapacity(t *testing.T) {
	four := passengerCandidate(ptrInt(4), "TAXI_STANDARD")
	cases := []struct {
		name       string
		candidate  Candidate
		product    Product
		passengers int
		want       string
	}{
		{"verified four accepts four", four, standard, 4, ReasonEligible},
		{"four rejects five", four, Product{Code: "TAXI_STANDARD", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 8}, 5, ReasonPassengerCapacityExceeded},
		{"six cannot match four", four, Product{Code: "TAXI_STANDARD", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 8}, 6, ReasonPassengerCapacityExceeded},
		{"eight needs an eligible eight-seat vehicle", passengerCandidate(ptrInt(8), "TAXI_MAX"), max8, 8, ReasonEligible},
		{"eight rejected when not eligible for product", passengerCandidate(ptrInt(8), "TAXI_STANDARD"), max8, 8, ReasonProductNotEligible},
	}
	for _, tc := range cases {
		got := EvaluatePassenger(tc.candidate, PassengerRequirement{ProductCode: tc.product.Code, PassengerCount: tc.passengers}, tc.product, now)
		if got.Reason != tc.want {
			t.Errorf("%s: reason = %s, want %s", tc.name, got.Reason, tc.want)
		}
	}
}

func TestLegacySeatsCannotDeterminePassengerCapacity(t *testing.T) {
	legacy := passengerCandidate(nil, "TAXI_STANDARD")
	legacy.Capability.TotalSeats = ptrInt(4)
	if got := EvaluatePassenger(legacy, PassengerRequirement{ProductCode: "TAXI_STANDARD", PassengerCount: 1}, standard, now); got.Reason != ReasonCapabilityUnknown {
		t.Fatalf("seats without verified passenger_capacity = %s, want CAPABILITY_UNKNOWN", got.Reason)
	}
}

func TestProductEligibilityStates(t *testing.T) {
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := map[string]struct {
		row  *Eligibility
		want string
	}{
		"missing (legacy comfort string alone)": {nil, ReasonProductNotEligible},
		"pending certification":                 {&Eligibility{Status: EligibilityPending}, ReasonVehicleNotCertified},
		"suspended":                             {&Eligibility{Status: EligibilitySuspended}, ReasonProductNotEligible},
		"expired status":                        {&Eligibility{Status: EligibilityExpired}, ReasonProductNotEligible},
		"eligible but past expiry":              {&Eligibility{Status: EligibilityEligible, ExpiresAt: &past}, ReasonProductNotEligible},
		"eligible not yet effective":            {&Eligibility{Status: EligibilityEligible, EffectiveAt: &future}, ReasonProductNotEligible},
		"revoked certification":                 {&Eligibility{Status: EligibilityRejected}, ReasonProductNotEligible},
	}
	for name, tc := range cases {
		candidate := passengerCandidate(ptrInt(4))
		if tc.row != nil {
			row := *tc.row
			row.ProductCode = "TAXI_COMFORT"
			candidate.Eligibility["TAXI_COMFORT"] = row
		}
		comfort := Product{Code: "TAXI_COMFORT", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 4}
		if got := EvaluatePassenger(candidate, PassengerRequirement{ProductCode: "TAXI_COMFORT", PassengerCount: 1}, comfort, now); got.Reason != tc.want {
			t.Errorf("%s: %s, want %s", name, got.Reason, tc.want)
		}
	}
}

func TestHardGateOrderPrecedesEverything(t *testing.T) {
	c := passengerCandidate(ptrInt(2))
	c.ProviderCompliant = false
	if got := EvaluatePassenger(c, PassengerRequirement{ProductCode: "TAXI_STANDARD", PassengerCount: 5}, standard, now); got.Reason != ReasonProviderNotCompliant {
		t.Fatalf("compliance must be checked before eligibility and capacity: %s", got.Reason)
	}
	c = passengerCandidate(ptrInt(4), "TAXI_STANDARD")
	c.Capability.WheelchairAccessible = nil
	if got := EvaluatePassenger(c, PassengerRequirement{ProductCode: "TAXI_STANDARD", PassengerCount: 1, WheelchairRequired: true}, standard, now); got.Reason != ReasonCapabilityUnknown {
		t.Fatalf("unknown mandatory accessibility must fail closed: %s", got.Reason)
	}
	c.Capability.WheelchairAccessible = ptrBool(false)
	if got := EvaluatePassenger(c, PassengerRequirement{ProductCode: "TAXI_STANDARD", PassengerCount: 1, WheelchairRequired: true}, standard, now); got.Reason != ReasonAccessibilityUnsupported {
		t.Fatalf("wheelchair unsupported = %s", got.Reason)
	}
}

var goodsStandard = Product{Code: "TAXI_GOODS_STANDARD", Version: 1, Purpose: PurposeGoods, Active: true, MaxPickups: 4, MaxDropoffs: 1}

func goodsVehicle() Candidate {
	return Candidate{ProviderOperational: true, ProviderCompliant: true, VehicleOperational: true, Eligibility: eligible("TAXI_GOODS_STANDARD"),
		Capability: &VehicleCapability{GoodsMaxWeightGrams: ptrInt64(40_000), GoodsMaxVolumeML: ptrInt64(300_000), GoodsMaxLengthMM: ptrInt64(900),
			GoodsMaxWidthMM: ptrInt64(700), GoodsMaxHeightMM: ptrInt64(500), GoodsMaxPackageCount: ptrInt64(20), FragileSafe: ptrBool(false),
			UprightSafe: ptrBool(false), WeatherProtected: ptrBool(true), TemperatureControlled: ptrBool(false), SecureCargoArea: ptrBool(false),
			CargoSeparationSupported: ptrBool(false)}}
}

func stop(ref string, grams int64) StopCargo {
	return StopCargo{StopRef: ref, WeightGrams: grams, VolumeML: 10_000, MaxLengthMM: 300, MaxWidthMM: 200, MaxHeightMM: 100, PackageCount: 1}
}

func goods(stops ...StopCargo) GoodsMission {
	return GoodsMission{ProductCode: "TAXI_GOODS_STANDARD", Stops: stops, Dropoffs: 1}
}

func TestCumulativeCargoFitRejectsCombinedOverload(t *testing.T) {
	a, b, c := stop("A", 12_000), stop("B", 17_000), stop("C", 17_000)
	for _, single := range []StopCargo{a, b, c} {
		if got := EvaluateGoods(goodsVehicle(), goods(single), goodsStandard, true, now); !got.Eligible {
			t.Fatalf("%s alone must fit: %s", single.StopRef, got.Reason)
		}
	}
	if got := EvaluateGoods(goodsVehicle(), goods(a, b), goodsStandard, true, now); !got.Eligible {
		t.Fatalf("A+B (29 kg) must fit: %s", got.Reason)
	}
	if got := EvaluateGoods(goodsVehicle(), goods(a, b, c), goodsStandard, true, now); got.Reason != ReasonCargoWeightExceeded {
		t.Fatalf("A+B+C (46 kg on a 40 kg vehicle) = %s, want CARGO_WEIGHT_EXCEEDED", got.Reason)
	}
}

func TestGoodsHardRejections(t *testing.T) {
	cases := map[string]struct {
		mutate func(*StopCargo)
		want   string
	}{
		"weight":        {func(s *StopCargo) { s.WeightGrams = 41_000 }, ReasonCargoWeightExceeded},
		"volume":        {func(s *StopCargo) { s.VolumeML = 400_000 }, ReasonCargoVolumeExceeded},
		"dimensions":    {func(s *StopCargo) { s.MaxLengthMM = 1_200 }, ReasonCargoDimensionExceeded},
		"fragile":       {func(s *StopCargo) { s.Fragile = true }, ReasonFragileHandlingUnsupported},
		"upright":       {func(s *StopCargo) { s.UprightRequired = true }, ReasonUprightHandlingUnsupported},
		"temperature":   {func(s *StopCargo) { s.TemperatureClass = TemperatureChilled; s.TemperatureMaxC = ptrFloat(5) }, ReasonTemperatureUnsupported},
		"secure cargo":  {func(s *StopCargo) { s.SecureCargoRequired = true }, ReasonSecureCargoUnsupported},
		"package count": {func(s *StopCargo) { s.PackageCount = 21 }, ReasonCargoPackageCountExceeded},
	}
	for name, tc := range cases {
		s := stop("A", 1_000)
		tc.mutate(&s)
		if got := EvaluateGoods(goodsVehicle(), goods(s), goodsStandard, true, now); got.Reason != tc.want {
			t.Errorf("%s: %s, want %s", name, got.Reason, tc.want)
		}
	}
	weather := goodsVehicle()
	weather.Capability.WeatherProtected = ptrBool(false)
	s := stop("A", 1_000)
	s.WeatherProtectionRequired = true
	if got := EvaluateGoods(weather, goods(s), goodsStandard, true, now); got.Reason != ReasonWeatherProtectionUnsupported {
		t.Errorf("weather: %s", got.Reason)
	}
}

func TestMixedTemperatureNeedsSeparation(t *testing.T) {
	v := goodsVehicle()
	v.Capability.TemperatureControlled, v.Capability.TemperatureMinC, v.Capability.TemperatureMaxC = ptrBool(true), ptrFloat(-25), ptrFloat(80)
	hot, frozen := stop("A", 1_000), stop("B", 1_000)
	hot.TemperatureClass, frozen.TemperatureClass = TemperatureHot, TemperatureFrozen
	if got := EvaluateGoods(v, goods(hot, frozen), goodsStandard, true, now); got.Reason != ReasonCargoSeparationUnsupported {
		t.Fatalf("hot+frozen without separation = %s", got.Reason)
	}
	v.Capability.CargoSeparationSupported = ptrBool(true)
	if got := EvaluateGoods(v, goods(hot, frozen), goodsStandard, true, now); !got.Eligible {
		t.Fatalf("hot+frozen with separation = %s", got.Reason)
	}
}

func TestMultiPickupPolicy(t *testing.T) {
	stops := []StopCargo{stop("A", 1_000), stop("B", 1_000), stop("C", 1_000), stop("D", 1_000)}
	if got := EvaluateGoods(goodsVehicle(), goods(stops...), goodsStandard, true, now); !got.Eligible {
		t.Fatalf("A+B+C+D within policy = %s", got.Reason)
	}
	if got := EvaluateGoods(goodsVehicle(), goods(append(stops, stop("E", 1_000))...), goodsStandard, true, now); got.Reason != ReasonPickupCountExceeded {
		t.Fatalf("five pickups on a four-pickup product = %s", got.Reason)
	}
	if got := EvaluateGoods(goodsVehicle(), goods(stops[:2]...), goodsStandard, false, now); got.Reason != ReasonMultiPickupUnsupported {
		t.Fatalf("multi-pickup with flag off = %s", got.Reason)
	}
	if got := EvaluateGoods(goodsVehicle(), goods(stops[0]), goodsStandard, false, now); !got.Eligible {
		t.Fatalf("single pickup with flag off must still work = %s", got.Reason)
	}
}

func TestPolicyParsingFailsClosed(t *testing.T) {
	valid := []byte(`{"policy_version":"v1","effective_at":"2026-09-01T00:00:00Z","products":[{"code":"taxi_goods_standard","version":1,"purpose":"goods","active":true,"max_pickups":2,"max_dropoffs":1}],"certification_required_fields":["passenger_capacity"]}`)
	policy, checksum, err := ParsePolicy(valid)
	if err != nil || checksum == "" || policy.MaxPickups("") != 2 || !policy.CertificationSensitive("passenger_capacity") {
		t.Fatalf("policy = %+v %s %v", policy, checksum, err)
	}
	for name, raw := range map[string]string{
		"unknown field (e.g. a price)": `{"policy_version":"v1","effective_at":"2026-09-01T00:00:00Z","products":[{"code":"X","version":1,"purpose":"GOODS","active":true,"max_pickups":2,"max_dropoffs":1,"fare_per_km":100}]}`,
		"goods without pickups":        `{"policy_version":"v1","effective_at":"2026-09-01T00:00:00Z","products":[{"code":"X","version":1,"purpose":"GOODS","active":true,"max_dropoffs":1}]}`,
		"duplicate product":            `{"policy_version":"v1","effective_at":"2026-09-01T00:00:00Z","products":[{"code":"X","version":1,"purpose":"PASSENGER","active":true,"max_passengers":4},{"code":"x","version":1,"purpose":"PASSENGER","active":true,"max_passengers":4}]}`,
	} {
		if _, _, err := ParsePolicy([]byte(raw)); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}
