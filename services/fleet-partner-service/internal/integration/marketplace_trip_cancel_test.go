package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// cancelRecordingContract is the source service's trip-cancel authority as
// Fleet sees it. Only the calls the cancel path makes are implemented.
type cancelRecordingContract struct {
	platformclients.ExecutionContract
	mu        sync.Mutex
	cancels   []platformclients.CallMeta
	tripIDs   []string
	cancelErr error
	tripState platformclients.SourceTripView
}

func (c *cancelRecordingContract) Configured() bool        { return true }
func (c *cancelRecordingContract) ServiceCategory() string { return platformclients.CategoryDispatch }
func (c *cancelRecordingContract) CancelExecution(_ context.Context, meta platformclients.CallMeta, tripID, _, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancels = append(c.cancels, meta)
	c.tripIDs = append(c.tripIDs, tripID)
	return c.cancelErr
}
func (c *cancelRecordingContract) ReadTrip(context.Context, platformclients.CallMeta, string) (platformclients.SourceTripView, error) {
	return c.tripState, nil
}
func (c *cancelRecordingContract) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cancels)
}

func newTripCancelService(pool *pgxpool.Pool, contract platformclients.ExecutionContract) *mktusecases.Service {
	return mktusecases.NewService(mktusecases.Options{
		Pool: pool, Projections: mktrepos.NewProjectionRepository(pool), Offers: mktrepos.NewOfferRepository(pool),
		Assignments: mktrepos.NewAssignmentRepository(pool), ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Trips: mktrepos.NewTripRepository(pool), Commands: mktrepos.NewCommandRepository(pool),
		OperatingModes: orgrepos.NewSettingsRepository(pool),
		Eligibility:    mktusecases.NewEligibilityChecker(app.NewMembershipReader(providerrepos.NewRepository(pool)), nil, nil),
		Events:         events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:        observability.NewMetrics(),
		Executions:     platformclients.ExecutionSet{platformclients.CategoryDispatch: contract},
		Opportunities:  mktusecases.OpportunityConfig{Enabled: true},
		Flags:          mktusecases.Flags{ProjectionEnabled: true, OrganizationAcceptEnabled: true, AssignmentEnabled: true},
	})
}

// FLEET-12.2: a dispatcher cancels a live canonical trip through Fleet. The
// command goes to the trip's source service with the organization's
// identity; the outcome is recorded durably; a duplicate request re-uses the
// first outcome instead of calling again; an unknown outcome is resolved by
// the reconciler reading the trip back; a refusal is a conflict. Another
// organization, a member without the permission and a missing reason are all
// refused before anything is sent.
func TestRow12_2_CancelCorporateTripThroughSourceService(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Trip Cancel A", "RC-TCANCEL-A")
	orgB := createOrganization(t, pool, "Trip Cancel B", "RC-TCANCEL-B")
	contract := &cancelRecordingContract{}
	service := newTripCancelService(pool, contract)

	seedTrip := func(requestID, platformTripID, driver string) string {
		_, assignment, membership := readyAssignment(t, pool, orgA.ID, requestID, driver, false)
		raw, event := sourceEvent(t, s1112Dispatch, "evt-"+platformTripID, mktusecases.EventTripCreated,
			"TRIP", platformTripID, 1, tripPayload(requestID, platformTripID, assignment.ID, *membership.ProviderID, orgA.ID, "assigned"))
		ingest(t, service, raw, event)
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_trip_projections WHERE organization_id=$1 AND platform_trip_id=$2`, orgA.ID, platformTripID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	tripID := seedTrip("req-tcancel-1", "trip-tcancel-1", "tcancel-drv-1")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: c.GetHeader("X-Test-Org"),
			OrganizationUserID: member, ActorType: tenant.ActorTypeOrganizationUser, ActorID: member})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	mkthttp.NewHandler(service, authorizer, idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	dispatcherA := seedApprovalMember(t, pool, orgA.ID, "rol_system_dispatcher", "tcancel-dispatcher-a")
	dispatcherB := seedApprovalMember(t, pool, orgB.ID, "rol_system_dispatcher", "tcancel-dispatcher-b")
	noRoleA := seedApprovalMember(t, pool, orgA.ID, "", "tcancel-norole-a")
	cancel := func(org, member, trip, key string, body map[string]any) (int, map[string]any) {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/marketplace/trips/"+trip+"/cancel", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Member", member)
		req.Header.Set("X-Test-Org", org)
		req.Header.Set("Idempotency-Key", key)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		var out map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &out)
		return recorder.Code, out
	}
	reason := map[string]any{"reason": "customer withdrew the order"}

	// Refusals send nothing to the source service.
	if code, _ := cancel(orgB.ID, dispatcherB, tripID, "k-b", reason); code != http.StatusNotFound {
		t.Fatalf("cross-tenant cancel = %d, want 404", code)
	}
	if code, _ := cancel(orgA.ID, noRoleA, tripID, "k-norole", reason); code != http.StatusForbidden {
		t.Fatalf("cancel without marketplace.cancel = %d, want 403", code)
	}
	if code, _ := cancel(orgA.ID, dispatcherA, tripID, "k-noreason", map[string]any{"reason": "  "}); code != http.StatusUnprocessableEntity {
		t.Fatalf("cancel without a reason = %d, want 422", code)
	}
	if n := contract.calls(); n != 0 {
		t.Fatalf("refused requests reached the source service %d times", n)
	}

	// Success: one call to the source, naming the organization and the source trip.
	code, body := cancel(orgA.ID, dispatcherA, tripID, "k-a-1", reason)
	if code != http.StatusOK || body["data"].(map[string]any)["status"] != "CANCELLED" {
		t.Fatalf("cancel = %d %v", code, body)
	}
	if contract.calls() != 1 || contract.cancels[0].OrganizationID != orgA.ID || contract.tripIDs[0] != "trip-tcancel-1" {
		t.Fatalf("source calls = %v trips = %v", contract.cancels, contract.tripIDs)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_marketplace_command_operations WHERE operation_type='CANCEL_EXECUTION' AND idempotency_key=$1`, "trip-cancel:"+tripID).Scan(&status); err != nil || status != "CONFIRMED" {
		t.Fatalf("cancel command = %q (%v), want CONFIRMED", status, err)
	}
	if n := rowsCount(t, pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='trip.cancellation_requested' AND resource_id=$2`, orgA.ID, tripID); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
	// A second dispatcher request (new idempotency key) re-uses the confirmed
	// command: no second remote call.
	if code, body := cancel(orgA.ID, dispatcherA, tripID, "k-a-2", reason); code != http.StatusOK || body["data"].(map[string]any)["status"] != "CANCELLED" {
		t.Fatalf("second cancel = %d %v", code, body)
	}
	if contract.calls() != 1 {
		t.Fatalf("second request called the source again: %d calls", contract.calls())
	}

	// Unknown outcome: accepted as pending, then resolved by read-back.
	pendingTrip := seedTrip("req-tcancel-2", "trip-tcancel-2", "tcancel-drv-2")
	contract.cancelErr = &platformclients.Error{Service: "driver-dispatch-delivery-service", Outcome: platformclients.OutcomeUnknown, Message: "timeout"}
	if code, body := cancel(orgA.ID, dispatcherA, pendingTrip, "k-p", reason); code != http.StatusAccepted || body["data"].(map[string]any)["status"] != "CANCELLATION_PENDING" {
		t.Fatalf("unknown-outcome cancel = %d %v", code, body)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_marketplace_command_operations SET next_attempt_at = now() - interval '1 minute' WHERE idempotency_key=$1`, "trip-cancel:"+pendingTrip); err != nil {
		t.Fatal(err)
	}
	contract.cancelErr = nil
	contract.tripState = platformclients.SourceTripView{TripID: "trip-tcancel-2", Status: "cancelled"}
	if _, err := service.ReconcileCommands(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_marketplace_command_operations WHERE idempotency_key=$1`, "trip-cancel:"+pendingTrip).Scan(&status); err != nil || status != "CONFIRMED" {
		t.Fatalf("reconciled cancel = %q (%v), want CONFIRMED", status, err)
	}

	// A source refusal (trip past pickup) is a conflict, recorded as rejected.
	lateTrip := seedTrip("req-tcancel-3", "trip-tcancel-3", "tcancel-drv-3")
	contract.cancelErr = &platformclients.Error{Service: "driver-dispatch-delivery-service", Outcome: platformclients.OutcomePermanent, Status: http.StatusConflict, Message: "in progress"}
	if code, _ := cancel(orgA.ID, dispatcherA, lateTrip, "k-late", reason); code != http.StatusConflict {
		t.Fatalf("refused cancel = %d, want 409", code)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_marketplace_command_operations WHERE idempotency_key=$1`, "trip-cancel:"+lateTrip).Scan(&status); err != nil || status != "REJECTED" {
		t.Fatalf("refused cancel command = %q (%v), want REJECTED", status, err)
	}
}
