ALTER TABLE fleet_partner_export_jobs
    DROP CONSTRAINT IF EXISTS fleet_partner_export_jobs_export_type_check_v2,
    ADD CONSTRAINT fleet_partner_export_jobs_export_type_check
        CHECK (export_type IN ('ORDERS'));
