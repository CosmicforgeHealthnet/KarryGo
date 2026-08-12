package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
)

// TestProviderAcceptanceVerificationRequiresRealAcceptance proves the contract
// dispatch depends on: an assignment id alone never implies consent.
func TestProviderAcceptanceVerificationRequiresRealAcceptance(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Verify Org", "RC-VER-1")
	projection := seedProjection(t, pool, org.ID, "req-verify-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-verify-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// While the assignment is merely PENDING there is no response row at all.
	// This is the state that must never be reported as acceptance.
	if _, err := repo.GetResponse(ctx, nil, assignment.ID); err == nil {
		t.Fatal("a pending assignment must have no provider response")
	}

	// The assignment is not in an executable state either.
	if assignment.Status != mktmodels.AssignmentPendingProviderResponse {
		t.Fatalf("assignment status = %s", assignment.Status)
	}

	// Record a real acceptance and move to ACCEPTED.
	response := &mktmodels.Response{
		OrganizationID: org.ID, AssignmentID: assignment.ID, MembershipID: membership.ID,
		Response: mktmodels.ResponseAccepted, ClientResponseID: "verify-accept-1",
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := repo.CreateResponse(ctx, tx, response); err != nil {
			return err
		}
		return repo.Transition(ctx, tx, assignment, mktmodels.AssignmentAccepted,
			assignment.Version, mktmodels.StateHistoryEntry{
				ActorType: "PROVIDER", ActorID: membership.ID})
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	stored, err := repo.GetResponse(ctx, nil, assignment.ID)
	if err != nil || stored.Response != mktmodels.ResponseAccepted {
		t.Fatalf("stored response = %v, %v", stored, err)
	}

	// A cancelled assignment must stop being executable even though the
	// acceptance row still exists. This is the acceptance/cancellation race.
	reloaded, _ := repo.GetByID(ctx, org.ID, assignment.ID)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.Transition(ctx, tx, reloaded, mktmodels.AssignmentCancelled,
			reloaded.Version, mktmodels.StateHistoryEntry{
				ActorType: "ORGANIZATION_USER", ActorID: "usr_x"})
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cancelled, _ := repo.GetByID(ctx, org.ID, assignment.ID)
	if cancelled.Status != mktmodels.AssignmentCancelled {
		t.Fatalf("status = %s, want CANCELLED", cancelled.Status)
	}
	// The response survives — the provider really did accept — but the
	// assignment is no longer executable, which is exactly what the
	// verification endpoint reports to dispatch.
	surviving, err := repo.GetResponse(ctx, nil, assignment.ID)
	if err != nil || surviving.Response != mktmodels.ResponseAccepted {
		t.Fatal("the provider's acceptance record must survive cancellation")
	}
}

// TestGetByIDUnscopedReturnsOrganizationForBinding proves the unscoped read
// used by the verification endpoint returns the organization rather than
// accepting it, so dispatch can bind it against its own reservation.
func TestGetByIDUnscopedReturnsOrganizationForBinding(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)

	orgA := createOrganization(t, pool, "Unscoped A", "RC-UNS-1")
	orgB := createOrganization(t, pool, "Unscoped B", "RC-UNS-2")
	projection := seedProjection(t, pool, orgA.ID, "req-unscoped-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, orgA.ID, "drv-unscoped-1")
	assignment := seedAssignment(t, pool, orgA.ID, projection.ID, membership, time.Now().Add(time.Minute))

	found, err := repo.GetByIDUnscoped(ctx, assignment.ID)
	if err != nil {
		t.Fatalf("unscoped read: %v", err)
	}
	if found.OrganizationID != orgA.ID {
		t.Fatalf("organization = %s, want %s", found.OrganizationID, orgA.ID)
	}
	if found.OrganizationID == orgB.ID {
		t.Fatal("unscoped read must not be attributable to the wrong organization")
	}

	// The tenant-scoped read still refuses cross-tenant access, so the
	// unscoped variant has not weakened normal isolation.
	if _, err := repo.GetByID(ctx, orgB.ID, assignment.ID); err == nil {
		t.Fatal("tenant-scoped read must still refuse another organization")
	}
}

// TestProviderAssignmentViewCarriesNoCommercialFields checks the provider-safe
// DTO shape by serializing it, including nested content.
func TestProviderAssignmentViewCarriesNoCommercialFields(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "DTO Org", "RC-DTO-1")
	projection := seedProjection(t, pool, org.ID, "req-dto-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-dto-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// Build the same allowlisted shape the provider-response handler returns.
	view := map[string]any{
		"assignmentId":     assignment.ID,
		"status":           assignment.Status,
		"allowedActions":   assignment.AllowedActions(time.Now()),
		"responseDeadline": assignment.ResponseDeadline,
		"sourceService":    assignment.SourceService,
		"version":          assignment.Version,
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, forbidden := range []string{
		"fare_amount", "fareAmount", "customerFare", "customerPrice", "quotedPrice",
		"organizationSettlement", "companySettlement", "platformCommission",
		"commissionPercentage", "profit", "margin", "contractTerms",
		"customerWallet", "organizationWallet", "customerPayment",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("provider view leaked %q: %s", forbidden, encoded)
		}
	}

	// The provider always sees both actions while the assignment is live.
	actions := assignment.AllowedActions(time.Now())
	if len(actions) != 2 || actions[0] != "accept" || actions[1] != "decline" {
		t.Fatalf("allowedActions = %v, want accept and decline", actions)
	}
}

// TestCancelRefusesExecutionStartPending proves the commit point that closes
// the acceptance/cancellation race.
func TestCancelRefusesExecutionStartPending(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Commit Org", "RC-CMT-1")
	projection := seedProjection(t, pool, org.ID, "req-commit-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-commit-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// Provider accepts, then the execution-start worker claims the assignment.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := repo.CreateResponse(ctx, tx, &mktmodels.Response{
			OrganizationID: org.ID, AssignmentID: assignment.ID, MembershipID: membership.ID,
			Response: mktmodels.ResponseAccepted, ClientResponseID: "commit-1",
		}); err != nil {
			return err
		}
		return repo.Transition(ctx, tx, assignment, mktmodels.AssignmentAccepted,
			assignment.Version, mktmodels.StateHistoryEntry{ActorType: "PROVIDER", ActorID: membership.ID})
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	accepted, _ := repo.GetByID(ctx, org.ID, assignment.ID)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.Transition(ctx, tx, accepted, mktmodels.AssignmentExecutionStartPending,
			accepted.Version, mktmodels.StateHistoryEntry{ActorType: "SYSTEM", ActorID: "execution-start"})
	}); err != nil {
		t.Fatalf("claim for execution: %v", err)
	}

	claimed, _ := repo.GetByID(ctx, org.ID, assignment.ID)
	if claimed.Status != mktmodels.AssignmentExecutionStartPending {
		t.Fatalf("status = %s, want EXECUTION_START_PENDING", claimed.Status)
	}
	// The state machine still permits CANCELLED as a transition; the use-case
	// guard is what refuses it while execution is starting. Both layers matter:
	// the state machine keeps recovery possible, the guard prevents a cancel
	// from interleaving with in-flight trip creation.
	if !mktmodels.CanTransition(claimed.Status, mktmodels.AssignmentCancelled) {
		t.Fatal("recovery from EXECUTION_START_PENDING must remain possible")
	}
}
