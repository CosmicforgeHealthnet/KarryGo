# Platform Service Change Register

## Section 7 contract correction

The authoritative cross-service status vocabulary is `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` remains reserved for unrelated delivery/operation queues. Because Section 7 migrations are still uncommitted and absent from Git history, their projection/case history constraints were corrected in place rather than adding a needless migration. The callback composition audit corrected the accidental doubled path to root-level `POST /internal/vendor/v1/verification-events`; Verification's worker target and Vendor OpenAPI/tests were updated together. API Gateway remains unchanged and does not expose this route.

## Section 7 pre-change authority audit (recorded before external edits)

Repository inspection on 2026-08-11 produced the following classifications. These are the controlling Section 7 findings and supersede older architecture-phase assumptions where the repository has since diverged.

| Capability | Classification | Evidence and consequence |
|---|---|---|
| `VERIFICATION_VENDOR_SUPPORT` | **PARTIAL** | `verification-compliance-service` has a real HMAC-v2, allowlisted, nonce-protected organization-submission workflow with idempotency, read-back, decisions, history/versioning and outbox, but it has no Vendor subject or the six Section 7 dimensions. Section 7 may add an additive Vendor-case contract while preserving this service as formal decision authority. |
| `MEDIA_REFERENCE_SUPPORT` | **AVAILABLE** | Correction recorded before any media-service edit: the authority is named `services/media-file-service` (the initial search used the wrong directory name). Its private HMAC-v2 API already provides nonce-protected, tenant/owner-scoped `GET /api/v1/media-files/internal/media/objects/:id` read-back, exact upload/safety/deletion state and source version. Vendor Service will call that contract and store only the opaque object id; deployment must allowlist `vendor-service`. |
| `BANK_RESOLUTION_SUPPORT` | **PARTIAL** | `payment-wallet-service` can resolve and register provider bank accounts through Paystack, but its existing routes require provider bearer identity and expose no Vendor-onboarding HMAC subject contract. Section 7 may add only an additive internal resolve/register/read projection for `vendor_business`, with Wallet retaining bank authority and no settlement/ledger/withdrawal expansion. |
| `ADMIN_VENDOR_REVIEW_SUPPORT` | **NOT_PRESENT** | `admin-backoffice-service` contains only scaffold READMEs and no review routes, persistence, or service-auth integration. Section 7 will not fake operator approval; manually gated cases remain blocked as `PENDING_EXTERNAL_DEPENDENCY — VENDOR ADMIN REVIEW`. |
| `NOTIFICATION_SUPPORT` | **PARTIAL** | `notification-service` has HMAC-protected push, email, websocket and in-app delivery, but no SMS implementation and no committed Vendor allowlist entry. Section 7 will emit durable domain events and will not claim delivery until deployment allowlisting exists. |

### Authorized minimal external changes for Section 7

- **verification-compliance-service**: additive Vendor verification cases, exact Section 7 status/dimension vocabulary, idempotent create/read/read-by-client-reference/admin decision routes, source-versioned history and outbox. Auth: existing HMAC v2 plus database nonce replay defense; caller allowlist narrowed by action. Migration: next sequential `000002`, including additive callback-delivery state on the existing outbox. A leased signed callback worker has bounded retry/dead-letter/replay; Vendor also polls by client reference until terminal. Rollback removes only the additive Vendor tables/columns/routes. Tests cover create/replay/read/decision/stale-version/auth/nonce/callback delivery/operator replay.
- **payment-wallet-service**: additive internal Vendor bank-onboarding resolve/register/read contract using existing Paystack and provider-bank persistence authority, returning only safe projection fields. Auth: HMAC v2 and explicit `vendor-service` caller check. Migration: none expected because existing provider bank records accept a namespaced provider type/id. Rollback: remove only the three internal routes/adapter. Feature flag: existing Wallet service availability plus Vendor-side integration flag. Tests: caller isolation, resolution, idempotent registration and masked response.
- **media-file-service**: no schema or route change; reuse the existing private object read contract. Deployment config must add `vendor-service` to `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS` and its secret to `MEDIA_FILE_SERVICE_SECRETS`. Vendor Service adds a fail-closed read client and object-state/ownership validation before accepting a document reference.
- **admin-backoffice-service**: **NONE**; implementing a full operator product exceeds the permitted minimal adapter and no generic review architecture exists.
- **notification-service**: no code/schema/route change; deployment must add `vendor-service=<secret>` to `NOTIFICATION_SERVICE_SECRETS` before delivery is available. No SMS is claimed.
- **api-gateway**: no change; its routing module is a scaffold and Vendor Service follows the repository's direct-service convention. Internal callbacks must never be exposed publicly.

Everything Vendor Service implementation requires outside `services/vendor-service`, tracked here as it is identified — not implemented speculatively ahead of the feature section that actually needs it, per the architecture task's own scope limits.

## Section 6 closure - no external service changes

- **API Gateway**: **NO CHANGE**; vendor-service remains directly addressable under the established local routing posture.
- **Media Service**: **NO CHANGE**; Vendor Service stores syntax-checked opaque media references only and does not claim media-file authority.
- **Verification, wallet, catalogue, dispatch, taxi, hauling, support, notification**: **NO CHANGE**; Section 6 does not call or mutate these services.
- **Admin backoffice, analytics, fleet-partner, customer service**: **NO CHANGE**; Section 6 adds no route, consumer, schema, or authority change to them.
- **Database ownership**: **NO CHANGE outside vendor-service**; migrations `000006` and `000007` belong only to the vendor-service database.

## Entry format

- **Implementation section**: which Vendor Service work triggered this
- **Affected service**: the other service that needs to change
- **Affected path/module**: where in that service
- **Change required**: precisely what
- **Why required**
- **Authority/boundary**: confirms the change respects that service's ownership (never business logic migrating into the wrong service)
- **New/changed migration**
- **New/changed route**
- **New/changed event**
- **New/changed configuration**
- **New/changed feature flag**
- **Tests added**
- **Rollback consideration**
- **Status**

---

## 1. `payment-wallet-service` — allowlist `vendor-service` as an HMAC caller

- **Implementation section**: architecture foundation (identified now; activated when checkout/payments are implemented)
- **Affected path/module**: `internal/config/config.go`, `PAYMENT_WALLET_SERVICE_SECRETS` (and `PAYMENT_WALLET_FINANCE_ALLOWED_CALLERS` if the finance-private API is used)
- **Change required**: add `vendor-service=<secret>` to the allowlist env var
- **Why required**: `payment_intents.source_service` is already free-text with no schema constraint (confirmed by reading `migrations/001_wallet_ledger.sql`), so the only real gate is HMAC caller identity
- **Authority/boundary**: no change to `payment-wallet-service`'s ledger authority; Vendor Service remains a caller, never a competing ledger
- **New/changed migration**: none
- **New/changed route**: none — reuses existing `POST /internal/payment-intents` and related endpoints
- **New/changed event**: none
- **New/changed configuration**: `PAYMENT_WALLET_SERVICE_SECRETS` env var value (deployment config, not code)
- **New/changed feature flag**: none on `payment-wallet-service`'s side; gated on Vendor Service's own `PAYMENT_WALLET_COMMERCE_ENABLED`
- **Tests added**: none yet
- **Rollback consideration**: removing the allowlist entry immediately blocks Vendor Service calls; no data migration to reverse
- **Status**: **NOT_STARTED** (no code depends on it yet; documented so the first checkout-implementing session doesn't have to re-derive this)

## 2. `media-file-service` — allowlist `vendor-service` and add commerce media purposes

- **Affected path/module**: `MEDIA_FILE_SERVICE_SECRETS`/`MEDIA_FILE_SERVICE_TOKENS` config; `internal/features/uploads/usecases/upload_service.go`'s `isAllowedPurpose()` code allowlist
- **Change required**: (a) allowlist `vendor-service` as a caller identity (config-only); (b) add purposes such as `vendor_logo`, `branch_photo`, `menu_item_photo` to the code allowlist when a feature actually needs them
- **Why required**: `owner_service` is enforced at the app layer to equal the caller's own HMAC identity — this is a real code gate, not free text in practice, even though the column itself has no DB constraint
- **Authority/boundary**: media-file-service remains the sole media authority; Vendor Service stores only references
- **New/changed migration**: none
- **New/changed route**: none
- **New/changed configuration**: `MEDIA_FILE_SERVICE_SECRETS`/`MEDIA_FILE_SERVICE_TOKENS`
- **Status**: **NOT_STARTED**

## 3. `notification-service` — allowlist `vendor-service`

- **Affected path/module**: `NOTIFICATION_SERVICE_SECRETS`
- **Change required**: add `vendor-service=<secret>`
- **Why required**: notification requests already accept a caller-supplied `source_service` string; only the HMAC allowlist gates who may send
- **New/changed migration**: none
- **Status**: **NOT_STARTED** — Section 5's client side (`internal/features/auth/clients.OTPNotifier`) is fully built and wired into `cmd/main.go` (constructed whenever `VENDOR_NOTIFICATION_URL` is set), but the actual allowlist entry on `notification-service` itself has not been made, so real (non-debug) email OTP delivery will 403 until this config change lands. Confirmed during Section 5 testing (all tests run with `VENDOR_AUTH_OTP_DEBUG=true`, which skips real delivery entirely — see below).
- **Additional finding from Section 5 (not previously known)**: `shared/go/notifications`' channel set is exactly `{push, email, websocket, in_app}` — **there is no SMS channel anywhere in the platform**, and push/in-app both require an already-registered device/session that cannot exist pre-login. Phone-number OTP delivery therefore has **no real production channel today, for any service**, not just Vendor Service (confirmed customer-service's own `LoggingOTPSender` falls back to logging for phone too, in production without printing the raw code, and — separately — prints the raw code in `debug=true` mode, which Section 5's own rule 11 explicitly forbids and this service deliberately does not replicate). Vendor Service's `OTPNotifier.SendOTP` returns a clear `service_unavailable` error for phone identifiers rather than pretending to deliver. **This is a platform-wide product gap** (a real SMS provider integration on `notification-service`, e.g. Termii/Twilio), not something in scope for this service to build, and is recorded here as a new, previously-undocumented dependency for whichever team owns `notification-service` next.

## 4. `verification-compliance-service` — Vendor verification authority

- **Affected path/module**: `internal/features/vendors`, migration `000002`, configuration and composition root
- **Change implemented**: idempotent create/read/read-by-client-reference/admin-decision, source-versioned history, durable callback delivery, exact six-dimension vocabulary
- **Why required**: formal verification must remain outside Vendor Service
- **Authority/boundary**: verification-compliance-service remains the sole formal-verification-decision authority; Vendor Service never self-certifies a vendor as verified
- **New/changed migration**: `000002_vendor_verification_cases`
- **New/changed route**: four `/internal/vendor-cases/...` routes
- **New/changed event**: `vendor.verification.case_created`, `vendor.verification.status_changed`
- **Status**: **IMPLEMENTED AND VERIFIED**; deployment must configure mutual HMAC/callback credentials. `admin-backoffice-service` remains the missing operator product.

## 5. `support-dispute-service` — new vendor caller route group

- **Affected path/module**: `internal/features/support/http/routes.go` (new `/vendor` group, mirroring the existing 3 provider-app groups), `complainant_type` enum
- **Change required**: a new bearer secret (`SUPPORT_DISPUTE_VENDOR_TOKEN_SECRET` or similar) and a new `complainant_type` value
- **Why required**: today's route groups are customer + 3 provider apps only; there is no vendor caller class
- **New/changed migration**: yes (widen `complainant_type` enum/check)
- **New/changed route**: yes
- **Status**: **NOT_STARTED**

## 6. `driver-dispatch-delivery-service` / `driver-hauling-service` / (future) `driver-taxi-service` — the real OTP/QR handover credential

- **Change required**: a genuine one-time-code/QR challenge-response system for pickup and delivery custody confirmation, replacing today's evidence-capture-only proof flows
- **Why required**: confirmed platform-wide absence — see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)
- **Authority/boundary**: this is a platform-wide logistics capability, not something Vendor Service is authorized to build a duplicate of; it should be built once, coordinated across the owning logistics services, most plausibly as an extension of dispatch's existing `ConfirmRecipient`/WP6.3 idempotency/fingerprint scaffolding
- **New/changed migration**: yes, on whichever service ends up owning it
- **New/changed route**: yes
- **Status**: **PENDING_EXTERNAL_DEPENDENCY** — this is the specification's own Case C outcome, not a Vendor Service implementation gap. Blocks only the final confirmed-handover step of platform-delivery fulfilment; does not block anything else in Vendor Service.

## 7. `driver-taxi-service` — future GOODS_DELIVERY capability

- **Change required**: an entirely new feature on a currently-empty service, distinct from passenger rides
- **Why required**: per CLAUDE.md's "Taxi Important Boundary" and the specification, this architecture phase records the future boundary only
- **Status**: **NOT_STARTED** — no implementation attempted, per explicit task instruction

## 8. `admin-backoffice-service` / `analytics-service` — routes/consumers once those services exist

- **Change required**: vendor-moderation routes on admin-backoffice-service; event consumption on analytics-service
- **Why required**: both are currently pure scaffolds with zero routes/migrations — there is nothing to integrate with yet
- **Status**: **NOT_STARTED**, blocked on those services' own foundational build-out (not a Vendor Service dependency to resolve)

## 9. `api-gateway` — future routing migration

- **Change required**: none required now. Vendor Service exposes its own direct routes (matching the existing customer/hauling/dispatch precedent) until a generic gateway routing layer exists (`api-gateway`'s `routing/`/`service_health/` are currently empty scaffolds)
- **Status**: **NOT_STARTED**, and not urgent — no Vendor Service code should depend on being reached through a gateway specifically

## 10. `CLAUDE.md` — documentation gaps found during this audit (partially closed this session)

- **Change required**: `fleet-partner-service` was not listed anywhere in CLAUDE.md's own service documentation despite being the platform's largest, most mature service; `infra/docker-compose.yml` and multiple `scripts/*bootstrap*.sh` files are referenced as if committed but do not exist anywhere in git history
- **Why required**: accuracy for future sessions
- **What was actually done**: this session added a one-line `fleet-partner-service` entry and a `vendor-service` entry to CLAUDE.md's "Backend Services"/"Default local databases"/"Important local ports" lists, and disclosed the docker-compose/bootstrap-script gap explicitly in [architecture/00_REPOSITORY_AUDIT_BASELINE.md](architecture/00_REPOSITORY_AUDIT_BASELINE.md) rather than fabricating those files
- **Status**: **PARTIAL** — full fleet-partner-service documentation and recreating the platform's local-dev tooling are both out of scope for this task

## Section 8A — CRM Foundation, Customer 360, Consent and Privacy

**No external service was modified for Section 8A.** Every entry below is either NO CHANGE or a recorded pending dependency. The repository was audited before any code was written, and where an authority does not exist the integration was left dormant rather than invented.

### 11. `customer-service` — NO CHANGE (three pending authority contracts recorded)

- **Audited on 2026-08-11.** Features present: `auth`, `identity`, `notifications`, `preferences` (directory scaffolding only, no Go files), `profile`, `request_history`, `saved_locations`. Migrations `001`–`005`.
- **Consent**: no consent concept exists anywhere in the service — no table, no field, no route, no event. **PENDING_EXTERNAL_DEPENDENCY — CUSTOMER MARKETING CONSENT AUTHORITY.** Vendor CRM defines the projection, the source-version rules and the `CUSTOMER_CONSENT_CHANGED` contract, and keeps ingestion disabled behind `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED`. Until an authority exists every decision is UNKNOWN and marketing is ineligible.
- **Privacy**: no delete-request, restriction, redaction or suppression concept exists. **PENDING_EXTERNAL_DEPENDENCY — CUSTOMER PRIVACY AUTHORITY.** The `CUSTOMER_PRIVACY_CHANGED` contract and the full redaction engine are implemented and tested; nothing currently instructs them.
- **Identity merge**: no identity-link, guest-upgrade or account-merge concept exists. **PENDING_EXTERNAL_DEPENDENCY — CUSTOMER IDENTITY LINK AUTHORITY.**
- **Presentation**: `GET /internal/customers/:id` (HMAC service-auth) exists and returns `{id, name, phone, email, status}`. It is **not** usable as a CRM display source: it carries canonical contact data, and storing any of it would make Vendor Service a second customer master. Vendor CRM therefore leaves `safe_display_label` permanently null. **PENDING_EXTERNAL_DEPENDENCY — PII-FREE CUSTOMER PRESENTATION CONTRACT** (a display-only projection with no contact fields) if vendor staff ever need a human-readable label.
- **Why no change was made now**: none of the four is required for Section 8A to be correct and deployable. Adding speculative consent/privacy tables to customer-service would be designing another service's domain from inside this one, which the service-boundary rules forbid.
- **Status**: **NO CHANGE** (four dependencies recorded above).

### 12. Vendor commerce order publisher — DOES NOT EXIST

- `services/vendor-service/internal/features/orders` is still a reserved `doc.go` boundary; no service publishes vendor commerce order events.
- Vendor CRM defines the `CRM_ORDER_FACT_CHANGED` contract, implements the fact projection, dedupe, source-version handling, refund-safe recomputation and rebuild path, and tests all of it against fixtures. The allowlisted caller is named (`vendor-order-service`) and the integration stays disabled behind `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED`.
- **No publisher was fabricated.** Until the order section exists, every CRM commercial figure honestly reports `NOT_AVAILABLE`.
- **Status**: **PENDING_EXTERNAL_DEPENDENCY — VENDOR COMMERCE ORDER AUTHORITY.**

### 13. `notification-service` — NO CHANGE

- Audited for Section 8A because later CRM packages (8E/8F campaigns) will depend on it. Channels are exactly push/email/websocket/in_app; there is no SMS sender.
- `notification_preferences` (`recipient_type`, `recipient_id`, `channel`, `enabled boolean DEFAULT true`) is a **delivery preference toggle owned by the delivery service**, not a marketing-consent authority: it has no purpose dimension, no source version, no consent semantics, and defaults to opt-in. Vendor CRM deliberately does **not** map it to consent — treating `enabled=true` as marketing ALLOW would manufacture permission nobody granted.
- Section 8A sends nothing and requires no notification change.
- **Note for later packages**: SMS *consent* is representable in the CRM projection even though SMS *delivery* does not exist. A future campaign package must not dispatch on a channel with no sender.
- **Status**: **NO CHANGE.**

### 14. `api-gateway` — NO CHANGE

- The six public CRM routes follow the existing direct-exposure precedent and need no gateway work.
- `POST /internal/vendor/v1/crm/events` is root-mounted and **must never be gateway-exposed**: it is the authority write path into consent, privacy and identity projections. It is not nested under `/api/v1/vendor`, repeating the Section 7 callback correction, and `internal/section8acontract` asserts that structurally.
- **Status**: **NO CHANGE.**

### 15. `payment-wallet-service` — NO CHANGE

- Section 8A holds no financial authority. CRM commercial figures are a read-model projection of order facts, not a balance, receivable, settlement or credit line. Nothing was read from or written to wallet.
- **Status**: **NO CHANGE.**

### 16. `support-dispute-service` — NO CHANGE

- Complaints, disputes and adjudication remain outside CRM authority. Section 8A implements no interactions (that is 8C), so nothing was needed.
- **Status**: **NO CHANGE.**

### 17. `media-file-service` — NO CHANGE

- Section 8A stores no media and performs no export (exports are 8H).
- **Status**: **NO CHANGE.**

### 18. `analytics-service` / `admin-backoffice-service` — NO CHANGE

- Both remain scaffolds. Section 8A emits its CRM outbox events durably (`vendor.crm.profile.created`, `.archived`, `.restored`, `.linked`, `.privacy_state.changed`); with no configured publisher they accumulate as `PENDING` and are never silently dropped, matching the Section 5/6/7 precedent.
- **Status**: **NO CHANGE.**

---

## Section 8B — Tags & Advanced Segmentation

Every platform service was reviewed for a required change. **All eighteen entries above remain NO CHANGE, and Section 8B adds none of its own.**

The targeted 8B correctness pass is also Vendor-Service-local: it adds no route, event, migration, credential or cross-service contract. The materialization fence, dependency invalidation, immediate privacy membership cleanup and Customer 360 scope correction all operate over existing Vendor-owned rows. Consent decisions remain owned by the existing authority and are never rewritten by restriction or marketing suppression.

That is not an oversight — it follows from what the section is. Segmentation reasons entirely over data Vendor Service already owns: its own tags, and the Section 8A relationship, commercial, consent and privacy projections. It consumes no external authority, so it needs no new caller allowlist, no new signed callback and no new integration credential. It contacts nobody, so it needs nothing from `notification-service`.

The reviews worth recording explicitly:

### `customer-service` — NO CHANGE

- Section 8B stores no canonical customer identity and adds no field that could hold one. Its field registry is a closed allowlist, and there is deliberately no `customer.email`, `customer.phone`, `customer.address` or `customer.date_of_birth` key in it — a rule cannot reference what the registry does not publish, and a test asserts those keys are absent rather than merely filtered.
- No new internal endpoint was needed on customer-service, and none was added.
- **Status**: **NO CHANGE.**

### `notification-service` — NO CHANGE

- This is the one worth stating plainly: Section 8B builds audiences and **sends nothing**. There is no route, permission, worker or event in this section that contacts a customer, and the contract test rejects a campaign- or send-shaped route structurally.
- Marketing eligibility remains a Section 8A derivation. A segment may *read* eligibility as a rule field, but segment membership is never permission — whatever section eventually sends must re-check consent and privacy at send time, against the state then rather than the state when the audience was built.
- The existing platform-wide gap (no SMS channel anywhere) is unchanged and unaffected: SMS consent stays representable and honoured if a sender ever appears.
- **Status**: **NO CHANGE.**

### `payment-wallet-service` — NO CHANGE

- Section 8B holds no financial authority. Money appears only as an integer count of minor units read from the Section 8A commercial projection, compared against a threshold. Nothing is charged, settled or reconciled, and no wallet endpoint is called.
- Cross-currency comparison is impossible by construction: Section 8A refuses order facts denominated differently from the business currency, so a rule's currency is validated but never converted.
- **Status**: **NO CHANGE.**

### `media-file-service` — NO CHANGE

- Section 8B stores no media and performs no export. Audience export belongs to Section 8H and will need its own review, its own step-up requirement and probably its own media integration.
- **Status**: **NO CHANGE.**

### `verification-compliance-service` — NO CHANGE

- Segmentation makes no verification decision and reads no verification state. No field in the registry exposes one.
- **Status**: **NO CHANGE.**

### `support-dispute-service` — NO CHANGE

- Complaints and disputes remain outside CRM authority. Section 8B implements no interactions (that is 8C).
- **Status**: **NO CHANGE.**

### `api-gateway` — NO CHANGE

- The 21 public routes follow the existing direct-exposure precedent established by Sections 5-8A and need no gateway work.
- Section 8B registers **no internal route at all**, so there is nothing here that must be kept off the gateway. That is the strongest form of the Section 7 callback lesson: the safest internal surface is the one that does not exist.
- **Status**: **NO CHANGE.**

### `analytics-service` / `admin-backoffice-service` — NO CHANGE

- Both remain scaffolds. Section 8B appends its domain events to the same transactional outbox as Sections 5-8A: `vendor.crm.tag.created`, `.tag.updated`, `.tag.archived`, `vendor.crm.segment.created`, `.updated`, `.activated`, `.archived`, `.materialized`. With no configured publisher they accumulate as `PENDING` and are never silently dropped.
- The `materialized` event deliberately carries the audience's **identity** — segment reference, definition version, definition hash, generation, member count, evaluated-at — and never its contents. A member list on an event bus would be a customer export nobody asked for.
- **Status**: **NO CHANGE.**

### Future reference protection (recorded now, acted on later)

`internal/features/crm.GenerationRetainer` is a port that decides whether a superseded membership generation may be cleaned up. Section 8B always answers "yes" because nothing references a past generation yet. Section 8E should implement it so a campaign can pin the exact generation it launched against, and so cleanup can never delete an audience a campaign is still reconciling against. This is a Vendor Service-internal seam, not a cross-service change.

## Section 8C review — all external services NO CHANGE

| Service | Status | Section 8C boundary |
|---|---|---|
| `customer-service` | **NO CHANGE** | It remains canonical identity/privacy/merge authority. Vendor consumes the already-established opaque profile and privacy seams; no new endpoint or publisher is fabricated. |
| `notification-service` | **NO CHANGE** | Due dates and overdue state are reminder-ready only. Section 8C sends no email, SMS, push, or in-app message. |
| `media-file-service` | **NO CHANGE** | Interactions and tasks accept no attachments or file payloads. |
| `payment-wallet-service` | **NO CHANGE** | CRM activity contains no money, wallet, refund, ledger, or settlement authority. |
| `support-dispute-service` | **NO CHANGE** | A CRM interaction is not a support case or dispute, and Section 8C creates no case lifecycle or evidence store. |
| `analytics-service` | **NO CHANGE** | Outbox events contain low-detail CRM identifiers/status metadata only; no analytics consumer or customer export was added. |
| `api-gateway` | **NO CHANGE** | Sixteen public routes follow the existing Vendor exposure; Section 8C adds zero internal routes. |

No repository code outside `services/vendor-service` is required for Section 8C. Future reminder delivery remains an explicit Notification Service integration dependency, not an implicit side effect of a due date.

## Section 8D review — all external services NO CHANGE

| Service | Status | Section 8D boundary |
|---|---|---|
| `customer-service` | **NO CHANGE** | Loyalty references the vendor-local CRM profile only. It stores no canonical identity or PII, and consumes privacy and identity-merge decisions through the existing Section 8A ingress rather than through any new contract. |
| `payment-wallet-service` | **NO CHANGE** | This is the boundary that mattered most. Section 8D holds a POINTS ledger: no currency, no minor-unit amount, no wallet reference, no conversion, no cash-out. It creates no wallet ledger entry, initiates no payout or refund, and emits no `payment.*`, `wallet.*`, `refund.*`, `settlement.*` or `payout.*` event. A schema test and a contract test both reject a monetary column or field structurally, so the boundary is enforced rather than merely intended. |
| `notification-service` | **NO CHANGE** | Nothing here contacts a customer. Points expiring soon is a read-model field, not a reminder; expiry notifications would be Section 8E/8F work and would need their own integration. |
| `media-file-service` | **NO CHANGE** | Loyalty has no document or media surface. |
| `support-dispute-service` | **NO CHANGE** | A points adjustment is not a support case; no case lifecycle or evidence store is created. |
| `analytics-service` | **NO CHANGE** | Balances and history are read through the Vendor API. No analytics export or read model was added. |
| `api-gateway` | **NO CHANGE** | Six public routes follow the existing Vendor exposure; Section 8D adds zero internal routes. |

No repository code outside `services/vendor-service` is required for Section 8D.

**Deferred integration dependencies, not fabricated.** The earning and redemption engines exist as internal domain primitives (`CreditPoints`, `DebitPoints`) with durable source-event deduplication, waiting for an authoritative producer. The Vendor Order domain that would call them is Section 9 work and does not exist, so Section 8D invents no order event, no earning formula, no checkout redemption and no order-rollback path. When that producer arrives it calls the primitives directly in-process; no new internal HTTP route is implied.

## Section 8E review — all external services NO CHANGE

| Service | Status | Section 8E boundary |
|---|---|---|
| `notification-service` | **NO CHANGE** | This is the boundary that mattered most. Section 8E prepares audiences and delivers nothing: no send endpoint, no scheduler, no template, no provider client, no callback handler, no delivery worker and no `scheduled_at`/`sent_at`/`delivered_at` column anywhere. It emits no `campaign.sent`, `notification.sent`, `sms.sent` or `email.sent` event. A migration test rejects a delivery column structurally, a contract test rejects a delivery path or schema, and an HTTP test asserts `POST .../send`, `/schedule`, `/deliver` and `/dispatch` all 404. The boundary fails the build rather than drifting. |
| `customer-service` | **NO CHANGE** | Campaign targets reference vendor-local CRM profile ids only. No canonical PII is read, stored or returned, and consent and privacy remain projections of that service's authority, consumed through the accepted Section 8A ingress. Section 8E adds no new consumption path. |
| `payment-wallet-service` | **NO CHANGE** | Campaigns hold no currency, amount, wallet reference or settlement. Nothing here is monetary. |
| `media-file-service` | **NO CHANGE** | No campaign creative, asset or attachment exists in Section 8E; there is no message body to attach anything to. |
| `support-dispute-service` | **NO CHANGE** | Service-recovery suppression uses the Vendor-local Section 8C signal only — an `OPEN`/`IN_PROGRESS` task linked to a `SERVICE_FOLLOW_UP` interaction. No dispute state is read, requested or inferred, and none is fabricated. `EXTERNAL_SERVICE_RECOVERY_SUPPORT = NOT_PRESENT`. |
| `verification-compliance-service` | **NO CHANGE** | Untouched; campaigns have no verification dimension. |
| `analytics-service` | **NO CHANGE** | Campaign experiment cohorts are recorded locally for a future analysis that does not exist yet. No read model, export or analytics contract was created. |
| `api-gateway` | **NO CHANGE** | Ten public routes follow the existing Vendor exposure; Section 8E adds zero internal routes. |

No repository code outside `services/vendor-service` is required for Section 8E.

**Deferred integration dependencies, not fabricated.**

- **Delivery (Section 8F).** The whole execution half — scheduling, notification handoff, send-time consent revalidation, send-time frequency enforcement against real delivery facts, send-time quiet-hours deferral, provider callbacks and delivery history — is deliberately absent. Section 8E's contribution to it is the mandatory recheck flags every prepared channel and snapshot carries, which are pinned true so 8F cannot accidentally treat `PREPARED` as authorisation.
- **Consent authority.** No service publishes authoritative marketing consent to this platform yet. Section 8E does not invent one: with `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED` false every channel is ineligible with `CONSENT_SOURCE_UNAVAILABLE`, which is the correct and safe answer rather than a degraded one.
- **Contact history.** Section 8E still owns no delivery facts. Section 8F adds Vendor-local opaque handoff history and enforces frequency caps at handoff time; it stores no destination PII.
- **Vendor commerce orders.** Still Section 9 work. Commercial segment rules therefore still match nobody, inherited unchanged from Section 8B.

## Section 8F external-service audit

- **Notification Service: no code change.** Existing HMAC v2 `POST /api/v1/notifications/send` plus durable idempotency supports email, push, and in-app handoff. SMS is absent and Vendor suppresses it. Notification currently has no delivery callback/event producer; Vendor's secured monotonic receiver is implemented, but production delivery receipts are a disclosed dependency.
- **Customer Service: no code change.** Vendor passes only its existing opaque canonical customer reference; Notification owns destination resolution.
- **All other services: no change.** No fabricated order, conversion, support/dispute, analytics, automation, AI, loyalty, payment, or export integration was added.

## Section 8G external-service audit

**No external service changed. None needed to.** Section 8G reasons entirely over Vendor-owned data plus the Section 8A customer relationship projection that already exists.

- **customer-service: no change.** A corporate contact stores the opaque Vendor-local profile reference Section 8A already holds. No new lookup, no contact resolution, no identity call. Corporate accounts are explicitly **not** customer-service records: the organization exists only inside this vendor's tenancy, and only its people are platform customers.
- **payment-wallet-service: no change.** Nothing here holds money. Cost centres carry no budget, limit, currency or balance, and there is no corporate credit, invoice or settlement concept to integrate.
- **notification-service: no change.** Section 8G contacts nobody. Recording a relationship sends no message, and no corporate route touches a delivery path.
- **verification-compliance-service: no change.** A corporate customer is not a vendor being onboarded; no KYB flow, verification dimension or document reference is involved.
- **media-file-service: no change.** No corporate document, logo or attachment surface exists in this section.
- **admin-backoffice-service, analytics-service, api-gateway, support-dispute-service and every driver service: no change.** No fabricated corporate order, contract, entitlement, scoring, export or operator-review integration was added.

**Disclosed dependencies, not worked around.**

- **Corporate commerce does not exist on this platform.** No service publishes corporate orders, invoices, credit or budgets, so Section 8G deliberately models none. `MEMBERSHIP_ROLE=APPROVER` is a CRM label describing participation and authorises nothing; when a corporate ordering domain exists it must define its own authority rather than reading meaning into this label.
- **Vendor commerce order facts remain absent** (inherited from Section 8B, unchanged here), so nothing derives corporate spend, volume or value, and no such field is stored or reported.

## Section 8H external-service audit

**One external change, and it was necessary.** Every other service is unchanged.

- **media-file-service: MINIMAL SECTION 8H CHANGE — DATA_EXPORT private CSV/JSONL artifact support.** Classification: **MINIMAL CHANGE REQUIRED**. Audit result was `PRIVATE_EXPORT_ARTIFACT_SUPPORT = PARTIAL`. The private object contract already provides exactly the controls a CRM export needs — HMAC service authentication, a caller allowlist, single-use request nonces, private objects with no public URL, content inspection, short-lived download grants, retention and deletion — and its MIME allowlist, caller list, size caps and TTLs are all configuration. What it could not do was accept the artifact: `validation.Detect` is signature-based only and refuses any content with no magic number, which is every CSV and JSONL file, and its closed upload-purpose vocabulary had no data-export purpose.

  The implemented capability is exactly `DATA_EXPORT`, `.csv` + `text/csv`, and `.jsonl` + `application/x-ndjson`, **PRIVATE ONLY**. The change is an **addition, not a relaxation**: `validation.ValidateExportContent` establishes the textual format by *inspecting and structurally parsing the bytes* — valid UTF-8, no NUL, no C0 control characters other than tab/CR/LF, no binary/executable signature, syntactically consistent CSV or one JSON object per non-empty JSONL line — reached only for that purpose. The declaration selects which claim to test; it never substitutes for the test, so a PDF, ELF binary, ZIP container, HTML/SVG document or shell script declared as `text/csv` is still refused. Exact extension/MIME pairing, the 20 MiB purpose ceiling, checksum verification, attachment disposition and nosniff apply to the new path. Signature validation for every existing media/document purpose is not relaxed and is provably unchanged (`TestDetect_StillRefusesTextForDocumentPurposes`, `TestDataExport_ExistingPurposesKeepTheirOldBehaviour`).

  Vendor uses the export id as Media's stable upload identity. A retry after a lost completion response reconciles the already-completed private object by checksum, byte count, purpose, visibility and canonical MIME; it never mints a second upload token or creates another logical artifact. Vendor also applies the export's 24-hour `retentionUntil`, and cancellation/expiry use Media's idempotent deletion contract.

  Deployment additionally needs `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS` to include `vendor-service` and `MEDIA_FILE_PRIVATE_ALLOWED_MIME_TYPES` to include `text/csv,application/x-ndjson`. Both are configuration.

  Final result: `PRIVATE_EXPORT_ARTIFACT_SUPPORT = AVAILABLE`.

- **customer-service: no change.** Intelligence reads the Section 8A projections that already exist; exports carry opaque relationship references and no contact detail.
- **payment-wallet-service: no change.** No score, dataset or column holds money. The RFM monetary dimension is a quintile of an existing projection, not a balance, and loyalty points remain non-monetary.
- **notification-service: no change.** Section 8H contacts nobody. A score is not permission and there is no path from one to a delivery.
- **support-dispute-service, verification-compliance-service, admin-backoffice-service, analytics-service, api-gateway and every driver service: no change.** No fabricated integration was added. In particular CRM intelligence stays Vendor-owned rather than moving into analytics-service; that service may later consume the aggregate outbox events, which carry counts and never a customer list.

**Disclosed dependencies, not worked around.**

- **No vendor commerce order publisher exists.** Every score is computed from Section 8A commercial projections, so a production deployment will legitimately report `NOT_AVAILABLE` for every relationship until authoritative order facts arrive. The section is built to be correct and honest in that state; it does not fill the gap with zeros, and it does not fabricate recency, frequency or spend.
- **No AI, ML or scoring service is integrated, and none is planned here.** Both models are versioned Go heuristics. A statistically validated probability model would be a different section with a different contract.

## Section 8I external-service audit and final dependency matrix

**No external service changed in Section 8I.** The only cross-service change in
the whole CRM programme remains Section 8H's minimal `media-file-service`
addition (the `DATA_EXPORT` purpose and its safe-text validation), documented
above. customer-service, notification-service, payment-wallet-service,
support-dispute-service, verification-compliance-service, analytics-service,
api-gateway and every driver service are unchanged by 8I.

### Final CRM authority matrix

| Authority | Owner | Vendor CRM's relationship to it |
|---|---|---|
| Canonical customer identity and contact details | customer-service | Projected by reference only; no email, phone, address or date of birth exists in any CRM table (`TestSection8I_NoCanonicalCustomerPIIInTheCRMSchema`) |
| Consent decisions | customer-service | Projected; fail-closed when absent, never inferred |
| Privacy and identity-merge instructions | customer-service | Applied through the Section 8A handler registry; never originated here |
| Vendor-local relationship, tags, segments, interactions, tasks, loyalty points, campaigns, corporate CRM, intelligence, export lifecycle | **vendor-service** | Owned outright |
| Message transport and destination resolution | notification-service | Handed an opaque customer reference; Vendor never resolves a destination |
| Money, settlement, refunds, payout, ledger | payment-wallet-service | No monetary column in CRM (`TestSection8I_NoMonetaryAuthorityInTheCRMSchema`); loyalty points are points |
| File bytes, private storage, content safety, retention | media-file-service | Vendor persists only an opaque `artifact_ref`, checksum, size and timestamps |
| Formal verification decisions | verification-compliance-service | Projected |
| Formal dispute lifecycle | support-dispute-service | Not represented in CRM |
| Authoritative commerce order facts | *future Order domain* | **NOT_PRESENT** — see below |

No cross-service database access exists in either direction.

### External dependency status

| Dependency | Status | Consequence today |
|---|---|---|
| `VENDOR_COMMERCE_ORDER_PUBLISHER` | **NOT_PRESENT** | Commercial projections stay `NOT_AVAILABLE`, so RFM and churn report `NOT_AVAILABLE`, commercial and score segment rules match nobody, and analytics report unavailable counts rather than zeros |
| `CUSTOMER_CONSENT_AUTHORITY` | **NOT_PRESENT** | Every channel is ineligible with `CONSENT_SOURCE_UNAVAILABLE`; campaigns can be prepared but nothing is sendable |
| `CUSTOMER_PRIVACY_EVENT_AUTHORITY` | **PARTIAL** | The ingress, handlers and fences are implemented and tested; no production producer publishes to them yet |
| `CUSTOMER_IDENTITY_MERGE_AUTHORITY` | **PARTIAL** | Same: merge is implemented and tested end to end, with no production producer |
| `PII_FREE_CUSTOMER_PRESENTATION` | **PARTIAL** | CRM stores an optional non-identifying display label only; a richer safe presentation needs customer-service |
| `NOTIFICATION_DELIVERY_EVENT_PRODUCER` | **NOT_PRESENT** | The secured receiver is implemented and dormant; no delivery is ever marked from a synthetic callback |
| `PRODUCTION_SMS_CHANNEL` | **NOT_PRESENT** | SMS deliveries are suppressed with `CHANNEL_UNAVAILABLE` rather than silently dropped |
| `PRIVATE_EXPORT_ARTIFACT_SUPPORT` | **AVAILABLE** | Delivered by the Section 8H media change; deployment must still allowlist `vendor-service` as a private caller |

None of these is a CRM code defect. Each fails closed, is documented, and no
production path fabricates the missing truth.


### Section 8 completion — dependency status (unchanged)

The Section 8 completion adds no external dependency. Loyalty tiers read the
Vendor-owned Section 8D ledger and nothing else, which is why they are active
today rather than code-ready-pending-a-producer.

The tables created by `000026`-`000028` support the remaining original Section 8
requirements anticipate the SAME producers already recorded as absent above —
a Vendor commerce Order authority (order lines, order type, corporate order
attribution), an RFQ authority, and a support fact producer. None of them
exists, none is fabricated; the CRM projections and read models remain
`NOT_AVAILABLE` until those authorities deliver facts.

**External services modified in the Section 8 completion: NONE.**

### Section 9A — dependency status

**External services modified in Section 9A: NONE.**

Section 9A is a Vendor-owned catalogue. It creates no monetary authority, no
customer identity, no logistics capability and no order surface, and it calls no
other service except one that already exists.

| Dependency | Status | Notes |
|---|---|---|
| `MEDIA_PRIVATE_OBJECT_OWNERSHIP_READ` | **AVAILABLE** | Item media reuses the Section 7 private ownership read (`GetObject`) unchanged. No Media contract change was needed or made; a deployment must simply keep `vendor-service` allowlisted as a private caller. With `VENDOR_MEDIA_URL`/`VENDOR_MEDIA_SECRET` unset, media attachment fails closed with `FOOD_MEDIA_UNAVAILABLE` rather than storing an unverified reference |

Future dependencies are recorded here rather than implemented:

| Future dependency | Needed by | Status |
|---|---|---|
| `ORDER_AUTHORITY` | A future Order domain that snapshots item, variant, modifier and price at order time | **PENDING_EXTERNAL_DOMAIN — ORDER AUTHORITY.** Not implemented, not stubbed, not fabricated. Section 9A only avoids designs that would make an immutable snapshot impossible: stable ids, an entity version on every aggregate, integer minor-unit prices, and archive-never-delete so a snapshotted reference stays resolvable |
| `INVENTORY_STOCK_AUTHORITY` | Section 9B's inventory-linked availability seam | **NOT_PRESENT.** No ingredient, stock, reservation or depletion concept exists in this section |
| `CATALOGUE_CONSUMPTION_CONTRACT` | Section 9C discovery and internal consumption | **IMPLEMENTED.** Five root-mounted `/internal/vendor/v1/food` routes use HMAC v2, caller allowlisting and nonce replay protection; no public frontend consumer is fabricated |

No requirement was invented for Customer Service, Wallet, Dispatch, Taxi,
Hauling, Notification, Verification, Support or Analytics. Nothing in this
section calls them, and no catalogue price is settlement.

### Section 9B dependency status

**External services modified in Section 9B: NONE.** The inventory integration
is a fail-closed seam only (`INVENTORY_AVAILABILITY_SUPPORT=NOT_PRESENT`);
there is no stock authority, callback, reservation or depletion model. Price
history, schedules, availability, profiles and catalogue revisions are owned
and persisted by Vendor Service. Section 9C exposes only the internal discovery
contract; a public customer consumer remains outside this section.
### Sections 10/11 — Cloud Kitchen and Pastry/Bakery

Vendor Service now owns Cloud Kitchen package/quantity-band and recurring-plan
configuration plus a first-class Pastry/Bakery catalogue and custom-cake quote
workflow. Both domains reserve through the shared production engine. Accepted
cake quotes produce an immutable Vendor snapshot and a durable
`PENDING_EXTERNAL_DOMAIN` conversion intent; they do not create Orders,
payments, Wallet movements, Inventory stock or delivery execution.

Closure migrations through `000041` add worker lease/replay state,
occurrence-to-job linkage, bounded pastry risk/logistics snapshot support,
immutable change-request decision history, scoped foreign keys, versioned
quantity-band history, immutable job source versions, optimistic occurrence
versions and durable operational alerts. Section 10/11 mutations reuse the
existing Vendor idempotency store; no parallel command-dedupe authority was
introduced. The external boundary remains unchanged:
`ORDER_AUTHORITY = NOT_PRESENT`, `CUSTOM_CAKE_ORDER_CONVERSION_SUPPORT =
PENDING_EXTERNAL_DOMAIN`, and no Inventory, Wallet, Dispatch, Taxi or Hauling
authority is introduced.

### Section 12A dependency status

**External services modified: NONE.** Vendor Service owns Shop catalogue and
commercial configuration. Existing Media ownership reads are reused for opaque
product media; Section 5 IAM, audit, idempotency and transactional outbox are
reused without creating parallel authorities.

| Dependency | Section 12A status |
|---|---|
| Media private object ownership | Existing optional integration; attachment fails closed when unavailable |
| Shop inventory authority | **IMPLEMENTED by Section 12B in Vendor Service** |
| Shop customer discovery/availability | **IMPLEMENTED by Section 12B as HMAC-v2 internal reads** |
| Order authority | **NOT PRESENT**; price resolution is configuration, not checkout |
| Wallet/payment | **NOT PRESENT** |
| Dispatch/Taxi/Hauling execution | **NOT PRESENT**; handling metadata is declarative only |

The Shop tables are repository-isolated from Food, Cloud Kitchen and Pastry.
A business may own all capabilities, but no catalogue row crosses those domain
queries merely because the `business_id` matches.

### Section 12B dependency status

**External services modified: NONE.** Inventory, reservations, discovery,
alerts and bulk processing are Vendor-owned and use the existing PostgreSQL,
IAM, idempotency, audit, outbox and HMAC-v2 foundations.

| Dependency | Section 12B status |
|---|---|
| Shop catalogue/commercial configuration | **AVAILABLE** in Section 12A; revisions are checked at bulk/discovery boundaries |
| PostgreSQL inventory authority | **IMPLEMENTED** locally; no Redis correctness dependency |
| Internal availability/reservation callers | **CONTRACT AVAILABLE** through allowlisted HMAC-v2 routes with nonce replay protection |
| Order authority | **NOT PRESENT**; caller source references do not become Orders |
| Wallet/payment | **NOT PRESENT** |
| Suppliers/purchasing | **NOT PRESENT**; supplier/source references remain opaque receipt evidence |
| Dispatch/Taxi/Hauling execution | **NOT PRESENT**; handling metadata remains declarative |

No other service schema, route, secret catalogue or deployment was changed by
Section 12B. Consumers must be allowlisted through deployment configuration
before calling the internal contract.
