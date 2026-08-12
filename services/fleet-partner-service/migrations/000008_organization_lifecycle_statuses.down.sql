-- Restore the Sections 3–7 status domain.
--
-- Rows carrying a Section 8 lifecycle status are first mapped back to the
-- nearest legacy status so the narrower constraint can be re-applied without
-- data loss: onboarding states collapse to PENDING_REVIEW, and CLOSED
-- collapses to ARCHIVED.

UPDATE fleet_partner_organizations
SET status = 'PENDING_REVIEW'
WHERE status IN ('DRAFT', 'CONTACT_VERIFIED', 'BUSINESS_SUBMITTED', 'UNDER_REVIEW', 'CHANGES_REQUIRED');

UPDATE fleet_partner_organizations
SET status = 'ARCHIVED'
WHERE status = 'CLOSED';

ALTER TABLE fleet_partner_organizations
    DROP CONSTRAINT IF EXISTS fleet_partner_organizations_status_check;

ALTER TABLE fleet_partner_organizations
    ADD CONSTRAINT fleet_partner_organizations_status_check
    CHECK (status IN ('PENDING_REVIEW', 'ACTIVE', 'SUSPENDED', 'ARCHIVED'));
