CREATE TABLE commerce_delivery_funding_allocations (
    id text PRIMARY KEY,
    source_service text NOT NULL,
    source_reference text NOT NULL,
    vendor_order_id text NOT NULL,
    sponsoring_vendor_id text NOT NULL,
    provider_type text NOT NULL,
    provider_id text NOT NULL,
    currency text NOT NULL,
    authoritative_delivery_minor bigint NOT NULL CHECK (authoritative_delivery_minor >= 0),
    customer_contribution_minor bigint NOT NULL CHECK (customer_contribution_minor >= 0),
    vendor_subsidy_minor bigint NOT NULL CHECK (vendor_subsidy_minor >= 0),
    platform_subsidy_minor bigint NOT NULL CHECK (platform_subsidy_minor >= 0),
	platform_approval_ref text NOT NULL DEFAULT '',
    customer_payment_reference text NOT NULL DEFAULT '',
    ledger_transaction_ref text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('APPLIED','REVERSED')),
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (customer_contribution_minor + vendor_subsidy_minor + platform_subsidy_minor = authoritative_delivery_minor),
	CHECK (platform_subsidy_minor = 0 OR platform_approval_ref <> ''),
    UNIQUE (source_service, source_reference),
    UNIQUE (source_service, idempotency_key)
);

CREATE INDEX commerce_delivery_funding_vendor_idx
    ON commerce_delivery_funding_allocations(vendor_order_id, status, created_at);
