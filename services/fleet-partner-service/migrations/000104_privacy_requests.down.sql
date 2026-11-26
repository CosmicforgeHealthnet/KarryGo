DELETE FROM fleet_partner_role_permissions WHERE permission_code IN('privacy.manage','privacy.decide','security_events.read');
DELETE FROM fleet_partner_permissions WHERE code IN('privacy.manage','privacy.decide','security_events.read');
DROP TRIGGER IF EXISTS trg_fleet_privacy_request_history_append_only ON fleet_partner_privacy_request_history;
DROP TABLE IF EXISTS fleet_partner_privacy_request_history;
DROP TABLE IF EXISTS fleet_partner_privacy_requests;
