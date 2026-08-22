package checkout

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

// Options configures the Section 17B checkout service.
type Options struct {
	Store *Store

	Food  FoodAuthority
	Shop  ShopAuthority
	Stock StockAuthority

	// ReservationTTL is the configurable checkout reservation lifetime. It is
	// required: Section 17B deliberately refuses to run on a hardcoded value.
	ReservationTTL time.Duration

	// StrandedGrace is how long a session may sit mid-coordination before
	// recovery treats it as abandoned by a dead process.
	StrandedGrace time.Duration

	Now func() time.Time
}

// Service is the Section 17B application boundary.
type Service struct {
	store *Store

	food  FoodAuthority
	shop  ShopAuthority
	stock StockAuthority

	ttl   time.Duration
	grace time.Duration

	now func() time.Time
}

// NewService validates the configured reservation TTL and builds the service.
func NewService(options Options) (*Service, error) {
	if options.Store == nil || options.Store.db == nil {
		return nil, ErrDependencyMissing
	}

	if options.ReservationTTL < MinReservationTTL ||
		options.ReservationTTL > MaxReservationTTL {
		return nil, ErrInvalidTTL
	}

	grace := options.StrandedGrace

	if grace <= 0 {
		grace = DefaultStrandedGrace
	}

	clock := options.Now

	if clock == nil {
		clock = time.Now
	}

	return &Service{
		store: options.Store,
		food:  options.Food,
		shop:  options.Shop,
		stock: options.Stock,
		ttl:   options.ReservationTTL,
		grace: grace,
		now:   clock,
	}, nil
}

// ReservationTTL exposes the configured TTL.
func (s *Service) ReservationTTL() time.Duration {
	return s.ttl
}

func (s *Service) requireDependencies() error {
	if s == nil ||
		s.store == nil ||
		s.store.db == nil ||
		s.food == nil ||
		s.shop == nil ||
		s.stock == nil {
		return ErrDependencyMissing
	}

	return nil
}

func validateCustomerID(customerID string) (string, error) {
	customerID = strings.TrimSpace(customerID)

	if customerID == "" {
		return "", ErrSessionNotFound
	}

	return customerID, nil
}

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

// lineOutcome is one revalidated line before it is grouped and persisted.
type lineOutcome struct {
	line Line

	// requiresStock is true only for a Shop line that resolved a real
	// Section 14 reservation basis.
	requiresStock bool
}

// StartCheckout turns an ACTIVE cart into a validated checkout candidate.
//
// It is idempotent for a cart that already owns a live session: the existing
// session is returned rather than a second one created, which is what makes a
// duplicated client request, a retried gateway call and a double-tapped
// button all safe.
//
// A validation refusal is a FAILED session, not an error. Errors are reserved
// for preconditions the customer must fix before checkout is even meaningful,
// and for an authority that could not answer at all.
func (s *Service) StartCheckout(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
) (Session, error) {
	if err := s.requireDependencies(); err != nil {
		return Session{}, err
	}

	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Session{}, err
	}

	cartID = strings.TrimSpace(cartID)

	if cartID == "" {
		return Session{}, cart.ErrCartNotFound
	}

	// Self-heal first. A session whose TTL has passed still owns the cart
	// until something settles it, and the customer's own retry must not be
	// the one request that stays blocked by it.
	if err = s.sweepCartSessions(ctx, customerID, cartID); err != nil {
		return Session{}, err
	}

	current, err := cart.ReadOwnedCart(
		ctx,
		s.store.db,
		customerID,
		cartID,
	)
	if err != nil {
		return Session{}, err
	}

	if current.Status == cart.StatusCheckoutInProgress {
		existing, liveErr := loadLiveSessionForCart(
			ctx,
			s.store.db,
			cartID,
			false,
		)

		if liveErr == nil &&
			existing.CustomerID == customerID &&
			!existing.Expired(s.now()) {
			return existing, nil
		}

		if liveErr != nil && !errors.Is(liveErr, ErrSessionNotFound) {
			return Session{}, liveErr
		}

		return Session{}, ErrCheckoutConflict
	}

	if current.Status != cart.StatusActive {
		return Session{}, CartPreconditionError{
			Code: PreconditionCartNotActive,
		}
	}

	if current.Version != expectedVersion {
		return Session{}, cart.VersionConflictError{
			Expected: expectedVersion,
			Actual:   current.Version,
		}
	}

	if len(current.Items) == 0 {
		return Session{}, CartPreconditionError{
			Code: PreconditionCartEmpty,
		}
	}

	if current.Address == nil ||
		strings.TrimSpace(current.Address.SourceAddressRef) == "" {
		// A checkout candidate that does not know where it is going cannot
		// be grouped, quoted or fulfilled. This is a cart precondition, not
		// a line-level validation outcome.
		return Session{}, CartPreconditionError{
			Code: PreconditionDeliveryAddress,
		}
	}

	outcomes, err := s.revalidateLines(ctx, current)

	if err != nil {
		return Session{}, err
	}

	groups, failureCode := s.assemble(current, outcomes)

	if failureCode != "" {
		return s.persistFailedValidation(
			ctx,
			customerID,
			current,
			groups,
			failureCode,
		)
	}

	return s.reserveAndFinalize(
		ctx,
		customerID,
		current,
		groups,
	)
}

// revalidateLines re-resolves every cart line against its owning authority.
//
// Every line in one session is evaluated at the same instant, so a service
// window cannot close between the first line and the last.
func (s *Service) revalidateLines(
	ctx context.Context,
	current cart.Cart,
) ([]lineOutcome, error) {
	at := s.now()

	outcomes := make([]lineOutcome, 0, len(current.Items))

	for _, item := range current.Items {
		request := LineRequest{
			CartItemID: item.ID,
			Domain:     item.Domain,

			ItemRef:     item.ItemRef,
			VariantRef:  item.VariantRef,
			MenuRef:     item.MenuRef,
			ListingRef:  item.ListingRef,
			SellUnitRef: item.SellUnitRef,
			SizeRef:     item.SizeRef,

			Quantity: item.Quantity,

			PricingBasis:            item.PricingBasis,
			RequestedMeasureRef:     item.RequestedMeasureRef,
			ExpectedMeasureQuantity: item.ExpectedMeasureQuantity,

			At: at,
		}

		for _, option := range item.Options {
			request.Options = append(
				request.Options,
				OptionRequest{
					GroupRef:  option.GroupRef,
					OptionRef: option.OptionRef,
					Quantity:  option.Quantity,
				},
			)
		}

		var (
			decision LineDecision
			err      error
		)

		switch item.Domain {
		case cart.DomainFood:
			decision, err = s.food.RevalidateFoodLine(ctx, request)

		case cart.DomainShop:
			decision, err = s.shop.RevalidateShopLine(ctx, request)

		default:
			return nil, cart.ErrDomainConflict
		}

		if err != nil {
			// The authority itself could not answer. Failing the whole
			// request is correct: a checkout candidate assembled from a
			// partially-unreachable catalogue would be a guess.
			return nil, err
		}

		outcome, err := buildLineOutcome(item, decision)

		if err != nil {
			return nil, err
		}

		outcomes = append(outcomes, outcome)
	}

	return outcomes, nil
}

// buildLineOutcome turns one authoritative decision into a persisted line.
func buildLineOutcome(
	item cart.CartItem,
	decision LineDecision,
) (lineOutcome, error) {
	line := Line{
		ID:         newID("chkl_"),
		CartItemID: item.ID,
		Domain:     item.Domain,

		// Grouping evidence defaults to the cart's own server-resolved
		// values so that even a refused line is still attributable.
		BusinessID: item.BusinessID,
		BranchID:   item.BranchID,

		ItemRef:     item.ItemRef,
		VariantRef:  item.VariantRef,
		MenuRef:     item.MenuRef,
		ListingRef:  item.ListingRef,
		SellUnitRef: item.SellUnitRef,

		Quantity: item.Quantity,

		SnapshotUnitPriceMinor:    item.UnitPriceMinor,
		SnapshotPriceReference:    item.PriceReference,
		SnapshotCatalogueRevision: item.CatalogueRevision,

		PricingBasis:            item.PricingBasis,
		RequestedMeasureRef:     item.RequestedMeasureRef,
		ExpectedMeasureQuantity: item.ExpectedMeasureQuantity,

		ReservationBasis: BasisNone,

		Status: LineUnavailable,
	}

	if !decision.Sellable {
		line.ReasonCode = firstNonEmpty(
			decision.ReasonCode,
			ReasonAuthorityUnavailable,
		)
		line.ReasonDetail = decision.ReasonDetail

		return lineOutcome{line: line}, nil
	}

	// A sellable answer must still be coherent with the cart's grouping
	// evidence. A listing that moved branch is not the line the customer put
	// in the cart.
	if decision.BusinessID != item.BusinessID ||
		decision.BranchID != item.BranchID {
		line.Status = LineInvalid
		line.ReasonCode = ReasonGroupingViolation
		line.ReasonDetail = "The catalogue item no longer belongs to the vendor branch it was added from."

		return lineOutcome{line: line}, nil
	}

	line.DisplayName = decision.DisplayName
	line.UnitPriceMinor = decision.UnitPriceMinor
	line.Currency = decision.Currency
	line.PriceReference = decision.PriceReference
	line.CatalogueRevision = decision.CatalogueRevision
	line.InventoryRevision = decision.InventoryRevision

	line.MeasureUnderfillBps = decision.MeasureUnderfillBps
	line.MeasureOverfillBps = decision.MeasureOverfillBps

	line.PriceChanged =
		decision.UnitPriceMinor != item.UnitPriceMinor ||
			decision.PriceReference != item.PriceReference

	line.CatalogueChanged =
		decision.CatalogueRevision != item.CatalogueRevision

	quoted, maxAuthorized, err := LineTotals(
		decision.UnitPriceMinor,
		item.Quantity,
		item.PricingBasis,
		decision.MeasureOverfillBps,
	)
	if err != nil {
		return lineOutcome{}, err
	}

	line.QuotedTotalMinor = quoted
	line.MaxAuthorizedTotalMinor = maxAuthorized

	line.Status = LineValid
	line.ReasonCode = ReasonSellable

	if line.PriceChanged || line.CatalogueChanged {
		line.Status = LineChanged

		line.ReasonCode = ReasonCatalogueChanged

		if line.PriceChanged {
			line.ReasonCode = ReasonPriceChanged
		}
	}

	if item.Domain != cart.DomainShop ||
		decision.ReservationBasis == BasisNone {
		// Food reserves nothing in Section 17B, and a Shop line whose basis
		// the authority left empty is not silently treated as unstocked.
		if item.Domain == cart.DomainShop {
			line.Status = LineInvalid
			line.ReasonCode = ReasonReservationBasisUnknown
			line.ReasonDetail = "This item's stock reservation basis could not be established authoritatively."
		}

		return lineOutcome{line: line}, nil
	}

	if strings.TrimSpace(decision.InventoryItemRef) == "" {
		line.Status = LineInvalid
		line.ReasonCode = ReasonInventoryIdentityMissing
		line.ReasonDetail = "This item has no generic Inventory identity in this branch."

		return lineOutcome{line: line}, nil
	}

	line.InventoryItemRef = decision.InventoryItemRef
	line.ReservationBasis = decision.ReservationBasis
	line.ReservationQuantity = decision.ReservationQuantity

	if line.ReservationQuantity <= 0 {
		line.Status = LineInvalid
		line.ReasonCode = ReasonReservationBasisUnknown
		line.ReasonDetail = "The stock quantity this line requires could not be computed."
		line.InventoryItemRef = ""
		line.ReservationBasis = BasisNone
		line.ReservationQuantity = 0

		return lineOutcome{line: line}, nil
	}

	return lineOutcome{line: line, requiresStock: true}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// assemble groups revalidated lines by Vendor business and branch and decides
// whether the candidate can proceed.
//
// The returned failure code is empty only when every line is sellable, one
// currency is in play, and Food's single-branch rule still holds.
func (s *Service) assemble(
	current cart.Cart,
	outcomes []lineOutcome,
) ([]Group, string) {
	type key struct {
		business string
		branch   string
	}

	order := make([]key, 0, 4)
	byKey := make(map[key]*Group, 4)

	failureCode := ""
	currency := ""

	for index := range outcomes {
		outcome := &outcomes[index]
		line := &outcome.line

		if line.Status.Blocking() && failureCode == "" {
			failureCode = FailureLineNotSellable
		}

		if line.Currency != "" {
			if currency == "" {
				currency = line.Currency
			} else if currency != line.Currency {
				failureCode = FailureCurrencyMismatch

				line.Status = LineInvalid
				line.ReasonCode = ReasonCurrencyMismatch
				line.ReasonDetail = "This checkout mixes more than one currency."
			}
		}

		k := key{business: line.BusinessID, branch: line.BranchID}

		group, found := byKey[k]

		if !found {
			group = &Group{
				ID:         newID("chkg_"),
				Domain:     current.Domain,
				BusinessID: k.business,
				BranchID:   k.branch,

				ReservationStatus: ReservationNotRequired,
			}

			group.ReservationSourceRef = group.ID

			byKey[k] = group
			order = append(order, k)
		}

		line.GroupID = group.ID

		if outcome.requiresStock {
			group.RequiresStockReservation = true
		}

		if line.CatalogueRevision > group.CatalogueRevision {
			group.CatalogueRevision = line.CatalogueRevision
		}

		if line.InventoryRevision != nil &&
			(group.InventoryRevision == nil ||
				*line.InventoryRevision > *group.InventoryRevision) {
			revision := *line.InventoryRevision
			group.InventoryRevision = &revision
		}

		if group.Currency == "" {
			group.Currency = line.Currency
		}

		group.Lines = append(group.Lines, *line)
	}

	// Section 17A binds a Food cart to exactly one business and branch. If
	// authoritative revalidation produced more than one Food group, the cart
	// evidence and the catalogue disagree and neither may be guessed at.
	if current.Domain == cart.DomainFood && len(order) > 1 {
		failureCode = FailureCartGroupingViolation
	}

	sort.Slice(order, func(i, j int) bool {
		if order[i].business == order[j].business {
			return order[i].branch < order[j].branch
		}

		return order[i].business < order[j].business
	})

	groups := make([]Group, 0, len(order))

	for _, k := range order {
		group := byKey[k]

		if failureCode != "" {
			// Nothing is reserved for a candidate that is not going to be
			// validated, so the durable row must not claim it will be.
			group.RequiresStockReservation = false
			group.ReservationSourceRef = ""
			group.ReservationStatus = ReservationNotRequired
		} else if group.RequiresStockReservation {
			group.ReservationStatus = ReservationPending
		}

		quoted := make([]int64, 0, len(group.Lines))
		maximum := make([]int64, 0, len(group.Lines))

		for _, line := range group.Lines {
			quoted = append(quoted, line.QuotedTotalMinor)
			maximum = append(maximum, line.MaxAuthorizedTotalMinor)
		}

		quotedTotal, quotedErr := SumTotals(quoted...)
		maximumTotal, maximumErr := SumTotals(maximum...)

		if quotedErr != nil || maximumErr != nil {
			failureCode = FailureLineNotSellable
			quotedTotal = 0
			maximumTotal = 0
		}

		group.QuotedTotalMinor = quotedTotal
		group.MaxAuthorizedTotalMinor = maximumTotal

		groups = append(groups, *group)
	}

	return groups, failureCode
}

// -----------------------------------------------------------------------------
// Persistence of the validation outcome
// -----------------------------------------------------------------------------

func newSessionFrom(
	customerID string,
	current cart.Cart,
	groups []Group,
	ttl time.Duration,
	now time.Time,
) (Session, error) {
	session := Session{
		ID:         newID("chks_"),
		CustomerID: customerID,

		CartID:      current.ID,
		CartVersion: current.Version,

		Domain: current.Domain,

		ReservationTTLSeconds: int(ttl / time.Second),

		ExpiresAt: now.Add(ttl),

		ReservationCoordinationState: CoordinationNone,

		Version: 1,
	}

	if current.Address != nil {
		session.AddressRef = current.Address.SourceAddressRef

		snapshottedAt := current.Address.SnapshottedAt
		session.AddressSnapshottedAt = &snapshottedAt
	}

	quoted := make([]int64, 0, len(groups))
	maximum := make([]int64, 0, len(groups))

	for _, group := range groups {
		quoted = append(quoted, group.QuotedTotalMinor)
		maximum = append(maximum, group.MaxAuthorizedTotalMinor)

		if session.Currency == "" {
			session.Currency = group.Currency
		}

		for _, line := range group.Lines {
			if line.PriceChanged {
				session.PriceChanged = true
			}

			if line.CatalogueChanged {
				session.CatalogueChanged = true
			}
		}
	}

	quotedTotal, err := SumTotals(quoted...)

	if err != nil {
		return Session{}, err
	}

	maximumTotal, err := SumTotals(maximum...)

	if err != nil {
		return Session{}, err
	}

	session.QuotedItemsTotalMinor = quotedTotal
	session.MaxAuthorizedItemsTotalMinor = maximumTotal

	return session, nil
}

// persistFailedValidation stores a refused candidate without touching the
// cart.
//
// The cart stays ACTIVE and mutable: the whole point of the per-line reasons
// is that the customer can fix the cart and try again.
func (s *Service) persistFailedValidation(
	ctx context.Context,
	customerID string,
	current cart.Cart,
	groups []Group,
	failureCode string,
) (Session, error) {
	now := s.now()

	session, err := newSessionFrom(
		customerID,
		current,
		groups,
		s.ttl,
		now,
	)
	if err != nil {
		return Session{}, err
	}

	session.Status = StatusFailed
	session.FailureCode = failureCode

	// The totals are left as computed rather than zeroed. They describe the
	// part of the cart that IS sellable, and zeroing only the session and its
	// groups would leave them contradicting their own lines - a refused
	// candidate that does not add up is harder to debug than one that does.
	settledAt := now
	session.SettledAt = &settledAt

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Session{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	session.ValidationRevision, err = nextValidationRevision(
		ctx,
		tx,
		current.ID,
	)
	if err != nil {
		return Session{}, err
	}

	if err = insertSession(ctx, tx, session); err != nil {
		return Session{}, err
	}

	for _, group := range groups {
		if err = insertGroup(
			ctx,
			tx,
			session.ID,
			customerID,
			group,
		); err != nil {
			return Session{}, err
		}

		for _, line := range group.Lines {
			if err = insertLine(
				ctx,
				tx,
				session.ID,
				customerID,
				line,
			); err != nil {
				return Session{}, err
			}
		}
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		customerID,
		EventValidationFailed,
		"",
		failureCode,
		session.Version,
	); err != nil {
		return Session{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}

	return loadOwnedSession(
		ctx,
		s.store.db,
		customerID,
		session.ID,
		false,
	)
}

// reserveAndFinalize commits the checkout candidate, then coordinates stock.
//
// The session and every group row - including the reservation source
// reference each group will use - are committed BEFORE the first Inventory
// call. That ordering is the whole recovery story: after this commit there is
// always a durable record naming every reservation that might exist.
func (s *Service) reserveAndFinalize(
	ctx context.Context,
	customerID string,
	current cart.Cart,
	groups []Group,
) (Session, error) {
	now := s.now()

	session, err := newSessionFrom(
		customerID,
		current,
		groups,
		s.ttl,
		now,
	)
	if err != nil {
		return Session{}, err
	}

	session.Status = StatusValidating

	needsReservation := false

	for _, group := range groups {
		if group.RequiresStockReservation {
			needsReservation = true
			break
		}
	}

	if needsReservation {
		session.ReservationCoordinationState = CoordinationInProgress
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Session{}, err
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	locked, err := cart.LockOwnedCartForCheckoutTx(
		ctx,
		tx,
		customerID,
		current.ID,
	)
	if err != nil {
		return Session{}, err
	}

	// Nothing decided against the unlocked read is trusted. A cart mutation
	// that landed in between bumped the version, so this is what makes
	// "cart mutation during checkout validation" a conflict rather than a
	// silently stale candidate.
	if locked.Version != current.Version {
		return Session{}, cart.VersionConflictError{
			Expected: current.Version,
			Actual:   locked.Version,
		}
	}

	if _, err = cart.BeginCheckoutTx(
		ctx,
		tx,
		locked,
		current.Version,
	); err != nil {
		return Session{}, err
	}

	session.ValidationRevision, err = nextValidationRevision(
		ctx,
		tx,
		current.ID,
	)
	if err != nil {
		return Session{}, err
	}

	if err = insertSession(ctx, tx, session); err != nil {
		return Session{}, err
	}

	for _, group := range groups {
		if err = insertGroup(
			ctx,
			tx,
			session.ID,
			customerID,
			group,
		); err != nil {
			return Session{}, err
		}

		for _, line := range group.Lines {
			if err = insertLine(
				ctx,
				tx,
				session.ID,
				customerID,
				line,
			); err != nil {
				return Session{}, err
			}
		}

		if group.RequiresStockReservation {
			if err = appendEvent(
				ctx,
				tx,
				session.ID,
				customerID,
				EventReservationRequested,
				group.ID,
				group.ReservationSourceRef,
				session.Version,
			); err != nil {
				return Session{}, err
			}
		}
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		customerID,
		EventValidationStarted,
		"",
		"",
		session.Version,
	); err != nil {
		return Session{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}

	committed = true

	return s.coordinateReservations(
		ctx,
		customerID,
		session,
		groups,
	)
}

// coordinateReservations holds stock for every group that needs it.
//
// Groups are visited in a stable business/branch order, so two customers
// racing over the same pair of vendors approach them in the same sequence.
func (s *Service) coordinateReservations(
	ctx context.Context,
	customerID string,
	session Session,
	groups []Group,
) (Session, error) {
	failureCode := ""

	for _, group := range groups {
		if !group.RequiresStockReservation {
			continue
		}

		lines := make([]ReserveLine, 0, len(group.Lines))

		for _, line := range group.Lines {
			if line.ReservationBasis == BasisNone {
				continue
			}

			lines = append(
				lines,
				ReserveLine{
					InventoryItemID: line.InventoryItemRef,
					Quantity:        line.ReservationQuantity,
				},
			)
		}

		if len(lines) == 0 {
			continue
		}

		result, err := s.stock.Reserve(
			ctx,
			ReserveRequest{
				BusinessID:    group.BusinessID,
				BranchID:      group.BranchID,
				SourceRef:     group.ReservationSourceRef,
				ExpiresAt:     session.ExpiresAt,
				CorrelationID: session.ID,
				Lines:         lines,
			},
		)

		if err != nil {
			if !errors.Is(err, ErrStockUnavailable) {
				// The Inventory authority itself failed. The durable
				// PENDING row means a reservation may still exist, so the
				// session is compensated rather than left dangling.
				if _, settleErr := s.settle(
					ctx,
					customerID,
					session.ID,
					StatusFailed,
					FailureReservationFailed,
					EventValidationFailed,
				); settleErr != nil {
					return Session{}, settleErr
				}

				return Session{}, err
			}

			failureCode = FailureStockUnavailable

			if markErr := s.markGroupReservationFailed(
				ctx,
				session,
				group.ID,
				ReasonStockUnavailable,
			); markErr != nil {
				return Session{}, markErr
			}

			break
		}

		if err = s.markGroupReserved(
			ctx,
			session,
			group.ID,
			result,
		); err != nil {
			return Session{}, err
		}
	}

	if failureCode != "" {
		return s.settle(
			ctx,
			customerID,
			session.ID,
			StatusFailed,
			failureCode,
			EventValidationFailed,
		)
	}

	return s.markValidated(ctx, customerID, session.ID)
}

func (s *Service) markGroupReserved(
	ctx context.Context,
	session Session,
	groupID string,
	result ReserveResult,
) error {
	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_checkout_groups
        SET reservation_status='RESERVED',
            inventory_reservation_ref=$3,
            reservation_expires_at=$4,
            reservation_failure_code='',
            updated_at=now()
        WHERE id=$1
          AND session_id=$2
          AND reservation_status='PENDING'
        `,
		groupID,
		session.ID,
		result.ReservationRef,
		result.ExpiresAt,
	); err != nil {
		return err
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		session.CustomerID,
		EventReservationHeld,
		groupID,
		result.ReservationRef,
		session.Version,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) markGroupReservationFailed(
	ctx context.Context,
	session Session,
	groupID string,
	reasonCode string,
) error {
	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_checkout_groups
        SET reservation_status='FAILED',
            reservation_failure_code=$3,
            updated_at=now()
        WHERE id=$1
          AND session_id=$2
          AND reservation_status='PENDING'
        `,
		groupID,
		session.ID,
		reasonCode,
	); err != nil {
		return err
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		session.CustomerID,
		EventReservationFailed,
		groupID,
		reasonCode,
		session.Version,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// markValidated closes a successful coordination.
//
// The status change is guarded so a concurrent expiry or cancellation cannot
// be overwritten by a late success: if the guard refuses, the session was
// settled by something else and that settlement stands.
func (s *Service) markValidated(
	ctx context.Context,
	customerID string,
	sessionID string,
) (Session, error) {
	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Session{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var version int64

	err = tx.QueryRow(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET status='VALIDATED',
            -- A candidate that never needed a hold never coordinated one, so
            -- it must not claim it completed coordination it never started.
            reservation_coordination_state=
                CASE
                    WHEN reservation_coordination_state='IN_PROGRESS'
                    THEN 'COMPLETE'
                    ELSE reservation_coordination_state
                END,
            validated_at=now(),
            version=version+1,
            updated_at=now()
        WHERE id=$1
          AND customer_id=$2
          AND status='VALIDATING'
        RETURNING version
        `,
		sessionID,
		customerID,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)

		return loadOwnedSession(
			ctx,
			s.store.db,
			customerID,
			sessionID,
			false,
		)
	}

	if err != nil {
		return Session{}, err
	}

	if err = appendEvent(
		ctx,
		tx,
		sessionID,
		customerID,
		EventValidated,
		"",
		"",
		version,
	); err != nil {
		return Session{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}

	return loadOwnedSession(
		ctx,
		s.store.db,
		customerID,
		sessionID,
		false,
	)
}

// -----------------------------------------------------------------------------
// Settlement: compensation, cancellation, expiry and crash recovery
// -----------------------------------------------------------------------------

// settle drives a live session to a terminal state, releasing every hold it
// might own and returning the cart to the customer.
//
// It is the single path used by validation failure, cancellation, TTL expiry
// and crash recovery, and it is idempotent at every step: a reservation that
// is already terminal, a group already released and a cart already ACTIVE are
// all successes. That is deliberate - four different callers can legally race
// here, and none of them may leave a hold behind or wedge a cart.
func (s *Service) settle(
	ctx context.Context,
	customerID string,
	sessionID string,
	target Status,
	failureCode string,
	eventAction string,
) (Session, error) {
	session, err := loadOwnedSession(
		ctx,
		s.store.db,
		customerID,
		sessionID,
		false,
	)
	if err != nil {
		return Session{}, err
	}

	if !session.Status.Live() {
		return session, nil
	}

	if err = s.beginCompensation(ctx, session); err != nil {
		return Session{}, err
	}

	for _, group := range session.Groups {
		if !group.RequiresStockReservation {
			continue
		}

		// FAILED is included deliberately. Section 14 refuses all-or-nothing,
		// so a refused reservation should have created nothing - but "should
		// have" is not evidence, and a leaked hold costs a vendor real
		// sellable stock. Asking is cheap; releaseGroup resolves from the
		// durable source reference and its guarded update leaves a FAILED
		// group's reason code intact.
		switch group.ReservationStatus {
		case ReservationPending,
			ReservationReserved,
			ReservationFailed:
		default:
			continue
		}

		if err = s.releaseGroup(ctx, session, group); err != nil {
			return Session{}, err
		}
	}

	return s.closeSession(
		ctx,
		session,
		target,
		failureCode,
		eventAction,
	)
}

func (s *Service) beginCompensation(
	ctx context.Context,
	session Session,
) error {
	holds := false

	for _, group := range session.Groups {
		if group.RequiresStockReservation &&
			group.ReservationStatus != ReservationReleased {
			holds = true
			break
		}
	}

	if !holds {
		return nil
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET reservation_coordination_state='COMPENSATING',
            updated_at=now()
        WHERE id=$1
          AND reservation_coordination_state IN ('IN_PROGRESS','COMPLETE','COMPENSATING','ORPHANED')
        `,
		session.ID,
	); err != nil {
		return err
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		session.CustomerID,
		EventCompensationStarted,
		"",
		"",
		session.Version,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// releaseGroup returns one group's stock, resolving the reservation from the
// durable source reference when the identity was never recorded.
//
// A PENDING group is exactly the crash case: the reservation may or may not
// exist, and only Section 14 knows.
func (s *Service) releaseGroup(
	ctx context.Context,
	session Session,
	group Group,
) error {
	reference := strings.TrimSpace(group.InventoryReservationRef)

	if reference == "" {
		resolved, err := s.stock.FindBySource(
			ctx,
			group.BusinessID,
			group.ReservationSourceRef,
		)
		if err != nil {
			return err
		}

		reference = strings.TrimSpace(resolved)
	}

	if reference != "" {
		if err := s.stock.Release(
			ctx,
			group.BusinessID,
			group.BranchID,
			reference,
		); err != nil {
			return err
		}
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_checkout_groups
        SET reservation_status='RELEASED',
            inventory_reservation_ref=$3,
            updated_at=now()
        WHERE id=$1
          AND session_id=$2
          AND reservation_status IN ('PENDING','RESERVED')
        `,
		group.ID,
		session.ID,
		reference,
	); err != nil {
		return err
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		session.CustomerID,
		EventReservationReleased,
		group.ID,
		reference,
		session.Version,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// closeSession writes the terminal state and hands the cart back.
func (s *Service) closeSession(
	ctx context.Context,
	session Session,
	target Status,
	failureCode string,
	eventAction string,
) (Session, error) {
	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Session{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	coordination := CoordinationNone

	if session.ReservationCoordinationState != CoordinationNone {
		coordination = CoordinationCompensated
	}

	var version int64

	err = tx.QueryRow(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET status=$2,
            failure_code=$3,
            reservation_coordination_state=$4,
            settled_at=now(),
            version=version+1,
            updated_at=now()
        WHERE id=$1
          AND status IN ('VALIDATING','VALIDATED')
        RETURNING version
        `,
		session.ID,
		target,
		failureCode,
		coordination,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		// Something else settled it first. Its settlement stands, and the
		// cart it released stays released.
		_ = tx.Rollback(ctx)

		return loadOwnedSession(
			ctx,
			s.store.db,
			session.CustomerID,
			session.ID,
			false,
		)
	}

	if err != nil {
		return Session{}, err
	}

	if _, err = cart.ReleaseCheckoutTx(
		ctx,
		tx,
		session.CustomerID,
		session.CartID,
	); err != nil {
		return Session{}, err
	}

	if err = appendEvent(
		ctx,
		tx,
		session.ID,
		session.CustomerID,
		eventAction,
		"",
		failureCode,
		version,
	); err != nil {
		return Session{}, err
	}

	if session.ReservationCoordinationState != CoordinationNone {
		if err = appendEvent(
			ctx,
			tx,
			session.ID,
			session.CustomerID,
			EventCompensationCompleted,
			"",
			"",
			version,
		); err != nil {
			return Session{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}

	return loadOwnedSession(
		ctx,
		s.store.db,
		session.CustomerID,
		session.ID,
		false,
	)
}

// -----------------------------------------------------------------------------
// Reads, cancellation and recovery
// -----------------------------------------------------------------------------

// GetSession reads one customer-owned session, settling it first if its
// reservation TTL has already elapsed.
//
// A live session past its TTL is never reported as valid: the stock behind it
// is either already back in Section 14's available pool or about to be, and
// telling a client otherwise would be the one lie this section cannot afford.
func (s *Service) GetSession(
	ctx context.Context,
	customerID string,
	sessionID string,
) (Session, error) {
	if err := s.requireDependencies(); err != nil {
		return Session{}, err
	}

	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Session{}, err
	}

	session, err := loadOwnedSession(
		ctx,
		s.store.db,
		customerID,
		strings.TrimSpace(sessionID),
		false,
	)
	if err != nil {
		return Session{}, err
	}

	if session.Status.Live() && session.Expired(s.now()) {
		return s.settle(
			ctx,
			customerID,
			session.ID,
			StatusExpired,
			"",
			EventExpired,
		)
	}

	return session, nil
}

// CancelSession releases a live checkout candidate at the customer's request.
func (s *Service) CancelSession(
	ctx context.Context,
	customerID string,
	sessionID string,
) (Session, error) {
	if err := s.requireDependencies(); err != nil {
		return Session{}, err
	}

	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Session{}, err
	}

	session, err := loadOwnedSession(
		ctx,
		s.store.db,
		customerID,
		strings.TrimSpace(sessionID),
		false,
	)
	if err != nil {
		return Session{}, err
	}

	if !session.Status.Live() {
		return Session{}, ErrSessionNotLive
	}

	return s.settle(
		ctx,
		customerID,
		session.ID,
		StatusCancelled,
		"",
		EventCancelled,
	)
}

// sweepCartSessions settles any live session for one cart whose TTL has
// already elapsed.
//
// This is what stops a lapsed checkout from wedging its own customer's cart
// between the TTL passing and the recovery worker's next tick.
func (s *Service) sweepCartSessions(
	ctx context.Context,
	customerID string,
	cartID string,
) error {
	session, err := loadLiveSessionForCart(
		ctx,
		s.store.db,
		cartID,
		false,
	)

	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	if session.CustomerID != customerID {
		// The cart is customer-scoped, so this cannot happen through the
		// public surface. Refusing rather than settling another customer's
		// session is still the only safe answer.
		return ErrSessionNotFound
	}

	if !session.Expired(s.now()) {
		return nil
	}

	_, err = s.settle(
		ctx,
		customerID,
		session.ID,
		StatusExpired,
		"",
		EventExpired,
	)

	return err
}

// SweepDueSessions is the Section 17B recovery pass.
//
// It closes two holes that nothing else can: a session whose reservation TTL
// elapsed while nobody was looking, and a session whose coordination was
// interrupted by a process that never came back. Both end the same way -
// every hold released, the cart handed back - because a customer must never
// be left holding stock they cannot buy or a cart they cannot edit.
func (s *Service) SweepDueSessions(
	ctx context.Context,
	limit int,
) (int, error) {
	if err := s.requireDependencies(); err != nil {
		return 0, err
	}

	if limit <= 0 || limit > MaxSweepBatch {
		limit = MaxSweepBatch
	}

	now := s.now()

	type candidate struct {
		id         string
		customerID string
		orphaned   bool
	}

	rows, err := s.store.db.Query(
		ctx,
		`
        SELECT id, customer_id, false
        FROM commerce_checkout_sessions
        WHERE status IN ('VALIDATING','VALIDATED')
          AND expires_at <= $1

        UNION ALL

        SELECT id, customer_id, true
        FROM commerce_checkout_sessions
        WHERE status='VALIDATING'
          AND expires_at > $1
          AND reservation_coordination_state IN ('IN_PROGRESS','COMPENSATING')
          AND updated_at < $2

        LIMIT $3
        `,
		now,
		now.Add(-s.grace),
		limit,
	)
	if err != nil {
		return 0, err
	}

	candidates := make([]candidate, 0, limit)

	for rows.Next() {
		var item candidate

		if err = rows.Scan(
			&item.id,
			&item.customerID,
			&item.orphaned,
		); err != nil {
			rows.Close()
			return 0, err
		}

		candidates = append(candidates, item)
	}

	rows.Close()

	if err = rows.Err(); err != nil {
		return 0, err
	}

	settled := 0

	var firstErr error

	for _, item := range candidates {
		target := StatusExpired
		failureCode := ""
		action := EventExpired

		if item.orphaned {
			target = StatusFailed
			failureCode = FailureCoordinationOrphaned
			action = EventOrphaned

			if err = s.markOrphaned(ctx, item.id); err != nil {
				if firstErr == nil {
					firstErr = err
				}

				continue
			}
		}

		if _, err = s.settle(
			ctx,
			item.customerID,
			item.id,
			target,
			failureCode,
			action,
		); err != nil {
			// One stuck session must not stop the sweep from healing the
			// rest of them.
			if firstErr == nil {
				firstErr = err
			}

			continue
		}

		settled++
	}

	return settled, firstErr
}

func (s *Service) markOrphaned(
	ctx context.Context,
	sessionID string,
) error {
	_, err := s.store.db.Exec(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET reservation_coordination_state='ORPHANED',
            updated_at=now()
        WHERE id=$1
          AND status='VALIDATING'
          AND reservation_coordination_state IN ('IN_PROGRESS','COMPENSATING')
        `,
		sessionID,
	)

	return err
}
