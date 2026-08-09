# Section 30 — Original API Mapping

## Evidence basis (read this before the table)

The Section 30 mandate asks for the original single-service Fleet API's
endpoints to be mapped onto the current corporate architecture. An
exhaustive repository search — `docs/`, `services/fleet-partner-service/docs/`,
every markdown/spec/design directory, and full-repository greps for
`"Fleet API"`, `"original fleet"`, `"fleet draft"` — found **no separate
"original Fleet API draft" specification document anywhere in this
repository.** Sections 13–29 built fleet-partner-service as a genuinely new
service (confirmed by `docs/prerequisite-stabilization-report.md`), composing
functionality directly against the corporate organization/provider-membership
model from the start, not by migrating forward from a prior single-owner
Fleet service that once existed in this codebase.

Per the mandate's own instruction — "Do not invent original endpoints that
cannot be found in repository evidence" — this document does not fabricate
specific original endpoint paths. Instead, the **ORIGINAL** column below
records the generic single-service fleet-management *concept* the user's own
mandate named (the ten areas listed verbatim in the mandate: Auth and owner
onboarding, Dashboard, Requests and trips, Drivers, Vehicles, Verification,
Wallet and transactions, Disputes and support, Account delete), and the
closest concrete repository evidence for what such a concept would have meant
is `services/fleet-partner-service/README.md`'s own "What it never owns"
section — the boundary list this service's authors wrote to explicitly reject
recreating those concepts locally:

> Canonical customer identity, provider identity, trip lifecycle, wallet
> ledgers, settlements, verification decisions, platform support truth,
> platform analytics truth, KarryGo internal-admin accounts, Partner API
> credentials... no accounts receivable... A CRM customer is a commercial
> relationship record, never a marketplace identity.

The **CURRENT** column records real, currently-mounted routes and owning
packages, verified by direct inspection of this repository's route
registration code (`grep` over every feature's `http` package) at the time
this document was written — not aspirational or planned routes.

## Mapping table

| # | Original concept (mandate-supplied) | Original intent | Current replacement | Canonical owning service/package | Compatibility behavior | Deprecated behavior | Response compatibility | Migration status | Removal policy |
|---|---|---|---|---|---|---|---|---|---|
| 1 | Auth and owner onboarding | A single fleet owner authenticates and onboards their fleet | `POST /fleet-partner/v1/auth/{start,verify,resend,refresh,logout}`, `POST /fleet-partner/v1/auth/select-organization`, `POST /fleet-partner/v1/auth/step-up/{start,verify}`, `POST /fleet-partner/v1/organizations`, `GET .../onboarding-status`, `POST .../submit` (`internal/features/auth/http`, `internal/features/organizations/http`) | Corporate identity + organization onboarding replaces "individual vs fleet" account-type choice with organization membership/roles from the first request | N/A — greenfield; no legacy identity model existed in this service to preserve | N/A | Never implemented as a single-owner model in this repository | Complete | N/A — nothing to remove |
| 2 | Dashboard | A single operational summary view for the fleet owner | Organization-scoped read models: `GET /fleet-partner/v1/organizations/current`, plus per-domain projection reads (trip projections, finance projections, tracking projections — each its own feature's `GET` routes) and delegated Wallet reads (`internal/features/companywallet/http`) | Read models are organization-scoped, sourced from event-driven projections of the owning execution/wallet services, never a locally-computed cross-domain aggregate that could drift from source truth | Projections carry `FreshnessState`/`SourceVersion` so a stale read is visible to the caller, not silently presented as current | Additive only — a dashboard read gaining a new projection field never removes an existing one without a cleanup release (WP5) | N/A | Complete for the projections that exist; a single "one-call dashboard aggregate" endpoint does not exist and is not required by any evidenced original contract | N/A |
| 3 | Requests and trips | The fleet owner views and manages delivery/ride requests and trip execution | `internal/features/marketplace/http` (organization-facing request/assignment views) and `internal/features/marketplace/http/internal_handlers.go` (`GET /providers/:providerId/{offers,assignments,memberships}` — provider-facing internal reads) | Canonical execution state remains in the owning execution service (driver-hauling-service / driver-dispatch-delivery-service / driver-taxi-service); fleet-partner-service holds `fleet_partner_marketplace_request_projections`/`fleet_partner_trip_projections` as read-side projections, never a competing source of truth (see README's "what it never owns") | Old clients reading a request/trip projection see the same shape regardless of which execution service authored the underlying event | A write (accept/decline/assignment) always routes to the authoritative execution service via `internal/platformclients`, never mutates the local projection directly and calls it done | Projection freshness fields are additive; core status/id fields are stable | Complete — projections exist for all three execution services' assignment/offer flows | N/A |
| 4 | Drivers | The fleet owner creates and manages individual drivers directly | `internal/features/providers` (`models`/`repositories`/`usecases`) plus organization-facing `POST/GET /fleet-partner/v1/providers`, `.../:membershipId/{suspend,provisioning-status,history}` (`internal/features/organizations/http/handlers.go`) | "Driver creation" maps to: resolve authoritative provider identity in the owning service → provision/associate where permitted → create an organization membership (`fleet_partner_organization_provider_memberships`) → reconcile sub-wallet linkage. **No unverified local driver identity is ever created** — `ProvisioningSaga` tracks the multi-step handoff and a partially-provisioned provider is never treated as active | A caller-supplied provider identity is never trusted directly; `platformclients.ContractSet` resolves it against the owning driver service first | Same JSON membership shape regardless of whether the provider was resolved-existing or organization-sponsored-provisioned | Complete | N/A |
| 5 | Vehicles | The fleet owner registers and tracks vehicles directly | `internal/features/assets` (asset lifecycle: `fleet_partner_assets`, inspections, maintenance, breakdowns, safety blocks — migrations 000040/000041) | Original vehicle operations map to asset lifecycle; where trip execution requires a platform vehicle, the asset stores an authoritative external vehicle reference and reconciles verification state with the owning service rather than asserting its own verification decision | A legacy vehicle identifier, if ever presented, is only accepted when it safely resolves to a real asset reference — never silently reinterpreted as an unrelated ID | Asset operational/safety state (inspections, breakdowns, return-to-service decisions) is local and authoritative; vehicle *verification* is not | Complete | N/A |
| 6 | Verification | The fleet owner's business, and their drivers/vehicles, are verified | Organization business verification: `GET /fleet-partner/v1/organizations/:organizationId/verification` (`internal/features/organizations/http`, backed by `internal/features/organizations/usecases/submission_activation.go`). Provider/vehicle verification: delegated, read via `internal/platformclients.VerificationClient.ReadOrganizationStatus` and the owning services' own verification truth | Three verification concepts are kept explicitly distinct, never collapsed into one ambiguous "verified" flag: (a) organization business verification — Fleet-orchestrated and stored locally; (b) provider identity/compliance verification — authoritative in the owning driver service; (c) vehicle/asset verification — authoritative in the owning driver service, reconciled locally | A stale/unknown remote verification read surfaces as its own state (`UNKNOWN_OUTCOME`/reconciliation-required), never silently coerced to "verified" or "unverified" | Response always names WHICH of the three verification concepts it describes | Complete | N/A |
| 7 | Wallet and transactions | The fleet owner views and manages a wallet with balance/transaction history | `GET /fleet-partner/v1/wallet`, `.../wallet/transactions[/:transactionId]`, `.../wallet/policy`, plus provider sub-wallet: `.../providers/:membershipId/sub-wallet[/statement|/allocations|/withdrawals]` (`internal/features/companywallet/http`) | A facade/projection over payment-wallet-service (`fleet_partner_wallet_transaction_projections`, `fleet_partner_finance_projections`). **Fleet never creates a local ledger** — every balance figure traces to a payment-wallet-service source reference; see README's "no accounts receivable" framing | Independent-provider settlement (via the owning driver service's own wallet integration) is entirely untouched by any of these routes — they only exist for organization-managed sub-wallet allocation | Provider-facing sub-wallet fields never include the organization's own wallet balance or another provider's allocation | Provider-facing routes (`.../provider/sub-wallet*`) are additive to the organization-facing ones, not a replacement | Complete | N/A |
| 8 | Disputes and support | The fleet owner raises and tracks support cases/disputes | `POST /internal/.../support/events` (support-event ingestion, `internal/features/engagement/http.go`), `GET/POST .../escalations[...]` (SLA escalation read/acknowledge/resolve) | Support-dispute-service is the authoritative case system for anything crossing into customer/provider-facing disputes; `fleet_partner_quality_cases`/`fleet_partner_escalations` are the proper LOCAL domain for company-internal quality tracking and SLA escalation, which are not the same thing as a support-dispute-service case | A legacy support-case ID is never resolved against a local quality-case ID by coincidence of format — the two are typed/sourced separately (`SourceService`/case-source discriminator) | Escalation/quality-case responses are additive-only across the `escalations.*` permission family | Complete for the local quality/escalation domain; the mandate's cross-service support-dispute-service facade is inherent in `internal/features/engagement`'s event ingestion, not a separate proxy read surface (no evidence a proxy read was ever required by an original contract) | N/A |
| 9 | Account delete | The fleet owner deletes their account, expecting immediate removal | `POST /fleet-partner/v1/organizations/:organizationId/close-request`, `.../closure-status`, `.../closure/{assess,approve,reject,execute}` (`internal/features/organizations/http/handlers.go`, backed by `internal/features/organizations/usecases/closure.go`) | Any legacy "delete account" intent maps to the completed organization closure workflow, never immediate destructive deletion. Closure requires auth, requires org authority, respects retention/legal holds (`fleet_partner_legal_holds`, Section 28), and revokes access per the closure workflow rather than dropping rows | A request literally named "delete" is documented (this row) to mean **controlled closure**, not physical deletion — the workflow returns a closure status/reference, exactly the compatibility shape WP32 requires | The `close-request`/`closure-status`/`closure/*` routes are the ONLY account-removal surface; there is no separate legacy "hard delete" route to keep compatible because none is evidenced to have ever existed | Complete | N/A |

## Notes on scope and what this table deliberately does not claim

- Every "CURRENT" cell above was verified against actual route-registration
  code in this repository at Section 30 implementation time, not inferred
  from feature names. Paths are quoted exactly as they appear in the
  handler-registration source.
- Rows 1–9 correspond one-to-one with the ten areas enumerated in the
  Section 30 mandate's own WP15 table (the tenth, "Requests and trips," was
  supplied as a single combined row by the mandate and is kept combined
  here as row 3).
- No row claims a specific original HTTP method/path that cannot be traced
  to either (a) a real current route, evidenced by inspection, or (b)
  README.md's own explicit architectural boundary statement. Where neither
  existed, the row says so plainly rather than inventing detail.
- This document is descriptive of the mapping as of Section 30's
  implementation. If a future section adds, renames, or removes any of the
  CURRENT-column routes, this table must be updated in the same change —
  it is not machine-verified against the live route manifest the way
  `section29contract`'s OpenAPI parity tests are (a possible future
  hardening, not attempted here to avoid inventing a large new contract
  surface beyond what Section 30 requires).
