package crm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Section 8D administrative tests: expiry sweeping, manual adjustment and
// reversal.

// recordingEvents captures published domain events so a test can inspect the
// exact payload that would reach a downstream consumer.
type recordingEvents struct {
	types    []string
	payloads []any
}

func (r *recordingEvents) Publish(_ context.Context, _ pgx.Tx, eventType, _, _ string,
	_ int64, _, _, _ string, payload any) error {
	r.types = append(r.types, eventType)
	r.payloads = append(r.payloads, payload)
	return nil
}

func (r *recordingEvents) dump(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"types": r.types, "payloads": r.payloads})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// ---------------------------------------------------------------------------
// Expiry
// ---------------------------------------------------------------------------

// TestLoyaltyExpiry_SweeperSettlesDueLotsOnceAndOnlyOnce covers the sweeper's
// core claims together: due lots expire, undue lots do not, only unconsumed
// points expire, and running the pass repeatedly cannot double-expire.
func TestLoyaltyExpiry_SweeperSettlesDueLotsOnceAndOnlyOnce(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(10))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_sweep"))

	env.credit(t, businessA, "vs_a", profile, 100, "ord_soon")
	// A second lot with a much longer window, issued after a policy change.
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryDays: intPtr(400), ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	env.credit(t, businessA, "vs_a", profile, 200, "ord_later")

	// Spend 40 of the soon-to-expire lot: only the remaining 60 may expire.
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 40, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_partial", CommandRef: "cmd_partial"}); err != nil {
		t.Fatal(err)
	}

	env.now = env.now.AddDate(0, 0, 11)
	swept, err := env.service.RunLoyaltyExpiryPass(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if swept != 1 {
		t.Fatalf("swept %d accounts; want 1", swept)
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 200 {
		t.Fatalf("balance = %d; want 200 (60 expired, 200 not yet due)", account.CurrentPoints)
	}
	entries, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{EntryType: LoyaltyExpire})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].DeltaPoints != -60 {
		t.Fatalf("expire entries = %+v; want a single -60 entry (only unconsumed points expire)", entries)
	}

	// Repeated passes find nothing more to do.
	for i := 0; i < 3; i++ {
		if _, err = env.service.RunLoyaltyExpiryPass(ctx, 10); err != nil {
			t.Fatal(err)
		}
	}
	entries, _, err = env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{EntryType: LoyaltyExpire})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expire entries after four passes = %d; expiry must not repeat", len(entries))
	}
	account = env.loyaltyAccount(t, businessA, profile.ID)
	if account.CurrentPoints != 200 {
		t.Fatalf("balance drifted to %d across repeat passes", account.CurrentPoints)
	}
}

// TestLoyaltyExpiry_OutageIsCaughtUpWithoutLoss models the sweeper being down
// for days across several businesses and customers: when it comes back nothing
// is lost, nothing double-expires, and no balance goes negative.
func TestLoyaltyExpiry_OutageIsCaughtUpWithoutLoss(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(5))
	env.seedLoyaltyProgram(t, businessB, "vs_b", LoyaltyProgramActive, intPtr(5))

	type held struct {
		profile Profile
		points  int64
	}
	var subjects []held
	for i, points := range []int64{100, 250, 75} {
		profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_outage_a"+itoa(i)))
		env.credit(t, businessA, "vs_a", profile, points, "ord_outage_a"+itoa(i))
		subjects = append(subjects, held{profile, points})
	}
	profileB := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_outage_b"))
	env.credit(t, businessB, "vs_b", profileB, 500, "ord_outage_b")

	// Three days of downtime past the expiry window.
	env.now = env.now.AddDate(0, 0, 8)

	total := 0
	for {
		swept, err := env.service.RunLoyaltyExpiryPass(ctx, 2) // deliberately smaller than the backlog
		if err != nil {
			t.Fatal(err)
		}
		total += swept
		if swept == 0 {
			break
		}
	}
	if total != 4 {
		t.Fatalf("caught up %d accounts; want all 4", total)
	}
	for _, subject := range subjects {
		account := env.loyaltyAccount(t, businessA, subject.profile.ID)
		if account.CurrentPoints != 0 {
			t.Fatalf("account %s = %d points after catch-up; want 0", account.ID, account.CurrentPoints)
		}
		if account.LifetimeDebitedPoints != subject.points {
			t.Fatalf("lifetime debited = %d; want the %d that expired", account.LifetimeDebitedPoints, subject.points)
		}
	}
	if account := env.loyaltyAccount(t, businessB, profileB.ID); account.CurrentPoints != 0 {
		t.Fatalf("business B account = %d; the sweeper must cross tenants", account.CurrentPoints)
	}
	due, err := env.service.DueExpiryLotCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if due != 0 {
		t.Fatalf("%d lots still due after catch-up", due)
	}
}

// TestLoyaltyExpiry_DisabledFlagDefersButDoesNotDiscard pins the operational
// contract: turning loyalty off stops the sweeper without losing the work, and
// the points stay unspendable in the meantime.
func TestLoyaltyExpiry_DisabledFlagDefersButDoesNotDiscard(t *testing.T) {
	flags := allFlagsOn()
	env := newTestEnv(t, flags)
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(5))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_flag_off"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_flag")
	env.now = env.now.AddDate(0, 0, 6)

	// Rebuild the service with loyalty disabled, sharing the same database.
	off := flags
	off.LoyaltyEnabled = false
	disabled := NewService(Options{DB: env.pool, Scopes: fixedScopes{"vs_a": {Global: true}}, Audit: env.audit,
		Flags: off, Now: func() time.Time { return env.now }})
	swept, err := disabled.RunLoyaltyExpiryPass(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if swept != 0 {
		t.Fatalf("a disabled sweeper swept %d accounts", swept)
	}
	due, err := env.service.DueExpiryLotCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if due != 1 {
		t.Fatalf("due lots = %d; disabling must defer work, not discard it", due)
	}
	// Re-enabled, the backlog is caught up.
	if swept, err = env.service.RunLoyaltyExpiryPass(ctx, 10); err != nil || swept != 1 {
		t.Fatalf("catch-up swept %d (err=%v); want 1", swept, err)
	}
}

// ---------------------------------------------------------------------------
// Manual adjustment
// ---------------------------------------------------------------------------

func TestLoyaltyAdjustment_CreditAndDebitWithProvenance(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_adjust"))

	// A manual credit enrols the customer and issues a lot under the current
	// expiry policy.
	entry, summary, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 500,
		Reason: "compensation for a cancelled order", CommandRef: "idem_adj_credit"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.EntryType != LoyaltyAdjustmentCredit || entry.DeltaPoints != 500 || summary.CurrentPoints != 500 {
		t.Fatalf("credit = %+v / account %+v; want ADJUSTMENT_CREDIT of 500", entry, summary)
	}
	lots := env.lots(t, businessA, summary.ID)
	if len(lots) != 1 || lots[0].ExpiresAt == nil || !lots[0].ExpiresAt.Equal(env.now.AddDate(0, 0, 30)) {
		t.Fatalf("manual credit lot = %+v; want one lot on the current expiry policy", lots)
	}

	// A manual debit consumes that lot and records the allocation.
	entry, summary, err = env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionDebit, Points: 200,
		Reason: "reversing a duplicate goodwill grant", CommandRef: "idem_adj_debit"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.EntryType != LoyaltyAdjustmentDebit || entry.DeltaPoints != -200 || summary.CurrentPoints != 300 {
		t.Fatalf("debit = %+v / account %+v; want ADJUSTMENT_DEBIT of 200", entry, summary)
	}
	var consumed int64
	if err = env.pool.QueryRow(ctx, `SELECT consumed_points FROM crm_loyalty_lot_consumptions
		WHERE business_id=$1 AND debit_entry_id=$2`, businessA, entry.ID).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if consumed != 200 {
		t.Fatalf("allocation = %d; want the full 200 recorded against the funding lot", consumed)
	}
}

func TestLoyaltyAdjustment_ValidationAndLimit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	limit := int64(1000)
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ManualAdjustmentLimit: &limit, ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_adj_validation"))

	cases := []struct {
		name string
		in   AdjustPointsInput
	}{
		{"no reason", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: 10, CommandRef: "i1"}},
		{"blank reason", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: 10, Reason: "   ", CommandRef: "i2"}},
		{"reason too long", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: 10,
			Reason: strings.Repeat("x", LoyaltyAdjustmentReasonMax+1), CommandRef: "i3"}},
		{"zero points", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: 0, Reason: "nothing", CommandRef: "i4"}},
		{"negative points", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: -50, Reason: "sneaky", CommandRef: "i5"}},
		{"unknown direction", AdjustPointsInput{Direction: "TRANSFER", Points: 10, Reason: "wrong", CommandRef: "i6"}},
		{"above the limit", AdjustPointsInput{Direction: LoyaltyDirectionCredit, Points: 1001, Reason: "too big", CommandRef: "i7"}},
	}
	for _, tc := range cases {
		tc.in.CustomerProfileRef = profile.ID
		if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, tc.in); err == nil {
			t.Errorf("%s: the adjustment was accepted", tc.name)
		}
	}
	// The boundary itself is allowed.
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 1000,
		Reason: "exactly at the configured limit", CommandRef: "i_boundary"}); err != nil {
		t.Fatalf("an adjustment at the limit was refused: %v", err)
	}
}

// TestLoyaltyAdjustment_DebitDoesNotEnrolOrOverdraw pins two related choices:
// a debit against a customer who never held points fails without leaving an
// empty account behind, and a debit larger than the balance is refused.
func TestLoyaltyAdjustment_DebitDoesNotEnrolOrOverdraw(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_adj_debit"))

	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionDebit, Points: 10,
		Reason: "no account exists", CommandRef: "idem_no_account"}); err == nil {
		t.Fatal("a manual debit against an unenrolled customer succeeded")
	}
	var accounts int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_accounts WHERE business_id=$1`, businessA).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 {
		t.Fatalf("a failed debit created %d accounts", accounts)
	}

	env.credit(t, businessA, "vs_a", profile, 100, "ord_adj_debit")
	if _, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionDebit, Points: 101,
		Reason: "overdraw attempt", CommandRef: "idem_overdraw"}); err == nil {
		t.Fatal("a manual debit overdrew the balance")
	}
	if account := env.loyaltyAccount(t, businessA, profile.ID); account.CurrentPoints != 100 {
		t.Fatalf("balance = %d after a refused debit", account.CurrentPoints)
	}
}

// TestLoyaltyAdjustment_ReasonNeverLeavesTheAuditLog is the disclosure test:
// an operator's free-text justification is recorded for audit and appears in
// no ledger row and no outbox payload.
func TestLoyaltyAdjustment_ReasonNeverLeavesTheAuditLog(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_reason"))

	// The shared harness has no publisher; wire a recording one so the event
	// payload itself can be inspected rather than merely assumed clean.
	events := &recordingEvents{}
	env.service = NewService(Options{DB: env.pool, Scopes: fixedScopes{"vs_a": {Global: true}}, Audit: env.audit,
		Events: events, Flags: allFlagsOn(), Now: func() time.Time { return env.now }})

	const secret = "customer threatened to complain to the regulator"
	entry, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 100,
		Reason: secret, CommandRef: "idem_reason"})
	if err != nil {
		t.Fatal(err)
	}

	// The ledger row carries a coarse code, never the operator's words.
	if entry.ReasonCode == nil || *entry.ReasonCode != "MANUAL_CREDIT" {
		t.Fatalf("ledger reason_code = %v; want the coarse MANUAL_CREDIT", entry.ReasonCode)
	}
	var ledgerBlob string
	if err = env.pool.QueryRow(ctx, `SELECT coalesce(string_agg(concat_ws(' ',entry_type,source_type,source_reference,
		idempotency_reference,reason_code),' '),'') FROM crm_loyalty_entries WHERE business_id=$1`, businessA).Scan(&ledgerBlob); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ledgerBlob, "regulator") {
		t.Fatal("the adjustment reason leaked into the points ledger")
	}
	// The published event is a downstream disclosure surface and must stay
	// clean of both the operator's words and the command key.
	published := events.dump(t)
	if !strings.Contains(published, "vendor.crm.loyalty.adjustment.applied") {
		t.Fatalf("no adjustment event was published: %s", published)
	}
	for _, forbidden := range []string{"regulator", "idem_reason", "@", "cus_reason"} {
		if strings.Contains(published, forbidden) {
			t.Fatalf("event payload leaked %q: %s", forbidden, published)
		}
	}
	// It is recorded where it belongs.
	var audited int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_audit_log
		WHERE vendor_id=$1 AND action=$2 AND reason=$3`, businessA, "crm.loyalty.adjustment.applied", secret).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("audit rows carrying the reason = %d; want exactly 1", audited)
	}
}

func TestLoyaltyAdjustment_IdempotentReplay(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_adj_idem"))

	first, _, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 100,
		Reason: "goodwill", CommandRef: "idem_replay"})
	if err != nil {
		t.Fatal(err)
	}
	replay, account, err := env.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 100,
		Reason: "goodwill", CommandRef: "idem_replay"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID || account.CurrentPoints != 100 {
		t.Fatalf("replay posted %s with balance %d; want the original %s and 100", replay.ID, account.CurrentPoints, first.ID)
	}
}

// ---------------------------------------------------------------------------
// Reversal
// ---------------------------------------------------------------------------

func TestLoyaltyReversal_CreditIsVoidedExactlyOnce(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_credit"))
	credited := env.credit(t, businessA, "vs_a", profile, 300, "ord_rev")

	reversal, account, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "credited against the wrong order", CommandRef: "idem_rev_c"})
	if err != nil {
		t.Fatal(err)
	}
	if reversal.DeltaPoints != -300 || account.CurrentPoints != 0 {
		t.Fatalf("reversal = %+v, balance %d; want -300 and a zero balance", reversal, account.CurrentPoints)
	}
	if reversal.ReversesEntryID == nil || *reversal.ReversesEntryID != credited.ID {
		t.Fatalf("reversal does not reference its original: %+v", reversal)
	}
	lots := env.lots(t, businessA, account.ID)
	if len(lots) != 1 || lots[0].RemainingPoints != 0 {
		t.Fatalf("lots = %+v; the reversed issuance must be drained", lots)
	}

	// A second attempt under a different command key is a conflict, not a
	// second debit.
	if _, _, err = env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "trying again", CommandRef: "idem_rev_c2"}); err == nil {
		t.Fatal("the same entry was reversed twice")
	} else if !strings.Contains(err.Error(), "already been reversed") {
		t.Fatalf("second reversal error = %v; want an already-reversed conflict", err)
	}
	// The same command key replays instead.
	replay, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "credited against the wrong order", CommandRef: "idem_rev_c"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != reversal.ID {
		t.Fatalf("replay = %s; want the original reversal %s", replay.ID, reversal.ID)
	}
	if account = env.loyaltyAccount(t, businessA, profile.ID); account.CurrentPoints != 0 {
		t.Fatalf("balance = %d after a replayed reversal", account.CurrentPoints)
	}
}

// TestLoyaltyReversal_ConsumedCreditIsRefused pins the refusal: taking the
// points out of some other lot would silently confiscate points that came from
// a different issuance, possibly with a different expiry.
func TestLoyaltyReversal_ConsumedCreditIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_consumed"))
	credited := env.credit(t, businessA, "vs_a", profile, 300, "ord_rev_consumed")
	env.credit(t, businessA, "vs_a", profile, 500, "ord_rev_other")

	// Spend one point out of the first lot.
	if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 1, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_touch", CommandRef: "cmd_touch"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "too late", CommandRef: "idem_rev_consumed"})
	if err == nil {
		t.Fatal("a partly-spent credit was reversed; the points no longer existed to take back")
	}
	if !strings.Contains(err.Error(), "already been used") {
		t.Fatalf("error = %v; want the consumed-points refusal", err)
	}
	if account := env.loyaltyAccount(t, businessA, profile.ID); account.CurrentPoints != 799 {
		t.Fatalf("balance = %d; a refused reversal must change nothing", account.CurrentPoints)
	}
}

// TestLoyaltyReversal_DebitRestoresTheOriginalLotsAndExpiry is why lot
// consumption is recorded: the reversal gives back the exact points, to the
// exact lots, with their original expiry.
func TestLoyaltyReversal_DebitRestoresTheOriginalLotsAndExpiry(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	program := env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_debit"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_lot_a")
	if _, err := env.service.UpdateLoyaltyProgram(ctx, actor, UpdateLoyaltyProgramInput{
		ExpiryDays: intPtr(90), ExpectedVersion: program.Version}); err != nil {
		t.Fatal(err)
	}
	env.credit(t, businessA, "vs_a", profile, 100, "ord_lot_b")

	// Spend 150: all of lot A (30-day) and 50 of lot B (90-day).
	debit, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 150, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_spend_mixed", CommandRef: "cmd_spend_mixed"})
	if err != nil {
		t.Fatal(err)
	}
	account := env.loyaltyAccount(t, businessA, profile.ID)
	before := env.lots(t, businessA, account.ID)

	reversal, account, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, debit.ID,
		ReverseLoyaltyEntryInput{Reason: "the order was never fulfilled", CommandRef: "idem_rev_d"})
	if err != nil {
		t.Fatal(err)
	}
	if reversal.DeltaPoints != 150 || account.CurrentPoints != 200 {
		t.Fatalf("reversal = %+v, balance %d; want +150 back to 200", reversal, account.CurrentPoints)
	}
	after := env.lots(t, businessA, account.ID)
	if len(after) != len(before) {
		t.Fatalf("reversal minted %d new lots; restored points must return to their own lot", len(after)-len(before))
	}
	for i := range after {
		if after[i].ID != before[i].ID {
			t.Fatalf("lot ordering changed: %+v vs %+v", before, after)
		}
		if (after[i].ExpiresAt == nil) != (before[i].ExpiresAt == nil) ||
			(after[i].ExpiresAt != nil && !after[i].ExpiresAt.Equal(*before[i].ExpiresAt)) {
			t.Fatalf("lot %s expiry changed from %v to %v; a reversal must not extend a window",
				after[i].ID, before[i].ExpiresAt, after[i].ExpiresAt)
		}
	}
	if after[0].RemainingPoints != 100 || after[1].RemainingPoints != 100 {
		t.Fatalf("restored lots = %+v; want both back to their issued amounts", after)
	}
}

// TestLoyaltyReversal_ExpiredRestorationIsNotSpendable is the subtle one: the
// debit is reversed after the lot's expiry has passed, so the points come back
// to their lot and are immediately expired in the same transaction rather than
// becoming a fresh spendable balance.
func TestLoyaltyReversal_ExpiredRestorationIsNotSpendable(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(10))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_expired"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_exp_rev")

	debit, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: profile.ID, Points: 100, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_spend_exp", CommandRef: "cmd_spend_exp"})
	if err != nil {
		t.Fatal(err)
	}
	// The lot's window lapses while the points are spent.
	env.now = env.now.AddDate(0, 0, 11)

	reversal, account, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, debit.ID,
		ReverseLoyaltyEntryInput{Reason: "order cancelled after the window", CommandRef: "idem_rev_exp"})
	if err != nil {
		t.Fatal(err)
	}
	if reversal.DeltaPoints != 100 {
		t.Fatalf("reversal = %+v; the points must be restored honestly before expiring", reversal)
	}
	if account.CurrentPoints != 0 {
		t.Fatalf("balance = %d; restored points past their expiry must not become spendable", account.CurrentPoints)
	}
	entries, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{EntryType: LoyaltyExpire})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].DeltaPoints != -100 {
		t.Fatalf("expire entries = %+v; want the restored points expired in the same transaction", entries)
	}
	summary, err := env.service.LoyaltySummaryFor(ctx, actor, profile)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AvailablePoints != 0 {
		t.Fatalf("available = %d; a reversal must not resurrect an expired lot", summary.AvailablePoints)
	}
}

// TestLoyaltyReversal_ForbiddenTypesAreRefused keeps the ledger readable:
// expiry is the programme's own rule, merges belong to identity authority, and
// a reversal chain would make the net effect of a history unreadable.
func TestLoyaltyReversal_ForbiddenTypesAreRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(5))
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_forbidden"))
	credited := env.credit(t, businessA, "vs_a", profile, 100, "ord_forbidden")

	reversal, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
		ReverseLoyaltyEntryInput{Reason: "wrong customer", CommandRef: "idem_rev_f1"})
	if err != nil {
		t.Fatal(err)
	}
	// A reversal cannot itself be reversed.
	if _, _, err = env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, reversal.ID,
		ReverseLoyaltyEntryInput{Reason: "undo the undo", CommandRef: "idem_rev_f2"}); err == nil {
		t.Fatal("a reversal was reversed; chains must not be possible")
	} else if !strings.Contains(err.Error(), "cannot be reversed") {
		t.Fatalf("error = %v; want a not-reversible refusal", err)
	}

	// Nor can a system expiry.
	env.credit(t, businessA, "vs_a", profile, 50, "ord_forbidden_exp")
	env.now = env.now.AddDate(0, 0, 6)
	if _, err = env.service.RunLoyaltyExpiryPass(ctx, 5); err != nil {
		t.Fatal(err)
	}
	expiries, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{EntryType: LoyaltyExpire})
	if err != nil {
		t.Fatal(err)
	}
	if len(expiries) == 0 {
		t.Fatal("no expiry entry was produced to test against")
	}
	if _, _, err = env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, expiries[0].ID,
		ReverseLoyaltyEntryInput{Reason: "give them back", CommandRef: "idem_rev_f3"}); err == nil {
		t.Fatal("a system expiry was reversed")
	}
}

func TestLoyaltyReversal_RequiresAReason(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_rev_reason"))
	credited := env.credit(t, businessA, "vs_a", profile, 100, "ord_rev_reason")

	for _, reason := range []string{"", "   ", strings.Repeat("y", LoyaltyAdjustmentReasonMax+1)} {
		if _, _, err := env.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, credited.ID,
			ReverseLoyaltyEntryInput{Reason: reason, CommandRef: "idem_rr"}); err == nil {
			t.Fatalf("a reversal with reason %q was accepted", reason)
		}
	}
}
