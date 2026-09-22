-- Section 14: copy for the network intents (shared/go/notifications/network.go).
--
-- The producing services name the fact and the audience; the words live
-- here. None of these templates interpolates a location, a contact detail or
-- an amount -- the requests that fill them are refused if they carry one.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('network.customer.recovery_started','en-NG','We are arranging another driver','Your driver cannot complete this trip. We are finding a replacement now.','["push","websocket","in_app"]',true),
 ('network.customer.provider_replaced','en-NG','Your new driver is on the way','A replacement driver has taken over your trip.','["push","websocket","in_app"]',true),
 ('network.customer.recovery_failed','en-NG','We could not complete your trip','We could not find a replacement. Any payment held for this trip will be handled by our team.','["push","websocket","in_app"]',true),
 ('network.customer.guarantee_breached','en-NG','Sorry for the delay','Your trip did not go as promised. We are working to put it right.','["push","websocket","in_app"]',true),
 ('network.customer.support_update','en-NG','Update on your support case','There is an update on your support case.','["push","websocket","in_app"]',true),
 ('network.provider.job_reassigned','en-NG','Job reassigned','This job has been reassigned to another provider. You no longer need to complete it.','["push","websocket","in_app"]',true),
 ('network.provider.replacement_assigned','en-NG','Job in progress assigned','You have been assigned a job another provider could not finish.','["push","websocket","in_app"]',true),
 ('network.provider.support_update','en-NG','Update on your support case','There is an update on your support case.','["push","websocket","in_app"]',true),
 ('network.vendor.delivery_recovery','en-NG','Delivery being reassigned','A delivery for one of your orders is being reassigned to another provider.','["push","websocket","in_app"]',true),
 ('network.fleet.managed_provider_replaced','en-NG','Managed job reassigned','A job held by one of your providers was reassigned during recovery.','["websocket","in_app"]',true),
 ('network.fleet.managed_recovery_started','en-NG','Managed job in recovery','A job held by one of your providers has entered recovery.','["websocket","in_app"]',true),
 ('network.admin.recovery_operator_required','en-NG','Recovery needs an operator','A recovery found no compatible replacement and is waiting for an operator.','["websocket","in_app"]',true),
 ('network.admin.support_action_unresolved','en-NG','Support action unresolved','A support action could not be confirmed with its owning service.','["websocket","in_app"]',true),
 ('network.admin.provider_safety_decline','en-NG','Provider safety decline','A provider declined work for a safety reason. Review the vehicle or route before it is offered again.','["websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET
    title=EXCLUDED.title, body=EXCLUDED.body,
    default_channels=EXCLUDED.default_channels, active=true, updated_at=now();
