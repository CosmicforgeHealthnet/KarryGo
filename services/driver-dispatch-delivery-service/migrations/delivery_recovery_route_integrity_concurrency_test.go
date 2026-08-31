package migrations_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/routeintegrity"
)

type recoveryFixture struct{ provider, customer, booking, trip string }

func TestDeliveryRecoveryActionRaceAndAppendOnlyContact(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	f := recoveryFixture{provider: uuid.NewString(), customer: uuid.NewString(), booking: uuid.NewString(), trip: uuid.NewString()}
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete) VALUES($1,$2,'verified',true,true)`, f.provider, "+23480"+f.provider[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,'accepted','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, f.booking, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,$3,$4,'in_progress','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, f.trip, f.booking, f.provider, f.customer); err != nil {
		t.Fatal(err)
	}
	service := deliveryrecovery.New(deliveryrecovery.NewRepository(pool), deliveryrecovery.Policy{Enabled: true, ReturnEnabled: true, GracePeriod: time.Millisecond})
	attempt, err := service.StartAttempt(ctx, deliveryrecovery.StartAttemptCommand{TripID: f.trip, ProviderID: f.provider, IdempotencyKey: "attempt-start"})
	if err != nil {
		t.Fatal(err)
	}
	contact, err := service.RecordContact(ctx, deliveryrecovery.ContactCommand{AttemptID: attempt.ID, ProviderID: f.provider, Method: "PHONE", Outcome: "UNREACHABLE", IdempotencyKey: "contact-1"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	attempt, err = service.FailAttempt(ctx, deliveryrecovery.FailAttemptCommand{AttemptID: attempt.ID, ProviderID: f.provider, ReasonCode: "RECEIVER_NO_ANSWER", IdempotencyKey: "attempt-fail"})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(time.Hour)
	commands := []deliveryrecovery.SelectRecoveryCommand{{BookingID: f.booking, CustomerID: f.customer, Action: deliveryrecovery.RetrySameDay, ScheduledFor: &when, ExpectedTripVersion: 1, IdempotencyKey: "recovery-a"}, {BookingID: f.booking, CustomerID: f.customer, Action: deliveryrecovery.Reschedule, ScheduledFor: &when, ExpectedTripVersion: 1, IdempotencyKey: "recovery-b"}}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, command := range commands {
		wg.Add(1)
		go func(command deliveryrecovery.SelectRecoveryCommand) {
			defer wg.Done()
			_, e := service.SelectRecovery(ctx, command)
			errCh <- e
		}(command)
	}
	wg.Wait()
	close(errCh)
	success := 0
	for e := range errCh {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("recovery race successes=%d want 1", success)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_delivery_contact_attempts SET outcome='DECLINED' WHERE id=$1::uuid`, contact.ID); err == nil {
		t.Fatal("append-only contact update succeeded")
	}
}

func TestRouteVersionAndOfflineCommandConcurrency(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	f := recoveryFixture{provider: uuid.NewString(), customer: uuid.NewString(), booking: uuid.NewString(), trip: uuid.NewString()}
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete) VALUES($1,$2,'verified',true,true)`, f.provider, "+23481"+f.provider[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,'accepted','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, f.booking, f.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,$3,$4,'in_progress','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, f.trip, f.booking, f.provider, f.customer); err != nil {
		t.Fatal(err)
	}
	repo := routeintegrity.NewRepository(pool)
	trip, err := repo.TripContext(ctx, f.trip, f.provider)
	if err != nil {
		t.Fatal(err)
	}
	plan := routeintegrity.RoutePlan{Reference: "route-v", Provider: "route-authority", EvidenceVersion: "e1", DistanceMeters: 1000, DurationSeconds: 300, OrderedPoints: []routeintegrity.Point{trip.Pickup, trip.Dropoff}}
	policy := routeintegrity.Policy{Enabled: true, CorridorMeters: 250, WrongWayToleranceDegrees: 110, StationaryThreshold: 10 * time.Minute, StaleThreshold: 2 * time.Minute, HighRiskCorridorMeters: 120, HighRiskStaleThreshold: time.Minute}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := repo.CreateRoute(ctx, trip, plan, policy, "REROUTE", time.Now().UTC())
			errCh <- e
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Fatal(e)
		}
	}
	var versions, current int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='CURRENT') FROM dispatch_expected_routes WHERE trip_id=$1::uuid`, f.trip).Scan(&versions, &current); err != nil {
		t.Fatal(err)
	}
	if versions != 2 || current != 1 {
		t.Fatalf("versions=%d current=%d", versions, current)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_expected_routes SET distance_meters=distance_meters+1 WHERE trip_id=$1::uuid AND version=1`, f.trip); err == nil {
		t.Fatal("immutable route evidence update succeeded")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM dispatch_expected_routes WHERE trip_id=$1::uuid AND version=1`, f.trip); err == nil {
		t.Fatal("immutable route history delete succeeded")
	}
	command := routeintegrity.DeviceCommand{TripID: f.trip, DeviceID: "device-a", DeviceCommandID: "command-a", CommandType: "LOCATION_FIX", Payload: map[string]any{"latitude": 6.5}, ClientTimestamp: time.Now().UTC(), LastKnownServerVersion: 1, IdempotencyKey: "offline-a"}
	fp := "stable-fingerprint"
	resultA := routeintegrity.DeviceCommandResult{ID: uuid.NewString(), State: "ACCEPTED", CommandType: "LOCATION_FIX", ServerVersion: 1}
	resultB := resultA
	resultB.ID = uuid.NewString()
	resultCh := make(chan error, 2)
	wg = sync.WaitGroup{}
	for _, result := range []routeintegrity.DeviceCommandResult{resultA, resultB} {
		wg.Add(1)
		go func(result routeintegrity.DeviceCommandResult) {
			defer wg.Done()
			_, e := repo.RecordDeviceCommand(ctx, f.provider, command, result, fp, time.Now().UTC())
			resultCh <- e
		}(result)
	}
	wg.Wait()
	close(resultCh)
	for e := range resultCh {
		if e != nil {
			t.Fatal(e)
		}
	}
	var commands int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_device_commands WHERE provider_id=$1::uuid`, f.provider).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != 1 {
		t.Fatalf("device commands=%d", commands)
	}
}

func TestDeliveryRecoveryMigrationRollbackReapply(t *testing.T) {
	pool, cleanup := operationalTestPoolThrough(t, "000048_delivery_recovery_route_integrity.up.sql")
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000048_delivery_recovery_route_integrity.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000048_delivery_recovery_route_integrity.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
}
