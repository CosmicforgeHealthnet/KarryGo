-- Section 21 Prompt 1: scheduled/bulk commerce, collaborative groups and RFQ.
-- These tables hold immutable commercial/operational evidence only. Orders,
-- production capacity and checkout remain owned by their existing authorities.

CREATE TABLE commerce_scheduled_orders (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    customer_id text NOT NULL,
    quote_id text,
    vendor_order_id text,
    branch_id text NOT NULL,
    desired_delivery_at timestamptz NOT NULL,
    delivery_window_start timestamptz,
    delivery_window_end timestamptz,
    production_ready_by timestamptz,
    expected_ready_at timestamptz,
    capacity_reservation_id text,
    capacity_status text NOT NULL DEFAULT 'NOT_REQUIRED'
        CHECK (capacity_status IN ('NOT_REQUIRED','RESERVED','RELEASED','CONFLICT')),
    fulfilment_plan_id text,
    tentative_vehicle_class text
        CHECK (tentative_vehicle_class IS NULL OR tentative_vehicle_class IN ('MOTORCYCLE','VAN','TRUCK')),
    scheduling_status text NOT NULL DEFAULT 'SCHEDULED'
        CHECK (scheduling_status IN ('SCHEDULED','ACCEPTED','CANCELLED','READY')),
    bulk_requirement jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_ref text NOT NULL,
    source_version bigint NOT NULL DEFAULT 1 CHECK (source_version > 0),
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    cancelled_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id, idempotency_key),
    CHECK (scheduling_status <> 'ACCEPTED' OR capacity_status IN ('NOT_REQUIRED','RESERVED')),
    CHECK ((capacity_status = 'RESERVED') = (capacity_reservation_id IS NOT NULL))
);
CREATE INDEX commerce_scheduled_orders_due_idx ON commerce_scheduled_orders (business_id, desired_delivery_at);
CREATE INDEX commerce_scheduled_orders_reservation_idx ON commerce_scheduled_orders (capacity_reservation_id);

CREATE TABLE commerce_group_sessions (
    id text PRIMARY KEY,
    organizer_id text NOT NULL,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    status text NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN','CLOSED','CONVERTING','CONVERTED','CANCELLED','EXPIRED')),
    closing_at timestamptz NOT NULL,
    budget_minor bigint CHECK (budget_minor IS NULL OR budget_minor >= 0),
    currency text NOT NULL DEFAULT 'EUR',
    invite_token_hash text NOT NULL,
    invite_expires_at timestamptz NOT NULL,
    invite_revoked_at timestamptz,
    conversion_parent_order_id text UNIQUE,
    conversion_quote_id text,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    converted_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX commerce_group_sessions_organizer_idx ON commerce_group_sessions (organizer_id, created_at DESC);

CREATE TABLE commerce_group_participants (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES commerce_group_sessions(id) ON DELETE CASCADE,
    display_name text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 120),
    token_hash text NOT NULL,
    token_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, token_hash)
);
CREATE TABLE commerce_group_selections (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES commerce_group_sessions(id) ON DELETE CASCADE,
    participant_id text NOT NULL REFERENCES commerce_group_participants(id) ON DELETE CASCADE,
    item_ref text NOT NULL,
    variant_ref text,
    quantity bigint NOT NULL CHECK (quantity > 0 AND quantity <= 1000000),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    currency text NOT NULL,
    snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, participant_id)
);
CREATE INDEX commerce_group_selections_session_idx ON commerce_group_selections (session_id, participant_id);
CREATE TABLE commerce_group_attributions (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES commerce_group_sessions(id) ON DELETE CASCADE,
    parent_order_id text NOT NULL,
    participant_id text NOT NULL,
    selection_id text NOT NULL,
    snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (parent_order_id, selection_id)
);

CREATE TABLE commerce_rfqs (
    id text PRIMARY KEY,
    requester_id text NOT NULL,
    business_id text,
    eligibility_scope jsonb NOT NULL DEFAULT '{}'::jsonb,
    servings bigint NOT NULL CHECK (servings > 0),
    requested_at timestamptz NOT NULL,
    destination_snapshot jsonb NOT NULL,
    menu_requirements jsonb NOT NULL DEFAULT '{}'::jsonb,
    packaging_requirements jsonb NOT NULL DEFAULT '{}'::jsonb,
    budget_minor bigint CHECK (budget_minor IS NULL OR budget_minor >= 0),
    currency text NOT NULL DEFAULT 'EUR',
    delivery_terms jsonb NOT NULL DEFAULT '{}'::jsonb,
    capacity_requirement jsonb NOT NULL DEFAULT '{}'::jsonb,
    deadline_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','OPEN','QUOTED','ACCEPTED','EXPIRED','CANCELLED','CONVERTED')),
    accepted_quote_id text,
    checkout_quote_id text,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    opened_at timestamptz,
    accepted_at timestamptz,
    converted_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX commerce_rfqs_deadline_idx ON commerce_rfqs (status, deadline_at);
CREATE TABLE commerce_rfq_quotes (
    id text PRIMARY KEY,
    rfq_id text NOT NULL REFERENCES commerce_rfqs(id) ON DELETE CASCADE,
    vendor_id text NOT NULL,
    branch_id text NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','SUPERSEDED','CLOSED','ACCEPTED','EXPIRED','DECLINED')),
    current_revision bigint NOT NULL DEFAULT 1 CHECK (current_revision > 0),
    accepted_revision bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (rfq_id, vendor_id, branch_id)
);
CREATE TABLE commerce_rfq_quote_revisions (
    id text PRIMARY KEY,
    quote_id text NOT NULL REFERENCES commerce_rfq_quotes(id) ON DELETE CASCADE,
    rfq_id text NOT NULL REFERENCES commerce_rfqs(id) ON DELETE CASCADE,
    revision bigint NOT NULL CHECK (revision > 0),
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    currency text NOT NULL,
    snapshot jsonb NOT NULL,
    capacity_requirement jsonb NOT NULL DEFAULT '{}'::jsonb,
    expires_at timestamptz NOT NULL,
    idempotency_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (quote_id, revision),
    UNIQUE (quote_id, idempotency_key)
);
CREATE UNIQUE INDEX commerce_rfq_one_accepted_quote_idx ON commerce_rfqs (id) WHERE status = 'ACCEPTED';

CREATE TABLE commerce_rfq_checkout_bindings (
    id text PRIMARY KEY,
    rfq_id text NOT NULL UNIQUE REFERENCES commerce_rfqs(id) ON DELETE RESTRICT,
    checkout_quote_id text NOT NULL UNIQUE REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    accepted_quote_id text NOT NULL REFERENCES commerce_rfq_quotes(id) ON DELETE RESTRICT,
    accepted_revision bigint NOT NULL CHECK (accepted_revision > 0),
    requester_id text NOT NULL,
    vendor_id text NOT NULL,
    branch_id text NOT NULL,
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    currency text NOT NULL,
    capacity_reservation_id text,
    binding_digest text NOT NULL CHECK (binding_digest ~ '^[0-9a-f]{64}$'),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX commerce_rfq_checkout_bindings_quote_idx ON commerce_rfq_checkout_bindings (checkout_quote_id);
