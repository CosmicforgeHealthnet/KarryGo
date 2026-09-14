package integration_test

// Section 12 Part B: corporate delivery inside Commerce.
//
// Database-backed because the property being certified is a ledger property:
// the customer's frozen delivery amount leaves escrow exactly once and is
// split between the provider and the organization according to Fleet's
// decision -- and the customer's own amount never changes because a corporate
// rider took the job.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

// platformDeliveryAllocation seeds one funded Commerce allocation whose
// delivery is executed by the platform, with escrow actually holding the money.
//
// deliveryMinor is the customer's FROZEN delivery amount: what they approved at
// quote time and what must still leave escrow whoever ends up executing.
func platformDeliveryAllocation(t *testing.T, pool *pgxpool.Pool, deliveryMinor int64) (vendorOrderID, parentReference string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()
	paymentID := uuid.New()
	vendorOrderID = "vo_managed_" + suffix
	parentReference = "parent_" + suffix

	if _, err := pool.Exec(ctx, `INSERT INTO payment_intents
			(id,reference,source_service,source_reference,customer_id,amount_kobo,platform_fee_kobo,currency,payment_method,status)
		VALUES($1,$2,'vendor-service',$3,$4,$5,0,'NGN','wallet','held')`,
		paymentID, "pi_"+suffix, parentReference, "customer_"+suffix, deliveryMinor); err != nil {
		t.Fatalf("seed payment intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_payment_intents
			(payment_intent_id,parent_commerce_reference,quote_reference,quote_version,quote_fingerprint,final_authorized_minor,max_authorized_minor,allocation_fingerprint,status,idempotency_key,request_fingerprint)
		VALUES($1,$2,$3,1,$4,$5,$5,$6,'HELD',$7,$8)`, paymentID, parentReference, "quote_"+suffix,
		"quote-fingerprint-"+suffix, deliveryMinor, "allocation-fingerprint-"+suffix,
		"intent-key-"+suffix, "intent-fingerprint-"+suffix); err != nil {
		t.Fatalf("seed commerce intent: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_payment_allocations
			(id,payment_intent_id,allocation_reference,scope,vendor_order_id,vendor_id,branch_id,delivery_responsibility,
			 effective_delivery_responsibility,effective_vendor_delivery_payable_minor,effective_provider_delivery_payable_minor,
			 currency,customer_payable_minor,max_authorized_minor,delivery_customer_minor,vendor_delivery_payable_minor,
			 provider_delivery_payable_minor,snapshot,snapshot_fingerprint,status)
		VALUES($1,$2,$3,'VENDOR',$4,$5,$6,'PLATFORM','PLATFORM',0,$7,'NGN',$7,$7,$7,0,$7,'{}',$8,'HELD')`,
		"allocation_"+suffix, paymentID, "vendor-allocation-"+suffix, vendorOrderID,
		"vendor_"+suffix, "branch_"+suffix, deliveryMinor, "snapshot-"+suffix); err != nil {
		t.Fatalf("seed platform delivery allocation: %v", err)
	}

	// Fund the commerce escrow so a release has something to release.
	escrowID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance)
		VALUES ($1,'job',$2,'commerce_escrow','NGN','credit')`,
		escrowID, "commerce:"+parentReference); err != nil {
		t.Fatalf("seed commerce escrow account: %v", err)
	}
	transactionID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO ledger_transactions
			(id,reference,transaction_type,status,source_service,source_reference,idempotency_key)
		VALUES($1,$2,'commerce_funding','posted','vendor-service',$3,$4)`,
		transactionID, "ldt_"+suffix, vendorOrderID, "fund-"+suffix); err != nil {
		t.Fatalf("seed funding transaction: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, memo)
		VALUES($1,$2,'credit',$3,'NGN','Commerce escrow funded')`,
		transactionID, escrowID, deliveryMinor); err != nil {
		t.Fatalf("fund commerce escrow: %v", err)
	}
	return vendorOrderID, parentReference
}

// uniqueID makes a per-run identity.
//
// testPool truncates only the finance_% tables, so commerce ledger entries
// survive between runs. A reused provider or organization id would sum this
// run's credit with the last one's and the assertion would be meaningless.
func uniqueID(prefix string) string { return prefix + "_" + uuid.NewString()[:8] }

func commerceRepo(t *testing.T) (*pgxpool.Pool, *walletrepositories.PostgresWalletRepository) {
	t.Helper()
	pool := testPool(t)
	return pool, walletrepositories.NewPostgresWalletRepository(pool)
}

func accountBalance(t *testing.T, pool *pgxpool.Pool, ownerType, ownerID, accountType string) int64 {
	t.Helper()
	var balance int64
	err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(CASE WHEN e.side='credit' THEN e.amount_kobo ELSE -e.amount_kobo END), 0)
		FROM ledger_entries e
		JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.owner_type=$1 AND a.owner_id=$2 AND a.account_type=$3`,
		ownerType, ownerID, accountType).Scan(&balance)
	if err != nil {
		t.Fatal(err)
	}
	return balance
}

// WAL-021/022. A corporate rider did not earn the customer's delivery fee.
// Crediting it to them directly would pay a salaried rider per delivery and
// never pay the organization what it is owed, and neither is recoverable.
func TestManagedCommerceDeliveryCannotBePaidDirectlyToTheProvider(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: "org_1", MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := repo.ReleaseCommerceDelivery(ctx, walletrepositories.ReleaseCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, ProviderType: "dispatch", ProviderID: "rider_1",
		CompletionReference: "mission_1", IdempotencyKey: "direct-" + vendorOrderID,
		TrustedServiceCompletion: true,
	})
	if err == nil {
		t.Fatal("a corporate delivery was paid directly to the rider")
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:rider_1", "provider_payable"); balance != 0 {
		t.Fatalf("the rider was credited %d despite the refusal", balance)
	}
}

// UNKNOWN is not a synonym for INDEPENDENT. A refused release can be retried
// once the executor is known; a payment to the wrong party cannot be undone.
func TestDeliveryWithAnUnknownExecutorIsRefusedRatherThanAssumedIndependent(t *testing.T) {
	pool, repo := commerceRepo(t)
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	_, err := repo.ReleaseCommerceDelivery(context.Background(), walletrepositories.ReleaseCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, ProviderType: "dispatch", ProviderID: "rider_1",
		CompletionReference: "mission_1", IdempotencyKey: "unknown-" + vendorOrderID,
		TrustedServiceCompletion: true,
	})
	if err == nil {
		t.Fatal("a delivery with no known executor was released anyway")
	}
}

// WAL-025. Nothing about an independent delivery changes.
func TestIndependentCommerceDeliveryStillPaysTheProviderInFull(t *testing.T) {
	pool, repo := commerceRepo(t)
	riderID := uniqueID("rider_indep")
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorIndependent,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseCommerceDelivery(ctx, walletrepositories.ReleaseCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, ProviderType: "dispatch", ProviderID: riderID,
		CompletionReference: "mission_1", IdempotencyKey: "indep-" + vendorOrderID,
		TrustedServiceCompletion: true,
	}); err != nil {
		t.Fatalf("an independent delivery was refused: %v", err)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+riderID, "provider_payable"); balance != 5000 {
		t.Fatalf("independent rider was credited %d, want the full frozen 5000", balance)
	}
}

// WAL-023. A salaried rider is paid by their employer, not per delivery. No
// provider line is written at all -- a token credit would make them look like
// they earned per job.
func TestSalariedCommerceRiderReceivesNothingPerDelivery(t *testing.T) {
	pool, repo := commerceRepo(t)
	riderID, orgID := uniqueID("rider_salaried"), uniqueID("org_salary")
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: orgID, MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: riderID,
		OrganizationID: orgID, MembershipID: "mem_1",
		ProviderShareMinor: 0, OrganizationShareMinor: 5000,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "salary-" + vendorOrderID,
	}); err != nil {
		t.Fatal(err)
	}

	if balance := accountBalance(t, pool, "provider", "dispatch:"+riderID, "provider_payable"); balance != 0 {
		t.Fatalf("a salaried rider was credited %d per delivery", balance)
	}
	if balance := accountBalance(t, pool, "organization", orgID, "organization_payable"); balance != 5000 {
		t.Fatalf("organization was credited %d, want the whole frozen 5000", balance)
	}
	var providerLines int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM ledger_entries e JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.owner_type='provider' AND a.owner_id=$1`, "dispatch:"+riderID).Scan(&providerLines); err != nil {
		t.Fatal(err)
	}
	if providerLines != 0 {
		t.Fatalf("a zero provider share still wrote %d ledger line(s)", providerLines)
	}
}

// WAL-024. A commission rider gets exactly what Fleet authorised and not a
// unit more; the remainder is the organization's.
func TestCommissionCommerceRiderGetsOnlyTheAuthorisedShare(t *testing.T) {
	pool, repo := commerceRepo(t)
	riderID, orgID := uniqueID("rider_comm"), uniqueID("org_comm")
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: orgID, MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: riderID,
		OrganizationID: orgID, MembershipID: "mem_1",
		ProviderShareMinor: 1500, OrganizationShareMinor: 3500,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "comm-" + vendorOrderID,
	}); err != nil {
		t.Fatal(err)
	}

	if balance := accountBalance(t, pool, "provider", "dispatch:"+riderID, "provider_payable"); balance != 1500 {
		t.Fatalf("commission rider credited %d, want exactly the authorised 1500", balance)
	}
	if balance := accountBalance(t, pool, "organization", orgID, "organization_payable"); balance != 3500 {
		t.Fatalf("organization credited %d, want 3500", balance)
	}
}

// WAL-020, and the invariant that matters most: the customer approved a
// delivery amount at quote time. Assigning a corporate rider must not change
// what leaves their escrow.
func TestTheCustomersFrozenDeliveryAmountIsUnchangedByTheExecutor(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	const frozen = 5000

	for _, scenario := range []struct {
		name              string
		providerShare     int64
		organizationShare int64
		providerID        string
		organizationID    string
	}{
		{"salaried", 0, frozen, uniqueID("rider_a"), uniqueID("org_a")},
		{"commission", 1500, frozen - 1500, uniqueID("rider_b"), uniqueID("org_b")},
		{"full to provider", frozen, 0, uniqueID("rider_c"), uniqueID("org_c")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			vendorOrderID, parentReference := platformDeliveryAllocation(t, pool, frozen)
			if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
				VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
				OrganizationID: scenario.organizationID, MembershipID: "mem_1",
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
				VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
				ProviderType: "dispatch", ProviderID: scenario.providerID,
				OrganizationID: scenario.organizationID, MembershipID: "mem_1",
				ProviderShareMinor: scenario.providerShare, OrganizationShareMinor: scenario.organizationShare,
				InstructionID: "msi_1", InstructionVersion: 1,
				IdempotencyKey: "frozen-" + vendorOrderID,
			}); err != nil {
				t.Fatal(err)
			}
			// Whatever the split, exactly the frozen amount left escrow.
			if balance := accountBalance(t, pool, "job", "commerce:"+parentReference, "commerce_escrow"); balance != 0 {
				t.Fatalf("escrow holds %d after release, want 0 -- the customer's frozen amount changed", balance)
			}
		})
	}
}

// A split that does not add up to the frozen amount invents or drops money.
func TestASplitThatDoesNotSumToTheFrozenAmountIsRefused(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()

	for _, scenario := range []struct {
		name                             string
		providerShare, organizationShare int64
	}{
		{"drops a residual", 1000, 3000},
		{"invents money", 3000, 3000},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)
			if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
				VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
				OrganizationID: "org_x", MembershipID: "mem_1",
			}); err != nil {
				t.Fatal(err)
			}
			_, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
				VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
				ProviderType: "dispatch", ProviderID: "rider_x",
				OrganizationID: "org_x", MembershipID: "mem_1",
				ProviderShareMinor: scenario.providerShare, OrganizationShareMinor: scenario.organizationShare,
				InstructionID: "msi_1", InstructionVersion: 1,
				IdempotencyKey: "bad-" + vendorOrderID,
			})
			if err == nil {
				t.Fatal("a split that did not reconcile with the frozen amount was accepted")
			}
		})
	}
}

// WAL-028. A replayed instruction returns what the first one did rather than
// paying twice.
func TestReplayingAManagedCommerceReleasePaysOnce(t *testing.T) {
	pool, repo := commerceRepo(t)
	riderID, orgID := uniqueID("rider_replay"), uniqueID("org_replay")
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: orgID, MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	input := walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: riderID,
		OrganizationID: orgID, MembershipID: "mem_1",
		ProviderShareMinor: 1500, OrganizationShareMinor: 3500,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "replay-" + vendorOrderID,
	}
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := repo.ReleaseManagedCommerceDelivery(ctx, input); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+riderID, "provider_payable"); balance != 1500 {
		t.Fatalf("three sends credited the rider %d, want 1500", balance)
	}
	if balance := accountBalance(t, pool, "organization", orgID, "organization_payable"); balance != 3500 {
		t.Fatalf("three sends credited the organization %d, want 3500", balance)
	}
}

// The same mission settled with a different split is a conflict, not a repeat.
// Collapsing the two is how a changed instruction gets silently ignored.
func TestTheSameMissionWithADifferentSplitIsAConflict(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: "org_conflict", MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	base := walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: "rider_conflict",
		OrganizationID: "org_conflict", MembershipID: "mem_1",
		ProviderShareMinor: 1500, OrganizationShareMinor: 3500,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "conflict-" + vendorOrderID,
	}
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, base); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.ProviderShareMinor, changed.OrganizationShareMinor = 2500, 2500
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, changed); err == nil {
		t.Fatal("a mission was re-settled with a different split")
	}
}

// WAL-027. The merchant's own payout is a separate release and is not touched
// by how the delivery was executed or split.
func TestTheMerchantPayoutIsUnaffectedByTheDeliveryExecutor(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: "org_merchant", MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := pool.QueryRow(ctx,
		`SELECT vendor_merchandise_payable_minor FROM commerce_payment_allocations WHERE vendor_order_id=$1`,
		vendorOrderID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: "rider_m",
		OrganizationID: "org_merchant", MembershipID: "mem_1",
		ProviderShareMinor: 1500, OrganizationShareMinor: 3500,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "merchant-" + vendorOrderID,
	}); err != nil {
		t.Fatal(err)
	}
	var after, vendorReleased int64
	if err := pool.QueryRow(ctx,
		`SELECT vendor_merchandise_payable_minor, vendor_released_minor
		 FROM commerce_payment_allocations WHERE vendor_order_id=$1`,
		vendorOrderID).Scan(&after, &vendorReleased); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("the merchant's payable moved from %d to %d when the delivery settled", before, after)
	}
	if vendorReleased != 0 {
		t.Fatalf("the delivery release paid the merchant %d", vendorReleased)
	}
}

// WAL-026. One vendor order can be delivered by more than one mission. The
// allocation's single assigned_provider pair records whoever was released
// last, so the executions are what answer "who actually delivered this?".
func TestOneOrderCanBeDeliveredByMoreThanOneMission(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: "org_multi", MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	// First mission takes part of the frozen amount.
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: "rider_one",
		OrganizationID: "org_multi", MembershipID: "mem_1",
		ProviderShareMinor: 1000, OrganizationShareMinor: 4000,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "multi-1-" + vendorOrderID,
	}); err != nil {
		t.Fatal(err)
	}

	executions, err := repo.ListDeliveryExecutions(ctx, vendorOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 {
		t.Fatalf("%d executions recorded, want 1", len(executions))
	}
	if executions[0].ProviderID != "rider_one" || executions[0].CompletionReference != "mission_1" {
		t.Fatalf("execution = %+v", executions[0])
	}
	if executions[0].ProviderShareMinor+executions[0].OrganizationShareMinor != executions[0].FrozenDeliveryMinor {
		t.Fatalf("execution split does not reconcile: %+v", executions[0])
	}
}

// Changing the executor after the money has moved would mean the release was
// decided against a different fact than the one now recorded. Corrections go
// through a reversal, not an edit.
func TestTheExecutorCannotBeChangedAfterTheMoneyMoved(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorIndependent,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseCommerceDelivery(ctx, walletrepositories.ReleaseCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, ProviderType: "dispatch", ProviderID: "rider_fixed",
		CompletionReference: "mission_1", IdempotencyKey: "fixed-" + vendorOrderID,
		TrustedServiceCompletion: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: "org_late", MembershipID: "mem_1",
	}); err == nil {
		t.Fatal("the executor was changed after the delivery had already been paid")
	}
}

// A managed executor must name its organization; an independent one must not
// carry a phantom organization the money would otherwise be owed to.
func TestExecutorModeRequiresTheRightIdentity(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	vendorOrderID, _ := platformDeliveryAllocation(t, pool, 5000)

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
	}); err == nil {
		t.Fatal("a managed executor was accepted with no organization")
	}
	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorIndependent,
		OrganizationID: "org_phantom", MembershipID: "mem_1",
	}); err == nil {
		t.Fatal("an independent executor was accepted with an organization")
	}
	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: "SOMETHING_ELSE",
	}); err == nil {
		t.Fatal("an unrecognised executor mode was accepted")
	}
}
