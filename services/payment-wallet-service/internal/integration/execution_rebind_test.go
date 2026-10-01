package integration_test

// Section 17: PostgreSQL-backed certification of the execution-provider
// rebind.
//
// The property that matters is a ledger property: after a replacement, the
// money is released once, to the provider who finished the job, and never to
// the one who walked away. That is only provable against the real ledger.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
)

const haulingSource = "driver-hauling-service"

type rebindHarness struct {
	pool    *pgxpool.Pool
	repo    *walletrepositories.PostgresWalletRepository
	service *walletusecases.WalletService
}

func newRebindHarness(t *testing.T) rebindHarness {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	return rebindHarness{pool: pool, repo: repo,
		service: walletusecases.NewWalletService(walletusecases.Options{Repository: repo})}
}

// heldHaulage seeds a held haulage payment bound to the original provider.
func (h rebindHarness) heldHaulage(t *testing.T, jobID, providerID string) {
	t.Helper()
	heldJob(t, h.pool, haulingSource, jobID, 10000, 1000)
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE payment_intents SET provider_type='truck_provider', provider_id=$2
		  WHERE source_service=$1 AND source_reference=$3`, haulingSource, providerID, jobID); err != nil {
		t.Fatal(err)
	}
}

func rebindRequest(jobID, providerID string, version int64, key string) walletmodels.ExecutionProviderRebindRequest {
	return walletmodels.ExecutionProviderRebindRequest{
		SourceService: haulingSource, SourceReference: jobID, CallerService: haulingSource,
		NewProviderType: "truck_provider", NewProviderID: providerID, ExpectedVersion: version,
		RecoveryCaseRef: "recovery-" + jobID, ReasonCode: "PROVIDER_REPLACED", IdempotencyKey: key,
	}
}

func appCode(err error) apperrors.Code {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func (h rebindHarness) auditCount(t *testing.T, jobID string) int {
	t.Helper()
	var count int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM execution_provider_rebind_audit WHERE source_reference=$1`, jobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The whole point: completion after a replacement pays the replacement, and
// the provider who left gets nothing.
func TestRebindReleasesEscrowToTheReplacementOnly(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_pay", "orig_1")

	rebind, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_pay", "repl_1", 1, "rebind-job_rebind_pay-1"))
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if rebind.PreviousProviderID != "orig_1" || rebind.NewProviderID != "repl_1" ||
		rebind.FromVersion != 1 || rebind.ToVersion != 2 {
		t.Fatalf("unexpected audit record %+v", rebind)
	}
	if _, err = h.repo.CompleteJob(context.Background(), haulingSource, "job_rebind_pay", time.Now().UTC()); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got := accountBalanceMinor(t, h.pool, "provider", "truck_provider:repl_1", "provider_payable"); got != 9000 {
		t.Fatalf("replacement payable=%d, want 9000", got)
	}
	if got := accountBalanceMinor(t, h.pool, "provider", "truck_provider:orig_1", "provider_payable"); got != 0 {
		t.Fatalf("the provider who left was paid %d", got)
	}
	// Completing again must not pay anyone a second time.
	_, _ = h.repo.CompleteJob(context.Background(), haulingSource, "job_rebind_pay", time.Now().UTC())
	if got := accountBalanceMinor(t, h.pool, "provider", "truck_provider:repl_1", "provider_payable"); got != 9000 {
		t.Fatalf("a repeated completion paid again: payable=%d", got)
	}
	assertEveryJournalBalances(t, h.pool)
}

// A rebind moves the beneficiary only: no ledger entry, no amount, no currency.
func TestRebindChangesNothingButTheBeneficiary(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_immutable", "orig_1")
	var before, after struct {
		amount, fee int64
		currency    string
		status      string
		customer    string
	}
	read := func(target *struct {
		amount, fee int64
		currency    string
		status      string
		customer    string
	}) {
		if err := h.pool.QueryRow(context.Background(), `SELECT amount_kobo, platform_fee_kobo, currency, status, customer_id
			FROM payment_intents WHERE source_reference='job_rebind_immutable'`).
			Scan(&target.amount, &target.fee, &target.currency, &target.status, &target.customer); err != nil {
			t.Fatal(err)
		}
	}
	read(&before)
	var entriesBefore, entriesAfter int
	_ = h.pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_entries`).Scan(&entriesBefore)

	if _, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_immutable", "repl_1", 1, "rebind-immutable")); err != nil {
		t.Fatal(err)
	}
	read(&after)
	_ = h.pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_entries`).Scan(&entriesAfter)
	if before != after {
		t.Fatalf("rebind changed the held payment: before=%+v after=%+v", before, after)
	}
	if entriesBefore != entriesAfter {
		t.Fatalf("rebind wrote %d ledger entries", entriesAfter-entriesBefore)
	}
}

func TestRebindRefusesAStaleVersion(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_stale", "orig_1")
	if _, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_stale", "repl_1", 1, "rebind-stale-a")); err != nil {
		t.Fatal(err)
	}
	_, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_stale", "repl_2", 1, "rebind-stale-b"))
	if appCode(err) != walletusecases.CodeRebindVersionConflict {
		t.Fatalf("a rebind decided on a stale binding must conflict, got %v", err)
	}
	// A second replacement decided on the current version is fine.
	if _, err = h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_stale", "repl_2", 2, "rebind-stale-c")); err != nil {
		t.Fatalf("second rebind on the current version: %v", err)
	}
	if h.auditCount(t, "job_rebind_stale") != 2 {
		t.Fatal("each successful rebind must leave exactly one audit row")
	}
}

func TestRebindReplaysAndRefusesReusedKeys(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_idem", "orig_1")
	request := rebindRequest("job_rebind_idem", "repl_1", 1, "rebind-idem")
	first, err := h.service.RebindExecutionProvider(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := h.service.RebindExecutionProvider(context.Background(), request)
	if err != nil || !replay.Replayed || replay.ID != first.ID {
		t.Fatalf("a repeated request must replay the original, got %+v err=%v", replay, err)
	}
	changed := request
	changed.NewProviderID = "repl_other"
	if _, err = h.service.RebindExecutionProvider(context.Background(), changed); appCode(err) != walletusecases.CodeRebindKeyReused {
		t.Fatalf("a reused key with different content must be refused, got %v", err)
	}
	if h.auditCount(t, "job_rebind_idem") != 1 {
		t.Fatal("a replay must not write a second audit row")
	}
}

// Six concurrent sends of one request produce one rebind and six identical
// answers -- none of them a version conflict.
func TestConcurrentRebindSendsApplyOnce(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_race", "orig_1")
	request := rebindRequest("job_rebind_race", "repl_1", 1, "rebind-race")
	var wg sync.WaitGroup
	ids := make([]string, 6)
	errs := make([]error, 6)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rebind, err := h.service.RebindExecutionProvider(context.Background(), request)
			ids[i], errs[i] = rebind.ID, err
		}(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("send %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatalf("sends disagree on the rebind: %v", ids)
		}
	}
	if h.auditCount(t, "job_rebind_race") != 1 {
		t.Fatal("concurrent sends must record exactly one rebind")
	}
}

func TestRebindRefusesAPaymentThatIsNotHeld(t *testing.T) {
	h := newRebindHarness(t)
	for _, status := range []string{"completed", "refunded", "partially_refunded", "failed", "pending"} {
		jobID := "job_rebind_" + status
		h.heldHaulage(t, jobID, "orig_1")
		if _, err := h.pool.Exec(context.Background(),
			`UPDATE payment_intents SET status=$2 WHERE source_reference=$1`, jobID, status); err != nil {
			t.Fatal(err)
		}
		_, err := h.service.RebindExecutionProvider(context.Background(),
			rebindRequest(jobID, "repl_1", 1, "rebind-"+status))
		if appCode(err) != walletusecases.CodeRebindNotHeld {
			t.Fatalf("status %s: a payment that is not held must refuse a rebind, got %v", status, err)
		}
		if h.auditCount(t, jobID) != 0 {
			t.Fatalf("status %s: refused rebind wrote an audit row", status)
		}
	}
}

// Only the service that owns the job may move its beneficiary.
func TestRebindCallerMustOwnTheJob(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_caller", "orig_1")
	for _, caller := range []string{"fleet-partner-service", "dispatch-delivery-service",
		"taxi-service", "support-dispute-service", "vendor-service", ""} {
		request := rebindRequest("job_rebind_caller", "repl_1", 1, "rebind-caller-"+caller)
		request.CallerService = caller
		_, err := h.service.RebindExecutionProvider(context.Background(), request)
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) || appErr.Status != 403 {
			t.Fatalf("caller %q must be forbidden, got %v", caller, err)
		}
	}
	// The directory-style spelling of the owner is accepted.
	request := rebindRequest("job_rebind_caller", "repl_1", 1, "rebind-caller-alias")
	request.CallerService = "hauling-service"
	if _, err := h.service.RebindExecutionProvider(context.Background(), request); err != nil {
		t.Fatalf("owner under its provisioned identity: %v", err)
	}
}

func TestRebindRequiresARecoveryCase(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_case", "orig_1")
	request := rebindRequest("job_rebind_case", "repl_1", 1, "rebind-case")
	request.RecoveryCaseRef = " "
	var appErr *apperrors.Error
	if _, err := h.service.RebindExecutionProvider(context.Background(), request); !errors.As(err, &appErr) ||
		appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("a rebind with no recovery case must be refused, got %v", err)
	}
}

func TestRebindToTheSameProviderIsNotARebind(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_same", "orig_1")
	_, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_same", "orig_1", 1, "rebind-same"))
	if appCode(err) != walletusecases.CodeRebindNoChange {
		t.Fatalf("rebinding to the bound provider must say so, got %v", err)
	}
}

func TestRebindAuditIsAppendOnly(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_audit", "orig_1")
	if _, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_audit", "repl_1", 1, "rebind-audit")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE execution_provider_rebind_audit SET new_provider_id='someone_else'`); err == nil {
		t.Fatal("the rebind audit must refuse an update")
	}
	if _, err := h.pool.Exec(context.Background(), `DELETE FROM execution_provider_rebind_audit`); err == nil {
		t.Fatal("the rebind audit must refuse a delete")
	}
	binding, err := h.service.GetExecutionProviderBinding(context.Background(),
		haulingSource, haulingSource, "job_rebind_audit")
	if err != nil || binding.Version != 2 || len(binding.History) != 1 {
		t.Fatalf("binding read-back: %+v err=%v", binding, err)
	}
}

// A rebind must not open the legacy payout path to organization-managed work.
func TestRebindKeepsManagedWorkOffTheLegacyPath(t *testing.T) {
	h := newRebindHarness(t)
	h.heldHaulage(t, "job_rebind_managed", "orig_1")
	if err := h.repo.MarkPaymentIntentManaged(context.Background(), haulingSource, "job_rebind_managed"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RebindExecutionProvider(context.Background(),
		rebindRequest("job_rebind_managed", "repl_1", 1, "rebind-managed")); err != nil {
		t.Fatalf("rebind of managed work: %v", err)
	}
	if _, err := h.repo.CompleteJob(context.Background(), haulingSource, "job_rebind_managed", time.Now().UTC()); err == nil {
		t.Fatal("after a rebind, managed work must still refuse legacy direct payout")
	}
	if got := accountBalanceMinor(t, h.pool, "provider", "truck_provider:repl_1", "provider_payable"); got != 0 {
		t.Fatalf("legacy path paid the replacement %d on managed work", got)
	}
}

// Migration 000017 must remove exactly what it added when rolled back.
func TestRebindMigrationRollsBackCleanly(t *testing.T) {
	h := newRebindHarness(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if err := database.ApplyMigrations(ctx, h.pool, migrationsDir); err != nil {
			t.Errorf("reapply after rollback: %v", err)
		}
	})
	exists := func(query string) bool {
		var found bool
		if err := h.pool.QueryRow(ctx, query).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	const table = `SELECT to_regclass('public.execution_provider_rebind_audit') IS NOT NULL`
	const column = `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_name='payment_intents' AND column_name='execution_binding_version')`
	if !exists(table) || !exists(column) {
		t.Fatal("000017 is not applied")
	}
	// Later migrations (000018 onward) are unwound first, so the rollback
	// under test is 000017's own rather than whichever migration is newest.
	migrations, err := database.ListMigrations(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.Number > 17 {
			if err = database.RollbackLastMigration(ctx, h.pool, migrationsDir); err != nil {
				t.Fatalf("rollback %s: %v", migration.Version, err)
			}
		}
	}
	if !exists(table) || !exists(column) {
		t.Fatal("unwinding later migrations removed 000017's objects")
	}
	if err = database.RollbackLastMigration(ctx, h.pool, migrationsDir); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if exists(table) || exists(column) {
		t.Fatal("rolling back 000017 left its table or column behind")
	}
	if !exists(`SELECT to_regclass('public.managed_settlements') IS NOT NULL`) {
		t.Fatal("rolling back 000017 removed something it does not own")
	}
}
