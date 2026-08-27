# Section 26 — Delivery Requirement and Vehicle Selection Engine traceability

Roadmap section numbers appear in this document only. No production source
file, directory, migration or OpenAPI contract carries one.

Owner: `services/vendor-service`, feature `internal/features/fulfilment`
(the requirement and selection engine) with persistence in
`internal/features/fulfilmentgroup` (the fulfilment plan aggregate).

---

## 1. Audit outcome

The audit ran before any code was written. A substantial engine already
existed — `internal/features/fulfilment/requirements.go` aggregated catalogue
metadata into a `Requirement`, and `vehicle.go` selected a class from it. What
was missing was not the idea but the parts that make a decision *explainable,
durable and safe under incomplete evidence*.

| Capability | Before | Evidence found | After |
|---|---|---|---|
| Estimated weight | AVAILABLE | `Requirement.TotalWeightGrams` + `WeightEvidence` | PRESERVED |
| Confirmed weight | PARTIAL | `Parcel.WeightGrams` written at packing; **no engine consumed it** | IMPLEMENTED_NOW |
| Estimated volume | AVAILABLE | `TotalVolumeMl` | PRESERVED |
| Confirmed volume | NOT_PRESENT | parcels store L/W/H, no volume | IMPLEMENTED_NOW (derived bounding box, marked `SYSTEM_CALCULATED`) |
| Max single-package dimensions | AVAILABLE | `LongestDimensionMm` | PRESERVED |
| Max single-package weight | NOT_PRESENT | — | IMPLEMENTED_NOW |
| Package count | PARTIAL | aggregated but **never a vehicle constraint** | IMPLEMENTED_NOW |
| Stackability | PARTIAL | `StackRestricted` → `FRAGILE_CARRY` only; no volume effect | IMPLEMENTED_NOW (configured derate) |
| Fragile / upright / spill | AVAILABLE | `HandlingCapability` vocabulary | PRESERVED |
| Hot / chilled / frozen | AVAILABLE | `HOT_HOLD`/`COLD_CHAIN`/`FROZEN_CHAIN` | PRESERVED |
| Temperature conflict | NOT_PRESENT | capabilities were unioned; HOT+FROZEN silently selected a van | IMPLEMENTED_NOW |
| Declared value / high value | NOT_PRESENT | — | IMPLEMENTED_NOW |
| Special handling | PARTIAL | `Parcel.SpecialHandlingRefs` — free text, unvalidated, unused | IMPLEMENTED_NOW (controlled vocabulary) |
| Ready window | NOT_PRESENT in engine | data exists in `commerce_vendor_order_readiness` | IMPLEMENTED_NOW |
| Promised receiver time | NOT_PRESENT in engine | `EvaluationRequest.PromisedMaxMinutes` existed and was **never populated** | IMPLEMENTED_NOW |
| Pickup count | PARTIAL | consolidation capped at co-location; no runtime capability flag | IMPLEMENTED_NOW |
| Route distance | PARTIAL | computed for vendor-own only; no vehicle limit | IMPLEMENTED_NOW |
| Serviceability | PARTIAL | flat `map[Service]bool` | EXTENDED (integration / service area / multi-pickup) |
| Motorcycle capability | AVAILABLE | mirrors dispatch's own envelope exactly | PRESERVED (now single-sourced) |
| Car goods capability | NOT_PRESENT | `driver-taxi-service` is **2 Go files**, a pure scaffold | MODELLED, permanently gated |
| Van / truck capability | AVAILABLE | hauling owns both classes | PRESERVED |
| Requirement snapshot | NOT_PRESENT | — | IMPLEMENTED_NOW |
| Config/policy version | NOT_PRESENT | `DefaultCapabilities()` was an unversioned literal | IMPLEMENTED_NOW |
| Reason codes | PARTIAL | one code per decision, no per-candidate evidence | EXTENDED |
| Integer overflow safety | NOT_PRESENT | unchecked `+`/`*` on caller-supplied measurements | IMPLEMENTED_NOW |
| Invalid input rejection | NOT_PRESENT | `Aggregate` accepted negatives | IMPLEMENTED_NOW |

### 1.1 Two live defects the audit found

Both were *unreachable code paths*, not crashes, which is why they had
survived: nothing failed, a rule simply never fired.

1. **`VEHICLE_OVERFLOW` could never trigger.** `delivery.EvaluationRequest`
   has always carried a `RequiredVehicleClass`, and `vendorFailure` has always
   checked it against the vendor's `SupportedVehicleClasses`. The planner
   never set it. A vendor whose fleet was motorcycles only would be handed a
   van-sized load.

2. **`SLA_OVERFLOW` could never trigger.** Same shape:
   `VendorEstimateMinutes` and `PromisedMaxMinutes` were both always zero.

The root cause of both was ordering — the planner evaluated the vendor's own
delivery policy *before* deciding what the load needed, so it had no class to
pass. Fixing it required moving requirement evaluation ahead of the
delivery-mode decision, which is now the shape of `resolveGroup`.

### 1.2 Taxi

`services/driver-taxi-service` contains exactly two Go files (`cmd/main.go`,
`internal/config/config.go`). There is no goods concept, no commerce mission
ingress, no capability contract and no feature flag. Section 26 therefore
**models the car class and refuses to execute it**, per its own instruction not
to implement taxi goods mode. See §5.

---

## 2. Source map

| Requirement | Implementation | Test evidence |
|---|---|---|
| Canonical typed requirement input | `internal/features/fulfilment/delivery_requirement.go` | `vehicle_selector_test.go` |
| Estimated vs confirmed, both retained | `DeliveryRequirement.Estimated` / `.Confirmed`, `Authoritative()` | `TestEngine_ConfirmedPackingEscalatesAndKeepsTheEstimate` |
| Provenance | `MeasurementSource` | `TestSnapshot_ReplaysFromStoredEvidence` |
| Integer/overflow safety | `addChecked`, `mulChecked`, plausibility ceilings | `TestEngine_PackingArithmeticCannotOverflow`, `TestEngine_ImplausibleMeasurementsAreRefused` |
| Invalid input rejection | `Validate()` on every profile | `TestEngine_InvalidInputIsRefused`, `TestEngine_ImpossibleTimingIsRefused` |
| Temperature conflict | `HandlingProfile.TemperatureConflict()` | `TestEngine_HotAndFrozenIsRefusedNotEscalated`, `TestEngine_ChilledAndFrozenIsAConflict`, `TestEngine_HotAndInsulatedIsNotAConflict` |
| Controlled special handling | `SpecialHandling`, `KnownSpecialHandling` | `TestEngine_SpecialHandlingFiltersClasses`, `TestEngine_UnrecognizedHandlingIsRecordedNotIgnored` |
| Versioned capability policy | `internal/features/fulfilment/vehicle_capability.go` | `TestSnapshot_PreservesTheHistoricalPolicy` |
| Serviceability vs physical suitability | `ServiceabilitySnapshot`, `physicalFit` / `executability` | `TestEngine_DisabledIntegrationIsNotAPhysicalRefusal` |
| Deterministic pure evaluator | `internal/features/fulfilment/vehicle_selector.go` `Evaluate` | `TestEngine_IsDeterministic` |
| Controlled reason vocabulary | `internal/features/fulfilment/requirement_reason.go` | `TestSnapshot_OnlyStoresControlledReasonCodes` |
| Immutable snapshot + fingerprint | `internal/features/fulfilment/requirement_snapshot.go` | `TestSnapshot_*` |
| Plan integration | `planner.go` `evaluateRequirement`, `PlanGroup.RequirementSnapshot` | `planner_requirement_test.go` |
| Vendor overflow evidence | `requiredVehicleClass`, `promisedMaxMinutes` | `TestPlanner_VendorVehicleOverflowFallsBackToPlatform`, `TestPlanner_VendorSLAOverflowFallsBackToPlatform` |
| Durable persistence | `migrations/000101_delivery_requirement_vehicle_selection.up.sql`, `internal/features/fulfilmentgroup/delivery_requirement.go` | `internal/database/delivery_requirement_vehicle_shape_test.go`, `fulfilmentgroup/delivery_requirement_test.go` |
| Composition-root mapping | `cmd/fulfilment_requirement_evidence.go` | `cmd/fulfilment_requirement_evidence_test.go` |

No file, directory or migration is named after a roadmap section.

---

## 3. The pipeline

```
Parcel / FulfilmentUnit / catalogue metadata
        ↓  RequirementFrom, ConfirmFromPacking
DeliveryRequirement          (typed, validated, estimated + confirmed)
        ↓  Evaluate(SelectionRequest)
VehicleDecision              (minimum physical class, executable class, reasons)
        ↓  Snapshot
RequirementSnapshot          (immutable, fingerprinted, replayable)
        ↓  requirementEvidenceFor
fulfilmentgroup.PlanInput.RequirementDecisions
        ↓  ActivatePlan  (one transaction)
commerce_fulfilment_plan_versions + commerce_delivery_requirement_snapshots
```

No second fulfilment planner was created. The engine produces a decision; the
existing Section 25 plan aggregate remains the only thing that versions,
activates and supersedes plans.

---

## 4. Selection invariants

Each is enforced structurally, not by convention, and pinned by a named test.

| Invariant | How it is enforced | Test |
|---|---|---|
| Smallest suitable vehicle | policy is rank-ordered by `Normalized()`; the first executable fit wins | `TestEngine_SelectsSmallestSuitableClass` |
| No multi-motorcycle fragmentation | `VehicleDecision` carries **one** class and no count — the answer is unrepresentable | `TestEngine_NeverAnswersWithSeveralMotorcycles` |
| No downgrade on provider shortage | neither `CapabilityPolicy` nor `ServiceabilitySnapshot` has any field for provider supply | `TestEngine_HasNoProviderSupplyInput` |
| Physical requirement ≠ executable service | separate `physicalFit` and `executability` predicates; both recorded per candidate | `TestEngine_DisabledIntegrationIsNotAPhysicalRefusal` |
| No suitable vehicle fails closed | `ReasonNoSuitableVehicle` / `ReasonNoExecutableService`; never a default to the largest truck | `TestEngine_ExceedsEveryClass`, `TestEngine_NothingEnabledFailsClosed` |
| Temperature conflict is never resolved by a bigger vehicle | refused **before** any candidate is evaluated | `TestEngine_HotAndFrozenIsRefusedNotEscalated` |
| Vendor cannot unsafely downsize | `hintFloor` can only raise the rank | `TestEngine_VendorHintCannotDownsizeAnOversizedLoad` |
| Multi-pickup fails closed | `SupportsMulti` false for every class; `ServiceabilitySnapshot.MultiPickup` absent means unsupported | `TestEngine_MultiPickupFailsClosedAgainstTodaysRuntimes` |

---

## 5. The taxi gate

Three independent layers, so no single change can open it:

1. **Capability policy** — `VehicleCar.GoodsModeActive = false`.
   `executability` refuses the class before any other check, so enabling every
   integration flag does not make it reachable
   (`TestEngine_CarGateSurvivesEveryIntegrationBeingEnabled`).
2. **Aggregate validation** — `fulfilmentgroup.RequirementDecision.Validate`
   returns `ErrConflict` for an executed car
   (`TestRequirementDecision_RefusesAnExecutedCar`).
3. **Database** — `selected_vehicle_class` `CHECK` excludes `CAR`, plus a named
   constraint restating why.

`MinimumPhysicalClass` **may** be `CAR`, and the supporting reason
`CAR_PHYSICALLY_SUITABLE_BUT_GOODS_MODE_DISABLED` is recorded when the engine
falls through. Without it, an operator reading a van cannot tell an oversized
load from a disabled integration. `driver-taxi-service` was not modified.

---

## 6. Configuration and explainability

`DefaultCapabilityPolicy()` is the single source of every threshold.
`DefaultCapabilities()` is now a **narrow view** of it rather than a second
table, so the motorcycle's 25 kg limit is stated once.

A snapshot embeds the **whole policy**, not only its version string. A version
alone stops explaining anything the day its thresholds are edited out of the
code; `TestSnapshot_PreservesTheHistoricalPolicy` replays a decision taken
under a 60 kg motorcycle limit that no longer exists.

### Missing-metadata policy

Four configured answers, none of them a guess:

| Policy | Behaviour |
|---|---|
| `PROCEED_ON_LOWER_BOUND` (default) | plan on the stated lower bound, record `MEASUREMENT_EVIDENCE_INCOMPLETE` |
| `CONSERVATIVE_ESCALATION` | refuse the smallest class |
| `REQUIRE_METADATA` | refuse to decide |
| `MANUAL_REVIEW` | refuse to decide, ask for a human |

`PROCEED_ON_LOWER_BOUND` is the default because it is what the platform
already does — aggregation reports `PARTIAL`/`ABSENT` and planning proceeds in
the open. Naming it makes that a stated choice rather than an accident of
where a check was not written. Changing the platform's product behaviour was
not this section's call.

---

## 7. Section 22, 24 and 25 preservation

- **Section 22** — the overflow vocabulary is reused verbatim, never
  duplicated. `VEHICLE_OVERFLOW` and `SLA_OVERFLOW` now actually fire (§1.1);
  `CAPACITY_OVERFLOW` and `DISTANCE_OVERFLOW` are unchanged. No second
  overflow engine was written.
- **Section 23** — no financial engine, ledger or fee exists anywhere in this
  work. `declared_value_minor` is risk metadata; `payment-wallet-service`
  remains the sole money authority.
- **Section 24** — untouched. The engine returns a decision and has no side
  effects: it creates no mission, calls no authority, charges nothing. An
  operational escalation therefore cannot alter a customer-approved price,
  because it has no route to one.
- **Section 25** — `Parcel` identity, `FulfilmentGroup`, `FulfilmentUnit` and
  plan versioning are unchanged. Evidence is written **in the plan's own
  transaction**, so a plan version can never exist without the decision that
  justified it. Replanning appends a version as before; snapshots are
  append-only and carry no version column of their own.

---

## 8. Persistence

Migration tip before: `000100_commerce_fulfilment_group_parcel_mission`.
Migration tip after: `000101_delivery_requirement_vehicle_selection`.

One new table, `commerce_delivery_requirement_snapshots`. No historical
migration was modified, and no existing table gained a column.

It is a table rather than a field inside the existing `routing_evidence` jsonb
because this evidence is read **by people**: Support answers "why was this a
van", operations counts how often the disabled taxi integration forces a larger
vehicle. Both are queries over reason codes and classes. A blob inside a plan
row can be read back; it cannot be counted without scanning every plan ever
made.

---

## 9. Known limitations, stated rather than hidden

1. **Taxi goods execution is DEFERRED.** Modelled, gated three ways, never
   selectable. Not a Section 26 gap: enabling it means building goods mode in
   `driver-taxi-service`, which this section was told not to do.
2. **Multi-pickup runtime is NOT_PRESENT** in both dispatch and hauling. The
   capability is modelled (`SupportsMulti`,
   `ServiceabilitySnapshot.MultiPickup`) so a future runtime is a
   configuration change; today every class answers false and a multi-pickup
   requirement fails closed. The existing 150 m co-location consolidation is
   unaffected — it produces one physical collection point, not a multi-stop
   route.
3. **Activation-time evidence is measurement-only.** `orders.MissionPlan`
   carries weight, package count and the committed class, but not the handling
   metadata behind the checkout decision. The committed class is therefore fed
   to the engine as a **floor**, and evidence that would contradict the plan is
   not filed at all rather than filed wrong
   (`TestRequirementEvidence_NeverDescribesASmallerVehicleThanThePlan`).
   Carrying the full checkout requirement through to activation would remove
   the compromise and is the natural next step.
4. **Truck sub-types stay with hauling.** Vendor Service selects `VAN` or
   `TRUCK`; choosing between a flatbed, a tipper and a refrigerated truck is
   the hauling matcher's job, and duplicating that taxonomy here would create
   two vocabularies that drift. `commercequote.VehicleClass` — the actual wire
   contract — has exactly `MOTORCYCLE`, `VAN`, `TRUCK`.
5. **PostgreSQL certification was not run.** See the certification report:
   no test database is configured in this environment.
