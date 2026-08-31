# Section 39 Traceability

Section 39 implements analytics, reporting and reliability as downstream decision support. Vendor Service remains the authoritative source for VendorOrder, inventory, purchasing, supplier, production, RFQ, CRM and fulfilment facts. Analytics Service consumes committed events and owns aggregate/read models.

## Capability Matrix

| Requirement | Source authority | Analytics event/read model | Endpoint/test | Readiness |
|---|---|---|---|---|
| Sales and AOV | VendorOrder | `commerce.order.completed`, `analytics_vendor_daily_metrics` | `/api/v1/analytics/commerce/vendor-reports` | PASS |
| Repeat customer and CRM cohorts | Vendor CRM projection | `commerce.crm.relationship`, `analytics_vendor_crm_cohorts` | report CRM block | PASS |
| Product/category performance | Catalogue/order lines | item/category dimension metrics | report dimension rollups | PASS |
| Stock-outs and lost opportunity | Inventory/search facts | `commerce.search.demand`, stockout estimate | report demand block | PASS |
| Inventory turnover/waste/expiry | Inventory movements | inventory movement and waste events | report inventory block | PASS |
| Purchase cost trends | Vendor purchasing/receiving | supplier metrics | report supplier/inventory blocks | PASS |
| Supplier on-time/short/damage | Purchasing receipt facts | supplier metrics | report supplier block | PASS/NOT_AVAILABLE_DUE_TO_DATA when damage evidence is absent |
| Preparation/picking time | Production/shop lifecycle | timing events | report operations block | PASS |
| Acceptance/cancellation/refund reasons | VendorOrder and Wallet/Support facts | daily operational metrics | report operations block | PASS |
| Own-rider vs platform fulfilment | VendorMission and platform mission refs | fulfilment performance events | report fulfilment block | PASS |
| Cloud Kitchen/Pastry utilization | Production capacity model | capacity metrics | report production block | PASS |
| RFQ conversion | Section 21 RFQ lifecycle | existing RFQ events | report RFQ block | PASS |
| Reliability score | Analytics aggregate | reliability snapshot table | `/api/v1/analytics/commerce/vendor-reliability` | PASS |
| Demand/price intelligence | Search/inventory/price aggregates | privacy-safe dimension metrics | report demand/price blocks | PASS |
| Future Order Guarantee | Evidence only | disabled policy model | no claim route | FUTURE_DISABLED |

## Reliability Policy

Policy version: `vendor_reliability_v1`.

Policy source: forensic verification found no approved pre-Section 39 product,
architecture, configuration or policy file defining non-equal reliability
weights. Because no repository fixture or current environment evidence showed
persisted V1 reliability snapshots, V1 remains the current unreleased policy
and uses the neutral deterministic technical default.

Components:
- acceptance: 0.20
- on-time readiness: 0.20
- vendor cancellation: 0.20
- availability/substitution accuracy: 0.20
- confirmed quality issue rate: 0.20

The score excludes verification status, trust status, star rating and popularity. Reported complaints are counted separately from confirmed/upheld vendor-fault outcomes from Support.

## Privacy And Accounting

Operational analytics are decision-support data, not audited accounting truth. Wallet remains the financial ledger and settlement authority.

Nearby price intelligence stores aggregate median and a coarse price range only
when all privacy gates pass:

- minimum 5 independent comparable external Vendors after excluding the
  requesting Vendor
- server-owned comparable product authority present
- server-owned geo bucket present, not caller-controlled precise radius
- normalized commercial unit
- bounded freshness/window reference
- low-sample cohorts suppressed

Competitor identity, exact competitor prices and confidential branch
identifiers are not stored or returned. Range values are coarse buckets, not raw
min/max contributor prices, to reduce differencing and filter-narrowing risk.

## Order Guarantee

ORDER_GUARANTEE_STATUS: `FUTURE_DISABLED`

Launch dependency: NO

Customer-facing: NO

Compensation enabled: NO

Wallet guarantee mutation: NO

Evidence readiness: PARTIAL, based on accepted-order, readiness, provider-arrival, promised-window, missing-item and exclusion evidence streams from existing operational sections.
