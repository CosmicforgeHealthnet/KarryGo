# Section 27 — Multi-Vendor Consolidation and Mission Planning

Requirement → authority → implementation → schema → surface → test → readiness.

Everything below is traceable to code in this repository. Where a capability
is genuinely absent — runtime multi-pickup, an ordinal quality signal — the row
says so rather than describing what would exist if it were not.

---

## 1. Audit baseline: what already existed

Section 27 is not greenfield. The audit found substantially more in place than
a naive reading of the requirement would suggest, and building past it would
have produced a second, competing planning stack.

| Capability | Baseline | Where |
|---|---|---|
| Consolidation clustering | AVAILABLE | `fulfilment/consolidate.go` — greedy merge on co-location, urgency, separation, combined capacity |
| Real logistics quotes | AVAILABLE | `shared/go/commercequote` via `Planner.platformMission` |
| Vehicle selection + reason vocabulary | AVAILABLE | Section 26 `vehicle_selector.go`, `requirement_reason.go` |
| Multi-pickup capability model | AVAILABLE | `ServiceabilitySnapshot.MultiPickup`, `ReasonMultiPickupUnsupported` |
| `Mode` enum (FASTEST/BALANCED/SAVER) | PARTIAL | Declared in `plan.go` and in `commerce_fulfilment_plans.mode`; only `SINGLE` was ever written |
| `selected_fulfilment_mode` on the quote | AVAILABLE | `checkoutquote` reads `p.mode` — no plumbing needed once the plan carries a real mode |
| Candidate alternatives + rejection reasons | NOT_PRESENT | — |
| Ready-time overlap, detour measurement | NOT_PRESENT | — |
| Quote-only planning | NOT_PRESENT | — |
| Cart versioning | AVAILABLE | `commerce_carts.version` + `commerce_cart_history` |
| Exactly-once recurrence pattern | AVAILABLE (elsewhere) | `commerce_recurring_occurrences` UNIQUE `(schedule_id, occurrence_key)` — the corporate-feeding precedent this section mirrors |
| Substitution policy evaluator | AVAILABLE, NOT REUSABLE HERE | `orders/substitution_policy.go` requires `SameBusiness`/`SameBranch`; it governs what a PICKER may swap after an order exists, not what an optimizer may propose before one does |
| Smart Basket / budget / recurring basket | NOT_PRESENT | — |

---

## 2. Fulfilment preference

| Requirement | Implementation | Test |
|---|---|---|
| Controlled FASTEST / BALANCED / SAVER | `fulfilment/fulfilment_preference.go` `ParseMode`, `Mode.Selectable` | `TestParseMode` |
| Default BALANCED | `DefaultMode`; empty input resolves to it, unknown input is refused | `TestParseMode`, `TestPlan_DefaultModeIsBalanced` |
| Mode in CheckoutQuote | `Plan.Mode` → `commerce_fulfilment_plans.mode` → `checkoutquote` `selected_fulfilment_mode` → quote fingerprint | `TestPlan_ModeFlowsIntoThePlanThatIsStored` |
| Mode in FulfilmentPlan version | `Plan.Mode`, `Plan.OptimizationPolicyVersion` | `TestPlan_DefaultModeIsBalanced` |
| Versioned, explainable thresholds | `OptimizationPolicy` + `DefaultOptimizationPolicyVersion`; stored per candidate | `TestPlan_BothAlternativesArePersistedWithReasons` |

The mode is the one value a customer may contribute to planning. It is safe
structurally rather than by validation: it selects among alternatives the
server built and the authorities priced, so it can no more set a fee than it
can set a distance.

---

## 3. Candidate construction and comparison

| Requirement | Implementation | Test |
|---|---|---|
| Destination grouping | Every candidate is rooted on one checkout session and one destination; no cross-customer batching exists | — (structural: `PlanRequest` carries one `Destination`) |
| Delivery-window compatibility | `readyWindowCheck` — gap tolerance plus each member's own `ReadyUntil` handover deadline | `TestPlan_ReadyWindowToleranceIsModeSpecific`, `TestPlan_ClosedHandoverWindowForcesASplitInEveryMode` |
| Ready-time overlap, mode-specific | `ModePolicy.ReadyGapTolerance` — 2 / 10 / 30 minutes | `TestPlan_ReadyWindowToleranceIsModeSpecific` |
| Handling compatibility | `temperatureConflict`, `fragileUnsafe`, `conflictingSeparation` | temperature / fragile / urgency tests |
| Route proximity | `OptimizationPolicy.ProximityRadiusMeters` bounds construction | `TestPlan_DistantPickupsAreNotEvenEvaluatedTogether` |
| Aggregate load → Section 26 | `cluster.combined()` → `Capability.Fits` via `Planner.classFits` | `TestPlan_CapacityExceededForcesASplit` |
| Route/service quote per candidate | `quoteCache` + `Planner.quoteCluster`; both alternatives priced before either is compared | `TestPlan_CoLocatedVendorsConsolidateIntoOneMission` (asserts 1 combined + 2 split quotes) |
| Consolidated candidate | `consolidatedClustering` | co-located tests |
| Split/parallel candidate | `splitClustering` — always built, always the baseline and the fallback | every mandatory-split test |
| Alternative persistence | `commerce_fulfilment_plan_candidates`, written in the plan's own transaction | `TestPlanCandidateMigrationShape` |
| Selection / rejection reason | Closed vocabulary, `KnownCandidateReason` | `TestPlan_BothAlternativesArePersistedWithReasons` |
| Deterministic comparison, no AI | `plan_comparison.go` — lexicographic per mode, tie-break on semantic fingerprint | `TestPlan_IdenticalInputsProduceIdenticalCandidates` |
| Dominance and materiality | `balancedBetter`, `losingReason` | BALANCED tests |
| Integer money, explicit time units | `int64` minor units, `time.Duration` tolerances, `int` minutes | throughout |

### Mandatory splits

Each is asserted to produce `INELIGIBLE` under the mode most motivated to
overrule it — usually SAVER, because SAVER is the mode that most wants the
cheaper consolidation.

| Condition | Reason code | Test |
|---|---|---|
| Capacity exceeded | `CAPACITY_EXCEEDED` | `TestPlan_CapacityExceededForcesASplit` |
| Temperature conflict | `TEMPERATURE_CONFLICT` | `TestPlan_TemperatureConflictForcesASplit` |
| Urgency conflict (hot food vs standard) | `URGENCY_CONFLICT` | `TestPlan_HotFoodIsNotMadeToTravelWithAStandardLoad` |
| Fragile unsafe under co-load | `FRAGILE_UNSAFE` | `TestPlan_FragileUnderHeavyLoadForcesASplit` |
| Fragile with light co-load still consolidates | — | `TestPlan_FragileWithLightCoLoadStillConsolidates` |
| Ready-time conflict | `READY_WINDOW_CONFLICT` | ready-window tests |
| ETA / detour exceeded | `ETA_DETOUR_EXCEEDED` | `TestPlan_ConsolidationBreakingAPromisedETAIsRefused`, `TestPlan_BalancedSplitsWhenConsolidationCostsRealTime` |
| Multi-stop unsupported | `MULTI_STOP_UNSUPPORTED` | `TestPlan_SeparatePickupsAreRefusedWhileMultiStopIsUnsupported` |
| Customer accepted FASTEST parallel | `CUSTOMER_FASTEST_PARALLEL_SELECTED` | `TestPlan_FastestChoosesParallelEvenWhenConsolidationIsCheaper` |

**Hard constraints cannot be outranked by price, structurally.** Two CHECK
constraints on `commerce_fulfilment_plan_candidates` make a constraint reason
storable only on an `INELIGIBLE` row, and an `INELIGIBLE` row storable only
with a constraint reason. The comparison function never sees an infeasible
candidate at all.

### One scenario, three modes

`modeScenarioRoutes()` fixes one set of physical facts — parallel finishes in
24 minutes for ₦2,900; consolidated finishes in 26 for ₦1,700 — and only the
customer's stated preference differs:

- FASTEST → split (`TestPlan_FastestChoosesParallelEvenWhenConsolidationIsCheaper`)
- SAVER → consolidated (`TestPlan_SaverChoosesConsolidationOnTheSameFacts`)
- BALANCED → consolidated, immaterial time cost (`TestPlan_BalancedConsolidatesWhenTheTimeCostIsImmaterial`)

---

## 4. Multi-stop: the live limitation

| Question | Answer |
|---|---|
| Dispatch runtime multi-pickup | **NOT_PRESENT** |
| Hauling runtime multi-pickup | **NOT_PRESENT** |
| Taxi | **DEFERRED / NOT_TOUCHED** — no goods capability, no `CAR` in the shared quote contract |
| Vendor planning model | Models it correctly and fails closed |
| Executable fallback | Split alternative, always built |
| Future readiness | Capability read from configuration; a test flips it and the same candidate becomes selectable |

The distinction the gate turns on is `pickup_stop_count`, computed by
clustering member branch coordinates within the co-location radius:

- Several vendors at ONE physical point — a food court, a market — are **one
  stop** and consolidate normally today
  (`TestPlan_CoLocatedPickupsAreOneStopAndStillConsolidate`).
- Vendors at separate points are **several stops** and are refused
  (`TestPlan_SeparatePickupsAreRefusedWhileMultiStopIsUnsupported`).
- Flipping the capability makes the refused candidate selectable with no other
  change (`TestPlan_ConsolidationOpensWhenTheRuntimeGainsMultiPickup`).

No quote is requested for an unexecutable multi-stop route, asserted by the
first of those tests.

---

## 5. Quote-only planning

| Requirement | Implementation | Test |
|---|---|---|
| Quote-only interface | `fulfilment/quote_planner.go` `QuotePlan`; `PlanningService.QuotePlan` skips the store | — |
| Creates no mission | Missions are not projected into `QuoteOnlyResult` at all | `TestOptimize_ManyBasketsCreateNoMissionsAndNoMoney` |
| Reserves no provider | No reservation dependency is wired | structural |
| Mutates no Wallet | No Wallet dependency is wired | structural + the test above |
| Mutates no cart | Cart mutation is a separate, explicit route | `TestOptimizeCart_NeverMutatesTheCart` |
| Deterministic under identical inputs | Same plan pipeline, same fingerprints | `TestOptimize_IsDeterministic` |

---

## 6. Smart Basket / Optimize My Cart

| Requirement | Implementation | Test |
|---|---|---|
| Proposal, never a mutation | `basket/optimize_cart.go` | `TestOptimizeCart_NeverMutatesTheCart` |
| Merchandise + delivery + vendor count + availability considered | `basket_optimizer.go`, `quoteBasket` | `TestOptimize_FewerVendorsWinWhenDeliveryDominates` |
| Customer constraints respected | `equivalence.go` `satisfies` | brand / quality / exact-only tests |
| Equivalent-product proposal | `EquivalenceBasis`, `allowanceAdmits` | `TestOptimizeCart_NeverMutatesTheCart` (basis asserted) |
| No false equivalence | Trade identifier, catalogue product or taxonomy node only — no name matching anywhere | `TestOptimize_NoAuthoritativeEquivalenceMeansNoSubstitution` |
| Acceptance → new cart version | `AcceptProposal` → `CartAuthority.ApplyProposal` under expected version | DB-gated |
| Acceptance idempotent | `ProposalAccepted` replay + `WHERE state='OPEN'` | DB-gated `TestProposal_ConcurrentAcceptanceProducesOneCartVersion` |
| Stale proposal protection | Cart id + version compared; `commerce_basket_proposals_live_idx` | DB-gated |
| Acceptance requotes | `AcceptanceResult.RequoteRequired` always true | contract + OpenAPI |
| Proposal expiry | `DefaultProposalTTL`, read-time check, durable sweep | `basketProposalExpiryJob` |
| Never proposes the current cart | `currentCartFingerprint` filter | `TestOptimizeCart_DoesNotProposeTheCartYouAlreadyHave` |

---

## 7. Budget optimizer

| Requirement | Implementation | Test |
|---|---|---|
| Typed input: budget, groups, quantities, brand, substitution, quality, preferences | `basket_requirements.go` | `TestOptimize_*` |
| Budget is the DELIVERED total | `FinalDeliveredTotalMinor` is the only figure compared | `TestOptimize_BudgetIsEvaluatedOnTheDeliveredTotal` |
| Real active listings only | `candidateSQL` — status, retail, logistics, branch, product, sell unit, price and stock all filtered | structural |
| Authoritative sellable stock | `on_hand - reserved - quarantined > 0` | structural |
| Authoritative price | Listing-unit override → sell unit → listing override → variant base; no invented discount | structural |
| Quantity satisfied exactly | `satisfies` refuses on insufficient stock | `TestOptimize_QuantityIsSatisfiedExactlyOrNotAtAll` |
| Hard brand constraint | Never substituted at any price | `TestOptimize_RequiredBrandIsNeverSubstituted` |
| Hard quality constraint | `offersGrade` against catalogue-stated grades | `TestOptimize_QualityConstraintIsHonouredWhereDataExists` |
| VALUE / CHEAPEST_DELIVERED | `ObjectiveValue` | `TestOptimize_BudgetFitReportsTheWholeBasket` |
| LOWEST_DELIVERY / FEWEST_VENDORS | `ObjectiveFewestVendors`, `basketAnchoredOn` | `TestOptimize_FewerVendorsWinWhenDeliveryDominates` |
| QUALITY_BALANCED | Implemented and gated on real data | `TestOptimize_QualityVariantIsNotFabricatedWithoutData` |
| No-budget-solution answer | Minimum feasible, shortfall, binding constraints | `TestOptimize_BudgetIsEvaluatedOnTheDeliveredTotal` |
| No complete basket | Separate state, no minimum quoted | `TestOptimize_NoCompleteBasketIsNotABudgetProblem` |
| Unmet requirement names its constraint | `blockingFailures`, ordered most-informative-first | `TestOptimize_UnmetRequirementNamesTheBindingConstraint` |
| Bounded deterministic search | `MaxEvaluatedBaskets`, `MaxCandidateVendors`, `MaxCandidateListingsPerNeed` | `TestOptimize_SearchIsBounded` |
| Truncation disclosed | `Result.SearchTruncated` | `TestOptimize_SearchIsBounded` |
| Honest optimality claim | `BEST_VALID_CANDIDATE_UNDER_CONFIGURED_SEARCH` | `TestOptimize_BudgetFitReportsTheWholeBasket` |
| Delivery per candidate, never a flat per-vendor fee | `quoteBasket` through the quote-only planner | `TestOptimize_FewerVendorsWinWhenDeliveryDominates` |
| Undeliverable basket never wins | Unserviceable quote excludes the basket | `TestOptimize_UndeliverableBasketNeverWinsOnPrice` |
| No N+1 | Candidates batched per requirement; logistics profiles batched per business | `attachLogistics` |
| Search complexity | Linear in candidate vendors, not exponential in requirements — see `basket_optimizer.go` header | `TestOptimize_SearchIsBounded` |

### QUALITY_BALANCED: NOT_AVAILABLE_DUE_TO_DATA

`shop_fresh_attributes.attribute_type = 'QUALITY_GRADE'` is a fresh-goods
SELECTION vocabulary — which grades a listing offers — and the platform holds
no ordinal quality score for anything.

So the two halves are treated differently and deliberately:

- A customer's **hard minimum-grade constraint** is checkable against that data
  and is enforced.
- **Ranking** one listing above another by quality is not, and no score was
  invented to make the variant appear.

The variant is generated only where every requirement has a candidate carrying
the signal; otherwise the response names the requirement that lacked it.

---

## 8. Recurring optimized basket

| Requirement | Implementation | Test |
|---|---|---|
| Template stores requirements, not SKU bindings | `Template.Requirements`; a category requirement cannot be `EXACT_ONLY` | `TestTemplate_CategoryRequirementIsNotBoundToASKU` |
| Every occurrence re-evaluates listings, stock, price, vendors, delivery | `GenerateOccurrence` → the shared `optimize` pipeline | — |
| SAME_VENDOR_PLAN | `sameVendorPlan` — repriced against today, not last week's receipt | `TestCompareRecurringPlans_ReportsBothSidesAndTheDeltas` |
| OPTIMIZED_PLAN | First ranked variant | as above |
| Merchandise / delivery / total / vendor / delivery deltas | `PlanComparison` | as above |
| Baseline unavailable is stated | `SameVendorReason` | `TestCompareRecurringPlans_SaysWhenTheOldVendorsCannotSupply` |
| Material-change detection | `materialChanges` — closed vocabulary | `TestMaterialChanges_NameEveryDifferenceThatMatters` |
| Operational differences are not material | Provider, route and mission grouping are excluded | `TestMaterialChanges_IgnoreOperationalDifferences` |
| Approval required for material change | `approvalStateFor` | `TestApprovalState_MaterialChangesNeedTheCustomer` |
| Bounded preauthorization only | Ceiling mandatory, enforced in service AND schema | `TestTemplate_AutoApprovalRequiresACeiling`, DB-gated `TestSchema_RefusesIncoherentRows` |
| Ceiling is a hard bound | Re-checked at the last point before the customer stops being asked | `TestApprovalState_CeilingIsAHardBound` |
| Disabled template generates nothing | `ErrTemplateDisabled`; sweep reads enabled only | structural |
| Explicit timezone | Required; empty refused explicitly because `LoadLocation("")` succeeds as UTC | `TestTemplate_RequiresATimezone` |
| Missed occurrence policy | SKIP, bounded by `MissedOccurrenceWindow` | `dueOccurrence` |
| Generation ≠ order | Five distinct stages; this capability reaches two | structural + OpenAPI |
| Template optimistic concurrency | `version` in the UPDATE predicate | DB-gated |

### Exactly-once

| Requirement | Implementation | Test |
|---|---|---|
| Stable occurrence identity | `Template.OccurrenceKey` — local wall-clock + IANA zone, nothing volatile | `TestOccurrenceKey_DependsOnlyOnTheSchedule` |
| Database uniqueness | `UNIQUE (template_id, occurrence_key)` | `TestBasketOptimizationMigrationShape` |
| Duplicate scheduler delivery | `ClaimOccurrence` `ON CONFLICT DO NOTHING`, claim-before-work | DB-gated `TestOccurrence_ConcurrentWorkersProduceOneGeneration` |
| Crash / restart recovery | Retry reads back its own claim | DB-gated `TestOccurrence_RetryAfterCrashResumesTheSameGeneration` |
| Not an in-memory lock | No lock exists anywhere in the path | structural |
| Timezone-correct scheduling | `NextOccurrence` computes in the template's zone, converts at the end | `TestNextOccurrence_IsComputedInTheCustomersTimezone` |
| Monthly never skips a month | `day_of_month` capped at 28, in service and schema | `TestNextOccurrence_MonthlyNeverSkipsAMonth` |

---

## 9. Price and Section 24

Nothing here writes a financial policy. Post-payment behaviour is unchanged:

| Requirement | Owner | Note |
|---|---|---|
| Internal replan cannot raise an approved charge | Section 24 | `orders/commercial_change.go` — untouched |
| Cheaper replan refunds | Section 24 | `ApplyCheaperFulfilmentBenefit` — untouched |
| Customer-requested change reapproves | Section 24 | Fingerprint-verified approval path — untouched |
| No new ledger | — | The optimizer is composed with no Wallet dependency |

---

## 10. Security

| Requirement | Implementation | Test |
|---|---|---|
| Customer from the token only | `customerSubject` in both HTTP surfaces | `TestRoutes_RefuseUnauthenticatedCallers` |
| No forgeable value in any request body | Asserted over DTO field tags | `TestRequestContracts_CarryNoAuthoritativeValues` |
| Destination by reference, never coordinates | `resolveCustomerAddress` | `TestOptimizeRequest_TakesAnAddressReferenceNotCoordinates` |
| Proposal isolation | Customer-scoped reads and writes | DB-gated `TestProposal_IsScopedToItsOwner` |
| Template isolation | As above | DB-gated `TestTemplate_IsScopedToItsOwner` |
| Client cannot choose the winner | No such field exists | `TestRequestContracts_CarryNoAuthoritativeValues` |
| Private vendor data protected | Only customer-visible listing data reaches a proposal | structural |
| Bounded request | `MaxRequirements`, `MaxRequirementQuantity`, search bounds | `TestOptimize_SearchIsBounded` |
| Disabled feature has no surface | `RegisterCustomerRoutes` returns early | `TestRoutes_DisabledFeatureRegistersNothing` |

---

## 11. Schema and surface

| Item | Value |
|---|---|
| Migration tip before | `000101_delivery_requirement_vehicle_selection` |
| Migration tip after | `000103_commerce_basket_optimization` |
| New migrations | `000102_commerce_fulfilment_plan_candidates`, `000103_commerce_basket_optimization` (both paired up/down) |
| Route count before | 829 |
| Route count after | 840 |
| New routes | 11, all customer-facing under `/api/v1/commerce/basket` |
| Changed contract | `POST /checkout/{sessionId}/fulfilment-plan` gained an optional preference body and a `candidates` response field |
| OpenAPI | `basket-optimization.openapi.json`, `commerce-fulfilment-optimization.openapi.json` |
| Route/spec/register parity | `TestRoutes_MatchTheSpecificationExactly`, `TestEveryProductionRouteIsDocumented`, `TestNoOpenAPIOperationIsPhantom`, `TestProductionRouteCountIsTheAcceptedSurface` |

No production file or directory carries a section number. Existing
`cmd/section17*.go` files predate this work and were not renamed.

---

## 12. Readiness

| Gate | Status |
|---|---|
| Vendor build / vet / gofmt | PASS |
| Vendor test suite (63 packages) | PASS — 944 passed, 818 skipped (DB-gated), 0 failed |
| Route, OpenAPI and register parity | PASS |
| Migration shape and pairing | PASS |
| Migration apply / rollback / reapply | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Concurrent occurrence claim | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Crash-and-retry occurrence resumption | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Concurrent proposal acceptance | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Ownership isolation | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Template optimistic concurrency | BLOCKED_BY_TEST_INFRASTRUCTURE |
| Database constraint enforcement | BLOCKED_BY_TEST_INFRASTRUCTURE |

`VENDOR_SERVICE_TEST_DATABASE_URL` is unset and Docker is unavailable in this
environment. Every blocked gate is written and skips cleanly; none is absent,
and none is reported as a pass.

To run them:

```bash
VENDOR_SERVICE_TEST_DATABASE_URL='postgres://...' \
  go test ./internal/features/basket/ ./internal/database/ -v
```
