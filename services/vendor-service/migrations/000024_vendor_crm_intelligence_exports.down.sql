-- Rollback of Section 8H's schema.
--
-- Section 8H owns every table below outright and changes no accepted table, so
-- the rollback is a straight drop in foreign-key order. Scores reference both
-- evaluations and profiles; scope states reference evaluations; nothing outside
-- 8H references any of them.
DROP TABLE IF EXISTS crm_exports;
DROP TABLE IF EXISTS crm_customer_intelligence_scores;
DROP TABLE IF EXISTS crm_intelligence_scope_states;
DROP TABLE IF EXISTS crm_intelligence_evaluations;
