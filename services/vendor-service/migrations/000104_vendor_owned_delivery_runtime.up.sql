-- Vendor-owned Rider execution and recoverable pre-custody Hybrid fallback.

ALTER TABLE commerce_vendor_orders
    ADD COLUMN effective_delivery_responsibility text,
    ADD COLUMN effective_delivery_fee_beneficiary text,
    ADD COLUMN effective_logistics_mission_ref text;

UPDATE commerce_vendor_orders SET
    effective_delivery_responsibility=delivery_responsibility,
    effective_delivery_fee_beneficiary=delivery_fee_beneficiary,
    effective_logistics_mission_ref=logistics_mission_ref;

ALTER TABLE commerce_vendor_orders
    ALTER COLUMN effective_delivery_responsibility SET NOT NULL,
    ALTER COLUMN effective_delivery_fee_beneficiary SET NOT NULL,
    ALTER COLUMN effective_logistics_mission_ref SET NOT NULL,
    ADD CONSTRAINT commerce_vendor_order_effective_delivery_check CHECK (
        (effective_delivery_responsibility='VENDOR_OWN_DELIVERY' AND effective_delivery_fee_beneficiary='VENDOR') OR
        (effective_delivery_responsibility='PLATFORM_LOGISTICS' AND effective_delivery_fee_beneficiary='PLATFORM_PROVIDER') OR
        (effective_delivery_responsibility='NONE' AND effective_delivery_fee_beneficiary='NONE')
    );

CREATE OR REPLACE FUNCTION initialize_commerce_vendor_order_effective_delivery()
RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.effective_delivery_responsibility IS NULL THEN NEW.effective_delivery_responsibility:=NEW.delivery_responsibility; END IF;
    IF NEW.effective_delivery_fee_beneficiary IS NULL THEN NEW.effective_delivery_fee_beneficiary:=NEW.delivery_fee_beneficiary; END IF;
    IF NEW.effective_logistics_mission_ref IS NULL THEN NEW.effective_logistics_mission_ref:=NEW.logistics_mission_ref; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER commerce_vendor_order_effective_delivery_init
BEFORE INSERT ON commerce_vendor_orders FOR EACH ROW
EXECUTE FUNCTION initialize_commerce_vendor_order_effective_delivery();

CREATE TABLE commerce_vendor_rider_availability (
    staff_id text PRIMARY KEY REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    state text NOT NULL CHECK (state IN ('OFFLINE','AVAILABLE','BUSY','SUSPENDED')),
    current_vendor_mission_id text REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vendor_rider_availability_staff_scope_fk
        FOREIGN KEY (staff_id,business_id) REFERENCES vendor_staff(id,vendor_id) ON DELETE RESTRICT,
    CONSTRAINT vendor_rider_availability_branch_scope_fk
        FOREIGN KEY (branch_id,business_id) REFERENCES vendor_branches(id,business_id) ON DELETE RESTRICT,
    CHECK ((state='BUSY') = (current_vendor_mission_id IS NOT NULL))
);

CREATE UNIQUE INDEX commerce_vendor_rider_one_current_mission_idx
    ON commerce_vendor_rider_availability(current_vendor_mission_id)
    WHERE current_vendor_mission_id IS NOT NULL;
CREATE INDEX commerce_vendor_rider_available_idx
    ON commerce_vendor_rider_availability(business_id,branch_id,state,updated_at);

CREATE TABLE commerce_vendor_rider_availability_operations (
    id text PRIMARY KEY,
    staff_id text NOT NULL REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    operation_ref text NOT NULL,
    request_fingerprint text NOT NULL,
    result_version bigint NOT NULL CHECK (result_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id,operation_ref)
);

CREATE TABLE commerce_vendor_mission_assignments (
    id text PRIMARY KEY,
    vendor_mission_id text NOT NULL REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    staff_id text NOT NULL REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    assigned_by_staff_id text NOT NULL REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    operation_ref text NOT NULL,
    request_fingerprint text NOT NULL,
    state text NOT NULL CHECK (state IN ('ACTIVE','REPLACED','RELEASED')),
    assigned_at timestamptz NOT NULL,
    ended_at timestamptz,
    end_reason text NOT NULL DEFAULT '',
    UNIQUE (business_id,operation_ref)
);

CREATE UNIQUE INDEX commerce_vendor_mission_one_active_assignment_idx
    ON commerce_vendor_mission_assignments(vendor_mission_id) WHERE state='ACTIVE';
CREATE UNIQUE INDEX commerce_vendor_rider_one_active_assignment_idx
    ON commerce_vendor_mission_assignments(staff_id) WHERE state='ACTIVE';

CREATE TABLE commerce_vendor_mission_runtime (
    vendor_mission_id text PRIMARY KEY REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    stage text NOT NULL CHECK (stage IN (
        'PLANNED','ASSIGNED','ACKNOWLEDGED','EN_ROUTE_PICKUP','ARRIVED_PICKUP',
        'IN_CUSTODY','EN_ROUTE_RECEIVER','ARRIVED_RECEIVER','COMPLETED',
        'RECEIVER_UNAVAILABLE','RETURNING','RETURNED','FAILED','FALLBACK_STARTED','FALLBACK_COMPLETED'
    )),
    pickup_arrived_at timestamptz,
    custody_verified_at timestamptz,
    receiver_arrived_at timestamptz,
    receiver_unavailable_reason text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO commerce_vendor_mission_runtime(vendor_mission_id,stage,version,updated_at)
SELECT id, CASE status
    WHEN 'PLANNED' THEN 'PLANNED' WHEN 'ASSIGNED' THEN 'ASSIGNED'
    WHEN 'READY_FOR_PICKUP' THEN 'ARRIVED_PICKUP'
    WHEN 'CUSTODY_TRANSFERRED' THEN 'IN_CUSTODY'
    WHEN 'OUT_FOR_DELIVERY' THEN 'EN_ROUTE_RECEIVER'
    WHEN 'DELIVERED' THEN 'COMPLETED'
    WHEN 'RETURN_REQUIRED' THEN 'RETURNING' WHEN 'RETURNED' THEN 'RETURNED'
    ELSE 'FAILED' END, version, updated_at
FROM commerce_vendor_missions
ON CONFLICT DO NOTHING;

CREATE TABLE commerce_vendor_mission_operations (
    id text PRIMARY KEY,
    vendor_mission_id text NOT NULL REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    staff_id text NOT NULL REFERENCES vendor_staff(id) ON DELETE RESTRICT,
    operation_ref text NOT NULL,
    action text NOT NULL,
    request_fingerprint text NOT NULL,
    result_stage text NOT NULL,
    result_mission_version bigint NOT NULL CHECK (result_mission_version > 0),
    evidence_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (business_id,operation_ref)
);

CREATE TABLE commerce_vendor_hybrid_fallbacks (
    id text PRIMARY KEY,
    operation_ref text NOT NULL UNIQUE,
    request_fingerprint text NOT NULL,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    vendor_mission_id text NOT NULL UNIQUE REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    group_id text NOT NULL REFERENCES commerce_fulfilment_groups(id) ON DELETE RESTRICT,
    source_plan_id text NOT NULL REFERENCES commerce_fulfilment_plan_versions(id) ON DELETE RESTRICT,
    source_plan_version bigint NOT NULL CHECK (source_plan_version > 0),
    source_plan_mission_id text NOT NULL REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,
    target_plan_id text REFERENCES commerce_fulfilment_plan_versions(id) ON DELETE RESTRICT,
    target_plan_version bigint CHECK (target_plan_version IS NULL OR target_plan_version > source_plan_version),
    target_plan_mission_id text REFERENCES commerce_fulfilment_plan_missions(id) ON DELETE RESTRICT,
    platform_mission_ref_id text REFERENCES commerce_platform_mission_refs(id) ON DELETE RESTRICT,
    wallet_transition_ref text NOT NULL DEFAULT '',
    reason_code text NOT NULL,
    state text NOT NULL CHECK (state IN ('STARTED','PLAN_ACTIVATED','FUNDING_TRANSITIONED','MISSION_LINKED','COMPLETED','FAILED')),
    failure_reason text NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX commerce_vendor_hybrid_fallback_state_idx
    ON commerce_vendor_hybrid_fallbacks(state,updated_at);

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_vendor_rider',p.id FROM vendor_permissions p
WHERE p.code='vendor.delivery.execute_assigned'
ON CONFLICT DO NOTHING;

COMMENT ON TABLE commerce_vendor_rider_availability IS
'Vendor staff Rider operational availability. This is not a marketplace provider registry.';
COMMENT ON TABLE commerce_vendor_hybrid_fallbacks IS
'Durable VendorMission to immutable platform replan/Wallet/PlatformMissionRef saga linkage.';
