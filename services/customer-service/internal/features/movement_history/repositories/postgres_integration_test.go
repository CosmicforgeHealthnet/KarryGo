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

// Commerce grouping decorates movements in BOTH arrival orders, because
// nothing coordinates Vendor's publish with the executing authorities'. It
// adds commercial context and never touches state, timestamps or provider
// history.
func TestPostgresCommerceGroupingDecoratesMovementsInEitherOrder(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	ctx := context.Background()
	customer := uuid.NewString()
	parent := "order-" + uuid.NewString()
	early, late := "grp-"+uuid.NewString(), "grp-"+uuid.NewString()

	grouping := events.MovementGrouping{CustomerID: customer, ProductCode: "FOOD", ParentContextType: "COMMERCE_ORDER", ParentContextRef: parent,
		FulfilmentPlanRef: "plan-1", Missions: []events.GroupingMission{
			{FulfilmentGroupRef: early, MissionSequence: 1, MissionCount: 2},
			{FulfilmentGroupRef: late, MissionSequence: 2, MissionCount: 2},
		}}

	// One movement is already projected when the grouping arrives.
	first := events.MovementSummary{CustomerID: customer, Authority: events.MovementAuthorityTaxi, SourceRef: "ride-" + early,
		MovementType: events.MovementTypeGoods, State: events.MovementStateInProgress, FulfilmentGroupRef: early}
	if outcome := project(t, repo, uuid.NewString(), 1, first); outcome != movementmodels.OutcomeApplied {
		t.Fatalf("first movement = %s", outcome)
	}
	envelope, err := events.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parent, 7, time.Now(), parent, grouping)
	if err != nil {
		t.Fatal(err)
	}
	outcome, applied, err := repo.ApplyGrouping(ctx, envelope, grouping)
	if err != nil || outcome != "APPLIED" || applied != 1 {
		t.Fatalf("grouping = %s/%d (%v)", outcome, applied, err)
	}

	// The second movement arrives after the grouping and picks it up.
	second := events.MovementSummary{CustomerID: customer, Authority: events.MovementAuthorityHauling, SourceRef: "haul-" + late,
		MovementType: events.MovementTypeHaulage, State: events.MovementStateAssigned, FulfilmentGroupRef: late}
	if outcome := project(t, repo, uuid.NewString(), 1, second); outcome != movementmodels.OutcomeApplied {
		t.Fatalf("second movement = %s", outcome)
	}

	for _, want := range []struct {
		authority, sourceRef string
		sequence             int
	}{{events.MovementAuthorityTaxi, "ride-" + early, 1}, {events.MovementAuthorityHauling, "haul-" + late, 2}} {
		movement, err := repo.Get(ctx, customer, want.authority, want.sourceRef)
		if err != nil {
			t.Fatal(err)
		}
		if movement.ParentContextRef == nil || *movement.ParentContextRef != parent {
			t.Fatalf("%s parent = %v, want %s", want.sourceRef, movement.ParentContextRef, parent)
		}
		if movement.ProductCode == nil || *movement.ProductCode != "FOOD" {
			t.Fatalf("commerce classification = %v", movement.ProductCode)
		}
		if movement.MissionSequence == nil || *movement.MissionSequence != want.sequence || movement.MissionCount == nil || *movement.MissionCount != 2 {
			t.Fatalf("%s grouping = %v/%v", want.sourceRef, movement.MissionSequence, movement.MissionCount)
		}
	}

	// A replayed grouping is acknowledged without reapplying, and a stale one
	// never rewrites a newer grouping.
	if outcome, _, err := repo.ApplyGrouping(ctx, envelope, grouping); err != nil || outcome != "DUPLICATE" {
		t.Fatalf("grouping replay = %s (%v)", outcome, err)
	}
	stale := grouping
	stale.ParentContextRef = "order-stale"
	staleEnvelope, err := events.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parent, 2, time.Now(), parent, stale)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, applied, err := repo.ApplyGrouping(ctx, staleEnvelope, stale); err != nil || outcome != "STALE" || applied != 0 {
		t.Fatalf("stale grouping = %s/%d (%v)", outcome, applied, err)
	}
	movement, err := repo.Get(ctx, customer, events.MovementAuthorityTaxi, "ride-"+early)
	if err != nil || movement.ParentContextRef == nil || *movement.ParentContextRef != parent {
		t.Fatalf("stale grouping rewrote the projection: %v (%v)", movement.ParentContextRef, err)
	}
	// Grouping decorates, it never becomes state.
	if movement.State != events.MovementStateInProgress {
		t.Fatalf("state = %s, want the authority's own state", movement.State)
	}
	versions, err := repo.ProjectedVersions(ctx, "COMMERCE", []string{parent, first.SourceRef})
	if err != nil || versions[parent] != 7 || versions[first.SourceRef] != 0 {
		t.Fatal("grouping-only reconciliation", versions, err)
	}
	// A newer execution event cannot erase the commerce classification.
	project(t, repo, uuid.NewString(), 2, first)
	movement, err = repo.Get(ctx, customer, first.Authority, first.SourceRef)
	if err != nil || (movement.ProductCode == nil || *movement.ProductCode != "FOOD") {
		t.Fatal("classification lost", movement, err)
	}
	foreign := grouping
	foreign.CustomerID = uuid.NewString()
	foreignEnvelope, _ := events.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parent, 8, time.Now(), parent, foreign)
	if outcome, _, err := repo.ApplyGrouping(ctx, foreignEnvelope, foreign); err != nil || outcome != "STALE" {
		t.Fatal("grouping owner changed", outcome, err)
	}
}

func TestPostgresAllFiveProductFiltersPreserveVariants(t *testing.T) {
	db := pool(t)
	repo := movementrepositories.NewPostgresRepository(db)
	ctx := context.Background()
	owner := uuid.NewString()
	cases := []struct{ family, authority, kind, variant string }{
		{"RIDE", "TAXI", events.MovementTypePassenger, "RIDE_STANDARD"},
		{"SEND", "DISPATCH", events.MovementTypeDispatch, "express"},
		{"TRUCK", "HAULING", events.MovementTypeHaulage, "general"},
		{"FOOD", "DISPATCH", events.MovementTypeDispatch, "FOOD"},
		{"SHOP", "HAULING", events.MovementTypeHaulage, "SHOP"},
	}
	for _, c := range cases {
		project(t, repo, uuid.NewString(), 1, events.MovementSummary{CustomerID: owner, Authority: c.authority, SourceRef: uuid.NewString(), ProductCode: c.variant, MovementType: c.kind, State: events.MovementStateAssigned})
	}
	for _, c := range cases {
		rows, total, err := repo.List(ctx, owner, movementrepositories.Filter{ProductCode: c.family, Page: 1, Limit: 10})
		if err != nil || total != 1 || len(rows) != 1 || (rows[0].ProductCode == nil || *rows[0].ProductCode != c.variant) {
			t.Fatal(c.family, rows, total, err)
		}
	}
}
