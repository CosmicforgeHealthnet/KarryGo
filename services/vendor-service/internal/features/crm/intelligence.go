package crm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Section 8H — CRM intelligence.
//
// Everything here is a deterministic heuristic computed in Go from commercial
// facts this service was given. There is no model artefact, no training, no
// probability and no generated prose. The same inputs always produce the same
// score, and a reader can be told exactly which observations produced it.
//
// The phrase that governs the whole package: A SCORE IS NOT PERMISSION. A
// relationship can be the most valuable in the book and still be one this
// vendor may not contact. Consent, privacy, suppression and quiet hours are
// decided by Sections 8A, 8E and 8F and are never consulted, weakened or
// implied here.

// Model keys. This is a closed set: a client cannot name a model, and no code
// path turns a request value into a model.
const (
	ModelRFM   = "RFM_V1"
	ModelChurn = "CHURN_V1"
)

// Intelligence scope. A BUSINESS score reasons over the whole book; a BRANCH
// score reasons over one branch's own relationship facts and nothing else.
const (
	IntelligenceScopeBusiness = "BUSINESS"
	IntelligenceScopeBranch   = "BRANCH"
)

// Score states.
//
// There is no PENDING. QUEUED and RUNNING describe an evaluation, not a
// relationship's score, so a profile-facing row is only ever one of these four.
const (
	ScoreNotAvailable = "NOT_AVAILABLE"
	ScoreCurrent      = "CURRENT"
	ScoreStale        = "STALE"
	ScoreFailed       = "FAILED"
)

// Scope-level intelligence states. These add the two evaluation-lifecycle
// values, because "a rebuild is running" is a true and useful answer about a
// scope even though it is never an answer about one customer's score.
const (
	IntelligenceScopeNotAvailable = "NOT_AVAILABLE"
	IntelligenceScopeQueued       = "QUEUED"
	IntelligenceScopeRunning      = "RUNNING"
	IntelligenceScopeCurrent      = "CURRENT"
	IntelligenceScopeStale        = "STALE"
	IntelligenceScopeFailed       = "FAILED"
)

// Evaluation statuses.
const (
	IntelligenceEvalQueued     = "QUEUED"
	IntelligenceEvalRunning    = "RUNNING"
	IntelligenceEvalCompleted  = "COMPLETED"
	IntelligenceEvalFailed     = "FAILED"
	IntelligenceEvalDeadLetter = "DEAD_LETTER"
	IntelligenceEvalSuperseded = "SUPERSEDED"
)

// Churn risk bands.
const (
	RiskLow      = "LOW"
	RiskMedium   = "MEDIUM"
	RiskHigh     = "HIGH"
	RiskCritical = "CRITICAL"
)

// Availability and failure reasons. Every non-CURRENT score carries at least
// one, so "no score" always says why.
const (
	ReasonSourceNotAvailable  = "SOURCE_NOT_AVAILABLE"
	ReasonInsufficientHistory = "INSUFFICIENT_HISTORY"
	ReasonInsufficientCohort  = "INSUFFICIENT_COHORT"
	ReasonSourceStale         = "SOURCE_STALE"
	ReasonModelError          = "MODEL_ERROR"
	ReasonPrivacySuppressed   = "PRIVACY_SUPPRESSED"
	ReasonMergedSource        = "MERGED_SOURCE"
	ReasonSupersededModel     = "SUPERSEDED_MODEL_VERSION"
)

// RFM explanation codes. They describe standing within this scope's scored
// cohort, which is the only thing a quintile can honestly describe.
const (
	ReasonCohortRelative  = "COHORT_RELATIVE"
	ReasonRecencyStrong   = "RECENCY_STRONG"
	ReasonRecencyWeak     = "RECENCY_WEAK"
	ReasonFrequencyStrong = "FREQUENCY_STRONG"
	ReasonFrequencyWeak   = "FREQUENCY_WEAK"
	ReasonMonetaryStrong  = "MONETARY_STRONG"
	ReasonMonetaryWeak    = "MONETARY_WEAK"
)

// Churn factor codes. Each is emitted only when the observation that supports
// it is actually present in the input.
const (
	ReasonOnExpectedCadence      = "ON_EXPECTED_CADENCE"
	ReasonRecencyIncreased       = "RECENCY_INCREASED"
	ReasonOverdueVersusCadence   = "OVERDUE_VERSUS_CADENCE"
	ReasonSeverelyOverdue        = "SEVERELY_OVERDUE"
	ReasonLongInactivity         = "LONG_INACTIVITY"
	ReasonExtendedInactivity     = "EXTENDED_INACTIVITY"
	ReasonHighEngagementLapsed   = "HIGH_HISTORICAL_ENGAGEMENT_LAPSED"
	ReasonSustainedEngagement    = "SUSTAINED_ENGAGEMENT"
	ReasonSingleObservedPurchase = "SINGLE_OBSERVED_PURCHASE"
)

// AllModelKeys is the closed registry order, used by rebuild-all paths and by
// the contract tests that assert the set has not grown by accident.
var AllModelKeys = []string{ModelRFM, ModelChurn}

// ValidModelKey reports whether a key names a model this deployment computes.
func ValidModelKey(key string) bool {
	for _, candidate := range AllModelKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

// ValidIntelligenceScope reports whether a scope value is legal.
func ValidIntelligenceScope(scope string) bool {
	return scope == IntelligenceScopeBusiness || scope == IntelligenceScopeBranch
}

// ---------------------------------------------------------------------------
// Model registry
// ---------------------------------------------------------------------------

// IntelligenceModel is a versioned, code-defined scoring definition.
//
// It is deliberately not a script, a template or a row: a model is Go, and
// changing one means editing this file and bumping its version. There is no
// path from an HTTP request to a formula, and none from a stored string to
// arithmetic.
type IntelligenceModel struct {
	Key     string
	Version int
	Label   string
	// Description is the human-readable definition. It is part of the hashed
	// descriptor, so documentation and behaviour cannot drift apart silently:
	// changing what the model claims to do changes its identity.
	Description string
	// RequiredSources names the authoritative facts the model consumes. A
	// deployment missing one produces NOT_AVAILABLE rather than a guess.
	RequiredSources []string
	// Descriptor is the canonical, order-stable string the definition hash is
	// taken over.
	Descriptor []string
}

// DefinitionHash identifies the exact scoring rules that produced a score.
//
// It is computed from the descriptor rather than from source code, so a
// comment or a refactor does not invalidate every historical score, while a
// change to a threshold, a band, a range or a claimed behaviour does.
func (m IntelligenceModel) DefinitionHash() string {
	sum := sha256.Sum256([]byte(strings.Join(append([]string{
		m.Key, strconv.Itoa(m.Version), m.Description,
	}, m.Descriptor...), "\n")))
	return hex.EncodeToString(sum[:])
}

// rfmModel is the RFM V1 definition.
//
// Scores are scope-relative quintiles of the scored cohort, computed from
// PostgreSQL's cume_dist so equal inputs always receive equal scores — two
// customers with identical behaviour can never land in different bands because
// of row order. A single-member cohort scores 5, which is correct: it is the
// strongest relationship in the set it is being compared against, and the
// COHORT_RELATIVE reason code says exactly that.
var rfmModel = IntelligenceModel{
	Key: ModelRFM, Version: 1, Label: "RFM (recency, frequency, monetary) V1",
	Description: "Scope-relative quintile scoring of observed completed-order recency, count and net spend. " +
		"Each dimension is 1 (weakest fifth of this scope's scored cohort) to 5 (strongest fifth); " +
		"the composite is their sum, 3 to 15. Scores are comparable within one scope and not across businesses.",
	RequiredSources: []string{
		"commercial.projection_state", "commercial.completed_order_count",
		"commercial.net_spend_minor", "commercial.last_completed_order_at",
	},
	Descriptor: []string{
		"dimension=recency;order=days_since_last_order DESC;bucket=ceil(cume_dist*5);range=1..5",
		"dimension=frequency;order=completed_order_count ASC;bucket=ceil(cume_dist*5);range=1..5",
		"dimension=monetary;order=net_spend_minor ASC;bucket=ceil(cume_dist*5);range=1..5",
		"composite=recency+frequency+monetary;range=3..15",
		"eligibility=projection_state<>NOT_AVAILABLE AND completed_order_count>=1 AND last_order_at IS NOT NULL",
		"minimum_cohort=" + strconv.Itoa(RFMMinimumCohort),
		"ties=equal inputs receive equal scores (cume_dist)",
	},
}

// churnModel is the Churn V1 definition.
//
// It is a risk HEURISTIC, not a prediction and not a probability. The whole
// idea is one observation: a relationship that has stopped buying at the pace
// it used to buy is at risk, and how overdue it is relative to its OWN
// observed cadence says more than a calendar threshold applied to everybody.
var churnModel = IntelligenceModel{
	Key: ModelChurn, Version: 1, Label: "Churn risk heuristic V1",
	Description: "Deterministic 0-100 risk heuristic derived from how overdue a relationship is against its own " +
		"observed purchase cadence, plus absolute inactivity and engagement depth. It is not a probability, " +
		"not a prediction and not a statistical model; it ranks relationships for follow-up.",
	RequiredSources: []string{
		"commercial.projection_state", "commercial.completed_order_count",
		"commercial.first_completed_order_at", "commercial.last_completed_order_at",
	},
	Descriptor: []string{
		"cadence_days=floor((last_order_at-first_order_at)/(order_count-1)), minimum 1",
		"overdue_ratio=days_since_last_order/cadence_days",
		"base: ratio<=1 => 0; ratio<=2 => 25; ratio<=3 => 50; ratio<=5 => 70; else 85",
		"inactivity: days>=365 => +15; else days>=180 => +10",
		"engagement: orders>=10 AND ratio>2 => +5; orders>=10 AND ratio<=1 => -5",
		"clamp=0..100",
		"bands: 0-24 LOW; 25-49 MEDIUM; 50-74 HIGH; 75-100 CRITICAL",
		"eligibility=projection_state<>NOT_AVAILABLE AND completed_order_count>=2 AND both order timestamps present",
	},
}

// intelligenceModels is the closed registry.
var intelligenceModels = map[string]IntelligenceModel{
	ModelRFM:   rfmModel,
	ModelChurn: churnModel,
}

// ModelFor returns the definition for a key.
func ModelFor(key string) (IntelligenceModel, bool) {
	model, ok := intelligenceModels[key]
	return model, ok
}

// IntelligenceModels lists the registry in a stable order.
func IntelligenceModels() []IntelligenceModel {
	out := make([]IntelligenceModel, 0, len(AllModelKeys))
	for _, key := range AllModelKeys {
		out = append(out, intelligenceModels[key])
	}
	return out
}

// ---------------------------------------------------------------------------
// RFM V1
// ---------------------------------------------------------------------------

// RFMMinimumCohort is the smallest scored cohort for which quintiles mean
// anything. Below it every relationship reports NOT_AVAILABLE with
// INSUFFICIENT_COHORT rather than being ranked against two other people — a
// score that says "top fifth of three" is worse than no score, because it
// reads exactly like "top fifth of thousands".
const RFMMinimumCohort = 5

// rfmDimensions is one relationship's position within the scored cohort,
// expressed as cumulative distribution in [0,1] where 1 is strongest.
type rfmDimensions struct {
	ProfileID              string
	RecencyCD              float64
	FrequencyCD            float64
	MonetaryCD             float64
	DaysSinceLastOrder     int64
	CompletedOrderCount    int
	NetSpendMinor          int64
	CommercialProjectionOK bool
}

// RFMScore is the computed output for one relationship.
type RFMScore struct {
	Recency     int
	Frequency   int
	Monetary    int
	Composite   int
	ReasonCodes []string
}

// quintileFromCumeDist maps a cumulative distribution onto 1..5.
//
// cume_dist is tie-equal by definition, so this is the property the whole
// model rests on: two relationships with identical behaviour receive an
// identical score, always, regardless of the order rows happened to arrive in.
func quintileFromCumeDist(cd float64) int {
	if math.IsNaN(cd) || cd <= 0 {
		return 1
	}
	if cd >= 1 {
		return 5
	}
	score := int(math.Ceil(cd * 5))
	if score < 1 {
		return 1
	}
	if score > 5 {
		return 5
	}
	return score
}

// computeRFM scores one relationship. It is pure: the same dimensions always
// produce the same score and the same explanation.
func computeRFM(in rfmDimensions) RFMScore {
	recency := quintileFromCumeDist(in.RecencyCD)
	frequency := quintileFromCumeDist(in.FrequencyCD)
	monetary := quintileFromCumeDist(in.MonetaryCD)

	reasons := []string{ReasonCohortRelative}
	switch recency {
	case 5:
		reasons = append(reasons, ReasonRecencyStrong)
	case 1:
		reasons = append(reasons, ReasonRecencyWeak)
	}
	switch frequency {
	case 5:
		reasons = append(reasons, ReasonFrequencyStrong)
	case 1:
		reasons = append(reasons, ReasonFrequencyWeak)
	}
	switch monetary {
	case 5:
		reasons = append(reasons, ReasonMonetaryStrong)
	case 1:
		reasons = append(reasons, ReasonMonetaryWeak)
	}
	return RFMScore{
		Recency: recency, Frequency: frequency, Monetary: monetary,
		Composite: recency + frequency + monetary, ReasonCodes: reasons,
	}
}

// ---------------------------------------------------------------------------
// Churn V1
// ---------------------------------------------------------------------------

// ChurnInput is one relationship's observed commercial history.
//
// Every field is an observed order fact. Nothing here is derived from when a
// profile row happened to be created, which would invent a history the vendor
// never saw.
type ChurnInput struct {
	ProfileID           string
	CompletedOrderCount int
	FirstOrderAt        time.Time
	LastOrderAt         time.Time
	AsOf                time.Time
}

// ChurnScore is the computed output for one relationship.
type ChurnScore struct {
	Risk        int
	Band        string
	ReasonCodes []string
}

// ChurnMinimumOrders is the point below which cadence is undefined.
//
// With one observed order there is no interval to be late against, so a
// single-purchase relationship is INSUFFICIENT_HISTORY rather than high risk.
// That is the rule that stops a brand-new customer being labelled a churn
// risk on their first day, which would be both wrong and actively harmful:
// the follow-up list would fill with people who just arrived.
const ChurnMinimumOrders = 2

// Churn band thresholds. Versioned with the model; changing one is a new
// model version, never a silent reinterpretation of stored scores.
const (
	churnBandMediumFrom   = 25
	churnBandHighFrom     = 50
	churnBandCriticalFrom = 75
)

// ChurnBandFor maps a risk score onto its band.
func ChurnBandFor(risk int) string {
	switch {
	case risk >= churnBandCriticalFrom:
		return RiskCritical
	case risk >= churnBandHighFrom:
		return RiskHigh
	case risk >= churnBandMediumFrom:
		return RiskMedium
	default:
		return RiskLow
	}
}

// computeChurn scores one relationship. It is pure and integer-only: there is
// no floating-point risk anywhere, because a decimal would invite reading the
// number as a likelihood.
func computeChurn(in ChurnInput) (ChurnScore, bool) {
	if in.CompletedOrderCount < ChurnMinimumOrders ||
		in.FirstOrderAt.IsZero() || in.LastOrderAt.IsZero() {
		return ChurnScore{}, false
	}
	spanDays := int64(in.LastOrderAt.Sub(in.FirstOrderAt) / (24 * time.Hour))
	if spanDays < 0 {
		spanDays = 0
	}
	intervals := int64(in.CompletedOrderCount - 1)
	cadence := spanDays / intervals
	if cadence < 1 {
		cadence = 1
	}
	elapsed := int64(in.AsOf.Sub(in.LastOrderAt) / (24 * time.Hour))
	if elapsed < 0 {
		elapsed = 0
	}

	// Integer ratio arithmetic throughout: comparisons are written as
	// multiplications so no float is introduced to represent "how overdue".
	reasons := make([]string, 0, 4)
	var risk int
	switch {
	case elapsed <= cadence:
		risk = 0
		reasons = append(reasons, ReasonOnExpectedCadence)
	case elapsed <= 2*cadence:
		risk = 25
		reasons = append(reasons, ReasonRecencyIncreased)
	case elapsed <= 3*cadence:
		risk = 50
		reasons = append(reasons, ReasonOverdueVersusCadence)
	case elapsed <= 5*cadence:
		risk = 70
		reasons = append(reasons, ReasonOverdueVersusCadence)
	default:
		risk = 85
		reasons = append(reasons, ReasonSeverelyOverdue)
	}

	switch {
	case elapsed >= 365:
		risk += 15
		reasons = append(reasons, ReasonExtendedInactivity)
	case elapsed >= 180:
		risk += 10
		reasons = append(reasons, ReasonLongInactivity)
	}

	if in.CompletedOrderCount >= 10 {
		switch {
		case elapsed > 2*cadence:
			risk += 5
			reasons = append(reasons, ReasonHighEngagementLapsed)
		case elapsed <= cadence:
			risk -= 5
			reasons = append(reasons, ReasonSustainedEngagement)
		}
	}

	if risk < 0 {
		risk = 0
	}
	if risk > 100 {
		risk = 100
	}
	sort.Strings(reasons)
	return ChurnScore{Risk: risk, Band: ChurnBandFor(risk), ReasonCodes: reasons}, true
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// scopeStateKey is the deterministic identity of one (business, model, scope,
// branch) pointer row, so the upsert never needs to invent an id.
func scopeStateKey(businessID, modelKey, scope, branchID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{businessID, modelKey, scope, branchID}, "\x00")))
	return "crmiss_" + hex.EncodeToString(sum[:16])
}

// intelligenceDependency is the segment dependency key for one model's fields,
// used so a score change restakes only the segments that actually read it.
func intelligenceDependency(modelKey string) string {
	switch modelKey {
	case ModelRFM:
		return "intelligence.rfm"
	case ModelChurn:
		return "intelligence.churn"
	}
	return ""
}

func fmtScopeLabel(scope, branchID string) string {
	if scope == IntelligenceScopeBranch {
		return fmt.Sprintf("%s:%s", scope, branchID)
	}
	return scope
}
