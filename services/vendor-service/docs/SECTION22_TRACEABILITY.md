# Delivery Policy, Promotions and Delivery Funding Traceability

Roadmap numbering is retained only in this certification document. Production
source and OpenAPI filenames are capability/domain named.

| Requirement | Authority/schema | Route/event | Test/evidence |
|---|---|---|---|
| PLATFORM/VENDOR/HYBRID policy, zones, radius, fee rules, hours, minimum, free threshold, capacity and bounded overflow | `features/delivery`, `features/fulfilment`; migrations `000077`, `000096`, `000097` | Existing delivery policy and fulfilment plan routes; `vendor.delivery.policy.changed` | delivery policy evaluator/quote and fulfilment hybrid tests |
| Pre-custody replan, identity/custody and customer guarantee | `features/fulfilment/hybrid_replan.go`; immutable plan evidence | Existing planning/outbox path | `hybrid_replan_test.go`, `hybrid_planner_test.go` |
| Percentage/fixed/coupon/bundle/quantity/free-delivery/delivery discounts | `features/promotions`; `commerce_promotion_*` | Seven `/api/v1/vendor/promotions` operations; `promotion.created`, `promotion.updated`, `promotion.coupon.created` | `calculation_test.go`, promotion/OpenAPI contract |
| Branch/capability/category/item/variant/segment/order/time eligibility | Promotions evaluator using Vendor capability, Food/Shop taxonomy and CRM membership authorities | CheckoutQuote create consumes only authoritative coupon code/context | scope, time, lifecycle and tenant-bound evaluator tests |
| Usage limits and coupon concurrency | Campaign row/coupon row locks plus `commerce_promotion_redemptions` | Quote creation reserves; order conversion consumes; lifecycle job releases expired abandoned reservations | CheckoutQuote store transaction and migration shape; canonical PostgreSQL suite |
| Deterministic combinability | Priority then campaign ID; EXCLUSIVE returns one result | Frozen adjustment priority | deterministic/exclusive tests |
| Immutable quote/order/refund evidence | `commerce_quote_adjustments`, parent/Vendor `promotion_snapshot` and `delivery_funding_snapshot` | Existing CheckoutQuote/order/refund flow | frozen campaign revision and allocation/refund regressions |
| CUSTOMER/VENDOR/PLATFORM/COMBINED attribution | CheckoutQuote/Orders allocation; Vendor `commerce_delivery_funding_operations` | No public funding control | order allocation and hybrid funding tests |
| HYBRID fallback gap | Approved customer contribution retained; policy selects Vendor/Platform/Combined gap sponsor | Existing plan/quote flow | `TestHybridPlatformFundingPreservesGuaranteeAndAttributesGap` |
| Wallet authority/provider earnings | Wallet migration `000009_commerce_delivery_funding`; balanced ledger transaction | HMAC-v2 POST/GET `/api/v1/payment-wallet/internal/commerce/delivery-funding` | Wallet repository/usecase/http/OpenAPI tests |
| Platform spend authorization | Campaign/policy approval references frozen and forwarded to Wallet | Vendor APIs reject non-Vendor sponsor; Wallet rejects missing approval | promotion, delivery-policy and Wallet validation tests |
| Tenant/IDOR safety | business/branch predicates, IAM scopes, quote customer ownership, Wallet source-service fingerprint | bearer Vendor/customer separation and HMAC internal surface | route/security integration plus targeted foreign-sponsor tests |
| Retry/recovery | Quote/order uniqueness, redemption/Wallet idempotency keys, UNKNOWN Wallet read-back | Existing jobs runtime and Wallet GET | concurrent/retry package tests and canonical regression |

Cancellation does not restore a consumed promotion: consumption is permanent
once an order is created. An abandoned expired quote reservation is released
idempotently. Refunds continue to use the frozen VendorOrder allocation and
funding snapshot; no current campaign or taxonomy read participates.
