-- Section 20: kitchen, bakery and production operations.
--
-- These columns/tables extend the existing ProductionJob authority.  They do
-- not create a parallel order or capacity subsystem.
ALTER TABLE production_jobs
    ADD COLUMN IF NOT EXISTS promised_ready_at timestamptz,
    ADD COLUMN IF NOT EXISTS ready_evidence_id text;

CREATE UNIQUE INDEX IF NOT EXISTS production_jobs_external_order_source_uq
    ON production_jobs (business_id, source_type, source_ref)
    WHERE source_type = 'EXTERNAL_ORDER' AND source_ref IS NOT NULL;

CREATE INDEX IF NOT EXISTS production_jobs_operational_queue_idx
    ON production_jobs (
        business_id, branch_id, status,
        (promised_ready_at IS NULL), promised_ready_at,
        (CASE priority WHEN 'URGENT' THEN 0 WHEN 'HIGH' THEN 1 ELSE 2 END),
        created_at, id
    );

CREATE TABLE IF NOT EXISTS production_job_steps (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    job_id text NOT NULL,
    production_line_id text,
    sequence_no integer NOT NULL CHECK (sequence_no >= 0),
    phase integer NOT NULL CHECK (phase >= 0),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    required boolean NOT NULL DEFAULT true,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','READY','IN_PROGRESS','BLOCKED','COMPLETED','SKIPPED','CANCELLED')),
    blocked_reason text NOT NULL DEFAULT '' CHECK (char_length(blocked_reason) <= 280),
    blocked_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT production_job_steps_job_fk
        FOREIGN KEY (job_id, business_id) REFERENCES production_jobs(id, business_id) ON DELETE CASCADE,
    CONSTRAINT production_job_steps_line_fk
        FOREIGN KEY (production_line_id, business_id) REFERENCES production_lines(id, business_id),
    UNIQUE (job_id, sequence_no)
);
CREATE INDEX IF NOT EXISTS production_job_steps_queue_idx
    ON production_job_steps (business_id, job_id, phase, sequence_no, id);

CREATE TABLE IF NOT EXISTS production_job_delay_events (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    job_id text NOT NULL,
    reason_code text NOT NULL CHECK (reason_code IN ('STAFF_SHORTAGE','EQUIPMENT_FAILURE','INGREDIENT_SHORTAGE','QUALITY_REWORK','CAPACITY_CONFLICT','LOGISTICS_CONSTRAINT','OTHER')),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 280),
    promised_ready_at timestamptz,
    previous_window_start timestamptz,
    previous_window_end timestamptz,
    new_window_start timestamptz,
    new_window_end timestamptz,
    actor_id text,
    correlation_id text,
    idempotency_key text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT production_job_delay_events_job_fk
        FOREIGN KEY (job_id, business_id) REFERENCES production_jobs(id, business_id) ON DELETE CASCADE,
    UNIQUE (business_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS production_job_delay_events_job_idx
    ON production_job_delay_events (business_id, job_id, recorded_at DESC, id);

CREATE TABLE IF NOT EXISTS production_job_ready_evidence (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    job_id text NOT NULL,
    packaging_profile jsonb NOT NULL CHECK (jsonb_typeof(packaging_profile) = 'object'),
    packaging_evidence jsonb NOT NULL CHECK (jsonb_typeof(packaging_evidence) = 'object'),
    actual_quantity integer CHECK (actual_quantity IS NULL OR actual_quantity >= 0),
    recorded_by text,
    correlation_id text,
    idempotency_key text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT production_job_ready_evidence_job_fk
        FOREIGN KEY (job_id, business_id) REFERENCES production_jobs(id, business_id) ON DELETE CASCADE,
    UNIQUE (business_id, job_id),
    UNIQUE (business_id, idempotency_key),
    UNIQUE (id, business_id)
);

ALTER TABLE production_jobs
    ADD CONSTRAINT production_jobs_ready_evidence_fk
    FOREIGN KEY (ready_evidence_id, business_id) REFERENCES production_job_ready_evidence(id, business_id);

-- Orders-owned operational read model. It carries timing only; it never
-- changes VendorOrder money or commercial lifecycle state.
CREATE TABLE IF NOT EXISTS commerce_vendor_order_readiness (
    vendor_order_id text PRIMARY KEY REFERENCES commerce_vendor_orders(id) ON DELETE CASCADE,
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE CASCADE,
    business_id text NOT NULL,
    branch_id text NOT NULL,
    production_job_id text NOT NULL,
    promised_ready_at timestamptz,
    expected_window_start timestamptz,
    expected_window_end timestamptz,
    source_version bigint NOT NULL DEFAULT 1 CHECK (source_version > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS commerce_vendor_order_readiness_branch_idx
    ON commerce_vendor_order_readiness (business_id, branch_id, expected_window_end, vendor_order_id);

ALTER TABLE commerce_order_missions
    ADD COLUMN IF NOT EXISTS production_ready_window_start timestamptz,
    ADD COLUMN IF NOT EXISTS production_ready_window_end timestamptz,
    ADD COLUMN IF NOT EXISTS production_timing_source_job_id text,
    ADD COLUMN IF NOT EXISTS production_timing_version bigint NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS commerce_order_missions_production_timing_idx
    ON commerce_order_missions (production_timing_source_job_id)
    WHERE production_timing_source_job_id IS NOT NULL;
