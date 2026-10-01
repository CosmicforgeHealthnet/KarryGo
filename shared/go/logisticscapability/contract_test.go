package logisticscapability

import (
	"testing"
	"time"
)

func dispatchCapability() Capability {
	return Capability{
		Authority: AuthorityDispatch, Version: 3, Available: true,
		MultiPickupSupported: true, MaxPickups: 5,
		MultiStopSupported: false, MaxDropoffs: 1,
		SecurePickupSupported: true, SecureDeliverySupported: true,
		CustodyEvidenceSupported: true, CorporateProviderSupported: true,
		VehicleClasses: []string{"MOTORCYCLE"}, MaxWeightGrams: 20000,
		MaxPackageCount: 6, HandlingSupported: []string{"FRAGILE", "HOT"},
		ObservedAt: time.Now().UTC(),
	}
}

func singlePickupCapability(authority Authority) Capability {
	return Capability{
		Authority: authority, Version: 1, Available: true,
		MultiPickupSupported: false, MaxPickups: 1,
		MultiStopSupported: false, MaxDropoffs: 1,
		SecurePickupSupported: true, SecureDeliverySupported: true,
		CustodyEvidenceSupported: true,
		ObservedAt:               time.Now().UTC(),
	}
}

// VND-002: one vocabulary, with both spellings of the Taxi Goods authority
// resolving to the value every service already persists.
func TestParseAuthority(t *testing.T) {
	cases := map[string]Authority{
		"DISPATCH":            AuthorityDispatch,
		"dispatch":            AuthorityDispatch,
		"  HAULING  ":         AuthorityHauling,
		"TAXI":                AuthorityTaxi,
		"TAXI_GOODS":          AuthorityTaxi,
		"VENDOR":              AuthorityVendorOwn,
		"VENDOR_OWN_DELIVERY": AuthorityVendorOwn,
	}
	for input, want := range cases {
		got, err := ParseAuthority(input)
		if err != nil {
			t.Fatalf("ParseAuthority(%q) errored: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseAuthority(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := ParseAuthority("ROCKET"); err == nil {
		t.Fatal("an unknown authority must be rejected, not defaulted")
	}
	// Vendor-own delivery is never a platform mission authority.
	if AuthorityVendorOwn.PlatformExecuted() {
		t.Fatal("vendor-own delivery must not be platform-executed")
	}
	for _, authority := range []Authority{AuthorityDispatch, AuthorityTaxi, AuthorityHauling} {
		if !authority.PlatformExecuted() {
			t.Fatalf("%s must be platform-executed", authority)
		}
	}
}

// VND-007/VND-008/Scenario S: a single-pickup authority must refuse a
// multi-pickup mission, and a multi-pickup one must accept it.
func TestMatch_MultiPickupCapability(t *testing.T) {
	requirement := Requirement{PickupCount: 3, DropoffCount: 1}

	if ok, reason := Match(dispatchCapability(), requirement); !ok {
		t.Fatalf("dispatch declares multi-pickup but refused: %s", reason)
	}
	for _, authority := range []Authority{AuthorityTaxi, AuthorityHauling} {
		ok, reason := Match(singlePickupCapability(authority), requirement)
		if ok {
			t.Fatalf("%s declares maxPickups=1 but accepted 3 pickups", authority)
		}
		if reason != ReasonMultiPickupNotAllowed {
			t.Fatalf("%s reason = %q, want %q", authority, reason, ReasonMultiPickupNotAllowed)
		}
	}
	// A single pickup is fine for every authority.
	single := Requirement{PickupCount: 1, DropoffCount: 1}
	for _, capability := range []Capability{dispatchCapability(),
		singlePickupCapability(AuthorityTaxi), singlePickupCapability(AuthorityHauling)} {
		if ok, reason := Match(capability, single); !ok {
			t.Fatalf("%s refused a single pickup: %s", capability.Authority, reason)
		}
	}
}

// An authority that declares MultiPickupSupported but a lower cap still
// refuses beyond that cap.
func TestMatch_PickupCapEnforcedIndependentlyOfFlag(t *testing.T) {
	capability := dispatchCapability()
	capability.MaxPickups = 2
	ok, reason := Match(capability, Requirement{PickupCount: 3})
	if ok || reason != ReasonTooManyPickups {
		t.Fatalf("ok=%v reason=%q, want refusal with %q", ok, reason, ReasonTooManyPickups)
	}
}

// A switched-off authority is never serviceable, whatever else it declares.
func TestMatch_UnavailableAuthorityIsNeverServiceable(t *testing.T) {
	capability := dispatchCapability()
	capability.Available = false
	ok, reason := Match(capability, Requirement{PickupCount: 1})
	if ok || reason != ReasonAuthorityUnavailable {
		t.Fatalf("ok=%v reason=%q, want refusal with %q", ok, reason, ReasonAuthorityUnavailable)
	}
}

// VND-016 / HLG-029: a protected handover must never silently downgrade. An
// authority that cannot verify simply is not serviceable for it.
func TestMatch_SecureHandoverCannotDowngrade(t *testing.T) {
	insecure := singlePickupCapability(AuthorityHauling)
	insecure.SecurePickupSupported = false
	insecure.SecureDeliverySupported = false

	if ok, reason := Match(insecure, Requirement{SecurePickup: true}); ok || reason != ReasonSecurePickupMissing {
		t.Fatalf("ok=%v reason=%q, want %q", ok, reason, ReasonSecurePickupMissing)
	}
	if ok, reason := Match(insecure, Requirement{SecureDelivery: true}); ok || reason != ReasonSecureDeliveryMissing {
		t.Fatalf("ok=%v reason=%q, want %q", ok, reason, ReasonSecureDeliveryMissing)
	}
	// An unprotected mission is still fine on the same authority.
	if ok, _ := Match(insecure, Requirement{}); !ok {
		t.Fatal("an unprotected mission must still be serviceable")
	}
}

func TestMatch_PhysicalBounds(t *testing.T) {
	capability := dispatchCapability()
	cases := []struct {
		name        string
		requirement Requirement
		want        string
	}{
		{"weight", Requirement{WeightGrams: 20001}, ReasonWeightExceeded},
		{"packages", Requirement{PackageCount: 7}, ReasonPackageCountExceeded},
		{"vehicle", Requirement{VehicleClass: "TRUCK"}, ReasonVehicleUnsupported},
		{"handling", Requirement{Handling: []string{"FROZEN"}}, ReasonHandlingUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := Match(capability, tc.requirement)
			if ok || reason != tc.want {
				t.Fatalf("ok=%v reason=%q, want %q", ok, reason, tc.want)
			}
		})
	}
	// At the bound exactly is still serviceable.
	if ok, reason := Match(capability, Requirement{WeightGrams: 20000, PackageCount: 6}); !ok {
		t.Fatalf("a requirement exactly at the declared bound must pass: %s", reason)
	}
}

// An undeclared bound is UNKNOWN, not zero-capacity. Treating it as zero
// would make every authority that omits a bound look unserviceable.
func TestMatch_UndeclaredBoundsAreNotZeroCapacity(t *testing.T) {
	capability := singlePickupCapability(AuthorityHauling) // declares no weight/volume bound
	if ok, reason := Match(capability, Requirement{WeightGrams: 5_000_000, VolumeMl: 900_000}); !ok {
		t.Fatalf("undeclared bounds must not refuse: %s", reason)
	}
	// And an authority declaring no vehicle classes accepts any class.
	if ok, reason := Match(capability, Requirement{VehicleClass: "TRUCK"}); !ok {
		t.Fatalf("undeclared vehicle classes must not refuse: %s", reason)
	}
}

func TestEvaluate_CarriesReasonAndSnapshot(t *testing.T) {
	answer := Evaluate(singlePickupCapability(AuthorityTaxi), Requirement{PickupCount: 2})
	if answer.Serviceable {
		t.Fatal("2 pickups against maxPickups=1 must not be serviceable")
	}
	if answer.Authority != AuthorityTaxi {
		t.Fatalf("authority = %q", answer.Authority)
	}
	if answer.ReasonCode != ReasonMultiPickupNotAllowed {
		t.Fatalf("reason = %q", answer.ReasonCode)
	}
	if answer.Capabilities == nil || answer.Capabilities.MaxPickups != 1 {
		t.Fatal("the answer must carry the capability snapshot it was decided from")
	}
	if answer.Version != 1 {
		t.Fatalf("version = %d", answer.Version)
	}
}

// VND-031: an authority with no credentials is absent from the available set,
// so Vendor cannot select an authority it cannot reach.
func TestSet_AvailableExcludesUnconfigured(t *testing.T) {
	set := Set{
		AuthorityDispatch: New(Options{BaseURL: "https://dispatch.internal", Secret: []byte("s"), Authority: AuthorityDispatch}),
		AuthorityTaxi:     New(Options{Authority: AuthorityTaxi}), // no URL/secret
		AuthorityHauling:  New(Options{BaseURL: "https://hauling.internal", Authority: AuthorityHauling}),
	}
	available := set.Available()
	if len(available) != 1 || available[0] != AuthorityDispatch {
		t.Fatalf("available = %v, want only DISPATCH", available)
	}
	if _, ok := set.For(AuthorityTaxi); ok {
		t.Fatal("an unconfigured authority must not resolve")
	}
	if _, ok := set.For(AuthorityDispatch); !ok {
		t.Fatal("a configured authority must resolve")
	}
}

// A protected multi-pickup mission is refused by an authority that can only
// issue one credential per mission, even though it routes across many
// pickups. Routing and protecting are different capabilities, and conflating
// them is how a mission gets accepted that can never complete its second
// handover.
func TestProtectedPickupsAreBoundedSeparatelyFromRouting(t *testing.T) {
	routesManyProtectsOne := Capability{
		Authority: AuthorityHauling, Available: true,
		MultiPickupSupported: true, MaxPickups: 10,
		SecurePickupSupported: true, MaxSecurePickups: 1,
	}

	if ok, reason := Match(routesManyProtectsOne, Requirement{
		PickupCount: 3, SecurePickup: true,
	}); ok || reason != ReasonSecurePickupLimit {
		t.Fatalf("a three-pickup protected mission was accepted: ok=%v reason=%s", ok, reason)
	}

	// The same three pickups without protection are fine: the routing bound
	// is genuinely ten and must not be narrowed by the credential bound.
	if ok, reason := Match(routesManyProtectsOne, Requirement{
		PickupCount: 3,
	}); !ok {
		t.Fatalf("an unprotected three-pickup mission was refused: %s", reason)
	}

	// One protected pickup is within the bound.
	if ok, reason := Match(routesManyProtectsOne, Requirement{
		PickupCount: 1, SecurePickup: true,
	}); !ok {
		t.Fatalf("a single protected pickup was refused: %s", reason)
	}
}

// An authority that routes many pickups and protects every one of them is
// unaffected.
func TestStopScopedAuthorityProtectsEveryPickup(t *testing.T) {
	stopScoped := Capability{
		Authority: AuthorityDispatch, Available: true,
		MultiPickupSupported: true, MaxPickups: 12,
		SecurePickupSupported: true, MaxSecurePickups: 12,
	}
	if ok, reason := Match(stopScoped, Requirement{
		PickupCount: 12, SecurePickup: true,
	}); !ok {
		t.Fatalf("a fully protected consolidated run was refused: %s", reason)
	}
	if ok, reason := Match(stopScoped, Requirement{
		PickupCount: 13, SecurePickup: true,
	}); ok || reason != ReasonTooManyPickups {
		t.Fatalf("the routing bound stopped applying: ok=%v reason=%s", ok, reason)
	}
}

// An authority that has not declared the bound is taken at its word on
// multi-pickup rather than being silently narrowed to one.
func TestUndeclaredSecureBoundFallsBackToTheRoutingBound(t *testing.T) {
	undeclared := Capability{
		Authority: AuthorityDispatch, Available: true,
		MultiPickupSupported: true, MaxPickups: 5,
		SecurePickupSupported: true,
	}
	if got := undeclared.SecurePickupLimit(); got != 5 {
		t.Fatalf("SecurePickupLimit() = %d; want the routing bound 5", got)
	}
	if ok, reason := Match(undeclared, Requirement{
		PickupCount: 4, SecurePickup: true,
	}); !ok {
		t.Fatalf("an undeclared bound narrowed the authority: %s", reason)
	}

	// With neither bound declared the answer is one, not unlimited.
	none := Capability{Authority: AuthorityTaxi, Available: true, SecurePickupSupported: true}
	if got := none.SecurePickupLimit(); got != 1 {
		t.Fatalf("SecurePickupLimit() = %d; want 1 when nothing is declared", got)
	}
}
