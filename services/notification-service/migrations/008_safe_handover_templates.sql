-- Handover notifications are retrieval hints. Credentials are never rendered
-- into message bodies or stored in Notification Service.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('booking.delivery_code_ready','en-NG','Delivery credential ready','A delivery credential is ready. Open the secure receiver view to retrieve it.','["push","websocket","in_app","email"]',true),
 ('package.delivery_code_ready','en-NG','Delivery credential ready','A delivery credential is ready. Open the secure receiver view to retrieve it.','["push","websocket","in_app","email"]',true),
 ('taxi.goods.pickup_credential_ready','en-NG','Pickup credential ready','A pickup credential is ready. Open the secure provider view to retrieve it.','["push","websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET title=EXCLUDED.title,body=EXCLUDED.body,default_channels=EXCLUDED.default_channels,active=EXCLUDED.active,updated_at=now();
