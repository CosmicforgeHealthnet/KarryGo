package delivery

import (
	"context"
	"errors"
	"time"
)

// ErrDuplicateBookingRef is returned by BookingRepository.CreatePendingBooking
// when the generated booking_ref collides with an existing row, so the service
// can retry with a fresh reference.
var ErrDuplicateBookingRef = errors.New("duplicate booking ref")

// BookingRepository persists partner bookings. All partner-scoped reads MUST
// filter by partner_id so one partner can never access another partner's data.
type BookingRepository interface {
	CreatePendingBooking(ctx context.Context, b *PartnerBooking) error
	UpdatePaymentAndDispatching(ctx context.Context, bookingID, paymentIntentID, dispatchBookingID string, eventPublishedAt time.Time) error
	FindByPartnerAndIDOrRef(ctx context.Context, partnerID, idOrRef string) (*PartnerBooking, error)
	ListByPartner(ctx context.Context, partnerID string, filter ListFilter) ([]PartnerBooking, int, error)
	UpdateStatus(ctx context.Context, bookingID, status string) error
}

// RiderSnapshotUpdater stores the rider/vehicle snapshot captured from the
// dispatch trip.started event together with the matching status transition.
// It is implemented by the infra booking repo and consumed by the webhooks
// dispatch-event processor and the internal callbacks.
type RiderSnapshotUpdater interface {
	UpdateRiderSnapshotAndStatus(ctx context.Context, bookingID string, snap RiderSnapshot, status string) error
}

// IdempotencyRecord is a stored idempotent response for POST /deliveries.
type IdempotencyRecord struct {
	Key         string
	PartnerID   string
	RequestHash string
	Response    []byte // raw JSON response body
	StatusCode  int
	ExpiresAt   time.Time
}

// IdempotencyStore persists partner idempotency keys.
type IdempotencyStore interface {
	// Get returns the record for key, or (nil, nil) when absent.
	Get(ctx context.Context, key string) (*IdempotencyRecord, error)
	// Put upserts the record (overwriting an expired one with the same key).
	Put(ctx context.Context, rec IdempotencyRecord) error
}

// Publisher publishes an event payload to a topic (Redis pub/sub).
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// PaymentIntentInput is the wallet payment-intent creation input.
type PaymentIntentInput struct {
	SourceService   string
	SourceReference string
	CustomerID      string
	CustomerEmail   string
	AmountKobo      int64
	Currency        string
	PaymentMethod   string
	IdempotencyKey  string
}

// PaymentIntentResult is the created payment intent.
type PaymentIntentResult struct {
	ID               string
	Reference        string
	AuthorizationURL string
}

// RefundInput is the wallet refund input.
type RefundInput struct {
	PaymentReference string
	SourceService    string
	SourceReference  string
	AmountKobo       int64
	Currency         string
	Reason           string
	IdempotencyKey   string
}

// WalletClient is the minimal payment-wallet surface the delivery service needs.
// Implemented by an adapter over shared/go/walletclient in cmd wiring.
type WalletClient interface {
	CreatePaymentIntent(ctx context.Context, in PaymentIntentInput) (PaymentIntentResult, error)
	PayFromWallet(ctx context.Context, intentID, idempotencyKey string) error
	RequestRefund(ctx context.Context, in RefundInput) error
}
