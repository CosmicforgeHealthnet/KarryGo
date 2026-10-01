package integration_test

// Section 12: PostgreSQL-backed certification of Fleet-managed work settlement.
//
// These run against the real ledger because the properties that matter here
// are properties of the database: that one instruction posts one journal even
// when it is sent twice, that the journal balances, that a salaried provider
// gets no per-job credit, and that the legacy direct-payout path cannot reach
// a managed job. None of that is provable against a mock.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

// walletLedgerPool gives each test a clean wallet ledger.
//
// It cleans the wallet-ledger tables rather than the finance_% ones the
// finance suite owns: a managed settlement releases a JOB ESCROW, which lives
// in this ledger.
func walletLedgerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	rawURL := os.Getenv("PAYMENT_WALLET_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("PAYMENT_WALLET_TEST_DATABASE_URL not set; skipping managed settlement certification")
	}
	requireTestDatabaseName(t, rawURL)

	pool, err := database.NewPool(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	migrateOnce.Do(func() {
		migrateErr = database.ApplyMigrations(context.Background(), pool, migrationsDir)
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE managed_settlements, ledger_entries, ledger_transactions, wallet_accounts, payment_intents, finance_outbox CASCADE`,
	); err != nil {
		t.Fatalf("clean wallet ledger: %v", err)
	}
	return pool
}

// heldJob creates a customer payment already held in escrow for one job.
//
// This is the state every completion path starts from: the customer has paid,
// the money is in the job's escrow account, and nobody has been credited yet.
func heldJob(t *testing.T, pool *pgxpool.Pool, sourceService, jobID string, amountMinor, platformFeeMinor int64) string {
	t.Helper()
	ctx := context.Background()
	intentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO payment_intents (
			id, reference, source_service, source_reference, customer_id,
			amount_kobo, platform_fee_kobo, currency, payment_method, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'NGN','wallet','held')`,
		intentID, "pay_"+intentID, sourceService, jobID, uuid.NewString(),
		amountMinor, platformFeeMinor,
	); err != nil {
		t.Fatalf("seed held payment: %v", err)
	}
	escrowID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance)
		VALUES ($1, 'job', $2, 'job_escrow', 'NGN', 'credit')
		ON CONFLICT (owner_type, owner_id, account_type, currency) DO NOTHING`,
		escrowID, sourceService+":"+jobID,
	); err != nil {
		t.Fatalf("seed escrow account: %v", err)
	}
	return intentID
}

func accountBalanceMinor(t *testing.T, pool *pgxpool.Pool, ownerType, ownerID, accountType string) int64 {
	t.Helper()
	var balance *int64
	// Credit-normal accounts: credits increase, debits decrease.
	err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(CASE WHEN e.side = 'credit' THEN e.amount_kobo ELSE -e.amount_kobo END), 0)
		FROM ledger_entries e
		JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.owner_type = $1 AND a.owner_id = $2 AND a.account_type = $3`,
		ownerType, ownerID, accountType).Scan(&balance)
	if err != nil {
		t.Fatalf("read %s balance: %v", accountType, err)
	}
	if balance == nil {
		return 0
	}
	return *balance
}

func journalCount(t *testing.T, pool *pgxpool.Pool, transactionType string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_transactions WHERE transaction_type = $1`, transactionType,
	).Scan(&count); err != nil {
		t.Fatalf("count journals: %v", err)
	}
	return count
}

// assertEveryJournalBalances is the invariant the whole ledger rests on.
func assertEveryJournalBalances(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT t.id,
		       COALESCE(SUM(CASE WHEN e.side = 'debit'  THEN e.amount_kobo ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN e.side = 'credit' THEN e.amount_kobo ELSE 0 END), 0)
		FROM ledger_transactions t
		JOIN ledger_entries e ON e.transaction_id = t.id
		GROUP BY t.id`)
	if err != nil {
		t.Fatalf("read journals: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var debits, credits int64
		if err := rows.Scan(&id, &debits, &credits); err != nil {
			t.Fatalf("scan journal: %v", err)
		}
		if debits != credits {
			t.Fatalf("journal %s does not balance: debits=%d credits=%d", id, debits, credits)
		}
	}
}

func commissionInstruction(jobID string) walletmodels.ManagedSettlementInstruction {
	// A commission provider on a 10,000 minor fare: the platform already took
	// 1,000 at payment time, leaving 9,000 distributable. Fleet resolved
	// 1,500 to the provider and 7,500 to the organization.
	return walletmodels.ManagedSettlementInstruction{
		InstructionID: "inst_" + jobID, InstructionVersion: 1,
		OrganizationID: "org_1", MembershipID: "mem_1",
		ProviderID: "prov_1", ProviderType: "taxi",
		SourceService: "taxi", SourceJobID: jobID,
		Currency: "NGN", GrossBasisMinor: 9000,
		ProviderShareMinor: 1500, OrganizationShareMinor: 7500,
		CompensationPlanID: "plan_1", CompensationPlanVersion: 3,
		CompensationResolutionVersion: 1,
		CorrelationID:                 "corr_" + jobID,
		IdempotencyKey:                "settlement:" + jobID,
	}
}

// -----------------------------------------------------------------------------
// FIN-03 / WAL-005 / WAL-009 — a commission provider's settlement
// -----------------------------------------------------------------------------

func TestManagedSettlementPostsOneBalancedJournalForACommissionProvider(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_commission"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)

	settlement, err := repo.PostManagedSettlement(context.Background(), commissionInstruction(jobID))
	if err != nil {
		t.Fatalf("post managed settlement: %v", err)
	}
	if settlement.Status != walletmodels.ManagedSettlementPosted {
		t.Fatalf("status=%q, want POSTED", settlement.Status)
	}
	if settlement.PlatformShareMinor != 1000 {
		t.Fatalf("platform share=%d, want the authorized 1000", settlement.PlatformShareMinor)
	}
	if got := settlement.TotalAllocatedMinor(); got != 10000 {
		t.Fatalf("allocated %d of a 10000 settleable amount: money was dropped or invented", got)
	}

	assertEveryJournalBalances(t, pool)
	if got := accountBalanceMinor(t, pool, "provider", "taxi:prov_1", "provider_payable"); got != 1500 {
		t.Fatalf("provider payable=%d, want 1500", got)
	}
	if got := accountBalanceMinor(t, pool, "organization", "org_1", "organization_payable"); got != 7500 {
		t.Fatalf("organization payable=%d, want 7500", got)
	}
	if got := accountBalanceMinor(t, pool, "platform", "karry-go", "platform_revenue"); got != 1000 {
		t.Fatalf("platform revenue=%d, want 1000", got)
	}
	if got := accountBalanceMinor(t, pool, "job", "taxi:"+jobID, "job_escrow"); got != -10000 {
		t.Fatalf("escrow=%d, want the full amount released", got)
	}
}

// -----------------------------------------------------------------------------
// FIN-02 / WAL-006 / Invariant 2 — a salaried provider is not paid per job
// -----------------------------------------------------------------------------

func TestSalariedProviderReceivesNoPerJobCredit(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_salaried"
	heldJob(t, pool, "dispatch", jobID, 10000, 1000)

	instruction := commissionInstruction(jobID)
	instruction.SourceService, instruction.ProviderType = "dispatch", "dispatch"
	// A pure salary plan resolves to nothing at the job level. The
	// organization receives the work entitlement; the provider is paid through
	// payroll.
	instruction.ProviderShareMinor, instruction.OrganizationShareMinor = 0, 9000

	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
		t.Fatalf("post managed settlement: %v", err)
	}
	assertEveryJournalBalances(t, pool)

	if got := accountBalanceMinor(t, pool, "provider", "dispatch:prov_1", "provider_payable"); got != 0 {
		t.Fatalf("salaried provider was credited %d for one job: that is double pay on top of salary", got)
	}
	if got := accountBalanceMinor(t, pool, "organization", "org_1", "organization_payable"); got != 9000 {
		t.Fatalf("organization payable=%d, want 9000", got)
	}
	// No zero-amount line was invented to represent the absent share.
	var providerLines int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM ledger_entries e JOIN wallet_accounts a ON a.id = e.account_id
		WHERE a.account_type = 'provider_payable'`).Scan(&providerLines); err != nil {
		t.Fatal(err)
	}
	if providerLines != 0 {
		t.Fatalf("%d provider lines written for a zero share", providerLines)
	}
}

// -----------------------------------------------------------------------------
// FIN-04 — a per-job provider
// -----------------------------------------------------------------------------

func TestPerJobProviderReceivesExactlyTheConfiguredAmount(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_perjob"
	heldJob(t, pool, "hauling", jobID, 10000, 1000)

	instruction := commissionInstruction(jobID)
	instruction.SourceService, instruction.ProviderType = "hauling", "hauling"
	instruction.ProviderShareMinor, instruction.OrganizationShareMinor = 2000, 7000

	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
		t.Fatalf("post managed settlement: %v", err)
	}
	assertEveryJournalBalances(t, pool)
	if got := accountBalanceMinor(t, pool, "provider", "hauling:prov_1", "provider_payable"); got != 2000 {
		t.Fatalf("per-job provider payable=%d, want exactly the configured 2000", got)
	}
}

// -----------------------------------------------------------------------------
// FIN-12 / FIN-13 / FIN-14 / WAL-004 / WAL-014 — idempotency
// -----------------------------------------------------------------------------

func TestResendingTheSameInstructionPostsOnce(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_retry"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	first, err := repo.PostManagedSettlement(context.Background(), instruction)
	if err != nil {
		t.Fatalf("first post: %v", err)
	}
	// This is the lost-response case: the caller never saw the answer and
	// sends the identical instruction again.
	second, err := repo.PostManagedSettlement(context.Background(), instruction)
	if err != nil {
		t.Fatalf("retry must return the original outcome, not an error: %v", err)
	}
	if first.ID != second.ID || first.LedgerTransactionID != second.LedgerTransactionID {
		t.Fatal("retry produced a different settlement: the money moved twice")
	}
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != 1 {
		t.Fatalf("%d settlement journals for one instruction", got)
	}
	if got := accountBalanceMinor(t, pool, "provider", "taxi:prov_1", "provider_payable"); got != 1500 {
		t.Fatalf("provider payable=%d after a retry, want 1500", got)
	}
}

func TestConcurrentSendsOfOneInstructionPostOnce(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_concurrent"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	// The synchronous completion path and the reconciliation worker can both
	// fire for the same completion. Exactly one journal must result.
	var wait sync.WaitGroup
	errs := make([]error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			_, errs[slot] = repo.PostManagedSettlement(context.Background(), instruction)
		}(i)
	}
	wait.Wait()

	var succeeded int
	for _, err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		t.Fatalf("every concurrent send failed: %v", errs)
	}
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != 1 {
		t.Fatalf("%d settlement journals from concurrent sends", got)
	}
	if got := accountBalanceMinor(t, pool, "provider", "taxi:prov_1", "provider_payable"); got != 1500 {
		t.Fatalf("provider payable=%d after concurrent sends, want 1500", got)
	}
	assertEveryJournalBalances(t, pool)
}

func TestSameIdentityWithDifferentAmountsIsAConflict(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_conflict"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)

	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
		t.Fatalf("first post: %v", err)
	}
	changed := instruction
	changed.ProviderShareMinor, changed.OrganizationShareMinor = 9000, 0
	_, err := repo.PostManagedSettlement(context.Background(), changed)
	if err == nil {
		t.Fatal("a changed instruction reusing the same identity must conflict, not overwrite")
	}
	if !strings.Contains(err.Error(), "different content") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := accountBalanceMinor(t, pool, "provider", "taxi:prov_1", "provider_payable"); got != 1500 {
		t.Fatalf("provider payable=%d: the refused instruction moved money", got)
	}
}

// -----------------------------------------------------------------------------
// WAL-009 — the allocation invariant
// -----------------------------------------------------------------------------

func TestUnderAndOverAllocationAreRefused(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)

	cases := map[string]struct{ provider, organization, gross int64 }{
		"under-allocated": {1000, 7000, 8000},
		"over-allocated":  {2000, 8000, 10000},
	}
	for name, amounts := range cases {
		t.Run(name, func(t *testing.T) {
			jobID := "job_" + strings.ReplaceAll(name, "-", "_")
			heldJob(t, pool, "taxi", jobID, 10000, 1000)
			instruction := commissionInstruction(jobID)
			instruction.ProviderShareMinor = amounts.provider
			instruction.OrganizationShareMinor = amounts.organization
			instruction.GrossBasisMinor = amounts.gross

			if _, err := repo.PostManagedSettlement(context.Background(), instruction); err == nil {
				t.Fatal("an allocation that does not account for the settleable amount must be refused")
			}
			if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != 0 {
				t.Fatalf("%d journals written for a refused allocation", got)
			}
		})
	}
}

func TestInstructionPlatformShareCannotOverrideTheAuthorizedFee(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_platform"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)

	instruction := commissionInstruction(jobID)
	invented := int64(0)
	instruction.PlatformShareMinor = &invented
	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err == nil {
		t.Fatal("Fleet must not be able to restate the platform's own share")
	}
}

// -----------------------------------------------------------------------------
// WAL-010 / Invariant 1 — the legacy path cannot pay managed work
// -----------------------------------------------------------------------------

func TestLegacyCompleteJobRefusesManagedWork(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_legacy_guard"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if _, err := pool.Exec(context.Background(),
		`UPDATE payment_intents SET provider_type='taxi', provider_id='prov_1' WHERE source_reference=$1`, jobID,
	); err != nil {
		t.Fatal(err)
	}

	if err := repo.MarkPaymentIntentManaged(context.Background(), "taxi", jobID); err != nil {
		t.Fatalf("mark managed: %v", err)
	}
	_, err := repo.CompleteJob(context.Background(), "taxi", jobID, time.Now().UTC())
	if err == nil {
		t.Fatal("the legacy direct-payout path must refuse organization-managed work")
	}
	if got := accountBalanceMinor(t, pool, "provider", "taxi:prov_1", "provider_payable"); got != 0 {
		t.Fatalf("legacy path credited the provider %d on a managed job", got)
	}
}

func TestLegacyCompleteJobStillWorksForIndependentProviders(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_independent"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if _, err := pool.Exec(context.Background(),
		`UPDATE payment_intents SET provider_type='taxi', provider_id='indie_1' WHERE source_reference=$1`, jobID,
	); err != nil {
		t.Fatal(err)
	}

	// FIN-01 / FIN-29: nothing about Section 12 may change what an independent
	// provider is paid.
	if _, err := repo.CompleteJob(context.Background(), "taxi", jobID, time.Now().UTC()); err != nil {
		t.Fatalf("independent settlement must be unaffected: %v", err)
	}
	if got := accountBalanceMinor(t, pool, "provider", "taxi:indie_1", "provider_payable"); got != 9000 {
		t.Fatalf("independent provider payable=%d, want the usual 9000", got)
	}
	assertEveryJournalBalances(t, pool)
}

func TestManagedSettlementRefusesAJobAlreadySettledLegacy(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_already_paid"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if _, err := pool.Exec(context.Background(),
		`UPDATE payment_intents SET provider_type='taxi', provider_id='indie_1' WHERE source_reference=$1`, jobID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompleteJob(context.Background(), "taxi", jobID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PostManagedSettlement(context.Background(), commissionInstruction(jobID)); err == nil {
		t.Fatal("settling an already-settled job would pay the same work twice")
	}
}

// -----------------------------------------------------------------------------
// FIN-30 / WAL-018 — reversal
// -----------------------------------------------------------------------------

func TestReversalCompensatesWithoutErasingHistory(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_reversed"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)
	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err != nil {
		t.Fatal(err)
	}

	reversed, err := repo.ReverseManagedSettlement(context.Background(),
		instruction.InstructionID, instruction.InstructionVersion, "CUSTOMER_REFUND")
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if reversed.Status != walletmodels.ManagedSettlementReversed || reversed.ReversalTransactionID == "" {
		t.Fatalf("unexpected reversal state: %+v", reversed)
	}
	// The original journal is still there. Correction is an addition, never an
	// edit.
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlement); got != 1 {
		t.Fatalf("original journal count=%d, want it preserved", got)
	}
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlementReversal); got != 1 {
		t.Fatalf("reversal journal count=%d", got)
	}
	assertEveryJournalBalances(t, pool)

	for _, check := range []struct {
		ownerType, ownerID, accountType string
	}{
		{"provider", "taxi:prov_1", "provider_payable"},
		{"organization", "org_1", "organization_payable"},
		{"platform", "karry-go", "platform_revenue"},
	} {
		if got := accountBalanceMinor(t, pool, check.ownerType, check.ownerID, check.accountType); got != 0 {
			t.Fatalf("%s balance=%d after reversal, want 0", check.accountType, got)
		}
	}
	if got := accountBalanceMinor(t, pool, "job", "taxi:"+jobID, "job_escrow"); got != 0 {
		t.Fatalf("escrow=%d after reversal, want the money returned", got)
	}

	// Reversing twice must not move the money a second time.
	if _, err := repo.ReverseManagedSettlement(context.Background(),
		instruction.InstructionID, instruction.InstructionVersion, "CUSTOMER_REFUND"); err != nil {
		t.Fatalf("repeat reversal must be idempotent: %v", err)
	}
	if got := journalCount(t, pool, walletmodels.JournalManagedSettlementReversal); got != 1 {
		t.Fatalf("%d reversal journals", got)
	}
}

// -----------------------------------------------------------------------------
// WAL-016 — read-back
// -----------------------------------------------------------------------------

func TestSettlementCanBeReadBackByEveryRoute(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_readback"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	instruction := commissionInstruction(jobID)
	posted, err := repo.PostManagedSettlement(context.Background(), instruction)
	if err != nil {
		t.Fatal(err)
	}

	byInstruction, err := repo.GetManagedSettlementByInstruction(context.Background(),
		instruction.InstructionID, instruction.InstructionVersion)
	if err != nil || byInstruction.ID != posted.ID {
		t.Fatalf("read back by instruction: %v %+v", err, byInstruction)
	}
	byKey, err := repo.GetManagedSettlementByIdempotencyKey(context.Background(), instruction.IdempotencyKey)
	if err != nil || byKey.ID != posted.ID {
		t.Fatalf("read back by key: %v %+v", err, byKey)
	}
	// This is the read an execution service makes after a timeout: a settled
	// job answers yes, so a lost response is never mistaken for a failure.
	bySource, err := repo.ListManagedSettlementsForSource(context.Background(), "taxi", jobID)
	if err != nil || len(bySource) != 1 || !bySource[0].Settled() {
		t.Fatalf("read back by job: %v %+v", err, bySource)
	}

	// And under the spelling the shared read client actually uses. Fleet
	// instructs with the source service as the instruction carried it while
	// the client normalises the case, so a case-sensitive match here meant an
	// execution service could never find its own settlement -- and an empty
	// answer reads as "not settled", which is how a job gets paid twice.
	for _, spelling := range []string{"TAXI", "Taxi", "taxi"} {
		found, err := repo.ListManagedSettlementsForSource(context.Background(), spelling, jobID)
		if err != nil || len(found) != 1 || !found[0].Settled() {
			t.Fatalf("read back by job as %q: %v %+v", spelling, err, found)
		}
	}
}

// -----------------------------------------------------------------------------
// FIN-26 — currency isolation
// -----------------------------------------------------------------------------

func TestInstructionInAnotherCurrencyIsRefused(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := "job_currency"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)

	instruction := commissionInstruction(jobID)
	instruction.Currency = "USD"
	if _, err := repo.PostManagedSettlement(context.Background(), instruction); err == nil {
		t.Fatal("settling an NGN payment against a USD instruction would be a silent conversion")
	}
}
