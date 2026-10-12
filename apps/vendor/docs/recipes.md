# Recipes and costs

Open More > Recipes. Select a recipe to create a draft version, then open that version to edit yield, ingredient components and packaging requirements. Validate before activation. Activation uses the current version and retires the previous active version on the backend; retirement is explicit. Retire an active version before archiving its recipe. Active/retired definitions are read-only; create a new draft for changes.

Creation selects actual catalogue targets through Food menus/sections/items, Pastry products or Cloud Kitchen packages. Food variants are selected from actual variant reads. Specialty discovery requires the deployed capability and staff access; unavailable sources are not replaced with fixtures in Live.

Component and packaging editors send only the backend input fields and replace the selected list explicitly. Yield and quantities preserve integer numerator/denominator ratios. Waste percentages map to integer basis points. Validation issues display the server messages. Scaling is a server preview and does not save a changed recipe.

Recipe-cost views include active recipe preview, version preview, latest saved snapshot and history. Snapshot creation records immutable evidence with the actual branch query. Missing totals and component costs remain unavailable; unknown evidence never becomes a zero price. Names are resolved from accessible production materials, with actual references retained when names cannot be read.

Writes use backend If-Match and Idempotency-Key support. The journal preserves the original request, including query scope, before submission and supports identical recovery after reopening. Lifecycle/validation/snapshot endpoints have no request body; the client omits it as required by their contracts. The production-material archive path also now correctly omits its body.

Mock uses a real seeded Food catalogue target and shared production materials/mappings/conversions. It exercises draft editing, validation, activation, retirement, scaling and incomplete-cost snapshots. Specialty Mock catalogues are not fabricated. Mock costing intentionally reports missing evidence; it is not a reproduction of the cost engine, database arithmetic limits, concurrency or permissions.

Material discovery reads up to 200 records. Existing references remain reviewable even if their names are unavailable. Recipe list pagination inherits the backend's scope filtering after page selection; deployed access and large scoped datasets still need final integration checks. No new backend endpoint was added. Database-backed integration and the full regression pass remain pending.
