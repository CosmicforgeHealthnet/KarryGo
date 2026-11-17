# Fleet Sections 21-72: dependency map for the next phase

Date 2026-11-17. Branch `staging`. Scope source: Corporate Logistics
Specification v2.0 as audited in `fleet-spec-v2-status.md` (snapshot
2026-11-13) and `spec-v2-audit/`. Sections 5-20 engineering is closed
(`fleet-sections-05-20-final-closure.md`); this map says which of Sections
21-72 can start now and which wait on an outstanding staging activation.

When the next specification document arrives it replaces the row contents
below as the authoritative scope; the activation dependencies stay valid
because they are properties of staging, not of the spec text.

## 1. Outstanding activations (the only things anything below can wait on)

| Id | Activation | Owner | Unblocks |
|---|---|---|---|
| A1 | Settings files for Wallet, Taxi, Hauling, Customer, Dispatch owned by `deploy` (mode stays 600) | Server root | Deploy + pairing of Wallet, Taxi, Hauling, Customer |
| A2 | Dispatch 070 stop/migrate/verify/start (named operator, separate reviewer, window) | Release operator + reviewer | Dispatch on current code; Fleet decision relay with read-back |
| A3 | Authorized staging OTP inbox for signed-in journeys | Owner | Every live journey certification |
| A4 | Wallet gate waiver removed (after A1 and an ACTIVE organization wallet read back) | Engineering, after A1 | Activation without waiver; sub-wallets live |

Engineering never waits on these. Only **live certification** does. Every
section below can be built and proven against disposable PostgreSQL now.

## 2. Map

Legend. **Start now**: Fleet-internal, buildable and certifiable on
disposable infrastructure and the Fleet staging runtime that is already live.
**Build now, certify after**: buildable now; live proof needs the listed
activation.

### Part 1 (Sections 21-33)

| § | Area | Remaining scope (status doc) | Engineering | Live certification needs |
|---|---|---|---|---|
| 21 | Warehouse, yard, dock, terminal | Receipts with lines, put-away, pick waves, packing | Start now | A3 |
| 22 | Finance, payroll, expenses, claims | Profitability, receivables ageing; payroll vs work-level double-pay (CMP-027) | Start now | A3; settlement postings A1 + A4 |
| 23 | Communication, support, quality, SLA | Operational exceptions | Start now | A3 |
| 24 | Analytics, reports, AI, automation | Reports, automation, recommendations (AI deferred, ADR-002) | Start now (reports, automation) | A3 |
| 25 | Files, documents, search, imports, exports | Done | - | A3 (export journey) |
| 26 | Events, contracts, synchronization | Consumers for Fleet events, missing producers, Section 61 names | Start now (Fleet side, signed outbox per ADR-005) | Wallet/Taxi/Hauling consumers A1; Dispatch producer A2 |
| 27 | Background jobs and scheduling | Scheduled-task runner, report/analytics workers | Start now | none (Fleet worker is live) |
| 28 | Security, privacy, audit | Step-up coverage, session revocation on suspension, field encryption | Start now | A3 |
| 29 | Reliability, observability | Histograms, trace export, PITR | Start now | none for metrics; PITR is server work |
| 30 | Migrations and compatibility | Backfills, migration lock, index strategy | Start now | none |
| 31 | Testing strategy | Journey tests, `-race`, secret scan | Start now | - |
| 32 | Work packages and build order | WP3/5/6 exit conditions | Build now, certify after | A1, A2 |
| 33 | Release and acceptance gates | Flag-gated marketplace writes, per-org allowlist | Start now (flags, allowlist) | A1, A2 for marketplace proof |

### Part 2 (Sections 34-72)

| § | Area | Remaining scope | Engineering | Live certification needs |
|---|---|---|---|---|
| 34-36 | Scope, runtime, API conventions | Seeds, route registry, `meta.version`, list `total` | Start now | none |
| 37-42 | Organization, access, providers, marketplace, trips, wallet (detail) | Capability suspension, approval thresholds/delegation, membership end, TIP allocation | Build now, certify after | A1 (Taxi/Hauling/Wallet), A2 (Dispatch), A3, A4 |
| 43-46 | CRM, orders, dispatch, workforce (detail) | Orders with stops/shipments, pickup-before-delivery, manifest reconciliation | Start now | A3; execution handoff A1/A2 |
| 47-49 | Assets, maintenance, compliance (detail) | Asset states, fuel anomalies, inspection template versions | Start now | A3 |
| 50-55 | Specialized, warehouse, finance, comms, integrations (detail) | Payroll calculation hash, step-up on sensitive exports | Start now | A3 |
| 56 | Security, privacy, retention, legal holds | Privacy requests, session risk, `/security-events` | Start now | A3 |
| 57 | Canonical schema | 26 missing tables | Start now | none |
| 58 | State machines | Shipment, warehouse receipt, incident matrix, separation of duties | Start now | A3 |
| 59 | API catalogue | 58 missing endpoints (`spec-v2-audit/section59-endpoints.tsv`) | Start now | A3 |
| 60 | Cross-service contracts | Dispatch execution contract; Dispatch/Taxi provider resolve | Build now, certify after | A2 (Dispatch), A1 (Taxi) |
| 61 | Event catalogue | Envelope fields, Part 2 names, missing domain events | Start now (Fleet producers) | consumers A1, A2 |
| 62 | Durable workers | Offer expiry, scheduler, fuel anomaly, reports | Start now | offer expiry live proof A1/A2 |
| 63 | Error catalogue | 47 of 58 codes (ADR-004: Section 63 wins) | Start now | none |
| 64 | Visibility and audit controls | Field shaping, KMS, audit minimum fields | Start now | A3 |
| 65 | Observability, SLOs, runbooks | `/version`, dependency status, SLOs, alerts, runbooks | Start now | none |
| 66 | Mandatory tests (103) | 13 missing, 53 partial | Start now | journey tests A1-A3 |
| 67 | Environment contract (90 vars) | 40 missing | Start now | pairing changes A1 |
| 68 | Migration and rollout plan | R2-R12 flags | Start now | A1, A2 to exercise |
| 69 | Work packages / definition of done | WP23, WP24 | Build now, certify after | A1-A3 |
| 70 | End-to-end journeys (12) | All 12 | Build now, certify after | A1, A2, A3, A4 |
| 71 | Release readiness checklist (40) | 5 missing, 23 partial | Build now, certify after | A1-A4 |
| 72 | Final implementation authority | Deviations need ADRs | Start now | none |

## 3. Recommended order for the next phase

1. Contract foundations that everything else builds on and that need no
   activation: Section 63 error catalogue, Section 57 missing tables, Section
   59 missing endpoints, Section 61 Fleet-side event envelope and names.
2. Fleet-internal feature depth: 21, 22 (incl. CMP-027 payroll double-pay
   guard), 23, 43-56, 58.
3. Operations: 27 scheduler, 29/65 observability and runbooks, 30
   migration lock, 67 environment contract.
4. As activations land: 26/60/61 cross-service consumers and producers
   (after A1, then A2), then the Section 70 journeys and the Section 71
   checklist (after A3, A4).

The Section 2 status counts are the 2026-11-13 snapshot; Sections 5-20
work since then closed some of the Part 2 detail rows (37-56). Re-check a
row against `spec-v2-audit/` before starting it, and record any deviation
from the specification as an ADR (Section 72).
