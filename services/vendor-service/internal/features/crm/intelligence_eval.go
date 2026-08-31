package crm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8H — intelligence materialization.
//
// The shape is Section 8B's, reused rather than reinvented: a durable
// evaluation row is the job, a generation is built alongside the live one, and
// a single committed transaction makes it current. A crash before that commit
// leaves the previous generation exactly as it was, which is the property that
// matters most here — an intelligence rebuild that failed halfway must never
// present as "nobody has a score" to the person looking at the dashboard.

// IntelligenceConfig tunes the materializer.
type IntelligenceConfig struct {
	BatchSize   int
	LeaseTTL    time.Duration
	MaxAttempts int
	Backoff     time.Duration
	// MaxCohort bounds one evaluation. Quintile scoring is inherently
	// cohort-relative, so the run reads the whole scored cohort; beyond this
	// bound it fails with a clear code rather than degrading unpredictably.
	MaxCohort int
}

func (c IntelligenceConfig) withDefaults() IntelligenceConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 500
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 2 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.Backoff <= 0 {
		c.Backoff = 30 * time.Second
	}
	if c.MaxCohort <= 0 {
		c.MaxCohort = 200000
	}
	return c
}

// IntelligenceEvaluation is one durable rebuild job.
type IntelligenceEvaluation struct {
	ID           string
	BusinessID   string
	BranchID     *string
	Scope        string
	ModelKey     string
	ModelVersion int
	Hash         string
	Generation   int64
	Status       string
	SourceAsOf   time.Time

	ProfileCount     int
	AvailableCount   int
	UnavailableCount int
	FailureCount     int

	ErrorCode   *string
	Attempts    int
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	Version     int64
}

// IntelligenceScopeState is the current-generation pointer for one scope and
// model, and the honest answer to "is there usable intelligence here at all".
type IntelligenceScopeState struct {
	BusinessID   string
	BranchID     *string
	Scope        string
	ModelKey     string
	Status       string
	Generation   *int64
	ModelVersion *int
	Hash         *string
	SourceAsOf   *time.Time
	ComputedAt   *time.Time
	StaleSince   *time.Time
	ErrorCode    *string

	ProfileCount     int
	AvailableCount   int
	UnavailableCount int
	Version          int64
}

const evaluationColumns = `id,business_id,branch_id,scope,model_key,model_version,definition_hash,generation,
	status,source_as_of,profile_count,available_count,unavailable_count,failure_count,error_code,
	worker_attempt_count,started_at,completed_at,created_at,version`

func scanIntelligenceEvaluation(row pgx.Row) (IntelligenceEvaluation, error) {
	var e IntelligenceEvaluation
	err := row.Scan(&e.ID, &e.BusinessID, &e.BranchID, &e.Scope, &e.ModelKey, &e.ModelVersion, &e.Hash,
		&e.Generation, &e.Status, &e.SourceAsOf, &e.ProfileCount, &e.AvailableCount, &e.UnavailableCount,
		&e.FailureCount, &e.ErrorCode, &e.Attempts, &e.StartedAt, &e.CompletedAt, &e.CreatedAt, &e.Version)
	return e, err
}

const scopeStateColumns = `business_id,branch_id,scope,model_key,status,current_generation,model_version,
	definition_hash,source_as_of,computed_at,stale_since,error_code,profile_count,available_count,
	unavailable_count,version`

func scanScopeState(row pgx.Row) (IntelligenceScopeState, error) {
	var s IntelligenceScopeState
	err := row.Scan(&s.BusinessID, &s.BranchID, &s.Scope, &s.ModelKey, &s.Status, &s.Generation,
		&s.ModelVersion, &s.Hash, &s.SourceAsOf, &s.ComputedAt, &s.StaleSince, &s.ErrorCode,
		&s.ProfileCount, &s.AvailableCount, &s.UnavailableCount, &s.Version)
	return s, err
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// Section 8H error codes.
const (
	CodeIntelligenceNotAvailable  = "CRM_INTELLIGENCE_NOT_AVAILABLE"
	CodeIntelligenceStale         = "CRM_INTELLIGENCE_STALE"
	CodeIntelligenceRebuilding    = "CRM_INTELLIGENCE_REBUILD_IN_PROGRESS"
	CodeIntelligenceModelNotFound = "CRM_INTELLIGENCE_MODEL_NOT_FOUND"
	CodeIntelligenceScopeDenied   = "CRM_INTELLIGENCE_SCOPE_DENIED"
	CodeIntelligenceDisabled      = "CRM_INTELLIGENCE_DISABLED"
)

func errIntelligenceDisabled() error {
	return crmError(apperrors.Unavailable("CRM intelligence is not enabled.", nil), CodeIntelligenceDisabled)
}

func errModelNotFound() error {
	return crmError(apperrors.NotFound("That intelligence model does not exist.", nil),
		CodeIntelligenceModelNotFound)
}

func errIntelligenceScopeDenied() error {
	// Indistinguishable from "that scope does not exist", so probing branch
	// references tells a caller nothing about the shape of the business.
	return crmError(apperrors.Forbidden("You cannot read intelligence for that scope.", nil),
		CodeIntelligenceScopeDenied)
}

func (s *Service) assertIntelligenceAvailable() error {
	if !s.flags.CRMEnabled || !s.flags.IntelligenceEnabled {
		return errIntelligenceDisabled()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Rebuild requests
// ---------------------------------------------------------------------------

// RebuildIntelligenceInput names one scope and model to recompute.
type RebuildIntelligenceInput struct {
	ModelKey  string
	ScopeType string
	BranchRef string
}

// RequestIntelligenceRebuild enqueues a durable rebuild.
//
// It is idempotent by construction rather than by key alone: an evaluation
// already queued or running for the same scope and model is returned instead
// of a second one being created. Two operators pressing rebuild produce one
// rebuild, which is what makes the button safe to press.
func (s *Service) RequestIntelligenceRebuild(ctx context.Context, actor Actor, scope Scope,
	in RebuildIntelligenceInput) (IntelligenceEvaluation, error) {
	if err := s.assertIntelligenceAvailable(); err != nil {
		return IntelligenceEvaluation{}, err
	}
	if !s.flags.WritesEnabled {
		return IntelligenceEvaluation{}, errWritesDisabled()
	}
	model, ok := ModelFor(strings.TrimSpace(in.ModelKey))
	if !ok {
		return IntelligenceEvaluation{}, errModelNotFound()
	}
	scopeType, branchID, err := s.resolveIntelligenceScope(ctx, actor, scope, in.ScopeType, in.BranchRef)
	if err != nil {
		return IntelligenceEvaluation{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IntelligenceEvaluation{}, err
	}
	defer tx.Rollback(ctx)

	// Serialise on the scope-state row so two concurrent requests cannot both
	// decide no run is in flight.
	state, err := s.lockScopeState(ctx, tx, actor.VendorID, model.Key, scopeType, branchID)
	if err != nil {
		return IntelligenceEvaluation{}, err
	}

	existing, err := scanIntelligenceEvaluation(tx.QueryRow(ctx, `SELECT `+evaluationColumns+`
		FROM crm_intelligence_evaluations
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4
		  AND status IN ('QUEUED','RUNNING')
		ORDER BY created_at DESC LIMIT 1`,
		actor.VendorID, model.Key, scopeType, derefOr(branchID)))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IntelligenceEvaluation{}, err
	}

	var nextGeneration int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(generation),0)+1 FROM crm_intelligence_evaluations
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4`,
		actor.VendorID, model.Key, scopeType, derefOr(branchID)).Scan(&nextGeneration); err != nil {
		return IntelligenceEvaluation{}, err
	}

	evaluation, err := scanIntelligenceEvaluation(tx.QueryRow(ctx, `INSERT INTO crm_intelligence_evaluations
		(id,business_id,branch_id,scope,model_key,model_version,definition_hash,generation,status,source_as_of)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'QUEUED',$9) RETURNING `+evaluationColumns,
		newID("crmeval_"), actor.VendorID, branchID, scopeType, model.Key, model.Version,
		model.DefinitionHash(), nextGeneration, s.now()))
	if err != nil {
		return IntelligenceEvaluation{}, err
	}

	if err = s.setScopeStatusTx(ctx, tx, state, IntelligenceScopeQueued, nil); err != nil {
		return IntelligenceEvaluation{}, err
	}
	if err = s.audit.Record(ctx, AuditEntry{
		ActorID: actor.StaffID, Action: "crm.intelligence.rebuild.requested",
		AggregateType: "crm_intelligence_evaluation", AggregateID: evaluation.ID,
		VendorID: actor.VendorID, BranchID: derefOr(branchID),
		After: map[string]any{"model_key": model.Key, "model_version": model.Version,
			"scope": scopeType, "generation": evaluation.Generation},
	}); err != nil {
		return IntelligenceEvaluation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IntelligenceEvaluation{}, err
	}
	s.metrics.Inc("crm_intelligence_rebuild_requested_total", "model", model.Key)
	return evaluation, nil
}

// resolveIntelligenceScope decides the scope one request may act on.
//
// A branch-scoped actor can only ever name their own branch, and can never
// reach BUSINESS scope: a business-wide score summarises trade at branches
// they are not authorised to see, so handing them one would leak exactly what
// branch scoping exists to prevent.
func (s *Service) resolveIntelligenceScope(ctx context.Context, actor Actor, scope Scope,
	scopeType, branchRef string) (string, *string, error) {
	scopeType = strings.ToUpper(strings.TrimSpace(scopeType))
	branchRef = strings.TrimSpace(branchRef)
	if scopeType == "" {
		if scope.Global {
			scopeType = IntelligenceScopeBusiness
		} else {
			scopeType = IntelligenceScopeBranch
		}
	}
	if !ValidIntelligenceScope(scopeType) {
		return "", nil, apperrors.Validation("Choose a supported scope.", []apperrors.FieldViolation{
			{Field: "scope_type", Message: "Scope must be BUSINESS or BRANCH."}})
	}
	if scopeType == IntelligenceScopeBusiness {
		if branchRef != "" {
			return "", nil, apperrors.Validation("A business-scope request cannot name a branch.",
				[]apperrors.FieldViolation{{Field: "branch_ref", Message: "Remove the branch."}})
		}
		if !scope.Global {
			return "", nil, errIntelligenceScopeDenied()
		}
		return IntelligenceScopeBusiness, nil, nil
	}
	if branchRef == "" {
		if !scope.Global && len(scope.BranchIDs) == 1 {
			branchRef = scope.BranchIDs[0]
		} else {
			return "", nil, apperrors.Validation("Name the branch.", []apperrors.FieldViolation{
				{Field: "branch_ref", Message: "A branch-scope request must name a branch."}})
		}
	}
	if !scope.Global && !containsString(scope.BranchIDs, branchRef) {
		return "", nil, errIntelligenceScopeDenied()
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branches
		WHERE id=$1 AND business_id=$2)`, branchRef, actor.VendorID).Scan(&exists); err != nil {
		return "", nil, err
	}
	if !exists {
		return "", nil, errIntelligenceScopeDenied()
	}
	return IntelligenceScopeBranch, &branchRef, nil
}

// lockScopeState reads the pointer row for update, creating it on first use.
func (s *Service) lockScopeState(ctx context.Context, tx pgx.Tx, businessID, modelKey, scopeType string,
	branchID *string) (IntelligenceScopeState, error) {
	id := scopeStateKey(businessID, modelKey, scopeType, derefOr(branchID))
	if _, err := tx.Exec(ctx, `INSERT INTO crm_intelligence_scope_states
		(id,business_id,branch_id,scope,model_key,status) VALUES ($1,$2,$3,$4,$5,'NOT_AVAILABLE')
		ON CONFLICT (id) DO NOTHING`, id, businessID, branchID, scopeType, modelKey); err != nil {
		return IntelligenceScopeState{}, err
	}
	return scanScopeState(tx.QueryRow(ctx, `SELECT `+scopeStateColumns+`
		FROM crm_intelligence_scope_states WHERE id=$1 FOR UPDATE`, id))
}

func (s *Service) setScopeStatusTx(ctx context.Context, tx pgx.Tx, state IntelligenceScopeState,
	status string, errorCode *string) error {
	// A scope already CURRENT stays presentable while a rebuild is queued or
	// running: an operator must keep seeing the last good numbers, marked as
	// of when they were computed, rather than a blank dashboard.
	if state.Status == IntelligenceScopeCurrent &&
		(status == IntelligenceScopeQueued || status == IntelligenceScopeRunning) {
		status = IntelligenceScopeStale
	}
	_, err := tx.Exec(ctx, `UPDATE crm_intelligence_scope_states SET status=$2, error_code=$3,
		stale_since=CASE WHEN $2 IN ('STALE','FAILED') THEN COALESCE(stale_since, now()) ELSE NULL END,
		updated_at=now(), version=version+1
		WHERE business_id=$1 AND model_key=$4 AND scope=$5 AND COALESCE(branch_id,'')=$6`,
		state.BusinessID, status, errorCode, state.ModelKey, state.Scope, derefOr(state.BranchID))
	return err
}

// ---------------------------------------------------------------------------
// Worker
// ---------------------------------------------------------------------------

// RunIntelligenceMaterializer claims and runs up to limit evaluations.
//
// Claiming is a lease with SKIP LOCKED, so running several instances is safe:
// two workers never adopt the same evaluation, and a worker that dies mid-run
// releases its claim when the lease lapses.
func (s *Service) RunIntelligenceMaterializer(ctx context.Context, workerID string, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.IntelligenceEnabled || !s.flags.IntelligenceWorkerEnabled {
		return 0, nil
	}
	if s.flags.CommerceKillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	processed := 0
	for i := 0; i < limit; i++ {
		evaluation, claimed, err := s.claimIntelligenceEvaluation(ctx, workerID)
		if err != nil {
			return processed, err
		}
		if !claimed {
			return processed, nil
		}
		processed++
		if err = s.runIntelligenceEvaluation(ctx, evaluation); err != nil {
			if failErr := s.failIntelligenceEvaluation(ctx, evaluation, err); failErr != nil {
				return processed, failErr
			}
		}
	}
	return processed, nil
}

func (s *Service) claimIntelligenceEvaluation(ctx context.Context, workerID string) (IntelligenceEvaluation, bool, error) {
	lease := fmt.Sprintf("%d milliseconds", s.intelligence.LeaseTTL.Milliseconds())
	evaluation, err := scanIntelligenceEvaluation(s.db.QueryRow(ctx, `UPDATE crm_intelligence_evaluations SET
			status='RUNNING', lease_owner=$1, lease_expires_at=now() + $2::interval,
			started_at=COALESCE(started_at, now()), worker_attempt_count=worker_attempt_count+1,
			next_attempt_at=now() + $2::interval, updated_at=now(), version=version+1
		WHERE id = (
			SELECT id FROM crm_intelligence_evaluations
			WHERE status IN ('QUEUED','RUNNING','FAILED')
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at <= now())
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+evaluationColumns, workerID, lease))
	if errors.Is(err, pgx.ErrNoRows) {
		return IntelligenceEvaluation{}, false, nil
	}
	if err != nil {
		return IntelligenceEvaluation{}, false, err
	}
	return evaluation, true, nil
}

// failIntelligenceEvaluation records a failed attempt, backing off and finally
// dead-lettering. The scope keeps its previous generation throughout: a failed
// rebuild degrades to "these numbers are stale", never to "there are none".
func (s *Service) failIntelligenceEvaluation(ctx context.Context, evaluation IntelligenceEvaluation, cause error) error {
	code := "MODEL_ERROR"
	var appErr *apperrors.Error
	if errors.As(cause, &appErr) {
		code = strings.ToUpper(string(appErr.Code))
		if len(code) > 60 {
			code = code[:60]
		}
	}
	status := IntelligenceEvalFailed
	if evaluation.Attempts >= s.intelligence.MaxAttempts {
		status = IntelligenceEvalDeadLetter
	}
	backoff := fmt.Sprintf("%d milliseconds", s.intelligence.Backoff.Milliseconds()*int64(evaluation.Attempts))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE crm_intelligence_evaluations SET status=$2, error_code=$3,
		lease_owner=NULL, lease_expires_at=NULL, next_attempt_at=now() + $4::interval,
		updated_at=now(), version=version+1 WHERE id=$1`, evaluation.ID, status, code, backoff); err != nil {
		return err
	}
	state, err := s.lockScopeState(ctx, tx, evaluation.BusinessID, evaluation.ModelKey,
		evaluation.Scope, evaluation.BranchID)
	if err != nil {
		return err
	}
	scopeStatus := IntelligenceScopeStale
	if state.Generation == nil {
		// Nothing good was ever built here, so there is no last-good state to
		// fall back to and FAILED is the truthful answer.
		scopeStatus = IntelligenceScopeFailed
	}
	if err = s.setScopeStatusTx(ctx, tx, IntelligenceScopeState{
		BusinessID: state.BusinessID, BranchID: state.BranchID, Scope: state.Scope,
		ModelKey: state.ModelKey, Status: scopeStatus,
	}, scopeStatus, &code); err != nil {
		return err
	}
	if err = s.audit.Record(ctx, AuditEntry{
		ActorID: "intelligence-materializer", Action: "crm.intelligence.evaluation.failed",
		AggregateType: "crm_intelligence_evaluation", AggregateID: evaluation.ID,
		VendorID: evaluation.BusinessID, BranchID: derefOr(evaluation.BranchID),
		After: map[string]any{"model_key": evaluation.ModelKey, "error_code": code, "status": status},
	}); err != nil {
		return err
	}
	if err = s.publishIntelligenceEvent(ctx, tx, evaluation, "vendor.crm.intelligence.failed",
		map[string]any{"error_code": code, "status": status}); err != nil {
		return err
	}
	s.metrics.Inc("crm_intelligence_evaluation_failure_total", "model", evaluation.ModelKey)
	return tx.Commit(ctx)
}

// ReplayIntelligenceEvaluation returns a dead-lettered evaluation to the queue.
// It is an operator hook rather than a Vendor-facing route: replaying failed
// infrastructure work is not ordinary CRM authority.
func (s *Service) ReplayIntelligenceEvaluation(ctx context.Context, businessID, evaluationID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE crm_intelligence_evaluations SET status='QUEUED',
		worker_attempt_count=0, next_attempt_at=now(), lease_owner=NULL, lease_expires_at=NULL,
		error_code=NULL, updated_at=now(), version=version+1
		WHERE business_id=$1 AND id=$2 AND status='DEAD_LETTER'`, businessID, evaluationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("intelligence evaluation %s is not dead-lettered", evaluationID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Generation build
// ---------------------------------------------------------------------------

// cohortRow is one candidate relationship with everything both models need.
//
// The cume_dist columns are NULL for a relationship outside the scored cohort,
// which is how ineligibility travels: there is no sentinel number that could
// be mistaken for a real position.
type cohortRow struct {
	ProfileID       string
	ProjectionState string
	OrderCount      int
	NetSpendMinor   int64
	FirstOrderAt    *time.Time
	LastOrderAt     *time.Time
	RecencyCD       *float64
	FrequencyCD     *float64
	MonetaryCD      *float64
	CohortSize      int
}

// cohortQuery builds the single streaming query that drives a generation.
//
// One query, not two, and no in-memory set: every relationship in the scope
// appears exactly once, already carrying its position in the scored cohort or
// NULL if it is not in it. A BRANCH evaluation reads the branch's own relation
// row and never the business aggregate, which is the whole reason branch
// intelligence can be shown to a branch actor at all.
func cohortQuery(scopeType string) string {
	if scopeType == IntelligenceScopeBranch {
		return `WITH eligible AS (
			SELECT r.profile_id AS id, r.completed_order_count AS orders, r.net_spend_minor AS spend,
			       r.first_relationship_at AS first_at, r.last_relationship_at AS last_at,
			       FLOOR(EXTRACT(EPOCH FROM ($3::timestamptz - r.last_relationship_at))/86400)::bigint AS days
			FROM crm_customer_branch_relations r
			JOIN vendor_customer_profiles p ON p.id=r.profile_id AND p.business_id=r.business_id
			WHERE r.business_id=$1 AND r.branch_id=$2
			  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE' AND p.status <> 'ARCHIVED'
			  AND r.projection_state <> 'NOT_AVAILABLE'
			  AND r.completed_order_count >= 1 AND r.last_relationship_at IS NOT NULL
		), ranked AS (
			SELECT id, orders, spend, first_at, last_at, days,
			       cume_dist() OVER (ORDER BY days DESC) AS rcd,
			       cume_dist() OVER (ORDER BY orders ASC) AS fcd,
			       cume_dist() OVER (ORDER BY spend ASC) AS mcd,
			       count(*) OVER () AS cohort_size
			FROM eligible
		)
		SELECT p.id, rel.projection_state, rel.completed_order_count, rel.net_spend_minor,
		       rel.first_relationship_at, rel.last_relationship_at,
		       k.rcd, k.fcd, k.mcd, COALESCE(k.cohort_size,0)
		FROM crm_customer_branch_relations rel
		JOIN vendor_customer_profiles p ON p.id=rel.profile_id AND p.business_id=rel.business_id
		LEFT JOIN ranked k ON k.id = p.id
		WHERE rel.business_id=$1 AND rel.branch_id=$2
		  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE' AND p.status <> 'ARCHIVED'
		ORDER BY p.id`
	}
	return `WITH eligible AS (
		SELECT p.id, p.completed_order_count AS orders, p.net_spend_minor AS spend,
		       p.first_completed_order_at AS first_at, p.last_completed_order_at AS last_at,
		       FLOOR(EXTRACT(EPOCH FROM ($3::timestamptz - p.last_completed_order_at))/86400)::bigint AS days
		FROM vendor_customer_profiles p
		WHERE p.business_id=$1 AND ($2='' OR $2='')
		  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE' AND p.status <> 'ARCHIVED'
		  AND p.commercial_projection_state <> 'NOT_AVAILABLE'
		  AND p.completed_order_count >= 1 AND p.last_completed_order_at IS NOT NULL
	), ranked AS (
		SELECT id, orders, spend, first_at, last_at, days,
		       cume_dist() OVER (ORDER BY days DESC) AS rcd,
		       cume_dist() OVER (ORDER BY orders ASC) AS fcd,
		       cume_dist() OVER (ORDER BY spend ASC) AS mcd,
		       count(*) OVER () AS cohort_size
		FROM eligible
	)
	SELECT p.id, p.commercial_projection_state, p.completed_order_count, p.net_spend_minor,
	       p.first_completed_order_at, p.last_completed_order_at,
	       k.rcd, k.fcd, k.mcd, COALESCE(k.cohort_size,0)
	FROM vendor_customer_profiles p
	LEFT JOIN ranked k ON k.id = p.id
	WHERE p.business_id=$1 AND ($2='' OR $2='')
	  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE' AND p.status <> 'ARCHIVED'
	ORDER BY p.id`
}

// scoreDraft is one row about to be written into the building generation.
type scoreDraft struct {
	ProfileID string
	State     string
	RFM       *RFMScore
	Churn     *ChurnScore
	Reasons   []string
}

// runIntelligenceEvaluation builds one generation and swaps it in.
func (s *Service) runIntelligenceEvaluation(ctx context.Context, evaluation IntelligenceEvaluation) error {
	model, ok := ModelFor(evaluation.ModelKey)
	if !ok {
		return errModelNotFound()
	}
	// A model whose definition changed since the job was queued must not write
	// scores stamped with the old identity. Superseding the run and letting the
	// next one carry the new hash is the honest outcome.
	if model.DefinitionHash() != evaluation.Hash || model.Version != evaluation.ModelVersion {
		return s.supersedeIntelligenceEvaluation(ctx, evaluation)
	}
	started := time.Now()

	// A previous attempt may have written part of this generation before dying.
	// Clearing it is safe precisely because a building generation is never the
	// live one.
	if _, err := s.db.Exec(ctx, `DELETE FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4 AND generation=$5`,
		evaluation.BusinessID, evaluation.ModelKey, evaluation.Scope,
		derefOr(evaluation.BranchID), evaluation.Generation); err != nil {
		return err
	}

	drafts, counts, err := s.buildGenerationDrafts(ctx, evaluation, model)
	if err != nil {
		return err
	}
	if err = s.writeScoreGeneration(ctx, evaluation, model, drafts); err != nil {
		return err
	}
	if err = s.swapIntelligenceGeneration(ctx, evaluation, model, counts); err != nil {
		return err
	}
	s.metrics.Inc("crm_intelligence_evaluation_total", "model", evaluation.ModelKey, "result", "success")
	s.metrics.Add("crm_intelligence_evaluation_duration_seconds", time.Since(started).Seconds())
	return nil
}

type generationCounts struct{ Profiles, Available, Unavailable int }

// buildGenerationDrafts streams the cohort and applies the pure model
// functions. Nothing here reads a request, a template or a stored formula.
func (s *Service) buildGenerationDrafts(ctx context.Context, evaluation IntelligenceEvaluation,
	model IntelligenceModel) ([]scoreDraft, generationCounts, error) {
	rows, err := s.db.Query(ctx, cohortQuery(evaluation.Scope),
		evaluation.BusinessID, derefOr(evaluation.BranchID), evaluation.SourceAsOf)
	if err != nil {
		return nil, generationCounts{}, err
	}
	defer rows.Close()

	drafts := make([]scoreDraft, 0, 256)
	counts := generationCounts{}
	for rows.Next() {
		var row cohortRow
		if err = rows.Scan(&row.ProfileID, &row.ProjectionState, &row.OrderCount, &row.NetSpendMinor,
			&row.FirstOrderAt, &row.LastOrderAt, &row.RecencyCD, &row.FrequencyCD, &row.MonetaryCD,
			&row.CohortSize); err != nil {
			return nil, generationCounts{}, err
		}
		counts.Profiles++
		if counts.Profiles > s.intelligence.MaxCohort {
			return nil, generationCounts{}, fmt.Errorf("intelligence cohort exceeds %d relationships",
				s.intelligence.MaxCohort)
		}
		draft := s.draftFor(model, row, evaluation.SourceAsOf)
		if draft.State == ScoreCurrent {
			counts.Available++
		} else {
			counts.Unavailable++
		}
		drafts = append(drafts, draft)
	}
	if err = rows.Err(); err != nil {
		return nil, generationCounts{}, err
	}
	return drafts, counts, nil
}

// draftFor applies one model to one cohort row.
//
// The unavailable branches are the important ones. A relationship with no
// commercial projection is SOURCE_NOT_AVAILABLE, not a zero; one with too
// little observed history is INSUFFICIENT_HISTORY, not dormant; and a cohort
// too small to rank against is INSUFFICIENT_COHORT, not a bottom quintile.
func (s *Service) draftFor(model IntelligenceModel, row cohortRow, asOf time.Time) scoreDraft {
	draft := scoreDraft{ProfileID: row.ProfileID, State: ScoreNotAvailable}

	if row.ProjectionState == string(ProjectionNotAvailable) {
		draft.Reasons = []string{ReasonSourceNotAvailable}
		return draft
	}

	switch model.Key {
	case ModelRFM:
		if row.RecencyCD == nil || row.FrequencyCD == nil || row.MonetaryCD == nil {
			draft.Reasons = []string{ReasonInsufficientHistory}
			return draft
		}
		if row.CohortSize < RFMMinimumCohort {
			draft.Reasons = []string{ReasonInsufficientCohort}
			return draft
		}
		score := computeRFM(rfmDimensions{
			ProfileID: row.ProfileID, RecencyCD: *row.RecencyCD,
			FrequencyCD: *row.FrequencyCD, MonetaryCD: *row.MonetaryCD,
			CompletedOrderCount: row.OrderCount, NetSpendMinor: row.NetSpendMinor,
		})
		draft.State = ScoreCurrent
		draft.RFM = &score
		draft.Reasons = score.ReasonCodes
		return draft

	case ModelChurn:
		if row.FirstOrderAt == nil || row.LastOrderAt == nil || row.OrderCount < ChurnMinimumOrders {
			draft.Reasons = []string{ReasonInsufficientHistory}
			if row.OrderCount == 1 {
				draft.Reasons = append(draft.Reasons, ReasonSingleObservedPurchase)
			}
			return draft
		}
		score, ok := computeChurn(ChurnInput{
			ProfileID: row.ProfileID, CompletedOrderCount: row.OrderCount,
			FirstOrderAt: *row.FirstOrderAt, LastOrderAt: *row.LastOrderAt, AsOf: asOf,
		})
		if !ok {
			draft.Reasons = []string{ReasonInsufficientHistory}
			return draft
		}
		draft.State = ScoreCurrent
		draft.Churn = &score
		draft.Reasons = score.ReasonCodes
		return draft
	}

	draft.Reasons = []string{ReasonModelError}
	return draft
}

// writeScoreGeneration inserts the building generation in bounded batches. The
// rows are written with is_current false: they become visible only at the swap.
func (s *Service) writeScoreGeneration(ctx context.Context, evaluation IntelligenceEvaluation,
	model IntelligenceModel, drafts []scoreDraft) error {
	batch := s.intelligence.BatchSize
	now := s.now()
	for start := 0; start < len(drafts); start += batch {
		end := start + batch
		if end > len(drafts) {
			end = len(drafts)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*16)
		for _, draft := range drafts[start:end] {
			base := len(args)
			var recency, frequency, monetary, composite, risk any
			var band any
			if draft.RFM != nil {
				recency, frequency, monetary, composite = draft.RFM.Recency, draft.RFM.Frequency,
					draft.RFM.Monetary, draft.RFM.Composite
			}
			if draft.Churn != nil {
				risk, band = draft.Churn.Risk, draft.Churn.Band
			}
			args = append(args, newID("crmscore_"), evaluation.BusinessID, evaluation.BranchID,
				evaluation.Scope, evaluation.ID, evaluation.Generation, draft.ProfileID,
				model.Key, model.Version, model.DefinitionHash(), draft.State,
				recency, frequency, monetary, composite, risk, band, draft.Reasons,
				evaluation.SourceAsOf, now)
			placeholders := make([]string, 20)
			for i := range placeholders {
				placeholders[i] = fmt.Sprintf("$%d", base+i+1)
			}
			values = append(values, "("+strings.Join(placeholders, ",")+")")
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO crm_customer_intelligence_scores
			(id,business_id,branch_id,scope,evaluation_id,generation,customer_profile_id,
			 model_key,model_version,definition_hash,state,
			 recency_score,frequency_score,monetary_score,composite_score,risk_score,risk_band,
			 reason_codes,source_as_of,computed_at) VALUES `+strings.Join(values, ","), args...); err != nil {
			return err
		}
	}
	return nil
}

// swapIntelligenceGeneration makes the new generation live in one transaction.
//
// The order is the whole design. Before anything becomes current, the run
// re-certifies against privacy: a relationship that was redacted or merged
// while the generation was being built has its brand-new row deleted inside
// this same transaction, so the swap cannot publish a score for someone who
// stopped being scoreable halfway through. Counts are then recomputed from the
// survivors rather than from what the build believed, so the availability
// figures describe what is actually current.
func (s *Service) swapIntelligenceGeneration(ctx context.Context, evaluation IntelligenceEvaluation,
	model IntelligenceModel, counts generationCounts) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	state, err := s.lockScopeState(ctx, tx, evaluation.BusinessID, evaluation.ModelKey,
		evaluation.Scope, evaluation.BranchID)
	if err != nil {
		return err
	}

	// Privacy re-certification. The profile rows are locked in a stable order
	// so this serialises against the privacy and merge handlers, which take the
	// same locks — either commit ordering ends up safe.
	if _, err = tx.Exec(ctx, `WITH fenced AS (
			SELECT p.id FROM vendor_customer_profiles p
			WHERE p.business_id=$1 AND (p.privacy_state='REDACTED' OR p.merge_state='MERGED_SOURCE'
				OR p.status='ARCHIVED')
			ORDER BY p.id FOR UPDATE)
		DELETE FROM crm_customer_intelligence_scores sc
		WHERE sc.business_id=$1 AND sc.model_key=$2 AND sc.scope=$3
		  AND COALESCE(sc.branch_id,'')=$4 AND sc.generation=$5
		  AND sc.customer_profile_id IN (SELECT id FROM fenced)`,
		evaluation.BusinessID, evaluation.ModelKey, evaluation.Scope,
		derefOr(evaluation.BranchID), evaluation.Generation); err != nil {
		return err
	}

	if err = tx.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER (WHERE state='CURRENT'),
			count(*) FILTER (WHERE state<>'CURRENT')
		FROM crm_customer_intelligence_scores
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4 AND generation=$5`,
		evaluation.BusinessID, evaluation.ModelKey, evaluation.Scope,
		derefOr(evaluation.BranchID), evaluation.Generation).
		Scan(&counts.Profiles, &counts.Available, &counts.Unavailable); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `UPDATE crm_customer_intelligence_scores SET is_current=false
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4
		  AND is_current AND generation <> $5`,
		evaluation.BusinessID, evaluation.ModelKey, evaluation.Scope,
		derefOr(evaluation.BranchID), evaluation.Generation); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE crm_customer_intelligence_scores SET is_current=true
		WHERE business_id=$1 AND model_key=$2 AND scope=$3 AND COALESCE(branch_id,'')=$4
		  AND generation=$5`,
		evaluation.BusinessID, evaluation.ModelKey, evaluation.Scope,
		derefOr(evaluation.BranchID), evaluation.Generation); err != nil {
		return err
	}

	now := s.now()
	if _, err = tx.Exec(ctx, `UPDATE crm_intelligence_evaluations SET status='COMPLETED',
		profile_count=$2, available_count=$3, unavailable_count=$4, completed_at=now(),
		lease_owner=NULL, lease_expires_at=NULL, error_code=NULL, updated_at=now(), version=version+1
		WHERE id=$1`, evaluation.ID, counts.Profiles, counts.Available, counts.Unavailable); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE crm_intelligence_scope_states SET status='CURRENT',
		current_generation=$2, current_evaluation_id=$3, model_version=$4, definition_hash=$5,
		source_as_of=$6, computed_at=$7, stale_since=NULL, error_code=NULL,
		profile_count=$8, available_count=$9, unavailable_count=$10, updated_at=now(), version=version+1
		WHERE business_id=$1 AND model_key=$11 AND scope=$12 AND COALESCE(branch_id,'')=$13`,
		evaluation.BusinessID, evaluation.Generation, evaluation.ID, model.Version, model.DefinitionHash(),
		evaluation.SourceAsOf, now, counts.Profiles, counts.Available, counts.Unavailable,
		evaluation.ModelKey, evaluation.Scope, derefOr(evaluation.BranchID)); err != nil {
		return err
	}
	_ = state

	// A score set that changed restakes exactly the segments that read this
	// model, and nothing else.
	if dependency := intelligenceDependency(evaluation.ModelKey); dependency != "" {
		if err = s.invalidateSegmentsForIntelligence(ctx, tx, evaluation.BusinessID, dependency); err != nil {
			return err
		}
	}
	if err = s.audit.Record(ctx, AuditEntry{
		ActorID: "intelligence-materializer", Action: "crm.intelligence.evaluation.completed",
		AggregateType: "crm_intelligence_evaluation", AggregateID: evaluation.ID,
		VendorID: evaluation.BusinessID, BranchID: derefOr(evaluation.BranchID),
		After: map[string]any{"model_key": evaluation.ModelKey, "model_version": model.Version,
			"generation": evaluation.Generation, "profile_count": counts.Profiles,
			"available_count": counts.Available, "unavailable_count": counts.Unavailable},
	}); err != nil {
		return err
	}
	if err = s.publishIntelligenceEvent(ctx, tx, evaluation, "vendor.crm.intelligence.materialized",
		map[string]any{"profile_count": counts.Profiles, "available_count": counts.Available,
			"unavailable_count": counts.Unavailable}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) supersedeIntelligenceEvaluation(ctx context.Context, evaluation IntelligenceEvaluation) error {
	_, err := s.db.Exec(ctx, `UPDATE crm_intelligence_evaluations SET status='SUPERSEDED',
		lease_owner=NULL, lease_expires_at=NULL, completed_at=now(), updated_at=now(), version=version+1
		WHERE id=$1`, evaluation.ID)
	return err
}

// publishIntelligenceEvent emits the aggregate outbox event.
//
// The payload is deliberately scope-level: a customer list on an event bus
// would be a far larger disclosure than the dashboard the event exists to
// refresh.
func (s *Service) publishIntelligenceEvent(ctx context.Context, tx pgx.Tx,
	evaluation IntelligenceEvaluation, eventType string, extra map[string]any) error {
	if s.events == nil {
		return nil
	}
	payload := map[string]any{
		"business_ref":  evaluation.BusinessID,
		"scope":         evaluation.Scope,
		"model_key":     evaluation.ModelKey,
		"model_version": evaluation.ModelVersion,
		"generation":    evaluation.Generation,
		"source_as_of":  evaluation.SourceAsOf,
	}
	if evaluation.BranchID != nil {
		payload["branch_ref"] = *evaluation.BranchID
	}
	for key, value := range extra {
		payload[key] = value
	}
	return s.events.Publish(ctx, tx, eventType, "crm_intelligence_evaluation", evaluation.ID,
		evaluation.Version, evaluation.BusinessID, "", "", payload)
}

// invalidateSegmentsForIntelligence restakes the dynamic segments that read a
// model's fields. It is dependency-scoped: a churn rebuild leaves an RFM-only
// segment alone.
func (s *Service) invalidateSegmentsForIntelligence(ctx context.Context, tx pgx.Tx,
	businessID, dependency string) error {
	keys := intelligenceFieldKeys(dependency)
	if len(keys) == 0 {
		return nil
	}
	return s.invalidateSegmentsForDependencies(ctx, tx, businessID, keys)
}

// ---------------------------------------------------------------------------
// Staleness
// ---------------------------------------------------------------------------

// MarkIntelligenceStaleTx marks every scope of a business stale for the models
// whose inputs just changed.
//
// It is called from inside the transaction that changed the input, so a crash
// cannot leave a scope labelled CURRENT after the facts under it moved. Scores
// stay readable and keep their as-of instant; STALE is a statement about
// freshness, never a reason to blank a dashboard.
func (s *Service) MarkIntelligenceStaleTx(ctx context.Context, tx pgx.Tx, businessID string, models ...string) error {
	if !s.flags.IntelligenceEnabled {
		return nil
	}
	if len(models) == 0 {
		models = AllModelKeys
	}
	_, err := tx.Exec(ctx, `UPDATE crm_intelligence_scope_states SET
			status=CASE WHEN status='CURRENT' THEN 'STALE' ELSE status END,
			stale_since=CASE WHEN status='CURRENT' THEN now() ELSE stale_since END,
			updated_at=now(), version=version+1
		WHERE business_id=$1 AND model_key = ANY($2::text[]) AND status='CURRENT'`, businessID, models)
	if err == nil {
		s.metrics.Inc("crm_intelligence_stale_total")
	}
	return err
}

// MarkIntelligenceStaleForBranchTx narrows staleness to one branch's scopes
// plus the business scope, for a change that only touched one branch relation.
func (s *Service) MarkIntelligenceStaleForBranchTx(ctx context.Context, tx pgx.Tx,
	businessID, branchID string, models ...string) error {
	if !s.flags.IntelligenceEnabled {
		return nil
	}
	if len(models) == 0 {
		models = AllModelKeys
	}
	_, err := tx.Exec(ctx, `UPDATE crm_intelligence_scope_states SET
			status=CASE WHEN status='CURRENT' THEN 'STALE' ELSE status END,
			stale_since=CASE WHEN status='CURRENT' THEN now() ELSE stale_since END,
			updated_at=now(), version=version+1
		WHERE business_id=$1 AND model_key = ANY($2::text[]) AND status='CURRENT'
		  AND (scope='BUSINESS' OR branch_id=$3)`, businessID, models, branchID)
	return err
}

// SuppressIntelligenceForProfileTx makes one relationship's scores
// non-actionable in the same transaction as the privacy or merge action that
// caused it.
//
// The rows are not deleted. They are demoted out of the current pointer and
// restated as suppressed, so no segment, export, analytic or Customer 360 read
// can reach them, while the fact that a score once existed remains auditable.
// Deleting would be indistinguishable from never having scored the person.
func (s *Service) SuppressIntelligenceForProfileTx(ctx context.Context, tx pgx.Tx,
	businessID, profileID, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE crm_customer_intelligence_scores SET
			is_current=false, state='NOT_AVAILABLE',
			recency_score=NULL, frequency_score=NULL, monetary_score=NULL, composite_score=NULL,
			risk_score=NULL, risk_band=NULL, reason_codes=ARRAY[$3]::text[]
		WHERE business_id=$1 AND customer_profile_id=$2 AND is_current`, businessID, profileID, reason)
	return err
}
