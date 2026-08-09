-- Section 15 A9: asynchronous bulk-order imports. File bytes remain at Media.
CREATE TABLE IF NOT EXISTS fleet_partner_bulk_imports(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 media_file_id text NOT NULL,file_name text NOT NULL DEFAULT '',file_format text NOT NULL CHECK(file_format IN('CSV','JSON')),
 content_type text NOT NULL DEFAULT '',size_bytes bigint NOT NULL DEFAULT 0 CHECK(size_bytes>=0),status text NOT NULL DEFAULT 'UPLOADED' CHECK(status IN('UPLOADED','VALIDATING','VALIDATION_FAILED','READY','IMPORTING','COMPLETED','PARTIALLY_COMPLETED','FAILED','CANCELLED')),
 total_rows integer NOT NULL DEFAULT 0,valid_rows integer NOT NULL DEFAULT 0,invalid_rows integer NOT NULL DEFAULT 0,imported_rows integer NOT NULL DEFAULT 0,failed_rows integer NOT NULL DEFAULT 0,
 error_report_media_id text NOT NULL DEFAULT '',request_fingerprint text NOT NULL,version bigint NOT NULL DEFAULT 1,created_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,cancelled_at timestamptz,
 lease_owner text,lease_expires_at timestamptz,attempts integer NOT NULL DEFAULT 0,last_error_code text NOT NULL DEFAULT '',UNIQUE(organization_id,media_file_id,request_fingerprint),UNIQUE(organization_id,id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_bulk_import_claim ON fleet_partner_bulk_imports(status,updated_at) WHERE status IN('UPLOADED','VALIDATING','READY','IMPORTING');
CREATE TABLE IF NOT EXISTS fleet_partner_bulk_import_rows(
 id text PRIMARY KEY,organization_id text NOT NULL,import_id text NOT NULL REFERENCES fleet_partner_bulk_imports(id) ON DELETE RESTRICT,row_number integer NOT NULL,row_reference text NOT NULL,row_fingerprint text NOT NULL,status text NOT NULL CHECK(status IN('VALID','INVALID','IMPORTED','FAILED')),
 normalized_data jsonb NOT NULL DEFAULT '{}'::jsonb,generated_order_id text,safe_error_code text NOT NULL DEFAULT '',safe_error_message text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(import_id,row_reference),UNIQUE(import_id,row_fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_fleet_bulk_import_rows ON fleet_partner_bulk_import_rows(organization_id,import_id,row_number);
CREATE TABLE IF NOT EXISTS fleet_partner_bulk_import_attempts(id text PRIMARY KEY,organization_id text NOT NULL,import_id text NOT NULL REFERENCES fleet_partner_bulk_imports(id) ON DELETE RESTRICT,attempt integer NOT NULL,outcome text NOT NULL,safe_error_code text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT now());
CREATE TRIGGER trg_fleet_bulk_import_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_bulk_import_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
INSERT INTO fleet_partner_permissions(code,category,description,is_active)VALUES('imports.read','imports','Read bulk import operations',true),('imports.manage','imports','Create and cancel bulk imports',true)ON CONFLICT(code)DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)SELECT 'rol_system_owner',code FROM fleet_partner_permissions WHERE code LIKE 'imports.%' ON CONFLICT DO NOTHING;INSERT INTO fleet_partner_role_permissions(role_id,permission_code)SELECT 'rol_system_operations',code FROM fleet_partner_permissions WHERE code LIKE 'imports.%' ON CONFLICT DO NOTHING;
