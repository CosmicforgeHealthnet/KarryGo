-- Section 32 final closure: allow the existing opaque projection to reference
-- either a platform order mission or a canonical VendorMission, never both.
ALTER TABLE commerce_handover_observations
    DROP CONSTRAINT commerce_handover_observations_mission_id_authority_fkey,
    DROP CONSTRAINT commerce_handover_observations_authority_check,
    ALTER COLUMN mission_id DROP NOT NULL,
    ADD COLUMN vendor_mission_id text REFERENCES commerce_vendor_missions(id) ON DELETE RESTRICT,
    ADD CONSTRAINT section32_handover_authority_check
        CHECK (authority IN ('DISPATCH','HAULING','TAXI','VENDOR_OWN_DELIVERY')),
    ADD CONSTRAINT section32_handover_mission_kind_check CHECK (
        (authority = 'VENDOR_OWN_DELIVERY' AND mission_id IS NULL AND vendor_mission_id IS NOT NULL) OR
        (authority <> 'VENDOR_OWN_DELIVERY' AND mission_id IS NOT NULL AND vendor_mission_id IS NULL)
    ),
    ADD CONSTRAINT section32_platform_mission_authority_fk
        FOREIGN KEY (mission_id, authority) REFERENCES commerce_order_missions(id, service) ON DELETE RESTRICT;

CREATE INDEX section32_handover_vendor_mission_idx
    ON commerce_handover_observations(vendor_mission_id, stage, source_version)
    WHERE vendor_mission_id IS NOT NULL;

CREATE OR REPLACE FUNCTION section32_guard_observation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.mission_id IS DISTINCT FROM OLD.mission_id OR
       NEW.vendor_mission_id IS DISTINCT FROM OLD.vendor_mission_id OR
       NEW.authority <> OLD.authority OR
       NEW.authority_mission_ref <> OLD.authority_mission_ref OR NEW.handover_ref <> OLD.handover_ref OR
       NEW.stage <> OLD.stage OR NEW.source_version < OLD.source_version OR
       (NEW.source_version = OLD.source_version AND NEW.status <> OLD.status) OR
       (OLD.status = 'CONSUMED' AND NEW.status <> 'CONSUMED') THEN
        RAISE EXCEPTION 'handover observation cannot change scope or regress';
    END IF;
    RETURN NEW;
END $$;
