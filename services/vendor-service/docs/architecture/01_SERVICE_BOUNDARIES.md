# 01 — Service Boundaries

## What Vendor Service owns

Vendor businesses; vendor brands; vendor capabilities; vendor branches; vendor staff and permissions; vendor CRM; Restaurant/Food catalogues; Cloud Kitchen configuration; Pastry/Bakery catalogue and production configuration; Shop/Grocery catalogue; inventory and stock control (a vendor's own sellable-item stock); suppliers; purchasing; goods receiving; recipes/ingredients/production management; vendor promotions; customer commerce carts; commerce checkout orchestration; parent commerce orders; vendor orders; substitution workflow; variable-weight commerce state; fulfilment groups/units/parcels/plans; vendor-owned delivery and vendor-owned riders; vendor operational reports/projections.

See [03_VENDOR_SERVICE_MODULE_MAP.md](03_VENDOR_SERVICE_MODULE_MAP.md) for the package-level breakdown.

## What Vendor Service must never become the authority for

Customer identity (owned by `customer-service`); platform provider identity (owned by each driver service); dispatch/taxi/hauling provider matching or trip lifecycle (owned by the respective driver services); the platform financial ledger or provider earnings ledger (owned by `payment-wallet-service`); the customer wallet; payment gateway processing; formal verification decisions (owned by `verification-compliance-service`); SMS/email/push infrastructure (owned by `notification-service`); permanent media storage (owned by `media-file-service`); formal dispute adjudication (owned by `support-dispute-service`); the fleet/company provider relationship (owned by `fleet-partner-service`).

Vendor Service calls into these authorities through their existing APIs (see [04_CROSS_SERVICE_INTEGRATIONS.md](04_CROSS_SERVICE_INTEGRATIONS.md)); it never queries their databases, never duplicates their business logic, and never invents a competing source of truth for anything in this list.

## Logistics authority (unchanged)

- Motorcycle → `driver-dispatch-delivery-service`.
- Car → `driver-taxi-service`.
- Van and Truck → `driver-hauling-service` (both — Vendor Service does not create a separate Van service, and never will).
- Booking/matching/execution stays inside each operational service. Vendor Service determines delivery requirements, the requested vehicle class, and a fulfilment plan; requests a quote; requests a mission; retains an opaque mission reference; consumes lifecycle events; replans when required. It never assigns a platform provider or mutates canonical platform trip state directly. See `internal/integrations/dispatch`, `/taxi`, `/hauling`.

## Fleet/logistics-company providers (unchanged)

A company-affiliated provider (managed by `fleet-partner-service`) behaves as an ordinary marketplace participant inside dispatch/taxi/hauling's own marketplace — Vendor Service does not create a special commerce-provider class for them, does not grant them matching priority, and does not bypass their verification/availability/eligibility checks. Confirmed against the actual code: `fleet-partner-service`'s `internal/features/marketplace` package is explicitly scoped ("the reserved feature boundary for marketplace request consumption and organization visibility") to *logistics job/trip offers assigned to a company's own fleet* — a demand marketplace for driver labor, not a goods-for-sale marketplace. There is no code-level overlap risk since the two live in different services with different databases, but the term "marketplace" is already load-bearing there. Vendor Service therefore avoids using "marketplace" as a primary term for its own commerce concepts, consistent with the specification's own vocabulary ("Vendor Orders", "Commerce Cart/Checkout", "Fulfilment" — never "marketplace").

Similarly, `fleet-partner-service`'s `internal/features/warehouse` package already establishes an `inventory_item` concept for 3PL warehouse stock-keeping owned by managed organizations. Vendor Service's own `inventory` (a vendor's sellable-item stock) is a different concept in a different database with different semantics, and the two must never be confused, merged, or cross-referenced by foreign key (see [Database Ownership](#database-ownership) below) — see `internal/features/inventory/doc.go` for the explicit disambiguation.

## API Gateway

`api-gateway` has no generic routing/edge layer today (`routing/` and `service_health/` are empty scaffolds — see [00_REPOSITORY_AUDIT_BASELINE.md](00_REPOSITORY_AUDIT_BASELINE.md)). Its only real surface is a B2B "Partner API" for external companies booking single-shot deliveries, unrelated to a commerce marketplace, and it already owns the term "partner" for that external-account concept — Vendor Service does not reuse "partner" for vendor accounts.

Per CLAUDE.md's existing, already-established exception ("Current local customer app may directly target `customer-service` via `CUSTOMER_API_BASE_URL` until gateway routing is completed" — the same pattern every driver-facing client already follows for dispatch/hauling), Vendor Service exposes its own direct customer- and vendor-facing routes under `/api/v1/vendor/...` rather than waiting on a generic gateway that does not exist and is out of scope to build here. Migrating those routes behind a real gateway later is a routing change, not an architecture change — no vendor-service business logic should ever depend on being reached through a gateway specifically.

## Section 6 boundary

Vendor Service is authoritative for VendorBusiness, VendorCapability, VendorBrand, VendorBranch, hours, radius service area, explicit operations settings, and BranchCapability. It does not own media bytes, verification decisions, catalogue items, delivery pricing, routing, or logistics execution. Media references remain opaque identifiers and the radius is only vendor configuration.

## Database ownership

Vendor Service gets its own PostgreSQL database (`vendor_service`, default local port `5445`) and its own Redis instance (default local port `6390`). No cross-service foreign keys. External entities (customer_id, wallet payment reference, dispatch/taxi/hauling mission reference, verification case reference, media reference, support case reference) are stored only as opaque text references, never joined against another service's tables. See `internal/database` and `migrations/`.
