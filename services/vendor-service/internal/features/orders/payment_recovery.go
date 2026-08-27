package orders

// Section 17F durable payment recovery.
//
// This is what makes a crash survivable. Every payment leg is committed before
// the money authority is called, so after any restart there is a durable list
// of legs whose outcome is unknown. Recovery asks the authority what actually
// happened rather than assuming, and it never relies on an in-memory goroutine
// having finished.
//
// The rule it exists to uphold: a paid checkout is never silently lost, and a
// payment that cannot complete is unwound through the Wallet authority rather
// than abandoned half-held.

import (
	"context"
	"errors"
	"time"
)

// RecoveryLimits bound one recovery pass.
type RecoveryLimits struct {
	// Grace is how long a leg may sit unresolved before recovery touches it.
	// It exists so an in-flight request is not raced by the sweep.
	Grace time.Duration

	// Deadline is how long a whole operation may stay unresolved before it is
	// treated as terminally failed and unwound. Without it, a leg the
	// authority never answers about would hold stock forever.
	Deadline time.Duration

	// Batch bounds one pass so a large backlog cannot monopolise a tick.
	Batch int
}

func (l RecoveryLimits) normalized() RecoveryLimits {
	if l.Grace <= 0 {
		l.Grace = 30 * time.Second
	}
	if l.Deadline <= 0 {
		l.Deadline = 15 * time.Minute
	}
	if l.Batch <= 0 || l.Batch > 500 {
		l.Batch = 100
	}
	return l
}

// RecoveryOutcome reports what one pass did, for logging and for tests.
type RecoveryOutcome struct {
	LegsReconciled int
	LegsResolved   int
	LegsAbandoned  int
	OperationsHeld int
	OperationsFail int
	Compensated    int

	// Activated counts orders whose fulfilment this pass resumed after a
	// crash left it half-done.
	Activated int
}

// UnresolvedLeg is one leg whose true state is not known locally.
type UnresolvedLeg struct {
	LegID              string
	PaymentOperationID string
	ParentOrderID      string
	CustomerID         string
	Scope              PaymentScope
	VendorOrderID      string
	BusinessID         string
	AmountMinor        int64
	Currency           string
	IdempotencyKey     string
	IntentReference    string
	OperationCreatedAt time.Time
}

// RecoverPayments reconciles every unresolved leg with the money authority.
//
// It is safe to run concurrently with a live request and safe to run repeatedly:
// asking the authority about a leg changes nothing, and re-holding a leg uses
// the same deterministic idempotency key the first attempt used.
func (s *Service) RecoverPayments(
	ctx context.Context, limits RecoveryLimits,
) (RecoveryOutcome, error) {
	var outcome RecoveryOutcome
	if !s.enabled || !s.PaymentEnabled() {
		return outcome, nil
	}
	limits = limits.normalized()
	now := s.now().UTC()

	legs, err := s.store.ListUnresolvedLegs(ctx, now.Add(-limits.Grace), limits.Batch)
	if err != nil {
		return outcome, err
	}

	for _, leg := range legs {
		outcome.LegsReconciled++

		result, resolveErr := s.resolveLeg(ctx, leg)
		if resolveErr != nil {
			// The authority still cannot tell us. If the whole operation has
			// been unresolved past its deadline, stop waiting: mark this leg
			// failed so the operation can unwind and stop holding stock.
			if now.Sub(leg.OperationCreatedAt) < limits.Deadline {
				continue
			}
			outcome.LegsAbandoned++
			result = LegResult{Status: "failed"}
		} else {
			outcome.LegsResolved++
		}

		if _, err = s.store.RecordLegResult(
			ctx, leg.LegID, result, redactAuthorityError(resolveErr), s.now().UTC(),
		); err != nil {
			return outcome, err
		}
	}

	compensated, err := s.compensateFailedOperations(ctx, limits)
	if err != nil {
		return outcome, err
	}
	outcome.Compensated = compensated

	// Terminal vendor allocations settle/refund independently. This is kept
	// separate from failed-operation compensation so A's cancellation can never
	// hold up B or C's settlement.
	//
	// Each sweep is resilient PER ITEM: one unreachable authority or one bad
	// order is skipped inside the sweep rather than stalling the others. A
	// SWEEP-LEVEL error is different — it means the sweep could not run at all
	// (its listing query failed) — and it must reach the durable job so the
	// coordinator retries, counts the failure and eventually dead-letters it.
	// Discarding it here would make a persistent database or Wallet outage
	// completely silent while the job kept reporting success.
	var sweepErrs []error
	if _, err = s.recoverTerminalPaymentLegs(ctx, limits.Batch); err != nil {
		sweepErrs = append(sweepErrs, err)
	}
	if _, err = s.RecoverCommercialChanges(ctx, limits.Batch); err != nil {
		sweepErrs = append(sweepErrs, err)
	}
	if _, err = s.RecoverCommercialAdjustments(ctx, limits.Batch); err != nil {
		sweepErrs = append(sweepErrs, err)
	}
	if _, err = s.RecoverMissingRefundObligations(ctx, limits.Batch); err != nil {
		sweepErrs = append(sweepErrs, err)
	}
	if _, err = s.RecoverCommerceRefundPolicies(ctx, limits.Batch); err != nil {
		sweepErrs = append(sweepErrs, err)
	}

	// The fulfilment-pending boundary. A process that died between holding a
	// payment and confirming stock or creating a mission left an order here,
	// and it is found by durable state rather than by a goroutine that no
	// longer exists.
	activated, err := s.resumeActivation(ctx, limits)
	if err != nil {
		sweepErrs = append(sweepErrs, err)
	}
	outcome.Activated = activated

	// Joined rather than first-wins, so one failing sweep never hides another
	// from the operator reading the job's error.
	return outcome, errors.Join(sweepErrs...)
}

// resumeActivation re-runs activation for every paid order that is not yet
// fully activated.
//
// A failure on one order does not stop the sweep: each order is independent,
// and one unreachable logistics authority must not stall every other purchase.
func (s *Service) resumeActivation(
	ctx context.Context, limits RecoveryLimits,
) (int, error) {
	if !s.FulfilmentEnabled() {
		return 0, nil
	}
	ids, err := s.store.ListOrdersAwaitingActivation(ctx, limits.Batch)
	if err != nil {
		return 0, err
	}

	activated := 0
	for _, id := range ids {
		if s.production != nil {
			if order, loadErr := s.store.GetParentByID(ctx, id); loadErr == nil && order.PaymentState == string(PaymentHeld) {
				for _, vendorOrder := range order.VendorOrders {
					_ = s.production.ActivateVendorOrderSystem(ctx, vendorOrder.ID)
				}
			}
		}
		outcome, activateErr := s.ActivateFulfilment(ctx, id)
		if activateErr != nil {
			continue
		}
		if outcome.InventoryConfirmed > 0 || outcome.MissionsCreated > 0 {
			activated++
		}
	}
	return activated, nil
}

// resolveLeg learns a leg's true state from the money authority.
//
// A leg that already has a reference is READ, never re-paid: the authority
// created something, and asking is the only safe way to learn what. A leg with
// no reference at all is re-attempted under its original idempotency key,
// which is safe precisely because the key is deterministic.
func (s *Service) resolveLeg(ctx context.Context, leg UnresolvedLeg) (LegResult, error) {
	if authority, ok := s.payments.(CommercePaymentAuthority); ok {
		result, err := authority.GetCommercePayment(ctx, leg.ParentOrderID)
		if err != nil && leg.IntentReference == "" {
			order, loadErr := s.store.GetParentByID(ctx, leg.ParentOrderID)
			if loadErr != nil {
				return LegResult{}, loadErr
			}
			operation, loadErr := s.store.GetOperationForOrder(ctx, leg.CustomerID, leg.ParentOrderID)
			if loadErr != nil {
				return LegResult{}, loadErr
			}
			if _, loadErr = s.authorizeCommercePayment(ctx, authority, order, operation, ""); loadErr != nil {
				return LegResult{}, loadErr
			}
			result, err = authority.GetCommercePayment(ctx, leg.ParentOrderID)
		}
		if err != nil {
			return LegResult{}, err
		}
		return LegResult{IntentID: result.IntentID, Reference: result.Reference, Status: result.Status, AmountKobo: leg.AmountMinor, SnapshotFingerprint: result.AllocationFingerprints[leg.VendorOrderID]}, nil
	}
	if leg.IntentReference != "" {
		return s.payments.GetLeg(ctx, leg.IntentReference)
	}

	request := HoldLegRequest{
		IdempotencyKey:  leg.IdempotencyKey,
		SourceService:   PaymentSourceService,
		SourceReference: leg.VendorOrderID,
		CustomerID:      leg.CustomerID,
		AmountKobo:      leg.AmountMinor,
		Currency:        leg.Currency,
		Metadata: map[string]any{
			"parent_order_id": leg.ParentOrderID,
			"scope":           string(leg.Scope),
			"recovery":        true,
		},
	}
	if leg.Scope == ScopeVendorLeg {
		request.ProviderType = PaymentProviderTypeVendor
		request.ProviderID = leg.BusinessID
	} else {
		request.SourceReference = leg.ParentOrderID
	}
	return s.payments.HoldLeg(ctx, request)
}

// compensateFailedOperations unwinds operations that can never complete.
//
// A partially-held payment is money the customer parted with for a purchase
// that will not happen. Every held leg is refunded through the Wallet
// authority — Vendor Commerce moves nothing itself — and only then is the
// operation terminal.
func (s *Service) compensateFailedOperations(
	ctx context.Context, limits RecoveryLimits,
) (int, error) {
	operations, err := s.store.ListCompensatingOperations(ctx, limits.Batch)
	if err != nil {
		return 0, err
	}

	compensated := 0
	for _, operation := range operations {
		unwound := true
		for _, leg := range operation.Legs {
			if leg.Status != LegHeld {
				continue
			}
			refund, refundErr := s.payments.RefundLeg(ctx, RefundLegRequest{
				IdempotencyKey:   "compensate-" + leg.ID,
				PaymentReference: leg.IntentReference,
				SourceService:    PaymentSourceService,
				SourceReference:  refundSourceReference(leg, operation),
				AmountKobo:       leg.AmountMinor,
				Currency:         leg.Currency,
				Reason:           "commerce payment could not be completed",
			})
			if refundErr != nil {
				// Leave it held and try again next pass. Abandoning a held leg
				// would be losing the customer's money.
				unwound = false
				continue
			}
			if !refund.Refunded() {
				// A pending/processing answer is not success. Leave the leg
				// held so the next recovery pass can read the authority truth.
				unwound = false
				continue
			}
			if refund.AmountKobo != 0 && refund.AmountKobo != leg.AmountMinor {
				unwound = false
				continue
			}
			if _, err = s.store.RecordLegRefund(
				ctx, leg.ID, refund.AmountKobo, s.now().UTC(),
			); err != nil {
				return compensated, err
			}
		}

		if !unwound {
			continue
		}
		if err = s.store.MarkOperationCompensated(
			ctx, operation.ID, s.now().UTC(),
		); err != nil {
			return compensated, err
		}
		compensated++
	}
	return compensated, nil
}

func refundSourceReference(leg PaymentLegRecord, operation PaymentOperation) string {
	if leg.Scope == ScopeVendorLeg {
		return leg.VendorOrderID
	}
	return operation.ParentOrderID
}

// RefundLegRequest asks the money authority to refund one leg.
//
// Refunding one projected leg never touches another. The current Commerce
// authority holds one parent intent and targets its immutable allocation; the
// legacy port remains only for already-created per-leg intents.
type RefundLegRequest struct {
	IdempotencyKey   string
	PaymentReference string
	SourceService    string
	SourceReference  string
	AmountKobo       int64
	Currency         string
	Reason           string
}

// RefundResult is the authority's answer about a refund.
type RefundResult struct {
	Reference  string
	Status     string
	AmountKobo int64
}

func (r RefundResult) Refunded() bool {
	switch r.Status {
	case "processed", "PROCESSED", "refunded", "REFUNDED":
		return true
	default:
		return false
	}
}
