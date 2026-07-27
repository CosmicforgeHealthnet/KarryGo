package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// seedProjection inserts a marketplace request projection.
func seedProjection(t *testing.T, pool *pgxpool.Pool, organizationID string, requestID string, status string) *mktmodels.RequestProjection {
	t.Helper()
	repo := mktrepos.NewProjectionRepository(pool)
	projection := &mktmodels.RequestProjection{
		OrganizationID:    organizationID,
		SourceService:     mktmodels.SourceDispatch,
		PlatformRequestID: requestID,
		ServiceType:       "dispatch",
		ProjectionStatus:  status,
		SourceStatus:      "broadcasting",
		OfferedAt:         time.Now(),
		SourceVersion:     1,
		FreshnessState:    mktmodels.FreshnessCurrent,
	}
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		return repo.Upsert(context.Background(), tx, projection)
	}); err != nil {
		t.Fatalf("seed projection: %v", err)
	}
	stored, err := repo.GetBySourceRequest(context.Background(), organizationID, mktmodels.SourceDispatch, requestID)
	if err != nil {
		t.Fatalf("reload projection: %v", err)
	}
	// Bring it to the requested status (Upsert always inserts as offered).
	if stored.ProjectionStatus != status {
		if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
			return repo.SetStatus(context.Background(), tx, organizationID, stored.ID, status, stored.Version)
		}); err != nil {
			t.Fatalf("set projection status: %v", err)
		}
		stored, _ = repo.GetByID(context.Background(), organizationID, stored.ID)
	}
	return stored
}

// seedProviderMembership creates an ACTIVE managed provider.
func seedProviderMembership(t *testing.T, pool *pgxpool.Pool, organizationID string, providerID string) *providermodels.Membership {
	t.Helper()
	repo := providerrepos.NewRepository(pool)
	now := time.Now()
	membership := &providermodels.Membership{
		ProviderService:       "driver-dispatch-delivery-service",
		ServiceCategory:       providermodels.CategoryDispatch,
		ProviderID:            &providerID,
		OnboardingSource:      providermodels.SourceSponsored,
		RelationshipType:      providermodels.RelationshipEmployee,
		Status:                providermodels.StatusActive,
		AssociationAcceptedAt: &now,
	}
	if err := repo.CreateMembership(context.Background(), pool, organizationID, membership); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return membership
}

func seedAssignment(t *testing.T, pool *pgxpool.Pool, organizationID string, projectionID string, membership *providermodels.Membership, deadline time.Time) *mktmodels.Assignment {
	t.Helper()
	repo := mktrepos.NewAssignmentRepository(pool)
	assignment := &mktmodels.Assignment{
		RequestProjectionID: projectionID,
		MembershipID:        membership.ID,
		ExecutingProviderID: *membership.ProviderID,
		SourceService:       mktmodels.SourceDispatch,
		AssignedByType:      "ORGANIZATION_USER",
		AssignedByID:        "usr_dispatcher",
		Status:              mktmodels.AssignmentCreated,
		ResponseDeadline:    deadline,
	}
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		if err := repo.Create(context.Background(), tx, organizationID, assignment); err != nil {
			return err
		}
		if err := repo.HoldReservation(context.Background(), tx, organizationID, assignment.ID,
			mktmodels.ResourceProvider, membership.ID); err != nil {
			return err
		}
		return repo.Transition(context.Background(), tx, assignment,
			mktmodels.AssignmentPendingProviderResponse, assignment.Version,
			mktmodels.StateHistoryEntry{ActorType: "ORGANIZATION_USER", ActorID: "usr_dispatcher"})
	}); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	stored, err := repo.GetByID(context.Background(), organizationID, assignment.ID)
	if err != nil {
		t.Fatalf("reload assignment: %v", err)
	}
	return stored
}

func TestAssignmentAlwaysOffersAcceptAndDecline(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Actions Org", "RC-ACT-1")
	projection := seedProjection(t, pool, org.ID, "req-actions-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-actions-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	actions := assignment.AllowedActions(time.Now())
	if len(actions) != 2 || actions[0] != "accept" || actions[1] != "decline" {
		t.Fatalf("allowedActions = %v; a pending assignment must always offer both", actions)
	}

	// Once resolved, no action remains.
	repo := mktrepos.NewAssignmentRepository(pool)
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		return repo.Transition(context.Background(), tx, assignment, mktmodels.AssignmentDeclined,
			assignment.Version, mktmodels.StateHistoryEntry{ActorType: "PROVIDER", ActorID: membership.ID})
	}); err != nil {
		t.Fatalf("decline: %v", err)
	}
	resolved, _ := repo.GetByID(context.Background(), org.ID, assignment.ID)
	if len(resolved.AllowedActions(time.Now())) != 0 {
		t.Fatal("a resolved assignment must offer no actions")
	}

	// An expired deadline also removes the actions — but never by accepting.
	expired := *assignment
	expired.Status = mktmodels.AssignmentPendingProviderResponse
	expired.ResponseDeadline = time.Now().Add(-time.Minute)
	if len(expired.AllowedActions(time.Now())) != 0 {
		t.Fatal("a past deadline must not leave actions available")
	}
}

func TestExactlyOneProviderResponsePerAssignment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Response Org", "RC-RSP-1")
	projection := seedProjection(t, pool, org.ID, "req-response-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-response-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	accept := &mktmodels.Response{
		OrganizationID: org.ID, AssignmentID: assignment.ID, MembershipID: membership.ID,
		Response: mktmodels.ResponseAccepted, ClientResponseID: "client-1",
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.CreateResponse(ctx, tx, accept)
	}); err != nil {
		t.Fatalf("first response: %v", err)
	}

	// A second response of any kind is refused by the database.
	reason := mktmodels.DeclineIllness
	decline := &mktmodels.Response{
		OrganizationID: org.ID, AssignmentID: assignment.ID, MembershipID: membership.ID,
		Response: mktmodels.ResponseDeclined, ReasonCode: &reason, ClientResponseID: "client-2",
	}
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.CreateResponse(ctx, tx, decline)
	})
	if !errors.Is(err, mktrepos.ErrResponseExists) {
		t.Fatalf("second response = %v, want ErrResponseExists", err)
	}

	// The response is immutable.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_assignment_responses SET response = 'DECLINED' WHERE id = $1`, accept.ID); err == nil {
		t.Fatal("assignment responses must be append-only")
	}
}

func TestConcurrentProviderResponsesSingleWinner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Race Org", "RC-RACE-2")
	projection := seedProjection(t, pool, org.ID, "req-race-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-race-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// Six concurrent responses: three accepts and three declines, exactly as
	// a flaky mobile connection would produce.
	var wg sync.WaitGroup
	results := make([]error, 6)
	for index := 0; index < 6; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			results[slot] = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
				locked, err := repo.LockForProvider(ctx, tx, assignment.ID, membership.ID)
				if err != nil {
					return err
				}
				if locked.Status != mktmodels.AssignmentPendingProviderResponse {
					return mktrepos.ErrResponseExists
				}
				response := mktmodels.ResponseAccepted
				target := mktmodels.AssignmentAccepted
				var reason *string
				if slot%2 == 1 {
					response = mktmodels.ResponseDeclined
					target = mktmodels.AssignmentDeclined
					code := mktmodels.DeclineAlreadyEngaged
					reason = &code
				}
				if err := repo.CreateResponse(ctx, tx, &mktmodels.Response{
					OrganizationID: org.ID, AssignmentID: assignment.ID, MembershipID: membership.ID,
					Response: response, ReasonCode: reason,
					ClientResponseID: "race-" + string(rune('a'+slot)),
				}); err != nil {
					return err
				}
				return repo.Transition(ctx, tx, locked, target, locked.Version,
					mktmodels.StateHistoryEntry{ActorType: "PROVIDER", ActorID: membership.ID})
			})
		}(index)
	}
	wg.Wait()

	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent responses produced %d winners, want exactly 1", winners)
	}

	var responseCount int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_assignment_responses WHERE assignment_id = $1`,
		assignment.ID).Scan(&responseCount)
	if responseCount != 1 {
		t.Fatalf("response rows = %d, want exactly 1", responseCount)
	}
}

func TestOneLiveAssignmentPerRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Single Org", "RC-SNG-1")
	projection := seedProjection(t, pool, org.ID, "req-single-1", mktmodels.ProjectionOrganizationAccepted)
	first := seedProviderMembership(t, pool, org.ID, "drv-single-1")
	second := seedProviderMembership(t, pool, org.ID, "drv-single-2")
	seedAssignment(t, pool, org.ID, projection.ID, first, time.Now().Add(time.Minute))

	// A second dispatcher assigning the same request loses.
	duplicate := &mktmodels.Assignment{
		RequestProjectionID: projection.ID,
		MembershipID:        second.ID,
		ExecutingProviderID: *second.ProviderID,
		SourceService:       mktmodels.SourceDispatch,
		AssignedByType:      "ORGANIZATION_USER",
		AssignedByID:        "usr_other",
		Status:              mktmodels.AssignmentCreated,
		ResponseDeadline:    time.Now().Add(time.Minute),
	}
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.Create(ctx, tx, org.ID, duplicate)
	})
	if !errors.Is(err, mktrepos.ErrAssignmentConflict) {
		t.Fatalf("second live assignment = %v, want ErrAssignmentConflict", err)
	}
}

func TestProviderCapacityCannotBeDoubleBooked(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "Capacity Org", "RC-CAP-1")
	firstProjection := seedProjection(t, pool, org.ID, "req-cap-1", mktmodels.ProjectionOrganizationAccepted)
	secondProjection := seedProjection(t, pool, org.ID, "req-cap-2", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-cap-1")

	first := seedAssignment(t, pool, org.ID, firstProjection.ID, membership, time.Now().Add(time.Minute))

	// The same provider cannot hold capacity for a second request.
	second := &mktmodels.Assignment{
		RequestProjectionID: secondProjection.ID,
		MembershipID:        membership.ID,
		ExecutingProviderID: *membership.ProviderID,
		SourceService:       mktmodels.SourceDispatch,
		AssignedByType:      "ORGANIZATION_USER", AssignedByID: "usr_x",
		Status: mktmodels.AssignmentCreated, ResponseDeadline: time.Now().Add(time.Minute),
	}
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := repo.Create(ctx, tx, org.ID, second); err != nil {
			return err
		}
		return repo.HoldReservation(ctx, tx, org.ID, second.ID, mktmodels.ResourceProvider, membership.ID)
	})
	if !errors.Is(err, mktrepos.ErrAssignmentConflict) {
		t.Fatalf("double booking = %v, want ErrAssignmentConflict", err)
	}

	// Releasing the first assignment frees the provider.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.ReleaseReservations(ctx, tx, first.ID)
	}); err != nil {
		t.Fatalf("release: %v", err)
	}
	held, err := repo.CountHeld(ctx, mktmodels.ResourceProvider, membership.ID)
	if err != nil || held != 0 {
		t.Fatalf("held after release = %d, %v", held, err)
	}
}

func TestAssignmentStateMachineRejectsIllegalMoves(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewAssignmentRepository(pool)
	org := createOrganization(t, pool, "FSM Org", "RC-FSM-2")
	projection := seedProjection(t, pool, org.ID, "req-fsm-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-fsm-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// A pending assignment cannot jump straight to completion or execution.
	for _, target := range []string{mktmodels.AssignmentCompleted, mktmodels.AssignmentInProgress,
		mktmodels.AssignmentReadyForExecution} {
		err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return repo.Transition(ctx, tx, assignment, target, assignment.Version,
				mktmodels.StateHistoryEntry{ActorType: "SYSTEM", ActorID: "test"})
		})
		if !errors.Is(err, mktrepos.ErrInvalidTransition) {
			t.Fatalf("PENDING -> %s = %v, want ErrInvalidTransition", target, err)
		}
	}

	// A stale version is refused even for a legal move.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.Transition(ctx, tx, assignment, mktmodels.AssignmentAccepted, 99,
			mktmodels.StateHistoryEntry{ActorType: "PROVIDER", ActorID: membership.ID})
	})
	if !errors.Is(err, concurrency.ErrVersionMismatch) {
		t.Fatalf("stale version = %v, want ErrVersionMismatch", err)
	}

	// History is append-only.
	history, err := repo.ListHistory(ctx, org.ID, assignment.ID)
	if err != nil || len(history) == 0 {
		t.Fatalf("history = %d, %v", len(history), err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM fleet_partner_assignment_state_history WHERE id = $1`, history[0].ID); err == nil {
		t.Fatal("assignment history must be append-only")
	}
}

func TestDeclineReasonCatalogueAndSafetyClassification(t *testing.T) {
	all := []string{
		mktmodels.DeclineIllness, mktmodels.DeclineFatigueOrRest, mktmodels.DeclineShiftEnding,
		mktmodels.DeclineUnsafeVehicle, mktmodels.DeclineVehicleDefect, mktmodels.DeclineExcessiveDistance,
		mktmodels.DeclineLocationMismatch, mktmodels.DeclineServiceMismatch, mktmodels.DeclineCapacityMismatch,
		mktmodels.DeclineCargoOrSafetyConcern, mktmodels.DeclineSecurityConcern,
		mktmodels.DeclinePersonalEmergency, mktmodels.DeclineAlreadyEngaged,
		mktmodels.DeclineDeviceOrNetworkIssue, mktmodels.DeclineOther,
	}
	if len(all) != 15 {
		t.Fatalf("catalogue size = %d, want 15", len(all))
	}
	for _, reason := range all {
		if !mktmodels.ValidDeclineReason(reason) {
			t.Fatalf("%s must be a valid reason", reason)
		}
		if len(mktmodels.FollowupsForReason(reason)) == 0 {
			t.Fatalf("%s must have at least one follow-up action", reason)
		}
	}
	if mktmodels.ValidDeclineReason("MADE_UP_REASON") {
		t.Fatal("unknown reasons must be rejected")
	}

	// Safety and welfare reasons are protected.
	for _, reason := range []string{
		mktmodels.DeclineIllness, mktmodels.DeclineFatigueOrRest, mktmodels.DeclineUnsafeVehicle,
		mktmodels.DeclineVehicleDefect, mktmodels.DeclineCargoOrSafetyConcern,
		mktmodels.DeclineSecurityConcern, mktmodels.DeclinePersonalEmergency,
	} {
		if !mktmodels.SafetyProtected(reason) {
			t.Fatalf("%s must be safety protected", reason)
		}
	}
	// A connectivity problem is never treated as misconduct.
	if mktmodels.SafetyProtected(mktmodels.DeclineDeviceOrNetworkIssue) {
		t.Fatal("device issues are not a safety classification")
	}
	followups := mktmodels.FollowupsForReason(mktmodels.DeclineDeviceOrNetworkIssue)
	if len(followups) != 1 || followups[0] != mktmodels.FollowupSupportTelemetry {
		t.Fatalf("device issue follow-ups = %v; must be telemetry only", followups)
	}
	// OTHER demands an explanation.
	if !mktmodels.RequiresNote(mktmodels.DeclineOther) {
		t.Fatal("OTHER must require a note")
	}
	if mktmodels.RequiresNote(mktmodels.DeclineIllness) {
		t.Fatal("only OTHER requires a note")
	}
}

func TestOfferRecipientDeclineDoesNotCloseOrganizationOffer(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	offers := mktrepos.NewOfferRepository(pool)
	org := createOrganization(t, pool, "Delegated Org", "RC-DEL-1")
	projection := seedProjection(t, pool, org.ID, "req-del-1", mktmodels.ProjectionOffered)

	offer := &mktmodels.Offer{
		RequestProjectionID: projection.ID,
		OperatingMode:       mktmodels.ModeDelegated,
		OfferedAt:           time.Now(),
		ExpiresAt:           time.Now().Add(5 * time.Minute),
	}
	firstProvider := seedProviderMembership(t, pool, org.ID, "drv-del-1")
	secondProvider := seedProviderMembership(t, pool, org.ID, "drv-del-2")

	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := offers.Create(ctx, tx, org.ID, offer); err != nil {
			return err
		}
		for _, membership := range []string{firstProvider.ID, secondProvider.ID} {
			if err := offers.CreateRecipient(ctx, tx, org.ID, &mktmodels.OfferRecipient{
				OfferID: offer.ID, MembershipID: membership, ExpiresAt: offer.ExpiresAt,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed offer: %v", err)
	}

	recipients, err := offers.ListRecipientsForMembership(ctx, org.ID, firstProvider.ID, 10)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("recipients = %d, %v", len(recipients), err)
	}

	// The first provider declines their own recipient row.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return offers.SetRecipientStatus(ctx, tx, recipients[0].ID, mktmodels.RecipientDeclined,
			recipients[0].Version, "client-decline-1", mktmodels.DeclineExcessiveDistance, "too far")
	}); err != nil {
		t.Fatalf("decline recipient: %v", err)
	}

	// The organization's offer stays open, and the other provider still has
	// an actionable offer.
	reloadedOffer, err := offers.GetByID(ctx, org.ID, offer.ID)
	if err != nil || reloadedOffer.Status != mktmodels.OfferOffered {
		t.Fatalf("offer status = %v, %v; one provider's decline must not close the organization offer",
			reloadedOffer.Status, err)
	}
	remaining, err := offers.ListRecipientsForMembership(ctx, org.ID, secondProvider.ID, 10)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("second provider recipients = %d, %v", len(remaining), err)
	}
	if !remaining[0].Actionable(time.Now()) {
		t.Fatal("the second provider's offer must remain actionable")
	}
}

func TestOneLiveOfferPerRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	offers := mktrepos.NewOfferRepository(pool)
	org := createOrganization(t, pool, "Offer Org", "RC-OFF-1")
	projection := seedProjection(t, pool, org.ID, "req-offer-1", mktmodels.ProjectionOffered)

	first := &mktmodels.Offer{RequestProjectionID: projection.ID, OperatingMode: mktmodels.ModeCentralized,
		OfferedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return offers.Create(ctx, tx, org.ID, first)
	}); err != nil {
		t.Fatalf("first offer: %v", err)
	}

	second := &mktmodels.Offer{RequestProjectionID: projection.ID, OperatingMode: mktmodels.ModeCentralized,
		OfferedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return offers.Create(ctx, tx, org.ID, second)
	})
	if err == nil {
		t.Fatal("a second live offer for the same request must be rejected")
	}
}

func TestOrganizationAcceptanceIsUniqueAndImmutable(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	offers := mktrepos.NewOfferRepository(pool)
	org := createOrganization(t, pool, "Acceptance Org", "RC-ACC-1")
	projection := seedProjection(t, pool, org.ID, "req-acc-1", mktmodels.ProjectionOffered)

	offer := &mktmodels.Offer{RequestProjectionID: projection.ID, OperatingMode: mktmodels.ModeHybrid,
		OfferedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	userID := "usr_dispatcher"
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := offers.Create(ctx, tx, org.ID, offer); err != nil {
			return err
		}
		return offers.CreateAcceptance(ctx, tx, org.ID, &mktmodels.Acceptance{
			OfferID: offer.ID, AcceptedByUserID: &userID,
			AcceptanceSource:    mktmodels.AcceptanceDispatcher,
			SourceReservationID: "res-1", SourceIdempotencyKey: "idem-1",
		})
	}); err != nil {
		t.Fatalf("seed acceptance: %v", err)
	}

	// A second acceptance for the same offer is impossible.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return offers.CreateAcceptance(ctx, tx, org.ID, &mktmodels.Acceptance{
			OfferID: offer.ID, AcceptedByUserID: &userID,
			AcceptanceSource:    mktmodels.AcceptanceDispatcher,
			SourceReservationID: "res-2", SourceIdempotencyKey: "idem-2",
		})
	})
	if err == nil {
		t.Fatal("only one organization acceptance per offer may exist")
	}

	// Acceptance is append-only.
	acceptance, err := offers.GetAcceptance(ctx, org.ID, offer.ID)
	if err != nil {
		t.Fatalf("get acceptance: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_organization_acceptances SET source_reservation_id = 'tampered' WHERE id = $1`,
		acceptance.ID); err == nil {
		t.Fatal("organization acceptance must be append-only")
	}

	// The actor constraint forbids a record with neither actor.
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_acceptances
			(id, organization_id, offer_id, acceptance_source, source_reservation_id, source_idempotency_key)
		VALUES ('oac_bad', $1, $2, 'ORGANIZATION_DISPATCHER', 'r', 'i')`, org.ID, offer.ID); err == nil {
		t.Fatal("an acceptance must name exactly one actor")
	}
}

func TestCommandIdempotencyAndConflict(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	commands := mktrepos.NewCommandRepository(pool)
	org := createOrganization(t, pool, "Command Org", "RC-CMD-1")

	operation := &mktmodels.CommandOperation{
		OrganizationID: org.ID, OperationType: mktmodels.CommandReserveRequest,
		SourceService: mktmodels.SourceDispatch, SourceRequestID: "req-cmd-1",
		IdempotencyKey: "idem-cmd-1", RequestFingerprint: "fp-1",
	}
	// Assertions stay outside the transaction: t.Fatal inside a WithinTx
	// closure would Goexit without releasing the pooled connection.
	var stored *mktmodels.CommandOperation
	var firstReplayed bool
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		result, replayed, err := commands.Begin(ctx, tx, operation)
		stored, firstReplayed = result, replayed
		return err
	}); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if firstReplayed {
		t.Fatal("first begin must not be a replay")
	}

	// Same key + same fingerprint replays the original operation.
	var replayResult *mktmodels.CommandOperation
	var replayed bool
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		result, isReplay, err := commands.Begin(ctx, tx, &mktmodels.CommandOperation{
			OrganizationID: org.ID, OperationType: mktmodels.CommandReserveRequest,
			SourceService: mktmodels.SourceDispatch, SourceRequestID: "req-cmd-1",
			IdempotencyKey: "idem-cmd-1", RequestFingerprint: "fp-1",
		})
		replayResult, replayed = result, isReplay
		return err
	}); err != nil {
		t.Fatalf("replay begin: %v", err)
	}
	if !replayed || replayResult.ID != stored.ID {
		t.Fatalf("replay = %v, id = %s want %s", replayed, replayResult.ID, stored.ID)
	}

	// Same key + different fingerprint is a conflict.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		_, _, err := commands.Begin(ctx, tx, &mktmodels.CommandOperation{
			OrganizationID: org.ID, OperationType: mktmodels.CommandReserveRequest,
			SourceService: mktmodels.SourceDispatch, SourceRequestID: "req-cmd-1",
			IdempotencyKey: "idem-cmd-1", RequestFingerprint: "fp-DIFFERENT",
		})
		return err
	})
	if !errors.Is(err, mktrepos.ErrIdempotencyConflict) {
		t.Fatalf("fingerprint conflict = %v, want ErrIdempotencyConflict", err)
	}
}

func TestTripProjectionIgnoresOlderAndDuplicateEvents(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	trips := mktrepos.NewTripRepository(pool)
	org := createOrganization(t, pool, "Trip Org", "RC-TRP-1")

	apply := func(status string, version int64, eventID string) bool {
		var applied bool
		if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			applied, err = trips.UpsertFromSource(ctx, tx, &mktmodels.TripProjection{
				OrganizationID: org.ID, SourceService: mktmodels.SourceDispatch,
				PlatformTripID: "trip-1", ExecutingProviderID: "drv-1",
				TripStatus: status, SourceVersion: version, LastEventID: &eventID,
			})
			return err
		}); err != nil {
			t.Fatalf("apply %s: %v", status, err)
		}
		return applied
	}

	if !apply("assigned", 1, "evt-1") {
		t.Fatal("first event must apply")
	}
	if !apply("in_progress", 2, "evt-2") {
		t.Fatal("next version must apply")
	}
	// An older version must be ignored, not regress the projection.
	if apply("assigned", 1, "evt-1-replay") {
		t.Fatal("an older source version must be ignored")
	}
	// A duplicate of the current version is a no-op.
	if apply("in_progress", 2, "evt-2-duplicate") {
		t.Fatal("a duplicate version must be a no-op")
	}

	stored, err := trips.GetBySourceTrip(ctx, nil, mktmodels.SourceDispatch, "trip-1")
	if err != nil {
		t.Fatalf("load trip: %v", err)
	}
	if stored.TripStatus != "in_progress" || stored.SourceVersion != 2 {
		t.Fatalf("trip = %s v%d; out-of-order events must not regress it",
			stored.TripStatus, stored.SourceVersion)
	}
}

func TestProjectionGapBlocksStateChangingCommands(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	trips := mktrepos.NewTripRepository(pool)
	org := createOrganization(t, pool, "Gap Org", "RC-GAP-1")

	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return trips.RecordGap(ctx, tx, org.ID, mktmodels.SourceDispatch, "REQUEST", "req-gap-1", 5, 9)
	}); err != nil {
		t.Fatalf("record gap: %v", err)
	}

	open, err := trips.HasOpenGap(ctx, mktmodels.SourceDispatch, "REQUEST", "req-gap-1")
	if err != nil || !open {
		t.Fatalf("HasOpenGap = %v, %v", open, err)
	}
	if !mktmodels.BlocksStateChangingCommands(mktmodels.FreshnessGapDetected) {
		t.Fatal("a detected gap must block state-changing commands")
	}
	if mktmodels.BlocksStateChangingCommands(mktmodels.FreshnessCurrent) {
		t.Fatal("a current projection must not be blocked")
	}

	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return trips.ResolveGaps(ctx, tx, mktmodels.SourceDispatch, "REQUEST", "req-gap-1")
	}); err != nil {
		t.Fatalf("resolve gap: %v", err)
	}
	stillOpen, _ := trips.HasOpenGap(ctx, mktmodels.SourceDispatch, "REQUEST", "req-gap-1")
	if stillOpen {
		t.Fatal("the gap must close after reconciliation")
	}
}

func TestMarketplaceTenantIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	projections := mktrepos.NewProjectionRepository(pool)
	assignments := mktrepos.NewAssignmentRepository(pool)

	orgA := createOrganization(t, pool, "Isolation A", "RC-ISO-1")
	orgB := createOrganization(t, pool, "Isolation B", "RC-ISO-2")
	projection := seedProjection(t, pool, orgA.ID, "req-iso-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, orgA.ID, "drv-iso-1")
	assignment := seedAssignment(t, pool, orgA.ID, projection.ID, membership, time.Now().Add(time.Minute))

	// Organization B cannot see A's projection or assignment.
	if _, err := projections.GetByID(ctx, orgB.ID, projection.ID); !errors.Is(err, mktrepos.ErrNotFound) {
		t.Fatalf("cross-tenant projection read = %v, want ErrNotFound", err)
	}
	if _, err := assignments.GetByID(ctx, orgB.ID, assignment.ID); !errors.Is(err, mktrepos.ErrNotFound) {
		t.Fatalf("cross-tenant assignment read = %v, want ErrNotFound", err)
	}
	listed, err := assignments.List(ctx, tenant.Query{OrganizationID: orgB.ID}, "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("organization B sees %d of A's assignments", len(listed))
	}

	// A different provider cannot lock another provider's assignment.
	other := seedProviderMembership(t, pool, orgA.ID, "drv-iso-2")
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		_, lockErr := assignments.LockForProvider(ctx, tx, assignment.ID, other.ID)
		return lockErr
	})
	if !errors.Is(err, mktrepos.ErrNotFound) {
		t.Fatalf("another provider locking the assignment = %v, want ErrNotFound", err)
	}
}
