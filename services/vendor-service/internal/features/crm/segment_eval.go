package crm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// EvaluationConfig bounds the materializer's appetite for database work. Every
// value is configuration rather than a magic number, because these are the
// knobs an operator reaches for when a large tenant makes evaluation expensive.
type EvaluationConfig struct {
	// BatchSize is how many membership rows one insert pass writes.
	BatchSize int
	// PreviewSampleDefault and PreviewSampleMax bound the preview sample.
	PreviewSampleDefault int
	PreviewSampleMax     int
	// PreviewCountBudget caps how many candidate rows a synchronous preview
	// will count before reporting a lower bound instead of an exact figure. A
	// preview must answer quickly or say why it cannot; it must never hang.
	PreviewCountBudget int
	// LeaseTTL is how long a worker owns a claimed evaluation.
	LeaseTTL time.Duration
	// MaxAttempts before an evaluation is dead-lettered.
	MaxAttempts int
	// RetryBackoff is the base delay between attempts.
	RetryBackoff time.Duration
}

// DefaultEvaluationConfig returns the initial tuning.
func DefaultEvaluationConfig() EvaluationConfig {
	return EvaluationConfig{
		BatchSize:            500,
		PreviewSampleDefault: 25,
		PreviewSampleMax:     100,
		PreviewCountBudget:   50000,
		LeaseTTL:             2 * time.Minute,
		MaxAttempts:          5,
		RetryBackoff:         30 * time.Second,
	}
}

// GenerationRetainer decides whether a superseded membership generation may be
// cleaned up.
//
// Section 8B always answers "yes": nothing references a past generation yet.
// The port exists so Section 8E can pin the exact generation a campaign was
// launched against without this file having to learn about campaigns — and so
// that cleanup can never silently delete an audience a campaign is still
// reconciling against.
type GenerationRetainer interface {
	Retained(ctx context.Context, businessID, segmentID string, generation int64) (bool, error)
}

type noRetention struct{}

func (noRetention) Retained(context.Context, string, string, int64) (bool, error) { return false, nil }

// ---------------------------------------------------------------------------
// Candidate query construction
// ---------------------------------------------------------------------------

// candidateQuery is the compiled SELECT for one rule at one scope.
type candidateQuery struct {
	from  string
	where string
	args  []any
}

// buildCandidateQuery renders the FROM/WHERE for a rule.
//
// The mandatory filters are applied outside the vendor rule and cannot be
// switched off by it: redacted and merged-away relationships are structurally
// excluded, manual exclusions are subtracted, and a branch-scoped segment is
// restricted to relationships that actually exist at that branch. A vendor can
// write any rule they like inside those walls; they cannot move the walls.
func (s *Service) buildCandidateQuery(businessID string, segment Segment, rule *ValidatedRule, asOf time.Time) candidateQuery {
	scope := segment.Scope()
	builder := &sqlBuilder{asOf: asOf, scope: scope, branchID: scope.BranchID, availability: s.registry.availability}

	businessArg := builder.bind(businessID)
	from := "vendor_customer_profiles p"
	conditions := []string{
		"p.business_id = " + businessArg,
		"p.privacy_state <> 'REDACTED'",
		"p.merge_state <> 'MERGED_SOURCE'",
	}
	if scope.Branch() {
		branchArg := builder.bind(scope.BranchID)
		from += " JOIN crm_customer_branch_relations r ON r.business_id = p.business_id" +
			" AND r.profile_id = p.id AND r.branch_id = " + branchArg
	}
	// An archived relationship is out of the working set by default. A rule
	// that deliberately names profile.status opts back in, so an operational
	// audience over archived relationships stays possible while the default
	// stays safe.
	if !segmentDependsOn(rule.Dependencies, "profile.status") {
		conditions = append(conditions, "p.status <> 'ARCHIVED'")
	}
	segmentArg := builder.bind(segment.ID)
	conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM crm_segment_exclusions x"+
		" WHERE x.business_id = p.business_id AND x.segment_id = "+segmentArg+" AND x.profile_id = p.id)")

	// `IS TRUE` collapses SQL's third value: a rule that could not be decided
	// for this relationship — because the data it needs was never observed —
	// selects nobody rather than everybody.
	conditions = append(conditions, "("+CompileRule(rule, builder)+") IS TRUE")

	return candidateQuery{from: from, where: strings.Join(conditions, " AND "), args: builder.args}
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

// PreviewInput is an unsaved definition to evaluate.
type PreviewInput struct {
	SegmentType string
	ScopeType   string
	BranchRef   string
	Rule        json.RawMessage
	SampleLimit int
}

// PreviewSample is one privacy-safe sample row.
type PreviewSample struct {
	ProfileRef                string   `json:"profile_ref"`
	DisplayLabel              *string  `json:"display_label"`
	CommercialProjectionState string   `json:"commercial_projection_state"`
	MatchedConditions         []string `json:"matched_conditions"`
	UnavailableConditions     []string `json:"unavailable_conditions"`
}

// PreviewResult is the answer to "who would this rule select".
type PreviewResult struct {
	DefinitionHash    string          `json:"definition_hash"`
	EvaluationAsOf    time.Time       `json:"evaluation_as_of"`
	MatchCount        int             `json:"match_count"`
	CountState        string          `json:"count_state"`
	Scope             string          `json:"scope"`
	BranchRef         *string         `json:"branch_ref,omitempty"`
	Sample            []PreviewSample `json:"sample"`
	RuleExplanation   string          `json:"rule_explanation"`
	UnavailableFields []string        `json:"unavailable_fields"`
	Warnings          []string        `json:"warnings"`
}

// Count states. EXACT is a real count; AT_LEAST means the count budget was
// reached, which is reported honestly rather than being turned into a guess.
const (
	CountExact   = "EXACT"
	CountAtLeast = "AT_LEAST"
)

// PreviewSegment evaluates an unsaved rule without persisting anything.
//
// A branch-scoped caller can only preview a branch-scoped rule for a branch
// they hold, so preview cannot be used to learn the size of the business-wide
// customer base — the count is the disclosure, not just the sample.
func (s *Service) PreviewSegment(ctx context.Context, actor Actor, scope Scope, in PreviewInput) (PreviewResult, error) {
	if !s.flags.SegmentationEnabled || s.flags.CommerceKillSwitch {
		return PreviewResult{}, errSegmentationDisabled()
	}
	scopeType := strings.ToUpper(strings.TrimSpace(in.ScopeType))
	if scopeType == "" {
		scopeType = ScopeBusiness
	}
	var branchID *string
	if branch := strings.TrimSpace(in.BranchRef); branch != "" {
		branchID = &branch
	}
	if err := authorizeObjectScope(scope, scopeType, branchID, CodeSegmentScopeDenied); err != nil {
		return PreviewResult{}, err
	}
	if scopeType == ScopeBusiness {
		branchID = nil
	}
	if len(in.Rule) == 0 {
		return PreviewResult{}, crmError(apperrors.Validation("A rule is required to preview.",
			[]apperrors.FieldViolation{{Field: "rule", Message: "A rule is required."}}), CodeSegmentRuleRequired)
	}

	segmentScope := SegmentScope{Type: scopeType, BranchID: derefOr(branchID)}
	rule, err := ValidateRule(in.Rule, s.registry, s.ruleLimits, segmentScope)
	if err != nil {
		s.metrics.Inc("crm_segment_rule_rejections_total")
		return PreviewResult{}, err
	}
	if err = s.validateRuleReferences(ctx, s.db, actor.VendorID, segmentScope, rule); err != nil {
		return PreviewResult{}, err
	}

	sampleLimit := in.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = s.evaluation.PreviewSampleDefault
	}
	if sampleLimit > s.evaluation.PreviewSampleMax {
		sampleLimit = s.evaluation.PreviewSampleMax
	}

	asOf := s.now()
	// Preview runs against a synthetic segment: it has no id yet, so exclusions
	// cannot apply and the empty id simply matches no exclusion row.
	pseudo := Segment{ID: "", BusinessID: actor.VendorID, ScopeType: scopeType, BranchID: branchID}
	query := s.buildCandidateQuery(actor.VendorID, pseudo, rule, asOf)

	started := time.Now()
	count, state, err := s.countCandidates(ctx, query)
	if err != nil {
		return PreviewResult{}, err
	}
	samples, err := s.sampleCandidates(ctx, query, rule, sampleLimit, asOf, pseudo)
	if err != nil {
		return PreviewResult{}, err
	}
	s.metrics.Inc("crm_segment_preview_total", "scope", scopeType)
	s.metrics.Add("crm_segment_preview_duration_seconds", time.Since(started).Seconds())

	result := PreviewResult{
		DefinitionHash: rule.Hash, EvaluationAsOf: asOf, MatchCount: count, CountState: state,
		Scope: scopeType, BranchRef: branchID, Sample: samples,
		RuleExplanation: DescribeRule(rule), UnavailableFields: rule.UnavailableFields,
		Warnings: []string{},
	}
	if len(rule.UnavailableFields) > 0 {
		result.Warnings = append(result.Warnings,
			"Some fields in this rule have no integrated data source yet, so conditions using them match nobody.")
	}
	if rule.NegatedAvailabilityGuarded {
		result.Warnings = append(result.Warnings,
			"This rule negates a condition whose data may be unobserved; customers with no data are not selected by the negation.")
	}
	if result.Sample == nil {
		result.Sample = []PreviewSample{}
	}
	if result.UnavailableFields == nil {
		result.UnavailableFields = []string{}
	}
	return result, nil
}

// countCandidates counts matches up to the configured budget.
//
// Beyond the budget the answer is a lower bound, explicitly labelled. That is
// deliberately not an estimate: an invented number in an audience preview is
// worse than an honest "at least N", because it will be believed.
func (s *Service) countCandidates(ctx context.Context, query candidateQuery) (int, string, error) {
	budget := s.evaluation.PreviewCountBudget
	sql := `SELECT count(*) FROM (SELECT 1 FROM ` + query.from + ` WHERE ` + query.where +
		` LIMIT ` + itoa(budget+1) + `) bounded`
	var count int
	if err := s.db.QueryRow(ctx, sql, query.args...).Scan(&count); err != nil {
		return 0, "", translateEvaluationError(err)
	}
	if count > budget {
		return budget, CountAtLeast, nil
	}
	return count, CountExact, nil
}

// sampleCandidates returns a bounded, privacy-safe sample with per-condition
// results.
//
// The sample carries a relationship reference, an optional safe display label
// and the projection state — never an email address, phone number or any other
// contact detail, because CRM has none to leak.
func (s *Service) sampleCandidates(ctx context.Context, query candidateQuery, rule *ValidatedRule,
	limit int, asOf time.Time, segment Segment) ([]PreviewSample, error) {
	sql := `SELECT p.id, p.safe_display_label, p.commercial_projection_state
		FROM ` + query.from + ` WHERE ` + query.where + ` ORDER BY p.id ASC LIMIT ` + itoa(limit)
	rows, err := s.db.Query(ctx, sql, query.args...)
	if err != nil {
		return nil, translateEvaluationError(err)
	}
	defer rows.Close()

	samples := make([]PreviewSample, 0, limit)
	ids := make([]string, 0, limit)
	for rows.Next() {
		var sample PreviewSample
		if err = rows.Scan(&sample.ProfileRef, &sample.DisplayLabel, &sample.CommercialProjectionState); err != nil {
			return nil, err
		}
		sample.MatchedConditions = []string{}
		sample.UnavailableConditions = []string{}
		samples = append(samples, sample)
		ids = append(ids, sample.ProfileRef)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return samples, nil
	}
	explanations, err := s.explainSample(ctx, segment, rule, ids, asOf)
	if err != nil {
		return nil, err
	}
	for index := range samples {
		if explanation, found := explanations[samples[index].ProfileRef]; found {
			samples[index].MatchedConditions = explanation.matched
			samples[index].UnavailableConditions = explanation.unavailable
		}
	}
	return samples, nil
}

type conditionOutcome struct {
	matched     []string
	unavailable []string
}

// explainSample evaluates each condition separately for a bounded set of
// relationships.
//
// The explanation is derived by running the very expressions the rule compiles
// to, so it cannot claim a condition matched when the evaluator disagreed. It
// reports which conditions matched and which could not be decided — never the
// underlying values, which the caller may not be entitled to see.
func (s *Service) explainSample(ctx context.Context, segment Segment, rule *ValidatedRule,
	ids []string, asOf time.Time) (map[string]conditionOutcome, error) {
	scope := segment.Scope()
	builder := &sqlBuilder{asOf: asOf, scope: scope, branchID: scope.BranchID, availability: s.registry.availability}
	businessArg := builder.bind(segment.BusinessID)
	idsArg := builder.bind(ids)

	var conditions []*Condition
	collect := func(condition *Condition) { conditions = append(conditions, condition) }
	walkConditions(rule.Root, collect)
	walkConditions(rule.Exclusion, collect)
	if len(conditions) == 0 {
		return map[string]conditionOutcome{}, nil
	}

	selects := []string{"p.id"}
	for _, condition := range conditions {
		selects = append(selects, "("+compileCondition(condition, builder)+")")
	}
	from := "vendor_customer_profiles p"
	if scope.Branch() {
		from += " LEFT JOIN crm_customer_branch_relations r ON r.business_id = p.business_id" +
			" AND r.profile_id = p.id AND r.branch_id = " + builder.bind(scope.BranchID)
	}
	sql := "SELECT " + strings.Join(selects, ", ") + " FROM " + from +
		" WHERE p.business_id = " + businessArg + " AND p.id = ANY(" + idsArg + ")"

	rows, err := s.db.Query(ctx, sql, builder.args...)
	if err != nil {
		return nil, translateEvaluationError(err)
	}
	defer rows.Close()

	out := map[string]conditionOutcome{}
	for rows.Next() {
		targets := make([]any, len(conditions)+1)
		var id string
		targets[0] = &id
		results := make([]*bool, len(conditions))
		for index := range conditions {
			targets[index+1] = &results[index]
		}
		if err = rows.Scan(targets...); err != nil {
			return nil, err
		}
		outcome := conditionOutcome{matched: []string{}, unavailable: []string{}}
		for index, condition := range conditions {
			switch {
			case results[index] == nil:
				outcome.unavailable = append(outcome.unavailable, condition.Path)
			case *results[index]:
				outcome.matched = append(outcome.matched, condition.Path)
			}
		}
		out[id] = outcome
	}
	return out, rows.Err()
}

// translateEvaluationError maps a database failure into a safe envelope. The
// underlying message never reaches the client: it would disclose table and
// column names the rule DSL deliberately hides.
func translateEvaluationError(err error) error {
	if err == nil {
		return nil
	}
	return crmError(apperrors.Internal("The segment rule could not be evaluated.", err), CodeSegmentEvalFailed)
}

// ---------------------------------------------------------------------------
// Materialization
// ---------------------------------------------------------------------------

// Evaluation is one durable materialization run.
type Evaluation struct {
	ID                       string
	BusinessID               string
	SegmentID                string
	SegmentDefinitionVersion int64
	DefinitionHash           *string
	Generation               int64
	Status                   string
	EvaluationAsOf           time.Time
	StartedAt                *time.Time
	CompletedAt              *time.Time
	CandidateCount           *int
	MemberCount              *int
	ExcludedCount            *int
	ErrorCode                *string
	AttemptCount             int
}

// EvaluationView is the safe wire projection of an evaluation.
type EvaluationView struct {
	EvaluationRef     string     `json:"evaluation_ref"`
	SegmentRef        string     `json:"segment_ref"`
	DefinitionVersion int64      `json:"definition_version"`
	Generation        int64      `json:"generation"`
	Status            string     `json:"status"`
	EvaluationAsOf    time.Time  `json:"evaluation_as_of"`
	MemberCount       *int       `json:"member_count,omitempty"`
	ErrorCode         *string    `json:"error_code,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	AttemptCount      int        `json:"attempt_count"`
}

// RequestMaterialization enqueues a rebuild of a dynamic segment's membership.
//
// Requesting a rebuild that is already queued or running for the same
// definition returns the run in flight instead of stacking duplicates, so a
// user hammering the button cannot create a queue of identical work.
func (s *Service) RequestMaterialization(ctx context.Context, actor Actor, scope Scope, segmentID string) (Evaluation, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return Evaluation{}, err
	}
	if !s.flags.SegmentMaterializationEnabled {
		return Evaluation{}, crmError(apperrors.Unavailable("Segment materialization is temporarily disabled.", nil), CodeFeatureDisabled)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Evaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	segment, err := s.loadSegmentForUpdate(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return Evaluation{}, err
	}
	if err = s.assertSegmentVisible(scope, segment); err != nil {
		return Evaluation{}, err
	}
	if err = authorizeObjectScope(scope, segment.ScopeType, segment.BranchID, CodeSegmentScopeDenied); err != nil {
		return Evaluation{}, err
	}
	if !segment.Dynamic() {
		return Evaluation{}, errDynamicOnly()
	}
	if segment.Status == SegmentArchived {
		return Evaluation{}, errSegmentState("An archived segment cannot be materialized.")
	}

	evaluation, err := s.enqueueEvaluationTx(ctx, tx, actor, segment)
	if err != nil {
		return Evaluation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Evaluation{}, err
	}
	return evaluation, nil
}

// enqueueEvaluationTx queues a run, reusing an in-flight one for the same
// definition.
func (s *Service) enqueueEvaluationTx(ctx context.Context, tx pgx.Tx, actor Actor, segment Segment) (Evaluation, error) {
	existing, err := scanEvaluation(tx.QueryRow(ctx, `SELECT `+evaluationCols+` FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND status IN ('QUEUED','RUNNING')
		  AND segment_definition_version=$3 ORDER BY generation DESC LIMIT 1`,
		segment.BusinessID, segment.ID, segment.CurrentDefinitionVersion))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, err
	}

	var nextGeneration int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(generation),0)+1 FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2`, segment.BusinessID, segment.ID).Scan(&nextGeneration); err != nil {
		return Evaluation{}, err
	}
	evaluation, err := scanEvaluation(tx.QueryRow(ctx, `INSERT INTO crm_segment_evaluations
		(id,business_id,segment_id,segment_definition_version,definition_hash,generation,status,evaluation_as_of)
		VALUES($1,$2,$3,$4,$5,$6,'QUEUED',$7) RETURNING `+evaluationCols,
		newID("crmev_"), segment.BusinessID, segment.ID, segment.CurrentDefinitionVersion,
		segment.DefinitionHash, nextGeneration, s.now()))
	if err != nil {
		return Evaluation{}, err
	}
	// QUEUED replaces NOT_EVALUATED, but never overwrites CURRENT or STALE: the
	// previously materialized audience is still the truth until the new run
	// succeeds, and saying otherwise would hide a usable audience.
	if _, err = tx.Exec(ctx, `UPDATE crm_segments SET evaluation_status=CASE
			WHEN evaluation_status IN ('CURRENT','STALE') THEN 'STALE' ELSE 'QUEUED' END,
			stale_since=COALESCE(stale_since, CASE WHEN evaluation_status='CURRENT' THEN now() ELSE stale_since END),
			updated_at=now()
		WHERE business_id=$1 AND id=$2`, segment.BusinessID, segment.ID); err != nil {
		return Evaluation{}, err
	}
	s.metrics.Inc("crm_segment_evaluations_total", "result", "queued")
	return evaluation, nil
}

const evaluationCols = `id,business_id,segment_id,segment_definition_version,definition_hash,generation,status,
evaluation_as_of,started_at,completed_at,candidate_count,member_count,excluded_count,error_code,worker_attempt_count`

func scanEvaluation(row rowScanner) (Evaluation, error) {
	var e Evaluation
	err := row.Scan(&e.ID, &e.BusinessID, &e.SegmentID, &e.SegmentDefinitionVersion, &e.DefinitionHash,
		&e.Generation, &e.Status, &e.EvaluationAsOf, &e.StartedAt, &e.CompletedAt, &e.CandidateCount,
		&e.MemberCount, &e.ExcludedCount, &e.ErrorCode, &e.AttemptCount)
	return e, err
}

// EvaluationViewOf renders an evaluation for the wire.
func EvaluationViewOf(evaluation Evaluation) EvaluationView {
	return EvaluationView{
		EvaluationRef: evaluation.ID, SegmentRef: evaluation.SegmentID,
		DefinitionVersion: evaluation.SegmentDefinitionVersion, Generation: evaluation.Generation,
		Status: evaluation.Status, EvaluationAsOf: evaluation.EvaluationAsOf,
		MemberCount: evaluation.MemberCount, ErrorCode: evaluation.ErrorCode,
		CompletedAt: evaluation.CompletedAt, AttemptCount: evaluation.AttemptCount,
	}
}

// RunMaterializationPass claims and runs up to `limit` due evaluations.
//
// It is the worker entry point, ticked by internal/jobs. Claiming is a
// lease-with-skip-locked so several workers can run concurrently without ever
// evaluating the same segment twice, and a worker that dies mid-run simply lets
// its lease lapse for another to pick up.
func (s *Service) RunMaterializationPass(ctx context.Context, workerID string, limit int) (int, error) {
	if !s.flags.SegmentationEnabled || !s.flags.SegmentMaterializationEnabled || s.flags.CommerceKillSwitch {
		return 0, nil
	}
	processed := 0
	for processed < limit {
		evaluation, claimed, err := s.claimEvaluation(ctx, workerID)
		if err != nil {
			return processed, err
		}
		if !claimed {
			return processed, nil
		}
		processed++
		if err = s.runEvaluation(ctx, evaluation, workerID); err != nil {
			if failErr := s.failEvaluation(ctx, evaluation, err); failErr != nil {
				return processed, failErr
			}
		}
	}
	return processed, nil
}

// ReplayMaterialization returns a dead-lettered evaluation to the durable
// queue. It is intentionally a code-level operator hook, not a Vendor-facing
// HTTP endpoint: replaying failed infrastructure work is an operational action,
// not ordinary CRM authority.
func (s *Service) ReplayMaterialization(ctx context.Context, businessID, evaluationID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE crm_segment_evaluations SET
		status='QUEUED', worker_attempt_count=0, next_attempt_at=now(),
		lease_owner=NULL, lease_expires_at=NULL, completed_at=NULL,
		error_code=NULL, error_detail_safe=NULL, updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND status='DEAD_LETTER'`, businessID, evaluationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("segment materialization %s is not dead-lettered", evaluationID)
	}
	return nil
}

func (s *Service) claimEvaluation(ctx context.Context, workerID string) (Evaluation, bool, error) {
	lease := fmt.Sprintf("%d milliseconds", s.evaluation.LeaseTTL.Milliseconds())
	evaluation, err := scanEvaluation(s.db.QueryRow(ctx, `UPDATE crm_segment_evaluations SET
			status='RUNNING', lease_owner=$1, lease_expires_at=now() + $2::interval,
			started_at=COALESCE(started_at, now()), worker_attempt_count=worker_attempt_count+1,
			next_attempt_at=now() + $2::interval, updated_at=now(), version=version+1
		WHERE id = (
			SELECT id FROM crm_segment_evaluations
			WHERE status IN ('QUEUED','RUNNING','FAILED')
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at <= now())
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+evaluationCols, workerID, lease))
	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, false, nil
	}
	if err != nil {
		return Evaluation{}, false, err
	}
	return evaluation, true, nil
}

// runEvaluation materializes one segment into a fresh generation and swaps it
// in atomically.
//
// The order matters and is the whole point of the design: the new generation is
// built alongside the old one, the swap is a single committed transaction, and
// the previous generation is only released afterwards. A failure at any point
// before the swap leaves the last good audience exactly as it was.
func (s *Service) runEvaluation(ctx context.Context, evaluation Evaluation, workerID string) error {
	actor := Actor{VendorID: evaluation.BusinessID, CorrelationID: "segment-materializer"}
	started := time.Now()

	segment, err := s.loadSegment(ctx, s.db, evaluation.BusinessID, evaluation.SegmentID)
	if err != nil {
		return err
	}
	if segment.Status == SegmentArchived || !segment.Dynamic() {
		// The definition changed out from under the run. Retiring it is correct:
		// materializing a retired segment would resurrect a dead audience.
		return s.supersedeEvaluation(ctx, evaluation)
	}
	if segment.CurrentDefinitionVersion != evaluation.SegmentDefinitionVersion {
		// A newer definition exists and has its own queued run.
		return s.supersedeEvaluation(ctx, evaluation)
	}

	version, err := s.CurrentVersion(ctx, s.db, evaluation.BusinessID, evaluation.SegmentID, evaluation.SegmentDefinitionVersion)
	if err != nil {
		return err
	}
	rule, err := s.ruleFromVersion(version)
	if err != nil {
		return err
	}

	// A previous attempt may have written part of this generation before dying.
	// Restarting from a clean slate is safe precisely because the generation is
	// not the live one.
	heartbeatCtx, stopHeartbeat := s.startEvaluationHeartbeat(ctx, evaluation, workerID)
	if _, err = s.db.Exec(heartbeatCtx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
		evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation); err != nil {
		_ = stopHeartbeat()
		return err
	}

	query := s.buildCandidateQuery(evaluation.BusinessID, segment, rule, evaluation.EvaluationAsOf)
	written, err := s.writeGeneration(heartbeatCtx, evaluation, query)
	heartbeatErr := stopHeartbeat()
	if heartbeatErr != nil {
		return heartbeatErr
	}
	if err != nil {
		return err
	}

	memberCount, err := s.swapGeneration(ctx, actor, evaluation, written, query)
	if err != nil {
		return err
	}
	s.metrics.Inc("crm_segment_evaluations_total", "result", "success")
	s.metrics.Add("crm_segment_evaluation_duration_seconds", time.Since(started).Seconds())
	s.metrics.Add("crm_segment_members_current", float64(memberCount), "source", MembershipDynamic)
	return s.cleanupSupersededGenerations(ctx, evaluation)
}

// startEvaluationHeartbeat keeps a claimed evaluation leased while its
// generation is being built. Without renewal, an evaluation that legitimately
// exceeds LeaseTTL could be adopted by a second worker and evaluated twice.
//
// The heartbeat stops before the short atomic generation swap. Its final
// successful renewal leaves a full LeaseTTL for that transaction, while also
// avoiding a race with the swap clearing lease_owner on success.
func (s *Service) startEvaluationHeartbeat(ctx context.Context, evaluation Evaluation, workerID string) (context.Context, func() error) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	interval := s.evaluation.LeaseTTL / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	lease := fmt.Sprintf("%d milliseconds", s.evaluation.LeaseTTL.Milliseconds())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				tag, err := s.db.Exec(runCtx, `UPDATE crm_segment_evaluations SET
					lease_expires_at=now() + $1::interval, updated_at=now()
					WHERE business_id=$2 AND id=$3 AND status='RUNNING' AND lease_owner=$4`,
					lease, evaluation.BusinessID, evaluation.ID, workerID)
				if err != nil {
					done <- err
					cancel()
					return
				}
				if tag.RowsAffected() != 1 {
					done <- fmt.Errorf("segment evaluation lease lost")
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, func() error {
		cancel()
		return <-done
	}
}

// ruleFromVersion rebuilds a ValidatedRule from a stored definition.
//
// It re-runs the same validator a client's rule went through. A stored rule
// that no longer validates — because a field was withdrawn from the registry,
// say — fails the run loudly instead of being evaluated under a guessed
// interpretation.
func (s *Service) ruleFromVersion(version SegmentVersion) (*ValidatedRule, error) {
	if len(version.RuleJSON) == 0 {
		return nil, crmError(apperrors.Conflict("This segment has no rule to evaluate.", nil), CodeSegmentRuleRequired)
	}
	document := map[string]any{"schema_version": RuleSchemaVersion, "root": json.RawMessage(version.RuleJSON)}
	if len(version.ExclusionJSON) > 0 {
		document["exclusion"] = json.RawMessage(version.ExclusionJSON)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	scope := SegmentScope{Type: version.ScopeType, BranchID: derefOr(version.BranchID)}
	return ValidateRule(raw, s.registry, s.ruleLimits, scope)
}

// writeGeneration inserts matching relationships into the new generation in
// bounded keyset batches, so a large tenant never produces one enormous
// statement or one enormous transaction.
func (s *Service) writeGeneration(ctx context.Context, evaluation Evaluation, query candidateQuery) (int, error) {
	batch := s.evaluation.BatchSize
	if batch <= 0 {
		batch = 500
	}
	cursor := ""
	total := 0
	base := len(query.args)
	// The membership id is derived from (segment, generation, profile) rather
	// than randomly generated, so re-running a partially written generation
	// after a crash reproduces exactly the same rows instead of duplicating
	// them under new ids.
	// Every placeholder carries an explicit cast: the id expression concatenates
	// parameters, and Postgres cannot infer a type for a bare parameter used
	// only inside `||`.
	sql := `INSERT INTO crm_segment_memberships
			(id,business_id,segment_id,profile_id,membership_source,evaluation_id,generation,segment_definition_version,matched_at)
			SELECT 'crmsm_' || md5($` + itoa(base+2) + `::text || ':' || ($` + itoa(base+3) + `::bigint)::text || ':' || c.id),
				$` + itoa(base+1) + `::text, $` + itoa(base+2) + `::text, c.id, 'DYNAMIC', $` + itoa(base+6) + `::text,
				$` + itoa(base+3) + `::bigint, $` + itoa(base+4) + `::bigint, now()
			FROM (SELECT p.id FROM ` + query.from + ` WHERE ` + query.where +
		` AND p.id > $` + itoa(base+5) + `::text ORDER BY p.id ASC LIMIT ` + itoa(batch) + `) c
			ON CONFLICT (business_id, segment_id, profile_id, generation) DO NOTHING
			RETURNING profile_id`

	for {
		args := append(append([]any{}, query.args...), evaluation.BusinessID, evaluation.SegmentID,
			evaluation.Generation, evaluation.SegmentDefinitionVersion, cursor, evaluation.ID)

		rows, err := s.db.Query(ctx, sql, args...)
		if err != nil {
			return total, translateEvaluationError(err)
		}
		last := ""
		written := 0
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return total, err
			}
			written++
			if id > last {
				last = id
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return total, err
		}
		total += written
		if written < batch {
			return total, nil
		}
		cursor = last
	}
}

// swapGeneration is the atomic hand-over from the previous audience to the new
// one.
//
// The final privacy sweep inside this transaction is what closes the
// materialize-versus-redact race: a REDACT that committed while the generation
// was being written is seen here, and the affected rows are removed before the
// generation ever becomes visible.
func (s *Service) swapGeneration(ctx context.Context, actor Actor, evaluation Evaluation, candidates int, query candidateQuery) (int, error) {
	return s.swapGenerationWithHook(ctx, actor, evaluation, candidates, query, nil)
}

// swapGenerationWithHook exists so real-PostgreSQL concurrency tests can pause
// at the exact certification boundary. The hook is never used by production;
// correctness comes from database row locks and the transaction, not this
// process-local test synchronizer.
func (s *Service) swapGenerationWithHook(ctx context.Context, actor Actor, evaluation Evaluation, candidates int,
	query candidateQuery, afterCertification func()) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock every profile represented by the candidate generation in stable id
	// order. ApplyPrivacy and ApplyIdentityLink take the same profile locks before
	// changing privacy/merge state or touching segment rows. Consequently either
	// their transaction commits first and this certification sees the new state,
	// or this entire pointer swap commits first and their immediate-removal path
	// runs before their operation can complete.
	lockedRows, err := tx.Query(ctx, `/* crm_generation_profile_fence */ SELECT p.id FROM vendor_customer_profiles p
		JOIN crm_segment_memberships m ON m.business_id=p.business_id AND m.profile_id=p.id
		WHERE m.business_id=$1 AND m.segment_id=$2 AND m.generation=$3
		ORDER BY p.id FOR UPDATE OF p`, evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation)
	if err != nil {
		return 0, err
	}
	for lockedRows.Next() {
		var profileID string
		if err = lockedRows.Scan(&profileID); err != nil {
			lockedRows.Close()
			return 0, err
		}
	}
	lockedRows.Close()
	if err = lockedRows.Err(); err != nil {
		return 0, err
	}

	// Re-evaluate the complete compiled rule after acquiring the privacy/merge
	// fence. This covers structural redaction/merge and effective consent
	// eligibility (including restriction and marketing suppression), rather than
	// relying on the earlier batch query or a narrow final DELETE predicate.
	base := len(query.args)
	certificationArgs := append(append([]any{}, query.args...), evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation)
	removed, err := tx.Exec(ctx, `DELETE FROM crm_segment_memberships m
		WHERE m.business_id=$`+itoa(base+1)+` AND m.segment_id=$`+itoa(base+2)+` AND m.generation=$`+itoa(base+3)+`
		  AND NOT EXISTS (SELECT 1 FROM `+query.from+` WHERE `+query.where+` AND p.id=m.profile_id)`, certificationArgs...)
	if err != nil {
		return 0, err
	}
	if removed.RowsAffected() > 0 {
		s.metrics.Add("crm_segment_privacy_removals_total", float64(removed.RowsAffected()))
	}

	var memberCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
		evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation).Scan(&memberCount); err != nil {
		return 0, err
	}
	if afterCertification != nil {
		afterCertification()
	}

	if _, err = tx.Exec(ctx, `UPDATE crm_segment_evaluations SET status='SUCCESS', completed_at=now(),
			candidate_count=$1, member_count=$2, excluded_count=$3, error_code=NULL, error_detail_safe=NULL,
			lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE business_id=$4 AND id=$5`,
		candidates, memberCount, candidates-memberCount, evaluation.BusinessID, evaluation.ID); err != nil {
		return 0, err
	}
	// The segment only points at the new generation once its membership is
	// complete and privacy-swept, and only if the definition it was built for is
	// still the current one.
	tag, err := tx.Exec(ctx, `UPDATE crm_segments SET current_evaluation_id=$1, current_member_count=$2,
			evaluation_status='CURRENT', last_evaluated_at=now(), stale_since=NULL,
			last_evaluation_error_code=NULL, updated_at=now(), version=version+1
		WHERE business_id=$3 AND id=$4 AND current_definition_version=$5 AND status <> 'ARCHIVED'`,
		evaluation.ID, memberCount, evaluation.BusinessID, evaluation.SegmentID, evaluation.SegmentDefinitionVersion)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		// The definition moved on mid-run. The run's own rows stay for audit but
		// never become the live audience.
		if _, err = tx.Exec(ctx, `UPDATE crm_segment_evaluations SET status='SUPERSEDED', updated_at=now()
			WHERE business_id=$1 AND id=$2`, evaluation.BusinessID, evaluation.ID); err != nil {
			return 0, err
		}
		if err = tx.Commit(ctx); err != nil {
			return 0, err
		}
		return memberCount, nil
	}

	if err = s.auditTx(ctx, tx, actor, "vendor.crm.segment.materialized", "crm_segment", evaluation.SegmentID, "",
		nil, map[string]any{"generation": evaluation.Generation, "member_count": memberCount,
			"definition_version": evaluation.SegmentDefinitionVersion}, ""); err != nil {
		return 0, err
	}
	// The event carries the audience's identity, never its contents: a member
	// list on an event bus would be a customer export nobody asked for.
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.segment.materialized", "crm_segment", evaluation.SegmentID,
		evaluation.Generation, map[string]any{
			"segment_ref": evaluation.SegmentID, "evaluation_ref": evaluation.ID,
			"definition_version": evaluation.SegmentDefinitionVersion, "definition_hash": evaluation.DefinitionHash,
			"generation": evaluation.Generation, "member_count": memberCount,
			"evaluated_at": evaluation.EvaluationAsOf,
		}); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return memberCount, nil
}

// cleanupSupersededGenerations releases membership rows from generations no
// longer in use, subject to the retention port.
func (s *Service) cleanupSupersededGenerations(ctx context.Context, evaluation Evaluation) error {
	rows, err := s.db.Query(ctx, `SELECT generation FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND generation < $3 AND status <> 'SUPERSEDED'`,
		evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation)
	if err != nil {
		return err
	}
	var generations []int64
	for rows.Next() {
		var generation int64
		if err = rows.Scan(&generation); err != nil {
			rows.Close()
			return err
		}
		generations = append(generations, generation)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}

	for _, generation := range generations {
		if err = s.releaseGeneration(ctx, evaluation, generation); err != nil {
			return err
		}
	}
	return nil
}

// releaseGeneration retires one superseded generation, under the evaluation
// row's own lock.
//
// The lock is what makes the retention question safe to ask. A consumer that
// wants to pin this generation — Section 8E's audience preparation is the first
// — takes the same lock before recording its dependency, so the two serialize:
// either the consumer commits first and the check below sees it, or this
// transaction commits first and the consumer finds the generation already
// SUPERSEDED and refuses rather than preparing from membership that is being
// deleted underneath it. Without the lock there is a window in which both
// decide the other is not there.
func (s *Service) releaseGeneration(ctx context.Context, evaluation Evaluation, generation int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM crm_segment_evaluations
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3 FOR UPDATE`,
		evaluation.BusinessID, evaluation.SegmentID, generation).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if status != RunSuccess {
		return nil
	}

	retained, err := s.retention.Retained(ctx, evaluation.BusinessID, evaluation.SegmentID, generation)
	if err != nil {
		return err
	}
	if retained {
		return nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
		evaluation.BusinessID, evaluation.SegmentID, generation); err != nil {
		return err
	}
	// The evaluation row itself survives as the audit record of what the
	// audience was; only its (now unreferenced) membership rows are released.
	if _, err = tx.Exec(ctx, `UPDATE crm_segment_evaluations SET status='SUPERSEDED', updated_at=now()
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3 AND status='SUCCESS'`,
		evaluation.BusinessID, evaluation.SegmentID, generation); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) supersedeEvaluation(ctx context.Context, evaluation Evaluation) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
		evaluation.BusinessID, evaluation.SegmentID, evaluation.Generation); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE crm_segment_evaluations SET status='SUPERSEDED', completed_at=now(),
		lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2`, evaluation.BusinessID, evaluation.ID)
	return err
}

// failEvaluation records a failed attempt and schedules a retry, or
// dead-letters after the configured ceiling.
//
// The previous successful generation is never touched here. A segment whose
// latest run failed reports FAILED alongside a member count that is still the
// last audience that actually worked — an empty audience would be a far more
// dangerous answer than a stale one.
func (s *Service) failEvaluation(ctx context.Context, evaluation Evaluation, cause error) error {
	code := CodeSegmentEvalFailed
	if appErr := apperrors.From(cause); appErr != nil && appErr.Details != nil {
		if detail, ok := appErr.Details["code"].(string); ok && detail != "" {
			code = detail
		}
	}
	status := RunFailed
	if evaluation.AttemptCount >= s.evaluation.MaxAttempts {
		status = RunDeadLetter
	}
	backoff := fmt.Sprintf("%d seconds", int64(s.evaluation.RetryBackoff.Seconds())*int64(evaluation.AttemptCount))

	if _, err := s.db.Exec(ctx, `UPDATE crm_segment_evaluations SET status=$1, error_code=$2,
			error_detail_safe=$3, lease_owner=NULL, lease_expires_at=NULL,
			next_attempt_at=now() + $4::interval, completed_at=CASE WHEN $1='DEAD_LETTER' THEN now() ELSE completed_at END,
			updated_at=now(), version=version+1
		WHERE business_id=$5 AND id=$6`,
		status, code, safeFailureDetail(cause), backoff, evaluation.BusinessID, evaluation.ID); err != nil {
		return err
	}
	// The segment reports the failure but keeps whatever membership it had.
	if _, err := s.db.Exec(ctx, `UPDATE crm_segments SET
			evaluation_status=CASE WHEN current_evaluation_id IS NULL THEN 'FAILED' ELSE 'STALE' END,
			last_evaluation_error_code=$1,
			stale_since=COALESCE(stale_since, now()), updated_at=now()
		WHERE business_id=$2 AND id=$3`, code, evaluation.BusinessID, evaluation.SegmentID); err != nil {
		return err
	}
	s.metrics.Inc("crm_segment_evaluation_failures_total", "status", status)
	return nil
}

// safeFailureDetail keeps a short, non-sensitive summary. Database messages can
// name tables and columns and are never persisted or returned.
func safeFailureDetail(cause error) string {
	if cause == nil {
		return ""
	}
	appErr := apperrors.From(cause)
	if appErr != nil && appErr.Code != apperrors.CodeInternal {
		return appErr.Message
	}
	return "The segment rule could not be evaluated."
}

// ---------------------------------------------------------------------------
// Invalidation
// ---------------------------------------------------------------------------

// markSegmentStaleTx flags one segment's materialized membership as outdated.
func (s *Service) markSegmentStaleTx(ctx context.Context, tx pgx.Tx, businessID, segmentID string) error {
	rows, err := tx.Query(ctx, `UPDATE crm_segments SET
			evaluation_status=CASE WHEN evaluation_status='CURRENT' THEN 'STALE' ELSE evaluation_status END,
			stale_since=COALESCE(stale_since, now()), updated_at=now()
		WHERE business_id=$1 AND id=$2 AND segment_type='DYNAMIC' AND status <> 'ARCHIVED'
		RETURNING id`, businessID, segmentID)
	if err != nil {
		return err
	}
	ids, err := segmentIDs(rows)
	if err != nil {
		return err
	}
	return s.enqueueInvalidatedSegmentsTx(ctx, tx, businessID, ids)
}

// invalidateSegmentsForDependency marks every dynamic segment that reads a
// given registry field as stale.
//
// Dependency tracking is what keeps a tag assignment from restaking every
// segment in the business: only segments whose stored dependency set actually
// contains the changed field are touched. Segments that do not read it stay
// CURRENT, because nothing about their answer changed.
func (s *Service) invalidateSegmentsForDependency(ctx context.Context, tx pgx.Tx, businessID, dependency string) error {
	return s.invalidateSegmentsForDependencies(ctx, tx, businessID, []string{dependency})
}

// invalidateSegmentsForDependencies marks and durably enqueues the dynamic
// definitions that read any changed field. The queue insert occurs in the same
// transaction as the projection/tag mutation, so a process crash cannot leave
// a segment labelled CURRENT after its inputs changed.
func (s *Service) invalidateSegmentsForDependencies(ctx context.Context, tx pgx.Tx, businessID string, dependencies []string) error {
	_, err := s.invalidateSegmentsForDependenciesReturningIDs(ctx, tx, businessID, dependencies)
	return err
}

// invalidateSegmentsForDependenciesReturningIDs is the dependency invalidator
// plus the exact affected segment set. Privacy handlers use the returned ids to
// remove now-ineligible current membership in the same transaction that marks
// and queues those segments; ordinary callers only need the error wrapper.
func (s *Service) invalidateSegmentsForDependenciesReturningIDs(ctx context.Context, tx pgx.Tx, businessID string, dependencies []string) ([]string, error) {
	rows, err := tx.Query(ctx, `UPDATE crm_segments g SET
			evaluation_status=CASE WHEN g.evaluation_status='CURRENT' THEN 'STALE' ELSE g.evaluation_status END,
			stale_since=COALESCE(g.stale_since, now()), updated_at=now()
		WHERE g.business_id=$1 AND g.segment_type='DYNAMIC' AND g.status <> 'ARCHIVED'
		  AND EXISTS (SELECT 1 FROM crm_segment_versions v
			WHERE v.business_id=g.business_id AND v.segment_id=g.id
			  AND v.definition_version=g.current_definition_version
			  AND v.dependency_set && $2::text[])
		RETURNING g.id`, businessID, dependencies)
	if err != nil {
		return nil, err
	}
	ids, err := segmentIDs(rows)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		s.metrics.Add("crm_segment_stale_total", float64(len(ids)), "cause", "dependency")
	}
	if err = s.enqueueInvalidatedSegmentsTx(ctx, tx, businessID, ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// invalidateSegmentsForProfile marks stale every dynamic segment the profile
// currently belongs to, plus every segment reading the changed dependencies.
func (s *Service) invalidateSegmentsForProfile(ctx context.Context, tx pgx.Tx, businessID, profileID string) error {
	rows, err := tx.Query(ctx, `UPDATE crm_segments g SET
			evaluation_status=CASE WHEN g.evaluation_status='CURRENT' THEN 'STALE' ELSE g.evaluation_status END,
			stale_since=COALESCE(g.stale_since, now()), updated_at=now()
		WHERE g.business_id=$1 AND g.segment_type='DYNAMIC' AND g.status <> 'ARCHIVED'
		  AND EXISTS (SELECT 1 FROM crm_segment_memberships m
			WHERE m.business_id=g.business_id AND m.segment_id=g.id AND m.profile_id=$2)
		RETURNING g.id`,
		businessID, profileID)
	if err != nil {
		return err
	}
	ids, err := segmentIDs(rows)
	if err != nil {
		return err
	}
	return s.enqueueInvalidatedSegmentsTx(ctx, tx, businessID, ids)
}

// invalidateAllDynamicSegmentsTx is reserved for changes whose effect is
// implicit in every rule, such as the default ACTIVE-profile filter or an
// identity merge. It is tenant-local; it never restakes other businesses.
func (s *Service) invalidateAllDynamicSegmentsTx(ctx context.Context, tx pgx.Tx, businessID string) error {
	rows, err := tx.Query(ctx, `UPDATE crm_segments SET
			evaluation_status=CASE WHEN evaluation_status='CURRENT' THEN 'STALE' ELSE evaluation_status END,
			stale_since=COALESCE(stale_since, now()), updated_at=now()
		WHERE business_id=$1 AND segment_type='DYNAMIC' AND status <> 'ARCHIVED'
		RETURNING id`, businessID)
	if err != nil {
		return err
	}
	ids, err := segmentIDs(rows)
	if err != nil {
		return err
	}
	return s.enqueueInvalidatedSegmentsTx(ctx, tx, businessID, ids)
}

func segmentIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) enqueueInvalidatedSegmentsTx(ctx context.Context, tx pgx.Tx, businessID string, segmentIDs []string) error {
	if !s.flags.SegmentMaterializationEnabled || s.flags.CommerceKillSwitch {
		return nil
	}
	actor := Actor{VendorID: businessID, CorrelationID: "segment-invalidation"}
	for _, segmentID := range segmentIDs {
		segment, err := s.loadSegment(ctx, tx, businessID, segmentID)
		if err != nil {
			return err
		}
		if _, err = s.enqueueEvaluationTx(ctx, tx, actor, segment); err != nil {
			return err
		}
	}
	return nil
}

// StaleSegmentBacklog reports how many dynamic segments are waiting to be
// rebuilt, for the metrics line.
func (s *Service) StaleSegmentBacklog(ctx context.Context) (int, error) {
	var depth int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM crm_segment_evaluations
		WHERE status IN ('QUEUED','FAILED') AND next_attempt_at <= now()`).Scan(&depth)
	return depth, err
}
