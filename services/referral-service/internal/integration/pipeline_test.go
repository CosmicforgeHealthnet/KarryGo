package integration_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// producerPool is a stand-in producer database: a private schema holding only
// a referral_outbox table, exactly as each producing service creates it.
func producerPool(t *testing.T, referralPool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	schema := "producer_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := referralPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = referralPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") })
	cfg := referralPool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
		CREATE TABLE referral_outbox (
			id text PRIMARY KEY, event_type text NOT NULL, schema_version int NOT NULL,
			aggregate_type text NOT NULL, aggregate_id text NOT NULL, payload jsonb NOT NULL,
			status text NOT NULL DEFAULT 'PENDING', available_at timestamptz NOT NULL DEFAULT now(),
			attempts int NOT NULL DEFAULT 0, lease_owner text, lease_expires_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz,
			last_error_redacted text NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	return pool
}

// The whole pipe: a producer's feed appends to its outbox, its relay delivers
// over signed HTTP to referral-service's real routes, and a referral results.
func TestProducerFeedRelaysEndToEnd(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	secret := []byte("customer-feed-secret")
	server := httptest.NewServer(signedRouter(t, pool, svc, serviceauth.Secrets{"customer-service": secret}))
	defer server.Close()

	producer := producerPool(t, pool)
	pepper := []byte("shared-pepper")
	feed, err := referralcontract.NewFeed(producer, "customer-service", pepper)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := feed.Relay(server.URL, secret, 5*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	announce := func(id, phone, linkCode string) {
		t.Helper()
		if err := feed.AccountRegistered(ctx, producer, referralcontract.Account{
			ActorType: referralcontract.ActorCustomer, ActorID: id, Phone: phone,
			LinkCode: linkCode, SignupAt: time.Now(), IsNewAccount: true,
		}); err != nil {
			t.Fatal(err)
		}
		if err := relay.RunPass(ctx); err != nil {
			t.Fatalf("relay pass: %v", err)
		}
	}

	ada, bola := "cust_"+uuid.NewString(), "cust_"+uuid.NewString()
	announce(ada, "+2348050000001", "")
	adaCode, err := svc.MyCode(ctx, referralcontract.ActorCustomer, ada)
	if err != nil {
		t.Fatalf("Ada's code after relay: %v", err)
	}
	announce(bola, "+2348050000002", adaCode.Code)

	if n := countRows(t, pool, `SELECT count(*) FROM referrals WHERE code = $1 AND code_source = 'link'`, adaCode.Code); n != 1 {
		t.Fatalf("referrals via the relayed pipe = %d, want 1", n)
	}
	pending, dead, err := mustStore(t, producer).Counts(ctx)
	if err != nil || pending != 0 || dead != 0 {
		t.Fatalf("producer outbox pending=%d dead=%d err=%v; want everything delivered", pending, dead, err)
	}
}

func mustStore(t *testing.T, pool *pgxpool.Pool) *outbox.Store {
	t.Helper()
	store, err := outbox.NewStore(pool, referralcontract.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
