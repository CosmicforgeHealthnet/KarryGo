-- Customer Ride milestones are hints to read the Taxi booking authority.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('taxi.booking.accepted','en-NG','Driver assigned','A driver accepted your ride.','["push","websocket","in_app"]',true),
 ('taxi.booking.en_route_pickup','en-NG','Driver on the way','Your driver is heading to the pickup.','["push","websocket","in_app"]',true),
 ('taxi.booking.arrived_at_pickup','en-NG','Driver arrived','Your driver has arrived at the pickup.','["push","websocket","in_app"]',true),
 ('taxi.booking.started','en-NG','Ride started','Your ride is underway.','["push","websocket","in_app"]',true),
 ('taxi.booking.completed','en-NG','Ride completed','Your ride is complete.','["push","websocket","in_app"]',true),
 ('taxi.booking.auto_completed','en-NG','Ride completed','Your ride is complete.','["push","websocket","in_app"]',true),
 ('taxi.booking.cancelled','en-NG','Ride cancelled','Your ride was cancelled.','["push","websocket","in_app"]',true),
 ('taxi.booking.admin_cancelled','en-NG','Ride cancelled','Your ride was cancelled.','["push","websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET
 title=EXCLUDED.title, body=EXCLUDED.body,
 default_channels=EXCLUDED.default_channels, active=true, updated_at=now();
