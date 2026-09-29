-- Post-pick physical re-evaluation evidence (Karryam V4 §14.6).
--
-- One row per (mission, evidence fingerprint): the same packed load evaluated
-- twice is the same decision. Rows are append-only. An ESCALATED row is also the
-- authority for which logistics authority/class/quote now executes the plan
-- mission slot, superseding the checkout-time frozen quote for EXECUTION only;
-- the customer's approved delivery price is never changed by it.
CREATE TABLE commerce_mission_physical_reevaluations (
    id text PRIMARY KEY,
    mission_id text NOT NULL,
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    plan_mission_ref text NOT NULL,
    vendor_order_id text NOT NULL,
    trigger text NOT NULL CHECK (trigger IN ('PACKING_COMPLETED')),
    evidence_fingerprint text NOT NULL CHECK (length(evidence_fingerprint) = 64),
    outcome text NOT NULL CHECK (outcome IN ('UNCHANGED', 'ESCALATED', 'REFUSED')),
    reason_code text NOT NULL DEFAULT '',
    policy_version text NOT NULL,
    previous_service text NOT NULL,
    previous_vehicle_class text NOT NULL,
    previous_mission_ref text NOT NULL,
    previous_fee_minor bigint NOT NULL CHECK (previous_fee_minor >= 0),
    selected_service text NOT NULL DEFAULT '',
    selected_vehicle_class text NOT NULL DEFAULT '',
    mission_ref text NOT NULL DEFAULT '',
    authority_fee_minor bigint NOT NULL DEFAULT 0 CHECK (authority_fee_minor >= 0),
    authority_quote_id text NOT NULL DEFAULT '',
    authority_quote_reference text NOT NULL DEFAULT '',
    customer_fee_minor bigint NOT NULL CHECK (customer_fee_minor >= 0),
    platform_funded_delta_minor bigint NOT NULL DEFAULT 0 CHECK (platform_funded_delta_minor >= 0),
    confirmed_weight_grams bigint NOT NULL DEFAULT 0 CHECK (confirmed_weight_grams >= 0),
    confirmed_package_count bigint NOT NULL DEFAULT 0 CHECK (confirmed_package_count >= 0),
    decision jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (mission_id, parent_order_id) REFERENCES commerce_order_missions(id, parent_order_id),
    UNIQUE (mission_id, evidence_fingerprint),
    CONSTRAINT commerce_mission_physical_escalation_chk CHECK (
        outcome <> 'ESCALATED' OR (
            selected_service IN ('DISPATCH', 'HAULING', 'TAXI')
            AND length(btrim(selected_vehicle_class)) > 0
            AND length(btrim(mission_ref)) > 0
            AND mission_ref <> previous_mission_ref
            AND length(btrim(authority_quote_id)) > 0
        )),
    CONSTRAINT commerce_mission_physical_refusal_chk CHECK (
        outcome <> 'REFUSED' OR length(btrim(reason_code)) > 0)
);

CREATE INDEX commerce_mission_physical_escalation_idx
    ON commerce_mission_physical_reevaluations(parent_order_id, plan_mission_ref, created_at DESC)
    WHERE outcome = 'ESCALATED';

CREATE FUNCTION commerce_mission_physical_reevaluations_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce_mission_physical_reevaluations is append-only';
END;
$$;

CREATE TRIGGER commerce_mission_physical_reevaluations_immutable
    BEFORE UPDATE OR DELETE ON commerce_mission_physical_reevaluations
    FOR EACH ROW EXECUTE FUNCTION commerce_mission_physical_reevaluations_immutable();
