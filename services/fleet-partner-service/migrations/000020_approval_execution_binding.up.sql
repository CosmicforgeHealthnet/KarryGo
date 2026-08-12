-- WP9.2: bind approvals to exactly what was approved.
--
-- This migration EXTENDS the approval framework created in 000006. It does not
-- introduce a second one. The tables, the append-only decision log and the
-- open-request uniqueness all stay; what they were missing is the machinery
-- that makes an approval mean something at execution time:
--
--   * INVALIDATED — an approval can now be killed without pretending it was
--     rejected by a human or that it merely expired. A changed payload and a
--     changed subject are different facts from "nobody approved it".
--   * expected_subject_version — an approval is taken against a subject as it
--     was. If the subject moved on, the approval no longer describes reality.
--   * payload fingerprint and version — 000006 already stored a fingerprint;
--     execution now has to present it, and the version pins which normalization
--     produced it.
--   * execution reference and executed_at — proof that this approval was spent,
--     once, on one operation.
--   * policy version — which rules were in force when the approval was granted.
--
-- Nothing here rewrites a historical decision: the decisions table is untouched
-- except to add the request version each decision was cast against.

-- ---------------------------------------------------------------------------
-- Policies
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_approval_policies
    -- Separation of duties as an explicit, auditable policy field rather than
    -- an implication of requester_may_approve. The two are different rules:
    -- one is about the requester, the other about anyone who already acted on
    -- this request in another capacity.
    ADD COLUMN IF NOT EXISTS separation_of_duties boolean NOT NULL DEFAULT true,
    -- Links an approval policy to the eligibility policy governing the same
    -- operation, so one publication changes both consistently.
    ADD COLUMN IF NOT EXISTS eligibility_policy_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS requires_current_eligibility boolean NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- Requests
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_approval_requests
    -- The subject version the approval was granted against. 0 means the
    -- requester asserted no version, which policy may still refuse.
    ADD COLUMN IF NOT EXISTS expected_subject_version bigint NOT NULL DEFAULT 0
        CHECK (expected_subject_version >= 0),
    -- Which normalization produced request_fingerprint. Without it, changing
    -- the hashing rules would silently invalidate or silently revalidate every
    -- open approval.
    ADD COLUMN IF NOT EXISTS payload_version integer NOT NULL DEFAULT 1
        CHECK (payload_version >= 1),
    -- The policy version in force when the request was raised.
    ADD COLUMN IF NOT EXISTS policy_version bigint NOT NULL DEFAULT 1
        CHECK (policy_version >= 1),
    -- Copied from the policy at request time so a later policy publication
    -- cannot retroactively change how many approvals this request needed.
    ADD COLUMN IF NOT EXISTS minimum_approvals integer NOT NULL DEFAULT 1
        CHECK (minimum_approvals >= 1),
    ADD COLUMN IF NOT EXISTS eligible_role_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS self_approval_allowed boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS separation_of_duties boolean NOT NULL DEFAULT true,
    -- The eligibility decision that authorized raising this request.
    ADD COLUMN IF NOT EXISTS eligibility_decision_id text,
    -- Where the approved operation actually happened. Set exactly once.
    ADD COLUMN IF NOT EXISTS execution_reference text,
    ADD COLUMN IF NOT EXISTS invalidated_at timestamptz,
    ADD COLUMN IF NOT EXISTS invalidation_reason text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS branch_id text,
    ADD COLUMN IF NOT EXISTS idempotency_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '';

-- INVALIDATED joins the state set. The constraint is replaced rather than
-- added to, because the original was an unnamed inline CHECK.
ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS fleet_partner_approval_requests_status_check;
ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_request_status;
ALTER TABLE fleet_partner_approval_requests
    ADD CONSTRAINT chk_fleet_partner_approval_request_status
    CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED', 'CANCELLED',
                      'INVALIDATED', 'EXECUTED'));

-- Execution is terminal and must be evidenced. A row claiming EXECUTED with no
-- reference and no timestamp is an assertion nobody can check.
ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_execution_evidence;
ALTER TABLE fleet_partner_approval_requests
    ADD CONSTRAINT chk_fleet_partner_approval_execution_evidence
    CHECK (status <> 'EXECUTED'
           OR (execution_reference IS NOT NULL AND execution_reference <> ''
               AND executed_at IS NOT NULL));

-- Likewise an INVALIDATED row must say why.
ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_invalidation_reason;
ALTER TABLE fleet_partner_approval_requests
    ADD CONSTRAINT chk_fleet_partner_approval_invalidation_reason
    CHECK (status <> 'INVALIDATED'
           OR (invalidated_at IS NOT NULL AND invalidation_reason <> ''));

-- One approval per execution. This is the exactly-once guarantee at the schema
-- level: a retried execute that reuses the reference collides instead of
-- spending a second approval on the same operation.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_execution_reference
    ON fleet_partner_approval_requests (organization_id, execution_reference)
    WHERE execution_reference IS NOT NULL;

-- Idempotent request creation within the tenant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_request_idempotency
    ON fleet_partner_approval_requests (organization_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_requests_branch
    ON fleet_partner_approval_requests (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_requests_subject
    ON fleet_partner_approval_requests (organization_id, resource_type, resource_id, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_requests_fingerprint
    ON fleet_partner_approval_requests (organization_id, request_fingerprint);

-- ---------------------------------------------------------------------------
-- Decisions
-- ---------------------------------------------------------------------------

-- Which version of the request a decision was cast against. The decisions
-- table stays append-only and immutable; this column is part of what each new
-- decision records, not a rewrite of the old ones.
ALTER TABLE fleet_partner_approval_decisions
    ADD COLUMN IF NOT EXISTS request_version bigint NOT NULL DEFAULT 1
        CHECK (request_version >= 1),
    -- The fingerprint the approver saw. If execution later presents a
    -- different one, the decision plainly did not cover it.
    ADD COLUMN IF NOT EXISTS approved_fingerprint text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS actor_role_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_decisions_request
    ON fleet_partner_approval_decisions (approval_request_id, decided_at);

-- One approver may not both request and decide when separation of duties is
-- on; that is enforced in the use case, where the requester is known. What the
-- schema can guarantee is that a single approver never counts twice, which the
-- 000006 unique index already does per step. This adds the per-request form,
-- so a multi-step policy cannot be satisfied by the same person twice.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_decision_per_request
    ON fleet_partner_approval_decisions (approval_request_id, approver_id);

-- ---------------------------------------------------------------------------
-- Executions
-- ---------------------------------------------------------------------------

-- The durable record of spending an approval. Separate from the request so the
-- attempt is recorded even when the protected operation ultimately fails, and
-- so a retry can be resolved to the same outcome instead of re-running.
CREATE TABLE IF NOT EXISTS fleet_partner_approval_executions (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    approval_request_id text NOT NULL REFERENCES fleet_partner_approval_requests (id) ON DELETE RESTRICT,
    -- The fingerprint presented at execution. Stored, not recomputed, so a
    -- mismatch is visible afterwards rather than only at the moment it failed.
    presented_fingerprint text NOT NULL,
    approved_fingerprint  text NOT NULL,
    expected_subject_version bigint NOT NULL DEFAULT 0,
    observed_subject_version bigint NOT NULL DEFAULT 0,
    execution_reference text NOT NULL,
    outcome             text NOT NULL
                        CHECK (outcome IN ('EXECUTED', 'REJECTED', 'UNKNOWN')),
    safe_reason_code    text NOT NULL DEFAULT '',
    actor_type          text NOT NULL,
    actor_id            text NOT NULL,
    idempotency_key     text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    causation_id        text NOT NULL DEFAULT '',
    occurred_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_fleet_partner_approval_execution
        UNIQUE (organization_id, execution_reference)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_executions_request
    ON fleet_partner_approval_executions (approval_request_id, occurred_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_execution_idempotency
    ON fleet_partner_approval_executions (organization_id, idempotency_key)
    WHERE idempotency_key <> '';

DROP TRIGGER IF EXISTS trg_fleet_partner_approval_executions_append_only
    ON fleet_partner_approval_executions;
CREATE TRIGGER trg_fleet_partner_approval_executions_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_approval_executions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Default approval policy versions
-- ---------------------------------------------------------------------------

-- These mirror the eligibility defaults from 000019 for the operations that
-- require a human decision. They are seeded per organization by the service
-- when an organization is created; no platform-wide row is inserted here
-- because fleet_partner_approval_policies.organization_id is NOT NULL and a
-- fabricated tenant would be worse than an absent default.
