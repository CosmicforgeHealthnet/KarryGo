-- Spec Sections 17 and 46: shift templates (provider_shift_templates) and
-- training enrolments (training_enrollments).

CREATE TABLE IF NOT EXISTS fleet_partner_shift_templates (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id         text NOT NULL REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    role              text NOT NULL DEFAULT '',
    timezone          text NOT NULL,
    -- Days of the week the template repeats on, ISO numbering 1 (Mon) - 7 (Sun).
    weekdays          smallint[] NOT NULL CHECK (cardinality(weekdays) BETWEEN 1 AND 7
                          AND weekdays <@ ARRAY[1,2,3,4,5,6,7]::smallint[]),
    start_local_time  text NOT NULL CHECK (start_local_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    end_local_time    text NOT NULL CHECK (end_local_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    required_capacity integer NOT NULL DEFAULT 1 CHECK (required_capacity > 0),
    break_rules       jsonb NOT NULL DEFAULT '{}'::jsonb,
    status            text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    created_by        text NOT NULL,
    version           bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (start_local_time <> end_local_time)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_shift_templates_scope
    ON fleet_partner_shift_templates (organization_id, branch_id, status);

-- Shifts remember the template they were generated from, and a template can
-- generate a given local day only once.
ALTER TABLE fleet_partner_shifts ADD COLUMN IF NOT EXISTS template_id text
    REFERENCES fleet_partner_shift_templates (id) ON DELETE RESTRICT;
ALTER TABLE fleet_partner_shifts ADD COLUMN IF NOT EXISTS template_local_date date;
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_shifts_template_day
    ON fleet_partner_shifts (template_id, template_local_date)
    WHERE template_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_training_enrollments (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    course_id       text NOT NULL REFERENCES fleet_partner_training_courses (id) ON DELETE RESTRICT,
    course_version  bigint NOT NULL,
    membership_id   text NOT NULL,
    status          text NOT NULL DEFAULT 'ASSIGNED'
                    CHECK (status IN ('ASSIGNED', 'PASSED', 'FAILED', 'CANCELLED')),
    assigned_by     text NOT NULL,
    assigned_at     timestamptz NOT NULL DEFAULT now(),
    due_at          timestamptz,
    completed_at    timestamptz,
    score           numeric(6, 2),
    completion_id   text REFERENCES fleet_partner_training_completions (id) ON DELETE RESTRICT,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'ASSIGNED' OR completion_id IS NULL)
);

-- One open enrolment per person per course.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_training_enrollment_open
    ON fleet_partner_training_enrollments (organization_id, course_id, membership_id)
    WHERE status = 'ASSIGNED';
CREATE INDEX IF NOT EXISTS idx_fleet_partner_training_enrollments_scope
    ON fleet_partner_training_enrollments (organization_id, status, assigned_at);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_training_enrollments_member
    ON fleet_partner_training_enrollments (organization_id, membership_id);
