# Fleet Sections 21-33: final engineering closure

Written 2026-11-27, round 2. This version replaces the earlier 2026-11-27 closure (whose verdicts are
superseded) after the owner decided the two open contracts, authorized the notification-backlog
reconciliation, the remaining security deployments and authenticated testing with two staff accounts.

## A. Baseline and final commits

- Starting commit: `ba1959bd` on `staging`, no concurrent work. Fleet ran `d5409a21` at migration `000105`.
- Commits this round: `9ad46a91`, `0102c77f`, `283a7b24`, `73ab7ff9`, `dd9f8681`, `0cbe08e4`, plus this
  documentation commit. Final code commit: `0cbe08e4`.

## B. Audit findings and decisions (round 2)

| Item | Finding (verified, not taken from earlier reports) | Decision | Commit |
|---|---|---|---|
| `GET/PATCH /privacy-settings` | No contract in the spec; the retention-policy store, retention executors and legal holds already exist and are enforced | Owner decision: expose only enforced controls. A versioned view over the retention store (four executor-enforced classes writable, six protected classes read-only, platform minimums and legal holds unchanged) | `0102c77f` |
| `POST /support-cases/{id}/messages` | Support/Dispute already owns organization cases and chat messages (with a per-case client-id index) but had no organization-scoped, idempotent message contract | Minimal contract in Support/Dispute (create + read-back by client id); Fleet keeps a text-free submission ledger | `9ad46a91`, `0102c77f` |
| Notification backlog | Evidence run `45973177043`: **zero** intents, Section 22/23 jobs, posting operations and escalations. The earlier "backlog since 2026-11-13" was wrong. Real defects: failed intents retried every 30 s forever; notification-service lacked all three Fleet templates | Attempt cap, maximum age with read-back before closing, templates migration 012 | `0102c77f` |
| Section 22 workers | Only wallet read-backs and Fleet-owned updates; expense posting needs an accounting service that does not exist | Enabled; no money can move | `dd9f8681` |
| notification-service OpenSSL | Own image on `alpine:3.22` with 3.5.9-r0 | Patched; deployed `73ab7ff9`, 3.5.9-r2 in build log | `283a7b24` |
| vendor-service OpenSSL | Same base, missed earlier | Patched; image build verified by Vendor CI. Deploy needs the gated vendor release (independent reviewer) | `283a7b24` |
| Dispatch websocket | Still runs `8f992eee` (pre-fix). `deploy-service.sh` refuses every Dispatch deploy until a separately reviewed change lifts the 070 hold; the 070 transition script refuses to run again now that 070-079 are applied | Not deployed: no approved post-070 procedure exists | - |
| Pairing safety | Pairing would have replaced a callee's built-in caller list | Guard added and tested; Support/Dispute lists callers explicitly, all kept | `73ab7ff9` |

## C. Exact counts

| Item | Count |
|---|---|
| New tenant routes (Sections 21-33) | 75 (72 + `GET/PATCH /privacy-settings` + support-case messages) |
| Fleet migrations | `000100`-`000106`, all reversible; staging head `000106` |
| Other migrations this round | Support/Dispute `011` (idempotent, down keeps messages); notification `012` (templates) |
| Full Fleet suite (local, real PostgreSQL) | 100 packages, 0 failures |
| govulncheck | every module in `go.work` scanned without stopping; 0 called vulnerabilities |

## D. Remaining items, by kind

| Kind | Item |
|---|---|
| External authorization | Dispatch websocket fix deployment (needs a reviewed change to the 070 deploy hold); vendor image deployment (gated vendor release with an independent reviewer); notification migration 012 on staging (notification starts with `MIGRATION=false`; needs an approved migration run) |
| Staging data dependency | The only organization is `UNDER_REVIEW`, so the Section 22/23 schedulers enqueue nothing and pre-activation rules block most operational writes until it is activated by Verification |
| Deliberately not built | `audit.recorded` (would duplicate the audit history), `security.risk_detected` (no risk engine specified) |
| Missing test evidence | `-race` (no C compiler on this host); Support/Dispute database tests run only in CI |
| Authenticated live journeys | See E |

## E. Authenticated staging testing (2026-11-28, Fleet `0f1c4894`)

Run with two accounts the owner controls, each code supplied by the owner from the inbox. A local
client kept codes, tokens and step-up references out of logs and the repository. No approval rule was
relaxed and no platform-admin access was used.

| # | Journey | Result |
|---|---|---|
| E1 | Owner sign-in (email code), organization selection | 200; organization `Cosmicforge` is **ACTIVE** (was `UNDER_REVIEW`) |
| E2 | Owner invites a staff admin; invitation resent (token rotated) | Delivered, `deliveryStatus=ACCEPTED`; the earlier link stopped working |
| E3 | Staff accepts invitation (sign-up with invitation token + email code) | 200; membership ACTIVE, `isOwner=false`, role `rol_system_admin` |
| E4 | Owner raises a `VEHICLE_LINKING` approval (rule: 1 approver, self-approval off, separation of duties on) | 201 PENDING |
| E5 | Either account decides without step-up | 403 "requires additional verification" (`approvals.decide`) |
| E6 | Owner executes before approval | 422 "has not been approved yet" |
| E7 | Owner approves own request **with** a valid step-up | 403 "You cannot approve a request you raised." |
| E8 | Staff approves with a wrong fingerprint | 409 "operation changed since it was presented" |
| E9 | Staff approves (step-up, correct fingerprint) | 200 APPROVED; one decision recorded, approver = staff |
| E10 | Same approve repeated | 409 "no longer open" |
| E11 | Execute with a payload different from the approved one | 409; the approval is **invalidated** (`payload_changed`) and cannot be used afterwards, even with the right payload |
| E12 | Second request: staff approves, owner executes | 200 EXECUTED |
| E13 | Execute again (new reference, either account) | 409 "already executed" |

Observations, not blocking: the create response for a new approval carries `createdAt`
`0001-01-01T00:00:00Z` and an empty `meta.requestId` (the stored row and later reads are correct); an
execution reference used by a refused attempt is consumed and cannot be reused.

Not run as live journeys: the operational Sections 21-33 flows that need real providers, vehicles,
orders or money (no real-money transfers are allowed on staging), and the Section 22/23 workers'
processing now that the organization is active.

## F. Verdicts

| Verdict | Answer | Evidence or blocker |
|---|---|---|
| **SECTIONS_21_33_ENGINEERING_COMPLETE** | **YES** | Every mandatory route, contract, event, permission, worker and recovery path implemented and tested; the two former contract gaps are built (B, 02) |
| **SECTIONS_21_33_RELEASE_GATES_GREEN** | **YES** | Every workflow green on the final code commit `0cbe08e4` (G) |
| **SECTIONS_21_33_STAGING_INTEGRATION_COMPLETE** | **PARTIAL** | Done: deployed `0cbe08e4`/`73ab7ff9`, migrations applied, privacy settings and support messages live and protected, Support/Dispute paired, Section 22/23 workers on. Not done: notification migration 012 not applied; no active organization, so workers have nothing to process; Dispatch and vendor security images not deployed (authorization) |
| **SECTIONS_21_33_LIVE_E2E_CERTIFIED** | **PARTIAL** | Authentication, staff invitation and approval separation of duties certified live (E1-E13); operational journeys needing providers, vehicles, orders or money not run |

## G. CI on the final code commit `0cbe08e4`

| Workflow | Run | Result |
|---|---|---|
| Section 31 mandatory acceptance (gofmt, govulncheck over every module, full module suites) | `45985568691` | success |
| Vendor Sections 46-48 gates (incl. vendor image build, OpenSSL 3.5.9-r2) | `45985569083` | success |
| Deployment safety acceptance (incl. pairing tests) | `45985568732` | success |
| Customer V4 backend certification | `45985568758` | success |
| Admin bridge Sections 2-5 / Admin 16-38 / Admin owner reads and Dispatch safety | `45985568705`, `45985568697`, `45985568849` | success |
| Fleet authentication v2 / Partner Web split / Email branding | `45985568807`, `45985568709`, `45985568708` | success |
| Deploy staging (automatic) / read-only evidence | `45985568753`, `45988729997` | success |

The first push (`73ab7ff9`) failed Section 31 and the Vendor gate only on gofmt of two new files, fixed in `0cbe08e4`.
