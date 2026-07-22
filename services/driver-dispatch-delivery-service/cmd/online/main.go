// Command online is a DEV-ONLY helper that marks the seeded dispatch riders as
// online (Postgres availability row + Redis live status/geo), so the customer
// bike-delivery flow can match a rider end-to-end without running the
// dispatch_provider app. The dev SQL seeder deliberately does not touch Redis
// (online state belongs to the live rider heartbeat), so this fills that gap
// for local testing. Mirrors the hauling service's cmd/online.
//
// Usage (after the bootstrap + seed):
//
//	cd services/driver-dispatch-delivery-service
//	go run ./cmd/online            # put the seeded riders online near Lagos
//	go run ./cmd/online --offline  # take them back offline
//
// NOTE: the live location key has a short TTL, so re-run before each test if
// the customer live-location endpoint starts returning 404 — geo membership
// for matching persists until --offline.
//
// It refuses to run unless DISPATCH_RIDER_DEBUG_OTP=true, the same flag that
// already gates the dev login path, so it can never run against a real
// deployment.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
)

// Seeded riders (must match seeds/dev_seed.sql), positioned near central Lagos
// (~6.5, 3.35) so they fall inside the default broadcast radius for typical
// Lagos bookings.
//
// Only go-online-ready riders belong here. seeds/dev_seed.sql also seeds 3
// riders (d3/d4/d5) that are deliberately NOT ready — they exist to exercise
// the three distinct checkOnlineGate failure branches (not verified, account
// suspended, no verified active bike) and must keep failing that gate, so
// they are intentionally left out of this list.
var seededRiders = []struct {
	ID       string
	Lat, Lng float64
}{
	{ID: "d1000000-0000-4000-8000-000000000001", Lat: 6.6018, Lng: 3.3515},
	{ID: "d2000000-0000-4000-8000-000000000002", Lat: 6.5005, Lng: 3.3580},
}

func main() {
	offline := flag.Bool("offline", false, "take the seeded riders offline instead of online")
	flag.Parse()

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		fatal(fmt.Sprintf("load config: %v", err))
	}

	if os.Getenv("DISPATCH_RIDER_DEBUG_OTP") != "true" {
		fatal("refusing to run: set DISPATCH_RIDER_DEBUG_OTP=true (this is a dev-only helper)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal(fmt.Sprintf("connect postgres: %v", err))
	}
	defer db.Close()

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		fatal(fmt.Sprintf("connect redis %s: %v", cfg.Redis.Addr, err))
	}

	repo := availability.NewPostgresRepository(db)
	live := availability.NewRedisLiveStore(client)
	now := time.Now().UTC()

	for _, rider := range seededRiders {
		if *offline {
			if _, err := repo.SetOffline(ctx, rider.ID, now); err != nil {
				fatal(fmt.Sprintf("set offline %s: %v", rider.ID, err))
			}
			if err := live.ClearProvider(ctx, rider.ID); err != nil {
				fatal(fmt.Sprintf("clear live state %s: %v", rider.ID, err))
			}
			fmt.Printf("offline: %s\n", rider.ID)
			continue
		}

		if _, err := repo.SetVerifiedToGoOnline(ctx, rider.ID, true); err != nil {
			fatal(fmt.Sprintf("verify rider %s: %v", rider.ID, err))
		}
		if _, err := repo.SetOnline(ctx, rider.ID, now); err != nil {
			fatal(fmt.Sprintf("set online %s: %v", rider.ID, err))
		}
		if err := live.SetStatus(ctx, rider.ID, availability.StatusOnline); err != nil {
			fatal(fmt.Sprintf("set live status %s: %v", rider.ID, err))
		}
		if err := live.SetLocation(ctx, rider.ID, availability.Location{
			ProviderID: rider.ID,
			Lat:        rider.Lat,
			Lng:        rider.Lng,
			UpdatedAt:  now,
		}, true); err != nil {
			fatal(fmt.Sprintf("set location %s: %v", rider.ID, err))
		}
		fmt.Printf("online: %s at (%.4f, %.4f)\n", rider.ID, rider.Lat, rider.Lng)
	}

	if *offline {
		fmt.Println("done: seeded riders are offline")
		return
	}
	fmt.Println("done: seeded riders are online (re-run if the live-location TTL lapses)")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
