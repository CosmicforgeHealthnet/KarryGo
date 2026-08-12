package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
)

// TestListProviderMemberships_CrossOrganizationHistoryAndProviderIsolation
// proves the Section 28/34 provider-self "membership history" read, wired
// through app.NewMembershipHistoryReader (the same composition-root adapter
// cmd/main.go uses), returns every membership a provider identity has ever
// held across organizations, and never another provider's memberships --
// against real Postgres, using the SAME raw provider_service value real
// membership creation uses (platformclients.OwningService), not the
// marketplace "DISPATCH"-style abbreviation.
func TestListProviderMemberships_CrossOrganizationHistoryAndProviderIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	orgA := createOrganization(t, pool, "Membership History Org A", "RC-MH-A")
	orgB := createOrganization(t, pool, "Membership History Org B", "RC-MH-B")

	providerID := "drv_history_" + randomHex()
	membershipA := seedProviderMembership(t, pool, orgA.ID, providerID)
	membershipB := seedProviderMembership(t, pool, orgB.ID, "drv_other_"+randomHex())

	svc := mktusecases.NewService(mktusecases.Options{
		Pool:              pool,
		Projections:       mktrepos.NewProjectionRepository(pool),
		Offers:            mktrepos.NewOfferRepository(pool),
		Assignments:       mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts:   mktrepos.NewExecutionStartRepository(pool),
		Trips:             mktrepos.NewTripRepository(pool),
		Commands:          mktrepos.NewCommandRepository(pool),
		MembershipHistory: app.NewMembershipHistoryReader(providerrepos.NewRepository(pool)),
	})

	memberships, err := svc.ListProviderMemberships(ctx, membershipA.ProviderService, providerID)
	if err != nil {
		t.Fatalf("ListProviderMemberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].MembershipID != membershipA.ID {
		t.Fatalf("memberships = %+v, want exactly the one membership for %q", memberships, providerID)
	}
	if memberships[0].OrganizationID != orgA.ID {
		t.Fatalf("membership organizationId = %q, want %q", memberships[0].OrganizationID, orgA.ID)
	}

	// A different provider identity sees only its own membership -- never
	// membershipA's, even though both live in the same provider_service.
	otherProviderID := *membershipB.ProviderID
	othersView, err := svc.ListProviderMemberships(ctx, membershipB.ProviderService, otherProviderID)
	if err != nil {
		t.Fatalf("ListProviderMemberships for other provider: %v", err)
	}
	if len(othersView) != 1 || othersView[0].MembershipID != membershipB.ID {
		t.Fatalf("other provider's memberships = %+v, want exactly membershipB", othersView)
	}
	for _, m := range othersView {
		if m.MembershipID == membershipA.ID {
			t.Fatal("cross-provider leak: saw another provider's membership")
		}
	}
}

// TestListProviderMemberships_UnwiredReaderReportsUnavailable proves a nil
// MembershipHistory reports the capability as unavailable rather than
// panicking or silently returning an empty (and therefore misleading) list.
func TestListProviderMemberships_UnwiredReaderReportsUnavailable(t *testing.T) {
	pool := testPool(t)
	svc := mktusecases.NewService(mktusecases.Options{
		Pool: pool, Projections: mktrepos.NewProjectionRepository(pool),
		Offers: mktrepos.NewOfferRepository(pool), Assignments: mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool), Trips: mktrepos.NewTripRepository(pool),
		Commands: mktrepos.NewCommandRepository(pool),
	})
	if _, err := svc.ListProviderMemberships(context.Background(), "driver-dispatch-delivery-service", "drv_x"); err == nil {
		t.Fatal("expected an unavailable error when MembershipHistory is not wired")
	}
}
