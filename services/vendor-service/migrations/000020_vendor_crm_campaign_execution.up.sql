-- Section 8F: durable campaign execution. No destination PII is stored here.
CREATE TABLE crm_campaign_runs (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    campaign_id text NOT NULL,
    campaign_version_id text NOT NULL,
    campaign_definition_version bigint NOT NULL CHECK (campaign_definition_version > 0),
    snapshot_id text NOT NULL,
    trigger_type text NOT NULL DEFAULT 'MANUAL' CHECK (trigger_type IN ('MANUAL','EVENT')),
    trigger_ref text,
    mode text NOT NULL CHECK (mode IN ('SEND_NOW','SCHEDULED')),
    status text NOT NULL CHECK (status IN ('SCHEDULED','QUEUED','RUNNING','PAUSED','COMPLETED','COMPLETED_WITH_FAILURES','CANCELLED','FAILED')),
    scheduled_for timestamptz NOT NULL,
    template_key text CHECK (template_key IS NULL OR char_length(template_key) BETWEEN 1 AND 120),
    locale text CHECK (locale IS NULL OR char_length(locale) BETWEEN 2 AND 16),
    title text CHECK (title IS NULL OR char_length(title) BETWEEN 1 AND 200),
    body text CHECK (body IS NULL OR char_length(body) BETWEEN 1 AND 4000),
    total_deliveries integer NOT NULL DEFAULT 0 CHECK (total_deliveries >= 0),
    handed_off_count integer NOT NULL DEFAULT 0 CHECK (handed_off_count >= 0),
    delivered_count integer NOT NULL DEFAULT 0 CHECK (delivered_count >= 0),
    suppressed_count integer NOT NULL DEFAULT 0 CHECK (suppressed_count >= 0),
    failed_count integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    requested_by_staff_ref text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE(id,business_id),
    CONSTRAINT crm_campaign_run_campaign_fk FOREIGN KEY(campaign_id,business_id) REFERENCES crm_campaigns(id,business_id),
    CONSTRAINT crm_campaign_run_version_fk FOREIGN KEY(campaign_version_id,business_id) REFERENCES crm_campaign_versions(id,business_id),
    CONSTRAINT crm_campaign_run_snapshot_fk FOREIGN KEY(snapshot_id,business_id) REFERENCES crm_campaign_audience_snapshots(id,business_id),
    CONSTRAINT crm_campaign_run_content_ck CHECK (template_key IS NOT NULL OR title IS NOT NULL OR body IS NOT NULL)
);
CREATE INDEX crm_campaign_runs_campaign_idx ON crm_campaign_runs(business_id,campaign_id,created_at DESC,id);
CREATE INDEX crm_campaign_runs_due_idx ON crm_campaign_runs(scheduled_for,id) WHERE status IN ('SCHEDULED','QUEUED');

CREATE TABLE crm_campaign_deliveries (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    run_id text NOT NULL,
    campaign_id text NOT NULL,
    snapshot_id text NOT NULL,
    target_id text NOT NULL,
    customer_profile_id text NOT NULL,
    channel text NOT NULL CHECK (channel IN ('EMAIL','PUSH','IN_APP','SMS')),
    variant_key text,
    is_control boolean NOT NULL DEFAULT false,
    status text NOT NULL CHECK (status IN ('PENDING','DEFERRED_QUIET_HOURS','SUPPRESSED','HANDED_OFF','ACCEPTED','DELIVERED','FAILED_TRANSIENT','FAILED_PERMANENT','CANCELLED')),
    suppression_reason text,
    notification_request_ref text NOT NULL,
    notification_message_ref text,
    attribution_ref text NOT NULL,
    consent_decision_at_handoff text CHECK (consent_decision_at_handoff IS NULL OR consent_decision_at_handoff IN ('ALLOW','DENY','UNKNOWN')),
    consent_eligible_at_handoff boolean,
    consent_checked_at timestamptz,
    consent_as_of timestamptz,
    consent_source_version bigint,
    policy_checked_at timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_event_version bigint NOT NULL DEFAULT 0 CHECK (last_event_version >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    handed_off_at timestamptz,
    completed_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE(id,business_id),
    UNIQUE(business_id,run_id,target_id,channel),
    UNIQUE(notification_request_ref),
    UNIQUE(attribution_ref),
    CONSTRAINT crm_campaign_delivery_run_fk FOREIGN KEY(run_id,business_id) REFERENCES crm_campaign_runs(id,business_id) ON DELETE CASCADE,
    CONSTRAINT crm_campaign_delivery_target_fk FOREIGN KEY(target_id,business_id) REFERENCES crm_campaign_targets(id,business_id),
    CONSTRAINT crm_campaign_delivery_profile_fk FOREIGN KEY(customer_profile_id,business_id) REFERENCES vendor_customer_profiles(id,business_id),
    CONSTRAINT crm_campaign_delivery_consent_evidence_ck CHECK (
      (consent_checked_at IS NULL AND consent_decision_at_handoff IS NULL AND consent_eligible_at_handoff IS NULL)
      OR (consent_checked_at IS NOT NULL AND consent_decision_at_handoff IS NOT NULL AND consent_eligible_at_handoff IS NOT NULL)
    )
);
CREATE INDEX crm_campaign_deliveries_run_idx ON crm_campaign_deliveries(business_id,run_id,created_at,id);
CREATE INDEX crm_campaign_deliveries_due_idx ON crm_campaign_deliveries(next_attempt_at,id) WHERE status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT');
CREATE INDEX crm_campaign_deliveries_profile_idx ON crm_campaign_deliveries(business_id,customer_profile_id,created_at DESC);

CREATE TABLE crm_campaign_delivery_history (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    delivery_id text NOT NULL,
    from_status text,
    to_status text NOT NULL,
    reason_code text,
    source text NOT NULL CHECK (source IN ('VENDOR','NOTIFICATION')),
    source_event_id text,
    source_version bigint,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT crm_campaign_delivery_history_fk FOREIGN KEY(delivery_id,business_id) REFERENCES crm_campaign_deliveries(id,business_id) ON DELETE CASCADE
);
CREATE INDEX crm_campaign_delivery_history_idx ON crm_campaign_delivery_history(business_id,delivery_id,occurred_at,id);

-- A successful handoff consumes frequency budget. It intentionally stores only
-- opaque local references, never a destination address or token.
CREATE TABLE crm_campaign_contact_history (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id),
    customer_profile_id text NOT NULL,
    campaign_id text NOT NULL,
    run_id text NOT NULL,
    delivery_id text NOT NULL,
    channel text NOT NULL CHECK (channel IN ('EMAIL','PUSH','IN_APP')),
    contacted_at timestamptz NOT NULL,
    UNIQUE(business_id,delivery_id),
    CONSTRAINT crm_campaign_contact_profile_fk FOREIGN KEY(customer_profile_id,business_id) REFERENCES vendor_customer_profiles(id,business_id),
    CONSTRAINT crm_campaign_contact_delivery_fk FOREIGN KEY(delivery_id,business_id) REFERENCES crm_campaign_deliveries(id,business_id) ON DELETE CASCADE
);
CREATE INDEX crm_campaign_contact_frequency_idx ON crm_campaign_contact_history(business_id,customer_profile_id,contacted_at DESC);

CREATE TABLE crm_campaign_delivery_event_inbox (
    event_id text PRIMARY KEY,
    payload_hash text NOT NULL,
    delivery_id text NOT NULL,
    source_service text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version > 0),
    received_at timestamptz NOT NULL DEFAULT now()
);
