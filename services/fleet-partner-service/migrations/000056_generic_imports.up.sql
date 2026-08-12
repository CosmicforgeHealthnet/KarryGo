-- Section 25C: generalize the order-only bulk-import aggregate into an
-- adapter-based import framework, additively, on the SAME tables.
--
-- The existing /orders/imports flow (engine='LEGACY', the column default) is
-- untouched: Create/RunWorkerPass/Claim/Cancel/Replay in
-- internal/features/bulkimports keep reading and writing exactly the columns
-- and status values they always have. A new engine='GENERIC' value adds
-- import-type-aware dry-run/commit/retry alongside it, on new columns with
-- safe defaults, so an old row (or an old code path) never observes any of
-- this migration's additions.
--
-- The two engines' worker claim queries are kept disjoint by the engine
-- column, not by hoping status string values never collide — 'VALIDATING'
-- and 'UPLOADED' are meaningful in both vocabularies, so a discriminator
-- column is the only safe way to keep the legacy claim query
-- (status IN ('UPLOADED','VALIDATING','READY','IMPORTING')) from ever
-- picking up a generic-engine row.

ALTER TABLE fleet_partner_bulk_imports
    ADD COLUMN IF NOT EXISTS import_type text NOT NULL DEFAULT 'PRIVATE_ORDER'
        CHECK (import_type IN ('PRIVATE_ORDER', 'WORKFORCE_PROFILE', 'ASSET', 'WAREHOUSE_INVENTORY')),
    ADD COLUMN IF NOT EXISTS engine text NOT NULL DEFAULT 'LEGACY'
        CHECK (engine IN ('LEGACY', 'GENERIC')),
    ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT 'COMMIT'
        CHECK (mode IN ('DRY_RUN', 'COMMIT')),
    ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version >= 1),
    ADD COLUMN IF NOT EXISTS file_fingerprint text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS external_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS committed_rows integer NOT NULL DEFAULT 0 CHECK (committed_rows >= 0),
    ADD COLUMN IF NOT EXISTS validation_version integer NOT NULL DEFAULT 0 CHECK (validation_version >= 0);

-- Widen the status vocabulary additively. The original status CHECK
-- (migration 000031) is an unnamed inline column constraint, so its
-- Postgres-assigned name is discovered rather than guessed.
DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_bulk_imports'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%status%'
      AND pg_get_constraintdef(con.oid) ILIKE '%UPLOADED%';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_bulk_imports DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;

ALTER TABLE fleet_partner_bulk_imports
    ADD CONSTRAINT fleet_partner_bulk_imports_status_check_v2
    CHECK (status IN (
        -- Legacy (engine='LEGACY') vocabulary, unchanged.
        'UPLOADED', 'VALIDATING', 'VALIDATION_FAILED', 'READY', 'IMPORTING',
        'COMPLETED', 'PARTIALLY_COMPLETED', 'FAILED', 'CANCELLED',
        -- Generic (engine='GENERIC') vocabulary, additive.
        'VALIDATION_PENDING', 'VALIDATED', 'COMMIT_PENDING', 'COMMITTING',
        'PARTIALLY_COMMITTED', 'COMMITTED', 'DEAD_LETTER'
    ));

CREATE INDEX IF NOT EXISTS idx_fleet_generic_import_claim_validate
    ON fleet_partner_bulk_imports (status, updated_at) WHERE engine = 'GENERIC' AND status = 'VALIDATION_PENDING';
CREATE INDEX IF NOT EXISTS idx_fleet_generic_import_claim_commit
    ON fleet_partner_bulk_imports (status, updated_at) WHERE engine = 'GENERIC' AND status = 'COMMIT_PENDING';
CREATE INDEX IF NOT EXISTS idx_fleet_generic_import_claim_retry
    ON fleet_partner_bulk_imports (status, updated_at) WHERE engine = 'GENERIC' AND status = 'FAILED';
CREATE INDEX IF NOT EXISTS idx_fleet_generic_import_tenant
    ON fleet_partner_bulk_imports (organization_id, import_type, created_at DESC) WHERE engine = 'GENERIC';

-- ---------------------------------------------------------------------------
-- Rows
-- ---------------------------------------------------------------------------
--
-- row_reference and row_fingerprint (migration 000031) are reused as the
-- generic engine's stable row key and data fingerprint rather than adding
-- duplicate columns for the same concept — they already carry
-- UNIQUE(import_id,row_reference) and UNIQUE(import_id,row_fingerprint),
-- which is exactly "identical retry is harmless, changed data under the same
-- key conflicts" for free. A corrected replacement row is a NEW row record
-- (never an in-place edit) whose row_reference is
-- "<externalReference>#v<rowVersion>" — unique per version — while
-- external_reference carries the stable identity shared across every
-- version of the same logical row.

ALTER TABLE fleet_partner_bulk_import_rows
    ADD COLUMN IF NOT EXISTS external_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version >= 1),
    ADD COLUMN IF NOT EXISTS commit_status text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS generated_subject_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS row_version integer NOT NULL DEFAULT 1 CHECK (row_version >= 1),
    ADD COLUMN IF NOT EXISTS replaces_row_id text REFERENCES fleet_partner_bulk_import_rows (id) ON DELETE SET NULL;

DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_bulk_import_rows'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%status%'
      AND pg_get_constraintdef(con.oid) ILIKE '%VALID%';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_bulk_import_rows DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;

ALTER TABLE fleet_partner_bulk_import_rows
    ADD CONSTRAINT fleet_partner_bulk_import_rows_status_check_v2
    CHECK (status IN ('VALID', 'INVALID', 'IMPORTED', 'FAILED', 'PENDING', 'COMMITTED', 'SKIPPED'));

ALTER TABLE fleet_partner_bulk_import_rows
    ADD CONSTRAINT fleet_partner_bulk_import_rows_commit_status_check
    CHECK (commit_status IN ('', 'PENDING', 'COMMITTED', 'FAILED', 'SKIPPED'));

CREATE INDEX IF NOT EXISTS idx_fleet_generic_import_rows_external_ref
    ON fleet_partner_bulk_import_rows (import_id, external_reference, row_version DESC)
    WHERE external_reference <> '';

-- ---------------------------------------------------------------------------
-- Backfill
-- ---------------------------------------------------------------------------
--
-- Every row that existed before this migration is a legacy order import; the
-- column defaults already give it import_type='PRIVATE_ORDER',
-- engine='LEGACY', mode='COMMIT' on the ADD COLUMN itself (Postgres applies a
-- non-volatile DEFAULT to existing rows immediately, in place, without a
-- table rewrite loop) — so there is nothing left for a separate UPDATE to do.

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('imports.validate', 'imports', 'Run dry-run validation for a generic import', true),
    ('imports.commit', 'imports', 'Commit validated rows of a generic import', true),
    ('imports.retry', 'imports', 'Retry failed rows or a dead-lettered generic import', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM fleet_partner_roles r
JOIN fleet_partner_permissions p ON p.code IN ('imports.validate', 'imports.commit', 'imports.retry')
WHERE r.id IN ('rol_system_owner', 'rol_system_operations')
ON CONFLICT DO NOTHING;
