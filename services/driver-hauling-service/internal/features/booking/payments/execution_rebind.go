package payments

// Section 17: keeping the held payment bound to whoever actually does the job.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"cosmicforge/logistics/shared/go/walletclient"
)

// RecoveryCaseLookup finds the recovery case that made providerID the
// booking's provider. It returns "" when there is none.
type RecoveryCaseLookup func(ctx context.Context, bookingID, providerID string) (string, error)

// WithRecoveryCases lets the settlement guard justify a rebind it has to make
// itself, because the eager one after recovery did not land.
func (c *WalletPaymentClient) WithRecoveryCases(lookup RecoveryCaseLookup) *WalletPaymentClient {
	c.recoveryCases = lookup
	return c
}

// RebindToReplacement moves the booking's held payment to the replacement.
//
// A booking with no wallet payment (cash) and one whose payment is no longer
// held (refunded, never secured) have nothing to move, so both are success.
func (c *WalletPaymentClient) RebindToReplacement(ctx context.Context, bookingID, providerID, recoveryCaseRef string) error {
	err := c.wallet.RebindToReplacement(ctx, sourceService, bookingID, providerType, providerID,
		recoveryCaseRef, "PROVIDER_REPLACED")
	if errors.Is(err, walletclient.ErrPaymentNotHeld) || isNotFound(err) {
		return nil
	}
	return err
}

// EnsureExecutionProvider is the settlement guard: it makes sure the held
// payment is bound to the provider about to be paid, before anything is
// released.
//
// Already bound is one read. Bound to someone else is corrected only when a
// recovery case explains the difference; without one the mismatch is
// unexplained, and paying either provider would be a guess -- so it is
// refused, and the settlement is left for reconciliation.
func (c *WalletPaymentClient) EnsureExecutionProvider(ctx context.Context, bookingID, providerID string) error {
	binding, err := c.wallet.ExecutionProviderBinding(ctx, sourceService, bookingID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("read payment binding: %w", err)
	}
	if binding.ProviderID == providerID && binding.ProviderType == providerType {
		return nil
	}
	if binding.Status != "held" {
		// Nothing left to release; CompleteJob will say so itself.
		return nil
	}
	if c.recoveryCases == nil {
		return fmt.Errorf("payment is bound to %q, booking to %q, and no recovery record is available",
			binding.ProviderID, providerID)
	}
	caseRef, err := c.recoveryCases(ctx, bookingID, providerID)
	if err != nil {
		return fmt.Errorf("find recovery case: %w", err)
	}
	if caseRef == "" {
		return fmt.Errorf("payment is bound to %q, booking to %q, and no recovery case explains it",
			binding.ProviderID, providerID)
	}
	return c.RebindToReplacement(ctx, bookingID, providerID, caseRef)
}

func isNotFound(err error) bool {
	var requestErr *walletclient.RequestError
	return errors.As(err, &requestErr) && requestErr.Status == http.StatusNotFound
}
