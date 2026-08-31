ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_event_type_check;
ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_event_type_check CHECK (event_type IN (
        'PICKED_UP','DELIVERING','DELIVERED','DELIVERY_FAILED',
        'TIMING_UPDATED','PICKUP_TIMING_UPDATED','ETA_UPDATED','TIMING_APPLIED',
        'TIMING_NO_CHANGE','TIMING_TOO_LATE','TIMING_TERMINAL','TIMING_UNKNOWN'
    ));

ALTER TABLE commerce_vendor_orders
    ADD COLUMN refund_policy_version text NOT NULL DEFAULT 'commerce-refund-policy.v1';

CREATE TABLE commerce_refund_policy_decisions (
    id text PRIMARY KEY,
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id text NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id text NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    mission_ref text NOT NULL DEFAULT '',
    trigger text NOT NULL,
    action text NOT NULL,
    reason_code text NOT NULL,
    policy_version text NOT NULL,
    plan_version bigint NOT NULL DEFAULT 0 CHECK (plan_version >= 0),
    automatic boolean NOT NULL,
    support_required boolean NOT NULL,
    customer_refund_minor bigint NOT NULL CHECK (customer_refund_minor >= 0),
    platform_funding_reversal_minor bigint NOT NULL CHECK (platform_funding_reversal_minor >= 0),
    other_funding_reversal_minor bigint NOT NULL CHECK (other_funding_reversal_minor >= 0),
    platform_loss_funding_minor bigint NOT NULL CHECK (platform_loss_funding_minor >= 0),
    vendor_merchandise_reduction_minor bigint NOT NULL CHECK (vendor_merchandise_reduction_minor >= 0),
    vendor_delivery_reduction_minor bigint NOT NULL CHECK (vendor_delivery_reduction_minor >= 0),
    provider_delivery_reduction_minor bigint NOT NULL CHECK (provider_delivery_reduction_minor >= 0),
    platform_revenue_reduction_minor bigint NOT NULL CHECK (platform_revenue_reduction_minor >= 0),
    authorization_release_minor bigint NOT NULL CHECK (authorization_release_minor >= 0),
    idempotency_key text NOT NULL UNIQUE,
    wallet_operation_ref text NOT NULL DEFAULT '',
    wallet_status text NOT NULL DEFAULT 'PENDING'
        CHECK (wallet_status IN ('PENDING','REQUESTED','APPLIED','PENDING_LIABILITY','UNKNOWN','FAILED','SUPPORT_REQUIRED')),
    projection_applied boolean NOT NULL DEFAULT false,
    support_case_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((support_required AND NOT automatic AND action = 'SUPPORT_CASE_REQUIRED') OR NOT support_required),
    CHECK (customer_refund_minor + platform_funding_reversal_minor + other_funding_reversal_minor >= platform_loss_funding_minor),
    CHECK (vendor_merchandise_reduction_minor + vendor_delivery_reduction_minor +
           provider_delivery_reduction_minor + platform_revenue_reduction_minor + authorization_release_minor =
           customer_refund_minor + platform_funding_reversal_minor + other_funding_reversal_minor - platform_loss_funding_minor)
);

CREATE INDEX commerce_refund_policy_decisions_recovery_idx
    ON commerce_refund_policy_decisions(wallet_status, updated_at, id)
    WHERE wallet_status IN ('PENDING','REQUESTED','UNKNOWN','PENDING_LIABILITY');

CREATE INDEX commerce_refund_policy_decisions_vendor_idx
    ON commerce_refund_policy_decisions(vendor_order_id, created_at, id);

CREATE OR REPLACE FUNCTION prevent_commerce_refund_policy_evidence_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.parent_order_id, NEW.vendor_order_id, NEW.order_line_id, NEW.mission_ref,
           NEW.trigger, NEW.action, NEW.reason_code, NEW.policy_version, NEW.plan_version,
           NEW.automatic, NEW.support_required, NEW.customer_refund_minor,
           NEW.platform_funding_reversal_minor, NEW.other_funding_reversal_minor,
           NEW.platform_loss_funding_minor, NEW.vendor_merchandise_reduction_minor,
           NEW.vendor_delivery_reduction_minor, NEW.provider_delivery_reduction_minor,
           NEW.platform_revenue_reduction_minor, NEW.authorization_release_minor,
           NEW.idempotency_key)
       IS DISTINCT FROM
       ROW(OLD.parent_order_id, OLD.vendor_order_id, OLD.order_line_id, OLD.mission_ref,
           OLD.trigger, OLD.action, OLD.reason_code, OLD.policy_version, OLD.plan_version,
           OLD.automatic, OLD.support_required, OLD.customer_refund_minor,
           OLD.platform_funding_reversal_minor, OLD.other_funding_reversal_minor,
           OLD.platform_loss_funding_minor, OLD.vendor_merchandise_reduction_minor,
           OLD.vendor_delivery_reduction_minor, OLD.provider_delivery_reduction_minor,
           OLD.platform_revenue_reduction_minor, OLD.authorization_release_minor,
           OLD.idempotency_key) THEN
        RAISE EXCEPTION 'commerce refund policy evidence is immutable';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_refund_policy_evidence_immutable
BEFORE UPDATE ON commerce_refund_policy_decisions
FOR EACH ROW EXECUTE FUNCTION prevent_commerce_refund_policy_evidence_mutation();
