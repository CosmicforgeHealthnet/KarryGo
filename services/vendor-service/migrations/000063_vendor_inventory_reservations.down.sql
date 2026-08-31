DROP VIEW IF EXISTS inventory_balance_availability;

CREATE VIEW inventory_balance_availability AS
SELECT
    b.id,
    b.business_id,
    b.branch_id,
    b.inventory_item_id,

    b.on_hand_quantity,
    b.reserved_quantity,
    b.quarantined_quantity,
    b.in_transit_quantity,
    b.incoming_quantity,
    b.backorder_quantity,

    CASE
        WHEN COUNT(l.id) > 0 THEN
            COALESCE(
                SUM(
                    CASE
                        WHEN l.status = 'ACTIVE'
                         AND (
                             l.expires_at IS NULL
                             OR l.expires_at > now()
                         )
                        THEN GREATEST(
                            l.on_hand_quantity
                            - l.reserved_quantity
                            - l.quarantined_quantity,
                            0
                        )
                        ELSE 0
                    END
                ),
                0
            )
        ELSE
            GREATEST(
                b.on_hand_quantity
                - b.reserved_quantity
                - b.quarantined_quantity,
                0
            )
    END AS available_quantity,

    COALESCE(
        SUM(
            CASE
                WHEN l.expires_at IS NOT NULL
                 AND l.expires_at <= now()
                 AND l.on_hand_quantity > 0
                THEN l.on_hand_quantity
                ELSE 0
            END
        ),
        0
    ) AS expired_quantity,

    b.revision,
    b.updated_at

FROM inventory_balances b

LEFT JOIN inventory_lots l
  ON l.business_id = b.business_id
 AND l.branch_id = b.branch_id
 AND l.inventory_item_id = b.inventory_item_id

GROUP BY
    b.id,
    b.business_id,
    b.branch_id,
    b.inventory_item_id,
    b.on_hand_quantity,
    b.reserved_quantity,
    b.quarantined_quantity,
    b.in_transit_quantity,
    b.incoming_quantity,
    b.backorder_quantity,
    b.revision,
    b.updated_at;

DROP TABLE IF EXISTS inventory_reservation_allocations;
DROP TABLE IF EXISTS inventory_reservation_lines;
DROP TABLE IF EXISTS inventory_reservations;