# Sections 33–35 traceability — operational evidence, recovery and transfer

## Authority map

| Capability | Canonical authority | Implementation status |
|---|---|---|
| Commercial Parcel identity and manifest source | Vendor fulfilment-group Parcel | Reused; identity is never recreated |
| Dispatch manifest freeze and custody evidence | Dispatch Commerce mission/stop/parcel runtime | Implemented in migration `000043` and `commercemission` |
| Evidence metadata | Canonical mission authority | Implemented as append-only Dispatch evidence; Vendor stores an authenticated projection |
| Media bytes and access | Media File Service | Reused; mission databases store opaque references only |
| Pickup/delivery/provider-transfer challenge | Existing `shared/go/handover` HMAC challenge authority | Reused; transfer has a separate provider-bound challenge step |
| Delivery exceptions and recovery | Dispatch Commerce mission | Implemented with closed reason/action vocabularies |
| Prepared Food vs Shop return policy | Vendor commercial product class; enforced by Dispatch return contract | Implemented; prepared Food cannot enter Shop inspection/restock |
| Provider replacement and transfer | Dispatch marketplace + canonical Dispatch mission | Implemented for Dispatch provider-to-provider custody |
| Provider earnings and settlement | Payment Wallet Service | Reused unchanged; old trip completion is frozen during replacement so no second full earning is created |

## Evidence and custody chain

Each logical pickup stop retains its own manifest reference/version, exact
Parcel set, package count and pickup challenge. Successful pickup is the freeze
boundary: the stop and Parcel manifest bindings become immutable, pickup
evidence is appended, and custody history opens for the assigned provider in
the same transaction. Damage, discrepancy, failed-attempt, return and transfer
evidence use controlled types, UTC-aware capture times, bounded coordinates and
opaque media references. Corrections append evidence; update/delete is blocked
by the database.

Post-custody replacement preserves the old provider as custodian until the
assigned replacement completes a two-provider challenge bound to the mission,
old and new provider, exact Parcel set, package count, manifest fingerprint and
location. Completion closes old custody, opens replacement custody, invalidates
the old trip handover, regenerates the receiver challenge and keeps the same
mission and Parcel identities. A direct post-custody provider reassignment is
not supported.

## Integration boundaries

- Vendor's `PlatformMissionRef` remains projection-only. Migration `000105`
  adds an append-only authenticated operational-event projection, not mission
  authority.
- Dispatch sends evidence, exception, return and transfer facts through its
  existing targeted transactional outbox. Vendor binds every event to its own
  reserved mission operation and Parcel scope before storing the projection.
- Hauling retains its existing single-pickup proof model. No safe multi-Parcel
  provider-transfer authority was fabricated in this work.
- Wallet remains the sole ledger. Dispatch and Vendor do not calculate or move
  provider earnings.
- Section 31 Vendor secure handover remains `PARTIAL / DEPENDENCY_GATED`.
  Vendor Rider to platform-provider post-custody transfer remains
  `NOT_PRESENT`; Dispatch provider-to-provider transfer does not change that.

## Schema and HTTP surface

- Dispatch: `000042` before, `000043` after.
- Vendor: `000104` before, `000105` after.
- Hauling remains at `000013`; Wallet remains at `000012`.
- Dispatch adds six provider commands and one HMAC-v2 evidence read. Vendor,
  Hauling and Wallet add no routes.
- Fully enabled Vendor route inventory remains 849. Dispatch production route
  registrations change from 149 to 156. Hauling remains 99.
- OpenAPI: `services/driver-dispatch-delivery-service/docs/openapi/commerce-dispatch-delivery.openapi.json`.

## Certification boundary

Migration shape, policy, routing/OpenAPI, package tests, build, vet, formatting
and diff checks run without PostgreSQL. Migration replay and race tests use only
`DISPATCH_TEST_DATABASE_URL`, create an isolated schema and skip truthfully when
that canonical environment variable is absent.

Sections 32 and 36 were not implemented.
