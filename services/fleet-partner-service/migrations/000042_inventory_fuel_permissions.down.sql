DELETE FROM fleet_partner_role_permissions WHERE permission_code IN(SELECT code FROM fleet_partner_permissions WHERE category IN('workforce','shifts','leave','qualifications','training','performance','assets','inspections','maintenance','breakdowns','inventory','fuel'));
DELETE FROM fleet_partner_permissions WHERE category IN('workforce','shifts','leave','qualifications','training','performance','assets','inspections','maintenance','breakdowns','inventory','fuel');
DROP TABLE IF EXISTS fleet_partner_fuel_approval_history;
DROP TABLE IF EXISTS fleet_partner_fuel_records;
DROP TABLE IF EXISTS fleet_partner_stock_movements;
DROP TABLE IF EXISTS fleet_partner_parts;
