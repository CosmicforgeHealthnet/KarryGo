package retentionexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// ActionClassExecute is passed to stepup.Verifier for initiating a
// retention run -- matches the privacy.retention.execute permission that
// already gates the route (migration 000072).
const ActionClassExecute = "privacy.retention.execute"

// RetentionJobType is the durable job type Runner.HandleJob is registered
// under (see internal/app's worker wiring).
const RetentionJobType = "retention.execute"

// SecurityPolicyReader mirrors legalhold's own narrow port: whether an
// organization currently requires step-up for sensitive actions. A nil
// reader, or one that errors, means "assume required" -- never "assume not
// required" -- same fail-closed default legalhold and securitysettings
// already established.
type SecurityPolicyReader interface {
	MFARequiredForSensitiveActions(ctx context.Context, organizationID string) (bool, error)
}

// Actor identifies who is initiating a run.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// Administrator implements the retention-execution administration surface:
// initiate (DRY_RUN runs synchronously in the request; EXECUTE runs
// enqueue a durable job), read back a run and its records.
type Administrator struct {
	pool           *pgxpool.Pool
	store          *Store
	runner         *Runner
	queue          *jobs.Queue
	stepUp         stepup.Verifier
	securityPolicy SecurityPolicyReader
	recorder       *audit.Recorder
}

// NewAdministrator builds the administrator. stepUp/securityPolicy may be
// nil, matching every other optional-dependency convention in this
// service -- step-up is then recorded if a reference is supplied but never
// actually verified, same disclosed posture legalhold/securitysettings
// already document.
func NewAdministrator(pool *pgxpool.Pool, runner *Runner, queue *jobs.Queue,
	stepUp stepup.Verifier, securityPolicy SecurityPolicyReader, recorder *audit.Recorder) *Administrator {
	if recorder == nil {
		recorder = audit.NewRecorder()
	}
	return &Administrator{
		pool: pool, store: NewStore(pool), runner: runner, queue: queue,
		stepUp: stepUp, securityPolicy: securityPolicy, recorder: recorder,
	}
}

func (a *Administrator) verifyStepUp(ctx context.Context, identityID, organizationID, sessionID, reference string) error {
	required := true
	if a.securityPolicy != nil {
		if r, err := a.securityPolicy.MFARequiredForSensitiveActions(ctx, organizationID); err == nil {
			required = r
		}
	}
	if !required || a.stepUp == nil {
		return nil
	}
	return a.stepUp.Verify(ctx, reference, identityID, sessionID, organizationID, ActionClassExecute)
}

// InitiateInput describes a new retention run request.
type InitiateInput struct {
	Actor           Actor
	IdentityID      string
	SessionID       string
	StepUpReference string
	RetentionClass  string
	Mode            string
	IdempotencyKey  string
	RequestID       string
	CorrelationID   string
}

// Initiate validates and starts a retention run. DRY_RUN executes fully
// inline before returning (safe: read-only, and the same DefaultBatchSize
// bound applies) so a preview response always carries real, just-computed
// counts. EXECUTE enqueues a durable job (internal/jobs) and returns
// immediately with the run PENDING -- a real mutation at any meaningful
// scale belongs behind the job framework's lease/retry/idempotency
// machinery, not a synchronous HTTP request.
//
// Idempotent: a second Initiate call with the same (organization, key)
// returns the SAME run without starting a second one (no re-verification of
// step-up either -- the action was already authorized once for this exact
// key).
func (a *Administrator) Initiate(ctx context.Context, input InitiateInput) (Run, error) {
	class := strings.TrimSpace(input.RetentionClass)
	mode := strings.TrimSpace(input.Mode)
	key := strings.TrimSpace(input.IdempotencyKey)
	if class == "" {
		return Run{}, apperrors.BadRequest("retentionClass is required.", nil)
	}
	if mode != ModeDryRun && mode != ModeExecute {
		return Run{}, apperrors.BadRequest("mode must be DRY_RUN or EXECUTE.", nil)
	}
	if key == "" {
		return Run{}, apperrors.BadRequest("idempotencyKey is required.", nil)
	}
	if !a.runner.SupportsClass(class) {
		return Run{}, apperrors.Forbidden(
			fmt.Sprintf("Retention class %s has no implemented execution -- it can be configured but not yet executed.", class), nil)
	}

	if existing, err := a.store.GetByIdempotencyKey(ctx, input.Actor.OrganizationID, key); err == nil {
		return existing, nil
	}

	if err := a.verifyStepUp(ctx, input.IdentityID, input.Actor.OrganizationID, input.SessionID, input.StepUpReference); err != nil {
		return Run{}, err
	}

	run, err := a.store.CreateRun(ctx, a.pool, Run{
		OrganizationID: input.Actor.OrganizationID, RetentionClass: class, Mode: mode,
		InitiatedBy: input.Actor.ActorID, IdempotencyKey: key,
	})
	if err != nil {
		return Run{}, err
	}

	if err := a.recorder.Record(ctx, a.pool, audit.Entry{
		OrganizationID: input.Actor.OrganizationID, ActorType: input.Actor.ActorType, ActorID: input.Actor.ActorID,
		Action: "retention.run.initiated", ResourceType: "retention_run", ResourceID: run.ID,
		NewValues:       map[string]any{"retentionClass": class, "mode": mode},
		RequestID:       input.RequestID,
		CorrelationID:   input.CorrelationID,
		StepUpReference: input.StepUpReference,
	}); err != nil {
		return Run{}, err
	}

	if mode == ModeDryRun {
		for {
			exhausted, runErr := a.runner.RunOnce(ctx, run.ID)
			if runErr != nil {
				return a.store.GetRun(ctx, input.Actor.OrganizationID, run.ID)
			}
			if exhausted {
				break
			}
		}
		return a.store.GetRun(ctx, input.Actor.OrganizationID, run.ID)
	}

	jobID, err := a.queue.Enqueue(ctx, a.pool, jobs.EnqueueRequest{
		OrganizationID: input.Actor.OrganizationID, JobType: RetentionJobType,
		DedupeKey: run.ID, IdempotencyKey: key,
		Payload:       jobPayload{RunID: run.ID},
		CorrelationID: input.CorrelationID,
	})
	if err != nil {
		return Run{}, err
	}
	if err := a.store.SetJobID(ctx, a.pool, run.ID, jobID); err != nil {
		return Run{}, err
	}
	run.JobID = jobID
	return run, nil
}

// Get returns one run scoped to organizationID.
func (a *Administrator) Get(ctx context.Context, organizationID, runID string) (Run, error) {
	return a.store.GetRun(ctx, organizationID, runID)
}

// ListRecords returns every immutable per-record outcome for a run --
// operator detail view; callers that must expose nothing sensitive to a
// broader audience should use the run's own counters, not this.
func (a *Administrator) ListRecords(ctx context.Context, runID string) ([]RunRecord, error) {
	return a.store.ListRunRecords(ctx, runID)
}
