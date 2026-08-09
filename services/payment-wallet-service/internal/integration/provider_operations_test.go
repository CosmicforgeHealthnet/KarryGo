package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"

	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
)

// providerFixture is one organization wallet funded with real postings, plus
// one provider sub-wallet bound to a membership.
type providerFixture struct {
	fixture          financeFixture
	organizationID   string
	membershipID     string
	walletID         string
	earningAccountID string
}

// fundWallet credits the organization wallet through a balanced journal, so
// the money the tests move is money that actually exists in the ledger.
func newProviderFixture(t *testing.T, organizationID string) providerFixture {
	t.Helper()
	f := newFinanceFixture(t)
	ctx := context.Background()

	membershipID := "mem_" + strings.ReplaceAll(organizationID, "org_", "")
	account, err := f.service.EnsureProviderSubWallet(ctx, usecases.EnsureProviderSubWalletInput{
		Meta:           serviceMeta("fleet-partner-service", "ensure-"+organizationID, organizationID),
		OrganizationID: organizationID, ProviderID: "prv_1",
		OrganizationMembershipID: membershipID, ProviderServiceCategory: "DISPATCH",
		Currency: "NGN"})
	if err != nil {
		t.Fatalf("ensure provider sub-wallet: %v", err)
	}

	wallet, err := f.service.Accounts().GetWallet(ctx, nil, organizationID, "NGN")
	if err != nil {
		t.Fatalf("read organization wallet: %v", err)
	}

	// Credit the organization wallet the way a marketplace settlement would:
	// a platform receivable is debited and the organization's own account is
	// credited. Nothing here touches the provider's account.
	receivable, err := f.ledger.EnsureAccount(ctx, nil, "", models.OwnerPlatform, "platform",
		models.AccountPlatformReceivable, "NGN", models.SideDebit)
	if err != nil {
		t.Fatalf("ensure receivable: %v", err)
	}
	// The posting and the balance move share one transaction: the deferred
	// balance trigger runs at COMMIT, so a journal built statement by statement
	// outside a transaction would be judged unbalanced after its first line.
	if err := database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		if _, postErr := f.ledger.Post(ctx, tx, repositories.Posting{
			OrganizationID: organizationID, JournalType: models.JournalWalletTopUp, Currency: "NGN",
			IdempotencyKey: "settlement-" + organizationID, ActorType: "SYSTEM", ActorID: "test",
			Lines: []repositories.PostingLine{
				{LedgerAccountID: receivable.ID, Side: models.SideDebit, AmountMinor: 5_000_000},
				{LedgerAccountID: wallet.LedgerAccountID, Side: models.SideCredit, AmountMinor: 5_000_000}},
		}); postErr != nil {
			return postErr
		}
		return f.service.Accounts().MoveWalletBalances(ctx, tx, wallet.ID, wallet.Version,
			5_000_000, 0, 0)
	}); err != nil {
		t.Fatalf("fund organization wallet: %v", err)
	}

	return providerFixture{fixture: f, organizationID: organizationID, membershipID: membershipID,
		walletID: wallet.ID, earningAccountID: account.ID}
}

func (p providerFixture) allocate(t *testing.T, key string, amount int64) *models.ProviderAllocation {
	t.Helper()
	allocation, err := p.fixture.service.PostProviderAllocation(context.Background(),
		usecases.ProviderAllocationInput{
			Meta:           serviceMeta("fleet-partner-service", key, p.organizationID),
			OrganizationID: p.organizationID, EarningAccountID: p.earningAccountID,
			OrganizationMembershipID: p.membershipID, AllocationType: models.AllocationSalary,
			BusinessReferenceType: models.ReferencePayroll, BusinessReferenceID: "payroll-" + key,
			AmountMinor: amount, Currency: "NGN", ReasonCode: "MONTHLY_SALARY",
			ProviderExplanation: "July salary."})
	if err != nil {
		t.Fatalf("post provider allocation: %v", err)
	}
	return allocation
}

// The organization wallet funds the provider; a customer settlement never
// credits the provider account directly. This asserts on the postings, which
// is where a collapsed flow would actually show up.
func TestProviderAllocationIsASeparateMovementFromSettlement(t *testing.T) {
	f := newProviderFixture(t, "org_alloc_separate")
	ctx := context.Background()

	allocation := f.allocate(t, "alloc-1", 700_000)
	readBack, err := f.fixture.service.FindProviderAllocationByKey(ctx,
		serviceMeta("fleet-partner-service", "read-alloc-1", f.organizationID),
		f.organizationID, "alloc-1")
	if err != nil || readBack.ID != allocation.ID {
		t.Fatalf("provider allocation read-back: value=%+v err=%v", readBack, err)
	}

	// The settlement journal that funded the wallet and the allocation journal
	// are distinct records with distinct types.
	var settlementType, allocationType string
	if err := f.fixture.pool.QueryRow(ctx,
		`SELECT journal_type FROM finance_journals WHERE idempotency_key = $1`,
		"settlement-"+f.organizationID).Scan(&settlementType); err != nil {
		t.Fatalf("read settlement journal: %v", err)
	}
	if err := f.fixture.pool.QueryRow(ctx,
		`SELECT journal_type FROM finance_journals WHERE id = $1`,
		allocation.JournalID).Scan(&allocationType); err != nil {
		t.Fatalf("read allocation journal: %v", err)
	}
	if settlementType == allocationType {
		t.Fatalf("settlement and provider allocation share journal type %q", allocationType)
	}
	if allocationType != models.JournalProviderAllocation {
		t.Fatalf("allocation journal type = %q, want %q", allocationType,
			models.JournalProviderAllocation)
	}

	// The settlement posting must not touch the provider's ledger account.
	var providerLines int
	if err := f.fixture.pool.QueryRow(ctx, `
		SELECT count(*) FROM finance_journal_lines l
		JOIN finance_journals j ON j.id = l.journal_id
		JOIN finance_earning_accounts e ON e.ledger_account_id = l.ledger_account_id
		WHERE j.idempotency_key = $1`, "settlement-"+f.organizationID).Scan(&providerLines); err != nil {
		t.Fatalf("inspect settlement lines: %v", err)
	}
	if providerLines != 0 {
		t.Fatalf("a customer settlement posted %d lines to a provider sub-wallet", providerLines)
	}
}

// Every journal must balance, including the ones this work package added.
func TestProviderOperationsLeaveTheJournalBalanced(t *testing.T) {
	f := newProviderFixture(t, "org_alloc_balanced")
	ctx := context.Background()
	f.allocate(t, "alloc-balance-1", 400_000)

	rows, err := f.fixture.pool.Query(ctx, `
		SELECT j.id,
		       COALESCE(SUM(CASE WHEN l.side = 'DEBIT' THEN l.amount_minor ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN l.side = 'CREDIT' THEN l.amount_minor ELSE 0 END), 0)
		FROM finance_journals j JOIN finance_journal_lines l ON l.journal_id = j.id
		GROUP BY j.id`)
	if err != nil {
		t.Fatalf("inspect journals: %v", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var id string
		var debit, credit int64
		if err := rows.Scan(&id, &debit, &credit); err != nil {
			t.Fatalf("scan journal totals: %v", err)
		}
		if debit != credit {
			t.Errorf("journal %s is unbalanced: debit %d, credit %d", id, debit, credit)
		}
		checked++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate journals: %v", err)
	}
	if checked == 0 {
		t.Fatal("no journals were checked; the assertion would pass vacuously")
	}
}

// A retried allocation with the same key returns the same record; the same key
// with different values is a conflict rather than a silent replay of the first.
func TestProviderAllocationIdempotency(t *testing.T) {
	f := newProviderFixture(t, "org_alloc_idem")
	ctx := context.Background()

	first := f.allocate(t, "idem-1", 300_000)
	second := f.allocate(t, "idem-1", 300_000)
	if first.ID != second.ID {
		t.Fatalf("a retry created a second allocation: %s vs %s", first.ID, second.ID)
	}

	var journals int
	if err := f.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_provider_allocations WHERE organization_id = $1`,
		f.organizationID).Scan(&journals); err != nil {
		t.Fatalf("count allocations: %v", err)
	}
	if journals != 1 {
		t.Fatalf("allocations = %d, want 1", journals)
	}

	_, err := f.fixture.service.PostProviderAllocation(ctx, usecases.ProviderAllocationInput{
		Meta:           serviceMeta("fleet-partner-service", "idem-1", f.organizationID),
		OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID,
		OrganizationMembershipID: f.membershipID, AllocationType: models.AllocationSalary,
		BusinessReferenceType: models.ReferencePayroll, BusinessReferenceID: "payroll-idem-1",
		AmountMinor: 999_999, Currency: "NGN", ReasonCode: "MONTHLY_SALARY"})
	if err == nil {
		t.Fatal("a reused idempotency key with different values must be rejected")
	}
}

// An allocation larger than the organization's available funds is refused. The
// check is against the authoritative balance, not a caller's belief about it.
func TestProviderAllocationRefusesInsufficientFunds(t *testing.T) {
	f := newProviderFixture(t, "org_alloc_funds")
	_, err := f.fixture.service.PostProviderAllocation(context.Background(),
		usecases.ProviderAllocationInput{
			Meta:           serviceMeta("fleet-partner-service", "overdraw", f.organizationID),
			OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID,
			OrganizationMembershipID: f.membershipID, AllocationType: models.AllocationSalary,
			BusinessReferenceType: models.ReferencePayroll, BusinessReferenceID: "payroll-big",
			AmountMinor: 50_000_000, Currency: "NGN", ReasonCode: "MONTHLY_SALARY"})
	if err == nil {
		t.Fatal("an allocation beyond the wallet balance must be refused")
	}
}

// Requesting a withdrawal reserves the funds immediately. Two concurrent
// requests against one sub-wallet must not both find the same balance free.
func TestProviderWithdrawalReservesFundsAndRefusesASecondOpenRequest(t *testing.T) {
	f := newProviderFixture(t, "org_wdr_reserve")
	ctx := context.Background()
	f.allocate(t, "wdr-fund", 500_000)

	first, err := f.fixture.service.RequestProviderWithdrawal(ctx, usecases.ProviderWithdrawalInput{
		Meta:           serviceMeta("fleet-partner-service", "wdr-1", f.organizationID),
		OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID,
		OrganizationMembershipID: f.membershipID, AmountMinor: 200_000, Currency: "NGN",
		DestinationReference: "payout_dest_opaque", PolicyMode: models.WithdrawalPolicyCompanyApproval})
	if err != nil {
		t.Fatalf("request withdrawal: %v", err)
	}
	if first.Status != models.WithdrawalPendingApproval {
		t.Fatalf("status = %q, want PENDING_APPROVAL", first.Status)
	}

	account, err := f.fixture.service.Accounts().GetEarningAccountByID(ctx, nil, f.earningAccountID)
	if err != nil {
		t.Fatalf("read earning account: %v", err)
	}
	if account.HeldMinor != 200_000 {
		t.Fatalf("held = %d, want the withdrawal amount reserved", account.HeldMinor)
	}
	if account.PayableMinor != 300_000 {
		t.Fatalf("payable = %d, want the balance reduced by the reservation", account.PayableMinor)
	}

	// A second open request against the same sub-wallet is refused by the
	// partial unique index, not by a check that could race.
	if _, err := f.fixture.service.RequestProviderWithdrawal(ctx, usecases.ProviderWithdrawalInput{
		Meta:           serviceMeta("fleet-partner-service", "wdr-2", f.organizationID),
		OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID,
		OrganizationMembershipID: f.membershipID, AmountMinor: 100_000, Currency: "NGN",
		DestinationReference: "payout_dest_opaque"}); err == nil {
		t.Fatal("a second open withdrawal for one sub-wallet must be refused")
	}
}

// Rejecting a withdrawal returns the reserved funds. Approving leaves them
// reserved for the execution worker.
func TestRejectedWithdrawalReleasesReservedFunds(t *testing.T) {
	f := newProviderFixture(t, "org_wdr_reject")
	ctx := context.Background()
	f.allocate(t, "wdr-reject-fund", 500_000)

	request, err := f.fixture.service.RequestProviderWithdrawal(ctx, usecases.ProviderWithdrawalInput{
		Meta:           serviceMeta("fleet-partner-service", "wdr-r-1", f.organizationID),
		OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID,
		OrganizationMembershipID: f.membershipID, AmountMinor: 150_000, Currency: "NGN",
		DestinationReference: "payout_dest_opaque", RequiresApproval: true})
	if err != nil {
		t.Fatalf("request withdrawal: %v", err)
	}
	// A different service identity decides, so separation of duties holds.
	if _, err := f.fixture.service.DecideProviderWithdrawal(ctx,
		serviceMeta("admin-backoffice-service", "wdr-r-decide", f.organizationID),
		f.organizationID, request.ID, request.Version, false, "", "NOT_ELIGIBLE"); err != nil {
		t.Fatalf("reject withdrawal: %v", err)
	}
	account, err := f.fixture.service.Accounts().GetEarningAccountByID(ctx, nil, f.earningAccountID)
	if err != nil {
		t.Fatalf("read earning account: %v", err)
	}
	if account.HeldMinor != 0 {
		t.Fatalf("held = %d after rejection, want 0", account.HeldMinor)
	}
	if account.PayableMinor != 500_000 {
		t.Fatalf("payable = %d after rejection, want the funds returned", account.PayableMinor)
	}
}

// The provider statement carries no customer, organization or commercial data.
func TestProviderStatementIsMinimal(t *testing.T) {
	f := newProviderFixture(t, "org_stmt_minimal")
	ctx := context.Background()
	f.allocate(t, "stmt-1", 250_000)

	statement, err := f.fixture.service.GenerateProviderStatement(ctx,
		usecases.ProviderStatementInput{
			Meta:           serviceMeta("fleet-partner-service", "stmt-read", f.organizationID),
			OrganizationID: f.organizationID, EarningAccountID: f.earningAccountID})
	if err != nil {
		t.Fatalf("generate provider statement: %v", err)
	}
	if len(statement.Entries) != 1 {
		t.Fatalf("statement entries = %d, want 1", len(statement.Entries))
	}
	entry := statement.Entries[0]
	if entry.Direction != models.DirectionCredit {
		t.Fatalf("direction = %q, want CREDIT", entry.Direction)
	}
	if entry.AmountMinor != 250_000 {
		t.Fatalf("amount = %d, want the allocated amount", entry.AmountMinor)
	}
	// The organization's own wallet balance is nowhere in the provider view.
	wallet, err := f.fixture.service.Accounts().GetWalletByID(ctx, nil, f.organizationID, f.walletID)
	if err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if statement.AvailableMinor == wallet.AvailableMinor {
		t.Fatal("the provider statement must not report the organization wallet balance")
	}
}

// One active sub-wallet per membership, service category and currency.
func TestProviderSubWalletIsSingularPerMembership(t *testing.T) {
	f := newProviderFixture(t, "org_subwallet_single")
	ctx := context.Background()

	again, err := f.fixture.service.EnsureProviderSubWallet(ctx,
		usecases.EnsureProviderSubWalletInput{
			Meta:           serviceMeta("fleet-partner-service", "ensure-again", f.organizationID),
			OrganizationID: f.organizationID, ProviderID: "prv_1",
			OrganizationMembershipID: f.membershipID, ProviderServiceCategory: "DISPATCH",
			Currency: "NGN"})
	if err != nil {
		t.Fatalf("re-ensure provider sub-wallet: %v", err)
	}
	if again.ID != f.earningAccountID {
		t.Fatalf("a second sub-wallet was created: %s vs %s", again.ID, f.earningAccountID)
	}

	var count int
	if err := f.fixture.pool.QueryRow(ctx, `
		SELECT count(*) FROM finance_earning_accounts
		WHERE organization_id = $1 AND organization_membership_id = $2
		  AND provider_service_category = 'DISPATCH' AND currency = 'NGN'`,
		f.organizationID, f.membershipID).Scan(&count); err != nil {
		t.Fatalf("count sub-wallets: %v", err)
	}
	if count != 1 {
		t.Fatalf("sub-wallets for one membership = %d, want 1", count)
	}
}
