-- FLEET-10.8: re-sending the provider association / first-login notice.
--
-- Fleet never issues provider credentials: providers sign in through their
-- owning provider service's passwordless email OTP flow. A "resend login"
-- only re-sends the association invitation (rotating its single-use token)
-- or the first-login notice for a membership still pending provider
-- acceptance or setup. These two columns are the durable rate-limit state
-- for that resend: when the last notice was handed to notification-service,
-- and how many organization-requested resends the invitation has had.
ALTER TABLE fleet_partner_provider_invitations
    ADD COLUMN IF NOT EXISTS last_sent_at timestamptz,
    ADD COLUMN IF NOT EXISTS resend_count integer NOT NULL DEFAULT 0;

ALTER TABLE fleet_partner_provider_invitations
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_provider_invitation_resend_count;
ALTER TABLE fleet_partner_provider_invitations
    ADD CONSTRAINT chk_fleet_partner_provider_invitation_resend_count
    CHECK (resend_count >= 0);
