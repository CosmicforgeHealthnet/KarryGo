-- A canonical observation revision also advances for expiry/invalidation,
-- which need not change the mission's business version.
CREATE SEQUENCE taxi_goods_handover_observation_seq;
ALTER TABLE taxi_goods_handover_challenges ADD COLUMN observation_version BIGINT NOT NULL
    DEFAULT nextval('taxi_goods_handover_observation_seq');
ALTER SEQUENCE taxi_goods_handover_observation_seq OWNED BY taxi_goods_handover_challenges.observation_version;
CREATE FUNCTION taxi_goods_bump_handover_observation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.observation_version := nextval('taxi_goods_handover_observation_seq');
    RETURN NEW;
END $$;
CREATE TRIGGER taxi_goods_handover_observation_revision BEFORE UPDATE ON taxi_goods_handover_challenges
FOR EACH ROW EXECUTE FUNCTION taxi_goods_bump_handover_observation();
