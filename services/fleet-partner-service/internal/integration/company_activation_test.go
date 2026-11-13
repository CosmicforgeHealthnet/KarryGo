package integration_test

import (
	"context"
	"sync"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// fakeVerification stands in for verification-compliance-service.
type fakeVerification struct {
	mu      sync.Mutex
	status  string
	version int64
	issues  []platformclients.VerificationIssue
	submits int
}

func (f *fakeVerification) set(status string, version int64, issues ...platformclients.VerificationIssue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.version, f.issues = status, version, issues
}

func (f *fakeVerification) SubmitOrganization(context.Context, platformclients.CallMeta, platformclients.VerificationSubmission) (platformclients.VerificationOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits++
	return platformclients.VerificationOutcome{ExternalReference: "vsub-1", Status: "PENDING", SourceVersion: 1}, nil
}

func (f *fakeVerification) ReadOrganizationStatus(context.Context, platformclients.CallMeta, string, string) (platformclients.VerificationOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return platformclients.VerificationOutcome{ExternalReference: "vsub-1", Status: f.status, SourceVersion: f.version, Issues: f.issues}, nil
}

func (f *fakeVerification) Configured() bool { return true }

// TestCompanyActivationFromVerificationDecision: a submitted company is
// activated once Verification approves it (ADR-006). Changes requested move it
// to CHANGES_REQUIRED with the operator's issues; a stale decision never
// regresses a newer one; a pending submission is dispatched by the
// reconciler.
func TestCompanyActivationFromVerificationDecision(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	ctx := context.Background()
	owner := onboardingLogin(t, router, h, "activation-owner@example.com", "+2348012345651")
	company := map[string]any{"legalName": "Activation Logistics", "registrationNumber": "RC-ACTIVATE", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Activation Owner"}
	code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0)
	if code != 201 {
		t.Fatal(body)
	}
	orgID := documentData(t, body)["organizationId"].(string)

	// A completed, submitted company: stages done, the four required
	// documents ready, one submission that never reached Verification.
	for _, statement := range []string{
		`UPDATE fleet_partner_organization_onboarding_stages SET status='COMPLETE' WHERE organization_id=$1 AND stage NOT IN ('VERIFICATION','WALLET_PROVISIONING','ACTIVATION')`,
		`UPDATE fleet_partner_organizations SET status='BUSINESS_SUBMITTED' WHERE id=$1`,
		`INSERT INTO fleet_partner_organization_documents(id,organization_id,document_type,file_id,status) SELECT 'odoc_'||t||'_'||$1, $1, t, 'file_'||t||'_'||$1, 'READY' FROM unnest(ARRAY['CERTIFICATE_OF_INCORPORATION','TAX_CLEARANCE','INSURANCE_POLICY','OPERATING_LICENSE']) t`,
		`INSERT INTO fleet_partner_organization_verification_requests(id,organization_id,submission_version,submission_snapshot,status,submitted_by) VALUES('vrq_act_'||$1,$1,1,'{"documentFileIds":["f1"]}','PENDING_DISPATCH','tester')`,
	} {
		if _, err := h.pool.Exec(ctx, statement, orgID); err != nil {
			t.Fatal(err)
		}
	}
	verification := &fakeVerification{}
	service := orgusecases.NewService(orgusecases.Options{
		Pool: h.pool, Organizations: orgrepos.NewOrganizationRepository(h.pool), Onboarding: orgrepos.NewOnboardingRepository(h.pool),
		Branches: orgrepos.NewBranchRepository(h.pool), Settings: orgrepos.NewSettingsRepository(h.pool),
		Contracts:        platformclients.ContractSet{Verification: verification, Wallet: platformclients.UnconfiguredWallet{Service: "payment-wallet-service"}},
		Eligibility:      eligibilityusecases.Build(config.Config{}, h.pool, nil),
		WalletGateWaived: true,
		Events:           events.NewDomainPublisher(events.NewOutboxRepository(h.pool)),
	})
	status := func() (org, verificationStatus string) {
		t.Helper()
		if err := h.pool.QueryRow(ctx, `SELECT o.status, v.status FROM fleet_partner_organizations o JOIN fleet_partner_organization_verification_requests v ON v.organization_id=o.id WHERE o.id=$1`, orgID).Scan(&org, &verificationStatus); err != nil {
			t.Fatal(err)
		}
		return
	}
	submission := "vrq_act_" + orgID

	// The reconciler dispatches the stuck submission; Verification accepts.
	if _, err := service.ReconcileVerifications(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if org, v := status(); org != "UNDER_REVIEW" || v != "SUBMITTED" || verification.submits != 1 {
		t.Fatalf("after dispatch: %s %s submits=%d", org, v, verification.submits)
	}

	// Still pending at Verification: read back, nothing changes.
	verification.set("PENDING", 1)
	if _, err := service.ReconcileVerifications(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if org, v := status(); org != "UNDER_REVIEW" || v != "UNDER_REVIEW" {
		t.Fatalf("pending read-back: %s %s", org, v)
	}

	// Operator requests changes: the company sees CHANGES_REQUIRED and why.
	result, err := service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: submission, Status: "CHANGES_REQUIRED", SourceVersion: 2, EventID: "evt-2",
		Issues: []platformclients.VerificationIssue{{Code: "DOCUMENT_EXPIRED", DocumentReference: "TAX_CLEARANCE", Remediation: "Upload a current tax clearance."}},
	})
	if err != nil || !result.Applied {
		t.Fatalf("changes required: %+v %v", result, err)
	}
	if org, v := status(); org != "CHANGES_REQUIRED" || v != "CHANGES_REQUIRED" {
		t.Fatalf("changes required: %s %s", org, v)
	}
	var issues int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_organization_verification_issues WHERE organization_id=$1 AND issue_code='DOCUMENT_EXPIRED'`, orgID).Scan(&issues); err != nil || issues != 1 {
		t.Fatalf("issues: %d %v", issues, err)
	}

	// The company resubmits (the onboarding flow moves it back); the same
	// submission is approved at a newer version.
	if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status='UNDER_REVIEW' WHERE id=$1`, orgID); err != nil {
		t.Fatal(err)
	}
	result, err = service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: submission, Status: "VERIFIED", SourceVersion: 3, EventID: "evt-3",
	})
	if err != nil || !result.Applied || !result.Activated {
		t.Fatalf("approval did not activate: %+v %v", result, err)
	}
	if org, v := status(); org != "ACTIVE" || v != "APPROVED" {
		t.Fatalf("after approval: %s %s", org, v)
	}

	// A late, older decision is ignored.
	result, err = service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: submission, Status: "CHANGES_REQUIRED", SourceVersion: 2, EventID: "evt-2-replay",
	})
	if err != nil || result.Applied {
		t.Fatalf("stale decision applied: %+v %v", result, err)
	}
	if org, v := status(); org != "ACTIVE" || v != "APPROVED" {
		t.Fatalf("stale decision regressed: %s %s", org, v)
	}

	// A decision about another (superseded) submission changes nothing.
	result, err = service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: "vrq_other", Status: "REJECTED", SourceVersion: 9,
	})
	if err != nil || result.Applied {
		t.Fatalf("other submission applied: %+v %v", result, err)
	}

	// The company is now ACTIVE: operational writes pass the lifecycle gate
	// (the providers use case rejects the empty body itself).
	if code, _ := onboardCall(t, router, owner, "POST", "/providers", map[string]any{}, 0); code != 422 {
		t.Fatalf("active company still gated: %d", code)
	}
}

// TestVerificationStatusMapping pins ADR-006's vocabulary mapping.
func TestVerificationStatusMapping(t *testing.T) {
	for in, want := range map[string]string{
		"VERIFIED": "APPROVED", "approved": "APPROVED", "PENDING": "UNDER_REVIEW", "IN_PROGRESS": "UNDER_REVIEW",
		"CHANGES_REQUIRED": "CHANGES_REQUIRED", "REJECTED": "REJECTED", "SUSPENDED": "REJECTED", "SOMETHING": "",
	} {
		if got := orgusecases.MapVerificationStatus(in); got != want {
			t.Errorf("%s -> %q, want %q", in, got, want)
		}
	}
}
