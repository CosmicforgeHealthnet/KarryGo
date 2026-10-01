package bookingusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
)

type riskProviders struct{ suspended bool }

func (r riskProviders) GetByID(context.Context, string) (providerauthmodels.Provider, error) {
	p := providerauthmodels.Provider{AccountStatus: providerauthmodels.AccountStatusActive, OnboardingStatus: providerauthmodels.OnboardingComplete,
		IdentityVerificationStatus: providerauthmodels.IdentityVerificationVerified, VehicleVerificationStatus: providerauthmodels.VehicleVerificationVerified,
		DocumentComplianceStatus: providerauthmodels.DocumentComplianceCompliant}
	if r.suspended {
		p.AccountStatus = providerauthmodels.AccountStatusSuspended
	}
	return p, nil
}

type riskVehicles struct{ none bool }

func (r riskVehicles) FirstActiveByProvider(context.Context, string) (providerprofilemodels.Vehicle, error) {
	if r.none {
		return providerprofilemodels.Vehicle{}, errors.New("no active vehicle")
	}
	return providerprofilemodels.Vehicle{ID: "v1", Status: "active"}, nil
}

type rejectingGate struct{ reject bool }

func (g rejectingGate) FilterPassengerCandidates(_ context.Context, _ bookingmodels.Booking, _ string, _ []byte, ids []string, _ string) ([]string, error) {
	if g.reject {
		return nil, nil
	}
	return ids, nil
}

type recordingRiskStore struct {
	bookings []bookingmodels.Booking
	reopened []string
}

func (r *recordingRiskStore) AcceptedScheduledBookings(context.Context, time.Time, int) ([]bookingmodels.Booking, error) {
	return r.bookings, nil
}
func (r *recordingRiskStore) ReopenForRecovery(_ context.Context, booking bookingmodels.Booking, _ string, trigger string) (bool, error) {
	r.reopened = append(r.reopened, booking.ID+":"+trigger)
	return true, nil
}
func (r *recordingRiskStore) ResolveRecoveredBookings(context.Context) (int, error) { return 0, nil }

func scheduledBooking() bookingmodels.Booking {
	provider := "provider-1"
	at := time.Now().Add(time.Hour)
	return bookingmodels.Booking{ID: "booking-1", Status: bookingmodels.StatusAccepted, ProviderID: &provider, ScheduledAt: &at, PassengerCount: 2}
}

func TestScheduledRiskKeepsHealthyAssignment(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo(), Providers: riskProviders{}, Vehicles: riskVehicles{}, CapabilityGate: rejectingGate{}})
	if trigger := svc.EvaluateScheduledRisk(context.Background(), scheduledBooking()); trigger != "" {
		t.Fatalf("healthy scheduled assignment flagged %s", trigger)
	}
}

func TestScheduledRiskDetectsInvalidAssignment(t *testing.T) {
	for name, tc := range map[string]struct {
		opts Options
		want string
	}{
		"provider suspended":       {Options{Providers: riskProviders{suspended: true}, Vehicles: riskVehicles{}}, RiskProviderNotCompliant},
		"vehicle not operational":  {Options{Providers: riskProviders{}, Vehicles: riskVehicles{none: true}}, RiskVehicleNotOperational},
		"product eligibility lost": {Options{Providers: riskProviders{}, Vehicles: riskVehicles{}, CapabilityGate: rejectingGate{reject: true}}, RiskProductNotEligible},
	} {
		tc.opts.Bookings = newFakeBookingRepo()
		if got := NewBookingService(tc.opts).EvaluateScheduledRisk(context.Background(), scheduledBooking()); got != tc.want {
			t.Errorf("%s: trigger = %q, want %q", name, got, tc.want)
		}
	}
}

func TestScheduledRiskMonitorRecoversInsteadOfCancelling(t *testing.T) {
	store := &recordingRiskStore{bookings: []bookingmodels.Booking{scheduledBooking()}}
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo(), Providers: riskProviders{suspended: true}, Vehicles: riskVehicles{}, RiskStore: store})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.RunScheduledRiskMonitor(ctx, time.Hour, 2*time.Hour)
	if len(store.reopened) != 1 || store.reopened[0] != "booking-1:"+RiskProviderNotCompliant {
		t.Fatalf("reopened = %v", store.reopened)
	}
	if b, err := svc.bookings.GetByID(context.Background(), "booking-1"); err == nil && b.Status == bookingmodels.StatusCancelled {
		t.Fatal("scheduled booking must never be cancelled by the risk monitor")
	}
}
