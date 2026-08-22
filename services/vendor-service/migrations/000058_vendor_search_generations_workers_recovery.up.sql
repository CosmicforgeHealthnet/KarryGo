-- Section 13H
-- Full rebuilds, projection generations, worker recovery and administration.
--
-- Search projections remain fully reconstructable derived state.
--
-- Global generation control is internal/admin-only at the application layer.

CREATE TABLE search_projection_generations (
    namespace_code TEXT NOT NULL
        CHECK (
            namespace_code IN (
                'FOOD',
                'SHOP'
            )
        ),

    generation BIGINT NOT NULL
        CHECK (generation > 0),

    status TEXT NOT NULL
        CHECK (
            status IN (
                'BUILDING',
                'READY',
                'ACTIVE',
                'FAILED',
                'RETIRED'
            )
        ),

    base_generation BIGINT NOT NULL DEFAULT 0
        CHECK (base_generation >= 0),

    rebuild_scope TEXT NOT NULL DEFAULT 'FULL'
        CHECK (
            rebuild_scope IN (
                'FULL',
                'BUSINESS',
                'BRANCH'
            )
        ),

    scope_ref TEXT,

    source_revisions JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (
            jsonb_typeof(source_revisions)='object'
        ),

    document_count BIGINT NOT NULL DEFAULT 0
        CHECK (document_count >= 0),

    processed_count BIGINT NOT NULL DEFAULT 0
        CHECK (processed_count >= 0),

    failure_code TEXT,

    failure_reason TEXT,

    requested_by TEXT NOT NULL,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    validated_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    retired_at TIMESTAMPTZ,

    worker_id TEXT,
    lease_token TEXT,
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (
        namespace_code,
        generation
    ),

    CHECK (
        btrim(requested_by) <> ''
    ),

    CHECK (
        (
            rebuild_scope='FULL'
            AND scope_ref IS NULL
        )
        OR
        (
            rebuild_scope IN ('BUSINESS', 'BRANCH')
            AND scope_ref IS NOT NULL
            AND btrim(scope_ref) <> ''
        )
    ),

    CHECK (
        failure_code IS NULL
        OR char_length(failure_code) <= 64
    ),

    CHECK (
        failure_reason IS NULL
        OR char_length(failure_reason) <= 500
    )
);


CREATE UNIQUE INDEX search_projection_one_active_generation_idx
    ON search_projection_generations (
        namespace_code
    )
    WHERE status='ACTIVE';


CREATE INDEX search_projection_generation_status_idx
    ON search_projection_generations (
        namespace_code,
        status,
        generation DESC
    );


-- Bootstrap an existing 13D-13G projection generation if data
-- already exists before the generation registry is introduced.

INSERT INTO search_projection_generations (
    namespace_code,
    generation,
    status,
    base_generation,
    rebuild_scope,
    requested_by,
    started_at,
    completed_at,
    validated_at,
    activated_at,
    document_count,
    processed_count
)
SELECT
    'FOOD',
    max(generation),
    'ACTIVE',
    0,
    'FULL',
    'SYSTEM_BOOTSTRAP',
    now(),
    now(),
    now(),
    now(),
    count(*),
    count(*)
FROM food_search_documents
HAVING count(*) > 0;


INSERT INTO search_projection_generations (
    namespace_code,
    generation,
    status,
    base_generation,
    rebuild_scope,
    requested_by,
    started_at,
    completed_at,
    validated_at,
    activated_at,
    document_count,
    processed_count
)
SELECT
    'SHOP',
    max(generation),
    'ACTIVE',
    0,
    'FULL',
    'SYSTEM_BOOTSTRAP',
    now(),
    now(),
    now(),
    now(),
    count(*),
    count(*)
FROM shop_search_documents
HAVING count(*) > 0;


CREATE TABLE search_projection_generation_history (
    id BIGSERIAL PRIMARY KEY,

    namespace_code TEXT NOT NULL,
    generation BIGINT NOT NULL,

    action TEXT NOT NULL
        CHECK (
            action IN (
                'REQUESTED',
                'STARTED',
                'PROGRESS',
                'BUILD_COMPLETED',
                'VALIDATED',
                'ACTIVATED',
                'FAILED',
                'RETRY_REQUESTED',
                'RETIRED'
            )
        ),

    from_status TEXT,
    to_status TEXT,

    worker_id TEXT,

    document_count BIGINT,
    processed_count BIGINT,

    failure_code TEXT,
    failure_reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    FOREIGN KEY (
        namespace_code,
        generation
    )
    REFERENCES search_projection_generations (
        namespace_code,
        generation
    )
    ON DELETE CASCADE,

    CHECK (
        failure_reason IS NULL
        OR char_length(failure_reason) <= 500
    )
);


CREATE INDEX search_projection_generation_history_lookup_idx
    ON search_projection_generation_history (
        namespace_code,
        generation,
        created_at
    );


CREATE TABLE search_projection_jobs (
    job_id TEXT PRIMARY KEY,

    job_type TEXT NOT NULL
        CHECK (
            job_type IN (
                'INCREMENTAL_PROJECTION',
                'FULL_REBUILD',
                'SCOPED_REBUILD',
                'GENERATION_VALIDATE_ACTIVATE',
                'CACHE_RECONCILE'
            )
        ),

    namespace_code TEXT NOT NULL
        CHECK (
            namespace_code IN (
                'FOOD',
                'SHOP'
            )
        ),

    generation BIGINT,

    scope_type TEXT
        CHECK (
            scope_type IS NULL
            OR scope_type IN (
                'FULL',
                'BUSINESS',
                'BRANCH'
            )
        ),

    scope_ref TEXT,

    reference_id TEXT,

    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (
            status IN (
                'PENDING',
                'LEASED',
                'RETRY',
                'COMPLETED',
                'DEAD_LETTER'
            )
        ),

    priority INTEGER NOT NULL DEFAULT 100
        CHECK (
            priority BETWEEN 0 AND 1000
        ),

    attempt_count INTEGER NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0),

    max_attempts INTEGER NOT NULL DEFAULT 5
        CHECK (
            max_attempts BETWEEN 1 AND 20
        ),

    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    lease_owner TEXT,
    lease_token TEXT,
    lease_expires_at TIMESTAMPTZ,
    heartbeat_at TIMESTAMPTZ,

    progress_checkpoint TEXT,

    failure_code TEXT,
    failure_reason TEXT,

    dedupe_key TEXT NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,

    CHECK (
        generation IS NULL
        OR generation > 0
    ),

    CHECK (
        progress_checkpoint IS NULL
        OR char_length(progress_checkpoint) <= 2048
    ),

    CHECK (
        failure_code IS NULL
        OR char_length(failure_code) <= 64
    ),

    CHECK (
        failure_reason IS NULL
        OR char_length(failure_reason) <= 500
    ),

    CHECK (
        btrim(dedupe_key) <> ''
    )
);


CREATE INDEX search_projection_jobs_claim_idx
    ON search_projection_jobs (
        status,
        available_at,
        priority,
        created_at
    );


CREATE INDEX search_projection_jobs_generation_idx
    ON search_projection_jobs (
        namespace_code,
        generation,
        job_type,
        status
    );


CREATE INDEX search_projection_jobs_lease_idx
    ON search_projection_jobs (
        lease_expires_at
    )
    WHERE status='LEASED';


COMMENT ON TABLE search_projection_generations IS
    'Reconstructable FOOD/SHOP search generations. Only ACTIVE generations should serve normal discovery traffic.';

COMMENT ON COLUMN search_projection_generations.base_generation IS
    'Active generation observed when rebuild was requested. Atomic activation uses this as compare-and-swap evidence to prevent competing generation activation.';

COMMENT ON TABLE search_projection_jobs IS
    'Persistent worker lease/retry/dead-letter/checkpoint state executed through the existing Vendor jobs.Job runner.';

COMMENT ON COLUMN search_projection_jobs.progress_checkpoint IS
    'Bounded opaque progress cursor only. Sensitive domain payloads must not be stored here.';

COMMENT ON TABLE search_projection_generation_history IS
    'Operational generation lifecycle history. Does not store raw catalogue, inventory, identity or sensitive payloads.';