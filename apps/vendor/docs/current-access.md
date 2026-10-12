# Current staff access

More > My access reads GET /api/v1/vendor/staff/me/access through the Access API in both modes. Loading uses the shared skeleton; errors clear the previous result and offer retry. Pull to refresh and the toolbar refresh re-read current grants. Empty access is explicit. Scope IDs remain selectable for support; no branch names are inferred.

The backend authenticates and re-resolves the active session/staff/vendor using existing RequireAuth. It takes the staff ID only from authctx, accepts no target staff/vendor input, and requires no team-management permission for self access. RoleService uses the same AuthorizationEvaluator.EffectivePermissions as existing authorization. Non-revoked assignment grants are deduplicated by permission and scope and enriched from the actual permission catalogue. Null branch_id means business-wide, not an unknown branch. The response uses Cache-Control: no-store.

This is assignment permission visibility. Record ownership, workflow state, feature flags, additional scope checks and step-up validation continue to apply in each backend operation. No role-name privileges are inferred and the page does not authorize requests. Existing server-denial handling remains in place.

Mock derives grants from current mock assignments and role permission_codes; revoked and other-staff assignments are excluded. Changes appear after refresh. Mock fixtures are not evidence of deployed grants.

Validation: changed-file Flutter analysis, two focused access tests (API integration and assignment/revocation/scope semantics), staff HTTP/usecase package compilation and 744-route contract validation passed. The updated Vendor backend must be deployed for Live access. Actual database/session/permission changes and phone/theme checks remain tracker item 5.
