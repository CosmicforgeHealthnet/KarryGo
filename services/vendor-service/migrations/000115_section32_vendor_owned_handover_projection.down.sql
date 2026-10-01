DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM commerce_handover_observations WHERE vendor_mission_id IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot roll back Vendor-owned handover projection while observations exist';
    END IF;
END $$;

DROP INDEX IF EXISTS section32_handover_vendor_mission_idx;
ALTER TABLE commerce_handover_observations
    DROP CONSTRAINT section32_platform_mission_authority_fk,
    DROP CONSTRAINT section32_handover_mission_kind_check,
    DROP CONSTRAINT section32_handover_authority_check,
    DROP COLUMN vendor_mission_id,
    ALTER COLUMN mission_id SET NOT NULL,
    ADD CONSTRAINT commerce_handover_observations_authority_check
        CHECK (authority IN ('DISPATCH','HAULING','TAXI')),
    ADD CONSTRAINT commerce_handover_observations_mission_id_authority_fkey
        FOREIGN KEY (mission_id, authority) REFERENCES commerce_order_missions(id, service) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION section32_guard_observation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.mission_id <> OLD.mission_id OR NEW.authority <> OLD.authority OR
       NEW.authority_mission_ref <> OLD.authority_mission_ref OR NEW.handover_ref <> OLD.handover_ref OR
       NEW.stage <> OLD.stage OR NEW.source_version < OLD.source_version OR
       (NEW.source_version = OLD.source_version AND NEW.status <> OLD.status) OR
       (OLD.status = 'CONSUMED' AND NEW.status <> 'CONSUMED') THEN
        RAISE EXCEPTION 'handover observation cannot change scope or regress';
    END IF;
    RETURN NEW;
END $$;
