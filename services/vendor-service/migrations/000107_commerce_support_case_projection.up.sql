CREATE TABLE commerce_support_case_links (
  id text PRIMARY KEY,
  operation_ref text NOT NULL CHECK (length(btrim(operation_ref)) BETWEEN 1 AND 160),
  request_fingerprint text NOT NULL CHECK (length(request_fingerprint)=64),
  support_case_ref text,
  category text NOT NULL CHECK (category IN (
    'WRONG_ITEM_OR_MEAL','MISSING_ITEM','UNAUTHORIZED_SUBSTITUTION',
    'DAMAGED_OR_SPOILT_PRODUCT','QUALITY_ISSUE','VENDOR_DELAY','DELIVERY_DELAY',
    'PICKUP_DISPUTE','RECEIVER_HANDOVER_DISPUTE','REFUND_PROBLEM',
    'VENDOR_OWNED_DELIVERY_ISSUE')),
  parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
  vendor_order_id text NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
  customer_id text NOT NULL,
  business_id text NOT NULL,
  support_status text NOT NULL DEFAULT 'CREATE_PENDING',
  resolution text NOT NULL DEFAULT '',
  resolution_version bigint NOT NULL DEFAULT 0 CHECK (resolution_version >= 0),
  financial_action_ref text NOT NULL DEFAULT '',
  refund_status text NOT NULL DEFAULT '',
  deterministic_policy_ref text NOT NULL DEFAULT '',
  source_event_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(customer_id, operation_ref),
  UNIQUE(support_case_ref)
);
CREATE INDEX commerce_support_case_order_idx ON commerce_support_case_links(parent_order_id, created_at DESC);
CREATE INDEX commerce_support_case_vendor_idx ON commerce_support_case_links(business_id, vendor_order_id, created_at DESC);

CREATE TABLE commerce_support_resolution_inbox (
  event_id text PRIMARY KEY,
  support_case_ref text NOT NULL,
  resolution_version bigint NOT NULL CHECK (resolution_version > 0),
  payload_hash text NOT NULL CHECK (length(payload_hash)=64),
  processed_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE commerce_support_case_links IS 'Safe Vendor/customer projection of Support-owned cases; Support remains adjudication authority and Wallet remains ledger authority.';
