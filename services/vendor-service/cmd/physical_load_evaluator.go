package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// physicalLoadEvaluator answers "is this mission's frozen vehicle class still
// safe for what was actually packed?" (Karryam V4 §14.6).
//
// The load is rebuilt from the VendorOrder's FINAL line quantities - picking
// outcomes, measured quantities and approved substitutions - and evaluated by
// the same Section 17C planner checkout used: catalogue logistics profiles,
// quantity aggregation, the versioned requirement/vehicle engine and the owning
// authority's real quote. Nothing here computes a fee.
type physicalLoadEvaluator struct {
	db      *pgxpool.Pool
	planner *fulfilment.Planner
}

func newPhysicalLoadEvaluator(db *pgxpool.Pool, planner *fulfilment.Planner) *physicalLoadEvaluator {
	if db == nil || planner == nil {
		return nil
	}
	return &physicalLoadEvaluator{db: db, planner: planner}
}

// finalLine is one line of the load as it will actually travel.
type finalLine struct {
	ID, Domain, BusinessID, BranchID      string
	ItemRef, VariantRef, ListingRef, Unit string
	Quantity                              int64
	MeasureQuantity                       int64
}

// vendorOrderFinalLines reads the load a VendorOrder will hand over.
func (e *physicalLoadEvaluator) vendorOrderFinalLines(ctx context.Context, vendorOrderID string) ([]finalLine, error) {
	rows, err := e.db.Query(ctx, `
		SELECT l.id,v.domain,v.business_id,v.branch_id,l.item_ref,l.variant_ref,l.listing_ref,l.sell_unit_ref,
		       l.quantity,o.outcome,COALESCE(o.picked_quantity,0),
		       COALESCE(o.actual_measure_quantity,0),COALESCE(l.expected_measure_quantity,0),COALESCE(vm.measurement_mode,'')
		  FROM commerce_vendor_order_lines l
		  JOIN commerce_vendor_orders v ON v.id=l.vendor_order_id
		  LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id
		  LEFT JOIN commerce_variable_measurements vm ON vm.order_line_id=l.id
		 WHERE l.vendor_order_id=$1
		 ORDER BY l.id`, vendorOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []finalLine
	for rows.Next() {
		var line finalLine
		var outcome *string
		var picked, actual, expected int64
		var measurementMode string
		if err = rows.Scan(&line.ID, &line.Domain, &line.BusinessID, &line.BranchID, &line.ItemRef,
			&line.VariantRef, &line.ListingRef, &line.Unit, &line.Quantity, &outcome, &picked,
			&actual, &expected, &measurementMode); err != nil {
			return nil, err
		}
		if outcome != nil && *outcome != "PENDING" {
			// Picking decided the quantity: what was picked is what travels.
			line.Quantity = picked
		}
		if measurementMode == "VARIABLE_WEIGHT" && line.Quantity > 0 {
			if actual <= 0 {
				return nil, orders.ErrPhysicalReplanRequired
			}
			line.MeasureQuantity = actual
		} else if actual > 0 && expected > 0 && line.Quantity > 0 {
			// A measured variable line travels at its measured size. Rounded
			// UP to whole sell units, so measurement can only size the vehicle
			// up, never down.
			if line.Quantity > math.MaxInt64/actual {
				return nil, orders.ErrPhysicalReplanRequired
			}
			total := line.Quantity * actual
			line.Quantity = total / expected
			if total%expected != 0 {
				line.Quantity++
			}
		}
		if line.Quantity > 0 {
			lines = append(lines, line)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// Approved substitutions travel too.
	subs, err := e.db.Query(ctx, `
		SELECT p.id,v.domain,v.business_id,v.branch_id,p.proposed_item_ref,p.proposed_variant_ref,
		       p.proposed_catalogue_ref,p.proposed_quantity
		  FROM commerce_substitution_proposals p
		  JOIN commerce_vendor_orders v ON v.id=p.vendor_order_id
		 WHERE p.vendor_order_id=$1 AND p.status='APPROVED' AND p.proposed_quantity>0
		 ORDER BY p.id`, vendorOrderID)
	if err != nil {
		return nil, err
	}
	defer subs.Close()
	for subs.Next() {
		var line finalLine
		if err = subs.Scan(&line.ID, &line.Domain, &line.BusinessID, &line.BranchID, &line.ItemRef,
			&line.VariantRef, &line.ListingRef, &line.Quantity); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, subs.Err()
}

// EvaluateMissionLoad implements orders.PhysicalLoadEvaluator.
func (e *physicalLoadEvaluator) EvaluateMissionLoad(
	ctx context.Context, order orders.ParentOrder, mission orders.OrderMission,
) (orders.PhysicalDecision, error) {
	decision := orders.PhysicalDecision{PolicyVersion: fulfilment.DefaultCapabilityPolicyVersion}
	groups := map[string]*fulfilment.GroupInput{}
	var keys []string
	var fingerprintLines []string
	for _, vendorOrderID := range mission.VendorOrderIDs {
		lines, err := e.vendorOrderFinalLines(ctx, vendorOrderID)
		if err != nil {
			return decision, err
		}
		for _, line := range lines {
			key := line.BusinessID + "|" + line.BranchID
			group, found := groups[key]
			if !found {
				group = &fulfilment.GroupInput{CheckoutGroupID: vendorOrderID, BusinessID: line.BusinessID,
					BranchID: line.BranchID, Currency: order.Currency}
				groups[key] = group
				keys = append(keys, key)
			}
			group.Lines = append(group.Lines, fulfilment.LineLogistics{
				CheckoutLineID: line.ID, Domain: line.Domain, BusinessID: line.BusinessID,
				BranchID: line.BranchID, ItemRef: line.ItemRef, VariantRef: line.VariantRef,
				ListingRef: line.ListingRef, SellUnitRef: line.Unit, Quantity: line.Quantity, MeasureQuantity: line.MeasureQuantity,
			})
			fingerprintLines = append(fingerprintLines, fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d",
				line.ID, line.ItemRef, line.VariantRef, line.ListingRef, line.Unit, line.Quantity, line.MeasureQuantity))
		}
	}
	sort.Strings(keys)
	sort.Strings(fingerprintLines)
	sum := sha256.Sum256([]byte(strings.Join(append([]string{
		mission.ID, string(mission.Service), mission.VehicleClass,
		fmt.Sprintf("%.6f,%.6f", order.Address.Latitude, order.Address.Longitude),
	}, fingerprintLines...), "\n")))
	decision.EvidenceFingerprint = hex.EncodeToString(sum[:])

	if len(keys) == 0 {
		// Nothing left to carry: the frozen class is trivially still safe.
		decision.Selected = true
		decision.Service, decision.VehicleClass = mission.Service, mission.VehicleClass
		return decision, nil
	}

	request := fulfilment.PlanRequest{
		CustomerID: order.CustomerID, CheckoutSessionID: "physical_" + mission.ID,
		Currency: order.Currency, CorrelationID: order.ID,
		Destination: fulfilment.Destination{Latitude: order.Address.Latitude,
			Longitude: order.Address.Longitude, Address: order.Address.Address},
	}
	current := mission.VehicleClass
	quoteWhen := func(class fulfilment.VehicleClass) bool {
		return physicalRank(string(class)) > physicalRank(current)
	}

	var evidence []fulfilment.RequirementSnapshot
	var selected *fulfilment.PlatformReevaluation
	for _, key := range keys {
		answer, err := e.planner.EvaluatePlatformLoad(ctx, request, *groups[key], quoteWhen)
		if err != nil {
			return decision, err
		}
		evidence = append(evidence, answer.Snapshot)
		decision.ConfirmedWeightGrams += answer.Requirement.TotalWeightGrams
		decision.ConfirmedPackageCount += answer.Requirement.PackageCount
		if !answer.Selected {
			decision.Selected, decision.ReasonCode = false, answer.ReasonCode
			decision.Evidence, _ = json.Marshal(evidence)
			return decision, nil
		}
		copied := answer
		if selected == nil || physicalRank(string(copied.VehicleClass)) > physicalRank(string(selected.VehicleClass)) {
			selected = &copied
		}
	}
	decision.Evidence, _ = json.Marshal(evidence)
	decision.Selected = true
	decision.Service = orders.LogisticsService(selected.Service)
	decision.VehicleClass = string(selected.VehicleClass)
	if physicalRank(decision.VehicleClass) > physicalRank(current) {
		if len(keys) > 1 {
			// A multi-pickup mission whose load outgrew its class needs a
			// consolidated replan; a single-stop quote would misstate the route.
			decision.Selected, decision.ReasonCode = false, "CONSOLIDATED_MISSION_REPLAN_REQUIRED"
			return decision, nil
		}
		if selected.Mission == nil {
			// The larger class is required but its authority did not quote:
			// the load is refused, never downgraded.
			decision.ReasonCode = nonEmptyReason(selected.ReasonCode, "ESCALATION_QUOTE_UNAVAILABLE")
			return decision, nil
		}
		decision.AuthorityFeeMinor = selected.Mission.Funding.GrossMinor
		decision.AuthorityQuoteID = selected.Mission.Quote.QuoteID
		decision.AuthorityQuoteReference = selected.Mission.Quote.QuoteReference
	}
	return decision, nil
}

func physicalRank(class string) int {
	switch strings.ToUpper(strings.TrimSpace(class)) {
	case "MOTORCYCLE":
		return 1
	case "CAR":
		return 2
	case "VAN":
		return 3
	case "TRUCK":
		return 4
	default:
		return 99
	}
}

func nonEmptyReason(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
