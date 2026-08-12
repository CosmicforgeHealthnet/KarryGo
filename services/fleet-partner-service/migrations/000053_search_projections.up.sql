-- Section 25: PostgreSQL full-text search projection.
--
-- This is a read-side projection, never an authority. A search failure must
-- never block an authoritative operation, and a stale or missing row here
-- means a subject is harder to find, not that it does not exist. No
-- Elasticsearch, OpenSearch or other external search cluster: PostgreSQL
-- tsvector plus a GIN index, per the binding scope for this section.
--
-- Permission is enforced in the same query that reads tenant and branch
-- scope, and required_permission is data the query filters on — never a
-- decision made after a row has already been returned to the caller.

CREATE TABLE IF NOT EXISTS fleet_partner_search_documents (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id             text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    subject_type          text NOT NULL CHECK (length(btrim(subject_type)) > 0),
    subject_reference     text NOT NULL CHECK (length(btrim(subject_reference)) > 0),
    title                 text NOT NULL CHECK (length(btrim(title)) > 0),
    -- Kept alongside the generated tsvector so a projection can be inspected
    -- or re-indexed without reaching back into the owning feature.
    searchable_text       text NOT NULL DEFAULT '',
    exact_reference_fields jsonb NOT NULL DEFAULT '{}'::jsonb,
    status                text NOT NULL DEFAULT '',
    -- The permission a caller must hold for this row to ever be returned.
    -- Never optional: a document with no required permission would be a
    -- document nobody meant to make searchable by everyone.
    required_permission   text NOT NULL CHECK (length(btrim(required_permission)) > 0),
    source_version        bigint NOT NULL DEFAULT 0,
    search_vector         tsvector GENERATED ALWAYS AS (
                               setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
                               setweight(to_tsvector('english', coalesce(searchable_text, '')), 'B')
                           ) STORED,
    indexed_at            timestamptz NOT NULL DEFAULT now(),
    available             boolean NOT NULL DEFAULT true,
    UNIQUE (organization_id, subject_type, subject_reference)
);

CREATE INDEX IF NOT EXISTS idx_fleet_search_vector
    ON fleet_partner_search_documents USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_fleet_search_tenant
    ON fleet_partner_search_documents (organization_id, available, indexed_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_search_subject
    ON fleet_partner_search_documents (organization_id, subject_type, subject_reference);
CREATE INDEX IF NOT EXISTS idx_fleet_search_branch
    ON fleet_partner_search_documents (organization_id, branch_id)
    WHERE branch_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('search.read', 'search', 'Search organization operational records', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM fleet_partner_roles r
JOIN fleet_partner_permissions p ON p.code = 'search.read'
WHERE r.id IN ('rol_system_owner', 'rol_system_operations', 'rol_system_admin',
               'rol_system_compliance', 'rol_system_crm', 'rol_system_analyst', 'rol_system_fleet')
ON CONFLICT DO NOTHING;
