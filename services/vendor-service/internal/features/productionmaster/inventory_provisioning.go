package productionmaster

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ensureMasterInventoryItemTx finds or creates the branch stock item for an
// ingredient or packaging material, plus its (zero) stock balance row, so an
// inventory-controlled material can be bought, counted and transferred
// without the merchant mapping a stock identity by hand.
func ensureMasterInventoryItemTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	itemType string,
	sourceRef string,
	unitCode string,
	family string,
) (string, error) {
	measureRef := strings.ToUpper(strings.TrimSpace(unitCode))
	unitKind := strings.ToUpper(strings.TrimSpace(family))
	switch unitKind {
	case "MASS", "VOLUME", "COUNT":
	default:
		unitKind = "CONFIGURED_MEASURE"
	}

	newID, err := randomID("invitem_")
	if err != nil {
		return "", err
	}

	var id string
	err = tx.QueryRow(ctx, `
        INSERT INTO inventory_items (
            id, business_id, branch_id, item_type, source_ref,
            stock_measure_ref, stock_measure_owner_key, stock_unit_kind,
            stock_basis_mode, tracking_mode, lot_policy,
            lot_tracking_enabled, status, version
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'SAME_UNIT','SIMPLE','FIFO',false,'ACTIVE',1)
        ON CONFLICT (business_id, branch_id, item_type, source_ref, stock_measure_ref)
        DO UPDATE SET status='ACTIVE', updated_at=now()
        RETURNING id
        `,
		newID, businessID, branchID, itemType, sourceRef,
		measureRef, strings.ToLower(itemType)+":"+sourceRef, unitKind,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	balanceID, err := randomID("ib_")
	if err != nil {
		return "", err
	}
	// The balance row starts at zero; the database only accepts balance
	// inserts inside a declared inventory mutation.
	if _, err = tx.Exec(ctx,
		`SELECT set_config('karrygo.inventory_mutation','1',true)`); err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
        INSERT INTO inventory_balances (id, business_id, branch_id, inventory_item_id)
        VALUES ($1,$2,$3,$4)
        ON CONFLICT (business_id, branch_id, inventory_item_id) DO NOTHING
        `, balanceID, businessID, branchID, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}
