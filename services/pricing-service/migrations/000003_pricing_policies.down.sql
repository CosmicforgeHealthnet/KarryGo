-- Reverse of 000003.
DROP TRIGGER IF EXISTS pricing_rollouts_append_only ON pricing_rollouts;
DROP TRIGGER IF EXISTS pricing_market_freezes_append_only ON pricing_market_freezes;
DROP TRIGGER IF EXISTS pricing_policy_activations_append_only ON pricing_policy_activations;
DROP TRIGGER IF EXISTS pricing_approval_decisions_append_only ON pricing_approval_decisions;
DROP TRIGGER IF EXISTS pricing_approval_decisions_no_self_approval ON pricing_approval_decisions;
DROP TRIGGER IF EXISTS pricing_policy_version_events_append_only ON pricing_policy_version_events;
DROP TRIGGER IF EXISTS pricing_policy_versions_guard ON pricing_policy_versions;

DROP TABLE IF EXISTS pricing_rollouts;
DROP TABLE IF EXISTS pricing_market_freezes;
DROP TABLE IF EXISTS pricing_policy_activations;
DROP TABLE IF EXISTS pricing_approval_decisions;
DROP TABLE IF EXISTS pricing_approval_requests;
DROP TABLE IF EXISTS pricing_policy_version_events;
DROP TABLE IF EXISTS pricing_policy_versions;
DROP TABLE IF EXISTS pricing_policies;

DROP FUNCTION IF EXISTS pricing_reject_self_approval();
DROP FUNCTION IF EXISTS pricing_guard_policy_version_mutation();
