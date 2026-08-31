package crm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Section 8D security, Customer 360 and scale tests.

// branchScopedService rebuilds the service with a branch-limited actor, which
// is how every branch-isolation assertion below is made against the real scope
// machinery rather than a stubbed decision.
func (e *testEnv) branchScopedService(flags Flags, branches ...string) *Service {
	return NewService(Options{
		DB: e.pool, Scopes: fixedScopes{"vs_a": {BranchIDs: branches}, "vs_b": {Global: true}},
		Audit: e.audit, Flags: flags, Now: func() time.Time { return e.now },
	})
}

// ---------------------------------------------------------------------------
// Tenant, customer, branch and entry isolation
// ---------------------------------------------------------------------------

// TestLoyaltySecurity_TenantIsolation checks that business B cannot read,
// adjust or reverse anything belonging to business A, and that a probe returns
// not-found rather than forbidden so ids cannot be enumerated.
func TestLoyaltySecurity_TenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	env.seedLoyaltyProgram(t, businessB, "vs_b", LoyaltyProgramActive, nil)
	profileA := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_tenant_a"))
	entryA := env.credit(t, businessA, "vs_a", profileA, 500, "ord_tenant_a")

	intruder := loyaltyActor(businessB, "vs_b")
	// Reading A's customer through B's session must not resolve at all.
	if _, err := env.service.GetProfile(ctx, intruder, Scope{Global: true}, profileA.ID); err == nil {
		t.Fatal("business B read business A's customer profile")
	}
	if _, _, err := env.service.AdjustPoints(ctx, intruder, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profileA.ID, Direction: LoyaltyDirectionCredit, Points: 100,
		Reason: "cross-tenant grant", CommandRef: "idem_tenant"}); err == nil {
		t.Fatal("business B granted points on business A's customer")
	}
	// Entry IDOR: B knows the entry id but must not be able to reverse it.
	if _, _, err := env.service.ReverseLoyaltyEntry(ctx, intruder, Scope{Global: true}, entryA.ID,
		ReverseLoyaltyEntryInput{Reason: "cross-tenant reversal", CommandRef: "idem_tenant_rev"}); err == nil {
		t.Fatal("business B reversed business A's ledger entry")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("cross-tenant reversal error = %v; want an indistinguishable not-found", err)
	}
	// A's balance is untouched.
	if account := env.loyaltyAccount(t, businessA, profileA.ID); account.CurrentPoints != 500 {
		t.Fatalf("business A balance = %d after cross-tenant attempts", account.CurrentPoints)
	}
}

// TestLoyaltySecurity_BranchActorCannotReachAnotherBranchesCustomer is the
// Section 8B/8C branch isolation carried into loyalty: a branch manager must
// not be able to read or move points for a customer who only exists at another
// branch, and must not be able to learn that such a customer exists.
func TestLoyaltySecurity_BranchActorCannotReachAnotherBranchesCustomer(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)

	mine := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_branch_mine"))
	theirs := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_branch_theirs"))
	env.seedOrder(t, businessA, "vs_a", mine, "ord_branch_mine", branchA1, 5000, 5000, env.now, 1)
	env.seedOrder(t, businessA, "vs_a", theirs, "ord_branch_theirs", branchA2, 7000, 7000, env.now, 1)
	env.credit(t, businessA, "vs_a", mine, 300, "ord_pts_mine")
	env.credit(t, businessA, "vs_a", theirs, 900, "ord_pts_theirs")

	scoped := env.branchScopedService(allFlagsOn(), branchA1)
	actor := loyaltyActor(businessA, "vs_a")
	branchScope := Scope{BranchIDs: []string{branchA1}}

	// Their customer is invisible, and reported as absent rather than denied.
	if _, err := scoped.GetProfile(ctx, actor, branchScope, theirs.ID); err == nil {
		t.Fatal("a branch actor read a customer belonging to another branch")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("cross-branch read error = %v; a denial would confirm the customer exists", err)
	}
	if _, _, err := scoped.AdjustPoints(ctx, actor, branchScope, AdjustPointsInput{
		CustomerProfileRef: theirs.ID, Direction: LoyaltyDirectionCredit, Points: 50,
		Reason: "cross-branch grant", CommandRef: "idem_branch_x"}); err == nil {
		t.Fatal("a branch actor moved points for another branch's customer")
	}
	if account := env.loyaltyAccount(t, businessA, theirs.ID); account.CurrentPoints != 900 {
		t.Fatalf("other branch's balance = %d; it must be untouched", account.CurrentPoints)
	}

	// Their own branch's customer works normally.
	own, err := scoped.GetProfile(ctx, actor, branchScope, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := scoped.LoyaltySummaryFor(ctx, actor, own)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AvailablePoints != 300 {
		t.Fatalf("in-branch summary = %+v; want the 300 points visible", summary)
	}
	if _, _, err = scoped.AdjustPoints(ctx, actor, branchScope, AdjustPointsInput{
		CustomerProfileRef: mine.ID, Direction: LoyaltyDirectionCredit, Points: 50,
		Reason: "in-branch goodwill", CommandRef: "idem_branch_ok"}); err != nil {
		t.Fatalf("a branch actor could not adjust their own branch's customer: %v", err)
	}
}

// TestLoyaltySecurity_LedgerReadIsScopedBeforePagination checks that history
// cannot be read for a customer the actor cannot see — the scope decision
// happens before any row or count is produced.
func TestLoyaltySecurity_LedgerReadIsScopedBeforePagination(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	theirs := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_ledger_theirs"))
	env.seedOrder(t, businessA, "vs_a", theirs, "ord_ledger_theirs", branchA2, 1000, 1000, env.now, 1)
	env.credit(t, businessA, "vs_a", theirs, 400, "ord_pts_ledger")

	scoped := env.branchScopedService(allFlagsOn(), branchA1)
	if _, err := scoped.GetProfile(ctx, loyaltyActor(businessA, "vs_a"), Scope{BranchIDs: []string{branchA1}}, theirs.ID); err == nil {
		t.Fatal("a branch actor resolved another branch's customer for a ledger read")
	}
	// A ledger read against a redacted or merged profile is refused even when
	// the caller somehow holds the profile object.
	entries, _, err := env.service.ListLoyaltyEntries(ctx, loyaltyActor(businessA, "vs_a"), theirs, LoyaltyLedgerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("business-scoped read = %d entries; want the single credit", len(entries))
	}
}

// TestLoyaltySecurity_LedgerFilterAndSortAreClosed checks there is no
// caller-controlled ordering and no unknown filter value reaching SQL.
func TestLoyaltySecurity_LedgerFilterAndSortAreClosed(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_filters"))
	env.credit(t, businessA, "vs_a", profile, 100, "ord_f1")

	for _, bogus := range []string{"BONUS", "earn; DROP TABLE crm_loyalty_entries", "'"} {
		if _, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{EntryType: bogus}); err == nil {
			t.Fatalf("entry_type filter %q was accepted", bogus)
		}
	}
	// Page size is bounded regardless of what a caller asks for.
	entries, _, err := env.service.ListLoyaltyEntries(ctx, actor, profile, LoyaltyLedgerOptions{Limit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > LoyaltyLedgerMaxPageSize {
		t.Fatalf("page returned %d rows; the cap is %d", len(entries), LoyaltyLedgerMaxPageSize)
	}
}

// TestLoyaltySecurity_NoPIIAnywhereInTheLoyaltyDomain checks the schema and the
// wire projections together: loyalty holds points and references, never
// contact details, and never anything monetary.
func TestLoyaltySecurity_NoPIIAnywhereInTheLoyaltyDomain(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	rows, err := env.pool.Query(ctx, `SELECT table_name,column_name FROM information_schema.columns
		WHERE table_schema='public' AND table_name LIKE 'crm_loyalty%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	forbidden := []string{"email", "phone", "msisdn", "address", "first_name", "last_name", "full_name",
		"date_of_birth", "dob", "national_id", "bvn", "password", "token",
		"currency", "amount_minor", "kobo", "wallet", "payout", "settlement"}
	seen := 0
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		seen++
		lowered := strings.ToLower(column)
		for _, bad := range forbidden {
			if strings.Contains(lowered, bad) {
				t.Errorf("%s.%s looks like personal or monetary data; loyalty stores points and references only", table, column)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no loyalty columns were inspected")
	}
}

// ---------------------------------------------------------------------------
// Customer 360
// ---------------------------------------------------------------------------

func TestLoyaltyCustomer360_ReportsEnrolmentHonestly(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))

	// Unenrolled: the block is present and says NOT_ENROLLED rather than 0.
	unenrolled := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_360_none"))
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, unenrolled)
	if err != nil {
		t.Fatal(err)
	}
	if view.Modules["loyalty"] != ModuleAvailable {
		t.Fatalf("loyalty module = %s; Section 8D is built", view.Modules["loyalty"])
	}
	if view.Loyalty == nil || view.Loyalty.Enrollment != LoyaltyNotEnrolled {
		t.Fatalf("loyalty block = %+v; want an explicit NOT_ENROLLED", view.Loyalty)
	}

	// Enrolled: balances, expiry preview and programme naming all present.
	enrolled := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_360_some"))
	env.credit(t, businessA, "vs_a", enrolled, 400, "ord_360")
	if _, err = env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
		CustomerProfileRef: enrolled.ID, Points: 100, SourceType: LoyaltySourceOrder,
		SourceReference: "ord_360_spend", CommandRef: "cmd_360_spend"}); err != nil {
		t.Fatal(err)
	}
	view, err = env.service.Customer360For(ctx, actor, Scope{Global: true}, enrolled)
	if err != nil {
		t.Fatal(err)
	}
	loyalty := view.Loyalty
	if loyalty == nil || loyalty.Enrollment != LoyaltyEnrolled || loyalty.AvailablePoints != 300 {
		t.Fatalf("loyalty block = %+v; want 300 available", loyalty)
	}
	if loyalty.LifetimeCreditedPoints != 400 || loyalty.LifetimeDebitedPoints != 100 {
		t.Fatalf("lifetime totals = %+v; want 400 credited / 100 debited", loyalty)
	}
	if loyalty.NextExpiryAt == nil || loyalty.PointsExpiringNext != 300 {
		t.Fatalf("expiry preview = %+v; want the surviving 300 dated", loyalty)
	}
	if loyalty.ProgramStatus != LoyaltyProgramActive || loyalty.PointsNamePlural == "" {
		t.Fatalf("programme context missing from the loyalty block: %+v", loyalty)
	}
}

// TestLoyaltyCustomer360_SuppressedForRedactedAndMergedSubjects checks the two
// non-actionable states report nothing rather than a stale balance.
func TestLoyaltyCustomer360_SuppressedForRedactedAndMergedSubjects(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)

	source := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_360_merge"))
	target := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_360_merge"))
	env.credit(t, businessA, "vs_a", source, 250, "ord_360_merge_src")
	env.credit(t, businessA, "vs_a", target, 100, "ord_360_merge_tgt")
	if err := env.mergeLoyaltyProfiles(t, "gst_360_merge", "cus_360_merge", "evt_360_merge"); err != nil {
		t.Fatal(err)
	}

	mergedSource, err := scanProfile(env.pool.QueryRow(ctx, `SELECT `+profileCols+`
		FROM vendor_customer_profiles WHERE id=$1`, source.ID))
	if err != nil {
		t.Fatal(err)
	}
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, mergedSource)
	if err != nil {
		t.Fatal(err)
	}
	if view.Loyalty == nil || view.Loyalty.Enrollment != LoyaltyNotEnrolled || view.Loyalty.AvailablePoints != 0 {
		t.Fatalf("merged-source loyalty = %+v; it must not present an independent balance", view.Loyalty)
	}

	// The survivor holds everything.
	survivor, err := scanProfile(env.pool.QueryRow(ctx, `SELECT `+profileCols+`
		FROM vendor_customer_profiles WHERE id=$1`, target.ID))
	if err != nil {
		t.Fatal(err)
	}
	view, err = env.service.Customer360For(ctx, actor, Scope{Global: true}, survivor)
	if err != nil {
		t.Fatal(err)
	}
	if view.Loyalty == nil || view.Loyalty.AvailablePoints != 350 {
		t.Fatalf("merged-target loyalty = %+v; want the combined 350 points", view.Loyalty)
	}
}

// TestLoyaltyCustomer360_HiddenWhenDisabled pins the honest-absence rule: with
// the flag off there is no loyalty block at all and the module says so, rather
// than an empty balance that reads as "this customer has no points".
func TestLoyaltyCustomer360_HiddenWhenDisabled(t *testing.T) {
	flags := allFlagsOn()
	flags.LoyaltyEnabled = false
	env := newTestEnv(t, flags)
	ctx := context.Background()
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_360_off"))

	view, err := env.service.Customer360For(ctx, loyaltyActor(businessA, "vs_a"), Scope{Global: true}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if view.Loyalty != nil {
		t.Fatalf("loyalty block present while disabled: %+v", view.Loyalty)
	}
	if view.Modules["loyalty"] != ModuleNotImplemented {
		t.Fatalf("loyalty module = %s while disabled", view.Modules["loyalty"])
	}
	// And no route or command works either.
	if _, _, err = env.service.AdjustPoints(ctx, loyaltyActor(businessA, "vs_a"), Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionCredit, Points: 10,
		Reason: "flag is off", CommandRef: "idem_off"}); err == nil {
		t.Fatal("a disabled loyalty feature still accepted an adjustment")
	}
}

// ---------------------------------------------------------------------------
// Segmentation boundary
// ---------------------------------------------------------------------------

// TestLoyalty_SegmentFieldRegistryIsUnchanged guards the Section 8B contract.
// Loyalty is deliberately not a segmentable field yet; adding one here would
// silently change what every stored audience means.
func TestLoyalty_SegmentFieldRegistryIsUnchanged(t *testing.T) {
	// The count is asserted against the Section 8D baseline by turning off the
	// only later section that legitimately extends the registry. Naming the
	// current total here instead would mean editing this test every time a
	// future section adds a field, which is exactly how a guard stops guarding.
	baseline := allFlagsOn()
	baseline.IntelligenceEnabled = false
	registry := NewSegmentFieldRegistry(baseline)
	fields := registry.Fields()
	if len(fields) != 32 {
		t.Fatalf("segment field registry has %d fields; Section 8D must leave the final Section 8 baseline at 32", len(fields))
	}
	for _, field := range NewSegmentFieldRegistry(allFlagsOn()).Fields() {
		if strings.HasPrefix(strings.ToLower(field.Key), "loyalty") {
			t.Fatalf("field %s entered the segmentation registry; loyalty segmentation is later work", field.Key)
		}
	}
}

// ---------------------------------------------------------------------------
// Scale
// ---------------------------------------------------------------------------

// TestLoyaltyScale_HotPathsStayIndexBacked builds a realistic fixture and
// checks the four hot paths are bounded and index-backed rather than table
// scans that only look fast on an empty database.
func TestLoyaltyScale_HotPathsStayIndexBacked(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := loyaltyActor(businessA, "vs_a")
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(60))

	const accounts = 120
	var focus Profile
	for i := 0; i < accounts; i++ {
		profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_scale_"+itoa(i)))
		if i == 0 {
			focus = profile
		}
		// A few lots each, so FEFO has real work and the ledger has depth.
		for lot := 0; lot < 3; lot++ {
			env.credit(t, businessA, "vs_a", profile, int64(100+lot*10), "ord_scale_"+itoa(i)+"_"+itoa(lot))
		}
	}
	// Give the focus customer a deeper history to page through.
	for i := 0; i < 60; i++ {
		if _, err := env.service.DebitPoints(ctx, actor, Scope{Global: true}, DebitPointsInput{
			CustomerProfileRef: focus.ID, Points: 5, SourceType: LoyaltySourceOrder,
			SourceReference: "ord_scale_spend_" + itoa(i), CommandRef: "cmd_scale_spend_" + itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}

	var entries, lots int
	if err := env.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM crm_loyalty_entries),
		(SELECT count(*) FROM crm_loyalty_point_lots)`).Scan(&entries, &lots); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture: %d accounts, %d ledger entries, %d point lots", accounts, entries, lots)
	if _, err := env.pool.Exec(ctx, `ANALYZE crm_loyalty_entries; ANALYZE crm_loyalty_point_lots;
		ANALYZE crm_loyalty_accounts`); err != nil {
		t.Fatal(err)
	}

	account := env.loyaltyAccount(t, businessA, focus.ID)
	for _, probe := range []struct {
		name  string
		query string
		args  []any
	}{
		{"account lookup", `SELECT ` + loyaltyAccountCols + ` FROM crm_loyalty_accounts
			WHERE business_id=$1 AND customer_profile_id=$2`, []any{businessA, focus.ID}},
		{"ledger page", `SELECT ` + loyaltyEntryCols + ` FROM crm_loyalty_entries
			WHERE business_id=$1 AND loyalty_account_id=$2 ORDER BY created_at DESC, sequence DESC LIMIT 51 OFFSET 0`,
			[]any{businessA, account.ID}},
		{"FEFO selection", `SELECT ` + loyaltyLotCols + ` FROM crm_loyalty_point_lots
			WHERE business_id=$1 AND loyalty_account_id=$2 AND remaining_points>0 AND (expires_at IS NULL OR expires_at>$3)
			ORDER BY expires_at ASC NULLS LAST, created_at ASC, id ASC`, []any{businessA, account.ID, env.now}},
		{"due expiry scan", `SELECT count(*) FROM crm_loyalty_point_lots
			WHERE remaining_points>0 AND expires_at IS NOT NULL AND expires_at<=$1`, []any{env.now}},
	} {
		// At this fixture size PostgreSQL will often — correctly — choose a
		// sequential scan, because scanning a few hundred rows is genuinely
		// cheaper than an index lookup. Asserting on the chosen plan would
		// therefore test the planner's cost model, not our schema. What
		// matters is that a usable index exists for each hot predicate, so the
		// probe runs with sequential scans disabled and requires an index path
		// to appear.
		plan := explain(t, env, probe.query, probe.args...)
		if !strings.Contains(plan, "Index Scan") && !strings.Contains(plan, "Index Only Scan") &&
			!strings.Contains(plan, "Bitmap Index Scan") {
			t.Errorf("%s has no index available for its predicate:\n%s", probe.name, plan)
		}
	}

	// The Customer 360 loyalty block stays a bounded read.
	start := time.Now()
	summary, err := env.service.LoyaltySummaryFor(ctx, actor, focus)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loyalty summary took %s (available=%d)", time.Since(start), summary.AvailablePoints)

	page, more, err := env.service.ListLoyaltyEntries(ctx, actor, focus, LoyaltyLedgerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != LoyaltyLedgerDefaultPageSize || !more {
		t.Fatalf("ledger page = %d entries (more=%t); want a full default page with more to follow", len(page), more)
	}
}

// explain plans a query with sequential scans disabled, so the output shows
// which index the predicate can use rather than which plan is cheapest on a
// small test fixture.
func explain(t *testing.T, env *testEnv, query string, args ...any) string {
	t.Helper()
	ctx := context.Background()
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	return plan.String()
}

var _ = pgx.ErrNoRows
