package bookingusecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

// WaitingStore persists waiting-time rates and each booking's waiting.
type WaitingStore interface {
	LatestPolicy(ctx context.Context) (bookingmodels.WaitingPolicy, bool, error)
	InsertPolicy(ctx context.Context, p bookingmodels.WaitingPolicy) error
	MarkArrived(ctx context.Context, bookingID, stop string, at time.Time, policy bookingmodels.WaitingPolicy) error
	MarkLeft(ctx context.Context, bookingID, stop string, at time.Time) error
	Get(ctx context.Context, bookingID string) (bookingmodels.BookingWaiting, error)
	SettleFee(ctx context.Context, bookingID string, feeKobo int64, status, reference string) (bool, error)
}

// WaitingChargeInput is a waiting fee to take from the customer's wallet and
// release to the driver.
type WaitingChargeInput struct {
	BookingID  string
	CustomerID string
	ProviderID string
	AmountKobo int64
}

// WaitingCharger takes a waiting fee from the customer's wallet as its own
// payment and releases it to the driver (the wallet deducts the usual
// commission). It answers PAID or OWED; an error means the outcome is unknown.
// Optional on a PaymentClient.
type WaitingCharger interface {
	ChargeWaiting(ctx context.Context, in WaitingChargeInput) (status, reference string, err error)
}

// WithWaiting turns on waiting time. defaults are the rates used until an
// admin sets their own.
func (s *BookingService) WithWaiting(store WaitingStore, defaults bookingmodels.WaitingPolicy) *BookingService {
	s.waiting = store
	s.waitingDefaults = defaults
	return s
}

// CurrentWaitingPolicy is the rates in force: the latest an admin set, else
// the configured defaults.
func (s *BookingService) CurrentWaitingPolicy(ctx context.Context) (bookingmodels.WaitingPolicy, error) {
	if s.waiting == nil {
		return bookingmodels.WaitingPolicy{}, apperrors.Unavailable("Waiting time is not enabled.", nil)
	}
	policy, ok, err := s.waiting.LatestPolicy(ctx)
	if err != nil {
		return bookingmodels.WaitingPolicy{}, err
	}
	if ok {
		return policy, nil
	}
	return s.waitingDefaults, nil
}

// SetWaitingPolicyInput is an admin's change to the rates.
type SetWaitingPolicyInput struct {
	FreeMinutes  int
	BlockMinutes int
	BlockFeeKobo int64
	PerformedBy  string
	Reason       string
}

// SetWaitingPolicy records new rates. They apply to bookings whose driver
// first arrives after this moment; bookings already being timed keep theirs.
func (s *BookingService) SetWaitingPolicy(ctx context.Context, in SetWaitingPolicyInput) (bookingmodels.WaitingPolicy, error) {
	if s.waiting == nil {
		return bookingmodels.WaitingPolicy{}, apperrors.Unavailable("Waiting time is not enabled.", nil)
	}
	policy := bookingmodels.WaitingPolicy{
		ID: uuid.NewString(), FreeMinutes: in.FreeMinutes, BlockMinutes: in.BlockMinutes, BlockFeeKobo: in.BlockFeeKobo,
		CreatedBy: strings.TrimSpace(in.PerformedBy), Reason: strings.TrimSpace(in.Reason), CreatedAt: time.Now().UTC(),
	}
	var fields []apperrors.FieldViolation
	if err := policy.Validate(); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "policy", Message: err.Error()})
	}
	if policy.CreatedBy == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "performed_by", Message: "Name the person making this change."})
	}
	if policy.Reason == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "reason", Message: "Give a reason for this change."})
	}
	if len(fields) > 0 {
		return bookingmodels.WaitingPolicy{}, apperrors.Validation("Check the waiting-time rates.", fields)
	}
	if err := s.waiting.InsertPolicy(ctx, policy); err != nil {
		return bookingmodels.WaitingPolicy{}, err
	}
	return policy, nil
}

// waitingArrived starts the clock at a stop. It never fails the trip step it
// is attached to: waiting time is billing evidence, not a gate.
func (s *BookingService) waitingArrived(ctx context.Context, bookingID, stop string) {
	if s.waiting == nil {
		return
	}
	policy, err := s.CurrentWaitingPolicy(ctx)
	if err == nil {
		err = s.waiting.MarkArrived(ctx, bookingID, stop, time.Now().UTC(), policy)
	}
	if err != nil {
		logging.Error("waiting", "arrive booking=%s stop=%s: %v", bookingID, stop, err)
	}
}

// waitingLeft stops the clock at a stop.
func (s *BookingService) waitingLeft(ctx context.Context, bookingID, stop string) {
	if s.waiting == nil {
		return
	}
	if err := s.waiting.MarkLeft(ctx, bookingID, stop, time.Now().UTC()); err != nil {
		logging.Error("waiting", "leave booking=%s stop=%s: %v", bookingID, stop, err)
	}
}

// MarkArrivedAtDropoff starts the drop-off clock. It changes no booking status:
// the trip is still en route to delivery until the driver hands over.
func (s *BookingService) MarkArrivedAtDropoff(ctx context.Context, id, providerID string) (bookingmodels.WaitingView, error) {
	if s.waiting == nil {
		return bookingmodels.WaitingView{}, apperrors.Unavailable("Waiting time is not enabled.", nil)
	}
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.WaitingView{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.WaitingView{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	if b.Status != bookingmodels.StatusEnRouteDelivery && b.Status != bookingmodels.StatusPickedUp {
		return bookingmodels.WaitingView{}, apperrors.Conflict("You can only arrive at the drop-off with the cargo on board.", nil)
	}
	s.waitingArrived(ctx, id, bookingmodels.WaitingStopDropoff)
	_ = s.addEvent(ctx, id, "driver_arrived_dropoff", bookingmodels.ActorProvider, providerID)
	return s.waitingView(ctx, id)
}

// ProviderWaiting returns a booking's waiting for its driver.
func (s *BookingService) ProviderWaiting(ctx context.Context, id, providerID string) (bookingmodels.WaitingView, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.WaitingView{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.WaitingView{}, apperrors.Forbidden("You cannot view this booking.", nil)
	}
	return s.waitingView(ctx, id)
}

// CustomerWaiting returns a booking's waiting for the customer who booked it.
func (s *BookingService) CustomerWaiting(ctx context.Context, id, customerID string) (bookingmodels.WaitingView, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.WaitingView{}, err
	}
	if b.CustomerID != customerID {
		return bookingmodels.WaitingView{}, apperrors.NotFound("Booking not found.", nil)
	}
	return s.waitingView(ctx, id)
}

// waitingView is the booking's waiting as of now; before the driver arrives
// anywhere it shows the rates that will apply, so the apps can explain them.
func (s *BookingService) waitingView(ctx context.Context, id string) (bookingmodels.WaitingView, error) {
	if s.waiting == nil {
		return bookingmodels.WaitingView{}, apperrors.Unavailable("Waiting time is not enabled.", nil)
	}
	record, err := s.waiting.Get(ctx, id)
	if errors.Is(err, bookingrepo.ErrNoWaiting) {
		policy, policyErr := s.CurrentWaitingPolicy(ctx)
		if policyErr != nil {
			return bookingmodels.WaitingView{}, policyErr
		}
		record = bookingmodels.BookingWaiting{BookingID: id, Policy: policy, FeeStatus: bookingmodels.WaitingFeePending}
	} else if err != nil {
		return bookingmodels.WaitingView{}, err
	}
	return record.View(time.Now().UTC()), nil
}

// settleWaiting decides what happens to the waiting fee once the job is done.
// paidBy says how this job's money moves: "wallet", "cash", or "" for a job
// settled outside the customer's wallet (fleet, vendor delivery), which is
// never charged waiting time.
func (s *BookingService) settleWaiting(ctx context.Context, b bookingmodels.Booking, paidBy string) {
	if s.waiting == nil {
		return
	}
	record, err := s.waiting.Get(ctx, b.ID)
	if errors.Is(err, bookingrepo.ErrNoWaiting) {
		return
	}
	if err != nil {
		logging.Error("waiting", "settle booking=%s: %v", b.ID, err)
		return
	}
	if record.FeeStatus != bookingmodels.WaitingFeePending {
		return // already settled; a retry must not charge twice
	}
	fee := record.FinalFeeKobo()
	status, reference := "", ""
	switch {
	case paidBy == "":
		status = bookingmodels.WaitingFeeNotApplicable
	case fee == 0:
		status = bookingmodels.WaitingFeeNone
	case paidBy == "cash":
		status = bookingmodels.WaitingFeeCash
	default:
		charger, ok := s.payments.(WaitingCharger)
		if !ok || b.ProviderID == nil {
			return // no wallet in this deployment; leave it pending
		}
		status, reference, err = charger.ChargeWaiting(ctx, WaitingChargeInput{
			BookingID: b.ID, CustomerID: b.CustomerID, ProviderID: *b.ProviderID, AmountKobo: fee,
		})
		if err != nil {
			logging.Error("waiting", "charge booking=%s fee=%d: outcome unknown: %v", b.ID, fee, err)
			return // stays pending for reconciliation; never guessed
		}
	}
	changed, err := s.waiting.SettleFee(ctx, b.ID, fee, status, reference)
	if err != nil {
		logging.Error("waiting", "record fee booking=%s: %v", b.ID, err)
		return
	}
	if changed && fee > 0 {
		_ = s.addEvent(ctx, b.ID, "waiting_fee_"+strings.ToLower(status), bookingmodels.ActorSystem, "")
	}
}
