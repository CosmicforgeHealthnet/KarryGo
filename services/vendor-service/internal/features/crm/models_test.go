package crm

import (
	"testing"
	"time"
)

// TestMarketingEligibilityFailsClosed is the single most important unit test in
// Section 8A: every path that is not an explicit, fresh, unobstructed ALLOW
// must be ineligible.
func TestMarketingEligibilityFailsClosed(t *testing.T) {
	base := MarketingEligibilityInput{
		PrivacyState:                PrivacyActive,
		MergeState:                  MergeNone,
		Status:                      StatusActive,
		Decision:                    ConsentAllow,
		ConsentIntegrationAvailable: true,
	}

	cases := []struct {
		name     string
		mutate   func(*MarketingEligibilityInput)
		eligible bool
		reason   string
	}{
		{"explicit allow is the only eligible case", func(*MarketingEligibilityInput) {}, true, ReasonConsentAllowed},
		{"missing decision is unknown, not allow", func(in *MarketingEligibilityInput) { in.Decision = "" }, false, ReasonConsentUnknown},
		{"unknown is not allow", func(in *MarketingEligibilityInput) { in.Decision = ConsentUnknown }, false, ReasonConsentUnknown},
		{"deny", func(in *MarketingEligibilityInput) { in.Decision = ConsentDeny }, false, ReasonConsentDenied},
		{"stale allow", func(in *MarketingEligibilityInput) { in.ConsentStale = true }, false, ReasonConsentStale},
		{"no consent authority wired", func(in *MarketingEligibilityInput) { in.ConsentIntegrationAvailable = false }, false, ReasonConsentUnavailable},
		{"archived", func(in *MarketingEligibilityInput) { in.Status = StatusArchived }, false, ReasonProfileArchived},
		{"restricted", func(in *MarketingEligibilityInput) { in.PrivacyState = PrivacyRestricted }, false, ReasonPrivacyRestricted},
		{"marketing suppressed", func(in *MarketingEligibilityInput) { in.MarketingSuppressed = true }, false, ReasonMarketingSuppressed},
		{"merged", func(in *MarketingEligibilityInput) { in.MergeState = MergeSource }, false, ReasonProfileMerged},
		{"redacted", func(in *MarketingEligibilityInput) { in.PrivacyState = PrivacyRedacted }, false, ReasonPrivacyRedacted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			eligible, reason := MarketingEligibility(in)
			if eligible != tc.eligible || reason != tc.reason {
				t.Fatalf("eligible=%t reason=%s; want eligible=%t reason=%s", eligible, reason, tc.eligible, tc.reason)
			}
		})
	}
}

// TestPrivacyOverridesConsentAllow proves precedence: a stored ALLOW never
// survives a privacy restriction.
func TestPrivacyOverridesConsentAllow(t *testing.T) {
	for _, state := range []PrivacyState{PrivacyRedacted, PrivacyRestricted, PrivacyMerged} {
		eligible, reason := MarketingEligibility(MarketingEligibilityInput{
			PrivacyState:                state,
			Status:                      StatusActive,
			Decision:                    ConsentAllow,
			ConsentIntegrationAvailable: true,
		})
		if eligible {
			t.Fatalf("privacy state %s did not override consent ALLOW (reason=%s)", state, reason)
		}
	}
}

// TestNoVendorOverrideExists is a structural guard: the eligibility rule takes
// no "vendor override" input, so no future caller can pass one.
func TestNoVendorOverrideExists(t *testing.T) {
	eligible, reason := MarketingEligibility(MarketingEligibilityInput{
		PrivacyState:                PrivacyActive,
		Status:                      StatusActive,
		Decision:                    ConsentUnknown,
		ConsentIntegrationAvailable: true,
	})
	if eligible || reason != ReasonConsentUnknown {
		t.Fatalf("UNKNOWN consent became eligible=%t reason=%s", eligible, reason)
	}
}

func TestConsentExpiryDowngradesToUnknown(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	projection := ConsentProjection{Decision: ConsentAllow, ExpiresAt: &expired}
	if got := projection.EffectiveDecision(now); got != ConsentUnknown {
		t.Fatalf("expired ALLOW returned %s; want UNKNOWN", got)
	}
	future := now.Add(time.Hour)
	live := ConsentProjection{Decision: ConsentAllow, ExpiresAt: &future}
	if got := live.EffectiveDecision(now); got != ConsentAllow {
		t.Fatalf("live ALLOW returned %s; want ALLOW", got)
	}
}

func TestValidateSubjectReferences(t *testing.T) {
	ref := func(v string) *string { return &v }
	cases := []struct {
		name        string
		subjectType SubjectType
		customer    *string
		guest       *string
		external    *string
		valid       bool
	}{
		{"customer with customer ref", SubjectCustomer, ref("cus_1"), nil, nil, true},
		{"customer carrying a guest ref", SubjectCustomer, ref("cus_1"), ref("gst_1"), nil, false},
		{"customer without a ref", SubjectCustomer, nil, nil, nil, false},
		{"guest with guest ref", SubjectGuest, nil, ref("gst_1"), nil, true},
		{"guest linked to a customer", SubjectGuest, ref("cus_1"), ref("gst_1"), nil, true},
		{"guest without a ref", SubjectGuest, nil, nil, nil, false},
		{"external with external ref", SubjectExternal, nil, nil, ref("ext_1"), true},
		{"external carrying a guest ref", SubjectExternal, nil, ref("gst_1"), ref("ext_1"), false},
		{"blank ref is not a ref", SubjectCustomer, ref("   "), nil, nil, false},
		{"unknown subject type", SubjectType("PARTNER"), ref("x"), nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateSubjectReferences(tc.subjectType, tc.customer, tc.guest, tc.external); got != tc.valid {
				t.Fatalf("got %t want %t", got, tc.valid)
			}
		})
	}
}

// TestComputeCommercialTotalsIgnoresNonCompleted proves cancelled, refunded and
// non-terminal facts are retained as evidence but never inflate a total.
func TestComputeCommercialTotalsIgnoresNonCompleted(t *testing.T) {
	at := func(day int) *time.Time {
		v := time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
		return &v
	}
	branch := "brn_1"
	facts := []OrderFact{
		{SourceOrderRef: "o1", OrderState: OrderCompleted, CompletedAt: at(1), Currency: "NGN", GrossMinor: 10_000, NetMinor: 10_000, BranchID: &branch},
		{SourceOrderRef: "o2", OrderState: OrderCompleted, CompletedAt: at(5), Currency: "NGN", GrossMinor: 30_000, NetMinor: 20_000, RefundedMinor: 10_000, BranchID: &branch},
		{SourceOrderRef: "o3", OrderState: OrderCancelled, Currency: "NGN", GrossMinor: 99_000, NetMinor: 99_000},
		{SourceOrderRef: "o4", OrderState: OrderRefunded, Currency: "NGN", GrossMinor: 55_000, NetMinor: 0, RefundedMinor: 55_000},
		{SourceOrderRef: "o5", OrderState: OrderUpdated, Currency: "NGN", GrossMinor: 77_000, NetMinor: 77_000},
	}

	totals := ComputeCommercialTotals(facts)
	if totals.CompletedOrderCount != 2 {
		t.Fatalf("completed count=%d want 2", totals.CompletedOrderCount)
	}
	if totals.GrossSpendMinor != 40_000 {
		t.Fatalf("gross=%d want 40000", totals.GrossSpendMinor)
	}
	if totals.NetSpendMinor != 30_000 {
		t.Fatalf("net=%d want 30000 (refund reduces net, does not add a second order)", totals.NetSpendMinor)
	}
	if totals.AverageOrderValueMinor != 15_000 {
		t.Fatalf("aov=%d want 15000", totals.AverageOrderValueMinor)
	}
	if totals.FirstCompletedOrderAt == nil || !totals.FirstCompletedOrderAt.Equal(*at(1)) {
		t.Fatalf("first completed = %v want Jan 1", totals.FirstCompletedOrderAt)
	}
	if totals.LastCompletedOrderAt == nil || !totals.LastCompletedOrderAt.Equal(*at(5)) {
		t.Fatalf("last completed = %v want Jan 5", totals.LastCompletedOrderAt)
	}
	if totals.LastOrderRef == nil || *totals.LastOrderRef != "o2" {
		t.Fatalf("last order ref = %v want o2", totals.LastOrderRef)
	}
}

// TestComputeCommercialTotalsIsIdempotent proves recomputation from the same
// fact set never drifts — the property that makes replay and merge safe.
func TestComputeCommercialTotalsIsIdempotent(t *testing.T) {
	completed := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	facts := []OrderFact{
		{SourceOrderRef: "o1", OrderState: OrderCompleted, CompletedAt: &completed, Currency: "NGN", GrossMinor: 1_000, NetMinor: 1_000},
	}
	first := ComputeCommercialTotals(facts)
	second := ComputeCommercialTotals(facts)
	third := ComputeCommercialTotals(append(facts[:0:0], facts...))
	if first.NetSpendMinor != second.NetSpendMinor || second.NetSpendMinor != third.NetSpendMinor {
		t.Fatalf("recomputation drifted: %d %d %d", first.NetSpendMinor, second.NetSpendMinor, third.NetSpendMinor)
	}
	if first.CompletedOrderCount != 1 {
		t.Fatalf("count=%d want 1", first.CompletedOrderCount)
	}
}

func TestAverageOrderValueUsesIntegerArithmetic(t *testing.T) {
	if got := AverageOrderValue(0, 0); got != 0 {
		t.Fatalf("no orders should give 0, got %d", got)
	}
	// 10_000 / 3 truncates rather than producing a float.
	if got := AverageOrderValue(10_000, 3); got != 3_333 {
		t.Fatalf("aov=%d want 3333", got)
	}
}

func TestPreferredBranchIsDeterministic(t *testing.T) {
	early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Highest completed-order count wins outright.
	got := PreferredBranch([]BranchRelation{
		{BranchID: "brn_a", CompletedOrderCount: 2, LastRelationshipAt: &late},
		{BranchID: "brn_b", CompletedOrderCount: 5, LastRelationshipAt: &early},
	})
	if got == nil || *got != "brn_b" {
		t.Fatalf("got %v want brn_b", got)
	}

	// Equal counts fall back to recency.
	got = PreferredBranch([]BranchRelation{
		{BranchID: "brn_a", CompletedOrderCount: 3, LastRelationshipAt: &early},
		{BranchID: "brn_b", CompletedOrderCount: 3, LastRelationshipAt: &late},
	})
	if got == nil || *got != "brn_b" {
		t.Fatalf("got %v want brn_b on recency", got)
	}

	// Equal counts and equal recency fall back to a stable id tie-break.
	got = PreferredBranch([]BranchRelation{
		{BranchID: "brn_z", CompletedOrderCount: 3, LastRelationshipAt: &late},
		{BranchID: "brn_a", CompletedOrderCount: 3, LastRelationshipAt: &late},
	})
	if got == nil || *got != "brn_a" {
		t.Fatalf("got %v want brn_a on stable tie-break", got)
	}

	// A branch with no completed orders is not a preference.
	if got = PreferredBranch([]BranchRelation{{BranchID: "brn_a"}}); got != nil {
		t.Fatalf("got %v want nil", got)
	}
}

// TestPrivacyStateForNeverDowngradesRedaction is the out-of-order privacy race
// in pure form: a RESTRICT arriving after a REDACT must not weaken it.
func TestPrivacyStateForNeverDowngradesRedaction(t *testing.T) {
	if got := PrivacyStateFor(PrivacyActionRestrict, PrivacyRedacted); got != PrivacyRedacted {
		t.Fatalf("RESTRICT downgraded REDACTED to %s", got)
	}
	if got := PrivacyStateFor(PrivacyActionRestrict, PrivacyMerged); got != PrivacyMerged {
		t.Fatalf("RESTRICT overwrote MERGED with %s", got)
	}
	if got := PrivacyStateFor(PrivacyActionRestrict, PrivacyActive); got != PrivacyRestricted {
		t.Fatalf("RESTRICT on ACTIVE gave %s", got)
	}
	if got := PrivacyStateFor(PrivacyActionRedact, PrivacyRestricted); got != PrivacyRedacted {
		t.Fatalf("REDACT on RESTRICTED gave %s", got)
	}
	// Marketing suppression must not masquerade as a state change.
	if got := PrivacyStateFor(PrivacyActionMarketingSuppress, PrivacyActive); got != PrivacyActive {
		t.Fatalf("MARKETING_SUPPRESS changed state to %s", got)
	}
}

func TestProfileMutationGuards(t *testing.T) {
	redacted := Profile{PrivacyState: PrivacyRedacted}
	if redacted.AcceptsOrdinaryMutation() {
		t.Fatal("redacted profile accepted ordinary mutation")
	}
	merged := Profile{PrivacyState: PrivacyMerged, MergeState: MergeSource}
	if merged.AcceptsOrdinaryMutation() {
		t.Fatal("merged-away profile accepted ordinary mutation")
	}
	if !(Profile{PrivacyState: PrivacyActive, MergeState: MergeNone}).AcceptsOrdinaryMutation() {
		t.Fatal("active profile rejected ordinary mutation")
	}
}

func TestLinkTypeForRejectsUnsupportedCombinations(t *testing.T) {
	if _, ok := LinkTypeFor(SubjectGuest, SubjectCustomer); !ok {
		t.Fatal("guest to customer should be supported")
	}
	if _, ok := LinkTypeFor(SubjectCustomer, SubjectGuest); ok {
		t.Fatal("customer to guest must not be coerced into a supported type")
	}
	if _, ok := LinkTypeFor(SubjectExternal, SubjectGuest); ok {
		t.Fatal("external to guest must not be supported")
	}
}

func TestRecencyAndFrequencyStayNilWithoutData(t *testing.T) {
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	if days, freq := recencyAndFrequency(nil, nil, nil, now); days != nil || freq != nil {
		t.Fatalf("derived recency/frequency from nothing: %v %v", days, freq)
	}
	zero := 0
	if days, freq := recencyAndFrequency(nil, &now, &zero, now); days != nil || freq != nil {
		t.Fatalf("derived recency/frequency from zero orders: %v %v", days, freq)
	}
}
