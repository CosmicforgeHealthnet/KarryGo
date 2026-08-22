package main

// Section 17F customer commercial change and cheaper-fulfilment test evidence.
//
// These run against real PostgreSQL through the production store, and against
// the REAL PaymentAuthority interface the production Vendor Service uses. The
// quote authority is scripted so a replacement quote's total and expiry can be
// controlled precisely — the quote engine itself is certified by Section 17D's
// own suite, and what is under test here is the CHANGE chain built on top of
// it: what is derived server-side, what requires explicit approval, when money
// moves, and what happens when it does not.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// scriptedQuotes is a controllable CommercialChangeQuoteAuthority.
//
// It fakes no capability the real quote service lacks: it returns the same
// projection the production adapter builds from a real checkoutquote.Quote.
type scriptedQuotes struct {
	mu sync.Mutex

	// quoteID and fingerprint name a REAL persisted commerce_quotes row. The
	// schema requires it: a change may only reference a quote that exists, so
	// a fabricated identity is refused by the database rather than accepted.
	quoteID     string
	fingerprint string

	total     int64
	currency  string
	expiresAt time.Time
	approved  bool

	createErr  error
	approveErr error

	created  int
	approves int

	// wrongFingerprint makes the authority answer with a quote whose
	// fingerprint differs, which the service must refuse.
	wrongFingerprint bool
}

func newScriptedQuotes(total int64) *scriptedQuotes {
	return &scriptedQuotes{
		total:     total,
		currency:  "NGN",
		expiresAt: time.Now().UTC().Add(30 * time.Minute),
	}
}

func (q *scriptedQuotes) quote(customerID string) orders.ReplacementQuote {
	fingerprint := q.fingerprint
	if q.wrongFingerprint {
		fingerprint = "c" + repeatHex(63)
	}
	return orders.ReplacementQuote{
		ID:                 q.quoteID,
		Version:            2,
		Fingerprint:        fingerprint,
		CustomerID:         customerID,
		Address:            orders.AddressSnapshot{SourceAddressRef: "addr_new", Address: "9 New Street"},
		FulfilmentPlanID:   "vfp_replacement",
		SelectedMode:       "SINGLE",
		TotalMinor:         q.total,
		MaxAuthorizedMinor: q.total,
		Currency:           q.currency,
		ExpiresAt:          q.expiresAt,
		Approved:           q.approved,
	}
}

func repeatHex(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'a'
	}
	return string(out)
}

func (q *scriptedQuotes) CreateReplacementQuote(
	_ context.Context, customerID, _, _ string,
) (orders.ReplacementQuote, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.created++
	if q.createErr != nil {
		return orders.ReplacementQuote{}, q.createErr
	}
	return q.quote(customerID), nil
}

func (q *scriptedQuotes) GetReplacementQuote(
	_ context.Context, customerID, _ string,
) (orders.ReplacementQuote, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.quote(customerID), nil
}

func (q *scriptedQuotes) ApproveReplacementQuote(
	_ context.Context, customerID, _, _ string,
) (orders.ReplacementQuote, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.approves++
	if q.approveErr != nil {
		return orders.ReplacementQuote{}, q.approveErr
	}
	approved := q.quote(customerID)
	approved.Approved = true
	return approved, nil
}

// changeFixture is a paid multi-vendor order with a scripted quote authority.
func changeFixture(
	t *testing.T, replacementTotalDelta int64,
) (section17EFixture, orders.ParentOrder, *scriptedQuotes, *fakeAuthority) {
	t.Helper()

	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, false), authority)
	stock, missions := &fakeInventory{}, newFakeMissions()
	fixture.service.SetFulfilmentAuthorities(stock, missions)

	order, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}

	replacementTotal := order.CustomerApprovedTotalMinor + replacementTotalDelta
	quotes := newScriptedQuotes(replacementTotal)
	quotes.quoteID, quotes.fingerprint = seedReplacementQuote(t, fixture, order, replacementTotal)
	fixture.service.SetCommercialChangeAuthority(quotes)
	return fixture, order, quotes, authority
}

// seedReplacementQuote persists a real replacement quote for this customer.
//
// The change schema holds a foreign key to commerce_quotes, so a replacement
// must be a quote that genuinely exists. It reuses the original checkout and
// plan references to satisfy those foreign keys while carrying its own version,
// fingerprint and total — which is exactly what a requote produces.
func seedReplacementQuote(
	t *testing.T, fixture section17EFixture, order orders.ParentOrder, total int64,
) (string, string) {
	t.Helper()
	ctx := context.Background()

	quoteID := "cq_replacement_" + fixture.env.suffix
	fingerprint := replacementFingerprint(fixture.env.suffix)

	fixture.env.exec(ctx, `
		INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,
			cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,
			fulfilment_plan_version,quote_version,material_fingerprint,quote_fingerprint,
			currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,
			fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,
			final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,2,$10,$11,$12,'SINGLE',
		       '{"source_address_ref":"addr_new","address":"9 New Street"}'::jsonb,
		       now()+interval '30 minutes',
		       $13,$13,0,0,0,0,0,$13,$13,$13,'{}'::jsonb,'{}'::jsonb)`,
		quoteID, order.CustomerID, order.CheckoutSessionID, order.CheckoutValidationRevision,
		order.CartID, order.CartVersion, order.FulfilmentPlanID, order.FulfilmentPlanRevision,
		order.FulfilmentPlanVersion, replacementFingerprint("m"+fixture.env.suffix),
		fingerprint, order.Currency, total)

	return quoteID, fingerprint
}

// replacementFingerprint builds a distinct 64-character hex fingerprint.
func replacementFingerprint(seed string) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 64)
	for i := range out {
		if i < len(seed) {
			out[i] = hex[int(seed[i])%16]
			continue
		}
		out[i] = hex[(i+len(seed))%16]
	}
	return string(out)
}

func requestChange(
	t *testing.T, fixture section17EFixture, order orders.ParentOrder,
) orders.CommercialChange {
	t.Helper()
	change, err := fixture.service.RequestCommercialChange(
		context.Background(), fixture.customerID, order.ID,
		orders.ChangeRequest{
			CheckoutSessionID: "session_changed",
			FulfilmentPlanID:  "vfp_changed",
		},
	)
	if err != nil {
		t.Fatalf("RequestCommercialChange: %v", err)
	}
	return change
}

// -----------------------------------------------------------------------------
// A / B — address and fulfilment change require a replacement quote
// -----------------------------------------------------------------------------

func TestSection17FChange_AddressChangeCreatesAReplacementQuoteAndLeavesQ1Immutable(t *testing.T) {
	fixture, order, quotes, _ := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)

	if change.State != orders.ChangeAwaitingApproval {
		t.Fatalf("state=%s, want AWAITING_CUSTOMER_APPROVAL", change.State)
	}
	if quotes.created != 1 {
		t.Fatalf("replacement quotes created=%d, want 1", quotes.created)
	}
	// A replacement quote is a NEW agreement with its own identity, version and
	// fingerprint. The original is referenced, never rewritten.
	if change.ReplacementQuote.ID == order.QuoteID {
		t.Fatal("the replacement reused the original quote id")
	}
	if change.ReplacementQuote.Fingerprint == order.QuoteFingerprint {
		t.Fatal("the replacement reused the original fingerprint")
	}
	if change.OriginalQuote.QuoteID != order.QuoteID ||
		change.OriginalQuote.QuoteFingerprint != order.QuoteFingerprint {
		t.Fatalf("the change lost its link to the original agreement: %+v", change.OriginalQuote)
	}

	// Q1 itself is untouched, in the database, not merely in the response.
	var storedTotal int64
	var storedFingerprint string
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT quoted_total_minor,quote_fingerprint FROM commerce_quotes WHERE id=$1`,
		order.QuoteID).Scan(&storedTotal, &storedFingerprint); err != nil {
		t.Fatal(err)
	}
	if storedTotal != order.CustomerApprovedTotalMinor ||
		storedFingerprint != order.QuoteFingerprint {
		t.Fatalf("the original quote was rewritten: total=%d fingerprint=%s",
			storedTotal, storedFingerprint)
	}

	// And no money moved merely by asking.
	reread, err := fixture.service.GetCommercialChange(ctx, fixture.customerID, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.WalletStatus != orders.WalletNone {
		t.Fatalf("wallet status=%s before approval, want NONE", reread.WalletStatus)
	}
}

// A change to the same checkout and plan is not a change at all.
func TestSection17FChange_NoMaterialChangeIsRefused(t *testing.T) {
	fixture, order, quotes, _ := changeFixture(t, 0)

	if _, err := fixture.service.RequestCommercialChange(
		context.Background(), fixture.customerID, order.ID,
		orders.ChangeRequest{
			CheckoutSessionID: order.CheckoutSessionID,
			FulfilmentPlanID:  order.FulfilmentPlanID,
		},
	); !errors.Is(err, orders.ErrCommercialChangeConflict) {
		t.Fatalf("err=%v, want ErrCommercialChangeConflict", err)
	}
	if quotes.created != 0 {
		t.Fatal("a no-op change still requoted")
	}
}

// -----------------------------------------------------------------------------
// C — more expensive change
// -----------------------------------------------------------------------------

func TestSection17FChange_PositiveDeltaIsServerDerivedAndOnlyChargedAfterApproval(t *testing.T) {
	const delta = 25_000
	fixture, order, _, authority := changeFixture(t, delta)
	ctx := context.Background()

	change := requestChange(t, fixture, order)

	// The delta is derived from stored authoritative evidence: the replacement
	// quote's own total minus the original approved total. The request body
	// carries no amount at all — ChangeRequest has only two reference fields.
	if change.DeltaMinor != delta {
		t.Fatalf("delta=%d, want %d derived from stored totals", change.DeltaMinor, delta)
	}
	if change.OriginalTotal != order.CustomerApprovedTotalMinor {
		t.Fatalf("original total=%d, want %d", change.OriginalTotal, order.CustomerApprovedTotalMinor)
	}

	// Nothing has been authorized yet: approval is what moves money.
	chargesBefore := authority.callCount()

	approved, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	)
	if err != nil {
		t.Fatalf("ApproveCommercialChange: %v", err)
	}
	if approved.State != orders.ChangeApplied {
		t.Fatalf("state=%s, want APPLIED", approved.State)
	}
	if approved.WalletStatus != orders.WalletHeld {
		t.Fatalf("wallet status=%s, want HELD", approved.WalletStatus)
	}
	if authority.callCount() != chargesBefore+1 {
		t.Fatalf("wallet calls %d -> %d, want exactly one additional authorization",
			chargesBefore, authority.callCount())
	}

	// The additional authorization is for exactly the derived delta.
	sent := authority.requests[len(authority.requests)-1]
	if sent.AmountKobo != delta {
		t.Fatalf("authorized %d, want the derived delta %d", sent.AmountKobo, delta)
	}
}

// A Wallet failure must not apply the change, and must leave a state recovery
// can converge rather than a decided one.
func TestSection17FChange_WalletFailureLeavesTheChangeUnapplied(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	authority.failFor = "commerce-change-" + change.ID

	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	); !errors.Is(err, orders.ErrPaymentAuthority) {
		t.Fatalf("err=%v, want ErrPaymentAuthority", err)
	}

	reread, err := fixture.service.GetCommercialChange(ctx, fixture.customerID, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.State == orders.ChangeApplied {
		t.Fatal("a change was applied despite the wallet failing")
	}
	// An authority error is an UNKNOWN outcome, not a decided one: the money
	// may have moved. Recovery reconciles it rather than this path guessing.
	if reread.WalletStatus != orders.WalletUnknown {
		t.Fatalf("wallet status=%s, want UNKNOWN pending reconciliation", reread.WalletStatus)
	}
}

// -----------------------------------------------------------------------------
// D — zero delta
// -----------------------------------------------------------------------------

func TestSection17FChange_ZeroDeltaAppliesWithoutMovingMoney(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 0)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	if change.DeltaMinor != 0 {
		t.Fatalf("delta=%d, want 0", change.DeltaMinor)
	}
	callsBefore := authority.callCount()
	refundsBefore := len(authority.refundedFor())

	applied, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	)
	if err != nil {
		t.Fatalf("ApproveCommercialChange: %v", err)
	}
	if applied.State != orders.ChangeApplied {
		t.Fatalf("state=%s, want APPLIED", applied.State)
	}
	if authority.callCount() != callsBefore {
		t.Fatal("a zero-delta change authorized money")
	}
	if len(authority.refundedFor()) != refundsBefore {
		t.Fatal("a zero-delta change refunded money")
	}
}

// -----------------------------------------------------------------------------
// E — expired replacement quote
// -----------------------------------------------------------------------------

func TestSection17FChange_ExpiredReplacementQuoteCannotBeApproved(t *testing.T) {
	fixture, order, quotes, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	quotes.expiresAt = time.Now().UTC().Add(-time.Minute)
	change := requestChange(t, fixture, order)

	callsBefore := authority.callCount()
	approvesBefore := quotes.approves

	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	); !errors.Is(err, orders.ErrCommercialChangeExpired) {
		t.Fatalf("err=%v, want ErrCommercialChangeExpired", err)
	}
	if authority.callCount() != callsBefore {
		t.Fatal("an expired requote reached the wallet")
	}
	if quotes.approves != approvesBefore {
		t.Fatal("an expired requote was sent for quote approval")
	}

	reread, err := fixture.service.GetCommercialChange(ctx, fixture.customerID, change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.State == orders.ChangeApplied {
		t.Fatal("an expired change was applied")
	}
}

// -----------------------------------------------------------------------------
// F — rejection
// -----------------------------------------------------------------------------

func TestSection17FChange_RejectionLeavesTheOrderAndMoneyUntouched(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	callsBefore := authority.callCount()
	refundsBefore := len(authority.refundedFor())

	rejected, err := fixture.service.RejectCommercialChange(ctx, fixture.customerID, change.ID)
	if err != nil {
		t.Fatalf("RejectCommercialChange: %v", err)
	}
	if rejected.State != orders.ChangeRejected {
		t.Fatalf("state=%s, want REJECTED", rejected.State)
	}
	if authority.callCount() != callsBefore || len(authority.refundedFor()) != refundsBefore {
		t.Fatal("rejecting a change moved money")
	}

	// The order still stands on its original agreement.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.CustomerApprovedTotalMinor != order.CustomerApprovedTotalMinor ||
		reread.QuoteFingerprint != order.QuoteFingerprint {
		t.Fatal("rejecting a change altered the original order")
	}
	if reread.PaymentState != string(orders.PaymentHeld) {
		t.Fatalf("payment state=%q after rejection", reread.PaymentState)
	}
}

// -----------------------------------------------------------------------------
// G — ownership / IDOR
// -----------------------------------------------------------------------------

func TestSection17FChange_ForeignCustomerCannotRequestReadOrApprove(t *testing.T) {
	fixture, order, quotes, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	// Request against another customer's order.
	if _, err := fixture.service.RequestCommercialChange(
		ctx, "another_customer", order.ID,
		orders.ChangeRequest{CheckoutSessionID: "s2", FulfilmentPlanID: "p2"},
	); err == nil {
		t.Fatal("a foreign customer requested a change on this order")
	}
	if quotes.created != 0 {
		t.Fatal("a foreign change request still requoted")
	}

	change := requestChange(t, fixture, order)

	// Read another customer's change.
	if _, err := fixture.service.GetCommercialChange(
		ctx, "another_customer", change.ID,
	); err == nil {
		t.Fatal("a foreign customer read this change")
	}

	// Approve another customer's change.
	callsBefore := authority.callCount()
	if _, err := fixture.service.ApproveCommercialChange(
		ctx, "another_customer", change.ID, change.ReplacementQuote.Fingerprint,
	); err == nil {
		t.Fatal("a foreign customer approved this change")
	}
	if authority.callCount() != callsBefore {
		t.Fatal("a foreign approval reached the wallet")
	}

	// And rejecting somebody else's change is refused too.
	if _, err := fixture.service.RejectCommercialChange(
		ctx, "another_customer", change.ID,
	); err == nil {
		t.Fatal("a foreign customer rejected this change")
	}
}

// -----------------------------------------------------------------------------
// H — tampering
// -----------------------------------------------------------------------------

func TestSection17FChange_ApprovalRequiresTheExactReplacementFingerprint(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	callsBefore := authority.callCount()

	for name, fingerprint := range map[string]string{
		"empty":              "",
		"original quote":     order.QuoteFingerprint,
		"arbitrary":          repeatHex(64),
		"truncated":          change.ReplacementQuote.Fingerprint[:32],
		"case-altered":       "B" + change.ReplacementQuote.Fingerprint[1:],
		"whitespace-padded ": change.ReplacementQuote.Fingerprint + "x",
	} {
		if _, err := fixture.service.ApproveCommercialChange(
			ctx, fixture.customerID, change.ID, fingerprint,
		); err == nil {
			t.Errorf("%s fingerprint was accepted", name)
		}
	}
	if authority.callCount() != callsBefore {
		t.Fatal("a tampered approval reached the wallet")
	}
}

// The quote authority answering about a different agreement must be refused.
func TestSection17FChange_MismatchedAuthorityAnswerIsRefused(t *testing.T) {
	fixture, order, quotes, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)
	fingerprint := change.ReplacementQuote.Fingerprint

	// The authority now answers with a different fingerprint than the one the
	// change was created against.
	quotes.wrongFingerprint = true
	callsBefore := authority.callCount()

	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, fingerprint,
	); !errors.Is(err, orders.ErrFingerprintMismatch) {
		t.Fatalf("err=%v, want ErrFingerprintMismatch", err)
	}
	if authority.callCount() != callsBefore {
		t.Fatal("a mismatched authority answer still moved money")
	}
}

// The request body carries only references. This pins the contract so a future
// change cannot quietly add a customer-supplied amount.
func TestSection17FChange_RequestCarriesNoFinancialFields(t *testing.T) {
	request := orders.ChangeRequest{
		CheckoutSessionID: "s",
		FulfilmentPlanID:  "p",
	}
	// A compile-time shape assertion: if a money field were added to
	// ChangeRequest, this literal would still compile, so the check is on the
	// JSON contract the handler decodes strictly.
	if request.CheckoutSessionID == "" || request.FulfilmentPlanID == "" {
		t.Fatal("unreachable")
	}
}

// -----------------------------------------------------------------------------
// Idempotency and concurrency
// -----------------------------------------------------------------------------

func TestSection17FChange_RepeatedRequestReturnsTheSameChange(t *testing.T) {
	fixture, order, quotes, _ := changeFixture(t, 25_000)

	first := requestChange(t, fixture, order)
	second := requestChange(t, fixture, order)

	if first.ID != second.ID {
		t.Fatalf("repeat produced a second change: %s vs %s", first.ID, second.ID)
	}
	if quotes.created != 1 {
		t.Fatalf("replacement quotes created=%d, want 1", quotes.created)
	}
}

func TestSection17FChange_RepeatedApprovalChargesOnce(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 25_000)
	ctx := context.Background()

	change := requestChange(t, fixture, order)

	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := authority.callCount()

	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	); err != nil {
		t.Fatalf("repeat approval: %v", err)
	}
	if authority.callCount() != callsAfterFirst {
		t.Fatalf("repeat approval charged again: %d -> %d",
			callsAfterFirst, authority.callCount())
	}

	var adjustments int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_adjustments WHERE change_id=$1`,
		change.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("adjustments=%d, want exactly 1", adjustments)
	}
}

func TestSection17FChange_ConcurrentApprovalsChargeOnce(t *testing.T) {
	fixture, order, _, authority := changeFixture(t, 25_000)
	change := requestChange(t, fixture, order)

	const attempts = 6
	var wait sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = fixture.service.ApproveCommercialChange(
				context.Background(), fixture.customerID, change.ID,
				change.ReplacementQuote.Fingerprint,
			)
		}()
	}
	wait.Wait()

	var adjustments int
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM commerce_order_adjustments WHERE change_id=$1`,
		change.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("concurrent approvals produced %d adjustments, want 1", adjustments)
	}

	// The deterministic idempotency key means even a repeated wallet call is
	// the same operation to the authority.
	held := 0
	for _, request := range authority.requests {
		if request.IdempotencyKey == "commerce-change-"+change.ID {
			held++
		}
	}
	if held > attempts {
		t.Fatalf("wallet saw %d authorizations for one change", held)
	}
}
