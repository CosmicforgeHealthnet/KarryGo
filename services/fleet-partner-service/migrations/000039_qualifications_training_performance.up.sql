-- Sections 17.2: qualifications, training and immutable explainable performance.
CREATE TABLE fleet_partner_qualifications(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,code text NOT NULL,name text NOT NULL,
 applicable_roles jsonb NOT NULL DEFAULT '[]',applicable_services jsonb NOT NULL DEFAULT '[]',applicable_asset_types jsonb NOT NULL DEFAULT '[]',
 validity_period_seconds bigint,evidence_required boolean NOT NULL DEFAULT false,status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,code)
);
CREATE TABLE fleet_partner_provider_qualifications(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,membership_id text NOT NULL,
 qualification_id text NOT NULL REFERENCES fleet_partner_qualifications(id) ON DELETE RESTRICT,evidence_media_id text NOT NULL DEFAULT '',verification_reference text NOT NULL DEFAULT '',
 issued_at timestamptz,expires_at timestamptz,status text NOT NULL DEFAULT 'PENDING_VERIFICATION' CHECK(status IN('PENDING_VERIFICATION','VERIFIED','REJECTED','EXPIRED','REVOKED')),
 verified_by text NOT NULL DEFAULT '',verified_at timestamptz,version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,membership_id,qualification_id),CHECK(expires_at IS NULL OR issued_at IS NULL OR expires_at>issued_at)
);
CREATE INDEX idx_fleet_provider_qualifications_expiry ON fleet_partner_provider_qualifications(organization_id,membership_id,status,expires_at);
CREATE TABLE fleet_partner_training_courses(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,code text NOT NULL,title text NOT NULL,provider_reference text NOT NULL DEFAULT '',
 required_roles jsonb NOT NULL DEFAULT '[]',required_services jsonb NOT NULL DEFAULT '[]',validity_period_seconds bigint,passing_score numeric(6,2) NOT NULL DEFAULT 0,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ARCHIVED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,code)
);
CREATE TABLE fleet_partner_training_completions(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,course_id text NOT NULL REFERENCES fleet_partner_training_courses(id) ON DELETE RESTRICT,
 membership_id text NOT NULL,score numeric(6,2) NOT NULL,status text NOT NULL CHECK(status IN('PASSED','FAILED','EXPIRED','REVOKED')),certificate_media_id text NOT NULL DEFAULT '',
 completed_at timestamptz NOT NULL,expires_at timestamptz,verified_by text NOT NULL DEFAULT '',version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,course_id,membership_id,completed_at)
);
CREATE INDEX idx_fleet_training_expiry ON fleet_partner_training_completions(organization_id,membership_id,status,expires_at);
CREATE TABLE fleet_partner_performance_snapshots(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,membership_id text NOT NULL,period_start timestamptz NOT NULL,period_end timestamptz NOT NULL,
 metric_inputs jsonb NOT NULL,calculated_metrics jsonb NOT NULL,explanation jsonb NOT NULL,source_versions jsonb NOT NULL,generation_status text NOT NULL CHECK(generation_status IN('COMPLETE','INCOMPLETE')),
 formula_version text NOT NULL,generated_at timestamptz NOT NULL DEFAULT now(),version bigint NOT NULL DEFAULT 1,UNIQUE(organization_id,membership_id,period_start,period_end,formula_version),CHECK(period_end>period_start)
);
CREATE TRIGGER trg_fleet_performance_snapshots_append_only BEFORE UPDATE OR DELETE ON fleet_partner_performance_snapshots FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
