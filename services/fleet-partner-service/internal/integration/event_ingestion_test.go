package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// newIngestionService builds a marketplace service for ingestion tests.
func newIngestionService(pool *pgxpool.Pool) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		// Trips and Commands match what the composition root wires. Omitting
		// Trips here is what let the production nil-wiring defect survive: the
		// fixture exercised a service shape no deployment actually runs.
		Trips:    mktrepos.NewTripRepository(pool),
		Commands: mktrepos.NewCommandRepository(pool),
		// The durable gate is wired here for the same reason Trips is: a
		// fixture that runs a service shape no deployment runs proves nothing
		// about the deployment.
		DurableEligibility: newEligibilityService(pool, healthyAuthority("", "")),
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:            observability.NewMetrics(),
	})
}

// inboundEvent builds a well-formed envelope with a correct payload hash.
func inboundEvent(t *testing.T, eventID string, eventType string, requestID string,
	version int64, payload map[string]any) ([]byte, mktusecases.InboundEvent) {
	t.Helper()
	if payload == nil {
		payload = map[string]any{}
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
		AggregateType:    "request",
		AggregateID:      requestID,
		AggregateVersion: version,
		OccurredAt:       time.Now().UTC(),
		CorrelationID:    "corr-" + eventID,
		PayloadHash:      events.HashPayload(encoded),
		Payload:          payload,
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return raw, event
}

// TestIngestEventAppliesRequestReserved proves the happy path commits.
func TestIngestEventAppliesRequestReserved(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Reserve Org", "RC-ING-1")
	projection := seedProjection(t, pool, org.ID, "req_ingest_reserve",
		mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_reserve_1", mktusecases.EventRequestReserved,
		"req_ingest_reserve", 2, map[string]any{"status": "reserved"})

	result, err := service.IngestEvent(ctx, raw, event)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Status != mktusecases.IngestAccepted {
		t.Fatalf("status = %q, want ACCEPTED", result.Status)
	}

	repo := mktrepos.NewProjectionRepository(pool)
	stored, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.ProjectionStatus != mktmodels.ProjectionReservationPending {
		t.Fatalf("projection status = %q, want RESERVATION_PENDING", stored.ProjectionStatus)
	}
	if stored.SourceVersion != 2 {
		t.Fatalf("source version = %d, want 2", stored.SourceVersion)
	}
	if stored.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", stored.FreshnessState)
	}
	if stored.LastEventID == nil || *stored.LastEventID != "evt_reserve_1" {
		t.Fatalf("last event id = %v, want evt_reserve_1", stored.LastEventID)
	}
}

// TestIngestEventRedeliveryIsDuplicate proves at-least-once delivery is safe.
//
// This is the property the producer's UNKNOWN-outcome path depends on: after a
// lost response it redelivers the SAME event id, and that must acknowledge
// without applying the change a second time.
func TestIngestEventRedeliveryIsDuplicate(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Dup Org", "RC-ING-2")
	projection := seedProjection(t, pool, org.ID, "req_ingest_dup",
		mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_dup_1", mktusecases.EventRequestReserved,
		"req_ingest_dup", 2, map[string]any{"status": "reserved"})

	if _, err := service.IngestEvent(ctx, raw, event); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	repo := mktrepos.NewProjectionRepository(pool)
	afterFirst, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload after first: %v", err)
	}

	result, err := service.IngestEvent(ctx, raw, event)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if result.Status != mktusecases.IngestDuplicate {
		t.Fatalf("status = %q, want DUPLICATE", result.Status)
	}

	afterSecond, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload after second: %v", err)
	}
	if afterSecond.Version != afterFirst.Version {
		t.Fatalf("redelivery mutated projection: version %d -> %d",
			afterFirst.Version, afterSecond.Version)
	}
}

// TestIngestEventPayloadHashMismatchRejected proves a tampered payload under a
// known event id is refused rather than applied.
func TestIngestEventPayloadHashMismatchRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Hash Org", "RC-ING-3")
	seedProjection(t, pool, org.ID, "req_ingest_hash", mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_hash_1", mktusecases.EventRequestReserved,
		"req_ingest_hash", 2, map[string]any{"status": "reserved"})
	// Same declared hash, different payload — exactly what an in-flight
	// modification looks like.
	event.Payload = map[string]any{"status": "cancelled"}

	_, err := service.IngestEvent(ctx, raw, event)
	var ingestErr *mktusecases.IngestError
	if !errors.As(err, &ingestErr) {
		t.Fatalf("error = %v, want *IngestError", err)
	}
	if ingestErr.Code != "payload_hash_mismatch" {
		t.Fatalf("code = %q, want payload_hash_mismatch", ingestErr.Code)
	}
}

// TestIngestEventSameIDDifferentPayloadRejected proves the inbox refuses to let
// one event id carry two different payloads.
func TestIngestEventSameIDDifferentPayloadRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Swap Org", "RC-ING-4")
	seedProjection(t, pool, org.ID, "req_ingest_swap", mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_swap_1", mktusecases.EventRequestReserved,
		"req_ingest_swap", 2, map[string]any{"status": "reserved"})
	if _, err := service.IngestEvent(ctx, raw, event); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	rawTwo, eventTwo := inboundEvent(t, "evt_swap_1", mktusecases.EventRequestReserved,
		"req_ingest_swap", 3, map[string]any{"status": "something_else"})
	_, err := service.IngestEvent(ctx, rawTwo, eventTwo)
	var ingestErr *mktusecases.IngestError
	if !errors.As(err, &ingestErr) {
		t.Fatalf("error = %v, want *IngestError", err)
	}
	if ingestErr.Code != "payload_hash_mismatch" {
		t.Fatalf("code = %q, want payload_hash_mismatch", ingestErr.Code)
	}
}

// TestIngestEventStaleVersionIgnored proves an out-of-order older event cannot
// move a projection backwards.
func TestIngestEventStaleVersionIgnored(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Stale Org", "RC-ING-5")
	projection := seedProjection(t, pool, org.ID, "req_ingest_stale",
		mktmodels.ProjectionOffered)

	// Both events are REQUEST-aggregate events, because only they share the
	// request projection's version counter. This test previously advanced the
	// projection with trip.completed.v1 at version 5 and then sent a request
	// event at version 3 — which read as a sequence only while the service
	// wrongly fed trip versions into the request counter. Trip-side staleness
	// is covered separately, against the trip projection's own sequence.
	raw, event := inboundEvent(t, "evt_stale_new", mktusecases.EventRequestCancelled,
		"req_ingest_stale", 5, map[string]any{"status": "cancelled"})
	if _, err := service.IngestEvent(ctx, raw, event); err != nil {
		t.Fatalf("newer ingest: %v", err)
	}

	// Version 3 arrives after version 5. Applying it would regress the
	// projection from CANCELLED back to RESERVATION_PENDING.
	rawOld, eventOld := inboundEvent(t, "evt_stale_old", mktusecases.EventRequestReserved,
		"req_ingest_stale", 3, map[string]any{"status": "reserved"})
	result, err := service.IngestEvent(ctx, rawOld, eventOld)
	if err != nil {
		t.Fatalf("older ingest: %v", err)
	}
	if result.Status != mktusecases.IngestIgnored {
		t.Fatalf("status = %q, want IGNORED", result.Status)
	}
	if result.Code != "stale_source_version" {
		t.Fatalf("code = %q, want stale_source_version", result.Code)
	}

	repo := mktrepos.NewProjectionRepository(pool)
	stored, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.ProjectionStatus != mktmodels.ProjectionCancelled {
		t.Fatalf("projection regressed to %q", stored.ProjectionStatus)
	}
	if stored.SourceVersion != 5 {
		t.Fatalf("source version = %d, want 5", stored.SourceVersion)
	}
}

// TestIngestEventGapMarksGapDetected proves a missing version is detected and
// blocks state-changing commands rather than being silently accepted.
func TestIngestEventGapMarksGapDetected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Gap Org", "RC-ING-6")
	projection := seedProjection(t, pool, org.ID, "req_ingest_gap",
		mktmodels.ProjectionOffered)

	// Seeded at source version 1; version 2 never arrives.
	raw, event := inboundEvent(t, "evt_gap_1", mktusecases.EventRequestReserved,
		"req_ingest_gap", 4, map[string]any{"status": "reserved"})
	result, err := service.IngestEvent(ctx, raw, event)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Status != mktusecases.IngestAccepted {
		t.Fatalf("status = %q, want ACCEPTED", result.Status)
	}

	repo := mktrepos.NewProjectionRepository(pool)
	stored, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED", stored.FreshnessState)
	}
	if !mktmodels.BlocksStateChangingCommands(stored.FreshnessState) {
		t.Fatal("GAP_DETECTED must block state-changing commands")
	}
}

// TestIngestEventUnknownTypeRejectedPermanently proves an unrecognised type is a
// permanent rejection, so the producer dead-letters instead of retrying forever.
func TestIngestEventUnknownTypeRejectedPermanently(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Unknown Org", "RC-ING-7")
	seedProjection(t, pool, org.ID, "req_ingest_unknown", mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_unknown_1", "platform.request.invented.v9",
		"req_ingest_unknown", 2, nil)

	_, err := service.IngestEvent(ctx, raw, event)
	var ingestErr *mktusecases.IngestError
	if !errors.As(err, &ingestErr) {
		t.Fatalf("error = %v, want *IngestError", err)
	}
	if ingestErr.Code != "unknown_event_type" {
		t.Fatalf("code = %q, want unknown_event_type", ingestErr.Code)
	}
}

// TestIngestEventUnknownProjectionIgnored proves an event for a request this
// organization never saw is acknowledged without inventing local state.
func TestIngestEventUnknownProjectionIgnored(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)

	raw, event := inboundEvent(t, "evt_orphan_1", mktusecases.EventRequestReserved,
		"req_never_projected", 2, map[string]any{"status": "reserved"})

	result, err := service.IngestEvent(ctx, raw, event)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Status != mktusecases.IngestIgnored {
		t.Fatalf("status = %q, want IGNORED", result.Status)
	}
	if result.Code != "projection_not_found" {
		t.Fatalf("code = %q, want projection_not_found", result.Code)
	}
}

// TestIngestEventTripCreatedRecordsCanonicalTrip proves the canonical trip id
// reported by the source lands on the projection.
func TestIngestEventTripCreatedRecordsCanonicalTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)
	org := createOrganization(t, pool, "Ingest Trip Org", "RC-ING-8")
	projection := seedProjection(t, pool, org.ID, "req_ingest_trip",
		mktmodels.ProjectionOffered)

	raw, event := inboundEvent(t, "evt_trip_1", mktusecases.EventTripCreated,
		"req_ingest_trip", 2, map[string]any{
			"tripId":    "trip_canonical_1",
			"requestId": "req_ingest_trip",
			"status":    "assigned",
		})

	if _, err := service.IngestEvent(ctx, raw, event); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	repo := mktrepos.NewProjectionRepository(pool)
	stored, err := repo.GetByID(ctx, org.ID, projection.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.PlatformTripID == nil || *stored.PlatformTripID != "trip_canonical_1" {
		t.Fatalf("platform trip id = %v, want trip_canonical_1", stored.PlatformTripID)
	}
	if stored.ProjectionStatus != mktmodels.ProjectionExecutionCreated {
		t.Fatalf("projection status = %q, want EXECUTION_CREATED", stored.ProjectionStatus)
	}
}

// TestIngestEventInvalidEnvelopeRejected covers the envelope validation rules.
func TestIngestEventInvalidEnvelopeRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	service := newIngestionService(pool)

	cases := []struct {
		name    string
		mutate  func(*mktusecases.InboundEvent)
		wantErr string
	}{
		{"missing event id", func(e *mktusecases.InboundEvent) { e.EventID = "" }, "invalid_envelope"},
		{"missing event type", func(e *mktusecases.InboundEvent) { e.EventType = "" }, "invalid_envelope"},
		{"zero schema version", func(e *mktusecases.InboundEvent) { e.SchemaVersion = 0 }, "schema_version_invalid"},
		{"missing source service", func(e *mktusecases.InboundEvent) { e.SourceService = "" }, "invalid_envelope"},
		{"missing aggregate id", func(e *mktusecases.InboundEvent) { e.AggregateID = "" }, "invalid_envelope"},
		{"negative version", func(e *mktusecases.InboundEvent) { e.AggregateVersion = -1 }, "invalid_envelope"},
		{"zero occurred at", func(e *mktusecases.InboundEvent) { e.OccurredAt = time.Time{} }, "invalid_envelope"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			raw, event := inboundEvent(t, "evt_invalid", mktusecases.EventRequestReserved,
				"req_invalid", 2, nil)
			testCase.mutate(&event)

			_, err := service.IngestEvent(ctx, raw, event)
			var ingestErr *mktusecases.IngestError
			if !errors.As(err, &ingestErr) {
				t.Fatalf("error = %v, want *IngestError", err)
			}
			if ingestErr.Code != testCase.wantErr {
				t.Fatalf("code = %q, want %q", ingestErr.Code, testCase.wantErr)
			}
		})
	}
}
