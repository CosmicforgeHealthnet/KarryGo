# Karryam Fleet: status against the Corporate Logistics Specification v2.0

Snapshot: 2026-11-13, branch `staging` at `9e3f4ba`. Service:
`services/fleet-partner-service` (the spec calls it the corporate logistics
service). The evidence for every row is in `spec-v2-audit/` (file references
below). The spec has two parts:

- **Part 1:** Sections 1-33 plus Appendices A-D. Overview and short requirements.
- **Part 2:** Sections 34-72. The detailed, binding version. Sections 37-56
  repeat the feature areas of 8-25 in more depth, and Sections 57-72 add the
  schema, state machines, full API catalogue, cross-service contracts, events,
  workers, errors, security, observability, tests, configuration, rollout,
  journeys and the release checklist.

Counts are requirements as each audit split them. I = implemented, P = partly,
M = missing, E = owned by another service. "Implemented" means real code and
storage. It may still be behind a feature flag that is off.

## 1. Headline

- **The feature backend is about three-quarters built** (Sections 8-25: 176
  implemented, 18 partly, 35 missing).
- **No end-to-end journey in Section 70 runs today.** Each one stops at a
  missing link between services, not at a missing screen:

  | Journey | Where it stops |
  |---|---|
  | Company onboarding | Never reaches ACTIVE. Verification decides `VERIFIED`, Fleet expects `APPROVED`, nobody reads the decision back, and nothing calls `TryActivate`. The wallet gate also needs a Wallet connection staging does not have. |
  | Managed provider | Only HAULING can be created. The membership never leaves PENDING_PROVIDER_ACCEPTANCE because nothing records the provider's acceptance, so no sub-wallet is requested. |
  | Marketplace offers | Opportunities never appear in production. Dispatch sends `platform.request.offered.v1`, but no production code creates the request projection or the offer. Hauling and Taxi send no offer events. The Dispatch execution API does not match Fleet's client. |
  | Private orders | Stop at CONFIRMED. Shipments never leave DRAFT. |
  | Assignments | Never reach IN_PROGRESS or COMPLETED from trip events. |

- **Fleet's own events reach no other service.** They go to a Redis stream that
  no service reads.
- **The machine-facing contracts mostly diverge from Part 2:**
  - **API paths:** the spec says `/api/v1/corporate-logistics`; the service
    uses `/fleet-partner/v1`.
  - **Error codes:** 47 of 58 Section 63 codes are absent.
  - **Event names and envelope:** they follow Part 1, not Section 61.
  - **Environment variables:** 40 of 90 are missing.

  These are decisions to take before building (Section 4).
- **The self-assessment files are wrong.** `docs/master-spec-release1-coverage.json`
  and `docs/release1-master-spec-acceptance.json` mark everything PASS. At
  least 7 of the 40 Section 71 items are not met.

## 2. Section-by-section status

### Part 1

| § | Title | I | P | M | E | What remains (evidence) |
|---|---|---|---|---|---|---|
| 1 | Purpose, scope, authority | 3 | 3 | 0 | 0 | Structure APIs, dispatch/taxi provisioning, reports/automation (A) |
| 2 | Frozen product decisions | 8 | 2 | 0 | 1 | Managed providers only for hauling (A) |
| 3 | Platform boundaries | 10 | 4 | 0 | 0 | No service consumes corporate events (A) |
| 4 | Target architecture | 12 | 0 | 0 | 0 | Done (A) |
| 5 | Repository and coding conventions | 8 | 4 | 0 | 0 | 13 placeholder feature dirs; concrete-type coupling (A) |
| 6 | API and data conventions | 11 | 3 | 0 | 0 | Money object unused; unknown-filter rejection only in CRM (A) |
| 7 | Multi-tenancy and isolation | 10 | 3 | 0 | 0 | Branch/department/settings have no API (A) |
| 8 | Auth, onboarding, lifecycle | 11 | 2 | 2 | 1 | Branch/structure APIs; settings API; activation pipeline (F, B). Password login deliberately not built (passwordless). |
| 9 | Users, roles, approvals | 9 | 2 | 4 | 0 | Staff PATCH, custom roles, role bindings, approval policies, delegations, branch-scoped roles (F, E) |
| 10 | Managed provider accounts | 7 | 1 | 8 | 0 | PATCH, reactivate, remove, resend login, capabilities, policies, availability, declines; dispatch/taxi contracts (F) |
| 11 | Acceptance, assignment, accept/decline | 12 | 1 | 0 | 0 | Unsafe-vehicle decline from marketplace assignments (F) |
| 12 | Marketplace and trip integration | 5 | 2 | 0 | 1 | Projection/offer creation; trip lifecycle projection (F, B) |
| 13 | Company wallet and sub-wallets | 9 | 1 | 0 | 1 | Provider self-service routes never mounted (F) |
| 14 | CRM, sales, contracts, SLAs | 10 | 0 | 1 | 0 | Leads and conversion (F) |
| 15 | Catalogue, orders, shipments, packages | 9 | 1 | 1 | 0 | Order PATCH/approve, order and shipment lifecycle commands (F, D) |
| 16 | Dispatch, routes, live control | 12 | 1 | 1 | 0 | Rescue cases; geofence-event read (F) |
| 17 | Workforce, shifts, training | 12 | 1 | 3 | 0 | Shift templates, training enrolments, provider self-view; performance only from private work (F) |
| 18 | Fleet, assets, maintenance, fuel | 10 | 1 | 1 | 0 | Defects, meter readings, tyres, unassign, asset cost history (F) |
| 19 | Compliance, safety, incidents, proof | 11 | 1 | 1 | 0 | Investigations, proof verification, compliance checks; incident transitions (F, D) |
| 20 | Specialized logistics | 11 | 2 | 1 | 0 | Capability schemas / order specialized data; taxi passenger (F) |
| 21 | Warehouse, yard, dock, terminal | 10 | 1 | 0 | 0 | Receipts with lines, put-away, pick waves, packing (F, E) |
| 22 | Finance, payroll, expenses, claims | 17 | 0 | 2 | 0 | Profitability, receivables ageing (F) |
| 23 | Communication, support, quality, SLA | 8 | 0 | 1 | 0 | Operational exceptions (F) |
| 24 | Analytics, reports, AI, automation | 2 | 1 | 11 | 0 | Reports, automation, recommendations, all AI (F) |
| 25 | Files, documents, search, imports, exports | 9 | 0 | 0 | 0 | Done (F) |
| 26 | Events, contracts, synchronization | 1 | 15 | 12 | 0 | Consumers for Fleet events, missing producers, event names (B) |
| 27 | Background jobs and scheduling | 8 | 8 | 4 | 0 | Scheduled-task runner never started; report/analytics workers (B) |
| 28 | Security, privacy, audit | 12 | 7 | 0 | 0 | Step-up on sensitive actions, session revocation on suspension, field encryption (C) |
| 29 | Reliability, observability | 9 | 5 | 0 | 0 | Histograms, trace export, point-in-time recovery (C) |
| 30 | Migrations and compatibility | 3 | 4 | 0 | 0 | Backfill jobs, migration lock, index strategy (D) |
| 31 | Testing strategy | 14 | 11 | 0 | 0 | Tests for the broken journeys, `-race`, secret scan (D) |
| 32 | Work packages and build order | 7 | 14 | 1 | 0 | WP3/5/6 exit conditions not met (D) |
| 33 | Release and acceptance gates | 7 | 11 | 2 | 0 | Marketplace writes not flag-gated; no per-org allowlist (D) |
| App. A | API catalogue (223) | 170 | 27* | 26 | - | *27 exist under another path (F) |
| App. B | Domain states (17) | 5 | 12 | 0 | - | Shipment, asset, work order, incident, invoice vocabularies (D) |
| App. C | Error codes (25) | 8 | 5 | 12 | - | (C) |
| App. D | Critical sequence flows (5) | 0 | 4 | 1 | - | D.1 provider, D.5 order break (D) |

### Part 2

| § | Title | I | P | M | E | What remains (evidence) |
|---|---|---|---|---|---|---|
| 34 | Binding scope and completion standard | 5 | 4 | 0 | 1 | Schema and route-registry gaps (A) |
| 35 | Service position and runtime | 23 | 10 | 3 | 0 | Seeds, automation module, ports (A) |
| 36 | Global API and data conventions | 12 | 7 | 2 | 0 | Canonical paths, `meta.version`, list `total` (A) |
| 37-42 | Organization, access, providers, marketplace, trips, wallet (detail) | 9 | 10 | 9 | - | Capability-level suspension, approval thresholds/delegations, membership end handling, TIP allocation (E) |
| 43-46 | CRM, orders, dispatch, workforce (detail) | 7 | 5 | 7 | - | Order with stops/shipments, pickup-before-delivery, manifest reconciliation (E) |
| 47-49 | Assets, maintenance, compliance (detail) | 6 | 13 | 3 | - | Asset RESERVED and other states, fuel amounts/anomalies, inspection template version (E) |
| 50-55 | Specialized, warehouse, finance, comms, integrations (detail) | 10 | 4 | 4 | - | Payroll calculation hash, step-up on sensitive exports (E) |
| 56 | Security, privacy, audit, retention, legal holds | 18 | 8 | 7 | 0 | Privacy requests, session risk, `/security-events`, 56.4 events (C) |
| 57 | Canonical PostgreSQL schema (142 tables) | 104 | 12 | 26 | - | 26 tables missing: teams, cost centres, locations, leads, provider availability, shift templates, enrolments, defects, receipts, reports, automation... (A) |
| 58 | Enforced state machines | 1 | 10 | 2 | 0 | Shipment transitions, warehouse receipt, incident matrix, separation of duties (D) |
| 59 | API catalogue (250) | 90 exact + 72 equivalent | 30 | 58 | - | Full list in `spec-v2-audit/section59-endpoints.tsv` (E) |
| 60 | Cross-service contracts | 3 | 10 | 3 | - | Dispatch execution contract, verification, provider resolve for dispatch/taxi (B) |
| 61 | Event architecture and catalogue | 0 | 11 | 3 | - | Envelope fields, Part 2 event names, missing domain events (B) |
| 62 | Durable workers and recovery | 9 | 13 | 7 | - | Offer expiry, scheduler, fuel anomaly, reports, analytics (B) |
| 63 | Error code catalogue (58) | 7 | 4 | 47 | - | Pick one catalogue (see Section 4) (C) |
| 64 | Security, visibility, audit controls | 17 | 22 | 1 | - | Field visibility shaping, KMS, audit minimum fields (C) |
| 65 | Observability, SLOs, runbooks | 7 | 28 | 18 | - | `/version`, dependency status, SLOs, alerts, 5 runbooks, RPO (C) |
| 66 | Mandatory tests (103) | 37 | 53 | 13 | - | (D) |
| 67 | Environment contract (90 vars) | 3 exact + 47 renamed | - | 40 | - | JWT/JWKS, KMS, event transport, pricing mode (D) |
| 68 | Migration and rollout plan | 8 | 19 | 4 | - | R2-R12 flags for onboarding/provisioning/marketplace writes (D) |
| 69 | Work packages and definition of done | 7 | 17 | 2 | - | WP23, WP24 (D) |
| 70 | End-to-end journeys (12) | 0 | 10 | 2 | - | See headline (D) |
| 71 | Release readiness checklist (40) | 12 | 23 | 5 | - | (D) |
| 72 | Final implementation authority | 0 | 1 | 0 | - | Deviations need ADRs (D) |

Evidence files (in `spec-v2-audit/`): A = `A-foundations.md`, B =
`B-events-integrations.md`, C = `C-security-errors-ops.md`, D =
`D-states-tests-rollout.md`, E = `E-api-and-detail-sections.md`, F =
`F-sections-8-25.md` (Sections 8-25 audit; rows 8.10, 9.3, 9.4 and 21.11
have since been updated as in the table above).

## 3. What was done on 2026-11-13 (this session)

- Staff management made real: directory, staff detail, invitations
  (list/resend/cancel), the roles and permission catalogue, and web pages.
- Automatic Head Office branch on company creation, with backfill (migration 000087).
- Workforce JSON contract; payroll and compensation date fixes.
- 404 instead of 500 on several lookups.
- New trips pages, CRM contact/location forms, broken links fixed.
- 95 Fleet module settings switched on in staging through
  `deploy/platform/fleet-staging-modules.txt`.
- Details: `fleet-end-to-end-activation-report.md`.

## 4. Decisions needed before the remaining build

1. **Company approval.** Who approves a company: an operator in the admin
   console (spec 70 step 12) or something else? Is staging auto-approved?
   And should the wallet gate be relaxed on staging until Wallet is connected?
2. **API path.** Keep `/fleet-partner/v1` or move to the binding
   `/api/v1/corporate-logistics`? The internal path is hard-coded in the
   dispatch, taxi and hauling services and in shared clients.
3. **Error catalogue.** Section 63 and Appendix C disagree. Pick one.
4. **Event transport.** Per-target signed HTTP delivery (as Dispatch,
   Hauling and Wallet already do) or a shared stream. Also: Part 1 or Part 2
   event names.
5. **Commercial values.** Pricing, commission and subscription stay
   undecided (Section 72), so no formulas will be coded.
6. **AI capabilities (Section 24/54).** Deferred by ADR-002. Confirm they
   stay deferred, or which provider and model to use.

## 5. Proposed build order

1. **Company activation pipeline** (unblocks every operational module).
2. **Managed providers end to end:** dispatch/taxi resolve-or-provision,
   association acceptance, sub-wallet.
3. **Marketplace:** projection and offer creation, Dispatch contract
   alignment, Hauling/Taxi offer events, offer expiry, assignment lifecycle
   from trip events.
4. **Orders and shipments:** lifecycle commands and execution projection.
5. **Missing APIs:**
   - structure (branches, locations, departments, teams, cost centres, settings);
   - roles and approvals;
   - provider actions;
   - leads;
   - shift templates and enrolments;
   - defects, meter readings, tyres;
   - warehouse receipts, put-away and picking;
   - rescue cases and exceptions;
   - profitability and receivables ageing.
6. **Events:** envelope, names, delivery to consumers, and the missing producers
   in other services.
7. **Security and operations:** step-up coverage, privacy requests, security
   events, error catalogue, histograms, `/version`, runbooks.
8. **Reports and automation (Section 24/54).**
9. **Section 66 tests, Section 71 checklist, environment contract.**

Frontend work for the provider and customer apps, and changes owned by other
services' teams, are listed in `fleet-spec-v2-handoff-brief.md`.
