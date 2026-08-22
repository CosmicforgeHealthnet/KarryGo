package main

// Section 17 closure, Part 4: immutability proven at the database, on real
// rows.
//
// The migration tests establish that the triggers exist. That is a claim about
// the catalogue. This file establishes what they actually do: a direct UPDATE
// against a persisted change or adjustment — the kind a repair script, a
// mistaken repository method or a compromised process would issue, bypassing
// every Go-level guard — is refused by PostgreSQL itself.
//
// The columns chosen are the ones the customer's agreement rests on. If any of
// them could be edited after the fact, the approved price would no longer be
// evidence of anything.

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// mutating a frozen column must fail. The helper reports the error so each
// case can assert the guard fired rather than the statement merely no-opping.
func mustRefuseUpdate(t *testing.T, fixture section17EFixture, sql string, args ...any) {
	t.Helper()

	_, err := fixture.env.pool.Exec(context.Background(), sql, args...)
	if err == nil {
		t.Fatalf("the database accepted a mutation of frozen evidence:\n%s", sql)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Fatalf("the mutation failed for the wrong reason (%v):\n%s", err, sql)
	}
}

// The agreement a change records — which order, which customer, which quotes,
// and the totals the delta is derived from — cannot be edited once written.
func TestSection17FChange_ApprovedChangeEvidenceCannotBeEdited(t *testing.T) {
	fixture, order, quotes, _ := changeFixture(t, 30_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	approved, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, quotes.fingerprint)
	if err != nil {
		t.Fatalf("ApproveCommercialChange: %v", err)
	}
	if approved.State != orders.ChangeApplied &&
		approved.State != orders.ChangeApproved {
		t.Fatalf("change state after approval = %q", approved.State)
	}

	// Each of these would rewrite what the customer agreed to.
	for name, sql := range map[string]string{
		"the customer it belongs to": `
			UPDATE commerce_order_changes SET customer_id='someone_else' WHERE id=$1`,
		"the order it applies to": `
			UPDATE commerce_order_changes SET parent_order_id='other_order' WHERE id=$1`,
		"the original quote": `
			UPDATE commerce_order_changes SET original_quote_id='other_quote' WHERE id=$1`,
		"the original total": `
			UPDATE commerce_order_changes SET original_total_minor=1 WHERE id=$1`,
		"the replacement quote": `
			UPDATE commerce_order_changes SET replacement_quote_id='other_quote' WHERE id=$1`,
		"the replacement total": `
			UPDATE commerce_order_changes SET replacement_total_minor=1 WHERE id=$1`,
		"the replacement fingerprint": `
			UPDATE commerce_order_changes
			   SET replacement_quote_fingerprint=repeat('a',64) WHERE id=$1`,
		"the delta itself": `
			UPDATE commerce_order_changes SET delta_minor=0 WHERE id=$1`,
		"the currency": `
			UPDATE commerce_order_changes SET currency='USD' WHERE id=$1`,
		"the address it was quoted for": `
			UPDATE commerce_order_changes
			   SET replacement_address_snapshot='{"address":"elsewhere"}'::jsonb WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuseUpdate(t, fixture, sql, change.ID)
		})
	}

	// Deleting it outright is refused too: evidence is append-only, not
	// merely edit-guarded.
	t.Run("deletion", func(t *testing.T) {
		mustRefuseUpdate(t, fixture,
			`DELETE FROM commerce_order_changes WHERE id=$1`, change.ID)
	})
}

// The mutable half must remain mutable, or the change could never progress.
// Freezing the whole row would be as wrong as freezing none of it.
func TestSection17FChange_LifecycleColumnsRemainWritable(t *testing.T) {
	fixture, order, _, _ := changeFixture(t, 30_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)

	if _, err := fixture.env.pool.Exec(ctx, `
		UPDATE commerce_order_changes
		   SET wallet_status='UNKNOWN', version=version+1, updated_at=now()
		 WHERE id=$1`, change.ID); err != nil {
		t.Fatalf("the guard froze a lifecycle column recovery must write: %v", err)
	}
}

// An adjustment records money that moved. Its amounts, its type and the key
// that makes it idempotent are all evidence.
func TestSection17FChange_AdjustmentEvidenceCannotBeEdited(t *testing.T) {
	fixture, order, quotes, _ := changeFixture(t, 40_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, quotes.fingerprint); err != nil {
		t.Fatalf("ApproveCommercialChange: %v", err)
	}

	var adjustmentID string
	if err := fixture.env.pool.QueryRow(ctx, `
		SELECT id FROM commerce_order_adjustments
		 WHERE parent_order_id=$1 ORDER BY created_at LIMIT 1`,
		order.ID).Scan(&adjustmentID); err != nil {
		t.Fatalf("a positive delta recorded no adjustment: %v", err)
	}

	for name, sql := range map[string]string{
		"the amount that moved": `
			UPDATE commerce_order_adjustments SET difference_minor=1 WHERE id=$1`,
		"what it was measured against": `
			UPDATE commerce_order_adjustments SET original_commitment_minor=0 WHERE id=$1`,
		"the eligible final": `
			UPDATE commerce_order_adjustments SET eligible_final_minor=0 WHERE id=$1`,
		"the kind of adjustment": `
			UPDATE commerce_order_adjustments
			   SET adjustment_type='CANCELLATION_REFUND' WHERE id=$1`,
		"the action taken": `
			UPDATE commerce_order_adjustments SET action='REFUND' WHERE id=$1`,
		"the key that makes it exactly-once": `
			UPDATE commerce_order_adjustments SET idempotency_key='reused' WHERE id=$1`,
		"the order it belongs to": `
			UPDATE commerce_order_adjustments SET parent_order_id='other' WHERE id=$1`,
		"the change that caused it": `
			UPDATE commerce_order_adjustments SET change_id=NULL WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuseUpdate(t, fixture, sql, adjustmentID)
		})
	}

	t.Run("deletion", func(t *testing.T) {
		mustRefuseUpdate(t, fixture,
			`DELETE FROM commerce_order_adjustments WHERE id=$1`, adjustmentID)
	})

	// The wallet outcome must stay writable: it is what reconciliation records.
	if _, err := fixture.env.pool.Exec(ctx, `
		UPDATE commerce_order_adjustments
		   SET wallet_status='UNKNOWN', wallet_reference='ref', version=version+1
		 WHERE id=$1`, adjustmentID); err != nil {
		t.Fatalf("the guard froze the wallet outcome recovery must write: %v", err)
	}
}

// The audit trail is wholly append-only: no row may be altered or removed.
func TestSection17FChange_EventTrailIsAppendOnly(t *testing.T) {
	fixture, order, _, _ := changeFixture(t, 20_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)

	var eventID string
	if err := fixture.env.pool.QueryRow(ctx, `
		SELECT id FROM commerce_order_change_events
		 WHERE change_id=$1 ORDER BY occurred_at LIMIT 1`, change.ID).Scan(&eventID); err != nil {
		t.Fatalf("requesting a change recorded no event: %v", err)
	}

	mustRefuseUpdate(t, fixture,
		`UPDATE commerce_order_change_events SET action='commerce.order.change_applied'
		  WHERE id=$1`, eventID)
	mustRefuseUpdate(t, fixture,
		`UPDATE commerce_order_change_events SET actor_type='SYSTEM' WHERE id=$1`, eventID)
	mustRefuseUpdate(t, fixture,
		`DELETE FROM commerce_order_change_events WHERE id=$1`, eventID)
}
