package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// tripEvent builds a TRIP-aggregate envelope.
//
// Distinct from inboundEvent, which builds request-aggregate envelopes: the
// aggregate type and the version sequence are exactly what this work package
// separated, so the tests must be able to say which one they mean.
func tripEvent(t *testing.T, eventID string, eventType string, tripID string,
	version int64, payload map[string]any) ([]byte, mktusecases.InboundEvent) {
	t.Helper()
	if payload == nil {
		payload = map[string]any{}
	}
	if _, present := payload["tripId"]; !present {
		payload["tripId"] = tripID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	event := mktusecases.InboundEvent{
		EventID:          eventID,
		EventType:        eventType,
		SchemaVersion:    1,
		SourceService:    mktmodels.SourceDispatch,
		AggregateType:    "TRIP",
		AggregateID:      tripID,
		AggregateVersion: version,
		OccurredAt:       time.Now().UTC(),
		CorrelationID:    "corr-" + eventID,
		PayloadHash:      fleetevents.HashPayload(encoded),
		Payload:          payload,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return raw, event
}

// tripFixture is an organization with a request projection and a trip id.
type tripFixture struct {
	pool       *pgxpool.Pool
	service    *mktusecases.Service
	orgID      string
	requestID  string
	tripID     string
	projection *mktmodels.RequestProjection
}

func newTripFixture(t *testing.T, name string) *tripFixture {
	t.Helper()
	pool := testPool(t)
	orgID := createOrganization(t, pool, name, "RC-"+ids.New("reg")).ID
	requestID := ids.New("req")
	return &tripFixture{
		pool:       pool,
		service:    newIngestionService(pool),
		orgID:      orgID,
		requestID:  requestID,
		tripID:     ids.New("trip"),
		projection: seedProjection(t, pool, orgID, requestID, mktmodels.ProjectionOffered),
	}
}

// createTrip ingests trip.created.v1 at version 1.
func (f *tripFixture) createTrip(t *testing.T, extra map[string]any) mktusecases.IngestResult {
	t.Helper()
	payload := map[string]any{
		"tripId":              f.tripID,
		"requestId":           f.requestID,
		"bookingId":           ids.New("bkg"),
		"executingProviderId": ids.New("prv"),
		"serviceType":         "dispatch",
		"status":              "assigned",
	}
	for key, value := range extra {
		payload[key] = value
	}
	raw, event := tripEvent(t, "evt_"+ids.New("e"), mktusecases.EventTripCreated,
		f.tripID, 1, payload)
	result, err := f.service.IngestEvent(context.Background(), raw, event)
	if err != nil {
		t.Fatalf("ingest trip.created: %v", err)
	}
	return result
}

// ingest sends one trip event and fails the test on transport-level error.
func (f *tripFixture) ingest(t *testing.T, eventType string, version int64,
	payload map[string]any) mktusecases.IngestResult {
	t.Helper()
	raw, event := tripEvent(t, "evt_"+ids.New("e"), eventType, f.tripID, version, payload)
	result, err := f.service.IngestEvent(context.Background(), raw, event)
	if err != nil {
		t.Fatalf("ingest %s v%d: %v", eventType, version, err)
	}
	return result
}

// trip reloads the projection under test.
func (f *tripFixture) trip(t *testing.T) *mktmodels.TripProjection {
	t.Helper()
	stored, err := mktrepos.NewTripRepository(f.pool).GetBySourceTripAnyOrganization(
		context.Background(), f.pool, mktmodels.SourceDispatch, f.tripID)
	if err != nil {
		t.Fatalf("reload trip projection: %v", err)
	}
	return stored
}

// request reloads the associated request projection.
func (f *tripFixture) request(t *testing.T) *mktmodels.RequestProjection {
	t.Helper()
	stored, err := mktrepos.NewProjectionRepository(f.pool).GetByID(
		context.Background(), f.orgID, f.projection.ID)
	if err != nil {
		t.Fatalf("reload request projection: %v", err)
	}
	return stored
}

// historyCount counts append-only trip history rows.
func (f *tripFixture) historyCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_trip_projection_history
		WHERE trip_projection_id = (
			SELECT id FROM fleet_partner_trip_projections
			WHERE source_service = $1 AND platform_trip_id = $2)`,
		mktmodels.SourceDispatch, f.tripID).Scan(&count); err != nil {
		t.Fatalf("count history: %v", err)
	}
	return count
}

// openCases counts live reconciliation cases for the trip.
func (f *tripFixture) openCases(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_type = 'TRIP' AND aggregate_source_id = $1
		  AND status IN ('OPEN', 'IN_PROGRESS')`, f.tripID).Scan(&count); err != nil {
		t.Fatalf("count cases: %v", err)
	}
	return count
}

// TestTripCreatedInsertsProjection covers the insertion path and the canonical
// identifiers a fleet operator needs.
func TestTripCreatedInsertsProjection(t *testing.T) {
	fixture := newTripFixture(t, "Trip Created Logistics")
	if result := fixture.createTrip(t, nil); result.Status != mktusecases.IngestAccepted {
		t.Fatalf("status = %q, want ACCEPTED", result.Status)
	}

	trip := fixture.trip(t)
	if trip.PlatformTripID != fixture.tripID {
		t.Fatalf("platform_trip_id = %q, want %q", trip.PlatformTripID, fixture.tripID)
	}
	if trip.OrganizationID != fixture.orgID {
		t.Fatalf("organization = %q, want %q — the tenant must come from the "+
			"request projection, not the event body", trip.OrganizationID, fixture.orgID)
	}
	if trip.SourceVersion != 1 {
		t.Fatalf("source_version = %d, want 1", trip.SourceVersion)
	}
	if trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", trip.FreshnessState)
	}
	if trip.TripStatus != "assigned" {
		t.Fatalf("trip_status = %q, want assigned", trip.TripStatus)
	}
	if trip.LastPayloadHash == nil || *trip.LastPayloadHash == "" {
		t.Fatal("last_payload_hash not stored; same-version conflicts become undetectable")
	}

	// Request-to-trip association, in both directions.
	if trip.PlatformRequestID == nil || *trip.PlatformRequestID != fixture.requestID {
		t.Fatalf("platform_request_id = %v, want %q", trip.PlatformRequestID, fixture.requestID)
	}
	request := fixture.request(t)
	if request.PlatformTripID == nil || *request.PlatformTripID != fixture.tripID {
		t.Fatalf("request.platform_trip_id = %v, want %q", request.PlatformTripID, fixture.tripID)
	}
	if request.ProjectionStatus != mktmodels.ProjectionExecutionCreated {
		t.Fatalf("request status = %q, want EXECUTION_CREATED", request.ProjectionStatus)
	}
}

// TestTripStatusProgressionAppliesContiguousVersions walks the normal
// lifecycle and asserts the version sequence stays contiguous and trusted.
func TestTripStatusProgressionAppliesContiguousVersions(t *testing.T) {
	fixture := newTripFixture(t, "Trip Progress Logistics")
	fixture.createTrip(t, nil)

	for index, status := range []string{"en_route_pickup", "arrived_pickup", "in_progress"} {
		version := int64(index + 2)
		fixture.ingest(t, mktusecases.EventTripStatusChanged, version,
			map[string]any{"status": status, "toStatus": status})
		trip := fixture.trip(t)
		if trip.TripStatus != status {
			t.Fatalf("v%d trip_status = %q, want %q", version, trip.TripStatus, status)
		}
		if trip.SourceVersion != version {
			t.Fatalf("source_version = %d, want %d", trip.SourceVersion, version)
		}
		if trip.FreshnessState != mktmodels.FreshnessCurrent {
			t.Fatalf("v%d freshness = %q, want CURRENT", version, trip.FreshnessState)
		}
	}

	if got := fixture.historyCount(t); got != 4 {
		t.Fatalf("history rows = %d, want 4 (created + three transitions)", got)
	}
	if got := fixture.openCases(t); got != 0 {
		t.Fatalf("contiguous progression opened %d reconciliation cases, want 0", got)
	}
}

// TestTripCompletionUpdatesBothProjectionsAtomically covers requirement 13.
func TestTripCompletionUpdatesBothProjectionsAtomically(t *testing.T) {
	fixture := newTripFixture(t, "Trip Completion Logistics")
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})
	fixture.ingest(t, mktusecases.EventProofCompleted, 3,
		map[string]any{"hasPhoto": true, "hasSignature": true, "verified": false})
	fixture.ingest(t, mktusecases.EventTripCompleted, 4,
		map[string]any{"status": "completed"})

	trip := fixture.trip(t)
	if trip.CompletedAt == nil {
		t.Fatal("completed_at not set from the source payload")
	}
	if trip.TripStatus != "completed" {
		t.Fatalf("trip_status = %q, want completed", trip.TripStatus)
	}
	if request := fixture.request(t); request.ProjectionStatus != mktmodels.ProjectionCompleted {
		t.Fatalf("request status = %q, want COMPLETED — completion must reach "+
			"both projections in one transaction", request.ProjectionStatus)
	}
}

// TestProofCompletionDoesNotAdvanceRequestProjection pins proof as evidence
// rather than a lifecycle transition.
func TestProofCompletionDoesNotAdvanceRequestProjection(t *testing.T) {
	fixture := newTripFixture(t, "Trip Proof Logistics")
	fixture.createTrip(t, nil)
	before := fixture.request(t).ProjectionStatus

	fixture.ingest(t, mktusecases.EventProofCompleted, 2,
		map[string]any{"hasPhoto": true, "verified": true})

	if after := fixture.request(t).ProjectionStatus; after != before {
		t.Fatalf("proof moved the request projection %q -> %q; proof is evidence, "+
			"not a state transition", before, after)
	}
	if trip := fixture.trip(t); trip.SourceVersion != 2 {
		t.Fatalf("proof did not advance the trip version: %d", trip.SourceVersion)
	}
	if got := fixture.historyCount(t); got != 2 {
		t.Fatalf("history rows = %d, want 2 — proof must still be recorded", got)
	}
}

// TestTripCancellationReleasesCapacityOnlyWhenTrusted covers requirement 14.
func TestTripCancellationReleasesCapacityOnlyWhenTrusted(t *testing.T) {
	fixture := newTripFixture(t, "Trip Cancel Logistics")
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripCancelled, 2,
		map[string]any{"status": "cancelled", "cancelledBy": "PROVIDER"})

	trip := fixture.trip(t)
	if trip.CancelledAt == nil {
		t.Fatal("cancelled_at not set from the source payload")
	}
	if request := fixture.request(t); request.ProjectionStatus != mktmodels.ProjectionCancelled {
		t.Fatalf("request status = %q, want CANCELLED", request.ProjectionStatus)
	}
}

// TestTerminalEventOnGappedProjectionDoesNotReleaseCapacity is the
// fail-closed gate.
//
// A cancellation arriving on a projection fleet cannot vouch for must not
// stand down the offer: releasing the provider and vehicle for a trip that may
// still be running is the expensive direction of the error.
func TestTerminalEventOnGappedProjectionDoesNotReleaseCapacity(t *testing.T) {
	fixture := newTripFixture(t, "Trip Gate Logistics")
	fixture.createTrip(t, nil)

	// Version 7 after version 1: five events never arrived.
	fixture.ingest(t, mktusecases.EventTripCancelled, 7,
		map[string]any{"status": "cancelled"})

	trip := fixture.trip(t)
	if trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED", trip.FreshnessState)
	}
	request := fixture.request(t)
	if request.ProjectionStatus == mktmodels.ProjectionCancelled {
		t.Fatal("a cancellation on a gapped projection released capacity; it must " +
			"wait for reconciliation")
	}
	if got := fixture.openCases(t); got != 1 {
		t.Fatalf("open reconciliation cases = %d, want 1", got)
	}
}

// TestIdenticalDuplicateTripEventIsIdempotent covers requirement 3.
func TestIdenticalDuplicateTripEventIsIdempotent(t *testing.T) {
	fixture := newTripFixture(t, "Trip Duplicate Logistics")
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})
	before := fixture.trip(t)

	// A different event id carrying the same version and the same payload:
	// a redelivery the inbox cannot dedupe, which the projection must.
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})

	after := fixture.trip(t)
	if after.Version != before.Version {
		t.Fatalf("row version moved %d -> %d on an identical redelivery",
			before.Version, after.Version)
	}
	if after.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", after.FreshnessState)
	}
	if got := fixture.openCases(t); got != 0 {
		t.Fatalf("an identical redelivery opened %d cases, want 0", got)
	}
}

// TestChangedPayloadAtSameVersionIsConflict covers requirement 5.
func TestChangedPayloadAtSameVersionIsConflict(t *testing.T) {
	fixture := newTripFixture(t, "Trip Conflict Logistics")
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})

	// Same version, different content. The source has contradicted itself.
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "arrived_pickup"})

	trip := fixture.trip(t)
	if trip.TripStatus != "in_progress" {
		t.Fatalf("trip_status = %q; a conflicting payload must not silently "+
			"overwrite the applied one", trip.TripStatus)
	}
	if trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED on conflict", trip.FreshnessState)
	}
	if got := fixture.openCases(t); got != 1 {
		t.Fatalf("conflict opened %d reconciliation cases, want 1", got)
	}
}

// TestOlderTripVersionIgnoredWithoutRegression covers requirement 4.
func TestOlderTripVersionIgnoredWithoutRegression(t *testing.T) {
	fixture := newTripFixture(t, "Trip Older Logistics")
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 3,
		map[string]any{"status": "proof_submitted"})

	result := fixture.ingest(t, mktusecases.EventTripStatusChanged, 2,
		map[string]any{"status": "in_progress"})
	if result.Status != mktusecases.IngestIgnored {
		t.Fatalf("status = %q, want IGNORED for an older version", result.Status)
	}

	trip := fixture.trip(t)
	if trip.TripStatus != "proof_submitted" || trip.SourceVersion != 3 {
		t.Fatalf("older event regressed the projection to %q v%d",
			trip.TripStatus, trip.SourceVersion)
	}
	if trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q; an ordinary out-of-order replay is not a gap",
			trip.FreshnessState)
	}
}

// TestMissingTripVersionMarksGapDetected covers requirement 6.
func TestMissingTripVersionMarksGapDetected(t *testing.T) {
	fixture := newTripFixture(t, "Trip Gap Logistics")
	fixture.createTrip(t, nil)

	fixture.ingest(t, mktusecases.EventTripStatusChanged, 5,
		map[string]any{"status": "in_progress"})

	trip := fixture.trip(t)
	if trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED", trip.FreshnessState)
	}
	// The newer state is still applied: it is genuinely more current.
	if trip.SourceVersion != 5 || trip.TripStatus != "in_progress" {
		t.Fatalf("gapped event was not applied: v%d %q", trip.SourceVersion, trip.TripStatus)
	}
	if got := fixture.openCases(t); got != 1 {
		t.Fatalf("gap opened %d reconciliation cases, want 1", got)
	}

	// A subsequent in-order event does not clear the gap: one contiguous
	// event is no evidence that the missing ones ever arrived.
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 6,
		map[string]any{"status": "proof_submitted"})
	if after := fixture.trip(t); after.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q; only reconciliation clears a gap", after.FreshnessState)
	}
}

// TestTripEventsCarryNoCustomerCommercialData covers requirement 10.
//
// Asserts on the stored row rather than on the mapper, so a field added to the
// projection later is caught here rather than assumed harmless.
func TestTripProjectionStoresNoCustomerCommercialData(t *testing.T) {
	fixture := newTripFixture(t, "Trip Minimization Logistics")
	// A hostile producer including everything it should not.
	fixture.createTrip(t, map[string]any{
		"receiverName":  "Confidential Receiver",
		"receiverPhone": "+2348099887766",
		"customerId":    "cus_secret",
		"pickupAddress": "12 Secret Lane",
	})

	var stored map[string]any
	rows, err := fixture.pool.Query(context.Background(), `
		SELECT to_jsonb(t) FROM fleet_partner_trip_projections t
		WHERE source_service = $1 AND platform_trip_id = $2`,
		mktmodels.SourceDispatch, fixture.tripID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("trip projection row not found")
	}
	if err := rows.Scan(&stored); err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Confidential Receiver", "+2348099887766",
		"cus_secret", "12 Secret Lane"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("trip projection stored restricted value %q: %s", secret, encoded)
		}
	}
}

// TestTripEventRejectionRollsBackEntireTransaction covers requirement 13's
// negative case and requirement 15.
//
// A trip event naming no canonical trip is refused. Nothing from that attempt
// — not the projection, not the history, and critically not the inbox entry —
// may survive, or the producer's redelivery would be swallowed as a duplicate
// of an event that was never applied.
func TestTripEventRejectionRollsBackEntireTransaction(t *testing.T) {
	fixture := newTripFixture(t, "Trip Rollback Logistics")

	payload := map[string]any{"requestId": fixture.requestID, "status": "assigned"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	event := mktusecases.InboundEvent{
		EventID: "evt_rollback_1", EventType: mktusecases.EventTripCreated,
		SchemaVersion: 1, SourceService: mktmodels.SourceDispatch,
		AggregateType: "request", AggregateID: fixture.requestID, AggregateVersion: 1,
		OccurredAt: time.Now().UTC(), PayloadHash: fleetevents.HashPayload(encoded),
		Payload: payload,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.IngestEvent(context.Background(), raw, event); err == nil {
		t.Fatal("a trip event with no canonical trip id was accepted")
	}

	var trips, inbox int
	if err := fixture.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_trip_projections`).Scan(&trips); err != nil {
		t.Fatal(err)
	}
	if trips != 0 {
		t.Fatalf("rejected event left %d trip projections behind", trips)
	}
	if err := fixture.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_inbox WHERE event_id = $1`,
		"evt_rollback_1").Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if inbox != 0 {
		t.Fatalf("rejected event left %d inbox rows; a redelivery would now be "+
			"swallowed as a duplicate of an event that never applied", inbox)
	}
}
