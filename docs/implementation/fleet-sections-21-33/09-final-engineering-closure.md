# Fleet Sections 21-33: final engineering closure

Written 2026-11-27 after a read-only re-audit (01), targeted repairs (03), full verification (06),
controlled staging activation (07) and a second read-only audit (below). This version replaces the
closure written on 2026-11-26, whose verdicts are superseded.

## A. Baseline

- Starting commit: `59a2a00f` on `staging`, no concurrent work. Fleet ran `dc1569ed` on staging at
  migration `000104`, with every Sections 21-33 switch off.
- Two CI gates were red (Section 31 govulncheck, Admin Bridge Trivy), and a third blocker was hidden
  behind the first.
- The specification was reconciled route by route: 99 catalogue routes against 871 mounted routes.

## B. Repair decisions

| Item | Specification | Finding | Decision | Code changed |
|---|---|---|---|---|
| Short pick | 51.2 | Replanning is not required; the order dead-ended and the order-level PICK re-recorded stock | Guard PICK; allow CANCEL from PICKING once no task is open, with a balancing RELEASE movement | Yes (`f38b8305`) |
| `/security/sessions` (+ revoke) | 59.10 | Missing organization capability | Governance routes over the auth session store (port; no new store), `security.manage` | Yes (`f4fe0df9`) |
| `/audit-events` | 56.3, 59.10 | `/audit-logs` is the same contract; the spec names it consistently | Thin alias, same handler | Yes (`f4fe0df9`) |
| `POST /sessions/{id}/revoke` | 56.3 | Command form of the existing DELETE | Thin alias | Yes (`f4fe0df9`) |
| AI recommendations | 24, 54.2 | Every required field present; rules-based is allowed | Keep | No |
| Report events | 54.4 | `report.ready/failed` never emitted | Emit in the same transaction, once | Yes (`2b820123`) |
| Legal-hold events | 56.4 | Audited only | Emit `legal_hold.applied/released` | Yes (`2b820123`) |
| `audit.recorded`, `security.risk_detected` | 56.4 | The first would duplicate the audit history; no risk engine is defined | Not built; recorded | No |
| `/privacy-settings` | 59.10 | No field definition anywhere (absent from the Section 57 schema) | Not built: would mean inventing the contract | No |
| `/support-cases/{id}/messages` | 59.10 | Platform support belongs to support-dispute-service (23.1) | Not built: needs a cross-service contract | No |
| About 45 path differences | 51.3-59.10 | The spec names the same routes differently in different sections; capabilities exist | Keep current paths | No |
| Fleet image OpenSSL | 28.3, 33.2 | Same vulnerable base; no gate scanned it | Patch fleet's own Dockerfile | Yes (`ad478ab2`) |
| Shared image OpenSSL | 33.2 | Admin Bridge gate | Patch, owner-authorized (9-service redeploy) | Yes (`c85fb5be`) |
| Media OpenTelemetry | 31 | GO-2026-6974, hidden | Bump to released v1.47.0, owner-authorized | Yes (`b4f0c77a`) |
| Dispatch/Hauling websocket | 31 | GO-2026-6278; not exploitable at runtime; only an untagged fix exists | Bump, owner-authorized; new Hauling socket test | Yes (`d5409a21`, `befb4c33`) |
| Staging switches | 33.1 | All off | Enable Sections 21-28 groups; keep wallet-posting and notification-backlog workers off | Yes (`b92ecf79`) |

## C. Exact counts (re-derived from the repository)

| Item | Count |
|---|---|
| Commits for Sections 21-33 on `staging` | 17 (`a1fc7371`, `0c1cbab2`, `421b70a2`, `19607d23`, `ae9c641e`, `dc1569ed`, `59a2a00f`, `f38b8305`, `2b820123`, `f4fe0df9`, `ad478ab2`, `c85fb5be`, `b4f0c77a`, `d5409a21`, `b92ecf79`, `befb4c33`, plus this documentation commit) |
| New tenant routes | 72 (warehouse 24, finance 7, exceptions/SLA 8, preferences 3, analytics 20, governance 8, aliases 2) |
| New migrations | 6 (`000100`-`000105`), all reversible; head `000105_security_session_management` |
| New tables | 22 (no table added in final closure) |
| New permissions | 29 (28 + `security.manage`) |
| New durable job kinds | 5, plus the warehouse sweep |
| Spec events newly produced | 7 core (26.2) + `report_run.ready/failed`, `legal_hold.applied/released`, `security.session_revoked` |
| Services changed outside Fleet | 4 by owner-authorized dependency fixes only (shared image, media, Dispatch, Hauling); no business logic |
| Full fleet suite (local and CI) | 100 packages, 0 failures |

## D. Second read-only audit (after all changes)

1. **Scope**: of the 44 files changed since `59a2a00f`, 34 are Fleet. The rest are the authorized
   dependency fixes (2 Dispatch, 3 Hauling incl. a test, 2 media, the shared Dockerfile), one staging
   switch list and one repo-root OpenAPI file. No frontend file changed.
2. **No duplication**: no new session store, audit history, ledger or table; the only migration in final
   closure seeds one permission.
3. **Earlier sections still compatible**: the full fleet suite, including the Section 5-20 contract and
   integration suites, passes; the auth `DELETE /sessions/:id` and `/audit-logs` behave as before.
4. **Contracts**: OpenAPI parity, route coverage, risk, query-allowlist, object-authorization and
   pre-activation registries all pass with the new routes.
5. **Migrations**: replay, rollback and reapply through `000105` pass; staging reports head `000105`.
6. **Security**: new routes refuse anonymous callers on staging (401); session management refuses
   branch-scoped callers and other tenants; govulncheck clean on every module; all ten rebuilt images
   carry OpenSSL 3.5.9-r2.
7. **Claims**: every claim in 01-08 links to a test, a run ID or a probe result.

## E. Remaining items, separated by kind

| Kind | Item |
|---|---|
| Genuine unimplemented requirement (needs a decision, not just code) | `/privacy-settings` (no contract defined); `/support-cases/{id}/messages` (cross-service contract with support-dispute-service) |
| Specification-compliant behaviour, accepted | Rules-based recommendations; short pick resolved by cancellation, not automatic replanning; current paths where the spec names a route two ways |
| Deliberately not built | `audit.recorded` (would duplicate the audit history), `security.risk_detected` (no risk engine specified) |
| Missing test evidence | `-race` not run; worker activity on staging evidenced only by the running container |
| External authorization blockers | Authenticated live journeys need staff sign-in (OTP inbox access) and two distinct staff identities; `SECTION23_WORKERS_ENABLED` needs an owner decision about the notification backlog before SLA escalation can run on staging; Dispatch's websocket fix reaches its staging runtime only when its owner deploys Dispatch; notification-service has its own Dockerfile on the same vulnerable OpenSSL base (not authorized here) |
| Deferred by plan | Platform-wide authenticated end-to-end certification |

## F. Verdicts

| Verdict | Answer | Evidence or blocker |
|---|---|---|
| **SECTIONS_21_33_ENGINEERING_COMPLETE** | **YES**, except the two contract gaps in E (`/privacy-settings`, support-case messages), which need a product and cross-service decision rather than more code | All other mandatory requirements implemented and tested (02, 06) |
| **SECTIONS_21_33_RELEASE_GATES_GREEN** | **YES** | Every workflow green on the final code commit `befb4c33`, including Section 31 mandatory acceptance (`45949687541`), Admin Bridge (`45949687535`) and Customer V4 certification (`45949687656`). This documentation commit changes no code. |
| **SECTIONS_21_33_STAGING_INTEGRATION_COMPLETE** | **PARTIAL** | Done: deployed on `d5409a21` (healthy, migration head `000105`), capabilities active, workers running, routes live and protected. Not done: Section 23 workers left off pending the notification-backlog decision, so SLA escalation does not run on staging; no authenticated staging check |
| **SECTIONS_21_33_LIVE_E2E_CERTIFIED** | **NO** | No authenticated journey has run; needs signed-in staff with inbox access and two distinct identities |
