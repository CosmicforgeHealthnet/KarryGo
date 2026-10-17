package shop

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type InventoryAvailability struct {
	BranchRef           string    `json:"branch_ref"`
	ListingRef          string    `json:"listing_ref"`
	VariantRef          string    `json:"variant_ref"`
	StockBasisRef       string    `json:"stock_basis_ref"`
	PhysicalOnHand      int64     `json:"physical_on_hand"`
	PhysicalAvailable   int64     `json:"physical_available"`
	ReservedQuantity    int64     `json:"reserved_quantity"`
	QuarantinedQuantity int64     `json:"quarantined_quantity"`
	InTransitQuantity   int64     `json:"in_transit_quantity"`
	BackorderQuantity   int64     `json:"backorder_quantity"`
	InventoryRevision   int64     `json:"inventory_revision"`
	EvaluatedAt         time.Time `json:"evaluated_at"`
}

type SellabilityResult struct {
	Sellable              bool                  `json:"sellable"`
	ReasonCode            string                `json:"reason_code"`
	Availability          InventoryAvailability `json:"availability"`
	PreorderBackorderMode string                `json:"preorder_backorder_mode"`
	CatalogueRevision     int64                 `json:"catalogue_revision"`
	InventoryRevision     int64                 `json:"inventory_revision"`
	EvaluatedAt           time.Time             `json:"evaluated_at"`
}

type DiscoverySellUnit struct {
	ID                   string `json:"id"`
	DisplayName          string `json:"display_name"`
	MeasureRef           string `json:"measure_ref"`
	MeasurementMode      string `json:"measurement_mode"`
	RetailEnabled        bool   `json:"retail_enabled"`
	WholesaleEnabled     bool   `json:"wholesale_enabled"`
	RetailPriceMinor     *int64 `json:"retail_price_minor,omitempty"`
	WholesalePriceMinor  *int64 `json:"wholesale_price_minor,omitempty"`
	MinimumOrderQuantity int64  `json:"minimum_order_quantity"`
	OrderMultiple        int64  `json:"order_multiple"`
}

type DiscoveryFreshOption struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default"`
}

type DiscoveryFreshAttribute struct {
	Type     string                 `json:"type"`
	Required bool                   `json:"required"`
	Options  []DiscoveryFreshOption `json:"options"`
}

type DiscoveryItem struct {
	CategoryRef             string                    `json:"category_ref"`
	CategoryName            string                    `json:"category_name"`
	ProductRef              string                    `json:"product_ref"`
	ProductName             string                    `json:"product_name"`
	Brand                   string                    `json:"brand,omitempty"`
	ProductClass            string                    `json:"product_class"`
	VariantRef              string                    `json:"variant_ref"`
	VariantName             string                    `json:"variant_name"`
	TemperatureZone         string                    `json:"temperature_zone"`
	ListingRef              string                    `json:"listing_ref"`
	OperatingModel          string                    `json:"operating_model,omitempty"`
	RetailEnabled           bool                      `json:"retail_enabled"`
	WholesaleEnabled        bool                      `json:"wholesale_enabled"`
	StartingPriceMinor      *int64                    `json:"starting_price_minor"`
	Sellability             SellabilityResult         `json:"sellability"`
	SellUnits               []DiscoverySellUnit       `json:"sell_units"`
	HandlingFlags           []string                  `json:"handling_flags"`
	FreshAttributes         []DiscoveryFreshAttribute `json:"fresh_attributes"`
	AutoHideWhenUnavailable bool                      `json:"-"`
}

type DiscoveryOptions struct {
	Search, CategoryID, ProductID, Brand, ProductClass, TemperatureZone string
	OperatingModel                                                      string
	InStockOnly, WholesaleOnly                                          bool
	Limit, Offset                                                       int
}

type InventoryMovement struct {
	ID                string    `json:"id"`
	BranchRef         string    `json:"branch_ref"`
	StockBasisRef     string    `json:"stock_basis_ref"`
	VariantRef        string    `json:"variant_ref"`
	LotRef            *string   `json:"lot_ref"`
	MovementType      string    `json:"movement_type"`
	Quantity          int64     `json:"quantity"`
	OnHandDelta       int64     `json:"on_hand_delta"`
	ReservedDelta     int64     `json:"reserved_delta"`
	QuarantinedDelta  int64     `json:"quarantined_delta"`
	InTransitDelta    int64     `json:"in_transit_delta"`
	SourceType        string    `json:"source_type"`
	SourceRef         string    `json:"source_ref"`
	ReasonCode        *string   `json:"reason_code"`
	OccurredAt        time.Time `json:"occurred_at"`
	InventoryRevision int64     `json:"inventory_revision"`
}

type MovementListOptions struct {
	VariantID, MovementType, SourceRef, LotID string
	From, To                                  *time.Time
	Limit, Offset                             int
}

func (s *Service) ListInventoryBalances(ctx context.Context, actor Actor, scope Scope, branchID string) ([]InventoryAvailability, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, err
	}
	query := strings.Replace(effectiveAvailabilityQuery(), "b.variant_id=$3", "TRUE", 1)
	rows, err := s.db.Query(ctx, query, actor.VendorID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InventoryAvailability{}
	for rows.Next() {
		var item InventoryAvailability
		if err = rows.Scan(&item.BranchRef, &item.ListingRef, &item.VariantRef, &item.StockBasisRef, &item.PhysicalOnHand, &item.PhysicalAvailable, &item.ReservedQuantity, &item.QuarantinedQuantity, &item.InTransitQuantity, &item.BackorderQuantity, &item.InventoryRevision); err != nil {
			return nil, err
		}
		item.EvaluatedAt = s.now()
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListInventoryMovements(ctx context.Context, actor Actor, scope Scope, branchID string, options MovementListOptions) ([]InventoryMovement, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if options.Limit <= 0 {
		options.Limit = 50
	}
	if options.Limit > 100 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,on_hand_delta,reserved_delta,quarantined_delta,in_transit_delta,source_type,source_ref,reason_code,occurred_at,inventory_revision FROM shop_inventory_movements WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR variant_id=$3) AND ($4='' OR movement_type=$4) AND ($5='' OR source_ref=$5) AND ($6='' OR lot_id=$6) AND ($7::timestamptz IS NULL OR occurred_at>=$7) AND ($8::timestamptz IS NULL OR occurred_at<$8) ORDER BY occurred_at DESC,id DESC LIMIT $9 OFFSET $10`, actor.VendorID, branchID, options.VariantID, options.MovementType, options.SourceRef, options.LotID, options.From, options.To, options.Limit+1, options.Offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []InventoryMovement{}
	for rows.Next() {
		var item InventoryMovement
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.StockBasisRef, &item.VariantRef, &item.LotRef, &item.MovementType, &item.Quantity, &item.OnHandDelta, &item.ReservedDelta, &item.QuarantinedDelta, &item.InTransitDelta, &item.SourceType, &item.SourceRef, &item.ReasonCode, &item.OccurredAt, &item.InventoryRevision); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > options.Limit
	if more {
		out = out[:options.Limit]
	}
	return out, more, nil
}

func (s *Service) GetInventoryReservation(ctx context.Context, actor Actor, scope Scope, id string) (InventoryReservation, error) {
	out, err := loadReservation(ctx, s.db, actor.VendorID, id)
	if err != nil {
		return out, err
	}
	for _, line := range out.Lines {
		if err = s.requireBranchScope(scope, line.BranchRef); err != nil {
			return InventoryReservation{}, err
		}
	}
	return out, nil
}

func (s *Service) GetReceipt(ctx context.Context, actor Actor, scope Scope, id string) (InventoryReceipt, error) {
	var out InventoryReceipt
	err := s.db.QueryRow(ctx, `SELECT id,branch_id,COALESCE(source_ref,''),received_at,status,posted_at,created_at,updated_at,version FROM shop_inventory_receipts WHERE id=$1 AND business_id=$2`, id, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.SourceRef, &out.ReceivedAt, &out.Status, &out.PostedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
	if err != nil {
		return out, mapDB(err, "Receipt", CodeStockBasisNotFound)
	}
	if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
		return InventoryReceipt{}, err
	}
	if err = loadReceiptLines(ctx, s.db, actor.VendorID, &out); err != nil {
		return InventoryReceipt{}, err
	}
	return out, nil
}

func loadReceiptLines(ctx context.Context, q querier, businessID string, out *InventoryReceipt) error {
	rows, err := q.Query(ctx, `SELECT id,stock_basis_id,variant_id,quantity,COALESCE(lot_number,''),COALESCE(batch_number,''),manufactured_at,best_before_at,expires_at,COALESCE(lot_id,'')
		FROM shop_inventory_receipt_lines WHERE business_id=$1 AND receipt_id=$2 ORDER BY id`, businessID, out.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out.Lines = []InventoryReceiptLine{}
	for rows.Next() {
		var line InventoryReceiptLine
		if err = rows.Scan(&line.ID, &line.StockBasisRef, &line.VariantRef, &line.Quantity, &line.LotNumber, &line.BatchNumber, &line.ManufacturedAt, &line.BestBeforeAt, &line.ExpiresAt, &line.LotRef); err != nil {
			return err
		}
		out.Lines = append(out.Lines, line)
	}
	return rows.Err()
}

func (s *Service) InventoryRevision(ctx context.Context, actor Actor, scope Scope, branchID string) (int64, time.Time, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return 0, time.Time{}, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return 0, time.Time{}, err
	}
	var revision int64
	var updated time.Time
	err := s.db.QueryRow(ctx, `SELECT revision,updated_at FROM shop_inventory_revisions WHERE business_id=$1 AND branch_id=$2`, actor.VendorID, branchID).Scan(&revision, &updated)
	if err == pgx.ErrNoRows {
		return 0, time.Time{}, nil
	}
	return revision, updated, err
}

func effectiveAvailabilityQuery() string {
	return `
	SELECT b.branch_id,s.listing_id,b.variant_id,b.stock_basis_id,b.on_hand_quantity,
	COALESCE(valid.sellable,0),COALESCE(active_res.reserved,0),b.quarantined_quantity,b.in_transit_quantity,b.backorder_quantity,
	COALESCE(r.revision,0)
	FROM shop_inventory_balances b
	JOIN shop_inventory_stock_bases s ON s.id=b.stock_basis_id AND s.business_id=b.business_id
	LEFT JOIN shop_inventory_revisions r ON r.business_id=b.business_id AND r.branch_id=b.branch_id
	LEFT JOIN LATERAL (
		SELECT COALESCE(sum(GREATEST(0,l.on_hand_quantity-l.quarantined_quantity-
			COALESCE((SELECT sum(a.quantity) FROM shop_inventory_reservation_allocations a
			JOIN shop_inventory_reservation_lines rl ON rl.id=a.reservation_line_id AND rl.business_id=a.business_id
			JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
			WHERE a.business_id=l.business_id AND a.lot_id=l.id AND a.status='HELD' AND rl.status='HELD'
			AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())),0))),0) sellable
		FROM shop_inventory_lots l WHERE l.business_id=b.business_id AND l.stock_basis_id=b.stock_basis_id
		AND l.on_hand_quantity>0 AND l.status='ACTIVE' AND (l.expires_at IS NULL OR l.expires_at>now())
	) valid ON true
	LEFT JOIN LATERAL (
		SELECT COALESCE(sum(a.quantity),0) reserved FROM shop_inventory_reservation_allocations a
		JOIN shop_inventory_reservation_lines rl ON rl.id=a.reservation_line_id AND rl.business_id=a.business_id
		JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
		WHERE a.business_id=b.business_id AND rl.stock_basis_id=b.stock_basis_id AND a.status='HELD' AND rl.status='HELD'
		AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())
	) active_res ON true
	WHERE b.business_id=$1 AND b.branch_id=$2 AND b.variant_id=$3`
}

// effectiveAvailabilitiesQuery resolves a bounded discovery page as one set.
// Keeping the target balances materialized prevents PostgreSQL's generic
// prepared-statement plan from re-running a lot scan once per variant at scale.
func effectiveAvailabilitiesQuery() string {
	return `
	WITH target AS MATERIALIZED (
		SELECT b.branch_id,s.listing_id,b.variant_id,b.stock_basis_id,b.on_hand_quantity,
			b.quarantined_quantity,b.in_transit_quantity,b.backorder_quantity
		FROM shop_inventory_balances b
		JOIN shop_inventory_stock_bases s ON s.id=b.stock_basis_id AND s.business_id=b.business_id
		WHERE b.business_id=$1 AND b.branch_id=$2 AND b.variant_id=ANY($3)
	), active_allocations AS MATERIALIZED (
		SELECT rl.stock_basis_id,a.lot_id,sum(a.quantity) quantity
		FROM shop_inventory_reservation_allocations a
		JOIN shop_inventory_reservation_lines rl ON rl.id=a.reservation_line_id AND rl.business_id=a.business_id
		JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
		JOIN target t ON t.stock_basis_id=rl.stock_basis_id
		WHERE a.business_id=$1 AND a.status='HELD' AND rl.status='HELD'
			AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())
		GROUP BY rl.stock_basis_id,a.lot_id
	), valid_lots AS (
		SELECT l.stock_basis_id,
			COALESCE(sum(GREATEST(0,l.on_hand_quantity-l.quarantined_quantity-COALESCE(a.quantity,0))),0) sellable
		FROM shop_inventory_lots l
		JOIN target t ON t.stock_basis_id=l.stock_basis_id
		LEFT JOIN active_allocations a ON a.stock_basis_id=l.stock_basis_id AND a.lot_id=l.id
		WHERE l.business_id=$1 AND l.on_hand_quantity>0 AND l.status='ACTIVE' AND (l.expires_at IS NULL OR l.expires_at>now())
		GROUP BY l.stock_basis_id
	), active_reserved AS (
		SELECT stock_basis_id,COALESCE(sum(quantity),0) reserved
		FROM active_allocations GROUP BY stock_basis_id
	)
	SELECT t.branch_id,t.listing_id,t.variant_id,t.stock_basis_id,t.on_hand_quantity,
		COALESCE(v.sellable,0),COALESCE(a.reserved,0),t.quarantined_quantity,t.in_transit_quantity,t.backorder_quantity,
		COALESCE(r.revision,0)
	FROM target t
	LEFT JOIN valid_lots v ON v.stock_basis_id=t.stock_basis_id
	LEFT JOIN active_reserved a ON a.stock_basis_id=t.stock_basis_id
	LEFT JOIN shop_inventory_revisions r ON r.business_id=$1 AND r.branch_id=t.branch_id`
}

func (s *Service) EffectiveAvailability(ctx context.Context, actor Actor, scope Scope, branchID, variantID string) (InventoryAvailability, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return InventoryAvailability{}, err
	}
	var out InventoryAvailability
	err := s.db.QueryRow(ctx, effectiveAvailabilityQuery(), actor.VendorID, branchID, variantID).Scan(&out.BranchRef, &out.ListingRef, &out.VariantRef, &out.StockBasisRef, &out.PhysicalOnHand, &out.PhysicalAvailable, &out.ReservedQuantity, &out.QuarantinedQuantity, &out.InTransitQuantity, &out.BackorderQuantity, &out.InventoryRevision)
	if err != nil {
		return out, mapDB(err, "Inventory balance", CodeStockBasisNotFound)
	}
	out.EvaluatedAt = s.now()
	return out, nil
}

func (s *Service) ResolveSellability(ctx context.Context, actor Actor, scope Scope, branchID, variantID, sellUnitID string, quantity int64, wholesale bool) (SellabilityResult, error) {
	result := SellabilityResult{ReasonCode: "OUT_OF_STOCK", EvaluatedAt: s.now()}
	if quantity <= 0 {
		return result, fieldError("quantity", "Quantity must be positive.")
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return result, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		result.ReasonCode = "SHOP_CAPABILITY_DISABLED"
		return result, err
	}
	var listingID, listingStatus, productStatus, variantStatus, unitStatus string
	var retailEnabled, wholesaleEnabled, unitEnabled bool
	var minimum, orderMultiple int64
	err := s.db.QueryRow(ctx, `SELECT l.id,l.status,p.status,v.status,su.status,l.retail_enabled,l.wholesale_enabled,lu.enabled,l.minimum_order_quantity,l.order_multiple
		FROM shop_branch_listings l JOIN shop_products p ON p.id=l.product_id AND p.business_id=l.business_id
		JOIN shop_variants v ON v.id=l.variant_id AND v.business_id=l.business_id
		JOIN shop_listing_sell_units lu ON lu.listing_id=l.id AND lu.business_id=l.business_id
		JOIN shop_sell_units su ON su.id=lu.sell_unit_id AND su.business_id=lu.business_id
		WHERE l.business_id=$1 AND l.branch_id=$2 AND l.variant_id=$3 AND su.id=$4`, actor.VendorID, branchID, variantID, sellUnitID).Scan(&listingID, &listingStatus, &productStatus, &variantStatus, &unitStatus, &retailEnabled, &wholesaleEnabled, &unitEnabled, &minimum, &orderMultiple)
	if err != nil {
		return result, mapDB(err, "Listing", CodeListingNotFound)
	}
	reason := "SELLABLE"
	switch {
	case productStatus != "ACTIVE":
		reason = "PRODUCT_INACTIVE"
	case variantStatus != "ACTIVE":
		reason = "VARIANT_INACTIVE"
	case listingStatus != "ACTIVE":
		reason = "LISTING_INACTIVE"
	case unitStatus != "ACTIVE" || !unitEnabled:
		reason = "SELL_UNIT_INACTIVE"
	case wholesale && !wholesaleEnabled:
		reason = "WHOLESALE_DISABLED"
	case !wholesale && !retailEnabled:
		reason = "RETAIL_DISABLED"
	case quantity < minimum:
		reason = "MOQ_NOT_MET"
	case quantity%orderMultiple != 0:
		reason = "ORDER_MULTIPLE_INVALID"
	}
	availability, availabilityErr := s.EffectiveAvailability(ctx, actor, scope, branchID, variantID)
	if availabilityErr != nil {
		result.ReasonCode = "INVENTORY_CONVERSION_UNRESOLVED"
		return result, nil
	}
	result.Availability = availability
	result.InventoryRevision = availability.InventoryRevision
	var policy string
	_ = s.db.QueryRow(ctx, `SELECT mode FROM shop_inventory_policies WHERE business_id=$1 AND branch_id=$2 AND variant_id=$3 AND status='ACTIVE'`, actor.VendorID, branchID, variantID).Scan(&policy)
	if policy == "" {
		policy = "NONE"
	}
	result.PreorderBackorderMode = policy
	if reason == "SELLABLE" && quantity > availability.PhysicalAvailable {
		if policy == "BACKORDER" {
			reason = "BACKORDER_AVAILABLE"
		} else if policy == "PREORDER" {
			reason = "PREORDER_AVAILABLE"
		} else {
			reason = "INSUFFICIENT_STOCK"
		}
	}
	result.Sellable = reason == "SELLABLE" || reason == "BACKORDER_AVAILABLE" || reason == "PREORDER_AVAILABLE"
	result.ReasonCode = reason
	catalogueRevision, _, _ := s.CatalogueRevision(ctx, actor, scope, branchID)
	result.CatalogueRevision = catalogueRevision
	_ = listingID
	return result, nil
}

func (s *Service) DiscoverCatalogue(ctx context.Context, actor Actor, scope Scope, branchID string, options DiscoveryOptions) ([]DiscoveryItem, bool, int64, int64, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, 0, 0, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, 0, 0, err
	}
	if options.Limit <= 0 {
		options.Limit = 50
	}
	if options.Limit > 100 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, false, 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Discovery always joins a branch-bounded listing page to catalogue
	// dimensions. Immediately after a large import PostgreSQL may not yet have
	// auto-analyzed those tables and can otherwise choose an O(n²) nested-loop
	// plan. Hash/merge joins keep this read bounded both before and after analyze.
	if _, err = tx.Exec(ctx, `SET LOCAL enable_nestloop = off`); err != nil {
		return nil, false, 0, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.name,p.id,p.name,COALESCE(p.brand,''),p.product_class,v.id,v.name,v.temperature_zone,l.id,COALESCE(l.operating_model,''),l.retail_enabled,l.wholesale_enabled,COALESCE(l.retail_price_override_minor,v.retail_price_minor),l.auto_hide_when_unavailable
		FROM shop_branch_listings l JOIN shop_variants v ON v.id=l.variant_id AND v.business_id=l.business_id
		JOIN shop_products p ON p.id=l.product_id AND p.business_id=l.business_id JOIN shop_categories c ON c.id=p.category_id AND c.business_id=p.business_id
		WHERE l.business_id=$1 AND l.branch_id=$2 AND l.status='ACTIVE' AND p.status='ACTIVE' AND v.status='ACTIVE' AND c.status='ACTIVE'
		AND ($3='' OR p.name ILIKE '%%'||$3||'%%' OR v.name ILIKE '%%'||$3||'%%' OR p.brand ILIKE '%%'||$3||'%%')
		AND ($4='' OR c.id=$4) AND ($5='' OR p.id=$5) AND ($6='' OR lower(p.brand)=lower($6)) AND ($7='' OR p.product_class=$7)
		AND ($8='' OR v.temperature_zone=$8) AND ($9='' OR l.operating_model=$9) AND (NOT $10 OR l.wholesale_enabled)
		ORDER BY c.position,p.name,v.position,v.id LIMIT $11 OFFSET $12`, actor.VendorID, branchID, strings.TrimSpace(options.Search), options.CategoryID, options.ProductID, options.Brand, options.ProductClass, options.TemperatureZone, options.OperatingModel, options.WholesaleOnly, options.Limit+1, options.Offset)
	if err != nil {
		return nil, false, 0, 0, err
	}
	defer rows.Close()
	var items []DiscoveryItem
	for rows.Next() {
		var item DiscoveryItem
		if err = rows.Scan(&item.CategoryRef, &item.CategoryName, &item.ProductRef, &item.ProductName, &item.Brand, &item.ProductClass, &item.VariantRef, &item.VariantName, &item.TemperatureZone, &item.ListingRef, &item.OperatingModel, &item.RetailEnabled, &item.WholesaleEnabled, &item.StartingPriceMinor, &item.AutoHideWhenUnavailable); err != nil {
			return nil, false, 0, 0, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, 0, 0, err
	}
	hasMore := len(items) > options.Limit
	if hasMore {
		items = items[:options.Limit]
	}
	variantIDs := make([]string, len(items))
	index := map[string]int{}
	for i := range items {
		variantIDs[i] = items[i].VariantRef
		index[items[i].VariantRef] = i
		items[i].SellUnits = []DiscoverySellUnit{}
		items[i].HandlingFlags = []string{}
		items[i].FreshAttributes = []DiscoveryFreshAttribute{}
		items[i].Sellability = SellabilityResult{ReasonCode: "INVENTORY_CONVERSION_UNRESOLVED", EvaluatedAt: s.now()}
	}
	if len(variantIDs) > 0 {
		availabilityRows, queryErr := tx.Query(ctx, effectiveAvailabilitiesQuery(), actor.VendorID, branchID, variantIDs)
		if queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		for availabilityRows.Next() {
			var availability InventoryAvailability
			if queryErr = availabilityRows.Scan(&availability.BranchRef, &availability.ListingRef, &availability.VariantRef, &availability.StockBasisRef, &availability.PhysicalOnHand, &availability.PhysicalAvailable, &availability.ReservedQuantity, &availability.QuarantinedQuantity, &availability.InTransitQuantity, &availability.BackorderQuantity, &availability.InventoryRevision); queryErr != nil {
				availabilityRows.Close()
				return nil, false, 0, 0, queryErr
			}
			availability.EvaluatedAt = s.now()
			i := index[availability.VariantRef]
			items[i].Sellability = SellabilityResult{Sellable: availability.PhysicalAvailable > 0, ReasonCode: "SELLABLE", Availability: availability, InventoryRevision: availability.InventoryRevision, EvaluatedAt: availability.EvaluatedAt}
			if !items[i].Sellability.Sellable {
				items[i].Sellability.ReasonCode = "OUT_OF_STOCK"
			}
		}
		availabilityRows.Close()
		if queryErr = availabilityRows.Err(); queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
	}
	if len(variantIDs) > 0 {
		policyRows, queryErr := tx.Query(ctx, `SELECT variant_id,mode FROM shop_inventory_policies WHERE business_id=$1 AND branch_id=$2 AND variant_id=ANY($3) AND status='ACTIVE'`, actor.VendorID, branchID, variantIDs)
		if queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		for policyRows.Next() {
			var variantID, mode string
			if queryErr = policyRows.Scan(&variantID, &mode); queryErr != nil {
				policyRows.Close()
				return nil, false, 0, 0, queryErr
			}
			item := &items[index[variantID]]
			item.Sellability.PreorderBackorderMode = mode
			if item.Sellability.Availability.PhysicalAvailable <= 0 && (mode == "PREORDER" || mode == "BACKORDER") {
				item.Sellability.Sellable = true
				item.Sellability.ReasonCode = mode + "_AVAILABLE"
			}
		}
		policyRows.Close()
		if queryErr = policyRows.Err(); queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
	}
	if len(variantIDs) > 0 {
		unitRows, queryErr := tx.Query(ctx, `SELECT su.variant_id,su.id,su.display_name,su.measure_definition_id,su.measurement_mode,
			su.retail_enabled AND l.retail_enabled,su.wholesale_enabled AND l.wholesale_enabled,
			COALESCE(lu.price_override_minor,su.price_minor,l.retail_price_override_minor,v.retail_price_minor),
			COALESCE(lu.wholesale_price_override_minor,su.wholesale_price_minor,l.wholesale_price_override_minor,v.wholesale_price_minor),
			GREATEST(su.minimum_order_quantity,l.minimum_order_quantity),GREATEST(su.order_multiple,l.order_multiple)
			FROM shop_listing_sell_units lu JOIN shop_sell_units su ON su.id=lu.sell_unit_id AND su.business_id=lu.business_id
			JOIN shop_branch_listings l ON l.id=lu.listing_id AND l.business_id=lu.business_id
			JOIN shop_variants v ON v.id=l.variant_id AND v.business_id=l.business_id
			WHERE l.business_id=$1 AND l.branch_id=$2 AND su.variant_id=ANY($3) AND lu.enabled AND su.status='ACTIVE'
			ORDER BY su.variant_id,lu.position,su.id`, actor.VendorID, branchID, variantIDs)
		if queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		for unitRows.Next() {
			var variant string
			var unit DiscoverySellUnit
			if queryErr = unitRows.Scan(&variant, &unit.ID, &unit.DisplayName, &unit.MeasureRef, &unit.MeasurementMode, &unit.RetailEnabled, &unit.WholesaleEnabled, &unit.RetailPriceMinor, &unit.WholesalePriceMinor, &unit.MinimumOrderQuantity, &unit.OrderMultiple); queryErr != nil {
				unitRows.Close()
				return nil, false, 0, 0, queryErr
			}
			i := index[variant]
			items[i].SellUnits = append(items[i].SellUnits, unit)
		}
		unitRows.Close()
		if queryErr = unitRows.Err(); queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		handlingRows, queryErr := tx.Query(ctx, `SELECT variant_id,handling_flag FROM shop_variant_handling_flags WHERE business_id=$1 AND variant_id=ANY($2) ORDER BY variant_id,handling_flag`, actor.VendorID, variantIDs)
		if queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		for handlingRows.Next() {
			var variantID, flag string
			if queryErr = handlingRows.Scan(&variantID, &flag); queryErr != nil {
				handlingRows.Close()
				return nil, false, 0, 0, queryErr
			}
			items[index[variantID]].HandlingFlags = append(items[index[variantID]].HandlingFlags, flag)
		}
		handlingRows.Close()
		if queryErr = handlingRows.Err(); queryErr != nil {
			return nil, false, 0, 0, queryErr
		}

		productIDs := make([]string, 0, len(items))
		productItems := map[string][]int{}
		for i := range items {
			if len(productItems[items[i].ProductRef]) == 0 {
				productIDs = append(productIDs, items[i].ProductRef)
			}
			productItems[items[i].ProductRef] = append(productItems[items[i].ProductRef], i)
		}
		freshRows, queryErr := tx.Query(ctx, `SELECT a.id,a.product_id,a.attribute_type,a.selection_required,o.option_value,o.display_name,o.is_default
			FROM shop_fresh_attributes a LEFT JOIN shop_fresh_attribute_options o ON o.attribute_id=a.id AND o.business_id=a.business_id AND o.status='ACTIVE'
			WHERE a.business_id=$1 AND a.product_id=ANY($2) AND a.status='ACTIVE' ORDER BY a.product_id,a.position,a.id,o.position,o.id`, actor.VendorID, productIDs)
		if queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		type freshEntry struct {
			product string
			value   DiscoveryFreshAttribute
		}
		var fresh []freshEntry
		freshIndex := map[string]int{}
		for freshRows.Next() {
			var attributeID, productID, attributeType string
			var required bool
			var optionValue, displayName *string
			var isDefault *bool
			if queryErr = freshRows.Scan(&attributeID, &productID, &attributeType, &required, &optionValue, &displayName, &isDefault); queryErr != nil {
				freshRows.Close()
				return nil, false, 0, 0, queryErr
			}
			position, exists := freshIndex[attributeID]
			if !exists {
				position = len(fresh)
				freshIndex[attributeID] = position
				fresh = append(fresh, freshEntry{product: productID, value: DiscoveryFreshAttribute{Type: attributeType, Required: required, Options: []DiscoveryFreshOption{}}})
			}
			if optionValue != nil {
				fresh[position].value.Options = append(fresh[position].value.Options, DiscoveryFreshOption{Value: *optionValue, DisplayName: *displayName, IsDefault: *isDefault})
			}
		}
		freshRows.Close()
		if queryErr = freshRows.Err(); queryErr != nil {
			return nil, false, 0, 0, queryErr
		}
		for _, entry := range fresh {
			for _, itemIndex := range productItems[entry.product] {
				items[itemIndex].FreshAttributes = append(items[itemIndex].FreshAttributes, entry.value)
			}
		}
	}
	if options.InStockOnly || len(items) > 0 {
		filtered := items[:0]
		for _, item := range items {
			if item.AutoHideWhenUnavailable && !item.Sellability.Sellable {
				continue
			}
			if !options.InStockOnly || item.Sellability.Availability.PhysicalAvailable > 0 {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	var catalogueRevision, inventoryRevision int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM shop_catalogue_revisions WHERE business_id=$1 AND branch_id=$2 AND domain='SHOP'),0),COALESCE((SELECT revision FROM shop_inventory_revisions WHERE business_id=$1 AND branch_id=$2),0)`, actor.VendorID, branchID).Scan(&catalogueRevision, &inventoryRevision); err != nil {
		return nil, false, 0, 0, err
	}
	for i := range items {
		items[i].Sellability.CatalogueRevision = catalogueRevision
		items[i].Sellability.InventoryRevision = inventoryRevision
		if items[i].Sellability.PreorderBackorderMode == "" {
			items[i].Sellability.PreorderBackorderMode = "NONE"
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, false, 0, 0, err
	}
	return items, hasMore, catalogueRevision, inventoryRevision, nil
}
