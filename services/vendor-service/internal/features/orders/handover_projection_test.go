package orders

import (
	"context"
	"sync"
	"testing"

	handover "cosmicforge/logistics/services/vendor-service/internal/integrations/handover"
	"github.com/google/uuid"
)

func TestSection32ProjectionConcurrentDuplicateStaleAndForeignMission(t *testing.T) {
	db := confidenceETATestPool(t)
	parent := seedETAParent(t, db)
	ctx := context.Background()
	id := "s32-" + uuid.NewString()
	_, err := db.Exec(ctx, `INSERT INTO commerce_order_missions(id,mission_ref,parent_order_id,customer_id,plan_mission_ref,service,vehicle_class,agreed_fee_minor,currency,status,external_ref)
	 SELECT $1,$1,id,customer_id,$1,'TAXI','CAR',0,'NGN','ACCEPTED',$1 FROM commerce_parent_orders WHERE id=$2`, id, parent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM commerce_handover_observations WHERE mission_id=$1`, id)
		_, _ = db.Exec(ctx, `DELETE FROM commerce_order_missions WHERE id=$1`, id)
	})
	s := &Service{store: NewStore(db)}
	o := handover.Observation{Binding: handover.Binding{Authority: "TAXI", MissionRef: id, FulfilmentRef: id}, HandoverRef: "handover-" + id, Stage: "PICKUP", Status: "CONSUMED", SourceVersion: 2}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- s.observeHandover(ctx, o) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	o.Status = "ACTIVE"
	o.SourceVersion = 1
	if err = s.observeHandover(ctx, o); err != nil {
		t.Fatal(err)
	}
	var count int
	var status string
	if err = db.QueryRow(ctx, `SELECT count(*),max(status) FROM commerce_handover_observations WHERE mission_id=$1`, id).Scan(&count, &status); err != nil || count != 1 || status != "CONSUMED" {
		t.Fatalf("count=%d status=%s err=%v", count, status, err)
	}
	o.MissionRef = "another-mission"
	if err = s.observeHandover(ctx, o); err == nil {
		t.Fatal("cross-mission observation accepted")
	}
}
