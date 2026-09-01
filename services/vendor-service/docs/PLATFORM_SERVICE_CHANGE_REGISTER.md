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
| `ADMIN_VENDOR_REVIEW_SUPPORT` | **NOT_PRESENT** | `admin-backoffice-service` contains only scaffold READMEs and no review routes, persistence, or service-auth integration. Section 7 will not fake operator approval; manually gated cases remain blocked as `PENDING_EXTERNAL_DEPENDENCY â€” VENDOR ADMIN REVIEW`. |
| `NOTIFICATION_SUPPORT` | **PARTIAL** | `notification-service` has HMAC-protected push, email, websocket and in-app delivery, but no SMS implementation and no committed Vendor allowlist entry. Section 7 will emit durable domain events and will not claim delivery until deployment allowlisting exists. |

### Authorized minimal external changes for Section 7

- **verification-compliance-service**: additive Vendor verification cases, exact Section 7 status/dimension vocabulary, idempotent create/read/read-by-client-reference/admin decision routes, source-versioned history and outbox. Auth: existing HMAC v2 plus database nonce replay defense; caller allowlist narrowed by action. Migration: next sequential `000002`, including additive callback-delivery state on the existing outbox. A leased signed callback worker has bounded retry/dead-letter/replay; Vendor also polls by client reference until terminal. Rollback removes only the additive Vendor tables/columns/routes. Tests cover create/replay/read/decision/stale-version/auth/nonce/callback delivery/operator replay.
- **payment-wallet-service**: additive internal Vendor bank-onboarding resolve/register/read contract using existing Paystack and provider-bank persistence authority, returning only safe projection fields. Auth: HMAC v2 and explicit `vendor-service` caller check. Migration: none expected because existing provider bank records accept a namespaced provider type/id. Rollback: remove only the three internal routes/adapter. Feature flag: existing Wallet service availability plus Vendor-side integration flag. Tests: caller isolation, resolution, idempotent registration and masked response.
- **media-file-service**: no schema or route change; reuse the existing private object read contract. Deployment config must add `vendor-service` to `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS` and its secret to `MEDIA_FILE_SERVICE_SECRETS`. Vendor Service adds a fail-closed read client and object-state/ownership validation before accepting a document reference.
- **admin-backoffice-service**: **NONE**; implementing a full operator product exceeds the permitted minimal adapter and no generic review architecture exists.
- **notification-service**: no code/schema/route change; deployment must add `vendor-service=<secret>` to `NOTIFICATION_SERVICE_SECRETS` before delivery is available. No SMS is claimed.
- **api-gateway**: no change; its routing module is a scaffold and Vendor Service follows the repository's direct-service convention. Internal callbacks must never be exposed publicly.

Everything Vendor Service implementation requires outside `services/vendor-service`, tracked here as it is identified â€” not implemented speculatively ahead of the feature section that actually needs it, per the architecture task's own scope limits.

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

## 1. `payment-wallet-service` â€” allowlist `vendor-service` as an HMAC caller

- **Implementation section**: architecture foundation (identified now; activated when checkout/payments are implemented)
- **Affected path/module**: `internal/config/config.go`, `PAYMENT_WALLET_SERVICE_SECRETS` (and `PAYMENT_WALLET_FINANCE_ALLOWED_CALLERS` if the finance-private API is used)
- **Change required**: add `vendor-service=<secret>` to the allowlist env var
- **Why required**: `payment_intents.source_service` is already free-text with no schema constraint (confirmed by reading `migrations/001_wallet_ledger.sql`), so the only real gate is HMAC caller identity
- **Authority/boundary**: no change to `payment-wallet-service`'s ledger authority; Vendor Service remains a caller, never a competing ledger
- **New/changed migration**: none
- **New/changed route**: none â€” reuses existing `POST /internal/payment-intents` and related endpoints
- **New/changed event**: none
- **New/changed configuration**: `PAYMENT_WALLET_SERVICE_SECRETS` env var value (deployment config, not code)
- **New/changed feature flag**: none on `payment-wallet-service`'s side; gated on Vendor Service's own `PAYMENT_WALLET_COMMERCE_ENABLED`
- **Tests added**: none yet
- **Rollback consideration**: removing the allowlist entry immediately blocks Vendor Service calls; no data migration to reverse
- **Status**: **NOT_STARTED** (no code depends on it yet; documented so the first checkout-implementing session doesn't have to re-derive this)

## 2. `media-file-service` â€” allowlist `vendor-service` and add commerce media purposes

- **Affected path/module**: `MEDIA_FILE_SERVICE_SECRETS`/`MEDIA_FILE_SERVICE_TOKENS` config; `internal/features/uploads/usecases/upload_service.go`'s `isAllowedPurpose()` code allowlist
- **Change required**: (a) allowlist `vendor-service` as a caller identity (config-only); (b) add purposes such as `vendor_logo`, `branch_photo`, `menu_item_photo` to the code allowlist when a feature actually needs them
- **Why required**: `owner_service` is enforced at the app layer to equal the caller's own HMAC identity â€” this is a real code gate, not free text in practice, even though the column itself has no DB constraint
- **Authority/boundary**: media-file-service remains the sole media authority; Vendor Service stores only references
- **New/changed migration**: none
- **New/changed route**: none
- **New/changed configuration**: `MEDIA_FILE_SERVICE_SECRETS`/`MEDIA_FILE_SERVICE_TOKENS`
- **Status**: **NOT_STARTED**

## 3. `notification-service` â€” allowlist `vendor-service`

- **Affected path/module**: `NOTIFICATION_SERVICE_SECRETS`
- **Change required**: add `vendor-service=<secret>`
- **Why required**: notification requests already accept a caller-supplied `source_service` string; only the HMAC allowlist gates who may send
- **New/changed migration**: none
- **Status**: **NOT_STARTED** â€” Section 5's client side (`internal/features/auth/clients.OTPNotifier`) is fully built and wired into `cmd/main.go` (constructed whenever `VENDOR_NOTIFICATION_URL` is set), but the actual allowlist entry on `notification-service` itself has not been made, so real (non-debug) email OTP delivery will 403 until this config change lands. Confirmed during Section 5 testing (all tests run with `VENDOR_AUTH_OTP_DEBUG=true`, which skips real delivery entirely â€” see below).
- **Additional finding from Section 5 (not previously known)**: `shared/go/notifications`' channel set is exactly `{push, email, websocket, in_app}` â€” **there is no SMS channel anywhere in the platform**, and push/in-app both require an already-registered device/session that cannot exist pre-login. Phone-number OTP delivery therefore has **no real production channel today, for any service**, not just Vendor Service (confirmed customer-service's own `LoggingOTPSender` falls back to logging for phone too, in production without printing the raw code, and â€” separately â€” prints the raw code in `debug=true` mode, which Section 5's own rule 11 explicitly forbids and this service deliberately does not replicate). Vendor Service's `OTPNotifier.SendOTP` returns a clear `service_unavailable` error for phone identifiers rather than pretending to deliver. **This is a platform-wide product gap** (a real SMS provider integration on `notification-service`, e.g. Termii/Twilio), not something in scope for this service to build, and is recorded here as a new, previously-undocumented dependency for whichever team owns `notification-service` next.

## 4. `verification-compliance-service` â€” Vendor verification authority

- **Affected path/module**: `internal/features/vendors`, migration `000002`, configuration and composition root
- **Change implemented**: idempotent create/read/read-by-client-reference/admin-decision, source-versioned history, durable callback delivery, exact six-dimension vocabulary
- **Why required**: formal verification must remain outside Vendor Service
- **Authority/boundary**: verification-compliance-service remains the sole formal-verification-decision authority; Vendor Service never self-certifies a vendor as verified
- **New/changed migration**: `000002_vendor_verification_cases`
- **New/changed route**: four `/internal/vendor-cases/...` routes
- **New/changed event**: `vendor.verification.case_created`, `vendor.verification.status_changed`
- **Status**: **IMPLEMENTED AND VERIFIED**; deployment must configure mutual HMAC/callback credentials. `admin-backoffice-service` remains the missing operator product.

## 5. `support-dispute-service` â€” new vendor caller route group

- **Affected path/module**: `internal/features/support/http/routes.go` (new `/vendor` group, mirroring the existing 3 provider-app groups), `complainant_type` enum
- **Change required**: a new bearer secret (`SUPPORT_DISPUTE_VENDOR_TOKEN_SECRET` or similar) and a new `complainant_type` value
- **Why required**: today's route groups are customer + 3 provider apps only; there is no vendor caller class
- **New/changed migration**: yes (widen `complainant_type` enum/check)
- **New/changed route**: yes
- **Status**: **NOT_STARTED**

## 6. `driver-dispatch-delivery-service` / `driver-hauling-service` / (future) `driver-taxi-service` â€” the real OTP/QR handover credential

- **Change required**: a genuine one-time-code/QR challenge-response system for pickup and delivery custody confirmation, replacing today's evidence-capture-only proof flows
- **Why required**: confirmed platform-wide absence â€” see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md)
- **Authority/boundary**: this is a platform-wide logistics capability, not something Vendor Service is authorized to build a duplicate of; it should be built once, coordinated across the owning logistics services, most plausibly as an extension of dispatch's existing `ConfirmRecipient`/WP6.3 idempotency/fingerprint scaffolding
- **New/changed migration**: yes, on whichever service ends up owning it
- **New/changed route**: yes
- **Status**: **PENDING_EXTERNAL_DEPENDENCY** â€” this is the specification's own Case C outcome, not a Vendor Service implementation gap. Blocks only the final confirmed-handover step of platform-delivery fulfilment; does not block anything else in Vendor Service.

## 7. `driver-taxi-service` â€” future GOODS_DELIVERY capability

- **Change required**: an entirely new feature on a currently-empty service, distinct from passenger rides
- **Why required**: per CLAUDE.md's "Taxi Important Boundary" and the specification, this architecture phase records the future boundary only
- **Status**: **NOT_STARTED** â€” no implementation attempted, per explicit task instruction

## 8. `admin-backoffice-service` / `analytics-service` â€” routes/consumers once those services exist

- **Change required**: vendor-moderation routes on admin-backoffice-service; event consumption on analytics-service
- **Why required**: both are currently pure scaffolds with zero routes/migrations â€” there is nothing to integrate with yet
- **Status**: **NOT_STARTED**, blocked on those services' own foundational build-out (not a Vendor Service dependency to resolve)

## 9. `api-gateway` â€” future routing migration

- **Change required**: none required now. Vendor Service exposes its own direct routes (matching the existing customer/hauling/dispatch precedent) until a generic gateway routing layer exists (`api-gateway`'s `routing/`/`service_health/` are currently empty scaffolds)
- **Status**: **NOT_STARTED**, and not urgent â€” no Vendor Service code should depend on being reached through a gateway specifically

## 10. `CLAUDE.md` â€” documentation gaps found during this audit (partially closed this session)

- **Change required**: `fleet-partner-service` was not listed anywhere in CLAUDE.md's own service documentation despite being the platform's largest, most mature service; `infra/docker-compose.yml` and multiple `scripts/*bootstrap*.sh` files are referenced as if committed but do not exist anywhere in git history
- **Why required**: accuracy for future sessions
- **What was actually done**: this session added a one-line `fleet-partner-service` entry and a `vendor-service` entry to CLAUDE.md's "Backend Services"/"Default local databases"/"Important local ports" lists, and disclosed the docker-compose/bootstrap-script gap explicitly in [architecture/00_REPOSITORY_AUDIT_BASELINE.md](architecture/00_REPOSITORY_AUDIT_BASELINE.md) rather than fabricating those files
- **Status**: **PARTIAL** â€” full fleet-partner-service documentation and recreating the platform's local-dev tooling are both out of scope for this task

## Section 8A â€” CRM Foundation, Customer 360, Consent and Privacy

**No external service was modified for Section 8A.** Every entry below is either NO CHANGE or a recorded pending dependency. The repository was audited before any code was written, and where an authority does not exist the integration was left dormant rather than invented.

### 11. `customer-service` â€” NO CHANGE (three pending authority contracts recorded)

- **Audited on 2026-08-11.** Features present: `auth`, `identity`, `notifications`, `preferences` (directory scaffolding only, no Go files), `profile`, `request_history`, `saved_locations`. Migrations `001`â€“`005`.
- **Consent**: no consent concept exists anywhere in the service â€” no table, no field, no route, no event. **PENDING_EXTERNAL_DEPENDENCY â€” CUSTOMER MARKETING CONSENT AUTHORITY.** Vendor CRM defines the projection, the source-version rules and the `CUSTOMER_CONSENT_CHANGED` contract, and keeps ingestion disabled behind `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED`. Until an authority exists every decision is UNKNOWN and marketing is ineligible.
- **Privacy**: no delete-request, restriction, redaction or suppression concept exists. **PENDING_EXTERNAL_DEPENDENCY â€” CUSTOMER PRIVACY AUTHORITY.** The `CUSTOMER_PRIVACY_CHANGED` contract and the full redaction engine are implemented and tested; nothing currently instructs them.
- **Identity merge**: no identity-link, guest-upgrade or account-merge concept exists. **PENDING_EXTERNAL_DEPENDENCY â€” CUSTOMER IDENTITY LINK AUTHORITY.**
- **Presentation**: `GET /internal/customers/:id` (HMAC service-auth) exists and returns `{id, name, phone, email, status}`. It is **not** usable as a CRM display source: it carries canonical contact data, and storing any of it would make Vendor Service a second customer master. Vendor CRM therefore leaves `safe_display_label` permanently null. **PENDING_EXTERNAL_DEPENDENCY â€” PII-FREE CUSTOMER PRESENTATION CONTRACT** (a display-only projection with no contact fields) if vendor staff ever need a human-readable label.
- **Why no change was made now**: none of the four is required for Section 8A to be correct and deployable. Adding speculative consent/privacy tables to customer-service would be designing another service's domain from inside this one, which the service-boundary rules forbid.
- **Status**: **NO CHANGE** (four dependencies recorded above).

### 12. Vendor commerce order publisher â€” DOES NOT EXIST

- `services/vendor-service/internal/features/orders` is still a reserved `doc.go` boundary; no service publishes vendor commerce order events.
- Vendor CRM defines the `CRM_ORDER_FACT_CHANGED` contract, implements the fact projection, dedupe, source-version handling, refund-safe recomputation and rebuild path, and tests all of it against fixtures. The allowlisted caller is named (`vendor-order-service`) and the integration stays disabled behind `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED`.
- **No publisher was fabricated.** Until the order section exists, every CRM commercial figure honestly reports `NOT_AVAILABLE`.
- **Status**: **PENDING_EXTERNAL_DEPENDENCY â€” VENDOR COMMERCE ORDER AUTHORITY.**

### 13. `notification-service` â€” NO CHANGE

- Audited for Section 8A because later CRM packages (8E/8F campaigns) will depend on it. Channels are exactly push/email/websocket/in_app; there is no SMS sender.
- `notification_preferences` (`recipient_type`, `recipient_id`, `channel`, `enabled boolean DEFAULT true`) is a **delivery preference toggle owned by the delivery service**, not a marketing-consent authority: it has no purpose dimension, no source version, no consent semantics, and defaults to opt-in. Vendor CRM deliberately does **not** map it to consent â€” treating `enabled=true` as marketing ALLOW would manufacture permission nobody granted.
- Section 8A sends nothing and requires no notification change.
- **Note for later packages**: SMS *consent* is representable in the CRM projection even though SMS *delivery* does not exist. A future campaign package must not dispatch on a channel with no sender.
- **Status**: **NO CHANGE.**

### 14. `api-gateway` â€” NO CHANGE

- The six public CRM routes follow the existing direct-exposure precedent and need no gateway work.
- `POST /internal/vendor/v1/crm/events` is root-mounted and **must never be gateway-exposed**: it is the authority write path into consent, privacy and identity projections. It is not nested under `/api/v1/vendor`, repeating the Section 7 callback correction, and `internal/section8acontract` asserts that structurally.
- **Status**: **NO CHANGE.**

### 15. `payment-wallet-service` â€” NO CHANGE

- Section 8A holds no financial authority. CRM commercial figures are a read-model projection of order facts, not a balance, receivable, settlement or credit line. Nothing was read from or written to wallet.
- **Status**: **NO CHANGE.**

### 16. `support-dispute-service` â€” NO CHANGE

- Complaints, disputes and adjudication remain outside CRM authority. Section 8A implements no interactions (that is 8C), so nothing was needed.
- **Status**: **NO CHANGE.**

### 17. `media-file-service` â€” NO CHANGE

- Section 8A stores no media and performs no export (exports are 8H).
- **Status**: **NO CHANGE.**

### 18. `analytics-service` / `admin-backoffice-service` â€” NO CHANGE

- Both remain scaffolds. Section 8A emits its CRM outbox events durably (`vendor.crm.profile.created`, `.archived`, `.restored`, `.linked`, `.privacy_state.changed`); with no configured publisher they accumulate as `PENDING` and are never silently dropped, matching the Section 5/6/7 precedent.
- **Status**: **NO CHANGE.**

---

## Section 8B â€” Tags & Advanced Segmentation

Every platform service was reviewed for a required change. **All eighteen entries above remain NO CHANGE, and Section 8B adds none of its own.**

The targeted 8B correctness pass is also Vendor-Service-local: it adds no route, event, migration, credential or cross-service contract. The materialization fence, dependency invalidation, immediate privacy membership cleanup and Customer 360 scope correction all operate over existing Vendor-owned rows. Consent decisions remain owned by the existing authority and are never rewritten by restriction or marketing suppression.

That is not an oversight â€” it follows from what the section is. Segmentation reasons entirely over data Vendor Service already owns: its own tags, and the Section 8A relationship, commercial, consent and privacy projections. It consumes no external authority, so it needs no new caller allowlist, no new signed callback and no new integration credential. It contacts nobody, so it needs nothing from `notification-service`.

The reviews worth recording explicitly:

### `customer-service` â€” NO CHANGE

- Section 8B stores no canonical customer identity and adds no field that could hold one. Its field registry is a closed allowlist, and there is deliberately no `customer.email`, `customer.phone`, `customer.address` or `customer.date_of_birth` key in it â€” a rule cannot reference what the registry does not publish, and a test asserts those keys are absent rather than merely filtered.
- No new internal endpoint was needed on customer-service, and none was added.
- **Status**: **NO CHANGE.**

### `notification-service` â€” NO CHANGE

- This is the one worth stating plainly: Section 8B builds audiences and **sends nothing**. There is no route, permission, worker or event in this section that contacts a customer, and the contract test rejects a campaign- or send-shaped route structurally.
- Marketing eligibility remains a Section 8A derivation. A segment may *read* eligibility as a rule field, but segment membership is never permission â€” whatever section eventually sends must re-check consent and privacy at send time, against the state then rather than the state when the audience was built.
- The existing platform-wide gap (no SMS channel anywhere) is unchanged and unaffected: SMS consent stays representable and honoured if a sender ever appears.
- **Status**: **NO CHANGE.**

### `payment-wallet-service` â€” NO CHANGE

- Section 8B holds no financial authority. Money appears only as an integer count of minor units read from the Section 8A commercial projection, compared against a threshold. Nothing is charged, settled or reconciled, and no wallet endpoint is called.
- Cross-currency comparison is impossible by construction: Section 8A refuses order facts denominated differently from the business currency, so a rule's currency is validated but never converted.
- **Status**: **NO CHANGE.**

### `media-file-service` â€” NO CHANGE

- Section 8B stores no media and performs no export. Audience export belongs to Section 8H and will need its own review, its own step-up requirement and probably its own media integration.
- **Status**: **NO CHANGE.**

### `verification-compliance-service` â€” NO CHANGE

- Segmentation makes no verification decision and reads no verification state. No field in the registry exposes one.
- **Status**: **NO CHANGE.**

### `support-dispute-service` â€” NO CHANGE

- Complaints and disputes remain outside CRM authority. Section 8B implements no interactions (that is 8C).
- **Status**: **NO CHANGE.**

### `api-gateway` â€” NO CHANGE

- The 21 public routes follow the existing direct-exposure precedent established by Sections 5-8A and need no gateway work.
- Section 8B registers **no internal route at all**, so there is nothing here that must be kept off the gateway. That is the strongest form of the Section 7 callback lesson: the safest internal surface is the one that does not exist.
- **Status**: **NO CHANGE.**

### `analytics-service` / `admin-backoffice-service` â€” NO CHANGE

- Both remain scaffolds. Section 8B appends its domain events to the same transactional outbox as Sections 5-8A: `vendor.crm.tag.created`, `.tag.updated`, `.tag.archived`, `vendor.crm.segment.created`, `.updated`, `.activated`, `.archived`, `.materialized`. With no configured publisher they accumulate as `PENDING` and are never silently dropped.
- The `materialized` event deliberately carries the audience's **identity** â€” segment reference, definition version, definition hash, generation, member count, evaluated-at â€” and never its contents. A member list on an event bus would be a customer export nobody asked for.
- **Status**: **NO CHANGE.**

### Future reference protection (recorded now, acted on later)

`internal/features/crm.GenerationRetainer` is a port that decides whether a superseded membership generation may be cleaned up. Section 8B always answers "yes" because nothing references a past generation yet. Section 8E should implement it so a campaign can pin the exact generation it launched against, and so cleanup can never delete an audience a campaign is still reconciling against. This is a Vendor Service-internal seam, not a cross-service change.

## Section 8C review â€” all external services NO CHANGE

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

## Section 8D review â€” all external services NO CHANGE

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

## Section 8E review â€” all external services NO CHANGE

| Service | Status | Section 8E boundary |
|---|---|---|
| `notification-service` | **NO CHANGE** | This is the boundary that mattered most. Section 8E prepares audiences and delivers nothing: no send endpoint, no scheduler, no template, no provider client, no callback handler, no delivery worker and no `scheduled_at`/`sent_at`/`delivered_at` column anywhere. It emits no `campaign.sent`, `notification.sent`, `sms.sent` or `email.sent` event. A migration test rejects a delivery column structurally, a contract test rejects a delivery path or schema, and an HTTP test asserts `POST .../send`, `/schedule`, `/deliver` and `/dispatch` all 404. The boundary fails the build rather than drifting. |
| `customer-service` | **NO CHANGE** | Campaign targets reference vendor-local CRM profile ids only. No canonical PII is read, stored or returned, and consent and privacy remain projections of that service's authority, consumed through the accepted Section 8A ingress. Section 8E adds no new consumption path. |
| `payment-wallet-service` | **NO CHANGE** | Campaigns hold no currency, amount, wallet reference or settlement. Nothing here is monetary. |
| `media-file-service` | **NO CHANGE** | No campaign creative, asset or attachment exists in Section 8E; there is no message body to attach anything to. |
| `support-dispute-service` | **NO CHANGE** | Service-recovery suppression uses the Vendor-local Section 8C signal only â€” an `OPEN`/`IN_PROGRESS` task linked to a `SERVICE_FOLLOW_UP` interaction. No dispute state is read, requested or inferred, and none is fabricated. `EXTERNAL_SERVICE_RECOVERY_SUPPORT = NOT_PRESENT`. |
| `verification-compliance-service` | **NO CHANGE** | Untouched; campaigns have no verification dimension. |
| `analytics-service` | **NO CHANGE** | Campaign experiment cohorts are recorded locally for a future analysis that does not exist yet. No read model, export or analytics contract was created. |
| `api-gateway` | **NO CHANGE** | Ten public routes follow the existing Vendor exposure; Section 8E adds zero internal routes. |

No repository code outside `services/vendor-service` is required for Section 8E.

**Deferred integration dependencies, not fabricated.**

- **Delivery (Section 8F).** The whole execution half â€” scheduling, notification handoff, send-time consent revalidation, send-time frequency enforcement against real delivery facts, send-time quiet-hours deferral, provider callbacks and delivery history â€” is deliberately absent. Section 8E's contribution to it is the mandatory recheck flags every prepared channel and snapshot carries, which are pinned true so 8F cannot accidentally treat `PREPARED` as authorisation.
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

- **media-file-service: MINIMAL SECTION 8H CHANGE â€” DATA_EXPORT private CSV/JSONL artifact support.** Classification: **MINIMAL CHANGE REQUIRED**. Audit result was `PRIVATE_EXPORT_ARTIFACT_SUPPORT = PARTIAL`. The private object contract already provides exactly the controls a CRM export needs â€” HMAC service authentication, a caller allowlist, single-use request nonces, private objects with no public URL, content inspection, short-lived download grants, retention and deletion â€” and its MIME allowlist, caller list, size caps and TTLs are all configuration. What it could not do was accept the artifact: `validation.Detect` is signature-based only and refuses any content with no magic number, which is every CSV and JSONL file, and its closed upload-purpose vocabulary had no data-export purpose.

  The implemented capability is exactly `DATA_EXPORT`, `.csv` + `text/csv`, and `.jsonl` + `application/x-ndjson`, **PRIVATE ONLY**. The change is an **addition, not a relaxation**: `validation.ValidateExportContent` establishes the textual format by *inspecting and structurally parsing the bytes* â€” valid UTF-8, no NUL, no C0 control characters other than tab/CR/LF, no binary/executable signature, syntactically consistent CSV or one JSON object per non-empty JSONL line â€” reached only for that purpose. The declaration selects which claim to test; it never substitutes for the test, so a PDF, ELF binary, ZIP container, HTML/SVG document or shell script declared as `text/csv` is still refused. Exact extension/MIME pairing, the 20 MiB purpose ceiling, checksum verification, attachment disposition and nosniff apply to the new path. Signature validation for every existing media/document purpose is not relaxed and is provably unchanged (`TestDetect_StillRefusesTextForDocumentPurposes`, `TestDataExport_ExistingPurposesKeepTheirOldBehaviour`).

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
| Authoritative commerce order facts | *future Order domain* | **NOT_PRESENT** â€” see below |

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


### Section 8 completion â€” dependency status (unchanged)

The Section 8 completion adds no external dependency. Loyalty tiers read the
Vendor-owned Section 8D ledger and nothing else, which is why they are active
today rather than code-ready-pending-a-producer.

The tables created by `000026`-`000028` support the remaining original Section 8
requirements anticipate the SAME producers already recorded as absent above â€”
a Vendor commerce Order authority (order lines, order type, corporate order
attribution), an RFQ authority, and a support fact producer. None of them
exists, none is fabricated; the CRM projections and read models remain
`NOT_AVAILABLE` until those authorities deliver facts.

**External services modified in the Section 8 completion: NONE.**

### Section 9A â€” dependency status

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
| `ORDER_AUTHORITY` | A future Order domain that snapshots item, variant, modifier and price at order time | **PENDING_EXTERNAL_DOMAIN â€” ORDER AUTHORITY.** Not implemented, not stubbed, not fabricated. Section 9A only avoids designs that would make an immutable snapshot impossible: stable ids, an entity version on every aggregate, integer minor-unit prices, and archive-never-delete so a snapshotted reference stays resolvable |
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
### Sections 10/11 â€” Cloud Kitchen and Pastry/Bakery

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

## Section 13 â€” Catalogue, Taxonomy and Search

Section 13 changed **no other service's code, schema, secret catalogue or
deployment**. What it adds to the platform is one new capability that other
services may consume, and two new deployment configuration keys on
vendor-service itself.

### New platform capability owned by Vendor Service

| Capability | Owner | Consumed how |
|---|---|---|
| Canonical Food/Shop taxonomy | vendor-service | Root-mounted HMAC-v2 internal API, `/internal/vendor/v1/taxonomy/...` |
| Food and Shop discovery/search | vendor-service | Root-mounted HMAC-v2 internal API, `/internal/vendor/v1/search/...` |
| Search projection generation administration | vendor-service | Root-mounted HMAC-v2 internal API, `/internal/vendor/v1/search/projections/...` |

### Deployment configuration required before either surface is usable

Both internal surfaces are registered only when their flag is on, and each
requires its **own** caller allowlist. The allowlists are separate on purpose: a
service cleared to run a discovery query is not thereby cleared to publish the
canonical vocabulary for every vendor on the platform.

| Key | Purpose |
|---|---|
| `VENDOR_TAXONOMY_ENABLED` | Master switch for both taxonomy surfaces |
| `VENDOR_TAXONOMY_WRITES_ENABLED` | Additionally gates taxonomy mutations |
| `VENDOR_TAXONOMY_INTERNAL_CALLERS` | Allowlist for platform taxonomy administration |
| `VENDOR_SEARCH_ENABLED` | Master switch for internal search |
| `VENDOR_SEARCH_ADMIN_ENABLED` | Additionally gates ranking and projection administration |
| `VENDOR_SEARCH_INTERNAL_CALLERS` | Allowlist for internal search and administration |

Configuration validation refuses to start the service with a surface enabled and
its allowlist empty. That combination is safe but silent â€” it looks enabled,
answers 403 to everyone, and the misconfiguration only surfaces as a mysterious
outage in whichever service was meant to consume it.

Each caller also needs an entry in the existing `VENDOR_SERVICE_SECRETS` HMAC
secret catalogue. Holding a configured secret proves identity; it does not
confer authority â€” the allowlist decides that separately.

### Ownership boundaries this section establishes

| Boundary | Rule |
|---|---|
| Canonical taxonomy | vendor-service is the sole authority. No other service defines, versions or publishes it. |
| Food catalogue truth | Section 9 (`food`) remains authoritative. Search is a derived read model. |
| Shop catalogue and inventory truth | Section 12 (`shop`) remains authoritative. A search result is not a stock reservation. |
| Redis | Cache only. Never authoritative for catalogue, availability, pricing, verification or checkout. |
| Long-term behavioural analytics | `analytics-service` remains authoritative. Vendor Service stores no customer search identity or history. |
| Orders, checkout, payment, logistics execution | Unchanged; Section 13 touches none of them. |

### Event boundaries

Section 13 taxonomy commands append domain events to vendor-service's existing
transactional outbox, in the same transaction as the state change. No new event
destination, transport or consumer contract was introduced, and no other service
is expected to consume a Section 13 event yet.

Bounded operational search events (`search.performed`, `search.zero_results`,
`search.result_selected`) are permitted with non-sensitive payloads. They are
operational telemetry, not an analytics warehouse: no per-customer row is
stored, and long-term behavioural analytics remains out of scope for this
service.

### Exclusions

Section 13 introduced no AI recommendation engine, personalized ML ranking,
vector embeddings, pgvector, semantic LLM search, OpenSearch or Elasticsearch
deployment, advertising billing, campaign authority, order creation, checkout
authority, payment/wallet execution, logistics execution, customer PII
profiling, or dark-store search ownership. No Section 13 migration enables a
PostgreSQL extension. An automated scan over Section 13's own source and
migrations enforces this
(`internal/section13security/exclusions_test.go`).


## Section 14 â€” Generic Inventory

Section 14 required **no change to any other platform service**, and none was
made. This entry records that deliberately, so a future reader does not go
looking for an unrecorded dependency.

- No new cross-service HMAC caller identity was introduced. The internal
  Inventory routes reuse the existing `serviceauth` verifier, the existing
  nonce store and an explicit caller allowlist configured on vendor-service
  itself.
- No notification, wallet, media, verification, admin or analytics contract was
  touched.
- No migration was added to any other service. Section 14 owns migrations
  `000060`-`000066` inside vendor-service only.
- No API gateway route was added. Section 14's public routes hang off the
  existing `/api/v1/vendor` base, and its internal routes are root-mounted and
  deliberately absent from the gateway.

### Configuration a deployment must supply

- `VENDOR_INVENTORY_INTERNAL_CALLERS` â€” comma-separated allowlist of service
  names permitted to call the internal Inventory routes. This fails closed: an
  empty or unset value grants no service any Inventory authority, which is the
  correct default for an environment that has no internal consumer yet.
- `VENDOR_INVENTORY_ENABLED` and `VENDOR_INVENTORY_WRITES_ENABLED` both default
  to `false`, per the service's existing fail-closed flag convention. Reads
  require the first; every mutating route requires both.

### Still open from earlier sections

Section 14 did not resolve, and does not depend on, the previously recorded
platform gaps: notification-service still has no SMS channel, and the
notification-service allowlist entry for `vendor-service` remains outstanding
from Section 5. Neither affects Inventory, which emits no notifications.


## Section 15 - Suppliers, Purchasing and Goods Receiving

Section 15 required **no change to any other platform service**, and none was
made. In particular it deliberately does not integrate with
`payment-wallet-service`: receiving stock records commercial cost and settles
nothing.

- No new cross-service HMAC identity. Section 15 registers no internal route at
  all, because no other service drives purchasing.
- No notification, media, verification, admin or analytics contract touched.
- No migration added to any other service. Section 15 owns `000067`-`000070`
  inside vendor-service only.
- No API gateway route added.

### Configuration a deployment must supply

- `VENDOR_PURCHASING_ENABLED` gates the whole surface, including both durable
  workers, and defaults to `false` per the service's fail-closed convention.

### The wallet boundary, recorded deliberately

If a future section adds supplier payment, it must be a separate authority that
reads purchasing cost evidence. Purchasing must not gain the ability to move
money, and `TestSection15D_NoWalletMutationOnReceipt` fails if it does.

<!-- SECTION 16B RECIPES -->
## Vendor Service — Section 16B

Added recipe/BOM production-definition authority.

No new Inventory authority was created.

No production batch or physical stock-consumption authority was created.

No COGS or Wallet/accounting authority was introduced.

Ingredient substitution is explicitly deferred until dietary/allergen compatibility can be enforced safely.

<!-- SECTION16C_IMPLEMENTATION -->
## Section 16C platform/service change

Vendor Service now owns production-batch orchestration.

No service boundary changed:

- Inventory authority remains inside Vendor Section 14.
- Existing Sections 10/11 production capacity/jobs are reused.
- Food/Pastry/Cloud catalogue authority is unchanged.
- Purchasing and Wallet are unchanged.
- No external service contract was introduced.

## Section 16D - operational cost

No change to any other platform service is required by Section 16D.

Section 16D reads Section 15's `vendor_purchase_cost_history` through an
in-service port. It calls no external service, requires no new credential, and
deliberately has no Wallet, payment-wallet, accounting, ledger or settlement
integration of any kind - cost here is operational analytics, not accounting.

## Section 17B - checkout validation and Inventory reservation

**No other platform service changes, and none is newly required.**

Section 17B is entirely internal to Vendor Service. It calls no other service:
not `payment-wallet-service`, not `notification-service`, not
`media-file-service`, not dispatch, taxi or hauling. It moves no money, sends
no message, creates no logistics mission and publishes no cross-service event.
No new HMAC caller identity needs allowlisting anywhere, and no other service
needs a migration, a route or a credential on Section 17B's account.

The only external identity it consumes is one Section 17A already established:
the **Customer Service access token**, verified with
`VENDOR_CUSTOMER_ACCESS_TOKEN_SECRET`. Section 17B adds no second consumer of
it and no new customer-facing scope.

### Vendor Service configuration this section adds

| Variable | Default | Notes |
|---|---|---|
| `COMMERCE_CHECKOUT_ENABLED` | `false` | Master switch. With `COMMERCE_SHOP_ENABLED=true` it additionally requires `VENDOR_INVENTORY_ENABLED` and `VENDOR_INVENTORY_WRITES_ENABLED`, because a Shop checkout holds real physical stock through Section 14. |
| `COMMERCE_CHECKOUT_RECOVERY_ENABLED` | `false` | The background sweep that releases lapsed and orphaned holds. |
| `COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS` | `600` | Required, `[60, 86400]`. Nothing in the code path assumes a value. |
| `COMMERCE_CHECKOUT_STRANDED_GRACE_SECONDS` | `120` | Must be positive and shorter than the TTL. |
| `COMMERCE_CHECKOUT_RECOVERY_INTERVAL_SECONDS` | `30` | `[5, 3600]`. |

`Config.Validate` refuses every one of those constraints at startup rather
than degrading to a checkout that silently holds stock forever or one that
answers only refusals.

### One deployment prerequisite that is not a flag

Section 14 is the stock authority, and Section 17B reads and reserves through
it alone. The Section 12B Shop inventory ledger is still separate during the
compatibility bridge, so a Shop variant whose stock lives only in the Section
12B tables will fail checkout - correctly, fail-closed, but visibly. **Finish
the Section 14C compatibility backfill for a branch before enabling
`COMMERCE_CHECKOUT_ENABLED` for it.**

### Reservations this section creates in Section 14

Checkout holds stock as `source_type = 'CHECKOUT'` with the checkout group id
as `source_ref`. Anything operating on Section 14 reservations - dashboards,
alerts, manual releases, the reservation-expiry worker - will now see these
alongside `ORDER`, `RFQ`, `MANUAL` and `INTERNAL` holds, and must not assume a
held reservation belongs to an order.


<!-- SECTION17A_CUSTOMER_CART -->
## Section 17A - Platform Service Change Register

Vendor Service owns the customer cart and exposes 12
`/api/v1/commerce/carts...` operations.

Customer Service provides an authenticated internal saved-location read used to
create immutable cart delivery-address snapshots.

Section 17A introduces no Wallet, Dispatch, Hauling or Taxi pricing/execution
authority and creates no ParentOrder or VendorOrder.


## Section 17C - Fulfilment planning and logistics integration

The delivery-policy extension in Vendor migration `000096_delivery_policy_hybrid_execution`
adds branch-scoped PLATFORM/VENDOR/HYBRID policy evidence, bounded fallback
and custody/replan state. It reuses the existing Dispatch and Hauling quote
contracts; no logistics-service schema, provider-matching, transfer, or return
authority was fabricated here.

Section 17C is the first Vendor Service section that required real code changes
in another service. Both were additive and both are registered here.

### driver-dispatch-delivery-service - DONE

**Why.** The service had no authoritative quote. Its only pricing surface was a
public, unauthenticated `POST /api/v1/customer/dispatch/bookings/estimate` that
returns a stateless fare with no reference, no expiry and no serviceability;
its availability endpoint requires a customer bearer token Vendor Service does
not have and must not forge. Section 17C's "authoritative logistics quote" and
"serviceability" gates cannot be met against either.

**Added.**
- Migration `000040_commerce_delivery_quotes` - the persisted quote, with CHECK
  constraints that refuse a fee on an unserviceable answer.
- `internal/features/commercequote` - the service, repository and handler.
- `POST /api/v1/internal/commerce/quotes`, behind the existing
  `authority.ServiceBoundary` gates (HMAC v2, caller allowlist, single-use
  nonce). It gets its OWN verifier trusting exactly one caller, so the vendor
  secret opens nothing else even if another surface's allowlist is later
  widened by mistake.
- Config `VENDOR_SERVICE_SECRET` and `COMMERCE_QUOTE_TTL_SECONDS`. The route is
  registered only when the secret is set.

**Not changed.** Pricing still comes from `shared/go/dispatchpricing`, the same
formula the customer and partner surfaces use - there is no commerce-specific
fare. No booking is created and no rider is reserved: a quote is an answer, not
a commitment.

### driver-hauling-service - DONE

**Why.** Same gap, and hauling owns both VAN and TRUCK, so it is the only
authority that can quote either.

**Added.**
- Migration `012_commerce_delivery_quotes` - as dispatch, plus the truck type
  the service would actually dispatch, so an operator can see why a quote and a
  match disagreed.
- `internal/features/commercequote`.
- `POST /api/v1/hauling/internal/commerce/quotes`.
- `fleetauthority.BoundaryWithDB` - the existing three gates, exposed for a
  surface with no fleetauthority handler of its own. Extracted rather than
  duplicated so replay protection has exactly one implementation in the
  service; a second hand-rolled nonce insert is how one of them ends up subtly
  weaker.
- Config `HAULING_COMMERCE_QUOTE_TTL_SECONDS`. The route is registered only
  when `HAULING_SERVICE_SECRETS` contains a `vendor-service` entry.

**Not changed.** Pricing is hauling's own existing fare model. The quote's
eligibility filter deliberately mirrors the matcher's own (radius, truck
capacity, truck type, proximity-only fallback when a truck cannot be resolved),
so a quote is never issued that the matcher would refuse to fill.

### Section 30 Truck / Hauling Commerce integration - DONE

**External service changes: YES, in the existing Hauling authority only.**
Vendor Service now sends committed VAN/TRUCK fulfilment-plan evidence to
Hauling rather than treating the Hauling mission as a legacy single-pickup
booking. No Van service, duplicate provider marketplace, duplicate OTP/QR
domain or duplicate Wallet flow was created.

**Added/extended.**
- Hauling migration `014_commerce_mission_execution_controls` adds
  `operation_ref`, semantic `request_fingerprint`, plan binding, pickup stops,
  Parcel/manifest metadata, scheduled pickup-window metadata, source version
  and `cancelled` mission state support.
- `shared/go/commercequote` now accepts multi-pickup stop evidence for
  VAN/TRUCK route pricing. Hauling prices the total route across all pickup
  legs and the dropoff with its existing fare model.
- `shared/go/commercemission` now carries operation refs, plan, stops, Parcels,
  manifest/version, handling, vehicle requirement, read-back and cancel
  commands.
- Hauling adds HMAC-protected internal operation read-back and pre-custody
  cancellation routes:
  `GET /api/v1/hauling/internal/commerce/missions/operations/{operationRef}`
  and
  `POST /api/v1/hauling/internal/commerce/missions/operations/{operationRef}/cancel`.
- Vendor's Hauling adapter performs read-back after unknown create/cancel
  outcomes and rejects same-operation/different-fingerprint retries through
  the Hauling authority.
- Config: `HAULING_COMMERCE_INTEGRATION_ENABLED` and
  `HAULING_COMMERCE_INTEGRATION_KILL_SWITCH`.

**Not changed.** Hauling still creates the canonical `haulage_bookings` row,
starts existing provider matching, uses existing cargo pickup/delivery handover
authority, and never receives or controls customer/Vendor financial allocations.
Wallet remains the only ledger and settlement authority.

### shared/go/commercequote - NEW

One contract for three services, for the same reason `shared/go/notifications`
exists: the caller and every implementing service must agree exactly, and three
private copies of a wire shape are three places for it to drift. It is a schema
and a client, never a pricing engine.

The client verifies the authority's answer rather than trusting it, because
Vendor Service is the party that would present a bad answer to a customer as a
delivery fee.

### driver-taxi-service - NOT STARTED, and deliberately not required

Section 17C works without taxi. The shared contract has no CAR class, the
class-to-service mapping refuses one, and no taxi client is wired. A future
GOODS_DELIVERY capability would implement the same
`POST <api-base>/internal/commerce/quotes` contract; until then there is
nothing to call and nothing pretends otherwise.

### Deployment prerequisites

| Service | Change | Required for |
|---|---|---|
| driver-dispatch-delivery-service | set `VENDOR_SERVICE_SECRET`, apply migration 000040 | motorcycle quotes |
| driver-hauling-service | add `vendor-service=<secret>` to `HAULING_SERVICE_SECRETS`, apply migration 012 | van/truck quotes |
| vendor-service | `VENDOR_DISPATCH_URL`/`VENDOR_HAULING_URL` as BARE ORIGINS with matching secrets | either |

The bare-origin note is not decoration: the shared client appends each
service's own API base and path, and a full path pasted into the base URL
produces a doubled path that 404s every call. This platform has already made
that mistake once with `HAULING_PAYMENT_URL`.

## Section 17D - immutable checkout quote lock

**External service changes: NONE.** Section 17D consumes the authoritative
checkout and fulfilment evidence already held in Vendor Service and creates
only Vendor Service migration `000079`. No route, migration, event, allowlist,
secret or deployment change is required in Customer, Wallet, Dispatch,
Hauling, Taxi or any order service.

At Section 17D closure the production promotion authority was intentionally
`NoPromotions`. That historical state is superseded by the Promotions and
Delivery Funding capability below, which implements the existing narrow
`PromotionAuthority` port without replacing CheckoutQuote.

Payment/Wallet release or refund is only represented as a pure future replan
disposition. No Wallet call is wired and no external payment configuration is
activated. Section 17E remains not started.

## Section 17E - ParentOrder and multi-vendor VendorOrders

**External service changes: NONE.** Section 17E consumes the approved Section
17D quote already held in Vendor Service and creates only Vendor Service
migration `000080`. No route, migration, event, allowlist, secret or deployment
change is required in Customer, Wallet, Dispatch, Hauling, Taxi, Media,
Notification or any other service.

It also requires no new Vendor Service credential and no new permission: the
vendor staff surface evaluates `vendor.orders.read` and `vendor.orders.manage`,
both seeded by Section 5's `000005` migration and already mapped to Owner,
General Manager, Branch Manager and (read only) Analyst/Auditor.

Two future dependencies are recorded here rather than assumed:

1. **Payment execution.** `commerce_parent_orders.payment_operation_ref` exists
   so a later payment-capable section can bind an operation to an order that
   already exists. Section 17E never writes it, and the prepared refund and
   settlement allocations are records, not instructions that have been carried
   out. Wiring `payment-wallet-service` still needs `vendor-service` added to
   `PAYMENT_WALLET_SERVICE_SECRETS`, exactly as
   `architecture/05_PAYMENT_ALLOCATION_MODEL.md` already records.
2. **Delivery confirmation for platform-executed missions.** Section 17E
   refuses a vendor-asserted delivery on a platform-executed allocation,
   because the confirming authority is the logistics service that performed it.
   Mission creation and agreed-fee protection are now live in Section 17F;
   completion remains an authority callback concern and is not fabricated by
   Vendor Service.

One upstream gap is disclosed rather than worked around: a vendor-funded
PRODUCT- or ORDER-scope promotion on a MULTI-vendor quote cannot be attributed,
because Section 17D's commercial adjustment carries no vendor identity. Section
17E refuses such an order (`ErrNotAttributable`) instead of splitting the cost
across vendors. Today this is unreachable in production - the wired promotion
authority is `NoPromotions` - but a future promotion authority must carry vendor
attribution on vendor-funded adjustments before it can ship.

## Section 17F - Payment binding, total price guarantee and recovery

**External service changes: YES.** Section 17F is the first Vendor Service
section since 17C to require real code in other services.

The 17F closure also wires customer commercial changes and cheaper-replan
benefits to the existing Wallet contract. Vendor Service stores the original /
replacement quote chain and immutable adjustment evidence, while Wallet remains
the only authority for additional authorization, refund and settlement.

### payment-wallet-service

**No code change.** Section 17F uses the existing internal contracts exactly as
they are: `POST /internal/payment-intents`, `.../pay-from-wallet`,
`GET /internal/payments/:reference` and `POST /internal/refunds`, all through
`shared/go/walletclient`. Vendor Commerce never writes a ledger entry, computes
a balance, settles vendor funds or declares a payment successful.

Configuration only: `vendor-service` must be added to
`PAYMENT_WALLET_SERVICE_SECRETS`, exactly as
`architecture/05_PAYMENT_ALLOCATION_MODEL.md` already recorded.

**One payment intent per VendorOrder** is forced by that contract, and the
audit that established it is worth recording. `CompleteJob` settles a whole
intent to a single provider and requires status `held`;
`CreateRefundReservation` also requires `held` and moves the intent to
`part_refunded`. With a single parent intent, refunding vendor A would
therefore block settling vendors B and C, and block any second refund - making
the specification's own multi-vendor refund-isolation test impossible to pass.
Splitting at creation makes the isolation structural instead.

### driver-dispatch-delivery-service and driver-hauling-service

**NEW internal ingress in both.** Before Section 17F each exposed exactly one
commerce route, `POST .../internal/commerce/quotes` - it could price a delivery
but nothing could ever ask it to run one. Each now also exposes
`POST .../internal/commerce/missions`, behind the same three gates its quote
surface already uses (HMAC v2, caller allowlist, single-use nonce), allowlisted
to `vendor-service` alone.

New tables: `commerce_missions` in each service (dispatch migration `000041`,
hauling `013`), keyed on the caller's `mission_ref` so a retried activation
returns the same mission rather than dispatching a second vehicle.

Hauling also gained one exported method,
`BookingService.StartCommerceMatching`, because a commerce booking is created
by the mission feature's own transaction rather than by `CreateBooking`. Its
existing match-recovery worker remains the safety net.

Configuration: each service must hold a secret for the `vendor-service` caller
(dispatch `VENDOR_SERVICE_SECRET`, hauling `HAULING_SERVICE_SECRETS`), and
Vendor Service needs `VENDOR_DISPATCH_URL`/`VENDOR_HAULING_URL` as BARE ORIGINS
with matching secrets. Without a secret the mission route is not registered at
all, so a deployment that does not serve commerce missions exposes no route to
probe.

### shared/go

New package `shared/go/commercemission`: the execution counterpart to
`commercequote`. The total-price guarantee lives in the contract itself -
`VerifyMission` refuses any answer whose fee differs from the agreed one, in
either direction - and `Service` structurally excludes Taxi.

### Taxi

**Unchanged and deliberately unused.** `driver-taxi-service` gained no route,
no table and no configuration. It is not a valid value of the shared contract's
`Service` type, and `commerce_order_missions.service` refuses it at the
database.

## Section 18 - Vendor order lifecycle

Vendor Service extends the existing commerce order tables with mutable line
outcomes, substitution decision evidence, refund-state projection and a
deduplicated Dispatch/Hauling event inbox. Inventory, Wallet, ParentOrder,
payment legs and missions remain Section 17 authorities. Platform logistics
events are accepted only for the matching mission and Vendor Own Delivery has
no Dispatch/Hauling dependency. Taxi remains unchanged and unused.

## Shop picking, substitution and variable measurement

Customer Service now owns `customer_quality_preferences` and exposes the
HMAC-protected read contract `/api/v1/customer/internal/customers/{customerId}/quality-preferences`.
Vendor Service consumes it through `internal/integrations/customer` and stores
an immutable order snapshot. The receiving deployment must allowlist the
`vendor-service` caller in its existing service-secret configuration. No
cross-service database read is permitted. Wallet capability responses remain
the source of truth for adjustable-authorisation versus cap-and-refund; an
unknown capability fails closed.

## Kitchen, bakery and production operations

Vendor Service only: migration `000091_vendor_production_operations` extends
the existing ProductionJob authority and adds the Orders timing projection.
No new cross-service route, database dependency, logistics fare mutation or
notification transport was introduced.
## Section 21 commerce

Vendor Service adds additive scheduled/group/RFQ evidence tables and routes,
plus immutable RFQ payment-policy/obligation and corporate recurring
schedule/occurrence/proposal tables. It reuses the existing Production
reservation, Orders conversion, CheckoutQuote/Checkout, Wallet and Fulfilment
authorities. No Dispatch, Hauling, Inventory authority, split payment or
corporate credit authority is introduced.

## Promotions and delivery funding

| Service | Status | Change |
|---|---|---|
| payment-wallet-service | **CHANGED** | HMAC-v2 internal apply/read operations and migration `000009_commerce_delivery_funding` provide exact customer/Vendor/Platform attribution, platform approval evidence, idempotent read-back, balanced ledger effects and delivery-provider earnings. |
| driver-dispatch-delivery-service | **NOT_TOUCHED** | Existing quote/mission authority is reused; promotion and funding rules remain outside Dispatch. |
| driver-hauling-service | **NOT_TOUCHED** | Existing quote/mission authority is reused; promotion and funding rules remain outside Hauling. |

Vendor Service migration `000097_promotion_campaigns_and_delivery_funding`
owns campaign/usage evidence, immutable quote/order snapshots and a Wallet
operation projection. CheckoutQuote remains the commercial calculator. Vendor
staff may create only Vendor-funded campaigns and Vendor-funded HYBRID fallback
policies. Platform/combined funding requires a stored external approval
reference and fails closed without it.

## Payment and Wallet commerce extension

| Service | Status | Change |
|---|---|---|
| payment-wallet-service | **CHANGED** | Migration `000010_commerce_payment_allocation_authority`; one parent commerce intent, immutable allocations, maximum hold, isolated release/refund, Vendor payable owner, scoped bank/withdrawal/read-back, audit and outbox evidence. |
| vendor-service | **CHANGED** | Migration `000098_vendor_finance_and_wallet_allocation_projection`; single-intent order adapter, durable allocation fingerprint, settlement/refund integration, and authenticated Vendor Finance façade. |
| driver-dispatch-delivery-service | **NOT_TOUCHED** | Existing commerce mission and delivery fact authority preserved; Wallet exposes the service-authorized provider-delivery release capability. |
| driver-hauling-service | **NOT_TOUCHED** | Existing commerce mission and delivery fact authority preserved. |
| customer-service | **NOT_TOUCHED** | Existing customer references remain authoritative; no identity or Wallet logic duplicated. |

## Commerce settlement and refund model

| Service | Status | Change |
|---|---|---|
| payment-wallet-service | **CHANGED** | Migration `000011`; existing Commerce allocation refund now records exact customer/funder outputs, escrow/released-beneficiary recovery sources, Platform-loss funding, component reductions and authorization release with allocation-local caps. Authenticated Dispatch/Hauling delivery release is caller/provider-bound and resolves its immutable allocation fingerprint without exposing Vendor-only intent reads. |
| vendor-service | **CHANGED** | Migration `000099`; deterministic versioned policy evidence, recovery/read-back, truthful projections, exact Inventory item release and Support handoff. No new public route. |
| driver-dispatch-delivery-service | **CHANGED** | Existing commerce-trip completion now invokes Wallet provider-delivery release with Dispatch-owned assigned-provider identity and a stable key. |
| driver-hauling-service | **CHANGED** | Existing commerce booking completion/recovery now invokes Wallet provider-delivery release with Hauling-owned assigned-provider identity and a stable key. |
| support-dispute-service | **CHANGED** | Existing HMAC-v2 organization-case route is allowlisted for `vendor-service`; Support remains the case/adjudication authority and receives no amount instruction. |
| customer-service | **NOT_TOUCHED** | Customer identity and references are reused; no financial authority added. |
| notification-service | **NOT_TOUCHED** | Existing order/outbox notifications are reused; no settlement transport is added. |
| driver-taxi-service | **NOT_TOUCHED** | Taxi remains outside Commerce fulfilment settlement. |

Deployment requires the existing matching HMAC v2 secret for `vendor-service`
in Payment Wallet. Dispatch/Hauling secrets remain required only for scoped
delivery-release/read-back operations they own.

## Fulfilment group, parcel and mission model

| Service | Status | Detail |
|---|---|---|
| `vendor-service` | **CHANGED** | New `internal/features/fulfilmentgroup`, migration `000100`, 13 routes, one OpenAPI contract, a narrow `FulfilmentProjection` port on the orders feature |
| `driver-dispatch-delivery-service` | **NOT_TOUCHED** | `shared/go/commercemission` is reused unchanged; its commerce ingress and event emission are as they were |
| `driver-hauling-service` | **NOT_TOUCHED** | Same |
| `driver-taxi-service` | **DEFERRED** | No commerce mission ingress exists. `AuthorityTaxi` is modelled in the vocabulary so a future integration needs no schema change; `ActiveForCommerce()` returns false and selection is refused at plan validation and reservation. Nothing was added to this service |
| `payment-wallet-service` | **NOT_TOUCHED** | Remains the financial authority. The plan stores a cost snapshot as evidence only |
| `support-dispute-service` | **NOT_TOUCHED** | Section 24's escalation path is unchanged |
| `notification-service` | **NOT_TOUCHED** | No new notification |
| `customer-service` | **NOT_TOUCHED** | — |

### External limitations, reported rather than worked around

- **Multi-stop pickup: PARTIAL.** `shared/go/commercemission.Request` carries
  one `Pickup` and one `Dropoff`. Vendor Service models multi-pickup fully and
  declares the authorities' real capability
  (`MultiStopPickup: false`); a replan therefore plans compatible units onto
  separate missions rather than one the authority would have to fail. No
  logistics service was redesigned to change this.
- **Provider transfer: PARTIAL.** No authority emits a transfer event today.
  `CustodyTransferred` is modelled and safe to record the moment one does;
  Vendor Service does not fabricate it.
- **Mission replacement notification: PARTIAL.** No authority reports that it
  replaced a mission. `ReplacePlatformMission` exists and preserves both rows,
  but is currently driven only from within this service.

---

## Dispatch Commerce delivery integration

| Service | Change | Boundary |
|---|---|---|
| `driver-dispatch-delivery-service` | MODIFIED | Canonical route-aware multi-pickup quote, mission, marketplace, custody, recovery, return and targeted lifecycle outbox authority; migration `000042`; 13 routes. |
| `vendor-service` | MODIFIED | Immutable plan/parcel adapter, `PlatformMissionRef` projection and one HMAC-v2 lifecycle callback. No Dispatch state authority. |
| `shared/go/dispatchcommerce` | ADDED | Versioned V1 wire contract and HMAC client. No business authority or persistence. |
| `payment-wallet-service` | NOT_TOUCHED | Existing funding, refund and provider/Vendor settlement authorities remain unchanged. |
| `driver-hauling-service` | NOT_TOUCHED | Existing hauling execution is unchanged. |
| `driver-taxi-service` | NOT_TOUCHED | Goods delivery remains out of scope. |
| Fleet partner service | NOT_TOUCHED | Commerce jobs use the public provider marketplace and are not Fleet private orders. |

The Dispatch outbox targets Vendor Service for mission, stop and parcel facts.
Vendor's Section 24 refund policy consumes failure evidence independently;
Dispatch never moves customer money or edits Vendor payable.

---

## Delivery Requirement and Vehicle Selection Engine

**No platform service was changed.** No new credential, allowlist entry, secret
or configuration is required in any other service.

| Service | Change | Note |
|---|---|---|
| `driver-dispatch-delivery-service` | NOT_TOUCHED | Its motorcycle envelope (25 kg / 80 L / 700 mm, insulated-box handling) is the authority. Vendor Service's capability policy mirrors it deliberately: a looser limit would produce plans dispatch then refuses, a tighter one would refuse work dispatch would take. |
| `driver-hauling-service` | NOT_TOUCHED | Owns both `VAN` and `TRUCK`, and resolves the actual truck type — flatbed, box, refrigerated — inside its own matcher. Vendor Service does not model those sub-types; duplicating the taxonomy would create two vocabularies that drift. |
| `driver-taxi-service` | NOT_TOUCHED | A two-file scaffold with no goods concept. The car class is modelled and permanently gated. **Enabling it is not a configuration change** — it requires building goods mode in that service. |
| `payment-wallet-service` | NOT_TOUCHED | No financial engine, ledger or fee exists in this work. `declared_value_minor` is risk metadata that may exclude a carrier class and never becomes a charge. |
| `notification-service` | NOT_TOUCHED | — |
| `api-gateway` | NOT_TOUCHED | No routes were added. |
| `shared/go/commercequote` | NOT_TOUCHED | Remains the canonical wire vocabulary: `MOTORCYCLE`, `VAN`, `TRUCK`. `CAR` is a Vendor-side conceptual class only and is never sent. |

### Future work this section does not perform

- **Taxi goods mode.** Would make `VehicleCar.GoodsModeActive` meaningful. It
  needs a goods capability, a commerce mission ingress and a quote contract in
  `driver-taxi-service`, plus `CAR` in `shared/go/commercequote.VehicleClass`.
- **Multi-pickup runtime.** `ServiceabilitySnapshot.MultiPickup` and
  `VehicleCapability.SupportsMulti` exist so this becomes configuration rather
  than a redesign. Both dispatch and hauling would first need to model a
  multi-stop mission; `shared/go/commercemission.Request` carries one pickup.


## Multi-Vendor Consolidation and Mission Planning

| Service | Status | Note |
|---|---|---|
| `driver-dispatch-delivery-service` | NOT_TOUCHED | Read for a quote through the existing `shared/go/commercequote` contract. Multi-pickup remains NOT_PRESENT and is declared as configuration, not asserted in code. |
| `driver-hauling-service` | NOT_TOUCHED | As above. |
| `driver-taxi-service` | NOT_TOUCHED | Still no goods capability. Deliberately not wired, as before. |
| `payment-wallet-service` | NOT_TOUCHED | Nothing in this capability holds, moves, refunds or projects money. The optimizer is composed with no Wallet dependency at all, which is what makes "a hundred candidate baskets create zero payment intents" structural. |
| `customer-service` | NOT_TOUCHED | Remains the customer identity authority. Commerce-owned preference and template records reference the customer; none duplicates a profile. |
| `notification-service` | NOT_TOUCHED | No notification is sent by this capability yet. See below. |
| `support-dispute-service` | NOT_TOUCHED | An optimizer finding no cheap basket is a structured no-solution response, not a support case. |
| `media-file-service` | NOT_TOUCHED | No media. |
| `api-gateway` | NOT_TOUCHED | No routes added there. |
| `shared/go/commercequote` | NOT_TOUCHED | Still one pickup per request, which is exactly why the multi-stop gate fails closed. |
| `shared/go/commercemission` | NOT_TOUCHED | Quote-only planning never reaches it. |

### Configuration this capability adds

All Vendor Service only, all defaulting to off or to the platform default:

| Variable | Default | Purpose |
|---|---|---|
| `COMMERCE_BASKET_OPTIMIZATION_ENABLED` | `false` | Master switch for Optimize My Cart and budget shopping. Requires fulfilment planning and checkout. |
| `COMMERCE_RECURRING_BASKET_ENABLED` | `false` | Recurring baskets. Separate because recurring generation is the only part that runs without a customer asking. |
| `COMMERCE_BASKET_PROPOSAL_TTL_SECONDS` | `600` | How long an offer stands. |
| `COMMERCE_BASKET_EXPIRY_INTERVAL_SECONDS` | `60` | Proposal expiry sweep. |
| `COMMERCE_RECURRING_BASKET_INTERVAL_SECONDS` | `60` | Recurrence sweep. Affects promptness only; correctness rests on the unique index. |
| `COMMERCE_FULFILMENT_PROXIMITY_RADIUS_METERS` | `3000` | Candidate-construction bound. Must be at least the consolidation radius, refused at boot otherwise. |
| `DISPATCH_MULTI_PICKUP_SUPPORTED` | `false` | Declares the runtime capability. Refused at boot unless the dispatch integration is enabled, so the flag cannot sit in a config file looking as though it did something. |
| `HAULING_MULTI_PICKUP_SUPPORTED` | `false` | As above. |

### Future work this capability does not perform

- **Multi-pickup runtime.** The planning model, the capability declaration, the
  stop-count computation and the reason code all exist, so enabling it is a
  configuration change plus real work in the logistics services: both would
  need to model a multi-stop mission, and
  `shared/go/commercemission.Request` carries one pickup.
- **Notifications.** A recurring occurrence awaiting approval is a good
  candidate for `notification-service`, and none is sent today. The generation
  is readable through the occurrence list; a customer who does not open the app
  will not be told.
- **An ordinal quality signal.** Until the catalogue carries one,
  QUALITY_BALANCED will keep reporting itself unavailable, which is the honest
  outcome rather than a gap to paper over.

## Vendor-owned and Hybrid delivery runtime — current dependency status

This entry supersedes the historical Section 5 OTP/QR absence classification.
`shared/go/handover` and Dispatch now provide a secure authority for Dispatch
Commerce missions, so the platform capability is `PARTIAL`. No approved
VendorMission-compatible adapter exists. Vendor pickup, receiver completion
and return therefore remain fail-closed behind `VendorHandoverAuthority`; no
competing credential domain or secret storage was added.

Wallet changed through migration `000012_commerce_delivery_responsibility_transition`
and an internal HMAC v2 command/read-back contract. Dispatch is reused through
its accepted Section 28 quote/create/read-back APIs and receives no Section 31
schema or route change. Customer Service, Support, Hauling and Taxi are not
touched by this capability.

## Operational evidence, delivery exceptions, returns and provider transfer

| Service | Change | Note |
|---|---|---|
| `driver-dispatch-delivery-service` | CHANGED | Extends the canonical Commerce mission with immutable manifest freeze, append-only evidence, controlled exceptions/returns and two-provider post-custody transfer; migration `000043`; seven routes. |
| `vendor-service` | CHANGED | Adds migration `000105`, an authenticated projection of Dispatch operational facts, and fulfilment/custody history mapping. It does not become mission or evidence authority. |
| `shared/go/handover` | CHANGED | Adds a distinct provider-transfer challenge step to the existing HMAC authority; no second OTP system. |
| `shared/go/dispatchcommerce` | CHANGED | Carries manifest version/timestamp, package count, custodian and evidence metadata. |
| `media-file-service` | REUSED | Remains the private media-byte/access authority; only opaque refs are stored elsewhere. |
| `payment-wallet-service` | REUSED | Remains the sole ledger and existing completion settlement authority. No split is needed because the old provider cannot complete after replacement begins. |
| `driver-hauling-service` | NOT_TOUCHED | No safe multi-Parcel transfer runtime was inferred from its single-pickup proof model. |
| `notification-service` | REUSED | Existing outbox/notification integrations only; no new notification authority. |
| `support-dispute-service` | REUSED | Controlled `SUPPORT` recovery may be projected; no automatic support case is fabricated. |

Section 31 remains truthful: Vendor secure handover is partial/dependency-gated
and Vendor Rider → platform provider post-custody transfer is not present.

## Realtime, notifications and confidence ETA

| Service | Change | Note |
|---|---|---|
| `vendor-service` | CHANGED | Migration `000106`; immutable mission ETA history/current pointer, deterministic confidence engine, scheduled cutoff worker, typed outbox notification intents and existing customer/vendor read-model enrichment. No new route. |
| `notification-service` | CHANGED | Migration `002`; durable source aggregate/event/version/correlation provenance and controlled Commerce templates. Existing preference, retry, in-app, WebSocket, push and email machinery is reused. No business-domain state added. |
| `driver-dispatch-delivery-service` | CHANGED | Existing Commerce events now carry authoritative plan, route, provider and stop-progress evidence. Existing Dispatch-owned receiver credential flow submits its notification intent through Notification Service. No migration or route added. |
| `driver-hauling-service` | REUSED | Existing provider notification ownership and delivery events remain unchanged; no unsupported Commerce ETA adapter was fabricated. |
| `customer-service` | REUSED | Customer identity/preferences remain authoritative; no frontend or duplicate ETA state added. |
| `payment-wallet-service` | REUSED | Existing refund completion projection supplies notification facts; money authority is unchanged. |
| `support-dispute-service` | REUSED | Existing support facts may produce bounded Vendor action intents; no support authority moved. |

All outbound user delivery remains Notification Service-owned. Vendor and
Dispatch provide only controlled semantic data and opaque recipient references.
