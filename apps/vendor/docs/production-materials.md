# Production materials

Open More > Production materials > Ingredients or Packaging.

Implemented: server search and status filters, detail views, creation, editing and explicit archive confirmation. Fields match the productionmaster handlers. Ingredient expiry requirements require perishable tracking. Update/archive use the current version through If-Match; all writes use Idempotency-Key. A request journal preserves the original body, key and version across uncertain outcomes and reopening. Other material writes pause until that outcome is resolved.

Mock supplies independent ingredient and packaging fixtures, prefilled creation forms, version conflicts and exact request replay. Live uses productionMasterApi and the signed-in vendor session. Mock does not certify database-backed production dependency guards or real permission assignments.

Conversions and branch configuration are implemented from each material detail screen. Conversions list, create and edit the backend ratio, source context, mode and status; archiving is the supported status update with confirmation. The destination unit and measurement family match the parent material. Non-standard exact conversions require a material-specific source; positive integer ratios remain bounded by the server limit.

Branch discovery uses accessible business branches. A missing configuration is presented as setup after reading the actual parent material. Configuration writes send only enabled, production_eligible, inventory_control_mode_override and inventory_item_id. This endpoint does not support If-Match, so the app does not invent optimistic locking. Changing the inherited/overridden mode never silently creates stock or mappings.

Inventory references come from paginated generic branch balances. The balance contract does not expose item-type/status metadata, so the server validates active INGREDIENT/PACKAGING ownership before saving. A saved mapping can be preserved if inventory discovery is unavailable; new reference choices are not fabricated. Creating generic inventory items remains outside these production-master routes. Mock includes separate ingredient and packaging balance fixtures and enforces their branch/type mapping. Generic production inventory remains separate from Shop inventory.

Lists use the backend limit of 200 with q/status filters; the contract has no offset pagination. No backend changes were required. Deployed Live integration and the final regression pass remain pending.
