-- Section 25D: generic asynchronous export framework. File bytes remain at
-- Media, exactly like fleet_partner_bulk_imports/documents. Fleet stores only
-- the reference and the operational metadata around producing it.
CREATE TABLE IF NOT EXISTS fleet_partner_export_jobs(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 export_type text NOT NULL CHECK(export_type IN('ORDERS')),format text NOT NULL DEFAULT 'CSV' CHECK(format IN('CSV')),
 filter_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,requested_columns jsonb NOT NULL DEFAULT '[]'::jsonb,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','READY','FAILED','EXPIRED','CANCELLED','DEAD_LETTER')),
 request_fingerprint text NOT NULL DEFAULT '',idempotency_key text NOT NULL DEFAULT '',
 source_cutoff_at timestamptz,media_file_id text NOT NULL DEFAULT '',row_count integer NOT NULL DEFAULT 0 CHECK(row_count>=0),
 expires_at timestamptz,created_by text NOT NULL DEFAULT '',version bigint NOT NULL DEFAULT 1 CHECK(version>=1),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,cancelled_at timestamptz,
 lease_owner text,lease_expires_at timestamptz,attempts integer NOT NULL DEFAULT 0,last_error_code text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_export_idempotency
    ON fleet_partner_export_jobs(organization_id,idempotency_key) WHERE idempotency_key<>'';
CREATE INDEX IF NOT EXISTS idx_fleet_export_claim
    ON fleet_partner_export_jobs(status,updated_at) WHERE status IN('PENDING','RUNNING');
CREATE INDEX IF NOT EXISTS idx_fleet_export_tenant
    ON fleet_partner_export_jobs(organization_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_export_expiry
    ON fleet_partner_export_jobs(status,expires_at) WHERE status='READY';

-- Immutable attempt history, same append-only trigger already used by
-- fleet_partner_bulk_import_attempts.
CREATE TABLE IF NOT EXISTS fleet_partner_export_attempts(
 id text PRIMARY KEY,organization_id text NOT NULL,export_id text NOT NULL REFERENCES fleet_partner_export_jobs(id) ON DELETE RESTRICT,
 attempt integer NOT NULL,outcome text NOT NULL,safe_error_code text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_fleet_export_attempts ON fleet_partner_export_attempts(organization_id,export_id,occurred_at DESC);
CREATE TRIGGER trg_fleet_export_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_export_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------
--
-- Three separate capabilities, mirroring documents' precedent: listing an
-- export job's status, requesting/cancelling one, and obtaining the file
-- itself are different things. A caller who may request an export is not
-- automatically someone who may download every export in the organization.

INSERT INTO fleet_partner_permissions(code,category,description,is_active)VALUES
    ('exports.read','exports','Read export job status and history',true),
    ('exports.manage','exports','Request and cancel data exports',true),
    ('exports.download','exports','Obtain short-lived access to a completed export file',true)
ON CONFLICT(code)DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;

INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code LIKE 'exports.%'
WHERE r.id IN('rol_system_owner','rol_system_operations') ON CONFLICT DO NOTHING;
