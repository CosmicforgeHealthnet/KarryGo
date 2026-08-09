CREATE TABLE hauling_authority_restrictions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL,
    subject_type text NOT NULL CHECK (subject_type IN ('ORGANIZATION','PROVIDER','ASSET','SERVICE_CAPABILITY','SHIPMENT')),
    subject_reference text NOT NULL,
    state text NOT NULL CHECK (state IN ('APPLIED','CLEARED')),
    reason_code text NOT NULL,
    source_reference text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    applied_at timestamptz,
    cleared_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, subject_type, subject_reference)
);
CREATE INDEX idx_hauling_authority_restrictions_active
    ON hauling_authority_restrictions(subject_type,subject_reference) WHERE state='APPLIED';

CREATE TABLE hauling_authority_restriction_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), restriction_id uuid NOT NULL REFERENCES hauling_authority_restrictions(id),
    organization_id text NOT NULL, subject_type text NOT NULL, subject_reference text NOT NULL,
    state text NOT NULL, reason_code text NOT NULL, source_reference text NOT NULL,
    source_version bigint NOT NULL, restriction_version bigint NOT NULL, recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE hauling_authority_restriction_operations (
    operation_reference text PRIMARY KEY, restriction_id uuid NOT NULL REFERENCES hauling_authority_restrictions(id),
    requested_state text NOT NULL CHECK (requested_state IN ('APPLIED','CLEARED')),
    request_fingerprint text NOT NULL, outcome text NOT NULL CHECK (outcome IN ('CONFIRMED','SUPERSEDED')),
    source_version bigint NOT NULL, restriction_version bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION reject_hauling_restriction_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'hauling restriction history is append-only'; END; $$;
CREATE TRIGGER hauling_restriction_history_append_only BEFORE UPDATE OR DELETE ON hauling_authority_restriction_history
FOR EACH ROW EXECUTE FUNCTION reject_hauling_restriction_history_mutation();
CREATE TRIGGER hauling_restriction_operations_append_only BEFORE UPDATE OR DELETE ON hauling_authority_restriction_operations
FOR EACH ROW EXECUTE FUNCTION reject_hauling_restriction_history_mutation();
