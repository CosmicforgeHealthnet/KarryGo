package serviceability

import (
	"errors"
	"testing"
	"time"
)

func approvedBookingQuote(now time.Time) Quote {
	return Quote{
		Pickup: Point{Latitude: 6.5, Longitude: 3.3}, Dropoff: Point{Latitude: 6.6, Longitude: 3.4},
		Decision: DecisionServiceable, ExpiresAt: now.Add(time.Minute),
		Package:   PackageDeclaration{PackageType: "parcel", SizeCategory: "medium", DeclaredWeightGrams: 1500, Fragile: true},
		Admission: PackageAdmission{Result: AdmissionAccepted}, PricingInputs: map[string]any{"service_tier": "express"},
	}
}

func TestBookingQuoteBinding(t *testing.T) {
	now := time.Now().UTC()
	quote := approvedBookingQuote(now)
	binding := BookingBinding{
		Pickup: quote.Pickup, Dropoff: quote.Dropoff, PackageType: "PARCEL",
		PackageSize: "medium", PackageWeightGrams: 1500, Fragile: true, ServiceTier: "EXPRESS",
	}
	if err := validateBookingBinding(quote, binding, now); err != nil {
		t.Fatalf("valid binding: %v", err)
	}
	changed := binding
	changed.Dropoff.Latitude += 0.01
	if err := validateBookingBinding(quote, changed, now); !errors.Is(err, ErrQuoteMismatch) {
		t.Fatalf("changed route err=%v", err)
	}
	changed = binding
	changed.PackageWeightGrams++
	if err := validateBookingBinding(quote, changed, now); !errors.Is(err, ErrQuoteMismatch) {
		t.Fatalf("changed package err=%v", err)
	}
	quote.ExpiresAt = now
	if err := validateBookingBinding(quote, binding, now); !errors.Is(err, ErrQuoteExpired) {
		t.Fatalf("expired quote err=%v", err)
	}
}

func TestBookingQuoteMustBeApproved(t *testing.T) {
	now := time.Now().UTC()
	quote := approvedBookingQuote(now)
	quote.Decision = DecisionOperationalReview
	binding := BookingBinding{Pickup: quote.Pickup, Dropoff: quote.Dropoff, PackageType: "parcel", PackageSize: "medium", PackageWeightGrams: 1500, Fragile: true, ServiceTier: "express"}
	if err := validateBookingBinding(quote, binding, now); !errors.Is(err, ErrQuoteRejected) {
		t.Fatalf("review quote err=%v", err)
	}
}
