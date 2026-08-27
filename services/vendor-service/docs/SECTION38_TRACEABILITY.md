# Section 38 support and dispute integration traceability

## Authority and lifecycle

`support-dispute-service` owns formal Commerce cases, evidence review, adjudication and the controlled resolution vocabulary. Vendor Service owns only an order-scoped safe projection and never accepts an adjudication from a customer or Vendor route. Wallet remains the sole ledger and the accepted Section 24 refund-policy snapshot is the only input a `REFUND_AUTHORIZED` resolution can execute.

The internal create/read-back and resolution callback use HMAC signature v2 with exact caller allowlists. The same operation reference is idempotent; a changed typed payload produces a fingerprint conflict. Support commits the resolution and its outbox event atomically. Vendor records an event inbox row and advances a projection only when `resolution_version` increases.

## Controlled issue categories

- `WRONG_ITEM_OR_MEAL`
- `MISSING_ITEM`
- `UNAUTHORIZED_SUBSTITUTION`
- `DAMAGED_OR_SPOILT_PRODUCT`
- `QUALITY_ISSUE`
- `VENDOR_DELAY`
- `DELIVERY_DELAY`
- `PICKUP_DISPUTE`
- `RECEIVER_HANDOVER_DISPUTE`
- `REFUND_PROBLEM`
- `VENDOR_OWNED_DELIVERY_ISSUE`

Case context carries opaque order, Vendor, mission, parcel, line, evidence, media, refund and substitution references only. Media bytes stay in media-file-service. Full profiles, addresses, contacts, payment data and raw OTP/QR/challenge material are excluded.

## Existing authorities reused

- Section 24 determines automatic versus Support-required refunds. Deterministic decisions bypass manual adjudication and remain auditable in `commerce_refund_policy_decisions`.
- Sections 33–35 remain the evidence, custody, return and provider-transfer authorities; cases store references rather than copies.
- Dispatch and Hauling missions remain canonical in their owning services. `PlatformMissionRef` is projection-only; Vendor-owned delivery uses the existing `VendorMission` domain.
- Notification Service receives controlled semantic case-resolution events from the Vendor transactional outbox. Refund-completion notification remains tied to authoritative refund completion, not Support resolution.

## Frontend handoff

Customer reporting uses `POST /api/v1/commerce/orders/{orderId}/support-cases` with an `Idempotency-Key`, one category above, a bounded description, the relevant `vendor_order_ref`, and optional bounded line references. Evidence/media upload remains a separate Media/evidence flow; send opaque references only through an authorized backend flow.

Customer reads expose `status`, `resolution`, `resolution_version`, and a separate `refund_status`. A resolved Support case may still show a refund as processing until Wallet confirms it. Vendor staff use the matching Vendor-order case routes and see no private customer profile or internal agent notes. No frontend was implemented here.

## Requirement map and readiness

| Requirement | Authority | Code / database | Route or event | Evidence |
|---|---|---|---|---|
| Controlled category and bounded context | Shared contract; Support validates | `shared/go/supportcase`; Support migration `007` | typed internal create | shared contract tests + migration shape |
| Customer/Vendor ownership and one logical link | Vendor | `orders/support_cases*.go`; Vendor migration `000107` | customer and Vendor order case routes | route parity + PostgreSQL race test |
| Unknown create outcome and fingerprint conflict | Support | Support repository + Vendor Support client | operation read-back | client lost-response test + PostgreSQL conflict test |
| Formal resolution and durable publication | Support | Support resolution transaction + `support_case_outbox` | `commerce.support_case.resolved` | PostgreSQL version/outbox test |
| Replay-safe customer/Vendor projection | Vendor | resolution inbox + case link | private Support callback | PostgreSQL replay/stale test |
| Manual refund after adjudication | Section 24 + Wallet | immutable refund policy record and existing Wallet execution/recovery | Support resolution references existing case/policy | orders/refund regression; no amount accepted |
| Case-resolution notification | Notification Service | Vendor domain outbox adapter | controlled semantic notification | events package regression |
| Evidence and secret safety | Dispatch/Hauling/VendorMission/Media authorities | server-derived opaque refs; no blob/credential columns | case context only | migration shape + contract validation |

Database concurrency tests are environment-gated by the established
`VENDOR_SERVICE_TEST_DATABASE_URL` and `SUPPORT_DISPUTE_TEST_DATABASE_URL`.
When those canonical databases are unavailable, schema shape and all
deterministic package tests remain executable but PostgreSQL certification is
reported as blocked by test infrastructure.

Section 37: **SKIPPED / NOT_IMPLEMENTED_BY_THIS_RUN**. Section 39: not started.
