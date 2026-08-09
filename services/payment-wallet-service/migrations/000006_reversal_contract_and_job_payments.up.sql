-- Migration 000006: durable reversal contract and authoritative job payments.
--
-- Two authoritative aggregates are added here.
--
-- finance_reversals is the durable contract for reversing an already posted
-- entry. The original posting is never edited: completion produces a separate
-- compensating journal and the reversal row records exactly which one. Every
-- ambiguity-producing state the executor can observe has an explicit name, so
-- an interrupted process resumes from stored state rather than from a guess.
--
-- finance_job_payments is Wallet's authoritative record that a specific job was
-- paid for. It exists so another service can verify payment for a dispatch
-- request or canonical trip without ever seeing a balance, a journal line or a
-- provider credential.

-- ---------------------------------------------------------------------------
-- Durable reversals
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_reversals (
    id                           text PRIMARY KEY,
    organization_id              text NOT NULL,
    original_journal_id          text NOT NULL REFERENCES finance_journals(id),
    original_reference           text NOT NULL DEFAULT '',
    reason_code                  text NOT NULL DEFAULT '',
    reason                       text NOT NULL DEFAULT '',
    amount_minor                 bigint NOT NULL CHECK (amount_minor > 0),
    currency                     char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    request_fingerprint          text NOT NULL,
    idempotency_key              text NOT NULL,
    expected_version             bigint NOT NULL DEFAULT 0,
    status                       text NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
                                     'REQUESTED', 'PENDING_APPROVAL', 'APPROVED', 'PROCESSING',
                                     'UNKNOWN', 'COMPLETED', 'FAILED_RETRYABLE',
                                     'FAILED_PERMANENT', 'REJECTED', 'RECONCILING', 'DEAD_LETTER')),
    external_operation_reference text NOT NULL DEFAULT '',
    attempts                     int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at              timestamptz NOT NULL DEFAULT now(),
    lease_owner                  text,
    lease_expires_at             timestamptz,
    correlation_id               text NOT NULL DEFAULT '',
    causation_id                 text NOT NULL DEFAULT '',
    created_by_actor_type        text NOT NULL DEFAULT '',
    created_by_actor_id          text NOT NULL DEFAULT '',
    approved_by_actor_type       text NOT NULL DEFAULT '',
    approved_by_actor_id         text NOT NULL DEFAULT '',
    approval_reference           text NOT NULL DEFAULT '',
    approval_policy_version      int NOT NULL DEFAULT 0,
    approved_at                  timestamptz,
    resulting_journal_id         text REFERENCES finance_journals(id),
    safe_error_code              text NOT NULL DEFAULT '',
    version                      bigint NOT NULL DEFAULT 1,
    created_at                   timestamptz NOT NULL DEFAULT now(),
    updated_at                   timestamptz NOT NULL DEFAULT now(),
    completed_at                 timestamptz
);

-- A repeat of an identical request must return the original result rather than
-- create a second reversal.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_reversals_idempotency
    ON finance_reversals (organization_id, idempotency_key);
-- A reversal can never execute twice: one compensating journal belongs to at
-- most one reversal, enforced by the database rather than by worker discipline.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_reversals_resulting_journal
    ON finance_reversals (resulting_journal_id) WHERE resulting_journal_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_finance_reversals_original
    ON finance_reversals (original_journal_id, status);
CREATE INDEX IF NOT EXISTS idx_finance_reversals_due
    ON finance_reversals (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_finance_reversals_lease
    ON finance_reversals (lease_expires_at) WHERE lease_owner IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_finance_reversals_org
    ON finance_reversals (organization_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Authoritative job payments
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS finance_job_payments (
    id                        text PRIMARY KEY,
    organization_id           text NOT NULL,
    source_service            text NOT NULL,
    dispatch_request_reference text NOT NULL DEFAULT '',
    trip_reference            text NOT NULL DEFAULT '',
    booking_reference         text NOT NULL DEFAULT '',
    payment_reference         text NOT NULL DEFAULT '',
    amount_minor              bigint NOT NULL CHECK (amount_minor > 0),
    currency                  char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status                    text NOT NULL CHECK (status IN (
                                  'PENDING', 'AUTHORIZED', 'PAID', 'FAILED',
                                  'REFUNDED', 'PARTIALLY_REFUNDED', 'REVERSED', 'UNKNOWN')),
    paid_at                   timestamptz,
    journal_id                text REFERENCES finance_journals(id),
    refund_state              text NOT NULL DEFAULT 'NONE' CHECK (refund_state IN (
                                  'NONE', 'REQUESTED', 'PROCESSING', 'COMPLETED', 'FAILED')),
    reversal_state            text NOT NULL DEFAULT 'NONE' CHECK (reversal_state IN (
                                  'NONE', 'REQUESTED', 'PROCESSING', 'COMPLETED', 'FAILED')),
    operation_reference       text NOT NULL DEFAULT '',
    idempotency_key           text NOT NULL DEFAULT '',
    request_fingerprint       text NOT NULL DEFAULT '',
    correlation_id            text NOT NULL DEFAULT '',
    version                   bigint NOT NULL DEFAULT 1,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    -- A verification that cannot be found by any safe reference is useless.
    CONSTRAINT finance_job_payments_has_reference CHECK (
        dispatch_request_reference <> '' OR trip_reference <> ''
        OR booking_reference <> '' OR payment_reference <> '')
);

-- Every supported search key resolves to at most one record inside a tenant.
-- Without this, "not found" and "ambiguous" would be indistinguishable to the
-- caller, and an ambiguous payment must never authorize money movement.
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_job_payments_dispatch
    ON finance_job_payments (organization_id, dispatch_request_reference)
    WHERE dispatch_request_reference <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_job_payments_trip
    ON finance_job_payments (organization_id, trip_reference)
    WHERE trip_reference <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_job_payments_booking
    ON finance_job_payments (organization_id, booking_reference)
    WHERE booking_reference <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_job_payments_payment_reference
    ON finance_job_payments (organization_id, payment_reference)
    WHERE payment_reference <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_job_payments_idempotency
    ON finance_job_payments (organization_id, idempotency_key)
    WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_finance_job_payments_org
    ON finance_job_payments (organization_id, status, updated_at DESC);

-- ---------------------------------------------------------------------------
-- Permissions and approval policy
-- ---------------------------------------------------------------------------

INSERT INTO finance_permissions (code, description, sensitive) VALUES
    ('finance.reversal.request', 'Request a durable reversal of a posted entry', true),
    ('finance.reversal.approve', 'Approve or reject a requested reversal',       true),
    ('finance.job_payment.view', 'Verify authoritative job payment state',       false)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, sensitive = EXCLUDED.sensitive;

-- Fleet initiates reversals and verifies job payments; it can neither approve a
-- reversal nor post the compensating journal itself.
INSERT INTO finance_role_permissions (role_id, permission_code) VALUES
    ('frol_finance_operator', 'finance.reversal.request'),
    ('frol_finance_operator', 'finance.job_payment.view'),
    ('frol_finance_viewer',   'finance.job_payment.view'),
    ('frol_finance_approver', 'finance.reversal.approve'),
    ('frol_finance_admin',    'finance.reversal.request'),
    ('frol_finance_admin',    'finance.reversal.approve'),
    ('frol_finance_admin',    'finance.job_payment.view')
ON CONFLICT DO NOTHING;

-- Reversal approval is governed by an explicit versioned policy so that an
-- approval decision can always be read back against the rule that allowed it.
-- Self-approval is off: the requester cannot approve their own reversal.
INSERT INTO finance_approval_policies
    (id, scope_id, operation, minimum_approvals, self_approval_allowed, policy_version)
VALUES
    ('fapp_platform_reversal_v1', 'PLATFORM', 'REVERSAL_APPROVAL', 1, false, 1)
ON CONFLICT (scope_id, operation, policy_version) DO NOTHING;
