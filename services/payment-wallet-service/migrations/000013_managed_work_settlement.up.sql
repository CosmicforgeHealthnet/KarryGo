-- Section 12: Fleet-managed work settlement.
--
-- A managed settlement is the posting of ONE Fleet ManagedWorkSettlementInstruction.
-- Fleet decides the split; this service decides only how it is posted. The row
-- exists so a posting can be found again by instruction, by idempotency key, or
-- by the execution job it settles -- which is what makes a lost response safe to
-- retry instead of a reason to guess.
CREATE TABLE IF NOT EXISTS managed_settlements (
    id                              uuid PRIMARY KEY,
    instruction_id                  text   NOT NULL,
    instruction_version             bigint NOT NULL CHECK (instruction_version > 0),

    organization_id                 text   NOT NULL,
    membership_id                   text   NOT NULL,
    provider_id                     text   NOT NULL,
    provider_type                   text   NOT NULL,

    source_service                  text   NOT NULL,
    source_job_id                   text   NOT NULL,
    source_assignment_id            text   NOT NULL DEFAULT '',

    currency                        text   NOT NULL,
    -- Every share is non-negative. A negative allocation is not a correction:
    -- corrections are reversals, which post their own compensating journal.
    gross_basis_minor               bigint NOT NULL CHECK (gross_basis_minor >= 0),
    provider_share_minor            bigint NOT NULL CHECK (provider_share_minor >= 0),
    organization_share_minor        bigint NOT NULL CHECK (organization_share_minor >= 0),
    platform_share_minor            bigint NOT NULL CHECK (platform_share_minor >= 0),

    compensation_plan_id            text   NOT NULL DEFAULT '',
    compensation_plan_version       bigint NOT NULL DEFAULT 0,
    compensation_resolution_version bigint NOT NULL DEFAULT 0,

    status                          text   NOT NULL CHECK (status IN (
        'PENDING', 'POSTED', 'FAILED_RETRYABLE', 'FAILED_TERMINAL',
        'RECONCILIATION_REQUIRED', 'REVERSED'
    )),
    failure_code                    text   NOT NULL DEFAULT '',

    ledger_transaction_id           uuid REFERENCES ledger_transactions(id),
    reversal_transaction_id         uuid REFERENCES ledger_transactions(id),

    idempotency_key                 text   NOT NULL,
    request_fingerprint             text   NOT NULL,
    correlation_id                  text   NOT NULL DEFAULT '',

    created_at                      timestamptz NOT NULL DEFAULT now(),
    updated_at                      timestamptz NOT NULL DEFAULT now(),
    posted_at                       timestamptz,
    reversed_at                     timestamptz
);

-- One instruction version posts exactly once. A re-sent instruction finds this
-- row instead of creating a second journal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_managed_settlements_instruction
    ON managed_settlements (instruction_id, instruction_version);

CREATE UNIQUE INDEX IF NOT EXISTS uq_managed_settlements_idempotency
    ON managed_settlements (idempotency_key);

-- Read-back by the execution job, for reconciliation from the service side.
CREATE INDEX IF NOT EXISTS idx_managed_settlements_source
    ON managed_settlements (source_service, source_job_id);

CREATE INDEX IF NOT EXISTS idx_managed_settlements_status
    ON managed_settlements (status)
    WHERE status IN ('PENDING', 'FAILED_RETRYABLE', 'RECONCILIATION_REQUIRED');

CREATE INDEX IF NOT EXISTS idx_managed_settlements_organization
    ON managed_settlements (organization_id, created_at DESC);

-- The legacy CompleteJob path pays the provider gross-minus-platform-fee
-- directly. For organization-managed work that is wrong: a salaried provider
-- would be paid per job on top of their salary. Marking the intent is what lets
-- Wallet refuse that path rather than relying on every caller to remember.
ALTER TABLE payment_intents
    ADD COLUMN IF NOT EXISTS settlement_mode text NOT NULL DEFAULT 'INDEPENDENT';

ALTER TABLE payment_intents
    DROP CONSTRAINT IF EXISTS chk_payment_intents_settlement_mode;
ALTER TABLE payment_intents
    ADD CONSTRAINT chk_payment_intents_settlement_mode
    CHECK (settlement_mode IN ('INDEPENDENT', 'ORGANIZATION_MANAGED'));
