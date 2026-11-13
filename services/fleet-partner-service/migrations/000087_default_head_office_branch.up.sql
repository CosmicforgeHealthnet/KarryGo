-- Every organization needs an active default branch before it can be
-- activated, but nothing created one. Organizations now get a "Head Office"
-- branch when they are created; this backfills one for every organization
-- that has no branch at all. Organizations that already have a branch are
-- left alone.
INSERT INTO fleet_partner_branches (id, organization_id, code, name, type, address, timezone, status, version)
SELECT 'brn_' || replace(gen_random_uuid()::text, '-', ''), o.id, 'HQ', 'Head Office', 'OFFICE', '', o.timezone, 'ACTIVE', 1
FROM fleet_partner_organizations o
WHERE NOT EXISTS (SELECT 1 FROM fleet_partner_branches b WHERE b.organization_id = o.id)
ON CONFLICT (organization_id, code) DO NOTHING;
