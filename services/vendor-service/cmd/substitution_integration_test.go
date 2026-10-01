package main

// Shop substitution through the REAL production path: vendor proposes a
// replacement from the live catalogue (ProposeSubstitution), the customer
// responds (RespondToSubstitution), and money moves only through the order
// service's own Wallet orchestration. Real PostgreSQL order, quote, plan and
// commercial-change state; the Wallet, Inventory and replacement-quote
// authorities are explicit doubles (the replacement quote engine itself is
// certified against real checkout/logistics by
// TestCustomer14ReplacementUsesFreshCheckoutAndLogisticsDB).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// replacementInventory records replacement allocations and is idempotent per key.
type replacementInventory struct {
	fakeInventory
	mu       sync.Mutex
	replaced map[string]int
}

func (r *replacementInventory) ResolveShopVariantItem(_ context.Context, _, _, variantRef string) (string, error) {
	return "item_" + variantRef, nil
}

func (r *replacementInventory) ReplaceReservationAllocation(_ context.Context, _, _ string, request orders.InventoryReplacementRequest) (orders.InventoryReplacementResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replaced[request.IdempotencyKey]++
	return orders.InventoryReplacementResult{}, nil
}

// substitutionQuotes serves both the commercial-change and the substitution
// replacement-quote contracts from one real persisted replacement quote.
type substitutionQuotes struct {
	*scriptedQuotes
	substitutionCalls int
}

func (q *substitutionQuotes) CreateSubstitutionReplacementQuote(_ context.Context, request orders.SubstitutionCommercialChangeRequest) (orders.ReplacementQuote, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.substitutionCalls++
	return q.quote(request.CustomerID), nil
}

// commerceWallet adds Wallet's commerce-allocation contract to fakeAuthority:
// allocation-local state, refunds idempotent per key, and read-back of a refund
// whose response was lost. loseNextResponse applies the refund but reports a
// transport failure, which is exactly the ambiguous outcome read-back resolves.
type commerceWallet struct {
	*fakeAuthority
	mu               sync.Mutex
	payable          map[string]int64
	refunded         map[string]int64
	operations       map[string]orders.CommerceFinancialResult
	refundCalls      int
	loseNextResponse bool
}

func newCommerceWallet(base *fakeAuthority, order orders.ParentOrder) *commerceWallet {
	w := &commerceWallet{fakeAuthority: base, payable: map[string]int64{}, refunded: map[string]int64{},
		operations: map[string]orders.CommerceFinancialResult{}}
	for _, vendorOrder := range order.VendorOrders {
		w.payable[vendorOrder.ID] = vendorOrder.CustomerPayableMinor
	}
	return w
}

func (w *commerceWallet) GetCommerceAllocationState(_ context.Context, _, vendorOrderID string) (orders.CommerceAllocationState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return orders.CommerceAllocationState{SnapshotFingerprint: "alloc_" + vendorOrderID,
		CustomerPayableMinor: w.payable[vendorOrderID], MaxAuthorizedMinor: w.payable[vendorOrderID],
		RefundCustomerMinor: w.refunded[vendorOrderID]}, nil
}

func (w *commerceWallet) GetCommerceRefundOperation(_ context.Context, key string) (orders.CommerceFinancialResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if op, ok := w.operations[key]; ok {
		return op, nil
	}
	return orders.CommerceFinancialResult{}, errors.New("no such refund operation")
}

func (w *commerceWallet) RefundCommerceAllocation(_ context.Context, request orders.CommerceRefundRequest) (orders.CommerceFinancialResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.refundCalls++
	if op, ok := w.operations[request.IdempotencyKey]; ok {
		return op, nil
	}
	op := orders.CommerceFinancialResult{Reference: "crf_" + request.IdempotencyKey, Status: "APPLIED", AmountMinor: request.CustomerRefundMinor}
	w.operations[request.IdempotencyKey] = op
	w.refunded[request.VendorOrderID] += request.CustomerRefundMinor
	if w.loseNextResponse {
		w.loseNextResponse = false
		return orders.CommerceFinancialResult{}, errors.New("wallet timeout")
	}
	return op, nil
}

func (w *commerceWallet) ReleaseCommerceAllocation(context.Context, orders.CommerceReleaseRequest) (orders.CommerceFinancialResult, error) {
	return orders.CommerceFinancialResult{}, errors.New("release not expected in substitution tests")
}

type substitutionEnv struct {
	*physicalEnv
	inventory *replacementInventory
	lineID    string
	unitPrice int64
}

func newSubstitutionEnv(t *testing.T) *substitutionEnv {
	return newSubstitutionEnvWithPolicy(t, "CONTACT_FIRST")
}

// newSubstitutionEnvWithPolicy seeds the customer's rule on the checkout line,
// so it reaches the order only through the quote evidence and the order's own
// picking snapshot — never through a test-side write to the order.
func newSubstitutionEnvWithPolicy(t *testing.T, policy string) *substitutionEnv {
	t.Helper()
	ctx := context.Background()
	s17dLineSubstitutionPolicy = policy
	defer func() { s17dLineSubstitutionPolicy = "" }()
	p := newPhysicalEnv(t, 1_000, s17cVariantOptions{})
	if p.vendorA.Domain != "SHOP" {
		t.Fatalf("vendor A domain=%s, want SHOP", p.vendorA.Domain)
	}
	inventory := &replacementInventory{replaced: map[string]int{}}
	p.fixture.service.SetFulfilmentAuthorities(inventory, p.missions)

	s := &substitutionEnv{physicalEnv: p, inventory: inventory}
	pool := p.fixture.env.pool
	if err := pool.QueryRow(ctx, `SELECT id,unit_price_minor FROM commerce_vendor_order_lines WHERE vendor_order_id=$1 ORDER BY id LIMIT 1`,
		p.vendorA.ID).Scan(&s.lineID, &s.unitPrice); err != nil {
		t.Fatal(err)
	}
	var frozen string
	if err := pool.QueryRow(ctx, `SELECT substitution_policy FROM commerce_shop_picking_snapshots WHERE order_line_id=$1`, s.lineID).Scan(&frozen); err != nil || frozen != policy {
		t.Fatalf("frozen picking policy=%q (%v), want the customer's %q", frozen, err, policy)
	}
	// The line was found short during picking.
	if _, err := pool.Exec(ctx, `
		INSERT INTO commerce_vendor_order_line_outcomes(id,parent_order_id,vendor_order_id,order_line_id,ordered_quantity,picked_quantity,unavailable_quantity,outcome,picking_state,quality_result)
		SELECT 'outcome_'||l.id,l.parent_order_id,l.vendor_order_id,l.id,l.quantity,0,l.quantity,'UNAVAILABLE','IN_PROGRESS','MET'
		  FROM commerce_vendor_order_lines l WHERE l.id=$1`, s.lineID); err != nil {
		t.Fatal(err)
	}
	current, _, err := p.fixture.service.Advance(ctx, p.actorA, p.vendorA.ID, orders.TransitionAccept, "", p.vendorA.Version)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, _, err = p.fixture.service.Advance(ctx, p.actorA, current.ID, orders.TransitionPicking, "", current.Version); err != nil {
		t.Fatalf("start picking: %v", err)
	}
	return s
}

// candidate seeds a live catalogue listing at an explicit branch price.
func (s *substitutionEnv) candidate(t *testing.T, priceMinor int64) string {
	t.Helper()
	env := s.fixture.env
	ctx := context.Background()
	variant := env.seedVariant(ctx, s17cVariantOptions{netWeightGrams: ptrInt64(1_000), stackable: true})
	listing := env.seedListing(ctx, variant)
	env.exec(ctx, `UPDATE shop_branch_listings SET retail_price_override_minor=$2 WHERE id=$1`, listing, priceMinor)
	return listing
}

func (s *substitutionEnv) propose(t *testing.T, listing string) orders.SubstitutionProposal {
	t.Helper()
	expires := time.Now().UTC().Add(30 * time.Minute)
	proposal, err := s.fixture.service.ProposeSubstitution(context.Background(), s.actorA, s.vendorA.ID, s.lineID,
		orders.SubstitutionProposal{ProposedCatalogueRef: listing, Reason: "out of stock", ExpiresAt: &expires},
		"sub-"+listing)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return proposal
}

func (s *substitutionEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := s.fixture.env.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (s *substitutionEnv) originalQuoteUnchanged(t *testing.T) {
	t.Helper()
	var fingerprint string
	if err := s.fixture.env.pool.QueryRow(context.Background(), `SELECT quote_fingerprint FROM commerce_quotes WHERE id=$1`,
		s.fixture.quoteID).Scan(&fingerprint); err != nil || fingerprint != s.fixture.finger {
		t.Fatalf("original quote mutated: %q (%v)", fingerprint, err)
	}
	reread, err := s.fixture.service.GetForCustomer(context.Background(), s.fixture.customerID, s.order.ID)
	if err != nil || reread.QuoteID != s.fixture.quoteID || reread.QuoteFingerprint != s.fixture.finger {
		t.Fatalf("order rebound away from its approved quote: %+v (%v)", reread, err)
	}
}

func TestSubstitution_EqualPriceMovesNoMoney(t *testing.T) {
	s := newSubstitutionEnv(t)
	ctx := context.Background()
	proposal := s.propose(t, s.candidate(t, s.unitPrice))
	if proposal.PriceDifferenceMinor != 0 {
		t.Fatalf("difference=%d, want 0", proposal.PriceDifferenceMinor)
	}
	holds, refunds := s.wallet.callCount(), len(s.wallet.refundedFor())

	resolved, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if resolved.Status != orders.SubstitutionApproved {
		t.Fatalf("status=%s", resolved.Status)
	}
	if s.wallet.callCount() != holds || len(s.wallet.refundedFor()) != refunds {
		t.Fatal("an equal-price substitution moved money")
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`, s.order.ID); n != 0 {
		t.Fatalf("adjustments=%d, want 0", n)
	}
	if s.inventory.replaced["substitution-inventory-"+proposal.ID] != 1 {
		t.Fatalf("replacement stock allocations=%v", s.inventory.replaced)
	}
	vo, err := orders.NewStore(s.fixture.env.pool).GetVendorOrderForCustomer(ctx, s.fixture.customerID, s.vendorA.ID)
	if err != nil || vo.Status != orders.VendorStatusPicking {
		t.Fatalf("vendor order status=%s, want back to PICKING", vo.Status)
	}
	s.originalQuoteUnchanged(t)
}

func TestSubstitution_CheaperRefundsTheDifferenceOnce(t *testing.T) {
	s := newSubstitutionEnv(t)
	ctx := context.Background()
	const cheaper = 300
	wallet := newCommerceWallet(s.wallet, s.order)
	s.fixture.service.SetPaymentAuthority(orders.PaymentOptions{Authority: wallet})
	proposal := s.propose(t, s.candidate(t, s.unitPrice-cheaper))
	if proposal.PriceDifferenceMinor != -cheaper {
		t.Fatalf("difference=%d, want %d", proposal.PriceDifferenceMinor, -cheaper)
	}
	holds := s.wallet.callCount()

	// The refund is applied but its response is lost: the outcome is unknown.
	wallet.loseNextResponse = true
	if _, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version); err == nil {
		t.Fatal("a lost Wallet response was reported as success")
	}
	var walletStatus string
	if err := s.fixture.env.pool.QueryRow(ctx, `SELECT wallet_status FROM commerce_refund_policy_decisions WHERE vendor_order_id=$1`, s.vendorA.ID).Scan(&walletStatus); err != nil || walletStatus != "UNKNOWN" {
		t.Fatalf("refund wallet status=%q (%v), want UNKNOWN pending read-back", walletStatus, err)
	}
	// The customer retries: read-back finds the applied refund; nothing is re-sent.
	resolved, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	if err != nil || resolved.Status != orders.SubstitutionApproved {
		t.Fatalf("retry: status=%s err=%v", resolved.Status, err)
	}
	if _, err = s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version); err != nil {
		t.Fatalf("second retry: %v", err)
	}
	if wallet.refundCalls != 1 || wallet.refunded[s.vendorA.ID] != cheaper {
		t.Fatalf("refund calls=%d refunded=%d, want one refund of %d", wallet.refundCalls, wallet.refunded[s.vendorA.ID], cheaper)
	}
	if s.wallet.callCount() != holds || len(s.wallet.refundedFor()) != 0 {
		t.Fatal("a cheaper substitution used the whole-leg payment path")
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_refund_policy_decisions WHERE vendor_order_id=$1 AND wallet_status='APPLIED'`, s.vendorA.ID); n != 1 {
		t.Fatalf("applied refund decisions=%d, want 1", n)
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1 AND action='REFUND'`, s.order.ID); n != 1 {
		t.Fatalf("refund adjustments=%d, want 1", n)
	}
	s.originalQuoteUnchanged(t)
}

func TestSubstitution_CostlierRequiresApprovalBeforeCharging(t *testing.T) {
	s := newSubstitutionEnv(t)
	ctx := context.Background()
	const extra = 400
	proposal := s.propose(t, s.candidate(t, s.unitPrice+extra))
	if proposal.PriceDifferenceMinor != extra {
		t.Fatalf("difference=%d, want %d", proposal.PriceDifferenceMinor, extra)
	}

	scripted := newScriptedQuotes(s.order.CustomerApprovedTotalMinor + extra)
	scripted.quoteID, scripted.fingerprint = seedReplacementQuote(t, s.fixture, s.order,
		s.order.CustomerApprovedTotalMinor+extra, s.order.SelectedFulfilmentMode)
	quotes := &substitutionQuotes{scriptedQuotes: scripted}
	s.fixture.service.SetCommercialChangeAuthority(quotes)
	holds := s.wallet.callCount()

	// 1. The customer's approval alone charges nothing: it names the change.
	_, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	var required *orders.SubstitutionChangeRequiredError
	if !errors.As(err, &required) || !errors.Is(err, orders.ErrCommercialChangeRequired) || required.ChangeID == "" {
		t.Fatalf("err=%v, want SubstitutionChangeRequiredError naming a change", err)
	}
	if s.wallet.callCount() != holds {
		t.Fatal("money moved before the customer approved the replacement quote")
	}
	pending, err := orders.NewStore(s.fixture.env.pool).GetSubstitutionForCustomer(ctx, s.fixture.customerID, proposal.ID)
	if err != nil || pending.Status != orders.SubstitutionPending {
		t.Fatalf("proposal=%s before approval, want PENDING", pending.Status)
	}
	// A repeated request reuses the same change and the same quote.
	_, err = s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	var again *orders.SubstitutionChangeRequiredError
	if !errors.As(err, &again) || again.ChangeID != required.ChangeID {
		t.Fatalf("second request err=%v, want the same change %s", err, required.ChangeID)
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_order_changes WHERE parent_order_id=$1`, s.order.ID); n != 1 {
		t.Fatalf("commercial changes=%d, want 1", n)
	}

	// 2. Wallet outcome unknown: nothing applies, retry converges on one hold.
	change, err := s.fixture.service.GetCommercialChange(ctx, s.fixture.customerID, required.ChangeID)
	if err != nil {
		t.Fatal(err)
	}
	s.wallet.failFor = "commerce-change-" + change.ID
	if _, err = s.fixture.service.ApproveCommercialChange(ctx, s.fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint); !errors.Is(err, orders.ErrPaymentAuthority) {
		t.Fatalf("err=%v, want ErrPaymentAuthority", err)
	}
	if _, err = s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version); !errors.Is(err, orders.ErrCommercialChangeRequired) {
		t.Fatalf("substitution became effective with an unknown Wallet outcome: %v", err)
	}
	s.wallet.failFor = ""
	applied, err := s.fixture.service.ApproveCommercialChange(ctx, s.fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint)
	if err != nil || applied.State != orders.ChangeApplied {
		t.Fatalf("retry approval: state=%s err=%v", applied.State, err)
	}

	// 3. Only now does the costlier substitution become effective.
	resolved, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	if err != nil || resolved.Status != orders.SubstitutionApproved {
		t.Fatalf("approve after change: status=%s err=%v", resolved.Status, err)
	}
	if _, err = s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version); err != nil {
		t.Fatalf("idempotent re-approval: %v", err)
	}

	var held int64
	keys := map[string]bool{}
	for _, request := range s.wallet.requests[holds:] {
		if request.IdempotencyKey == "commerce-change-"+change.ID && !keys[request.IdempotencyKey] {
			keys[request.IdempotencyKey] = true
			held += request.AmountKobo
		}
	}
	if held != extra {
		t.Fatalf("extra authorization=%d, want exactly %d once", held, extra)
	}
	if len(s.wallet.refundedFor()) != 0 {
		t.Fatal("a costlier substitution refunded money")
	}
	// Re-approval re-sends the swap under the same idempotency key (the
	// Inventory authority's retry contract); it is still one operation.
	if len(s.inventory.replaced) != 1 || s.inventory.replaced["substitution-inventory-"+proposal.ID] == 0 {
		t.Fatalf("replacement stock allocations=%v", s.inventory.replaced)
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_quotes WHERE customer_id=$1`, s.fixture.customerID); n != 2 {
		t.Fatalf("quotes=%d, want the original plus one replacement", n)
	}
	s.originalQuoteUnchanged(t)
}

// A customer who rejects keeps their money and the line is not replaced.
func TestSubstitution_RejectionMovesNothing(t *testing.T) {
	s := newSubstitutionEnv(t)
	proposal := s.propose(t, s.candidate(t, s.unitPrice-200))
	resolved, err := s.fixture.service.RespondToSubstitution(context.Background(), s.fixture.customerID, proposal.ID, false, proposal.Version)
	if err != nil || resolved.Status != orders.SubstitutionRejected {
		t.Fatalf("reject: status=%s err=%v", resolved.Status, err)
	}
	if len(s.wallet.refundedFor()) != 0 || len(s.inventory.replaced) != 0 {
		t.Fatal("a rejected substitution moved money or stock")
	}
}

// The customer's rule is authoritative: an EXACT_ONLY line cannot be replaced.
func TestSubstitution_ExactOnlyLineRefusesAnyProposal(t *testing.T) {
	s := newSubstitutionEnvWithPolicy(t, "EXACT_ONLY")
	expires := time.Now().UTC().Add(30 * time.Minute)
	_, err := s.fixture.service.ProposeSubstitution(context.Background(), s.actorA, s.vendorA.ID, s.lineID,
		orders.SubstitutionProposal{ProposedCatalogueRef: s.candidate(t, s.unitPrice), Reason: "out of stock", ExpiresAt: &expires}, "sub-exact")
	if !errors.Is(err, orders.ErrOrderConflict) {
		t.Fatalf("err=%v, want the customer's EXACT_ONLY rule to refuse", err)
	}
	if n := s.count(t, `SELECT count(*) FROM commerce_substitution_proposals WHERE vendor_order_id=$1`, s.vendorA.ID); n != 0 {
		t.Fatalf("proposals=%d, want none", n)
	}
}
