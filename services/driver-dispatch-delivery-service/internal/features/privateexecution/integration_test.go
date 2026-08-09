package privateexecution

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

type fakeDispatchAuthority struct {
	mu                                sync.Mutex
	pool                              *pgxpool.Pool
	providerEligible, vehicleEligible bool
	createCalls                       int
	createErr                         error
	result                            *marketplace.ExecutionResult
	tripStatus                        string
}

func newFakeDispatchAuthority(pool *pgxpool.Pool) *fakeDispatchAuthority {
	return &fakeDispatchAuthority{pool: pool, providerEligible: true, vehicleEligible: true, tripStatus: "active"}
}

func (f *fakeDispatchAuthority) Reserve(ctx context.Context, in marketplace.ReserveInput) (*marketplace.ReservationResult, error) {
	id := uuid.NewString()
	_, err := f.pool.Exec(ctx, `INSERT INTO dispatch_organization_reservations
		(id,request_id,organization_id,status,acceptance_source,accepting_provider_id,
		organization_membership_id,source_request_version,idempotency_key,request_fingerprint,reserved_at)
		VALUES($1,$2,$3,'RESERVED','PROVIDER_DIRECT',$4,$5,1,$6,$6,now())`,
		id, in.RequestID, in.OrganizationID, in.AcceptingProviderID, in.OrganizationMembershipID, in.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	return &marketplace.ReservationResult{ReservationID: id, RequestID: in.RequestID, RequestVersion: 2, Status: "RESERVED"}, nil
}
func (f *fakeDispatchAuthority) ValidateProvider(context.Context, marketplace.ProviderEligibilityInput) (*marketplace.EligibilityResult, error) {
	return &marketplace.EligibilityResult{Eligible: f.providerEligible, RequestVersion: 2}, nil
}
func (f *fakeDispatchAuthority) ValidateVehicle(context.Context, marketplace.VehicleEligibilityInput) (*marketplace.EligibilityResult, error) {
	return &marketplace.EligibilityResult{Eligible: f.vehicleEligible, RequestVersion: 2}, nil
}
func (f *fakeDispatchAuthority) CreateExecution(_ context.Context, in marketplace.ExecutionInput) (*marketplace.ExecutionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.result == nil {
		tripID, bookingID := uuid.NewString(), uuid.NewString()
		_, err := f.pool.Exec(context.Background(), `INSERT INTO trips
			(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,
			organization_id,settlement_organization_id,organization_membership_id,
			fleet_assignment_id,provider_response_id,acceptance_source,execution_source,
			execution_mode,fleet_correlation_id,source_version)
			VALUES($1,$2,$3,$4,'assigned','private pickup',0,0,'private dropoff',0,0,0,'','',
			$5,$6,$7,$8,$9,'PROVIDER_DIRECT','PRIVATE_ORDER','ORGANIZATION_MANAGED',$10,1)`,
			tripID, bookingID, in.ExecutingProviderID, uuid.NewString(), in.OrganizationID,
			in.SettlementOrganizationID, in.OrganizationMembershipID, in.FleetAssignmentID,
			in.ProviderResponseID, in.CorrelationID)
		if err != nil {
			return nil, err
		}
		f.result = &marketplace.ExecutionResult{ExecutionID: uuid.NewString(), TripID: tripID, RequestID: in.RequestID, BookingID: bookingID, Status: "CREATED", RequestVersion: 3, TripVersion: 1, CreatedAt: time.Now()}
	}
	return f.result, f.createErr
}
func (f *fakeDispatchAuthority) ReadExecutionByIdempotencyKey(context.Context, string) (*marketplace.ExecutionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.result == nil {
		return nil, errors.New("not found")
	}
	return f.result, nil
}
func (f *fakeDispatchAuthority) ReadTrip(_ context.Context, tripID string) (*marketplace.TripView, error) {
	return &marketplace.TripView{TripID: tripID, Status: f.tripStatus, SourceVersion: 2}, nil
}

type fakeCanceller struct{ authority *fakeDispatchAuthority }

func (c fakeCanceller) CancelOrganizationTrip(context.Context, string, string, string) (*tripfeature.Trip, error) {
	c.authority.tripStatus = "cancelled"
	return nil, nil
}

func dispatchPrivateDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func validDispatchPrivateInput() CreateRequest {
	return CreateRequest{
		OrganizationID: uuid.NewString(), SettlementOrganizationID: uuid.NewString(),
		ExecutingProviderID: uuid.NewString(), OrganizationMembershipID: uuid.NewString(),
		PrivateOrderID: uuid.NewString(), ShipmentID: uuid.NewString(),
		PrivateAssignmentID: uuid.NewString(), ProviderResponseID: uuid.NewString(),
		FleetOperationID: uuid.NewString(), ServiceCategory: "DISPATCH",
		VehicleID: uuid.NewString(), IdempotencyKey: uuid.NewString(), CorrelationID: uuid.NewString(),
		Route: RouteSnapshot{PickupAddress: "private pickup", DropoffAddress: "private dropoff"},
	}
}

func seedDispatchProviderVehicle(t *testing.T, pool *pgxpool.Pool, in CreateRequest) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO providers
		(id,phone,full_name,verification_status,is_active,onboarding_complete)
		VALUES($1,$2,'private test provider','verified',true,true)`,
		in.ExecutingProviderID, "+234"+in.ExecutingProviderID[:10])
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO bikes
		(id,provider_id,bike_type,brand,model,year,color,plate_number,verification_status,is_active)
		VALUES($1,$2,'dispatch_bike','test','test',2024,'black',$3,'verified',true)`,
		in.VehicleID, in.ExecutingProviderID, "PX-"+in.VehicleID[:8])
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivateExecutionPostgres_CreateReplayMetadataAndCanonicalReadback(t *testing.T) {
	pool := dispatchPrivateDB(t)
	authority := newFakeDispatchAuthority(pool)
	svc := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, false)
	in := validDispatchPrivateInput()
	seedDispatchProviderVehicle(t, pool, in)
	ctx := context.Background()
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusConfirmed || created.CanonicalRequestID == "" || created.CanonicalReservationID == "" || created.CanonicalTripID == "" || created.SourceVersion != 1 {
		t.Fatalf("unexpected create: %#v", created)
	}
	replay, err := svc.Create(ctx, in)
	if err != nil || replay.CanonicalTripID != created.CanonicalTripID {
		t.Fatalf("replay: %#v %v", replay, err)
	}
	read, err := svc.Read(ctx, in.FleetOperationID)
	if err != nil || read.CanonicalTripID != created.CanonicalTripID {
		t.Fatalf("readback: %#v %v", read, err)
	}
	if authority.createCalls != 1 {
		t.Fatalf("canonical create calls=%d", authority.createCalls)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_private_execution_mappings WHERE fleet_operation_id=$1
		AND organization_id=$2 AND settlement_organization_id=$3 AND executing_provider_id=$4
		AND fleet_private_order_id=$5 AND fleet_shipment_id=$6 AND fleet_assignment_id=$7`,
		in.FleetOperationID, in.OrganizationID, in.SettlementOrganizationID, in.ExecutingProviderID,
		in.PrivateOrderID, in.ShipmentID, in.PrivateAssignmentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("mapping count=%d", count)
	}
	changed := in
	changed.Route.DropoffAddress = "changed"
	if _, err = svc.Create(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed fingerprint err=%v", err)
	}
}

func TestPrivateExecutionPostgres_ConcurrentDuplicateCreatesOneCanonicalMapping(t *testing.T) {
	pool := dispatchPrivateDB(t)
	authority := newFakeDispatchAuthority(pool)
	svc := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, false)
	in := validDispatchPrivateInput()
	seedDispatchProviderVehicle(t, pool, in)
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	results := make(chan View, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, err := svc.Create(ctx, in); results <- v; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	trip := ""
	for v := range results {
		if trip == "" {
			trip = v.CanonicalTripID
		}
		if v.CanonicalTripID != trip {
			t.Fatalf("different trips %s %s", trip, v.CanonicalTripID)
		}
	}
	if authority.createCalls != 1 {
		t.Fatalf("canonical create calls=%d", authority.createCalls)
	}
	var mappings int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_private_execution_mappings WHERE fleet_operation_id=$1`, in.FleetOperationID).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if mappings != 1 {
		t.Fatalf("mapping count=%d", mappings)
	}
}

func TestPrivateExecutionPostgres_TimeoutAfterCommitUsesReadbackWithoutDuplicate(t *testing.T) {
	pool := dispatchPrivateDB(t)
	authority := newFakeDispatchAuthority(pool)
	authority.createErr = context.DeadlineExceeded
	svc := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, false)
	in := validDispatchPrivateInput()
	seedDispatchProviderVehicle(t, pool, in)
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first error=%v", err)
	}
	authority.createErr = nil
	read, err := svc.Create(context.Background(), in)
	if err != nil || read.Status != StatusConfirmed || read.CanonicalTripID == "" {
		t.Fatalf("readback: %#v %v", read, err)
	}
	if authority.createCalls != 1 {
		t.Fatalf("blindly resubmitted %d times", authority.createCalls)
	}
}

func TestPrivateExecutionPostgres_ProviderVehicleAndFeatureControlsFailClosed(t *testing.T) {
	pool := dispatchPrivateDB(t)
	for _, tc := range []struct {
		name              string
		provider, vehicle bool
		want              error
	}{
		{"provider", false, true, ErrProviderIneligible}, {"vehicle", true, false, ErrVehicleIneligible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authority := newFakeDispatchAuthority(pool)
			authority.providerEligible = tc.provider
			authority.vehicleEligible = tc.vehicle
			in := validDispatchPrivateInput()
			seedDispatchProviderVehicle(t, pool, in)
			_, err := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, false).Create(context.Background(), in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
	authority := newFakeDispatchAuthority(pool)
	if _, err := NewService(NewRepository(pool), authority, fakeCanceller{authority}, false, false).Create(context.Background(), validDispatchPrivateInput()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled err=%v", err)
	}
	if _, err := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, true).Create(context.Background(), validDispatchPrivateInput()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("kill switch err=%v", err)
	}
}

func TestPrivateExecutionPostgres_AuthoritativeAvailabilityFailsClosed(t *testing.T) {
	pool := dispatchPrivateDB(t)
	in := validDispatchPrivateInput()
	seedDispatchProviderVehicle(t, pool, in)
	repo := marketplace.NewRepository(pool)
	available, err := repo.ProviderIsAvailable(context.Background(), nil, in.ExecutingProviderID)
	if err != nil || available {
		t.Fatalf("missing availability available=%v err=%v", available, err)
	}
	if _, err = pool.Exec(context.Background(), `INSERT INTO provider_availability
		(provider_id,status,verified_to_go_online,session_start) VALUES($1,'online',true,now())`, in.ExecutingProviderID); err != nil {
		t.Fatal(err)
	}
	available, err = repo.ProviderIsAvailable(context.Background(), nil, in.ExecutingProviderID)
	if err != nil || !available {
		t.Fatalf("online availability=%v err=%v", available, err)
	}
	if _, err = pool.Exec(context.Background(), `UPDATE provider_availability SET status='offline' WHERE provider_id=$1`, in.ExecutingProviderID); err != nil {
		t.Fatal(err)
	}
	available, err = repo.ProviderIsAvailable(context.Background(), nil, in.ExecutingProviderID)
	if err != nil || available {
		t.Fatalf("offline availability=%v err=%v", available, err)
	}
}

func TestPrivateExecutionPostgres_CancellationReplayConflictAndReadback(t *testing.T) {
	pool := dispatchPrivateDB(t)
	authority := newFakeDispatchAuthority(pool)
	svc := NewService(NewRepository(pool), authority, fakeCanceller{authority}, true, false)
	in := validDispatchPrivateInput()
	seedDispatchProviderVehicle(t, pool, in)
	created, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	cancel := CancelRequest{Reason: "company requested", IdempotencyKey: uuid.NewString()}
	first, err := svc.Cancel(context.Background(), in.FleetOperationID, cancel)
	if err != nil || first.Status != "CANCELLED" || first.SourceVersion != 2 {
		t.Fatalf("cancel: %#v %v", first, err)
	}
	replayed, err := svc.Cancel(context.Background(), in.FleetOperationID, cancel)
	if err != nil || replayed.Status != "CANCELLED" {
		t.Fatalf("cancel replay: %#v %v", replayed, err)
	}
	read, err := svc.ReadCancellation(context.Background(), in.FleetOperationID)
	if err != nil || read.Status != "CANCELLED" || read.SourceVersion != 2 {
		t.Fatalf("cancel readback: %#v %v", read, err)
	}
	if _, err = svc.Cancel(context.Background(), in.FleetOperationID, CancelRequest{Reason: "changed", IdempotencyKey: cancel.IdempotencyKey}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancel fingerprint err=%v", err)
	}
	var status string
	if err = pool.QueryRow(context.Background(), `SELECT status FROM dispatch_private_execution_mappings WHERE canonical_trip_id=$1`, created.CanonicalTripID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusCancelled {
		t.Fatalf("mapping status=%s", status)
	}
}
