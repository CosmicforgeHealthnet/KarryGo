package providerauthrepositories

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	providerauthclients "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/clients"
	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// referralTestPool gives the test a private schema with every hauling
// migration applied, in the order the service's own runner uses.
func referralTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL not set; skipping PostgreSQL referral feed test")
	}
	ctx := context.Background()
	schema := "referral_feed_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgxpool.New(ctx, dsn)
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
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, _ := filepath.Glob("../../../../migrations/*.sql")
	sort.Strings(files)
	for _, file := range files {
		if strings.HasSuffix(file, ".down.sql") {
			continue
		}
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), err)
		}
	}
	return pool
}

func providerAnnouncements(t *testing.T, pool *pgxpool.Pool, providerID string) []referralcontract.AccountRegistered {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM referral_outbox WHERE aggregate_id = $1`, "hauling:"+providerID)
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

func TestNewProvidersAreAnnouncedOnce(t *testing.T) {
	pool := referralTestPool(t)
	feed, err := referralcontract.NewFeed(pool, "driver-hauling-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresProviderRepository(pool)
	repo.SetSignupAnnouncer(providerauthclients.NewReferralAnnouncer(feed))
	ctx := context.Background()

	created, err := repo.UpsertByPhone(ctx, "+2348041110001", providerauthmodels.Signup{LinkCode: "LINKCODE", DeviceID: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.UpsertByPhone(ctx, "+2348041110001", providerauthmodels.Signup{TypedCode: "OTHER"})
	if err != nil || again.ID != created.ID {
		t.Fatalf("returning provider: %v", err)
	}
	got := providerAnnouncements(t, pool, created.ID)
	if len(got) != 1 || got[0].LinkCode != "LINKCODE" || got[0].ActorType != referralcontract.ActorHauling ||
		got[0].PhoneHash != referralcontract.HashPhone([]byte("pepper"), "+2348041110001") || !got[0].IsNewAccount {
		t.Fatalf("announcements = %+v, want exactly the first sign-up", got)
	}

	byEmail, err := repo.UpsertByEmail(ctx, "Truck.Driver@example.com", providerauthmodels.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	if got := providerAnnouncements(t, pool, byEmail.ID); len(got) != 1 {
		t.Fatalf("email sign-up announcements = %d, want 1", len(got))
	}
}

// The approval sweep announces each approved, self-registered provider once
// and never a fleet-created driver.
func TestApprovalSweepAnnouncesEachProviderOnce(t *testing.T) {
	pool := referralTestPool(t)
	ctx := context.Background()
	feed, err := referralcontract.NewFeed(pool, "driver-hauling-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresProviderRepository(pool)
	approved, err := repo.UpsertByPhone(ctx, "+2348041110010", providerauthmodels.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := repo.UpsertByPhone(ctx, "+2348041110011", providerauthmodels.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	fleet, err := repo.UpsertByPhone(ctx, "+2348041110012", providerauthmodels.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE truck_providers SET onboarding_status = 'complete' WHERE id IN ($1, $2)`, approved.ID, fleet.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE truck_providers SET managed_by_organization_id = gen_random_uuid() WHERE id = $1`, fleet.ID); err != nil {
		t.Fatal(err)
	}
	_ = pending
	for run, want := range []int{1, 0} {
		n, err := feed.AnnounceApprovals(ctx, pool, providerauthclients.HaulingApprovals, 100)
		if err != nil || n != want {
			t.Fatalf("sweep %d announced %d, %v; want %d", run+1, n, err, want)
		}
	}
	var events int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM referral_outbox WHERE event_type = $1 AND aggregate_id = $2`,
		referralcontract.EventProviderOnboardingCompleted, "hauling:"+approved.ID).Scan(&events)
	if events != 1 {
		t.Fatalf("approval events = %d, want 1", events)
	}
}
