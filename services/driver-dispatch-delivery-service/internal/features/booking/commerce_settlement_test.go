package booking

import (
	"context"
	"testing"
	"time"
)

type commerceCompletionRepo struct {
	*fakeRepo
	parentID string
	vendors  []string
}

func (r *commerceCompletionRepo) CommerceMissionScope(context.Context, string) (string, []string, bool, error) {
	return r.parentID, append([]string(nil), r.vendors...), true, nil
}

type recordingCommerceSettlement struct{ calls []string }

func (s *recordingCommerceSettlement) ReleaseCommerceDelivery(_ context.Context, parentID, vendorID, providerID, completion string) error {
	s.calls = append(s.calls, parentID+"/"+vendorID+"/"+providerID+"/"+completion)
	return nil
}

func TestCommerceTripCompletionSettlesEachProviderAllocationOncePerStableScope(t *testing.T) {
	base := newFakeRepo()
	base.bookings["booking-a"] = &Booking{ID: "booking-a", CustomerID: "customer-a", ProviderID: "provider-a", Status: StatusAccepted, PaymentStatus: PaymentPaid}
	repo := &commerceCompletionRepo{fakeRepo: base, parentID: "parent-a", vendors: []string{"vendor-a", "vendor-b"}}
	settlement := &recordingCommerceSettlement{}
	service := NewService(repo, nil, nil, nil, nil, nil, 10).WithCommerceSettlement(settlement)
	if err := service.HandleTripCompleted(context.Background(), "booking-a", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(settlement.calls) != 2 || settlement.calls[0] != "parent-a/vendor-a/provider-a/booking-a" || settlement.calls[1] != "parent-a/vendor-b/provider-a/booking-a" {
		t.Fatalf("settlement calls=%v", settlement.calls)
	}
}
