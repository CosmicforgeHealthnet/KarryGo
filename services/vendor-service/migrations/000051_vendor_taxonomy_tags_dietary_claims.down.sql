DROP TRIGGER IF EXISTS
    canonical_taxonomy_semantic_history_guard
ON canonical_taxonomy_semantic_history;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_semantic_history_append_only();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_dietary_assignment_history
ON canonical_taxonomy_dietary_claim_assignments;

DROP TRIGGER IF EXISTS
    canonical_taxonomy_dietary_claim_history
ON canonical_taxonomy_dietary_claims;

DROP TRIGGER IF EXISTS
    canonical_taxonomy_tag_assignment_history
ON canonical_taxonomy_tag_assignments;

DROP TRIGGER IF EXISTS
    canonical_taxonomy_tag_history
ON canonical_taxonomy_tags;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_semantic_history_record();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_dietary_assignment_guard
ON canonical_taxonomy_dietary_claim_assignments;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_validate_dietary_assignment();


DROP TRIGGER IF EXISTS
    canonical_taxonomy_tag_assignment_guard
ON canonical_taxonomy_tag_assignments;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_validate_tag_assignment();


DROP TABLE IF EXISTS canonical_taxonomy_semantic_history;

DROP TABLE IF EXISTS
    canonical_taxonomy_dietary_claim_assignments;

DROP TABLE IF EXISTS
    canonical_taxonomy_dietary_claims;

DROP TABLE IF EXISTS
    canonical_taxonomy_tag_assignments;

DROP TABLE IF EXISTS
    canonical_taxonomy_tags;