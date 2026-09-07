-- Opaque observations only. Challenge authority remains in logistics.
ALTER TABLE commerce_order_missions ADD CONSTRAINT section32_mission_authority_unique UNIQUE(id, service);
CREATE TABLE commerce_handover_observations (
    mission_id TEXT NOT NULL,
    authority TEXT NOT NULL CHECK (authority IN ('DISPATCH','HAULING','TAXI')),
    authority_mission_ref TEXT NOT NULL CHECK (length(btrim(authority_mission_ref)) > 0),
    handover_ref TEXT NOT NULL CHECK (length(btrim(handover_ref)) > 0),
    stage TEXT NOT NULL CHECK (stage IN ('PICKUP','DELIVERY')),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE','CONSUMED','INVALIDATED','EXPIRED','LOCKED')),
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, handover_ref),
    FOREIGN KEY (mission_id, authority) REFERENCES commerce_order_missions(id, service) ON DELETE RESTRICT
);
CREATE INDEX section32_handover_mission_idx ON commerce_handover_observations(mission_id, stage, source_version);

CREATE FUNCTION section32_guard_observation() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER section32_observation_guard BEFORE UPDATE ON commerce_handover_observations
FOR EACH ROW EXECUTE FUNCTION section32_guard_observation();
