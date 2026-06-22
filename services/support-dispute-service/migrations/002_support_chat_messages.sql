CREATE TABLE support_chat_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  complaint_id UUID NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
  sender_type TEXT NOT NULL
    CHECK (sender_type IN ('customer', 'taxi_provider', 'dispatch_provider', 'hauling_provider', 'admin')),
  sender_id TEXT NOT NULL,
  content TEXT NOT NULL,
  media_url TEXT,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX support_chat_messages_complaint_created_idx
  ON support_chat_messages(complaint_id, created_at);

CREATE INDEX support_complaints_complainant_service_status_idx
  ON complaints(complainant_id, complainant_type, service_type, status);
