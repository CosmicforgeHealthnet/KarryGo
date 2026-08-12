package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type executionPortStub struct {
	result privateusecases.ExecutionOutcome
	err    error
	calls  int
}

type privateEligibilityAlways struct{}

func (privateEligibilityAlways) Validate(context.Context, string, string, string, string, string) (bool, error) {
	return true, nil
}

func (s *executionPortStub) Submit(context.Context, tenant.Context, models.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	s.calls++
	return s.result, s.err
}
func (s *executionPortStub) ReadBack(context.Context, tenant.Context, models.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	s.calls++
	return s.result, s.err
}
func (s *executionPortStub) Cancel(context.Context, tenant.Context, models.ExecutionOperation, string) (privateusecases.ExecutionOutcome, error) {
	s.calls++
	return s.result, s.err
}
func (s *executionPortStub) ReadCancellation(context.Context, tenant.Context, models.ExecutionOperation) (privateusecases.ExecutionOutcome, error) {
	s.calls++
	return s.result, s.err
}

func privateWorkerFixture(t *testing.T, operationID string) (*shipmentsFixture,
	*privateusecases.Service, *privaterepos.Repository, string) {
	t.Helper()
	return privateWorkerFixtureOnPool(t, testPool(t), operationID)
}

func privateWorkerFixtureOnPool(t *testing.T, pool *pgxpool.Pool, operationID string) (*shipmentsFixture,
	*privateusecases.Service, *privaterepos.Repository, string) {
	t.Helper()
	f := newShipmentsFixtureOnPool(t, pool)
	ctx := context.Background()
	customer := f.customer(t, "Private Worker "+operationID)
	item := f.activeCatalogItem(t, "WORKER_"+operationID)
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	assignmentID := "pas_" + operationID
	insertPrivateExecutionFixture(t, f, order.ID, shipment.ID, assignmentID, operationID)
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET
		status='PENDING',completed_at=NULL,source_version=0,next_attempt_at=now() WHERE id=$1`, operationID); err != nil {
		t.Fatal(err)
	}
	repo := privaterepos.NewRepository(f.pool)
	service := privateusecases.NewService(privateusecases.Options{Pool: f.pool, Repository: repo,
		Audit: audit.NewRecorder(), Flags: privateusecases.Flags{Enabled: true, WritesEnabled: true,
			WorkerEnabled: true, DispatchEnabled: true}})
	return f, service, repo, assignmentID
}

func TestPrivateExecutionWorker_ConfirmsAtomicallyAndDoesNotDuplicate(t *testing.T) {
	f, service, repo, assignmentID := privateWorkerFixture(t, "pex_worker_confirm")
	port := &executionPortStub{result: privateusecases.ExecutionOutcome{Definite: true,
		Confirmed: true, CanonicalRequestReference: "request-worker", CanonicalTripReference: "trip-worker",
		NormalizedStatus: "CONFIRMED", SourceVersion: 1}}
	result, err := service.RunExecutionPass(context.Background(), port, privateusecases.ExecutionWorkerConfig{
		Owner: "worker-1", Lease: 3 * time.Second, BatchSize: 10, MaxAttempts: 3})
	if err != nil || result.Confirmed != 1 || port.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, port.calls, err)
	}
	op, err := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, "pex_worker_confirm")
	if err != nil || op.Status != models.ExecutionConfirmed || op.CanonicalTripReference != "trip-worker" {
		t.Fatalf("operation=%+v err=%v", op, err)
	}
	assignment, err := repo.GetAssignment(context.Background(), nil, f.tenant.OrganizationID, assignmentID)
	if err != nil || assignment.Status != models.StatusReady {
		t.Fatalf("assignment=%+v err=%v", assignment, err)
	}
	result, err = service.RunExecutionPass(context.Background(), port, privateusecases.ExecutionWorkerConfig{
		Owner: "worker-1", Lease: 3 * time.Second, BatchSize: 10, MaxAttempts: 3})
	if err != nil || result.Claimed != 0 || port.calls != 1 {
		t.Fatalf("duplicate pass result=%+v calls=%d err=%v", result, port.calls, err)
	}
}

func TestPrivateExecutionWorker_DeadLetterCreatesCaseAndSafeReplay(t *testing.T) {
	f, service, repo, _ := privateWorkerFixture(t, "pex_worker_dead")
	port := &executionPortStub{err: &platformclients.Error{Service: "dispatch",
		Outcome: platformclients.OutcomeValidation, Status: 422}}
	result, err := service.RunExecutionPass(context.Background(), port, privateusecases.ExecutionWorkerConfig{
		Owner: "worker-dead", Lease: 3 * time.Second, BatchSize: 10, MaxAttempts: 3})
	if err != nil || result.Failed != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	op, err := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, "pex_worker_dead")
	if err != nil || op.Status != models.ExecutionDeadLetter {
		t.Fatalf("operation=%+v err=%v", op, err)
	}
	var cases, attempts int
	if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_private_execution_reconciliation_cases WHERE execution_operation_id=$1`, op.ID).Scan(&cases); err != nil || cases != 1 {
		t.Fatalf("cases=%d err=%v", cases, err)
	}
	if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_private_execution_attempts WHERE operation_id=$1`, op.ID).Scan(&attempts); err != nil || attempts < 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	tenantCtx := f.tenant
	tenantCtx.CorrelationID = "replay-worker"
	if err = service.ReplayExecution(context.Background(), tenantCtx, op.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayed, _ := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, op.ID)
	if replayed.Status != models.ExecutionPending {
		t.Fatalf("replayed status=%s", replayed.Status)
	}
}

func TestPrivateExecutionWorker_UnknownDoesNotBlindlyResubmitAndLeaseOwnership(t *testing.T) {
	f, service, repo, _ := privateWorkerFixture(t, "pex_worker_unknown")
	port := &executionPortStub{err: errors.New("timeout after write")}
	result, err := service.RunExecutionPass(context.Background(), port, privateusecases.ExecutionWorkerConfig{
		Owner: "worker-unknown", Lease: 3 * time.Second, BatchSize: 10, MaxAttempts: 3,
		BaseBackoff: time.Hour})
	if err != nil || result.Unknown != 1 || port.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, port.calls, err)
	}
	op, _ := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, "pex_worker_unknown")
	if op.Status != models.ExecutionUnknown {
		t.Fatalf("status=%s", op.Status)
	}
	if err := service.ReplayExecution(context.Background(), f.tenant, op.ID); err == nil {
		t.Fatal("unknown authoritative outcome was replayed as a fresh create")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE fleet_partner_private_execution_operations SET
		status='SUBMITTING',lease_owner='owner-a',lease_expires_at=now()-interval '1 second' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.HeartbeatExecution(context.Background(), "owner-b", op.ID, time.Minute); !errors.Is(err, privaterepos.ErrConflict) {
		t.Fatalf("foreign owner heartbeat err=%v", err)
	}
	if recovered, err := repo.RecoverExpiredExecutions(context.Background()); err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	recovered, _ := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, op.ID)
	if recovered.Status != models.ExecutionUnknown {
		t.Fatalf("recovered status=%s", recovered.Status)
	}
}

func TestPrivateExecutionWorker_CancellationConfirmedPendingUnknownAndRejected(t *testing.T) {
	tests := []struct {
		name          string
		initial       string
		outcome       privateusecases.ExecutionOutcome
		err           error
		wantStatus    string
		wantCancelled int
		wantUnknown   int
		wantFailed    int
	}{
		{name: "confirmed", initial: models.ExecutionCancellationPending,
			outcome: privateusecases.ExecutionOutcome{Definite: true, Cancelled: true,
				NormalizedStatus: "CANCELLED", SourceVersion: 4},
			wantStatus: models.ExecutionCancelled, wantCancelled: 1},
		{name: "pending", initial: models.ExecutionCancellationPending,
			outcome:    privateusecases.ExecutionOutcome{Definite: false, NormalizedStatus: "PENDING", SourceVersion: 3},
			wantStatus: models.ExecutionCancellationUnknown, wantUnknown: 1},
		{name: "unknown timeout", initial: models.ExecutionCancellationPending,
			err:        errors.New("timeout after cancellation may have committed"),
			wantStatus: models.ExecutionCancellationUnknown, wantUnknown: 1},
		{name: "readback confirms", initial: models.ExecutionCancellationUnknown,
			outcome: privateusecases.ExecutionOutcome{Definite: true, Cancelled: true,
				NormalizedStatus: "CANCELLED", SourceVersion: 5},
			wantStatus: models.ExecutionCancelled, wantCancelled: 1},
		{name: "source rejects", initial: models.ExecutionCancellationPending,
			err:        &platformclients.Error{Service: "dispatch", Outcome: platformclients.OutcomeValidation, Status: 409},
			wantStatus: models.ExecutionDeadLetter, wantFailed: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			operationID := "pex_cancel_" + strings.ReplaceAll(tc.name, " ", "_")
			f, service, repo, _ := privateWorkerFixture(t, operationID)
			if _, err := f.pool.Exec(context.Background(), `UPDATE fleet_partner_private_execution_operations SET
				status=$2,canonical_request_reference='request-cancel',canonical_trip_reference='trip-cancel',
				source_version=3,next_attempt_at=now() WHERE id=$1`, operationID, tc.initial); err != nil {
				t.Fatal(err)
			}
			port := &executionPortStub{result: tc.outcome, err: tc.err}
			result, err := service.RunExecutionPass(context.Background(), port, privateusecases.ExecutionWorkerConfig{
				Owner: "cancel-worker", Lease: 3 * time.Second, BatchSize: 1, MaxAttempts: 3, BaseBackoff: time.Hour})
			if err != nil {
				t.Fatal(err)
			}
			if result.Cancelled != tc.wantCancelled || result.Unknown != tc.wantUnknown || result.Failed != tc.wantFailed {
				t.Fatalf("result=%+v", result)
			}
			current, err := repo.GetExecution(context.Background(), nil, f.tenant.OrganizationID, operationID)
			if err != nil || current.Status != tc.wantStatus {
				t.Fatalf("status=%s want=%s err=%v", current.Status, tc.wantStatus, err)
			}
			if current.Status == models.ExecutionCancelled && current.SourceVersion < tc.outcome.SourceVersion {
				t.Fatalf("source version=%d want >=%d", current.SourceVersion, tc.outcome.SourceVersion)
			}
		})
	}
}

func TestPrivateProviderDeclineCreatesNoExecutionAndReassignmentRequiresNewDecision(t *testing.T) {
	f, _, repo, assignmentID := privateWorkerFixture(t, "pex_decline_flow")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM fleet_partner_private_execution_operations WHERE id='pex_decline_flow'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_private_assignments SET
		status='PENDING_PROVIDER_RESPONSE',response_deadline=now()+interval '10 minutes'
		WHERE id=$1`, assignmentID); err != nil {
		t.Fatal(err)
	}
	service := privateusecases.NewService(privateusecases.Options{Pool: f.pool, Repository: repo,
		Eligibility: privateEligibilityAlways{}, Audit: audit.NewRecorder(),
		Flags: privateusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true,
			DispatchEnabled: true, HaulingEnabled: true}})
	assignment, err := repo.GetAssignment(ctx, nil, f.tenant.OrganizationID, assignmentID)
	if err != nil {
		t.Fatal(err)
	}
	declined, err := service.Respond(ctx, f.tenant.OrganizationID, assignmentID,
		privateusecases.RespondInput{ProviderID: assignment.ProviderID, Decision: "DECLINE",
			ReasonCode: "FATIGUE_OR_REST_REQUIRED", IdempotencyKey: "decline-flow",
			RequestFingerprint: "decline-flow-fingerprint", ExpectedVersion: assignment.Version})
	if err != nil || declined.Status != models.StatusDeclined {
		t.Fatalf("declined=%+v err=%v", declined, err)
	}
	var executions int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_operations
		WHERE assignment_id=$1`, assignmentID).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("decline execution rows=%d err=%v", executions, err)
	}
	replacement, err := service.Reassign(ctx, f.tenant, assignmentID, privateusecases.ReassignInput{
		ProviderMembershipID: "mem_replacement", ProviderID: "provider_replacement",
		VehicleOrTruckReference: "vehicle_replacement", ExecutionService: "DISPATCH",
		ExpectedVersion: declined.Version})
	if err != nil || replacement.Status != models.StatusPending || replacement.ID == assignmentID {
		t.Fatalf("replacement=%+v err=%v", replacement, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_private_execution_operations
		WHERE assignment_id IN($1,$2)`, assignmentID, replacement.ID).Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("execution existed before replacement provider decision: rows=%d err=%v", executions, err)
	}
}
