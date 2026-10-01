package integration_test

// Section 12 WAL-029/030: refunding and disputing a corporate Commerce
// delivery.
//
// Database-backed because every property here is a ledger property: who is
// debited, how much, and that two refunds cannot between them take back more
// than was paid out.

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

type settledDelivery struct {
	vendorOrderID   string
	parentReference string
	fingerprint     string
	providerID      string
	organizationID  string
	frozenMinor     int64
}

func snapshotFingerprint(t *testing.T, pool *pgxpool.Pool, vendorOrderID string) string {
	t.Helper()
	var fingerprint string
	if err := pool.QueryRow(context.Background(),
		`SELECT snapshot_fingerprint FROM commerce_payment_allocations WHERE vendor_order_id=$1`,
		vendorOrderID).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

// settleManagedDelivery seeds a funded platform delivery and settles it through
// Fleet with the given split.
func settleManagedDelivery(t *testing.T, pool *pgxpool.Pool, repo *walletrepositories.PostgresWalletRepository,
	frozen, providerShare, organizationShare int64) settledDelivery {
	t.Helper()
	ctx := context.Background()
	vendorOrderID, parentReference := platformDeliveryAllocation(t, pool, frozen)
	providerID, organizationID := uniqueID("rider"), uniqueID("org")

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorManaged,
		OrganizationID: organizationID, MembershipID: "mem_1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseManagedCommerceDelivery(ctx, walletrepositories.ReleaseManagedCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, CompletionReference: "mission_1",
		ProviderType: "dispatch", ProviderID: providerID,
		OrganizationID: organizationID, MembershipID: "mem_1",
		ProviderShareMinor: providerShare, OrganizationShareMinor: organizationShare,
		InstructionID: "msi_1", InstructionVersion: 1,
		IdempotencyKey: "settle-" + vendorOrderID,
	}); err != nil {
		t.Fatal(err)
	}
	return settledDelivery{
		vendorOrderID: vendorOrderID, parentReference: parentReference,
		fingerprint: snapshotFingerprint(t, pool, vendorOrderID),
		providerID:  providerID, organizationID: organizationID, frozenMinor: frozen,
	}
}

// settleIndependentDelivery seeds and settles a delivery the legacy way.
func settleIndependentDelivery(t *testing.T, pool *pgxpool.Pool, repo *walletrepositories.PostgresWalletRepository,
	frozen int64) settledDelivery {
	t.Helper()
	ctx := context.Background()
	vendorOrderID, parentReference := platformDeliveryAllocation(t, pool, frozen)
	providerID := uniqueID("rider_indep")

	if err := repo.SetCommerceDeliveryExecutor(ctx, walletrepositories.SetCommerceDeliveryExecutorInput{
		VendorOrderID: vendorOrderID, ExecutorMode: walletmodels.ExecutorIndependent,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReleaseCommerceDelivery(ctx, walletrepositories.ReleaseCommerceDeliveryInput{
		VendorOrderID: vendorOrderID, ProviderType: "dispatch", ProviderID: providerID,
		CompletionReference: "mission_1", IdempotencyKey: "settle-indep-" + vendorOrderID,
		TrustedServiceCompletion: true,
	}); err != nil {
		t.Fatal(err)
	}
	return settledDelivery{
		vendorOrderID: vendorOrderID, parentReference: parentReference,
		fingerprint: snapshotFingerprint(t, pool, vendorOrderID),
		providerID:  providerID, frozenMinor: frozen,
	}
}

// deliveryRefund builds a refund that takes `amount` of delivery money back
// from the parties named, and returns it to the customer.
func deliveryRefund(delivery settledDelivery, kind, item string, amount, providerRecovery, organizationRecovery int64) walletrepositories.RefundCommerceAllocationInput {
	return walletrepositories.RefundCommerceAllocationInput{
		VendorOrderID: delivery.vendorOrderID, ItemReference: item, Kind: kind,
		CustomerRefundMinor:            amount,
		ProviderRecoveryMinor:          providerRecovery,
		OrganizationRecoveryMinor:      organizationRecovery,
		ProviderDeliveryReductionMinor: amount,
		ExpectedSnapshotFingerprint:    delivery.fingerprint,
		IdempotencyKey:                 kind + "-" + item + "-" + delivery.vendorOrderID,
	}
}

// Requirement 8: an independent delivery behaves exactly as it did.
func TestIndependentDeliveryRefundStillRecoversFromTheProvider(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleIndependentDelivery(t, pool, repo, 5000)

	if _, err := repo.RefundCommerceAllocation(ctx,
		deliveryRefund(delivery, walletrepositories.RefundKindRefund, "item_1", 2000, 2000, 0)); err != nil {
		t.Fatal(err)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 3000 {
		t.Fatalf("independent provider holds %d after a 2000 refund of 5000, want 3000", balance)
	}
}

// Requirement 2/3: the provider gives back only their own share.
func TestManagedRefundRecoversFromBothPartiesInProportion(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	// A 1000 refund of a 1500/3500 split is 300 from the provider and 700 from
	// the organization.
	plan, err := repo.PlanDeliveryRecovery(ctx, delivery.vendorOrderID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ProviderMinor != 300 || plan.OrganizationMinor != 700 {
		t.Fatalf("plan = %+v, want 300/700", plan)
	}
	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 1000, plan.ProviderMinor, plan.OrganizationMinor)); err != nil {
		t.Fatal(err)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 1200 {
		t.Fatalf("provider holds %d, want 1500-300=1200", balance)
	}
	if balance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable"); balance != 2800 {
		t.Fatalf("organization holds %d, want 3500-700=2800", balance)
	}
}

// Requirement 9, the one that matters most. A salaried rider received nothing,
// so a refund must take nothing from them.
func TestASalariedRidersRefundTakesNothingFromTheRider(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 0, 5000)

	plan, err := repo.PlanDeliveryRecovery(ctx, delivery.vendorOrderID, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ProviderMinor != 0 {
		t.Fatalf("plan takes %d from a rider who received nothing", plan.ProviderMinor)
	}
	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 5000, 0, 5000)); err != nil {
		t.Fatal(err)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 0 {
		t.Fatalf("a salaried rider's balance moved to %d", balance)
	}
	if balance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable"); balance != 0 {
		t.Fatalf("organization holds %d after giving back everything, want 0", balance)
	}
}

// Recovering the organization's share from the rider is the mistake this whole
// section exists to prevent, so it is refused outright.
func TestARefundCannotTakeTheOrganizationsShareFromTheRider(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 0, 5000)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 5000, 5000, 0)); err == nil {
		t.Fatal("a refund clawed the organization's share out of a salaried rider")
	}
}

// Requirement 14: repeated partial refunds can never exceed what was paid out.
func TestRepeatedPartialRefundsNeverExceedWhatWasPaid(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	var recovered int64
	for step := 0; step < 4; step++ {
		plan, err := repo.PlanDeliveryRecovery(ctx, delivery.vendorOrderID, 1250)
		if err != nil {
			t.Fatalf("step %d plan: %v", step, err)
		}
		if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
			walletrepositories.RefundKindRefund, uniqueID("item"), 1250,
			plan.ProviderMinor, plan.OrganizationMinor)); err != nil {
			t.Fatalf("step %d refund: %v", step, err)
		}
		recovered += 1250
	}
	if recovered != 5000 {
		t.Fatalf("recovered %d, want the full 5000", recovered)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 0 {
		t.Fatalf("provider holds %d after the delivery was fully refunded", balance)
	}
	if balance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable"); balance != 0 {
		t.Fatalf("organization holds %d after the delivery was fully refunded", balance)
	}

	// A fifth refund has nothing left to take.
	if _, err := repo.PlanDeliveryRecovery(ctx, delivery.vendorOrderID, 1); err == nil {
		t.Fatal("a refund was planned against an exhausted pool")
	}
}

// Requirement 11/12: a retry must not double-recover.
func TestRetryingARefundDoesNotDoubleRecover(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)
	refund := deliveryRefund(delivery, walletrepositories.RefundKindRefund, "item_1", 1000, 300, 700)

	for attempt := 0; attempt < 4; attempt++ {
		if _, err := repo.RefundCommerceAllocation(ctx, refund); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 1200 {
		t.Fatalf("four identical refunds left the provider at %d, want 1200", balance)
	}
	if balance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable"); balance != 2800 {
		t.Fatalf("four identical refunds left the organization at %d, want 2800", balance)
	}
}

// Requirement 16: concurrent refunds must not both see the full pool.
func TestConcurrentRefundsCannotBothRecoverTheSameMoney(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	// Two refunds that are each individually valid but cannot both succeed:
	// together they would recover 6000 from a 5000 pool.
	first := deliveryRefund(delivery, walletrepositories.RefundKindRefund, "item_a", 3000, 900, 2100)
	second := deliveryRefund(delivery, walletrepositories.RefundKindRefund, "item_b", 3000, 900, 2100)

	var wait sync.WaitGroup
	results := make([]error, 2)
	for index, refund := range []walletrepositories.RefundCommerceAllocationInput{first, second} {
		wait.Add(1)
		go func(index int, refund walletrepositories.RefundCommerceAllocationInput) {
			defer wait.Done()
			_, results[index] = repo.RefundCommerceAllocation(ctx, refund)
		}(index, refund)
	}
	wait.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of two conflicting refunds succeeded, want exactly 1 (%v)", succeeded, results)
	}
	providerBalance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable")
	organizationBalance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable")
	if providerBalance < 0 || organizationBalance < 0 {
		t.Fatalf("a concurrent refund drove a balance negative: provider %d organization %d",
			providerBalance, organizationBalance)
	}
	if providerBalance+organizationBalance != 2000 {
		t.Fatalf("parties hold %d in total after one 3000 refund of 5000, want 2000",
			providerBalance+organizationBalance)
	}
}

// WAL-030. A dispute reversal moves money the same way and is its own
// operation, so it does not collide with a refund on the same item.
func TestDisputeReversalRecoversFromBothParties(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindDispute, "item_1", 1000, 300, 700)); err != nil {
		t.Fatal(err)
	}
	if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 1200 {
		t.Fatalf("provider holds %d after a dispute reversal, want 1200", balance)
	}

	var operationType string
	if err := pool.QueryRow(ctx, `
		SELECT operation_type FROM commerce_financial_operations
		WHERE source_reference=$1 AND item_reference='item_1'`,
		delivery.vendorOrderID).Scan(&operationType); err != nil {
		t.Fatal(err)
	}
	if operationType != "DISPUTE_REVERSAL" {
		t.Fatalf("operation recorded as %q, want DISPUTE_REVERSAL", operationType)
	}
}

// Requirement 15: the two must share one pool, in either order.
func TestARefundAndADisputeShareOneRecoveryPool(t *testing.T) {
	for _, order := range []struct {
		name  string
		first string
		then  string
	}{
		{"refund then dispute", walletrepositories.RefundKindRefund, walletrepositories.RefundKindDispute},
		{"dispute then refund", walletrepositories.RefundKindDispute, walletrepositories.RefundKindRefund},
	} {
		t.Run(order.name, func(t *testing.T) {
			pool, repo := commerceRepo(t)
			ctx := context.Background()
			delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

			if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
				order.first, "item_1", 3000, 900, 2100)); err != nil {
				t.Fatal(err)
			}
			// 2000 remains. Asking for 3000 must be refused, not netted off
			// against a pool the second operation thinks is still whole.
			if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
				order.then, "item_2", 3000, 900, 2100)); err == nil {
				t.Fatal("the second operation recovered money the first had already taken")
			}
			// What does remain is recoverable.
			if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
				order.then, "item_3", 2000, 600, 1400)); err != nil {
				t.Fatalf("the remaining pool was refused: %v", err)
			}
			if balance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); balance != 0 {
				t.Fatalf("provider holds %d after the pool was exhausted", balance)
			}
		})
	}
}

// Requirement 4: the reversal must reconcile exactly.
func TestARecoveryThatDoesNotReconcileIsRefused(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	// Recovers 1000 but returns 900 to the customer: 100 would vanish.
	unbalanced := deliveryRefund(delivery, walletrepositories.RefundKindRefund, "item_1", 900, 300, 700)
	if _, err := repo.RefundCommerceAllocation(ctx, unbalanced); err == nil {
		t.Fatal("a refund that did not reconcile was accepted")
	}
}

// Requirement 6/7: rounding must not create or destroy money, end to end.
func TestAnAwkwardlyRoundedRefundStillReconciles(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	// 1/3 : 2/3 does not divide evenly at almost any refund size.
	delivery := settleManagedDelivery(t, pool, repo, 3000, 1000, 2000)

	plan, err := repo.PlanDeliveryRecovery(ctx, delivery.vendorOrderID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TotalMinor() != 1000 {
		t.Fatalf("plan totals %d, want exactly 1000", plan.TotalMinor())
	}
	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 1000, plan.ProviderMinor, plan.OrganizationMinor)); err != nil {
		t.Fatal(err)
	}
	providerBalance := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable")
	organizationBalance := accountBalance(t, pool, "organization", delivery.organizationID, "organization_payable")
	if providerBalance+organizationBalance != 2000 {
		t.Fatalf("parties hold %d after a 1000 refund of 3000, want 2000", providerBalance+organizationBalance)
	}
}

// Requirement: an independent delivery has no organization to recover from.
func TestAnIndependentDeliveryRefusesOrganizationRecovery(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleIndependentDelivery(t, pool, repo, 5000)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 1000, 500, 500)); err == nil {
		t.Fatal("an independent delivery recovered from an organization")
	}
}

// Requirement 10: the recovery is bounded by the durable evidence of what was
// paid, not by the customer-facing delivery payable.
func TestRecoveryIsBoundedByWhatWasActuallyPaid(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 5000, 1501, 3499)); err == nil {
		t.Fatal("a refund recovered more from the provider than the provider was paid")
	}
	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_2", 5000, 1499, 3501)); err == nil {
		t.Fatal("a refund recovered more from the organization than it was paid")
	}
}

// Requirement 1: recovery must address the right order. A plan for an order
// that does not exist must fail rather than quietly return zero.
func TestARecoveryPlanForAnUnknownOrderFails(t *testing.T) {
	_, repo := commerceRepo(t)
	if _, err := repo.PlanDeliveryRecovery(context.Background(), "vo_does_not_exist", 100); err == nil {
		t.Fatal("a recovery was planned against an order that does not exist")
	}
}

// Requirement 17: a refused refund must leave nothing behind.
func TestARefusedRefundMovesNothingAndRecordsNothing(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	before := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable")
	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 9000, 1500, 7500)); err == nil {
		t.Fatal("an over-recovery was accepted")
	}
	if after := accountBalance(t, pool, "provider", "dispatch:"+delivery.providerID, "provider_payable"); after != before {
		t.Fatalf("a refused refund moved the provider balance from %d to %d", before, after)
	}
	var operations int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_financial_operations WHERE source_reference=$1 AND item_reference='item_1'`,
		delivery.vendorOrderID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("a refused refund recorded %d operation(s)", operations)
	}
	var recovered int64
	if err := pool.QueryRow(ctx,
		`SELECT refunded_provider_minor FROM commerce_delivery_executions WHERE vendor_order_id=$1`,
		delivery.vendorOrderID).Scan(&recovered); err != nil {
		t.Fatal(err)
	}
	if recovered != 0 {
		t.Fatalf("a refused refund recorded %d as recovered", recovered)
	}
}

// Requirement 18: the reversal is announced durably, in the same transaction.
func TestARefundEmitsItsEvidenceWithTheMoney(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindDispute, "item_1", 1000, 300, 700)); err != nil {
		t.Fatal(err)
	}
	var events int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM finance_outbox
		WHERE event_type = 'commerce.dispute_reversal.created.v1'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events == 0 {
		t.Fatal("a dispute reversal moved money and announced nothing")
	}
}

// The per-mission evidence must record what each mission gave back, so a
// second refund knows what is left.
func TestRecoveryIsAttributedToTheMissionThatWasPaid(t *testing.T) {
	pool, repo := commerceRepo(t)
	ctx := context.Background()
	delivery := settleManagedDelivery(t, pool, repo, 5000, 1500, 3500)

	if _, err := repo.RefundCommerceAllocation(ctx, deliveryRefund(delivery,
		walletrepositories.RefundKindRefund, "item_1", 1000, 300, 700)); err != nil {
		t.Fatal(err)
	}
	executions, err := repo.ListDeliveryExecutions(ctx, delivery.vendorOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 {
		t.Fatalf("%d executions, want 1", len(executions))
	}
	if executions[0].RefundedProviderMinor != 300 || executions[0].RefundedOrganizationMinor != 700 {
		t.Fatalf("execution recorded %d/%d recovered, want 300/700",
			executions[0].RefundedProviderMinor, executions[0].RefundedOrganizationMinor)
	}
}
