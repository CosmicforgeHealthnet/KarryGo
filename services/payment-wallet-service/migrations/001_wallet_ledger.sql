CREATE TABLE IF NOT EXISTS wallet_accounts (
    id uuid PRIMARY KEY,
    owner_type text NOT NULL,
    owner_id text NOT NULL,
    account_type text NOT NULL,
    currency text NOT NULL DEFAULT 'NGN',
    normal_balance text NOT NULL CHECK (normal_balance IN ('debit', 'credit')),
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_type, owner_id, account_type, currency)
);

CREATE INDEX IF NOT EXISTS idx_wallet_accounts_owner ON wallet_accounts(owner_type, owner_id);

CREATE TABLE IF NOT EXISTS ledger_transactions (
    id uuid PRIMARY KEY,
    reference text NOT NULL UNIQUE,
    transaction_type text NOT NULL,
    status text NOT NULL DEFAULT 'posted',
    source_service text NOT NULL DEFAULT '',
    source_reference text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL DEFAULT '',
    external_reference text NOT NULL DEFAULT '',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_transactions_source
    ON ledger_transactions(source_service, source_reference, transaction_type)
    WHERE source_service <> '' AND source_reference <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_transactions_idempotency
    ON ledger_transactions(idempotency_key, transaction_type)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS ledger_entries (
    id bigserial PRIMARY KEY,
    transaction_id uuid NOT NULL REFERENCES ledger_transactions(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES wallet_accounts(id),
    side text NOT NULL CHECK (side IN ('debit', 'credit')),
    amount_kobo bigint NOT NULL CHECK (amount_kobo > 0),
    currency text NOT NULL DEFAULT 'NGN',
    memo text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ledger_entries_account_id ON ledger_entries(account_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_transaction_id ON ledger_entries(transaction_id);

CREATE TABLE IF NOT EXISTS payment_intents (
    id uuid PRIMARY KEY,
    reference text NOT NULL UNIQUE,
    source_service text NOT NULL,
    source_reference text NOT NULL,
    customer_id text NOT NULL,
    customer_email text NOT NULL DEFAULT '',
    provider_id text NOT NULL DEFAULT '',
    provider_type text NOT NULL DEFAULT '',
    amount_kobo bigint NOT NULL CHECK (amount_kobo > 0),
    platform_fee_kobo bigint NOT NULL DEFAULT 0 CHECK (platform_fee_kobo >= 0),
    currency text NOT NULL DEFAULT 'NGN',
    payment_method text NOT NULL,
    status text NOT NULL,
    paystack_reference text UNIQUE,
    authorization_url text,
    access_code text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_service, source_reference)
);

CREATE INDEX IF NOT EXISTS idx_payment_intents_customer_id ON payment_intents(customer_id);
CREATE INDEX IF NOT EXISTS idx_payment_intents_provider_id ON payment_intents(provider_id);

CREATE TABLE IF NOT EXISTS paystack_webhook_events (
    id uuid PRIMARY KEY,
    event_key text NOT NULL UNIQUE,
    event_type text NOT NULL,
    reference text NOT NULL DEFAULT '',
    payload jsonb NOT NULL,
    processed_at timestamptz,
    processing_error text,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_paystack_webhook_events_reference ON paystack_webhook_events(reference);

CREATE TABLE IF NOT EXISTS provider_bank_accounts (
    id uuid PRIMARY KEY,
    provider_type text NOT NULL,
    provider_id text NOT NULL,
    bank_code text NOT NULL,
    bank_name text NOT NULL DEFAULT '',
    account_number text NOT NULL,
    account_name text NOT NULL,
    recipient_code text NOT NULL UNIQUE,
    currency text NOT NULL DEFAULT 'NGN',
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_type, provider_id, bank_code, account_number)
);

CREATE INDEX IF NOT EXISTS idx_provider_bank_accounts_provider ON provider_bank_accounts(provider_type, provider_id);

CREATE TABLE IF NOT EXISTS withdrawals (
    id uuid PRIMARY KEY,
    reference text NOT NULL UNIQUE,
    provider_type text NOT NULL,
    provider_id text NOT NULL,
    bank_account_id uuid NOT NULL REFERENCES provider_bank_accounts(id),
    amount_kobo bigint NOT NULL CHECK (amount_kobo > 0),
    currency text NOT NULL DEFAULT 'NGN',
    status text NOT NULL,
    paystack_transfer_code text UNIQUE,
    paystack_transfer_id text,
    failure_reason text,
    idempotency_key text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_withdrawals_idempotency
    ON withdrawals(provider_type, provider_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS refunds (
    id uuid PRIMARY KEY,
    reference text NOT NULL UNIQUE,
    payment_intent_id uuid NOT NULL REFERENCES payment_intents(id),
    amount_kobo bigint NOT NULL CHECK (amount_kobo > 0),
    currency text NOT NULL DEFAULT 'NGN',
    reason text NOT NULL DEFAULT '',
    status text NOT NULL,
    paystack_refund_reference text UNIQUE,
    idempotency_key text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_refunds_idempotency
    ON refunds(payment_intent_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key text NOT NULL,
    scope text NOT NULL,
    actor text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope, actor)
);
