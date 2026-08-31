package fulfilment

// The Section 17C customer-facing service.
//
// It turns a validated Section 17B checkout session into a persisted candidate
// plan. It owns no checkout state: the session, its groups and its revalidated
// totals are read through a narrow port, so this package never imports
// checkout and checkout learns nothing about planning.

import (
	"context"
	"strings"
	"time"
)

// CheckoutGroupSnapshot is one validated Section 17B group, as planning needs
// it. Every value is server-resolved checkout evidence.
type CheckoutGroupSnapshot struct {
	CheckoutGroupID string

	BusinessID string
	BranchID   string

	// QuotedTotalMinor is the group's revalidated merchandise total. It is
	// used ONLY for the vendor's own minimum-order and free-delivery rules; no
	// platform fee is ever derived from it.
	QuotedTotalMinor int64

	Currency string

	Lines []LineLogistics
}

// CheckoutSnapshot is the validated checkout a plan is built from.
type CheckoutSnapshot struct {
	SessionID string

	CustomerID string

	ValidationRevision int64

	// Plannable is false when the session is not VALIDATED, has expired, or
	// otherwise cannot support a plan. Planning refuses rather than guessing.
	Plannable bool

	Currency string

	Destination Destination

	Groups []CheckoutGroupSnapshot
}

// CheckoutAuthority resolves a validated checkout session for planning.
//
// A narrow port rather than a checkout import: it is what keeps
// internal/architecture_test.go's boundary rule true and lets planning be
// tested without a checkout at all.
type CheckoutAuthority interface {
	ResolveForPlanning(
		ctx context.Context,
		customerID, sessionID string,
	) (CheckoutSnapshot, error)
}

// ServiceOptions configures the service.
type ServiceOptions struct {
	Store    *Store
	Planner  *Planner
	Checkout CheckoutAuthority

	Now func() time.Time
}

// PlanningService is the Section 17C entry point.
//
// Named PlanningService rather than Service because Service already names the
// logistics authority enum in vehicle.go, and two different meanings of one
// word in one package is how a reader ends up mistaking a DISPATCH constant
// for a dependency.
type PlanningService struct {
	store    *Store
	planner  *Planner
	checkout CheckoutAuthority

	now func() time.Time
}

// NewService builds the service. A missing dependency is a wiring error, so it
// is reported at build time rather than becoming a runtime 500.
func NewService(options ServiceOptions) (*PlanningService, error) {
	if options.Store == nil ||
		options.Planner == nil ||
		options.Checkout == nil {
		return nil, ErrDependencyMissing
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &PlanningService{
		store:    options.Store,
		planner:  options.Planner,
		checkout: options.Checkout,
		now:      now,
	}, nil
}

// Plan builds and persists a candidate plan for one checkout session.
//
// customerID comes from the verified Customer Service bearer token and nowhere
// else. There is deliberately no customer id, business id, branch id or fee
// anywhere in the request contract: a customer cannot overwrite a logistics
// fee, and a vendor cannot inject another vendor's pickup, because neither is
// accepted as input at all.
func (s *PlanningService) Plan(
	ctx context.Context,
	customerID, sessionID string,
) (Plan, error) {
	return s.PlanFor(ctx, customerID, sessionID, DefaultMode)
}

// PlanFor builds a plan optimized for one fulfilment preference.
//
// The mode is the single value a customer may contribute to planning, and it
// is safe for a structural reason rather than a hopeful one: it selects among
// alternatives the server constructed and the logistics authorities priced.
// There is still no fee, no business id, no branch id and no customer id in
// the request contract, so a customer can no more choose a delivery price
// here than they could before.
func (s *PlanningService) PlanFor(
	ctx context.Context,
	customerID, sessionID string,
	mode Mode,
) (Plan, error) {
	customerID = strings.TrimSpace(customerID)
	sessionID = strings.TrimSpace(sessionID)

	if customerID == "" || sessionID == "" {
		return Plan{}, ErrCheckoutNotPlannable
	}

	if !mode.Selectable() {
		mode = DefaultMode
	}

	snapshot, err := s.checkout.ResolveForPlanning(ctx, customerID, sessionID)
	if err != nil {
		return Plan{}, err
	}

	if !snapshot.Plannable || len(snapshot.Groups) == 0 {
		return Plan{}, ErrCheckoutNotPlannable
	}

	// The session's own customer is used, never the one supplied alongside it.
	// They are the same value here, and asserting that rather than assuming it
	// is what makes a future refactor of the port safe.
	if strings.TrimSpace(snapshot.CustomerID) != customerID {
		return Plan{}, ErrCheckoutNotPlannable
	}

	groups := make([]GroupInput, 0, len(snapshot.Groups))

	for _, group := range snapshot.Groups {
		currency := group.Currency

		if strings.TrimSpace(currency) == "" {
			currency = snapshot.Currency
		}

		groups = append(groups, GroupInput{
			CheckoutGroupID: group.CheckoutGroupID,
			BusinessID:      group.BusinessID,
			BranchID:        group.BranchID,
			OrderValueMinor: group.QuotedTotalMinor,
			Currency:        currency,
			Lines:           group.Lines,
		})
	}

	plan, events, err := s.planner.Build(ctx, PlanRequest{
		CustomerID:                 customerID,
		CheckoutSessionID:          snapshot.SessionID,
		CheckoutValidationRevision: snapshot.ValidationRevision,
		Currency:                   snapshot.Currency,
		Destination:                snapshot.Destination,
		Groups:                     groups,
		Mode:                       mode,
		CorrelationID:              snapshot.SessionID,
	})
	if err != nil {
		return Plan{}, err
	}

	return s.store.Save(ctx, plan, events)
}

// QuotePlan answers what a prospective basket would cost to deliver.
//
// It delegates straight to the planner and deliberately does NOT touch the
// store. That is what makes it quote-only: the one thing this service adds
// over the planner is persistence, and this method is the path that skips it.
//
// It lives on the service rather than only on the planner so the composition
// root has a single fulfilment dependency to hand out, and so a caller that
// only ever needs a quote can be given a port with exactly this one method on
// it - unable to reach Plan, unable to reach the store.
func (s *PlanningService) QuotePlan(
	ctx context.Context,
	request QuoteOnlyRequest,
) (QuoteOnlyResult, error) {
	if s == nil || s.planner == nil {
		return QuoteOnlyResult{}, ErrDependencyMissing
	}

	return s.planner.QuotePlan(ctx, request)
}

// Get reads one plan the customer owns.
//
// A live plan past its expiry is reported as expired rather than as live: the
// quotes inside it have lapsed, so presenting it as current would present fees
// no authority still stands behind.
func (s *PlanningService) Get(
	ctx context.Context,
	customerID, planID string,
) (Plan, error) {
	plan, err := s.store.Get(ctx, customerID, planID)
	if err != nil {
		return Plan{}, err
	}

	return s.markExpired(plan), nil
}

// GetForCheckout reads the current plan for a checkout session.
//
// It returns a terminal UNSERVICEABLE plan too: the customer was just told
// which vendors cannot be delivered and why, and that answer must survive a
// page reload.
func (s *PlanningService) GetForCheckout(
	ctx context.Context,
	customerID, sessionID string,
) (Plan, error) {
	plan, err := s.store.GetCurrentForCheckout(ctx, customerID, sessionID)
	if err != nil {
		return Plan{}, err
	}

	return s.markExpired(plan), nil
}

func (s *PlanningService) markExpired(plan Plan) Plan {
	if plan.Status.Live() && plan.Expired(s.now().UTC()) {
		plan.Status = StatusExpired
	}

	return plan
}

// SweepExpired marks lapsed plans expired. It is the durable counterpart to
// the read-time check above: a plan nobody reads must still stop being live.
func (s *PlanningService) SweepExpired(
	ctx context.Context,
	limit int,
) (int64, error) {
	return s.store.ExpireDue(ctx, s.now().UTC(), limit)
}
