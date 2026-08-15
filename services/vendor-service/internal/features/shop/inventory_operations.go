package shop

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	PermInventoryAdjust     = "vendor.shop.inventory.adjust"
	PermInventoryTransfer   = "vendor.shop.inventory.transfer"
	PermInventoryStocktake  = "vendor.shop.inventory.stocktake"
	PermInventoryQuarantine = "vendor.shop.inventory.quarantine"
	PermInventoryPolicy     = "vendor.shop.inventory.manage_policy"
	PermBulkImportManage    = "vendor.shop.bulk_import.manage"

	CodeInventoryStale       = "SHOP_INVENTORY_STALE_REVISION"
	CodeAdjustmentNotFound   = "SHOP_ADJUSTMENT_NOT_FOUND"
	CodeQuarantineNotFound   = "SHOP_QUARANTINE_NOT_FOUND"
	CodeTransferNotFound     = "SHOP_TRANSFER_NOT_FOUND"
	CodeStocktakeNotFound    = "SHOP_STOCKTAKE_NOT_FOUND"
	CodeInventoryPolicyLimit = "SHOP_INVENTORY_POLICY_LIMIT_EXCEEDED"
)

type InventoryPolicy struct {
	ID                         string     `json:"id"`
	BranchRef                  string     `json:"branch_ref"`
	ListingRef                 string     `json:"listing_ref"`
	VariantRef                 string     `json:"variant_ref"`
	Mode                       string     `json:"mode"`
	MaximumOutstandingQuantity *int64     `json:"maximum_outstanding_quantity"`
	MaximumPerRequest          *int64     `json:"maximum_per_request"`
	AvailableFrom              *time.Time `json:"available_from"`
	ExpectedRestockAt          *time.Time `json:"expected_restock_at"`
	LeadTimeDays               *int       `json:"lead_time_days"`
	CutoffAt                   *time.Time `json:"cutoff_at"`
	AllocationPolicy           string     `json:"allocation_policy"`
	Status                     string     `json:"status"`
	Version                    int64      `json:"version"`
}

type SetInventoryPolicyInput struct {
	Mode                       string     `json:"mode"`
	MaximumOutstandingQuantity *int64     `json:"maximum_outstanding_quantity"`
	MaximumPerRequest          *int64     `json:"maximum_per_request"`
	AvailableFrom              *time.Time `json:"available_from"`
	ExpectedRestockAt          *time.Time `json:"expected_restock_at"`
	LeadTimeDays               *int       `json:"lead_time_days"`
	CutoffAt                   *time.Time `json:"cutoff_at"`
	AllocationPolicy           string     `json:"allocation_policy"`
	ExpectedVersion            int64      `json:"expected_version"`
}

func (s *Service) SetInventoryPolicy(ctx context.Context, actor Actor, scope Scope, listingID string, input SetInventoryPolicyInput) (InventoryPolicy, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryPolicy{}, err
	}
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	if !contains([]string{"NONE", "PREORDER", "BACKORDER"}, input.Mode) {
		return InventoryPolicy{}, fieldError("mode", "Use NONE, PREORDER or BACKORDER.")
	}
	if input.AllocationPolicy == "" {
		input.AllocationPolicy = "FIFO_BY_CREATED_AT"
	}
	if !contains([]string{"FIFO_BY_CREATED_AT", "PRIORITY_THEN_FIFO"}, input.AllocationPolicy) {
		return InventoryPolicy{}, fieldError("allocation_policy", "Unknown allocation policy.")
	}
	for field, value := range map[string]*int64{"maximum_outstanding_quantity": input.MaximumOutstandingQuantity, "maximum_per_request": input.MaximumPerRequest} {
		if value != nil && (*value <= 0 || *value > MaxInventoryQuantityPerLine) {
			return InventoryPolicy{}, fieldError(field, "Value must be positive and within the controlled per-line limit.")
		}
	}
	var out InventoryPolicy
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var branchID, variantID string
		if err := tx.QueryRow(ctx, `SELECT branch_id,variant_id FROM shop_branch_listings WHERE id=$1 AND business_id=$2`, listingID, actor.VendorID).Scan(&branchID, &variantID); err != nil {
			return mapDB(err, "Listing", CodeListingNotFound)
		}
		if err := s.requireBranchScope(scope, branchID); err != nil {
			return err
		}
		id := newID("sip_")
		err := tx.QueryRow(ctx, `INSERT INTO shop_inventory_policies
			(id,business_id,branch_id,listing_id,variant_id,mode,maximum_outstanding_quantity,maximum_per_request,available_from,expected_restock_at,lead_time_days,cutoff_at,allocation_policy)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (business_id,branch_id,variant_id) DO UPDATE SET mode=EXCLUDED.mode,
			maximum_outstanding_quantity=EXCLUDED.maximum_outstanding_quantity,maximum_per_request=EXCLUDED.maximum_per_request,
			available_from=EXCLUDED.available_from,expected_restock_at=EXCLUDED.expected_restock_at,lead_time_days=EXCLUDED.lead_time_days,
			cutoff_at=EXCLUDED.cutoff_at,allocation_policy=EXCLUDED.allocation_policy,status='ACTIVE',updated_at=now(),version=shop_inventory_policies.version+1
			RETURNING id,branch_id,listing_id,variant_id,mode,maximum_outstanding_quantity,maximum_per_request,available_from,
			expected_restock_at,lead_time_days,cutoff_at,allocation_policy,status,version`, id, actor.VendorID, branchID, listingID, variantID, input.Mode,
			input.MaximumOutstandingQuantity, input.MaximumPerRequest, input.AvailableFrom, input.ExpectedRestockAt, input.LeadTimeDays, input.CutoffAt, input.AllocationPolicy).
			Scan(&out.ID, &out.BranchRef, &out.ListingRef, &out.VariantRef, &out.Mode, &out.MaximumOutstandingQuantity, &out.MaximumPerRequest, &out.AvailableFrom, &out.ExpectedRestockAt, &out.LeadTimeDays, &out.CutoffAt, &out.AllocationPolicy, &out.Status, &out.Version)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.policy.changed", eventType: "shop.inventory.policy.changed", aggregateType: "shop_inventory_policy", aggregateID: out.ID, branchID: branchID, version: out.Version, after: out, payload: out})
	})
	return out, err
}

type InventoryAdjustment struct {
	ID               string    `json:"id"`
	BranchRef        string    `json:"branch_ref"`
	StockBasisRef    string    `json:"stock_basis_ref"`
	VariantRef       string    `json:"variant_ref"`
	QuantityDelta    int64     `json:"quantity_delta"`
	ReasonCode       string    `json:"reason_code"`
	Note             string    `json:"note,omitempty"`
	ExpectedRevision int64     `json:"expected_revision"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	Version          int64     `json:"version"`
}

type AdjustInventoryInput struct {
	StockBasisID     string `json:"stock_basis_id"`
	QuantityDelta    int64  `json:"quantity_delta"`
	ReasonCode       string `json:"reason_code"`
	Note             string `json:"note"`
	ExpectedRevision int64  `json:"expected_revision"`
}

var adjustmentReasons = []string{"FOUND_STOCK", "DATA_CORRECTION", "DAMAGE", "WASTE", "SPOILAGE", "THEFT_LOSS", "EXPIRY", "MANUAL_CORRECTION", "OTHER_CONTROLLED"}

func (s *Service) AdjustInventory(ctx context.Context, actor Actor, scope Scope, input AdjustInventoryInput) (InventoryAdjustment, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryAdjustment{}, err
	}
	if input.QuantityDelta == 0 || input.QuantityDelta > MaxInventoryQuantityPerLine || input.QuantityDelta < -MaxInventoryQuantityPerLine {
		return InventoryAdjustment{}, fieldError("quantity_delta", "Quantity delta must be non-zero and within the controlled per-command limit.")
	}
	input.ReasonCode = strings.ToUpper(strings.TrimSpace(input.ReasonCode))
	if !contains(adjustmentReasons, input.ReasonCode) {
		return InventoryAdjustment{}, fieldError("reason_code", "Unknown adjustment reason.")
	}
	if len(input.Note) > 1000 {
		return InventoryAdjustment{}, fieldError("note", "Note cannot exceed 1000 characters.")
	}
	var out InventoryAdjustment
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		branchID, variantID, revision, err := lockInventoryBalance(ctx, tx, actor.VendorID, input.StockBasisID)
		if err != nil {
			return err
		}
		if err = s.requireBranchScope(scope, branchID); err != nil {
			return err
		}
		if revision != input.ExpectedRevision {
			return coded(apperrors.Conflict("Inventory changed after it was loaded.", nil), CodeInventoryStale)
		}
		out = InventoryAdjustment{ID: newID("sia_"), BranchRef: branchID, StockBasisRef: input.StockBasisID, VariantRef: variantID, QuantityDelta: input.QuantityDelta, ReasonCode: input.ReasonCode, Note: input.Note, ExpectedRevision: input.ExpectedRevision, Status: "POSTED", Version: 1}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_adjustments(id,business_id,branch_id,stock_basis_id,variant_id,quantity_delta,reason_code,note,expected_revision,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))`, out.ID, actor.VendorID, branchID, input.StockBasisID, variantID, input.QuantityDelta, input.ReasonCode, input.Note, input.ExpectedRevision, actor.StaffID); err != nil {
			return err
		}
		if err = applyPhysicalDelta(ctx, tx, actor.VendorID, branchID, input.StockBasisID, variantID, input.QuantityDelta, input.ReasonCode, "ADJUSTMENT", out.ID, actor.StaffID); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT created_at FROM shop_inventory_adjustments WHERE id=$1`, out.ID).Scan(&out.CreatedAt); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.adjusted", eventType: "shop.inventory.adjusted", aggregateType: "shop_inventory_adjustment", aggregateID: out.ID, branchID: branchID, version: 1, after: out, payload: out, reason: input.ReasonCode})
	})
	return out, err
}

func lockInventoryBalance(ctx context.Context, tx pgx.Tx, businessID, basisID string) (branchID, variantID string, revision int64, err error) {
	err = tx.QueryRow(ctx, `SELECT branch_id,variant_id,revision FROM shop_inventory_balances
		WHERE business_id=$1 AND stock_basis_id=$2 FOR UPDATE`, businessID, basisID).Scan(&branchID, &variantID, &revision)
	if err != nil {
		err = mapDB(err, "Inventory balance", CodeStockBasisNotFound)
	}
	return
}

func applyPhysicalDelta(ctx context.Context, tx pgx.Tx, businessID, branchID, basisID, variantID string, delta int64, reason, sourceType, sourceRef, actorRef string) error {
	if delta > 0 {
		lotID := newID("sil_")
		if _, err := tx.Exec(ctx, `INSERT INTO shop_inventory_lots(id,business_id,branch_id,stock_basis_id,variant_id,source_ref,received_at,on_hand_quantity)
			VALUES($1,$2,$3,$4,$5,$6,now(),$7)`, lotID, businessID, branchID, basisID, variantID, sourceRef, delta); err != nil {
			return err
		}
	} else {
		remaining := -delta
		rows, err := tx.Query(ctx, `SELECT id,on_hand_quantity-reserved_quantity-quarantined_quantity
			FROM shop_inventory_lots WHERE business_id=$1 AND stock_basis_id=$2 AND status='ACTIVE'
			AND (expires_at IS NULL OR expires_at>now()) AND on_hand_quantity>reserved_quantity+quarantined_quantity
			ORDER BY expires_at ASC NULLS LAST,received_at,id FOR UPDATE`, businessID, basisID)
		if err != nil {
			return err
		}
		type lot struct {
			id        string
			available int64
		}
		var lots []lot
		for rows.Next() {
			var item lot
			if err = rows.Scan(&item.id, &item.available); err != nil {
				rows.Close()
				return err
			}
			lots = append(lots, item)
		}
		rows.Close()
		for _, item := range lots {
			if remaining == 0 {
				break
			}
			take := item.available
			if take > remaining {
				take = remaining
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=on_hand_quantity-$3,
				status=CASE WHEN on_hand_quantity-$3=0 THEN 'DEPLETED' ELSE status END,updated_at=now(),version=version+1
				WHERE id=$1 AND business_id=$2`, item.id, businessID, take); err != nil {
				return err
			}
			remaining -= take
		}
		if remaining != 0 {
			return coded(apperrors.Conflict("Adjustment exceeds unreserved sellable physical stock.", nil), CodeInventoryInsufficient)
		}
	}
	var balanceRevision int64
	if err := tx.QueryRow(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity+$3,revision=revision+1,updated_at=now()
		WHERE business_id=$1 AND stock_basis_id=$2 RETURNING revision`, businessID, basisID, delta).Scan(&balanceRevision); err != nil {
		return err
	}
	revision, err := bumpInventoryRevision(ctx, tx, businessID, branchID)
	if err != nil {
		return err
	}
	movementType := "ADJUSTMENT_IN"
	if delta < 0 {
		movementType = "ADJUSTMENT_OUT"
	}
	quantity := delta
	if quantity < 0 {
		quantity = -quantity
	}
	_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,on_hand_delta,source_type,source_ref,reason_code,actor_ref,inventory_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13)`, newID("sim_"), businessID, branchID, basisID, variantID, movementType, quantity, delta, sourceType, sourceRef, reason, actorRef, revision)
	_ = balanceRevision
	return err
}

type InventoryQuarantine struct {
	ID            string     `json:"id"`
	BranchRef     string     `json:"branch_ref"`
	StockBasisRef string     `json:"stock_basis_ref"`
	VariantRef    string     `json:"variant_ref"`
	Quantity      int64      `json:"quantity"`
	ReasonCode    string     `json:"reason_code"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at"`
	Version       int64      `json:"version"`
}

func (s *Service) QuarantineInventory(ctx context.Context, actor Actor, scope Scope, basisID string, quantity int64, reason, note string) (InventoryQuarantine, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryQuarantine{}, err
	}
	reason = strings.ToUpper(strings.TrimSpace(reason))
	if quantity <= 0 || quantity > MaxInventoryQuantityPerLine || !contains([]string{"QUALITY_CHECK", "DAMAGED", "RECALL", "TEMPERATURE_EXCURSION", "SUSPECTED_SPOILAGE", "OTHER_CONTROLLED"}, reason) {
		return InventoryQuarantine{}, fieldError("quantity", "Use a positive quantity and controlled quarantine reason.")
	}
	var out InventoryQuarantine
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		branchID, variantID, _, err := lockInventoryBalance(ctx, tx, actor.VendorID, basisID)
		if err != nil {
			return err
		}
		if err = s.requireBranchScope(scope, branchID); err != nil {
			return err
		}
		out = InventoryQuarantine{ID: newID("siq_"), BranchRef: branchID, StockBasisRef: basisID, VariantRef: variantID, Quantity: quantity, ReasonCode: reason, Status: "ACTIVE", Version: 1}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_quarantines(id,business_id,branch_id,stock_basis_id,variant_id,quantity,reason_code,note)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, out.ID, actor.VendorID, branchID, basisID, variantID, quantity, reason, note); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,on_hand_quantity-reserved_quantity-quarantined_quantity FROM shop_inventory_lots
			WHERE business_id=$1 AND stock_basis_id=$2 AND status='ACTIVE' AND (expires_at IS NULL OR expires_at>now())
			AND on_hand_quantity>reserved_quantity+quarantined_quantity ORDER BY expires_at ASC NULLS LAST,received_at,id FOR UPDATE`, actor.VendorID, basisID)
		if err != nil {
			return err
		}
		type lot struct {
			id        string
			available int64
		}
		var lots []lot
		for rows.Next() {
			var item lot
			if err = rows.Scan(&item.id, &item.available); err != nil {
				rows.Close()
				return err
			}
			lots = append(lots, item)
		}
		rows.Close()
		remaining := quantity
		for _, item := range lots {
			if remaining == 0 {
				break
			}
			take := item.available
			if take > remaining {
				take = remaining
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET quarantined_quantity=quarantined_quantity+$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, item.id, actor.VendorID, take); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_quarantine_allocations(id,business_id,quarantine_id,lot_id,quantity) VALUES($1,$2,$3,$4,$5)`, newID("siqa_"), actor.VendorID, out.ID, item.id, take); err != nil {
				return err
			}
			remaining -= take
		}
		if remaining != 0 {
			return coded(apperrors.Conflict("Quarantine exceeds sellable physical stock.", nil), CodeInventoryInsufficient)
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET quarantined_quantity=quarantined_quantity+$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, basisID, quantity); err != nil {
			return err
		}
		revision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, branchID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,quarantined_delta,source_type,source_ref,reason_code,actor_ref,inventory_revision)
			VALUES($1,$2,$3,$4,$5,'QUARANTINE_IN',$6,$6,'QUARANTINE',$7,$8,NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, branchID, basisID, variantID, quantity, out.ID, reason, actor.StaffID, revision); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT created_at FROM shop_inventory_quarantines WHERE id=$1`, out.ID).Scan(&out.CreatedAt); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.quarantined", eventType: "shop.inventory.quarantined", aggregateType: "shop_inventory_quarantine", aggregateID: out.ID, branchID: branchID, version: 1, after: out, payload: out, reason: reason})
	})
	return out, err
}

func (s *Service) ResolveQuarantine(ctx context.Context, actor Actor, scope Scope, id, action string) (InventoryQuarantine, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryQuarantine{}, err
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	if action != "RELEASE" && action != "DISPOSE" {
		return InventoryQuarantine{}, fieldError("action", "Use RELEASE or DISPOSE.")
	}
	var out InventoryQuarantine
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id,branch_id,stock_basis_id,variant_id,quantity,reason_code,status,created_at,resolved_at,version FROM shop_inventory_quarantines WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.StockBasisRef, &out.VariantRef, &out.Quantity, &out.ReasonCode, &status, &out.CreatedAt, &out.ResolvedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Quarantine", CodeQuarantineNotFound)
		}
		if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
			return err
		}
		target := "RELEASED"
		movement := "QUARANTINE_RELEASE"
		onHandDelta := int64(0)
		if action == "DISPOSE" {
			target = "DISPOSED"
			movement = "WASTE"
			onHandDelta = -out.Quantity
		}
		if status == target {
			out.Status = target
			return nil
		}
		if status != "ACTIVE" {
			return coded(apperrors.Conflict("Quarantine is already resolved.", nil), CodeReservationStateConflict)
		}
		rows, err := tx.Query(ctx, `SELECT id,lot_id,quantity FROM shop_inventory_quarantine_allocations WHERE quarantine_id=$1 AND business_id=$2 AND status='ACTIVE' ORDER BY lot_id FOR UPDATE`, id, actor.VendorID)
		if err != nil {
			return err
		}
		type allocation struct {
			id, lot  string
			quantity int64
		}
		var allocations []allocation
		for rows.Next() {
			var a allocation
			if err = rows.Scan(&a.id, &a.lot, &a.quantity); err != nil {
				rows.Close()
				return err
			}
			allocations = append(allocations, a)
		}
		rows.Close()
		for _, a := range allocations {
			if action == "DISPOSE" {
				_, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=on_hand_quantity-$3,quarantined_quantity=quarantined_quantity-$3,status=CASE WHEN on_hand_quantity-$3=0 THEN 'DEPLETED' ELSE status END,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, a.lot, actor.VendorID, a.quantity)
			} else {
				_, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET quarantined_quantity=quarantined_quantity-$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, a.lot, actor.VendorID, a.quantity)
			}
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_quarantine_allocations SET status=$3,updated_at=now() WHERE id=$1 AND business_id=$2`, a.id, actor.VendorID, target); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity+$3,quarantined_quantity=quarantined_quantity-$4,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, out.StockBasisRef, onHandDelta, out.Quantity); err != nil {
			return err
		}
		revision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, out.BranchRef)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,on_hand_delta,quarantined_delta,source_type,source_ref,reason_code,actor_ref,inventory_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'QUARANTINE',$10,$11,NULLIF($12,''),$13)`, newID("sim_"), actor.VendorID, out.BranchRef, out.StockBasisRef, out.VariantRef, movement, out.Quantity, onHandDelta, -out.Quantity, id, out.ReasonCode, actor.StaffID, revision); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `UPDATE shop_inventory_quarantines SET status=$3,resolved_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 RETURNING status,resolved_at,version`, id, actor.VendorID, target).Scan(&out.Status, &out.ResolvedAt, &out.Version)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.quarantine.resolved", eventType: "shop.inventory.quarantine.resolved", aggregateType: "shop_inventory_quarantine", aggregateID: id, branchID: out.BranchRef, version: out.Version, after: out, payload: out, reason: action})
	})
	return out, err
}

type InventoryStocktake struct {
	ID               string     `json:"id"`
	BranchRef        string     `json:"branch_ref"`
	Type             string     `json:"stocktake_type"`
	SnapshotRevision int64      `json:"snapshot_revision"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	PostedAt         *time.Time `json:"posted_at"`
	Version          int64      `json:"version"`
}

func (s *Service) CreateStocktake(ctx context.Context, actor Actor, scope Scope, branchID, stocktakeType string, basisIDs []string) (InventoryStocktake, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryStocktake{}, err
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return InventoryStocktake{}, err
	}
	stocktakeType = strings.ToUpper(strings.TrimSpace(stocktakeType))
	if !contains([]string{"FULL_STOCKTAKE", "CYCLE_COUNT"}, stocktakeType) {
		return InventoryStocktake{}, fieldError("stocktake_type", "Unknown stocktake type.")
	}
	var out InventoryStocktake
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM shop_inventory_revisions WHERE business_id=$1 AND branch_id=$2),0)`, actor.VendorID, branchID).Scan(&revision); err != nil {
			return err
		}
		out = InventoryStocktake{ID: newID("sist_"), BranchRef: branchID, Type: stocktakeType, SnapshotRevision: revision, Status: "COUNTING", Version: 1}
		if err := tx.QueryRow(ctx, `INSERT INTO shop_inventory_stocktakes(id,business_id,branch_id,stocktake_type,snapshot_revision,status) VALUES($1,$2,$3,$4,$5,'COUNTING') RETURNING started_at`, out.ID, actor.VendorID, branchID, stocktakeType, revision).Scan(&out.StartedAt); err != nil {
			return err
		}
		query := `SELECT stock_basis_id,variant_id,on_hand_quantity FROM shop_inventory_balances WHERE business_id=$1 AND branch_id=$2`
		args := []any{actor.VendorID, branchID}
		if len(basisIDs) > 0 {
			query += ` AND stock_basis_id=ANY($3)`
			args = append(args, basisIDs)
		}
		query += ` ORDER BY stock_basis_id`
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		type stocktakeLine struct {
			basis, variant string
			expected       int64
		}
		var lines []stocktakeLine
		for rows.Next() {
			var line stocktakeLine
			if err = rows.Scan(&line.basis, &line.variant, &line.expected); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, line)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, line := range lines {
			if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_stocktake_lines(id,business_id,stocktake_id,stock_basis_id,variant_id,expected_quantity) VALUES($1,$2,$3,$4,$5,$6)`, newID("sistl_"), actor.VendorID, out.ID, line.basis, line.variant, line.expected); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.stocktake.started", eventType: "shop.inventory.stocktake.started", aggregateType: "shop_inventory_stocktake", aggregateID: out.ID, branchID: branchID, version: 1, after: out, payload: out})
	})
	return out, err
}

func (s *Service) CountStocktakeLine(ctx context.Context, actor Actor, scope Scope, stocktakeID, basisID string, counted int64) error {
	if counted < 0 || counted > MaxInventoryQuantityPerLine {
		return fieldError("counted_quantity", "Count must be non-negative and within the controlled per-command limit.")
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var branch, status string
		if err := tx.QueryRow(ctx, `SELECT branch_id,status FROM shop_inventory_stocktakes WHERE id=$1 AND business_id=$2 FOR UPDATE`, stocktakeID, actor.VendorID).Scan(&branch, &status); err != nil {
			return mapDB(err, "Stocktake", CodeStocktakeNotFound)
		}
		if err := s.requireBranchScope(scope, branch); err != nil {
			return err
		}
		if status != "COUNTING" {
			return coded(apperrors.Conflict("Stocktake is not accepting counts.", nil), CodeReservationStateConflict)
		}
		tag, err := tx.Exec(ctx, `UPDATE shop_inventory_stocktake_lines SET counted_quantity=$4,variance_quantity=$4-expected_quantity,updated_at=now() WHERE stocktake_id=$1 AND business_id=$2 AND stock_basis_id=$3`, stocktakeID, actor.VendorID, basisID, counted)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errNotFound("Stocktake line", CodeStocktakeNotFound)
		}
		return nil
	})
}

func (s *Service) PostStocktake(ctx context.Context, actor Actor, scope Scope, id string) (InventoryStocktake, error) {
	var out InventoryStocktake
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var snapshot, current int64
		err := tx.QueryRow(ctx, `SELECT id,branch_id,stocktake_type,snapshot_revision,status,started_at,posted_at,version FROM shop_inventory_stocktakes WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.Type, &snapshot, &out.Status, &out.StartedAt, &out.PostedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Stocktake", CodeStocktakeNotFound)
		}
		out.SnapshotRevision = snapshot
		if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
			return err
		}
		if out.Status == "POSTED" {
			return nil
		}
		if out.Status != "COUNTING" && out.Status != "READY" {
			return coded(apperrors.Conflict("Stocktake cannot be posted from this state.", nil), CodeReservationStateConflict)
		}
		rows, err := tx.Query(ctx, `SELECT stock_basis_id,variant_id,expected_quantity,counted_quantity FROM shop_inventory_stocktake_lines WHERE stocktake_id=$1 AND business_id=$2 ORDER BY stock_basis_id FOR UPDATE`, id, actor.VendorID)
		if err != nil {
			return err
		}
		type line struct {
			basis, variant    string
			expected, counted int64
		}
		var lines []line
		for rows.Next() {
			var l line
			var counted *int64
			if err = rows.Scan(&l.basis, &l.variant, &l.expected, &counted); err != nil {
				rows.Close()
				return err
			}
			if counted == nil {
				rows.Close()
				return fieldError("counted_quantity", "Every stocktake line must be counted.")
			}
			l.counted = *counted
			lines = append(lines, l)
		}
		rows.Close()
		// Lock every counted balance before inspecting the branch revision. All
		// physical movements lock the balance first and then bump that revision,
		// so this ordering makes a concurrent movement either commit before the
		// snapshot check or observe the stocktake's new balance revision after it.
		for _, l := range lines {
			branchID, _, _, lockErr := lockInventoryBalance(ctx, tx, actor.VendorID, l.basis)
			if lockErr != nil {
				return lockErr
			}
			if branchID != out.BranchRef {
				return errNotFound("Stocktake line", CodeStocktakeNotFound)
			}
		}
		if err = tx.QueryRow(ctx, `SELECT revision FROM shop_inventory_revisions WHERE business_id=$1 AND branch_id=$2 FOR UPDATE`, actor.VendorID, out.BranchRef).Scan(&current); err != nil {
			return err
		}
		if current != snapshot {
			_, _ = tx.Exec(ctx, `UPDATE shop_inventory_stocktakes SET status='STALE',updated_at=now(),version=version+1 WHERE id=$1`, id)
			return coded(apperrors.Conflict("Inventory moved after this stocktake snapshot; recount is required.", nil), CodeInventoryStale)
		}
		for _, l := range lines {
			delta := l.counted - l.expected
			if delta != 0 {
				if err = applyPhysicalDelta(ctx, tx, actor.VendorID, out.BranchRef, l.basis, l.variant, delta, "DATA_CORRECTION", "STOCKTAKE", id, actor.StaffID); err != nil {
					return err
				}
			}
		}
		err = tx.QueryRow(ctx, `UPDATE shop_inventory_stocktakes SET status='POSTED',posted_at=now(),updated_at=now(),version=version+1 WHERE id=$1 RETURNING status,posted_at,version`, id).Scan(&out.Status, &out.PostedAt, &out.Version)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.stocktake.posted", eventType: "shop.inventory.stocktake.posted", aggregateType: "shop_inventory_stocktake", aggregateID: id, branchID: out.BranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

// sortedStrings returns a defensive deterministic copy for multi-resource
// operations such as transfer dispatch.
func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
