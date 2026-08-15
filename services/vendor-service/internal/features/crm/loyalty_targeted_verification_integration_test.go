package crm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// installLoyaltyEntryBarrier adds a test-only database barrier at the actual
// immutable-ledger insert. The caller holds a session advisory lock; a
// production transaction reaches the trigger only after it has acquired the
// real profile/account/lot locks. This lets tests prove ordering from observed
// PostgreSQL lock state rather than from process timing.
func installLoyaltyEntryBarrier(t *testing.T, env *testEnv, entryType string, key int64) func() {
	t.Helper()
	functionName := fmt.Sprintf("crm_test_loyalty_entry_barrier_%d", key)
	triggerName := fmt.Sprintf("crm_test_loyalty_entry_barrier_%d", key)
	if _, err := env.pool.Exec(context.Background(), fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.entry_type = '%s' THEN
				PERFORM pg_advisory_xact_lock(%d);
			END IF;
			RETURN NEW;
		END $$`, functionName, entryType, key)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(context.Background(), fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON crm_loyalty_entries
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	conn, err := env.pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(context.Background(), `SELECT pg_advisory_lock($1)`, key); err != nil {
		conn.Release()
		t.Fatal(err)
	}
	var once sync.Once
	unlock := func() {
		once.Do(func() {
			_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
			conn.Release()
		})
	}
	t.Cleanup(func() {
		unlock()
		_, _ = env.pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON crm_loyalty_entries`, triggerName))
		_, _ = env.pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})
	return unlock
}

func waitForLoyaltyDBWaiters(t *testing.T, env *testEnv, pattern string, advisoryOnly bool, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		query := `SELECT count(*) FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`
		args := []any{pattern}
		if advisoryOnly {
			query += ` AND wait_event='advisory'`
		}
		if err := env.pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("PostgreSQL never exposed %d waiting transaction(s) for %q", want, pattern)
}

func applyTargetedLoyaltyMerge(ctx context.Context, env *testEnv, sourceRef, targetRef, eventID string) error {
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = env.service.ApplyIdentityLink(ctx, tx, actorFor(businessA, "vs_a"), IdentityLinkInput{
		AuthorityService: "customer-service", AuthorityReference: "targeted-verification", AuthorityVersion: 1,
		EvidenceEventID: eventID,
		Source:          SubjectRef{Type: SubjectGuest, Ref: sourceRef},
		Target:          SubjectRef{Type: SubjectCustomer, Ref: targetRef},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func assertNoOrphanLoyaltyConsumptions(t *testing.T, env *testEnv) {
	t.Helper()
	var count int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*)
		FROM crm_loyalty_lot_consumptions c
		LEFT JOIN crm_loyalty_accounts a ON a.id=c.loyalty_account_id AND a.business_id=c.business_id
		LEFT JOIN crm_loyalty_entries e ON e.id=c.debit_entry_id AND e.business_id=c.business_id
		LEFT JOIN crm_loyalty_point_lots l ON l.id=c.lot_id AND l.business_id=c.business_id
		WHERE a.id IS NULL OR e.id IS NULL OR l.id IS NULL`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("orphan loyalty consumption allocations=%d", count)
	}
}

func TestLoyaltyTargeted_MergeVersusReversalDeterministicOrderings(t *testing.T) {
	for _, ordering := range []string{"reversal-first", "merge-first"} {
		t.Run(ordering, func(t *testing.T) {
			env := newTestEnv(t, allFlagsOn())
			ctx := context.Background()
			actor := loyaltyActor(businessA, "vs_a")
			env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
			sourceRef := "gst_targeted_" + strings.ReplaceAll(ordering, "-", "_")
			targetRef := "cus_targeted_" + strings.ReplaceAll(ordering, "-", "_")
			source := env.seedProfile(t, businessA, "vs_a", guestSubject(sourceRef))
			target := env.seedProfile(t, businessA, "vs_a", customerSubject(targetRef))
			env.credit(t, businessA, "vs_a", source, 500, "ord_targeted_source_"+ordering)
			env.credit(t, businessA, "vs_a", target, 200, "ord_targeted_target_"+ordering)
			sourceAccount := env.loyaltyAccount(t, businessA, source.ID)
			originalExpiry := *env.lots(t, businessA, sourceAccount.ID)[0].ExpiresAt
			debit, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
				CustomerProfileRef: source.ID, Points: 100, SourceType: LoyaltySourceOrder,
				SourceReference: "ord_targeted_debit_" + ordering, CommandRef: "cmd_targeted_debit_" + ordering,
			})
			if err != nil {
				t.Fatal(err)
			}

			var reversalErr, mergeErr error
			if ordering == "reversal-first" {
				unlock := installLoyaltyEntryBarrier(t, env, LoyaltyReversal, 88101)
				reversalDone := make(chan error, 1)
				go func() {
					_, _, e := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, debit.ID,
						ReverseLoyaltyEntryInput{Reason: "targeted reversal-first verification", CommandRef: "cmd_targeted_rev_first"})
					reversalDone <- e
				}()
				waitForLoyaltyDBWaiters(t, env, "%INSERT INTO crm_loyalty_entries%", true, 1)
				mergeDone := make(chan error, 1)
				go func() {
					mergeDone <- applyTargetedLoyaltyMerge(ctx, env, sourceRef, targetRef, "evt_targeted_rev_first")
				}()
				waitForLoyaltyDBWaiters(t, env, "%SELECT id FROM vendor_customer_profiles WHERE business_id%", false, 1)
				unlock()
				reversalErr, mergeErr = <-reversalDone, <-mergeDone
			} else {
				unlock := installLoyaltyEntryBarrier(t, env, LoyaltyMergeOut, 88102)
				mergeDone := make(chan error, 1)
				go func() {
					mergeDone <- applyTargetedLoyaltyMerge(ctx, env, sourceRef, targetRef, "evt_targeted_merge_first")
				}()
				waitForLoyaltyDBWaiters(t, env, "%INSERT INTO crm_loyalty_entries%", true, 1)
				reversalDone := make(chan error, 1)
				go func() {
					_, _, e := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, debit.ID,
						ReverseLoyaltyEntryInput{Reason: "targeted merge-first verification", CommandRef: "cmd_targeted_merge_first"})
					reversalDone <- e
				}()
				waitForLoyaltyDBWaiters(t, env, "%crm_loyalty_profile_fence%", false, 1)
				unlock()
				mergeErr, reversalErr = <-mergeDone, <-reversalDone
			}
			if mergeErr != nil {
				t.Fatalf("merge failed: %v", mergeErr)
			}
			wantTarget, wantReversals := int64(600), 0
			if ordering == "reversal-first" {
				wantTarget, wantReversals = 700, 1
				if reversalErr != nil {
					t.Fatalf("reversal-first reversal failed: %v", reversalErr)
				}
			} else if reversalErr == nil || !strings.Contains(strings.ToLower(reversalErr.Error()), "merged") {
				t.Fatalf("merge-first reversal error=%v; want deterministic merged-source rejection", reversalErr)
			}

			sourceAccount = env.loyaltyAccount(t, businessA, source.ID)
			targetAccount := env.loyaltyAccount(t, businessA, target.ID)
			if sourceAccount.Status != LoyaltyAccountMerged || sourceAccount.CurrentPoints != 0 {
				t.Fatalf("source remains actionable: %+v", sourceAccount)
			}
			if targetAccount.CurrentPoints != wantTarget {
				t.Fatalf("target balance=%d want=%d", targetAccount.CurrentPoints, wantTarget)
			}
			var reversals, mergeOut, mergeIn int
			if err = env.pool.QueryRow(ctx, `SELECT
				count(*) FILTER (WHERE reverses_entry_id=$2),
				count(*) FILTER (WHERE loyalty_account_id=$3 AND entry_type=$4),
				count(*) FILTER (WHERE loyalty_account_id=$5 AND entry_type=$6)
				FROM crm_loyalty_entries WHERE business_id=$1`, businessA, debit.ID, sourceAccount.ID,
				LoyaltyMergeOut, targetAccount.ID, LoyaltyMergeIn).Scan(&reversals, &mergeOut, &mergeIn); err != nil {
				t.Fatal(err)
			}
			if reversals != wantReversals || mergeOut != 1 || mergeIn != 1 {
				t.Fatalf("reversals=%d merge_out=%d merge_in=%d; want %d/1/1", reversals, mergeOut, mergeIn, wantReversals)
			}
			var transferredExpiries int
			if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_point_lots l
				JOIN crm_loyalty_entries e ON e.id=l.source_entry_id AND e.business_id=l.business_id
				WHERE l.business_id=$1 AND l.loyalty_account_id=$2 AND e.entry_type=$3 AND l.expires_at=$4`,
				businessA, targetAccount.ID, LoyaltyMergeIn, originalExpiry).Scan(&transferredExpiries); err != nil {
				t.Fatal(err)
			}
			if transferredExpiries != 1 {
				t.Fatalf("transferred lots retaining original expiry=%d want=1", transferredExpiries)
			}
			var originalRows int
			if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries
				WHERE business_id=$1 AND id=$2 AND entry_type=$3 AND delta_points=-100`, businessA, debit.ID, LoyaltyRedeem).Scan(&originalRows); err != nil || originalRows != 1 {
				t.Fatalf("original immutable debit rows=%d err=%v", originalRows, err)
			}
			assertLedgerMatchesLots(t, env, businessA, sourceAccount)
			assertLedgerMatchesLots(t, env, businessA, targetAccount)
			assertNoOrphanLoyaltyConsumptions(t, env)
		})
	}
}

func TestLoyaltyTargeted_ExpiryWorkersCoordinateAcrossProcesses(t *testing.T) {
	t.Run("two workers divide the same due population", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(5))
		profiles := []Profile{
			env.seedProfile(t, businessA, "vs_a", customerSubject("cus_exp_worker_a")),
			env.seedProfile(t, businessA, "vs_a", customerSubject("cus_exp_worker_b")),
		}
		for i, profile := range profiles {
			env.credit(t, businessA, "vs_a", profile, 40, fmt.Sprintf("ord_exp_worker_%d_a", i))
			env.credit(t, businessA, "vs_a", profile, 60, fmt.Sprintf("ord_exp_worker_%d_b", i))
		}
		env.now = env.now.AddDate(0, 0, 6)
		unlock := installLoyaltyEntryBarrier(t, env, LoyaltyExpire, 88201)
		start := make(chan struct{})
		results := make(chan struct {
			settled int
			err     error
		}, 2)
		for range 2 {
			go func() {
				<-start
				settled, err := env.service.RunLoyaltyExpiryPass(ctx, 1)
				results <- struct {
					settled int
					err     error
				}{settled, err}
			}()
		}
		close(start)
		// One worker owns the programme row and has reached the immutable
		// ledger insert. The other has already skipped that worker's account,
		// claimed the second account, and is waiting on the shared programme
		// row before it posts its own sequence. This is the actual two-level
		// coordination mechanism; two workers cannot both reach the insert at
		// once because programme configuration is itself read FOR UPDATE.
		waitForLoyaltyDBWaiters(t, env, "%INSERT INTO crm_loyalty_entries%", true, 1)
		waitForLoyaltyDBWaiters(t, env, "%crm_loyalty_program_configs%FOR UPDATE%", false, 1)
		unlock()
		total := 0
		for range 2 {
			result := <-results
			if result.err != nil {
				t.Fatal(result.err)
			}
			total += result.settled
		}
		if total != 2 {
			t.Fatalf("two workers settled %d accounts; want 2", total)
		}
		for _, profile := range profiles {
			account := env.loyaltyAccount(t, businessA, profile.ID)
			if account.CurrentPoints != 0 || account.CurrentPoints < 0 {
				t.Fatalf("account %s balance=%d after expiration", account.ID, account.CurrentPoints)
			}
			var entries int
			var expired int64
			if err := env.pool.QueryRow(ctx, `SELECT count(*),coalesce(sum(-delta_points),0)
				FROM crm_loyalty_entries WHERE loyalty_account_id=$1 AND entry_type=$2`, account.ID, LoyaltyExpire).Scan(&entries, &expired); err != nil {
				t.Fatal(err)
			}
			if entries != 1 || expired != 100 {
				t.Fatalf("account %s expiry entries=%d points=%d want 1/100", account.ID, entries, expired)
			}
			for _, lot := range env.lots(t, businessA, account.ID) {
				if lot.RemainingPoints != 0 {
					t.Fatalf("lot %s retained %d due points", lot.ID, lot.RemainingPoints)
				}
			}
			assertLedgerMatchesLots(t, env, businessA, account)
		}
		assertNoOrphanLoyaltyConsumptions(t, env)
	})

	t.Run("same account is skipped while another worker owns it", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(5))
		profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_exp_worker_same"))
		env.credit(t, businessA, "vs_a", profile, 30, "ord_exp_same_a")
		env.credit(t, businessA, "vs_a", profile, 70, "ord_exp_same_b")
		env.now = env.now.AddDate(0, 0, 6)
		unlock := installLoyaltyEntryBarrier(t, env, LoyaltyExpire, 88202)
		firstDone := make(chan error, 1)
		go func() {
			settled, err := env.service.RunLoyaltyExpiryPass(ctx, 1)
			if err == nil && settled != 1 {
				err = fmt.Errorf("first worker settled %d accounts; want 1", settled)
			}
			firstDone <- err
		}()
		waitForLoyaltyDBWaiters(t, env, "%INSERT INTO crm_loyalty_entries%", true, 1)
		secondSettled, secondErr := env.service.RunLoyaltyExpiryPass(ctx, 1)
		if secondErr != nil || secondSettled != 0 {
			t.Fatalf("second worker settled=%d err=%v; want a safe SKIP LOCKED result", secondSettled, secondErr)
		}
		unlock()
		if err := <-firstDone; err != nil {
			t.Fatal(err)
		}
		account := env.loyaltyAccount(t, businessA, profile.ID)
		var entries int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_entries
			WHERE loyalty_account_id=$1 AND entry_type=$2`, account.ID, LoyaltyExpire).Scan(&entries); err != nil {
			t.Fatal(err)
		}
		if entries != 1 || account.CurrentPoints != 0 {
			t.Fatalf("same-account expiry entries=%d balance=%d want 1/0", entries, account.CurrentPoints)
		}
		assertLedgerMatchesLots(t, env, businessA, account)
	})
}

func TestLoyaltyTargeted_RestrictAndMarketingSuppressFollowSection8A(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	restricted := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_loyalty_restrict"))
	env.credit(t, businessA, "vs_a", restricted, 300, "ord_loyalty_restrict")
	if err := env.applyPrivacy(t, businessA, "vs_a", &restricted, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_loyalty_restrict",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}
	restricted = env.reload(t, businessA, restricted.ID)
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, restricted)
	if err != nil || view.Loyalty == nil || view.Loyalty.Enrollment != LoyaltyEnrolled || view.Loyalty.AvailablePoints != 300 {
		t.Fatalf("restricted Customer360 loyalty=%+v err=%v", view.Loyalty, err)
	}
	if _, err = env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: restricted.ID, Points: 50, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_loyalty_restrict_debit", CommandRef: "cmd_loyalty_restrict_debit",
	}); err != nil {
		t.Fatalf("RESTRICT unexpectedly blocked lawful points accounting: %v", err)
	}
	consent, err := env.service.ConsentSummaryFor(ctx, actor, restricted)
	if err != nil {
		t.Fatal(err)
	}
	for channel, state := range consent.Marketing {
		if state.Eligible {
			t.Fatalf("restricted profile remained marketing-targetable on %s", channel)
		}
	}

	suppressed := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_loyalty_marketing_suppress"))
	env.credit(t, businessA, "vs_a", suppressed, 175, "ord_loyalty_marketing_suppress")
	if err = env.applyPrivacy(t, businessA, "vs_a", &suppressed, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_loyalty_marketing_suppress",
		Action: PrivacyActionMarketingSuppress, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}
	suppressed = env.reload(t, businessA, suppressed.ID)
	if suppressed.PrivacyState != PrivacyActive {
		t.Fatalf("MARKETING_SUPPRESS changed privacy state to %s", suppressed.PrivacyState)
	}
	account := env.loyaltyAccount(t, businessA, suppressed.ID)
	if account.CurrentPoints != 175 || account.Status != LoyaltyAccountActive {
		t.Fatalf("marketing suppression changed loyalty account: %+v", account)
	}
	if _, err = env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: suppressed.ID, Points: 25, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_loyalty_marketing_after", CommandRef: "cmd_loyalty_marketing_after",
	}); err != nil {
		t.Fatalf("marketing suppression incorrectly blocked accounting: %v", err)
	}
	if account = env.loyaltyAccount(t, businessA, suppressed.ID); account.CurrentPoints != 200 {
		t.Fatalf("marketing-suppressed balance=%d want=200", account.CurrentPoints)
	}
}

func TestLoyaltyTargeted_MergedSourceCannotReenrolOrMutate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	source := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_merged_mutations"))
	target := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_merged_mutations"))
	original := env.credit(t, businessA, "vs_a", source, 300, "ord_merged_mutations_source")
	env.credit(t, businessA, "vs_a", target, 100, "ord_merged_mutations_target")
	if err := applyTargetedLoyaltyMerge(ctx, env, "gst_merged_mutations", "cus_merged_mutations", "evt_merged_mutations"); err != nil {
		t.Fatal(err)
	}
	merged := env.reload(t, businessA, source.ID)
	summary, err := env.service.LoyaltySummaryFor(ctx, actor, merged)
	if err != nil || summary.Enrollment != LoyaltyNotEnrolled {
		t.Fatalf("merged-source public summary=%+v err=%v", summary, err)
	}

	assertRejected := func(label string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "merged") {
			t.Fatalf("%s error=%v; want merged-source rejection", label, err)
		}
	}
	_, _, err = env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: source.ID, Direction: LoyaltyDirectionCredit, Points: 10,
		Reason: "must not recreate merged source", CommandRef: "cmd_merged_manual_credit",
	})
	assertRejected("manual credit", err)
	_, _, err = env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: source.ID, Direction: LoyaltyDirectionDebit, Points: 10,
		Reason: "must not spend merged source", CommandRef: "cmd_merged_manual_debit",
	})
	assertRejected("manual debit", err)
	_, err = env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
		CustomerProfileRef: source.ID, Points: 10, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_merged_trusted_credit", CommandRef: "cmd_merged_trusted_credit",
	})
	assertRejected("trusted credit", err)
	_, err = env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: source.ID, Points: 10, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_merged_trusted_debit", CommandRef: "cmd_merged_trusted_debit",
	})
	assertRejected("trusted debit", err)
	_, _, err = env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, original.ID,
		ReverseLoyaltyEntryInput{Reason: "must not reverse merged source", CommandRef: "cmd_merged_reverse"})
	assertRejected("reversal", err)

	// A source that had no account at merge time exercises the lazy creation
	// path directly. Two concurrent first credits must both stop at the profile
	// fence and must not materialize a new ACTIVE account.
	neverEnrolledSource := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_merged_never_enrolled"))
	env.seedProfile(t, businessA, "vs_a", customerSubject("cus_merged_never_enrolled"))
	if err = applyTargetedLoyaltyMerge(ctx, env, "gst_merged_never_enrolled", "cus_merged_never_enrolled", "evt_merged_never_enrolled"); err != nil {
		t.Fatal(err)
	}
	credit := func(ref string) func() error {
		return func() error {
			_, e := env.service.CreditPoints(ctx, actor, Scope{Global: true}, CreditPointsInput{
				CustomerProfileRef: neverEnrolledSource.ID, Points: 25, SourceType: LoyaltySourceOrder,
				SourceReference: ref, CommandRef: "cmd_" + ref,
			})
			return e
		}
	}
	a, b := race(credit("ord_merged_first_a"), credit("ord_merged_first_b"))
	assertRejected("concurrent first credit A", a)
	assertRejected("concurrent first credit B", b)
	var activeSourceAccounts int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id IN ($2,$3) AND status=$4`, businessA,
		source.ID, neverEnrolledSource.ID, LoyaltyAccountActive).Scan(&activeSourceAccounts); err != nil {
		t.Fatal(err)
	}
	if activeSourceAccounts != 0 {
		t.Fatalf("merged sources have %d ACTIVE account(s)", activeSourceAccounts)
	}
	var neverEnrolledAccounts int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_accounts
		WHERE business_id=$1 AND customer_profile_id=$2`, businessA, neverEnrolledSource.ID).Scan(&neverEnrolledAccounts); err != nil {
		t.Fatal(err)
	}
	if neverEnrolledAccounts != 0 {
		t.Fatalf("lazy mutation recreated %d account(s) for a merged never-enrolled source", neverEnrolledAccounts)
	}
	targetAccount := env.loyaltyAccount(t, businessA, target.ID)
	if targetAccount.CurrentPoints != 400 {
		t.Fatalf("canonical target balance=%d want=400", targetAccount.CurrentPoints)
	}
	assertLedgerMatchesLots(t, env, businessA, env.loyaltyAccount(t, businessA, source.ID))
	assertLedgerMatchesLots(t, env, businessA, targetAccount)
}
