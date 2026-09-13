package checkoutquote

import (
	"context"
	"encoding/json"
	"errors"
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

type Store struct {
	db     *pgxpool.Pool
	outbox *vendorevents.DomainPublisher
}

func NewStore(db *pgxpool.Pool, publishers ...*vendorevents.DomainPublisher) *Store {
	store := &Store{db: db}
	if len(publishers) > 0 {
		store.outbox = publishers[0]
	}
	return store
}

func (s *Store) LoadSource(ctx context.Context, customerID, sessionID, planID string, now time.Time) (SourceSnapshot, error) {
	if s == nil || s.db == nil {
		return SourceSnapshot{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SourceSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	source, err := loadSource(ctx, tx, customerID, sessionID, planID, now)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SourceSnapshot{}, err
	}
	return source, nil
}

func loadSource(ctx context.Context, q queryer, customerID, sessionID, planID string, now time.Time) (SourceSnapshot, error) {
	var source SourceSnapshot
	var checkoutStatus, planStatus, planFailure string
	err := q.QueryRow(ctx, `
		SELECT s.id,s.validation_revision,s.expires_at,s.cart_id,s.cart_version,s.customer_id,s.currency,
		       a.source_address_ref,a.title_snapshot,a.address_snapshot,a.latitude_snapshot,a.longitude_snapshot,
		       a.description_snapshot,a.snapshotted_at,s.quoted_items_total_minor,s.max_authorized_items_total_minor,
		       p.id,p.plan_revision,p.version,p.expires_at,p.mode,p.status,p.failure_code,
		       p.delivery_gross_minor,p.delivery_customer_minor,p.delivery_vendor_funded_minor,p.delivery_platform_funded_minor,
		       s.status
		  FROM commerce_checkout_sessions s
		  JOIN commerce_cart_addresses a ON a.cart_id=s.cart_id AND a.source_address_ref=s.address_ref
		  JOIN commerce_fulfilment_plans p ON p.id=$3 AND p.checkout_session_id=s.id AND p.customer_id=s.customer_id
		 WHERE s.id=$2 AND s.customer_id=$1
		   AND p.checkout_validation_revision=s.validation_revision`, customerID, sessionID, planID).Scan(
		&source.CheckoutSessionID, &source.CheckoutValidationRevision, &source.CheckoutExpiresAt,
		&source.CartID, &source.CartVersion, &source.CustomerID, &source.Currency,
		&source.Address.SourceAddressRef, &source.Address.Title, &source.Address.Address,
		&source.Address.Latitude, &source.Address.Longitude, &source.Address.Description,
		&source.Address.SnapshottedAt, &source.QuotedItemsTotalMinor, &source.MaxAuthorizedItemsTotalMinor,
		&source.FulfilmentPlanID, &source.FulfilmentPlanRevision, &source.FulfilmentPlanVersion,
		&source.FulfilmentPlanExpiresAt, &source.SelectedFulfilmentMode, &planStatus, &planFailure,
		&source.DeliveryFunding.GrossMinor, &source.DeliveryFunding.CustomerMinor,
		&source.DeliveryFunding.VendorFundedMinor, &source.DeliveryFunding.PlatformFundedMinor,
		&checkoutStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceSnapshot{}, ErrSourceNotQuotable
	}
	if err != nil {
		return SourceSnapshot{}, err
	}
	if checkoutStatus != "VALIDATED" || planStatus != "PLANNED" || planFailure != "" ||
		source.CheckoutValidationRevision <= 0 ||
		!source.CheckoutExpiresAt.After(now) || !source.FulfilmentPlanExpiresAt.After(now) {
		return SourceSnapshot{}, ErrSourceNotQuotable
	}

	source.Groups, err = loadSourceGroups(ctx, q, source)
	if err != nil {
		return SourceSnapshot{}, err
	}
	source.Lines, err = loadSourceLines(ctx, q, source)
	if err != nil {
		return SourceSnapshot{}, err
	}
	source.Missions, err = loadSourceMissions(ctx, q, source)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if len(source.Groups) == 0 || len(source.Lines) == 0 {
		return SourceSnapshot{}, ErrSourceNotQuotable
	}
	return source, nil
}

func loadSourceGroups(ctx context.Context, q queryer, source SourceSnapshot) ([]GroupSnapshot, error) {
	rows, err := q.Query(ctx, `
		SELECT cg.id,cg.business_id,cg.branch_id,cg.quoted_total_minor,cg.max_authorized_total_minor,
		       cg.inventory_reservation_ref,cg.reservation_source_ref,pg.id,COALESCE(pg.mission_id,''),
		       pg.gross_fee_minor,pg.customer_fee_minor,pg.vendor_funded_minor,pg.platform_funded_minor,
		       cg.reservation_status,pg.serviceable
		  FROM commerce_checkout_groups cg
		  JOIN commerce_fulfilment_plan_groups pg
		    ON pg.plan_id=$3 AND pg.checkout_group_id=cg.id
		 WHERE cg.session_id=$2 AND cg.customer_id=$1
		 ORDER BY cg.id`, source.CustomerID, source.CheckoutSessionID, source.FulfilmentPlanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []GroupSnapshot
	for rows.Next() {
		var group GroupSnapshot
		var reservationStatus string
		var serviceable bool
		if err = rows.Scan(&group.CheckoutGroupID, &group.BusinessID, &group.BranchID,
			&group.ProductSubtotalMinor, &group.MaxProductSubtotalMinor,
			&group.InventoryReservationRef, &group.InventoryReservationSourceRef,
			&group.LogisticsPlanGroupRef, &group.LogisticsMissionRef,
			&group.DeliveryFunding.GrossMinor, &group.DeliveryFunding.CustomerMinor,
			&group.DeliveryFunding.VendorFundedMinor, &group.DeliveryFunding.PlatformFundedMinor,
			&reservationStatus, &serviceable); err != nil {
			return nil, err
		}
		if !serviceable || (reservationStatus != "RESERVED" && reservationStatus != "NOT_REQUIRED") || !group.DeliveryFundingBalanced() {
			return nil, ErrSourceNotQuotable
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (g GroupSnapshot) DeliveryFundingBalanced() bool {
	return g.DeliveryFunding.CustomerMinor+g.DeliveryFunding.VendorFundedMinor+g.DeliveryFunding.PlatformFundedMinor == g.DeliveryFunding.GrossMinor
}

func loadSourceLines(ctx context.Context, q queryer, source SourceSnapshot) ([]LineSnapshot, error) {
	rows, err := q.Query(ctx, `
		SELECT l.id,l.group_id,l.cart_item_id,l.domain,l.business_id,l.branch_id,l.item_ref,l.variant_ref,
		       l.listing_ref,l.sell_unit_ref,l.quantity,l.unit_price_minor,l.price_reference,l.catalogue_revision,
		       l.inventory_revision,l.pricing_basis,l.requested_measure_ref,l.expected_measure_quantity,
		       l.measure_underfill_bps,l.measure_overfill_bps,l.quoted_total_minor,l.max_authorized_total_minor,
		       l.inventory_item_ref,g.inventory_reservation_ref
		  FROM commerce_checkout_lines l
		  JOIN commerce_checkout_groups g ON g.id=l.group_id AND g.session_id=l.session_id
		 WHERE l.session_id=$1 AND l.customer_id=$2 AND l.validation_status IN ('VALID','CHANGED')
		 ORDER BY l.cart_item_id,l.id`, source.CheckoutSessionID, source.CustomerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []LineSnapshot
	for rows.Next() {
		var line LineSnapshot
		if err = rows.Scan(&line.CheckoutLineID, &line.CheckoutGroupID, &line.CartItemID, &line.Domain,
			&line.BusinessID, &line.BranchID, &line.ItemRef, &line.VariantRef, &line.ListingRef,
			&line.SellUnitRef, &line.Quantity, &line.VendorOnlineUnitPriceMinor, &line.PriceReference,
			&line.CatalogueRevision, &line.InventoryRevision, &line.PricingBasis,
			&line.RequestedMeasureRef, &line.ExpectedMeasureQuantity, &line.MeasureUnderfillBps,
			&line.MeasureOverfillBps, &line.QuotedTotalMinor, &line.MaxAuthorizedTotalMinor,
			&line.InventoryItemRef, &line.InventoryReservationRef); err != nil {
			return nil, err
		}
		line.OnlinePriceDisclosure = "VENDOR_ONLINE_PRICE"
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadSourceMissions(ctx context.Context, q queryer, source SourceSnapshot) ([]MissionSnapshot, error) {
	rows, err := q.Query(ctx, `
		SELECT m.id,m.authority,m.service,m.vehicle_class,m.authority_quote_id,m.authority_quote_reference,
		       m.authority_quote_revision,m.authority_quote_expires_at,m.gross_fee_minor,m.customer_fee_minor,
		       m.vendor_funded_minor,m.platform_funded_minor,
		       ARRAY(SELECT pg.checkout_group_id FROM commerce_fulfilment_plan_groups pg WHERE pg.mission_id=m.id ORDER BY pg.checkout_group_id)
		  FROM commerce_fulfilment_missions m WHERE m.plan_id=$1 ORDER BY m.id`, source.FulfilmentPlanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var missions []MissionSnapshot
	for rows.Next() {
		var mission MissionSnapshot
		if err = rows.Scan(&mission.MissionID, &mission.Authority, &mission.Service, &mission.VehicleClass,
			&mission.AuthorityQuoteID, &mission.AuthorityQuoteReference, &mission.AuthorityQuoteRevision,
			&mission.AuthorityQuoteExpiresAt, &mission.Funding.GrossMinor, &mission.Funding.CustomerMinor,
			&mission.Funding.VendorFundedMinor, &mission.Funding.PlatformFundedMinor, &mission.GroupIDs); err != nil {
			return nil, err
		}
		missions = append(missions, mission)
	}
	return missions, rows.Err()
}

func (s *Store) Save(ctx context.Context, quote Quote, now time.Time) (Quote, error) {
	if s == nil || s.db == nil {
		return Quote{}, ErrDependencyMissing
	}
	// The per-checkout advisory lock serializes version allocation. READ
	// COMMITTED is intentional: a waiter must see the quote committed by the
	// lock holder before deciding whether the same material already exists.
	// A transaction-level snapshot taken before waiting would turn a harmless
	// concurrent duplicate into a serialization failure.
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Quote{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,17))`, quote.CheckoutSessionID); err != nil {
		return Quote{}, err
	}
	// Re-read all material source evidence after acquiring the serialization
	// lock. This closes the load/build/save race even when an active identical
	// quote already exists: a stale quote is never returned as if it were a
	// current creation result.
	currentSource, err := loadSource(ctx, tx, quote.CustomerID, quote.CheckoutSessionID, quote.FulfilmentPlanID, now)
	if err != nil {
		if errors.Is(err, ErrSourceNotQuotable) {
			return Quote{}, ErrQuoteConflict
		}
		return Quote{}, err
	}
	if currentSource.CheckoutExpiresAt.Before(quote.ExpiresAt) || currentSource.FulfilmentPlanExpiresAt.Before(quote.ExpiresAt) {
		return Quote{}, ErrQuoteConflict
	}
	currentSourceFingerprint, err := SourceFingerprint(currentSource)
	if err != nil {
		return Quote{}, err
	}
	expectedSourceFingerprint, err := QuoteSourceFingerprint(quote)
	if err != nil {
		return Quote{}, err
	}
	if currentSourceFingerprint != expectedSourceFingerprint {
		return Quote{}, ErrQuoteConflict
	}

	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM commerce_quotes WHERE checkout_session_id=$1 AND material_fingerprint=$2 AND expires_at>$3 ORDER BY quote_version DESC LIMIT 1`, quote.CheckoutSessionID, quote.MaterialFingerprint, now).Scan(&existingID)
	if err == nil {
		existing, loadErr := loadQuote(ctx, tx, quote.CustomerID, existingID, now)
		if loadErr != nil {
			return Quote{}, loadErr
		}
		if err = tx.Commit(ctx); err != nil {
			return Quote{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, err
	}

	var valid bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM commerce_checkout_sessions s
			JOIN commerce_cart_addresses a ON a.cart_id=s.cart_id AND a.source_address_ref=s.address_ref
			JOIN commerce_fulfilment_plans p ON p.id=$4 AND p.checkout_session_id=s.id AND p.customer_id=s.customer_id
			WHERE s.id=$1 AND s.customer_id=$2 AND s.validation_revision=$3 AND s.status='VALIDATED'
			  AND s.expires_at>$8 AND p.plan_revision=$5 AND p.version=$6 AND p.status='PLANNED'
			  AND p.failure_code='' AND p.checkout_validation_revision=$3
			  AND p.expires_at>$8 AND p.mode=$7 AND s.cart_id=$9 AND s.cart_version=$10
			  AND a.source_address_ref=$11 AND a.title_snapshot=$12 AND a.address_snapshot=$13
			  AND a.latitude_snapshot=$14 AND a.longitude_snapshot=$15
			  AND a.description_snapshot=$16 AND a.snapshotted_at=$17
		)`, quote.CheckoutSessionID, quote.CustomerID, quote.CheckoutValidationRevision,
		quote.FulfilmentPlanID, quote.FulfilmentPlanRevision, quote.FulfilmentPlanVersion,
		quote.SelectedFulfilmentMode, now, quote.CartID, quote.CartVersion,
		quote.Address.SourceAddressRef, quote.Address.Title, quote.Address.Address,
		quote.Address.Latitude, quote.Address.Longitude, quote.Address.Description,
		quote.Address.SnapshottedAt).Scan(&valid)
	if err != nil || !valid {
		if err != nil {
			return Quote{}, err
		}
		return Quote{}, ErrQuoteConflict
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(quote_version),0)+1 FROM commerce_quotes WHERE checkout_session_id=$1`, quote.CheckoutSessionID).Scan(&quote.QuoteVersion); err != nil {
		return Quote{}, err
	}
	quote.ID = "cq_" + uuid.NewString()
	quote.QuoteFingerprint, err = QuoteFingerprint(quote)
	if err != nil {
		return Quote{}, err
	}
	quote.CreatedAt = now
	addressJSON, _ := json.Marshal(quote.Address)
	fundingJSON, _ := json.Marshal(quote.FundingBreakdown)
	disclosureJSON, _ := json.Marshal(quote.PriceDisclosure)
	_, err = tx.Exec(ctx, `
		INSERT INTO commerce_quotes(
			id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,
			fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,
			material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,
			product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,
			discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,
			funding_breakdown,price_disclosure_snapshot,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`,
		quote.ID, quote.CustomerID, quote.CheckoutSessionID, quote.CheckoutValidationRevision,
		quote.CartID, quote.CartVersion, quote.FulfilmentPlanID, quote.FulfilmentPlanRevision,
		quote.FulfilmentPlanVersion, quote.QuoteVersion, quote.MaterialFingerprint, quote.QuoteFingerprint,
		quote.Currency, quote.SelectedFulfilmentMode, addressJSON, quote.ExpiresAt,
		quote.ProductSubtotalMinor, quote.MaxProductSubtotalMinor, quote.DeliverySubtotalMinor,
		quote.FeeSubtotalMinor, quote.TaxSubtotalMinor, quote.DiscountTotalMinor, quote.SubsidyTotalMinor,
		quote.FinalPayableMinor, quote.QuotedTotalMinor, quote.MaxAuthorizedTotalMinor,
		fundingJSON, disclosureJSON, quote.CreatedAt)
	if err != nil {
		return Quote{}, err
	}
	if err = reservePromotionUsage(ctx, tx, quote, now); err != nil {
		return Quote{}, err
	}
	if err = insertQuoteChildren(ctx, tx, quote); err != nil {
		return Quote{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_quote_events(id,quote_id,customer_id,action,quote_version,quote_fingerprint,occurred_at) VALUES($1,$2,$3,'commerce.quote.created',$4,$5,$6)`,
		"cqe_"+uuid.NewString(), quote.ID, quote.CustomerID, quote.QuoteVersion, quote.QuoteFingerprint, now); err != nil {
		return Quote{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func reservePromotionUsage(ctx context.Context, tx pgx.Tx, quote Quote, now time.Time) error {
	seen := map[string]bool{}
	for _, adjustment := range quote.Adjustments {
		if adjustment.PromotionType == "" || seen[adjustment.SourceRef] {
			continue
		}
		seen[adjustment.SourceRef] = true
		var status string
		var startsAt, endsAt time.Time
		var version int64
		var sponsor, platformApprovalRef string
		var globalLimit, customerLimit *int64
		if err := tx.QueryRow(ctx, `SELECT status,starts_at,ends_at,version,global_usage_limit,per_customer_usage_limit,funding_sponsor,platform_approval_ref
			FROM commerce_promotion_campaigns WHERE id=$1 AND business_id=$2 FOR UPDATE`, adjustment.SourceRef, adjustment.BusinessID).
			Scan(&status, &startsAt, &endsAt, &version, &globalLimit, &customerLimit, &sponsor, &platformApprovalRef); err != nil {
			return ErrInvalidAdjustment
		}
		if (status != "ACTIVE" && status != "SCHEDULED") || now.Before(startsAt) || !now.Before(endsAt) || version != adjustment.CampaignVersion {
			return ErrInvalidAdjustment
		}
		if (sponsor == "VENDOR" && adjustment.FundingSource != FundingVendor) ||
			(sponsor == "PLATFORM" && adjustment.FundingSource != FundingPlatform) ||
			((sponsor == "PLATFORM" || sponsor == "COMBINED") && (platformApprovalRef == "" || adjustment.PlatformApprovalRef != platformApprovalRef)) {
			return ErrInvalidAdjustment
		}
		var globalUsed, customerUsed int64
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE customer_id=$2)
			FROM commerce_promotion_redemptions WHERE campaign_id=$1 AND status IN ('RESERVED','CONSUMED')`, adjustment.SourceRef, quote.CustomerID).
			Scan(&globalUsed, &customerUsed); err != nil {
			return err
		}
		if (globalLimit != nil && globalUsed >= *globalLimit) || (customerLimit != nil && customerUsed >= *customerLimit) {
			return ErrInvalidAdjustment
		}
		var couponID *string
		if adjustment.CouponDigest != "" {
			var id string
			var couponLimit, couponCustomerLimit *int64
			if err := tx.QueryRow(ctx, `SELECT id,usage_limit,per_customer_usage_limit FROM commerce_promotion_coupons
				WHERE campaign_id=$1 AND business_id=$2 AND code_digest=$3 AND enabled AND revoked_at IS NULL FOR UPDATE`,
				adjustment.SourceRef, adjustment.BusinessID, adjustment.CouponDigest).Scan(&id, &couponLimit, &couponCustomerLimit); err != nil {
				return ErrInvalidAdjustment
			}
			var used, customerCouponUsed int64
			if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE customer_id=$2) FROM commerce_promotion_redemptions
				WHERE coupon_id=$1 AND status IN ('RESERVED','CONSUMED')`, id, quote.CustomerID).Scan(&used, &customerCouponUsed); err != nil {
				return err
			}
			if (couponLimit != nil && used >= *couponLimit) || (couponCustomerLimit != nil && customerCouponUsed >= *couponCustomerLimit) {
				return ErrInvalidAdjustment
			}
			couponID = &id
		}
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_promotion_redemptions
			(id,campaign_id,coupon_id,business_id,customer_id,quote_id,idempotency_key,status)
			VALUES($1,$2,$3,$4,$5,$6,$7,'RESERVED') ON CONFLICT (campaign_id,quote_id) DO NOTHING`,
			"redemption_"+uuid.NewString(), adjustment.SourceRef, couponID, adjustment.BusinessID, quote.CustomerID, quote.ID, "quote:"+quote.ID+":campaign:"+adjustment.SourceRef); err != nil {
			return err
		}
	}
	return nil
}

func insertQuoteChildren(ctx context.Context, tx pgx.Tx, quote Quote) error {
	for _, group := range quote.Groups {
		funding, _ := json.Marshal(group.DeliveryFunding)
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_quote_groups(id,quote_id,customer_id,checkout_group_id,business_id,branch_id,product_subtotal_minor,max_product_subtotal_minor,inventory_reservation_ref,inventory_reservation_source_ref,logistics_plan_group_ref,logistics_mission_ref,delivery_funding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			"cqg_"+uuid.NewString(), quote.ID, quote.CustomerID, group.CheckoutGroupID, group.BusinessID, group.BranchID,
			group.ProductSubtotalMinor, group.MaxProductSubtotalMinor, group.InventoryReservationRef,
			group.InventoryReservationSourceRef, group.LogisticsPlanGroupRef, group.LogisticsMissionRef, funding); err != nil {
			return err
		}
	}
	for _, line := range quote.Lines {
		snapshot, _ := json.Marshal(line)
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_quote_lines(id,quote_id,checkout_line_id,checkout_group_id,cart_item_id,snapshot,vendor_online_unit_price_minor,price_reference,catalogue_revision,inventory_revision,quoted_total_minor,max_authorized_total_minor,price_context,store_price_parity_claimed) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'VENDOR_ONLINE_PRICE',false)`,
			"cql_"+uuid.NewString(), quote.ID, line.CheckoutLineID, line.CheckoutGroupID, line.CartItemID, snapshot,
			line.VendorOnlineUnitPriceMinor, line.PriceReference, line.CatalogueRevision, line.InventoryRevision,
			line.QuotedTotalMinor, line.MaxAuthorizedTotalMinor); err != nil {
			return err
		}
	}
	for _, mission := range quote.Missions {
		snapshot, _ := json.Marshal(mission)
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_quote_missions(id,quote_id,fulfilment_mission_id,snapshot) VALUES($1,$2,$3,$4)`,
			"cqm_"+uuid.NewString(), quote.ID, mission.MissionID, snapshot); err != nil {
			return err
		}
	}
	for _, adjustment := range quote.Adjustments {
		evidence := adjustment.EligibilityEvidence
		if len(evidence) == 0 {
			evidence = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_quote_adjustments(id,quote_id,adjustment_id,adjustment_type,scope,funding_source,source_ref,description,amount_minor,
			business_id,branch_id,promotion_type,campaign_version,eligibility_evidence,coupon_digest,platform_approval_ref,calculation_base_minor,delivery_discount_minor,stack_priority,applied_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
			"cqa_"+uuid.NewString(), quote.ID, adjustment.ID, adjustment.Type, adjustment.Scope,
			adjustment.FundingSource, adjustment.SourceRef, adjustment.Description, adjustment.AmountMinor,
			adjustment.BusinessID, adjustment.BranchID, adjustment.PromotionType, adjustment.CampaignVersion,
			evidence, adjustment.CouponDigest, adjustment.PlatformApprovalRef, adjustment.CalculationBaseMinor,
			adjustment.DeliveryDiscountMinor, adjustment.StackPriority, adjustment.AppliedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Get(ctx context.Context, customerID, quoteID string, now time.Time) (Quote, error) {
	if s == nil || s.db == nil {
		return Quote{}, ErrDependencyMissing
	}
	return loadQuote(ctx, s.db, customerID, quoteID, now)
}

func loadQuote(ctx context.Context, q queryer, customerID, quoteID string, now time.Time) (Quote, error) {
	var quote Quote
	var addressJSON, fundingJSON, disclosureJSON []byte
	var approvedAt *time.Time
	err := q.QueryRow(ctx, `
		SELECT q.id,q.quote_version,q.quote_fingerprint,q.material_fingerprint,q.customer_id,
		       q.checkout_session_id,q.checkout_validation_revision,q.cart_id,q.cart_version,
		       q.fulfilment_plan_id,q.fulfilment_plan_revision,q.fulfilment_plan_version,q.currency,
		       q.selected_fulfilment_mode,q.address_snapshot,q.expires_at,q.product_subtotal_minor,
		       q.max_product_subtotal_minor,q.delivery_subtotal_minor,q.fee_subtotal_minor,q.tax_subtotal_minor,
		       q.discount_total_minor,q.subsidy_total_minor,q.final_payable_minor,q.quoted_total_minor,
		       q.max_authorized_total_minor,q.funding_breakdown,q.price_disclosure_snapshot,q.created_at,
		       a.approved_at
		  FROM commerce_quotes q LEFT JOIN commerce_quote_approvals a ON a.quote_id=q.id
		 WHERE q.id=$1 AND q.customer_id=$2`, quoteID, customerID).Scan(
		&quote.ID, &quote.QuoteVersion, &quote.QuoteFingerprint, &quote.MaterialFingerprint, &quote.CustomerID,
		&quote.CheckoutSessionID, &quote.CheckoutValidationRevision, &quote.CartID, &quote.CartVersion,
		&quote.FulfilmentPlanID, &quote.FulfilmentPlanRevision, &quote.FulfilmentPlanVersion, &quote.Currency,
		&quote.SelectedFulfilmentMode, &addressJSON, &quote.ExpiresAt, &quote.ProductSubtotalMinor,
		&quote.MaxProductSubtotalMinor, &quote.DeliverySubtotalMinor, &quote.FeeSubtotalMinor, &quote.TaxSubtotalMinor,
		&quote.DiscountTotalMinor, &quote.SubsidyTotalMinor, &quote.FinalPayableMinor, &quote.QuotedTotalMinor,
		&quote.MaxAuthorizedTotalMinor, &fundingJSON, &disclosureJSON, &quote.CreatedAt, &approvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return Quote{}, err
	}
	if err = json.Unmarshal(addressJSON, &quote.Address); err != nil {
		return Quote{}, err
	}
	if err = json.Unmarshal(fundingJSON, &quote.FundingBreakdown); err != nil {
		return Quote{}, err
	}
	if err = json.Unmarshal(disclosureJSON, &quote.PriceDisclosure); err != nil {
		return Quote{}, err
	}
	quote.ApprovedAt = approvedAt
	quote.Expired = !quote.ExpiresAt.After(now)
	quote.State = StateQuoted
	if quote.Expired {
		quote.State = StateExpired
	} else if approvedAt != nil {
		quote.State = StateApproved
	}
	if quote.Groups, err = loadQuoteGroups(ctx, q, quote.ID); err != nil {
		return Quote{}, err
	}
	if quote.Lines, err = loadQuoteLines(ctx, q, quote.ID); err != nil {
		return Quote{}, err
	}
	if quote.Missions, err = loadQuoteMissions(ctx, q, quote.ID); err != nil {
		return Quote{}, err
	}
	if quote.Adjustments, err = loadQuoteAdjustments(ctx, q, quote.ID); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func loadQuoteGroups(ctx context.Context, q queryer, quoteID string) ([]GroupSnapshot, error) {
	rows, err := q.Query(ctx, `SELECT checkout_group_id,business_id,branch_id,product_subtotal_minor,max_product_subtotal_minor,inventory_reservation_ref,inventory_reservation_source_ref,logistics_plan_group_ref,logistics_mission_ref,delivery_funding FROM commerce_quote_groups WHERE quote_id=$1 ORDER BY checkout_group_id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []GroupSnapshot
	for rows.Next() {
		var value GroupSnapshot
		var funding []byte
		if err = rows.Scan(&value.CheckoutGroupID, &value.BusinessID, &value.BranchID, &value.ProductSubtotalMinor, &value.MaxProductSubtotalMinor, &value.InventoryReservationRef, &value.InventoryReservationSourceRef, &value.LogisticsPlanGroupRef, &value.LogisticsMissionRef, &funding); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(funding, &value.DeliveryFunding); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func loadQuoteLines(ctx context.Context, q queryer, quoteID string) ([]LineSnapshot, error) {
	rows, err := q.Query(ctx, `SELECT snapshot FROM commerce_quote_lines WHERE quote_id=$1 ORDER BY cart_item_id,id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []LineSnapshot
	for rows.Next() {
		var raw []byte
		var value LineSnapshot
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func loadQuoteMissions(ctx context.Context, q queryer, quoteID string) ([]MissionSnapshot, error) {
	rows, err := q.Query(ctx, `SELECT snapshot FROM commerce_quote_missions WHERE quote_id=$1 ORDER BY fulfilment_mission_id,id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []MissionSnapshot
	for rows.Next() {
		var raw []byte
		var value MissionSnapshot
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func loadQuoteAdjustments(ctx context.Context, q queryer, quoteID string) ([]Adjustment, error) {
	rows, err := q.Query(ctx, `SELECT adjustment_id,adjustment_type,scope,funding_source,source_ref,description,amount_minor,
		business_id,branch_id,promotion_type,campaign_version,eligibility_evidence,coupon_digest,platform_approval_ref,calculation_base_minor,delivery_discount_minor,stack_priority,applied_at
		FROM commerce_quote_adjustments WHERE quote_id=$1 ORDER BY adjustment_id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Adjustment
	for rows.Next() {
		var value Adjustment
		if err = rows.Scan(&value.ID, &value.Type, &value.Scope, &value.FundingSource, &value.SourceRef, &value.Description, &value.AmountMinor,
			&value.BusinessID, &value.BranchID, &value.PromotionType, &value.CampaignVersion, &value.EligibilityEvidence, &value.CouponDigest,
			&value.PlatformApprovalRef, &value.CalculationBaseMinor, &value.DeliveryDiscountMinor, &value.StackPriority, &value.AppliedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) Approve(ctx context.Context, customerID, quoteID, fingerprint string, now time.Time) (Quote, error) {
	if s == nil || s.db == nil {
		return Quote{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Quote{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var storedFingerprint, sessionID, planID, cartID, mode string
	var quoteVersion, checkoutRevision, planRevision, planVersion, cartVersion int64
	var expiresAt time.Time
	var addressJSON []byte
	err = tx.QueryRow(ctx, `SELECT quote_fingerprint,quote_version,checkout_session_id,checkout_validation_revision,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,expires_at,cart_id,cart_version,selected_fulfilment_mode,address_snapshot FROM commerce_quotes WHERE id=$1 AND customer_id=$2 FOR UPDATE`, quoteID, customerID).Scan(&storedFingerprint, &quoteVersion, &sessionID, &checkoutRevision, &planID, &planRevision, &planVersion, &expiresAt, &cartID, &cartVersion, &mode, &addressJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return Quote{}, err
	}
	if storedFingerprint != fingerprint {
		return Quote{}, ErrFingerprintMismatch
	}
	if !expiresAt.After(now) {
		return Quote{}, ErrQuoteExpired
	}
	var address AddressSnapshot
	if err = json.Unmarshal(addressJSON, &address); err != nil {
		return Quote{}, err
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM commerce_checkout_sessions s
		JOIN commerce_cart_addresses a ON a.cart_id=s.cart_id AND a.source_address_ref=s.address_ref
		JOIN commerce_fulfilment_plans p ON p.id=$4 AND p.checkout_session_id=s.id AND p.customer_id=s.customer_id
		WHERE s.id=$1 AND s.customer_id=$2 AND s.validation_revision=$3 AND s.status='VALIDATED'
		  AND s.expires_at>$7 AND s.cart_id=$8 AND s.cart_version=$9
		  AND p.checkout_validation_revision=$3 AND p.plan_revision=$5 AND p.version=$6
		  AND p.status='PLANNED' AND p.failure_code='' AND p.expires_at>$7 AND p.mode=$10
		  AND a.source_address_ref=$11 AND a.title_snapshot=$12 AND a.address_snapshot=$13
		  AND a.latitude_snapshot=$14 AND a.longitude_snapshot=$15
		  AND a.description_snapshot=$16 AND a.snapshotted_at=$17
	)`, sessionID, customerID, checkoutRevision, planID, planRevision, planVersion, now,
		cartID, cartVersion, mode, address.SourceAddressRef, address.Title, address.Address,
		address.Latitude, address.Longitude, address.Description, address.SnapshottedAt).Scan(&valid); err != nil {
		return Quote{}, err
	}
	if !valid {
		return Quote{}, ErrQuoteConflict
	}
	storedQuote, err := loadQuote(ctx, tx, customerID, quoteID, now)
	if err != nil {
		return Quote{}, err
	}
	currentSource, err := loadSource(ctx, tx, customerID, sessionID, planID, now)
	if err != nil {
		if errors.Is(err, ErrSourceNotQuotable) {
			return Quote{}, ErrQuoteConflict
		}
		return Quote{}, err
	}
	if currentSource.CheckoutExpiresAt.Before(storedQuote.ExpiresAt) || currentSource.FulfilmentPlanExpiresAt.Before(storedQuote.ExpiresAt) {
		return Quote{}, ErrQuoteConflict
	}
	currentSourceFingerprint, err := SourceFingerprint(currentSource)
	if err != nil {
		return Quote{}, err
	}
	storedSourceFingerprint, err := QuoteSourceFingerprint(storedQuote)
	if err != nil {
		return Quote{}, err
	}
	if currentSourceFingerprint != storedSourceFingerprint {
		return Quote{}, ErrQuoteConflict
	}
	tag, err := tx.Exec(ctx, `INSERT INTO commerce_quote_approvals(id,quote_id,customer_id,quote_fingerprint,approved_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT (quote_id) DO NOTHING`, "cqp_"+uuid.NewString(), quoteID, customerID, fingerprint, now)
	if err != nil {
		return Quote{}, err
	}
	if tag.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, `INSERT INTO commerce_quote_events(id,quote_id,customer_id,action,quote_version,quote_fingerprint,occurred_at) VALUES($1,$2,$3,'commerce.quote.approved',$4,$5,$6)`, "cqe_"+uuid.NewString(), quoteID, customerID, quoteVersion, fingerprint, now); err != nil {
			return Quote{}, err
		}
		if err = s.outbox.Publish(ctx, tx, "commerce.quote.locked", "CommerceQuote", quoteID, quoteVersion, "", quoteID, quoteID, map[string]any{
			"quote_ref":                      quoteID,
			"quote_version":                  quoteVersion,
			"quote_fingerprint":              fingerprint,
			"expires_at":                     storedQuote.ExpiresAt.UTC(),
			"currency":                       storedQuote.Currency,
			"final_total_minor":              storedQuote.FinalPayableMinor,
			"maximum_authorized_total_minor": storedQuote.MaxAuthorizedTotalMinor,
		}); err != nil {
			return Quote{}, err
		}
	}
	quote, err := loadQuote(ctx, tx, customerID, quoteID, now)
	if err != nil {
		return Quote{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

var _ StoreAuthority = (*Store)(nil)
