-- Preserve company type on the existing tenant root. Empty is an explicitly
-- unknown legacy value, never a fabricated legal classification.
ALTER TABLE fleet_partner_organizations ADD COLUMN organization_type text NOT NULL DEFAULT '';
