package crm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Section 8D true-concurrency tests.
//
// These are the tests the design exists for. Every one runs two real
// transactions against real PostgreSQL and releases them together at a
// barrier, so what is being checked is the lock ordering and the constraint
// set, not a Go mutex that would not survive a second process.

// race runs two functions simultaneously behind a barrier and returns their
// errors. There is no sleeping: both goroutines block on the same channel and
// are released at once, so the interleaving is genuinely concurrent rather
// than staged by timing.
func race(first, second func() error) (error, error) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	var firstErr, secondErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; firstErr = first() }()
	go func() { defer wg.Done(); <-start; secondErr = second() }()
	close(start)
	wg.Wait()
	return firstErr, secondErr
}

func exactlyOneSucceeded(t *testing.T, label string, a, b error) {
	t.Helper()
	switch {
	case a == nil && b == nil:
		t.Fatalf("%s: both concurrent attempts succeeded", label)
	case a != nil && b != nil:
		t.Fatalf("%s: both concurrent attempts failed (%v / %v)", label, a, b)
	}
}

// mergeLoyaltyProfiles applies one authoritative guest-into-customer merge the
// way customer-service would.
func (e *testEnv) mergeLoyaltyProfiles(t *testing.T, guestRef, customerRef, eventID string) error {
	return e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyIdentityLink(context.Background(), tx, actorFor(businessA, "vs_a"), IdentityLinkInput{
			AuthorityService: "customer-service", AuthorityReference: "merge", AuthorityVersion: 1,
			EvidenceEventID: eventID,
			Source:          SubjectRef{Type: SubjectGuest, Ref: guestRef},
			Target:          SubjectRef{Type: SubjectCustomer, Ref: customerRef},
		})
	})
}

// TestLoyaltyConcurrency_TwoDebitsCannotOverdraw is the headline invariant:
// 100 points, two simultaneous 80-point debits, at most one may win and the
// balance may never go negative.
func TestLoyaltyConcurrency_TwoDebitsCannotOverdraw(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race_debit"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_race")

	debit := func(ref string) func() error {
		return func() error {
			_, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
				CustomerProfileRef: profile.ID, Points: 80, SourceType: LoyaltySourceOrder,
				SourceReference: ref, CommandRef: "cmd_" + ref})
			return err
		}
	}
	a, b := race(debit("ord_race_a"), debit("ord_race_b"))
	exactlyOneSucceeded(t, "concurrent debits", a, b)

	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 20 {
		t.Fatalf("balance = %d; want exactly 20 left after one 80-point debit", account.CurrentPoints)
	}
	if account.CurrentPoints != account.LifetimeCreditedPoints-account.LifetimeDebitedPoints {
		t.Fatalf("aggregates diverged under contention: %+v", account)
	}
	assertLedgerMatchesLots(t, env, businessA, account)
}

// TestLoyaltyConcurrency_FirstCreditCreatesOneAccount checks the lazy-creation
// race: two credits arriving together for a customer with no account must
// produce one account, not two balances for the same person.
func TestLoyaltyConcurrency_FirstCreditCreatesOneAccount(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race_create"))

	credit := func(ref string) func() error {
		return func() error {
			_, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
				CustomerProfileRef: profile.ID, Points: 50, SourceType: LoyaltySourceOrder,
				SourceReference: ref, CommandRef: "cmd_" + ref})
			return err
		}
	}
	a, b := race(credit("ord_create_a"), credit("ord_create_b"))
	if a != nil && b != nil {
		t.Fatalf("both first credits failed: %v / %v", a, b)
	}

	var accounts int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2`, businessA, profile.ID).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 {
		t.Fatalf("concurrent first credits created %d accounts; want exactly 1", accounts)
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	expected := int64(50)
	if a == nil && b == nil {
		expected = 100 // both credits are legitimate and distinct
	}
	if account.CurrentPoints != expected {
		t.Fatalf("balance = %d; want %d", account.CurrentPoints, expected)
	}
	assertLedgerMatchesLots(t, env, businessA, account)
}

// TestLoyaltyConcurrency_ExpirySweepVersusDebit puts a due lot between the
// sweeper and a debit. Whichever transaction wins, the points can be spent or
// expired but never both.
func TestLoyaltyConcurrency_ExpirySweepVersusDebit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(10))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race_expiry"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_race_exp")
	env.now = env.now.AddDate(0, 0, 11)

	sweepErr, debitErr := race(
		func() error { _, err := env.service.RunLoyaltyExpiryPass(ctx, 5); return err },
		func() error {
			_, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
				CustomerProfileRef: profile.ID, Points: 100, SourceType: LoyaltySourceOrder,
				SourceReference: "ord_race_spend", CommandRef: "cmd_race_spend"})
			return err
		})
	if sweepErr != nil {
		t.Fatalf("the sweeper failed under contention: %v", sweepErr)
	}
	// This is the invariant: whoever won, the expired lot could not be spent.
	if debitErr == nil {
		t.Fatal("an expired lot funded a debit that ran against the sweeper")
	}
	// The sweeper legitimately skips an account another transaction holds, so
	// drain the backlog before asserting the settled state; deferral is
	// acceptable, double consumption is not.
	for {
		swept, err := env.service.RunLoyaltyExpiryPass(ctx, 5)
		if err != nil {
			t.Fatal(err)
		}
		if swept == 0 {
			break
		}
	}

	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 0 {
		t.Fatalf("balance = %d; the expired points must be gone exactly once", account.CurrentPoints)
	}
	var expiries int
	var expired int64
	if err := env.pool.QueryRow(ctx, `SELECT count(*),coalesce(sum(-delta_points),0) FROM crm_loyalty_entries
		WHERE loyalty_account_id=$1 AND entry_type=$2`, account.ID, LoyaltyExpire).Scan(&expiries, &expired); err != nil {
		t.Fatal(err)
	}
	if expiries != 1 || expired != 100 {
		t.Fatalf("expiry entries = %d totalling %d; want one entry of 100 (no double consumption)", expiries, expired)
	}
	assertLedgerMatchesLots(t, env, businessA, account)
}

// TestLoyaltyConcurrency_DoubleReversal fires two reversals of the same entry
// at once under different command keys. Exactly one may post.
func TestLoyaltyConcurrency_DoubleReversal(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race_reversal"))
	credited := env.credit(t, businessA, "vs_a", profile, 400, "ord_race_rev")

	reverse := func(key string) func() error {
		return func() error {
			_, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
				ReverseLoyaltyEntryInput{Reason: "double-submitted correction", CommandRef: key})
			return err
		}
	}
	a, b := race(reverse("idem_race_rev_a"), reverse("idem_race_rev_b"))
	exactlyOneSucceeded(t, "concurrent reversals", a, b)

	var reversals int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries
		WHERE business_id=$1 AND reverses_entry_id=$2`, businessA, credited.ID).Scan(&reversals); err != nil {
		t.Fatal(err)
	}
	if reversals != 1 {
		t.Fatalf("%d reversals exist for one entry; the ledger must permit exactly one", reversals)
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 0 {
		t.Fatalf("balance = %d after one reversal of 400; want 0", account.CurrentPoints)
	}
	assertLedgerMatchesLots(t, env, businessA, account)
}

// TestLoyaltyConcurrency_MergeVersusCreditAndDebit races an identity merge
// against activity on the source account. Points must neither vanish nor
// duplicate, and the source must end up non-actionable either way.
func TestLoyaltyConcurrency_MergeVersusCreditAndDebit(t *testing.T) {
	for _, mode := range []string{"credit", "debit"} {
		t.Run(mode, func(t *testing.T) {
			env := newTestEnv(t, allFlagsOn())
			ctx := context.Background()
			actor := loyaltyActor(businessA, "vs_a")
			env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
			// customer-service only ever merges a guest into a customer, so the
			// source is the guest identity and the target the customer one.
			source := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_merge_src_"+mode))
			target := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_merge_tgt_"+mode))
			env.credit(t, businessA, "vs_a", source, 500, "ord_merge_src_"+mode)
			env.credit(t, businessA, "vs_a", target, 200, "ord_merge_tgt_"+mode)

			activity := func() error {
				if mode == "credit" {
					_, err := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
						CustomerProfileRef: source.ID, Points: 100, SourceType: LoyaltySourceOrder,
						SourceReference: "ord_race_merge_" + mode, CommandRef: "cmd_race_merge_" + mode})
					return err
				}
				_, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
					CustomerProfileRef: source.ID, Points: 100, SourceType: LoyaltySourceOrder,
					SourceReference: "ord_race_merge_" + mode, CommandRef: "cmd_race_merge_" + mode})
				return err
			}
			mergeErr, activityErr := race(
				func() error {
					return env.mergeLoyaltyProfiles(t, "gst_merge_src_"+mode, "cus_merge_tgt_"+mode, "evt_race_merge_"+mode)
				},
				activity)
			if mergeErr != nil {
				t.Fatalf("merge failed under contention: %v", mergeErr)
			}

			sourceAccount := env.loyaltyAccount(t, businessA, source.ID)
			targetAccount := env.loyaltyAccount(t, businessA, target.ID)
			if sourceAccount.Status != LoyaltyAccountMerged {
				t.Fatalf("source account status = %s; a merged-away account must not stay actionable", sourceAccount.Status)
			}
			if sourceAccount.CurrentPoints != 0 {
				t.Fatalf("source retained %d points after the merge", sourceAccount.CurrentPoints)
			}
			// Conservation: whatever the interleaving, the target holds every
			// point that legitimately existed and not one more.
			want := int64(700)
			if activityErr == nil {
				if mode == "credit" {
					want = 800
				} else {
					want = 600
				}
			}
			if targetAccount.CurrentPoints != want {
				t.Fatalf("target = %d points (activity err=%v); want %d — points were lost or duplicated",
					targetAccount.CurrentPoints, activityErr, want)
			}
			assertLedgerMatchesLots(t, env, businessA, targetAccount)
			assertLedgerMatchesLots(t, env, businessA, sourceAccount)
		})
	}
}

// TestLoyaltyConcurrency_MergeVersusExpiry checks that a merge racing the
// sweeper neither loses points nor hands the target a fresh expiry window.
func TestLoyaltyConcurrency_MergeVersusExpiry(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
	source := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_merge_exp_src"))
	target := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_merge_exp_tgt"))
	env.credit(t, businessA, "vs_a", source, 300, "ord_merge_exp")
	sourceAccount := env.loyaltyAccount(t, businessA, source.ID)
	originalExpiry := *env.lots(t, businessA, sourceAccount.ID)[0].ExpiresAt

	// Well inside the window: the sweeper has nothing to do, so the merge must
	// carry the lot across untouched.
	env.now = env.now.AddDate(0, 0, 10)
	mergeErr, sweepErr := race(
		func() error {
			return env.mergeLoyaltyProfiles(t, "gst_merge_exp_src", "cus_merge_exp_tgt", "evt_merge_exp")
		},
		func() error { _, err := env.service.RunLoyaltyExpiryPass(ctx, 5); return err })
	if mergeErr != nil || sweepErr != nil {
		t.Fatalf("merge=%v sweep=%v", mergeErr, sweepErr)
	}

	targetAccount := env.loyaltyAccount(t, businessA, target.ID)
	if targetAccount.CurrentPoints != 300 {
		t.Fatalf("target = %d points; want the full 300 transferred", targetAccount.CurrentPoints)
	}
	lots := env.lots(t, businessA, targetAccount.ID)
	if len(lots) != 1 || lots[0].ExpiresAt == nil {
		t.Fatalf("target lots = %+v; want one lot carrying an expiry", lots)
	}
	if !lots[0].ExpiresAt.Equal(originalExpiry) {
		t.Fatalf("transferred expiry = %s; want the original %s — a merge must not buy a fresh window",
			lots[0].ExpiresAt, originalExpiry)
	}

	// And the transferred points still expire on the original schedule.
	env.now = originalExpiry.Add(time.Hour)
	if _, err := env.service.RunLoyaltyExpiryPass(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if targetAccount = env.loyaltyAccount(t, businessA, target.ID); targetAccount.CurrentPoints != 0 {
		t.Fatalf("target = %d points past the original expiry; the window was extended by the merge", targetAccount.CurrentPoints)
	}
}

// TestLoyaltyConcurrency_RedactionVersusAdjustment races a privacy decision
// against a points grant. Once the redaction commits, no adjustment may
// survive against that customer.
func TestLoyaltyConcurrency_RedactionVersusAdjustment(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race_redact"))
	env.credit(t, businessA, "vs_a", profile, 200, "ord_race_redact")

	redactErr, adjustErr := race(
		func() error {
			return env.tx(t, func(tx pgx.Tx) error {
				return env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{
					SourceService: "customer-service", SourceVersion: 1,
					PrivacySubjectRef: "cus_race_redact", Action: PrivacyActionRedact, ObservedAt: env.now})
			})
		},
		func() error {
			_, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
				CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 50,
				Reason: "grant racing a redaction", CommandRef: "idem_race_redact"})
			return err
		})
	if redactErr != nil {
		t.Fatalf("redaction failed under contention: %v", redactErr)
	}

	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.Status != LoyaltyAccountRedacted {
		t.Fatalf("account status = %s; the privacy decision must win", account.Status)
	}
	// Whichever order the two transactions serialised in, no further movement
	// is possible now.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 10,
		Reason: "after redaction", CommandRef: "idem_after_redact"}); err == nil {
		t.Fatal("points were granted to a redacted customer after the redaction committed")
	}
	if adjustErr == nil && account.CurrentPoints != 250 {
		t.Fatalf("the adjustment reported success but the balance is %d", account.CurrentPoints)
	}
	assertLedgerMatchesLots(t, env, businessA, account)
}

// assertLedgerMatchesLots is the invariant every concurrency test closes with:
// the account aggregate, the replayed ledger and the surviving lots must all
// agree. If a race ever tore one of them, this is what catches it.
func assertLedgerMatchesLots(t *testing.T, env *testEnv, business string, account LoyaltyAccount) {
	t.Helper()
	ctx := context.Background()
	var ledgerTotal, credited, debited int64
	if err := env.pool.QueryRow(ctx, `SELECT coalesce(sum(delta_points),0),
		coalesce(sum(delta_points) FILTER (WHERE delta_points>0),0),
		coalesce(sum(-delta_points) FILTER (WHERE delta_points<0),0)
		FROM crm_loyalty_entries WHERE business_id=$1 AND loyalty_account_id=$2`,
		business, account.ID).Scan(&ledgerTotal, &credited, &debited); err != nil {
		t.Fatal(err)
	}
	fresh := env.loyaltyAccount(t, business, account.CustomerProfileID)
	if fresh.CurrentPoints != ledgerTotal {
		t.Fatalf("account %s balance %d but the ledger sums to %d", fresh.ID, fresh.CurrentPoints, ledgerTotal)
	}
	if fresh.LifetimeCreditedPoints != credited || fresh.LifetimeDebitedPoints != debited {
		t.Fatalf("account %s lifetime totals (%d/%d) disagree with the ledger (%d/%d)",
			fresh.ID, fresh.LifetimeCreditedPoints, fresh.LifetimeDebitedPoints, credited, debited)
	}
	var lotTotal int64
	if err := env.pool.QueryRow(ctx, `SELECT coalesce(sum(remaining_points),0) FROM crm_loyalty_point_lots
		WHERE business_id=$1 AND loyalty_account_id=$2`, business, account.ID).Scan(&lotTotal); err != nil {
		t.Fatal(err)
	}
	if lotTotal != fresh.CurrentPoints {
		t.Fatalf("account %s balance %d but its lots hold %d remaining points", fresh.ID, fresh.CurrentPoints, lotTotal)
	}
	// Sequences must be dense and unique.
	var maxSequence, entryCount int64
	if err := env.pool.QueryRow(ctx, `SELECT coalesce(max(sequence),0),count(*) FROM crm_loyalty_entries
		WHERE business_id=$1 AND loyalty_account_id=$2`, business, account.ID).Scan(&maxSequence, &entryCount); err != nil {
		t.Fatal(err)
	}
	if maxSequence != entryCount {
		t.Fatalf("account %s has %d entries but a maximum sequence of %d; ordering has a gap or a repeat",
			fresh.ID, entryCount, maxSequence)
	}
}
