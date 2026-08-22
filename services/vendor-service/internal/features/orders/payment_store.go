package orders

// Section 17F payment persistence.
//
// The ordering here is the whole safety property: the payment operation and
// every leg are committed BEFORE the first call to the money authority is made.
// A crash at any point after that leaves a durable record naming exactly which
// legs may have money against them, so recovery can ask the authority what
// really happened instead of guessing.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
)

const paymentOperationColumns = `
	o.id,o.parent_order_id,o.customer_id,o.quote_id,o.quote_version,o.quote_fingerprint,
	o.quoted_total_minor,o.max_authorized_total_minor,o.currency,o.allocation_fingerprint,
	o.payment_method,o.state,o.failure_code,o.version,o.created_at,o.updated_at,
	o.settled_at,o.terminal_at`

const paymentLegColumns = `
	l.id,l.payment_operation_id,l.scope,COALESCE(l.vendor_order_id,''),l.business_id,
	l.amount_minor,l.currency,l.idempotency_key,l.intent_reference,l.intent_id,
	l.authority_status,l.status,l.failure_reason,l.refunded_minor,l.settled_minor,
	l.version,l.created_at,l.updated_at`

// EnsureOperation creates the payment operation and its legs, or returns the
// one that already exists.
//
// Idempotency is structural: commerce_payment_operations.parent_order_id and
// .quote_id are both UNIQUE, so a retried start can never produce a second
// operation and never a second set of legs.
//
// The binding is verified on every call, including a retry. An operation
// already bound to a different agreement is refused rather than reused, which
// is what stops a quote swap or an amount tamper from riding an existing
// operation.
func (s *Store) EnsureOperation(
	ctx context.Context,
	order ParentOrder,
	binding PaymentBinding,
	legs []PaymentLeg,
	now time.Time,
) (PaymentOperation, bool, error) {
	if s == nil || s.db == nil {
		return PaymentOperation{}, false, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PaymentOperation{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize per order so two concurrent starts agree on one operation
	// rather than racing to the UNIQUE constraint and both failing.
	if _, err = tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,17))`, order.ID); err != nil {
		return PaymentOperation{}, false, err
	}

	existing, err := loadOperationBy(ctx, tx, `o.parent_order_id=$1 AND o.customer_id=$2`,
		order.ID, order.CustomerID)
	if err == nil {
		// A retry must present the same agreement. A different one is a
		// different payment and may not reuse this operation.
		if !existing.Binding.Matches(binding) {
			return PaymentOperation{}, false, ErrBindingMismatch
		}
		if err = tx.Commit(ctx); err != nil {
			return PaymentOperation{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrOrderNotFound) {
		return PaymentOperation{}, false, err
	}

	operationID := "cpop_" + uuid.NewString()
	if _, err = tx.Exec(ctx, `
		INSERT INTO commerce_payment_operations(
			id,parent_order_id,customer_id,quote_id,quote_version,quote_fingerprint,
			quoted_total_minor,max_authorized_total_minor,currency,allocation_fingerprint,
			payment_method,state,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'wallet','PENDING_PAYMENT',1,$11,$11)`,
		operationID, order.ID, order.CustomerID, binding.QuoteID, binding.QuoteVersion,
		binding.QuoteFingerprint, binding.QuotedTotalMinor, binding.MaxAuthorizedTotalMinor,
		binding.Currency, binding.AllocationFingerprint, now); err != nil {
		return PaymentOperation{}, false, err
	}

	for _, leg := range legs {
		var vendorOrderRef any
		if leg.VendorOrderID != "" {
			vendorOrderRef = leg.VendorOrderID
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO commerce_payment_legs(
				id,payment_operation_id,parent_order_id,scope,vendor_order_id,business_id,
				amount_minor,currency,idempotency_key,status,version,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'PENDING',1,$10,$10)`,
			"cpl_"+uuid.NewString(), operationID, order.ID, leg.Scope, vendorOrderRef,
			leg.BusinessID, leg.AmountMinor, binding.Currency, leg.IdempotencyKey,
			now); err != nil {
			return PaymentOperation{}, false, err
		}
	}

	if err = appendPaymentEvent(ctx, tx, operationID, "", order.ID, order.CustomerID,
		"commerce.payment.operation_started", "CUSTOMER", order.CustomerID,
		"", string(PaymentPending), "", now); err != nil {
		return PaymentOperation{}, false, err
	}

	created, err := loadOperationBy(ctx, tx, `o.id=$1`, operationID)
	if err != nil {
		return PaymentOperation{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PaymentOperation{}, false, err
	}
	return created, true, nil
}

// GetOperationForOrder reads one order's payment operation.
func (s *Store) GetOperationForOrder(
	ctx context.Context, customerID, parentOrderID string,
) (PaymentOperation, error) {
	if s == nil || s.db == nil {
		return PaymentOperation{}, ErrDependencyMissing
	}
	return loadOperationBy(ctx, s.db,
		`o.parent_order_id=$1 AND o.customer_id=$2`, parentOrderID, customerID)
}

func loadOperationBy(
	ctx context.Context, q queryer, where string, args ...any,
) (PaymentOperation, error) {
	var operation PaymentOperation
	err := q.QueryRow(ctx,
		`SELECT `+paymentOperationColumns+
			` FROM commerce_payment_operations o WHERE `+where, args...).Scan(
		&operation.ID, &operation.ParentOrderID, &operation.CustomerID,
		&operation.Binding.QuoteID, &operation.Binding.QuoteVersion,
		&operation.Binding.QuoteFingerprint, &operation.Binding.QuotedTotalMinor,
		&operation.Binding.MaxAuthorizedTotalMinor, &operation.Binding.Currency,
		&operation.Binding.AllocationFingerprint, &operation.PaymentMethod,
		&operation.State, &operation.FailureCode, &operation.Version,
		&operation.CreatedAt, &operation.UpdatedAt, &operation.SettledAt,
		&operation.TerminalAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOrderNotFound
	}
	if err != nil {
		return PaymentOperation{}, err
	}
	operation.Binding.CustomerID = operation.CustomerID

	if operation.Legs, err = loadLegs(ctx, q, operation.ID); err != nil {
		return PaymentOperation{}, err
	}
	return operation, nil
}

func loadLegs(ctx context.Context, q queryer, operationID string) ([]PaymentLegRecord, error) {
	rows, err := q.Query(ctx,
		`SELECT `+paymentLegColumns+
			` FROM commerce_payment_legs l WHERE l.payment_operation_id=$1
			  ORDER BY l.scope DESC, l.vendor_order_id, l.id`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var legs []PaymentLegRecord
	for rows.Next() {
		var leg PaymentLegRecord
		if err = rows.Scan(&leg.ID, &leg.PaymentOperationID, &leg.Scope, &leg.VendorOrderID,
			&leg.BusinessID, &leg.AmountMinor, &leg.Currency, &leg.IdempotencyKey,
			&leg.IntentReference, &leg.IntentID, &leg.AuthorityStatus, &leg.Status,
			&leg.FailureReason, &leg.RefundedMinor, &leg.SettledMinor,
			&leg.Version, &leg.CreatedAt, &leg.UpdatedAt); err != nil {
			return nil, err
		}
		legs = append(legs, leg)
	}
	return legs, rows.Err()
}

// RecordLegResult stores what the Wallet authority answered for one leg and
// re-derives the operation state.
//
// It is idempotent: replaying the same authority answer produces the same row
// and no second financial effect, which is what makes a retried callback or a
// re-read during recovery safe.
func (s *Store) RecordLegResult(
	ctx context.Context,
	legID string,
	result LegResult,
	failureReason string,
	now time.Time,
) (PaymentOperation, error) {
	if s == nil || s.db == nil {
		return PaymentOperation{}, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PaymentOperation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var operationID, parentOrderID, customerID, previousStatus string
	if err = tx.QueryRow(ctx, `
		SELECT l.payment_operation_id,l.parent_order_id,o.customer_id,l.status
		  FROM commerce_payment_legs l
		  JOIN commerce_payment_operations o ON o.id=l.payment_operation_id
		 WHERE l.id=$1 FOR UPDATE OF l`, legID).Scan(
		&operationID, &parentOrderID, &customerID, &previousStatus); errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOrderNotFound
	} else if err != nil {
		return PaymentOperation{}, err
	}
	// Lock the operation so concurrent leg answers serialize on the state they
	// both re-derive.
	var previousState string
	if err = tx.QueryRow(ctx,
		`SELECT state FROM commerce_payment_operations WHERE id=$1 FOR UPDATE`,
		operationID).Scan(&previousState); err != nil {
		return PaymentOperation{}, err
	}

	status := StatusFor(result)
	if _, err = tx.Exec(ctx, `
		UPDATE commerce_payment_legs
		   SET intent_reference=CASE WHEN $2 <> '' THEN $2 ELSE intent_reference END,
		       intent_id=CASE WHEN $3 <> '' THEN $3 ELSE intent_id END,
		       authority_status=$4,status=$5,failure_reason=$6,
		       version=version+1,updated_at=$7
		 WHERE id=$1`,
		legID, result.Reference, result.IntentID, result.Status, status,
		failureReason, now); err != nil {
		return PaymentOperation{}, err
	}

	action := "commerce.payment.leg_held"
	if status == LegFailed {
		action = "commerce.payment.leg_failed"
	}
	if err = appendPaymentEvent(ctx, tx, operationID, legID, parentOrderID, customerID,
		action, "AUTHORITY", "", previousStatus, string(status),
		result.Status, now); err != nil {
		return PaymentOperation{}, err
	}

	operation, err := refreshPaymentState(
		ctx, tx, s.outbox, operationID, parentOrderID, customerID, previousState, now,
	)
	if err != nil {
		return PaymentOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PaymentOperation{}, err
	}
	return operation, nil
}

// refreshPaymentState re-derives the operation state from its legs and mirrors
// it onto the order.
//
// The state is never written from a caller's opinion: a partially-held payment
// cannot report itself as fully held because somebody said so.
func refreshPaymentState(
	ctx context.Context,
	tx pgx.Tx,
	outbox *vendorevents.DomainPublisher,
	operationID, parentOrderID, customerID, previousState string,
	now time.Time,
) (PaymentOperation, error) {
	legs, err := loadLegs(ctx, tx, operationID)
	if err != nil {
		return PaymentOperation{}, err
	}
	state := DerivePaymentState(legs)

	var terminalAt, settledAt any
	if state.Terminal() {
		terminalAt = now
	}
	if state == PaymentHeld {
		settledAt = now
	}

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_payment_operations
		   SET state=$1,
		       terminal_at=COALESCE(terminal_at,$2::timestamptz),
		       settled_at=COALESCE(settled_at,$3::timestamptz),
		       version=version+1,updated_at=$4
		 WHERE id=$5`,
		state, terminalAt, settledAt, now, operationID); err != nil {
		return PaymentOperation{}, err
	}

	// The order mirrors the payment state so no reader has to join to learn
	// whether an order may be worked.
	if _, err = tx.Exec(ctx, `
		UPDATE commerce_parent_orders
		   SET payment_state=$1,version=version+1,updated_at=$2
		 WHERE id=$3`, state, now, parentOrderID); err != nil {
		return PaymentOperation{}, err
	}

	// Payment authority is the only source allowed to make a vendor order
	// actionable. Lock the children under the same transaction as the Wallet
	// result, then advance each child monotonically. A retry sees the already
	// advanced status and emits nothing twice.
	rows, err := tx.Query(ctx, `
		SELECT id,status,version FROM commerce_vendor_orders
		 WHERE parent_order_id=$1 ORDER BY id FOR UPDATE`, parentOrderID)
	if err != nil {
		return PaymentOperation{}, err
	}
	type childState struct {
		id      string
		status  VendorStatus
		version int64
	}
	var children []childState
	for rows.Next() {
		var child childState
		if err = rows.Scan(&child.id, &child.status, &child.version); err != nil {
			rows.Close()
			return PaymentOperation{}, err
		}
		children = append(children, child)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return PaymentOperation{}, err
	}
	rows.Close()
	for _, child := range children {
		status := child.status
		if state != PaymentHeld && (status == VendorStatusCreated || status == VendorStatusPendingPayment) {
			if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='PENDING_PAYMENT',version=version+1,updated_at=$1 WHERE id=$2 AND version=$3`, now, child.id, child.version); err != nil {
				return PaymentOperation{}, err
			}
			status = VendorStatusPendingPayment
		}
		if state != PaymentHeld {
			continue
		}
		if status == VendorStatusCreated || status == VendorStatusPendingPayment {
			if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='PAID',version=version+1,updated_at=$1 WHERE id=$2`, now, child.id); err != nil {
				return PaymentOperation{}, err
			}
			if err = appendEvent(ctx, tx, outbox, parentOrderID, child.id, customerID, ActionPaid, "AUTHORITY", operationID, string(status), string(VendorStatusPaid), now); err != nil {
				return PaymentOperation{}, err
			}
			status = VendorStatusPaid
		}
		if status == VendorStatusPaid {
			if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='SENT_TO_VENDOR',version=version+1,updated_at=$1 WHERE id=$2`, now, child.id); err != nil {
				return PaymentOperation{}, err
			}
			if err = appendEvent(ctx, tx, outbox, parentOrderID, child.id, customerID, ActionSentToVendor, "SYSTEM", operationID, string(status), string(VendorStatusSentToVendor), now); err != nil {
				return PaymentOperation{}, err
			}
		}
	}

	if previousState != string(state) {
		if err = appendPaymentEvent(ctx, tx, operationID, "", parentOrderID, customerID,
			"commerce.payment.state_changed", "SYSTEM", "",
			previousState, string(state), "", now); err != nil {
			return PaymentOperation{}, err
		}
	}

	return loadOperationBy(ctx, tx, `o.id=$1`, operationID)
}

// MarkLegRequested records that the authority is about to be called.
//
// It is written before the call so a crash mid-call still leaves evidence that
// money may exist against this leg.
func (s *Store) MarkLegRequested(ctx context.Context, leg PaymentLegRecord, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentOrderID, customerID string
	if err = tx.QueryRow(ctx, `
		SELECT l.parent_order_id,o.customer_id
		  FROM commerce_payment_legs l
		  JOIN commerce_payment_operations o ON o.id=l.payment_operation_id
		 WHERE l.id=$1`, leg.ID).Scan(&parentOrderID, &customerID); err != nil {
		return err
	}
	if err = appendPaymentEvent(ctx, tx, leg.PaymentOperationID, leg.ID,
		parentOrderID, customerID, "commerce.payment.leg_requested", "SYSTEM", "",
		string(LegPending), string(LegPending), "", now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appendPaymentEvent(
	ctx context.Context,
	tx pgx.Tx,
	operationID, legID, parentOrderID, customerID, action, actorType, actorRef,
	fromState, toState, authorityDetail string,
	now time.Time,
) error {
	var legRef any
	if legID != "" {
		legRef = legID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_payment_events(
			id,payment_operation_id,payment_leg_id,parent_order_id,customer_id,
			action,actor_type,actor_ref,from_state,to_state,authority_detail,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		"cpe_"+uuid.NewString(), operationID, legRef, parentOrderID, customerID,
		action, actorType, actorRef, fromState, toState, authorityDetail, now)
	return err
}

// -----------------------------------------------------------------------------
// Recovery reads and writes
// -----------------------------------------------------------------------------

// ListUnresolvedLegs finds legs whose true state is not known locally.
//
// The grace cutoff keeps the sweep from racing a request that is still in
// flight, and the operation must still be live: a terminal operation's legs are
// somebody else's problem, not recovery's.
func (s *Store) ListUnresolvedLegs(
	ctx context.Context, before time.Time, limit int,
) ([]UnresolvedLeg, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT l.id,l.payment_operation_id,l.parent_order_id,o.customer_id,l.scope,
		       COALESCE(l.vendor_order_id,''),l.business_id,l.amount_minor,l.currency,
		       l.idempotency_key,l.intent_reference,o.created_at
		  FROM commerce_payment_legs l
		  JOIN commerce_payment_operations o ON o.id=l.payment_operation_id
		 WHERE l.status='PENDING'
		   AND l.updated_at < $1
		   AND o.state IN ('PENDING_PAYMENT','AUTHORIZING')
		 ORDER BY l.updated_at, l.id
		 LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var legs []UnresolvedLeg
	for rows.Next() {
		var leg UnresolvedLeg
		if err = rows.Scan(&leg.LegID, &leg.PaymentOperationID, &leg.ParentOrderID,
			&leg.CustomerID, &leg.Scope, &leg.VendorOrderID, &leg.BusinessID,
			&leg.AmountMinor, &leg.Currency, &leg.IdempotencyKey,
			&leg.IntentReference, &leg.OperationCreatedAt); err != nil {
			return nil, err
		}
		legs = append(legs, leg)
	}
	return legs, rows.Err()
}

// ListCompensatingOperations finds operations that must be unwound.
func (s *Store) ListCompensatingOperations(
	ctx context.Context, limit int,
) ([]PaymentOperation, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx,
		`SELECT id FROM commerce_payment_operations
		  WHERE state='COMPENSATING' ORDER BY updated_at, id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	operations := make([]PaymentOperation, 0, len(ids))
	for _, id := range ids {
		operation, loadErr := loadOperationBy(ctx, s.db, `o.id=$1`, id)
		if loadErr != nil {
			return nil, loadErr
		}
		operations = append(operations, operation)
	}
	return operations, nil
}

// RecordLegRefund stores a refund the money authority performed.
//
// The database CHECK keeps refunded plus settled within the leg's own amount,
// so a double refund is refused by the schema rather than only by this code.
func (s *Store) RecordLegRefund(
	ctx context.Context, legID string, amountKobo int64, now time.Time,
) (PaymentOperation, error) {
	if s == nil || s.db == nil {
		return PaymentOperation{}, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PaymentOperation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var operationID, parentOrderID, customerID, previousState, previousLegStatus string
	var legAmountMinor int64
	var alreadyRefunded, alreadySettled int64
	if err = tx.QueryRow(ctx, `
		SELECT l.payment_operation_id,l.parent_order_id,o.customer_id,o.state,l.status,l.amount_minor,l.refunded_minor,l.settled_minor
		  FROM commerce_payment_legs l
		  JOIN commerce_payment_operations o ON o.id=l.payment_operation_id
		 WHERE l.id=$1 FOR UPDATE OF l`, legID).Scan(
		&operationID, &parentOrderID, &customerID, &previousState,
		&previousLegStatus, &legAmountMinor, &alreadyRefunded, &alreadySettled); errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOrderNotFound
	} else if err != nil {
		return PaymentOperation{}, err
	}
	if amountKobo == 0 {
		amountKobo = legAmountMinor
	}

	// Replaying the same refund must not double it.
	if alreadyRefunded >= amountKobo {
		operation, loadErr := loadOperationBy(ctx, tx, `o.id=$1`, operationID)
		if loadErr != nil {
			return PaymentOperation{}, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return PaymentOperation{}, err
		}
		return operation, nil
	}

	newRefunded := alreadyRefunded + amountKobo
	newSettled := alreadySettled
	if newSettled > 0 {
		newSettled -= amountKobo
		if newSettled < 0 {
			newSettled = 0
		}
	}
	if _, err = tx.Exec(ctx, `
		UPDATE commerce_payment_legs
		   SET status='REFUNDED',refunded_minor=$2,settled_minor=$3,version=version+1,updated_at=$4
		 WHERE id=$1`, legID, newRefunded, newSettled, now); err != nil {
		return PaymentOperation{}, err
	}

	if err = appendPaymentEvent(ctx, tx, operationID, legID, parentOrderID, customerID,
		"commerce.payment.compensation_started", "AUTHORITY", "",
		previousLegStatus, string(LegRefunded), "", now); err != nil {
		return PaymentOperation{}, err
	}

	operation, err := loadOperationBy(ctx, tx, `o.id=$1`, operationID)
	if err != nil {
		return PaymentOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PaymentOperation{}, err
	}
	return operation, nil
}

// RecordLegSettled records an authoritative Wallet completion for one vendor
// leg. Replaying it is harmless and cannot settle a different leg.
func (s *Store) RecordLegSettled(ctx context.Context, legID string, amountKobo int64, now time.Time) (PaymentOperation, error) {
	if s == nil || s.db == nil {
		return PaymentOperation{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return PaymentOperation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var operationID, parentOrderID, customerID string
	var already int64
	if err = tx.QueryRow(ctx, `SELECT l.payment_operation_id,l.parent_order_id,o.customer_id,l.settled_minor FROM commerce_payment_legs l JOIN commerce_payment_operations o ON o.id=l.payment_operation_id WHERE l.id=$1 FOR UPDATE`, legID).Scan(&operationID, &parentOrderID, &customerID, &already); errors.Is(err, pgx.ErrNoRows) {
		return PaymentOperation{}, ErrOrderNotFound
	} else if err != nil {
		return PaymentOperation{}, err
	}
	if amountKobo == 0 {
		if err = tx.QueryRow(ctx, `SELECT amount_minor FROM commerce_payment_legs WHERE id=$1`, legID).Scan(&amountKobo); err != nil {
			return PaymentOperation{}, err
		}
	}
	if already >= amountKobo {
		op, e := loadOperationBy(ctx, tx, `o.id=$1`, operationID)
		if e != nil {
			return PaymentOperation{}, e
		}
		if e = tx.Commit(ctx); e != nil {
			return PaymentOperation{}, e
		}
		return op, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_payment_legs SET status='SETTLED',settled_minor=$2,version=version+1,updated_at=$3 WHERE id=$1`, legID, amountKobo, now); err != nil {
		return PaymentOperation{}, err
	}
	if err = appendPaymentEvent(ctx, tx, operationID, legID, parentOrderID, customerID, "commerce.payment.leg_settled", "AUTHORITY", "", string(LegHeld), string(LegSettled), "", now); err != nil {
		return PaymentOperation{}, err
	}
	op, err := loadOperationBy(ctx, tx, `o.id=$1`, operationID)
	if err != nil {
		return PaymentOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PaymentOperation{}, err
	}
	return op, nil
}

// PaymentLegForVendorOrder resolves the isolated leg that belongs to one
// VendorOrder. It is deliberately not a parent-wide lookup.
func (s *Store) PaymentLegForVendorOrder(ctx context.Context, customerID, vendorOrderID string) (PaymentLegRecord, PaymentOperation, error) {
	if s == nil || s.db == nil {
		return PaymentLegRecord{}, PaymentOperation{}, ErrDependencyMissing
	}
	var operationID string
	if err := s.db.QueryRow(ctx, `SELECT l.payment_operation_id FROM commerce_payment_legs l JOIN commerce_payment_operations o ON o.id=l.payment_operation_id WHERE o.customer_id=$1 AND l.vendor_order_id=$2`, customerID, vendorOrderID).Scan(&operationID); errors.Is(err, pgx.ErrNoRows) {
		return PaymentLegRecord{}, PaymentOperation{}, ErrOrderNotFound
	} else if err != nil {
		return PaymentLegRecord{}, PaymentOperation{}, err
	}
	op, err := loadOperationBy(ctx, s.db, `o.id=$1`, operationID)
	if err != nil {
		return PaymentLegRecord{}, PaymentOperation{}, err
	}
	for _, leg := range op.Legs {
		if leg.VendorOrderID == vendorOrderID {
			return leg, op, nil
		}
	}
	return PaymentLegRecord{}, PaymentOperation{}, ErrOrderNotFound
}

type terminalPaymentLeg struct {
	LegID, CustomerID, VendorOrderID string
	Completed, Cancelled             bool
}

func (s *Store) ListTerminalPaymentLegs(ctx context.Context, limit int) ([]terminalPaymentLeg, error) {
	rows, err := s.db.Query(ctx, `SELECT l.id,o.customer_id,l.vendor_order_id,(v.status='COMPLETED'),(v.status IN ('CANCELLED','REJECTED')) FROM commerce_payment_legs l JOIN commerce_payment_operations o ON o.id=l.payment_operation_id JOIN commerce_vendor_orders v ON v.id=l.vendor_order_id WHERE l.scope='VENDOR' AND l.status='HELD' AND v.status IN ('COMPLETED','CANCELLED','REJECTED') ORDER BY l.updated_at,l.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []terminalPaymentLeg
	for rows.Next() {
		var x terminalPaymentLeg
		if err = rows.Scan(&x.LegID, &x.CustomerID, &x.VendorOrderID, &x.Completed, &x.Cancelled); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// MarkOperationCompensated closes out a fully unwound operation.
//
// It refuses while any leg is still held, so an operation can never be
// declared unwound while the customer's money is still sitting in escrow.
func (s *Store) MarkOperationCompensated(
	ctx context.Context, operationID string, now time.Time,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var parentOrderID, customerID, previousState string
	if err = tx.QueryRow(ctx,
		`SELECT parent_order_id,customer_id,state FROM commerce_payment_operations
		  WHERE id=$1 FOR UPDATE`, operationID).Scan(
		&parentOrderID, &customerID, &previousState); err != nil {
		return err
	}

	var stillHeld int
	if err = tx.QueryRow(ctx,
		`SELECT count(*) FROM commerce_payment_legs
		  WHERE payment_operation_id=$1 AND status='HELD'`, operationID).Scan(&stillHeld); err != nil {
		return err
	}
	if stillHeld > 0 {
		return ErrPaymentConflict
	}

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_payment_operations
		   SET state='COMPENSATED',failure_code=CASE WHEN failure_code='' THEN 'PAYMENT_UNWOUND' ELSE failure_code END,
		       terminal_at=COALESCE(terminal_at,$2::timestamptz),version=version+1,updated_at=$2
		 WHERE id=$1`, operationID, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE commerce_parent_orders
		   SET payment_state='COMPENSATED',version=version+1,updated_at=$2
		 WHERE id=$1`, parentOrderID, now); err != nil {
		return err
	}

	if err = appendPaymentEvent(ctx, tx, operationID, "", parentOrderID, customerID,
		"commerce.payment.compensation_completed", "SYSTEM", "",
		previousState, string(PaymentCompensated), "", now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
