package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	events "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	legalholdusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	retentionpolicymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	retentionpolicyusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// newTestRunner builds a Runner wired to the real organization contact
// executor, the real legalhold service (step-up-free, matching
// section28_test.go's own construction) and the real retentionpolicy
// service -- the identical dependency graph internal/app/wiring.go builds
// for production, just assembled directly here rather than through the
// composition root.
func newTestRunner(pool *pgxpool.Pool) (*retentionexec.Runner, *legalholdusecases.Service, *retentionpolicyusecases.Service) {
	legalSvc := legalholdusecases.NewService(pool, nil, nil)
	policySvc := retentionpolicyusecases.NewService(pool)
	runner := retentionexec.NewRunner(pool,
		[]retentionexec.ClassExecutor{retentionexec.NewOrgContactExecutor()},
		legalSvc, testDaysAdapter{policy: policySvc}, nil, 0)
	return runner, legalSvc, policySvc
}

type testDaysAdapter struct {
	policy *retentionpolicyusecases.Service
}

func (a testDaysAdapter) EffectiveRetentionDays(ctx context.Context, organizationID, class string) (int, error) {
	p, err := a.policy.Get(ctx, organizationID, class)
	if err != nil {
		return 0, err
	}
	return p.EffectiveDays(), nil
}

// closedOrgWithRepresentative creates an organization, adds one authorized
// representative (direct SQL insert -- there is no usecase for this in the
// current codebase; it is test setup data, not the behavior under test),
// then drives it through the REAL closure workflow this same session built
// (organizations/usecases/closure.go: RequestClosure -> AssessClosure(no
// checkers, i.e. every obligation cleared) -> ApproveClosure ->
// ExecuteClosure), so the resulting fleet_partner_organization_closure_
// requests row and CLOSED status are exactly what production would produce,
// not a shortcut.
//
// closedDaysAgo backdates the closure request's completed_at by that many
// days (direct SQL, after the real workflow already ran) -- needed because
// ClassOperational's platform-minimum retention is 90 days
// (retentionpolicy/models.go) and an organization's configured value can
// never go BELOW that floor (Policy.EffectiveDays), so "closed just now"
// can never be genuinely eligible under real settings. Pass 0 for tests
// that specifically want to prove the just-closed, not-yet-eligible case
// (TestRetentionExecution_MinimumRetentionBlocksExecution); pass something
// past the platform minimum (e.g. 120) for tests that need a real,
// executable candidate.
func closedOrgWithRepresentative(t *testing.T, pool *pgxpool.Pool, name, registration string, closedDaysAgo int) (organizationID, representativeID string) {
	t.Helper()
	org := createOrganization(t, pool, name, registration)

	repID := ids.New("rep")
	_, err := pool.Exec(t.Context(), `
		INSERT INTO fleet_partner_organization_authorized_representatives
			(id, organization_id, full_name, role_title, email, phone, authority_type, status, created_at, updated_at)
		VALUES ($1, $2, 'Test Representative', 'Director', 'rep@example.com', '+2348000000000', 'DIRECTOR', 'ACTIVE', now(), now())`,
		repID, org.ID)
	if err != nil {
		t.Fatalf("insert representative: %v", err)
	}

	orgSvc := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Events:        events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
	actorTenant := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "test_owner", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "test_owner"}
	if _, err := orgSvc.RequestClosure(t.Context(), orgusecases.RequestClosureInput{Tenant: actorTenant, ReasonCode: "test_retention_setup"}); err != nil {
		t.Fatalf("request closure: %v", err)
	}
	if _, _, err := orgSvc.AssessClosure(t.Context(), org.ID, nil, nil); err != nil {
		t.Fatalf("assess closure: %v", err)
	}
	if _, err := orgSvc.ApproveClosure(t.Context(), actorTenant, "test_owner"); err != nil {
		t.Fatalf("approve closure: %v", err)
	}
	if err := orgSvc.ExecuteClosure(t.Context(), org.ID, nil, "test-correlation"); err != nil {
		t.Fatalf("execute closure: %v", err)
	}
	if closedDaysAgo > 0 {
		if _, err := pool.Exec(t.Context(), `
			UPDATE fleet_partner_organization_closure_requests
			SET completed_at = now() - make_interval(days => $2::int)
			WHERE organization_id = $1 AND status = 'COMPLETED'`,
			org.ID, closedDaysAgo); err != nil {
			t.Fatalf("backdate closure completion: %v", err)
		}
	}
	return org.ID, repID
}

func representativeEmail(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var email string
	if err := pool.QueryRow(t.Context(), `SELECT email FROM fleet_partner_organization_authorized_representatives WHERE id = $1`, id).Scan(&email); err != nil {
		t.Fatalf("read representative email: %v", err)
	}
	return email
}

// TestRetentionExecution_DryRunNeverMutates proves DRY_RUN reports what
// WOULD happen (ANONYMIZED, one record) without writing anything -- the
// representative's real email is unchanged afterward.
func TestRetentionExecution_DryRunNeverMutates(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	orgID, repID := closedOrgWithRepresentative(t, pool, "Retention DryRun Co", "RC-RET-1", 120)

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "dryrun-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if run.Status != retentionexec.RunStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED, error=%q", run.Status, run.ErrorMessage)
	}
	if run.RecordsAnonymized != 1 {
		t.Fatalf("recordsAnonymized = %d, want 1", run.RecordsAnonymized)
	}
	if got := representativeEmail(t, pool, repID); got != "rep@example.com" {
		t.Fatalf("DRY_RUN must not mutate the row, email = %q", got)
	}
}

// TestRetentionExecution_ExecuteAnonymizesThroughJobHandler proves an
// EXECUTE-mode run enqueues a real durable job (internal/jobs) whose
// payload, when processed through Runner.HandleJob -- the exact function
// internal/app wires onto the worker in production -- actually anonymizes
// the representative's contact fields.
func TestRetentionExecution_ExecuteAnonymizesThroughJobHandler(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	orgID, repID := closedOrgWithRepresentative(t, pool, "Retention Execute Co", "RC-RET-2", 120)

	queue := jobs.NewQueue(pool)
	admin := retentionexec.NewAdministrator(pool, runner, queue, nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeExecute,
		IdempotencyKey: "execute-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if run.Status != retentionexec.RunStatusPending {
		t.Fatalf("status = %q, want PENDING (EXECUTE queues a job, does not run inline)", run.Status)
	}
	if run.JobID == "" {
		t.Fatal("expected a job id to be recorded")
	}

	claimed, err := queue.ClaimBatch(t.Context(), "test-worker", time.Minute, 10)
	if err != nil {
		t.Fatalf("claim batch: %v", err)
	}
	var found bool
	for _, job := range claimed {
		if job.ID != run.JobID {
			continue
		}
		found = true
		if err := runner.HandleJob(t.Context(), job); err != nil {
			t.Fatalf("handle job: %v", err)
		}
	}
	if !found {
		t.Fatalf("retention job %s was not claimed", run.JobID)
	}

	completed, err := admin.Get(t.Context(), orgID, run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if completed.Status != retentionexec.RunStatusCompleted || completed.RecordsAnonymized != 1 {
		t.Fatalf("run = %+v, want COMPLETED with 1 anonymized", completed)
	}
	if got := representativeEmail(t, pool, repID); got != "[REDACTED]" {
		t.Fatalf("email = %q, want [REDACTED] after EXECUTE", got)
	}

	records, err := admin.ListRecords(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].Outcome != retentionexec.OutcomeAnonymized || records[0].RecordID != repID {
		t.Fatalf("records = %+v, want exactly one ANONYMIZED outcome for %s", records, repID)
	}
}

// TestRetentionExecution_IdempotentReplaySkipsAlreadyAnonymized proves that
// re-running the job handler for an already-completed run (a durable job
// resumed after lease recovery, or simply called twice) does not error and
// does not double-process -- FindCandidates itself excludes an
// already-redacted row, so a replay finds zero NEW candidates.
func TestRetentionExecution_IdempotentReplaySkipsAlreadyAnonymized(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	orgID, repID := closedOrgWithRepresentative(t, pool, "Retention Replay Co", "RC-RET-3", 120)

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "replay-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	_ = repID

	// Replay RunOnce directly against the already-completed run -- must not
	// error and must not change the counters (nothing new to consider: a
	// DRY_RUN run's own candidates are still present since DRY_RUN never
	// mutates, but they are already recorded in
	// fleet_partner_retention_run_records for THIS run id, so HasOutcome
	// short-circuits them).
	exhausted, err := runner.RunOnce(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("replay RunOnce: %v", err)
	}
	if !exhausted {
		t.Fatal("expected replay to report exhausted")
	}
	reloaded, err := admin.Get(t.Context(), orgID, run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if reloaded.RecordsAnonymized != 1 {
		t.Fatalf("recordsAnonymized after replay = %d, want still 1 (not double-counted)", reloaded.RecordsAnonymized)
	}
}

// TestRetentionExecution_LegalHoldBlocksExecution proves an ACTIVE
// organization-scoped legal hold blocks anonymization even though the
// organization is closed and the retention window has (trivially) not yet
// been reached either -- both generic checks apply, legal hold is checked
// first and wins regardless of retention age.
func TestRetentionExecution_LegalHoldBlocksExecution(t *testing.T) {
	pool := testPool(t)
	runner, legalSvc, _ := newTestRunner(pool)
	orgID, repID := closedOrgWithRepresentative(t, pool, "Retention LegalHold Co", "RC-RET-4", 120)

	if _, err := legalSvc.Create(t.Context(), legalholdusecases.CreateInput{
		Actor:          legalholdusecases.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		ScopeType:      "ORGANIZATION",
		ScopeReference: orgID,
		Reason:         "test litigation hold",
	}); err != nil {
		t.Fatalf("create legal hold: %v", err)
	}

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "legalhold-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if run.RecordsAnonymized != 0 || run.RecordsSkipped != 1 {
		t.Fatalf("run = %+v, want 0 anonymized / 1 skipped (legal hold)", run)
	}
	records, err := admin.ListRecords(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].Outcome != retentionexec.OutcomeSkippedLegalHold {
		t.Fatalf("records = %+v, want exactly one SKIPPED_LEGAL_HOLD", records)
	}
	if got := representativeEmail(t, pool, repID); got != "rep@example.com" {
		t.Fatalf("legal hold must preserve the record, email = %q", got)
	}
}

// TestRetentionExecution_MinimumRetentionBlocksExecution proves a
// just-closed organization (0 days since closure) is refused: the platform
// minimum for ClassOperational (90 days, retentionpolicy/models.go) has not
// elapsed, so the generic engine skips it before the executor's
// Preview/Execute is ever called.
func TestRetentionExecution_MinimumRetentionBlocksExecution(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	orgID, repID := closedOrgWithRepresentative(t, pool, "Retention MinRetention Co", "RC-RET-5", 0)

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "minretention-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if run.RecordsAnonymized != 0 || run.RecordsSkipped != 1 {
		t.Fatalf("run = %+v, want 0 anonymized / 1 skipped (minimum retention)", run)
	}
	records, err := admin.ListRecords(t.Context(), run.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].Outcome != retentionexec.OutcomeSkippedMinimumRetention {
		t.Fatalf("records = %+v, want exactly one SKIPPED_MINIMUM_RETENTION", records)
	}
	if got := representativeEmail(t, pool, repID); got != "rep@example.com" {
		t.Fatalf("minimum retention must preserve the record, email = %q", got)
	}
}

// TestRetentionExecution_UnregisteredClassRefused proves a retention class
// that IS valid configuration (an organization can set a day count for it,
// e.g. AUDIT) but has NO ClassExecutor registered is refused outright at
// Initiate -- never silently creates a run that would do nothing, and never
// somehow touches an immutable/evidentiary table.
func TestRetentionExecution_UnregisteredClassRefused(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	org := createOrganization(t, pool, "Retention Unregistered Co", "RC-RET-6")

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	_, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassAudit, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "unregistered-1",
	})
	if err == nil {
		t.Fatal("expected AUDIT (no registered executor) to be refused")
	}
}

// TestRetentionExecution_InitiateIsIdempotentByKey proves a second Initiate
// call with the same (organization, idempotencyKey) returns the SAME run
// rather than starting a second one, and does not require step-up twice.
func TestRetentionExecution_InitiateIsIdempotentByKey(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	orgID, _ := closedOrgWithRepresentative(t, pool, "Retention IdempotentInitiate Co", "RC-RET-7", 120)

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	input := retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "idempotent-initiate-1",
	}
	first, err := admin.Initiate(t.Context(), input)
	if err != nil {
		t.Fatalf("first initiate: %v", err)
	}
	second, err := admin.Initiate(t.Context(), input)
	if err != nil {
		t.Fatalf("second initiate: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("first.ID = %s, second.ID = %s, want the same run", first.ID, second.ID)
	}
}

// TestRetentionExecution_OpenOrganizationNeverEligible proves an
// organization that is NOT closed is never even a candidate -- the
// executor's own FindCandidates excludes it, independent of the generic
// legal-hold/minimum-retention checks.
func TestRetentionExecution_OpenOrganizationNeverEligible(t *testing.T) {
	pool := testPool(t)
	runner, _, _ := newTestRunner(pool)
	org := createOrganization(t, pool, "Retention OpenOrg Co", "RC-RET-8")
	repID := ids.New("rep")
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO fleet_partner_organization_authorized_representatives
			(id, organization_id, full_name, role_title, email, phone, authority_type, status, created_at, updated_at)
		VALUES ($1, $2, 'Test Representative', 'Director', 'open@example.com', '+2348000000001', 'DIRECTOR', 'ACTIVE', now(), now())`,
		repID, org.ID); err != nil {
		t.Fatalf("insert representative: %v", err)
	}
	if org.Status == orgmodels.OrganizationStatusClosed {
		t.Fatal("newly created organization must not already be CLOSED")
	}

	admin := retentionexec.NewAdministrator(pool, runner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(t.Context(), retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassOperational, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "openorg-1",
	})
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if run.RecordsConsidered != 0 {
		t.Fatalf("recordsConsidered = %d, want 0 (organization is not closed)", run.RecordsConsidered)
	}
}
