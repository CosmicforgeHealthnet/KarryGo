-- Reverse Section 8 final completion in dependency order.
DROP INDEX IF EXISTS crm_relationship_signals_lapsed_idx;

ALTER TABLE crm_task_trigger_evaluations
    DROP COLUMN IF EXISTS last_evaluated_at,
    DROP COLUMN IF EXISTS resolved_at;
ALTER TABLE crm_task_trigger_evaluations
    DROP CONSTRAINT IF EXISTS crm_task_trigger_status_ck;
ALTER TABLE crm_task_trigger_evaluations
    ADD CONSTRAINT crm_task_trigger_evaluations_status_check
    CHECK (status IN ('FIRED', 'SUPERSEDED', 'CANCELLED'));

ALTER TABLE crm_task_history
    DROP COLUMN IF EXISTS trigger_key,
    DROP COLUMN IF EXISTS related_support_case_ref,
    DROP COLUMN IF EXISTS related_rfq_ref,
    DROP COLUMN IF EXISTS related_order_ref;
ALTER TABLE crm_interaction_revisions
    DROP COLUMN IF EXISTS related_support_case_ref,
    DROP COLUMN IF EXISTS related_rfq_ref,
    DROP COLUMN IF EXISTS related_order_ref;

ALTER TABLE crm_tasks
    DROP CONSTRAINT IF EXISTS crm_task_related_case_ck,
    DROP CONSTRAINT IF EXISTS crm_task_related_rfq_ck,
    DROP CONSTRAINT IF EXISTS crm_task_related_order_ck,
    DROP CONSTRAINT IF EXISTS crm_task_origin_ck;
ALTER TABLE crm_tasks ALTER COLUMN created_by_staff_id SET NOT NULL;
ALTER TABLE crm_interactions
    DROP CONSTRAINT IF EXISTS crm_interaction_related_case_ck,
    DROP CONSTRAINT IF EXISTS crm_interaction_related_rfq_ck,
    DROP CONSTRAINT IF EXISTS crm_interaction_related_order_ck;

DROP TABLE IF EXISTS crm_task_trigger_configs;

ALTER TABLE crm_relationship_signals
    DROP COLUMN IF EXISTS churn_risk_score,
    DROP COLUMN IF EXISTS value_band_rank,
    DROP COLUMN IF EXISTS completed_order_count,
    DROP COLUMN IF EXISTS last_completed_order_at;
DROP TABLE IF EXISTS crm_relationship_signal_configs;

DROP INDEX IF EXISTS crm_value_band_rank_uq;
DROP INDEX IF EXISTS crm_value_band_key_uq;
DROP TABLE IF EXISTS crm_customer_value_band_definitions;
ALTER TABLE crm_customer_value_projections
    DROP CONSTRAINT IF EXISTS crm_customer_value_band_ck,
    DROP CONSTRAINT IF EXISTS crm_customer_value_state_ck,
    DROP CONSTRAINT IF EXISTS crm_customer_value_projections_state_check;
ALTER TABLE crm_customer_value_projections
    ADD CONSTRAINT crm_customer_value_projections_state_check CHECK (state IN ('NOT_AVAILABLE', 'CURRENT', 'STALE', 'FAILED'));
ALTER TABLE crm_customer_value_projections
    ADD CONSTRAINT crm_customer_value_band_ck CHECK ((state = 'CURRENT') = (band IS NOT NULL));

ALTER TABLE crm_corporate_rfq_projections
    DROP COLUMN IF EXISTS latest_rfq_outcome,
    DROP COLUMN IF EXISTS resolved_rfq_count;
ALTER TABLE crm_corporate_commercial_projections
    DROP CONSTRAINT IF EXISTS crm_corporate_commercial_money_ck;
ALTER TABLE crm_corporate_commercial_projections
    DROP COLUMN IF EXISTS last_branch_ref,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS net_spend_minor;
ALTER TABLE crm_corporate_rfq_facts
    DROP CONSTRAINT IF EXISTS crm_rfq_fact_ref_ck,
    DROP CONSTRAINT IF EXISTS crm_rfq_fact_branch_fk;
ALTER TABLE crm_corporate_rfq_facts
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS branch_id;

ALTER TABLE crm_customer_service_recovery_projections
    DROP COLUMN IF EXISTS latest_refund_at,
    DROP COLUMN IF EXISTS latest_refund_ref;
DROP INDEX IF EXISTS crm_support_facts_branch_idx;
ALTER TABLE crm_customer_support_facts
    DROP CONSTRAINT IF EXISTS crm_support_fact_order_ref_ck,
    DROP CONSTRAINT IF EXISTS crm_support_fact_case_ref_ck,
    DROP CONSTRAINT IF EXISTS crm_support_fact_branch_fk;
ALTER TABLE crm_customer_support_facts
    DROP COLUMN IF EXISTS source_refund_ref,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS branch_id;

ALTER TABLE crm_customer_branch_relations DROP COLUMN IF EXISTS observed_order_count;
ALTER TABLE crm_customer_item_affinities DROP COLUMN IF EXISTS net_spend_minor;
ALTER TABLE crm_customer_category_affinities DROP COLUMN IF EXISTS net_spend_minor;
ALTER TABLE crm_customer_order_fact_lines DROP COLUMN IF EXISTS net_line_amount_minor;
