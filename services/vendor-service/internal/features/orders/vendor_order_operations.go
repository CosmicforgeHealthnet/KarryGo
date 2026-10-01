package orders

// Section 18 operational evidence.  The purchased line and commercial
// allocations remain append-only Section 17 evidence; this file owns only the
// mutable picking/decision state and the authenticated logistics event inbox.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
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
	CaseID                                                     string
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

	// CommerceStatus is the normalized fulfilment state this event means
	// (VND-020). It is derived from Source and EventType when a caller leaves
	// it unset, so an existing caller keeps working and a boundary that has
	// already normalized does not have to do it twice.
	//
	// EventType keeps the authority's own richer value: the normalization
	// decides what happens, it does not replace what was reported.
	CommerceStatus logistics.ExecutionStatus

	SourceVersion int64
	OccurredAt    time.Time
	Payload       json.RawMessage
	ParcelRefs    []string
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
	var currentAmount, currentVersion, platformFeeTaxMinor int64
	if err = tx.QueryRow(ctx, `SELECT parent_order_id,customer_id,refund_state,status,refund_operation_ref,refund_refunded_minor,version FROM commerce_vendor_orders WHERE id=$1 FOR UPDATE`, vendorOrderID).
		Scan(&parentID, &customerID, &currentState, &currentStatus, &currentOperation, &currentAmount, &currentVersion); errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	} else if err != nil {
		return err
	}
	if len(expectedVersions) > 0 && currentVersion != expectedVersions[0] {
		return ErrVersionConflict
	}
	if err = tx.QueryRow(ctx, `SELECT state,platform_fee_tax_minor FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentID).Scan(&previousParentState, &platformFeeTaxMinor); err != nil {
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
	// Commercial lifecycle and financial outcome are independent. A cancelled
	// order may truthfully remain CANCELLED while its refund is PENDING; a
	// Wallet command must never rewrite the operational history.
	status := currentStatus
	terminal := VendorStatus(status).Terminal()
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
	if _, err = refreshParentState(ctx, tx, parentID, customerID, previousParentState, platformFeeTaxMinor, now, s.movements()); err != nil {
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
	result, err := s.store.UpsertLineOutcome(ctx, actor.VendorID, vendorOrderID, lineID, expectedVersion, picked, unavailable, actualMeasure, outcome, s.now().UTC())
	if err != nil {
		return result, err
	}
	if outcome == AvailabilityUnavailable {
		policy, policyErr := s.store.FrozenSubstitutionPolicy(ctx, vendorOrderID, lineID)
		if policyErr != nil {
			return result, policyErr
		}
		if policy == PolicyAutoRefund {
			decision, decisionErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerItemUnavailable, Order: order, OrderLineID: lineID, SubstitutionPolicy: policy})
			if decisionErr != nil {
				return result, decisionErr
			}
			if _, decisionErr = s.ensureAndExecuteRefundDecision(ctx, order, "", "commerce-item-unavailable-"+lineID, decision, false); decisionErr != nil {
				return result, decisionErr
			}
		}
	}
	return result, nil
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
	// The frozen line policy is authoritative. In particular, EXACT_ONLY and
	// AUTO_REFUND are terminal no-replacement choices; a vendor request must
	// never turn either into an arbitrary replacement proposal.
	var frozenPolicy string
	if err = s.store.db.QueryRow(ctx, `
		SELECT COALESCE(p.substitution_policy,o.substitution_policy,'EXACT_ONLY')
		  FROM commerce_vendor_order_lines l
		  LEFT JOIN commerce_shop_picking_snapshots p ON p.order_line_id=l.id
		  LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id
		 WHERE l.id=$1 AND l.vendor_order_id=$2`, lineID, vendorOrderID).Scan(&frozenPolicy); err != nil {
		return SubstitutionProposal{}, err
	}
	policy, parseErr := ParseSubstitutionPolicy(frozenPolicy)
	if parseErr != nil || policy == PolicyExactOnly || policy == PolicyAutoRefund {
		return SubstitutionProposal{}, ErrOrderConflict
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
	var appliedChangeID string
	if approve {
		proposal, err := s.store.GetSubstitutionForCustomer(ctx, strings.TrimSpace(customerID), strings.TrimSpace(proposalID))
		if err != nil {
			return SubstitutionProposal{}, err
		}
		if proposal.Status == SubstitutionPending && proposal.Version != expectedVersion {
			return SubstitutionProposal{}, ErrVersionConflict
		}
		if proposal.Status != SubstitutionPending && proposal.Status != SubstitutionApproved {
			return SubstitutionProposal{}, ErrOrderConflict
		}
		if proposal.Status == SubstitutionPending && proposal.ExpiresAt != nil && !proposal.ExpiresAt.After(s.now().UTC()) {
			return SubstitutionProposal{}, ErrCommercialChangeExpired
		}
		if proposal.Status == SubstitutionPending && proposal.PriceDifferenceMinor > 0 {
			if appliedChangeID, err = s.ensureSubstitutionCommercialChange(ctx, proposal); err != nil {
				// A more-expensive replacement is not a direct customer
				// decision. The durable commercial-change path must obtain the
				// extra authorization before the proposal can become effective.
				return SubstitutionProposal{}, err
			}
		}
	}
	resolved, err := s.store.RespondSubstitution(ctx, strings.TrimSpace(customerID), strings.TrimSpace(proposalID), approve, expectedVersion, appliedChangeID, s.now().UTC())
	if err != nil {
		return SubstitutionProposal{}, err
	}
	if approve {
		if err = s.applySubstitutionInventory(ctx, resolved); err != nil {
			return resolved, err
		}
		if resolved.PriceDifferenceMinor < 0 {
			order, loadErr := s.store.GetVendorOrderForCustomer(ctx, resolved.CustomerID, resolved.VendorOrderID)
			if loadErr != nil {
				return resolved, loadErr
			}
			decision, decisionErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerReplacementCheaper, Order: order, OrderLineID: resolved.OrderLineID, DifferenceMinor: -resolved.PriceDifferenceMinor})
			if decisionErr != nil {
				return resolved, decisionErr
			}
			if _, decisionErr = s.ensureAndExecuteRefundDecision(ctx, order, "", "commerce-replacement-refund-"+resolved.ID, decision, false); decisionErr != nil {
				return resolved, decisionErr
			}
		}
	}
	return resolved, nil
}

func (s *Store) FrozenSubstitutionPolicy(ctx context.Context, vendorOrderID, lineID string) (SubstitutionPolicy, error) {
	var raw string
	err := s.db.QueryRow(ctx, `SELECT COALESCE(p.substitution_policy,o.substitution_policy,'EXACT_ONLY')
		FROM commerce_vendor_order_lines l
		LEFT JOIN commerce_shop_picking_snapshots p ON p.order_line_id=l.id
		LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id
		WHERE l.id=$1 AND l.vendor_order_id=$2`, lineID, vendorOrderID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrderNotFound
	}
	if err != nil {
		return "", err
	}
	return ParseSubstitutionPolicy(raw)
}

// ensureSubstitutionCommercialChange returns the APPLIED change that authorizes
// a costlier replacement, or the error naming the change still to approve.
func (s *Service) ensureSubstitutionCommercialChange(ctx context.Context, proposal SubstitutionProposal) (string, error) {
	authority, ok := s.changeQuotes.(SubstitutionCommercialChangeQuoteAuthority)
	if !ok || authority == nil {
		return "", ErrCommercialChangeRequired
	}
	req, err := s.store.SubstitutionCommercialContext(ctx, proposal)
	if err != nil {
		return "", err
	}
	quote, err := authority.CreateSubstitutionReplacementQuote(ctx, SubstitutionCommercialChangeRequest{
		CustomerID: req.CustomerID, CheckoutSessionID: req.CheckoutSessionID, FulfilmentPlanID: req.FulfilmentPlanID,
		CartItemID: req.CartItemID, ItemRef: req.ItemRef, VariantRef: req.VariantRef, ListingRef: req.ListingRef,
		SellUnitRef: req.SellUnitRef, UnitPriceMinor: req.UnitPriceMinor,
	})
	if err != nil {
		return "", err
	}
	if quote.CustomerID != proposal.CustomerID || quote.ID == "" || quote.Fingerprint == "" || quote.TotalMinor <= 0 {
		return "", ErrCommercialChangeRequired
	}
	parent, err := s.store.GetParent(ctx, proposal.CustomerID, proposal.ParentOrderID)
	if err != nil {
		return "", err
	}
	quoteSession := quote.CheckoutSessionID
	if quoteSession == "" {
		quoteSession = req.CheckoutSessionID
	}
	change, err := s.store.CreateCommercialChange(ctx, parent, ChangeRequest{CheckoutSessionID: quoteSession, FulfilmentPlanID: quote.FulfilmentPlanID}, quote, s.now().UTC())
	if errors.Is(err, ErrCommercialChangeConflict) {
		return "", ErrCommercialChangeRequired
	}
	if err != nil {
		return "", err
	}
	// The customer has approved the replacement quote and Wallet holds the
	// extra amount: the costlier replacement may now become effective.
	if change.State == ChangeApplied {
		return change.ID, nil
	}
	if change.State == ChangeRejected || change.State == ChangeExpired {
		return "", ErrCommercialChangeExpired
	}
	// Not yet approved/paid: tell the customer WHICH change to approve. The
	// change's own approval records the one POSITIVE_DELTA adjustment and the
	// Wallet hold; a second adjustment here would duplicate that evidence.
	return "", &SubstitutionChangeRequiredError{ChangeID: change.ID, State: change.State}
}

// SubstitutionChangeRequiredError refuses a costlier substitution until its
// replacement quote is approved and the extra amount is held by Wallet. It
// names the commercial change the customer must approve.
type SubstitutionChangeRequiredError struct {
	ChangeID string
	State    CommercialChangeState
}

func (e *SubstitutionChangeRequiredError) Error() string {
	return "substitution requires approval of commercial change " + e.ChangeID
}

func (e *SubstitutionChangeRequiredError) Unwrap() error { return ErrCommercialChangeRequired }

func (s *Service) ApplyLogisticsEvent(ctx context.Context, event LogisticsOrderEvent) (VendorOrder, ParentOrder, error) {
	if !s.enabled {
		return VendorOrder{}, ParentOrder{}, ErrDisabled
	}
	slot, err := s.store.resolveLogisticsSlotRef(ctx, event)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	event.MissionRef = slot
	order, parent, err := s.store.ApplyLogisticsEvent(ctx, event, s.now().UTC())
	if err != nil {
		return order, parent, err
	}
	// The authority's own evidence is projected onto the fulfilment aggregate:
	// mission status and parcel custody. Stale and duplicate events are
	// absorbed there rather than applied, so an at-least-once pipeline cannot
	// walk a delivered parcel backwards.
	if s.fulfilmentProjection != nil {
		if projectErr := s.fulfilmentProjection.ProjectLogisticsEvent(ctx, event); projectErr != nil {
			return order, parent, projectErr
		}
	}
	if strings.ToUpper(strings.TrimSpace(event.EventType)) != "DELIVERY_FAILED" {
		return order, parent, nil
	}
	failure, err := parseDeliveryFailurePayload(event.Payload)
	if err != nil {
		return order, parent, err
	}
	leg, _, err := s.store.PaymentLegForVendorOrder(ctx, order.CustomerID, order.ID)
	if err != nil {
		return order, parent, err
	}
	decision, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: order, MaxAuthorizedMinor: leg.AmountMinor, DeliveryFailurePolicy: failure.Policy, PreserveProviderEarning: failure.PreserveProviderEarning})
	if err != nil {
		return order, parent, err
	}
	_, err = s.ensureAndExecuteRefundDecision(ctx, order, event.MissionRef, "commerce-delivery-failure-"+event.EventID, decision, false)
	return order, parent, err
}

type deliveryFailurePayload struct {
	Policy                  DeliveryFailurePolicy `json:"financial_policy"`
	PreserveProviderEarning bool                  `json:"preserve_provider_earning"`
}

func parseDeliveryFailurePayload(raw json.RawMessage) (deliveryFailurePayload, error) {
	var payload deliveryFailurePayload
	if len(raw) == 0 || len(raw) > 32<<10 || json.Unmarshal(raw, &payload) != nil {
		return payload, ErrOrderConflict
	}
	switch payload.Policy {
	case DeliveryFailureDeliveryOnly, DeliveryFailureGoodsAndDelivery, DeliveryFailureCustomerCaused, DeliveryFailureReplanPossible, DeliveryFailureAmbiguous:
		return payload, nil
	default:
		return payload, ErrOrderConflict
	}
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
	if outcome == AvailabilityUnavailable && (picked != 0 || unavailable != ordered) {
		return LineOutcome{}, ErrOrderConflict
	}
	if outcome == AvailabilityFullyAvailable && picked != ordered {
		var replacements int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(proposed_quantity),0) FROM commerce_substitution_proposals WHERE order_line_id=$1 AND vendor_order_id=$2 AND status='APPROVED'`, lineID, vendorOrderID).Scan(&replacements); err != nil {
			return LineOutcome{}, err
		}
		if replacements != ordered-picked {
			return LineOutcome{}, ErrOrderConflict
		}
	}
	var current LineOutcome
	err = tx.QueryRow(ctx, `SELECT id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,version,created_at,updated_at FROM commerce_vendor_order_line_outcomes WHERE order_line_id=$1 FOR UPDATE`, lineID).Scan(&current.ID, &current.OrderedQuantity, &current.PickedQuantity, &current.UnavailableQuantity, &current.ActualMeasureQuantity, &current.Outcome, &current.Version, &current.CreatedAt, &current.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current.ID = "cvoo_" + uuid.NewString()
		current.Version = 1
		_, err = tx.Exec(ctx, `INSERT INTO commerce_vendor_order_line_outcomes(id,parent_order_id,vendor_order_id,order_line_id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,picking_state,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $9='FULLY_AVAILABLE' THEN 'COMPLETE' ELSE 'BLOCKED' END,1,$10,$10)`, current.ID, parentID, vendorOrderID, lineID, ordered, picked, unavailable, actualMeasure, outcome, now)
		if err != nil {
			return LineOutcome{}, err
		}
	} else if err != nil {
		return LineOutcome{}, err
	} else {
		if current.Version != expectedVersion {
			return LineOutcome{}, ErrVersionConflict
		}
		tag, e := tx.Exec(ctx, `UPDATE commerce_vendor_order_line_outcomes SET picked_quantity=$1,unavailable_quantity=$2,actual_measure_quantity=$3,outcome=$4,picking_state=CASE WHEN $4='FULLY_AVAILABLE' THEN 'COMPLETE' ELSE 'BLOCKED' END,version=version+1,updated_at=$5 WHERE order_line_id=$6 AND version=$7`, picked, unavailable, actualMeasure, outcome, now, lineID, current.Version)
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
	err := s.db.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE idempotency_key=$1`, idempotencyKey).Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, optionalTime{&p.RespondedAt}, &p.UpdatedAt, &p.CaseID)
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
	var parentID, ownerCustomer, branch, currentStatus, lineSnapshot, frozenPolicy string
	var qualitySnapshot []byte
	var originalPrice, orderedQuantity int64
	if err = tx.QueryRow(ctx, `SELECT v.parent_order_id,v.customer_id,v.branch_id,v.status,l.unit_price_minor,l.quantity,l.snapshot::text,COALESCE(sp.substitution_policy,o.substitution_policy,'CONTACT_FIRST'),COALESCE(sp.quality_preference_snapshot,'[]'::jsonb) FROM commerce_vendor_orders v JOIN commerce_vendor_order_lines l ON l.vendor_order_id=v.id LEFT JOIN commerce_shop_picking_snapshots sp ON sp.order_line_id=l.id LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id WHERE v.id=$1 AND v.business_id=$2 AND l.id=$3 FOR UPDATE OF v, l`, vendorOrderID, businessID, lineID).Scan(&parentID, &ownerCustomer, &branch, &currentStatus, &originalPrice, &orderedQuantity, &lineSnapshot, &frozenPolicy, &qualitySnapshot); errors.Is(err, pgx.ErrNoRows) {
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
	var caseID string
	if err = tx.QueryRow(ctx, `SELECT id FROM commerce_substitution_cases WHERE order_line_id=$1 AND state IN ('OPEN','AWAITING_CUSTOMER','AUTO_REFUND_PENDING') ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, lineID).Scan(&caseID); errors.Is(err, pgx.ErrNoRows) {
		caseID = "scase_" + uuid.NewString()
		policy := "CONTACT_FIRST"
		switch frozenPolicy {
		case "EXACT_ONLY", "AUTO_REFUND":
			return SubstitutionProposal{}, ErrOrderConflict
		case "CONTACT_FIRST", "SPECIFIC_REPLACEMENT", "VENDOR_SUGGEST_AND_APPROVE":
			policy = frozenPolicy
		}
		caseSnapshot := json.RawMessage(lineSnapshot)
		if !json.Valid(caseSnapshot) {
			caseSnapshot = json.RawMessage(`{}`)
		}
		policySnapshot := json.RawMessage(fmt.Sprintf(`{"policy":%q}`, frozenPolicy))
		if frozenPolicy == "" {
			policySnapshot = json.RawMessage(`{"policy":"CONTACT_FIRST"}`)
		}
		if len(qualitySnapshot) == 0 || !json.Valid(qualitySnapshot) {
			qualitySnapshot = []byte(`[]`)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_substitution_cases(id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,unavailable_quantity,original_line_snapshot,policy_snapshot,quality_snapshot,deadline_at,fallback_policy,state,reason,version,idempotency_key,correlation_ref,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12,$13,'AWAITING_CUSTOMER',$14,1,$15,$16,$17,$17)`, caseID, parentID, vendorOrderID, lineID, customerID, businessID, branch, orderedQuantity, caseSnapshot, policySnapshot, qualitySnapshot, proposal.ExpiresAt, policy, proposal.Reason, "case-for-"+idempotencyKey, idempotencyKey, now); err != nil {
			return SubstitutionProposal{}, err
		}
	} else if err != nil {
		return SubstitutionProposal{}, err
	}
	proposal.CaseID = caseID
	_, err = tx.Exec(ctx, `INSERT INTO commerce_substitution_proposals(id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,version,idempotency_key,created_at,updated_at,case_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'PENDING',1,$17,$18,$18,$19)`, proposal.ID, parentID, vendorOrderID, lineID, customerID, businessID, branch, proposal.ProposedItemRef, proposal.ProposedVariantRef, proposal.ProposedCatalogueRef, proposal.ReplacementSnapshot, proposal.ProposedQuantity, proposal.ProposedPriceMinor, proposal.PriceDifferenceMinor, proposal.Reason, proposal.ExpiresAt, idempotencyKey, now, caseID)
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

// RespondSubstitution records the customer's decision. appliedChangeID names
// the commercial change that authorized a costlier replacement; the store
// verifies it is APPLIED for this order itself rather than trusting the caller.
func (s *Store) RespondSubstitution(ctx context.Context, customerID, proposalID string, approve bool, expectedVersion int64, appliedChangeID string, now time.Time) (SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return SubstitutionProposal{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SubstitutionProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p SubstitutionProposal
	err = tx.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE id=$1 AND customer_id=$2 FOR UPDATE`, proposalID, customerID).Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, optionalTime{&p.RespondedAt}, &p.UpdatedAt, &p.CaseID)
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
		if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_proposals SET status='EXPIRED',version=version+1,updated_at=$1,responded_at=$1 WHERE id=$2 AND status='PENDING'`, now, proposalID); err != nil {
			return SubstitutionProposal{}, err
		}
		if p.CaseID != "" {
			if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_cases SET state='EXPIRED',version=version+1,updated_at=$1,resolved_at=$1 WHERE id=$2 AND state IN ('OPEN','AWAITING_CUSTOMER')`, now, p.CaseID); err != nil {
				return SubstitutionProposal{}, err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return SubstitutionProposal{}, err
		}
		return SubstitutionProposal{}, ErrCommercialChangeExpired
	}
	if p.Version != expectedVersion {
		return SubstitutionProposal{}, ErrVersionConflict
	}
	changeApplied := false
	if approve && p.PriceDifferenceMinor > 0 && strings.TrimSpace(appliedChangeID) != "" {
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_order_changes WHERE id=$1 AND parent_order_id=$2 AND customer_id=$3 AND state='APPLIED')`, appliedChangeID, p.ParentOrderID, p.CustomerID).Scan(&changeApplied); err != nil {
			return SubstitutionProposal{}, err
		}
	}
	if approve && p.PriceDifferenceMinor > 0 && !changeApplied {
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
	if p.CaseID != "" {
		caseState := "REJECTED"
		if approve {
			caseState = "APPROVED"
		}
		caseWhere := `id=$3 AND state IN ('OPEN','AWAITING_CUSTOMER')`
		if !approve {
			caseWhere += ` AND NOT EXISTS (SELECT 1 FROM commerce_substitution_proposals WHERE case_id=$3 AND status='PENDING')`
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_cases SET state=$1,version=version+1,updated_at=$2,resolved_at=$2 WHERE `+caseWhere, caseState, now, p.CaseID); err != nil {
			return SubstitutionProposal{}, err
		}
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
	if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='PICKING',version=version+1,updated_at=$1 WHERE id=$2 AND status IN ('SUBSTITUTION_REQUIRED','AWAITING_CUSTOMER') AND NOT EXISTS (SELECT 1 FROM commerce_substitution_proposals WHERE vendor_order_id=$2 AND status='PENDING')`, now, p.VendorOrderID); err != nil {
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
	return scanSubstitution(s.db.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE id=$1 AND customer_id=$2`, proposalID, customerID))
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
	var candidateSnapshot struct {
		SellUnitRef string `json:"sell_unit_ref"`
	}
	if err = json.Unmarshal(proposal.ReplacementSnapshot, &candidateSnapshot); err != nil {
		return SubstitutionCommercialChangeRequest{}, err
	}
	request.SellUnitRef = candidateSnapshot.SellUnitRef
	return request, nil
}

// optionalTime scans a nullable timestamp into a plain time.Time; NULL (for
// example a proposal nobody has answered yet) is the zero time.
type optionalTime struct{ target *time.Time }

func (o optionalTime) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*o.target = time.Time{}
	case time.Time:
		*o.target = value
	default:
		return fmt.Errorf("unsupported timestamp %T", src)
	}
	return nil
}

func scanSubstitution(row scanner) (SubstitutionProposal, error) {
	var p SubstitutionProposal
	err := row.Scan(&p.ID, &p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.CustomerID, &p.BusinessID, &p.BranchID, &p.ProposedItemRef, &p.ProposedVariantRef, &p.ProposedCatalogueRef, &p.ReplacementSnapshot, &p.ProposedQuantity, &p.ProposedPriceMinor, &p.PriceDifferenceMinor, &p.Reason, &p.ExpiresAt, &p.Status, &p.ResponseRef, &p.Version, &p.CreatedAt, optionalTime{&p.RespondedAt}, &p.UpdatedAt, &p.CaseID)
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
	rows, err := s.db.Query(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE customer_id=$1 AND status='PENDING' ORDER BY created_at,id LIMIT $2`, customerID, limit)
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

// ListApprovedSubstitutions is the durable recovery feed for the downstream
// Inventory step. Approval is already authoritative Orders state; a process
// crash after it can safely replay the idempotent Inventory command.
func (s *Store) ListApprovedSubstitutions(ctx context.Context, limit int) ([]SubstitutionProposal, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE status='APPROVED' ORDER BY updated_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SubstitutionProposal, 0)
	for rows.Next() {
		p, scanErr := scanSubstitution(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// resolveLogisticsSlotRef maps the reference an executing authority knows —
// the live mission reference Vendor gave it at creation — to the plan slot the
// VendorOrder is bound to. Dispatch's envelope already carries the slot.
// Only the mission row the SAME authority executes resolves, so an event from
// a replaced or cancelled executor keeps its stale reference and is refused
// by the store's own binding check rather than moving the order.
func (s *Store) resolveLogisticsSlotRef(ctx context.Context, event LogisticsOrderEvent) (string, error) {
	ref := strings.TrimSpace(event.MissionRef)
	if s == nil || s.db == nil || ref == "" {
		return ref, nil
	}
	var slot string
	err := s.db.QueryRow(ctx, `SELECT plan_mission_ref FROM commerce_order_missions
		WHERE parent_order_id=$1 AND mission_ref=$2 AND service=$3`,
		event.ParentOrderID, ref, string(event.Source)).Scan(&slot)
	if errors.Is(err, pgx.ErrNoRows) {
		return ref, nil
	}
	if err != nil {
		return "", err
	}
	return slot, nil
}

// ApplyLogisticsEvent is the only path by which platform execution may move a
// VendorOrder into pickup/delivery states. Source versions and event ids make
// retries and delayed events monotonic and harmless.
func (s *Store) ApplyLogisticsEvent(ctx context.Context, event LogisticsOrderEvent, now time.Time) (VendorOrder, ParentOrder, error) {
	if s == nil || s.db == nil {
		return VendorOrder{}, ParentOrder{}, ErrDependencyMissing
	}
	if event.Source != LogisticsDispatch && event.Source != LogisticsHauling && event.Source != LogisticsTaxi {
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
	current, err = scanVendorOrder(tx.QueryRow(ctx, `SELECT `+vendorColumns+` FROM commerce_vendor_orders v WHERE v.id=$1 AND v.parent_order_id=$2 FOR UPDATE`, event.VendorOrderID, event.ParentOrderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorOrder{}, ParentOrder{}, ErrOrderNotFound
	}
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if current.DeliveryResponsibility != DeliveryPlatform || current.LogisticsMissionRef != event.MissionRef {
		return VendorOrder{}, ParentOrder{}, ErrForbidden
	}
	// A slot survives replanning, but its executing authority can change.
	// Check the live binding before mutating the order, even when an old
	// executor names the still-valid slot instead of its replaced mission ref.
	var executor, missionStatus string
	err = tx.QueryRow(ctx, `SELECT service,status FROM commerce_order_missions
	 WHERE parent_order_id=$1 AND plan_mission_ref=$2 ORDER BY updated_at DESC,id LIMIT 1`, current.ParentOrderID, current.LogisticsMissionRef).Scan(&executor, &missionStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		executor = string(serviceFor(current))
	} else if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	} else if missionStatus != string(MissionAccepted) {
		return VendorOrder{}, ParentOrder{}, ErrForbidden
	}
	if executor != string(event.Source) {
		return VendorOrder{}, ParentOrder{}, ErrForbidden
	}
	timingEvent := isLogisticsTimingEvent(event.EventType)
	failureEvent := strings.EqualFold(strings.TrimSpace(event.EventType), "DELIVERY_FAILED")
	signalEvent := isLogisticsSignalEvent(event.EventType)

	// The authority's own status is normalized before it is acted on
	// (VND-020). The raw event type is still what gets stored, so nothing
	// richer is lost; what the normalization buys is that a service sending
	// its real state — Taxi's AT_DROPOFF, Hauling's en_route_delivery — is
	// understood instead of being silently filed as stale.
	commerceStatus := event.CommerceStatus
	if commerceStatus == logistics.StatusUnknown {
		commerceStatus = logistics.NormalizeStatusFor(string(event.Source), event.EventType)
	}
	pickedUp := commerceStatus == logistics.StatusPickupVerified
	delivering := commerceStatus == logistics.StatusInTransit
	delivered := commerceStatus == logistics.StatusDeliveryVerified
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
	switch {
	case pickedUp:
		target = VendorStatusPickedUp
	case delivering:
		target = VendorStatusDelivering
	case delivered:
		target = VendorStatusDelivered
	}
	ignored := !timingEvent && !failureEvent && !signalEvent && rank(current.Status) >= rank(target)
	if !ignored {
		valid := signalEvent || failureEvent ||
			(pickedUp && (current.Status == VendorStatusReadyForPickup || current.Status == VendorStatusReady)) ||
			(delivering && (current.Status == VendorStatusPickedUp || current.Status == VendorStatusHandedOver)) ||
			(delivered && (current.Status == VendorStatusDelivering || current.Status == VendorStatusPickedUp || current.Status == VendorStatusHandedOver))
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
	if timingEvent {
		if err = s.applyLogisticsTimingTx(ctx, tx, event, now); err != nil {
			return VendorOrder{}, ParentOrder{}, err
		}
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
	if signalEvent {
		if _, _, e := s.applyDispatchConfidenceETATx(ctx, tx, event, current); e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		if signalNotificationAction(event.EventType) && s.outbox != nil {
			action := "commerce.vendor_order.logistics_" + strings.ToLower(event.EventType)
			if e := appendEvent(ctx, tx, s.outbox, current.ParentOrderID, current.ID, current.CustomerID,
				action, "PLATFORM", string(event.Source), string(current.Status), string(current.Status), now); e != nil {
				return VendorOrder{}, ParentOrder{}, e
			}
		}
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
	if failureEvent {
		_, _, _ = s.applyDispatchConfidenceETATx(ctx, tx, event, current)
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
		// Activity uses the canonical Vendor transition; preserve the raw
		// authority vocabulary in commerce_logistics_order_events above.
		action := "commerce.vendor_order.logistics_" + strings.ToLower(string(target))
		if e = appendEvent(ctx, tx, s.outbox, current.ParentOrderID, current.ID, current.CustomerID, action, "PLATFORM", string(event.Source), string(current.Status), string(target), now); e != nil {
			return VendorOrder{}, ParentOrder{}, e
		}
		if _, _, e = s.applyDispatchConfidenceETATx(ctx, tx, event, current); e != nil {
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
		parent, e = refreshParentState(ctx, tx, current.ParentOrderID, current.CustomerID, previousState, platformFeeTax, now, s.movements())
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
