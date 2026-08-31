UPDATE capacity_reservations SET source_type='MANUAL' WHERE source_type='PASTRY_PRODUCT';
ALTER TABLE production_slots DROP COLUMN IF EXISTS archived_at;
UPDATE production_jobs SET source_type='MANUAL' WHERE source_type='PASTRY_PRODUCT';
DROP INDEX IF EXISTS capacity_reservation_allocations_line_window_idx;
ALTER TABLE cloud_kitchen_quantity_bands
    DROP COLUMN IF EXISTS capacity_units_per_item;
DROP TABLE IF EXISTS cloud_kitchen_package_versions;
ALTER TABLE cloud_kitchen_corporate_plans
    DROP COLUMN IF EXISTS reservation_horizon_days;
ALTER TABLE pastry_accepted_snapshots
    DROP COLUMN IF EXISTS accepted_at,
    DROP COLUMN IF EXISTS vendor_terms,
    DROP COLUMN IF EXISTS capacity_snapshot;
DROP TABLE IF EXISTS pastry_product_price_history;
DROP TABLE IF EXISTS pastry_product_versions;
UPDATE pastry_custom_requests SET status='ACCEPTED' WHERE status='ACCEPTED_PENDING_ORDER';
UPDATE pastry_custom_requests SET status='SUBMITTED' WHERE status IN ('UNDER_REVIEW','QUOTE_PENDING');
UPDATE pastry_custom_requests SET status='DECLINED' WHERE status IN ('EXPIRED','CONVERTED');
ALTER TABLE pastry_custom_requests DROP CONSTRAINT IF EXISTS pastry_custom_requests_status_check;
ALTER TABLE pastry_custom_requests ADD CONSTRAINT pastry_custom_requests_status_check
    CHECK (status IN ('DRAFT','SUBMITTED','QUOTED','ACCEPTED','DECLINED','CANCELLED','COMPLETED'));
ALTER TABLE pastry_custom_requests
    DROP COLUMN IF EXISTS change_policy,
    DROP COLUMN IF EXISTS corporate_profile_ref;
DROP TABLE IF EXISTS pastry_quote_capacity_requirements;
ALTER TABLE pastry_quotes
    DROP CONSTRAINT IF EXISTS pastry_quotes_production_window_check,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS negotiation_note,
    DROP COLUMN IF EXISTS negotiation_reason,
    DROP COLUMN IF EXISTS vendor_terms,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approval_required,
    DROP COLUMN IF EXISTS production_end,
    DROP COLUMN IF EXISTS production_start;
UPDATE pastry_quotes SET status='DECLINED' WHERE status IN ('SUPERSEDED','CANCELLED');
ALTER TABLE pastry_quotes DROP CONSTRAINT IF EXISTS pastry_quotes_status_check;
ALTER TABLE pastry_quotes ADD CONSTRAINT pastry_quotes_status_check
    CHECK (status IN ('DRAFT','PENDING_APPROVAL','APPROVED','ACCEPTED','DECLINED','EXPIRED'));
ALTER TABLE capacity_reservation_allocations
    DROP CONSTRAINT IF EXISTS capacity_reservation_allocations_line_fk,
    DROP COLUMN IF EXISTS production_line_id;
ALTER TABLE capacity_reservations
    DROP CONSTRAINT IF EXISTS capacity_reservations_source_type_check;
ALTER TABLE capacity_reservations
    ADD CONSTRAINT capacity_reservations_source_type_check
    CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                           'CUSTOM_CAKE_QUOTE','PRODUCTION_JOB','EXTERNAL_ORDER'));
ALTER TABLE production_jobs
    DROP CONSTRAINT IF EXISTS production_jobs_source_type_check;
ALTER TABLE production_jobs
    ADD CONSTRAINT production_jobs_source_type_check
    CHECK (source_type IN ('MANUAL','BULK_PACKAGE','CORPORATE_OCCURRENCE',
                           'CUSTOM_CAKE_QUOTE','EXTERNAL_ORDER'));

ALTER TABLE pastry_products
    DROP COLUMN IF EXISTS workload_units_per_item,
    DROP COLUMN IF EXISTS cleanup_buffer_minutes,
    DROP COLUMN IF EXISTS setup_buffer_minutes;
ALTER TABLE cloud_kitchen_packages
    DROP COLUMN IF EXISTS cleanup_buffer_minutes,
    DROP COLUMN IF EXISTS setup_buffer_minutes;
ALTER TABLE production_lines
    DROP COLUMN IF EXISTS cleanup_buffer_minutes,
    DROP COLUMN IF EXISTS setup_buffer_minutes;
