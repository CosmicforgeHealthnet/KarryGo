-- Section 17F: inventory finalization and logistics mission execution.
--
-- Two things happen once, and only once, a payment is actually held:
--
--   * the stock a VendorOrder reserved is CONFIRMED, so it is committed to
--     this purchase rather than merely held against a checkout that might
--     still lapse;
--   * a logistics mission is created for every platform-executed allocation.
--
-- Vendor-owned delivery deliberately gets NO platform mission: the vendor is
-- the executor, and its own delivery state already tracks that work.
-- Taxi is deliberately never an executor for commerce.

-- Inventory finalization timing, recorded on the allocation that owns the
-- stock. Without an explicit column, "has this reservation been confirmed"
-- would have to be inferred, and an inferred answer is how stock gets
-- double-committed.
ALTER TABLE commerce_vendor_orders
    ADD COLUMN inventory_state TEXT NOT NULL DEFAULT 'HELD'
        CHECK (inventory_state IN ('NOT_REQUIRED','HELD','CONFIRMED','COMMITTED','RELEASED'));

-- A vendor order that needs no stock is NOT_REQUIRED from the start, so the
-- finalization worker never tries to confirm a reservation that never existed.
UPDATE commerce_vendor_orders
   SET inventory_state = 'NOT_REQUIRED'
 WHERE reservation_required = false;

ALTER TABLE commerce_vendor_orders
    ADD CONSTRAINT commerce_vendor_orders_inventory_state_chk
        CHECK (
            (reservation_required = false AND inventory_state = 'NOT_REQUIRED')
            OR (reservation_required = true AND inventory_state <> 'NOT_REQUIRED')
        );

CREATE INDEX commerce_vendor_orders_inventory_pending_idx
    ON commerce_vendor_orders(inventory_state, updated_at, id)
    WHERE inventory_state IN ('HELD','CONFIRMED');

COMMENT ON COLUMN commerce_vendor_orders.inventory_state IS
'Section 17F stock finalization. HELD until payment is held, CONFIRMED once it is, COMMITTED on completion, RELEASED on cancellation. Explicit so stock is never double-committed on an inferred answer.';


-- One logistics mission per platform-executed fulfilment group.
CREATE TABLE commerce_order_missions (
    id TEXT PRIMARY KEY,

    -- Our own mission identity, and the idempotency key the executing
    -- authority keys on. UNIQUE here and there means a retried activation can
    -- never dispatch a second rider for the same goods.
    mission_ref TEXT NOT NULL UNIQUE,

    parent_order_id TEXT NOT NULL
        REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,

    -- The Section 17C plan mission this executes. One plan mission may cover
    -- several vendor allocations, which is exactly the consolidation case.
    plan_mission_ref TEXT NOT NULL,

    service TEXT NOT NULL CHECK (service IN ('DISPATCH','HAULING')),
    vehicle_class TEXT NOT NULL DEFAULT '',

    -- What the customer already paid for this delivery. The executing
    -- authority is told this figure and must not answer with another: an
    -- internal re-plan is a platform cost, never a customer charge.
    agreed_fee_minor BIGINT NOT NULL CHECK (agreed_fee_minor >= 0),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','ACCEPTED','REFUSED','FAILED')),

    -- The authority's own booking or trip identity, recorded not generated.
    external_ref TEXT NOT NULL DEFAULT '',
    refusal_code TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',

    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_order_missions_order_scope_fk
        FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id),

    CONSTRAINT commerce_order_missions_accepted_evidence_chk
        CHECK (status <> 'ACCEPTED' OR length(btrim(external_ref)) > 0),

    CONSTRAINT commerce_order_missions_refused_evidence_chk
        CHECK (status <> 'REFUSED' OR length(btrim(refusal_code)) > 0),

    -- One mission per plan group of one order.
    UNIQUE (parent_order_id, plan_mission_ref),
    UNIQUE (id, parent_order_id)
);

CREATE INDEX commerce_order_missions_parent_idx
    ON commerce_order_missions(parent_order_id, plan_mission_ref);
CREATE INDEX commerce_order_missions_pending_idx
    ON commerce_order_missions(status, updated_at, id)
    WHERE status = 'PENDING';


-- Which vendor allocations a mission carries. A consolidated mission names
-- more than one, so a later settlement can always answer which vendors a
-- single delivery served.
CREATE TABLE commerce_order_mission_allocations (
    id TEXT PRIMARY KEY,
    mission_id TEXT NOT NULL
        REFERENCES commerce_order_missions(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL,
    vendor_order_id TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_order_mission_allocations_mission_scope_fk
        FOREIGN KEY (mission_id, parent_order_id)
        REFERENCES commerce_order_missions(id, parent_order_id),

    CONSTRAINT commerce_order_mission_allocations_vendor_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id),

    -- A vendor allocation is carried by at most one mission.
    UNIQUE (vendor_order_id)
);

CREATE INDEX commerce_order_mission_allocations_mission_idx
    ON commerce_order_mission_allocations(mission_id, vendor_order_id);

COMMENT ON TABLE commerce_order_missions IS
'Section 17F logistics execution. Platform-executed allocations only: vendor-owned delivery has no platform mission, and Taxi is never an executor for commerce.';
COMMENT ON COLUMN commerce_order_missions.agreed_fee_minor IS
'What the customer already paid for this delivery. The executing authority is told this figure and its answer is refused if it differs, so an internal re-plan can never raise a customer charge.';
