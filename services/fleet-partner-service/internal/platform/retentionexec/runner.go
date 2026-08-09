package retentionexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// DefaultBatchSize bounds how many candidates one RunOnce pass considers --
// "bounded batches" is a hard Section 28 requirement, not a performance
// nicety: an unbounded pass over a large table inside one durable-job
// attempt would risk lease expiry (the job framework would then recover and
// retry from a partially-applied state) and makes "how much did this run
// actually touch" unpredictable. A DRY_RUN or EXECUTE run that has more
// candidates than one batch is left PENDING/RUNNING-incomplete for a
// subsequent scheduled pass to continue -- see RunOnce's returned
// exhausted flag.
const DefaultBatchSize = 200

// Runner is the generic retention execution engine: legal-hold and
// minimum-retention checks are applied identically for every class, so no
// individual ClassExecutor can forget or misapply them.
type Runner struct {
	pool          *pgxpool.Pool
	store         *Store
	executors     map[string]ClassExecutor
	legalHold     LegalHoldChecker
	retentionDays EffectiveRetentionDaysReader
	recorder      *audit.Recorder
	batchSize     int
}

// NewRunner builds the runner. executors is the CLOSED allowlist of
// implemented classes -- a class not present here can never be executed
// (Initiate refuses to even create a run for it; RunOnce refuses to start
// one that somehow exists anyway).
func NewRunner(pool *pgxpool.Pool, executors []ClassExecutor, legalHold LegalHoldChecker,
	retentionDays EffectiveRetentionDaysReader, recorder *audit.Recorder, batchSize int) *Runner {
	if recorder == nil {
		recorder = audit.NewRecorder()
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	registry := make(map[string]ClassExecutor, len(executors))
	for _, e := range executors {
		registry[e.Class()] = e
	}
	return &Runner{
		pool: pool, store: NewStore(pool), executors: registry,
		legalHold: legalHold, retentionDays: retentionDays, recorder: recorder, batchSize: batchSize,
	}
}

// SupportsClass reports whether a ClassExecutor is registered for class --
// Administrator.Initiate checks this before ever creating a run row, so an
// unimplemented class is refused up front rather than creating a run that
// RunOnce would only fail later.
func (r *Runner) SupportsClass(class string) bool {
	_, ok := r.executors[class]
	return ok
}

// ErrClassNotExecutable means no ClassExecutor is registered for a run's
// retention class -- the class exists in retentionpolicy's configuration
// vocabulary (an organization can set a day count for it) but has no
// implemented execution behavior, most classes in this schema today (see
// the package doc comment for exactly which ones this pass implements).
var ErrClassNotExecutable = errors.New("retentionexec: no executor registered for this retention class")

// RunOnce processes up to one batch of candidates for runID and reports
// whether the batch was exhausted (fewer candidates than batchSize were
// found, meaning this run is now complete) or not (a future call should
// continue it). Idempotent: safe to call more than once for the same runID,
// including after a partial batch already recorded some outcomes -- see
// Store.RecordOutcome/HasOutcome.
//
// A run already in a terminal status (COMPLETED or FAILED) is a true no-op:
// RunOnce returns immediately without touching the run row again. This is
// required, not defensive: a replay against an already-COMPLETED run would
// otherwise find zero remaining candidates (every one it already decided is
// now excluded by the executor's own FindCandidates -- an anonymized row no
// longer matches "not yet anonymized") and, seeing an empty batch, call
// CompleteRun again with a FRESH zero-valued RunCounters -- silently
// overwriting the run's real, already-correct final counts with zeros. This
// exact bug was caught by
// TestRetentionExecution_IdempotentReplaySkipsAlreadyAnonymized failing
// (recordsAnonymized went from 1 to 0 after a second RunOnce call) before
// this guard existed.
func (r *Runner) RunOnce(ctx context.Context, runID string) (exhausted bool, err error) {
	run, err := r.store.GetRunByID(ctx, runID)
	if err != nil {
		return false, err
	}
	if run.Status == RunStatusCompleted || run.Status == RunStatusFailed {
		return true, nil
	}
	executor, ok := r.executors[run.RetentionClass]
	if !ok {
		_ = r.store.CompleteRun(ctx, r.pool, runID, RunStatusFailed, ErrClassNotExecutable.Error())
		return true, ErrClassNotExecutable
	}
	if err := r.store.MarkRunning(ctx, r.pool, runID); err != nil {
		return false, err
	}

	candidates, err := executor.FindCandidates(ctx, r.pool, run.OrganizationID, r.batchSize)
	if err != nil {
		_ = r.store.CompleteRun(ctx, r.pool, runID, RunStatusFailed, err.Error())
		return true, fmt.Errorf("retentionexec: find candidates: %w", err)
	}

	effectiveDays, err := r.retentionDays.EffectiveRetentionDays(ctx, run.OrganizationID, run.RetentionClass)
	if err != nil {
		_ = r.store.CompleteRun(ctx, r.pool, runID, RunStatusFailed, err.Error())
		return true, fmt.Errorf("retentionexec: resolve effective retention days: %w", err)
	}

	var counters RunCounters
	newlyDecided := 0
	for _, candidate := range candidates {
		outcome, _, procErr := r.decide(ctx, run, executor, candidate, effectiveDays)
		if outcome == "" {
			if procErr != nil {
				// decide() failed before it could even record an outcome
				// (e.g. the outcome-insert itself errored) -- surface as a
				// run-level failure rather than silently under-counting.
				// Whatever this pass DID manage to decide before the hard
				// error is still real, persisted work -- added before the
				// run is marked FAILED, not discarded with it.
				_ = r.store.AddCounters(ctx, r.pool, runID, counters)
				_ = r.store.CompleteRun(ctx, r.pool, runID, RunStatusFailed, procErr.Error())
				return true, procErr
			}
			// Already decided in a prior attempt at this run (idempotent
			// replay) -- not new work, not counted again.
			continue
		}
		newlyDecided++
		counters.Considered++
		switch outcome {
		case OutcomeArchived:
			counters.Archived++
		case OutcomeAnonymized:
			counters.Anonymized++
		case OutcomeFailed:
			counters.Failed++
		default:
			counters.Skipped++
		}
	}

	// This pass's tally is added to the run's running totals unconditionally
	// -- whether or not this pass turns out to be the exhausting one. See
	// Store.AddCounters' own doc comment for why a run spanning more than
	// one pass previously lost every non-final pass's counts entirely.
	if err := r.store.AddCounters(ctx, r.pool, runID, counters); err != nil {
		return false, err
	}

	// exhausted is true either because the batch came back short (the
	// ordinary case), OR because every candidate in a FULL batch was
	// already decided in this run and zero new progress was made.
	//
	// The second clause exists because a ClassExecutor's own
	// "not yet processed" filter (OrgContactExecutor's `email <> $2`,
	// this package's other executors' equivalents) is a DATABASE-STATE
	// filter, satisfied only once a real EXECUTE-mode mutation has
	// happened. DRY_RUN never mutates, by design -- so in DRY_RUN mode,
	// whenever the true candidate count is >= batchSize, FindCandidates
	// would otherwise keep handing back the exact same full batch on
	// every pass forever (each candidate individually short-circuited by
	// Store.HasOutcome as "already decided this run", but len(candidates)
	// never drops below batchSize to signal exhaustion) -- and
	// Administrator.Initiate's DRY_RUN path loops on RunOnce
	// synchronously inside the originating request until exhausted is
	// true, so this was a genuine infinite-loop/request-hang risk any
	// time DRY_RUN candidates >= batchSize, not merely a slow path.
	// Caught by TestSupportMetadataExecutor_BatchLimitEnforced
	// deliberately exercising batchSize=1 against 2 eligible candidates.
	exhausted = len(candidates) < r.batchSize || newlyDecided == 0
	if exhausted {
		if err := r.store.CompleteRun(ctx, r.pool, runID, RunStatusCompleted, ""); err != nil {
			return true, err
		}
	}
	return exhausted, nil
}

// decide applies the generic legal-hold and minimum-retention checks to one
// candidate, then dispatches to the executor's Preview (DRY_RUN) or
// Execute (EXECUTE, inside its own transaction) and persists the
// resulting immutable outcome. Returns outcome="" when the candidate was
// already decided in a prior attempt at this run (idempotent replay,
// nothing new happened).
func (r *Runner) decide(ctx context.Context, run Run, executor ClassExecutor, candidate Candidate, effectiveDays int) (outcome string, detail string, err error) {
	already, err := r.store.HasOutcome(ctx, r.pool, run.ID, candidate.RecordType, candidate.RecordID)
	if err != nil {
		return "", "", err
	}
	if already {
		return "", "", nil
	}

	persist := func(outcome, detail string) (string, string, error) {
		recErr := r.store.RecordOutcome(ctx, r.pool, RunRecord{
			RunID: run.ID, OrganizationID: run.OrganizationID,
			RecordType: candidate.RecordType, RecordID: candidate.RecordID,
			Outcome: outcome, Detail: detail,
		})
		if recErr != nil {
			return "", "", recErr
		}
		return outcome, detail, nil
	}

	scopeType, scopeReference := executor.ScopeForLegalHold(candidate)
	if scopeReference == "" {
		// Convention: an empty scopeReference means "the run's own
		// organization" -- see OrgContactExecutor.ScopeForLegalHold's doc
		// comment for why an executor would ever return this.
		scopeReference = run.OrganizationID
	}
	underHold, err := r.legalHold.IsUnderHold(ctx, run.OrganizationID, scopeType, scopeReference)
	if err != nil {
		return persist(OutcomeFailed, "legal hold check failed: "+err.Error())
	}
	if underHold {
		return persist(OutcomeSkippedLegalHold, "an active legal hold covers this scope")
	}

	eligibleSince, err := executor.EligibleSince(ctx, r.pool, candidate)
	if err != nil {
		return persist(OutcomeFailed, "could not resolve eligibility window: "+err.Error())
	}
	daysSinceEligible := int(time.Since(eligibleSince).Hours() / 24)
	if daysSinceEligible < effectiveDays {
		return persist(OutcomeSkippedMinimumRetention,
			fmt.Sprintf("eligible %d day(s) ago, organization's effective retention is %d day(s)", daysSinceEligible, effectiveDays))
	}

	if run.Mode == ModeDryRun {
		previewOutcome, previewDetail, previewErr := executor.Preview(ctx, r.pool, candidate)
		if previewErr != nil {
			return persist(OutcomeFailed, previewErr.Error())
		}
		return persist(previewOutcome, previewDetail)
	}

	var execOutcome, execDetail string
	txErr := database.WithinTx(ctx, r.pool, func(tx pgx.Tx) error {
		o, d, execErr := executor.Execute(ctx, tx, candidate)
		if execErr != nil {
			return execErr
		}
		execOutcome, execDetail = o, d
		return r.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: run.OrganizationID, ActorType: "system", ActorID: "retention_run:" + run.ID,
			Action: "retention.record." + toLowerOutcome(o), ResourceType: candidate.RecordType, ResourceID: candidate.RecordID,
			Reason: execDetail,
		})
	})
	if txErr != nil {
		return persist(OutcomeFailed, txErr.Error())
	}
	return persist(execOutcome, execDetail)
}

// jobPayload is the durable job's body -- just the run ID; every other
// input (organization, class, mode) is already durably recorded on the run
// row itself, so the job payload never duplicates it and can never drift
// from it.
type jobPayload struct {
	RunID string `json:"runId"`
}

// HandleJob adapts RunOnce to jobs.Handler's signature
// (func(ctx, jobs.Job) error), looping RunOnce until the run is exhausted.
// Looping internally (rather than one RunOnce per job execution,
// re-enqueuing for more) is a deliberate choice for the realistic data
// volumes this pass's one executor (organization contact fields) touches --
// each individual RunOnce call is still bounded by DefaultBatchSize per
// database round-trip, which is what "bounded batches" actually protects
// against (unbounded memory/lock scope in a single pass), not the number of
// passes a job may internally take.
func (r *Runner) HandleJob(ctx context.Context, job jobs.Job) error {
	var payload jobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("retentionexec: invalid job payload: %w", err)
	}
	if payload.RunID == "" {
		return fmt.Errorf("retentionexec: job payload missing runId")
	}
	for {
		exhausted, err := r.RunOnce(ctx, payload.RunID)
		if err != nil {
			return err
		}
		if exhausted {
			return nil
		}
	}
}

func toLowerOutcome(outcome string) string {
	switch outcome {
	case OutcomeArchived:
		return "archived"
	case OutcomeAnonymized:
		return "anonymized"
	default:
		return "processed"
	}
}
