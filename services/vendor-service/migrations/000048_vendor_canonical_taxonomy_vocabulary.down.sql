DROP TRIGGER IF EXISTS canonical_taxonomy_term_history_append_only ON canonical_taxonomy_term_history;
DROP FUNCTION IF EXISTS canonical_taxonomy_prevent_term_history_mutation();

DROP TRIGGER IF EXISTS canonical_taxonomy_node_terms_history ON canonical_taxonomy_node_terms;
DROP FUNCTION IF EXISTS canonical_taxonomy_record_term_history();

DROP TRIGGER IF EXISTS canonical_taxonomy_node_terms_validate ON canonical_taxonomy_node_terms;
DROP FUNCTION IF EXISTS canonical_taxonomy_validate_node_term();

DROP TABLE IF EXISTS canonical_taxonomy_term_history;
DROP TABLE IF EXISTS canonical_taxonomy_node_terms;
