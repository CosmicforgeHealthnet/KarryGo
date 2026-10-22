package bookingusecases

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/taxi-service/internal/features/admininterventions"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	marketrepositories "cosmicforge/logistics/services/taxi-service/internal/features/markets/repositories"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/admincommand"
	ownerfixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
)

// TestFindEligibleCandidates_ExcludesAdministrativelyRestricted proves that
// a Taxi-owned administrative intervention (recorded through the real owner
// executor against the real migrated schema) removes the provider/vehicle
// from the real matcher candidate filter, and that lifting restores them.
func TestFindEligibleCandidates_ExcludesAdministrativelyRestricted(t *testing.T) {
	db := ownerfixture.Database(t, "../../../../migrations")
	addr := os.Getenv("TAXI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TAXI_TEST_REDIS_ADDR is required for matching integration tests")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()
	marketID, _ := seedFixtureForMatching(t, db)
	providerID := seedCompliantOnlineProvider(t, db, rdb, marketID, 1.0005, -9.9995)
	vehicleID := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,seats,status) VALUES($1,$2,'economy',$3,4,'active')`, vehicleID, providerID, "PL-"+vehicleID[:8]); err != nil {
		t.Fatal(err)
	}
	store := availabilityrepositories.NewRedisAvailabilityStore(rdb)
	if err := store.SetOnline(ctx, availabilityrepositories.ProviderStatus{ProviderID: providerID, VehicleID: vehicleID, MarketID: marketID, VehicleClass: "economy", Lat: 1.0005, Lng: -9.9995}); err != nil {
		t.Fatal(err)
	}
	svc := NewBookingService(Options{
		Bookings: bookingrepositories.NewPostgresBookingRepository(db), Markets: marketrepositories.NewPostgresMarketRepository(db),
		Pricing: newFakePricing(), OperationalState: availabilityrepositories.NewPostgresOperationalStateRepository(db),
		Availability: store, Providers: providerauthrepositories.NewPostgresProviderRepository(db),
		AdminGate: admininterventions.NewGate(db),
	})
	booking := bookingmodels.Booking{ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy", Stops: []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}}}
	offered := func() bool {
		for _, c := range svc.findEligibleCandidates(ctx, booking, 15) {
			if c.ProviderID == providerID {
				return true
			}
		}
		return false
	}
	spec, _ := admincommand.ForOwner("driver-taxi-service")
	executor := &admininterventions.Executor{}
	apply := func(kind, id, action string, expected int64, params map[string]string) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		in := admincommand.Execution{Command: admincommand.Command{SubjectKind: kind, SubjectID: id, Action: action, ExpectedVersion: expected, ReasonCode: "SAFETY_REVIEW", Params: params}, Spec: spec.Actions[action], DecisionID: uuid.NewString(), Staff: "staff-a", Now: time.Now().UTC()}
		if _, err := executor.Apply(ctx, tx, in); err != nil {
			t.Fatal(action, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !offered() {
		t.Fatal("unrestricted provider not a candidate")
	}
	apply("providers", providerID, "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "NO_NEW_ASSIGNMENTS"})
	if offered() {
		t.Fatal("NO_NEW_ASSIGNMENTS provider offered a new booking")
	}
	apply("providers", providerID, "LIFT_PROVIDER_RESTRICTION", 1, nil)
	if !offered() {
		t.Fatal("lift did not restore matching eligibility")
	}
	apply("vehicles", vehicleID, "RESTRICT_VEHICLE", 0, map[string]string{"restriction": "OUT_OF_SERVICE"})
	if offered() {
		t.Fatal("provider offered a booking on an out-of-service vehicle")
	}
	// The provider was online on that vehicle, so the executor also took them
	// out of availability durably.
	var state string
	if err := db.QueryRow(ctx, `SELECT state FROM provider_operational_state WHERE provider_id=$1`, providerID).Scan(&state); err != nil || state != "OFFLINE" {
		t.Fatal("provider left AVAILABLE on an out-of-service vehicle", state, err)
	}
	apply("vehicles", vehicleID, "LIFT_VEHICLE_RESTRICTION", 1, nil)
	// The driver goes online again (state machine back to AVAILABLE).
	if _, err := db.Exec(ctx, `UPDATE provider_operational_state SET state='AVAILABLE',version=version+1 WHERE provider_id=$1`, providerID); err != nil {
		t.Fatal(err)
	}
	if !offered() {
		t.Fatal("vehicle lift did not restore matching eligibility")
	}
	// SUSPEND_ONLINE: the executor forces the idle provider OFFLINE, which the
	// matcher's durable operational-state check also excludes.
	apply("providers", providerID, "RESTRICT_PROVIDER", 2, map[string]string{"restriction": "SUSPEND_ONLINE"})
	if offered() {
		t.Fatal("suspended provider offered a booking")
	}
}
