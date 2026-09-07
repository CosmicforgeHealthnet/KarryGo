package bookingusecases

import (
	"context"
	"testing"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
)

type commerceProviderViewRepo struct {
	bookingrepo.BookingRepository
	booking  bookingmodels.Booking
	metadata *bookingmodels.CommerceOperations
}

func (r commerceProviderViewRepo) GetByID(context.Context, string) (bookingmodels.Booking, error) {
	return r.booking, nil
}

func (r commerceProviderViewRepo) CommerceProviderMetadata(context.Context, string) (*bookingmodels.CommerceOperations, error) {
	return r.metadata, nil
}

func TestGetProviderBookingCommerceViewKeepsOperationsAndStripsCommerceFinance(t *testing.T) {
	providerID := "provider-1"
	estimate, final := int64(100000), int64(120000)
	repo := commerceProviderViewRepo{
		booking: bookingmodels.Booking{
			ID: "booking-1", ProviderID: &providerID,
			FareEstimateKobo: &estimate, FareFinalKobo: &final,
			PaymentMethod: bookingmodels.PaymentMethodWallet,
			PaymentStatus: bookingmodels.PaymentStatusHeld,
		},
		metadata: &bookingmodels.CommerceOperations{
			SourceType: "VENDOR_COMMERCE", OperationRef: "operation-1",
			VehicleClass: "TRUCK", LoadingDurationSeconds: 900,
		},
	}
	service := NewBookingService(Options{Bookings: repo})

	view, err := service.GetProviderBooking(context.Background(), "booking-1", providerID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Commerce == nil || view.Commerce.OperationRef != "operation-1" ||
		view.Commerce.LoadingDurationSeconds != 900 {
		t.Fatalf("operational metadata missing: %+v", view.Commerce)
	}
	if view.FareEstimateKobo != nil || view.FareFinalKobo != nil ||
		view.PaymentMethod != "" || view.PaymentStatus != "" {
		t.Fatalf("commerce financial data leaked: %+v", view)
	}
}
