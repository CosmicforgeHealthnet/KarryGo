ALTER TABLE pastry_quotes DROP CONSTRAINT IF EXISTS pastry_quotes_status_check;
ALTER TABLE pastry_quotes ADD CONSTRAINT pastry_quotes_status_check CHECK (status IN ('DRAFT','PENDING_APPROVAL','APPROVED','ACCEPTED','DECLINED','EXPIRED','SUPERSEDED'));
ALTER TABLE pastry_quotes ADD COLUMN IF NOT EXISTS superseded_at timestamptz;
ALTER TABLE pastry_accepted_snapshots ADD COLUMN IF NOT EXISTS logistics_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE pastry_accepted_snapshots ADD COLUMN IF NOT EXISTS risk_flags jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE pastry_milestone_history ADD COLUMN IF NOT EXISTS actor_id text;
ALTER TABLE pastry_milestone_history ADD COLUMN IF NOT EXISTS correlation_id text;
ALTER TABLE pastry_milestone_history ADD COLUMN IF NOT EXISTS note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280);
ALTER TABLE pastry_custom_requests ADD COLUMN IF NOT EXISTS cancellation_policy jsonb NOT NULL DEFAULT '{}'::jsonb;
