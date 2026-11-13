-- Fleet identifies organizations and submissions with prefixed text ids
-- (org_..., vrq_...), not UUIDs, so every submission from Fleet failed on
-- insert. Widen them to text; existing UUID values convert losslessly.
ALTER TABLE organization_verification_submissions
  ALTER COLUMN organization_id TYPE text USING organization_id::text,
  ALTER COLUMN submission_id TYPE text USING submission_id::text;

-- Outbox aggregates follow: organization events carry Fleet's organization id.
ALTER TABLE verification_outbox
  ALTER COLUMN aggregate_id TYPE text USING aggregate_id::text;

-- Organization decisions are delivered to Fleet (ADR-006 in Fleet).
CREATE INDEX IF NOT EXISTS verification_outbox_organization_delivery_idx
  ON verification_outbox (delivery_status, next_attempt_at, lease_expires_at)
  WHERE event_type IN ('organization.verification.submitted.v1',
                       'organization.verification.status_changed.v1');
