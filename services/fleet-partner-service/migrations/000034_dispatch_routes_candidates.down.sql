DELETE FROM fleet_partner_role_permissions WHERE permission_code IN('dispatch.read','dispatch.plan','dispatch.approve','dispatch.publish','dispatch.override_warning','routes.read','routes.manage','routes.optimize','routes.approve','routes.publish','routes.replan','control_tower.read','tracking.read','geofences.read','geofences.manage','alerts.read','alerts.acknowledge','alerts.resolve','alerts.replay');
DELETE FROM fleet_partner_permissions WHERE code IN('dispatch.read','dispatch.plan','dispatch.approve','dispatch.publish','dispatch.override_warning','routes.read','routes.manage','routes.optimize','routes.approve','routes.publish','routes.replan','control_tower.read','tracking.read','geofences.read','geofences.manage','alerts.read','alerts.acknowledge','alerts.resolve','alerts.replay');
DROP TABLE IF EXISTS fleet_partner_route_publication_snapshots;
DROP TABLE IF EXISTS fleet_partner_dispatch_route_history;
DROP TABLE IF EXISTS fleet_partner_assignment_candidates;
DROP TABLE IF EXISTS fleet_partner_route_stops;
DROP TABLE IF EXISTS fleet_partner_route_plans;
DROP TABLE IF EXISTS fleet_partner_dispatch_plans;
