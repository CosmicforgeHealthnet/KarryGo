ALTER TABLE fleet_partner_provider_invitations
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_provider_invitation_resend_count;
ALTER TABLE fleet_partner_provider_invitations
    DROP COLUMN IF EXISTS resend_count,
    DROP COLUMN IF EXISTS last_sent_at;
