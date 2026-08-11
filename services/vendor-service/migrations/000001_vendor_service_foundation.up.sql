-- Vendor Service foundation migration.
--
-- This is architecture-phase scaffolding only: it establishes the extensions
-- and a service identity marker table so the migration runner, health check,
-- and migration-replay tests have something real to exercise. It defines no
-- business tables. Business schema (vendors, catalogues, orders, inventory,
-- etc.) is added by later, section-scoped migrations per
-- docs/architecture/12_MIGRATION_OWNERSHIP.md.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS vendor_service_identity (
    id boolean PRIMARY KEY DEFAULT true,
    service_name text NOT NULL DEFAULT 'vendor-service',
    initialized_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vendor_service_identity_singleton CHECK (id)
);

INSERT INTO vendor_service_identity (id) VALUES (true)
ON CONFLICT (id) DO NOTHING;
