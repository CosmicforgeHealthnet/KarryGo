-- Section 28: tamper-evident audit chaining. Adds a hash chain over the
-- already-append-only fleet_partner_audit_records table (the
-- BEFORE UPDATE OR DELETE trigger from migration 000001 is untouched and
-- still the primary defense; chaining adds the ability to DETECT a
-- modification that somehow bypassed it -- a superuser UPDATE, a restored
-- backup missing rows, etc).
--
-- Partitioned by organization + calendar month (chain_partition =
-- "<organizationId>:<YYYY-MM>") rather than one global chain, so
-- verification and concurrent-append locking never serialize across
-- unrelated organizations or grow one unbounded chain forever.
--
-- Legacy boundary: existing rows keep hash_version = 0 (the DEFAULT) and
-- NULL chain_partition/previous_entry_hash/entry_hash -- they predate
-- chaining and are not, and cannot retroactively be made, part of a chain.
-- hash_version = 1 is the first real chaining scheme; a verification pass
-- treats every hash_version = 0 row as an unverifiable-but-expected legacy
-- entry, never as tamper evidence.
-- causation_id/approval_reference/idempotency_key_reference/step_up_reference
-- close a real gap alongside chaining: Section 24's canonical audit field
-- list includes them, but the original migration 000001 table only ever had
-- request_id/correlation_id. All four are part of the canonical hash input
-- below, so they needed to exist as real columns regardless of chaining.
-- Nullable and additive: existing call sites (audit.Recorder.Record's ~30
-- callers) are unaffected and simply leave them unset; legalhold and
-- security-settings, the two features that already carry a step-up
-- reference through their usecase inputs, populate step_up_reference for
-- real -- see internal/platform/audit's own doc comment for which callers
-- populate which fields today.
-- chain_sequence is a bigserial, not a derivative of occurred_at: Postgres
-- assigns it strictly in commit order regardless of wall-clock resolution,
-- so it -- not occurred_at -- is what VerifyChain orders by to reconstruct
-- true append order. Two entries appended a wall-clock millisecond apart
-- (a real possibility under concurrent load even serialized by the
-- per-partition advisory lock in Record) still get a correct, unambiguous
-- order this way; ordering by occurred_at alone could tie and then fall
-- back to id, which is random, not insertion-order.
ALTER TABLE fleet_partner_audit_records
    ADD COLUMN IF NOT EXISTS causation_id text,
    ADD COLUMN IF NOT EXISTS approval_reference text,
    ADD COLUMN IF NOT EXISTS idempotency_key_reference text,
    ADD COLUMN IF NOT EXISTS step_up_reference text,
    ADD COLUMN IF NOT EXISTS chain_partition text,
    ADD COLUMN IF NOT EXISTS previous_entry_hash text,
    ADD COLUMN IF NOT EXISTS entry_hash text,
    ADD COLUMN IF NOT EXISTS hash_version integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS chain_sequence bigserial;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_audit_chain_partition
    ON fleet_partner_audit_records (chain_partition, chain_sequence)
    WHERE chain_partition IS NOT NULL;

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('security.audit_chain.verify', 'security', 'Verify audit hash chain integrity', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'security.audit_chain.verify' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
ON CONFLICT DO NOTHING;
