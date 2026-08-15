package crm

import (
	"strings"
	"testing"
	"time"
)

// Section 8H model unit tests. They need no database because the models are
// pure functions — which is the point of computing scores in Go rather than in
// a SQL expression nobody can call twice with the same inputs.

func TestRFM_QuintileMappingIsDeterministicAndTieSafe(t *testing.T) {
	cases := map[float64]int{
		0.0: 1, 0.01: 1, 0.2: 1,
		0.21: 2, 0.4: 2,
		0.41: 3, 0.6: 3,
		0.61: 4, 0.8: 4,
		0.81: 5, 1.0: 5,
	}
	for cd, want := range cases {
		if got := quintileFromCumeDist(cd); got != want {
			t.Errorf("cume_dist %.2f mapped to %d; want %d", cd, got, want)
		}
	}
	// Every score is inside the documented range, for any input including ones
	// the database should never produce.
	for _, cd := range []float64{-1, 0, 0.5, 1, 2} {
		if score := quintileFromCumeDist(cd); score < 1 || score > 5 {
			t.Errorf("cume_dist %.2f produced %d, outside 1..5", cd, score)
		}
	}
	// A lone scored relationship is the strongest in the set it is compared
	// against, and the COHORT_RELATIVE reason is what stops that reading as
	// "top fifth of thousands".
	if score := quintileFromCumeDist(1.0); score != 5 {
		t.Fatalf("a single-member cohort scored %d; want 5", score)
	}
	// Determinism: the same input, a hundred times, is the same answer.
	first := computeRFM(rfmDimensions{RecencyCD: 0.55, FrequencyCD: 0.2, MonetaryCD: 0.9})
	for i := 0; i < 100; i++ {
		again := computeRFM(rfmDimensions{RecencyCD: 0.55, FrequencyCD: 0.2, MonetaryCD: 0.9})
		if again.Composite != first.Composite || again.Recency != first.Recency ||
			again.Frequency != first.Frequency || again.Monetary != first.Monetary ||
			strings.Join(again.ReasonCodes, ",") != strings.Join(first.ReasonCodes, ",") {
			t.Fatalf("RFM is not deterministic: %+v then %+v", first, again)
		}
	}
	// Ties: two relationships with identical positions score identically.
	left := computeRFM(rfmDimensions{ProfileID: "a", RecencyCD: 0.4, FrequencyCD: 0.4, MonetaryCD: 0.4})
	right := computeRFM(rfmDimensions{ProfileID: "z", RecencyCD: 0.4, FrequencyCD: 0.4, MonetaryCD: 0.4})
	if left.Composite != right.Composite || left.Recency != right.Recency {
		t.Fatalf("equal inputs produced different scores: %+v vs %+v", left, right)
	}
}

func TestRFM_DirectionsAndCompositeAreCorrect(t *testing.T) {
	// The cume_dist for recency is taken over days DESCENDING, so a higher
	// value means fewer days since the last order. Every dimension therefore
	// reads the same way in the score: 5 is strongest.
	strong := computeRFM(rfmDimensions{RecencyCD: 1.0, FrequencyCD: 1.0, MonetaryCD: 1.0})
	weak := computeRFM(rfmDimensions{RecencyCD: 0.05, FrequencyCD: 0.05, MonetaryCD: 0.05})
	if strong.Recency <= weak.Recency || strong.Frequency <= weak.Frequency ||
		strong.Monetary <= weak.Monetary {
		t.Fatalf("dimension direction is inverted: strong=%+v weak=%+v", strong, weak)
	}
	if strong.Composite != 15 || weak.Composite != 3 {
		t.Fatalf("composite bounds wrong: strong=%d weak=%d; want 15 and 3",
			strong.Composite, weak.Composite)
	}
	mixed := computeRFM(rfmDimensions{RecencyCD: 1.0, FrequencyCD: 0.5, MonetaryCD: 0.05})
	if mixed.Composite != mixed.Recency+mixed.Frequency+mixed.Monetary {
		t.Fatalf("composite %d is not the sum of %d+%d+%d", mixed.Composite,
			mixed.Recency, mixed.Frequency, mixed.Monetary)
	}
	// The explanation names only what is true, and always says the score is
	// relative to a cohort.
	if !containsString(strong.ReasonCodes, ReasonCohortRelative) {
		t.Fatal("an RFM score must state that it is cohort-relative")
	}
	if !containsString(strong.ReasonCodes, ReasonRecencyStrong) ||
		containsString(strong.ReasonCodes, ReasonRecencyWeak) {
		t.Fatalf("reason codes contradict the score: %v", strong.ReasonCodes)
	}
	if !containsString(weak.ReasonCodes, ReasonMonetaryWeak) {
		t.Fatalf("a bottom-quintile monetary score did not say so: %v", weak.ReasonCodes)
	}
}

func TestChurn_NewCustomerIsNeverAutomaticallyAtRisk(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	// One observed order: there is no interval to be late against, so the model
	// declines to answer rather than calling a brand-new customer a risk.
	if _, ok := computeChurn(ChurnInput{CompletedOrderCount: 1,
		FirstOrderAt: now.AddDate(0, 0, -1), LastOrderAt: now.AddDate(0, 0, -1), AsOf: now}); ok {
		t.Fatal("a single-purchase relationship produced a churn score")
	}
	// No orders at all is likewise unanswerable, not maximum risk.
	if _, ok := computeChurn(ChurnInput{CompletedOrderCount: 0, AsOf: now}); ok {
		t.Fatal("a relationship with no observed orders produced a churn score")
	}
	// A customer who ordered twice yesterday and the day before is on cadence,
	// not lapsed.
	score, ok := computeChurn(ChurnInput{CompletedOrderCount: 2,
		FirstOrderAt: now.AddDate(0, 0, -2), LastOrderAt: now.AddDate(0, 0, -1), AsOf: now})
	if !ok {
		t.Fatal("two observed orders should be scoreable")
	}
	if score.Band != RiskLow {
		t.Fatalf("a customer ordering on cadence is %s risk; want LOW (score=%d reasons=%v)",
			score.Band, score.Risk, score.ReasonCodes)
	}
}

func TestChurn_ScoreRangeBandsAndDeterminism(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	// Monthly buyer, four months silent: overdue against its own cadence.
	lapsed := ChurnInput{CompletedOrderCount: 12,
		FirstOrderAt: now.AddDate(0, -12, 0), LastOrderAt: now.AddDate(0, -4, 0), AsOf: now}
	score, ok := computeChurn(lapsed)
	if !ok {
		t.Fatal("a lapsed relationship should be scoreable")
	}
	if score.Risk < 0 || score.Risk > 100 {
		t.Fatalf("risk %d is outside 0..100", score.Risk)
	}
	if score.Band != ChurnBandFor(score.Risk) {
		t.Fatalf("band %s does not match score %d", score.Band, score.Risk)
	}
	if score.Band == RiskLow {
		t.Fatalf("a four-months-silent monthly buyer is %s; that is not a lapse the model should miss "+
			"(score=%d reasons=%v)", score.Band, score.Risk, score.ReasonCodes)
	}
	for i := 0; i < 50; i++ {
		again, _ := computeChurn(lapsed)
		if again.Risk != score.Risk || again.Band != score.Band {
			t.Fatalf("churn is not deterministic: %d/%s then %d/%s",
				score.Risk, score.Band, again.Risk, again.Band)
		}
		if strings.Join(again.ReasonCodes, ",") != strings.Join(score.ReasonCodes, ",") {
			t.Fatalf("churn reasons are not deterministic: %v then %v", score.ReasonCodes, again.ReasonCodes)
		}
	}

	// Band boundaries are exactly where the model says they are.
	boundaries := map[int]string{
		0: RiskLow, 24: RiskLow, 25: RiskMedium, 49: RiskMedium,
		50: RiskHigh, 74: RiskHigh, 75: RiskCritical, 100: RiskCritical,
	}
	for risk, want := range boundaries {
		if got := ChurnBandFor(risk); got != want {
			t.Errorf("risk %d banded %s; want %s", risk, got, want)
		}
	}
}

func TestChurn_ExplanationsOnlyNameObservedFacts(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	// On cadence and deeply engaged.
	steady, _ := computeChurn(ChurnInput{CompletedOrderCount: 24,
		FirstOrderAt: now.AddDate(0, -24, 0), LastOrderAt: now.AddDate(0, 0, -20), AsOf: now})
	if !containsString(steady.ReasonCodes, ReasonOnExpectedCadence) {
		t.Fatalf("an on-cadence relationship did not say so: %v", steady.ReasonCodes)
	}
	if !containsString(steady.ReasonCodes, ReasonSustainedEngagement) {
		t.Fatalf("a deeply engaged on-cadence relationship did not say so: %v", steady.ReasonCodes)
	}
	if containsString(steady.ReasonCodes, ReasonLongInactivity) {
		t.Fatalf("a 20-day-old order was reported as long inactivity: %v", steady.ReasonCodes)
	}

	// Silent for over a year.
	gone, _ := computeChurn(ChurnInput{CompletedOrderCount: 15,
		FirstOrderAt: now.AddDate(-3, 0, 0), LastOrderAt: now.AddDate(0, -14, 0), AsOf: now})
	if !containsString(gone.ReasonCodes, ReasonExtendedInactivity) {
		t.Fatalf("a fourteen-month absence did not report extended inactivity: %v", gone.ReasonCodes)
	}
	if !containsString(gone.ReasonCodes, ReasonHighEngagementLapsed) {
		t.Fatalf("a lapsed high-engagement relationship did not say so: %v", gone.ReasonCodes)
	}
	if containsString(gone.ReasonCodes, ReasonOnExpectedCadence) {
		t.Fatalf("contradictory reasons: %v", gone.ReasonCodes)
	}

	// Every emitted code is from the closed vocabulary.
	known := map[string]bool{
		ReasonOnExpectedCadence: true, ReasonRecencyIncreased: true, ReasonOverdueVersusCadence: true,
		ReasonSeverelyOverdue: true, ReasonLongInactivity: true, ReasonExtendedInactivity: true,
		ReasonHighEngagementLapsed: true, ReasonSustainedEngagement: true,
	}
	for _, score := range []ChurnScore{steady, gone} {
		for _, code := range score.ReasonCodes {
			if !known[code] {
				t.Errorf("churn emitted unknown reason code %q", code)
			}
		}
	}
}

// TestChurn_NoProbabilityLanguageAnywhere is a documentation guard with teeth.
// The model must never describe itself as a likelihood, because a reader who
// believes "72" means "72% will leave" will make a decision the model cannot
// support.
func TestChurn_NoProbabilityLanguageAnywhere(t *testing.T) {
	forbidden := []string{"probability", "probabilit", "likelihood", "% likely", "predict",
		"machine learning", "neural", "model artifact", "confidence interval"}
	for _, model := range IntelligenceModels() {
		haystack := strings.ToLower(model.Description + " " + model.Label + " " +
			strings.Join(model.Descriptor, " "))
		for _, term := range forbidden {
			// "prediction" is permitted only in a sentence that denies it.
			if strings.Contains(haystack, term) && !strings.Contains(haystack, "not a "+term) &&
				!strings.Contains(haystack, "not a prediction") {
				t.Errorf("model %s describes itself with %q", model.Key, term)
			}
		}
	}
}

// TestModelGovernance_KeysAreClosedAndHashesAreStable pins the identity
// contract: a score can always be traced to the exact rules that produced it.
func TestModelGovernance_KeysAreClosedAndHashesAreStable(t *testing.T) {
	if len(AllModelKeys) != 2 {
		t.Fatalf("model registry holds %d keys; Section 8H defines exactly RFM_V1 and CHURN_V1", len(AllModelKeys))
	}
	for _, key := range []string{"RFM_V2", "CHURN_V2", "ML_CHURN", "", "rfm_v1", "'; DROP TABLE"} {
		if ValidModelKey(key) {
			t.Errorf("model key %q was accepted; the registry must be closed", key)
		}
	}
	for _, model := range IntelligenceModels() {
		hash := model.DefinitionHash()
		if len(hash) != 64 {
			t.Fatalf("model %s hash is %d characters; want a full SHA-256", model.Key, len(hash))
		}
		// Stable across calls: two runs of the same model version agree.
		if again := model.DefinitionHash(); again != hash {
			t.Fatalf("model %s hash is not stable: %s then %s", model.Key, hash, again)
		}
		if model.Version < 1 {
			t.Fatalf("model %s has no version", model.Key)
		}
		if len(model.RequiredSources) == 0 {
			t.Fatalf("model %s declares no required sources", model.Key)
		}
	}
	// Two different models never share an identity.
	if rfmModel.DefinitionHash() == churnModel.DefinitionHash() {
		t.Fatal("RFM and churn share a definition hash")
	}
	// A changed threshold is a changed identity. This is the property that
	// stops a silent reinterpretation of historical scores.
	altered := churnModel
	altered.Descriptor = append(append([]string{}, churnModel.Descriptor...), "bands: 0-30 LOW")
	if altered.DefinitionHash() == churnModel.DefinitionHash() {
		t.Fatal("changing a documented threshold did not change the definition hash")
	}
	renamed := rfmModel
	renamed.Description = rfmModel.Description + " (revised)"
	if renamed.DefinitionHash() == rfmModel.DefinitionHash() {
		t.Fatal("changing what a model claims to do did not change its identity")
	}
}

// TestIntelligenceScope_IsClosed guards the other closed vocabulary.
func TestIntelligenceScope_IsClosed(t *testing.T) {
	if !ValidIntelligenceScope(IntelligenceScopeBusiness) || !ValidIntelligenceScope(IntelligenceScopeBranch) {
		t.Fatal("the two real scopes must validate")
	}
	for _, scope := range []string{"", "GLOBAL", "business", "BRANCH_SET", "ALL"} {
		if ValidIntelligenceScope(scope) {
			t.Errorf("scope %q was accepted", scope)
		}
	}
}
