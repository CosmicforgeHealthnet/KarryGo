-- Section 14 (43.3 / 59.6): sales leads and their conversion.
--
-- A lead is an organization-owned prospect captured before anyone knows
-- whether it is a real customer. It is not a CRM customer and never a
-- KarryGo marketplace customer identity: conversion creates (or maps onto) a
-- tenant-private company CRM customer and, optionally, a sales opportunity
-- against it. Nothing here touches customer-service.
--
-- Conversion happens exactly once. The lead row is the serialization point:
-- the converting transaction locks it, and the CHECK constraints below make
-- a CONVERTED lead without a customer, or a customer link on an unconverted
-- lead, unrepresentable rather than merely unlikely.

CREATE TABLE IF NOT EXISTS fleet_partner_sales_leads (
    id                         text PRIMARY KEY,
    organization_id            text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                  text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    source                     text NOT NULL
                               CHECK (source IN ('WEBSITE', 'REFERRAL', 'EVENT', 'INBOUND_CALL',
                                                 'INBOUND_EMAIL', 'OUTBOUND', 'PARTNER', 'IMPORT', 'OTHER')),
    company_name               text NOT NULL CHECK (length(btrim(company_name)) > 0),
    -- Who the lead is: name, role, email, phone as captured. A snapshot, not
    -- a CRM contact; conversion does not copy it into a contact record.
    contact_snapshot           jsonb NOT NULL DEFAULT '{}'::jsonb,
    status                     text NOT NULL DEFAULT 'NEW'
                               CHECK (status IN ('NEW', 'CONTACTED', 'QUALIFIED', 'DISQUALIFIED', 'CONVERTED')),
    owner_user_id              text,
    score                      integer CHECK (score IS NULL OR (score >= 0 AND score <= 100)),
    next_action                text,
    next_action_at             timestamptz,
    notes                      text,
    disqualification_reason    text,
    converted_customer_id      text REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    converted_opportunity_id   text REFERENCES fleet_partner_sales_opportunities (id) ON DELETE RESTRICT,
    converted_at               timestamptz,
    converted_by               text,
    conversion_idempotency_key text,
    create_idempotency_key     text,
    create_fingerprint         text,
    version                    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                 text NOT NULL,
    updated_by                 text NOT NULL DEFAULT '',
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'CONVERTED') = (converted_customer_id IS NOT NULL)),
    CHECK (status <> 'CONVERTED' OR converted_at IS NOT NULL),
    CHECK (converted_opportunity_id IS NULL OR converted_customer_id IS NOT NULL),
    CHECK (status <> 'DISQUALIFIED'
           OR (disqualification_reason IS NOT NULL AND length(btrim(disqualification_reason)) > 0))
);

CREATE INDEX IF NOT EXISTS idx_fleet_sales_leads_cursor
    ON fleet_partner_sales_leads (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_sales_leads_status
    ON fleet_partner_sales_leads (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_sales_leads_owner
    ON fleet_partner_sales_leads (organization_id, owner_user_id, next_action_at)
    WHERE owner_user_id IS NOT NULL;
-- A create retried under the same Idempotency-Key finds the lead it made
-- instead of making a second one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_sales_leads_create_key
    ON fleet_partner_sales_leads (organization_id, create_idempotency_key)
    WHERE create_idempotency_key IS NOT NULL;
-- One opportunity is produced by at most one lead.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_sales_leads_opportunity
    ON fleet_partner_sales_leads (converted_opportunity_id)
    WHERE converted_opportunity_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_sales_lead_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    lead_id          text NOT NULL REFERENCES fleet_partner_sales_leads (id) ON DELETE RESTRICT,
    change_type      text NOT NULL
                     CHECK (change_type IN ('CREATED', 'UPDATED', 'ASSIGNED', 'STATUS_CHANGED', 'CONVERTED')),
    previous_status  text NOT NULL DEFAULT '',
    new_status       text NOT NULL,
    reason_code      text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_sales_lead_history
    ON fleet_partner_sales_lead_history (organization_id, lead_id, occurred_at);

CREATE TRIGGER trg_fleet_sales_lead_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_sales_lead_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
