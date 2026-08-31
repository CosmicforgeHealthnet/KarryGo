// Ops surface (phase B10): read-only live/unmatched views, admin actions,
// and route replay. Every mutating action here is operator-initiated,
// requires a non-empty reason, and is recorded to taxi_admin_audit_events
// (internal/adminaudit) -- never a customer/driver-facing call path. Ops
// mutations reuse the SAME wallet call (refundIfHeld) the customer/provider
// cancel paths already use; nothing here issues a raw wallet write of its
// own, matching the addendum's explicit "never bypass the payment client"
// requirement for this surface.
package bookingusecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/adminaudit"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

// ─── Live / unmatched views ─────────────────────────────────────────────────

// LiveProvider is the ops-view shape of one online provider -- deliberately
// not availabilityrepositories.ProviderStatus verbatim, so this package's
// wire shape does not silently change if that internal DTO grows a field
// this view has no business exposing.
type LiveProvider struct {
	ProviderID   string  `json:"provider_id"`
	VehicleClass string  `json:"vehicle_class"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	UpdatedAt    int64   `json:"updated_at"`
}

type LiveOperationsView struct {
	ActiveBookings  []bookingmodels.PublicBooking `json:"active_bookings"`
	OnlineProviders []LiveProvider                `json:"online_providers"`
}

// ListLiveOperations is the ops "what is happening right now" view: every
// active booking plus every online provider, optionally scoped to one
// market. There is no "role-scoped precision" reduction here -- an
// HMAC-signed internal service call authenticates a CALLING SERVICE, not a
// staff role, and this platform has no mechanism yet for a differentiated
// staff-role view over an internal channel (that would need
// admin-backoffice-service to carry its own staff identity through this
// call, which it does not do today). Full precision for any authenticated
// internal caller is the honest behavior, not a stand-in for the addendum's
// wording.
func (s *BookingService) ListLiveOperations(ctx context.Context, marketID string) (LiveOperationsView, error) {
	if s.availability == nil {
		return LiveOperationsView{}, apperrors.Unavailable("Live operations are not available right now.", nil)
	}
	bookings, err := s.bookings.ListActive(ctx, marketID)
	if err != nil {
		return LiveOperationsView{}, err
	}
	online, err := s.availability.GetOnlineProviders(ctx)
	if err != nil {
		return LiveOperationsView{}, err
	}

	view := LiveOperationsView{
		ActiveBookings:  make([]bookingmodels.PublicBooking, len(bookings)),
		OnlineProviders: make([]LiveProvider, len(online)),
	}
	for i, b := range bookings {
		view.ActiveBookings[i] = b.Public()
	}
	for i, p := range online {
		view.OnlineProviders[i] = LiveProvider{
			ProviderID: p.ProviderID, VehicleClass: p.VehicleClass,
			Lat: p.Lat, Lng: p.Lng, UpdatedAt: p.UpdatedAt,
		}
	}
	return view, nil
}

// UnmatchedFilter narrows GET /internal/operations/unmatched. OlderThan is a
// DURATION ("bookings that have been unmatched for at least this long"), not
// an absolute time -- the usecase resolves it against time.Now() so the
// caller never has to compute a timestamp itself. Zero means no age filter.
//
// Filtering by match ATTEMPT COUNT (named in the build plan) is not
// implemented: it requires the taxi_match_candidates audit trail, which
// phase B2's matcher was scoped to write but never actually wired up -- the
// same disclosed, pre-existing gap earnings insights (phase B8) already
// documents for acceptance-rate reporting. Age/market/zone filtering, which
// IS fully backed by real columns on taxi_bookings, works today.
type UnmatchedFilter struct {
	MarketID        string
	OperatingZoneID string
	OlderThan       time.Duration
}

func (s *BookingService) ListUnmatchedBookings(ctx context.Context, filter UnmatchedFilter) ([]bookingmodels.PublicBooking, error) {
	var cutoff time.Time
	if filter.OlderThan > 0 {
		cutoff = time.Now().Add(-filter.OlderThan)
	}
	bookings, err := s.bookings.ListUnmatched(ctx, filter.MarketID, filter.OperatingZoneID, cutoff)
	if err != nil {
		return nil, err
	}
	out := make([]bookingmodels.PublicBooking, len(bookings))
	for i, b := range bookings {
		out[i] = b.Public()
	}
	return out, nil
}

// ─── Metrics ─────────────────────────────────────────────────────────────

// MetricsSnapshot is the current-state data GET /metrics formats as
// Prometheus text (internal/features/operations/http). Deliberately minimal
// -- bookings by status and a single online-provider count -- rather than
// the addendum §27 table's full breadth: this platform has no metrics
// library anywhere yet (confirmed by searching for one before writing
// this), and every label here is bounded cardinality (Status has a fixed,
// small value set) per that same section's own explicit warning against
// ever labelling by booking_id.
type MetricsSnapshot struct {
	BookingsByStatus map[string]int64
	ProvidersOnline  int64
}

func (s *BookingService) GetMetricsSnapshot(ctx context.Context) (MetricsSnapshot, error) {
	byStatus, err := s.bookings.CountByStatus(ctx)
	if err != nil {
		return MetricsSnapshot{}, err
	}
	var online int64
	if s.availability != nil {
		providers, err := s.availability.GetOnlineProviders(ctx)
		if err != nil {
			return MetricsSnapshot{}, err
		}
		online = int64(len(providers))
	}
	return MetricsSnapshot{BookingsByStatus: byStatus, ProvidersOnline: online}, nil
}

// ─── Admin actions ──────────────────────────────────────────────────────────

func requireReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "reason", Message: "A reason is required for this action."},
		})
	}
	return reason, nil
}

func snapshotBooking(b bookingmodels.Booking) []byte {
	data, err := json.Marshal(b.Public())
	if err != nil {
		return nil
	}
	return data
}

// AdminCancelBooking force-cancels a booking from any non-terminal status,
// including in_progress (the one cancel path that reaches an active trip --
// see AdminCancel's own repository doc comment). Refunds a held payment the
// same way a customer/provider cancel does; never a raw wallet write.
func (s *BookingService) AdminCancelBooking(ctx context.Context, operatorID, bookingID, reason string) (bookingmodels.Booking, error) {
	if s.adminAudit == nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Admin operations are not available right now.", nil)
	}
	reason, err := requireReason(reason)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	bookingID = strings.TrimSpace(bookingID)

	existing, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	before := snapshotBooking(existing)

	tx, err := s.bookings.BeginTx(ctx)
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("This ride could not be updated. Try again.", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cancelled, err := s.bookings.AdminCancel(ctx, tx, bookingID, reason)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	if existing.ProviderID != nil {
		if current, gerr := s.operationalState.Get(ctx, *existing.ProviderID); gerr == nil {
			_, _ = s.operationalState.Transition(ctx, tx, *existing.ProviderID, availabilitymodels.StateAvailable, "admin_cancelled: "+reason, nil, current.Version)
			// Best-effort: a failure to release the provider must not block
			// the cancel itself from committing -- the recovery worker and
			// the provider's own next heartbeat both self-correct a stale
			// operational state eventually.
		}
	}

	if err := s.bookings.RecordEvent(ctx, tx, bookingmodels.BookingEvent{
		BookingID: bookingID, EventType: "taxi.booking.admin_cancelled", ActorType: "admin", ActorID: operatorID,
	}); err != nil {
		return bookingmodels.Booking{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("This ride could not be updated. Try again.", err)
	}

	if rerr := s.refundIfHeld(ctx, existing, "admin_cancelled: "+reason); rerr != nil {
		logging.Error("taxi-payments", "admin cancel refund booking=%s: %v", bookingID, rerr)
	} else if existing.PaymentStatus == bookingmodels.PaymentHeld {
		if serr := s.bookings.SetPaymentStatus(ctx, nil, bookingID, bookingmodels.PaymentRefunded); serr != nil {
			logging.Error("taxi-payments", "admin cancel set refunded booking=%s: %v", bookingID, serr)
		} else {
			cancelled.PaymentStatus = bookingmodels.PaymentRefunded
		}
	}

	if err := s.adminAudit.Record(ctx, adminaudit.Action{
		OperatorID: operatorID, Action: "cancel", EntityType: "booking", EntityID: bookingID,
		Reason: reason, Before: before, After: snapshotBooking(cancelled),
	}); err != nil {
		logging.Error("taxi-ops", "record audit booking=%s: %v", bookingID, err)
	}
	return cancelled, nil
}

// AdminForceRecover manually re-triggers matching for a booking stuck in
// pending_match or awaiting_acceptance -- the same matchBooking goroutine
// every other offer uses, guarded by the SAME booking-level lease (B2), so
// calling this on a booking whose matcher is still alive is a harmless
// no-op (the lease acquire simply fails). Intended for an operator who spots
// a stuck booking before the recovery worker's own next sweep.
func (s *BookingService) AdminForceRecover(ctx context.Context, operatorID, bookingID, reason string) (bookingmodels.Booking, error) {
	if s.adminAudit == nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Admin operations are not available right now.", nil)
	}
	reason, err := requireReason(reason)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	bookingID = strings.TrimSpace(bookingID)

	booking, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if booking.Status != bookingmodels.StatusPendingMatch && booking.Status != bookingmodels.StatusAwaitingAccept {
		return bookingmodels.Booking{}, apperrors.Conflict("Only a pending or awaiting-acceptance ride can be force-recovered.", nil)
	}

	s.spawnMatcher(bookingID)

	if err := s.adminAudit.Record(ctx, adminaudit.Action{
		OperatorID: operatorID, Action: "force_recover", EntityType: "booking", EntityID: bookingID, Reason: reason,
	}); err != nil {
		logging.Error("taxi-ops", "record audit booking=%s: %v", bookingID, err)
	}
	return booking, nil
}

// operationalStateForBookingStatus is release-hold's own "what should this
// driver's state actually be right now" lookup -- the same status-to-state
// ladder lifecycle.go's withTransition call sites already encode
// (MarkMatched->OFFERED, MarkEnRoutePickup->EN_ROUTE_PICKUP, ...), read back
// out for one purpose: releasing a hold must restore the driver to whatever
// their REAL booking progress implies, never unconditionally AVAILABLE.
//
// Found live, not by any test: pausing a driver with an ACCEPTED booking
// still assigned, then releasing the hold, put them back to AVAILABLE --
// eligible for a brand new offer while they still had an existing
// passenger's ride to carry out. A driver mid-ride released from a hold
// must return to the state matching that ride, not to "ready for anything."
func operationalStateForBookingStatus(status bookingmodels.Status) availabilitymodels.OperationalState {
	switch status {
	case bookingmodels.StatusAwaitingAccept, bookingmodels.StatusAccepted:
		return availabilitymodels.StateOffered
	case bookingmodels.StatusEnRoutePickup:
		return availabilitymodels.StateEnRoutePickup
	case bookingmodels.StatusArrivedAtPickup:
		return availabilitymodels.StateAtPickup
	case bookingmodels.StatusInProgress:
		return availabilitymodels.StateOnTrip
	default:
		// Terminal, scheduled, or pending_match/unmatched (no provider
		// assigned to any of these by definition once this function is
		// reached) -- nothing outstanding for this driver, AVAILABLE is
		// correct.
		return availabilitymodels.StateAvailable
	}
}

// adminProviderTransition is shared by AdminPauseProvider/AdminReleaseHold:
// both resolve the booking's assigned provider, then perform one guarded
// operational-state transition, gated by fromStates -- pause only from a
// state that is not already a hold, release only FROM a hold, so this
// action can never be used to yank a provider off ON_TRIP/EN_ROUTE_PICKUP
// state without going through the real trip lifecycle. resolveTo computes
// the TARGET state from the booking itself, since release-hold's correct
// target depends on the booking's current status (see
// operationalStateForBookingStatus), while pause-provider's is always the
// same constant regardless of it.
func (s *BookingService) adminProviderTransition(ctx context.Context, operatorID, bookingID, reason, action string, resolveTo func(bookingmodels.Booking) availabilitymodels.OperationalState, allowed func(availabilitymodels.OperationalState) bool) (availabilitymodels.State, error) {
	if s.adminAudit == nil {
		return availabilitymodels.State{}, apperrors.Unavailable("Admin operations are not available right now.", nil)
	}
	reason, err := requireReason(reason)
	if err != nil {
		return availabilitymodels.State{}, err
	}
	bookingID = strings.TrimSpace(bookingID)

	booking, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return availabilitymodels.State{}, err
	}
	if booking.ProviderID == nil {
		return availabilitymodels.State{}, apperrors.Conflict("This ride has no assigned driver.", nil)
	}
	providerID := *booking.ProviderID

	current, err := s.operationalState.Get(ctx, providerID)
	if err != nil {
		return availabilitymodels.State{}, apperrors.Unavailable("Driver status could not be read.", err)
	}
	if !allowed(current.Current) {
		return availabilitymodels.State{}, apperrors.Conflict("This driver is not in a state this action can be applied to.", nil)
	}

	to := resolveTo(booking)
	updated, err := s.operationalState.Transition(ctx, nil, providerID, to, "admin_"+action+": "+reason, nil, current.Version)
	if err != nil {
		return availabilitymodels.State{}, err
	}

	if err := s.adminAudit.Record(ctx, adminaudit.Action{
		OperatorID: operatorID, Action: action, EntityType: "provider", EntityID: providerID, Reason: reason,
		Before: []byte(`{"state":"` + string(current.Current) + `"}`),
		After:  []byte(`{"state":"` + string(updated.Current) + `"}`),
	}); err != nil {
		logging.Error("taxi-ops", "record audit provider=%s: %v", providerID, err)
	}
	return updated, nil
}

// AdminPauseProvider places the booking's assigned driver into SAFETY_HOLD
// -- an operator-initiated hold for a safety/compliance concern, distinct
// from PAUSED (a driver's own short self-directed break, never operator-
// triggered in this build). Refused if the driver is already in ANY hold
// state (use release-hold first) or already OFFLINE (nothing to pause).
func (s *BookingService) AdminPauseProvider(ctx context.Context, operatorID, bookingID, reason string) (availabilitymodels.State, error) {
	always := func(bookingmodels.Booking) availabilitymodels.OperationalState {
		return availabilitymodels.StateSafetyHold
	}
	return s.adminProviderTransition(ctx, operatorID, bookingID, reason, "pause_provider", always, func(cur availabilitymodels.OperationalState) bool {
		return cur != availabilitymodels.StateSafetyHold && cur != availabilitymodels.StateComplianceHold && cur != availabilitymodels.StateOffline
	})
}

// AdminReleaseHold returns the booking's assigned driver to whichever
// operational state their CURRENT booking status implies (see
// operationalStateForBookingStatus) -- only FROM a hold state, so this can
// never be used to bypass the real trip lifecycle for a driver who is
// genuinely mid-ride, and never blindly to AVAILABLE while they still have
// an outstanding ride.
func (s *BookingService) AdminReleaseHold(ctx context.Context, operatorID, bookingID, reason string) (availabilitymodels.State, error) {
	return s.adminProviderTransition(ctx, operatorID, bookingID, reason, "release_hold", func(b bookingmodels.Booking) availabilitymodels.OperationalState {
		return operationalStateForBookingStatus(b.Status)
	}, func(cur availabilitymodels.OperationalState) bool {
		return cur == availabilitymodels.StateSafetyHold || cur == availabilitymodels.StateComplianceHold
	})
}

// ─── Route replay ───────────────────────────────────────────────────────────

// ReplayEvent is one entry in a booking's merged timeline. Kind identifies
// which source table it came from; Detail is a small, kind-specific summary
// string, never the full row (a replay is a human-readable timeline, not a
// second copy of every table's schema).
type ReplayEvent struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"` // lifecycle_event | operational_state | location_point
	Detail string    `json:"detail"`
}

type BookingReplay struct {
	BookingID string        `json:"booking_id"`
	Events    []ReplayEvent `json:"events"`
}

// GetBookingReplay merges booking_events + this booking's own operational-
// state history + its full (unfiltered -- including flagged) telemetry
// trail into one ordered timeline. Read-only: nothing here can be edited, a
// correction is a new event elsewhere, never a mutation of history.
//
// Notification/payment references (named in the build plan) are NOT
// included: this service has no local record of what notification-service
// or payment-wallet-service actually did with an event it published to
// them, only that the event was handed off (the outbox row). Adding those
// references would mean either querying two other services live (a replay
// read should not depend on two more services being up) or duplicating
// their data locally (a sync problem this service does not need to own) --
// disclosed here rather than faked with the outbox row's own existence.
func (s *BookingService) GetBookingReplay(ctx context.Context, bookingID string) (BookingReplay, error) {
	bookingID = strings.TrimSpace(bookingID)
	if _, err := s.bookings.GetByID(ctx, bookingID); err != nil {
		return BookingReplay{}, err
	}

	var events []ReplayEvent

	lifecycleEvents, err := s.bookings.ListEvents(ctx, bookingID)
	if err != nil {
		return BookingReplay{}, err
	}
	for _, e := range lifecycleEvents {
		events = append(events, ReplayEvent{At: e.CreatedAt, Kind: "lifecycle_event", Detail: e.EventType + " by " + e.ActorType + ":" + e.ActorID})
	}

	if s.operationalHistory != nil {
		stateChanges, err := s.operationalHistory.ListHistoryForBooking(ctx, bookingID)
		if err != nil {
			return BookingReplay{}, err
		}
		for _, c := range stateChanges {
			events = append(events, ReplayEvent{At: c.ChangedAt, Kind: "operational_state", Detail: "driver -> " + c.ToState})
		}
	}

	if s.telemetry != nil {
		points, err := s.telemetry.ListAll(ctx, bookingID)
		if err != nil {
			return BookingReplay{}, err
		}
		for _, p := range points {
			detail := "location point"
			if len(p.IntegrityFlags) > 0 {
				detail += " (flagged: " + strings.Join(p.IntegrityFlags, ",") + ")"
			}
			events = append(events, ReplayEvent{At: p.RecordedAt, Kind: "location_point", Detail: detail})
		}
	}

	sortReplayEvents(events)
	return BookingReplay{BookingID: bookingID, Events: events}, nil
}

func sortReplayEvents(events []ReplayEvent) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].At.Before(events[j-1].At); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}
