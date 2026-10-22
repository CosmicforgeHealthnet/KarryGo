-- Investigation cases and their history are decision evidence. Rollback is
-- refused once any case exists; empty tables can be removed.
DO $$ BEGIN
 IF to_regclass('fleet_partner_admin_investigations') IS NOT NULL AND EXISTS(SELECT 1 FROM fleet_partner_admin_investigations) THEN
  RAISE EXCEPTION 'Fleet investigation evidence requires an approved forward retention migration';
 END IF;
END $$;
DROP TABLE IF EXISTS fleet_partner_admin_investigation_history;
DROP TABLE IF EXISTS fleet_partner_admin_investigations;
DROP FUNCTION IF EXISTS reject_fleet_partner_admin_investigation_mutation();
