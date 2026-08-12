CREATE TABLE IF NOT EXISTS organization_verification_submissions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), organization_id UUID NOT NULL,
 submission_id UUID NOT NULL, idempotency_key TEXT NOT NULL,
 request_fingerprint TEXT NOT NULL, snapshot JSONB NOT NULL,
 document_file_ids JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING',
 restriction_status TEXT NOT NULL DEFAULT 'NONE', source_version BIGINT NOT NULL DEFAULT 1,
 issues JSONB NOT NULL DEFAULT '[]', decided_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,submission_id), UNIQUE(idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_org_verification_org ON organization_verification_submissions(organization_id,updated_at DESC);
CREATE TABLE IF NOT EXISTS verification_service_nonces(caller TEXT NOT NULL,nonce TEXT NOT NULL,expires_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(caller,nonce));
CREATE TABLE IF NOT EXISTS verification_outbox(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),aggregate_id UUID NOT NULL,event_type TEXT NOT NULL,event_version BIGINT NOT NULL,payload JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(aggregate_id,event_type,event_version));
CREATE TABLE IF NOT EXISTS verification_commands(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), command_type TEXT NOT NULL,
 idempotency_key TEXT NOT NULL, request_fingerprint TEXT NOT NULL,
 response_body JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(command_type,idempotency_key)
);
