package walletusecases

import (
	"context"
	"strings"
	"time"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

// JobPaymentRecord is the authoritative statement that a unit of dispatched
// work was paid for. It carries references other services already own; no new
// identifier is minted here.
type JobPaymentRecord struct {
	OrganizationID           string
	SourceService            string
	DispatchRequestReference string
	TripReference            string
	BookingReference         string
	PaymentReference         string
	AmountMinor              int64
	Currency                 string
	Status                   string
	PaidAt                   *time.Time
	IdempotencyKey           string
	CorrelationID            string
}

// JobPaymentRecorder is the finance-side writer of authoritative job payment
// verifications. The wallet payment flow owns when a payment happened; the
// finance domain owns the record other services are allowed to read.
type JobPaymentRecorder interface {
	RecordJobPaymentFromWallet(context.Context, JobPaymentRecord) error
}

// jobPaymentStatusFor maps a payment intent's lifecycle onto the verification
// statuses a consuming service reasons about.
//
// Anything not explicitly recognised becomes UNKNOWN rather than a guess:
// downstream gates fail closed on UNKNOWN, which is the correct behaviour for
// a state this service does not understand.
func jobPaymentStatusFor(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case walletmodels.PaymentStatusRequiresPay, walletmodels.PaymentStatusPending:
		return "PENDING"
	case walletmodels.PaymentStatusHeld:
		return "AUTHORIZED"
	case walletmodels.PaymentStatusCompleted:
		return "PAID"
	case walletmodels.PaymentStatusFailed:
		return "FAILED"
	case walletmodels.PaymentStatusRefunded:
		return "REFUNDED"
	case walletmodels.PaymentStatusPartRefunded:
		return "PARTIALLY_REFUNDED"
	default:
		return "UNKNOWN"
	}
}

// organizationFromIntent reads the owning organization off the intent metadata.
//
// A job payment with no organization cannot be recorded as a tenant-scoped
// verification, and inventing a tenant would be worse than recording nothing:
// the lookup would then answer for an organization that never paid.
func organizationFromIntent(intent walletmodels.PaymentIntent) string {
	for _, key := range []string{"organization_id", "organizationId"} {
		if value, ok := intent.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// recordJobPayment mirrors a job payment intent into the authoritative
// verification record.
//
// Failures are deliberately not propagated to the caller: the payment itself
// already succeeded and must not be rolled back because a projection write
// failed. The verification stays absent, and an absent verification blocks
// downstream money movement rather than authorizing it.
func (s *WalletService) recordJobPayment(ctx context.Context, intent walletmodels.PaymentIntent) {
	if s.jobPayments == nil {
		return
	}
	if kind, _ := intent.Metadata["kind"].(string); kind == "wallet_topup" {
		return
	}
	organizationID := organizationFromIntent(intent)
	if organizationID == "" || intent.SourceService == "" || intent.AmountKobo <= 0 {
		return
	}
	status := jobPaymentStatusFor(intent.Status)
	var paidAt *time.Time
	if status == "PAID" {
		value := intent.UpdatedAt
		paidAt = &value
	}
	record := JobPaymentRecord{
		OrganizationID:           organizationID,
		SourceService:            intent.SourceService,
		DispatchRequestReference: intent.SourceReference,
		TripReference:            metadataString(intent, "trip_id", "tripId", "platform_trip_id"),
		BookingReference:         metadataString(intent, "booking_reference", "bookingReference"),
		PaymentReference:         intent.Reference,
		AmountMinor:              intent.AmountKobo,
		Currency:                 strings.ToUpper(walletmodels.DefaultCurrency(intent.Currency)),
		Status:                   status,
		PaidAt:                   paidAt,
		// The status is part of the key so each transition is recorded once and
		// a replayed webhook cannot re-open a settled verification.
		IdempotencyKey: "job-payment:" + intent.Reference,
		CorrelationID:  intent.Reference,
	}
	_ = s.jobPayments.RecordJobPaymentFromWallet(ctx, record)
}

func metadataString(intent walletmodels.PaymentIntent, keys ...string) string {
	for _, key := range keys {
		if value, ok := intent.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
