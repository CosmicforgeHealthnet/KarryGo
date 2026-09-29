package commercemission

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sharedmission "cosmicforge/logistics/shared/go/commercemission"
)

func TestHaulingCommerceBookingPersistsReceiverAccount(t *testing.T) {
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	id, senderID, receiverID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	request := sharedmission.Request{MissionRef: "receiver-" + id, OperationRef: "receiver-op-" + id, ParentOrderRef: "order-" + id,
		VendorOrderRefs: []string{"vo-" + id}, CustomerRef: senderID, ReceiverCustomerRef: receiverID,
		Pickup:  sharedmission.Point{Address: "pickup", Latitude: 6.4, Longitude: 3.3},
		Dropoff: sharedmission.Point{Address: "dropoff", Latitude: 6.5, Longitude: 3.4, Phone: "+390001234567"},
		Load:    sharedmission.Load{WeightGrams: 1000}, AgreedFeeMinor: 1000, Currency: "NGN"}
	record, err := NewRepository(pool).CreateMissionWithBooking(ctx, CreateInput{Request: request, RequestFingerprint: "receiver-fingerprint-" + id, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM commerce_missions WHERE mission_ref=$1`, request.MissionRef)
		_, _ = pool.Exec(context.Background(), `DELETE FROM haulage_bookings WHERE id=$1::uuid`, record.BookingID)
	})
	var stored string
	if err = pool.QueryRow(ctx, `SELECT receiver_customer_id::text FROM haulage_bookings WHERE id=$1::uuid`, record.BookingID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != receiverID || stored == senderID {
		t.Fatalf("commerce booking receiver account=%s", stored)
	}
}
