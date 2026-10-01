# Section 25 — Fulfilment Group, Parcel and Mission Model traceability

Roadmap section numbers appear in this document only. No production source
file, directory, migration or OpenAPI contract carries one.

Owner: `services/vendor-service`, feature `internal/features/fulfilmentgroup`.

---

## 1. Audit outcome

The audit ran before any code was written. What already existed, and what did
not, decided the whole shape of this section.

| Concept | Before | Evidence found | After |
|---|---|---|---|
| FulfilmentGroup | NOT_PRESENT | `commerce_fulfilment_plans` is bound to a **checkout session**, not a commerce order | IMPLEMENTED_NOW |
| FulfilmentUnit | NOT_PRESENT | `commerce_vendor_orders` + plan groups approximate it; no unit entity | IMPLEMENTED_NOW |
| Parcel | NOT_PRESENT | only a `parcel_ref` **text column** on `commerce_delivery_execution` | IMPLEMENTED_NOW |
| Parcel manifest | NOT_PRESENT | — | IMPLEMENTED_NOW |
| FulfilmentPlan (order-scoped, versioned) | NOT_PRESENT | checkout-scoped plan with `plan_revision` + supersede | IMPLEMENTED_NOW |
| Plan versioning | PARTIAL | `ReplanBeforeCustody` bumped `PlanRevision` **in place**, in memory | IMPLEMENTED_NOW |
| PlatformMissionRef | PARTIAL | `commerce_order_missions` had mission_ref/plan_mission_ref/external_ref/status, but no group, plan-version, slot sequence, or versioned status projection | EXTENDED |
| VendorMission | NOT_PRESENT | `commerce_delivery_execution` existed in migration `000096` with **zero Go code reading or writing it** | IMPLEMENTED_NOW |
| Multi-mission group | PARTIAL | several `commerce_order_missions` per parent order already possible | EXTENDED |
| Multi-pickup platform mission | NOT_PRESENT | `shared/go/commercemission.Request` carries **one** `Pickup` and one `Dropoff` | MODELLED; execution fails closed (see §7) |
| Parcel identity preservation | NOT_PRESENT | — | IMPLEMENTED_NOW |
| Reassignment | AVAILABLE (logistics-owned) | provider choice belongs to Dispatch/Hauling | PRESERVED |
| Transfer | NOT_PRESENT | `commerce_delivery_custody_events` schema only, no code | MODELLED; no authority emits it yet (see §7) |
| Return | PARTIAL | custody-state enum + Section 24 refund policy; no return/mission linkage | IMPLEMENTED_NOW |
| Custody linkage | NOT_PRESENT | schema only, no code | IMPLEMENTED_NOW |
| Replan | PARTIAL | in-memory only; destroyed history | IMPLEMENTED_NOW |
| Plan history | NOT_PRESENT | — | IMPLEMENTED_NOW |

The two most consequential findings:

1. **`internal/features/fulfilment` is a PRE-order package.** Its `Plan` is
   keyed on a checkout session and answers "what would delivery cost". Section
   25 is post-order. Merging them would have given one type two lifecycles and
   made "which plan is in force" ambiguous, so the new aggregate is a sibling
   package rather than an extension of that one. The existing package is
   untouched.

2. **Section 22's `commerce_delivery_execution` and
   `commerce_delivery_custody_events` had no Go code at all** — migration
   `000096` created them and only a shape test referenced them. So "vendor-owned
   execution state" and "custody events" were schema stubs, not capabilities.
   They are left in place (no historical migration is edited); the working
   implementation is the new aggregate.

---

## 2. Domain authority

| Concept | Owner | Enforced by |
|---|---|---|
| FulfilmentGroup | Vendor Service | `commerce_fulfilment_groups`, `UNIQUE (parent_order_id)` |
| FulfilmentUnit | Vendor Service | `commerce_fulfilment_units`, `UNIQUE (vendor_order_id)` |
| Parcel + manifest | Vendor Service | `commerce_fulfilment_parcels`, never deleted, no DELETE route |
| FulfilmentPlan + history | Vendor Service | `commerce_fulfilment_plan_versions`, `UNIQUE (group_id, version)` |
| PlatformMissionRef | Vendor Service **projection only** | separate table; no route creates or mutates a canonical mission |
| VendorMission | Vendor Service **canonical, vendor delivery only** | `owner='VENDOR' AND authority='VENDOR'` check constraint |
| Canonical platform mission | Dispatch / Hauling / Taxi | `external_ref` treated as opaque; authority named in its own column |
| Ledger, holds, refunds, settlement | payment-wallet-service | plan stores a cost **snapshot**; no financial write anywhere in the feature |
| Dispute adjudication | support-dispute-service | untouched |
| User/provider notification | notification-service | untouched |

---

## 3. Requirement map

Requirement numbers are the source specification's own.

### Fulfilment group (1–3)

| Req | Where | Evidence |
|---|---|---|
| 1 group per CommerceOrder | `group.go` `Group`; migration `UNIQUE (parent_order_id)` | `TestGroupSurvivesReplanning`, `TestFulfilmentAggregateInvariantsAreConstraints` |
| 2 stable identity, no duplicate under concurrency | `store.go` `EnsureGroup` (insert-or-read-back on conflict) | `TestIntegration_ConcurrentReplansProduceOneVersion` (DB) |
| 3 no invented lifecycle states | `Group` carries **no status column** — the purchase's state is already derived on `commerce_parent_orders` | migration comment; `TestGroupBeforePlanningIsNotPlanned` |

### Fulfilment unit (4–7)

| Req | Where | Evidence |
|---|---|---|
| 4 one VendorOrder + branch obligation | `Unit`; composite FK `(vendor_order_id, parent_order_id)` | `TestSameBranchObligation_DistinguishesBranches` |
| 5 no duplicate active unit | `UNIQUE (vendor_order_id)`, `UNIQUE (group_id, vendor_order_id)` | shape test |
| 6 multi-branch safety | unit is keyed to the vendor order, and each vendor order has exactly one branch; `SameBranchObligation` requires branch equality | `TestSameBranchObligation_DistinguishesBranches`, `TestPlanMissions_MultiMissionMixedOwnership` |
| 7 commercial identity preserved | vendor/branch resolved from `commerce_vendor_orders` **inside the transaction**, never from a request body | `EnsureUnit`; `TestIntegration_CrossVendorParcelAccessIsRefused` |

### Parcel (8–18)

| Req | Where | Evidence |
|---|---|---|
| 8 parcel is its own concept | `parcel.go` `Parcel`, distinct from order line / vendor order / unit / mission | package doc |
| 9 identity immutable across everything | no delete path anywhere; status is a projection | `TestParcelIdentitySurvivesTheWholeJourney` |
| 10 idempotent creation | `UNIQUE (unit_id, idempotency_key)`; `Idempotency-Key` header **required** | `TestParcelInput_RequiresIdempotencyKey`, `TestIntegration_ConcurrentPackingCreatesOneParcel` |
| 11 authoritative source | business/branch denormalised from the unit; `UNIQUE (id, business_id)` for scoped FKs | `TestIntegration_CrossVendorParcelAccessIsRefused` |
| 12 manifest content | `Manifest` — refs + packed quantity + variable measure + media/handling refs, never a catalogue copy | `TestManifest_RoundTrips` |
| 13 manifest snapshot after custody | `sealed_at` set **from custody evidence** in `recordCustodyTx`; caller `SealedAt` ignored | `TestIntegration_CustodySealsTheManifest`, `TestParcelStatus_InCustodySealsTheManifest` |
| 14 parcel split | `UNIQUE (unit_id, sequence)`; several parcels per unit | `TestIntegration_UnitHoldsSeveralParcels` |
| 15 no cross-vendor merge | `CollectParcelIDs` collects, never merges | `TestBuildStops_MultiPickupOneDestination`, `TestCrossVendorParcelsKeepSeparateSources` |
| 16 cross-vendor rule | each parcel keeps its own `business_id`/`branch_id`/`vendor_order_id` | same |
| 17 status never contradicts custody | `AdvanceParcelStatus` is forward-only and derived from `ParcelStatusFor` | `TestAdvanceParcelStatus_NeverRegresses` |
| 18 reuse existing OTP/QR | `handover_ref` stores a **reference** to a credential issued elsewhere; no code generates or verifies one | `mission.go` field doc; `TestNoRouteWritesAPlatformMission` |

### Fulfilment plan (19–27)

| Req | Where | Evidence |
|---|---|---|
| 19 plan content | `Plan` — reason, planner version, policy version, delivery mode, cost snapshot, routing evidence, missions, stops, timestamps | OpenAPI `Plan` schema |
| 20 monotonic versions | `UNIQUE (group_id, version)`; `ActivatePlan` computes `current + 1` under a row lock | `TestIntegration_ConcurrentReplansProduceOneVersion` |
| 21 current-plan pointer | `current_plan_id`/`current_plan_version` updated in the **same transaction** as the version and its slots | `ActivatePlan`; `commerce_fulfilment_groups_plan_pointer_chk` |
| 22 concurrent replan protection | row lock + `ExpectedVersion` + `UNIQUE (group_id, version)`; loser gets `ErrStalePlan`. Entry point `Service.Replan` | `TestIntegration_ConcurrentReplansProduceOneVersion`, `TestBuildPlanMissions_SlotRefsAreStableAndDistinct` |
| 23 plan immutability | superseded rows are marked, never edited | `TestIntegration_ConcurrentReplansProduceOneVersion` asserts v1's reason is unchanged |
| 24 controlled replan reason | `PlanReason` **reuses** the delivery feature's `OverflowTrigger` values verbatim | `TestPlanReasonsReuseOverflowVocabulary` |
| 25 Section 22 hybrid integration | the six triggers are plan reasons; no parallel planner was built | `TestPlanReasonsReuseOverflowVocabulary` |
| 26 pre-custody replan preserves identities | group/unit/parcel untouched; a new version is appended | `TestReplanChangesTheMissionNotTheIdentities` |
| 27 post-custody change | `AssignVendorMission` refuses reassignment once `InCustody()`; parcel changes are custody events, not recreation | `store.go`; `TestParcelIdentitySurvivesTheWholeJourney` |

### Platform missions (28–33, 55–56, 62)

| Req | Where | Evidence |
|---|---|---|
| 28 projection content | `PlatformMissionRef` — group, plan, slot, authority, mission_ref, external_ref, link state, versioned status, timestamps | OpenAPI schema |
| 29 opaque identifier | `external_ref` is never parsed; `authority` is an explicit column | package doc; `TestNoRouteWritesAPlatformMission` |
| 30 controlled authority vocabulary | `CHECK (authority IN ('DISPATCH','HAULING','TAXI'))`; authority chosen by the planner, never by a caller | migration; no request DTO carries an authority |
| 31 mission creation through the owning contract | existing `shared/go/commercemission` HMAC client, unchanged | `cmd/section17f_fulfilment_adapters.go` (untouched) |
| 32 unknown outcome recovery | reservation is durable **before** the authority call; recovery reads back and links | `TestIntegration_UnknownOutcomeRecoveryLinksRatherThanRecreates` |
| 33 slot maps to at most one live mission | partial unique index `..._one_live_slot_idx` on `plan_mission_id` | `TestIntegration_ConcurrentMissionReservationYieldsOneRef` |
| 55 provider reassignment | nothing here reacts: same ref, same parcel, only the projection version moves | `TestProviderReassignmentInsideOneMissionChangesNothingHere` |
| 56 mission replacement | `ReplacePlatformMission` marks the old row `REPLACED` and links `replaces_id`; old row keeps its `external_ref` | `TestMissionReplacementKeepsTheOldReferenceReadable`, `TestIntegration_ReturnKeepsTheSameParcel` |
| 62 status projection with source | `status_source_version` + `..._status_chk`; regression refused | `TestIntegration_StaleStatusProjectionIsAbsorbed` |

### Multi-mission and grouping (34–40, 53)

| Req | Where | Evidence |
|---|---|---|
| 34 N missions per group | plan holds `[]PlanMission`; no cardinality limit below 64 | `TestPlanMissions_MultiMissionMixedOwnership` |
| 35 multi-pickup mission | several `PICKUP` stops + one `DROPOFF` | `TestBuildStops_MultiPickupOneDestination` |
| 36 compatibility rules | `grouping.go` `Compatible` — vehicle, service class, capacity, package count, handling, authority, stop limit, **unknown evidence**; applied by `BuildPlanMissions` on replan with the composition root's `authorityCapabilities()` | `grouping_test.go`, `TestAuthorityCapabilities_MatchTheExecutingContract` |
| 37 no unsafe consolidation | motorcycle vs truck refused before anything else; SLA mismatch refused; vendor-owned never merged | `TestCompatible_SharedDestinationIsNotEnough` |
| 38 pickup sequence | persisted `sequence`, `UNIQUE (plan_mission_id, sequence)` | `TestPlanMission_RejectsDuplicateStopSequence` |
| 39 one receiver destination | partial unique index `..._one_dropoff_idx`; `Validate` requires exactly one | `TestPlanMission_RejectsTwoDestinations` |
| 40 no multi-customer batching | not implemented; a mission's stops all belong to one group | `TestPlanMission_RejectsTwoDestinations` |
| 53 stable mission slot | `slot_ref` = the Section 17C plan mission ref; `UNIQUE (plan_id, slot_ref)` | `TestPlanMissions_ConsolidatedPlatformMission` |

### Vendor mission (41–48, 63–65)

| Req | Where | Evidence |
|---|---|---|
| 41 canonical for vendor delivery | `commerce_vendor_missions`, separate table from the projection | package doc |
| 42 no PlatformMissionRef for a VendorMission | `reserveMissions` branches on owner; owner/authority check constraint | `TestPlanMissions_VendorOwnedNeverConsolidated` |
| 43 reuse the existing rider roster | `assigned_staff_id REFERENCES vendor_staff(id)`; `role_vendor_rider` already existed (migration `000005`) | `AssignVendorMission` verifies the staff member's own vendor |
| 44 reuse Section 22 capacity/policy | no capacity or zone rule is redefined here | `internal/features/delivery` untouched |
| 45 HYBRID is not an owner | `Owner` has exactly two values | `TestPlanMissionOwnerAndAuthorityAgree`, `TestDeliveryModeFor` |
| 46 OWN_RIDER_FIRST fallback | `ReasonOwnRiderFirst` plan reason; identities preserved across the version | `TestReplanChangesTheMissionNotTheIdentities` |
| 47 capacity overflow, mixed ownership | one group holds platform and vendor missions together | `TestPlanMissions_MultiMissionMixedOwnership` |
| 48 vehicle overflow | replan to `AuthorityHauling`, same parcel | `TestReplanChangesTheMissionNotTheIdentities` |
| 63 lifecycle vocabulary | **reuses** `delivery.ExecutionState` verbatim | `TestVendorMissionVocabularyMatchesDeliveryExecutionState` (values **and** transition table) |
| 64 explicit transitions | `TransitionVendorMission` legal-successor table; no arbitrary PATCH | `TestTransitionVendorMission_RejectsJumpToDelivered` |
| 65 reuse delivery proof | `handover_ref`/`proof_ref` are references to evidence owned elsewhere | field docs |

### Return, transfer, custody (57–61)

| Req | Where | Evidence |
|---|---|---|
| 57 return keeps the parcel | `commerce_parcel_returns` names the parcel; no replacement identity exists | `TestReturnNamesTheSameParcelAndMayNameANewMission`, `TestIntegration_ReturnKeepsTheSameParcel` |
| 58 return mission linkage | `origin_*` and `return_*` mission columns; exactly one carrier each | `LinkReturnMission` |
| 59 provider transfer | `CustodyTransferred` keeps the parcel in transit under a new custodian | `TestCustodyTransferKeepsParcelInTransit` |
| 60 custody history | `commerce_parcel_custody_events`, append-only, source-attributed | `TestIntegration_CustodySealsTheManifest` |
| 61 current custodian derived | `custodian_type`/`custodian_ref` updated only inside `recordCustodyTx` | store code; no DTO sets it |

### Events, recovery, integrity (66–74, 91–98)

| Req | Where | Evidence |
|---|---|---|
| 66 at-least-once idempotency | `UNIQUE (parcel_id, source, source_event_id)`; duplicate insert is a no-op | `TestIntegration_CustodySealsTheManifest` |
| 67 out-of-order safety | `AcceptStatusProjection` requires strictly newer; `AdvanceParcelStatus` forward-only | `TestAcceptStatusProjection_RefusesStaleAndDuplicate`, `TestIntegration_StaleStatusProjectionIsAbsorbed` |
| 68 existing outbox | `internal/events.DomainPublisher`, published inside the same transaction | `store.publish` |
| 69 existing worker framework | no new scheduler; recovery rides the existing activation/recovery path | `orders` recovery job unchanged |
| 70 mission-create recovery | reservation before call; read-back links | `TestIntegration_UnknownOutcomeRecoveryLinksRatherThanRecreates` |
| 71 activation atomicity | version + slots + stops + parcel assignment + supersession + pointer, one transaction | `ActivatePlan` |
| 72 partial creation recovery | reservation is **per slot**, so already-linked slots are skipped and only the unresolved one is retried | `EnsurePlatformMissionRef` read-back |
| 73 hard relationships | composite scope FKs throughout; no orphan possible | migration |
| 74 no destructive deletes | no DELETE route exists on this surface | `TestNoDestructiveHistoryRoutes` |
| 91–92 refund/failure integration | Section 24 policy untouched; this section supplies the group/unit/parcel/plan/mission relationships it needs | `refund_policy.go` unchanged |
| 93 finance integration | plan stores a cost snapshot only; no ledger state | `Plan.PlannedCostMinor` docs |
| 94–97 schema, JSON, indexes, constraints | relational columns for ownership/identity/version/status; JSONB only for bounded immutable snapshots | `TestFulfilmentAggregateInvariantsAreConstraints` |
| 98 OCC | `If-Match` required on every mutation | `TestEveryMutationHasConcurrencyControl` |

### Security (75–79, 117)

| Req | Where | Evidence |
|---|---|---|
| 75 vendor isolation | tenancy resolved from the row, never from the request; foreign rows answer `NotFound` | `TestIntegration_CrossVendorParcelAccessIsRefused` |
| 76 branch isolation | `authorizeBranch` checks `Scope.Covers(row.BranchID)`; list queries filter in SQL | `service.go` |
| 77 no mission-ref injection | no request DTO carries a mission reference or external ref | `TestNoRouteWritesAPlatformMission` |
| 78 HMAC v2 reused | the existing logistics-event ingress is used; no second protocol | `orders/logistics_event_ingress.go` unchanged |
| 79 caller binding | ingress already allowlists exactly Dispatch and Hauling and consumes a nonce | same |
| 117 security tests | cross-vendor IDOR, branch escape, injection, illegal transition, replay, stale version | listed above |

---

## 4. Files

**Created**

```
migrations/000100_commerce_fulfilment_group_parcel_mission.up.sql
migrations/000100_commerce_fulfilment_group_parcel_mission.down.sql
internal/features/fulfilmentgroup/doc.go
internal/features/fulfilmentgroup/group.go
internal/features/fulfilmentgroup/parcel.go
internal/features/fulfilmentgroup/plan.go
internal/features/fulfilmentgroup/mission.go
internal/features/fulfilmentgroup/grouping.go
internal/features/fulfilmentgroup/store.go
internal/features/fulfilmentgroup/service.go
internal/features/fulfilmentgroup/http.go
internal/features/fulfilmentgroup/grouping_test.go
internal/features/fulfilmentgroup/plan_test.go
internal/features/fulfilmentgroup/mission_test.go
internal/features/fulfilmentgroup/parcel_test.go
internal/features/fulfilmentgroup/identity_test.go
internal/features/fulfilmentgroup/concurrency_integration_test.go
internal/database/fulfilment_group_parcel_mission_shape_test.go
internal/fulfilmentcontract/openapi_test.go
cmd/fulfilment_operations.go
cmd/fulfilment_operations_test.go
docs/openapi/fulfilment-group-parcel-mission.openapi.json
docs/SECTION25_TRACEABILITY.md
```

**Modified**

```
cmd/main.go                                       (composition + routes)
internal/features/orders/fulfilment.go            (FulfilmentProjection port + two call sites)
internal/features/orders/service.go               (optional projection field)
internal/features/orders/vendor_order_operations.go (project logistics events)
docs/API_ROUTE_REGISTER.md
docs/IMPLEMENTATION_STATUS.md
docs/FRONTEND_REQUIREMENTS.md
docs/UI_UX_REQUIREMENTS.md
docs/PLATFORM_SERVICE_CHANGE_REGISTER.md
```

No production file or directory carries a section number. No historical
migration was edited. No other service's code was changed.

---

## 5. Routes

13 added, all under `/api/v1/vendor`:

| Method | Path |
|---|---|
| GET | `/fulfilment/groups/{groupId}` |
| GET | `/fulfilment/groups/{groupId}/plans` |
| GET | `/fulfilment/units/{unitId}/parcels` |
| POST | `/fulfilment/units/{unitId}/parcels` |
| PUT | `/fulfilment/units/{unitId}/readiness` |
| GET | `/fulfilment/parcels/{parcelId}` |
| GET | `/fulfilment/parcels/{parcelId}/custody` |
| PUT | `/fulfilment/parcels/{parcelId}/manifest` |
| POST | `/fulfilment/parcels/{parcelId}/returns` |
| GET | `/fulfilment/vendor-missions` |
| GET | `/fulfilment/vendor-missions/{missionId}` |
| PUT | `/fulfilment/vendor-missions/{missionId}/rider` |
| PUT | `/fulfilment/vendor-missions/{missionId}/status` |

Gated on `COMMERCE_ORDERS_ENABLED`; with it off, no route is registered at all.
Router / OpenAPI / register parity is asserted by
`internal/fulfilmentcontract`.

---

## 6. What was deliberately NOT built

- No second order system, wallet, pricing engine, hybrid planner, refund policy,
  OTP/QR system, event bus, scheduler or provider identity.
- No canonical platform mission moved into Vendor Service.
- No multi-customer route batching.
- No frontend.
- No new feature flag: the aggregate is post-order by definition and rides
  `COMMERCE_ORDERS_ENABLED`.

---

## 7. External limitations, reported not worked around

| Capability | Status | Consequence here |
|---|---|---|
| **Taxi commerce mission execution** | **SUPERSEDED BY SECTION 29.** A distinct, feature-gated Taxi Goods authority now implements CAR quote/mission execution and is present in the shared contracts | Selection remains false by default and requires real Taxi quote availability. See `SECTION29_TRACEABILITY.md`; passenger bookings remain outside Commerce. |
| **Multi-stop pickup at the logistics authority** | **PARTIAL.** `shared/go/commercemission.Request` carries one `Pickup` and one `Dropoff`; Dispatch's `Create` maps to a single-pickup `dispatch_bookings` row, Hauling likewise | Vendor Service models multi-pickup fully (stops, persisted sequence, compatibility rules). `authorityCapabilities()` in the composition root declares `MultiStopPickup: false` for both, read off those contracts. A **replan** (`Service.Replan` -> `BuildPlanMissions` -> `GroupCandidates`) therefore plans compatible units onto **separate missions** rather than one the authority would have to fail. The **initial** translation deliberately does not re-decide grouping: checkout-time consolidation already clusters only groups sharing a pickup point within a tight radius (`internal/features/fulfilment/consolidate.go`), so that cluster is one physical collection a single-pickup contract genuinely executes, and splitting it would break a delivery that was quoted once. What is new is that each allocation is now recorded as its own pickup stop. |
| **Provider transfer evidence** | **PARTIAL.** No logistics service emits a transfer event today; the ingress accepts `PICKED_UP`, `DELIVERING`, `DELIVERED`, `DELIVERY_FAILED` and the timing events | `CustodyTransferred` is modelled and safe to record the moment an authority emits it. Vendor Service does not fabricate one. |
| **Mission replacement notification** | **PARTIAL.** No authority reports that it replaced a mission | `ReplacePlatformMission` exists and preserves both rows; it is currently driven only from within this service. |

Per the specification's own rule, none of these caused a change to Dispatch,
Hauling or Taxi. Each is reported as an external limitation with a safe local
fallback.

---

## 8. Verification

| Gate | Result |
|---|---|
| `go build ./...` (vendor-service) | PASS |
| `go vet ./...` (vendor-service) | PASS |
| `gofmt -l` | clean |
| `internal/features/fulfilmentgroup` unit tests | PASS |
| `internal/database` migration shape tests | PASS |
| `internal/fulfilmentcontract` route/OpenAPI parity | PASS |
| `internal` architecture-boundary test | PASS |
| `internal/features/orders` (regression) | PASS |
| `cmd` composition tests | PASS |
| DB-backed concurrency certification | **BLOCKED_BY_TEST_INFRASTRUCTURE** — `VENDOR_SERVICE_TEST_DATABASE_URL` unset and Docker unavailable. 10 tests in `concurrency_integration_test.go` skip cleanly and run unchanged once a database exists. |
