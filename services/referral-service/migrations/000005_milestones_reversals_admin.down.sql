DROP TABLE IF EXISTS referral_admin_actions;
DROP INDEX IF EXISTS fraud_flags_referral_idx;
DELETE FROM fraud_flags WHERE flag_type = 'collusive_job';
ALTER TABLE fraud_flags DROP CONSTRAINT IF EXISTS fraud_flags_flag_type_check;
ALTER TABLE fraud_flags ADD CONSTRAINT fraud_flags_flag_type_check CHECK (flag_type IN (
    'self_referral', 'duplicate_identity', 'identity_conflict', 'duplicate_device',
    'duplicate_bank', 'repeated_reversals', 'manual'));
DROP INDEX IF EXISTS reward_ledger_reverses_idx;
ALTER TABLE referral_track_jobs DROP COLUMN IF EXISTS reversed_at;
DROP TABLE IF EXISTS settled_job_reversals;
ALTER TABLE referral_persons DROP COLUMN IF EXISTS power_user_since;
DROP TABLE IF EXISTS referral_milestones;
