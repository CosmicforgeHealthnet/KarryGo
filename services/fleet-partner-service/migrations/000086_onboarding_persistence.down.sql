-- Refuse to discard legal information once collected.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM fleet_partner_organizations WHERE organization_type <> '') THEN
  RAISE EXCEPTION 'organization_type contains legal data; rollback refused';
 END IF;
END $$;
ALTER TABLE fleet_partner_organizations DROP COLUMN organization_type;
