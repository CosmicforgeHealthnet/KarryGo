-- Section 13G
-- Redis cache, incremental search projection and invalidation.
--
-- AUTHORITATIVE HIERARCHY:
--
-- PostgreSQL Food / Shop Domain
--              |
--              v
-- PostgreSQL Search Projection
--              |
--              v
-- Redis Cache
--
-- Redis is explicitly NON-AUTHORITATIVE.

CREATE TABLE search_cache_revisions (
    namespace_code TEXT PRIMARY KEY
        CHECK (
            namespace_code IN (
                'FOOD',
                'SHOP'
            )
        ),

    revision BIGINT NOT NULL
        CHECK (revision > 0),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO search_cache_revisions (
    namespace_code,
    revision
)
VALUES
    ('FOOD', 1),
    ('SHOP', 1);


CREATE TABLE search_projection_source_checkpoints (
    namespace_code TEXT NOT NULL
        CHECK (
            namespace_code IN (
                'FOOD',
                'SHOP'
            )
        ),

    source_kind TEXT NOT NULL,
    source_ref TEXT NOT NULL,

    latest_revision BIGINT NOT NULL
        CHECK (latest_revision > 0),

    last_event_id TEXT NOT NULL,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (
        namespace_code,
        source_kind,
        source_ref
    ),

    CHECK (
        btrim(source_kind) <> ''
        AND btrim(source_ref) <> ''
        AND btrim(last_event_id) <> ''
    )
);


CREATE TABLE search_projection_event_ledger (
    event_id TEXT PRIMARY KEY,

    namespace_code TEXT NOT NULL
        CHECK (
            namespace_code IN (
                'FOOD',
                'SHOP'
            )
        ),

    event_type TEXT NOT NULL,

    source_kind TEXT NOT NULL,
    source_ref TEXT NOT NULL,

    source_revision BIGINT NOT NULL
        CHECK (source_revision > 0),

    generation BIGINT NOT NULL
        CHECK (generation > 0),

    payload_hash TEXT NOT NULL
        CHECK (
            char_length(payload_hash) = 64
        ),

    outcome TEXT NOT NULL
        CHECK (
            outcome IN (
                'APPLIED',
                'STALE'
            )
        ),

    cache_revision BIGINT NOT NULL
        CHECK (cache_revision > 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (
        btrim(event_id) <> ''
        AND btrim(event_type) <> ''
        AND btrim(source_kind) <> ''
        AND btrim(source_ref) <> ''
    )
);


CREATE INDEX search_projection_event_source_idx
    ON search_projection_event_ledger (
        namespace_code,
        source_kind,
        source_ref,
        source_revision DESC
    );


CREATE INDEX search_projection_event_processed_idx
    ON search_projection_event_ledger (
        processed_at DESC
    );


COMMENT ON TABLE search_cache_revisions IS
    'Monotonic namespace cache revision. Redis keys include this revision so correctness does not depend on wildcard deletion.';

COMMENT ON TABLE search_projection_source_checkpoints IS
    'Latest consumed authoritative revision per search projection source aggregate.';

COMMENT ON TABLE search_projection_event_ledger IS
    'Idempotency and stale-event evidence for incremental Section 13 search projection events.';