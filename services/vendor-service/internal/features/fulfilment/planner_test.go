package fulfilment

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/shared/go/commercequote"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeLogistics struct {
	profiles map[string]LineProfile
	err      error
}

func (f fakeLogistics) ResolveLineProfiles(
	_ context.Context, _ string, lines []LineLogistics,
) (map[string]LineProfile, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := map[string]LineProfile{}

	for _, line := range lines {
		if profile, ok := f.profiles[line.CheckoutLineID]; ok {
			out[line.CheckoutLineID] = profile
		}
	}

	return out, nil
}

type fakeBranches struct {
	branches map[string]Branch
	err      error
}

func (f fakeBranches) ResolveBranch(
	_ context.Context, businessID, branchID string,
) (Branch, bool, error) {
	if f.err != nil {
		return Branch{}, false, f.err
	}

	// Scoped by BOTH ids, mirroring the real adapter: a branch borrowed from
	// another business must not resolve.
	branch, ok := f.branches[businessID+"|"+branchID]

	return branch, ok, nil
}

type fakeVendorOwn struct {
	enabled  bool
	policies map[string]delivery.Policy
}

func (f fakeVendorOwn) PlatformEnabled() bool { return f.enabled }

func (f fakeVendorOwn) ResolvePolicy(
	_ context.Context, businessID, branchID string,
) (delivery.Policy, bool, error) {
	policy, ok := f.policies[businessID+"|"+branchID]

	return policy, ok, nil
}

type fakeAuthority struct {
	available bool
	quote     commercequote.Quote
	err       error

	// resolve answers per ROUTE rather than with one fixed quote.
	//
	// Without it every route costs the same, and a mode test cannot tell
	// "SAVER chose the cheaper plan" apart from "SAVER chose the only plan".
	// Both would pass, and only one would mean anything.
	resolve func(commercequote.Request) commercequote.Quote

	requests []commercequote.Request
}

func (f *fakeAuthority) Available() bool { return f.available }

func (f *fakeAuthority) RequestQuote(
	_ context.Context, request commercequote.Request,
) (commercequote.Quote, error) {
	f.requests = append(f.requests, request)

	if f.err != nil {
		return commercequote.Quote{}, f.err
	}

	quote := f.quote

	if f.resolve != nil {
		quote = f.resolve(request)
	}

	quote.QuoteReference = request.QuoteReference
	quote.VehicleClass = request.VehicleClass

	if quote.Serviceable && quote.ExpiresAt.IsZero() {
		quote.ExpiresAt = time.Now().Add(10 * time.Minute)
	}

	return quote, nil
}

func serviceableQuote(id string, fee int64) commercequote.Quote {
	return commercequote.Quote{
		QuoteID:            id,
		Revision:           1,
		Serviceable:        true,
		ReasonCode:         commercequote.ReasonServiceable,
		PickupRef:          id + ":pickup",
		DropoffRef:         id + ":dropoff",
		DistanceKm:         4.9,
		EstimatedMinutes:   26,
		FeeMinor:           fee,
		Currency:           "NGN",
		AvailableProviders: 2,
	}
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

const (
	branchLat = 6.4550
	branchLng = 3.4100

	dropLat = 6.4300
	dropLng = 3.4500
)

func activeBranch(lat, lng float64) Branch {
	return Branch{
		Status:                "ACTIVE",
		Latitude:              lat,
		Longitude:             lng,
		HasServiceArea:        true,
		AreaCenterLatitude:    lat,
		AreaCenterLongitude:   lng,
		AreaRadiusMeters:      15000,
		AcceptingNewOrders:    true,
		HasOperationalSetting: true,
	}
}

type harness struct {
	logistics fakeLogistics
	branches  fakeBranches
	vendorOwn fakeVendorOwn
	dispatch  *fakeAuthority
	hauling   *fakeAuthority

	// multiPickup declares which runtimes can drive a multi-stop collection.
	//
	// Nil means none can, which is production truth today. A test that wants
	// the future behaviour sets it explicitly rather than the planner
	// guessing.
	multiPickup map[Service]bool
}

func newHarness() *harness {
	return &harness{
		logistics: fakeLogistics{profiles: map[string]LineProfile{
			"l1": {Known: true, WeightGrams: ptr64(2000)},
		}},
		branches: fakeBranches{branches: map[string]Branch{
			"bizA|brA": activeBranch(branchLat, branchLng),
		}},
		vendorOwn: fakeVendorOwn{policies: map[string]delivery.Policy{}},
		dispatch: &fakeAuthority{
			available: true, quote: serviceableQuote("dq_1", 150000),
		},
		hauling: &fakeAuthority{
			available: true, quote: serviceableQuote("hq_1", 700000),
		},
	}
}

func (h *harness) planner(t *testing.T) *Planner {
	t.Helper()

	planner, err := NewPlanner(Options{
		Logistics:   h.logistics,
		Branches:    h.branches,
		VendorOwn:   h.vendorOwn,
		Dispatch:    h.dispatch,
		Hauling:     h.hauling,
		MultiPickup: h.multiPickup,
	})
	if err != nil {
		t.Fatal(err)
	}

	return planner
}

func ptr64(v int64) *int64 { return &v }

func request(groups ...GroupInput) PlanRequest {
	return PlanRequest{
		CustomerID:                 "cust_1",
		CheckoutSessionID:          "cs_1",
		CheckoutValidationRevision: 1,
		Currency:                   "NGN",
		Destination: Destination{
			Latitude: dropLat, Longitude: dropLng, Address: "1 Test Street",
		},
		Groups: groups,
	}
}

func groupA() GroupInput {
	return GroupInput{
		CheckoutGroupID: "g1",
		BusinessID:      "bizA",
		BranchID:        "brA",
		OrderValueMinor: 500000,
		Currency:        "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l1", Domain: "SHOP",
			BusinessID: "bizA", BranchID: "brA", Quantity: 1,
		}},
	}
}

// ---------------------------------------------------------------------------
// Platform logistics
// ---------------------------------------------------------------------------

func TestPlan_PlatformDispatchUsesTheAuthorityFee(t *testing.T) {
	h := newHarness()

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %q, want PLANNED (groups: %+v)",
			plan.Status, plan.Groups)
	}

	if len(plan.Missions) != 1 {
		t.Fatalf("missions = %d, want 1", len(plan.Missions))
	}

	mission := plan.Missions[0]

	if mission.Service != ServiceDispatch ||
		mission.Authority != AuthorityPlatform {
		t.Fatalf("unexpected mission: %+v", mission)
	}

	// The fee is the authority's, carried verbatim.
	if mission.Funding.GrossMinor != 150000 {
		t.Fatalf("fee = %d, want the authority's 150000",
			mission.Funding.GrossMinor)
	}

	if mission.Quote.QuoteID != "dq_1" {
		t.Fatalf("quote id = %q, want the authority's", mission.Quote.QuoteID)
	}

	// Platform delivery money never lands on the vendor.
	if mission.FeeBeneficiary != BeneficiaryPlatformProvider {
		t.Fatalf("beneficiary = %q, want PLATFORM_PROVIDER",
			mission.FeeBeneficiary)
	}

	if !mission.Funding.Balanced() || !plan.Funding.Balanced() {
		t.Fatal("funding does not balance")
	}

	// The mission must name the groups it covers.
	if len(mission.GroupIDs) != 1 || mission.GroupIDs[0] != "g1" {
		t.Fatalf("group ids = %v", mission.GroupIDs)
	}
}

func TestPlan_HeavyLoadGoesToHauling(t *testing.T) {
	h := newHarness()
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(300000),
	}

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %q: %+v", plan.Status, plan.Groups)
	}

	if plan.Missions[0].Service != ServiceHauling {
		t.Fatalf("service = %q, want HAULING", plan.Missions[0].Service)
	}

	if len(h.dispatch.requests) != 0 {
		t.Fatal("dispatch must not be asked to quote a 300kg load")
	}
}

// The plan carries the authority's real load, not a placeholder.
func TestPlan_QuoteRequestCarriesTheRealLoad(t *testing.T) {
	h := newHarness()
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(2000), VolumeMl: ptr64(1500),
		TemperatureClass: "HOT",
	}

	if _, _, err := h.planner(t).Build(
		context.Background(), request(groupA()),
	); err != nil {
		t.Fatal(err)
	}

	if len(h.dispatch.requests) != 1 {
		t.Fatalf("expected one dispatch quote request, got %d",
			len(h.dispatch.requests))
	}

	sent := h.dispatch.requests[0]

	if sent.Load.TotalWeightGrams != 2000 || sent.Load.PackageCount != 1 {
		t.Fatalf("load not carried: %+v", sent.Load)
	}

	if sent.Urgency != commercequote.Urgency(UrgencyImmediate) {
		t.Fatalf("urgency = %q, want IMMEDIATE for hot food", sent.Urgency)
	}

	if len(sent.Load.Capabilities) == 0 {
		t.Fatal("handling capabilities were not carried to the authority")
	}
}

// ---------------------------------------------------------------------------
// Vendor own delivery
// ---------------------------------------------------------------------------

func vendorPolicy() delivery.Policy {
	return delivery.Policy{
		ID: "vdp_1", BusinessID: "bizA", BranchID: "brA",
		Enabled: true, Currency: "NGN",
		BaseFeeMinor: 50000, PerKmFeeMinor: 10000,
		EstimatedMinutesBase: 20, Version: 3,
	}
}

func TestPlan_VendorOwnDeliveryCreatesNoPlatformMission(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true
	h.vendorOwn.policies["bizA|brA"] = vendorPolicy()

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %q: %+v", plan.Status, plan.Groups)
	}

	mission := plan.Missions[0]

	if mission.Authority != AuthorityVendorOwn ||
		mission.Service != ServiceVendorOwn {
		t.Fatalf("unexpected mission: %+v", mission)
	}

	// The whole point: no platform mission, no platform rider, no quote call.
	if len(h.dispatch.requests) != 0 || len(h.hauling.requests) != 0 {
		t.Fatal("vendor own delivery must not call a platform authority")
	}

	if mission.Quote.QuoteID != "" {
		t.Fatal("vendor own delivery must not claim a platform quote")
	}

	// Vendor-owned delivery is the one case where the vendor is owed the fee.
	if mission.FeeBeneficiary != BeneficiaryVendor {
		t.Fatalf("beneficiary = %q, want VENDOR", mission.FeeBeneficiary)
	}

	if mission.Funding.GrossMinor <= 0 || !mission.Funding.Balanced() {
		t.Fatalf("unexpected funding: %+v", mission.Funding)
	}
}

// A vendor free-delivery threshold is vendor-funded: the delivery still costs
// what it costs.
func TestPlan_VendorFreeDeliveryIsVendorFunded(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true

	policy := vendorPolicy()
	threshold := int64(400000)
	policy.FreeDeliveryThresholdMinor = &threshold
	h.vendorOwn.policies["bizA|brA"] = policy

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	mission := plan.Missions[0]

	if mission.Funding.CustomerMinor != 0 {
		t.Fatalf("customer = %d, want 0", mission.Funding.CustomerMinor)
	}

	if mission.Funding.VendorFundedMinor != mission.Funding.GrossMinor ||
		mission.Funding.GrossMinor == 0 {
		t.Fatalf("free delivery must still record its cost: %+v",
			mission.Funding)
	}
}

// A vendor whose own delivery refuses falls through to platform logistics.
// That is the whole reason the two authorities are independent.
func TestPlan_VendorOwnRefusalFallsThroughToPlatform(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true

	policy := vendorPolicy()
	policy.MinimumOrderMinor = 9000000 // far above the order value
	h.vendorOwn.policies["bizA|brA"] = policy

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %q: %+v", plan.Status, plan.Groups)
	}

	if plan.Missions[0].Authority != AuthorityPlatform {
		t.Fatalf("expected a platform fallback, got %+v", plan.Missions[0])
	}
}

// A multi-vendor order may genuinely mix authorities.
func TestPlan_MixedVendorAndPlatformDelivery(t *testing.T) {
	h := newHarness()
	h.vendorOwn.enabled = true
	h.vendorOwn.policies["bizA|brA"] = vendorPolicy()

	h.branches.branches["bizB|brB"] = activeBranch(6.5244, 3.3792)
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(1500),
	}

	groupB := GroupInput{
		CheckoutGroupID: "g2",
		BusinessID:      "bizB",
		BranchID:        "brB",
		OrderValueMinor: 300000,
		Currency:        "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP",
			BusinessID: "bizB", BranchID: "brB", Quantity: 1,
		}},
	}

	plan, _, err := h.planner(t).Build(
		context.Background(), request(groupA(), groupB),
	)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %q: %+v", plan.Status, plan.Groups)
	}

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2", len(plan.Missions))
	}

	authorities := map[Authority]bool{}

	for _, mission := range plan.Missions {
		authorities[mission.Authority] = true
	}

	if !authorities[AuthorityVendorOwn] || !authorities[AuthorityPlatform] {
		t.Fatalf("expected one of each authority, got %v", authorities)
	}
}

// ---------------------------------------------------------------------------
// Consolidation
// ---------------------------------------------------------------------------

func TestPlan_CoLocatedVendorsConsolidateIntoOneMission(t *testing.T) {
	h := newHarness()

	// Same market: ~50 m apart.
	h.branches.branches["bizB|brB"] = activeBranch(6.45505, 3.41030)
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(1500),
	}

	groupB := GroupInput{
		CheckoutGroupID: "g2", BusinessID: "bizB", BranchID: "brB",
		OrderValueMinor: 300000, Currency: "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP",
			BusinessID: "bizB", BranchID: "brB", Quantity: 1,
		}},
	}

	plan, _, err := h.planner(t).Build(
		context.Background(), request(groupA(), groupB),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Missions) != 1 {
		t.Fatalf("missions = %d, want 1 consolidated", len(plan.Missions))
	}

	mission := plan.Missions[0]

	if !mission.Consolidated || len(mission.GroupIDs) != 2 {
		t.Fatalf("mission not recorded as consolidated: %+v", mission)
	}

	// The consolidated mission is quoted ONCE for its real combined load, not
	// assembled from two per-vendor quotes discounted afterwards.
	//
	// The split alternative is quoted too, and that is the point rather than
	// an accident: the optimizer compares two priced plans, so each vendor is
	// also asked what its own separate mission would cost. Three questions,
	// three distinct routes, no fee invented for any of them. Asserting a
	// single quote here would be asserting that the alternative was never
	// evaluated.
	var combinedRequests, singleRequests int

	for _, quoted := range h.dispatch.requests {
		switch quoted.Load.TotalWeightGrams {
		case 3500:
			combinedRequests++
		case 2000, 1500:
			singleRequests++
		default:
			t.Fatalf("unexpected quoted load: %d",
				quoted.Load.TotalWeightGrams)
		}
	}

	if combinedRequests != 1 {
		t.Fatalf("combined-load quotes = %d, want exactly 1", combinedRequests)
	}

	if singleRequests != 2 {
		t.Fatalf("split-alternative quotes = %d, want 2", singleRequests)
	}

	// And exactly one delivery fee, attributed back across both groups.
	var attributed Funding

	for _, group := range plan.Groups {
		attributed = attributed.Add(group.Funding)

		if !group.Funding.Balanced() {
			t.Fatalf("group share does not balance: %+v", group)
		}
	}

	if attributed.GrossMinor != mission.Funding.GrossMinor {
		t.Fatalf("attributed %d != mission %d",
			attributed.GrossMinor, mission.Funding.GrossMinor)
	}
}

// Distant vendors must not be consolidated: one mission has one pickup, so a
// combined fee there would be a fee for a route nobody can drive.
func TestPlan_DistantVendorsAreNotConsolidated(t *testing.T) {
	h := newHarness()

	h.branches.branches["bizB|brB"] = activeBranch(6.6018, 3.3515) // ~17 km
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(1500),
	}

	groupB := GroupInput{
		CheckoutGroupID: "g2", BusinessID: "bizB", BranchID: "brB",
		OrderValueMinor: 300000, Currency: "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP",
			BusinessID: "bizB", BranchID: "brB", Quantity: 1,
		}},
	}

	plan, _, err := h.planner(t).Build(
		context.Background(), request(groupA(), groupB),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2 separate", len(plan.Missions))
	}

	for _, mission := range plan.Missions {
		if mission.Consolidated {
			t.Fatal("distant pickups must not consolidate")
		}
	}
}

// An immediate group must not be merged with a standard one.
func TestPlan_UrgencyBlocksConsolidation(t *testing.T) {
	h := newHarness()

	h.branches.branches["bizB|brB"] = activeBranch(6.45505, 3.41030)
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(1500), TemperatureClass: "HOT",
	}

	groupB := GroupInput{
		CheckoutGroupID: "g2", BusinessID: "bizB", BranchID: "brB",
		OrderValueMinor: 300000, Currency: "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP",
			BusinessID: "bizB", BranchID: "brB", Quantity: 1,
		}},
	}

	plan, _, err := h.planner(t).Build(
		context.Background(), request(groupA(), groupB),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2 — urgency must not be merged away",
			len(plan.Missions))
	}
}

// ---------------------------------------------------------------------------
// Serviceability failure
// ---------------------------------------------------------------------------

func TestPlan_AuthorityRefusalMakesTheGroupUnserviceable(t *testing.T) {
	h := newHarness()
	h.dispatch.quote = commercequote.Quote{
		Serviceable: false,
		ReasonCode:  commercequote.ReasonNoProviderAvailable,
	}
	h.hauling.available = false

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	// Not silently dropped: the group is present with the authority's own
	// reason.
	if len(plan.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(plan.Groups))
	}

	group := plan.Groups[0]

	if group.Serviceable ||
		group.ReasonCode != commercequote.ReasonNoProviderAvailable {
		t.Fatalf("unexpected group: %+v", group)
	}

	if group.Funding.GrossMinor != 0 || group.MissionID != "" {
		t.Fatalf("an unserviceable group carried a charge: %+v", group)
	}

	if len(plan.Missions) != 0 {
		t.Fatal("an unserviceable plan must create no mission")
	}
}

// An authority fault must never become a cheaper delivery.
func TestPlan_AuthorityFaultDoesNotProduceAFee(t *testing.T) {
	h := newHarness()
	h.dispatch.err = errors.New("timeout")
	h.hauling.available = false

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	if plan.Groups[0].ReasonCode != ReasonAuthorityUnavailable {
		t.Fatalf("reason = %q", plan.Groups[0].ReasonCode)
	}

	if plan.Funding.GrossMinor != 0 {
		t.Fatalf("a fault produced a fee: %+v", plan.Funding)
	}
}

// An already-lapsed quote is not usable evidence.
func TestPlan_ExpiredQuoteIsRefused(t *testing.T) {
	h := newHarness()

	quote := serviceableQuote("dq_old", 150000)
	quote.ExpiresAt = time.Now().Add(-time.Minute)
	h.dispatch.quote = quote
	h.hauling.available = false

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}
}

// One unserviceable group must not silently drop it from the plan, and must
// not let the rest be presented as a complete order.
func TestPlan_OneBadGroupIsReportedNotDropped(t *testing.T) {
	h := newHarness()

	// bizB's branch is deliberately absent from the branch authority.
	groupB := GroupInput{
		CheckoutGroupID: "g2", BusinessID: "bizB", BranchID: "brB",
		OrderValueMinor: 300000, Currency: "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP", Quantity: 1,
		}},
	}

	plan, _, err := h.planner(t).Build(
		context.Background(), request(groupA(), groupB),
	)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want both reported", len(plan.Groups))
	}

	refused := plan.Unserviceable()

	if len(refused) != 1 || refused[0].CheckoutGroupID != "g2" {
		t.Fatalf("unexpected refusals: %+v", refused)
	}

	if refused[0].ReasonCode != ReasonBranchUnknown {
		t.Fatalf("reason = %q, want %q",
			refused[0].ReasonCode, ReasonBranchUnknown)
	}
}

func TestPlan_InactiveBranchRefuses(t *testing.T) {
	h := newHarness()

	branch := activeBranch(branchLat, branchLng)
	branch.Status = "PAUSED"
	h.branches.branches["bizA|brA"] = branch

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Groups[0].ReasonCode != ReasonBranchInactive {
		t.Fatalf("reason = %q", plan.Groups[0].ReasonCode)
	}
}

func TestPlan_BranchNotAcceptingOrdersRefuses(t *testing.T) {
	h := newHarness()

	branch := activeBranch(branchLat, branchLng)
	branch.AcceptingNewOrders = false
	h.branches.branches["bizA|brA"] = branch

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Groups[0].ReasonCode != ReasonBranchNotAcceptingOrders {
		t.Fatalf("reason = %q", plan.Groups[0].ReasonCode)
	}
}

func TestPlan_MissingDeliveryAddressRefusesEveryGroup(t *testing.T) {
	h := newHarness()

	planRequest := request(groupA())
	planRequest.Destination = Destination{}

	plan, _, err := h.planner(t).Build(context.Background(), planRequest)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable ||
		plan.Groups[0].ReasonCode != ReasonNoDeliveryAddress {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

// A deployment with no logistics integration enabled and no vendor-own policy
// has no delivery option at all, and must say so.
func TestPlan_NoAuthorityConfigured(t *testing.T) {
	h := newHarness()
	h.dispatch.available = false
	h.hauling.available = false

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("status = %q, want UNSERVICEABLE", plan.Status)
	}

	if plan.Groups[0].Serviceable {
		t.Fatal("no authority can serve, so no group may be serviceable")
	}
}

// ---------------------------------------------------------------------------
// Plan expiry
// ---------------------------------------------------------------------------

// A plan may never outlive the quotes inside it.
func TestPlan_ExpiryNeverExceedsTheEarliestQuote(t *testing.T) {
	h := newHarness()

	quote := serviceableQuote("dq_1", 150000)
	quote.ExpiresAt = time.Now().Add(2 * time.Minute)
	h.dispatch.quote = quote

	planner, err := NewPlanner(Options{
		Logistics: h.logistics, Branches: h.branches,
		VendorOwn: h.vendorOwn, Dispatch: h.dispatch, Hauling: h.hauling,
		PlanTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, _, err := planner.Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.ExpiresAt.After(quote.ExpiresAt) {
		t.Fatalf("plan expiry %v outlives its quote %v",
			plan.ExpiresAt, quote.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// Funding attribution
// ---------------------------------------------------------------------------

func TestShareFunding_AlwaysSumsBack(t *testing.T) {
	total := Funding{
		GrossMinor: 100003, CustomerMinor: 70003, VendorFundedMinor: 30000,
	}

	members := []planned{
		{requirement: Requirement{TotalWeightGrams: 1}},
		{requirement: Requirement{TotalWeightGrams: 2}},
		{requirement: Requirement{TotalWeightGrams: 7}},
	}

	shares := shareFunding(total, members)

	var sum Funding

	for _, share := range shares {
		if !share.Balanced() {
			t.Fatalf("share does not balance: %+v", share)
		}

		sum = sum.Add(share)
	}

	if sum.GrossMinor != total.GrossMinor ||
		sum.CustomerMinor != total.CustomerMinor ||
		sum.VendorFundedMinor != total.VendorFundedMinor {
		t.Fatalf("shares %+v do not sum to %+v", sum, total)
	}
}

func TestShareFunding_ZeroWeightSplitsEvenly(t *testing.T) {
	total := Funding{GrossMinor: 100, CustomerMinor: 100}

	shares := shareFunding(total, []planned{
		{requirement: Requirement{}}, {requirement: Requirement{}},
	})

	if shares[0].GrossMinor != 50 || shares[1].GrossMinor != 50 {
		t.Fatalf("unexpected split: %+v", shares)
	}
}

func TestNewPlanner_RequiresItsAuthorities(t *testing.T) {
	if _, err := NewPlanner(Options{}); !errors.Is(err, ErrDependencyMissing) {
		t.Fatalf("expected ErrDependencyMissing, got %v", err)
	}
}
