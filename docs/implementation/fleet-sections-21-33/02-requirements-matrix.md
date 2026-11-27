# Fleet Sections 21-33: requirements matrix

Re-audited on 2026-11-27 against the full specification text (Part I Sections 21-33 and the binding
Part II catalogues 51.3-56.3 and 59.8-59.10) and the 871 mounted routes. Status values:

- **DONE**: implemented and proven by a test.
- **PRE-EXISTING**: implemented before this campaign and re-verified.
- **PATH DIFFERS**: the capability exists under another path; the specification itself names the same
  route differently in two places, so the path cannot be binding on its own (examples below).
- **NOT BUILT**: with the reason.

## How the path question was settled

The specification contradicts itself on paths: Section 51.3 says `/warehouse/receipts`, Section 59.8 says
`/warehouse-receipts`; Section 52.3 says `/payroll/periods`, Section 59.9 says `/payroll-periods`;
Section 23.1 says `/support/cases`, Sections 53.3 and 59.10 say `/support-cases`. A route was therefore
added under the specification's name only where every mention agrees and no equivalent exists. Two cases
met that bar (`/audit-events`, `POST /sessions/{id}/revoke`); both are thin aliases over the existing
handler. Everything else is matched by capability.

## Section 21 / 51 - Warehouse

| Requirement | Status | Evidence |
|---|---|---|
| Receipts: expected, received, damaged, quarantined; discrepancy blocks completion | DONE | `TestSection51WarehouseJourney` |
| Put-away and pick tasks leased, scan-validated (source, destination, item, quantity) | DONE | same |
| Concurrent picking cannot allocate the same units twice | DONE | WH-003 concurrent release |
| Stock changes only through append-only movements | DONE | movement immutability check; **fixed**: the order-level PICK command no longer re-picks a wave order (see 03) |
| Short pick | DONE | Spec requires no replanning (it is never mentioned). Fixed the real defect: the order dead-ended. Now cancellable once no task is open; picked units return to stock with a balancing RELEASE movement. Covered end to end, and the test fails on the old code. |
| Zones/bins (`/warehouses/{id}/zones`, `/bins`, `/warehouse-zones`, `/storage-bins`) | PATH DIFFERS | typed storage locations (`ZONE`, `BIN`, ...) under `/warehouses/:id/locations` |
| Inventory items, stock, adjustments | PATH DIFFERS | `/inventory`, `/inventory/receipts`, `/inventory/movements` (`ADJUSTMENT`) |
| Warehouse tasks | PATH DIFFERS | `/putaway-tasks`, `/pick-tasks` (claim/complete) |
| Dock overlap, yard state order, returns disposition | PRE-EXISTING / DONE | `section19_21contract`, gate-event ordering test |

## Section 22 / 52 - Finance

| Requirement | Status | Evidence |
|---|---|---|
| Payroll calculation fingerprint; changed inputs refuse approval | DONE | `TestSection52FinanceAnalysis` |
| Payroll finalize / allocate | PATH DIFFERS | `approve` / `post` |
| Credit notes, expense decisions, provider allocations approve/cancel | PATH DIFFERS | `/invoices/:id/credit-note`, `/expenses/:id/approve|reject`, `/providers/:id/sub-wallet/allocations/:id/approve|reject` |
| Ageing, profitability, cost allocation | DONE | unit + integration tests |
| Wallet remains the ledger authority | DONE | no ledger tables; postings via payment-wallet-service |

## Section 23 / 53 - Communication, support, SLA

| Requirement | Status | Evidence |
|---|---|---|
| Exceptions, SLA escalation, breaches, notification preferences | DONE | `TestSection53ExceptionEscalationJourney` |
| Quality reviews | PATH DIFFERS | `/quality/cases` |
| Support cases (create with organization context) | PRE-EXISTING | `/support/cases` (the path Section 23.1 itself uses) |
| `POST /support-cases/{id}/messages` (59.10) | NOT BUILT | Platform support cases belong to support-dispute-service (Section 23.1). Relaying messages needs a new cross-service contract; recorded as a future expansion. |

## Section 24 / 54 - Analytics and automation

| Requirement | Status | Evidence |
|---|---|---|
| Reports with permission and filter snapshots, async runs, expiry | DONE | `TestSection24AnalyticsRecommendationsAutomationAndGovernance` |
| Report events `report.ready` / `report.failed` (54.4) | DONE (**fixed**) | were never emitted; now published once with the state change (asserted) |
| Recommendations: input references, model/version, confidence, explanation, human decision, no protected writes | DONE | all fields present (migration 000103); rules-based, which 54.2 permits |
| Automation: approved catalogue, separation of duties, exactly once, loop guard | DONE | same test |
| `/reports/{id}/runs`, `/scheduled-reports`, `/automation-rules/{id}/enable|activate` | PATH DIFFERS | `/reports/:id/run`, schedule on the report definition, `approve` |

## Sections 25-27

| Requirement | Status | Evidence |
|---|---|---|
| Files, search, imports, exports, API clients, webhooks | PRE-EXISTING / PATH DIFFERS | `section25contract`; API clients and webhooks under `/integrations/...`; uploads under `/documents/upload-requests` |
| Section 26.2 core events | DONE | `internal/events/canonical.go` |
| Durable jobs | DONE | five job kinds plus warehouse sweep |

## Section 28 / 56 - Security, privacy, audit

| Requirement | Status | Evidence |
|---|---|---|
| `GET /security/sessions`, `POST /security/sessions/{id}/revoke` (59.10) | DONE (**new**) | over the auth feature's session store; organization-scoped, reasoned, audited, `security.session_revoked` event; integration test |
| `GET /sessions` (self) | PRE-EXISTING | auth feature |
| `POST /sessions/{id}/revoke` (56.3) | DONE (alias) | same handler as `DELETE /sessions/:id` |
| `GET /audit-events` (56.3 and 59.10) | DONE (alias) | same handler, filters, permission as `/audit-logs`; router test proves identical output |
| `POST /security/step-up` | PATH DIFFERS | two-phase `/auth/step-up/start` + `/verify` (an OTP challenge cannot be one call) |
| Retention policies / actions | PATH DIFFERS | `PUT /retention-policies/:class`, `/retention-runs` |
| Legal holds, privacy requests, security events | DONE | |
| Events `legal_hold.applied/released` (56.4) | DONE (**fixed**) | were audited only; now published (asserted) |
| Events `audit.recorded`, `security.risk_detected` (56.4) | NOT BUILT | `audit.recorded` would copy every audit row onto the outbox, duplicating the append-only audit history; no risk engine or `session_risk_records` contract is defined |
| `GET/PATCH /privacy-settings` (59.10) | NOT BUILT | No field definition anywhere in the specification (absent from the Section 57 schema catalogue); building it would mean inventing the contract |

## Sections 29-33

| Requirement | Status | Evidence |
|---|---|---|
| Reliability, observability, DR | PRE-EXISTING | `section29contract` |
| Reversible migrations | DONE | 000100-000105 replay/rollback; tip test at `000105` |
| Mandatory tests | DONE | see 06 |
| Release gates | DONE | all workflows green on the final code; see 09 |
