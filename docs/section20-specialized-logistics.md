# Section 20: specialized logistics

Section 20 extends existing orders, shipments, assets, trips, custody, Media and execution authorities; it does not duplicate them. The shared organization capability authority from migration `000004` is extended with typed configuration and immutable history. Each organization must explicitly enable a module before any module data is read or written, and the matching global module flag must also be on.

Supported capability codes are `COURIER_LAST_MILE`, `TAXI_PASSENGER`, `TRUCKING_HAULAGE`, `HEAVY_SPECIALIZED`, `FREIGHT_FORWARDING`, `COLD_CHAIN`, `MEDICAL_SENSITIVE`, `AGRICULTURAL_FOOD`, `MOVING_RELOCATION`, and `TOWING_RECOVERY`. Typed validation rejects record types and fields outside the selected module. Commands require expected version and creation/command routes require the shared idempotency store. Exact routes and permissions are in `openapi/section20-specialized-logistics.openapi.json`.

Taxi is data-only and always fails closed for execution; configuration cannot enable a taxi execution client. Taxi records reject financial fields. Haulage records reuse Hauling authority references. Freight and cross-border records hold broker/customs references but calculate no duty or tax. COD, detention, demurrage, insurance and other monetary data are external references only—Fleet creates no wallet or ledger mutation.

Cold-chain sensor versions are monotonic; excursions create quarantine state for review. Medical records use restricted visibility and immutable access audit. Evidence remains an opaque, ownership-validated Media reference. Specialized permits and documents are checked by the shared worker, which also evaluates cold-chain excursions and reconciliation gaps.

All write and worker flags default false. `SPECIALIZED_LOGISTICS_ENABLED` is the parent switch; each module has its own named flag and `SECTION20_WORKERS_ENABLED` controls scheduling. Organization capabilities default disabled. Roll out the schema and OpenAPI first, enable the parent and one module for selected organizations, then start workers. Roll back by disabling the module and parent flags before applying the paired down migration.
