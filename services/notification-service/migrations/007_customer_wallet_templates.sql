-- Production financial hints use no amounts or account details in title/body.
INSERT INTO notification_templates(key, locale, title, body, default_channels, active)
VALUES
 ('payment.topup_success','en-NG','Wallet funded','Your wallet top-up completed. Open Wallet for the current receipt.','["push","websocket","in_app"]',true),
 ('payment.success','en-NG','Payment completed','Your payment completed. Open Wallet for the current receipt.','["push","websocket","in_app"]',true),
 ('payment.failed','en-NG','Payment update','A payment did not complete. Open Wallet for its current status.','["push","websocket","in_app"]',true),
 ('refund.completed','en-NG','Refund completed','A refund completed. Open Wallet for its current status.','["push","websocket","in_app"]',true),
 ('refund.failed','en-NG','Refund update','A refund could not complete. Open Wallet for its current status.','["push","websocket","in_app"]',true),
 ('withdrawal.completed','en-NG','Withdrawal completed','Your withdrawal completed. Open Wallet for its current status.','["push","websocket","in_app"]',true),
 ('withdrawal.failed','en-NG','Withdrawal update','Your withdrawal did not complete. Open Wallet for its current status.','["push","websocket","in_app"]',true),
 ('withdrawal.reversed','en-NG','Withdrawal reversed','Your withdrawal was reversed. Open Wallet for its current status.','["push","websocket","in_app"]',true)
ON CONFLICT (key, locale) DO UPDATE SET title=EXCLUDED.title,body=EXCLUDED.body,default_channels=EXCLUDED.default_channels,active=EXCLUDED.active,updated_at=now();
