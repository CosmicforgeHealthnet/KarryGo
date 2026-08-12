DELETE FROM fleet_partner_role_permissions WHERE permission_code IN ('imports.validate', 'imports.commit', 'imports.retry');
DELETE FROM fleet_partner_permissions WHERE code IN ('imports.validate', 'imports.commit', 'imports.retry');

DROP INDEX IF EXISTS idx_fleet_generic_import_rows_external_ref;

ALTER TABLE fleet_partner_bulk_import_rows
    DROP CONSTRAINT IF EXISTS fleet_partner_bulk_import_rows_commit_status_check,
    DROP CONSTRAINT IF EXISTS fleet_partner_bulk_import_rows_status_check_v2,
    ADD CONSTRAINT fleet_partner_bulk_import_rows_status_check CHECK (status IN ('VALID', 'INVALID', 'IMPORTED', 'FAILED'));

ALTER TABLE fleet_partner_bulk_import_rows
    DROP COLUMN IF EXISTS replaces_row_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS generated_subject_reference,
    DROP COLUMN IF EXISTS commit_status,
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS external_reference;

DROP INDEX IF EXISTS idx_fleet_generic_import_tenant;
DROP INDEX IF EXISTS idx_fleet_generic_import_claim_retry;
DROP INDEX IF EXISTS idx_fleet_generic_import_claim_commit;
DROP INDEX IF EXISTS idx_fleet_generic_import_claim_validate;

ALTER TABLE fleet_partner_bulk_imports
    DROP CONSTRAINT IF EXISTS fleet_partner_bulk_imports_status_check_v2,
    ADD CONSTRAINT fleet_partner_bulk_imports_status_check
        CHECK (status IN ('UPLOADED', 'VALIDATING', 'VALIDATION_FAILED', 'READY', 'IMPORTING',
                           'COMPLETED', 'PARTIALLY_COMPLETED', 'FAILED', 'CANCELLED'));

ALTER TABLE fleet_partner_bulk_imports
    DROP COLUMN IF EXISTS validation_version,
    DROP COLUMN IF EXISTS committed_rows,
    DROP COLUMN IF EXISTS external_reference,
    DROP COLUMN IF EXISTS file_fingerprint,
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS mode,
    DROP COLUMN IF EXISTS engine,
    DROP COLUMN IF EXISTS import_type;
