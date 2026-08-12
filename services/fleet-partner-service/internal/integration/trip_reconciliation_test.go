package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// gapTripFixture ingests a trip and then leaves it gapped, which is the state
// a real missed delivery produces.
func gapTripFixture(t *testing.T, name string) *tripFixture {
	t.Helper()
	fixture := newTripFixture(t, name)
	fixture.createTrip(t, nil)
	fixture.ingest(t, mktusecases.EventTripStatusChanged, 6,
		map[string]any{"status": "in_progress"})
	if trip := fixture.trip(t); trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("fixture setup: freshness = %q, want GAP_DETECTED", trip.FreshnessState)
	}
	return fixture
}

// tripCaseStatus reads the live or historical case for a trip.
func tripCaseStatus(t *testing.T, fixture *tripFixture) (string, string, int) {
	t.Helper()
	var status string
	var code *string
	var attempts int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT status, last_error_code, attempts
		FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_type = 'TRIP' AND aggregate_source_id = $1
		ORDER BY updated_at DESC LIMIT 1`, fixture.tripID).Scan(&status, &code, &attempts); err != nil {
		t.Fatalf("read trip case: %v", err)
	}
	if code == nil {
		return status, "", attempts
	}
	return status, *code, attempts
}

// TestTripReconciliationRepairsGappedProjection is the end-to-end path this
// work package exists to deliver: a projection populated by real ingested
// events, gapped by a real missed delivery, repaired by a read-back.
func TestTripReconciliationRepairsGappedProjection(t *testing.T) {
	fixture := gapTripFixture(t, "Trip Repair Logistics")

	completed := time.Now().UTC()
	stub := &stubExecution{
		configured: true,
		tripView: platformclients.SourceTripView{
			TripID: fixture.tripID, Status: "completed", SourceVersion: 9,
			CompletedAt: &completed,
		},
	}

	result, err := newReconciliationService(fixture.pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{})
	if err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}
	if result.Claimed != 1 || result.Resolved != 1 {
		t.Fatalf("claimed=%d resolved=%d, want 1 and 1 (%+v)",
			result.Claimed, result.Resolved, result)
	}

	trip := fixture.trip(t)
	if trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", trip.FreshnessState)
	}
	if trip.SourceVersion != 9 || trip.TripStatus != "completed" {
		t.Fatalf("projection not repaired: v%d %q", trip.SourceVersion, trip.TripStatus)
	}
	if trip.CompletedAt == nil {
		t.Fatal("completed_at not repaired from authoritative state")
	}
	if trip.LastReconciledAt == nil {
		t.Fatal("last_reconciled_at not set; the repair is not auditable")
	}

	// The request projection derives from the trip outcome and must not be
	// left contradicting the trip view the operator was just told to trust.
	if request := fixture.request(t); request.ProjectionStatus != mktmodels.ProjectionCompleted {
		t.Fatalf("request status = %q, want COMPLETED after trip repair",
			request.ProjectionStatus)
	}
	if status, _, _ := tripCaseStatus(t, fixture); status != mktmodels.ReconciliationResolved {
		t.Fatalf("case status = %q, want RESOLVED", status)
	}
}

// TestTripReconciliationSweepOpensCasesForSilentProjections proves
// reconciliation reaches ordinary production trips, not only those an
// ingest-time gap happened to flag.
func TestTripReconciliationSweepOpensCasesForSilentProjections(t *testing.T) {
	fixture := newTripFixture(t, "Trip Sweep Logistics")
	fixture.createTrip(t, nil)

	// A healthy projection that simply stopped hearing anything.
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE fleet_partner_trip_projections
		SET last_event_at = now() - interval '3 hours'
		WHERE platform_trip_id = $1`, fixture.tripID); err != nil {
		t.Fatal(err)
	}

	service := newReconciliationService(fixture.pool, &stubExecution{configured: false})

	// First pass marks it STALE; nothing is open yet because the sweep runs
	// after the case scan.
	if _, err := service.RunReconciliation(context.Background(),
		mktusecases.ReconciliationConfig{StaleAfter: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if trip := fixture.trip(t); trip.FreshnessState != mktmodels.FreshnessStale {
		t.Fatalf("freshness = %q, want STALE", trip.FreshnessState)
	}

	// Second pass sees the stale projection and enqueues a repair.
	result, err := service.RunReconciliation(context.Background(),
		mktusecases.ReconciliationConfig{StaleAfter: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result.CasesOpened != 1 {
		t.Fatalf("cases opened = %d, want 1 — a silent projection must be "+
			"examined, not left stale forever", result.CasesOpened)
	}
	if got := fixture.openCases(t); got != 1 {
		t.Fatalf("live cases = %d, want 1", got)
	}

	// Repeated sweeps must not pile up duplicate cases for one aggregate.
	if _, err := service.RunReconciliation(context.Background(),
		mktusecases.ReconciliationConfig{StaleAfter: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if got := fixture.openCases(t); got != 1 {
		t.Fatalf("live cases = %d after a second sweep, want 1", got)
	}
}

// TestTripReconciliationFailsClosedOnRetryableFailure covers the outage case.
func TestTripReconciliationFailsClosedOnRetryableFailure(t *testing.T) {
	fixture := gapTripFixture(t, "Trip Outage Logistics")
	stub := &stubExecution{configured: true, readErr: errors.New("dispatch unreachable")}

	result, err := newReconciliationService(fixture.pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Resolved != 0 || result.Deferred != 1 {
		t.Fatalf("resolved=%d deferred=%d, want 0 and 1", result.Resolved, result.Deferred)
	}

	trip := fixture.trip(t)
	if trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q; an outage must not be read as a repair",
			trip.FreshnessState)
	}
	if trip.SourceVersion != 6 {
		t.Fatalf("source_version = %d, want 6 unchanged", trip.SourceVersion)
	}
	status, code, attempts := tripCaseStatus(t, fixture)
	if status != mktmodels.ReconciliationOpen {
		t.Fatalf("case status = %q, want OPEN for retry", status)
	}
	if code != "source_read_failed" {
		t.Fatalf("case code = %q, want source_read_failed", code)
	}
	// Exactly one attempt per pass. Claiming through ResolveReconciliation
	// charged two, silently halving every retry budget.
	if attempts != 1 {
		t.Fatalf("attempts = %d after one pass, want 1", attempts)
	}
}

// TestTripReconciliationAuthoritativeNotFoundIsNotDestructive pins the most
// dangerous classification.
//
// A source reporting no such trip is indistinguishable from a read replica
// that has not caught up. One reading is destructive and irreversible, so the
// projection is never terminated on this evidence.
func TestTripReconciliationAuthoritativeNotFoundIsNotDestructive(t *testing.T) {
	fixture := gapTripFixture(t, "Trip NotFound Logistics")
	stub := &stubExecution{configured: true, readErr: platformclients.ErrNotConfigured}

	if _, err := newReconciliationService(fixture.pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{}); err != nil {
		t.Fatal(err)
	}

	trip := fixture.trip(t)
	if trip.CancelledAt != nil || trip.CompletedAt != nil {
		t.Fatal("a not-found read-back terminated the projection")
	}
	if trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED", trip.FreshnessState)
	}
	if status, _, _ := tripCaseStatus(t, fixture); status != mktmodels.ReconciliationOpen {
		t.Fatalf("case status = %q, want OPEN", status)
	}
}

// TestTripReconciliationDeadLettersAfterRetryLimit covers the exhaustion path.
func TestTripReconciliationDeadLettersAfterRetryLimit(t *testing.T) {
	fixture := gapTripFixture(t, "Trip DeadLetter Logistics")
	stub := &stubExecution{configured: true, readErr: errors.New("dispatch unreachable")}
	service := newReconciliationService(fixture.pool, stub)

	// Zero backoff so successive passes are immediately due.
	config := mktusecases.ReconciliationConfig{
		MaxAttempts: 3,
		BaseBackoff: time.Nanosecond,
		MaxBackoff:  time.Nanosecond,
	}
	for pass := 0; pass < 3; pass++ {
		if _, err := service.RunReconciliation(context.Background(), config); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	status, code, _ := tripCaseStatus(t, fixture)
	if status != mktmodels.ReconciliationFailed {
		t.Fatalf("case status = %q after exhausting retries, want FAILED_PERMANENT", status)
	}
	if code != "source_read_failed" {
		t.Fatalf("dead-letter code = %q, want source_read_failed", code)
	}

	// Dead-lettered cases must be visible to an operator.
	cases, err := mktrepos.NewTripRepository(fixture.pool).
		ListDeadLetteredCases(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].AggregateSourceID != fixture.tripID {
		t.Fatalf("dead-letter listing = %+v, want one case for %s", cases, fixture.tripID)
	}

	// The projection stays untrusted, which is the whole point of stopping.
	if trip := fixture.trip(t); trip.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness = %q, want GAP_DETECTED", trip.FreshnessState)
	}
}

// TestTripReconciliationOperatorReplay covers the recovery path out of the
// dead-letter queue.
func TestTripReconciliationOperatorReplay(t *testing.T) {
	fixture := gapTripFixture(t, "Trip Replay Logistics")
	failing := &stubExecution{configured: true, readErr: errors.New("dispatch unreachable")}
	config := mktusecases.ReconciliationConfig{
		MaxAttempts: 2, BaseBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond,
	}
	failingService := newReconciliationService(fixture.pool, failing)
	for pass := 0; pass < 2; pass++ {
		if _, err := failingService.RunReconciliation(context.Background(), config); err != nil {
			t.Fatal(err)
		}
	}
	if status, _, _ := tripCaseStatus(t, fixture); status != mktmodels.ReconciliationFailed {
		t.Fatalf("setup: case status = %q, want FAILED_PERMANENT", status)
	}

	// The operator fixes the underlying cause and replays.
	var caseID string
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT id FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_type = 'TRIP' AND aggregate_source_id = $1`,
		fixture.tripID).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	replayed, err := mktrepos.NewTripRepository(fixture.pool).
		ReplayReconciliationCase(context.Background(), fixture.pool, caseID)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed {
		t.Fatal("replay reported no case reopened")
	}
	if status, _, attempts := tripCaseStatus(t, fixture); status != mktmodels.ReconciliationOpen ||
		attempts != 0 {
		t.Fatalf("after replay status=%q attempts=%d, want OPEN and 0 — a replay "+
			"that kept its attempts would dead-letter again immediately", status, attempts)
	}

	// With a healthy source the replayed case now resolves.
	healthy := &stubExecution{
		configured: true,
		tripView: platformclients.SourceTripView{
			TripID: fixture.tripID, Status: "in_progress", SourceVersion: 11,
		},
	}
	if _, err := newReconciliationService(fixture.pool, healthy).
		RunReconciliation(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := tripCaseStatus(t, fixture); status != mktmodels.ReconciliationResolved {
		t.Fatalf("case status = %q after replay, want RESOLVED", status)
	}
	if trip := fixture.trip(t); trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", trip.FreshnessState)
	}
}

// TestTripReconciliationLeaseRecovery proves an abandoned case returns to the
// queue rather than being stranded IN_PROGRESS forever.
func TestTripReconciliationLeaseRecovery(t *testing.T) {
	fixture := gapTripFixture(t, "Trip Lease Logistics")

	// Simulate a worker that claimed the case and then died: IN_PROGRESS with
	// a lease that has since expired.
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE fleet_partner_projection_reconciliation_cases
		SET status = 'IN_PROGRESS', lease_owner = 'crashed-worker',
			lease_expires_at = now() - interval '5 minutes'
		WHERE aggregate_type = 'TRIP' AND aggregate_source_id = $1`,
		fixture.tripID); err != nil {
		t.Fatal(err)
	}

	stub := &stubExecution{
		configured: true,
		tripView: platformclients.SourceTripView{
			TripID: fixture.tripID, Status: "in_progress", SourceVersion: 8,
		},
	}
	result, err := newReconciliationService(fixture.pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recovered != 1 {
		t.Fatalf("recovered = %d, want 1 — an abandoned lease must return to "+
			"the queue", result.Recovered)
	}
	if result.Resolved != 1 {
		t.Fatalf("resolved = %d, want 1; the recovered case should be repaired "+
			"in the same pass", result.Resolved)
	}
	if trip := fixture.trip(t); trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", trip.FreshnessState)
	}
}

// TestTripReconciliationNeverMovesProjectionBackwards guards against a lagging
// replica undoing events already applied.
func TestTripReconciliationNeverMovesProjectionBackwards(t *testing.T) {
	fixture := gapTripFixture(t, "Trip Lagging Logistics")
	stub := &stubExecution{
		configured: true,
		tripView: platformclients.SourceTripView{
			TripID: fixture.tripID, Status: "assigned", SourceVersion: 2,
		},
	}

	if _, err := newReconciliationService(fixture.pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{}); err != nil {
		t.Fatal(err)
	}

	trip := fixture.trip(t)
	if trip.SourceVersion != 6 {
		t.Fatalf("source_version = %d, want 6 — a stale read-back moved the "+
			"projection backwards", trip.SourceVersion)
	}
	if trip.TripStatus != "in_progress" {
		t.Fatalf("trip_status = %q, want in_progress unchanged", trip.TripStatus)
	}
	// The aggregate was reachable and we are not behind it, so the gap closes.
	if trip.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness = %q, want CURRENT", trip.FreshnessState)
	}
}
