// Command referral-backfill announces every existing customer to the
// referral programme once, so accounts created before the programme launched
// get a referral code too. Backfilled accounts are marked as not new and can
// never be attributed to a referrer.
//
// It only writes to this service's referral_outbox; the running service's
// relay (CUSTOMER_REFERRAL_FEED_ENABLED=true) delivers the events. Re-running
// is safe: referral-service ignores accounts it already knows.
//
//	cd services/customer-service && go run ./cmd/referral-backfill
package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/customer-service/internal/config"
	"cosmicforge/logistics/services/customer-service/internal/database"
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
	feed, err := referralcontract.NewFeed(db, "customer-service", cfg.ReferralIdentityPepper)
	if err != nil {
		logging.Fatal("backfill", "CUSTOMER_REFERRAL_IDENTITY_PEPPER is required: %v", err)
	}

	announced, after := 0, ""
	for {
		rows, err := db.Query(ctx, `
			SELECT id::text, COALESCE(phone, ''), COALESCE(email, ''), created_at
			FROM customers WHERE id::text > $1 ORDER BY id::text LIMIT $2`, after, batchSize)
		if err != nil {
			logging.Fatal("backfill", "read customers: %v", err)
		}
		var batch []referralcontract.Account
		for rows.Next() {
			var id, phone, email string
			var createdAt time.Time
			if err := rows.Scan(&id, &phone, &email, &createdAt); err != nil {
				logging.Fatal("backfill", "scan: %v", err)
			}
			batch = append(batch, referralcontract.Account{
				ActorType: referralcontract.ActorCustomer, ActorID: id,
				Phone: phone, Email: email, SignupAt: createdAt, IsNewAccount: false,
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
		logging.Notice("backfill", "announced %d customers so far", announced)
	}
	logging.Success("backfill", "done: %d customers queued for referral-service", announced)
}
