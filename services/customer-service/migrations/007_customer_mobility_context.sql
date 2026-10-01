-- Section 6: reusable customer mobility context.
--
-- This service re-applies every migration file on each MIGRATION=true boot
-- (internal/database/migrate.go keeps no tracker), so every statement here
-- must be idempotent: IF NOT EXISTS on tables, columns and indexes, and
-- CHECK constraints declared inline with their column.

-- ─── Trusted contacts (extends the canonical emergency_contacts entity) ────
-- One person is one row. /profile/emergency-contacts and /trusted-contacts
-- are two presentations of the same record. Consent is explicit per
-- capability. Trip sharing always backfills to FALSE; rows that already
-- exist were created as emergency contacts, so they keep emergency alerts.
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS emergency_alerts_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS trip_sharing_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1);
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS client_request_key TEXT;
ALTER TABLE emergency_contacts ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS emergency_contacts_client_request_uidx
    ON emergency_contacts(customer_id, client_request_key) WHERE client_request_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS emergency_contacts_customer_active_idx
    ON emergency_contacts(customer_id, active, created_at);

-- ─── Mobility preferences (one row per customer, optimistic version) ───────
-- A preference is reusable context, never an automatic hard booking
-- requirement: operational services freeze hard requirements per booking.
CREATE TABLE IF NOT EXISTS customer_mobility_preferences (
    customer_id               UUID PRIMARY KEY REFERENCES customers(id) ON DELETE CASCADE,
    accessibility_required    BOOLEAN NOT NULL DEFAULT FALSE,
    wheelchair_required       BOOLEAN,
    child_seat_preference     TEXT CHECK (child_seat_preference IN ('NOT_NEEDED','INFANT','TODDLER','BOOSTER')),
    pet_travel_preference     TEXT CHECK (pet_travel_preference IN ('NONE','SMALL_PET','LARGE_PET','SERVICE_ANIMAL')),
    luggage_default           TEXT NOT NULL DEFAULT 'NONE' CHECK (luggage_default IN ('NONE','CARRY_ON','STANDARD','LARGE')),
    preferred_passenger_count INT CHECK (preferred_passenger_count BETWEEN 1 AND 8),
    communication_preferences JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(communication_preferences) = 'object'),
    safety_share_default      BOOLEAN NOT NULL DEFAULT FALSE,
    special_assistance_notes  TEXT CHECK (special_assistance_notes IS NULL OR length(special_assistance_notes) <= 500),
    version                   BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Guest / family travellers ─────────────────────────────────────────────
-- A guest is reusable context only and never becomes a registered customer.
-- Rows are deactivated, never deleted, so historical bookings keep a
-- resolvable reference.
CREATE TABLE IF NOT EXISTS customer_guest_travellers (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id                UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    name                       TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    phone                      TEXT,
    relationship_label         TEXT CHECK (relationship_label IS NULL OR length(relationship_label) <= 60),
    accessibility_notes        TEXT CHECK (accessibility_notes IS NULL OR length(accessibility_notes) <= 500),
    mobility_requirements_json JSONB CHECK (mobility_requirements_json IS NULL OR jsonb_typeof(mobility_requirements_json) = 'object'),
    active                     BOOLEAN NOT NULL DEFAULT TRUE,
    version                    BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    client_request_key         TEXT,
    request_fingerprint        TEXT,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS customer_guest_travellers_customer_idx
    ON customer_guest_travellers(customer_id, active, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS customer_guest_travellers_client_request_uidx
    ON customer_guest_travellers(customer_id, client_request_key) WHERE client_request_key IS NOT NULL;
