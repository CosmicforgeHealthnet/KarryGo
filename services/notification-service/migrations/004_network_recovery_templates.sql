-- Section 14 closure: copy for the recovery intents Taxi (and Hauling) now
-- send. Additive to 003.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('network.provider.recovery_transfer_required','en-NG','Cargo handover required','This job continues with another provider. Hand over the cargo before the job can continue.','["push","websocket","in_app"]',true),
 ('network.admin.recovery_failed','en-NG','Recovery failed','A recovery ended without completing the job.','["websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET
    title=EXCLUDED.title, body=EXCLUDED.body,
    default_channels=EXCLUDED.default_channels, active=true, updated_at=now();
