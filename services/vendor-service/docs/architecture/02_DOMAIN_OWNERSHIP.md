# 02 — Domain Ownership

Restates the specification's business-ownership rules as the concrete mapping this codebase follows. This is the same table CLAUDE.md keeps for the existing services, extended for Vendor Service; a future session should append to CLAUDE.md's own "Business Ownership" convention rather than maintaining two competing lists.

Section 6 implements the vendor business aggregate: Business is the tenant root; capabilities and brands belong to one business; branches belong to one business and may reference only a same-business brand; branch capabilities may reference only a same-business capability. Parent status gates effective behavior without mutating child lifecycle state.

| Domain | Owning service | Notes |
|---|---|---|
| Customer auth/profile/preferences/history | `customer-service` | Unchanged. |
| Taxi booking, matching, trip lifecycle | `driver-taxi-service` | Unchanged (currently a scaffold — see [00](00_REPOSITORY_AUDIT_BASELINE.md)). |
| Package delivery booking, rider matching, delivery lifecycle, proof of delivery | `driver-dispatch-delivery-service` | Unchanged. |
| Truck/van haulage booking, matching, cargo workflow | `driver-hauling-service` | Unchanged. Van stays here — never a separate service. |
| Payments, wallets, ledger, withdrawals, refunds, Paystack | `payment-wallet-service` | Unchanged. Vendor Service's checkout allocations are recorded here, not duplicated. |
| Notifications | `notification-service` | Unchanged. |
| File uploads and media metadata | `media-file-service` | Unchanged. |
| Verification/compliance | `verification-compliance-service` | Unchanged in principle; currently mostly scaffold, vendor-business verification is future work (see [PLATFORM_SERVICE_CHANGE_REGISTER.md](../PLATFORM_SERVICE_CHANGE_REGISTER.md)). |
| Support/disputes | `support-dispute-service` | Unchanged; a new vendor caller group is future work. |
| Admin operations | `admin-backoffice-service` | Unchanged; currently a pure scaffold. |
| Analytics | `analytics-service` | Unchanged; currently a pure scaffold. |
| Fleet/company provider relationships | `fleet-partner-service` | Unchanged. Vendor Service never reads or writes its tables. |
| **Vendor business management, staff IAM** | **`vendor-service`** | New. `internal/features/vendors`, `capabilities`, `brands`, `branches`, `staff`, `auth`. |
| **Vendor CRM** | **`vendor-service`** | New. `internal/features/crm`. References `customer-service`'s customer_id as an opaque external id only. |
| **Restaurant/Food, Cloud Kitchen, Pastry/Bakery, Shop/Grocery/Market/Wholesale catalogues** | **`vendor-service`** | New. `internal/features/food`, `cloud_kitchen`, `pastry`, `shop`, `catalog`. |
| **Vendor inventory, suppliers, purchasing, goods receiving, recipes/production** | **`vendor-service`** | New. `internal/features/inventory`, `purchasing`, `recipes`, `production`. Distinct from `fleet-partner-service`'s warehouse inventory concept — see [01_SERVICE_BOUNDARIES.md](01_SERVICE_BOUNDARIES.md). |
| **Commerce cart/checkout, parent commerce orders, vendor orders, substitutions, promotions** | **`vendor-service`** | New. `internal/features/orders`, `substitutions`, `promotions`. |
| **Vendor-owned delivery, commerce fulfilment planning** | **`vendor-service`** | New. `internal/features/delivery` (vendor-owned riders only — not a platform provider marketplace), `fulfilment` (orchestrates dispatch/taxi/hauling execution requests; never assigns a platform provider itself). |
| **Vendor operational reports/projections** | **`vendor-service`** | New. `internal/features/analytics` (vendor's own dashboards) and `finance_projection` (read-model over `payment-wallet-service`'s authoritative allocations — never a competing ledger). |

## Section 8A — CRM relationship ownership

Vendor Service owns the **vendor-local commercial relationship**: the CRM profile lifecycle, commercial summary projections derived from authoritative events, branch/capability relationship projections, acquisition references, the local consent projection, the local privacy projection and its application, the local identity-link/merge projection, the Customer 360 read model, and the CRM audit/outbox/inbox state.

customer-service remains authoritative for canonical customer identity, authentication, canonical profile and names, canonical email and phone, saved addresses, credentials, identity merge decisions, canonical privacy status, and marketing consent wherever that authority comes to live.

The dividing line is deliberate and load-bearing: **Vendor Service must not become a second customer master.** It therefore stores opaque, authority-owned subject references and no canonical contact data of any kind, and does not treat a hash of a customer reference as an anonymised identity — a hash is still a stable identifier.

Financial authority is unaffected. CRM commercial figures are a read-model projection of order facts, not a wallet balance, receivable, settlement or credit line.
