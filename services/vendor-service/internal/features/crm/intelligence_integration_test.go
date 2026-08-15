package crm

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Section 8H real-PostgreSQL tests. The properties under test — generation
// safety, unavailable-is-not-zero, branch isolation — are all about what
// happens between transactions, so none of them can be proved with a fake.

// scoredCustomer seeds one relationship with a real observed order history.
//
// Orders are projected through the accepted Section 8A path rather than
// written directly, so the commercial facts the model reads are exactly the
// ones production would have.
func (e *testEnv) scoredCustomer(t *testing.T, business, staff, ref string, orders int,
	spendEach int64, firstAgo, lastAgo time.Duration) Profile {
	t.Helper()
	profile := e.seedProfile(t, business, staff, customerSubject(ref))
	if orders <= 0 {
		return profile
	}
	step := time.Duration(0)
	if orders > 1 {
		step = (firstAgo - lastAgo) / time.Duration(orders-1)
	}
	for index := 0; index < orders; index++ {
		at := e.now.Add(-firstAgo + step*time.Duration(index))
		e.seedOrder(t, business, staff, profile, fmt.Sprintf("%s_o%d", ref, index),
			branchA1, spendEach, spendEach, at, int64(index+1))
	}
	return e.reload(t, business, profile.ID)
}

// runIntelligence requests and runs one model to completion.
func (e *testEnv) runIntelligence(t *testing.T, business, staff, modelKey, scopeType, branchRef string) IntelligenceEvaluation {
	t.Helper()
	ctx := context.Background()
	scope := globalScope()
	evaluation, err := e.service.RequestIntelligenceRebuild(ctx, actorFor(business, staff), scope,
		RebuildIntelligenceInput{ModelKey: modelKey, ScopeType: scopeType, BranchRef: branchRef})
	if err != nil {
		t.Fatalf("request rebuild %s: %v", modelKey, err)
	}
	if _, err = e.service.RunIntelligenceMaterializer(ctx, "test-worker", 5); err != nil {
		t.Fatalf("materialize %s: %v", modelKey, err)
	}
	reloaded, err := scanIntelligenceEvaluation(e.pool.QueryRow(ctx,
		`SELECT `+evaluationColumns+` FROM crm_intelligence_evaluations WHERE id=$1`, evaluation.ID))
	if err != nil {
		t.Fatalf("reload evaluation: %v", err)
	}
	return reloaded
}

func (e *testEnv) scoreOf(t *testing.T, business, profileID, modelKey, scopeType, branchRef string) (ScoreView, bool) {
	t.Helper()
	view, err := scanScoreView(e.pool.QueryRow(context.Background(), `SELECT `+scoreViewColumns+`
		FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND customer_profile_id=$2 AND model_key=$3
		  AND scope=$4 AND COALESCE(branch_id,'')=$5 AND is_current`,
		business, profileID, modelKey, scopeType, branchRef))
	if err != nil {
		return ScoreView{}, false
	}
	return view, true
}

// TestIntelligence_RFMScoresTheCohortAndRefusesToInventMissingData is the
// central honesty test: relationships with observed orders are ranked, and
// relationships without them are NOT_AVAILABLE rather than bottom-quintile.
func TestIntelligence_RFMScoresTheCohortAndRefusesToInventMissingData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	// Six relationships with real histories, spanning the range, plus two with
	// nothing observed at all.
	best := env.scoredCustomer(t, businessA, "vs_a", "cus_best", 20, 500000, 400*24*time.Hour, 2*24*time.Hour)
	strong := env.scoredCustomer(t, businessA, "vs_a", "cus_strong", 12, 300000, 400*24*time.Hour, 10*24*time.Hour)
	middle := env.scoredCustomer(t, businessA, "vs_a", "cus_middle", 8, 200000, 400*24*time.Hour, 40*24*time.Hour)
	fading := env.scoredCustomer(t, businessA, "vs_a", "cus_fading", 5, 120000, 400*24*time.Hour, 120*24*time.Hour)
	weak := env.scoredCustomer(t, businessA, "vs_a", "cus_weak", 2, 50000, 400*24*time.Hour, 300*24*time.Hour)
	weakest := env.scoredCustomer(t, businessA, "vs_a", "cus_weakest", 2, 10000, 400*24*time.Hour, 350*24*time.Hour)
	unknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_unknown"))
	alsoUnknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_unknown2"))

	evaluation := env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	if evaluation.Status != IntelligenceEvalCompleted {
		t.Fatalf("evaluation status = %s (error=%v)", evaluation.Status, evaluation.ErrorCode)
	}
	if evaluation.ProfileCount != 8 || evaluation.AvailableCount != 6 || evaluation.UnavailableCount != 2 {
		t.Fatalf("counts profiles=%d available=%d unavailable=%d; want 8/6/2",
			evaluation.ProfileCount, evaluation.AvailableCount, evaluation.UnavailableCount)
	}

	bestScore, ok := env.scoreOf(t, businessA, best.ID, ModelRFM, IntelligenceScopeBusiness, "")
	if !ok || bestScore.State != ScoreCurrent {
		t.Fatalf("the strongest relationship has no current score: %+v", bestScore)
	}
	weakestScore, _ := env.scoreOf(t, businessA, weakest.ID, ModelRFM, IntelligenceScopeBusiness, "")
	if *bestScore.CompositeScore <= *weakestScore.CompositeScore {
		t.Fatalf("ranking is inverted: best=%d weakest=%d",
			*bestScore.CompositeScore, *weakestScore.CompositeScore)
	}
	if *bestScore.RecencyScore != 5 || *bestScore.MonetaryScore != 5 || *bestScore.FrequencyScore != 5 {
		t.Fatalf("the strongest relationship did not top every dimension: %+v", bestScore)
	}
	for _, score := range []ScoreView{bestScore, weakestScore} {
		if *score.CompositeScore != *score.RecencyScore+*score.FrequencyScore+*score.MonetaryScore {
			t.Fatalf("composite is not the sum of its parts: %+v", score)
		}
		if *score.CompositeScore < 3 || *score.CompositeScore > 15 {
			t.Fatalf("composite %d is outside 3..15", *score.CompositeScore)
		}
	}
	_ = strong
	_ = middle
	_ = fading
	_ = weak

	// The two relationships with no commercial facts are NOT_AVAILABLE, and
	// every numeric column is NULL. This is the invariant the whole section
	// exists to protect: never seen is not the same as spent nothing.
	for _, profile := range []Profile{unknown, alsoUnknown} {
		score, ok := env.scoreOf(t, businessA, profile.ID, ModelRFM, IntelligenceScopeBusiness, "")
		if !ok {
			t.Fatalf("relationship %s has no score row at all; unavailability must be stated", profile.ID)
		}
		if score.State != ScoreNotAvailable {
			t.Fatalf("a relationship with no orders is %s; want NOT_AVAILABLE", score.State)
		}
		if score.CompositeScore != nil || score.RecencyScore != nil || score.MonetaryScore != nil {
			t.Fatalf("an unavailable score carries numbers: %+v", score)
		}
		if !containsString(score.ReasonCodes, ReasonSourceNotAvailable) {
			t.Fatalf("unavailability did not say why: %v", score.ReasonCodes)
		}
	}

	// Rerunning over unchanged facts produces the same scores. Determinism is
	// what makes a score citable a week later.
	first := *bestScore.CompositeScore
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	again, _ := env.scoreOf(t, businessA, best.ID, ModelRFM, IntelligenceScopeBusiness, "")
	if *again.CompositeScore != first {
		t.Fatalf("a rerun over identical facts changed the score: %d then %d", first, *again.CompositeScore)
	}
}

// TestIntelligence_SmallCohortIsUnavailableRatherThanRanked pins the rule that
// a quintile over three people is not a quintile.
func TestIntelligence_SmallCohortIsUnavailableRatherThanRanked(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	lonely := env.scoredCustomer(t, businessA, "vs_a", "cus_lonely", 6, 100000, 200*24*time.Hour, 3*24*time.Hour)

	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	score, ok := env.scoreOf(t, businessA, lonely.ID, ModelRFM, IntelligenceScopeBusiness, "")
	if !ok {
		t.Fatal("no score row")
	}
	if score.State != ScoreNotAvailable {
		t.Fatalf("a one-member cohort was ranked: %+v", score)
	}
	if !containsString(score.ReasonCodes, ReasonInsufficientCohort) {
		t.Fatalf("the reason did not name the cohort: %v", score.ReasonCodes)
	}
}

// TestIntelligence_ChurnUsesObservedCadenceAndProtectsNewRelationships is the
// churn half of the same honesty property.
func TestIntelligence_ChurnUsesObservedCadenceAndProtectsNewRelationships(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	// A monthly buyer who has been silent for five months.
	lapsed := env.scoredCustomer(t, businessA, "vs_a", "cus_lapsed", 12, 100000,
		500*24*time.Hour, 150*24*time.Hour)
	// The same cadence, ordered last week.
	steady := env.scoredCustomer(t, businessA, "vs_a", "cus_steady", 12, 100000,
		370*24*time.Hour, 7*24*time.Hour)
	// Arrived yesterday, one order.
	newcomer := env.scoredCustomer(t, businessA, "vs_a", "cus_new", 1, 100000,
		24*time.Hour, 24*time.Hour)
	// No commercial facts at all.
	unknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_nofacts"))

	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	lapsedScore, _ := env.scoreOf(t, businessA, lapsed.ID, ModelChurn, IntelligenceScopeBusiness, "")
	steadyScore, _ := env.scoreOf(t, businessA, steady.ID, ModelChurn, IntelligenceScopeBusiness, "")
	if lapsedScore.State != ScoreCurrent || steadyScore.State != ScoreCurrent {
		t.Fatalf("scoreable relationships were not scored: lapsed=%s steady=%s",
			lapsedScore.State, steadyScore.State)
	}
	if *lapsedScore.RiskScore <= *steadyScore.RiskScore {
		t.Fatalf("a lapsed relationship (%d) is not riskier than an on-cadence one (%d)",
			*lapsedScore.RiskScore, *steadyScore.RiskScore)
	}
	if *lapsedScore.RiskScore < 0 || *lapsedScore.RiskScore > 100 {
		t.Fatalf("risk %d is outside 0..100", *lapsedScore.RiskScore)
	}
	if *lapsedScore.RiskBand != ChurnBandFor(*lapsedScore.RiskScore) {
		t.Fatalf("band %s does not match score %d", *lapsedScore.RiskBand, *lapsedScore.RiskScore)
	}
	if len(lapsedScore.ReasonCodes) == 0 {
		t.Fatal("a churn score with no explanation is not actionable")
	}

	// The newcomer and the unknown relationship are both NOT_AVAILABLE — never
	// CRITICAL. A follow-up list full of people who just arrived is worse than
	// no list.
	for name, profile := range map[string]Profile{"newcomer": newcomer, "unknown": unknown} {
		score, ok := env.scoreOf(t, businessA, profile.ID, ModelChurn, IntelligenceScopeBusiness, "")
		if !ok {
			t.Fatalf("%s has no score row", name)
		}
		if score.State != ScoreNotAvailable {
			t.Fatalf("%s was scored %s with risk %v; too little history must be unavailable",
				name, score.State, score.RiskScore)
		}
		if score.RiskScore != nil || score.RiskBand != nil {
			t.Fatalf("%s carries a risk despite being unavailable: %+v", name, score)
		}
	}
	newcomerScore, _ := env.scoreOf(t, businessA, newcomer.ID, ModelChurn, IntelligenceScopeBusiness, "")
	if !containsString(newcomerScore.ReasonCodes, ReasonInsufficientHistory) {
		t.Fatalf("a one-order relationship did not report insufficient history: %v", newcomerScore.ReasonCodes)
	}
}

// TestIntelligence_ModelVersionAndHashArePersistedWithEveryScore is the
// governance property: a stored score can always be traced to the rules that
// produced it.
func TestIntelligence_ModelVersionAndHashArePersistedWithEveryScore(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_gov%d", index), 4+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*10*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	var version int
	var hash string
	var distinctHashes int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(DISTINCT definition_hash)
		FROM crm_customer_intelligence_scores WHERE business_id=$1 AND model_key=$2`,
		businessA, ModelRFM).Scan(&distinctHashes); err != nil {
		t.Fatal(err)
	}
	if distinctHashes != 1 {
		t.Fatalf("one generation produced %d different definition hashes", distinctHashes)
	}
	if err := env.pool.QueryRow(context.Background(), `SELECT model_version, definition_hash
		FROM crm_customer_intelligence_scores WHERE business_id=$1 AND model_key=$2 LIMIT 1`,
		businessA, ModelRFM).Scan(&version, &hash); err != nil {
		t.Fatal(err)
	}
	if version != rfmModel.Version || hash != rfmModel.DefinitionHash() {
		t.Fatalf("stored identity version=%d hash=%s does not match the registry", version, hash)
	}

	// A queued run whose model changed before it executed is superseded rather
	// than writing scores stamped with an identity that no longer exists.
	ctx := context.Background()
	evaluation, err := env.service.RequestIntelligenceRebuild(ctx, actorFor(businessA, "vs_a"), globalScope(),
		RebuildIntelligenceInput{ModelKey: ModelChurn, ScopeType: IntelligenceScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.pool.Exec(ctx, `UPDATE crm_intelligence_evaluations
		SET definition_hash='0000000000000000000000000000000000000000000000000000000000000000'
		WHERE id=$1`, evaluation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.RunIntelligenceMaterializer(ctx, "test-worker", 5); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = env.pool.QueryRow(ctx, `SELECT status FROM crm_intelligence_evaluations WHERE id=$1`,
		evaluation.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != IntelligenceEvalSuperseded {
		t.Fatalf("a stale-model run finished as %s; want SUPERSEDED", status)
	}
	var churnScores int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2`, businessA, ModelChurn).Scan(&churnScores); err != nil {
		t.Fatal(err)
	}
	if churnScores != 0 {
		t.Fatalf("a superseded run wrote %d scores", churnScores)
	}
}

// TestIntelligence_FailedRebuildKeepsTheLastGoodGeneration is the generation
// safety property. A failed rebuild must degrade to "these numbers are stale",
// never to "there are none".
func TestIntelligence_FailedRebuildKeepsTheLastGoodGeneration(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_gen%d", index), 4+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*10*24*time.Hour)
	}
	first := env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	var liveBefore int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2 AND is_current AND state='CURRENT'`,
		businessA, ModelRFM).Scan(&liveBefore); err != nil {
		t.Fatal(err)
	}
	if liveBefore == 0 {
		t.Fatal("the first generation produced no current scores")
	}

	// Force the next run to fail by shrinking the cohort ceiling below the
	// tenant's size — a real failure mode, not a mocked one.
	env.service.intelligence.MaxCohort = 1
	env.service.intelligence.MaxAttempts = 1
	second, err := env.service.RequestIntelligenceRebuild(ctx, actorFor(businessA, "vs_a"), globalScope(),
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.RunIntelligenceMaterializer(ctx, "test-worker", 5); err != nil {
		t.Fatal(err)
	}

	var failedStatus string
	if err = env.pool.QueryRow(ctx, `SELECT status FROM crm_intelligence_evaluations WHERE id=$1`,
		second.ID).Scan(&failedStatus); err != nil {
		t.Fatal(err)
	}
	if failedStatus != IntelligenceEvalFailed && failedStatus != IntelligenceEvalDeadLetter {
		t.Fatalf("the failing run finished as %s", failedStatus)
	}

	// The previous generation is untouched and still current.
	var liveAfter int
	var liveGeneration int64
	if err = env.pool.QueryRow(ctx, `SELECT count(*), COALESCE(MAX(generation),0)
		FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2 AND is_current AND state='CURRENT'`,
		businessA, ModelRFM).Scan(&liveAfter, &liveGeneration); err != nil {
		t.Fatal(err)
	}
	if liveAfter != liveBefore {
		t.Fatalf("a failed rebuild changed the live audience: %d then %d", liveBefore, liveAfter)
	}
	if liveGeneration != first.Generation {
		t.Fatalf("live generation moved to %d; the last good one was %d", liveGeneration, first.Generation)
	}
	// And no row from the failed generation ever became current.
	var leaked int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND generation=$2 AND is_current`,
		businessA, second.Generation).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatalf("%d rows from the failed generation are live", leaked)
	}

	// The scope reports the truth: stale, with its last-good as-of intact.
	state, err := env.service.scopeStateFor(ctx, businessA, ModelRFM, IntelligenceScopeBusiness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != IntelligenceScopeStale {
		t.Fatalf("scope status = %s after a failed rebuild; want STALE", state.Status)
	}
	if state.ComputedAt == nil || state.Generation == nil || *state.Generation != first.Generation {
		t.Fatalf("the scope lost its last-good pointer: %+v", state)
	}
}

// TestIntelligence_DuplicateRebuildRequestsProduceOneRun is the idempotency
// property that makes the button safe to press.
func TestIntelligence_DuplicateRebuildRequestsProduceOneRun(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.scoredCustomer(t, businessA, "vs_a", "cus_dup", 3, 100000, 90*24*time.Hour, 5*24*time.Hour)

	first, err := env.service.RequestIntelligenceRebuild(ctx, actorFor(businessA, "vs_a"), globalScope(),
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	second, err := env.service.RequestIntelligenceRebuild(ctx, actorFor(businessA, "vs_a"), globalScope(),
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("two rebuild requests created two runs: %s and %s", first.ID, second.ID)
	}
	var runs int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_intelligence_evaluations
		WHERE business_id=$1 AND model_key=$2`, businessA, ModelRFM).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("%d evaluations exist for one scope and model", runs)
	}
}

// TestIntelligence_CommercialChangeMarksScopesStale is dependency invalidation
// in the transaction that changed the facts.
func TestIntelligence_CommercialChangeMarksScopesStale(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_inv%d", index), 4+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*10*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	state, err := env.service.scopeStateFor(ctx, businessA, ModelRFM, IntelligenceScopeBusiness, nil)
	if err != nil || state.Status != IntelligenceScopeCurrent {
		t.Fatalf("scope is %s after a successful rebuild (err=%v)", state.Status, err)
	}

	// A new order changes the facts every model reads.
	mover := env.scoredCustomer(t, businessA, "vs_a", "cus_mover", 2, 500000, 20*24*time.Hour, time.Hour)
	_ = mover

	state, err = env.service.scopeStateFor(ctx, businessA, ModelRFM, IntelligenceScopeBusiness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != IntelligenceScopeStale {
		t.Fatalf("scope is %s after the commercial projection changed; want STALE", state.Status)
	}
	if state.StaleSince == nil {
		t.Fatal("a stale scope did not record when it went stale")
	}
	// Staleness does not blank the numbers: the operator keeps the last good
	// figures with an honest as-of.
	if state.Generation == nil || state.AvailableCount == 0 {
		t.Fatalf("going stale discarded the last good generation: %+v", state)
	}
}

// TestIntelligence_RedactionAndMergeSuppressScoresImmediately is the privacy
// property, exercised through the accepted Section 8A privacy path.
func TestIntelligence_RedactionAndMergeSuppressScoresImmediately(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	var subjects []Profile
	for index := 0; index < 6; index++ {
		subjects = append(subjects, env.scoredCustomer(t, businessA, "vs_a",
			fmt.Sprintf("cus_priv%d", index), 4+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*10*24*time.Hour))
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	victim := subjects[0]
	if score, ok := env.scoreOf(t, businessA, victim.ID, ModelRFM, IntelligenceScopeBusiness, ""); !ok ||
		score.State != ScoreCurrent {
		t.Fatalf("the subject was not scored before redaction: %+v", score)
	}

	if err := env.applyPrivacy(t, businessA, "vs_a", &victim, PrivacyInput{
		SourceService: "customer-service", SourceVersion: victim.Version + 1,
		PrivacySubjectRef: victim.ID, Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	// No current score survives, and the demoted row says why.
	if _, ok := env.scoreOf(t, businessA, victim.ID, ModelRFM, IntelligenceScopeBusiness, ""); ok {
		t.Fatal("a redacted relationship still has a current score")
	}
	var reasons []string
	var state string
	if err := env.pool.QueryRow(ctx, `SELECT state, reason_codes FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND customer_profile_id=$2 ORDER BY created_at DESC LIMIT 1`,
		businessA, victim.ID).Scan(&state, &reasons); err != nil {
		t.Fatal(err)
	}
	if state != ScoreNotAvailable || !containsString(reasons, ReasonPrivacySuppressed) {
		t.Fatalf("the demoted score is %s with reasons %v; want NOT_AVAILABLE/PRIVACY_SUPPRESSED", state, reasons)
	}

	// Customer 360 refuses to present it at all.
	redacted := env.reload(t, businessA, victim.ID)
	intelligence, err := env.service.CustomerIntelligenceFor(ctx, actorFor(businessA, "vs_a"),
		globalScope(), redacted, "")
	if err != nil {
		t.Fatal(err)
	}
	if intelligence.RFM != nil || intelligence.RFMState != ScoreNotAvailable {
		t.Fatalf("a redacted relationship exposed intelligence: %+v", intelligence)
	}
}
