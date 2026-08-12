package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
)

type syncAuthority struct {
	version int64
	status  string
}

func (a syncAuthority) Read(context.Context, string, string) (synchronization.AuthoritativeState, error) {
	raw, _ := json.Marshal(events.ContractPayload{SubjectReference: "provider_1", SourceVersion: a.version, Status: a.status})
	return synchronization.AuthoritativeState{SourceVersion: a.version, SafeState: raw}, nil
}

func syncEnvelope(t *testing.T, eventID string, version int64, status string) events.Envelope {
	t.Helper()
	payload, _ := json.Marshal(events.ContractPayload{SubjectReference: "provider_1", SourceVersion: version, Status: status})
	return events.Envelope{EventID: eventID, EventType: "provider.availability.changed.v1", SchemaVersion: 1, AggregateType: "provider", AggregateID: "provider_1", AggregateVersion: version, OrganizationID: "", CorrelationID: "cor_sync", OccurredAt: time.Now().UTC(), Producer: "driver-dispatch-delivery-service", Payload: payload}
}

func TestSection26VersionConflictGapReconciliationAndSafeOperatorViews(t *testing.T) {
	f := newSearchFixture(t)
	service := synchronization.New(f.pool, synchronization.Flags{Enabled: true, WorkerEnabled: true})
	apply := func(envelope events.Envelope) {
		envelope.OrganizationID = f.orgID
		if err := database.WithinTx(context.Background(), f.pool, func(tx pgx.Tx) error { return service.ApplyTx(context.Background(), tx, envelope) }); err != nil {
			t.Fatalf("apply %s: %v", envelope.EventID, err)
		}
	}
	apply(syncEnvelope(t, "evt_sync_1", 1, "AVAILABLE"))
	apply(syncEnvelope(t, "evt_sync_conflict", 1, "UNAVAILABLE"))
	var state string
	var version int64
	if err := f.pool.QueryRow(context.Background(), `SELECT gap_conflict_state,source_version FROM fleet_partner_synchronization_projections WHERE organization_id=$1 AND subject_reference='provider_1'`, f.orgID).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "CONFLICT" || version != 1 {
		t.Fatalf("same-version conflict state=%s version=%d", state, version)
	}
	apply(syncEnvelope(t, "evt_sync_gap", 3, "UNAVAILABLE"))
	apply(syncEnvelope(t, "evt_sync_lower", 2, "AVAILABLE"))
	if err := f.pool.QueryRow(context.Background(), `SELECT gap_conflict_state,source_version FROM fleet_partner_synchronization_projections WHERE organization_id=$1 AND subject_reference='provider_1'`, f.orgID).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "GAP" || version != 3 {
		t.Fatalf("gap/lower-version guard state=%s version=%d", state, version)
	}
	service.RegisterAuthority("provider.availability.changed.v1", syncAuthority{version: 4, status: "AVAILABLE"})
	processed, err := service.RunReconciliationPass(context.Background(), synchronization.WorkerConfig{Owner: "test-sync", BatchSize: 5, MaxAttempts: 3, Lease: time.Minute})
	if err != nil || processed != 1 {
		t.Fatalf("reconcile processed=%d err=%v", processed, err)
	}
	projections, err := service.ListProjections(context.Background(), f.orgID)
	if err != nil || len(projections) != 1 {
		t.Fatalf("safe projections=%v err=%v", projections, err)
	}
	encoded, _ := json.Marshal(projections)
	for _, forbidden := range []string{"password", "token", "credential", "privateMessage", "bankDetails"} {
		if json.Valid(encoded) && containsFold(string(encoded), forbidden) {
			t.Fatalf("operator projection leaks %s: %s", forbidden, encoded)
		}
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT gap_conflict_state,source_version FROM fleet_partner_synchronization_projections WHERE organization_id=$1 AND subject_reference='provider_1'`, f.orgID).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "CURRENT" || version != 4 {
		t.Fatalf("authoritative repair state=%s version=%d", state, version)
	}
}

func TestSection26DeadLetterReplayRequeuesAndAttemptsAreAppendOnly(t *testing.T) {
	f := newSearchFixture(t)
	service := synchronization.New(f.pool, synchronization.Flags{Enabled: true, WorkerEnabled: true})
	envelope := syncEnvelope(t, "evt_sync_dead_letter", 3, "UNAVAILABLE")
	envelope.OrganizationID = f.orgID
	if err := database.WithinTx(context.Background(), f.pool, func(tx pgx.Tx) error {
		return service.ApplyTx(context.Background(), tx, envelope)
	}); err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunReconciliationPass(context.Background(), synchronization.WorkerConfig{Owner: "test-sync", BatchSize: 5, MaxAttempts: 1, Lease: time.Minute})
	if err != nil || processed != 1 {
		t.Fatalf("dead-letter pass processed=%d err=%v", processed, err)
	}
	var deadLetterID, projectionID, caseID string
	if err = f.pool.QueryRow(context.Background(), `SELECT d.id,p.id,c.id FROM fleet_partner_synchronization_dead_letters d JOIN fleet_partner_synchronization_projections p ON p.organization_id=d.organization_id AND p.projection_type=d.event_type AND p.source_service=d.source_service AND p.subject_reference=d.subject_reference JOIN fleet_partner_synchronization_cases c ON c.projection_id=p.id WHERE d.organization_id=$1 AND d.status='OPEN' AND c.status='DEAD_LETTER'`, f.orgID).Scan(&deadLetterID, &projectionID, &caseID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(context.Background(), `UPDATE fleet_partner_synchronization_attempts SET outcome='TAMPERED' WHERE projection_id=$1`, projectionID); err == nil {
		t.Fatal("synchronization attempt history accepted an update")
	}
	if err = service.Replay(context.Background(), f.orgID, deadLetterID); err != nil {
		t.Fatal(err)
	}
	var deadStatus, caseStatus, reconciliationStatus string
	if err = f.pool.QueryRow(context.Background(), `SELECT d.status,c.status,p.reconciliation_status FROM fleet_partner_synchronization_dead_letters d JOIN fleet_partner_synchronization_cases c ON c.id=$2 JOIN fleet_partner_synchronization_projections p ON p.id=$3 WHERE d.id=$1`, deadLetterID, caseID, projectionID).Scan(&deadStatus, &caseStatus, &reconciliationStatus); err != nil {
		t.Fatal(err)
	}
	if deadStatus != "REPLAYED" || caseStatus != "OPEN" || reconciliationStatus != "PENDING" {
		t.Fatalf("replay state dead=%s case=%s projection=%s", deadStatus, caseStatus, reconciliationStatus)
	}
}

func containsFold(value, needle string) bool {
	return len(needle) > 0 && strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}
