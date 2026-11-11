-- Retain purpose history and never resurrect legacy challenges.
DROP INDEX IF EXISTS uq_fleet_partner_identity_email_lower;
