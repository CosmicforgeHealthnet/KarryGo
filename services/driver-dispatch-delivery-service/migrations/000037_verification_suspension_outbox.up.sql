ALTER TABLE providers
  ADD COLUMN IF NOT EXISTS verification_source_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS verification_suspension_reason TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_outbox_aggregate_event_version
  ON dispatch_marketplace_outbox(aggregate_type,aggregate_id,aggregate_version,event_type);
