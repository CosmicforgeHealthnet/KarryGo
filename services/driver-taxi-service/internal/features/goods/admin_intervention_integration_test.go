package goods

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/features/admininterventions"
	"cosmicforge/logistics/shared/go/admincommand"
	ownerfixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
)

// TestGoodsOfferRefusesAdministrativelyRestrictedProvider proves the goods
// offer is atomically gated by the Taxi-owned intervention overlay: the
// guarded AVAILABLE->OFFERED write refuses a NO_NEW_ASSIGNMENTS provider and
// an OUT_OF_SERVICE vehicle, and succeeds once lifted.
func TestGoodsOfferRefusesAdministrativelyRestrictedProvider(t *testing.T) {
	db := ownerfixture.Database(t, "../../../migrations")
	ctx := context.Background()
	f := seedGoodsRaceFixture(t, db)
	vehicle := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,seats,status) VALUES($1,$2,'economy',$3,4,'active')`, vehicle, f.provider, "PL-"+vehicle[:8]); err != nil {
		t.Fatal(err)
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
	repo := NewRepository(db)
	apply("providers", f.provider, "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "NO_NEW_ASSIGNMENTS"})
	if err := repo.Offer(ctx, f.mission, f.provider, vehicle); !errors.Is(err, ErrConflict) {
		t.Fatal("restricted provider offered goods work", err)
	}
	apply("providers", f.provider, "LIFT_PROVIDER_RESTRICTION", 1, nil)
	apply("vehicles", vehicle, "RESTRICT_VEHICLE", 0, map[string]string{"restriction": "OUT_OF_SERVICE"})
	// The idle provider was AVAILABLE and may have been on this vehicle, so
	// the executor took them offline; put them back to isolate the vehicle gate.
	if _, err := db.Exec(ctx, `UPDATE provider_operational_state SET state='AVAILABLE',version=version+1 WHERE provider_id=$1`, f.provider); err != nil {
		t.Fatal(err)
	}
	if err := repo.Offer(ctx, f.mission, f.provider, vehicle); !errors.Is(err, ErrConflict) {
		t.Fatal("out-of-service vehicle offered goods work", err)
	}
	apply("vehicles", vehicle, "LIFT_VEHICLE_RESTRICTION", 1, nil)
	if err := repo.Offer(ctx, f.mission, f.provider, vehicle); err != nil {
		t.Fatal("lifted provider/vehicle not offerable", err)
	}
}
