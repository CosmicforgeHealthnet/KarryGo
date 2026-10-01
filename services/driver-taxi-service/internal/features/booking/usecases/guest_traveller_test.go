package bookingusecases

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
)

// guestContext answers the booking customer's own profile AND one guest, so
// a test can prove which of the two actually gated the ride.
type guestContext struct {
	own    capability.CustomerMobilityHints
	guests map[string]capability.GuestTraveller
	err    error
	asked  []string
}

func (g *guestContext) MobilityHints(context.Context, string) capability.CustomerMobilityHints {
	return g.own
}

func (g *guestContext) GuestTraveller(_ context.Context, customerID, id string) (capability.GuestTraveller, error) {
	g.asked = append(g.asked, customerID+"/"+id)
	if g.err != nil {
		return capability.GuestTraveller{}, g.err
	}
	guest, ok := g.guests[id]
	if !ok {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerNotFound
	}
	return guest, nil
}

func newGuestContext() *guestContext {
	yes, no, phone := true, false, "+2348011111111"
	return &guestContext{
		// The customer booking the ride uses a wheelchair; the guest does not.
		own: capability.CustomerMobilityHints{Status: capability.CustomerContextUsed, Version: 9, WheelchairRequired: &yes},
		guests: map[string]capability.GuestTraveller{"guest-1": {
			Snapshot: capability.TravellerSnapshot{GuestTravellerRef: "guest-1", Name: "Ada", Phone: &phone, SourceVersion: 4},
			Hints:    capability.CustomerMobilityHints{Status: capability.CustomerContextUsed, Version: 4, WheelchairRequired: &no, PetTravelPreference: strPtr("SMALL_PET")},
		}},
	}
}

func strPtr(s string) *string { return &s }

// E2E-02 (Customer V4 §5.1/§5.2): booking for a saved traveller freezes that
// traveller into the booking, the traveller's needs gate matching instead of
// the customer's, and the booking stays the customer's.
func TestGuestBookingFreezesTravellerAndUsesTheirNeeds(t *testing.T) {
	ctx := newGuestContext()
	s := &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: ctx}
	booking := bookingmodels.Booking{CustomerID: "customer-a", VehicleClass: "comfort", PassengerCount: 1, RidePurpose: bookingmodels.RidePurposePassenger}
	if err := s.freezePassengerProduct(context.Background(), CreateBookingInput{GuestTravellerID: " guest-1 "}, &booking, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(ctx.asked) != 1 || ctx.asked[0] != "customer-a/guest-1" {
		t.Fatalf("traveller must be read for the booking customer only: %v", ctx.asked)
	}
	var snap capability.PassengerSnapshot
	if err := json.Unmarshal(booking.RequirementSnapshot, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Traveller == nil || snap.Traveller.Kind != capability.TravellerKindGuest || snap.Traveller.GuestTravellerRef != "guest-1" ||
		snap.Traveller.Name != "Ada" || snap.Traveller.SourceVersion != 4 {
		t.Fatalf("traveller not frozen: %+v", snap.Traveller)
	}
	if snap.WheelchairRequired {
		t.Fatal("the customer's own wheelchair need must not gate a guest's ride")
	}
	if !snap.PetTravel || snap.Sources["pet_travel"] != capability.SourceGuestTraveller || snap.Sources["wheelchair_required"] != capability.SourceGuestTraveller {
		t.Fatalf("guest needs must be sourced from the traveller: %+v %v", snap.PassengerRequirement, snap.Sources)
	}
	if booking.CustomerID != "customer-a" {
		t.Fatal("the booking must remain the customer's")
	}
	// The customer's own read shows the frozen traveller.
	public := booking.Public()
	if public.Requirements == nil || !strings.Contains(string(public.Requirements.Traveller), `"name":"Ada"`) {
		t.Fatalf("owner view lost the traveller: %+v", public.Requirements)
	}

	// An explicit request still wins over the traveller's saved need.
	explicit := true
	again := bookingmodels.Booking{CustomerID: "customer-a", VehicleClass: "comfort", PassengerCount: 1, RidePurpose: bookingmodels.RidePurposePassenger}
	if err := s.freezePassengerProduct(context.Background(), CreateBookingInput{GuestTravellerID: "guest-1",
		HardRequirements: capability.HardRequirementInput{WheelchairRequired: &explicit}}, &again, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(again.RequirementSnapshot, &snap)
	if !snap.WheelchairRequired || snap.Sources["wheelchair_required"] != capability.SourceRequest {
		t.Fatalf("explicit request must win: %v", snap.Sources)
	}
}

func TestGuestBookingNeverFallsBackToBookingTheCustomer(t *testing.T) {
	passenger := func() bookingmodels.Booking {
		return bookingmodels.Booking{CustomerID: "customer-a", VehicleClass: "comfort", PassengerCount: 1, RidePurpose: bookingmodels.RidePurposePassenger}
	}
	cases := []struct {
		name    string
		service *BookingService
		input   CreateBookingInput
		booking bookingmodels.Booking
		code    string
	}{
		{"foreign or unknown traveller", &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: newGuestContext()},
			CreateBookingInput{GuestTravellerID: "someone-elses"}, passenger(), "validation_failed"},
		{"customer service unavailable", &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: &guestContext{err: capability.ErrGuestTravellerUnavailable}},
			CreateBookingInput{GuestTravellerID: "guest-1"}, passenger(), "service_unavailable"},
		{"no customer context configured", &BookingService{products: staticPolicy{policy: passengerPolicy()}},
			CreateBookingInput{GuestTravellerID: "guest-1"}, passenger(), "service_unavailable"},
		{"context without guest support", &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: staticHints{Status: capability.CustomerContextUsed}},
			CreateBookingInput{GuestTravellerID: "guest-1"}, passenger(), "service_unavailable"},
		{"service products disabled", &BookingService{customerContext: newGuestContext()},
			CreateBookingInput{GuestTravellerID: "guest-1"}, passenger(), "validation_failed"},
		{"parcel ride", &BookingService{products: staticPolicy{policy: passengerPolicy()}, customerContext: newGuestContext()},
			CreateBookingInput{GuestTravellerID: "guest-1"}, bookingmodels.Booking{CustomerID: "customer-a", VehicleClass: "comfort", PassengerCount: 1, RidePurpose: bookingmodels.RidePurposeParcel}, "validation_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			booking := tc.booking
			err := tc.service.freezePassengerProduct(context.Background(), tc.input, &booking, time.Now())
			if appCode(err) != tc.code {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
			if booking.RequirementSnapshot != nil {
				t.Fatalf("a refused guest booking must freeze nothing: %s", booking.RequirementSnapshot)
			}
		})
	}
}
