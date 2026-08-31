ALTER TABLE pastry_custom_requests DROP COLUMN IF EXISTS cancellation_policy;
ALTER TABLE pastry_milestone_history DROP COLUMN IF EXISTS note;
ALTER TABLE pastry_milestone_history DROP COLUMN IF EXISTS correlation_id;
ALTER TABLE pastry_milestone_history DROP COLUMN IF EXISTS actor_id;
ALTER TABLE pastry_accepted_snapshots DROP COLUMN IF EXISTS risk_flags;
ALTER TABLE pastry_accepted_snapshots DROP COLUMN IF EXISTS logistics_snapshot;
ALTER TABLE pastry_quotes DROP COLUMN IF EXISTS superseded_at;
ALTER TABLE pastry_quotes DROP CONSTRAINT IF EXISTS pastry_quotes_status_check;
ALTER TABLE pastry_quotes ADD CONSTRAINT pastry_quotes_status_check CHECK (status IN ('DRAFT','PENDING_APPROVAL','APPROVED','ACCEPTED','DECLINED','EXPIRED'));
