package authrepositories

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// referralTestPool gives the test a private schema holding just the rider
// identity tables and the referral outbox.
func referralTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_TEST_DATABASE_URL or TEST_DATABASE_URL to run the referral feed test")
	}
	ctx := context.Background()
	schema := "referral_feed_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, file := range []string{"001_dispatch_rider_auth.sql", "002_dispatch_rider_auth_alter.sql", "003_dispatch_rider_auth_signup.sql",
		"000004_create_providers.up.sql", "000011_create_verification_audit.up.sql", "000012_expand_verification_audit_actions.up.sql",
		"000013_add_fully_approved_audit.up.sql", "000068_referral_outbox.up.sql"} {
		sql, err := os.ReadFile("../../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return pool
}

func riderAnnouncements(t *testing.T, pool *pgxpool.Pool, riderID string) []referralcontract.AccountRegistered {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM referral_outbox WHERE aggregate_id = $1`, "dispatch:"+riderID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []referralcontract.AccountRegistered
	for rows.Next() {
		var raw []byte
		_ = rows.Scan(&raw)
		var envelope outbox.Envelope
		_ = json.Unmarshal(raw, &envelope)
		var payload referralcontract.AccountRegistered
		_ = json.Unmarshal(envelope.Payload, &payload)
		out = append(out, payload)
	}
	return out
}

// Both creation paths announce exactly once; a returning rider announces
// nothing.
func TestNewRidersAreAnnouncedOnce(t *testing.T) {
	pool := referralTestPool(t)
	feed, err := referralcontract.NewFeed(pool, "driver-dispatch-delivery-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresIdentityRepository(pool)
	repo.SetSignupAnnouncer(authclients.NewReferralAnnouncer(feed))
	ctx := context.Background()

	signedUp, err := repo.CreateForSignup(ctx, "+2348031110001", "rider@example.com", authmodels.Signup{TypedCode: "abcd-efgh", DeviceID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	got := riderAnnouncements(t, pool, signedUp.ID)
	if len(got) != 1 || got[0].TypedCode != "abcd-efgh" || got[0].ActorType != referralcontract.ActorDispatch ||
		got[0].EmailHash != referralcontract.HashEmail([]byte("pepper"), "rider@example.com") || !got[0].IsNewAccount {
		t.Fatalf("signup announcements = %+v", got)
	}

	legacy, err := repo.UpsertByPhone(ctx, "+2348031110002", authmodels.Signup{LinkCode: "LINK"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.UpsertByPhone(ctx, "+2348031110002", authmodels.Signup{LinkCode: "OTHER"})
	if err != nil || again.ID != legacy.ID {
		t.Fatalf("returning rider: %v (id %s vs %s)", err, again.ID, legacy.ID)
	}
	if got := riderAnnouncements(t, pool, legacy.ID); len(got) != 1 || got[0].LinkCode != "LINK" {
		t.Fatalf("legacy announcements = %+v, want exactly the first sign-up", got)
	}

	// A duplicate sign-up is a conflict and must leave no announcement behind.
	if _, err := repo.CreateForSignup(ctx, "+2348031110001", "", authmodels.Signup{}); err == nil {
		t.Fatal("expected a conflict for an existing phone")
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM referral_outbox`).Scan(&n)
	if n != 2 {
		t.Fatalf("outbox rows = %d, want 2", n)
	}
}

// The approval sweep announces each approved rider exactly once, using the
// fully_approved audit time, and ignores riders who are not approved yet.
func TestApprovalSweepAnnouncesEachRiderOnce(t *testing.T) {
	pool := referralTestPool(t)
	ctx := context.Background()
	feed, err := referralcontract.NewFeed(pool, "driver-dispatch-delivery-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	approved, pending := uuid.NewString(), uuid.NewString()
	for i, id := range []string{approved, pending} {
		if _, err := pool.Exec(ctx, `INSERT INTO providers (id, phone, verification_status, onboarding_complete) VALUES ($1, $2, $3, $4)`,
			id, "+23480500000"+string(rune('1'+i)), map[bool]string{true: "verified", false: "pending_review"}[id == approved], id == approved); err != nil {
			t.Fatal(err)
		}
	}
	approvedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	if _, err := pool.Exec(ctx, `INSERT INTO verification_audit (provider_id, step, action, from_status, to_status, created_at)
		VALUES ($1, 'all', 'fully_approved', 'pending', 'approved', $2)`, approved, approvedAt); err != nil {
		t.Fatal(err)
	}
	for run, want := range []int{1, 0} {
		n, err := feed.AnnounceApprovals(ctx, pool, authclients.DispatchApprovals, 100)
		if err != nil || n != want {
			t.Fatalf("sweep %d announced %d, %v; want %d", run+1, n, err, want)
		}
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM referral_outbox WHERE event_type = $1 AND aggregate_id = $2`,
		referralcontract.EventProviderOnboardingCompleted, "dispatch:"+approved).Scan(&raw); err != nil {
		t.Fatalf("approval event for the approved rider: %v", err)
	}
	var envelope outbox.Envelope
	_ = json.Unmarshal(raw, &envelope)
	var payload referralcontract.ProviderOnboardingCompleted
	_ = json.Unmarshal(envelope.Payload, &payload)
	if !payload.CompletedAt.Equal(approvedAt) {
		t.Fatalf("approval time = %s, want the fully_approved audit time %s", payload.CompletedAt, approvedAt)
	}
}
