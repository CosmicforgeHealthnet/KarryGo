package main

// Section 17E adapters.
//
// The orders feature never imports Section 17D's package. It receives an
// already-verified approved quote through this adapter, exactly as Section 17C
// receives checkout evidence through its own.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
)

// section17EScopeAdapter implements orders.ScopeResolver over the Section 5
// authorization evaluator, so order permissions are the same grants every
// other vendor surface evaluates.
type section17EScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a section17EScopeAdapter) ResolveScope(
	ctx context.Context, staffID, permission string,
) (orders.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return orders.Scope{}, err
	}
	var scope orders.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			// A vendor-wide grant subsumes every branch-scoped one.
			return orders.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

// section17EQuoteAuthority resolves an approved Section 17D quote into order
// source evidence.
//
// It reads the quote tables directly rather than importing the checkoutquote
// feature, for the same reason every other Section 17 adapter lives here: the
// orders feature must not learn Section 17D's internals, and Section 17D must
// learn nothing about orders.
type section17EQuoteAuthority struct {
	db *pgxpool.Pool
}

func newSection17EQuoteAuthority(db *pgxpool.Pool) *section17EQuoteAuthority {
	return &section17EQuoteAuthority{db: db}
}

// LoadApprovedQuote implements orders.QuoteAuthority.
//
// It refuses unless, at THIS moment:
//
//   - the quote exists, belongs to this customer, is approved and unexpired;
//   - the checkout session is still VALIDATED at the same validation revision
//     and has not lapsed;
//   - every quoted group still holds the same Section 14 reservation;
//   - the fulfilment plan is still PLANNED at the same revision and version.
//
// Approval and order creation are two separate commitments at two different
// moments, so the second one re-verifies rather than trusting the first.
func (a *section17EQuoteAuthority) LoadApprovedQuote(
	ctx context.Context, customerID, quoteID string, now time.Time,
) (orders.QuoteSource, error) {
	if a == nil || a.db == nil {
		return orders.QuoteSource{}, orders.ErrDependencyMissing
	}

	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return orders.QuoteSource{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := loadSection17EQuoteSource(ctx, tx, customerID, quoteID, now)
	if err != nil {
		return orders.QuoteSource{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return orders.QuoteSource{}, err
	}
	return source, nil
}

type section17EQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadSection17EQuoteSource(
	ctx context.Context,
	q section17EQueryer,
	customerID, quoteID string,
	now time.Time,
) (orders.QuoteSource, error) {
	var source orders.QuoteSource
	var addressJSON, fundingJSON, disclosureJSON []byte
	var approvedAt *time.Time

	// The join to commerce_quote_approvals is an inner condition of "orderable":
	// an unapproved quote is not an agreement anybody committed to.
	err := q.QueryRow(ctx, `
		SELECT q.id,q.quote_version,q.quote_fingerprint,q.expires_at,a.approved_at,
		       q.customer_id,q.checkout_session_id,q.checkout_validation_revision,
		       q.cart_id,q.cart_version,q.fulfilment_plan_id,q.fulfilment_plan_revision,
		       q.fulfilment_plan_version,s.domain,q.currency,q.selected_fulfilment_mode,
		       q.address_snapshot,q.product_subtotal_minor,q.max_product_subtotal_minor,
		       q.delivery_subtotal_minor,q.fee_subtotal_minor,q.tax_subtotal_minor,
		       q.discount_total_minor,q.subsidy_total_minor,q.quoted_total_minor,
		       q.max_authorized_total_minor,q.funding_breakdown,q.price_disclosure_snapshot
		  FROM commerce_quotes q
		  JOIN commerce_quote_approvals a ON a.quote_id=q.id AND a.customer_id=q.customer_id
		  JOIN commerce_checkout_sessions s
		    ON s.id=q.checkout_session_id AND s.customer_id=q.customer_id
		  JOIN commerce_fulfilment_plans p
		    ON p.id=q.fulfilment_plan_id AND p.customer_id=q.customer_id
		 WHERE q.id=$1 AND q.customer_id=$2
		   AND q.expires_at>$3
		   AND s.status='VALIDATED' AND s.failure_code=''
		   AND s.validation_revision=q.checkout_validation_revision
		   AND s.cart_id=q.cart_id AND s.cart_version=q.cart_version
		   AND s.expires_at>$3
		   AND p.status='PLANNED' AND p.failure_code=''
		   AND p.checkout_session_id=q.checkout_session_id
		   AND p.checkout_validation_revision=q.checkout_validation_revision
		   AND p.plan_revision=q.fulfilment_plan_revision
		   AND p.version=q.fulfilment_plan_version
		   AND p.expires_at>$3`,
		quoteID, customerID, now).Scan(
		&source.QuoteID, &source.QuoteVersion, &source.QuoteFingerprint, &source.ExpiresAt,
		&approvedAt, &source.CustomerID, &source.CheckoutSessionID,
		&source.CheckoutValidationRevision, &source.CartID, &source.CartVersion,
		&source.FulfilmentPlanID, &source.FulfilmentPlanRevision, &source.FulfilmentPlanVersion,
		&source.Domain, &source.Currency, &source.SelectedFulfilmentMode, &addressJSON,
		&source.ProductSubtotalMinor, &source.MaxProductSubtotalMinor,
		&source.DeliverySubtotalMinor, &source.FeeSubtotalMinor, &source.TaxSubtotalMinor,
		&source.DiscountTotalMinor, &source.SubsidyTotalMinor, &source.FinalPayableMinor,
		&source.MaxAuthorizedTotalMinor, &fundingJSON, &disclosureJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return orders.QuoteSource{}, orders.ErrQuoteNotOrderable
	}
	if err != nil {
		return orders.QuoteSource{}, err
	}
	if approvedAt == nil {
		return orders.QuoteSource{}, orders.ErrQuoteNotOrderable
	}
	source.ApprovedAt = *approvedAt

	if err = json.Unmarshal(addressJSON, &source.Address); err != nil {
		return orders.QuoteSource{}, err
	}
	source.FundingBreakdown = json.RawMessage(fundingJSON)
	source.PriceDisclosureSnapshot = json.RawMessage(disclosureJSON)

	if err = loadSection17EReceiver(ctx, q, &source); err != nil {
		return orders.QuoteSource{}, err
	}
	if source.Groups, err = loadSection17EGroups(ctx, q, source); err != nil {
		return orders.QuoteSource{}, err
	}
	if source.Lines, err = loadSection17ELines(ctx, q, source); err != nil {
		return orders.QuoteSource{}, err
	}
	if source.Adjustments, err = loadSection17EAdjustments(ctx, q, source.QuoteID); err != nil {
		return orders.QuoteSource{}, err
	}
	if len(source.Groups) == 0 || len(source.Lines) == 0 {
		return orders.QuoteSource{}, orders.ErrQuoteNotOrderable
	}
	return source, nil
}

// loadSection17EReceiver snapshots the delivery contact.
//
// Section 17D does not fingerprint the receiver, so the read is bound to the
// exact cart version the quote froze. A receiver change bumps that version and
// a cart in checkout cannot be edited at all, so this can never snapshot a
// contact the customer did not approve.
func loadSection17EReceiver(
	ctx context.Context, q section17EQueryer, source *orders.QuoteSource,
) error {
	var name, phone string
	err := q.QueryRow(ctx, `
		SELECT r.receiver_name,r.receiver_phone
		  FROM commerce_cart_receivers r
		  JOIN commerce_carts c ON c.id=r.cart_id
		 WHERE r.cart_id=$1 AND c.customer_id=$2 AND c.version=$3`,
		source.CartID, source.CustomerID, source.CartVersion).Scan(&name, &phone)
	if errors.Is(err, pgx.ErrNoRows) {
		// No receiver is a normal purchase: the customer is the recipient.
		return nil
	}
	if err != nil {
		return err
	}
	source.HasReceiver = true
	source.Receiver = orders.ReceiverSnapshot{ReceiverName: name, ReceiverPhone: phone}
	return nil
}

// loadSection17EGroups reads each vendor/branch allocation and re-checks that
// the Section 14 reservation the quote named is still the one being held.
func loadSection17EGroups(
	ctx context.Context, q section17EQueryer, source orders.QuoteSource,
) ([]orders.GroupSource, error) {
	rows, err := q.Query(ctx, `
		SELECT g.checkout_group_id,g.business_id,g.branch_id,cg.domain,
		       g.product_subtotal_minor,g.max_product_subtotal_minor,
		       cg.requires_stock_reservation,g.inventory_reservation_ref,
		       g.inventory_reservation_source_ref,g.logistics_plan_group_ref,
		       g.logistics_mission_ref,COALESCE(m.snapshot->>'authority',''),
		       (g.delivery_funding->>'gross_minor')::bigint,
		       (g.delivery_funding->>'customer_minor')::bigint,
		       (g.delivery_funding->>'vendor_funded_minor')::bigint,
		       (g.delivery_funding->>'platform_funded_minor')::bigint,
		       cg.reservation_status
		  FROM commerce_quote_groups g
		  JOIN commerce_checkout_groups cg
		    ON cg.id=g.checkout_group_id AND cg.session_id=$2 AND cg.customer_id=$3
		  -- The authority comes from the quote's OWN frozen mission snapshot,
		  -- never from the live plan row. Delivery responsibility decides who
		  -- gets paid, so it has to be the evidence the customer approved.
		  LEFT JOIN commerce_quote_missions m
		    ON m.quote_id=g.quote_id
		   AND m.fulfilment_mission_id=NULLIF(g.logistics_mission_ref,'')
		 WHERE g.quote_id=$1
		 ORDER BY g.checkout_group_id`,
		source.QuoteID, source.CheckoutSessionID, source.CustomerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []orders.GroupSource
	for rows.Next() {
		var group orders.GroupSource
		var authority, reservationStatus string
		var requiresReservation bool
		if err = rows.Scan(&group.QuoteGroupRef, &group.BusinessID, &group.BranchID, &group.Domain,
			&group.ProductSubtotalMinor, &group.MaxProductSubtotalMinor,
			&requiresReservation, &group.ReservationRef, &group.ReservationSourceRef,
			&group.LogisticsPlanGroupRef, &group.LogisticsMissionRef, &authority,
			&group.DeliveryGrossMinor, &group.DeliveryCustomerMinor,
			&group.DeliveryVendorFundedMinor, &group.DeliveryPlatformFundedMinor,
			&reservationStatus); err != nil {
			return nil, err
		}

		// Stock isolation starts here: a group whose reservation has been
		// released or has failed can no longer become an order, and it fails
		// the whole creation rather than producing an order nobody can honour.
		group.ReservationRequired = requiresReservation
		if requiresReservation && reservationStatus != "RESERVED" {
			return nil, orders.ErrQuoteNotOrderable
		}
		if !requiresReservation && reservationStatus != "NOT_REQUIRED" {
			return nil, orders.ErrQuoteNotOrderable
		}

		group.DeliveryResponsibility = section17EResponsibility(authority, group.DeliveryGrossMinor)
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// section17EResponsibility maps the plan's own authority onto the order's
// delivery responsibility. An unknown or absent authority with a real fee is
// refused by the caller rather than defaulted to somebody.
func section17EResponsibility(authority string, grossMinor int64) orders.DeliveryResponsibility {
	switch authority {
	case "VENDOR_OWN_DELIVERY":
		return orders.DeliveryVendorOwn
	case "PLATFORM_LOGISTICS":
		return orders.DeliveryPlatform
	default:
		if grossMinor > 0 {
			// A fee with no named executor would be money owed to nobody.
			return orders.DeliveryResponsibility("")
		}
		return orders.DeliveryNone
	}
}

// loadSection17ELines reads the approved item snapshots and joins the display
// name from the checkout evidence the quote was built from.
//
// The name is presentation-only and is deliberately not part of Section 17D's
// commercial fingerprint, so reading it here changes no approved figure.
func loadSection17ELines(
	ctx context.Context, q section17EQueryer, source orders.QuoteSource,
) ([]orders.LineSource, error) {
	rows, err := q.Query(ctx, `
		SELECT l.snapshot,COALESCE(cl.display_name,'')
		  FROM commerce_quote_lines l
		  LEFT JOIN commerce_checkout_lines cl
		    ON cl.id=l.checkout_line_id AND cl.session_id=$2 AND cl.customer_id=$3
		 WHERE l.quote_id=$1
		 ORDER BY l.cart_item_id,l.id`,
		source.QuoteID, source.CheckoutSessionID, source.CustomerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []orders.LineSource
	for rows.Next() {
		var raw []byte
		var displayName string
		if err = rows.Scan(&raw, &displayName); err != nil {
			return nil, err
		}

		var snapshot struct {
			CheckoutLineID          string `json:"checkout_line_id"`
			CheckoutGroupID         string `json:"checkout_group_id"`
			CartItemID              string `json:"cart_item_id"`
			Domain                  string `json:"domain"`
			ItemRef                 string `json:"item_ref"`
			VariantRef              string `json:"variant_ref"`
			ListingRef              string `json:"listing_ref"`
			SellUnitRef             string `json:"sell_unit_ref"`
			Quantity                int64  `json:"quantity"`
			UnitPriceMinor          int64  `json:"vendor_online_unit_price_minor"`
			PriceReference          string `json:"price_reference"`
			CatalogueRevision       int64  `json:"catalogue_revision"`
			InventoryRevision       *int64 `json:"inventory_revision"`
			PricingBasis            string `json:"pricing_basis"`
			RequestedMeasureRef     string `json:"requested_measure_ref"`
			ExpectedMeasureQuantity int64  `json:"expected_measure_quantity"`
			MeasureUnderfillBps     int    `json:"measure_underfill_bps"`
			MeasureOverfillBps      int    `json:"measure_overfill_bps"`
			QuotedTotalMinor        int64  `json:"quoted_total_minor"`
			MaxAuthorizedTotalMinor int64  `json:"max_authorized_total_minor"`
			InventoryItemRef        string `json:"inventory_item_ref"`
			OnlinePriceDisclosure   string `json:"online_price_disclosure"`
		}
		if err = json.Unmarshal(raw, &snapshot); err != nil {
			return nil, err
		}

		name := displayName
		if name == "" {
			// A receipt must still name something. The catalogue reference is
			// the honest fallback, never a fabricated product name.
			name = snapshot.ItemRef
		}

		lines = append(lines, orders.LineSource{
			QuoteLineRef:            snapshot.CheckoutLineID,
			QuoteGroupRef:           snapshot.CheckoutGroupID,
			CartItemID:              snapshot.CartItemID,
			ItemName:                name,
			ItemRef:                 snapshot.ItemRef,
			VariantRef:              snapshot.VariantRef,
			ListingRef:              snapshot.ListingRef,
			SellUnitRef:             snapshot.SellUnitRef,
			Domain:                  snapshot.Domain,
			Quantity:                snapshot.Quantity,
			UnitPriceMinor:          snapshot.UnitPriceMinor,
			QuotedTotalMinor:        snapshot.QuotedTotalMinor,
			MaxAuthorizedTotalMinor: snapshot.MaxAuthorizedTotalMinor,
			PricingBasis:            snapshot.PricingBasis,
			RequestedMeasureRef:     snapshot.RequestedMeasureRef,
			ExpectedMeasureQuantity: snapshot.ExpectedMeasureQuantity,
			MeasureUnderfillBps:     snapshot.MeasureUnderfillBps,
			MeasureOverfillBps:      snapshot.MeasureOverfillBps,
			PriceReference:          snapshot.PriceReference,
			CatalogueRevision:       snapshot.CatalogueRevision,
			InventoryRevision:       snapshot.InventoryRevision,
			InventoryItemRef:        snapshot.InventoryItemRef,
			OnlinePriceDisclosure:   snapshot.OnlinePriceDisclosure,
		})
	}
	return lines, rows.Err()
}

func loadSection17EAdjustments(
	ctx context.Context, q section17EQueryer, quoteID string,
) ([]orders.AdjustmentSource, error) {
	rows, err := q.Query(ctx, `
		SELECT adjustment_id,adjustment_type,scope,funding_source,source_ref,
		       description,amount_minor
		  FROM commerce_quote_adjustments
		 WHERE quote_id=$1 ORDER BY adjustment_id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var adjustments []orders.AdjustmentSource
	for rows.Next() {
		var adjustment orders.AdjustmentSource
		if err = rows.Scan(&adjustment.AdjustmentID, &adjustment.Type, &adjustment.Scope,
			&adjustment.FundingSource, &adjustment.SourceRef, &adjustment.Description,
			&adjustment.AmountMinor); err != nil {
			return nil, err
		}
		adjustments = append(adjustments, adjustment)
	}
	return adjustments, rows.Err()
}

var _ orders.QuoteAuthority = (*section17EQuoteAuthority)(nil)
