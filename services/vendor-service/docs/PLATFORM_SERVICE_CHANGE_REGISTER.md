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
