# KARRYAM Sections 15, 16, 17 and 18 — closure

Baseline: `5c20c108a2d266a3e75b492659df16af3e606726` (staging).

Sections 6–14 were treated as closed architecture throughout. Pricing was not
touched: no fare, quote, estimate, surge, fuel, traffic, per-kilometre or
per-metre calculation, and no bidding or dynamic pricing code, was read for
modification or changed.

---

## 1. What this phase actually found

The platform already implemented most of what Sections 15–18 ask for. The
audit's job was to find the places where it did not, and there were four that
mattered.

**Hauling could not start.** `cmd/main.go` registered the private-assignment
relay and the shared Section 10 company-assignment facade on the same provider
path. Gin panics when two handlers claim one route, so
`driver-hauling-service` died during route registration on every boot — not
only in corporate configurations. Nothing in Hauling's own suite built the
router the way `main.go` does, so nothing noticed. Fleet's black-box test
noticed, by timing out for 32 seconds waiting for a health check that would
never come.

**Vendor's custody chain accepted impossible moves.** A logistics authority
reporting DELIVERED for a parcel still sitting with the vendor was recorded as
`VENDOR -> CUSTOMER` and advanced the parcel to DELIVERED, although no
provider had ever collected it.

**Hauling over-promised protected pickup.** It declared multi-pickup up to ten
AND secure pickup, but `hauling_handover_challenges` is keyed
`(booking_id, stage)` with one ACTIVE row per stage. A protected three-pickup
mission would have passed the capability gate and then been unable to issue a
credential for its second pickup.

**Dispatch had no operator repair surface.** It had the Fleet delivery outbox,
the worker, the retry schedule and the `DEAD_LETTER` terminal state that Taxi
and Hauling have — and, unlike them, no way to look at any of it. Repairing a
dead-lettered Dispatch event meant editing the table by hand.

---

## 2. Section 15 — Commerce Fulfilment & Custody

| ID | Outcome | Evidence |
|---|---|---|
| CFC-001 freeze fulfilment authority | ALREADY EXISTED | `vendor-service/internal/integrations/logistics` persists the frozen authority with `SelectedBy`; re-selection requires an explicit replan (VND-004). |
| CFC-002 vendor remains sender | ALREADY EXISTED | `fulfilmentgroup.CustodianVendor` is the origin custodian of every commerce parcel; `custodychain.PartySender` is what it maps to. |
| CFC-003 vendor never selects provider | ALREADY EXISTED | Section 11's reflective structural test over `MissionIntent`, `MissionRequest`, `commercemission.Request` and both `dispatchcommerce` request types; preserved unchanged. |
| CFC-004 provider identity late-bound | ALREADY EXISTED | `MissionIntent` carries no provider field; identity arrives with the authority's assignment projection. |
| CFC-005 Commerce pickup credential | ALREADY EXISTED | Dispatch `ArrivePickup`/`VerifyPickup`, Taxi Goods `IssuePickupCredential`/`VerifyPickup`, Hauling `GetPickupCredential`/`ConfirmCommercePickup` — all provider-held, Vendor-verified. Certified live. |
| CFC-006 direct Taxi credential | ALREADY EXISTED | Taxi `VerifyPickup(bookingID, providerID, code)` is called by the provider with the customer's code. `goods.PurposeForStage` deliberately does not resolve it. |
| CFC-007 delivery credential | ALREADY EXISTED | Receiver-held, provider-verified in all three authorities. |
| CFC-008 credential type metadata | **DONE** | `custodychain.Direction` on `dispatchcommerce.PickupCredential`, `goods.Credential`, Hauling's `PickupCredentialView`/`VerificationCodeView` and Vendor's `handover.Observation`. |
| CFC-009 credential security | ALREADY EXISTED | Single ACTIVE challenge per context, terminal-state trigger, attempt ceiling, keyed hashes only. |
| CFC-010 QR and OTP are one authority | ALREADY EXISTED | `handover.NewSecureCodeRecord` issues both from one binding; `TestSecurePickupOTPAndQRShareSingleUseLifecycle`. |
| CFC-011 expired credential | ALREADY EXISTED | `expires_at` + `CodeRecord.IsExpired`; expiry now travels in `Direction.ExpiresAt` so a client can tell expired from wrong without a second call. |
| CFC-012 failed verification | ALREADY EXISTED | Attempts are counted and the challenge locks; execution is not cancelled. |
| CFC-013 explicit custody state | ALREADY EXISTED | `commerce_parcel_custody_events` is an append-only chain with from/to custodian, source and source version. |
| CFC-014 sender → provider | ALREADY EXISTED | Recorded on verified pickup; the manifest seals on the same transition. |
| CFC-015 provider → receiver | ALREADY EXISTED | Recorded on verified delivery. |
| CFC-016 custody immutability | ALREADY EXISTED | Append-only triggers in Dispatch; `ON CONFLICT DO NOTHING` plus monotonic projection in Vendor. |
| CFC-017 custody conflict | **DONE** | `custodychain.Allowed` + `fulfilmentgroup.checkCustodyTransition`. Sender→receiver, receiver→provider and provider→provider outside a transfer are refused with stable codes. |
| CFC-018 reassignment before pickup | ALREADY EXISTED (newly certified) | `dispatch_trip_handover_identity_guard` invalidates the ACTIVE challenge with reason `PROVIDER_REASSIGNED`; certified against the real migration chain. |
| CFC-019 reassignment after pickup | ALREADY EXISTED | `custodyops` transfer workflow; `custodychain` refuses provider→provider under any other operation. |
| CFC-020 pickup evidence | ALREADY EXISTED | `dispatch_evidence_refs` / `custody_evidence` store media references only. |
| CFC-021 delivery evidence | ALREADY EXISTED | Evidence is bound to mission, stop, provider and manifest version; it does not by itself complete a delivery. |
| CFC-022 multi-stop evidence | ALREADY EXISTED | Dispatch evidence and challenges are keyed per stop. |
| CFC-023 Dispatch capability | ALREADY EXISTED | `multiPickupSupported=true, maxPickups=12`; now also `maxSecurePickups=12`. |
| CFC-024 Taxi Goods capability | ALREADY EXISTED | `multiPickupSupported=false, maxPickups=1`; `maxSecurePickups=1` stated explicitly. |
| CFC-025 Hauling capability | **DONE** | Freshly audited. Multi-pickup routing is real and preserved at 10. The protected-pickup bound is 1 and is now declared, because the handover challenge is keyed per booking and stage. |
| CFC-026 Vendor consumes capability | ALREADY EXISTED | `logistics.Registry` asks the authority; Vendor hardcodes no size assumption. |
| CFC-027 capability versioning | ALREADY EXISTED | `Version` + `ObservedAt`. Hauling's version moved 1 → 2 with this declaration change. |
| CFC-028 execution completes | ALREADY EXISTED | Vendor's normalized status advances only from an authority report. |
| CFC-029 completion idempotency | ALREADY EXISTED | Dedupe on `(parcel_id, source, source_event_id)`, monotonic projection, idempotent Wallet keys. |
| CFC-030 financial handoff | ALREADY EXISTED | Sections 12–14 path reused; no second settlement path was created. |

## 3. Section 16 — Identity, Membership & Authorization

| ID | Outcome | Evidence |
|---|---|---|
| IAM-001 provider auth ownership | ALREADY EXISTED | Each execution service owns its own provider login. |
| IAM-002 corporate membership ownership | ALREADY EXISTED | `providercompat.ReadCorporateContext` is the single source. |
| IAM-003 never trust client organization | ALREADY EXISTED (newly certified) | `TestClientSuppliedOrganizationCannotBeSpoofed`. |
| IAM-004 never trust client membership | ALREADY EXISTED (newly certified) | `TestClientSuppliedMembershipCannotBeSpoofed`. |
| IAM-005 provider relay identity | ALREADY EXISTED | Relays carry the authenticated provider; Fleet resolves the rest. |
| IAM-006 source service ownership | ALREADY EXISTED | `callerOwnsSource`. |
| IAM-007 cross-service spoofing tests | **DONE** | `TestCrossServiceCompletedWorkSpoofingIsRefused` — 12 wrong pairings including Vendor and Wallet; plus provider-token-on-internal-route, certified live. |
| IAM-008 membership status | **DONE** | `providercompat.MembershipStatus` with Fleet's real ten values and a class for each. |
| IAM-009 suspended member | ALREADY EXISTED (now distinguishable) | Refusal names `SUSPENDED`. |
| IAM-010 expired member | ALREADY EXISTED (now distinguishable) | Refusal names `EXPIRED`. |
| IAM-011 terminated member | ALREADY EXISTED (now distinguishable) | Refusal names `TERMINATED` for DECLINED, CANCELLED, ENDING and REMOVED. |
| IAM-012 already-active work | **DONE** | Policy stated in `MembershipStatus.AllowsActiveWorkCompletion` and enforced structurally by `TestCorporateGateStaysOnAcquisitionPathsOnly`, which fails if the gate is added to a lifecycle step. |
| IAM-013 membership snapshot | ALREADY EXISTED (newly certified) | `TestSuspensionDuringWorkStillRecordsTheCompletion`. |
| IAM-014 RBAC roles | ALREADY EXISTED | Provider bearer, organization roles, internal HMAC callers and platform admin are separate surfaces. |
| IAM-015 provider permissions | ALREADY EXISTED | Provider reads are scoped to the authenticated provider. |
| IAM-016 organization dispatcher | ALREADY EXISTED | Fleet's permission manifest grants compensation and payroll separately. |
| IAM-017 organization admin | ALREADY EXISTED | Admin rights are explicitly permissioned, not inferred from membership. |
| IAM-018 internal service callers | ALREADY EXISTED (newly certified) | Per-operation allowlists; no universal bypass. Certified live on Hauling and Dispatch. |
| IAM-019 admin/operator audit | ALREADY EXISTED, extended | Fleet's audit chain; Dispatch requeues now record who repaired what and how often. |
| IAM-020 financial visibility | ALREADY EXISTED | Section 14 server projection; unchanged. |

## 4. Section 17 — Events, Outbox, Idempotency & Reconciliation

| ID | Outcome | Evidence |
|---|---|---|
| EVT-001 event envelope | ALREADY EXISTED (newly certified) | `TestDurableEventCarriesItsIdentityAndVersion`. |
| EVT-002 same-transaction outbox | ALREADY EXISTED (newly certified) | `TestCustodyMutationAndItsEventShareOneTransaction`. |
| EVT-003 no publish before commit | ALREADY EXISTED | Outbox pattern throughout; the same test proves it. |
| EVT-004 consumer idempotency | ALREADY EXISTED | Inbox dedupe and `ON CONFLICT DO NOTHING`. |
| EVT-005 aggregate version | ALREADY EXISTED | `aggregate_version` / `source_version` on every durable event. |
| EVT-006 out-of-order events | ALREADY EXISTED | Monotonic projections refuse regression rather than erroring. |
| EVT-007 retry policy | ALREADY EXISTED | Bounded exponential backoff in each outbox worker. |
| EVT-008 terminal failure | ALREADY EXISTED | `DEAD_LETTER` / `FAILED_TERMINAL`. |
| EVT-009 unknown outcome | ALREADY EXISTED | `RECONCILIATION_REQUIRED`; `managedsettlement.Outcome.Unknown`. |
| EVT-010 dead-letter visibility | **DONE** | Dispatch operator surface: `GET /api/v1/internal/dispatch/v1/operations/outbox`. Taxi and Hauling already had theirs. |
| EVT-011 safe requeue | **DONE** | `POST .../outbox/:id/requeue`. Payload, hash and aggregate version untouched; repair counted; a delivered event is refused. |
| EVT-012 assignment event | ALREADY EXISTED | Durable assignment projection; LIST reads recover it. |
| EVT-013 provider response event | ALREADY EXISTED | Fleet's authoritative response state. |
| EVT-014 execution start event | ALREADY EXISTED | Fleet execution-start reconciliation. |
| EVT-015 pickup/custody event | ALREADY EXISTED (newly certified) | Custody and its event share one transaction. |
| EVT-016 completion event | ALREADY EXISTED | Completion is durable in the execution service before anything downstream is told. |
| EVT-017 Commerce fulfilment event | ALREADY EXISTED | Vendor's normalized state converges through its reconciliation workers. |
| EVT-018 completed-work event | ALREADY EXISTED | Sections 8–10 exactly-once behaviour preserved and re-certified by the spoofing matrix. |
| EVT-019 settlement event | ALREADY EXISTED | Section 12 Wallet managed-settlement events untouched. |
| EVT-020 payroll event | ALREADY EXISTED | Fleet payroll posting recovery. |
| EVT-021 policy invalidation | ALREADY EXISTED | No service caches membership, operating mode or visibility policy: each is read per operation, so there is nothing to invalidate. The section's own guidance is not to add invalidation events where no dependent cache exists. |
| REC-001 execution services | ALREADY EXISTED | Taxi `managedexec` reconciliation, Hauling `completedwork` worker, Dispatch outbox worker plus the new operator surface. |
| REC-002 Fleet | ALREADY EXISTED | Finance operations, reconciliation cases, expiry and stuck-assignment sweeps. |
| REC-003 Vendor | ALREADY EXISTED | Mission, custody and payment-release reconciliation workers. |
| REC-004 Wallet | ALREADY EXISTED | Managed-settlement reconciliation worker from Section 12. |
| REC-005 stable repair identity | **DONE** | A Dispatch requeue reuses the event's own identity; nothing new is fabricated. |
| REC-006 reconciliation observability | **DONE** | The Dispatch operator row carries status, attempt count, last error, next retry and stable id — the last service that could not answer those without SQL. |
| REC-007 no silent force-complete | ALREADY EXISTED | Reconciliation reads authority state; `custodychain` refuses a terminal transition without evidence. |

## 5. Section 18 — Notifications, Media, Verification, Support & Safety

| ID | Outcome | Evidence |
|---|---|---|
| NTF-001 notification is not source of truth | ALREADY EXISTED | Every actionable notification has a LIST/detail API behind it (Section 10 added the corporate ones). |
| NTF-002 organization offer | ALREADY EXISTED | Offer LIST/detail is authoritative. |
| NTF-003 company assignment | ALREADY EXISTED | Assignment LIST/detail is authoritative; certified live as mounted. |
| NTF-004 provider response result | ALREADY EXISTED | Notified after the authoritative state commits. |
| NTF-005 credential availability | ALREADY EXISTED | Hauling's "verification required" notification deliberately carries no code; the app fetches the credential from the authority. |
| NTF-006 settlement | ALREADY EXISTED | `notifications.Validate` refuses a provider-addressed notification carrying a concealed financial class. |
| NTF-007 payroll | ALREADY EXISTED | Provider payroll view is projected by the same visibility policy. |
| NTF-008 support / SOS | ALREADY EXISTED | Support case events cover opened, responded, resolved. |
| NTF-009 idempotent notification | ALREADY EXISTED | Recipient-keyed idempotency (`bookingID:recipientID`). |
| MED-001 media service ownership | ALREADY EXISTED | Execution services store references; `dispatch_evidence_refs` is commented as carrying no bytes. |
| MED-002 typed media contract | ALREADY EXISTED | `shared/go/mediaclient` + typed upload authorizations. |
| MED-003 authorized retrieval | ALREADY EXISTED | `POST /objects/:id/download-authorizations` issues a signed, expiring grant. |
| MED-004 expiry | ALREADY EXISTED | The signed URL expires; the media reference does not change. |
| MED-005 stop-scoped evidence | ALREADY EXISTED | Evidence rows carry `stop_id`. |
| MED-006 retention | ALREADY EXISTED | `retention_until` and `legal_hold`. |
| MED-007 delete authorization | ALREADY EXISTED | `PhysicalDeletionBlocked` refuses deletion under retention or legal hold and records the block reason; providers cannot reach the internal media surface at all. |
| VER-001 typed verification result | ALREADY EXISTED | Verification-compliance's seven-value history including `PENDING_REVIEW`; Vendor's typed projections. |
| VER-002 unavailable is not rejected | ALREADY EXISTED | `VERIFICATION_STATE_UNAVAILABLE` is distinct from `VERIFICATION_REQUIREMENTS_NOT_SATISFIED`, and both fail closed. |
| VER-003 readiness | ALREADY EXISTED | Dispatch eligibility reads verification status, onboarding completeness and vehicle state. |
| VER-004 membership does not bypass safety | ALREADY EXISTED | Dispatch's eligibility check states it explicitly and enforces it: organization sponsorship never skips platform verification or suspension. |
| VER-005 explicit corporate bypass | ALREADY EXISTED | There is no bypass to encode; none is inferred from membership. |
| VER-006 verification expiry | ALREADY EXISTED (Dispatch, Vendor) / DEFERRED (Hauling) | Dispatch and Vendor treat an expired document as unverified. Hauling stores a licence expiry as free text and has no document-verification model, so there is nothing for expiry to transition. See remaining gaps. |
| VER-007 verification events | ALREADY EXISTED | Verification callbacks are durable, versioned and replay-protected. |
| SAF-001 incident references | ALREADY EXISTED (organization dimension deferred) | Complaints carry service type, booking reference, complainant, respondent and incident location. Organization is not carried; see remaining gaps. |
| SAF-002 execution ownership | ALREADY EXISTED (newly certified) | support-dispute-service writes only its own tables; it has no statement touching any trip or booking table. |
| SAF-003 SOS open | ALREADY EXISTED | Durable complaint with emergency priority. |
| SAF-004 SOS acknowledgement | ALREADY EXISTED | Acknowledgement, operator, timestamp and action are recorded. |
| SAF-005 safety escalation | ALREADY EXISTED | Escalation is a request; the owning execution service decides the lifecycle action. |
| SAF-006 operator audit | ALREADY EXISTED | `complaint_events` audit trail. |
| SAF-007 safety decline protection | ALREADY EXISTED | Fleet's protected decline reasons are unchanged. |
| SAF-008 active custody safety case | ALREADY EXISTED | `custodyops.IncidentCommand` with `SAFETY_INCIDENT` and an explicit recovery action; custody is never left unresolved. |
| SAF-009 support case recovery | ALREADY EXISTED | Case state is readable through the API regardless of push or email delivery. |

---

## 6. Closure decision

- **SECTION 15 — CLOSED.** Authority is frozen and persisted, Vendor remains
  the sender and never selects a provider, provider identity is late-bound,
  all three credential directions are explicit and structurally separated,
  replay is blocked, custody is explicit and its illegal transitions are now
  refused, evidence is scoped, capability is truthful in all three authorities
  including the protected-pickup bound that was over-promised, completion
  comes from the execution authority, and the Section 12 financial path is
  reused. Pricing untouched.
- **SECTION 16 — CLOSED.** Provider auth ownership is unchanged, Fleet
  membership is authoritative, organization and membership spoofing both fail,
  cross-service assignment spoofing fails across twelve pairings, suspended,
  expired and terminated members cannot take new managed work and are
  distinguishable, the active-work policy is explicit and structurally
  enforced, the membership snapshot survives, RBAC is enforced per surface and
  financial visibility remains a server decision.
- **SECTION 17 — CLOSED.** Mutation and event atomicity is proved rather than
  asserted, events carry stable identity and version, consumers are
  idempotent, out-of-order events cannot regress state, retry is bounded,
  terminal failure is visible, ambiguous outcomes reconcile, and every one of
  the six services can now be repaired by an operator without SQL — Dispatch
  was the last that could not.
- **SECTION 18 — CLOSED.** Notifications are secondary to recoverable API
  state and respect financial visibility, credential notifications carry no
  reusable secret, evidence is media-service references under retention and
  legal hold, verification is typed with unavailable distinct from rejected,
  corporate membership does not bypass safety readiness, support and safety
  reference executions without mutating them, and custody safety incidents
  have an explicit recovery path. Two dimensions are listed below as genuine
  remaining gaps; neither is in the section's own definition of done.

## 7. Remaining gaps (genuine, not restatements)

1. **Hauling has no document-verification model (VER-006).** Its licence
   expiry is a free-text profile field that no readiness gate reads. Dispatch
   has `verification_steps` with expiry; Hauling has provider status and
   onboarding completeness only. Closing this is a feature — a verification
   dimension for hauling providers and trucks — not a conformance change, and
   inventing one here would have meant guessing which documents Hauling
   actually requires.

2. **A safety incident cannot name the organization (SAF-001).** Complaints
   carry the execution reference, the service, the complainant and the
   respondent, and identity is resolved from the owning execution service. But
   the internal identity endpoints return id, name, phone and status — not
   corporate membership — so an incident involving an organization-managed
   provider does not record which organization. Closing this means
   support-dispute-service reading Fleet's corporate context, which is a new
   cross-service dependency with its own credential and configuration.

3. **Dispatch's Commerce pickup credential is compatibility-encoded.**
   `securehandover.CompatibilityCode` wraps the code for the legacy transport.
   It is correct and tested, but it is the one place where the credential a
   provider sees is not the credential the store holds, and it should be
   retired when the legacy transport is.
