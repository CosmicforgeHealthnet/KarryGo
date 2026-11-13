package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// TestWS4ProofVerifyAndReject covers spec 49.3 GET /proofs/{id} and POST
// /proofs/{id}/verify: decisions against the policy snapshot, separation
// between submitter and verifier, finality, immutability of the record,
// mandatory-proof satisfaction, tenant isolation, audit and outbox.
func TestWS4ProofVerifyAndReject(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _ := section1921Scope(t, pool)
	org := scope.OrganizationID
	submitter := scope
	reviewer := scope
	reviewer.ActorID, reviewer.OrganizationUserID = ids.New("usr"), ids.New("usr")
	otherScope, _ := section1921Scope(t, pool)
	p := proof.NewService(proof.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), HashSecret: []byte("0123456789abcdef0123456789abcdef"), Flags: proof.Flags{Policies: true, Records: true}})

	_, snap, err := p.SavePolicy(ctx, scope, "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: org, RequiredProofTypes: json.RawMessage(`["OTP"]`), MinimumCount: 1, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	newOTP := func(key, order string) proof.Record {
		r, err := p.CreateRecord(ctx, submitter, proof.RecordInput{PolicySnapshotReference: snap.ID, ProofType: "OTP", OrderID: order, ActorType: "ORGANIZATION_USER", Value: "135790", SourceTimestamp: time.Now().UTC(), Source: "MANUAL", IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := newOTP("ws4-proof-1", "order-ws4-1")
	if ok, _ := p.MandatorySatisfied(ctx, org, snap.ID, "order-ws4-1", "", "", ""); ok {
		t.Fatal("pending proof satisfied the policy")
	}
	if _, err = p.Decide(ctx, submitter, first.ID, "VERIFIED", proof.DecisionInput{}); appStatus(err) != 403 {
		t.Fatalf("self verification = %v", err)
	}
	if _, err = p.Decide(ctx, otherScope, first.ID, "VERIFIED", proof.DecisionInput{}); appStatus(err) != 404 {
		t.Fatalf("cross-tenant verification = %v", err)
	}
	verified, err := p.Decide(ctx, reviewer, first.ID, "verified", proof.DecisionInput{})
	if err != nil || verified.EffectiveStatus != "VERIFIED" || len(verified.Verifications) != 1 || verified.Verifications[0].AuthorityType != proof.AuthorityOrganization {
		t.Fatalf("verify = %+v %v", verified, err)
	}
	if verified.Record.VerificationStatus != "PENDING" {
		t.Fatal("the immutable record was rewritten")
	}
	if _, err = p.Decide(ctx, reviewer, first.ID, "REJECTED", proof.DecisionInput{ReasonCodes: []string{"LATE"}}); appStatus(err) != 409 {
		t.Fatalf("second decision = %v", err)
	}
	if ok, err := p.MandatorySatisfied(ctx, org, snap.ID, "order-ws4-1", "", "", ""); err != nil || !ok {
		t.Fatalf("verified proof did not satisfy the policy: %v", err)
	}

	second := newOTP("ws4-proof-2", "order-ws4-2")
	if _, err = p.Decide(ctx, reviewer, second.ID, "REJECTED", proof.DecisionInput{}); appStatus(err) != 400 {
		t.Fatalf("rejection without reason = %v", err)
	}
	rejected, err := p.Decide(ctx, reviewer, second.ID, "REJECTED", proof.DecisionInput{ReasonCodes: []string{"wrong_recipient"}})
	if err != nil || rejected.EffectiveStatus != "REJECTED" {
		t.Fatalf("reject = %+v %v", rejected, err)
	}
	if ok, _ := p.MandatorySatisfied(ctx, org, snap.ID, "order-ws4-2", "", "", ""); ok {
		t.Fatal("rejected proof satisfied the policy")
	}

	// A record that does not satisfy its snapshot cannot be verified: the
	// snapshot requires OTP only, so a stored PHOTO proof (written directly,
	// as an authority projection would) fails the re-check.
	photo := ids.New("prfrec")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_proof_records(id,organization_id,policy_snapshot_reference,proof_type,order_id,actor_type,actor_reference,media_file_id,source_timestamp,source,idempotency_key)VALUES($1,$2,$3,'PHOTO','order-ws4-3','ORGANIZATION_USER',$4,'media-x',now(),'IMPORT',$1)`, photo, org, snap.ID, submitter.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Decide(ctx, reviewer, photo, "VERIFIED", proof.DecisionInput{}); appStatus(err) != 409 {
		t.Fatalf("verifying a proof outside its policy = %v", err)
	}

	got, err := p.GetRecord(ctx, scope, first.ID)
	if err != nil || got.EffectiveStatus != "VERIFIED" {
		t.Fatalf("get = %+v %v", got, err)
	}
	if _, err = p.GetRecord(ctx, otherScope, first.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant get = %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_proof_verifications SET status='REJECTED' WHERE proof_record_id=$1`, first.ID); err == nil {
		t.Fatal("verification decision was mutable")
	}

	// Concurrent decisions on one record: exactly one lands.
	third := newOTP("ws4-proof-3", "order-ws4-4")
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, d := range []string{"VERIFIED", "REJECTED"} {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			_, e := p.Decide(ctx, reviewer, third.ID, d, proof.DecisionInput{ReasonCodes: []string{"RACE"}})
			outcomes <- e
		}(d)
	}
	wg.Wait()
	close(outcomes)
	statuses := map[int]int{}
	for e := range outcomes {
		statuses[appStatus(e)]++
	}
	if statuses[0] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent decisions = %v", statuses)
	}

	if ws4CountOutbox(t, pool, org, "proof.verified.v1")+ws4CountOutbox(t, pool, org, "proof.rejected.v1") != 3 {
		t.Fatal("proof decision events missing")
	}
	if ws4CountAudit(t, pool, org, "proof.verified") < 1 || ws4CountAudit(t, pool, org, "proof.rejected") < 1 {
		t.Fatal("proof decision audit missing")
	}
}

// recordingAuthority is an execution-service restriction authority double
// that records every command and can fail its first calls.
type recordingAuthority struct {
	mu         sync.Mutex
	failFirst  int
	applies    []platformclients.RestrictionCommand
	clears     []platformclients.RestrictionCommand
	configured bool
}

func (a *recordingAuthority) Configured() bool { return a.configured }
func (a *recordingAuthority) Apply(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applies = append(a.applies, in)
	if a.failFirst > 0 {
		a.failFirst--
		return platformclients.RestrictionOutcome{}, &platformclients.Error{Service: "test-authority", Outcome: platformclients.OutcomeTransient}
	}
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "APPLIED", SourceVersion: in.ExpectedVersion}, nil
}
func (a *recordingAuthority) Read(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: id, State: "APPLIED", SourceVersion: 1}, nil
}
func (a *recordingAuthority) Clear(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clears = append(a.clears, in)
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "CLEARED", SourceVersion: in.ExpectedVersion}, nil
}
func (a *recordingAuthority) ReadClear(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: id, State: "CLEARED", SourceVersion: 2}, nil
}
func (a *recordingAuthority) applyCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.applies)
}

// TestWS4RestrictionPropagationReconcilesUntilConfirmed covers spec 19.1 /
// 19.5: a provider restriction is persisted and enforced locally, emits a
// durable event, calls each execution authority, and -- when an authority
// fails or is not reachable from this deployment -- is retried by the durable
// reconciliation job until every authority confirms it.
func TestWS4RestrictionPropagationReconcilesUntilConfirmed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _ := section1921Scope(t, pool)
	org := scope.OrganizationID
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	dispatch := &recordingAuthority{configured: true, failFirst: 1}
	// Hauling is not configured in this deployment at first.
	first := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: dispatch, Hauling: nil, Flags: compliance.Flags{Compliance: true, Restrictions: true}})

	r, err := first.ApplyRestriction(ctx, scope, compliance.RestrictInput{SubjectType: "PROVIDER", SubjectReference: "membership-ws4", ReasonCode: "safety_block", SourceType: "INCIDENT", SourceReference: "inc-ws4", Severity: "CRITICAL"})
	if err != nil {
		t.Fatalf("a durable restriction must not fail because propagation did: %v", err)
	}
	if r.State != "APPLIED" {
		t.Fatalf("restriction = %+v", r)
	}
	// Persisted and enforced locally at once.
	gate, err := first.CheckSubject(ctx, org, "PROVIDER", "membership-ws4")
	if err != nil || gate.Eligible {
		t.Fatalf("local enforcement = %+v %v", gate, err)
	}
	// Durable events in the same transaction as the restriction.
	if ws4CountOutbox(t, pool, org, "restriction.applied.v1") != 1 || ws4CountOutbox(t, pool, org, "restriction.propagation_required.v1") != 1 {
		t.Fatal("restriction events missing")
	}
	// The execution service was called with the restriction's command.
	if dispatch.applyCount() != 1 || dispatch.applies[0].RestrictionID != r.ID || dispatch.applies[0].SubjectReference != "membership-ws4" || dispatch.applies[0].ReasonCode != "SAFETY_BLOCK" {
		t.Fatalf("dispatch commands = %+v", dispatch.applies)
	}
	view, err := first.RestrictionDetail(ctx, scope, r.ID)
	if err != nil || view.FullyPropagated || len(view.Propagation) != 2 {
		t.Fatalf("propagation after first attempt = %+v %v", view, err)
	}
	for _, op := range view.Propagation {
		if op.Status != "RETRY_SCHEDULED" || op.Attempts != 1 {
			t.Fatalf("operation = %+v", op)
		}
		if op.Authority == "HAULING" && op.LastErrorCode != "AUTHORITY_NOT_CONFIGURED" {
			t.Fatalf("hauling operation = %+v", op)
		}
	}
	var jobsQueued int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=$2`, org, compliance.JobRestrictionReconcile).Scan(&jobsQueued); err != nil || jobsQueued != 2 {
		t.Fatalf("reconcile jobs = %d %v", jobsQueued, err)
	}

	// Configuration is restored; the durable reconciliation job (run by the
	// real worker) confirms the restriction at every authority.
	hauling := &recordingAuthority{configured: true}
	second := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: dispatch, Hauling: hauling, Flags: compliance.Flags{Compliance: true, Restrictions: true}})
	worker := &jobs.Worker{Queue: jobs.NewQueue(pool), Metrics: observability.NewMetrics(), Owner: "ws4-test-worker", PollInterval: 20 * time.Millisecond, LeaseDuration: time.Minute, HeartbeatInterval: time.Second}
	second.RegisterWorkers(worker)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	go worker.Run(runCtx)
	for {
		view, err = second.RestrictionDetail(ctx, scope, r.ID)
		if err == nil && view.FullyPropagated {
			break
		}
		if runCtx.Err() != nil {
			t.Fatalf("restriction never confirmed: %+v %v", view, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	if hauling.applyCount() == 0 || dispatch.applyCount() < 2 {
		t.Fatalf("reconciliation calls dispatch=%d hauling=%d", dispatch.applyCount(), hauling.applyCount())
	}
	if ws4CountOutbox(t, pool, org, "restriction.propagation_confirmed.v1") != 2 {
		t.Fatal("propagation confirmations missing")
	}
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_restriction_attempts WHERE organization_id=$1`, org).Scan(&attempts); err != nil || attempts != 4 {
		t.Fatalf("attempt rows = %d %v", attempts, err)
	}

	// Lifting propagates the same way and only clears once confirmed.
	cleared, err := second.ClearRestriction(ctx, scope, r.ID, "Investigation closed", view.Version)
	if err != nil || cleared.State != "CLEARED" {
		t.Fatalf("clear = %+v %v", cleared, err)
	}
	if gate, _ = second.CheckSubject(ctx, org, "PROVIDER", "membership-ws4"); !gate.Eligible {
		t.Fatal("cleared restriction still blocks")
	}
	list, err := second.ListRestrictions(ctx, scope, "PROVIDER", "membership-ws4", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d %v", len(list), err)
	}
	otherScope, _ := section1921Scope(t, pool)
	if _, err = second.RestrictionDetail(ctx, otherScope, r.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant restriction = %v", err)
	}
}

// TestWS4ComplianceCheckRunsEveryRequirementStrictestGoverns covers spec
// 49.1-49.3 compliance checks: every active requirement of the subject is
// evaluated in one transaction, the strictest result governs, a blocking
// failure applies a restriction, and the check is immutable and tenant scoped.
func TestWS4ComplianceCheckRunsEveryRequirementStrictestGoverns(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, branch := section1921Scope(t, pool)
	org := scope.OrganizationID
	asset := ws4Asset(t, pool, org, branch)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	authority := &recordingAuthority{configured: true}
	c := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: authority, Hauling: authority, Flags: compliance.Flags{Compliance: true, Restrictions: true}})
	activate := func(code, doc string, blocks bool) compliance.Requirement {
		req, err := c.CreateRequirement(ctx, scope, compliance.Requirement{Code: code, Name: code, SubjectType: "ASSET", RequirementType: "DOCUMENT", RequiredDocumentType: doc, Severity: "HIGH", BlocksOperation: blocks, EffectiveFrom: time.Now().Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		req, err = c.UpdateRequirement(ctx, scope, req.ID, req.Version, compliance.Requirement{}, "ACTIVATE")
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	warning := activate("ASSET_REGISTRATION", "REGISTRATION", false)
	blocking := activate("ASSET_INSURANCE", "INSURANCE", true)
	if _, err := c.CreateRequirement(ctx, scope, compliance.Requirement{Code: "DRAFT_ONLY", Name: "draft", SubjectType: "ASSET", RequirementType: "DOCUMENT", RequiredDocumentType: "X", Severity: "HIGH", EffectiveFrom: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}

	check, err := c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "asset", SubjectReference: asset, SourceType: "manual", SourceVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if check.RequirementCount != 2 || check.OverallState != compliance.StateRestricted || check.GoverningRequirementID != blocking.ID || len(check.RestrictionReferences) != 1 {
		t.Fatalf("check = %+v", check)
	}
	states := map[string]string{}
	for _, r := range check.Results {
		states[r.RequirementID] = r.State
	}
	if states[warning.ID] != compliance.StateNonCompliant || states[blocking.ID] != compliance.StateRestricted {
		t.Fatalf("results = %+v", check.Results)
	}
	if authority.applyCount() != 2 {
		t.Fatalf("restriction propagation calls = %d", authority.applyCount())
	}
	got, err := c.GetCheck(ctx, scope, check.ID)
	if err != nil || got.OverallState != compliance.StateRestricted || len(got.Results) != 2 {
		t.Fatalf("get check = %+v %v", got, err)
	}
	list, err := c.ListChecks(ctx, scope, "ASSET", asset)
	if err != nil || len(list) != 1 {
		t.Fatalf("list checks = %d %v", len(list), err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_compliance_checks SET overall_state='COMPLIANT' WHERE id=$1`, check.ID); err == nil {
		t.Fatal("compliance check was mutable")
	}
	// An older evidence version cannot overwrite a newer evaluation.
	if _, err = c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "ASSET", SubjectReference: asset, SourceType: "MANUAL", SourceVersion: 0}); appStatus(err) != 409 {
		t.Fatalf("stale evidence version = %v", err)
	}
	// Validation and tenant isolation.
	if _, err = c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "PLANET", SubjectReference: asset, SourceType: "MANUAL"}); appStatus(err) != 400 {
		t.Fatalf("invalid subject type = %v", err)
	}
	otherScope, otherBranch := section1921Scope(t, pool)
	foreignAsset := ws4Asset(t, pool, otherScope.OrganizationID, otherBranch)
	if _, err = c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "ASSET", SubjectReference: foreignAsset, SourceType: "MANUAL", SourceVersion: 1}); appStatus(err) != 404 {
		t.Fatalf("cross-tenant subject = %v", err)
	}
	if _, err = c.GetCheck(ctx, otherScope, check.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant check = %v", err)
	}
	if _, err = c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "ORGANIZATION", SubjectReference: otherScope.OrganizationID, SourceType: "MANUAL", SourceVersion: 1}); appStatus(err) != 404 {
		t.Fatalf("another organization as subject = %v", err)
	}
	// A subject with no applicable requirement is NOT_APPLICABLE, still recorded.
	none, err := c.RunCheck(ctx, scope, compliance.CheckInput{SubjectType: "ORGANIZATION", SubjectReference: org, SourceType: "MANUAL", SourceVersion: 1})
	if err != nil || none.OverallState != compliance.StateNotApplicable || none.RequirementCount != 0 {
		t.Fatalf("no requirements = %+v %v", none, err)
	}
	if ws4CountOutbox(t, pool, org, "compliance_check.completed.v1") != 2 || ws4CountAudit(t, pool, org, "compliance_check.completed") != 2 {
		t.Fatal("compliance check events or audit missing")
	}
	// The whole check is one transaction: a failure leaves no partial check.
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_compliance_checks WHERE organization_id=$1`, org).Scan(&count); err != nil || count != 2 {
		t.Fatalf("checks = %d %v", count, err)
	}
}
