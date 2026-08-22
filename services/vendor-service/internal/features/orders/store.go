package orders

// Section 17E persistence.
//
// Order creation is one PostgreSQL transaction. Multi-vendor creation is
// therefore atomic by construction: if the third VendorOrder cannot be
// written, no ParentOrder is left behind claiming a purchase succeeded.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Store owns the Section 17E tables.
type Store struct {
	db     *pgxpool.Pool
	outbox *vendorevents.DomainPublisher
}

// NewStore builds the order store.
func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// SetEventPublisher wires the existing transactional outbox into lifecycle
// mutations. It is optional for isolated package tests.
func (s *Store) SetEventPublisher(publisher *vendorevents.DomainPublisher) {
	if s != nil {
		s.outbox = publisher
	}
}

const parentColumns = `
	p.id,p.customer_id,p.quote_id,p.quote_version,p.quote_fingerprint,
	p.checkout_session_id,p.checkout_validation_revision,p.cart_id,p.cart_version,
	p.fulfilment_plan_id,p.fulfilment_plan_revision,p.fulfilment_plan_version,
	p.domain,p.currency,p.selected_fulfilment_mode,p.address_snapshot,
	p.has_receiver,p.receiver_snapshot,
	p.customer_approved_total_minor,p.max_authorized_total_minor,
	p.product_subtotal_minor,p.delivery_subtotal_minor,p.fee_subtotal_minor,
	p.tax_subtotal_minor,p.discount_total_minor,p.subsidy_total_minor,
	p.platform_fee_tax_minor,p.funding_breakdown,p.price_disclosure_snapshot,
	p.payment_operation_ref,p.state,p.payment_state,p.vendor_order_count,
	p.refund_prepared_minor,p.settlement_prepared_minor,
	p.version,p.created_at,p.updated_at,p.terminal_at`

const vendorColumns = `
	v.id,v.parent_order_id,v.customer_id,v.business_id,v.branch_id,v.domain,v.currency,
	v.quote_group_ref,v.product_subtotal_minor,v.max_product_subtotal_minor,
	v.product_vendor_funded_minor,v.product_platform_funded_minor,v.product_other_funded_minor,
	v.order_vendor_funded_minor,v.order_platform_funded_minor,v.order_other_funded_minor,
	v.delivery_responsibility,v.delivery_fee_beneficiary,v.delivery_gross_minor,
	v.delivery_customer_minor,v.delivery_vendor_funded_minor,v.delivery_platform_funded_minor,
	v.delivery_other_funded_minor,
	v.customer_payable_minor,v.vendor_payable_minor,
	v.reservation_required,v.inventory_state,v.inventory_reservation_ref,v.inventory_reservation_source_ref,
	v.fulfilment_mode,v.logistics_plan_group_ref,v.logistics_mission_ref,
	v.vendor_own_delivery_state,v.status,v.cancel_reason_code,v.cancelled_by,
	v.refund_allocated_minor,v.settlement_allocated_minor,v.refund_state,v.refund_operation_ref,v.refund_refunded_minor,v.rejection_reason,
	v.version,v.created_at,v.updated_at,v.terminal_at`

// -----------------------------------------------------------------------------
// Creation
// -----------------------------------------------------------------------------

// Create writes one ParentOrder and every VendorOrder of an approved quote.
//
// Idempotency is structural: commerce_parent_orders.quote_id is UNIQUE, so the
// exact approved agreement can only ever produce one ParentOrder. A retry
// returns the existing order and reports created=false rather than duplicating
// an order, a vendor order, a line or an allocation.
func (s *Store) Create(
	ctx context.Context,
	source QuoteSource,
	plan AllocationPlan,
	lines []LineSource,
	now time.Time,
) (ParentOrder, bool, error) {
	if s == nil || s.db == nil {
		return ParentOrder{}, false, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return ParentOrder{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize creation per quote. Two concurrent creations then agree on one
	// winner instead of racing to the UNIQUE constraint and both failing.
	if _, err = tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,17))`, source.QuoteID); err != nil {
		return ParentOrder{}, false, err
	}

	var existingID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM commerce_parent_orders WHERE quote_id=$1 AND customer_id=$2`,
		source.QuoteID, source.CustomerID).Scan(&existingID)
	if err == nil {
		existing, loadErr := loadParent(ctx, tx, source.CustomerID, existingID)
		if loadErr != nil {
			return ParentOrder{}, false, loadErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return ParentOrder{}, false, commitErr
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ParentOrder{}, false, err
	}

	parentID := "cpo_" + uuid.NewString()
	addressJSON, err := json.Marshal(source.Address)
	if err != nil {
		return ParentOrder{}, false, err
	}
	receiverJSON := []byte(`{}`)
	if source.HasReceiver {
		if receiverJSON, err = json.Marshal(source.Receiver); err != nil {
			return ParentOrder{}, false, err
		}
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO commerce_parent_orders(
			id,customer_id,quote_id,quote_version,quote_fingerprint,
			checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,
			domain,currency,selected_fulfilment_mode,address_snapshot,
			has_receiver,receiver_snapshot,
			customer_approved_total_minor,max_authorized_total_minor,
			product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,
			tax_subtotal_minor,discount_total_minor,subsidy_total_minor,
			platform_fee_tax_minor,funding_breakdown,price_disclosure_snapshot,
			state,vendor_order_count,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
		       $19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,'CREATED',$30,1,$31,$31)`,
		parentID, source.CustomerID, source.QuoteID, source.QuoteVersion, source.QuoteFingerprint,
		source.CheckoutSessionID, source.CheckoutValidationRevision, source.CartID, source.CartVersion,
		source.FulfilmentPlanID, source.FulfilmentPlanRevision, source.FulfilmentPlanVersion,
		source.Domain, source.Currency, source.SelectedFulfilmentMode, addressJSON,
		source.HasReceiver, receiverJSON,
		source.FinalPayableMinor, source.MaxAuthorizedTotalMinor,
		source.ProductSubtotalMinor, source.DeliverySubtotalMinor, source.FeeSubtotalMinor,
		source.TaxSubtotalMinor, source.DiscountTotalMinor, source.SubsidyTotalMinor,
		plan.PlatformFeeTaxMinor, []byte(source.FundingBreakdown), []byte(source.PriceDisclosureSnapshot),
		len(plan.Vendors), now); err != nil {
		return ParentOrder{}, false, err
	}

	if err = insertVendorOrders(ctx, tx, parentID, source, plan, lines, now); err != nil {
		return ParentOrder{}, false, err
	}

	for _, allocation := range plan.ParentAllocations {
		if _, err = tx.Exec(ctx, `
			INSERT INTO commerce_order_allocations(
				id,parent_order_id,vendor_order_id,allocation_type,beneficiary_type,
				beneficiary_ref,funding_source,source_ref,amount_minor,created_at)
			VALUES($1,$2,NULL,$3,$4,$5,$6,$7,$8,$9)`,
			"coa_"+uuid.NewString(), parentID, allocation.AllocationType,
			allocation.BeneficiaryType, allocation.BeneficiaryRef, allocation.FundingSource,
			allocation.SourceRef, allocation.AmountMinor, now); err != nil {
			return ParentOrder{}, false, err
		}
	}

	if err = appendEvent(ctx, tx, s.outbox, parentID, "", source.CustomerID,
		ActionOrderCreated, "CUSTOMER", source.CustomerID, "", string(ParentStateCreated), now); err != nil {
		return ParentOrder{}, false, err
	}

	created, err := loadParent(ctx, tx, source.CustomerID, parentID)
	if err != nil {
		return ParentOrder{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ParentOrder{}, false, err
	}
	return created, true, nil
}

func insertVendorOrders(
	ctx context.Context,
	tx pgx.Tx,
	parentID string,
	source QuoteSource,
	plan AllocationPlan,
	lines []LineSource,
	now time.Time,
) error {
	groups := make(map[string]GroupSource, len(source.Groups))
	for _, group := range source.Groups {
		groups[group.QuoteGroupRef] = group
	}

	for _, vendor := range plan.Vendors {
		group, found := groups[vendor.QuoteGroupRef]
		if !found {
			return ErrQuoteNotOrderable
		}

		vendorOwnState := VendorOwnNotApplicable
		if vendor.DeliveryResponsibility == DeliveryVendorOwn {
			vendorOwnState = VendorOwnPending
		}

		vendorOrderID := "cvo_" + uuid.NewString()

		if _, err := tx.Exec(ctx, `
			INSERT INTO commerce_vendor_orders(
				id,parent_order_id,customer_id,business_id,branch_id,domain,currency,
				quote_group_ref,product_subtotal_minor,max_product_subtotal_minor,
				product_vendor_funded_minor,product_platform_funded_minor,product_other_funded_minor,
				order_vendor_funded_minor,order_platform_funded_minor,order_other_funded_minor,
				delivery_responsibility,delivery_fee_beneficiary,delivery_gross_minor,
				delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,
				delivery_other_funded_minor,
				customer_payable_minor,vendor_payable_minor,
				reservation_required,inventory_reservation_ref,inventory_reservation_source_ref,
				fulfilment_mode,logistics_plan_group_ref,logistics_mission_ref,
				vendor_own_delivery_state,status,version,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
			       $20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,'CREATED',1,$33,$33)`,
			vendorOrderID, parentID, source.CustomerID, vendor.BusinessID, vendor.BranchID,
			group.Domain, source.Currency, vendor.QuoteGroupRef,
			vendor.ProductSubtotalMinor, vendor.MaxProductSubtotalMinor,
			vendor.ProductVendorFundedMinor, vendor.ProductPlatformFundedMinor, vendor.ProductOtherFundedMinor,
			vendor.OrderVendorFundedMinor, vendor.OrderPlatformFundedMinor, vendor.OrderOtherFundedMinor,
			vendor.DeliveryResponsibility, vendor.DeliveryFeeBeneficiary, vendor.DeliveryGrossMinor,
			vendor.DeliveryCustomerMinor, vendor.DeliveryVendorFundedMinor,
			vendor.DeliveryPlatformFundedMinor, vendor.DeliveryOtherFundedMinor,
			vendor.CustomerPayableMinor, vendor.VendorPayableMinor,
			group.ReservationRequired, group.ReservationRef, group.ReservationSourceRef,
			source.SelectedFulfilmentMode, group.LogisticsPlanGroupRef, group.LogisticsMissionRef,
			vendorOwnState, now); err != nil {
			return err
		}

		if err := insertVendorOrderLines(
			ctx, tx, parentID, vendorOrderID, vendor, lines, now,
		); err != nil {
			return err
		}

		for _, allocation := range vendor.Allocations {
			if _, err := tx.Exec(ctx, `
				INSERT INTO commerce_order_allocations(
					id,parent_order_id,vendor_order_id,allocation_type,beneficiary_type,
					beneficiary_ref,funding_source,source_ref,amount_minor,created_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				"coa_"+uuid.NewString(), parentID, vendorOrderID, allocation.AllocationType,
				allocation.BeneficiaryType, allocation.BeneficiaryRef, allocation.FundingSource,
				allocation.SourceRef, allocation.AmountMinor, now); err != nil {
				return err
			}
		}

		if err := appendEvent(ctx, tx, nil, parentID, vendorOrderID, source.CustomerID,
			ActionVendorOrderCreated, "CUSTOMER", source.CustomerID,
			"", string(VendorStatusCreated), now); err != nil {
			return err
		}
	}
	return nil
}

func insertVendorOrderLines(
	ctx context.Context,
	tx pgx.Tx,
	parentID, vendorOrderID string,
	vendor VendorAllocation,
	lines []LineSource,
	now time.Time,
) error {
	commitment, err := json.Marshal(map[string]any{
		"delivery_responsibility": vendor.DeliveryResponsibility,
		"fee_beneficiary":         vendor.DeliveryFeeBeneficiary,
	})
	if err != nil {
		return err
	}

	written := 0
	for _, line := range lines {
		if line.QuoteGroupRef != vendor.QuoteGroupRef {
			continue
		}
		snapshot, marshalErr := json.Marshal(line)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO commerce_vendor_order_lines(
				id,vendor_order_id,parent_order_id,quote_line_ref,cart_item_id,
				item_name,item_ref,variant_ref,listing_ref,sell_unit_ref,
				quantity,unit_price_minor,quoted_total_minor,max_authorized_total_minor,
				discount_allocated_minor,fee_allocated_minor,tax_allocated_minor,
				pricing_basis,requested_measure_ref,expected_measure_quantity,
				measure_underfill_bps,measure_overfill_bps,
				price_reference,catalogue_revision,inventory_revision,inventory_item_ref,
				online_price_disclosure,store_price_parity_claimed,
				delivery_commitment,snapshot,created_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,0,0,$16,$17,$18,
			       $19,$20,$21,$22,$23,$24,'VENDOR_ONLINE_PRICE',false,$25,$26,$27)`,
			"cvl_"+uuid.NewString(), vendorOrderID, parentID, line.QuoteLineRef, line.CartItemID,
			line.ItemName, line.ItemRef, line.VariantRef, line.ListingRef, line.SellUnitRef,
			line.Quantity, line.UnitPriceMinor, line.QuotedTotalMinor, line.MaxAuthorizedTotalMinor,
			vendor.LineDiscounts[line.CartItemID],
			line.PricingBasis, line.RequestedMeasureRef, line.ExpectedMeasureQuantity,
			line.MeasureUnderfillBps, line.MeasureOverfillBps,
			line.PriceReference, line.CatalogueRevision, line.InventoryRevision, line.InventoryItemRef,
			commitment, snapshot, now); err != nil {
			return err
		}
		written++
	}

	// A vendor order with no item snapshot could never be honoured or
	// reconciled, so an empty allocation fails the whole creation.
	if written == 0 {
		return ErrQuoteNotOrderable
	}
	return nil
}

func appendEvent(
	ctx context.Context,
	tx pgx.Tx,
	outbox *vendorevents.DomainPublisher,
	parentID, vendorOrderID, customerID, action, actorType, actorRef, from, to string,
	now time.Time,
) error {
	var vendorRef any
	if vendorOrderID != "" {
		vendorRef = vendorOrderID
	}
	aggregateVersion := int64(1)
	vendorBusinessID := ""
	if vendorOrderID != "" {
		if err := tx.QueryRow(ctx, `SELECT version,business_id FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID).Scan(&aggregateVersion, &vendorBusinessID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO commerce_order_events(
			id,parent_order_id,vendor_order_id,customer_id,action,
			actor_type,actor_ref,from_status,to_status,occurred_at,
			correlation_ref,idempotency_ref,event_version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		"coe_"+uuid.NewString(), parentID, vendorRef, customerID, action,
		actorType, actorRef, from, to, now, actorRef, actorRef, aggregateVersion)
	if err != nil {
		return err
	}
	if outbox != nil && vendorOrderID != "" {
		return outbox.Publish(ctx, tx, action, "VendorOrder", vendorOrderID, aggregateVersion, vendorBusinessID, actorRef, actorRef, map[string]string{"from": from, "to": to})
	}
	return nil
}

// -----------------------------------------------------------------------------
// Reads
// -----------------------------------------------------------------------------

// GetParent reads one purchase and every vendor allocation inside it.
func (s *Store) GetParent(ctx context.Context, customerID, orderID string) (ParentOrder, error) {
	if s == nil || s.db == nil {
		return ParentOrder{}, ErrDependencyMissing
	}
	return loadParent(ctx, s.db, customerID, orderID)
}

// ListParents reads a customer's purchases, newest first.
func (s *Store) ListParents(ctx context.Context, customerID string, limit int) ([]ParentOrder, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+parentColumns+` FROM commerce_parent_orders p
		  WHERE p.customer_id=$1 ORDER BY p.created_at DESC, p.id LIMIT $2`,
		customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []ParentOrder
	for rows.Next() {
		order, scanErr := scanParent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		orders = append(orders, order)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// The summary list deliberately carries no vendor orders: a customer
	// listing does not need every allocation of every past purchase.
	return orders, nil
}

func loadParent(ctx context.Context, q queryer, customerID, orderID string) (ParentOrder, error) {
	row := q.QueryRow(ctx,
		`SELECT `+parentColumns+` FROM commerce_parent_orders p
		  WHERE p.id=$1 AND p.customer_id=$2`, orderID, customerID)

	order, err := scanParent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ParentOrder{}, ErrOrderNotFound
	}
	if err != nil {
		return ParentOrder{}, err
	}
	// A confirmed commercial change overlays the active fulfilment evidence
	// without rewriting the original quote/order evidence captured above. The
	// immutable change row retains the before/after chain; execution reads the
	// latest applied address and plan.
	var appliedAddress []byte
	var appliedPlan string
	if overlayErr := q.QueryRow(ctx, `SELECT applied_address_snapshot,applied_fulfilment_plan_id FROM commerce_order_change_applications WHERE parent_order_id=$1 ORDER BY applied_at DESC,id DESC LIMIT 1`, order.ID).Scan(&appliedAddress, &appliedPlan); overlayErr == nil {
		if jsonErr := json.Unmarshal(appliedAddress, &order.Address); jsonErr != nil {
			return ParentOrder{}, jsonErr
		}
		order.FulfilmentPlanID = appliedPlan
	} else if !errors.Is(overlayErr, pgx.ErrNoRows) {
		return ParentOrder{}, overlayErr
	}

	if order.VendorOrders, err = loadVendorOrders(ctx, q, order.ID); err != nil {
		return ParentOrder{}, err
	}
	if order.PlatformAllocations, err = loadPlatformAllocations(ctx, q, order.ID); err != nil {
		return ParentOrder{}, err
	}
	return order, nil
}

// loadPlatformAllocations reads the rows that belong to no single vendor.
func loadPlatformAllocations(ctx context.Context, q queryer, parentID string) ([]Allocation, error) {
	return scanAllocations(ctx, q, `
		SELECT id,COALESCE(vendor_order_id,''),allocation_type,beneficiary_type,
		       beneficiary_ref,funding_source,source_ref,amount_minor
		  FROM commerce_order_allocations
		 WHERE parent_order_id=$1 AND vendor_order_id IS NULL
		 ORDER BY allocation_type,funding_source,id`, parentID)
}

type scanner interface{ Scan(...any) error }

func scanParent(row scanner) (ParentOrder, error) {
	var order ParentOrder
	var addressJSON, receiverJSON, fundingJSON, disclosureJSON []byte

	if err := row.Scan(
		&order.ID, &order.CustomerID, &order.QuoteID, &order.QuoteVersion, &order.QuoteFingerprint,
		&order.CheckoutSessionID, &order.CheckoutValidationRevision, &order.CartID, &order.CartVersion,
		&order.FulfilmentPlanID, &order.FulfilmentPlanRevision, &order.FulfilmentPlanVersion,
		&order.Domain, &order.Currency, &order.SelectedFulfilmentMode, &addressJSON,
		&order.HasReceiver, &receiverJSON,
		&order.CustomerApprovedTotalMinor, &order.MaxAuthorizedTotalMinor,
		&order.ProductSubtotalMinor, &order.DeliverySubtotalMinor, &order.FeeSubtotalMinor,
		&order.TaxSubtotalMinor, &order.DiscountTotalMinor, &order.SubsidyTotalMinor,
		&order.PlatformFeeTaxMinor, &fundingJSON, &disclosureJSON,
		&order.PaymentOperationRef, &order.State, &order.PaymentState, &order.VendorOrderCount,
		&order.RefundPreparedMinor, &order.SettlementPreparedMinor,
		&order.Version, &order.CreatedAt, &order.UpdatedAt, &order.TerminalAt,
	); err != nil {
		return ParentOrder{}, err
	}

	if err := json.Unmarshal(addressJSON, &order.Address); err != nil {
		return ParentOrder{}, err
	}
	if order.HasReceiver {
		if err := json.Unmarshal(receiverJSON, &order.Receiver); err != nil {
			return ParentOrder{}, err
		}
	}
	order.FundingBreakdown = json.RawMessage(fundingJSON)
	order.PriceDisclosureSnapshot = json.RawMessage(disclosureJSON)
	return order, nil
}

func loadVendorOrders(ctx context.Context, q queryer, parentID string) ([]VendorOrder, error) {
	rows, err := q.Query(ctx,
		`SELECT `+vendorColumns+` FROM commerce_vendor_orders v
		  WHERE v.parent_order_id=$1 ORDER BY v.quote_group_ref, v.id`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []VendorOrder
	for rows.Next() {
		order, scanErr := scanVendorOrder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		orders = append(orders, order)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for i := range orders {
		if orders[i].Lines, err = loadLines(ctx, q, orders[i].ID); err != nil {
			return nil, err
		}
		if orders[i].Allocations, err = loadAllocations(ctx, q, orders[i].ID); err != nil {
			return nil, err
		}
	}
	return orders, nil
}

func scanVendorOrder(row scanner) (VendorOrder, error) {
	var order VendorOrder
	err := row.Scan(
		&order.ID, &order.ParentOrderID, &order.CustomerID, &order.BusinessID, &order.BranchID,
		&order.Domain, &order.Currency, &order.QuoteGroupRef,
		&order.ProductSubtotalMinor, &order.MaxProductSubtotalMinor,
		&order.ProductVendorFundedMinor, &order.ProductPlatformFundedMinor, &order.ProductOtherFundedMinor,
		&order.OrderVendorFundedMinor, &order.OrderPlatformFundedMinor, &order.OrderOtherFundedMinor,
		&order.DeliveryResponsibility, &order.DeliveryFeeBeneficiary, &order.DeliveryGrossMinor,
		&order.DeliveryCustomerMinor, &order.DeliveryVendorFundedMinor, &order.DeliveryPlatformFundedMinor,
		&order.DeliveryOtherFundedMinor,
		&order.CustomerPayableMinor, &order.VendorPayableMinor,
		&order.ReservationRequired, &order.InventoryState, &order.ReservationRef, &order.ReservationSourceRef,
		&order.FulfilmentMode, &order.LogisticsPlanGroupRef, &order.LogisticsMissionRef,
		&order.VendorOwnDeliveryState, &order.Status, &order.CancelReasonCode, &order.CancelledBy,
		&order.RefundAllocatedMinor, &order.SettlementAllocatedMinor, &order.RefundState, &order.RefundOperationRef, &order.RefundRefundedMinor, &order.RejectionReason,
		&order.Version, &order.CreatedAt, &order.UpdatedAt, &order.TerminalAt,
	)
	return order, err
}

func loadLines(ctx context.Context, q queryer, vendorOrderID string) ([]OrderLine, error) {
	rows, err := q.Query(ctx, `
		SELECT id,quote_line_ref,cart_item_id,item_name,item_ref,variant_ref,listing_ref,
		       sell_unit_ref,quantity,unit_price_minor,quoted_total_minor,max_authorized_total_minor,
		       discount_allocated_minor,fee_allocated_minor,tax_allocated_minor,
		       pricing_basis,requested_measure_ref,expected_measure_quantity,
		       measure_underfill_bps,measure_overfill_bps,price_reference,catalogue_revision,
		       inventory_revision,inventory_item_ref,online_price_disclosure,
		       store_price_parity_claimed,delivery_commitment
		  FROM commerce_vendor_order_lines
		 WHERE vendor_order_id=$1 ORDER BY cart_item_id,id`, vendorOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []OrderLine
	for rows.Next() {
		var line OrderLine
		var commitment []byte
		if err = rows.Scan(
			&line.ID, &line.QuoteLineRef, &line.CartItemID, &line.ItemName, &line.ItemRef,
			&line.VariantRef, &line.ListingRef, &line.SellUnitRef, &line.Quantity,
			&line.UnitPriceMinor, &line.QuotedTotalMinor, &line.MaxAuthorizedTotalMinor,
			&line.DiscountAllocatedMinor, &line.FeeAllocatedMinor, &line.TaxAllocatedMinor,
			&line.PricingBasis, &line.RequestedMeasureRef, &line.ExpectedMeasureQuantity,
			&line.MeasureUnderfillBps, &line.MeasureOverfillBps, &line.PriceReference,
			&line.CatalogueRevision, &line.InventoryRevision, &line.InventoryItemRef,
			&line.OnlinePriceDisclosure, &line.StorePriceParityClaimed, &commitment,
		); err != nil {
			return nil, err
		}
		line.DeliveryCommitment = json.RawMessage(commitment)
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadAllocations(ctx context.Context, q queryer, vendorOrderID string) ([]Allocation, error) {
	return scanAllocations(ctx, q, `
		SELECT id,COALESCE(vendor_order_id,''),allocation_type,beneficiary_type,
		       beneficiary_ref,funding_source,source_ref,amount_minor
		  FROM commerce_order_allocations
		 WHERE vendor_order_id=$1 ORDER BY allocation_type,funding_source,id`, vendorOrderID)
}

func scanAllocations(ctx context.Context, q queryer, query, arg string) ([]Allocation, error) {
	rows, err := q.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allocations []Allocation
	for rows.Next() {
		var allocation Allocation
		if err = rows.Scan(&allocation.ID, &allocation.VendorOrderID, &allocation.AllocationType,
			&allocation.BeneficiaryType, &allocation.BeneficiaryRef, &allocation.FundingSource,
			&allocation.SourceRef, &allocation.AmountMinor); err != nil {
			return nil, err
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}

// GetVendorOrderForCustomer reads one vendor allocation of the customer's own
// purchase.
func (s *Store) GetVendorOrderForCustomer(
	ctx context.Context, customerID, vendorOrderID string,
) (VendorOrder, error) {
	if s == nil || s.db == nil {
		return VendorOrder{}, ErrDependencyMissing
	}
	return loadVendorOrderBy(ctx, s.db,
		`v.id=$1 AND v.customer_id=$2`, vendorOrderID, customerID)
}

// GetVendorOrderForVendor reads one vendor allocation belonging to the acting
// vendor. Tenant scoping is in the WHERE clause, so a foreign order is a
// not-found rather than a filtered-out row.
func (s *Store) GetVendorOrderForVendor(
	ctx context.Context, businessID, vendorOrderID string,
) (VendorOrder, error) {
	if s == nil || s.db == nil {
		return VendorOrder{}, ErrDependencyMissing
	}
	return loadVendorOrderBy(ctx, s.db,
		`v.id=$1 AND v.business_id=$2`, vendorOrderID, businessID)
}

func loadVendorOrderBy(
	ctx context.Context, q queryer, where string, args ...any,
) (VendorOrder, error) {
	order, err := scanVendorOrder(q.QueryRow(ctx,
		`SELECT `+vendorColumns+` FROM commerce_vendor_orders v WHERE `+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return VendorOrder{}, ErrOrderNotFound
	}
	if err != nil {
		return VendorOrder{}, err
	}
	if order.Lines, err = loadLines(ctx, q, order.ID); err != nil {
		return VendorOrder{}, err
	}
	if order.Allocations, err = loadAllocations(ctx, q, order.ID); err != nil {
		return VendorOrder{}, err
	}
	return order, nil
}

// VendorQueueFilter narrows a vendor's own order queue.
type VendorQueueFilter struct {
	BranchID string
	Status   VendorStatus
	Limit    int
}

// ListVendorOrders reads the acting vendor's queue within its authorised
// branch scope.
func (s *Store) ListVendorOrders(
	ctx context.Context, businessID string, scope Scope, filter VendorQueueFilter,
) ([]VendorOrder, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}

	conditions := []string{"v.business_id=$1"}
	args := []any{businessID}

	if !scope.Global {
		if len(scope.BranchIDs) == 0 {
			return nil, ErrForbidden
		}
		args = append(args, scope.BranchIDs)
		conditions = append(conditions, fmt.Sprintf("v.branch_id = ANY($%d)", len(args)))
	}
	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		conditions = append(conditions, fmt.Sprintf("v.branch_id=$%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("v.status=$%d", len(args)))
	}
	args = append(args, filter.Limit)

	rows, err := s.db.Query(ctx,
		`SELECT `+vendorColumns+` FROM commerce_vendor_orders v WHERE `+
			strings.Join(conditions, " AND ")+
			fmt.Sprintf(` ORDER BY v.created_at DESC, v.id LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []VendorOrder
	for rows.Next() {
		order, scanErr := scanVendorOrder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

// -----------------------------------------------------------------------------
// Transitions
// -----------------------------------------------------------------------------

// TransitionRequest is one lifecycle command against one VendorOrder.
type TransitionRequest struct {
	VendorOrderID   string
	ExpectedVersion int64
	Transition      Transition
	CancelledBy     CancelActor
	ReasonCode      string

	ActorType string
	ActorRef  string

	// RequireCustomerCancellable re-checks the customer withdrawal window
	// INSIDE the transaction. Checking it only before the lock would let a
	// vendor's concurrent handover slip past between the read and the write.
	RequireCustomerCancellable bool

	// BusinessID scopes a vendor staff command. CustomerID scopes a customer
	// command. Exactly one is set, and the scope is part of the UPDATE guard
	// so a foreign order can never be transitioned.
	BusinessID string
	CustomerID string
}

// Transition applies one lifecycle command and re-derives the parent state.
//
// The parent row is locked first so concurrent transitions on sibling vendor
// orders serialize on the aggregate they both update, and the VendorOrder
// UPDATE carries a version guard so a stale client cannot overwrite a change
// it never saw.
func (s *Store) Transition(
	ctx context.Context, request TransitionRequest, now time.Time,
) (VendorOrder, ParentOrder, error) {
	if s == nil || s.db == nil {
		return VendorOrder{}, ParentOrder{}, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scopeClause := "v.id=$1"
	args := []any{request.VendorOrderID}
	if request.BusinessID != "" {
		args = append(args, request.BusinessID)
		scopeClause += " AND v.business_id=$2"
	} else {
		args = append(args, request.CustomerID)
		scopeClause += " AND v.customer_id=$2"
	}

	var parentID string
	if err = tx.QueryRow(ctx,
		`SELECT v.parent_order_id FROM commerce_vendor_orders v WHERE `+scopeClause,
		args...).Scan(&parentID); errors.Is(err, pgx.ErrNoRows) {
		return VendorOrder{}, ParentOrder{}, ErrOrderNotFound
	} else if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}

	// Locking the parent first is what serializes concurrent transitions on
	// sibling vendor orders: they both have to re-derive the same aggregate.
	var customerID, previousState string
	var platformFeeTax int64
	if err = tx.QueryRow(ctx,
		`SELECT customer_id,state,platform_fee_tax_minor
		   FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`,
		parentID).Scan(&customerID, &previousState, &platformFeeTax); err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}

	current, err := loadVendorOrderBy(ctx, tx, scopeClause, args...)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if request.ExpectedVersion != current.Version {
		return VendorOrder{}, ParentOrder{}, ErrVersionConflict
	}
	if request.RequireCustomerCancellable && !CancellableByCustomer(current.Status) {
		return VendorOrder{}, ParentOrder{}, ErrOrderConflict
	}

	outcome, err := ApplyTransition(current, request.Transition, request.CancelledBy)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if request.Transition == TransitionPack {
		var pending int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM commerce_substitution_proposals WHERE vendor_order_id=$1 AND status='PENDING' AND (expires_at IS NULL OR expires_at>$2)`, current.ID, now).Scan(&pending); err != nil {
			return VendorOrder{}, ParentOrder{}, err
		}
		if pending > 0 {
			return VendorOrder{}, ParentOrder{}, ErrOrderConflict
		}
	}

	var terminalAt any
	cancelledBy := ""
	reason := ""
	rejectionReason := ""
	if outcome.Terminal {
		terminalAt = now
	}
	if outcome.Status == VendorStatusCancelled {
		cancelledBy = string(request.CancelledBy)
		reason = request.ReasonCode
	}
	if outcome.Status == VendorStatusRejected {
		rejectionReason = request.ReasonCode
		if rejectionReason == "" {
			return VendorOrder{}, ParentOrder{}, ErrOrderConflict
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE commerce_vendor_orders
		   SET status=$1,vendor_own_delivery_state=$2,terminal_at=$3::timestamptz,
		       cancelled_by=$4,cancel_reason_code=$5,rejection_reason=$6,
		       refund_allocated_minor=$7,settlement_allocated_minor=$8,
		       version=version+1,updated_at=$9
		 WHERE id=$10 AND version=$11`,
		outcome.Status, outcome.VendorOwnDelivery, terminalAt,
		cancelledBy, reason, rejectionReason,
		outcome.RefundAllocatedMinor, outcome.SettlementAllocatedMinor,
		now, current.ID, current.Version)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if tag.RowsAffected() != 1 {
		return VendorOrder{}, ParentOrder{}, ErrVersionConflict
	}

	if err = appendEvent(ctx, tx, s.outbox, parentID, current.ID, customerID,
		outcome.Action, request.ActorType, request.ActorRef,
		string(current.Status), string(outcome.Status), now); err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}

	parent, err := refreshParentState(
		ctx, tx, parentID, customerID, previousState, platformFeeTax, now,
	)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}

	updated, err := loadVendorOrderBy(ctx, tx, `v.id=$1`, current.ID)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	return updated, parent, nil
}

// refreshParentState re-derives the aggregate from its children.
//
// The parent state is never written from a caller's opinion; it is recomputed
// from every VendorOrder every time one of them moves.
func refreshParentState(
	ctx context.Context,
	tx pgx.Tx,
	parentID, customerID, previousState string,
	platformFeeTaxMinor int64,
	now time.Time,
) (ParentOrder, error) {
	// Only the children's status and prepared amounts matter here, so this
	// deliberately does not reload every line and allocation of every sibling
	// on each transition.
	rows, err := tx.Query(ctx, `
		SELECT status,refund_allocated_minor,settlement_allocated_minor
		  FROM commerce_vendor_orders WHERE parent_order_id=$1`, parentID)
	if err != nil {
		return ParentOrder{}, err
	}
	var children []VendorOrder
	for rows.Next() {
		var child VendorOrder
		if err = rows.Scan(&child.Status, &child.RefundAllocatedMinor,
			&child.SettlementAllocatedMinor); err != nil {
			rows.Close()
			return ParentOrder{}, err
		}
		children = append(children, child)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return ParentOrder{}, err
	}

	statuses := make([]VendorStatus, 0, len(children))
	for _, child := range children {
		statuses = append(statuses, child.Status)
	}
	state := DeriveParentState(statuses)
	refund, settlement := DeriveParentPreparedAmounts(children, platformFeeTaxMinor)

	var terminalAt any
	if TerminalParentState(state) {
		terminalAt = now
	}

	if _, err = tx.Exec(ctx, `
		UPDATE commerce_parent_orders
		   SET state=$1,refund_prepared_minor=$2,settlement_prepared_minor=$3,
		       terminal_at=CASE WHEN $1 IN ('COMPLETED','PARTIALLY_COMPLETED','CANCELLED','REFUNDED','PARTIALLY_REFUNDED')
		                         THEN COALESCE(terminal_at,$4::timestamptz)
		                         ELSE NULL END,
		       version=version+1,updated_at=$5
		 WHERE id=$6`,
		state, refund, settlement, terminalAt, now, parentID); err != nil {
		return ParentOrder{}, err
	}

	if previousState != string(state) {
		if err = appendEvent(ctx, tx, nil, parentID, "", customerID,
			ActionOrderStateChanged, "SYSTEM", "", previousState, string(state), now); err != nil {
			return ParentOrder{}, err
		}
	}

	return loadParent(ctx, tx, customerID, parentID)
}
