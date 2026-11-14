package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Sections 11-12 end to end inside Fleet: a source opportunity becomes one
// projection + organization offer per eligible organization, source request
// events keep every organization's copy honest, offers expire, and the
// canonical trip drives the assignment to IN_PROGRESS and COMPLETED.
//
// Every event here is ingested exactly as the HTTP endpoint delivers it: the
// sourceService is the authenticated SERVICE NAME, not the stored category.

const s1112Dispatch = "driver-dispatch-delivery-service"

// forbiddenCommercialFields must never reach an organization projection or a
// provider-facing view (spec 12.4).
var forbiddenCommercialFields = []string{
	"fare", "price", "commission", "margin", "settlementAmount", "contract",
	"customerPhone", "contactPhone", "profit",
}

func newOpportunityService(pool *pgxpool.Pool, enabled bool) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Trips:           mktrepos.NewTripRepository(pool),
		Commands:        mktrepos.NewCommandRepository(pool),
		OperatingModes:  orgrepos.NewSettingsRepository(pool),
		Eligibility: mktusecases.NewEligibilityChecker(
			app.NewMembershipReader(providerrepos.NewRepository(pool)), nil, nil),
		DurableEligibility: newEligibilityService(pool, healthyAuthority("", "")),
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:            observability.NewMetrics(),
		Opportunities:      mktusecases.OpportunityConfig{Enabled: enabled},
		Flags: mktusecases.Flags{ProjectionEnabled: true, OrganizationAcceptEnabled: true,
			AssignmentEnabled: true},
	})
}

func setOperatingMode(t *testing.T, pool *pgxpool.Pool, organizationID string, mode string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_organization_settings (organization_id, operating_mode)
		VALUES ($1, $2)
		ON CONFLICT (organization_id) DO UPDATE SET operating_mode = EXCLUDED.operating_mode`,
		organizationID, mode); err != nil {
		t.Fatalf("set operating mode: %v", err)
	}
}

// sourceEvent builds an envelope exactly as a producer delivers it.
func sourceEvent(t *testing.T, sourceService string, eventID string, eventType string,
	aggregateType string, aggregateID string, version int64,
	payload map[string]any) ([]byte, mktusecases.InboundEvent) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	event := mktusecases.InboundEvent{
		EventID: eventID, EventType: eventType, SchemaVersion: 1,
		SourceService: sourceService, AggregateType: aggregateType, AggregateID: aggregateID,
		AggregateVersion: version, OccurredAt: time.Now().UTC(),
		CorrelationID: "corr-" + eventID, PayloadHash: events.HashPayload(encoded), Payload: payload,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return raw, event
}

func ingest(t *testing.T, service *mktusecases.Service, raw []byte,
	event mktusecases.InboundEvent) mktusecases.IngestResult {
	t.Helper()
	result, err := service.IngestEvent(context.Background(), raw, event)
	if err != nil {
		t.Fatalf("ingest %s: %v", event.EventType, err)
	}
	return result
}

func offeredPayload(requestID string, expiresIn time.Duration) map[string]any {
	return map[string]any{
		"sourceService": s1112Dispatch,
		"requestId":     requestID,
		"bookingId":     "bkg-" + requestID,
		"status":        "broadcasting",
		"serviceType":   "dispatch",
		"attemptNumber": 1,
		"expiresAt":     time.Now().Add(expiresIn).UTC().Format(time.RFC3339),
		// Commercial and personal data a careless producer might add. None of
		// it may land in Fleet.
		"fareAmount":       250000,
		"commissionAmount": 37500,
		"customerPhone":    "+2348000000000",
		"pickupSummary":    map[string]any{"area": "Ikeja", "contactPhone": "+2348000000001", "fare": 5},
		"operationalRequirements": map[string]any{
			"packageSize": "small", "contractPrice": 99,
		},
	}
}

// opportunityWorld seeds three organizations: A (CENTRALIZED, one provider),
// B (DELEGATED, two providers) and C (eligible-looking provider, but the
// organization itself is SUSPENDED) plus D (ACTIVE, provider suspended).
type opportunityWorld struct {
	orgA, orgB, orgC, orgD string
	providerA              *providermodels.Membership
	providersB             []*providermodels.Membership
}

func seedOpportunityWorld(t *testing.T, pool *pgxpool.Pool, prefix string) opportunityWorld {
	t.Helper()
	ctx := context.Background()
	w := opportunityWorld{}
	a := createOrganization(t, pool, prefix+" Org A", "RC-"+prefix+"-A")
	b := createOrganization(t, pool, prefix+" Org B", "RC-"+prefix+"-B")
	c := createOrganization(t, pool, prefix+" Org C", "RC-"+prefix+"-C")
	d := createOrganization(t, pool, prefix+" Org D", "RC-"+prefix+"-D")
	w.orgA, w.orgB, w.orgC, w.orgD = a.ID, b.ID, c.ID, d.ID
	w.providerA = seedProviderMembership(t, pool, a.ID, prefix+"-drv-a1")
	w.providersB = []*providermodels.Membership{
		seedProviderMembership(t, pool, b.ID, prefix+"-drv-b1"),
		seedProviderMembership(t, pool, b.ID, prefix+"-drv-b2"),
	}
	seedProviderMembership(t, pool, c.ID, prefix+"-drv-c1")
	membershipD := seedProviderMembership(t, pool, d.ID, prefix+"-drv-d1")
	setOperatingMode(t, pool, a.ID, mktmodels.ModeCentralized)
	setOperatingMode(t, pool, b.ID, mktmodels.ModeDelegated)
	setOperatingMode(t, pool, c.ID, mktmodels.ModeCentralized)
	setOperatingMode(t, pool, d.ID, mktmodels.ModeCentralized)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status = 'SUSPENDED' WHERE id = $1`, c.ID); err != nil {
		t.Fatalf("suspend org C: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships
		SET status = 'SUSPENDED', suspended_at = now() WHERE id = $1`, membershipD.ID); err != nil {
		t.Fatalf("suspend membership D: %v", err)
	}
	return w
}

func projectionFor(t *testing.T, pool *pgxpool.Pool, organizationID string, requestID string) *mktmodels.RequestProjection {
	t.Helper()
	projection, err := mktrepos.NewProjectionRepository(pool).GetBySourceRequest(context.Background(),
		organizationID, mktmodels.SourceDispatch, requestID)
	if err != nil {
		t.Fatalf("load projection for %s: %v", organizationID, err)
	}
	return projection
}

func offerFor(t *testing.T, pool *pgxpool.Pool, organizationID string, projectionID string) *mktmodels.Offer {
	t.Helper()
	var offerID string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND request_projection_id = $2 ORDER BY created_at DESC LIMIT 1`,
		organizationID, projectionID).Scan(&offerID); err != nil {
		t.Fatalf("load offer: %v", err)
	}
	offer, err := mktrepos.NewOfferRepository(pool).GetByID(context.Background(), organizationID, offerID)
	if err != nil {
		t.Fatalf("reload offer: %v", err)
	}
	return offer
}

func TestS1112_OfferedEventCreatesOpportunitiesForEligibleOrganizations(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	w := seedOpportunityWorld(t, pool, "S1112OPP")
	service := newOpportunityService(pool, true)
	requestID := "req-s1112-opp"

	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-opp-1", mktusecases.EventRequestOffered,
		"REQUEST", requestID, 2, offeredPayload(requestID, 30*time.Second))
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("offered result = %+v, want ACCEPTED", result)
	}

	// Exactly the two eligible organizations received an opportunity.
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_marketplace_request_projections
		WHERE platform_request_id = $1`, requestID); got != 2 {
		t.Fatalf("projections = %d, want 2 (A and B only)", got)
	}
	for _, excluded := range []string{w.orgC, w.orgD} {
		if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_marketplace_request_projections
			WHERE organization_id = $1`, excluded); got != 0 {
			t.Fatalf("ineligible organization %s received %d projections", excluded, got)
		}
	}

	projectionA := projectionFor(t, pool, w.orgA, requestID)
	projectionB := projectionFor(t, pool, w.orgB, requestID)
	for _, projection := range []*mktmodels.RequestProjection{projectionA, projectionB} {
		if projection.ProjectionStatus != mktmodels.ProjectionOffered || projection.SourceVersion != 2 ||
			projection.FreshnessState != mktmodels.FreshnessCurrent {
			t.Fatalf("projection %s = status %s version %d freshness %s", projection.ID,
				projection.ProjectionStatus, projection.SourceVersion, projection.FreshnessState)
		}
		if projection.PlatformBookingID == nil || *projection.PlatformBookingID != "bkg-"+requestID {
			t.Fatalf("booking reference not projected: %v", projection.PlatformBookingID)
		}
	}
	offerA := offerFor(t, pool, w.orgA, projectionA.ID)
	offerB := offerFor(t, pool, w.orgB, projectionB.ID)
	if offerA.Status != mktmodels.OfferOffered || offerA.OperatingMode != mktmodels.ModeCentralized {
		t.Fatalf("offer A = %s/%s", offerA.Status, offerA.OperatingMode)
	}
	if offerB.OperatingMode != mktmodels.ModeDelegated {
		t.Fatalf("offer B mode = %s", offerB.OperatingMode)
	}
	// The organization gets a human-usable window even though the source
	// broadcast window is 30s; the source stays authoritative at reservation.
	if offerA.ExpiresAt.Before(time.Now().Add(90 * time.Second)) {
		t.Fatalf("offer deadline %s is shorter than the minimum response window", offerA.ExpiresAt)
	}
	// Centralized: the dispatcher accepts, no provider recipients. Delegated:
	// every eligible provider is a recipient.
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_provider_offer_recipients WHERE offer_id = $1`, offerA.ID); got != 0 {
		t.Fatalf("centralized recipients = %d, want 0", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_provider_offer_recipients
		WHERE offer_id = $1 AND status = 'PENDING'`, offerB.ID); got != 2 {
		t.Fatalf("delegated recipients = %d, want 2", got)
	}

	// Audit and outbox are written in the ingestion transaction, per org.
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE action = 'marketplace.opportunity_received' AND resource_id IN ($1, $2)`,
		projectionA.ID, projectionB.ID); got != 2 {
		t.Fatalf("opportunity audit rows = %d, want 2", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_outbox
		WHERE event_type = 'organization.offer.received.v1' AND aggregate_id IN ($1, $2)`,
		offerA.ID, offerB.ID); got != 2 {
		t.Fatalf("offer.received outbox rows = %d, want 2", got)
	}

	// Spec 12.4 data minimization: nothing commercial or personal is stored.
	var stored string
	if err := pool.QueryRow(ctx, `SELECT pickup_summary::text || dropoff_summary::text || operational_requirements::text
		FROM fleet_partner_marketplace_request_projections WHERE id = $1`, projectionB.ID).Scan(&stored); err != nil {
		t.Fatalf("read projection payload: %v", err)
	}
	for _, field := range forbiddenCommercialFields {
		if strings.Contains(strings.ToLower(stored), strings.ToLower(field)) {
			t.Fatalf("projection stored forbidden field %q: %s", field, stored)
		}
	}
	if !strings.Contains(stored, "Ikeja") || !strings.Contains(stored, "small") {
		t.Fatalf("allowlisted operational facts missing: %s", stored)
	}
	providerPage, err := service.ListProviderOffers(ctx, mktmodels.SourceDispatch, *w.providersB[0].ProviderID, 10, 0)
	if err != nil {
		t.Fatalf("provider offers: %v", err)
	}
	if len(providerPage.Items) != 1 || providerPage.Items[0].OfferID != offerB.ID {
		t.Fatalf("provider sees %+v, want offer %s", providerPage.Items, offerB.ID)
	}
	forbiddenFieldsAbsent(t, "provider offer page", providerPage, forbiddenCommercialFields)
	corporate, err := service.GetCorporateOffer(ctx, tenant.Context{OrganizationID: w.orgB}, offerB.ID)
	if err != nil {
		t.Fatalf("corporate offer: %v", err)
	}
	forbiddenFieldsAbsent(t, "corporate offer", corporate, forbiddenCommercialFields)

	// Redelivery is a duplicate and opens nothing twice.
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestDuplicate {
		t.Fatalf("redelivery = %+v, want DUPLICATE", result)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_organization_request_offers o
		JOIN fleet_partner_marketplace_request_projections p ON p.id = o.request_projection_id
		WHERE p.platform_request_id = $1`, requestID); got != 2 {
		t.Fatalf("offers after redelivery = %d, want 2", got)
	}
	// A re-announcement under a new event id is applied to the existing copies
	// (as a version advance), never as new offers.
	raw2, event2 := sourceEvent(t, s1112Dispatch, "evt-s1112-opp-2", mktusecases.EventRequestOffered,
		"REQUEST", requestID, 3, offeredPayload(requestID, 30*time.Second))
	if result := ingest(t, service, raw2, event2); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("re-announcement = %+v", result)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_organization_request_offers o
		JOIN fleet_partner_marketplace_request_projections p ON p.id = o.request_projection_id
		WHERE p.platform_request_id = $1`, requestID); got != 2 {
		t.Fatalf("offers after re-announcement = %d, want 2", got)
	}
}

func TestS1112_OfferedEventIgnoredWithoutEligibleOrganizationOrWhenDisabled(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "S1112 Empty Org", "RC-S1112-EMPTY")
	setOperatingMode(t, pool, org.ID, mktmodels.ModeCentralized)

	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-none", mktusecases.EventRequestOffered,
		"REQUEST", "req-s1112-none", 2, offeredPayload("req-s1112-none", time.Minute))
	result := ingest(t, newOpportunityService(pool, true), raw, event)
	if result.Status != mktusecases.IngestIgnored || result.Code != "no_eligible_organization" {
		t.Fatalf("result = %+v, want IGNORED no_eligible_organization", result)
	}

	// With creation switched off the event is acknowledged as before.
	seedProviderMembership(t, pool, org.ID, "s1112-disabled-drv")
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-disabled", mktusecases.EventRequestOffered,
		"REQUEST", "req-s1112-disabled", 2, offeredPayload("req-s1112-disabled", time.Minute))
	result = ingest(t, newOpportunityService(pool, false), raw, event)
	if result.Status != mktusecases.IngestIgnored || result.Code != "projection_not_found" {
		t.Fatalf("disabled result = %+v, want IGNORED projection_not_found", result)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_marketplace_request_projections`); got != 0 {
		t.Fatalf("projections = %d, want 0", got)
	}

	// A producer that is not an execution source is permanently refused.
	_, err := newOpportunityService(pool, true).IngestEvent(context.Background(), raw,
		func() mktusecases.InboundEvent {
			e := event
			e.SourceService = "support-dispute-service"
			e.EventID = "evt-x"
			return e
		}())
	if err == nil || !strings.Contains(err.Error(), "unknown_source_service") {
		t.Fatalf("unknown producer err = %v, want unknown_source_service", err)
	}
}

func TestS1112_SourceReservationAndClaimStandDownOtherOrganizations(t *testing.T) {
	pool := testPool(t)
	w := seedOpportunityWorld(t, pool, "S1112RSV")
	service := newOpportunityService(pool, true)
	requestID := "req-s1112-rsv"
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-rsv-1", mktusecases.EventRequestOffered,
		"REQUEST", requestID, 2, offeredPayload(requestID, time.Minute))
	ingest(t, service, raw, event)

	// Org A wins the source reservation.
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-rsv-2", mktusecases.EventRequestReserved,
		"REQUEST", requestID, 3, map[string]any{"requestId": requestID, "organizationId": w.orgA, "status": "reserved"})
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("reserved = %+v", result)
	}
	projectionA := projectionFor(t, pool, w.orgA, requestID)
	projectionB := projectionFor(t, pool, w.orgB, requestID)
	if projectionA.ProjectionStatus != mktmodels.ProjectionReservationPending {
		t.Fatalf("winner projection = %s, want RESERVATION_PENDING", projectionA.ProjectionStatus)
	}
	if projectionB.ProjectionStatus != mktmodels.ProjectionCancelled {
		t.Fatalf("loser projection = %s, want CANCELLED", projectionB.ProjectionStatus)
	}
	offerB := offerFor(t, pool, w.orgB, projectionB.ID)
	if offerB.Status != mktmodels.OfferCancelled || offerB.ReasonCode == nil ||
		*offerB.ReasonCode != "source_request_reserved_by_other_organization" {
		t.Fatalf("loser offer = %s/%v", offerB.Status, offerB.ReasonCode)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_provider_offer_recipients
		WHERE offer_id = $1 AND status = 'PENDING'`, offerB.ID); got != 0 {
		t.Fatalf("loser recipients still pending = %d", got)
	}
	// The provider no longer sees Accept/Decline on the stood-down offer.
	page, err := service.ListProviderOffers(context.Background(), mktmodels.SourceDispatch, *w.providersB[0].ProviderID, 10, 0)
	if err != nil {
		t.Fatalf("provider offers: %v", err)
	}
	for _, item := range page.Items {
		if len(item.AllowedActions) != 0 {
			t.Fatalf("stood-down offer still actionable: %+v", item)
		}
	}

	// A second request: an independent provider takes it before any org.
	claimed := "req-s1112-claim"
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-claim-1", mktusecases.EventRequestOffered,
		"REQUEST", claimed, 2, offeredPayload(claimed, 30*time.Second))
	ingest(t, service, raw, event)
	before := offerFor(t, pool, w.orgA, projectionFor(t, pool, w.orgA, claimed).ID)
	// A later broadcast window extends a still-open offer...
	later := time.Now().Add(10 * time.Minute).UTC()
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-claim-2", mktusecases.EventRequestUpdated,
		"REQUEST", claimed, 3, map[string]any{"requestId": claimed, "status": "broadcasting",
			"attemptNumber": 2, "expiresAt": later.Format(time.RFC3339)})
	ingest(t, service, raw, event)
	extended := offerFor(t, pool, w.orgA, projectionFor(t, pool, w.orgA, claimed).ID)
	if !extended.ExpiresAt.After(before.ExpiresAt) {
		t.Fatalf("offer deadline not extended: %s -> %s", before.ExpiresAt, extended.ExpiresAt)
	}
	// ...and an accepted-by-someone-else update stands every open copy down.
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-claim-3", mktusecases.EventRequestUpdated,
		"REQUEST", claimed, 4, map[string]any{"requestId": claimed, "status": "accepted"})
	ingest(t, service, raw, event)
	for _, org := range []string{w.orgA, w.orgB} {
		projection := projectionFor(t, pool, org, claimed)
		if projection.ProjectionStatus != mktmodels.ProjectionCancelled {
			t.Fatalf("claimed projection for %s = %s, want CANCELLED", org, projection.ProjectionStatus)
		}
		if offer := offerFor(t, pool, org, projection.ID); offer.Status != mktmodels.OfferCancelled {
			t.Fatalf("claimed offer for %s = %s", org, offer.Status)
		}
	}
}

func TestS1112_OfferExpiryWorkerExpiresOnceAndRecordsIt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	w := seedOpportunityWorld(t, pool, "S1112EXP")
	service := newOpportunityService(pool, true)
	requestID := "req-s1112-exp"
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-exp-1", mktusecases.EventRequestOffered,
		"REQUEST", requestID, 2, offeredPayload(requestID, time.Minute))
	ingest(t, service, raw, event)
	projectionA := projectionFor(t, pool, w.orgA, requestID)
	projectionB := projectionFor(t, pool, w.orgB, requestID)
	offerA := offerFor(t, pool, w.orgA, projectionA.ID)
	offerB := offerFor(t, pool, w.orgB, projectionB.ID)

	// Not yet due: nothing happens.
	if expired, err := service.ExpireOffers(ctx, 50); err != nil || expired != 0 {
		t.Fatalf("early expiry = %d, %v", expired, err)
	}
	// Org A has already accepted (a dispatcher won the lock first): its offer
	// must not be expired. Org B's lapses.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_request_offers
		SET status = 'ACCEPTED', expires_at = now() - interval '1 minute' WHERE id = $1`, offerA.ID); err != nil {
		t.Fatalf("accept A: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_request_offers
		SET expires_at = now() - interval '1 minute' WHERE id = $1`, offerB.ID); err != nil {
		t.Fatalf("age B: %v", err)
	}

	// Two worker processes race; the offer is expired exactly once.
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			expired, err := service.ExpireOffers(ctx, 50)
			if err != nil {
				t.Errorf("expire: %v", err)
			}
			results <- expired
		}()
	}
	wg.Wait()
	close(results)
	total := 0
	for n := range results {
		total += n
	}
	if total != 1 {
		t.Fatalf("expired %d offers across concurrent workers, want exactly 1", total)
	}
	if offer := offerFor(t, pool, w.orgB, projectionB.ID); offer.Status != mktmodels.OfferExpired {
		t.Fatalf("offer B = %s, want EXPIRED", offer.Status)
	}
	if offer := offerFor(t, pool, w.orgA, projectionA.ID); offer.Status != mktmodels.OfferAccepted {
		t.Fatalf("accepted offer A was touched: %s", offer.Status)
	}
	if projection := projectionFor(t, pool, w.orgB, requestID); projection.ProjectionStatus != mktmodels.ProjectionExpired {
		t.Fatalf("projection B = %s, want EXPIRED", projection.ProjectionStatus)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_provider_offer_recipients
		WHERE offer_id = $1 AND status = 'EXPIRED'`, offerB.ID); got != 2 {
		t.Fatalf("expired recipients = %d, want 2", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE action = 'marketplace.offer_expired' AND resource_id = $1 AND organization_id = $2`,
		offerB.ID, w.orgB); got != 1 {
		t.Fatalf("expiry audit rows = %d, want 1", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_outbox
		WHERE event_type = 'organization.offer.expired.v1' AND aggregate_id = $1 AND organization_id = $2`,
		offerB.ID, w.orgB); got != 1 {
		t.Fatalf("expiry outbox rows = %d, want 1", got)
	}
	// The shared worker pass (as both binaries run it) is idempotent.
	if expired, synced := app.RunMarketplaceLifecyclePass(ctx, service, 50); expired != 0 || synced != 0 {
		t.Fatalf("second pass = %d/%d, want 0/0", expired, synced)
	}
}

// readyAssignment seeds an accepted request with an assignment whose execution
// start has been confirmed (READY_FOR_EXECUTION) unless stopAtAccepted.
func readyAssignment(t *testing.T, pool *pgxpool.Pool, organizationID string, requestID string,
	providerID string, stopAtAccepted bool) (*mktmodels.RequestProjection, *mktmodels.Assignment, *providermodels.Membership) {
	t.Helper()
	ctx := context.Background()
	projection := seedProjection(t, pool, organizationID, requestID, mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, organizationID, providerID)
	assignment := seedAssignment(t, pool, organizationID, projection.ID, membership, time.Now().Add(time.Minute))
	repo := mktrepos.NewAssignmentRepository(pool)
	steps := []string{mktmodels.AssignmentAccepted, mktmodels.AssignmentReadyForExecution}
	if stopAtAccepted {
		steps = steps[:1]
	}
	for _, next := range steps {
		if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return repo.Transition(ctx, tx, assignment, next, assignment.Version,
				mktmodels.StateHistoryEntry{ActorType: "SYSTEM", ActorID: "test"})
		}); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
		reloaded, err := repo.GetByID(ctx, organizationID, assignment.ID)
		if err != nil {
			t.Fatalf("reload assignment: %v", err)
		}
		assignment = reloaded
	}
	return projection, assignment, membership
}

func tripPayload(requestID string, tripID string, assignmentID string, providerID string,
	settlementOrg string, status string) map[string]any {
	return map[string]any{
		"sourceService": s1112Dispatch, "requestId": requestID, "tripId": tripID,
		"fleetAssignmentId": assignmentID, "executingProviderId": providerID,
		"settlementOrganizationId": settlementOrg, "organizationId": settlementOrg,
		"status": status, "fleetExecutionOperationId": "",
	}
}

func TestS1112_TripLifecycleDrivesAssignmentToInProgressAndCompleted(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "S1112 Trip Org", "RC-S1112-TRIP")
	requestID, tripID := "req-s1112-trip", "trip-s1112-1"
	_, assignment, membership := readyAssignment(t, pool, org.ID, requestID, "s1112-trip-drv", false)
	service := newOpportunityService(pool, true)

	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-trip-1", mktusecases.EventTripCreated,
		"TRIP", tripID, 1, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "assigned"))
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("trip.created = %+v", result)
	}
	assignments := mktrepos.NewAssignmentRepository(pool)
	if current, _ := assignments.GetByID(ctx, org.ID, assignment.ID); current.Status != mktmodels.AssignmentReadyForExecution {
		t.Fatalf("assignment after trip.created = %s, want READY_FOR_EXECUTION", current.Status)
	}

	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-trip-2", mktusecases.EventTripStatusChanged,
		"TRIP", tripID, 2, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "in_progress"))
	ingest(t, service, raw, event)
	started, _ := assignments.GetByID(ctx, org.ID, assignment.ID)
	if started.Status != mktmodels.AssignmentInProgress || started.StartedAt == nil {
		t.Fatalf("assignment after in_progress = %s (startedAt %v), want IN_PROGRESS", started.Status, started.StartedAt)
	}

	payload := tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "completed")
	payload["completedAt"] = time.Now().UTC().Format(time.RFC3339)
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-trip-3", mktusecases.EventTripCompleted, "TRIP", tripID, 3, payload)
	ingest(t, service, raw, event)
	completed, _ := assignments.GetByID(ctx, org.ID, assignment.ID)
	if completed.Status != mktmodels.AssignmentCompleted || completed.CompletedAt == nil {
		t.Fatalf("assignment after trip.completed = %s, want COMPLETED", completed.Status)
	}
	if held, _ := assignments.CountHeld(ctx, mktmodels.ResourceProvider, membership.ID); held != 0 {
		t.Fatalf("provider capacity still held after completion: %d", held)
	}
	if projection := projectionFor(t, pool, org.ID, requestID); projection.ProjectionStatus != mktmodels.ProjectionCompleted {
		t.Fatalf("request projection = %s, want COMPLETED", projection.ProjectionStatus)
	}
	// Completion carries the settlement references, never an amount.
	var completedPayload string
	if err := pool.QueryRow(ctx, `SELECT payload::text FROM fleet_partner_outbox
		WHERE event_type = 'organization.assignment.completed.v1' AND aggregate_id = $1`, assignment.ID).Scan(&completedPayload); err != nil {
		t.Fatalf("completed outbox: %v", err)
	}
	for _, want := range []string{tripID, org.ID, *membership.ProviderID} {
		if !strings.Contains(completedPayload, want) {
			t.Fatalf("completion payload missing %q: %s", want, completedPayload)
		}
	}
	for _, field := range forbiddenCommercialFields {
		if strings.Contains(strings.ToLower(completedPayload), strings.ToLower(field)) {
			t.Fatalf("completion payload carries %q: %s", field, completedPayload)
		}
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_audit_records
		WHERE resource_id = $1 AND action IN ('assignment.started', 'assignment.completed')`, assignment.ID); got != 2 {
		t.Fatalf("trip-driven audit rows = %d, want 2", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_assignment_state_history
		WHERE assignment_id = $1 AND actor_id = 'trip-lifecycle'`, assignment.ID); got != 2 {
		t.Fatalf("trip-driven history rows = %d, want 2", got)
	}

	// A stale redelivery of the status change cannot move a completed
	// assignment backwards.
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-trip-2b", mktusecases.EventTripStatusChanged,
		"TRIP", tripID, 2, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "in_progress"))
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestIgnored {
		t.Fatalf("stale trip event = %+v, want IGNORED", result)
	}
	if again, _ := assignments.GetByID(ctx, org.ID, assignment.ID); again.Status != mktmodels.AssignmentCompleted {
		t.Fatalf("assignment regressed to %s", again.Status)
	}
}

func TestS1112_TripSyncRecoversAssignmentAfterExecutionStartConfirms(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "S1112 Sync Org", "RC-S1112-SYNC")
	requestID, tripID := "req-s1112-sync", "trip-s1112-sync"
	_, assignment, membership := readyAssignment(t, pool, org.ID, requestID, "s1112-sync-drv", true)
	service := newOpportunityService(pool, true)

	// The trip runs and completes before Fleet's execution-start worker has
	// confirmed the start: the assignment is still ACCEPTED and must not be
	// pushed past the commit point the worker owns.
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-sync-1", mktusecases.EventTripCreated,
		"TRIP", tripID, 1, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "assigned"))
	ingest(t, service, raw, event)
	payload := tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, org.ID, "completed")
	payload["completedAt"] = time.Now().UTC().Format(time.RFC3339)
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-sync-2", mktusecases.EventTripCompleted, "TRIP", tripID, 2, payload)
	ingest(t, service, raw, event)
	assignments := mktrepos.NewAssignmentRepository(pool)
	if current, _ := assignments.GetByID(ctx, org.ID, assignment.ID); current.Status != mktmodels.AssignmentAccepted {
		t.Fatalf("assignment = %s, want still ACCEPTED", current.Status)
	}
	if synced, err := service.SyncAssignmentsFromTrips(ctx, 50); err != nil || synced != 0 {
		t.Fatalf("premature sync = %d, %v", synced, err)
	}

	// The worker confirms execution (as after a restart and read-back).
	current, _ := assignments.GetByID(ctx, org.ID, assignment.ID)
	for _, next := range []string{mktmodels.AssignmentExecutionStartPending, mktmodels.AssignmentReadyForExecution} {
		if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return assignments.Transition(ctx, tx, current, next, current.Version,
				mktmodels.StateHistoryEntry{ActorType: "SYSTEM", ActorID: "execution-start"})
		}); err != nil {
			t.Fatalf("transition %s: %v", next, err)
		}
		current, _ = assignments.GetByID(ctx, org.ID, assignment.ID)
	}

	// The recovery sweep, run concurrently from two processes, completes it
	// exactly once.
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			synced, err := service.SyncAssignmentsFromTrips(ctx, 50)
			if err != nil {
				t.Errorf("sync: %v", err)
			}
			counts <- synced
		}()
	}
	wg.Wait()
	close(counts)
	total := 0
	for n := range counts {
		total += n
	}
	if total != 1 {
		t.Fatalf("assignments synced = %d, want exactly 1", total)
	}
	if done, _ := assignments.GetByID(ctx, org.ID, assignment.ID); done.Status != mktmodels.AssignmentCompleted {
		t.Fatalf("assignment after sync = %s, want COMPLETED", done.Status)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_outbox
		WHERE event_type = 'organization.assignment.completed.v1' AND aggregate_id = $1`, assignment.ID); got != 1 {
		t.Fatalf("completion events = %d, want 1", got)
	}
}

func TestS1112_TripCannotMoveAnotherTenantsAssignment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "S1112 Tenant A", "RC-S1112-TA")
	orgB := createOrganization(t, pool, "S1112 Tenant B", "RC-S1112-TB")
	requestID := "req-s1112-tenant"
	seedProjection(t, pool, orgA.ID, requestID, mktmodels.ProjectionOrganizationAccepted)
	_, foreign, membership := readyAssignment(t, pool, orgB.ID, "req-s1112-tenant-b", "s1112-tenant-drv", false)
	service := newOpportunityService(pool, true)

	// The trip belongs to org A's request but names org B's assignment.
	payload := tripPayload(requestID, "trip-s1112-tenant", foreign.ID, *membership.ProviderID, orgA.ID, "in_progress")
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-tenant-1", mktusecases.EventTripCreated,
		"TRIP", "trip-s1112-tenant", 1, payload)
	ingest(t, service, raw, event)
	if current, _ := mktrepos.NewAssignmentRepository(pool).GetByID(ctx, orgB.ID, foreign.ID); current.Status != mktmodels.AssignmentReadyForExecution {
		t.Fatalf("foreign assignment moved to %s", current.Status)
	}
}

func TestS1112_CorporateTripRoutes(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "S1112 Routes A", "RC-S1112-RA")
	orgB := createOrganization(t, pool, "S1112 Routes B", "RC-S1112-RB")
	service := newOpportunityService(pool, true)
	requestID, tripID := "req-s1112-routes", "trip-s1112-routes"
	_, assignment, membership := readyAssignment(t, pool, orgA.ID, requestID, "s1112-routes-drv", false)
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-routes-1", mktusecases.EventTripCreated,
		"TRIP", tripID, 1, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, orgA.ID, "assigned"))
	ingest(t, service, raw, event)
	raw, event = sourceEvent(t, s1112Dispatch, "evt-s1112-routes-2", mktusecases.EventTripStatusChanged,
		"TRIP", tripID, 2, tripPayload(requestID, tripID, assignment.ID, *membership.ProviderID, orgA.ID, "in_progress"))
	ingest(t, service, raw, event)

	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	mkthttp.NewHandler(service, authorizer, nil).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))

	dispatcherA := seedApprovalMember(t, pool, orgA.ID, "rol_system_dispatcher", "s1112-dispatcher-a")
	dispatcherB := seedApprovalMember(t, pool, orgB.ID, "rol_system_dispatcher", "s1112-dispatcher-b")
	noRoleA := seedApprovalMember(t, pool, orgA.ID, "", "s1112-norole-a")
	call := func(org string, member string, path string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", member)
		req.Header.Set("X-Test-Org", org)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		var body map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder.Code, body
	}

	code, body := call(orgA.ID, dispatcherA, "/fleet-partner/v1/marketplace/trips")
	if code != http.StatusOK {
		t.Fatalf("list trips = %d %v", code, body)
	}
	items := body["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("trips listed = %d, want 1: %v", len(items), body)
	}
	tripProjectionID := items[0].(map[string]any)["tripId"].(string)

	code, body = call(orgA.ID, dispatcherA, "/fleet-partner/v1/marketplace/trips/"+tripProjectionID)
	if code != http.StatusOK {
		t.Fatalf("get trip = %d %v", code, body)
	}
	data := body["data"].(map[string]any)
	if data["trip"].(map[string]any)["platformTripId"] != tripID {
		t.Fatalf("trip detail = %v", data)
	}
	if len(data["history"].([]any)) != 2 {
		t.Fatalf("trip history = %v, want 2 entries", data["history"])
	}
	if data["assignment"].(map[string]any)["status"] != mktmodels.AssignmentInProgress {
		t.Fatalf("trip assignment = %v, want IN_PROGRESS", data["assignment"])
	}
	forbiddenFieldsAbsent(t, "trip detail", data, forbiddenCommercialFields)

	// Another tenant cannot see it; its own list is empty.
	if code, _ := call(orgB.ID, dispatcherB, "/fleet-partner/v1/marketplace/trips/"+tripProjectionID); code != http.StatusNotFound {
		t.Fatalf("cross-tenant get = %d, want 404", code)
	}
	if code, body := call(orgB.ID, dispatcherB, "/fleet-partner/v1/marketplace/trips"); code != http.StatusOK ||
		len(body["data"].(map[string]any)["items"].([]any)) != 0 {
		t.Fatalf("cross-tenant list = %d %v", code, body)
	}
	// Default deny: no marketplace.read, no trips.
	if code, _ := call(orgA.ID, noRoleA, "/fleet-partner/v1/marketplace/trips"); code != http.StatusForbidden {
		t.Fatalf("no-role list = %d, want 403", code)
	}
	// Invalid filter is a validation error.
	if code, _ := call(orgA.ID, dispatcherA, "/fleet-partner/v1/marketplace/trips?sourceService=ROCKET"); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("invalid filter = %d, want 4xx validation", code)
	}
}

// seedCategoryMembership creates an ACTIVE, association-accepted managed
// provider in the given category for an eligible organization.
func seedCategoryMembership(t *testing.T, pool *pgxpool.Pool, organizationID string,
	providerService string, category string, providerID string) *providermodels.Membership {
	t.Helper()
	now := time.Now()
	membership := &providermodels.Membership{
		ProviderService: providerService, ServiceCategory: category, ProviderID: &providerID,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: providermodels.StatusActive, AssociationAcceptedAt: &now,
	}
	if err := providerrepos.NewRepository(pool).CreateMembership(context.Background(), pool, organizationID, membership); err != nil {
		t.Fatalf("seed %s membership: %v", category, err)
	}
	seedEligibleOrganization(t, pool, organizationID)
	markMembershipSynced(t, pool, membership.ID)
	return membership
}

// TestS1112_TaxiAndHaulingProducerEnvelopesCreateOpportunities ingests the
// exact envelopes the new Taxi (migration 029) and Hauling (migration 034)
// feeds deliver -- lowercase aggregate types, their payload shapes, their
// signed service names -- and proves each opens an opportunity only for
// organizations managing that service's providers.
func TestS1112_TaxiAndHaulingProducerEnvelopesCreateOpportunities(t *testing.T) {
	pool := testPool(t)
	taxiOrg := createOrganization(t, pool, "S1112 Taxi Co", "RC-S1112-TAXI")
	haulingOrg := createOrganization(t, pool, "S1112 Hauling Co", "RC-S1112-HAUL")
	seedCategoryMembership(t, pool, taxiOrg.ID, "driver-taxi-service", mktmodels.SourceTaxi, "s1112-taxi-drv")
	seedCategoryMembership(t, pool, haulingOrg.ID, "driver-hauling-service", mktmodels.SourceHauling, "s1112-haul-drv")
	setOperatingMode(t, pool, taxiOrg.ID, mktmodels.ModeHybrid)
	setOperatingMode(t, pool, haulingOrg.ID, mktmodels.ModeCentralized)
	service := newOpportunityService(pool, true)

	taxiRequest := "taxi-booking-s1112"
	raw, event := sourceEvent(t, "driver-taxi-service", "evt-s1112-taxi-1", mktusecases.EventRequestOffered,
		"request", taxiRequest, 1, map[string]any{
			"sourceService": "driver-taxi-service", "requestId": taxiRequest, "bookingId": taxiRequest,
			"sourceVersion": 1, "status": "pending_match", "serviceType": "taxi",
			"vehicleClass": "economy", "passengerCount": 2,
		})
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("taxi offered = %+v", result)
	}
	var taxiProjections, wrongCategory int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE organization_id=$1 AND source_service='TAXI'),
		count(*) FILTER (WHERE organization_id=$2)
		FROM fleet_partner_marketplace_request_projections WHERE platform_request_id=$3`,
		taxiOrg.ID, haulingOrg.ID, taxiRequest).Scan(&taxiProjections, &wrongCategory)
	if taxiProjections != 1 || wrongCategory != 0 {
		t.Fatalf("taxi opportunity projections = %d (hauling org %d), want 1/0", taxiProjections, wrongCategory)
	}
	var requirements string
	_ = pool.QueryRow(context.Background(), `SELECT operational_requirements::text FROM fleet_partner_marketplace_request_projections
		WHERE platform_request_id=$1`, taxiRequest).Scan(&requirements)
	if !strings.Contains(requirements, "economy") || !strings.Contains(requirements, "passengerCount") {
		t.Fatalf("taxi operational facts not projected: %s", requirements)
	}
	// HYBRID: providers are recipients too.
	if got := countRows(t, pool, `SELECT count(*) FROM fleet_partner_provider_offer_recipients rec
		JOIN fleet_partner_organization_request_offers o ON o.id = rec.offer_id
		JOIN fleet_partner_marketplace_request_projections p ON p.id = o.request_projection_id
		WHERE p.platform_request_id = $1`, taxiRequest); got != 1 {
		t.Fatalf("hybrid taxi recipients = %d, want 1", got)
	}
	// Taken by an independent driver.
	raw, event = sourceEvent(t, "driver-taxi-service", "evt-s1112-taxi-2", mktusecases.EventRequestUpdated,
		"request", taxiRequest, 2, map[string]any{"requestId": taxiRequest, "status": "accepted", "sourceVersion": 2})
	ingest(t, service, raw, event)
	var taxiStatus string
	_ = pool.QueryRow(context.Background(), `SELECT projection_status FROM fleet_partner_marketplace_request_projections
		WHERE platform_request_id=$1`, taxiRequest).Scan(&taxiStatus)
	if taxiStatus != mktmodels.ProjectionCancelled {
		t.Fatalf("taxi projection after independent claim = %s, want CANCELLED", taxiStatus)
	}

	haulRequest := "haul-booking-s1112"
	raw, event = sourceEvent(t, "driver-hauling-service", "evt-s1112-haul-1", mktusecases.EventRequestOffered,
		"request", haulRequest, 1, map[string]any{
			"sourceService": "driver-hauling-service", "requestId": haulRequest, "bookingId": haulRequest,
			"sourceVersion": 1, "status": "pending_match", "serviceType": "hauling", "weightKg": 750,
			"helpersRequired": 2,
		})
	if result := ingest(t, service, raw, event); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("hauling offered = %+v", result)
	}
	raw, event = sourceEvent(t, "driver-hauling-service", "evt-s1112-haul-2", mktusecases.EventRequestExpired,
		"request", haulRequest, 2, map[string]any{"requestId": haulRequest, "status": "unmatched", "sourceVersion": 2})
	ingest(t, service, raw, event)
	var haulStatus, offerStatus string
	_ = pool.QueryRow(context.Background(), `SELECT p.projection_status, o.status
		FROM fleet_partner_marketplace_request_projections p
		JOIN fleet_partner_organization_request_offers o ON o.request_projection_id = p.id
		WHERE p.platform_request_id=$1 AND p.organization_id=$2 AND p.source_service='HAULING'`,
		haulRequest, haulingOrg.ID).Scan(&haulStatus, &offerStatus)
	if haulStatus != mktmodels.ProjectionExpired || offerStatus != mktmodels.OfferCancelled {
		t.Fatalf("hauling after unmatched = %s/%s, want EXPIRED/CANCELLED", haulStatus, offerStatus)
	}
}

// TestS1112_MarketplaceBranchScopeCannotBeWidenedByQueryParameter applies the
// staging branch-scope fix (spec 7b) to the marketplace surface: a dispatcher
// whose only grant is at branch A cannot read, list or assign branch B work by
// naming ?branchId=A.
func TestS1112_MarketplaceBranchScopeCannotBeWidenedByQueryParameter(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "S1112 Branch Scope Co", "RC-S1112-BS")
	branchA := seedBranch(t, pool, org.ID, "S1112 Branch A")
	branchB := seedBranch(t, pool, org.ID, "S1112 Branch B")
	service := newOpportunityService(pool, true)

	_, assignmentA, membershipA := readyAssignment(t, pool, org.ID, "req-s1112-bs-a", "s1112-bs-drv-a", false)
	_, assignmentB, membershipB := readyAssignment(t, pool, org.ID, "req-s1112-bs-b", "s1112-bs-drv-b", false)
	for membership, branch := range map[string]string{membershipA.ID: branchA, membershipB.ID: branchB} {
		if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships
			SET home_branch_id = $2 WHERE id = $1`, membership, branch); err != nil {
			t.Fatalf("home branch: %v", err)
		}
	}
	raw, event := sourceEvent(t, s1112Dispatch, "evt-s1112-bs-trip", mktusecases.EventTripCreated, "TRIP",
		"trip-s1112-bs-b", 1, tripPayload("req-s1112-bs-b", "trip-s1112-bs-b", assignmentB.ID,
			*membershipB.ProviderID, org.ID, "assigned"))
	ingest(t, service, raw, event)
	var tripB string
	if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_trip_projections WHERE platform_trip_id = 'trip-s1112-bs-b'`).Scan(&tripB); err != nil {
		t.Fatalf("trip projection: %v", err)
	}

	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	mkthttp.NewHandler(service, authorizer, nil).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	branchOnly := seedMember(t, pool, org.ID, seedIdentity(t, pool,
		"s1112-branch-only-"+randomHex()[:6]+"@example.com", "+23481"+randomHex()[:8]), false, nil)
	bindRole(t, pool, org.ID, branchOnly.ID, "rol_system_dispatcher", "BRANCH", branchA)
	get := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", branchOnly.ID)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder.Code
	}

	for _, path := range []string{
		"/fleet-partner/v1/marketplace/assignments/" + assignmentB.ID,
		"/fleet-partner/v1/marketplace/assignments/" + assignmentB.ID + "?branchId=" + branchA,
		"/fleet-partner/v1/marketplace/assignments/" + assignmentB.ID + "/history?branchId=" + branchA,
		"/fleet-partner/v1/marketplace/trips/" + tripB + "?branchId=" + branchA,
	} {
		if code := get(path); code != http.StatusNotFound {
			t.Fatalf("branch-A-only dispatcher GET %s = %d, want 404", path, code)
		}
	}
	if code := get("/fleet-partner/v1/marketplace/assignments/" + assignmentA.ID + "?branchId=" + branchA); code != http.StatusOK {
		t.Fatalf("own-branch assignment = %d, want 200", code)
	}
	// Assigning a branch-B provider is refused as not found.
	_, err := service.Assign(ctx, mktusecases.AssignInput{
		Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: branchOnly.ID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: branchOnly.ID,
			PermittedBranchIDs: []string{branchA}},
		ProjectionID:   seedProjection(t, pool, org.ID, "req-s1112-bs-c", mktmodels.ProjectionOrganizationAccepted).ID,
		MembershipID:   membershipB.ID,
		IdempotencyKey: "s1112-bs-assign",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("cross-branch assign err = %v, want not found", err)
	}
}
