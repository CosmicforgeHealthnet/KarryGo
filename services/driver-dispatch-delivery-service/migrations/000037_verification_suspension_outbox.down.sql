DROP INDEX IF EXISTS uq_dispatch_outbox_aggregate_event_version;
ALTER TABLE providers
  DROP COLUMN IF EXISTS verification_suspension_reason,
  DROP COLUMN IF EXISTS verification_source_version;
