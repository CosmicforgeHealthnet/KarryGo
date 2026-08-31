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
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/taxi-service/internal/config"
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
	flag.Parse()

	_ = godotenv.Load()
	cfg := config.Load()

	if os.Getenv("TAXI_OTP_DEBUG") != "true" {
		fatal("refusing to run: set TAXI_OTP_DEBUG=true (this is a dev-only helper)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	if *offline {
		if err := store.SetOffline(ctx, seededProviderID); err != nil {
			fatal("set offline %s: %v", seededProviderID, err)
		}
		success("seeded provider set OFFLINE")
		return
	}

	if err := store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
		ProviderID: seededProviderID, VehicleID: seededVehicleID, VehicleClass: seededVehicleClass,
		MarketID: seededMarketID, Lat: seededLat, Lng: seededLng, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		fatal("set online %s: %v", seededProviderID, err)
	}
	success("seeded provider set ONLINE (TTL %s) -- create a Lagos booking to match", ttl)
	fmt.Println("  Note: TTL expires; re-run this before testing again if it has been a while.")
}

func success(format string, args ...any) {
	fmt.Printf("\033[1;32m[taxi-online]\033[0m "+format+"\n", args...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[taxi-online] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
