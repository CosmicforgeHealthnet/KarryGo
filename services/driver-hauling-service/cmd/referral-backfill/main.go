// Command referral-backfill announces every existing self-registered truck
// provider to the referral programme once, so providers who signed up before
// the programme launched get a referral code too. Backfilled accounts are
// marked as not new and can never be attributed to a referrer. Drivers
// created by a fleet organization are skipped: fleets are outside the
// personal referral programme.
//
// It only writes to this service's referral_outbox; the running service's
// relay (HAULING_REFERRAL_FEED_ENABLED=true) delivers the events. Re-running
// is safe: referral-service ignores accounts it already knows.
//
//	cd services/driver-hauling-service && go run ./cmd/referral-backfill
package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/referralcontract"
)

const batchSize = 500

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("backfill", "connect: %v", err)
	}
	defer db.Close()
	feed, err := referralcontract.NewFeed(db, "driver-hauling-service", cfg.ReferralIdentityPepper)
	if err != nil {
		logging.Fatal("backfill", "HAULING_REFERRAL_IDENTITY_PEPPER is required: %v", err)
	}

	announced, after := 0, ""
	for {
		rows, err := db.Query(ctx, `
			SELECT id::text, COALESCE(phone, ''), COALESCE(email, ''), location_state, created_at
			FROM truck_providers
			WHERE managed_by_organization_id IS NULL AND id::text > $1
			ORDER BY id::text LIMIT $2`, after, batchSize)
		if err != nil {
			logging.Fatal("backfill", "read providers: %v", err)
		}
		var batch []referralcontract.Account
		for rows.Next() {
			var id, phone, email, state string
			var createdAt time.Time
			if err := rows.Scan(&id, &phone, &email, &state, &createdAt); err != nil {
				logging.Fatal("backfill", "scan: %v", err)
			}
			batch = append(batch, referralcontract.Account{
				ActorType: referralcontract.ActorHauling, ActorID: id, Phone: phone, Email: email,
				LocationState: state, SignupAt: createdAt, IsNewAccount: false,
			})
			after = id
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			for _, account := range batch {
				if err := feed.AccountRegistered(ctx, tx, account); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			logging.Fatal("backfill", "append batch ending %s: %v", after, err)
		}
		announced += len(batch)
		logging.Notice("backfill", "announced %d providers so far", announced)
	}
	logging.Success("backfill", "done: %d providers queued for referral-service", announced)
}
