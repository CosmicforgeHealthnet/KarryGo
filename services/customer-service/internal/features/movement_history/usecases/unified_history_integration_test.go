package movementusecases_test

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

	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	movementusecases "cosmicforge/logistics/services/customer-service/internal/features/movement_history/usecases"
	"cosmicforge/logistics/shared/go/events"
)

func historyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: CUSTOMER_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations")
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, _ := os.ReadFile(filepath.Join(dir, name))
		if _, err := pool.Exec(context.Background(), string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

// The unified movement history: one customer, three execution authorities and
// Vendor's grouping, ending in a single ordered history the customer can read.
//
// Each authority publishes for itself and only for itself, which is what
// makes one history safe to assemble from four independent publishers.
func TestPostgresUnifiedMovementHistoryAcrossAuthorities(t *testing.T) {
	pool := historyPool(t)
	ctx := context.Background()
	repo := movementrepositories.NewPostgresRepository(pool)
	callers, err := movementusecases.ParseCallerAuthorities(
		"driver-taxi-service=TAXI;driver-dispatch-delivery-service=DISPATCH;driver-hauling-service=HAULING")
	if err != nil {
		t.Fatal(err)
	}
	service := movementusecases.NewService(movementusecases.Options{
		Repository: repo, Callers: callers, ConsumerEnabled: true, ReadEnabled: true,
		GroupingCallers: []string{"vendor-service"},
	})

	customer := uuid.NewString()
	parent := "order-" + uuid.NewString()
	rides := map[string]string{
		events.MovementAuthorityTaxi:     "taxi-" + uuid.NewString(),
		events.MovementAuthorityDispatch: "dispatch-" + uuid.NewString(),
		events.MovementAuthorityHauling:  "haulage-" + uuid.NewString(),
	}
	publisher := map[string]string{
		events.MovementAuthorityTaxi:     "driver-taxi-service",
		events.MovementAuthorityDispatch: "driver-dispatch-delivery-service",
		events.MovementAuthorityHauling:  "driver-hauling-service",
	}
	movementType := map[string]string{
		events.MovementAuthorityTaxi:     events.MovementTypeGoods,
		events.MovementAuthorityDispatch: events.MovementTypeDispatch,
		events.MovementAuthorityHauling:  events.MovementTypeHaulage,
	}
	eventName := map[string]string{
		events.MovementAuthorityTaxi:     events.TaxiBookingCompleted,
		events.MovementAuthorityDispatch: "dispatch.delivery.completed",
		events.MovementAuthorityHauling:  "hauling.haulage.completed",
	}

	publish := func(authority, state string, version int64, occurredAt time.Time) (events.Envelope, error) {
		summary := events.MovementSummary{
			CustomerID: customer, Authority: authority, SourceRef: rides[authority],
			MovementType: movementType[authority], State: state, FulfilmentGroupRef: "group-" + authority,
		}
		envelope, err := events.NewMovementEnvelope(uuid.NewString(), eventName[authority], publisher[authority],
			"movement", version, occurredAt, parent, "", summary)
		if err != nil {
			return events.Envelope{}, err
		}
		_, err = service.Ingest(ctx, publisher[authority], envelope)
		return envelope, err
	}

	base := time.Now().Add(-time.Hour)
	order := []string{events.MovementAuthorityHauling, events.MovementAuthorityDispatch, events.MovementAuthorityTaxi}
	for index, authority := range order {
		if _, err = publish(authority, events.MovementStateCompleted, 2, base.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatalf("%s: %v", authority, err)
		}
	}

	// An authority may publish only its own movements.
	crossed := events.MovementSummary{CustomerID: customer, Authority: events.MovementAuthorityHauling,
		SourceRef: rides[events.MovementAuthorityHauling], MovementType: events.MovementTypeHaulage, State: events.MovementStateCancelled}
	envelope, err := events.NewMovementEnvelope(uuid.NewString(), "hauling.haulage.cancelled", "driver-taxi-service",
		"movement", 9, time.Now(), parent, "", crossed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Ingest(ctx, "driver-taxi-service", envelope); err == nil {
		t.Fatal("Taxi published a Haulage movement")
	}
	// A grouping publisher is not a movement authority.
	if _, err = service.Ingest(ctx, "vendor-service", envelope); err == nil {
		t.Fatal("Vendor published a movement")
	}

	// Vendor groups the three movements into one purchase.
	grouping := events.MovementGrouping{CustomerID: customer, ParentContextType: "COMMERCE_ORDER", ParentContextRef: parent,
		FulfilmentPlanRef: "plan-" + parent}
	for index, authority := range []string{events.MovementAuthorityTaxi, events.MovementAuthorityDispatch, events.MovementAuthorityHauling} {
		grouping.Missions = append(grouping.Missions, events.GroupingMission{
			FulfilmentGroupRef: "group-" + authority, MissionSequence: index + 1, MissionCount: 3,
		})
	}
	groupingEnvelope, err := events.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parent, 4, time.Now(), parent, grouping)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.IngestGrouping(ctx, "vendor-service", groupingEnvelope); err != nil {
		t.Fatalf("grouping: %v", err)
	}
	// A movement authority may not publish grouping either.
	if _, err = service.IngestGrouping(ctx, "driver-taxi-service", groupingEnvelope); err == nil {
		t.Fatal("Taxi published commerce grouping")
	}

	// One history, newest first, every authority present and grouped.
	history, page, err := service.List(ctx, customer, movementusecases.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(history) != 3 {
		t.Fatalf("history = %d rows (total %d), want the customer's three movements", len(history), page.Total)
	}
	seen := map[string]bool{}
	for index, movement := range history {
		seen[movement.Authority] = true
		if movement.State != events.MovementStateCompleted {
			t.Fatalf("%s state = %s", movement.Authority, movement.State)
		}
		if movement.ParentContextRef == nil || *movement.ParentContextRef != parent {
			t.Fatalf("%s is not grouped into the purchase: %v", movement.Authority, movement.ParentContextRef)
		}
		if movement.MissionCount == nil || *movement.MissionCount != 3 {
			t.Fatalf("%s mission count = %v", movement.Authority, movement.MissionCount)
		}
		if index > 0 && history[index-1].LastEventAt.Before(movement.LastEventAt) {
			t.Fatal("history is not ordered newest first")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("authorities in history = %v", seen)
	}

	// Filtering by the purchase returns exactly that purchase's movements.
	grouped, page, err := service.List(ctx, customer, movementusecases.ListQuery{ParentContextRef: parent})
	if err != nil || page.Total != 3 || len(grouped) != 3 {
		t.Fatalf("purchase filter = %d rows (total %d) %v", len(grouped), page.Total, err)
	}

	// A stale republish of one movement never rewinds the history.
	if _, err = publish(events.MovementAuthorityTaxi, events.MovementStateInProgress, 1, time.Now()); err != nil {
		t.Fatalf("stale republish must be acknowledged: %v", err)
	}
	taxi, err := service.Get(ctx, customer, events.MovementAuthorityTaxi, rides[events.MovementAuthorityTaxi])
	if err != nil {
		t.Fatal(err)
	}
	if taxi.State != events.MovementStateCompleted {
		t.Fatalf("stale event rewound the movement to %s", taxi.State)
	}

	// Reconciliation answers each authority about its own movements only.
	versions, err := service.ProjectedVersions(ctx, "driver-taxi-service", []string{rides[events.MovementAuthorityTaxi], rides[events.MovementAuthorityDispatch]})
	if err != nil {
		t.Fatal(err)
	}
	if versions[rides[events.MovementAuthorityTaxi]] != 2 {
		t.Fatalf("Taxi reconciliation = %v", versions)
	}
	if _, ok := versions[rides[events.MovementAuthorityDispatch]]; ok {
		t.Fatal("Taxi read a Dispatch movement through reconciliation")
	}
}
