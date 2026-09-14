-- CMP-027: the durable record of which compensation components were already
-- paid with a completed job's own money.
--
-- Payroll and managed work settlement are computed by different code, at
-- different times, from different tables. Without a shared record of what has
-- already been paid, both will eventually decide the same commission is theirs
-- to pay -- and nothing downstream can detect it, because a duplicated
-- component looks exactly like a legitimate second one.
--
-- This table is that record. Payroll consults it and skips what it names.
CREATE TABLE IF NOT EXISTS fleet_partner_work_settled_components (
    id                                text PRIMARY KEY,
    organization_id                   text NOT NULL,
    membership_id                     text NOT NULL,

    source_service                    text NOT NULL,
    source_job_id                     text NOT NULL,
    -- The Fleet-side assignment this work belongs to. Payroll iterates
    -- assignments, not execution-service job ids, so without this column it
    -- could not match its own rows against what was already paid.
    source_assignment_id              text NOT NULL DEFAULT '',

    completed_work_id                 text NOT NULL,
    compensation_resolution_id        text NOT NULL,
    settlement_instruction_id         text NOT NULL DEFAULT '',
    wallet_posting_operation_id       text NOT NULL DEFAULT '',

    rule_id                           text NOT NULL,
    component                         text NOT NULL,
    settlement_level                  text NOT NULL CHECK (settlement_level IN ('WORK', 'PAYROLL')),
    settlement_strategy               text NOT NULL DEFAULT '',

    amount_minor                      bigint NOT NULL CHECK (amount_minor >= 0),
    currency                          text NOT NULL,

    settled_at                        timestamptz NOT NULL DEFAULT now(),
    created_at                        timestamptz NOT NULL DEFAULT now()
);

-- One component of one job is paid once. This is the constraint that makes a
-- replayed resolution harmless rather than a second payment record.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fp_work_settled_component
    ON fleet_partner_work_settled_components (organization_id, source_service, source_job_id, rule_id);

-- Payroll's own lookup: everything this membership was already paid inside a
-- period.
CREATE INDEX IF NOT EXISTS idx_fp_work_settled_membership_period
    ON fleet_partner_work_settled_components (organization_id, membership_id, settled_at);

CREATE INDEX IF NOT EXISTS idx_fp_work_settled_resolution
    ON fleet_partner_work_settled_components (compensation_resolution_id);

-- Payroll's matching key: it walks completed assignments, so it must be able
-- to ask "was this assignment's commission already paid?" directly.
CREATE INDEX IF NOT EXISTS idx_fp_work_settled_assignment
    ON fleet_partner_work_settled_components (organization_id, source_assignment_id, rule_id);
