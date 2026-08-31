-- The Section 8C additions are reversed first: the corporate columns carry
-- foreign keys into the tables this migration is about to drop, and restoring
-- NOT NULL requires no corporate-subject rows to remain.
DELETE FROM crm_task_history
WHERE task_id IN (SELECT id FROM crm_tasks WHERE corporate_customer_id IS NOT NULL);
DELETE FROM crm_tasks WHERE corporate_customer_id IS NOT NULL;
DELETE FROM crm_interaction_revisions
WHERE interaction_id IN (SELECT id FROM crm_interactions WHERE corporate_customer_id IS NOT NULL);
DELETE FROM crm_interactions WHERE corporate_customer_id IS NOT NULL;

ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_corporate_source_interaction_fk;
ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_corporate_cost_center_fk;
ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_corporate_location_fk;
ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_corporate_contact_fk;
ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_corporate_fk;
ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_subject_ck;
DROP INDEX IF EXISTS crm_tasks_corporate_status_idx;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS corporate_cost_center_id;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS corporate_location_id;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS corporate_contact_id;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS corporate_customer_id;
ALTER TABLE crm_tasks ALTER COLUMN original_profile_id SET NOT NULL;
ALTER TABLE crm_tasks ALTER COLUMN customer_profile_id SET NOT NULL;

ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interactions_corporate_subject_uq;
ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interaction_corporate_cost_center_fk;
ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interaction_corporate_location_fk;
ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interaction_corporate_contact_fk;
ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interaction_corporate_fk;
ALTER TABLE crm_interactions DROP CONSTRAINT IF EXISTS crm_interaction_subject_ck;
DROP INDEX IF EXISTS crm_interactions_corporate_timeline_idx;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS corporate_cost_center_id;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS corporate_location_id;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS corporate_contact_id;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS corporate_customer_id;
ALTER TABLE crm_interactions ALTER COLUMN original_profile_id SET NOT NULL;
ALTER TABLE crm_interactions ALTER COLUMN customer_profile_id SET NOT NULL;

DROP TABLE IF EXISTS corporate_customer_memberships;
DROP TABLE IF EXISTS corporate_customer_contacts;
DROP TABLE IF EXISTS corporate_customer_cost_centers;
DROP TABLE IF EXISTS corporate_customer_locations;
DROP TABLE IF EXISTS corporate_customer_branch_scopes;
DROP TABLE IF EXISTS corporate_customer_profiles;
DROP TABLE IF EXISTS corporate_customer_code_counters;
