# KARRYAM Sections 15–18 — the matrices

Companion to `KARRYAM_SECTION15_16_17_18_CLOSURE.md`. Every value here was
read out of the running code or a live service, not copied from a design
document.

---

## 1. Credential matrix

The platform runs **three** pickup directions, not the two Sections 15.2's
CFC-005/CFC-006 pair names. The third is real and was found by audit: Dispatch
and Hauling direct (non-Commerce) parcel work protects the CUSTOMER from a
driver who is not the assigned one, so the provider presents and the customer
verifies — the Commerce roles with a consumer in the vendor's place, not the
Taxi direct contract.

| Flow | Holder | Verifier | Purpose constant | Enforced by |
|---|---|---|---|---|
| Commerce pickup (Dispatch) | Provider | Vendor / sender | `PurposeCommercePickup` | `commercemission.MultiStopMissionService.ArrivePickup` issues to the provider; `VerifyPickup` refuses any caller that is not `dispatchcommerce.SignatureCaller` |
| Commerce pickup (Taxi Goods) | Provider | Vendor / sender | `PurposeCommercePickup` | `goods.Service.IssuePickupCredential` / `goods.Service.VerifyPickup(caller, …)` |
| Commerce pickup (Hauling) | Provider | Vendor / sender | `PurposeCommercePickup` | `bookingusecases.GetPickupCredential` / `ConfirmCommercePickup`, mounted at `POST <api>/internal/commerce/missions/:missionRef/handover/PICKUP/confirm` behind HMAC + Vendor allowlist |
| Direct Taxi pickup | Customer / passenger / sender | Taxi provider | `PurposeDirectPickup` | `bookingusecases.VerifyPickup(bookingID, providerID, code)` — the provider submits the customer's code |
| Direct parcel pickup (Dispatch, Hauling) | Provider | Customer / sender | `PurposeSenderVerifiedPickup` | Dispatch `trip.VerifyPickupCodeForCustomer`; Hauling `VerifyDriverAtPickup(customerID, …)` |
| Delivery (all flows) | Receiver / customer | Provider / execution service | `PurposeDelivery` | Dispatch `verifyDeliveryCode`; Taxi Goods `VerifyDelivery(providerID, …)`; Hauling `verifyDeliveryCode` |
| Return | Provider | Vendor / sender | `PurposeReturn` | `custodyops` return workflow |
| Provider transfer | Provider (outgoing) | Provider (incoming) | `PurposeProviderTransfer` | `custodyops.TransferCommand`, both confirmations required |

The matrix itself lives in `shared/go/custodychain/credential.go` and is
unexported behind `DescriptorFor`, so no caller can add a seventh direction at
runtime. `TestNoPurposeLetsOnePartyApproveItself`,
`TestCommerceAndDirectPickupPointOppositeWays`,
`TestThreePickupDirectionsStayDistinct` and `TestVerifierIsExclusivePerPurpose`
fail if any two directions collapse or if a new purpose is added without
deciding who may not verify it.

## 2. Capability matrix (actual runtime values)

Read live from the booted services — see
`KARRYAM_SECTION15_18_LIVE_EVIDENCE.txt`.

| | Dispatch | Taxi Goods | Hauling |
|---|---|---|---|
| authority | `DISPATCH` | `TAXI` | `HAULING` |
| capability version | 1 | 1 | **2** (was 1) |
| multiPickupSupported | true | false | true |
| maxPickups | 12 | 1 | 10 |
| **maxSecurePickups** | **12** | **1** | **1** |
| multiStopSupported | false | false | false |
| maxDropoffs | 1 | 1 | 1 |
| securePickupSupported | true | true | true |
| secureDeliverySupported | true | true | true |
| custodyEvidenceSupported | true | true | true |
| vehicleClasses | MOTORCYCLE | CAR | VAN, TRUCK |

`maxSecurePickups` is new. It is separate from `maxPickups` because routing
across several pickups and issuing a credential per pickup are different
capabilities, and Hauling genuinely has the first without the second: its
handover challenge is keyed `(booking_id, stage)` with one ACTIVE row per
stage. An undeclared bound reads as `maxPickups`, and an undeclared
`maxPickups` reads as 1 — never as unlimited.

## 3. Custody matrix

| Transition | Operation | Allowed | Refusal when wrong |
|---|---|---|---|
| NONE → SENDER | `PREPARATION` | yes | — |
| SENDER → SENDER | `PREPARATION` | yes | — |
| SENDER → PROVIDER | `PICKUP` | yes, after a verified credential | — |
| SENDER → RECEIVER | `PICKUP` / `DELIVERY` | only vendor-owned delivery (`SenderDelivers`) | `CUSTODY_SKIPS_PROVIDER` |
| PROVIDER → RECEIVER | `DELIVERY` | yes, after a verified credential | — |
| PROVIDER → PROVIDER | `PROVIDER_TRANSFER` | yes | `CUSTODY_TRANSFER_NOT_SUPPORTED` / `CUSTODY_OPERATION_MISMATCH` under any other operation |
| PROVIDER → PROVIDER | `FAILED_ATTEMPT` | yes, custody unchanged | — |
| PROVIDER → SENDER | `RETURN_COMPLETED` | yes | — |
| RECEIVER → anything | any | no | `CUSTODY_REVERSES_TERMINAL` |
| any where `from` ≠ recorded holder | any | no | `CUSTODY_CURRENT_HOLDER_MISMATCH` |
| unknown party or operation | any | no | `CUSTODY_PARTY_UNKNOWN` |

Reassignment before pickup leaves custody with the sender and revokes the old
provider's credential (`PROVIDER_REASSIGNED`, certified against the real
migration chain). Reassignment after pickup requires the explicit transfer
workflow. A stale or duplicate report that would not move the parcel forward
stays a silent no-op, because an at-least-once pipeline redelivers and
treating that as an error would make a working system look broken.

## 4. Authorization matrix

| Actor | May | May not |
|---|---|---|
| Provider (bearer token) | Own offers, assignments, execution, own financial view, permitted safety actions | Any internal route (401 — certified live on Hauling), another provider's work, organization admin actions |
| Organization dispatcher | Company assignment operations, organization execution oversight | Compensation plan changes, payroll administration, unrestricted Wallet data — each needs its own grant |
| Organization admin | Explicitly permissioned admin operations | Anything not granted; admin rights are never inferred from membership |
| Internal service (HMAC v2) | Exactly the operations its caller name is allowlisted for | Another service's surface — Fleet cannot read Vendor's commerce capability (403, live); Vendor cannot read Dispatch's outbox (403, live); no universal bypass exists |
| Platform admin / operator console | Inspect and repair event delivery, reconciliation cases, payroll posting | Mutate execution-domain tables directly; repairs go through the owning service |

Financial visibility stays a server projection under
`providercompat.FinancialVisibilityPolicy`. Organization-only and
platform-only classes are unreachable under every policy, not merely off by
default.

## 5. Durable event inventory

| Event | Producer | Consumer | Outbox | Idempotency key | Source version | Reconciliation owner |
|---|---|---|---|---|---|---|
| corporate assignment created / updated | Fleet | execution services | `fleet_partner_outbox` | assignment id + version | assignment version | Fleet |
| provider accepted / declined | execution service | Fleet | service outbox | client response id | response version | Fleet |
| execution reserved / started | execution service | Fleet | service outbox | reservation / execution id | request version | Fleet |
| pickup completed / custody transferred | execution service | Vendor | `dispatch_marketplace_outbox`, `hauling_domain_outbox` | `(parcel_id, source, source_event_id)` at the consumer | mission / trip version | Vendor |
| execution completed | execution service | Fleet, Vendor, Wallet | service outbox | `<source>-completed-work:<job>:<version>` | source version | execution service |
| Commerce fulfilment changed | Vendor | Vendor projections | `vendor_outbox` | event id | aggregate version | Vendor |
| completed work recorded | Fleet | Fleet compensation | `fleet_partner_outbox` | ingestion idempotency key | source version | Fleet |
| settlement instruction created | Fleet | Wallet | `fleet_partner_outbox` | `(instruction_id, instruction_version)` | instruction version | Fleet |
| wallet posting completed / failed / reversed | Wallet | Fleet, execution services | wallet events | instruction id + version | instruction version | Wallet |
| payroll approved / posted | Fleet | Wallet | `fleet_partner_outbox` | payroll run id | run version | Fleet |
| membership changed | Fleet | execution services (read on demand) | `fleet_partner_outbox` | membership id + version | membership version | Fleet |

No service caches membership, operating mode, compensation policy or
visibility policy, so there is no cache for a policy-invalidation event to
invalidate (EVT-021).

## 6. Reconciliation inventory

| Service | Reconciles | Operator surface |
|---|---|---|
| Taxi | Fleet completed-work delivery | `GET/POST <api>/internal/fleet/completed-work[/:id/requeue]` |
| Dispatch | Fleet event delivery (all targets) | `GET/POST /api/v1/internal/dispatch/v1/operations/outbox[/:id/requeue]` — **new** |
| Hauling | Fleet completed-work delivery | `GET/POST <api>/internal/fleet/completed-work[/:id/requeue]` |
| Fleet | Execution start, completed-work projection, settlement posting, expired reservations, stuck assignments, payroll posting | `/finance/operations/:id/replay`, `/finance/reconciliation-cases/...`, `/operations/outbox/replay`, `/operations/backlogs` |
| Vendor | Authority mission existence and status, pickup, custody, delivery, fulfilment completion, payment release | `/projection-jobs/:jobId/replay` and the durable job surface |
| Wallet | Provider, organization, merchant and managed postings, payroll posting, pending adapter operations | `/operations/:operationType/:id/replay`, `/reconciliation` |

## 7. Certification matrix

Evidence classes are reported exactly as run.

| Scenario | Result | Evidence |
|---|---|---|
| CUST-01 Dispatch commerce pickup | PASS — LIVE (surface) / PASS — DB-BACKED (flow) | Live: Vendor-only access to the capability and handover surfaces. DB: `commercemission` multi-stop pickup suite. |
| CUST-02 Taxi Goods commerce pickup | PASS — COMPONENT | `goods` credential direction and verify-caller tests. |
| CUST-03 Hauling commerce pickup | PASS — LIVE | Vendor reaches the handover surface past the HMAC boundary; no other caller does. |
| CUST-04 direct Taxi pickup keeps its own direction | PASS — COMPONENT | `TestTaxiGoodsDoesNotResolveDirectTaxiPickup`; `TestThreePickupDirectionsStayDistinct`. |
| CUST-05 Dispatch multi-pickup | PASS — DB-BACKED | `multi_stop` suite; stop-scoped challenges and evidence. |
| CUST-06 Taxi Goods rejects multi-pickup | PASS — COMPONENT | `TestTaxiGoodsCapabilityRefusesAMultiPickupCommerceMission`. |
| CUST-07 Hauling capability truth | PASS — LIVE | maxPickups=10 preserved; maxSecurePickups=1 declared; a protected three-pickup haul is refused `SECURE_PICKUP_LIMIT_EXCEEDED`. |
| CUST-08 delivery credential | PASS — COMPONENT | Receiver-held direction asserted in all three authorities. |
| CUST-09 replay pickup credential | PASS — DB-BACKED | `TestSecurePickupOTPAndQRShareSingleUseLifecycle`. |
| CUST-10 reassignment before pickup | PASS — DB-BACKED | `TestProviderReassignmentBeforePickupRevokesTheOldCredential` on the real migration chain. |
| CUST-11 reassignment after pickup | PASS — COMPONENT | `TestProviderSwapNeedsAnExplicitTransfer`; `custodyops` transfer workflow. |
| AUTH-01 organization spoof | PASS — DB-BACKED | `TestClientSuppliedOrganizationCannotBeSpoofed`. |
| AUTH-02 membership spoof | PASS — DB-BACKED | `TestClientSuppliedMembershipCannotBeSpoofed`. |
| AUTH-03 cross-service assignment | PASS — DB-BACKED | `TestCrossServiceCompletedWorkSpoofingIsRefused` (12 pairings). |
| AUTH-04 suspended member, new work | PASS — COMPONENT | `TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct`. |
| AUTH-05 suspended member, active work | PASS — DB-BACKED | `TestSuspensionDuringWorkStillRecordsTheCompletion`. |
| AUTH-06 provider RBAC | PASS — LIVE | A provider token on an internal route is 401. |
| AUTH-07 dispatcher RBAC | PASS — COMPONENT | Fleet permission manifest separates compensation and payroll. |
| EVT-01 mutation + outbox atomicity | PASS — DB-BACKED | `TestCustodyMutationAndItsEventShareOneTransaction`. |
| EVT-02 duplicate event | PASS — DB-BACKED | `TestIntegration_LateHandoverAfterDeliveryStaysANoOp`. |
| EVT-03 out-of-order event | PASS — DB-BACKED | Same test: a late pickup does not un-deliver a delivered parcel. |
| EVT-04 timeout, unknown outcome | PASS — COMPONENT | Section 12 `managedsettlement.Outcome.Unknown`, re-read under the same identity. |
| EVT-05 dead letter + requeue | PASS — LIVE + DB-BACKED | Live: operator list, requeue and refusals on Dispatch. DB: identity preserved, repair counted, delivered event refused. |
| REC-01 execution/Fleet drift | PASS — COMPONENT | Existing completed-work reconciliation suites. |
| REC-02 Vendor/execution drift | PASS — DB-BACKED | Vendor mission and custody reconciliation suites. |
| REC-03 Wallet/Fleet drift | PASS — DB-BACKED | Section 12 managed-settlement certification, re-run green. |
| NTF-01 assignment push lost | PASS — LIVE | The corporate assignment LIST route is mounted and closed to anonymous callers. |
| NTF-02 offer push lost | PASS — COMPONENT | Offer LIST is part of the same shared facade. |
| NTF-03 financial notification privacy | PASS — COMPONENT | `notifications.Validate` refuses concealed classes to provider recipients. |
| MED-01 evidence upload stores a reference | PASS — COMPONENT | Evidence tables carry `media_ref`; the schema comment states no bytes are stored. |
| MED-02 unauthorized evidence read | PASS — COMPONENT | Media retrieval requires a signed download authorization. |
| MED-03 expired signed URL | PASS — COMPONENT | The grant expires; the media reference does not change. |
| VER-01 verification pending | PASS — COMPONENT | Pending is not approved in Dispatch eligibility or Vendor projections. |
| VER-02 verification unavailable | PASS — COMPONENT | `VERIFICATION_STATE_UNAVAILABLE` is distinct from a requirements failure. |
| VER-03 corporate provider, failed safety | PASS — COMPONENT | Dispatch eligibility refuses regardless of sponsorship. |
| SAF-01 SOS on an active trip | PASS — COMPONENT | Complaints carry the booking reference and service type. |
| SAF-02 operator cancellation request | PASS — COMPONENT | support-dispute-service writes only its own tables. |
| SAF-03 SOS during goods custody | PASS — COMPONENT | `custodyops.IncidentCommand` requires a recovery action. |
