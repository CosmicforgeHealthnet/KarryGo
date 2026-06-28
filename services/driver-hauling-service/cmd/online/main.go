// Command online is a DEV-ONLY helper that marks the seeded providers as online
// in Redis, so the customer booking flow can match a truck end-to-end without
// running the truck_provider app. The dev SQL seeder deliberately does not touch
// Redis (online state belongs to the live provider heartbeat), so this fills
// that gap for local testing.
//
// Usage (after the bootstrap + seed):
//
//	cd services/driver-hauling-service
//	go run ./cmd/online            # put the seeded providers online (~90s TTL refreshed)
//	go run ./cmd/online --offline  # take them back offline
//
// It refuses to run unless HAULING_OTP_DEBUG=true, the same flag that already
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

	"cosmicforge/logistics/services/hauling-service/internal/config"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
)

// Seeded providers (must match seeds/dev_seed.sql). Each goes online with its
// "van" truck and a Lagos position near the seeded booking pickups (~6.59, 3.34)
// so they fall inside the default match radius for typical Lagos bookings.
var seededOnline = []availabilityrepositories.ProviderStatus{
	{ProviderID: "a0000000-0000-4000-8000-000000000001", TruckID: "b1000000-0000-4000-8000-000000000001", Lat: 6.6018, Lng: 3.3515},
	{ProviderID: "a0000000-0000-4000-8000-000000000002", TruckID: "b2000000-0000-4000-8000-000000000001", Lat: 6.5005, Lng: 3.3580},
	{ProviderID: "a0000000-0000-4000-8000-000000000003", TruckID: "b3000000-0000-4000-8000-000000000001", Lat: 6.4400, Lng: 3.4700},
}

func main() {
	offline := flag.Bool("offline", false, "take the seeded providers offline instead of online")
	flag.Parse()

	_ = godotenv.Load()
	cfg := config.Load()

	if os.Getenv("HAULING_OTP_DEBUG") != "true" {
		fatal("refusing to run: set HAULING_OTP_DEBUG=true (this is a dev-only helper)")
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
	store := availabilityrepositories.NewRedisAvailabilityStore(client)

	ttl := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	if ttl <= 0 {
		ttl = 90 * time.Second
	}

	for _, st := range seededOnline {
		if *offline {
			if err := store.SetOffline(ctx, st.ProviderID); err != nil {
				fatal("set offline %s: %v", st.ProviderID, err)
			}
			continue
		}
		st.UpdatedAt = time.Now().Unix()
		if err := store.SetOnline(ctx, st, ttl); err != nil {
			fatal("set online %s: %v", st.ProviderID, err)
		}
	}

	if *offline {
		success("%d seeded providers set OFFLINE", len(seededOnline))
		return
	}
	success("%d seeded providers set ONLINE (TTL %s) — create a Lagos booking to match", len(seededOnline), ttl)
	fmt.Println("  Note: TTL expires; re-run this before testing, or run the truck_provider app for a live heartbeat.")
}

func success(format string, args ...any) {
	fmt.Printf("\033[1;32m[hauling-online]\033[0m "+format+"\n", args...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[hauling-online] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
