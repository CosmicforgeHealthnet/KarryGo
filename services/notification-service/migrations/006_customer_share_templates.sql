INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('share.created','en-NG','Journey sharing enabled','Your journey share is ready. Open Share Journey to see its current status.','["push","websocket","in_app"]',true),
 ('share.revoked','en-NG','Journey sharing stopped','Your journey share was revoked. Open Share Journey to see its current status.','["push","websocket","in_app"]',true),
 ('share.expired','en-NG','Journey share expired','Your journey share expired. Open Share Journey to see its current status.','["push","websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET title=EXCLUDED.title,body=EXCLUDED.body,default_channels=EXCLUDED.default_channels,active=EXCLUDED.active,updated_at=now();
