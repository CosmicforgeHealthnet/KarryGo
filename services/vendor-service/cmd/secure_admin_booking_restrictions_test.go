package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"errors"
	"testing"
	"time"
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

func TestVendorCheckoutFailsClosedForExpiredVerificationOnApprovedQuote(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	e.f.env.exec(ctx, `INSERT INTO vendor_onboarding_profiles(business_id,status) VALUES($1,'IN_PROGRESS') ON CONFLICT(business_id) DO UPDATE SET status='IN_PROGRESS'`, e.f.env.businessA)
	var branchCapability string
	if err := e.f.env.pool.QueryRow(ctx, `SELECT id FROM vendor_branch_capabilities WHERE business_id=$1 AND branch_id=$2 AND capability_type='SHOP_GROCERY'`, e.f.env.businessA, e.f.env.branchA).Scan(&branchCapability); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, projection := range []struct {
		id, dimension, subjectType, subjectRef string
		expiresAt                              time.Time
	}{
		{"section11-identity-" + e.f.env.suffix, "IDENTITY", "BUSINESS", e.f.env.businessA, now.Add(time.Hour)},
		{"section11-business-" + e.f.env.suffix, "BUSINESS", "BUSINESS", e.f.env.businessA, now.Add(time.Hour)},
		{"section11-location-" + e.f.env.suffix, "LOCATION", "BRANCH", e.f.env.branchA, now.Add(-time.Second)},
	} {
		e.f.env.exec(ctx, `INSERT INTO vendor_verification_projections(id,business_id,dimension,subject_type,subject_ref,source_service,source_ref,source_version,status,observed_at,expires_at) VALUES($1,$2,$3,$4,$5,'verification-compliance-service',$1,1,'VERIFIED',$6,$7)`, projection.id, e.f.env.businessA, projection.dimension, projection.subjectType, projection.subjectRef, now.Add(-time.Hour), projection.expiresAt)
	}
	_ = branchCapability // SHOP does not require FOOD_KITCHEN evidence.
	if _, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("expired Verification evidence did not block stale approved quote: %v", err)
	}
}

func TestVendorCheckoutRejectsSuspendedMerchantAndClosedBranch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, context.Context, vendorAdminFixture)
	}{
		{"suspended merchant", func(_ *testing.T, ctx context.Context, e vendorAdminFixture) {
			e.f.env.exec(ctx, `UPDATE vendor_businesses SET status='SUSPENDED' WHERE id=$1`, e.f.env.businessA)
		}},
		{"closed branch", func(_ *testing.T, ctx context.Context, e vendorAdminFixture) {
			e.f.env.exec(ctx, `UPDATE vendor_branches SET status='CLOSED' WHERE id=$1`, e.f.env.branchA)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := newVendorAdminFixture(t)
			ctx := context.Background()
			test.mutate(t, ctx, e)
			if _, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger); !errors.Is(err, orders.ErrQuoteNotOrderable) {
				t.Fatalf("restricted approved quote created an order: %v", err)
			}
		})
	}
}

func TestVendorCheckoutRejectsTakenDownRealListing(t *testing.T) {
	e := vendorAdminFixtureFor(t, newSection17EFixture(t, false))
	ctx := context.Background()
	var listing string
	if err := e.f.env.pool.QueryRow(ctx, `SELECT snapshot->>'listing_ref' FROM commerce_quote_lines WHERE quote_id=$1 AND COALESCE(snapshot->>'listing_ref','')<>'' LIMIT 1`, e.f.quoteID).Scan(&listing); err != nil {
		t.Fatal(err)
	}
	variant := e.f.env.seedVariant(ctx, s17cVariantOptions{})
	e.f.env.exec(ctx, `INSERT INTO shop_branch_listings(id,business_id,branch_id,product_id,variant_id,status) VALUES($1,$2,$3,$4,$5,'INACTIVE')`, listing, e.f.env.businessA, e.f.env.branchA, e.f.env.productA, variant)
	if _, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("taken-down real listing on approved quote created an order: %v", err)
	}
}
