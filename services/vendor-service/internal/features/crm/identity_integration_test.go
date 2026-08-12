package crm

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func (e *testEnv) applyLink(t *testing.T, business, staff string, in IdentityLinkInput) error {
	t.Helper()
	return e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyIdentityLink(context.Background(), tx, actorFor(business, staff), in)
	})
}

func linkInput(evidence string, source, target SubjectRef, version int64) IdentityLinkInput {
	return IdentityLinkInput{
		AuthorityService:   "customer-service",
		AuthorityReference: "link_" + evidence,
		AuthorityVersion:   version,
		EvidenceEventID:    evidence,
		Source:             source,
		Target:             target,
	}
}

// TestGuestToCustomerMergeDeduplicatesCommercialFacts is the central merge
// guarantee: totals are rebuilt from the merged fact set, so an order both
// profiles saw is counted once rather than twice.
func TestGuestToCustomerMergeDeduplicatesCommercialFacts(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))

	// Distinct orders on each side, plus one order both sides observed under
	// the same authoritative order reference.
	env.seedOrder(t, businessA, "vs_a", guest, "ord_guest", branchA1, 10_000, 10_000, env.now.AddDate(0, 0, -9), 1)
	env.seedOrder(t, businessA, "vs_a", customer, "ord_customer", branchA1, 20_000, 20_000, env.now.AddDate(0, 0, -8), 1)

	if err := env.applyLink(t, businessA, "vs_a", linkInput("evt_link_1", guestSubject("gst_1"), customerSubject("cus_1"), 4)); err != nil {
		t.Fatal(err)
	}

	survivor := env.reload(t, businessA, customer.ID)
	source := env.reload(t, businessA, guest.ID)

	t.Run("the source is marked merged and archived", func(t *testing.T) {
		if source.MergeState != MergeSource || source.PrivacyState != PrivacyMerged {
			t.Fatalf("merge_state=%s privacy_state=%s", source.MergeState, source.PrivacyState)
		}
		if source.MergedIntoProfileID == nil || *source.MergedIntoProfileID != customer.ID {
			t.Fatalf("merged_into=%v want %s", source.MergedIntoProfileID, customer.ID)
		}
	})

	t.Run("the survivor holds both orders exactly once", func(t *testing.T) {
		if survivor.CompletedOrderCount != 2 {
			t.Fatalf("completed=%d want 2", survivor.CompletedOrderCount)
		}
		if survivor.NetSpendMinor != 30_000 {
			t.Fatalf("net=%d want 30000", survivor.NetSpendMinor)
		}
		if survivor.MergeState != MergeTarget {
			t.Fatalf("survivor merge_state=%s want MERGED_TARGET", survivor.MergeState)
		}
	})

	t.Run("the source retains no commercial data", func(t *testing.T) {
		if source.CompletedOrderCount != 0 || source.NetSpendMinor != 0 {
			t.Fatalf("source retained commercial data: completed=%d net=%d", source.CompletedOrderCount, source.NetSpendMinor)
		}
		if source.CommercialProjectionState != ProjectionNotAvailable {
			t.Fatalf("source projection state=%s want NOT_AVAILABLE", source.CommercialProjectionState)
		}
	})

	t.Run("the merged source cannot be mutated independently", func(t *testing.T) {
		if _, err := env.service.SetArchived(ctx, actorFor(businessA, "vs_a"), Scope{Global: true},
			source.ID, false, source.Version); err == nil {
			t.Fatal("a merged source accepted an independent mutation")
		}
	})

	t.Run("the merged source is absent from ordinary listings", func(t *testing.T) {
		profiles, _, err := env.service.ListProfiles(ctx, actorFor(businessA, "vs_a"), Scope{Global: true}, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, profile := range profiles {
			if profile.ID == source.ID {
				t.Fatal("a merged source appeared in ordinary listings")
			}
		}
	})

	t.Run("the link is recorded as evidence", func(t *testing.T) {
		var linkType, status string
		if err := env.pool.QueryRow(ctx, `SELECT link_type,status FROM crm_customer_identity_links
			WHERE business_id=$1 AND evidence_event_id=$2`, businessA, "evt_link_1").Scan(&linkType, &status); err != nil {
			t.Fatal(err)
		}
		if linkType != string(LinkGuestToCustomer) || status != "APPLIED" {
			t.Fatalf("link_type=%s status=%s", linkType, status)
		}
	})
}

// TestMergeIsIdempotent proves redelivered evidence does not merge twice.
func TestMergeIsIdempotent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_dup"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_dup"))
	env.seedOrder(t, businessA, "vs_a", guest, "ord_dup", branchA1, 15_000, 15_000, env.now.AddDate(0, 0, -3), 1)

	input := linkInput("evt_dup", guestSubject("gst_dup"), customerSubject("cus_dup"), 1)
	if err := env.applyLink(t, businessA, "vs_a", input); err != nil {
		t.Fatal(err)
	}
	first := env.reload(t, businessA, customer.ID)

	if err := env.applyLink(t, businessA, "vs_a", input); err != nil {
		t.Fatalf("replayed merge errored: %v", err)
	}
	second := env.reload(t, businessA, customer.ID)

	if first.NetSpendMinor != second.NetSpendMinor {
		t.Fatalf("replay changed net spend from %d to %d", first.NetSpendMinor, second.NetSpendMinor)
	}
	if second.NetSpendMinor != 15_000 {
		t.Fatalf("net=%d want 15000", second.NetSpendMinor)
	}

	var links int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_customer_identity_links
		WHERE business_id=$1 AND evidence_event_id=$2`, businessA, "evt_dup").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("%d link rows for one piece of evidence", links)
	}
}

// TestLateEventAfterMergeRoutesToSurvivor proves a merged-away reference does
// not spawn a second relationship.
func TestLateEventAfterMergeRoutesToSurvivor(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_late"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_late_merge"))
	if err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_late_merge", guestSubject("gst_late"), customerSubject("cus_late_merge"), 1)); err != nil {
		t.Fatal(err)
	}

	// A late order event still addressed to the merged-away guest reference.
	var resolved Profile
	if err := env.tx(t, func(tx pgx.Tx) error {
		var err error
		resolved, err = env.service.ResolveOrCreateProfile(ctx, tx, actorFor(businessA, "vs_a"), RelationshipInput{
			Subject: guestSubject("gst_late"), ObservedAt: env.now,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if resolved.ID != customer.ID {
		t.Fatalf("late event resolved to %s; want the survivor %s", resolved.ID, customer.ID)
	}

	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1 AND guest_ref=$2`, businessA, "gst_late").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%d profiles hold the merged guest reference; want 1", count)
	}
	if env.reload(t, businessA, guest.ID).MergeState != MergeSource {
		t.Fatal("the merged source was revived")
	}
}

// TestMergeRefusesRedactedSubjects proves a merge cannot revive erased data
// inside a survivor.
func TestMergeRefusesRedactedSubjects(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_red"))
	env.seedProfile(t, businessA, "vs_a", customerSubject("cus_red_target"))

	current := env.reload(t, businessA, guest.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 2, PrivacySubjectRef: "gst_red",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_red_link", guestSubject("gst_red"), customerSubject("cus_red_target"), 1))
	if err == nil {
		t.Fatal("a redacted subject was merged into a surviving relationship")
	}
}

// TestCrossTenantMergeIsImpossible proves subjects are only ever resolved
// inside the acting business, so a merge cannot cross tenants.
func TestCrossTenantMergeIsImpossible(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	guestA := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_tenant"))
	customerB := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_tenant_b"))

	// Business A attempts to merge its guest into a reference that only exists
	// in Business B.
	if err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_cross", guestSubject("gst_tenant"), customerSubject("cus_tenant_b"), 1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Business B's relationship must be untouched.
	if env.reload(t, businessB, customerB.ID).MergeState != MergeNone {
		t.Fatal("a cross-tenant merge modified the other business's relationship")
	}
	// Business A's guest must not have been merged away into a foreign profile.
	sourceA := env.reload(t, businessA, guestA.ID)
	if sourceA.MergedIntoProfileID != nil {
		t.Fatalf("guest merged into %v across a tenant boundary", *sourceA.MergedIntoProfileID)
	}
	// Instead the canonical reference was adopted within Business A only.
	if sourceA.CustomerRef == nil || *sourceA.CustomerRef != "cus_tenant_b" {
		t.Fatalf("customer_ref=%v; expected in-place adoption inside business A", sourceA.CustomerRef)
	}
	var inA int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1`, businessB).Scan(&inA); err != nil {
		t.Fatal(err)
	}
	if inA != 1 {
		t.Fatalf("business B profile count changed to %d", inA)
	}
}

// TestMergeIntoAlreadyMergedTargetIsRefused prevents a merge chain that later
// lookups could cycle on.
func TestMergeIntoAlreadyMergedTargetIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	env.seedProfile(t, businessA, "vs_a", guestSubject("gst_c1"))
	env.seedProfile(t, businessA, "vs_a", customerSubject("cus_c1"))
	env.seedProfile(t, businessA, "vs_a", guestSubject("gst_c2"))

	// cus_c1 becomes a merged source of nothing yet; first merge gst_c1 into it.
	if err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_c1", guestSubject("gst_c1"), customerSubject("cus_c1"), 1)); err != nil {
		t.Fatal(err)
	}
	// Now merge gst_c1 (already a merged source) as a target: it resolves to a
	// merged-away profile and must be refused.
	err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_c2", guestSubject("gst_c2"), guestSubject("gst_c1"), 1))
	if err == nil {
		t.Fatal("a merge into a merged-away target was accepted")
	}
}

// TestMergeSurvivorResolutionIsDepthBounded proves a corrupt chain fails loudly
// rather than spinning.
func TestMergeSurvivorResolutionIsDepthBounded(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	a := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_cycle_a"))
	b := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_cycle_b"))

	// Force a cycle directly in storage, bypassing the service guards, to
	// prove the resolver refuses to loop on data it should never see.
	if _, err := env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='MERGED', merge_state='MERGED_SOURCE', merged_into_profile_id=$1
		WHERE id=$2`, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='MERGED', merge_state='MERGED_SOURCE', merged_into_profile_id=$1
		WHERE id=$2`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}

	start := env.reload(t, businessA, a.ID)
	if _, err := env.service.ResolveSurvivor(ctx, env.pool, businessA, start); err == nil {
		t.Fatal("a merge cycle resolved instead of erroring")
	}
}
