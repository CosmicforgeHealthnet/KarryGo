-- Section 28 gap closure: the organization closure workflow
-- (internal/features/organizations/usecases/closure.go) already implemented
-- RequestClosure, AssessClosure, ExecuteClosure and GetClosureStatus, but
-- ExecuteClosure's own precondition (request.Status ==
-- models.ClosureStatusApproved) could never be satisfied by any existing
-- code path -- nothing in the service ever moved a request from
-- APPROVAL_REQUIRED to APPROVED. The workflow was permanently stuck one
-- step before completion. This migration adds the permission the new
-- ApproveClosure/RejectClosure/AssessClosure/ExecuteClosure HTTP routes
-- require to close that gap.
--
-- Deliberately its own permission, not reusing organization.close (which
-- only ever gated the INITIAL request): approving or executing a closure is
-- materially more consequential than requesting one -- a request is
-- reversible right up until approval, approval and execution are not.
-- Granted to rol_system_owner only (full authority over the organization),
-- unlike security.audit_chain.verify which also went to
-- rol_system_compliance -- compliance reads/audits the closure process, it
-- does not decide to shut the organization down.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('organization.closure.manage', 'organization', 'Assess, approve, reject and execute organization closure', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'organization.closure.manage' FROM fleet_partner_roles r
WHERE r.id = 'rol_system_owner'
ON CONFLICT DO NOTHING;
