ALTER TABLE dispatch_route_location_fixes ADD COLUMN device_command_ref UUID NULL REFERENCES dispatch_device_commands(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX dispatch_route_fix_device_command ON dispatch_route_location_fixes(device_command_ref) WHERE device_command_ref IS NOT NULL;
CREATE INDEX dispatch_route_fix_observed_history ON dispatch_route_location_fixes(trip_id, LEAST(client_recorded_at,received_at) DESC, id DESC);
