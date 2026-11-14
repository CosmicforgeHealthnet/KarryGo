package integration_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"

	approvalmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// FLEET-9.12 / 9.13: organization approval policies and approval delegations,
// against real PostgreSQL through the real usecases (and, at the end, the
// real HTTP handlers). The existing approval engine (9.10/9.11) is exercised
// through the paths these changes touch: quorum, self-approval, separation of
// duties and amount thresholds.

func strPtr(value string) *string { return &value }
func intPtr(value int) *int       { return &value }
func boolPtr(value bool) *bool    { return &value }
func i64Ptr(value int64) *int64   { return &value }

func httpStatusOf(t *testing.T, err error) int {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an application error", err)
	}
	return appErr.Status
}

// activationPolicy publishes an organization policy for company activation.
func (f *approvalFixture) activationPolicy(t *testing.T, manager string, code string,
	steps []approvalmodels.StepSpec, mutate func(*approvalusecases.PolicyInput)) *approvalmodels.Policy {
	t.Helper()
	input := approvalusecases.PolicyInput{
		Actor: f.actor(manager), Code: code, Name: strPtr("Activation " + code),
		ResourceType: approvalrepos.ResourceOrganization, Action: "ACTIVATE",
		Steps: steps, IdempotencyKey: "policy-" + code + "-" + f.orgID,
	}
	if mutate != nil {
		mutate(&input)
	}
	result, err := f.service.CreatePolicy(context.Background(), input)
	if err != nil {
		t.Fatalf("create policy %s: %v", code, err)
	}
	return result.Policy
}

func (f *approvalFixture) decide(memberID string, requestID string, client string) (*approvalmodels.Request, error) {
	return f.service.Decide(context.Background(), approvalusecases.DecideInput{
		Actor: f.actor(memberID), ApprovalRequestID: requestID,
		Decision: approvalmodels.DecisionApproved, ClientDecisionID: client,
	})
}

// stranger creates a second organization with an owner in the same database.
// testPool can be taken only once per test, so a second fixture is not an
// option.
func (f *approvalFixture) stranger(t *testing.T, name string, registration string) (string, string) {
	t.Helper()
	org := createOrganization(t, f.pool, name, registration)
	return org.ID, seedApprovalMember(t, f.pool, org.ID, "rol_system_owner", "stranger")
}

// TestApprovalPolicyLifecycleIsVersionedAndAudited covers create, idempotent
// replay, validation, version conflict, archive and the append-only history.
func TestApprovalPolicyLifecycleIsVersionedAndAudited(t *testing.T) {
	f := newApprovalFixture(t, "Policy CRUD Co", "RC-WS1-POL-1")
	ctx := context.Background()
	manager := f.requester // the owner holds approval_policies.manage

	base := approvalusecases.PolicyInput{
		Actor: f.actor(manager), Code: "big-payouts", Name: strPtr("Big payouts"),
		ResourceType: "provider_allocation", Action: "wallet.provider_allocations.approve",
		ThresholdAmountMinor: i64Ptr(5_000_000), ThresholdCurrency: strPtr("NGN"),
		Steps: []approvalmodels.StepSpec{
			{Quorum: 1, EligibleRoleIDs: []string{"rol_system_finance"}},
			{Quorum: 1, EligibleRoleIDs: []string{"rol_system_owner"}},
		},
		IdempotencyKey: "pol-create-1",
	}
	created, err := f.service.CreatePolicy(ctx, base)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	policy := created.Policy
	if policy.Code != "BIG-PAYOUTS" || policy.Version != 1 || policy.Origin != approvalmodels.OriginOrganization ||
		len(policy.Steps) != 2 || !policy.SeparationOfDuties || policy.RequesterMayApprove {
		t.Fatalf("created policy: %+v", policy)
	}

	// Retried with the same key: replayed, not duplicated.
	replay, err := f.service.CreatePolicy(ctx, base)
	if err != nil || !replay.Replayed || replay.Policy.ID != policy.ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	// Same key, different policy: refused.
	other := base
	other.Code = "OTHER"
	if _, err := f.service.CreatePolicy(ctx, other); err == nil || codeOf(t, err) != errorcodes.IdempotencyConflictCode {
		t.Fatalf("reused key = %v", err)
	}
	// Same code under a new key: the code is taken.
	dup := base
	dup.IdempotencyKey = "pol-create-dup"
	if _, err := f.service.CreatePolicy(ctx, dup); err == nil || httpStatusOf(t, err) != http.StatusConflict {
		t.Fatalf("duplicate code = %v", err)
	}

	// Validation: every rule reported, nothing written.
	for name, mutate := range map[string]func(*approvalusecases.PolicyInput){
		"unknown role": func(in *approvalusecases.PolicyInput) {
			in.Steps = []approvalmodels.StepSpec{{Quorum: 1, EligibleRoleIDs: []string{"rol_nope"}}}
		},
		"zero quorum":        func(in *approvalusecases.PolicyInput) { in.Steps = []approvalmodels.StepSpec{{Quorum: 0}} },
		"no steps":           func(in *approvalusecases.PolicyInput) { in.Steps = nil },
		"threshold, no ccy":  func(in *approvalusecases.PolicyInput) { in.ThresholdCurrency = nil },
		"lowercase currency": func(in *approvalusecases.PolicyInput) { in.ThresholdCurrency = strPtr("ngn") },
		"negative threshold": func(in *approvalusecases.PolicyInput) { in.ThresholdAmountMinor = i64Ptr(-1) },
		"expiry too short":   func(in *approvalusecases.PolicyInput) { in.ExpirySeconds = intPtr(10) },
		"unknown branch":     func(in *approvalusecases.PolicyInput) { in.BranchID = strPtr("brn_missing") },
		"six steps":          func(in *approvalusecases.PolicyInput) { in.Steps = make([]approvalmodels.StepSpec, 6) },
		"missing name":       func(in *approvalusecases.PolicyInput) { in.Name = nil },
		"missing action":     func(in *approvalusecases.PolicyInput) { in.Action = "" },
	} {
		input := base
		input.Code = "INVALID"
		input.IdempotencyKey = "pol-invalid-" + name
		mutate(&input)
		if _, err := f.service.CreatePolicy(ctx, input); err == nil || httpStatusOf(t, err) != http.StatusUnprocessableEntity {
			t.Fatalf("%s: err = %v; want 422", name, err)
		}
	}

	// Update requires the current version.
	update := approvalusecases.PolicyInput{Actor: f.actor(manager), PolicyID: policy.ID,
		Name: strPtr("Very big payouts"), ExpectedVersion: 1, Reason: "tighten"}
	updated, err := f.service.UpdatePolicy(ctx, update)
	if err != nil || updated.Version != 2 || updated.Name != "Very big payouts" {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if _, err := f.service.UpdatePolicy(ctx, update); err == nil || httpStatusOf(t, err) != http.StatusConflict {
		t.Fatalf("stale update = %v; want 409", err)
	}
	update.ExpectedVersion = 0
	if _, err := f.service.UpdatePolicy(ctx, update); err == nil || httpStatusOf(t, err) != http.StatusBadRequest {
		t.Fatalf("update without version = %v; want 400", err)
	}

	// Concurrent edits at the same version: exactly one wins.
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := f.service.UpdatePolicy(ctx, approvalusecases.PolicyInput{Actor: f.actor(manager),
				PolicyID: policy.ID, ExpirySeconds: intPtr(3600 + index), ExpectedVersion: 2})
			results <- err
		}(index)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if httpStatusOf(t, err) != http.StatusConflict {
			t.Fatalf("concurrent edit: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent edits won = %d; want exactly 1", wins)
	}

	archived, err := f.service.ArchivePolicy(ctx, approvalusecases.PolicyInput{Actor: f.actor(manager),
		PolicyID: policy.ID, ExpectedVersion: 3, Reason: "retired"})
	if err != nil || archived.Status != approvalmodels.PolicyStatusArchived || archived.ArchivedAt == nil {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
	if _, err := f.service.UpdatePolicy(ctx, approvalusecases.PolicyInput{Actor: f.actor(manager),
		PolicyID: policy.ID, Name: strPtr("again"), ExpectedVersion: 4}); err == nil ||
		codeOf(t, err) != errorcodes.ApprovalPolicyNotEditable {
		t.Fatalf("edit archived = %v", err)
	}

	versions, err := f.service.PolicyVersions(ctx, f.orgID, nil, policy.ID)
	if err != nil || len(versions) != 4 || versions[0].Version != 4 || versions[0].Status != approvalmodels.PolicyStatusArchived {
		t.Fatalf("versions = %+v, %v", versions, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_policy_versions SET change_reason = 'x' WHERE policy_id = $1`, policy.ID); err == nil {
		t.Fatal("policy versions must be append-only")
	}

	var audits, events int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1
		AND resource_id = $2 AND action IN ('approval_policy.created','approval_policy.updated','approval_policy.archived')`,
		f.orgID, policy.ID).Scan(&audits); err != nil || audits != 4 {
		t.Fatalf("policy audit rows = %d, %v; want 4", audits, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id = $1`,
		policy.ID).Scan(&events); err != nil || events != 4 {
		t.Fatalf("policy outbox rows = %d, %v; want 4", events, err)
	}

	// Another organization cannot see or touch it.
	strangerOrg, strangerOwner := f.stranger(t, "Policy Stranger Co", "RC-WS1-POL-2")
	if _, err := f.service.GetPolicy(ctx, strangerOrg, nil, policy.ID); err == nil || httpStatusOf(t, err) != http.StatusNotFound {
		t.Fatalf("cross-tenant read = %v; want 404", err)
	}
	strangerActor := f.actor(strangerOwner)
	strangerActor.OrganizationID = strangerOrg
	if _, err := f.service.ArchivePolicy(ctx, approvalusecases.PolicyInput{Actor: strangerActor,
		PolicyID: policy.ID, ExpectedVersion: 4}); err == nil || httpStatusOf(t, err) != http.StatusNotFound {
		t.Fatalf("cross-tenant archive = %v; want 404", err)
	}
}

// TestTemplatePolicyIsNotEditableThroughTheApprovalPolicyAPI proves the rows
// derived from the platform eligibility policy stay governed by it.
func TestTemplatePolicyIsNotEditableThroughTheApprovalPolicyAPI(t *testing.T) {
	f := newApprovalFixture(t, "Template Co", "RC-WS1-POL-3")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "ws1-template")
	template, err := f.service.GetPolicy(ctx, f.orgID, nil, request.PolicyID)
	if err != nil || template.Origin != approvalmodels.OriginTemplate {
		t.Fatalf("template = %+v, %v", template, err)
	}
	_, err = f.service.UpdatePolicy(ctx, approvalusecases.PolicyInput{Actor: f.actor(f.requester),
		PolicyID: template.ID, Name: strPtr("hijack"), ExpectedVersion: template.Version})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalPolicyNotEditable {
		t.Fatalf("template edit = %v; want approval_policy_not_editable", err)
	}
}

// TestOrganizationPolicyGovernsSequentialSteps proves a two-step policy is
// frozen onto the request and walked in order, each step with its own roles.
func TestOrganizationPolicyGovernsSequentialSteps(t *testing.T) {
	f := newApprovalFixture(t, "Steps Co", "RC-WS1-POL-4")
	ctx := context.Background()
	policy := f.activationPolicy(t, f.requester, "TWO-STEP", []approvalmodels.StepSpec{
		{Quorum: 1, EligibleRoleIDs: []string{"rol_system_admin"}},
		{Quorum: 1, EligibleRoleIDs: []string{"rol_system_owner"}},
	}, nil)

	request := f.request(t, map[string]any{"reason": "ready"}, "ws1-steps")
	if request.PolicyID != policy.ID || request.PolicyVersion != 1 || request.MinimumApprovals != 2 ||
		request.SeparationOfDuties != true || request.SelfApprovalAllowed {
		t.Fatalf("request did not pin the organization policy: %+v", request)
	}
	steps, err := f.service.Steps(ctx, f.orgID, request.ID)
	if err != nil || len(steps) != 2 || steps[1].EligibleRoleIDs[0] != "rol_system_owner" {
		t.Fatalf("steps = %+v, %v", steps, err)
	}

	owner2 := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner2")
	admin := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "admin1")
	admin2 := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "admin2")

	// Step 1 names admins only: an owner cannot decide it.
	if _, err := f.decide(owner2, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("owner on step 1 = %v", err)
	}
	after, err := f.decide(admin, request.ID, "")
	if err != nil || after.Status != approvalmodels.StatusPending || after.CurrentStep != 2 {
		t.Fatalf("after step 1 = %+v, %v", after, err)
	}
	// Step 2 names owners only.
	if _, err := f.decide(admin2, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("admin on step 2 = %v", err)
	}
	approved, err := f.decide(owner2, request.ID, "")
	if err != nil || approved.Status != approvalmodels.StatusApproved {
		t.Fatalf("after step 2 = %+v, %v", approved, err)
	}
	history, _ := f.service.History(ctx, f.orgID, request.ID)
	stepCompleted := 0
	for _, entry := range history {
		if entry.ReasonCode == "step_completed" {
			stepCompleted++
		}
	}
	if stepCompleted != 1 {
		t.Fatalf("step_completed history entries = %d; want 1", stepCompleted)
	}
}

// TestPolicyEditDoesNotWeakenInFlightRequests proves the rules are frozen
// onto the request: relaxing the policy only governs requests raised later.
func TestPolicyEditDoesNotWeakenInFlightRequests(t *testing.T) {
	f := newApprovalFixture(t, "Pinned Co", "RC-WS1-POL-5")
	ctx := context.Background()
	policy := f.activationPolicy(t, f.requester, "PINNED", []approvalmodels.StepSpec{{Quorum: 2}}, nil)

	inFlight := f.request(t, map[string]any{"reason": "first"}, "ws1-pin-1")
	if inFlight.MinimumApprovals != 2 || inFlight.PolicyVersion != 1 {
		t.Fatalf("in-flight request: %+v", inFlight)
	}
	if _, err := f.service.UpdatePolicy(ctx, approvalusecases.PolicyInput{Actor: f.actor(f.requester),
		PolicyID: policy.ID, Steps: []approvalmodels.StepSpec{{Quorum: 1}}, ExpectedVersion: 1,
		Reason: "relax"}); err != nil {
		t.Fatalf("relax policy: %v", err)
	}

	first := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "pin1")
	second := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "pin2")
	still, err := f.decide(first, inFlight.ID, "")
	if err != nil || still.Status != approvalmodels.StatusPending {
		t.Fatalf("one approval completed a request frozen at quorum 2: %+v, %v", still, err)
	}
	done, err := f.decide(second, inFlight.ID, "")
	if err != nil || done.Status != approvalmodels.StatusApproved {
		t.Fatalf("second approval = %+v, %v", done, err)
	}

	next := f.request(t, map[string]any{"reason": "second"}, "ws1-pin-2")
	if next.PolicyVersion != 2 || next.MinimumApprovals != 1 {
		t.Fatalf("new request did not take version 2: %+v", next)
	}
}

// TestOrganizationPolicyCannotWeakenThePlatformFloor proves an organization
// policy cannot allow self-approval, drop separation of duties or extend the
// approval window beyond the platform eligibility policy.
func TestOrganizationPolicyCannotWeakenThePlatformFloor(t *testing.T) {
	f := newApprovalFixture(t, "Floor Co", "RC-WS1-POL-6")
	f.activationPolicy(t, f.requester, "LAX", []approvalmodels.StepSpec{{Quorum: 1}},
		func(in *approvalusecases.PolicyInput) {
			in.RequesterMayApprove = boolPtr(true)
			in.SeparationOfDuties = boolPtr(false)
			in.ExpirySeconds = intPtr(30 * 24 * 60 * 60)
		})
	request := f.request(t, map[string]any{"reason": "ready"}, "ws1-floor")
	if request.SelfApprovalAllowed || !request.SeparationOfDuties ||
		time.Until(request.ExpiresAt) > 25*time.Hour {
		t.Fatalf("platform floor weakened: self=%v sod=%v ttl=%v", request.SelfApprovalAllowed,
			request.SeparationOfDuties, time.Until(request.ExpiresAt))
	}
	if _, err := f.decide(f.requester, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalSelfDecisionForbidden {
		t.Fatalf("self approval under a lax policy = %v", err)
	}
}

// TestAmountThresholdSelectsTheGoverningPolicy proves thresholds compare
// integer minor units in the same currency, read from the fingerprinted
// payload.
func TestAmountThresholdSelectsTheGoverningPolicy(t *testing.T) {
	f := newApprovalFixture(t, "Threshold Co", "RC-WS1-POL-7")
	small := f.activationPolicy(t, f.requester, "SMALL", []approvalmodels.StepSpec{{Quorum: 1}}, nil)
	big := f.activationPolicy(t, f.requester, "BIG", []approvalmodels.StepSpec{{Quorum: 2}},
		func(in *approvalusecases.PolicyInput) {
			in.ThresholdAmountMinor = i64Ptr(1_000_000)
			in.ThresholdCurrency = strPtr("NGN")
		})

	for _, tc := range []struct {
		name    string
		payload map[string]any
		policy  string
		minimum int
	}{
		{"below threshold", map[string]any{"amountMinor": 999_999, "currency": "NGN"}, small.ID, 1},
		{"at threshold", map[string]any{"amountMinor": 1_000_000, "currency": "NGN"}, big.ID, 2},
		{"above threshold (JSON number)", map[string]any{"amountMinor": float64(5_000_000), "currency": "NGN"}, big.ID, 2},
		{"other currency", map[string]any{"amountMinor": 5_000_000, "currency": "USD"}, small.ID, 1},
		{"no amount", map[string]any{"reason": "ready"}, small.ID, 1},
	} {
		request := f.request(t, tc.payload, "ws1-threshold-"+tc.name)
		if request.PolicyID != tc.policy || request.MinimumApprovals != tc.minimum {
			t.Fatalf("%s: policy=%s minimum=%d; want %s/%d", tc.name, request.PolicyID,
				request.MinimumApprovals, tc.policy, tc.minimum)
		}
		if amount, ok := tc.payload["amountMinor"]; ok && tc.payload["currency"] == "NGN" {
			if request.AmountMinor == nil || float64(*request.AmountMinor) != toFloat(amount) || request.AmountCurrency != "NGN" {
				t.Fatalf("%s: recorded amount %v %s", tc.name, request.AmountMinor, request.AmountCurrency)
			}
		}
	}

	for name, payload := range map[string]map[string]any{
		"fractional minor units":  {"amountMinor": 10.5, "currency": "NGN"},
		"negative amount":         {"amountMinor": -1, "currency": "NGN"},
		"amount without currency": {"amountMinor": 2_000_000},
	} {
		_, err := f.service.Request(context.Background(), approvalusecases.RequestInput{
			Actor: f.actor(f.requester), ResourceType: approvalrepos.ResourceOrganization,
			ResourceID: f.orgID, Action: "ACTIVATE", OperationType: eligibilitymodels.OperationCompanyActivation,
			Payload: payload, IdempotencyKey: "ws1-bad-amount-" + name,
		})
		if err == nil || httpStatusOf(t, err) != http.StatusUnprocessableEntity {
			t.Fatalf("%s: err = %v; want 422", name, err)
		}
	}
}

func toFloat(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case float64:
		return typed
	}
	return -1
}

// TestQuorumRaceCompletesExactlyOnce proves concurrent approvers on a quorum
// of two produce exactly two counted decisions and one APPROVED transition.
func TestQuorumRaceCompletesExactlyOnce(t *testing.T) {
	f := newApprovalFixture(t, "Race Co", "RC-WS1-POL-8")
	ctx := context.Background()
	f.activationPolicy(t, f.requester, "QUORUM-2", []approvalmodels.StepSpec{{Quorum: 2}}, nil)
	request := f.request(t, map[string]any{"reason": "race"}, "ws1-race")

	approvers := []string{
		seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "race1"),
		seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "race2"),
		seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "race3"),
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(approvers))
	for _, approver := range approvers {
		wg.Add(1)
		go func(approver string) {
			defer wg.Done()
			_, err := f.decide(approver, request.ID, "")
			errs <- err
		}(approver)
	}
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		if codeOf(t, err) != errorcodes.ApprovalAlreadyResolved {
			t.Fatalf("losing approver = %v; want approval_already_resolved", err)
		}
	}
	if succeeded != 2 {
		t.Fatalf("successful decisions = %d; want exactly 2", succeeded)
	}
	reloaded, _ := f.service.Get(ctx, f.orgID, request.ID)
	decisions, _ := f.service.Decisions(ctx, f.orgID, request.ID)
	if reloaded.Status != approvalmodels.StatusApproved || len(decisions) != 2 {
		t.Fatalf("status=%s decisions=%d", reloaded.Status, len(decisions))
	}
}

// TestApprovedRequestTakesNoFurtherDecisions pins the 9.10/9.11 repair found
// while adding steps: an APPROVED request used to accept further decisions,
// so a late rejection could flip a completed approval to REJECTED.
func TestApprovedRequestTakesNoFurtherDecisions(t *testing.T) {
	f := newApprovalFixture(t, "Closed Co", "RC-WS1-POL-9")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "ws1-closed")
	approver := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "closed1")
	objector := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "closed2")
	if approved, err := f.decide(approver, request.ID, ""); err != nil || approved.Status != approvalmodels.StatusApproved {
		t.Fatalf("approve = %+v, %v", approved, err)
	}
	_, err := f.service.Decide(ctx, approvalusecases.DecideInput{Actor: f.actor(objector),
		ApprovalRequestID: request.ID, Decision: approvalmodels.DecisionRejected, Reason: "late"})
	if err == nil || codeOf(t, err) != errorcodes.ApprovalAlreadyResolved {
		t.Fatalf("late rejection = %v; want approval_already_resolved", err)
	}
	reloaded, _ := f.service.Get(ctx, f.orgID, request.ID)
	decisions, _ := f.service.Decisions(ctx, f.orgID, request.ID)
	if reloaded.Status != approvalmodels.StatusApproved || len(decisions) != 1 {
		t.Fatalf("status=%s decisions=%d; the approval must stand", reloaded.Status, len(decisions))
	}
}

// ── Delegations ──────────────────────────────────────────────────────────────

type delegationFixture struct {
	*approvalFixture
	admin     string // delegator: holds an eligible role and the decide permission
	delegate  string // compliance: holds decide, but no eligible role
	expiresAt time.Time
}

func newDelegationFixture(t *testing.T, name string, registration string) *delegationFixture {
	t.Helper()
	f := newApprovalFixture(t, name, registration)
	return &delegationFixture{
		approvalFixture: f,
		admin:           seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "delegator"),
		delegate:        seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "delegate"),
		expiresAt:       time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second),
	}
}

func (f *delegationFixture) grant(t *testing.T, from string, to string, key string,
	mutate func(*approvalusecases.DelegationInput)) (*approvalusecases.DelegationResult, error) {
	t.Helper()
	input := approvalusecases.DelegationInput{Actor: f.actor(from), DelegateID: to,
		ExpiresAt: f.expiresAt, Reason: "annual leave", IdempotencyKey: key}
	if mutate != nil {
		mutate(&input)
	}
	return f.service.CreateDelegation(context.Background(), input)
}

// TestDelegateDecidesOnTheDelegatorsAuthority covers the happy path and that
// the decision records whose authority it exercised.
func TestDelegateDecidesOnTheDelegatorsAuthority(t *testing.T) {
	f := newDelegationFixture(t, "Delegation Co", "RC-WS1-DEL-1")
	ctx := context.Background()
	request := f.request(t, map[string]any{"reason": "ready"}, "ws1-del-1")

	// Without a delegation the compliance officer holds no eligible role.
	if _, err := f.decide(f.delegate, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("undelegated decision = %v", err)
	}

	created, err := f.grant(t, f.admin, f.delegate, "ws1-del-key-1", func(in *approvalusecases.DelegationInput) {
		in.ResourceType = approvalrepos.ResourceOrganization
		in.Action = "ACTIVATE"
	})
	if err != nil || created.Replayed || created.Delegation.Status(time.Now()) != approvalmodels.DelegationActive {
		t.Fatalf("create delegation = %+v, %v", created, err)
	}
	replay, err := f.grant(t, f.admin, f.delegate, "ws1-del-key-1", func(in *approvalusecases.DelegationInput) {
		in.ResourceType = approvalrepos.ResourceOrganization
		in.Action = "ACTIVATE"
	})
	if err != nil || !replay.Replayed || replay.Delegation.ID != created.Delegation.ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}

	approved, err := f.decide(f.delegate, request.ID, "")
	if err != nil || approved.Status != approvalmodels.StatusApproved {
		t.Fatalf("delegated decision = %+v, %v", approved, err)
	}
	decisions, _ := f.service.Decisions(ctx, f.orgID, request.ID)
	if len(decisions) != 1 || decisions[0].ApproverID != f.delegate || decisions[0].OnBehalfOf == nil ||
		*decisions[0].OnBehalfOf != f.admin || decisions[0].DelegationID == nil ||
		*decisions[0].DelegationID != created.Delegation.ID {
		t.Fatalf("decision did not record the delegation: %+v", decisions)
	}

	var audits, events int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1
		AND action = 'approval.delegation_created' AND resource_id = $2`, f.orgID, created.Delegation.ID).Scan(&audits)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id = $1 AND event_type = $2`,
		created.Delegation.ID, approvalusecases.EventApprovalDelegationCreated).Scan(&events)
	if audits != 1 || events != 1 {
		t.Fatalf("delegation audit=%d outbox=%d; want 1/1", audits, events)
	}

	// A delegation scoped to another action does not reach this one.
	other := f.request(t, map[string]any{"reason": "second"}, "ws1-del-1b")
	_ = other
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_delegations SET action = 'SOMETHING_ELSE' WHERE id = $1`,
		created.Delegation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.decide(f.delegate, other.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("out-of-scope delegation = %v", err)
	}
}

// TestDelegationCreationRules covers every refusal a delegation can meet.
func TestDelegationCreationRules(t *testing.T) {
	f := newDelegationFixture(t, "Delegation Rules Co", "RC-WS1-DEL-2")
	viewer := seedApprovalMember(t, f.pool, f.orgID, "rol_system_view_only", "viewer")
	dispatcher := seedApprovalMember(t, f.pool, f.orgID, "rol_system_dispatcher", "dispatcher")
	_, strangerOwner := f.stranger(t, "Delegation Stranger Co", "RC-WS1-DEL-3")

	for _, tc := range []struct {
		name   string
		from   string
		to     string
		mutate func(*approvalusecases.DelegationInput)
		status int
		code   apperrors.Code
	}{
		{"self", f.admin, f.admin, nil, http.StatusUnprocessableEntity, ""},
		{"delegate lacks decide", f.admin, viewer, nil, http.StatusUnprocessableEntity, errorcodes.ApprovalDelegationInvalid},
		{"delegator lacks decide", dispatcher, f.delegate, nil, http.StatusForbidden, errorcodes.PermissionEscalationForbidden},
		{"delegate in another organization", f.admin, strangerOwner, nil, http.StatusNotFound, ""},
		{"longer than 90 days", f.admin, f.delegate, func(in *approvalusecases.DelegationInput) {
			in.ExpiresAt = time.Now().Add(91 * 24 * time.Hour)
		}, http.StatusUnprocessableEntity, ""},
		{"backdated", f.admin, f.delegate, func(in *approvalusecases.DelegationInput) {
			past := time.Now().Add(-2 * time.Hour)
			in.EffectiveFrom = &past
		}, http.StatusUnprocessableEntity, ""},
		{"already expired", f.admin, f.delegate, func(in *approvalusecases.DelegationInput) {
			in.ExpiresAt = time.Now().Add(-time.Minute)
		}, http.StatusUnprocessableEntity, ""},
		{"unknown branch", f.admin, f.delegate, func(in *approvalusecases.DelegationInput) {
			in.BranchID = "brn_missing"
		}, http.StatusUnprocessableEntity, ""},
	} {
		_, err := f.grant(t, tc.from, tc.to, "ws1-rule-"+tc.name, tc.mutate)
		if err == nil || httpStatusOf(t, err) != tc.status || (tc.code != "" && codeOf(t, err) != tc.code) {
			t.Fatalf("%s: err = %v; want %d %s", tc.name, err, tc.status, tc.code)
		}
	}

	if _, err := f.grant(t, f.admin, f.delegate, "ws1-rule-ok", nil); err != nil {
		t.Fatalf("valid delegation: %v", err)
	}
	// An overlapping delegation of the same scope to the same person.
	if _, err := f.grant(t, f.admin, f.delegate, "ws1-rule-overlap", nil); err == nil ||
		codeOf(t, err) != errorcodes.ApprovalDelegationOverlap {
		t.Fatalf("overlap = %v", err)
	}
	// The same key for a different delegation.
	if _, err := f.grant(t, f.admin, f.delegate, "ws1-rule-ok", func(in *approvalusecases.DelegationInput) {
		in.Action = "OTHER"
	}); err == nil || codeOf(t, err) != errorcodes.IdempotencyConflictCode {
		t.Fatalf("reused key = %v", err)
	}
}

// TestDelegationKeepsSeparationOfDuties proves a delegation never lets one
// person's authority count twice, and never lets the requester approve.
func TestDelegationKeepsSeparationOfDuties(t *testing.T) {
	f := newDelegationFixture(t, "Delegation SoD Co", "RC-WS1-DEL-4")
	ctx := context.Background()
	f.activationPolicy(t, f.requester, "QUORUM-2", []approvalmodels.StepSpec{{Quorum: 2}}, nil)
	if _, err := f.grant(t, f.admin, f.delegate, "ws1-sod-1", nil); err != nil {
		t.Fatal(err)
	}

	// Delegate decides on the admin's authority; the admin then cannot add a
	// second decision of their own.
	request := f.request(t, map[string]any{"reason": "sod-1"}, "ws1-sod-r1")
	if _, err := f.decide(f.delegate, request.ID, ""); err != nil {
		t.Fatalf("delegated decision: %v", err)
	}
	if _, err := f.decide(f.admin, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalSeparationOfDuties {
		t.Fatalf("delegator after delegate = %v; want separation of duties", err)
	}
	reloaded, _ := f.service.Get(ctx, f.orgID, request.ID)
	if reloaded.Status != approvalmodels.StatusPending {
		t.Fatalf("one person's authority completed a quorum of two: %s", reloaded.Status)
	}

	// The admin decides first; their authority is spent and the delegate has
	// nothing left to exercise.
	second := f.request(t, map[string]any{"reason": "sod-2"}, "ws1-sod-r2")
	if _, err := f.decide(f.admin, second.ID, ""); err != nil {
		t.Fatalf("admin decision: %v", err)
	}
	if _, err := f.decide(f.delegate, second.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("delegate after delegator = %v; want not eligible", err)
	}

	// The requester delegating their own authority cannot approve their own
	// request through someone else.
	if _, err := f.grant(t, f.requester, f.delegate, "ws1-sod-requester", func(in *approvalusecases.DelegationInput) {
		in.Action = "ACTIVATE"
	}); err != nil {
		t.Fatalf("requester delegation: %v", err)
	}
	third := f.request(t, map[string]any{"reason": "sod-3"}, "ws1-sod-r3")
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_delegations SET revoked_at = now(), revoked_by = 'test'
		WHERE delegator_id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.decide(f.delegate, third.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("approval on the requester's own authority = %v; want not eligible", err)
	}

	// A delegate who raised the request cannot approve it via a delegation.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_delegations SET revoked_at = NULL, revoked_by = NULL
		WHERE delegator_id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Request(ctx, approvalusecases.RequestInput{Actor: f.actor(f.delegate),
		ResourceType: approvalrepos.ResourceOrganization, ResourceID: f.orgID, Action: "ACTIVATE",
		OperationType: eligibilitymodels.OperationCompanyActivation,
		Payload:       map[string]any{"reason": "own"}, IdempotencyKey: "ws1-sod-own"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.decide(f.delegate, own.Request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalSelfDecisionForbidden {
		t.Fatalf("requester deciding via delegation = %v; want self-decision forbidden", err)
	}
}

// TestDelegationNeverExceedsTheDelegatorsLiveAuthority covers expiry,
// revocation of the delegator's role, delegator suspension and the absence of
// re-delegation chains.
func TestDelegationNeverExceedsTheDelegatorsLiveAuthority(t *testing.T) {
	f := newDelegationFixture(t, "Delegation Live Co", "RC-WS1-DEL-5")
	ctx := context.Background()
	created, err := f.grant(t, f.admin, f.delegate, "ws1-live-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := f.request(t, map[string]any{"reason": "live"}, "ws1-live-r1")

	refused := func(label string) {
		t.Helper()
		if _, err := f.decide(f.delegate, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
			t.Fatalf("%s: delegated decision = %v; want not eligible", label, err)
		}
	}

	// Expired.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_delegations
		SET effective_from = now() - interval '2 hours', expires_at = now() - interval '1 hour' WHERE id = $1`,
		created.Delegation.ID); err != nil {
		t.Fatal(err)
	}
	refused("expired delegation")
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_approval_delegations
		SET effective_from = now() - interval '1 hour', expires_at = now() + interval '1 day' WHERE id = $1`,
		created.Delegation.ID); err != nil {
		t.Fatal(err)
	}

	// Delegator suspended.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_organization_users SET status = 'SUSPENDED' WHERE id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	refused("suspended delegator")
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_organization_users SET status = 'ACTIVE' WHERE id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}

	// Delegator lost the eligible role.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_role_bindings SET revoked_at = now() WHERE organization_user_id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}
	refused("delegator without a role")
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_role_bindings SET revoked_at = NULL WHERE organization_user_id = $1`, f.admin); err != nil {
		t.Fatal(err)
	}

	// No chain: the compliance officer re-delegates to a colleague. The
	// colleague gets only what the compliance officer holds by their OWN
	// roles, which is no eligible role at all.
	colleague := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "colleague")
	if _, err := f.grant(t, f.delegate, colleague, "ws1-live-chain", nil); err != nil {
		t.Fatalf("chain delegation creation: %v", err)
	}
	if _, err := f.decide(colleague, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("re-delegated authority = %v; want not eligible", err)
	}

	// With everything restored the original delegation works again.
	if approved, err := f.decide(f.delegate, request.ID, ""); err != nil || approved.Status != approvalmodels.StatusApproved {
		t.Fatalf("restored delegation = %+v, %v", approved, err)
	}
}

// TestDelegationRevocation covers who may revoke, version conflicts and the
// terminal state.
func TestDelegationRevocation(t *testing.T) {
	f := newDelegationFixture(t, "Delegation Revoke Co", "RC-WS1-DEL-6")
	ctx := context.Background()
	created, err := f.grant(t, f.admin, f.delegate, "ws1-revoke-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := created.Delegation.ID

	revoke := func(actor string, version int64, mayRevokeAny bool) (*approvalmodels.Delegation, error) {
		return f.service.RevokeDelegation(ctx, approvalusecases.RevokeDelegationInput{Actor: f.actor(actor),
			DelegationID: id, ExpectedVersion: version, Reason: "back from leave", MayRevokeAny: mayRevokeAny})
	}
	if _, err := revoke(f.delegate, 1, false); err == nil || httpStatusOf(t, err) != http.StatusForbidden {
		t.Fatalf("delegate revoking = %v; want 403", err)
	}
	if _, err := revoke(f.admin, 7, false); err == nil || httpStatusOf(t, err) != http.StatusConflict {
		t.Fatalf("stale revoke = %v; want 409", err)
	}
	revoked, err := revoke(f.admin, 1, false)
	if err != nil || revoked.Status(time.Now()) != approvalmodels.DelegationRevoked || revoked.Version != 2 {
		t.Fatalf("revoke = %+v, %v", revoked, err)
	}
	if _, err := revoke(f.admin, 2, false); err == nil || codeOf(t, err) != errorcodes.ApprovalDelegationEnded {
		t.Fatalf("second revoke = %v; want ended", err)
	}

	request := f.request(t, map[string]any{"reason": "after revoke"}, "ws1-revoke-r1")
	if _, err := f.decide(f.delegate, request.ID, ""); err == nil || codeOf(t, err) != errorcodes.ApprovalActorNotEligible {
		t.Fatalf("revoked delegation used = %v", err)
	}

	// A manager may revoke someone else's delegation.
	other, err := f.grant(t, f.admin, f.delegate, "ws1-revoke-2", func(in *approvalusecases.DelegationInput) {
		in.Action = "ACTIVATE"
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RevokeDelegation(ctx, approvalusecases.RevokeDelegationInput{Actor: f.actor(f.requester),
		DelegationID: other.Delegation.ID, ExpectedVersion: 1, MayRevokeAny: true}); err != nil {
		t.Fatalf("manager revoke: %v", err)
	}

	listed, err := f.service.ListDelegations(ctx, f.orgID, nil, approvalrepos.DelegationFilter{Status: "revoked"})
	if err != nil || len(listed) != 2 {
		t.Fatalf("revoked delegations = %d, %v; want 2", len(listed), err)
	}
	strangerOrg, _ := f.stranger(t, "Revoke Stranger Co", "RC-WS1-DEL-7")
	if _, err := f.service.GetDelegation(ctx, strangerOrg, nil, id); err == nil || httpStatusOf(t, err) != http.StatusNotFound {
		t.Fatalf("cross-tenant delegation read = %v; want 404", err)
	}
}

// TestApprovalPolicyAndDelegationRoutes drives the HTTP layer: default deny,
// idempotency, If-Match, cross-tenant 404 and the revoke-any permission.
func TestApprovalPolicyAndDelegationRoutes(t *testing.T) {
	f := newGateHTTPFixture(t, "Policy Route Co", "RC-WS1-HTTP-1")
	admin := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "httpadmin")
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "httpowner")
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "httpcompliance")
	nobody := seedApprovalMember(t, f.pool, f.orgID, "", "httpnobody")
	adminRouter := f.router(t, f.orgID, admin, nil)
	complianceRouter := f.router(t, f.orgID, compliance, nil)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/fleet-partner/v1/approval-policies"},
		{http.MethodPost, "/fleet-partner/v1/approval-policies"},
		{http.MethodGet, "/fleet-partner/v1/approval-delegations"},
		{http.MethodPost, "/fleet-partner/v1/approval-delegations"},
	} {
		if status, _ := gateCall(t, f.router(t, f.orgID, nobody, nil), route.method, route.path,
			map[string]any{}, map[string]string{"Idempotency-Key": "deny"}); status != http.StatusForbidden {
			t.Fatalf("%s %s by a member without permissions = %d; want 403", route.method, route.path, status)
		}
	}
	// Compliance may decide approvals but not manage policy.
	if status, _ := gateCall(t, complianceRouter, http.MethodGet, "/fleet-partner/v1/approval-policies", nil, nil); status != http.StatusForbidden {
		t.Fatalf("compliance listing policies = %d; want 403", status)
	}

	body := map[string]any{"code": "route-policy", "name": "Route policy", "resourceType": "ORGANIZATION",
		"action": "ACTIVATE", "steps": []map[string]any{{"quorum": 1, "eligibleRoleIds": []string{"rol_system_admin"}}}}
	status, response := gateCall(t, adminRouter, http.MethodPost, "/fleet-partner/v1/approval-policies", body, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("create without Idempotency-Key = %d %v", status, response)
	}
	status, response = gateCall(t, adminRouter, http.MethodPost, "/fleet-partner/v1/approval-policies", body,
		map[string]string{"Idempotency-Key": "route-policy-1"})
	if status != http.StatusCreated {
		t.Fatalf("create policy = %d %v", status, response)
	}
	policy := response["data"].(map[string]any)
	policyID := policy["id"].(string)
	if status, response = gateCall(t, adminRouter, http.MethodPost, "/fleet-partner/v1/approval-policies", body,
		map[string]string{"Idempotency-Key": "route-policy-1"}); status != http.StatusOK ||
		response["meta"].(map[string]any)["replayed"] != true {
		t.Fatalf("replayed create = %d %v", status, response)
	}
	patch := map[string]any{"thresholdAmountMinor": 250000, "thresholdCurrency": "NGN"}
	if status, _ = gateCall(t, adminRouter, http.MethodPatch, "/fleet-partner/v1/approval-policies/"+policyID, patch, nil); status != http.StatusBadRequest {
		t.Fatalf("patch without If-Match = %d; want 400", status)
	}
	if status, _ = gateCall(t, adminRouter, http.MethodPatch, "/fleet-partner/v1/approval-policies/"+policyID, patch,
		map[string]string{"If-Match": "9"}); status != http.StatusConflict {
		t.Fatalf("stale patch = %d; want 409", status)
	}
	if status, _ = gateCall(t, adminRouter, http.MethodPatch, "/fleet-partner/v1/approval-policies/"+policyID,
		map[string]any{"thresholdAmountMinor": 12.5, "thresholdCurrency": "NGN"}, map[string]string{"If-Match": "1"}); status != http.StatusBadRequest {
		t.Fatalf("fractional threshold = %d; want 400", status)
	}
	status, response = gateCall(t, adminRouter, http.MethodPatch, "/fleet-partner/v1/approval-policies/"+policyID, patch,
		map[string]string{"If-Match": "1"})
	if status != http.StatusOK || response["data"].(map[string]any)["version"].(float64) != 2 ||
		response["data"].(map[string]any)["thresholdCurrency"] != "NGN" {
		t.Fatalf("patch = %d %v", status, response)
	}
	if status, response = gateCall(t, adminRouter, http.MethodGet, "/fleet-partner/v1/approval-policies/"+policyID+"/versions", nil, nil); status != http.StatusOK ||
		len(response["data"].([]any)) != 2 {
		t.Fatalf("versions = %d %v", status, response)
	}

	// Delegation over HTTP: the delegate cannot revoke; an approval-policy
	// manager who is not the delegator can.
	expires := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	status, response = gateCall(t, adminRouter, http.MethodPost, "/fleet-partner/v1/approval-delegations",
		map[string]any{"delegateId": compliance, "expiresAt": expires, "reason": "travel"},
		map[string]string{"Idempotency-Key": "route-delegation-1"})
	if status != http.StatusCreated {
		t.Fatalf("create delegation = %d %v", status, response)
	}
	delegationID := response["data"].(map[string]any)["id"].(string)
	if response["data"].(map[string]any)["status"] != approvalmodels.DelegationActive {
		t.Fatalf("delegation view = %v", response)
	}
	if status, response = gateCall(t, complianceRouter, http.MethodGet, "/fleet-partner/v1/approval-delegations?delegateId="+compliance, nil, nil); status != http.StatusOK ||
		len(response["data"].([]any)) != 1 {
		t.Fatalf("delegate listing = %d %v", status, response)
	}
	if status, _ = gateCall(t, complianceRouter, http.MethodPost, "/fleet-partner/v1/approval-delegations/"+delegationID+"/revoke",
		map[string]any{"reason": "no"}, map[string]string{"If-Match": "1"}); status != http.StatusForbidden {
		t.Fatalf("delegate revoke = %d; want 403", status)
	}
	status, response = gateCall(t, f.router(t, f.orgID, owner, nil), http.MethodPost,
		"/fleet-partner/v1/approval-delegations/"+delegationID+"/revoke", map[string]any{"reason": "policy"},
		map[string]string{"If-Match": "1"})
	if status != http.StatusOK || response["data"].(map[string]any)["status"] != approvalmodels.DelegationRevoked {
		t.Fatalf("manager revoke = %d %v", status, response)
	}

	// Another organization's administrator sees neither.
	// A second organization in the same database: testPool may be taken only
	// once per test.
	other := createOrganization(t, f.pool, "Policy Route Other Co", "RC-WS1-HTTP-2")
	otherAdmin := seedApprovalMember(t, f.pool, other.ID, "rol_system_admin", "otheradmin")
	otherRouter := f.router(t, other.ID, otherAdmin, nil)
	for _, path := range []string{"/fleet-partner/v1/approval-policies/" + policyID,
		"/fleet-partner/v1/approval-delegations/" + delegationID} {
		if status, _ := gateCall(t, otherRouter, http.MethodGet, path, nil, nil); status != http.StatusNotFound {
			t.Fatalf("cross-tenant %s = %d; want 404", path, status)
		}
	}
	if status, _ := gateCall(t, otherRouter, http.MethodPatch, "/fleet-partner/v1/approval-policies/"+policyID,
		patch, map[string]string{"If-Match": "2"}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant patch = %d; want 404", status)
	}
}
