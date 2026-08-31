-- Section 17F: payment binding and total-price guarantee.
--
-- Vendor Commerce is not a money authority. Nothing here is a ledger, a
-- balance or a settlement. These tables bind an approved commercial agreement
-- to a payment operation performed by payment-wallet-service, and record what
-- that authority answered.
--
-- One leg per VendorOrder is deliberate. The Wallet contract settles and
-- refunds a payment intent as a whole, to a single provider, so a single parent
-- intent could never refund one vendor while settling another. Splitting at
-- creation makes multi-vendor refund isolation structural.

-- An order exists before it is paid, so it needs a payment state that is
-- separate from its fulfilment aggregate state. A PENDING_PAYMENT order is
-- never fulfilment-ready.
ALTER TABLE commerce_parent_orders
    ADD COLUMN payment_state TEXT NOT NULL DEFAULT 'PENDING_PAYMENT'
        CHECK (payment_state IN (
            'PENDING_PAYMENT','AUTHORIZING','HELD','FAILED','COMPENSATING','COMPENSATED'
        ));

CREATE INDEX commerce_parent_orders_payment_state_idx
    ON commerce_parent_orders(payment_state, updated_at DESC, id)
    WHERE payment_state IN ('PENDING_PAYMENT','AUTHORIZING','COMPENSATING');

COMMENT ON COLUMN commerce_parent_orders.payment_state IS
'Section 17F payment lifecycle, separate from the fulfilment aggregate state. An order that is not HELD has not been paid and may not be worked as if it were.';


-- One payment operation per order, bound to the exact approved agreement.
CREATE TABLE commerce_payment_operations (
    id TEXT PRIMARY KEY,

    -- One operation per order, and one per quote. A retried start finds the
    -- existing operation instead of creating a second financial effect.
    parent_order_id TEXT NOT NULL UNIQUE
        REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    quote_id TEXT NOT NULL UNIQUE,

    -- The binding. Every column is server-resolved from approved evidence and
    -- none of it is ever accepted from a request body. A materially different
    -- input is a different binding and is refused under this operation.
    quote_version BIGINT NOT NULL CHECK (quote_version > 0),
    quote_fingerprint TEXT NOT NULL CHECK (quote_fingerprint ~ '^[0-9a-f]{64}$'),
    quoted_total_minor BIGINT NOT NULL CHECK (quoted_total_minor >= 0),
    max_authorized_total_minor BIGINT NOT NULL
        CHECK (max_authorized_total_minor >= quoted_total_minor),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    allocation_fingerprint TEXT NOT NULL
        CHECK (allocation_fingerprint ~ '^[0-9a-f]{64}$'),

    payment_method TEXT NOT NULL DEFAULT 'wallet'
        CHECK (payment_method IN ('wallet')),

    state TEXT NOT NULL DEFAULT 'PENDING_PAYMENT'
        CHECK (state IN (
            'PENDING_PAYMENT','AUTHORIZING','HELD','FAILED','COMPENSATING','COMPENSATED'
        )),
    failure_code TEXT NOT NULL DEFAULT '',

    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at TIMESTAMPTZ,
    terminal_at TIMESTAMPTZ,

    CONSTRAINT commerce_payment_operations_order_scope_fk
        FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id),

    CONSTRAINT commerce_payment_operations_terminal_chk CHECK (
        (state IN ('FAILED','COMPENSATED')) = (terminal_at IS NOT NULL)
    ),

    CONSTRAINT commerce_payment_operations_failure_chk CHECK (
        state <> 'FAILED' OR failure_code <> ''
    ),

    UNIQUE (id, parent_order_id)
);

CREATE INDEX commerce_payment_operations_state_idx
    ON commerce_payment_operations(state, updated_at, id)
    WHERE state IN ('PENDING_PAYMENT','AUTHORIZING','COMPENSATING');
CREATE INDEX commerce_payment_operations_customer_idx
    ON commerce_payment_operations(customer_id, created_at DESC, id);


-- One payment leg. A vendor leg pays exactly one VendorOrder's own share; the
-- platform leg pays the parent-scope fee and tax that belongs to no vendor.
CREATE TABLE commerce_payment_legs (
    id TEXT PRIMARY KEY,
    payment_operation_id TEXT NOT NULL
        REFERENCES commerce_payment_operations(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL
        REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,

    scope TEXT NOT NULL CHECK (scope IN ('VENDOR','PLATFORM')),

    -- NULL only for the platform leg.
    vendor_order_id TEXT,
    business_id TEXT NOT NULL DEFAULT '',

    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    -- Deterministic, so a retried start reuses it and the Wallet authority
    -- returns the same intent rather than creating a second one.
    idempotency_key TEXT NOT NULL UNIQUE,

    -- The Wallet authority's own identities, recorded and never generated here.
    intent_reference TEXT NOT NULL DEFAULT '',
    intent_id TEXT NOT NULL DEFAULT '',
    -- The raw status the authority reported, kept verbatim so the Vendor-side
    -- reading can always be checked against the authority's own word.
    authority_status TEXT NOT NULL DEFAULT '',

    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','HELD','SETTLED','REFUNDED','FAILED')),
    failure_reason TEXT NOT NULL DEFAULT '',

    refunded_minor BIGINT NOT NULL DEFAULT 0 CHECK (refunded_minor >= 0),
    settled_minor BIGINT NOT NULL DEFAULT 0 CHECK (settled_minor >= 0),

    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_payment_legs_operation_scope_fk
        FOREIGN KEY (payment_operation_id, parent_order_id)
        REFERENCES commerce_payment_operations(id, parent_order_id),

    CONSTRAINT commerce_payment_legs_vendor_scope_fk
        FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id),

    -- A vendor leg always names its vendor order and business; the platform
    -- leg never does. This is what makes "vendor A cannot receive vendor B's
    -- allocation" a shape rather than a convention.
    CONSTRAINT commerce_payment_legs_scope_chk CHECK (
        (scope = 'VENDOR' AND vendor_order_id IS NOT NULL AND business_id <> '')
        OR (scope = 'PLATFORM' AND vendor_order_id IS NULL AND business_id = '')
    ),

    -- Money can never exceed what this leg was authorized for.
    CONSTRAINT commerce_payment_legs_amounts_chk CHECK (
        refunded_minor <= amount_minor
        AND settled_minor <= amount_minor
        AND refunded_minor + settled_minor <= amount_minor
    ),

    -- A held leg must carry the authority's own reference. "HELD" is only
    -- sayable with real evidence from the money authority.
    CONSTRAINT commerce_payment_legs_held_evidence_chk CHECK (
        status = 'PENDING' OR status = 'FAILED'
        OR (length(btrim(intent_reference)) > 0 AND length(btrim(intent_id)) > 0)
    ),

    -- At most one leg per vendor order, and at most one platform leg.
    UNIQUE (payment_operation_id, vendor_order_id),
    UNIQUE (id, payment_operation_id)
);

CREATE UNIQUE INDEX commerce_payment_legs_one_platform_idx
    ON commerce_payment_legs(payment_operation_id)
    WHERE scope = 'PLATFORM';

CREATE INDEX commerce_payment_legs_operation_idx
    ON commerce_payment_legs(payment_operation_id, scope, id);
CREATE INDEX commerce_payment_legs_reference_idx
    ON commerce_payment_legs(intent_reference)
    WHERE intent_reference <> '';
CREATE INDEX commerce_payment_legs_recovery_idx
    ON commerce_payment_legs(status, updated_at, id)
    WHERE status = 'PENDING';


-- Append-only payment evidence. It is the audit trail for every interaction
-- with the money authority, including the ones that failed.
CREATE TABLE commerce_payment_events (
    id TEXT PRIMARY KEY,
    payment_operation_id TEXT NOT NULL
        REFERENCES commerce_payment_operations(id) ON DELETE RESTRICT,
    payment_leg_id TEXT,
    parent_order_id TEXT NOT NULL,
    customer_id TEXT NOT NULL,

    action TEXT NOT NULL CHECK (action IN (
        'commerce.payment.operation_started',
        'commerce.payment.leg_requested',
        'commerce.payment.leg_held',
        'commerce.payment.leg_failed',
        'commerce.payment.operation_held',
        'commerce.payment.operation_failed',
        'commerce.payment.compensation_started',
        'commerce.payment.compensation_completed',
        'commerce.payment.state_changed'
    )),

    actor_type TEXT NOT NULL CHECK (actor_type IN ('CUSTOMER','SYSTEM','AUTHORITY')),
    actor_ref TEXT NOT NULL DEFAULT '',
    from_state TEXT NOT NULL DEFAULT '',
    to_state TEXT NOT NULL DEFAULT '',
    -- The authority's own answer, redacted of anything that is not a status or
    -- a reference. No card, account or balance detail is ever stored here.
    authority_detail TEXT NOT NULL DEFAULT '',

    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT commerce_payment_events_leg_scope_fk
        FOREIGN KEY (payment_leg_id, payment_operation_id)
        REFERENCES commerce_payment_legs(id, payment_operation_id)
);

CREATE INDEX commerce_payment_events_operation_idx
    ON commerce_payment_events(payment_operation_id, occurred_at, id);


-- Payment evidence is append-only at the database, so no future writer can
-- rewrite what the money authority actually said. The operation and leg rows
-- are deliberately NOT frozen: their status is what an authority answer
-- changes.
CREATE OR REPLACE FUNCTION commerce_payment_reject_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce payment evidence is immutable';
END;
$$;

CREATE TRIGGER commerce_payment_events_immutable
    BEFORE UPDATE OR DELETE ON commerce_payment_events
    FOR EACH ROW EXECUTE FUNCTION commerce_payment_reject_mutation();

COMMENT ON TABLE commerce_payment_operations IS
'Section 17F payment binding. Binds one order to one payment operation against payment-wallet-service. It is not a ledger and holds no balance.';
COMMENT ON TABLE commerce_payment_legs IS
'One payment leg per VendorOrder plus one platform fee/tax leg. Split at creation because the Wallet contract settles and refunds an intent as a whole, to a single provider.';
COMMENT ON COLUMN commerce_payment_legs.authority_status IS
'The raw status payment-wallet-service reported. Vendor Commerce never invents a payment status and never treats an unrecognised answer as success.';
