package marketplace_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tripCancelAdapter struct{ trips *tripfeature.Service }

func (a tripCancelAdapter) CancelOrganizationTrip(ctx context.Context, tripID, reason, correlationID string) error {
	_, err := a.trips.CancelOrganizationTrip(ctx, tripID, reason, correlationID)
	return err
}

// newCancellableExecutionServer is newExecutionServer with the real trip
// feature installed as the organization-cancel authority, as the router does.
func newCancellableExecutionServer(t *testing.T, pool *pgxpool.Pool, verifier marketplace.AcceptanceVerifier) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())
	flags := executionFlags()
	service := marketplace.NewService(marketplace.NewRepository(pool), flags, verifier).
		WithTripCanceller(tripCancelAdapter{trips: tripfeature.NewService(tripfeature.NewPostgresRepository(pool), nil)})
	v := serviceauth.NewVerifier(serviceauth.Secrets{fleetService: []byte(fleetSecret)}, time.Minute)
	marketplace.RegisterInternalRoutes(engine.Group("/api/v1/internal/marketplace"),
		marketplace.NewHandler(service, flags), v, []string{fleetService})
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

// FLEET-12.2: Fleet cancels an organization-managed canonical trip through
// Dispatch's internal marketplace API. Only the organization that holds the
// trip can cancel it; the cancellation goes through the trip feature's own
// state machine and outbox; a repeat is a safe no-op; a trip already past
// pickup is a conflict; and only Fleet may call the route.
func TestFleetCancelsOrganizationManagedTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000091")
	verifier := &stubVerifier{acceptance: acceptanceFor("asg_cancel", "org_A", providerID, "mem_c", "resp_c", requestID)}
	server := newCancellableExecutionServer(t, pool, verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-cancel-res")
	status, created := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_c", "asg_cancel", "resp_c"),
		"idem-cancel-exec", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("execution = %d (%v)", status, created)
	}
	tripID := dataOf(t, created)["tripId"].(string)
	path := "/api/v1/internal/marketplace/trips/" + tripID + "/cancel"

	// Another organization cannot cancel it, and learns nothing about it.
	if status, body := signedCall(t, server, http.MethodPost, path,
		map[string]any{"organizationId": "org_B", "reason": "not mine", "idempotencyKey": "k-b"},
		"k-b", fleetService, fleetSecret); status != http.StatusNotFound {
		t.Fatalf("cross-organization cancel = %d (%v), want 404", status, body)
	}
	// Only Fleet may call the route.
	if status, _ := signedCall(t, server, http.MethodPost, path,
		map[string]any{"organizationId": "org_A", "idempotencyKey": "k-x"}, "k-x", "some-other-service", fleetSecret); status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("unknown caller = %d, want 401/403", status)
	}
	var stillAssigned string
	_ = pool.QueryRow(ctx, `SELECT status FROM trips WHERE id = $1`, tripID).Scan(&stillAssigned)
	if stillAssigned == "cancelled" {
		t.Fatal("a refused cancel changed the trip")
	}

	status, body := signedCall(t, server, http.MethodPost, path,
		map[string]any{"organizationId": "org_A", "reason": "customer withdrew order", "idempotencyKey": "k-a"},
		"k-a", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, body)["status"] != "cancelled" {
		t.Fatalf("cancel = %d (%v)", status, body)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT status FROM trips WHERE id = $1`, tripID).Scan(&stored); err != nil || stored != "cancelled" {
		t.Fatalf("stored trip status = %q (%v)", stored, err)
	}
	var logged int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM trip_state_log WHERE trip_id = $1 AND to_status = 'cancelled'`, tripID).Scan(&logged)
	if logged != 1 {
		t.Fatalf("cancellation state-log rows = %d, want 1 (the trip feature's own path)", logged)
	}

	// A repeat (Fleet retry or read-back) returns the cancelled trip and
	// writes nothing more.
	if status, body := signedCall(t, server, http.MethodPost, path,
		map[string]any{"organizationId": "org_A", "reason": "retry", "idempotencyKey": "k-a"},
		"k-a", fleetService, fleetSecret); status != http.StatusOK || dataOf(t, body)["status"] != "cancelled" {
		t.Fatalf("repeat cancel = %d (%v)", status, body)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM trip_state_log WHERE trip_id = $1 AND to_status = 'cancelled'`, tripID).Scan(&logged)
	if logged != 1 {
		t.Fatalf("repeat cancel wrote another state-log row: %d", logged)
	}
}

// A trip already in progress cannot be cancelled remotely: the trip feature
// refuses and nothing changes.
func TestFleetCannotCancelATripPastPickup(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	requestID := seedBroadcast(t, pool)
	providerID := seedProvider(t, pool, "+2348010000092")
	verifier := &stubVerifier{acceptance: acceptanceFor("asg_late", "org_A", providerID, "mem_l", "resp_l", requestID)}
	server := newCancellableExecutionServer(t, pool, verifier)
	reservationID := reserveFor(t, server, requestID, "org_A", "idem-late-res")
	status, created := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/executions",
		executionBodyFor(requestID, reservationID, "org_A", providerID, "mem_l", "asg_late", "resp_l"),
		"idem-late-exec", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("execution = %d (%v)", status, created)
	}
	tripID := dataOf(t, created)["tripId"].(string)
	if _, err := pool.Exec(ctx, `UPDATE trips SET status = 'in_progress' WHERE id = $1`, tripID); err != nil {
		t.Fatal(err)
	}
	if status, body := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/trips/"+tripID+"/cancel",
		map[string]any{"organizationId": "org_A", "reason": "too late", "idempotencyKey": "k-late"},
		"k-late", fleetService, fleetSecret); status != http.StatusConflict {
		t.Fatalf("cancel in progress = %d (%v), want 409", status, body)
	}
	var stored string
	_ = pool.QueryRow(ctx, `SELECT status FROM trips WHERE id = $1`, tripID).Scan(&stored)
	if stored != "in_progress" {
		t.Fatalf("refused cancel changed the trip to %q", stored)
	}
}
