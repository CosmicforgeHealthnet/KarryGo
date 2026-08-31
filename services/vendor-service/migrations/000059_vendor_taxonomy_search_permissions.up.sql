-- Section 13I: canonical taxonomy and search permission vocabulary.
--
-- Three of these six capabilities are PLATFORM capabilities, not vendor
-- capabilities. Global taxonomy publication, ranking-weight administration and
-- search projection generation administration decide what every vendor on the
-- platform is discoverable as; a vendor holding them could reclassify or
-- outrank the rest of the marketplace. They are therefore deliberately granted
-- to NO vendor role family below — not even Owner.
--
-- That separation survives on its own for a structural reason worth recording:
-- migration 000005's "Owner: every permission" grant is a one-time snapshot
-- taken when 000005 ran, so a permission introduced here is never swept into
-- role_owner retroactively. This migration relies on that and adds no grant
-- that would undo it. internal/features/taxonomy's Section 13I IAM tests pin
-- the result against the live database rather than against this comment.
--
-- Authorization for the three platform capabilities is external to Vendor
-- staff IAM: the platform taxonomy and projection-administration surfaces are
-- root-mounted internal APIs authenticated with serviceauth HMAC-v2 and an
-- explicit caller allowlist (see docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md).
-- The permission codes are still catalogued here so that a future platform
-- operator console has a single, named vocabulary to bind to, and so that the
-- IAM tests can assert their absence from every vendor role.

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
 ('perm_taxonomy_read','vendor.taxonomy.read','Read published canonical taxonomy facets, nodes and vocabulary needed to classify a vendor catalogue.','taxonomy',false),
 ('perm_taxonomy_mapping_manage','vendor.taxonomy.mapping.manage','Manage canonical taxonomy mappings and semantic assignments for resources inside the vendor''s own scope.','taxonomy',true),
 ('perm_taxonomy_platform_admin','vendor.taxonomy.platform.admin','PLATFORM ONLY. Administer and publish the global canonical taxonomy for every vendor.','taxonomy_platform',true),
 ('perm_search_read','vendor.search.read','Read Food and Shop search projections, facets and suggestions.','search',false),
 ('perm_search_ranking_admin','vendor.search.ranking.admin','PLATFORM ONLY. Inspect and administer global search ranking profiles and weights.','search_platform',true),
 ('perm_search_projection_admin','vendor.search.projection.admin','PLATFORM ONLY. Administer search projection generations, rebuilds, activation and recovery.','search_platform',true)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Vendor grants: read and own-scope mapping only.
-- ---------------------------------------------------------------------------

-- Owner and General Manager may read taxonomy and search and may classify
-- their own catalogue. Neither receives a platform capability.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_owner'), ('role_general_manager')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN ('vendor.taxonomy.read','vendor.taxonomy.mapping.manage','vendor.search.read')
ON CONFLICT DO NOTHING;

-- Branch Manager classifies within their own branch scope; the branch bound
-- comes from the role assignment's branch_id, not from a separate permission.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions
WHERE code IN ('vendor.taxonomy.read','vendor.taxonomy.mapping.manage','vendor.search.read')
ON CONFLICT DO NOTHING;

-- Catalogue-owning operational roles classify the catalogue they maintain.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_kitchen_pastry'), ('role_shop_inventory')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN ('vendor.taxonomy.read','vendor.taxonomy.mapping.manage','vendor.search.read')
ON CONFLICT DO NOTHING;

-- Marketing reads how the catalogue is classified and discovered, but does not
-- reclassify it: a marketing-driven mapping change is a discovery change.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_marketing', id FROM vendor_permissions
WHERE code IN ('vendor.taxonomy.read','vendor.search.read')
ON CONFLICT DO NOTHING;

-- Read-only roles read only.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', id FROM vendor_permissions
WHERE code IN ('vendor.taxonomy.read','vendor.search.read')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_procurement', id FROM vendor_permissions
WHERE code IN ('vendor.taxonomy.read','vendor.search.read')
ON CONFLICT DO NOTHING;

-- Finance, CRM/Support and Vendor Rider receive nothing here: none of them
-- classifies a catalogue or administers discovery.

-- No role receives vendor.taxonomy.platform.admin,
-- vendor.search.ranking.admin or vendor.search.projection.admin.
