package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
)

// newOfferService builds the marketplace service against the test database.
func newOfferService(t *testing.T, pool *pgxpool.Pool) *mktusecases.Service {
	t.Helper()
	return mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Events:          events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
}

// seedOfferWithRecipients creates a live offer addressed to the given providers.
func seedOfferWithRecipients(t *testing.T, pool *pgxpool.Pool, organizationID string,
	projectionID string, memberships []*providermodels.Membership, expiresIn time.Duration) (
	*mktmodels.Offer, []mktmodels.OfferRecipient) {
	t.Helper()
	offers := mktrepos.NewOfferRepository(pool)
	offer := &mktmodels.Offer{
		RequestProjectionID: projectionID,
		OperatingMode:       mktmodels.ModeDelegated,
		OfferedAt:           time.Now(),
		ExpiresAt:           time.Now().Add(expiresIn),
	}
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		if err := offers.Create(context.Background(), tx, organizationID, offer); err != nil {
			return err
		}
		for _, membership := range memberships {
			if err := offers.CreateRecipient(context.Background(), tx, organizationID,
				&mktmodels.OfferRecipient{
					OfferID: offer.ID, MembershipID: membership.ID, ExpiresAt: offer.ExpiresAt,
				}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed offer: %v", err)
	}

	var recipients []mktmodels.OfferRecipient
	for _, membership := range memberships {
		found, err := offers.ListRecipientsForMembership(context.Background(), organizationID,
			membership.ID, 10)
		if err != nil || len(found) == 0 {
			t.Fatalf("load recipient for %s: %v", membership.ID, err)
		}
		recipients = append(recipients, found[0])
	}
	return offer, recipients
}

func dispatchActor(providerID string) mktusecases.ProviderActor {
	return mktusecases.ProviderActor{
		ExecutingProviderID: providerID,
		SourceService:       mktmodels.SourceDispatch,
	}
}

func TestOfferAcceptanceCommitsTheWholeDelegatedConsequence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Offer Accept Org", "RC-OFA-1")
	projection := seedProjection(t, pool, org.ID, "req-ofa-1", mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "drv-ofa-1")
	offer, recipients := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)

	result, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor:                 dispatchActor(*provider.ProviderID),
		OfferID:               offer.ID,
		ClientResponseID:      "client-ofa-1",
		ExpectedVersion:       recipients[0].Version,
		DispatchReservationID: "res-ofa-1",
	})
	if err != nil {
		t.Fatalf("accept offer: %v", err)
	}
	if result.Assignment == nil {
		t.Fatal("acceptance produced no assignment")
	}

	// The assignment must NOT be left pending: the provider already decided,
	// and a pending assignment would invite asking them to accept twice.
	if result.Assignment.Status != mktmodels.AssignmentAccepted {
		t.Fatalf("assignment status = %s, want ACCEPTED", result.Assignment.Status)
	}
	if len(result.AllowedActions) != 0 {
		t.Fatalf("allowedActions = %v; an answered offer leaves no action", result.AllowedActions)
	}

	// Organization acceptance, attributed to the provider not a dispatcher.
	acceptance, err := mktrepos.NewOfferRepository(pool).GetAcceptance(ctx, org.ID, offer.ID)
	if err != nil {
		t.Fatalf("organization acceptance missing: %v", err)
	}
	if acceptance.AcceptanceSource != mktmodels.AcceptanceProviderDirect {
		t.Fatalf("acceptance source = %s, want PROVIDER_DIRECT", acceptance.AcceptanceSource)
	}
	if acceptance.AcceptedByMembershipID == nil ||
		*acceptance.AcceptedByMembershipID != provider.ID {
		t.Fatal("acceptance must name the accepting provider membership")
	}

	// Immutable provider response.
	response, err := mktrepos.NewAssignmentRepository(pool).GetResponse(ctx, nil, result.Assignment.ID)
	if err != nil || response.Response != mktmodels.ResponseAccepted {
		t.Fatalf("provider response = %v, %v", response, err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_assignment_responses SET response = 'DECLINED' WHERE id = $1`,
		response.ID); err == nil {
		t.Fatal("provider responses must be immutable")
	}

	// Exactly one execution-start operation.
	executions := mktrepos.NewExecutionStartRepository(pool)
	operation, err := executions.GetByAssignment(ctx, result.Assignment.ID)
	if err != nil {
		t.Fatalf("execution-start operation missing: %v", err)
	}
	if operation.ProviderResponseID != response.ID {
		t.Fatal("execution work must reference the provider's actual response")
	}
	var executionCount int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_execution_start_operations`).Scan(&executionCount)
	if executionCount != 1 {
		t.Fatalf("execution-start rows = %d, want exactly 1", executionCount)
	}

	// Audit and outbox.
	var auditCount, outboxCount int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_audit_records WHERE action = 'offer.provider_accepted'`).
		Scan(&auditCount)
	if auditCount != 1 {
		t.Fatalf("audit records = %d, want 1", auditCount)
	}
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_outbox WHERE event_type = 'organization.offer.provider_accepted.v1'`).
		Scan(&outboxCount)
	if outboxCount != 1 {
		t.Fatalf("outbox records = %d, want 1", outboxCount)
	}
}

func TestOfferOwnershipIsEnforcedBySQLBinding(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Owner Org", "RC-OWN-1")
	projection := seedProjection(t, pool, org.ID, "req-own-1", mktmodels.ProjectionOffered)
	invited := seedProviderMembership(t, pool, org.ID, "drv-own-invited")
	outsider := seedProviderMembership(t, pool, org.ID, "drv-own-outsider")
	// Only the invited provider is a recipient.
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{invited}, 5*time.Minute)

	// A different provider cannot accept an offer that was never made to them.
	if _, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*outsider.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-own-1",
	}); err == nil {
		t.Fatal("a provider accepted an offer that was not made to them")
	}
	// Nor decline it.
	if _, err := service.DeclineOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*outsider.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-own-2", ReasonCode: mktmodels.DeclineAlreadyEngaged,
	}); err == nil {
		t.Fatal("a provider declined an offer that was not made to them")
	}

	// The invited provider's recipient row is untouched by those attempts.
	offers := mktrepos.NewOfferRepository(pool)
	recipients, _ := offers.ListRecipientsForMembership(ctx, org.ID, invited.ID, 10)
	if len(recipients) != 1 || !recipients[0].Actionable(time.Now()) {
		t.Fatal("the invited provider's offer must remain actionable")
	}
}

func TestOneProviderDeclineLeavesOfferOpenForOthers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Decline Org", "RC-ODL-1")
	projection := seedProjection(t, pool, org.ID, "req-odl-1", mktmodels.ProjectionOffered)
	first := seedProviderMembership(t, pool, org.ID, "drv-odl-1")
	second := seedProviderMembership(t, pool, org.ID, "drv-odl-2")
	offer, recipients := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{first, second}, 5*time.Minute)

	result, err := service.DeclineOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*first.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-odl-1", ExpectedVersion: recipients[0].Version,
		ReasonCode: mktmodels.DeclineExcessiveDistance,
	})
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if result.Status != mktmodels.RecipientDeclined {
		t.Fatalf("status = %s, want DECLINED", result.Status)
	}

	// The ORGANIZATION's offer is untouched — one provider's "no" is not the
	// organization's "no".
	offers := mktrepos.NewOfferRepository(pool)
	reloaded, err := offers.GetByID(ctx, org.ID, offer.ID)
	if err != nil || reloaded.Status != mktmodels.OfferOffered {
		t.Fatalf("offer status = %v, %v; must remain open", reloaded.Status, err)
	}
	// And the second provider can still act.
	remaining, _ := offers.ListRecipientsForMembership(ctx, org.ID, second.ID, 10)
	if len(remaining) != 1 || !remaining[0].Actionable(time.Now()) {
		t.Fatal("the second provider's offer must remain actionable")
	}
	// The second provider can in fact accept it.
	if _, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*second.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-odl-2",
	}); err != nil {
		t.Fatalf("second provider could not accept after the first declined: %v", err)
	}
}

func TestOfferDeclineValidatesReasonAndFlagsSafety(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Reason Org", "RC-ORS-1")
	projection := seedProjection(t, pool, org.ID, "req-ors-1", mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "drv-ors-1")
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)

	base := mktusecases.OfferResponseInput{
		Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID,
	}

	// A missing reason is refused.
	missing := base
	missing.ClientResponseID = "c1"
	if _, err := service.DeclineOffer(ctx, missing); err == nil {
		t.Fatal("a decline without a reason was accepted")
	}
	// An unknown reason is refused.
	unknown := base
	unknown.ClientResponseID, unknown.ReasonCode = "c2", "MADE_UP"
	if _, err := service.DeclineOffer(ctx, unknown); err == nil {
		t.Fatal("an unknown decline reason was accepted")
	}
	// OTHER requires an explanation.
	other := base
	other.ClientResponseID, other.ReasonCode = "c3", mktmodels.DeclineOther
	if _, err := service.DeclineOffer(ctx, other); err == nil {
		t.Fatal("OTHER without a note was accepted")
	}

	// A safety reason succeeds and is flagged as protected in the event.
	safety := base
	safety.ClientResponseID, safety.ReasonCode = "c4", mktmodels.DeclineUnsafeVehicle
	if _, err := service.DeclineOffer(ctx, safety); err != nil {
		t.Fatalf("safety decline: %v", err)
	}
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM fleet_partner_outbox
		WHERE event_type = 'organization.offer.provider_declined.v1'`).Scan(&payload); err != nil {
		t.Fatalf("decline event missing: %v", err)
	}
	// The publisher wraps domain payloads in a corporate envelope, so the
	// domain fields live one level down.
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	decoded, ok := envelope["payload"].(map[string]any)
	if !ok {
		decoded = envelope
	}
	if decoded["safetyProtected"] != true {
		t.Fatalf("safetyProtected = %v, want true for UNSAFE_VEHICLE", decoded["safetyProtected"])
	}
	// A safety decline must not create any punitive record.
	for _, table := range []string{"fleet_partner_provider_membership_history"} {
		var punitive int
		_ = pool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE to_status IN ('SUSPENDED','REMOVED')`).Scan(&punitive)
		if punitive != 0 {
			t.Fatalf("a safety decline created %d punitive %s rows", punitive, table)
		}
	}
}

func TestOfferResponseReplaysOnSameClientResponseID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Replay Org", "RC-ORP-1")
	projection := seedProjection(t, pool, org.ID, "req-orp-1", mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "drv-orp-1")
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)

	input := mktusecases.OfferResponseInput{
		Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-replay-1",
	}
	first, err := service.AcceptOffer(ctx, input)
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}

	// A redelivered relay replays the original outcome instead of colliding
	// with its own earlier delivery.
	second, err := service.AcceptOffer(ctx, input)
	if err != nil {
		t.Fatalf("replay accept: %v", err)
	}
	if !second.Replayed {
		t.Fatal("the second delivery must be reported as a replay")
	}
	if second.RecipientID != first.RecipientID {
		t.Fatal("replay returned a different recipient")
	}

	// Exactly one of everything.
	var assignments, responses, executions int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_assignments`).Scan(&assignments)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_assignment_responses`).Scan(&responses)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_execution_start_operations`).Scan(&executions)
	if assignments != 1 || responses != 1 || executions != 1 {
		t.Fatalf("replay duplicated state: assignments=%d responses=%d executions=%d",
			assignments, responses, executions)
	}
}

func TestOfferRejectsStaleVersionAndExpiredOffer(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "Stale Org", "RC-OST-1")
	projection := seedProjection(t, pool, org.ID, "req-ost-1", mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "drv-ost-1")
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)

	// Stale expected version.
	if _, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-stale", ExpectedVersion: 99,
	}); err == nil {
		t.Fatal("a stale expected version was accepted")
	}

	// An expired offer is no longer actionable.
	expiredOrg := createOrganization(t, pool, "Expired Org", "RC-OEX-1")
	expiredProjection := seedProjection(t, pool, expiredOrg.ID, "req-oex-1", mktmodels.ProjectionOffered)
	expiredProvider := seedProviderMembership(t, pool, expiredOrg.ID, "drv-oex-1")
	expiredOffer, _ := seedOfferWithRecipients(t, pool, expiredOrg.ID, expiredProjection.ID,
		[]*providermodels.Membership{expiredProvider}, -time.Minute)

	if _, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*expiredProvider.ProviderID), OfferID: expiredOffer.ID,
		ClientResponseID: "client-expired",
	}); err == nil {
		t.Fatal("an expired offer was accepted")
	}
}

func TestConcurrentOfferAcceptanceProducesOneWinner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Race Offer Org", "RC-ORC-1")
	projection := seedProjection(t, pool, org.ID, "req-orc-1", mktmodels.ProjectionOffered)

	var memberships []*providermodels.Membership
	for i := 0; i < 4; i++ {
		memberships = append(memberships,
			seedProviderMembership(t, pool, org.ID, "drv-orc-"+string(rune('a'+i))))
	}
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID, memberships, 5*time.Minute)

	var wg sync.WaitGroup
	errs := make([]error, len(memberships))
	for index := range memberships {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			service := newOfferService(t, pool)
			_, errs[slot] = service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
				Actor:            dispatchActor(*memberships[slot].ProviderID),
				OfferID:          offer.ID,
				ClientResponseID: "client-race-" + string(rune('a'+slot)),
			})
		}(index)
	}
	wg.Wait()

	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (errs=%v)", winners, errs)
	}

	// One assignment, one response, one execution operation, one acceptance.
	var assignments, responses, executions, acceptances int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_assignments`).Scan(&assignments)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_assignment_responses`).Scan(&responses)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_execution_start_operations`).Scan(&executions)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_organization_acceptances`).Scan(&acceptances)
	if assignments != 1 || responses != 1 || executions != 1 || acceptances != 1 {
		t.Fatalf("race produced assignments=%d responses=%d executions=%d acceptances=%d",
			assignments, responses, executions, acceptances)
	}
}

func TestProviderOfferViewCarriesNoCommercialFields(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newOfferService(t, pool)
	org := createOrganization(t, pool, "DTO Offer Org", "RC-ODT-1")
	projection := seedProjection(t, pool, org.ID, "req-odt-1", mktmodels.ProjectionOffered)
	provider := seedProviderMembership(t, pool, org.ID, "drv-odt-1")
	offer, _ := seedOfferWithRecipients(t, pool, org.ID, projection.ID,
		[]*providermodels.Membership{provider}, 5*time.Minute)

	result, err := service.AcceptOffer(ctx, mktusecases.OfferResponseInput{
		Actor: dispatchActor(*provider.ProviderID), OfferID: offer.ID,
		ClientResponseID: "client-dto-1",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	// Serialize the same allowlisted shape the handler returns.
	view := map[string]any{
		"offerId":        result.OfferID,
		"recipientId":    result.RecipientID,
		"status":         result.Status,
		"assignmentId":   result.Assignment.ID,
		"allowedActions": result.AllowedActions,
		"replayed":       result.Replayed,
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
			t.Fatalf("provider offer view leaked %q: %s", forbidden, encoded)
		}
	}
}
