# Promotions

Open **More → Promotions**. This workspace uses the seven existing public promotion operations. No new route, promotion field, sales statistic or payment authority is added.

## Flow

1. Browse offers and filter by the actual campaign statuses.
2. Create an offer, select the audience, dates and limits, then review the vendor-funded discount. Creation saves a `DRAFT`; it does not automatically activate an offer.
3. Open Offer details to edit the definition, activate/schedule/pause/end/archive using the allowed backend transitions, create a coupon, or read usage.
4. Coupon creation confirms an opaque `coupon_id`. Copy the entered code from the confirmation. The backend has no public coupon listing, plaintext retrieval, revocation or editing operation, so those controls are absent.

All eight campaign types are exposed: percentage, fixed, coupon, bundle, quantity break, free delivery, delivery percentage and delivery fixed discounts. Bundle items and item/variant audience targets can be selected from the existing Food/Shop catalogue. Branch options come from the authorized business branch list. Other advanced audience targets accept existing capability/category/customer-segment references; richer discovery belongs to the corresponding unfinished taxonomy/CRM workspaces.

Dates are displayed in the device timezone and submitted as UTC. Currency is read from the business where access permits and remains an explicit input. Money is converted to the app's integer minor-unit convention; percentages use integer basis points. Tier ranges must be consecutive and each tier has one percentage or fixed discount.

## Backend authority

- `GET /api/v1/vendor/promotions` returns `{promotions: [...]}`; a null list is treated as empty.
- `POST /api/v1/vendor/promotions` creates a vendor-funded draft.
- `GET /api/v1/vendor/promotions/{promotionId}` reads the complete campaign and version.
- `PATCH /api/v1/vendor/promotions/{promotionId}` replaces the editable definition with `If-Match`.
- `POST /api/v1/vendor/promotions/{promotionId}/status` uses `If-Match` and the actual transition table.
- `POST /api/v1/vendor/promotions/{promotionId}/coupons` creates a code with optional usage limits.
- `GET /api/v1/vendor/promotions/{promotionId}/usage` returns the count of `RESERVED` and `CONSUMED` redemptions. This is not revenue or completed-order analytics.

Definitions are configured with `funding_sponsor: VENDOR` and `vendor_funding_bps: 10000`. Existing platform/combined definitions are readable but cannot be edited into a vendor-funded definition. Status/coupon operations remain subject to their existing server permissions and rules. The client cannot approve platform spending or apply discounts to checkout itself.

Backend access fixes accompany the workspace: list reads now follow detail branch visibility; updating a saved definition checks its existing branch reach and vendor sponsor; status and coupon writes require management coverage of all targeted branches. A branch-scoped manager cannot change a shared campaign covering an unauthorized branch or a business-wide campaign. Duplicate coupon codes return a clear terminal 422 validation response rather than an ambiguous server failure.

## Mock, Live and recovery

Mock supplies distinct active, paused and draft offers and implements all seven routes, version conflicts, coupon uniqueness and idempotent write replay. Its usage figures are fixtures. Mock writes affect only the selected mock business. Live calls the existing server through the generated contract API, without fixture fallback.

Before a write, the client persists an immutable operation, body, target, original idempotency key and applicable version. Storage is isolated by mode, server, vendor and staff; Live uses the existing secure preference storage. An uncertain result blocks additional writes and offers an explicit retry of the saved update. Recovery is available after reopening Promotions. Storage failure before submission blocks the request; version conflicts require fresh reads. Accepted writes with failed local cleanup are shown as recovery pending rather than an unqualified success.

Live requires the server's promotion routes, quote-lock feature flag, migration 97, permission grants and lifecycle worker. This implementation has not been certified against a deployed Live server. Mock does not run the server lifecycle worker or checkout/redemption engine.

## Validation

Five focused frontend checks cover duplicate-code rejection without stuck recovery, a lost coupon response across reopening, identical replay, stale-version refusal, quantity-tier create/edit contracts and 360×800 dark-mode list/detail/editor layout. Backend promotion tests include covered/uncovered/shared campaign branch access and duplicate-code error translation.

Sources: `internal/features/promotions/http.go`, `service.go`, `campaign.go`, `calculation.go`, migration `000097_promotion_campaigns_and_delivery_funding.up.sql` and the composition in `cmd/main.go`.
