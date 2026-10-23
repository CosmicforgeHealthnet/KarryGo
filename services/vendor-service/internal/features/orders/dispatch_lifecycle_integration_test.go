package orders

import (
	"context"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"testing"
)

// Follow the canonical Vendor integration convention: use an existing test
// plan fixture, but create a separate superseded reference and remove only our
// reference/events. Never modify the fixture's active mission or projection.
func TestDispatchLifecycleDuplicateStaleAndForeignParcelPostgres(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL unavailable")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	id := uuid.NewString()
	ref, parcel := seedAdministrativeDispatchReference(t, pool)
	if _, err = pool.Exec(ctx, `INSERT INTO commerce_platform_mission_refs(id,group_id,plan_id,plan_mission_id,authority,mission_ref,link_state) SELECT $1,group_id,plan_id,plan_mission_id,'DISPATCH',$1,'SUPERSEDED' FROM commerce_platform_mission_refs WHERE id=$2`, id, ref); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM commerce_platform_operational_events WHERE operation_ref=$1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM commerce_platform_mission_refs WHERE id=$1`, id)
	}()
	svc := &Service{store: NewStore(pool), enabled: true}
	e := lifecycleEvent()
	p, _ := validateDispatchLifecycle(e)
	p.SourceRef = id
	p.Missions[0].ParcelRefs = []string{parcel}
	e.EventID = id + ":assigned"
	e.Payload, _ = json.Marshal(p)
	e.PayloadHash = hashDispatchPayload(e.Payload)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.ApplyDispatchCommerceEvent(ctx, e) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	e.EventID = id + ":stale"
	p.Missions[0].Version = 1
	p.Missions[0].State = "PLANNED"
	e.Payload, _ = json.Marshal(p)
	e.PayloadHash = hashDispatchPayload(e.Payload)
	if err = svc.ApplyDispatchCommerceEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = pool.QueryRow(ctx, `SELECT dispatch_domain_projection FROM commerce_platform_mission_refs WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored dispatchcommerce.DeliveryLifecycleProjection
	if json.Unmarshal(raw, &stored) != nil || len(stored.Missions) != 1 || stored.Missions[0].State != "ASSIGNED" {
		t.Fatal("stale projection regressed", string(raw))
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM commerce_platform_operational_events WHERE event_id=$1`, id+":assigned").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	e.EventID = id + ":foreign"
	p.Missions[0].ParcelRefs = []string{"unrelated-parcel"}
	e.Payload, _ = json.Marshal(p)
	e.PayloadHash = hashDispatchPayload(e.Payload)
	if err = svc.ApplyDispatchCommerceEvent(ctx, e); err == nil {
		t.Fatal("foreign parcel accepted")
	}
}
