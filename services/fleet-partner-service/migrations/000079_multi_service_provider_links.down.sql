-- Reverse of 000079.
--
-- The three source_service CHECKs are narrowed back to the two services they
-- named before. Any taxi projection row would violate the restored constraint,
-- so those rows are removed first: they are a projection of state that
-- driver-taxi-service still holds authoritatively, and re-projecting them is
-- what the inbox is for. Nothing canonical is lost.
DELETE FROM fleet_partner_private_execution_projections WHERE source_service = 'driver-taxi-service';
DELETE FROM fleet_partner_tracking_projections WHERE source_service = 'driver-taxi-service';
DELETE FROM fleet_partner_tracking_event_inbox WHERE source_service = 'driver-taxi-service';

ALTER TABLE fleet_partner_private_execution_projections
    DROP CONSTRAINT IF EXISTS fleet_partner_private_execution_projections_source_service_check;
ALTER TABLE fleet_partner_private_execution_projections
    ADD CONSTRAINT fleet_partner_private_execution_projections_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service', 'driver-hauling-service'));

ALTER TABLE fleet_partner_tracking_projections
    DROP CONSTRAINT IF EXISTS fleet_partner_tracking_projections_source_service_check;
ALTER TABLE fleet_partner_tracking_projections
    ADD CONSTRAINT fleet_partner_tracking_projections_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service', 'driver-hauling-service'));

ALTER TABLE fleet_partner_tracking_event_inbox
    DROP CONSTRAINT IF EXISTS fleet_partner_tracking_event_inbox_source_service_check;
ALTER TABLE fleet_partner_tracking_event_inbox
    ADD CONSTRAINT fleet_partner_tracking_event_inbox_source_service_check
        CHECK (source_service IN ('driver-dispatch-delivery-service', 'driver-hauling-service'));

DELETE FROM fleet_partner_role_permissions
 WHERE permission_code IN ('managed_members.read', 'managed_members.manage',
                           'managed_assets.read', 'managed_assets.manage');
DELETE FROM fleet_partner_permissions
 WHERE code IN ('managed_members.read', 'managed_members.manage',
                'managed_assets.read', 'managed_assets.manage');

DROP TABLE IF EXISTS fleet_partner_managed_asset_links;

ALTER TABLE fleet_partner_organization_provider_memberships
    DROP CONSTRAINT IF EXISTS fk_fleet_membership_member_tenant;
DROP INDEX IF EXISTS idx_fleet_memberships_member;
DROP INDEX IF EXISTS uq_fleet_member_live_authority;
ALTER TABLE fleet_partner_organization_provider_memberships
    DROP COLUMN IF EXISTS member_id;

ALTER TABLE fleet_partner_managed_provider_members
    DROP CONSTRAINT IF EXISTS uq_fleet_managed_member_tenant;
DROP TABLE IF EXISTS fleet_partner_managed_provider_members;
