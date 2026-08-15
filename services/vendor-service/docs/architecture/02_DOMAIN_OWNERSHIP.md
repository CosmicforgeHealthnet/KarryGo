# 02 — Domain Ownership

## Section 8H ownership

Section 8H owns **what this vendor infers about its own relationships**, and the files it produces from its own CRM data. That is a narrow claim deliberately: a score is a derived opinion computed from facts other sections own, and an export is a projection of tables other sections own.

It does not own the **facts**. Commercial projections belong to Section 8A, which projects the authority's data; loyalty to 8D; campaign results to 8F; corporate relationships to 8G. Section 8H reads them and writes nothing back.

It does not own **permission**. This is the boundary that matters most: consent, privacy state, marketing suppression, frequency and quiet hours are owned by Sections 8A, 8E and 8F. A score is not consulted to decide any of them and does not weaken any of them, and there is deliberately no code path from a score to a send.

It does not own **storage**. Export artifacts live in media-file-service's private object contract. Vendor Service holds no files, serves no bytes and stores no URL, because the alternative to a private store is not a local file — it is no download at all.

It does not own **truth about the future**. RFM and Churn V1 are heuristics, and the contract says so in the schema, the descriptions and a test. A model that claimed to predict would need validation this section does not have and does not pretend to.

## Section 8G ownership

Section 8G owns the **commercial relationship between one vendor and one organization**: its identity and vendor-local `CORP-` reference, its lifecycle, which of the vendor's branches deal with it, who its people are in relation to the vendor, where its sites are, and how the vendor labels its internal cost centres. That record exists only inside the owning vendor's tenancy; nothing about it is platform-wide.

It does not own the **people**. A corporate contact is a reference to a Section 8A customer relationship, which in turn projects customer-service's authority. No contact detail is copied here, and the merge and redaction behaviour is Section 8A's, reached through the existing `PrivacyHandler` port rather than reimplemented.

It does not own **money, orders or agreements**. There is no corporate order, invoice, credit line, budget, contract or quotation, and a cost centre is a label with no budget, limit, currency or balance. `payment-wallet-service` remains the only financial authority, and a future corporate-ordering domain must define its own authority rather than reading it into a CRM label — which is exactly why `MEMBERSHIP_ROLE=APPROVER` approves nothing.

It does not own **branches**. Branch scope references Section 6's branch graph and is validated against it; a corporate account can point at branches, never define them.

The dividing line from Section 8C holds in the other direction: corporate interactions and tasks are Section 8C's records with a corporate subject, not a second activity domain. The engine, vocabularies, bounds, state machine and immutable-correction model stay owned by 8C.

## Section 8E ownership

Vendor Service owns campaign definitions, immutable campaign versions, audience selection, audience preview, immutable audience snapshots, campaign targets, snapshot-time consent evidence, experiment cohort assignment, and campaign policy definition (frequency and quiet hours).

It does not own delivery. notification-service remains the authority for every channel, every delivery receipt and every provider retry, and Section 8E contains no code that could contact it. It does not own consent: customer-service remains the authority, and Section 8E only records what that authority's projection said at one moment, as evidence, with a mandatory recheck attached.

The audience itself is owned by Section 8B. Campaigns select through the segment engine and have no second way to reach a customer, which is what keeps one set of branch, privacy and consent walls rather than two that could disagree.

The boundary that most needs stating: **a prepared audience is not permission to contact anybody**. Section 8E can say who a campaign would target and what consent looked like when it was frozen. Only Section 8F, revalidating against current consent immediately before a delivery handoff, can decide that a message may actually be sent.

## Section 8D ownership

`crm_loyalty_program_configs`, `crm_loyalty_accounts`, `crm_loyalty_entries`, `crm_loyalty_point_lots` and `crm_loyalty_lot_consumptions` are Vendor-owned loyalty POINTS, linked only to the Section 8A opaque customer relationship. They are not money and do not make Vendor a second financial authority: there is no currency, no minor-unit amount, no wallet reference and no conversion path in the schema or the API. `payment-wallet-service` continues to own every naira on the platform, and a vendor granting 500 points is a marketing decision recorded here, not a transfer of value.

Vendor owns the programme configuration, the balance, the ledger, expiry, reversal and manual adjustment. It does not own who the customer is: privacy and identity-merge decisions arrive from `customer-service` through the existing Section 8A ingress, and loyalty obeys them through the same registry every other CRM domain uses. Redaction makes the account non-actionable and hides it; the ledger is retained because it holds integer movements and vendor-local references with no personal data to erase, and because it is the evidence that a balance was legitimately granted and spent.

Vendor does not own what an order is worth in points. The earning and redemption engines exist as internal primitives, but no formula converting money to points lives anywhere in this service — the Vendor Order domain that would define one is Section 9 work, and inventing a rate here would have committed the platform to a number nobody has decided.

## Section 8C ownership

`crm_interactions` and `crm_tasks` are Vendor relationship workflow, linked only to the Section 8A opaque customer relationship. Redaction suppresses ordinary display and makes open work non-actionable; identity merge moves history and work to the canonical relationship under the shared profile lock. Vendor does not reinterpret the authority's privacy state, transfer canonical PII, or turn an escalated interaction into a formal support/dispute decision.

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

## Section 8B ownership

Vendor tags and segments are Vendor Service's own classification of its own commercial relationships, so they are owned here outright — unlike consent, which belongs to the customer and is only projected.

That difference is visible in the merge behaviour: on an identity merge, tags and explicit segment membership **transfer** to the surviving relationship, because the vendor decided those things about the human and a merge does not change that decision. Consent does not transfer, because it belongs to the identity and the authority re-asserts it. Derived segment membership transfers neither way: it is recomputed, because copying it would assert a match the rule was never asked about.

The dividing line from Section 8A holds: segmentation reads the projections, and stores no second copy of a profile, a consent decision, a privacy state or an order fact.
