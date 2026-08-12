CREATE TABLE IF NOT EXISTS vendor_verification_cases (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 client_reference TEXT NOT NULL UNIQUE,
 business_id TEXT NOT NULL,
 dimension TEXT NOT NULL CHECK (dimension IN ('IDENTITY','BUSINESS','BANK','LOCATION','BRAND','FOOD_KITCHEN')),
 subject_type TEXT NOT NULL CHECK (subject_type IN ('BUSINESS','REPRESENTATIVE','BRANCH','BRAND','CAPABILITY','BRANCH_CAPABILITY')),
 subject_ref TEXT NOT NULL,
 document_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
 request_fingerprint TEXT NOT NULL,
 idempotency_key TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL DEFAULT 'PENDING_REVIEW' CHECK (status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
 source_version BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
 reason_code TEXT,
 remediation TEXT,
 expires_at TIMESTAMPTZ,
 observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX vendor_verification_cases_business_idx ON vendor_verification_cases(business_id,dimension,updated_at DESC);

CREATE TABLE IF NOT EXISTS vendor_verification_case_history (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 case_id UUID NOT NULL REFERENCES vendor_verification_cases(id),
 source_version BIGINT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
 reason_code TEXT,
 remediation TEXT,
 expires_at TIMESTAMPTZ,
 observed_at TIMESTAMPTZ NOT NULL,
 decided_by TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(case_id,source_version)
);

ALTER TABLE verification_outbox
 ADD COLUMN IF NOT EXISTS delivery_status TEXT NOT NULL DEFAULT 'PENDING'
   CHECK (delivery_status IN ('PENDING','FAILED','DELIVERED','DEAD_LETTER')),
 ADD COLUMN IF NOT EXISTS delivery_attempts INTEGER NOT NULL DEFAULT 0,
 ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 ADD COLUMN IF NOT EXISTS lease_owner TEXT,
 ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ,
 ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
 ADD COLUMN IF NOT EXISTS last_error TEXT,
 ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS verification_outbox_vendor_delivery_idx
 ON verification_outbox(delivery_status,next_attempt_at,lease_expires_at)
 WHERE event_type='vendor.verification.status_changed';
