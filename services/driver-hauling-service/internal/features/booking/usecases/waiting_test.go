package bookingusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type fakeWaitingStore struct {
	policies []bookingmodels.WaitingPolicy
	records  map[string]bookingmodels.BookingWaiting
}

func newFakeWaitingStore() *fakeWaitingStore {
	return &fakeWaitingStore{records: map[string]bookingmodels.BookingWaiting{}}
}

func (f *fakeWaitingStore) LatestPolicy(context.Context) (bookingmodels.WaitingPolicy, bool, error) {
	if len(f.policies) == 0 {
		return bookingmodels.WaitingPolicy{}, false, nil
	}
	return f.policies[len(f.policies)-1], true, nil
}

func (f *fakeWaitingStore) InsertPolicy(_ context.Context, p bookingmodels.WaitingPolicy) error {
	f.policies = append(f.policies, p)
	return nil
}

func (f *fakeWaitingStore) MarkArrived(_ context.Context, id, stop string, at time.Time, p bookingmodels.WaitingPolicy) error {
	w, ok := f.records[id]
	if !ok {
		w = bookingmodels.BookingWaiting{BookingID: id, Policy: p, FeeStatus: bookingmodels.WaitingFeePending}
	}
	t := at
	if stop == bookingmodels.WaitingStopPickup && w.PickupArrivedAt == nil {
		w.PickupArrivedAt = &t
	}
	if stop == bookingmodels.WaitingStopDropoff && w.DropoffArrivedAt == nil {
		w.DropoffArrivedAt = &t
	}
	f.records[id] = w
	return nil
}

func (f *fakeWaitingStore) MarkLeft(_ context.Context, id, stop string, at time.Time) error {
	w, ok := f.records[id]
	if !ok {
		return nil
	}
	t := at
	if stop == bookingmodels.WaitingStopPickup && w.PickupArrivedAt != nil && w.PickupLeftAt == nil {
		w.PickupLeftAt = &t
	}
	if stop == bookingmodels.WaitingStopDropoff && w.DropoffArrivedAt != nil && w.DropoffLeftAt == nil {
		w.DropoffLeftAt = &t
	}
	f.records[id] = w
	return nil
}

func (f *fakeWaitingStore) Get(_ context.Context, id string) (bookingmodels.BookingWaiting, error) {
	w, ok := f.records[id]
	if !ok {
		return bookingmodels.BookingWaiting{}, bookingrepo.ErrNoWaiting
	}
	return w, nil
}

func (f *fakeWaitingStore) SettleFee(_ context.Context, id string, fee int64, status, ref string) (bool, error) {
	w, ok := f.records[id]
	if !ok || w.FeeStatus != bookingmodels.WaitingFeePending {
		return false, nil
	}
	w.FeeKobo, w.FeeStatus, w.FeeReference = fee, status, ref
	f.records[id] = w
	return true, nil
}

// chargingPayments is a payment client that can also charge waiting fees.
type chargingPayments struct {
	unknownPaymentClient
	status string
	err    error
	calls  []WaitingChargeInput
}

func (c *chargingPayments) ChargeWaiting(_ context.Context, in WaitingChargeInput) (string, string, error) {
	c.calls = append(c.calls, in)
	return c.status, "ref-" + in.BookingID, c.err
}

var testWaitingDefaults = bookingmodels.WaitingPolicy{FreeMinutes: 60, BlockMinutes: 30, BlockFeeKobo: 200000}

// seedWait records a closed pickup wait of the given length.
func seedWait(store *fakeWaitingStore, id string, waited time.Duration) {
	arrived := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	left := arrived.Add(waited)
	store.records[id] = bookingmodels.BookingWaiting{
		BookingID: id, Policy: testWaitingDefaults, FeeStatus: bookingmodels.WaitingFeePending,
		PickupArrivedAt: &arrived, PickupLeftAt: &left,
	}
}

func waitingService(repo *fakeBookingRepo, payments PaymentClient, store *fakeWaitingStore) *BookingService {
	return NewBookingService(Options{Bookings: repo, Payments: payments}).WithWaiting(store, testWaitingDefaults)
}

func TestWaitingPolicyFallsBackToDefaultsThenAdminRates(t *testing.T) {
	store := newFakeWaitingStore()
	svc := waitingService(&fakeBookingRepo{}, nil, store)
	ctx := context.Background()

	got, err := svc.CurrentWaitingPolicy(ctx)
	if err != nil || got.BlockFeeKobo != 200000 {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	if _, err := svc.SetWaitingPolicy(ctx, SetWaitingPolicyInput{FreeMinutes: 45, BlockMinutes: 15, BlockFeeKobo: 100000}); err == nil {
		t.Fatal("a change without a name and reason was accepted")
	} else {
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) || appErr.Code != "validation_failed" {
			t.Fatalf("want validation error, got %v", err)
		}
	}
	if _, err := svc.SetWaitingPolicy(ctx, SetWaitingPolicyInput{
		FreeMinutes: 45, BlockMinutes: 15, BlockFeeKobo: 100000, PerformedBy: "ops@karryam.com", Reason: "Lagos rates",
	}); err != nil {
		t.Fatalf("SetWaitingPolicy: %v", err)
	}
	if got, _ := svc.CurrentWaitingPolicy(ctx); got.FreeMinutes != 45 || got.BlockFeeKobo != 100000 {
		t.Fatalf("admin rates not in force: %+v", got)
	}
}

func TestArrivedAtDropoffChecksDriverAndStatus(t *testing.T) {
	provider := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: &provider, Status: bookingmodels.StatusAccepted,
	}}
	store := newFakeWaitingStore()
	svc := waitingService(repo, nil, store)
	ctx := context.Background()

	if _, err := svc.MarkArrivedAtDropoff(ctx, "b1", "someone-else"); err == nil {
		t.Fatal("another driver started the drop-off clock")
	}
	if _, err := svc.MarkArrivedAtDropoff(ctx, "b1", provider); err == nil {
		t.Fatal("drop-off clock started before the cargo was picked up")
	}
	repo.booking.Status = bookingmodels.StatusEnRouteDelivery
	view, err := svc.MarkArrivedAtDropoff(ctx, "b1", provider)
	if err != nil {
		t.Fatalf("MarkArrivedAtDropoff: %v", err)
	}
	if !view.Stops[1].Running || view.Stops[1].ArrivedAt == nil {
		t.Fatalf("drop-off clock not running: %+v", view.Stops[1])
	}
	if _, err := svc.CustomerWaiting(ctx, "b1", "other-customer"); err == nil {
		t.Fatal("another customer read this booking's waiting")
	}
}

func TestSettleWaitingOutcomes(t *testing.T) {
	provider := "provider-1"
	booking := bookingmodels.Booking{ID: "b1", CustomerID: "c1", ProviderID: &provider}
	ctx := context.Background()

	cases := []struct {
		name     string
		waited   time.Duration
		paidBy   string
		charger  *chargingPayments
		want     string
		wantFee  int64
		wantCall bool
	}{
		{"wallet paid", 95 * time.Minute, "wallet", &chargingPayments{status: bookingmodels.WaitingFeePaid}, bookingmodels.WaitingFeePaid, 400000, true},
		{"wallet short is owed", 95 * time.Minute, "wallet", &chargingPayments{status: bookingmodels.WaitingFeeOwed}, bookingmodels.WaitingFeeOwed, 400000, true},
		{"within free time", 30 * time.Minute, "wallet", &chargingPayments{status: bookingmodels.WaitingFeePaid}, bookingmodels.WaitingFeeNone, 0, false},
		{"cash job", 95 * time.Minute, "cash", &chargingPayments{}, bookingmodels.WaitingFeeCash, 400000, false},
		{"fleet job", 95 * time.Minute, "", &chargingPayments{}, bookingmodels.WaitingFeeNotApplicable, 400000, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newFakeWaitingStore()
			seedWait(store, "b1", c.waited)
			svc := waitingService(&fakeBookingRepo{booking: booking}, c.charger, store)
			svc.settleWaiting(ctx, booking, c.paidBy)
			got := store.records["b1"]
			if got.FeeStatus != c.want || got.FeeKobo != c.wantFee {
				t.Fatalf("status=%s fee=%d, want %s %d", got.FeeStatus, got.FeeKobo, c.want, c.wantFee)
			}
			if (len(c.charger.calls) > 0) != c.wantCall {
				t.Fatalf("wallet charged %d times, want charge=%v", len(c.charger.calls), c.wantCall)
			}
			// A retry never charges again.
			svc.settleWaiting(ctx, booking, c.paidBy)
			if len(c.charger.calls) > 1 {
				t.Fatal("the waiting fee was charged twice")
			}
		})
	}
}

func TestSettleWaitingUnknownOutcomeStaysPending(t *testing.T) {
	provider := "provider-1"
	booking := bookingmodels.Booking{ID: "b1", CustomerID: "c1", ProviderID: &provider}
	store := newFakeWaitingStore()
	seedWait(store, "b1", 95*time.Minute)
	charger := &chargingPayments{err: errors.New("timeout")}
	svc := waitingService(&fakeBookingRepo{booking: booking}, charger, store)
	svc.settleWaiting(context.Background(), booking, "wallet")
	if got := store.records["b1"].FeeStatus; got != bookingmodels.WaitingFeePending {
		t.Fatalf("unknown outcome recorded as %s; it must stay pending", got)
	}
}
