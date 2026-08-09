package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// newAutoReassignmentService builds a marketplace service with the real
// eligibility checker and the real candidate membership lister (both wired
// the same way the composition root wires them).
func newAutoReassignmentService(pool *pgxpool.Pool, reassignmentEnabled bool, automatic bool) *mktusecases.Service {
	providerRepo := providerrepos.NewRepository(pool)
	return mktusecases.NewService(mktusecases.Options{
		Pool:        pool,
		Projections: mktrepos.NewProjectionRepository(pool),
		Offers:      mktrepos.NewOfferRepository(pool),
		Assignments: mktrepos.NewAssignmentRepository(pool),
		Trips:       mktrepos.NewTripRepository(pool),
		Commands:    mktrepos.NewCommandRepository(pool),
		Events:      events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Eligibility: mktusecases.NewEligibilityChecker(app.NewMembershipReader(providerRepo), nil, nil),
		// Reassign also consults the durable eligibility gate; a real,
		// always-healthy stub authority is required or every reassignment
		// attempt fails with "eligibility could not be confirmed" before
		// ever reaching the candidate-selection logic under test.
		DurableEligibility:   newEligibilityService(pool, healthyAuthority("", "")),
		CandidateMemberships: app.NewCandidateMembershipLister(providerRepo),
		Flags: mktusecases.Flags{
			ReassignmentEnabled:          reassignmentEnabled,
			AutomaticReassignmentEnabled: automatic,
		},
	})
}

// TestSection27_ReassignmentEnabledFlagIsNowWired proves the pre-existing
// (but never-wired) Flags.ReassignmentEnabled gate actually works once set
// -- Reassign refuses when off, succeeds when on. This is the manual,
// human-dispatcher path; before this fix it always refused regardless of
// configuration, in every deployment.
func TestSection27_ReassignmentEnabledFlagIsNowWired(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Reassignment FlagWired Co", "RC-S27-REW-1")
	projection := seedProjection(t, pool, org.ID, "req-s27-reflag-1", mktmodels.ProjectionOrganizationAccepted)
	membershipA := seedProviderMembership(t, pool, org.ID, "drv-s27-reflag-a")
	membershipB := seedProviderMembership(t, pool, org.ID, "drv-s27-reflag-b")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membershipA, time.Now().Add(time.Hour))

	disabled := newAutoReassignmentService(pool, false, false)
	_, err := disabled.Reassign(ctx, mktusecases.ReassignInput{
		Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr_dispatcher",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_dispatcher"},
		AssignmentID: assignment.ID, MembershipID: membershipB.ID, Reason: "manual_test",
	})
	if err == nil {
		t.Fatal("expected Reassign to refuse when ReassignmentEnabled is false")
	}

	enabled := newAutoReassignmentService(pool, true, false)
	replacement, err := enabled.Reassign(ctx, mktusecases.ReassignInput{
		Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr_dispatcher",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_dispatcher"},
		AssignmentID: assignment.ID, MembershipID: membershipB.ID, Reason: "manual_test",
	})
	if err != nil {
		t.Fatalf("expected Reassign to succeed when ReassignmentEnabled is true: %v", err)
	}
	if replacement.MembershipID != membershipB.ID {
		t.Fatalf("replacement membership = %s, want %s", replacement.MembershipID, membershipB.ID)
	}
}

// TestSection27_AutoReassignStaleCase_TriesCandidatesAndSucceeds proves the
// automatic coordinator finds an eligible candidate (excluding the one that
// already failed) and reassigns to it, using nothing but the existing
// Reassign eligibility check -- no invented scoring.
func TestSection27_AutoReassignStaleCase_TriesCandidatesAndSucceeds(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Auto Reassign Success Co", "RC-S27-AR-1")
	projection := seedProjection(t, pool, org.ID, "req-s27-auto-1", mktmodels.ProjectionOrganizationAccepted)
	membershipA := seedProviderMembership(t, pool, org.ID, "drv-s27-auto-a")
	membershipB := seedProviderMembership(t, pool, org.ID, "drv-s27-auto-b")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membershipA, time.Now().Add(-time.Minute))

	// Expire it (real, unmodified expiry path) to open a real reassignment
	// case with membershipA excluded.
	expiryService := newExpiryService(pool)
	if _, err := expiryService.ExpireAssignments(ctx, 50); err != nil {
		t.Fatal(err)
	}
	assignRepo := mktrepos.NewAssignmentRepository(pool)
	openCase, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected an open reassignment case: %v", err)
	}

	service := newAutoReassignmentService(pool, true, true)
	replacement, err := service.AutoReassignStaleCase(ctx,
		tenant.Context{OrganizationID: org.ID, ActorType: tenant.ActorTypeSystem, ActorID: "test-coordinator"},
		*openCase, "automatic_reassignment")
	if err != nil {
		t.Fatalf("auto reassign: %v", err)
	}
	if replacement.MembershipID != membershipB.ID {
		t.Fatalf("auto-reassigned to %s, want the only other active membership %s", replacement.MembershipID, membershipB.ID)
	}
	if replacement.MembershipID == membershipA.ID {
		t.Fatal("auto reassignment must never pick the excluded (just-failed) membership")
	}

	replacementRow, err := assignRepo.GetByID(ctx, org.ID, replacement.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacementRow.ReplacesAssignmentID == nil || *replacementRow.ReplacesAssignmentID != assignment.ID {
		t.Fatalf("replacement.ReplacesAssignmentID = %v, want %s", replacementRow.ReplacesAssignmentID, assignment.ID)
	}

	// The case is now resolved -- a second staleness scan must not find it.
	stillOpen, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err == nil {
		t.Fatalf("expected no open reassignment case after successful auto-reassignment, got %+v", stillOpen)
	}
}

// TestSection27_AutoReassignStaleCase_NoEligibleCandidateLeavesCaseOpen
// proves that when no other active membership exists, the coordinator
// returns ErrNoEligibleCandidate and performs no mutation -- the case stays
// open for a human, the existing safe fallback.
func TestSection27_AutoReassignStaleCase_NoEligibleCandidateLeavesCaseOpen(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Auto Reassign NoCandidate Co", "RC-S27-AR-2")
	projection := seedProjection(t, pool, org.ID, "req-s27-auto-2", mktmodels.ProjectionOrganizationAccepted)
	membershipA := seedProviderMembership(t, pool, org.ID, "drv-s27-auto-nc-a")
	seedAssignment(t, pool, org.ID, projection.ID, membershipA, time.Now().Add(-time.Minute))

	expiryService := newExpiryService(pool)
	if _, err := expiryService.ExpireAssignments(ctx, 50); err != nil {
		t.Fatal(err)
	}
	assignRepo := mktrepos.NewAssignmentRepository(pool)
	openCase, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected an open reassignment case: %v", err)
	}

	service := newAutoReassignmentService(pool, true, true)
	_, err = service.AutoReassignStaleCase(ctx,
		tenant.Context{OrganizationID: org.ID, ActorType: tenant.ActorTypeSystem, ActorID: "test-coordinator"},
		*openCase, "automatic_reassignment")
	if !errors.Is(err, mktusecases.ErrNoEligibleCandidate) {
		t.Fatalf("err = %v, want ErrNoEligibleCandidate (only the excluded membership exists)", err)
	}

	stillOpen, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected the case to remain open for a human: %v", err)
	}
	if stillOpen.ID != openCase.ID {
		t.Fatalf("open case id = %s, want unchanged %s", stillOpen.ID, openCase.ID)
	}
}

// TestSection27_AutoReassignStaleCase_DisabledByDefault proves the
// coordinator refuses to run at all unless AutomaticReassignmentEnabled is
// explicitly set -- the safe default (human review) is preserved.
func TestSection27_AutoReassignStaleCase_DisabledByDefault(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Auto Reassign Disabled Co", "RC-S27-AR-3")
	projection := seedProjection(t, pool, org.ID, "req-s27-auto-3", mktmodels.ProjectionOrganizationAccepted)
	membershipA := seedProviderMembership(t, pool, org.ID, "drv-s27-auto-dis-a")
	seedAssignment(t, pool, org.ID, projection.ID, membershipA, time.Now().Add(-time.Minute))

	expiryService := newExpiryService(pool)
	if _, err := expiryService.ExpireAssignments(ctx, 50); err != nil {
		t.Fatal(err)
	}
	assignRepo := mktrepos.NewAssignmentRepository(pool)
	openCase, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected an open reassignment case: %v", err)
	}

	service := newAutoReassignmentService(pool, true, false) // automatic=false
	_, err = service.AutoReassignStaleCase(ctx,
		tenant.Context{OrganizationID: org.ID, ActorType: tenant.ActorTypeSystem, ActorID: "test-coordinator"},
		*openCase, "automatic_reassignment")
	if err == nil {
		t.Fatal("expected AutoReassignStaleCase to refuse when AutomaticReassignmentEnabled is false")
	}
}
