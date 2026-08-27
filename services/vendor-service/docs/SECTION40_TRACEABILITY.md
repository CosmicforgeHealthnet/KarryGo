# Section 40 Traceability

Section 40 is a security, privacy and compliance hardening pass. It reuses the
existing authentication, RBAC, HMAC, Wallet, Media, Support, Analytics and
handover authorities. It does not create a new auth service, RBAC system,
Wallet, Media service, Support system, Analytics service or event bus.

## Security Capability Matrix

| Threat | Authority | Implementation evidence | Negative test / gate | Readiness |
|---|---|---|---|---|
| TENANT_ESCAPE | Vendor Auth/RBAC + tenant-scoped repositories | Vendor routes resolve `Actor.VendorID` from authenticated context; Shop/CRM/Inventory/Order repositories include business scope | Vendor full regression; existing CRM/Shop/Inventory isolation tests | PASS |
| BRANCH_ESCAPE | Vendor RBAC scope resolver | Branch-scoped Shop/Inventory/Food/Purchasing services require `Scope.Covers(branch_id)` | existing Shop/Inventory/Section 9B security tests | PASS |
| CUSTOMER_IDOR | Customer-owned CommerceOrder reads | Customer support/order actions validate parent order customer ownership | existing Commerce order/support tests | PASS |
| CRM_IDOR | Vendor CRM relationship service | CRM profile reads and mutations use business/branch scoped relationships | existing CRM privacy/isolation tests | PASS |
| INVENTORY_IDOR | Shop/Inventory authority | Stock basis, movement, stocktake and transfer access is business/branch scoped | existing Shop inventory security tests | PASS |
| QUOTE_IDOR | CheckoutQuote authority | CheckoutQuote is derived from owned checkout session and fingerprinted source snapshot | existing checkout quote tests | PASS |
| MISSION_IDOR | VendorMission / Dispatch / Hauling authorities | Vendor rider/provider actions are assignment scoped; platform mission reads stay in provider services | existing mission/transfer tests | PASS |
| MEDIA_IDOR | media-file-service metadata + Dispatch mission authority | Dispatch reads active private Media metadata and requires the exact Dispatch owner/mission plus exact Parcel metadata when present before any operational-evidence or provider-transfer write | `TestMediaAttachmentAuthorizerMissionAndParcelBinding`; `TestOperationalEvidenceMediaAuthorizationFailsClosed`; `TestProviderTransferUsesSameMediaAuthorizationBoundary` | PASS |
| SUPPORT_IDOR | support-dispute-service + Vendor projection | Case creation is order scoped; formal adjudication stays Support-owned | existing Section 38 support tests | PASS |
| ANALYTICS_IDOR | analytics-service | Vendor report/reliability reads require HMAC v2, allowlisted caller and matching `X-Analytics-Business-ID`; admin backoffice is explicitly allowlisted | `TestAnalyticsReadRequiresHMACAndBusinessScope` | PASS |
| RBAC_ESCALATION | Vendor Auth/RBAC | Finance/bank/staff/role-sensitive actions require specific permissions; Vendor Rider remains staff-limited | existing auth/security tests | PASS |
| HMAC_REPLAY | shared `serviceauth` + endpoint nonce cache | HMAC v2 binds caller/method/path/body/timestamp/nonce; Analytics read routes reject replayed nonce | `TestAnalyticsHMACReplayRejected`; shared serviceauth tests | PASS |
| HMAC_CALLER_CONFUSION | service allowlists | Analytics reads allow only analytics-client-service/vendor-service/admin-backoffice-service; ingests allow only configured internal producers | Analytics route tests | PASS |
| OTP_REPLAY | handover/auth challenge authorities | OTP hashes/attempt counters remain in auth/handover authorities; Vendor Commerce stores refs/state only | existing auth/handover tests; DB race certification env-gated | PASS/BLOCKED_BY_TEST_INFRASTRUCTURE for DB race |
| SECRET_LEAK | config + log-safe DTOs | Raw OTP/card/gateway secrets are not introduced by Section 40 changes | static search; changed tests | PASS |
| PII_OVEREXPOSURE | Vendor/Support/Analytics/Notification data minimization | Analytics events use aggregate/opaque refs; Support and notification payloads keep semantic refs | Section 38/39 traceability and tests | PASS |
| MEDIA_REF_SWAP | media-file-service relation validation | MediaRef possession/read access is insufficient: Dispatch separately proves attachment to the target mission and, when metadata is Parcel-bound, the exact Parcel set | `TestMediaAttachmentAuthorizerMissionAndParcelBinding` (cross-mission, cross-Parcel, invalid and valid cases) | PASS |
| AUDIT_TAMPERING | `vendor_audit_log` + PostgreSQL | Sensitive Vendor actions continue to insert audit evidence; migration `000108_vendor_audit_append_only` rejects direct `UPDATE` and `DELETE` in the database | `TestVendorAuditAppendOnlyMigrationShape`; `TestVendorAuditLogDatabaseAppendOnly` (PostgreSQL env-gated) | PASS / DB execution BLOCKED_BY_TEST_INFRASTRUCTURE |
| AUTH_ABUSE | established Redis rate limiter | Vendor auth uses central `internal/platform/ratelimit`; Customer auth has OTP/rate-limit repos | existing auth tests | PASS |
| SEARCH_ABUSE | shared Redis rate limiter | HMAC-authorized Food/Shop search, facet and suggestion operations use the atomic shared Redis fixed-window limiter keyed by authenticated caller | `TestSearchRateLimitAllowanceIsolationAndReset`; `shared/go/ratelimit` tests | PASS / Redis race execution BLOCKED_BY_TEST_INFRASTRUCTURE |
| QUOTE_ABUSE | shared Redis rate limiter + CheckoutQuote authority | Every new quote request is rate-limited by authenticated customer independently of quote idempotency/fingerprinting | CheckoutQuote rate-limit tests; `shared/go/ratelimit` tests | PASS / Redis race execution BLOCKED_BY_TEST_INFRASTRUCTURE |
| OTP_ABUSE | auth/handover authority | OTP resend/verify attempts are challenge-authority controlled | existing auth tests | PASS |
| REPORT_ABUSE | shared Redis rate limiter | Customer and Vendor support-case creation are limited by authenticated principal; Analytics report/reliability reads are limited by caller + business + branch | `TestSupportRateLimitAllowanceIsolationAndReset`; Analytics read rate-limit test | PASS / Redis race execution BLOCKED_BY_TEST_INFRASTRUCTURE |
| CONTACT_ABUSE | Notification/proxy authority | Notification receives semantic events and opaque recipient refs; contact fields are minimized | notification contract tests | PASS |
| FINANCIAL_TAMPERING | Wallet authority | Vendor/Support/Analytics provide evidence only; Wallet remains ledger authority | Wallet commerce tests | PASS |
| STATE_TAMPERING | VendorOrder/Dispatch/Handover authorities | Terminal states require commands/evidence/authority, not arbitrary client state fields | existing order/dispatch tests | PASS |
| ANALYTICS_INFERENCE | analytics-service | Benchmark requires 5 external comparable vendors, excludes requester, coarse ranges and server-owned buckets | commerceanalytics privacy tests | PASS |
| RESTRICTED_ITEM_BYPASS | platform-owned Vendor Commerce projection of canonical taxonomy | One typed/versioned policy (`vendor_commerce_restricted_goods_v1`) classifies ALLOWED/RESTRICTED/PROHIBITED/REGULATED_EXTERNAL_DOMAIN and is reused at catalogue create/update/import, listing activation, RFQ, checkout and final fulfilment release | taxonomy, Shop and Orders restricted-goods tests | PASS |
| PHARMACY_BYPASS | canonical restricted-goods policy | Prescription, OTC, pharmaceutical and regulated medicinal goods resolve to REGULATED_EXTERNAL_DOMAIN and fail closed at RFQ and final fulfilment as well as catalogue/checkout | `TestRestrictedGoodsPolicyMedicineAndGeneralGoods`; `TestRFQRestrictedGoodsGate`; `TestSnapshotRestrictedGoodsGateBlocksLegacyFulfilment` | PASS |

## Section 40 Hardening Changes

- Analytics vendor report and reliability read routes now require internal HMAC
  v2 authentication, caller allowlists, nonce replay rejection and explicit
  business-scope matching unless the caller is admin backoffice.
- Reliability `vendor_reliability_v1` now uses the neutral equal-weight
  technical default because no approved pre-existing KarryGo weight policy was
  found and no persisted V1 snapshots were found.
- Price benchmarks require five independent comparable external Vendors after
  requester exclusion and use server-owned comparable-product, geo-bucket,
  unit-normalization and freshness/window gates.
- Shop Commerce rejects pharmacy/medicine products on manual product
  create/update, bulk catalogue import and checkout revalidation of stale
  legacy records.
- The focused repair introduced one platform-owned, typed and versioned
  restricted-goods decision function in the existing canonical taxonomy
  package. Vendor/customer payload fields are facts only and cannot override
  the decision. Existing platform taxonomy publication/governance remains
  restricted by `vendor.taxonomy.platform.admin`; its command/change/
  publication history is append-only and auditable.
- RFQ requirements and final fulfilment release now reuse that same policy.
  A legacy or newly prohibited item is rejected before inventory confirmation,
  delivery funding, mission creation or custody transfer; order and financial
  history are not rewritten.
- Dispatch now reads Media metadata through the existing authenticated Media
  client before evidence persistence. It requires active Dispatch ownership,
  exact mission ownership, an evidence purpose and exact Parcel metadata when
  the upload is Parcel-bound. The same boundary protects provider transfer.
- Search, quote creation, Commerce support creation and expensive Analytics
  reads use `shared/go/ratelimit`, an atomic Redis `INCR`/`PEXPIRE` script.
  Limits are operation-specific and centrally configured; idempotency and
  query bounds remain separate controls.
- Migration `000108_vendor_audit_append_only` adds a PostgreSQL trigger that
  permits normal audit inserts and rejects updates/deletes. Its down migration
  removes only the guard and preserves all audit rows.

## Focused Repair History

The forensic verification immediately before this repair classified Dispatch
MediaRef relation checks, general restricted-goods enforcement, RFQ/final
fulfilment medicine enforcement, non-auth abuse limiting and database audit
immutability as **PARTIAL**. The controls above are the limited repair. Their
non-database behavior is executed by focused tests. PostgreSQL append-only and
distributed Redis race execution remain infrastructure-gated; that is a test
environment dependency, not reported as an executed runtime PASS.

### Abuse configuration

Vendor defaults are configurable with `VENDOR_ABUSE_RATE_LIMIT_REDIS_PREFIX`,
`VENDOR_SEARCH_RATE_MAX`, `VENDOR_SEARCH_RATE_WINDOW_SECONDS`,
`VENDOR_QUOTE_RATE_MAX`, `VENDOR_QUOTE_RATE_WINDOW_SECONDS`,
`VENDOR_SUPPORT_RATE_MAX` and `VENDOR_SUPPORT_RATE_WINDOW_SECONDS`. Analytics
uses `ANALYTICS_REDIS_*`, `ANALYTICS_READ_RATE_LIMIT_REDIS_PREFIX`,
`ANALYTICS_READ_RATE_MAX` and `ANALYTICS_READ_RATE_WINDOW_SECONDS`. Production
uses Redis; test fakes do not replace that authority.

## Frontend Handoff

- Customer/receiver/provider PII should render only server-returned minimized
  fields; do not infer hidden profile data.
- Analytics benchmark suppression is expected for low-sample cohorts and should
  be shown as unavailable rather than empty.
- Rate-limited actions should show retry/cooldown UX without exposing account
  existence. The controlled response is HTTP 429.
- Media refs require signed/time-bound access from media-file-service; refs are
  not directly fetchable URLs. Evidence attachment can also be rejected when
  an otherwise valid ref belongs to another mission or Parcel.
- Restricted/prohibited products are rejected by a server-owned policy.
  Pharmacy/medicine Vendor Commerce is backend-excluded. Frontend may explain
  either rejection but must not offer an override.
- Authorization errors should be treated as controlled unavailable/forbidden
  states; do not reveal private object existence.

## Dependencies

Canonical PostgreSQL-backed certification for migration replay and security
race tests remains gated by the established service test database environment
variables. No random database or fabricated connection string is permitted.
