// Command referral-backfill announces every existing dispatch rider to the
// referral programme once, so riders who signed up before the programme
// launched get a referral code too. Backfilled accounts are marked as not new
// and can never be attributed to a referrer. (Approvals are announced by the
// running service's approval sweep, not here.)
//
// It only writes to this service's referral_outbox; the running service's
// relay (DISPATCH_REFERRAL_FEED_ENABLED=true) delivers the events. Re-running
// is safe: referral-service ignores what it already knows.
//
//	cd services/driver-dispatch-delivery-service && go run ./cmd/referral-backfill
package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
	"cosmicforge/logistics/shared/go/referralcontract"
)

const batchSize = 500

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()
	feed, err := referralcontract.NewFeed(db, "driver-dispatch-delivery-service", cfg.ReferralIdentityPepper)
	if err != nil {
		log.Fatalf("DISPATCH_REFERRAL_IDENTITY_PEPPER is required: %v", err)
	}

	announced, after := 0, ""
	for {
		rows, err := db.Query(ctx, `
			SELECT i.id::text, i.phone_number, COALESCE(i.email, ''), i.created_at, COALESCE(p.state, '')
			FROM dispatch_rider_identities i
			LEFT JOIN providers p ON p.id = i.id
			WHERE i.id::text > $1
			ORDER BY i.id::text LIMIT $2`, after, batchSize)
		if err != nil {
			log.Fatalf("read riders: %v", err)
		}
		var batch []referralcontract.Account
		for rows.Next() {
			var id, phone, email, state string
			var createdAt time.Time
			if err := rows.Scan(&id, &phone, &email, &createdAt, &state); err != nil {
				log.Fatalf("scan: %v", err)
			}
			batch = append(batch, referralcontract.Account{
				ActorType: referralcontract.ActorDispatch, ActorID: id, Phone: phone, Email: email,
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
			log.Fatalf("append batch ending %s: %v", after, err)
		}
		announced += len(batch)
		log.Printf("announced %d riders so far", announced)
	}
	log.Printf("done: %d riders queued for referral-service", announced)
}
