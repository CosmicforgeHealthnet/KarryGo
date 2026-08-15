package crm

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Section 8D core tests: programme configuration, account and ledger
// invariants, point lots and FEFO consumption.
//
// Every one of these runs against real PostgreSQL. The invariants Section 8D
// claims — a balance that can never go negative, a sequence that cannot repeat,
// an expiry that cannot be rewritten — are enforced partly by constraints and
// partly by lock ordering, and neither survives being mocked.

func loyaltyActor(business, staff string) Actor { return actorFor(business, staff) }

// seedLoyaltyProgram switches a business's programme on with the given expiry
// policy.
func (e *testEnv) seedLoyaltyProgram(t *testing.T, business, staff string, status string, expiryDays *int) LoyaltyProgram {
	t.Helper()
	in := UpdateLoyaltyProgramInput{Status: &status, ExpectedVersion: 0}
	if expiryDays != nil {
		policy := LoyaltyExpiryFixedDays
		in.ExpiryPolicy, in.ExpiryDays = &policy, expiryDays
	}
	current, err := e.service.GetLoyaltyProgram(context.Background(), loyaltyActor(business, staff))
	if err != nil {
		t.Fatal(err)
	}
	in.ExpectedVersion = current.Version
	program, err := e.service.UpdateLoyaltyProgram(context.Background(), loyaltyActor(business, staff), in)
	if err != nil {
		t.Fatalf("seed loyalty programme: %v", err)
	}
	return program
}

// credit posts a trusted credit through the domain primitive.
func (e *testEnv) credit(t *testing.T, business, staff string, profile Profile, points int64, ref string) LoyaltyEntry {
	t.Helper()
	entry, err := e.service.CreditPoints(context.Background(), loyaltyActor(business, staff), Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: profile.ID, Points: points, SourceType: LoyaltySourceOrder,
		SourceReference: ref, CommandRef: "cmd_" + ref,
	})
	if err != nil {
		t.Fatalf("credit %s: %v", ref, err)
	}
	return entry
}

func (e *testEnv) loyaltyAccount(t *testing.T, business, profileID string) LoyaltyAccount {
	t.Helper()
	account, err := scanLoyaltyAccount(e.pool.QueryRow(context.Background(),
		`SELECT `+loyaltyAccountCols+` FROM crm_loyalty_accounts WHERE business_id=$1 AND customer_profile_id=$2`,
		business, profileID))
	if err != nil {
		t.Fatalf("load loyalty account: %v", err)
	}
	return account
}

func (e *testEnv) lots(t *testing.T, business, accountID string) []LoyaltyPointLot {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT `+loyaltyLotCols+` FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2 ORDER BY created_at, id`, business, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []LoyaltyPointLot
	for rows.Next() {
		lot, scanErr := scanLoyaltyLot(rows)
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		out = append(out, lot)
	}
	return out
}

func intPtr(v int) *int { return &v }

// ---------------------------------------------------------------------------
// Programme configuration
// ---------------------------------------------------------------------------

// TestLoyaltyProgram_DefaultsAreUnwrittenAndReadsNeverCreate pins the choice
// that a GET is a read: a business that has never configured loyalty gets a
// synthetic disabled default at version 0, and no row appears behind it.
func TestLoyaltyProgram_DefaultsAreUnwrittenAndReadsNeverCreate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	program, err := env.service.GetLoyaltyProgram(ctx, loyaltyActor(businessA, "vs_a"))
	if err != nil {
		t.Fatal(err)
	}
	if program.Status != LoyaltyProgramDisabled || program.Version != 0 || program.Persisted {
		t.Fatalf("default programme = %+v; want a disabled, unwritten default at version 0", program)
	}
	var rows int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_program_configs WHERE business_id=$1`,
		businessA).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("reading the programme created %d rows; a GET must not write", rows)
	}
}

func TestLoyaltyProgram_StatusTransitionsAndVersioning(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")

	active := LoyaltyProgramActive
	program, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{Status: &active, ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	if program.Status != LoyaltyProgramActive || program.Version != 1 || !program.Persisted {
		t.Fatalf("first write = %+v; want ACTIVE at version 1", program)
	}
	paused := LoyaltyProgramPaused
	program, err = env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{Status: &paused, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if program.Status != LoyaltyProgramPaused || program.Version != 2 {
		t.Fatalf("pause = %+v; want PAUSED at version 2", program)
	}
	disabled := LoyaltyProgramDisabled
	program, err = env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{Status: &disabled, ExpectedVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if program.Status != LoyaltyProgramDisabled || program.Version != 3 {
		t.Fatalf("disable = %+v; want DISABLED at version 3", program)
	}

	// A stale version loses rather than silently overwriting.
	if _, err = env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{Status: &active, ExpectedVersion: 2}); err == nil {
		t.Fatal("a stale expected_version was accepted; configuration must not be last-write-wins")
	} else if !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("stale update error = %v; want a version conflict", err)
	}
}

func TestLoyaltyProgram_ExpiryPolicyValidation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")

	fixed := LoyaltyExpiryFixedDays
	// FIXED_DAYS without a window is not a valid programme.
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &fixed, ExpectedVersion: 0}); err == nil {
		t.Fatal("FIXED_DAYS without expiry_days was accepted")
	}
	never := LoyaltyExpiryNever
	// NEVER with a window is equally incoherent.
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &never, ExpiryDays: intPtr(30), ExpectedVersion: 0}); err == nil {
		t.Fatal("NEVER with expiry_days was accepted")
	}
	bogus := "ROLLING"
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &bogus, ExpectedVersion: 0}); err == nil {
		t.Fatal("an unknown expiry policy was accepted")
	}
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &fixed, ExpiryDays: intPtr(0), ExpectedVersion: 0}); err == nil {
		t.Fatal("a zero-day expiry window was accepted")
	}
	program, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &fixed, ExpiryDays: intPtr(30), ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	if program.ExpiryPolicy != LoyaltyExpiryFixedDays || program.ExpiryDays == nil || *program.ExpiryDays != 30 {
		t.Fatalf("programme = %+v; want FIXED_DAYS/30", program)
	}
}

// TestLoyaltyProgram_ExpiryChangeDoesNotRewriteIssuedLots is the whole reason
// expiry is stamped per lot: a business that shortens its window must not be
// able to retroactively void points customers already hold.
func TestLoyaltyProgram_ExpiryChangeDoesNotRewriteIssuedLots(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_expiry_policy"))

	env.credit(t, businessA, "vs_a", profile, 100, "ord_1")
	account := env.loyaltyAccount(t, businessA, profile.ID)
	before := env.lots(t, businessA, account.ID)
	if len(before) != 1 || before[0].ExpiresAt == nil {
		t.Fatalf("lots = %+v; want one lot with an expiry", before)
	}
	original := *before[0].ExpiresAt
	if want := env.now.AddDate(0, 0, 30); !original.Equal(want) {
		t.Fatalf("lot expiry = %s; want %s", original, want)
	}

	// Shorten the window drastically, then lengthen it, then switch it off.
	for _, change := range []UpdateLoyaltyProgramInput{
		{ExpiryDays: intPtr(1), ExpectedVersion: program.Version},
		{ExpiryDays: intPtr(365), ExpectedVersion: program.Version + 1},
	} {
		if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, change); err != nil {
			t.Fatal(err)
		}
	}
	never := LoyaltyExpiryNever
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &never, ClearExpiryDays: true, ExpectedVersion: program.Version + 2}); err != nil {
		t.Fatal(err)
	}

	after := env.lots(t, businessA, account.ID)
	if len(after) != 1 || after[0].ExpiresAt == nil || !after[0].ExpiresAt.Equal(original) {
		t.Fatalf("lot expiry after three policy changes = %+v; want the original %s", after, original)
	}
}

// TestLoyaltyProgram_TenantIsolation stops one business reading or writing
// another's programme through its own session.
func TestLoyaltyProgram_TenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))

	program, err := env.service.GetLoyaltyProgram(ctx, loyaltyActor(businessB, "vs_b"))
	if err != nil {
		t.Fatal(err)
	}
	if program.Persisted || program.Status != LoyaltyProgramDisabled {
		t.Fatalf("business B read = %+v; it must not see business A's programme", program)
	}
	active := LoyaltyProgramActive
	if _, err = env.service.UpdateLoyaltyProgram(ctx, loyaltyActor(businessB, "vs_b"),
		UpdateLoyaltyProgramInput{Status: &active, ExpectedVersion: 1}); err == nil {
		t.Fatal("business B wrote using business A's version; tenancy is not enforced")
	}
}

// ---------------------------------------------------------------------------
// Accounts and ledger invariants
// ---------------------------------------------------------------------------

// TestLoyalty_FirstCreditCreatesAccountLazily pins lazy enrolment: no account
// exists until points actually move.
func TestLoyalty_FirstCreditCreatesAccountLazily(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_lazy"))

	summary, err := env.service.LoyaltySummaryFor(ctx, loyaltyActor(businessA, "vs_a"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Enrollment != LoyaltyNotEnrolled {
		t.Fatalf("enrollment = %s; an unenrolled customer must not be reported as a zero balance", summary.Enrollment)
	}
	var accounts int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_accounts WHERE business_id=$1`, businessA).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 {
		t.Fatalf("reading a summary created %d accounts", accounts)
	}

	env.credit(t, businessA, "vs_a", profile, 250, "ord_lazy")
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 250 || account.LifetimeCreditedPoints != 250 || account.LifetimeDebitedPoints != 0 {
		t.Fatalf("account = %+v; want 250 credited", account)
	}
}

// TestLoyalty_LedgerSequenceAndBalanceAreDeterministic walks a mixed history
// and checks that every posted row states the balance that actually resulted,
// and that the aggregate agrees with the ledger at the end.
func TestLoyalty_LedgerSequenceAndBalanceAreDeterministic(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_ledger"))

	env.credit(t, businessA, "vs_a", profile, 100, "ord_a")
	env.credit(t, businessA, "vs_a", profile, 250, "ord_b")
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 120, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_c", CommandRef: "cmd_ord_c"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 30,
		Reason: "goodwill after a late delivery", CommandRef: "idem_adj_1"}); err != nil {
		t.Fatal(err)
	}

	entries, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("ledger has %d entries; want 4", len(entries))
	}
	// Replay oldest-first: sequence must be dense and ascending, and each
	// balance_after must equal the running total.
	var running int64
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		expectedSequence := int64(len(entries) - i)
		if entry.Sequence != expectedSequence {
			t.Fatalf("entry %s sequence = %d; want %d", entry.ID, entry.Sequence, expectedSequence)
		}
		running += entry.DeltaPoints
		if entry.BalanceAfter != running {
			t.Fatalf("entry %s balance_after = %d; replaying the ledger gives %d", entry.ID, entry.BalanceAfter, running)
		}
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != running {
		t.Fatalf("account balance = %d but the ledger replays to %d", account.CurrentPoints, running)
	}
	if account.CurrentPoints != account.LifetimeCreditedPoints-account.LifetimeDebitedPoints {
		t.Fatalf("account aggregates disagree: %+v", account)
	}
	if account.CurrentPoints != 260 {
		t.Fatalf("balance = %d; want 100+250-120+30 = 260", account.CurrentPoints)
	}
}

// TestLoyalty_LedgerRowsAreImmutable asserts the ledger offers no update or
// delete path through the service API at all — corrections go through
// compensating entries, which is what makes history worth reading.
func TestLoyalty_LedgerRowsAreImmutable(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_immutable"))
	entry := env.credit(t, businessA, "vs_a", profile, 100, "ord_imm")

	before, err := scanLoyaltyEntry(env.pool.QueryRow(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries WHERE id=$1`, entry.ID))
	if err != nil {
		t.Fatal(err)
	}
	// Reverse it: the compensating entry appears, the original is untouched.
	if _, _, err = env.service.ReverseLoyaltyEntry(ctx, loyaltyActor(businessA, "vs_a"), Scope{Global: true}, entry.ID,
		ReverseLoyaltyEntryInput{Reason: "posted against the wrong customer", CommandRef: "idem_rev_1"}); err != nil {
		t.Fatal(err)
	}
	after, err := scanLoyaltyEntry(env.pool.QueryRow(ctx, `SELECT `+loyaltyEntryCols+` FROM crm_loyalty_entries WHERE id=$1`, entry.ID))
	if err != nil {
		t.Fatal(err)
	}
	if after.DeltaPoints != before.DeltaPoints || after.BalanceAfter != before.BalanceAfter ||
		after.Sequence != before.Sequence || after.EntryType != before.EntryType {
		t.Fatalf("original entry changed after reversal:\nbefore %+v\nafter  %+v", before, after)
	}
}

// TestLoyalty_NegativeBalanceIsRefusedAtEveryLayer covers both the application
// check and the database constraint behind it.
func TestLoyalty_NegativeBalanceIsRefusedAtEveryLayer(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_negative"))
	env.credit(t, businessA, "vs_a", profile, 50, "ord_neg")

	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 51, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_over", CommandRef: "cmd_over"}); err == nil {
		t.Fatal("a debit larger than the balance succeeded")
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 50 {
		t.Fatalf("balance = %d after a refused debit; want 50 untouched", account.CurrentPoints)
	}
	// The constraint is the backstop if application logic is ever bypassed.
	if _, err := env.pool.Exec(ctx, `UPDATE crm_loyalty_accounts SET current_points=-1 WHERE id=$1`, account.ID); err == nil {
		t.Fatal("the database accepted a negative loyalty balance")
	}
}

func TestLoyalty_OverflowAndZeroPointsAreRejected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_overflow"))

	for _, points := range []int64{0, -5, MaxLoyaltyPoints + 1, 1 << 62} {
		if _, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
			CustomerProfileRef: profile.ID, Points: points, SourceType: LoyaltySourceOrder,
			SourceReference: "ord_of", CommandRef: "cmd_of"}); err == nil {
			t.Fatalf("credit of %d points was accepted", points)
		}
	}
	// Two credits that individually fit but together exceed the cap.
	half := MaxLoyaltyPoints / 2
	env.credit(t, businessA, "vs_a", profile, half, "ord_half_1")
	env.credit(t, businessA, "vs_a", profile, half, "ord_half_2")
	if _, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: profile.ID, Points: 1000, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_half_3", CommandRef: "cmd_half_3"}); err == nil {
		t.Fatal("a credit past the maximum balance was accepted")
	}
}

// TestLoyalty_DuplicateCommandAndSourceEventDoNotDoubleCredit covers the two
// durable dedupe paths: a retried command key, and a redelivered producer
// event arriving under a different key.
func TestLoyalty_DuplicateCommandAndSourceEventDoNotDoubleCredit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_idem"))

	first := env.credit(t, businessA, "vs_a", profile, 100, "ord_dup")
	// Same command key: replays the original entry.
	replay, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: profile.ID, Points: 100, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_dup", CommandRef: "cmd_ord_dup"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID {
		t.Fatalf("retry posted a new entry %s; want the original %s replayed", replay.ID, first.ID)
	}
	// Same producer event under a fresh command key: still one credit.
	redelivery, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: profile.ID, Points: 100, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_dup", CommandRef: "cmd_ord_dup_retry"})
	if err != nil {
		t.Fatal(err)
	}
	if redelivery.ID != first.ID {
		t.Fatalf("a redelivered order event posted %s; the same order must credit once", redelivery.ID)
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 100 {
		t.Fatalf("balance = %d after two redeliveries; want 100", account.CurrentPoints)
	}
}

// ---------------------------------------------------------------------------
// Point lots and FEFO
// ---------------------------------------------------------------------------

func TestLoyaltyLots_ExpiryStampReflectsPolicyAtIssueTime(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_lots"))

	// NEVER: no expiry.
	env.credit(t, businessA, "vs_a", profile, 100, "ord_never")
	account := env.loyaltyAccount(t, businessA, profile.ID)
	lots := env.lots(t, businessA, account.ID)
	if len(lots) != 1 || lots[0].ExpiresAt != nil {
		t.Fatalf("NEVER lot = %+v; want no expiry", lots)
	}

	// Switch to FIXED_DAYS: only the next credit gets a window.
	fixed := LoyaltyExpiryFixedDays
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &fixed, ExpiryDays: intPtr(10), ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	env.credit(t, businessA, "vs_a", profile, 60, "ord_fixed")
	lots = env.lots(t, businessA, account.ID)
	if len(lots) != 2 {
		t.Fatalf("lots = %d; want 2", len(lots))
	}
	if lots[0].ExpiresAt != nil {
		t.Fatalf("the pre-existing lot gained an expiry: %+v", lots[0])
	}
	if lots[1].ExpiresAt == nil || !lots[1].ExpiresAt.Equal(env.now.AddDate(0, 0, 10)) {
		t.Fatalf("new lot expiry = %+v; want %s", lots[1].ExpiresAt, env.now.AddDate(0, 0, 10))
	}
}

// TestLoyaltyLots_FEFOConsumesEarliestExpiryFirst pins the consumption order:
// the points closest to dying are spent first, and non-expiring points last,
// so a customer never watches a balance expire while an immortal lot sat
// untouched.
func TestLoyaltyLots_FEFOConsumesEarliestExpiryFirst(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_fefo"))

	// A non-expiring lot first, then a far lot, then a near lot.
	env.credit(t, businessA, "vs_a", profile, 100, "ord_immortal")
	fixed := LoyaltyExpiryFixedDays
	version := program.Version
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryPolicy: &fixed, ExpiryDays: intPtr(90), ExpectedVersion: version}); err != nil {
		t.Fatal(err)
	}
	env.credit(t, businessA, "vs_a", profile, 50, "ord_far")
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryDays: intPtr(10), ExpectedVersion: version + 1}); err != nil {
		t.Fatal(err)
	}
	env.credit(t, businessA, "vs_a", profile, 30, "ord_near")

	account := env.loyaltyAccount(t, businessA, profile.ID)
	// Spend 60: all 30 of the near lot, then 30 of the far lot, immortal untouched.
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 60, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_spend", CommandRef: "cmd_spend"}); err != nil {
		t.Fatal(err)
	}
	lots := env.lots(t, businessA, account.ID)
	byRef := map[string]LoyaltyPointLot{}
	for _, lot := range lots {
		switch {
		case lot.ExpiresAt == nil:
			byRef["immortal"] = lot
		case lot.ExpiresAt.Equal(env.now.AddDate(0, 0, 90)):
			byRef["far"] = lot
		default:
			byRef["near"] = lot
		}
	}
	if byRef["near"].RemainingPoints != 0 {
		t.Fatalf("nearest-expiring lot has %d left; FEFO must spend it first", byRef["near"].RemainingPoints)
	}
	if byRef["far"].RemainingPoints != 20 {
		t.Fatalf("far lot has %d left; want 20 after partial consumption", byRef["far"].RemainingPoints)
	}
	if byRef["immortal"].RemainingPoints != 100 {
		t.Fatalf("non-expiring lot has %d left; it must be consumed last", byRef["immortal"].RemainingPoints)
	}

	// The allocation record must account for exactly the points spent.
	var allocated int64
	if err := env.pool.QueryRow(ctx, `SELECT coalesce(sum(consumed_points),0) FROM crm_loyalty_lot_consumptions
		WHERE business_id=$1 AND loyalty_account_id=$2`, businessA, account.ID).Scan(&allocated); err != nil {
		t.Fatal(err)
	}
	if allocated != 60 {
		t.Fatalf("allocations total %d; want exactly the 60 points debited", allocated)
	}
}

// TestLoyaltyLots_ExpiredLotsCannotFundADebit is the offline-safety property:
// the sweeper has not run, but the expired lot is still unspendable because
// the debit settles expiry before it looks at the balance.
func TestLoyaltyLots_ExpiredLotsCannotFundADebit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(10))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_expired_debit"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_will_expire")

	// Move the clock past the window without running the sweeper.
	env.now = env.now.AddDate(0, 0, 11)

	summary, err := env.service.LoyaltySummaryFor(ctx, actor, profile)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AvailablePoints != 0 || summary.PendingExpiryPoints != 100 {
		t.Fatalf("summary = %+v; expired points must not read as available even before the sweeper runs", summary)
	}
	if _, err = env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 10, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_late_spend", CommandRef: "cmd_late_spend"}); err == nil {
		t.Fatal("expired points were spent because the sweeper had not run")
	}
	// The refused debit rolled back, inline settlement included — a rejected
	// command must leave no trace. The bookkeeping is simply still owed.
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 100 {
		t.Fatalf("balance = %d after a rolled-back debit; want the unsettled 100", account.CurrentPoints)
	}
	var expireEntries int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries
		WHERE loyalty_account_id=$1 AND entry_type=$2`, account.ID, LoyaltyExpire).Scan(&expireEntries); err != nil {
		t.Fatal(err)
	}
	if expireEntries != 0 {
		t.Fatalf("EXPIRE entries = %d; a rolled-back debit must not leave a settlement behind", expireEntries)
	}

	// A command that does commit settles the debt on the way through: a manual
	// credit of 25 lands on an account whose 100 expired points are gone.
	if _, _, err = env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 25,
		Reason: "goodwill after the expiry", CommandRef: "idem_after_expiry"}); err != nil {
		t.Fatal(err)
	}
	account = env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 25 {
		t.Fatalf("balance = %d; want the 100 expired away and only the new 25 left", account.CurrentPoints)
	}
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries
		WHERE loyalty_account_id=$1 AND entry_type=$2`, account.ID, LoyaltyExpire).Scan(&expireEntries); err != nil {
		t.Fatal(err)
	}
	if expireEntries != 1 {
		t.Fatalf("EXPIRE entries = %d; want exactly one settlement", expireEntries)
	}
}

func TestLoyaltyLots_RemainingPointsConstraintHolds(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_lot_ck"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_ck")
	account := env.loyaltyAccount(t, businessA, profile.ID)
	lots := env.lots(t, businessA, account.ID)

	for _, value := range []int64{-1, 101} {
		if _, err := env.pool.Exec(ctx, `UPDATE crm_loyalty_point_lots SET remaining_points=$1 WHERE id=$2`,
			value, lots[0].ID); err == nil {
			t.Fatalf("the database accepted remaining_points=%d against issued_points=100", value)
		}
	}
}

// ---------------------------------------------------------------------------
// Redemption policy
// ---------------------------------------------------------------------------

func TestLoyaltyRedemption_MinimumAndIncrementAreEnforced(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	minimum, increment := int64(100), int64(50)
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		MinimumRedemptionPoints: &minimum, RedemptionIncrementPoints: &increment,
		ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_redeem_policy"))
	env.credit(t, businessA, "vs_a", profile, 500, "ord_redeem")

	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 50, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_r1", CommandRef: "cmd_r1"}); err == nil {
		t.Fatal("a redemption below the configured minimum was accepted")
	}
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 175, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_r2", CommandRef: "cmd_r2"}); err == nil {
		t.Fatal("a redemption off the configured increment was accepted")
	}
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 150, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_r3", CommandRef: "cmd_r3"}); err != nil {
		t.Fatalf("a compliant redemption was refused: %v", err)
	}
	// A manual debit is a correction, not a redemption, so the redemption
	// granularity rules deliberately do not apply to it.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionDebit, Points: 7,
		Reason: "removing points granted in error", CommandRef: "idem_manual_odd"}); err != nil {
		t.Fatalf("a manual debit was blocked by redemption granularity: %v", err)
	}
}

// TestLoyaltyProgramState_GatesEarnRedeemAdjustAndReversalDifferently pins the
// deliberate asymmetry: a paused or switched-off programme stops new activity
// but never traps a mistake the business has already made.
func TestLoyaltyProgramState_GatesEarnRedeemAdjustAndReversalDifferently(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_states"))
	credited := env.credit(t, businessA, "vs_a", profile, 500, "ord_states")

	paused := LoyaltyProgramPaused
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		Status: &paused, ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: profile.ID, Points: 10, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_paused", CommandRef: "cmd_paused"}); err == nil {
		t.Fatal("a paused programme still earned points")
	}
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 10, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_paused_r", CommandRef: "cmd_paused_r"}); err == nil {
		t.Fatal("a paused programme still redeemed points")
	}
	// Corrections remain possible while paused.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 5,
		Reason: "compensation agreed before the pause", CommandRef: "idem_paused_adj"}); err != nil {
		t.Fatalf("a manual adjustment was blocked while paused: %v", err)
	}

	disabled := LoyaltyProgramDisabled
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		Status: &disabled, ExpectedVersion: program.Version + 1}); err != nil {
		t.Fatal(err)
	}
	// A switched-off programme grants nothing new, by hand or otherwise.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 5,
		Reason: "should not be possible", CommandRef: "idem_disabled_adj"}); err == nil {
		t.Fatal("a disabled programme still granted points by hand")
	}
	// But an entry posted while it was on can still be corrected: a business
	// that switches loyalty off must still be able to fix its own mistakes.
	reversal, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "credited the wrong order", CommandRef: "idem_disabled_rev"})
	if err != nil {
		t.Fatalf("reversal was blocked on a disabled programme: %v", err)
	}
	if reversal.EntryType != LoyaltyReversal || reversal.DeltaPoints != -500 {
		t.Fatalf("reversal = %+v; want a REVERSAL of -500", reversal)
	}
}

// ---------------------------------------------------------------------------
// Redacted and merged relationships
// ---------------------------------------------------------------------------

// TestLoyalty_RedactedCustomerIsNonActionable checks the Section 8A privacy
// decision reaches loyalty: the account stops accepting movement, disappears
// from Customer 360, and its ledger — which holds no personal data — survives
// as audit evidence.
func TestLoyalty_RedactedCustomerIsNonActionable(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_redact_loyalty"))
	env.credit(t, businessA, "vs_a", profile, 400, "ord_redact")
	account := env.loyaltyAccount(t, businessA, profile.ID)

	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{
			SourceService: "customer-service", SourceVersion: 1,
			PrivacySubjectRef: "cus_redact_loyalty", Action: PrivacyActionRedact, ObservedAt: env.now})
	}); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := env.pool.QueryRow(ctx, `SELECT status FROM crm_loyalty_accounts WHERE id=$1`, account.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != LoyaltyAccountRedacted {
		t.Fatalf("account status = %s after redaction; want REDACTED", status)
	}
	// The ledger is retained: it holds integer movements and a profile
	// reference, no personal data to erase.
	var entries int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries WHERE loyalty_account_id=$1`,
		account.ID).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if entries == 0 {
		t.Fatal("redaction destroyed the points ledger; it carries no PII and is the evidence a balance was legitimate")
	}
	// No adjustment can reach the customer any more.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 10,
		Reason: "should be impossible", CommandRef: "idem_redacted"}); err == nil {
		t.Fatal("points were granted to a redacted customer")
	}
	// And the loyalty summary reports nothing.
	reloaded, err := scanProfile(env.pool.QueryRow(ctx, `SELECT `+profileCols+` FROM vendor_customer_profiles WHERE id=$1`, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := env.service.LoyaltySummaryFor(ctx, actor, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Enrollment != LoyaltyNotEnrolled || summary.AvailablePoints != 0 {
		t.Fatalf("redacted summary = %+v; loyalty data must be suppressed", summary)
	}
}

// TestLoyalty_MarketingSuppressionKeepsTheBalance pins the separation: opting
// out of marketing is not forfeiting points a customer earned.
func TestLoyalty_MarketingSuppressionKeepsTheBalance(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_suppress_loyalty"))
	env.credit(t, businessA, "vs_a", profile, 300, "ord_suppress")

	for _, action := range []PrivacyAction{PrivacyActionMarketingSuppress, PrivacyActionRestrict} {
		if err := env.tx(t, func(tx pgx.Tx) error {
			return env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{
				SourceService: "customer-service", SourceVersion: 1,
				PrivacySubjectRef: "cus_suppress_loyalty", Action: action, ObservedAt: env.now})
		}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.Status != LoyaltyAccountActive || account.CurrentPoints != 300 {
		t.Fatalf("account = %+v; marketing suppression and restriction must not confiscate points", account)
	}
	reloaded, err := scanProfile(env.pool.QueryRow(ctx, `SELECT `+profileCols+` FROM vendor_customer_profiles WHERE id=$1`, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := env.service.LoyaltySummaryFor(ctx, actor, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AvailablePoints != 300 {
		t.Fatalf("available = %d after suppression; want the balance intact", summary.AvailablePoints)
	}
}
