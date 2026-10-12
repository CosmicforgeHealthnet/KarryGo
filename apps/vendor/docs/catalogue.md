# Catalogue workspace

The Catalogue tab opens branch-scoped food menus or business-wide shop products. Detailed menu, product, food-item and modifier-group management uses full screens. Section, food-variant and modifier edits use short sheets. Archive, restore and publication commands have explicit confirmations. Editors keep unsaved input after a rejected save and offer discard confirmation when leaving.

## Implemented workflows

- Food menus: create for a real branch capability, edit, activate, pause and archive.
- Sections: create, edit, archive and reorder; open a section to manage its food items.
- Food items: create, edit integer prices and vendor dietary/allergen codes, activate, pause and archive.
- Food variants: create, edit, inherit or override base price, archive and reorder.
- Shared modifier groups: create, edit selection limits and archive; manage priced modifiers and their order.
- Item options: attach existing groups, inspect their shared definitions, detach and reorder attachments.
- Shop categories: create, edit parent/name/slug/position, archive and restore.
- Shop products: create/edit category, class, descriptive information and operating-model membership; activate, pause and archive.
- Shop variants: create/edit packaging, SKU, physical dimensions, prices and handling requirements; pause/reactivate or archive.
- Branch listings: select an existing product variant, configure retail/wholesale selling, price overrides and quantity rules, activate/deactivate and archive.
- Paginated collections expose Load more. Food ordering commands are disabled until the complete active collection is loaded.

## Contract and state rules

Live uses the existing staff APIs, with their real fields and entity versions. There is no Live-to-Mock fallback. Commands use idempotency keys; an identical retry retains its key. Editors submit changed fields rather than echoing unchanged pricing or wholesale fields, which have separate server permissions. A 403 disables the rejected command. Conflicts and uncertain outcomes require a refresh before further writes. A successful write followed by a failed read stays successful and clearly requests a refresh.

The backend remains responsible for permission, publication, parent/category, operating-model, pricing and archive guards. Available UI actions do not promise a command will pass those checks. Archived parent resources disable descendant edits.

Money entry converts decimal text to integer minor amounts without floating-point rounding, matching the app's existing two-decimal currency presentation. Listing labels come from real product and variant reads, not synthetic backend fields. No stock values or availability guarantees are invented.

Mock mode is the default. Catalogue edits persist per business for the current mock client's lifetime. New entities use the server's draft/active defaults, branch listings retain real product/variant relationships, and stale versions and idempotent retries are exercised. Mock presets supply form values for click-through testing; Live creation forms do not inherit them. Mock fixtures are not a replacement for the server's complete policy enforcement.

## Advanced catalogue configuration

Menus open **Menu schedules** directly. Food items, products, variants and branch listings open **Selling settings** beside their existing details. Shared trade measures are also accessible from the Shop products list, including an empty catalogue. Branch listings expose **Selling models** for their branch.

- Menu schedules: create, edit and archive weekly local-time windows, including overnight service and optional start/end dates. Day chips and native time/date pickers use ISO weekdays and display the backend's branch timezone. Clearing dates uses the explicit clear flags. Overlapping windows are allowed by the server; a menu without active windows is unrestricted by schedules.
- Food operational availability: Available, Sold out or Paused, optional reason and future restore time. This state is independent of publishing the item. Selling status displays the authoritative effective decision.
- Food profiles: preparation ranges, logistics handling, branch-specific preparation/price overrides and availability-source configuration. Profiles use their own version, including version zero for an unconfigured profile; clearing restores the server's defaults/inheritance. Inventory-linked availability remains dependent on the actual inventory authority and does not fabricate stock.
- Trade measures: read platform definitions; create/edit/archive owned definitions and add/remove owned regional aliases. Exact MASS/VOLUME/COUNT quantities remain distinct from local or approximate measures. Platform definitions and platform aliases cannot be edited or removed.
- Calibrations and sell units: actual product, variant, measure and optional branch selectors, approximate quantities, controlled calibration sources, fractions, measurement rules, retail/wholesale prices and order constraints. Backend validation remains authoritative for compatible modes, conversion and permissions.
- Package relationships: select two real variants and an actual ratio, then archive when needed. No ratio is inferred from package names.
- Fresh attributes: only types admitted by the product's actual class, controlled options with editable customer labels, optional required selection and a single default at most. Upsert uses the attribute's version and can restore an archived attribute.
- Variant identifiers: create and archive supported SKUs/trade identifiers with optional package level. Display the actual external-verification state; adding an identifier does not verify it.
- Branch sell units: **Choose sell units** updates the listing's selected IDs using the listing version. Newly created product sell units are not automatically enabled on a branch. Per-unit enablement, calibration and price overrides use the listing-unit version, preserving the backend's two separate scopes.
- Quantity pricing: create/edit/archive bounded or open-ended ranges against the listing or an already enabled sell unit. Current server price resolution applies quantity bands in its wholesale path; the UI does not promise retail discounts from the stored flag.
- Branch selling models: select only model codes returned as eligible by the server, choose an optional primary model and archive the configuration with an explicit warning about affected listings.
- Food and Shop price history: paginated read-only records, using actual amounts, inheritance and recorded changes.

Short configurations use sheets; sell units, schedules, measures, calibrations and fresh configuration use full editors. Existing native transitions, teal actions, purpose-specific icons, discard confirmation and save/archive feedback are retained. Merely opening an editor does not start a save spinner. Required related selectors read the actual backend collections; Live never substitutes Mock choices.

Mock configuration is shared with the existing catalogue records, persists for the current mock client's lifetime and supplies creation presets. It exercises independent versions, idempotent retries, immutable platform definitions, branch selection, supported fresh choices, compatible calibrations, packaging cycles and nonoverlapping quantity bands. Its schedule-time evaluator supports the existing Africa/Lagos and UTC fixtures; Live evaluates timezones on the server. The mock is a representative fixture, not a replica of every backend authorization or audit rule.

## Price preview and identifier lookup

Food item and branch-listing details expose **Price preview** directly. Food previews read the actual variants and attached modifier definitions, enforce each group's selection limits, and send only `variant_ref` and `modifier_refs`. The backend returns the base, selected extras and effective unit price. Changing choices clears the previous result.

Shop previews read the actual branch listing and its enabled sell units. Requests send quantity, retail/wholesale channel and an optional selected sell-unit ID. The UI displays the server's unit price, total, source, order rules and failure reason. An unpriced or unavailable answer remains nullable and is never displayed as a free item. Returned availability `UNAVAILABLE` is presented as stock information not provided, rather than as zero stock. Current backend quantity bands apply in its wholesale price-resolution path.

**Find a product** opens from the Shop products list. Enter or paste an identifier and optionally select its actual type. The server returns the matching identifier, variant and product; actions open their existing detail screens. A specific `SHOP_IDENTIFIER_NOT_FOUND` response produces a no-match state. Other 404s, permission failures and unavailable services remain errors. This increment adds text lookup, not camera scanning. External verification is shown from the actual identifier record.

These are authenticated catalogue reads. Food's structured preview uses POST but is read-only: it has no idempotency key and does not create an order or move funds. The generated food client and its regeneration source now follow the actual `PermCatalogueRead` handler instead of the older snapshot's write-header assumption. Live never falls back to sample results.

Mock previews use the same in-memory catalogue, configuration, modifier attachments, listing associations and quantity tiers as the editors. The populated fixture has an owned `DEMO-PACK-01` identifier and prefilled lookup input; Live input starts empty. Reads leave entity versions and price history unchanged.

## Remaining catalogue surfaces

Shop inventory is available from Catalogue > Shop > Inventory. See [inventory.md](inventory.md) for implemented stock/receiving/adjustment/count/alert flows and remaining inventory surfaces.

Catalogue item/product details now expose Manage photos with target-bound uploads, previews, removal and Food primary-photo selection. Mock uses real sample bytes; Live uses the authorized Vendor -> Media flow and requires deployment/scanner setup. See media.md.
