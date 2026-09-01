// Command online is a DEV-ONLY helper that marks the seeded clean provider
// as online in Redis, so the customer booking flow can match a driver
// end-to-end without running any driver app. The dev SQL seeder deliberately
// does not touch Redis (online state belongs to the live provider
// heartbeat), so this fills that gap for local testing.
//
// Only the ONE fully-compliant seeded provider (caca4b5a-...) is ever put
// online here -- the other four seeded providers exist specifically to
// prove PUT /provider/availability refuses them with their own named
// compliance-gate reason, and forcing them into Redis directly would bypass
// the exact thing they are seeded to test.
//
// Usage (after the bootstrap + seed):
//
//	cd services/driver-taxi-service
//	go run ./cmd/online            # put the clean provider online (~90s TTL refreshed)
//	go run ./cmd/online --offline  # take them back offline
//
// It refuses to run unless TAXI_OTP_DEBUG=true, the same flag that already
// gates the dev login path, so it can never run against a real deployment.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/taxi-service/internal/config"
	"cosmicforge/logistics/services/taxi-service/internal/database"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

// Must match seeds/dev_seed.sql: the clean provider, their one active
// vehicle, and a Lagos Mainland position (~6.52, 3.37, the operating zone's
// own center) so they fall inside the default match radius for any
// Lagos-Mainland booking.
const (
	seededProviderID   = "caca4b5a-7018-4fa8-9d08-c3f14f6d910c"
	seededVehicleID    = "7e4cc5a5-f9e2-4978-846e-56fdbeb390d9"
	seededVehicleClass = "comfort"
	seededMarketID     = "a0000000-0000-4000-8000-000000000001"
	seededLat          = 6.525
	seededLng          = 3.375
)

func main() {
	offline := flag.Bool("offline", false, "take the seeded provider offline instead of online")
	keepalive := flag.Bool("keepalive", false, "stay running and refresh the online TTL until interrupted (for hand-paced app testing)")
	flag.Parse()

	_ = godotenv.Load()
	cfg := config.Load()

	if os.Getenv("TAXI_OTP_DEBUG") != "true" {
		fatal("refusing to run: set TAXI_OTP_DEBUG=true (this is a dev-only helper)")
	}

	// A one-shot run only needs a short timeout; keepalive must outlive it,
	// so it runs against a cancellable background context instead and stops
	// on SIGINT/SIGTERM.
	var ctx context.Context
	var cancel context.CancelFunc
	if *keepalive {
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	}
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		fatal("connect to redis at %s: %v", cfg.Redis.Addr, err)
	}

	ttl := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	store := availabilityrepositories.NewRedisAvailabilityStoreWithTTL(client, ttl)

	// Redis availability alone is NOT enough to be matchable: the matcher's
	// own eligibility filter (findEligibleCandidates) reads
	// provider_operational_state from Postgres and skips any provider whose
	// state is not AVAILABLE. A provider written only into Redis therefore
	// shows up in GET /customer/availability but is silently never offered a
	// booking -- the exact symptom this helper existed to avoid.
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("connect to postgres: %v", err)
	}
	defer db.Close()
	states := availabilityrepositories.NewPostgresOperationalStateRepository(db)

	if *offline {
		if err := store.SetOffline(ctx, seededProviderID); err != nil {
			fatal("set offline %s: %v", seededProviderID, err)
		}
		if err := transition(ctx, states, availabilitymodels.StateOffline, "dev_helper_offline"); err != nil {
			fatal("set operational state OFFLINE: %v", err)
		}
		success("seeded provider set OFFLINE (redis + operational state)")
		return
	}

	if err := store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
		ProviderID: seededProviderID, VehicleID: seededVehicleID, VehicleClass: seededVehicleClass,
		MarketID: seededMarketID, Lat: seededLat, Lng: seededLng, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		fatal("set online %s: %v", seededProviderID, err)
	}
	if err := transition(ctx, states, availabilitymodels.StateAvailable, "dev_helper_online"); err != nil {
		fatal("set operational state AVAILABLE: %v", err)
	}
	success("seeded provider set ONLINE (TTL %s, state AVAILABLE) -- create a Lagos %s booking to match", ttl, seededVehicleClass)

	if !*keepalive {
		fmt.Println("  Note: the TTL above expires -- re-run this before testing again, or use --keepalive.")
		return
	}

	// The online entry is a TTL'd Redis key, so a hand-paced walkthrough on a
	// real device (where a human takes minutes to tap through screens)
	// silently loses the only matchable driver partway through and every
	// booking then goes unmatched. Refreshing at a third of the TTL keeps
	// them continuously matchable for as long as this stays running.
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	fmt.Printf("  keepalive: refreshing every %s -- press Ctrl-C to stop and go offline.\n", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Best-effort clean shutdown on its own fresh context: ctx is
			// already cancelled by the signal that got us here.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = store.SetOffline(shutdownCtx, seededProviderID)
			_ = transition(shutdownCtx, states, availabilitymodels.StateOffline, "dev_helper_keepalive_stopped")
			fmt.Println()
			success("keepalive stopped -- seeded provider set OFFLINE")
			return
		case <-ticker.C:
			if err := store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
				ProviderID: seededProviderID, VehicleID: seededVehicleID, VehicleClass: seededVehicleClass,
				MarketID: seededMarketID, Lat: seededLat, Lng: seededLng, UpdatedAt: time.Now().Unix(),
			}); err != nil && ctx.Err() == nil {
				fatal("refresh online %s: %v", seededProviderID, err)
			}
			// The matcher flips the provider to OFFERED/ON_TRIP during a real
			// booking; forcing AVAILABLE back would corrupt a live ride, so
			// only an actually-OFFLINE provider is restored here.
			if err := restoreAvailableIfOffline(ctx, states); err != nil && ctx.Err() == nil {
				fatal("refresh operational state: %v", err)
			}
		}
	}
}

// transition moves the seeded provider to the given operational state,
// reading the current version first so the repository's guarded
// compare-and-set upsert succeeds whether or not a row already exists.
func transition(
	ctx context.Context,
	states *availabilityrepositories.PostgresOperationalStateRepository,
	to availabilitymodels.OperationalState,
	reason string,
) error {
	current, err := states.Get(ctx, seededProviderID)
	if err != nil {
		return err
	}
	if current.Current == to {
		return nil
	}
	_, err = states.Transition(ctx, nil, seededProviderID, to, reason, nil, current.Version)
	return err
}

// restoreAvailableIfOffline puts the provider back to AVAILABLE only if they
// are currently OFFLINE. During a live booking the matcher legitimately owns
// this state (OFFERED, EN_ROUTE_PICKUP, ON_TRIP...), and overwriting it from
// here would break the very ride being tested.
func restoreAvailableIfOffline(
	ctx context.Context,
	states *availabilityrepositories.PostgresOperationalStateRepository,
) error {
	current, err := states.Get(ctx, seededProviderID)
	if err != nil {
		return err
	}
	if current.Current != availabilitymodels.StateOffline {
		return nil
	}
	_, err = states.Transition(ctx, nil, seededProviderID, availabilitymodels.StateAvailable, "dev_helper_keepalive", nil, current.Version)
	return err
}

func success(format string, args ...any) {
	fmt.Printf("\033[1;32m[taxi-online]\033[0m "+format+"\n", args...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[taxi-online] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
