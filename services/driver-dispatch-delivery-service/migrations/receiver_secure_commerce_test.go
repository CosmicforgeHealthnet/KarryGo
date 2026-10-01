package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	dispatchmission "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercemission"
	"cosmicforge/logistics/shared/go/commercemission"
)

func TestDispatchCommerceBookingPersistsReceiverAccount(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	id, senderID, receiverID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	request := commercemission.Request{MissionRef: "receiver-" + id, ParentOrderRef: "order-" + id, VendorOrderRefs: []string{"vo-" + id}, CustomerRef: senderID, ReceiverCustomerRef: receiverID,
		Pickup: commercemission.Point{Address: "pickup", Latitude: 6.4, Longitude: 3.3}, Dropoff: commercemission.Point{Address: "dropoff", Latitude: 6.5, Longitude: 3.4, Phone: "+390001234567"},
		AgreedFeeMinor: 1000, Currency: "NGN", Load: commercemission.Load{WeightGrams: 1000}}
	record, err := dispatchmission.NewRepository(pool).CreateMissionWithBooking(ctx, dispatchmission.CreateInput{Request: request, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM commerce_missions WHERE mission_ref=$1`, request.MissionRef)
		_, _ = pool.Exec(context.Background(), `DELETE FROM dispatch_bookings WHERE id=$1::uuid`, record.BookingID)
	})
	var stored string
	if err = pool.QueryRow(ctx, `SELECT receiver_customer_id::text FROM dispatch_bookings WHERE id=$1::uuid`, record.BookingID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != receiverID || stored == senderID {
		t.Fatalf("commerce booking receiver account=%s", stored)
	}
}
