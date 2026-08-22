package purchasing

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Purchasing operational read models.
//
// Every figure here is a bounded, indexed query against the 15A/15B/15C
// tables. No projection table is maintained, because a projection would add a
// second copy of purchasing truth that could drift from the record, and the
// query plans certified in 15D show indexed reads are sufficient. If a future
// vendor outgrows that, a projection can be added behind this same interface
// without changing any caller.

// PurchaseOrderCounts is the order pipeline by state.
type PurchaseOrderCounts struct {
	Draft             int64 `json:"draft"`
	Submitted         int64 `json:"submitted"`
	Approved          int64 `json:"approved"`
	Ordered           int64 `json:"ordered"`
	PartiallyReceived int64 `json:"partially_received"`
	Received          int64 `json:"received"`
	Closed            int64 `json:"closed"`
	Rejected          int64 `json:"rejected"`
	Cancelled         int64 `json:"cancelled"`
}

// DeliveryCounts describes outstanding delivery pressure.
type DeliveryCounts struct {
	Overdue             int64 `json:"overdue"`
	MateriallyOverdue   int64 `json:"materially_overdue"`
	ExpectedToday       int64 `json:"expected_today"`
	ExpectedSoon        int64 `json:"expected_soon"`
	OutstandingNoDate   int64 `json:"outstanding_without_expected_date"`
	ReceivedLast7Days   int64 `json:"received_last_7_days"`
	AwaitingApprovalNow int64 `json:"awaiting_approval"`
}

// ReceivingCounts describes the receiving desk.
type ReceivingCounts struct {
	ReceiptsToday       int64 `json:"receipts_today"`
	AwaitingInspection  int64 `json:"awaiting_inspection"`
	InspectedNotPosted  int64 `json:"inspected_not_posted"`
	PostedLast7Days     int64 `json:"posted_last_7_days"`
	ReversedLast30Days  int64 `json:"reversed_last_30_days"`
	TotalPostedReceipts int64 `json:"total_posted_receipts"`
}

// DiscrepancyCounts describes outstanding supplier problems.
type DiscrepancyCounts struct {
	Open             int64  `json:"open"`
	ApprovalRequired int64  `json:"approval_required"`
	Unresolved       int64  `json:"unresolved"`
	ShortDelivery    int64  `json:"short_delivery"`
	OverDelivery     int64  `json:"over_delivery"`
	Damaged          int64  `json:"damaged"`
	Rejected         int64  `json:"rejected"`
	WrongItem        int64  `json:"wrong_item"`
	QualityIssue     int64  `json:"quality_issue"`
	CostVariance     int64  `json:"cost_variance"`
	OldestOpenAgeHrs *int64 `json:"oldest_open_age_hours"`
}

// SupplierCounts describes the supplier base.
type SupplierCounts struct {
	Active                 int64 `json:"active"`
	Archived               int64 `json:"archived"`
	WithOverdueDeliveries  int64 `json:"with_overdue_deliveries"`
	WithUnresolvedIssues   int64 `json:"with_unresolved_discrepancies"`
	WithRecentCostVariance int64 `json:"with_recent_cost_variance"`
	UsedInLast30Days       int64 `json:"used_last_30_days"`
}

// PurchasingDashboard is the whole operational picture for one scope.
type PurchasingDashboard struct {
	BusinessID string   `json:"business_id"`
	BranchIDs  []string `json:"branch_ids"`
	Scope      string   `json:"scope"`

	PurchaseOrders PurchaseOrderCounts `json:"purchase_orders"`
	Deliveries     DeliveryCounts      `json:"deliveries"`
	Receiving      ReceivingCounts     `json:"receiving"`
	Discrepancies  DiscrepancyCounts   `json:"discrepancies"`
	Suppliers      SupplierCounts      `json:"suppliers"`

	GeneratedAt time.Time `json:"generated_at"`
}

// materiallyOverdueDays is the threshold at which "late" becomes "escalate".
const materiallyOverdueDays = 7

// expectedSoonDays is the look-ahead window for upcoming deliveries.
const expectedSoonDays = 3

// PurchasingDashboard builds the operational read model for the caller's scope.
func (s *GoodsReceivingService) PurchasingDashboard(
	ctx context.Context,
	actor Actor,
	scope Scope,
	branchFilter []string,
) (PurchasingDashboard, error) {
	if s == nil || s.db == nil || !actor.Validate() || !scope.HasAny() {
		return PurchasingDashboard{}, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil)
	}

	branches, restricted, err := resolvePerformanceBranches(scope, branchFilter)
	if err != nil {
		return PurchasingDashboard{}, err
	}

	out := PurchasingDashboard{
		BusinessID:  actor.VendorID,
		BranchIDs:   branches,
		GeneratedAt: time.Now().UTC(),
	}
	if restricted {
		out.Scope = "BRANCH"
	} else {
		out.Scope = "BUSINESS"
	}

	if err := s.dashboardOrders(ctx, actor.VendorID, branches, restricted, &out); err != nil {
		return PurchasingDashboard{}, err
	}
	if err := s.dashboardReceiving(ctx, actor.VendorID, branches, restricted, &out); err != nil {
		return PurchasingDashboard{}, err
	}
	if err := s.dashboardDiscrepancies(ctx, actor.VendorID, branches, restricted, &out); err != nil {
		return PurchasingDashboard{}, err
	}
	if err := s.dashboardSuppliers(ctx, actor.VendorID, branches, restricted, &out); err != nil {
		return PurchasingDashboard{}, err
	}

	return out, nil
}

func (s *GoodsReceivingService) dashboardOrders(
	ctx context.Context,
	businessID string,
	branches []string,
	restricted bool,
	out *PurchasingDashboard,
) error {
	// One pass over the tenant's orders produces every status count and every
	// delivery-pressure figure. Splitting these into separate round trips would
	// be the N+1 shape this dashboard exists to avoid.
	return s.db.QueryRow(ctx,
		`SELECT
		    count(*) FILTER (WHERE status = 'DRAFT'),
		    count(*) FILTER (WHERE status = 'SUBMITTED'),
		    count(*) FILTER (WHERE status = 'APPROVED'),
		    count(*) FILTER (WHERE status = 'ORDERED'),
		    count(*) FILTER (WHERE status = 'PARTIALLY_RECEIVED'),
		    count(*) FILTER (WHERE status = 'RECEIVED'),
		    count(*) FILTER (WHERE status = 'CLOSED'),
		    count(*) FILTER (WHERE status = 'REJECTED'),
		    count(*) FILTER (WHERE status = 'CANCELLED'),

		    -- Overdue applies only to genuinely outstanding delivery. A DRAFT,
		    -- REJECTED, CANCELLED, RECEIVED or CLOSED order is never overdue.
		    count(*) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date IS NOT NULL
		          AND expected_delivery_date < current_date
		    ),
		    count(*) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date IS NOT NULL
		          AND expected_delivery_date < current_date - ($5::int)
		    ),
		    count(*) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date = current_date
		    ),
		    count(*) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date > current_date
		          AND expected_delivery_date <= current_date + ($6::int)
		    ),
		    count(*) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date IS NULL
		    ),
		    count(*) FILTER (
		        WHERE status IN ('RECEIVED','CLOSED')
		          AND received_at >= now() - interval '7 days'
		    ),
		    count(*) FILTER (WHERE status = 'SUBMITTED')
		   FROM vendor_purchase_orders
		  WHERE business_id = $1
		    AND (NOT $2::boolean OR branch_id = ANY($3::text[]))
		    AND $4::boolean`,
		businessID, restricted, branches, true,
		materiallyOverdueDays, expectedSoonDays,
	).Scan(
		&out.PurchaseOrders.Draft,
		&out.PurchaseOrders.Submitted,
		&out.PurchaseOrders.Approved,
		&out.PurchaseOrders.Ordered,
		&out.PurchaseOrders.PartiallyReceived,
		&out.PurchaseOrders.Received,
		&out.PurchaseOrders.Closed,
		&out.PurchaseOrders.Rejected,
		&out.PurchaseOrders.Cancelled,
		&out.Deliveries.Overdue,
		&out.Deliveries.MateriallyOverdue,
		&out.Deliveries.ExpectedToday,
		&out.Deliveries.ExpectedSoon,
		&out.Deliveries.OutstandingNoDate,
		&out.Deliveries.ReceivedLast7Days,
		&out.Deliveries.AwaitingApprovalNow,
	)
}

func (s *GoodsReceivingService) dashboardReceiving(
	ctx context.Context,
	businessID string,
	branches []string,
	restricted bool,
	out *PurchasingDashboard,
) error {
	return s.db.QueryRow(ctx,
		`SELECT
		    count(*) FILTER (WHERE received_at >= date_trunc('day', now())),
		    count(*) FILTER (WHERE status = 'DRAFT'),
		    count(*) FILTER (WHERE status = 'INSPECTED'),
		    count(*) FILTER (
		        WHERE status = 'POSTED' AND posted_at >= now() - interval '7 days'
		    ),
		    count(*) FILTER (
		        WHERE status = 'REVERSED' AND reversed_at >= now() - interval '30 days'
		    ),
		    count(*) FILTER (WHERE status = 'POSTED')
		   FROM vendor_goods_receipts
		  WHERE business_id = $1
		    AND (NOT $2::boolean OR branch_id = ANY($3::text[]))`,
		businessID, restricted, branches,
	).Scan(
		&out.Receiving.ReceiptsToday,
		&out.Receiving.AwaitingInspection,
		&out.Receiving.InspectedNotPosted,
		&out.Receiving.PostedLast7Days,
		&out.Receiving.ReversedLast30Days,
		&out.Receiving.TotalPostedReceipts,
	)
}

func (s *GoodsReceivingService) dashboardDiscrepancies(
	ctx context.Context,
	businessID string,
	branches []string,
	restricted bool,
	out *PurchasingDashboard,
) error {
	var oldestHours *float64

	if err := s.db.QueryRow(ctx,
		`SELECT
		    count(*) FILTER (WHERE status = 'OPEN'),
		    count(*) FILTER (WHERE status = 'APPROVAL_REQUIRED'),
		    count(*) FILTER (WHERE status IN ('OPEN','APPROVAL_REQUIRED')),
		    count(*) FILTER (WHERE discrepancy_type = 'SHORT_DELIVERY'),
		    count(*) FILTER (WHERE discrepancy_type = 'OVER_DELIVERY'),
		    count(*) FILTER (WHERE discrepancy_type = 'DAMAGED'),
		    count(*) FILTER (WHERE discrepancy_type = 'REJECTED'),
		    count(*) FILTER (WHERE discrepancy_type = 'WRONG_ITEM'),
		    count(*) FILTER (WHERE discrepancy_type = 'QUALITY_ISSUE'),
		    count(*) FILTER (WHERE discrepancy_type = 'COST_VARIANCE'),
		    max(EXTRACT(EPOCH FROM (now() - created_at)) / 3600.0)
		        FILTER (WHERE status IN ('OPEN','APPROVAL_REQUIRED'))
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1
		    AND (NOT $2::boolean OR branch_id = ANY($3::text[]))`,
		businessID, restricted, branches,
	).Scan(
		&out.Discrepancies.Open,
		&out.Discrepancies.ApprovalRequired,
		&out.Discrepancies.Unresolved,
		&out.Discrepancies.ShortDelivery,
		&out.Discrepancies.OverDelivery,
		&out.Discrepancies.Damaged,
		&out.Discrepancies.Rejected,
		&out.Discrepancies.WrongItem,
		&out.Discrepancies.QualityIssue,
		&out.Discrepancies.CostVariance,
		&oldestHours,
	); err != nil {
		return err
	}

	// Absent rather than zero: no open discrepancy means there is no age, not
	// an age of zero hours.
	if oldestHours != nil {
		hours := int64(*oldestHours)
		out.Discrepancies.OldestOpenAgeHrs = &hours
	}

	return nil
}

func (s *GoodsReceivingService) dashboardSuppliers(
	ctx context.Context,
	businessID string,
	branches []string,
	restricted bool,
	out *PurchasingDashboard,
) error {
	if err := s.db.QueryRow(ctx,
		`SELECT
		    count(*) FILTER (WHERE status = 'ACTIVE'),
		    count(*) FILTER (WHERE status = 'ARCHIVED')
		   FROM vendor_suppliers
		  WHERE business_id = $1`,
		businessID,
	).Scan(&out.Suppliers.Active, &out.Suppliers.Archived); err != nil {
		return err
	}

	// Distinct-supplier rollups. Each is a single grouped scan over an indexed
	// tenant slice rather than a per-supplier query.
	if err := s.db.QueryRow(ctx,
		`SELECT
		    count(DISTINCT supplier_id) FILTER (
		        WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')
		          AND expected_delivery_date IS NOT NULL
		          AND expected_delivery_date < current_date
		    ),
		    count(DISTINCT supplier_id) FILTER (
		        WHERE created_at >= now() - interval '30 days'
		    )
		   FROM vendor_purchase_orders
		  WHERE business_id = $1
		    AND (NOT $2::boolean OR branch_id = ANY($3::text[]))`,
		businessID, restricted, branches,
	).Scan(&out.Suppliers.WithOverdueDeliveries, &out.Suppliers.UsedInLast30Days); err != nil {
		return err
	}

	return s.db.QueryRow(ctx,
		`SELECT
		    count(DISTINCT supplier_id) FILTER (
		        WHERE status IN ('OPEN','APPROVAL_REQUIRED')
		    ),
		    count(DISTINCT supplier_id) FILTER (
		        WHERE discrepancy_type = 'COST_VARIANCE'
		          AND created_at >= now() - interval '30 days'
		    )
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1
		    AND (NOT $2::boolean OR branch_id = ANY($3::text[]))`,
		businessID, restricted, branches,
	).Scan(&out.Suppliers.WithUnresolvedIssues, &out.Suppliers.WithRecentCostVariance)
}

// OutstandingDelivery is one purchase order with delivery still owed.
type OutstandingDelivery struct {
	PurchaseOrderID     string     `json:"purchase_order_id"`
	PurchaseOrderNumber string     `json:"purchase_order_number"`
	BranchID            string     `json:"branch_id"`
	SupplierID          string     `json:"supplier_id"`
	SupplierName        string     `json:"supplier_name"`
	Status              string     `json:"status"`
	ExpectedDelivery    *time.Time `json:"expected_delivery_date"`
	DaysOverdue         int64      `json:"days_overdue"`
	DaysUntilDue        int64      `json:"days_until_due"`
	TotalAmountMinor    int64      `json:"total_amount_minor"`
	Currency            string     `json:"currency_code"`
	OrderedAt           *time.Time `json:"ordered_at"`
}

// DeliveryWindow selects which outstanding orders to return.
type DeliveryWindow string

const (
	// DeliveryOverdue is expected date already passed and delivery still owed.
	DeliveryOverdue DeliveryWindow = "OVERDUE"

	// DeliveryUpcoming is due today or within the look-ahead window.
	DeliveryUpcoming DeliveryWindow = "UPCOMING"
)

// ListOutstandingDeliveries returns overdue or upcoming purchase orders.
//
// Only ORDERED and PARTIALLY_RECEIVED qualify: a fully RECEIVED order has no
// delivery outstanding and must never appear in an overdue view, and a DRAFT
// or CANCELLED order was never promised.
func (s *GoodsReceivingService) ListOutstandingDeliveries(
	ctx context.Context,
	actor Actor,
	scope Scope,
	window DeliveryWindow,
	branchFilter []string,
	limit int,
) ([]OutstandingDelivery, error) {
	if s == nil || s.db == nil || !actor.Validate() || !scope.HasAny() {
		return nil, apperrors.Forbidden("Purchasing access is not permitted.", nil)
	}

	branches, restricted, err := resolvePerformanceBranches(scope, branchFilter)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	overdue := window != DeliveryUpcoming

	rows, err := s.db.Query(ctx,
		`SELECT
		    o.id,
		    o.purchase_order_number,
		    o.branch_id,
		    o.supplier_id,
		    o.supplier_name_snapshot,
		    o.status,
		    o.expected_delivery_date,
		    GREATEST(current_date - o.expected_delivery_date, 0),
		    GREATEST(o.expected_delivery_date - current_date, 0),
		    o.total_amount_minor,
		    o.currency_code,
		    o.ordered_at
		   FROM vendor_purchase_orders o
		  WHERE o.business_id = $1
		    AND o.status IN ('ORDERED','PARTIALLY_RECEIVED')
		    AND o.expected_delivery_date IS NOT NULL
		    AND (NOT $2::boolean OR o.branch_id = ANY($3::text[]))
		    AND (
		        ($4::boolean AND o.expected_delivery_date < current_date)
		        OR
		        (NOT $4::boolean
		         AND o.expected_delivery_date >= current_date
		         AND o.expected_delivery_date <= current_date + ($5::int))
		    )
		  ORDER BY o.expected_delivery_date ASC, o.id ASC
		  LIMIT $6`,
		actor.VendorID, restricted, branches, overdue, expectedSoonDays, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]OutstandingDelivery, 0, limit)

	for rows.Next() {
		var item OutstandingDelivery
		if err := rows.Scan(
			&item.PurchaseOrderID,
			&item.PurchaseOrderNumber,
			&item.BranchID,
			&item.SupplierID,
			&item.SupplierName,
			&item.Status,
			&item.ExpectedDelivery,
			&item.DaysOverdue,
			&item.DaysUntilDue,
			&item.TotalAmountMinor,
			&item.Currency,
			&item.OrderedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

// normalizeDeliveryWindow maps a request string onto the supported windows.
func normalizeDeliveryWindow(raw string) DeliveryWindow {
	if strings.EqualFold(strings.TrimSpace(raw), string(DeliveryUpcoming)) {
		return DeliveryUpcoming
	}
	return DeliveryOverdue
}
