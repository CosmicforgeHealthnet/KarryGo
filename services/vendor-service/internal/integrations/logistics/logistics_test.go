package logistics

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

// -----------------------------------------------------------------------------
// VND-002 canonical authority vocabulary
// -----------------------------------------------------------------------------

func TestAuthorityAliasesResolveToOneCanonicalValue(t *testing.T) {
	cases := map[string]Authority{
		"DISPATCH":            Dispatch,
		"dispatch":            Dispatch,
		"TAXI":                Taxi,
		"TAXI_GOODS":          Taxi,
		"taxi_goods":          Taxi,
		"HAULING":             Hauling,
		"VENDOR":              VendorOwn,
		"VENDOR_OWN_DELIVERY": VendorOwn,
	}
	for input, want := range cases {
		got, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q): unexpected error %v", input, err)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := Parse("UBER"); err == nil {
		t.Fatal("an unknown authority must not parse: silently accepting one would let a typo route real work nowhere")
	}
	if _, err := Parse(""); err == nil {
		t.Fatal("an empty authority must not parse")
	}
}

func TestTaxiGoodsAndTaxiAreTheSameAuthority(t *testing.T) {
	if !Equal("TAXI", "TAXI_GOODS") {
		t.Fatal("TAXI and TAXI_GOODS must name one authority; treating them as two is how a mission ends up with no executor")
	}
	if !Equal("VENDOR", "VENDOR_OWN_DELIVERY") {
		t.Fatal("VENDOR and VENDOR_OWN_DELIVERY must name one authority")
	}
	if Equal("TAXI", "HAULING") {
		t.Fatal("distinct authorities must not compare equal")
	}
	if Equal("TAXI", "NONSENSE") {
		t.Fatal("an unparseable value must never compare equal to a real authority")
	}
}

func TestOnlyPlatformAuthoritiesReceiveMissions(t *testing.T) {
	for _, authority := range PlatformAuthorities() {
		if !PlatformExecuted(authority) {
			t.Fatalf("%q must be a platform execution authority", authority)
		}
	}
	if PlatformExecuted(VendorOwn) {
		t.Fatal("vendor-owned delivery must never be treated as a platform authority: it creates no mission in any driver service")
	}
	if len(PlatformAuthorities()) != 3 {
		t.Fatalf("expected exactly three platform authorities, got %d", len(PlatformAuthorities()))
	}
}

func TestNormalizeReturnsCanonicalSpelling(t *testing.T) {
	if got := Normalize("taxi_goods"); got != "TAXI" {
		t.Fatalf("Normalize(taxi_goods) = %q, want TAXI", got)
	}
	if got := Normalize("not-an-authority"); got != "" {
		t.Fatalf("Normalize of an unknown value must be empty, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// VND-020 normalized execution status
// -----------------------------------------------------------------------------

func TestEachAuthorityRichStatusNormalizes(t *testing.T) {
	cases := []struct {
		authority Authority
		raw       string
		want      ExecutionStatus
	}{
		// Dispatch's own lifecycle vocabulary.
		{Dispatch, "OFFERING", StatusAssigning},
		{Dispatch, "ASSIGNED", StatusAssigned},
		{Dispatch, "EN_ROUTE_PICKUP", StatusProviderEnRoute},
		{Dispatch, "PICKUP_VERIFICATION", StatusArrivedPickup},
		{Dispatch, "PICKED_UP", StatusPickupVerified},
		{Dispatch, "DELIVERY_VERIFICATION", StatusArrivedDelivery},
		{Dispatch, "DELIVERED", StatusDeliveryVerified},
		{Dispatch, "COMPLETED", StatusCompleted},
		{Dispatch, "RETURNED", StatusFailed},

		// Taxi Goods.
		{Taxi, "MATCHING", StatusAssigning},
		{Taxi, "PICKUP_HANDOVER_REQUIRED", StatusArrivedPickup},
		{Taxi, "AT_DROPOFF", StatusArrivedDelivery},
		{Taxi, "UNMATCHED", StatusFailed},

		// Hauling stores its statuses lower-case.
		{Hauling, "awaiting_acceptance", StatusAssigning},
		{Hauling, "arrived_at_pickup", StatusArrivedPickup},
		{Hauling, "en_route_delivery", StatusInTransit},
		{Hauling, "delivered", StatusDeliveryVerified},
		{Hauling, "unmatched", StatusFailed},

		// The collapsed vocabulary every authority already sends today.
		{Hauling, "DELIVERING", StatusInTransit},
		{Taxi, "DELIVERY_FAILED", StatusFailed},
		{Dispatch, "REFUSED", StatusFailed},
	}
	for _, testCase := range cases {
		if got := NormalizeStatus(testCase.authority, testCase.raw); got != testCase.want {
			t.Fatalf("NormalizeStatus(%s, %q) = %q, want %q", testCase.authority, testCase.raw, got, testCase.want)
		}
	}
}

func TestUnknownStatusIsNeverGuessed(t *testing.T) {
	if got := NormalizeStatus(Dispatch, "SOMETHING_NEW"); got != StatusUnknown {
		t.Fatalf("an unmapped status must be StatusUnknown, got %q: guessing is how a projection skips a custody transfer", got)
	}
	if Advances(StatusAssigned, StatusUnknown) {
		t.Fatal("an unknown status must never advance a mission")
	}
}

func TestAuthoritySpecificStatusWinsOverTheSharedVocabulary(t *testing.T) {
	// Dispatch's own IN_TRANSIT and the collapsed DELIVERING both mean the
	// same commerce state; the point is that the authority table is consulted
	// first, so a private spelling can never be read as another service's.
	if got := NormalizeStatus(Dispatch, "IN_TRANSIT"); got != StatusInTransit {
		t.Fatalf("dispatch IN_TRANSIT = %q", got)
	}
	// Without a known authority only the shared vocabulary applies.
	if got := NormalizeStatusFor("not-an-authority", "AT_DROPOFF"); got != StatusUnknown {
		t.Fatalf("a private spelling from an unknown authority must not be interpreted, got %q", got)
	}
	if got := NormalizeStatusFor("not-an-authority", "DELIVERED"); got != StatusDeliveryVerified {
		t.Fatalf("the shared vocabulary must still apply, got %q", got)
	}
}

func TestDuplicateCallbackDoesNotAdvanceState(t *testing.T) {
	current := NormalizeStatus(Hauling, "picked_up")
	repeat := NormalizeStatus(Hauling, "picked_up")
	if Advances(current, repeat) {
		t.Fatal("a repeated pickup event must not advance the mission a second time (VND-018)")
	}
	delivered := NormalizeStatus(Hauling, "delivered")
	if !Advances(current, delivered) {
		t.Fatal("delivery must advance a picked-up mission")
	}
	if Advances(delivered, current) {
		t.Fatal("a stale pickup event arriving after delivery must not walk the mission backwards")
	}
}

func TestTerminalStateIsNeverOverwritten(t *testing.T) {
	for _, terminal := range []ExecutionStatus{StatusCompleted, StatusCancelled, StatusFailed} {
		if !terminal.Terminal() {
			t.Fatalf("%q must be terminal", terminal)
		}
		for _, next := range KnownStatusValues() {
			if Advances(terminal, next) {
				t.Fatalf("terminal %q must not advance to %q: a duplicate callback would otherwise re-complete an order", terminal, next)
			}
		}
	}
}

func TestCustodyWithProviderTracksTheHandoverWindow(t *testing.T) {
	holding := map[ExecutionStatus]bool{
		StatusPickupVerified:  true,
		StatusInTransit:       true,
		StatusArrivedDelivery: true,
	}
	for _, status := range KnownStatusValues() {
		if got := status.CustodyWithProvider(); got != holding[status] {
			t.Fatalf("%q custody-with-provider = %v, want %v", status, got, holding[status])
		}
	}
}

// -----------------------------------------------------------------------------
// VND-021 error normalization
// -----------------------------------------------------------------------------

func TestErrorClassificationMatrix(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FailureCategory
	}{
		{"mission client unconfigured", commercemission.ErrNotConfigured, FailureDependencyUnavailable},
		{"mission authority down", commercemission.ErrAuthorityUnavailable, FailureDependencyUnavailable},
		{"dispatch client unconfigured", dispatchcommerce.ErrNotConfigured, FailureDependencyUnavailable},
		{"dispatch authority down", dispatchcommerce.ErrAuthorityUnavailable, FailureDependencyUnavailable},
		{"capability client unconfigured", logisticscapability.ErrNotConfigured, FailureDependencyUnavailable},
		{"dispatch idempotency conflict", dispatchcommerce.ErrConflict, FailureExecutionConflict},
		{"authority answered about another mission", commercemission.ErrMissionMismatch, FailureExecutionConflict},
		{"authority re-priced", commercemission.ErrRepriced, FailureExecutionConflict},
		{"invalid mission request", commercemission.ErrInvalidRequest, FailureCapabilityMismatch},
		{"unknown authority", logisticscapability.ErrUnknownAuthority, FailureCapabilityMismatch},
		{"unserviceable area", errors.New("destination is outside service area"), FailureNotServiceable},
		{"over capacity", errors.New("load exceeds vehicle capacity"), FailureCapabilityMismatch},
		{"no provider", errors.New("no rider accepted the request"), FailureProviderUnavailable},
		{"credential", errors.New("pickup credential verification failed"), FailureCredential},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			failure := Classify(Dispatch, testCase.err)
			if failure.Category != testCase.want {
				t.Fatalf("Classify(%v) = %q, want %q", testCase.err, failure.Category, testCase.want)
			}
			if failure.Authority != Dispatch {
				t.Fatalf("classification lost the authority: %q", failure.Authority)
			}
			if !errors.Is(failure, testCase.err) {
				t.Fatal("the originating error must stay reachable for logs")
			}
		})
	}
	if Classify(Dispatch, nil).Category != FailureNone {
		t.Fatal("a nil error must classify as no failure")
	}
}

func TestUnrecognisedFailureNeedsReconciliationRatherThanAssumingFailure(t *testing.T) {
	failure := Classify(Hauling, errors.New("read tcp 10.0.0.7:443: connection reset by peer"))
	if failure.Category != FailureReconciliationRequired {
		t.Fatalf("an unrecognised transport error after a create must require reconciliation, got %q: assuming failure is how a second vehicle gets dispatched", failure.Category)
	}
	if !failure.Category.NeedsReconciliation() {
		t.Fatal("the category must report that it needs reconciliation")
	}
	if failure.Retryable() {
		t.Fatal("an unknown outcome must not be blindly retried; it must be reconciled first")
	}
}

func TestTimeoutIsAnUnknownOutcome(t *testing.T) {
	failure := Classify(Taxi, errors.New("Post \"https://taxi/internal\": context deadline exceeded"))
	if failure.Category != FailureReconciliationRequired {
		t.Fatalf("a create timeout is an unknown outcome, got %q (VND-024, Scenario U)", failure.Category)
	}
}

func TestSafeMessageNeverCarriesTheUnderlyingText(t *testing.T) {
	internal := errors.New("pq: relation \"taxi_goods_missions\" does not exist at 10.0.0.7:5432")
	failure := Classify(Taxi, internal)
	if failure.Category.Message() == "" {
		t.Fatal("every category must have a customer-safe message")
	}
	for _, rendered := range []string{failure.Error(), failure.Category.Message()} {
		if contains(rendered, "relation") || contains(rendered, "10.0.0.7") || contains(rendered, "pq:") {
			t.Fatalf("internal detail leaked into a customer-facing string: %q", rendered)
		}
	}
	for _, category := range KnownFailureCategories() {
		if category.Message() == "" {
			t.Fatalf("category %q has no safe message", category)
		}
	}
}

func TestRetryabilityIsExplicitPerCategory(t *testing.T) {
	retryable := map[FailureCategory]bool{
		FailureDependencyUnavailable: true,
		FailureProviderUnavailable:   true,
		FailureAssignmentTimeout:     true,
	}
	for _, category := range KnownFailureCategories() {
		if got := category.Retryable(); got != retryable[category] {
			t.Fatalf("%q retryable = %v, want %v", category, got, retryable[category])
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// -----------------------------------------------------------------------------
// VND-005 … VND-009, VND-030, VND-031 capability, serviceability, readiness
// -----------------------------------------------------------------------------

type stubAdapter struct {
	capability Capability
	err        error
	calls      int
}

func (s *stubAdapter) CheckCapability(context.Context) (Capability, error) {
	s.calls++
	if s.err != nil {
		return Capability{}, s.err
	}
	return s.capability, nil
}

func (s *stubAdapter) CheckServiceability(_ context.Context, requirement Requirement) (Serviceability, error) {
	s.calls++
	if s.err != nil {
		return Serviceability{}, s.err
	}
	return logisticscapability.Evaluate(s.capability, requirement), nil
}

func multiPickupCapability(authority Authority, maxPickups int64) Capability {
	return Capability{
		Authority: authority, Version: 1, Available: true,
		MultiPickupSupported: maxPickups > 1, MaxPickups: maxPickups,
		MultiStopSupported: false, MaxDropoffs: 1,
		SecurePickupSupported: true, SecureDeliverySupported: true,
		CustodyEvidenceSupported: true, CorporateProviderSupported: true,
		VehicleClasses: []string{"MOTORCYCLE"},
		MaxWeightGrams: 25_000, MaxPackageCount: 40,
		ObservedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
}

func testRegistry(t *testing.T) (*Registry, map[Authority]*stubAdapter) {
	t.Helper()
	stubs := map[Authority]*stubAdapter{
		Dispatch: {capability: multiPickupCapability(Dispatch, 12)},
		Taxi:     {capability: multiPickupCapability(Taxi, 1)},
		Hauling:  {capability: multiPickupCapability(Hauling, 10)},
	}
	adapters := map[Authority]CapabilityAdapter{}
	for authority, stub := range stubs {
		adapters[authority] = stub
	}
	registry := NewRegistry(RegistryOptions{
		Adapters: adapters,
		Enabled:  map[Authority]bool{Dispatch: true, Taxi: true, Hauling: true},
		Now:      func() time.Time { return time.Unix(1_700_000_100, 0).UTC() },
	})
	return registry, stubs
}

func TestMultiPickupRespectsDeclaredCapability(t *testing.T) {
	registry, _ := testRegistry(t)
	threePickups := Requirement{PickupCount: 3, DropoffCount: 1, PackageCount: 3}

	eligible := registry.EligibleAuthorities(context.Background(), threePickups)
	if len(eligible) != 2 {
		t.Fatalf("expected Dispatch and Hauling to be eligible for three pickups, got %v", eligible)
	}
	for _, authority := range eligible {
		if authority == Taxi {
			t.Fatal("Taxi Goods declares maxPickups=1 and must not be offered a three-pickup mission (VND-007, VND-030)")
		}
	}

	if err := registry.Authorize(context.Background(), Taxi, threePickups); err == nil {
		t.Fatal("authorizing a three-pickup mission against a single-pickup authority must fail")
	} else {
		failure := Classify(Taxi, err)
		if failure.Category != FailureCapabilityMismatch {
			t.Fatalf("expected a capability mismatch, got %q", failure.Category)
		}
		if failure.ReasonCode != logisticscapability.ReasonMultiPickupNotAllowed {
			t.Fatalf("expected the authority's own reason code, got %q", failure.ReasonCode)
		}
	}

	if err := registry.Authorize(context.Background(), Dispatch, threePickups); err != nil {
		t.Fatalf("Dispatch declares multi-pickup support and must accept: %v", err)
	}
}

func TestDisabledAuthorityIsNeverSelectedAndNeverContacted(t *testing.T) {
	registry, stubs := testRegistry(t)
	registry = NewRegistry(RegistryOptions{
		Adapters: map[Authority]CapabilityAdapter{Dispatch: stubs[Dispatch], Taxi: stubs[Taxi], Hauling: stubs[Hauling]},
		Enabled:  map[Authority]bool{Dispatch: true, Taxi: false, Hauling: true},
	})
	before := stubs[Taxi].calls

	if registry.Usable(Taxi) {
		t.Fatal("a disabled authority must not be usable")
	}
	err := registry.Authorize(context.Background(), Taxi, Requirement{PickupCount: 1, DropoffCount: 1})
	if err == nil {
		t.Fatal("a disabled authority must refuse authorization (VND-032)")
	}
	failure := Classify(Taxi, err)
	if failure.Category != FailureDependencyUnavailable || failure.ReasonCode != ReasonDisabled {
		t.Fatalf("expected a disabled dependency failure, got %q/%q", failure.Category, failure.ReasonCode)
	}
	if stubs[Taxi].calls != before {
		t.Fatal("a disabled authority must not be contacted at all")
	}

	// And it must not be silently replaced by a different authority.
	eligible := registry.EligibleAuthorities(context.Background(), Requirement{PickupCount: 1, DropoffCount: 1})
	for _, authority := range eligible {
		if authority == Taxi {
			t.Fatal("a disabled authority must never appear as eligible")
		}
	}
}

func TestUnconfiguredAuthorityFailsPredictably(t *testing.T) {
	registry := NewRegistry(RegistryOptions{
		Adapters: map[Authority]CapabilityAdapter{},
		Enabled:  map[Authority]bool{Hauling: true},
	})
	err := registry.Authorize(context.Background(), Hauling, Requirement{PickupCount: 1, DropoffCount: 1})
	failure := Classify(Hauling, err)
	if failure.Category != FailureDependencyUnavailable || failure.ReasonCode != ReasonNotConfigured {
		t.Fatalf("enabled-without-credentials must fail as an unconfigured dependency, got %q/%q", failure.Category, failure.ReasonCode)
	}
}

func TestVendorOwnDeliveryIsNeverAuthorizedAsAPlatformAuthority(t *testing.T) {
	registry, _ := testRegistry(t)
	err := registry.Authorize(context.Background(), VendorOwn, Requirement{PickupCount: 1, DropoffCount: 1})
	if err == nil {
		t.Fatal("vendor-owned delivery must never be authorized as a platform mission")
	}
	if Classify(VendorOwn, err).ReasonCode != ReasonNotPlatform {
		t.Fatalf("unexpected reason: %v", err)
	}
}

func TestProtectedHandoverCannotDowngrade(t *testing.T) {
	insecure := multiPickupCapability(Hauling, 10)
	insecure.SecureDeliverySupported = false
	registry := NewRegistry(RegistryOptions{
		Adapters: map[Authority]CapabilityAdapter{Hauling: &stubAdapter{capability: insecure}},
		Enabled:  map[Authority]bool{Hauling: true},
	})
	err := registry.Authorize(context.Background(), Hauling, Requirement{
		PickupCount: 1, DropoffCount: 1, SecureDelivery: true,
	})
	if err == nil {
		t.Fatal("a protected delivery must not be sent to an authority that cannot protect it (VND-016)")
	}
	failure := Classify(Hauling, err)
	if failure.Category != FailureCredential {
		t.Fatalf("expected a credential failure, got %q", failure.Category)
	}
}

func TestReadinessReportsEachAuthorityWithoutRaising(t *testing.T) {
	registry := NewRegistry(RegistryOptions{
		Adapters: map[Authority]CapabilityAdapter{
			Dispatch: &stubAdapter{capability: multiPickupCapability(Dispatch, 12)},
			Hauling:  &stubAdapter{err: commercemission.ErrAuthorityUnavailable},
		},
		Enabled: map[Authority]bool{Dispatch: true, Taxi: true, Hauling: true},
		Now:     func() time.Time { return time.Unix(1_700_000_100, 0).UTC() },
	})
	states := map[Authority]Readiness{}
	for _, state := range registry.ReadinessAll(context.Background()) {
		states[state.Authority] = state
	}
	if len(states) != 3 {
		t.Fatalf("readiness must cover every platform authority, got %d", len(states))
	}
	if !states[Dispatch].Reachable || states[Dispatch].ReasonCode != ReasonReady {
		t.Fatalf("dispatch readiness: %+v", states[Dispatch])
	}
	if states[Taxi].Configured || states[Taxi].ReasonCode != ReasonNotConfigured {
		t.Fatalf("an enabled authority with no adapter must report NOT_CONFIGURED: %+v", states[Taxi])
	}
	if states[Hauling].Reachable || states[Hauling].ReasonCode != ReasonUnreachable {
		t.Fatalf("an authority whose capability query fails must report UNREACHABLE: %+v", states[Hauling])
	}
	if states[Hauling].CheckedAt.IsZero() {
		t.Fatal("a probed authority must record when it was checked")
	}
}

func TestUnavailableAuthorityIsNotServiceable(t *testing.T) {
	down := multiPickupCapability(Dispatch, 12)
	down.Available = false
	down.UnavailableReason = "MAINTENANCE"
	registry := NewRegistry(RegistryOptions{
		Adapters: map[Authority]CapabilityAdapter{Dispatch: &stubAdapter{capability: down}},
		Enabled:  map[Authority]bool{Dispatch: true},
	})
	if err := registry.Authorize(context.Background(), Dispatch, Requirement{PickupCount: 1, DropoffCount: 1}); err == nil {
		t.Fatal("an authority that declares itself unavailable must not be authorized")
	}
	state := registry.Readiness(context.Background(), Dispatch)
	if state.Reachable || state.ReasonCode != "MAINTENANCE" {
		t.Fatalf("readiness must carry the authority's own reason: %+v", state)
	}
}

// -----------------------------------------------------------------------------
// VND-010, VND-012 mission intent
// -----------------------------------------------------------------------------

func validIntent() MissionIntent {
	return MissionIntent{
		CommerceOrderRef: "order-1", MissionRef: "mission-1",
		Authority: Hauling, SelectedBy: SelectedAtCheckout, SourceVersion: 3,
		Pickups:              []Location{{Ref: "stop-1", Address: "12 Marina", Latitude: 6.45, Longitude: 3.39}},
		Delivery:             Location{Ref: "drop-1", Address: "5 Awolowo Road", Latitude: 6.43, Longitude: 3.42},
		Cargo:                CargoMetadata{WeightGrams: 4000, PackageCount: 2, VehicleClass: "VAN"},
		SecurePickupRequired: true, SecureDeliveryRequired: true,
		CorrelationID: "corr-1",
	}
}

func TestMissionIntentValidation(t *testing.T) {
	if err := validIntent().Validate(); err != nil {
		t.Fatalf("a complete intent must validate: %v", err)
	}
	mutations := map[string]func(*MissionIntent){
		"no order":       func(m *MissionIntent) { m.CommerceOrderRef = "" },
		"no mission":     func(m *MissionIntent) { m.MissionRef = "" },
		"vendor own":     func(m *MissionIntent) { m.Authority = VendorOwn },
		"no source":      func(m *MissionIntent) { m.SelectedBy = "" },
		"no pickup":      func(m *MissionIntent) { m.Pickups = nil },
		"no delivery":    func(m *MissionIntent) { m.Delivery = Location{} },
		"no correlation": func(m *MissionIntent) { m.CorrelationID = "" },
	}
	for name, mutate := range mutations {
		intent := validIntent()
		mutate(&intent)
		if err := intent.Validate(); !errors.Is(err, ErrInvalidIntent) {
			t.Fatalf("%s: expected an invalid intent, got %v", name, err)
		}
	}
}

func TestIdempotencyIdentityIsDeterministicAndReplanAware(t *testing.T) {
	first := validIntent()
	second := validIntent()
	if first.IdempotencyKey() != second.IdempotencyKey() {
		t.Fatal("the same intent must produce the same execution identity, or a retry creates a second vehicle (VND-012, Scenario U)")
	}
	if first.Fingerprint() != second.Fingerprint() {
		t.Fatal("the same intent must fingerprint identically")
	}

	replan := validIntent()
	replan.SourceVersion = 4
	if replan.IdempotencyKey() == first.IdempotencyKey() {
		t.Fatal("an explicit replan must not reuse the original execution identity")
	}

	otherAuthority := validIntent()
	otherAuthority.Authority = Dispatch
	if otherAuthority.IdempotencyKey() == first.IdempotencyKey() {
		t.Fatal("the authority must participate in the execution identity")
	}

	changed := validIntent()
	changed.Cargo.WeightGrams = 9000
	if changed.Fingerprint() == first.Fingerprint() {
		t.Fatal("a changed load must change the fingerprint: a retry that quietly asks for something else is a conflict, not a duplicate")
	}
	if changed.IdempotencyKey() != first.IdempotencyKey() {
		t.Fatal("the identity is the mission, not its contents; the fingerprint is what detects the change")
	}
}

func TestRequirementIsDerivedFromTheActualPickups(t *testing.T) {
	intent := validIntent()
	intent.Pickups = []Location{{Address: "a"}, {Address: "b"}, {Address: "c"}}
	requirement := intent.Requirement()
	if requirement.PickupCount != 3 {
		t.Fatalf("pickup count must come from the pickups themselves, got %d", requirement.PickupCount)
	}
	if requirement.DropoffCount != 1 {
		t.Fatalf("commerce missions have exactly one delivery, got %d", requirement.DropoffCount)
	}
	if !requirement.SecurePickup || !requirement.SecureDelivery {
		t.Fatal("protected-handover requirements must reach the capability gate")
	}
}
