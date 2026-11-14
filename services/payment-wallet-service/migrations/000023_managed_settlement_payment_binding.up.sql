-- FLEET-13.11: bind each managed settlement to the payment it released.
--
-- A managed settlement is named canonically -- TAXI, DISPATCH or HAULING plus
-- the execution service's own job id -- because Fleet, Wallet and the
-- execution service must all name it the same way. The payment intent it
-- releases was created earlier by the execution service under its own
-- provisioned name ("taxi-service", "dispatch-delivery-service",
-- "driver-hauling-service") and, for Dispatch, keyed by the booking rather
-- than the trip. The two identities therefore differ, so the settlement row
-- records the payment it actually resolved to instead of leaving every later
-- reader (reversal, the legacy-payout guard, the stuck-job sweep) to
-- re-derive it.
ALTER TABLE managed_settlements
    ADD COLUMN IF NOT EXISTS payment_intent_id uuid REFERENCES payment_intents(id),
    ADD COLUMN IF NOT EXISTS payment_source_service text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payment_source_reference text NOT NULL DEFAULT '';

-- Settlements that posted before these columns existed resolved their payment
-- by an exact match on the settlement identity, so that same match recovers
-- it.
UPDATE managed_settlements s
SET payment_intent_id = i.id,
    payment_source_service = i.source_service,
    payment_source_reference = i.source_reference
FROM payment_intents i
WHERE s.payment_intent_id IS NULL
  AND s.status IN ('POSTED', 'REVERSED')
  AND i.source_service = s.source_service
  AND i.source_reference = s.source_job_id;

-- One standing settlement per payment. Two canonical identities that resolve
-- to the same payment (two trips of one Dispatch booking, say) must never both
-- post: the held-status check already refuses the second, and this index makes
-- that structural.
CREATE UNIQUE INDEX IF NOT EXISTS uq_managed_settlements_posted_payment
    ON managed_settlements (payment_intent_id)
    WHERE status = 'POSTED' AND payment_intent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_managed_settlements_payment_intent
    ON managed_settlements (payment_intent_id)
    WHERE payment_intent_id IS NOT NULL;
