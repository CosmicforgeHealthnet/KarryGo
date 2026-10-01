package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestNewStoreRejectsUnsafeTableNames(t *testing.T) {
	for _, bad := range []string{"", "Outbox", "a b", "x;drop table y", "1abc", strings.Repeat("a", 64)} {
		if _, err := NewStore(nil, bad); err == nil {
			t.Fatalf("NewStore(%q) accepted an unsafe table name", bad)
		}
	}
	if _, err := NewStore(nil, "referral_outbox"); err != nil {
		t.Fatalf("NewStore(referral_outbox) error = %v", err)
	}
}

func TestNewEnvelopeRequiresIdentity(t *testing.T) {
	if _, err := NewEnvelope("", "t", "a", "1", 1, nil); err == nil {
		t.Fatal("expected missing producer to be rejected")
	}
	env, err := NewEnvelope("payment-wallet-service", "x.y.v1", "payment_intent", "pi_1", 1, map[string]int{"n": 1})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	if !strings.HasPrefix(env.EventID, "evt_") || env.SchemaVersion != 1 || string(env.Payload) != `{"n":1}` {
		t.Fatalf("unexpected envelope %+v", env)
	}
}

func TestHTTPPublisherSignsAndTreatsConflictAsDelivered(t *testing.T) {
	secret := []byte("s3cret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"payment-wallet-service": secret}, time.Minute)
	status := http.StatusOK
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := verifier.VerifyRequest(r)
		if err != nil || caller != "payment-wallet-service" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		if r.Header.Get("X-Event-ID") != "evt_1" || r.URL.Path != "/internal/referral/v1/events" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
	}))
	defer server.Close()

	publisher, err := NewHTTPPublisher(server.URL, "/internal/referral/v1/events", "payment-wallet-service", secret, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPPublisher() error = %v", err)
	}
	record := Record{ID: "evt_1", Payload: json.RawMessage(`{"eventId":"evt_1"}`)}
	for _, code := range []int{http.StatusOK, http.StatusAccepted, http.StatusConflict} {
		status = code
		if err := publisher.Publish(context.Background(), record); err != nil {
			t.Fatalf("status %d: Publish() error = %v", code, err)
		}
	}
	if gotBody != `{"eventId":"evt_1"}` {
		t.Fatalf("consumer received %q", gotBody)
	}
	status = http.StatusInternalServerError
	if err := publisher.Publish(context.Background(), record); err == nil {
		t.Fatal("expected a 500 to be retried")
	}
}

// TestStoreAgainstPostgres exercises append, typed claims, leases, backoff
// and dead-lettering against real SQL. Gated on OUTBOX_TEST_DATABASE_URL.
func TestStoreAgainstPostgres(t *testing.T) {
	rawURL := os.Getenv("OUTBOX_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("OUTBOX_TEST_DATABASE_URL not set; skipping PostgreSQL outbox test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, rawURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	table := "outbox_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %[1]s (
			id text PRIMARY KEY,
			event_type text NOT NULL,
			schema_version int NOT NULL CHECK (schema_version > 0),
			aggregate_type text NOT NULL,
			aggregate_id text NOT NULL,
			payload jsonb NOT NULL,
			status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','DEAD_LETTERED')),
			available_at timestamptz NOT NULL DEFAULT now(),
			attempts int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			lease_owner text,
			lease_expires_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			published_at timestamptz,
			last_error_redacted text NOT NULL DEFAULT ''
		)`, table)); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+table) })

	store, err := NewStore(pool, table)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	// An append inside a rolled-back transaction must leave nothing behind.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	rolledBack, _ := NewEnvelope("p", "a.v1", "agg", "1", 1, map[string]string{})
	if err := store.Append(ctx, tx, rolledBack); err != nil {
		t.Fatalf("append in tx: %v", err)
	}
	_ = tx.Rollback(ctx)

	for i, eventType := range []string{"a.v1", "b.v1", "a.v1"} {
		env, _ := NewEnvelope("p", eventType, "agg", fmt.Sprint(i), 1, map[string]int{"i": i})
		if err := store.Append(ctx, pool, env); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	pending, dead, err := store.Counts(ctx)
	if err != nil || pending != 3 || dead != 0 {
		t.Fatalf("Counts() = %d, %d, %v; want 3 pending (rolled-back append must not persist)", pending, dead, err)
	}

	// A typed claim only takes its own event type.
	claimed, err := store.ClaimBatch(ctx, "relay-a", time.Minute, 10, []string{"b.v1"})
	if err != nil || len(claimed) != 1 || claimed[0].EventType != "b.v1" {
		t.Fatalf("typed claim = %+v, %v", claimed, err)
	}
	// A leased row cannot be claimed by a second relay.
	other, err := store.ClaimBatch(ctx, "relay-b", time.Minute, 10, nil)
	if err != nil || len(other) != 2 {
		t.Fatalf("second relay claimed %d rows, %v; want the 2 unleased ones", len(other), err)
	}
	// Only the lease owner may mark a row.
	if err := store.MarkPublished(ctx, "relay-b", claimed[0].ID); err == nil {
		t.Fatal("non-owner was allowed to mark a row published")
	}
	if err := store.MarkPublished(ctx, "relay-a", claimed[0].ID); err != nil {
		t.Fatalf("MarkPublished() error = %v", err)
	}

	// Failure schedules a backoff; exhausting attempts dead-letters the row.
	if err := store.MarkFailed(ctx, "relay-b", other[0].ID, errors.New("boom"), 1); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	if err := store.MarkFailed(ctx, "relay-b", other[1].ID, errors.New("boom"), 5); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	var status string
	var availableInFuture bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT status, available_at > now() FROM %s WHERE id = $1`, table), other[1].ID).Scan(&status, &availableInFuture); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != StatusPending || !availableInFuture {
		t.Fatalf("retried row status=%s availableInFuture=%v; want PENDING with backoff", status, availableInFuture)
	}
	pending, dead, _ = store.Counts(ctx)
	if pending != 1 || dead != 1 {
		t.Fatalf("Counts() = %d pending, %d dead; want 1 and 1", pending, dead)
	}
	if n, err := store.ReplayDeadLettered(ctx, 10); err != nil || n != 1 {
		t.Fatalf("ReplayDeadLettered() = %d, %v", n, err)
	}
	pending, dead, _ = store.Counts(ctx)
	if pending != 2 || dead != 0 {
		t.Fatalf("after replay Counts() = %d pending, %d dead; want 2 and 0", pending, dead)
	}
}
