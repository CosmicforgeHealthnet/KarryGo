-- Section 25D gap-close: widen export_type additively to add the four
-- dataset adapters wired in this change (SHIPMENTS, PROVIDERS_WORKFORCE,
-- ASSETS_MAINTENANCE, WAREHOUSE_INVENTORY). The remaining datasets named in
-- the spec (COMPLIANCE_INCIDENTS, INVOICES_RECEIVABLES, FULFILMENT_ORDERS,
-- SUPPORT_CASE_PROJECTIONS) are not added here because no adapter exists yet
-- to serve them — adding the DB value without a registered Exporter would
-- let a request be accepted and then fail at generation time instead of
-- being refused up front as "recognized but not available."
DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_export_jobs'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%export_type%'
      AND pg_get_constraintdef(con.oid) ILIKE '%ORDERS%';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_export_jobs DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;

ALTER TABLE fleet_partner_export_jobs
    ADD CONSTRAINT fleet_partner_export_jobs_export_type_check_v2
    CHECK (export_type IN ('ORDERS', 'SHIPMENTS', 'PROVIDERS_WORKFORCE',
                            'ASSETS_MAINTENANCE', 'WAREHOUSE_INVENTORY'));
