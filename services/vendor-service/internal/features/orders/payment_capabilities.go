package orders

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PaymentCapability string

const (
	PaymentAdjustableAuthCapture PaymentCapability = "ADJUSTABLE_AUTH_CAPTURE"
	PaymentCapAndRefund          PaymentCapability = "CAP_AND_REFUND"
	PaymentUnsupported           PaymentCapability = "UNSUPPORTED"
)

var ErrUnknownPaymentCapability = errors.New("wallet payment capability is unknown")

func ParsePaymentCapability(value string) (PaymentCapability, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch PaymentCapability(normalized) {
	case PaymentAdjustableAuthCapture, PaymentCapAndRefund, PaymentUnsupported:
		return PaymentCapability(normalized), nil
	default:
		return "", ErrUnknownPaymentCapability
	}
}

func RequirePaymentCapability(value string) (PaymentCapability, error) {
	capability, err := ParsePaymentCapability(value)
	if err != nil || capability == PaymentUnsupported {
		return "", ErrUnknownPaymentCapability
	}
	return capability, nil
}

// SnapshotPaymentCapability persists the Wallet answer alongside the payment
// operation. An unknown answer is rejected before it can authorize a variable
// measurement or commercial adjustment.
func (s *Store) SnapshotPaymentCapability(ctx context.Context, parentOrderID, operationRef, raw string, now time.Time) (PaymentCapability, error) {
	capability, err := ParsePaymentCapability(raw)
	if err != nil {
		return "", err
	}
	if s == nil || s.db == nil {
		return "", ErrDependencyMissing
	}
	_, err = s.db.Exec(ctx, `INSERT INTO commerce_payment_capability_snapshots(id,parent_order_id,payment_operation_ref,capability,observed_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(parent_order_id,payment_operation_ref) DO UPDATE SET capability=EXCLUDED.capability,observed_at=EXCLUDED.observed_at`, "pcap_"+uuid.NewString(), parentOrderID, operationRef, capability, now.UTC())
	return capability, err
}
