-- Rollback the Section 25 search projection.

DELETE FROM fleet_partner_role_permissions WHERE permission_code = 'search.read';
DELETE FROM fleet_partner_permissions WHERE code = 'search.read';

DROP INDEX IF EXISTS idx_fleet_search_branch;
DROP INDEX IF EXISTS idx_fleet_search_subject;
DROP INDEX IF EXISTS idx_fleet_search_tenant;
DROP INDEX IF EXISTS idx_fleet_search_vector;
DROP TABLE IF EXISTS fleet_partner_search_documents;
