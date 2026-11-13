-- Organization-managed dispatch providers (Fleet FLEET-10.2).
--
-- fleet-partner-service resolves a person by email and phone here, asks for an
-- ORGANIZATION_SPONSORED identity when nobody matches, and records an
-- association the RIDER must accept in their own app. Fleet never writes these
-- tables; it calls the private contract and reads the association back.
--
-- Additive only: every new providers column is nullable or defaulted, so the
-- independent rider flow is untouched. Organization identifiers are Fleet's
-- opaque TEXT ids (org_..., pmb_..., brn_...) and carry no foreign key.

ALTER TABLE providers
    ADD COLUMN IF NOT EXISTS managed_by_organization_id TEXT NULL;
ALTER TABLE providers
    ADD COLUMN IF NOT EXISTS onboarding_mode TEXT NOT NULL DEFAULT 'INDEPENDENT';
ALTER TABLE providers DROP CONSTRAINT IF EXISTS chk_providers_onboarding_mode;
ALTER TABLE providers
    ADD CONSTRAINT chk_providers_onboarding_mode CHECK (
        onboarding_mode IN ('INDEPENDENT', 'ORGANIZATION_SPONSORED')
    );

CREATE TABLE IF NOT EXISTS dispatch_organization_provider_links (
    link_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     TEXT NOT NULL,
    provider_id         UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    membership_id       TEXT NOT NULL,
    branch_id           TEXT NULL,
    state               TEXT NOT NULL,
    version             BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    idempotency_key     TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    decided_at          TIMESTAMPTZ NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_dispatch_org_provider_link_state CHECK (
        state IN ('REQUESTED', 'ACTIVE', 'SUSPENDED', 'REVOKED', 'REJECTED')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_org_provider_link_idempotency
    ON dispatch_organization_provider_links (idempotency_key);

-- One live organization relationship per provider: a rider cannot be managed
-- by two organizations for the same service at once.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_org_provider_link_live
    ON dispatch_organization_provider_links (provider_id)
    WHERE state IN ('REQUESTED', 'ACTIVE', 'SUSPENDED');

CREATE INDEX IF NOT EXISTS idx_dispatch_org_provider_links_org
    ON dispatch_organization_provider_links (organization_id, state, updated_at DESC);

-- Every state change, whoever made it. The provider's own decision is recorded
-- with actor_type PROVIDER so "who consented" is never ambiguous.
CREATE TABLE IF NOT EXISTS dispatch_organization_provider_link_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    link_id         UUID NOT NULL REFERENCES dispatch_organization_provider_links(link_id) ON DELETE RESTRICT,
    organization_id TEXT NOT NULL,
    prior_state     TEXT NULL,
    new_state       TEXT NOT NULL,
    source_version  BIGINT NOT NULL,
    actor_type      TEXT NOT NULL CHECK (actor_type IN ('SERVICE', 'PROVIDER')),
    actor_id        TEXT NOT NULL,
    reason_code     TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT '',
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (link_id, source_version)
);

-- Replay ledger for the non-link commands (sponsored provisioning, link
-- transitions): the same Idempotency-Key with the same body returns the first
-- answer; with a different body it is refused.
CREATE TABLE IF NOT EXISTS dispatch_organization_provider_operations (
    operation_type      TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    resource_id         TEXT NOT NULL,
    response_body       JSONB NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (operation_type, idempotency_key)
);
