# Sections 8 & 9 completion map

Status of every Section 8 (Taxi) and Section 9 (Dispatch) item at the close of
this phase. "ALREADY EXISTED" means the baseline commit already implemented it
and this phase only certified it.

Flags referenced here are listed with their shipped defaults in
"Feature flags" at the end.

## Section 8 - Taxi

| Item | Status | Notes |
| --- | --- | --- |
| TAX-001 corporate context client | ALREADY EXISTED | Typed Fleet client + `providercompat.ReadCorporateContext`. |
| TAX-002 provider session enrichment | ALREADY EXISTED | `TokenResult.CorporateContext` / `FinancialVisibilityPolicy`. |
| TAX-003 fail-closed managed enforcement | DONE | `providercompat.AuthorizeManagedOperation` + `fleetprovider.FleetError`. Dependency failure -> retryable 503; invalid membership / ineligible service / wrong provider / wrong organization -> definite 403. No branch downgrades a managed operation to independent. |
| TAX-004 TAXI service eligibility | DONE | `serviceEligibility["TAXI"]` asserted on every managed route and inside `managedexec.Eligibility`. Taxi readiness (operational state, active vehicle, no conflicting work) stays authoritative and is checked BEFORE Fleet, so an outage cannot mask a Taxi-side reason. |
| TAX-005..011 Fleet facade certification | DONE | `managedexec/contract_test.go` pins Taxi's routes against the paths Fleet's execution adapter actually calls; facade DTOs match `providerResponseBody`/`offerResponseBody`; HMAC caller identity, provider binding, source service, correlation id and idempotency all asserted. |
| TAX-012 private execution client | DONE | `internal/features/managedexec`. Implements Fleet's EXISTING `ExecutionContract` (reserve, read-back, release, eligibility, confirm, execution read-back, request/trip reads, cancel) under Taxi's own internal marketplace base. No second protocol. |
| TAX-013 execution reservation | DONE | `taxi_fleet_execution_reservations` + partial unique index on live reservations per booking. Total idempotency: the same key always resolves to the same reservation. |
| TAX-014 corporate execution start | DONE | Corporate work runs on the NORMAL taxi trip engine: corporate identity is metadata on `taxi_bookings`, and execution start calls the same `AcceptBooking` path an independent ride uses (payment hold, guarded transition, operational state). No `corporate_taxi_trips`. |
| TAX-014A assignment/execution consistency | DONE | Partial unique index `uq_taxi_bookings_fleet_assignment`; `BindCorporateExecution` refuses to rebind a booking already carrying a different assignment. |
| TAX-015 authoritative completed work | DONE | Canonical `providercompat.CompletedWorkRequest` to `POST /internal/fleet-partner/v1/completed-work`. No Taxi-specific completion contract. |
| TAX-015A completion source of truth | DONE | `completedWorkPayloadFor` returns nothing for any status other than `completed`; regression-tested across every other status. |
| TAX-016 durable outbox | DONE | Enqueued in the SAME transaction as the completion, on the EXISTING `taxi_outbox` extended with a delivery target. One worker, two destinations. |
| TAX-016A outbox data | DONE | Immutable snapshot carries every field needed to rebuild the request; nothing mutable is re-read at delivery time. No secrets persisted. |
| TAX-016B retry policy | DONE | Bounded exponential backoff. Terminal (source mismatch, wrong provider, invalid membership, invalid payload) dead-letters immediately; retryable backs off; ambiguous is redelivered. |
| TAX-017 reconciliation | DONE | Explicit states PENDING / DELIVERED / RETRY_SCHEDULED / FAILED_TERMINAL / RECONCILIATION_REQUIRED, plus an HMAC operator list + requeue surface. Repair needs no manual SQL. |
| TAX-017A ambiguous response handling | DONE | UNKNOWN is never treated as failure: the row is redelivered under the SAME idempotency key, and an exhausted ambiguous budget becomes RECONCILIATION_REQUIRED rather than a claimed failure. |
| TAX-018 financial DTO audit | DONE | `respond` is the single provider response choke point; every provider booking/trip/earnings response passes through the projection. |
| TAX-019 server-side enforcement | DONE | Concealment happens server-side. Nothing relies on the app hiding a field. |
| TAX-019A typed projection | DONE | `ProviderFinancialProjection` with pointer fields: unauthorized values are ABSENT, not zero. Organization/platform margin have no field at all. |
| TAX-019B independent compatibility | DONE | Independent providers get `IndependentVisibilityPolicy` and lose nothing; the name heuristic runs only for managed providers. Regression-tested. |
| TAX-020 managed payment isolation | DONE | `settleCompletedBooking` routes by the booking's PERSISTED settlement mode, so a Fleet outage cannot reroute corporate work onto the independent path. |
| TAX-020A feature-gated managed settlement | DONE | `TAXI_MANAGED_SETTLEMENT_ENABLED` (off). With isolation on and no Section 12 handoff, corporate work is deliberately NOT paid directly; no faked Wallet success, no double pay, no allocation computed in Taxi. |
| TAX-021..025 Vendor / Taxi Goods | ALREADY EXISTED (certified) | Existing Goods flow unchanged. Vendor still never selects the provider; provider-held pickup credential and custody unchanged. |
| TAX-026 capability truth | DONE | `GET /internal/commerce/capabilities` reports `multiPickupSupported=false`, `maxPickups=1`. No claimed parity with Dispatch. |
| TAX-027 corporate events | ALREADY EXISTED | Existing booking events already carry aggregate id/version, occurredAt and payload. No duplicate domain events added. |
| TAX-028 enable Fleet Taxi source | DEFERRED BY DESIGN | `FLEET_TAXI_INTEGRATION_ENABLED` defaults false; the source stays `UnimplementedExecution` until a deployment enables it after certification. This is the required controlled rollout, not an unfinished item. |

## Section 9 - Dispatch

| Item | Status | Notes |
| --- | --- | --- |
| DSP-001 Fleet integration | ALREADY EXISTED | Marketplace, assignments, provider response, private execution. |
| DSP-002 corporate-context conformance | DONE | One `resolveCorporateContext`: the canonical Section 7 endpoint is authoritative, the legacy membership-summary read is normalized onto the same model as a compatibility fallback. No second interpretation. |
| DSP-003 persisted operating mode | DONE | Fleet's persisted mode is carried through unchanged; tested for CENTRALIZED, DELEGATED and HYBRID. Nothing defaults or hardcodes a mode. |
| DSP-004..007 marketplace / assignments / execution | ALREADY EXISTED (certified) | Unchanged; full suite passes against a live database. |
| DSP-008 canonical completed-work ingestion | DONE | `sourceService=DISPATCH` on the shared `providercompat.CompletedWorkRequest`. No Dispatch-specific contract. |
| DSP-009 reuse existing outbox | DONE | Rides `dispatch_marketplace_outbox` and its worker; the deliverer routes by event type. No second outbox framework. |
| DSP-010 idempotent delivery | DONE | Key derived from trip + authoritative completion version. DUPLICATE counts as success; UNKNOWN stays UNKNOWN so the retry is a redelivery of the same event id. |
| DSP-011..013 typed visibility | DONE | Typed registry is the PRIMARY protection at the single `RespondSuccess` choke point, covering every finance-bearing provider DTO. Name matching demoted to defence-in-depth and applied only to managed riders. |
| DSP-014 managed settlement isolation | DONE | Completion AND the settlement reconciliation worker both honour the boundary. Route read from the trip's persisted `execution_mode`; an unresolvable route resolves to MANAGED. |
| DSP-014A Section 12 handoff | DONE | `ManagedSettlementHandoff` is the seam. No corporate compensation computed in Dispatch. |
| DSP-015..020 Vendor / custody / recovery regression | ALREADY EXISTED (certified) | Full Dispatch suite passes against a live database, including Commerce, multi-pickup, custody, evidence, handover, receiver verification, recovery and reassignment. |
| DSP-021..023 events / idempotency / security | ALREADY EXISTED (certified) | Unchanged, exercised by the passing suite. |
| DSP-024 reconciliation conformance | DONE | Completed-work delivery uses the existing delivery-attempt, dead-letter and ambiguity machinery. |
| DSP-025 preserve existing behaviour | DONE | Both new flags default off; with them off, behaviour is identical to before. |

## Feature flags (shipped defaults)

| Flag | Default | Effect |
| --- | --- | --- |
| `TAXI_FLEET_PROVIDER_WORK_ENABLED` | false | Taxi provider Fleet facade routes. |
| `TAXI_FLEET_COMPLETED_WORK_ENABLED` | false | Completed-work enqueue + delivery. |
| `TAXI_TYPED_FINANCIAL_VISIBILITY_ENABLED` | true | Server-side typed projection. |
| `TAXI_FLEET_MANAGED_EXECUTION_ENABLED` | false | Fleet-facing private execution surface. |
| `TAXI_MANAGED_SETTLEMENT_ENABLED` | false | Section 12 settlement isolation. |
| `DISPATCH_FLEET_COMPLETED_WORK_ENABLED` | false | Completed-work enqueue + delivery. |
| `DISPATCH_MANAGED_SETTLEMENT_ENABLED` | false | Section 12 settlement isolation. |
| `FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED` | true | Unchanged. |
| `FLEET_COMPENSATION_RESOLVER_ENABLED` | false | Sections 12/13. |
| `FLEET_COMPLETED_WORK_PROJECTION_ENABLED` | false | Gates ingestion only (now decoupled from the two below). |
| `FLEET_SETTLEMENT_INSTRUCTION_ENABLED` | false | Sections 12/13. |
| `FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED` | true | Unchanged. |
| `FLEET_TAXI_INTEGRATION_ENABLED` | false | TAX-028 controlled rollout of the TAXI execution source. |

## Section 12 handoff

Every independent-vs-managed settlement branch Section 12 must connect to Wallet:

1. `bookingusecases.settleCompletedBooking` (Taxi completion) - managed branch,
   `ManagedSettlementHandoff.HandleManagedCompletion`.
2. `trip.settleCompletedTrip` (Dispatch completion) - managed branch,
   `ManagedSettlementHandoff.HandleManagedCompletion`.
3. `trip.reconcileOne` (Dispatch settlement reconciliation worker) - managed
   branch, same interface.

Each currently withholds the direct-to-provider payout and relies on Fleet
holding the canonical completed-work record. The downstream model Section 12
completes is:

    execution completion -> Fleet CompletedWork -> Fleet CompensationResolution
      -> ManagedWorkSettlementInstruction -> Wallet
