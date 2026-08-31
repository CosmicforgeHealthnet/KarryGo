package crm

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return parsed
}

// Experiment assignment is pure and deterministic, so it is tested without a
// database. That is the point of the design: an assignment that needed a stored
// lookup could not be reproduced by an analyst months later.

func variantSet(inputs ...ExperimentVariantInput) []ExperimentVariantInput { return inputs }

func control(bps int) ExperimentVariantInput {
	return ExperimentVariantInput{VariantKey: "CONTROL", DisplayName: "Holdout", AllocationBps: bps, IsControl: true}
}

func treatment(key string, bps int) ExperimentVariantInput {
	return ExperimentVariantInput{VariantKey: key, DisplayName: key, AllocationBps: bps}
}

func enabled() *bool { yes := true; return &yes }

func TestExperiment_AllocationMustTotalExactly10000(t *testing.T) {
	cases := []struct {
		name     string
		variants []ExperimentVariantInput
	}{
		{"under", variantSet(control(1000), treatment("A", 4000))},
		{"over", variantSet(control(1000), treatment("A", 5000), treatment("B", 5000))},
		{"negative", variantSet(control(-1000), treatment("A", 11000))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, err := validateExperiment(enabled(), testCase.variants); err == nil {
				t.Fatal("expected an allocation rejection")
			}
		})
	}

	on, variants, err := validateExperiment(enabled(), variantSet(control(1000), treatment("A", 4500), treatment("B", 4500)))
	if err != nil || !on || len(variants) != 3 {
		t.Fatalf("valid allocation rejected: on=%t variants=%d err=%v", on, len(variants), err)
	}
}

func TestExperiment_StructuralRejections(t *testing.T) {
	cases := []struct {
		name     string
		variants []ExperimentVariantInput
	}{
		{"two controls", variantSet(control(1000),
			ExperimentVariantInput{VariantKey: "CONTROL_2", AllocationBps: 1000, IsControl: true},
			treatment("A", 8000))},
		{"duplicate key", variantSet(treatment("A", 5000), treatment("A", 5000))},
		{"control over half", variantSet(control(6000), treatment("A", 4000))},
		{"no treatment", variantSet(control(10000))},
		{"invalid key", variantSet(treatment("bad key!", 10000))},
		{"too many variants", variantSet(treatment("A", 1000), treatment("B", 1000), treatment("C", 1000),
			treatment("D", 1000), treatment("E", 1000), treatment("F", 5000))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, err := validateExperiment(enabled(), testCase.variants); err == nil {
				t.Fatalf("expected %s to be rejected", testCase.name)
			}
		})
	}
}

func TestExperiment_VariantsRequireAnEnabledExperiment(t *testing.T) {
	if _, _, err := validateExperiment(nil, variantSet(treatment("A", 10000))); err == nil {
		t.Fatal("variants without an enabled experiment should be rejected")
	}
	on, variants, err := validateExperiment(nil, nil)
	if err != nil || on || variants != nil {
		t.Fatalf("no experiment should be a clean no-op: on=%t err=%v", on, err)
	}
}

// TestExperiment_AssignmentIsDeterministicAndStable pins the property the whole
// design rests on. The same campaign version and the same relationship must
// always land in the same cohort, with no runtime randomness anywhere.
func TestExperiment_AssignmentIsDeterministicAndStable(t *testing.T) {
	version := CampaignVersion{
		ID: "crmcv_fixed", ExperimentEnabled: true,
		Variants: []ExperimentVariant{
			{VariantKey: "CONTROL", AllocationBps: 1000, IsControl: true},
			{VariantKey: "A", AllocationBps: 4500},
			{VariantKey: "B", AllocationBps: 4500},
		},
	}
	allocator := newExperimentAllocator(version)

	for _, profileID := range []string{"crmp_1", "crmp_2", "crmp_3", "crmp_4", "crmp_5"} {
		first := allocator.Assign(profileID)
		for repeat := 0; repeat < 25; repeat++ {
			again := allocator.Assign(profileID)
			if again != first {
				t.Fatalf("assignment for %s is not stable: %+v then %+v", profileID, first, again)
			}
		}
		// A freshly constructed allocator must agree with the first one: there
		// is no per-process seed anywhere.
		if rebuilt := newExperimentAllocator(version).Assign(profileID); rebuilt != first {
			t.Fatalf("assignment for %s depends on allocator identity: %+v vs %+v", profileID, first, rebuilt)
		}
	}
}

// TestExperiment_BucketIsExactAndRangeAligned checks the actual arithmetic
// rather than an approximate distribution: a bucket is assigned to the cohort
// whose cumulative range contains it, and nothing else.
func TestExperiment_BucketIsExactAndRangeAligned(t *testing.T) {
	version := CampaignVersion{
		ID: "crmcv_ranges", ExperimentEnabled: true,
		Variants: []ExperimentVariant{
			{VariantKey: "CONTROL", AllocationBps: 2000, IsControl: true},
			{VariantKey: "A", AllocationBps: 3000},
			{VariantKey: "B", AllocationBps: 5000},
		},
	}
	allocator := newExperimentAllocator(version)
	// Control first, then key ascending: CONTROL [0,2000), A [2000,5000),
	// B [5000,10000).
	expected := []struct {
		upper int
		key   string
	}{{2000, "CONTROL"}, {5000, "A"}, {10000, "B"}}
	if len(allocator.ranges) != len(expected) {
		t.Fatalf("expected %d ranges, got %d", len(expected), len(allocator.ranges))
	}
	for index, want := range expected {
		got := allocator.ranges[index]
		if got.upperExclusive != want.upper || got.key != want.key {
			t.Fatalf("range %d = (%d,%s), want (%d,%s)", index, got.upperExclusive, got.key, want.upper, want.key)
		}
	}

	// Every assignment must fall in the range its bucket belongs to.
	for _, profileID := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		assignment := allocator.Assign(profileID)
		if assignment.Bucket < 0 || assignment.Bucket >= AssignmentBuckets {
			t.Fatalf("bucket out of range: %d", assignment.Bucket)
		}
		var want string
		switch {
		case assignment.Bucket < 2000:
			want = "CONTROL"
		case assignment.Bucket < 5000:
			want = "A"
		default:
			want = "B"
		}
		if assignment.VariantKey != want {
			t.Fatalf("bucket %d assigned to %s, want %s", assignment.Bucket, assignment.VariantKey, want)
		}
		if assignment.IsControl != (want == "CONTROL") {
			t.Fatalf("control flag disagrees with variant %s", assignment.VariantKey)
		}
	}
}

// TestExperiment_NewCampaignVersionMayReassign proves assignment is scoped to
// the campaign version, which is what makes a re-prepared audience under a NEW
// definition legitimately re-randomisable while the old one stays reproducible.
func TestExperiment_NewCampaignVersionMayReassign(t *testing.T) {
	variants := []ExperimentVariant{
		{VariantKey: "CONTROL", AllocationBps: 5000, IsControl: true},
		{VariantKey: "A", AllocationBps: 5000},
	}
	first := newExperimentAllocator(CampaignVersion{ID: "crmcv_v1", ExperimentEnabled: true, Variants: variants})
	second := newExperimentAllocator(CampaignVersion{ID: "crmcv_v2", ExperimentEnabled: true, Variants: variants})

	differed := false
	for index := 0; index < 200; index++ {
		profileID := "crmp_" + itoa(index)
		if first.Assign(profileID).VariantKey != second.Assign(profileID).VariantKey {
			differed = true
			break
		}
	}
	if !differed {
		t.Fatal("two campaign versions produced identical assignments for 200 relationships; the version is not in the hash")
	}
}

// TestExperiment_AssignmentInputCarriesNoContactData is the privacy property.
// The bucket must be a function of the campaign version and the opaque profile
// reference only — never of an email address or phone number, which would both
// leak PII into a hash and make cohorts move when a customer changed contact
// details.
func TestExperiment_AssignmentInputCarriesNoContactData(t *testing.T) {
	base := experimentBucket("crmcv_1", "crmp_1")
	if again := experimentBucket("crmcv_1", "crmp_1"); again != base {
		t.Fatal("bucket is not a pure function of its two inputs")
	}
	if changed := experimentBucket("crmcv_1", "crmp_2"); changed == base {
		// Not impossible, but with 10,000 buckets a collision here would make
		// the rest of the assertion meaningless, so pick another pair.
		if experimentBucket("crmcv_1", "crmp_3") == base {
			t.Fatal("bucket does not vary with the relationship")
		}
	}
	if namespaced := experimentBucket("crmcv_1", "crmp_1"+experimentNamespace); namespaced == base {
		t.Fatal("namespace is not separating inputs")
	}
}

// TestExperiment_NoExperimentMeansNoCohort proves a campaign without an
// experiment produces no variant at all, rather than a synthetic treatment
// group of everyone that a later analysis would misread.
func TestExperiment_NoExperimentMeansNoCohort(t *testing.T) {
	allocator := newExperimentAllocator(CampaignVersion{ID: "crmcv_plain"})
	assignment := allocator.Assign("crmp_1")
	if assignment.VariantKey != "" || assignment.IsControl {
		t.Fatalf("expected no cohort, got %+v", assignment)
	}
	control, treatment, perVariant := allocator.ProjectedCohorts([]string{"a", "b", "c"})
	if control != 0 || treatment != 3 || len(perVariant) != 0 {
		t.Fatalf("expected 0/3/none, got %d/%d/%d", control, treatment, len(perVariant))
	}
}

func TestQuietHours_CrossMidnightAndValidation(t *testing.T) {
	policy, err := validateQuietHours(enabled(), "22:00", "07:00", "Africa/Lagos")
	if err != nil {
		t.Fatalf("valid cross-midnight window rejected: %v", err)
	}
	if !policy.Enabled || policy.StartMinute != 22*60 || policy.EndMinute != 7*60 {
		t.Fatalf("unexpected policy: %+v", policy)
	}

	// Africa/Lagos is UTC+1, so 21:30Z is 22:30 local and inside the window,
	// while 19:30Z is 20:30 local and outside it. A policy that ignored the
	// zone would get both wrong.
	inside, err := policy.Blocks(mustTime(t, "2026-08-12T21:30:00Z"))
	if err != nil || !inside {
		t.Fatalf("22:30 local should be inside quiet hours: blocked=%t err=%v", inside, err)
	}
	outside, err := policy.Blocks(mustTime(t, "2026-08-12T19:30:00Z"))
	if err != nil || outside {
		t.Fatalf("20:30 local should be outside quiet hours: blocked=%t err=%v", outside, err)
	}
	// The early-morning end of the same cross-midnight window.
	early, err := policy.Blocks(mustTime(t, "2026-08-13T04:00:00Z"))
	if err != nil || !early {
		t.Fatalf("05:00 local should be inside quiet hours: blocked=%t err=%v", early, err)
	}

	// A same-day window is the ordinary case and must not be treated as
	// crossing midnight.
	daytime, err := validateQuietHours(enabled(), "09:00", "17:00", "Africa/Lagos")
	if err != nil {
		t.Fatalf("valid daytime window rejected: %v", err)
	}
	if blocked, blockErr := daytime.Blocks(mustTime(t, "2026-08-12T21:30:00Z")); blockErr != nil || blocked {
		t.Fatalf("22:30 local is outside a 09:00-17:00 window: blocked=%t err=%v", blocked, blockErr)
	}
}

func TestQuietHours_Rejections(t *testing.T) {
	cases := []struct{ name, start, end, zone string }{
		{"unknown zone", "22:00", "07:00", "Mars/Olympus"},
		{"non-iana local", "22:00", "07:00", "Local"},
		{"missing zone", "22:00", "07:00", ""},
		{"bad start", "25:00", "07:00", "Africa/Lagos"},
		{"bad end", "22:00", "7pm", "Africa/Lagos"},
		{"zero length", "22:00", "22:00", "Africa/Lagos"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := validateQuietHours(enabled(), testCase.start, testCase.end, testCase.zone); err == nil {
				t.Fatalf("expected %s to be rejected", testCase.name)
			}
		})
	}
	// Disabled is a clean zero policy that evaluates to "never blocks".
	policy, err := validateQuietHours(nil, "", "", "")
	if err != nil || policy.Enabled {
		t.Fatalf("disabled quiet hours should be a no-op: %+v err=%v", policy, err)
	}
	if blocked, blockErr := policy.Blocks(mustTime(t, "2026-08-12T23:00:00Z")); blockErr != nil || blocked {
		t.Fatalf("a disabled policy must never block: blocked=%t err=%v", blocked, blockErr)
	}
}

func TestFrequencyPolicy_Validation(t *testing.T) {
	count, window := 3, 1440
	policy, err := validateFrequencyPolicy(&count, &window)
	if err != nil || !policy.Enabled || policy.Cap != 3 || policy.WindowMinutes != 1440 {
		t.Fatalf("valid frequency policy rejected: %+v err=%v", policy, err)
	}
	// A cap without a window (or the reverse) is half a policy and is refused
	// rather than silently completed with a default nobody chose.
	if _, err = validateFrequencyPolicy(&count, nil); err == nil {
		t.Fatal("a cap without a window should be rejected")
	}
	if _, err = validateFrequencyPolicy(nil, &window); err == nil {
		t.Fatal("a window without a cap should be rejected")
	}
	for _, bad := range []int{0, -1, 1001} {
		value := bad
		if _, err = validateFrequencyPolicy(&value, &window); err == nil {
			t.Fatalf("cap %d should be rejected", bad)
		}
	}
	for _, bad := range []int{0, -1, 525601} {
		value := bad
		if _, err = validateFrequencyPolicy(&count, &value); err == nil {
			t.Fatalf("window %d should be rejected", bad)
		}
	}

	// Whatever the policy says, Section 8E can only ever report that it will be
	// re-evaluated at send time. There is no delivery history to evaluate it
	// against, and an empty history would make every cap pass.
	projection := frequencyProjection(policy)
	if projection.EvaluationState != string(ProjectionNotAvailable) || !projection.RecheckRequired {
		t.Fatalf("frequency projection must be NOT_AVAILABLE and recheck-required: %+v", projection)
	}
}

// TestCampaignDefinitionHash_IsOrderIndependent proves an edit that reorders
// channels, exclusions or variants is not a change, so it does not mint a
// pointless new immutable version.
func TestCampaignDefinitionHash_IsOrderIndependent(t *testing.T) {
	first, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Winback", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL", "PUSH"},
		ExclusionSegmentRefs: []string{"crmseg_2", "crmseg_3"}, ExperimentEnabled: enabled(),
		Variants: variantSet(treatment("B", 4500), control(1000), treatment("A", 4500)),
	})
	if err != nil {
		t.Fatalf("first definition rejected: %v", err)
	}
	second, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Winback", SourceSegmentRef: "crmseg_1", Channels: []string{"PUSH", "EMAIL"},
		ExclusionSegmentRefs: []string{"crmseg_3", "crmseg_2"}, ExperimentEnabled: enabled(),
		Variants: variantSet(control(1000), treatment("A", 4500), treatment("B", 4500)),
	})
	if err != nil {
		t.Fatalf("second definition rejected: %v", err)
	}
	if first.Hash() != second.Hash() {
		t.Fatal("reordering channels, exclusions or variants changed the definition hash")
	}

	// A real change must change the hash.
	third, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Winback", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL"},
		ExclusionSegmentRefs: []string{"crmseg_2", "crmseg_3"}, ExperimentEnabled: enabled(),
		Variants: variantSet(control(1000), treatment("A", 4500), treatment("B", 4500)),
	})
	if err != nil {
		t.Fatalf("third definition rejected: %v", err)
	}
	if third.Hash() == first.Hash() {
		t.Fatal("dropping a channel did not change the definition hash")
	}
}

func TestCampaignDefinition_ChannelAndAudienceValidation(t *testing.T) {
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Bad channel", SourceSegmentRef: "crmseg_1", Channels: []string{"CARRIER_PIGEON"},
	}); err == nil {
		t.Fatal("an unsupported channel should be rejected")
	}
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "No channel", SourceSegmentRef: "crmseg_1",
	}); err == nil {
		t.Fatal("a campaign with no channel should be rejected")
	}
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "No audience", Channels: []string{"EMAIL"},
	}); err == nil {
		t.Fatal("a campaign with no source segment should be rejected")
	}
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Self excluding", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL"},
		ExclusionSegmentRefs: []string{"crmseg_1"},
	}); err == nil {
		t.Fatal("excluding the source segment from itself should be rejected")
	}
	tooMany := make([]string, 0, MaxExclusionSegments+1)
	for index := 0; index <= MaxExclusionSegments; index++ {
		tooMany = append(tooMany, "crmseg_x"+itoa(index))
	}
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Too many exclusions", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL"},
		ExclusionSegmentRefs: tooMany,
	}); err == nil {
		t.Fatal("more than the exclusion ceiling should be rejected")
	}
	if _, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Bad objective", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL"},
		Objective: "SECURITY_OTP",
	}); err == nil {
		t.Fatal("a non-marketing objective should be rejected")
	}
	// Service-recovery suppression defaults on: opting out has to be a choice
	// somebody made, not one they forgot to make.
	definition, err := validateCampaignDefinition(CampaignDefinitionInput{
		Name: "Default", SourceSegmentRef: "crmseg_1", Channels: []string{"EMAIL"},
	})
	if err != nil || !definition.ServiceRecoverySuppression {
		t.Fatalf("service-recovery suppression should default on: %+v err=%v", definition, err)
	}
}
