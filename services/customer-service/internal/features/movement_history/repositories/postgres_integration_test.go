package movementrepositories_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	movementmodels "cosmicforge/logistics/services/customer-service/internal/features/movement_history/models"
	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	"cosmicforge/logistics/shared/go/events"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: CUSTOMER_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations")
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, _ := os.ReadFile(filepath.Join(dir, name))
		if _, err := p.Exec(context.Background(), string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return p
}

func project(t *testing.T, repo *movementrepositories.PostgresRepository, eventID string, version int64, s events.MovementSummary) string {
	t.Helper()
	envelope, err := events.NewMovementEnvelope(eventID, "taxi.booking.started", "driver-taxi-service", "taxi_booking", version, time.Now(), "", "", s)
	if err != nil {
		t.Fatal(err)
	}
	movement, err := movementmodels.FromEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := repo.Project(context.Background(), envelope, movement)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestPostgresProjectionDedupeStaleAndTerminal(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	customer, ref := uuid.NewString(), "booking-"+uuid.NewString()
	s := events.MovementSummary{CustomerID: customer, Authority: "TAXI", SourceRef: ref, MovementType: "PASSENGER", State: "SEARCHING"}
	prefix := uuid.NewString()
	if got := project(t, repo, prefix+"-1", 1, s); got != movementmodels.OutcomeApplied {
		t.Fatalf("first = %s", got)
	}
	if got := project(t, repo, prefix+"-1", 1, s); got != movementmodels.OutcomeDuplicate {
		t.Fatalf("replay = %s", got)
	}
	s.State = "COMPLETED"
	if got := project(t, repo, prefix+"-3", 3, s); got != movementmodels.OutcomeApplied {
		t.Fatalf("completed = %s", got)
	}
	s.State = "ASSIGNED"
	if got := project(t, repo, prefix+"-2", 2, s); got != movementmodels.OutcomeStale {
		t.Fatalf("stale = %s", got)
	}
	if got := project(t, repo, prefix+"-4", 4, s); got != movementmodels.OutcomeTerminalProtected {
		t.Fatalf("terminal regression = %s", got)
	}
	row, err := repo.Get(context.Background(), customer, "TAXI", ref)
	if err != nil || row.State != "COMPLETED" || row.SourceVersion != 3 {
		t.Fatalf("row = %+v %v", row, err)
	}
	var inbox int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM customer_movement_event_inbox WHERE event_id LIKE $1`, prefix+"%").Scan(&inbox); err != nil || inbox != 4 {
		t.Fatalf("inbox rows = %d %v (duplicate must not add a row)", inbox, err)
	}
}
