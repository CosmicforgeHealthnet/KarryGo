package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/walletclient"
)

// waitingReference is the waiting fee's own wallet payment, kept apart from
// the fare (which is the booking id), so neither can disturb the other.
func waitingReference(bookingID string) string { return bookingID + ":waiting" }

// ChargeWaiting takes the waiting fee from the customer's wallet as its own
// payment, then releases it to the driver; the wallet deducts the usual
// commission, exactly as for the fare. A wallet that is short answers OWED.
// Every step is idempotent by key, so a retry after an unknown outcome
// cannot charge twice.
func (c *WalletPaymentClient) ChargeWaiting(ctx context.Context, in bookingusecases.WaitingChargeInput) (string, string, error) {
	source := waitingReference(in.BookingID)
	intent, err := c.wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   sourceService,
		SourceReference: source,
		CustomerID:      in.CustomerID,
		ProviderID:      in.ProviderID,
		ProviderType:    providerType,
		AmountKobo:      in.AmountKobo,
		Currency:        currencyNGN,
		PaymentMethod:   walletclient.MethodWallet,
		IdempotencyKey:  "hauling-waiting-intent-" + in.BookingID,
	})
	if err != nil {
		return "", "", fmt.Errorf("create waiting payment: %w", err)
	}
	if _, err := c.wallet.PayFromWallet(ctx, intent.ID, "hauling-waiting-pay-"+in.BookingID); err != nil {
		if walletShort(err) {
			return bookingmodels.WaitingFeeOwed, intent.Reference, nil
		}
		readBack, readErr := c.wallet.GetPayment(ctx, intent.Reference)
		if readErr != nil || !walletSecured(readBack.Status) {
			return "", "", fmt.Errorf("pay waiting fee: outcome unknown: %w", err)
		}
	}
	if _, err := c.wallet.CompleteJob(ctx, sourceService, source); err != nil {
		readBack, readErr := c.wallet.GetPayment(ctx, intent.Reference)
		if readErr != nil || !walletSettled(readBack.Status) {
			return "", "", fmt.Errorf("release waiting fee: outcome unknown: %w", err)
		}
	}
	return bookingmodels.WaitingFeePaid, intent.Reference, nil
}

// walletShort reports the wallet refusing a payment for a low balance.
func walletShort(err error) bool {
	var refusal *walletclient.RequestError
	return errors.As(err, &refusal) && refusal.Status == http.StatusConflict &&
		strings.Contains(strings.ToLower(refusal.Message), "not enough")
}
