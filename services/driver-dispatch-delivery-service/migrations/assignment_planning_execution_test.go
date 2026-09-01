package migrations_test

import (
	"context"
	"sync"
	"testing"
	"time"

	planning "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/missionplanning"
	"github.com/google/uuid"
)

type routeAuthorityStub struct{}

func (routeAuthorityStub) Evaluate(context.Context, planning.PlanRequest) (planning.RouteEvidence, error) {
	return planning.RouteEvidence{Reference: "route:test:v1", Serviceable: true, DistanceKM: 12, DurationSeconds: 1200}, nil
}

func executionPlanRequest() planning.PlanRequest {
	now := time.Now().UTC().Add(-time.Minute)
	return planning.PlanRequest{SourceRef: "commerce-" + uuid.NewString(), SourceVersion: 1, CustomerID: uuid.NewString(), Receiver: planning.Point{Address: "Receiver", Latitude: 6.5, Longitude: 3.4}, ReceiverName: "Receiver", ReceiverPhone: "+2348012345678", FareAmount: 5000, Currency: "NGN", PaymentRef: "wallet-hold", ServiceClass: "STANDARD", VehicleClass: "MOTORCYCLE", MaxWeightGrams: 10000, MaxVolumeML: 30000, OperationRef: "plan-" + uuid.NewString(), Units: []planning.Unit{{UnitRef: "unit-a", VendorRef: "vendor-a", BranchRef: "branch-a", OrderRef: "order-a", Pickup: planning.Point{Address: "Pickup", Latitude: 6.4, Longitude: 3.3}, Ready: planning.ReadyWindow{From: now, To: now.Add(time.Hour)}, ParcelRefs: []string{"parcel-a"}}}, Parcels: []planning.Parcel{{ParcelRef: "parcel-a", UnitRef: "unit-a", WeightGrams: 1000, VolumeML: 1000, PackagePolicyVersion: "package-v1"}}}
}

func TestPlannedMissionCanonicalExecutionManifestVersionAndCustody(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	repo := planning.NewRepository(pool)
	service := planning.New(repo, planning.NewPlanner(), true).WithRouteAuthority(routeAuthorityStub{})
	in := executionPlanRequest()
	first, err := service.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Missions) != 1 {
		t.Fatalf("missions=%d", len(first.Missions))
	}
	bySource, err := service.ReadBySource(ctx, in.SourceRef, in.SourceVersion)
	if err != nil {
		t.Fatalf("read by source: %v", err)
	}
	if bySource.GroupRef != first.GroupRef {
		t.Fatalf("source read-back escaped scope: %+v", bySource)
	}
	if _, err := service.ReadBySource(ctx, in.SourceRef, in.SourceVersion+1); err == nil {
		t.Fatal("expected wrong source version to fail closed")
	}
	var bookings, requests, links int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM dispatch_delivery_missions WHERE delivery_group_id=$1 AND booking_id IS NOT NULL),(SELECT count(*) FROM request_broadcasts b JOIN dispatch_delivery_missions m ON m.booking_id=b.booking_id WHERE m.delivery_group_id=$1),(SELECT count(*) FROM dispatch_mission_execution_links l JOIN dispatch_delivery_missions m ON m.id=l.mission_id WHERE m.delivery_group_id=$1)`, first.GroupRef).Scan(&bookings, &requests, &links); err != nil {
		t.Fatal(err)
	}
	if bookings != 1 || requests != 1 || links != 1 {
		t.Fatalf("booking=%d request=%d links=%d", bookings, requests, links)
	}
	replayed, err := service.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.GroupRef != first.GroupRef {
		t.Fatalf("replay=%+v", replayed)
	}
	replan := in
	replan.OperationRef = "replan-" + uuid.NewString()
	replan.ExpectedPlanVersion = 1
	second, err := service.Replan(ctx, first.GroupRef, replan)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || second.Missions[0].ManifestRef != first.Missions[0].ManifestRef || second.Missions[0].ManifestVersion != 2 {
		t.Fatalf("replan=%+v", second)
	}
	command := planning.CustodyCommand{MissionRef: second.Missions[0].MissionRef, ManifestVersion: second.Missions[0].ManifestVersion, SourceRef: in.SourceRef, SourceVersion: in.SourceVersion}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := command
			c.OperationRef = "custody-" + uuid.NewString()
			errs <- service.ActivateCustody(ctx, second.Missions[0].ManifestRef, c)
		}()
	}
	wg.Wait()
	close(errs)
	success, failed := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			failed++
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("custody success=%d failed=%d", success, failed)
	}
}
