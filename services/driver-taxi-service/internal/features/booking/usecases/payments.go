// Payment integration (phase B4): wallet only -- no cash, no card. Every wallet call
// goes through shared/go/walletclient, which this file depends on via a
// local, narrow PaymentClient interface -- the same "consumer declares what
// it needs" convention as every other cross-package dependency in this
// service (see MarketLookup's own doc comment).
//
// Sequencing, confirmed against payment-wallet-service's actual usecase
// code before writing this (not assumed): CompleteJob's only requirement is
// that the intent ROW currently has provider_id/provider_type set -- it does
// not matter whether that was populated at CreatePaymentIntent time or via
// a later AssignProvider call. Since the matcher always knows the candidate
// provider at the moment it creates the intent, this build passes
// ProviderID/ProviderType directly on CreatePaymentIntent and never calls
// AssignProvider at all -- one fewer HTTP round trip for the exact same
// end state CompleteJob checks.
//
// Money only actually moves at TWO points: PayFromWallet (on accept, the
// customer's wallet is debited into escrow) and CompleteJob (on completion,
// escrow moves to the driver). Every candidate a booking is OFFERED to
// (MarkMatched) gets its own fresh, unpaid intent; an offer that times out
// or is rejected before accept was never paid, so nothing needs refunding --
// the abandoned unpaid intent is simply never touched again.
package bookingusecases

import (
	"context"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/walletclient"
)

// PaymentServiceName is this service's identity in every payment-wallet
// call (source_service on intents, and the first path segment CompleteJob/
// RequestRefund resolve by) and MUST match the ServiceName cmd/main.go signs
// walletclient.Client requests with -- exported so that wiring code uses the
// exact same literal instead of a second copy.
//
// "taxi-service", NOT "driver-taxi-service" -- this does NOT match the
// on-disk directory name (services/driver-taxi-service), it matches this
// module's own go.mod name (module cosmicforge/logistics/services/taxi-
// service) and, more importantly, the caller identity payment-wallet-service
// was ALREADY pre-provisioned to trust before this session
// (PAYMENT_WALLET_SERVICE_SECRETS already contains
// "taxi-service=development-payment-wallet-service-secret", and taxi's own
// .env already sets TAXI_PAYMENT_SECRET to that exact value). Using
// "driver-taxi-service" here -- the naming convention hauling/dispatch
// happen to follow -- would have silently 403'd every call. The platform is
// genuinely inconsistent about this across services (notification-service's
// own trust list uses "driver-taxi-service" instead), so this is not
// derivable from a single convention; it was confirmed by reading
// payment-wallet-service's actual pre-provisioned config before writing this
// constant, not assumed.
const PaymentServiceName = "taxi-service"

// PaymentClient is the slice of walletclient.Client this service calls.
// walletclient.Client (a value type) already implements every method here
// with matching signatures, so it satisfies this interface with no adapter.
type PaymentClient interface {
	CreatePaymentIntent(ctx context.Context, request walletclient.PaymentIntentRequest) (walletclient.PaymentIntent, error)
	PayFromWallet(ctx context.Context, paymentIntentID string, idempotencyKey string) (walletclient.PaymentIntent, error)
	CompleteJobAt(ctx context.Context, sourceService string, sourceReference string, completedAt time.Time) (walletclient.PaymentIntent, error)
	RequestRefund(ctx context.Context, request walletclient.RefundRequest) (walletclient.Refund, error)
}

// fareKobo picks the settlement amount: the reconciled final fare once one
// exists, the original estimate otherwise. Never computed here -- both
// numbers are snapshots from internal/pricing, per that package's own
// package doc comment.
func fareKobo(b bookingmodels.Booking) int64 {
	if b.FareFinalKobo != nil {
		return *b.FareFinalKobo
	}
	if b.FareEstimateKobo != nil {
		return *b.FareEstimateKobo
	}
	return 0
}

// createPaymentIntentForCandidate creates a fresh, unpaid intent for one
// offer, with the candidate provider already attached. Returns (nil, nil)
// only when payments are not configured at all (local dev): every booking is
// wallet-paid now, matching hauling and dispatch.
func (s *BookingService) createPaymentIntentForCandidate(ctx context.Context, booking bookingmodels.Booking, providerID string) (*string, error) {
	if s.payments == nil {
		// The ONE no-charge degrade path left now that cash is gone: no
		// payment-wallet integration configured (local dev without
		// TAXI_PAYMENT_URL/_SECRET). Booking still works and charges nobody,
		// rather than blocking every booking from ever being offered.
		return nil, nil
	}
	intent, err := s.payments.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   PaymentServiceName,
		SourceReference: booking.ID,
		CustomerID:      booking.CustomerID,
		ProviderID:      providerID,
		ProviderType:    "taxi",
		AmountKobo:      fareKobo(booking),
		Currency:        "NGN",
		PaymentMethod:   "wallet",
		IdempotencyKey:  "taxi-intent-" + booking.ID + "-" + providerID,
	})
	if err != nil {
		return nil, err
	}
	return &intent.ID, nil
}

// holdPayment charges the customer's wallet into escrow. Called from
// AcceptBooking; a failure here must block the accept entirely (the
// addendum's own requirement) -- see AcceptBooking's own doc comment for
// how a hold succeeding but the DB transition then failing (a narrow race
// against a concurrent timeout) is unwound.
func (s *BookingService) holdPayment(ctx context.Context, booking bookingmodels.Booking) error {
	if s.payments == nil || booking.PaymentIntentID == nil {
		// No intent was created for this offer (payments not configured) --
		// nothing to hold.
		return nil
	}
	_, err := s.payments.PayFromWallet(ctx, *booking.PaymentIntentID, "taxi-pay-"+booking.ID)
	return err
}

// settlePayment moves held escrow to the driver on completion. Best-effort
// and non-fatal (see MarkCompleted's own doc comment for why): the ride is
// genuinely complete for the passenger/driver either way, and a settlement
// failure with no recovery worker yet (phase B10) is a disclosed, known gap,
// not something this call should mask by refusing to record the ride's own
// completion.
func (s *BookingService) settlePayment(ctx context.Context, booking bookingmodels.Booking) error {
	if s.payments == nil || booking.PaymentIntentID == nil {
		return nil
	}
	// The ride's completion time rides along so the wallet can record when the
	// job actually finished (the referral programme's time rules use it).
	var completedAt time.Time
	if booking.CompletedAt != nil {
		completedAt = *booking.CompletedAt
	}
	_, err := s.payments.CompleteJobAt(ctx, PaymentServiceName, booking.ID, completedAt)
	return err
}

// refundIfHeld reverses an escrowed charge on cancellation. A no-op when
// payments are not configured, and for a booking whose payment never
// actually reached 'held' (cancelling before accept -- the only point money
// moves at all -- never charged anything).
func (s *BookingService) refundIfHeld(ctx context.Context, booking bookingmodels.Booking, reason string) error {
	if s.payments == nil || booking.PaymentStatus != bookingmodels.PaymentHeld {
		return nil
	}
	_, err := s.payments.RequestRefund(ctx, walletclient.RefundRequest{
		// PaymentReference is a REQUIRED field payment-wallet-service
		// validates BEFORE it ever tries the source-pair fallback below --
		// confirmed by reading RequestRefund's actual usecase code (a plain
		// wallet booking id here will never match a real intent reference,
		// which is exactly what triggers that NotFound-then-fallback path
		// to run). Leaving this empty is not "no reference given, please use
		// the source pair" -- it is an immediate validation_failed with no
		// fallback attempted at all. Found live, not assumed: an earlier
		// version of this call omitted PaymentReference entirely and every
		// refund failed with "Check your refund request."
		PaymentReference: booking.ID,
		SourceService:    PaymentServiceName,
		SourceReference:  booking.ID,
		AmountKobo:       fareKobo(booking),
		Currency:         "NGN",
		Reason:           reason,
		IdempotencyKey:   "taxi-refund-" + booking.ID,
	})
	return err
}

// paymentErrorMessage maps a wallet-client failure (usually a bubbled-up
// apperrors.Error from the internal HTTP call, e.g. insufficient balance)
// to passenger-facing wording, falling back to a generic message for a
// transport-level failure that carries no useful detail to show.
func paymentErrorMessage(err error) string {
	if appErr := apperrors.From(err); appErr != nil && strings.TrimSpace(appErr.Message) != "" {
		return appErr.Message
	}
	return "Payment could not be processed. Try again."
}
