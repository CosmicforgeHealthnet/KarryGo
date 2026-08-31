-- Commerce support/dispute authority: typed opaque context, idempotent source
-- operations, versioned resolutions, and a durable transactional outbox.
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS source_schema_version integer;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS request_fingerprint text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS commerce_category text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS commerce_context jsonb;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS resolution_type text;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS resolution_version bigint NOT NULL DEFAULT 0;
ALTER TABLE complaints ADD COLUMN IF NOT EXISTS financial_action_ref text;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='complaints_commerce_category_chk') THEN
ALTER TABLE complaints ADD CONSTRAINT complaints_commerce_category_chk CHECK (
  commerce_category IS NULL OR commerce_category IN (
    'WRONG_ITEM_OR_MEAL','MISSING_ITEM','UNAUTHORIZED_SUBSTITUTION',
    'DAMAGED_OR_SPOILT_PRODUCT','QUALITY_ISSUE','VENDOR_DELAY','DELIVERY_DELAY',
    'PICKUP_DISPUTE','RECEIVER_HANDOVER_DISPUTE','REFUND_PROBLEM',
    'VENDOR_OWNED_DELIVERY_ISSUE'
  )
); END IF; END $$;
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='complaints_commerce_context_chk') THEN
ALTER TABLE complaints ADD CONSTRAINT complaints_commerce_context_chk CHECK (
  commerce_context IS NULL OR jsonb_typeof(commerce_context) = 'object'
); END IF; END $$;
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='complaints_commerce_fingerprint_chk') THEN
ALTER TABLE complaints ADD CONSTRAINT complaints_commerce_fingerprint_chk CHECK (
  request_fingerprint IS NULL OR length(request_fingerprint) = 64
); END IF; END $$;
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='complaints_resolution_version_chk') THEN
ALTER TABLE complaints ADD CONSTRAINT complaints_resolution_version_chk CHECK (resolution_version >= 0); END IF; END $$;

CREATE TABLE IF NOT EXISTS support_case_outbox (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type text NOT NULL CHECK (event_type = 'commerce.support_case.resolved'),
  aggregate_ref uuid NOT NULL REFERENCES complaints(id) ON DELETE RESTRICT,
  aggregate_version bigint NOT NULL CHECK (aggregate_version > 0),
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','DEAD_LETTER')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  UNIQUE(event_type, aggregate_ref, aggregate_version)
);
CREATE INDEX IF NOT EXISTS support_case_outbox_pending_idx ON support_case_outbox(next_attempt_at, created_at) WHERE status='PENDING';

COMMENT ON COLUMN complaints.commerce_context IS 'Opaque operational references only; no profile, address, payment, OTP/QR material, or media bytes.';
