-- Booking payment method + payment status.
--
-- payment_method: how the customer pays for the trip.
--   'wallet' — held from the customer wallet balance when the provider accepts.
--   'card'   — paid up-front via a Paystack payment intent created at booking time.
--   'cash'   — record-only; the provider collects cash on delivery.
-- payment_status mirrors the payment-wallet-service payment intent lifecycle for
-- this booking so the hauling service can answer "is this trip paid?" without a
-- round-trip on every read.
ALTER TABLE haulage_bookings
  ADD COLUMN IF NOT EXISTS payment_method TEXT   NOT NULL DEFAULT 'wallet',
  ADD COLUMN IF NOT EXISTS payment_status TEXT   NOT NULL DEFAULT 'unpaid';
