-- FLEET-13.11: carry the execution service's payment reference to Wallet.
--
-- A settlement is named by the execution service and its own job id. The
-- job's payment in payment-wallet was created earlier by that service, and
-- Dispatch creates it per BOOKING while it completes (and reports to Fleet)
-- per TRIP. Fleet cannot derive one from the other, so the execution service
-- states the payment reference on the completed-work record and Fleet copies
-- it, unchanged, into the immutable instruction it delivers. Empty means the
-- payment is keyed by the job id itself (Taxi, Hauling, and every record made
-- before this column existed).
ALTER TABLE fleet_partner_completed_work_records
    ADD COLUMN IF NOT EXISTS payment_source_reference text NOT NULL DEFAULT '';

ALTER TABLE fleet_partner_managed_work_settlement_instructions
    ADD COLUMN IF NOT EXISTS payment_source_reference text NOT NULL DEFAULT '';
