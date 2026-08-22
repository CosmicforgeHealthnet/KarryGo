package purchasing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Supplier performance is derived from purchasing evidence at read time. There
// is no score, and there is no projection table: every figure below is a fact
// the 15A/15B/15C tables already record.
//
// The hard rule here is that absence of evidence is never reported as good
// performance. A supplier nobody has ever received from does not have a 100%
// on-time rate; it has no on-time rate. Each ratio therefore carries its own
// denominator and status, so a caller can tell "perfect" apart from "unknown".

const (
	// MetricOK means the ratio was computed from a real denominator.
	MetricOK = "OK"

	// MetricInsufficientHistory means there is no denominator yet. The value
	// is null and must not be rendered as zero or as a perfect score.
	MetricInsufficientHistory = "INSUFFICIENT_HISTORY"
)

// minimumRateSample is the smallest denominator this service will compute a
// rate from. One delivery is an anecdote, not a delivery record, and a 0%
// or 100% rate from a single observation reads as far more certain than it is.
const minimumRateSample = 3

// Ratio is a rate that knows whether it is allowed to exist.
//
// Value is nil whenever Status is not MetricOK. Callers that ignore Status and
// coalesce Value to zero will be wrong, which is why Value is a pointer rather
// than a float with a sentinel.
type Ratio struct {
	Value       *float64 `json:"value"`
	Status      string   `json:"metric_status"`
	SampleCount int64    `json:"sample_count"`
}

func ratio(numerator, denominator int64) Ratio {
	if denominator < minimumRateSample {
		return Ratio{Status: MetricInsufficientHistory, SampleCount: denominator}
	}
	value := float64(numerator) / float64(denominator)
	return Ratio{Value: &value, Status: MetricOK, SampleCount: denominator}
}

// Duration in whole hours, absent when never observed.
type AverageHours struct {
	Value       *float64 `json:"value_hours"`
	Status      string   `json:"metric_status"`
	SampleCount int64    `json:"sample_count"`
}

// MoneyAverage is an average of minor-unit amounts. It stays an integer
// average in minor units so no float ever becomes commercial evidence.
type MoneyAverage struct {
	Value       *int64 `json:"value_minor"`
	Status      string `json:"metric_status"`
	SampleCount int64  `json:"sample_count"`
}

// SupplierPerformance is the full evidence-based summary for one supplier.
type SupplierPerformance struct {
	BusinessID string `json:"business_id"`
	SupplierID string `json:"supplier_id"`

	// The window the evidence was drawn from. Both are nil when the supplier
	// has no purchasing history at all.
	ObservedFrom *time.Time `json:"observed_from"`
	ObservedTo   *time.Time `json:"observed_to"`

	// Order activity.
	TotalPurchaseOrders     int64      `json:"total_purchase_orders"`
	TotalOrderedLines       int64      `json:"total_ordered_lines"`
	OrdersOutstanding       int64      `json:"orders_outstanding"`
	OrdersFullyReceived     int64      `json:"orders_fully_received"`
	OrdersCancelled         int64      `json:"orders_cancelled"`
	LastOrderedAt           *time.Time `json:"last_ordered_at"`
	LastDeliveredAt         *time.Time `json:"last_delivered_at"`
	TotalGoodsReceipts      int64      `json:"total_goods_receipts"`
	TotalPostedGoodsReceipt int64      `json:"total_posted_goods_receipts"`

	// Delivery timeliness. OnTimeRate has a denominator only once deliveries
	// with a stated expected date exist.
	OnTimeDeliveries      int64        `json:"on_time_deliveries"`
	LateDeliveries        int64        `json:"late_deliveries"`
	OnTimeRate            Ratio        `json:"on_time_rate"`
	AverageActualLeadTime AverageHours `json:"average_actual_lead_time"`

	// Quantity outcomes, in inventory units.
	TotalOrderedQuantity  int64 `json:"total_ordered_quantity"`
	TotalDeliveredQuantiy int64 `json:"total_delivered_quantity"`
	TotalAcceptedQuantity int64 `json:"total_accepted_quantity"`
	TotalShortQuantity    int64 `json:"total_short_quantity"`
	TotalDamagedQuantity  int64 `json:"total_damaged_quantity"`
	TotalRejectedQuantity int64 `json:"total_rejected_quantity"`

	ShortDeliveryLines int64 `json:"short_delivery_lines"`
	DamagedLines       int64 `json:"damaged_lines"`
	RejectedLines      int64 `json:"rejected_lines"`
	ReceiptLineCount   int64 `json:"receipt_line_count"`

	// Rates over receipt lines. Denominator is receipt lines, never orders,
	// because a line is the unit an inspection actually judges.
	ShortDeliveryRate Ratio `json:"short_delivery_rate"`
	DamageRate        Ratio `json:"damage_rate"`
	RejectionRate     Ratio `json:"rejection_rate"`
	AcceptanceRate    Ratio `json:"acceptance_rate"`

	// Discrepancies.
	WrongItemDiscrepancies    int64 `json:"wrong_item_discrepancies"`
	QualityDiscrepancies      int64 `json:"quality_discrepancies"`
	CostVarianceDiscrepancies int64 `json:"cost_variance_discrepancies"`
	OpenDiscrepancies         int64 `json:"open_discrepancies"`
	UnresolvedDiscrepancies   int64 `json:"unresolved_discrepancies"`
	TotalDiscrepancies        int64 `json:"total_discrepancies"`

	// Cost behaviour.
	CostObservations      int64        `json:"cost_observations"`
	CostVarianceRate      Ratio        `json:"cost_variance_rate"`
	AverageCostVariance   MoneyAverage `json:"average_cost_variance"`
	LatestObservedCost    *int64       `json:"latest_observed_unit_cost_minor"`
	LatestObservedCostAt  *time.Time   `json:"latest_observed_cost_at"`
	LatestObservedCostCcy string       `json:"latest_observed_currency"`

	// Raw lead-time accumulators. Unexported so they never serialize: the
	// caller-facing figure is AverageActualLeadTime, which carries its own
	// status and cannot be mistaken for a real average when unobserved.
	leadTimeHours   float64
	leadTimeSamples int64
}

// SupplierPerformanceQuery narrows the evidence window.
type SupplierPerformanceQuery struct {
	// BranchIDs restricts evidence to those branches. Empty means every branch
	// the caller's scope already allows, which the service applies itself.
	BranchIDs []string

	From *time.Time
	To   *time.Time
}

// SupplierPerformance computes the summary for one supplier.
//
// The supplier must belong to the actor's business, and the branch filter is
// intersected with the caller's scope before any evidence is read, so a
// branch-scoped caller can never widen the window by asking for a branch they
// do not hold.
func (s *GoodsReceivingService) SupplierPerformance(
	ctx context.Context,
	actor Actor,
	scope Scope,
	supplierID string,
	query SupplierPerformanceQuery,
) (SupplierPerformance, error) {
	if s == nil || s.db == nil {
		return SupplierPerformance{}, errors.New("purchasing: service unavailable")
	}
	if !actor.Validate() {
		return SupplierPerformance{}, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil)
	}
	if !scope.HasAny() {
		return SupplierPerformance{}, apperrors.Forbidden(
			"Purchasing access is not permitted.", nil)
	}

	supplierID = strings.TrimSpace(supplierID)
	if supplierID == "" {
		return SupplierPerformance{}, notFound("Supplier")
	}

	branches, restricted, err := resolvePerformanceBranches(scope, query.BranchIDs)
	if err != nil {
		return SupplierPerformance{}, err
	}

	// A supplier belonging to another business is indistinguishable from one
	// that does not exist.
	var exists bool
	if err := s.db.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM vendor_suppliers
		     WHERE id = $1 AND business_id = $2
		 )`, supplierID, actor.VendorID).Scan(&exists); err != nil {
		return SupplierPerformance{}, err
	}
	if !exists {
		return SupplierPerformance{}, notFound("Supplier")
	}

	out := SupplierPerformance{
		BusinessID: actor.VendorID,
		SupplierID: supplierID,
	}

	if err := s.loadOrderEvidence(ctx, actor.VendorID, supplierID, branches, restricted, query, &out); err != nil {
		return SupplierPerformance{}, err
	}
	if err := s.loadReceiptEvidence(ctx, actor.VendorID, supplierID, branches, restricted, query, &out); err != nil {
		return SupplierPerformance{}, err
	}
	if err := s.loadDiscrepancyEvidence(ctx, actor.VendorID, supplierID, branches, restricted, query, &out); err != nil {
		return SupplierPerformance{}, err
	}
	if err := s.loadCostEvidence(ctx, actor.VendorID, supplierID, branches, restricted, query, &out); err != nil {
		return SupplierPerformance{}, err
	}

	out.finalizeRates()

	return out, nil
}

// resolvePerformanceBranches intersects a requested branch filter with the
// caller's scope. A global scope with no filter reads every branch; anything
// else is an explicit allow-list.
func resolvePerformanceBranches(
	scope Scope,
	requested []string,
) ([]string, bool, error) {
	clean := make([]string, 0, len(requested))
	for _, branch := range requested {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		if !scope.Covers(branch) {
			// Asking for a branch outside the caller's scope is refused rather
			// than silently narrowed, so a caller cannot probe which branches
			// exist by watching totals change.
			return nil, false, apperrors.Forbidden(
				"That branch is outside your purchasing scope.", nil)
		}
		clean = append(clean, branch)
	}

	if len(clean) > 0 {
		return clean, true, nil
	}
	if scope.Global {
		return nil, false, nil
	}
	if len(scope.BranchIDs) == 0 {
		return nil, false, apperrors.Forbidden(
			"That branch is outside your purchasing scope.", nil)
	}
	return append([]string(nil), scope.BranchIDs...), true, nil
}

func (s *GoodsReceivingService) loadOrderEvidence(
	ctx context.Context,
	businessID string,
	supplierID string,
	branches []string,
	restricted bool,
	query SupplierPerformanceQuery,
	out *SupplierPerformance,
) error {
	row := s.db.QueryRow(ctx,
		`SELECT
		    count(*),
		    count(*) FILTER (WHERE status IN ('ORDERED','PARTIALLY_RECEIVED')),
		    count(*) FILTER (WHERE status IN ('RECEIVED','CLOSED')),
		    count(*) FILTER (WHERE status = 'CANCELLED'),
		    max(ordered_at),
		    min(created_at),
		    max(created_at)
		   FROM vendor_purchase_orders
		  WHERE business_id = $1
		    AND supplier_id = $2
		    AND (NOT $3::boolean OR branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR created_at >= $5)
		    AND ($6::timestamptz IS NULL OR created_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To)

	if err := row.Scan(
		&out.TotalPurchaseOrders,
		&out.OrdersOutstanding,
		&out.OrdersFullyReceived,
		&out.OrdersCancelled,
		&out.LastOrderedAt,
		&out.ObservedFrom,
		&out.ObservedTo,
	); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	return s.db.QueryRow(ctx,
		`SELECT
		    coalesce(count(l.id), 0),
		    coalesce(sum(l.converted_inventory_quantity), 0)
		   FROM vendor_purchase_order_lines l
		   JOIN vendor_purchase_orders o
		     ON o.id = l.purchase_order_id
		    AND o.business_id = l.business_id
		  WHERE o.business_id = $1
		    AND o.supplier_id = $2
		    AND (NOT $3::boolean OR o.branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR o.created_at >= $5)
		    AND ($6::timestamptz IS NULL OR o.created_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(&out.TotalOrderedLines, &out.TotalOrderedQuantity)
}

func (s *GoodsReceivingService) loadReceiptEvidence(
	ctx context.Context,
	businessID string,
	supplierID string,
	branches []string,
	restricted bool,
	query SupplierPerformanceQuery,
	out *SupplierPerformance,
) error {
	// Receipt-level facts: counts, last delivery, timeliness and lead time.
	// Only POSTED receipts count as delivery evidence; a draft inspection is
	// not a delivery, and a reversed receipt is evidence that was withdrawn.
	err := s.db.QueryRow(ctx,
		`SELECT
		    count(*),
		    count(*) FILTER (WHERE r.status = 'POSTED'),
		    max(r.received_at) FILTER (WHERE r.status = 'POSTED'),
		    count(*) FILTER (
		        WHERE r.status = 'POSTED'
		          AND o.expected_delivery_date IS NOT NULL
		          AND r.received_at::date <= o.expected_delivery_date
		    ),
		    count(*) FILTER (
		        WHERE r.status = 'POSTED'
		          AND o.expected_delivery_date IS NOT NULL
		          AND r.received_at::date > o.expected_delivery_date
		    ),
		    coalesce(
		        avg(
		            EXTRACT(EPOCH FROM (r.received_at - o.ordered_at)) / 3600.0
		        ) FILTER (
		            WHERE r.status = 'POSTED' AND o.ordered_at IS NOT NULL
		        ), 0
		    ),
		    count(*) FILTER (
		        WHERE r.status = 'POSTED' AND o.ordered_at IS NOT NULL
		    )
		   FROM vendor_goods_receipts r
		   JOIN vendor_purchase_orders o
		     ON o.id = r.purchase_order_id
		    AND o.business_id = r.business_id
		  WHERE r.business_id = $1
		    AND r.supplier_id = $2
		    AND (NOT $3::boolean OR r.branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR r.received_at >= $5)
		    AND ($6::timestamptz IS NULL OR r.received_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(
		&out.TotalGoodsReceipts,
		&out.TotalPostedGoodsReceipt,
		&out.LastDeliveredAt,
		&out.OnTimeDeliveries,
		&out.LateDeliveries,
		&out.leadTimeHours,
		&out.leadTimeSamples,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// Line-level inspection outcomes.
	return s.db.QueryRow(ctx,
		`SELECT
		    coalesce(count(l.id), 0),
		    coalesce(sum(l.delivered_quantity), 0),
		    coalesce(sum(l.accepted_quantity), 0),
		    coalesce(sum(l.short_quantity), 0),
		    coalesce(sum(l.damaged_quantity), 0),
		    coalesce(sum(l.rejected_quantity), 0),
		    coalesce(count(l.id) FILTER (WHERE l.short_quantity > 0), 0),
		    coalesce(count(l.id) FILTER (WHERE l.damaged_quantity > 0), 0),
		    coalesce(count(l.id) FILTER (WHERE l.rejected_quantity > 0), 0)
		   FROM vendor_goods_receipt_lines l
		   JOIN vendor_goods_receipts r
		     ON r.id = l.goods_receipt_id
		    AND r.business_id = l.business_id
		  WHERE r.business_id = $1
		    AND r.supplier_id = $2
		    AND r.status = 'POSTED'
		    AND (NOT $3::boolean OR r.branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR r.received_at >= $5)
		    AND ($6::timestamptz IS NULL OR r.received_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(
		&out.ReceiptLineCount,
		&out.TotalDeliveredQuantiy,
		&out.TotalAcceptedQuantity,
		&out.TotalShortQuantity,
		&out.TotalDamagedQuantity,
		&out.TotalRejectedQuantity,
		&out.ShortDeliveryLines,
		&out.DamagedLines,
		&out.RejectedLines,
	)
}

func (s *GoodsReceivingService) loadDiscrepancyEvidence(
	ctx context.Context,
	businessID string,
	supplierID string,
	branches []string,
	restricted bool,
	query SupplierPerformanceQuery,
	out *SupplierPerformance,
) error {
	return s.db.QueryRow(ctx,
		`SELECT
		    count(*),
		    count(*) FILTER (WHERE discrepancy_type = 'WRONG_ITEM'),
		    count(*) FILTER (WHERE discrepancy_type = 'QUALITY_ISSUE'),
		    count(*) FILTER (WHERE discrepancy_type = 'COST_VARIANCE'),
		    count(*) FILTER (WHERE status = 'OPEN'),
		    count(*) FILTER (WHERE status IN ('OPEN','APPROVAL_REQUIRED'))
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1
		    AND supplier_id = $2
		    AND (NOT $3::boolean OR branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR created_at >= $5)
		    AND ($6::timestamptz IS NULL OR created_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(
		&out.TotalDiscrepancies,
		&out.WrongItemDiscrepancies,
		&out.QualityDiscrepancies,
		&out.CostVarianceDiscrepancies,
		&out.OpenDiscrepancies,
		&out.UnresolvedDiscrepancies,
	)
}

func (s *GoodsReceivingService) loadCostEvidence(
	ctx context.Context,
	businessID string,
	supplierID string,
	branches []string,
	restricted bool,
	query SupplierPerformanceQuery,
	out *SupplierPerformance,
) error {
	if err := s.db.QueryRow(ctx,
		`SELECT count(*)
		   FROM vendor_purchase_cost_history
		  WHERE business_id = $1
		    AND supplier_id = $2
		    AND (NOT $3::boolean OR branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR observed_at >= $5)
		    AND ($6::timestamptz IS NULL OR observed_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(&out.CostObservations); err != nil {
		return err
	}

	// The most recent observation, which is the number an operator asks for
	// when negotiating. Absent rather than zero when never observed.
	err := s.db.QueryRow(ctx,
		`SELECT purchase_unit_cost_minor, observed_at, currency
		   FROM vendor_purchase_cost_history
		  WHERE business_id = $1
		    AND supplier_id = $2
		    AND (NOT $3::boolean OR branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR observed_at >= $5)
		    AND ($6::timestamptz IS NULL OR observed_at <= $6)
		  ORDER BY observed_at DESC, id DESC
		  LIMIT 1`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(&out.LatestObservedCost, &out.LatestObservedCostAt, &out.LatestObservedCostCcy)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// Average variance is taken over cost-variance discrepancies, in minor
	// units, using an integer average so no float becomes evidence.
	var varianceSum, varianceCount int64
	if err := s.db.QueryRow(ctx,
		`SELECT coalesce(sum(abs(variance_minor)), 0), count(*)
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1
		    AND supplier_id = $2
		    AND discrepancy_type = 'COST_VARIANCE'
		    AND (NOT $3::boolean OR branch_id = ANY($4::text[]))
		    AND ($5::timestamptz IS NULL OR created_at >= $5)
		    AND ($6::timestamptz IS NULL OR created_at <= $6)`,
		businessID, supplierID, restricted, branches, query.From, query.To,
	).Scan(&varianceSum, &varianceCount); err != nil {
		return err
	}

	if varianceCount > 0 {
		average := varianceSum / varianceCount
		out.AverageCostVariance = MoneyAverage{
			Value:       &average,
			Status:      MetricOK,
			SampleCount: varianceCount,
		}
	} else {
		out.AverageCostVariance = MoneyAverage{
			Status:      MetricInsufficientHistory,
			SampleCount: 0,
		}
	}

	return nil
}

// finalizeRates turns raw counts into rates that refuse to exist without a
// denominator. Every rate below is computed last, in one place, so no caller
// can add a metric that quietly divides by zero.
func (p *SupplierPerformance) finalizeRates() {
	deliveriesWithExpectation := p.OnTimeDeliveries + p.LateDeliveries
	p.OnTimeRate = ratio(p.OnTimeDeliveries, deliveriesWithExpectation)

	p.ShortDeliveryRate = ratio(p.ShortDeliveryLines, p.ReceiptLineCount)
	p.DamageRate = ratio(p.DamagedLines, p.ReceiptLineCount)
	p.RejectionRate = ratio(p.RejectedLines, p.ReceiptLineCount)

	// Acceptance is measured in quantity, not lines: accepting 1 of 100 units
	// on every line is not a 100% acceptance record.
	if p.TotalDeliveredQuantiy > 0 {
		value := float64(p.TotalAcceptedQuantity) / float64(p.TotalDeliveredQuantiy)
		p.AcceptanceRate = Ratio{
			Value:       &value,
			Status:      MetricOK,
			SampleCount: p.TotalDeliveredQuantiy,
		}
	} else {
		p.AcceptanceRate = Ratio{
			Status:      MetricInsufficientHistory,
			SampleCount: 0,
		}
	}

	p.CostVarianceRate = ratio(p.CostVarianceDiscrepancies, p.CostObservations)

	if p.leadTimeSamples >= minimumRateSample {
		hours := p.leadTimeHours
		p.AverageActualLeadTime = AverageHours{
			Value:       &hours,
			Status:      MetricOK,
			SampleCount: p.leadTimeSamples,
		}
	} else {
		p.AverageActualLeadTime = AverageHours{
			Status:      MetricInsufficientHistory,
			SampleCount: p.leadTimeSamples,
		}
	}
}
