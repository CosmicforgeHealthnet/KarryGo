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

	providerauthclients "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/clients"
	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// referralTestPool gives the test a private schema with every taxi
// migration applied, in the order the service's own runner uses.
func referralTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL not set; skipping PostgreSQL referral feed test")
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
	rows, err := pool.Query(context.Background(), `SELECT payload FROM referral_outbox WHERE aggregate_id = $1`, "taxi:"+providerID)
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

func TestNewDriversAreAnnouncedOnce(t *testing.T) {
	pool := referralTestPool(t)
	feed, err := referralcontract.NewFeed(pool, "driver-taxi-service", []byte("pepper"))
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
	if len(got) != 1 || got[0].LinkCode != "LINKCODE" || got[0].ActorType != referralcontract.ActorTaxi ||
		got[0].PhoneHash != referralcontract.HashPhone([]byte("pepper"), "+2348041110001") || !got[0].IsNewAccount {
		t.Fatalf("announcements = %+v, want exactly the first sign-up", got)
	}

}

// The approval sweep announces a driver once all approval dimensions hold.
func TestApprovalSweepAnnouncesEachDriverOnce(t *testing.T) {
	pool := referralTestPool(t)
	ctx := context.Background()
	feed, err := referralcontract.NewFeed(pool, "driver-taxi-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresProviderRepository(pool)
	driver, err := repo.UpsertByPhone(ctx, "+2348041110020", providerauthmodels.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	// Onboarding complete alone is not approval.
	if _, err := pool.Exec(ctx, `UPDATE taxi_providers SET onboarding_status = 'complete' WHERE id = $1`, driver.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := feed.AnnounceApprovals(ctx, pool, providerauthclients.TaxiApprovals, 100); err != nil || n != 0 {
		t.Fatalf("partially approved driver announced (%d, %v)", n, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE taxi_providers SET identity_verification_status = 'verified',
		vehicle_verification_status = 'verified', document_compliance_status = 'compliant' WHERE id = $1`, driver.ID); err != nil {
		t.Fatal(err)
	}
	for run, want := range []int{1, 0} {
		n, err := feed.AnnounceApprovals(ctx, pool, providerauthclients.TaxiApprovals, 100)
		if err != nil || n != want {
			t.Fatalf("sweep %d announced %d, %v; want %d", run+1, n, err, want)
		}
	}
}
