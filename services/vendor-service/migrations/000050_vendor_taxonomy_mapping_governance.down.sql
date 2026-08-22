DROP TRIGGER IF EXISTS
    canonical_taxonomy_governance_history_guard
ON canonical_taxonomy_governance_history;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_governance_history_append_only();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_governance_case_guard_trigger
ON canonical_taxonomy_governance_cases;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_governance_case_guard();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_mapping_history_guard
ON canonical_taxonomy_mapping_history;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_mapping_history_append_only();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_mapping_history_trigger
ON canonical_taxonomy_mappings;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_record_mapping_history();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_mapping_validate
ON canonical_taxonomy_mappings;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_validate_mapping();


DROP TABLE IF EXISTS canonical_taxonomy_governance_history;
DROP TABLE IF EXISTS canonical_taxonomy_governance_cases;

DROP TABLE IF EXISTS canonical_taxonomy_mapping_commands;
DROP TABLE IF EXISTS canonical_taxonomy_mapping_history;
DROP TABLE IF EXISTS canonical_taxonomy_mappings;