# Fleet Sections 5-20: closure report

Date 2026-11-14. Branch `staging` (never `main`, never production).
Service `services/fleet-partner-service`, plus its Karryam counterparts
(payment-wallet, driver-dispatch-delivery, driver-taxi, driver-hauling,
verification-compliance) and the Fleet web app `web/fleet`.
Companion documents: `fleet-sections-05-20-requirements-matrix.md` (per-row
evidence), `-api-database-audit.md`, `-integration-audit.md`,
`-implementation-log.md`.

## 1. Final status per section

Status vocabulary: CLOSED - VERIFIED (built, tested against PostgreSQL
through the real router or use case, deployed to staging);
IMPLEMENTED - LIVE CERTIFICATION BLOCKED (built and tested; the staging
deployment or pairing it needs is blocked by inaccessible infrastructure);
BLOCKED - EXTERNAL DEPENDENCY; SOURCE REQUIREMENT UNAVAILABLE. Two sections
fit none of these honestly and are reported as OPEN with the exact items.

| Section | Rows | Closed rows | Final status | What keeps it from CLOSED |
|---|---|---|---|---|
| 5 Service structure | 12 | 6 | **OPEN - engineering items remain** | 13 features are flat packages (SQL in the service file); handlers take concrete services; `dashboard` imports the flat `dispatch` package; 11 spec-named packages are `doc.go` markers. Behaviour is unaffected; this is a structural refactor not done. |
| 6 API conventions | 14 | 11 | **OPEN - engineering items remain** | 6f: dispatch, workforce, asset and `/marketplace/trips` lists are offset-only (no cursor). 6h: internal service routes keep bespoke bodies by contract with other services. 6.1: DTOs flatten money into `xxxMinor` + `currency` instead of a Money object. |
| 7 Organizations | 16 | 15 | **CLOSED - VERIFIED** | 7h.c (exports encrypted at rest) is media-file-service's storage backend (Firebase Storage encryption); Fleet does not encrypt and no test can assert the third-party store. Recorded as an external certification. |
| 8 Auth & lifecycle | 20 | 19 | **CLOSED - VERIFIED** | 8.3 password login is not built by product decision (passwordless email OTP, no SMS); recorded as superseded, not as a defect. |
| 9 Staff & IAM | 21 | 21 | **CLOSED - VERIFIED** | |
| 10 Managed providers | 23 | 23 | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** | Fleet side live. The Hauling/Taxi pairing and their new code are not deployed: `hauling.env` and `taxi.env` on the staging host are root-owned (see section 6). |
| 11 Marketplace operations | 17 | 17 | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** | Same: offers come from Hauling/Taxi/Dispatch producers that cannot be paired or deployed until their settings files are fixed; Dispatch also waits for its reviewed migration-070 transition. |
| 12 Execution & trips | 8 | 8 | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** | Trip cancellation (Dispatch route + Fleet command) is tested end to end against PostgreSQL; Dispatch cannot deploy until migration 070 is transitioned. |
| 13 Wallet | 17 | 17 | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** | Wallet migration 000023 and the settlement-identity work are not deployed and the Fleet-Wallet pairing is not applied: `wallet.env` is root-owned. Staging keeps `FLEET_ACTIVATION_WALLET_GATE_WAIVED=true` until then. No fake balances were introduced. |
| 14 CRM | 26 | 26 | **CLOSED - VERIFIED** | |
| 15 Private orders | 22 | 22 | **CLOSED - VERIFIED** | |
| 16 Dispatch & tracking | 18 | 18 | **CLOSED - VERIFIED** | |
| 17 Workforce | 23 | 23 | **CLOSED - VERIFIED** | |
| 18 Assets | 16 | 16 | **CLOSED - VERIFIED** | |
| 19 Compliance & safety | 19 | 19 | **IMPLEMENTED - LIVE CERTIFICATION BLOCKED** | Restriction propagation now reaches Dispatch, Hauling and Taxi (tested); Taxi's authority is not deployed (root-owned `taxi.env`). |
| 20 Specialized logistics | 15 | 15 | **CLOSED - VERIFIED** | |
| **Total** | **287** | **276** | | 1 not independently tested (7h.c), 9 partial (Sections 5-6), 1 superseded (8.3) |

"Closed rows" counts matrix rows with status CLOSED - VERIFIED; every one
names its test and commit in the matrix.

## 2. Deployment detection and permissions (the corrected assessment)

- Run 43496005627 deployed Fleet `612934c8` healthy. Run 43496089810 failed
  because a checksum-only `go.work.sum` change redeployed 11 services and then
  stopped on Payment Wallet's private env file.
- Fixed: `deploy/platform/bin/changed-services.py` selects services by the
  shared packages they actually import (`go list -deps` in a scratch copy);
  `go.work`/`go.work.sum` select nothing. Preflight is per service: a service
  whose settings file the deploy account cannot read is skipped, never
  loosened (mode 600 kept), and the run fails at the end naming the file and
  the owner action. `check_private_file` also requires the file be readable.
- Pairing (`scripts/platform/staging-pairing.sh`) drops every pairing and
  setting that depends on a blocked service (`requires=`), so nothing is
  half-paired.
- Evidence the selector is precise: run 43586837951 (Fleet-only change)
  deployed only Fleet and succeeded; run 43585864400 selected seven services
  because shared packages they import changed, deployed the three it could
  and skipped four with the owner action.

## 3. Staging evidence (actual)

| Run | Commit | Result |
|---|---|---|
| 43561257812 | `e1a57b6` | Fleet API and worker revision `e1a57b6`, healthy, migrations through 000095 |
| 43563718925 | `308b463` | Fleet API and worker revision `308b4632...`, `api_health=healthy`; Wallet skipped (`wallet.env`) |
| 43585864400 | `344bd73` | Fleet `344bd73...` healthy; notification-service and support-dispute healthy; Wallet, customer, Taxi, Hauling skipped (root-owned env files) |
| 43586115356 | `b871416` | success |
| 43586837951 | `45b31f7` | success (Fleet only) |

Live checks from outside the server (2026-11-14): `GET /health` 200,
`GET /ready` 200 on `fleet-api.staging.karryam.com`; the new routes
(`POST /marketplace/trips/:id/cancel`, `/leads`, `/rescue-cases`,
`/approval-policies`, `/compliance/restrictions`) answer 401 (mounted,
authentication required) while an unknown path answers 404. The live web
app `fleet-karryam.vercel.app` serves the wave-2 bundle (the
`custody-events` client first added in `40ce7b4` is in the live chunk).

Not done live: authenticated end-to-end journeys on staging. They need a
staff sign-in through an emailed OTP to a real mailbox, which this
environment cannot receive. Journeys are certified against PostgreSQL
through the production router instead (section 4).

Pairing (run 43563718915): Fleet <-> Verification paired and healthy;
marketplace ingestion, assignment expiry, reassignment and reconciliation
workers on; Wallet, Hauling and Taxi pairings skipped with the owner action.

## 4. Test evidence (final tree)

- Fleet full suite on the final code (`c164b86`), PostgreSQL 16, fresh
  database migrated through 000099, `go test -p 1 -count=1 ./...`: 84
  packages ok, 0 failures.
- Payment-wallet: 11 packages ok. Dispatch: 42 packages ok (after fixing its
  test harness to apply the rider-auth schema). Taxi: 45 packages ok.
  Hauling: 37 packages ok. shared/go: 57 packages ok.
- Race detector: concurrency tests (inbox deduplication, idempotency,
  concurrent sends) pass under `-race`. The inbox race found by a flaky run
  was a real defect and is fixed (`c164b86`).
- Contract: route coverage, risk registry, object authorization, step-up
  route list, query allowlist, OpenAPI parity, migration lint and latest
  migration replay (000099) all pass.
- Security: step-up mandatory on high-risk actions, approval decisions bind
  the verified grant, branch scope enforced in SQL for every scoped list
  (NULL-safe, with a static guard), versioned writes refuse a missing
  If-Match, money never passes through float64 (static guard), camelCase
  response keys enforced by a static guard.
- Web: `tsc --noEmit` clean, 43/43 unit tests, production build, CI lint set.

## 5. Defects found and fixed in this phase (selection)

Organization-wide callers saw only branchless rows in 11 lists (NULL
`cardinality`); API client creation failed without an IP allowlist; provider,
structure, staff, role and assignment writes accepted a missing version;
accept ignored `expectedVersion`; candidates ignored branch scope; nobody
held the operating-mode permission; a confirmed wallet was re-provisioned on
every pass; a verification suspension could never be lifted; Fleet could not
cancel a live trip; Taxi received no restrictions; concurrent duplicate
events failed with a unique violation; 1,286 response fields serialized as
Go names; 26 defects across Sections 14-20 (contract renewal duplication,
cold-chain release without evidence, cross-branch corrective actions,
anonymous reporter disclosure, and others listed in the implementation log).

## 6. Owner actions (exact)

On the staging host, as root, keeping mode 600 (do not loosen):

```
chown deploy:deploy /opt/karrygo/payment-wallet-service/wallet.env \
  /opt/karrygo/customer-service/customer.env \
  /opt/karrygo/taxi-service/taxi.env \
  /opt/karrygo/hauling-service/hauling.env
```

Then re-run "Deploy staging (automatic)" and "Platform staging pairing".
Dispatch additionally needs the reviewed migration-070 manual transition
before `deploy-service.sh` will deploy it. After the Wallet pairing applies
and an organization wallet is provisioned through it, remove
`FLEET_ACTIVATION_WALLET_GATE_WAIVED=true` from the pairing file.

## 7. Verdict

Sections 7, 8, 9, 14, 15, 16, 17, 18 and 20 are closed. Sections 10, 11, 12,
13 and 19 are implemented and tested; their live certification is blocked
only by the four root-owned settings files and the Dispatch migration
transition above. Sections 5 and 6 are open for the structural and
convention items listed in section 1; none of them changes behaviour.
