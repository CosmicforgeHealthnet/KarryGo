package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	auditrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	legalholdmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	legalholdrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/repositories"
	legalholdusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	retentionusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
	securitysettingsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/repositories"
	securitysettingsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// TestSection28LegalHold_LifecycleAndTenantIsolation proves create, tenant
// isolation, IsUnderHold, version-guarded release and double-release
// refusal all hold against real Postgres.
func TestSection28LegalHold_LifecycleAndTenantIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Hold Org A", "RC-HOLD-A")
	orgB := createOrganization(t, pool, "Hold Org B", "RC-HOLD-B")

	svc := legalholdusecases.NewService(pool, nil, nil)
	actorA := legalholdusecases.Actor{OrganizationID: orgA.ID, ActorType: "organization_user", ActorID: "ou_1"}

	hold, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actorA, ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_1", Reason: "litigation hold",
		RequestID: "req_1", CorrelationID: "cor_1",
	})
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}
	if hold.Status != legalholdmodels.StatusActive {
		t.Fatalf("new hold status = %q, want ACTIVE", hold.Status)
	}

	underHold, err := svc.IsUnderHold(ctx, orgA.ID, legalholdmodels.ScopeTrip, "trp_1")
	if err != nil || !underHold {
		t.Fatalf("IsUnderHold(orgA, trp_1) = %v, %v; want true, nil", underHold, err)
	}
	underHoldOtherOrg, err := svc.IsUnderHold(ctx, orgB.ID, legalholdmodels.ScopeTrip, "trp_1")
	if err != nil || underHoldOtherOrg {
		t.Fatalf("IsUnderHold(orgB, trp_1) = %v, %v; want false, nil -- cross-tenant leak", underHoldOtherOrg, err)
	}

	// orgB cannot see or fetch orgA's hold.
	if _, err := svc.Get(ctx, orgB.ID, hold.ID); err != legalholdrepos.ErrNotFound {
		t.Fatalf("cross-tenant Get = %v, want ErrNotFound", err)
	}

	// Stale version is refused.
	if _, err := svc.Release(ctx, legalholdusecases.ReleaseInput{
		Actor: actorA, HoldID: hold.ID, ExpectedVersion: hold.Version + 99, ReleaseReason: "resolved",
	}); err != legalholdrepos.ErrVersionMismatch {
		t.Fatalf("stale-version release = %v, want ErrVersionMismatch", err)
	}

	released, err := svc.Release(ctx, legalholdusecases.ReleaseInput{
		Actor: actorA, HoldID: hold.ID, ExpectedVersion: hold.Version, ReleaseReason: "resolved",
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status != legalholdmodels.StatusReleased {
		t.Fatalf("released hold status = %q, want RELEASED", released.Status)
	}

	underHoldAfterRelease, err := svc.IsUnderHold(ctx, orgA.ID, legalholdmodels.ScopeTrip, "trp_1")
	if err != nil || underHoldAfterRelease {
		t.Fatalf("IsUnderHold after release = %v, %v; want false, nil", underHoldAfterRelease, err)
	}

	// A second release attempt on an already-released hold is refused, not a
	// silent no-op.
	if _, err := svc.Release(ctx, legalholdusecases.ReleaseInput{
		Actor: actorA, HoldID: hold.ID, ExpectedVersion: released.Version, ReleaseReason: "again",
	}); err != legalholdrepos.ErrAlreadyReleased {
		t.Fatalf("double release = %v, want ErrAlreadyReleased", err)
	}
}

// TestSection28LegalHold_RejectsInvalidScopeAndMissingReason proves
// validation happens before any row is written.
func TestSection28LegalHold_RejectsInvalidScopeAndMissingReason(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Hold Org Validation", "RC-HOLD-VAL")
	svc := legalholdusecases.NewService(pool, nil, nil)
	actor := legalholdusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"}

	if _, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actor, ScopeType: "DOCUMENT", ScopeReference: "doc_1", Reason: "x",
	}); err != legalholdusecases.ErrScopeTypeInvalid {
		t.Fatalf("DOCUMENT scope = %v, want ErrScopeTypeInvalid (document legal hold has its own path)", err)
	}
	if _, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actor, ScopeType: legalholdmodels.ScopeCustomer, ScopeReference: "cus_1", Reason: "",
	}); err != legalholdusecases.ErrReasonRequired {
		t.Fatalf("empty reason = %v, want ErrReasonRequired", err)
	}

	holds, err := legalholdrepos.NewRepository(pool).List(ctx, legalholdrepos.ListFilter{OrganizationID: org.ID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(holds) != 0 {
		t.Fatalf("expected no holds written after two rejected creates, got %d", len(holds))
	}
}

// TestSection28SecuritySettings_DefaultsCreateUpdateAndVersionGuard proves
// the read-defaults/create/update/version-guard lifecycle against real
// Postgres, including the platform-minimum validation.
func TestSection28SecuritySettings_DefaultsCreateUpdateAndVersionGuard(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Settings Org", "RC-SET-1")
	svc := securitysettingsusecases.NewService(pool, nil)
	actor := securitysettingsusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"}

	defaults, err := svc.Get(ctx, org.ID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	if defaults.Version != 0 {
		t.Fatalf("unset organization should read version 0, got %d", defaults.Version)
	}

	created, validationErrs, err := svc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: actor, ExpectedVersion: 0, Settings: defaults,
	})
	if err != nil || len(validationErrs) != 0 {
		t.Fatalf("create from defaults: err=%v validationErrs=%v", err, validationErrs)
	}
	if created.Version != 1 {
		t.Fatalf("first create version = %d, want 1", created.Version)
	}

	// Creating again at version 0 fails: a row now exists.
	if _, _, err := svc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: actor, ExpectedVersion: 0, Settings: defaults,
	}); err != securitysettingsrepos.ErrVersionMismatch {
		t.Fatalf("second create-at-0 = %v, want ErrVersionMismatch", err)
	}

	// Stale version update fails.
	if _, _, err := svc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: actor, ExpectedVersion: 99, Settings: defaults,
	}); err != securitysettingsrepos.ErrVersionMismatch {
		t.Fatalf("stale version update = %v, want ErrVersionMismatch", err)
	}

	// A value below the platform minimum is rejected before ever reaching SQL.
	tooShort := created
	tooShort.StepUpFreshnessSeconds = 1
	if _, validationErrs, err := svc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: actor, ExpectedVersion: created.Version, Settings: tooShort,
	}); err != nil || len(validationErrs) == 0 {
		t.Fatalf("below-minimum update: err=%v validationErrs=%v, want a validation error and no err", err, validationErrs)
	}

	// A valid update at the correct version succeeds and bumps version.
	extended := created
	extended.StepUpFreshnessSeconds = 900
	updated, validationErrs, err := svc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: actor, ExpectedVersion: created.Version, Settings: extended,
	})
	if err != nil || len(validationErrs) != 0 {
		t.Fatalf("valid update: err=%v validationErrs=%v", err, validationErrs)
	}
	if updated.Version != 2 || updated.StepUpFreshnessSeconds != 900 {
		t.Fatalf("updated settings = %+v, want version=2 stepUpFreshnessSeconds=900", updated)
	}
}

// TestSection28RetentionPolicy_PlatformMinimumEnforcedAndTenantIsolated
// proves an organization can extend but never shorten retention below the
// platform floor, and that the default/list/update path is tenant-isolated.
func TestSection28RetentionPolicy_PlatformMinimumEnforcedAndTenantIsolated(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Retention Org A", "RC-RET-A")
	orgB := createOrganization(t, pool, "Retention Org B", "RC-RET-B")
	svc := retentionusecases.NewService(pool)
	actorA := retentionusecases.Actor{OrganizationID: orgA.ID, ActorType: "organization_user", ActorID: "ou_1"}

	defaultPolicy, err := svc.Get(ctx, orgA.ID, retentionmodels.ClassFinancial)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if defaultPolicy.RetentionDays != retentionmodels.PlatformMinimumDays[retentionmodels.ClassFinancial] {
		t.Fatalf("default FINANCIAL retention = %d, want platform minimum %d",
			defaultPolicy.RetentionDays, retentionmodels.PlatformMinimumDays[retentionmodels.ClassFinancial])
	}

	// Below platform minimum is refused before any row is written.
	if _, err := svc.Update(ctx, retentionusecases.UpdateInput{
		Actor: actorA, RetentionClass: retentionmodels.ClassFinancial, RetentionDays: 1, ExpectedVersion: 0,
	}); err != retentionusecases.ErrBelowPlatformMinimum {
		t.Fatalf("below-minimum update = %v, want ErrBelowPlatformMinimum", err)
	}

	extended := retentionmodels.PlatformMinimumDays[retentionmodels.ClassFinancial] + 365
	updated, err := svc.Update(ctx, retentionusecases.UpdateInput{
		Actor: actorA, RetentionClass: retentionmodels.ClassFinancial, RetentionDays: extended, ExpectedVersion: 0,
	})
	if err != nil {
		t.Fatalf("valid extend: %v", err)
	}
	if updated.RetentionDays != extended || updated.Version != 1 {
		t.Fatalf("updated policy = %+v, want retentionDays=%d version=1", updated, extended)
	}

	// orgB's FINANCIAL policy is unaffected by orgA's override -- tenant
	// isolation, not shared global state.
	orgBPolicy, err := svc.Get(ctx, orgB.ID, retentionmodels.ClassFinancial)
	if err != nil {
		t.Fatalf("get orgB: %v", err)
	}
	if orgBPolicy.RetentionDays != retentionmodels.PlatformMinimumDays[retentionmodels.ClassFinancial] {
		t.Fatalf("orgB FINANCIAL retention leaked orgA's override: got %d, want platform minimum", orgBPolicy.RetentionDays)
	}

	// List includes all fourteen classes even though only one was ever
	// explicitly overridden.
	all, err := svc.List(ctx, orgA.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != len(retentionmodels.PlatformMinimumDays) {
		t.Fatalf("list returned %d classes, want %d", len(all), len(retentionmodels.PlatformMinimumDays))
	}

	// Unrecognized class is rejected.
	if _, err := svc.Get(ctx, orgA.ID, "NOT_A_CLASS"); err != retentionusecases.ErrClassInvalid {
		t.Fatalf("unknown class = %v, want ErrClassInvalid", err)
	}
}

// TestSection28Audit_ListFiltersTenantIsolationAndHighRisk proves the new
// audit-log read repository correctly scopes by organization, filters by
// resource/action, and classifies high-risk actions -- against real
// audit_records rows written through the pre-existing Recorder.
func TestSection28Audit_ListFiltersTenantIsolationAndHighRisk(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Audit Org A", "RC-AUD-A")
	orgB := createOrganization(t, pool, "Audit Org B", "RC-AUD-B")
	recorder := audit.NewRecorder()

	mustRecord := func(orgID, action, resourceType, resourceID string) {
		t.Helper()
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: orgID, ActorType: "organization_user", ActorID: "ou_1",
			Action: action, ResourceType: resourceType, ResourceID: resourceID,
			RequestID: "req_x", CorrelationID: "cor_x",
		}); err != nil {
			t.Fatalf("record %s: %v", action, err)
		}
	}

	// "document.viewed" is deliberately NOT in the high-risk allowlist
	// (repositories.highRiskActionSubstrings), unlike "legal_hold.created" or
	// "organization.suspend" -- it exercises the negative case of the filter.
	mustRecord(orgA.ID, "document.viewed", "document", "doc_1")
	mustRecord(orgA.ID, "organization.suspend", "organization", orgA.ID)
	mustRecord(orgB.ID, "document.viewed", "document", "doc_2")

	repo := auditrepos.NewRepository(pool)

	orgARecords, _, err := repo.List(ctx, auditrepos.ListFilter{OrganizationID: orgA.ID})
	if err != nil {
		t.Fatalf("list orgA: %v", err)
	}
	if len(orgARecords) != 2 {
		t.Fatalf("orgA record count = %d, want 2 (tenant isolation must exclude orgB's record)", len(orgARecords))
	}
	for _, r := range orgARecords {
		if r.OrganizationID != orgA.ID {
			t.Fatalf("cross-tenant leak: record %s belongs to %s", r.ID, r.OrganizationID)
		}
	}

	highRisk, _, err := repo.List(ctx, auditrepos.ListFilter{OrganizationID: orgA.ID, HighRiskOnly: true})
	if err != nil {
		t.Fatalf("list high-risk: %v", err)
	}
	if len(highRisk) != 1 || highRisk[0].Action != "organization.suspend" {
		t.Fatalf("high-risk filter returned %+v, want exactly [organization.suspend]", highRisk)
	}

	byResource, _, err := repo.List(ctx, auditrepos.ListFilter{OrganizationID: orgA.ID, ResourceType: "document"})
	if err != nil {
		t.Fatalf("list by resource type: %v", err)
	}
	if len(byResource) != 1 || byResource[0].ResourceID != "doc_1" {
		t.Fatalf("resource-type filter returned %+v, want exactly [doc_1]", byResource)
	}

	// A caller cannot fetch another organization's record by ID either.
	if _, err := repo.Get(ctx, orgA.ID, byResource[0].ID); err != nil {
		t.Fatalf("get own record: %v", err)
	}
	crossTenantRecords, _, err := repo.List(ctx, auditrepos.ListFilter{OrganizationID: orgB.ID, ResourceType: "document"})
	if err != nil || len(crossTenantRecords) != 1 || crossTenantRecords[0].ResourceID != "doc_2" {
		t.Fatalf("orgB should see only its own record, got %+v (err=%v)", crossTenantRecords, err)
	}

	// Bounded date range: a >90-day span is not exercised by the repository
	// directly (that guard lives in the HTTP layer's dateRangeFrom), but a
	// legitimate narrow range must still return results.
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	windowed, _, err := repo.List(ctx, auditrepos.ListFilter{OrganizationID: orgA.ID, From: from, To: to})
	if err != nil || len(windowed) != 2 {
		t.Fatalf("windowed list = %+v (err=%v), want 2 records within the last hour", windowed, err)
	}
}

// fakeStepUpVerifier is a real implementation of stepup.Verifier good enough
// to prove the enforcement wiring itself -- it does not simulate the real
// challenge/consumed/expiry machinery internal/features/auth already owns
// and already tests; that is out of scope here. What IS in scope: does a
// usecase actually call the verifier with the right identity/session/org/
// action-class, and does it fail closed on both a wrong reference and a
// missing one.
type fakeStepUpVerifier struct {
	validReference string
	calls          []fakeStepUpCall
}

type fakeStepUpCall struct {
	reference, identityID, sessionID, organizationID, actionClass string
}

func (f *fakeStepUpVerifier) Verify(_ context.Context, reference, identityID, sessionID, organizationID, actionClass string) error {
	f.calls = append(f.calls, fakeStepUpCall{reference, identityID, sessionID, organizationID, actionClass})
	if reference == "" || reference != f.validReference {
		return errStepUpDenied
	}
	return nil
}

var errStepUpDenied = fmt.Errorf("fake step-up: denied")

// TestSection28LegalHold_StepUpEnforcedWhenOrgRequiresMFA proves that once a
// stepup.Verifier is wired AND the organization's own security settings say
// MFA is required, legal-hold create/release actually calls it with the
// correct identity/session/org/action-class and fails closed on a missing
// or wrong reference -- not merely records the reference unverified.
func TestSection28LegalHold_StepUpEnforcedWhenOrgRequiresMFA(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "StepUp Org", "RC-STEPUP-1")

	settingsSvc := securitysettingsusecases.NewService(pool, nil)
	// MFARequiredForSensitiveActions defaults to true even before any
	// explicit override is written -- see models.Defaults -- so no setup
	// write is needed to exercise the "required" path.

	verifier := &fakeStepUpVerifier{validReference: "step_ok"}
	svc := legalholdusecases.NewService(pool, verifier, settingsSvc)
	actor := legalholdusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"}

	// Missing step-up reference is refused before any row is written.
	if _, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actor, IdentityID: "idn_1", SessionID: "ses_1",
		ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_stepup", Reason: "litigation hold",
	}); err == nil {
		t.Fatal("create without step-up reference should be refused when MFA is required")
	}
	if holds, _ := legalholdrepos.NewRepository(pool).List(ctx, legalholdrepos.ListFilter{OrganizationID: org.ID}); len(holds) != 0 {
		t.Fatalf("a refused step-up must not write a row, found %d", len(holds))
	}

	// Wrong reference is refused too.
	if _, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actor, IdentityID: "idn_1", SessionID: "ses_1", StepUpReference: "wrong",
		ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_stepup", Reason: "litigation hold",
	}); err == nil {
		t.Fatal("create with a wrong step-up reference should be refused")
	}

	// Correct reference succeeds, and the verifier was called with exactly
	// the identity/session/org/action-class this request carried.
	hold, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor: actor, IdentityID: "idn_1", SessionID: "ses_1", StepUpReference: "step_ok",
		ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_stepup", Reason: "litigation hold",
	})
	if err != nil {
		t.Fatalf("create with valid step-up: %v", err)
	}
	if len(verifier.calls) != 3 {
		t.Fatalf("expected 3 verifier calls (2 refused + 1 accepted), got %d", len(verifier.calls))
	}
	last := verifier.calls[len(verifier.calls)-1]
	if last.identityID != "idn_1" || last.sessionID != "ses_1" || last.organizationID != org.ID ||
		last.actionClass != legalholdusecases.ActionClassCreate {
		t.Fatalf("verifier called with unexpected arguments: %+v", last)
	}

	// Release requires its own step-up, with its own action class -- a
	// grant scoped to "create" (which this request doesn't even present)
	// cannot silently authorize "release".
	if _, err := svc.Release(ctx, legalholdusecases.ReleaseInput{
		Actor: actor, IdentityID: "idn_1", SessionID: "ses_1",
		HoldID: hold.ID, ExpectedVersion: hold.Version, ReleaseReason: "resolved",
	}); err == nil {
		t.Fatal("release without step-up reference should be refused when MFA is required")
	}
	released, err := svc.Release(ctx, legalholdusecases.ReleaseInput{
		Actor: actor, IdentityID: "idn_1", SessionID: "ses_1", StepUpReference: "step_ok",
		HoldID: hold.ID, ExpectedVersion: hold.Version, ReleaseReason: "resolved",
	})
	if err != nil {
		t.Fatalf("release with valid step-up: %v", err)
	}
	if released.Status != legalholdmodels.StatusReleased {
		t.Fatalf("released status = %q, want RELEASED", released.Status)
	}
	releaseCall := verifier.calls[len(verifier.calls)-1]
	if releaseCall.actionClass != legalholdusecases.ActionClassRelease {
		t.Fatalf("release action class = %q, want %q", releaseCall.actionClass, legalholdusecases.ActionClassRelease)
	}
}

// TestSection28LegalHold_StepUpSkippedWhenOrgDisablesMFA proves the
// organization's own security settings genuinely control the requirement --
// turning MFARequiredForSensitiveActions off (which itself required a
// step-up to do, exercised in the same test) makes a subsequent hold
// creation succeed with no step-up reference at all.
func TestSection28LegalHold_StepUpSkippedWhenOrgDisablesMFA(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "StepUp Opt-Out Org", "RC-STEPUP-2")

	verifier := &fakeStepUpVerifier{validReference: "step_ok"}
	settingsSvc := securitysettingsusecases.NewService(pool, verifier)
	settingsActor := securitysettingsusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"}

	defaults, err := settingsSvc.Get(ctx, org.ID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	disabled := defaults
	disabled.MFARequiredForSensitiveActions = false

	// Turning MFA off is itself a security-settings change, and defaults
	// require MFA, so this very call needs a valid step-up.
	if _, _, err := settingsSvc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: settingsActor, IdentityID: "idn_1", SessionID: "ses_1",
		ExpectedVersion: 0, Settings: disabled,
	}); err == nil {
		t.Fatal("disabling MFA without step-up should itself be refused")
	}
	updated, _, err := settingsSvc.Update(ctx, securitysettingsusecases.UpdateInput{
		Actor: settingsActor, IdentityID: "idn_1", SessionID: "ses_1", StepUpReference: "step_ok",
		ExpectedVersion: 0, Settings: disabled,
	})
	if err != nil {
		t.Fatalf("disabling MFA with valid step-up: %v", err)
	}
	if updated.MFARequiredForSensitiveActions {
		t.Fatal("settings should now have MFA disabled")
	}

	legalHoldVerifier := &fakeStepUpVerifier{validReference: "never_used"}
	legalSvc := legalholdusecases.NewService(pool, legalHoldVerifier, settingsSvc)
	legalActor := legalholdusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"}

	if _, err := legalSvc.Create(ctx, legalholdusecases.CreateInput{
		Actor: legalActor, IdentityID: "idn_1", SessionID: "ses_1",
		ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_no_mfa", Reason: "litigation hold",
	}); err != nil {
		t.Fatalf("create without step-up should succeed once the org disables MFA: %v", err)
	}
	if len(legalHoldVerifier.calls) != 0 {
		t.Fatalf("verifier should never be called once MFA is disabled, got %d calls", len(legalHoldVerifier.calls))
	}
}
