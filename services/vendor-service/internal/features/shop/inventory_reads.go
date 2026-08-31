package shop

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

func boundedInventoryPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *Service) ListReceipts(ctx context.Context, actor Actor, scope Scope, branchID, status string, limit, offset int) ([]InventoryReceipt, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status = strings.ToUpper(strings.TrimSpace(status))
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,COALESCE(source_ref,''),received_at,status,posted_at,created_at,updated_at,version
		FROM shop_inventory_receipts WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR status=$3)
		ORDER BY received_at DESC,id LIMIT $4 OFFSET $5`, actor.VendorID, branchID, status, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []InventoryReceipt
	for rows.Next() {
		var item InventoryReceipt
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.SourceRef, &item.ReceivedAt, &item.Status, &item.PostedAt, &item.CreatedAt, &item.UpdatedAt, &item.Version); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func (s *Service) ListTransfers(ctx context.Context, actor Actor, scope Scope, branchID, status string, limit, offset int) ([]InventoryTransfer, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status = strings.ToUpper(strings.TrimSpace(status))
	rows, err := s.db.Query(ctx, `SELECT id,source_branch_id,destination_branch_id,status,notes,dispatched_at,received_at,created_at,updated_at,version
		FROM shop_inventory_transfers WHERE business_id=$1 AND (source_branch_id=$2 OR destination_branch_id=$2) AND ($3='' OR status=$3)
		ORDER BY created_at DESC,id LIMIT $4 OFFSET $5`, actor.VendorID, branchID, status, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []InventoryTransfer
	for rows.Next() {
		var item InventoryTransfer
		if err = rows.Scan(&item.ID, &item.SourceBranchRef, &item.DestinationBranchRef, &item.Status, &item.Notes, &item.DispatchedAt, &item.ReceivedAt, &item.CreatedAt, &item.UpdatedAt, &item.Version); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

type InventoryStocktakeLine struct {
	StockBasisRef    string `json:"stock_basis_ref"`
	VariantRef       string `json:"variant_ref"`
	ExpectedQuantity int64  `json:"expected_quantity"`
	CountedQuantity  *int64 `json:"counted_quantity,omitempty"`
	VarianceQuantity int64  `json:"variance_quantity"`
}

type InventoryStocktakeDetail struct {
	InventoryStocktake
	Lines []InventoryStocktakeLine `json:"lines"`
}

func (s *Service) GetStocktake(ctx context.Context, actor Actor, scope Scope, id string) (InventoryStocktakeDetail, error) {
	var out InventoryStocktakeDetail
	err := s.db.QueryRow(ctx, `SELECT id,branch_id,stocktake_type,snapshot_revision,status,started_at,posted_at,version FROM shop_inventory_stocktakes WHERE id=$1 AND business_id=$2`, id, actor.VendorID).
		Scan(&out.ID, &out.BranchRef, &out.Type, &out.SnapshotRevision, &out.Status, &out.StartedAt, &out.PostedAt, &out.Version)
	if err != nil {
		return out, mapDB(err, "Stocktake", CodeStocktakeNotFound)
	}
	if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
		return InventoryStocktakeDetail{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT stock_basis_id,variant_id,expected_quantity,counted_quantity,variance_quantity FROM shop_inventory_stocktake_lines WHERE business_id=$1 AND stocktake_id=$2 ORDER BY stock_basis_id`, actor.VendorID, id)
	if err != nil {
		return InventoryStocktakeDetail{}, err
	}
	defer rows.Close()
	out.Lines = []InventoryStocktakeLine{}
	for rows.Next() {
		var line InventoryStocktakeLine
		if err = rows.Scan(&line.StockBasisRef, &line.VariantRef, &line.ExpectedQuantity, &line.CountedQuantity, &line.VarianceQuantity); err != nil {
			return InventoryStocktakeDetail{}, err
		}
		out.Lines = append(out.Lines, line)
	}
	return out, rows.Err()
}

func (s *Service) ListStocktakes(ctx context.Context, actor Actor, scope Scope, branchID, status string, limit, offset int) ([]InventoryStocktake, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status = strings.ToUpper(strings.TrimSpace(status))
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,stocktake_type,snapshot_revision,status,started_at,posted_at,version FROM shop_inventory_stocktakes WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR status=$3) ORDER BY started_at DESC,id LIMIT $4 OFFSET $5`, actor.VendorID, branchID, status, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []InventoryStocktake
	for rows.Next() {
		var item InventoryStocktake
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.Type, &item.SnapshotRevision, &item.Status, &item.StartedAt, &item.PostedAt, &item.Version); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func (s *Service) ListQuarantines(ctx context.Context, actor Actor, scope Scope, branchID, status string, limit, offset int) ([]InventoryQuarantine, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status = strings.ToUpper(strings.TrimSpace(status))
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,stock_basis_id,variant_id,quantity,reason_code,status,created_at,resolved_at,version FROM shop_inventory_quarantines WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR status=$3) ORDER BY created_at DESC,id LIMIT $4 OFFSET $5`, actor.VendorID, branchID, status, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []InventoryQuarantine
	for rows.Next() {
		var item InventoryQuarantine
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.StockBasisRef, &item.VariantRef, &item.Quantity, &item.ReasonCode, &item.Status, &item.CreatedAt, &item.ResolvedAt, &item.Version); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func (s *Service) ListReorderRules(ctx context.Context, actor Actor, scope Scope, branchID, status string, limit, offset int) ([]ReorderRule, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status = strings.ToUpper(strings.TrimSpace(status))
	rows, err := s.db.Query(ctx, `SELECT r.id,r.branch_id,r.stock_basis_id,r.variant_id,r.minimum_quantity,r.target_quantity,
		GREATEST(0,r.target_quantity-COALESCE(a.available,0)),r.expiring_soon_hours,r.status,r.version
		FROM shop_inventory_reorder_rules r LEFT JOIN LATERAL (
			SELECT COALESCE(sum(GREATEST(0,l.on_hand_quantity-l.quarantined_quantity-
			COALESCE((SELECT sum(ra.quantity) FROM shop_inventory_reservation_allocations ra
			JOIN shop_inventory_reservation_lines rl ON rl.id=ra.reservation_line_id AND rl.business_id=ra.business_id
			JOIN shop_inventory_reservations rv ON rv.id=rl.reservation_id AND rv.business_id=rl.business_id
			WHERE ra.business_id=l.business_id AND ra.lot_id=l.id AND ra.status='HELD' AND rl.status='HELD'
			AND rv.status IN ('HELD','CONFIRMED') AND (rv.expires_at IS NULL OR rv.expires_at>now())),0))),0) available
			FROM shop_inventory_lots l WHERE l.business_id=r.business_id AND l.stock_basis_id=r.stock_basis_id
			AND l.status='ACTIVE' AND (l.expires_at IS NULL OR l.expires_at>now())) a ON true
		WHERE r.business_id=$1 AND r.branch_id=$2 AND ($3='' OR r.status=$3)
		ORDER BY r.updated_at DESC,r.id LIMIT $4 OFFSET $5`, actor.VendorID, branchID, status, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []ReorderRule
	for rows.Next() {
		var item ReorderRule
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.StockBasisRef, &item.VariantRef, &item.MinimumQuantity, &item.TargetQuantity, &item.RecommendedReorder, &item.ExpiringSoonHours, &item.Status, &item.Version); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

type InventoryAlert struct {
	ID            string          `json:"id"`
	BranchRef     string          `json:"branch_ref"`
	StockBasisRef *string         `json:"stock_basis_ref,omitempty"`
	Type          string          `json:"alert_type"`
	Severity      string          `json:"severity"`
	Details       json.RawMessage `json:"details"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func (s *Service) ListInventoryAlerts(ctx context.Context, actor Actor, scope Scope, branchID, status, alertType string, limit, offset int) ([]InventoryAlert, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset = boundedInventoryPage(limit, offset)
	status, alertType = strings.ToUpper(strings.TrimSpace(status)), strings.ToUpper(strings.TrimSpace(alertType))
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,stock_basis_id,alert_type,severity,details,status,created_at,updated_at FROM shop_inventory_alerts WHERE business_id=$1 AND branch_id=$2 AND ($3='' OR status=$3) AND ($4='' OR alert_type=$4) ORDER BY created_at DESC,id LIMIT $5 OFFSET $6`, actor.VendorID, branchID, status, alertType, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []InventoryAlert
	for rows.Next() {
		var item InventoryAlert
		if err = rows.Scan(&item.ID, &item.BranchRef, &item.StockBasisRef, &item.Type, &item.Severity, &item.Details, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func (s *Service) TransitionInventoryAlert(ctx context.Context, actor Actor, scope Scope, id, target string) (InventoryAlert, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryAlert{}, err
	}
	target = strings.ToUpper(strings.TrimSpace(target))
	if target != "ACKNOWLEDGED" && target != "RESOLVED" {
		return InventoryAlert{}, fieldError("status", "Use ACKNOWLEDGED or RESOLVED.")
	}
	var out InventoryAlert
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT id,branch_id,stock_basis_id,alert_type,severity,details,status,created_at,updated_at FROM shop_inventory_alerts WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.StockBasisRef, &out.Type, &out.Severity, &out.Details, &current, &out.CreatedAt, &out.UpdatedAt); err != nil {
			return mapDB(err, "Inventory alert", CodeAdjustmentNotFound)
		}
		if err := s.requireBranchScope(scope, out.BranchRef); err != nil {
			return err
		}
		if current == target || (current == "RESOLVED" && target == "ACKNOWLEDGED") {
			out.Status = current
			return nil
		}
		if current == "RESOLVED" {
			return coded(apperrors.Conflict("Resolved alerts cannot be reopened.", nil), CodeReservationStateConflict)
		}
		if err := tx.QueryRow(ctx, `UPDATE shop_inventory_alerts SET status=$3,updated_at=now() WHERE id=$1 AND business_id=$2 RETURNING status,updated_at`, id, actor.VendorID, target).Scan(&out.Status, &out.UpdatedAt); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.alert." + strings.ToLower(target), eventType: "shop.inventory.alert." + strings.ToLower(target), aggregateType: "shop_inventory_alert", aggregateID: id, branchID: out.BranchRef, version: out.UpdatedAt.UnixNano(), after: out, payload: out})
	})
	return out, err
}

type BulkOperationRowView struct {
	RowNumber        int             `json:"row_number"`
	Payload          json.RawMessage `json:"payload"`
	ValidationErrors json.RawMessage `json:"validation_errors"`
	Status           string          `json:"status"`
}

type BulkOperationDetail struct {
	BulkOperationJob
	Rows []BulkOperationRowView `json:"rows"`
}

func (s *Service) GetBulkOperation(ctx context.Context, actor Actor, scope Scope, id string) (BulkOperationDetail, error) {
	var out BulkOperationDetail
	err := s.db.QueryRow(ctx, `SELECT id,COALESCE(branch_id,''),operation_type,dry_run,status,row_count,error_count,created_at,updated_at,completed_at FROM shop_bulk_operation_jobs WHERE id=$1 AND business_id=$2`, id, actor.VendorID).
		Scan(&out.ID, &out.BranchRef, &out.OperationType, &out.DryRun, &out.Status, &out.RowCount, &out.ErrorCount, &out.CreatedAt, &out.UpdatedAt, &out.CompletedAt)
	if err != nil {
		return out, mapDB(err, "Bulk operation", CodeAdjustmentNotFound)
	}
	if out.BranchRef != "" {
		if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
			return BulkOperationDetail{}, err
		}
	} else if !scope.Global {
		return BulkOperationDetail{}, errForbidden()
	}
	rows, err := s.db.Query(ctx, `SELECT row_number,payload,validation_errors,status FROM shop_bulk_operation_rows WHERE job_id=$1 AND business_id=$2 ORDER BY row_number`, id, actor.VendorID)
	if err != nil {
		return BulkOperationDetail{}, err
	}
	defer rows.Close()
	out.Rows = []BulkOperationRowView{}
	for rows.Next() {
		var row BulkOperationRowView
		if err = rows.Scan(&row.RowNumber, &row.Payload, &row.ValidationErrors, &row.Status); err != nil {
			return BulkOperationDetail{}, err
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}
