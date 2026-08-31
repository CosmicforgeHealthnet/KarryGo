-- Section 17D: Pricing, Promotions & Immutable Quote Lock.
--
-- The rows below are the exact commercial agreement shown to a customer.
-- They never become a payment or an order and are never rewritten. Approval
-- is append-only evidence in a separate table.

CREATE TABLE commerce_quotes (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    checkout_session_id TEXT NOT NULL,
    checkout_validation_revision BIGINT NOT NULL CHECK (checkout_validation_revision > 0),
    cart_id TEXT NOT NULL,
    cart_version BIGINT NOT NULL CHECK (cart_version > 0),
    fulfilment_plan_id TEXT NOT NULL,
    fulfilment_plan_revision BIGINT NOT NULL CHECK (fulfilment_plan_revision > 0),
    fulfilment_plan_version BIGINT NOT NULL CHECK (fulfilment_plan_version > 0),
    quote_version BIGINT NOT NULL CHECK (quote_version > 0),
    material_fingerprint TEXT NOT NULL CHECK (material_fingerprint ~ '^[0-9a-f]{64}$'),
    quote_fingerprint TEXT NOT NULL CHECK (quote_fingerprint ~ '^[0-9a-f]{64}$'),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    selected_fulfilment_mode TEXT NOT NULL
        CHECK (selected_fulfilment_mode IN ('SINGLE','FASTEST','BALANCED','SAVER')),
    address_snapshot JSONB NOT NULL CHECK (jsonb_typeof(address_snapshot) = 'object'),
    expires_at TIMESTAMPTZ NOT NULL,

    product_subtotal_minor BIGINT NOT NULL CHECK (product_subtotal_minor >= 0),
    max_product_subtotal_minor BIGINT NOT NULL CHECK (max_product_subtotal_minor >= product_subtotal_minor),
    delivery_subtotal_minor BIGINT NOT NULL CHECK (delivery_subtotal_minor >= 0),
    fee_subtotal_minor BIGINT NOT NULL CHECK (fee_subtotal_minor >= 0),
    tax_subtotal_minor BIGINT NOT NULL CHECK (tax_subtotal_minor >= 0),
    discount_total_minor BIGINT NOT NULL CHECK (discount_total_minor >= 0),
    subsidy_total_minor BIGINT NOT NULL CHECK (subsidy_total_minor BETWEEN 0 AND discount_total_minor),
    final_payable_minor BIGINT NOT NULL CHECK (final_payable_minor >= 0),
    quoted_total_minor BIGINT NOT NULL CHECK (quoted_total_minor >= 0),
    max_authorized_total_minor BIGINT NOT NULL CHECK (max_authorized_total_minor >= quoted_total_minor),
    funding_breakdown JSONB NOT NULL CHECK (jsonb_typeof(funding_breakdown) = 'object'),
    price_disclosure_snapshot JSONB NOT NULL CHECK (jsonb_typeof(price_disclosure_snapshot) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_quotes_checkout_scope_fk
        FOREIGN KEY (checkout_session_id, customer_id)
        REFERENCES commerce_checkout_sessions(id, customer_id),
    CONSTRAINT commerce_quotes_plan_scope_fk
        FOREIGN KEY (fulfilment_plan_id, customer_id)
        REFERENCES commerce_fulfilment_plans(id, customer_id),
    CONSTRAINT commerce_quotes_total_chk CHECK (
        final_payable_minor = quoted_total_minor
        AND product_subtotal_minor + delivery_subtotal_minor + fee_subtotal_minor + tax_subtotal_minor
            = final_payable_minor + discount_total_minor
        AND max_product_subtotal_minor + delivery_subtotal_minor + fee_subtotal_minor + tax_subtotal_minor
            = max_authorized_total_minor + discount_total_minor
    ),
    UNIQUE (id, customer_id),
    UNIQUE (checkout_session_id, quote_version),
    UNIQUE (quote_fingerprint)
);

CREATE INDEX commerce_quotes_customer_idx
    ON commerce_quotes(customer_id, created_at DESC, id);
CREATE INDEX commerce_quotes_checkout_idx
    ON commerce_quotes(checkout_session_id, quote_version DESC);
CREATE INDEX commerce_quotes_live_material_idx
    ON commerce_quotes(checkout_session_id, material_fingerprint, expires_at DESC);
CREATE INDEX commerce_quotes_expiry_idx ON commerce_quotes(expires_at, id);

CREATE TABLE commerce_quote_groups (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    checkout_group_id TEXT NOT NULL,
    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    product_subtotal_minor BIGINT NOT NULL CHECK (product_subtotal_minor >= 0),
    max_product_subtotal_minor BIGINT NOT NULL CHECK (max_product_subtotal_minor >= product_subtotal_minor),
    inventory_reservation_ref TEXT NOT NULL DEFAULT '',
    inventory_reservation_source_ref TEXT NOT NULL DEFAULT '',
    logistics_plan_group_ref TEXT NOT NULL,
    logistics_mission_ref TEXT NOT NULL DEFAULT '',
    delivery_funding JSONB NOT NULL CHECK (jsonb_typeof(delivery_funding) = 'object'),
    CONSTRAINT commerce_quote_groups_quote_scope_fk
        FOREIGN KEY (quote_id, customer_id) REFERENCES commerce_quotes(id, customer_id),
    UNIQUE (quote_id, checkout_group_id)
);

CREATE TABLE commerce_quote_lines (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    checkout_line_id TEXT NOT NULL,
    checkout_group_id TEXT NOT NULL,
    cart_item_id TEXT NOT NULL,
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    vendor_online_unit_price_minor BIGINT NOT NULL CHECK (vendor_online_unit_price_minor >= 0),
    price_reference TEXT NOT NULL,
    catalogue_revision BIGINT NOT NULL CHECK (catalogue_revision >= 0),
    inventory_revision BIGINT CHECK (inventory_revision IS NULL OR inventory_revision >= 0),
    quoted_total_minor BIGINT NOT NULL CHECK (quoted_total_minor >= 0),
    max_authorized_total_minor BIGINT NOT NULL CHECK (max_authorized_total_minor >= quoted_total_minor),
    price_context TEXT NOT NULL CHECK (price_context = 'VENDOR_ONLINE_PRICE'),
    store_price_parity_claimed BOOLEAN NOT NULL DEFAULT false CHECK (store_price_parity_claimed = false),
    UNIQUE (quote_id, cart_item_id)
);

CREATE TABLE commerce_quote_missions (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    fulfilment_mission_id TEXT NOT NULL,
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    UNIQUE (quote_id, fulfilment_mission_id)
);

CREATE TABLE commerce_quote_adjustments (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    adjustment_id TEXT NOT NULL,
    adjustment_type TEXT NOT NULL CHECK (adjustment_type IN ('DISCOUNT','FEE','TAX')),
    scope TEXT NOT NULL CHECK (scope IN ('PRODUCT','DELIVERY','ORDER')),
    funding_source TEXT NOT NULL
        CHECK (funding_source IN ('CUSTOMER','VENDOR','PLATFORM','DELIVERY_SUBSIDY','OTHER_APPROVED')),
    source_ref TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    UNIQUE (quote_id, adjustment_id)
);

CREATE TABLE commerce_quote_approvals (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL,
    customer_id TEXT NOT NULL,
    quote_fingerprint TEXT NOT NULL CHECK (quote_fingerprint ~ '^[0-9a-f]{64}$'),
    approved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_quote_approvals_quote_scope_fk
        FOREIGN KEY (quote_id, customer_id) REFERENCES commerce_quotes(id, customer_id),
    UNIQUE (quote_id),
    UNIQUE (quote_id, customer_id, quote_fingerprint)
);

CREATE TABLE commerce_quote_events (
    id TEXT PRIMARY KEY,
    quote_id TEXT NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('commerce.quote.created','commerce.quote.approved')),
    quote_version BIGINT NOT NULL CHECK (quote_version > 0),
    quote_fingerprint TEXT NOT NULL CHECK (quote_fingerprint ~ '^[0-9a-f]{64}$'),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX commerce_quote_events_quote_idx
    ON commerce_quote_events(quote_id, occurred_at, id);

-- Quote agreements and their snapshots are append-only. The approval and
-- event tables are append-only evidence too. This trigger is deliberately
-- database-level so a future repository cannot accidentally rewrite history.
CREATE OR REPLACE FUNCTION commerce_quote_reject_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce quote evidence is immutable';
END;
$$;

CREATE TRIGGER commerce_quotes_immutable
    BEFORE UPDATE OR DELETE ON commerce_quotes
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_groups_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_groups
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_lines_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_lines
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_missions_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_missions
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_adjustments_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_adjustments
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_approvals_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_approvals
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();
CREATE TRIGGER commerce_quote_events_immutable
    BEFORE UPDATE OR DELETE ON commerce_quote_events
    FOR EACH ROW EXECUTE FUNCTION commerce_quote_reject_mutation();

COMMENT ON TABLE commerce_quotes IS
'Section 17D immutable checkout quote. It is customer-approved commercial evidence, not a payment, ParentOrder or VendorOrder.';
COMMENT ON COLUMN commerce_quotes.quote_fingerprint IS
'Deterministically binds every material approved input including quote_version. A changed cart, allocation, address, plan, fee, tax or promotion creates a different agreement.';
COMMENT ON COLUMN commerce_quote_lines.price_context IS
'Explicit online-price disclosure. No row claims parity with a vendor physical-store price.';
