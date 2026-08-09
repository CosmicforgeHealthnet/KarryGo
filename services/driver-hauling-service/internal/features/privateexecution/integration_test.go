package privateexecution

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptedVerifier struct{ acceptance Acceptance }

func (v acceptedVerifier) Verify(context.Context, string) (Acceptance, error) {
	return v.acceptance, nil
}

func haulingPrivateDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func validHaulingPrivateInput() CreateRequest {
	return CreateRequest{
		OrganizationID: uuid.NewString(), SettlementOrganizationID: uuid.NewString(),
		ExecutingProviderID: uuid.NewString(), OrganizationMembershipID: uuid.NewString(),
		PrivateOrderID: uuid.NewString(), ShipmentID: uuid.NewString(),
		PrivateAssignmentID: uuid.NewString(), ProviderResponseID: uuid.NewString(),
		FleetOperationID: uuid.NewString(), ServiceCategory: "HAULING",
		TruckID: uuid.NewString(), IdempotencyKey: uuid.NewString(), CorrelationID: uuid.NewString(),
		Route: RouteSnapshot{PickupAddress: "private pickup", DropoffAddress: "private dropoff"},
		Cargo: CargoSnapshot{CargoType: "general", WeightKG: 50, Description: "minimized fixture"},
	}
}

func seedHaulingAuthority(t *testing.T, pool *pgxpool.Pool, in CreateRequest) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO truck_providers(id,phone,email,status,onboarding_status,service_type)
		VALUES($1,$2,$3,'active','complete','hauling')`, in.ExecutingProviderID,
		"+234"+in.ExecutingProviderID[:10], in.ExecutingProviderID+"@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status)
		VALUES($1,$2,'flatbed',5000,$3,'active')`, in.TruckID, in.ExecutingProviderID, "PX-"+in.TruckID[:8])
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO hauling_organization_provider_links
		(organization_id,provider_id,membership_id,state) VALUES($1,$2,$3,'ACTIVE')`,
		in.OrganizationID, in.ExecutingProviderID, in.OrganizationMembershipID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO hauling_organization_truck_links
		(organization_id,truck_id,state) VALUES($1,$2,'ACTIVE')`, in.OrganizationID, in.TruckID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO hauling_provider_availability
		(provider_id,state,shift_open,changed_by) VALUES($1,'AVAILABLE',true,'test')`, in.ExecutingProviderID)
	if err != nil {
		t.Fatal(err)
	}
}

func verifierFor(in CreateRequest) Acceptance {
	return Acceptance{AssignmentID: in.PrivateAssignmentID, OrganizationID: in.OrganizationID,
		ProviderID: in.ExecutingProviderID, ProviderService: "HAULING",
		OrganizationMembershipID: in.OrganizationMembershipID,
		ProviderResponseID:       in.ProviderResponseID, ResponseStatus: "ACCEPTED"}
}

func TestPrivateExecutionPostgres_IdempotencyReadbackMetadataAndOutbox(t *testing.T) {
	pool := haulingPrivateDB(t)
	in := validHaulingPrivateInput()
	seedHaulingAuthority(t, pool, in)
	svc := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false)
	ctx := context.Background()

	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "CONFIRMED" || created.CanonicalBookingID == "" || created.SourceVersion != 1 {
		t.Fatalf("unexpected creation: %#v", created)
	}
	replayed, err := svc.Create(ctx, in)
	if err != nil || replayed.CanonicalBookingID != created.CanonicalBookingID {
		t.Fatalf("idempotent replay: %#v %v", replayed, err)
	}
	read, err := svc.Read(ctx, in.FleetOperationID)
	if err != nil || read.CanonicalBookingID != created.CanonicalBookingID {
		t.Fatalf("readback: %#v %v", read, err)
	}
	byTrip, err := svc.ReadByTrip(ctx, created.CanonicalBookingID)
	if err != nil || byTrip.FleetOperationID != in.FleetOperationID {
		t.Fatalf("trip readback: %#v %v", byTrip, err)
	}

	var bookings, mappings, events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM haulage_bookings WHERE id=$1`, created.CanonicalBookingID).Scan(&bookings); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM hauling_private_execution_mappings WHERE fleet_operation_id=$1
		AND organization_id=$2 AND settlement_organization_id=$3 AND executing_provider_id=$4
		AND fleet_private_order_id=$5 AND fleet_shipment_id=$6 AND fleet_assignment_id=$7`,
		in.FleetOperationID, in.OrganizationID, in.SettlementOrganizationID, in.ExecutingProviderID,
		in.PrivateOrderID, in.ShipmentID, in.PrivateAssignmentID).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM hauling_domain_outbox WHERE aggregate_id=$1
		AND event_type='trip.created.v1' AND event_version=1
		AND NOT (payload ? 'customerName') AND NOT (payload ? 'fare')`, created.CanonicalBookingID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if bookings != 1 || mappings != 1 || events != 1 {
		t.Fatalf("bookings=%d mappings=%d events=%d", bookings, mappings, events)
	}

	changed := in
	changed.Cargo.WeightKG++
	if _, err = svc.Create(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed fingerprint err=%v", err)
	}
}

func TestPrivateExecutionPostgres_ConcurrentDuplicateCreatesOneExecution(t *testing.T) {
	pool := haulingPrivateDB(t)
	in := validHaulingPrivateInput()
	seedHaulingAuthority(t, pool, in)
	svc := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false)
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
	canonical := ""
	for v := range results {
		if canonical == "" {
			canonical = v.CanonicalBookingID
		}
		if v.CanonicalBookingID != canonical {
			t.Fatalf("different canonical execution: %q != %q", v.CanonicalBookingID, canonical)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hauling_private_execution_mappings WHERE fleet_operation_id=$1`, in.FleetOperationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("mapping count=%d", count)
	}
}

func TestPrivateExecutionPostgres_AuthorityAcceptanceAndAvailabilityFailClosed(t *testing.T) {
	pool := haulingPrivateDB(t)
	t.Run("acceptance required", func(t *testing.T) {
		in := validHaulingPrivateInput()
		seedHaulingAuthority(t, pool, in)
		bad := verifierFor(in)
		bad.ResponseStatus = "DECLINED"
		_, err := NewService(NewRepository(pool), acceptedVerifier{bad}, true, false).Create(context.Background(), in)
		if err == nil {
			t.Fatal("declined assignment created execution")
		}
	})
	t.Run("provider link membership authority", func(t *testing.T) {
		in := validHaulingPrivateInput()
		seedHaulingAuthority(t, pool, in)
		if _, err := pool.Exec(context.Background(), `UPDATE hauling_organization_provider_links SET state='REVOKED' WHERE organization_id=$1 AND provider_id=$2`, in.OrganizationID, in.ExecutingProviderID); err != nil {
			t.Fatal(err)
		}
		_, err := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false).Create(context.Background(), in)
		if !errors.Is(err, ErrIneligible) {
			t.Fatalf("provider authority err=%v", err)
		}
	})
	t.Run("truck link authority", func(t *testing.T) {
		in := validHaulingPrivateInput()
		seedHaulingAuthority(t, pool, in)
		if _, err := pool.Exec(context.Background(), `UPDATE hauling_organization_truck_links SET state='REVOKED' WHERE organization_id=$1 AND truck_id=$2`, in.OrganizationID, in.TruckID); err != nil {
			t.Fatal(err)
		}
		_, err := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false).Create(context.Background(), in)
		if !errors.Is(err, ErrIneligible) {
			t.Fatalf("truck authority err=%v", err)
		}
	})
	t.Run("availability", func(t *testing.T) {
		in := validHaulingPrivateInput()
		seedHaulingAuthority(t, pool, in)
		if _, err := pool.Exec(context.Background(), `UPDATE hauling_provider_availability SET state='UNAVAILABLE',shift_open=false WHERE provider_id=$1`, in.ExecutingProviderID); err != nil {
			t.Fatal(err)
		}
		_, err := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false).Create(context.Background(), in)
		if !errors.Is(err, ErrIneligible) {
			t.Fatalf("availability err=%v", err)
		}
	})
}

func TestPrivateExecutionPostgres_CancellationIdempotencyConcurrencyAndTerminalGuard(t *testing.T) {
	pool := haulingPrivateDB(t)
	in := validHaulingPrivateInput()
	seedHaulingAuthority(t, pool, in)
	svc := NewService(NewRepository(pool), acceptedVerifier{verifierFor(in)}, true, false)
	ctx := context.Background()
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	cancel := CancelRequest{Reason: "company requested", IdempotencyKey: uuid.NewString()}
	const workers = 6
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := svc.Cancel(ctx, in.FleetOperationID, cancel); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent cancel: %v", err)
		}
	}
	read, err := svc.ReadCancellation(ctx, in.FleetOperationID)
	if err != nil || read.Status != "CANCELLED" || read.SourceVersion != 2 {
		t.Fatalf("cancellation readback: %#v %v", read, err)
	}
	if _, err = svc.Cancel(ctx, in.FleetOperationID, CancelRequest{Reason: "different", IdempotencyKey: cancel.IdempotencyKey}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancel fingerprint err=%v", err)
	}
	var events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM hauling_domain_outbox WHERE aggregate_id=$1 AND event_type='trip.cancelled.v1' AND event_version=2`, created.CanonicalBookingID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("cancel events=%d", events)
	}

	terminal := validHaulingPrivateInput()
	seedHaulingAuthority(t, pool, terminal)
	terminalSvc := NewService(NewRepository(pool), acceptedVerifier{verifierFor(terminal)}, true, false)
	v, err := terminalSvc.Create(ctx, terminal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE haulage_bookings SET status='completed' WHERE id=$1`, v.CanonicalBookingID); err != nil {
		t.Fatal(err)
	}
	if _, err = terminalSvc.Cancel(ctx, terminal.FleetOperationID, CancelRequest{Reason: "too late", IdempotencyKey: uuid.NewString()}); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("completed cancellation err=%v", err)
	}
}
