-- Section 13: company wallets, provider sub-wallets, allocations and
-- withdrawals.
--
-- Fleet owns references, instructions, policy and approval workflow. It owns
-- no ledger and no authoritative balance: every table here stores an opaque
-- Wallet reference plus a freshness state that says how much the local copy
-- may be trusted, and the answer is never "enough to move money on its own".

-- ---------------------------------------------------------------------------
-- Organization wallet link
-- ---------------------------------------------------------------------------
--
-- The link already existed as the onboarding wallet-provisioning projection.
-- It is extended rather than replaced: a second organization-wallet table
-- would mean two answers to "is this organization's wallet ready", and
-- activation depends on that answer.

ALTER TABLE fleet_partner_organization_wallet_provisioning
    ADD COLUMN IF NOT EXISTS currency             char(3),
    ADD COLUMN IF NOT EXISTS freshness_state      text NOT NULL DEFAULT 'UNKNOWN',
    ADD COLUMN IF NOT EXISTS reconciliation_state text NOT NULL DEFAULT 'IDLE',
    ADD COLUMN IF NOT EXISTS correlation_id       text NOT NULL DEFAULT '';

ALTER TABLE fleet_partner_organization_wallet_provisioning
    DROP CONSTRAINT IF EXISTS fleet_partner_organization_wallet_provisioning_status_check;

-- The link states of section 9.2. UNKNOWN, STALE and RECONCILING are distinct
-- from FAILED on purpose: they mean "ask again", and every one of them fails a
-- sensitive-operation check closed rather than being read as a soft ACTIVE.
ALTER TABLE fleet_partner_organization_wallet_provisioning
    ADD CONSTRAINT fleet_partner_organization_wallet_provisioning_status_check
    CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'PROVISIONING', 'ACTIVE', 'SUSPENDED',
                      'RESTRICTED', 'UNKNOWN', 'STALE', 'RECONCILING', 'FAILED_RETRYABLE',
                      'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED', 'CLOSED'));

ALTER TABLE fleet_partner_organization_wallet_provisioning
    ADD CONSTRAINT fleet_partner_organization_wallet_freshness_check
    CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING'));

-- ---------------------------------------------------------------------------
-- Provider sub-wallet link
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_provider_subwallet_links
    ADD COLUMN IF NOT EXISTS provider_id       text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_service  text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS currency          char(3),
    ADD COLUMN IF NOT EXISTS freshness_state   text NOT NULL DEFAULT 'UNKNOWN',
    ADD COLUMN IF NOT EXISTS version           bigint NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS last_error_code   text NOT NULL DEFAULT '';

ALTER TABLE fleet_partner_provider_subwallet_links
    DROP CONSTRAINT IF EXISTS fleet_partner_provider_subwallet_links_status_check;

ALTER TABLE fleet_partner_provider_subwallet_links
    ADD CONSTRAINT fleet_partner_provider_subwallet_links_status_check
    CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'PROVISIONING', 'ACTIVE', 'SUSPENDED',
                      'RESTRICTED', 'UNKNOWN', 'STALE', 'RECONCILING', 'FAILED_RETRYABLE',
                      'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED', 'DISABLED', 'CLOSED'));

ALTER TABLE fleet_partner_provider_subwallet_links
    ADD CONSTRAINT fleet_partner_provider_subwallet_links_freshness_check
    CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING'));

-- One active sub-wallet link per membership, service category and currency.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_partner_subwallet_links_active
    ON fleet_partner_provider_subwallet_links
       (organization_id, provider_membership_id, provider_service, currency)
    WHERE status NOT IN ('CLOSED', 'DISABLED') AND currency IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Organization withdrawal policy
-- ---------------------------------------------------------------------------
--
-- Company policy narrows what a provider may do. It can never widen it: the
-- Wallet-side compliance, verification and account-status rules are applied
-- independently, and no value here can switch them off.

CREATE TABLE IF NOT EXISTS fleet_partner_organization_withdrawal_policies (
    organization_id   text PRIMARY KEY REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    policy_mode       text NOT NULL DEFAULT 'COMPANY_APPROVAL_REQUIRED'
                      CHECK (policy_mode IN ('PROVIDER_DIRECT', 'COMPANY_APPROVAL_REQUIRED',
                                             'COMPANY_INITIATED_ONLY', 'WITHDRAWALS_DISABLED')),
    minimum_amount_minor bigint NOT NULL DEFAULT 0 CHECK (minimum_amount_minor >= 0),
    approval_threshold_minor bigint NOT NULL DEFAULT 0 CHECK (approval_threshold_minor >= 0),
    allocation_approval_threshold_minor bigint NOT NULL DEFAULT 0
                      CHECK (allocation_approval_threshold_minor >= 0),
    currency          char(3),
    updated_by        text NOT NULL DEFAULT '',
    version           bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Provider allocation instructions
-- ---------------------------------------------------------------------------
--
-- The instruction is Fleet's record of a decision. The money movement itself
-- belongs to Wallet; wallet_transaction_reference points at it once Wallet
-- confirms, and is never derived locally.

CREATE TABLE IF NOT EXISTS fleet_partner_provider_allocation_instructions (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id               text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    provider_membership_id  text NOT NULL
                            REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    provider_id             text NOT NULL DEFAULT '',
    provider_subwallet_id   text NOT NULL DEFAULT '',
    organization_wallet_id  text NOT NULL DEFAULT '',
    allocation_type         text NOT NULL
                            CHECK (allocation_type IN ('SALARY', 'WAGE', 'PER_JOB', 'ALLOWANCE',
                                                       'REIMBURSEMENT', 'BONUS', 'ADJUSTMENT_CREDIT',
                                                       'APPROVED_DEDUCTION', 'REVERSAL')),
    amount_minor            bigint NOT NULL CHECK (amount_minor > 0),
    currency                char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    -- Section 15 reference types are deliberately not accepted yet.
    business_reference_type text NOT NULL
                            CHECK (business_reference_type IN ('TRIP', 'ASSIGNMENT', 'PAYROLL_REFERENCE',
                                                               'EXPENSE_REFERENCE', 'MANUAL_APPROVED_REFERENCE')),
    business_reference_id   text NOT NULL,
    reason_code             text NOT NULL,
    provider_explanation    text NOT NULL DEFAULT '',
    -- Never returned on a provider-facing route.
    private_note            text NOT NULL DEFAULT '',
    legal_basis_reference   text NOT NULL DEFAULT '',
    requested_by            text NOT NULL,
    approval_request_id     text,
    finance_operation_id    text,
    wallet_operation_id     text NOT NULL DEFAULT '',
    wallet_transaction_reference text NOT NULL DEFAULT '',
    idempotency_key         text NOT NULL,
    request_fingerprint     text NOT NULL,
    expected_version        bigint NOT NULL DEFAULT 0,
    status                  text NOT NULL DEFAULT 'DRAFT'
                            CHECK (status IN ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'SUBMISSION_PENDING',
                                              'PROCESSING', 'UNKNOWN', 'COMPLETED', 'FAILED_RETRYABLE',
                                              'FAILED_PERMANENT', 'REJECTED', 'RECONCILING', 'REVERSED',
                                              'DEAD_LETTER')),
    source_version          bigint NOT NULL DEFAULT 0,
    last_event_id           text NOT NULL DEFAULT '',
    freshness_state         text NOT NULL DEFAULT 'UNKNOWN'
                            CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING')),
    reconciliation_state    text NOT NULL DEFAULT 'IDLE'
                            CHECK (reconciliation_state IN ('IDLE', 'PENDING', 'PROCESSING', 'RETRYABLE_FAILURE',
                                                            'UNKNOWN', 'DEAD_LETTER')),
    safe_error_code         text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    causation_id            text NOT NULL DEFAULT '',
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by              text NOT NULL,
    updated_by              text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    completed_at            timestamptz,
    UNIQUE (organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_allocation_instructions_org
    ON fleet_partner_provider_allocation_instructions (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_allocation_instructions_membership
    ON fleet_partner_provider_allocation_instructions (organization_id, provider_membership_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_allocation_instructions_branch
    ON fleet_partner_provider_allocation_instructions (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fleet_allocation_instructions_status
    ON fleet_partner_provider_allocation_instructions (status, created_at)
    WHERE status IN ('APPROVED', 'SUBMISSION_PENDING', 'PROCESSING', 'UNKNOWN', 'RECONCILING', 'FAILED_RETRYABLE');

-- One live allocation per protected business purpose. A completed trip funds
-- exactly one per-job allocation; a second one for the same trip is a mistake
-- or a duplicate submission, and the database is where that is settled rather
-- than in whichever handler happens to run first.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_allocation_instructions_business_purpose
    ON fleet_partner_provider_allocation_instructions
       (organization_id, provider_membership_id, business_reference_type, business_reference_id, allocation_type)
    WHERE business_reference_type IN ('TRIP', 'ASSIGNMENT')
      AND allocation_type IN ('PER_JOB', 'BONUS')
      AND status NOT IN ('REJECTED', 'FAILED_PERMANENT', 'DEAD_LETTER', 'REVERSED');

CREATE TABLE IF NOT EXISTS fleet_partner_provider_allocation_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    instruction_id  text NOT NULL
                    REFERENCES fleet_partner_provider_allocation_instructions (id) ON DELETE RESTRICT,
    previous_status text NOT NULL,
    new_status      text NOT NULL,
    safe_reason_code text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    correlation_id  text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_allocation_history_instruction
    ON fleet_partner_provider_allocation_history (instruction_id, occurred_at);

CREATE TRIGGER trg_fleet_allocation_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_provider_allocation_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Provider withdrawal requests
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_provider_withdrawal_requests (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id               text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    provider_membership_id  text NOT NULL
                            REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    provider_id             text NOT NULL DEFAULT '',
    provider_subwallet_id   text NOT NULL DEFAULT '',
    amount_minor            bigint NOT NULL CHECK (amount_minor > 0),
    currency                char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    -- Opaque. Fleet never stores an account number, a sort code or a bank name.
    destination_reference   text NOT NULL,
    policy_mode             text NOT NULL
                            CHECK (policy_mode IN ('PROVIDER_DIRECT', 'COMPANY_APPROVAL_REQUIRED',
                                                   'COMPANY_INITIATED_ONLY', 'WITHDRAWALS_DISABLED')),
    requested_by_actor_type text NOT NULL
                            CHECK (requested_by_actor_type IN ('PROVIDER', 'ORGANIZATION_USER', 'SERVICE')),
    requested_by            text NOT NULL,
    approval_request_id     text,
    decided_by              text NOT NULL DEFAULT '',
    decided_at              timestamptz,
    decision_reason_code    text NOT NULL DEFAULT '',
    finance_operation_id    text,
    wallet_operation_id     text NOT NULL DEFAULT '',
    wallet_withdrawal_reference text NOT NULL DEFAULT '',
    idempotency_key         text NOT NULL,
    request_fingerprint     text NOT NULL,
    expected_version        bigint NOT NULL DEFAULT 0,
    status                  text NOT NULL DEFAULT 'REQUESTED'
                            CHECK (status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED',
                                              'SUBMISSION_PENDING', 'PROCESSING', 'UNKNOWN', 'COMPLETED',
                                              'FAILED_RETRYABLE', 'FAILED_PERMANENT', 'RECONCILING',
                                              'CANCELLED', 'DEAD_LETTER')),
    source_version          bigint NOT NULL DEFAULT 0,
    last_event_id           text NOT NULL DEFAULT '',
    freshness_state         text NOT NULL DEFAULT 'UNKNOWN'
                            CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING')),
    reconciliation_state    text NOT NULL DEFAULT 'IDLE'
                            CHECK (reconciliation_state IN ('IDLE', 'PENDING', 'PROCESSING', 'RETRYABLE_FAILURE',
                                                            'UNKNOWN', 'DEAD_LETTER')),
    safe_error_code         text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    causation_id            text NOT NULL DEFAULT '',
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    completed_at            timestamptz,
    UNIQUE (organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_withdrawal_requests_org
    ON fleet_partner_provider_withdrawal_requests (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_withdrawal_requests_membership
    ON fleet_partner_provider_withdrawal_requests (organization_id, provider_membership_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_withdrawal_requests_status
    ON fleet_partner_provider_withdrawal_requests (status, created_at)
    WHERE status IN ('APPROVED', 'SUBMISSION_PENDING', 'PROCESSING', 'UNKNOWN', 'RECONCILING', 'FAILED_RETRYABLE');

-- One open withdrawal per membership. Two open requests against one sub-wallet
-- is how the same balance gets promised twice.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_withdrawal_requests_open
    ON fleet_partner_provider_withdrawal_requests (organization_id, provider_membership_id)
    WHERE status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'SUBMISSION_PENDING',
                     'PROCESSING', 'UNKNOWN', 'RECONCILING');

CREATE TABLE IF NOT EXISTS fleet_partner_provider_withdrawal_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    withdrawal_id    text NOT NULL
                     REFERENCES fleet_partner_provider_withdrawal_requests (id) ON DELETE RESTRICT,
    previous_status  text NOT NULL,
    new_status       text NOT NULL,
    safe_reason_code text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_withdrawal_history_withdrawal
    ON fleet_partner_provider_withdrawal_history (withdrawal_id, occurred_at);

CREATE TRIGGER trg_fleet_withdrawal_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_provider_withdrawal_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Wallet transaction projections
-- ---------------------------------------------------------------------------
--
-- A display-only copy of what Wallet reported. Balances are NOT derived from
-- summing this table anywhere; the column comment is not the control, the
-- absence of any aggregate query over it is.

CREATE TABLE IF NOT EXISTS fleet_partner_wallet_transaction_projections (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id           text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    wallet_reference    text NOT NULL,
    transaction_reference text NOT NULL,
    subject_type        text NOT NULL,
    subject_id          text NOT NULL DEFAULT '',
    direction           text NOT NULL CHECK (direction IN ('CREDIT', 'DEBIT')),
    category            text NOT NULL,
    amount_minor        bigint NOT NULL CHECK (amount_minor >= 0),
    currency            char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    safe_status         text NOT NULL,
    occurred_at         timestamptz NOT NULL,
    source_service      text NOT NULL DEFAULT 'payment-wallet-service',
    source_version      bigint NOT NULL CHECK (source_version > 0),
    last_event_id       text NOT NULL DEFAULT '',
    freshness_state     text NOT NULL DEFAULT 'CURRENT'
                        CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING')),
    last_synced_at      timestamptz NOT NULL DEFAULT now(),
    version             bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, wallet_reference, transaction_reference)
);

CREATE INDEX IF NOT EXISTS idx_fleet_wallet_transactions_cursor
    ON fleet_partner_wallet_transaction_projections (organization_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_wallet_transactions_subject
    ON fleet_partner_wallet_transaction_projections (organization_id, subject_type, subject_id);

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('wallet.read',                        'wallet', 'Read the organization wallet projection',            true),
    ('wallet.transactions.read',           'wallet', 'Read organization wallet transactions',              true),
    ('wallet.provider_subwallet.read',     'wallet', 'Read provider sub-wallets and statements',           true),
    ('wallet.provider_allocations.create', 'wallet', 'Request an allocation to a provider sub-wallet',     true),
    ('wallet.provider_allocations.approve','wallet', 'Approve a provider allocation or approved deduction', true),
    ('wallet.provider_withdrawals.read',   'wallet', 'Read provider withdrawal requests',                  true),
    ('wallet.provider_withdrawals.approve','wallet', 'Approve a provider withdrawal',                      true),
    ('wallet.provider_withdrawals.reject', 'wallet', 'Reject a provider withdrawal',                       true),
    ('wallet.policies.manage',             'wallet', 'Manage organization withdrawal and allocation policy', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code LIKE 'wallet.%'
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_finance', code FROM fleet_partner_permissions
WHERE code LIKE 'wallet.%'
ON CONFLICT DO NOTHING;

-- Administration and operations can see money move; they cannot move it.
-- Compliance and analysts read only.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'wallet.read'),
    ('rol_system_admin',      'wallet.transactions.read'),
    ('rol_system_admin',      'wallet.provider_subwallet.read'),
    ('rol_system_admin',      'wallet.provider_withdrawals.read'),
    ('rol_system_operations', 'wallet.provider_subwallet.read'),
    ('rol_system_operations', 'wallet.provider_withdrawals.read'),
    ('rol_system_compliance', 'wallet.read'),
    ('rol_system_compliance', 'wallet.transactions.read'),
    ('rol_system_analyst',    'wallet.read')
ON CONFLICT DO NOTHING;
