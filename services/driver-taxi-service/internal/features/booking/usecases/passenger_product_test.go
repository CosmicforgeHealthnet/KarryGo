package bookingusecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type staticPolicy struct {
	policy capability.Policy
	err    error
}

func (p staticPolicy) CurrentPolicy(context.Context) (capability.Policy, error) { return p.policy, p.err }

type staticHints capability.CustomerMobilityHints

func (h staticHints) MobilityHints(context.Context, string) capability.CustomerMobilityHints {
	return capability.CustomerMobilityHints(h)
}

func passengerPolicy() capability.Policy {
	return capability.Policy{Version: "p2", Products: []capability.Product{
		{Code: "TAXI_COMFORT", Version: 2, Purpose: capability.PurposePassenger, Active: true, MaxPassengers: 4, LegacyVehicleClass: "comfort"},
	}}
}

func appCode(err error) string {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return string(appErr.Code)
	}
	return ""
}

func TestFreezeRefusesStatedRequirementsWhenProductsDisabled(t *testing.T) {
	s := &BookingService{}
	wheelchair := true
	booking := bookingmodels.Booking{CustomerID: "c", VehicleClass: "comfort", PassengerCount: 1}
	err := s.freezePassengerProduct(context.Background(), CreateBookingInput{HardRequirements: capability.HardRequirementInput{WheelchairRequired: &wheelchair}}, &booking, time.Now())
	if err == nil || appCode(err) != "validation_failed" {
		t.Fatalf("a stated wheelchair requirement must not be silently dropped: %v", err)
	}
	if err := s.freezePassengerProduct(context.Background(), CreateBookingInput{}, &booking, time.Now()); err != nil || booking.RequirementSnapshot != nil {
		t.Fatalf("legacy booking without requirements = %v / %s", err, booking.RequirementSnapshot)
	}
}

func TestFreezeStampsImmutableSnapshotWithCustomerHandoff(t *testing.T) {
	saved := true
	s := &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: staticHints{Status: capability.CustomerContextUsed, Version: 3, WheelchairRequired: &saved}}
	booking := bookingmodels.Booking{CustomerID: "c", VehicleClass: "comfort", PassengerCount: 2}
	if err := s.freezePassengerProduct(context.Background(), CreateBookingInput{}, &booking, time.Now()); err != nil {
		t.Fatal(err)
	}
	if booking.ServiceProductCode == nil || *booking.ServiceProductCode != "TAXI_COMFORT" || *booking.ServiceProductVersion != 2 ||
		*booking.EligibilityPolicyVersion != "p2" || *booking.GuaranteeProfileVersion != capability.GuaranteeProfileNone {
		t.Fatalf("product columns = %+v", booking)
	}
	var snap capability.PassengerSnapshot
	if err := json.Unmarshal(booking.RequirementSnapshot, &snap); err != nil || !snap.WheelchairRequired || snap.CustomerContext.Version != 3 {
		t.Fatalf("snapshot = %+v (%v)", snap, err)
	}
}

func TestFreezeMapsPolicyFailuresToClientErrors(t *testing.T) {
	booking := bookingmodels.Booking{CustomerID: "c", VehicleClass: "comfort", PassengerCount: 1}
	down := &BookingService{products: staticPolicy{err: errors.New("db down")}}
	if err := down.freezePassengerProduct(context.Background(), CreateBookingInput{}, &booking, time.Now()); appCode(err) != "service_unavailable" {
		t.Fatalf("policy outage = %v", err)
	}
	s := &BookingService{products: staticPolicy{policy: passengerPolicy()}}
	if err := s.freezePassengerProduct(context.Background(), CreateBookingInput{ServiceProductCode: "TAXI_NOPE"}, &booking, time.Now()); appCode(err) != "validation_failed" {
		t.Fatalf("unknown product = %v", err)
	}
}
