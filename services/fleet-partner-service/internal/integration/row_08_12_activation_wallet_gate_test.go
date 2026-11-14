package integration_test

// FLEET-8.12: a company is activated only after its checks pass AND its
// organization wallet is confirmed active by payment-wallet-service. Unlike
// TestCompanyActivationFromVerificationDecision, the wallet gate is NOT
// waived here.

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

// fakeOrgWallet stands in for payment-wallet-service's organization wallet
// contract. It records every provisioning request and read-back.
type fakeOrgWallet struct {
	mu           sync.Mutex
	ensureStatus string
	ensureErr    error
	readStatus   string
	ensures      []platformclients.WalletProvisionRequest
	reads        []string
}

func (f *fakeOrgWallet) EnsureOrganizationWallet(_ context.Context, _ platformclients.CallMeta, request platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensures = append(f.ensures, request)
	if f.ensureErr != nil {
		return platformclients.WalletAccount{}, f.ensureErr
	}
	return platformclients.WalletAccount{AccountID: "wal_" + request.OrganizationID, Status: f.ensureStatus, SourceVersion: 1}, nil
}

func (f *fakeOrgWallet) EnsureProviderSubWallet(context.Context, platformclients.CallMeta, platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{}, platformclients.ErrNotConfigured
}

func (f *fakeOrgWallet) ReadAccount(_ context.Context, _ platformclients.CallMeta, accountID string) (platformclients.WalletAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, accountID)
	return platformclients.WalletAccount{AccountID: accountID, Status: f.readStatus, SourceVersion: int64(len(f.reads) + 1)}, nil
}

func (f *fakeOrgWallet) DisableForClosure(context.Context, platformclients.CallMeta, string, string) error {
	return nil
}

func (f *fakeOrgWallet) Configured() bool { return true }

func (f *fakeOrgWallet) set(ensureStatus, readStatus string, ensureErr error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureStatus, f.readStatus, f.ensureErr = ensureStatus, readStatus, ensureErr
}

func (f *fakeOrgWallet) counts() (ensures, reads int, keys map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys = map[string]bool{}
	for _, request := range f.ensures {
		keys[request.IdempotencyKey] = true
	}
	return len(f.ensures), len(f.reads), keys
}

func TestRow8_12_ActivationWaitsForOrganizationWallet(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	ctx := context.Background()

	newCompany := func(email, phone, legalName, registration string) string {
		t.Helper()
		owner := onboardingLogin(t, router, h, email, phone)
		company := map[string]any{"legalName": legalName, "registrationNumber": registration, "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Wallet Gate Owner"}
		code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0)
		if code != 201 {
			t.Fatal(body)
		}
		orgID := documentData(t, body)["organizationId"].(string)
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
		return orgID
	}

	verification := &fakeVerification{}
	wallet := &fakeOrgWallet{}
	service := orgusecases.NewService(orgusecases.Options{
		Pool: h.pool, Organizations: orgrepos.NewOrganizationRepository(h.pool), Onboarding: orgrepos.NewOnboardingRepository(h.pool),
		Branches: orgrepos.NewBranchRepository(h.pool), Settings: orgrepos.NewSettingsRepository(h.pool),
		Contracts:   platformclients.ContractSet{Verification: verification, Wallet: wallet},
		Eligibility: eligibilityusecases.Build(config.Config{}, h.pool, nil),
		Events:      events.NewDomainPublisher(events.NewOutboxRepository(h.pool)),
	})
	state := func(orgID string) (org, walletStatus, walletStage string) {
		t.Helper()
		if err := h.pool.QueryRow(ctx, `SELECT o.status, COALESCE(w.status,''), COALESCE(s.status,'')
			FROM fleet_partner_organizations o
			LEFT JOIN fleet_partner_organization_wallet_provisioning w ON w.organization_id=o.id
			LEFT JOIN fleet_partner_organization_onboarding_stages s ON s.organization_id=o.id AND s.stage='WALLET_PROVISIONING'
			WHERE o.id=$1`, orgID).Scan(&org, &walletStatus, &walletStage); err != nil {
			t.Fatal(err)
		}
		return
	}
	reconcile := func() {
		t.Helper()
		if _, err := service.ReconcileVerifications(ctx, 100); err != nil {
			t.Fatal(err)
		}
	}

	orgID := newCompany("r812-owner@example.com", "+2348012347601", "Wallet Gate Logistics", "RC-R812-1")
	submission := "vrq_act_" + orgID
	reconcile() // dispatches the submission
	if org, _, _ := state(orgID); org != "UNDER_REVIEW" {
		t.Fatalf("after dispatch: %s", org)
	}

	// 1. Verification approves; the wallet service accepts the request but the
	// wallet is still being provisioned. The company is NOT activated.
	wallet.set("PROVISIONING", "PROVISIONING", nil)
	result, err := service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: submission, Status: "VERIFIED", SourceVersion: 2, EventID: "evt-r812-2",
	})
	if err != nil || !result.Applied || result.Activated {
		t.Fatalf("approval with a provisioning wallet: %+v %v", result, err)
	}
	if org, w, stage := state(orgID); org != "UNDER_REVIEW" || w != "PROVISIONING" || stage != "IN_PROGRESS" {
		t.Fatalf("after approval: org=%s wallet=%s stage=%s, want UNDER_REVIEW/PROVISIONING/IN_PROGRESS", org, w, stage)
	}
	if ensures, _, keys := wallet.counts(); ensures != 1 || !keys["org-wallet:"+orgID] {
		t.Fatalf("provisioning requests = %d keys=%v, want exactly one keyed org-wallet:%s", ensures, keys, orgID)
	}
	assessment, err := service.EvaluateActivation(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range assessment.Gates {
		if gate.Passed != (gate.Name != "organization_wallet_active") {
			t.Fatalf("gate %s passed=%v; only the wallet gate should block: %+v", gate.Name, gate.Passed, assessment.Gates)
		}
	}

	// A replayed decision and repeated reconciler passes do not send a second
	// provisioning request; they read the confirmed account back instead.
	result, err = service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: orgID, SubmissionID: submission, Status: "VERIFIED", SourceVersion: 2, EventID: "evt-r812-2",
	})
	if err != nil || result.Applied || result.Activated {
		t.Fatalf("replayed approval: %+v %v", result, err)
	}
	reconcile()
	reconcile()
	ensures, reads, _ := wallet.counts()
	if ensures != 1 || reads < 2 {
		t.Fatalf("after replay + reconciles: ensures=%d reads=%d, want 1 ensure and read-backs", ensures, reads)
	}
	if org, w, _ := state(orgID); org != "UNDER_REVIEW" || w != "PROVISIONING" {
		t.Fatalf("still provisioning: org=%s wallet=%s", org, w)
	}

	// 3. A wallet that comes up restricted keeps the company inactive.
	wallet.set("PROVISIONING", "RESTRICTED", nil)
	reconcile()
	if org, w, stage := state(orgID); org != "UNDER_REVIEW" || w != "RESTRICTED" || stage != "IN_PROGRESS" {
		t.Fatalf("restricted wallet: org=%s wallet=%s stage=%s", org, w, stage)
	}

	// 2. The wallet service confirms the wallet ACTIVE; the reconciler reads
	// that back and activation completes through TryActivate.
	wallet.set("PROVISIONING", "ACTIVE", nil)
	reconcile()
	if org, w, stage := state(orgID); org != "ACTIVE" || w != "ACTIVE" || stage != "COMPLETE" {
		t.Fatalf("active wallet: org=%s wallet=%s stage=%s, want ACTIVE/ACTIVE/COMPLETE", org, w, stage)
	}
	if ensures, _, _ := wallet.counts(); ensures != 1 {
		t.Fatalf("provisioning requests over the whole flow = %d, want 1", ensures)
	}
	if n := rowsCount(t, h.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1 AND event_type='organization.activated.v1'`, orgID); n != 1 {
		t.Fatalf("activation events = %d, want 1", n)
	}
	// Further passes change nothing and send nothing.
	reconcile()
	if ensures, _, _ := wallet.counts(); ensures != 1 {
		t.Fatalf("provisioning requests after activation = %d, want 1", ensures)
	}

	// 3b. Provisioning that fails keeps a second company inactive; the retry
	// reuses the same idempotency key, so it can never create a second wallet.
	failed := newCompany("r812-failed@example.com", "+2348012347602", "Wallet Failure Logistics", "RC-R812-2")
	reconcile()
	before, _, _ := wallet.counts()
	wallet.set("", "", &platformclients.Error{Outcome: platformclients.OutcomeTransient})
	result, err = service.ApplyVerificationOutcome(ctx, orgusecases.VerificationOutcomeInput{
		OrganizationID: failed, SubmissionID: "vrq_act_" + failed, Status: "VERIFIED", SourceVersion: 2, EventID: "evt-r812-f2",
	})
	if err != nil || !result.Applied || result.Activated {
		t.Fatalf("approval with a failing wallet: %+v %v", result, err)
	}
	if org, w, _ := state(failed); org != "UNDER_REVIEW" || w != "FAILED_RETRYABLE" {
		t.Fatalf("failed provisioning: org=%s wallet=%s", org, w)
	}
	reconcile()
	if org, _, _ := state(failed); org != "UNDER_REVIEW" {
		t.Fatalf("failed provisioning activated after retry: %s", org)
	}
	after, _, _ := wallet.counts()
	wallet.mu.Lock()
	for _, request := range wallet.ensures[before:] {
		if request.IdempotencyKey != "org-wallet:"+failed {
			t.Fatalf("retry used key %q", request.IdempotencyKey)
		}
	}
	wallet.mu.Unlock()
	if after-before < 2 {
		t.Fatalf("failed provisioning was not retried: %d requests", after-before)
	}
}
