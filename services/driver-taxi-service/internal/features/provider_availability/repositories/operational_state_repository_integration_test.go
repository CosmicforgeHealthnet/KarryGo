package availabilityrepositories

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
)

// testDB connects to a real Postgres instance for the guarded-CAS and audit
// tests below, which a Go-level fake cannot exercise honestly: the whole
// point is the database enforcing the version match, not this package's own
// bookkeeping. Skips when no test database is configured, matching the
// TAXI_TEST_DATABASE_URL / HAULING_TEST_DATABASE_URL convention already used
// elsewhere on this platform.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for PostgreSQL operational-state tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedProvider inserts a throwaway taxi_providers row so the operational_state
// foreign key is satisfiable, and registers cleanup.
func seedProvider(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	phone := "+234" + uuid.NewString()[:10]
	if _, err := db.Exec(context.Background(),
		`INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, id, phone); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM taxi_providers WHERE id = $1`, id)
	})
	return id
}

func TestOperationalState_GetReturnsOfflineForUnknownProvider(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)

	state, err := repo.Get(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Current != availabilitymodels.StateOffline || state.Version != 0 {
		t.Fatalf("expected offline/version 0 for a provider with no row, got %+v", state)
	}
}

func TestOperationalState_FirstTransitionSucceedsFromVersionZero(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)
	ctx := context.Background()

	state, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "went_online", nil, 0)
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if state.Current != availabilitymodels.StateAvailable || state.Version != 1 {
		t.Fatalf("expected AVAILABLE/version 1, got %+v", state)
	}
}

// The core guarantee: a stale writer (one that read an old version) must be
// refused, not silently overwrite whatever the fresher writer already did.
// This is the exact defect class the addendum's §35.1 test requirements name
// ("two offers race for one provider, provider ends with at most one active
// booking") one level down, at the state-machine primitive itself.
func TestOperationalState_StaleVersionTransitionIsRejected(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)
	ctx := context.Background()

	first, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "went_online", nil, 0)
	if err != nil {
		t.Fatalf("first Transition: %v", err)
	}

	// A second writer advances the state, believing it still holds version 1.
	second, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateOffered, "offer_sent", nil, first.Version)
	if err != nil {
		t.Fatalf("second Transition: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("expected version 2 after the second transition, got %d", second.Version)
	}

	// A third writer, racing against the second but still holding the OLD
	// version (1), must be refused -- not allowed to clobber the OFFERED state
	// back to something else.
	_, err = repo.Transition(ctx, nil, providerID, availabilitymodels.StateComplianceHold, "stale_write", nil, first.Version)
	if err == nil {
		t.Fatal("a transition against a stale version must be rejected")
	}

	current, err := repo.Get(ctx, providerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if current.Current != availabilitymodels.StateOffered {
		t.Fatalf("the stale write must not have applied; state should still be OFFERED, got %v", current.Current)
	}
}

func TestOperationalState_HistoryRecordsFromAndToState(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)
	ctx := context.Background()

	first, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "went_online", nil, 0)
	if err != nil {
		t.Fatalf("first Transition: %v", err)
	}
	if _, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateOffline, "went_offline", nil, first.Version); err != nil {
		t.Fatalf("second Transition: %v", err)
	}

	rows, err := db.Query(ctx, `
		SELECT from_state, to_state FROM provider_operational_state_history
		WHERE provider_id = $1 ORDER BY changed_at ASC
	`, providerID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	defer rows.Close()

	var pairs [][2]string
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			t.Fatalf("scan: %v", err)
		}
		pairs = append(pairs, [2]string{from, to})
	}

	if len(pairs) != 2 {
		t.Fatalf("expected 2 history rows, got %d: %+v", len(pairs), pairs)
	}
	// First transition: no prior row, so from_state must be OFFLINE (a
	// provider with no row has never transitioned before).
	if pairs[0][0] != string(availabilitymodels.StateOffline) || pairs[0][1] != string(availabilitymodels.StateAvailable) {
		t.Fatalf("first history row wrong: %+v", pairs[0])
	}
	// Second transition: from_state must be the REAL prior state (AVAILABLE),
	// not blank -- this is the exact bug caught and fixed before this test was
	// written (the first draft hardcoded from_state to "").
	if pairs[1][0] != string(availabilitymodels.StateAvailable) || pairs[1][1] != string(availabilitymodels.StateOffline) {
		t.Fatalf("second history row wrong: %+v", pairs[1])
	}
}

// ─── StateAsOf / ListHistory (phase B8, earnings insights) ─────────────────

func TestOperationalState_StateAsOfReturnsEmptyWithNoHistory(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)

	state, err := repo.StateAsOf(context.Background(), providerID, time.Now())
	if err != nil {
		t.Fatalf("StateAsOf: %v", err)
	}
	if state != "" {
		t.Errorf("state = %q, want empty for a provider with no history at all", state)
	}
}

func TestOperationalState_StateAsOfReturnsMostRecentBeforeCutoff(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)
	ctx := context.Background()

	first, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "went_online", nil, 0)
	if err != nil {
		t.Fatalf("first Transition: %v", err)
	}
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond) // ensure the second transition sorts strictly after cutoff
	if _, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateOffline, "went_offline", nil, first.Version); err != nil {
		t.Fatalf("second Transition: %v", err)
	}

	state, err := repo.StateAsOf(ctx, providerID, cutoff)
	if err != nil {
		t.Fatalf("StateAsOf: %v", err)
	}
	if state != string(availabilitymodels.StateAvailable) {
		t.Errorf("state = %q, want AVAILABLE -- the OFFLINE transition happened AFTER cutoff", state)
	}
}

func TestOperationalState_ListHistoryReturnsOnlyRowsInHalfOpenWindow(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresOperationalStateRepository(db)
	providerID := seedProvider(t, db)
	ctx := context.Background()

	from := time.Now()
	time.Sleep(10 * time.Millisecond)
	first, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "went_online", nil, 0)
	if err != nil {
		t.Fatalf("first Transition: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	second, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateOnTrip, "matched", nil, first.Version)
	if err != nil {
		t.Fatalf("second Transition: %v", err)
	}
	until := time.Now()
	time.Sleep(10 * time.Millisecond)
	// A third transition strictly AFTER 'until' must not appear in the result.
	if _, err := repo.Transition(ctx, nil, providerID, availabilitymodels.StateAvailable, "trip_ended", nil, second.Version); err != nil {
		t.Fatalf("third Transition: %v", err)
	}

	entries, err := repo.ListHistory(ctx, providerID, from, until)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want exactly 2 (the third transition is after 'until')", len(entries))
	}
	if entries[0].ToState != string(availabilitymodels.StateAvailable) || entries[1].ToState != string(availabilitymodels.StateOnTrip) {
		t.Errorf("entries = %+v, want [AVAILABLE, ON_TRIP] ascending", entries)
	}
}
