package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/models"
	"cosmicforge/logistics/shared/go/commerceanalytics"
)

var ErrDuplicateEvent = errors.New("duplicate analytics event")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Ingest(ctx context.Context, event commerceanalytics.Envelope) (bool, error) {
	if err := commerceanalytics.ValidateEnvelope(event); err != nil {
		return false, err
	}
	hash, err := commerceanalytics.Fingerprint(event.Payload)
	if err != nil {
		return false, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO analytics_commerce_events
			(event_id,event_type,schema_version,source_service,source_aggregate,source_id,source_version,
			 business_id,vendor_id,branch_id,occurred_at,payload,payload_hash,status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12,$13,'PROCESSED')
		ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, event.EventType, event.SchemaVersion, event.SourceService, event.SourceAggregate,
		event.SourceID, event.SourceVersion, event.BusinessID, event.VendorID, event.BranchID,
		event.OccurredAt, event.Payload, hash)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, ErrDuplicateEvent
	}
	if err := r.applyAggregate(ctx, tx, event); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) applyAggregate(ctx context.Context, tx pgx.Tx, e commerceanalytics.Envelope) error {
	day := e.OccurredAt.UTC().Truncate(24 * time.Hour)
	businessID := first(e.BusinessID, e.VendorID)
	branchID := e.BranchID

	switch e.EventType {
	case commerceanalytics.EventOrderCompleted:
		var p commerceanalytics.OrderCompletedPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "order_count", 1, "completed_value_kobo", p.OrderValueKobo); err != nil {
			return err
		}
		if p.ItemID != "" {
			if err := r.bumpDimension(ctx, tx, businessID, branchID, "ITEM", p.ItemID, day, p.Quantity, 1, p.OrderValueKobo, 0, 0); err != nil {
				return err
			}
		}
		if p.CategoryID != "" {
			if err := r.bumpDimension(ctx, tx, businessID, branchID, "CATEGORY", p.CategoryID, day, p.Quantity, 1, p.OrderValueKobo, 0, 0); err != nil {
				return err
			}
		}
	case commerceanalytics.EventOrderAccepted:
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "accepted_count", 1, "", 0)
	case commerceanalytics.EventOrderRejected:
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "rejected_count", 1, "", 0)
	case commerceanalytics.EventOrderCancelled:
		var p commerceanalytics.OperationalStatePayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if p.Reason == "CUSTOMER" {
			return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "cancelled_customer_count", 1, "", 0)
		}
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "cancelled_vendor_count", 1, "", 0)
	case commerceanalytics.EventOrderRefunded:
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "refund_count", 1, "", 0)
	case commerceanalytics.EventPreparationTiming, commerceanalytics.EventPickingTiming:
		var p commerceanalytics.DurationPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if e.EventType == commerceanalytics.EventPreparationTiming {
			if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "preparation_seconds", p.DurationSeconds, "preparation_count", 1); err != nil {
				return err
			}
			return nil
		}
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "picking_seconds", p.DurationSeconds, "picking_count", 1)
	case commerceanalytics.EventFulfilmentPerformance:
		var p commerceanalytics.FulfilmentPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		modePrefix := "platform"
		if p.Mode == "OWN_RIDER" || p.Mode == "VENDOR_OWNED" {
			modePrefix = "own_rider"
		}
		if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, modePrefix+"_count", 1, "", 0); err != nil {
			return err
		}
		if !p.Completed {
			if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, modePrefix+"_failures", 1, "", 0); err != nil {
				return err
			}
		}
		if p.OnTimeReadiness {
			return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, modePrefix+"_on_time", 1, "", 0)
		}
	case commerceanalytics.EventItemAvailability:
		return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "availability_events", 1, "", 0)
	case commerceanalytics.EventInventoryMovement, commerceanalytics.EventWasteExpiry:
		var p commerceanalytics.InventoryMovementPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if p.Reason == "STOCK_OUT" || p.Quantity < 0 {
			if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "stockout_events", 1, "", 0); err != nil {
				return err
			}
		}
	case commerceanalytics.EventPurchaseReceived, commerceanalytics.EventSupplierDelivery:
		var p commerceanalytics.PurchaseReceivedPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		return r.upsertSupplier(ctx, tx, businessID, p.SupplierID, p.ItemID, day, p)
	case commerceanalytics.EventProductionCapacity:
		var p commerceanalytics.ProductionCapacityPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		return r.upsertCapacity(ctx, tx, businessID, branchID, p.ProductionType, day, p.AvailableUnits, p.UsedUnits)
	case commerceanalytics.EventRFQStateChanged:
		var p commerceanalytics.RFQPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		return r.upsertRFQ(ctx, tx, businessID, branchID, day, p.State)
	case commerceanalytics.EventRFQQuoteSubmitted:
		return r.upsertRFQ(ctx, tx, businessID, branchID, day, "QUOTE_SUBMITTED")
	case commerceanalytics.EventRFQAccepted:
		return r.upsertRFQ(ctx, tx, businessID, branchID, day, "ACCEPTED")
	case commerceanalytics.EventCRMRelationship:
		var p commerceanalytics.CRMRelationshipPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		return r.upsertCRM(ctx, tx, businessID, branchID, day, p.Cohort, p.LifetimeKobo)
	case commerceanalytics.EventSearchDemand:
		var p commerceanalytics.SearchDemandPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		stockout := int64(0)
		if p.OutOfStock || p.Unavailable {
			stockout = 1
			if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "stockout_events", 1, "lost_opportunity_estimate_kobo", 0); err != nil {
				return err
			}
		}
		if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "demand_events", 1, "", 0); err != nil {
			return err
		}
		if p.ItemID != "" {
			return r.bumpDimension(ctx, tx, businessID, branchID, "ITEM", p.ItemID, day, 0, 0, 0, 1, stockout)
		}
		if p.CategoryID != "" {
			return r.bumpDimension(ctx, tx, businessID, branchID, "CATEGORY", p.CategoryID, day, 0, 0, 0, 1, stockout)
		}
	case commerceanalytics.EventSupportOutcome, commerceanalytics.EventSupportCaseResolved:
		var p commerceanalytics.SupportOutcomePayload
		if err := decode(e.Payload, &p); err != nil || p.CaseRef == "" {
			var generic map[string]any
			if err := decode(e.Payload, &generic); err != nil {
				return err
			}
			p.CaseRef, _ = generic["case_ref"].(string)
			p.Category, _ = generic["category"].(string)
			p.Outcome = "CONFIRMED"
			if resolution, ok := generic["resolution"].(string); ok && resolution == "DISMISSED" {
				p.Outcome = "DISMISSED"
			}
			p.VendorFault = p.Outcome == "CONFIRMED"
		}
		if err := r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "quality_reported", 1, "", 0); err != nil {
			return err
		}
		if p.VendorFault && p.Outcome == "CONFIRMED" {
			return r.bumpDaily(ctx, tx, businessID, e.VendorID, branchID, day, "quality_confirmed", 1, "", 0)
		}
	case commerceanalytics.EventPriceSnapshot:
		var p commerceanalytics.PriceSnapshotPayload
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		return r.upsertPrice(ctx, tx, businessID, branchID, p, day)
	case commerceanalytics.EventGuaranteeEvidence:
		return nil
	}
	return nil
}

func (r *Repository) bumpDaily(ctx context.Context, tx pgx.Tx, businessID, vendorID, branchID string, day time.Time, field1 string, value1 int64, field2 string, value2 int64) error {
	if field1 == "" {
		return nil
	}
	sql := fmt.Sprintf(`INSERT INTO analytics_vendor_daily_metrics(business_id,vendor_id,branch_id,metric_date,%s)
		VALUES ($1,NULLIF($2,''),NULLIF($3,''),$4,$5)
		ON CONFLICT (business_id, branch_id, metric_date) DO UPDATE SET %s = analytics_vendor_daily_metrics.%s + EXCLUDED.%s, updated_at=now()`,
		field1, field1, field1, field1)
	args := []any{businessID, vendorID, branchID, day, value1}
	if field2 != "" {
		sql = fmt.Sprintf(`INSERT INTO analytics_vendor_daily_metrics(business_id,vendor_id,branch_id,metric_date,%s,%s)
			VALUES ($1,NULLIF($2,''),NULLIF($3,''),$4,$5,$6)
			ON CONFLICT (business_id, branch_id, metric_date) DO UPDATE SET %s = analytics_vendor_daily_metrics.%s + EXCLUDED.%s, %s = analytics_vendor_daily_metrics.%s + EXCLUDED.%s, updated_at=now()`,
			field1, field2, field1, field1, field1, field2, field2, field2)
		args = append(args, value2)
	}
	_, err := tx.Exec(ctx, sql, args...)
	return err
}

func (r *Repository) bumpDimension(ctx context.Context, tx pgx.Tx, businessID, branchID, typ, id string, day time.Time, units, orders, value, demand, stockoutDemand int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_dimension_metrics
		(business_id,branch_id,dimension_type,dimension_id,metric_date,units_count,orders_count,value_kobo,demand_events,stockout_demand_events)
		VALUES ($1,COALESCE(NULLIF($2,''),''),$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (business_id, branch_id, dimension_type, dimension_id, metric_date) DO UPDATE SET
			units_count=analytics_vendor_dimension_metrics.units_count+EXCLUDED.units_count,
			orders_count=analytics_vendor_dimension_metrics.orders_count+EXCLUDED.orders_count,
			value_kobo=analytics_vendor_dimension_metrics.value_kobo+EXCLUDED.value_kobo,
			demand_events=analytics_vendor_dimension_metrics.demand_events+EXCLUDED.demand_events,
			stockout_demand_events=analytics_vendor_dimension_metrics.stockout_demand_events+EXCLUDED.stockout_demand_events,
			updated_at=now()`,
		businessID, branchID, typ, id, day, units, orders, value, demand, stockoutDemand)
	return err
}

func (r *Repository) upsertSupplier(ctx context.Context, tx pgx.Tx, businessID, supplierID, itemID string, day time.Time, p commerceanalytics.PurchaseReceivedPayload) error {
	if supplierID == "" {
		return nil
	}
	onTime := int64(0)
	if p.ExpectedAt != "" && p.ReceivedAt != "" && p.ReceivedAt <= p.ExpectedAt {
		onTime = 1
	}
	_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_supplier_metrics
		(business_id,supplier_id,item_id,metric_date,deliveries_count,on_time_count,ordered_quantity,accepted_quantity,damaged_quantity,total_purchase_cost_kobo,unit_cost_samples)
		VALUES ($1,$2,COALESCE(NULLIF($3,''),''),$4,1,$5,$6,$7,$8,$9,1)
		ON CONFLICT (business_id,supplier_id,item_id,metric_date) DO UPDATE SET
			deliveries_count=analytics_vendor_supplier_metrics.deliveries_count+1,
			on_time_count=analytics_vendor_supplier_metrics.on_time_count+EXCLUDED.on_time_count,
			ordered_quantity=analytics_vendor_supplier_metrics.ordered_quantity+EXCLUDED.ordered_quantity,
			accepted_quantity=analytics_vendor_supplier_metrics.accepted_quantity+EXCLUDED.accepted_quantity,
			damaged_quantity=analytics_vendor_supplier_metrics.damaged_quantity+EXCLUDED.damaged_quantity,
			total_purchase_cost_kobo=analytics_vendor_supplier_metrics.total_purchase_cost_kobo+EXCLUDED.total_purchase_cost_kobo,
			unit_cost_samples=analytics_vendor_supplier_metrics.unit_cost_samples+1,
			updated_at=now()`,
		businessID, supplierID, itemID, day, onTime, p.OrderedQuantity, p.AcceptedQuantity, p.DamagedQuantity, p.UnitCostKobo)
	return err
}

func (r *Repository) upsertCapacity(ctx context.Context, tx pgx.Tx, businessID, branchID, productionType string, day time.Time, available, used int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_capacity_metrics
		(business_id,branch_id,production_type,metric_date,available_units,used_units,samples)
		VALUES ($1,COALESCE(NULLIF($2,''),''),$3,$4,$5,$6,1)
		ON CONFLICT (business_id,branch_id,production_type,metric_date) DO UPDATE SET
			available_units=analytics_vendor_capacity_metrics.available_units+EXCLUDED.available_units,
			used_units=analytics_vendor_capacity_metrics.used_units+EXCLUDED.used_units,
			samples=analytics_vendor_capacity_metrics.samples+1,
			updated_at=now()`, businessID, branchID, productionType, day, available, used)
	return err
}

func (r *Repository) upsertRFQ(ctx context.Context, tx pgx.Tx, businessID, branchID string, day time.Time, state string) error {
	column := map[string]string{"RECEIVED": "received", "QUOTE_SUBMITTED": "quotes_submitted", "ACCEPTED": "accepted", "REJECTED": "rejected", "EXPIRED": "expired"}[state]
	if column == "" {
		return nil
	}
	sql := fmt.Sprintf(`INSERT INTO analytics_vendor_rfq_metrics(business_id,branch_id,metric_date,%s)
		VALUES ($1,COALESCE(NULLIF($2,''),''),$3,1)
		ON CONFLICT (business_id,branch_id,metric_date) DO UPDATE SET %s=analytics_vendor_rfq_metrics.%s+1, updated_at=now()`, column, column, column)
	_, err := tx.Exec(ctx, sql, businessID, branchID, day)
	return err
}

func (r *Repository) upsertCRM(ctx context.Context, tx pgx.Tx, businessID, branchID string, day time.Time, cohort string, lifetime int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_crm_cohorts(business_id,branch_id,cohort,metric_date,customers,total_lifetime_kobo,policy_version)
		VALUES ($1,COALESCE(NULLIF($2,''),''),$3,$4,1,$5,$6)
		ON CONFLICT (business_id,branch_id,cohort,metric_date,policy_version) DO UPDATE SET
			customers=analytics_vendor_crm_cohorts.customers+1,
			total_lifetime_kobo=analytics_vendor_crm_cohorts.total_lifetime_kobo+EXCLUDED.total_lifetime_kobo,
			updated_at=now()`,
		businessID, branchID, cohort, day, lifetime, commerceanalytics.CohortPolicyVersion)
	return err
}

func (r *Repository) upsertPrice(ctx context.Context, tx pgx.Tx, businessID, branchID string, p commerceanalytics.PriceSnapshotPayload, day time.Time) error {
	if p.ItemID == "" {
		return nil
	}
	if !commerceanalytics.PriceBenchmarkAllowed(p) {
		_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_dimension_metrics
			(business_id,branch_id,dimension_type,dimension_id,metric_date,sample_size,own_price_kobo)
			VALUES ($1,COALESCE(NULLIF($2,''),''),'PRICE_ITEM',$3,$4,$5,$6)
			ON CONFLICT (business_id,branch_id,dimension_type,dimension_id,metric_date) DO UPDATE SET
				sample_size=GREATEST(analytics_vendor_dimension_metrics.sample_size, EXCLUDED.sample_size),
				own_price_kobo=EXCLUDED.own_price_kobo,
				nearby_median_kobo=NULL,
				nearby_min_kobo=NULL,
				nearby_max_kobo=NULL,
				updated_at=now()`,
			businessID, branchID, p.ItemID, day, p.SampleSize, p.OwnPriceKobo)
		return err
	}
	low, high := commerceanalytics.CoarseBenchmarkRange(p.NearbyMinKobo, p.NearbyMaxKobo)
	_, err := tx.Exec(ctx, `INSERT INTO analytics_vendor_dimension_metrics
		(business_id,branch_id,dimension_type,dimension_id,metric_date,sample_size,nearby_median_kobo,nearby_min_kobo,nearby_max_kobo,own_price_kobo)
		VALUES ($1,COALESCE(NULLIF($2,''),''),'PRICE_ITEM',$3,$4,$5,NULLIF($6,0),NULLIF($7,0),NULLIF($8,0),$9)
		ON CONFLICT (business_id,branch_id,dimension_type,dimension_id,metric_date) DO UPDATE SET
			sample_size=GREATEST(analytics_vendor_dimension_metrics.sample_size, EXCLUDED.sample_size),
			nearby_median_kobo=EXCLUDED.nearby_median_kobo,
			nearby_min_kobo=EXCLUDED.nearby_min_kobo,
			nearby_max_kobo=EXCLUDED.nearby_max_kobo,
			own_price_kobo=EXCLUDED.own_price_kobo,
			updated_at=now()`,
		businessID, branchID, p.ItemID, day, p.SampleSize, p.NearbyMedianKobo, low, high, p.OwnPriceKobo)
	return err
}

func (r *Repository) Report(ctx context.Context, businessID, branchID string, w models.Window) (models.OperationalReport, error) {
	report := models.OperationalReport{
		BusinessID:                      businessID,
		BranchID:                        branchID,
		WindowStart:                     w.Start,
		WindowEnd:                       w.End,
		ReportVersion:                   "commerce_operational_report_v1",
		Classification:                  "OPERATIONAL_DECISION_SUPPORT_NOT_AUDITED_ACCOUNTING",
		GuaranteeStatus:                 commerceanalytics.GuaranteeStatusFutureDisabled,
		UnavailableAuthoritativeMetrics: []string{"supplier_damage_evidence_when_receiving_damage_quantity_absent"},
	}
	err := r.db.QueryRow(ctx, `SELECT
		COALESCE(sum(order_count),0), COALESCE(sum(completed_value_kobo),0),
		COALESCE(sum(accepted_count),0), COALESCE(sum(rejected_count),0),
		COALESCE(sum(cancelled_vendor_count),0), COALESCE(sum(cancelled_customer_count),0),
		COALESCE(sum(refund_count),0), COALESCE(sum(preparation_seconds),0), COALESCE(sum(preparation_count),0),
		COALESCE(sum(picking_seconds),0), COALESCE(sum(picking_count),0),
		COALESCE(sum(own_rider_count),0), COALESCE(sum(platform_count),0),
		COALESCE(sum(own_rider_failures),0), COALESCE(sum(platform_failures),0),
		COALESCE(sum(own_rider_on_time),0), COALESCE(sum(platform_on_time),0),
		COALESCE(sum(stockout_events),0), COALESCE(sum(demand_events),0),
		COALESCE(sum(lost_opportunity_estimate_kobo),0), COALESCE(max(updated_at), now())
		FROM analytics_vendor_daily_metrics
		WHERE business_id=$1 AND ($2='' OR branch_id=$2) AND metric_date >= $3::date AND metric_date < $4::date`,
		businessID, branchID, w.Start, w.End).Scan(
		&report.Sales.OrderCount, &report.Sales.CompletedValueKobo,
		new(int64), new(int64),
		&report.Operations.VendorCancellationCount, &report.Operations.CustomerCancellationCount,
		new(int64), new(int64), new(int64), new(int64), new(int64),
		&report.Fulfilment.OwnRiderOrders, &report.Fulfilment.PlatformOrders,
		new(int64), new(int64), new(int64), new(int64),
		&report.Inventory.StockOutEvents, &report.Demand.DemandEvents,
		&report.Demand.LostOpportunityEstimateKobo, &report.DataAsOf)
	if err != nil {
		return report, err
	}
	if report.Sales.OrderCount > 0 {
		report.Sales.AverageOrderValue = float64(report.Sales.CompletedValueKobo) / float64(report.Sales.OrderCount)
	}
	report.Demand.CalculationVersion = commerceanalytics.DemandPolicyVersion
	report.Demand.EstimateLabel = "ESTIMATE_NOT_LOST_SALE_COUNT"
	report.PriceIntelligence.CalculationVersion = commerceanalytics.PriceBenchmarkVersion
	report.PriceIntelligence.MinimumCohort = commerceanalytics.MinimumBenchmarkCohort
	report.CRM.Policy = commerceanalytics.CohortPolicyVersion
	_ = r.db.QueryRow(ctx, `SELECT
		COALESCE(sum(CASE WHEN sample_size >= $4 THEN 1 ELSE 0 END),0),
		COALESCE(sum(CASE WHEN sample_size > 0 AND sample_size < $4 THEN 1 ELSE 0 END),0)
		FROM analytics_vendor_dimension_metrics
		WHERE business_id=$1 AND ($2='' OR branch_id=$2) AND metric_date >= $3::date AND dimension_type='PRICE_ITEM'`,
		businessID, branchID, w.Start, commerceanalytics.MinimumBenchmarkCohort).Scan(&report.PriceIntelligence.BenchmarksAvailable, &report.PriceIntelligence.LowSampleSuppressed)
	return report, nil
}

func (r *Repository) Reliability(ctx context.Context, businessID, branchID string, w models.Window) (models.ReliabilityScore, error) {
	var accepted, rejected, vendorCancelled, orderCount, onTime, availabilityEvents, qualityConfirmed int64
	var dataAsOf *time.Time
	err := r.db.QueryRow(ctx, `SELECT
		COALESCE(sum(accepted_count),0), COALESCE(sum(rejected_count),0), COALESCE(sum(cancelled_vendor_count),0),
		COALESCE(sum(order_count),0), COALESCE(sum(own_rider_on_time + platform_on_time),0),
		COALESCE(sum(availability_events),0), COALESCE(sum(quality_confirmed),0), max(updated_at)
		FROM analytics_vendor_daily_metrics
		WHERE business_id=$1 AND ($2='' OR branch_id=$2) AND metric_date >= $3::date AND metric_date < $4::date`,
		businessID, branchID, w.Start, w.End).Scan(&accepted, &rejected, &vendorCancelled, &orderCount, &onTime, &availabilityEvents, &qualityConfirmed, &dataAsOf)
	if err != nil {
		return models.ReliabilityScore{}, err
	}
	sample := accepted + rejected + orderCount
	weights := commerceanalytics.CurrentReliabilityWeightPolicy().Weights
	components := map[string]float64{
		"acceptance":                         models.Rate(accepted, accepted+rejected),
		"on_time_readiness":                  models.Rate(onTime, orderCount),
		"vendor_cancellation":                1 - models.Rate(vendorCancelled, accepted+vendorCancelled),
		"availability_substitution_accuracy": 1 - models.Rate(availabilityEvents, orderCount+availabilityEvents),
		"quality_issue_rate":                 1 - models.Rate(qualityConfirmed, orderCount+qualityConfirmed),
	}
	score := models.ReliabilityScore{
		BusinessID:       businessID,
		BranchID:         branchID,
		WindowStart:      w.Start,
		WindowEnd:        w.End,
		PolicyVersion:    commerceanalytics.ReliabilityPolicyVersion,
		Status:           "INSUFFICIENT_DATA",
		ComponentValues:  components,
		ComponentWeights: weights,
		SampleSize:       sample,
		CalculatedAt:     time.Now().UTC(),
	}
	if sample >= 10 {
		value := 0
		for k, weight := range weights {
			value += int(components[k] * weight * 100)
		}
		score.Score = &value
		score.Status = "CALCULATED"
	}
	rawComponents, _ := json.Marshal(components)
	rawWeights, _ := json.Marshal(weights)
	_, err = r.db.Exec(ctx, `INSERT INTO analytics_vendor_reliability_scores
		(id,business_id,branch_id,window_start,window_end,policy_version,score,status,component_values,component_weights,input_event_max_occurred_at,sample_size)
		VALUES ($1,$2,COALESCE(NULLIF($3,''),''),$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (business_id,branch_id,window_start,window_end,policy_version) DO UPDATE SET
			score=EXCLUDED.score,status=EXCLUDED.status,component_values=EXCLUDED.component_values,
			component_weights=EXCLUDED.component_weights,input_event_max_occurred_at=EXCLUDED.input_event_max_occurred_at,
			sample_size=EXCLUDED.sample_size,calculated_at=now()`,
		fmt.Sprintf("rel_%s_%s_%d_%d", businessID, branchID, w.Start.Unix(), w.End.Unix()),
		businessID, branchID, w.Start, w.End, score.PolicyVersion, score.Score, score.Status, rawComponents, rawWeights, dataAsOf, sample)
	return score, err
}

func decode(raw []byte, out any) error {
	return json.Unmarshal(raw, out)
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
