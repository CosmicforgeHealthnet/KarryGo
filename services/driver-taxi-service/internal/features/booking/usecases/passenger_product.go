package bookingusecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ProductPolicySource supplies the current service-product policy. It is nil
// while TAXI_SERVICE_PRODUCTS_ENABLED is off.
type ProductPolicySource interface {
	CurrentPolicy(ctx context.Context) (capability.Policy, error)
}

// CustomerContextSource hands saved customer mobility needs to Taxi. It is
// nil when the customer context integration is not configured.
type CustomerContextSource interface {
	MobilityHints(ctx context.Context, customerID string) capability.CustomerMobilityHints
}

// GuestTravellerSource reads the traveller a customer books for. The customer
// context client implements it; without it, booking for someone else is
// refused as unavailable.
type GuestTravellerSource interface {
	GuestTraveller(ctx context.Context, customerID, guestTravellerID string) (capability.GuestTraveller, error)
}

func requestStatesRequirements(input CreateBookingInput) bool {
	hard := input.HardRequirements
	truthy := func(v *bool) bool { return v != nil && *v }
	return strings.TrimSpace(input.ServiceProductCode) != "" || strings.TrimSpace(input.GuestTravellerID) != "" || truthy(hard.WheelchairRequired) || truthy(hard.ChildSeatRequired) || truthy(hard.PetTravel)
}

// freezePassengerProduct resolves the product and hard requirements for a new
// booking and stamps the immutable snapshot onto it. With service products
// disabled, a request that states a product or a hard requirement is refused
// instead of being silently dropped: an ignored wheelchair requirement would
// match any vehicle.
func (s *BookingService) freezePassengerProduct(ctx context.Context, input CreateBookingInput, booking *bookingmodels.Booking, now time.Time) error {
	if s.products == nil {
		if requestStatesRequirements(input) {
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "requirements", Message: "Service products and ride requirements are not available yet."},
			})
		}
		return nil
	}
	policy, err := s.products.CurrentPolicy(ctx)
	if err != nil {
		return apperrors.Unavailable("Ride could not be created.", err)
	}
	hints := capability.CustomerMobilityHints{Status: capability.CustomerContextNotRequested}
	var guest *capability.GuestTraveller
	if id := strings.TrimSpace(input.GuestTravellerID); id != "" {
		// Booking for someone else: the traveller's own needs gate matching,
		// never the booking customer's profile, and an unreadable traveller
		// refuses the booking rather than booking the customer instead.
		if booking.RidePurpose != bookingmodels.RidePurposePassenger {
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "guest_traveller_id", Message: "A traveller applies only to passenger rides."}})
		}
		source, ok := s.customerContext.(GuestTravellerSource)
		if !ok {
			return apperrors.Unavailable("Booking for someone else is not available right now.", nil)
		}
		read, err := source.GuestTraveller(ctx, booking.CustomerID, id)
		switch {
		case errors.Is(err, capability.ErrGuestTravellerNotFound):
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "guest_traveller_id", Message: "Select one of your saved travellers."}})
		case err != nil:
			return apperrors.Unavailable("Booking for someone else is not available right now.", err)
		}
		guest, hints = &read, read.Hints
	} else if s.customerContext != nil {
		hints = s.customerContext.MobilityHints(ctx, booking.CustomerID)
	}
	snapshot, product, err := capability.BuildPassengerSnapshot(policy, input.ServiceProductCode, booking.VehicleClass, booking.PassengerCount,
		input.HardRequirements, input.Preferences, hints, now)
	switch {
	case errors.Is(err, capability.ErrUnknownProduct), errors.Is(err, capability.ErrProductPurposeMismatch):
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "service_product_code", Message: "Select an available ride product."}})
	case errors.Is(err, capability.ErrProductClassMismatch):
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "service_product_code", Message: "This ride product does not match the vehicle class."}})
	case errors.Is(err, capability.ErrProductPassengerLimit):
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "passenger_count", Message: "Too many passengers for this ride product."}})
	case err != nil:
		return apperrors.Unavailable("Ride could not be created.", err)
	}
	if guest != nil {
		snapshot = snapshot.ForGuest(guest.Snapshot)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return apperrors.Unavailable("Ride could not be created.", err)
	}
	code, version, policyVersion, guarantee := product.Code, product.Version, snapshot.PolicyVersion, snapshot.GuaranteeProfileVersion
	booking.ServiceProductCode = &code
	booking.ServiceProductVersion = &version
	booking.EligibilityPolicyVersion = &policyVersion
	booking.GuaranteeProfileVersion = &guarantee
	booking.RequirementSnapshot = raw
	return nil
}
