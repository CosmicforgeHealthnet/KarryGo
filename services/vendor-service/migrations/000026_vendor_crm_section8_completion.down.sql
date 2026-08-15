-- Reverses the Section 8 completion schema, leaving Sections 5-8H exactly as
-- they were. The order matters: dependants before their referents, and the
-- additive columns on accepted tables are dropped last so a partial failure
-- cannot leave an 8H-era table carrying a completion-era constraint.

DROP TABLE IF EXISTS crm_task_trigger_evaluations;

ALTER TABLE crm_tasks DROP CONSTRAINT IF EXISTS crm_task_trigger_provenance_ck;
DROP INDEX IF EXISTS crm_tasks_trigger_idx;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS completion_outcome;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS trigger_evaluated_at;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS trigger_source_ref;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS trigger_version;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS trigger_key;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS related_support_case_ref;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS related_rfq_ref;
ALTER TABLE crm_tasks DROP COLUMN IF EXISTS related_order_ref;

ALTER TABLE crm_interactions DROP COLUMN IF EXISTS related_support_case_ref;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS related_rfq_ref;
ALTER TABLE crm_interactions DROP COLUMN IF EXISTS related_order_ref;

DROP TABLE IF EXISTS crm_relationship_signals;
DROP TABLE IF EXISTS crm_customer_value_projections;
DROP TABLE IF EXISTS crm_loyalty_tier_definitions;

DROP TABLE IF EXISTS crm_corporate_rfq_projections;
DROP TABLE IF EXISTS crm_corporate_rfq_facts;
DROP TABLE IF EXISTS crm_corporate_commercial_projections;

DROP TABLE IF EXISTS crm_customer_service_recovery_projections;
DROP TABLE IF EXISTS crm_customer_support_facts;

DROP TABLE IF EXISTS crm_customer_item_affinities;
DROP TABLE IF EXISTS crm_customer_category_affinities;
DROP TABLE IF EXISTS crm_customer_order_fact_lines;

ALTER TABLE vendor_customer_profiles DROP COLUMN IF EXISTS observed_order_count;

DROP INDEX IF EXISTS crm_order_facts_corporate_idx;
DROP INDEX IF EXISTS crm_order_facts_type_idx;
ALTER TABLE crm_customer_order_facts DROP CONSTRAINT IF EXISTS crm_order_fact_corporate_fk;
ALTER TABLE crm_customer_order_facts DROP COLUMN IF EXISTS corporate_customer_id;
ALTER TABLE crm_customer_order_facts DROP COLUMN IF EXISTS order_type;
ALTER TABLE crm_customer_order_facts DROP CONSTRAINT IF EXISTS crm_customer_order_facts_id_business_uq;
