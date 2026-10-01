package bookingusecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/services/taxi-service/internal/pricing"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

// Executor is bookingrepositories.Executor (itself an alias for
// dbtx.Executor). Re-exported under a short local name purely for
// readability in this file's many function signatures below.
type Executor = bookingrepositories.Executor

// OperationalStateStore is the slice of provider_availability's operational
// state machine the lifecycle orchestrator needs. Declared locally rather
// than importing that feature's repository package for an interface type --
// this service's existing convention for cross-feature dependencies (see
// MarketLookup in booking_service.go) -- but Transition's exec parameter is
// typed as bookingrepositories.Executor (an alias for dbtx.Executor, the
// SAME underlying type provider_availability's own Executor is also an
// alias for). That identity, not mere structural similarity, is what lets
// *PostgresOperationalStateRepository satisfy this interface and lets a
// single pgx.Tx be shared across both repositories -- see internal/dbtx's
// doc comment for why two independently-declared interfaces would not work
// here even if shaped identically.
type OperationalStateStore interface {
	Get(ctx context.Context, providerID string) (availabilitymodels.State, error)
	Transition(ctx context.Context, exec Executor, providerID string, to availabilitymodels.OperationalState, reason string, bookingID *string, expectedVersion int64) (availabilitymodels.State, error)
}

// transitionPayload is the outbox event body for every booking transition
// below. Kept small (identifiers + the facts that gate downstream
// consumers), matching CreateBooking's own bookingCreatedPayload -- a
// subscriber that needs more can always read the booking back by ID.
type transitionPayload struct {
	BookingID  string    `json:"booking_id"`
	Status     string    `json:"status"`
	ProviderID *string   `json:"provider_id,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func outboxPayloadFor(b bookingmodels.Booking) ([]byte, error) {
	return json.Marshal(transitionPayload{BookingID: b.ID, Status: string(b.Status), ProviderID: b.ProviderID, UpdatedAt: b.UpdatedAt})
}

// actorForTransition derives the audit-trail actor for one guarded
// transition, from data withTransition already has -- NOT a new parameter
// threaded through all 11 call sites. This is a deliberate, disclosed
// precision tradeoff: eventType alone is enough for every call site in this
// file except EventBookingOfferExpired (shared by a provider's explicit
// reject and the matcher's own timeout) and EventBookingCancelled (shared by
// the customer and provider cancel paths), both of which the existing
// `reason` string already distinguishes ("offer_rejected" vs
// "offer_timed_out"; "booking_cancelled_by_customer" vs
// "..._by_provider") -- so no new signal needs to be invented, only read.
func actorForTransition(eventType, reason, providerID, customerID string) (actorType, actorID string) {
	switch eventType {
	case outbox.EventBookingMatched, outbox.EventBookingUnmatched:
		return "system", "matcher"
	case outbox.EventBookingOfferExpired:
		if strings.Contains(reason, "rejected") {
			return "provider", providerID
		}
		return "system", "matcher"
	case outbox.EventBookingCancelled:
		if strings.Contains(reason, "customer") {
			return "customer", customerID
		}
		return "provider", providerID
	default:
		// Accepted/EnRoutePickup/Arrived/Started/Completed: every remaining
		// event in this file is always provider-initiated.
		if providerID != "" {
			return "provider", providerID
		}
		return "system", "matcher"
	}
}

// withTransition opens one transaction, runs bookingStep (the guarded
// booking status change), optionally transitions the given provider's
// operational state, and always writes an outbox event -- all inside the
// SAME transaction. A crash or a rejected CAS can never leave the booking's
// status, the provider's operational state, and the outbox disagreeing
// about what just happened, per the addendum's explicit requirement for the
// operational-state coupling; the outbox write extends that same guarantee
// to every downstream event consumer.
//
// Pass an empty providerID to skip the operational-state step entirely
// (used when no provider is assigned yet, e.g. a pre-match customer cancel)
// -- the outbox event still gets written either way.
func (s *BookingService) withTransition(
	ctx context.Context,
	bookingID, providerID string,
	newState availabilitymodels.OperationalState,
	reason, eventType string,
	bookingStep func(exec Executor) (bookingmodels.Booking, error),
) (bookingmodels.Booking, error) {
	// Operational state is touched only when BOTH a real provider AND a real
	// target state are given -- neither alone is a safe skip signal:
	//   - CancelByCustomer's pre-match branch has providerID=="" (no
	//     provider assigned yet) but always passes a real newState, and must
	//     skip.
	//   - AcceptBooking has a real providerID (needed for MarkAccepted's own
	//     SQL guard) but deliberately passes newState=="" to skip. Gating on
	//     providerID alone here was a real bug caught during live
	//     verification: it let this call through and wrote a literal empty
	//     string into provider_operational_state.state, corrupting it. See
	//     TestAcceptBooking_DoesNotCorruptOperationalState.
	touchesOperationalState := newState != "" && providerID != ""

	var currentVersion int64
	if touchesOperationalState {
		current, err := s.operationalState.Get(ctx, providerID)
		if err != nil {
			return bookingmodels.Booking{}, apperrors.Unavailable("This ride could not be updated. Try again.", err)
		}
		currentVersion = current.Version
	}

	tx, err := s.bookings.BeginTx(ctx)
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("This ride could not be updated. Try again.", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	booking, err := bookingStep(tx)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	if touchesOperationalState {
		bookingIDCopy := bookingID
		if _, err := s.operationalState.Transition(ctx, tx, providerID, newState, reason, &bookingIDCopy, currentVersion); err != nil {
			return bookingmodels.Booking{}, err
		}
	}

	payload, err := outboxPayloadFor(booking)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if err := outbox.Write(ctx, tx, "booking", booking.ID, eventType, payload); err != nil {
		return bookingmodels.Booking{}, err
	}

	actorType, actorID := actorForTransition(eventType, reason, providerID, booking.CustomerID)
	if err := s.bookings.RecordEvent(ctx, tx, bookingmodels.BookingEvent{
		BookingID: booking.ID, EventType: eventType, ActorType: actorType, ActorID: actorID,
	}); err != nil {
		return bookingmodels.Booking{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("This ride could not be updated. Try again.", err)
	}
	return booking, nil
}

// MarkEnRoutePickup: accepted -> en_route_pickup. The provider's operational
// state moves EN_ROUTE_PICKUP in the same transaction.
// The notifier adapters below exist so notifyCustomer can be handed a plain
// function value. Taking a method value directly off s.notifier would
// dereference it before the nil check runs, which panics whenever the
// notification integration is unconfigured (local dev).
func (s *BookingService) notifierEnRoutePickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerEnRoutePickup(ctx, customerID, bookingID, data)
}

func (s *BookingService) notifierArrivedPickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerArrivedPickup(ctx, customerID, bookingID, data)
}

func (s *BookingService) notifierStarted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerStarted(ctx, customerID, bookingID, data)
}

func (s *BookingService) notifierCompleted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerCompleted(ctx, customerID, bookingID, data)
}

func (s *BookingService) notifierCancelledByProvider(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerCancelledByProvider(ctx, customerID, bookingID, data)
}

func (s *BookingService) notifierDeliveryCodeReady(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	s.notifier.NotifyCustomerDeliveryCodeReady(ctx, customerID, bookingID, data)
}

// notifyCustomer fires one passenger-facing notification after a transition
// has already committed. Fire-and-forget by contract: the ride has happened
// whether or not the message lands, so this never affects the return value.
func (s *BookingService) notifyCustomer(
	ctx context.Context,
	booking bookingmodels.Booking,
	send func(context.Context, string, string, map[string]interface{}),
) {
	if s.notifier == nil || booking.CustomerID == "" {
		return
	}
	send(ctx, booking.CustomerID, booking.ID, notifyData(booking))
}

func (s *BookingService) MarkEnRoutePickup(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
	bookingID, providerID = strings.TrimSpace(bookingID), strings.TrimSpace(providerID)
	booking, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateEnRoutePickup, "booking_en_route_pickup", outbox.EventBookingEnRoutePickup,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.MarkEnRoutePickup(ctx, exec, bookingID, providerID)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	s.notifyCustomer(ctx, booking, s.notifierEnRoutePickup)
	return booking, nil
}

// MarkArrivedAtPickup: en_route_pickup -> arrived_at_pickup.
// MarkArrivedAtPickup also generates the pickup verification code (phase
// B5) in the SAME transaction as the status change, when a
// PickupVerifications repository is configured -- see generatePickupCode's
// own doc comment for why a nil repository is a safe skip, not an error.
func (s *BookingService) MarkArrivedAtPickup(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
	bookingID, providerID = strings.TrimSpace(bookingID), strings.TrimSpace(providerID)
	booking, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAtPickup, "booking_arrived_at_pickup", outbox.EventBookingArrived,
		func(exec Executor) (bookingmodels.Booking, error) {
			b, err := s.bookings.MarkArrivedAtPickup(ctx, exec, bookingID, providerID)
			if err != nil {
				return bookingmodels.Booking{}, err
			}
			if err := s.generatePickupCode(ctx, exec, bookingID); err != nil {
				return bookingmodels.Booking{}, err
			}
			return b, nil
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	// The pickup credential was generated in the same transaction, so this
	// doubles as "your code is ready".
	s.notifyCustomer(ctx, booking, s.notifierArrivedPickup)
	return booking, nil
}

// MarkInProgress is the "start trip" action: arrived_at_pickup -> in_progress.
//
// B3 STUB, matching the build plan's explicit instruction for this phase:
// this does not yet require pickup verification. Phase B5 will insert a
// pickup_verification_pending hop before this transition and refuse to start
// unless verification has succeeded; until then it behaves as if
// verification always passes.
func (s *BookingService) MarkInProgress(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
	bookingID, providerID = strings.TrimSpace(bookingID), strings.TrimSpace(providerID)
	booking, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateOnTrip, "booking_started", outbox.EventBookingStarted,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.MarkInProgress(ctx, exec, bookingID, providerID)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	s.notifyCustomer(ctx, booking, s.notifierStarted)
	// A parcel's drop-off window just opened, so the sender can now fetch the
	// receiver's code.
	if booking.IsParcel() {
		s.notifyCustomer(ctx, booking, s.notifierDeliveryCodeReady)
	}
	return booking, nil
}

type CompleteBookingInput struct {
	BookingID         string
	ProviderID        string
	ActualDistanceKm  float64
	ActualDurationMin int
}

// MarkCompleted: in_progress -> completed. Reconciles the final fare from
// PricingClient (never recomputed locally -- see internal/pricing's own
// package doc comment) using the booking's original quote reference and
// vehicle class against what the driver actually reported for this trip.
// The provider's operational state returns to AVAILABLE: the addendum's
// ten-state machine has no separate post-trip cooldown state, so a driver
// who just completed a ride becomes immediately eligible for a new offer,
// matching real ride-hailing behavior.
//
// Settling payment (the wallet call itself) is phase B4's job -- this method
// only records the reconciled fare and status; payment_status is untouched.
func (s *BookingService) MarkCompleted(ctx context.Context, input CompleteBookingInput) (bookingmodels.Booking, error) {
	bookingID, providerID := strings.TrimSpace(input.BookingID), strings.TrimSpace(input.ProviderID)
	if input.ActualDistanceKm < 0 || input.ActualDurationMin < 0 {
		return bookingmodels.Booking{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "actual_distance_km", Message: "Trip distance and duration must not be negative."},
		})
	}

	// Read outside the transaction: PricingQuoteReference/VehicleClass never
	// change after creation, so a moment-old read cannot go stale in a way
	// that matters -- the guarded transition below is still the sole
	// authority over whether this booking is actually completable right now.
	existing, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	// Phase B6: prefer the server-computed distance from the validated
	// telemetry trail over whatever the driver app reports -- addendum §8's
	// own requirement, so a trip's settled fare cannot be inflated by a
	// client simply claiming a longer distance than was actually driven.
	// Falls back to the client-submitted figure when telemetry is not
	// configured (local dev without a telemetry repository) or the trail
	// has fewer than two clean points (too early in rollout, or every point
	// this trip reported was flagged) -- see resolveActualDistanceKm.
	actualDistanceKm := s.resolveActualDistanceKm(ctx, bookingID, input.ActualDistanceKm)

	finalFare, err := s.pricing.Reconcile(ctx, pricing.ReconcileRequest{
		QuoteReference: existing.PricingQuoteReference, VehicleClass: existing.VehicleClass,
		ActualDistanceKm: actualDistanceKm, ActualDurationMin: input.ActualDurationMin,
	})
	if err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("The final fare could not be calculated. Try again.", err)
	}

	completed, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAvailable, "booking_completed", outbox.EventBookingCompleted,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.MarkCompleted(ctx, exec, bookingID, providerID, actualDistanceKm, finalFare.FareKobo)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	// Settlement is best-effort and non-fatal here -- see settlePayment's own
	// doc comment for why a failure must not undo the ride's own completion,
	// which is already true for the passenger and driver regardless of
	// whether the money has moved yet.
	if err := s.settlePayment(ctx, completed); err != nil {
		logging.Error("taxi-payments", "settle booking=%s: %v", bookingID, err)
	} else if s.payments != nil {
		if serr := s.bookings.SetPaymentStatus(ctx, nil, bookingID, bookingmodels.PaymentPaid); serr != nil {
			logging.Error("taxi-payments", "set payment paid booking=%s: %v", bookingID, serr)
		} else {
			completed.PaymentStatus = bookingmodels.PaymentPaid
		}
	}
	s.notifyCustomer(ctx, completed, s.notifierCompleted)
	return completed, nil
}

// CancelByCustomer cancels from any of bookingmodels.ActiveStatuses except
// StatusInProgress (see the repository's cancellableByCustomer, the actual
// enforced guard). If a provider had already been assigned, their
// operational state releases back to AVAILABLE in the same transaction;
// otherwise (a pre-match cancel) no provider state exists to touch, but the
// outbox event is still written.
func (s *BookingService) CancelByCustomer(ctx context.Context, bookingID, customerID, reason string) (bookingmodels.Booking, error) {
	bookingID, customerID, reason = strings.TrimSpace(bookingID), strings.TrimSpace(customerID), strings.TrimSpace(reason)

	existing, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	providerID := ""
	if existing.ProviderID != nil {
		providerID = *existing.ProviderID
	}
	cancelled, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAvailable, "booking_cancelled_by_customer", outbox.EventBookingCancelled,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.CancelByCustomer(ctx, exec, bookingID, customerID, reason)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	// existing (read before the transition) is what refundIfHeld gates on:
	// money only ever moves at accept (PayFromWallet), so payment_status was
	// already 'held' or not BEFORE this cancel -- the cancel itself never
	// changes that fact, it only decides whether to reverse it.
	if rerr := s.refundIfHeld(ctx, existing, "cancelled_by_customer: "+reason); rerr != nil {
		logging.Error("taxi-payments", "refund booking=%s: %v", bookingID, rerr)
	} else if existing.PaymentStatus == bookingmodels.PaymentHeld {
		if serr := s.bookings.SetPaymentStatus(ctx, nil, bookingID, bookingmodels.PaymentRefunded); serr != nil {
			logging.Error("taxi-payments", "set payment refunded booking=%s: %v", bookingID, serr)
		} else {
			cancelled.PaymentStatus = bookingmodels.PaymentRefunded
		}
	}
	// Tell the driver their job is gone; they may already be en route.
	if s.notifier != nil && cancelled.ProviderID != nil && *cancelled.ProviderID != "" {
		s.notifier.NotifyDriverCancelled(
			ctx, *cancelled.ProviderID, cancelled.ID, notifyData(cancelled),
		)
	}
	return cancelled, nil
}

// CancelByProvider cancels from any status in the repository's
// cancellableByProvider set (accepted through arrived_at_pickup -- once a
// trip is in progress, provider-initiated cancellation is no longer a
// simple status flip, the same boundary CancelByCustomer draws).
func (s *BookingService) CancelByProvider(ctx context.Context, bookingID, providerID, reason string) (bookingmodels.Booking, error) {
	bookingID, providerID, reason = strings.TrimSpace(bookingID), strings.TrimSpace(providerID), strings.TrimSpace(reason)

	existing, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}

	cancelled, err := s.withTransition(ctx, bookingID, providerID, availabilitymodels.StateAvailable, "booking_cancelled_by_provider", outbox.EventBookingCancelled,
		func(exec Executor) (bookingmodels.Booking, error) {
			return s.bookings.CancelByProvider(ctx, exec, bookingID, providerID, reason)
		})
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if rerr := s.refundIfHeld(ctx, existing, "cancelled_by_provider: "+reason); rerr != nil {
		logging.Error("taxi-payments", "refund booking=%s: %v", bookingID, rerr)
	} else if existing.PaymentStatus == bookingmodels.PaymentHeld {
		if serr := s.bookings.SetPaymentStatus(ctx, nil, bookingID, bookingmodels.PaymentRefunded); serr != nil {
			logging.Error("taxi-payments", "set payment refunded booking=%s: %v", bookingID, serr)
		} else {
			cancelled.PaymentStatus = bookingmodels.PaymentRefunded
		}
	}
	s.notifyCustomer(ctx, cancelled, s.notifierCancelledByProvider)
	return cancelled, nil
}

func (s *BookingService) GetByIDForProvider(ctx context.Context, id, providerID string) (bookingmodels.Booking, error) {
	return s.bookings.GetByIDForProvider(ctx, strings.TrimSpace(id), strings.TrimSpace(providerID))
}

func (s *BookingService) ListActiveByProvider(ctx context.Context, providerID string) ([]bookingmodels.Booking, error) {
	return s.bookings.ListActiveByProvider(ctx, strings.TrimSpace(providerID))
}
