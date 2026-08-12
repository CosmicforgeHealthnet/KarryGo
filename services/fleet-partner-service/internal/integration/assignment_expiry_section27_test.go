package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
)

// newExpiryService builds a marketplace service with a real outbox
// publisher (ExpireAssignments always publishes an event on expiry) and no
// execution contract configured -- the Section 27 assignment expiry and
// reassignment staleness workers never call out to an execution service.
func newExpiryService(pool *pgxpool.Pool) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		Pool:        pool,
		Projections: mktrepos.NewProjectionRepository(pool),
		Offers:      mktrepos.NewOfferRepository(pool),
		Assignments: mktrepos.NewAssignmentRepository(pool),
		Trips:       mktrepos.NewTripRepository(pool),
		Commands:    mktrepos.NewCommandRepository(pool),
		Events:      events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
}

// TestSection27_ExpireAssignments_ReleasesOverdueAndOpensReassignmentCase
// proves the assignment expiry worker's underlying call releases an overdue
// PENDING_PROVIDER_RESPONSE assignment, transitions it to EXPIRED, and opens
// a reassignment case rather than silently dropping the work.
func TestSection27_ExpireAssignments_ReleasesOverdueAndOpensReassignmentCase(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Expiry Section27 Co", "RC-S27-EXP-1")
	projection := seedProjection(t, pool, org.ID, "req-s27-expiry-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-s27-expiry-1")
	// Deadline already in the past -- immediately eligible for ClaimExpired.
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(-time.Minute))

	service := newExpiryService(pool)
	expired, err := service.ExpireAssignments(ctx, 50)
	if err != nil {
		t.Fatalf("expire assignments: %v", err)
	}
	if expired < 1 {
		t.Fatalf("expired = %d, want at least 1", expired)
	}

	assignRepo := mktrepos.NewAssignmentRepository(pool)
	reloaded, err := assignRepo.GetByID(ctx, org.ID, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != mktmodels.AssignmentExpired {
		t.Fatalf("status = %s, want EXPIRED", reloaded.Status)
	}

	openCase, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected an open reassignment case: %v", err)
	}
	if openCase.OriginAssignmentID != assignment.ID {
		t.Fatalf("reassignment case origin = %s, want %s", openCase.OriginAssignmentID, assignment.ID)
	}
	if openCase.Trigger != mktmodels.TriggerExpired {
		t.Fatalf("reassignment case trigger = %s, want EXPIRED", openCase.Trigger)
	}

	// A second pass must be a no-op -- the assignment is no longer PENDING,
	// so ClaimExpired's WHERE clause excludes it, and expiry never fires
	// twice for the same assignment.
	expiredAgain, err := service.ExpireAssignments(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if expiredAgain != 0 {
		t.Fatalf("second pass expired = %d, want 0 (already resolved)", expiredAgain)
	}
}

// TestSection27_ExpireAssignments_NotYetOverdueIsUntouched proves an
// assignment still within its response deadline is never expired early.
func TestSection27_ExpireAssignments_NotYetOverdueIsUntouched(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Expiry NotDue Section27 Co", "RC-S27-EXP-2")
	projection := seedProjection(t, pool, org.ID, "req-s27-expiry-2", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-s27-expiry-2")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Hour))

	service := newExpiryService(pool)
	if _, err := service.ExpireAssignments(ctx, 50); err != nil {
		t.Fatal(err)
	}

	assignRepo := mktrepos.NewAssignmentRepository(pool)
	reloaded, err := assignRepo.GetByID(ctx, org.ID, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != mktmodels.AssignmentPendingProviderResponse {
		t.Fatalf("status = %s, want unchanged PENDING_PROVIDER_RESPONSE", reloaded.Status)
	}
}

// TestSection27_ScanStaleReassignmentCases_CountsOnlyOldOpenCases proves the
// reassignment coordinator's staleness read-back counts OPEN/IN_PROGRESS
// cases older than the threshold, and nothing else.
func TestSection27_ScanStaleReassignmentCases_CountsOnlyOldOpenCases(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Reassignment Stale Co", "RC-S27-STALE-1")
	projection := seedProjection(t, pool, org.ID, "req-s27-stale-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-s27-stale-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(-time.Minute))

	service := newExpiryService(pool)
	if _, err := service.ExpireAssignments(ctx, 50); err != nil {
		t.Fatal(err)
	}
	assignRepo := mktrepos.NewAssignmentRepository(pool)
	openCase, err := assignRepo.GetOpenReassignmentCase(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("expected an open reassignment case: %v", err)
	}
	_ = assignment

	// Freshly opened -- not yet stale under a 5-minute threshold.
	fresh, err := service.ScanStaleReassignmentCases(ctx, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if fresh != 0 {
		t.Fatalf("fresh stale count = %d, want 0 for a case opened moments ago", fresh)
	}

	// Backdate the case's created_at past the threshold.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_reassignment_cases SET created_at = now() - interval '10 minutes' WHERE id = $1`,
		openCase.ID); err != nil {
		t.Fatal(err)
	}
	stale, err := service.ScanStaleReassignmentCases(ctx, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if stale < 1 {
		t.Fatalf("stale count = %d, want at least 1 after backdating past the threshold", stale)
	}

	// Resolving the case removes it from the stale count.
	if err := assignRepo.ResolveReassignmentCase(ctx, pool, openCase.ID, assignment.ID, openCase.Version); err != nil {
		t.Fatal(err)
	}
	resolvedStale, err := service.ScanStaleReassignmentCases(ctx, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedStale >= stale {
		t.Fatalf("stale count after resolving = %d, want fewer than %d", resolvedStale, stale)
	}
}
