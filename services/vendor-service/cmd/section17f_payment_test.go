package main

// Section 17F payment-binding integration tests.
//
// The money authority is a scriptable fake rather than a live
// payment-wallet-service, because what is under test here is the BINDING and
// the durability around it: which legs exist, what they are bound to, what is
// recorded before a call is made, and what happens when the authority refuses.
// The adapter that speaks the real Wallet contract is exercised separately.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// fakeAuthority records every request and answers from a script.
type fakeAuthority struct {
	mu sync.Mutex

	configured bool
	requests   []orders.HoldLegRequest

	// failFor makes one specific idempotency key refuse, so a partial
	// authorization can be produced deterministically.
	failFor string

	// answer overrides the status returned for a key.
	answer map[string]string

	// readAnswer overrides what a recovery read reports for a reference, so a
	// crash whose true outcome differs from the local record can be staged.
	readAnswer map[string]string
	reads      []string
	readFails  bool

	refunds     []orders.RefundLegRequest
	refundFails bool
}

func newFakeAuthority() *fakeAuthority {
	return &fakeAuthority{
		configured: true,
		answer:     map[string]string{},
		readAnswer: map[string]string{},
	}
}

func (a *fakeAuthority) Configured() bool { return a.configured }

func (a *fakeAuthority) PaymentCapability(context.Context, string, string) (orders.PaymentCapability, error) {
	return orders.PaymentCapAndRefund, nil
}

func (a *fakeAuthority) HoldLeg(
	_ context.Context, request orders.HoldLegRequest,
) (orders.LegResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, request)

	if a.failFor != "" && request.IdempotencyKey == a.failFor {
		return orders.LegResult{}, errors.New("wallet refused: insufficient balance")
	}
	status := "held"
	if override, ok := a.answer[request.IdempotencyKey]; ok {
		status = override
	}
	return orders.LegResult{
		IntentID:   "pi_" + request.IdempotencyKey,
		Reference:  "ref_" + request.IdempotencyKey,
		Status:     status,
		AmountKobo: request.AmountKobo,
	}, nil
}

func (a *fakeAuthority) GetLeg(_ context.Context, reference string) (orders.LegResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reads = append(a.reads, reference)
	status := "held"
	if override, ok := a.readAnswer[reference]; ok {
		status = override
	}
	if a.readFails {
		return orders.LegResult{}, errors.New("wallet unreachable")
	}
	return orders.LegResult{IntentID: "pi", Reference: reference, Status: status}, nil
}

func (a *fakeAuthority) RefundLeg(
	_ context.Context, request orders.RefundLegRequest,
) (orders.RefundResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refundFails {
		return orders.RefundResult{}, errors.New("wallet refused the refund")
	}
	a.refunds = append(a.refunds, request)
	return orders.RefundResult{
		Reference:  "rf_" + request.IdempotencyKey,
		Status:     "processed",
		AmountKobo: request.AmountKobo,
	}, nil
}

func (a *fakeAuthority) refundedFor() []orders.RefundLegRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]orders.RefundLegRequest(nil), a.refunds...)
}

func (a *fakeAuthority) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

// withPayment attaches a fake money authority to an existing Section 17E
// fixture.
func withPayment(fixture section17EFixture, authority *fakeAuthority) section17EFixture {
	fixture.service.SetPaymentAuthority(orders.PaymentOptions{Authority: authority})
	return fixture
}

func TestSection17FPayment_BindsOneLegPerVendorOrder(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, operation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}

	if len(operation.Legs) != 2 {
		t.Fatalf("legs=%d, want one per vendor order", len(operation.Legs))
	}
	if operation.State != orders.PaymentHeld {
		t.Fatalf("state=%s, want HELD", operation.State)
	}
	if order.PaymentState != string(orders.PaymentHeld) {
		t.Fatalf("order payment state=%q, want HELD mirrored", order.PaymentState)
	}

	// The binding carries the whole approved agreement.
	if operation.Binding.QuoteFingerprint != fixture.finger ||
		operation.Binding.QuotedTotalMinor != order.CustomerApprovedTotalMinor ||
		operation.Binding.Currency != order.Currency ||
		len(operation.Binding.AllocationFingerprint) != 64 {
		t.Fatalf("binding incomplete: %+v", operation.Binding)
	}

	// Each leg pays exactly its own vendor's share and names that vendor as the
	// beneficiary. No leg can be used to pay another vendor.
	byVendorOrder := map[string]orders.PaymentLegRecord{}
	for _, leg := range operation.Legs {
		byVendorOrder[leg.VendorOrderID] = leg
	}
	var total int64
	for _, vendorOrder := range order.VendorOrders {
		leg, found := byVendorOrder[vendorOrder.ID]
		if !found {
			t.Fatalf("no leg for vendor order %s", vendorOrder.ID)
		}
		if leg.AmountMinor != vendorOrder.CustomerPayableMinor {
			t.Errorf("%s leg=%d, want %d",
				vendorOrder.BusinessID, leg.AmountMinor, vendorOrder.CustomerPayableMinor)
		}
		if leg.BusinessID != vendorOrder.BusinessID {
			t.Errorf("leg for %s names business %s",
				vendorOrder.BusinessID, leg.BusinessID)
		}
		if leg.Status != orders.LegHeld || leg.IntentReference == "" {
			t.Errorf("leg not held with authority evidence: %+v", leg)
		}
		total += leg.AmountMinor
	}
	if total != order.CustomerApprovedTotalMinor {
		t.Fatalf("legs total %d, approved total %d", total, order.CustomerApprovedTotalMinor)
	}

	// The authority was asked to settle each leg to its own vendor.
	for _, request := range authority.requests {
		if request.ProviderType != orders.PaymentProviderTypeVendor {
			t.Errorf("vendor leg provider type=%q", request.ProviderType)
		}
		if request.ProviderID == "" {
			t.Error("vendor leg names no beneficiary")
		}
	}
}

func TestSection19VariableFinalizationUsesCapAndReconcilesVendorSettlement(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()
	parent, _, err := fixture.service.CreateFromQuote(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatalf("CreateFromQuote: %v", err)
	}
	vendor := parent.VendorOrders[0]
	line := vendor.Lines[0]
	unitPrice := line.QuotedTotalMinor / 100
	maxAmount := unitPrice * 110
	finalAmount := unitPrice * 105
	capDelta := maxAmount - line.QuotedTotalMinor
	// This fixture starts as a fixed line; the durable measurement row is the
	// only new evidence needed to exercise finalization, while the immutable
	// quote line remains untouched.
	fixture.env.exec(ctx, `UPDATE commerce_parent_orders SET max_authorized_total_minor=$1 WHERE id=$2`, parent.CustomerApprovedTotalMinor+capDelta, parent.ID)
	fixture.env.exec(ctx, `UPDATE commerce_vendor_orders SET max_product_subtotal_minor=max_product_subtotal_minor+$2 WHERE id=$1`, vendor.ID, capDelta)
	fixture.env.exec(ctx, `INSERT INTO commerce_variable_measurements(id,parent_order_id,vendor_order_id,order_line_id,measurement_mode,estimated_quantity,unit_price_minor,estimated_amount_minor,tolerance_quantity,hard_max_quantity,reserved_upper_quantity,actual_quantity,final_amount_minor,financial_state,wallet_capability_snapshot,version) VALUES($1,$2,$3,$4,'VARIABLE_COUNT',100,$5,$6,20,110,110,105,$7,'ACTUAL_RECORDED','CAP_AND_REFUND',1)`, "cvm_section19_"+fixture.env.suffix, parent.ID, vendor.ID, line.ID, unitPrice, line.QuotedTotalMinor, finalAmount)
	order, operation, err := fixture.service.StartPayment(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	fixture.env.exec(ctx, `INSERT INTO commerce_payment_capability_snapshots(id,parent_order_id,payment_operation_ref,capability) VALUES($1,$2,$3,'CAP_AND_REFUND') ON CONFLICT(parent_order_id,payment_operation_ref) DO NOTHING`, "pcap_section19_"+fixture.env.suffix, order.ID, operation.ID)
	fixture.env.exec(ctx, `UPDATE commerce_payment_legs SET amount_minor=$1 WHERE payment_operation_id=$2 AND vendor_order_id=$3`, maxAmount, operation.ID, vendor.ID)
	finalized, err := fixture.service.FinalizeVariableMeasurement(ctx, fixture.actor(fixture.staffA, fixture.env.businessA), vendor.ID, line.ID, 1)
	if err != nil {
		t.Fatalf("FinalizeVariableMeasurement: %v", err)
	}
	if finalized.State != orders.FinancialFinalized || finalized.FinalAmountMinor != finalAmount {
		t.Fatalf("finalized=%+v", finalized)
	}
	refunds := authority.refundedFor()
	if len(refunds) != 1 || refunds[0].AmountKobo != maxAmount-finalAmount {
		t.Fatalf("refunds=%+v, want one cap release of %d", refunds, maxAmount-finalAmount)
	}
	var customerPayable, vendorPayable int64
	if err = fixture.env.pool.QueryRow(ctx, `SELECT customer_payable_minor,vendor_payable_minor FROM commerce_vendor_orders WHERE id=$1`, vendor.ID).Scan(&customerPayable, &vendorPayable); err != nil {
		t.Fatal(err)
	}
	if customerPayable != vendor.CustomerPayableMinor+(finalAmount-line.QuotedTotalMinor) || vendorPayable != vendor.VendorPayableMinor+(finalAmount-line.QuotedTotalMinor) {
		t.Fatalf("settlement customer=%d vendor=%d want=%d/%d", customerPayable, vendorPayable, vendor.CustomerPayableMinor+(finalAmount-line.QuotedTotalMinor), vendor.VendorPayableMinor+(finalAmount-line.QuotedTotalMinor))
	}
	receipt, err := orders.NewStore(fixture.env.pool).FinalReceiptForCustomer(ctx, fixture.customerID, vendor.ID)
	if err != nil {
		t.Fatalf("FinalReceiptForCustomer: %v", err)
	}
	if len(receipt) != 1 || receipt[0].OriginalAmountMinor != line.QuotedTotalMinor || receipt[0].EstimatedAmountMinor != line.QuotedTotalMinor || receipt[0].FinalAmountMinor != finalAmount || receipt[0].CustomerPayableMinor != customerPayable || receipt[0].VendorPayableMinor != vendorPayable {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestSection19VariableFinalizationParentCapRaceRequiresReapproval(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection19VariableFixture(t, true), authority)
	ctx := context.Background()
	parent, _, err := fixture.service.CreateFromQuote(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatalf("CreateFromQuote: %v", err)
	}
	var holdDelta, finalDelta int64
	for _, vendor := range parent.VendorOrders {
		delta := vendor.Lines[0].MaxAuthorizedTotalMinor - vendor.Lines[0].QuotedTotalMinor
		actualDelta := (vendor.Lines[0].QuotedTotalMinor / 100) * 10
		holdDelta += delta
		if actualDelta > finalDelta {
			finalDelta = actualDelta
		}
	}
	fixture.env.exec(ctx, `UPDATE commerce_parent_orders SET max_authorized_total_minor=customer_approved_total_minor+$2 WHERE id=$1`, parent.ID, holdDelta)
	for _, vendor := range parent.VendorOrders {
		line := vendor.Lines[0]
		unitPrice := line.QuotedTotalMinor / 100
		finalAmount := unitPrice * 110
		fixture.env.exec(ctx, `UPDATE commerce_vendor_orders SET max_product_subtotal_minor=max_product_subtotal_minor+$2 WHERE id=$1`, vendor.ID, finalAmount-line.QuotedTotalMinor)
		fixture.env.exec(ctx, `INSERT INTO commerce_variable_measurements(id,parent_order_id,vendor_order_id,order_line_id,measurement_mode,estimated_quantity,unit_price_minor,estimated_amount_minor,tolerance_quantity,hard_max_quantity,reserved_upper_quantity,actual_quantity,final_amount_minor,financial_state,wallet_capability_snapshot,version) VALUES($1,$2,$3,$4,'VARIABLE_COUNT',100,$5,$6,20,110,110,110,$7,'ACTUAL_RECORDED','CAP_AND_REFUND',1)`, "cvm_race_"+vendor.ID, parent.ID, vendor.ID, line.ID, unitPrice, line.QuotedTotalMinor, finalAmount)
	}
	order, operation, err := fixture.service.StartPayment(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	fixture.env.exec(ctx, `INSERT INTO commerce_payment_capability_snapshots(id,parent_order_id,payment_operation_ref,capability) VALUES($1,$2,$3,'CAP_AND_REFUND') ON CONFLICT(parent_order_id,payment_operation_ref) DO NOTHING`, "pcap_race_"+fixture.env.suffix, order.ID, operation.ID)
	// The authorization was started against the complete frozen hold. The
	// finalization race is intentionally evaluated against a tighter parent
	// authority so one of the two actuals must request reapproval.
	fixture.env.exec(ctx, `UPDATE commerce_parent_orders SET max_authorized_total_minor=customer_approved_total_minor+$2 WHERE id=$1`, parent.ID, finalDelta)
	var currentMax, approved int64
	if err = fixture.env.pool.QueryRow(ctx, `SELECT max_authorized_total_minor,customer_approved_total_minor FROM commerce_parent_orders WHERE id=$1`, parent.ID).Scan(&currentMax, &approved); err != nil {
		t.Fatal(err)
	}
	t.Logf("parent approved=%d max=%d finalDelta=%d holdDelta=%d", approved, currentMax, finalDelta, holdDelta)
	for _, leg := range operation.Legs {
		if leg.VendorOrderID == "" {
			continue
		}
		fixture.env.exec(ctx, `UPDATE commerce_payment_legs SET amount_minor=(SELECT hard_max_quantity*unit_price_minor FROM commerce_variable_measurements WHERE vendor_order_id=$1) WHERE payment_operation_id=$2 AND vendor_order_id=$1`, leg.VendorOrderID, operation.ID)
	}
	results := make(chan error, len(parent.VendorOrders))
	var wg sync.WaitGroup
	for i, vendor := range parent.VendorOrders {
		wg.Add(1)
		go func(i int, vendor orders.VendorOrder) {
			defer wg.Done()
			results <- func() error {
				_, err := fixture.service.FinalizeVariableMeasurement(ctx, fixture.actor([]string{fixture.staffA, fixture.staffB}[i], vendor.BusinessID), vendor.ID, vendor.Lines[0].ID, 1)
				return err
			}()
		}(i, vendor)
	}
	wg.Wait()
	close(results)
	finalized, reapproval := 0, 0
	for err := range results {
		if err == nil {
			finalized++
		} else if errors.Is(err, orders.ErrCommercialChangeRequired) {
			reapproval++
		} else {
			t.Fatalf("parent race error=%v", err)
		}
	}
	if finalized != 1 || reapproval != 1 {
		t.Fatalf("finalized=%d reapproval=%d, want one each", finalized, reapproval)
	}
}

// A retry must not create a second operation, a second leg or a second
// financial effect.
func TestSection17FPayment_RetryIsIdempotent(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	first, firstOperation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := authority.callCount()

	second, secondOperation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("retry created a second order: %s vs %s", first.ID, second.ID)
	}
	if secondOperation.ID != firstOperation.ID {
		t.Fatalf("retry created a second payment operation: %s vs %s",
			firstOperation.ID, secondOperation.ID)
	}
	// Every leg already answered, so the authority is not asked again at all.
	if authority.callCount() != callsAfterFirst {
		t.Fatalf("retry called the money authority again: %d -> %d",
			callsAfterFirst, authority.callCount())
	}

	for table, query := range map[string]string{
		"operations": `SELECT count(*) FROM commerce_payment_operations WHERE parent_order_id=$1`,
		"legs":       `SELECT count(*) FROM commerce_payment_legs WHERE parent_order_id=$1`,
	} {
		var count int
		if err = fixture.env.pool.QueryRow(ctx, query, first.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := 1
		if table == "legs" {
			want = 2
		}
		if count != want {
			t.Errorf("%s=%d, want %d after a retry", table, count, want)
		}
	}
}

func TestSection17FPayment_ConcurrentStartsConvergeOnOneOperation(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)

	const attempts = 8
	ids := make(chan string, attempts)
	var wait sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, operation, err := fixture.service.StartPayment(
				context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
			)
			if err == nil {
				ids <- operation.ID
			}
		}()
	}
	wait.Wait()
	close(ids)

	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		} else if id != expected {
			t.Errorf("concurrent starts produced %q and %q", expected, id)
		}
	}

	var rows int
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM commerce_payment_operations WHERE quote_id=$1`,
		fixture.quoteID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("payment operations=%d err=%v", rows, err)
	}
}

// Payment may not start from an expired quote, and the refusal must happen
// before any money is asked for.
func TestSection17FPayment_ExpiredQuoteCannotStartPayment(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	// The quote row is immutable by design, so expiry is simulated the way it
	// really happens: the checkout evidence underneath it lapses.
	fixture.env.exec(ctx,
		`UPDATE commerce_checkout_sessions SET expires_at=now()-interval '1 minute'
		  WHERE customer_id=$1`, fixture.customerID)

	if _, _, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); !errors.Is(err, orders.ErrQuoteNotOrderable) {
		t.Fatalf("err=%v, want ErrQuoteNotOrderable", err)
	}
	if authority.callCount() != 0 {
		t.Fatal("an expired quote reached the money authority")
	}
}

func TestSection17FPayment_FingerprintMismatchIsRefusedBeforeAnyCharge(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)

	if _, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID,
		"0000000000000000000000000000000000000000000000000000000000000000",
	); !errors.Is(err, orders.ErrFingerprintMismatch) {
		t.Fatalf("err=%v, want ErrFingerprintMismatch", err)
	}
	if authority.callCount() != 0 {
		t.Fatal("a mismatched fingerprint reached the money authority")
	}
}

// A customer cannot pay against somebody else's quote.
func TestSection17FPayment_QuoteSwapIsRefused(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)

	if _, _, err := fixture.service.StartPayment(
		context.Background(), "another_customer", fixture.quoteID, fixture.finger,
	); err == nil {
		t.Fatal("a foreign customer started payment on this quote")
	}
	if authority.callCount() != 0 {
		t.Fatal("a quote swap reached the money authority")
	}
}

// An operation already bound to one agreement may not be reused for another.
func TestSection17FPayment_ReboundAllocationIsRefused(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, _, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Move one vendor's share to another without changing the total. The
	// allocation binding must catch it even though every other field matches.
	fixture.env.exec(ctx, `
		UPDATE commerce_vendor_orders
		   SET customer_payable_minor = customer_payable_minor + 1
		 WHERE parent_order_id=$1 AND business_id=$2`, order.ID, fixture.env.businessA)
	fixture.env.exec(ctx, `
		UPDATE commerce_vendor_orders
		   SET customer_payable_minor = customer_payable_minor - 1
		 WHERE parent_order_id=$1 AND business_id=$2`, order.ID, fixture.env.businessB)

	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); !errors.Is(err, orders.ErrBindingMismatch) {
		t.Fatalf("err=%v, want ErrBindingMismatch", err)
	}
}

// The dangerous middle: one leg held, another refused. The operation must
// report the truth and unwind rather than half-succeeding.
func TestSection17FPayment_PartialAuthorizationUnwindsRatherThanHalfSucceeding(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Refuse the second vendor's leg.
	second := fixture.vendorOrderFor(t, order, fixture.env.businessB)
	authority.failFor = "vendor-order-payment-" + second.ID

	_, operation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if !errors.Is(err, orders.ErrPaymentAuthority) {
		t.Fatalf("err=%v, want ErrPaymentAuthority", err)
	}

	// The essential property: a partially authorized payment NEVER reports
	// itself as held.
	if operation.State == orders.PaymentHeld {
		t.Fatalf("a partially authorized payment reported state HELD")
	}

	// The order must not read as paid either.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.PaymentState == string(orders.PaymentHeld) {
		t.Fatal("a partially authorized payment reported the order as paid")
	}

	// An error from the authority is an UNKNOWN outcome, not a decided one: a
	// timeout may still have moved money. The leg therefore stays PENDING with
	// the reason recorded, and durable recovery asks the authority what really
	// happened rather than this code guessing either way.
	var unresolved int
	for _, leg := range operation.Legs {
		if leg.VendorOrderID != second.ID {
			continue
		}
		unresolved++
		if leg.Status != orders.LegPending {
			t.Errorf("refused leg status=%q, want PENDING pending recovery", leg.Status)
		}
		if leg.FailureReason == "" {
			t.Error("a refused leg records no reason for recovery to act on")
		}
	}
	if unresolved != 1 {
		t.Fatalf("refused legs=%d, want 1", unresolved)
	}
}

// Vendor Commerce never treats an unrecognised authority answer as success.
func TestSection17FPayment_UnknownAuthorityAnswerIsNotSuccess(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority.answer["vendor-order-payment-"+order.VendorOrders[0].ID] = "processing"

	_, operation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	if operation.State == orders.PaymentHeld {
		t.Fatal("an unrecognised authority answer was treated as held")
	}
	for _, leg := range operation.Legs {
		if leg.Status != orders.LegPending {
			t.Fatalf("leg status=%q, want PENDING for an unknown answer", leg.Status)
		}
		// The raw answer is still kept, so recovery can act on the truth.
		if leg.AuthorityStatus != "processing" {
			t.Fatalf("authority status=%q, want it kept verbatim", leg.AuthorityStatus)
		}
	}
}

// Every leg row must be durable BEFORE the authority is called, so a crash
// mid-call still leaves evidence that money may exist.
func TestSection17FPayment_LegsAreDurableBeforeTheAuthorityIsCalled(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Refuse the FIRST leg the service happens to reach, so the run stops
	// early and the remaining legs were never called for.
	first := fixture.vendorOrderFor(t, order, fixture.env.businessA)
	authority.failFor = "vendor-order-payment-" + first.ID

	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); err == nil {
		t.Fatal("expected the refused leg to surface an error")
	}

	// Both legs exist regardless of how far the run got.
	var legs int
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_payment_legs WHERE parent_order_id=$1`,
		order.ID).Scan(&legs); err != nil {
		t.Fatal(err)
	}
	if legs != 2 {
		t.Fatalf("durable legs=%d, want 2 written before any call", legs)
	}

	// And the request was recorded before the call that failed.
	var requested int
	if err = fixture.env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_payment_events
		 WHERE parent_order_id=$1 AND action='commerce.payment.leg_requested'`,
		order.ID).Scan(&requested); err != nil {
		t.Fatal(err)
	}
	if requested == 0 {
		t.Fatal("no leg request was recorded before calling the authority")
	}
}

func TestSection17FPayment_EvidenceIsAppendOnly(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	order, _, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, statement := range map[string]string{
		"update": `UPDATE commerce_payment_events SET authority_detail='forged'
		            WHERE parent_order_id=$1`,
		"delete": `DELETE FROM commerce_payment_events WHERE parent_order_id=$1`,
	} {
		if _, err = fixture.env.pool.Exec(ctx, statement, order.ID); err == nil {
			t.Errorf("database allowed %s on payment evidence", name)
		}
	}
}

// A deployment without a money authority has no payment behaviour at all.
func TestSection17FPayment_WithoutAnAuthorityPaymentIsRefused(t *testing.T) {
	fixture := newSection17EFixture(t, false)

	if fixture.service.PaymentEnabled() {
		t.Fatal("payment reported enabled with no authority wired")
	}
	if _, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	); !errors.Is(err, orders.ErrPaymentNotConfigured) {
		t.Fatalf("err=%v, want ErrPaymentNotConfigured", err)
	}
}
