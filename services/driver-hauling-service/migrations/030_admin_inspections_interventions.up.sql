-- Hauling-owned administrative interventions (Admin Section 10).
-- Inspection cases, verified inspection-to-Media associations, legacy
-- inspection reference mappings, cargo suitability/remediation decisions and
-- the per-subject administrative standing version. Service-local: Media
-- object ids are opaque references and never foreign keys into another owner.

CREATE OR REPLACE FUNCTION hauling_reject_admin_intervention_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Hauling administrative intervention evidence is append-only'; END $$;

-- Administrative standing version per enrolled subject. version fences every
-- SUBJECT-scoped owner command; the real effect lives in the domain tables.
CREATE TABLE IF NOT EXISTS hauling_admin_intervention_heads (
 subject_kind text NOT NULL CHECK (subject_kind IN ('trucks','providers','booking-cargo-inputs','recoveries','product-eligibility')),
 subject_id text NOT NULL CHECK (length(subject_id) BETWEEN 1 AND 128),
 version bigint NOT NULL CHECK (version > 0),
 state text NOT NULL CHECK (state ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 organization_id text NOT NULL DEFAULT '',
 last_decision_id uuid,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (subject_kind, subject_id)
);

-- Inspection case. id is the command case_ref. Ownership (organization) is
-- copied from the truck's authoritative organization binding at recording.
CREATE TABLE IF NOT EXISTS hauling_admin_inspections (
 id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$'),
 truck_id uuid NOT NULL REFERENCES trucks(id),
 provider_id uuid NOT NULL REFERENCES truck_providers(id),
 organization_id text NOT NULL CHECK (organization_id <> ''),
 inspection_kind text NOT NULL CHECK (inspection_kind IN ('ROADWORTHINESS','CARGO_BAY','REFRIGERATION','LIFTING_EQUIPMENT','SAFETY_EQUIPMENT')),
 performed_on date NOT NULL,
 state text NOT NULL CHECK (state IN ('RECORDED','CONCLUDED')),
 conclusion text CHECK (conclusion IN ('PASS','FAIL','CONDITIONAL')),
 valid_until date,
 restriction_id uuid,
 evidence_count integer NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
 version bigint NOT NULL CHECK (version > 0),
 recorded_decision_id uuid NOT NULL UNIQUE,
 concluded_decision_id uuid UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((state = 'CONCLUDED') = (conclusion IS NOT NULL)),
 CHECK ((state = 'CONCLUDED') = (concluded_decision_id IS NOT NULL)),
 CHECK (valid_until IS NULL OR valid_until >= performed_on),
 CHECK (conclusion IS DISTINCT FROM 'FAIL' OR valid_until IS NULL),
 CHECK ((conclusion = 'FAIL') = (restriction_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS hauling_admin_inspections_truck ON hauling_admin_inspections(truck_id, created_at DESC);

-- Identity, ownership and a concluded result are immutable; only the forward
-- RECORDED -> CONCLUDED transition and evidence counting may change a row.
CREATE OR REPLACE FUNCTION hauling_guard_admin_inspection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' THEN RAISE EXCEPTION 'Hauling inspection cases are never deleted'; END IF;
 IF NEW.id <> OLD.id OR NEW.truck_id <> OLD.truck_id OR NEW.provider_id <> OLD.provider_id OR NEW.organization_id <> OLD.organization_id
  OR NEW.inspection_kind <> OLD.inspection_kind OR NEW.performed_on <> OLD.performed_on OR NEW.recorded_decision_id <> OLD.recorded_decision_id
  OR NEW.created_at <> OLD.created_at OR NEW.version <> OLD.version + 1 OR NEW.evidence_count < OLD.evidence_count THEN
  RAISE EXCEPTION 'Hauling inspection identity, ownership and version are immutable';
 END IF;
 IF OLD.state = 'CONCLUDED' AND (NEW.state <> 'CONCLUDED' OR NEW.conclusion IS DISTINCT FROM OLD.conclusion OR NEW.valid_until IS DISTINCT FROM OLD.valid_until
  OR NEW.restriction_id IS DISTINCT FROM OLD.restriction_id OR NEW.concluded_decision_id IS DISTINCT FROM OLD.concluded_decision_id) THEN
  RAISE EXCEPTION 'Hauling inspection conclusion is immutable';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS hauling_admin_inspections_guard ON hauling_admin_inspections;
CREATE TRIGGER hauling_admin_inspections_guard BEFORE UPDATE OR DELETE ON hauling_admin_inspections FOR EACH ROW EXECUTE FUNCTION hauling_guard_admin_inspection();
DROP TRIGGER IF EXISTS hauling_admin_inspections_no_truncate ON hauling_admin_inspections;
CREATE TRIGGER hauling_admin_inspections_no_truncate BEFORE TRUNCATE ON hauling_admin_inspections FOR EACH STATEMENT EXECUTE FUNCTION hauling_reject_admin_intervention_mutation();

CREATE TABLE IF NOT EXISTS hauling_admin_inspection_history (
 id bigserial PRIMARY KEY,
 inspection_id text NOT NULL REFERENCES hauling_admin_inspections(id),
 version bigint NOT NULL CHECK (version > 0),
 action text NOT NULL CHECK (action ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 state text NOT NULL,
 conclusion text,
 decision_id uuid NOT NULL UNIQUE,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (inspection_id, version)
);
DROP TRIGGER IF EXISTS hauling_admin_inspection_history_immutable ON hauling_admin_inspection_history;
CREATE TRIGGER hauling_admin_inspection_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON hauling_admin_inspection_history FOR EACH STATEMENT EXECUTE FUNCTION hauling_reject_admin_intervention_mutation();

-- Hauling attests which Media objects belong to which inspection. Media grants
-- evidence access only through hauling's case authority over that inspection.
CREATE TABLE IF NOT EXISTS hauling_admin_inspection_evidence (
 inspection_id text NOT NULL REFERENCES hauling_admin_inspections(id),
 media_object_id uuid NOT NULL,
 attached_by uuid NOT NULL,
 attached_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (inspection_id, media_object_id)
);
DROP TRIGGER IF EXISTS hauling_admin_inspection_evidence_immutable ON hauling_admin_inspection_evidence;
CREATE TRIGGER hauling_admin_inspection_evidence_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON hauling_admin_inspection_evidence FOR EACH STATEMENT EXECUTE FUNCTION hauling_reject_admin_intervention_mutation();

-- Exact mapping of a truck's recorded opaque legacy inspection_ref to one
-- CONCLUDED modern inspection of the same truck. One mapping per legacy ref.
CREATE TABLE IF NOT EXISTS hauling_admin_legacy_inspection_mappings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 truck_id uuid NOT NULL REFERENCES trucks(id),
 legacy_inspection_ref text NOT NULL CHECK (legacy_inspection_ref <> ''),
 inspection_id text NOT NULL REFERENCES hauling_admin_inspections(id),
 eligibility_id uuid NOT NULL,
 decision_id uuid NOT NULL UNIQUE,
 mapped_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (truck_id, legacy_inspection_ref)
);
DROP TRIGGER IF EXISTS hauling_admin_legacy_inspection_mappings_immutable ON hauling_admin_legacy_inspection_mappings;
CREATE TRIGGER hauling_admin_legacy_inspection_mappings_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON hauling_admin_legacy_inspection_mappings FOR EACH STATEMENT EXECUTE FUNCTION hauling_reject_admin_intervention_mutation();

-- Current cargo suitability decision per (booking, truck). UNSUITABLE and a
-- PENDING remediation block assigning that truck to that booking; SUITABLE
-- never authorizes an assignment on its own (matching re-evaluates fit).
CREATE TABLE IF NOT EXISTS hauling_cargo_suitability_decisions (
 booking_id uuid NOT NULL REFERENCES haulage_bookings(id),
 truck_id uuid NOT NULL REFERENCES trucks(id),
 decision text NOT NULL CHECK (decision IN ('SUITABLE','UNSUITABLE','SUITABLE_WITH_REMEDIATION')),
 remediation_state text NOT NULL CHECK (remediation_state IN ('NONE','PENDING','COMPLETED')),
 inputs_fingerprint text NOT NULL CHECK (inputs_fingerprint ~ '^[0-9a-f]{64}$'),
 capability_version bigint NOT NULL,
 fit_eligible boolean NOT NULL,
 fit_reason text NOT NULL DEFAULT '',
 evidence_ref text NOT NULL DEFAULT '',
 decision_id uuid NOT NULL UNIQUE,
 version bigint NOT NULL CHECK (version > 0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (booking_id, truck_id),
 CHECK ((decision = 'SUITABLE_WITH_REMEDIATION') = (remediation_state <> 'NONE')),
 CHECK (decision <> 'SUITABLE' OR fit_eligible)
);
CREATE UNIQUE INDEX IF NOT EXISTS hauling_cargo_suitability_one_pending ON hauling_cargo_suitability_decisions(booking_id) WHERE remediation_state = 'PENDING';
CREATE INDEX IF NOT EXISTS hauling_cargo_suitability_blocking ON hauling_cargo_suitability_decisions(booking_id, truck_id) WHERE decision = 'UNSUITABLE' OR remediation_state = 'PENDING';

CREATE TABLE IF NOT EXISTS hauling_cargo_suitability_history (
 id bigserial PRIMARY KEY,
 booking_id uuid NOT NULL,
 truck_id uuid NOT NULL,
 action text NOT NULL CHECK (action ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 decision text NOT NULL,
 remediation_state text NOT NULL,
 inputs_fingerprint text NOT NULL,
 fit_eligible boolean NOT NULL,
 fit_reason text NOT NULL DEFAULT '',
 evidence_ref text NOT NULL DEFAULT '',
 decision_id uuid NOT NULL UNIQUE,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS hauling_cargo_suitability_history_immutable ON hauling_cargo_suitability_history;
CREATE TRIGGER hauling_cargo_suitability_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON hauling_cargo_suitability_history FOR EACH STATEMENT EXECUTE FUNCTION hauling_reject_admin_intervention_mutation();
