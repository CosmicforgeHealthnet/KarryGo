DROP INDEX IF EXISTS dispatch_route_fix_observed_history;
DROP INDEX IF EXISTS dispatch_route_fix_device_command;
ALTER TABLE dispatch_route_location_fixes DROP COLUMN IF EXISTS device_command_ref;
