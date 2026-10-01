package bookingusecases

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/services/taxi-service/internal/routing"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/walletclient"
)

// AvailabilityCandidateStore is the slice of provider_availability's Redis
// store the matcher needs: finding nearby drivers and coordinating who gets
// offered a booking. Declared locally (see MarketLookup's own comment for
// why), returning availabilityrepositories.ProviderStatus directly since
// that plain data struct lives in the repository package, not a models
// package, in this one case.
type AvailabilityCandidateStore interface {
	NearbyProviders(ctx context.Context, marketID, vehicleClass string, center geo.Point, radiusKm float64, limit int) ([]availabilityrepositories.ProviderStatus, error)
	// GetOnlineProviders backs the phase B10 ops "live operations" view --
	// unlike NearbyProviders, not scoped to one market/class partition,
	// since an ops view needs the whole picture, not a passenger's own
	// candidate search.
	GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error)
	AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error)
	ReleaseMatchLock(ctx context.Context, providerID string) error
	MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error
	GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error)
}

type ProviderComplianceLookup interface {
	GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error)
}

type VehicleFitLookup interface {
	FirstActiveByProvider(ctx context.Context, providerID string) (providerprofilemodels.Vehicle, error)
}

// CandidateScore is the addendum §6.2 scoring breakdown for one candidate.
// Total is the sum (penalties already negative), and candidates are offered
// highest-Total-first.
//
// Five of the seven dimensions are computed from real data this build
// already has (pickup ETA via routing, vehicle seat fit, the provider's own
// rating column, idle time from provider_operational_state.EffectiveAt, and
// heading-vs-bearing alignment). CancellationRiskPenalty and
// SafetyOrFraudPenalty are always 0 here: neither has supporting
// infrastructure in this build (no cancellation-rate history table, and
// fraud/safety scoring -- RideGuard, addendum §10/§12 -- is explicitly P1+,
// see the build plan's exclusion list). They are declared, not omitted, so a
// later phase that adds the missing data sources has an obvious place to
// wire them in without changing this struct's shape.
type CandidateScore struct {
	ProviderID              string
	PickupETAScore          float64
	VehicleFitScore         float64
	ReliabilityScore        float64
	IdleTimeFairnessScore   float64
	DirectionAlignmentScore float64
	CancellationRiskPenalty float64
	SafetyOrFraudPenalty    float64
	Total                   float64
}

// spawnMatcher starts the search for a driver in the background. Uses
// context.Background(), not the caller's request context: the HTTP request
// that created this booking returns long before matching finishes, and its
// context is cancelled the moment that response is written.
//
// A nil lease (Options.Lease unset -- e.g. a unit test that only exercises
// CreateBooking itself) is a deliberate no-op, not an error: matching is an
// optional capability of BookingService, not a hard requirement of booking
// creation.
func (s *BookingService) spawnMatcher(bookingID string) {
	if s.lease == nil {
		return
	}
	go s.matchBooking(context.Background(), bookingID)
}

// matchBooking is the search loop for one booking. Exactly one goroutine (in
// exactly one service instance) runs this for a given booking at a time,
// enforced by the booking-level matcher lease (addendum §30) -- see
// internal/features/booking/matching's own doc comment for why this is a
// different lock from the per-driver offer lock used below.
// searchDeadlineFor computes when a booking's matcher should give up.
// Anchored to MatchingStartedAt, NEVER CreatedAt (phase B9): for an
// immediate booking CreateBooking sets both to the same instant, but a ride
// booked hours or days ahead of pickup has a CreatedAt far in the past by
// the time ScheduledDispatchWorker promotes it -- reusing CreatedAt here
// would leave the deadline already expired before matchBooking's loop ever
// runs once. Extracted as a pure function (no clock read, no goroutine)
// specifically so this exact calculation -- the bug this comment used to
// live inline next to -- has a deterministic unit test, not just live
// verification.
func searchDeadlineFor(booking bookingmodels.Booking, window time.Duration) time.Time {
	searchStart := booking.CreatedAt
	if booking.MatchingStartedAt != nil {
		searchStart = *booking.MatchingStartedAt
	}
	return searchStart.Add(window)
}

func (s *BookingService) matchBooking(ctx context.Context, bookingID string) {
	acquired, err := s.lease.Acquire(ctx, bookingID, s.matching.InstanceID, s.matching.SearchWindow+30*time.Second)
	if err != nil {
		logging.Error("taxi-matcher", "acquire lease booking=%s: %v", bookingID, err)
		return
	}
	if !acquired {
		// Another instance (or another goroutine within this one, which
		// should never happen but the lease guards it regardless) is already
		// matching this booking.
		return
	}
	defer func() { _ = s.lease.Release(ctx, bookingID, s.matching.InstanceID) }()

	booking, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		logging.Error("taxi-matcher", "read booking=%s: %v", bookingID, err)
		return
	}
	searchDeadline := searchDeadlineFor(booking, s.matching.SearchWindow)
	radius := s.matching.BaseRadiusKm
	expandedOnce := false

	for time.Now().Before(searchDeadline) {
		current, err := s.bookings.GetByID(ctx, bookingID)
		if err != nil {
			logging.Error("taxi-matcher", "read booking=%s: %v", bookingID, err)
			return
		}
		if current.Status != bookingmodels.StatusPendingMatch {
			// Resolved elsewhere (customer cancelled while we were between
			// scans) -- nothing left for this matcher to do.
			return
		}

		candidates := s.findEligibleCandidates(ctx, current, radius)
		if len(candidates) == 0 {
			if !expandedOnce {
				radius = s.matching.ExpandedRadiusKm
				expandedOnce = true
			}
			sleepOrDone(ctx, s.matching.PollInterval)
			continue
		}

		scored := s.scoreCandidates(ctx, current, candidates)

		offeredThisScan := false
		for _, candidate := range scored {
			ttl := s.matching.MatchTimeout + 15*time.Second
			locked, err := s.availability.AcquireMatchLock(ctx, candidate.ProviderID, bookingID, ttl)
			if err != nil || !locked {
				continue // already offered another booking; try the next candidate
			}

			// A fresh, unpaid intent per offer, with the candidate already
			// attached -- see payments.go's own doc comment for why this
			// makes a separate AssignProvider call unnecessary. A wallet
			// failure here (e.g. payment-wallet-service unreachable) makes
			// this candidate unusable for now: release the lock and try the
			// next one rather than stall the whole search on one provider.
			paymentIntentID, err := s.createPaymentIntentForCandidate(ctx, current, candidate.ProviderID)
			if err != nil {
				logging.Error("taxi-matcher", "create payment intent booking=%s provider=%s: %v", bookingID, candidate.ProviderID, err)
				_ = s.availability.ReleaseMatchLock(ctx, candidate.ProviderID)
				continue
			}

			_, err = s.withTransition(ctx, bookingID, candidate.ProviderID, availabilitymodels.StateOffered, "booking_matched", outbox.EventBookingMatched,
				func(exec Executor) (bookingmodels.Booking, error) {
					return s.bookings.MarkMatched(ctx, exec, bookingID, candidate.ProviderID, paymentIntentID)
				})
			if err != nil {
				// The booking was resolved elsewhere between our read above
				// and this attempt (e.g. cancelled). Release the lock we just
				// took and stop -- there is nothing left to offer.
				_ = s.availability.ReleaseMatchLock(ctx, candidate.ProviderID)
				return
			}
			offeredThisScan = true
			// Tell the driver they have an offer. Keyed per recipient, so a
			// booking offered to a second driver still notifies them.
			if s.notifier != nil {
				s.notifier.NotifyDriverMatched(
					ctx, candidate.ProviderID, bookingID, notifyData(current),
				)
			}

			switch s.waitForAcceptance(ctx, bookingID, candidate.ProviderID) {
			case matchOutcomeAccepted:
				return
			case matchOutcomeStop:
				_ = s.availability.ReleaseMatchLock(ctx, candidate.ProviderID)
				return
			case matchOutcomeRequeue:
				_ = s.availability.MarkRejected(ctx, bookingID, candidate.ProviderID, s.matching.SearchWindow)
				_ = s.availability.ReleaseMatchLock(ctx, candidate.ProviderID)
			}
			break // re-scan from the top: radius/eligibility may have changed
		}
		if !offeredThisScan {
			sleepOrDone(ctx, s.matching.PollInterval)
		}
	}

	// Search window exhausted. Guarded on status: if a match landed in the
	// exact instant the deadline fired, this simply finds zero rows and
	// leaves that booking alone rather than clobbering it.
	if _, err := s.withTransition(ctx, bookingID, "", "", "", outbox.EventBookingUnmatched,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.MarkUnmatched(ctx, exec, bookingID, "No driver was found nearby.")
		}); err != nil {
		logging.Notice("taxi-matcher", "booking=%s already resolved by the time the search window elapsed", bookingID)
	} else if s.notifier != nil {
		if unmatched, rerr := s.bookings.GetByID(ctx, bookingID); rerr == nil {
			s.notifier.NotifyCustomerUnmatched(
				ctx, unmatched.CustomerID, bookingID, notifyData(unmatched),
			)
		}
	}
}

type matchOutcome int

const (
	matchOutcomeAccepted matchOutcome = iota
	// matchOutcomeRequeue covers BOTH a genuine timeout and an explicit
	// reject: RejectBooking (HTTP-triggered, a different code path from this
	// goroutine) performs the same guarded reset directly, and this poll
	// loop simply notices the resulting status change on its next tick --
	// no second goroutine or wake signal is needed for a prompt-feeling
	// reject, only a short PollInterval.
	matchOutcomeRequeue
	// matchOutcomeStop covers the booking being resolved some other way
	// (customer cancelled, a hard read error) -- nothing left to do.
	matchOutcomeStop
)

// waitForAcceptance polls the booking's status every PollInterval until
// either it resolves or MatchTimeout elapses. On timeout it attempts the
// guarded reset itself -- the exact "try the reset first, then interpret
// the outcome" decision point verified working in
// driver-hauling-service/booking_repository.go: a reset that finds zero
// rows means an accept landed in the race window between the last poll and
// the deadline firing, and the correct response is to re-read and confirm,
// never to assume either outcome.
func (s *BookingService) waitForAcceptance(ctx context.Context, bookingID, providerID string) matchOutcome {
	deadline := time.Now().Add(s.matching.MatchTimeout)
	for time.Now().Before(deadline) {
		sleepOrDone(ctx, s.matching.PollInterval)
		current, err := s.bookings.GetByID(ctx, bookingID)
		if err != nil {
			return matchOutcomeStop
		}
		switch {
		case current.Status == bookingmodels.StatusAccepted && current.ProviderID != nil && *current.ProviderID == providerID:
			return matchOutcomeAccepted
		case current.Status == bookingmodels.StatusAwaitingAccept:
			continue
		case current.Status == bookingmodels.StatusPendingMatch:
			// Reset by RejectBooking (or, in principle, a recovery worker in
			// a future phase) -- that caller already did its own
			// MarkRejected/ReleaseMatchLock cleanup, so this loop only needs
			// to hand control back to the outer scan.
			return matchOutcomeRequeue
		default:
			return matchOutcomeStop
		}
	}

	_, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAvailable, "offer_timed_out", outbox.EventBookingOfferExpired,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.ResetToMatching(ctx, exec, bookingID, providerID)
		})
	if err == nil {
		return matchOutcomeRequeue
	}
	final, ferr := s.bookings.GetByID(ctx, bookingID)
	if ferr == nil && final.Status == bookingmodels.StatusAccepted && final.ProviderID != nil && *final.ProviderID == providerID {
		return matchOutcomeAccepted
	}
	return matchOutcomeStop
}

// findEligibleCandidates applies every hard filter the addendum §6.2 lists,
// in order, before any candidate is scored: rejected-set membership
// (already offered and declined/timed out for THIS booking), operational
// state (must be AVAILABLE -- Redis presence in the geo index alone does
// not guarantee this; Postgres, the durable authority, is re-checked per
// candidate), and compliance (the same six-dimension CanGoAvailable check
// the online gate itself uses). Vehicle class is already filtered by
// construction: NearbyProviders queries exactly one (market, vehicleClass)
// geo-index partition, per addendum §4.2.
func (s *BookingService) findEligibleCandidates(ctx context.Context, booking bookingmodels.Booking, radiusKm float64) []availabilityrepositories.ProviderStatus {
	if len(booking.Stops) == 0 {
		return nil
	}
	pickup := geo.Point{Lat: booking.Stops[0].Lat, Lng: booking.Stops[0].Lng}

	nearby, err := s.availability.NearbyProviders(ctx, booking.MarketID, booking.VehicleClass, pickup, radiusKm, 50)
	if err != nil {
		logging.Error("taxi-matcher", "nearby providers booking=%s: %v", booking.ID, err)
		return nil
	}
	if len(nearby) == 0 {
		return nil
	}

	rejected, err := s.availability.GetRejectedProviders(ctx, booking.ID)
	if err != nil {
		rejected = map[string]bool{}
	}

	eligible := make([]availabilityrepositories.ProviderStatus, 0, len(nearby))
	for _, candidate := range nearby {
		if rejected[candidate.ProviderID] {
			continue
		}
		state, err := s.operationalState.Get(ctx, candidate.ProviderID)
		if err != nil || !state.Current.Eligible() {
			continue
		}
		provider, err := s.providers.GetByID(ctx, candidate.ProviderID)
		if err != nil {
			continue
		}
		if ok, _ := provider.CanGoAvailable(); !ok {
			continue
		}
		// Parcel opt-in. Checked HERE in Postgres, right after the provider
		// load this loop already pays for, rather than by adding a third
		// dimension to the Redis availability index: that index is
		// partitioned by (market, vehicleClass) and a third key would
		// fragment every existing proximity query for one boolean.
		//
		// Consequence, and it is correct rather than a matcher fault: in a
		// market where few drivers have opted in, a parcel booking will
		// exhaust its search window and land unmatched more often than a
		// passenger ride from the same pickup.
		if booking.IsParcel() && !provider.AcceptsParcels {
			continue
		}
		eligible = append(eligible, candidate)
	}
	return s.applyCapabilityGate(ctx, booking, eligible)
}

// applyCapabilityGate filters candidates through the Section 7 hard gates
// before any scoring. A gate error while enforcing fails closed.
func (s *BookingService) applyCapabilityGate(ctx context.Context, booking bookingmodels.Booking, eligible []availabilityrepositories.ProviderStatus) []availabilityrepositories.ProviderStatus {
	if s.capability == nil || len(eligible) == 0 {
		return eligible
	}
	ids := make([]string, 0, len(eligible))
	for _, candidate := range eligible {
		ids = append(ids, candidate.ProviderID)
	}
	productCode := ""
	if booking.ServiceProductCode != nil {
		productCode = *booking.ServiceProductCode
	}
	// The frozen booking snapshot is authoritative; an older booking without
	// one falls back to the gate's own lookup and legacy class mapping.
	kept, err := s.capability.FilterPassengerCandidates(ctx, booking, productCode, booking.RequirementSnapshot, ids, s.matching.InstanceID)
	if err != nil {
		logging.Error("taxi-matcher", "capability gate booking=%s: %v", booking.ID, err)
		return nil
	}
	allowed := make(map[string]bool, len(kept))
	for _, id := range kept {
		allowed[id] = true
	}
	filtered := eligible[:0]
	for _, candidate := range eligible {
		if allowed[candidate.ProviderID] {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

// scoreCandidates implements addendum §6.2. Pickup ETA is scored across the
// WHOLE surviving candidate set in one Matrix call rather than N sequential
// Route calls, per the addendum's own instruction.
func (s *BookingService) scoreCandidates(ctx context.Context, booking bookingmodels.Booking, candidates []availabilityrepositories.ProviderStatus) []CandidateScore {
	pickup := geo.Point{Lat: booking.Stops[0].Lat, Lng: booking.Stops[0].Lng}

	origins := make([]geo.Point, len(candidates))
	for i, c := range candidates {
		origins[i] = geo.Point{Lat: c.Lat, Lng: c.Lng}
	}
	matrix, matrixErr := s.routes.Matrix(ctx, origins, []geo.Point{pickup})

	scores := make([]CandidateScore, 0, len(candidates))
	for i, c := range candidates {
		scores = append(scores, s.scoreOne(ctx, booking, c, i, matrix, matrixErr, pickup))
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].Total > scores[j].Total })
	return scores
}

func (s *BookingService) scoreOne(
	ctx context.Context, booking bookingmodels.Booking, c availabilityrepositories.ProviderStatus,
	index int, matrix routing.MatrixResult, matrixErr error, pickup geo.Point,
) CandidateScore {
	var etaSeconds int64
	if matrixErr == nil && index < len(matrix.Rows) && len(matrix.Rows[index]) > 0 {
		etaSeconds = matrix.Rows[index][0].DurationS
	} else {
		// Matrix unavailable (routing outage, already absorbed by the
		// haversine fallback decorator at a lower layer -- this branch is
		// belt-and-braces): a crude distance/assumed-speed estimate, good
		// enough to ORDER candidates sensibly, never persisted as a real ETA.
		distanceKm := geo.HaversineKm(geo.Point{Lat: c.Lat, Lng: c.Lng}, pickup)
		etaSeconds = int64(distanceKm / 25.0 * 3600)
	}
	pickupETAScore := -float64(etaSeconds) / 60.0

	vehicleFitScore := 0.0
	if vehicle, err := s.vehicles.FirstActiveByProvider(ctx, c.ProviderID); err == nil {
		vehicleFitScore = -math.Abs(float64(vehicle.Seats - booking.PassengerCount))
	}

	reliabilityScore := 0.0
	if provider, err := s.providers.GetByID(ctx, c.ProviderID); err == nil {
		reliabilityScore = provider.Rating
	}

	idleTimeFairnessScore := 0.0
	if state, err := s.operationalState.Get(ctx, c.ProviderID); err == nil {
		idleMinutes := time.Since(state.EffectiveAt).Minutes()
		const cap = 30.0 // a driver idle for hours must not dominate every score forever
		if idleMinutes > cap {
			idleMinutes = cap
		}
		if idleMinutes > 0 {
			idleTimeFairnessScore = idleMinutes
		}
	}

	directionAlignmentScore := 0.0
	if c.Heading != 0 {
		bearingToPickup := geo.BearingDegrees(geo.Point{Lat: c.Lat, Lng: c.Lng}, pickup)
		diff := angleDifference(c.Heading, bearingToPickup)
		directionAlignmentScore = (180.0 - diff) / 180.0 * 10.0
	}

	// Deliberately 0 -- see CandidateScore's own doc comment.
	cancellationRiskPenalty := 0.0
	safetyOrFraudPenalty := 0.0

	return CandidateScore{
		ProviderID:     c.ProviderID,
		PickupETAScore: pickupETAScore, VehicleFitScore: vehicleFitScore, ReliabilityScore: reliabilityScore,
		IdleTimeFairnessScore: idleTimeFairnessScore, DirectionAlignmentScore: directionAlignmentScore,
		CancellationRiskPenalty: cancellationRiskPenalty, SafetyOrFraudPenalty: safetyOrFraudPenalty,
		Total: pickupETAScore + vehicleFitScore + reliabilityScore + idleTimeFairnessScore + directionAlignmentScore - cancellationRiskPenalty - safetyOrFraudPenalty,
	}
}

func angleDifference(a, b float64) float64 {
	diff := math.Abs(a - b)
	if diff > 180 {
		diff = 360 - diff
	}
	return diff
}

// sleepOrDone sleeps for d unless ctx is cancelled first -- context.Background()
// (what spawnMatcher actually uses) is never cancelled, so this is a plain
// sleep in production; a future caller that passes a real cancellable
// context (e.g. a graceful-shutdown signal) gets an immediate, clean exit
// instead of waiting out the full interval.
func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// AcceptBooking: awaiting_acceptance -> accepted.
//
// Deliberately no operational-state change: MarkMatched already moved the
// provider to OFFERED, and that state's Eligible()==false already covers
// the whole window from "offer sent" through "accepted, not yet begun
// driving" -- there is no separate state in the addendum's ten-state table
// for "accepted but idle", so transitioning to the SAME state again here
// would only be a redundant history row. MarkEnRoutePickup (phase B3) is
// what first moves operational state away from OFFERED.
//
// The per-driver Redis offer lock is released here: the booking is now
// committed, so it no longer needs an exclusive offer reservation.
// AcceptBooking holds payment BEFORE attempting the DB transition, not
// after: the addendum requires a failed charge to block the accept
// entirely, and reading that outcome first is the only way to refuse
// cleanly. This creates one narrow, deliberately-handled race: if the hold
// succeeds but MarkAccepted's guarded CAS then loses (e.g. an internal
// timeout reset this exact offer in the same instant), the customer would
// otherwise have been charged for a ride that just went to someone else --
// so that specific failure path immediately reverses the hold via a direct
// RequestRefund, not the gated refundIfHeld (which reads payment_status
// from Postgres; that column is still 'unpaid' at this point since it is
// only set to 'held' once the transition below actually succeeds).
func (s *BookingService) AcceptBooking(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
	bookingID, providerID = strings.TrimSpace(bookingID), strings.TrimSpace(providerID)

	existing, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	if err := s.holdPayment(ctx, existing); err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable(paymentErrorMessage(err), err)
	}

	booking, err := s.withTransition(ctx, bookingID, providerID, "", "", outbox.EventBookingAccepted,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.MarkAccepted(ctx, exec, bookingID, providerID)
		})
	if err != nil {
		if s.payments != nil && existing.PaymentIntentID != nil {
			// PaymentReference is required by payment-wallet-service's own
			// validation before it ever tries the source-pair fallback --
			// see refundIfHeld's own doc comment for how this was found live.
			if _, rerr := s.payments.RequestRefund(ctx, walletclient.RefundRequest{
				PaymentReference: bookingID,
				SourceService:    PaymentServiceName, SourceReference: bookingID,
				AmountKobo: fareKobo(existing), Currency: "NGN",
				Reason: "accept_failed_after_hold", IdempotencyKey: "taxi-refund-" + bookingID,
			}); rerr != nil {
				logging.Error("taxi-payments", "compensating refund booking=%s: %v", bookingID, rerr)
			}
		}
		return bookingmodels.Booking{}, err
	}

	if s.payments != nil {
		if serr := s.bookings.SetPaymentStatus(ctx, nil, bookingID, bookingmodels.PaymentHeld); serr != nil {
			logging.Error("taxi-payments", "set payment held booking=%s: %v", bookingID, serr)
		} else {
			// The DB write succeeded; reflect it in the object actually
			// returned to the caller, not just in Postgres -- withTransition
			// above returned booking BEFORE this write ran, so its
			// PaymentStatus field is otherwise stale.
			booking.PaymentStatus = bookingmodels.PaymentHeld
		}
	}

	// Availability is nil-able (see Options' own doc comment) -- a service
	// built without it can still accept/reject; releasing a lock is
	// best-effort bookkeeping, never load-bearing for the transition itself.
	if s.availability != nil {
		_ = s.availability.ReleaseMatchLock(ctx, providerID)
	}
	if s.notifier != nil {
		s.notifier.NotifyCustomerAccepted(
			ctx, booking.CustomerID, booking.ID, notifyData(booking),
		)
	}
	return booking, nil
}

// RejectBooking: awaiting_acceptance -> pending_match (the same guarded
// reset as an offer timeout, see ResetToMatching's own doc comment).
//
// This runs on the HTTP request goroutine, a DIFFERENT goroutine from the
// one running matchBooking's search loop for this booking. It does not
// spawn a second matcher -- the booking-level lease would just reject that
// as a no-op anyway -- it relies on the already-running loop's own
// waitForAcceptance poll noticing the status flip within one PollInterval.
func (s *BookingService) RejectBooking(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
	bookingID, providerID = strings.TrimSpace(bookingID), strings.TrimSpace(providerID)
	booking, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAvailable, "offer_rejected", outbox.EventBookingOfferExpired,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.ResetToMatching(ctx, exec, bookingID, providerID)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	// Best-effort bookkeeping: a failure here must not undo the real reset
	// that already committed above -- the driver simply risks being
	// re-offered the same booking sooner than ideal, not a correctness bug.
	// Availability is nil-able, same as AcceptBooking above.
	if s.availability != nil {
		_ = s.availability.MarkRejected(ctx, bookingID, providerID, s.matching.SearchWindow)
		_ = s.availability.ReleaseMatchLock(ctx, providerID)
	}
	return booking, nil
}
