-- Reverse of 000100_commerce_fulfilment_group_parcel_mission.up.sql.
--
-- Dropped in reverse dependency order. The deferred current-plan foreign key
-- on commerce_fulfilment_groups is removed first, because it points forward at
-- a table that is dropped before the group table itself.

DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    'perm_fulfilment_read',
    'perm_fulfilment_parcel_manage',
    'perm_fulfilment_mission_manage'
);

DELETE FROM vendor_permissions
WHERE id IN (
    'perm_fulfilment_read',
    'perm_fulfilment_parcel_manage',
    'perm_fulfilment_mission_manage'
);

ALTER TABLE IF EXISTS commerce_fulfilment_groups
    DROP CONSTRAINT IF EXISTS commerce_fulfilment_groups_current_plan_fk;

DROP TABLE IF EXISTS commerce_parcel_returns;
DROP TABLE IF EXISTS commerce_parcel_custody_events;
DROP TABLE IF EXISTS commerce_vendor_missions;
DROP TABLE IF EXISTS commerce_platform_mission_refs;
DROP TABLE IF EXISTS commerce_fulfilment_plan_mission_parcels;
DROP TABLE IF EXISTS commerce_fulfilment_plan_stops;
DROP TABLE IF EXISTS commerce_fulfilment_plan_missions;
DROP TABLE IF EXISTS commerce_fulfilment_plan_versions;
DROP TABLE IF EXISTS commerce_fulfilment_parcels;
DROP TABLE IF EXISTS commerce_fulfilment_units;
DROP TABLE IF EXISTS commerce_fulfilment_groups;
