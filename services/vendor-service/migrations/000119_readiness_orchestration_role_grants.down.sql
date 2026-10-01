-- Reverses 000119 by removing only the grants it made.
--
-- It deletes by permission code rather than by role, so a grant somebody added
-- later by hand to a different role is left alone -- and so is any other
-- permission these roles hold. A rollback that cleared a role's whole grant set
-- would take unrelated capabilities away from every Owner in the estate.
DELETE FROM vendor_role_permissions rp
 USING vendor_permissions p
 WHERE p.id = rp.permission_id
   AND p.code IN (
       'vendor.readiness.read',
       'vendor.readiness.manage',
       'vendor.orchestration.read',
       'vendor.orchestration.override'
   )
   AND rp.role_id IN (
       'role_owner',
       'role_general_manager',
       'role_branch_manager',
       'role_kitchen_pastry',
       'role_shop_inventory',
       'role_analyst_auditor'
   );
