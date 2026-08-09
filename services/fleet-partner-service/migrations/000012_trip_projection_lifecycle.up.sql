-- WP5: trip projection lifecycle and reconciliation leases.
--
-- Three gaps closed here:
--   1. Trip projections had no way to distinguish a redelivered event from a
--      DIFFERENT event claiming the same source version. Storing the applied
--      payload hash makes that a detectable conflict rather than a silent
--      overwrite.
--   2. Reconciliation cases had no lease, so two workers could process the
--      same case concurrently and an abandoned case had no recovery path.
--   3. Nothing stopped a duplicate open case per aggregate, so a repeated
--      sweep could enqueue the same repair many times over.

-- 1. Payload-hash conflict detection ----------------------------------------

ALTER TABLE fleet_partner_trip_projections
    ADD COLUMN IF NOT EXISTS last_payload_hash text;

-- 2. Reconciliation case leases ---------------------------------------------

ALTER TABLE fleet_partner_projection_reconciliation_cases
    ADD COLUMN IF NOT EXISTS lease_owner text;

ALTER TABLE fleet_partner_projection_reconciliation_cases
    ADD COLUMN IF NOT EXISTS lease_expires_at timestamptz;

-- Recovery scan for cases whose worker died mid-flight.
CREATE INDEX IF NOT EXISTS idx_fleet_reconciliation_lease
    ON fleet_partner_projection_reconciliation_cases (lease_expires_at)
    WHERE status = 'IN_PROGRESS';

-- Operators list the dead-letter queue by recency.
CREATE INDEX IF NOT EXISTS idx_fleet_reconciliation_dead_letter
    ON fleet_partner_projection_reconciliation_cases (updated_at DESC)
    WHERE status = 'FAILED_PERMANENT';

-- 3. One open case per aggregate --------------------------------------------
--
-- Partial: resolved and dead-lettered cases are history and may repeat for the
-- same aggregate, but an aggregate must never have two live repairs racing
-- each other to write the same projection.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_reconciliation_open_aggregate
    ON fleet_partner_projection_reconciliation_cases
       (source_service, aggregate_type, aggregate_source_id)
    WHERE status IN ('OPEN', 'IN_PROGRESS');

-- 4. Sweep support ----------------------------------------------------------
--
-- The stale sweep and the case-opening sweep both scan trip projections by
-- freshness and event recency.
CREATE INDEX IF NOT EXISTS idx_fleet_trip_projection_freshness
    ON fleet_partner_trip_projections (freshness_state, last_event_at);

-- Trip projections are looked up by their canonical source identity on every
-- ingested trip event, which is the hottest read on this table.
CREATE INDEX IF NOT EXISTS idx_fleet_trip_projection_source
    ON fleet_partner_trip_projections (source_service, platform_trip_id);

-- Request-to-trip association lookups.
CREATE INDEX IF NOT EXISTS idx_fleet_trip_projection_request
    ON fleet_partner_trip_projections (source_service, platform_request_id)
    WHERE platform_request_id IS NOT NULL;
