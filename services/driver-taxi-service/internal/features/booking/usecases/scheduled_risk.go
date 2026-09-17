package bookingusecases

import (
	"context"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/logging"
)

// ScheduledRiskStore finds accepted future bookings and recovers them.
type ScheduledRiskStore interface {
	AcceptedScheduledBookings(ctx context.Context, before time.Time, limit int) ([]bookingmodels.Booking, error)
	// ReopenForRecovery atomically releases the invalid provider, returns
	// the booking to matching, increments its replacement count and opens a
	// recovery case. It never cancels the booking.
	ReopenForRecovery(ctx context.Context, booking bookingmodels.Booking, providerID, trigger string) (bool, error)
	// ResolveRecoveredBookings closes recovery cases whose booking has a new
	// provider or has reached a terminal state.
	ResolveRecoveredBookings(ctx context.Context) (int, error)
}

// Risk trigger codes for scheduled passenger rides.
const (
	RiskProviderNotCompliant  = "SCHEDULED_PROVIDER_NOT_COMPLIANT"
	RiskVehicleNotOperational = "SCHEDULED_VEHICLE_NOT_OPERATIONAL"
	RiskProductNotEligible    = "SCHEDULED_PRODUCT_NOT_ELIGIBLE"
)

// EvaluateScheduledRisk revalidates one accepted scheduled booking. It
// returns an empty trigger when the assignment is still safe.
func (s *BookingService) EvaluateScheduledRisk(ctx context.Context, booking bookingmodels.Booking) string {
	if booking.ProviderID == nil {
		return ""
	}
	providerID := *booking.ProviderID
	provider, err := s.providers.GetByID(ctx, providerID)
	if err != nil {
		return ""
	}
	if ok, _ := provider.CanGoAvailable(); !ok {
		return RiskProviderNotCompliant
	}
	if _, err := s.vehicles.FirstActiveByProvider(ctx, providerID); err != nil {
		return RiskVehicleNotOperational
	}
	if s.capability != nil {
		kept, err := s.capability.FilterPassengerCandidates(ctx, booking, "", nil, []string{providerID}, s.matching.InstanceID)
		if err == nil && len(kept) == 0 {
			return RiskProductNotEligible
		}
	}
	return ""
}

// RunScheduledRiskMonitor revalidates accepted scheduled rides ahead of
// pickup. An invalid assignment starts recovery and re-matching instead of
// cancelling the customer's booking.
func (s *BookingService) RunScheduledRiskMonitor(ctx context.Context, interval, horizon time.Duration) {
	if s.riskStore == nil {
		return
	}
	sweep := func() {
		bookings, err := s.riskStore.AcceptedScheduledBookings(ctx, time.Now().Add(horizon), 100)
		if err != nil {
			logging.Error("taxi-risk-monitor", "list scheduled bookings: %v", err)
			return
		}
		for _, booking := range bookings {
			trigger := s.EvaluateScheduledRisk(ctx, booking)
			if trigger == "" {
				continue
			}
			reopened, err := s.riskStore.ReopenForRecovery(ctx, booking, *booking.ProviderID, trigger)
			if err != nil {
				logging.Error("taxi-risk-monitor", "recover booking=%s: %v", booking.ID, err)
				continue
			}
			if reopened {
				logging.Notice("taxi-risk-monitor", "booking=%s recovery started trigger=%s", booking.ID, trigger)
				s.spawnMatcher(booking.ID)
			}
		}
		if _, err := s.riskStore.ResolveRecoveredBookings(ctx); err != nil {
			logging.Error("taxi-risk-monitor", "resolve recovered bookings: %v", err)
		}
	}
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
