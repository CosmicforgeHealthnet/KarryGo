CREATE TABLE commerce_payment_intents (
    payment_intent_id uuid PRIMARY KEY REFERENCES payment_intents(id),
    parent_commerce_reference text NOT NULL UNIQUE,
    quote_reference text NOT NULL,
    quote_version bigint NOT NULL CHECK (quote_version > 0),
    quote_fingerprint text NOT NULL,
    final_authorized_minor bigint NOT NULL CHECK (final_authorized_minor > 0),
    max_authorized_minor bigint NOT NULL CHECK (max_authorized_minor >= final_authorized_minor),
    allocation_fingerprint text NOT NULL,
    status text NOT NULL CHECK (status IN (
        'CREATED','HELD','PARTIALLY_RELEASED','RELEASED',
        'PARTIALLY_REFUNDED','REFUNDED','FAILED'
    )),
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key)
);

CREATE INDEX commerce_payment_intents_quote_idx
    ON commerce_payment_intents(quote_reference, quote_version);

CREATE TABLE commerce_payment_allocations (
    id text PRIMARY KEY,
    payment_intent_id uuid NOT NULL REFERENCES commerce_payment_intents(payment_intent_id),
    allocation_reference text NOT NULL,
    scope text NOT NULL CHECK (scope IN ('VENDOR','PLATFORM')),
    vendor_order_id text NOT NULL DEFAULT '',
    vendor_id text NOT NULL DEFAULT '',
    branch_id text NOT NULL DEFAULT '',
    assigned_provider_type text NOT NULL DEFAULT '',
    assigned_provider_id text NOT NULL DEFAULT '',
    delivery_responsibility text NOT NULL DEFAULT 'NONE'
        CHECK (delivery_responsibility IN ('NONE','PLATFORM','VENDOR')),
    currency text NOT NULL,
    customer_payable_minor bigint NOT NULL CHECK (customer_payable_minor >= 0),
    max_authorized_minor bigint NOT NULL CHECK (max_authorized_minor >= customer_payable_minor),
    merchandise_gross_minor bigint NOT NULL DEFAULT 0 CHECK (merchandise_gross_minor >= 0),
    vendor_product_discount_minor bigint NOT NULL DEFAULT 0 CHECK (vendor_product_discount_minor >= 0),
    platform_product_discount_minor bigint NOT NULL DEFAULT 0 CHECK (platform_product_discount_minor >= 0),
    platform_commission_minor bigint NOT NULL DEFAULT 0 CHECK (platform_commission_minor >= 0),
    delivery_customer_minor bigint NOT NULL DEFAULT 0 CHECK (delivery_customer_minor >= 0),
    vendor_delivery_subsidy_minor bigint NOT NULL DEFAULT 0 CHECK (vendor_delivery_subsidy_minor >= 0),
    platform_delivery_subsidy_minor bigint NOT NULL DEFAULT 0 CHECK (platform_delivery_subsidy_minor >= 0),
    other_funding_minor bigint NOT NULL DEFAULT 0 CHECK (other_funding_minor >= 0),
    vendor_merchandise_payable_minor bigint NOT NULL DEFAULT 0 CHECK (vendor_merchandise_payable_minor >= 0),
    vendor_delivery_payable_minor bigint NOT NULL DEFAULT 0 CHECK (vendor_delivery_payable_minor >= 0),
    provider_delivery_payable_minor bigint NOT NULL DEFAULT 0 CHECK (provider_delivery_payable_minor >= 0),
    platform_revenue_minor bigint NOT NULL DEFAULT 0 CHECK (platform_revenue_minor >= 0),
    vendor_released_minor bigint NOT NULL DEFAULT 0 CHECK (vendor_released_minor >= 0),
    delivery_released_minor bigint NOT NULL DEFAULT 0 CHECK (delivery_released_minor >= 0),
    refunded_minor bigint NOT NULL DEFAULT 0 CHECK (refunded_minor >= 0),
    status text NOT NULL DEFAULT 'CREATED' CHECK (status IN (
        'CREATED','HELD','PARTIALLY_RELEASED','RELEASED',
        'REFUND_PENDING','PARTIALLY_REFUNDED','REFUNDED'
    )),
    snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    snapshot_fingerprint text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (payment_intent_id, allocation_reference),
    CHECK ((scope = 'VENDOR' AND vendor_order_id <> '' AND vendor_id <> '') OR
           (scope = 'PLATFORM' AND vendor_order_id = '' AND vendor_id = '')),
    CHECK ((assigned_provider_type = '' AND assigned_provider_id = '') OR
           (assigned_provider_type <> '' AND assigned_provider_id <> '')),
    CHECK (vendor_released_minor <= vendor_merchandise_payable_minor + platform_revenue_minor),
    CHECK (delivery_released_minor <= vendor_delivery_payable_minor + provider_delivery_payable_minor),
    CHECK (refunded_minor <= max_authorized_minor + platform_product_discount_minor + platform_delivery_subsidy_minor + other_funding_minor),
    CHECK (customer_payable_minor + platform_product_discount_minor + platform_delivery_subsidy_minor + other_funding_minor =
           vendor_merchandise_payable_minor + vendor_delivery_payable_minor +
           provider_delivery_payable_minor + platform_revenue_minor)
);

CREATE UNIQUE INDEX commerce_payment_allocations_vendor_order_idx
    ON commerce_payment_allocations(vendor_order_id)
    WHERE vendor_order_id <> '';
CREATE INDEX commerce_payment_allocations_intent_status_idx
    ON commerce_payment_allocations(payment_intent_id, status);
CREATE INDEX commerce_payment_allocations_vendor_idx
    ON commerce_payment_allocations(vendor_id, status, created_at)
    WHERE vendor_id <> '';

CREATE TABLE commerce_financial_operations (
    id text PRIMARY KEY,
    payment_intent_id uuid NOT NULL REFERENCES commerce_payment_intents(payment_intent_id),
    allocation_id text REFERENCES commerce_payment_allocations(id),
    operation_type text NOT NULL CHECK (operation_type IN (
        'HOLD','VENDOR_RELEASE','DELIVERY_RELEASE','PARTIAL_REFUND'
    )),
    source_reference text NOT NULL,
    item_reference text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    status text NOT NULL CHECK (status IN ('APPLIED','PENDING_LIABILITY','FAILED')),
    amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
    ledger_reference text NOT NULL DEFAULT '',
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (operation_type, idempotency_key)
);

CREATE INDEX commerce_financial_operations_source_idx
    ON commerce_financial_operations(operation_type, source_reference, created_at);
CREATE INDEX commerce_financial_operations_allocation_idx
    ON commerce_financial_operations(allocation_id, operation_type, created_at)
    WHERE allocation_id IS NOT NULL;
CREATE UNIQUE INDEX commerce_refund_allocation_item_idx
    ON commerce_financial_operations(allocation_id, item_reference)
    WHERE operation_type = 'PARTIAL_REFUND';
CREATE UNIQUE INDEX commerce_release_allocation_type_idx
    ON commerce_financial_operations(allocation_id, operation_type)
    WHERE operation_type IN ('VENDOR_RELEASE', 'DELIVERY_RELEASE');

CREATE OR REPLACE FUNCTION guard_commerce_intent_snapshot_immutability()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.payment_intent_id,NEW.parent_commerce_reference,NEW.quote_reference,NEW.quote_version,
           NEW.quote_fingerprint,NEW.final_authorized_minor,NEW.max_authorized_minor,NEW.allocation_fingerprint,
           NEW.idempotency_key,NEW.request_fingerprint)
       IS DISTINCT FROM
       ROW(OLD.payment_intent_id,OLD.parent_commerce_reference,OLD.quote_reference,OLD.quote_version,
           OLD.quote_fingerprint,OLD.final_authorized_minor,OLD.max_authorized_minor,OLD.allocation_fingerprint,
           OLD.idempotency_key,OLD.request_fingerprint) THEN
        RAISE EXCEPTION 'commerce payment snapshot is immutable';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_intent_snapshot_immutable
BEFORE UPDATE ON commerce_payment_intents
FOR EACH ROW EXECUTE FUNCTION guard_commerce_intent_snapshot_immutability();

CREATE OR REPLACE FUNCTION guard_commerce_allocation_snapshot_immutability()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.payment_intent_id,NEW.allocation_reference,NEW.scope,NEW.vendor_order_id,NEW.vendor_id,NEW.branch_id,
           NEW.delivery_responsibility,NEW.currency,NEW.customer_payable_minor,NEW.max_authorized_minor,
           NEW.merchandise_gross_minor,NEW.vendor_product_discount_minor,NEW.platform_product_discount_minor,
           NEW.platform_commission_minor,NEW.delivery_customer_minor,NEW.vendor_delivery_subsidy_minor,
           NEW.platform_delivery_subsidy_minor,NEW.other_funding_minor,NEW.vendor_merchandise_payable_minor,
           NEW.vendor_delivery_payable_minor,NEW.provider_delivery_payable_minor,NEW.platform_revenue_minor,
           NEW.snapshot,NEW.snapshot_fingerprint)
       IS DISTINCT FROM
       ROW(OLD.payment_intent_id,OLD.allocation_reference,OLD.scope,OLD.vendor_order_id,OLD.vendor_id,OLD.branch_id,
           OLD.delivery_responsibility,OLD.currency,OLD.customer_payable_minor,OLD.max_authorized_minor,
           OLD.merchandise_gross_minor,OLD.vendor_product_discount_minor,OLD.platform_product_discount_minor,
           OLD.platform_commission_minor,OLD.delivery_customer_minor,OLD.vendor_delivery_subsidy_minor,
           OLD.platform_delivery_subsidy_minor,OLD.other_funding_minor,OLD.vendor_merchandise_payable_minor,
           OLD.vendor_delivery_payable_minor,OLD.provider_delivery_payable_minor,OLD.platform_revenue_minor,
           OLD.snapshot,OLD.snapshot_fingerprint) THEN
        RAISE EXCEPTION 'commerce allocation snapshot is immutable';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_allocation_snapshot_immutable
BEFORE UPDATE ON commerce_payment_allocations
FOR EACH ROW EXECUTE FUNCTION guard_commerce_allocation_snapshot_immutability();
