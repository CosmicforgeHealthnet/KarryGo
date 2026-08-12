-- Reverses WP9.1 durable eligibility decisions.

DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('eligibility.decisions.read', 'eligibility.decisions.refresh',
                          'eligibility.policies.read', 'eligibility.policies.manage',
                          'approvals.requests.read', 'approvals.requests.create',
                          'approvals.requests.decide', 'approvals.requests.cancel',
                          'approvals.requests.execute');

DELETE FROM fleet_partner_permissions
WHERE code IN ('eligibility.decisions.read', 'eligibility.decisions.refresh',
               'eligibility.policies.read', 'eligibility.policies.manage',
               'approvals.requests.read', 'approvals.requests.create',
               'approvals.requests.decide', 'approvals.requests.cancel',
               'approvals.requests.execute');

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_history_append_only
    ON fleet_partner_eligibility_decision_history;
DROP TABLE IF EXISTS fleet_partner_eligibility_decision_history;

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_sources_append_only
    ON fleet_partner_eligibility_decision_sources;
DROP TABLE IF EXISTS fleet_partner_eligibility_decision_sources;

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_reasons_append_only
    ON fleet_partner_eligibility_decision_reasons;
DROP TABLE IF EXISTS fleet_partner_eligibility_decision_reasons;

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_decision_immutable
    ON fleet_partner_eligibility_decisions;
DROP TABLE IF EXISTS fleet_partner_eligibility_decisions;
DROP FUNCTION IF EXISTS fleet_partner_eligibility_decision_immutable();

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_policy_immutable
    ON fleet_partner_eligibility_policies;
DROP TABLE IF EXISTS fleet_partner_eligibility_policies;
DROP FUNCTION IF EXISTS fleet_partner_eligibility_policy_immutable();

DROP TABLE IF EXISTS fleet_partner_eligibility_reason_codes;
