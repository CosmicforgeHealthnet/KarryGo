package orders

// Section 17E usecases.
//
// Creation is customer-driven from an approved quote. Lifecycle transitions are
// vendor-staff-driven and scoped to the acting vendor's own branches. Neither
// path creates a payment operation or moves money.

import (
	"context"
	"strings"
	"time"
)

// Options configures the order service.
type Options struct {
	Store  *Store
	Quotes QuoteAuthority
	Scopes ScopeResolver

	// Enabled is the deployment capability switch. With it false the service
	// is not built and no route is registered.
	Enabled bool

	Now func() time.Time
}

// Service owns Section 17E order orchestration.
type Service struct {
	store   *Store
	quotes  QuoteAuthority
	scopes  ScopeResolver
	enabled bool
	now     func() time.Time

	// payments is the Section 17F money authority. It is nil until
	// SetPaymentAuthority wires one, so a deployment without Wallet
	// credentials has a working order service and no payment surface at all.
	payments      PaymentAuthority
	customerEmail CustomerEmailResolver
	changeQuotes  CommercialChangeQuoteAuthority

	// inventory and missions are the Section 17F fulfilment authorities. Both
	// nil until SetFulfilmentAuthorities wires them, so a deployment without
	// them cannot activate fulfilment rather than failing halfway through it.
	inventory InventoryAuthority
	missions  MissionAuthority
}

// NewService builds the order service.
func NewService(options Options) (*Service, error) {
	if options.Store == nil || options.Quotes == nil || options.Scopes == nil {
		return nil, ErrDependencyMissing
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		store:   options.Store,
		quotes:  options.Quotes,
		scopes:  options.Scopes,
		enabled: options.Enabled,
		now:     now,
	}, nil
}

const defaultListLimit = 50

// -----------------------------------------------------------------------------
// Customer surface
// -----------------------------------------------------------------------------

// CreateFromQuote turns one approved quote into one ParentOrder.
//
// The caller supplies the fingerprint it is committing to. A quote whose
// commercial content has moved on produces a different fingerprint, so a client
// can never order an agreement other than the one it displayed.
func (s *Service) CreateFromQuote(
	ctx context.Context, customerID, quoteID, fingerprint string,
) (ParentOrder, bool, error) {
	if !s.enabled {
		return ParentOrder{}, false, ErrDisabled
	}
	customerID = strings.TrimSpace(customerID)
	quoteID = strings.TrimSpace(quoteID)
	fingerprint = strings.TrimSpace(fingerprint)
	if customerID == "" || quoteID == "" || fingerprint == "" {
		return ParentOrder{}, false, ErrQuoteNotOrderable
	}

	now := s.now().UTC()

	source, err := s.quotes.LoadApprovedQuote(ctx, customerID, quoteID, now)
	if err != nil {
		return ParentOrder{}, false, err
	}
	if source.QuoteFingerprint != fingerprint {
		return ParentOrder{}, false, ErrFingerprintMismatch
	}
	if source.CustomerID != customerID {
		return ParentOrder{}, false, ErrForbidden
	}

	plan, err := ComputeAllocation(source)
	if err != nil {
		return ParentOrder{}, false, err
	}

	return s.store.Create(ctx, source, plan, source.Lines, now)
}

// GetForCustomer reads one of the customer's own purchases.
func (s *Service) GetForCustomer(
	ctx context.Context, customerID, orderID string,
) (ParentOrder, error) {
	if !s.enabled {
		return ParentOrder{}, ErrDisabled
	}
	return s.store.GetParent(ctx, strings.TrimSpace(customerID), strings.TrimSpace(orderID))
}

// ListForCustomer reads the customer's purchases, newest first.
func (s *Service) ListForCustomer(
	ctx context.Context, customerID string, limit int,
) ([]ParentOrder, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	if limit <= 0 || limit > defaultListLimit {
		limit = defaultListLimit
	}
	return s.store.ListParents(ctx, strings.TrimSpace(customerID), limit)
}

// CancelByCustomer withdraws the customer from ONE vendor allocation.
//
// Cancelling one vendor order never touches another vendor's stock, allocation
// or fulfilment state: the sibling orders keep running and the ParentOrder
// reports the mixed outcome honestly.
func (s *Service) CancelByCustomer(
	ctx context.Context, customerID, vendorOrderID, reasonCode string, expectedVersion int64,
) (VendorOrder, ParentOrder, error) {
	if !s.enabled {
		return VendorOrder{}, ParentOrder{}, ErrDisabled
	}
	customerID = strings.TrimSpace(customerID)

	current, err := s.store.GetVendorOrderForCustomer(
		ctx, customerID, strings.TrimSpace(vendorOrderID),
	)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	// Once the goods have left the vendor, withdrawing is no longer a
	// cancellation. Section 17E refuses rather than inventing a returns flow.
	// The same rule is re-checked under the transaction's lock, so a vendor
	// handing over concurrently cannot be overtaken by this cancellation.
	if !CancellableByCustomer(current.Status) {
		return VendorOrder{}, ParentOrder{}, ErrOrderConflict
	}

	moved, parent, err := s.store.Transition(ctx, TransitionRequest{
		VendorOrderID:              current.ID,
		ExpectedVersion:            expectedVersion,
		Transition:                 TransitionCancel,
		CancelledBy:                CancelByCustomer,
		ReasonCode:                 strings.TrimSpace(reasonCode),
		ActorType:                  "CUSTOMER",
		ActorRef:                   customerID,
		CustomerID:                 customerID,
		RequireCustomerCancellable: true,
	}, s.now().UTC())
	if err != nil {
		return moved, parent, err
	}
	return s.afterTransition(ctx, moved, parent)
}

// afterTransition finalizes stock for an allocation that just went terminal.
//
// A failure here is deliberately NOT surfaced as a failed transition: the
// transition itself is committed and correct, and the stock move is retried by
// the recovery sweep. Reporting a failure would tell a caller its cancellation
// did not happen when it did.
func (s *Service) afterTransition(
	ctx context.Context, moved VendorOrder, parent ParentOrder,
) (VendorOrder, ParentOrder, error) {
	if !moved.Status.Terminal() {
		return moved, parent, nil
	}
	_ = s.FinalizeInventoryFor(ctx, moved)
	// Money movement is deliberately after the lifecycle commit. A crash or
	// unavailable Wallet leaves the terminal vendor order durable; the payment
	// recovery sweep retries the exact isolated leg without blocking siblings.
	_ = s.settleOrRefundTerminalLeg(ctx, moved)
	return moved, parent, nil
}

// -----------------------------------------------------------------------------
// Vendor surface
// -----------------------------------------------------------------------------

// authorize resolves the acting staff member's reach for one permission.
func (s *Service) authorize(
	ctx context.Context, actor Actor, permission string,
) (Scope, error) {
	if !s.enabled {
		return Scope{}, ErrDisabled
	}
	if actor.StaffID == "" || actor.VendorID == "" {
		return Scope{}, ErrForbidden
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, permission)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, ErrForbidden
	}
	return scope, nil
}

// ListForVendor reads the acting vendor's own order queue.
func (s *Service) ListForVendor(
	ctx context.Context, actor Actor, filter VendorQueueFilter,
) ([]VendorOrder, error) {
	scope, err := s.authorize(ctx, actor, PermOrdersRead)
	if err != nil {
		return nil, err
	}
	// A branch filter outside the actor's scope is a forbidden request, not a
	// silently empty page: an empty page would still confirm the branch exists.
	if filter.BranchID != "" && !scope.Covers(filter.BranchID) {
		return nil, ErrForbidden
	}
	if filter.Limit <= 0 || filter.Limit > defaultListLimit {
		filter.Limit = defaultListLimit
	}
	return s.store.ListVendorOrders(ctx, actor.VendorID, scope, filter)
}

// GetForVendor reads one of the acting vendor's own allocations.
func (s *Service) GetForVendor(
	ctx context.Context, actor Actor, vendorOrderID string,
) (VendorOrder, error) {
	scope, err := s.authorize(ctx, actor, PermOrdersRead)
	if err != nil {
		return VendorOrder{}, err
	}
	order, err := s.store.GetVendorOrderForVendor(
		ctx, actor.VendorID, strings.TrimSpace(vendorOrderID),
	)
	if err != nil {
		return VendorOrder{}, err
	}
	// A branch-scoped actor reading outside its branches gets not-found, so a
	// probe cannot map which branches hold orders.
	if !scope.Covers(order.BranchID) {
		return VendorOrder{}, ErrOrderNotFound
	}
	return order, nil
}

// Advance applies one vendor lifecycle command to one VendorOrder.
func (s *Service) Advance(
	ctx context.Context,
	actor Actor,
	vendorOrderID string,
	transition Transition,
	reasonCode string,
	expectedVersion int64,
) (VendorOrder, ParentOrder, error) {
	scope, err := s.authorize(ctx, actor, PermOrdersManage)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}

	current, err := s.store.GetVendorOrderForVendor(
		ctx, actor.VendorID, strings.TrimSpace(vendorOrderID),
	)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if !scope.Covers(current.BranchID) {
		return VendorOrder{}, ParentOrder{}, ErrOrderNotFound
	}

	request := TransitionRequest{
		VendorOrderID:   current.ID,
		ExpectedVersion: expectedVersion,
		Transition:      transition,
		ReasonCode:      strings.TrimSpace(reasonCode),
		ActorType:       "VENDOR_STAFF",
		ActorRef:        actor.StaffID,
		BusinessID:      actor.VendorID,
	}
	if transition == TransitionCancel {
		request.CancelledBy = CancelByVendor
	}

	moved, parent, err := s.store.Transition(ctx, request, s.now().UTC())
	if err != nil {
		return moved, parent, err
	}
	return s.afterTransition(ctx, moved, parent)
}
