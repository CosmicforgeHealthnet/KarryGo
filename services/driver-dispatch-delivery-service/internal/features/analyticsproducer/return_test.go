package analyticsproducer

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	"cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// returnExecution creates the return-to-sender execution record exactly as
// deliveryrecovery does: a dispatch_bookings row linked to the original trip
// by dispatch_return_workflows, inserted in one transaction.
func returnExecution(t *testing.T, db *pgxpool.Pool, customer, originalTrip, originalBooking, rider string) string {
	t.Helper()
	ctx := context.Background()
	attempt, recovery, workflow := uuid.NewString(), uuid.NewString(), uuid.NewString()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,arrived_at,grace_expires_at,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,1,now(),now()+interval '5 minutes','test-policy',9,$4,'fp')`, attempt, originalTrip, rider, "attempt-"+attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,'RETURN_TO_SENDER','REQUESTED','test-policy',9,$6,'fp')`, recovery, attempt, originalTrip, originalBooking, customer, "recovery-"+recovery); err != nil {
		t.Fatal(err)
	}
	var returnBooking string
	if err := tx.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,package_type) VALUES($1,'b',0,0,'a',0,0,0,'Original sender','','return_manifest') RETURNING id::text`, customer).Scan(&returnBooking); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_return_workflows(id,recovery_action_id,original_trip_id,return_booking_id,route_reference,route_snapshot,policy_version) VALUES($1,$2,$3,$4,'route','{}','test-policy')`, workflow, recovery, originalTrip, returnBooking); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return returnBooking
}

// Decision 3 (Sections 6 and 9): a return-to-sender is part of the original
// customer booking. Its execution record stays traceable to the original but
// is never counted, listed, cancelled or reviewed as a second customer order.
func TestPostgresDispatchReturnIsPartOfTheOriginalBooking(t *testing.T) {
	db := testfixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	repo := booking.NewPostgresRepository(db)
	service := booking.NewService(repo, nil, nil, nil, nil, nil, 0)
	rider := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'P1','N1')`, rider); err != nil {
		t.Fatal(err)
	}
	customer := uuid.NewString()
	var original string
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'a',6.52,3.37,'b',0,0,100,'Ada','+2348000000000') RETURNING id::text`, customer).Scan(&original); err != nil {
		t.Fatal(err)
	}
	originalTrip := insertTrip(t, db, original, rider)
	tripEvent(t, db, "trip.delivery_failed.v1", originalTrip, 3, rider)

	returnBooking := returnExecution(t, db, customer, originalTrip, original, rider)
	returnTrip := insertTrip(t, db, returnBooking, rider)
	for i, event := range []string{"trip.completed.v1", "trip.cancelled.v1", "trip.delivery_failed.v1"} {
		tripEvent(t, db, event, returnTrip, int64(10+i), rider)
	}
	// A recovery on the return trip is real and is kept.
	tripEvent(t, db, "dispatch.delivery_recovery.selected.v1", returnTrip, 20, rider)
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='cancelled' WHERE id=$1`, returnBooking); err != nil {
		t.Fatal(err)
	}

	// The order is the original booking: committed, assigned, failed once.
	wantKinds(t, "original booking", kindCounts(t, db, "DISPATCH_PARCEL", original, originalTrip), map[networkanalytics.Kind]int{
		networkanalytics.KindMissionCommitted: 1, networkanalytics.KindMissionAssigned: 1, networkanalytics.KindMissionFailed: 1})
	// The return execution adds no order lifecycle fact of its own.
	wantKinds(t, "return execution", kindCounts(t, db, "DISPATCH_PARCEL", returnBooking, returnTrip), map[networkanalytics.Kind]int{})
	var recoveries int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND aggregate_id=$1 AND event_type='dispatch.delivery_recovery.selected.v1'`, returnTrip).Scan(&recoveries); err != nil {
		t.Fatal(err)
	}
	if recoveries != 1 {
		t.Fatalf("return trip recovery fact dropped: %d facts", recoveries)
	}

	// Traceable: the return execution names the original trip and booking.
	var linkedTrip, linkedBooking string
	if err := db.QueryRow(ctx, `SELECT rw.original_trip_id::text, t.booking_id::text FROM dispatch_return_workflows rw JOIN trips t ON t.id=rw.original_trip_id WHERE rw.return_booking_id=$1`, returnBooking).Scan(&linkedTrip, &linkedBooking); err != nil {
		t.Fatal(err)
	}
	if linkedTrip != originalTrip || linkedBooking != original {
		t.Fatalf("return links to trip %s booking %s, want %s %s", linkedTrip, linkedBooking, originalTrip, original)
	}
	if isReturn, err := repo.IsReturnExecution(ctx, returnBooking); err != nil || !isReturn {
		t.Fatalf("return execution not recognised: %v %v", isReturn, err)
	}
	if isReturn, err := repo.IsReturnExecution(ctx, original); err != nil || isReturn {
		t.Fatalf("original booking recognised as a return: %v %v", isReturn, err)
	}

	// The customer sees one booking, and cannot treat the return as its own.
	listed, err := service.ListBookings(ctx, customer, booking.ListBookingsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != original {
		t.Fatalf("customer list %+v, want only the original booking %s", listed, original)
	}
	if _, err := service.GetBooking(ctx, customer, returnBooking); err != nil {
		t.Fatalf("return execution no longer readable for traceability: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='searching' WHERE id=$1`, returnBooking); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelBooking(ctx, customer, returnBooking, ""); !isConflict(err) {
		t.Fatalf("customer cancelled a return execution as its own booking: %v", err)
	}
	if _, err := service.SubmitReview(ctx, customer, returnBooking, booking.ReviewInput{Rating: 5}); !isConflict(err) {
		t.Fatalf("customer reviewed a return execution as its own booking: %v", err)
	}
}

func isConflict(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Status == http.StatusConflict
}
