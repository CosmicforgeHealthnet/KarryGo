-- Section 17E: ParentOrder and multi-vendor VendorOrders.
--
-- One customer commerce purchase becomes exactly one ParentOrder and one
-- financially and operationally isolated VendorOrder per vendor/branch
-- allocation. The rows here are created from an approved, unexpired Section
-- 17D quote and never recompute a historical price from the live catalogue.
--
-- Nothing here executes a payment. payment_operation_ref exists so a later
-- section can bind an operation to an order that already exists; it is never
-- written by a customer request.

CREATE TABLE commerce_parent_orders (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,

    -- The exact approved commercial agreement. UNIQUE is what makes order
    -- creation idempotent: one approved quote can only ever produce one
    -- ParentOrder, no matter how many times creation is retried.
    quote_id TEXT NOT NULL UNIQUE,
    quote_version BIGINT NOT NULL CHECK (quote_version > 0),
    quote_fingerprint TEXT NOT NULL CHECK (quote_fingerprint ~ '^[0-9a-f]{64}$'),

    checkout_session_id TEXT NOT NULL,
    checkout_validation_revision BIGINT NOT NULL CHECK (checkout_validation_revision > 0),
    cart_id TEXT NOT NULL,
    cart_version BIGINT NOT NULL CHECK (cart_version > 0),
    fulfilment_plan_id TEXT NOT NULL,
    fulfilment_plan_revision BIGINT NOT NULL CHECK (fulfilment_plan_revision > 0),
    fulfilment_plan_version BIGINT NOT NULL CHECK (fulfilment_plan_version > 0),

    domain TEXT NOT NULL CHECK (domain IN ('FOOD','SHOP')),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    selected_fulfilment_mode TEXT NOT NULL
        CHECK (selected_fulfilment_mode IN ('SINGLE','FASTEST','BALANCED','SAVER')),

    -- Immutable references/snapshots. A later Customer Service edit to the
    -- saved location, or a later cart receiver change, never rewrites what
    -- this purchase was agreed against.
    address_snapshot JSONB NOT NULL CHECK (jsonb_typeof(address_snapshot) = 'object'),
    has_receiver BOOLEAN NOT NULL DEFAULT false,
    receiver_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(receiver_snapshot) = 'object'),

    -- The customer-approved figures, copied verbatim from the quote.
    customer_approved_total_minor BIGINT NOT NULL CHECK (customer_approved_total_minor >= 0),
    max_authorized_total_minor BIGINT NOT NULL
        CHECK (max_authorized_total_minor >= customer_approved_total_minor),
    product_subtotal_minor BIGINT NOT NULL CHECK (product_subtotal_minor >= 0),
    delivery_subtotal_minor BIGINT NOT NULL CHECK (delivery_subtotal_minor >= 0),
    fee_subtotal_minor BIGINT NOT NULL CHECK (fee_subtotal_minor >= 0),
    tax_subtotal_minor BIGINT NOT NULL CHECK (tax_subtotal_minor >= 0),
    discount_total_minor BIGINT NOT NULL CHECK (discount_total_minor >= 0),
    subsidy_total_minor BIGINT NOT NULL
        CHECK (subsidy_total_minor BETWEEN 0 AND discount_total_minor),
    funding_breakdown JSONB NOT NULL CHECK (jsonb_typeof(funding_breakdown) = 'object'),
    price_disclosure_snapshot JSONB NOT NULL
        CHECK (jsonb_typeof(price_disclosure_snapshot) = 'object'),

    -- Parent-scope customer money that belongs to no single vendor.
    platform_fee_tax_minor BIGINT NOT NULL DEFAULT 0 CHECK (platform_fee_tax_minor >= 0),

    -- Set by a later payment-capable section, never by a customer request.
    payment_operation_ref TEXT NOT NULL DEFAULT '',

    state TEXT NOT NULL DEFAULT 'CREATED'
        CHECK (state IN ('CREATED','IN_PROGRESS','COMPLETED','PARTIALLY_COMPLETED','CANCELLED')),

    vendor_order_count INTEGER NOT NULL CHECK (vendor_order_count > 0),

    -- Prepared, never executed. Money is not moved by Section 17E.
    refund_prepared_minor BIGINT NOT NULL DEFAULT 0 CHECK (refund_prepared_minor >= 0),
    settlement_prepared_minor BIGINT NOT NULL DEFAULT 0 CHECK (settlement_prepared_minor >= 0),

    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    terminal_at TIMESTAMPTZ,

    CONSTRAINT commerce_parent_orders_quote_scope_fk
        FOREIGN KEY (quote_id, customer_id) REFERENCES commerce_quotes(id, customer_id),

    -- The approved total decomposes exactly. It is never re-derived from a
    -- live catalogue, so the identity is asserted here once, at write time.
    CONSTRAINT commerce_parent_orders_total_chk CHECK (
        product_subtotal_minor + delivery_subtotal_minor + fee_subtotal_minor + tax_subtotal_minor
            = customer_approved_total_minor + discount_total_minor
    ),

    CONSTRAINT commerce_parent_orders_receiver_chk CHECK (
        has_receiver = false OR receiver_snapshot ? 'receiver_name'
    ),

    CONSTRAINT commerce_parent_orders_terminal_chk CHECK (
        (state IN ('COMPLETED','PARTIALLY_COMPLETED','CANCELLED')) = (terminal_at IS NOT NULL)
    ),

    UNIQUE (id, customer_id)
);

CREATE INDEX commerce_parent_orders_customer_idx
    ON commerce_parent_orders(customer_id, created_at DESC, id);
CREATE INDEX commerce_parent_orders_state_idx
    ON commerce_parent_orders(state, updated_at DESC, id);
CREATE INDEX commerce_parent_orders_checkout_idx
    ON commerce_parent_orders(checkout_session_id, id);


-- One financially and operationally isolated vendor/branch allocation.
--
-- Every money column here is THIS vendor's own share. No column ever holds a
-- gross customer figure, so a reader cannot accidentally pay one vendor with
-- another vendor's allocation.
CREATE TABLE commerce_vendor_orders (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,

    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    domain TEXT NOT NULL CHECK (domain IN ('FOOD','SHOP')),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    -- The Section 17D/17B allocation this vendor order was cut from.
    quote_group_ref TEXT NOT NULL,

    product_subtotal_minor BIGINT NOT NULL CHECK (product_subtotal_minor >= 0),
    max_product_subtotal_minor BIGINT NOT NULL
        CHECK (max_product_subtotal_minor >= product_subtotal_minor),

    -- Allocated discount/subsidy, by who funded it.
    product_vendor_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (product_vendor_funded_minor >= 0),
    product_platform_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (product_platform_funded_minor >= 0),
    product_other_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (product_other_funded_minor >= 0),
    order_vendor_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (order_vendor_funded_minor >= 0),
    order_platform_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (order_platform_funded_minor >= 0),
    order_other_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (order_other_funded_minor >= 0),

    -- Allocated delivery responsibility and its funding.
    delivery_responsibility TEXT NOT NULL
        CHECK (delivery_responsibility IN ('PLATFORM_LOGISTICS','VENDOR_OWN_DELIVERY','NONE')),
    delivery_fee_beneficiary TEXT NOT NULL
        CHECK (delivery_fee_beneficiary IN ('VENDOR','PLATFORM_PROVIDER','NONE')),
    delivery_gross_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_gross_minor >= 0),
    delivery_customer_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_customer_minor >= 0),
    delivery_vendor_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_vendor_funded_minor >= 0),
    delivery_platform_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_platform_funded_minor >= 0),
    -- Benefit funded by an approved source that is neither the vendor nor the
    -- platform. It has its own column so a delivery subsidy is never
    -- misreported as platform money.
    delivery_other_funded_minor BIGINT NOT NULL DEFAULT 0 CHECK (delivery_other_funded_minor >= 0),

    -- What this vendor order accounts for of the customer's approved total,
    -- and what the vendor is owed for it.
    customer_payable_minor BIGINT NOT NULL CHECK (customer_payable_minor >= 0),
    vendor_payable_minor BIGINT NOT NULL CHECK (vendor_payable_minor >= 0),

    -- Section 14 stock isolation. Each vendor order owns its own reservation
    -- relationship; nothing here can reach another vendor's stock.
    reservation_required BOOLEAN NOT NULL DEFAULT false,
    inventory_reservation_ref TEXT NOT NULL DEFAULT '',
    inventory_reservation_source_ref TEXT NOT NULL DEFAULT '',

    fulfilment_mode TEXT NOT NULL
        CHECK (fulfilment_mode IN ('SINGLE','FASTEST','BALANCED','SAVER')),
    logistics_plan_group_ref TEXT NOT NULL DEFAULT '',
    logistics_mission_ref TEXT NOT NULL DEFAULT '',

    vendor_own_delivery_state TEXT NOT NULL DEFAULT 'NOT_APPLICABLE'
        CHECK (vendor_own_delivery_state IN
            ('NOT_APPLICABLE','PENDING','DISPATCHED','DELIVERED','CANCELLED')),

    status TEXT NOT NULL DEFAULT 'CREATED'
        CHECK (status IN
            ('CREATED','PREPARING','READY','HANDED_OVER','DELIVERED','COMPLETED','CANCELLED')),

    cancel_reason_code TEXT NOT NULL DEFAULT '',
    cancelled_by TEXT NOT NULL DEFAULT ''
        CHECK (cancelled_by IN ('','VENDOR','CUSTOMER','PLATFORM')),

    -- Prepared allocations. Section 17E computes and records them; it moves
    -- no money and creates no payment operation.
    refund_allocated_minor BIGINT NOT NULL DEFAULT 0 CHECK (refund_allocated_minor >= 0),
    settlement_allocated_minor BIGINT NOT NULL DEFAULT 0 CHECK (settlement_allocated_minor >= 0),

    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    terminal_at TIMESTAMPTZ,

    CONSTRAINT commerce_vendor_orders_parent_scope_fk
        FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id),

    -- Delivery funding stays attributable: the three funded parts always
    -- reconstruct the gross fee actually charged for this allocation.
    CONSTRAINT commerce_vendor_orders_delivery_funding_chk CHECK (
        delivery_customer_minor + delivery_vendor_funded_minor
            + delivery_platform_funded_minor + delivery_other_funded_minor
            = delivery_gross_minor
    ),

    -- Vendor-owned delivery is the only case where delivery money may land on
    -- the vendor. Platform delivery never does.
    CONSTRAINT commerce_vendor_orders_beneficiary_chk CHECK (
        (delivery_responsibility = 'VENDOR_OWN_DELIVERY' AND delivery_fee_beneficiary = 'VENDOR')
        OR (delivery_responsibility = 'PLATFORM_LOGISTICS' AND delivery_fee_beneficiary = 'PLATFORM_PROVIDER')
        OR (delivery_responsibility = 'NONE' AND delivery_fee_beneficiary = 'NONE' AND delivery_gross_minor = 0)
    ),

    CONSTRAINT commerce_vendor_orders_vendor_own_state_chk CHECK (
        (delivery_responsibility = 'VENDOR_OWN_DELIVERY')
            = (vendor_own_delivery_state <> 'NOT_APPLICABLE')
    ),

    -- Reserved stock must name the Section 14 identity holding it.
    CONSTRAINT commerce_vendor_orders_reservation_chk CHECK (
        reservation_required = false
        OR length(btrim(inventory_reservation_ref)) > 0
    ),

    -- Food never carries a Shop stock reservation, exactly as Section 17B.
    CONSTRAINT commerce_vendor_orders_food_reservation_chk CHECK (
        domain <> 'FOOD'
        OR (reservation_required = false AND inventory_reservation_ref = '')
    ),

    CONSTRAINT commerce_vendor_orders_cancel_evidence_chk CHECK (
        (status = 'CANCELLED') = (cancelled_by <> '')
    ),

    CONSTRAINT commerce_vendor_orders_terminal_chk CHECK (
        (status IN ('COMPLETED','CANCELLED')) = (terminal_at IS NOT NULL)
    ),

    -- A settlement allocation is only sayable for a completed vendor order,
    -- and a refund allocation only for a cancelled one.
    CONSTRAINT commerce_vendor_orders_settlement_chk CHECK (
        status = 'COMPLETED' OR settlement_allocated_minor = 0
    ),
    CONSTRAINT commerce_vendor_orders_refund_chk CHECK (
        status = 'CANCELLED' OR refund_allocated_minor = 0
    ),

    -- One vendor/branch allocation per parent order, and one vendor order per
    -- quote group. A retry can never duplicate an allocation.
    UNIQUE (parent_order_id, quote_group_ref),
    UNIQUE (parent_order_id, business_id, branch_id),
    UNIQUE (id, parent_order_id),
    UNIQUE (id, customer_id)
);

CREATE INDEX commerce_vendor_orders_parent_idx
    ON commerce_vendor_orders(parent_order_id, quote_group_ref);
CREATE INDEX commerce_vendor_orders_branch_queue_idx
    ON commerce_vendor_orders(business_id, branch_id, status, created_at DESC, id);
CREATE INDEX commerce_vendor_orders_customer_idx
    ON commerce_vendor_orders(customer_id, created_at DESC, id);


-- The item snapshots a historical receipt is rendered from.
--
-- Append-only: a later catalogue rename, repricing or delisting must never
-- change what this purchase said.
CREATE TABLE commerce_vendor_order_lines (
    id TEXT PRIMARY KEY,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,

    quote_line_ref TEXT NOT NULL,
    cart_item_id TEXT NOT NULL,

    item_name TEXT NOT NULL,
    item_ref TEXT NOT NULL,
    variant_ref TEXT NOT NULL DEFAULT '',
    listing_ref TEXT NOT NULL DEFAULT '',
    sell_unit_ref TEXT NOT NULL DEFAULT '',

    quantity BIGINT NOT NULL CHECK (quantity > 0),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    quoted_total_minor BIGINT NOT NULL CHECK (quoted_total_minor >= 0),
    max_authorized_total_minor BIGINT NOT NULL
        CHECK (max_authorized_total_minor >= quoted_total_minor),

    discount_allocated_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_allocated_minor >= 0),
    fee_allocated_minor BIGINT NOT NULL DEFAULT 0 CHECK (fee_allocated_minor >= 0),
    tax_allocated_minor BIGINT NOT NULL DEFAULT 0 CHECK (tax_allocated_minor >= 0),

    pricing_basis TEXT NOT NULL,
    requested_measure_ref TEXT NOT NULL DEFAULT '',
    expected_measure_quantity BIGINT NOT NULL DEFAULT 0,
    measure_underfill_bps INTEGER NOT NULL DEFAULT 0
        CHECK (measure_underfill_bps BETWEEN 0 AND 10000),
    measure_overfill_bps INTEGER NOT NULL DEFAULT 0
        CHECK (measure_overfill_bps BETWEEN 0 AND 10000),

    price_reference TEXT NOT NULL,
    catalogue_revision BIGINT NOT NULL CHECK (catalogue_revision >= 0),
    inventory_revision BIGINT CHECK (inventory_revision IS NULL OR inventory_revision >= 0),
    inventory_item_ref TEXT NOT NULL DEFAULT '',

    -- The same explicit disclosure Section 17D made. No row claims parity
    -- with a vendor physical-store price.
    online_price_disclosure TEXT NOT NULL
        CHECK (online_price_disclosure = 'VENDOR_ONLINE_PRICE'),
    store_price_parity_claimed BOOLEAN NOT NULL DEFAULT false
        CHECK (store_price_parity_claimed = false),

    delivery_commitment JSONB NOT NULL CHECK (jsonb_typeof(delivery_commitment) = 'object'),
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_vendor_order_lines_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id),

    UNIQUE (vendor_order_id, cart_item_id),
    UNIQUE (parent_order_id, quote_line_ref)
);

CREATE INDEX commerce_vendor_order_lines_vendor_idx
    ON commerce_vendor_order_lines(vendor_order_id, cart_item_id);


-- The explicit allocation register.
--
-- Every part of the customer's approved total is recorded here as its own
-- attributable row: which vendor order it belongs to, who benefits from it,
-- and who funded it. "Who actually paid for this delivery" is answerable by
-- reading rows, never by re-deriving business rules afterwards.
CREATE TABLE commerce_order_allocations (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,

    -- NULL only for parent-scope money that belongs to no single vendor
    -- (platform fee and tax). Product and delivery money is always a
    -- particular vendor order's.
    vendor_order_id TEXT,

    allocation_type TEXT NOT NULL
        CHECK (allocation_type IN ('PRODUCT','DELIVERY','FEE','TAX','DISCOUNT')),
    beneficiary_type TEXT NOT NULL
        CHECK (beneficiary_type IN ('VENDOR','PLATFORM_PROVIDER','PLATFORM','CUSTOMER')),
    beneficiary_ref TEXT NOT NULL DEFAULT '',
    funding_source TEXT NOT NULL
        CHECK (funding_source IN ('CUSTOMER','VENDOR','PLATFORM','DELIVERY_SUBSIDY','OTHER_APPROVED')),
    source_ref TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_order_allocations_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id),

    CONSTRAINT commerce_order_allocations_vendor_scope_chk CHECK (
        vendor_order_id IS NOT NULL
        OR allocation_type IN ('FEE','TAX','DISCOUNT')
    ),

    CONSTRAINT commerce_order_allocations_vendor_beneficiary_chk CHECK (
        beneficiary_type <> 'VENDOR' OR length(btrim(beneficiary_ref)) > 0
    )
);

CREATE INDEX commerce_order_allocations_parent_idx
    ON commerce_order_allocations(parent_order_id, allocation_type, id);
CREATE INDEX commerce_order_allocations_vendor_idx
    ON commerce_order_allocations(vendor_order_id, allocation_type, id);


-- Append-only lifecycle evidence for both aggregates.
CREATE TABLE commerce_order_events (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT,
    customer_id TEXT NOT NULL,

    action TEXT NOT NULL CHECK (action IN (
        'commerce.order.created',
        'commerce.order.state_changed',
        'commerce.vendor_order.created',
        'commerce.vendor_order.accepted',
        'commerce.vendor_order.ready',
        'commerce.vendor_order.handed_over',
        'commerce.vendor_order.delivered',
        'commerce.vendor_order.completed',
        'commerce.vendor_order.cancelled'
    )),

    actor_type TEXT NOT NULL CHECK (actor_type IN ('CUSTOMER','VENDOR_STAFF','PLATFORM','SYSTEM')),
    actor_ref TEXT NOT NULL DEFAULT '',

    from_status TEXT NOT NULL DEFAULT '',
    to_status TEXT NOT NULL DEFAULT '',

    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_order_events_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id)
);

CREATE INDEX commerce_order_events_parent_idx
    ON commerce_order_events(parent_order_id, occurred_at, id);
CREATE INDEX commerce_order_events_vendor_idx
    ON commerce_order_events(vendor_order_id, occurred_at, id);


-- Snapshots, allocations and lifecycle evidence are append-only. The rule is
-- enforced at the database rather than in a repository, so a future writer
-- cannot rewrite what a customer was charged or which vendor earned what.
--
-- The two aggregate rows are deliberately NOT frozen: their status, prepared
-- allocations and version are exactly what a lifecycle transition changes.
CREATE OR REPLACE FUNCTION commerce_order_reject_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce order evidence is immutable';
END;
$$;

CREATE TRIGGER commerce_vendor_order_lines_immutable
    BEFORE UPDATE OR DELETE ON commerce_vendor_order_lines
    FOR EACH ROW EXECUTE FUNCTION commerce_order_reject_mutation();
CREATE TRIGGER commerce_order_allocations_immutable
    BEFORE UPDATE OR DELETE ON commerce_order_allocations
    FOR EACH ROW EXECUTE FUNCTION commerce_order_reject_mutation();
CREATE TRIGGER commerce_order_events_immutable
    BEFORE UPDATE OR DELETE ON commerce_order_events
    FOR EACH ROW EXECUTE FUNCTION commerce_order_reject_mutation();

COMMENT ON TABLE commerce_parent_orders IS
'Section 17E ParentOrder: one customer commerce purchase, created exactly once from one approved Section 17D quote. Aggregate state is derived from its VendorOrders and never hides a mixed outcome.';
COMMENT ON TABLE commerce_vendor_orders IS
'Section 17E VendorOrder: one financially and operationally isolated vendor/branch allocation. Every money column is this vendor own share; no column holds a gross customer figure.';
COMMENT ON TABLE commerce_order_allocations IS
'Section 17E allocation register. Delivery, platform and discount allocations are recorded explicitly so no vendor can ever receive another vendor allocation.';
COMMENT ON COLUMN commerce_parent_orders.payment_operation_ref IS
'Bound by a later payment-capable section. Section 17E never accepts it from a customer request and never executes a payment.';
