DO $$ DECLARE c text; BEGIN
  SELECT conname INTO c FROM pg_constraint WHERE conrelid='fleet_partner_export_jobs'::regclass AND contype='c' AND pg_get_constraintdef(oid) ILIKE '%export_type%';
  IF c IS NOT NULL THEN EXECUTE 'ALTER TABLE fleet_partner_export_jobs DROP CONSTRAINT '||quote_ident(c); END IF;
END $$;
ALTER TABLE fleet_partner_export_jobs ADD CONSTRAINT fleet_partner_export_jobs_export_type_check_v3
  CHECK (export_type IN ('ORDERS','SHIPMENTS','PROVIDERS_WORKFORCE','ASSETS_MAINTENANCE','WAREHOUSE_INVENTORY','COMPLIANCE_INCIDENTS','INVOICES_RECEIVABLES','FULFILMENT_ORDERS','SUPPORT_CASE_PROJECTIONS'));

ALTER TABLE fleet_partner_search_documents ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'INTERNAL'
  CHECK (visibility IN ('INTERNAL','ORGANIZATION'));

CREATE TABLE fleet_partner_search_index_jobs (
  id text PRIMARY KEY, organization_id text NOT NULL, branch_id text, subject_type text NOT NULL,
  subject_reference text NOT NULL, source_version bigint NOT NULL, operation text NOT NULL CHECK(operation IN('UPSERT','REMOVE')),
  status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','LEASED','COMPLETED','DEAD_LETTER')),
  attempts int NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT now(), lease_owner text,
  lease_expires_at timestamptz, heartbeat_at timestamptz, last_error_code text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(organization_id,subject_type,subject_reference,source_version,operation)
);
CREATE INDEX fleet_partner_search_jobs_claim ON fleet_partner_search_index_jobs(status,available_at) WHERE status IN('PENDING','LEASED');
CREATE TABLE fleet_partner_search_index_attempts (
  id text PRIMARY KEY, job_id text NOT NULL REFERENCES fleet_partner_search_index_jobs(id), organization_id text NOT NULL,
  attempt int NOT NULL, outcome text NOT NULL, safe_error_code text NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER fleet_search_index_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_search_index_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE OR REPLACE FUNCTION fleet_enqueue_search_index() RETURNS trigger AS $$
DECLARE r jsonb; op text; v bigint; BEGIN
  r:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
  op:=CASE WHEN TG_OP='DELETE' THEN 'REMOVE' ELSE 'UPSERT' END;
  v:=COALESCE((r->>'version')::bigint,1);
  INSERT INTO fleet_partner_search_index_jobs(id,organization_id,branch_id,subject_type,subject_reference,source_version,operation)
  VALUES('sij_'||replace(gen_random_uuid()::text,'-',''),r->>'organization_id',NULLIF(r->>'branch_id',''),TG_ARGV[0],r->>'id',v,op)
  ON CONFLICT DO NOTHING; RETURN COALESCE(NEW,OLD);
END $$ LANGUAGE plpgsql;
CREATE TRIGGER search_customers AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_company_customers FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('CUSTOMER');
CREATE TRIGGER search_workforce AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_workforce_profiles FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('PROVIDER_WORKFORCE');
CREATE TRIGGER search_shipments AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_shipments FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('SHIPMENT');
CREATE TRIGGER search_packages AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_packages FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('PACKAGE');
CREATE TRIGGER search_manifests AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_manifests FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('MANIFEST');
CREATE TRIGGER search_routes AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_route_plans FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('ROUTE');
CREATE TRIGGER search_assets AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_assets FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('ASSET');
CREATE TRIGGER search_incidents AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_incidents FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('INCIDENT');
CREATE TRIGGER search_compliance AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_compliance_requirements FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('COMPLIANCE');
CREATE TRIGGER search_invoices AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_invoices FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('INVOICE');
CREATE TRIGGER search_inventory AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_inventory_items FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('WAREHOUSE_INVENTORY');
CREATE TRIGGER search_fulfilment AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_fulfilment_orders FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('FULFILMENT_ORDER');
CREATE TRIGGER search_support AFTER INSERT OR UPDATE OR DELETE ON fleet_partner_support_case_projections FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index('SUPPORT_CASE');

-- Deployment backfill: triggers cover every transition after this migration,
-- while these durable jobs make pre-existing rows converge through the exact
-- same worker rather than bypassing its version and permission rules.
DO $$ DECLARE spec record; BEGIN
  FOR spec IN SELECT * FROM (VALUES
    ('fleet_partner_company_customers','CUSTOMER'),
    ('fleet_partner_workforce_profiles','PROVIDER_WORKFORCE'),
    ('fleet_partner_shipments','SHIPMENT'),
    ('fleet_partner_packages','PACKAGE'),
    ('fleet_partner_manifests','MANIFEST'),
    ('fleet_partner_route_plans','ROUTE'),
    ('fleet_partner_assets','ASSET'),
    ('fleet_partner_incidents','INCIDENT'),
    ('fleet_partner_compliance_requirements','COMPLIANCE'),
    ('fleet_partner_invoices','INVOICE'),
    ('fleet_partner_inventory_items','WAREHOUSE_INVENTORY'),
    ('fleet_partner_fulfilment_orders','FULFILMENT_ORDER'),
    ('fleet_partner_support_case_projections','SUPPORT_CASE')
  ) AS v(table_name,subject_type)
  LOOP
    EXECUTE format(
      'INSERT INTO fleet_partner_search_index_jobs(id,organization_id,branch_id,subject_type,subject_reference,source_version,operation) '
      'SELECT ''sij_''||replace(gen_random_uuid()::text,''-'',''''),t.organization_id,NULLIF(to_jsonb(t)->>''branch_id'',''''),%L,t.id,COALESCE(NULLIF(to_jsonb(t)->>''version'','''')::bigint,1),''UPSERT'' FROM %I t ON CONFLICT DO NOTHING',
      spec.subject_type,spec.table_name);
  END LOOP;
END $$;

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
 ('search.replay','search','Replay dead-lettered search indexing work',true)
ON CONFLICT(code) DO UPDATE SET is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT id,'search.replay' FROM fleet_partner_roles WHERE id IN('rol_system_owner','rol_system_admin') ON CONFLICT DO NOTHING;
