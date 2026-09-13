# Section 43 — API Contract Families Traceability

## Certification result

Section 43 inventories the existing fully enabled production router; it does not
replace mature paths with speculative prefixes. The before and after router
surface is 857 routes. No route was added, renamed, removed, or moved between
authorities. The 867 declarations across 48 split OpenAPI documents resolve to
857 unique production operations; the ten duplicate declarations are explicit,
accepted overlaps between mature per-section contracts.

The deterministic machine artifact is
`SECTION43_ROUTE_PARITY.json`. It lists registered routes, OpenAPI routes,
approved exclusions, duplicates, family counts, and both empty mismatch sets.

## Before/after family matrix

| Contract family | Before | Section 43 result | Actual production boundary | Authority / notes |
|---|---|---|---|---|
| Vendor Authentication | PASS | PASS | `/api/v1/vendor/auth` | Existing Vendor authentication/session authority; 7 routes; rate limits retained |
| Vendor Business | PASS | PASS | `/api/v1/vendor` mature business, staff, IAM and settings paths | Vendor membership and object policy derive business/branch scope |
| Vendor CRM | PASS | PASS | `/api/v1/vendor/crm` | Privacy-minimized Vendor projections; Customer retains reusable preferences |
| Vendor Catalogue | PASS | PASS | Existing `/api/v1/vendor/food`, `/shop`, `/pastry`, taxonomy and production paths | Existing Food/Shop split retained; no pharmacy routes |
| Vendor Inventory | PASS | PASS | Existing `/api/v1/vendor/inventory`, stock and reservation paths | Branch-scoped Inventory authority; optimistic writes retained |
| Vendor Purchasing | PASS | PASS | `/api/v1/vendor/purchasing` and mature supplier/goods-receipt paths | Vendor purchasing authority; financial objects remain references |
| Vendor Orders | PARTIAL metadata | PASS | `/api/v1/vendor/commerce/orders` | Idempotency and If-Match declarations aligned with existing middleware/handlers |
| Vendor Delivery | PASS | PASS | Existing `/api/v1/vendor/delivery` and `/fulfilment` projections | Execution remains Dispatch/Hauling/Taxi Goods/VendorMission authority |
| Customer Food | AUTHORITY_PRESERVED | PASS | Existing internal Food discovery contract plus commerce cart/checkout | No duplicate public discovery route fabricated |
| Customer Shop | AUTHORITY_PRESERVED | PASS | Existing internal Shop search/catalogue and commerce cart/checkout | Restricted-goods checks retained; pharmacy excluded |
| Commerce Cart/Checkout/Order | PARTIAL metadata | PASS | `/api/v1/commerce` | Customer subject is token-derived; retry/version headers are explicit on critical writes |
| Internal Integrations | PASS | PASS | Existing `/internal/vendor/v1`, `/api/v1/internal`, and mature root internal paths | HMAC v2, allowlisted callers, nonce/timestamp/method/path/body binding retained |
| Advanced Commerce | PASS / DEPENDENCY_GATED | PASS / DEPENDENCY_GATED | Existing basket, optimization, RFQ, recurring and fulfilment paths | Existing flags/adapters retained; no algorithm or money authority fabricated |
| Order Guarantee | FUTURE | FUTURE_DISABLED | No production route | Zero operations; no money logic implemented |

The family classifier records 7 authentication, 121 CRM, 160 catalogue, 73
inventory, 56 purchasing, 29 Vendor order, 27 delivery, 39 core commerce, 23
advanced-commerce, 80 internal and 242 remaining Vendor business/operational
routes. The three service probes are explicit ownership exclusions and remain
counted in the machine inventory. The exact route-by-route inventory is in the
JSON artifact rather than duplicated here.

## Proposed boundary mapping and retained compatibility

The specification's prefixes are architectural families. Existing paths remain
canonical where clients and earlier sections already depend on them. In
particular, `/api/v1/vendor/commerce/orders`, `/api/v1/vendor/food`, mature Shop
paths, `/api/v1/commerce`, `/internal/vendor/v1`, `/api/v1/internal`, and legacy
root internal paths are retained. `ready_for_pickup` remains a compatibility
alias for `ready-for-pickup`; neither was removed. No new deprecation was
introduced. Existing deprecation metadata remains authoritative.

The ten approved duplicate OpenAPI declarations are the two fulfilment-plan
operations shared by the optimization and Section 17C contracts, plus eight
shop-picking/substitution operations shared by the Section 18 and focused Shop
contract. `TestSection43ApprovedOpenAPIDuplicatesAreExplicit` rejects any
unapproved overlap.

## OpenAPI closure

- All 48 Vendor JSON OpenAPI documents are parsed and validated.
- All 867 operations have globally unique `operationId` values. Existing IDs
  were preserved; only 167 previously missing IDs were deterministically added.
- All operations have response contracts. The 66 genuinely missing response
  maps in Sections 16A–16D now describe actual 200/201 production status and
  standard safe failures.
- All template parameters are declared as required path parameters. The 161
  genuine omissions were added without changing route shapes.
- Local `$ref` targets and referenced security schemes resolve. Misplaced
  Section 5 reusable responses and Section 7 request bodies were corrected into
  `components` without changing runtime semantics.
- Sections 16A–16D now declare Vendor bearer security. Their writes declare the
  existing idempotency requirement; known version-sensitive operations retain
  their existing If-Match convention.
- Vendor-order and substitution write contracts now expose the middleware's
  `Idempotency-Key` requirement and handler-level `If-Match` requirement.
- The repository tool `tools/section43-openapi-normalize` validates all split
  contracts and can add only deterministic missing structural metadata with
  `-write`; it never renames an accepted operation ID or route.

## Security, retry, and concurrency rules

Customer identity is obtained from the verified Customer bearer subject.
Customer-owned cart, plan, basket, order, change and substitution stores scope
reads and writes by that subject; request bodies do not become identity
authority. Vendor staff identity and business/branch reach come from the Vendor
token and membership/object policy. Body identifiers are only resource inputs
after server-side scope validation.

Public command middleware provides tenant-scoped request fingerprints: the same
key and fingerprint replays the original result, a different fingerprint returns
409, and another tenant cannot replay the key. Section 43 added the missing
middleware application to Section 16D cost commands, matching the already
injected store and adjacent Section 16 command families. Version-sensitive
writes continue to use If-Match/expectedVersion and return 409 for missing or
stale versions. Section 21 operations that historically carry an idempotency key
inside their typed command remain on that compatible convention.

Internal routes remain on shared HMAC v2. Authentication binds caller, method,
path, body digest, timestamp and nonce; mutation payloads retain correlation,
causation and operation/event identity where implemented. Public bearer tokens
cannot authenticate those routes.

The complete OpenAPI write inventory is 530 operations: 470 explicitly declare
`Idempotency-Key`; the other 60 were individually reviewed in their existing
families and intentionally retain their compatible contract:

| Classification | Operations reviewed | Reason |
|---|---:|---|
| IDEMPOTENCY REQUIRED | 470 | Header is explicit and enforced by shared middleware or the operation handler; includes checkout quote/order create, Vendor order decisions, substitutions, purchasing, Food operations, promotions, mission and handover commands |
| IDEMPOTENCY NOT REQUIRED | reviewed within the 60 compatible exceptions | Authentication/session transitions, quote-only planners, previews/search-like POSTs, or naturally idempotent versioned replacement/disable operations that do not create a second economic command |
| IDEMPOTENCY PROVIDED BY AUTHORITY | reviewed within the 60 compatible exceptions | Internal event/source-version dedupe, immutable proposal/occurrence identity, Section 21 typed command keys, and source-owned Inventory/Wallet/logistics operation identity |

The 60 compatible exceptions are confined to the already mature Section 5,
8A/8E/8F, 12 discovery, 17C, fulfilment, basket, Section 21 and internal event
contracts. They are not excluded from route/OpenAPI parity. Section 43 did not
pretend that a retry header exists where the production handler uses a typed
operation identity or performs a side-effect-free calculation.

## External authorities and gateway

Payment Wallet remains authoritative for holds, allocations, capture,
settlement, refunds, ledger truth and customer debit. Vendor returns only
operation/reference/status projections. Media File remains authoritative for
bytes and signed access; Vendor stores opaque references. Customer Service
retains reusable preferences and canonical customer identity. Verification
Compliance retains verification truth. Dispatch, Hauling, Taxi Goods and
VendorMission retain mission, provider, custody and handover authority.

OTP/QR remains relay/status only in Vendor. A presented credential is relayed in
memory to the frozen mission authority and is not exposed as CRUD, echoed, or
persisted by Vendor.

`services/api-gateway` has no Vendor Service proxy or route mapping. Therefore
there is no gateway mapping to synchronize and no Vendor internal route is made
public by that service. Vendor public ingress remains the existing deployment
boundary; gateway tests were not modified.

## Exclusions and out-of-scope items

`GET /health`, `GET /ready`, and `GET /api/v1/vendor/meta` are the only approved
service-platform exclusions from the human route-ownership assertion. They are
still present in both machine inventories. Pharmacy/medicine commerce is out of
scope and remains rejected. Section 37 stays intentionally skipped. No Section
44+ work, frontend code, speculative route, migration, or Order Guarantee money
logic was added.

## Certification evidence

- `TestProductionRouteCountIsTheAcceptedSurface`: before/after count pinned at 857.
- `TestEveryProductionRouteIsDocumented`: router to OpenAPI/register parity.
- `TestNoOpenAPIOperationIsPhantom`: OpenAPI to router parity.
- `TestSection43ParityArtifactIsCurrent`: deterministic zero-gap artifact.
- `TestSection43AllOpenAPIDocumentsAreStructurallyValid`: operation IDs,
  responses, refs, security scheme references and path parameters.
- `TestSection43CriticalWriteContractsDeclareRetryAndConcurrency`: critical
  quote, order, substitution and handover retry/version contracts.
- Existing Section 40–42 suites certify object authorization, IDOR protection,
  HMAC v2, replay/tamper resistance, idempotency, expectedVersion, privacy and
  pharmacy exclusion.

Final command results are recorded in `IMPLEMENTATION_STATUS.md` after the full
Vendor regression, repository test/vet/build, formatter, OpenAPI and Git checks.
