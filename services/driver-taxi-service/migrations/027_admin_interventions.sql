-- Taxi-owned administrative interventions (Admin Section 8).
--
-- Every row here is written only by the Taxi owner command executor
-- (internal/features/admininterventions) inside the same transaction as the
-- admin_owner_command_decisions row. Nothing in this file rewrites native taxi
-- records: trip status, completion, location, fares and the free-text
-- taxi_providers.license_expiry_date are never touched. The overlay is read by
-- the go-online gate, the heartbeat path and every assignment path (passenger
-- matching, goods offers, managed and private execution) through the
-- taxi_admin_online_block / taxi_admin_assignment_block functions below, so
-- there is exactly one definition of "administratively eligible".

CREATE OR REPLACE FUNCTION taxi_admin_intervention_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Taxi administrative intervention history is append-only'; END $$;

-- ─── Subject standing (providers / vehicles / independent-rides) ───────────
-- One versioned aggregate per subject. Its version is the admincommand
-- SUBJECT version fence; absent row = version 0, no restriction.
CREATE TABLE IF NOT EXISTS taxi_admin_subject_standing (
 subject_kind text NOT NULL CHECK (subject_kind IN ('providers','vehicles','independent-rides')),
 subject_id text NOT NULL,
 version bigint NOT NULL CHECK (version > 0),
 restriction text CHECK (restriction IS NULL
   OR (subject_kind = 'providers' AND restriction IN ('SUSPEND_ONLINE','NO_NEW_ASSIGNMENTS'))
   OR (subject_kind = 'vehicles' AND restriction = 'OUT_OF_SERVICE')),
 restricted_at timestamptz,
 updated_at timestamptz NOT NULL,
 CHECK ((restriction IS NULL) = (restricted_at IS NULL)),
 PRIMARY KEY (subject_kind, subject_id)
);
CREATE INDEX IF NOT EXISTS taxi_admin_subject_standing_restricted ON taxi_admin_subject_standing(subject_kind, subject_id) WHERE restriction IS NOT NULL;

-- Append-only history of every restriction/lift.
CREATE TABLE IF NOT EXISTS taxi_admin_restriction_history (
 decision_id uuid PRIMARY KEY,
 subject_kind text NOT NULL CHECK (subject_kind IN ('providers','vehicles')),
 subject_id text NOT NULL,
 standing_version bigint NOT NULL CHECK (standing_version > 0),
 action text NOT NULL CHECK (action IN ('RESTRICT_PROVIDER','LIFT_PROVIDER_RESTRICTION','RESTRICT_VEHICLE','LIFT_VEHICLE_RESTRICTION')),
 prior_restriction text,
 resulting_restriction text,
 staff_ref text NOT NULL,
 reason_code text NOT NULL,
 recorded_at timestamptz NOT NULL,
 UNIQUE (subject_kind, subject_id, standing_version)
);

-- ─── Canonical document decisions ──────────────────────────────────────────
-- Canonical expiry: expires_on is a calendar date in Africa/Lagos and the
-- document is valid through the END of that day there. expires_at stores the
-- exclusive instant (the following midnight, Africa/Lagos) as timestamptz; a
-- document is in force while now() < expires_at. This record, not the native
-- free-text license field, is authoritative for administrative gates.
CREATE TABLE IF NOT EXISTS taxi_admin_document_decisions (
 decision_id uuid PRIMARY KEY,
 subject_kind text NOT NULL CHECK (subject_kind IN ('providers','vehicles')),
 subject_id text NOT NULL,
 document text NOT NULL CHECK (document IN ('DRIVER_LICENSE','VEHICLE_REGISTRATION','INSURANCE','ROADWORTHINESS')),
 decision text NOT NULL CHECK (decision IN ('VERIFIED','REJECTED','EXPIRED')),
 expires_on date,
 expires_at timestamptz,
 evidence_ref text,
 standing_version bigint NOT NULL CHECK (standing_version > 0),
 staff_ref text NOT NULL,
 reason_code text NOT NULL,
 recorded_at timestamptz NOT NULL,
 CHECK ((subject_kind = 'providers' AND document = 'DRIVER_LICENSE') OR (subject_kind = 'vehicles' AND document IN ('VEHICLE_REGISTRATION','INSURANCE','ROADWORTHINESS'))),
 CHECK ((expires_on IS NULL) = (expires_at IS NULL)),
 CHECK (decision <> 'VERIFIED' OR (expires_at IS NOT NULL AND expires_at > recorded_at)),
 UNIQUE (subject_kind, subject_id, standing_version)
);
CREATE INDEX IF NOT EXISTS taxi_admin_document_decisions_latest ON taxi_admin_document_decisions(subject_kind, subject_id, document, standing_version DESC);

-- A reverification request stays open until a LATER (higher standing
-- version) VERIFIED decision for the same document exists. Derived, never
-- updated: both tables are append-only.
CREATE TABLE IF NOT EXISTS taxi_admin_reverification_requests (
 decision_id uuid PRIMARY KEY,
 subject_kind text NOT NULL CHECK (subject_kind IN ('providers','vehicles')),
 subject_id text NOT NULL,
 document text NOT NULL CHECK (document IN ('DRIVER_LICENSE','VEHICLE_REGISTRATION','INSURANCE','ROADWORTHINESS')),
 standing_version bigint NOT NULL CHECK (standing_version > 0),
 staff_ref text NOT NULL,
 reason_code text NOT NULL,
 recorded_at timestamptz NOT NULL,
 CHECK ((subject_kind = 'providers' AND document = 'DRIVER_LICENSE') OR (subject_kind = 'vehicles' AND document IN ('VEHICLE_REGISTRATION','INSURANCE','ROADWORTHINESS'))),
 UNIQUE (subject_kind, subject_id, standing_version)
);
CREATE INDEX IF NOT EXISTS taxi_admin_reverification_requests_subject ON taxi_admin_reverification_requests(subject_kind, subject_id, document);

-- ─── Incidents ─────────────────────────────────────────────────────────────
-- Case-versioned incident records keyed by the Bridge case_ref. Exactly one
-- of provider_id/vehicle_id/booking_id mirrors the subject, so the incident's
-- links are typed references rather than a free-form subject string.
CREATE TABLE IF NOT EXISTS taxi_admin_incidents (
 case_ref text PRIMARY KEY,
 subject_kind text NOT NULL CHECK (subject_kind IN ('providers','vehicles','independent-rides')),
 subject_id text NOT NULL,
 provider_id text,
 vehicle_id text,
 booking_id text,
 category text NOT NULL CHECK (category IN ('SAFETY','CONDUCT','VEHICLE_CONDITION','DOCUMENT','FRAUD_SIGNAL','SERVICE_QUALITY')),
 severity text NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
 occurred_on date NOT NULL,
 state text NOT NULL CHECK (state IN ('OPEN','REMEDIATION_IN_PROGRESS','RESOLVED','CLOSED')),
 resolution text CHECK (resolution IN ('REMEDIATED','NO_FINDING','ESCALATED_TO_RESTRICTION')),
 version bigint NOT NULL CHECK (version > 0),
 opened_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 resolved_at timestamptz,
 closed_at timestamptz,
 CHECK ((subject_kind = 'providers' AND provider_id = subject_id AND vehicle_id IS NULL AND booking_id IS NULL)
     OR (subject_kind = 'vehicles' AND vehicle_id = subject_id AND provider_id IS NULL AND booking_id IS NULL)
     OR (subject_kind = 'independent-rides' AND booking_id = subject_id AND provider_id IS NULL AND vehicle_id IS NULL)),
 CHECK ((state IN ('RESOLVED','CLOSED')) = (resolution IS NOT NULL AND resolved_at IS NOT NULL)),
 CHECK ((state = 'CLOSED') = (closed_at IS NOT NULL)),
 CHECK (closed_at IS NULL OR closed_at >= resolved_at)
);
CREATE INDEX IF NOT EXISTS taxi_admin_incidents_subject ON taxi_admin_incidents(subject_kind, subject_id, state);

-- Defense in depth for the incident machine: identity is immutable, every
-- update advances the version by exactly one, and only the legal edges
-- OPEN->REMEDIATION_IN_PROGRESS|RESOLVED, REMEDIATION_IN_PROGRESS->
-- REMEDIATION_IN_PROGRESS|RESOLVED and RESOLVED->CLOSED exist. RESOLVED and
-- CLOSED are terminal in that order; incidents are never deleted.
CREATE OR REPLACE FUNCTION taxi_admin_incident_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'Taxi incidents cannot be deleted'; END IF;
 IF NEW.case_ref <> OLD.case_ref OR NEW.subject_kind <> OLD.subject_kind OR NEW.subject_id <> OLD.subject_id
    OR NEW.category <> OLD.category OR NEW.severity <> OLD.severity OR NEW.occurred_on <> OLD.occurred_on
    OR NEW.opened_at <> OLD.opened_at OR NEW.version <> OLD.version + 1 THEN
  RAISE EXCEPTION 'Taxi incident identity is immutable and versions advance by one';
 END IF;
 IF NOT ((OLD.state = 'OPEN' AND NEW.state IN ('REMEDIATION_IN_PROGRESS','RESOLVED'))
      OR (OLD.state = 'REMEDIATION_IN_PROGRESS' AND NEW.state IN ('REMEDIATION_IN_PROGRESS','RESOLVED'))
      OR (OLD.state = 'RESOLVED' AND NEW.state = 'CLOSED' AND NEW.resolution = OLD.resolution AND NEW.resolved_at = OLD.resolved_at)) THEN
  RAISE EXCEPTION 'Illegal taxi incident transition % -> %', OLD.state, NEW.state;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS taxi_admin_incident_guard ON taxi_admin_incidents;
CREATE TRIGGER taxi_admin_incident_guard BEFORE UPDATE OR DELETE ON taxi_admin_incidents FOR EACH ROW EXECUTE FUNCTION taxi_admin_incident_guard();
DROP TRIGGER IF EXISTS taxi_admin_incident_no_truncate ON taxi_admin_incidents;
CREATE TRIGGER taxi_admin_incident_no_truncate BEFORE TRUNCATE ON taxi_admin_incidents FOR EACH STATEMENT EXECUTE FUNCTION taxi_admin_intervention_append_only();

CREATE TABLE IF NOT EXISTS taxi_admin_incident_remediations (
 decision_id uuid PRIMARY KEY,
 case_ref text NOT NULL REFERENCES taxi_admin_incidents(case_ref),
 remediation text NOT NULL CHECK (remediation IN ('RETRAINING','DOCUMENT_RESUBMISSION','VEHICLE_REPAIR','WARNING_ISSUED')),
 due_on date NOT NULL,
 incident_version bigint NOT NULL CHECK (incident_version > 0),
 recorded_at timestamptz NOT NULL,
 UNIQUE (case_ref, incident_version)
);

CREATE TABLE IF NOT EXISTS taxi_admin_incident_transitions (
 decision_id uuid PRIMARY KEY,
 case_ref text NOT NULL REFERENCES taxi_admin_incidents(case_ref),
 action text NOT NULL CHECK (action IN ('OPEN_INCIDENT','START_REMEDIATION','RESOLVE_INCIDENT','CLOSE_INCIDENT')),
 from_state text,
 to_state text NOT NULL,
 incident_version bigint NOT NULL CHECK (incident_version > 0),
 staff_ref text NOT NULL,
 reason_code text NOT NULL,
 recorded_at timestamptz NOT NULL,
 UNIQUE (case_ref, incident_version)
);

DO $$
DECLARE t text;
BEGIN
 FOREACH t IN ARRAY ARRAY['taxi_admin_restriction_history','taxi_admin_document_decisions','taxi_admin_reverification_requests','taxi_admin_incident_remediations','taxi_admin_incident_transitions'] LOOP
  EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_append_only', t);
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION taxi_admin_intervention_append_only()', t || '_append_only', t);
 END LOOP;
END $$;

-- ─── Gates ─────────────────────────────────────────────────────────────────
-- taxi_admin_document_block: REVERIFICATION_REQUIRED while any request lacks a
-- later VERIFIED decision for its document; DOCUMENT_INVALID when the latest
-- decision for any recorded document is REJECTED/EXPIRED or its canonical
-- expiry has passed; NULL otherwise. Documents never decided do not block:
-- native onboarding compliance remains the gate for those.
CREATE OR REPLACE FUNCTION taxi_admin_document_block(p_kind text, p_id text, p_at timestamptz) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT CASE
  WHEN EXISTS (SELECT 1 FROM taxi_admin_reverification_requests r
                WHERE r.subject_kind = p_kind AND r.subject_id = p_id
                  AND NOT EXISTS (SELECT 1 FROM taxi_admin_document_decisions d
                                   WHERE d.subject_kind = r.subject_kind AND d.subject_id = r.subject_id
                                     AND d.document = r.document AND d.decision = 'VERIFIED'
                                     AND d.standing_version > r.standing_version)) THEN 'REVERIFICATION_REQUIRED'
  WHEN EXISTS (SELECT 1 FROM (SELECT DISTINCT ON (document) decision, expires_at
                                FROM taxi_admin_document_decisions
                               WHERE subject_kind = p_kind AND subject_id = p_id
                               ORDER BY document, standing_version DESC) latest
                WHERE latest.decision <> 'VERIFIED' OR latest.expires_at <= p_at) THEN 'DOCUMENT_INVALID'
 END
$$;

-- taxi_admin_online_block: the reason a provider (optionally on a specific
-- vehicle) may not be online, or NULL.
CREATE OR REPLACE FUNCTION taxi_admin_online_block(p_provider text, p_vehicle text, p_at timestamptz) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT COALESCE(
  (SELECT 'provider_online_suspended' FROM taxi_admin_subject_standing WHERE subject_kind = 'providers' AND subject_id = p_provider AND restriction = 'SUSPEND_ONLINE'),
  (SELECT CASE taxi_admin_document_block('providers', p_provider, p_at) WHEN 'REVERIFICATION_REQUIRED' THEN 'provider_reverification_required' WHEN 'DOCUMENT_INVALID' THEN 'provider_document_invalid' END),
  (SELECT 'vehicle_out_of_service' FROM taxi_admin_subject_standing WHERE subject_kind = 'vehicles' AND subject_id = p_vehicle AND restriction = 'OUT_OF_SERVICE'),
  (SELECT CASE taxi_admin_document_block('vehicles', p_vehicle, p_at) WHEN 'REVERIFICATION_REQUIRED' THEN 'vehicle_reverification_required' WHEN 'DOCUMENT_INVALID' THEN 'vehicle_document_invalid' END))
$$;

-- taxi_admin_assignment_block: the reason a provider/vehicle may not receive
-- NEW work (offers, reservations, private execution), or NULL. Everything
-- that blocks going online also blocks assignment.
CREATE OR REPLACE FUNCTION taxi_admin_assignment_block(p_provider text, p_vehicle text, p_at timestamptz) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT COALESCE(
  taxi_admin_online_block(p_provider, p_vehicle, p_at),
  (SELECT 'provider_no_new_assignments' FROM taxi_admin_subject_standing WHERE subject_kind = 'providers' AND subject_id = p_provider AND restriction = 'NO_NEW_ASSIGNMENTS'))
$$;
