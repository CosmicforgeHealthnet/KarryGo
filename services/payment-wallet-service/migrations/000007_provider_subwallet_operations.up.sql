-- Section 13 authority: provider sub-wallet ownership metadata, company
-- authorized provider allocations, and provider withdrawals.
--
-- This migration is additive. It does NOT create a second ledger: every value
-- movement introduced here posts through finance_journals/finance_journal_lines
-- exactly like the pre-existing flows, and the tables below are operation
-- records that point at those journals.

-- ---------------------------------------------------------------------------
-- Provider sub-wallet ownership metadata
-- ---------------------------------------------------------------------------
--
-- The existing finance_earning_accounts row already IS the provider financial
-- account. What it lacked was the corporate ownership semantics Section 13
-- requires: which organization wallet funds it, which provider identity owns
-- it, which organization membership it belongs to, and which service category
-- it earns for. Those are added here rather than in a parallel account model,
-- because two provider account models would be two sources of truth.

ALTER TABLE finance_earning_accounts
    ADD COLUMN IF NOT EXISTS organization_wallet_id      text REFERENCES finance_wallets(id),
    ADD COLUMN IF NOT EXISTS provider_id                 text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS organization_membership_id  text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_service_category   text NOT NULL DEFAULT '';

-- One active account per membership, service category and currency. The index
-- is partial on membership so pre-existing accounts (which carry no membership
-- yet) are not forced into a uniqueness class they were never designed for.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_earning_accounts_membership
    ON finance_earning_accounts (organization_id, organization_membership_id,
                                 provider_service_category, currency)
    WHERE organization_membership_id <> '' AND status <> 'CLOSED';

CREATE INDEX IF NOT EXISTS idx_finance_earning_accounts_org_wallet
    ON finance_earning_accounts (organization_wallet_id)
    WHERE organization_wallet_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_earning_accounts_provider
    ON finance_earning_accounts (organization_id, provider_id)
    WHERE provider_id <> '';

-- ---------------------------------------------------------------------------
-- Company authorized provider allocations
-- ---------------------------------------------------------------------------
--
-- An allocation moves value from the organization's own wallet to one provider
-- sub-wallet. It is deliberately a different record and a different journal
-- from a marketplace settlement: a customer payment credits the organization,
-- and only a later, separately authorized company decision moves any part of
-- it to a provider. Collapsing the two would make the provider's statement a
-- window onto the customer's fare.

CREATE TABLE IF NOT EXISTS finance_provider_allocations (
    id                         text PRIMARY KEY,
    organization_id            text NOT NULL,
    organization_wallet_id     text NOT NULL REFERENCES finance_wallets(id),
    earning_account_id         text NOT NULL REFERENCES finance_earning_accounts(id),
    provider_id                text NOT NULL DEFAULT '',
    organization_membership_id text NOT NULL DEFAULT '',
    allocation_type            text NOT NULL
                                   CHECK (allocation_type IN ('SALARY', 'WAGE', 'PER_JOB', 'ALLOWANCE',
                                                              'REIMBURSEMENT', 'BONUS', 'ADJUSTMENT_CREDIT',
                                                              'APPROVED_DEDUCTION', 'REVERSAL')),
    -- Section 15 reference types (PRIVATE_ORDER, SHIPMENT) are intentionally
    -- absent: the constraint is where a premature Section 15 reference would
    -- otherwise slip in unnoticed.
    business_reference_type    text NOT NULL
                                   CHECK (business_reference_type IN ('TRIP', 'ASSIGNMENT', 'PAYROLL_REFERENCE',
                                                                     'EXPENSE_REFERENCE', 'MANUAL_APPROVED_REFERENCE')),
    business_reference_id      text NOT NULL,
    amount_minor               bigint NOT NULL CHECK (amount_minor > 0),
    currency                   char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    reason_code                text NOT NULL,
    -- Provider-visible text. Never carries fare, margin or commission; the
    -- statement projection reads this column and nothing else for free text.
    provider_explanation       text NOT NULL DEFAULT '',
    journal_id                 text NOT NULL REFERENCES finance_journals(id),
    reverses_allocation_id     text REFERENCES finance_provider_allocations(id),
    status                     text NOT NULL DEFAULT 'POSTED'
                                   CHECK (status IN ('POSTED', 'REVERSED')),
    -- The instruction that produced this allocation, as Fleet named it.
    source_service             text NOT NULL DEFAULT '',
    source_reference           text NOT NULL DEFAULT '',
    approval_reference         text NOT NULL DEFAULT '',
    requested_by_actor_type    text NOT NULL DEFAULT 'SERVICE',
    requested_by_actor_id      text NOT NULL DEFAULT '',
    idempotency_key            text NOT NULL,
    request_fingerprint        text NOT NULL,
    correlation_id             text NOT NULL DEFAULT '',
    causation_id               text NOT NULL DEFAULT '',
    version                    bigint NOT NULL DEFAULT 1,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_provider_allocations_idempotency
    ON finance_provider_allocations (organization_id, idempotency_key);

-- One normal allocation per protected business purpose. Deductions, reversals
-- and manual references are excluded because a provider can legitimately be
-- paid more than once against the same payroll period or expense claim, while
-- a completed trip or assignment must fund exactly one per-job allocation.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_provider_allocations_business_purpose
    ON finance_provider_allocations (organization_id, earning_account_id,
                                     business_reference_type, business_reference_id, allocation_type)
    WHERE business_reference_type IN ('TRIP', 'ASSIGNMENT')
      AND allocation_type IN ('PER_JOB', 'BONUS');

CREATE INDEX IF NOT EXISTS idx_finance_provider_allocations_account
    ON finance_provider_allocations (earning_account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_finance_provider_allocations_org
    ON finance_provider_allocations (organization_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_provider_allocations_single_reversal
    ON finance_provider_allocations (reverses_allocation_id)
    WHERE reverses_allocation_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Provider withdrawals
-- ---------------------------------------------------------------------------
--
-- A withdrawal takes value out of a provider sub-wallet toward a payout
-- destination held elsewhere. Only the opaque destination reference is stored:
-- account numbers live with the payout provider.

CREATE TABLE IF NOT EXISTS finance_provider_withdrawals (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL,
    earning_account_id          text NOT NULL REFERENCES finance_earning_accounts(id),
    organization_membership_id  text NOT NULL DEFAULT '',
    provider_id                 text NOT NULL DEFAULT '',
    amount_minor                bigint NOT NULL CHECK (amount_minor > 0),
    currency                    char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    destination_reference       text NOT NULL,
    -- The policy mode Fleet asserted at request time, kept as execution
    -- evidence: a later policy change must not rewrite why this one was allowed.
    policy_mode                 text NOT NULL DEFAULT 'PROVIDER_DIRECT'
                                    CHECK (policy_mode IN ('PROVIDER_DIRECT', 'COMPANY_APPROVAL_REQUIRED',
                                                           'COMPANY_INITIATED_ONLY', 'WITHDRAWALS_DISABLED')),
    approval_reference          text NOT NULL DEFAULT '',
    status                      text NOT NULL DEFAULT 'REQUESTED'
                                    CHECK (status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED',
                                                      'PROCESSING', 'COMPLETED', 'FAILED_RETRYABLE',
                                                      'FAILED_PERMANENT', 'CANCELLED', 'DEAD_LETTER')),
    hold_journal_id             text REFERENCES finance_journals(id),
    payout_journal_id           text REFERENCES finance_journals(id),
    external_reference          text NOT NULL DEFAULT '',
    safe_error_code             text NOT NULL DEFAULT '',
    attempts                    integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at             timestamptz NOT NULL DEFAULT now(),
    lease_owner                 text,
    lease_expires_at            timestamptz,
    requested_by_actor_type     text NOT NULL DEFAULT 'SERVICE',
    requested_by_actor_id       text NOT NULL DEFAULT '',
    decided_by_actor_type       text NOT NULL DEFAULT '',
    decided_by_actor_id         text NOT NULL DEFAULT '',
    decided_at                  timestamptz,
    idempotency_key             text NOT NULL,
    request_fingerprint         text NOT NULL,
    correlation_id              text NOT NULL DEFAULT '',
    causation_id                text NOT NULL DEFAULT '',
    version                     bigint NOT NULL DEFAULT 1,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    completed_at                timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_provider_withdrawals_idempotency
    ON finance_provider_withdrawals (organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_finance_provider_withdrawals_account
    ON finance_provider_withdrawals (earning_account_id, status);

CREATE INDEX IF NOT EXISTS idx_finance_provider_withdrawals_due
    ON finance_provider_withdrawals (status, next_attempt_at)
    WHERE status IN ('APPROVED', 'PROCESSING', 'FAILED_RETRYABLE');

CREATE INDEX IF NOT EXISTS idx_finance_provider_withdrawals_lease
    ON finance_provider_withdrawals (lease_expires_at)
    WHERE lease_owner IS NOT NULL;

-- At most one open withdrawal per provider sub-wallet. Two concurrent open
-- requests against the same payable balance is how a sub-wallet gets drained
-- twice while both requests still look individually valid.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_provider_withdrawals_open
    ON finance_provider_withdrawals (earning_account_id)
    WHERE status IN ('REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'PROCESSING');

-- ---------------------------------------------------------------------------
-- Permissions and approval policy
-- ---------------------------------------------------------------------------

INSERT INTO finance_permissions (code, description, sensitive) VALUES
    ('finance.provider_allocation.post',    'Post a company authorized allocation to a provider sub-wallet', true),
    ('finance.provider_allocation.reverse', 'Reverse a posted provider allocation',                          true),
    ('finance.provider_withdrawal.request', 'Request a provider sub-wallet withdrawal',                      true),
    ('finance.provider_withdrawal.approve', 'Approve or reject a provider sub-wallet withdrawal',            true),
    ('finance.provider_withdrawal.view',    'Read provider sub-wallet withdrawals',                          false)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, sensitive = EXCLUDED.sensitive;

-- Fleet initiates allocations and withdrawal requests. It cannot approve a
-- withdrawal and it cannot reverse an allocation: both are approver authority,
-- and a service that could do either could undo its own money movement.
INSERT INTO finance_role_permissions (role_id, permission_code) VALUES
    ('frol_finance_operator', 'finance.provider_allocation.post'),
    ('frol_finance_operator', 'finance.provider_withdrawal.request'),
    ('frol_finance_operator', 'finance.provider_withdrawal.view'),
    ('frol_finance_viewer',   'finance.provider_withdrawal.view'),
    ('frol_finance_approver', 'finance.provider_withdrawal.approve'),
    ('frol_finance_approver', 'finance.provider_allocation.reverse'),
    ('frol_finance_admin',    'finance.provider_allocation.post'),
    ('frol_finance_admin',    'finance.provider_allocation.reverse'),
    ('frol_finance_admin',    'finance.provider_withdrawal.request'),
    ('frol_finance_admin',    'finance.provider_withdrawal.approve'),
    ('frol_finance_admin',    'finance.provider_withdrawal.view')
ON CONFLICT DO NOTHING;

-- Withdrawal approval is governed by an explicit versioned policy. Fleet's own
-- COMPANY_APPROVAL_REQUIRED policy sits on top of this and never replaces it:
-- an organization cannot configure its way past the platform control.
INSERT INTO finance_approval_policies
    (id, scope_id, operation, minimum_approvals, self_approval_allowed, policy_version)
VALUES
    ('fapp_platform_provider_withdrawal_v1', 'PLATFORM', 'PROVIDER_WITHDRAWAL_APPROVAL', 1, false, 1)
ON CONFLICT (scope_id, operation, policy_version) DO NOTHING;
