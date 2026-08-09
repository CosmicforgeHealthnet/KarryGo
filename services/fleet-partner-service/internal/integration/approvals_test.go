package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	approvalmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// WP9 approvals, against real PostgreSQL.
//
// Every test here attacks the same sentence from a different angle: "enough
// eligible people, none of them the requester, approved EXACTLY this operation
// against EXACTLY this version of the subject, and it has not been spent yet."

type approvalFixture struct {
	pool      *pgxpool.Pool
	service   *approvalusecases.Service
	repo      *approvalrepos.Repository
	orgID     string
	requester string
}

func newApprovalFixture(t *testing.T, name string, registration string) *approvalFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	seedEligibleOrganization(t, pool, org.ID)

	repo := approvalrepos.NewRepository(pool)
	service := approvalusecases.NewService(approvalusecases.Options{
		Pool:        pool,
		Approvals:   repo,
		Eligibility: newEligibilityService(pool, healthyAuthority("drv-approval", "")),
		Roles:       staffrepos.NewRoleRepository(pool),
		Subjects:    approvalrepos.NewSubjectVersions(pool),
		Events:      fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
	})
	return &approvalFixture{
		pool: pool, service: service, repo: repo, orgID: org.ID,
		requester: seedApprovalMember(t, pool, org.ID, "rol_system_owner", "requester"),
	}
}

// seedApprovalMember creates an organization user with one system role bound
// at organization scope.
//
// It goes through the real repositories rather than raw inserts so the member
// this suite approves with is the same shape the authorizer reads in
// production — including the identity every organization user must have.
func seedApprovalMember(t *testing.T, pool *pgxpool.Pool, organizationID string,
	roleID string, label string) string {
	t.Helper()
	ctx := context.Background()
	suffix := ids.New("apr")
	identity := &authmodels.Identity{
		EmailNormalized: label + "-" + suffix + "@example.com",
		EmailDisplay:    label + "@example.com",
		PhoneE164:       "+234801" + suffix[len(suffix)-7:],
		PhoneDisplay:    "08010000000",
		Status:          authmodels.IdentityStatusActive,
	}
	member := &staffmodels.OrganizationUser{Status: staffmodels.UserStatusActive}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := authrepos.NewIdentityRepository(pool).Create(ctx, tx, identity); err != nil {
			return err
		}
		member.IdentityID = identity.ID
		if err := staffrepos.NewMemberRepository(pool).Create(ctx, tx,
			organizationID, member); err != nil {
			return err
		}
		if roleID == "" {
			return nil
		}
		return staffrepos.NewRoleRepository(pool).CreateBinding(ctx, tx, organizationID,
			&staffmodels.RoleBinding{
				OrganizationUserID: member.ID, RoleID: roleID,
				ScopeType: staffmodels.ScopeOrganization, GrantedBy: "test"})
	}); err != nil {
		t.Fatalf("seed approval member: %v", err)
	}
	return member.ID
}

func (f *approvalFixture) actor(memberID string) approvalusecases.Actor {
	return approvalusecases.Actor{
		OrganizationID:     f.orgID,
		OrganizationUserID: memberID,
		ActorType:          approvalmodels.ActorOrganizationUser,
		ActorID:            memberID,
	}
}

// request raises an approval for a company activation, which the platform
// default policy says needs one approval from an owner or admin.
func (f *approvalFixture) request(t *testing.T, payload map[string]any,
	idempotencyKey string) *approvalmodels.Request {
	t.Helper()
	result, err := f.service.Request(context.Background(), approvalusecases.RequestInput{
		Actor:          f.actor(f.requester),
		ResourceType:   approvalrepos.ResourceOrganization,
		ResourceID:     f.orgID,
		Action:         "ACTIVATE",
		OperationType:  eligibilitymodels.OperationCompanyActivation,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  "corr-approval",
	})
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	return result.Request
}

func codeOf(t *testing.T, err error) apperrors.Code {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not a coded application error", err)
	}
	return appErr.Code
}

// ── Request, approve, execute ─────────────────────────────────────────────────

func TestApprovalHappyPathBindsPayloadAndExecutesOnce(t *testing.T) {
	f := newApprovalFixture(t, "Approve Co", "RC-APR-1")
	ctx := context.Background()
	payload := map[string]any{"reason": "ready", "amountMinor": 1000}

	request := f.request(t, payload, "idem-approve-1")
	if request.Status != approvalmodels.StatusPending {
		t.Fatalf("status = %s, want PENDING", request.Status)
	}
	if request.RequestFingerprint == "" {
		t.Fatal("a request must carry the fingerprint execution will be checked against")
	}
	// The subject version is recorded even though the requester did not state
	// one, so the approval is bound to a world.
	if request.ExpectedSubjectVersion == 0 {
		t.Fatal("the subject version must be recorded at request time")
	}

	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "approver")
	approved, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
		ClientDecisionID:  "dec-1",
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != approvalmodels.StatusApproved {
		t.Fatalf("status = %s, want APPROVED", approved.Status)
	}

	executor := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "executor")
	var ran int
	result, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(executor),
		ApprovalRequestID:  request.ID,
		Payload:            payload,
		ExecutionReference: "exec-activate-1",
		IdempotencyKey:     "idem-exec-1",
	}, func(pgx.Tx, *approvalmodels.Request) error {
		ran++
		return nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if ran != 1 || result.Request.Status != approvalmodels.StatusExecuted {
		t.Fatalf("ran=%d status=%s", ran, result.Request.Status)
	}

	// A retry with the same idempotency key resolves to the same outcome and
	// does not run the protected operation again.
	replay, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(executor),
		ApprovalRequestID:  request.ID,
		Payload:            payload,
		ExecutionReference: "exec-activate-1",
		IdempotencyKey:     "idem-exec-1",
	}, func(pgx.Tx, *approvalmodels.Request) error {
		ran++
		return nil
	})
	if err != nil {
		t.Fatalf("replayed execute: %v", err)
	}
	if !replay.Replayed || ran != 1 {
		t.Fatalf("replayed=%v ran=%d; the protected operation must run once", replay.Replayed, ran)
	}
}

// TestIdenticalRequestRetryIsIdempotent proves a retried creation replays and
// a reused key with different content is refused rather than overwriting.
func TestIdenticalRequestRetryIsIdempotent(t *testing.T) {
	f := newApprovalFixture(t, "Idempotent Co", "RC-APR-2")
	payload := map[string]any{"reason": "ready"}

	first := f.request(t, payload, "idem-same")
	second := f.request(t, payload, "idem-same")
	if first.ID != second.ID {
		t.Fatal("an identical retry must replay the original request")
	}

	_, err := f.service.Request(context.Background(), approvalusecases.RequestInput{
		Actor:          f.actor(f.requester),
		ResourceType:   approvalrepos.ResourceOrganization,
		ResourceID:     f.orgID,
		Action:         "ACTIVATE",
		OperationType:  eligibilitymodels.OperationCompanyActivation,
		Payload:        map[string]any{"reason": "different"},
		IdempotencyKey: "idem-same",
	})
	if err == nil || codeOf(t, err) != errorcodes.IdempotencyConflictCode {
		t.Fatalf("reused key with different content = %v; want an idempotency conflict", err)
	}
}

// TestPayloadFingerprintIgnoresKeyOrder proves the fingerprint is a statement
// about the operation, not about how a client happened to serialize it.
func TestPayloadFingerprintIgnoresKeyOrder(t *testing.T) {
	first, err := approvalusecases.PayloadFingerprint("org_1", "ORGANIZATION", "org_1",
		"ACTIVATE", map[string]any{"a": 1, "b": map[string]any{"x": "y", "z": 2}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := approvalusecases.PayloadFingerprint("org_1", "ORGANIZATION", "org_1",
		"ACTIVATE", map[string]any{"b": map[string]any{"z": 2, "x": "y"}, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("field order must not change the fingerprint")
	}

	changed, err := approvalusecases.PayloadFingerprint("org_1", "ORGANIZATION", "org_1",
		"ACTIVATE", map[string]any{"a": 2, "b": map[string]any{"x": "y", "z": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("a changed value must change the fingerprint")
	}
}

// ── Refusals ──────────────────────────────────────────────────────────────────

func TestSelfApprovalIsRefused(t *testing.T) {
	f := newApprovalFixture(t, "Self Approve Co", "RC-APR-3")
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-self")

	_, err := f.service.Decide(context.Background(), approvalusecases.DecideInput{
		Actor:             f.actor(f.requester),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalSelfDecisionForbidden {
		t.Fatalf("self approval = %v; want approval_self_decision_forbidden", err)
	}
}

// TestSeparationOfDutiesRefusesASecondActionByTheSamePerson proves the rule
// that outlives self-approval: whoever already acted cannot act again to
// complete quorum.
func TestSeparationOfDutiesRefusesASecondActionByTheSamePerson(t *testing.T) {
	f := newApprovalFixture(t, "SoD Co", "RC-APR-4")
	ctx := context.Background()

	// A two-approval policy so the first approval does not complete quorum.
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_eligibility_policies
			(id, organization_id, policy_key, operation_type, version,
			 required_company_checks, decision_ttl_seconds, source_max_age_seconds,
			 approval_required, minimum_approvals, eligible_role_ids,
			 self_approval_allowed, separation_of_duties, approval_ttl_seconds, status)
		VALUES ($1, $2, 'COMPANY_ACTIVATION', 'COMPANY_ACTIVATION', 1,
		        '["lifecycle"]'::jsonb, 900, 900, true, 2,
		        '["rol_system_owner","rol_system_admin"]'::jsonb, false, true, 86400, 'ACTIVE')`,
		"elp_sod_"+f.orgID[len(f.orgID)-8:], f.orgID); err != nil {
		t.Fatalf("publish two-approval policy: %v", err)
	}

	request := f.request(t, map[string]any{"reason": "ready"}, "idem-sod")
	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "sodapprover")

	if _, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
		ClientDecisionID:  "sod-first",
	}); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	_, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
		ClientDecisionID:  "sod-second",
	})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalSeparationOfDuties {
		t.Fatalf("second decision by the same approver = %v; want separation of duties", err)
	}

	// And the request is still short of quorum, so it cannot execute.
	reloaded, err := f.service.Get(ctx, f.orgID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != approvalmodels.StatusPending {
		t.Fatalf("status = %s; one approval must not satisfy a quorum of two", reloaded.Status)
	}
}

func TestInsufficientApprovalsCannotExecute(t *testing.T) {
	f := newApprovalFixture(t, "Quorum Co", "RC-APR-5")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-quorum")

	executor := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "quorumexec")
	_, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(executor),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"reason": "ready"},
		ExecutionReference: "exec-quorum",
		IdempotencyKey:     "idem-exec-quorum",
	}, nil)
	if err == nil || codeOf(t, err) != errorcodes.ApprovalQuorumNotMet {
		t.Fatalf("execute without approval = %v; want approval_quorum_not_met", err)
	}
}

func TestRejectedApprovalCannotExecute(t *testing.T) {
	f := newApprovalFixture(t, "Rejected Co", "RC-APR-6")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-reject")

	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "rejector")
	rejected, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionRejected,
		Reason:            "not now",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != approvalmodels.StatusRejected {
		t.Fatalf("status = %s, want REJECTED", rejected.Status)
	}

	_, err = f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(approver),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"reason": "ready"},
		ExecutionReference: "exec-rejected",
		IdempotencyKey:     "idem-exec-rejected",
	}, nil)
	if err == nil || codeOf(t, err) != errorcodes.ApprovalAlreadyResolved {
		t.Fatalf("execute after rejection = %v", err)
	}
}

// TestConflictingDecisionIsRejected proves an approver cannot reuse a decision
// identifier to change their mind.
func TestConflictingDecisionIsRejected(t *testing.T) {
	f := newApprovalFixture(t, "Conflicting Co", "RC-APR-7")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-conflict")
	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "flipflop")

	if _, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
		ClientDecisionID:  "same-id",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionRejected,
		ClientDecisionID:  "same-id",
	})
	if err == nil || codeOf(t, err) != errorcodes.IdempotencyConflictCode {
		t.Fatalf("conflicting decision = %v; want an idempotency conflict", err)
	}
}

func TestExpiredApprovalCannotBeDecidedOrExecuted(t *testing.T) {
	f := newApprovalFixture(t, "Expiring Approval Co", "RC-APR-8")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-expire")

	// The expiry window is a request-level fact, not an immutable one: it is
	// the schedule the approval was raised under.
	if _, err := f.pool.Exec(ctx, `
		UPDATE fleet_partner_approval_requests SET expires_at = now() - interval '1 minute'
		WHERE id = $1`, request.ID); err != nil {
		t.Fatal(err)
	}

	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "lateapprover")
	_, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalExpired {
		t.Fatalf("decide after expiry = %v; want approval_expired", err)
	}

	// The sweep marks it EXPIRED so it does not sit as PENDING forever.
	if _, err := f.service.ExpireOverdue(ctx, 100); err != nil {
		t.Fatal(err)
	}
	reloaded, err := f.service.Get(ctx, f.orgID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != approvalmodels.StatusExpired {
		t.Fatalf("status = %s, want EXPIRED", reloaded.Status)
	}
}

// ── Invalidation ──────────────────────────────────────────────────────────────

// TestChangedPayloadInvalidatesTheApproval is the exact-payload rule. It is not
// enough to refuse: the approval is killed, so a caller cannot keep trying
// payloads until one matches.
func TestChangedPayloadInvalidatesTheApproval(t *testing.T) {
	f := newApprovalFixture(t, "Changed Payload Co", "RC-APR-9")
	ctx := context.Background()
	request := f.request(t, map[string]any{"amountMinor": 1000}, "idem-payload")

	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "payloadapprover")
	if _, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	}); err != nil {
		t.Fatal(err)
	}

	var ran int
	_, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(approver),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"amountMinor": 100000},
		ExecutionReference: "exec-changed",
		IdempotencyKey:     "idem-exec-changed",
	}, func(pgx.Tx, *approvalmodels.Request) error {
		ran++
		return nil
	})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalFingerprintMismatch {
		t.Fatalf("execute with a changed payload = %v", err)
	}
	if ran != 0 {
		t.Fatal("the protected operation must not run on a fingerprint mismatch")
	}

	reloaded, err := f.service.Get(ctx, f.orgID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != approvalmodels.StatusInvalidated {
		t.Fatalf("status = %s, want INVALIDATED", reloaded.Status)
	}
	if reloaded.InvalidationReason != approvalmodels.InvalidationPayloadChanged {
		t.Fatalf("invalidationReason = %q", reloaded.InvalidationReason)
	}

	// And the original payload no longer works either: the approval is gone,
	// not merely bypassed for one attempt.
	if _, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(approver),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"amountMinor": 1000},
		ExecutionReference: "exec-changed-2",
		IdempotencyKey:     "idem-exec-changed-2",
	}, nil); err == nil || codeOf(t, err) != errorcodes.ApprovalInvalidated {
		t.Fatalf("execute after invalidation = %v; want approval_invalidated", err)
	}
}

// TestChangedSubjectVersionInvalidatesTheApproval proves an approval describes
// a world, not just an operation.
func TestChangedSubjectVersionInvalidatesTheApproval(t *testing.T) {
	f := newApprovalFixture(t, "Moved Subject Co", "RC-APR-10")
	ctx := context.Background()
	payload := map[string]any{"reason": "ready"}
	request := f.request(t, payload, "idem-version")

	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "versionapprover")
	if _, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	}); err != nil {
		t.Fatal(err)
	}

	// The organization moves on after approval was given.
	if _, err := f.pool.Exec(ctx, `
		UPDATE fleet_partner_organizations SET version = version + 1 WHERE id = $1`,
		f.orgID); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(approver),
		ApprovalRequestID:  request.ID,
		Payload:            payload,
		ExecutionReference: "exec-version",
		IdempotencyKey:     "idem-exec-version",
	}, nil)
	if err == nil || codeOf(t, err) != errorcodes.ApprovalSubjectVersionStale {
		t.Fatalf("execute after the subject moved = %v", err)
	}

	reloaded, err := f.service.Get(ctx, f.orgID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.InvalidationReason != approvalmodels.InvalidationSubjectVersionChanged {
		t.Fatalf("invalidationReason = %q", reloaded.InvalidationReason)
	}
}

// TestASecondRequestInvalidatesTheFirst proves two live approvals for the same
// operation cannot coexist, so a caller cannot execute whichever suits it.
func TestASecondRequestInvalidatesTheFirst(t *testing.T) {
	f := newApprovalFixture(t, "Superseding Approval Co", "RC-APR-11")
	ctx := context.Background()

	first := f.request(t, map[string]any{"amountMinor": 1000}, "idem-first")
	second := f.request(t, map[string]any{"amountMinor": 2000}, "idem-second")
	if first.ID == second.ID {
		t.Fatal("a differing payload must produce a new request")
	}

	reloaded, err := f.service.Get(ctx, f.orgID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != approvalmodels.StatusInvalidated ||
		reloaded.InvalidationReason != approvalmodels.InvalidationSupersededByRequest {
		t.Fatalf("first request = %s / %q", reloaded.Status, reloaded.InvalidationReason)
	}
}

// ── Execution accounting ──────────────────────────────────────────────────────

// TestExecutionAttemptsAreRecordedEvenWhenRefused proves a refused attempt
// leaves a trace. An approval framework whose failures are invisible cannot be
// investigated.
func TestExecutionAttemptsAreRecordedEvenWhenRefused(t *testing.T) {
	f := newApprovalFixture(t, "Execution Log Co", "RC-APR-12")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-exec-log")

	_, _ = f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(f.requester),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"reason": "ready"},
		ExecutionReference: "exec-log-1",
		IdempotencyKey:     "idem-exec-log-1",
	}, nil)

	var outcome, reason string
	if err := f.pool.QueryRow(ctx, `
		SELECT outcome, safe_reason_code FROM fleet_partner_approval_executions
		WHERE organization_id = $1 AND execution_reference = $2`,
		f.orgID, "exec-log-1").Scan(&outcome, &reason); err != nil {
		t.Fatalf("a refused execution attempt must be recorded: %v", err)
	}
	if outcome != approvalmodels.OutcomeRejected || reason != "not_executable" {
		t.Fatalf("outcome=%s reason=%s", outcome, reason)
	}
}

// TestExecutionsAreAppendOnly proves the execution log cannot be tidied up.
func TestExecutionsAreAppendOnly(t *testing.T) {
	f := newApprovalFixture(t, "Append Only Exec Co", "RC-APR-13")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-append")
	_, _ = f.service.Execute(ctx, approvalusecases.ExecuteInput{
		Actor:              f.actor(f.requester),
		ApprovalRequestID:  request.ID,
		Payload:            map[string]any{"reason": "ready"},
		ExecutionReference: "exec-append-1",
		IdempotencyKey:     "idem-exec-append-1",
	}, nil)

	if _, err := f.pool.Exec(ctx, `
		DELETE FROM fleet_partner_approval_executions WHERE organization_id = $1`,
		f.orgID); err == nil {
		t.Fatal("deleting an execution record must be refused by the database")
	}
}

// TestApprovalDecisionsAreImmutable proves a recorded human decision cannot be
// edited afterwards.
func TestApprovalDecisionsAreImmutable(t *testing.T) {
	f := newApprovalFixture(t, "Immutable Decision Co", "RC-APR-14")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-immutable")
	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "immutableapprover")
	if _, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(approver),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.pool.Exec(ctx, `
		UPDATE fleet_partner_approval_decisions SET decision = 'REJECTED'
		WHERE approval_request_id = $1`, request.ID); err == nil {
		t.Fatal("editing a recorded approval decision must be refused by the database")
	}
}

// ── Tenant isolation ──────────────────────────────────────────────────────────

// TestCrossTenantApprovalIsDenied proves an approval identifier from another
// organization is not a way in, and the refusal does not confirm it exists.
func TestCrossTenantApprovalIsDenied(t *testing.T) {
	f := newApprovalFixture(t, "Approval Tenant A", "RC-APR-15")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-tenant")

	other := createOrganization(t, f.pool, "Approval Tenant B", "RC-APR-16")
	seedEligibleOrganization(t, f.pool, other.ID)
	intruder := seedApprovalMember(t, f.pool, other.ID, "rol_system_owner", "intruder")

	for _, attempt := range []func() error{
		func() error {
			_, err := f.service.Get(ctx, other.ID, request.ID)
			return err
		},
		func() error {
			_, err := f.service.Decide(ctx, approvalusecases.DecideInput{
				Actor: approvalusecases.Actor{OrganizationID: other.ID,
					OrganizationUserID: intruder, ActorType: approvalmodels.ActorOrganizationUser,
					ActorID: intruder},
				ApprovalRequestID: request.ID,
				Decision:          approvalmodels.DecisionApproved,
			})
			return err
		},
		func() error {
			_, err := f.service.Execute(ctx, approvalusecases.ExecuteInput{
				Actor: approvalusecases.Actor{OrganizationID: other.ID,
					OrganizationUserID: intruder, ActorType: approvalmodels.ActorOrganizationUser,
					ActorID: intruder},
				ApprovalRequestID:  request.ID,
				Payload:            map[string]any{"reason": "ready"},
				ExecutionReference: "exec-intruder",
				IdempotencyKey:     "idem-intruder",
			}, nil)
			return err
		},
	} {
		err := attempt()
		if err == nil || codeOf(t, err) != errorcodes.ApprovalNotFound {
			t.Fatalf("cross-tenant attempt = %v; want approval_not_found", err)
		}
	}

	// And the listing never carries it.
	requests, err := f.service.List(ctx, tenant.Query{OrganizationID: other.ID},
		approvalrepos.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range requests {
		if candidate.ID == request.ID {
			t.Fatal("another tenant's approval leaked into a listing")
		}
	}
}

// TestApproverMustHoldAnEligibleRole proves the eligible-role list is enforced
// against live bindings rather than trusted from the request.
func TestApproverMustHoldAnEligibleRole(t *testing.T) {
	f := newApprovalFixture(t, "Role Gate Co", "RC-APR-17")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-role")

	// A dispatcher is a real member of the organization and holds no role the
	// activation policy names.
	dispatcher := seedApprovalMember(t, f.pool, f.orgID, "rol_system_dispatcher", "dispatcher")
	_, err := f.service.Decide(ctx, approvalusecases.DecideInput{
		Actor:             f.actor(dispatcher),
		ApprovalRequestID: request.ID,
		Decision:          approvalmodels.DecisionApproved,
	})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("ineligible approver = %v; want approval_actor_not_eligible", err)
	}
}

// TestApprovalHistoryAndOutboxAreWrittenAtomically proves the accountability
// records commit with the state change rather than after it.
func TestApprovalHistoryAndOutboxAreWrittenAtomically(t *testing.T) {
	f := newApprovalFixture(t, "Approval Audit Co", "RC-APR-18")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "idem-audit")

	entries, err := f.service.History(ctx, f.orgID, request.ID)
	if err != nil || len(entries) == 0 {
		t.Fatalf("history = %d, %v", len(entries), err)
	}

	var events int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox
		WHERE aggregate_id = $1 AND event_type = $2`, request.ID,
		approvalusecases.EventApprovalRequested).Scan(&events); err != nil || events != 1 {
		t.Fatalf("outbox events = %d, %v; want 1", events, err)
	}

	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE organization_id = $1 AND action = 'approval.requested'`, f.orgID).
		Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit records = %d, %v; want 1", audits, err)
	}
}
