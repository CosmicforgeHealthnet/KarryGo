package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"errors"
	"testing"
)

func TestVendorSecureAdminPauseBlocksApprovedQuoteButPreservesCommittedOrder(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	e.enroll(t, "branches", e.f.env.branchA)
	pause := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 1, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, pause); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatal("approved quote bypassed native branch pause", err)
	}
	var count int
	if err := e.f.env.pool.QueryRow(ctx, `SELECT count(*) FROM commerce_parent_orders WHERE quote_id=$1`, e.f.quoteID).Scan(&count); err != nil || count != 0 {
		t.Fatal("paused branch created order", count, err)
	}
	reopen := e.cmd("REOPEN_BRANCH", "branches", e.f.env.branchA, "", 2, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, reopen); err != nil {
		t.Fatal(err)
	}
	original, created, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger)
	if err != nil || !created {
		t.Fatal("approved reopening did not restore booking", err)
	}
	pause = e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 3, map[string]string{})
	if _, err = e.svc.Execute(ctx, e.actor, "case-1", e.scope, pause); err != nil {
		t.Fatal(err)
	}
	replay, created, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger)
	if err != nil || created || replay.ID != original.ID {
		t.Fatal("pause disrupted committed order readback", err)
	}
}
