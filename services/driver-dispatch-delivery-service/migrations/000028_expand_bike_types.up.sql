ALTER TABLE bikes DROP CONSTRAINT bikes_bike_type_check;
ALTER TABLE bikes ADD CONSTRAINT bikes_bike_type_check
    CHECK (bike_type IN ('motorcycle', 'dispatch_bike', 'scooter', 'tricycle', 'bicycle', 'electric_bike'));
