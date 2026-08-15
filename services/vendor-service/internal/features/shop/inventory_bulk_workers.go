package shop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

type ReorderRule struct {
	ID                 string `json:"id"`
	BranchRef          string `json:"branch_ref"`
	StockBasisRef      string `json:"stock_basis_ref"`
	VariantRef         string `json:"variant_ref"`
	MinimumQuantity    int64  `json:"minimum_quantity"`
	TargetQuantity     int64  `json:"target_quantity"`
	RecommendedReorder int64  `json:"recommended_reorder_quantity"`
	ExpiringSoonHours  *int   `json:"expiring_soon_hours"`
	Status             string `json:"status"`
	Version            int64  `json:"version"`
}

func (s *Service) SetReorderRule(ctx context.Context, actor Actor, scope Scope, basisID string, minimum, target int64, expiringHours *int) (ReorderRule, error) {
	if err := s.requireWrites(); err != nil {
		return ReorderRule{}, err
	}
	if minimum < 0 || target <= minimum || target > MaxInventoryQuantityPerLine {
		return ReorderRule{}, fieldError("target_quantity", "Target must be greater than the non-negative minimum.")
	}
	var out ReorderRule
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		branch, variant, _, err := lockInventoryBalance(ctx, tx, actor.VendorID, basisID)
		if err != nil {
			return err
		}
		if err = s.requireBranchScope(scope, branch); err != nil {
			return err
		}
		id := newID("sirr_")
		err = tx.QueryRow(ctx, `INSERT INTO shop_inventory_reorder_rules(id,business_id,branch_id,stock_basis_id,variant_id,minimum_quantity,target_quantity,expiring_soon_hours) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(business_id,branch_id,variant_id) DO UPDATE SET minimum_quantity=EXCLUDED.minimum_quantity,target_quantity=EXCLUDED.target_quantity,expiring_soon_hours=EXCLUDED.expiring_soon_hours,status='ACTIVE',updated_at=now(),version=shop_inventory_reorder_rules.version+1 RETURNING id,branch_id,stock_basis_id,variant_id,minimum_quantity,target_quantity,expiring_soon_hours,status,version`, id, actor.VendorID, branch, basisID, variant, minimum, target, expiringHours).Scan(&out.ID, &out.BranchRef, &out.StockBasisRef, &out.VariantRef, &out.MinimumQuantity, &out.TargetQuantity, &out.ExpiringSoonHours, &out.Status, &out.Version)
		if err != nil {
			return err
		}
		var available int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(GREATEST(0,l.on_hand_quantity-l.quarantined_quantity-
			COALESCE((SELECT sum(a.quantity) FROM shop_inventory_reservation_allocations a
			JOIN shop_inventory_reservation_lines rl ON rl.id=a.reservation_line_id AND rl.business_id=a.business_id
			JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
			WHERE a.business_id=l.business_id AND a.lot_id=l.id AND a.status='HELD' AND rl.status='HELD'
			AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())),0))),0)
			FROM shop_inventory_lots l WHERE l.business_id=$1 AND l.stock_basis_id=$2 AND l.status='ACTIVE' AND (l.expires_at IS NULL OR l.expires_at>now())`, actor.VendorID, basisID).Scan(&available); err != nil {
			return err
		}
		if available < target {
			out.RecommendedReorder = target - available
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.reorder_rule.changed", eventType: "shop.inventory.reorder_rule.changed", aggregateType: "shop_inventory_reorder_rule", aggregateID: out.ID, branchID: branch, version: out.Version, after: out, payload: out})
	})
	return out, err
}

// GenerateInventoryAlerts is an idempotent bounded worker pass. Alerts aid
// operations; synchronous sellability remains authoritative without it.
func (s *Service) GenerateInventoryAlerts(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `WITH candidates AS (
		SELECT b.business_id,b.branch_id,b.stock_basis_id,
			CASE WHEN a.available<=0 THEN 'OUT_OF_STOCK' ELSE 'LOW_STOCK' END alert_type,
			CASE WHEN a.available<=0 THEN 'CRITICAL' ELSE 'WARNING' END severity,
			'quantity:'||b.stock_basis_id dedupe_key,
			jsonb_build_object('available',a.available,'minimum',r.minimum_quantity,'target',r.target_quantity) details,
			b.updated_at sort_at
		FROM shop_inventory_balances b
		JOIN shop_inventory_reorder_rules r ON r.business_id=b.business_id AND r.stock_basis_id=b.stock_basis_id AND r.status='ACTIVE'
		JOIN LATERAL (SELECT COALESCE(sum(GREATEST(0,l.on_hand_quantity-l.quarantined_quantity-
			COALESCE((SELECT sum(ra.quantity) FROM shop_inventory_reservation_allocations ra
			JOIN shop_inventory_reservation_lines rl ON rl.id=ra.reservation_line_id AND rl.business_id=ra.business_id
			JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
			WHERE ra.business_id=l.business_id AND ra.lot_id=l.id AND ra.status='HELD' AND rl.status='HELD'
			AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())),0))),0) available
			FROM shop_inventory_lots l WHERE l.business_id=b.business_id AND l.stock_basis_id=b.stock_basis_id
			AND l.status='ACTIVE' AND (l.expires_at IS NULL OR l.expires_at>now())) a ON true
		WHERE a.available<=r.minimum_quantity
		UNION ALL
		SELECT l.business_id,l.branch_id,l.stock_basis_id,
			CASE WHEN l.expires_at<=now() THEN 'EXPIRED' ELSE 'EXPIRING_SOON' END,
			CASE WHEN l.expires_at<=now() THEN 'CRITICAL' ELSE 'WARNING' END,
			'expiry:'||l.id,
			jsonb_build_object('lot_id',l.id,'expires_at',l.expires_at,'on_hand_quantity',l.on_hand_quantity),l.updated_at
		FROM shop_inventory_lots l
		LEFT JOIN shop_inventory_reorder_rules r ON r.business_id=l.business_id AND r.stock_basis_id=l.stock_basis_id AND r.status='ACTIVE'
		WHERE l.status='ACTIVE' AND l.on_hand_quantity>0 AND l.expires_at IS NOT NULL
		AND (l.expires_at<=now() OR (r.expiring_soon_hours IS NOT NULL AND l.expires_at<=now()+make_interval(hours=>r.expiring_soon_hours)))
		UNION ALL
		SELECT q.business_id,q.branch_id,q.stock_basis_id,'QUARANTINED','WARNING','quarantine:'||q.id,
			jsonb_build_object('quarantine_id',q.id,'quantity',q.quantity,'reason_code',q.reason_code),q.created_at
		FROM shop_inventory_quarantines q WHERE q.status='ACTIVE'
		UNION ALL
		SELECT b.business_id,b.branch_id,b.stock_basis_id,'BACKORDER_OUTSTANDING','WARNING','backorder:'||b.stock_basis_id,
			jsonb_build_object('outstanding_quantity',b.backorder_quantity),b.updated_at
		FROM shop_inventory_balances b WHERE b.backorder_quantity>0
		UNION ALL
		SELECT b.business_id,b.branch_id,b.stock_basis_id,'RESERVATION_CONFLICT','WARNING','reservation-pressure:'||b.stock_basis_id,
			jsonb_build_object('reserved_quantity',b.reserved_quantity,'available',b.on_hand_quantity-b.reserved_quantity-b.quarantined_quantity),b.updated_at
		FROM shop_inventory_balances b WHERE b.reserved_quantity>0 AND b.on_hand_quantity-b.reserved_quantity-b.quarantined_quantity<=0
		UNION ALL
		SELECT t.business_id,t.source_branch_id,l.source_stock_basis_id,'TRANSFER_DISCREPANCY','WARNING','transfer-discrepancy:'||l.id,
			jsonb_build_object('transfer_id',t.id,'line_id',l.id,'damaged_quantity',l.damaged_quantity,'missing_quantity',l.missing_quantity),l.updated_at
		FROM shop_inventory_transfer_lines l JOIN shop_inventory_transfers t ON t.id=l.transfer_id AND t.business_id=l.business_id
		WHERE l.damaged_quantity>0 OR l.missing_quantity>0
		UNION ALL
		SELECT st.business_id,st.branch_id,l.stock_basis_id,'STOCKTAKE_VARIANCE','WARNING','stocktake-variance:'||l.id,
			jsonb_build_object('stocktake_id',st.id,'expected_quantity',l.expected_quantity,'counted_quantity',l.counted_quantity,'variance_quantity',l.variance_quantity),l.updated_at
		FROM shop_inventory_stocktake_lines l JOIN shop_inventory_stocktakes st ON st.id=l.stocktake_id AND st.business_id=l.business_id
		WHERE l.counted_quantity IS NOT NULL AND l.variance_quantity<>0
	), bounded AS (
		SELECT * FROM candidates ORDER BY sort_at,dedupe_key LIMIT $1
	)
	INSERT INTO shop_inventory_alerts(id,business_id,branch_id,stock_basis_id,alert_type,severity,dedupe_key,details)
	SELECT 'sial_'||gen_random_uuid(),business_id,branch_id,stock_basis_id,alert_type,severity,dedupe_key,details FROM bounded
	ON CONFLICT (business_id,dedupe_key,status) DO UPDATE SET alert_type=EXCLUDED.alert_type,severity=EXCLUDED.severity,details=EXCLUDED.details,updated_at=now()`, limit)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Service) ReconcileInventoryBalance(ctx context.Context, businessID, basisID string) error {
	var balanceOnHand, balanceReserved, balanceQuarantined, lotOnHand, lotReserved, lotQuarantined int64
	if err := s.db.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity,quarantined_quantity FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2`, businessID, basisID).Scan(&balanceOnHand, &balanceReserved, &balanceQuarantined); err != nil {
		return err
	}
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(sum(on_hand_quantity),0),COALESCE(sum(reserved_quantity),0),COALESCE(sum(quarantined_quantity),0) FROM shop_inventory_lots WHERE business_id=$1 AND stock_basis_id=$2 AND status<>'ARCHIVED'`, businessID, basisID).Scan(&lotOnHand, &lotReserved, &lotQuarantined); err != nil {
		return err
	}
	if balanceOnHand != lotOnHand || balanceReserved != lotReserved || balanceQuarantined != lotQuarantined {
		return coded(apperrors.Conflict("Inventory balance does not reconcile with lot evidence.", nil), CodeInventoryStale)
	}
	return nil
}

type BulkOperationInput struct {
	BranchID                  string            `json:"branch_id"`
	OperationType             string            `json:"operation_type"`
	DryRun                    bool              `json:"dry_run"`
	ExpectedCatalogueRevision *int64            `json:"expected_catalogue_revision"`
	ExpectedInventoryRevision *int64            `json:"expected_inventory_revision"`
	Rows                      []json.RawMessage `json:"rows"`
}
type BulkOperationJob struct {
	ID            string     `json:"id"`
	BranchRef     string     `json:"branch_ref,omitempty"`
	OperationType string     `json:"operation_type"`
	DryRun        bool       `json:"dry_run"`
	Status        string     `json:"status"`
	RowCount      int        `json:"row_count"`
	ErrorCount    int        `json:"error_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

func bulkFingerprint(input BulkOperationInput) string {
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateBulkOperationRow(operation string, raw json.RawMessage) []map[string]string {
	fail := func(field, message string) []map[string]string {
		return []map[string]string{{"field": field, "message": message}}
	}
	switch operation {
	case "CATALOGUE_IMPORT":
		if _, err := parseBulkCatalogueRows([][]byte{raw}); err != nil {
			return fail("row", err.Error())
		}
	case "INITIAL_BALANCE", "RECEIPT", "STOCK_ADJUSTMENT":
		var row bulkStockRow
		if json.Unmarshal(raw, &row) != nil || strings.TrimSpace(row.StockBasisID) == "" {
			return fail("stock_basis_id", "A stock basis is required.")
		}
		if row.QuantityDelta == 0 || row.QuantityDelta > MaxInventoryQuantityPerLine || row.QuantityDelta < -MaxInventoryQuantityPerLine || ((operation == "INITIAL_BALANCE" || operation == "RECEIPT") && row.QuantityDelta < 0) {
			return fail("quantity_delta", "Use a non-zero quantity; initial balances and receipts must be positive.")
		}
	case "PRICE_UPDATE":
		var row bulkPriceRow
		if json.Unmarshal(raw, &row) != nil || strings.TrimSpace(row.VariantID) == "" || row.ExpectedVersion <= 0 {
			return fail("variant_id", "A variant and positive expected version are required.")
		}
		if row.RetailPriceMinor != nil && *row.RetailPriceMinor < 0 {
			return fail("retail_price_minor", "Price cannot be negative.")
		}
	case "LISTING_UPDATE":
		var row bulkListingRow
		if json.Unmarshal(raw, &row) != nil || strings.TrimSpace(row.ListingID) == "" || row.ExpectedVersion <= 0 {
			return fail("listing_id", "A listing and positive expected version are required.")
		}
		if row.Status != "" && !contains([]string{"ACTIVE", "INACTIVE", "ARCHIVED"}, strings.ToUpper(row.Status)) {
			return fail("status", "Unknown listing status.")
		}
	}
	return nil
}

func (s *Service) SubmitBulkOperation(ctx context.Context, actor Actor, scope Scope, input BulkOperationInput) (BulkOperationJob, error) {
	input.OperationType = strings.ToUpper(strings.TrimSpace(input.OperationType))
	if input.OperationType == "ADJUSTMENT" {
		input.OperationType = "STOCK_ADJUSTMENT"
	}
	if !contains([]string{"CATALOGUE_IMPORT", "INITIAL_BALANCE", "RECEIPT", "STOCK_ADJUSTMENT", "PRICE_UPDATE", "LISTING_UPDATE"}, input.OperationType) {
		return BulkOperationJob{}, fieldError("operation_type", "Unknown bulk operation.")
	}
	if len(input.Rows) == 0 || len(input.Rows) > 1000 {
		return BulkOperationJob{}, fieldError("rows", "Provide between 1 and 1000 rows.")
	}
	if input.BranchID != "" {
		if err := s.requireBranchScope(scope, input.BranchID); err != nil {
			return BulkOperationJob{}, err
		}
	} else if !scope.Global {
		return BulkOperationJob{}, errForbidden()
	}
	fingerprint := bulkFingerprint(input)
	var out BulkOperationJob
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		out = BulkOperationJob{ID: newID("sboj_"), BranchRef: input.BranchID, OperationType: input.OperationType, DryRun: input.DryRun, Status: "VALIDATED", RowCount: len(input.Rows)}
		err := tx.QueryRow(ctx, `INSERT INTO shop_bulk_operation_jobs(id,business_id,branch_id,operation_type,dry_run,request_fingerprint,expected_catalogue_revision,expected_inventory_revision,status,row_count) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,'VALIDATED',$9) ON CONFLICT(business_id,operation_type,request_fingerprint) DO UPDATE SET updated_at=shop_bulk_operation_jobs.updated_at RETURNING id,status,created_at,updated_at,completed_at,error_count`, out.ID, actor.VendorID, input.BranchID, input.OperationType, input.DryRun, fingerprint, input.ExpectedCatalogueRevision, input.ExpectedInventoryRevision, len(input.Rows)).Scan(&out.ID, &out.Status, &out.CreatedAt, &out.UpdatedAt, &out.CompletedAt, &out.ErrorCount)
		if err != nil {
			return err
		}
		validationErrors := 0
		for i, row := range input.Rows {
			if !json.Valid(row) {
				return fieldError("rows", "Every bulk row must be valid JSON.")
			}
			errorsForRow := validateBulkOperationRow(input.OperationType, row)
			status := "VALID"
			if len(errorsForRow) > 0 {
				status = "ERROR"
				validationErrors++
			}
			errorsJSON, _ := json.Marshal(errorsForRow)
			if _, err = tx.Exec(ctx, `INSERT INTO shop_bulk_operation_rows(id,business_id,job_id,row_number,payload,validation_errors,status) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(job_id,row_number) DO NOTHING`, newID("sbor_"), actor.VendorID, out.ID, i+1, row, errorsJSON, status); err != nil {
				return err
			}
		}
		if validationErrors > 0 {
			if err = tx.QueryRow(ctx, `UPDATE shop_bulk_operation_jobs SET error_count=$3,updated_at=now() WHERE id=$1 AND business_id=$2 RETURNING error_count,updated_at`, out.ID, actor.VendorID, validationErrors).Scan(&out.ErrorCount, &out.UpdatedAt); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.bulk.validated", eventType: "shop.bulk.validated", aggregateType: "shop_bulk_operation_job", aggregateID: out.ID, branchID: input.BranchID, version: 1, after: out, payload: out})
	})
	return out, err
}

type bulkStockRow struct {
	StockBasisID     string `json:"stock_basis_id"`
	QuantityDelta    int64  `json:"quantity_delta"`
	ReasonCode       string `json:"reason_code"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type bulkPriceRow struct {
	VariantID        string `json:"variant_id"`
	RetailPriceMinor *int64 `json:"retail_price_minor"`
	ExpectedVersion  int64  `json:"expected_version"`
}
type bulkListingRow struct {
	ListingID               string `json:"listing_id"`
	Status                  string `json:"status"`
	AutoHideWhenUnavailable *bool  `json:"auto_hide_when_unavailable"`
	ExpectedVersion         int64  `json:"expected_version"`
}

func (s *Service) ApplyBulkOperation(ctx context.Context, actor Actor, scope Scope, jobID string) (BulkOperationJob, error) {
	var out BulkOperationJob
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var expectedCatalogue, expectedInventory *int64
		err := tx.QueryRow(ctx, `SELECT id,COALESCE(branch_id,''),operation_type,dry_run,status,row_count,error_count,created_at,updated_at,completed_at,expected_catalogue_revision,expected_inventory_revision FROM shop_bulk_operation_jobs WHERE id=$1 AND business_id=$2 FOR UPDATE`, jobID, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.OperationType, &out.DryRun, &out.Status, &out.RowCount, &out.ErrorCount, &out.CreatedAt, &out.UpdatedAt, &out.CompletedAt, &expectedCatalogue, &expectedInventory)
		if err != nil {
			return mapDB(err, "Bulk operation", CodeAdjustmentNotFound)
		}
		if out.BranchRef != "" {
			if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
				return err
			}
		} else if !scope.Global {
			return errForbidden()
		}
		if out.Status == "COMPLETED" {
			return nil
		}
		if out.Status != "VALIDATED" {
			return coded(apperrors.Conflict("Bulk operation is not ready to apply.", nil), CodeReservationStateConflict)
		}
		if out.DryRun {
			err = tx.QueryRow(ctx, `UPDATE shop_bulk_operation_jobs SET status='COMPLETED',completed_at=now(),updated_at=now() WHERE id=$1 RETURNING status,completed_at,updated_at`, jobID).Scan(&out.Status, &out.CompletedAt, &out.UpdatedAt)
			return err
		}
		if out.ErrorCount > 0 {
			return coded(apperrors.Conflict("Bulk validation errors must be corrected before apply.", nil), CodeInventoryStale)
		}
		if expectedInventory != nil && out.BranchRef != "" {
			var revision int64
			_ = tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM shop_inventory_revisions WHERE business_id=$1 AND branch_id=$2),0)`, actor.VendorID, out.BranchRef).Scan(&revision)
			if revision != *expectedInventory {
				return coded(apperrors.Conflict("Inventory changed after bulk validation.", nil), CodeInventoryStale)
			}
		}
		if expectedCatalogue != nil && out.BranchRef != "" {
			var revision int64
			_ = tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM shop_catalogue_revisions WHERE business_id=$1 AND branch_id=$2 AND domain='SHOP'),0)`, actor.VendorID, out.BranchRef).Scan(&revision)
			if revision != *expectedCatalogue {
				return coded(apperrors.Conflict("Catalogue changed after bulk validation.", nil), CodeInventoryStale)
			}
		}
		rows, err := tx.Query(ctx, `SELECT id,payload FROM shop_bulk_operation_rows WHERE job_id=$1 AND business_id=$2 ORDER BY row_number FOR UPDATE`, jobID, actor.VendorID)
		if err != nil {
			return err
		}
		type row struct {
			id      string
			payload []byte
		}
		var batch []row
		for rows.Next() {
			var item row
			if err = rows.Scan(&item.id, &item.payload); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, item)
		}
		rows.Close()
		switch out.OperationType {
		case "CATALOGUE_IMPORT":
			payloads := make([][]byte, len(batch))
			for i := range batch {
				payloads[i] = batch[i].payload
			}
			if err = s.applyBulkCatalogueRows(ctx, tx, actor, scope, payloads); err != nil {
				return err
			}
		case "INITIAL_BALANCE", "RECEIPT", "STOCK_ADJUSTMENT":
			parsed := make([]bulkStockRow, len(batch))
			for i, item := range batch {
				if err = json.Unmarshal(item.payload, &parsed[i]); err != nil {
					return fieldError("rows", "Invalid stock row.")
				}
			}
			sort.Slice(parsed, func(i, j int) bool { return parsed[i].StockBasisID < parsed[j].StockBasisID })
			for _, item := range parsed {
				branch, variant, revision, lockErr := lockInventoryBalance(ctx, tx, actor.VendorID, item.StockBasisID)
				if lockErr != nil {
					return lockErr
				}
				if err = s.requireBranchScope(scope, branch); err != nil {
					return err
				}
				if revision != item.ExpectedRevision {
					return coded(apperrors.Conflict("Inventory changed after bulk validation.", nil), CodeInventoryStale)
				}
				if out.OperationType == "INITIAL_BALANCE" && revision != 0 {
					return coded(apperrors.Conflict("Initial balance is only valid before inventory movements.", nil), CodeInventoryStale)
				}
				reason := strings.ToUpper(item.ReasonCode)
				if reason == "" {
					reason = "DATA_CORRECTION"
				}
				if err = applyPhysicalDelta(ctx, tx, actor.VendorID, branch, item.StockBasisID, variant, item.QuantityDelta, reason, "BULK_OPERATION", jobID, actor.StaffID); err != nil {
					return err
				}
			}
		case "PRICE_UPDATE":
			currency, readErr := s.businessCurrency(ctx, tx, actor.VendorID)
			if readErr != nil {
				return readErr
			}
			for _, item := range batch {
				var price bulkPriceRow
				if err = json.Unmarshal(item.payload, &price); err != nil {
					return fieldError("rows", "Invalid price row.")
				}
				if price.RetailPriceMinor != nil && *price.RetailPriceMinor < 0 {
					return fieldError("rows.retail_price_minor", "Price cannot be negative.")
				}
				var productID string
				var previous *int64
				var currentVersion int64
				err = tx.QueryRow(ctx, `SELECT product_id,retail_price_minor,version FROM shop_variants WHERE id=$1 AND business_id=$2 FOR UPDATE`, price.VariantID, actor.VendorID).Scan(&productID, &previous, &currentVersion)
				if err != nil || currentVersion != price.ExpectedVersion {
					return errStaleVersion()
				}
				var version int64
				if err = tx.QueryRow(ctx, `UPDATE shop_variants SET retail_price_minor=$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 RETURNING version`, price.VariantID, actor.VendorID, price.RetailPriceMinor).Scan(&version); err != nil {
					return err
				}
				if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{subjectType: "VARIANT_RETAIL", subjectRef: price.VariantID, productRef: productID, variantRef: price.VariantID, previousAmount: previous, amount: price.RetailPriceMinor, reason: ReasonPriceChanged, subjectVersion: version}); err != nil {
					return err
				}
				if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, price.VariantID); err != nil {
					return err
				}
			}
		case "LISTING_UPDATE":
			for _, item := range batch {
				var listing bulkListingRow
				if err = json.Unmarshal(item.payload, &listing); err != nil {
					return fieldError("rows", "Invalid listing row.")
				}
				var branch string
				err = tx.QueryRow(ctx, `UPDATE shop_branch_listings SET status=COALESCE(NULLIF($3,''),status),auto_hide_when_unavailable=COALESCE($4,auto_hide_when_unavailable),updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 AND version=$5 RETURNING branch_id`, listing.ListingID, actor.VendorID, listing.Status, listing.AutoHideWhenUnavailable, listing.ExpectedVersion).Scan(&branch)
				if err != nil {
					return errStaleVersion()
				}
				if err = s.requireBranchScope(scope, branch); err != nil {
					return err
				}
				if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, branch); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_bulk_operation_rows SET status='APPLIED',updated_at=now() WHERE job_id=$1 AND business_id=$2`, jobID, actor.VendorID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `UPDATE shop_bulk_operation_jobs SET status='COMPLETED',completed_at=now(),updated_at=now() WHERE id=$1 RETURNING status,completed_at,updated_at`, jobID).Scan(&out.Status, &out.CompletedAt, &out.UpdatedAt)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.bulk.applied", eventType: "shop.bulk.applied", aggregateType: "shop_bulk_operation_job", aggregateID: jobID, branchID: out.BranchRef, version: 1, after: out, payload: out})
	})
	return out, err
}

// RunBulkOperationPass leases and applies a bounded set of validated jobs.
// Each job is still applied in one transaction by ApplyBulkOperation; the
// lease only coordinates workers and never weakens row/version checks.
func (s *Service) RunBulkOperationPass(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	workerID := newID("shop-bulk-worker-")
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH claim AS (
		SELECT id FROM shop_bulk_operation_jobs
		WHERE status='VALIDATED' AND (lease_expires_at IS NULL OR lease_expires_at<=now())
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $1
	)
	UPDATE shop_bulk_operation_jobs j SET lease_owner=$2,lease_expires_at=now()+interval '5 minutes',attempts=attempts+1,updated_at=now()
	FROM claim WHERE j.id=claim.id RETURNING j.id,j.business_id,j.attempts`, limit, workerID)
	if err != nil {
		return 0, err
	}
	type claimedJob struct {
		id, business string
		attempts     int
	}
	var jobs []claimedJob
	for rows.Next() {
		var job claimedJob
		if err = rows.Scan(&job.id, &job.business, &job.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, job)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	processed := 0
	for _, job := range jobs {
		actor := Actor{VendorID: job.business, StaffID: workerID}
		_, applyErr := s.ApplyBulkOperation(ctx, actor, Scope{Global: true}, job.id)
		if applyErr == nil {
			if _, err = s.db.Exec(ctx, `UPDATE shop_bulk_operation_jobs SET lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND business_id=$2`, job.id, job.business); err != nil {
				return processed, err
			}
			processed++
			continue
		}
		dead := job.attempts >= 5
		status := "VALIDATED"
		if dead {
			status = "DEAD_LETTER"
		}
		if _, err = s.db.Exec(ctx, `UPDATE shop_bulk_operation_jobs SET status=$3,lease_owner=NULL,lease_expires_at=NULL,error_count=CASE WHEN $3='DEAD_LETTER' THEN row_count ELSE error_count END,updated_at=now() WHERE id=$1 AND business_id=$2`, job.id, job.business, status); err != nil {
			return processed, err
		}
		if dead {
			if _, err = s.db.Exec(ctx, `UPDATE shop_bulk_operation_rows SET status='ERROR',validation_errors=jsonb_build_array(jsonb_build_object('code','BULK_APPLY_FAILED','message',left($3,500))),updated_at=now() WHERE job_id=$1 AND business_id=$2`, job.id, job.business, applyErr.Error()); err != nil {
				return processed, err
			}
		}
		processed++
	}
	return processed, nil
}

// ReconcileExpiredReservations performs bounded cleanup with row skipping;
// correctness is also enforced synchronously by reads/reservation attempts.
func (s *Service) ReconcileExpiredReservations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT l.business_id,l.stock_basis_id FROM shop_inventory_reservation_lines l JOIN shop_inventory_reservations r ON r.id=l.reservation_id AND r.business_id=l.business_id WHERE l.status='HELD' AND r.status IN ('HELD','CONFIRMED') AND r.expires_at<=now() ORDER BY l.business_id,l.stock_basis_id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type basis struct{ business, id string }
	var bases []basis
	for rows.Next() {
		var item basis
		if err = rows.Scan(&item.business, &item.id); err != nil {
			rows.Close()
			return 0, err
		}
		bases = append(bases, item)
	}
	rows.Close()
	processed := 0
	for _, item := range bases {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return processed, beginErr
		}
		var expired expiredReleaseEvidence
		if releaseErr := releaseExpiredForBasis(ctx, tx, item.business, item.id, s.now(), &expired); releaseErr != nil {
			_ = tx.Rollback(ctx)
			return processed, releaseErr
		}
		if expired.PhysicalQuantity > 0 || expired.BackorderQuantity > 0 {
			actor := Actor{VendorID: item.business}
			if recordErr := s.record(ctx, tx, actor, evidence{action: "shop.inventory.reservation.expired", eventType: "shop.inventory.reservation.expired", aggregateType: "shop_inventory_stock_basis", aggregateID: item.id, branchID: expired.BranchRef, version: expired.InventoryRevision, after: expired, payload: expired, reason: "HOLD_EXPIRED"}); recordErr != nil {
				_ = tx.Rollback(ctx)
				return processed, recordErr
			}
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return processed, commitErr
		}
		processed++
	}
	return processed, nil
}

// ReconcileExpiredLots posts EXPIRY evidence for unencumbered expired lots.
// Sellability already excludes them synchronously; this worker reconciles the
// materialized physical balance and deliberately skips reserved/quarantined
// lots so it can never invalidate an active workflow behind its back.
func (s *Service) ReconcileExpiredLots(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT business_id,id,stock_basis_id FROM shop_inventory_lots WHERE status='ACTIVE' AND expires_at<=now() AND on_hand_quantity>0 ORDER BY expires_at,id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct{ business, lot, basis string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.business, &item.lot, &item.basis); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	processed := 0
	for _, candidate := range candidates {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return processed, beginErr
		}
		branchID, variantID, _, lockErr := lockInventoryBalance(ctx, tx, candidate.business, candidate.basis)
		if lockErr != nil {
			_ = tx.Rollback(ctx)
			return processed, lockErr
		}
		var onHand, reserved, quarantined int64
		var status string
		var expiresAt *time.Time
		lockErr = tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity,quarantined_quantity,status,expires_at FROM shop_inventory_lots WHERE id=$1 AND business_id=$2 FOR UPDATE`, candidate.lot, candidate.business).Scan(&onHand, &reserved, &quarantined, &status, &expiresAt)
		if lockErr != nil {
			_ = tx.Rollback(ctx)
			return processed, lockErr
		}
		if status != "ACTIVE" || expiresAt == nil || expiresAt.After(s.now()) || onHand == 0 || reserved > 0 || quarantined > 0 {
			_ = tx.Rollback(ctx)
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=0,status='EXPIRED',updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, candidate.lot, candidate.business); err != nil {
			_ = tx.Rollback(ctx)
			return processed, err
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity-$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, candidate.business, candidate.basis, onHand); err != nil {
			_ = tx.Rollback(ctx)
			return processed, err
		}
		revision, revisionErr := bumpInventoryRevision(ctx, tx, candidate.business, branchID)
		if revisionErr != nil {
			_ = tx.Rollback(ctx)
			return processed, revisionErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,on_hand_delta,source_type,source_ref,reason_code,inventory_revision) VALUES($1,$2,$3,$4,$5,$6,'EXPIRY',$7,$8,'LOT_EXPIRY',$6,'EXPIRY_RECONCILIATION',$9)`, newID("sim_"), candidate.business, branchID, candidate.basis, variantID, candidate.lot, onHand, -onHand, revision); err != nil {
			_ = tx.Rollback(ctx)
			return processed, err
		}
		payload := map[string]any{"lot_id": candidate.lot, "stock_basis_id": candidate.basis, "expired_quantity": onHand, "expires_at": expiresAt, "inventory_revision": revision}
		if err = s.record(ctx, tx, Actor{VendorID: candidate.business}, evidence{action: "shop.inventory.lot.expired", eventType: "shop.inventory.lot.expired", aggregateType: "shop_inventory_lot", aggregateID: candidate.lot, branchID: branchID, version: revision, after: payload, payload: payload, reason: "EXPIRY_RECONCILIATION"}); err != nil {
			_ = tx.Rollback(ctx)
			return processed, err
		}
		if err = tx.Commit(ctx); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
