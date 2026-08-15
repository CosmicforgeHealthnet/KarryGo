package crm

import (
	"context"
	"strings"
	"testing"
)

// Section 8 completion tests for loyalty tiers.
//
// These run against real PostgreSQL because the properties being claimed —
// that two rungs cannot share a threshold, that a concurrent editor cannot
// silently overwrite a ladder, that standing follows the ledger without a
// recalculation job — are enforced by constraints, an advisory lock and a
// read-time join. None of the three survives being mocked.

func tierEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnv(t, Flags{
		CRMEnabled: true, WritesEnabled: true,
		SegmentationEnabled: true, LoyaltyEnabled: true,
	})
}

func ladder(pairs ...any) []LoyaltyTierInput {
	out := make([]LoyaltyTierInput, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, LoyaltyTierInput{
			TierKey:               pairs[i].(string),
			DisplayName:           strings.Title(strings.ToLower(pairs[i].(string))), //nolint:staticcheck // ASCII keys only
			MinimumLifetimePoints: int64(pairs[i+1].(int)),
		})
	}
	return out
}

// TestLoyaltyTiers_UnconfiguredBusinessHasNoLadderRatherThanADefaultOne pins
// the distinction the whole feature rests on. A vendor who has never defined
// tiers has NOT_AVAILABLE standing, not the bottom rung of a ladder the
// platform invented for them.
func TestLoyaltyTiers_UnconfiguredBusinessHasNoLadderRatherThanADefaultOne(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()

	set, err := e.service.GetLoyaltyTiers(ctx, businessA)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Tiers) != 0 || set.Version != 0 {
		t.Fatalf("unconfigured business reported %d tiers at version %d; want an empty set at 0",
			len(set.Tiers), set.Version)
	}

	profile := e.seedProfile(t, businessA, "vs_a", SubjectRef{Type: SubjectCustomer, Ref: "cus_tier_none"})
	tier, err := e.service.ResolveCustomerLoyaltyTier(ctx, businessA, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tier.State != LoyaltyTierNotAvailable {
		t.Fatalf("state = %q; want NOT_AVAILABLE. A business with no ladder must not "+
			"report a tier, because presenting one would invent a status the vendor never defined",
			tier.State)
	}
	if tier.TierKey != "" {
		t.Fatalf("unavailable standing carried tier key %q", tier.TierKey)
	}
}

// TestLoyaltyTiers_RankIsDerivedFromThresholdsNotSupplied proves the ladder has
// exactly one answer to "which tier is higher": ranks come from the thresholds,
// so a caller cannot submit an ordering that disagrees with its own numbers.
func TestLoyaltyTiers_RankIsDerivedFromThresholdsNotSupplied(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// Deliberately submitted highest-first.
	set, err := e.service.ReplaceLoyaltyTiers(ctx, actor,
		ladder("PLATINUM", 5000, "BRONZE", 0, "GOLD", 1000), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Tiers) != 3 {
		t.Fatalf("got %d tiers; want 3", len(set.Tiers))
	}
	wantOrder := []struct {
		key  string
		rank int
	}{{"BRONZE", 1}, {"GOLD", 2}, {"PLATINUM", 3}}
	for i, want := range wantOrder {
		if set.Tiers[i].TierKey != want.key || set.Tiers[i].Rank != want.rank {
			t.Fatalf("tier %d = %s rank %d; want %s rank %d",
				i, set.Tiers[i].TierKey, set.Tiers[i].Rank, want.key, want.rank)
		}
	}
}

// TestLoyaltyTiers_AmbiguousOrOversizedLaddersAreRefused covers the set-level
// rules that no per-row constraint can express.
func TestLoyaltyTiers_AmbiguousOrOversizedLaddersAreRefused(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	cases := []struct {
		name  string
		input []LoyaltyTierInput
		why   string
	}{
		{"duplicate threshold", ladder("BRONZE", 0, "SILVER", 0),
			"two rungs at one entry point make membership ambiguous"},
		{"duplicate key", []LoyaltyTierInput{
			{TierKey: "GOLD", DisplayName: "Gold", MinimumLifetimePoints: 100},
			{TierKey: "gold", DisplayName: "Gold Again", MinimumLifetimePoints: 200},
		}, "keys are case-normalised, so these are the same tier"},
		{"too many tiers", ladder("A_1", 1, "B_1", 2, "C_1", 3, "D_1", 4,
			"E_1", 5, "F_1", 6, "G_1", 7, "H_1", 8, "I_1", 9),
			"nine exceeds the ladder ceiling of eight"},
		{"negative threshold", []LoyaltyTierInput{
			{TierKey: "BRONZE", DisplayName: "Bronze", MinimumLifetimePoints: -1},
		}, "a negative lifetime is not reachable"},
		{"bad key format", []LoyaltyTierInput{
			{TierKey: "1BAD", DisplayName: "Bad", MinimumLifetimePoints: 0},
		}, "keys must start with a letter"},
		{"empty display name", []LoyaltyTierInput{
			{TierKey: "BRONZE", DisplayName: "   ", MinimumLifetimePoints: 0},
		}, "a rung with no name cannot be presented"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.service.ReplaceLoyaltyTiers(ctx, actor, tc.input, nil); err == nil {
				t.Fatalf("ladder was accepted but should not have been: %s", tc.why)
			}
		})
	}

	// None of the refusals left a partial ladder behind.
	set, err := e.service.GetLoyaltyTiers(ctx, businessA)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Tiers) != 0 {
		t.Fatalf("a rejected write left %d tiers behind; every refusal must be total", len(set.Tiers))
	}
}

// TestLoyaltyTiers_ConcurrentEditorsCannotSilentlyOverwrite pins the optimistic
// concurrency contract on the one new mutation this completion adds.
func TestLoyaltyTiers_ConcurrentEditorsCannotSilentlyOverwrite(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	first, err := e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("BRONZE", 0, "GOLD", 1000), nil)
	if err != nil {
		t.Fatal(err)
	}
	stale := first.Version

	if _, err = e.service.ReplaceLoyaltyTiers(ctx, actor,
		ladder("BRONZE", 0, "GOLD", 2000), &stale); err != nil {
		t.Fatal(err)
	}

	// A second editor still holding the original version is refused rather than
	// having their view of the ladder silently win.
	_, err = e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("BRONZE", 0, "GOLD", 3000), &stale)
	if err == nil {
		t.Fatal("a write against a stale version succeeded; concurrent editors would overwrite each other")
	}

	set, err := e.service.GetLoyaltyTiers(ctx, businessA)
	if err != nil {
		t.Fatal(err)
	}
	if set.Tiers[1].MinimumLifetimePoints != 2000 {
		t.Fatalf("GOLD threshold = %d; want the second writer's 2000 preserved",
			set.Tiers[1].MinimumLifetimePoints)
	}
}

// TestLoyaltyTiers_ReplacementArchivesRatherThanDeletes keeps history pointable.
func TestLoyaltyTiers_ReplacementArchivesRatherThanDeletes(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	if _, err := e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("BRONZE", 0, "GOLD", 1000), nil); err != nil {
		t.Fatal(err)
	}
	set, err := e.service.GetLoyaltyTiers(ctx, businessA)
	if err != nil {
		t.Fatal(err)
	}
	v := set.Version
	if _, err = e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("STARTER", 0), &v); err != nil {
		t.Fatal(err)
	}

	var archived int
	if err = e.pool.QueryRow(ctx, `SELECT count(*) FROM crm_loyalty_tier_definitions
		WHERE business_id=$1 AND status='ARCHIVED'`, businessA).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 2 {
		t.Fatalf("archived rows = %d; want the 2 superseded rungs retained so audit "+
			"records that referenced them still point at something", archived)
	}
}

// TestLoyaltyTiers_StandingFollowsTheLedgerWithoutRecalculation is the central
// claim: because standing is derived at read time, a credit, a reversal and a
// threshold change are each reflected immediately with no job in between.
func TestLoyaltyTiers_StandingFollowsTheLedgerWithoutRecalculation(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	e.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)

	if _, err := e.service.ReplaceLoyaltyTiers(ctx, actor,
		ladder("BRONZE", 0, "GOLD", 1000, "PLATINUM", 5000), nil); err != nil {
		t.Fatal(err)
	}
	profile := e.seedProfile(t, businessA, "vs_a", SubjectRef{Type: SubjectCustomer, Ref: "cus_tier_ledger"})

	// No account yet: a ladder exists but nothing has been earned. That is
	// NO_TIER, which is not the same statement as NOT_AVAILABLE.
	tier, err := e.service.ResolveCustomerLoyaltyTier(ctx, businessA, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tier.State != LoyaltyTierNone {
		t.Fatalf("state = %q; want NO_TIER for a configured ladder not yet reached", tier.State)
	}
	if tier.NextTierKey != "BRONZE" {
		t.Fatalf("next tier = %q; want BRONZE", tier.NextTierKey)
	}

	entry := e.credit(t, businessA, "vs_a", profile, 1500, "ord_tier_1")
	if tier, err = e.service.ResolveCustomerLoyaltyTier(ctx, businessA, profile.ID); err != nil {
		t.Fatal(err)
	}
	if tier.State != LoyaltyTierCurrent || tier.TierKey != "GOLD" {
		t.Fatalf("after 1500 points: state=%q tier=%q; want CURRENT/GOLD", tier.State, tier.TierKey)
	}
	if tier.NextTierKey != "PLATINUM" || tier.PointsToNextTier == nil || *tier.PointsToNextTier != 3500 {
		t.Fatalf("next = %q at %v; want PLATINUM at 3500", tier.NextTierKey, tier.PointsToNextTier)
	}

	// Reversing the credit removes the lifetime points that earned the rung, so
	// standing falls back with no recalculation step.
	if _, _, err = e.service.ReverseLoyaltyEntry(ctx, actor, Scope{Global: true}, entry.ID,
		ReverseLoyaltyEntryInput{Reason: "posted in error", CommandRef: "cmd_rev_tier"}); err != nil {
		t.Fatal(err)
	}
	if tier, err = e.service.ResolveCustomerLoyaltyTier(ctx, businessA, profile.ID); err != nil {
		t.Fatal(err)
	}
	// BRONZE sits at threshold 0, so it is held by anyone with an account: the
	// claim being tested is that the rung the reversed credit BOUGHT is
	// surrendered with it, not that standing disappears entirely.
	if tier.State != LoyaltyTierCurrent || tier.TierKey != "BRONZE" {
		t.Fatalf("after reversal: state=%q tier=%q; want CURRENT/BRONZE. A credit posted "+
			"in error must not confer permanent status, which is exactly what reversing it undoes",
			tier.State, tier.TierKey)
	}
	if tier.LifetimePoints == nil || *tier.LifetimePoints != 0 {
		t.Fatalf("lifetime points after reversal = %v; want 0. Standing counts credit "+
			"entries that have not been reversed, not the immutable lifetime counter",
			tier.LifetimePoints)
	}
}

// TestLoyaltyTiers_RedemptionDoesNotDemote documents the chosen definition:
// standing recognises what a customer has earned over the relationship, so
// spending points must not cost them their rung.
func TestLoyaltyTiers_RedemptionDoesNotDemote(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	e.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)

	if _, err := e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("BRONZE", 0, "GOLD", 1000), nil); err != nil {
		t.Fatal(err)
	}
	profile := e.seedProfile(t, businessA, "vs_a", SubjectRef{Type: SubjectCustomer, Ref: "cus_tier_spend"})
	e.credit(t, businessA, "vs_a", profile, 1200, "ord_tier_spend")

	if _, _, err := e.service.AdjustPoints(ctx, actor, Scope{Global: true}, AdjustPointsInput{
		CustomerProfileRef: profile.ID, Direction: LoyaltyDirectionDebit, Points: 1000,
		Reason: "redeemed against a reward", CommandRef: "cmd_tier_redeem",
	}); err != nil {
		t.Fatal(err)
	}

	tier, err := e.service.ResolveCustomerLoyaltyTier(ctx, businessA, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tier.State != LoyaltyTierCurrent || tier.TierKey != "GOLD" {
		t.Fatalf("after redemption: state=%q tier=%q; want GOLD retained. Tier is lifetime "+
			"credited points, not spendable balance, so redeeming must not demote", tier.State, tier.TierKey)
	}
}

// TestLoyaltyTiers_AreTenantScoped keeps one business's ladder invisible to
// another, which matters because a ladder discloses commercial strategy.
func TestLoyaltyTiers_AreTenantScoped(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()

	if _, err := e.service.ReplaceLoyaltyTiers(ctx, actorFor(businessA, "vs_a"),
		ladder("BRONZE", 0, "GOLD", 1000), nil); err != nil {
		t.Fatal(err)
	}
	set, err := e.service.GetLoyaltyTiers(ctx, businessB)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Tiers) != 0 {
		t.Fatalf("business B saw %d of business A's tiers", len(set.Tiers))
	}
}

// TestLoyaltyTiers_CarryNoMonetaryAuthority scans the live schema. A tier that
// silently conferred value would have made this CRM feature a commerce
// feature, and payment-wallet-service is the only monetary authority here.
func TestLoyaltyTiers_CarryNoMonetaryAuthority(t *testing.T) {
	e := tierEnv(t)
	rows, err := e.pool.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		 WHERE table_name = 'crm_loyalty_tier_definitions'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	banned := []string{"amount", "minor", "currency", "discount", "benefit",
		"credit_limit", "balance", "price", "fee", "multiplier"}
	for rows.Next() {
		var col string
		if err = rows.Scan(&col); err != nil {
			t.Fatal(err)
		}
		for _, b := range banned {
			if strings.Contains(col, b) {
				t.Fatalf("crm_loyalty_tier_definitions.%s looks monetary or benefit-bearing. "+
					"Tiers confer recognition only; money belongs to payment-wallet-service", col)
			}
		}
	}
}

// TestLoyaltyTiers_AppearInCustomer360 confirms the composition point, since
// tier standing is presented through the customer record rather than its own
// resource.
func TestLoyaltyTiers_AppearInCustomer360(t *testing.T) {
	e := tierEnv(t)
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	e.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)

	if _, err := e.service.ReplaceLoyaltyTiers(ctx, actor, ladder("BRONZE", 0, "GOLD", 500), nil); err != nil {
		t.Fatal(err)
	}
	profile := e.seedProfile(t, businessA, "vs_a", SubjectRef{Type: SubjectCustomer, Ref: "cus_tier_360"})
	e.credit(t, businessA, "vs_a", profile, 800, "ord_tier_360")

	view, err := e.service.Customer360For(ctx, actor, Scope{Global: true}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if view.LoyaltyTier == nil {
		t.Fatal("Customer 360 carried no loyalty_tier block")
	}
	if view.LoyaltyTier.State != LoyaltyTierCurrent || view.LoyaltyTier.TierKey != "GOLD" {
		t.Fatalf("360 tier = %q/%q; want CURRENT/GOLD",
			view.LoyaltyTier.State, view.LoyaltyTier.TierKey)
	}
}
