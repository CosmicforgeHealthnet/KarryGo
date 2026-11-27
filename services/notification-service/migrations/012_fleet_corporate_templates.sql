-- Fleet corporate notification copy (Sections 23-24). Fleet queues these
-- template keys for organization staff; without them notification-service
-- refused every Fleet intent with "template not found".
--
-- In-app and websocket only: Fleet never sends a recipient email or phone
-- for these, and none of the copy interpolates a name, contact detail,
-- location or amount.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('fleet.escalation.due','en-NG','Escalation needs attention','An escalation in your organization is due. Open the escalation queue to acknowledge it.','["websocket","in_app"]',true),
 ('fleet.communication.message','en-NG','New message','There is a new message in one of your organization''s conversations.','["websocket","in_app"]',true),
 ('fleet.automation.notice','en-NG','Automation update','An approved automation rule in your organization raised a notice.','["websocket","in_app"]',true)
ON CONFLICT (key, locale) DO NOTHING;
