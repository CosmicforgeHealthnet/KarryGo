-- Section 13A — KarryGo Canonical Discovery Taxonomy rollback.
--
-- Reverse only objects introduced by migration 000047.
-- Existing Food, Shop, vendor_outbox and shared platform infrastructure
-- remain untouched.

DROP TABLE IF EXISTS canonical_taxonomy_publication_history;
DROP TABLE IF EXISTS canonical_taxonomy_change_history;
DROP TABLE IF EXISTS canonical_taxonomy_commands;
DROP TABLE IF EXISTS canonical_taxonomy_nodes;
DROP TABLE IF EXISTS canonical_taxonomy_facets;
DROP TABLE IF EXISTS canonical_taxonomy_versions;
DROP TABLE IF EXISTS canonical_taxonomy_namespaces;

DROP FUNCTION IF EXISTS canonical_taxonomy_namespace_guard();
DROP FUNCTION IF EXISTS canonical_taxonomy_history_append_only_guard();
DROP FUNCTION IF EXISTS canonical_taxonomy_node_guard();
DROP FUNCTION IF EXISTS canonical_taxonomy_facet_draft_guard();
DROP FUNCTION IF EXISTS canonical_taxonomy_version_guard();