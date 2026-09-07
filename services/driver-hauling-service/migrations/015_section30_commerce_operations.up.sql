-- Section 30 additive Commerce operational metadata.
--
-- Loading duration is caller-supplied scheduling evidence. It is bounded by
-- both the shared contract and this database constraint, is included in the
-- mission fingerprint, and never controls price or provider selection.

ALTER TABLE commerce_missions
  ADD COLUMN IF NOT EXISTS loading_duration_seconds BIGINT NOT NULL DEFAULT 0
    CHECK (loading_duration_seconds BETWEEN 0 AND 28800);

COMMENT ON COLUMN commerce_missions.loading_duration_seconds IS
'Expected cargo loading duration used for scheduling/provider operations; zero means the legacy/default Hauling policy.';
