package orders

// Section 18 operational evidence.  The purchased line and commercial
// allocations remain append-only Section 17 evidence; this file owns only the
// mutable picking/decision state and the authenticated logistics event inbox.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type LineAvailability string

const (
	AvailabilityPending        LineAvailability = "PENDING"
	AvailabilityFullyAvailable LineAvailability = "FULLY_AVAILABLE"
	AvailabilityPartially      LineAvailability = "PARTIALLY_AVAILABLE"
	AvailabilityUnavailable    LineAvailability = "UNAVAILABLE"
)

type LineOutcome struct {
	ID, ParentOrderID, VendorOrderID, OrderLineID                               string
	OrderedQuantity, PickedQuantity, UnavailableQuantity, ActualMeasureQuantity int64
	Outcome                                                                     LineAvailability
	Version                                                                     int64
	CreatedAt, UpdatedAt                                                        time.Time
}

type SubstitutionStatus string

const (
	SubstitutionPending   SubstitutionStatus = "PENDING"
	SubstitutionApproved  SubstitutionStatus = "APPROVED"
	SubstitutionRejected  SubstitutionStatus = "REJECTED"
	SubstitutionExpired   SubstitutionStatus = "EXPIRED"
	SubstitutionCancelled SubstitutionStatus = "CANCELLED"
)

type SubstitutionProposal struct {
	ID, ParentOrderID, VendorOrderID, OrderLineID, CustomerID  string
	BusinessID, BranchID                                       string
	ProposedItemRef, ProposedVariantRef, ProposedCatalogueRef  string
	ReplacementSnapshot                                        json.RawMessage
	ProposedQuantity, ProposedPriceMinor, PriceDifferenceMinor int64
	Reason                                                     string
	ExpiresAt                                                  *time.Time
	Status                                                     SubstitutionStatus
	ResponseRef                                                string
	Version                                                    int64
	CreatedAt, RespondedAt, UpdatedAt                          time.Time
}

type LogisticsOrderEvent struct {
	EventID, MissionRef, VendorOrderID, ParentOrderID string
	Source                                            LogisticsService
	EventType                                         string
	SourceVersion                                     int64
	OccurredAt                                        time.Time
	Payload                                           json.RawMessage
}

// ValidateVariableWeightAmount is the single arithmetic guard for a picked
// variable-measure line.  It uses integer minor units and never permits a
// result outside the customer's frozen authorization ceiling.
func ValidateVariableWeightAmount(finalTotalMinor, maxAuthorizedTotalMinor int64) error {
	if finalTotalMinor < 0 || maxAuthorizedTotalMinor < 0 || finalTotalMinor > maxAuthorizedTotalMinor {
		return ErrOrderConflict
	}
	return nil
}

// VariableWeightDelta returns the exact signed commercial delta after the
// picked measure is known. A positive value must go through explicit customer
// commercial-change approval; a negative value is eligible for the existing
// cheaper-adjustment/refund policy.
func VariableWeightDelta(quotedTotalMinor, finalTotalMinor, maxAuthorizedTotalMinor int64) (int64, error) {
	if err := ValidateVariableWeightAmount(finalTotalMinor, maxAuthorizedTotalMinor); err != nil {
		return 0, err
	}
	if quotedTotalMinor < 0 {
		return 0, ErrOrderConflict
	}
	return finalTotalMinor - quotedTotalMinor, nil
}

// SetRefundState records the Wallet authority outcome without claiming that a
// request was a successful refund.  It is deliberately idempotent and scoped
// to one VendorOrder/payment leg.
func (s *Store) SetRefundState(ctx context.Context, vendorOrderID, state, operationRef string, refundedMinor int64, now time.Time, expectedVersions ...int64) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	if state != "REFUND_PENDING" && state != "REFUNDED" && state != "PARTIALLY_REFUNDED" && state != "UNKNOWN" && state != "FAILED" {
		return ErrOrderConflict
	}
	if refundedMinor < 0 {
		return ErrOrderConflict
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var parentID, customerID, currentState, currentStatus, currentOperation, previousParentState string
	var currentAmount, currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT parent_order_id,customer_id,refund_state,status,refund_operation_ref,refund_refunded_minor,version FROM commerce_vendor_orders WHERE id=$1 FOR UPDATE`, vendorOrderID).
		Scan(&parentID, &customerID, &currentState, &currentStatus, &currentOperation, &currentAmount, &currentVersion); errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	} else if err != nil {
		return err
	}
	if len(expectedVersions) > 0 && currentVersion != expectedVersions[0] {
		return ErrVersionConflict
	}
	if err = tx.QueryRow(ctx, `SELECT state FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentID).Scan(&previousParentState); err != nil {
		return err
	}
	// A replay of the same Wallet result is a successful no-op. A later
	// authoritative result may advance REFUND_PENDING to a terminal refund.
	if currentState == state && currentOperation == operationRef && currentAmount == refundedMinor {
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	status := currentStatus
	terminal := false
	switch state {
	case "REFUND_PENDING":
		status = string(VendorStatusRefundPending)
	case "REFUNDED":
		status = string(VendorStatusRefunded)
		terminal = true
	case "PARTIALLY_REFUNDED":
		status = string(VendorStatusPartiallyRefunded)
		terminal = true
	}
	if status == string(VendorStatusCompleted) || status == string(VendorStatusCancelled) || status == string(VendorStatusRejected) || status == string(VendorStatusRefunded) || status == string(VendorStatusPartiallyRefunded) {
		terminal = true
	}
	var terminalAt any
	if terminal {
		terminalAt = now
	}
	tag, err := tx.Exec(ctx, `UPDATE commerce_vendor_orders
		SET refund_state=$1,refund_operation_ref=CASE WHEN $2<>'' THEN $2 ELSE refund_operation_ref END,
		    refund_refunded_minor=$3,status=$4,terminal_at=CASE WHEN $5 THEN $6::timestamptz ELSE NULL END,
		    cancelled_by='',updated_at=$7,version=version+1
		WHERE id=$8 AND version=$9`, state, operationRef, refundedMinor, status, terminal, terminalAt, now, vendorOrderID, currentVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrVersionConflict
	}
	if action := refundEventAction(state); action != "" {
		if err = appendEvent(ctx, tx, s.outbox, parentID, vendorOrderID, customerID, action, "AUTHORITY", operationRef, currentStatus, status, now); err != nil {
			return err
		}
	}
	if _, err = refreshParentState(ctx, tx, parentID, customerID, previousParentState, 0, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func refundEventAction(state string) string {
	switch state {
	case "REFUND_PENDING":
		return "commerce.vendor_order.refund_pending"
	case "REFUNDED":
		return "commerce.vendor_order.refunded"
	case "PARTIALLY_REFUNDED":
		return "commerce.vendor_order.partially_refunded"
	case "UNKNOWN":
		return "commerce.vendor_order.refund_unknown"
	case "FAILED":
		return "commerce.vendor_order.refund_failed"
	default:
		return ""
	}
}

func (s *Service) ReportLineAvailability(ctx context.Context, actor Actor, vendorOrderID, lineID string,
	expectedVersion, picked, unavailable, actualMeasure int64, outcome LineAvailability) (LineOutcome, error) {
	if !s.enabled {
		return LineOutcome{}, ErrDisabled
	}
	scope, err := s.authorize(ctx, actor, PermOrdersManage)
	if err != nil {
		return LineOutcome{}, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID)
	if err != nil {
		return LineOutcome{}, err
	}
	if !scope.Covers(order.BranchID) {
		return LineOutcome{}, ErrOrderNotFound
	}
	if order.Domain != "SHOP" {
		return LineOutcome{}, ErrOrderConflict
	}
	return s.store.UpsertLineOutcome(ctx, actor.VendorID, vendorOrderID, lineID, expectedVersion, picked, unavailable, actualMeasure, outcome, s.now().UTC())
}

func (s *Service) ProposeSubstitution(ctx context.Context, actor Actor, vendorOrderID, lineID string,
	proposal SubstitutionProposal, idempotencyKey string) (SubstitutionProposal, error) {
	if !s.enabled {
		return SubstitutionProposal{}, ErrDisabled
	}
	scope, err := s.authorize(ctx, actor, PermOrdersManage)
	if err != nil {
		return SubstitutionProposal{}, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID)
	if err != nil {
		return SubstitutionProposal{}, err
	}
	if !scope.Covers(order.BranchID) {
		return SubstitutionProposal{}, ErrOrderNotFound
	}
	if order.Domain != "SHOP" || (order.Status != VendorStatusPicking && order.Status != VendorStatusSubstitutionReq && order.Status != VendorStatusAwaitingCustomer) {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	if proposal.Version > 0 && proposal.Version != order.Version {
		return SubstitutionProposal{}, ErrVersionConflict
	}
	// The request may carry only a candidate reference. Identity, price,
	// quantity and the evidence snapshot are resolved from the live Shop
	// catalogue inside the Vendor Service transaction.
	resolved, err := s.store.ResolveSubstitutionCandidate(ctx, actor.VendorID, vendorOrderID, lineID, proposal)
	if err != nil {
		return SubstitutionProposal{}, err
	}
	proposal.ProposedItemRef = resolved.ProposedItemRef
	proposal.ProposedVariantRef = resolved.ProposedVariantRef
	proposal.ProposedCatalogueRef = resolved.ProposedCatalogueRef
	proposal.ReplacementSnapshot = resolved.ReplacementSnapshot
	proposal.ProposedQuantity = resolved.ProposedQuantity
	proposal.ProposedPriceMinor = resolved.ProposedPriceMinor
	proposal.CustomerID = order.CustomerID
	return s.store.CreateSubstitutionProposal(ctx, actor.VendorID, vendorOrderID, lineID, order.CustomerID, proposal, idempotencyKey, s.now().UTC())
}

func (s *Service) RespondToSubstitution(ctx context.Context, customerID, proposalID string, approve bool, expectedVersion int64) (SubstitutionProposal, error) {
	if !s.enabled {
		return SubstitutionProposal{}, ErrDisabled
	}
	if approve {
		proposal, err := s.store.GetSubstitutionForCustomer(ctx, strings.TrimSpace(customerID), strings.TrimSpace(proposalID))
		if err != nil {
			return SubstitutionProposal{}, err
		}
		if proposal.PriceDifferenceMinor > 0 {
			if err = s.ensureSubstitutionCommercialChange(ctx, proposal); err != nil && !errors.Is(err, ErrCommercialChangeRequired) {
				return SubstitutionProposal{}, err
			}
		}
	}
	return s.store.RespondSubstitution(ctx, strings.TrimSpace(customerID), strings.TrimSpace(proposalID), approve, expectedVersion, s.now().UTC())
}

func (s *Service) ensureSubstitutionCommercialChange(ctx context.Context, proposal SubstitutionProposal) error {
	authority, ok := s.changeQuotes.(SubstitutionCommercialChangeQuoteAuthority)
	if !ok || authority == nil {
		return ErrCommercialChangeRequired
	}
	req, err := s.store.SubstitutionCommercialContext(ctx, proposal)
	if err != nil {
		return err
	}
	quote, err := authority.CreateSubstitutionReplacementQuote(ctx, SubstitutionCommercialChangeRequest{
		CustomerID: req.CustomerID, CheckoutSessionID: req.CheckoutSessionID, FulfilmentPlanID: req.FulfilmentPlanID,
		CartItemID: req.CartItemID, ItemRef: req.ItemRef, VariantRef: req.VariantRef, ListingRef: req.ListingRef,
		SellUnitRef: req.SellUnitRef, UnitPriceMinor: req.UnitPriceMinor,
	})
	if err != nil {
		return err
	}
	if quote.CustomerID != proposal.CustomerID || quote.ID == "" || quote.Fingerprint == "" || quote.TotalMinor <= 0 {
		return ErrCommercialChangeRequired
	}
	parent, err := s.store.GetParent(ctx, proposal.CustomerID, proposal.ParentOrderID)
	if err != nil {
		return err
	}
	change, err := s.store.CreateCommercialChange(ctx, parent, ChangeRequest{CheckoutSessionID: req.CheckoutSessionID, FulfilmentPlanID: req.FulfilmentPlanID}, quote, s.now().UTC())
	if errors.Is(err, ErrCommercialChangeConflict) {
		return ErrCommercialChangeRequired
	}
	if err != nil {
		return err
	}
	if err == nil && change.DeltaMinor > 0 {
		adjustment, e := s.store.EnsureChangeAdjustment(ctx, change, "POSITIVE_DELTA", "AUTHORIZE", "customer-approved substitution replacement quote", "substitution-change-"+proposal.ID, s.now().UTC())
		if e == nil {
			_, e = s.store.db.Exec(ctx, `UPDATE commerce_order_adjustments SET vendor_order_id=$1 WHERE id=$2 AND vendor_order_id IS NULL`, proposal.VendorOrderID, adjustment.ID)
		}
		err = e
	}
	if err != nil {
		return err
	}
	return ErrCommercialChangeRequired
}

func (s *Service) ApplyLogisticsEvent(ctx context.Context, event LogisticsOrderEvent) (VendorOrder, ParentOrder, error) {
	if !s.enabled {
		return VendorOrder{}, ParentOrder{}, ErrDisabled
	}
	return s.store.ApplyLogisticsEvent(ctx, event, s.now().UTC())
}

func (s *Store) UpsertLineOutcome(ctx context.Context, businessID, vendorOrderID, lineID string,
	expectedVersion, picked, unavailable, actualMeasure int64, outcome LineAvailability, now time.Time) (LineOutcome, error) {
	if s == nil || s.db == nil {
		return LineOutcome{}, ErrDependencyMissing
	}
	if picked < 0 || unavailable < 0 || actualMeasure < 0 {
		return LineOutcome{}, ErrOrderConflict
	}
	if outcome != AvailabilityFullyAvailable && outcome != AvailabilityPartially && outcome != AvailabilityUnavailable && outcome != AvailabilityPending {
		return LineOutcome{}, ErrOrderConflict
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return LineOutcome{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var parentID string
	if err = tx.QueryRow(ctx, `SELECT parent_order_id FROM commerce_vendor_orders WHERE id=$1 AND business_id=$2`, vendorOrderID, businessID).Scan(&parentID); errors.Is(err, pgx.ErrNoRows) {
		return LineOutcome{}, ErrOrderNotFound
	} else if err != nil {
		return LineOutcome{}, err
	}
	var ordered int64
	if err = tx.QueryRow(ctx, `SELECT quantity FROM commerce_vendor_order_lines WHERE id=$1 AND vendor_order_id=$2`, lineID, vendorOrderID).Scan(&ordered); errors.Is(err, pgx.ErrNoRows) {
		return LineOutcome{}, ErrOrderNotFound
	} else if err != nil {
		return LineOutcome{}, err
	}
	if picked+unavailable > ordered {
		return LineOutcome{}, ErrOrderConflict
	}
	var current LineOutcome
	err = tx.QueryRow(ctx, `SELECT id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,version,created_at,updated_at FROM commerce_vendor_order_line_outcomes WHERE order_line_id=$1 FOR UPDATE`, lineID).Scan(&current.ID, &current.OrderedQuantity, &current.PickedQuantity, &current.UnavailableQuantity, &current.ActualMeasureQuantity, &current.Outcome, &current.Version, &current.CreatedAt, &current.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current.ID = "cvoo_" + uuid.NewString()
		current.Version = 1
		_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_order_line_outcomes(id,parent_order_id,vendor_order_id,order_line_id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10,$10)`, current.ID, parentID, vendorOrderID, lineID, ordered, picked, unavailable, actualMeasure, outcome, now)
		if err != nil {
			return LineOutcome{}, err
		}
	} else if err != nil {
		return LineOutcome{}, err
	} else {
		if current.Version != expectedVersion {
			return LineOutcome{}, ErrVersionConflict
		}
		tag, e := tx.Exec(ctx, `UPDATE commerce_vendor_order_line_outcomes SET picked_quantity=$1,unavailable_quantity=$2,actual_measure_quantity=$3,outcome=$4,version=version+1,updated_at=$5 WHERE order_line_id=$6 AND version=$7`, picked, unavailable, actualMeasure, outcome, now, lineID, current.Version)
		if e != nil {
			return LineOutcome{}, e
		}
		if tag.RowsAffected() != 1 {
			return LineOutcome{}, ErrVersionConflict
		}
	}
	if err = tx.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,version,created_at,updated_at FROM commerce_vendor_order_line_outcomes WHERE order_line_id=$1`, lineID).Scan(&current.ID, &current.ParentOrderID, &current.VendorOrderID, &current.OrderLineID, &current.OrderedQuantity, &current.PickedQuantity, &current.UnavailableQuantity, &current.ActualMeasureQuantity, &current.Outcome, &current.Version, &current.CreatedAt, &current.UpdatedAt); err != nil {
		return LineOutcome{}, err
	}
	var lifecycleStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID).Scan(&lifecycleStatus); err != nil {
		return LineOutcome{}, err
	}
	if err = appendEvent(ctx, tx, s.outbox, parentID, vendorOrderID, businessID, ActionPicking, "VENDOR", businessID, lifecycleStatus, lifecycleStatus, now); err != nil {
		return LineOutcome{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LineOutcome{}, err
	}
	return current, nil
}

// ResolveSubstitutionCandidate is the Shop catalogue boundary for lifecycle
// substitutions. The caller supplies only a listing reference (and optional
// variant/sell-unit hints); all commercial and descriptive fields come from
// the authoritative live listing.
func (s *Store) ResolveSubstitutionCandidate(ctx context.Context, businessID, vendorOrderID, lineID string, requested SubstitutionProposal) (SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return SubstitutionProposal{}, ErrDependencyMissing
	}
	if strings.TrimSpace(requested.ProposedCatalogueRef) == "" {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	var branchID, domain string
	var originalQuantity int64
	if err := s.db.QueryRow(ctx, `SELECT v.branch_id,v.domain,l.quantity FROM commerce_vendor_orders v JOIN commerce_vendor_order_lines l ON l.vendor_order_id=v.id WHERE v.id=$1 AND v.business_id=$2 AND l.id=$3`, vendorOrderID, businessID, lineID).Scan(&branchID, &domain, &originalQuantity); errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionProposal{}, ErrOrderNotFound
	} else if err != nil {
		return SubstitutionProposal{}, err
	}
	if domain != "SHOP" {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	var listingID, variantID, productID, productName, variantName, productBrand string
	var listingStatus, productStatus, variantStatus string
	var listingSellUnit, sellUnitName string
	var packQuantity int64
	var retailPrice, listingPrice, sellUnitPrice, sellUnitOverride *int64
	query := `SELECT l.id,l.variant_id,l.product_id,p.name,v.name,COALESCE(p.brand,''),l.status,p.status,v.status,
		COALESCE(lu.sell_unit_id,''),COALESCE(su.display_name,''),v.pack_quantity,
		v.retail_price_minor,l.retail_price_override_minor,su.price_minor,lu.price_override_minor
		FROM shop_branch_listings l
		JOIN shop_products p ON p.id=l.product_id AND p.business_id=l.business_id
		JOIN shop_variants v ON v.id=l.variant_id AND v.business_id=l.business_id
		LEFT JOIN shop_listing_sell_units lu ON lu.listing_id=l.id AND lu.business_id=l.business_id AND lu.enabled=true
		LEFT JOIN shop_sell_units su ON su.id=lu.sell_unit_id AND su.business_id=l.business_id
		WHERE l.id=$1 AND l.business_id=$2 AND l.branch_id=$3 ORDER BY lu.position,lu.id LIMIT 1`
	if err := s.db.QueryRow(ctx, query, requested.ProposedCatalogueRef, businessID, branchID).Scan(&listingID, &variantID, &productID, &productName, &variantName, &productBrand, &listingStatus, &productStatus, &variantStatus, &listingSellUnit, &sellUnitName, &packQuantity, &retailPrice, &listingPrice, &sellUnitPrice, &sellUnitOverride); err != nil {
		// A missing optional sell-unit join yields no row only when the listing
		// itself is absent; an explicitly requested incompatible variant is
		// rejected below after the base listing lookup.
		if errors.Is(err, pgx.ErrNoRows) {
			return SubstitutionProposal{}, ErrOrderNotFound
		}
		return SubstitutionProposal{}, err
	}
	if listingID == "" || listingStatus != "ACTIVE" || productStatus != "ACTIVE" || variantStatus != "ACTIVE" {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	if requested.ProposedVariantRef != "" && requested.ProposedVariantRef != variantID {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	// Price precedence is branch sell-unit override, branch listing override,
	// sell-unit base price, then variant retail price. A live active listing
	// without a price is not a valid substitution candidate.
	var authoritativePrice int64
	switch {
	case sellUnitOverride != nil:
		authoritativePrice = *sellUnitOverride
	case listingPrice != nil:
		authoritativePrice = *listingPrice
	case sellUnitPrice != nil:
		authoritativePrice = *sellUnitPrice
	case retailPrice != nil:
		authoritativePrice = *retailPrice
	default:
		return SubstitutionProposal{}, ErrOrderConflict
	}
	snapshot, err := json.Marshal(map[string]any{
		"catalogue_ref": listingID, "product_ref": productID, "variant_ref": variantID,
		"product_name": productName, "variant_name": variantName, "brand": productBrand,
		"sell_unit_ref": listingSellUnit, "sell_unit_name": sellUnitName,
		"pack_quantity": packQuantity, "price_minor": authoritativePrice,
		"business_id": businessID, "branch_id": branchID,
	})
	if err != nil {
		return SubstitutionProposal{}, err
	}
	return SubstitutionProposal{ProposedItemRef: productID, ProposedVariantRef: variantID, ProposedCatalogueRef: listingID, ReplacementSnapshot: snapshot, ProposedQuantity: originalQuantity, ProposedPriceMinor: authoritativePrice}, nil
}

func (s *Store) CreateSubstitutionProposal(ctx context.Context, businessID, vendorOrderID, lineID, customerID string,
	proposal SubstitutionProposal, idempotencyKey string, now time.Time) (SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return SubstitutionProposal{}, ErrDependencyMissing
	}
	if strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(proposal.Reason) == "" || proposal.ProposedQuantity <= 0 || proposal.ProposedPriceMinor < 0 {
		return SubstitutionProposal{}, ErrOrderConflict
	}
	var p SubstitutionProposal
	err := s.db.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at FROM commerce_substitution_proposals WHERE idempotency_key=$1`, idempotencyKey).Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, &p.RespondedAt, &p.UpdatedAt)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionProposal{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SubstitutionProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var parentID, ownerCustomer, branch, currentStatus string
	var originalPrice int64
	if err = tx.QueryRow(ctx, `SELECT v.parent_order_id,v.customer_id,v.branch_id,v.status,l.unit_price_minor FROM commerce_vendor_orders v JOIN commerce_vendor_order_lines l ON l.vendor_order_id=v.id WHERE v.id=$1 AND v.business_id=$2 AND l.id=$3 FOR UPDATE`, vendorOrderID, businessID, lineID).Scan(&parentID, &ownerCustomer, &branch, &currentStatus, &originalPrice); errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionProposal{}, ErrOrderNotFound
	} else if err != nil {
		return SubstitutionProposal{}, err
	}
	if ownerCustomer != customerID {
		return SubstitutionProposal{}, ErrForbidden
	}
	if proposal.ReplacementSnapshot == nil {
		proposal.ReplacementSnapshot = json.RawMessage(`{}`)
	}
	proposal.ID = "csp_" + uuid.NewString()
	proposal.ParentOrderID = parentID
	proposal.VendorOrderID = vendorOrderID
	proposal.OrderLineID = lineID
	proposal.CustomerID = customerID
	proposal.BusinessID = businessID
	proposal.BranchID = branch
	proposal.PriceDifferenceMinor = proposal.ProposedPriceMinor - originalPrice
	proposal.Status = SubstitutionPending
	proposal.Version = 1
	proposal.CreatedAt = now
	proposal.UpdatedAt = now
	_, err = tx.Exec(ctx, `INSERT INTO commerce_substitution_proposals(id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,version,idempotency_key,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'PENDING',1,$17,$18,$18)`, proposal.ID, parentID, vendorOrderID, lineID, customerID, businessID, branch, proposal.ProposedItemRef, proposal.ProposedVariantRef, proposal.ProposedCatalogueRef, proposal.ReplacementSnapshot, proposal.ProposedQuantity, proposal.ProposedPriceMinor, proposal.PriceDifferenceMinor, proposal.Reason, proposal.ExpiresAt, idempotencyKey, now)
	if err != nil {
		return SubstitutionProposal{}, err
	}
	if tag, e := tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='SUBSTITUTION_REQUIRED',version=version+1,updated_at=$1 WHERE id=$2 AND status IN ('PICKING','ACCEPTED')`, now, vendorOrderID); e != nil {
		return SubstitutionProposal{}, e
	} else if tag.RowsAffected() == 1 {
		if e = appendEvent(ctx, tx, s.outbox, parentID, vendorOrderID, customerID, "commerce.vendor_order.substitution_required", "VENDOR", businessID, currentStatus, "SUBSTITUTION_REQUIRED", now); e != nil {
			return SubstitutionProposal{}, e
		}
	}
	if err = appendEvent(ctx, tx, s.outbox, parentID, vendorOrderID, customerID, "commerce.vendor_order.substitution_proposed", "VENDOR", businessID, currentStatus, "SUBSTITUTION_REQUIRED", now); err != nil {
		return SubstitutionProposal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SubstitutionProposal{}, err
	}
	return proposal, nil
}

func (s *Store) RespondSubstitution(ctx context.Context, customerID, proposalID string, approve bool, expectedVersion int64, now time.Time) (SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return SubstitutionProposal{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SubstitutionProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p SubstitutionProposal
	err = tx.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at FROM commerce_substitution_proposals WHERE id=$1 AND customer_id=$2 FOR UPDATE`, proposalID, customerID).Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, &p.RespondedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionProposal{}, ErrOrderNotFound
	}
	if err != nil {
		return SubstitutionProposal{}, err
	}
	if p.Status != SubstitutionPending {
		if (approve && p.Status == SubstitutionApproved) || (!approve && p.Status == SubstitutionRejected) {
			return p, nil
		}
		return SubstitutionProposal{}, ErrOrderConflict
	}
	if p.ExpiresAt != nil && now.After(*p.ExpiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE commerce_substitution_proposals SET status='EXPIRED',version=version+1,updated_at=$1,responded_at=$1 WHERE id=$2`, now, proposalID)
		return SubstitutionProposal{}, ErrCommercialChangeExpired
	}
	if p.Version != expectedVersion {
		return SubstitutionProposal{}, ErrVersionConflict
	}
	if approve && p.PriceDifferenceMinor > 0 {
		// Preserve a durable Section 17 adjustment request instead of dropping
		// the customer decision at the HTTP boundary. The proposal remains
		// PENDING until the replacement quote/Wallet approval path completes;
		// no stock or order price is changed here.
		_, err = tx.Exec(ctx, `INSERT INTO commerce_order_adjustments(id,parent_order_id,vendor_order_id,adjustment_type,original_commitment_minor,eligible_final_minor,difference_minor,policy,action,idempotency_key,wallet_status,version,created_at,updated_at) VALUES($1,$2,$3,'SUBSTITUTION',$4,$5,$6,'SUBSTITUTION_REPLACEMENT_QUOTE_REQUIRED','AUTHORIZE',$7,'PENDING',1,$8,$8) ON CONFLICT(idempotency_key) DO NOTHING`, "coa_"+uuid.NewString(), p.ParentOrderID, p.VendorOrderID, p.ProposedPriceMinor-p.PriceDifferenceMinor, p.ProposedPriceMinor, p.PriceDifferenceMinor, "substitution-positive-"+p.ID, now)
		if err != nil {
			return SubstitutionProposal{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return SubstitutionProposal{}, err
		}
		return SubstitutionProposal{}, ErrCommercialChangeRequired
	}
	state := SubstitutionRejected
	if approve {
		state = SubstitutionApproved
	}
	tag, err := tx.Exec(ctx, `UPDATE commerce_substitution_proposals SET status=$1,version=version+1,responded_at=$2,updated_at=$2,response_ref=$3 WHERE id=$4 AND version=$5`, state, now, customerID, proposalID, p.Version)
	if err != nil {
		return SubstitutionProposal{}, err
	}
	if tag.RowsAffected() != 1 {
		return SubstitutionProposal{}, ErrVersionConflict
	}
	if approve && p.PriceDifferenceMinor < 0 {
		_, err = tx.Exec(ctx, `INSERT INTO commerce_order_adjustments(id,parent_order_id,vendor_order_id,adjustment_type,original_commitment_minor,eligible_final_minor,difference_minor,policy,action,idempotency_key,wallet_status,version,created_at,updated_at) VALUES($1,$2,$3,'SUBSTITUTION',$4,$5,$6,'CUSTOMER_APPROVED_CHEAPER_SUBSTITUTION','REFUND',$7,'PENDING',1,$8,$8) ON CONFLICT (idempotency_key) DO NOTHING`, "coa_"+uuid.NewString(), p.ParentOrderID, p.VendorOrderID, p.ProposedPriceMinor-p.PriceDifferenceMinor, p.ProposedPriceMinor, -p.PriceDifferenceMinor, "substitution-"+p.ID, now)
		if err != nil {
			return SubstitutionProposal{}, err
		}
	}
	// A customer decision is lifecycle evidence and releases the vendor order
	// from SUBSTITUTION_REQUIRED back to picking. The update is in the same
	// transaction as the proposal response and therefore cannot be observed as
	// an approval without the corresponding order transition.
	if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='PICKING',version=version+1,updated_at=$1 WHERE id=$2 AND status IN ('SUBSTITUTION_REQUIRED','AWAITING_CUSTOMER')`, now, p.VendorOrderID); err != nil {
		return SubstitutionProposal{}, err
	}
	action := "commerce.vendor_order.substitution_rejected"
	if approve {
		action = "commerce.vendor_order.substitution_approved"
	}
	if err = appendEvent(ctx, tx, s.outbox, p.ParentOrderID, p.VendorOrderID, p.CustomerID, action, "CUSTOMER", customerID, "SUBSTITUTION_REQUIRED", "PICKING", now); err != nil {
		return SubstitutionProposal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SubstitutionProposal{}, err
	}
	p.Status = state
	p.Version++
	p.RespondedAt = now
	p.UpdatedAt = now
	return p, nil
}

func (s *Store) GetSubstitutionForCustomer(ctx context.Context, customerID, proposalID string) (SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return SubstitutionProposal{}, ErrDependencyMissing
	}
	return scanSubstitution(s.db.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at FROM commerce_substitution_proposals WHERE id=$1 AND customer_id=$2`, proposalID, customerID))
}

func (s *Store) SubstitutionCommercialContext(ctx context.Context, proposal SubstitutionProposal) (SubstitutionCommercialChangeRequest, error) {
	if s == nil || s.db == nil {
		return SubstitutionCommercialChangeRequest{}, ErrDependencyMissing
	}
	var request SubstitutionCommercialChangeRequest
	err := s.db.QueryRow(ctx, `SELECT p.checkout_session_id,p.fulfilment_plan_id,l.cart_item_id FROM commerce_parent_orders p JOIN commerce_vendor_order_lines l ON l.parent_order_id=p.id AND l.vendor_order_id=$2 WHERE p.id=$1 AND p.customer_id=$3 AND l.id=$4`, proposal.ParentOrderID, proposal.VendorOrderID, proposal.CustomerID, proposal.OrderLineID).Scan(&request.CheckoutSessionID, &request.FulfilmentPlanID, &request.CartItemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionCommercialChangeRequest{}, ErrOrderNotFound
	}
	if err != nil {
		return SubstitutionCommercialChangeRequest{}, err
	}
	request.CustomerID = proposal.CustomerID
	request.ItemRef, request.VariantRef, request.ListingRef = proposal.ProposedItemRef, proposal.ProposedVariantRef, proposal.ProposedCatalogueRef
	request.UnitPriceMinor = proposal.ProposedPriceMinor
	return request, nil
}

func scanSubstitution(row scanner) (SubstitutionProposal, error) {
	var p SubstitutionProposal
	err := row.Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, &p.RespondedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionProposal{}, ErrOrderNotFound
	}
	return p, err
}

func (s *Store) ListPendingSubstitutions(ctx context.Context, customerID string, limit int) ([]SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at FROM commerce_substitution_proposals WHERE customer_id=$1 AND status='PENDING' ORDER BY created_at,id LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SubstitutionProposal
	for rows.Next() {
		p, e := scanSubstitution(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ApplyLogisticsEvent is the only path by which platform execution may move a
// VendorOrder into pickup/delivery states. Source versions and event ids make
// retries and delayed events monotonic and harmless.
func (s *Store) ApplyLogisticsEvent(ctx context.Context, event LogisticsOrderEvent, now time.Time) (VendorOrder, ParentOrder, error) {
	if s == nil || s.db == nil {
		return VendorOrder{}, ParentOrder{}, ErrDependencyMissing
	}
	if event.Source != LogisticsDispatch && event.Source != LogisticsHauling {
		return VendorOrder{}, ParentOrder{}, ErrForbidden
	}
	if strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.MissionRef) == "" ||
		strings.TrimSpace(event.VendorOrderID) == "" || strings.TrimSpace(event.ParentOrderID) == "" ||
		event.SourceVersion < 1 || event.OccurredAt.IsZero() {
		return VendorOrder{}, ParentOrder{}, ErrOrderConflict
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existing string
	var existingSource, existingMission, existingVendor, existingParent, existingType string
	if err = tx.QueryRow(ctx, `SELECT id,source,mission_ref,vendor_order_id,parent_order_id,event_type FROM commerce_logistics_order_events WHERE event_id=$1`, event.EventID).Scan(&existing, &existingSource, &existingMission, &existingVendor, &existingParent, &existingType); err == nil {
		if existingSource != string(event.Source) || existingMission != event.MissionRef || existingVendor != event.VendorOrderID || existingParent != event.ParentOrderID || existingType != event.EventType {
			return VendorOrder{}, ParentOrder{}, ErrOrderConflict
		}
		var bid, cid string
		_ = tx.QueryRow(ctx, `SELECT business_id,customer_id FROM commerce_vendor_orders WHERE id=$1`, event.VendorOrderID).Scan(&bid, &cid)
		parent, e := loadParent(ctx, tx, cid, event.ParentOrderID)
		if e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		vo, e := loadVendorOrderBy(ctx, tx, `v.id=$1`, event.VendorOrderID)
		return vo, parent, e
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return VendorOrder{}, ParentOrder{}, err
	}
	var current VendorOrder
	if err = tx.QueryRow(ctx, `SELECT `+vendorColumns+` FROM commerce_vendor_orders v WHERE v.id=$1 AND v.parent_order_id=$2 FOR UPDATE`, event.VendorOrderID, event.ParentOrderID).Scan(&current.ID, &current.ParentOrderID, &current.CustomerID, &current.BusinessID, &current.BranchID, &current.Domain, &current.Currency, &current.QuoteGroupRef, &current.ProductSubtotalMinor, &current.MaxProductSubtotalMinor, &current.ProductVendorFundedMinor, &current.ProductPlatformFundedMinor, &current.ProductOtherFundedMinor, &current.OrderVendorFundedMinor, &current.OrderPlatformFundedMinor, &current.OrderOtherFundedMinor, &current.DeliveryResponsibility, &current.DeliveryFeeBeneficiary, &current.DeliveryGrossMinor, &current.DeliveryCustomerMinor, &current.DeliveryVendorFundedMinor, &current.DeliveryPlatformFundedMinor, &current.DeliveryOtherFundedMinor, &current.CustomerPayableMinor, &current.VendorPayableMinor, &current.ReservationRequired, &current.InventoryState, &current.ReservationRef, &current.ReservationSourceRef, &current.FulfilmentMode, &current.LogisticsPlanGroupRef, &current.LogisticsMissionRef, &current.VendorOwnDeliveryState, &current.Status, &current.CancelReasonCode, &current.CancelledBy, &current.RefundAllocatedMinor, &current.SettlementAllocatedMinor, &current.RefundState, &current.RefundOperationRef, &current.RefundRefundedMinor, &current.RejectionReason, &current.Version, &current.CreatedAt, &current.UpdatedAt, &current.TerminalAt); errors.Is(err, pgx.ErrNoRows) {
		return VendorOrder{}, ParentOrder{}, ErrOrderNotFound
	}
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if current.DeliveryResponsibility != DeliveryPlatform || current.LogisticsMissionRef != event.MissionRef {
		return VendorOrder{}, ParentOrder{}, ErrForbidden
	}
	rank := func(v VendorStatus) int {
		switch v {
		case VendorStatusReadyForPickup, VendorStatusReady:
			return 1
		case VendorStatusPickedUp, VendorStatusHandedOver:
			return 2
		case VendorStatusDelivering:
			return 3
		case VendorStatusDelivered, VendorStatusCompleted:
			return 4
		}
		return 0
	}
	target := VendorStatus(event.EventType)
	if event.EventType == "PICKED_UP" {
		target = VendorStatusPickedUp
	}
	if event.EventType == "DELIVERING" {
		target = VendorStatusDelivering
	}
	if event.EventType == "DELIVERED" {
		target = VendorStatusDelivered
	}
	ignored := rank(current.Status) >= rank(target)
	if !ignored {
		valid := (event.EventType == "PICKED_UP" && (current.Status == VendorStatusReadyForPickup || current.Status == VendorStatusReady)) ||
			(event.EventType == "DELIVERING" && (current.Status == VendorStatusPickedUp || current.Status == VendorStatusHandedOver)) ||
			(event.EventType == "DELIVERED" && (current.Status == VendorStatusDelivering || current.Status == VendorStatusPickedUp || current.Status == VendorStatusHandedOver))
		if !valid {
			return VendorOrder{}, ParentOrder{}, ErrOrderConflict
		}
	}
	eventTag, err := tx.Exec(ctx, `INSERT INTO commerce_logistics_order_events(id,event_id,source,mission_ref,vendor_order_id,parent_order_id,event_type,source_version,occurred_at,received_at,payload,applied,ignored_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`, "cloe_"+uuid.NewString(), event.EventID, event.Source, event.MissionRef, event.VendorOrderID, event.ParentOrderID, event.EventType, event.SourceVersion, event.OccurredAt, now, event.Payload, !ignored, func() string {
		if ignored {
			return "STALE_OR_DUPLICATE"
		}
		return ""
	}())
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if eventTag.RowsAffected() == 0 {
		parent, e := loadParent(ctx, tx, current.CustomerID, current.ParentOrderID)
		if e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		updated, e := loadVendorOrderBy(ctx, tx, `v.id=$1`, current.ID)
		if e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		if err = tx.Commit(ctx); err != nil {
			return VendorOrder{}, ParentOrder{}, err
		}
		return updated, parent, nil
	}
	if !ignored {
		tag, e := tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status=$1,version=version+1,updated_at=$2 WHERE id=$3 AND version=$4`, target, now, current.ID, current.Version)
		if e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		if tag.RowsAffected() != 1 {
			return VendorOrder{}, ParentOrder{}, ErrVersionConflict
		}
		action := "commerce.vendor_order.logistics_" + strings.ToLower(event.EventType)
		if e = appendEvent(ctx, tx, s.outbox, current.ParentOrderID, current.ID, current.CustomerID, action, "PLATFORM", string(event.Source), string(current.Status), string(target), now); e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
	}
	var previousState string
	var platformFeeTax int64
	if e := tx.QueryRow(ctx, `SELECT state,platform_fee_tax_minor FROM commerce_parent_orders WHERE id=$1`, current.ParentOrderID).Scan(&previousState, &platformFeeTax); e != nil {
		return VendorOrder{}, ParentOrder{}, e
	}
	var parent ParentOrder
	var e error
	if ignored {
		parent, e = loadParent(ctx, tx, current.CustomerID, current.ParentOrderID)
	} else {
		parent, e = refreshParentState(ctx, tx, current.ParentOrderID, current.CustomerID, previousState, platformFeeTax, now)
	}
	if e != nil {
		return VendorOrder{}, ParentOrder{}, e
	}
	updated, e := loadVendorOrderBy(ctx, tx, `v.id=$1`, current.ID)
	if e != nil {
		return VendorOrder{}, ParentOrder{}, e
	}
	if err = tx.Commit(ctx); err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	return updated, parent, nil
}
