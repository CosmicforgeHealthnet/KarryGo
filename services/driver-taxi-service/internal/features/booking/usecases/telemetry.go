package bookingusecases

import (
	"context"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingtelemetry "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

// PositionUpdater is the slice of provider_availability's Redis store this
// package needs to publish a driver's LIVE position once a telemetry point
// has passed validation -- see availabilityrepositories.AvailabilityStore's
// own Heartbeat for the exact effects (status key + geo index write).
// Declared locally rather than depending on availabilityusecases.
// AvailabilityService, for the same "consumer declares what it needs"
// reason as MarketLookup/AvailabilityCandidateStore above -- and because the
// assigned BOOKING (market/vehicle-class/vehicle-id, all already loaded for
// the ownership check) is the authoritative source here, not whatever the
// driver last heartbeat with, so the usecase-level Heartbeat's own
// preserve-and-re-resolve logic would be the wrong tool for this call site.
type PositionUpdater interface {
	Heartbeat(ctx context.Context, status availabilityrepositories.ProviderStatus) error
}

// telemetryActiveStatuses is the subset of statuses during which a location
// point can be attributed to a specific ride: an offer (pending_match/
// awaiting_acceptance) has no committed driver yet, and a terminal/completed
// booking has nothing left to track.
var telemetryActiveStatuses = map[bookingmodels.Status]bool{
	bookingmodels.StatusAccepted:        true,
	bookingmodels.StatusEnRoutePickup:   true,
	bookingmodels.StatusArrivedAtPickup: true,
	bookingmodels.StatusInProgress:      true,
}

// LocationPointInput is one GPS fix from a driver app's batched upload.
// Sequence is caller-assigned and must be monotonically increasing per
// booking -- the client, not the server, owns numbering (typically a simple
// local counter), since the server has no other way to know intended order
// across a batch that may include queued-while-offline points.
type LocationPointInput struct {
	BookingID     string
	Sequence      int64
	Lat           float64
	Lng           float64
	Accuracy      float64
	Speed         float64
	Heading       float64
	RecordedAt    time.Time
	Source        string
	ClientEventID string
}

// LocationPointResult reports what happened to one submitted point. Error
// is set (and Stored left false) for a point this call refused outright --
// wrong owner, inactive ride, invalid coordinate -- as distinct from a point
// that was accepted and durably stored despite being flagged (IntegrityFlags
// non-empty, Error empty, Stored true): the addendum's own distinction
// between "not this driver's ride to report on" and "reported, but looks
// wrong" (see the package doc comment on bookingtelemetry).
type LocationPointResult struct {
	BookingID      string   `json:"booking_id"`
	Sequence       int64    `json:"sequence"`
	Stored         bool     `json:"stored"`
	IntegrityFlags []string `json:"integrity_flags,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// IngestLocationPoints processes one batch from one authenticated driver.
// Never fails the whole batch for one bad point -- each point gets its own
// result, matching the addendum §8 requirement that ingestion "never
// silently drops or silently trusts" a point: a rejected point's caller
// finds out via its own Error, not a generic batch failure that obscures
// which point (if any) was fine.
func (s *BookingService) IngestLocationPoints(ctx context.Context, providerID string, inputs []LocationPointInput) ([]LocationPointResult, error) {
	if s.telemetry == nil {
		return nil, apperrors.Unavailable("Trip telemetry is not available right now.", nil)
	}
	if len(inputs) == 0 {
		return nil, apperrors.BadRequest("At least one location point is required.", nil)
	}

	results := make([]LocationPointResult, 0, len(inputs))
	bookings := map[string]bookingmodels.Booking{}
	lastPoint := map[string]*bookingtelemetry.Point{}
	seeded := map[string]bool{}
	now := time.Now()

	for _, in := range inputs {
		result := LocationPointResult{BookingID: in.BookingID, Sequence: in.Sequence}

		fix := geo.Point{Lat: in.Lat, Lng: in.Lng}
		if !fix.HasFix() {
			result.Error = "Location is not a valid coordinate."
			results = append(results, result)
			continue
		}

		booking, ok := bookings[in.BookingID]
		if !ok {
			fetched, err := s.bookings.GetByIDForProvider(ctx, in.BookingID, providerID)
			if err != nil {
				result.Error = "This ride is not assigned to you."
				results = append(results, result)
				continue
			}
			booking = fetched
			bookings[in.BookingID] = booking
		}
		if !telemetryActiveStatuses[booking.Status] {
			result.Error = "This ride is not currently active."
			results = append(results, result)
			continue
		}

		if !seeded[in.BookingID] {
			existing, has, err := s.telemetry.Latest(ctx, in.BookingID, providerID)
			if err != nil {
				result.Error = "Point could not be processed."
				results = append(results, result)
				continue
			}
			if has {
				p := existing
				lastPoint[in.BookingID] = &p
			}
			seeded[in.BookingID] = true
		}

		point := bookingtelemetry.Point{
			BookingID: in.BookingID, ProviderID: providerID, Sequence: in.Sequence,
			Lat: in.Lat, Lng: in.Lng,
			RecordedAt: in.RecordedAt, ReceivedAt: now,
			Source:        firstNonEmptyString(in.Source, "gps"),
			ClientEventID: in.ClientEventID,
		}
		if in.Accuracy != 0 {
			v := in.Accuracy
			point.AccuracyM = &v
		}
		if in.Speed != 0 {
			v := in.Speed
			point.SpeedMps = &v
		}
		if in.Heading != 0 {
			v := in.Heading
			point.HeadingDeg = &v
		}

		flags := bookingtelemetry.Classify(lastPoint[in.BookingID], point, now, s.telemetryConfig)
		point.IntegrityFlags = flags
		result.IntegrityFlags = flags

		inserted, err := s.telemetry.Insert(ctx, point)
		if err != nil {
			result.Error = "Point could not be stored."
			results = append(results, result)
			continue
		}
		result.Stored = inserted

		// The next point in THIS batch for the same booking continues the
		// trail from what was just processed, flagged or not -- matching
		// what a fresh Latest() read would see immediately afterward.
		stored := point
		lastPoint[in.BookingID] = &stored

		if inserted && len(flags) == 0 && s.positions != nil {
			// Best-effort: a live-tracking mirror failure must never fail
			// telemetry ingestion. The durable point above is already
			// safely stored either way.
			_ = s.positions.Heartbeat(ctx, availabilityrepositories.ProviderStatus{
				ProviderID:   providerID,
				VehicleID:    derefOrEmpty(booking.VehicleID),
				MarketID:     booking.MarketID,
				VehicleClass: booking.VehicleClass,
				Lat:          in.Lat, Lng: in.Lng,
				Heading: in.Heading, Speed: in.Speed, Accuracy: in.Accuracy,
			})
		}

		results = append(results, result)
	}

	return results, nil
}

// resolveActualDistanceKm prefers the server-computed distance from the
// booking's validated telemetry trail (see bookingtelemetry.ActualDistanceKm)
// over clientReported, falling back to clientReported when telemetry is not
// configured, the trail read fails, or fewer than two clean points exist to
// derive a path from. Called from MarkCompleted in lifecycle.go.
func (s *BookingService) resolveActualDistanceKm(ctx context.Context, bookingID string, clientReported float64) float64 {
	if s.telemetry == nil {
		return clientReported
	}
	trail, err := s.telemetry.ValidatedTrail(ctx, bookingID)
	if err != nil {
		return clientReported
	}
	if km, ok := bookingtelemetry.ActualDistanceKm(trail); ok {
		return km
	}
	return clientReported
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
