package orders

// Section 17F payment usecases.
//
// The flow is deliberately order-first: the ParentOrder and its VendorOrders
// are created (exactly once, from an approved quote) BEFORE any money is asked
// for. That is what makes each payment leg able to name a real VendorOrder as
// its source reference, and it removes the worst failure mode by construction —
// there is no window where money exists for an order that was never created.
//
// The remaining window, money held for an order whose legs did not all answer,
// is durable and recoverable: every leg row is committed before the first call
// to the money authority.

import (
	"context"
	"strings"
)

// PaymentOptions extends the order service with the money authority.
type PaymentOptions struct {
	Authority PaymentAuthority

	// CustomerEmail resolves the email the Wallet authority requires for a
	// card-capable intent. It is a port so this feature never learns how to
	// reach Customer Service.
	CustomerEmail CustomerEmailResolver
}

// CustomerEmailResolver answers the customer's email for a payment.
type CustomerEmailResolver interface {
	CustomerEmail(ctx context.Context, customerID string) (string, error)
}

// noCustomerEmail is the safe default. An absent email is not an error: the
// Wallet authority only requires one for card-capable methods, and Section 17F
// pays from wallet balance.
type noCustomerEmail struct{}

func (noCustomerEmail) CustomerEmail(context.Context, string) (string, error) {
	return "", nil
}

// SetPaymentAuthority wires the money authority onto the order service.
//
// It is separate from NewService so a deployment without Wallet credentials
// builds a fully working order service with no payment surface at all, rather
// than a payment surface that can only ever fail.
func (s *Service) SetPaymentAuthority(options PaymentOptions) {
	s.payments = options.Authority
	s.customerEmail = options.CustomerEmail
	if s.customerEmail == nil {
		s.customerEmail = noCustomerEmail{}
	}
}

// PaymentEnabled reports whether a real money authority is wired.
func (s *Service) PaymentEnabled() bool {
	return s != nil && s.payments != nil && s.payments.Configured()
}

// StartPayment binds a payment operation to an approved quote and asks the
// money authority to hold each leg.
//
// Security properties, all enforced here rather than trusted from the caller:
//
//   - the quote must still be approved and unexpired, re-verified at THIS
//     moment rather than at approval time;
//   - the presented fingerprint must match the approved agreement, so a client
//     cannot swap one quote for another;
//   - every amount is read from the stored order, never from the request, so a
//     tampered total has nothing to tamper with;
//   - an operation already bound to a different agreement is refused rather
//     than reused.
func (s *Service) StartPayment(
	ctx context.Context, customerID, quoteID, fingerprint string,
) (ParentOrder, PaymentOperation, error) {
	if !s.enabled {
		return ParentOrder{}, PaymentOperation{}, ErrDisabled
	}
	if !s.PaymentEnabled() {
		return ParentOrder{}, PaymentOperation{}, ErrPaymentNotConfigured
	}

	customerID = strings.TrimSpace(customerID)
	quoteID = strings.TrimSpace(quoteID)
	fingerprint = strings.TrimSpace(fingerprint)
	if customerID == "" || quoteID == "" || fingerprint == "" {
		return ParentOrder{}, PaymentOperation{}, ErrQuoteNotOrderable
	}

	// Creating the order first is what gives each leg a real VendorOrder to
	// name. It is exactly-once per approved quote, so a retried payment start
	// finds the same order rather than creating a second one.
	//
	// A quote that has expired, lost its stock reservation or had its checkout
	// revalidated is refused here, before any money is asked for.
	order, _, err := s.CreateFromQuote(ctx, customerID, quoteID, fingerprint)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}

	binding, err := BindingFor(order)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}
	// The presented fingerprint must be the one the order was cut from. This
	// is a second, independent check: CreateFromQuote verified it against the
	// live quote, and this verifies it against the frozen order.
	if binding.QuoteFingerprint != fingerprint {
		return ParentOrder{}, PaymentOperation{}, ErrFingerprintMismatch
	}

	legs, err := PlanPaymentLegs(order)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}

	operation, _, err := s.store.EnsureOperation(ctx, order, binding, legs, s.now().UTC())
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}

	// Every leg row is durable at this point, so a crash from here on is
	// recoverable by asking the authority about each one.
	operation, err = s.authorizeLegs(ctx, order, operation)
	if err != nil {
		return order, operation, err
	}

	refreshed, err := s.store.GetParent(ctx, customerID, order.ID)
	if err != nil {
		return order, operation, err
	}
	return refreshed, operation, nil
}

// authorizeLegs asks the money authority to hold each pending leg.
//
// It is safe to call repeatedly. Each leg carries a deterministic idempotency
// key, so a leg already held returns the same intent rather than a second one,
// and a leg already answered is skipped entirely.
func (s *Service) authorizeLegs(
	ctx context.Context, order ParentOrder, operation PaymentOperation,
) (PaymentOperation, error) {
	email, err := s.customerEmail.CustomerEmail(ctx, order.CustomerID)
	if err != nil {
		// An unavailable email lookup must not strand a payment. The wallet
		// method does not require one.
		email = ""
	}

	current := operation
	for _, leg := range operation.Legs {
		if leg.Status != LegPending {
			continue
		}

		now := s.now().UTC()
		// Written before the call, so a crash mid-call still leaves evidence
		// that money may exist against this leg.
		if err = s.store.MarkLegRequested(ctx, leg, now); err != nil {
			return current, err
		}

		request := HoldLegRequest{
			IdempotencyKey:  leg.IdempotencyKey,
			SourceService:   PaymentSourceService,
			SourceReference: leg.VendorOrderID,
			CustomerID:      order.CustomerID,
			CustomerEmail:   email,
			AmountKobo:      leg.AmountMinor,
			Currency:        leg.Currency,
			Metadata: map[string]any{
				"parent_order_id":   order.ID,
				"quote_id":          order.QuoteID,
				"quote_fingerprint": order.QuoteFingerprint,
				"scope":             string(leg.Scope),
			},
		}
		if leg.Scope == ScopeVendorLeg {
			// Naming the beneficiary at creation is what lets the authority
			// settle this leg to this vendor, and only this vendor, later.
			request.ProviderType = PaymentProviderTypeVendor
			request.ProviderID = leg.BusinessID
		} else {
			// The platform leg names the ParentOrder, because it pays for the
			// order-level charge rather than for any vendor's goods.
			request.SourceReference = order.ID
		}

		result, holdErr := s.payments.HoldLeg(ctx, request)
		if holdErr == nil && result.AmountKobo != 0 && result.AmountKobo != leg.AmountMinor {
			// A Wallet answer for another amount is not a successful hold for
			// this leg. Keep it unresolved so recovery reads the authority again.
			holdErr = ErrPaymentAuthority
		}
		failureReason := ""
		if holdErr != nil {
			// The authority's own words are recorded. Vendor Commerce never
			// decides that a payment succeeded, and never decides that a
			// timeout means failure: an unrecognised answer keeps the leg
			// PENDING so recovery re-reads it.
			failureReason = redactAuthorityError(holdErr)
			result = LegResult{Status: ""}
		}

		current, err = s.store.RecordLegResult(ctx, leg.ID, result, failureReason, s.now().UTC())
		if err != nil {
			return current, err
		}
		if holdErr != nil {
			return current, ErrPaymentAuthority
		}
	}
	return current, nil
}

// GetPaymentForCustomer reads one order's payment operation.
func (s *Service) GetPaymentForCustomer(
	ctx context.Context, customerID, parentOrderID string,
) (PaymentOperation, error) {
	if !s.enabled {
		return PaymentOperation{}, ErrDisabled
	}
	return s.store.GetOperationForOrder(ctx,
		strings.TrimSpace(customerID), strings.TrimSpace(parentOrderID))
}

// PaymentSourceService is the identity Vendor Commerce presents to the money
// authority. payment-wallet-service records it on every intent and ledger row,
// so a settlement is always traceable back to commerce.
const PaymentSourceService = "vendor-service"

// PaymentProviderTypeVendor is the provider type a vendor leg settles to.
const PaymentProviderTypeVendor = "vendor"

// redactAuthorityError keeps an authority failure loggable without carrying
// anything that is not a status or a reason.
func redactAuthorityError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	const limit = 200
	if len(message) > limit {
		return message[:limit]
	}
	return message
}
