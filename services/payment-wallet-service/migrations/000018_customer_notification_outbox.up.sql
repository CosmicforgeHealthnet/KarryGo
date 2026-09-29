-- Future Wallet state transitions commit a reference-only notification intent
-- in the same transaction. Existing rows are not replayed on deployment.
CREATE TABLE wallet_notification_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 entity_ref text NOT NULL,
 event_type text NOT NULL,
 recipient_type text NOT NULL,
 recipient_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 available_at timestamptz NOT NULL DEFAULT now(),
 leased_until timestamptz,
 attempts integer NOT NULL DEFAULT 0,
 delivered_at timestamptz,
 dead_at timestamptz,
 UNIQUE(entity_ref,event_type)
);
CREATE INDEX wallet_notification_outbox_pending_idx ON wallet_notification_outbox(available_at,id)
 WHERE delivered_at IS NULL AND dead_at IS NULL;

CREATE FUNCTION wallet_payment_notification_intent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_name text;
BEGIN
 IF NEW.status = OLD.status THEN RETURN NEW; END IF;
 IF NEW.status IN ('held','completed','refunded','partially_refunded') AND OLD.status NOT IN ('held','completed','refunded','partially_refunded') THEN
   IF NEW.metadata->>'kind' = 'wallet_topup' THEN event_name := 'payment.topup_success'; ELSE event_name := 'payment.success'; END IF;
 ELSIF NEW.status = 'failed' THEN
   event_name := 'payment.failed';
 ELSE RETURN NEW; END IF;
 IF NEW.customer_id <> '' THEN
   INSERT INTO wallet_notification_outbox(entity_ref,event_type,recipient_type,recipient_id)
   VALUES(NEW.reference,event_name,'customer',NEW.customer_id) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER payment_notification_intent AFTER UPDATE OF status ON payment_intents
 FOR EACH ROW EXECUTE FUNCTION wallet_payment_notification_intent();

CREATE FUNCTION wallet_refund_notification_intent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE customer_ref text; event_name text;
BEGIN
 IF NEW.status = OLD.status OR NEW.status NOT IN ('processed','failed') THEN RETURN NEW; END IF;
 SELECT customer_id INTO customer_ref FROM payment_intents WHERE id=NEW.payment_intent_id;
 IF customer_ref = '' OR customer_ref IS NULL THEN RETURN NEW; END IF;
 IF NEW.status = 'processed' THEN event_name := 'refund.completed'; ELSE event_name := 'refund.failed'; END IF;
 INSERT INTO wallet_notification_outbox(entity_ref,event_type,recipient_type,recipient_id)
 VALUES(NEW.reference,event_name,'customer',customer_ref) ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER refund_notification_intent AFTER UPDATE OF status ON refunds
 FOR EACH ROW EXECUTE FUNCTION wallet_refund_notification_intent();

CREATE FUNCTION wallet_withdrawal_notification_intent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_name text;
BEGIN
 IF NEW.status = OLD.status OR NEW.provider_id = '' THEN RETURN NEW; END IF;
 CASE NEW.status WHEN 'paid' THEN event_name := 'withdrawal.completed'; WHEN 'failed' THEN event_name := 'withdrawal.failed'; WHEN 'reversed' THEN event_name := 'withdrawal.reversed'; ELSE RETURN NEW; END CASE;
 INSERT INTO wallet_notification_outbox(entity_ref,event_type,recipient_type,recipient_id)
 VALUES(NEW.reference,event_name,'provider',NEW.provider_id) ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER withdrawal_notification_intent AFTER UPDATE OF status ON withdrawals
 FOR EACH ROW EXECUTE FUNCTION wallet_withdrawal_notification_intent();
