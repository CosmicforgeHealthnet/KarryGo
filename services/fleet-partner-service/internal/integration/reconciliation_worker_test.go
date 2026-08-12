package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// stubExecution is a dispatch execution contract with scripted read-backs.
//
// Only the two read methods this worker uses are given behaviour; every other
// method embeds UnimplementedExecution, which fails closed. That means a
// worker that started calling something new fails loudly here rather than
// silently getting a zero value.
type stubExecution struct {
	platformclients.UnimplementedExecution
	configured        bool
	requestView       platformclients.SourceRequestView
	tripView          platformclients.SourceTripView
	readErr           error
	requestementCalls int
}

func (s *stubExecution) Configured() bool { return s.configured }

func (s *stubExecution) ReadRequest(context.Context, platformclients.CallMeta,
	string) (platformclients.SourceRequestView, error) {
	s.requestementCalls++
	if s.readErr != nil {
		return platformclients.SourceRequestView{}, s.readErr
	}
	return s.requestView, nil
}

func (s *stubExecution) ReadTrip(context.Context, platformclients.CallMeta,
	string) (platformclients.SourceTripView, error) {
	if s.readErr != nil {
		return platformclients.SourceTripView{}, s.readErr
	}
	return s.tripView, nil
}

// newReconciliationService builds a marketplace service backed by the stub.
func newReconciliationService(pool *pgxpool.Pool,
	execution platformclients.ExecutionContract) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		Pool:        pool,
		Projections: mktrepos.NewProjectionRepository(pool),
		Offers:      mktrepos.NewOfferRepository(pool),
		Assignments: mktrepos.NewAssignmentRepository(pool),
		Trips:       mktrepos.NewTripRepository(pool),
		Commands:    mktrepos.NewCommandRepository(pool),
		Executions: platformclients.ExecutionSet{
			platformclients.CategoryDispatch: execution,
		},
	})
}

// gapProjection seeds a projection that a missed event left gapped.
func gapProjection(t *testing.T, pool *pgxpool.Pool, organizationID string,
	requestID string) *mktmodels.RequestProjection {
	t.Helper()
	projection := seedProjection(t, pool, organizationID, requestID, mktmodels.ProjectionOffered)
	if _, err := pool.Exec(context.Background(), `
		UPDATE fleet_partner_marketplace_request_projections
		SET freshness_state = 'GAP_DETECTED', source_version = 2
		WHERE id = $1`, projection.ID); err != nil {
		t.Fatalf("mark projection gapped: %v", err)
	}
	reloaded, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organizationID, projection.ID)
	if err != nil {
		t.Fatalf("reload gapped projection: %v", err)
	}
	return reloaded
}

// openCase records a reconciliation case for an aggregate.
func openCase(t *testing.T, pool *pgxpool.Pool, organizationID string,
	aggregateType string, sourceID string) {
	t.Helper()
	trips := mktrepos.NewTripRepository(pool)
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		return trips.OpenReconciliation(context.Background(), tx, organizationID,
			mktmodels.SourceDispatch, aggregateType, sourceID, "gap_detected",
			time.Now().UTC().Add(-time.Minute))
	}); err != nil {
		t.Fatalf("open reconciliation case: %v", err)
	}
}

// caseStatus reads a case's status and error code.
func caseStatus(t *testing.T, pool *pgxpool.Pool, sourceID string) (string, string) {
	t.Helper()
	var status string
	var code *string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, last_error_code
		FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_source_id = $1`, sourceID).Scan(&status, &code); err != nil {
		t.Fatalf("read reconciliation case: %v", err)
	}
	if code == nil {
		return status, ""
	}
	return status, *code
}

// TestReconciliationRepairsGappedProjectionFromSource is the happy path: a
// read-back closes a hole that events could never have closed on their own.
func TestReconciliationRepairsGappedProjectionFromSource(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Reconcile Logistics", "RC-"+ids.New("reg"))
	requestID := ids.New("req")
	projection := gapProjection(t, pool, organization.ID, requestID)

	stub := &stubExecution{
		configured: true,
		requestView: platformclients.SourceRequestView{
			RequestID: requestID, Status: "cancelled", SourceVersion: 9,
		},
	}
	openCase(t, pool, organization.ID, mktmodels.ReconciliationAggregateRequest, requestID)

	result, err := newReconciliationService(pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{})
	if err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}
	if result.Claimed != 1 || result.Resolved != 1 {
		t.Fatalf("claimed=%d resolved=%d, want 1 and 1 (%+v)", result.Claimed, result.Resolved, result)
	}

	repaired, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organization.ID, projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness=%s, want CURRENT — the gap was not cleared", repaired.FreshnessState)
	}
	if repaired.SourceVersion != 9 {
		t.Fatalf("source_version=%d, want 9 from the read-back", repaired.SourceVersion)
	}
	if repaired.ProjectionStatus != mktmodels.ProjectionCancelled {
		t.Fatalf("projection_status=%s, want CANCELLED", repaired.ProjectionStatus)
	}
	if repaired.LastReconciledAt == nil {
		t.Fatal("last_reconciled_at not set; the repair is not auditable")
	}
	if status, _ := caseStatus(t, pool, requestID); status != mktmodels.ReconciliationResolved {
		t.Fatalf("case status=%s, want RESOLVED", status)
	}
}

// TestReconciliationFailsClosedWhenSourceUnavailable is the property that
// matters most here.
//
// An unreachable source tells us nothing. The projection must stay gapped —
// still refusing state-changing commands — rather than being quietly declared
// current, which would be the one outcome worse than staying broken.
func TestReconciliationFailsClosedWhenSourceUnavailable(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Outage Logistics", "RC-"+ids.New("reg"))
	requestID := ids.New("req")
	projection := gapProjection(t, pool, organization.ID, requestID)

	stub := &stubExecution{configured: true, readErr: errors.New("dispatch unreachable")}
	openCase(t, pool, organization.ID, mktmodels.ReconciliationAggregateRequest, requestID)

	result, err := newReconciliationService(pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{})
	if err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}
	if result.Resolved != 0 || result.Deferred != 1 {
		t.Fatalf("resolved=%d deferred=%d, want 0 and 1 (%+v)",
			result.Resolved, result.Deferred, result)
	}

	unchanged, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organization.ID, projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness=%s, want GAP_DETECTED — an outage must not be "+
			"mistaken for a successful repair", unchanged.FreshnessState)
	}
	if unchanged.SourceVersion != 2 {
		t.Fatalf("source_version=%d, want 2 unchanged", unchanged.SourceVersion)
	}
	status, code := caseStatus(t, pool, requestID)
	if status != mktmodels.ReconciliationOpen {
		t.Fatalf("case status=%s, want OPEN for retry", status)
	}
	if code != "source_read_failed" {
		t.Fatalf("case error code=%q, want source_read_failed", code)
	}
}

// TestReconciliationFailsClosedWhenClientUnconfigured covers the deployment
// mistake rather than the outage: no dispatch client at all.
func TestReconciliationFailsClosedWhenClientUnconfigured(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Unconfigured Logistics", "RC-"+ids.New("reg"))
	requestID := ids.New("req")
	projection := gapProjection(t, pool, organization.ID, requestID)

	stub := &stubExecution{configured: false}
	openCase(t, pool, organization.ID, mktmodels.ReconciliationAggregateRequest, requestID)

	if _, err := newReconciliationService(pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{}); err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}
	if stub.requestementCalls != 0 {
		t.Fatalf("an unconfigured client was called %d times", stub.requestementCalls)
	}

	unchanged, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organization.ID, projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.FreshnessState != mktmodels.FreshnessGapDetected {
		t.Fatalf("freshness=%s, want GAP_DETECTED", unchanged.FreshnessState)
	}
	if status, code := caseStatus(t, pool, requestID); status != mktmodels.ReconciliationOpen ||
		code != "source_client_not_configured" {
		t.Fatalf("case status=%s code=%q, want OPEN/source_client_not_configured", status, code)
	}
}

// TestReconciliationNeverMovesProjectionBackwards guards against a lagging
// read replica undoing events we already applied.
func TestReconciliationNeverMovesProjectionBackwards(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Lagging Logistics", "RC-"+ids.New("reg"))
	requestID := ids.New("req")
	projection := gapProjection(t, pool, organization.ID, requestID)

	// The read-back answers with an OLDER version than the projection holds.
	stub := &stubExecution{
		configured: true,
		requestView: platformclients.SourceRequestView{
			RequestID: requestID, Status: "broadcasting", SourceVersion: 1,
		},
	}
	openCase(t, pool, organization.ID, mktmodels.ReconciliationAggregateRequest, requestID)

	if _, err := newReconciliationService(pool, stub).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{}); err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}

	repaired, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organization.ID, projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.SourceVersion != 2 {
		t.Fatalf("source_version=%d, want 2 — a stale read-back moved the "+
			"projection backwards", repaired.SourceVersion)
	}
	// The aggregate was reachable and we are not behind it, so the gap closes
	// even though no state was copied across.
	if repaired.FreshnessState != mktmodels.FreshnessCurrent {
		t.Fatalf("freshness=%s, want CURRENT", repaired.FreshnessState)
	}
}

// TestStaleSweepFlagsSilentProjections covers the sweep half of the pass.
func TestStaleSweepFlagsSilentProjections(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Silent Logistics", "RC-"+ids.New("reg"))
	projection := seedProjection(t, pool, organization.ID, ids.New("req"),
		mktmodels.ProjectionOffered)

	// Backdate the last source event well past the threshold.
	if _, err := pool.Exec(context.Background(), `
		UPDATE fleet_partner_marketplace_request_projections
		SET last_source_event_at = now() - interval '2 hours'
		WHERE id = $1`, projection.ID); err != nil {
		t.Fatal(err)
	}

	result, err := newReconciliationService(pool, &stubExecution{configured: true}).
		RunReconciliation(context.Background(), mktusecases.ReconciliationConfig{
			StaleAfter: time.Minute,
		})
	if err != nil {
		t.Fatalf("run reconciliation: %v", err)
	}
	if result.StaleRequests != 1 {
		t.Fatalf("stale requests=%d, want 1", result.StaleRequests)
	}

	swept, err := mktrepos.NewProjectionRepository(pool).
		GetByID(context.Background(), organization.ID, projection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if swept.FreshnessState != mktmodels.FreshnessStale {
		t.Fatalf("freshness=%s, want STALE", swept.FreshnessState)
	}
}
