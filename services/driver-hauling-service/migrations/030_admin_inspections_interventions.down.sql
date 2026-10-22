-- Inspection, evidence-association, legacy-mapping and suitability records are
-- administrative evidence; they may be removed only while empty.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hauling_admin_inspections)
 OR EXISTS(SELECT 1 FROM hauling_cargo_suitability_decisions)
 OR EXISTS(SELECT 1 FROM hauling_cargo_suitability_history)
 OR EXISTS(SELECT 1 FROM hauling_admin_intervention_heads)
 THEN RAISE EXCEPTION 'Hauling administrative intervention evidence requires approved forward retention migration'; END IF;
END $$;
DROP TABLE IF EXISTS hauling_cargo_suitability_history;
DROP TABLE IF EXISTS hauling_cargo_suitability_decisions;
DROP TABLE IF EXISTS hauling_admin_legacy_inspection_mappings;
DROP TABLE IF EXISTS hauling_admin_inspection_evidence;
DROP TABLE IF EXISTS hauling_admin_inspection_history;
DROP TABLE IF EXISTS hauling_admin_inspections;
DROP TABLE IF EXISTS hauling_admin_intervention_heads;
DROP FUNCTION IF EXISTS hauling_guard_admin_inspection();
DROP FUNCTION IF EXISTS hauling_reject_admin_intervention_mutation();
