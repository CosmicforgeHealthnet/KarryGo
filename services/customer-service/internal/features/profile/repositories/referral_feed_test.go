package profilerepositories

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	profileclients "cosmicforge/logistics/services/customer-service/internal/features/profile/clients"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// testDB is gated on CUSTOMER_TEST_DATABASE_URL and applies this service's
// migrations (which are written to be re-runnable).
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	rawURL := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("CUSTOMER_TEST_DATABASE_URL not set; skipping PostgreSQL test")
	}
	if !strings.Contains(rawURL, "test") {
		t.Fatal("refusing to run against a database whose name does not contain \"test\"")
	}
	pool, err := pgxpool.New(context.Background(), rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, _ := filepath.Glob("../../../../migrations/*.sql")
	sort.Strings(files)
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return pool
}

func announcements(t *testing.T, pool *pgxpool.Pool, customerID string) []referralcontract.AccountRegistered {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload FROM referral_outbox WHERE aggregate_id = $1`, "customer:"+customerID)
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

func TestNewCustomerIsAnnouncedOnceWithHashesOnly(t *testing.T) {
	pool := testDB(t)
	feed, err := referralcontract.NewFeed(pool, "customer-service", []byte("pepper"))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPostgresCustomerRepository(pool)
	repo.SetSignupAnnouncer(profileclients.NewReferralAnnouncer(feed))
	ctx := context.Background()
	phone := "+23480" + strings.ReplaceAll(uuid.NewString()[:8], "-", "1")

	created, err := repo.UpsertByPhone(ctx, phone, profilemodels.Signup{LinkCode: "LINKCODE", TypedCode: "typed", DeviceID: "device-1"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.UpsertByPhone(ctx, phone, profilemodels.Signup{TypedCode: "OTHER"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != created.ID {
		t.Fatal("a returning login must return the same customer")
	}
	got := announcements(t, pool, created.ID)
	if len(got) != 1 {
		t.Fatalf("announcements = %d, want exactly 1 (a returning login announces nothing)", len(got))
	}
	a := got[0]
	if a.PhoneHash != referralcontract.HashPhone([]byte("pepper"), phone) || a.LinkCode != "LINKCODE" || a.TypedCode != "typed" || !a.IsNewAccount || a.DeviceHash == "" {
		t.Fatalf("unexpected announcement %+v", a)
	}
	var raw string
	_ = pool.QueryRow(ctx, `SELECT payload::text FROM referral_outbox WHERE aggregate_id = $1`, "customer:"+created.ID).Scan(&raw)
	if strings.Contains(raw, phone) || strings.Contains(raw, "device-1") {
		t.Fatal("a raw phone number or device id left customer-service")
	}
}

type failingAnnouncer struct{}

func (failingAnnouncer) AnnounceCustomer(context.Context, pgx.Tx, profilemodels.Customer, profilemodels.Signup) error {
	return errors.New("outbox unavailable")
}

func TestFailedAnnouncementRollsBackTheAccount(t *testing.T) {
	pool := testDB(t)
	repo := NewPostgresCustomerRepository(pool)
	repo.SetSignupAnnouncer(failingAnnouncer{})
	email := "ada-" + uuid.NewString() + "@example.com"
	if _, err := repo.UpsertByEmail(context.Background(), email, profilemodels.Signup{}); err == nil {
		t.Fatal("expected the sign-up to fail with its announcement")
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM customers WHERE email = $1`, email).Scan(&n)
	if n != 0 {
		t.Fatal("the account was created without its referral announcement")
	}
}
