-- Share notifications are hints. The capability token never enters this outbox.
CREATE TABLE IF NOT EXISTS journey_share_notification_outbox (
 id uuid PRIMARY KEY,
 session_id uuid NOT NULL REFERENCES journey_share_sessions(id),
 customer_id uuid NOT NULL REFERENCES customers(id),
 event_type text NOT NULL CHECK(event_type IN ('share.created','share.revoked','share.expired')),
 created_at timestamptz NOT NULL DEFAULT now(),
 available_at timestamptz NOT NULL DEFAULT now(),
 leased_until timestamptz,
 attempts integer NOT NULL DEFAULT 0,
 delivered_at timestamptz,
 dead_at timestamptz
);
CREATE INDEX IF NOT EXISTS journey_share_notification_pending_idx
 ON journey_share_notification_outbox(available_at,id)
 WHERE delivered_at IS NULL AND dead_at IS NULL;
