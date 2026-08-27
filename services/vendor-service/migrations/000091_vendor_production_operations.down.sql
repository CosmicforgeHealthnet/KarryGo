ALTER TABLE production_jobs DROP CONSTRAINT IF EXISTS production_jobs_ready_evidence_fk;
DROP TABLE IF EXISTS commerce_vendor_order_readiness;
DROP INDEX IF EXISTS commerce_order_missions_production_timing_idx;
ALTER TABLE commerce_order_missions
    DROP COLUMN IF EXISTS production_ready_window_start,
    DROP COLUMN IF EXISTS production_ready_window_end,
    DROP COLUMN IF EXISTS production_timing_source_job_id,
    DROP COLUMN IF EXISTS production_timing_version;
DROP TABLE IF EXISTS production_job_ready_evidence;
DROP TABLE IF EXISTS production_job_delay_events;
DROP TABLE IF EXISTS production_job_steps;
DROP INDEX IF EXISTS production_jobs_operational_queue_idx;
DROP INDEX IF EXISTS production_jobs_external_order_source_uq;
ALTER TABLE production_jobs
    DROP COLUMN IF EXISTS ready_evidence_id,
    DROP COLUMN IF EXISTS promised_ready_at;
