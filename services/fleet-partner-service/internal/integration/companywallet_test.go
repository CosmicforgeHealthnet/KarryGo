package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	walletmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	walletrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	walletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// walletFixture is one organization with a linked wallet and one managed
// provider with a linked sub-wallet, both marked ACTIVE and CURRENT.
type walletFixture struct {
	pool         *pgxpool.Pool
	service      *walletusecases.Service
	repo         *walletrepos.Repository
	wallet       *stubWallet
	organization string
	membership   string
	tenant       tenant.Context
}

// stubWallet stands in for payment-wallet-service. It records what was asked
// and answers what the test configures, including the outcomes that matter
// most: an unknown result, and a read-back that resolves it.
type stubWallet struct {
	account      platformclients.WalletAccount
	accountErr   error
	submitResult platformclients.WalletOperation
	submitErr    error
	readResult   platformclients.WalletOperation
	readErr      error
	submits      []platformclients.WalletOperationRequest
	readBacks    int
}

func (s *stubWallet) Configured() bool { return true }

func (s *stubWallet) EnsureOrganizationWallet(context.Context, platformclients.CallMeta,
	platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return s.account, s.accountErr
}

func (s *stubWallet) EnsureProviderSubWallet(context.Context, platformclients.CallMeta,
	platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return s.account, s.accountErr
}

func (s *stubWallet) ReadAccount(context.Context, platformclients.CallMeta,
	string) (platformclients.WalletAccount, error) {
	return s.account, s.accountErr
}

func (s *stubWallet) DisableForClosure(context.Context, platformclients.CallMeta, string, string) error {
	return nil
}

func (s *stubWallet) SubmitOperation(_ context.Context, _ platformclients.CallMeta,
	request platformclients.WalletOperationRequest) (platformclients.WalletOperation, error) {
	s.submits = append(s.submits, request)
	return s.submitResult, s.submitErr
}

func (s *stubWallet) ReadOperation(context.Context, platformclients.CallMeta, string, string,
	string) (platformclients.WalletOperation, error) {
	s.readBacks++
	return s.readResult, s.readErr
}

func (s *stubWallet) ReadOperationByKey(context.Context, platformclients.CallMeta, string, string,
	string) (platformclients.WalletOperation, error) {
	s.readBacks++
	return s.readResult, s.readErr
}

func (s *stubWallet) ReadProjection(context.Context, platformclients.CallMeta, string, string,
	string) (platformclients.WalletOperation, error) {
	return s.readResult, s.readErr
}

func (s *stubWallet) VerifyJobPayment(context.Context, platformclients.CallMeta, string, string,
	string) (platformclients.JobPaymentVerification, error) {
	return platformclients.JobPaymentVerification{Settled: true}, nil
}

func (s *stubWallet) ReadReversal(context.Context, platformclients.CallMeta, string,
	string) (platformclients.WalletReversal, error) {
	return platformclients.WalletReversal{}, nil
}

// allowingEligibility passes every gate. The gate itself is exercised by the
// eligibility suite; these tests are about what happens once it is passed.
type allowingEligibility struct{}

func (allowingEligibility) Require(context.Context, string, string, string, string, string,
	string) error {
	return nil
}

// completedWork reports every referenced job as complete unless told otherwise,
// so a test can isolate the completion gate from everything else.
type completedWork struct{ err error }

func (c completedWork) RequireCompletedDelivery(context.Context, string, string, bool) error {
	return c.err
}

func newWalletFixture(t *testing.T, flags walletusecases.Flags, work completedWork) *walletFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	org := createOrganization(t, pool, "Wallet Fixture Ltd", "RC-"+ids.New("reg")[4:14])
	membershipID := ids.New("mem")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id,
			 onboarding_source, status)
		VALUES ($1, $2, 'driver-dispatch-delivery-service', 'DISPATCH', $3,
		        'ORGANIZATION_SPONSORED', 'ACTIVE')`,
		membershipID, org.ID, "prv_external_1"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_wallet_provisioning
			(organization_id, status, wallet_account_id, currency, freshness_state,
			 source_version, synced_at)
		VALUES ($1, 'ACTIVE', 'fwal_org_1', 'NGN', 'CURRENT', 3, now())`, org.ID); err != nil {
		t.Fatalf("seed wallet link: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_provider_subwallet_links
			(provider_membership_id, organization_id, wallet_account_id, provider_id,
			 provider_service, currency, status, freshness_state, source_version, synced_at)
		VALUES ($1, $2, 'fear_sub_1', 'prv_external_1', 'DISPATCH', 'NGN', 'ACTIVE',
		        'CURRENT', 7, now())`, membershipID, org.ID); err != nil {
		t.Fatalf("seed sub-wallet link: %v", err)
	}

	wallet := &stubWallet{
		account:      platformclients.WalletAccount{AccountID: "fwal_org_1", Status: "ACTIVE", SourceVersion: 3},
		submitResult: platformclients.WalletOperation{ID: "fpal_1", Status: "POSTED", Version: 1},
		readResult:   platformclients.WalletOperation{ID: "fpal_1", Status: "POSTED", Version: 1},
	}
	repo := walletrepos.NewRepository(pool)
	service := walletusecases.NewService(walletusecases.Options{
		Pool: pool, Repository: repo, Wallet: wallet,
		Eligibility: allowingEligibility{}, Work: work,
		Events:  events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics: observability.NewMetrics(), Flags: flags,
		StaleAfter: time.Hour,
	})
	return &walletFixture{pool: pool, service: service, repo: repo, wallet: wallet,
		organization: org.ID, membership: membershipID,
		tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr_staff_1",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_staff_1",
			RequestID: "req_1", CorrelationID: "cor_1"}}
}

func allFlagsOn() walletusecases.Flags {
	return walletusecases.Flags{WalletReadEnabled: true, SubWalletEnabled: true,
		AllocationEnabled: true, WithdrawalEnabled: true, ProjectionRefreshEnabled: true,
		ReconciliationEnabled: true}
}

func salaryAllocation(f *walletFixture, key string) walletusecases.AllocationInput {
	return walletusecases.AllocationInput{Tenant: f.tenant, MembershipID: f.membership,
		AllocationType: "SALARY", AmountMinor: 700_000, Currency: "NGN",
		BusinessReferenceType: "PAYROLL_REFERENCE", BusinessReferenceID: "payroll-2026-07",
		ReasonCode: "MONTHLY_SALARY", IdempotencyKey: key}
}

func TestAllocationIsIdempotentAndFingerprintChecked(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()

	first, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "key-1"))
	if err != nil {
		t.Fatalf("first allocation: %v", err)
	}
	second, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "key-1"))
	if err != nil {
		t.Fatalf("retry of identical allocation: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("a retry created a second allocation: %s vs %s", first.ID, second.ID)
	}

	// The same key with a different amount describes a different operation.
	// Answering with the first result would silently discard the second
	// caller's intent, so it is a conflict.
	conflicting := salaryAllocation(f, "key-1")
	conflicting.AmountMinor = 900_000
	if _, err := f.service.RequestAllocation(ctx, conflicting); err == nil {
		t.Fatal("a reused idempotency key with different values must be rejected")
	}
}

func TestDuplicatePerJobAllocationIsRefused(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	perJob := func(key string) walletusecases.AllocationInput {
		return walletusecases.AllocationInput{Tenant: f.tenant, MembershipID: f.membership,
			AllocationType: "PER_JOB", AmountMinor: 25_000, Currency: "NGN",
			BusinessReferenceType: "TRIP", BusinessReferenceID: "trp_1",
			ReasonCode: "JOB_COMPLETED", IdempotencyKey: key}
	}
	if _, err := f.service.RequestAllocation(ctx, perJob("job-key-1")); err != nil {
		t.Fatalf("first per-job allocation: %v", err)
	}
	// A different idempotency key for the same completed trip is not a retry;
	// it is a second payment for one piece of work.
	_, err := f.service.RequestAllocation(ctx, perJob("job-key-2"))
	if err == nil {
		t.Fatal("a second per-job allocation for the same trip must be refused")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error for duplicate business purpose: %v", err)
	}
}

func TestFailedWorkCannotFundPerJobAllocation(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{err: context.Canceled})
	_, err := f.service.RequestAllocation(context.Background(),
		walletusecases.AllocationInput{Tenant: f.tenant, MembershipID: f.membership,
			AllocationType: "PER_JOB", AmountMinor: 25_000, Currency: "NGN",
			BusinessReferenceType: "TRIP", BusinessReferenceID: "trp_failed",
			ReasonCode: "JOB_COMPLETED", IdempotencyKey: "failed-job"})
	if err == nil {
		t.Fatal("a per-job allocation against unconfirmed work must be refused")
	}
}

// A deduction takes money from someone who is not in the room, so it is
// refused without a provider-visible explanation and a stated legal basis.
func TestDeductionRequiresExplanationAndLegalBasis(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	base := walletusecases.AllocationInput{Tenant: f.tenant, MembershipID: f.membership,
		AllocationType: "APPROVED_DEDUCTION", AmountMinor: 10_000, Currency: "NGN",
		BusinessReferenceType: "MANUAL_APPROVED_REFERENCE", BusinessReferenceID: "case-1",
		ReasonCode: "DAMAGE_RECOVERY", IdempotencyKey: "ded-1"}

	if _, err := f.service.RequestAllocation(ctx, base); err == nil {
		t.Fatal("a deduction without a provider explanation must be refused")
	}
	withExplanation := base
	withExplanation.ProviderExplanation = "Agreed recovery for vehicle damage."
	if _, err := f.service.RequestAllocation(ctx, withExplanation); err == nil {
		t.Fatal("a deduction without a legal basis reference must be refused")
	}
	complete := withExplanation
	complete.LegalBasisReference = "contract-clause-7"
	instruction, err := f.service.RequestAllocation(ctx, complete)
	if err != nil {
		t.Fatalf("a fully justified deduction: %v", err)
	}
	// A deduction always requires approval regardless of amount.
	if instruction.Status != walletmodels.AllocationPendingApproval {
		t.Fatalf("deduction status = %q, want %q", instruction.Status,
			walletmodels.AllocationPendingApproval)
	}
}

func TestSection15ReferenceTypesAreRejectedAtTheService(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	for _, referenceType := range []string{"PRIVATE_ORDER", "SHIPMENT"} {
		_, err := f.service.RequestAllocation(context.Background(),
			walletusecases.AllocationInput{Tenant: f.tenant, MembershipID: f.membership,
				AllocationType: "PER_JOB", AmountMinor: 1_000, Currency: "NGN",
				BusinessReferenceType: referenceType, BusinessReferenceID: "x",
				ReasonCode: "R", IdempotencyKey: "s15-" + referenceType})
		if err == nil {
			t.Errorf("%s must not be an accepted business reference type", referenceType)
		}
	}
}

// The requester may not approve their own allocation. One person making and
// authorizing the same financial fact is what separation of duties exists to
// prevent.
func TestAllocationSelfApprovalIsRefused(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	input := salaryAllocation(f, "sod-1")
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_withdrawal_policies
			(organization_id, policy_mode, allocation_approval_threshold_minor)
		VALUES ($1, 'COMPANY_APPROVAL_REQUIRED', 1)`, f.organization); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	instruction, err := f.service.RequestAllocation(ctx, input)
	if err != nil {
		t.Fatalf("request allocation: %v", err)
	}
	if instruction.Status != walletmodels.AllocationPendingApproval {
		t.Fatalf("status = %q, want PENDING_APPROVAL", instruction.Status)
	}
	if _, err := f.service.DecideAllocation(ctx, f.tenant, instruction.ID, instruction.Version,
		true, "ok"); err == nil {
		t.Fatal("the requester must not be able to approve their own allocation")
	}

	approver := f.tenant
	approver.ActorID = "usr_staff_2"
	approver.OrganizationUserID = "usr_staff_2"
	approved, err := f.service.DecideAllocation(ctx, approver, instruction.ID,
		instruction.Version, true, "ok")
	if err != nil {
		t.Fatalf("approval by a second person: %v", err)
	}
	if approved.Status != walletmodels.AllocationApproved {
		t.Fatalf("status after approval = %q", approved.Status)
	}
}

// Two approvers deciding concurrently must produce one decision and one
// conflict, never two decisions.
func TestConcurrentAllocationDecisionsProduceOneWinner(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_withdrawal_policies
			(organization_id, policy_mode, allocation_approval_threshold_minor)
		VALUES ($1, 'COMPANY_APPROVAL_REQUIRED', 1)`, f.organization); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	instruction, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "race-1"))
	if err != nil {
		t.Fatalf("request allocation: %v", err)
	}

	type outcome struct{ err error }
	results := make(chan outcome, 2)
	for _, actor := range []string{"usr_a", "usr_b"} {
		go func(actorID string) {
			approver := f.tenant
			approver.ActorID = actorID
			approver.OrganizationUserID = actorID
			_, decideErr := f.service.DecideAllocation(ctx, approver, instruction.ID,
				instruction.Version, true, "ok")
			results <- outcome{err: decideErr}
		}(actor)
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if (<-results).err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent approvals succeeded %d times, want exactly 1", successes)
	}
}

// An UNKNOWN Wallet outcome must be read back, never resubmitted: re-issuing
// a payment because the first answer was lost is how a provider gets paid
// twice.
func TestUnknownOutcomeIsResolvedByReadBackNotResubmission(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	instruction, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "unknown-1"))
	if err != nil {
		t.Fatalf("request allocation: %v", err)
	}

	f.wallet.submitErr = &platformclients.Error{Outcome: platformclients.OutcomeUnknown}
	if err := f.service.RunPass(ctx, walletusecases.WorkerConfig{}); err != nil {
		t.Fatalf("first worker pass: %v", err)
	}
	stored, err := f.repo.GetAllocation(ctx, nil, f.organization, instruction.ID, nil)
	if err != nil {
		t.Fatalf("read allocation: %v", err)
	}
	if stored.Status != walletmodels.AllocationUnknown {
		t.Fatalf("status after unknown outcome = %q, want UNKNOWN", stored.Status)
	}
	submitsAfterFirstPass := len(f.wallet.submits)

	// The second pass must ask, not act.
	f.wallet.submitErr = nil
	if err := f.service.RunPass(ctx, walletusecases.WorkerConfig{}); err != nil {
		t.Fatalf("second worker pass: %v", err)
	}
	if len(f.wallet.submits) != submitsAfterFirstPass {
		t.Fatalf("the worker resubmitted an unknown operation: %d submits, want %d",
			len(f.wallet.submits), submitsAfterFirstPass)
	}
	if f.wallet.readBacks == 0 {
		t.Fatal("an unknown outcome must be resolved by an authoritative read-back")
	}
	resolved, err := f.repo.GetAllocation(ctx, nil, f.organization, instruction.ID, nil)
	if err != nil {
		t.Fatalf("read allocation: %v", err)
	}
	if resolved.Status != walletmodels.AllocationCompleted {
		t.Fatalf("status after read-back = %q, want COMPLETED", resolved.Status)
	}
}

// A stale local projection must fail a sensitive operation closed rather than
// authorize it: the account may have been restricted since it was copied.
func TestStaleWalletProjectionBlocksAllocation(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		UPDATE fleet_partner_organization_wallet_provisioning
		SET freshness_state = 'STALE' WHERE organization_id = $1`, f.organization); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	// The authoritative read is also unavailable, so nothing can restore trust.
	f.wallet.accountErr = &platformclients.Error{Outcome: platformclients.OutcomeUnknown}
	if _, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "stale-1")); err == nil {
		t.Fatal("an allocation against a stale, unconfirmable wallet must be refused")
	}
}

func TestSuspendedSubWalletBlocksAllocation(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		UPDATE fleet_partner_provider_subwallet_links
		SET status = 'SUSPENDED' WHERE provider_membership_id = $1`, f.membership); err != nil {
		t.Fatalf("suspend sub-wallet: %v", err)
	}
	f.wallet.account = platformclients.WalletAccount{AccountID: "fear_sub_1",
		Status: "SUSPENDED", SourceVersion: 8}
	if _, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "suspended-1")); err == nil {
		t.Fatal("an allocation to a suspended sub-wallet must be refused")
	}
}

// The provider statement is the data-minimization boundary. This asserts on
// the serialized bytes rather than on struct fields, because the question is
// what actually reaches the provider.
func TestProviderStatementExcludesCommercialData(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	input := salaryAllocation(f, "stmt-1")
	input.PrivateNote = "Margin on this job was 41 percent; do not disclose."
	input.ProviderExplanation = "July salary."
	if _, err := f.service.RequestAllocation(ctx, input); err != nil {
		t.Fatalf("request allocation: %v", err)
	}

	statement, err := f.service.ProviderStatement(ctx, walletusecases.StatementInput{
		Tenant: f.tenant, MembershipID: f.membership, ProviderScoped: true})
	if err != nil {
		t.Fatalf("provider statement: %v", err)
	}
	encoded, err := json.Marshal(statement)
	if err != nil {
		t.Fatalf("marshal statement: %v", err)
	}
	body := strings.ToLower(string(encoded))

	for _, forbidden := range []string{
		"privatenote", "margin", "commission", "customerfare", "fare",
		"customerprice", "settlementamount", "walletbalance", "organizationprofit",
		"contractprice", "quotationprice", "paymentmethod", "bank", "destination",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("provider statement leaked %q: %s", forbidden, encoded)
		}
	}
	// The safe fields are present, so the test is not passing by emptiness.
	if !strings.Contains(body, "july salary") {
		t.Errorf("provider statement lost the provider-visible explanation: %s", encoded)
	}
	if len(statement.Entries) != 1 {
		t.Fatalf("statement entries = %d, want 1", len(statement.Entries))
	}
}

// A provider statement must never contain another provider's transactions.
func TestProviderStatementIsScopedToOneMembership(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()

	other := ids.New("mem")
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id,
			 onboarding_source, status)
		VALUES ($1, $2, 'driver-dispatch-delivery-service', 'DISPATCH', 'prv_external_2',
		        'ORGANIZATION_SPONSORED', 'ACTIVE')`, other, f.organization); err != nil {
		t.Fatalf("seed second membership: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_provider_subwallet_links
			(provider_membership_id, organization_id, wallet_account_id, provider_id,
			 provider_service, currency, status, freshness_state, source_version, synced_at)
		VALUES ($1, $2, 'fear_sub_2', 'prv_external_2', 'DISPATCH', 'NGN', 'ACTIVE',
		        'CURRENT', 1, now())`, other, f.organization); err != nil {
		t.Fatalf("seed second sub-wallet: %v", err)
	}

	if _, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "mine")); err != nil {
		t.Fatalf("allocation for first provider: %v", err)
	}
	theirs := salaryAllocation(f, "theirs")
	theirs.MembershipID = other
	theirs.BusinessReferenceID = "payroll-other"
	if _, err := f.service.RequestAllocation(ctx, theirs); err != nil {
		t.Fatalf("allocation for second provider: %v", err)
	}

	statement, err := f.service.ProviderStatement(ctx, walletusecases.StatementInput{
		Tenant: f.tenant, MembershipID: f.membership, ProviderScoped: true})
	if err != nil {
		t.Fatalf("provider statement: %v", err)
	}
	if len(statement.Entries) != 1 {
		t.Fatalf("statement leaked another provider's entries: %d entries", len(statement.Entries))
	}
}

// Cross-tenant identifiers are answered exactly like missing ones. A
// distinguishable response would be an existence oracle.
func TestCrossTenantAllocationReadsAreNotFound(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	instruction, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "tenant-1"))
	if err != nil {
		t.Fatalf("request allocation: %v", err)
	}
	other := createOrganization(t, f.pool, "Other Tenant Ltd", "RC-"+ids.New("reg")[4:14])
	intruder := f.tenant
	intruder.OrganizationID = other.ID
	if _, err := f.service.GetAllocation(ctx, intruder, instruction.ID); err == nil {
		t.Fatal("an allocation must not be readable from another organization")
	}
}

func TestWithdrawalPolicyModesAreEnforced(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	setPolicy := func(mode string) {
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO fleet_partner_organization_withdrawal_policies (organization_id, policy_mode)
			VALUES ($1, $2)
			ON CONFLICT (organization_id) DO UPDATE SET policy_mode = EXCLUDED.policy_mode`,
			f.organization, mode); err != nil {
			t.Fatalf("set policy %s: %v", mode, err)
		}
	}
	request := func(key, actorType string) (walletmodels.WithdrawalRequest, error) {
		return f.service.RequestWithdrawal(ctx, walletusecases.WithdrawalInput{
			Tenant: f.tenant, MembershipID: f.membership, RequestedByActorType: actorType,
			AmountMinor: 50_000, Currency: "NGN", DestinationReference: "bank_ref_opaque_1",
			IdempotencyKey: key})
	}

	setPolicy("WITHDRAWALS_DISABLED")
	if _, err := request("dis-1", walletusecases.RequesterProvider); err == nil {
		t.Fatal("WITHDRAWALS_DISABLED must refuse every request")
	}

	setPolicy("COMPANY_INITIATED_ONLY")
	if _, err := request("cio-1", walletusecases.RequesterProvider); err == nil {
		t.Fatal("COMPANY_INITIATED_ONLY must refuse a provider-opened request")
	}
	staffRequest, err := request("cio-2", walletusecases.RequesterStaff)
	if err != nil {
		t.Fatalf("COMPANY_INITIATED_ONLY staff request: %v", err)
	}
	if staffRequest.Status != walletmodels.WithdrawalPendingApproval {
		t.Fatalf("status = %q, want PENDING_APPROVAL", staffRequest.Status)
	}
}

// Two open withdrawals against one sub-wallet would promise the same balance
// twice. The database refuses the second.
func TestOnlyOneOpenWithdrawalPerMembership(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	first := walletusecases.WithdrawalInput{Tenant: f.tenant, MembershipID: f.membership,
		RequestedByActorType: walletusecases.RequesterStaff, AmountMinor: 20_000,
		Currency: "NGN", DestinationReference: "bank_ref_1", IdempotencyKey: "w-1"}
	if _, err := f.service.RequestWithdrawal(ctx, first); err != nil {
		t.Fatalf("first withdrawal: %v", err)
	}
	second := first
	second.IdempotencyKey = "w-2"
	second.AmountMinor = 30_000
	if _, err := f.service.RequestWithdrawal(ctx, second); err == nil {
		t.Fatal("a second open withdrawal for one membership must be refused")
	}
}

func TestWithdrawalCannotBeDecidedTwice(t *testing.T) {
	f := newWalletFixture(t, allFlagsOn(), completedWork{})
	ctx := context.Background()
	request, err := f.service.RequestWithdrawal(ctx, walletusecases.WithdrawalInput{
		Tenant: f.tenant, MembershipID: f.membership,
		RequestedByActorType: walletusecases.RequesterStaff, AmountMinor: 20_000,
		Currency: "NGN", DestinationReference: "bank_ref_1", IdempotencyKey: "decide-1"})
	if err != nil {
		t.Fatalf("request withdrawal: %v", err)
	}
	approver := f.tenant
	approver.ActorID = "usr_approver"
	approver.OrganizationUserID = "usr_approver"
	if _, err := f.service.DecideWithdrawal(ctx, approver, f.membership, request.ID,
		request.Version, true, "ok"); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if _, err := f.service.DecideWithdrawal(ctx, approver, f.membership, request.ID,
		request.Version+1, false, "changed my mind"); err == nil {
		t.Fatal("a resolved withdrawal must not accept a second decision")
	}
}

// Fleet must own no ledger and no authoritative balance. This asserts on the
// live schema so a future migration that adds one is caught here.
func TestFleetOwnsNoLocalLedgerOrBalance(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var ledgerTables int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_tables
		WHERE schemaname = 'public'
		  AND (tablename LIKE '%journal%' OR tablename LIKE '%ledger%'
		       OR tablename LIKE '%account_balance%')`).Scan(&ledgerTables); err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	if ledgerTables != 0 {
		t.Fatalf("fleet-partner-service must own no ledger tables, found %d", ledgerTables)
	}

	// No Section 13 table may carry a column that reads as an authoritative
	// balance. Amounts are per-transaction facts, never running totals.
	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name LIKE 'fleet\_partner\_%'
		  AND (column_name LIKE '%balance%' OR column_name = 'available_minor'
		       OR column_name = 'payable_minor' OR column_name = 'held_minor'
		       OR column_name = 'settled_minor')`)
	if err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		t.Errorf("fleet must store no authoritative balance: %s.%s", table, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
}

// Section 15 (private orders, the service catalogue) now exists, built in
// migrations 000025/000026. What this guard protects has moved: a company
// wallet balance must remain Section 13's alone, so no company-wallet table
// may hold a foreign key straight into a Section 15 order or catalogue
// table. Orders reference money through the Wallet service contract, an
// opaque reference, never a local join.
func TestCompanyWalletHasNoDirectSection15ForeignKey(t *testing.T) {
	pool := testPool(t)
	rows, err := pool.Query(context.Background(), `
		SELECT tc.table_name, kcu.column_name, ccu.table_name AS referenced_table
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = 'public'
		  AND tc.table_name LIKE '%company_wallet%'
		  AND ccu.table_name IN ('fleet_partner_private_orders', 'fleet_partner_order_stops',
		                          'fleet_partner_service_catalog_items')`)
	if err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, column, referenced string
		if err := rows.Scan(&table, &column, &referenced); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		t.Errorf("company wallet table %s.%s must not directly reference Section 15 table %s", table, column, referenced)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate constraints: %v", err)
	}
}

// Section 13 writes are gated. With the flags off the routes and use cases
// must refuse rather than quietly operate.
func TestDisabledFlagsRefuseWrites(t *testing.T) {
	f := newWalletFixture(t, walletusecases.Flags{WalletReadEnabled: true, SubWalletEnabled: true},
		completedWork{})
	ctx := context.Background()
	if _, err := f.service.RequestAllocation(ctx, salaryAllocation(f, "off-1")); err == nil {
		t.Fatal("allocations must be refused when the flag is off")
	}
	if _, err := f.service.RequestWithdrawal(ctx, walletusecases.WithdrawalInput{
		Tenant: f.tenant, MembershipID: f.membership, AmountMinor: 1_000, Currency: "NGN",
		DestinationReference: "ref", IdempotencyKey: "off-2"}); err == nil {
		t.Fatal("withdrawals must be refused when the flag is off")
	}
}

// The kill switch overrides every write, including ones whose feature flag is
// on. That is what makes it usable during an incident.
func TestKillSwitchOverridesEnabledFlags(t *testing.T) {
	flags := allFlagsOn()
	flags.KillSwitch = true
	f := newWalletFixture(t, flags, completedWork{})
	if _, err := f.service.RequestAllocation(context.Background(),
		salaryAllocation(f, "kill-1")); err == nil {
		t.Fatal("the kill switch must refuse allocations even with the flag on")
	}
}
