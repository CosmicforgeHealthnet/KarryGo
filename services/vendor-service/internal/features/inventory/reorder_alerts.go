package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *StockService) SetReorderRule(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input SetReorderRuleInput,
) (
	ReorderRule,
	error,
) {
	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.InventoryItemID =
		strings.TrimSpace(
			input.InventoryItemID,
		)

	input.Status =
		strings.ToUpper(
			strings.TrimSpace(
				input.Status,
			),
		)

	if input.Status == "" {
		input.Status =
			ReorderActive
	}

	if input.MinimumQuantity < 0 ||
		input.TargetQuantity <=
			input.MinimumQuantity {

		return ReorderRule{},
			ErrReorderRuleInvalid
	}

	if input.ExpiringSoonHours != nil &&
		(*input.ExpiringSoonHours < 1 ||
			*input.ExpiringSoonHours > 8760) {

		return ReorderRule{},
			ErrReorderRuleInvalid
	}

	if input.Status !=
		ReorderActive &&
		input.Status !=
			ReorderInactive &&
		input.Status !=
			ReorderArchived {

		return ReorderRule{},
			ErrReorderRuleInvalid
	}

	var out ReorderRule

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			_, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			err =
				tx.QueryRow(
					ctx,
					`
                    INSERT INTO inventory_reorder_rules (
                        id,
                        business_id,
                        branch_id,
                        inventory_item_id,
                        minimum_quantity,
                        target_quantity,
                        expiring_soon_hours,
                        supplier_ref,
                        status
                    )
                    VALUES (
                        $1,$2,$3,$4,$5,$6,$7,
                        NULLIF($8,''),
                        $9
                    )
                    ON CONFLICT (
                        business_id,
                        branch_id,
                        inventory_item_id
                    )
                    DO UPDATE SET
                        minimum_quantity=
                            EXCLUDED.minimum_quantity,
                        target_quantity=
                            EXCLUDED.target_quantity,
                        expiring_soon_hours=
                            EXCLUDED.expiring_soon_hours,
                        supplier_ref=
                            EXCLUDED.supplier_ref,
                        status=
                            EXCLUDED.status,
                        updated_at=now(),
                        version=
                            inventory_reorder_rules.version+1
                    RETURNING
                        id,
                        business_id,
                        branch_id,
                        inventory_item_id,
                        minimum_quantity,
                        target_quantity,
                        expiring_soon_hours,
                        COALESCE(supplier_ref,''),
                        status,
                        version
                    `,
					stockID("irr_"),
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
					input.MinimumQuantity,
					input.TargetQuantity,
					input.ExpiringSoonHours,
					strings.TrimSpace(
						input.SupplierRef,
					),
					input.Status,
				).Scan(
					&out.ID,
					&out.BusinessID,
					&out.BranchID,
					&out.InventoryItemID,
					&out.MinimumQuantity,
					&out.TargetQuantity,
					&out.ExpiringSoonHours,
					&out.SupplierRef,
					&out.Status,
					&out.Version,
				)

			if err != nil {
				return err
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					input.BranchID,
				)

			revision, _ :=
				currentInventoryRevisionQ(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
				)

			if revision == 0 {
				revision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.reorder_rule.changed",
				"inventory.reorder_rule.changed",
				"inventory_reorder_rule",
				out.ID,
				"",
				revision,
				map[string]any{
					"reorder_rule": out,
				},
			)
		},
	)

	return out, err
}

func upsertAlertTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	itemID string,
	alertType string,
	severity string,
	dedupeKey string,
	details map[string]any,
) (
	string,
	error,
) {
	raw, err :=
		json.Marshal(
			details,
		)

	if err != nil {
		return "", err
	}

	id :=
		stockID("ial_")

	err =
		tx.QueryRow(
			ctx,
			`
            INSERT INTO inventory_alerts (
                id,
                business_id,
                branch_id,
                inventory_item_id,
                alert_type,
                severity,
                dedupe_key,
                details,
                status
            )
            VALUES (
                $1,$2,$3,NULLIF($4,''),
                $5,$6,$7,$8,'OPEN'
            )
            ON CONFLICT (
                business_id,
                dedupe_key
            )
            DO UPDATE SET
                severity=
                    EXCLUDED.severity,
                details=
                    EXCLUDED.details,
                status=
                    CASE
                        WHEN inventory_alerts.status='RESOLVED'
                        THEN 'OPEN'
                        ELSE inventory_alerts.status
                    END,
                resolved_at=
                    CASE
                        WHEN inventory_alerts.status='RESOLVED'
                        THEN NULL
                        ELSE inventory_alerts.resolved_at
                    END,
                updated_at=now()
            RETURNING id
            `,
			id,
			businessID,
			branchID,
			itemID,
			alertType,
			severity,
			dedupeKey,
			json.RawMessage(raw),
		).Scan(
			&id,
		)

	return id, err
}

func resolveAlertKeyTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	dedupeKey string,
) error {
	_, err :=
		tx.Exec(
			ctx,
			`
            UPDATE inventory_alerts
               SET status='RESOLVED',
                   resolved_at=COALESCE(resolved_at,now()),
                   updated_at=now()
             WHERE business_id=$1
               AND dedupe_key=$2
               AND status <> 'RESOLVED'
            `,
			businessID,
			dedupeKey,
		)

	return err
}

func (s *StockService) generateInventoryAlertsPrimary(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	if s == nil ||
		s.db == nil {
		return 0,
			ErrDependencyMissing
	}

	if limit <= 0 {
		limit = 200
	}

	if limit > 1000 {
		limit = 1000
	}

	tx, err :=
		s.db.Begin(
			ctx,
		)

	if err != nil {
		return 0, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	generated := 0

	rows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                r.business_id,
                r.branch_id,
                r.inventory_item_id,
                r.minimum_quantity,
                r.target_quantity,
                r.expiring_soon_hours,
                COALESCE(
                    a.available_quantity,
                    0
                ),
                COALESCE(
                    a.backorder_quantity,
                    0
                )
              FROM inventory_reorder_rules r
              LEFT JOIN inventory_balance_availability a
                ON a.business_id=r.business_id
               AND a.branch_id=r.branch_id
               AND a.inventory_item_id=r.inventory_item_id
             WHERE r.status='ACTIVE'
             ORDER BY r.business_id,
                      r.branch_id,
                      r.inventory_item_id
             LIMIT $1
            `,
			limit,
		)

	if err != nil {
		return 0, err
	}

	type ruleRow struct {
		BusinessID string
		BranchID   string
		ItemID     string

		Minimum int64
		Target  int64

		ExpiringHours *int

		Available int64
		Backorder int64
	}

	var rules []ruleRow

	for rows.Next() {
		var item ruleRow

		if err := rows.Scan(
			&item.BusinessID,
			&item.BranchID,
			&item.ItemID,
			&item.Minimum,
			&item.Target,
			&item.ExpiringHours,
			&item.Available,
			&item.Backorder,
		); err != nil {
			rows.Close()
			return 0, err
		}

		rules =
			append(
				rules,
				item,
			)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}

	rows.Close()

	for _, rule := range rules {

		lowKey :=
			"LOW_STOCK:" +
				rule.BranchID +
				":" +
				rule.ItemID

		outKey :=
			"OUT_OF_STOCK:" +
				rule.BranchID +
				":" +
				rule.ItemID

		if rule.Available <= 0 {
			_, err :=
				upsertAlertTx(
					ctx,
					tx,
					rule.BusinessID,
					rule.BranchID,
					rule.ItemID,
					AlertOutOfStock,
					"CRITICAL",
					outKey,
					map[string]any{
						"available_quantity": rule.Available,

						"minimum_quantity": rule.Minimum,

						"target_quantity": rule.Target,

						"suggested_reorder_quantity": rule.Target,
					},
				)

			if err != nil {
				return generated, err
			}

			generated++

			if err :=
				resolveAlertKeyTx(
					ctx,
					tx,
					rule.BusinessID,
					lowKey,
				); err != nil {

				return generated, err
			}
		} else if rule.Available <=
			rule.Minimum {

			_, err :=
				upsertAlertTx(
					ctx,
					tx,
					rule.BusinessID,
					rule.BranchID,
					rule.ItemID,
					AlertLowStock,
					"WARNING",
					lowKey,
					map[string]any{
						"available_quantity": rule.Available,

						"minimum_quantity": rule.Minimum,

						"target_quantity": rule.Target,

						"suggested_reorder_quantity": rule.Target -
							rule.Available,
					},
				)

			if err != nil {
				return generated, err
			}

			generated++

			if err :=
				resolveAlertKeyTx(
					ctx,
					tx,
					rule.BusinessID,
					outKey,
				); err != nil {

				return generated, err
			}
		} else {
			if err :=
				resolveAlertKeyTx(
					ctx,
					tx,
					rule.BusinessID,
					lowKey,
				); err != nil {

				return generated, err
			}

			if err :=
				resolveAlertKeyTx(
					ctx,
					tx,
					rule.BusinessID,
					outKey,
				); err != nil {

				return generated, err
			}
		}

		backorderKey :=
			"BACKORDER:" +
				rule.BranchID +
				":" +
				rule.ItemID

		if rule.Backorder > 0 {
			_, err :=
				upsertAlertTx(
					ctx,
					tx,
					rule.BusinessID,
					rule.BranchID,
					rule.ItemID,
					AlertBackorder,
					"WARNING",
					backorderKey,
					map[string]any{
						"backorder_quantity": rule.Backorder,
					},
				)

			if err != nil {
				return generated, err
			}

			generated++
		} else if err :=
			resolveAlertKeyTx(
				ctx,
				tx,
				rule.BusinessID,
				backorderKey,
			); err != nil {

			return generated, err
		}

		if rule.ExpiringHours != nil {
			var expiring int64

			err :=
				tx.QueryRow(
					ctx,
					`
                    SELECT COALESCE(
                        SUM(
                            on_hand_quantity
                            - reserved_quantity
                            - quarantined_quantity
                        ),
                        0
                    )
                      FROM inventory_lots
                     WHERE business_id=$1
                       AND branch_id=$2
                       AND inventory_item_id=$3
                       AND status='ACTIVE'
                       AND expires_at > now()
                       AND expires_at <=
                           now()
                           + make_interval(
                               hours => $4
                           )
                    `,
					rule.BusinessID,
					rule.BranchID,
					rule.ItemID,
					*rule.ExpiringHours,
				).Scan(
					&expiring,
				)

			if err != nil {
				return generated, err
			}

			key :=
				"EXPIRING_SOON:" +
					rule.BranchID +
					":" +
					rule.ItemID

			if expiring > 0 {
				_, err :=
					upsertAlertTx(
						ctx,
						tx,
						rule.BusinessID,
						rule.BranchID,
						rule.ItemID,
						AlertExpiringSoon,
						"WARNING",
						key,
						map[string]any{
							"quantity": expiring,

							"threshold_hours": *rule.ExpiringHours,
						},
					)

				if err != nil {
					return generated, err
				}

				generated++
			} else if err :=
				resolveAlertKeyTx(
					ctx,
					tx,
					rule.BusinessID,
					key,
				); err != nil {

				return generated, err
			}
		}
	}

	// Detect impossible materialized balances against lot truth.
	mismatchRows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                b.business_id,
                b.branch_id,
                b.inventory_item_id,
                b.on_hand_quantity,
                b.reserved_quantity,
                b.quarantined_quantity,
                COALESCE(SUM(l.on_hand_quantity),0),
                COALESCE(SUM(l.reserved_quantity),0),
                COALESCE(SUM(l.quarantined_quantity),0)
              FROM inventory_balances b
              LEFT JOIN inventory_lots l
                ON l.business_id=b.business_id
               AND l.branch_id=b.branch_id
               AND l.inventory_item_id=b.inventory_item_id
             GROUP BY
                b.business_id,
                b.branch_id,
                b.inventory_item_id,
                b.on_hand_quantity,
                b.reserved_quantity,
                b.quarantined_quantity
             HAVING
                b.on_hand_quantity
                    <> COALESCE(SUM(l.on_hand_quantity),0)
                OR b.reserved_quantity
                    <> COALESCE(SUM(l.reserved_quantity),0)
                OR b.quarantined_quantity
                    <> COALESCE(SUM(l.quarantined_quantity),0)
             LIMIT $1
            `,
			limit,
		)

	if err != nil {
		return generated, err
	}

	for mismatchRows.Next() {
		var (
			business string
			branch   string
			itemID   string

			balanceOnHand     int64
			balanceReserved   int64
			balanceQuarantine int64
			lotOnHand         int64
			lotReserved       int64
			lotQuarantine     int64
		)

		if err :=
			mismatchRows.Scan(
				&business,
				&branch,
				&itemID,
				&balanceOnHand,
				&balanceReserved,
				&balanceQuarantine,
				&lotOnHand,
				&lotReserved,
				&lotQuarantine,
			); err != nil {

			mismatchRows.Close()
			return generated, err
		}

		_, err :=
			upsertAlertTx(
				ctx,
				tx,
				business,
				branch,
				itemID,
				AlertBalanceMismatch,
				"CRITICAL",
				"BALANCE_MISMATCH:"+
					branch+
					":"+
					itemID,
				map[string]any{
					"balance_on_hand": balanceOnHand,

					"lot_on_hand": lotOnHand,

					"balance_reserved": balanceReserved,

					"lot_reserved": lotReserved,

					"balance_quarantined": balanceQuarantine,

					"lot_quarantined": lotQuarantine,
				},
			)

		if err != nil {
			mismatchRows.Close()
			return generated, err
		}

		generated++
	}

	if err := mismatchRows.Err(); err != nil {
		mismatchRows.Close()
		return generated, err
	}

	mismatchRows.Close()

	transferRows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                t.business_id,
                t.source_branch_id,
                t.id,
                l.id,
                l.source_inventory_item_id,
                l.damaged_quantity,
                l.missing_quantity
              FROM inventory_transfers t
              JOIN inventory_transfer_lines l
                ON l.transfer_id=t.id
               AND l.business_id=t.business_id
             WHERE (
                l.damaged_quantity > 0
                OR l.missing_quantity > 0
             )
             ORDER BY t.updated_at DESC
             LIMIT $1
            `,
			limit,
		)

	if err != nil {
		return generated, err
	}

	for transferRows.Next() {
		var (
			business string
			branch   string
			transfer string
			line     string
			item     string
			damaged  int64
			missing  int64
		)

		if err :=
			transferRows.Scan(
				&business,
				&branch,
				&transfer,
				&line,
				&item,
				&damaged,
				&missing,
			); err != nil {

			transferRows.Close()
			return generated, err
		}

		_, err :=
			upsertAlertTx(
				ctx,
				tx,
				business,
				branch,
				item,
				AlertTransferException,
				"CRITICAL",
				fmt.Sprintf(
					"TRANSFER_EXCEPTION:%s:%s",
					transfer,
					line,
				),
				map[string]any{
					"transfer_id": transfer,

					"line_id": line,

					"damaged_quantity": damaged,

					"missing_quantity": missing,
				},
			)

		if err != nil {
			transferRows.Close()
			return generated, err
		}

		generated++
	}

	if err := transferRows.Err(); err != nil {
		transferRows.Close()
		return generated, err
	}

	transferRows.Close()

	if err := tx.Commit(ctx); err != nil {
		return generated, err
	}

	return generated, nil
}

func scanAlert(
	row interface {
		Scan(...any) error
	},
) (
	InventoryAlert,
	error,
) {
	var (
		out InventoryAlert
		raw []byte
	)

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.BranchID,
			&out.InventoryItemID,
			&out.Type,
			&out.Severity,
			&out.DedupeKey,
			&raw,
			&out.Status,
			&out.CreatedAt,
			&out.UpdatedAt,
			&out.AcknowledgedAt,
			&out.ResolvedAt,
		)

	if err == nil {
		_ = json.Unmarshal(
			raw,
			&out.Details,
		)
	}

	return out, err
}

func (s *StockService) ListAlerts(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
	status string,
	limit int,
) (
	[]InventoryAlert,
	error,
) {
	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			branchID,
			false,
		); err != nil {

		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 500 {
		limit = 500
	}

	rows, err :=
		s.db.Query(
			ctx,
			`
            SELECT
                id,
                business_id,
                branch_id,
                COALESCE(inventory_item_id,''),
                alert_type,
                severity,
                dedupe_key,
                details,
                status,
                created_at,
                updated_at,
                acknowledged_at,
                resolved_at
              FROM inventory_alerts
             WHERE business_id=$1
               AND branch_id=$2
               AND (
                   $3=''
                   OR status=$3
               )
             ORDER BY created_at DESC,id DESC
             LIMIT $4
            `,
			actor.BusinessID,
			branchID,
			strings.ToUpper(
				strings.TrimSpace(
					status,
				),
			),
			limit,
		)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []InventoryAlert

	for rows.Next() {
		alert, err :=
			scanAlert(
				rows,
			)

		if err != nil {
			return nil, err
		}

		out =
			append(
				out,
				alert,
			)
	}

	return out, rows.Err()
}

func (s *StockService) AlertBranch(
	ctx context.Context,
	businessID string,
	id string,
) (
	string,
	error,
) {
	var branch string

	err :=
		s.db.QueryRow(
			ctx,
			`
            SELECT branch_id
              FROM inventory_alerts
             WHERE id=$1
               AND business_id=$2
            `,
			id,
			businessID,
		).Scan(
			&branch,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "",
			ErrAlertNotFound
	}

	return branch, err
}

func (s *StockService) SetAlertStatus(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
	status string,
) (
	InventoryAlert,
	error,
) {
	branchID, err :=
		s.AlertBranch(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return InventoryAlert{},
			err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			branchID,
			true,
		); err != nil {

		return InventoryAlert{},
			err
	}

	status =
		strings.ToUpper(
			strings.TrimSpace(
				status,
			),
		)

	if status !=
		AlertAcknowledged &&
		status !=
			AlertResolved {

		return InventoryAlert{},
			ErrInvalidSource
	}

	tx, err :=
		s.db.Begin(
			ctx,
		)

	if err != nil {
		return InventoryAlert{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var out InventoryAlert

	timeColumn :=
		"acknowledged_at"

	if status ==
		AlertResolved {

		timeColumn =
			"resolved_at"
	}

	query :=
		`
        UPDATE inventory_alerts
           SET status=$3,
               ` +
			timeColumn +
			`=COALESCE(` +
			timeColumn +
			`,now()),
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
        RETURNING
            id,
            business_id,
            branch_id,
            COALESCE(inventory_item_id,''),
            alert_type,
            severity,
            dedupe_key,
            details,
            status,
            created_at,
            updated_at,
            acknowledged_at,
            resolved_at
        `

	out, err =
		scanAlert(
			tx.QueryRow(
				ctx,
				query,
				id,
				actor.BusinessID,
				status,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return InventoryAlert{},
			ErrAlertNotFound
	}

	if err != nil {
		return InventoryAlert{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return InventoryAlert{}, err
	}

	return out, nil
}
