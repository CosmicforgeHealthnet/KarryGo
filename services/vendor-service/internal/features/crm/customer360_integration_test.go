package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestCustomer360WithoutCommercialDataIsHonest is the "do not fabricate zero"
// guarantee: a relationship with no observed orders reports NOT_AVAILABLE and
// omits every figure, rather than presenting a customer who spent nothing.
func TestCustomer360WithoutCommercialDataIsHonest(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_nodata"))
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, profile)
	if err != nil {
		t.Fatal(err)
	}

	if view.Commercial.State != ProjectionNotAvailable {
		t.Fatalf("state=%s want NOT_AVAILABLE", view.Commercial.State)
	}
	if view.Commercial.CompletedOrderCount != nil {
		t.Fatalf("order count reported as %d when nothing was ever observed", *view.Commercial.CompletedOrderCount)
	}
	if view.Commercial.NetSpendMinor != nil || view.Commercial.GrossSpendMinor != nil {
		t.Fatal("spend figures reported with no commercial data")
	}
	if view.Commercial.Currency != nil {
		t.Fatal("a currency was guessed with no commercial data")
	}
	if view.Commercial.RecencyDays != nil || view.Commercial.OrderFrequencyPerYear != nil {
		t.Fatal("recency/frequency inferred from profile creation rather than orders")
	}
}

// TestCustomer360ReportsFutureModulesAsUnimplemented keeps the view honest
// about 8B-8H.
func TestCustomer360ReportsFutureModulesAsUnimplemented(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_modules"))
	view, err := env.service.Customer360For(context.Background(), actorFor(businessA, "vs_a"), Scope{Global: true}, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"tags", "segments", "interactions", "tasks", "loyalty", "campaigns", "corporate", "intelligence"} {
		state, present := view.Modules[module]
		if !present {
			t.Fatalf("module %s missing from the availability map", module)
		}
		if state != ModuleNotImplemented {
			t.Fatalf("module %s reported %s while Section 8B-8H are unbuilt", module, state)
		}
	}
	if view.Capability.CategoryAffinity != ModuleNotImplemented || view.Capability.ItemAffinity != ModuleNotImplemented {
		t.Fatal("category/item affinity claimed availability without a catalogue")
	}
}

// TestCustomer360WithCommercialData covers the populated case end to end.
func TestCustomer360WithCommercialData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_data"))
	first := env.now.AddDate(0, 0, -60)
	last := env.now.AddDate(0, 0, -10)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_d1", branchA1, 100_000, 100_000, first, 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_d2", branchA2, 200_000, 150_000, last, 1)

	loaded := env.reload(t, businessA, profile.ID)
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, loaded)
	if err != nil {
		t.Fatal(err)
	}

	if view.Commercial.State != ProjectionCurrent {
		t.Fatalf("state=%s want CURRENT", view.Commercial.State)
	}
	if *view.Commercial.CompletedOrderCount != 2 {
		t.Fatalf("completed=%d want 2", *view.Commercial.CompletedOrderCount)
	}
	if *view.Commercial.GrossSpendMinor != 300_000 || *view.Commercial.NetSpendMinor != 250_000 {
		t.Fatalf("gross=%d net=%d want 300000/250000", *view.Commercial.GrossSpendMinor, *view.Commercial.NetSpendMinor)
	}
	if *view.Commercial.AverageOrderValueMinor != 125_000 {
		t.Fatalf("aov=%d want 125000", *view.Commercial.AverageOrderValueMinor)
	}
	if view.Commercial.Currency == nil || *view.Commercial.Currency != "NGN" {
		t.Fatalf("currency=%v want NGN", view.Commercial.Currency)
	}
	if !view.Commercial.FirstCompletedOrderAt.Equal(first) || !view.Commercial.LastCompletedOrderAt.Equal(last) {
		t.Fatalf("first=%v last=%v", view.Commercial.FirstCompletedOrderAt, view.Commercial.LastCompletedOrderAt)
	}
	if view.Commercial.RecencyDays == nil || *view.Commercial.RecencyDays != 10 {
		t.Fatalf("recency=%v want 10", view.Commercial.RecencyDays)
	}
	if len(view.Branches) != 2 {
		t.Fatalf("branch summaries=%d want 2", len(view.Branches))
	}
	if view.Profile.DisplayLabel != nil {
		t.Fatal("a display label was populated despite there being no approved presentation contract")
	}
}

// TestRefundReducesNetWithoutDoubleCounting is the refund-safety property
// against real storage.
func TestRefundReducesNetWithoutDoubleCounting(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_refund"))
	completed := env.now.AddDate(0, 0, -5)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_ref", branchA1, 50_000, 50_000, completed, 1)

	before := env.reload(t, businessA, profile.ID)
	if before.NetSpendMinor != 50_000 || before.CompletedOrderCount != 1 {
		t.Fatalf("pre-refund net=%d completed=%d", before.NetSpendMinor, before.CompletedOrderCount)
	}

	// The same order reference, now partially refunded, at a higher version.
	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyOrderFact(ctx, tx, actor, before, OrderFactInput{
			SourceService: "vendor-order-service", SourceOrderRef: "ord_ref", SourceEventID: "evt_ref_2",
			SourceVersion: 2, OrderState: OrderCompleted, CompletedAt: &completed, Currency: "NGN",
			GrossMinor: 50_000, NetMinor: 30_000, RefundedMinor: 20_000,
			BranchID: ptr(branchA1), ObservedAt: env.now,
		})
	}); err != nil {
		t.Fatal(err)
	}

	after := env.reload(t, businessA, profile.ID)
	if after.CompletedOrderCount != 1 {
		t.Fatalf("completed=%d; a refund created a second order", after.CompletedOrderCount)
	}
	if after.NetSpendMinor != 30_000 {
		t.Fatalf("net=%d want 30000", after.NetSpendMinor)
	}
	if after.GrossSpendMinor != 50_000 {
		t.Fatalf("gross=%d want 50000", after.GrossSpendMinor)
	}

	var facts int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_order_facts
		WHERE business_id=$1 AND source_order_ref=$2`, businessA, "ord_ref").Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if facts != 1 {
		t.Fatalf("%d fact rows for one order reference", facts)
	}
}

// TestStaleOrderVersionIsRejected proves an out-of-order redelivery cannot roll
// a projection backwards.
func TestStaleOrderVersionIsRejected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_staleorder"))
	completed := env.now.AddDate(0, 0, -2)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_s", branchA1, 40_000, 20_000, completed, 5)

	current := env.reload(t, businessA, profile.ID)
	err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyOrderFact(ctx, tx, actor, current, OrderFactInput{
			SourceService: "vendor-order-service", SourceOrderRef: "ord_s", SourceEventID: "evt_s_old",
			SourceVersion: 2, OrderState: OrderCompleted, CompletedAt: &completed, Currency: "NGN",
			GrossMinor: 40_000, NetMinor: 40_000, BranchID: ptr(branchA1), ObservedAt: env.now,
		})
	})
	if err == nil {
		t.Fatal("a stale order version overwrote a newer projection")
	}
	if after := env.reload(t, businessA, profile.ID); after.NetSpendMinor != 20_000 {
		t.Fatalf("net=%d; the stale version took effect", after.NetSpendMinor)
	}
}

// TestSameVersionConflictingOrderIsRejected proves two different facts cannot
// share one version.
func TestSameVersionConflictingOrderIsRejected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_conflict"))
	completed := env.now.AddDate(0, 0, -2)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_c", branchA1, 10_000, 10_000, completed, 3)

	current := env.reload(t, businessA, profile.ID)
	err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyOrderFact(ctx, tx, actor, current, OrderFactInput{
			SourceService: "vendor-order-service", SourceOrderRef: "ord_c", SourceEventID: "evt_c_other",
			SourceVersion: 3, OrderState: OrderCompleted, CompletedAt: &completed, Currency: "NGN",
			GrossMinor: 99_000, NetMinor: 99_000, BranchID: ptr(branchA1), ObservedAt: env.now,
		})
	})
	if err == nil {
		t.Fatal("conflicting data at the same source version was accepted")
	}
	if after := env.reload(t, businessA, profile.ID); after.GrossSpendMinor != 10_000 {
		t.Fatalf("gross=%d; the conflicting value was applied", after.GrossSpendMinor)
	}
}

// TestIdenticalOrderRedeliveryIsANoOp proves at-least-once delivery is safe.
func TestIdenticalOrderRedeliveryIsANoOp(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_redeliver"))
	completed := env.now.AddDate(0, 0, -2)

	env.seedOrder(t, businessA, "vs_a", profile, "ord_rd", branchA1, 25_000, 25_000, completed, 1)
	first := env.reload(t, businessA, profile.ID)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_rd", branchA1, 25_000, 25_000, completed, 1)
	second := env.reload(t, businessA, profile.ID)

	if first.Version != second.Version {
		t.Fatalf("redelivery bumped the version from %d to %d", first.Version, second.Version)
	}
	if second.NetSpendMinor != 25_000 || second.CompletedOrderCount != 1 {
		t.Fatalf("redelivery inflated totals: net=%d completed=%d", second.NetSpendMinor, second.CompletedOrderCount)
	}
}

// TestCommercialProjectionStaleness proves the freshness marker actually flips.
func TestCommercialProjectionStaleness(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_stalefresh"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_f", branchA1, 1_000, 1_000, env.now.AddDate(0, 0, -1), 1)

	loaded := env.reload(t, businessA, profile.ID)
	view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if view.Commercial.Stale {
		t.Fatal("a just-observed projection was reported stale")
	}

	// Advance the clock past the freshness window.
	env.now = env.now.Add(CommercialStaleAfter + time.Hour)
	view, err = env.service.Customer360For(ctx, actor, Scope{Global: true}, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Commercial.Stale {
		t.Fatal("an old projection was not reported stale")
	}
}
