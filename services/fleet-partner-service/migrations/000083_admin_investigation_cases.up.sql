-- Fleet-owned standalone investigation cases (Section 10).
--
-- A case is opened only through the admin command executor against an existing
-- Fleet shipment, private assignment or execution link. The owning organization
-- (Fleet's canonical tenant) is copied from that subject's authoritative row at
-- open time and can never be supplied or changed by a caller.
--
-- State machine: OPEN -> FINDING_RECORDED -> CLOSED. The version is the
-- case's own administrative version fenced by the shared command ledger.
CREATE TABLE IF NOT EXISTS fleet_partner_admin_investigations (
    case_ref              text PRIMARY KEY CHECK (length(case_ref) BETWEEN 1 AND 128),
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    subject_kind          text NOT NULL CHECK (subject_kind IN ('shipments', 'private-assignments', 'execution-links')),
    subject_id            text NOT NULL,
    shipment_id           text REFERENCES fleet_partner_shipments (id) ON DELETE RESTRICT,
    private_assignment_id text REFERENCES fleet_partner_private_assignments (id) ON DELETE RESTRICT,
    execution_link_id     text REFERENCES fleet_partner_private_execution_operations (id) ON DELETE RESTRICT,
    category              text NOT NULL CHECK (category IN ('CARGO_DAMAGE', 'CARGO_LOSS', 'SAFETY', 'CONDUCT', 'SERVICE_FAILURE')),
    severity              text NOT NULL CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    occurred_on           date NOT NULL,
    state                 text NOT NULL CHECK (state IN ('OPEN', 'FINDING_RECORDED', 'CLOSED')),
    finding               text CHECK (finding IN ('SUBSTANTIATED', 'UNSUBSTANTIATED', 'INCONCLUSIVE')),
    finding_note          text CHECK (finding_note IS NULL OR length(finding_note) <= 280),
    closure               text CHECK (closure IN ('RESOLVED', 'REFERRED_TO_OWNER', 'NO_FURTHER_ACTION')),
    version               bigint NOT NULL CHECK (version > 0),
    opened_by             text NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    -- Exactly the one subject column named by subject_kind carries subject_id.
    CONSTRAINT fleet_partner_admin_investigation_subject_ck CHECK (
        (subject_kind = 'shipments' AND shipment_id = subject_id AND private_assignment_id IS NULL AND execution_link_id IS NULL)
     OR (subject_kind = 'private-assignments' AND private_assignment_id = subject_id AND shipment_id IS NULL AND execution_link_id IS NULL)
     OR (subject_kind = 'execution-links' AND execution_link_id = subject_id AND shipment_id IS NULL AND private_assignment_id IS NULL)),
    -- A finding exists exactly once the case left OPEN; a closure only when CLOSED.
    CONSTRAINT fleet_partner_admin_investigation_state_ck CHECK (
        (state = 'OPEN' AND finding IS NULL AND closure IS NULL)
     OR (state = 'FINDING_RECORDED' AND finding IS NOT NULL AND closure IS NULL)
     OR (state = 'CLOSED' AND finding IS NOT NULL AND closure IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS fleet_partner_admin_investigation_subject ON fleet_partner_admin_investigations (subject_kind, subject_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS fleet_partner_admin_investigation_org ON fleet_partner_admin_investigations (organization_id, state);

-- Append-only transition history: one row per committed command decision.
CREATE TABLE IF NOT EXISTS fleet_partner_admin_investigation_history (
    decision_id     uuid PRIMARY KEY,
    case_ref        text NOT NULL REFERENCES fleet_partner_admin_investigations (case_ref) ON DELETE RESTRICT,
    organization_id text NOT NULL,
    action          text NOT NULL CHECK (action IN ('OPEN_INVESTIGATION', 'RECORD_FINDING', 'CLOSE_INVESTIGATION')),
    prior_state     text NOT NULL,
    resulting_state text NOT NULL,
    version         bigint NOT NULL CHECK (version > 0),
    staff_ref       text NOT NULL,
    reason_code     text NOT NULL,
    params          jsonb NOT NULL DEFAULT '{}'::jsonb,
    approval_id     text,
    recorded_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (case_ref, version)
);
CREATE OR REPLACE FUNCTION reject_fleet_partner_admin_investigation_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Fleet investigation history is append-only and cases cannot be deleted'; END $$;
DROP TRIGGER IF EXISTS fleet_partner_admin_investigation_history_immutable ON fleet_partner_admin_investigation_history;
CREATE TRIGGER fleet_partner_admin_investigation_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON fleet_partner_admin_investigation_history FOR EACH STATEMENT EXECUTE FUNCTION reject_fleet_partner_admin_investigation_mutation();
DROP TRIGGER IF EXISTS fleet_partner_admin_investigation_undeletable ON fleet_partner_admin_investigations;
CREATE TRIGGER fleet_partner_admin_investigation_undeletable BEFORE DELETE OR TRUNCATE ON fleet_partner_admin_investigations FOR EACH STATEMENT EXECUTE FUNCTION reject_fleet_partner_admin_investigation_mutation();
