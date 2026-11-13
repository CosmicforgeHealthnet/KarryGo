-- SECTION30-CLEANUP-RELEASE: data-only policy version switch; version 1 is archived (never deleted) and the down migration reactivates it. No schema change.
-- Company activation policy, version 2.
--
-- Version 1 could never authorize an activation:
--   * its "lifecycle" check required the company to be ACTIVE already, and
--     activation is the move to ACTIVE (the check now accepts UNDER_REVIEW
--     for this operation only, so version 2 keeps it);
--   * its "approval" check requires every onboarding stage to be complete,
--     including the ACTIVATION stage itself;
--   * it required REGISTRATION_CERTIFICATE and TAX_IDENTIFICATION, document
--     types onboarding never collects.
-- The activation evaluator already checks the onboarding stages needed for
-- submission, so version 2 drops "approval" and keeps lifecycle,
-- verification, restrictions and the documents onboarding actually collects. Policy versions are immutable; version 1 is
-- archived, not edited.
UPDATE fleet_partner_eligibility_policies
   SET status = 'ARCHIVED', archived_at = now()
 WHERE organization_id IS NULL AND policy_key = 'COMPANY_ACTIVATION' AND status = 'ACTIVE'
   AND id <> 'elp_default_company_activation_v2';

INSERT INTO fleet_partner_eligibility_policies
    (id, organization_id, policy_key, operation_type, version,
     required_company_checks, required_provider_checks, required_vehicle_checks,
     required_documents, decision_ttl_seconds, source_max_age_seconds,
     approval_required, minimum_approvals, eligible_role_ids,
     self_approval_allowed, separation_of_duties, approval_ttl_seconds,
     acknowledgement_required, subscription_required,
     financial_threshold_minor, financial_threshold_currency)
VALUES
    ('elp_default_company_activation_v2', NULL, 'COMPANY_ACTIVATION', 'COMPANY_ACTIVATION', 2,
     '["lifecycle","verification","restrictions","documents"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb,
     '["CERTIFICATE_OF_INCORPORATION","TAX_CLEARANCE","INSURANCE_POLICY","OPERATING_LICENSE"]'::jsonb,
     900, 900, true, 1, '["rol_system_owner","rol_system_admin"]'::jsonb, false, true, 86400, false, false, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

UPDATE fleet_partner_eligibility_policies
   SET status = 'ACTIVE', archived_at = NULL
 WHERE id = 'elp_default_company_activation_v2';
