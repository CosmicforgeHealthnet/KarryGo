package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// membershipReader adapts the providers repository onto the marketplace port.
//
// The composition root supplies the same shape; the fixture uses it so the
// suspension test exercises the real lookup rather than a stand-in verdict.
type membershipReader struct {
	repo *providerrepos.Repository
}

func (r membershipReader) GetMembership(ctx context.Context, organizationID string,
	membershipID string) (*providermodels.Membership, error) {
	membership, err := r.repo.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		return nil, mktusecases.ErrMembershipNotFound
	}
	return membership, nil
}

// ackFixture is one organization with a pending assignment for one provider.
type ackFixture struct {
	pool       *pgxpool.Pool
	service    *mktusecases.Service
	repo       *mktrepos.AssignmentRepository
	orgID      string
	membership *providermodels.Membership
	assignment *mktmodels.Assignment
	projection *mktmodels.RequestProjection
}

func newAckFixture(t *testing.T, name string, registration string, withEligibility bool) *ackFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	projection := seedProjection(t, pool, org.ID, "req-"+registration, mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-"+registration)
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Hour))

	// The durable gate is real in this fixture; only the authorities it
	// consults are stubbed, because they belong to another service.
	seedEligibleOrganization(t, pool, org.ID)
	markMembershipSynced(t, pool, membership.ID)
	service := newIngestionService(pool)
	if withEligibility {
		service = newAcknowledgementService(pool,
			newEligibilityService(pool, healthyAuthority(providerIdentity(membership), "")))
	}
	return &ackFixture{
		pool: pool, service: service, repo: mktrepos.NewAssignmentRepository(pool),
		orgID: org.ID, membership: membership, assignment: assignment, projection: projection,
	}
}

// newAcknowledgementService wires the membership reader the suspension check
// needs.
func newAcknowledgementService(pool *pgxpool.Pool,
	gate *eligibilityusecases.Service) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		DurableEligibility: gate,
		Pool:               pool,
		Projections:        mktrepos.NewProjectionRepository(pool),
		Offers:             mktrepos.NewOfferRepository(pool),
		Assignments:        mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts:    mktrepos.NewExecutionStartRepository(pool),
		Trips:              mktrepos.NewTripRepository(pool),
		Commands:           mktrepos.NewCommandRepository(pool),
		Eligibility: mktusecases.NewEligibilityChecker(
			membershipReader{repo: providerrepos.NewRepository(pool)}, nil, nil),
		Events: fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
	})
}

// providerActor is the identity an execution service relays.
func (f *ackFixture) providerActor() mktusecases.ProviderActor {
	return mktusecases.ProviderActor{
		ExecutingProviderID: f.assignment.ExecutingProviderID,
		SourceService:       mktmodels.SourceDispatch,
	}
}

func (f *ackFixture) acknowledge(key string, expectedVersion int64) (*mktusecases.AcknowledgementResult, error) {
	return f.service.AcknowledgeAssignmentForProvider(context.Background(),
		mktusecases.AcknowledgeAssignmentInput{
			Actor:           f.providerActor(),
			AssignmentID:    f.assignment.ID,
			IdempotencyKey:  key,
			ExpectedVersion: expectedVersion,
			RequestID:       "req-ack",
			CorrelationID:   "corr-ack",
		})
}

func errorCode(t *testing.T, err error) apperrors.Code {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an application error", err)
	}
	return appErr.Code
}

// TestAcknowledgementIsNotAcceptance is the load-bearing test of WP6.1.
//
// Everything else here guards an edge; this one guards the meaning. An
// acknowledgement that could move an assignment into ACCEPTED — or that
// removed a provider's ability to decline — would be a consent forgery, not a
// receipt.
func TestAcknowledgementIsNotAcceptance(t *testing.T) {
	f := newAckFixture(t, "Ack Meaning", "RC-ACK-1", false)

	result, err := f.acknowledge("ack-key-1", f.assignment.Version)
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if result.AcknowledgementState != mktmodels.AcknowledgementAcknowledged {
		t.Fatalf("acknowledgementState = %s", result.AcknowledgementState)
	}

	stored, err := f.repo.GetByID(context.Background(), f.orgID, f.assignment.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != mktmodels.AssignmentPendingProviderResponse {
		t.Fatalf("status = %s; acknowledgement must never move the assignment", stored.Status)
	}
	if stored.AcceptedAt != nil {
		t.Fatal("acknowledgement must not set accepted_at")
	}
	if !stored.Acknowledged() {
		t.Fatal("assignment must be marked acknowledged")
	}

	// No provider response exists: nobody has agreed to anything.
	if _, err := f.repo.GetResponse(context.Background(), nil, f.assignment.ID); err == nil {
		t.Fatal("acknowledgement must not create a provider response")
	}

	// Both actions survive, in the same order, for the same reason they always
	// must: a provider who has merely seen work is still free to refuse it.
	actions := stored.AllowedActions(time.Now())
	if len(actions) != 2 || actions[0] != "accept" || actions[1] != "decline" {
		t.Fatalf("allowedActions = %v; acknowledgement must not narrow them", actions)
	}
}

// TestAcknowledgementLeavesSafetyDeclineIntact proves the specific right that
// an acknowledgement gate could plausibly erode.
func TestAcknowledgementLeavesSafetyDeclineIntact(t *testing.T) {
	f := newAckFixture(t, "Ack Safety", "RC-ACK-2", false)
	if _, err := f.acknowledge("ack-key-safety", 0); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	reloaded, _ := f.repo.GetByID(context.Background(), f.orgID, f.assignment.ID)
	result, err := f.service.DeclineAssignment(context.Background(), mktusecases.DeclineAssignmentInput{
		Actor: mktusecases.ProviderActor{
			MembershipID:        f.membership.ID,
			ExecutingProviderID: f.assignment.ExecutingProviderID,
			SourceService:       mktmodels.SourceDispatch,
			OrganizationID:      f.orgID,
		},
		AssignmentID:     f.assignment.ID,
		ClientResponseID: "decline-after-ack",
		ReasonCode:       mktmodels.DeclineUnsafeVehicle,
		Note:             "brake warning light is on",
		ExpectedVersion:  reloaded.Version,
	})
	if err != nil {
		t.Fatalf("safety decline after acknowledgement must remain available: %v", err)
	}
	if result.Assignment.Status != mktmodels.AssignmentDeclined {
		t.Fatalf("status = %s, want DECLINED", result.Assignment.Status)
	}
}

// TestAcknowledgementIsIdempotent proves a retried acknowledgement replays.
func TestAcknowledgementIsIdempotent(t *testing.T) {
	f := newAckFixture(t, "Ack Idempotent", "RC-ACK-3", false)

	first, err := f.acknowledge("ack-key-retry", 0)
	if err != nil {
		t.Fatalf("first acknowledge: %v", err)
	}
	second, err := f.acknowledge("ack-key-retry", 0)
	if err != nil {
		t.Fatalf("retry must replay, not fail: %v", err)
	}
	if !second.Replayed {
		t.Fatal("second acknowledgement must be reported as a replay")
	}
	if first.Assignment.ID != second.Assignment.ID {
		t.Fatal("replay must return the same assignment")
	}

	// Exactly one acknowledgement row exists.
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_assignment_acknowledgements
		WHERE assignment_id = $1`, f.assignment.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("acknowledgement rows = %d, want exactly 1", count)
	}
}

// TestAcknowledgementRejectsReusedKeyWithDifferentRequest proves a used
// idempotency key cannot be turned into a blank cheque.
func TestAcknowledgementRejectsReusedKeyWithDifferentRequest(t *testing.T) {
	f := newAckFixture(t, "Ack Fingerprint", "RC-ACK-4", false)

	if _, err := f.acknowledge("ack-key-shared", 2); err != nil {
		t.Fatalf("first acknowledge: %v", err)
	}
	// Same key, different expected version: a different command wearing a used
	// key, which must be refused rather than answered with the old result.
	_, err := f.acknowledge("ack-key-shared", 99)
	if err == nil {
		t.Fatal("a reused key with different inputs must be rejected")
	}
	if code := errorCode(t, err); code != errorcodes.IdempotencyConflictCode {
		t.Fatalf("code = %s, want %s", code, errorcodes.IdempotencyConflictCode)
	}
}

// TestAcknowledgementRejectsKeyReusedAcrossAssignments proves the key cannot
// acknowledge a second assignment.
func TestAcknowledgementRejectsKeyReusedAcrossAssignments(t *testing.T) {
	f := newAckFixture(t, "Ack Key Scope", "RC-ACK-5", false)
	if _, err := f.acknowledge("ack-key-scoped", 0); err != nil {
		t.Fatalf("first acknowledge: %v", err)
	}

	// Resolve the first assignment so the provider's capacity hold is released;
	// a provider can only be held by one live assignment at a time.
	if err := database.WithinTx(context.Background(), f.pool, func(tx pgx.Tx) error {
		reloaded, err := f.repo.GetByID(context.Background(), f.orgID, f.assignment.ID)
		if err != nil {
			return err
		}
		if err := f.repo.Transition(context.Background(), tx, reloaded,
			mktmodels.AssignmentCancelled, reloaded.Version,
			mktmodels.StateHistoryEntry{ActorType: "ORGANIZATION_USER", ActorID: "usr_x"}); err != nil {
			return err
		}
		return f.repo.ReleaseReservations(context.Background(), tx, reloaded.ID)
	}); err != nil {
		t.Fatalf("resolve first assignment: %v", err)
	}

	// A second assignment for the same provider, reached with the same key.
	second := seedProjection(t, f.pool, f.orgID, "req-RC-ACK-5-b", mktmodels.ProjectionOrganizationAccepted)
	other := seedAssignment(t, f.pool, f.orgID, second.ID, f.membership, time.Now().Add(time.Hour))

	_, err := f.service.AcknowledgeAssignmentForProvider(context.Background(),
		mktusecases.AcknowledgeAssignmentInput{
			Actor:          f.providerActor(),
			AssignmentID:   other.ID,
			IdempotencyKey: "ack-key-scoped",
		})
	if err == nil {
		t.Fatal("an idempotency key must not acknowledge a second assignment")
	}
	if code := errorCode(t, err); code != errorcodes.IdempotencyConflictCode {
		t.Fatalf("code = %s, want %s", code, errorcodes.IdempotencyConflictCode)
	}
}

// TestAcknowledgementRefusesForeignProvider is the IDOR guard: a valid caller
// signature must not let one provider acknowledge another's work.
func TestAcknowledgementRefusesForeignProvider(t *testing.T) {
	f := newAckFixture(t, "Ack IDOR", "RC-ACK-6", false)

	_, err := f.service.AcknowledgeAssignmentForProvider(context.Background(),
		mktusecases.AcknowledgeAssignmentInput{
			Actor: mktusecases.ProviderActor{
				ExecutingProviderID: "drv-someone-else",
				SourceService:       mktmodels.SourceDispatch,
			},
			AssignmentID:   f.assignment.ID,
			IdempotencyKey: "ack-key-idor",
		})
	if err == nil {
		t.Fatal("a provider must not acknowledge another provider's assignment")
	}
	if code := errorCode(t, err); code != errorcodes.AssignmentNotOwnedByProvider {
		t.Fatalf("code = %s, want %s", code, errorcodes.AssignmentNotOwnedByProvider)
	}

	// And nothing was written.
	stored, _ := f.repo.GetByID(context.Background(), f.orgID, f.assignment.ID)
	if stored.Acknowledged() {
		t.Fatal("a refused acknowledgement must leave no trace")
	}
}

// TestAcknowledgementRefusesForeignSourceService proves one execution service
// cannot answer for another's provider.
func TestAcknowledgementRefusesForeignSourceService(t *testing.T) {
	f := newAckFixture(t, "Ack Cross Service", "RC-ACK-7", false)

	_, err := f.service.AcknowledgeAssignmentForProvider(context.Background(),
		mktusecases.AcknowledgeAssignmentInput{
			Actor: mktusecases.ProviderActor{
				ExecutingProviderID: f.assignment.ExecutingProviderID,
				SourceService:       mktmodels.SourceTaxi,
			},
			AssignmentID:   f.assignment.ID,
			IdempotencyKey: "ack-key-cross",
		})
	if err == nil {
		t.Fatal("a taxi caller must not acknowledge a dispatch assignment")
	}
}

// TestAcknowledgementRejectsStaleExpectedVersion proves optimistic
// concurrency is enforced.
func TestAcknowledgementRejectsStaleExpectedVersion(t *testing.T) {
	f := newAckFixture(t, "Ack Stale", "RC-ACK-8", false)

	_, err := f.acknowledge("ack-key-stale", f.assignment.Version+5)
	if err == nil {
		t.Fatal("a stale expected version must be rejected")
	}
	if code := errorCode(t, err); code != errorcodes.ExpectedVersionMismatch {
		t.Fatalf("code = %s, want %s", code, errorcodes.ExpectedVersionMismatch)
	}
}

// TestAcknowledgementRefusesResolvedAssignment proves there is nothing to
// receive once the assignment is over.
func TestAcknowledgementRefusesResolvedAssignment(t *testing.T) {
	f := newAckFixture(t, "Ack Resolved", "RC-ACK-9", false)

	if err := database.WithinTx(context.Background(), f.pool, func(tx pgx.Tx) error {
		return f.repo.Transition(context.Background(), tx, f.assignment,
			mktmodels.AssignmentCancelled, f.assignment.Version,
			mktmodels.StateHistoryEntry{ActorType: "ORGANIZATION_USER", ActorID: "usr_x"})
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	_, err := f.acknowledge("ack-key-resolved", 0)
	if err == nil {
		t.Fatal("a cancelled assignment must not be acknowledgeable")
	}
	if code := errorCode(t, err); code != errorcodes.AssignmentNotAcknowledgeable {
		t.Fatalf("code = %s, want %s", code, errorcodes.AssignmentNotAcknowledgeable)
	}
}

// TestAcknowledgementRefusesSuspendedProvider proves a suspended membership
// cannot record a receipt.
func TestAcknowledgementRefusesSuspendedProvider(t *testing.T) {
	f := newAckFixture(t, "Ack Suspended", "RC-ACK-10", true)

	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_organization_provider_memberships
		SET status = 'SUSPENDED' WHERE id = $1`, f.membership.ID); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	_, err := f.acknowledge("ack-key-suspended", 0)
	if err == nil {
		t.Fatal("a suspended provider must not acknowledge")
	}
	if code := errorCode(t, err); code != errorcodes.ActorNotAuthorized {
		t.Fatalf("code = %s, want %s", code, errorcodes.ActorNotAuthorized)
	}
}

// TestConcurrentAcknowledgementProducesOneRecord proves two simultaneous
// acknowledgements resolve to exactly one row and one winner.
func TestConcurrentAcknowledgementProducesOneRecord(t *testing.T) {
	f := newAckFixture(t, "Ack Race", "RC-ACK-11", false)

	var wg sync.WaitGroup
	results := make([]error, 2)
	start := make(chan struct{})
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			<-start
			_, err := f.service.AcknowledgeAssignmentForProvider(context.Background(),
				mktusecases.AcknowledgeAssignmentInput{
					Actor:          f.providerActor(),
					AssignmentID:   f.assignment.ID,
					IdempotencyKey: "ack-race-key-" + string(rune('a'+slot)),
				})
			results[slot] = err
		}(index)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1 (errors: %v)", successes, results)
	}

	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_assignment_acknowledgements
		WHERE assignment_id = $1`, f.assignment.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("acknowledgement rows = %d, want exactly 1", count)
	}
}

// TestAcknowledgementIsAppendOnly proves the record cannot be rewritten.
func TestAcknowledgementIsAppendOnly(t *testing.T) {
	f := newAckFixture(t, "Ack Immutable", "RC-ACK-12", false)
	if _, err := f.acknowledge("ack-key-immutable", 0); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	if _, err := f.pool.Exec(context.Background(), `
		UPDATE fleet_partner_assignment_acknowledgements
		SET actor_id = 'someone-else' WHERE assignment_id = $1`, f.assignment.ID); err == nil {
		t.Fatal("acknowledgement records must be append-only")
	}
	if _, err := f.pool.Exec(context.Background(), `
		DELETE FROM fleet_partner_assignment_acknowledgements
		WHERE assignment_id = $1`, f.assignment.ID); err == nil {
		t.Fatal("acknowledgement records must not be deletable")
	}
}

// TestAcknowledgementWritesHistoryAuditAndOutboxAtomically proves the four
// writes commit together.
func TestAcknowledgementWritesHistoryAuditAndOutboxAtomically(t *testing.T) {
	f := newAckFixture(t, "Ack Atomic", "RC-ACK-13", false)
	if _, err := f.acknowledge("ack-key-atomic", 0); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	ctx := context.Background()

	var history int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_assignment_state_history
		WHERE assignment_id = $1 AND from_status = to_status`, f.assignment.ID).Scan(&history); err != nil {
		t.Fatalf("history: %v", err)
	}
	if history != 1 {
		t.Fatalf("acknowledgement history rows = %d, want 1", history)
	}

	var audits int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_audit_records
		WHERE resource_id = $1 AND action = 'assignment.acknowledged'`,
		f.assignment.ID).Scan(&audits); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if audits != 1 {
		t.Fatalf("audit rows = %d, want 1", audits)
	}

	var events int
	if err := f.pool.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_outbox
		WHERE aggregate_id = $1 AND event_type = $2`,
		f.assignment.ID, mktusecases.EventAssignmentAcknowledged).Scan(&events); err != nil {
		t.Fatalf("outbox: %v", err)
	}
	if events != 1 {
		t.Fatalf("outbox rows = %d, want 1", events)
	}
}

// TestDeparturePolicyBlocksUnacknowledgedExecutionStart proves the opt-in
// policy, and that it withholds departure without discarding the acceptance.
func TestDeparturePolicyBlocksUnacknowledgedExecutionStart(t *testing.T) {
	f := newAckFixture(t, "Ack Policy", "RC-ACK-14", false)
	ctx := context.Background()

	if _, err := f.pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_settings
			(organization_id, require_assignment_acknowledgement)
		VALUES ($1, true)
		ON CONFLICT (organization_id)
		DO UPDATE SET require_assignment_acknowledgement = true`, f.orgID); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	required, err := f.repo.RequiresAcknowledgement(ctx, nil, f.orgID)
	if err != nil || !required {
		t.Fatalf("policy = %v, %v; want true", required, err)
	}

	// An organization that never configured settings is not subject to the
	// policy — the default must not block anyone.
	otherOrg := createOrganization(t, f.pool, "Ack No Policy", "RC-ACK-14B")
	defaulted, err := f.repo.RequiresAcknowledgement(ctx, nil, otherOrg.ID)
	if err != nil || defaulted {
		t.Fatalf("unset policy = %v, %v; want false", defaulted, err)
	}

	// Accepting still works: the policy gates departure, never the decision.
	result, err := f.service.AcceptAssignment(ctx, mktusecases.AcceptAssignmentInput{
		Actor: mktusecases.ProviderActor{
			MembershipID:        f.membership.ID,
			ExecutingProviderID: f.assignment.ExecutingProviderID,
			SourceService:       mktmodels.SourceDispatch,
			OrganizationID:      f.orgID,
		},
		AssignmentID:     f.assignment.ID,
		ClientResponseID: "accept-under-policy",
		ExpectedVersion:  f.assignment.Version,
	})
	if err != nil {
		t.Fatalf("accept must remain available under the policy: %v", err)
	}
	if result.Assignment.Status != mktmodels.AssignmentAccepted {
		t.Fatalf("status = %s, want ACCEPTED", result.Assignment.Status)
	}
	// Departure was withheld rather than attempted.
	if result.ExecutionStartStatus != mktmodels.ExecutionNotRequested {
		t.Fatalf("executionStartStatus = %s; departure must be withheld until acknowledged",
			result.ExecutionStartStatus)
	}

	// Acknowledging afterwards is still possible — ACCEPTED is acknowledgeable
	// precisely so a provider can satisfy this policy after deciding.
	if _, err := f.acknowledge("ack-key-policy", 0); err != nil {
		t.Fatalf("acknowledge after acceptance: %v", err)
	}
	stored, _ := f.repo.GetByID(ctx, f.orgID, f.assignment.ID)
	if !stored.Acknowledged() {
		t.Fatal("assignment must be acknowledged")
	}
	if stored.Status != mktmodels.AssignmentAccepted {
		t.Fatalf("status = %s; acknowledgement must not disturb the acceptance", stored.Status)
	}
}
