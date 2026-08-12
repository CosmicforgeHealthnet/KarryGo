-- Financial foundation for the corporate/marketplace side of
-- payment-wallet-service (WP7).
--
-- Everything here is additive: the original consumer ledger
-- (wallet_accounts / ledger_transactions / ledger_entries) is untouched and
-- keeps serving the customer and provider-app flows. The finance_* tables
-- below are the authoritative record for organization wallets, provider
-- earning accounts, subscriptions, commissions, holds, settlements, refunds
-- and reconciliation.
--
-- Money is ALWAYS integer minor units plus an explicit ISO currency. No column
-- in this file is a floating point type.

-- ---------------------------------------------------------------------------
-- Shared guards
-- ---------------------------------------------------------------------------

-- finance_block_mutation makes a table append-only at the row level. Posted
-- financial history must never be edited in place; corrections are recorded as
-- compensating entries that stand next to the original.
CREATE OR REPLACE FUNCTION finance_block_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'finance: % rows are append-only (attempted %)', TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Chart of accounts, journals and balances
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_ledger_accounts (
    id                text PRIMARY KEY,
    organization_id   text,
    owner_type        text NOT NULL CHECK (owner_type IN (
                          'ORGANIZATION_WALLET', 'PROVIDER_EARNING', 'PLATFORM')),
    owner_ref         text NOT NULL DEFAULT '',
    account_code      text NOT NULL,
    currency          char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    normal_balance    text NOT NULL CHECK (normal_balance IN ('DEBIT', 'CREDIT')),
    status            text NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE', 'RESTRICTED', 'CLOSED')),
    version           bigint NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_type, owner_ref, account_code, currency)
);

CREATE INDEX IF NOT EXISTS idx_finance_ledger_accounts_org
    ON finance_ledger_accounts (organization_id, currency);
CREATE INDEX IF NOT EXISTS idx_finance_ledger_accounts_owner
    ON finance_ledger_accounts (owner_type, owner_ref);

-- Atomic balance projection. Updated in the same transaction as every posting,
-- so a read of this table and a re-aggregation of finance_journal_lines can
-- never disagree.
CREATE TABLE IF NOT EXISTS finance_account_balances (
    ledger_account_id text PRIMARY KEY REFERENCES finance_ledger_accounts(id),
    currency          char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    debit_minor       bigint NOT NULL DEFAULT 0 CHECK (debit_minor >= 0),
    credit_minor      bigint NOT NULL DEFAULT 0 CHECK (credit_minor >= 0),
    version           bigint NOT NULL DEFAULT 1,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS finance_journals (
    id                    text PRIMARY KEY,
    organization_id       text,
    journal_type          text NOT NULL,
    currency              char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status                text NOT NULL DEFAULT 'POSTED' CHECK (status = 'POSTED'),
    reference             text NOT NULL DEFAULT '',
    source_service        text NOT NULL DEFAULT '',
    source_reference      text NOT NULL DEFAULT '',
    idempotency_key       text NOT NULL DEFAULT '',
    request_fingerprint   text NOT NULL DEFAULT '',
    correlation_id        text NOT NULL DEFAULT '',
    causation_id          text NOT NULL DEFAULT '',
    reverses_journal_id   text REFERENCES finance_journals(id),
    posted_by_actor_type  text NOT NULL DEFAULT 'SYSTEM',
    posted_by_actor_id    text NOT NULL DEFAULT 'system',
    memo                  text NOT NULL DEFAULT '',
    posted_at             timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_journals_idempotency
    ON finance_journals (idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_journals_org_posted
    ON finance_journals (organization_id, posted_at DESC);
CREATE INDEX IF NOT EXISTS idx_finance_journals_source
    ON finance_journals (source_service, source_reference);
CREATE INDEX IF NOT EXISTS idx_finance_journals_reverses
    ON finance_journals (reverses_journal_id) WHERE reverses_journal_id IS NOT NULL;
-- A posted journal may be reversed at most once; a second compensating entry
-- for the same original would silently double the correction.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_journals_single_reversal
    ON finance_journals (reverses_journal_id) WHERE reverses_journal_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS finance_journal_lines (
    id                text PRIMARY KEY,
    journal_id        text NOT NULL REFERENCES finance_journals(id),
    line_no           int NOT NULL CHECK (line_no > 0),
    ledger_account_id text NOT NULL REFERENCES finance_ledger_accounts(id),
    side              text NOT NULL CHECK (side IN ('DEBIT', 'CREDIT')),
    amount_minor      bigint NOT NULL CHECK (amount_minor > 0),
    currency          char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    memo              text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (journal_id, line_no)
);

CREATE INDEX IF NOT EXISTS idx_finance_journal_lines_account
    ON finance_journal_lines (ledger_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_finance_journal_lines_journal
    ON finance_journal_lines (journal_id);

-- Balance is enforced by the database, not only by application code. A
-- deferred constraint trigger runs at COMMIT, after all of a journal's lines
-- have been inserted, so an unbalanced or mixed-currency journal cannot exist
-- even momentarily outside a transaction.
CREATE OR REPLACE FUNCTION finance_assert_journal_balanced() RETURNS trigger AS $$
DECLARE
    line_count      int;
    currency_count  int;
    foreign_lines   int;
    debit_total     bigint;
    credit_total    bigint;
BEGIN
    SELECT count(*),
           count(DISTINCT currency),
           count(*) FILTER (WHERE currency <> NEW.currency),
           COALESCE(sum(amount_minor) FILTER (WHERE side = 'DEBIT'), 0),
           COALESCE(sum(amount_minor) FILTER (WHERE side = 'CREDIT'), 0)
      INTO line_count, currency_count, foreign_lines, debit_total, credit_total
      FROM finance_journal_lines
     WHERE journal_id = NEW.id;

    IF line_count < 2 THEN
        RAISE EXCEPTION 'finance: journal % must contain at least two lines', NEW.id;
    END IF;
    IF currency_count <> 1 OR foreign_lines > 0 THEN
        RAISE EXCEPTION 'finance: journal % mixes currencies', NEW.id;
    END IF;
    IF debit_total <> credit_total THEN
        RAISE EXCEPTION 'finance: journal % is unbalanced (debit %, credit %)',
            NEW.id, debit_total, credit_total;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS finance_journals_balanced ON finance_journals;
CREATE CONSTRAINT TRIGGER finance_journals_balanced
    AFTER INSERT ON finance_journals
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION finance_assert_journal_balanced();

DROP TRIGGER IF EXISTS finance_journals_append_only ON finance_journals;
CREATE TRIGGER finance_journals_append_only
    BEFORE UPDATE OR DELETE ON finance_journals
    FOR EACH ROW EXECUTE FUNCTION finance_block_mutation();

DROP TRIGGER IF EXISTS finance_journal_lines_append_only ON finance_journal_lines;
CREATE TRIGGER finance_journal_lines_append_only
    BEFORE UPDATE OR DELETE ON finance_journal_lines
    FOR EACH ROW EXECUTE FUNCTION finance_block_mutation();

-- ---------------------------------------------------------------------------
-- Organization wallets and provider earning accounts
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_wallets (
    id                       text PRIMARY KEY,
    organization_id          text NOT NULL,
    currency                 char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status                   text NOT NULL DEFAULT 'ACTIVE'
                                 CHECK (status IN ('ACTIVE', 'RESTRICTED', 'SUSPENDED', 'CLOSED')),
    available_minor          bigint NOT NULL DEFAULT 0,
    held_minor               bigint NOT NULL DEFAULT 0 CHECK (held_minor >= 0),
    pending_settlement_minor bigint NOT NULL DEFAULT 0 CHECK (pending_settlement_minor >= 0),
    restrictions             jsonb NOT NULL DEFAULT '[]'::jsonb,
    ledger_account_id        text NOT NULL REFERENCES finance_ledger_accounts(id),
    version                  bigint NOT NULL DEFAULT 1,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, currency)
);

CREATE INDEX IF NOT EXISTS idx_finance_wallets_status
    ON finance_wallets (status, currency);

CREATE TABLE IF NOT EXISTS finance_earning_accounts (
    id                     text PRIMARY KEY,
    provider_reference     text NOT NULL,
    provider_service       text NOT NULL,
    organization_id        text,
    currency               char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status                 text NOT NULL DEFAULT 'ACTIVE'
                               CHECK (status IN ('ACTIVE', 'RESTRICTED', 'SUSPENDED', 'CLOSED')),
    payable_minor          bigint NOT NULL DEFAULT 0,
    held_minor             bigint NOT NULL DEFAULT 0 CHECK (held_minor >= 0),
    settled_minor          bigint NOT NULL DEFAULT 0 CHECK (settled_minor >= 0),
    restrictions           jsonb NOT NULL DEFAULT '[]'::jsonb,
    -- Opaque pointer to a payout destination held elsewhere. Bank details are
    -- never duplicated into this table.
    payout_destination_ref text NOT NULL DEFAULT '',
    ledger_account_id      text NOT NULL REFERENCES finance_ledger_accounts(id),
    version                bigint NOT NULL DEFAULT 1,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_service, provider_reference, currency)
);

CREATE INDEX IF NOT EXISTS idx_finance_earning_accounts_org
    ON finance_earning_accounts (organization_id, currency);
CREATE INDEX IF NOT EXISTS idx_finance_earning_accounts_status
    ON finance_earning_accounts (status, currency);

-- ---------------------------------------------------------------------------
-- Subscriptions, invoices and payments
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_subscription_plans (
    id               text PRIMARY KEY,
    code             text NOT NULL UNIQUE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    currency         char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    -- NULL means "pricing is not configured". Invoice creation is blocked for
    -- such a plan rather than defaulting to a price nobody agreed.
    price_minor      bigint CHECK (price_minor IS NULL OR price_minor >= 0),
    billing_interval text NOT NULL CHECK (billing_interval IN ('WEEK', 'MONTH', 'YEAR')),
    interval_count   int NOT NULL DEFAULT 1 CHECK (interval_count > 0),
    grace_days       int NOT NULL DEFAULT 0 CHECK (grace_days >= 0),
    status           text NOT NULL DEFAULT 'DRAFT'
                         CHECK (status IN ('DRAFT', 'ACTIVE', 'RETIRED')),
    effective_from   timestamptz NOT NULL DEFAULT now(),
    effective_to     timestamptz,
    version          bigint NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_finance_subscription_plans_status
    ON finance_subscription_plans (status, currency);

CREATE TABLE IF NOT EXISTS finance_subscriptions (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL,
    plan_id              text NOT NULL REFERENCES finance_subscription_plans(id),
    currency             char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status               text NOT NULL DEFAULT 'PENDING' CHECK (status IN (
                             'PENDING', 'ACTIVE', 'PAST_DUE', 'GRACE_PERIOD',
                             'SUSPENDED', 'CANCELLED', 'EXPIRED')),
    current_period_start timestamptz,
    current_period_end   timestamptz,
    grace_expires_at     timestamptz,
    activated_at         timestamptz,
    cancelled_at         timestamptz,
    latest_invoice_id    text,
    idempotency_key      text NOT NULL DEFAULT '',
    request_fingerprint  text NOT NULL DEFAULT '',
    version              bigint NOT NULL DEFAULT 1,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- One live subscription per organization; terminal ones stay for history.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_subscriptions_live
    ON finance_subscriptions (organization_id)
    WHERE status IN ('PENDING', 'ACTIVE', 'PAST_DUE', 'GRACE_PERIOD', 'SUSPENDED');
CREATE INDEX IF NOT EXISTS idx_finance_subscriptions_status
    ON finance_subscriptions (status, current_period_end);
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_subscriptions_idempotency
    ON finance_subscriptions (organization_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS finance_invoices (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    subscription_id     text REFERENCES finance_subscriptions(id),
    number              text NOT NULL UNIQUE,
    currency            char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    subtotal_minor      bigint NOT NULL CHECK (subtotal_minor >= 0),
    tax_minor           bigint NOT NULL DEFAULT 0 CHECK (tax_minor >= 0),
    total_minor         bigint NOT NULL CHECK (total_minor >= 0),
    amount_paid_minor   bigint NOT NULL DEFAULT 0 CHECK (amount_paid_minor >= 0),
    status              text NOT NULL DEFAULT 'OPEN' CHECK (status IN (
                            'DRAFT', 'OPEN', 'PARTIALLY_PAID', 'PAID',
                            'VOID', 'UNCOLLECTIBLE', 'EXPIRED')),
    period_start        timestamptz,
    period_end          timestamptz,
    issued_at           timestamptz NOT NULL DEFAULT now(),
    due_at              timestamptz NOT NULL,
    paid_at             timestamptz,
    idempotency_key     text NOT NULL DEFAULT '',
    request_fingerprint text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    version             bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_invoices_idempotency
    ON finance_invoices (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_invoices_org_status
    ON finance_invoices (organization_id, status, due_at);
CREATE INDEX IF NOT EXISTS idx_finance_invoices_due
    ON finance_invoices (status, due_at) WHERE status IN ('OPEN', 'PARTIALLY_PAID');

CREATE TABLE IF NOT EXISTS finance_invoice_lines (
    id                text PRIMARY KEY,
    invoice_id        text NOT NULL REFERENCES finance_invoices(id) ON DELETE CASCADE,
    line_no           int NOT NULL CHECK (line_no > 0),
    description       text NOT NULL,
    quantity          int NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_amount_minor bigint NOT NULL CHECK (unit_amount_minor >= 0),
    amount_minor      bigint NOT NULL CHECK (amount_minor >= 0),
    currency          char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (invoice_id, line_no)
);

CREATE TABLE IF NOT EXISTS finance_payments (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    invoice_id          text NOT NULL REFERENCES finance_invoices(id),
    amount_minor        bigint NOT NULL CHECK (amount_minor > 0),
    currency            char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    method              text NOT NULL,
    status              text NOT NULL DEFAULT 'PENDING' CHECK (status IN (
                            'PENDING', 'SUCCEEDED', 'FAILED', 'UNKNOWN', 'REVERSED')),
    external_reference  text NOT NULL DEFAULT '',
    journal_id          text REFERENCES finance_journals(id),
    idempotency_key     text NOT NULL DEFAULT '',
    request_fingerprint text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    last_error          text NOT NULL DEFAULT '',
    version             bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_payments_idempotency
    ON finance_payments (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_payments_invoice
    ON finance_payments (invoice_id, status);

-- ---------------------------------------------------------------------------
-- Commissions and earning allocations
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_commission_rules (
    id                     text PRIMARY KEY,
    -- NULL scope is the platform default; an organization row overrides it.
    organization_id        text,
    service_type           text NOT NULL,
    currency               char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    rule_version           int NOT NULL CHECK (rule_version > 0),
    commission_bps         int NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
    fleet_fee_bps          int NOT NULL DEFAULT 0 CHECK (fleet_fee_bps BETWEEN 0 AND 10000),
    provider_share_bps     int NOT NULL CHECK (provider_share_bps BETWEEN 0 AND 10000),
    organization_share_bps int NOT NULL CHECK (organization_share_bps BETWEEN 0 AND 10000),
    status                 text NOT NULL DEFAULT 'ACTIVE'
                               CHECK (status IN ('ACTIVE', 'RETIRED')),
    effective_from         timestamptz NOT NULL DEFAULT now(),
    effective_to           timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    -- The net split must consume exactly the residual, or the derived journal
    -- cannot balance.
    CHECK (provider_share_bps + organization_share_bps = 10000),
    CHECK (commission_bps + fleet_fee_bps <= 10000)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_commission_rules_version
    ON finance_commission_rules (COALESCE(organization_id, ''), service_type, currency, rule_version);
CREATE INDEX IF NOT EXISTS idx_finance_commission_rules_lookup
    ON finance_commission_rules (service_type, currency, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS finance_earning_allocations (
    id                        text PRIMARY KEY,
    organization_id           text NOT NULL,
    source_service            text NOT NULL,
    job_reference             text NOT NULL,
    currency                  char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    rule_id                   text NOT NULL REFERENCES finance_commission_rules(id),
    rule_version              int NOT NULL,
    -- The exact rule as applied, frozen at posting time. A later rule change
    -- must never silently restate a historical allocation.
    rule_snapshot             jsonb NOT NULL,
    gross_minor               bigint NOT NULL CHECK (gross_minor > 0),
    commission_minor          bigint NOT NULL CHECK (commission_minor >= 0),
    fleet_fee_minor           bigint NOT NULL CHECK (fleet_fee_minor >= 0),
    provider_earning_minor    bigint NOT NULL CHECK (provider_earning_minor >= 0),
    organization_earning_minor bigint NOT NULL CHECK (organization_earning_minor >= 0),
    provider_reference        text NOT NULL DEFAULT '',
    earning_account_id        text REFERENCES finance_earning_accounts(id),
    wallet_id                 text REFERENCES finance_wallets(id),
    journal_id                text NOT NULL REFERENCES finance_journals(id),
    status                    text NOT NULL DEFAULT 'POSTED'
                                  CHECK (status IN ('POSTED', 'REVERSED')),
    idempotency_key           text NOT NULL DEFAULT '',
    request_fingerprint       text NOT NULL DEFAULT '',
    correlation_id            text NOT NULL DEFAULT '',
    causation_id              text NOT NULL DEFAULT '',
    version                   bigint NOT NULL DEFAULT 1,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, source_service, job_reference),
    -- The split must reconstruct the gross exactly.
    CHECK (commission_minor + fleet_fee_minor + provider_earning_minor
           + organization_earning_minor = gross_minor)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_allocations_idempotency
    ON finance_earning_allocations (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_allocations_earning_account
    ON finance_earning_allocations (earning_account_id, status);

-- ---------------------------------------------------------------------------
-- Holds
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_holds (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL,
    subject_type            text NOT NULL CHECK (subject_type IN ('WALLET', 'EARNING_ACCOUNT')),
    wallet_id               text REFERENCES finance_wallets(id),
    earning_account_id      text REFERENCES finance_earning_accounts(id),
    reason_code             text NOT NULL,
    reason                  text NOT NULL DEFAULT '',
    amount_minor            bigint NOT NULL CHECK (amount_minor > 0),
    currency                char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    authority_service       text NOT NULL DEFAULT '',
    authoritative_reference text NOT NULL DEFAULT '',
    reference_verified      boolean NOT NULL DEFAULT false,
    state                   text NOT NULL DEFAULT 'PROVISIONAL' CHECK (state IN (
                                'PROVISIONAL', 'ACTIVE', 'RELEASED', 'EXPIRED', 'CANCELLED')),
    expires_at              timestamptz,
    released_at             timestamptz,
    journal_id              text REFERENCES finance_journals(id),
    release_journal_id      text REFERENCES finance_journals(id),
    idempotency_key         text NOT NULL DEFAULT '',
    request_fingerprint     text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    causation_id            text NOT NULL DEFAULT '',
    version                 bigint NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CHECK ((subject_type = 'WALLET' AND wallet_id IS NOT NULL AND earning_account_id IS NULL)
        OR (subject_type = 'EARNING_ACCOUNT' AND earning_account_id IS NOT NULL AND wallet_id IS NULL)),
    -- An unverified incident/dispute reference may only support a bounded
    -- hold. Indefinitely freezing money on an unconfirmed claim is exactly the
    -- failure mode this constraint exists to make impossible.
    CHECK (reference_verified OR expires_at IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_holds_idempotency
    ON finance_holds (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_holds_state
    ON finance_holds (state, expires_at);
CREATE INDEX IF NOT EXISTS idx_finance_holds_org
    ON finance_holds (organization_id, state);
CREATE INDEX IF NOT EXISTS idx_finance_holds_subject
    ON finance_holds (subject_type, wallet_id, earning_account_id);

-- ---------------------------------------------------------------------------
-- Settlements
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_settlement_batches (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL,
    beneficiary_type        text NOT NULL CHECK (beneficiary_type IN ('PROVIDER', 'COMPANY')),
    beneficiary_reference   text NOT NULL,
    earning_account_id      text REFERENCES finance_earning_accounts(id),
    wallet_id               text REFERENCES finance_wallets(id),
    currency                char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor             bigint NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    item_count              int NOT NULL DEFAULT 0 CHECK (item_count >= 0),
    status                  text NOT NULL DEFAULT 'PENDING_APPROVAL' CHECK (status IN (
                                'PENDING_APPROVAL', 'APPROVED', 'PROCESSING', 'COMPLETED',
                                'PARTIALLY_FAILED', 'CANCELLED')),
    requested_by_actor_type text NOT NULL DEFAULT '',
    requested_by_actor_id   text NOT NULL DEFAULT '',
    approved_by_actor_type  text NOT NULL DEFAULT '',
    approved_by_actor_id    text NOT NULL DEFAULT '',
    approved_at             timestamptz,
    idempotency_key         text NOT NULL DEFAULT '',
    request_fingerprint     text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    version                 bigint NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_settlement_batches_idempotency
    ON finance_settlement_batches (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_settlement_batches_status
    ON finance_settlement_batches (status, created_at);

CREATE TABLE IF NOT EXISTS finance_settlement_items (
    id                    text PRIMARY KEY,
    batch_id              text NOT NULL REFERENCES finance_settlement_batches(id),
    organization_id       text NOT NULL,
    earning_allocation_id text REFERENCES finance_earning_allocations(id),
    amount_minor          bigint NOT NULL CHECK (amount_minor > 0),
    currency              char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status                text NOT NULL DEFAULT 'PENDING' CHECK (status IN (
                              'PENDING', 'PROCESSING', 'UNKNOWN', 'SUCCEEDED',
                              'RETRYABLE_FAILURE', 'PERMANENT_FAILURE', 'REVERSED',
                              'RECONCILING', 'DEAD_LETTER')),
    attempts              int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at       timestamptz NOT NULL DEFAULT now(),
    lease_owner           text,
    lease_expires_at      timestamptz,
    external_reference    text NOT NULL DEFAULT '',
    journal_id            text REFERENCES finance_journals(id),
    reversal_journal_id   text REFERENCES finance_journals(id),
    idempotency_key       text NOT NULL,
    last_error            text NOT NULL DEFAULT '',
    version               bigint NOT NULL DEFAULT 1,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    completed_at          timestamptz,
    UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_finance_settlement_items_due
    ON finance_settlement_items (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_finance_settlement_items_lease
    ON finance_settlement_items (lease_expires_at) WHERE lease_owner IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_finance_settlement_items_batch
    ON finance_settlement_items (batch_id, status);
-- One settlement item per allocation: paying the same earning twice is the
-- exact failure a marketplace cannot recover from.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_settlement_items_allocation
    ON finance_settlement_items (earning_allocation_id)
    WHERE earning_allocation_id IS NOT NULL
      AND status <> 'PERMANENT_FAILURE'
      AND status <> 'REVERSED';

-- ---------------------------------------------------------------------------
-- Refunds and reversals
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_refunds (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL,
    original_journal_id     text NOT NULL REFERENCES finance_journals(id),
    original_reference      text NOT NULL DEFAULT '',
    amount_minor            bigint NOT NULL CHECK (amount_minor > 0),
    currency                char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    reason_code             text NOT NULL DEFAULT '',
    reason                  text NOT NULL DEFAULT '',
    status                  text NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
                                'REQUESTED', 'APPROVED', 'REJECTED', 'PROCESSING',
                                'UNKNOWN', 'COMPLETED', 'FAILED', 'RECONCILING',
                                'DEAD_LETTER', 'REVERSED')),
    requested_by_actor_type text NOT NULL DEFAULT '',
    requested_by_actor_id   text NOT NULL DEFAULT '',
    approved_by_actor_type  text NOT NULL DEFAULT '',
    approved_by_actor_id    text NOT NULL DEFAULT '',
    approved_at             timestamptz,
    attempts                int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at         timestamptz NOT NULL DEFAULT now(),
    lease_owner             text,
    lease_expires_at        timestamptz,
    external_reference      text NOT NULL DEFAULT '',
    journal_id              text REFERENCES finance_journals(id),
    reversal_journal_id     text REFERENCES finance_journals(id),
    last_error              text NOT NULL DEFAULT '',
    idempotency_key         text NOT NULL DEFAULT '',
    request_fingerprint     text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    version                 bigint NOT NULL DEFAULT 1,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    completed_at            timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_refunds_idempotency
    ON finance_refunds (organization_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_refunds_original
    ON finance_refunds (original_journal_id, status);
CREATE INDEX IF NOT EXISTS idx_finance_refunds_due
    ON finance_refunds (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_finance_refunds_lease
    ON finance_refunds (lease_expires_at) WHERE lease_owner IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Reconciliation and statements
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_reconciliation_runs (
    id               text PRIMARY KEY,
    scope            text NOT NULL,
    status           text NOT NULL DEFAULT 'RUNNING'
                         CHECK (status IN ('RUNNING', 'COMPLETED', 'FAILED')),
    findings_count   int NOT NULL DEFAULT 0 CHECK (findings_count >= 0),
    checked_count    int NOT NULL DEFAULT 0 CHECK (checked_count >= 0),
    lease_owner      text,
    lease_expires_at timestamptz,
    started_at       timestamptz NOT NULL DEFAULT now(),
    completed_at     timestamptz,
    last_error       text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_finance_reconciliation_runs_status
    ON finance_reconciliation_runs (status, started_at DESC);

CREATE TABLE IF NOT EXISTS finance_reconciliation_findings (
    id              text PRIMARY KEY,
    run_id          text NOT NULL REFERENCES finance_reconciliation_runs(id),
    organization_id text,
    entity_type     text NOT NULL,
    entity_id       text NOT NULL,
    finding_code    text NOT NULL,
    details         jsonb NOT NULL DEFAULT '{}'::jsonb,
    resolved_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_finance_findings_run
    ON finance_reconciliation_findings (run_id, finding_code);
CREATE INDEX IF NOT EXISTS idx_finance_findings_open
    ON finance_reconciliation_findings (entity_type, entity_id) WHERE resolved_at IS NULL;

CREATE TABLE IF NOT EXISTS finance_statements (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL,
    statement_type     text NOT NULL CHECK (statement_type IN ('TRANSACTION', 'SETTLEMENT')),
    currency           char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    period_start       timestamptz NOT NULL,
    period_end         timestamptz NOT NULL,
    line_count         int NOT NULL DEFAULT 0 CHECK (line_count >= 0),
    total_debit_minor  bigint NOT NULL DEFAULT 0 CHECK (total_debit_minor >= 0),
    total_credit_minor bigint NOT NULL DEFAULT 0 CHECK (total_credit_minor >= 0),
    content_hash       text NOT NULL,
    generated_by_type  text NOT NULL DEFAULT '',
    generated_by_id    text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_finance_statements_org
    ON finance_statements (organization_id, statement_type, period_end DESC);

-- ---------------------------------------------------------------------------
-- History, audit, idempotency and outbox
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_state_history (
    id              bigserial PRIMARY KEY,
    organization_id text,
    entity_type     text NOT NULL,
    entity_id       text NOT NULL,
    from_state      text NOT NULL DEFAULT '',
    to_state        text NOT NULL,
    actor_type      text NOT NULL DEFAULT 'SYSTEM',
    actor_id        text NOT NULL DEFAULT 'system',
    reason          text NOT NULL DEFAULT '',
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_finance_state_history_entity
    ON finance_state_history (entity_type, entity_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS finance_audit_records (
    id               text PRIMARY KEY,
    organization_id  text,
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    action           text NOT NULL,
    resource_type    text NOT NULL,
    resource_id      text NOT NULL,
    previous_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    new_summary      jsonb NOT NULL DEFAULT '{}'::jsonb,
    reason           text,
    request_id       text NOT NULL DEFAULT '',
    correlation_id   text NOT NULL DEFAULT '',
    network_meta     jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_finance_audit_resource
    ON finance_audit_records (resource_type, resource_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_finance_audit_org
    ON finance_audit_records (organization_id, occurred_at DESC);

DROP TRIGGER IF EXISTS finance_audit_append_only ON finance_audit_records;
CREATE TRIGGER finance_audit_append_only
    BEFORE UPDATE OR DELETE ON finance_audit_records
    FOR EACH ROW EXECUTE FUNCTION finance_block_mutation();

CREATE TABLE IF NOT EXISTS finance_idempotency_keys (
    scope_id            text NOT NULL,
    endpoint            text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    status              text NOT NULL CHECK (status IN ('IN_PROGRESS', 'COMPLETED')),
    response_status     int,
    response_body       bytea,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL,
    PRIMARY KEY (scope_id, endpoint, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_finance_idempotency_expiry
    ON finance_idempotency_keys (expires_at);
CREATE INDEX IF NOT EXISTS idx_finance_idempotency_fingerprint
    ON finance_idempotency_keys (request_fingerprint);

-- One-time nonces provide replay protection in addition to HMAC timestamp
-- freshness. A captured request inside the freshness window is still rejected.
CREATE TABLE IF NOT EXISTS finance_service_request_nonces (
    service_name text NOT NULL,
    nonce text NOT NULL,
    request_fingerprint text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_name, nonce)
);
CREATE INDEX IF NOT EXISTS idx_finance_service_request_nonces_expiry
    ON finance_service_request_nonces (expires_at);

CREATE TABLE IF NOT EXISTS finance_worker_leases (
    worker_name text PRIMARY KEY,
    lease_owner text NOT NULL,
    lease_expires_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_finance_worker_leases_expiry
    ON finance_worker_leases (lease_expires_at);

CREATE TABLE IF NOT EXISTS finance_outbox (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    schema_version      int NOT NULL CHECK (schema_version > 0),
    aggregate_type      text NOT NULL,
    aggregate_id        text NOT NULL,
    aggregate_version   bigint NOT NULL DEFAULT 0,
    organization_id     text,
    payload             jsonb NOT NULL,
    headers             jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              text NOT NULL DEFAULT 'PENDING'
                            CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
    available_at        timestamptz NOT NULL DEFAULT now(),
    attempts            int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_owner         text,
    lease_expires_at    timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz,
    last_error_redacted text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_finance_outbox_due
    ON finance_outbox (status, available_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_finance_outbox_lease
    ON finance_outbox (lease_expires_at) WHERE lease_owner IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_finance_outbox_aggregate
    ON finance_outbox (aggregate_type, aggregate_id);

-- ---------------------------------------------------------------------------
-- Finance RBAC and approval policy
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_permissions (
    code        text PRIMARY KEY,
    description text NOT NULL,
    sensitive   boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS finance_roles (
    id          text PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    system_role boolean NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS finance_role_permissions (
    role_id         text NOT NULL REFERENCES finance_roles(id) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES finance_permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE IF NOT EXISTS finance_role_grants (
    id              text PRIMARY KEY,
    -- NULL organization means a platform-scoped grant (operations staff and
    -- calling services); a value scopes the grant to one tenant.
    organization_id text,
    subject_type    text NOT NULL CHECK (subject_type IN ('SERVICE', 'USER')),
    subject_id      text NOT NULL,
    role_id         text NOT NULL REFERENCES finance_roles(id),
    granted_by      text NOT NULL DEFAULT '',
    granted_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_role_grants_unique
    ON finance_role_grants (COALESCE(organization_id, ''), subject_type, subject_id, role_id);
CREATE INDEX IF NOT EXISTS idx_finance_role_grants_subject
    ON finance_role_grants (subject_type, subject_id);

CREATE TABLE IF NOT EXISTS finance_approval_policies (
    id                    text PRIMARY KEY,
    scope_id              text NOT NULL DEFAULT 'PLATFORM',
    operation             text NOT NULL,
    minimum_approvals     int NOT NULL DEFAULT 1 CHECK (minimum_approvals >= 1),
    self_approval_allowed boolean NOT NULL DEFAULT false,
    policy_version        int NOT NULL DEFAULT 1 CHECK (policy_version > 0),
    status                text NOT NULL DEFAULT 'ACTIVE'
                              CHECK (status IN ('ACTIVE', 'RETIRED')),
    effective_from        timestamptz NOT NULL DEFAULT now(),
    effective_to          timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope_id, operation, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_finance_approval_policies_lookup
    ON finance_approval_policies (scope_id, operation, status, effective_from DESC);

-- Seeded catalogue. Re-running these statements is a no-op, which is what lets
-- the integration suite restore the catalogue after truncating.
INSERT INTO finance_permissions (code, description, sensitive) VALUES
    ('finance.wallet.view',           'View organization wallets and earning accounts', false),
    ('finance.transaction.view',      'View ledger transactions and statements',        false),
    ('finance.subscription.manage',   'Create and manage subscriptions and invoices',   true),
    ('finance.payment.confirm',       'Confirm an authoritative invoice payment',       true),
    ('finance.commission.manage',     'Configure versioned commission and earning rules', true),
    ('finance.hold.create',           'Place a hold on funds',                          true),
    ('finance.hold.release',          'Release a hold on funds',                        true),
    ('finance.settlement.initiate',   'Request a settlement batch',                     true),
    ('finance.settlement.approve',    'Approve a settlement batch for execution',       true),
    ('finance.refund.request',        'Request a refund',                               true),
    ('finance.refund.approve',        'Approve a refund for execution',                 true),
    ('finance.reconciliation.run',    'Run financial reconciliation',                   true),
    ('finance.statement.export',      'Export transaction and settlement statements',   false),
    ('finance.wallet.restrict',       'Restrict a wallet or earning account',           true),
    ('finance.wallet.suspend',        'Suspend or close a wallet or earning account',   true)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, sensitive = EXCLUDED.sensitive;

INSERT INTO finance_roles (id, name, description) VALUES
    ('frol_finance_viewer',     'finance_viewer',     'Read-only access to financial records'),
    ('frol_finance_operator',   'finance_operator',   'Initiates financial operations'),
    ('frol_finance_approver',   'finance_approver',   'Approves financial operations'),
    ('frol_finance_reconciler', 'finance_reconciler', 'Runs reconciliation'),
    ('frol_finance_admin',      'finance_admin',      'Full financial administration')
ON CONFLICT (id) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_viewer', code FROM finance_permissions
WHERE code IN ('finance.wallet.view', 'finance.transaction.view', 'finance.statement.export')
ON CONFLICT DO NOTHING;

INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_operator', code FROM finance_permissions
WHERE code IN ('finance.wallet.view', 'finance.transaction.view', 'finance.statement.export',
               'finance.subscription.manage', 'finance.hold.create', 'finance.hold.release',
               'finance.settlement.initiate', 'finance.refund.request')
ON CONFLICT DO NOTHING;

INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_approver', code FROM finance_permissions
WHERE code IN ('finance.wallet.view', 'finance.transaction.view',
               'finance.settlement.approve', 'finance.refund.approve')
ON CONFLICT DO NOTHING;

INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_reconciler', code FROM finance_permissions
WHERE code IN ('finance.wallet.view', 'finance.transaction.view', 'finance.reconciliation.run')
ON CONFLICT DO NOTHING;

-- The admin role holds every permission. Holding both "initiate" and "approve"
-- is still not enough to approve your own request: separation of duties is
-- enforced against the actor identity, not the role.
INSERT INTO finance_role_permissions (role_id, permission_code)
SELECT 'frol_finance_admin', code FROM finance_permissions
ON CONFLICT DO NOTHING;

INSERT INTO finance_approval_policies
    (id, scope_id, operation, minimum_approvals, self_approval_allowed, policy_version)
VALUES
    ('fapp_platform_settlement_v1', 'PLATFORM', 'SETTLEMENT_APPROVAL', 1, false, 1),
    ('fapp_platform_refund_v1',     'PLATFORM', 'REFUND_APPROVAL',     1, false, 1)
ON CONFLICT (scope_id, operation, policy_version) DO NOTHING;

-- Service identities are authorized independently from possession of an HMAC
-- secret. Fleet may initiate and read operations but cannot approve them.
INSERT INTO finance_role_grants
    (id, organization_id, subject_type, subject_id, role_id, granted_by)
VALUES
    ('frgr_fleet_operator', NULL, 'SERVICE', 'fleet-partner-service', 'frol_finance_operator', 'migration'),
    ('frgr_wallet_reconciler', NULL, 'SERVICE', 'payment-wallet-service', 'frol_finance_reconciler', 'migration'),
    ('frgr_wallet_admin', NULL, 'SERVICE', 'payment-wallet-service', 'frol_finance_admin', 'migration'),
    ('frgr_admin_approver', NULL, 'SERVICE', 'admin-backoffice-service', 'frol_finance_admin', 'migration')
ON CONFLICT DO NOTHING;
