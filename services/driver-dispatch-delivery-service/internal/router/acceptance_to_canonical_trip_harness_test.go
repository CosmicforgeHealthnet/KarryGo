package router_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/router"
)

const crossServiceSecret = "stage-one-cross-service-hmac-secret"

type dispatchHarnessReady struct {
	URL               string `json:"url"`
	ProviderID        string `json:"providerId"`
	PrivateProviderID string `json:"privateProviderId"`
	PrivateVehicleID  string `json:"privateVehicleId"`
	RequestID         string `json:"requestId"`
	Token             string `json:"token"`
}

// TestAcceptanceToCanonicalTripDispatchHarness is the dispatch half of the
// cross-module integration test. Go's internal-package boundary prevents one
// test binary from importing both sibling services, so the fleet test starts
// this helper as a subprocess and drives it only through real HTTP.
func TestAcceptanceToCanonicalTripDispatchHarness(t *testing.T) {
	if os.Getenv("KARRYGO_CROSS_SERVICE_DISPATCH_HELPER") != "1" {
		t.Skip("started only by TestAcceptanceToOneCanonicalTrip")
	}
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	readyPath := os.Getenv("KARRYGO_CROSS_SERVICE_READY_FILE")
	fleetURL := os.Getenv("KARRYGO_CROSS_SERVICE_FLEET_URL")
	if dsn == "" || readyPath == "" || fleetURL == "" {
		t.Fatal("dispatch helper requires database, ready-file, and fleet URL settings")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect dispatch database: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `
		TRUNCATE dispatch_marketplace_outbox_deliveries, dispatch_marketplace_outbox,
			dispatch_marketplace_command_operations, dispatch_organization_reservations,
			dispatch_fleet_relay_operations, trips, request_broadcasts, bikes, providers CASCADE`); err != nil {
		t.Fatalf("clean dispatch database: %v", err)
	}

	var providerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO providers (id, phone, verification_status, is_active, onboarding_complete, operation_type)
		VALUES (gen_random_uuid(), '+2348099000001', 'verified', true, true, 'fleet')
		RETURNING id`).Scan(&providerID); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	var privateProviderID string
	if err := pool.QueryRow(ctx, `INSERT INTO providers
		(id,phone,verification_status,is_active,onboarding_complete,operation_type)
		VALUES(gen_random_uuid(),'+2348099000002','verified',true,true,'fleet') RETURNING id`).Scan(&privateProviderID); err != nil {
		t.Fatalf("seed private provider: %v", err)
	}
	var requestID string
	var vehicleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO bikes(provider_id,bike_type,brand,model,year,color,plate_number,verification_status,is_active)
		VALUES($1,'dispatch_bike','KarryGo','Private',2025,'black',$2,'verified',true)
		RETURNING id`, privateProviderID, "E2E-"+privateProviderID[:8]).Scan(&vehicleID); err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_availability(provider_id,status,
		verified_to_go_online,session_start) VALUES($1,'online',true,now())`, privateProviderID); err != nil {
		t.Fatalf("seed provider availability: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO request_broadcasts (booking_id, broadcast_radius_km, expires_at, booking_payload)
		VALUES (gen_random_uuid(), 5, now() + interval '30 minutes', '{}'::jsonb)
		RETURNING id`).Scan(&requestID); err != nil {
		t.Fatalf("seed broadcast: %v", err)
	}

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	accessSecret := []byte("stage-one-provider-access-token-secret")
	authService := authusecases.NewAuthUsecase(authusecases.Options{
		AccessTokenSecret:  accessSecret,
		RefreshTokenSecret: accessSecret,
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    time.Hour,
		Redis:              redisClient,
	})
	token, _, err := authService.TokenUsecase().GenerateAccessToken(
		providerID, "+2348099000001", uuid.NewString())
	if err != nil {
		t.Fatalf("mint provider token: %v", err)
	}

	cfg := config.Config{
		AppEnv:                                "test",
		ServiceName:                           "driver-dispatch-delivery-service",
		FleetServiceURL:                       fleetURL,
		FleetServiceCaller:                    "driver-dispatch-delivery-service",
		FleetServiceSecret:                    []byte(crossServiceSecret),
		FleetServiceTimeout:                   5 * time.Second,
		MarketplaceInternalAPIEnabled:         true,
		MarketplaceReservationEnabled:         true,
		MarketplaceExecutionEnabled:           true,
		MarketplaceOutboxEnabled:              true,
		MarketplaceProviderEligibilityEnabled: true,
		MarketplaceVehicleEligibilityEnabled:  true,
		PrivateExecutionEnabled:               true,
		ProviderOrganizationOffersEnabled:     true,
		ProviderCompanyAssignmentsEnabled:     true,
		FleetRelayEnabled:                     true,
		FleetRelayBatchSize:                   10,
		FleetRelayLeaseDuration:               10 * time.Second,
		FleetRelayRetryLimit:                  5,
		FleetRelayBackoffBase:                 100 * time.Millisecond,
		FleetRelayInterval:                    100 * time.Millisecond,
		HandoverSecret:                        []byte("stage-one-handover-code-secret"),
		AccessTokenSecret:                     accessSecret,
		AccessTokenTTL:                        time.Hour,
		RefreshTokenTTL:                       time.Hour,
	}

	tripService := tripfeature.NewService(tripfeature.NewPostgresRepository(pool),
		tripfeature.NewLocalProofStorage(t.TempDir(), "local-private://"))
	engine := router.New(cfg, pool, redisClient, authService, nil, tripService, nil, nil)
	stopRequested := make(chan struct{}, 1)
	engine.POST("/__cross_service_stop", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
		select {
		case stopRequested <- struct{}{}:
		default:
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: engine, ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	workerCtx, stopWorkersContext := context.WithCancel(ctx)
	stopWorkers := router.StartMarketplaceWorkers(workerCtx, cfg, pool)

	ready := dispatchHarnessReady{
		URL:               "http://" + listener.Addr().String(),
		ProviderID:        providerID,
		PrivateProviderID: privateProviderID,
		PrivateVehicleID:  vehicleID,
		RequestID:         requestID,
		Token:             token,
	}
	encoded, err := json.Marshal(ready)
	if err != nil {
		t.Fatalf("encode ready data: %v", err)
	}
	if err := os.WriteFile(readyPath, encoded, 0o600); err != nil {
		t.Fatalf("write ready data: %v", err)
	}

	select {
	case <-stopRequested:
	case <-ctx.Done():
		t.Fatalf("dispatch helper timed out: %v", ctx.Err())
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown dispatch server: %v", err)
	}
	stopWorkersContext()
	stopWorkers()
	if err := <-serveDone; err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve dispatch: %v", err)
	}
}
