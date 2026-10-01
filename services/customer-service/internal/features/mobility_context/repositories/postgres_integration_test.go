package mobilityrepositories_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
)

// integrationPool requires an explicit, disposable database. It never falls
// back to a default DSN.
func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: CUSTOMER_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

func seedCustomer(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	phone := "+23480" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	if err := pool.QueryRow(context.Background(), `INSERT INTO customers (id, phone) VALUES (gen_random_uuid(), $1) RETURNING id::text`, phone).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPostgresMobilityPreferencesVersioning(t *testing.T) {
	pool := integrationPool(t)
	repo := mobilityrepositories.NewPostgresRepository(pool)
	ctx := context.Background()
	customer := seedCustomer(t, pool)
	prefs := mobilitymodels.DefaultPreferences(customer)
	if err := prefs.Normalize(); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.SavePreferences(ctx, prefs, 0)
	if err != nil || saved.Version != 1 {
		t.Fatalf("create = %+v %v", saved, err)
	}
	if _, err := repo.SavePreferences(ctx, prefs, 0); !errors.Is(err, mobilityrepositories.ErrVersionConflict) {
		t.Fatalf("duplicate create err = %v", err)
	}
	if updated, err := repo.SavePreferences(ctx, prefs, 1); err != nil || updated.Version != 2 {
		t.Fatalf("update = %+v %v", updated, err)
	}
	if _, err := repo.SavePreferences(ctx, prefs, 1); !errors.Is(err, mobilityrepositories.ErrVersionConflict) {
		t.Fatalf("stale update err = %v", err)
	}
}

func TestPostgresGuestTravellerOwnershipAndSoftDelete(t *testing.T) {
	pool := integrationPool(t)
	repo := mobilityrepositories.NewPostgresRepository(pool)
	ctx := context.Background()
	a, b := seedCustomer(t, pool), seedCustomer(t, pool)
	guest := mobilitymodels.GuestTraveller{CustomerID: a, Name: "Mum"}
	created, _, err := repo.CreateGuest(ctx, guest, "key-1", "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	if replay, replayed, err := repo.CreateGuest(ctx, guest, "key-1", "fp-1"); err != nil || !replayed || replay.ID != created.ID {
		t.Fatalf("replay = %+v %t %v", replay, replayed, err)
	}
	if _, _, err := repo.CreateGuest(ctx, guest, "key-1", "fp-2"); !errors.Is(err, mobilityrepositories.ErrRequestKeyReused) {
		t.Fatalf("key reuse err = %v", err)
	}
	if _, err := repo.GetGuest(ctx, b, created.ID); err == nil {
		t.Fatal("customer B must not read customer A's guest")
	}
	if err := repo.DeactivateGuest(ctx, b, created.ID); err == nil {
		t.Fatal("customer B must not deactivate customer A's guest")
	}
	if err := repo.DeactivateGuest(ctx, a, created.ID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM customer_guest_travellers WHERE id=$1 AND NOT active`, created.ID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("soft-deleted row count = %d %v", rows, err)
	}
}

func TestPostgresTrustedAndEmergencyContactsShareOneRecord(t *testing.T) {
	pool := integrationPool(t)
	repo := profilerepositories.NewPostgresCustomerRepository(pool)
	ctx := context.Background()
	customer := seedCustomer(t, pool)

	legacy, err := repo.AddEmergencyContact(ctx, customer, "Legacy", "08030000000", "Sister")
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := repo.ListTrustedContacts(ctx, customer)
	if err != nil || len(trusted) != 1 || trusted[0].ID != legacy.ID {
		t.Fatalf("legacy contact not visible as trusted: %+v %v", trusted, err)
	}
	if trusted[0].TripSharingEnabled || !trusted[0].EmergencyAlertsEnabled {
		t.Fatalf("legacy row consent = %+v, want sharing false and alerts true", trusted[0])
	}

	created, _, err := repo.CreateTrustedContact(ctx, profilemodels.TrustedContact{CustomerID: customer, Name: "New", Phone: "+2348031111111"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	emergency, err := repo.GetEmergencyContacts(ctx, customer)
	if err != nil || len(emergency) != 2 {
		t.Fatalf("trusted contact not visible via emergency route: %d %v", len(emergency), err)
	}
	if err := repo.DeactivateTrustedContact(ctx, customer, created.ID); err != nil {
		t.Fatal(err)
	}
	if emergency, _ = repo.GetEmergencyContacts(ctx, customer); len(emergency) != 1 {
		t.Fatalf("deactivated contact still listed by emergency route: %d", len(emergency))
	}
	var tables int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name='customer_trusted_contacts'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("a duplicate trusted contacts table must not exist: %d %v", tables, err)
	}
}
