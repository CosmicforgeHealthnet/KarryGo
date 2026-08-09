-- WP6.1: company/provider assignment acknowledgement.
--
-- Acknowledgement answers a question acceptance cannot: did the assigned
-- provider actually SEE this work? Today the only recorded provider signal is
-- accept or decline, so an assignment that a provider never opened is
-- indistinguishable from one they opened and ignored. Operations needs that
-- distinction, and some organizations make it a departure precondition.
--
-- Acknowledgement is explicitly NOT acceptance. Nothing here can move an
-- assignment into ACCEPTED, and nothing here removes the provider's right to
-- decline afterwards — which is why the status CHECK constraint is untouched
-- and acknowledgement lives in its own columns and its own append-only table.
--
-- Additive and reversible: two ALTERs adding defaulted columns and one new
-- table.

-- 1. Acknowledgement state on the assignment ---------------------------------

ALTER TABLE fleet_partner_provider_assignments
    ADD COLUMN IF NOT EXISTS acknowledgement_state text NOT NULL DEFAULT 'NOT_ACKNOWLEDGED';

ALTER TABLE fleet_partner_provider_assignments
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_assignment_acknowledgement_state;

ALTER TABLE fleet_partner_provider_assignments
    ADD CONSTRAINT chk_fleet_partner_assignment_acknowledgement_state
    CHECK (acknowledgement_state IN ('NOT_ACKNOWLEDGED', 'ACKNOWLEDGED'));

ALTER TABLE fleet_partner_provider_assignments
    ADD COLUMN IF NOT EXISTS acknowledged_at timestamptz;

ALTER TABLE fleet_partner_provider_assignments
    ADD COLUMN IF NOT EXISTS acknowledged_by_actor_type text;

ALTER TABLE fleet_partner_provider_assignments
    ADD COLUMN IF NOT EXISTS acknowledged_by_actor_id text;

-- Operators list unacknowledged live work; the partial index keeps that scan
-- off the acknowledged majority.
CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignments_unacknowledged
    ON fleet_partner_provider_assignments (organization_id, response_deadline)
    WHERE acknowledgement_state = 'NOT_ACKNOWLEDGED'
      AND status IN ('CREATED', 'PENDING_PROVIDER_RESPONSE', 'ACCEPTED');

-- 2. Organization policy -----------------------------------------------------
--
-- Defaults to false. A policy that defaulted to true would silently block
-- departure for every existing organization the moment this migration ran.
ALTER TABLE fleet_partner_organization_settings
    ADD COLUMN IF NOT EXISTS require_assignment_acknowledgement boolean NOT NULL DEFAULT false;

-- 3. The immutable acknowledgement record ------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_assignment_acknowledgements (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    assignment_id         text NOT NULL
                          REFERENCES fleet_partner_provider_assignments (id) ON DELETE RESTRICT,
    membership_id         text NOT NULL,
    -- Opaque provider identifier owned by the execution service.
    executing_provider_id text NOT NULL,
    source_service        text NOT NULL CHECK (source_service IN ('DISPATCH', 'TAXI', 'HAULING')),
    actor_type            text NOT NULL CHECK (actor_type IN ('PROVIDER', 'ORGANIZATION_USER', 'SYSTEM')),
    actor_id              text NOT NULL,
    -- The version the caller believed it was acting on. Stored so an audit can
    -- reconstruct what the client saw, not only what the server decided.
    expected_version      bigint NOT NULL DEFAULT 0,
    idempotency_key       text NOT NULL,
    -- Hash of the meaningful inputs. The same key with a different fingerprint
    -- is a different request wearing a used key, and is rejected rather than
    -- replayed.
    request_fingerprint   text NOT NULL,
    request_id            text NOT NULL DEFAULT '',
    correlation_id        text NOT NULL DEFAULT '',
    acknowledged_at       timestamptz NOT NULL DEFAULT now(),
    created_at            timestamptz NOT NULL DEFAULT now(),
    -- Exactly one acknowledgement per assignment: a second one is a replay,
    -- never a second event.
    CONSTRAINT uq_fleet_partner_assignment_acknowledgement UNIQUE (assignment_id)
);

-- An idempotency key is scoped to the provider that minted it, so two
-- providers cannot collide and one cannot suppress the other's request.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_acknowledgement_idempotency
    ON fleet_partner_assignment_acknowledgements (membership_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_acknowledgements_org
    ON fleet_partner_assignment_acknowledgements (organization_id, acknowledged_at DESC);

-- Append-only: an acknowledgement is evidence of what a provider saw and when.
-- Rewriting it after the fact would destroy the only thing it is good for.
DROP TRIGGER IF EXISTS trg_fleet_partner_acknowledgements_append_only
    ON fleet_partner_assignment_acknowledgements;
CREATE TRIGGER trg_fleet_partner_acknowledgements_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_assignment_acknowledgements
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
