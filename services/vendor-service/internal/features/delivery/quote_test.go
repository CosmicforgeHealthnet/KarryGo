package delivery

import (
	"math"
	"testing"
)

// Lagos reference points. Roughly 4.9 km apart, which keeps the arithmetic in
// these tests recognisable rather than arbitrary.
const (
	branchLat = 6.4550
	branchLng = 3.4100

	nearLat = 6.4300
	nearLng = 3.4500

	farLat = 6.6018
	farLng = 3.3515
)

func i64(v int64) *int64 { return &v }
func iptr(v int) *int    { return &v }

func basePolicy() Policy {
	return Policy{
		ID:                    "vdp_test",
		BusinessID:            "biz_1",
		BranchID:              "brn_1",
		Enabled:               true,
		Currency:              "NGN",
		BaseFeeMinor:          50000,
		PerKmFeeMinor:         10000,
		EstimatedMinutesBase:  15,
		EstimatedMinutesPerKm: 3,
		Version:               7,
	}
}

func baseRequest(policy Policy) QuoteRequest {
	return QuoteRequest{
		PlatformEnabled: true,
		Policy:          policy,
		Area: ServiceArea{
			CenterLatitude:  branchLat,
			CenterLongitude: branchLng,
			RadiusMeters:    10000,
		},
		HasArea:          true,
		PickupLatitude:   branchLat,
		PickupLongitude:  branchLng,
		DropoffLatitude:  nearLat,
		DropoffLongitude: nearLng,
		OrderValueMinor:  500000,
		Currency:         "NGN",
		TotalWeightGrams: 3000,
	}
}

func TestQuote_ServiceableChargesFromVendorPolicy(t *testing.T) {
	decision := Quote(baseRequest(basePolicy()))

	if !decision.Serviceable {
		t.Fatalf("expected serviceable, got refusal %q", decision.ReasonCode)
	}

	if decision.ReasonCode != ReasonServiceable {
		t.Fatalf("reason = %q, want %q", decision.ReasonCode, ReasonServiceable)
	}

	// The beneficiary is derived, and for vendor-own delivery it is the only
	// case in the platform where the vendor is owed the delivery money.
	if decision.FeeBeneficiary != BeneficiaryVendor {
		t.Fatalf("beneficiary = %q, want %q",
			decision.FeeBeneficiary, BeneficiaryVendor)
	}

	want := 50000 + int64(math.Round(10000*decision.DistanceKm))

	if decision.Funding.GrossFeeMinor != want {
		t.Fatalf("gross = %d, want %d", decision.Funding.GrossFeeMinor, want)
	}

	if decision.Funding.CustomerFeeMinor != want {
		t.Fatalf("customer = %d, want the full gross %d",
			decision.Funding.CustomerFeeMinor, want)
	}

	if decision.Funding.VendorFundedMinor != 0 {
		t.Fatalf("vendor funded = %d, want 0 without a free-delivery rule",
			decision.Funding.VendorFundedMinor)
	}

	if !decision.Funding.Balanced() {
		t.Fatal("funding split does not sum to the gross fee")
	}

	if decision.PolicyVersion != 7 {
		t.Fatalf("policy version = %d, want the exact policy revision 7",
			decision.PolicyVersion)
	}

	if decision.EstimatedMinutes <= 0 {
		t.Fatal("expected a positive estimated duration")
	}
}

// Free delivery is a vendor-funded promotion, not an absence of cost. The
// gross must survive so settlement can still answer who paid for it.
func TestQuote_FreeDeliveryIsVendorFundedNotFree(t *testing.T) {
	policy := basePolicy()
	policy.FreeDeliveryThresholdMinor = i64(400000)

	decision := Quote(baseRequest(policy))

	if !decision.Serviceable {
		t.Fatalf("expected serviceable, got %q", decision.ReasonCode)
	}

	if decision.Funding.GrossFeeMinor <= 0 {
		t.Fatal("free delivery must still record what the delivery costs")
	}

	if decision.Funding.CustomerFeeMinor != 0 {
		t.Fatalf("customer = %d, want 0 above the free threshold",
			decision.Funding.CustomerFeeMinor)
	}

	if decision.Funding.VendorFundedMinor != decision.Funding.GrossFeeMinor {
		t.Fatalf("vendor funded = %d, want the whole gross %d",
			decision.Funding.VendorFundedMinor,
			decision.Funding.GrossFeeMinor)
	}

	if !decision.Funding.Balanced() {
		t.Fatal("funding split does not sum to the gross fee")
	}
}

func TestQuote_BelowFreeThresholdCustomerPays(t *testing.T) {
	policy := basePolicy()
	policy.FreeDeliveryThresholdMinor = i64(900000)

	decision := Quote(baseRequest(policy))

	if decision.Funding.CustomerFeeMinor != decision.Funding.GrossFeeMinor {
		t.Fatalf("customer = %d, want the full gross %d below the threshold",
			decision.Funding.CustomerFeeMinor, decision.Funding.GrossFeeMinor)
	}
}

func TestQuote_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*QuoteRequest)
		wantHit string
	}{
		{
			name:    "platform switch off",
			mutate:  func(r *QuoteRequest) { r.PlatformEnabled = false },
			wantHit: ReasonPlatformDisabled,
		},
		{
			name:    "vendor switch off",
			mutate:  func(r *QuoteRequest) { r.Policy.Enabled = false },
			wantHit: ReasonVendorDisabled,
		},
		{
			name:    "currency mismatch",
			mutate:  func(r *QuoteRequest) { r.Currency = "USD" },
			wantHit: ReasonCurrencyMismatch,
		},
		{
			name: "null island dropoff",
			mutate: func(r *QuoteRequest) {
				r.DropoffLatitude, r.DropoffLongitude = 0, 0
			},
			wantHit: ReasonInvalidCoordinate,
		},
		{
			name:    "no service area configured",
			mutate:  func(r *QuoteRequest) { r.HasArea = false },
			wantHit: ReasonNoServiceArea,
		},
		{
			name: "outside service area",
			mutate: func(r *QuoteRequest) {
				r.DropoffLatitude, r.DropoffLongitude = farLat, farLng
			},
			wantHit: ReasonOutsideArea,
		},
		{
			name: "below minimum order",
			mutate: func(r *QuoteRequest) {
				r.Policy.MinimumOrderMinor = 900000
			},
			wantHit: ReasonBelowMinimum,
		},
		{
			name: "over weight limit",
			mutate: func(r *QuoteRequest) {
				r.Policy.MaxPackageWeightGrams = i64(1000)
			},
			wantHit: ReasonOverWeightLimit,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := baseRequest(basePolicy())
			testCase.mutate(&request)

			decision := Quote(request)

			if decision.Serviceable {
				t.Fatal("expected a refusal")
			}

			if decision.ReasonCode != testCase.wantHit {
				t.Fatalf("reason = %q, want %q",
					decision.ReasonCode, testCase.wantHit)
			}

			// A refusal must never carry a charge. Anything else would let a
			// fee reach a plan that has no delivery behind it.
			if decision.Funding.GrossFeeMinor != 0 ||
				decision.Funding.CustomerFeeMinor != 0 {
				t.Fatalf("refusal carried a fee: %+v", decision.Funding)
			}

			if decision.FeeBeneficiary != "" {
				t.Fatalf("refusal named a beneficiary: %q",
					decision.FeeBeneficiary)
			}
		})
	}
}

// The policy override may only narrow the Section 6 service area. This is the
// structural guard against a vendor granting itself a wider reach by editing
// its own delivery policy.
func TestQuote_PolicyRadiusOnlyNarrows(t *testing.T) {
	policy := basePolicy()
	policy.MaxDeliveryRadiusMeters = iptr(1000)

	if decision := Quote(baseRequest(policy)); decision.Serviceable ||
		decision.ReasonCode != ReasonOutsideArea {
		t.Fatalf("a narrower policy radius must refuse a further dropoff, got %+v",
			decision)
	}

	// Now the other direction: a huge policy radius against a small branch
	// service area must NOT extend the reach.
	widening := basePolicy()
	widening.MaxDeliveryRadiusMeters = iptr(200000)

	request := baseRequest(widening)
	request.Area.RadiusMeters = 500

	if decision := Quote(request); decision.Serviceable {
		t.Fatal("a policy radius must never widen the branch service area")
	}
}

func TestEffectiveRadiusMeters(t *testing.T) {
	area := ServiceArea{RadiusMeters: 8000}

	if got := effectiveRadiusMeters(area, nil); got != 8000 {
		t.Fatalf("nil override = %d, want the branch radius 8000", got)
	}

	if got := effectiveRadiusMeters(area, iptr(3000)); got != 3000 {
		t.Fatalf("narrower override = %d, want 3000", got)
	}

	if got := effectiveRadiusMeters(area, iptr(50000)); got != 8000 {
		t.Fatalf("wider override = %d, want the branch radius 8000", got)
	}
}

// Reach is measured from the service-area centre, which Section 6 allows to
// differ from the branch pin.
func TestQuote_ReachIsMeasuredFromServiceAreaCentre(t *testing.T) {
	request := baseRequest(basePolicy())
	request.Area.CenterLatitude = nearLat
	request.Area.CenterLongitude = nearLng
	request.Area.RadiusMeters = 800

	if decision := Quote(request); !decision.Serviceable {
		t.Fatalf("dropoff at the area centre must be serviceable, got %q",
			decision.ReasonCode)
	}

	// The same tight radius centred on the branch instead excludes it.
	request.Area.CenterLatitude = branchLat
	request.Area.CenterLongitude = branchLng

	if decision := Quote(request); decision.Serviceable {
		t.Fatal("expected the dropoff to fall outside a branch-centred radius")
	}
}

func TestPolicyInputValidation(t *testing.T) {
	valid := PolicyInput{Enabled: true, Currency: "ngn"}

	if err := valid.validate(); err != nil {
		t.Fatalf("lowercase currency should normalize, got %v", err)
	}

	cases := map[string]PolicyInput{
		"empty currency":        {Currency: ""},
		"non alpha currency":    {Currency: "N1G"},
		"negative base":         {Currency: "NGN", BaseFeeMinor: -1},
		"negative per km":       {Currency: "NGN", PerKmFeeMinor: -1},
		"negative minimum":      {Currency: "NGN", MinimumOrderMinor: -1},
		"negative free":         {Currency: "NGN", FreeDeliveryThresholdMinor: i64(-1)},
		"free below minimum":    {Currency: "NGN", MinimumOrderMinor: 500, FreeDeliveryThresholdMinor: i64(100)},
		"zero radius override":  {Currency: "NGN", MaxDeliveryRadiusMeters: iptr(0)},
		"huge radius override":  {Currency: "NGN", MaxDeliveryRadiusMeters: iptr(200001)},
		"zero weight limit":     {Currency: "NGN", MaxPackageWeightGrams: i64(0)},
		"negative minutes base": {Currency: "NGN", EstimatedMinutesBase: -1},
		"absurd minutes per km": {Currency: "NGN", EstimatedMinutesPerKm: 5000},
	}

	for name, input := range cases {
		if err := input.validate(); err == nil {
			t.Fatalf("%s: expected validation failure", name)
		}
	}
}
