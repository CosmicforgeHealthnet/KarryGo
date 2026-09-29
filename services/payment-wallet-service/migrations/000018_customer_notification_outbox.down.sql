DROP TRIGGER IF EXISTS withdrawal_notification_intent ON withdrawals;
DROP FUNCTION IF EXISTS wallet_withdrawal_notification_intent();
DROP TRIGGER IF EXISTS refund_notification_intent ON refunds;
DROP FUNCTION IF EXISTS wallet_refund_notification_intent();
DROP TRIGGER IF EXISTS payment_notification_intent ON payment_intents;
DROP FUNCTION IF EXISTS wallet_payment_notification_intent();
DROP TABLE IF EXISTS wallet_notification_outbox;
