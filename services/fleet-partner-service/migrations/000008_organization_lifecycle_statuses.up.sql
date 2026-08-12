-- Extend the organization status domain for the Section 8 lifecycle.
--
-- Migration 000002 (Sections 3–7) constrained status to
-- ('PENDING_REVIEW','ACTIVE','SUSPENDED','ARCHIVED'). Section 8 introduces
-- the full onboarding lifecycle:
--
--   DRAFT -> CONTACT_VERIFIED -> BUSINESS_SUBMITTED -> UNDER_REVIEW -> ACTIVE
--   UNDER_REVIEW -> CHANGES_REQUIRED -> BUSINESS_SUBMITTED
--   ACTIVE <-> SUSPENDED ; ACTIVE|SUSPENDED -> CLOSED
--
-- This migration is additive: it replaces the CHECK constraint rather than
-- editing 000002, which may already be applied elsewhere. PENDING_REVIEW is
-- retained so existing rows stay valid and are treated as DRAFT by the
-- state machine.

ALTER TABLE fleet_partner_organizations
    DROP CONSTRAINT IF EXISTS fleet_partner_organizations_status_check;

ALTER TABLE fleet_partner_organizations
    ADD CONSTRAINT fleet_partner_organizations_status_check
    CHECK (status IN (
        'DRAFT',
        'PENDING_REVIEW',
        'CONTACT_VERIFIED',
        'BUSINESS_SUBMITTED',
        'UNDER_REVIEW',
        'CHANGES_REQUIRED',
        'ACTIVE',
        'SUSPENDED',
        'CLOSED',
        'ARCHIVED'
    ));
