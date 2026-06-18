CREATE TABLE IF NOT EXISTS notification_messages (
    id uuid PRIMARY KEY,
    idempotency_key text NOT NULL UNIQUE,
    source_service text NOT NULL,
    event_type text NOT NULL,
    recipient_type text NOT NULL,
    recipient_id text NOT NULL,
    recipient_email text,
    recipient_phone text,
    channels jsonb NOT NULL DEFAULT '[]'::jsonb,
    template_key text,
    locale text NOT NULL DEFAULT 'en-NG',
    title text NOT NULL DEFAULT '',
    body text NOT NULL DEFAULT '',
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    template_data jsonb NOT NULL DEFAULT '{}'::jsonb,
    priority text NOT NULL DEFAULT 'normal',
    status text NOT NULL DEFAULT 'queued',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_messages_recipient ON notification_messages(recipient_type, recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_messages_event_type ON notification_messages(event_type);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id uuid PRIMARY KEY,
    message_id uuid NOT NULL REFERENCES notification_messages(id) ON DELETE CASCADE,
    channel text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    attempts integer NOT NULL DEFAULT 0,
    provider text,
    provider_message_id text,
    last_error text,
    next_attempt_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_message_id ON notification_deliveries(message_id);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status_channel ON notification_deliveries(status, channel);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_retry ON notification_deliveries(next_attempt_at) WHERE status = 'retrying';

CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
    id bigserial PRIMARY KEY,
    delivery_id uuid NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
    provider text NOT NULL,
    provider_message_id text,
    status text NOT NULL,
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_delivery_attempts_delivery_id ON notification_delivery_attempts(delivery_id);

CREATE TABLE IF NOT EXISTS notification_templates (
    key text NOT NULL,
    locale text NOT NULL DEFAULT 'en-NG',
    title text NOT NULL,
    body text NOT NULL,
    default_channels jsonb NOT NULL DEFAULT '["push", "websocket", "in_app"]'::jsonb,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key, locale)
);

CREATE TABLE IF NOT EXISTS notification_preferences (
    recipient_type text NOT NULL,
    recipient_id text NOT NULL,
    channel text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (recipient_type, recipient_id, channel)
);

CREATE TABLE IF NOT EXISTS notification_devices (
    id uuid PRIMARY KEY,
    recipient_type text NOT NULL,
    recipient_id text NOT NULL,
    token text NOT NULL UNIQUE,
    platform text NOT NULL DEFAULT 'unknown',
    app text NOT NULL DEFAULT 'unknown',
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_devices_recipient ON notification_devices(recipient_type, recipient_id) WHERE active = true;
