-- SECTION30-CLEANUP-RELEASE: CHECK widening only; existing purposes retained, no rows/identity tables dropped. Rollback removes only the added email expression index and never resurrects superseded challenges.
-- Preserve identities/memberships; fail rather than merge legacy collisions.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_identity_email_lower
 ON fleet_partner_corporate_identities (lower(email_normalized)) WHERE status <> 'ARCHIVED';
ALTER TABLE fleet_partner_auth_otp_challenges DROP CONSTRAINT IF EXISTS fleet_partner_auth_otp_challenges_purpose_check;
ALTER TABLE fleet_partner_auth_otp_challenges ADD CONSTRAINT fleet_partner_auth_otp_challenges_purpose_check
 CHECK (purpose IN ('SIGN_UP_OR_LOGIN','SIGN_UP','SIGN_IN','STAFF_INVITATION','ORGANIZATION_SELECTION','STEP_UP','CONTACT_CHANGE','PROVIDER_INVITATION'));
UPDATE fleet_partner_auth_otp_challenges SET status='SUPERSEDED',updated_at=now()
 WHERE status IN ('PENDING','EXPIRED');
