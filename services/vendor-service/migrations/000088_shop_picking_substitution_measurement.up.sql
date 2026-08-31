-- Shop picking, substitution cases, structured quality and variable
-- measurement evidence.  This extends the existing Orders/Shop/Inventory
-- authorities; it does not create a second order or payment system.

ALTER TABLE commerce_vendor_order_line_outcomes
    ADD COLUMN IF NOT EXISTS measurement_mode TEXT NOT NULL DEFAULT 'FIXED_UNIT'
        CHECK (measurement_mode IN ('FIXED_UNIT','VARIABLE_WEIGHT','VARIABLE_COUNT')),
    ADD COLUMN IF NOT EXISTS quality_result TEXT NOT NULL DEFAULT 'MET'
        CHECK (quality_result IN ('MET','PARTIAL','NOT_AVAILABLE','SUBSTITUTE')),
    ADD COLUMN IF NOT EXISTS picking_state TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (picking_state IN ('PENDING','IN_PROGRESS','COMPLETE','BLOCKED')),
    ADD COLUMN IF NOT EXISTS substitution_policy TEXT NOT NULL DEFAULT 'EXACT_ONLY'
        CHECK (substitution_policy IN ('EXACT_ONLY','SPECIFIC_REPLACEMENT','SAME_BRAND','SIMILAR_WITHIN_PRICE_TOLERANCE','VENDOR_SUGGEST_AND_APPROVE','AUTO_REFUND','CONTACT_FIRST')),
    ADD COLUMN IF NOT EXISTS substitution_policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(substitution_policy_snapshot) = 'object'),
    ADD COLUMN IF NOT EXISTS quality_preference_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(quality_preference_snapshot) = 'array'),
    ADD COLUMN IF NOT EXISTS expected_vendor_order_version BIGINT NOT NULL DEFAULT 1 CHECK (expected_vendor_order_version > 0),
    ADD COLUMN IF NOT EXISTS correlation_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_ref TEXT NOT NULL DEFAULT '';

CREATE TABLE commerce_shop_picking_snapshots (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL UNIQUE REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    ordered_quantity BIGINT NOT NULL CHECK (ordered_quantity > 0),
    variant_ref TEXT NOT NULL DEFAULT '',
    sell_unit_ref TEXT NOT NULL DEFAULT '',
    measurement_mode TEXT NOT NULL CHECK (measurement_mode IN ('FIXED_UNIT','VARIABLE_WEIGHT','VARIABLE_COUNT')),
    measurement_basis TEXT NOT NULL DEFAULT '',
    measurement_min_quantity BIGINT NOT NULL DEFAULT 0 CHECK (measurement_min_quantity >= 0),
    measurement_max_quantity BIGINT NOT NULL DEFAULT 0 CHECK (measurement_max_quantity >= measurement_min_quantity),
    measurement_tolerance_quantity BIGINT NOT NULL DEFAULT 0 CHECK (measurement_tolerance_quantity >= 0),
    variable_count_allowed BOOLEAN NOT NULL DEFAULT false,
    estimated_variable_quantity BIGINT NOT NULL DEFAULT 0 CHECK (estimated_variable_quantity >= 0),
    substitution_policy TEXT NOT NULL CHECK (substitution_policy IN ('EXACT_ONLY','SPECIFIC_REPLACEMENT','SAME_BRAND','SIMILAR_WITHIN_PRICE_TOLERANCE','VENDOR_SUGGEST_AND_APPROVE','AUTO_REFUND','CONTACT_FIRST')),
    substitution_policy_snapshot JSONB NOT NULL CHECK (jsonb_typeof(substitution_policy_snapshot) = 'object'),
    quality_preference_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(quality_preference_snapshot) = 'array'),
    preselected_replacement_ref TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_shop_picking_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id)
);
CREATE INDEX commerce_shop_picking_vendor_idx ON commerce_shop_picking_snapshots(vendor_order_id, branch_id, updated_at, id);

CREATE TABLE commerce_substitution_cases (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    unavailable_quantity BIGINT NOT NULL CHECK (unavailable_quantity > 0),
    original_line_snapshot JSONB NOT NULL CHECK (jsonb_typeof(original_line_snapshot) = 'object'),
    policy_snapshot JSONB NOT NULL CHECK (jsonb_typeof(policy_snapshot) = 'object'),
    quality_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(quality_snapshot) = 'array'),
    deadline_at TIMESTAMPTZ,
    fallback_policy TEXT NOT NULL CHECK (fallback_policy IN ('EXACT_ONLY','AUTO_REFUND','CONTACT_FIRST','SPECIFIC_REPLACEMENT','VENDOR_SUGGEST_AND_APPROVE')),
    state TEXT NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN','AWAITING_CUSTOMER','APPROVED','REJECTED','AUTO_REFUND_PENDING','RESOLVED','EXPIRED','CANCELLED')),
    reason TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    correlation_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT commerce_substitution_case_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id),
    CONSTRAINT commerce_substitution_case_customer_fk FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id),
    UNIQUE (id, parent_order_id)
);
CREATE UNIQUE INDEX commerce_substitution_one_active_case_idx
    ON commerce_substitution_cases(order_line_id) WHERE state IN ('OPEN','AWAITING_CUSTOMER','AUTO_REFUND_PENDING');
CREATE INDEX commerce_substitution_case_recovery_idx ON commerce_substitution_cases(state, deadline_at, updated_at, id);

CREATE TABLE commerce_substitution_case_proposals (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES commerce_substitution_cases(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    candidate_item_ref TEXT NOT NULL,
    candidate_variant_ref TEXT NOT NULL DEFAULT '',
    candidate_listing_ref TEXT NOT NULL DEFAULT '',
    candidate_snapshot JSONB NOT NULL CHECK (jsonb_typeof(candidate_snapshot) = 'object'),
    proposed_quantity BIGINT NOT NULL CHECK (proposed_quantity > 0),
    proposed_price_minor BIGINT NOT NULL CHECK (proposed_price_minor >= 0),
    price_difference_minor BIGINT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','ACCEPTED','REJECTED','EXPIRED','CANCELLED')),
    expires_at TIMESTAMPTZ,
    response_ref TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_substitution_proposal_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id),
    CONSTRAINT commerce_substitution_proposal_case_scope_fk FOREIGN KEY (case_id, parent_order_id)
        REFERENCES commerce_substitution_cases(id, parent_order_id)
);
CREATE INDEX commerce_substitution_case_customer_idx ON commerce_substitution_case_proposals(customer_id, state, created_at DESC, id);
CREATE INDEX commerce_substitution_case_expiry_idx ON commerce_substitution_case_proposals(state, expires_at, id) WHERE state = 'PENDING';

CREATE TABLE commerce_order_line_quality_preferences (
    id TEXT PRIMARY KEY,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    preference_type TEXT NOT NULL CHECK (preference_type IN ('RIPENESS','FIRMNESS','CUT_SIZE','PREPARATION','CLEANING','SORTING')),
    preference_value TEXT NOT NULL CHECK (length(btrim(preference_value)) > 0),
    source TEXT NOT NULL CHECK (source IN ('CUSTOMER_SAVED','CHECKOUT_EXPLICIT','PLATFORM_DEFAULT')),
    source_version BIGINT NOT NULL DEFAULT 0 CHECK (source_version >= 0),
    taxonomy_version BIGINT NOT NULL DEFAULT 1 CHECK (taxonomy_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_line_id, preference_type)
);

CREATE TABLE commerce_order_line_quality_results (
    id TEXT PRIMARY KEY,
    preference_id TEXT NOT NULL REFERENCES commerce_order_line_quality_preferences(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    result TEXT NOT NULL CHECK (result IN ('MET','PARTIAL','NOT_AVAILABLE','SUBSTITUTE')),
    observed_value TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    picker_ref TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (preference_id)
);

CREATE TABLE commerce_variable_measurements (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL UNIQUE REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    measurement_mode TEXT NOT NULL CHECK (measurement_mode IN ('VARIABLE_WEIGHT','VARIABLE_COUNT')),
    estimated_quantity BIGINT NOT NULL CHECK (estimated_quantity > 0),
    unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
    estimated_amount_minor BIGINT NOT NULL CHECK (estimated_amount_minor >= 0),
    tolerance_quantity BIGINT NOT NULL DEFAULT 0 CHECK (tolerance_quantity >= 0),
    hard_max_quantity BIGINT NOT NULL CHECK (hard_max_quantity >= estimated_quantity),
    reserved_upper_quantity BIGINT NOT NULL CHECK (reserved_upper_quantity >= estimated_quantity AND reserved_upper_quantity <= hard_max_quantity),
    actual_quantity BIGINT NOT NULL DEFAULT 0 CHECK (actual_quantity >= 0 AND actual_quantity <= reserved_upper_quantity),
    final_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (final_amount_minor >= 0),
    financial_state TEXT NOT NULL DEFAULT 'ESTIMATED' CHECK (financial_state IN ('NOT_REQUIRED','ESTIMATED','ACTUAL_RECORDED','FINANCIAL_PENDING','REAPPROVAL_REQUIRED','FINALIZED','REFUND_PENDING','FAILED_RECOVERABLE')),
    wallet_capability_snapshot TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_variable_measurement_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id)
);
CREATE INDEX commerce_variable_measurement_recovery_idx ON commerce_variable_measurements(financial_state, updated_at, id)
    WHERE financial_state IN ('FINANCIAL_PENDING','REAPPROVAL_REQUIRED','REFUND_PENDING','FAILED_RECOVERABLE');

CREATE TABLE commerce_payment_capability_snapshots (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    payment_operation_ref TEXT NOT NULL,
    capability TEXT NOT NULL CHECK (capability IN ('ADJUSTABLE_AUTH_CAPTURE','CAP_AND_REFUND','UNSUPPORTED')),
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (parent_order_id, payment_operation_ref)
);

CREATE TABLE commerce_order_adjustment_evidence (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    adjustment_id TEXT REFERENCES commerce_order_adjustments(id) ON DELETE RESTRICT,
    case_id TEXT REFERENCES commerce_substitution_cases(id) ON DELETE RESTRICT,
    proposal_id TEXT REFERENCES commerce_substitution_case_proposals(id) ON DELETE RESTRICT,
    adjustment_type TEXT NOT NULL,
    original_amount_minor BIGINT NOT NULL CHECK (original_amount_minor >= 0),
    replacement_amount_minor BIGINT NOT NULL CHECK (replacement_amount_minor >= 0),
    difference_minor BIGINT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity >= 0),
    funding_effect JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(funding_effect) = 'object'),
    inventory_effect JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(inventory_effect) = 'object'),
    customer_decision TEXT NOT NULL DEFAULT '',
    commercial_reference TEXT NOT NULL DEFAULT '',
    correlation_ref TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX commerce_adjustment_evidence_parent_idx ON commerce_order_adjustment_evidence(parent_order_id, created_at, id);

ALTER TABLE commerce_order_events DROP CONSTRAINT IF EXISTS commerce_order_events_action_check;
ALTER TABLE commerce_order_events ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
    'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created','commerce.vendor_order.paid','commerce.vendor_order.sent_to_vendor','commerce.vendor_order.accepted','commerce.vendor_order.rejected','commerce.vendor_order.preparing','commerce.vendor_order.picking','commerce.vendor_order.substitution_required','commerce.vendor_order.substitution_proposed','commerce.vendor_order.substitution_approved','commerce.vendor_order.substitution_rejected','commerce.vendor_order.packed','commerce.vendor_order.ready','commerce.vendor_order.ready_for_pickup','commerce.vendor_order.handed_over','commerce.vendor_order.picked_up','commerce.vendor_order.delivering','commerce.vendor_order.delivered','commerce.vendor_order.completed','commerce.vendor_order.cancelled','commerce.vendor_order.refund_pending','commerce.vendor_order.refund_unknown','commerce.vendor_order.refund_failed','commerce.vendor_order.refunded','commerce.vendor_order.partially_refunded','commerce.vendor_order.logistics_picked_up','commerce.vendor_order.logistics_delivering','commerce.vendor_order.logistics_delivered',
    'commerce.vendor_order.picking_started','commerce.vendor_order.availability_recorded','commerce.vendor_order.quality_recorded','commerce.vendor_order.substitution_case_opened','commerce.vendor_order.substitution_case_resolved','commerce.vendor_order.variable_measurement_recorded','commerce.vendor_order.variable_measurement_finalized','commerce.vendor_order.packing_blocked','commerce.vendor_order.packing_completed'));

COMMENT ON TABLE commerce_substitution_cases IS 'Durable unresolved substitution case; proposals are replaceable child evidence.';
COMMENT ON TABLE commerce_variable_measurements IS 'Frozen estimate/cap and authoritative actual measurement for one Shop line.';
COMMENT ON TABLE commerce_order_adjustment_evidence IS 'Immutable side-by-side commercial and fulfilment adjustment evidence.';
