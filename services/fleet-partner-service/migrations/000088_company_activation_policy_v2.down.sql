-- Policy versions are immutable and cannot be deleted: archive version 2 and
-- make version 1 active again.
UPDATE fleet_partner_eligibility_policies
   SET status = 'ARCHIVED', archived_at = now()
 WHERE id = 'elp_default_company_activation_v2';

UPDATE fleet_partner_eligibility_policies
   SET status = 'ACTIVE', archived_at = NULL
 WHERE id = 'elp_default_company_activation';
