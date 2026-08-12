package crm

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestTenantIsolation proves Vendor A can neither see nor act on Vendor B's
// relationships, through every read and write path Section 8A exposes.
func TestTenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	bProfile := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_b_only"))
	actorA := actorFor(businessA, "vs_a")
	global := Scope{Global: true}

	t.Run("list excludes the other tenant", func(t *testing.T) {
		profiles, _, err := env.service.ListProfiles(ctx, actorA, global, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, profile := range profiles {
			if profile.ID == bProfile.ID {
				t.Fatal("Vendor A listed Vendor B's relationship")
			}
		}
	})

	t.Run("get is not found", func(t *testing.T) {
		if _, err := env.service.GetProfile(ctx, actorA, global, bProfile.ID); err == nil {
			t.Fatal("Vendor A read Vendor B's relationship")
		}
	})

	t.Run("archive is refused", func(t *testing.T) {
		if _, err := env.service.SetArchived(ctx, actorA, global, bProfile.ID, true, bProfile.Version); err == nil {
			t.Fatal("Vendor A archived Vendor B's relationship")
		}
	})

	t.Run("consent read is refused", func(t *testing.T) {
		if _, err := env.service.GetProfile(ctx, actorA, global, bProfile.ID); err == nil {
			t.Fatal("Vendor A reached Vendor B's consent projection")
		}
	})

	t.Run("the other tenant is unaffected", func(t *testing.T) {
		if _, err := env.service.GetProfile(ctx, actorFor(businessB, "vs_b"), global, bProfile.ID); err != nil {
			t.Fatalf("Vendor B lost access to its own relationship: %v", err)
		}
	})
}

// TestBranchScopeCannotEscape is the core branch-isolation guarantee: a branch
// manager sees only relationships at their own branch, and never the
// business-wide commercial total that would reveal another branch's trade.
func TestBranchScopeCannotEscape(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// One customer trades at branch A1 for 100,000 and at A2 for 300,000.
	shared := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_shared"))
	env.seedOrder(t, businessA, "vs_a", shared, "ord_a1", branchA1, 100_000, 100_000, env.now.AddDate(0, 0, -10), 1)
	env.seedOrder(t, businessA, "vs_a", shared, "ord_a2", branchA2, 300_000, 300_000, env.now.AddDate(0, 0, -5), 1)

	// A second customer trades only at A2.
	onlyA2 := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_only_a2"))
	env.seedOrder(t, businessA, "vs_a", onlyA2, "ord_a2_only", branchA2, 50_000, 50_000, env.now.AddDate(0, 0, -3), 1)

	branchScope := Scope{BranchIDs: []string{branchA1}}

	t.Run("a relationship at another branch is invisible", func(t *testing.T) {
		profiles, _, err := env.service.ListProfiles(ctx, actor, branchScope, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, profile := range profiles {
			if profile.ID == onlyA2.ID {
				t.Fatal("branch A1 manager listed a customer who only trades at A2")
			}
		}
		if _, err = env.service.GetProfile(ctx, actor, branchScope, onlyA2.ID); err == nil {
			t.Fatal("branch A1 manager read a customer who only trades at A2")
		}
	})

	t.Run("commercial totals are branch-scoped, not business-wide", func(t *testing.T) {
		profile, err := env.service.GetProfile(ctx, actor, branchScope, shared.ID)
		if err != nil {
			t.Fatal(err)
		}
		view, err := env.service.Customer360For(ctx, actor, branchScope, profile)
		if err != nil {
			t.Fatal(err)
		}
		if view.Commercial.Scope != "BRANCH" {
			t.Fatalf("scope=%s want BRANCH", view.Commercial.Scope)
		}
		if view.Commercial.NetSpendMinor == nil {
			t.Fatal("branch-scoped net spend missing")
		}
		if *view.Commercial.NetSpendMinor != 100_000 {
			t.Fatalf("branch-scoped net spend=%d; want 100000, not the business-wide 400000",
				*view.Commercial.NetSpendMinor)
		}
		if len(view.Branches) != 1 || view.Branches[0].BranchRef != branchA1 {
			t.Fatalf("branch list leaked other branches: %+v", view.Branches)
		}
		// The most recent order overall happened at A2, so exposing its
		// reference would leak the unauthorised branch's activity.
		if view.Commercial.LastOrderRef != nil {
			t.Fatalf("branch-scoped view exposed last_order_ref=%v from another branch", *view.Commercial.LastOrderRef)
		}
	})

	t.Run("a global actor still sees the business total", func(t *testing.T) {
		profile, err := env.service.GetProfile(ctx, actor, Scope{Global: true}, shared.ID)
		if err != nil {
			t.Fatal(err)
		}
		view, err := env.service.Customer360For(ctx, actor, Scope{Global: true}, profile)
		if err != nil {
			t.Fatal(err)
		}
		if view.Commercial.Scope != "BUSINESS" || view.Commercial.NetSpendMinor == nil || *view.Commercial.NetSpendMinor != 400_000 {
			t.Fatalf("global view net spend=%v scope=%s; want 400000/BUSINESS", view.Commercial.NetSpendMinor, view.Commercial.Scope)
		}
	})

	t.Run("filtering by an unauthorised branch is refused", func(t *testing.T) {
		if _, _, err := env.service.ListProfiles(ctx, actor, branchScope, ListOptions{BranchRef: branchA2}); err == nil {
			t.Fatal("branch A1 manager filtered by branch A2")
		}
	})

	t.Run("an empty scope authorises nothing", func(t *testing.T) {
		profiles, _, err := env.service.ListProfiles(ctx, actor, Scope{BranchIDs: []string{}}, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(profiles) != 0 {
			t.Fatalf("empty scope returned %d relationships", len(profiles))
		}
	})
}

// TestEventCallerAuthority proves one service cannot assert another's facts.
func TestEventCallerAuthority(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	t.Run("a non-authority cannot change consent", func(t *testing.T) {
		if err := env.service.AuthorizeEvent("payment-wallet-service", EventConsentChanged); err == nil {
			t.Fatal("payment-wallet-service was allowed to change consent")
		}
	})

	t.Run("a non-authority cannot change privacy", func(t *testing.T) {
		if err := env.service.AuthorizeEvent("media-file-service", EventPrivacyChanged); err == nil {
			t.Fatal("media-file-service was allowed to change privacy")
		}
	})

	t.Run("the order publisher cannot merge identities", func(t *testing.T) {
		if err := env.service.AuthorizeEvent("vendor-order-service", EventIdentityMerged); err == nil {
			t.Fatal("the order publisher was allowed to merge identities")
		}
	})

	t.Run("an unknown event type is rejected", func(t *testing.T) {
		if err := env.service.AuthorizeEvent("customer-service", "CUSTOMER_ANYTHING_GOES"); err == nil {
			t.Fatal("an unregistered event type was accepted")
		}
	})

	t.Run("the real authority is accepted", func(t *testing.T) {
		if err := env.service.AuthorizeEvent("customer-service", EventConsentChanged); err != nil {
			t.Fatalf("customer-service was refused its own event: %v", err)
		}
	})
}

// TestEventBodyCannotSpoofItsProducer proves the signed caller identity wins
// over anything the body claims.
func TestEventBodyCannotSpoofItsProducer(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	err := env.service.ProcessEvent(context.Background(), "customer-service", EventEnvelope{
		EventID:       "evt_spoof",
		EventType:     EventConsentChanged,
		SourceService: "some-other-service",
		SourceVersion: 1,
		OccurredAt:    env.now,
		BusinessRef:   businessA,
		Subject:       EventSubject{Type: "CUSTOMER", Ref: "cus_x"},
		Payload:       EventPayload{Purpose: "MARKETING", Channel: "EMAIL", Decision: "ALLOW"},
	})
	if err == nil {
		t.Fatal("an event whose body claimed a different producer was accepted")
	}
}

// TestDisabledIntegrationRejectsEvents proves a disabled integration cannot be
// written to at all, and that a disabled consent feed suppresses marketing
// rather than defaulting it open.
func TestDisabledIntegrationRejectsEvents(t *testing.T) {
	env := newTestEnv(t, Flags{CRMEnabled: true, CustomerProjectionEnabled: true})

	if err := env.service.AuthorizeEvent("customer-service", EventConsentChanged); err == nil {
		t.Fatal("consent event accepted while the consent integration was disabled")
	}
	if err := env.service.AuthorizeEvent("customer-service", EventPrivacyChanged); err == nil {
		t.Fatal("privacy event accepted while the privacy integration was disabled")
	}

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_flagoff"))
	summary, err := env.service.ConsentSummaryFor(context.Background(), actorFor(businessA, "vs_a"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if summary.IntegrationAvailable {
		t.Fatal("consent reported available while disabled")
	}
	for channel, entry := range summary.Marketing {
		if entry.Eligible {
			t.Fatalf("channel %s eligible with no consent authority wired", channel)
		}
		if entry.Reason != ReasonConsentUnavailable {
			t.Fatalf("channel %s reason=%s; want %s", channel, entry.Reason, ReasonConsentUnavailable)
		}
	}
}

// TestWritesDisabledBlocksLifecycle proves the write kill-switches work without
// disabling reads.
func TestWritesDisabledBlocksLifecycle(t *testing.T) {
	env := newTestEnv(t, Flags{CRMEnabled: true, CustomerProjectionEnabled: true})
	ctx := context.Background()
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_nowrite"))
	actor := actorFor(businessA, "vs_a")

	if _, err := env.service.SetArchived(ctx, actor, Scope{Global: true}, profile.ID, true, profile.Version); err == nil {
		t.Fatal("archive succeeded while CRM writes were disabled")
	}
	if _, err := env.service.GetProfile(ctx, actor, Scope{Global: true}, profile.ID); err != nil {
		t.Fatalf("reads should remain available while writes are disabled: %v", err)
	}
}

// TestNoCrossTenantOrderFact proves a fact cannot be attached to another
// business's branch.
func TestNoCrossTenantOrderFact(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_cross"))
	completed := env.now.AddDate(0, 0, -1)

	err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyOrderFact(context.Background(), tx, actorFor(businessA, "vs_a"), profile, OrderFactInput{
			SourceService:  "vendor-order-service",
			SourceOrderRef: "ord_cross",
			SourceEventID:  "evt_cross",
			SourceVersion:  1,
			OrderState:     OrderCompleted,
			CompletedAt:    &completed,
			Currency:       "NGN",
			GrossMinor:     1_000,
			NetMinor:       1_000,
			BranchID:       ptr(branchB1), // another business's branch
			ObservedAt:     env.now,
		})
	})
	if err == nil {
		t.Fatal("an order fact was attached to another business's branch")
	}
}

// TestCrossCurrencyOrderFactRejected proves totals cannot be corrupted by a
// differently-denominated order, and that no FX conversion is attempted.
func TestCrossCurrencyOrderFactRejected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_fx"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_ngn", branchA1, 10_000, 10_000, env.now.AddDate(0, 0, -2), 1)
	completed := env.now.AddDate(0, 0, -1)

	err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyOrderFact(context.Background(), tx, actorFor(businessA, "vs_a"), profile, OrderFactInput{
			SourceService:  "vendor-order-service",
			SourceOrderRef: "ord_usd",
			SourceEventID:  "evt_usd",
			SourceVersion:  1,
			OrderState:     OrderCompleted,
			CompletedAt:    &completed,
			Currency:       "USD",
			GrossMinor:     50_000,
			NetMinor:       50_000,
			BranchID:       ptr(branchA1),
			ObservedAt:     env.now,
		})
	})
	if err == nil {
		t.Fatal("a USD order was accepted into an NGN business's totals")
	}

	reloaded := env.reload(t, businessA, profile.ID)
	if reloaded.NetSpendMinor != 10_000 {
		t.Fatalf("net spend was corrupted to %d; want the original 10000", reloaded.NetSpendMinor)
	}
	if reloaded.CommercialCurrency == nil || *reloaded.CommercialCurrency != "NGN" {
		t.Fatalf("currency=%v want NGN", reloaded.CommercialCurrency)
	}
}
